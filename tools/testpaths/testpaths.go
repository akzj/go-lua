// Package testpaths resolves paths to the reference C Lua toolchain for tools tests.
package testpaths

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	// EnvCLua overrides the path to the reference C Lua 5.5 binary (e.g. lua-master/lua).
	EnvCLua = "GO_LUA_C_LUA"
	// EnvDisasm overrides the path to tools/disasm.lua used by bccompare tests.
	EnvDisasm = "GO_LUA_DISASM_SCRIPT"
)

// ModuleRoot returns the go-lua repository root (directory containing go.mod).
func ModuleRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrInvalid
	}
	// This file is tools/testpaths/testpaths.go
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}

// ReferenceLuaExe returns the path to the C Lua reference binary.
// Resolution order: GO_LUA_C_LUA, then <repo>/lua-master/lua.
func ReferenceLuaExe() (string, error) {
	if p := os.Getenv(EnvCLua); p != "" {
		return filepath.Clean(p), nil
	}
	root, err := ModuleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "lua-master", "lua"), nil
}

// DisasmLuaScript returns the path to tools/disasm.lua.
// Resolution order: GO_LUA_DISASM_SCRIPT, then <repo>/tools/disasm.lua.
func DisasmLuaScript() (string, error) {
	if p := os.Getenv(EnvDisasm); p != "" {
		return filepath.Clean(p), nil
	}
	root, err := ModuleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "tools", "disasm.lua"), nil
}

// FileExists reports whether path exists and is not a directory.
func FileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
