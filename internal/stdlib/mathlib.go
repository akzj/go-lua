package stdlib

import (
	"math"
	"math/bits"
	"time"

	luaapi "github.com/akzj/go-lua/internal/api"
	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// xoshiro256** PRNG — exact port of C Lua 5.5 lmathlib.c (64-bit path)
// ---------------------------------------------------------------------------

// ranState holds the four 64-bit words of xoshiro256** state.
type ranState struct {
	s [4]uint64
}

// globalRanState is the shared PRNG state for math.random/math.randomseed.
// In C Lua this is stored as a userdata upvalue; we use a package-level var.
var globalRanState ranState

func init() {
	// Default seed (matches C Lua's initial randseed call with makeseed)
	setSeed(&globalRanState, uint64(time.Now().UnixNano()), 0)
}

// nextRand implements xoshiro256** — returns the next pseudo-random uint64.
// Exact port of C Lua's nextrand() for 64-bit Rand64.
func nextRand(s *[4]uint64) uint64 {
	s0 := s[0]
	s1 := s[1]
	s2 := s[2] ^ s0
	s3 := s[3] ^ s1
	res := bits.RotateLeft64(s1*5, 7) * 9
	s[0] = s0 ^ s3
	s[1] = s1 ^ s2
	s[2] = s2 ^ (s1 << 17)
	s[3] = bits.RotateLeft64(s3, 45)
	return res
}

// i2d converts a random uint64 to a float64 in [0, 1).
// Matches C Lua's I2d for FIGS=53 (DBL_MANT_DIG).
const (
	figs       = 53
	shift64FIG = 64 - figs                            // 11
	scaleFIG   = 0.5 / float64(uint64(1)<<(figs-1))   // 2^(-53)
)

func i2d(x uint64) float64 {
	sx := int64(x >> shift64FIG) // take top 53 bits as signed
	res := float64(sx) * scaleFIG
	if sx < 0 {
		res += 1.0 // correct two's complement
	}
	return res
}

// project projects a random uint64 into [0, n] using rejection sampling.
// Exact port of C Lua's project().
func project(ran uint64, n uint64, state *ranState) uint64 {
	if n == 0 {
		return 0
	}
	lim := n
	// Spread '1' bits until lim becomes a Mersenne number (all 1s)
	for sh := uint(1); (lim & (lim + 1)) != 0; sh *= 2 {
		lim |= lim >> sh
	}
	for (ran & lim) > n {
		ran = nextRand(&state.s)
	}
	return ran & lim
}

// setSeed initializes the PRNG state — exact port of C Lua's setseed().
func setSeed(state *ranState, n1, n2 uint64) {
	state.s[0] = n1
	state.s[1] = 0xff // avoid zero state
	state.s[2] = n2
	state.s[3] = 0
	for i := 0; i < 16; i++ {
		nextRand(&state.s) // discard initial values to spread seed
	}
}

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
		d := math.Ceil(L.CheckNumber(1))
		if floatFitsInt(d) {
			L.PushInteger(int64(d))
		} else {
			L.PushNumber(d)
		}
	}
	return 1
}

func math_floor(L *luaapi.State) int {
	if L.IsInteger(1) {
		L.SetTop(1)
	} else {
		d := math.Floor(L.CheckNumber(1))
		if floatFitsInt(d) {
			L.PushInteger(int64(d))
		} else {
			L.PushNumber(d)
		}
	}
	return 1
}

// floatFitsInt checks if a float can be exactly represented as int64.
// Matches C Lua's lua_numbertointeger macro.
func floatFitsInt(n float64) bool {
	const minInt = float64(math.MinInt64)         // -2^63
	const maxInt = -float64(math.MinInt64)         // 2^63 (as float, not representable as int64)
	return n >= minInt && n < maxInt
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
	// C Lua: lua_tointegerx — handles integers, floats that are exact ints, and strings
	if n, ok := L.ToInteger(1); ok {
		L.PushInteger(n)
	} else if f, ok := L.ToNumber(1); ok {
		// Float or string-as-float: check if it's an exact integer
		if math.IsNaN(f) || math.IsInf(f, 0) || !floatFitsInt(f) {
			L.PushFail()
		} else {
			i := int64(f)
			if float64(i) == f {
				L.PushInteger(i)
			} else {
				L.PushFail()
			}
		}
	} else {
		L.CheckAny(1) // ensure there's an argument
		L.PushFail()
	}
	return 1
}

func math_type(L *luaapi.State) int {
	if L.Type(1) == object.TypeNumber {
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
	state := &globalRanState
	rv := nextRand(&state.s) // next pseudo-random value
	switch L.GetTop() {
	case 0: // no arguments: random float in [0,1)
		L.PushNumber(i2d(rv))
		return 1
	case 1: // only upper limit
		low := int64(1)
		up := L.CheckInteger(1)
		if up == 0 { // math.random(0) → full-range random integer
			L.PushInteger(int64(rv))
			return 1
		}
		L.ArgCheck(1 <= up, 1, "interval is empty")
		// project into [0, up-low] then shift
		p := project(rv, uint64(up)-uint64(low), state)
		L.PushInteger(int64(p + uint64(low)))
		return 1
	case 2: // lower and upper limits
		low := L.CheckInteger(1)
		up := L.CheckInteger(2)
		L.ArgCheck(low <= up, 1, "interval is empty")
		// Use unsigned subtraction to handle full int64 range
		p := project(rv, uint64(up)-uint64(low), state)
		L.PushInteger(int64(p + uint64(low)))
		return 1
	default:
		L.PushString("wrong number of arguments")
		L.Error()
		return 0 // unreachable
	}
}

func math_randomseed(L *luaapi.State) int {
	state := &globalRanState
	var n1, n2 uint64
	if L.IsNoneOrNil(1) {
		// "random" seed — use time-based
		n1 = uint64(time.Now().UnixNano())
		n2 = nextRand(&state.s) // extra randomness from current state
	} else {
		n1 = uint64(L.CheckInteger(1))
		n2 = uint64(L.OptInteger(2, 0))
	}
	setSeed(state, n1, n2)
	// Return the two seed values (C Lua returns 2)
	L.PushInteger(int64(n1))
	L.PushInteger(int64(n2))
	return 2
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

func math_frexp(L *luaapi.State) int {
	x := L.CheckNumber(1)
	frac, exp := math.Frexp(x)
	L.PushNumber(frac)
	L.PushInteger(int64(exp))
	return 2
}

func math_ldexp(L *luaapi.State) int {
	x := L.CheckNumber(1)
	ep := L.CheckInteger(2)
	L.PushNumber(math.Ldexp(x, int(ep)))
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
		"frexp":      math_frexp,
		"ldexp":      math_ldexp,
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
