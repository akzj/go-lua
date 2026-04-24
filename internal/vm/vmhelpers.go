// vmhelpers.go — VM helper functions (arithmetic, comparison, coercion, table access).
package vm

import (
	"math"
	"strconv"
	"strings"

	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/gc"
	"github.com/akzj/go-lua/internal/luastring"
	"github.com/akzj/go-lua/internal/metamethod"
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/table"
)

// ---------------------------------------------------------------------------
// Number conversion helpers
// ---------------------------------------------------------------------------

// toNumber tries to convert a TValue to float64.
func toNumber(v object.TValue) (float64, bool) {
	switch v.Tt {
	case object.TagFloat:
		return v.Float(), true
	case object.TagInteger:
		return float64(v.N), true
	case object.TagShortStr, object.TagLongStr:
		return stringToNumber(v.Obj.(*object.LuaString).Data)
	}
	return 0, false
}

// toInteger tries to convert a TValue to int64.
// toIntegerStrict converts a TValue to int64 without string coercion.
// Used for bitwise ops which in Lua 5.5 do NOT coerce strings.
func toIntegerStrict(v object.TValue) (int64, bool) {
	switch v.Tt {
	case object.TagInteger:
		return v.N, true
	case object.TagFloat:
		return floatToInteger(v.Float())
	}
	return 0, false
}

func toInteger(v object.TValue) (int64, bool) {
	switch v.Tt {
	case object.TagInteger:
		return v.N, true
	case object.TagFloat:
		return floatToInteger(v.Float())
	case object.TagShortStr, object.TagLongStr:
		return stringToInteger(v.Obj.(*object.LuaString).Data)
	}
	return 0, false
}

// floatToInteger converts a float64 to int64 if it has an exact integer value
// and is within int64 range. Mirrors: luaO_cast_number2int in lobject.c.
func floatToInteger(f float64) (int64, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	i := int64(f)
	if float64(i) == f && i != 0 {
		return i, true
	}
	if f == 0 {
		return 0, true
	}
	return 0, false
}

// toFloat extracts a float64 from a TValue that may hold int64 or float64.
func toFloat(v object.TValue) float64 {
	switch v.Tt {
	case object.TagFloat:
		return v.Float()
	case object.TagInteger:
		return float64(v.N)
	}
	return 0
}

// floatToIntegerFloor converts float to integer rounding toward negative infinity.
func floatToIntegerFloor(f float64) (int64, bool) {
	fl := math.Floor(f)
	i := int64(fl)
	if float64(i) == fl && !math.IsInf(fl, 0) && !math.IsNaN(fl) {
		return i, true
	}
	return 0, false
}

// floatToIntegerCeil converts float to integer rounding toward positive infinity.
func floatToIntegerCeil(f float64) (int64, bool) {
	c := math.Ceil(f)
	i := int64(c)
	if float64(i) == c && !math.IsInf(c, 0) && !math.IsNaN(c) {
		return i, true
	}
	return 0, false
}

// toNumberTV converts a TValue to a numeric TValue, preserving int/float type.
// For strings, uses StringToNumber which returns int for "10", float for "10.5".
// This is used for arithmetic coercion where type preservation matters.
func toNumberTV(v object.TValue) (object.TValue, bool) {
	switch v.Tt {
	case object.TagFloat, object.TagInteger:
		return v, true
	case object.TagShortStr, object.TagLongStr:
		return object.StringToNumber(v.Obj.(*object.LuaString).Data)
	}
	return object.Nil, false
}

// toNumberNS converts a TValue to a numeric TValue WITHOUT string coercion.
// This mirrors C Lua's tonumberns: only handles int64 and float64 types.
// Strings and all other types return failure, causing fallthrough to metamethod dispatch.
func toNumberNS(v object.TValue) (object.TValue, bool) {
	switch v.Tt {
	case object.TagFloat, object.TagInteger:
		return v, true
	}
	return object.Nil, false
}

// toNumberNSFloat converts a TValue to float64 WITHOUT string coercion.
// This mirrors C Lua's tonumberns for float-only ops (POW, DIV).
// Only handles int64 (promoted to float) and float64 types.
func toNumberNSFloat(v object.TValue) (float64, bool) {
	switch v.Tt {
	case object.TagFloat:
		return v.Float(), true
	case object.TagInteger:
		return float64(v.Integer()), true
	}
	return 0, false
}

// arithBinTV performs a binary arithmetic operation on two TValues with proper
// int/float type preservation. If both operands are integers, uses intOp;
// otherwise converts both to float and uses floatOp.
func arithBinTV(a, b object.TValue, intOp func(int64, int64) int64, floatOp func(float64, float64) float64) object.TValue {
	if a.IsInteger() && b.IsInteger() {
		return object.MakeInteger(intOp(a.Integer(), b.Integer()))
	}
	fa := toFloat(a)
	fb := toFloat(b)
	return object.MakeFloat(floatOp(fa, fb))
}

// stringToNumber tries to parse a string as a number (float64).
func stringToNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0, false
	}
	// Try hex
	if len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		// Parse hex integer
		var n int64
		for _, c := range s[2:] {
			var d int64
			switch {
			case c >= '0' && c <= '9':
				d = int64(c - '0')
			case c >= 'a' && c <= 'f':
				d = int64(c-'a') + 10
			case c >= 'A' && c <= 'F':
				d = int64(c-'A') + 10
			default:
				return 0, false
			}
			n = n*16 + d
		}
		return float64(n), true
	}
	// Try integer first
	isInt := true
	for i, c := range s {
		if c == '.' || c == 'e' || c == 'E' {
			isInt = false
			break
		}
		if i == 0 && (c == '-' || c == '+') {
			continue
		}
		if c < '0' || c > '9' {
			isInt = false
			break
		}
	}
	if isInt {
		var n int64
		neg := false
		start := 0
		if s[0] == '-' {
			neg = true
			start = 1
		} else if s[0] == '+' {
			start = 1
		}
		for _, c := range s[start:] {
			if c < '0' || c > '9' {
				return 0, false
			}
			n = n*10 + int64(c-'0')
		}
		if neg {
			n = -n
		}
		return float64(n), true
	}
	// Parse as float using simple approach
	// Use math to parse
	var f float64
	var n int
	n, _ = parseFloat(s, &f)
	if n == len(s) {
		return f, true
	}
	return 0, false
}

// parseFloat is a simple float parser. Returns number of chars consumed.
func parseFloat(s string, result *float64) (int, error) {
	// Use a simple state machine
	i := 0
	neg := false
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		neg = s[i] == '-'
		i++
	}
	intPart := 0.0
	hasInt := false
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		intPart = intPart*10 + float64(s[i]-'0')
		i++
		hasInt = true
	}
	fracPart := 0.0
	if i < len(s) && s[i] == '.' {
		i++
		scale := 0.1
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			fracPart += float64(s[i]-'0') * scale
			scale *= 0.1
			i++
			hasInt = true
		}
	}
	if !hasInt {
		return 0, nil
	}
	val := intPart + fracPart
	// Exponent
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		expNeg := false
		if i < len(s) && (s[i] == '-' || s[i] == '+') {
			expNeg = s[i] == '-'
			i++
		}
		exp := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			exp = exp*10 + int(s[i]-'0')
			i++
		}
		if expNeg {
			val *= math.Pow(10, float64(-exp))
		} else {
			val *= math.Pow(10, float64(exp))
		}
	}
	if neg {
		val = -val
	}
	*result = val
	return i, nil
}

// stringToInteger tries to parse a string as an integer.
func stringToInteger(s string) (int64, bool) {
	f, ok := stringToNumber(s)
	if !ok {
		return 0, false
	}
	return floatToInteger(f)
}

// ---------------------------------------------------------------------------
// Arithmetic helpers
// ---------------------------------------------------------------------------

// iDiv performs integer floor division (m // n).
func iDiv(L *state.LuaState, m, n int64) int64 {
	if uint64(n)+1 <= 1 { // n == 0 or n == -1
		if n == 0 {
			RunError(L, "attempt to divide by zero")
		}
		return -m // n == -1; avoid overflow with MinInt64 // -1
	}
	q := m / n
	if (m^n) < 0 && m%n != 0 {
		q--
	}
	return q
}

// iMod performs integer modulus (m % n).
func iMod(L *state.LuaState, m, n int64) int64 {
	if uint64(n)+1 <= 1 {
		if n == 0 {
			RunError(L, "attempt to perform 'n%0'")
		}
		return 0 // m % -1 == 0
	}
	r := m % n
	if r != 0 && (r^n) < 0 {
		r += n
	}
	return r
}

// fMod performs float modulus.
func fMod(m, n float64) float64 {
	r := math.Mod(m, n)
	if r != 0 && (math.Signbit(r) != math.Signbit(n)) {
		r += n
	}
	return r
}

// shiftL performs left shift (or right shift if y < 0).
func shiftL(x, y int64) int64 {
	if y < 0 {
		if y <= -64 {
			return 0
		}
		return int64(uint64(x) >> uint(-y))
	}
	if y >= 64 {
		return 0
	}
	return int64(uint64(x) << uint(y))
}

// ---------------------------------------------------------------------------
// Comparison helpers
// ---------------------------------------------------------------------------

// ltIntFloat checks i < f with proper handling of precision.
// intFitsFloat checks whether an integer can be converted to float64 without
// rounding (i.e., it fits in the 53-bit mantissa of IEEE 754 double).
const maxIntFitsFloat = int64(1) << 53

func intFitsFloat(i int64) bool {
	// Check if i is in [-2^53, 2^53]
	return uint64(maxIntFitsFloat+i) <= uint64(2*maxIntFitsFloat)
}

// ltIntFloat checks i < f with full precision (mirrors C Lua's LTintfloat).
func ltIntFloat(i int64, f float64) bool {
	if f != f { // NaN
		return false
	}
	if intFitsFloat(i) {
		return float64(i) < f
	}
	// i doesn't fit in float — convert f to int using ceil, then compare as ints
	// i < f  ⟺  i < ceil(f)
	if fi, ok := floatToIntegerCeil(f); ok {
		return i < fi
	}
	// f is outside int64 range — result depends on sign of f
	return f > 0
}

// leIntFloat checks i <= f with full precision (mirrors C Lua's LEintfloat).
func leIntFloat(i int64, f float64) bool {
	if f != f {
		return false
	}
	if intFitsFloat(i) {
		return float64(i) <= f
	}
	// i <= f  ⟺  i <= floor(f)
	if fi, ok := floatToIntegerFloor(f); ok {
		return i <= fi
	}
	return f > 0
}

// ltFloatInt checks f < i with full precision (mirrors C Lua's LTfloatint).
func ltFloatInt(f float64, i int64) bool {
	if f != f {
		return false
	}
	if intFitsFloat(i) {
		return f < float64(i)
	}
	// f < i  ⟺  floor(f) < i
	if fi, ok := floatToIntegerFloor(f); ok {
		return fi < i
	}
	return f < 0
}

// leFloatInt checks f <= i with full precision (mirrors C Lua's LEfloatint).
func leFloatInt(f float64, i int64) bool {
	if f != f {
		return false
	}
	if intFitsFloat(i) {
		return f <= float64(i)
	}
	// f <= i  ⟺  ceil(f) <= i
	if fi, ok := floatToIntegerCeil(f); ok {
		return fi <= i
	}
	return f < 0
}

// ltNum compares two numbers (l < r).
func ltNum(l, r object.TValue) bool {
	if l.IsInteger() {
		li := l.N
		if r.IsInteger() {
			return li < r.N
		}
		return ltIntFloat(li, r.Float())
	}
	lf := l.Float()
	if r.IsFloat() {
		return lf < r.Float()
	}
	return ltFloatInt(lf, r.N)
}

// leNum compares two numbers (l <= r).
func leNum(l, r object.TValue) bool {
	if l.IsInteger() {
		li := l.N
		if r.IsInteger() {
			return li <= r.N
		}
		return leIntFloat(li, r.Float())
	}
	lf := l.Float()
	if r.IsFloat() {
		return lf <= r.Float()
	}
	return leFloatInt(lf, r.N)
}

// LessThan performs l < r with metamethods.
func LessThan(L *state.LuaState, l, r object.TValue) bool {
	if l.IsNumber() && r.IsNumber() {
		return ltNum(l, r)
	}
	if l.IsString() && r.IsString() {
		return l.Obj.(*object.LuaString).Data < r.Obj.(*object.LuaString).Data
	}
	// Try metamethod
	return callOrderTM(L, l, r, metamethod.TM_LT)
}

// LessEqual performs l <= r with metamethods.
func LessEqual(L *state.LuaState, l, r object.TValue) bool {
	if l.IsNumber() && r.IsNumber() {
		return leNum(l, r)
	}
	if l.IsString() && r.IsString() {
		return l.Obj.(*object.LuaString).Data <= r.Obj.(*object.LuaString).Data
	}
	return callOrderTM(L, l, r, metamethod.TM_LE)
}

// callOrderTM calls an order metamethod (__lt or __le).
func callOrderTM(L *state.LuaState, l, r object.TValue, event metamethod.TMS) bool {
	tm := metamethod.GetTMByObj(L.Global, l, event)
	if tm.IsNil() {
		tm = metamethod.GetTMByObj(L.Global, r, event)
	}
	if tm.IsNil() {
		if event == metamethod.TM_LE {
			// Try __lt with swapped operands: a <= b iff !(b < a)
			tm = metamethod.GetTMByObj(L.Global, r, metamethod.TM_LT)
			if tm.IsNil() {
				tm = metamethod.GetTMByObj(L.Global, l, metamethod.TM_LT)
			}
			if !tm.IsNil() {
				result := callTMRes(L, tm, r, l)
				return result.IsFalsy() // !(b < a)
			}
		}
		// Build comparison error message: "two <type> values" or "<type1> with <type2> values"
		lt := metamethod.ObjTypeName(L.Global, l)
		rt := metamethod.ObjTypeName(L.Global, r)
		if lt == rt {
			RunError(L, "attempt to compare two "+lt+" values")
		} else {
			RunError(L, "attempt to compare "+lt+" with "+rt)
		}
	}
	result := callTMRes(L, tm, l, r)
	return !result.IsFalsy()
}

// callOrderITM calls an order metamethod for comparison-with-immediate opcodes.
// Mirrors: luaT_callorderiTM in ltm.c
// p1 is the register value, im is the immediate integer, flip indicates whether
// arguments should be swapped (for GT/GE), isf indicates the immediate is float,
// event is TM_LT or TM_LE.
func callOrderITM(L *state.LuaState, p1 object.TValue, im int64, flip bool, isf bool, event metamethod.TMS) bool {
	// Create TValue for the immediate
	var p2 object.TValue
	if isf {
		p2 = object.MakeFloat(float64(im))
	} else {
		p2 = object.MakeInteger(im)
	}
	// Flip arguments if needed (GT/GE use flip)
	if flip {
		return callOrderTM(L, p2, p1, event)
	}
	return callOrderTM(L, p1, p2, event)
}

// EqualObj performs t1 == t2 with metamethods.
func EqualObj(L *state.LuaState, t1, t2 object.TValue) bool {
	if t1.Tt != t2.Tt {
		// Different tags — check int/float cross-comparison
		if t1.IsNumber() && t2.IsNumber() {
			if t1.IsInteger() && t2.IsFloat() {
				i2, ok := floatToInteger(t2.Float())
				return ok && t1.N == i2
			}
			if t1.IsFloat() && t2.IsInteger() {
				i1, ok := floatToInteger(t1.Float())
				return ok && i1 == t2.N
			}
		}
		// Short vs Long string comparison by content
		if t1.IsString() && t2.IsString() {
			return t1.Obj.(*object.LuaString).Data == t2.Obj.(*object.LuaString).Data
		}
		return false
	}
	// Same base type
	switch t1.Tt {
	case object.TagNil:
		return true
	case object.TagFalse, object.TagTrue:
		return t1.Tt == t2.Tt
	case object.TagInteger:
		return t1.N == t2.N
	case object.TagFloat:
		return t1.Float() == t2.Float()
	case object.TagShortStr:
		return t1.Obj.(*object.LuaString).Data == t2.Obj.(*object.LuaString).Data
	case object.TagLongStr:
		return t1.Obj.(*object.LuaString).Data == t2.Obj.(*object.LuaString).Data
	case object.TagTable:
		h1 := t1.Obj.(*table.Table)
		h2 := t2.Obj.(*table.Table)
		if h1 == h2 {
			return true
		}
		if L == nil {
			return false
		}
		// Try __eq metamethod
		tm := getTableTM(L.Global, h1, metamethod.TM_EQ)
		if tm.IsNil() {
			tm = getTableTM(L.Global, h2, metamethod.TM_EQ)
		}
		if tm.IsNil() {
			return false
		}
		result := callTMRes(L, tm, t1, t2)
		return !result.IsFalsy()
	case object.TagUserdata:
		if t1.Obj == t2.Obj {
			return true
		}
		if L == nil {
			return false
		}
		// Try __eq metamethod for userdata
		// For userdata, the metatable IS the table to search (not metatable's metatable)
		u1 := t1.Obj.(*object.Userdata)
		u2 := t2.Obj.(*object.Userdata)
		var tm object.TValue
		if mt1, ok := u1.MetaTable.(*table.Table); ok && mt1 != nil {
			tmName := L.Global.TMNames[metamethod.TM_EQ]
			if tmName != nil {
				tm, _ = mt1.GetStr(tmName)
			}
		}
		if tm.IsNil() {
			if mt2, ok := u2.MetaTable.(*table.Table); ok && mt2 != nil {
				tmName := L.Global.TMNames[metamethod.TM_EQ]
				if tmName != nil {
					tm, _ = mt2.GetStr(tmName)
				}
			}
		}
		if tm.IsNil() {
			return false
		}
		result := callTMRes(L, tm, t1, t2)
		return !result.IsFalsy()
	case object.TagLuaClosure, object.TagCClosure:
		return t1.Obj == t2.Obj // pointer comparison — struct pointers are comparable
	case object.TagLightCFunc:
		// Use interface data word for unique closure identity (reflect.Pointer is shared)
		return object.LightCFuncEqual(t1.Obj, t2.Obj)
	default:
		if t1.Tt == object.TagLightCFunc || t2.Tt == object.TagLightCFunc {
			return false // different tags already checked above
		}
		return t1.Obj == t2.Obj
	}
}

// rawEqualObj performs raw equality (no metamethods).
func rawEqualObj(t1, t2 object.TValue) bool {
	if t1.Tt != t2.Tt {
		if t1.IsNumber() && t2.IsNumber() {
			if t1.IsInteger() && t2.IsFloat() {
				i2, ok := floatToInteger(t2.Float())
				return ok && t1.N == i2
			}
			if t1.IsFloat() && t2.IsInteger() {
				i1, ok := floatToInteger(t1.Float())
				return ok && i1 == t2.N
			}
		}
		// Short vs Long string comparison by content
		if t1.IsString() && t2.IsString() {
			return t1.Obj.(*object.LuaString).Data == t2.Obj.(*object.LuaString).Data
		}
		return false
	}
	switch t1.Tt {
	case object.TagNil:
		return true
	case object.TagFalse, object.TagTrue:
		return t1.Tt == t2.Tt
	case object.TagInteger:
		return t1.N == t2.N
	case object.TagFloat:
		return t1.Float() == t2.Float()
	case object.TagShortStr, object.TagLongStr:
		return t1.Obj.(*object.LuaString).Data == t2.Obj.(*object.LuaString).Data
	case object.TagLuaClosure, object.TagCClosure:
		return t1.Obj == t2.Obj // pointer comparison
	case object.TagLightCFunc:
		return object.LightCFuncEqual(t1.Obj, t2.Obj)
	default:
		return t1.Obj == t2.Obj
	}
}

// getTableTM gets a metamethod from a table's metatable.
func getTableTM(g *state.GlobalState, t *table.Table, event metamethod.TMS) object.TValue {
	mt := t.GetMetatable()
	if mt == nil {
		return object.Nil
	}
	if g == nil {
		return object.Nil
	}
	tmName := g.TMNames[event]
	if tmName == nil {
		return object.Nil
	}
	v, _ := mt.GetStr(tmName)
	return v
}

// ---------------------------------------------------------------------------
// Metamethod call helpers
// ---------------------------------------------------------------------------

// callTMRes calls a metamethod with two arguments and returns the result.
// IMPORTANT: Uses L.Top as scratch space. Callers must ensure L.Top is above
// any registers they need preserved (posCall moves result to the func slot).
func callTMRes(L *state.LuaState, tm, p1, p2 object.TValue) object.TValue {
	// Push: tm, p1, p2
	top := L.Top
	L.Stack[top].Val = tm
	L.Stack[top+1].Val = p1
	L.Stack[top+2].Val = p2
	L.Top = top + 3
	Call(L, top, 1)
	result := L.Stack[top].Val // posCall moves result to func slot
	L.Top = top                // restore top
	return result
}

// callTM calls a metamethod with 3 args (tm, p1, p2, p3) — for __newindex etc.
func callTM(L *state.LuaState, tm, p1, p2, p3 object.TValue) {
	top := L.Top
	L.Stack[top].Val = tm
	L.Stack[top+1].Val = p1
	L.Stack[top+2].Val = p2
	L.Stack[top+3].Val = p3
	L.Top = top + 4
	Call(L, top, 0)
}

// opErrorMsg builds a type error message with optional variable info.
// Mirrors: luaG_opinterror + luaG_typeerror in ldebug.c
func opErrorMsg(L *state.LuaState, p1, p2 object.TValue, op string, reg1, reg2 int) string {
	// Pick the wrong operand (first non-number)
	badReg := reg1
	badVal := p1
	if p1.IsNumber() {
		badReg = reg2
		badVal = p2
	}
	info := ""
	if badReg >= 0 {
		info = varInfo(L, badReg)
	}
	return "attempt to " + op + " a " + metamethod.ObjTypeName(L.Global, badVal) + " value" + info
}

// tryBinTM tries a binary metamethod.
// reg1, reg2 are register hints for p1, p2 (-1 if not a register).
func tryBinTM(L *state.LuaState, p1, p2 object.TValue, res int, event metamethod.TMS, reg1, reg2 int) {
	tm := metamethod.GetTMByObj(L.Global, p1, event)
	if tm.IsNil() {
		tm = metamethod.GetTMByObj(L.Global, p2, event)
	}
	if tm.IsNil() {
		if event == metamethod.TM_CONCAT {
			RunError(L, opErrorMsg(L, p1, p2, "concatenate", reg1, reg2))
		}
		if event >= metamethod.TM_BAND && event <= metamethod.TM_SHR || event == metamethod.TM_BNOT {
			// If both are numbers but can't convert to int, give specific error
			// Mirrors: luaG_tointerror in ldebug.c
			if p1.IsNumber() && p2.IsNumber() {
				// Find the non-integer operand for the error message
				badReg := reg2
				if p1.IsFloat() {
					badReg = reg1
				}
				RunError(L, "number"+varInfo(L, badReg)+" has no integer representation")
			}
			RunError(L, opErrorMsg(L, p1, p2, "perform bitwise operation on", reg1, reg2))
		}
		RunError(L, opErrorMsg(L, p1, p2, "perform arithmetic on", reg1, reg2))
	}
	// Ensure L.Top is above res so callTMRes doesn't overwrite
	// the destination register with its call frame arguments.
	if res >= L.Top {
		L.Top = res + 1
	}
	result := callTMRes(L, tm, p1, p2)
	L.Stack[res].Val = result
}

// tryBiniTM tries a binary metamethod with an integer immediate operand.
func tryBiniTM(L *state.LuaState, p1 object.TValue, imm int, flip bool, res int, event metamethod.TMS, reg1 int) {
	p2 := object.MakeInteger(int64(imm))
	if flip {
		tryBinTM(L, p2, p1, res, event, -1, reg1)
	} else {
		tryBinTM(L, p1, p2, res, event, reg1, -1)
	}
}

// tryBinKTM tries a binary metamethod with a constant operand (possibly flipped).
func tryBinKTM(L *state.LuaState, p1, p2 object.TValue, flip bool, res int, event metamethod.TMS, reg1 int) {
	if flip {
		tryBinTM(L, p2, p1, res, event, -1, reg1)
	} else {
		tryBinTM(L, p1, p2, res, event, reg1, -1)
	}
}

// tryConcatTM tries the __concat metamethod for two values.
func tryConcatTM(L *state.LuaState) {
	top := L.Top
	p1 := L.Stack[top-2].Val
	p2 := L.Stack[top-1].Val
	tm := metamethod.GetTMByObj(L.Global, p1, metamethod.TM_CONCAT)
	if tm.IsNil() {
		tm = metamethod.GetTMByObj(L.Global, p2, metamethod.TM_CONCAT)
	}
	if tm.IsNil() {
		// Report the type of the non-string/non-number operand
		errType := p1
		if p1.IsString() || p1.IsNumber() {
			errType = p2
		}
		RunError(L, "attempt to concatenate a "+metamethod.ObjTypeName(L.Global, errType)+" value")
	}
	result := callTMRes(L, tm, p1, p2)
	L.Stack[top-2].Val = result
	L.Top = top - 1
}

// ---------------------------------------------------------------------------
// String/Number coercion for concat
// ---------------------------------------------------------------------------

func toStringForConcat(v object.TValue) (string, bool) {
	switch v.Tt {
	case object.TagShortStr, object.TagLongStr:
		return v.Obj.(*object.LuaString).Data, true
	case object.TagInteger:
		return intToString(v.N), true
	case object.TagFloat:
		return floatToString(v.Float()), true
	}
	return "", false
}

func intToString(i int64) string {
	// Stack-allocated buffer avoids heap allocation for small integers.
	var buf [20]byte
	return string(strconv.AppendInt(buf[:0], i, 10))
}

func floatToString(f float64) string {
	if f == 0 {
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	if math.IsInf(f, 1) {
		return "inf"
	}
	if math.IsInf(f, -1) {
		return "-inf"
	}
	if math.IsNaN(f) {
		return "-nan"
	}
	return formatFloat(f)
}

func formatFloat(f float64) string {
	// Lua uses %.14g format
	// Simple implementation
	if f == float64(int64(f)) && !math.IsInf(f, 0) && math.Abs(f) < 1e15 {
		return intToString(int64(f)) + ".0"
	}
	// For non-integer floats, use a reasonable representation
	buf := make([]byte, 0, 32)
	buf = appendFloat(buf, f)
	return string(buf)
}

func appendFloat(buf []byte, f float64) []byte {
	// Simple float formatting — use Go's standard approach
	neg := false
	if f < 0 {
		neg = true
		f = -f
	}
	if neg {
		buf = append(buf, '-')
	}
	// Integer part
	intPart := math.Floor(f)
	fracPart := f - intPart
	// Write integer part
	if intPart == 0 {
		buf = append(buf, '0')
	} else {
		digits := make([]byte, 0, 20)
		ip := intPart
		for ip >= 1 {
			d := math.Mod(ip, 10)
			digits = append(digits, byte('0'+int(d)))
			ip = math.Floor(ip / 10)
		}
		for i := len(digits) - 1; i >= 0; i-- {
			buf = append(buf, digits[i])
		}
	}
	if fracPart > 0 {
		buf = append(buf, '.')
		for i := 0; i < 14 && fracPart > 0; i++ {
			fracPart *= 10
			d := int(fracPart)
			buf = append(buf, byte('0'+d))
			fracPart -= float64(d)
		}
		// Trim trailing zeros
		for len(buf) > 0 && buf[len(buf)-1] == '0' {
			buf = buf[:len(buf)-1]
		}
	}
	return buf
}

// ---------------------------------------------------------------------------
// Concat
// ---------------------------------------------------------------------------

// internString creates a properly interned LuaString via the global string table.
// This ensures correct hash values for table key lookups.
func internString(L *state.LuaState, s string) *object.LuaString {
	st := L.Global.StringTable.(*luastring.StringTable)
	return st.Intern(s)
}

// makeInternedString creates a TValue string that is properly interned.
func makeInternedString(L *state.LuaState, s string) object.TValue {
	return object.MakeString(internString(L, s))
}

// makeConcatString creates a string TValue from a concat result.
// Short strings (≤ MaxShortLen) are interned through the string table to
// maintain pointer identity — this is required for correct table key lookups
// where the type tag (TagShortStr vs TagLongStr) determines the search path.
// Long strings are created without interning; they use content comparison.
func makeConcatString(L *state.LuaState, s string) object.TValue {
	if len(s) <= luastring.MaxShortLen {
		// Short strings must be interned for correct table key identity
		return object.MakeString(internString(L, s))
	}
	// Long strings: non-interned, content comparison for equality
	ls := &object.LuaString{
		Data:    s,
		Hash_:   0,     // computed lazily if used as table key
		IsShort: false, // non-interned — content comparison for equality
	}
	L.Global.LinkGC(ls) // register with GC so it gets swept
	return object.MakeString(ls)
}

// Concat concatenates 'total' values on the stack from L.Top-total to L.Top-1.
func Concat(L *state.LuaState, total int) {
	if total == 1 {
		return
	}
	for total > 1 {
		top := L.Top
		n := 2
		p1 := L.Stack[top-2].Val
		p2 := L.Stack[top-1].Val
		s1, ok1 := toStringForConcat(p1)
		s2, ok2 := toStringForConcat(p2)
		if !ok1 || !ok2 {
			// tryConcatTM handles L.Top adjustment internally:
			// sets L.Stack[top-2] = result and L.Top = top - 1
			tryConcatTM(L)
			total -= n - 1
			// Do NOT adjust L.Top here — tryConcatTM already did it
		} else if len(s2) == 0 {
			// Result is first operand (already converted to string)
			if !p1.IsString() {
				L.Stack[top-2].Val = makeInternedString(L, s1)
			}
			total -= n - 1
			L.Top -= n - 1
		} else if len(s1) == 0 {
			// Result is second operand converted to string
			if !p2.IsString() {
				L.Stack[top-2].Val = makeInternedString(L, s2)
			} else {
				L.Stack[top-2].Val = L.Stack[top-1].Val
			}
			total -= n - 1
			L.Top -= n - 1
		} else {
			// Collect as many consecutive string-convertible values as possible.
			// Walk the stack backwards (toward lower indices), appending in
			// reverse order, then build the result with strings.Builder.
			// This avoids the O(n²) prepend of the old []string{sv}, parts... approach.
			var buf [8]string
			parts := buf[:0]
			parts = append(parts, s2, s1) // reverse order: s2 first, s1 second
			for n < total {
				sv, ok := toStringForConcat(L.Stack[top-n-1].Val)
				if !ok {
					break
				}
				parts = append(parts, sv)
				n++
			}
			// Reverse parts to get correct left-to-right order
			for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
				parts[i], parts[j] = parts[j], parts[i]
			}
			// Pre-size builder to avoid intermediate allocations
			totalLen := 0
			for _, p := range parts {
				totalLen += len(p)
			}
			var b strings.Builder
			b.Grow(totalLen)
			for _, p := range parts {
				b.WriteString(p)
			}
			L.Stack[top-n].Val = makeConcatString(L, b.String())
			total -= n - 1
			L.Top -= n - 1
		}
	}
}

// ---------------------------------------------------------------------------
// ObjLen — # operator
// ---------------------------------------------------------------------------

// ObjLen computes #rb and stores in L.Stack[ra].
func ObjLen(L *state.LuaState, ra int, rb object.TValue) {
	switch rb.Tt {
	case object.TagTable:
		h := rb.Obj.(*table.Table)
		mt := h.GetMetatable()
		if mt != nil {
			tm := getTableTM(L.Global, h, metamethod.TM_LEN)
			if !tm.IsNil() {
				// Ensure L.Top is above ra so callTMRes doesn't clobber live registers
				if L.Top <= ra {
					L.Top = ra + 1
				}
				result := callTMRes(L, tm, rb, rb)
				L.Stack[ra].Val = result
				return
			}
		}
		L.Stack[ra].Val = object.MakeInteger(h.RawLen())
	case object.TagShortStr, object.TagLongStr:
		s := rb.Obj.(*object.LuaString)
		L.Stack[ra].Val = object.MakeInteger(int64(len(s.Data)))
	default:
		tm := metamethod.GetTMByObj(L.Global, rb, metamethod.TM_LEN)
		if tm.IsNil() {
			RunError(L, "attempt to get length of a "+metamethod.ObjTypeName(L.Global, rb)+" value")
		}
		// Ensure L.Top is above ra so callTMRes doesn't clobber live registers
		if L.Top <= ra {
			L.Top = ra + 1
		}
		result := callTMRes(L, tm, rb, rb)
		L.Stack[ra].Val = result
	}
}

// ---------------------------------------------------------------------------
// Table access with metamethods
// ---------------------------------------------------------------------------

// FinishGet completes a table get with __index metamethod chain.
func FinishGet(L *state.LuaState, t, key object.TValue, ra int) {
	for loop := 0; loop < maxTagLoop; loop++ {
		var tm object.TValue
		if t.IsTable() {
			h := t.Obj.(*table.Table)
			tm = getTableTM(L.Global, h, metamethod.TM_INDEX)
			if tm.IsNil() {
				L.Stack[ra].Val = object.Nil
				return
			}
		} else {
			tm = metamethod.GetTMByObj(L.Global, t, metamethod.TM_INDEX)
			if tm.IsNil() {
				runTypeErrorByVal(L, t, "index")
			}
		}
		if tm.IsFunction() {
			// Ensure L.Top is above ra so callTMRes doesn't overwrite
			// the destination register with its call frame arguments.
			// (ra may be beyond L.Top in complex expressions like `x == 1 and a.z`)
			if ra >= L.Top {
				L.Top = ra + 1
			}
			result := callTMRes(L, tm, t, key)
			L.Stack[ra].Val = result
			return
		}
		// tm is a table — repeat with tm as the new table
		t = tm
		if t.IsTable() {
			h := t.Obj.(*table.Table)
			val, found := h.Get(key)
			if found && !val.IsNil() {
				L.Stack[ra].Val = val
				return
			}
		}
	}
	RunError(L, "'__index' chain too long; possible loop")
}

// tableSetWithMeta sets a key in a table, checking for __newindex metamethod
// when the key doesn't already exist. This is the "fast set + fallback" pattern
// matching C Lua's luaV_fastset / luaV_finishset.
func tableSetWithMeta(L *state.LuaState, tval object.TValue, key, val object.TValue) {
	// Check for nil/NaN key — C Lua raises luaG_runerror, not panic
	if key.IsNil() {
		RunError(L, "table index is nil")
	}
	if key.IsFloat() && math.IsNaN(key.Float()) {
		RunError(L, "table index is NaN")
	}
	h := tval.Obj.(*table.Table)
	// Fast path: key already exists → overwrite in single lookup (no rehash)
	if h.SetIfExists(key, val) {
		gc.BarrierBack(L.Global, h) // GC write barrier: table mutated
		return
	}
	// Key absent — check for __newindex metamethod
	tm := getTableTM(L.Global, h, metamethod.TM_NEWINDEX)
	if tm.IsNil() {
		// No metamethod — raw set (may trigger rehash)
		h.Set(key, val)
		trackTableResize(L.Global, h) // track resize delta for GC debt
		gc.BarrierBack(L.Global, h)   // GC write barrier: table mutated
		return
	}
	// Has __newindex — delegate to FinishSet which handles the chain
	FinishSet(L, tval, key, val)
}

// FinishSet completes a table set with __newindex metamethod chain.
func FinishSet(L *state.LuaState, t, key, val object.TValue) {
	for loop := 0; loop < maxTagLoop; loop++ {
		var tm object.TValue
		if t.IsTable() {
			h := t.Obj.(*table.Table)
			tm = getTableTM(L.Global, h, metamethod.TM_NEWINDEX)
			if tm.IsNil() {
				h.Set(key, val)
				trackTableResize(L.Global, h) // track resize delta for GC debt
				gc.BarrierBack(L.Global, h)   // GC write barrier: table mutated
				return
			}
		} else {
			tm = metamethod.GetTMByObj(L.Global, t, metamethod.TM_NEWINDEX)
			if tm.IsNil() {
				runTypeErrorByVal(L, t, "index")
			}
		}
		if tm.IsFunction() {
			callTM(L, tm, t, key, val)
			return
		}
		// tm is a table — repeat
		t = tm
		if t.IsTable() {
			h := t.Obj.(*table.Table)
			_, found := h.Get(key)
			if found {
				h.Set(key, val)
				gc.BarrierBack(L.Global, h) // GC write barrier: table mutated
				return
			}
		}
	}
	RunError(L, "'__newindex' chain too long; possible loop")
}

// ---------------------------------------------------------------------------
// For loop helpers
// ---------------------------------------------------------------------------

// forerror raises a for-loop type error matching C Lua's luaG_forerror:
// "bad 'for' <what> (number expected, got <typename>)"
func forerror(L *state.LuaState, val object.TValue, what string) {
	RunError(L, "bad 'for' "+what+" (number expected, got "+metamethod.ObjTypeName(L.Global, val)+")")
}

// forLimit implements C Lua's forlimit: convert a for-loop limit to integer
// using floor (step>0) or ceil (step<0) rounding. Returns true to skip the loop.
// This matches lvm.c forlimit exactly.
func forLimit(L *state.LuaState, init int64, plimit object.TValue, limit *int64, step int64) bool {
	// First try exact integer conversion (handles integer values and exact float-to-int)
	if li, ok := flttointeger(plimit, step); ok {
		*limit = li
	} else {
		// Not coercible to integer with floor/ceil rounding
		fl, ok := toNumber(plimit)
		if !ok {
			forerror(L, plimit, "limit")
		}
		// fl is a float out of integer bounds
		if fl > 0 { // positive, too large
			if step < 0 {
				return true // init must be less than limit, but limit is huge positive
			}
			*limit = math.MaxInt64 // truncate
		} else { // negative or zero, too small
			if step > 0 {
				return true // init must be greater than limit, but limit is huge negative
			}
			*limit = math.MinInt64 // truncate
		}
	}
	// Check if loop should not run
	if step > 0 {
		return init > *limit
	}
	return init < *limit
}

// flttointeger converts a TValue to integer using floor (step>0) or ceil (step<0)
// rounding mode, matching C Lua's luaV_tointeger with F2Ifloor/F2Iceil.
func flttointeger(v object.TValue, step int64) (int64, bool) {
	switch v.Tt {
	case object.TagInteger:
		return v.N, true
	case object.TagFloat:
		f := v.Float()
		return floatToIntegerRounded(f, step)
	case object.TagShortStr, object.TagLongStr:
		// String: try to parse as number, then convert with rounding
		s := v.Obj.(*object.LuaString).Data
		if n, ok := stringToInteger(s); ok {
			return n, true
		}
		// Try as float then round
		if f, ok := stringToNumber(s); ok {
			return floatToIntegerRounded(f, step)
		}
		return 0, false
	}
	return 0, false
}

// floatToIntegerRounded converts float to integer using floor (step>0) or ceil (step<0).
// Matches C Lua's luaV_flttointeger with F2Ifloor/F2Iceil.
func floatToIntegerRounded(f float64, step int64) (int64, bool) {
	fl := math.Floor(f)
	if f != fl {
		// Not integral — apply rounding
		if step < 0 {
			// F2Iceil: convert floor to ceiling
			fl += 1
		}
		// else F2Ifloor: keep floor
	}
	// Check if fl is in int64 range
	if fl >= -9223372036854775808.0 && fl < 9223372036854775808.0 {
		return int64(fl), true
	}
	return 0, false
}

// forPrep prepares a numeric for loop. Returns true to skip the loop.
func forPrep(L *state.LuaState, ra int) bool {
	pinit := L.Stack[ra].Val
	plimit := L.Stack[ra+1].Val
	pstep := L.Stack[ra+2].Val

	if pinit.IsInteger() && pstep.IsInteger() {
		init := pinit.N
		step := pstep.N
		if step == 0 {
			RunError(L, "'for' step is zero")
		}
		// Convert limit using forlimit logic (matches C Lua's forlimit)
		// forLimit handles both conversion and skip-check (init > limit or init < limit)
		var limit int64
		if skip := forLimit(L, init, plimit, &limit, step); skip {
			return true
		}
		// Compute count
		var count uint64
		if step > 0 {
			count = uint64(limit) - uint64(init)
			if step != 1 {
				count /= uint64(step)
			}
		} else {
			count = uint64(init) - uint64(limit)
			count /= uint64(-(step + 1)) + 1
		}
		// Rearrange: ra=count, ra+1=step, ra+2=init (control variable)
		L.Stack[ra].Val = object.MakeInteger(int64(count))
		L.Stack[ra+1].Val = object.MakeInteger(step)
		L.Stack[ra+2].Val = object.MakeInteger(init)
	} else {
		// Float loop
		finit, ok1 := toNumber(pinit)
		flimit, ok2 := toNumber(plimit)
		fstep, ok3 := toNumber(pstep)
		if !ok1 {
			forerror(L, pinit, "initial value")
		}
		if !ok2 {
			forerror(L, plimit, "limit")
		}
		if !ok3 {
			forerror(L, pstep, "step")
		}
		if fstep == 0 {
			RunError(L, "'for' step is zero")
		}
		if fstep > 0 && finit > flimit {
			return true
		}
		if fstep < 0 && finit < flimit {
			return true
		}
		// Rearrange: ra=limit, ra+1=step, ra+2=init (control variable)
		L.Stack[ra].Val = object.MakeFloat(flimit)
		L.Stack[ra+1].Val = object.MakeFloat(fstep)
		L.Stack[ra+2].Val = object.MakeFloat(finit)
	}
	return false
}

// forLoopInt handles the integer for-loop fast path.
// Extracted from forLoop to keep inline cost low.
// Integer loop: ra=count, ra+1=step, ra+2=control.
// Sets N directly instead of using MakeInteger to reduce inline cost.
func forLoopInt(stack []object.StackValue, ra int) bool {
	count := uint64(stack[ra].Val.N)
	if count > 0 {
		stack[ra].Val.N = int64(count - 1)
		stack[ra+2].Val.N += stack[ra+1].Val.N
		return true
	}
	return false
}

// forLoopFloat handles the float for-loop path.
// Float loop: ra=limit, ra+1=step, ra+2=control
func forLoopFloat(stack []object.StackValue, ra int) bool {
	step := stack[ra+1].Val.Float()
	limit := stack[ra].Val.Float()
	idx := stack[ra+2].Val.Float()
	idx += step
	if step > 0 {
		if idx <= limit {
			stack[ra+2].Val = object.MakeFloat(idx)
			return true
		}
	} else {
		if limit <= idx {
			stack[ra+2].Val = object.MakeFloat(idx)
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Closure creation
// ---------------------------------------------------------------------------

// pushClosure creates a new Lua closure and stores it in L.Stack[ra].
func pushClosure(L *state.LuaState, p *object.Proto, encup []*closure.UpVal, base, ra int) {
	nup := len(p.Upvalues)
	ncl := closure.NewLClosure(p, nup)
	L.Global.LinkGC(ncl) // V5: register in allgc chain
	L.Stack[ra].Val = object.TValue{Tt: object.TagLuaClosure, Obj: ncl}
	for i := 0; i < nup; i++ {
		if p.Upvalues[i].InStack {
			ncl.UpVals[i] = closure.FindUpval(L, base+int(p.Upvalues[i].Idx))
		} else {
			ncl.UpVals[i] = encup[p.Upvalues[i].Idx]
		}
	}
}

// ---------------------------------------------------------------------------
// Vararg handling
// ---------------------------------------------------------------------------

// adjustVarargs adjusts the stack for a vararg function call.
// Mirrors C Lua's luaT_adjustvarargs / buildhiddenargs.
// Layout after adjustment:
//
//	[old ci.Func] ... [extra args] [func copy] [fixed params] ...
//	ci.Func now points to the func copy (after the extra args).
func adjustVarargs(L *state.LuaState, ci *state.CallInfo, p *object.Proto) {
	nfixparams := int(p.NumParams)
	totalargs := L.Top - ci.Func - 1
	nextra := totalargs - nfixparams
	if nextra < 0 {
		// Fill missing fixed params with nil
		for i := totalargs; i < nfixparams; i++ {
			L.Stack[L.Top].Val = object.Nil
			L.Top++
		}
		nextra = 0
		totalargs = nfixparams
	}

	if p.Flag&object.PF_VATAB != 0 {
		// === PF_VATAB path: create vararg table ===
		// Mirrors: luaT_adjustvarargs + createvarargtab in ltm.c
		CheckStack(L, int(p.MaxStackSize)+1)
		t := table.New(nextra, 1)
		L.Global.LinkGC(t) // V5: register in allgc chain
		size := t.EstimateBytes()
		t.GCHeader.ObjSize = size
		L.Global.GCTotalBytes += size
		// V5 GC sweep handles dealloc accounting — no AddCleanup needed
		// Set t.n = nextra
		st := L.Global.StringTable.(*luastring.StringTable)
		nKey := object.MakeString(st.Intern("n"))
		t.Set(nKey, object.MakeInteger(int64(nextra)))
		// Set t[1..nextra] = extra args
		for i := 0; i < nextra; i++ {
			t.SetInt(int64(i+1), L.Stack[ci.Func+nfixparams+1+i].Val)
		}
		// Place table at the vararg parameter slot (after fixed params)
		L.Stack[ci.Func+nfixparams+1].Val = object.TValue{Tt: object.TagTable, Obj: t}
		// Set top to after all params (fixed + vararg table)
		L.Top = ci.Func + 1 + nfixparams + 1
		ci.Top = ci.Func + 1 + int(p.MaxStackSize)
		// NOTE: no stack shift, no ci.Func change, no NExtraArgs
	} else {
		// === PF_VAHID path: existing hidden args behavior ===
		// Mirrors: buildhiddenargs in ltm.c
		ci.NExtraArgs = nextra
		CheckStack(L, int(p.MaxStackSize)+1)
		// Copy function to top of stack
		L.Stack[L.Top].Val = L.Stack[ci.Func].Val
		L.Top++
		// Copy fixed parameters above extra args
		for i := 1; i <= nfixparams; i++ {
			L.Stack[L.Top].Val = L.Stack[ci.Func+i].Val
			L.Stack[ci.Func+i].Val = object.Nil // erase original (for GC)
			L.Top++
		}
		// ci.Func now lives after the hidden (extra) arguments
		ci.Func += totalargs + 1
		ci.Top = ci.Func + 1 + int(p.MaxStackSize)
		// Set vararg parameter slot to nil (mirrors C Lua: setnilvalue)
		L.Stack[ci.Func+nfixparams+1].Val = object.Nil
	}
}

// getVarargs copies vararg values to the stack starting at ra.
// When vatab >= 0, reads from the vararg table at ci.Func+vatab+1.
// When vatab < 0, reads from hidden stack args below ci.Func.
// Mirrors: luaT_getvarargs in ltm.c
func getVarargs(L *state.LuaState, ci *state.CallInfo, ra int, n int, vatab int) {
	var h *table.Table
	if vatab >= 0 {
		h = L.Stack[ci.Func+vatab+1].Val.Obj.(*table.Table)
	}

	// Get number of available vararg args — mirrors getnumargs() in ltm.c
	var nExtra int
	if h == nil {
		nExtra = ci.NExtraArgs
	} else {
		// Read t.n from the vararg table and validate
		st := L.Global.StringTable.(*luastring.StringTable)
		nKey := object.MakeString(st.Intern("n"))
		nVal, ok := h.Get(nKey)
		if !ok || nVal.Tt != object.TagInteger {
			RunError(L, "vararg table has no proper 'n'")
		}
		iv := nVal.Integer()
		// C Lua: l_castS2U(ivalue(&res)) > cast_uint(INT_MAX/2)
		// Negative values become huge unsigned → error
		const maxN = int64(0x7fffffff / 2) // INT_MAX/2
		if uint64(iv) > uint64(maxN) {
			RunError(L, "vararg table has no proper 'n'")
		}
		nExtra = int(iv)
	}

	if n < 0 {
		n = nExtra
		CheckStack(L, n)
		L.Top = ra + n
	}

	touse := n
	if nExtra < touse {
		touse = nExtra
	}

	if h == nil {
		// Read from hidden stack args
		varBase := ci.Func - nExtra
		for i := 0; i < touse; i++ {
			L.Stack[ra+i].Val = L.Stack[varBase+i].Val
		}
	} else {
		// Read from vararg table
		for i := 0; i < touse; i++ {
			val, ok := h.GetInt(int64(i + 1))
			if ok {
				L.Stack[ra+i].Val = val
			} else {
				L.Stack[ra+i].Val = object.Nil
			}
		}
	}
	// Fill remaining with nil
	for i := touse; i < n; i++ {
		L.Stack[ra+i].Val = object.Nil
	}
}

// ---------------------------------------------------------------------------
// API helpers — exported wrappers for metamethod-aware table operations
// ---------------------------------------------------------------------------

// APISetTable performs t[key] = val with __newindex metamethod support.
// Used by the C API's lua_settable.
func APISetTable(L *state.LuaState, t, key, val object.TValue) {
	if t.IsTable() {
		tableSetWithMeta(L, t, key, val)
	} else {
		FinishSet(L, t, key, val)
	}
}

// APIGetTable performs result = t[key] with __index metamethod support.
// Used by the C API's lua_gettable. Writes result to L.Stack[ra].
func APIGetTable(L *state.LuaState, t, key object.TValue, ra int) {
	FinishGet(L, t, key, ra)
}
