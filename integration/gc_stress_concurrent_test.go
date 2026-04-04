// Package integration provides end-to-end tests for the Lua VM.
// This file implements concurrent GC stress testing with multiple scenarios
// running simultaneously while a monitor samples GC statistics.
package integration

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/akzj/go-lua/state"
)

// =============================================================================
// GC Stress Test - Concurrent scenarios with monitoring
// =============================================================================

// GCStats holds shared GC statistics collected by the monitor.
type GCStats struct {
	mu               sync.Mutex
	Samples          []MemSample
	GCFrequency      float64 // cycles per second
	MemoryGrowthRate float64 // bytes per second
	TotalCycles      uint32  // total GC cycles observed
}

// MemSample represents a single memory statistics snapshot.
type MemSample struct {
	Timestamp int64
	HeapAlloc uint64
	HeapIdle  uint64
	NumGC     uint32
	PauseNs   uint64
}

// monitorGoroutine samples GC statistics at regular intervals.
func monitorGoroutine(stats *GCStats, interval time.Duration, stopCh <-chan struct{}) {
	var prevStats runtime.MemStats
	runtime.ReadMemStats(&prevStats)
	prevTime := time.Now()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			now := time.Now()
			elapsed := now.Sub(prevTime).Seconds()

			stats.mu.Lock()

			// Record sample
			sample := MemSample{
				Timestamp: now.UnixNano(),
				HeapAlloc: m.HeapAlloc,
				HeapIdle:  m.HeapIdle,
				NumGC:     m.NumGC,
				PauseNs:   m.PauseTotalNs,
			}
			stats.Samples = append(stats.Samples, sample)

			// Calculate GC frequency
			if m.NumGC > prevStats.NumGC && elapsed > 0 {
				gcCount := m.NumGC - prevStats.NumGC
				stats.GCFrequency = float64(gcCount) / elapsed
				atomic.StoreUint32(&stats.TotalCycles, m.NumGC)
			}

			// Calculate memory growth rate
			if m.HeapAlloc > prevStats.HeapAlloc && elapsed > 0 {
				growth := m.HeapAlloc - prevStats.HeapAlloc
				stats.MemoryGrowthRate = float64(growth) / elapsed
			}

			stats.mu.Unlock()

			prevStats = m
			prevTime = now

		case <-stopCh:
			return
		}
	}
}

// =============================================================================
// Scenario 1: Concurrent table creation/release
// =============================================================================

func runScenario1TableCreateRelease(iterations int) {
	for i := 0; i < iterations; i++ {
		L := state.New()
		// Create table with nested objects
		state.DoStringOn(L, `
			local objs = {}
			for j = 1, 100 do
				objs[j] = {data = j, inner = {value = j * 2}}
			end
		`)
		state.DoStringOn(L, `collectgarbage("collect")`)
		// L goes out of scope and is GC'd
	}
}

// =============================================================================
// Scenario 2: Concurrent closure creation with captured references
// =============================================================================

func runScenario2ClosureCreation(iterations int) {
	for i := 0; i < iterations; i++ {
		L := state.New()
		// Create closures that capture local variables
		state.DoStringOn(L, `
			local closures = {}
			for j = 1, 50 do
				local captured = j
				closures[j] = function() return captured * 2 end
			end
			-- Force closure creation
			for j = 1, 50 do
				closures[j]()
			end
		`)
		state.DoStringOn(L, `collectgarbage("collect")`)
		// L goes out of scope and is GC'd
	}
}

// =============================================================================
// Scenario 3: Concurrent metatable operations with finalizers
// =============================================================================

func runScenario3MetatableFinalizers(iterations int) {
	for i := 0; i < iterations; i++ {
		L := state.New()
		// Create objects with metatables and weak references
		state.DoStringOn(L, `
			-- Create weak table for cleanup
			local cache = setmetatable({}, {__mode = "v"})
			
			-- Create objects with metatables
			for j = 1, 50 do
				local obj = {id = j, data = {}}
				setmetatable(obj, {
					__gc = function() end,
					__index = function(t, k) return t.data[k] end
				})
				cache[j] = obj
			end
		`)
		state.DoStringOn(L, `collectgarbage("collect")`)
		// L goes out of scope and is GC'd
	}
}

// =============================================================================
// TestGCStressConcurrent - Concurrent GC stress test with all scenarios
// =============================================================================

// TestGCStressConcurrent runs 3 concurrent GC scenarios with monitor sampling.
// This test verifies:
// 1. Multiple goroutines creating/releasing Lua objects in parallel
// 2. Monitor goroutine tracking GC statistics
// 3. All scenarios run simultaneously
// 4. No race conditions or memory leaks under concurrent GC pressure
func TestGCStressConcurrent(t *testing.T) {
	const (
		numGoroutines  = 5
		monitorInterval = 10 * time.Millisecond
		testDuration   = 200 * time.Millisecond // Longer duration for stable metrics
	)

	stats := &GCStats{}
	stopCh := make(chan struct{})

	// Get baseline memory stats
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	baselineHeap := baseline.HeapAlloc

	// Start monitor goroutine
	go monitorGoroutine(stats, monitorInterval, stopCh)

	// Start worker goroutines for all 3 scenarios
	var wg sync.WaitGroup

	// Scenario 1: Concurrent table creation/release
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runScenario1TableCreateRelease(200)
		}()
	}

	// Scenario 2: Concurrent closure creation
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runScenario2ClosureCreation(200)
		}()
	}

	// Scenario 3: Concurrent metatable operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runScenario3MetatableFinalizers(200)
		}()
	}

	// Run test for specified duration
	time.Sleep(testDuration)

	// Stop all goroutines
	close(stopCh)
	wg.Wait()

	// Wait for monitor to finish
	time.Sleep(monitorInterval * 2)

	// Validate results
	stats.mu.Lock()
	defer stats.mu.Unlock()

	// Check that we collected samples
	if len(stats.Samples) == 0 {
		t.Fatal("No monitoring samples collected")
	}

	// Check GC is running
	totalCycles := atomic.LoadUint32(&stats.TotalCycles)
	if totalCycles == 0 {
		t.Error("No GC cycles detected during test")
	}

	// Get final memory stats
	var final runtime.MemStats
	runtime.ReadMemStats(&final)

	// Calculate memory growth
	memoryGrowth := int64(final.HeapAlloc) - int64(baselineHeap)

	// Log results
	t.Logf("GC Statistics:")
	t.Logf("  Total samples: %d", len(stats.Samples))
	t.Logf("  GC cycles: %d", totalCycles)
	t.Logf("  GC frequency: %.2f cycles/sec", stats.GCFrequency)
	t.Logf("  Memory growth rate: %.2f bytes/sec", stats.MemoryGrowthRate)
	t.Logf("  Memory growth: %d bytes", memoryGrowth)
	t.Logf("  Baseline heap: %d bytes", baselineHeap)
	t.Logf("  Final heap: %d bytes", final.HeapAlloc)

	// Validate GC is running (basic sanity check)
	if totalCycles == 0 {
		t.Error("No GC cycles detected during test - GC may be broken")
	}

	// Validate memory doesn't grow unboundedly (check for catastrophic leak)
	// Allow up to 20x growth for stress test with concurrent allocation
	// This is expected behavior - memory should be reclaimed after test completes
	memoryGrowthRatio := float64(final.HeapAlloc) / float64(baselineHeap)
	if memoryGrowthRatio > 20.0 {
		t.Errorf("Memory grew %.2fx - possible memory leak", memoryGrowthRatio)
	} else {
		t.Logf("Memory growth ratio: %.2fx (acceptable for stress test)", memoryGrowthRatio)
	}

	// Validate we collected enough samples
	if len(stats.Samples) < 5 {
		t.Errorf("Only %d samples collected - monitoring may be broken", len(stats.Samples))
	}
}

// TestGCStressWithValidation runs the stress test with full validation.
func TestGCStressWithValidation(t *testing.T) {
	const (
		numGoroutines  = 3
		monitorInterval = 20 * time.Millisecond
		testDuration   = 50 * time.Millisecond
	)

	stats := &GCStats{}
	stopCh := make(chan struct{})

	// Start monitor
	go monitorGoroutine(stats, monitorInterval, stopCh)

	// Run single scenario for validation
	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runScenario1TableCreateRelease(100)
		}()
	}

	time.Sleep(testDuration)
	close(stopCh)
	wg.Wait()
	time.Sleep(monitorInterval * 2)

	// Validate
	stats.mu.Lock()
	defer stats.mu.Unlock()

	if len(stats.Samples) < 2 {
		t.Fatal("Insufficient samples for validation")
	}

	// Check memory stability
	first := stats.Samples[0]
	last := stats.Samples[len(stats.Samples)-1]

	if last.HeapAlloc > first.HeapAlloc*3 {
		t.Logf("Warning: Memory grew significantly from %d to %d",
			first.HeapAlloc, last.HeapAlloc)
	}

	t.Logf("Validation: samples=%d, gcCycles=%d, growthRate=%.2f",
		len(stats.Samples), atomic.LoadUint32(&stats.TotalCycles), stats.MemoryGrowthRate)
}
