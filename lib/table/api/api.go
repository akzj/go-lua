// Package api defines the Lua table library interface.
// No implementation details - only interfaces.
//
// Reference: lua-master/ltablib.c (Lua 5.5.1)
//
// Constraint: must NOT import internal/ packages to avoid circular dependency.
package api

import (
	"github.com/akzj/go-lua/api"
	types "github.com/akzj/go-lua/types/api"
)

// LuaAPI mirrors the Lua API interface from api package.
type LuaAPI = api.LuaAPI

// LuaInteger matches types.LuaInteger.
type LuaInteger = types.LuaInteger

// LuaNumber matches types.LuaNumber.
type LuaNumber = types.LuaNumber

// =============================================================================
// Table Library Interface
// =============================================================================

// TableLib provides Lua table library functions (table.insert, table.remove, etc.).
//
// Invariants:
// - Open() registers functions in the global table under "table"
// - Returns 1 (number of values pushed on success), per luaopen_* convention
//
// Design:
// - Uses Go function types directly (not CFunction/unsafe.Pointer)
// - Each function receives LuaAPI for stack access
// - Returns int (number of values pushed)
type TableLib interface {
	// Open opens the table library, registering its functions.
	// L: the Lua state to operate on
	// Returns: number of values pushed onto the stack (always 1 = the module table)
	//
	// Side effects: sets global variable "table" with all table functions
	Open(L LuaAPI) int
}

// LuaFunc is the type for Lua Go functions.
// Matches the signature used in lua.h: typedef int (*lua_CFunction)(lua_State*).
//
// Why not use types.CFunction?
// - types.CFunction is unsafe.Pointer (FFI style)
// - LuaFunc is a proper Go function type for this implementation
type LuaFunc func(L LuaAPI) int

// FunctionRegistration provides methods for registering functions in a table.
// This is how library functions are registered (mirrors luaL_Reg pattern).
//
// Usage pattern:
//   1. L.CreateTable(0, len(funcs))  // create library table
//   2. For each function:
//      - L.PushGoFunction(fn)       // push function onto stack
//      - L.SetField(-2, name)        // set in table
//   3. L.SetGlobal(name)            // set table as global
//
// Why this approach?
// - luaL_newlib uses lua_createtable + luaL_setfuncs
// - We replicate this with CreateTable + PushGoFunction + SetField
type FunctionRegistration interface {
	// CreateTable creates a new empty table on the stack.
	CreateTable(narr, nrec int)

	// PushGoFunction pushes a Go function onto the stack.
	PushGoFunction(fn LuaFunc)

	// SetField sets a table field from the value on top of stack.
	// table[key] = value, then pops value.
	SetField(idx int, key string)

	// SetGlobal sets a global variable and pops it from stack.
	SetGlobal(name string)
}

// =============================================================================
// Function Arity Constants (for table.pack / table.unpack)
// =============================================================================

const (
	// TablePackArity is the arity for table.pack: all arguments are packed
	TablePackArity = -1
	// TableUnpackDefaultStart is the default start index for table.unpack
	TableUnpackDefaultStart = 1
)

// =============================================================================
// Table Operation Flags (for internal use by implementation)
// =============================================================================

const (
	// TAB_R: table supports read operation
	TAB_R = 1
	// TAB_W: table supports write operation
	TAB_W = 2
	// TAB_L: table supports length operation
	TAB_L = 4
	// TAB_RW: table supports read/write operations
	TAB_RW = TAB_R | TAB_W
)

// =============================================================================
// Error Messages (for consistency across implementation)
// =============================================================================

const (
	// ErrInvalidValueAtIndex is the error message for invalid value at index in concat
	ErrInvalidValueAtIndex = "invalid value (%s) at index %d in table for 'concat'"
	// ErrPositionOutOfBounds is the error message for position out of bounds
	ErrPositionOutOfBounds = "position out of bounds"
	// ErrInvalidOrderFunction is the error message for invalid order function in sort
	ErrInvalidOrderFunction = "invalid order function for sorting"
	// ErrArrayTooBig is the error message for array too big
	ErrArrayTooBig = "array too big"
)

// =============================================================================
// Internal Interface (used by table.move to check tables)
// =============================================================================

// TableChecker checks if a value at stack index is a valid table for operations.
// This is used internally by table.move to validate source and destination tables.
type TableChecker interface {
	// CheckTable validates that the value at index is a valid table for given operations.
	// what: combination of TAB_R, TAB_W, TAB_L flags
	// Panics if validation fails.
	CheckTable(idx int, what int)
}
