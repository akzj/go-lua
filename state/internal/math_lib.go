// Math library implementation for Lua 5.4/5.5
package internal

import (
	"fmt"
	"math"
	"math/rand"

	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
)

// =============================================================================
// Helpers
// =============================================================================

// checkNumber extracts a numeric argument (integer or float) as float64.
func checkNumber(stack []types.TValue, base int, argn int, fname string) float64 {
	idx := base + argn
	if idx >= len(stack) {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected, got no value)", argn, fname))
	}
	v := stack[idx]
	if v == nil || v.IsNil() {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected, got nil)", argn, fname))
	}
	if v.IsInteger() {
		return float64(v.GetInteger())
	}
	if v.IsFloat() {
		return float64(v.GetFloat())
	}
	luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected, got %s)", argn, fname, luaTypeName(v)))
	return 0
}

// optNumber extracts an optional numeric argument with a default.
func optNumber(stack []types.TValue, base int, argn int, def float64) float64 {
	idx := base + argn
	if idx >= len(stack) {
		return def
	}
	v := stack[idx]
	if v == nil || v.IsNil() {
		return def
	}
	if v.IsInteger() {
		return float64(v.GetInteger())
	}
	if v.IsFloat() {
		return float64(v.GetFloat())
	}
	return def
}

// nArgs returns how many args were passed (excluding the function slot).
// NOTE: This is already defined in string_lib.go — this declaration is a placeholder
// to indicate the function is available in this package.

// =============================================================================
// math.abs(x)
// =============================================================================
func bmathAbs(stack []types.TValue, base int) int {
	idx := base + 1
	if idx >= len(stack) {
		luaErrorString("bad argument #1 to 'abs' (number expected, got no value)")
	}
	v := stack[idx]
	if v.IsInteger() {
		n := v.GetInteger()
		if n < 0 {
			n = -n
		}
		stack[base] = types.NewTValueInteger(n)
		return 1
	}
	x := checkNumber(stack, base, 1, "abs")
	stack[base] = types.NewTValueFloat(types.LuaNumber(math.Abs(x)))
	return 1
}

// =============================================================================
// math.ceil(x)
// =============================================================================
func bmathCeil(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "ceil")
	stack[base] = types.NewTValueInteger(types.LuaInteger(math.Ceil(x)))
	return 1
}

// =============================================================================
// math.floor(x)
// =============================================================================
func bmathFloor(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "floor")
	stack[base] = types.NewTValueInteger(types.LuaInteger(math.Floor(x)))
	return 1
}

// =============================================================================
// math.max(x, ...)
// =============================================================================
func bmathMax(stack []types.TValue, base int) int {
	n := nArgs(stack, base)
	if n < 1 {
		luaErrorString("bad argument #1 to 'max' (value expected)")
	}
	maxVal := checkNumber(stack, base, 1, "max")
	maxIsInt := stack[base+1].IsInteger()
	for i := 2; i <= n; i++ {
		v := checkNumber(stack, base, i, "max")
		if v > maxVal {
			maxVal = v
			maxIsInt = stack[base+i].IsInteger()
		}
	}
	if maxIsInt {
		stack[base] = types.NewTValueInteger(types.LuaInteger(maxVal))
	} else {
		stack[base] = types.NewTValueFloat(types.LuaNumber(maxVal))
	}
	return 1
}

// =============================================================================
// math.min(x, ...)
// =============================================================================
func bmathMin(stack []types.TValue, base int) int {
	n := nArgs(stack, base)
	if n < 1 {
		luaErrorString("bad argument #1 to 'min' (value expected)")
	}
	minVal := checkNumber(stack, base, 1, "min")
	minIsInt := stack[base+1].IsInteger()
	for i := 2; i <= n; i++ {
		v := checkNumber(stack, base, i, "min")
		if v < minVal {
			minVal = v
			minIsInt = stack[base+i].IsInteger()
		}
	}
	if minIsInt {
		stack[base] = types.NewTValueInteger(types.LuaInteger(minVal))
	} else {
		stack[base] = types.NewTValueFloat(types.LuaNumber(minVal))
	}
	return 1
}

// =============================================================================
// math.sin(x)
// =============================================================================
func bmathSin(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "sin")
	stack[base] = types.NewTValueFloat(types.LuaNumber(math.Sin(x)))
	return 1
}

// =============================================================================
// math.cos(x)
// =============================================================================
func bmathCos(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "cos")
	stack[base] = types.NewTValueFloat(types.LuaNumber(math.Cos(x)))
	return 1
}

// =============================================================================
// math.tan(x)
// =============================================================================
func bmathTan(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "tan")
	stack[base] = types.NewTValueFloat(types.LuaNumber(math.Tan(x)))
	return 1
}

// =============================================================================
// math.atan(y [, x])
// =============================================================================
func bmathAtan(stack []types.TValue, base int) int {
	y := checkNumber(stack, base, 1, "atan")
	x := optNumber(stack, base, 2, 1.0)
	stack[base] = types.NewTValueFloat(types.LuaNumber(math.Atan2(y, x)))
	return 1
}

// =============================================================================
// math.sqrt(x)
// =============================================================================
func bmathSqrt(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "sqrt")
	stack[base] = types.NewTValueFloat(types.LuaNumber(math.Sqrt(x)))
	return 1
}

// =============================================================================
// math.exp(x)
// =============================================================================
func bmathExp(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "exp")
	stack[base] = types.NewTValueFloat(types.LuaNumber(math.Exp(x)))
	return 1
}

// =============================================================================
// math.log(x [, base])
// =============================================================================
func bmathLog(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "log")
	n := nArgs(stack, base)
	var result float64
	if n >= 2 {
		b := checkNumber(stack, base, 2, "log")
		if b == 10.0 {
			result = math.Log10(x)
		} else if b == 2.0 {
			result = math.Log2(x)
		} else {
			result = math.Log(x) / math.Log(b)
		}
	} else {
		result = math.Log(x)
	}
	stack[base] = types.NewTValueFloat(types.LuaNumber(result))
	return 1
}

// =============================================================================
// math.fmod(x, y)
// =============================================================================
func bmathFmod(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "fmod")
	y := checkNumber(stack, base, 2, "fmod")
	stack[base] = types.NewTValueFloat(types.LuaNumber(math.Mod(x, y)))
	return 1
}

// =============================================================================
// math.modf(x) — returns integer part and fractional part
// =============================================================================
func bmathModf(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "modf")
	// Special cases: inf and NaN
	if math.IsInf(x, 0) {
		stack[base] = types.NewTValueFloat(types.LuaNumber(x))
		stack[base+1] = types.NewTValueFloat(0.0)
		return 2
	}
	if math.IsNaN(x) {
		stack[base] = types.NewTValueFloat(types.LuaNumber(x))
		stack[base+1] = types.NewTValueFloat(types.LuaNumber(x))
		return 2
	}
	i, f := math.Modf(x)
	// For whole numbers, return integer type for the integer part
	stack[base] = types.NewTValueInteger(types.LuaInteger(i))
	stack[base+1] = types.NewTValueFloat(types.LuaNumber(f))
	return 2
}

// =============================================================================
// math.deg(x) — radians to degrees
// =============================================================================
func bmathDeg(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "deg")
	stack[base] = types.NewTValueFloat(types.LuaNumber(x * 180.0 / math.Pi))
	return 1
}

// =============================================================================
// math.rad(x) — degrees to radians
// =============================================================================
func bmathRad(stack []types.TValue, base int) int {
	x := checkNumber(stack, base, 1, "rad")
	stack[base] = types.NewTValueFloat(types.LuaNumber(x * math.Pi / 180.0))
	return 1
}

// =============================================================================
// math.random([m [, n]])
// =============================================================================
func bmathRandom(stack []types.TValue, base int) int {
	n := nArgs(stack, base)
	switch {
	case n == 0:
		// random float in [0, 1)
		stack[base] = types.NewTValueFloat(types.LuaNumber(rand.Float64()))
		return 1
	case n == 1:
		// random integer in [1, m]
		m := checkNumber(stack, base, 1, "random")
		if m < 1 {
			luaErrorString("bad argument #1 to 'random' (interval is empty)")
		}
		stack[base] = types.NewTValueInteger(types.LuaInteger(rand.Int63n(int64(m)) + 1))
		return 1
	default:
		// random integer in [m, n]
		lo := checkNumber(stack, base, 1, "random")
		hi := checkNumber(stack, base, 2, "random")
		if lo > hi {
			luaErrorString("bad argument #2 to 'random' (interval is empty)")
		}
		r := rand.Int63n(int64(hi)-int64(lo)+1) + int64(lo)
		stack[base] = types.NewTValueInteger(types.LuaInteger(r))
		return 1
	}
}

// =============================================================================
// math.randomseed([x])
// =============================================================================
func bmathRandomseed(stack []types.TValue, base int) int {
	n := nArgs(stack, base)
	if n >= 1 {
		seed := checkNumber(stack, base, 1, "randomseed")
		rand.Seed(int64(seed))
	} else {
		// Lua 5.4+: randomseed() with no args uses a "random" seed
		rand.Seed(0)
	}
	return 0
}

// =============================================================================
// math.type(x) — returns "integer", "float", or false
// =============================================================================
func bmathType(stack []types.TValue, base int) int {
	idx := base + 1
	if idx >= len(stack) {
		luaErrorString("bad argument #1 to 'type' (value expected)")
	}
	v := stack[idx]
	if v == nil || v.IsNil() {
		stack[base] = types.NewTValueBoolean(false)
		return 1
	}
	if v.IsInteger() {
		stack[base] = types.NewTValueString("integer")
		return 1
	}
	if v.IsFloat() {
		stack[base] = types.NewTValueString("float")
		return 1
	}
	stack[base] = types.NewTValueBoolean(false)
	return 1
}

// =============================================================================
// math.tointeger(x) — convert to integer if exact, else nil
// =============================================================================
func bmathTointeger(stack []types.TValue, base int) int {
	idx := base + 1
	if idx >= len(stack) {
		stack[base] = types.NewTValueNil()
		return 1
	}
	v := stack[idx]
	if v == nil || v.IsNil() {
		stack[base] = types.NewTValueNil()
		return 1
	}
	if v.IsInteger() {
		stack[base] = v
		return 1
	}
	if v.IsFloat() {
		f := float64(v.GetFloat())
		i := int64(f)
		if float64(i) == f {
			stack[base] = types.NewTValueInteger(types.LuaInteger(i))
			return 1
		}
	}
	stack[base] = types.NewTValueNil()
	return 1
}

// =============================================================================
// math.ult(m, n) — unsigned less than
// =============================================================================
func bmathUlt(stack []types.TValue, base int) int {
	m := checkNumber(stack, base, 1, "ult")
	n := checkNumber(stack, base, 2, "ult")
	stack[base] = types.NewTValueBoolean(uint64(int64(m)) < uint64(int64(n)))
	return 1
}

// =============================================================================
// registerMathLib populates the math module table with all functions + constants.
// =============================================================================
func registerMathLib(mathMod tableapi.TableInterface) {
	// Functions
	funcs := map[string]func([]types.TValue, int) int{
		"abs":        bmathAbs,
		"ceil":       bmathCeil,
		"floor":      bmathFloor,
		"max":        bmathMax,
		"min":        bmathMin,
		"sin":        bmathSin,
		"cos":        bmathCos,
		"tan":        bmathTan,
		"atan":       bmathAtan,
		"sqrt":       bmathSqrt,
		"exp":        bmathExp,
		"log":        bmathLog,
		"fmod":       bmathFmod,
		"modf":       bmathModf,
		"deg":        bmathDeg,
		"rad":        bmathRad,
		"random":     bmathRandom,
		"randomseed": bmathRandomseed,
		"type":       bmathType,
		"tointeger":  bmathTointeger,
		"ult":        bmathUlt,
	}
	for name, fn := range funcs {
		key := types.NewTValueString(name)
		mathMod.Set(key, &goFuncWrapper{fn: fn})
	}

	// Constants
	constants := map[string]types.TValue{
		"pi":         types.NewTValueFloat(types.LuaNumber(math.Pi)),
		"huge":       types.NewTValueFloat(types.LuaNumber(math.Inf(1))),
		"maxinteger": types.NewTValueInteger(types.LuaInteger(math.MaxInt64)),
		"mininteger": types.NewTValueInteger(types.LuaInteger(math.MinInt64)),
	}
	for name, val := range constants {
		key := types.NewTValueString(name)
		mathMod.Set(key, val)
	}
}
