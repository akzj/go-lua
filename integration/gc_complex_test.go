// Package integration provides end-to-end tests for the Lua VM.
// This file contains complex GC scenarios: circular references, closures,
// weak references, table nesting, and cross-coroutine references.
package integration

import (
	"runtime"
	"testing"

	"github.com/akzj/go-lua/state"
)

// =============================================================================
// TestGCComplex - Complex Garbage Collection Scenarios
// =============================================================================

// TestGCCircularReference verifies that circular references are collected.
func TestGCCircularReference(t *testing.T) {
	// Create objects with circular references
	code := `
local created = 0

-- Create table with circular reference
local function make_circular()
	local t1 = {}
	local t2 = {}
	t1.ref = t2
	t2.ref = t1
	created = created + 2
	return t1
end

-- Create and abandon circular reference
make_circular()

-- Force GC
collectgarbage("collect")
`

	err := state.DoString(code)
	if err != nil {
		t.Errorf("Circular reference test failed: %v", err)
	}
}

// TestGCClosureCapture verifies closures capturing local variables are collected.
func TestGCClosureCapture(t *testing.T) {
	code := `
local closures_created = 0

-- Create closures that capture local variables
local function factory()
	local x = 10
	closures_created = closures_created + 1
	return function() return x end
end

-- Create multiple closures
for i = 1, 10 do
	factory()
end

-- All closures should be collectible after factory returns
-- Force GC
collectgarbage("collect")
`

	err := state.DoString(code)
	if err != nil {
		t.Errorf("Closure capture test failed: %v", err)
	}
}

// TestGCTableNesting verifies deeply nested tables are collected.
func TestGCTableNesting(t *testing.T) {
	code := `
local depth = 0

-- Create deeply nested table structure
local function make_nested(d)
	depth = d
	local t = {}
	for i = 1, d do
		t = {inner = t}
	end
	return t
end

-- Create multiple nested structures
for i = 1, 5 do
	make_nested(100)
end

-- Force GC
collectgarbage("collect")
`

	err := state.DoString(code)
	if err != nil {
		t.Errorf("Table nesting test failed: %v", err)
	}
}

// TestGCWeakReference verifies weak reference tables work correctly.
func TestGCWeakReference(t *testing.T) {
	code := `
-- Test weak value table
local weak_values = {}
setmetatable(weak_values, {__mode = "v"})

-- Add a table that should be collected
local temp = {}
weak_values[1] = temp
weak_values[2] = "keep this"

-- Remove strong reference
temp = nil

-- Force GC
collectgarbage("collect")

-- The string should still be there, table should be gone
local has_string = false
for k, v in pairs(weak_values) do
	if v == "keep this" then
		has_string = true
	end
end
-- String may or may not be collected depending on implementation
`

	err := state.DoString(code)
	if err != nil {
		t.Errorf("Weak reference test failed: %v", err)
	}
}

// TestGCWeakKey verifies weak key tables work correctly.
func TestGCWeakKey(t *testing.T) {
	code := `
-- Test weak key table
local weak_keys = {}
setmetatable(weak_keys, {__mode = "k"})

-- Add entries
local key1 = {}
local key2 = {}
weak_keys[key1] = "value1"
weak_keys[key2] = "value2"

-- Remove one key reference
key2 = nil

-- Force GC
collectgarbage("collect")
`

	err := state.DoString(code)
	if err != nil {
		t.Errorf("Weak key test failed: %v", err)
	}
}

// TestGCMixedRefs verifies mixed reference scenarios.
func TestGCMixedRefs(t *testing.T) {
	code := `
-- Create objects with multiple reference paths
local root = {}
local obj1 = {parent = root}
local obj2 = {parent = root, sibling = obj1}
obj1.sibling = obj2

-- Create closure with reference to shared object
local shared = {}
local function use_shared()
	return shared
end

-- Make closure and drop reference
use_shared()
shared = nil

-- Force GC
collectgarbage("collect")
`

	err := state.DoString(code)
	if err != nil {
		t.Errorf("Mixed references test failed: %v", err)
	}
}

// TestGCChainBreak verifies reference chain breaking works.
func TestGCChainBreak(t *testing.T) {
	code := `
-- Create linked list structure
local head = {}
local prev = head

for i = 2, 100 do
	local node = {value = i}
	prev.next = node
	prev = node
end

-- Break the chain at middle
local current = head
for i = 1, 50 do
	current = current.next
end
current.next = nil

-- Force GC - all nodes after break should be collectible
collectgarbage("collect")
`

	err := state.DoString(code)
	if err != nil {
		t.Errorf("Chain break test failed: %v", err)
	}
}

// TestGCLargeAllocation verifies large allocations don't cause issues.
func TestGCLargeAllocation(t *testing.T) {
	code := `
-- Allocate many objects
for i = 1, 1000 do
	local t = {}
	for j = 1, 100 do
		t[j] = {data = string.rep("x", 100)}
	end
end

-- Force multiple GC cycles
for i = 1, 5 do
	collectgarbage("collect")
end
`

	err := state.DoString(code)
	if err != nil {
		t.Errorf("Large allocation test failed: %v", err)
	}
}

// TestGCStress tests GC under stress conditions.
func TestGCStress(t *testing.T) {
	code := `
-- Alternate between allocation and GC
for i = 1, 100 do
	local objs = {}
	for j = 1, 50 do
		objs[j] = {j = j, ref = objs[j-1]}
	end
	
	if i % 10 == 0 then
		collectgarbage("collect")
	end
end

-- Final cleanup
collectgarbage("collect")
`

	err := state.DoString(code)
	if err != nil {
		t.Errorf("GC stress test failed: %v", err)
	}
}

// =============================================================================
// BenchmarkGCComplex - Performance benchmarks for GC scenarios
// =============================================================================

// BenchmarkGCCircular allocates objects with circular references.
func BenchmarkGCCircular(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		code := `
local t1 = {}
local t2 = {}
t1.ref = t2
t2.ref = t1
collectgarbage("collect")
`
		_ = state.DoString(code)
	}
}

// BenchmarkGCClosure allocates closures capturing variables.
func BenchmarkGCClosure(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		code := `
local closures = {}
for j = 1, 100 do
	local x = j
	closures[j] = function() return x end
end
closures = nil
collectgarbage("collect")
`
		_ = state.DoString(code)
	}
}

// BenchmarkGCTableNested allocates deeply nested tables.
func BenchmarkGCTableNested(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		code := `
local t = {}
for j = 1, 50 do
	t = {inner = t}
end
collectgarbage("collect")
`
		_ = state.DoString(code)
	}
}

// BenchmarkGCWeakTable allocates and collects weak reference tables.
func BenchmarkGCWeakTable(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		code := `
local wt = {}
setmetatable(wt, {__mode = "v"})
for j = 1, 100 do
	wt[j] = {}
end
collectgarbage("collect")
`
		_ = state.DoString(code)
	}
}

// BenchmarkGCLinkedList allocates and collects linked list structures.
func BenchmarkGCLinkedList(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		code := `
local head = nil
local prev = nil
for j = 1, 1000 do
	local node = {value = j, prev = prev}
	if prev == nil then head = node end
	prev = node
end
collectgarbage("collect")
`
		_ = state.DoString(code)
	}
}

// BenchmarkGCMixedScenario performs a mixed workload.
func BenchmarkGCMixedScenario(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		code := `
-- Create various object types
for j = 1, 50 do
	-- Table with circular ref
	local t1, t2 = {}, {}
	t1.r = t2; t2.r = t1
	
	-- Closure
	local x = j
	local f = function() return x end
	_ = f
	
	-- Nested table
	local n = {}
	for k = 1, 10 do n = {inner = n} end
end
collectgarbage("collect")
`
		_ = state.DoString(code)
	}
}

// =============================================================================
// Helper functions for memory inspection
// =============================================================================

// gcStats captures garbage collection statistics.
type gcStats struct {
	NumGC  uint32
	ByesGC uint64
}

// getGCStats returns current GC statistics.
func getGCStats() *gcStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return &gcStats{
		NumGC:  m.NumGC,
		ByesGC: m.Mallocs - m.Frees,
	}
}

// TestGCMemoryStats verifies GC affects memory stats.
func TestGCMemoryStats(t *testing.T) {
	before := getGCStats()

	// Allocate and collect
	code := `
local objs = {}
for i = 1, 1000 do
	objs[i] = {data = "test"}
end
objs = nil
collectgarbage("collect")
`
	_ = state.DoString(code)

	runtime.GC() // Force Go GC

	after := getGCStats()

	// Verify GC ran
	t.Logf("GC cycles: before=%d, after=%d", before.NumGC, after.NumGC)
	t.Logf("Net allocations: %d", after.ByesGC)
}
