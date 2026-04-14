// Package api defines the Lua state, global state, and call frame types.
//
// LuaState is the per-thread (coroutine) state. GlobalState is shared across
// all threads in a Lua instance. CallInfo represents a single activation record
// in the call chain.
//
// Reference: .analysis/04-call-return-error.md §1, .analysis/07-runtime-infrastructure.md §2
package api

import (
	objectapi "github.com/akzj/go-lua/internal/object/api"
)

// ---------------------------------------------------------------------------
// CFunction is the CANONICAL type for Go functions callable from Lua.
// It receives the Lua state and returns the number of results pushed.
// All other modules should reference this type (or alias it).
// ---------------------------------------------------------------------------
type CFunction func(L *LuaState) int

// KFunction is a continuation function for yieldable C calls.
type KFunction func(L *LuaState, status int, ctx int) int

// ---------------------------------------------------------------------------
// Thread status codes (matches C TStatus)
// ---------------------------------------------------------------------------
const (
	StatusOK        = 0 // LUA_OK
	StatusYield     = 1 // LUA_YIELD
	StatusErrRun    = 2 // LUA_ERRRUN
	StatusErrSyntax = 3 // LUA_ERRSYNTAX
	StatusErrMem    = 4 // LUA_ERRMEM
	StatusErrErr    = 5 // LUA_ERRERR

	// StatusCloseKTop: special status for closing TBC variables without
	// resetting L.Top. Used by OP_RETURN k-bit path where return values
	// sit on the stack above the TBC variables.
	// Matches C Lua's CLOSEKTOP = LUA_ERRERR + 1 in lfunc.h.
	StatusCloseKTop = 6
)

// ---------------------------------------------------------------------------
// CallInfo status flags (packed into CallStatus uint32)
// Matches C exactly — verified against lstate.h
// ---------------------------------------------------------------------------
const (
	CISTNResults  uint32 = 0xFF       // bits 0-7: nresults + 1
	CISTC         uint32 = 1 << 15    // call is running a C function
	CISTFresh     uint32 = 1 << 16    // fresh luaV_execute frame
	CISTClsRet    uint32 = 1 << 17    // closing tbc variables on return
	CISTTBC       uint32 = 1 << 18    // has to-be-closed variables
	CISTOAH       uint32 = 1 << 19    // original allowhook value
	CISTHooked    uint32 = 1 << 20    // running a debug hook
	CISTYPCall    uint32 = 1 << 21    // yieldable protected call
	CISTTail      uint32 = 1 << 22    // call was tail called
	CISTHookYield uint32 = 1 << 23    // last hook call yielded
	CISTFin       uint32 = 1 << 24    // function called a finalizer

	CISTCCMTShift  = 8  // bits 8-11: __call metamethod count
	CISTCCMTMask   = 0xF << CISTCCMTShift
	CISTRecstShift = 12 // bits 12-14: recovery status
)

// MaxCCalls is the maximum C call depth before stack overflow.
const MaxCCalls = 200

// MaxStack is the maximum Lua stack size.
const MaxStack = 1_000_000

// BasicStackSize is the initial stack allocation.
const BasicStackSize = 40 // 2 * LUA_MINSTACK

// ExtraStack is reserved space beyond stack_last for error handling.
const ExtraStack = 5

// MaxResults is the maximum number of results a function can return.
const MaxResults = 250

// MultiRet signals "all results" in call/return (LUA_MULTRET = -1).
const MultiRet = -1

// ---------------------------------------------------------------------------
// CallInfo is the activation record for a single function call.
// Forms a doubly-linked list. The Lua/C distinction is handled by
// checking CallStatus & CISTC.
// ---------------------------------------------------------------------------
type CallInfo struct {
	Func int       // stack index of function slot
	Top  int       // stack index of top for this call
	Prev *CallInfo // caller's CallInfo
	Next *CallInfo // callee's CallInfo (allocated lazily)

	// Lua function fields (valid when CallStatus & CISTC == 0)
	SavedPC    int  // index into Proto.Code (next instruction)
	Trap       bool // tracing active (hooks)
	NExtraArgs int  // extra vararg arguments

	// C function fields (valid when CallStatus & CISTC != 0)
	K          KFunction // continuation for yields
	OldErrFunc int       // saved error function stack index
	Ctx        int       // continuation context

	// Ephemeral union (reused for different purposes)
	NYield int // number of values yielded
	NRes   int // number of values returned

	CallStatus uint32 // CIST_* flags
}

// IsLua returns true if this call frame is running a Lua function.
func (ci *CallInfo) IsLua() bool {
	return ci.CallStatus&CISTC == 0
}

// IsLuaCode returns true if running Lua bytecode (not C, not hook).
func (ci *CallInfo) IsLuaCode() bool {
	return ci.CallStatus&(CISTC|CISTHooked) == 0
}

// NResults returns the number of results the caller expects.
// Returns MultiRet (-1) if caller wants all results.
func (ci *CallInfo) NResults() int {
	return int(ci.CallStatus&CISTNResults) - 1
}

// SetNResults encodes the expected result count into CallStatus.
func (ci *CallInfo) SetNResults(n int) {
	ci.CallStatus = (ci.CallStatus &^ CISTNResults) | uint32(n+1)&CISTNResults
}

// ---------------------------------------------------------------------------
// LuaState is the per-thread (coroutine) state.
// Each coroutine has its own LuaState with its own stack.
// ---------------------------------------------------------------------------
type LuaState struct {
	// C3 FIX: Stack uses []StackValue (not []TValue) for TBC support.
	// Each slot has a TBCDelta field for the to-be-closed linked list.
	Stack   []objectapi.StackValue // the value stack
	Top     int                    // index of first free slot
	CI      *CallInfo              // current call info
	BaseCI  CallInfo               // embedded base CallInfo (C host level)

	Global    *GlobalState // shared global state
	OpenUpval any          // head of open upvalue list (typed in closure module)
	TBCList   int          // stack index of top to-be-closed variable (-1 = none)

	Status    int    // thread status (StatusOK, StatusYield, etc.)
	AllowHook bool   // hook enable flag
	NCCalls   uint32 // C call depth (low 16) + non-yieldable count (high 16)
	NCI       int    // number of CallInfo nodes in the list

	ErrFunc int // error handler stack index
	OldPC   int // last traced PC

	// C10 FIX: Debug hook fields (needed by debug.sethook/gethook)
	Hook          any  // debug hook function (typed later)
	BaseHookCount int  // base hook count setting
	HookCount     int  // current hook countdown
	HookMask      int  // which hooks active (call, return, line, count)
	FTransfer     int  // hook value transfer: index of first value
	NTransfer     int  // hook value transfer: number of values
}

// Yieldable returns true if the current coroutine can yield.
func (L *LuaState) Yieldable() bool {
	return (L.NCCalls & 0xFFFF0000) == 0
}

// CCalls returns the current C call depth.
func (L *LuaState) CCalls() int {
	return int(L.NCCalls & 0xFFFF)
}

// StackLast returns the index of the last usable stack slot.
func (L *LuaState) StackLast() int {
	return len(L.Stack) - ExtraStack
}

// ---------------------------------------------------------------------------
// GlobalState is shared across all threads in a Lua instance.
// ---------------------------------------------------------------------------
type GlobalState struct {
	Registry objectapi.TValue // the registry table
	Seed     uint32           // randomized hash seed

	// I1 FIX: Concrete types where no circular dependency exists
	TMNames [25]*objectapi.LuaString // metamethod name strings
	MT      [9]any                   // metatables for basic types (9 = NumTypes, any to avoid Table import cycle)

	MainThread *LuaState           // the main thread
	Panic      CFunction           // unprotected error handler
	MemErrMsg  *objectapi.LuaString // pre-allocated "not enough memory" string

	// String interning table (typed in luastring module)
	StringTable any

	// API string cache (typed in luastring module)
	StringCache any

	// GCTotalBytes tracks total Lua-level object allocations (bytes).
	// Mirrors C Lua's gettotalbytes(g) for collectgarbage("count").
	// Monotonically increasing — Go's GC handles actual freeing.
	GCTotalBytes int64
}

// ---------------------------------------------------------------------------
// LuaError is the CANONICAL error type used for Lua runtime errors.
// Lua errors are propagated via panic(LuaError{...}).
// All modules should use this type (not define their own).
// ---------------------------------------------------------------------------
type LuaError struct {
	Status  int              // error code (StatusErrRun, etc.)
	Message objectapi.TValue // error object (usually a string)
}

func (e LuaError) Error() string {
	return "lua error" // detailed formatting done by the API layer
}

// ---------------------------------------------------------------------------
// LuaYield is the type used for coroutine yield via panic/recover.
// Distinct from LuaError so recover() can distinguish error from yield.
// I5 FIX: Added for coroutine support.
// ---------------------------------------------------------------------------
type LuaYield struct {
	NResults int // number of results to yield
}

// ---------------------------------------------------------------------------
// Hook mask constants (matches C LUA_MASK*)
// ---------------------------------------------------------------------------
const (
	MaskCall  = 1 << 0 // LUA_MASKCALL
	MaskRet   = 1 << 1 // LUA_MASKRET
	MaskLine  = 1 << 2 // LUA_MASKLINE
	MaskCount = 1 << 3 // LUA_MASKCOUNT
)

// ---------------------------------------------------------------------------
// Registry indices (matches C LUA_RIDX_*)
// ---------------------------------------------------------------------------
const (
	RegistryIndexGlobals    = 2 // LUA_RIDX_GLOBALS
	RegistryIndexMainThread = 3 // LUA_RIDX_MAINTHREAD
	RegistryIndexLast       = 3 // LUA_RIDX_LAST
)
