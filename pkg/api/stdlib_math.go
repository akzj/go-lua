// Package api provides the public Lua API
// This file implements the math standard library module
package api

import (
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

	// Register as global
	s.SetGlobal("math")
}

// stdMathAbs implements math.abs(x)
// Returns absolute value of x.
func stdMathAbs(L *State) int {
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
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(math.Ceil(x))
	return 1
}

// stdMathFloor implements math.floor(x)
// Returns largest integer <= x.
func stdMathFloor(L *State) int {
	x, ok := L.ToNumber(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(math.Floor(x))
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

	L.PushNumber(max)
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

	L.PushNumber(min)
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