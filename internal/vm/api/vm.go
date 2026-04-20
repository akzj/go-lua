// VM execution loop — the heart of the Lua interpreter.
//
// This is the Go equivalent of C's lvm.c. The core is Execute(),
// a giant switch on opcodes that runs Lua bytecode.
//
// Reference: lua-master/lvm.c, .analysis/05-vm-execution-loop.md
package api

import (
	"math"
	"runtime"
	"strings"
	"sync/atomic"

	closureapi "github.com/akzj/go-lua/internal/closure/api"
	luastringapi "github.com/akzj/go-lua/internal/luastring/api"
	mmapi "github.com/akzj/go-lua/internal/metamethod/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	opcodeapi "github.com/akzj/go-lua/internal/opcode/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
	tableapi "github.com/akzj/go-lua/internal/table/api"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const maxTagLoop = 2000

// ---------------------------------------------------------------------------
// Periodic GC helper
// ---------------------------------------------------------------------------

// checkPeriodicGC increments the allocation counter and triggers a GC step
// every 5000 allocations if a step function is registered.
func checkPeriodicGC(g *stateapi.GlobalState, L *stateapi.LuaState) {
	g.GCAllocCount++
	if g.GCAllocCount%5000 == 0 && g.GCStepFn != nil {
		g.GCStepFn(L)
	}
}

// ---------------------------------------------------------------------------
// Number conversion helpers
// ---------------------------------------------------------------------------

// ToNumber tries to convert a TValue to float64.
func ToNumber(v objectapi.TValue) (float64, bool) {
	switch v.Tt {
	case objectapi.TagFloat:
		return v.Val.(float64), true
	case objectapi.TagInteger:
		return float64(v.Val.(int64)), true
	case objectapi.TagShortStr, objectapi.TagLongStr:
		return stringToNumber(v.Val.(*objectapi.LuaString).Data)
	}
	return 0, false
}

// ToInteger tries to convert a TValue to int64.
// toIntegerStrict converts a TValue to int64 without string coercion.
// Used for bitwise ops which in Lua 5.5 do NOT coerce strings.
func toIntegerStrict(v objectapi.TValue) (int64, bool) {
	switch v.Tt {
	case objectapi.TagInteger:
		return v.Val.(int64), true
	case objectapi.TagFloat:
		return FloatToInteger(v.Val.(float64))
	}
	return 0, false
}

func ToInteger(v objectapi.TValue) (int64, bool) {
	switch v.Tt {
	case objectapi.TagInteger:
		return v.Val.(int64), true
	case objectapi.TagFloat:
		return FloatToInteger(v.Val.(float64))
	case objectapi.TagShortStr, objectapi.TagLongStr:
		return stringToInteger(v.Val.(*objectapi.LuaString).Data)
	}
	return 0, false
}

// FloatToInteger converts a float64 to int64 if it has an exact integer value
// and is within int64 range. Mirrors: luaO_cast_number2int in lobject.c.
func FloatToInteger(f float64) (int64, bool) {
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
func toFloat(v objectapi.TValue) float64 {
	switch val := v.Val.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
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
func toNumberTV(v objectapi.TValue) (objectapi.TValue, bool) {
	switch v.Tt {
	case objectapi.TagFloat, objectapi.TagInteger:
		return v, true
	case objectapi.TagShortStr, objectapi.TagLongStr:
		return objectapi.StringToNumber(v.Val.(*objectapi.LuaString).Data)
	}
	return objectapi.Nil, false
}

// toNumberNS converts a TValue to a numeric TValue WITHOUT string coercion.
// This mirrors C Lua's tonumberns: only handles int64 and float64 types.
// Strings and all other types return failure, causing fallthrough to metamethod dispatch.
func toNumberNS(v objectapi.TValue) (objectapi.TValue, bool) {
	switch v.Tt {
	case objectapi.TagFloat, objectapi.TagInteger:
		return v, true
	}
	return objectapi.Nil, false
}

// ToNumberNS converts a TValue to float64 WITHOUT string coercion.
// This mirrors C Lua's tonumberns for float-only ops (POW, DIV).
// Only handles int64 (promoted to float) and float64 types.
func ToNumberNS(v objectapi.TValue) (float64, bool) {
	switch v.Tt {
	case objectapi.TagFloat:
		return v.Float(), true
	case objectapi.TagInteger:
		return float64(v.Integer()), true
	}
	return 0, false
}


// arithBinTV performs a binary arithmetic operation on two TValues with proper
// int/float type preservation. If both operands are integers, uses intOp;
// otherwise converts both to float and uses floatOp.
func arithBinTV(a, b objectapi.TValue, intOp func(int64, int64) int64, floatOp func(float64, float64) float64) objectapi.TValue {
	if a.IsInteger() && b.IsInteger() {
		return objectapi.MakeInteger(intOp(a.Integer(), b.Integer()))
	}
	fa := toFloat(a)
	fb := toFloat(b)
	return objectapi.MakeFloat(floatOp(fa, fb))
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
	return FloatToInteger(f)
}

// ---------------------------------------------------------------------------
// Arithmetic helpers
// ---------------------------------------------------------------------------

// IDiv performs integer floor division (m // n).
func IDiv(L *stateapi.LuaState, m, n int64) int64 {
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

// IMod performs integer modulus (m % n).
func IMod(L *stateapi.LuaState, m, n int64) int64 {
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

// FMod performs float modulus.
func FMod(m, n float64) float64 {
	r := math.Mod(m, n)
	if r != 0 && (math.Signbit(r) != math.Signbit(n)) {
		r += n
	}
	return r
}

// ShiftL performs left shift (or right shift if y < 0).
func ShiftL(x, y int64) int64 {
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
func ltNum(l, r objectapi.TValue) bool {
	if l.IsInteger() {
		li := l.Val.(int64)
		if r.IsInteger() {
			return li < r.Val.(int64)
		}
		return ltIntFloat(li, r.Val.(float64))
	}
	lf := l.Val.(float64)
	if r.IsFloat() {
		return lf < r.Val.(float64)
	}
	return ltFloatInt(lf, r.Val.(int64))
}

// leNum compares two numbers (l <= r).
func leNum(l, r objectapi.TValue) bool {
	if l.IsInteger() {
		li := l.Val.(int64)
		if r.IsInteger() {
			return li <= r.Val.(int64)
		}
		return leIntFloat(li, r.Val.(float64))
	}
	lf := l.Val.(float64)
	if r.IsFloat() {
		return lf <= r.Val.(float64)
	}
	return leFloatInt(lf, r.Val.(int64))
}

// LessThan performs l < r with metamethods.
func LessThan(L *stateapi.LuaState, l, r objectapi.TValue) bool {
	if l.IsNumber() && r.IsNumber() {
		return ltNum(l, r)
	}
	if l.IsString() && r.IsString() {
		return l.Val.(*objectapi.LuaString).Data < r.Val.(*objectapi.LuaString).Data
	}
	// Try metamethod
	return callOrderTM(L, l, r, mmapi.TM_LT)
}

// LessEqual performs l <= r with metamethods.
func LessEqual(L *stateapi.LuaState, l, r objectapi.TValue) bool {
	if l.IsNumber() && r.IsNumber() {
		return leNum(l, r)
	}
	if l.IsString() && r.IsString() {
		return l.Val.(*objectapi.LuaString).Data <= r.Val.(*objectapi.LuaString).Data
	}
	return callOrderTM(L, l, r, mmapi.TM_LE)
}

// callOrderTM calls an order metamethod (__lt or __le).
func callOrderTM(L *stateapi.LuaState, l, r objectapi.TValue, event mmapi.TMS) bool {
	tm := mmapi.GetTMByObj(L.Global, l, event)
	if tm.IsNil() {
		tm = mmapi.GetTMByObj(L.Global, r, event)
	}
	if tm.IsNil() {
		if event == mmapi.TM_LE {
			// Try __lt with swapped operands: a <= b iff !(b < a)
			tm = mmapi.GetTMByObj(L.Global, r, mmapi.TM_LT)
			if tm.IsNil() {
				tm = mmapi.GetTMByObj(L.Global, l, mmapi.TM_LT)
			}
			if !tm.IsNil() {
				result := callTMRes(L, tm, r, l)
				return result.IsFalsy() // !(b < a)
			}
		}
		// Build comparison error message: "two <type> values" or "<type1> with <type2> values"
		lt := mmapi.ObjTypeName(L.Global, l)
		rt := mmapi.ObjTypeName(L.Global, r)
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
func callOrderITM(L *stateapi.LuaState, p1 objectapi.TValue, im int64, flip bool, isf bool, event mmapi.TMS) bool {
	// Create TValue for the immediate
	var p2 objectapi.TValue
	if isf {
		p2 = objectapi.MakeFloat(float64(im))
	} else {
		p2 = objectapi.MakeInteger(im)
	}
	// Flip arguments if needed (GT/GE use flip)
	if flip {
		return callOrderTM(L, p2, p1, event)
	}
	return callOrderTM(L, p1, p2, event)
}

// EqualObj performs t1 == t2 with metamethods.
func EqualObj(L *stateapi.LuaState, t1, t2 objectapi.TValue) bool {
	if t1.Tt != t2.Tt {
		// Different tags — check int/float cross-comparison
		if t1.IsNumber() && t2.IsNumber() {
			if t1.IsInteger() && t2.IsFloat() {
				i2, ok := FloatToInteger(t2.Val.(float64))
				return ok && t1.Val.(int64) == i2
			}
			if t1.IsFloat() && t2.IsInteger() {
				i1, ok := FloatToInteger(t1.Val.(float64))
				return ok && i1 == t2.Val.(int64)
			}
		}
		return false
	}
	// Same base type
	switch t1.Tt {
	case objectapi.TagNil:
		return true
	case objectapi.TagFalse, objectapi.TagTrue:
		return t1.Tt == t2.Tt
	case objectapi.TagInteger:
		i1, ok1 := t1.Val.(int64)
		i2, ok2 := t2.Val.(int64)
		if ok1 && ok2 {
			return i1 == i2
		}
		// Defensive: tag says integer but value may be float
		f1 := toFloat(t1)
		f2 := toFloat(t2)
		return f1 == f2
	case objectapi.TagFloat:
		f1, ok1 := t1.Val.(float64)
		f2, ok2 := t2.Val.(float64)
		if ok1 && ok2 {
			return f1 == f2
		}
		ff1 := toFloat(t1)
		ff2 := toFloat(t2)
		return ff1 == ff2
	case objectapi.TagShortStr:
		return t1.Val.(*objectapi.LuaString).Data == t2.Val.(*objectapi.LuaString).Data
	case objectapi.TagLongStr:
		return t1.Val.(*objectapi.LuaString).Data == t2.Val.(*objectapi.LuaString).Data
	case objectapi.TagTable:
		h1 := t1.Val.(*tableapi.Table)
		h2 := t2.Val.(*tableapi.Table)
		if h1 == h2 {
			return true
		}
		if L == nil {
			return false
		}
		// Try __eq metamethod
		tm := getTableTM(L.Global, h1, mmapi.TM_EQ)
		if tm.IsNil() {
			tm = getTableTM(L.Global, h2, mmapi.TM_EQ)
		}
		if tm.IsNil() {
			return false
		}
		result := callTMRes(L, tm, t1, t2)
		return !result.IsFalsy()
	case objectapi.TagLuaClosure, objectapi.TagCClosure:
		return t1.Val == t2.Val // pointer comparison — struct pointers are comparable
	case objectapi.TagLightCFunc:
		// Use interface data word for unique closure identity (reflect.Pointer is shared)
		return objectapi.LightCFuncEqual(t1.Val, t2.Val)
	default:
		if t1.Tt == objectapi.TagLightCFunc || t2.Tt == objectapi.TagLightCFunc {
			return false // different tags already checked above
		}
		return t1.Val == t2.Val
	}
}

// RawEqualObj performs raw equality (no metamethods).
func RawEqualObj(t1, t2 objectapi.TValue) bool {
	if t1.Tt != t2.Tt {
		if t1.IsNumber() && t2.IsNumber() {
			if t1.IsInteger() && t2.IsFloat() {
				i2, ok := FloatToInteger(t2.Val.(float64))
				return ok && t1.Val.(int64) == i2
			}
			if t1.IsFloat() && t2.IsInteger() {
				i1, ok := FloatToInteger(t1.Val.(float64))
				return ok && i1 == t2.Val.(int64)
			}
		}
		return false
	}
	switch t1.Tt {
	case objectapi.TagNil:
		return true
	case objectapi.TagFalse, objectapi.TagTrue:
		return t1.Tt == t2.Tt
	case objectapi.TagInteger:
		i1, ok1 := t1.Val.(int64)
		i2, ok2 := t2.Val.(int64)
		if ok1 && ok2 {
			return i1 == i2
		}
		// Defensive: tag says integer but value may be float
		f1 := toFloat(t1)
		f2 := toFloat(t2)
		return f1 == f2
	case objectapi.TagFloat:
		f1, ok1 := t1.Val.(float64)
		f2, ok2 := t2.Val.(float64)
		if ok1 && ok2 {
			return f1 == f2
		}
		ff1 := toFloat(t1)
		ff2 := toFloat(t2)
		return ff1 == ff2
	case objectapi.TagShortStr, objectapi.TagLongStr:
		return t1.Val.(*objectapi.LuaString).Data == t2.Val.(*objectapi.LuaString).Data
	case objectapi.TagLuaClosure, objectapi.TagCClosure:
		return t1.Val == t2.Val // pointer comparison
	case objectapi.TagLightCFunc:
		return objectapi.LightCFuncEqual(t1.Val, t2.Val)
	default:
		return t1.Val == t2.Val
	}
}

// getTableTM gets a metamethod from a table's metatable.
func getTableTM(g *stateapi.GlobalState, t *tableapi.Table, event mmapi.TMS) objectapi.TValue {
	mt := t.GetMetatable()
	if mt == nil {
		return objectapi.Nil
	}
	if g == nil {
		return objectapi.Nil
	}
	tmName := g.TMNames[event]
	if tmName == nil {
		return objectapi.Nil
	}
	v, _ := mt.GetStr(tmName)
	return v
}

// ---------------------------------------------------------------------------
// Metamethod call helpers
// ---------------------------------------------------------------------------

// callTMRes calls a metamethod with two arguments and returns the result.
// IMPORTANT: Uses L.Top as scratch space. Callers must ensure L.Top is above
// any registers they need preserved (PosCall moves result to the func slot).
func callTMRes(L *stateapi.LuaState, tm, p1, p2 objectapi.TValue) objectapi.TValue {
	// Push: tm, p1, p2
	top := L.Top
	L.Stack[top].Val = tm
	L.Stack[top+1].Val = p1
	L.Stack[top+2].Val = p2
	L.Top = top + 3
	Call(L, top, 1)
	result := L.Stack[top].Val // PosCall moves result to func slot
	L.Top = top                // restore top
	return result
}

// callTM calls a metamethod with 3 args (tm, p1, p2, p3) — for __newindex etc.
func callTM(L *stateapi.LuaState, tm, p1, p2, p3 objectapi.TValue) {
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
func opErrorMsg(L *stateapi.LuaState, p1, p2 objectapi.TValue, op string, reg1, reg2 int) string {
	// Pick the wrong operand (first non-number)
	badReg := reg1
	badVal := p1
	if p1.IsNumber() {
		badReg = reg2
		badVal = p2
	}
	info := ""
	if badReg >= 0 {
		info = VarInfo(L, badReg)
	}
	return "attempt to " + op + " a " + mmapi.ObjTypeName(L.Global, badVal) + " value" + info
}

// tryBinTM tries a binary metamethod.
// reg1, reg2 are register hints for p1, p2 (-1 if not a register).
func tryBinTM(L *stateapi.LuaState, p1, p2 objectapi.TValue, res int, event mmapi.TMS, reg1, reg2 int) {
	tm := mmapi.GetTMByObj(L.Global, p1, event)
	if tm.IsNil() {
		tm = mmapi.GetTMByObj(L.Global, p2, event)
	}
	if tm.IsNil() {
		if event == mmapi.TM_CONCAT {
			RunError(L, opErrorMsg(L, p1, p2, "concatenate", reg1, reg2))
		}
		if event >= mmapi.TM_BAND && event <= mmapi.TM_SHR || event == mmapi.TM_BNOT {
			// If both are numbers but can't convert to int, give specific error
			// Mirrors: luaG_tointerror in ldebug.c
			if p1.IsNumber() && p2.IsNumber() {
				// Find the non-integer operand for the error message
				badReg := reg2
				if p1.IsFloat() {
					badReg = reg1
				}
				RunError(L, "number"+VarInfo(L, badReg)+" has no integer representation")
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
func tryBiniTM(L *stateapi.LuaState, p1 objectapi.TValue, imm int, flip bool, res int, event mmapi.TMS, reg1 int) {
	p2 := objectapi.MakeInteger(int64(imm))
	if flip {
		tryBinTM(L, p2, p1, res, event, -1, reg1)
	} else {
		tryBinTM(L, p1, p2, res, event, reg1, -1)
	}
}

// tryBinKTM tries a binary metamethod with a constant operand (possibly flipped).
func tryBinKTM(L *stateapi.LuaState, p1, p2 objectapi.TValue, flip bool, res int, event mmapi.TMS, reg1 int) {
	if flip {
		tryBinTM(L, p2, p1, res, event, -1, reg1)
	} else {
		tryBinTM(L, p1, p2, res, event, reg1, -1)
	}
}

// tryConcatTM tries the __concat metamethod for two values.
func tryConcatTM(L *stateapi.LuaState) {
	top := L.Top
	p1 := L.Stack[top-2].Val
	p2 := L.Stack[top-1].Val
	tm := mmapi.GetTMByObj(L.Global, p1, mmapi.TM_CONCAT)
	if tm.IsNil() {
		tm = mmapi.GetTMByObj(L.Global, p2, mmapi.TM_CONCAT)
	}
	if tm.IsNil() {
		// Report the type of the non-string/non-number operand
		errType := p1
		if p1.IsString() || p1.IsNumber() {
			errType = p2
		}
		RunError(L, "attempt to concatenate a "+mmapi.ObjTypeName(L.Global, errType)+" value")
	}
	result := callTMRes(L, tm, p1, p2)
	L.Stack[top-2].Val = result
	L.Top = top - 1
}

// ---------------------------------------------------------------------------
// String/Number coercion for concat
// ---------------------------------------------------------------------------

func toStringForConcat(v objectapi.TValue) (string, bool) {
	switch v.Tt {
	case objectapi.TagShortStr, objectapi.TagLongStr:
		return v.Val.(*objectapi.LuaString).Data, true
	case objectapi.TagInteger:
		return intToString(v.Val.(int64)), true
	case objectapi.TagFloat:
		return floatToString(v.Val.(float64)), true
	}
	return "", false
}

func intToString(i int64) string {
	// Simple integer to string
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
		if i < 0 { // MinInt64
			return "-9223372036854775808"
		}
	}
	buf := make([]byte, 0, 20)
	for i > 0 {
		buf = append(buf, byte('0'+i%10))
		i /= 10
	}
	// Reverse
	for l, r := 0, len(buf)-1; l < r; l, r = l+1, r-1 {
		buf[l], buf[r] = buf[r], buf[l]
	}
	if neg {
		return "-" + string(buf)
	}
	return string(buf)
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
func internString(L *stateapi.LuaState, s string) *objectapi.LuaString {
	st := L.Global.StringTable.(*luastringapi.StringTable)
	return st.Intern(s)
}

// makeInternedString creates a TValue string that is properly interned.
func makeInternedString(L *stateapi.LuaState, s string) objectapi.TValue {
	return objectapi.MakeString(internString(L, s))
}

// Concat concatenates 'total' values on the stack from L.Top-total to L.Top-1.
func Concat(L *stateapi.LuaState, total int) {
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
			// Collect as many strings as possible
			var parts []string
			parts = append(parts, s1, s2)
			for n < total {
				sv, ok := toStringForConcat(L.Stack[top-n-1].Val)
				if !ok {
					break
				}
				parts = append([]string{sv}, parts...)
				n++
			}
			result := strings.Join(parts, "")
			L.Stack[top-n].Val = makeInternedString(L, result)
			total -= n - 1
			L.Top -= n - 1
		}
	}
}

// ---------------------------------------------------------------------------
// ObjLen — # operator
// ---------------------------------------------------------------------------

// ObjLen computes #rb and stores in L.Stack[ra].
func ObjLen(L *stateapi.LuaState, ra int, rb objectapi.TValue) {
	switch rb.Tt {
	case objectapi.TagTable:
		h := rb.Val.(*tableapi.Table)
		mt := h.GetMetatable()
		if mt != nil {
			tm := getTableTM(L.Global, h, mmapi.TM_LEN)
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
		L.Stack[ra].Val = objectapi.MakeInteger(h.RawLen())
	case objectapi.TagShortStr, objectapi.TagLongStr:
		s := rb.Val.(*objectapi.LuaString)
		L.Stack[ra].Val = objectapi.MakeInteger(int64(len(s.Data)))
	default:
		tm := mmapi.GetTMByObj(L.Global, rb, mmapi.TM_LEN)
		if tm.IsNil() {
			RunError(L, "attempt to get length of a "+mmapi.ObjTypeName(L.Global, rb)+" value")
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
func FinishGet(L *stateapi.LuaState, t, key objectapi.TValue, ra int) {
	for loop := 0; loop < maxTagLoop; loop++ {
		var tm objectapi.TValue
		if t.IsTable() {
			h := t.Val.(*tableapi.Table)
			tm = getTableTM(L.Global, h, mmapi.TM_INDEX)
			if tm.IsNil() {
				L.Stack[ra].Val = objectapi.Nil
				return
			}
		} else {
			tm = mmapi.GetTMByObj(L.Global, t, mmapi.TM_INDEX)
			if tm.IsNil() {
				RunTypeErrorByVal(L, t, "index")
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
			h := t.Val.(*tableapi.Table)
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
func tableSetWithMeta(L *stateapi.LuaState, tval objectapi.TValue, key, val objectapi.TValue) {
	// Check for nil/NaN key — C Lua raises luaG_runerror, not panic
	if key.IsNil() {
		RunError(L, "table index is nil")
	}
	if key.IsFloat() && math.IsNaN(key.Float()) {
		RunError(L, "table index is NaN")
	}
	h := tval.Val.(*tableapi.Table)
	// Fast path: key already exists → just overwrite
	_, found := h.Get(key)
	if found {
		h.Set(key, val)
		return
	}
	// Key absent — check for __newindex metamethod
	tm := getTableTM(L.Global, h, mmapi.TM_NEWINDEX)
	if tm.IsNil() {
		// No metamethod — raw set
		h.Set(key, val)
		return
	}
	// Has __newindex — delegate to FinishSet which handles the chain
	FinishSet(L, tval, key, val)
}

// FinishSet completes a table set with __newindex metamethod chain.
func FinishSet(L *stateapi.LuaState, t, key, val objectapi.TValue) {
	for loop := 0; loop < maxTagLoop; loop++ {
		var tm objectapi.TValue
		if t.IsTable() {
			h := t.Val.(*tableapi.Table)
			tm = getTableTM(L.Global, h, mmapi.TM_NEWINDEX)
			if tm.IsNil() {
				h.Set(key, val)
				return
			}
		} else {
			tm = mmapi.GetTMByObj(L.Global, t, mmapi.TM_NEWINDEX)
			if tm.IsNil() {
				RunTypeErrorByVal(L, t, "index")
			}
		}
		if tm.IsFunction() {
			callTM(L, tm, t, key, val)
			return
		}
		// tm is a table — repeat
		t = tm
		if t.IsTable() {
			h := t.Val.(*tableapi.Table)
			_, found := h.Get(key)
			if found {
				h.Set(key, val)
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
func forerror(L *stateapi.LuaState, val objectapi.TValue, what string) {
	RunError(L, "bad 'for' "+what+" (number expected, got "+mmapi.ObjTypeName(L.Global, val)+")")
}


// forLimit implements C Lua's forlimit: convert a for-loop limit to integer
// using floor (step>0) or ceil (step<0) rounding. Returns true to skip the loop.
// This matches lvm.c forlimit exactly.
func forLimit(L *stateapi.LuaState, init int64, plimit objectapi.TValue, limit *int64, step int64) bool {
	// First try exact integer conversion (handles integer values and exact float-to-int)
	if li, ok := flttointeger(plimit, step); ok {
		*limit = li
	} else {
		// Not coercible to integer with floor/ceil rounding
		fl, ok := ToNumber(plimit)
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
func flttointeger(v objectapi.TValue, step int64) (int64, bool) {
	switch v.Tt {
	case objectapi.TagInteger:
		return v.Val.(int64), true
	case objectapi.TagFloat:
		f := v.Val.(float64)
		return floatToIntegerRounded(f, step)
	case objectapi.TagShortStr, objectapi.TagLongStr:
		// String: try to parse as number, then convert with rounding
		s := v.Val.(*objectapi.LuaString).Data
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

// ForPrep prepares a numeric for loop. Returns true to skip the loop.
func ForPrep(L *stateapi.LuaState, ra int) bool {
	pinit := L.Stack[ra].Val
	plimit := L.Stack[ra+1].Val
	pstep := L.Stack[ra+2].Val

	if pinit.IsInteger() && pstep.IsInteger() {
		init := pinit.Val.(int64)
		step := pstep.Val.(int64)
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
			count /= uint64(-(step+1)) + 1
		}
		// Rearrange: ra=count, ra+1=step, ra+2=init (control variable)
		L.Stack[ra].Val = objectapi.MakeInteger(int64(count))
		L.Stack[ra+1].Val = objectapi.MakeInteger(step)
		L.Stack[ra+2].Val = objectapi.MakeInteger(init)
	} else {
		// Float loop
		finit, ok1 := ToNumber(pinit)
		flimit, ok2 := ToNumber(plimit)
		fstep, ok3 := ToNumber(pstep)
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
		L.Stack[ra].Val = objectapi.MakeFloat(flimit)
		L.Stack[ra+1].Val = objectapi.MakeFloat(fstep)
		L.Stack[ra+2].Val = objectapi.MakeFloat(finit)
	}
	return false
}

// ForLoop performs one iteration of a numeric for loop.
// Returns true if the loop should continue (jump back).
func ForLoop(L *stateapi.LuaState, ra int) bool {
	if L.Stack[ra+1].Val.IsInteger() {
		// Integer loop: ra=count, ra+1=step, ra+2=control
		count := uint64(L.Stack[ra].Val.Integer())
		if count > 0 {
			step := L.Stack[ra+1].Val.Integer()
			idx := L.Stack[ra+2].Val.Integer()
			L.Stack[ra].Val = objectapi.MakeInteger(int64(count - 1))
			idx += step
			L.Stack[ra+2].Val = objectapi.MakeInteger(idx)
			return true
		}
	} else {
		// Float loop: ra=limit, ra+1=step, ra+2=control
		step := L.Stack[ra+1].Val.Float()
		limit := L.Stack[ra].Val.Float()
		idx := L.Stack[ra+2].Val.Float()
		idx += step
		if step > 0 {
			if idx <= limit {
				L.Stack[ra+2].Val = objectapi.MakeFloat(idx)
				return true
			}
		} else {
			if limit <= idx {
				L.Stack[ra+2].Val = objectapi.MakeFloat(idx)
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Closure creation
// ---------------------------------------------------------------------------

// PushClosure creates a new Lua closure and stores it in L.Stack[ra].
func PushClosure(L *stateapi.LuaState, p *objectapi.Proto, encup []*closureapi.UpVal, base, ra int) {
	nup := len(p.Upvalues)
	ncl := closureapi.NewLClosure(p, nup)
	L.Global.LinkGC(ncl) // V5: register in allgc chain
	L.Stack[ra].Val = objectapi.TValue{Tt: objectapi.TagLuaClosure, Val: ncl}
	for i := 0; i < nup; i++ {
		if p.Upvalues[i].InStack {
			ncl.UpVals[i] = closureapi.FindUpval(L, base+int(p.Upvalues[i].Idx))
		} else {
			ncl.UpVals[i] = encup[p.Upvalues[i].Idx]
		}
	}
}

// ---------------------------------------------------------------------------
// Vararg handling
// ---------------------------------------------------------------------------

// AdjustVarargs adjusts the stack for a vararg function call.
// Mirrors C Lua's luaT_adjustvarargs / buildhiddenargs.
// Layout after adjustment:
//   [old ci.Func] ... [extra args] [func copy] [fixed params] ...
//   ci.Func now points to the func copy (after the extra args).
func AdjustVarargs(L *stateapi.LuaState, ci *stateapi.CallInfo, p *objectapi.Proto) {
	nfixparams := int(p.NumParams)
	totalargs := L.Top - ci.Func - 1
	nextra := totalargs - nfixparams
	if nextra < 0 {
		// Fill missing fixed params with nil
		for i := totalargs; i < nfixparams; i++ {
			L.Stack[L.Top].Val = objectapi.Nil
			L.Top++
		}
		nextra = 0
		totalargs = nfixparams
	}

	if p.Flag&objectapi.PF_VATAB != 0 {
		// === PF_VATAB path: create vararg table ===
		// Mirrors: luaT_adjustvarargs + createvarargtab in ltm.c
		CheckStack(L, int(p.MaxStackSize)+1)
		t := tableapi.New(nextra, 1)
		L.Global.LinkGC(t) // V5: register in allgc chain
		size := t.EstimateBytes()
		atomic.AddInt64(&L.Global.GCTotalBytes, size)
		// Register dealloc cleanup (coexists with any __gc SetFinalizer)
		gcTotalBytes := &L.Global.GCTotalBytes
		runtime.AddCleanup(t, func(sz int64) {
			atomic.AddInt64(gcTotalBytes, -sz)
		}, size)
		// Set t.n = nextra
		st := L.Global.StringTable.(*luastringapi.StringTable)
		nKey := objectapi.MakeString(st.Intern("n"))
		t.Set(nKey, objectapi.MakeInteger(int64(nextra)))
		// Set t[1..nextra] = extra args
		for i := 0; i < nextra; i++ {
			t.SetInt(int64(i+1), L.Stack[ci.Func+nfixparams+1+i].Val)
		}
		// Place table at the vararg parameter slot (after fixed params)
		L.Stack[ci.Func+nfixparams+1].Val = objectapi.TValue{Tt: objectapi.TagTable, Val: t}
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
			L.Stack[ci.Func+i].Val = objectapi.Nil // erase original (for GC)
			L.Top++
		}
		// ci.Func now lives after the hidden (extra) arguments
		ci.Func += totalargs + 1
		ci.Top = ci.Func + 1 + int(p.MaxStackSize)
		// Set vararg parameter slot to nil (mirrors C Lua: setnilvalue)
		L.Stack[ci.Func+nfixparams+1].Val = objectapi.Nil
	}
}

// GetVarargs copies vararg values to the stack starting at ra.
// When vatab >= 0, reads from the vararg table at ci.Func+vatab+1.
// When vatab < 0, reads from hidden stack args below ci.Func.
// Mirrors: luaT_getvarargs in ltm.c
func GetVarargs(L *stateapi.LuaState, ci *stateapi.CallInfo, ra int, n int, vatab int) {
	var h *tableapi.Table
	if vatab >= 0 {
		h = L.Stack[ci.Func+vatab+1].Val.Val.(*tableapi.Table)
	}

	// Get number of available vararg args — mirrors getnumargs() in ltm.c
	var nExtra int
	if h == nil {
		nExtra = ci.NExtraArgs
	} else {
		// Read t.n from the vararg table and validate
		st := L.Global.StringTable.(*luastringapi.StringTable)
		nKey := objectapi.MakeString(st.Intern("n"))
		nVal, ok := h.Get(nKey)
		if !ok || nVal.Tt != objectapi.TagInteger {
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
				L.Stack[ra+i].Val = objectapi.Nil
			}
		}
	}
	// Fill remaining with nil
	for i := touse; i < n; i++ {
		L.Stack[ra+i].Val = objectapi.Nil
	}
}

// ---------------------------------------------------------------------------
// Execute — the main VM execution loop
// ---------------------------------------------------------------------------

// FinishOp finishes execution of an opcode interrupted by a yield.
// When a metamethod yields (via panic(LuaYield{})), intermediate Go frames
// (callTMRes, tryBinTM, callOrderTM) are destroyed. This function places
// the metamethod result into the correct register before Execute resumes.
// Also handles __close interruption (OP_CLOSE/OP_RETURN).
// Mirrors: luaV_finishOp in lvm.c:568-618
func FinishOp(L *stateapi.LuaState, ci *stateapi.CallInfo) {
	cl := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
	code := cl.Proto.Code
	inst := code[ci.SavedPC-1] // interrupted instruction
	op := opcodeapi.GetOpCode(inst)
	base := ci.Func + 1
	switch op {

	// Category 1: Binary arithmetic metamethods
	// savedpc-1 = OP_MMBIN/MMBINI/MMBINK (the interrupted instruction)
	// savedpc-2 = OP_ADD/SUB/etc (the original arithmetic instruction)
	// Pop TM result from stack top → store in RA of original arith op
	case opcodeapi.OP_MMBIN, opcodeapi.OP_MMBINI, opcodeapi.OP_MMBINK:
		L.Top--
		prevInst := code[ci.SavedPC-2]
		dest := base + opcodeapi.GetArgA(prevInst)
		L.Stack[dest].Val = L.Stack[L.Top].Val

	// Category 2: Unary + Table Get
	// Pop TM result from stack top → store in RA of this instruction
	case opcodeapi.OP_UNM, opcodeapi.OP_BNOT, opcodeapi.OP_LEN,
		opcodeapi.OP_GETTABUP, opcodeapi.OP_GETTABLE, opcodeapi.OP_GETI,
		opcodeapi.OP_GETFIELD, opcodeapi.OP_SELF:
		L.Top--
		dest := base + opcodeapi.GetArgA(inst)
		L.Stack[dest].Val = L.Stack[L.Top].Val

	// Category 3: Comparisons
	// Evaluate TM result as boolean, then conditional jump.
	// savedpc points to the OP_JMP instruction after the comparison.
	// If res != k, skip the jump (savedpc++).
	case opcodeapi.OP_LT, opcodeapi.OP_LE,
		opcodeapi.OP_LTI, opcodeapi.OP_LEI,
		opcodeapi.OP_GTI, opcodeapi.OP_GEI,
		opcodeapi.OP_EQ:
		res := !L.Stack[L.Top-1].Val.IsFalsy()
		L.Top--
		if res != (opcodeapi.GetArgK(inst) != 0) {
			ci.SavedPC++ // skip jump
		}

	// Category 4: Concat
	// Reposition TM result, adjust top, continue concat loop
	case opcodeapi.OP_CONCAT:
		top := L.Top - 1
		a := opcodeapi.GetArgA(inst)
		total := (top - 1) - (base + a)
		L.Stack[top-2].Val = L.Stack[top].Val // put TM result in proper position
		L.Top = top - 1
		if total > 1 {
			Concat(L, total) // concat remaining (may yield again)
		}

	// Category 5: Close/Return (already implemented)
	case opcodeapi.OP_CLOSE:
		ci.SavedPC--
	case opcodeapi.OP_RETURN:
		ra := base + opcodeapi.GetArgA(inst)
		L.Top = ra + ci.NRes
		ci.SavedPC--

	default:
		// OP_TFORCALL, OP_CALL, OP_TAILCALL,
		// OP_SETTABUP, OP_SETTABLE, OP_SETI, OP_SETFIELD
		// No action needed — results already in correct place or no result needed
	}
}

// Execute runs the VM main loop for the given CallInfo.
// This is the Go equivalent of luaV_execute in lvm.c.
func Execute(L *stateapi.LuaState, ci *stateapi.CallInfo) {
startfunc:
	cl := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
	k := cl.Proto.Constants
	code := cl.Proto.Code
	base := ci.Func + 1

	// Mirrors: luaG_tracecall in ldebug.c — fire call hook at function entry.
	// For tail calls, PreTailCall sets CISTTail and savedpc=0, then jumps here.
	// The call hook for non-tail calls is already fired by PreCall (non-vararg)
	// or OP_VARARGPREP (vararg). Only tail calls need this path.
	// For vararg tail calls, defer to OP_VARARGPREP.
	if L.HookMask != 0 && ci.CallStatus&stateapi.CISTTail != 0 &&
		ci.SavedPC == 0 && !cl.Proto.IsVararg() {
		CallHook(L, ci)
	}

	for {
		inst := code[ci.SavedPC]
		ci.SavedPC++ // increment BEFORE hook check — mirrors C Lua vmfetch

		// Hook dispatch: fire line/count hooks if active.
		// Skip for OP_VARARGPREP — C Lua's luaG_tracecall returns 0 (trap=0)
		// for vararg functions, so traceexec is not called for instruction 0.
		// The call hook and OldPC adjustment happen inside OP_VARARGPREP instead.
		// Mirrors: vmfetch trap check in lvm.c + luaG_tracecall in ldebug.c
		if L.HookMask&(stateapi.MaskLine|stateapi.MaskCount) != 0 && L.AllowHook &&
			opcodeapi.GetOpCode(inst) != opcodeapi.OP_VARARGPREP {
			TraceExec(L, ci)
		}
		op := opcodeapi.GetOpCode(inst)
		ra := base + opcodeapi.GetArgA(inst)

		switch op {

		// ===== Load/Move =====

		case opcodeapi.OP_MOVE:
			rb := base + opcodeapi.GetArgB(inst)
			L.Stack[ra].Val = L.Stack[rb].Val

		case opcodeapi.OP_LOADI:
			L.Stack[ra].Val = objectapi.MakeInteger(int64(opcodeapi.GetArgSBx(inst)))

		case opcodeapi.OP_LOADF:
			L.Stack[ra].Val = objectapi.MakeFloat(float64(opcodeapi.GetArgSBx(inst)))

		case opcodeapi.OP_LOADK:
			L.Stack[ra].Val = k[opcodeapi.GetArgBx(inst)]

		case opcodeapi.OP_LOADKX:
			ax := opcodeapi.GetArgAx(code[ci.SavedPC])
			ci.SavedPC++
			L.Stack[ra].Val = k[ax]

		case opcodeapi.OP_LOADFALSE:
			L.Stack[ra].Val = objectapi.False

		case opcodeapi.OP_LFALSESKIP:
			L.Stack[ra].Val = objectapi.False
			ci.SavedPC++ // skip next instruction

		case opcodeapi.OP_LOADTRUE:
			L.Stack[ra].Val = objectapi.True

		case opcodeapi.OP_LOADNIL:
			b := opcodeapi.GetArgB(inst)
			for i := 0; i <= b; i++ {
				L.Stack[ra+i].Val = objectapi.Nil
			}

		// ===== Upvalues =====

		case opcodeapi.OP_GETUPVAL:
			b := opcodeapi.GetArgB(inst)
			L.Stack[ra].Val = cl.UpVals[b].Get(L.Stack)

		case opcodeapi.OP_SETUPVAL:
			b := opcodeapi.GetArgB(inst)
			cl.UpVals[b].Set(L.Stack, L.Stack[ra].Val)

		case opcodeapi.OP_CLOSE:
			closureapi.CloseUpvals(L, ra)
			CloseTBC(L, ra)

		case opcodeapi.OP_TBC:
			// To-be-closed: mark the variable in the TBC linked list
			MarkTBC(L, ra)

		// ===== Table access =====

		case opcodeapi.OP_GETTABUP:
			b := opcodeapi.GetArgB(inst)
			upval := cl.UpVals[b].Get(L.Stack)
			rc := k[opcodeapi.GetArgC(inst)]
			if upval.IsTable() {
				h := upval.Val.(*tableapi.Table)
				val, found := h.Get(rc)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, upval, rc, ra)
				}
			} else {
				FinishGet(L, upval, rc, ra)
			}

		case opcodeapi.OP_GETTABLE:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsTable() {
				h := rb.Val.(*tableapi.Table)
				val, found := h.Get(rc)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, rb, rc, ra)
				}
			} else {
				FinishGet(L, rb, rc, ra)
			}

		case opcodeapi.OP_GETI:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			c := int64(opcodeapi.GetArgC(inst))
			if rb.IsTable() {
				h := rb.Val.(*tableapi.Table)
				val, found := h.GetInt(c)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, rb, objectapi.MakeInteger(c), ra)
				}
			} else {
				FinishGet(L, rb, objectapi.MakeInteger(c), ra)
			}

		case opcodeapi.OP_GETFIELD:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := k[opcodeapi.GetArgC(inst)]
			if rb.IsTable() {
				h := rb.Val.(*tableapi.Table)
				val, found := h.Get(rc)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, rb, rc, ra)
				}
			} else {
				FinishGet(L, rb, rc, ra)
			}

		case opcodeapi.OP_SETTABUP:
			b := opcodeapi.GetArgB(inst)
			upval := cl.UpVals[opcodeapi.GetArgA(inst)].Get(L.Stack)
			rb := k[b]
			var rc objectapi.TValue
			if opcodeapi.GetArgK(inst) != 0 {
				rc = k[opcodeapi.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcodeapi.GetArgC(inst)].Val
			}
			ra = 0 // OP_SETTABUP uses A for upvalue index, not register
			if upval.IsTable() {
				tableSetWithMeta(L, upval, rb, rc)
			} else {
				FinishSet(L, upval, rb, rc)
			}

		case opcodeapi.OP_SETTABLE:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			var rc objectapi.TValue
			if opcodeapi.GetArgK(inst) != 0 {
				rc = k[opcodeapi.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcodeapi.GetArgC(inst)].Val
			}
			tval := L.Stack[ra].Val
			if tval.IsTable() {
				tableSetWithMeta(L, tval, rb, rc)
			} else {
				FinishSet(L, tval, rb, rc)
			}

		case opcodeapi.OP_SETI:
			b := int64(opcodeapi.GetArgB(inst))
			var rc objectapi.TValue
			if opcodeapi.GetArgK(inst) != 0 {
				rc = k[opcodeapi.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcodeapi.GetArgC(inst)].Val
			}
			tval := L.Stack[ra].Val
			if tval.IsTable() {
				tableSetWithMeta(L, tval, objectapi.MakeInteger(b), rc)
			} else {
				FinishSet(L, tval, objectapi.MakeInteger(b), rc)
			}

		case opcodeapi.OP_SETFIELD:
			rb := k[opcodeapi.GetArgB(inst)]
			var rc objectapi.TValue
			if opcodeapi.GetArgK(inst) != 0 {
				rc = k[opcodeapi.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcodeapi.GetArgC(inst)].Val
			}
			tval := L.Stack[ra].Val
			if tval.IsTable() {
				tableSetWithMeta(L, tval, rb, rc)
			} else {
				FinishSet(L, tval, rb, rc)
			}

		case opcodeapi.OP_NEWTABLE:
			b := opcodeapi.GetArgVB(inst)
			c := opcodeapi.GetArgVC(inst)
			if b > 0 {
				b = 1 << (b - 1)
			}
			if opcodeapi.GetArgK(inst) != 0 {
				c += opcodeapi.GetArgAx(code[ci.SavedPC]) * (opcodeapi.MaxArgVC + 1)
			}
			ci.SavedPC++ // skip extra arg
			t := tableapi.New(c, b)
			L.Global.LinkGC(t) // V5: register in allgc chain
			size := t.EstimateBytes()
			atomic.AddInt64(&L.Global.GCTotalBytes, size)
			// Register dealloc cleanup (coexists with any __gc SetFinalizer)
			gcTotalBytes := &L.Global.GCTotalBytes
			runtime.AddCleanup(t, func(sz int64) {
				atomic.AddInt64(gcTotalBytes, -sz)
			}, size)
			L.Stack[ra].Val = objectapi.TValue{Tt: objectapi.TagTable, Val: t}

			// Periodic GC: run Lua GC during tight allocation loops.
			// V5 GC handles __gc via finobj list.
			if g := L.Global; !g.GCStopped {
				checkPeriodicGC(g, L)
			}

		case opcodeapi.OP_SELF:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := k[opcodeapi.GetArgC(inst)]
			L.Stack[ra+1].Val = rb // save table as self
			if rb.IsTable() {
				h := rb.Val.(*tableapi.Table)
				val, found := h.Get(rc)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, rb, rc, ra)
				}
			} else {
				FinishGet(L, rb, rc, ra)
			}

		// ===== Arithmetic with immediate =====

		case opcodeapi.OP_ADDI:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ic := int64(opcodeapi.GetArgSC(inst))
			if rb.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() + ic)
				ci.SavedPC++ // skip MMBIN
			} else if rb.IsFloat() {
				L.Stack[ra].Val = objectapi.MakeFloat(rb.Float() + float64(ic))
				ci.SavedPC++
			}
			// else: fall through to MMBINI on next instruction

		// ===== Arithmetic with constant =====

		case opcodeapi.OP_ADDK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() + kc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a + b }, func(a, b float64) float64 { return a + b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_SUBK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() - kc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a - b }, func(a, b float64) float64 { return a - b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_MULK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() * kc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a * b }, func(a, b float64) float64 { return a * b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_MODK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(IMod(L, rb.Integer(), kc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = objectapi.MakeInteger(IMod(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = objectapi.MakeFloat(FMod(toFloat(nb), toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_POWK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			nb, ok1 := ToNumberNS(rb)
			nc, ok2 := ToNumberNS(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeFloat(math.Pow(nb, nc))
				ci.SavedPC++
			}

		case opcodeapi.OP_DIVK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			nb, ok1 := ToNumberNS(rb)
			nc, ok2 := ToNumberNS(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeFloat(nb / nc)
				ci.SavedPC++
			}

		case opcodeapi.OP_IDIVK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(IDiv(L, rb.Integer(), kc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = objectapi.MakeInteger(IDiv(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = objectapi.MakeFloat(math.Floor(toFloat(nb) / toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		// ===== Bitwise with constant =====

		case opcodeapi.OP_BANDK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib & ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BORK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib | ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BXORK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib ^ ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_SHLI:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ic := int64(opcodeapi.GetArgSC(inst))
			ib, ok := toIntegerStrict(rb)
			if ok {
				L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ic, ib))
				ci.SavedPC++
			}

		case opcodeapi.OP_SHRI:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ic := int64(opcodeapi.GetArgSC(inst))
			ib, ok := toIntegerStrict(rb)
			if ok {
				L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ib, -ic))
				ci.SavedPC++
			}

		// ===== Arithmetic register-register =====

		case opcodeapi.OP_ADD:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() + rc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a + b }, func(a, b float64) float64 { return a + b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_SUB:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() - rc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a - b }, func(a, b float64) float64 { return a - b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_MUL:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() * rc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a * b }, func(a, b float64) float64 { return a * b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_MOD:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(IMod(L, rb.Integer(), rc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = objectapi.MakeInteger(IMod(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = objectapi.MakeFloat(FMod(toFloat(nb), toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_POW:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			nb, ok1 := ToNumberNS(rb)
			nc, ok2 := ToNumberNS(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeFloat(math.Pow(nb, nc))
				ci.SavedPC++
			}

		case opcodeapi.OP_DIV:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			nb, ok1 := ToNumberNS(rb)
			nc, ok2 := ToNumberNS(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeFloat(nb / nc)
				ci.SavedPC++
			}

		case opcodeapi.OP_IDIV:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(IDiv(L, rb.Integer(), rc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = objectapi.MakeInteger(IDiv(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = objectapi.MakeFloat(math.Floor(toFloat(nb) / toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		// ===== Bitwise register-register =====

		case opcodeapi.OP_BAND:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib & ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BOR:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib | ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BXOR:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib ^ ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_SHL:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ib, ic))
				ci.SavedPC++
			}

		case opcodeapi.OP_SHR:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ib, -ic))
				ci.SavedPC++
			}

		// ===== Metamethod fallback =====

		case opcodeapi.OP_MMBIN:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			tm := mmapi.TMS(opcodeapi.GetArgC(inst))
			prevInst := code[ci.SavedPC-2]
			result := base + opcodeapi.GetArgA(prevInst)
			tryBinTM(L, L.Stack[ra].Val, rb, result, tm, ra-base, opcodeapi.GetArgB(inst))

		case opcodeapi.OP_MMBINI:
			imm := opcodeapi.GetArgSB(inst)
			tm := mmapi.TMS(opcodeapi.GetArgC(inst))
			flip := opcodeapi.GetArgK(inst) != 0
			prevInst := code[ci.SavedPC-2]
			result := base + opcodeapi.GetArgA(prevInst)
			tryBiniTM(L, L.Stack[ra].Val, imm, flip, result, tm, ra-base)

		case opcodeapi.OP_MMBINK:
			imm := k[opcodeapi.GetArgB(inst)]
			tm := mmapi.TMS(opcodeapi.GetArgC(inst))
			flip := opcodeapi.GetArgK(inst) != 0
			prevInst := code[ci.SavedPC-2]
			result := base + opcodeapi.GetArgA(prevInst)
			tryBinKTM(L, L.Stack[ra].Val, imm, flip, result, tm, ra-base)

		// ===== Unary =====

		case opcodeapi.OP_UNM:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			if rb.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(-rb.Integer())
			} else if rb.IsFloat() {
				L.Stack[ra].Val = objectapi.MakeFloat(-rb.Float())
			} else {
				tryBinTM(L, rb, rb, ra, mmapi.TM_UNM, opcodeapi.GetArgB(inst), opcodeapi.GetArgB(inst))
			}

		case opcodeapi.OP_BNOT:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ib, ok := toIntegerStrict(rb)
			if ok {
				L.Stack[ra].Val = objectapi.MakeInteger(^ib)
			} else {
				tryBinTM(L, rb, rb, ra, mmapi.TM_BNOT, opcodeapi.GetArgB(inst), opcodeapi.GetArgB(inst))
			}

		case opcodeapi.OP_NOT:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			if rb.IsFalsy() {
				L.Stack[ra].Val = objectapi.True
			} else {
				L.Stack[ra].Val = objectapi.False
			}

		case opcodeapi.OP_LEN:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ObjLen(L, ra, rb)

		case opcodeapi.OP_CONCAT:
			n := opcodeapi.GetArgB(inst)
			L.Top = ra + n
			Concat(L, n)
			L.Stack[ra].Val = L.Stack[L.Top-1].Val
			L.Top = ci.Top // restore top

			// Periodic GC: string concatenation allocates new strings.
			if g := L.Global; !g.GCStopped {
				checkPeriodicGC(g, L)
			}

		// ===== Comparison =====

		case opcodeapi.OP_EQ:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			// Bump L.Top to frame top so callTMRes scratch space
			// doesn't clobber live registers (metamethod dispatch).
			L.Top = ci.Top
			cond := EqualObj(L, L.Stack[ra].Val, rb)
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++ // skip jump
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_LT:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			L.Top = ci.Top
			cond := LessThan(L, L.Stack[ra].Val, rb)
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_LE:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			L.Top = ci.Top
			cond := LessEqual(L, L.Stack[ra].Val, rb)
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_EQK:
			rb := k[opcodeapi.GetArgB(inst)]
			cond := RawEqualObj(L.Stack[ra].Val, rb)
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_EQI:
			im := int64(opcodeapi.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() == im
			} else if v.IsFloat() {
				cond = v.Float() == float64(im)
			}
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_LTI:
			im := int64(opcodeapi.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() < im
			} else if v.IsFloat() {
				cond = v.Float() < float64(im)
			} else {
				// Metamethod fallback: callOrderITM(L, v, im, false, isf, TM_LT)
				isf := opcodeapi.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, false, isf, mmapi.TM_LT)
			}
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_LEI:
			im := int64(opcodeapi.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() <= im
			} else if v.IsFloat() {
				cond = v.Float() <= float64(im)
			} else {
				isf := opcodeapi.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, false, isf, mmapi.TM_LE)
			}
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_GTI:
			im := int64(opcodeapi.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() > im
			} else if v.IsFloat() {
				cond = v.Float() > float64(im)
			} else {
				// GTI: a > im ⟺ im < a, so flip=true, TM_LT
				isf := opcodeapi.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, true, isf, mmapi.TM_LT)
			}
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_GEI:
			im := int64(opcodeapi.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() >= im
			} else if v.IsFloat() {
				cond = v.Float() >= float64(im)
			} else {
				// GEI: a >= im ⟺ im <= a, so flip=true, TM_LE
				isf := opcodeapi.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, true, isf, mmapi.TM_LE)
			}
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_TEST:
			cond := !L.Stack[ra].Val.IsFalsy()
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_TESTSET:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			if rb.IsFalsy() == (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++ // condition failed, skip jump
			} else {
				L.Stack[ra].Val = rb
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		// ===== Jump =====

		case opcodeapi.OP_JMP:
			ci.SavedPC += opcodeapi.GetArgSJ(inst)

		// ===== For loops =====

		case opcodeapi.OP_FORPREP:
			if ForPrep(L, ra) {
				ci.SavedPC += opcodeapi.GetArgBx(inst) + 1 // skip loop
			}

		case opcodeapi.OP_FORLOOP:
			if ForLoop(L, ra) {
				ci.SavedPC -= opcodeapi.GetArgBx(inst) // jump back
			}

		case opcodeapi.OP_TFORPREP:
			// Swap control and closing variables
			// Before: ra=iter, ra+1=state, ra+2=control, ra+3=closing
			// After:  ra=iter, ra+1=state, ra+2=closing(tbc), ra+3=control
			temp := L.Stack[ra+3].Val
			L.Stack[ra+3].Val = L.Stack[ra+2].Val
			L.Stack[ra+2].Val = temp
			// Mark the closing variable (now at ra+2) as to-be-closed
			// C Lua: halfProtect(luaF_newtbcupval(L, ra + 2))
			MarkTBC(L, ra+2)
			ci.SavedPC += opcodeapi.GetArgBx(inst)
			// Fall through to TFORCALL
			inst = code[ci.SavedPC]
			ci.SavedPC++
			ra = base + opcodeapi.GetArgA(inst)
			goto tforcall

		case opcodeapi.OP_TFORCALL:
			goto tforcall

		case opcodeapi.OP_TFORLOOP:
			if !L.Stack[ra+3].Val.IsNil() {
				ci.SavedPC -= opcodeapi.GetArgBx(inst) // jump back
			}

		// ===== Call/Return =====

		case opcodeapi.OP_CALL:
			b := opcodeapi.GetArgB(inst)
			nresults := opcodeapi.GetArgC(inst) - 1
			if b != 0 {
				L.Top = ra + b
			}
			// SavedPC already set by dispatch loop
			newci := PreCall(L, ra, nresults)
			if newci != nil {
				ci = newci
				goto startfunc
			}
			// C function already executed

		case opcodeapi.OP_TAILCALL:
			b := opcodeapi.GetArgB(inst)
			nparams1 := opcodeapi.GetArgC(inst)
			delta := 0
			if nparams1 != 0 {
				delta = ci.NExtraArgs + nparams1
			}
			if b != 0 {
				L.Top = ra + b
			} else {
				b = L.Top - ra
			}
			if opcodeapi.GetArgK(inst) != 0 {
				closureapi.CloseUpvals(L, base)
			}
			n := PreTailCall(L, ci, ra, b, delta)
			if n < 0 {
				// Lua function — ci.Func already adjusted by delta
				goto startfunc
			}
			// C function executed — restore func and finish
			ci.Func -= delta
			PosCall(L, ci, n)
			goto ret

		case opcodeapi.OP_RETURN:
			b := opcodeapi.GetArgB(inst)
			n := b - 1
			nparams1 := opcodeapi.GetArgC(inst)
			if n < 0 {
				n = L.Top - ra
			}
			if opcodeapi.GetArgK(inst) != 0 {
				// C Lua: save nres, ensure stack space, close upvals+TBC, refresh base/ra
				// Use StatusCloseKTop so callCloseMethod does NOT reset L.Top —
				// return values sit on the stack above the TBC variables.
				ci.NRes = n
				if L.Top < ci.Top {
					L.Top = ci.Top
				}
				closureapi.CloseUpvals(L, base)
				CloseTBCWithError(L, base, stateapi.StatusCloseKTop, objectapi.Nil, true)
				// After close, stack may have been reallocated by __close calls.
				// Refresh base and ra from ci (which uses offsets, not pointers).
				base = ci.Func + 1
				ra = base + opcodeapi.GetArgA(inst)
			}
			if nparams1 != 0 {
				ci.Func -= ci.NExtraArgs + nparams1
			}
			L.Top = ra + n
			PosCall(L, ci, n)
			goto ret

		case opcodeapi.OP_RETURN0:
			if L.HookMask != 0 {
				// Hooks active — fall back to full PosCall (fires return hook)
				L.Top = ra
				PosCall(L, ci, 0)
			} else {
				// Fast path — no hooks
				nres := ci.NResults()
				L.CI = ci.Prev
				L.Top = base - 1
				for i := 0; i < nres; i++ {
					L.Stack[L.Top].Val = objectapi.Nil
					L.Top++
				}
				if nres < 0 {
					L.Top = base - 1
				}
			}
			goto ret

		case opcodeapi.OP_RETURN1:
			if L.HookMask != 0 {
				// Hooks active — fall back to full PosCall (fires return hook)
				L.Top = ra + 1
				PosCall(L, ci, 1)
			} else {
				// Fast path — no hooks
				nres := ci.NResults()
				L.CI = ci.Prev
				if nres == 0 {
					L.Top = base - 1
				} else {
					L.Stack[base-1].Val = L.Stack[ra].Val
					L.Top = base
					for i := 1; i < nres; i++ {
						L.Stack[L.Top].Val = objectapi.Nil
						L.Top++
					}
				}
			}
			goto ret

		// ===== Closure/Vararg =====

		case opcodeapi.OP_CLOSURE:
			bx := opcodeapi.GetArgBx(inst)
			p := cl.Proto.Protos[bx]
			PushClosure(L, p, cl.UpVals, base, ra)

			// Periodic GC: closures are heap-allocated objects.
			if g := L.Global; !g.GCStopped {
				checkPeriodicGC(g, L)
			}

		case opcodeapi.OP_VARARG:
			n := opcodeapi.GetArgC(inst) - 1
			vatab := -1
			if opcodeapi.GetArgK(inst) != 0 {
				vatab = opcodeapi.GetArgB(inst)
			}
			GetVarargs(L, ci, ra, n, vatab)

		case opcodeapi.OP_VARARGPREP:
			AdjustVarargs(L, ci, cl.Proto)
			// Update base after adjustment
			base = ci.Func + 1
			// Fire call hook AFTER adjustment (deferred from PreCall).
			// Mirrors: OP_VARARGPREP in lvm.c calls luaD_hookcall after
			// luaT_adjustvarargs, so debug.getlocal sees correct params.
			if L.HookMask != 0 {
				CallHook(L, ci)
				L.OldPC = 1 // next opcode seen as "new" line
			}

		// ===== Table construction =====

		case opcodeapi.OP_SETLIST:
			n := opcodeapi.GetArgVB(inst)
			last := opcodeapi.GetArgVC(inst)
			h := L.Stack[ra].Val.Val.(*tableapi.Table)
			if n == 0 {
				n = L.Top - ra - 1
			}
			last += n
			if opcodeapi.GetArgK(inst) != 0 {
				last += opcodeapi.GetArgAx(code[ci.SavedPC]) * (opcodeapi.MaxArgVC + 1)
				ci.SavedPC++
			}
			for i := n; i > 0; i-- {
				h.SetInt(int64(last), L.Stack[ra+i].Val)
				last--
			}
			L.Top = ci.Top // restore top

		// ===== Lua 5.5 new opcodes =====

		case opcodeapi.OP_GETVARG:
			// OP_GETVARG: ra = vararg[rc]
			// Mirrors C Lua's luaT_getvararg (ltm.c):
			//   integer key → read from hidden vararg stack slots
			//   string "n"  → return number of extra args
			//   anything else → nil
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			switch rc.Tt {
			case objectapi.TagInteger:
				idx := rc.Val.(int64)
				nExtra := ci.NExtraArgs
				if uint64(idx-1) < uint64(nExtra) {
					varBase := ci.Func - nExtra
					L.Stack[ra].Val = L.Stack[varBase+int(idx)-1].Val
				} else {
					L.Stack[ra].Val = objectapi.Nil
				}
			case objectapi.TagFloat:
				f := rc.Val.(float64)
				if idx, ok := FloatToInteger(f); ok {
					nExtra := ci.NExtraArgs
					if uint64(idx-1) < uint64(nExtra) {
						varBase := ci.Func - nExtra
						L.Stack[ra].Val = L.Stack[varBase+int(idx)-1].Val
					} else {
						L.Stack[ra].Val = objectapi.Nil
					}
				} else {
					L.Stack[ra].Val = objectapi.Nil
				}
			case objectapi.TagShortStr, objectapi.TagLongStr:
				s := rc.Val.(*objectapi.LuaString)
				if s.Data == "n" {
					L.Stack[ra].Val = objectapi.MakeInteger(int64(ci.NExtraArgs))
				} else {
					L.Stack[ra].Val = objectapi.Nil
				}
			default:
				L.Stack[ra].Val = objectapi.Nil
			}

		case opcodeapi.OP_ERRNNIL:
			// Error if value is not nil — used for global redefinition check
			if !L.Stack[ra].Val.IsNil() {
				bx := opcodeapi.GetArgBx(inst)
				if bx > 0 {
					// bx-1 is the constant index for the variable name
					name := k[bx-1]
					if name.IsString() {
						RunError(L, "global '"+name.StringVal().String()+"' already defined")
					} else {
						RunError(L, "global already defined")
					}
				} else {
					RunError(L, "global already defined")
				}
			}

		case opcodeapi.OP_EXTRAARG:
			// Should never be executed directly
			panic("OP_EXTRAARG should not be executed")

		default:
			RunError(L, "unknown opcode")
		}
		continue

	tforcall:
		// Generic for: call iterator
		L.Stack[ra+5].Val = L.Stack[ra+3].Val // copy control
		L.Stack[ra+4].Val = L.Stack[ra+1].Val // copy state
		L.Stack[ra+3].Val = L.Stack[ra].Val   // copy function
		L.Top = ra + 3 + 3
		{
			nr := opcodeapi.GetArgC(inst)
			Call(L, ra+3, nr)
		}
		L.Top = ci.Top // restore top
		// Next instruction should be TFORLOOP
		inst = code[ci.SavedPC]
		ci.SavedPC++
		ra = base + opcodeapi.GetArgA(inst)
		if !L.Stack[ra+3].Val.IsNil() {
			ci.SavedPC -= opcodeapi.GetArgBx(inst)
		}
		continue

	ret:
		if ci.CallStatus&stateapi.CISTFresh != 0 {
			return // end this frame
		}
		ci = L.CI
		// Correct 'oldpc' for the caller's frame after a return.
		// The callee overwrites L.OldPC with its own PCs during execution.
		// Set it to the caller's current SavedPC so the line hook won't
		// fire a spurious event for the same line as the CALL instruction.
		// Only done here (not in PosCall) because coroutine yield/resume
		// needs OldPC left alone to fire the correct line event on resume.
		// Mirrors: rethook in ldo.c (L->oldpc = pcRel(ci->u.l.savedpc, ...))
		if ci.IsLua() {
			L.OldPC = ci.SavedPC - 1
		}
		goto startfunc
	}
}
