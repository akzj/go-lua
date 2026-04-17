package api

import (
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

// TestWeakTableValues tests that weak values are collected by GC.
func TestWeakTableValues(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	code := `
-- Test 1: Weak value should be collected after GC
local t = setmetatable({}, {__mode = "v"})
do
  local obj = {1, 2, 3}
  t[1] = obj
  assert(t[1] ~= nil, "value should exist before GC")
end
collectgarbage()
assert(t[1] == nil, "weak value should be nil after GC")

-- Test 2: Non-pointer values persist in weak tables
local t2 = setmetatable({}, {__mode = "v"})
t2[1] = 42
t2[2] = "hello"
t2[3] = true
collectgarbage()
assert(t2[1] == 42, "integer should persist")
assert(t2[2] == "hello", "string should persist")
assert(t2[3] == true, "boolean should persist")

-- Test 3: Values with external refs survive GC
local t3 = setmetatable({}, {__mode = "v"})
local keeper = {10, 20, 30}
t3[1] = keeper
collectgarbage()
assert(t3[1] == keeper, "referenced value should survive")

return "PASS"
`
	err := L.DoString(code)
	if err != nil {
		t.Fatalf("weak table test failed: %v", err)
	}
}

// TestWeakTableKeys tests that weak keys are collected by GC.
func TestWeakTableKeys(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	code := `
-- Test: Weak key table — entry removed when key is collected
local t = setmetatable({}, {__mode = "k"})
do
  local key = {}
  t[key] = "value"
  -- verify it's there
  local found = false
  for k, v in next, t do
    if v == "value" then found = true end
  end
  assert(found, "entry should exist before GC")
end
collectgarbage()
-- After GC, the key should be collected and entry removed
local count = 0
for k, v in next, t do
  count = count + 1
end
assert(count == 0, "weak key table should be empty after GC, got " .. count)

return "PASS"
`
	err := L.DoString(code)
	if err != nil {
		t.Fatalf("weak key test failed: %v", err)
	}
}

// TestWeakTableKV tests __mode="kv" (both weak keys and values).
func TestWeakTableKV(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	code := `
-- Matches coroutine.lua:478 pattern
local C = setmetatable({}, {__mode = "kv"})
do
  local obj = function() return 42 end
  C[1] = obj
  assert(C[1] ~= nil)
end
collectgarbage()
assert(C[1] == nil, "weak kv value should be nil after GC")

return "PASS"
`
	err := L.DoString(code)
	if err != nil {
		t.Fatalf("weak kv test failed: %v", err)
	}
}
