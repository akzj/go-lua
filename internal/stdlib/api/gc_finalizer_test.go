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
