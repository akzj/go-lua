package api

import (
	"testing"
)

// TestXPCallYieldFix tests xpcall+yield in coroutines.
// This was a crash bug: luaB_xpcall didn't set ci.K (continuation),
// so PCall took PATH A (non-yieldable) instead of PATH B, causing
// nil closure panic when resuming after yield inside xpcall.
func TestXPCallYieldFix(t *testing.T) {
	L := newState(t)

	// Test 1: basic xpcall+yield with error handler
	doString(t, L, `
local function f(a, b) a = coroutine.yield(a); error{a + b} end
local function g(x) return x[1]*2 end
co = coroutine.wrap(function()
    coroutine.yield(xpcall(f, g, 10, 20))
end)
assert(co() == 10)
local r, msg = co(100)
assert(not r and msg == 240)
`)

	// Test 2: xpcall+yield without error (success path)
	doString(t, L, `
co = coroutine.wrap(function()
    return xpcall(function()
        coroutine.yield(42)
        return 99
    end, function(e) return e end)
end)
assert(co() == 42)
local ok, val = co()
assert(ok == true and val == 99)
`)

	// Test 3: nested xpcall(pcall(...yield...))
	doString(t, L, `
local f = function (s, i) return coroutine.yield(i) end
local f1 = coroutine.wrap(function ()
             return xpcall(pcall, function (...) return ... end,
               function ()
                 local s = 0
                 for i in f, nil, 1 do pcall(function () s = s + i end) end
                 error({s})
               end)
           end)
f1()
for i = 1, 10 do assert(f1(i) == i) end
local r1, r2, v = f1(nil)
assert(r1 and not r2 and v[1] == (10 + 1)*10/2)
`)

	t.Log("All xpcall+yield tests PASSED")
}
