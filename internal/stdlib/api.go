// Package api defines the interface for Lua standard library registration.
//
// Each library is a Go function that registers Lua functions into a state.
// Libraries use the public Go API (internal/api/) to interact with the VM.
//
// Reference: .analysis/09-standard-libraries.md
package stdlib

import (
	luaapi "github.com/akzj/go-lua/internal/api"
)

// OpenFunc is the type for library opener functions.
// Each takes a State and registers its functions. Returns 1 (the library table).
type OpenFunc func(L *luaapi.State) int

// Library describes a standard library for registration.
type Library struct {
	Name string   // library name (e.g., "string", "table")
	Open OpenFunc // opener function
}

// StandardLibraries returns the list of all standard libraries in load order.
// Order matters: base must be first (it defines _G and core functions).
func StandardLibraries() []Library {
	return []Library{
		{"_G", OpenBase},
		{"package", OpenPackage},
		{"coroutine", OpenCoroutineLib},
		{"debug", OpenDebug},
		{"io", OpenIO},
		{"math", OpenMath},
		{"os", OpenOS},
		{"string", OpenString},
		{"table", OpenTable},
		{"utf8", OpenUTF8},
	}
}

// Individual Library Openers are defined in their respective files:
// - OpenBase:      baselib.go
// - OpenTable:     tablelib.go
// - OpenMath:      mathlib.go
// - OpenString:    stringlib.go
// - OpenIO:        iolib.go
// - OpenOS:        oslib.go
// - OpenDebug:     debuglib.go
// - OpenUTF8:      utf8lib.go
// - OpenCoroutine: corolib.go
// - OpenPackage:   packagelib.go
// - OpenConsole:   consolelib.go (preloaded, use require("console"))

// OpenAll opens all standard libraries. Convenience function.
func OpenAll(L *luaapi.State) {
	for _, lib := range StandardLibraries() {
		L.Require(lib.Name, luaapi.CFunction(lib.Open), true)
		L.Pop(1) // pop the library table left by Require
	}

	// Preload test-helper modules into package.preload so that
	// require("tracegc") works without a .lua file on disk.
	preloadModules := map[string]luaapi.CFunction{
		"tracegc": luaapi.CFunction(OpenTraceGC),
		"console": luaapi.CFunction(OpenConsole),
	}
	L.GetGlobal("package")          // push package table
	L.GetField(-1, "preload")       // push package.preload
	for name, opener := range preloadModules {
		L.PushCFunction(opener)
		L.SetField(-2, name)        // package.preload[name] = opener
	}
	L.Pop(2) // pop preload + package
}
