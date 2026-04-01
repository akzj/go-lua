// Package internal implements the Lua base library.
// This package provides implementations for:
//   - print(): print function
//   - pairs()/ipairs(): iterators
//   - type(): type query
//   - tonumber()/tostring(): type conversion
//   - error(): error handling
//   - pcall(): protected call
//
// Reference: lua-master/lbaselib.c
package internal

import (
	baselib "github.com/akzj/go-lua/lib/base/api"
)

// =============================================================================
// BaseLib Implementation
// =============================================================================

// BaseLib is the implementation of the Lua base library.
type BaseLib struct{}

// NewBaseLib creates a new BaseLib instance.
func NewBaseLib() baselib.BaseLib {
	return &BaseLib{}
}

// Open implements baselib.BaseLib.Open.
// Registers all base library functions in the global table.
func (b *BaseLib) Open(L baselib.LuaAPI) int {
	// Set global _G
	L.PushGlobalTable()
	L.PushValue(-1) // copy of global table
	L.SetField(-2, "_G")

	// Set global _VERSION
	L.PushString("Lua 5.5.1")
	L.SetField(-2, "_VERSION")

	// Set base library functions
	// Note: Functions are registered via SetGlobal for each name
	// TODO: Implement full registration with luaL_setfuncs equivalent

	L.Pop() // pop global table

	// Return 1 (module table)
	return 1
}

// Ensure types implement LuaFunc
var _ baselib.LuaFunc = print
var _ baselib.LuaFunc = pairs
var _ baselib.LuaFunc = ipairs
var _ baselib.LuaFunc = btype
var _ baselib.LuaFunc = tonumber
var _ baselib.LuaFunc = tostring
var _ baselib.LuaFunc = error
var _ baselib.LuaFunc = pcall
var _ baselib.LuaFunc = assert
var _ baselib.LuaFunc = luaSelect

// =============================================================================
// Function Implementations (Stubs)
// =============================================================================

// print prints values to stdout.
// print(...) -> void
func print(L baselib.LuaAPI) int {
	n := L.GetTop()
	for i := 1; i <= n; i++ {
		if i > 1 {
			// Tab separator between values
		}
		if L.IsString(i) {
			s, _ := L.ToString(i)
			_ = s // TODO: output s
		}
	}
	return 0
}

// pairs iterates over table key-value pairs.
// pairs(t) -> iter, t, nil
func pairs(L baselib.LuaAPI) int {
	panic("TODO: implement pairs")
}

// ipairs iterates over table integer key-value pairs.
// ipairs(t) -> iter, t, 0
func ipairs(L baselib.LuaAPI) int {
	panic("TODO: implement ipairs")
}

// btype returns the type of a value.
// type(v) -> string
func btype(L baselib.LuaAPI) int {
	panic("TODO: implement type")
}

// tonumber converts a value to a number.
// tonumber(e [, base]) -> number | nil
func tonumber(L baselib.LuaAPI) int {
	panic("TODO: implement tonumber")
}

// tostring converts a value to a string.
// tostring(v) -> string
func tostring(L baselib.LuaAPI) int {
	panic("TODO: implement tostring")
}

// error raises a Lua error.
// error(message [, level]) -> never returns
func error(L baselib.LuaAPI) int {
	panic("TODO: implement error")
}

// pcall is a protected call.
// pcall(f, ...) -> status, result...
func pcall(L baselib.LuaAPI) int {
	panic("TODO: implement pcall")
}

// assert checks a condition.
// assert(v [, message]) -> v
func assert(L baselib.LuaAPI) int {
	panic("TODO: implement assert")
}

// luaSelect returns values based on arguments.
// select(index, ...) -> values...
// Named luaSelect to avoid Go's reserved 'select' keyword.
func luaSelect(L baselib.LuaAPI) int {
	panic("TODO: implement select")
}
