package lua

// defaultCPUCheckInterval is the number of VM instructions between each
// hook invocation.  Checking every single instruction is too expensive;
// 1000 gives a good balance between overhead and accuracy.
const defaultCPUCheckInterval = 1000

// SetCPULimit sets the maximum number of Lua VM instructions this state may
// execute.  When the limit is reached the currently running Lua code receives
// a Lua error: "CPU limit exceeded: <limit> instructions".
//
// Set limit=0 to remove the CPU limit and clear the count hook installed by
// a previous call (unless a context is still active via [State.SetContext],
// in which case the context-only hook remains).
//
// Internally this uses the debug hook mechanism with [MaskCount].  Setting a
// CPU limit will override any previously set count hook.  Line and call hooks
// set via [State.SetHook] are NOT affected — only the count component changes.
//
// When used together with [State.SetContext], both checks are combined into a
// single hook for efficiency.
//
// Example:
//
//	L.SetCPULimit(1_000_000) // allow at most ~1 million instructions
//	err := L.DoString(`while true do end`) // will error
func (L *State) SetCPULimit(limit int64) {
	L.cpuLimit = limit
	L.cpuCounter = 0

	if limit <= 0 {
		L.cpuLimit = 0
		// Reinstall combined hook — will remove hook entirely if no
		// context is active, or keep a context-only hook if one is set.
		L.installCombinedHook()
		return
	}

	interval := defaultCPUCheckInterval
	// If the limit is very small, reduce the interval so we don't overshoot.
	if int64(interval) > limit {
		interval = int(limit)
		if interval < 1 {
			interval = 1
		}
	}
	L.cpuCheckInterval = interval

	// Install the combined hook that checks both CPU limit and context.
	L.installCombinedHook()
}

// ResetCPUCounter resets the instruction counter to 0 without changing the
// limit.  This is useful when reusing a state across multiple script
// executions so that each execution starts with a fresh budget.
func (L *State) ResetCPUCounter() {
	L.cpuCounter = 0
}

// CPUInstructionsUsed returns the approximate number of VM instructions
// executed since the last [State.SetCPULimit] or [State.ResetCPUCounter] call.
// The value is approximate because the count hook fires every N instructions
// (default 1000), not on every single instruction.
func (L *State) CPUInstructionsUsed() int64 {
	return L.cpuCounter
}
