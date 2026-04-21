package lua

// ---------------------------------------------------------------------------
// Call and load operations
// ---------------------------------------------------------------------------

// Call calls a function in unprotected mode.
// nArgs arguments are on the stack above the function.
// nResults is the number of expected results (or MultiRet for all).
func (L *State) Call(nArgs, nResults int) {
	L.s.Call(nArgs, nResults)
}

// PCall calls a function in protected mode.
// Returns a status code: OK on success, or an error code.
// If msgHandler is non-zero, it is the stack index of a message handler.
func (L *State) PCall(nArgs, nResults, msgHandler int) int {
	return L.s.PCall(nArgs, nResults, msgHandler)
}

// Load loads a Lua chunk from a string without executing it.
// Pushes the compiled chunk as a function on success.
// Returns a status code.
func (L *State) Load(code string, name string, mode string) int {
	return L.s.Load(code, name, mode)
}

// DoString loads and executes a Lua string.
// Returns nil on success, or an error.
func (L *State) DoString(code string) error {
	return L.s.DoString(code)
}

// DoFile loads and executes a Lua file.
// Returns nil on success, or an error.
func (L *State) DoFile(filename string) error {
	return L.s.DoFile(filename)
}

// Error raises a Lua error with the value at the top of the stack
// as the error object. This function does not return.
func (L *State) Error() int {
	return L.s.Error()
}
