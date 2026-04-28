package lua

import (
	"testing"
	"time"
)

func TestTimerDelay(t *testing.T) {
	L := NewState()
	defer L.Close()

	start := time.Now()
	err := L.DoString(`
		local timer = require("timer")
		local async = require("async")
		local f = timer.delay(0.05) -- 50ms
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// The future should be on the stack as a userdata from DoString
	// Actually DoString pops results. Let's use a different approach:
	// use the scheduler to test delay with await.
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Fatalf("delay setup took too long: %v", elapsed)
	}
}

func TestTimerDelayWithScheduler(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Set up a global variable to track completion
	err := L.DoString(`
		local timer = require("timer")
		local async = require("async")
		done = false
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Create a coroutine that awaits a delay
	err = L.DoString(`
		function delayTest()
			local timer = require("timer")
			local async = require("async")
			local f = timer.delay(0.05)
			async.await(f)
			done = true
		end
	`)
	if err != nil {
		t.Fatalf("function setup failed: %v", err)
	}

	sched := NewScheduler(L)

	// Push the function and spawn it
	L.GetGlobal("delayTest")
	_, err = sched.Spawn(L)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	// Wait for the coroutine to complete
	err = sched.WaitAll(2 * time.Second)
	if err != nil {
		t.Fatalf("WaitAll failed: %v", err)
	}

	// Check that done is true
	L.GetGlobal("done")
	if !L.ToBoolean(-1) {
		t.Fatal("expected done=true after delay")
	}
	L.Pop(1)
}

func TestTimerAfter(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local timer = require("timer")
		counter = 0
		timer.after(0.01, function() counter = counter + 1 end)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	tm := GetTimerManager(L)

	// Before tick, counter should be 0
	L.GetGlobal("counter")
	n, _ := L.ToInteger(-1)
	L.Pop(1)
	if n != 0 {
		t.Fatalf("expected counter=0 before tick, got %d", n)
	}

	// Wait for timer to expire
	time.Sleep(50 * time.Millisecond)

	// Tick should fire the callback
	tm.Tick(L)

	L.GetGlobal("counter")
	n, _ = L.ToInteger(-1)
	L.Pop(1)
	if n != 1 {
		t.Fatalf("expected counter=1 after tick, got %d", n)
	}

	// Second tick should NOT fire again (one-shot)
	time.Sleep(50 * time.Millisecond)
	tm.Tick(L)

	L.GetGlobal("counter")
	n, _ = L.ToInteger(-1)
	L.Pop(1)
	if n != 1 {
		t.Fatalf("expected counter=1 after second tick (one-shot), got %d", n)
	}
}

func TestTimerEvery(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local timer = require("timer")
		counter = 0
		timer_id = timer.every(0.01, function() counter = counter + 1 end)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	tm := GetTimerManager(L)

	// Wait and tick multiple times
	time.Sleep(30 * time.Millisecond)
	tm.Tick(L)

	L.GetGlobal("counter")
	n1, _ := L.ToInteger(-1)
	L.Pop(1)
	if n1 < 1 {
		t.Fatalf("expected counter >= 1 after first tick, got %d", n1)
	}

	// Wait again and tick — should fire again (repeating)
	time.Sleep(30 * time.Millisecond)
	tm.Tick(L)

	L.GetGlobal("counter")
	n2, _ := L.ToInteger(-1)
	L.Pop(1)
	if n2 <= n1 {
		t.Fatalf("expected counter to increase after second tick, got %d (was %d)", n2, n1)
	}
}

func TestTimerCancel(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local timer = require("timer")
		counter = 0
		timer_id = timer.after(0.01, function() counter = counter + 1 end)
		timer.cancel(timer_id)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	tm := GetTimerManager(L)

	// Wait and tick — should NOT fire (canceled)
	time.Sleep(50 * time.Millisecond)
	tm.Tick(L)

	L.GetGlobal("counter")
	n, _ := L.ToInteger(-1)
	L.Pop(1)
	if n != 0 {
		t.Fatalf("expected counter=0 after canceled timer tick, got %d", n)
	}
}

func TestTimerCancelEvery(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local timer = require("timer")
		counter = 0
		timer_id = timer.every(0.01, function() counter = counter + 1 end)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	tm := GetTimerManager(L)

	// Let it fire once
	time.Sleep(30 * time.Millisecond)
	tm.Tick(L)

	L.GetGlobal("counter")
	n1, _ := L.ToInteger(-1)
	L.Pop(1)
	if n1 < 1 {
		t.Fatalf("expected counter >= 1 before cancel, got %d", n1)
	}

	// Cancel it
	err = L.DoString(`require("timer").cancel(timer_id)`)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	// Wait and tick — should NOT fire again
	time.Sleep(30 * time.Millisecond)
	tm.Tick(L)

	L.GetGlobal("counter")
	n2, _ := L.ToInteger(-1)
	L.Pop(1)
	if n2 != n1 {
		t.Fatalf("expected counter=%d after cancel, got %d", n1, n2)
	}
}

func TestTimerManagerPending(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local timer = require("timer")
		timer.after(0.5, function() end)
		timer.after(0.5, function() end)
		timer.every(0.5, function() end)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	tm := GetTimerManager(L)
	if tm.Pending() != 3 {
		t.Fatalf("expected 3 pending timers, got %d", tm.Pending())
	}
}

func TestTimerDelayNegative(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local timer = require("timer")
		timer.delay(-1)
	`)
	if err == nil {
		t.Fatal("expected error for negative delay")
	}
}

func TestTimerEveryZero(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local timer = require("timer")
		timer.every(0, function() end)
	`)
	if err == nil {
		t.Fatal("expected error for zero interval")
	}
}
