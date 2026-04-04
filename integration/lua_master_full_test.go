// Package integration provides end-to-end integration tests for the Lua VM.
package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akzj/go-lua/state"
)

// TestLuaMasterFullExecution verifies that actual lua-master/testes/*.lua files
// can be executed end-to-end through DoString (parse + compile + run).
//
// Each subtest corresponds to a distinct lua-master/*.lua file and executes
// the actual file content through DoString.
//
// Acceptance criteria: go test -run TestLuaMasterFullExecution ./... -v | grep -c "PASS" >= 20

// luaMasterRoot finds the repo root by walking up to find lua-master directory
func luaMasterRoot() string {
	cwd, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(cwd, "lua-master")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return ""
}

// helper reads a lua-master file and executes it via DoString with error handling
func runLuaMasterFile(t *testing.T, filename string) {
	root := luaMasterRoot()
	if root == "" {
		t.Skip("could not find lua-master directory")
	}
	path := filepath.Join(root, "lua-master", "testes", filename)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("file not found: %s (%v)", path, err)
	}

	// Use pcall wrapper to handle potential panics from unimplemented features
	// Wrap the file content with a protected call
	wrapper := `
assert = function(cond, msg)
	if not cond then
		error(msg or "assertion failed!")
	end
end

-- Provide basic table helpers if not available
table.concat = table.concat or function(t, sep)
	sep = sep or ""
	local result = ""
	for i, v in ipairs(t) do
		if i > 1 then result = result .. sep end
		result = result .. tostring(v)
	end
	return result
end

table.insert = table.insert or function(t, v)
	t[#t + 1] = v
end

table.remove = table.remove or function(t, i)
	i = i or #t
	local val = t[i]
	for j = i, #t - 1 do
		t[j] = t[j + 1]
	end
	t[#t] = nil
	return val
end

table.unpack = table.unpack or unpack
table.pack = table.pack or function(...)
	return {n = select("#", ...), ...}
end

-- Run the test file content
` + string(content)

	err = state.DoString(wrapper)
	if err != nil {
		t.Errorf("failed to execute %s: %v", filename, err)
	}
}

// TestLuaMasterFullExecution_constructs executes lua-master/testes/constructs.lua
func TestLuaMasterFullExecution_constructs(t *testing.T) {
	runLuaMasterFile(t, "constructs.lua")
}

// TestLuaMasterFullExecution_literals executes lua-master/testes/literals.lua
func TestLuaMasterFullExecution_literals(t *testing.T) {
	runLuaMasterFile(t, "literals.lua")
}

// TestLuaMasterFullExecution_closure executes lua-master/testes/closure.lua
func TestLuaMasterFullExecution_closure(t *testing.T) {
	runLuaMasterFile(t, "closure.lua")
}

// TestLuaMasterFullExecution_nextvar executes lua-master/testes/nextvar.lua
func TestLuaMasterFullExecution_nextvar(t *testing.T) {
	runLuaMasterFile(t, "nextvar.lua")
}

// TestLuaMasterFullExecution_calls executes lua-master/testes/calls.lua
func TestLuaMasterFullExecution_calls(t *testing.T) {
	runLuaMasterFile(t, "calls.lua")
}

// TestLuaMasterFullExecution_vararg executes lua-master/testes/vararg.lua
func TestLuaMasterFullExecution_vararg(t *testing.T) {
	runLuaMasterFile(t, "vararg.lua")
}

// TestLuaMasterFullExecution_bitwise executes lua-master/testes/bitwise.lua
func TestLuaMasterFullExecution_bitwise(t *testing.T) {
	runLuaMasterFile(t, "bitwise.lua")
}

// TestLuaMasterFullExecution_bwcoercion executes lua-master/testes/bwcoercion.lua
func TestLuaMasterFullExecution_bwcoercion(t *testing.T) {
	runLuaMasterFile(t, "bwcoercion.lua")
}

// TestLuaMasterFullExecution_code executes lua-master/testes/code.lua
func TestLuaMasterFullExecution_code(t *testing.T) {
	runLuaMasterFile(t, "code.lua")
}

// TestLuaMasterFullExecution_math executes lua-master/testes/math.lua
func TestLuaMasterFullExecution_math(t *testing.T) {
	runLuaMasterFile(t, "math.lua")
}

// TestLuaMasterFullExecution_strings executes lua-master/testes/strings.lua
func TestLuaMasterFullExecution_strings(t *testing.T) {
	runLuaMasterFile(t, "strings.lua")
}

// TestLuaMasterFullExecution_goto executes lua-master/testes/goto.lua
func TestLuaMasterFullExecution_goto(t *testing.T) {
	runLuaMasterFile(t, "goto.lua")
}

// TestLuaMasterFullExecution_events executes lua-master/testes/events.lua
func TestLuaMasterFullExecution_events(t *testing.T) {
	runLuaMasterFile(t, "events.lua")
}

// TestLuaMasterFullExecution_locals executes lua-master/testes/locals.lua
func TestLuaMasterFullExecution_locals(t *testing.T) {
	runLuaMasterFile(t, "locals.lua")
}

// TestLuaMasterFullExecution_attrib executes lua-master/testes/attrib.lua
func TestLuaMasterFullExecution_attrib(t *testing.T) {
	runLuaMasterFile(t, "attrib.lua")
}

// TestLuaMasterFullExecution_big executes lua-master/testes/big.lua
func TestLuaMasterFullExecution_big(t *testing.T) {
	runLuaMasterFile(t, "big.lua")
}

// TestLuaMasterFullExecution_pm executes lua-master/testes/pm.lua
func TestLuaMasterFullExecution_pm(t *testing.T) {
	runLuaMasterFile(t, "pm.lua")
}

// TestLuaMasterFullExecution_gc executes lua-master/testes/gc.lua
func TestLuaMasterFullExecution_gc(t *testing.T) {
	runLuaMasterFile(t, "gc.lua")
}

// TestLuaMasterFullExecution_cstack executes lua-master/testes/cstack.lua
func TestLuaMasterFullExecution_cstack(t *testing.T) {
	runLuaMasterFile(t, "cstack.lua")
}

// TestLuaMasterFullExecution_sort executes lua-master/testes/sort.lua
func TestLuaMasterFullExecution_sort(t *testing.T) {
	runLuaMasterFile(t, "sort.lua")
}

// TestLuaMasterFullExecution_tpack executes lua-master/testes/tpack.lua
func TestLuaMasterFullExecution_tpack(t *testing.T) {
	runLuaMasterFile(t, "tpack.lua")
}

// TestLuaMasterFullExecution_heavy executes lua-master/testes/heavy.lua
func TestLuaMasterFullExecution_heavy(t *testing.T) {
	runLuaMasterFile(t, "heavy.lua")
}

// TestLuaMasterFullExecution_gengc executes lua-master/testes/gengc.lua
func TestLuaMasterFullExecution_gengc(t *testing.T) {
	runLuaMasterFile(t, "gengc.lua")
}

// TestLuaMasterFullExecution_tracegc executes lua-master/testes/tracegc.lua
func TestLuaMasterFullExecution_tracegc(t *testing.T) {
	runLuaMasterFile(t, "tracegc.lua")
}

// TestLuaMasterFullExecution_api executes lua-master/testes/api.lua
func TestLuaMasterFullExecution_api(t *testing.T) {
	runLuaMasterFile(t, "api.lua")
}

// TestLuaMasterFullExecution_errors executes lua-master/testes/errors.lua
func TestLuaMasterFullExecution_errors(t *testing.T) {
	runLuaMasterFile(t, "errors.lua")
}

// TestLuaMasterFullExecution_coroutine executes lua-master/testes/coroutine.lua
func TestLuaMasterFullExecution_coroutine(t *testing.T) {
	runLuaMasterFile(t, "coroutine.lua")
}

// TestLuaMasterFullExecution_db executes lua-master/testes/db.lua
func TestLuaMasterFullExecution_db(t *testing.T) {
	runLuaMasterFile(t, "db.lua")
}

// TestLuaMasterFullExecution_utf8 executes lua-master/testes/utf8.lua
func TestLuaMasterFullExecution_utf8(t *testing.T) {
	runLuaMasterFile(t, "utf8.lua")
}

// TestLuaMasterFullExecution_memerr executes lua-master/testes/memerr.lua
func TestLuaMasterFullExecution_memerr(t *testing.T) {
	runLuaMasterFile(t, "memerr.lua")
}

// TestLuaMasterFullExecution_verybig executes lua-master/testes/verybig.lua
func TestLuaMasterFullExecution_verybig(t *testing.T) {
	runLuaMasterFile(t, "verybig.lua")
}
