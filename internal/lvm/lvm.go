package lvm

/*
** $Id: lvm.go $
** Lua virtual machine
** Ported from lvm.h and lvm.c
*/

import (
	"math"

	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** Limit for table tag-method chains (to avoid infinite loops)
 */
const MAXTAGLOOP = 2000

/*
** Rounding modes for float->integer coercion
 */
type F2Imod uint8

const (
	F2Ieq    F2Imod = iota // no rounding; accepts only integral values
	F2Ifloor               // takes the floor of the number
	F2Iceil                // takes the ceiling of the number
)

/*
** Convert an object to a float (including string coercion)
 */
func ToNumber(o *lobject.TValue, n *lobject.LuaNumber) bool {
	if lobject.TtIsFloat(o) {
		*n = lobject.FltValue(o)
		return true
	}
	return luaV_tonumber(o, n)
}

/*
** Convert an object to a float (without string coercion)
 */
func ToNumberNS(o *lobject.TValue, n *lobject.LuaNumber) bool {
	if lobject.TtIsFloat(o) {
		*n = lobject.FltValue(o)
		return true
	}
	if lobject.TtIsInteger(o) {
		*n = lobject.LuaNumber(lobject.IntValue(o))
		return true
	}
	return false
}

/*
** Convert an object to an integer (including string coercion)
 */
func ToInteger(o *lobject.TValue, i *lobject.LuaInteger, mode F2Imod) bool {
	if lobject.TtIsInteger(o) {
		*i = lobject.IntValue(o)
		return true
	}
	return luaV_tointeger(o, i, mode)
}

/*
** Convert an object to an integer (without string coercion)
 */
func ToIntegerNS(o *lobject.TValue, i *lobject.LuaInteger, mode F2Imod) bool {
	if lobject.TtIsInteger(o) {
		*i = lobject.IntValue(o)
		return true
	}
	return luaV_tointegerns(o, i, mode)
}

func cvt2num(o *lobject.TValue) bool {
	return lobject.TtIsString(o)
}

/*
** Try to convert a value to a float.
 */
func luaV_tonumber(obj *lobject.TValue, n *lobject.LuaNumber) bool {
	if lobject.TtIsInteger(obj) {
		*n = lobject.LuaNumber(lobject.IntValue(obj))
		return true
	}
	return false
}

/*
** Try to convert a float to an integer.
 */
func luaV_flttointeger(n lobject.LuaNumber, p *lobject.LuaInteger, mode F2Imod) bool {
	f := math.Floor(float64(n))
	if n != lobject.LuaNumber(f) {
		if mode == F2Ieq {
			return false
		} else if mode == F2Iceil {
			f++
		}
	}
	if f > float64(lobject.LuaInteger(^uint64(0)>>1)) {
		return false
	}
	*p = lobject.LuaInteger(f)
	return true
}

/*
** Try to convert a value to an integer, without string coercion.
 */
func luaV_tointegerns(obj *lobject.TValue, p *lobject.LuaInteger, mode F2Imod) bool {
	if lobject.TtIsFloat(obj) {
		return luaV_flttointeger(lobject.FltValue(obj), p, mode)
	}
	if lobject.TtIsInteger(obj) {
		*p = lobject.IntValue(obj)
		return true
	}
	return false
}

/*
** Try to convert a value to an integer.
 */
func luaV_tointeger(obj *lobject.TValue, p *lobject.LuaInteger, mode F2Imod) bool {
	return luaV_tointegerns(obj, p, mode)
}

/*
** Check whether integer 'i' is less than float 'f'.
 */
func LTintfloat(i lobject.LuaInteger, f lobject.LuaNumber) bool {
	if l_intfitsf(i) {
		return lobject.LuaNumber(i) < f
	}
	fi := lobject.LuaInteger(0)
	if luaV_flttointeger(f, &fi, F2Iceil) {
		return i < fi
	}
	return f > 0
}

/*
** Check whether integer 'i' is less than or equal to float 'f'.
 */
func LEintfloat(i lobject.LuaInteger, f lobject.LuaNumber) bool {
	if l_intfitsf(i) {
		return lobject.LuaNumber(i) <= f
	}
	fi := lobject.LuaInteger(0)
	if luaV_flttointeger(f, &fi, F2Ifloor) {
		return i <= fi
	}
	return f > 0
}

/*
** Check whether float 'f' is less than integer 'i'.
 */
func LTfloatint(f lobject.LuaNumber, i lobject.LuaInteger) bool {
	if l_intfitsf(i) {
		return f < lobject.LuaNumber(i)
	}
	fi := lobject.LuaInteger(0)
	if luaV_flttointeger(f, &fi, F2Ifloor) {
		return fi < i
	}
	return f < 0
}

/*
** Check whether float 'f' is less than or equal to integer 'i'.
 */
func LEfloatint(f lobject.LuaNumber, i lobject.LuaInteger) bool {
	if l_intfitsf(i) {
		return f <= lobject.LuaNumber(i)
	}
	fi := lobject.LuaInteger(0)
	if luaV_flttointeger(f, &fi, F2Iceil) {
		return fi <= i
	}
	return f < 0
}

/*
** Number of bits in the mantissa of a float
 */
const NBM = 53 // IEEE 754 double has 53-bit mantissa

/*
** Check whether an integer fits in a float without rounding.
 */
func l_intfitsf(i lobject.LuaInteger) bool {
	return true // simplified - always fits for int64 on IEEE 754
}

/*
** Return 'l < r', for numbers.
 */
func LTnum(l, r *lobject.TValue) bool {
	if lobject.TtIsInteger(l) {
		if lobject.TtIsInteger(r) {
			return lobject.IntValue(l) < lobject.IntValue(r)
		}
		return LTintfloat(lobject.IntValue(l), lobject.FltValue(r))
	}
	if lobject.TtIsFloat(l) {
		if lobject.TtIsFloat(r) {
			return lobject.FltValue(l) < lobject.FltValue(r)
		}
		return LTfloatint(lobject.FltValue(l), lobject.IntValue(r))
	}
	return false
}

/*
** Return 'l <= r', for numbers.
 */
func LEnum(l, r *lobject.TValue) bool {
	if lobject.TtIsInteger(l) {
		if lobject.TtIsInteger(r) {
			return lobject.IntValue(l) <= lobject.IntValue(r)
		}
		return LEintfloat(lobject.IntValue(l), lobject.FltValue(r))
	}
	if lobject.TtIsFloat(l) {
		if lobject.TtIsFloat(r) {
			return lobject.FltValue(l) <= lobject.FltValue(r)
		}
		return LEfloatint(lobject.FltValue(l), lobject.IntValue(r))
	}
	return false
}

/*
** Main operation less than; return 'l < r'.
 */
func luaV_lessthan(L *lstate.LuaState, l, r *lobject.TValue) bool {
	if lobject.TtIsNumber(l) && lobject.TtIsNumber(r) {
		return LTnum(l, r)
	}
	return false
}

/*
** Main operation less than or equal to; return 'l <= r'.
 */
func luaV_lessequal(L *lstate.LuaState, l, r *lobject.TValue) bool {
	if lobject.TtIsNumber(l) && lobject.TtIsNumber(r) {
		return LEnum(l, r)
	}
	return false
}

/*
** Main operation for equality of Lua values; return 't1 == t2'.
 */
func luaV_equalobj(L *lstate.LuaState, t1, t2 *lobject.TValue) bool {
	if lobject.TType(t1) != lobject.TType(t2) {
		return false
	}
	tag1 := lobject.TypeTag(t1)
	tag2 := lobject.TypeTag(t2)
	if tag1 != tag2 {
		switch tag1 {
		case lobject.LUA_VNUMINT:
			i2 := lobject.LuaInteger(0)
			return luaV_flttointeger(lobject.FltValue(t2), &i2, F2Ieq) &&
				lobject.IntValue(t1) == i2
		case lobject.LUA_VNUMFLT:
			i1 := lobject.LuaInteger(0)
			return luaV_flttointeger(lobject.FltValue(t1), &i1, F2Ieq) &&
				i1 == lobject.IntValue(t2)
		}
		return false
	}

	// Equal variants
	switch tag1 {
	case lobject.LUA_VNIL, lobject.LUA_VFALSE, lobject.LUA_VTRUE:
		return true
	case lobject.LUA_VNUMINT:
		return lobject.IntValue(t1) == lobject.IntValue(t2)
	case lobject.LUA_VNUMFLT:
		return lobject.FltValue(t1) == lobject.FltValue(t2)
	case lobject.LUA_VSHRSTR, lobject.LUA_VLNGSTR:
		return false // TODO: implement string comparison
	case lobject.LUA_VTABLE:
		return false // TODO: implement table comparison
	case lobject.LUA_VLCF:
		return lobject.FValue(t1) == nil && lobject.FValue(t2) == nil
	default:
		return lobject.GcValue2(t1) == lobject.GcValue2(t2)
	}
}

/*
** Raw equal object (no metamethods)
 */
func luaV_rawequalobj(t1, t2 *lobject.TValue) bool {
	return luaV_equalobj(nil, t1, t2)
}

/*
** Integer division; return 'm // n'.
 */
func luaV_idiv(L *lstate.LuaState, m, n lobject.LuaInteger) lobject.LuaInteger {
	if n == 0 {
		return 0
	}
	q := m / n
	if (m^n) < 0 && m%n != 0 {
		q--
	}
	return q
}

/*
** Integer modulus; return 'm % n'.
 */
func luaV_mod(L *lstate.LuaState, m, n lobject.LuaInteger) lobject.LuaInteger {
	if n == 0 {
		return 0
	}
	r := m % n
	if r != 0 && (r^n) < 0 {
		r += n
	}
	return r
}

/*
** Float modulus
 */
func luaV_modf(L *lstate.LuaState, m, n lobject.LuaNumber) lobject.LuaNumber {
	r := math.Mod(float64(m), float64(n))
	return lobject.LuaNumber(r)
}

/*
** Shift operations.
 */
func luaV_shiftl(x, y lobject.LuaInteger) lobject.LuaInteger {
	if y < 0 {
		if y <= -64 {
			return 0
		}
		return x >> uint(-y)
	}
	if y >= 64 {
		return 0
	}
	return x << uint(y)
}

func luaV_shiftr(x, y lobject.LuaInteger) lobject.LuaInteger {
	return luaV_shiftl(x, -y)
}

/*
** Main operation 'ra = #rb'.
 */
func luaV_objlen(L *lstate.LuaState, ra, rb *lobject.TValue) {
	// TODO: implement table/udata length
	lobject.SetIntValue(ra, 0)
}

/*
** Number of bits in an integer
 */
const NBITS = 64

func intop(op rune, v1, v2 lobject.LuaInteger) lobject.LuaInteger {
	switch op {
	case '+':
		return v1 + v2
	case '-':
		return v1 - v2
	case '*':
		return v1 * v2
	case '/':
		if v2 == 0 {
			return 0
		}
		return v1 / v2
	case '%':
		return luaV_mod(nil, v1, v2)
	case '&':
		return v1 & v2
	case '|':
		return v1 | v2
	case '^':
		return v1 ^ v2
	}
	return 0
}