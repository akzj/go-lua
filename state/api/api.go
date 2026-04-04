// Package api defines Lua state management interfaces.
// NO dependencies on internal/ packages - pure interface definitions.
//
// Reference: lua-master/lstate.h, lua-master/ldo.c
//
// Design constraints:
// - LuaStateInterface is the only public interface
// - All implementation details live in state/internal/
// - Dependencies: types (TValue, etc), vm (VMExecutor), table (Table), mem (Allocator)
package api

import (
	gc "github.com/akzj/go-lua/gc"
	memapi "github.com/akzj/go-lua/mem/api"
	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
)

// LuaStateInterface manages a Lua execution state (thread/coroutine).

// =============================================================================
// Lua Status Constants
// =============================================================================

type Status = types.TStatus

const (
	LUA_OK          Status = 0
	LUA_YIELD       Status = 1
	LUA_ERRERR      Status = 2
	LUA_ERRRUN      Status = 3
	LUA_ERRSYNTAX   Status = 4
	LUA_ERRMEM      Status = 5
	LUA_ERRGCMM     Status = 6
	LUA_ERRFILE     Status = 7
	LUA_ERRRUNNING  Status = 8
	LuaErrorStatus  Status = 9 // any other error not listed above
)

// RegistryIndex is the pseudo-index that refers to the registry table.
// Used with lua_getfield/lua_setfield to access the registry.
const LUA_REGISTRYINDEX = -10000

// =============================================================================
// CallInfo (frame state)
// =============================================================================

// CallInfo tracks a single function call frame.
//
// Invariants:
// - Func <= Top <= StackTop
// - Previous/Next form a doubly-linked list of frames
//
// Why store both func and previous frame?
// - func points to the function being executed
// - previous links to the caller's frame for return
type CallInfo interface {
	// Func returns the stack index of the function for this frame.
	Func() int

	// Top returns one past the top of the stack for this frame.
	// The frame can use registers [Func, Top).
	Top() int

	// Prev returns the previous CallInfo in the call stack.
	Prev() CallInfo

	// SetFunc sets the function index for this frame.
	SetFunc(idx int)

	// SetTop sets the top of the stack for this frame.
	SetTop(idx int)

	// NResults returns the expected number of return values.
	NResults() int

	// SetNResults sets the expected number of return values.
	SetNResults(n int)

	// SetPrev sets the previous CallInfo in the call stack (internal use).
	SetPrev(prev CallInfo)
}

// =============================================================================
// Global State
// =============================================================================

// GlobalState holds state shared by all threads in the Lua VM.
//
// Invariants:
// - Registry is always present
// - All collectable objects are tracked for GC
//
// Why not put Registry in LuaState?
// - Registry is shared between threads, not per-thread
type GlobalState interface {
	// Allocator returns the memory allocator.
	Allocator() memapi.Allocator

	// Registry returns the global registry table.
	Registry() tableapi.TableInterface

	// CurrentThread returns the main thread.
	// Returns LuaStateInterface to avoid circular dependency with api.LuaState.
	CurrentThread() LuaStateInterface

	// GC returns the garbage collector interface.
	// Provides access to GC control operations.
	GC() gc.GCCollector
}

// =============================================================================
// Lua State Interface
// =============================================================================

// LuaStateInterface manages a Lua execution state (thread/coroutine).
//
// Invariants:
// - Stack grows upward: base <= top <= max
// - CallInfo stack is a linked list: ci->prev->prev->...->base_ci
// - The main thread's ci always points to base_ci
//
// Why a separate NewThread vs constructor?
// - lua_newstate creates the first thread (with global state)
// - lua_newthread creates additional threads (share global state)
// - This mirrors Lua C API semantics
type LuaStateInterface interface {
	// =====================================================================
	// Thread Management
	// =====================================================================

	// NewThread creates a new thread (coroutine) that shares this thread's global state.
	// Returns a new LuaState that can be resumed independently.
	//
	// Why not a separate Thread type?
	// - In Lua, threads ARE states (lua_State*)
	// - The Go type system represents this cleanly
	NewThread() LuaStateInterface

	// Status returns the current status of this thread.
	// See LUA_OK, LUA_YIELD, LUA_ERR*, etc.
	Status() Status

	// =====================================================================
	// Stack Operations
	// =====================================================================

	// PushValue pushes a copy of the value at index onto the top of the stack.
	//
	// Why positive index for global?
	// - Positive indices are relative to the bottom of the stack
	// - Negative indices are relative to the top (cleaner for locals)
	PushValue(idx int)

	// Pop removes (and discards) the value at the top of the stack.
	Pop()

	// Top returns the index of the top element in the stack.
	// Stack is 1-based: top() == 0 means empty stack.
	Top() int

	// SetTop sets the top of the stack to the given index.
	// Can be used to pop or push values.
	SetTop(idx int)

	// =====================================================================
	// Function Calls
	// =====================================================================

	// Call calls a function.
	// nArgs: number of arguments on the stack (function + args)
	// nResults: number of expected results (-1 for variable)
	//
	// Preconditions:
	// - Stack has function at top-nArgs
	// - Function is callable (closure, C function, or table with __call)
	//
	// Postconditions:
	// - Function and arguments are removed from stack
	// - Results are pushed onto the stack
	Call(nArgs, nResults int)

	// Resume resumes a thread that was yielded.
	// Returns error if thread cannot be resumed.
	//
	// Why Resume instead of just Call?
	// - Call is for normal function invocation
	// - Resume is for resuming a coroutine from yield
	// - Lua coroutines are symmetric: resume from yield point
	Resume() error

	// Yield suspends the current thread.
	// Returns control to the caller who invoked Resume.
	//
	// Why is this needed?
	// - Coroutines require yield to suspend execution
	// - lua_yield takes (nResults) args to return to resume caller
	Yield(nResults int) error

	// =====================================================================
	// Global State Access
	// =====================================================================

	// Global returns the global state shared by all threads.
	Global() GlobalState

	// =====================================================================
	// Internal (for VM integration)
	// =====================================================================

	// Stack returns the internal stack slice for direct access.
	// Why expose this? The VM needs direct access for efficiency.
	// Callers should not modify the stack directly.
	Stack() []types.TValue

	// StackSize returns the current size of the stack buffer.
	StackSize() int

	// GrowStack ensures at least n free slots are available.
	GrowStack(n int)

	// CurrentCI returns the current call frame.
	CurrentCI() CallInfo

	// PushCI pushes a new call frame.
	PushCI(ci CallInfo)

	// PopCI pops the current call frame.
	PopCI()
}

// =============================================================================
// Factory Function
// =============================================================================

// DefaultLuaState is the default LuaState instance.
// Initialized by internal.init() before any user code runs.
var DefaultLuaState LuaStateInterface

// New creates a new Lua state with its own global state.
// This is lua_open / lua_newstate equivalent.
func New(alloc memapi.Allocator) LuaStateInterface {
	return DefaultLuaState
}
