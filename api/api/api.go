// Package api provides Lua C API compatible interfaces for Go.
// This package mirrors lua.h and lauxlib.h from the Lua 5.5.1 C implementation.
//
// Reference: lua-master/lua.h, lua-master/lauxlib.h, lua-master/lapi.c
//
// Design constraints:
// - All public types are interfaces (no implementations)
// - Implementations live in api/internal/
// - Dependencies: state (LuaStateInterface), gc, vm, types, table, mem
//
// Why this module exists:
// - lib/ standard library needs Lua VM access
// - C API compatible interface makes porting Lua C code easier
// - Clean separation: api is the public face, internal has the messy details
package api

import (
	memapi "github.com/akzj/go-lua/mem/api"
	stateapi "github.com/akzj/go-lua/state/api"
	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
)

// =============================================================================
// Type Constants (mirrors lua.h)
// =============================================================================

// Lua basic types
const (
	LUA_TNONE          = -1 // special "no value" type
	LUA_TNIL           = types.LUA_TNIL
	LUA_TBOOLEAN       = types.LUA_TBOOLEAN
	LUA_TLIGHTUSERDATA = types.LUA_TLIGHTUSERDATA
	LUA_TNUMBER        = types.LUA_TNUMBER
	LUA_TSTRING        = types.LUA_TSTRING
	LUA_TTABLE         = types.LUA_TTABLE
	LUA_TFUNCTION      = types.LUA_TFUNCTION
	LUA_TUSERDATA      = types.LUA_TUSERDATA
	LUA_TTHREAD        = types.LUA_TTHREAD
	LUA_NUMTYPES       = types.LUA_NUMTYPES
)

// Lua status codes (from lua.h)
type Status = stateapi.Status

const (
	LUA_OK        Status = 0
	LUA_YIELD     Status = 1
	LUA_ERRRUN    Status = 2
	LUA_ERRSYNTAX Status = 3
	LUA_ERRMEM    Status = 4
	LUA_ERRERR    Status = 5
)

// Arithmetic operators (for lua_arith)
const (
	LUA_OPADD  = 0 // ORDER TM, ORDER OP
	LUA_OPSUB  = 1
	LUA_OPMUL  = 2
	LUA_OPMOD  = 3
	LUA_OPPOW  = 4
	LUA_OPDIV  = 5
	LUA_OPIDIV = 6
	LUA_OPBAND = 7
	LUA_OPBOR  = 8
	LUA_OPBXOR = 9
	LUA_OPSHL  = 10
	LUA_OPSHR  = 11
	LUA_OPUNM  = 12
	LUA_OPBNOT = 13
)

// Comparison operators (for lua_compare)
const (
	LUA_OPEQ = 0
	LUA_OPLT = 1
	LUA_OPLE = 2
)

// GC control (for lua_gc - lua.h LUA_GC* constants)
const (
	LUA_GCSTOP     = 0
	LUA_GCRESTART  = 1
	LUA_GCCOLLECT  = 2
	LUA_GCCOUNT    = 3
	LUA_GCCOUNTB   = 4
	LUA_GCSTEP     = 5
	LUA_GCISRUNNING = 6
)

// Special constants
const (
	LUA_MULTRET         = -1
	LUA_REGISTRYINDEX   = stateapi.LUA_REGISTRYINDEX
	LUA_RIDX_GLOBALS    = 2
	LUA_RIDX_MAINTHREAD = 3
	LUA_RIDX_LAST       = 3
	LUA_MINSTACK        = 20
)

// =============================================================================
// Lua State (lua_State)
// =============================================================================

// LuaAPI is the main Lua API interface, mirroring lua.h.
//
// Invariants:
// - All stack operations maintain: 0 <= top <= stacksize
// - Index conversion: positive = from bottom, negative = from top
// - Pseudo-indices allow access to registry and upvalues
//
// Why embed LuaStateInterface?
// - LuaStateInterface provides thread management
// - LuaAPI adds C API compatible methods
// - Separation allows testing each independently
type LuaAPI interface {
	// NewThread creates a new thread (coroutine).
	// Returns LuaAPI to avoid circular dependency.
	NewThread() LuaAPI

	// =====================================================================
	// Basic Stack Operations (lua.h lua_gettop, lua_settop, etc.)
	// =====================================================================

	// AbsIndex converts a possibly-relative index to an absolute index.
	//
	// Why need this?
	// - Positive: already absolute (stack bottom-relative)
	// - Negative: relative to top (e.g., -1 = top, -2 = below top)
	// - Pseudo: registry, upvalues
	AbsIndex(idx int) int

	// Rotate rotates the stack elements between idx and top.
	// n > 0: elements move n positions up (toward top)
	// n < 0: elements move |n| positions down (toward bottom)
	//
	// Why this operation?
	// - Implements lua_insert, lua_remove, lua_replace
	// - Needed for argument shuffling in varargs
	Rotate(idx int, n int)

	// Copy copies the value at fromidx to toidx.
	Copy(fromidx, toidx int)

	// CheckStack ensures there are at least n free slots in the stack.
	// Returns true if successful, false if would need to grow beyond limits.
	//
	// Why check before growing?
	// - lua_checkstack prevents stack overflow attacks
	// - Lua protects against excessive stack growth
	CheckStack(n int) bool

	// XMove moves n values from one state to another.
	// Used for coroutine operations and Lua-C interop.
	XMove(to LuaAPI, n int)

	// =====================================================================
	// Type Checking (lua_is*, lua_type, lua_typename)
	// =====================================================================

	// Type returns the type of the value at idx, or LUA_TNONE if invalid.
	Type(idx int) int

	// TypeName returns the name of the type.
	// Note: This is a static method in C, but instance method in Go for convenience.
	TypeName(tp int) string

	// IsNone(idx int) returns true if index is not a valid stack position.
	IsNone(idx int) bool

	// IsNil(idx int) returns true if value at idx is nil.
	IsNil(idx int) bool

	// IsNoneOrNil returns true if index is none or nil.
	IsNoneOrNil(idx int) bool

	// IsBoolean returns true if value at idx is boolean.
	IsBoolean(idx int) bool

	// IsString returns true if value at idx is string or number (coercible).
	// Note: Lua allows numbers to be used as strings without explicit conversion.
	IsString(idx int) bool

	// IsFunction returns true if value at idx is a function.
	IsFunction(idx int) bool

	// IsTable returns true if value at idx is a table.
	IsTable(idx int) bool

	// IsLightUserData returns true if value at idx is light userdata.
	IsLightUserData(idx int) bool

	// IsThread returns true if value at idx is a thread.
	IsThread(idx int) bool

	// IsInteger returns true if value at idx is an integer (not float).
	IsInteger(idx int) bool

	// IsNumber returns true if value at idx is a number (int or float).
	IsNumber(idx int) bool

	// =====================================================================
	// Value Conversion (lua_to*, lua_push*)
	// =====================================================================

	// ToInteger extracts integer value at idx. Returns 0 if not an integer.
	ToInteger(idx int) (int64, bool)

	// ToNumber extracts number value at idx. Returns 0 if not a number.
	ToNumber(idx int) (float64, bool)

	// ToString extracts string at idx. Returns empty if not a string.
	// Sets *len to string length if len != nil.
	ToString(idx int) (string, bool)

	// ToBoolean returns the boolean value at idx.
	// Follows Lua semantics: false and nil are false, everything else is true.
	ToBoolean(idx int) bool

	// ToPointer returns a pointer for debugging purposes.
	// Not useful for actual program logic.
	ToPointer(idx int) interface{}

	// ToThread returns the thread at idx.
	ToThread(idx int) LuaAPI

	// PushNil pushes nil onto the stack.
	PushNil()

	// PushInteger pushes an integer onto the stack.
	PushInteger(n int64)

	// PushNumber pushes a number onto the stack.
	PushNumber(n float64)

	// PushString pushes a string onto the stack.
	PushString(s string)

	// PushBoolean pushes a boolean onto the stack.
	PushBoolean(b bool)

	// PushLightUserData pushes light userdata (a pointer) onto the stack.
	// Light userdata has no metatable and is not managed by GC.
	PushLightUserData(p interface{})

	// =====================================================================
	// Table Operations (lua_get*, lua_set*)
	// =====================================================================

	// GetTable gets value at table[key], pushes result onto stack.
	// idx: stack index of table
	// Returns the type of the pushed value.
	GetTable(idx int) int

	// GetField gets value at table.key, pushes result onto stack.
	// idx: stack index of table
	// k: field name
	// Returns the type of the pushed value.
	GetField(idx int, k string) int

	// GetI gets value at table[n], pushes result onto stack.
	// idx: stack index of table
	// n: integer key
	// Returns the type of the pushed value.
	GetI(idx int, n int64) int

	// RawGet is like GetTable but without metamethods.
	RawGet(idx int) int

	// RawGetI gets value at table[n] without metamethods.
	RawGetI(idx int, n int64) int

	// CreateTable creates a new table and pushes it onto stack.
	// narr: suggested size for array part
	// nrec: suggested size for record part
	CreateTable(narr, nrec int)

	// SetTable pops key and value from stack, sets table[key] = value.
	// idx: stack index of table
	// Uses metamethods.
	SetTable(idx int)

	// SetField sets table.key = value (value on stack).
	// idx: stack index of table
	// k: field name
	// Uses metamethods.
	SetField(idx int, k string)

	// SetI sets table[n] = value (value on stack).
	// idx: stack index of table
	// n: integer key
	// Uses metamethods.
	SetI(idx int, n int64)

	// RawSet is like SetTable but without metamethods.
	RawSet(idx int)

	// RawSetI sets table[n] = value without metamethods.
	RawSetI(idx int, n int64)

	// GetGlobal is shorthand for GetTable with global table.
	// Pushes the global value onto the stack.
	GetGlobal(name string) int

	// SetGlobal sets a global variable.
	// Pops value from stack.
	SetGlobal(name string)

	// =====================================================================
	// Metatable Operations
	// =====================================================================

	// GetMetatable returns true if the value at idx has a metatable.
	// If it does, pushes the metatable onto the stack.
	GetMetatable(idx int) bool

	// SetMetatable pops metatable from stack and sets it on value at idx.
	SetMetatable(idx int)

	// =====================================================================
	// Call Operations (lua_call, lua_pcall)
	// =====================================================================

	// Call calls a function.
	// nArgs: number of arguments (function + args on stack)
	// nResults: number of expected results, or LUA_MULTRET for variable
	//
	// Pre: function at top-nArgs, args above it
	// Post: function and args consumed, results pushed
	//
	// Why separate Call from state.Call?
	// - state.Call is minimal (VM needs basic suspend/resume)
	// - LuaAPI.Call provides full semantics (error handling, metamethods)
	Call(nArgs, nResults int)

	// PCall is like Call but with error handling.
	// errfunc: stack index of error handler (0 = no handler)
	// Returns LUA_OK on success, or error code on failure.
	PCall(nArgs, nResults, errfunc int) int

	// =====================================================================
	// Error Handling
	// =====================================================================

	// Error raises a Lua error.
	// The error value is the value at the top of the stack.
	// This function never returns (longjmp in C).
	Error() int

	// ErrorMessage raises an error with the value at the top of the stack.
	// This is like Error() but keeps the error message on the stack.
	ErrorMessage() int

	// Where prepends location information to the error message.
	// lvl: call level (1 = where the current function is)
	Where(level int)

	// =====================================================================
	// GC Control
	// =====================================================================

	// GC controls the garbage collector.
	// what: GC_* constant
	// args: additional arguments (varies by what)
	// Returns additional result (varies by what).
	GC(what int, args ...int) int

	// =====================================================================
	// Miscellaneous
	// =====================================================================

	// Next pops key from stack, pushes next key-value pair from table at idx.
	// Returns true if there is a next pair, false if iteration is complete.
	// If there is a next pair: pushes key then value.
	Next(idx int) bool

	// Concat concatenates n values from the top of the stack.
	// Pops n values, pushes their concatenation.
	// Panics if any non-string/non-number value is involved.
	Concat(n int)

	// Len pushes the length of the value at idx onto the stack.
	// Works with __len metamethod.
	Len(idx int)

	// Compare compares two values.
	// eq/lt/le: comparison operators
	// Returns true if comparison succeeds, false if not (or on error).
	Compare(idx1, idx2 int, op int) bool

	// RawLen returns the raw length of the object at idx.
	// For strings: byte length
	// For tables: array part size (without metamethods)
	// For userdata: size in bytes
	RawLen(idx int) uint

	// =====================================================================
	// Registry Access (lua.h pseudo-indices)
	// =====================================================================

	// Registry returns the registry table.
	Registry() tableapi.TableInterface

	// Ref pops a value from the stack and creates a reference to it.
	// Returns the reference id. Use lua_unref to release.
	// t must be a table (usually the registry).
	// LUA_NOREF = -2, LUA_REFNIL = -1
	Ref(t tableapi.TableInterface) int

	// UnRef releases a reference created by Ref.
	UnRef(t tableapi.TableInterface, ref int)
}

// =============================================================================
// luaL_* Auxiliary Functions (lauxlib.h)
// =============================================================================

// LuaLib is a collection of auxiliary functions for building Lua libraries.
// Mirrors the luaL_* functions from lauxlib.h.
//
// Why separate from LuaAPI?
// - luaL_* are convenience/wrapper functions
// - LuaAPI is the core C API
// - luaL_* can be implemented in terms of LuaAPI
type LuaLib interface {
	// NewState creates a new Lua state with standard allocator.
	NewState() LuaAPI

	// NewStateWithAllocator creates a new Lua state with custom allocator.
	NewStateWithAllocator(alloc memapi.Allocator) LuaAPI

	// Register registers a function in the global table.
	Register(name string, fn types.CFunction)

	// LoadString loads a string as a Lua chunk.
	// Returns error status if compilation fails.
	LoadString(code string) (LuaAPI, error)

	// DoString compiles and executes a string.
	// Returns error if compilation or execution fails.
	DoString(code string) error

	// LoadBuffer loads a buffer as a Lua chunk.
	// name: chunk name for error messages
	// mode: "t" for text, "b" for binary, "bt" for both
	LoadBuffer(buff []byte, name, mode string) (LuaAPI, error)

	// DoBuffer loads and executes a buffer.
	DoBuffer(buff []byte, name string) error

	// CheckInteger checks that argument arg is an integer.
	// Panics with error message if not.
	CheckInteger(L LuaAPI, arg int) int64

	// OptInteger returns arg if present, or default value.
	OptInteger(L LuaAPI, arg int, def int64) int64

	// CheckNumber checks that argument arg is a number.
	// Panics with error message if not.
	CheckNumber(L LuaAPI, arg int) float64

	// OptNumber returns arg if present, or default value.
	OptNumber(L LuaAPI, arg int, def float64) float64

	// CheckString checks that argument arg is a string.
	CheckString(L LuaAPI, arg int) string

	// OptString returns arg if present, or default value.
	OptString(L LuaAPI, arg int, def string) string

	// CheckAny checks that argument arg exists (any type).
	// Panics with error message if not.
	CheckAny(L LuaAPI, arg int)

	// CheckType panics if arg is not of expected type.
	CheckType(L LuaAPI, arg, t int)

	// ArgError raises an error for a bad argument.
	ArgError(L LuaAPI, arg int, extraMsg string) int

	// TypeError raises a type error for argument arg.
	TypeError(L LuaAPI, arg int, tname string) int

	// NewMetatable creates a new metatable and pushes it onto the stack.
	// Returns true if created, false if it already existed.
	NewMetatable(tname string) bool

	// SetMetatableByName sets a metatable by name (from registry).
	SetMetatableByName(L LuaAPI, objIdx int, tname string)

	// TestUData checks if value at idx is userdata of name.
	// Returns nil if not.
	TestUData(L LuaAPI, idx int, tname string) interface{}

	// CheckUData checks that value at idx is userdata of name.
	// Panics if not.
	CheckUData(L LuaAPI, idx int, tname string) interface{}

	// LoadFile loads a file as a Lua chunk.
	LoadFile(filename string) (LuaAPI, error)

	// DoFile loads and executes a file.
	DoFile(filename string) error

	// GSub performs global substitution, returns new string.
	// Replaces pattern p with r in s.
	GSub(s, p, r string) string

	// Where prepends location to error message (stack level).
	Where(L LuaAPI, level int)

	// Error raises a formatted error.
	Error(L LuaAPI, fmt string, args ...interface{}) int

	// CallMeta calls a metamethod.
	// obj: index of object
	// event: metamethod name (e.g., "__add")
	// Returns true if metamethod was called and pushed a result.
	CallMeta(L LuaAPI, obj int, event string) bool

	// Len pushes the length of value at idx.
	// Works with __len metamethod.
	Len(L LuaAPI, idx int) int64

	// GetMetaField pushes a field from the metatable.
	// Returns true if metatable exists and field was pushed.
	GetMetaField(L LuaAPI, obj int, e string) bool

	// Openlibs opens all standard libraries.
	OpenLibs(L LuaAPI)

	// RequireF calls a function as a module loader.
	RequireF(L LuaAPI, modname string, openf types.CFunction, glb bool)

	// NewLib creates a new library table from a list of functions.
	NewLib(L LuaAPI, regs []LuaL_Reg)

	// SetFuncs registers functions in a table.
	// nup: number of upvalues each function receives.
	SetFuncs(L LuaAPI, regs []LuaL_Reg, nup int)

	// NewLibTable creates a new table with capacity for functions.
	NewLibTable(L LuaAPI, regs []LuaL_Reg) tableapi.TableInterface
}

// LuaL_Reg represents a function registration (mirrors lauxlib.h luaL_Reg).
type LuaL_Reg struct {
	Name string
	Func types.CFunction
}

// =============================================================================
// Default API Implementation Access
// =============================================================================

// DefaultLuaAPI is the default LuaAPI instance.
// Initialized by internal.init() before user code runs.
var DefaultLuaAPI LuaAPI

// DefaultLuaLib is the default LuaLib implementation.
var DefaultLuaLib LuaLib

// New creates a new Lua state with default allocator.
func New() LuaAPI {
	return DefaultLuaAPI
}

// NewWithAllocator creates a new Lua state with custom allocator.
func NewWithAllocator(alloc memapi.Allocator) LuaAPI {
	return DefaultLuaLib.NewStateWithAllocator(alloc)
}

// =============================================================================
// Helper Functions (stateless, pure)
// =============================================================================

// Typename returns the name of a Lua type.
// This is a package-level function (lua_typename in C).
func Typename(tp int) string {
	switch tp {
	case LUA_TNIL:
		return "nil"
	case LUA_TBOOLEAN:
		return "boolean"
	case LUA_TLIGHTUSERDATA:
		return "userdata"
	case LUA_TNUMBER:
		return "number"
	case LUA_TSTRING:
		return "string"
	case LUA_TTABLE:
		return "table"
	case LUA_TFUNCTION:
		return "function"
	case LUA_TUSERDATA:
		return "userdata"
	case LUA_TTHREAD:
		return "thread"
	default:
		return "no value"
	}
}

// StatusString returns a human-readable status message.
func StatusString(status Status) string {
	switch status {
	case LUA_OK:
		return "OK"
	case LUA_YIELD:
		return "Yield"
	case LUA_ERRRUN:
		return "Runtime error"
	case LUA_ERRSYNTAX:
		return "Syntax error"
	case LUA_ERRMEM:
		return "Memory error"
	case LUA_ERRERR:
		return "Error in error handler"
	default:
		return "Unknown error"
	}
}
