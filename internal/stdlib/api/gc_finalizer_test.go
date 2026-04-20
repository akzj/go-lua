package api

import (
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

// TestGCFinalizer verifies that tables with __gc metamethod have their
// __gc called when collectgarbage("collect") is invoked after the table
// becomes unreachable.
func TestGCFinalizer(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	script := `
-- Create a table with __gc, store evidence of __gc firing in a global
_G.gc_fired = false
local t = setmetatable({}, {__gc = function(self)
    _G.gc_fired = true
end})
t = nil
collectgarbage("collect")
assert(_G.gc_fired == true, "expected __gc to fire after collectgarbage")
`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("gc finalizer test failed: %v", err)
	}
}

// TestGCFinalizerMultiple verifies multiple __gc finalizers fire.
func TestGCFinalizerMultiple(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	script := `
_G.gc_count = 0
for i = 1, 5 do
    local t = setmetatable({}, {__gc = function(self)
        _G.gc_count = _G.gc_count + 1
    end})
end
-- all 5 tables are now unreachable (loop var goes out of scope each iteration)
-- Multiple collect cycles to ensure all finalizers run (Go GC timing)
for i = 1, 3 do
    collectgarbage("collect")
end
assert(_G.gc_count == 5, "expected 5 __gc calls, got " .. tostring(_G.gc_count))
`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("gc finalizer multiple test failed: %v", err)
	}
}

// TestGCFinalizerErrorSwallowed verifies that errors in __gc are silently discarded.
func TestGCFinalizerErrorSwallowed(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	script := `
_G.gc_after_error = false
-- First: a table whose __gc errors
local t1 = setmetatable({}, {__gc = function(self)
    error("boom")
end})
-- Second: a table whose __gc should still fire despite the first one erroring
local t2 = setmetatable({}, {__gc = function(self)
    _G.gc_after_error = true
end})
t1 = nil
t2 = nil
collectgarbage("collect")
assert(_G.gc_after_error == true, "expected __gc to fire even after error in another __gc")
`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("gc finalizer error swallowed test failed: %v", err)
	}
}

// TestGCFinalizerManyErrors is a stress test: many __gc finalizers that all
// error. Without proper stack cleanup (SetTop after PCall), this would overflow
// the Lua stack because each failed PCall leaves an error object.
func TestGCFinalizerManyErrors(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	script := `
_G.gc_ok = false
-- Create 100 tables whose __gc all error
for i = 1, 100 do
    local t = setmetatable({}, {__gc = function(self)
        error("gc error " .. i)
    end})
end
-- Create one more that succeeds, to prove the stack is not corrupted
local sentinel = setmetatable({}, {__gc = function(self)
    _G.gc_ok = true
end})
sentinel = nil
-- Multiple cycles to collect everything
for i = 1, 5 do
    collectgarbage("collect")
end
assert(_G.gc_ok == true, "expected sentinel __gc to fire after 100 erroring __gc calls")
`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("gc finalizer many errors test failed: %v", err)
	}
}

// TestGCTotalBytesDecreases verifies that collectgarbage("count") returns a
// lower value after tables become unreachable and are collected.
// This tests the AddCleanup-based dealloc tracking on tables.
func TestGCTotalBytesDecreases(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	script := `
-- Get baseline count after initial setup
collectgarbage("collect")
local baseline = collectgarbage("count")

-- Create many tables to increase the count significantly
local holder = {}
for i = 1, 1000 do
    holder[i] = {i, i+1, i+2, i+3}
end

-- Verify count increased
collectgarbage("collect")
local after_alloc = collectgarbage("count")
assert(after_alloc > baseline,
    string.format("expected count to increase: baseline=%.1f, after_alloc=%.1f",
        baseline, after_alloc))

-- Drop all references
holder = nil

-- Collect garbage — AddCleanup should decrement GCTotalBytes
collectgarbage("collect")
collectgarbage("collect")  -- second pass for cleanup callbacks
local after_gc = collectgarbage("count")

-- The count should decrease after collection.
-- It may not drop all the way to baseline (other allocations happen),
-- but it should be significantly lower than after_alloc.
local decrease = after_alloc - after_gc
assert(decrease > 0,
    string.format("expected count to decrease after GC: after_alloc=%.1f, after_gc=%.1f",
        after_alloc, after_gc))

-- Verify the decrease is substantial (at least 50% of what we allocated)
local allocated = after_alloc - baseline
assert(decrease > allocated * 0.5,
    string.format("expected substantial decrease: allocated=%.1f, decreased=%.1f",
        allocated, decrease))
`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("GCTotalBytes decrease test failed: %v", err)
	}
}
