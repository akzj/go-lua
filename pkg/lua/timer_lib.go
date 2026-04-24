package lua

import (
	"sync"
	"sync/atomic"
	"time"
)

func init() {
	RegisterGlobal("timer", OpenTimer)
}

// ---------------------------------------------------------------------------
// Timer module — delay, after, every, cancel
// ---------------------------------------------------------------------------

// Timer represents a scheduled timer managed by TimerManager.
type Timer struct {
	ID       int64
	Interval time.Duration
	Repeat   bool
	Callback int // Lua registry reference to the callback function
	NextFire time.Time
	Canceled bool
}

// TimerManager manages all active timers for a Lua State.
// NOT thread-safe for Tick (must be called from the same goroutine as Lua).
// The mutex protects cancel operations which may come from any goroutine.
type TimerManager struct {
	mu     sync.Mutex
	timers map[int64]*Timer
	nextID atomic.Int64
}

// NewTimerManager creates a new TimerManager.
func NewTimerManager() *TimerManager {
	return &TimerManager{
		timers: make(map[int64]*Timer),
	}
}

// Add registers a timer and returns its ID.
func (tm *TimerManager) Add(t *Timer) int64 {
	id := tm.nextID.Add(1)
	t.ID = id
	tm.mu.Lock()
	tm.timers[id] = t
	tm.mu.Unlock()
	return id
}

// Cancel marks a timer as canceled.
func (tm *TimerManager) Cancel(id int64) {
	tm.mu.Lock()
	if t, ok := tm.timers[id]; ok {
		t.Canceled = true
	}
	tm.mu.Unlock()
}

// Tick checks all timers and fires expired ones.
// Calls the Lua callback for each expired timer via PCall.
// Must be called from the same goroutine that owns the Lua State.
// Returns the number of remaining active timers.
func (tm *TimerManager) Tick(L *State) int {
	tm.mu.Lock()
	now := time.Now()
	// Collect timers to fire (avoid calling Lua while holding lock)
	type fireInfo struct {
		id       int64
		callback int
		repeat   bool
		interval time.Duration
	}
	var toFire []fireInfo
	for id, t := range tm.timers {
		if t.Canceled {
			L.Unref(RegistryIndex, t.Callback)
			delete(tm.timers, id)
			continue
		}
		if now.After(t.NextFire) || now.Equal(t.NextFire) {
			toFire = append(toFire, fireInfo{
				id:       id,
				callback: t.Callback,
				repeat:   t.Repeat,
				interval: t.Interval,
			})
			if t.Repeat {
				t.NextFire = now.Add(t.Interval)
			} else {
				delete(tm.timers, id)
			}
		}
	}
	remaining := len(tm.timers)
	tm.mu.Unlock()

	// Fire callbacks outside the lock
	for _, fi := range toFire {
		L.RawGetI(RegistryIndex, int64(fi.callback))
		L.PCall(0, 0, 0)
		if !fi.repeat {
			L.Unref(RegistryIndex, fi.callback)
		}
	}

	return remaining
}

// Pending returns the number of active (non-canceled) timers.
func (tm *TimerManager) Pending() int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	count := 0
	for _, t := range tm.timers {
		if !t.Canceled {
			count++
		}
	}
	return count
}

const timerManagerKey = "__timer_manager__"

// getTimerManager retrieves or creates the TimerManager for the given State.
// Stored via UserValue on the internal api.State so it survives wrapFunction.
func getTimerManager(L *State) *TimerManager {
	v := L.UserValue(timerManagerKey)
	if tm, ok := v.(*TimerManager); ok {
		return tm
	}
	tm := NewTimerManager()
	L.SetUserValue(timerManagerKey, tm)
	return tm
}

// GetTimerManager returns the TimerManager for a State, creating one if needed.
// This is the public API for host applications that need to call Tick() in
// their event loop.
func GetTimerManager(L *State) *TimerManager {
	return getTimerManager(L)
}

// OpenTimer opens the "timer" module and pushes it onto the stack.
// Registered globally via init(), so `require("timer")` works automatically.
//
// Lua API:
//
//	local timer = require("timer")
//	local f = timer.delay(0.5)         -- returns Future, resolves after 500ms
//	local id = timer.after(1.0, fn)    -- calls fn after 1 second
//	local id = timer.every(0.1, fn)    -- calls fn every 100ms
//	timer.cancel(id)                   -- cancel a timer
func OpenTimer(L *State) {
	L.NewLib(map[string]Function{
		"delay":  timerDelay,
		"after":  timerAfter,
		"every":  timerEvery,
		"cancel": timerCancel,
	})
}

// timerDelay implements timer.delay(seconds) -> Future.
// Creates a Future that resolves (with nil) after the given delay.
// The delay runs in a goroutine — use async.await(f) to yield until done.
//
// Lua: local f = timer.delay(0.5)
func timerDelay(L *State) int {
	seconds := L.CheckNumber(1)
	if seconds < 0 {
		L.ArgError(1, "delay must be non-negative")
		return 0
	}
	dur := time.Duration(seconds * float64(time.Second))
	future := NewFuture()
	go func() {
		time.Sleep(dur)
		future.Resolve(nil)
	}()
	L.PushUserdata(future)
	return 1
}

// timerAfter implements timer.after(seconds, callback) -> timerID.
// Registers a one-shot timer. The callback fires when TimerManager.Tick()
// detects that the timer has expired.
//
// Lua: local id = timer.after(1.0, function() print("done") end)
func timerAfter(L *State) int {
	seconds := L.CheckNumber(1)
	L.CheckType(2, TypeFunction)
	if seconds < 0 {
		L.ArgError(1, "delay must be non-negative")
		return 0
	}
	dur := time.Duration(seconds * float64(time.Second))

	// Store callback in registry
	L.PushValue(2)
	ref := L.Ref(RegistryIndex)

	tm := getTimerManager(L)
	id := tm.Add(&Timer{
		Interval: dur,
		Repeat:   false,
		Callback: ref,
		NextFire: time.Now().Add(dur),
	})

	L.PushInteger(id)
	return 1
}

// timerEvery implements timer.every(seconds, callback) -> timerID.
// Registers a repeating timer. The callback fires repeatedly when
// TimerManager.Tick() detects that the interval has elapsed.
//
// Lua: local id = timer.every(0.1, function() print("tick") end)
func timerEvery(L *State) int {
	seconds := L.CheckNumber(1)
	L.CheckType(2, TypeFunction)
	if seconds <= 0 {
		L.ArgError(1, "interval must be positive")
		return 0
	}
	dur := time.Duration(seconds * float64(time.Second))

	// Store callback in registry
	L.PushValue(2)
	ref := L.Ref(RegistryIndex)

	tm := getTimerManager(L)
	id := tm.Add(&Timer{
		Interval: dur,
		Repeat:   true,
		Callback: ref,
		NextFire: time.Now().Add(dur),
	})

	L.PushInteger(id)
	return 1
}

// timerCancel implements timer.cancel(timerID).
// Cancels a previously registered timer (one-shot or repeating).
//
// Lua: timer.cancel(id)
func timerCancel(L *State) int {
	id := L.CheckInteger(1)
	tm := getTimerManager(L)
	tm.Cancel(id)
	return 0
}
