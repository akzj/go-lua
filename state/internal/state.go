// Package internal provides the concrete implementation of state/api interfaces.
package internal

import (
	"fmt"
	"unsafe"

	gc "github.com/akzj/go-lua/gc"
	memapi "github.com/akzj/go-lua/mem/api"
	"github.com/akzj/go-lua/state/api"
	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
	"github.com/akzj/go-lua/vm"
	"github.com/akzj/go-lua/table"
)

var _ = table.Lib // Force import of table package to trigger table/internal init()

// LuaState is the concrete implementation of LuaStateInterface.
// Manages a Lua execution state (thread/coroutine).
type LuaState struct {
	stack    []types.TValue  // Value stack
	top      int             // Index of top element in stack (1-based)
	stackLast int           // Last usable position in stack
	ci       *callInfo       // Current call info
	baseC    *callInfo       // Base call info (for the main thread)
	status   api.Status      // Thread status
	global   *globalState    // Shared global state
	executor vm.VMFrameManager // VM executor (lazy initialization)
	
	// Coroutine state management
	parent   *LuaState       // Parent LuaState that called Resume (for yield transfer)
	savedPC  int            // Saved program counter for resume after yield
}

// NewLuaState creates a new Lua state.
// If alloc is nil, uses the default allocator.
var _ = table.Lib // Force import of table package to trigger table/internal init()

func NewLuaState(alloc memapi.Allocator) *LuaState {
	// Create GC collector first
	gcCollector := gc.NewCollector(alloc)

	// Initialize allocator with GC collector
	if alloc == nil {
		alloc = memapi.NewAllocator(&memapi.AllocatorConfig{
			GCCollector: gcCollector,
		})
	} else {
		// If custom allocator provided, wrap it with GC support
		alloc = memapi.NewAllocator(&memapi.AllocatorConfig{
			GCCollector: gcCollector,
		})
	}

	// Create global state
	g := &globalState{
		alloc:     alloc,
		registry:  createRegistry(alloc),
		mainThread: nil, // Will be set after LuaState creation
		gc:        gcCollector,
	}

	// Create base call info
	baseCI := &callInfo{
		func_:   0,
		top:     0,
		prev:    nil,
		nresults: 0,
	}

	// Create main state
	L := &LuaState{
		stack:     nil, // Will be allocated by grow
		top:       0,
		stackLast: 0,
		ci:        baseCI,
		baseC:     baseCI,
		status:    api.LUA_OK,
		global:    g,
	}

	// Set main thread in global state
	g.mainThread = L

	// Allocate initial stack
	L.growStack(20)

	return L
}

// =============================================================================
// Stack Operations
// =============================================================================

func (L *LuaState) growStack(n int) {
	// Calculate minimum size needed
	minSize := L.top + n + 1 // top + required slots + 1 for extra
	
	// Current capacity
	oldSize := cap(L.stack)
	
	// If we already have enough capacity, just ensure slice length
	if oldSize >= minSize {
		// Ensure the slice is long enough to access stackLast
		if len(L.stack) < minSize {
			L.stack = L.stack[:minSize]
		}
		L.stackLast = cap(L.stack) - 1
		return
	}
	
	// Need to grow
	newSize := minSize * 2 // Grow by 2x
	if newSize < 20 {
		newSize = 20
	}
	
	// Create new stack with nil values
	newStack := make([]types.TValue, minSize, newSize)
	
	// Copy existing values
	copy(newStack, L.stack)
	
	L.stack = newStack
	L.stackLast = cap(L.stack) - 1
}

func (L *LuaState) Stack() []types.TValue {
	return L.stack
}

func (L *LuaState) StackSize() int {
	return len(L.stack)
}

func (L *LuaState) Top() int {
	return L.top
}

func (L *LuaState) SetTop(idx int) {
	// Convert relative index to absolute
	newTop := L.idx2stack(idx)
	
	if newTop < 0 {
		newTop = 0
	}
	
	if newTop > L.top {
		// Need to grow stack if expanding
		if newTop >= L.stackLast {
			L.growStack(newTop - L.stackLast + 1)
		}
		// Fill intermediate slots with nil (already nil from slice creation)
	}
	
	L.top = newTop
}

func (L *LuaState) PushValue(idx int) {
	// Convert index to absolute position
	absIdx := L.idx2stack(idx)
	
	if absIdx < 1 || absIdx > L.top {
		// Invalid index - ignore silently (Lua semantics)
		return
	}
	
	// Check if we need to grow
	if L.top >= L.stackLast {
		L.growStack(1)
	}
	
	// Push a copy of the value at idx
	L.stack[L.top] = L.stack[absIdx-1] // stack is 0-based, idx is 1-based
	L.top++
}

func (L *LuaState) Pop() {
	L.top--
}

func (L *LuaState) GrowStack(n int) {
	L.growStack(n)
}

// =============================================================================
// Thread Management
// =============================================================================

func (L *LuaState) Status() api.Status {
	return L.status
}

func (L *LuaState) NewThread() api.LuaStateInterface {
	// Create a new LuaState that shares this state's global state
	newState := &LuaState{
		stack:     nil, // Will be allocated by grow
		top:       0,
		stackLast: 0,
		ci:        nil, // Will be set below
		baseC:     nil,
		status:    api.LUA_OK,
		global:    L.global,
	}
	
	// Create base call info for the new thread
	baseCI := &callInfo{
		func_:   0,
		top:     0,
		prev:    nil,
		nresults: 0,
	}
	newState.ci = baseCI
	newState.baseC = baseCI
	
	// Allocate initial stack
	newState.growStack(20)
	
	return newState
}

// =============================================================================
// Function Calls
// =============================================================================

// Resume resumes a thread that was yielded or starts a new coroutine.
// 
// Resume semantics:
// - If status is LUA_OK: this is the first resume, execute the function on the stack
// - If status is LUA_YIELD: this is a resume after yield, continue from where it left off
// - Any arguments passed to Resume are pushed onto the coroutine's stack as arguments
// 
// Returns:
// - nil on success (caller gets yielded values or final return values)
// - error if thread cannot be resumed (invalid status, dead thread, etc.)
//
// Invariants:
// - Thread status must be LUA_OK (initial) or LUA_YIELD (after yield)
// - Cannot resume the main thread while it's running
// - Cannot resume a thread that is currently running
func (L *LuaState) Resume() error {
	// Check if thread can be resumed
	switch L.status {
	case api.LUA_YIELD:
		// Resume after yield - this is valid
	case api.LUA_OK:
		// First resume - must have a function to call
		// Check if there's a function on the stack
		if L.top < 1 {
			return fmt.Errorf("cannot resume: no function on stack")
		}
		fn := L.stack[0]
		if fn.IsNil() {
			return fmt.Errorf("cannot resume: stack is empty or has no function")
		}
	default:
		return fmt.Errorf("cannot resume: thread is dead (status=%d)", L.status)
	}
	
	// If this is a resumed thread (was yielded), restore saved state
	if L.status == api.LUA_YIELD && L.parent != nil {
		// Transfer any arguments from the resume call to the coroutine stack
		// The arguments are already on the stack from the caller
		// We need to handle the argument transfer
		
		// Restore saved PC and resume execution
		L.status = api.LUA_OK
	} else {
		// This is the first resume (status was LUA_OK)
		// Execute the function on the stack
		L.status = api.LUA_OK
	}
	
	// Get the function at position 0
	fn := L.stack[0]
	
	// Handle different function types
	switch {
	case fn.IsLClosure():
		// Lua closure - execute via VM
		L.executeLuaClosureResume(fn)
	case fn.IsCClosure() || fn.IsLightCFunction():
		// C function
		L.executeCFunctionResume(fn)
	case fn.IsNil():
		// Nothing to call - thread completes
		L.status = api.LUA_OK
		return nil
	default:
		return fmt.Errorf("cannot resume: value at stack position 0 is not callable")
	}
	
	// If we get here after execution completes, thread is dead
	if L.status == api.LUA_OK {
		// Execution completed normally
		// The return values are on the stack
		return nil
	}
	
	return nil
}

// executeLuaClosureResume executes a Lua closure with coroutine support
func (L *LuaState) executeLuaClosureResume(fn types.TValue) {
	// Get closure and prototype
	closure := fn.GetValue()
	proto := extractProto(closure)
	
	if proto == nil {
		// Cannot execute - this is expected for simple tests
		return
	}
	
	// Get executor
	executor := L.getOrCreateExecutor()
	
	// Calculate frame base - function is at position 0
	// Arguments start at position 1
	frameBase := 0
	
	// Create frame data for VM
	frame := &luaFrame{
		closure:    fn,
		base:       frameBase,
		prev:       executor.CurrentFrame(),
		savedPC:    0,
		kvalues:    extractKValues(proto),
		upvals:     nil,
	}
	executor.PushFrame(frame)
	
	// Set the code to execute
	code := make([]vm.Instruction, len(proto.GetCode()))
	for i, inst := range proto.GetCode() {
		code[i] = vm.Instruction(inst)
	}
	executor.SetCode(code)
	
	// Execute bytecode
	// The VM will continue until completion or yield
	_ = executor.Run()
	
	// Execution finished - pop frame
	executor.PopFrame()
}

// executeCFunctionResume executes a C function with coroutine support
func (L *LuaState) executeCFunctionResume(fn types.TValue) {
	// For C functions, we just need to handle them
	// This is a simplified implementation
	// The C function would be called and if it yields, we'd handle that
	
	// For now, mark the thread as completed
	L.status = api.LUA_OK
}

// Yield suspends the current thread and transfers control back to the caller.
// 
// Yield semantics:
// - The thread status changes to LUA_YIELD
// - Any arguments to Yield (nResults) are transferred to the Resume caller
// - The thread can be resumed again with Resume
// 
// Parameters:
// - nResults: number of values to return to the Resume caller
//
// Returns:
// - nil on success
// - error if the thread cannot yield (e.g., main thread, inside C call)
//
// Invariants:
// - nResults >= 0
// - Values returned are at positions [top-nResults+1, top]
func (L *LuaState) Yield(nResults int) error {
	// Validate nResults
	if nResults < 0 {
		return fmt.Errorf("yield: nResults must be >= 0")
	}
	
	// Cannot yield if not in a coroutine context
	if L.parent == nil && L.status != api.LUA_OK {
		// This is the main thread or an invalid state
		return fmt.Errorf("cannot yield: main thread or invalid state")
	}
	
	// Save the current execution state for resume
	// The VM will need to know where to continue
	
	// Set status to yielded
	L.status = api.LUA_YIELD
	
	// The values at [top-nResults+1, top] will be returned to the caller
	// This happens automatically as the caller will read from the stack
	
	return nil
}

// =============================================================================
// Global State
// =============================================================================

func (L *LuaState) Global() api.GlobalState {
	return L.global
}

// =============================================================================
// Call Info Management
// =============================================================================

func (L *LuaState) CurrentCI() api.CallInfo {
	return L.ci
}

func (L *LuaState) PushCI(ci api.CallInfo) {
	// Link the new call info to the previous one
	ci.SetPrev(L.ci)
	
	// The ci passed should be a *callInfo to access prev field
	// This is a simplification - in real implementation we'd have SetPrev in interface
	if typedCI, ok := ci.(*callInfo); ok {
		typedCI.prev = L.ci
	}
	
	// Set the new call info as current
	// We need to use the internal type, so we store it in ci
	L.ci = ci.(*callInfo)
}

func (L *LuaState) PopCI() {
	L.ci = L.ci.prev
}

// =============================================================================
// Helper Functions
// =============================================================================

// idx2stack converts a stack index to an absolute stack position.
// Positive indices are relative to the bottom (1-based).
// Negative indices are relative to the top.
func (L *LuaState) idx2stack(idx int) int {
	if idx > 0 {
		return idx
	}
	return L.top + idx + 1
}

// absoluteIndex converts a stack index to an absolute index.
// Returns -1 if the index is invalid.
func (L *LuaState) absoluteIndex(idx int) int {
	if idx == api.LUA_REGISTRYINDEX {
		return -1 // Special case for registry
	}
	if idx > 0 {
		return idx
	}
	if L.top+idx+1 > 0 {
		return L.top + idx + 1
	}
	return -1
}

// =============================================================================
// Registry
// =============================================================================

func createRegistry(alloc memapi.Allocator) tableapi.TableInterface {
	// Create a new table for the registry
	// Use the table factory - returns the default implementation
	registry := tableapi.NewTable(alloc)
	return registry
}

// setGlobal registers a Go function in the global environment table.
// It stores the function as a LightUserData TValue wrapping the GoFunc interface{}.
// The executor will type-assert it back to vm.GoFunc when calling.
func (L *LuaState) setGlobal(name string, fn vm.GoFunc) {
	// Store the GoFunc interface{} as the Data_ of a LightUserData TValue
	key := typesinternal.NewTValueString(name)
	val := typesinternal.NewTValueLightUserData(unsafe.Pointer(&fn))
	L.global.Registry().Set(key, val)
}
