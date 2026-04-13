package api

import (
	"math"
	"math/rand"

	luaapi "github.com/akzj/go-lua/internal/api/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
)

// ---------------------------------------------------------------------------
// Math library
// Reference: lua-master/lmathlib.c
// ---------------------------------------------------------------------------

func math_abs(L *luaapi.State) int {
	if L.IsInteger(1) {
		n := L.CheckInteger(1)
		if n < 0 {
			n = -n
		}
		L.PushInteger(n)
	} else {
		L.PushNumber(math.Abs(L.CheckNumber(1)))
	}
	return 1
}

func math_sin(L *luaapi.State) int {
	L.PushNumber(math.Sin(L.CheckNumber(1)))
	return 1
}

func math_cos(L *luaapi.State) int {
	L.PushNumber(math.Cos(L.CheckNumber(1)))
	return 1
}

func math_tan(L *luaapi.State) int {
	L.PushNumber(math.Tan(L.CheckNumber(1)))
	return 1
}

func math_asin(L *luaapi.State) int {
	L.PushNumber(math.Asin(L.CheckNumber(1)))
	return 1
}

func math_acos(L *luaapi.State) int {
	L.PushNumber(math.Acos(L.CheckNumber(1)))
	return 1
}

func math_atan(L *luaapi.State) int {
	y := L.CheckNumber(1)
	x := L.OptNumber(2, 1.0)
	L.PushNumber(math.Atan2(y, x))
	return 1
}

func math_ceil(L *luaapi.State) int {
	if L.IsInteger(1) {
		L.SetTop(1) // integer already
	} else {
		L.PushInteger(int64(math.Ceil(L.CheckNumber(1))))
	}
	return 1
}

func math_floor(L *luaapi.State) int {
	if L.IsInteger(1) {
		L.SetTop(1)
	} else {
		L.PushInteger(int64(math.Floor(L.CheckNumber(1))))
	}
	return 1
}

func math_fmod(L *luaapi.State) int {
	if L.IsInteger(1) && L.IsInteger(2) {
		d := L.CheckInteger(2)
		if d == 0 {
			L.ArgCheck(false, 2, "zero")
		}
		// Use Go % which has same semantics as C fmod for integers
		L.PushInteger(L.CheckInteger(1) % d)
	} else {
		L.PushNumber(math.Mod(L.CheckNumber(1), L.CheckNumber(2)))
	}
	return 1
}

func math_sqrt(L *luaapi.State) int {
	L.PushNumber(math.Sqrt(L.CheckNumber(1)))
	return 1
}

func math_log(L *luaapi.State) int {
	x := L.CheckNumber(1)
	var res float64
	if L.IsNoneOrNil(2) {
		res = math.Log(x)
	} else {
		base := L.CheckNumber(2)
		if base == 2.0 {
			res = math.Log2(x)
		} else if base == 10.0 {
			res = math.Log10(x)
		} else {
			res = math.Log(x) / math.Log(base)
		}
	}
	L.PushNumber(res)
	return 1
}

func math_exp(L *luaapi.State) int {
	L.PushNumber(math.Exp(L.CheckNumber(1)))
	return 1
}

func math_max(L *luaapi.State) int {
	n := L.GetTop()
	L.ArgCheck(n >= 1, 1, "value expected")
	imax := 1
	for i := 2; i <= n; i++ {
		if L.Compare(imax, i, luaapi.OpLT) {
			imax = i
		}
	}
	L.PushValue(imax)
	return 1
}

func math_min(L *luaapi.State) int {
	n := L.GetTop()
	L.ArgCheck(n >= 1, 1, "value expected")
	imin := 1
	for i := 2; i <= n; i++ {
		if L.Compare(i, imin, luaapi.OpLT) {
			imin = i
		}
	}
	L.PushValue(imin)
	return 1
}

func math_tointeger(L *luaapi.State) int {
	if L.IsInteger(1) {
		L.SetTop(1)
	} else {
		n := L.CheckNumber(1)
		i := int64(n)
		if float64(i) == n {
			L.PushInteger(i)
		} else {
			L.PushFail()
		}
	}
	return 1
}

func math_type(L *luaapi.State) int {
	if L.Type(1) == objectapi.TypeNumber {
		if L.IsInteger(1) {
			L.PushString("integer")
		} else {
			L.PushString("float")
		}
	} else {
		L.CheckAny(1)
		L.PushFail()
	}
	return 1
}

func math_random(L *luaapi.State) int {
	switch L.GetTop() {
	case 0: // no arguments: random float in [0,1)
		L.PushNumber(rand.Float64())
	case 1: // upper limit
		u := L.CheckInteger(1)
		L.ArgCheck(1 <= u, 1, "interval is empty")
		L.PushInteger(int64(rand.Intn(int(u))) + 1)
	default: // lower and upper limits
		lo := L.CheckInteger(1)
		up := L.CheckInteger(2)
		L.ArgCheck(lo <= up, 2, "interval is empty")
		r := up - lo + 1
		L.PushInteger(int64(rand.Intn(int(r))) + lo)
	}
	return 1
}

func math_randomseed(L *luaapi.State) int {
	if L.IsNoneOrNil(1) {
		rand.Seed(0) // reset
	} else {
		rand.Seed(L.CheckInteger(1))
	}
	return 0
}

// math_modf implements math.modf(x) — returns integral and fractional parts.
func math_modf(L *luaapi.State) int {
	if L.IsInteger(1) {
		L.PushInteger(L.CheckInteger(1)) // integer: integral = x
		L.PushNumber(0.0)                // fractional = 0.0
	} else {
		x := L.CheckNumber(1)
		if math.IsInf(x, 0) {
			L.PushNumber(x)   // integral = ±inf
			L.PushNumber(0.0) // fractional = 0.0 (C Lua behavior)
		} else if math.IsNaN(x) {
			L.PushNumber(x) // NaN
			L.PushNumber(x) // NaN
		} else {
			i, f := math.Modf(x)
			L.PushNumber(i)
			L.PushNumber(f)
		}
	}
	return 2
}

// math_deg implements math.deg(x) — radians to degrees.
func math_deg(L *luaapi.State) int {
	L.PushNumber(L.CheckNumber(1) * (180.0 / math.Pi))
	return 1
}

// math_rad implements math.rad(x) — degrees to radians.
func math_rad(L *luaapi.State) int {
	L.PushNumber(L.CheckNumber(1) * (math.Pi / 180.0))
	return 1
}

// math_ult implements math.ult(m, n) — unsigned integer comparison.
func math_ult(L *luaapi.State) int {
	m := L.CheckInteger(1)
	n := L.CheckInteger(2)
	L.PushBoolean(uint64(m) < uint64(n))
	return 1
}

// OpenMath opens the math library.
func OpenMath(L *luaapi.State) int {
	mathFuncs := map[string]luaapi.CFunction{
		"abs":        math_abs,
		"acos":       math_acos,
		"asin":       math_asin,
		"atan":       math_atan,
		"ceil":       math_ceil,
		"cos":        math_cos,
		"deg":        math_deg,
		"exp":        math_exp,
		"floor":      math_floor,
		"fmod":       math_fmod,
		"log":        math_log,
		"max":        math_max,
		"min":        math_min,
		"modf":       math_modf,
		"rad":        math_rad,
		"random":     math_random,
		"randomseed": math_randomseed,
		"sin":        math_sin,
		"sqrt":       math_sqrt,
		"tan":        math_tan,
		"tointeger":  math_tointeger,
		"type":       math_type,
		"ult":        math_ult,
	}
	L.NewLib(mathFuncs)

	// Constants
	L.PushNumber(math.Pi)
	L.SetField(-2, "pi")

	L.PushInteger(math.MaxInt64)
	L.SetField(-2, "maxinteger")
	L.PushInteger(math.MinInt64)
	L.SetField(-2, "mininteger")
	L.PushNumber(math.Inf(1))
	L.SetField(-2, "huge")

	return 1
}
