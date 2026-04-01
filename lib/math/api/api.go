// Package api defines the Lua math library interface.
// No implementation details - only interfaces.
//
// Reference: lua-master/lmathlib.c
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
// Math Library Interface
// =============================================================================

// MathLib provides Lua math library functions (math.abs, math.sin, etc.).
//
// Invariants:
// - Open() registers functions in the global table under "math"
// - Returns 1 (number of values pushed on success), per luaopen_* convention
//
// Design:
// - Uses Go function types directly (not CFunction/unsafe.Pointer)
// - Each function receives LuaAPI for stack access
// - Returns int (number of values pushed)
type MathLib interface {
	// Open opens the math library, registering its functions.
	// L: the Lua state to operate on
	// Returns: number of values pushed onto the stack (always 1 = the module table)
	//
	// Side effects: sets global variable "math" with all math functions
	Open(L LuaAPI) int
}

// LuaFunc is the type for Lua Go functions.
// Matches the signature used in lua.h: typedef int (*lua_CFunction)(lua_State*).
//
// Why not use types.CFunction?
// - types.CFunction is unsafe.Pointer (FFI style)
// - LuaFunc is a proper Go function type for this implementation
type LuaFunc func(L LuaAPI) int

// =============================================================================
// Math Function Signatures
// =============================================================================
// All functions follow the pattern: func(L LuaAPI) int returning number of
// results pushed onto the stack.
//
// Pushing results:
// - For integer results: use L.PushInteger(i) or L.PushNumber(float64(i))
// - For float results: use L.PushNumber(float64) — NOT raw float
//
// Why not use L.PushFloat()?
// - Lua doesn't have a separate float type; numbers are unified
// - Use math.Float() helper only when converting LuaInteger to LuaNumber
//
// math.random special handling:
// - math.random() -> [0,1) using rand.Float64()
// - math.random(n) -> [1,n] using rand.Intn(int(n)) + 1
// - math.random(n,m) -> [n,m] using rand.Intn(int(m-n+1)) + int(n)