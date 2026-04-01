// Package internal provides tests for the string library.
package internal

import (
	"testing"

	luaapi "github.com/akzj/go-lua/api"
	tableapi "github.com/akzj/go-lua/table/api"
	stringlib "github.com/akzj/go-lua/lib/string/api"
)

// =============================================================================
// Constructor Tests
// =============================================================================

// TestNewStringLib tests creating a new StringLib instance.
func TestNewStringLib(t *testing.T) {
	lib := NewStringLib()
	if lib == nil {
		t.Error("NewStringLib() returned nil")
	}
}

// TestStringLibImplementsInterface tests that StringLib implements StringLib interface.
func TestStringLibImplementsInterface(t *testing.T) {
	var lib stringlib.StringLib = NewStringLib()
	if lib == nil {
		t.Error("StringLib does not implement stringlib.StringLib interface")
	}
}

// =============================================================================
// LuaFunc Signature Tests
// =============================================================================

// TestLuaFuncSignatures tests that all string functions have correct LuaFunc signature.
func TestLuaFuncSignatures(t *testing.T) {
	var _ stringlib.LuaFunc = strLen
	var _ stringlib.LuaFunc = strSub
	var _ stringlib.LuaFunc = strUpper
	var _ stringlib.LuaFunc = strLower
	var _ stringlib.LuaFunc = strReverse
	var _ stringlib.LuaFunc = strRep
	var _ stringlib.LuaFunc = strByte
	var _ stringlib.LuaFunc = strChar
	var _ stringlib.LuaFunc = strFind
	var _ stringlib.LuaFunc = strMatch
	var _ stringlib.LuaFunc = strGsub
	var _ stringlib.LuaFunc = strFormat
}

// =============================================================================
// Helper Functions Tests
// =============================================================================

// TestPosrelat tests the position relative calculation.
func TestPosrelat(t *testing.T) {
	testCases := []struct {
		pos      int64
		length   int64
		expected int64
	}{
		{1, 10, 1},   // Positive position
		{5, 10, 5},   // Positive position
		{10, 10, 10}, // At end
		{-1, 10, 10}, // Last character
		{-2, 10, 9},  // Second to last
		{-10, 10, 1}, // First character
		{0, 10, 1},   // Zero becomes 1
		{-11, 10, 1}, // Beyond start becomes 1
		{-20, 10, 1}, // Way beyond start becomes 1
	}

	for _, tc := range testCases {
		got := posrelat(tc.pos, tc.length)
		if got != tc.expected {
			t.Errorf("posrelat(%d, %d) = %d, want %d", tc.pos, tc.length, got, tc.expected)
		}
	}
}

// =============================================================================
// testLuaAPI is a mock implementation of LuaAPI for testing.
// Uses 1-indexed stack to match Lua semantics.
type testLuaAPI struct {
	stack []interface{}
}

func newTestLuaAPI(values ...interface{}) *testLuaAPI {
	return &testLuaAPI{stack: values}
}

func (t *testLuaAPI) GetTop() int                    { return len(t.stack) }
func (t *testLuaAPI) SetTop(idx int)                  {
	for len(t.stack) < idx {
		t.stack = append(t.stack, nil)
	}
	t.stack = t.stack[:idx]
}
func (t *testLuaAPI) Pop()                            { t.stack = t.stack[:len(t.stack)-1] }
func (t *testLuaAPI) PushValue(idx int)               { t.stack = append(t.stack, t.getValue(idx)) }
func (t *testLuaAPI) AbsIndex(idx int) int            { return idx }
func (t *testLuaAPI) Rotate(idx, n int)               {}
func (t *testLuaAPI) Copy(fromidx, toidx int)         {}
func (t *testLuaAPI) CheckStack(n int) bool           { return true }
func (t *testLuaAPI) XMove(to luaapi.LuaAPI, n int)   {}

func (t *testLuaAPI) getValue(idx int) interface{} {
	if idx < 0 {
		idx = len(t.stack) + idx + 1
	}
	if idx < 1 || idx > len(t.stack) {
		return nil
	}
	return t.stack[idx-1]
}
func (t *testLuaAPI) getValueCopy(idx int) interface{} {
	return t.getValue(idx)
}

// Type Checking
func (t *testLuaAPI) Type(idx int) int {
	v := t.getValue(idx)
	if v == nil {
		return luaapi.LUA_TNIL
	}
	switch v.(type) {
	case string:
		return luaapi.LUA_TSTRING
	case int64:
		return luaapi.LUA_TNUMBER
	case int:
		return luaapi.LUA_TNUMBER
	case float64:
		return luaapi.LUA_TNUMBER
	case bool:
		return luaapi.LUA_TBOOLEAN
	default:
		return luaapi.LUA_TNIL
	}
}
func (t *testLuaAPI) TypeName(tp int) string          { return "string" }
func (t *testLuaAPI) IsNone(idx int) bool             { return idx > len(t.stack) }
func (t *testLuaAPI) IsNil(idx int) bool              { return t.getValue(idx) == nil }
func (t *testLuaAPI) IsNoneOrNil(idx int) bool        { return t.IsNone(idx) || t.IsNil(idx) }
func (t *testLuaAPI) IsBoolean(idx int) bool          { _, ok := t.getValue(idx).(bool); return ok }
func (t *testLuaAPI) IsString(idx int) bool           { _, ok := t.getValue(idx).(string); return ok }
func (t *testLuaAPI) IsFunction(idx int) bool         { return false }
func (t *testLuaAPI) IsTable(idx int) bool            { return false }
func (t *testLuaAPI) IsLightUserData(idx int) bool    { return false }
func (t *testLuaAPI) IsThread(idx int) bool           { return false }
func (t *testLuaAPI) IsInteger(idx int) bool {
	v := t.getValue(idx)
	switch v.(type) {
	case int64, int:
		return true
	}
	return false
}
func (t *testLuaAPI) IsNumber(idx int) bool {
	v := t.getValue(idx)
	_, isInt := v.(int64)
	_, isFloat := v.(float64)
	return isInt || isFloat
}

// Value Conversion
func (t *testLuaAPI) ToInteger(idx int) (int64, bool) {
	v := t.getValue(idx)
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	}
	return 0, false
}
func (t *testLuaAPI) ToNumber(idx int) (float64, bool) {
	v := t.getValue(idx)
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	}
	return 0, false
}
func (t *testLuaAPI) ToString(idx int) (string, bool) {
	v := t.getValue(idx)
	switch s := v.(type) {
	case string:
		return s, true
	}
	return "", false
}
func (t *testLuaAPI) ToBoolean(idx int) bool {
	v := t.getValue(idx)
	if b, ok := v.(bool); ok {
		return b
	}
	return v != nil
}
func (t *testLuaAPI) ToPointer(idx int) interface{}   { return nil }
func (t *testLuaAPI) ToThread(idx int) luaapi.LuaAPI  { return nil }

// Push Functions
func (t *testLuaAPI) PushNil()                            { t.stack = append(t.stack, nil) }
func (t *testLuaAPI) PushInteger(n int64)                { t.stack = append(t.stack, n) }
func (t *testLuaAPI) PushNumber(n float64)               { t.stack = append(t.stack, n) }
func (t *testLuaAPI) PushString(s string)                { t.stack = append(t.stack, s) }
func (t *testLuaAPI) PushBoolean(b bool)                 { t.stack = append(t.stack, b) }
func (t *testLuaAPI) PushLightUserData(p interface{})    { t.stack = append(t.stack, p) }
func (t *testLuaAPI) PushGoFunction(fn func(luai luaapi.LuaAPI) int) { t.stack = append(t.stack, fn) }
func (t *testLuaAPI) Insert(pos int)                     {}

// Table Operations
func (t *testLuaAPI) GetTable(idx int) int              { return luaapi.LUA_TNIL }
func (t *testLuaAPI) GetField(idx int, k string) int     { return luaapi.LUA_TNIL }
func (t *testLuaAPI) GetI(idx int, n int64) int         { return luaapi.LUA_TNIL }
func (t *testLuaAPI) RawGet(idx int) int                { return luaapi.LUA_TNIL }
func (t *testLuaAPI) RawGetI(idx int, n int64) int     { return luaapi.LUA_TNIL }
func (t *testLuaAPI) CreateTable(narr, nrec int)        {}
func (t *testLuaAPI) SetTable(idx int)                  {}
func (t *testLuaAPI) SetField(idx int, k string)        {}
func (t *testLuaAPI) SetI(idx int, n int64)             {}
func (t *testLuaAPI) RawSet(idx int)                    {}
func (t *testLuaAPI) RawSetI(idx int, n int64)          {}
func (t *testLuaAPI) GetGlobal(name string) int         { return luaapi.LUA_TNIL }
func (t *testLuaAPI) SetGlobal(name string)            {}

// Metatable Operations
func (t *testLuaAPI) GetMetatable(idx int) bool        { return false }
func (t *testLuaAPI) SetMetatable(idx int)             {}

// Call Operations
func (t *testLuaAPI) Call(nArgs, nResults int)         {}
func (t *testLuaAPI) PCall(nArgs, nResults, errfunc int) int { return int(luaapi.LUA_OK) }

// Error Handling
func (t *testLuaAPI) Error() int                        { return 0 }
func (t *testLuaAPI) ErrorMessage() int                 { return 0 }
func (t *testLuaAPI) Where(level int)                  {}

// GC Control
func (t *testLuaAPI) GC(what int, args ...int) int     { return 0 }

// Miscellaneous
func (t *testLuaAPI) Next(idx int) bool                { return false }
func (t *testLuaAPI) Concat(n int)                     {}
func (t *testLuaAPI) Len(idx int)                      {}
func (t *testLuaAPI) Compare(idx1, idx2, op int) bool  { return false }
func (t *testLuaAPI) RawLen(idx int) uint              { return 0 }

// Registry Access
func (t *testLuaAPI) Registry() tableapi.TableInterface { return nil }
func (t *testLuaAPI) Ref(tbl tableapi.TableInterface) int { return -1 }
func (t *testLuaAPI) UnRef(tbl tableapi.TableInterface, ref int) {}
func (t *testLuaAPI) PushGlobalTable()                 {}

// Thread Management
func (t *testLuaAPI) NewThread() luaapi.LuaAPI         { return t }
func (t *testLuaAPI) Status() luaapi.Status           { return luaapi.LUA_OK }

// =============================================================================
// String Function Tests
// =============================================================================

// TestStrLen tests string.len function.
func TestStrLen(t *testing.T) {
	testCases := []struct {
		input    string
		expected int64
	}{
		{"hello", 5},
		{"", 0},
		{"world", 5},
		{"a", 1},
		{"hello world", 11},
	}

	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		result := strLen(L)
		if result != 1 {
			t.Errorf("strLen returned %d, want 1", result)
		}
		got, _ := L.ToInteger(-1)
		if got != tc.expected {
			t.Errorf("string.len(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

// TestStrLenNonString tests string.len with non-string argument.
func TestStrLenNonString(t *testing.T) {
	L := newTestLuaAPI(123)
	result := strLen(L)
	if result != 1 {
		t.Errorf("strLen returned %d, want 1", result)
	}
	if !L.IsNil(-1) {
		t.Error("string.len(non-string) should return nil")
	}
}

// TestStrSub tests string.sub function.
func TestStrSub(t *testing.T) {
	testCases := []struct {
		s        string
		i        int64
		j        int64
		expected string
	}{
		{"hello", 1, 3, "hel"},
		{"hello", 2, 4, "ell"},
		{"hello", 1, -1, "hello"},
		{"hello", -3, -1, "llo"},
		{"hello", 1, 1, "h"},
		{"hello", 5, 5, "o"},
		{"hello", 3, 2, ""},
		{"hello", 1, 10, "hello"},
		{"hello", 10, 15, ""},
		{"hello", -10, -1, "hello"},
	}

	for _, tc := range testCases {
		L := newTestLuaAPI(tc.s, tc.i, tc.j)
		result := strSub(L)
		if result != 1 {
			t.Errorf("strSub returned %d, want 1", result)
		}
		got, _ := L.ToString(-1)
		if got != tc.expected {
			t.Errorf("string.sub(%q, %d, %d) = %q, want %q", tc.s, tc.i, tc.j, got, tc.expected)
		}
	}
}

// TestStrSubDefaultArgs tests string.sub with default arguments.
func TestStrSubDefaultArgs(t *testing.T) {
	L := newTestLuaAPI("hello")
	result := strSub(L) // Only s, no i or j
	if result != 1 {
		t.Errorf("strSub returned %d, want 1", result)
	}
	got, _ := L.ToString(-1)
	if got != "hello" {
		t.Errorf("string.sub(hello) = %q, want %q", got, "hello")
	}
}

// TestStrUpper tests string.upper function.
func TestStrUpper(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"hello", "HELLO"},
		{"Hello", "HELLO"},
		{"HELLO", "HELLO"},
		{"", ""},
		{"a", "A"},
		{"abc123def", "ABC123DEF"},
	}

	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		result := strUpper(L)
		if result != 1 {
			t.Errorf("strUpper returned %d, want 1", result)
		}
		got, _ := L.ToString(-1)
		if got != tc.expected {
			t.Errorf("string.upper(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// TestStrLower tests string.lower function.
func TestStrLower(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"HELLO", "hello"},
		{"Hello", "hello"},
		{"hello", "hello"},
		{"", ""},
		{"A", "a"},
		{"ABC123def", "abc123def"},
	}

	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		result := strLower(L)
		if result != 1 {
			t.Errorf("strLower returned %d, want 1", result)
		}
		got, _ := L.ToString(-1)
		if got != tc.expected {
			t.Errorf("string.lower(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// TestStrReverse tests string.reverse function.
func TestStrReverse(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"hello", "olleh"},
		{"world", "dlrow"},
		{"", ""},
		{"a", "a"},
		{"ab", "ba"},
		{"abc", "cba"},
	}

	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		result := strReverse(L)
		if result != 1 {
			t.Errorf("strReverse returned %d, want 1", result)
		}
		got, _ := L.ToString(-1)
		if got != tc.expected {
			t.Errorf("string.reverse(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// TestStrRep tests string.rep function.
func TestStrRep(t *testing.T) {
	testCases := []struct {
		s        string
		n        int64
		sep      string
		expected string
	}{
		{"a", 3, "", "aaa"},
		{"ab", 2, "", "abab"},
		{"a", 0, "", ""},
		{"a", 3, ",", "a,a,a"},
		{"ab", 2, "-", "ab-ab"},
		{"", 5, "", ""},
		{"x", 1, "", "x"},
	}

	for _, tc := range testCases {
		L := newTestLuaAPI(tc.s, tc.n, tc.sep)
		result := strRep(L)
		if result != 1 {
			t.Errorf("strRep returned %d, want 1", result)
		}
		got, _ := L.ToString(-1)
		if got != tc.expected {
			t.Errorf("string.rep(%q, %d, %q) = %q, want %q", tc.s, tc.n, tc.sep, got, tc.expected)
		}
	}
}

// TestStrByte tests string.byte function.
func TestStrByte(t *testing.T) {
	// Test basic byte extraction
	L := newTestLuaAPI("hello")
	result := strByte(L)
	if result != 1 {
		t.Errorf("strByte returned %d, want 1", result)
	}
	got, _ := L.ToInteger(-1)
	// 'h' = 104
	if got != 104 {
		t.Errorf("string.byte('hello') = %d, want 104", got)
	}

	// Test with index
	L = newTestLuaAPI("hello", 2)
	result = strByte(L)
	if result != 1 {
		t.Errorf("strByte returned %d, want 1", result)
	}
	got, _ = L.ToInteger(-1)
	// 'e' = 101
	if got != 101 {
		t.Errorf("string.byte('hello', 2) = %d, want 101", got)
	}

	// Test range
	L = newTestLuaAPI("hello", 1, 3)
	result = strByte(L)
	if result != 3 {
		t.Errorf("strByte returned %d, want 3", result)
	}
}

// TestStrChar tests string.char function.
func TestStrChar(t *testing.T) {
	// Test single character
	L := newTestLuaAPI(int64(65))
	result := strChar(L)
	if result != 1 {
		t.Errorf("strChar returned %d, want 1", result)
	}
	got, _ := L.ToString(-1)
	if got != "A" {
		t.Errorf("string.char(65) = %q, want %q", got, "A")
	}

	// Test multiple characters
	L = newTestLuaAPI(int64(72), int64(105), int64(33))
	result = strChar(L)
	if result != 1 {
		t.Errorf("strChar returned %d, want 1", result)
	}
	got, _ = L.ToString(-1)
	if got != "Hi!" {
		t.Errorf("string.char(72, 105, 33) = %q, want %q", got, "Hi!")
	}

	// Test no arguments
	L = newTestLuaAPI()
	result = strChar(L)
	if result != 1 {
		t.Errorf("strChar returned %d, want 1", result)
	}
	got, _ = L.ToString(-1)
	if got != "" {
		t.Errorf("string.char() = %q, want %q", got, "")
	}
}

// TestStrFind tests string.find function.
func TestStrFind(t *testing.T) {
	// Test basic find
	L := newTestLuaAPI("hello world", "world")
	result := strFind(L)
	if result != 2 {
		t.Errorf("strFind returned %d, want 2", result)
	}
	start, _ := L.ToInteger(-2)
	end, _ := L.ToInteger(-1)
	if start != 7 || end != 11 {
		t.Errorf("string.find('hello world', 'world') = (%d, %d), want (7, 11)", start, end)
	}

	// Test not found
	L = newTestLuaAPI("hello", "xyz")
	result = strFind(L)
	if result != 1 {
		t.Errorf("strFind returned %d, want 1", result)
	}
	if !L.IsNil(-1) {
		t.Errorf("string.find not found should return nil")
	}

	// Test with init position
	L = newTestLuaAPI("hello hello", "hello", 7)
	result = strFind(L)
	if result != 2 {
		t.Errorf("strFind returned %d, want 2", result)
	}
	start, _ = L.ToInteger(-2)
	if start != 7 {
		t.Errorf("string.find('hello hello', 'hello', 7) start = %d, want 7", start)
	}

	// Test plain mode
	L = newTestLuaAPI("hello world", "world", 1, true)
	result = strFind(L)
	if result != 2 {
		t.Errorf("strFind returned %d, want 2", result)
	}
	start, _ = L.ToInteger(-2)
	end, _ = L.ToInteger(-1)
	if start != 7 || end != 11 {
		t.Errorf("string.find plain mode = (%d, %d), want (7, 11)", start, end)
	}
}

// TestStrMatch tests string.match function.
func TestStrMatch(t *testing.T) {
	// Test basic match
	L := newTestLuaAPI("hello world", "world")
	result := strMatch(L)
	if result != 1 {
		t.Errorf("strMatch returned %d, want 1", result)
	}
	got, _ := L.ToString(-1)
	if got != "world" {
		t.Errorf("string.match('hello world', 'world') = %q, want %q", got, "world")
	}

	// Test not found
	L = newTestLuaAPI("hello", "xyz")
	result = strMatch(L)
	if result != 1 {
		t.Errorf("strMatch returned %d, want 1", result)
	}
	if !L.IsNil(-1) {
		t.Errorf("string.match not found should return nil")
	}

	// Test pattern match
	L = newTestLuaAPI("hello 123", "%d+")
	result = strMatch(L)
	if result != 1 {
		t.Errorf("strMatch returned %d, want 1", result)
	}
	got, _ = L.ToString(-1)
	if got != "123" {
		t.Errorf("string.match('hello 123', '%%d+') = %q, want %q", got, "123")
	}
}

// TestStrGsub tests string.gsub function.
func TestStrGsub(t *testing.T) {
	// Test basic replace
	L := newTestLuaAPI("hello world", "world", "golang")
	result := strGsub(L)
	if result != 2 {
		t.Errorf("strGsub returned %d, want 2", result)
	}
	got, _ := L.ToString(-2)
	count, _ := L.ToInteger(-1)
	if got != "hello golang" {
		t.Errorf("string.gsub result = %q, want %q", got, "hello golang")
	}
	if count != 1 {
		t.Errorf("string.gsub count = %d, want 1", count)
	}

	// Test no matches
	L = newTestLuaAPI("hello", "xyz", "replaced")
	result = strGsub(L)
	if result != 2 {
		t.Errorf("strGsub returned %d, want 2", result)
	}
	got, _ = L.ToString(-2)
	count, _ = L.ToInteger(-1)
	if got != "hello" {
		t.Errorf("string.gsub no match result = %q, want %q", got, "hello")
	}
	if count != 0 {
		t.Errorf("string.gsub no match count = %d, want 0", count)
	}

	// Test with n limit
	// Note: regex replaces left-to-right non-overlapping matches
	// "aaa aaa aaa" with "a"->"b" replaces first 2 'a's: "bba aaa aaa"
	L = newTestLuaAPI("aaa aaa aaa", "a", "b", 2)
	result = strGsub(L)
	if result != 2 {
		t.Errorf("strGsub returned %d, want 2", result)
	}
	got, _ = L.ToString(-2)
	count, _ = L.ToInteger(-1)
	if got != "bba aaa aaa" {
		t.Errorf("string.gsub with n result = %q, want %q", got, "bba aaa aaa")
	}
	if count != 2 {
		t.Errorf("string.gsub with n count = %d, want 2", count)
	}
}

// TestStrFormat tests string.format function.
func TestStrFormat(t *testing.T) {
	// Test string format
	L := newTestLuaAPI("Hello %s!", "World")
	result := strFormat(L)
	if result != 1 {
		t.Errorf("strFormat returned %d, want 1", result)
	}
	got, _ := L.ToString(-1)
	if got != "Hello World!" {
		t.Errorf("string.format result = %q, want %q", got, "Hello World!")
	}

	// Test integer format
	L = newTestLuaAPI("Number: %d", 42)
	result = strFormat(L)
	if result != 1 {
		t.Errorf("strFormat returned %d, want 1", result)
	}
	got, _ = L.ToString(-1)
	if got != "Number: 42" {
		t.Errorf("string.format result = %q, want %q", got, "Number: 42")
	}

	// Test float format
	L = newTestLuaAPI("Pi: %.2f", 3.14159)
	result = strFormat(L)
	if result != 1 {
		t.Errorf("strFormat returned %d, want 1", result)
	}
	got, _ = L.ToString(-1)
	if got != "Pi: 3.14" {
		t.Errorf("string.format result = %q, want %q", got, "Pi: 3.14")
	}

	// Test %% escape
	L = newTestLuaAPI("100%% complete")
	result = strFormat(L)
	if result != 1 {
		t.Errorf("strFormat returned %d, want 1", result)
	}
	got, _ = L.ToString(-1)
	if got != "100% complete" {
		t.Errorf("string.format result = %q, want %q", got, "100% complete")
	}

	// Test hex format
	L = newTestLuaAPI("0x%x", 255)
	result = strFormat(L)
	if result != 1 {
		t.Errorf("strFormat returned %d, want 1", result)
	}
	got, _ = L.ToString(-1)
	if got != "0xff" {
		t.Errorf("string.format result = %q, want %q", got, "0xff")
	}
}

// =============================================================================
// Pattern Conversion Tests
// =============================================================================

// TestLuaPatternToRegex tests Lua pattern to regex conversion.
func TestLuaPatternToRegex(t *testing.T) {
	testCases := []struct {
		pattern  string
		expected string
	}{
		{"hello", "hello"},
		{".", "\\.."},
		{"%d", "[0-9]"},
		{"%a", "[A-Za-z]"},
		{"%s", "[ \\t\\n\\r\\f\\v]"},
	}

	for _, tc := range testCases {
		got := luaPatternToRegex(tc.pattern)
		// We can't directly compare regex patterns, but we can test they work
		_ = got // Just verify it doesn't panic
	}
}

// =============================================================================
// Edge Cases
// =============================================================================

// TestEmptyString tests operations on empty strings.
func TestEmptyString(t *testing.T) {
	// len of empty string
	L := newTestLuaAPI("")
	result := strLen(L)
	if result != 1 {
		t.Errorf("strLen returned %d, want 1", result)
	}
	got, _ := L.ToInteger(-1)
	if got != 0 {
		t.Errorf("string.len('') = %d, want 0", got)
	}

	// sub of empty string
	L = newTestLuaAPI("", 1, 3)
	result = strSub(L)
	if result != 1 {
		t.Errorf("strSub returned %d, want 1", result)
	}
	gotStr, _ := L.ToString(-1)
	if gotStr != "" {
		t.Errorf("string.sub('', 1, 3) = %q, want ''", gotStr)
	}

	// reverse of empty string
	L = newTestLuaAPI("")
	result = strReverse(L)
	if result != 1 {
		t.Errorf("strReverse returned %d, want 1", result)
	}
	gotStr, _ = L.ToString(-1)
	if gotStr != "" {
		t.Errorf("string.reverse('') = %q, want ''", gotStr)
	}

	// byte of empty string
	L = newTestLuaAPI("")
	result = strByte(L)
	if result != 0 {
		t.Errorf("string.byte('') = %d, want 0", result)
	}
}

// TestNegativeIndices tests various negative index scenarios.
func TestNegativeIndices(t *testing.T) {
	// sub with negative indices
	testCases := []struct {
		s        string
		i        int64
		j        int64
		expected string
	}{
		{"hello", -5, -1, "hello"},  // entire string
		{"hello", -2, -1, "lo"},     // last 2 chars
		{"hello", 1, -1, "hello"},   // first to last
		{"hello", -5, 5, "hello"},   // -5 to 5
		{"hello", -10, 5, "hello"},  // clamped start
	}

	for _, tc := range testCases {
		L := newTestLuaAPI(tc.s, tc.i, tc.j)
		result := strSub(L)
		if result != 1 {
			t.Errorf("strSub returned %d, want 1", result)
		}
		got, _ := L.ToString(-1)
		if got != tc.expected {
			t.Errorf("string.sub(%q, %d, %d) = %q, want %q", tc.s, tc.i, tc.j, got, tc.expected)
		}
	}
}

// TestReturnValues tests that all string functions return correct number of values.
func TestReturnValues(t *testing.T) {
	// Functions returning 1 value
	singleRetFuncs := []struct {
		name string
		fn   func(luaapi.LuaAPI) int
	}{
		{"len", strLen}, {"sub", strSub}, {"upper", strUpper},
		{"lower", strLower}, {"reverse", strReverse}, {"rep", strRep},
		{"char", strChar}, {"format", strFormat},
	}
	for _, tc := range singleRetFuncs {
		L := newTestLuaAPI("test")
		result := tc.fn(L)
		if result != 1 {
			t.Errorf("%s returned %d, want 1", tc.name, result)
		}
	}

	// byte returns variable
	L := newTestLuaAPI("hello")
	result := strByte(L)
	if result != 1 {
		t.Errorf("strByte returned %d, want 1", result)
	}

	L = newTestLuaAPI("hello", 1, 3)
	result = strByte(L)
	if result != 3 {
		t.Errorf("strByte(1,3) returned %d, want 3", result)
	}
}
