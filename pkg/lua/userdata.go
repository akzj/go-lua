package lua

// ---------------------------------------------------------------------------
// Userdata operations
// ---------------------------------------------------------------------------

// NewUserdata creates a new full userdata with nUV user values and pushes it
// onto the stack. The size parameter is ignored (Go manages memory).
// Returns a handle that can be used with SetUserdataValue.
func (L *State) NewUserdata(size int, nUV int) {
	L.s.NewUserdata(size, nUV)
}

// UserdataValue returns the Go value stored in the userdata at idx.
// Returns nil if the value is not a full userdata or light userdata.
func (L *State) UserdataValue(idx int) any {
	return L.s.ToUserdata(idx)
}

// SetUserdataValue sets the Go value stored in the full userdata at idx.
func (L *State) SetUserdataValue(idx int, v any) {
	ud := L.s.GetUserdataObj(idx)
	if ud != nil {
		ud.Data = v
	}
}

// GetIUserValue pushes the n-th user value of the userdata at idx onto the stack.
// Returns the type of the pushed value, or TypeNone if invalid.
func (L *State) GetIUserValue(idx int, n int) Type {
	return toPublicType(L.s.GetIUserValue(idx, n))
}

// SetIUserValue sets the n-th user value of the userdata at idx to the value
// at the top of the stack. Pops the value. Returns false if the operation fails.
func (L *State) SetIUserValue(idx int, n int) bool {
	return L.s.SetIUserValue(idx, n)
}

// GetUpvalue pushes the value of upvalue n of the closure at funcIdx.
// Returns (name, true) if the upvalue exists, ("", false) otherwise.
func (L *State) GetUpvalue(funcIdx, n int) (string, bool) {
	return L.s.GetUpvalue(funcIdx, n)
}

// SetUpvalue sets upvalue n of the closure at funcIdx from the top of the stack.
// Returns (name, true) if the upvalue exists, ("", false) otherwise.
func (L *State) SetUpvalue(funcIdx, n int) (string, bool) {
	return L.s.SetUpvalue(funcIdx, n)
}
