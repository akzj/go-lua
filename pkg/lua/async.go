package lua

import "sync"

// Future represents the result of an async operation.
// Thread-safe: can be resolved from any goroutine.
type Future struct {
	mu      sync.Mutex
	done    bool
	value   any
	err     error
	waiters []chan struct{} // notify when done
}

// NewFuture creates a new unresolved Future.
func NewFuture() *Future {
	return &Future{}
}

// Resolve completes the Future with a value.
// Thread-safe. Can be called from any goroutine.
// Only the first call takes effect; subsequent calls are no-ops.
func (f *Future) Resolve(value any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return
	}
	f.done = true
	f.value = value
	for _, w := range f.waiters {
		close(w)
	}
	f.waiters = nil
}

// Reject completes the Future with an error.
// Thread-safe. Only the first Resolve/Reject takes effect.
func (f *Future) Reject(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return
	}
	f.done = true
	f.err = err
	for _, w := range f.waiters {
		close(w)
	}
	f.waiters = nil
}

// IsDone returns whether the Future has been resolved or rejected.
func (f *Future) IsDone() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.done
}

// Wait returns a channel that closes when the Future completes.
// If the Future is already done, the returned channel is already closed.
func (f *Future) Wait() <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	ch := make(chan struct{})
	f.waiters = append(f.waiters, ch)
	return ch
}

// Result returns the value and error. Only meaningful after IsDone() == true.
func (f *Future) Result() (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.value, f.err
}
