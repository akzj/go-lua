// Package internal provides the concrete implementation of state/api interfaces.
package internal

import (
	"unsafe"

	memapi "github.com/akzj/go-lua/mem/api"
	"github.com/akzj/go-lua/state/api"
	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
)

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
}

// NewLuaState creates a new Lua state.
// If alloc is nil, uses the default allocator.
func NewLuaState(alloc memapi.Allocator) *LuaState {
	// Initialize allocator
	if alloc == nil {
		alloc = memapi.DefaultAllocator
	}

	// Create global state
	g := &globalState{
		alloc:     alloc,
		registry:  createRegistry(alloc),
		mainThread: nil, // Will be set after LuaState creation
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

func (L *LuaState) Call(nArgs, nResults int) {
	panic("TODO: implement Call")
}

func (L *LuaState) Resume() error {
	panic("TODO: implement Resume")
}

func (L *LuaState) Yield(nResults int) error {
	panic("TODO: implement Yield")
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

// EnsurePointerSize ensures the TValue slice has room for n elements.
func EnsurePointerSize(s []types.TValue, n int) []types.TValue {
	if cap(s) >= n {
		return s
	}
	newSize := n * 2
	if newSize < 32 {
		newSize = 32
	}
	return make([]types.TValue, len(s), newSize)
}

// Memory allocation helpers (placeholder - actual impl in mem module)
func realloc(alloc memapi.Allocator, old unsafe.Pointer, oldSize, newSize uint) unsafe.Pointer {
	return alloc.SafeRealloc(old, memapi.LuaMem(oldSize), memapi.LuaMem(newSize))
}
