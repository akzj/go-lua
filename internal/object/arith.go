// Arithmetic operations for Lua values (used by constant folding and VM).
//
// Reference: lua-master/lobject.c (luaO_rawarith, intarith, numarith)
package object

import "math"

// Lua arithmetic operation codes (matches C lua.h LUA_OP*)
const (
	LuaOpAdd  = 0  // LUA_OPADD
	LuaOpSub  = 1  // LUA_OPSUB
	LuaOpMul  = 2  // LUA_OPMUL
	LuaOpMod  = 3  // LUA_OPMOD
	LuaOpPow  = 4  // LUA_OPPOW
	LuaOpDiv  = 5  // LUA_OPDIV
	LuaOpIDiv = 6  // LUA_OPIDIV
	LuaOpBAnd = 7  // LUA_OPBAND
	LuaOpBOr  = 8  // LUA_OPBOR
	LuaOpBXor = 9  // LUA_OPBXOR
	LuaOpShl  = 10 // LUA_OPSHL
	LuaOpShr  = 11 // LUA_OPSHR
	LuaOpUnm  = 12 // LUA_OPUNM
	LuaOpBNot = 13 // LUA_OPBNOT
)

// intArith performs integer arithmetic. Mirrors C intarith.
func intArith(op int, v1, v2 int64) int64 {
	switch op {
	case LuaOpAdd:
		return v1 + v2
	case LuaOpSub:
		return v1 - v2
	case LuaOpMul:
		return v1 * v2
	case LuaOpMod:
		return IntMod(v1, v2)
	case LuaOpIDiv:
		return IntIDiv(v1, v2)
	case LuaOpBAnd:
		return v1 & v2
	case LuaOpBOr:
		return v1 | v2
	case LuaOpBXor:
		return v1 ^ v2
	case LuaOpShl:
		return ShiftLeft(v1, v2)
	case LuaOpShr:
		return ShiftLeft(v1, -v2)
	case LuaOpUnm:
		return -v1
	case LuaOpBNot:
		return ^v1
	default:
		panic("intArith: invalid op")
	}
}

// numArith performs float arithmetic. Mirrors C numarith.
func numArith(op int, v1, v2 float64) float64 {
	switch op {
	case LuaOpAdd:
		return v1 + v2
	case LuaOpSub:
		return v1 - v2
	case LuaOpMul:
		return v1 * v2
	case LuaOpDiv:
		return v1 / v2
	case LuaOpPow:
		return math.Pow(v1, v2)
	case LuaOpIDiv:
		return FloatIDiv(v1, v2)
	case LuaOpMod:
		return FloatMod(v1, v2)
	case LuaOpUnm:
		return -v1
	default:
		panic("numArith: invalid op")
	}
}

// RawArith performs raw arithmetic on TValues (no metamethods).
// Returns (result, true) on success, (Nil, false) on failure.
// Mirrors: luaO_rawarith in lobject.c
func RawArith(op int, p1, p2 TValue) (TValue, bool) {
	switch op {
	case LuaOpBAnd, LuaOpBOr, LuaOpBXor, LuaOpShl, LuaOpShr, LuaOpBNot:
		// Operate only on integers
		i1, ok1 := ToIntegerNS(p1)
		i2, ok2 := ToIntegerNS(p2)
		if ok1 && ok2 {
			return MakeInteger(intArith(op, i1, i2)), true
		}
		return Nil, false

	case LuaOpDiv, LuaOpPow:
		// Operate only on floats
		n1, ok1 := p1.ToNumber()
		n2, ok2 := p2.ToNumber()
		if ok1 && ok2 {
			return MakeFloat(numArith(op, n1, n2)), true
		}
		return Nil, false

	default:
		// Other operations: try integer first, then float
		if p1.IsInteger() && p2.IsInteger() {
			return MakeInteger(intArith(op, p1.Integer(), p2.Integer())), true
		}
		n1, ok1 := p1.ToNumber()
		n2, ok2 := p2.ToNumber()
		if ok1 && ok2 {
			return MakeFloat(numArith(op, n1, n2)), true
		}
		return Nil, false
	}
}

// ToIntegerNS converts to integer with floor mode (for bitwise ops).
// Mirrors: luaV_tointegerns with LUA_FLOORN2I mode.
func ToIntegerNS(v TValue) (int64, bool) {
	switch v.Tt {
	case TagInteger:
		return v.N, true
	case TagFloat:
		f := v.Float()
		// Exact mode: only convert if float has exact integer representation
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		i := int64(f)
		if float64(i) != f {
			return 0, false // not exactly representable as integer
		}
		return i, true
	default:
		return 0, false
	}
}

// ---------------------------------------------------------------------------
// Integer arithmetic helpers (Lua semantics)
// ---------------------------------------------------------------------------

// IntMod computes Lua integer modulo: a - floor(a/b)*b
func IntMod(a, b int64) int64 {
	r := a % b
	if r != 0 && (r^b) < 0 {
		r += b
	}
	return r
}

// IntIDiv computes Lua integer floor division.
func IntIDiv(a, b int64) int64 {
	q := a / b
	if (a^b) < 0 && a%b != 0 {
		q--
	}
	return q
}

// ShiftLeft performs Lua shift left (negative count = shift right).
func ShiftLeft(x, y int64) int64 {
	if y >= 64 || y <= -64 {
		return 0
	}
	if y >= 0 {
		return int64(uint64(x) << uint(y))
	}
	return int64(uint64(x) >> uint(-y))
}

// FloatIDiv computes Lua float floor division.
func FloatIDiv(a, b float64) float64 {
	return math.Floor(a / b)
}

// FloatMod computes Lua float modulo.
func FloatMod(a, b float64) float64 {
	r := math.Mod(a, b)
	if r != 0 && math.Signbit(r) != math.Signbit(b) {
		r += b
	}
	return r
}

// ---------------------------------------------------------------------------
// CeilLog2 — compute ceil(log2(x)) for x > 0.
// Mirrors: luaO_ceillog2 in lobject.c
// ---------------------------------------------------------------------------

var log2Table = [256]byte{
	0, 1, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 4, 4, 4, 4, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5,
	6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6,
	7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
}

// CeilLog2 returns ceil(log2(x)) for x > 0.
func CeilLog2(x uint) byte {
	l := 0
	x--
	for x >= 256 {
		l += 8
		x >>= 8
	}
	return byte(l) + log2Table[x]
}
