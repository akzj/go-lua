package lua

import (
	"github.com/akzj/go-lua/internal/api"
)

// ---------------------------------------------------------------------------
// Auxiliary functions (luaL_* equivalents)
// ---------------------------------------------------------------------------

// CheckString checks that the argument at idx is a string and returns it.
// Raises a Lua error if the check fails.
func (L *State) CheckString(idx int) string {
	return L.s.CheckString(idx)
}

// CheckInteger checks that the argument at idx is an integer and returns it.
// Raises a Lua error if the check fails.
func (L *State) CheckInteger(idx int) int64 {
	return L.s.CheckInteger(idx)
}

// CheckNumber checks that the argument at idx is a number and returns it.
// Raises a Lua error if the check fails.
func (L *State) CheckNumber(idx int) float64 {
	return L.s.CheckNumber(idx)
}

// CheckType checks that the argument at idx has the given type.
// Raises a Lua error if the check fails.
func (L *State) CheckType(idx int, tp Type) {
	L.s.CheckType(idx, toInternalType(tp))
}

// CheckAny checks that there is an argument at idx (any type).
// Raises a Lua error if the index is not valid.
func (L *State) CheckAny(idx int) {
	L.s.CheckAny(idx)
}

// OptString returns the string at idx, or def if the argument is nil/absent.
func (L *State) OptString(idx int, def string) string {
	return L.s.OptString(idx, def)
}

// OptInteger returns the integer at idx, or def if the argument is nil/absent.
func (L *State) OptInteger(idx int, def int64) int64 {
	return L.s.OptInteger(idx, def)
}

// OptNumber returns the number at idx, or def if the argument is nil/absent.
func (L *State) OptNumber(idx int, def float64) float64 {
	return L.s.OptNumber(idx, def)
}

// ArgError raises an error for argument arg with the given message.
// This function does not return.
func (L *State) ArgError(arg int, extraMsg string) int {
	return L.s.ArgError(arg, extraMsg)
}

// TypeError raises a type error for argument arg.
// This function does not return.
func (L *State) TypeError(arg int, tname string) int {
	return L.s.TypeError(arg, tname)
}

// Where pushes a string "source:line: " for the given call level.
func (L *State) Where(level int) {
	L.s.Where(level)
}

// Errorf raises a formatted Lua error.
// This function does not return.
func (L *State) Errorf(format string, args ...interface{}) int {
	return L.s.Errorf(format, args...)
}

// SetFuncs registers functions from a map into the table at the top of the stack.
// nUp is the number of upvalues (must be on the stack above the table).
func (L *State) SetFuncs(funcs map[string]Function, nUp int) {
	apiFuncs := make(map[string]api.CFunction, len(funcs))
	for name, fn := range funcs {
		if fn != nil {
			apiFuncs[name] = L.wrapFunction(fn)
		} else {
			apiFuncs[name] = nil
		}
	}
	L.s.SetFuncs(apiFuncs, nUp)
}

// NewLib creates a new table and registers functions into it.
func (L *State) NewLib(funcs map[string]Function) {
	apiFuncs := make(map[string]api.CFunction, len(funcs))
	for name, fn := range funcs {
		if fn != nil {
			apiFuncs[name] = L.wrapFunction(fn)
		} else {
			apiFuncs[name] = nil
		}
	}
	L.s.NewLib(apiFuncs)
}

// Require loads a module. If the module is already in package.loaded,
// pushes the cached value. Otherwise calls openf to load it.
// If global is true, also sets it as a global.
func (L *State) Require(modname string, openf Function, global bool) {
	L.s.Require(modname, L.wrapFunction(openf), global)
}

// NewMetatable creates a new metatable in the registry with the given name.
// If the registry already has a table with that name, pushes it and returns false.
// Otherwise creates a new table, stores it, and returns true.
func (L *State) NewMetatable(tname string) bool {
	return L.s.NewMetatable(tname)
}

// TestUdata checks if the value at idx is a userdata with metatable matching
// registry[tname]. Returns true if it matches.
func (L *State) TestUdata(idx int, tname string) bool {
	return L.s.TestUdata(idx, tname)
}

// CheckUdata checks that the value at idx is a userdata with metatable matching
// registry[tname]. Raises a type error if not.
func (L *State) CheckUdata(idx int, tname string) {
	L.s.CheckUdata(idx, tname)
}

// Ref creates a reference in the table at idx.
// Pops the top value and stores it, returning an integer key.
// If the value is nil, returns RefNil and pops without storing.
func (L *State) Ref(t int) int {
	return L.s.Ref(t)
}

// Unref frees a reference in the table at idx.
func (L *State) Unref(t int, ref int) {
	L.s.Unref(t, ref)
}

// CheckOption checks that the argument at idx is a string matching one of
// the options. Returns the index of the matched option.
// If def is non-empty and the argument is nil/absent, uses def.
func (L *State) CheckOption(idx int, def string, opts []string) int {
	return L.s.CheckOption(idx, def, opts)
}

// LenI returns the length of the value at idx as an integer.
// May trigger __len metamethod. Raises an error if the result is not an integer.
func (L *State) LenI(idx int) int64 {
	return L.s.LenI(idx)
}

// TolString converts the value at idx to a string, using __tostring
// metamethod if present. Pushes the result and returns it.
func (L *State) TolString(idx int) string {
	return L.s.TolString(idx)
}

// ArgCheck checks a condition for argument arg.
// If cond is false, raises an error with extraMsg.
func (L *State) ArgCheck(cond bool, arg int, extraMsg string) {
	L.s.ArgCheck(cond, arg, extraMsg)
}

// ArgExpected checks that argument arg satisfies a condition.
// If cond is false, raises a type error with tname.
func (L *State) ArgExpected(cond bool, arg int, tname string) {
	L.s.ArgExpected(cond, arg, tname)
}
