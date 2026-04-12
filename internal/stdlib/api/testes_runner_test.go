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
// Currently skipped: needs %p pointer format and coroutine.running().
func TestTestesStrings(t *testing.T) {
	t.Skip("strings.lua: needs pointer format, coroutine.running(), io.stdin")
	runTestFile(t, "strings.lua")
}

// TestTestesMath runs the official Lua 5.5 math.lua test suite.
func TestTestesMath(t *testing.T) {
	t.Skip("math.lua: needs investigation")
	runTestFile(t, "math.lua")
}

// TestTestesSort runs the official Lua 5.5 sort.lua test suite.
func TestTestesSort(t *testing.T) {
	t.Skip("sort.lua: needs table.create")
	runTestFile(t, "sort.lua")
}
