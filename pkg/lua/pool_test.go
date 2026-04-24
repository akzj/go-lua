package lua

import (
	"sync"
	"testing"
)

func TestStatePool_GetPut(t *testing.T) {
	pool := NewStatePool(PoolConfig{MaxStates: 4})
	defer pool.Close()

	// Get a State, use it, put it back.
	L := pool.Get()
	if L == nil {
		t.Fatal("Get returned nil")
	}
	if err := L.DoString("x = 1 + 2"); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	pool.Put(L)

	// Get again — should be the same State (LIFO).
	L2 := pool.Get()
	if L2 != L {
		t.Error("expected same State from pool (LIFO reuse)")
	}
	pool.Put(L2)
}

func TestStatePool_MaxStates(t *testing.T) {
	pool := NewStatePool(PoolConfig{MaxStates: 2})
	defer pool.Close()

	// Create 3 States.
	s1 := pool.Get()
	s2 := pool.Get()
	s3 := pool.Get()

	// Put all 3 back — only 2 should be kept.
	pool.Put(s1)
	pool.Put(s2)
	pool.Put(s3) // this one should be closed

	stats := pool.Stats()
	if stats.Available != 2 {
		t.Errorf("expected 2 available, got %d", stats.Available)
	}
}

func TestStatePool_InitFunc(t *testing.T) {
	initCalled := 0
	pool := NewStatePool(PoolConfig{
		MaxStates: 2,
		InitFunc: func(L *State) {
			initCalled++
			// Set a global so we can verify later.
			L.PushInteger(42)
			L.SetGlobal("poolInit")
		},
	})
	defer pool.Close()

	L := pool.Get()
	if initCalled != 1 {
		t.Errorf("expected InitFunc called once, got %d", initCalled)
	}

	// Verify the global was set.
	L.GetGlobal("poolInit")
	v, ok := L.ToInteger(-1)
	L.Pop(1)
	if !ok || v != 42 {
		t.Errorf("expected poolInit=42, got %d (ok=%v)", v, ok)
	}
	pool.Put(L)

	// Get the same State back — InitFunc should NOT be called again.
	L2 := pool.Get()
	_ = L2
	if initCalled != 1 {
		t.Errorf("expected InitFunc still 1 (reused State), got %d", initCalled)
	}
	pool.Put(L2)

	// Drain pool and get a fresh one.
	all := pool.Get()
	pool.Put(all)
	// Get both out so pool is empty, then get a new one.
	a := pool.Get()
	b := pool.Get() // forces new creation
	if initCalled != 2 {
		t.Errorf("expected InitFunc called twice (new State), got %d", initCalled)
	}
	pool.Put(a)
	pool.Put(b)
}

func TestStatePool_Sandbox(t *testing.T) {
	cfg := SandboxConfig{CPULimit: 100_000}
	pool := NewStatePool(PoolConfig{
		MaxStates: 2,
		Sandbox:   &cfg,
	})
	defer pool.Close()

	L := pool.Get()
	// Sandboxed State should not have 'dofile'.
	L.GetGlobal("dofile")
	if !L.IsNil(-1) {
		t.Error("sandboxed State should not have 'dofile'")
	}
	L.Pop(1)
	pool.Put(L)
}

func TestStatePool_Concurrent(t *testing.T) {
	pool := NewStatePool(PoolConfig{MaxStates: 4})
	defer pool.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			L := pool.Get()
			_ = L.DoString("x = 1 + 1")
			pool.Put(L)
		}()
	}
	wg.Wait()

	stats := pool.Stats()
	if stats.Available > 4 {
		t.Errorf("pool exceeded MaxStates: available=%d", stats.Available)
	}
}

func TestStatePool_Close(t *testing.T) {
	pool := NewStatePool(PoolConfig{MaxStates: 4})

	// Put some States in.
	L1 := pool.Get()
	L2 := pool.Get()
	pool.Put(L1)
	pool.Put(L2)

	pool.Close()

	stats := pool.Stats()
	if stats.Available != 0 {
		t.Errorf("expected 0 available after Close, got %d", stats.Available)
	}

	// Put after Close should close the State (not panic).
	L3 := pool.Get()
	pool.Put(L3) // should not panic
}

func TestStatePool_Stats(t *testing.T) {
	pool := NewStatePool(PoolConfig{MaxStates: 4})
	defer pool.Close()

	stats := pool.Stats()
	if stats.MaxStates != 4 {
		t.Errorf("expected MaxStates=4, got %d", stats.MaxStates)
	}
	if stats.Created != 0 {
		t.Errorf("expected Created=0, got %d", stats.Created)
	}

	L := pool.Get()
	stats = pool.Stats()
	if stats.Created != 1 {
		t.Errorf("expected Created=1, got %d", stats.Created)
	}
	if stats.Available != 0 {
		t.Errorf("expected Available=0, got %d", stats.Available)
	}

	pool.Put(L)
	stats = pool.Stats()
	if stats.Available != 1 {
		t.Errorf("expected Available=1, got %d", stats.Available)
	}
}

func TestStatePool_DefaultMaxStates(t *testing.T) {
	pool := NewStatePool(PoolConfig{}) // no MaxStates set
	defer pool.Close()

	stats := pool.Stats()
	if stats.MaxStates != 8 {
		t.Errorf("expected default MaxStates=8, got %d", stats.MaxStates)
	}
}
