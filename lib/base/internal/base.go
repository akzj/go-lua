// Package internal implements the Lua base library.
// This package provides implementations for:
//   - print(): print function
//   - pairs()/ipairs(): iterators
//   - type(): type query
//   - tonumber()/tostring(): type conversion
//   - error(): error handling
//   - pcall(): protected call
//   - assert(): assertion
//   - select(): argument selection
//
// Reference: lua-master/lbaselib.c
package internal

import (
	"fmt"
	"strconv"

	baselib "github.com/akzj/go-lua/lib/base/api"
	luaapi "github.com/akzj/go-lua/api"
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
	// Push global table
	L.PushGlobalTable()

	// Set global _G (points to itself)
	L.PushValue(-1) // copy of global table
	L.SetField(-2, "_G")

	// Set global _VERSION
	L.PushString("Lua 5.4.6")
	L.SetField(-2, "_VERSION")

	// Register base functions directly in global table
	register := func(name string, fn baselib.LuaFunc) {
		L.PushGoFunction(fn)
		L.SetField(-2, name)
	}

	register("print", print)
	register("type", btype)
	register("pairs", pairs)
	register("ipairs", ipairs)
	register("error", error)
	register("pcall", pcall)
	register("assert", assert)
	register("tonumber", tonumber)
	register("tostring", tostring)
	register("select", luaSelect)

	// Pop global table
	L.Pop()

	// Return 1 (module table on stack, which is the global table)
	return 1
}

// Ensure BaseLib implements baselib.BaseLib
var _ baselib.BaseLib = (*BaseLib)(nil)

// Ensure types implement LuaFunc (compile-time check)
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
// Function Implementations
// =============================================================================

// print prints values to stdout.
// print(...) -> void
// Separates values with tabs, adds newline at end.
func print(L baselib.LuaAPI) int {
	n := L.GetTop()
	for i := 1; i <= n; i++ {
		if i > 1 {
			fmt.Print("\t")
		}
		// Convert value to string using tostring logic
		s := toStringValue(L, i)
		fmt.Print(s)
	}
	fmt.Println()
	return 0
}

// toStringValue converts a Lua value at index to its string representation.
func toStringValue(L baselib.LuaAPI, idx int) string {
	// Check for __tostring metamethod first
	if L.GetMetatable(idx) {
		L.PushString("__tostring")
		L.GetTable(-2)
		if L.IsFunction(-1) {
			// Call the metamethod
			L.Insert(-2) // move metamethod below value
			L.Call(1, 1)
			result, ok := L.ToString(-1)
			L.Pop() // pop result
			L.Pop() // pop metatable
			if ok {
				return result
			}
		}
		L.Pop() // pop metatable (or nil if no __tostring)
	}

	// Fall back to type-based conversion
	switch L.Type(idx) {
	case luaapi.LUA_TNIL:
		return "nil"
	case luaapi.LUA_TSTRING:
		s, _ := L.ToString(idx)
		return s
	case luaapi.LUA_TNUMBER:
		if L.IsInteger(idx) {
			i, _ := L.ToInteger(idx)
			return strconv.FormatInt(i, 10)
		}
		n, _ := L.ToNumber(idx)
		return strconv.FormatFloat(n, 'f', -1, 64)
	case luaapi.LUA_TBOOLEAN:
		if L.ToBoolean(idx) {
			return "true"
		}
		return "false"
	case luaapi.LUA_TTABLE:
		return fmt.Sprintf("table: %p", L.ToPointer(idx))
	case luaapi.LUA_TFUNCTION:
		return fmt.Sprintf("function: %p", L.ToPointer(idx))
	case luaapi.LUA_TUSERDATA:
		return fmt.Sprintf("userdata: %p", L.ToPointer(idx))
	case luaapi.LUA_TTHREAD:
		return fmt.Sprintf("thread: %p", L.ToPointer(idx))
	case luaapi.LUA_TLIGHTUSERDATA:
		return fmt.Sprintf("lightuserdata: %p", L.ToPointer(idx))
	default:
		return ""
	}
}

// pairs iterates over table key-value pairs.
// pairs(t) -> iter, t, nil
// Returns a forloop iterator that yields (key, value) pairs.
func pairs(L baselib.LuaAPI) int {
	// Arg 1 must be a table
	if L.Type(1) != luaapi.LUA_TTABLE {
		L.PushString("table expected")
		L.Error()
		return 0
	}

	// Push the iteration function (pairs_iter)
	L.PushGoFunction(pairsIter)
	// Push the table
	L.PushValue(1)
	// Push initial state: nil (start with first key)
	L.PushNil()

	return 3
}

// pairsIter is the iterator function for pairs.
// Called by forloop: iter(state, key) -> key, value
func pairsIter(L baselib.LuaAPI) int {
	// State is at index 2, key is at index 3
	L.PushValue(2) // table
	L.PushValue(3) // current key
	L.Next(1)      // pops key, pushes next key and value

	if L.IsNil(-1) {
		// No more elements
		L.Pop() // pop nil
		return 0
	}

	// Return key and value (key is at -2, value is at -1)
	return 2
}

// ipairs iterates over table integer key-value pairs starting from 1.
// ipairs(t) -> iter, t, 0
// Returns a forloop iterator that yields (index, value) pairs for array indices.
func ipairs(L baselib.LuaAPI) int {
	// Arg 1 must be a table
	if L.Type(1) != luaapi.LUA_TTABLE {
		L.PushString("table expected")
		L.Error()
		return 0
	}

	// Push the iteration function (ipairs_iter)
	L.PushGoFunction(ipairsIter)
	// Push the table
	L.PushValue(1)
	// Push initial state: 0 (start before first index)
	L.PushInteger(0)

	return 3
}

// ipairsIter is the iterator function for ipairs.
// Called by forloop: iter(state, key) -> key, value
// State is the current index, key is previous index.
func ipairsIter(L baselib.LuaAPI) int {
	// State (table) at index 2, previous index at index 3
	idx, _ := L.ToInteger(3)
	idx++

	// Get table[idx]
	L.PushValue(2) // table
	L.PushInteger(idx)
	L.GetI(-2, idx)
	L.Pop() // pop table, leave value

	if L.IsNil(-1) {
		// No more elements
		L.Pop() // pop nil
		return 0
	}

	// Return the new index and value
	L.PushInteger(idx)
	// Swap so index is at -2, value is at -1
	L.Insert(-2)
	return 2
}

// btype returns the type of a value.
// type(v) -> string
func btype(L baselib.LuaAPI) int {
	if L.IsNone(1) {
		L.PushNil()
		return 1
	}
	tp := L.Type(1)
	L.PushString(L.TypeName(tp))
	return 1
}

// tonumber converts a value to a number.
// tonumber(e [, base]) -> number | nil
// If base is given, e must be a string to be parsed in that base.
func tonumber(L baselib.LuaAPI) int {
	if L.IsNoneOrNil(2) {
		// No base specified: try direct conversion
		if L.Type(1) == luaapi.LUA_TNUMBER {
			// Already a number, just return it
			L.PushValue(1)
			return 1
		}
		if L.Type(1) == luaapi.LUA_TSTRING {
			// Try to parse as number
			s, ok := L.ToString(1)
			if !ok {
				L.PushNil()
				return 1
			}
			// Try integer first
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				L.PushInteger(i)
				return 1
			}
			// Try float
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				L.PushNumber(f)
				return 1
			}
			L.PushNil()
			return 1
		}
		L.PushNil()
		return 1
	}

	// Base specified: e must be a string
	if L.Type(1) != luaapi.LUA_TSTRING {
		L.PushNil()
		return 1
	}
	baseVal, baseOk := L.ToInteger(2)
	if !baseOk || baseVal < 2 || baseVal > 36 {
		L.PushNil()
		return 1
	}
	base := int(baseVal)

	s, _ := L.ToString(1)
	val, err := strconv.ParseInt(s, base, 64)
	if err != nil {
		L.PushNil()
		return 1
	}
	L.PushInteger(val)
	return 1
}

// tostring converts a value to a string.
// tostring(v) -> string
// Uses __tostring metamethod if available.
func tostring(L baselib.LuaAPI) int {
	if L.IsNone(1) {
		L.PushNil()
		return 1
	}
	s := toStringValue(L, 1)
	L.PushString(s)
	return 1
}

// error raises a Lua error.
// error(message [, level]) -> never returns
// If level is 1 (default), error points to the caller.
// If level is 0, no location info is added.
func error(L baselib.LuaAPI) int {
	level := int64(1)
	if !L.IsNoneOrNil(2) {
		level, _ = L.ToInteger(2)
	}

	// If message is a string and level > 0, add location info
	if L.Type(1) == luaapi.LUA_TSTRING && level > 0 {
		L.Where(int(level))
		L.Insert(-2) // put location before message
		L.Concat(2)  // concatenate
	}

	// Call L.Error which never returns
	L.Error()
	return 0 // never reached
}

// pcall is a protected call.
// pcall(f, ...) -> status, result...
// Returns true and results if successful, false and error message if failed.
func pcall(L baselib.LuaAPI) int {
	// Move function and args up, so we can use PCall
	nArgs := L.GetTop() - 1 // number of arguments (excluding function)

	// PCall expects: function at top-nArgs, args above it
	// We have: function at 1, args at 2..top
	// So nArgs = top - 1

	// Call PCall with no error handler (errfunc = 0)
	status := L.PCall(nArgs, luaapi.LUA_MULTRET, 0)

	if status == 0 { // 0 = LUA_OK
		// Success: push true and all results
		L.Insert(L.GetTop() + 1) // make room at bottom
		L.PushBoolean(true)
		L.Insert(1)
		return L.GetTop()
	}

	// Error: push false and error message
	L.PushBoolean(false)
	L.PushString(luaapi.StatusString(luaapi.Status(status)))
	return 2
}

// assert checks a condition.
// assert(v [, message]) -> v
// Raises error if v is false or nil.
func assert(L baselib.LuaAPI) int {
	if !L.ToBoolean(1) {
		if L.IsNoneOrNil(2) {
			L.PushString("assertion failed!")
		} else {
			L.PushValue(2)
		}
		L.Error()
		return 0 // never reached
	}
	// Return the original value
	return 1
}

// luaSelect returns values based on arguments.
// select(index, ...) -> values...
// If index is '#', returns the number of variadic arguments.
// Otherwise, returns all arguments from index to end.
func luaSelect(L baselib.LuaAPI) int {
	if L.GetTop() < 1 {
		L.Error()
		return 0
	}

	// Check if first argument is '#'
	if L.Type(1) == luaapi.LUA_TSTRING {
		s, _ := L.ToString(1)
		if s == "#" {
			L.PushInteger(int64(L.GetTop() - 1))
			return 1
		}
	}

	// Get the index
	idx, ok := L.ToInteger(1)
	if !ok {
		L.PushString("bad argument #1 to 'select' (number expected)")
		L.Error()
		return 0
	}

	n := L.GetTop() - 1 // number of variadic arguments

	// Negative index counts from end
	if idx < 0 {
		idx = int64(n) + idx + 1
	}

	if idx < 1 || idx > int64(n) {
		L.PushNil()
		return 1
	}

	// Return values from idx to n
	for i := idx; i <= int64(n); i++ {
		L.PushValue(int(i) + 1) // +1 because 1 is the index arg
	}

	return int(n) - int(idx) + 1
}
