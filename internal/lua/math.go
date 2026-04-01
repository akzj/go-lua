package lua

/*
** $Id: math.go $
** Standard mathematical library
** Ported from lmathlib.c
*/

import (
	"math"
	"math/rand"
	"unsafe"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lauxlib"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** Constants
 */
var (
	pi    = math.Pi
	huge  = math.MaxFloat64
	nan   = math.NaN()
)

func init() {
	rand.Seed(1)
}

/*
** Helper functions
*/

// luaB_abs - absolute value
func math_abs(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	if lapi.Lua_isinteger(LS, 1) != 0 {
		n := lapi.Lua_tointeger(LS, 1)
		if n < 0 {
			lapi.Lua_pushinteger(LS, -n)
			return 1
		}
		lapi.Lua_pushinteger(LS, n)
		return 1
	}
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, math.Abs(float64(n)))
	return 1
}

// luaB_sin - sine
func math_sin(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Sin(float64(n))))
	return 1
}

// luaB_cos - cosine
func math_cos(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Cos(float64(n))))
	return 1
}

// luaB_tan - tangent
func math_tan(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Tan(float64(n))))
	return 1
}

// luaB_asin - arcsine
func math_asin(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Asin(float64(n))))
	return 1
}

// luaB_acos - arccosine
func math_acos(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Acos(float64(n))))
	return 1
}

// luaB_atan - arctangent
func math_atan(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	y := lapi.Lua_tonumberx(LS, 1, nil)
	var x lobject.LuaNumber = 1
	if lapi.Lua_gettop(LS) >= 2 {
		x = lapi.Lua_tonumberx(LS, 2, nil)
	}
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Atan2(float64(y), float64(x))))
	return 1
}

// luaB_sinh - hyperbolic sine
func math_sinh(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Sinh(float64(n))))
	return 1
}

// luaB_cosh - hyperbolic cosine
func math_cosh(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Cosh(float64(n))))
	return 1
}

// luaB_tanh - hyperbolic tangent
func math_tanh(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Tanh(float64(n))))
	return 1
}

// luaB_asinh - inverse hyperbolic sine
func math_asinh(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Asinh(float64(n))))
	return 1
}

// luaB_acosh - inverse hyperbolic cosine
func math_acosh(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Acosh(float64(n))))
	return 1
}

// luaB_atanh - inverse hyperbolic tangent
func math_atanh(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Atanh(float64(n))))
	return 1
}

// luaB_floor - floor
func math_floor(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	if lapi.Lua_isinteger(LS, 1) != 0 {
		return 1 // Already an integer
	}
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Floor(float64(n))))
	return 1
}

// luaB_ceil - ceiling
func math_ceil(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	if lapi.Lua_isinteger(LS, 1) != 0 {
		return 1 // Already an integer
	}
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Ceil(float64(n))))
	return 1
}

// luaB_sqrt - square root
func math_sqrt(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Sqrt(float64(n))))
	return 1
}

// luaB_log - logarithm
func math_log(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	x := lapi.Lua_tonumberx(LS, 1, nil)
	base := math.E
	if lapi.Lua_gettop(LS) >= 2 {
		base = float64(lapi.Lua_tonumberx(LS, 2, nil))
	}
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Log(float64(x))/math.Log(base)))
	return 1
}

// luaB_exp - exponential
func math_exp(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Exp(float64(n))))
	return 1
}

// luaB_deg - convert radians to degrees
func math_deg(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, n*180.0/math.Pi)
	return 1
}

// luaB_rad - convert degrees to radians
func math_rad(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	lapi.Lua_pushnumber(LS, n*math.Pi/180.0)
	return 1
}

// luaB_min - minimum
func math_min(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_gettop(LS)
	if n < 1 {
		lauxlib.LuaL_error(LS, "wrong number of arguments")
	}
	
	// Check if all are integers
	allInts := true
	for i := 1; i <= n; i++ {
		if lapi.Lua_isinteger(LS, i) == 0 {
			allInts = false
			break
		}
	}
	
	if allInts {
		minVal := lapi.Lua_tointeger(LS, 1)
		for i := 2; i <= n; i++ {
			v := lapi.Lua_tointeger(LS, i)
			if v < minVal {
				minVal = v
			}
		}
		lapi.Lua_pushinteger(LS, minVal)
	} else {
		minVal := float64(lapi.Lua_tonumberx(LS, 1, nil))
		for i := 2; i <= n; i++ {
			v := float64(lapi.Lua_tonumberx(LS, i, nil))
			if v < minVal {
				minVal = v
			}
		}
		lapi.Lua_pushnumber(LS, lobject.LuaNumber(minVal))
	}
	return 1
}

// luaB_max - maximum
func math_max(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_gettop(LS)
	if n < 1 {
		lauxlib.LuaL_error(LS, "wrong number of arguments")
	}
	
	// Check if all are integers
	allInts := true
	for i := 1; i <= n; i++ {
		if lapi.Lua_isinteger(LS, i) == 0 {
			allInts = false
			break
		}
	}
	
	if allInts {
		maxVal := lapi.Lua_tointeger(LS, 1)
		for i := 2; i <= n; i++ {
			v := lapi.Lua_tointeger(LS, i)
			if v > maxVal {
				maxVal = v
			}
		}
		lapi.Lua_pushinteger(LS, maxVal)
	} else {
		maxVal := float64(lapi.Lua_tonumberx(LS, 1, nil))
		for i := 2; i <= n; i++ {
			v := float64(lapi.Lua_tonumberx(LS, i, nil))
			if v > maxVal {
				maxVal = v
			}
		}
		lapi.Lua_pushnumber(LS, lobject.LuaNumber(maxVal))
	}
	return 1
}

// luaB_fmod - modulo for floats
func math_fmod(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	a := lapi.Lua_tonumberx(LS, 1, nil)
	b := lapi.Lua_tonumberx(LS, 2, nil)
	if b == 0 {
		lauxlib.LuaL_error(LS, "zero divisor")
	}
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(math.Mod(float64(a), float64(b))))
	return 1
}

// luaB_modf - integer and fractional parts
func math_modf(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_tonumberx(LS, 1, nil)
	i := math.Floor(float64(n))
	f := float64(n) - i
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(i))
	lapi.Lua_pushnumber(LS, lobject.LuaNumber(f))
	return 2
}

// luaB_random - random number
func math_random(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	low := lauxlib.LuaL_optinteger(LS, 1, 1)
	high := lauxlib.LuaL_optinteger(LS, 2, 0)
	
	if high < low {
		lauxlib.LuaL_error(LS, "interval is empty")
	}
	
	if high == 0 {
		lapi.Lua_pushnumber(LS, lobject.LuaNumber(rand.Float64()))
	} else {
		n := low + lobject.LuaInteger(rand.Int63n(int64(high-low+1)))
		lapi.Lua_pushinteger(LS, n)
	}
	return 1
}

// luaB_randomseed - seed the random generator
func math_randomseed(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	seed := lauxlib.LuaL_checkinteger(LS, 1)
	rand.Seed(int64(seed))
	lapi.Lua_pushinteger(LS, seed)
	return 1
}

// luaB_tointeger - convert to integer
func math_tointeger(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	i := lapi.Lua_tointegerx(LS, 1, nil)
	lapi.Lua_pushinteger(LS, i)
	return 1
}

// math_type - get number type info
func math_type(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	if lapi.Lua_isinteger(LS, 1) != 0 {
		lapi.Lua_pushstring(LS, "integer")
	} else {
		lapi.Lua_pushstring(LS, "float")
	}
	return 1
}

/*
** Math library functions
*/
var mathlibs = []lauxlib.LuaL_Reg{
	{"abs", math_abs},
	{"sin", math_sin},
	{"cos", math_cos},
	{"tan", math_tan},
	{"asin", math_asin},
	{"acos", math_acos},
	{"atan", math_atan},
	{"sinh", math_sinh},
	{"cosh", math_cosh},
	{"tanh", math_tanh},
	{"asinh", math_asinh},
	{"acosh", math_acosh},
	{"atanh", math_atanh},
	{"floor", math_floor},
	{"ceil", math_ceil},
	{"sqrt", math_sqrt},
	{"log", math_log},
	{"exp", math_exp},
	{"deg", math_deg},
	{"rad", math_rad},
	{"min", math_min},
	{"max", math_max},
	{"fmod", math_fmod},
	{"modf", math_modf},
	{"random", math_random},
	{"randomseed", math_randomseed},
	{"tointeger", math_tointeger},
	{"type", math_type},
}

/*
** OpenMath - open math library
 */
func OpenMath(L *lstate.LuaState) {
	lauxlib.LuaL_newlib(L, mathlibs)
	lapi.Lua_pushnumber(L, pi)
	lapi.Lua_setfield(L, -2, "pi")
	lapi.Lua_pushnumber(L, huge)
	lapi.Lua_setfield(L, -2, "huge")
	lapi.Lua_pushinteger(L, math.MaxInt64)
	lapi.Lua_setfield(L, -2, "maxinteger")
	lapi.Lua_pushinteger(L, math.MinInt64)
	lapi.Lua_setfield(L, -2, "mininteger")
}
