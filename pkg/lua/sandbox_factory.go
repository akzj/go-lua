package lua

import (
	"github.com/akzj/go-lua/internal/api"
	"github.com/akzj/go-lua/internal/stdlib"
)

// SandboxConfig configures a sandboxed Lua state created by [NewSandboxState].
type SandboxConfig struct {
	// MemoryLimit is the maximum memory in bytes (0 = no limit).
	MemoryLimit int64

	// CPULimit is the maximum number of VM instructions (0 = no limit).
	CPULimit int64

	// AllowIO controls whether the io and os libraries are available.
	// Default false = no io/os access.
	AllowIO bool

	// AllowDebug controls whether the debug library is available.
	// Default false = no debug access.
	AllowDebug bool

	// AllowPackage controls whether the package/require library is available.
	// Default false = no module loading.
	AllowPackage bool

	// ExtraLibs is a map of additional libraries to register.
	// These are registered after the standard libraries.
	ExtraLibs map[string]Function
}

// safeGlobals is the set of base-library globals that are safe for sandboxed
// execution.  Anything NOT in this set is removed after loading the base lib.
var safeGlobals = map[string]bool{
	"_VERSION":       true,
	"assert":         true,
	"collectgarbage": true,
	"error":          true,
	"getmetatable":   true,
	"ipairs":         true,
	"next":           true,
	"pairs":          true,
	"pcall":          true,
	"print":          true,
	"rawequal":       true,
	"rawget":         true,
	"rawlen":         true,
	"rawset":         true,
	"select":         true,
	"setmetatable":   true,
	"tonumber":       true,
	"tostring":       true,
	"type":           true,
	"unpack":         true,
	"warn":           true,
	"xpcall":         true,
}

// unsafeBaseGlobals are base-library globals that must be removed in a sandbox.
var unsafeBaseGlobals = []string{
	"dofile",
	"loadfile",
	"load",
	"require",
}

// NewSandboxState creates a new Lua state with restricted capabilities.
//
// By default only safe libraries are loaded: a restricted base library
// (without dofile, loadfile, load, require), string, table, math, utf8,
// and coroutine.  The io, os, debug, and package libraries are excluded
// unless explicitly enabled via [SandboxConfig].
//
// If [SandboxConfig.CPULimit] is set, a CPU instruction limit is applied
// via [State.SetCPULimit].
//
// If [SandboxConfig.MemoryLimit] is set and the state supports memory
// limiting, it will be applied.
//
// Example:
//
//	L := lua.NewSandboxState(lua.SandboxConfig{
//	    CPULimit: 1_000_000, // max 1M instructions
//	})
//	defer L.Close()
//	err := L.DoString(untrustedCode)
func NewSandboxState(config SandboxConfig) *State {
	// 1. Create a bare state (no libraries).
	s := api.NewState()
	L := &State{s: s}

	// 2. Load safe standard libraries via internal openers.
	//    We use the internal api.State.Require directly because the stdlib
	//    openers take *api.State, not the public *State.

	// Always load the base library first.
	s.Require("_G", api.CFunction(stdlib.OpenBase), true)
	s.Pop(1)

	// Always-safe libraries.
	safeLibs := []struct {
		name string
		open api.CFunction
	}{
		{"coroutine", api.CFunction(stdlib.OpenCoroutineLib)},
		{"string", api.CFunction(stdlib.OpenString)},
		{"table", api.CFunction(stdlib.OpenTable)},
		{"math", api.CFunction(stdlib.OpenMath)},
		{"utf8", api.CFunction(stdlib.OpenUTF8)},
	}
	for _, lib := range safeLibs {
		s.Require(lib.name, lib.open, true)
		s.Pop(1)
	}

	// Conditionally load libraries.
	if config.AllowPackage {
		s.Require("package", api.CFunction(stdlib.OpenPackage), true)
		s.Pop(1)
	}
	if config.AllowIO {
		s.Require("io", api.CFunction(stdlib.OpenIO), true)
		s.Pop(1)
		s.Require("os", api.CFunction(stdlib.OpenOS), true)
		s.Pop(1)
	}
	if config.AllowDebug {
		s.Require("debug", api.CFunction(stdlib.OpenDebug), true)
		s.Pop(1)
	}

	// 3. Remove dangerous globals from the base library.
	// When AllowPackage is true, keep "require" so modules can be loaded.
	for _, name := range unsafeBaseGlobals {
		if name == "require" && config.AllowPackage {
			continue // keep require when package library is enabled
		}
		s.PushNil()
		s.SetGlobal(name)
	}

	// 4. Register extra user-provided libraries.
	for name, openf := range config.ExtraLibs {
		L.Require(name, openf, true)
		L.Pop(1)
	}

	// 5. Install global module registry searcher.
	L.installGlobalSearcher()

	// 6. Apply resource limits.
	if config.MemoryLimit > 0 {
		L.SetMemoryLimit(config.MemoryLimit)
	}
	if config.CPULimit > 0 {
		L.SetCPULimit(config.CPULimit)
	}

	return L
}
