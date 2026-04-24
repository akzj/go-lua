package lua

import (
	"fmt"
	"time"
)

// Scheduler manages async coroutines within a single Lua State.
// NOT thread-safe — must be used from a single goroutine (the main/event loop goroutine).
type Scheduler struct {
	L       *State
	pending []pendingCoroutine
}

type pendingCoroutine struct {
	thread *State  // the coroutine
	future *Future // what it's waiting for (nil = resume unconditionally)
	ref    int     // registry reference to prevent GC
}

// NewScheduler creates a new Scheduler for the given State.
func NewScheduler(L *State) *Scheduler {
	return &Scheduler{L: L}
}

// Spawn creates a new coroutine from the function at the top of L's stack
// and starts executing it. If it yields (via await), it's added to the pending list.
// The function is popped from the stack.
func (s *Scheduler) Spawn(L *State) error {
	// Stack: [..., function]
	co := L.NewThread() // pushes thread onto L's stack → [..., function, thread]

	// Move the function from L to co's stack
	L.PushValue(-2)  // copy function → [..., function, thread, function]
	L.XMove(co, 1)   // move copy to co → L: [..., function, thread], co: [function]

	// Save a registry reference to the thread to prevent GC
	L.PushValue(-1)            // copy thread → [..., function, thread, thread]
	ref := L.Ref(RegistryIndex) // pops thread copy → [..., function, thread]

	L.Pop(2) // pop thread and original function → [...]

	// Resume the coroutine (function is already on co's stack)
	status, _ := co.Resume(L, 0)

	return s.handleResumeResult(status, pendingCoroutine{
		thread: co,
		future: nil,
		ref:    ref,
	})
}

// Tick checks all pending coroutines and resumes any whose Future is done.
// Returns the number of still-pending coroutines.
// Call this in your event loop / main loop.
func (s *Scheduler) Tick() int {
	remaining := make([]pendingCoroutine, 0, len(s.pending))

	for _, pc := range s.pending {
		if pc.future == nil || pc.future.IsDone() {
			// Resume this coroutine
			if pc.future != nil {
				val, err := pc.future.Result()
				if err != nil {
					pc.thread.PushNil()
					pc.thread.PushString(err.Error())
					status, _ := pc.thread.Resume(s.L, 2)
					if err2 := s.collectResumed(status, pc, &remaining); err2 != nil {
						// Coroutine errored — already unref'd inside collectResumed
						continue
					}
				} else {
					pc.thread.PushAny(val)
					status, _ := pc.thread.Resume(s.L, 1)
					if err2 := s.collectResumed(status, pc, &remaining); err2 != nil {
						continue
					}
				}
			} else {
				// No future, just resume
				status, _ := pc.thread.Resume(s.L, 0)
				if err := s.collectResumed(status, pc, &remaining); err != nil {
					continue
				}
			}
		} else {
			remaining = append(remaining, pc)
		}
	}

	s.pending = remaining
	return len(s.pending)
}

// collectResumed handles the result of resuming a coroutine during Tick.
// If the coroutine yielded again, it's appended to dst. If it finished or
// errored, the registry reference is released.
func (s *Scheduler) collectResumed(status int, pc pendingCoroutine, dst *[]pendingCoroutine) error {
	if status == Yield {
		// Check if it yielded a Future
		var future *Future
		if pc.thread.GetTop() >= 1 {
			ud := pc.thread.UserdataValue(1)
			if f, ok := ud.(*Future); ok {
				future = f
			}
			pc.thread.Pop(pc.thread.GetTop()) // clean yielded values
		}
		*dst = append(*dst, pendingCoroutine{
			thread: pc.thread,
			future: future,
			ref:    pc.ref,
		})
		return nil
	}

	// Finished (OK) or error — release reference
	s.L.Unref(RegistryIndex, pc.ref)

	if status != OK {
		msg, _ := pc.thread.ToString(-1)
		return fmt.Errorf("coroutine error: %s", msg)
	}
	return nil
}

// handleResumeResult handles the result of the initial Resume in Spawn.
func (s *Scheduler) handleResumeResult(status int, pc pendingCoroutine) error {
	if status == Yield {
		// Check if it yielded a Future
		var future *Future
		if pc.thread.GetTop() >= 1 {
			ud := pc.thread.UserdataValue(1)
			if f, ok := ud.(*Future); ok {
				future = f
			}
			pc.thread.Pop(pc.thread.GetTop()) // clean yielded values
		}
		s.pending = append(s.pending, pendingCoroutine{
			thread: pc.thread,
			future: future,
			ref:    pc.ref,
		})
		return nil
	}

	if status == OK {
		// Coroutine finished immediately
		s.L.Unref(RegistryIndex, pc.ref)
		return nil
	}

	// Error
	msg, _ := pc.thread.ToString(-1)
	s.L.Unref(RegistryIndex, pc.ref)
	return fmt.Errorf("coroutine error: %s", msg)
}

// Pending returns the number of pending coroutines.
func (s *Scheduler) Pending() int {
	return len(s.pending)
}

// WaitAll blocks until all pending coroutines complete, polling with Tick.
// Useful for testing. Returns error on timeout.
func (s *Scheduler) WaitAll(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for s.Pending() > 0 {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout: %d coroutines still pending", s.Pending())
		}
		s.Tick()
		time.Sleep(1 * time.Millisecond)
	}
	return nil
}
