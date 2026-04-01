package lua

import (
	"testing"

	"github.com/akzj/go-lua/internal/lauxlib"
)

func TestLuaLNewstate(t *testing.T) {
	// Just test that we can create a state
	L := lauxlib.LuaL_newstate()
	if L == nil {
		t.Error("LuaL_newstate returned nil")
	}
}

func TestBaselibsRegistered(t *testing.T) {
	// Verify baselibs has expected functions
	expectedFuncs := []string{
		"print", "tonumber", "tostring", "error",
		"type", "getmetatable", "setmetatable",
		"rawequal", "rawlen", "rawget", "rawset",
		"pairs", "next", "ipairs", "select",
		"pcall", "xpcall", "assert",
	}

	funcMap := make(map[string]bool)
	for _, r := range baselibs {
		funcMap[r.Name] = true
	}

	for _, name := range expectedFuncs {
		if !funcMap[name] {
			t.Errorf("expected function %q not found in baselibs", name)
		}
	}
}
