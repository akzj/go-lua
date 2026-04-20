package api

import (
	"strings"
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

// TestPackageSearchersTable verifies that package.searchers is a table
// with the expected number of entries.
func TestPackageSearchersTable(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	code := `
		assert(type(package.searchers) == "table", "package.searchers should be a table")
		assert(#package.searchers >= 2, "should have at least 2 searchers, got " .. #package.searchers)
		assert(#package.searchers == 4, "should have exactly 4 searchers, got " .. #package.searchers)
		for i = 1, #package.searchers do
			assert(type(package.searchers[i]) == "function",
				"searcher " .. i .. " should be a function, got " .. type(package.searchers[i]))
		end
		return true
	`
	status := L.Load(code, "=test", "t")
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("load error: %s", msg)
	}
	status = L.PCall(0, 1, 0)
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("runtime error: %s", msg)
	}
}

// TestPackageSearchersPreload verifies that the preload searcher works
// and that require returns both the module value and the extra value.
func TestPackageSearchersPreload(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	code := `
		-- Register a preload function
		package.preload["mymod"] = function(modname, extra)
			return { name = modname, extra = extra }
		end

		-- require should find it via preload searcher
		local m, extra = require("mymod")
		assert(type(m) == "table", "module should be a table")
		assert(m.name == "mymod", "module name should be 'mymod', got " .. tostring(m.name))
		assert(extra == ":preload:", "extra should be ':preload:', got " .. tostring(extra))

		-- Second require should return cached value
		local m2, extra2 = require("mymod")
		assert(m2 == m, "cached module should be same object")

		return true
	`
	status := L.Load(code, "=test", "t")
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("load error: %s", msg)
	}
	status = L.PCall(0, 1, 0)
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("runtime error: %s", msg)
	}
}

// TestPackageSearchersCustom verifies that a custom searcher can be
// inserted into package.searchers and is called by require.
func TestPackageSearchersCustom(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	code := `
		-- Insert a custom searcher at position 1 (before preload)
		local called = false
		table.insert(package.searchers, 1, function(modname)
			if modname == "custom_mod" then
				called = true
				return function(name, extra)
					return { custom = true, name = name, extra = extra }
				end, "custom_extra"
			end
			return "not handled by custom searcher"
		end)

		local m, extra = require("custom_mod")
		assert(called, "custom searcher should have been called")
		assert(type(m) == "table", "module should be a table")
		assert(m.custom == true, "module should have custom=true")
		assert(m.name == "custom_mod", "module name mismatch")
		assert(m.extra == "custom_extra", "extra should be 'custom_extra', got " .. tostring(m.extra))
		assert(extra == "custom_extra", "require second return should be 'custom_extra'")

		return true
	`
	status := L.Load(code, "=test", "t")
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("load error: %s", msg)
	}
	status = L.PCall(0, 1, 0)
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("runtime error: %s", msg)
	}
}

// TestPackageSearchersNotTable verifies that require errors when
// package.searchers is not a table (matches attrib.lua line 243).
func TestPackageSearchersNotTable(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	code := `
		local searchers = package.searchers
		package.searchers = 3
		local st, msg = pcall(require, 'a')
		package.searchers = searchers
		assert(not st, "require should fail when searchers is not a table")
		assert(string.find(msg, "must be a table"), "error should mention 'must be a table', got: " .. msg)
		return true
	`
	status := L.Load(code, "=test", "t")
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("load error: %s", msg)
	}
	status = L.PCall(0, 1, 0)
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("runtime error: %s", msg)
	}
}

// TestPackageSearchersErrorMessage verifies the error message format
// when no searcher finds the module (matches attrib.lua line 51-68).
func TestPackageSearchersErrorMessage(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	code := `
		local oldpath = package.path
		local oldcpath = package.cpath
		package.path = "?.lua;?/?"
		package.cpath = "?.so;?/init"

		local st, msg = pcall(require, 'XXX')

		package.path = oldpath
		package.cpath = oldcpath

		assert(not st, "require should fail for nonexistent module")
		-- Check key parts of the error message
		assert(string.find(msg, "module 'XXX' not found:"), "should start with module not found, got: " .. msg)
		assert(string.find(msg, "no field package.preload%['XXX'%]"), "should mention preload, got: " .. msg)
		assert(string.find(msg, "no file 'XXX.lua'"), "should mention XXX.lua, got: " .. msg)
		assert(string.find(msg, "no file 'XXX/XXX'"), "should mention XXX/XXX, got: " .. msg)
		assert(string.find(msg, "no file 'XXX.so'"), "should mention XXX.so, got: " .. msg)
		assert(string.find(msg, "no file 'XXX/init'"), "should mention XXX/init, got: " .. msg)

		return true
	`
	status := L.Load(code, "=test", "t")
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("load error: %s", msg)
	}
	status = L.PCall(0, 1, 0)
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		if strings.Contains(msg, "got:") {
			t.Fatalf("assertion failed: %s", msg)
		}
		t.Fatalf("runtime error: %s", msg)
	}
}

// TestPackageSearchersRemove verifies that removing a searcher works.
func TestPackageSearchersRemove(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	code := `
		-- Remove all searchers except a custom one
		local orig = {}
		for i = 1, #package.searchers do
			orig[i] = package.searchers[i]
		end

		-- Clear all searchers
		for i = #package.searchers, 1, -1 do
			table.remove(package.searchers, i)
		end

		-- Add only our custom searcher
		package.searchers[1] = function(modname)
			if modname == "only_custom" then
				return function() return 42 end, "custom_path"
			end
			return "custom searcher: module '" .. modname .. "' not found"
		end

		local m, extra = require("only_custom")
		assert(m == 42, "should get 42, got " .. tostring(m))
		assert(extra == "custom_path", "should get custom_path, got " .. tostring(extra))

		-- Restore original searchers
		for i = #package.searchers, 1, -1 do
			table.remove(package.searchers, i)
		end
		for i, s in ipairs(orig) do
			package.searchers[i] = s
		end

		return true
	`
	status := L.Load(code, "=test", "t")
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("load error: %s", msg)
	}
	status = L.PCall(0, 1, 0)
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("runtime error: %s", msg)
	}
}
