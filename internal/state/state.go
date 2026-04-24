// LuaState initialization and stack management.
//
// Reference: lua-master/lstate.c
package state

import (
	"fmt"
	"math/rand"
	"os"

	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/luastring"
	"github.com/akzj/go-lua/internal/table"
)

// ---------------------------------------------------------------------------
// NewState — create a fully initialized Lua state
// Mirrors: lua_newstate + f_luaopen + stack_init + init_registry + luaT_init
// ---------------------------------------------------------------------------

// NewState creates a new Lua state with stack, base CI, registry, globals,
// string table, and TM names. This is the Go equivalent of lua_newstate.
func NewState() *LuaState {
	L := &LuaState{}
	g := &GlobalState{}
	L.Global = g

	// Random hash seed
	g.Seed = rand.Uint32()

	// Initialize V5 GC state (before any objects are created)
	g.CurrentWhite = object.WhiteBit0
	g.GCState = object.GCSpause
	// GC tuning defaults (C Lua 5.5.1: LUAI_GCPAUSE=200, LUAI_GCMUL=200, LUAI_GCSTEPSIZE=13)
	g.GCPause = 250 // C Lua 5.5.1 default (LUAI_GCPAUSE)
	g.GCStepMul = 200
	g.GCStepSize = 13
	// Initial GC debt: give 64KB of allocation credit before first GC triggers.
	// This matches the minDebt in SetPause and avoids premature collection
	// during state initialization.
	g.GCDebt = 64 * 1024

	// String table
	strtab := luastring.NewStringTable(g.Seed)
	strtab.OnCreate = func(obj object.GCObject) {
		g.LinkGC(obj) // V5: register new strings in allgc chain
	}
	g.StringTable = strtab

	// Pre-init thread fields (mirrors preinit_thread)
	L.Status = StatusOK
	L.AllowHook = true
	L.NCCalls = 0
	L.OpenUpval = nil
	L.TBCList = -1
	L.ErrFunc = 0
	L.OldPC = 0
	L.BaseCI.Prev = nil
	L.BaseCI.Next = nil

	// Initialize stack (mirrors stack_init)
	stackInit(L)

	// Set main thread and link into GC chain
	g.MainThread = L
	g.LinkGC(L)

	// Initialize registry (mirrors init_registry)
	initRegistry(L, g)

	// Initialize TM names (mirrors luaT_init)
	initTMNames(g, strtab)

	// Pre-allocate memory error message
	g.MemErrMsg = strtab.Intern("not enough memory")

	// Mark main thread as non-yieldable (mirrors incnny)
	L.NCCalls += 0x00010000 // increment non-yieldable count

	// Install default warning handler (mirrors luaL_newstate → lua_setwarnf(warnfon))
	DefaultWarnHandler(g)

	return L
}

// stackInit allocates the stack and sets up the base CallInfo.
// Mirrors: stack_init in lstate.c
func stackInit(L *LuaState) {
	// Allocate stack with extra space and capacity headroom
	// to avoid reallocation on moderate growth.
	size := BasicStackSize + ExtraStack
	if cap(L.Stack) >= size {
		// Reuse existing stack slice from pool — just reslice and clear
		L.Stack = L.Stack[:size]
	} else {
		// Allocate new stack with capacity headroom
		L.Stack = make([]object.StackValue, size, size+size/2)
	}
	for i := range L.Stack {
		L.Stack[i].Val = object.Nil
	}

	// Reset CI to base
	resetCI(L)

	// Top starts at 1 (slot 0 is the function entry for base CI)
	L.Top = 1
}

// resetCI resets the CallInfo chain to the base CI.
// Mirrors: resetCI in lstate.c
func resetCI(L *LuaState) {
	ci := &L.BaseCI
	L.CI = ci
	ci.Func = 0 // function slot at stack[0]
	L.Stack[0].Val = object.Nil // nil function entry for base CI
	ci.Top = 1 + 20 // func + LUA_MINSTACK (20)
	ci.K = nil
	ci.CallStatus = CISTC // base CI is a "C" frame
	L.Status = StatusOK
	L.ErrFunc = 0
	L.NCI = 0
}

// initRegistry creates the registry table with predefined values.
// Mirrors: init_registry in lstate.c
func initRegistry(L *LuaState, g *GlobalState) {
	// Create registry table pre-sized for LUA_RIDX_LAST entries
	// Matches C: luaH_resize(L, registry, LUA_RIDX_LAST, 0)
	registry := table.New(RegistryIndexLast, 0)
	g.LinkGC(registry) // link to allgc so sweep resets mark bits each cycle
	registry.GCHeader.ObjSize = registry.EstimateBytes()
	g.GCTotalBytes += registry.GCHeader.ObjSize

	// Store as TValue in GlobalState
	g.Registry = object.TValue{
		Tt:  object.TagTable,
		Obj: registry,
	}

	// registry[1] = false (placeholder, matches C)
	registry.SetInt(1, object.False)

	// registry[LUA_RIDX_GLOBALS] = new table (the global table _G)
	globals := table.New(0, 0)
	g.LinkGC(globals) // link to allgc so sweep resets mark bits each cycle
	globals.GCHeader.ObjSize = globals.EstimateBytes()
	g.GCTotalBytes += globals.GCHeader.ObjSize
	registry.SetInt(int64(RegistryIndexGlobals), object.TValue{
		Tt:  object.TagTable,
		Obj: globals,
	})

	// registry[LUA_RIDX_MAINTHREAD] = L (as thread TValue)
	registry.SetInt(int64(RegistryIndexMainThread), object.TValue{
		Tt:  object.TagThread,
		Obj: L,
	})
}

// initTMNames interns all 25 metamethod name strings.
// Mirrors: luaT_init in ltm.c
func initTMNames(g *GlobalState, strtab *luastring.StringTable) {
	// TMNames string array is defined in metamethod/api/api.go
	eventNames := [25]string{
		"__index", "__newindex", "__gc", "__mode", "__len", "__eq",
		"__add", "__sub", "__mul", "__mod", "__pow", "__div", "__idiv",
		"__band", "__bor", "__bxor", "__shl", "__shr",
		"__unm", "__bnot", "__lt", "__le",
		"__concat", "__call", "__close",
	}
	for i := 0; i < 25; i++ {
		g.TMNames[i] = strtab.Intern(eventNames[i])
	}
}

// ---------------------------------------------------------------------------
// Stack management
// ---------------------------------------------------------------------------

// GrowStack ensures there are at least n free stack slots above L.Top.
// If not enough space, reallocates the stack.
// Mirrors: luaD_growstack / luaD_reallocstack in ldo.c
func GrowStack(L *LuaState, n int) {
	needed := L.Top + n + ExtraStack
	if needed <= len(L.Stack) {
		return // already enough space
	}

	// If we're already in error stack space (size > MaxStack), we can't grow further
	if len(L.Stack) > MaxStack+ExtraStack {
		panic(LuaError{Status: StatusErrMem, Message: object.MakeString(
			&object.LuaString{Data: "stack overflow", IsShort: true})})
	}

	// Calculate new size — at least double, but enough for needed
	newSize := 2 * len(L.Stack)
	if newSize < needed {
		newSize = needed
	}

	// Cap at MaxStack (normal growth) or errorStackSize (overflow recovery)
	if L.Top+n > MaxStack {
		// Stack overflow — grow to error stack size to allow error handling
		errorStackSize := MaxStack + 200
		if needed <= errorStackSize+ExtraStack {
			newSize = errorStackSize + ExtraStack
		} else {
			panic(LuaError{Status: StatusErrMem, Message: object.MakeString(
				&object.LuaString{Data: "stack overflow", IsShort: true})})
		}
	} else {
		if newSize > MaxStack+ExtraStack {
			newSize = MaxStack + ExtraStack
		}
	}

	reallocStack(L, newSize)
}

// reallocStack grows the stack to newSize, preserving all values.
// Since upvalues use StackIdx (not pointers), no upvalue fixup is needed.
func reallocStack(L *LuaState, newSize int) {
	oldSize := len(L.Stack)

	// Fast path: if the underlying array has enough capacity, just reslice.
	if newSize <= cap(L.Stack) {
		L.Stack = L.Stack[:newSize]
		// Initialize new slots to nil
		for i := oldSize; i < newSize; i++ {
			L.Stack[i].Val = object.Nil
		}
		return
	}

	// Slow path: allocate new slice with 50% extra capacity headroom
	// to reduce future reallocations.
	newCap := newSize + newSize/2
	newStack := make([]object.StackValue, newSize, newCap)
	copy(newStack, L.Stack)

	// Initialize new slots to nil
	for i := oldSize; i < newSize; i++ {
		newStack[i].Val = object.Nil
	}

	L.Stack = newStack
}

// StackCheck checks that there are at least n free stack slots.
// Returns true if enough space exists, false otherwise.
func StackCheck(L *LuaState, n int) bool {
	return L.Top+n <= L.StackLast()
}

// EnsureStack ensures at least n free stack slots, growing if needed.
// This is the primary function other modules should call.
func EnsureStack(L *LuaState, n int) {
	if !StackCheck(L, n) {
		GrowStack(L, n)
	}
}

// PushValue pushes a TValue onto the stack and increments Top.
// Panics if stack overflow.
func PushValue(L *LuaState, v object.TValue) {
	if L.Top >= len(L.Stack) {
		GrowStack(L, 1)
	}
	L.Stack[L.Top].Val = v
	L.Top++
}

// ---------------------------------------------------------------------------
// CallInfo management
// ---------------------------------------------------------------------------

// ciSlabSize is the number of CallInfos allocated in each slab.
// Reduces GC pressure: one allocation per 32 calls instead of one per call.
const ciSlabSize = 32

// NewCI allocates or reuses the next CallInfo in the chain.
// Mirrors: luaE_extendCI in lstate.c
func NewCI(L *LuaState) *CallInfo {
	ci := L.CI
	if ci.Next != nil {
		// Reuse existing next CI
		L.CI = ci.Next
		return ci.Next
	}

	// Allocate from slab
	if L.CISlab == nil || L.CISlabIdx >= len(L.CISlab) {
		L.CISlab = make([]CallInfo, ciSlabSize)
		L.CISlabIdx = 0
	}
	newCI := &L.CISlab[L.CISlabIdx]
	L.CISlabIdx++
	newCI.Prev = ci
	newCI.Next = nil
	ci.Next = newCI
	L.CI = newCI
	L.NCI++
	return newCI
}

// FreeCI frees all CallInfo nodes after the current one.
// Mirrors: freeCI in lstate.c
func FreeCI(L *LuaState) {
	ci := L.CI
	next := ci.Next
	ci.Next = nil
	for next != nil {
		n := next.Next
		next = n
		L.NCI--
	}
}

// ShrinkCI frees half of the unused CallInfo nodes.
// Mirrors: luaE_shrinkCI in lstate.c
func ShrinkCI(L *LuaState) {
	ci := L.CI.Next // first free CallInfo
	if ci == nil {
		return
	}
	for {
		next := ci.Next
		if next == nil {
			break
		}
		// Remove 'next' from list
		next2 := next.Next
		ci.Next = next2
		L.NCI--
		if next2 == nil {
			break
		}
		next2.Prev = ci
		ci = next2
	}
}

// ---------------------------------------------------------------------------
// NewThread — create a new coroutine thread
// Mirrors: lua_newthread in lstate.c
// ---------------------------------------------------------------------------

// NewThread creates a new Lua thread (coroutine) sharing the same GlobalState.
func NewThread(L *LuaState) *LuaState {
	L1 := getLuaState() // from pool instead of &LuaState{}
	g := L.Global
	L1.Global = g

	// Pre-init (mirrors preinit_thread)
	L1.Status = StatusOK
	L1.AllowHook = true
	L1.NCCalls = 0
	L1.OpenUpval = nil
	L1.TBCList = -1
	L1.ErrFunc = 0
	L1.OldPC = 0
	L1.BaseCI.Prev = nil
	L1.BaseCI.Next = nil

	// Copy hook settings from parent
	L1.HookMask = L.HookMask
	L1.BaseHookCount = L.BaseHookCount
	L1.Hook = L.Hook
	L1.HookCount = L1.BaseHookCount

	// Initialize stack
	stackInit(L1)

	// Link new thread into GC chain
	g.LinkGC(L1)

	return L1
}

// ---------------------------------------------------------------------------
// LinkGC links a new collectable object into the allgc chain and sets its
// initial white mark. This is the Go equivalent of C Lua's luaC_newobj.
// Must be called for every new collectable object immediately after creation.
// ---------------------------------------------------------------------------
func (g *GlobalState) LinkGC(obj object.GCObject) {
	h := obj.GC()
	h.Marked = g.CurrentWhite
	h.Next = g.Allgc
	g.Allgc = obj
	// Track allocation for debt-based GC pacing.
	// When GCDebt reaches ≤0, the VM triggers a GC step.
	if h.ObjSize == 0 {
		h.ObjSize = 64 // default estimate for non-table objects (closures, strings, etc.)
	}
	g.TrackAllocation(h.ObjSize)
}

// TrackAllocation increments GCTotalBytes and decrements GCDebt by n bytes.
// Used for debt-based GC pacing: when GCDebt reaches 0, a GC step triggers.
// Lua is single-threaded so no atomics needed.
//
// If a MemoryLimit is set and GCTotalBytes exceeds it, a full GC is attempted
// (via GCDrainFn). If still over limit after GC, panics with StatusErrMem.
func (g *GlobalState) TrackAllocation(n int64) {
	g.GCTotalBytes += n
	g.GCDebt -= n
	if g.MemoryLimit > 0 && g.GCTotalBytes > g.MemoryLimit {
		g.checkMemoryLimit()
	}
}

// checkMemoryLimit is the slow path for memory limit enforcement.
// Separated from TrackAllocation to keep the fast path inlineable.
//
//go:noinline
func (g *GlobalState) checkMemoryLimit() {
	// Try a full GC to reclaim memory before erroring.
	// Use GCDrainFn (lightweight GC) to avoid re-entrancy issues.
	if g.GCDrainFn != nil && !g.GCRunning && g.MainThread != nil {
		g.GCDrainFn(g.MainThread)
	}
	// After GC, check again
	if g.GCTotalBytes > g.MemoryLimit {
		panic(LuaError{
			Status:  StatusErrMem,
			Message: object.MakeString(g.MemErrMsg),
		})
	}
}

// ---------------------------------------------------------------------------
// CloseState — cleanup a Lua state
// Mirrors: close_state in lstate.c (simplified — Go GC handles memory)
// ---------------------------------------------------------------------------

// CloseState cleans up a Lua state. In Go, most cleanup is handled by GC,
// but we nil out references to help the collector.
// Mirrors C Lua's close_state: runs all pending __gc finalizers before closing.
func CloseState(L *LuaState) {
	// Run a full GC cycle + call all pending __gc finalizers.
	// Uses GCStepFn callback which runs GCCollect (mark/sweep + GCTM)
	// without importing the api/api package.
	if L.Global != nil && L.Global.GCStepFn != nil {
		L.Global.GCStepFn(L)
	}

	// Mark state as closed
	if L.Global != nil {
		L.Global.GCClosed = true
	}

	// Reset CI chain
	L.CI = &L.BaseCI
	FreeCI(L)

	// Nil out references
	L.Stack = nil
	L.OpenUpval = nil
	L.Global = nil
}

// ---------------------------------------------------------------------------
// Warning system — mirrors C Lua's lua_warning / lua_setwarnf / luaE_warnerror
// ---------------------------------------------------------------------------

// Warning issues a warning message through the registered handler.
// If tocont is true, the message is a continuation (more parts follow).
// Mirrors C Lua's luaE_warning (lstate.c).
func (g *GlobalState) Warning(msg string, tocont bool) {
	if g.WarnF != nil {
		g.WarnF(g.WarnUd, msg, tocont)
	}
}

// WarnError generates a warning from a __gc error.
// Produces: "error in __gc (errmsg)"
// Mirrors C Lua's luaE_warnerror (lstate.c).
func (g *GlobalState) WarnError(where string, errMsg string) {
	g.Warning("error in ", true)
	g.Warning(where, true)
	g.Warning(" (", true)
	g.Warning(errMsg, true)
	g.Warning(")", false)
}

// SetWarnF sets the warning handler function and its user data.
// Mirrors C Lua's lua_setwarnf.
func (g *GlobalState) SetWarnF(f func(ud any, msg string, tocont bool), ud any) {
	g.WarnF = f
	g.WarnUd = ud
}

// ---------------------------------------------------------------------------
// Default warning handler — mirrors lauxlib.c warnfon/warnfoff/warnfcont
// ---------------------------------------------------------------------------

// DefaultWarnHandler is the standard warning handler installed by luaL_newstate.
// It supports @on/@off control messages and prints warnings to stderr.
// The handler uses a state machine with three modes:
//   - warnOff: warnings are off (only @on control message is processed)
//   - warnOn: ready for a new message (checks control messages, prints prefix)
//   - warnCont: continuing a multi-part message
//
// State is stored via closure over a *warnState.
func DefaultWarnHandler(g *GlobalState) {
	ws := &warnState{mode: warnOn} // warnings start enabled
	g.SetWarnF(func(ud any, msg string, tocont bool) {
		ws.handle(g, msg, tocont)
	}, g.MainThread)
}

// warnMode represents the warning handler state machine mode.
type warnMode int

const (
	warnOff  warnMode = iota // warnings disabled
	warnOn                   // ready for new message
	warnCont                 // in middle of multi-part message
)

type warnState struct {
	mode warnMode
}

// checkControl checks if msg is a control message (starts with '@', not a
// continuation). Returns true if it was a control message.
func (ws *warnState) checkControl(g *GlobalState, msg string, tocont bool) bool {
	if tocont || len(msg) == 0 || msg[0] != '@' {
		return false
	}
	switch msg[1:] {
	case "off":
		ws.mode = warnOff
	case "on":
		ws.mode = warnOn
	}
	// Unknown control messages are silently ignored (C Lua behavior)
	return true
}

func (ws *warnState) handle(g *GlobalState, msg string, tocont bool) {
	switch ws.mode {
	case warnOff:
		ws.checkControl(g, msg, tocont)
	case warnOn:
		if ws.checkControl(g, msg, tocont) {
			return
		}
		// Start new warning: accumulate in buffer
		g.WarnBuf = "Lua warning: " + msg
		if tocont {
			ws.mode = warnCont
		} else {
			fmt.Fprintln(os.Stderr, g.WarnBuf)
			g.WarnBuf = ""
			// stay in warnOn
		}
	case warnCont:
		g.WarnBuf += msg
		if !tocont {
			fmt.Fprintln(os.Stderr, g.WarnBuf)
			g.WarnBuf = ""
			ws.mode = warnOn
		}
	}
}
