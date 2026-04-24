package lua

import (
	"context"
	"sync"
	"sync/atomic"
)

// Task represents a unit of Lua work to be executed asynchronously by an
// [Executor].
type Task struct {
	// ID identifies this task (for correlating results).
	ID string

	// Code is the Lua source code to execute.
	// Either Code or Func must be set (not both).
	Code string

	// Func is a function to run with a Lua State.
	// Use this for complex operations that need direct State access.
	// Either Code or Func must be set (not both).
	Func func(L *State) (any, error)
}

// Result is the outcome of an async Lua [Task].
type Result struct {
	// ID matches the [Task.ID] that produced this result.
	ID string

	// Value is the return value (from [Task.Func], or nil for [Task.Code]).
	Value any

	// Error is non-nil if the Lua execution failed.
	Error error
}

// ExecutorConfig configures an [Executor].
type ExecutorConfig struct {
	// PoolConfig configures the underlying [StatePool].
	PoolConfig PoolConfig

	// ResultBuffer is the size of the results channel buffer.
	// Default: 64.
	ResultBuffer int
}

// Executor manages async Lua execution using a pool of States.
//
// Thread-safe: [Executor.Submit] can be called from any goroutine.
// Each submitted [Task] runs in its own goroutine with an exclusively-owned
// Lua [State] obtained from the underlying [StatePool].
type Executor struct {
	pool    *StatePool
	results chan Result
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	pending int64 // atomic counter of pending tasks
}

// NewExecutor creates a new async Lua executor.
func NewExecutor(config ExecutorConfig) *Executor {
	if config.ResultBuffer <= 0 {
		config.ResultBuffer = 64
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Executor{
		pool:    NewStatePool(config.PoolConfig),
		results: make(chan Result, config.ResultBuffer),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Submit submits a task for async execution.
// Returns immediately. Results arrive on the [Executor.Results] channel.
// Returns false if the executor has been shut down.
func (e *Executor) Submit(task Task) bool {
	select {
	case <-e.ctx.Done():
		return false
	default:
	}

	atomic.AddInt64(&e.pending, 1)
	e.wg.Add(1)

	go func() {
		defer e.wg.Done()
		defer atomic.AddInt64(&e.pending, -1)

		L := e.pool.Get()
		defer e.pool.Put(L)

		var result Result
		result.ID = task.ID

		if task.Func != nil {
			result.Value, result.Error = task.Func(L)
		} else {
			result.Error = L.DoString(task.Code)
		}

		// Send result (respect context cancellation).
		select {
		case e.results <- result:
		case <-e.ctx.Done():
		}
	}()

	return true
}

// Results returns the channel on which execution results are delivered.
func (e *Executor) Results() <-chan Result {
	return e.results
}

// Pending returns the number of tasks currently executing.
func (e *Executor) Pending() int64 {
	return atomic.LoadInt64(&e.pending)
}

// Shutdown gracefully shuts down the executor.
// It waits for all pending tasks to complete, then closes the pool and
// the results channel.
func (e *Executor) Shutdown() {
	e.cancel()
	e.wg.Wait()
	e.pool.Close()
	close(e.results)
}
