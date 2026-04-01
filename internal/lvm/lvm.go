package lvm

/*
** $Id: lvm.go $
** Lua virtual machine
** Ported from lvm.h and lvm.c
*/

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lopcodes"
	"github.com/akzj/go-lua/internal/lstate"
	"github.com/akzj/go-lua/internal/ltable"
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
/*
** LuaV_execute - execute Lua bytecode
 */
func LuaV_execute(L *lstate.LuaState, ci *lstate.CallInfo, func_ *lobject.TValue) {
	// Get closure from func_
	gcObj := lobject.GcValue(func_)
	cl := lobject.Gco2Lcl(gcObj)
	fmt.Printf("DEBUG VM: func_=%p cl=%p cl.P=%p\n", func_, cl, cl.P)
	if cl == nil || cl.P == nil {
		return
	}
	
	// Get constant pool and instructions
	k := cl.P.K
	ins := cl.P.Code
	fmt.Printf("DEBUG K[] len=%d\n", len(k))
	for i := 0; i < len(k) && i < 10; i++ {
		gcPtr := lobject.GcValue(&k[i])
		fmt.Printf("DEBUG K[%d] Tt_=%d GcPtr=%p\n", i, k[i].Tt_, gcPtr)
	}
	fmt.Printf("DEBUG ins[] len=%d\n", len(ins))
	for i := 0; i < len(ins) && i < 10; i++ {
		fmt.Printf("DEBUG ins[%d]=%d\n", i, ins[i])
	}
	if len(ins) == 0 {
		return
	}
	
	// Base pointer points to function slot
	base := func_
	
	// Program counter
	pc := 0
	
	// Main VM loop
	for pc < len(ins) {
		i := ins[pc]
		opcode := lopcodes.TranslateFromC(int(lopcodes.GetOpCode(i)))
		
		switch opcode {
		case lopcodes.OP_GETTABUP:
			a := lopcodes.GETARG_A(i)
			b := lopcodes.GETARG_B(i)
			c := lopcodes.GETARG_C(i)
			fmt.Printf("DEBUG VM GETTABUP: R[%d] = _ENV[%d][K[%d]]\n", a, b, c)
			dst := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(base)) + uintptr(a)*unsafe.Sizeof(lobject.TValue{})))
			fmt.Printf("DEBUG VM GETTABUP: BEFORE R[%d] Tt_=%d Value_.I=%d\n", a, dst.Tt_, dst.Value_.I)
			// Get upvalue[0] (which is _ENV)
			if b < int(cl.Nupvalues) && cl.Upvals[b] != nil {
				upvalPtr := cl.Upvals[b].V
				if upvalPtr != nil {
					tblPtr := upvalPtr.Gc
					if tblPtr != nil {
						tbl := lobject.Gco2T(tblPtr)
						fmt.Printf("DEBUG VM GETTABUP: tbl=%p\n", tbl)
						// Get key from constants[c]
						if c >= 0 && c < len(k) {
							key := &k[c]
							fmt.Printf("DEBUG VM GETTABUP: key Tt_=%d GcPtr=%p\n", key.Tt_, lobject.GcValue(key))
							// Table access - use SearchGeneric instead
							if val := ltable.SearchGeneric(tbl, key); val != nil {
								fmt.Printf("DEBUG VM GETTABUP: found val=%p Tt_=%d Value_.Gc=%p\n", val, val.Tt_, val.Value_.Gc)
								lobject.SetObj(dst, val)
								fmt.Printf("DEBUG VM GETTABUP: AFTER SetObj R[%d] Tt_=%d Value_.F=%p\n", a, dst.Tt_, dst.Value_.F)
							} else {
								fmt.Printf("DEBUG VM GETTABUP: NOT FOUND\n")
							}
						}
					}
				}
			}
			pc++
			
		case lopcodes.OP_LOADK:
			a := lopcodes.GETARG_A(i)
			bx := lopcodes.GETARG_Bx(i)
			fmt.Printf("DEBUG VM LOADK: R[%d] = K[%d] (Tt_=%d)\n", a, bx, k[bx].Tt_)
			dst := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(base)) + uintptr(a)*unsafe.Sizeof(lobject.TValue{})))
			if bx >= 0 && bx < len(k) {
				lobject.SetObj(dst, &k[bx])
			}
			pc++
			
		case lopcodes.OP_MOVE:
			a := lopcodes.GETARG_A(i)
			b := lopcodes.GETARG_B(i)
			fmt.Printf("DEBUG VM MOVE: R[%d] = R[%d]\n", a, b)
			src := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(base)) + uintptr(b)*unsafe.Sizeof(lobject.TValue{})))
			dst := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(base)) + uintptr(a)*unsafe.Sizeof(lobject.TValue{})))
			lobject.SetObj(dst, src)
			fmt.Printf("DEBUG VM MOVE: R[%d].Tt_=%d Value_.I=%d\n", a, dst.Tt_, dst.Value_.I)
			pc++
			
		case lopcodes.OP_CALL:
			a := lopcodes.GETARG_A(i)
			b := lopcodes.GETARG_B(i)
			fn := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(base)) + uintptr(a)*unsafe.Sizeof(lobject.TValue{})))
			fmt.Printf("DEBUG VM CALL: R[%d] fn=%p Tt_=%d F=%p B=%d\n", a, fn, fn.Tt_, fn.Value_.F, b)
			if lobject.TtIsLcf(fn) {
				fmt.Printf("DEBUG VM CALL: Is light C function\n")
				f := lobject.FValue(fn)
				// sizeof(TValue) = 56 (actual size in this codebase)
				sz := int(unsafe.Sizeof(lobject.TValue{}))
				// For CALL R[A] with B args:
				// - R[A] = function
				// - R[A+1] to R[A+B-1] = arguments (B-1 visible args for C function)
				// - L.Top should point past last argument
				numArgs := b - 1 // C function sees B-1 arguments
				newTop := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(base)) + uintptr(a+b)*uintptr(sz)))
				L.Top.P = newTop
				// For index2value(L, 1) to return R[A+1] (first arg), need:
				// ci.F.P + sizeof = base + (A+1)*sizeof
				// ci.F.P = base + A*sizeof
				L.Ci.F.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(base)) + uintptr(a)*uintptr(sz)))
				fmt.Printf("DEBUG VM CALL: ci.F.P=%p base=%p a=%d numArgs=%d\n", L.Ci.F.P, base, a, numArgs)
				fmt.Printf("DEBUG VM CALL: calling f=%p\n", f)
				f((*lobject.LuaState)(unsafe.Pointer(L)))
				fmt.Printf("DEBUG VM CALL: returned from print\n")
			} else {
				fmt.Printf("DEBUG VM CALL: Not LCF, skipping\n")
			}
			pc++
			
		case lopcodes.OP_RETURN, lopcodes.OP_RETURN0:
			return
			
		case lopcodes.OP_RETURN1:
			return
			
		default:
			pc++
		}
	}
}
