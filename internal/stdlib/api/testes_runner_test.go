package api

import (
	"os"
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

const testesDir = "../../../lua-master/testes/"

func runTestFile(t *testing.T, filename string) {
	t.Helper()
	path := testesDir + filename
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("testes file not found: %s", path)
	}
	L := luaapi.NewState()
	OpenAll(L)
	err := L.DoFile(path)
	if err != nil {
		t.Fatalf("%s: %v", filename, err)
	}
}

// TestTestesStrings runs the official Lua 5.5 strings.lua test suite.
// Superseded by TestTestesWide which sets _port/_soft and applies needed patches.
func TestTestesStrings(t *testing.T) {
	t.Skip("Superseded by TestTestesWide (strings.lua passes there with _port patches)")
	runTestFile(t, "strings.lua")
}

// TestTestesMath runs the official Lua 5.5 math.lua test suite.
// Superseded by TestTestesWide which sets _port/_soft flags.
func TestTestesMath(t *testing.T) {
	t.Skip("Superseded by TestTestesWide (math.lua passes there)")
	runTestFile(t, "math.lua")
}

// TestTestesSort runs the official Lua 5.5 sort.lua test suite.
// Superseded by TestTestesWide which sets _port/_soft flags.
func TestTestesSort(t *testing.T) {
	t.Skip("Superseded by TestTestesWide (sort.lua passes there)")
	runTestFile(t, "sort.lua")
}
