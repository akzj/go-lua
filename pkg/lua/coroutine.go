package lua

// ---------------------------------------------------------------------------
// Coroutine operations
// ---------------------------------------------------------------------------

// NewThread creates a new Lua thread (coroutine), pushes it onto the stack,
// and returns a *State representing the new thread.
func (L *State) NewThread() *State {
	apiThread := L.s.NewThread()
	return &State{s: apiThread}
}

// Resume starts or resumes a coroutine.
// from is the calling coroutine (or nil).
// Returns (status, nresults). Status is OK when finished, or Yield when suspended.
func (L *State) Resume(from *State, nArgs int) (int, int) {
	var fromAPI *State
	if from != nil {
		fromAPI = from
	}
	if fromAPI != nil {
		return L.s.Resume(fromAPI.s, nArgs)
	}
	return L.s.Resume(nil, nArgs)
}

// Yield yields the current coroutine. nResults values on the stack are
// passed back to the resume caller.
func (L *State) Yield(nResults int) int {
	return L.s.Yield(nResults)
}

// YieldK yields with a continuation function.
func (L *State) YieldK(nResults int, ctx int, k Function) int {
	if k != nil {
		return L.s.YieldK(nResults, ctx, L.wrapFunction(k))
	}
	return L.s.YieldK(nResults, ctx, nil)
}

// XMove moves n values from L's stack to to's stack.
func (L *State) XMove(to *State, n int) {
	L.s.XMove(to.s, n)
}

// ToThread converts the value at idx to a *State (thread).
// Returns nil if the value is not a thread.
func (L *State) ToThread(idx int) *State {
	apiThread := L.s.ToThread(idx)
	if apiThread == nil {
		return nil
	}
	return &State{s: apiThread}
}

// Status returns the status of the coroutine.
func (L *State) Status() int {
	return L.s.Status()
}

// IsYieldable returns true if the running coroutine can yield.
func (L *State) IsYieldable() bool {
	return L.s.IsYieldable()
}
