package lua

import "context"

// SetContext associates a Go [context.Context] with this Lua state.
// When the context is cancelled or times out, the currently running Lua code
// receives a Lua error: "context cancelled: <reason>".
//
// Internally this uses the debug hook mechanism with [MaskCount], the same
// mechanism used by [State.SetCPULimit]. The combined hook checks both
// context cancellation and CPU limits on every interval (default 1000
// instructions). Setting a context will reinstall the hook to include the
// context check; any previously set count hook is replaced.
//
// Pass nil to remove the context association and its hook (unless a CPU limit
// is still active, in which case the CPU-only hook remains).
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	L.SetContext(ctx)
//	err := L.DoString(`while true do end`) // will error after ~5 seconds
func (L *State) SetContext(ctx context.Context) {
	L.ctx = ctx
	L.s.Ctx = ctx // also store on api.State so wrapFunction wrappers can access it
	L.installCombinedHook()
}

// Context returns the associated [context.Context], or [context.Background]
// if none was set. Reads from the internal api.State so the context is
// accessible even from wrapFunction-created State wrappers.
func (L *State) Context() context.Context {
	if L.s.Ctx != nil {
		return L.s.Ctx
	}
	if L.ctx != nil {
		return L.ctx
	}
	return context.Background()
}

// installCombinedHook installs a single count hook that checks both context
// cancellation and CPU limits. This is called by both [SetContext] and
// [SetCPULimit] to ensure the two features coexist in a single hook.
//
// If neither context nor CPU limit is active, the hook is removed.
func (L *State) installCombinedHook() {
	hasCtx := L.ctx != nil
	hasCPU := L.cpuLimit > 0

	if !hasCtx && !hasCPU {
		// Nothing to check — remove hook.
		L.cpuCheckInterval = 0
		L.SetHook(nil, 0, 0)
		return
	}

	interval := L.cpuCheckInterval
	if interval == 0 {
		interval = defaultCPUCheckInterval
	}
	L.cpuCheckInterval = interval

	// Capture L (the original State with ctx/CPU fields) in the closure.
	// The hook parameter is a throwaway wrapper — we must NOT use it for
	// bookkeeping because it has zero-valued fields.
	L.SetHook(func(_ *State, event int, _ int) {
		if event != HookEventCount {
			return
		}

		// Check context cancellation.
		if L.ctx != nil {
			select {
			case <-L.ctx.Done():
				L.Errorf("context cancelled: %v", L.ctx.Err())
			default:
			}
		}

		// Check CPU limit.
		if L.cpuLimit > 0 {
			L.cpuCounter += int64(L.cpuCheckInterval)
			if L.cpuCounter >= L.cpuLimit {
				L.Errorf("CPU limit exceeded: %d instructions", L.cpuLimit)
			}
		}
	}, MaskCount, interval)
}
