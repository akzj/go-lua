package lua_test

import (
	"fmt"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustDoString(t *testing.T, L *lua.State, code string) {
	t.Helper()
	if err := L.DoString(code); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func getGlobalInt(t *testing.T, L *lua.State, name string) int64 {
	t.Helper()
	L.GetGlobal(name)
	v, ok := L.ToInteger(-1)
	if !ok {
		t.Fatalf("global %q is not an integer", name)
	}
	L.Pop(1)
	return v
}

func getGlobalString(t *testing.T, L *lua.State, name string) string {
	t.Helper()
	L.GetGlobal(name)
	s, ok := L.ToString(-1)
	if !ok {
		t.Fatalf("global %q is not a string", name)
	}
	L.Pop(1)
	return s
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestReloadModule_Basic(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Load initial module
	mustDoString(t, L, `
		package.preload["mymod"] = function()
			local M = {}
			function M.greet(name)
				return "hello " .. name
			end
			return M
		end
	`)

	// Use the module
	mustDoString(t, L, `
		local m = require("mymod")
		result1 = m.greet("world")
	`)

	if s := getGlobalString(t, L, "result1"); s != "hello world" {
		t.Fatalf("expected 'hello world', got %q", s)
	}

	// Update the preload with new implementation
	mustDoString(t, L, `
		package.preload["mymod"] = function()
			local M = {}
			function M.greet(name)
				return "hi " .. name .. "!"
			end
			return M
		end
	`)

	// Reload
	result, err := L.ReloadModule("mymod")
	if err != nil {
		t.Fatal(err)
	}
	if result.Replaced != 1 {
		t.Fatalf("expected 1 replaced, got %d", result.Replaced)
	}

	// Verify new behavior through cached require
	mustDoString(t, L, `
		local m = require("mymod")
		result2 = m.greet("world")
	`)

	if s := getGlobalString(t, L, "result2"); s != "hi world!" {
		t.Fatalf("expected 'hi world!', got %q", s)
	}
}

func TestReloadModule_PreservesState(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Module with stateful upvalue
	mustDoString(t, L, `
		package.preload["counter"] = function()
			local M = {}
			local count = 0
			function M.increment()
				count = count + 1
				return count
			end
			function M.get()
				return count
			end
			return M
		end
	`)

	// Increment 3 times
	mustDoString(t, L, `
		local c = require("counter")
		c.increment()
		c.increment()
		c.increment()
		before_reload = c.get()
	`)

	if v := getGlobalInt(t, L, "before_reload"); v != 3 {
		t.Fatalf("expected 3, got %d", v)
	}

	// Update increment to add 10 instead of 1
	mustDoString(t, L, `
		package.preload["counter"] = function()
			local M = {}
			local count = 0
			function M.increment()
				count = count + 10
				return count
			end
			function M.get()
				return count
			end
			return M
		end
	`)

	// Reload
	result, err := L.ReloadModule("counter")
	if err != nil {
		t.Fatal(err)
	}
	if result.Replaced != 2 {
		t.Fatalf("expected 2 replaced, got %d", result.Replaced)
	}

	// Verify: count preserved at 3, new increment adds 10
	mustDoString(t, L, `
		local c = require("counter")
		after_reload = c.get()
		c.increment()
		after_increment = c.get()
	`)

	if v := getGlobalInt(t, L, "after_reload"); v != 3 {
		t.Fatalf("expected 3 after reload, got %d", v)
	}
	if v := getGlobalInt(t, L, "after_increment"); v != 13 {
		t.Fatalf("expected 13, got %d", v)
	}
}

func TestReloadModule_CompileError(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Load initial module
	mustDoString(t, L, `
		package.preload["bugmod"] = function()
			local M = {}
			function M.hello() return "ok" end
			return M
		end
	`)
	mustDoString(t, L, `require("bugmod")`)

	// Set broken preload — the function itself is valid Lua, but it
	// errors at runtime when require() calls it.
	mustDoString(t, L, `
		package.preload["bugmod"] = function()
			error("deliberate load failure")
		end
	`)

	// Reload should fail
	_, err := L.ReloadModule("bugmod")
	if err == nil {
		t.Fatal("expected error for broken module")
	}

	// Verify old module is still intact
	mustDoString(t, L, `
		local m = require("bugmod")
		check = m.hello()
	`)
	if s := getGlobalString(t, L, "check"); s != "ok" {
		t.Fatalf("expected 'ok', got %q", s)
	}
}

func TestReloadModule_AddedFunction(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Initial module with one function
	mustDoString(t, L, `
		package.preload["addmod"] = function()
			local M = {}
			function M.foo() return "foo" end
			return M
		end
	`)
	mustDoString(t, L, `require("addmod")`)

	// New version with an extra function
	mustDoString(t, L, `
		package.preload["addmod"] = function()
			local M = {}
			function M.foo() return "foo_v2" end
			function M.bar() return "bar_new" end
			return M
		end
	`)

	result, err := L.ReloadModule("addmod")
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 {
		t.Fatalf("expected 1 added, got %d", result.Added)
	}

	// Verify both functions work
	mustDoString(t, L, `
		local m = require("addmod")
		foo_result = m.foo()
		bar_result = m.bar()
	`)
	if s := getGlobalString(t, L, "foo_result"); s != "foo_v2" {
		t.Fatalf("expected 'foo_v2', got %q", s)
	}
	if s := getGlobalString(t, L, "bar_result"); s != "bar_new" {
		t.Fatalf("expected 'bar_new', got %q", s)
	}
}

func TestReloadModule_RemovedFunction(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Initial module with two functions
	mustDoString(t, L, `
		package.preload["remmod"] = function()
			local M = {}
			function M.foo() return "foo" end
			function M.bar() return "bar" end
			return M
		end
	`)
	mustDoString(t, L, `require("remmod")`)

	// New version removes bar
	mustDoString(t, L, `
		package.preload["remmod"] = function()
			local M = {}
			function M.foo() return "foo_v2" end
			return M
		end
	`)

	result, err := L.ReloadModule("remmod")
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 {
		t.Fatalf("expected 1 removed, got %d", result.Removed)
	}

	// Old bar function should still exist (not removed from table)
	mustDoString(t, L, `
		local m = require("remmod")
		foo_result = m.foo()
		bar_result = m.bar()
	`)
	if s := getGlobalString(t, L, "foo_result"); s != "foo_v2" {
		t.Fatalf("expected 'foo_v2', got %q", s)
	}
	if s := getGlobalString(t, L, "bar_result"); s != "bar" {
		t.Fatalf("expected 'bar' (old version preserved), got %q", s)
	}
}

func TestPrepareReload_Abort(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mustDoString(t, L, `
		package.preload["abortmod"] = function()
			local M = {}
			function M.hello() return "v1" end
			return M
		end
	`)
	mustDoString(t, L, `require("abortmod")`)

	// Update preload
	mustDoString(t, L, `
		package.preload["abortmod"] = function()
			local M = {}
			function M.hello() return "v2" end
			return M
		end
	`)

	// Prepare but abort
	plan, err := L.PrepareReload("abortmod")
	if err != nil {
		t.Fatal(err)
	}
	plan.Abort()

	// Verify nothing changed
	mustDoString(t, L, `
		local m = require("abortmod")
		check = m.hello()
	`)
	if s := getGlobalString(t, L, "check"); s != "v1" {
		t.Fatalf("expected 'v1' after abort, got %q", s)
	}
}

func TestPrepareReload_Commit(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mustDoString(t, L, `
		package.preload["commitmod"] = function()
			local M = {}
			function M.hello() return "v1" end
			return M
		end
	`)
	mustDoString(t, L, `require("commitmod")`)

	// Update preload
	mustDoString(t, L, `
		package.preload["commitmod"] = function()
			local M = {}
			function M.hello() return "v2" end
			return M
		end
	`)

	plan, err := L.PrepareReload("commitmod")
	if err != nil {
		t.Fatal(err)
	}

	// Verify plan metadata
	if len(plan.Pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(plan.Pairs))
	}
	if plan.Pairs[0].Name != "hello" {
		t.Fatalf("expected pair name 'hello', got %q", plan.Pairs[0].Name)
	}

	result := plan.Commit()
	if result.Replaced != 1 {
		t.Fatalf("expected 1 replaced, got %d", result.Replaced)
	}

	// Verify update applied
	mustDoString(t, L, `
		local m = require("commitmod")
		check = m.hello()
	`)
	if s := getGlobalString(t, L, "check"); s != "v2" {
		t.Fatalf("expected 'v2' after commit, got %q", s)
	}
}

func TestReloadModule_MultipleReloads(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Version 1
	mustDoString(t, L, `
		package.preload["multimod"] = function()
			local M = {}
			local count = 0
			function M.inc()
				count = count + 1
				return count
			end
			return M
		end
	`)
	mustDoString(t, L, `
		local m = require("multimod")
		m.inc() -- count = 1
	`)

	// Version 2: inc adds 10
	mustDoString(t, L, `
		package.preload["multimod"] = function()
			local M = {}
			local count = 0
			function M.inc()
				count = count + 10
				return count
			end
			return M
		end
	`)
	result, err := L.ReloadModule("multimod")
	if err != nil {
		t.Fatal(err)
	}
	if result.Replaced != 1 {
		t.Fatalf("reload 1: expected 1 replaced, got %d", result.Replaced)
	}

	mustDoString(t, L, `
		local m = require("multimod")
		m.inc() -- count = 1 + 10 = 11
	`)

	// Version 3: inc adds 100
	mustDoString(t, L, `
		package.preload["multimod"] = function()
			local M = {}
			local count = 0
			function M.inc()
				count = count + 100
				return count
			end
			return M
		end
	`)
	result, err = L.ReloadModule("multimod")
	if err != nil {
		t.Fatal(err)
	}

	mustDoString(t, L, `
		local m = require("multimod")
		final = m.inc() -- count = 11 + 100 = 111
	`)

	if v := getGlobalInt(t, L, "final"); v != 111 {
		t.Fatalf("expected 111 after 3 reloads, got %d", v)
	}
}

func TestReloadModule_NotLoaded(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	_, err := L.ReloadModule("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-loaded module")
	}
}

func TestReloadModule_NonTableModule(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Load a module that returns a non-table value
	mustDoString(t, L, `
		package.preload["stringmod"] = function()
			return "just a string"
		end
		require("stringmod")
	`)

	_, err := L.ReloadModule("stringmod")
	if err == nil {
		t.Fatal("expected error for non-table module")
	}
}

func TestHotReload_LuaAPI(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Load a module
	mustDoString(t, L, `
		package.preload["luamod"] = function()
			local M = {}
			function M.value() return 42 end
			return M
		end
		require("luamod")
	`)

	// Update preload
	mustDoString(t, L, `
		package.preload["luamod"] = function()
			local M = {}
			function M.value() return 99 end
			return M
		end
	`)

	// Use Lua hotreload API
	mustDoString(t, L, `
		local hr = require("hotreload")
		local result, err = hr.reload("luamod")
		if err then error(err) end
		reload_replaced = result.replaced
		reload_module = result.module
	`)

	if v := getGlobalInt(t, L, "reload_replaced"); v != 1 {
		t.Fatalf("expected 1 replaced via Lua API, got %d", v)
	}
	if s := getGlobalString(t, L, "reload_module"); s != "luamod" {
		t.Fatalf("expected module name 'luamod', got %q", s)
	}

	// Verify the module was actually updated
	mustDoString(t, L, `
		local m = require("luamod")
		lua_check = m.value()
	`)
	if v := getGlobalInt(t, L, "lua_check"); v != 99 {
		t.Fatalf("expected 99, got %d", v)
	}
}

func TestHotReload_LuaPrepareCommit(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mustDoString(t, L, `
		package.preload["planmod"] = function()
			local M = {}
			function M.val() return 1 end
			return M
		end
		require("planmod")
	`)

	// Update
	mustDoString(t, L, `
		package.preload["planmod"] = function()
			local M = {}
			function M.val() return 2 end
			return M
		end
	`)

	// Use prepare/commit API from Lua
	mustDoString(t, L, `
		local hr = require("hotreload")
		local plan, err = hr.prepare("planmod")
		if err then error(err) end

		local info = plan:info()
		plan_matched = info.matched

		local result = plan:commit()
		plan_replaced = result.replaced
	`)

	if v := getGlobalInt(t, L, "plan_matched"); v != 1 {
		t.Fatalf("expected 1 matched, got %d", v)
	}
	if v := getGlobalInt(t, L, "plan_replaced"); v != 1 {
		t.Fatalf("expected 1 replaced, got %d", v)
	}

	// Verify update
	mustDoString(t, L, `
		local m = require("planmod")
		plan_check = m.val()
	`)
	if v := getGlobalInt(t, L, "plan_check"); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}

}

// ---------------------------------------------------------------------------
// New comprehensive tests for bug fixes and edge cases
// ---------------------------------------------------------------------------

// Test 1: Shared upvalues between functions — after reload, both functions
// should still share the same upvalue (the old count variable).
func TestReloadModule_SharedUpvalues(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mustDoString(t, L, `
		package.preload["shared"] = function()
			local M = {}
			local count = 0
			function M.inc() count = count + 1; return count end
			function M.get() return count end
			return M
		end
	`)
	mustDoString(t, L, `
		local m = require("shared")
		m.inc(); m.inc()  -- count = 2
	`)

	// Reload with new inc (adds 10 instead of 1)
	mustDoString(t, L, `
		package.preload["shared"] = function()
			local M = {}
			local count = 0
			function M.inc() count = count + 10; return count end
			function M.get() return count end
			return M
		end
	`)

	result, err := L.ReloadModule("shared")
	if err != nil {
		t.Fatal(err)
	}
	if result.Replaced != 2 {
		t.Fatalf("expected 2 replaced, got %d", result.Replaced)
	}

	// Verify: count preserved at 2, inc now adds 10, and get still sees same count
	mustDoString(t, L, `
		local m = require("shared")
		shared_get_after = m.get()       -- should be 2 (preserved)
		m.inc()
		shared_get_after_inc = m.get()   -- should be 12 (2 + 10)
	`)

	if v := getGlobalInt(t, L, "shared_get_after"); v != 2 {
		t.Fatalf("expected count=2 after reload, got %d", v)
	}
	if v := getGlobalInt(t, L, "shared_get_after_inc"); v != 12 {
		t.Fatalf("expected count=12 after inc, got %d", v)
	}
}

// Test 2: GC stress during reload — force GC around reload to expose
// write barrier issues (BUG-1 regression test).
// Uses a function that returns a constant from the Proto (no upvalue),
// so each reload's new code directly changes the return value.
func TestReloadModule_GCStress(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mustDoString(t, L, `
		package.preload["gcmod"] = function()
			local M = {}
			function M.version() return 0 end
			return M
		end
	`)
	mustDoString(t, L, `require("gcmod")`)

	for i := 0; i < 100; i++ {
		mustDoString(t, L, fmt.Sprintf(`
			package.preload["gcmod"] = function()
				local M = {}
				function M.version() return %d end
				return M
			end
		`, i+1))

		// Force GC before reload
		mustDoString(t, L, `collectgarbage("collect")`)

		_, err := L.ReloadModule("gcmod")
		if err != nil {
			t.Fatalf("reload %d: %v", i, err)
		}

		// Force GC after reload — this is the critical moment where
		// a missing write barrier would cause the new Proto to be swept
		mustDoString(t, L, `collectgarbage("collect")`)

		// Verify the function returns the new constant (from new Proto)
		mustDoString(t, L, `gc_result = require("gcmod").version()`)
		expected := int64(i + 1)
		got := getGlobalInt(t, L, "gc_result")
		if got != expected {
			t.Fatalf("reload %d: expected %d, got %d", i, expected, got)
		}
	}
}

// Test 3: Stress test — 100 reloads preserving state
func TestReloadModule_Stress100(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mustDoString(t, L, `
		package.preload["stress"] = function()
			local M = {}
			local counter = 0
			function M.inc() counter = counter + 1; return counter end
			function M.get() return counter end
			return M
		end
	`)
	mustDoString(t, L, `
		local m = require("stress")
		for i = 1, 10 do m.inc() end  -- counter = 10
	`)

	for i := 0; i < 100; i++ {
		mustDoString(t, L, fmt.Sprintf(`
			package.preload["stress"] = function()
				local M = {}
				local counter = 0
				function M.inc() counter = counter + %d; return counter end
				function M.get() return counter end
				return M
			end
		`, i+1))

		_, err := L.ReloadModule("stress")
		if err != nil {
			t.Fatalf("reload %d: %v", i, err)
		}
	}

	// counter should be preserved through all reloads (still 10)
	// inc should now add 100 (last reload: i=99 → i+1=100)
	mustDoString(t, L, `
		local m = require("stress")
		stress_before = m.get()
		m.inc()
		stress_after = m.get()
	`)

	if v := getGlobalInt(t, L, "stress_before"); v != 10 {
		t.Fatalf("expected counter=10, got %d", v)
	}
	if v := getGlobalInt(t, L, "stress_after"); v != 110 {
		t.Fatalf("expected 110 (10+100), got %d", v)
	}
}

// Test 4: PrepareReload side effects — documents that init code runs during
// Prepare even if Abort is called (BUG-2 documentation test).
func TestPrepareReload_SideEffects(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mustDoString(t, L, `
		SIDE_EFFECT = 0
		package.preload["sidemod"] = function()
			SIDE_EFFECT = SIDE_EFFECT + 1
			local M = {}
			function M.get() return SIDE_EFFECT end
			return M
		end
	`)
	mustDoString(t, L, `require("sidemod")`) // SIDE_EFFECT = 1

	// Update preload (same code — will increment SIDE_EFFECT again)
	mustDoString(t, L, `
		package.preload["sidemod"] = function()
			SIDE_EFFECT = SIDE_EFFECT + 1
			local M = {}
			function M.get() return SIDE_EFFECT end
			return M
		end
	`)

	plan, err := L.PrepareReload("sidemod")
	if err != nil {
		t.Fatal(err)
	}
	plan.Abort() // abort — but side effect already happened

	// SIDE_EFFECT is 2 because PrepareReload ran the init code via require()
	if v := getGlobalInt(t, L, "SIDE_EFFECT"); v != 2 {
		t.Fatalf("expected SIDE_EFFECT=2 (init ran during prepare), got %d", v)
	}
}

// Test 5: Module with metatables — documents that only top-level function
// entries in the module table are matched, not metatable functions.
func TestReloadModule_WithMetatable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mustDoString(t, L, `
		package.preload["metamod"] = function()
			local M = {}
			local internal = {x = 10}
			setmetatable(M, {
				__index = function(t, k)
					if k == "x" then return internal.x end
				end
			})
			function M.setX(v) internal.x = v end
			return M
		end
	`)
	mustDoString(t, L, `
		local m = require("metamod")
		m.setX(42)
		meta_before = m.x  -- 42 via __index
	`)

	if v := getGlobalInt(t, L, "meta_before"); v != 42 {
		t.Fatalf("expected 42, got %d", v)
	}

	mustDoString(t, L, `
		package.preload["metamod"] = function()
			local M = {}
			local internal = {x = 10}
			setmetatable(M, {
				__index = function(t, k)
					if k == "x" then return internal.x * 2 end
				end
			})
			function M.setX(v) internal.x = v end
			return M
		end
	`)

	result, err := L.ReloadModule("metamod")
	if err != nil {
		t.Fatal(err)
	}
	// Only setX is a top-level entry; __index is in the metatable
	if result.Replaced != 1 {
		t.Fatalf("expected 1 replaced (setX only), got %d", result.Replaced)
	}

	// After reload, setX is replaced but __index metatable is unchanged
	// (it's the old metatable since we mutate the old module table in-place)
	mustDoString(t, L, `
		local m = require("metamod")
		meta_after = m.x  -- still uses old __index → 42
	`)
	if v := getGlobalInt(t, L, "meta_after"); v != 42 {
		t.Fatalf("expected 42 (old metatable preserved), got %d", v)
	}
}

// Test 6: Upvalue that is itself a function — after reload, the old helper
// function upvalue is transferred to the new closure.
func TestReloadModule_FunctionUpvalue(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mustDoString(t, L, `
		package.preload["fnup"] = function()
			local M = {}
			local helper = function(x) return x * 2 end
			function M.calc(x) return helper(x) end
			return M
		end
	`)
	mustDoString(t, L, `
		local m = require("fnup")
		fnup_before = m.calc(5)  -- 10
	`)

	if v := getGlobalInt(t, L, "fnup_before"); v != 10 {
		t.Fatalf("expected 10, got %d", v)
	}

	mustDoString(t, L, `
		package.preload["fnup"] = function()
			local M = {}
			local helper = function(x) return x * 3 end  -- changed!
			function M.calc(x) return helper(x) + 1 end  -- changed!
			return M
		end
	`)

	_, err := L.ReloadModule("fnup")
	if err != nil {
		t.Fatal(err)
	}

	// After reload: calc uses new code (helper(x) + 1)
	// But helper upvalue was transferred from old → still x*2
	// So result = 5*2 + 1 = 11 (old helper, new calc body)
	mustDoString(t, L, `
		local m = require("fnup")
		fnup_after = m.calc(5)
	`)

	if v := getGlobalInt(t, L, "fnup_after"); v != 11 {
		t.Fatalf("expected 11 (old helper x*2 + new body +1), got %d", v)
	}
}

// Test 7: Upvalue count mismatch — function should be skipped as incompatible
// when the new version has a different number of upvalues.
func TestReloadModule_UpvalueCountMismatch(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mustDoString(t, L, `
		package.preload["upmismatch"] = function()
			local M = {}
			local a = 1
			function M.get() return a end
			return M
		end
	`)
	mustDoString(t, L, `require("upmismatch")`)

	// New version adds an extra upvalue
	mustDoString(t, L, `
		package.preload["upmismatch"] = function()
			local M = {}
			local a = 1
			local b = 2
			function M.get() return a + b end
			return M
		end
	`)

	result, err := L.ReloadModule("upmismatch")
	if err != nil {
		t.Fatal(err)
	}

	if result.Skipped != 1 {
		t.Fatalf("expected 1 skipped (incompatible), got %d", result.Skipped)
	}
	if result.Replaced != 0 {
		t.Fatalf("expected 0 replaced, got %d", result.Replaced)
	}

	// Old function should still work with original value
	mustDoString(t, L, `
		local m = require("upmismatch")
		upmismatch_val = m.get()
	`)

	if v := getGlobalInt(t, L, "upmismatch_val"); v != 1 {
		t.Fatalf("expected 1 (old function), got %d", v)
	}
}
