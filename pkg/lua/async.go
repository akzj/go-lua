package lua

import (
	"context"
	"errors"
	"sync"
)

// ErrCancelled is returned by Future.Result when the Future was cancelled.
var ErrCancelled = errors.New("cancelled")

// Future represents the result of an async operation.
// Thread-safe: can be resolved from any goroutine.
type Future struct {
	mu      sync.Mutex
	done    bool
	value   any
	err     error
	waiters []chan struct{}     // notify when done
	cancel  context.CancelFunc // cancels associated context when Future completes
}

// NewFuture creates a new unresolved Future.
func NewFuture() *Future {
	return &Future{}
}

// NewFutureWithContext creates a Future with an associated context.
// The returned context is derived from parent and will be cancelled when
// the Future completes (Resolve, Reject, or Cancel). Use this context in
// Go goroutines to support cancellation of in-flight operations (HTTP
// requests, IO, etc.).
//
// Example:
//
//	future, ctx := lua.NewFutureWithContext(parentCtx)
//	go func() {
//	    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
//	    resp, err := http.DefaultClient.Do(req)
//	    if err != nil {
//	        future.Reject(err)
//	        return
//	    }
//	    future.Resolve(body)
//	}()
func NewFutureWithContext(parent context.Context) (*Future, context.Context) {
	ctx, cancelFn := context.WithCancel(parent)
	return &Future{cancel: cancelFn}, ctx
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
	if f.cancel != nil {
		f.cancel() // release context resources
	}
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
	if f.cancel != nil {
		f.cancel() // release context resources
	}
	for _, w := range f.waiters {
		close(w)
	}
	f.waiters = nil
}

// Cancel cancels the Future. Waiters are notified with ErrCancelled.
// Thread-safe. No-op if already done.
func (f *Future) Cancel() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return
	}
	f.done = true
	f.err = ErrCancelled
	if f.cancel != nil {
		f.cancel()
	}
	for _, w := range f.waiters {
		close(w)
	}
	f.waiters = nil
}

// IsCancelled returns true if the Future was cancelled.
func (f *Future) IsCancelled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.done && f.err == ErrCancelled
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
