package lua

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Future tests
// ---------------------------------------------------------------------------

func TestFuture_ResolveValue(t *testing.T) {
	f := NewFuture()
	f.Resolve(42)
	if !f.IsDone() {
		t.Fatal("expected done")
	}
	val, err := f.Result()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Fatalf("expected 42, got %v", val)
	}
}

func TestFuture_RejectError(t *testing.T) {
	f := NewFuture()
	f.Reject(fmt.Errorf("boom"))
	if !f.IsDone() {
		t.Fatal("expected done")
	}
	val, err := f.Result()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "boom" {
		t.Fatalf("expected 'boom', got %v", err)
	}
	if val != nil {
		t.Fatalf("expected nil val, got %v", val)
	}
}

func TestFuture_Wait(t *testing.T) {
	f := NewFuture()
	ch := f.Wait()
	select {
	case <-ch:
		t.Fatal("should not be done yet")
	default:
	}
	f.Resolve("hello")
	select {
	case <-ch:
		// good
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for future")
	}
}

func TestFuture_WaitAlreadyDone(t *testing.T) {
	f := NewFuture()
	f.Resolve(99)
	ch := f.Wait()
	select {
	case <-ch:
		// good — already done
	default:
		t.Fatal("Wait() on done future should return closed channel")
	}
}

func TestFuture_ConcurrentResolve(t *testing.T) {
	f := NewFuture()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			f.Resolve(v)
		}(i)
	}
	wg.Wait()
	if !f.IsDone() {
		t.Fatal("expected done")
	}
	val, _ := f.Result()
	// Value should be one of 0..99 (first writer wins)
	v, ok := val.(int)
	if !ok {
		t.Fatalf("expected int, got %T", val)
	}
	if v < 0 || v >= 100 {
		t.Fatalf("unexpected value: %d", v)
	}
}

func TestFuture_DoubleResolveIgnored(t *testing.T) {
	f := NewFuture()
	f.Resolve("first")
	f.Resolve("second") // should be ignored
	val, _ := f.Result()
	if val != "first" {
		t.Fatalf("expected 'first', got %v", val)
	}
}

func TestFuture_ResolveAfterRejectIgnored(t *testing.T) {
	f := NewFuture()
	f.Reject(fmt.Errorf("err"))
	f.Resolve("val") // should be ignored
	_, err := f.Result()
	if err == nil || err.Error() != "err" {
		t.Fatalf("expected error 'err', got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Scheduler tests
// ---------------------------------------------------------------------------

func TestScheduler_SpawnSimple(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	// Load a function that just sets a global
	err := L.DoString(`
		result = nil
		function task()
			result = 42
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	if _, err := sched.Spawn(L); err != nil {
		t.Fatal(err)
	}

	// Should have completed immediately (no yield)
	if sched.Pending() != 0 {
		t.Fatalf("expected 0 pending, got %d", sched.Pending())
	}

	// Verify result
	L.GetGlobal("result")
	val := L.ToAny(-1)
	L.Pop(1)
	if val != int64(42) {
		t.Fatalf("expected 42, got %v (%T)", val, val)
	}
}

func TestScheduler_SpawnWithFuture(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	// Create a future in Go
	future := NewFuture()

	// Register a Go function that returns the future
	L.PushFunction(func(L *State) int {
		L.PushUserdata(future)
		return 1
	})
	L.SetGlobal("get_future")

	// Load async module
	err := L.DoString(`
		local async = require("async")
		result = nil
		function task()
			local f = get_future()
			local val = async.await(f)
			result = val
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	if _, err := sched.Spawn(L); err != nil {
		t.Fatal(err)
	}

	// Should be pending (waiting for future)
	if sched.Pending() != 1 {
		t.Fatalf("expected 1 pending, got %d", sched.Pending())
	}

	// Resolve the future
	future.Resolve("hello from go")

	// Tick should resume the coroutine
	sched.Tick()

	if sched.Pending() != 0 {
		t.Fatalf("expected 0 pending after tick, got %d", sched.Pending())
	}

	// Verify result
	L.GetGlobal("result")
	val := L.ToAny(-1)
	L.Pop(1)
	if val != "hello from go" {
		t.Fatalf("expected 'hello from go', got %v", val)
	}
}

func TestScheduler_SpawnWithFutureError(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)
	future := NewFuture()

	L.PushFunction(func(L *State) int {
		L.PushUserdata(future)
		return 1
	})
	L.SetGlobal("get_future")

	err := L.DoString(`
		local async = require("async")
		result_val = nil
		result_err = nil
		function task()
			local f = get_future()
			local val, err = async.await(f)
			result_val = val
			result_err = err
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	if _, err := sched.Spawn(L); err != nil {
		t.Fatal(err)
	}

	// Reject the future
	future.Reject(fmt.Errorf("something failed"))

	sched.Tick()

	if sched.Pending() != 0 {
		t.Fatalf("expected 0 pending, got %d", sched.Pending())
	}

	// result_val should be nil, result_err should be the error message
	L.GetGlobal("result_val")
	val := L.ToAny(-1)
	L.Pop(1)
	if val != nil {
		t.Fatalf("expected nil val, got %v", val)
	}

	L.GetGlobal("result_err")
	errVal := L.ToAny(-1)
	L.Pop(1)
	if errVal != "something failed" {
		t.Fatalf("expected 'something failed', got %v", errVal)
	}
}

func TestScheduler_MultipleCoroutines(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	f1 := NewFuture()
	f2 := NewFuture()

	L.PushFunction(func(L *State) int {
		L.PushUserdata(f1)
		return 1
	})
	L.SetGlobal("get_f1")

	L.PushFunction(func(L *State) int {
		L.PushUserdata(f2)
		return 1
	})
	L.SetGlobal("get_f2")

	err := L.DoString(`
		local async = require("async")
		r1 = nil
		r2 = nil
		function task1()
			local f = get_f1()
			r1 = async.await(f)
		end
		function task2()
			local f = get_f2()
			r2 = async.await(f)
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task1")
	sched.Spawn(L) //nolint:errcheck
	L.GetGlobal("task2")
	sched.Spawn(L) //nolint:errcheck

	if sched.Pending() != 2 {
		t.Fatalf("expected 2 pending, got %d", sched.Pending())
	}

	// Resolve f2 first
	f2.Resolve("two")
	sched.Tick()
	if sched.Pending() != 1 {
		t.Fatalf("expected 1 pending, got %d", sched.Pending())
	}

	// Resolve f1
	f1.Resolve("one")
	sched.Tick()
	if sched.Pending() != 0 {
		t.Fatalf("expected 0 pending, got %d", sched.Pending())
	}

	L.GetGlobal("r1")
	v1 := L.ToAny(-1)
	L.Pop(1)
	L.GetGlobal("r2")
	v2 := L.ToAny(-1)
	L.Pop(1)

	if v1 != "one" {
		t.Fatalf("expected 'one', got %v", v1)
	}
	if v2 != "two" {
		t.Fatalf("expected 'two', got %v", v2)
	}
}

func TestScheduler_NestedAwait(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	f1 := NewFuture()
	f2 := NewFuture()
	callCount := 0

	L.PushFunction(func(L *State) int {
		callCount++
		if callCount == 1 {
			L.PushUserdata(f1)
		} else {
			L.PushUserdata(f2)
		}
		return 1
	})
	L.SetGlobal("next_future")

	err := L.DoString(`
		local async = require("async")
		result = nil
		function task()
			local f1 = next_future()
			local v1 = async.await(f1)
			local f2 = next_future()
			local v2 = async.await(f2)
			result = v1 .. " " .. v2
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	sched.Spawn(L)

	if sched.Pending() != 1 {
		t.Fatalf("expected 1 pending, got %d", sched.Pending())
	}

	f1.Resolve("hello")
	sched.Tick()

	// Should still be pending (waiting for f2)
	if sched.Pending() != 1 {
		t.Fatalf("expected 1 pending after first resolve, got %d", sched.Pending())
	}

	f2.Resolve("world")
	sched.Tick()

	if sched.Pending() != 0 {
		t.Fatalf("expected 0 pending after second resolve, got %d", sched.Pending())
	}

	L.GetGlobal("result")
	val := L.ToAny(-1)
	L.Pop(1)
	if val != "hello world" {
		t.Fatalf("expected 'hello world', got %v", val)
	}
}

func TestScheduler_WaitAll(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)
	future := NewFuture()

	L.PushFunction(func(L *State) int {
		L.PushUserdata(future)
		return 1
	})
	L.SetGlobal("get_future")

	err := L.DoString(`
		local async = require("async")
		result = nil
		function task()
			local f = get_future()
			result = async.await(f)
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	sched.Spawn(L)

	// Resolve in background
	go func() {
		time.Sleep(10 * time.Millisecond)
		future.Resolve("async result")
	}()

	if err := sched.WaitAll(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("result")
	val := L.ToAny(-1)
	L.Pop(1)
	if val != "async result" {
		t.Fatalf("expected 'async result', got %v", val)
	}
}

func TestScheduler_WaitAllTimeout(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)
	future := NewFuture() // never resolved

	L.PushFunction(func(L *State) int {
		L.PushUserdata(future)
		return 1
	})
	L.SetGlobal("get_future")

	err := L.DoString(`
		local async = require("async")
		function task()
			local f = get_future()
			async.await(f)
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	sched.Spawn(L)

	err = sched.WaitAll(50 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// ---------------------------------------------------------------------------
// async library tests (Lua-level)
// ---------------------------------------------------------------------------

func TestAsync_GoString(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	err := L.DoString(`
		local async = require("async")
		result = nil
		function task()
			local f = async.go("return 42")
			result = async.await(f)
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	sched.Spawn(L)

	if err := sched.WaitAll(5 * time.Second); err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("result")
	val := L.ToAny(-1)
	L.Pop(1)
	// DoString returns int64 for integer values
	if val != int64(42) {
		t.Fatalf("expected 42, got %v (%T)", val, val)
	}
}

func TestAsync_GoStringError(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	err := L.DoString(`
		local async = require("async")
		result_val = nil
		result_err = nil
		function task()
			local f = async.go("error('boom')")
			local val, err = async.await(f)
			result_val = val
			result_err = err
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	sched.Spawn(L)

	if err := sched.WaitAll(5 * time.Second); err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("result_val")
	val := L.ToAny(-1)
	L.Pop(1)
	if val != nil {
		t.Fatalf("expected nil val, got %v", val)
	}

	L.GetGlobal("result_err")
	errVal := L.ToAny(-1)
	L.Pop(1)
	errStr, ok := errVal.(string)
	if !ok {
		t.Fatalf("expected string error, got %T: %v", errVal, errVal)
	}
	if len(errStr) == 0 {
		t.Fatal("expected non-empty error message")
	}
}

func TestAsync_Resolve(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	err := L.DoString(`
		local async = require("async")
		result = nil
		function task()
			local f = async.resolve("immediate")
			result = async.await(f)
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	sched.Spawn(L)

	// async.resolve creates an already-done future, so await returns immediately
	// The coroutine should complete without needing a Tick
	if sched.Pending() != 0 {
		t.Fatalf("expected 0 pending, got %d", sched.Pending())
	}

	L.GetGlobal("result")
	val := L.ToAny(-1)
	L.Pop(1)
	if val != "immediate" {
		t.Fatalf("expected 'immediate', got %v", val)
	}
}

func TestAsync_Reject(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	err := L.DoString(`
		local async = require("async")
		result_val = nil
		result_err = nil
		function task()
			local f = async.reject("nope")
			local val, err = async.await(f)
			result_val = val
			result_err = err
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	sched.Spawn(L)

	if sched.Pending() != 0 {
		t.Fatalf("expected 0 pending, got %d", sched.Pending())
	}

	L.GetGlobal("result_err")
	errVal := L.ToAny(-1)
	L.Pop(1)
	if errVal != "nope" {
		t.Fatalf("expected 'nope', got %v", errVal)
	}
}

func TestAsync_GoWithFunction(t *testing.T) {
	L := NewState()
	defer L.Close()

	// async.go with a function should error (can't move closures across goroutines)
	err := L.DoString(`
		local async = require("async")
		local ok, msg = pcall(function()
			async.go(function() return 1 end)
		end)
		assert(not ok, "expected error for function arg")
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Integration test: full async flow with goroutine
// ---------------------------------------------------------------------------

func TestAsync_FullFlow(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	// Register a Go function that does async work
	L.PushFunction(func(L *State) int {
		url := L.CheckString(1)
		future := NewFuture()
		go func() {
			// Simulate async work
			time.Sleep(5 * time.Millisecond)
			future.Resolve("response from " + url)
		}()
		L.PushUserdata(future)
		return 1
	})
	L.SetGlobal("http_get_async")

	err := L.DoString(`
		local async = require("async")
		results = {}
		function fetch_task()
			local f1 = http_get_async("https://example.com/a")
			local r1 = async.await(f1)
			results[1] = r1

			local f2 = http_get_async("https://example.com/b")
			local r2 = async.await(f2)
			results[2] = r2
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("fetch_task")
	sched.Spawn(L)

	if err := sched.WaitAll(5 * time.Second); err != nil {
		t.Fatal(err)
	}

	// Check results
	L.GetGlobal("results")
	L.GetField(-1, "1") // Lua arrays are 1-based but stored as fields
	// Actually, let's use raw get
	L.Pop(1) // pop the field attempt

	// Use Lua to extract
	L.Pop(1) // pop results table
	err = L.DoString(`
		assert(results[1] == "response from https://example.com/a",
			"got: " .. tostring(results[1]))
		assert(results[2] == "response from https://example.com/b",
			"got: " .. tostring(results[2]))
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAsync_ConcurrentGoroutines(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	// Register an async sleep function
	L.PushFunction(func(L *State) int {
		ms := L.CheckInteger(1)
		val := L.CheckString(2)
		future := NewFuture()
		go func() {
			time.Sleep(time.Duration(ms) * time.Millisecond)
			future.Resolve(val)
		}()
		L.PushUserdata(future)
		return 1
	})
	L.SetGlobal("async_sleep")

	err := L.DoString(`
		local async = require("async")
		r1 = nil
		r2 = nil
		function task1()
			local f = async_sleep(20, "slow")
			r1 = async.await(f)
		end
		function task2()
			local f = async_sleep(5, "fast")
			r2 = async.await(f)
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task1")
	sched.Spawn(L) //nolint:errcheck
	L.GetGlobal("task2")
	sched.Spawn(L) //nolint:errcheck

	if err := sched.WaitAll(5 * time.Second); err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("r1")
	v1 := L.ToAny(-1)
	L.Pop(1)
	L.GetGlobal("r2")
	v2 := L.ToAny(-1)
	L.Pop(1)

	if v1 != "slow" {
		t.Fatalf("expected 'slow', got %v", v1)
	}
	if v2 != "fast" {
		t.Fatalf("expected 'fast', got %v", v2)
	}
}

// ---------------------------------------------------------------------------
// New tests for async enhancements
// ---------------------------------------------------------------------------

func TestScheduler_OnError(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	var gotErr error
	sched.OnError = func(err error) {
		gotErr = err
	}

	future := NewFuture()
	L.PushFunction(func(L *State) int {
		L.PushUserdata(future)
		return 1
	})
	L.SetGlobal("get_future")

	err := L.DoString(`
		local async = require("async")
		function error_after_await()
			local f = get_future()
			async.await(f)
			error("boom in coroutine")
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("error_after_await")
	_, err = sched.Spawn(L)
	if err != nil {
		t.Fatal(err)
	}

	if sched.Pending() != 1 {
		t.Fatalf("expected 1 pending, got %d", sched.Pending())
	}

	// Resolve future — coroutine resumes and then errors
	future.Resolve("go")
	sched.Tick()

	if gotErr == nil {
		t.Fatal("expected OnError to be called")
	}
	if sched.Pending() != 0 {
		t.Fatalf("expected 0 pending after error, got %d", sched.Pending())
	}

	// Verify without OnError set, error is just swallowed (no panic)
	L2 := NewState()
	defer L2.Close()
	sched2 := NewScheduler(L2) // no OnError set

	f2 := NewFuture()
	L2.PushFunction(func(L *State) int {
		L.PushUserdata(f2)
		return 1
	})
	L2.SetGlobal("get_future")

	err = L2.DoString(`
		local async = require("async")
		function error_task()
			local f = get_future()
			async.await(f)
			error("should not panic")
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L2.GetGlobal("error_task")
	sched2.Spawn(L2) //nolint:errcheck

	f2.Resolve("x")
	sched2.Tick() // should not panic even without OnError
}

func TestFuture_Cancel(t *testing.T) {
	f := NewFuture()
	ch := f.Wait()

	f.Cancel()

	if !f.IsDone() {
		t.Fatal("expected done after cancel")
	}
	if !f.IsCancelled() {
		t.Fatal("expected IsCancelled")
	}
	val, err := f.Result()
	if val != nil {
		t.Fatalf("expected nil value, got %v", val)
	}
	if err != ErrCancelled {
		t.Fatalf("expected ErrCancelled, got %v", err)
	}

	// Channel should be closed
	select {
	case <-ch:
		// good
	default:
		t.Fatal("Wait channel should be closed after Cancel")
	}
}

func TestFuture_CancelAlreadyDone(t *testing.T) {
	f := NewFuture()
	f.Resolve("hello")

	f.Cancel() // should be no-op

	if f.IsCancelled() {
		t.Fatal("should not be cancelled after Resolve")
	}
	val, err := f.Result()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "hello" {
		t.Fatalf("expected 'hello', got %v", val)
	}
}

func TestScheduler_Cancel(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)
	future := NewFuture()

	L.PushFunction(func(L *State) int {
		L.PushUserdata(future)
		return 1
	})
	L.SetGlobal("get_future")

	err := L.DoString(`
		local async = require("async")
		function cancellable_task()
			local f = get_future()
			async.await(f)
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("cancellable_task")
	h, err := sched.Spawn(L)
	if err != nil {
		t.Fatal(err)
	}

	if sched.Pending() != 1 {
		t.Fatalf("expected 1 pending, got %d", sched.Pending())
	}

	// Cancel via handle
	sched.Cancel(h)

	if sched.Pending() != 0 {
		t.Fatalf("expected 0 pending after cancel, got %d", sched.Pending())
	}

	// Future should be cancelled
	if !future.IsCancelled() {
		t.Fatal("expected future to be cancelled")
	}
}

func TestScheduler_Destroy(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	f1 := NewFuture()
	f2 := NewFuture()

	L.PushFunction(func(L *State) int {
		L.PushUserdata(f1)
		return 1
	})
	L.SetGlobal("get_f1")

	L.PushFunction(func(L *State) int {
		L.PushUserdata(f2)
		return 1
	})
	L.SetGlobal("get_f2")

	err := L.DoString(`
		local async = require("async")
		function t1()
			async.await(get_f1())
		end
		function t2()
			async.await(get_f2())
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("t1")
	sched.Spawn(L) //nolint:errcheck
	L.GetGlobal("t2")
	sched.Spawn(L) //nolint:errcheck

	if sched.Pending() != 2 {
		t.Fatalf("expected 2 pending, got %d", sched.Pending())
	}

	sched.Destroy()

	if sched.Pending() != 0 {
		t.Fatalf("expected 0 pending after destroy, got %d", sched.Pending())
	}

	// Futures should be cancelled
	if !f1.IsCancelled() {
		t.Fatal("expected f1 cancelled")
	}
	if !f2.IsCancelled() {
		t.Fatal("expected f2 cancelled")
	}
}

func TestScheduler_SpawnReturnsHandle(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	f1 := NewFuture()
	f2 := NewFuture()

	L.PushFunction(func(L *State) int {
		L.PushUserdata(f1)
		return 1
	})
	L.SetGlobal("get_f1")

	L.PushFunction(func(L *State) int {
		L.PushUserdata(f2)
		return 1
	})
	L.SetGlobal("get_f2")

	err := L.DoString(`
		local async = require("async")
		function t1()
			async.await(get_f1())
		end
		function t2()
			async.await(get_f2())
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("t1")
	h1, err := sched.Spawn(L)
	if err != nil {
		t.Fatal(err)
	}
	L.GetGlobal("t2")
	h2, err := sched.Spawn(L)
	if err != nil {
		t.Fatal(err)
	}

	if h1 == nil {
		t.Fatal("expected non-nil handle h1")
	}
	if h2 == nil {
		t.Fatal("expected non-nil handle h2")
	}
	if h1.ID() == h2.ID() {
		t.Fatalf("expected unique IDs, got %d and %d", h1.ID(), h2.ID())
	}
	if h1.ID() <= 0 || h2.ID() <= 0 {
		t.Fatalf("expected positive IDs, got %d and %d", h1.ID(), h2.ID())
	}

	// Cleanup
	f1.Resolve("done")
	f2.Resolve("done")
	sched.WaitAll(time.Second)
}

func TestScheduler_CoroutineRequire(t *testing.T) {
	// Regression: NewThread must inherit GlobalSearcher so require() works in coroutines
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	err := L.DoString(`
		result = nil
		function task()
			local async = require("async")
			local f = async.resolve(42)
			result = async.await(f)
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	_, err = sched.Spawn(L)
	if err != nil {
		t.Fatal(err)
	}

	if sched.Pending() != 0 {
		sched.Tick()
	}

	L.GetGlobal("result")
	val := L.ToAny(-1)
	L.Pop(1)
	if val != int64(42) {
		t.Fatalf("expected 42, got %v (%T)", val, val)
	}
}

// ---------------------------------------------------------------------------
// Edge case: Spawn during Tick (Bug 1)
// ---------------------------------------------------------------------------

func TestScheduler_SpawnDuringTick(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	f1 := NewFuture()

	// Register a Go function that spawns a new coroutine when called
	L.PushFunction(func(L *State) int {
		// Define and push a function that sets a global
		err := L.DoString(`function spawned_task() spawned_result = "hello" end`)
		if err != nil {
			t.Fatal(err)
		}
		L.GetGlobal("spawned_task")
		_, err = sched.Spawn(L)
		if err != nil {
			t.Fatal(err)
		}
		return 0
	})
	L.SetGlobal("spawn_sibling")

	L.PushFunction(func(L *State) int {
		L.PushUserdata(f1)
		return 1
	})
	L.SetGlobal("get_future")

	err := L.DoString(`
		local async = require("async")
		function task()
			local f = get_future()
			async.await(f)
			spawn_sibling()  -- this calls Spawn during Tick
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	if _, err := sched.Spawn(L); err != nil {
		t.Fatal(err)
	}

	if sched.Pending() != 1 {
		t.Fatalf("expected 1 pending, got %d", sched.Pending())
	}

	// Resolve the future so Tick will resume the coroutine
	f1.Resolve("done")
	sched.Tick()

	// The spawned coroutine completed immediately (no yield), so
	// spawned_result should be set. The key assertion is that the
	// spawned coroutine was NOT lost.
	L.GetGlobal("spawned_result")
	val := L.ToAny(-1)
	L.Pop(1)
	if val != "hello" {
		t.Fatalf("expected 'hello' from spawned coroutine, got %v", val)
	}
}

func TestScheduler_SpawnDuringTick_Yielding(t *testing.T) {
	// Variant: spawned coroutine yields (has a future), so it must
	// survive in pending after the Tick swap.
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	f1 := NewFuture()
	f2 := NewFuture() // for the spawned coroutine

	L.PushFunction(func(L *State) int {
		L.PushUserdata(f2)
		return 1
	})
	L.SetGlobal("get_f2")

	// Register a Go function that spawns a yielding coroutine
	L.PushFunction(func(L *State) int {
		err := L.DoString(`
			function spawned_yielding()
				local async = require("async")
				spawned_yield_result = async.await(get_f2())
			end
		`)
		if err != nil {
			t.Fatal(err)
		}
		L.GetGlobal("spawned_yielding")
		_, err = sched.Spawn(L)
		if err != nil {
			t.Fatal(err)
		}
		return 0
	})
	L.SetGlobal("spawn_yielding_sibling")

	L.PushFunction(func(L *State) int {
		L.PushUserdata(f1)
		return 1
	})
	L.SetGlobal("get_f1")

	err := L.DoString(`
		local async = require("async")
		function task()
			local f = get_f1()
			async.await(f)
			spawn_yielding_sibling()
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("task")
	if _, err := sched.Spawn(L); err != nil {
		t.Fatal(err)
	}

	// Resolve f1 so Tick resumes task, which spawns the yielding sibling
	f1.Resolve("go")
	remaining := sched.Tick()

	// The spawned coroutine yielded on f2, so it must be in pending
	if remaining != 1 {
		t.Fatalf("expected 1 pending (spawned yielding coroutine), got %d", remaining)
	}

	// Now resolve f2
	f2.Resolve("spawned value")
	remaining = sched.Tick()

	if remaining != 0 {
		t.Fatalf("expected 0 pending, got %d", remaining)
	}

	L.GetGlobal("spawned_yield_result")
	val := L.ToAny(-1)
	L.Pop(1)
	if val != "spawned value" {
		t.Fatalf("expected 'spawned value', got %v", val)
	}
}

// ---------------------------------------------------------------------------
// Edge case: Destroy during Tick (Bug 2)
// ---------------------------------------------------------------------------

func TestScheduler_DestroyDuringTick(t *testing.T) {
	L := NewState()
	defer L.Close()

	sched := NewScheduler(L)

	f1 := NewFuture()
	f2 := NewFuture()

	// OnError calls Destroy — this is the scenario that caused double-Unref
	sched.OnError = func(err error) {
		sched.Destroy()
	}

	L.PushFunction(func(L *State) int {
		L.PushUserdata(f1)
		return 1
	})
	L.SetGlobal("get_f1")

	L.PushFunction(func(L *State) int {
		L.PushUserdata(f2)
		return 1
	})
	L.SetGlobal("get_f2")

	err := L.DoString(`
		local async = require("async")
		function good_task()
			async.await(get_f1())
		end
		function error_task()
			async.await(get_f2())
			error("boom in coroutine")
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Spawn both: error_task will error after resume, good_task is also pending
	L.GetGlobal("error_task")
	sched.Spawn(L) //nolint:errcheck
	L.GetGlobal("good_task")
	sched.Spawn(L) //nolint:errcheck

	if sched.Pending() != 2 {
		t.Fatalf("expected 2 pending, got %d", sched.Pending())
	}

	// Resolve both futures so both will be resumed in the same Tick
	f1.Resolve("ok")
	f2.Resolve("ok")

	// Tick should NOT panic even though OnError calls Destroy mid-iteration
	sched.Tick()

	if sched.Pending() != 0 {
		t.Fatalf("expected 0 pending after destroy, got %d", sched.Pending())
	}
}
