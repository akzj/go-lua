package lua_test

import (
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
