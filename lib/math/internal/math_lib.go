// Package internal implements the Lua math library.
// This package provides implementations for:
//   - math.abs(x): absolute value
//   - math.ceil(x): ceiling
//   - math.floor(x): floor
//   - math.max(x, ...): maximum
//   - math.min(x, ...): minimum
//   - math.sqrt(x): square root
//   - math.pow(x, y): power
//   - math.sin(x), math.cos(x), math.tan(x): trigonometric
//   - math.asin(x), math.acos(x), math.atan(x), math.atan2(y, x): inverse trigonometric
//   - math.log(x [, base]): logarithm
//   - math.log10(x): base-10 logarithm
//   - math.exp(x): exponential
//   - math.deg(x): radians to degrees
//   - math.rad(x): degrees to radians
//   - math.random([n [, m]]): random number
//   - math.randomseed(x): set random seed
//
// Reference: lua-master/lmathlib.c
package internal

import (
	"math"
	"math/rand"

	mathlib "github.com/akzj/go-lua/lib/math/api"
)

// Global RNG for standalone function access
var globalRNG = rand.New(rand.NewSource(0))

// MathLib is the implementation of the Lua math library.
type MathLib struct {
	rng *rand.Rand
}

// NewMathLib creates a new MathLib instance.
func NewMathLib() mathlib.MathLib {
	return &MathLib{
		rng: rand.New(rand.NewSource(0)),
	}
}

// Open implements mathlib.MathLib.Open.
// Registers all math library functions in the global table under "math".
func (m *MathLib) Open(L mathlib.LuaAPI) int {
	// Create "math" table: 0 array elements, 22 predefined fields
	L.CreateTable(0, 22)

	// Register all math functions using PushGoFunction + SetField
	register := func(name string, fn mathlib.LuaFunc) {
		L.PushGoFunction(fn)
		L.SetField(-2, name)
	}

	register("abs", mathAbs)
	register("ceil", mathCeil)
	register("floor", mathFloor)
	register("max", mathMax)
	register("min", mathMin)
	register("sqrt", mathSqrt)
	register("pow", mathPow)
	register("sin", mathSin)
	register("cos", mathCos)
	register("tan", mathTan)
	register("asin", mathAsin)
	register("acos", mathAcos)
	register("atan", mathAtan)
	register("atan2", mathAtan2)
	register("log", mathLog)
	register("log10", mathLog10)
	register("exp", mathExp)
	register("deg", mathDeg)
	register("rad", mathRad)
	register("random", mathRandom)
	register("randomseed", mathRandomseed)

	// luaopen_math convention: return 1 (the module table stays on stack)
	return 1
}

// Ensure MathLib implements MathLib interface
var _ mathlib.MathLib = (*MathLib)(nil)

// Ensure types implement LuaFunc (compile-time check)
var _ mathlib.LuaFunc = mathAbs
var _ mathlib.LuaFunc = mathCeil
var _ mathlib.LuaFunc = mathFloor
var _ mathlib.LuaFunc = mathMax
var _ mathlib.LuaFunc = mathMin
var _ mathlib.LuaFunc = mathSqrt
var _ mathlib.LuaFunc = mathPow
var _ mathlib.LuaFunc = mathSin
var _ mathlib.LuaFunc = mathCos
var _ mathlib.LuaFunc = mathTan
var _ mathlib.LuaFunc = mathAsin
var _ mathlib.LuaFunc = mathAcos
var _ mathlib.LuaFunc = mathAtan
var _ mathlib.LuaFunc = mathAtan2
var _ mathlib.LuaFunc = mathLog
var _ mathlib.LuaFunc = mathLog10
var _ mathlib.LuaFunc = mathExp
var _ mathlib.LuaFunc = mathDeg
var _ mathlib.LuaFunc = mathRad
var _ mathlib.LuaFunc = mathRandom
var _ mathlib.LuaFunc = mathRandomseed

// =============================================================================
// Basic Arithmetic Functions
// =============================================================================

// mathAbs returns the absolute value of x.
// math.abs(x) -> number
func mathAbs(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Abs(x))
	return 1
}

// mathCeil returns the smallest integer greater than or equal to x.
// math.ceil(x) -> number
func mathCeil(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Ceil(x))
	return 1
}

// mathFloor returns the largest integer less than or equal to x.
// math.floor(x) -> number
func mathFloor(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Floor(x))
	return 1
}

// =============================================================================
// Min/Max Functions
// =============================================================================

// mathMax returns the maximum value among arguments.
// math.max(x, ...) -> number
func mathMax(L mathlib.LuaAPI) int {
	n := L.GetTop()
	if n < 1 {
		L.PushNil()
		return 1
	}
	max := toNumber(L, 1)
	for i := 2; i <= n; i++ {
		v := toNumber(L, i)
		if v > max {
			max = v
		}
	}
	L.PushNumber(max)
	return 1
}

// mathMin returns the minimum value among arguments.
// math.min(x, ...) -> number
func mathMin(L mathlib.LuaAPI) int {
	n := L.GetTop()
	if n < 1 {
		L.PushNil()
		return 1
	}
	min := toNumber(L, 1)
	for i := 2; i <= n; i++ {
		v := toNumber(L, i)
		if v < min {
			min = v
		}
	}
	L.PushNumber(min)
	return 1
}

// =============================================================================
// Power and Root Functions
// =============================================================================

// mathSqrt returns the square root of x.
// math.sqrt(x) -> number
func mathSqrt(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Sqrt(x))
	return 1
}

// mathPow returns x raised to the power y.
// math.pow(x, y) -> number
func mathPow(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	y := toNumber(L, 2)
	L.PushNumber(math.Pow(x, y))
	return 1
}

// =============================================================================
// Trigonometric Functions
// =============================================================================

// mathSin returns the sine of x (in radians).
// math.sin(x) -> number
func mathSin(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Sin(x))
	return 1
}

// mathCos returns the cosine of x (in radians).
// math.cos(x) -> number
func mathCos(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Cos(x))
	return 1
}

// mathTan returns the tangent of x (in radians).
// math.tan(x) -> number
func mathTan(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Tan(x))
	return 1
}

// =============================================================================
// Inverse Trigonometric Functions
// =============================================================================

// mathAsin returns the arc sine of x (in radians).
// math.asin(x) -> number
func mathAsin(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Asin(x))
	return 1
}

// mathAcos returns the arc cosine of x (in radians).
// math.acos(x) -> number
func mathAcos(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Acos(x))
	return 1
}

// mathAtan returns the arc tangent of x (in radians).
// math.atan(x) -> number
func mathAtan(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Atan(x))
	return 1
}

// mathAtan2 returns the arc tangent of y/x (in radians).
// math.atan2(y, x) -> number
func mathAtan2(L mathlib.LuaAPI) int {
	y := toNumber(L, 1)
	x := toNumber(L, 2)
	L.PushNumber(math.Atan2(y, x))
	return 1
}

// =============================================================================
// Logarithmic and Exponential Functions
// =============================================================================

// mathLog returns the logarithm of x with optional base.
// math.log(x [, base]) -> number
func mathLog(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	base := optNumber(L, 2, math.E)
	L.PushNumber(math.Log(x) / math.Log(base))
	return 1
}

// mathLog10 returns the base-10 logarithm of x.
// math.log10(x) -> number
func mathLog10(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Log10(x))
	return 1
}

// mathExp returns e raised to the power of x.
// math.exp(x) -> number
func mathExp(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(math.Exp(x))
	return 1
}

// =============================================================================
// Angle Conversion Functions
// =============================================================================

// mathDeg converts angles from radians to degrees.
// math.deg(x) -> number
func mathDeg(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(x * 180 / math.Pi)
	return 1
}

// mathRad converts angles from degrees to radians.
// math.rad(x) -> number
func mathRad(L mathlib.LuaAPI) int {
	x := toNumber(L, 1)
	L.PushNumber(x * math.Pi / 180)
	return 1
}

// =============================================================================
// Random Number Functions
// =============================================================================

// mathRandom returns a pseudo-random number.
//
// math.random() -> [0,1) float
// math.random(n) -> [1,n] integer
// math.random(n, m) -> [n,m] integer
func mathRandom(L mathlib.LuaAPI) int {
	nargs := L.GetTop()

	switch nargs {
	case 0:
		// math.random() -> [0,1)
		L.PushNumber(globalRNG.Float64())
	case 1:
		// math.random(n) -> [1,n]
		n := int(toInteger(L, 1))
		if n < 1 {
			n = 1
		}
		L.PushInteger(int64(globalRNG.Intn(n) + 1))
	case 2:
		// math.random(n, m) -> [n,m]
		n := int(toInteger(L, 1))
		m := int(toInteger(L, 2))
		if n > m {
			n, m = m, n
		}
		L.PushInteger(int64(n + globalRNG.Intn(m-n+1)))
	default:
		L.PushNumber(globalRNG.Float64())
	}
	return 1
}

// mathRandomseed sets the seed for the pseudo-random number generator.
// math.randomseed(x) -> ()
func mathRandomseed(L mathlib.LuaAPI) int {
	seed := int64(toInteger(L, 1))
	globalRNG = rand.New(rand.NewSource(seed))
	// Lua's randomseed doesn't return a value, just sets the seed
	return 0
}

// GetRNG returns the random number generator (for testing purposes)
func (m *MathLib) GetRNG() *rand.Rand {
	return m.rng
}

// SetSeed sets the random seed (for testing purposes)
func (m *MathLib) SetSeed(seed int64) {
	m.rng = rand.New(rand.NewSource(seed))
}

// SetGlobalSeed sets the global RNG seed (for testing purposes)
func SetGlobalSeed(seed int64) {
	globalRNG = rand.New(rand.NewSource(seed))
}

// =============================================================================
// Helper functions (using raw LuaAPI methods)
// =============================================================================

// toNumber extracts a number at the given index, panics if not a number.
func toNumber(L mathlib.LuaAPI, idx int) float64 {
	if !L.IsNumber(idx) {
		L.ErrorMessage()
	}
	n, _ := L.ToNumber(idx)
	return n
}

// optNumber returns number at idx, or def if nil/absent.
func optNumber(L mathlib.LuaAPI, idx int, def float64) float64 {
	if L.IsNoneOrNil(idx) {
		return def
	}
	return toNumber(L, idx)
}

// toInteger extracts an integer at the given index, panics if not an integer.
func toInteger(L mathlib.LuaAPI, idx int) int64 {
	if !L.IsInteger(idx) {
		L.ErrorMessage()
	}
	i, _ := L.ToInteger(idx)
	return i
}
