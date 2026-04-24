package lua

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestExecutor_SubmitCode(t *testing.T) {
	exec := NewExecutor(ExecutorConfig{})
	defer exec.Shutdown()

	ok := exec.Submit(Task{ID: "t1", Code: "x = 1 + 2"})
	if !ok {
		t.Fatal("Submit returned false")
	}

	select {
	case r := <-exec.Results():
		if r.ID != "t1" {
			t.Errorf("expected ID=t1, got %s", r.ID)
		}
		if r.Error != nil {
			t.Errorf("unexpected error: %v", r.Error)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestExecutor_SubmitFunc(t *testing.T) {
	exec := NewExecutor(ExecutorConfig{})
	defer exec.Shutdown()

	exec.Submit(Task{
		ID: "func1",
		Func: func(L *State) (any, error) {
			if err := L.DoString("x = 42"); err != nil {
				return nil, err
			}
			L.GetGlobal("x")
			v, _ := L.ToInteger(-1)
			L.Pop(1)
			return v, nil
		},
	})

	select {
	case r := <-exec.Results():
		if r.ID != "func1" {
			t.Errorf("expected ID=func1, got %s", r.ID)
		}
		if r.Error != nil {
			t.Fatalf("unexpected error: %v", r.Error)
		}
		v, ok := r.Value.(int64)
		if !ok || v != 42 {
			t.Errorf("expected value=42, got %v", r.Value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestExecutor_Concurrent(t *testing.T) {
	exec := NewExecutor(ExecutorConfig{
		PoolConfig:   PoolConfig{MaxStates: 4},
		ResultBuffer: 20,
	})
	defer exec.Shutdown()

	const n = 10
	for i := 0; i < n; i++ {
		exec.Submit(Task{
			ID:   fmt.Sprintf("task-%d", i),
			Code: fmt.Sprintf("x = %d * 2", i),
		})
	}

	seen := make(map[string]bool)
	for i := 0; i < n; i++ {
		select {
		case r := <-exec.Results():
			if r.Error != nil {
				t.Errorf("task %s failed: %v", r.ID, r.Error)
			}
			seen[r.ID] = true
		case <-time.After(10 * time.Second):
			t.Fatalf("timeout: got %d/%d results", i, n)
		}
	}

	for i := 0; i < n; i++ {
		id := fmt.Sprintf("task-%d", i)
		if !seen[id] {
			t.Errorf("missing result for %s", id)
		}
	}
}

func TestExecutor_Error(t *testing.T) {
	exec := NewExecutor(ExecutorConfig{})
	defer exec.Shutdown()

	exec.Submit(Task{
		ID:   "bad",
		Code: "this is not valid lua!!!",
	})

	select {
	case r := <-exec.Results():
		if r.ID != "bad" {
			t.Errorf("expected ID=bad, got %s", r.ID)
		}
		if r.Error == nil {
			t.Error("expected error for invalid Lua code")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for error result")
	}
}

func TestExecutor_Pending(t *testing.T) {
	exec := NewExecutor(ExecutorConfig{
		ResultBuffer: 10,
	})
	defer exec.Shutdown()

	// Submit a slow task.
	exec.Submit(Task{
		ID: "slow",
		Func: func(L *State) (any, error) {
			time.Sleep(200 * time.Millisecond)
			return nil, nil
		},
	})

	// Give goroutine time to start.
	time.Sleep(50 * time.Millisecond)
	p := exec.Pending()
	if p != 1 {
		t.Errorf("expected Pending=1, got %d", p)
	}

	// Wait for completion.
	<-exec.Results()
	time.Sleep(50 * time.Millisecond)
	p = exec.Pending()
	if p != 0 {
		t.Errorf("expected Pending=0, got %d", p)
	}
}

func TestExecutor_Shutdown(t *testing.T) {
	exec := NewExecutor(ExecutorConfig{
		ResultBuffer: 10,
	})

	// Submit tasks that take a bit.
	for i := 0; i < 3; i++ {
		exec.Submit(Task{
			ID: fmt.Sprintf("s-%d", i),
			Func: func(L *State) (any, error) {
				time.Sleep(100 * time.Millisecond)
				return nil, nil
			},
		})
	}

	// Drain results in background so Shutdown doesn't block on full channel.
	done := make(chan struct{})
	go func() {
		for range exec.Results() {
		}
		close(done)
	}()

	// Shutdown should wait for all tasks.
	exec.Shutdown()

	// Results channel should be closed after Shutdown.
	select {
	case <-done:
		// good
	case <-time.After(5 * time.Second):
		t.Fatal("Results channel not closed after Shutdown")
	}
}

func TestExecutor_NonBlocking(t *testing.T) {
	exec := NewExecutor(ExecutorConfig{
		ResultBuffer: 10,
	})
	defer exec.Shutdown()

	start := time.Now()
	exec.Submit(Task{
		ID: "slow",
		Func: func(L *State) (any, error) {
			time.Sleep(500 * time.Millisecond)
			return nil, nil
		},
	})
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("Submit blocked for %v (expected non-blocking)", elapsed)
	}

	// Drain result.
	<-exec.Results()
}

func TestExecutor_WithModules(t *testing.T) {
	exec := NewExecutor(ExecutorConfig{
		PoolConfig: PoolConfig{
			MaxStates: 2,
			InitFunc: func(L *State) {
				// Register a simple module as a global.
				L.PushInteger(99)
				L.SetGlobal("MAGIC")
			},
		},
		ResultBuffer: 10,
	})
	defer exec.Shutdown()

	exec.Submit(Task{
		ID: "mod",
		Func: func(L *State) (any, error) {
			L.GetGlobal("MAGIC")
			v, ok := L.ToInteger(-1)
			L.Pop(1)
			if !ok || v != 99 {
				return nil, fmt.Errorf("expected MAGIC=99, got %d (ok=%v)", v, ok)
			}
			return v, nil
		},
	})

	select {
	case r := <-exec.Results():
		if r.Error != nil {
			t.Fatalf("unexpected error: %v", r.Error)
		}
		if r.Value.(int64) != 99 {
			t.Errorf("expected 99, got %v", r.Value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestExecutor_LongRunning(t *testing.T) {
	exec := NewExecutor(ExecutorConfig{
		PoolConfig:   PoolConfig{MaxStates: 4},
		ResultBuffer: 10,
	})
	defer exec.Shutdown()

	// Submit a slow task and a fast task.
	exec.Submit(Task{
		ID: "slow",
		Func: func(L *State) (any, error) {
			time.Sleep(300 * time.Millisecond)
			return "slow-done", nil
		},
	})
	exec.Submit(Task{
		ID:   "fast",
		Code: "x = 1",
	})

	// Fast task should complete before slow task.
	results := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case r := <-exec.Results():
			results[r.ID] = true
			if r.Error != nil {
				t.Errorf("task %s failed: %v", r.ID, r.Error)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout")
		}
	}

	if !results["slow"] || !results["fast"] {
		t.Errorf("missing results: %v", results)
	}
}

func TestExecutor_RaceDetector(t *testing.T) {
	exec := NewExecutor(ExecutorConfig{
		PoolConfig:   PoolConfig{MaxStates: 2},
		ResultBuffer: 20,
	})

	var wg sync.WaitGroup
	const n = 8

	// Multiple goroutines submitting concurrently.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			exec.Submit(Task{
				ID:   fmt.Sprintf("race-%d", id),
				Code: fmt.Sprintf("x = %d", id),
			})
		}(i)
	}

	// Drain results concurrently.
	received := make(chan struct{})
	go func() {
		count := 0
		for range exec.Results() {
			count++
			if count >= n {
				break
			}
		}
		close(received)
	}()

	wg.Wait()

	select {
	case <-received:
		// All results received.
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for all results")
	}

	exec.Shutdown()
}

func TestExecutor_SubmitAfterShutdown(t *testing.T) {
	exec := NewExecutor(ExecutorConfig{ResultBuffer: 10})

	// Drain results so Shutdown doesn't block.
	go func() {
		for range exec.Results() {
		}
	}()

	exec.Shutdown()

	ok := exec.Submit(Task{ID: "late", Code: "x = 1"})
	if ok {
		t.Error("Submit should return false after Shutdown")
	}
}
