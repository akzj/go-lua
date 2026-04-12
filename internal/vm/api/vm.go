// VM execution loop — the heart of the Lua interpreter.
//
// This is the Go equivalent of C's lvm.c. The core is Execute(),
// a giant switch on opcodes that runs Lua bytecode.
//
// Reference: lua-master/lvm.c, .analysis/05-vm-execution-loop.md
package api

import (
	"math"
	"strings"

	closureapi "github.com/akzj/go-lua/internal/closure/api"
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

// FloatToInteger converts a float64 to int64 if it has an exact integer value.
func FloatToInteger(f float64) (int64, bool) {
	i := int64(f)
	if float64(i) == f {
		return i, true
	}
	return 0, false
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
func ltIntFloat(i int64, f float64) bool {
	return float64(i) < f
}

// leIntFloat checks i <= f.
func leIntFloat(i int64, f float64) bool {
	return float64(i) <= f
}

// ltFloatInt checks f < i.
func ltFloatInt(f float64, i int64) bool {
	return f < float64(i)
}

// leFloatInt checks f <= i.
func leFloatInt(f float64, i int64) bool {
	return f <= float64(i)
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
		RunError(L, "attempt to compare two "+objectapi.TypeNames[l.Type()]+" values")
	}
	result := callTMRes(L, tm, l, r)
	return !result.IsFalsy()
}

// EqualObj performs t1 == t2 with metamethods.
func EqualObj(L *stateapi.LuaState, t1, t2 objectapi.TValue) bool {
	if t1.Type() != t2.Type() {
		// Different base types — check int/float cross-comparison
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
		return t1.Val.(int64) == t2.Val.(int64)
	case objectapi.TagFloat:
		return t1.Val.(float64) == t2.Val.(float64)
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
		tm := getTableTM(h1, mmapi.TM_EQ)
		if tm.IsNil() {
			tm = getTableTM(h2, mmapi.TM_EQ)
		}
		if tm.IsNil() {
			return false
		}
		result := callTMRes(L, tm, t1, t2)
		return !result.IsFalsy()
	case objectapi.TagLuaClosure, objectapi.TagCClosure, objectapi.TagLightCFunc:
		return t1.Val == t2.Val
	default:
		return t1.Val == t2.Val
	}
}

// RawEqualObj performs raw equality (no metamethods).
func RawEqualObj(t1, t2 objectapi.TValue) bool {
	if t1.Type() != t2.Type() {
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
		return t1.Val.(int64) == t2.Val.(int64)
	case objectapi.TagFloat:
		return t1.Val.(float64) == t2.Val.(float64)
	case objectapi.TagShortStr, objectapi.TagLongStr:
		return t1.Val.(*objectapi.LuaString).Data == t2.Val.(*objectapi.LuaString).Data
	default:
		return t1.Val == t2.Val
	}
}

// getTableTM gets a metamethod from a table's metatable.
// getTableTM needs GlobalState to look up TM name strings.
// We pass it through a package-level variable set by Execute.
var gState *stateapi.GlobalState

func getTableTM(t *tableapi.Table, event mmapi.TMS) objectapi.TValue {
	mt := t.GetMetatable()
	if mt == nil {
		return objectapi.Nil
	}
	if gState == nil {
		return objectapi.Nil
	}
	tmName := gState.TMNames[event]
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
func callTMRes(L *stateapi.LuaState, tm, p1, p2 objectapi.TValue) objectapi.TValue {
	// Push: tm, p1, p2
	top := L.Top
	L.Stack[top].Val = tm
	L.Stack[top+1].Val = p1
	L.Stack[top+2].Val = p2
	L.Top = top + 3
	Call(L, top, 1)
	result := L.Stack[L.Top-1].Val
	L.Top--
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

// tryBinTM tries a binary metamethod.
func tryBinTM(L *stateapi.LuaState, p1, p2 objectapi.TValue, res int, event mmapi.TMS) {
	tm := mmapi.GetTMByObj(L.Global, p1, event)
	if tm.IsNil() {
		tm = mmapi.GetTMByObj(L.Global, p2, event)
	}
	if tm.IsNil() {
		if event == mmapi.TM_CONCAT {
			RunError(L, "attempt to concatenate a "+objectapi.TypeNames[p1.Type()]+" value")
		}
		RunError(L, "attempt to perform arithmetic on a "+objectapi.TypeNames[p1.Type()]+" value")
	}
	result := callTMRes(L, tm, p1, p2)
	L.Stack[res].Val = result
}

// tryBiniTM tries a binary metamethod with an integer immediate operand.
func tryBiniTM(L *stateapi.LuaState, p1 objectapi.TValue, imm int, flip bool, res int, event mmapi.TMS) {
	p2 := objectapi.MakeInteger(int64(imm))
	if flip {
		tryBinTM(L, p2, p1, res, event)
	} else {
		tryBinTM(L, p1, p2, res, event)
	}
}

// tryBinKTM tries a binary metamethod with a constant operand (possibly flipped).
func tryBinKTM(L *stateapi.LuaState, p1, p2 objectapi.TValue, flip bool, res int, event mmapi.TMS) {
	if flip {
		tryBinTM(L, p2, p1, res, event)
	} else {
		tryBinTM(L, p1, p2, res, event)
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
		RunError(L, "attempt to concatenate a "+objectapi.TypeNames[p1.Type()]+" value")
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
			tryConcatTM(L)
		} else if len(s2) == 0 {
			// Result is first operand (already converted to string)
			if !p1.IsString() {
				ls := &objectapi.LuaString{Data: s1, IsShort: len(s1) <= 40}
				L.Stack[top-2].Val = objectapi.MakeString(ls)
			}
		} else if len(s1) == 0 {
			L.Stack[top-2].Val = L.Stack[top-1].Val
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
			ls := &objectapi.LuaString{Data: result, IsShort: len(result) <= 40}
			L.Stack[top-n].Val = objectapi.MakeString(ls)
		}
		total -= n - 1
		L.Top -= n - 1
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
			tm := getTableTM(h, mmapi.TM_LEN)
			if !tm.IsNil() {
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
			RunError(L, "attempt to get length of a "+objectapi.TypeNames[rb.Type()]+" value")
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
			tm = getTableTM(h, mmapi.TM_INDEX)
			if tm.IsNil() {
				L.Stack[ra].Val = objectapi.Nil
				return
			}
		} else {
			tm = mmapi.GetTMByObj(L.Global, t, mmapi.TM_INDEX)
			if tm.IsNil() {
				RunError(L, "attempt to index a "+objectapi.TypeNames[t.Type()]+" value")
			}
		}
		if tm.IsFunction() {
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

// FinishSet completes a table set with __newindex metamethod chain.
func FinishSet(L *stateapi.LuaState, t, key, val objectapi.TValue) {
	for loop := 0; loop < maxTagLoop; loop++ {
		var tm objectapi.TValue
		if t.IsTable() {
			h := t.Val.(*tableapi.Table)
			tm = getTableTM(h, mmapi.TM_NEWINDEX)
			if tm.IsNil() {
				h.Set(key, val)
				return
			}
		} else {
			tm = mmapi.GetTMByObj(L.Global, t, mmapi.TM_NEWINDEX)
			if tm.IsNil() {
				RunError(L, "attempt to index a "+objectapi.TypeNames[t.Type()]+" value")
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
		// Convert limit
		var limit int64
		if li, ok := ToInteger(plimit); ok {
			limit = li
		} else {
			fl, ok := ToNumber(plimit)
			if !ok {
				RunError(L, "'for' limit must be a number")
			}
			if fl > 0 {
				if step < 0 {
					return true
				}
				limit = math.MaxInt64
			} else {
				if step > 0 {
					return true
				}
				limit = math.MinInt64
			}
		}
		// Check if loop should run
		if step > 0 && init > limit {
			return true
		}
		if step < 0 && init < limit {
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
			RunError(L, "'for' initial value must be a number")
		}
		if !ok2 {
			RunError(L, "'for' limit must be a number")
		}
		if !ok3 {
			RunError(L, "'for' step must be a number")
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
	ci.NExtraArgs = nextra
	// Ensure enough stack space before copying
	CheckStack(L, int(p.MaxStackSize)+1)
	// Copy function to top of stack (matches C buildhiddenargs)
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
}

// GetVarargs copies vararg values to the stack starting at ra.
// After AdjustVarargs, the extra (vararg) arguments are stored just below
// ci.Func: positions ci.Func-NExtraArgs .. ci.Func-1
func GetVarargs(L *stateapi.LuaState, ci *stateapi.CallInfo, ra int, n int) {
	nExtra := ci.NExtraArgs
	varBase := ci.Func - nExtra
	if n < 0 {
		n = nExtra
		CheckStack(L, n)
		L.Top = ra + n
	}
	for i := 0; i < n; i++ {
		if i < nExtra {
			L.Stack[ra+i].Val = L.Stack[varBase+i].Val
		} else {
			L.Stack[ra+i].Val = objectapi.Nil
		}
	}
}

// ---------------------------------------------------------------------------
// Execute — the main VM execution loop
// ---------------------------------------------------------------------------

// Execute runs the VM main loop for the given CallInfo.
// This is the Go equivalent of luaV_execute in lvm.c.
func Execute(L *stateapi.LuaState, ci *stateapi.CallInfo) {
	// Set global state for metamethod lookups
	gState = L.Global

startfunc:
	cl := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
	k := cl.Proto.Constants
	code := cl.Proto.Code
	base := ci.Func + 1

	for {
		inst := code[ci.SavedPC]
		ci.SavedPC++
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

		case opcodeapi.OP_TBC:
			// To-be-closed: mark the variable
			// Simplified: just note it (full TBC support deferred)

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
				h := upval.Val.(*tableapi.Table)
				h.Set(rb, rc)
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
				h := tval.Val.(*tableapi.Table)
				h.Set(rb, rc)
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
				h := tval.Val.(*tableapi.Table)
				h.SetInt(b, rc)
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
				h := tval.Val.(*tableapi.Table)
				h.Set(rb, rc)
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
			L.Stack[ra].Val = objectapi.TValue{Tt: objectapi.TagTable, Val: t}

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
			// else: fall through to MMBIN on next instruction

		// ===== Arithmetic with constant =====

		case opcodeapi.OP_ADDK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() + kc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := ToNumber(rb)
				nc, ok2 := ToNumber(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = objectapi.MakeFloat(nb + nc)
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
				nb, ok1 := ToNumber(rb)
				nc, ok2 := ToNumber(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = objectapi.MakeFloat(nb - nc)
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
				nb, ok1 := ToNumber(rb)
				nc, ok2 := ToNumber(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = objectapi.MakeFloat(nb * nc)
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
				nb, ok1 := ToNumber(rb)
				nc, ok2 := ToNumber(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = objectapi.MakeFloat(FMod(nb, nc))
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_POWK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			nb, ok1 := ToNumber(rb)
			nc, ok2 := ToNumber(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeFloat(math.Pow(nb, nc))
				ci.SavedPC++
			}

		case opcodeapi.OP_DIVK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			nb, ok1 := ToNumber(rb)
			nc, ok2 := ToNumber(kc)
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
				nb, ok1 := ToNumber(rb)
				nc, ok2 := ToNumber(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = objectapi.MakeFloat(math.Floor(nb / nc))
					ci.SavedPC++
				}
			}

		// ===== Bitwise with constant =====

		case opcodeapi.OP_BANDK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			ib, ok1 := ToInteger(rb)
			ic, ok2 := ToInteger(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib & ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BORK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			ib, ok1 := ToInteger(rb)
			ic, ok2 := ToInteger(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib | ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BXORK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			ib, ok1 := ToInteger(rb)
			ic, ok2 := ToInteger(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib ^ ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_SHLI:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ic := int64(opcodeapi.GetArgSC(inst))
			ib, ok := ToInteger(rb)
			if ok {
				L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ic, ib))
				ci.SavedPC++
			}

		case opcodeapi.OP_SHRI:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ic := int64(opcodeapi.GetArgSC(inst))
			ib, ok := ToInteger(rb)
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
				nb, ok1 := ToNumber(rb)
				nc, ok2 := ToNumber(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = objectapi.MakeFloat(nb + nc)
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
				nb, ok1 := ToNumber(rb)
				nc, ok2 := ToNumber(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = objectapi.MakeFloat(nb - nc)
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
				nb, ok1 := ToNumber(rb)
				nc, ok2 := ToNumber(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = objectapi.MakeFloat(nb * nc)
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
				nb, ok1 := ToNumber(rb)
				nc, ok2 := ToNumber(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = objectapi.MakeFloat(FMod(nb, nc))
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_POW:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			nb, ok1 := ToNumber(rb)
			nc, ok2 := ToNumber(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeFloat(math.Pow(nb, nc))
				ci.SavedPC++
			}

		case opcodeapi.OP_DIV:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			nb, ok1 := ToNumber(rb)
			nc, ok2 := ToNumber(rc)
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
				nb, ok1 := ToNumber(rb)
				nc, ok2 := ToNumber(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = objectapi.MakeFloat(math.Floor(nb / nc))
					ci.SavedPC++
				}
			}

		// ===== Bitwise register-register =====

		case opcodeapi.OP_BAND:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := ToInteger(rb)
			ic, ok2 := ToInteger(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib & ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BOR:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := ToInteger(rb)
			ic, ok2 := ToInteger(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib | ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BXOR:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := ToInteger(rb)
			ic, ok2 := ToInteger(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib ^ ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_SHL:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := ToInteger(rb)
			ic, ok2 := ToInteger(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ib, ic))
				ci.SavedPC++
			}

		case opcodeapi.OP_SHR:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := ToInteger(rb)
			ic, ok2 := ToInteger(rc)
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
			tryBinTM(L, L.Stack[ra].Val, rb, result, tm)

		case opcodeapi.OP_MMBINI:
			imm := opcodeapi.GetArgSB(inst)
			tm := mmapi.TMS(opcodeapi.GetArgC(inst))
			flip := opcodeapi.GetArgK(inst) != 0
			prevInst := code[ci.SavedPC-2]
			result := base + opcodeapi.GetArgA(prevInst)
			tryBiniTM(L, L.Stack[ra].Val, imm, flip, result, tm)

		case opcodeapi.OP_MMBINK:
			imm := k[opcodeapi.GetArgB(inst)]
			tm := mmapi.TMS(opcodeapi.GetArgC(inst))
			flip := opcodeapi.GetArgK(inst) != 0
			prevInst := code[ci.SavedPC-2]
			result := base + opcodeapi.GetArgA(prevInst)
			tryBinKTM(L, L.Stack[ra].Val, imm, flip, result, tm)

		// ===== Unary =====

		case opcodeapi.OP_UNM:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			if rb.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(-rb.Integer())
			} else if rb.IsFloat() {
				L.Stack[ra].Val = objectapi.MakeFloat(-rb.Float())
			} else {
				tryBinTM(L, rb, rb, ra, mmapi.TM_UNM)
			}

		case opcodeapi.OP_BNOT:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ib, ok := ToInteger(rb)
			if ok {
				L.Stack[ra].Val = objectapi.MakeInteger(^ib)
			} else {
				tryBinTM(L, rb, rb, ra, mmapi.TM_BNOT)
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

		// ===== Comparison =====

		case opcodeapi.OP_EQ:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			cond := EqualObj(L, L.Stack[ra].Val, rb)
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++ // skip jump
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_LT:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			cond := LessThan(L, L.Stack[ra].Val, rb)
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_LE:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
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
			temp := L.Stack[ra+3].Val
			L.Stack[ra+3].Val = L.Stack[ra+2].Val
			L.Stack[ra+2].Val = temp
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
			ci.SavedPC = ci.SavedPC // save PC
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
				closureapi.CloseUpvals(L, base)
			}
			if nparams1 != 0 {
				ci.Func -= ci.NExtraArgs + nparams1
			}
			L.Top = ra + n
			PosCall(L, ci, n)
			goto ret

		case opcodeapi.OP_RETURN0:
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
			goto ret

		case opcodeapi.OP_RETURN1:
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
			goto ret

		// ===== Closure/Vararg =====

		case opcodeapi.OP_CLOSURE:
			bx := opcodeapi.GetArgBx(inst)
			p := cl.Proto.Protos[bx]
			PushClosure(L, p, cl.UpVals, base, ra)

		case opcodeapi.OP_VARARG:
			n := opcodeapi.GetArgC(inst) - 1
			GetVarargs(L, ci, ra, n)

		case opcodeapi.OP_VARARGPREP:
			AdjustVarargs(L, ci, cl.Proto)
			// Update base after adjustment
			base = ci.Func + 1

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
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			idx, ok := ToInteger(rc)
			if !ok || idx < 1 {
				L.Stack[ra].Val = objectapi.Nil
			} else {
				nExtra := ci.NExtraArgs
				varBase := ci.Func + 1 - nExtra
				i := int(idx) - 1
				if i < nExtra {
					L.Stack[ra].Val = L.Stack[varBase+i].Val
				} else {
					L.Stack[ra].Val = objectapi.Nil
				}
			}

		case opcodeapi.OP_ERRNNIL:
			// Error if value is not nil
			if !L.Stack[ra].Val.IsNil() {
				RunError(L, "value is not nil")
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
		goto startfunc
	}
}
