// Package api defines the Lua base library interface.
// No implementation details - only interfaces.
//
// Reference: lua-master/lbaselib.c
//
// Constraint: must NOT import internal/ packages to avoid circular dependency.
package api

import (
	"github.com/akzj/go-lua/api"
	types "github.com/akzj/go-lua/types/api"
)

// LuaAPI mirrors the Lua API interface from api/api package.
type LuaAPI = api.LuaAPI

// LuaInteger matches types.LuaInteger.
type LuaInteger = types.LuaInteger

// LuaNumber matches types.LuaNumber.
type LuaNumber = types.LuaNumber

// =============================================================================
// Base Library Interface
// =============================================================================

// BaseLib provides Lua base library functions (print, pairs, ipairs, type,
// tonumber, tostring, error, pcall, etc.).
//
// Invariants:
// - Open() registers functions in the global table (_G)
// - Returns 1 (number of values pushed on success), per luaopen_* convention
//
// Design:
// - Uses Go function types directly (not CFunction/unsafe.Pointer)
// - Each function receives LuaAPI for stack access
// - Returns int (number of values pushed)
type BaseLib interface {
	// Open opens the base library, registering its functions.
	// L: the Lua state to operate on
	// Returns: number of values pushed onto the stack (always 1 = the module table)
	//
	// Side effects: sets global variables _G, _VERSION, and all base functions
	Open(L LuaAPI) int
}

// LuaFunc is the type for Lua Go functions.
// Matches the signature used in lua.h: typedef int (*lua_CFunction)(lua_State*).
//
// Why not use types.CFunction?
// - types.CFunction is unsafe.Pointer (FFI style)
// - LuaFunc is a proper Go function type for this implementation
type LuaFunc func(L LuaAPI) int
