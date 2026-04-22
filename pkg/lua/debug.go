package lua

import (
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/state"
)

// HookEvent constants for use with [HookFunc] callbacks.
const (
	HookEventCall     = state.HookCall     // function call
	HookEventReturn   = state.HookReturn   // function return
	HookEventLine     = state.HookLine     // new source line
	HookEventCount    = state.HookCount    // instruction count reached
	HookEventTailCall = state.HookTailCall // tail call
)

// HookFunc is the callback type for debug hooks set via [State.SetHook].
//
// The callback receives the Lua state, the hook event (one of the HookEvent*
// constants), and the current source line number (-1 for non-line events).
// Inside the callback you may inspect the call stack via [State.GetStack]
// and [State.GetInfo], but you must not yield or raise errors.
type HookFunc func(L *State, event int, currentLine int)

// SetHook sets the debug hook function with the given mask and count.
// mask is a combination of [MaskCall], [MaskRet], [MaskLine], [MaskCount].
// count is the instruction count for count hooks (0 to disable count hooks).
// Pass nil as f to remove the current hook.
//
// Example:
//
//	L.SetHook(func(L *lua.State, event int, line int) {
//	    if event == lua.HookEventLine {
//	        fmt.Printf("executing line %d\n", line)
//	    }
//	}, lua.MaskLine, 0)
func (L *State) SetHook(f HookFunc, mask, count int) {
	ls := L.s.Internal.(*state.LuaState)
	if f == nil {
		ls.Hook = nil
		ls.HookMask = 0
		ls.BaseHookCount = 0
		ls.HookCount = 0
		ls.AllowHook = true
		L.goHook = nil
		return
	}
	L.goHook = f
	cfn := state.CFunction(func(ls2 *state.LuaState) int {
		f(&State{s: L.s}, ls2.HookEvent, ls2.HookLine)
		return 0
	})
	ls.Hook = object.TValue{
		Tt:  object.TagLightCFunc,
		Obj: cfn,
	}
	ls.HookMask = mask
	ls.BaseHookCount = count
	ls.HookCount = count
}

// GetHook returns the current hook function, mask, and count.
// Returns (nil, 0, 0) if no hook is set via the public API.
func (L *State) GetHook() (HookFunc, int, int) {
	return L.goHook, L.HookMask(), L.HookCount()
}

// ---------------------------------------------------------------------------
// Stack inspection
// ---------------------------------------------------------------------------

// GetStack fills a DebugInfo for the given call level.
// Level 0 is the current running function, level 1 is the function that
// called the current one, etc.
// Returns (info, true) on success, (nil, false) if the level is invalid.
func (L *State) GetStack(level int) (*DebugInfo, bool) {
	ar, ok := L.s.GetStack(level)
	if !ok {
		return nil, false
	}
	d := &DebugInfo{internal: ar}
	d.copyFromInternal()
	return d, true
}

// GetInfo fills debug info fields specified by the what string.
// Characters in what select which fields to fill:
//   - 'n': Name, NameWhat
//   - 'S': Source, ShortSrc, What, LineDefined, LastLineDefined
//   - 'l': CurrentLine
//   - 'u': NUps, NParams, IsVararg
//   - 'f': pushes the function onto the stack
//   - 'r': FTransfer, NTransfer
//   - 't': IsTailCall, ExtraArgs
func (L *State) GetInfo(what string, ar *DebugInfo) bool {
	if ar == nil || ar.internal == nil {
		return false
	}
	ok := L.s.GetInfo(what, ar.internal)
	ar.copyFromInternal()
	return ok
}

// GetLocal pushes the value of local variable n of the function at the
// given debug level. Returns the variable name, or "" if not found.
func (L *State) GetLocal(ar *DebugInfo, n int) string {
	if ar == nil || ar.internal == nil {
		return ""
	}
	return L.s.GetLocal(ar.internal, n)
}

// SetLocal sets the value of local variable n from the top of the stack.
// Returns the variable name, or "" if not found.
func (L *State) SetLocal(ar *DebugInfo, n int) string {
	if ar == nil || ar.internal == nil {
		return ""
	}
	return L.s.SetLocal(ar.internal, n)
}

// ---------------------------------------------------------------------------
// Legacy hook API (deprecated — use SetHook/GetHook instead)
// ---------------------------------------------------------------------------

// SetHookFields sets the hook mask and count on the state.
// Deprecated: Use [State.SetHook] instead for a complete hook API.
func (L *State) SetHookFields(mask, count int) {
	L.s.SetHookFields(mask, count)
}

// ClearHookFields clears all hook fields.
// Deprecated: Use SetHook(nil, 0, 0) instead.
func (L *State) ClearHookFields() {
	L.s.ClearHookFields()
}

// SetHookMarker sets a non-nil marker to indicate hooks are active.
// Deprecated: Use [State.SetHook] instead.
func (L *State) SetHookMarker() {
	L.s.SetHookMarker()
}

// HookMask returns the current hook mask.
func (L *State) HookMask() int {
	return L.s.HookMask()
}

// HookCount returns the base hook count (for count hooks).
func (L *State) HookCount() int {
	return L.s.HookCount()
}

// HookActive returns true if any hooks are set.
func (L *State) HookActive() bool {
	return L.s.HookActive()
}

// HasCallFrames returns true if the thread has call frames above the base.
func (L *State) HasCallFrames() bool {
	return L.s.HasCallFrames()
}

// ---------------------------------------------------------------------------
// Upvalue identity and joining
// ---------------------------------------------------------------------------

// UpvalueId returns a unique identifier for upvalue n of the closure at funcIdx.
func (L *State) UpvalueId(funcIdx, n int) interface{} {
	return L.s.UpvalueId(funcIdx, n)
}

// UpvalueJoin makes the n1-th upvalue of funcIdx1 refer to n2-th of funcIdx2.
func (L *State) UpvalueJoin(funcIdx1, n1, funcIdx2, n2 int) {
	L.s.UpvalueJoin(funcIdx1, n1, funcIdx2, n2)
}
