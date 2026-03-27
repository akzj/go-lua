// Package api provides the public Lua API
// This file implements the math standard library module
package api

import (
	"github.com/akzj/go-lua/pkg/object"
	"math"
	"math/rand"
	"time"
)

// init seeds the random number generator
func init() {
	rand.Seed(time.Now().UnixNano())
}

// openMathLib registers the math module
func (s *State) openMathLib() {
	// Create module table
	s.NewTable()
	tableIdx := s.GetTop()

	// Register functions
	funcs := map[string]Function{
		"abs":    stdMathAbs,
		"ceil":   stdMathCeil,
		"floor":  stdMathFloor,
		"max":    stdMathMax,
		"min":    stdMathMin,
		"sqrt":   stdMathSqrt,
		"pow":    stdMathPow,
		"random": stdMathRandom,
		"sin":    stdMathSin,
		"cos":    stdMathCos,
		"tan":    stdMathTan,
		"log":    stdMathLog,
		"exp":    stdMathExp,
		"asin":   stdMathAsin,
		"acos":   stdMathAcos,
		"atan":   stdMathAtan,
		"deg":    stdMathDeg,
		"rad":    stdMathRad,
		"modf":   stdMathModf,
		"fmod":   stdMathFmod,
		"ult":       stdMathUlt,
		"tointeger": stdMathToInteger,
		"type":      stdMathType,
	}

	for name, fn := range funcs {
		s.PushFunction(fn)
		s.SetField(tableIdx, name)
	}

	// Register constants
	s.PushNumber(math.Pi)
	s.SetField(tableIdx, "pi")

	s.PushNumber(math.Inf(1)) // +Inf for math.huge
	s.SetField(tableIdx, "huge")

	// Integer limits (Lua 5.3+)
	s.PushInteger(math.MinInt64)
	s.SetField(tableIdx, "mininteger")

	s.PushInteger(math.MaxInt64)
	s.SetField(tableIdx, "maxinteger")

	// Register as global
	s.SetGlobal("math")
}

// stdMathAbs implements math.abs(x)
// Returns absolute value of x.
func stdMathAbs(L *State) int {
	v := L.vm.GetStack(1)
	if v.Type == object.TypeNumber && v.IsInt {
		// Input is integer: return absolute value as integer
		val := v.Value.Int
		if val < 0 {
			val = -val
		}
		L.PushInteger(val)
		return 1
	}
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(math.Abs(x))
	return 1
}

// stdMathCeil implements math.ceil(x)
// Returns smallest integer >= x.
func stdMathCeil(L *State) int {
	v := L.vm.GetStack(1)
	if v.Type == object.TypeNumber && v.IsInt {
		// Input is integer: ceil of integer is itself
		L.PushInteger(v.Value.Int)
		return 1
	}
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	result := math.Ceil(x)
	// Ceil always returns an integer value
	L.PushInteger(int64(result))
	return 1
}

// stdMathFloor implements math.floor(x)
// Returns largest integer <= x.
func stdMathFloor(L *State) int {
	v := L.vm.GetStack(1)
	if v.Type == object.TypeNumber && v.IsInt {
		// Input is integer: floor of integer is itself
		L.PushInteger(v.Value.Int)
		return 1
	}
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	result := math.Floor(x)
	// Floor always returns an integer value
	L.PushInteger(int64(result))
	return 1
}

// stdMathMax implements math.max(x, ...)
// Returns maximum of all arguments.
func stdMathMax(L *State) int {
	top := L.GetTop()
	if top == 0 {
		L.PushNumber(0)
		return 1
	}

	// Check if all inputs are integers
	allInt := true
	for i := 1; i <= top; i++ {
		v := L.vm.GetStack(i)
		if v.Type != object.TypeNumber || !v.IsInt {
			allInt = false
			break
		}
	}

	max, ok := L.ToNumber(1)
	if !ok {
		max = 0
	}

	for i := 2; i <= top; i++ {
		if num, ok := L.ToNumber(i); ok {
			if num > max {
				max = num
			}
		}
	}

	if allInt {
		L.PushInteger(int64(max))
	} else {
		L.PushNumber(max)
	}
	return 1
}

// stdMathMin implements math.min(x, ...)
// Returns minimum of all arguments.
func stdMathMin(L *State) int {
	top := L.GetTop()
	if top == 0 {
		L.PushNumber(0)
		return 1
	}

	// Check if all inputs are integers
	allInt := true
	for i := 1; i <= top; i++ {
		v := L.vm.GetStack(i)
		if v.Type != object.TypeNumber || !v.IsInt {
			allInt = false
			break
		}
	}

	min, ok := L.ToNumber(1)
	if !ok {
		min = 0
	}

	for i := 2; i <= top; i++ {
		if num, ok := L.ToNumber(i); ok {
			if num < min {
				min = num
			}
		}
	}

	if allInt {
		L.PushInteger(int64(min))
	} else {
		L.PushNumber(min)
	}
	return 1
}

// stdMathSqrt implements math.sqrt(x)
// Returns square root of x.
func stdMathSqrt(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(math.Sqrt(x))
	return 1
}

// stdMathPow implements math.pow(x, y)
// Returns x raised to power y.
func stdMathPow(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		x = 0
	}
	y, ok := L.ToNumber(2)
	if !ok {
		y = 0
	}
	L.PushNumber(math.Pow(x, y))
	return 1
}

// stdMathRandom implements math.random([m [, n]])
// Returns random number:
//   - random() -> [0, 1)
//   - random(n) -> [1, n]
//   - random(m, n) -> [m, n]
func stdMathRandom(L *State) int {
	top := L.GetTop()

	if top == 0 {
		// random() -> [0, 1)
		L.PushNumber(rand.Float64())
		return 1
	}

	if top == 1 {
		// random(n) -> [1, n]
		n, ok := L.ToNumber(1)
		if !ok || n < 1 {
			L.PushNumber(0)
			return 1
		}
		L.PushNumber(float64(rand.Intn(int(n)) + 1))
		return 1
	}

	// random(m, n) -> [m, n]
	m, ok := L.ToNumber(1)
	if !ok {
		m = 1
	}
	n, ok := L.ToNumber(2)
	if !ok {
		n = 1
	}

	if n < m {
		L.PushNumber(0)
		return 1
	}

	range_ := n - m + 1
	result := float64(rand.Intn(int(range_))) + m
	L.PushNumber(result)
	return 1
}

// stdMathSin implements math.sin(x)
// Returns sine of x (in radians).
func stdMathSin(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(math.Sin(x))
	return 1
}

// stdMathCos implements math.cos(x)
// Returns cosine of x (in radians).
func stdMathCos(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(math.Cos(x))
	return 1
}

// stdMathTan implements math.tan(x)
// Returns tangent of x (in radians).
func stdMathTan(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(math.Tan(x))
	return 1
}

// stdMathLog implements math.log(x [, base])
// Returns logarithm of x. If base is provided, uses that base.
func stdMathLog(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	
	if L.GetTop() >= 2 {
		base, ok := L.ToNumber(2)
		if ok && base > 0 && base != 1 {
			L.PushNumber(math.Log(x) / math.Log(base))
			return 1
		}
	}
	
	L.PushNumber(math.Log(x))
	return 1
}

// stdMathExp implements math.exp(x)
// Returns e^x.
func stdMathExp(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(1)
		return 1
	}
	L.PushNumber(math.Exp(x))
	return 1
}

// stdMathAsin implements math.asin(x)
// Returns arcsine of x in radians.
func stdMathAsin(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(math.Asin(x))
	return 1
}

// stdMathAcos implements math.acos(x)
// Returns arccosine of x in radians.
func stdMathAcos(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(math.Acos(x))
	return 1
}

// stdMathAtan implements math.atan(y [, x])
// Returns arctangent. If x is provided, returns atan2(y, x).
func stdMathAtan(L *State) int {
	y, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	
	if L.GetTop() >= 2 {
		x, ok := L.ToNumber(2)
		if ok {
			L.PushNumber(math.Atan2(y, x))
			return 1
		}
	}
	
	L.PushNumber(math.Atan(y))
	return 1
}

// stdMathDeg implements math.deg(x)
// Converts radians to degrees.
func stdMathDeg(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(x * 180 / math.Pi)
	return 1
}

// stdMathRad implements math.rad(x)
// Converts degrees to radians.
func stdMathRad(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(x * math.Pi / 180)
	return 1
}

// stdMathModf implements math.modf(x)
// Returns integer and fractional parts of x.
func stdMathModf(L *State) int {
	v := L.vm.GetStack(1)
	if v.Type == object.TypeNumber && v.IsInt {
		// Input is integer: return (integer, 0.0 float)
		L.PushInteger(v.Value.Int)
		L.PushNumber(0.0)
		return 2
	}
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		L.PushNumber(0)
		return 2
	}
	intPart := math.Trunc(x)
	fracPart := x - intPart
	L.PushNumber(intPart)
	L.PushNumber(fracPart)
	return 2
}

// stdMathFmod implements math.fmod(x, y)
// Returns remainder of x/y.
func stdMathFmod(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	y, ok := L.ToNumber(2)
	if !ok || y == 0 {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(math.Mod(x, y))
	return 1
}

// stdMathUlt implements math.ult(m, n)
// Returns true if m < n using unsigned comparison.
func stdMathUlt(L *State) int {
	m, ok := L.ToNumber(1)
	if !ok {
		L.PushBoolean(false)
		return 1
	}
	n, ok := L.ToNumber(2)
	if !ok {
		L.PushBoolean(false)
		return 1
	}
	// Convert to uint64 for unsigned comparison
	L.PushBoolean(uint64(m) < uint64(n))
	return 1
}// stdMathToInteger implements math.tointeger(x)
// Returns x if it is an integer value, otherwise nil.
// Since the VM uses float64 for all numbers, this checks if the number
// has no fractional part.
// Returns nil for NaN and infinity values as they cannot be represented as integers.
func stdMathToInteger(L *State) int {
	v := L.vm.GetStack(1)
	if v.Type == object.TypeNumber && v.IsInt {
		// Already an integer
		L.PushInteger(v.Value.Int)
		return 1
	}
	x, ok := L.ToNumber(1)
	if !ok {
		// Not a number
		L.PushNil()
		return 1
	}
	
	// NaN and infinity cannot be represented as integers
	if math.IsNaN(x) || math.IsInf(x, 0) {
		L.PushNil()
		return 1
	}
	
	// Check if x is an integer value (no fractional part)
	if x == math.Trunc(x) {
		L.PushInteger(int64(x))
		return 1
	}
	
	// Has fractional part, return nil
	L.PushNil()
	return 1
}

// stdMathType implements math.type(x)
// Returns "integer" if x is an integer, "float" if x is a float, nil otherwise.
func stdMathType(L *State) int {
	v := L.vm.GetStack(1)
	if v.Type == object.TypeNumber {
		if v.IsInt {
			L.PushString("integer")
		} else {
			L.PushString("float")
		}
		return 1
	}
	
	// Not a number
	L.PushNil()
	return 1
}