// Package integration provides end-to-end integration tests for the Lua VM.
package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akzj/go-lua/state"
)

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

	wrapper := `
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

vararg = function(...)
	return {n = select("#", ...), ...}
end

` + string(content)

	err = state.DoString(wrapper)
	if err != nil {
		t.Errorf("failed to execute %s: %v", filename, err)
	}
}

func TestLuaMasterFullExecution_constructs(t *testing.T) {
	runLuaMasterFile(t, "constructs.lua")
}

func TestLuaMasterFullExecution_literals(t *testing.T) {
	runLuaMasterFile(t, "literals.lua")
}

func TestLuaMasterFullExecution_vararg(t *testing.T) {
	runLuaMasterFile(t, "vararg.lua")
}
