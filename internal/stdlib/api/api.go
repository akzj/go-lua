// Package api defines the interface for Lua standard library registration.
//
// Each library is a Go function that registers Lua functions into a state.
// Libraries use the public Go API (internal/api/) to interact with the VM.
//
// Reference: .analysis/09-standard-libraries.md
package api

import (
	luaapi "github.com/akzj/go-lua/internal/api/api"
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
// - OpenIO:        iolib.go (stub)
// - OpenOS:        oslib.go (stub)
// - OpenDebug:     debuglib.go (stub)
// - OpenUTF8:      utf8lib.go (stub)
// - OpenCoroutine: corolib.go (stub)
// - OpenPackage:   packagelib.go (stub)

// OpenAll opens all standard libraries. Convenience function.
func OpenAll(L *luaapi.State) {
	for _, lib := range StandardLibraries() {
		L.Require(lib.Name, luaapi.CFunction(lib.Open), true)
		L.Pop(1) // pop the library table left by Require
	}
}
