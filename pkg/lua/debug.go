package lua

// ---------------------------------------------------------------------------
// Debug interface
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

// SetHookFields sets the hook mask and count on the state.
// mask is a combination of MaskCall, MaskRet, MaskLine, MaskCount.
func (L *State) SetHookFields(mask, count int) {
	L.s.SetHookFields(mask, count)
}

// ClearHookFields clears all hook fields.
func (L *State) ClearHookFields() {
	L.s.ClearHookFields()
}

// SetHookMarker sets a non-nil marker to indicate hooks are active.
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
// This can be used to check if two closures share the same upvalue.
// Returns nil if the upvalue doesn't exist.
// Mirrors: lua_upvalueid in lapi.c
func (L *State) UpvalueId(funcIdx, n int) interface{} {
	return L.s.UpvalueId(funcIdx, n)
}

// UpvalueJoin makes the n1-th upvalue of the closure at funcIdx1 refer to
// the n2-th upvalue of the closure at funcIdx2.
// Both closures must be Lua closures (not C closures).
// Mirrors: lua_upvaluejoin in lapi.c
func (L *State) UpvalueJoin(funcIdx1, n1, funcIdx2, n2 int) {
	L.s.UpvalueJoin(funcIdx1, n1, funcIdx2, n2)
}
