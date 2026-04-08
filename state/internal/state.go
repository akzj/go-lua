// Package internal provides the concrete implementation of state/api interfaces.
package internal

import (
	"unsafe"
	"fmt"
	"strconv"

	gc "github.com/akzj/go-lua/gc"
	memapi "github.com/akzj/go-lua/mem/api"
	"github.com/akzj/go-lua/state/api"
	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
	"github.com/akzj/go-lua/vm"
	"github.com/akzj/go-lua/table"
	bcapi "github.com/akzj/go-lua/bytecode/api"
	bc "github.com/akzj/go-lua/bytecode"
	"github.com/akzj/go-lua/parse"
)

// =============================================================================
// LuaError — distinct error type for Lua error() mechanism
// =============================================================================

// LuaError is the panic value used by error(). pcall/xpcall catch this
// specific type via recover() + type assertion, distinguishing Lua errors
// from Go bugs.
type LuaError struct {
	Msg types.TValue // The error object (usually a string)
}

func (e *LuaError) Error() string {
	if e.Msg == nil || e.Msg.IsNil() {
		return "error object is nil"
	}
	if e.Msg.IsString() {
		if s, ok := e.Msg.GetValue().(string); ok {
			return s
		}
	}
	if e.Msg.IsInteger() {
		return fmt.Sprintf("%d", e.Msg.GetInteger())
	}
	if e.Msg.IsFloat() {
		return fmt.Sprintf("%g", e.Msg.GetFloat())
	}
	return "(error object)"
}

// luaError is a helper that panics with a LuaError.
func luaError(msg types.TValue) {
	panic(&LuaError{Msg: msg})
}

// luaErrorString is a convenience helper for string error messages.
func luaErrorString(msg string) {
	panic(&LuaError{Msg: types.NewTValueString(msg)})
}

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
	
	lastErr  error          // Last execution error (set by Call, checked by DoStringOn)

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

	// Open base library — register Go functions in the global environment
	L.openBaseLib()

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
	// Use the internal package directly to get fresh table instances
	registry := table.NewTable()
	return registry
}

// goFuncWrapper wraps a GoFunc so it can be stored in tables.
// Implements types.TValue (all 27 methods).
type goFuncWrapper struct {
	fn vm.GoFunc
}

func (w *goFuncWrapper) IsNil() bool             { return false }
func (w *goFuncWrapper) IsBoolean() bool          { return false }
func (w *goFuncWrapper) IsNumber() bool           { return false }
func (w *goFuncWrapper) IsInteger() bool          { return false }
func (w *goFuncWrapper) IsFloat() bool            { return false }
func (w *goFuncWrapper) IsString() bool           { return false }
func (w *goFuncWrapper) IsTable() bool            { return false }
func (w *goFuncWrapper) IsFunction() bool         { return true }
func (w *goFuncWrapper) IsThread() bool            { return false }
func (w *goFuncWrapper) IsUserData() bool         { return false }
func (w *goFuncWrapper) IsLightUserData() bool    { return false }
func (w *goFuncWrapper) IsCollectable() bool     { return true }
func (w *goFuncWrapper) IsTrue() bool             { return true }
func (w *goFuncWrapper) IsFalse() bool            { return false }
func (w *goFuncWrapper) IsLClosure() bool        { return true }
func (w *goFuncWrapper) IsCClosure() bool         { return false }
func (w *goFuncWrapper) IsLightCFunction() bool   { return false }
func (w *goFuncWrapper) IsClosure() bool          { return true }
func (w *goFuncWrapper) IsProto() bool            { return false }
func (w *goFuncWrapper) IsUpval() bool            { return false }
func (w *goFuncWrapper) IsShortString() bool      { return false }
func (w *goFuncWrapper) IsLongString() bool       { return false }
func (w *goFuncWrapper) IsEmpty() bool            { return false }
func (w *goFuncWrapper) GetTag() int              { return types.Ctb(int(types.LUA_VLCL)) }
func (w *goFuncWrapper) GetBaseType() int         { return int(types.LUA_TFUNCTION) }
func (w *goFuncWrapper) GetValue() interface{}   { return w.fn }
func (w *goFuncWrapper) GetGC() *types.GCObject  { return nil }
func (w *goFuncWrapper) GetInteger() types.LuaInteger { return 0 }
func (w *goFuncWrapper) GetFloat() types.LuaNumber   { return 0 }
func (w *goFuncWrapper) GetPointer() unsafe.Pointer { return nil }

// unwrapGoFunc returns the underlying GoFunc for VM invocation.
func (w *goFuncWrapper) unwrapGoFunc() vm.GoFunc { return w.fn }

// setGlobal registers a Go function in the global environment table.
func (L *LuaState) setGlobal(name string, fn vm.GoFunc) {
	key := types.NewTValueString(name)
	val := &goFuncWrapper{fn: fn}
	L.global.Registry().Set(key, val)
}

// setGlobalValue registers an arbitrary TValue in the global environment table.
func (L *LuaState) setGlobalValue(name string, val types.TValue) {
	key := types.NewTValueString(name)
	L.global.Registry().Set(key, val)
}

// tableWrapper wraps a tableapi.TableInterface so it can be stored as a types.TValue.
// Implements types.TValue (all 27 methods).
// Note: IsCollectable returns false to avoid triggering the GC code path in the VM.
// Tables are managed by the Go GC, not Lua's GC system.
type tableWrapper struct {
	tbl tableapi.TableInterface
}

func (w *tableWrapper) IsNil() bool             { return w.tbl == nil }
func (w *tableWrapper) IsBoolean() bool          { return false }
func (w *tableWrapper) IsNumber() bool           { return false }
func (w *tableWrapper) IsInteger() bool          { return false }
func (w *tableWrapper) IsFloat() bool            { return false }
func (w *tableWrapper) IsString() bool           { return false }
func (w *tableWrapper) IsTable() bool            { return w.tbl != nil }
func (w *tableWrapper) IsFunction() bool         { return false }
func (w *tableWrapper) IsThread() bool            { return false }
func (w *tableWrapper) IsUserData() bool         { return false }
func (w *tableWrapper) IsLightUserData() bool    { return false }
func (w *tableWrapper) IsCollectable() bool     { return false } // Key fix: not collectable, avoids GC code path
func (w *tableWrapper) IsTrue() bool             { return true }
func (w *tableWrapper) IsFalse() bool            { return false }
func (w *tableWrapper) IsLClosure() bool        { return false }
func (w *tableWrapper) IsCClosure() bool         { return false }
func (w *tableWrapper) IsLightCFunction() bool   { return false }
func (w *tableWrapper) IsClosure() bool          { return false }
func (w *tableWrapper) IsProto() bool            { return false }
func (w *tableWrapper) IsUpval() bool            { return false }
func (w *tableWrapper) IsShortString() bool      { return false }
func (w *tableWrapper) IsLongString() bool       { return false }
func (w *tableWrapper) IsEmpty() bool            { return w.tbl == nil }
func (w *tableWrapper) GetTag() int              { return int(types.Ctb(int(types.LUA_VTABLE))) }
func (w *tableWrapper) GetBaseType() int         { return int(types.LUA_TTABLE) }
func (w *tableWrapper) GetValue() interface{}    { return w.tbl }
func (w *tableWrapper) GetGC() *types.GCObject   { return nil }
func (w *tableWrapper) GetInteger() types.LuaInteger { return 0 }
func (w *tableWrapper) GetFloat() types.LuaNumber   { return 0 }
func (w *tableWrapper) GetPointer() unsafe.Pointer { return nil }

// newTable creates a new empty table wrapped as a types.TValue.
func newTableTValue() types.TValue {
	return &tableWrapper{tbl: table.NewTable()}
}

// createModuleTable is a helper to create a fresh module table.
func createModuleTable() tableapi.TableInterface {
	return table.NewTable()
}

// =============================================================================
// Base Library — Go function implementations
// =============================================================================

// bprint implements Lua's print function.
// Pushes no return values, prints arguments to stdout.
func bprint(stack []types.TValue, base int) int {
	for i := 1; i < len(stack)-base; i++ {
		if i > 1 {
			fmt.Print("\t")
		}
		v := stack[base+i]
		if v == nil || v.IsNil() {
			fmt.Print("nil")
		} else if v.IsInteger() {
			fmt.Print(v.GetInteger())
		} else if v.IsFloat() {
			fmt.Print(v.GetFloat())
		} else if v.IsBoolean() {
			if v.IsTrue() {
				fmt.Print("true")
			} else {
				fmt.Print("false")
			}
		} else if v.IsString() {
			if s, ok := v.GetValue().(string); ok {
				fmt.Print(s)
			}
		} else if v.IsFunction() {
			fmt.Printf("function: %p", v.GetValue())
		} else if v.IsTable() {
			fmt.Printf("table: %p", v.GetValue())
		} else {
			fmt.Print(v.GetBaseType())
		}
	}
	fmt.Println()
	return 0
}

// btype implements Lua's type function.
// Returns the type name of the value at stack[base+1].
func btype(stack []types.TValue, base int) int {
	if base+1 < len(stack) {
		v := stack[base+1]
		var t string
		switch {
		case v == nil || v.IsNil():
			t = "nil"
		case v.IsInteger():
			t = "number"
		case v.IsFloat():
			t = "number"
		case v.IsBoolean():
			t = "boolean"
		case v.IsString():
			t = "string"
		case v.IsFunction():
			t = "function"
		case v.IsTable():
			t = "table"
		case v.IsThread():
			t = "thread"
		case v.IsUserData():
			t = "userdata"
		case v.IsLightUserData():
			t = "userdata"
		default:
			t = "unknown"
		}
		stack[base] = types.NewTValueString(t)
	} else {
		stack[base] = types.NewTValueNil()
	}
	return 1
}

// bassert implements Lua's assert function.
// Uses LuaError so pcall can catch assertion failures.
func bassert(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 1 {
		luaErrorString("bad argument #1 to 'assert' (value expected)")
		return 0
	}
	v := stack[base+1]
	if v == nil || v.IsFalse() || v.IsNil() {
		msg := "assertion failed!"
		if nArgs >= 2 && base+2 < len(stack) {
			if m := stack[base+2]; m != nil && !m.IsNil() {
				if s, ok := m.GetValue().(string); ok {
					msg = s
				} else if m.IsInteger() {
					msg = fmt.Sprintf("%d", m.GetInteger())
				} else if m.IsFloat() {
					msg = fmt.Sprintf("%g", m.GetFloat())
				}
			}
		}
		luaErrorString(msg)
		return 0
	}
	// Return all arguments (assert returns its arguments on success)
	for i := 0; i < nArgs; i++ {
		stack[base+i] = stack[base+1+i]
	}
	return nArgs
}

// btostring implements Lua's tostring function.
func btostring(stack []types.TValue, base int) int {
	if base+1 < len(stack) {
		v := stack[base+1]
		var s string
		switch {
		case v.IsNil():
			s = "nil"
		case v.IsInteger():
			s = fmt.Sprintf("%d", v.GetInteger())
		case v.IsFloat():
			s = fmt.Sprintf("%g", v.GetFloat())
		case v.IsBoolean():
			if v.IsTrue() {
				s = "true"
			} else {
				s = "false"
			}
		case v.IsString():
			if sv, ok := v.GetValue().(string); ok {
				s = sv
			}
		case v.IsTable():
			s = "table: " + fmt.Sprintf("%p", v)
		case v.IsFunction():
			s = "function: " + fmt.Sprintf("%p", v)
		default:
			s = ""
		}
		stack[base] = types.NewTValueString(s)
	} else {
		stack[base] = types.NewTValueNil()
	}
	return 1
}

// btonumber implements Lua's tonumber function.
func btonumber(stack []types.TValue, base int) int {
	if base+1 < len(stack) {
		v := stack[base+1]
		if v.IsInteger() {
			stack[base] = types.NewTValueInteger(v.GetInteger())
			return 1
		}
		if v.IsFloat() {
			stack[base] = types.NewTValueFloat(v.GetFloat())
			return 1
		}
		if v.IsString() {
			if s, ok := v.GetValue().(string); ok {
				// Try integer first
				var i int64
				if n, err := fmt.Sscanf(s, "%d", &i); err == nil && n == 1 {
					stack[base] = types.NewTValueInteger(types.LuaInteger(i))
					return 1
				}
				// Try float
				var f float64
				if n, err := fmt.Sscanf(s, "%g", &f); err == nil && n == 1 {
					stack[base] = types.NewTValueFloat(types.LuaNumber(f))
					return 1
				}
			}
		}
	}
	stack[base] = types.NewTValueNil()
	return 1
}

// =============================================================================
// Phase 2 Base Library Functions
// =============================================================================

// berror implements Lua's error(msg [, level]) function.
// Raises a Lua error by panicking with LuaError.
func berror(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 1 {
		luaError(types.NewTValueNil())
		return 0
	}
	msg := stack[base+1]
	luaError(msg)
	return 0 // unreachable
}

// bpcall implements Lua's pcall(f, ...) function.
// Calls f in protected mode. If f raises an error, returns false + errmsg.
// If f succeeds, returns true + results.
func bpcall(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 1 {
		stack[base] = types.NewTValueBoolean(false)
		stack[base+1] = types.NewTValueString("bad argument #1 to 'pcall' (value expected)")
		return 2
	}

	fn := stack[base+1]

	// Build args for the called function: fn at [0], extra args at [1..]
	extraArgs := nArgs - 1
	callStack := make([]types.TValue, 1+extraArgs)
	callStack[0] = fn
	for i := 0; i < extraArgs; i++ {
		callStack[i+1] = stack[base+2+i]
	}

	// Try to call the function, catching LuaError panics
	var nRet int
	var luaErr *LuaError
	func() {
		defer func() {
			if r := recover(); r != nil {
				if le, ok := r.(*LuaError); ok {
					luaErr = le
				} else {
					// Re-panic for Go bugs (non-LuaError panics)
					panic(r)
				}
			}
		}()
		// Extract the GoFunc from the function value
		if gf, ok := fn.GetValue().(vm.GoFunc); ok {
			nRet = gf(callStack, 0)
		} else {
			luaErr = &LuaError{Msg: types.NewTValueString("attempt to call a non-function value")}
		}
	}()

	if luaErr != nil {
		stack[base] = types.NewTValueBoolean(false)
		if luaErr.Msg != nil {
			stack[base+1] = luaErr.Msg
		} else {
			stack[base+1] = types.NewTValueNil()
		}
		return 2
	}

	// Success: return true + results
	stack[base] = types.NewTValueBoolean(true)
	for i := 0; i < nRet; i++ {
		stack[base+1+i] = callStack[i]
	}
	return 1 + nRet
}

// bxpcall implements Lua's xpcall(f, handler, ...) function.
// Like pcall but with a message handler function.
func bxpcall(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 2 {
		stack[base] = types.NewTValueBoolean(false)
		stack[base+1] = types.NewTValueString("bad argument #1 to 'xpcall' (value expected)")
		return 2
	}

	fn := stack[base+1]
	handler := stack[base+2]

	// Build args for the called function
	extraArgs := nArgs - 2
	callStack := make([]types.TValue, 1+extraArgs)
	callStack[0] = fn
	for i := 0; i < extraArgs; i++ {
		callStack[i+1] = stack[base+3+i]
	}

	var nRet int
	var luaErr *LuaError
	func() {
		defer func() {
			if r := recover(); r != nil {
				if le, ok := r.(*LuaError); ok {
					luaErr = le
				} else {
					panic(r)
				}
			}
		}()
		if gf, ok := fn.GetValue().(vm.GoFunc); ok {
			nRet = gf(callStack, 0)
		} else {
			luaErr = &LuaError{Msg: types.NewTValueString("attempt to call a non-function value")}
		}
	}()

	if luaErr != nil {
		// Call the handler with the error message
		errMsg := luaErr.Msg
		if errMsg == nil {
			errMsg = types.NewTValueNil()
		}

		// Try to call handler
		handlerStack := []types.TValue{handler, errMsg}
		var handlerResult types.TValue
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Handler itself failed - use original error
					handlerResult = errMsg
				}
			}()
			if hf, ok := handler.GetValue().(vm.GoFunc); ok {
				nH := hf(handlerStack, 0)
				if nH > 0 {
					handlerResult = handlerStack[0]
				} else {
					handlerResult = errMsg
				}
			} else {
				handlerResult = errMsg
			}
		}()

		stack[base] = types.NewTValueBoolean(false)
		stack[base+1] = handlerResult
		return 2
	}

	// Success
	stack[base] = types.NewTValueBoolean(true)
	for i := 0; i < nRet; i++ {
		stack[base+1+i] = callStack[i]
	}
	return 1 + nRet
}

// bselect implements Lua's select(index, ...) function.
func bselect(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 1 {
		luaErrorString("bad argument #1 to 'select' (number or string expected, got no value)")
		return 0
	}

	idx := stack[base+1]

	// select('#', ...) returns count of varargs
	if idx.IsString() {
		if s, ok := idx.GetValue().(string); ok && s == "#" {
			count := nArgs - 1 // everything after the index arg
			stack[base] = types.NewTValueInteger(types.LuaInteger(count))
			return 1
		}
	}

	// select(n, ...) returns from nth element onwards
	var n int
	if idx.IsInteger() {
		n = int(idx.GetInteger())
	} else if idx.IsFloat() {
		n = int(idx.GetFloat())
	} else {
		luaErrorString("bad argument #1 to 'select' (number or string expected)")
		return 0
	}

	varargCount := nArgs - 1
	if n < 0 {
		n = varargCount + n + 1
	}
	if n < 1 {
		luaErrorString("bad argument #1 to 'select' (index out of range)")
		return 0
	}
	if n > varargCount {
		return 0
	}

	// Return from nth vararg onwards
	nRet := varargCount - n + 1
	for i := 0; i < nRet; i++ {
		stack[base+i] = stack[base+1+n+i]
	}
	return nRet
}

// extractTable extracts a tableapi.TableInterface from a TValue.
func extractTable(v types.TValue) tableapi.TableInterface {
	if v == nil || v.IsNil() {
		return nil
	}
	if !v.IsTable() {
		return nil
	}
	val := v.GetValue()
	if tbl, ok := val.(tableapi.TableInterface); ok {
		return tbl
	}
	return nil
}

// bnext implements Lua's next(table [, index]) function.
func bnext(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 1 {
		luaErrorString("bad argument #1 to 'next' (table expected, got no value)")
		return 0
	}

	tbl := extractTable(stack[base+1])
	if tbl == nil {
		luaErrorString("bad argument #1 to 'next' (table expected)")
		return 0
	}

	var key types.TValue
	if nArgs >= 2 && stack[base+2] != nil {
		key = stack[base+2]
	} else {
		key = types.NewTValueNil()
	}

	nextKey, nextVal, ok := tbl.Next(key)
	if !ok {
		stack[base] = types.NewTValueNil()
		return 1
	}
	stack[base] = nextKey
	stack[base+1] = nextVal
	return 2
}

// bipairsIter is the iterator function for ipairs.
// It captures the table and returns the next integer key-value pair.
func makeIpairsIter(tbl tableapi.TableInterface) vm.GoFunc {
	return func(stack []types.TValue, base int) int {
		// stack[base+1] = table (invariant state), stack[base+2] = control variable (index)
		var idx types.LuaInteger
		nArgs := len(stack) - base - 1
		if nArgs >= 2 && stack[base+2] != nil && stack[base+2].IsInteger() {
			idx = stack[base+2].GetInteger()
		}
		idx++

		val := tbl.GetInt(idx)
		if val == nil || val.IsNil() {
			stack[base] = types.NewTValueNil()
			return 1
		}
		stack[base] = types.NewTValueInteger(idx)
		stack[base+1] = val
		return 2
	}
}

// bipairs implements Lua's ipairs(t) function.
// Returns iterator function, table, 0
func bipairs(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 1 {
		luaErrorString("bad argument #1 to 'ipairs' (table expected, got no value)")
		return 0
	}

	tbl := extractTable(stack[base+1])
	if tbl == nil {
		luaErrorString("bad argument #1 to 'ipairs' (table expected)")
		return 0
	}

	// Return: iterator function, table, 0
	iterFn := makeIpairsIter(tbl)
	stack[base] = &goFuncWrapper{fn: iterFn}
	stack[base+1] = stack[base+1] // table (invariant state) - already there
	stack[base+2] = types.NewTValueInteger(0) // initial control variable
	return 3
}

// makePairsIter creates the iterator function for pairs.
func makePairsIter(tbl tableapi.TableInterface) vm.GoFunc {
	return func(stack []types.TValue, base int) int {
		nArgs := len(stack) - base - 1
		var key types.TValue
		if nArgs >= 2 && stack[base+2] != nil {
			key = stack[base+2]
		} else {
			key = types.NewTValueNil()
		}

		nextKey, nextVal, ok := tbl.Next(key)
		if !ok {
			stack[base] = types.NewTValueNil()
			return 1
		}
		stack[base] = nextKey
		stack[base+1] = nextVal
		return 2
	}
}

// bpairs implements Lua's pairs(t) function.
// Returns next, table, nil
func bpairs(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 1 {
		luaErrorString("bad argument #1 to 'pairs' (table expected, got no value)")
		return 0
	}

	tbl := extractTable(stack[base+1])
	if tbl == nil {
		luaErrorString("bad argument #1 to 'pairs' (table expected)")
		return 0
	}

	// Return: next function, table, nil
	iterFn := makePairsIter(tbl)
	stack[base] = &goFuncWrapper{fn: iterFn}
	stack[base+1] = stack[base+1] // table
	stack[base+2] = types.NewTValueNil() // initial key
	return 3
}

// brawget implements Lua's rawget(table, index) function.
func brawget(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 2 {
		luaErrorString("bad argument #1 to 'rawget' (table expected)")
		return 0
	}

	tbl := extractTable(stack[base+1])
	if tbl == nil {
		luaErrorString("bad argument #1 to 'rawget' (table expected)")
		return 0
	}

	key := stack[base+2]
	result := tbl.Get(key)
	if result == nil {
		result = types.NewTValueNil()
	}
	stack[base] = result
	return 1
}

// brawset implements Lua's rawset(table, index, value) function.
func brawset(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 3 {
		luaErrorString("bad argument #1 to 'rawset' (table expected)")
		return 0
	}

	tbl := extractTable(stack[base+1])
	if tbl == nil {
		luaErrorString("bad argument #1 to 'rawset' (table expected)")
		return 0
	}

	key := stack[base+2]
	value := stack[base+3]
	tbl.Set(key, value)
	// rawset returns the table
	stack[base] = stack[base+1]
	return 1
}

// brawlen implements Lua's rawlen(v) function.
func brawlen(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 1 {
		luaErrorString("bad argument #1 to 'rawlen' (table or string expected)")
		return 0
	}

	v := stack[base+1]
	if v.IsString() {
		if s, ok := v.GetValue().(string); ok {
			stack[base] = types.NewTValueInteger(types.LuaInteger(len(s)))
			return 1
		}
	}

	tbl := extractTable(v)
	if tbl != nil {
		n := tbl.Len()
		if n == 0 {
			n = int(tableSequenceLen(tbl))
		}
		stack[base] = types.NewTValueInteger(types.LuaInteger(n))
		return 1
	}

	luaErrorString("bad argument #1 to 'rawlen' (table or string expected)")
	return 0
}

// brawequal implements Lua's rawequal(v1, v2) function.
func brawequal(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 2 {
		luaErrorString("bad argument #1 to 'rawequal' (value expected)")
		return 0
	}

	v1 := stack[base+1]
	v2 := stack[base+2]

	equal := rawEqual(v1, v2)
	stack[base] = types.NewTValueBoolean(equal)
	return 1
}

// rawEqual performs raw equality comparison (no metamethods).
func rawEqual(v1, v2 types.TValue) bool {
	if v1 == nil && v2 == nil {
		return true
	}
	if v1 == nil || v2 == nil {
		return false
	}
	// Different base types are never equal
	if v1.GetTag() != v2.GetTag() {
		// Special case: both nil variants
		if v1.IsNil() && v2.IsNil() {
			return true
		}
		return false
	}
	// Same type
	if v1.IsNil() {
		return true
	}
	if v1.IsBoolean() {
		return v1.IsTrue() == v2.IsTrue()
	}
	if v1.IsInteger() {
		return v1.GetInteger() == v2.GetInteger()
	}
	if v1.IsFloat() {
		return v1.GetFloat() == v2.GetFloat()
	}
	if v1.IsString() {
		s1, ok1 := v1.GetValue().(string)
		s2, ok2 := v2.GetValue().(string)
		return ok1 && ok2 && s1 == s2
	}
	// For tables, functions, etc. — compare by identity (pointer)
	return v1.GetValue() == v2.GetValue()
}

// bsetmetatable implements Lua's setmetatable(table, metatable) function.
func bsetmetatable(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 2 {
		luaErrorString("bad argument #1 to 'setmetatable' (table expected)")
		return 0
	}

	tbl := extractTable(stack[base+1])
	if tbl == nil {
		luaErrorString("bad argument #1 to 'setmetatable' (table expected)")
		return 0
	}

	mt := stack[base+2]
	if mt == nil || mt.IsNil() {
		tbl.SetMetatable(nil)
	} else {
		mtTbl := extractTable(mt)
		if mtTbl == nil {
			luaErrorString("bad argument #2 to 'setmetatable' (nil or table expected)")
			return 0
		}
		// SetMetatable expects a types.Table interface.
		// Our TableImpl wraps a *Table which implements types.Table.
		// We need to get the underlying *Table.
		tbl.SetMetatable(getInternalTable(mtTbl))
	}

	// Return the original table
	stack[base] = stack[base+1]
	return 1
}

// getInternalTable extracts the underlying types.Table from a TableInterface.
// Uses duck-type interface matching TableImpl.RawTable().
func getInternalTable(tbl tableapi.TableInterface) types.Table {
	// Try direct type assertion to types.Table
	if t, ok := interface{}(tbl).(types.Table); ok {
		return t
	}
	// Use RawTable() duck-type (matches table/internal.TableImpl)
	type rawTableProvider interface {
		RawTable() types.Table
	}
	if p, ok := interface{}(tbl).(rawTableProvider); ok {
		return p.RawTable()
	}
	return nil
}

// bgetmetatable implements Lua's getmetatable(object) function.
func bgetmetatable(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 1 {
		luaErrorString("bad argument #1 to 'getmetatable' (value expected)")
		return 0
	}

	tbl := extractTable(stack[base+1])
	if tbl == nil {
		stack[base] = types.NewTValueNil()
		return 1
	}

	mt := tbl.GetMetatable()
	if mt == nil {
		stack[base] = types.NewTValueNil()
		return 1
	}

	// Wrap the raw types.Table back into a TableInterface via table.WrapRawTable
	wrapped := table.WrapRawTable(mt)
	if wrapped != nil {
		stack[base] = &tableWrapper{tbl: wrapped}
	} else {
		stack[base] = types.NewTValueNil()
	}
	return 1
}

// bunpack implements Lua's table.unpack(list [, i [, j]]) function.
func bunpack(stack []types.TValue, base int) int {
	nArgs := len(stack) - base - 1
	if nArgs < 1 {
		luaErrorString("bad argument #1 to 'unpack' (table expected)")
		return 0
	}

	tbl := extractTable(stack[base+1])
	if tbl == nil {
		luaErrorString("bad argument #1 to 'unpack' (table expected)")
		return 0
	}

	i := types.LuaInteger(1)
	// Compute j: use Len() first, but if 0, probe with GetInt to find actual length
	jVal := types.LuaInteger(tbl.Len())
	if jVal == 0 {
		// Len() may return 0 for hash-only tables; probe for actual length
		jVal = tableSequenceLen(tbl)
	}
	j := jVal

	if nArgs >= 2 && stack[base+2] != nil && !stack[base+2].IsNil() {
		if stack[base+2].IsInteger() {
			i = stack[base+2].GetInteger()
		} else if stack[base+2].IsFloat() {
			i = types.LuaInteger(stack[base+2].GetFloat())
		}
	}
	if nArgs >= 3 && stack[base+3] != nil && !stack[base+3].IsNil() {
		if stack[base+3].IsInteger() {
			j = stack[base+3].GetInteger()
		} else if stack[base+3].IsFloat() {
			j = types.LuaInteger(stack[base+3].GetFloat())
		}
	}

	if j < i {
		return 0
	}

	n := int(j - i + 1)
	for k := 0; k < n; k++ {
		val := tbl.GetInt(i + types.LuaInteger(k))
		if val == nil {
			val = types.NewTValueNil()
		}
		stack[base+k] = val
	}
	return n
}

// tableSequenceLen probes a table with GetInt to find the sequence length.
// This handles tables where Len() returns 0 because all integer keys are in hash.
func tableSequenceLen(tbl tableapi.TableInterface) types.LuaInteger {
	var n types.LuaInteger
	for {
		n++
		val := tbl.GetInt(n)
		if val == nil || val.IsNil() {
			return n - 1
		}
	}
}

// bwarn implements Lua's warn() function (stub — prints to stderr).
func bwarn(stack []types.TValue, base int) int {
	// Stub: just ignore warnings for now
	return 0
}

// =============================================================================
// Suppress unused import
// =============================================================================
var _ = strconv.Itoa // keep strconv import

// makeRequire creates a require GoFunc that captures the registry.
// This allows require to look up modules in package.loaded.
func makeRequire(registry tableapi.TableInterface, globalEnv tableapi.TableInterface) vm.GoFunc {
	return func(stack []types.TValue, base int) int {
		if base+1 >= len(stack) {
			stack[base] = types.NewTValueNil()
			return 1
		}

		modNameVal := stack[base+1]
		if !modNameVal.IsString() {
			stack[base] = types.NewTValueNil()
			return 1
		}

		modName := modNameVal.GetValue().(string)

		// Look up the module in the global environment (registry)
		key := types.NewTValueString(modName)
		var result types.TValue
		result = registry.Get(key)
		if result != nil && !result.IsNil() {
			// Unwrap tableWrapper or goFuncWrapper if present
			if tw, ok := result.GetValue().(*tableWrapper); ok {
				stack[base] = &tableWrapper{tbl: tw.tbl}
				return 1
			}
			// Skip goFuncWrapper - it's a loader, not the module
			if _, isFunc := result.GetValue().(vm.GoFunc); !isFunc {
				stack[base] = result
				return 1
			}
		}

		// Module not found in registry - try looking up as global variable
		// This handles cases where modules like "string", "math" are globals
		if globalEnv != nil {
			if result = globalEnv.Get(key); result != nil && !result.IsNil() {
				// Unwrap tableWrapper or goFuncWrapper if present
				if tw, ok := result.GetValue().(*tableWrapper); ok {
					stack[base] = &tableWrapper{tbl: tw.tbl}
					return 1
				}
				stack[base] = result
				return 1
			}
		}
		
		// Module not found - return nil
		stack[base] = types.NewTValueNil()
		return 1
	}
}

// gcMode tracks the current GC mode for collectgarbage mode switching.
var gcMode = "incremental" // Lua default

// bcollectgarbage implements Lua's collectgarbage([opt [, arg]]) function.
// Stub implementation — Go manages its own GC, so most operations are no-ops.
func bcollectgarbage(stack []types.TValue, base int) int {
	opt := "collect"
	if base+1 < len(stack) && stack[base+1].IsString() {
		opt = stack[base+1].GetValue().(string)
	}
	switch opt {
	case "collect":
		stack[base] = types.NewTValueInteger(0)
		return 1
	case "count":
		// Return approximate memory in KB
		stack[base] = types.NewTValueFloat(100.0) // fake ~100KB
		if base+1 < len(stack) {
			stack[base+1] = types.NewTValueFloat(0)
		}
		return 2
	case "isrunning":
		stack[base] = types.NewTValueBoolean(true)
		return 1
	case "incremental":
		prev := gcMode
		gcMode = "incremental"
		stack[base] = types.NewTValueString(prev)
		return 1
	case "generational":
		prev := gcMode
		gcMode = "generational"
		stack[base] = types.NewTValueString(prev)
		return 1
	case "stop", "restart":
		stack[base] = types.NewTValueBoolean(true)
		return 1
	case "step":
		stack[base] = types.NewTValueBoolean(false)
		return 1
	case "param":
		stack[base] = types.NewTValueInteger(0)
		return 1
	default:
		stack[base] = types.NewTValueInteger(0)
		return 1
	}
}

// bload implements Lua's load(chunk [, chunkname [, mode [, env]]]) function.
// Compiles a string or function into a callable Lua closure.
func bload(stack []types.TValue, base int) int {
	if base+1 >= len(stack) {
		stack[base] = types.NewTValueNil()
		if base+1 < len(stack) {
			stack[base+1] = types.NewTValueString("bad argument #1 to 'load'")
		}
		return 2
	}

	chunkVal := stack[base+1]
	if !chunkVal.IsString() {
		// TODO: support function chunks
		stack[base] = types.NewTValueNil()
		if base+1 < len(stack) {
			stack[base+1] = types.NewTValueString("attempt to load a non-string chunk")
		}
		return 2
	}

	chunkStr := chunkVal.GetValue().(string)
	chunkName := "=(load)"
	if base+2 < len(stack) && stack[base+2].IsString() {
		chunkName = stack[base+2].GetValue().(string)
	}
	_ = chunkName

	// Parse the chunk
	parser := parse.NewParser()
	chunk, err := parser.Parse(chunkStr)
	if err != nil {
		stack[base] = types.NewTValueNil()
		stack[base+1] = types.NewTValueString(err.Error())
		return 2
	}

	// Compile the chunk
	compiler := bc.NewCompiler("load")
	proto, err := compiler.Compile(chunk)
	if err != nil {
		stack[base] = types.NewTValueNil()
		stack[base+1] = types.NewTValueString(err.Error())
		return 2
	}

	// Create a closure wrapper that can be called
	closure := &loadedClosure{proto: proto}
	stack[base] = closure
	return 1
}

// loadedClosure wraps a compiled prototype so it can be called as a Lua function.
// Implements types.TValue.
type loadedClosure struct {
	proto bcapi.Prototype
}

func (c *loadedClosure) IsNil() bool             { return false }
func (c *loadedClosure) IsBoolean() bool          { return false }
func (c *loadedClosure) IsNumber() bool           { return false }
func (c *loadedClosure) IsInteger() bool          { return false }
func (c *loadedClosure) IsFloat() bool            { return false }
func (c *loadedClosure) IsString() bool           { return false }
func (c *loadedClosure) IsTable() bool            { return false }
func (c *loadedClosure) IsFunction() bool         { return true }
func (c *loadedClosure) IsThread() bool            { return false }
func (c *loadedClosure) IsUserData() bool         { return false }
func (c *loadedClosure) IsLightUserData() bool    { return false }
func (c *loadedClosure) IsCollectable() bool     { return true }
func (c *loadedClosure) IsTrue() bool             { return true }
func (c *loadedClosure) IsFalse() bool            { return false }
func (c *loadedClosure) IsLClosure() bool        { return true }
func (c *loadedClosure) IsCClosure() bool         { return false }
func (c *loadedClosure) IsLightCFunction() bool   { return false }
func (c *loadedClosure) IsClosure() bool          { return true }
func (c *loadedClosure) IsProto() bool            { return false }
func (c *loadedClosure) IsUpval() bool            { return false }
func (c *loadedClosure) IsShortString() bool      { return false }
func (c *loadedClosure) IsLongString() bool       { return false }
func (c *loadedClosure) IsEmpty() bool            { return false }
func (c *loadedClosure) GetTag() int              { return types.Ctb(int(types.LUA_VLCL)) }
func (c *loadedClosure) GetBaseType() int         { return int(types.LUA_TFUNCTION) }
func (c *loadedClosure) GetValue() interface{}   { return c }
func (c *loadedClosure) GetGC() *types.GCObject  { return nil }
func (c *loadedClosure) GetInteger() types.LuaInteger { return 0 }
func (c *loadedClosure) GetFloat() types.LuaNumber   { return 0 }
func (c *loadedClosure) GetPointer() unsafe.Pointer { return nil }
func (c *loadedClosure) GetProto() bcapi.Prototype { return c.proto }

// openBaseLib registers base library functions in the global environment.
func (L *LuaState) openBaseLib() {
	// Register base functions
	L.setGlobal("print", bprint)
	L.setGlobal("type", btype)
	L.setGlobal("assert", bassert)
	L.setGlobal("tostring", btostring)
	L.setGlobal("tonumber", btonumber)

	// Phase 2 base functions
	L.setGlobal("error", berror)
	L.setGlobal("pcall", bpcall)
	L.setGlobal("xpcall", bxpcall)
	L.setGlobal("select", bselect)
	L.setGlobal("next", bnext)
	L.setGlobal("ipairs", bipairs)
	L.setGlobal("pairs", bpairs)
	L.setGlobal("rawget", brawget)
	L.setGlobal("rawset", brawset)
	L.setGlobal("rawlen", brawlen)
	L.setGlobal("rawequal", brawequal)
	L.setGlobal("setmetatable", bsetmetatable)
	L.setGlobal("getmetatable", bgetmetatable)
	L.setGlobal("unpack", bunpack)
	L.setGlobal("warn", bwarn)
	L.setGlobal("collectgarbage", bcollectgarbage)
	L.setGlobal("load", bload)

	// Register _VERSION
	L.setGlobalValue("_VERSION", types.NewTValueString("Lua 5.4"))

	// Create package table with loaded and preload sub-tables
	// Use createModuleTable() to get fresh table instances
	packageTbl := createModuleTable()
	loadedTbl := createModuleTable()
	preloadTbl := createModuleTable()

	// Pre-populate package.loaded with stub module tables
	moduleNames := []string{"debug", "string", "math", "table", "io", "os", "coroutine", "utf8"}
	var tableMod tableapi.TableInterface
	var stringMod tableapi.TableInterface
	var mathMod tableapi.TableInterface
	for _, name := range moduleNames {
		modTbl := createModuleTable()
		key := types.NewTValueString(name)
		loadedTbl.Set(key, &tableWrapper{tbl: modTbl})
		// Also make each module accessible as a global
		L.setGlobalValue(name, &tableWrapper{tbl: modTbl})
		if name == "table" {
			tableMod = modTbl
		}
		if name == "string" {
			stringMod = modTbl
		}
		if name == "math" {
			mathMod = modTbl
		}
	}

	// Register string library functions
	if stringMod != nil {
		registerStringLib(stringMod)
	}

	// Register math library functions
	if mathMod != nil {
		registerMathLib(mathMod)
	}

	// Register table library functions
	if tableMod != nil {
		registerTableLib(tableMod)
		// Also register table.unpack (from Phase 2)
		unpackKey := types.NewTValueString("unpack")
		tableMod.Set(unpackKey, &goFuncWrapper{fn: bunpack})
	}

	// Set package.loaded and package.preload in package table
	loadedKey := types.NewTValueString("loaded")
	preloadKey := types.NewTValueString("preload")
	packageTbl.Set(loadedKey, &tableWrapper{tbl: loadedTbl})
	packageTbl.Set(preloadKey, &tableWrapper{tbl: preloadTbl})

	// Set package.config, package.path, package.cpath
	configKey := types.NewTValueString("config")
	packageTbl.Set(configKey, types.NewTValueString("/\n;\n?\n!\n-\n"))
	pathKey := types.NewTValueString("path")
	packageTbl.Set(pathKey, types.NewTValueString("./?.lua"))
	cpathKey := types.NewTValueString("cpath")
	packageTbl.Set(cpathKey, types.NewTValueString(""))

	// Set package itself in package.loaded
	selfKey := types.NewTValueString("package")
	loadedTbl.Set(selfKey, &tableWrapper{tbl: packageTbl})

	// Register package as a global
	L.setGlobalValue("package", &tableWrapper{tbl: packageTbl})

	// Register require function (closure captures registry)
	L.setGlobal("require", makeRequire(L.global.Registry(), L.global.Registry()))

	// Register _G as a reference to the global environment table
	L.setGlobalValue("_G", &tableWrapper{tbl: L.global.Registry()})
}
