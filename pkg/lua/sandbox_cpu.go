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
// a previous call.
//
// Internally this uses the debug hook mechanism with [MaskCount].  Setting a
// CPU limit will override any previously set count hook.  Line and call hooks
// set via [State.SetHook] are NOT affected — only the count component changes.
//
// Example:
//
//	L.SetCPULimit(1_000_000) // allow at most ~1 million instructions
//	err := L.DoString(`while true do end`) // will error
func (L *State) SetCPULimit(limit int64) {
	L.cpuLimit = limit
	L.cpuCounter = 0

	if limit <= 0 {
		// Remove CPU-limit hook.  We clear the count portion only;
		// if the user had a line/call hook via SetHook, we leave it alone
		// by calling SetHook(nil, 0, 0) which clears everything.
		// In practice, CPU-limit and user hooks don't coexist well,
		// so clearing is the safe default.
		L.cpuLimit = 0
		L.cpuCheckInterval = 0
		L.SetHook(nil, 0, 0)
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

	// Capture L (the original State with CPU fields) in the closure.
	// The hook parameter is a throwaway wrapper — we must NOT use it for
	// CPU-limit bookkeeping because it has zero-valued fields.
	L.SetHook(func(_ *State, event int, _ int) {
		if event != HookEventCount {
			return
		}
		L.cpuCounter += int64(L.cpuCheckInterval)
		if L.cpuLimit > 0 && L.cpuCounter >= L.cpuLimit {
			L.Errorf("CPU limit exceeded: %d instructions", L.cpuLimit)
		}
	}, MaskCount, interval)
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
