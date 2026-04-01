// Package internal provides tests for the math library.
package internal

import (
	"math"
	"testing"

	luaapi "github.com/akzj/go-lua/api"
	tableapi "github.com/akzj/go-lua/table/api"
	mathlib "github.com/akzj/go-lua/lib/math/api"
)

// =============================================================================
// Constructor Tests
// =============================================================================

// TestNewMathLib tests creating a new MathLib instance.
func TestNewMathLib(t *testing.T) {
	lib := NewMathLib()
	if lib == nil {
		t.Error("NewMathLib() returned nil")
	}
}

// TestMathLibImplementsInterface tests that MathLib implements MathLib interface.
func TestMathLibImplementsInterface(t *testing.T) {
	var lib mathlib.MathLib = NewMathLib()
	if lib == nil {
		t.Error("MathLib does not implement mathlib.MathLib interface")
	}
}

// =============================================================================
// LuaFunc Signature Tests
// =============================================================================

// TestLuaFuncSignatures tests that all math functions have correct LuaFunc signature.
func TestLuaFuncSignatures(t *testing.T) {
	// Basic Arithmetic Functions
	var _ mathlib.LuaFunc = mathAbs
	var _ mathlib.LuaFunc = mathCeil
	var _ mathlib.LuaFunc = mathFloor

	// Min/Max Functions
	var _ mathlib.LuaFunc = mathMax
	var _ mathlib.LuaFunc = mathMin

	// Power and Root Functions
	var _ mathlib.LuaFunc = mathSqrt
	var _ mathlib.LuaFunc = mathPow

	// Trigonometric Functions
	var _ mathlib.LuaFunc = mathSin
	var _ mathlib.LuaFunc = mathCos
	var _ mathlib.LuaFunc = mathTan

	// Inverse Trigonometric Functions
	var _ mathlib.LuaFunc = mathAsin
	var _ mathlib.LuaFunc = mathAcos
	var _ mathlib.LuaFunc = mathAtan
	var _ mathlib.LuaFunc = mathAtan2

	// Logarithmic and Exponential Functions
	var _ mathlib.LuaFunc = mathLog
	var _ mathlib.LuaFunc = mathLog10
	var _ mathlib.LuaFunc = mathExp

	// Angle Conversion Functions
	var _ mathlib.LuaFunc = mathDeg
	var _ mathlib.LuaFunc = mathRad

	// Random Number Functions
	var _ mathlib.LuaFunc = mathRandom
	var _ mathlib.LuaFunc = mathRandomseed
}

// =============================================================================
// Helper Functions Tests
// =============================================================================

// TestHelperFunctionsExist verifies helper functions exist.
func TestHelperFunctionsExist(t *testing.T) {
	_ = toNumber
	_ = optNumber
	_ = toInteger
	_ = SetGlobalSeed
}

// =============================================================================
// Global RNG Tests
// =============================================================================

// TestSetGlobalSeed tests the SetGlobalSeed function.
func TestSetGlobalSeed(t *testing.T) {
	SetGlobalSeed(42)
	SetGlobalSeed(0)
	SetGlobalSeed(-1)
}

// =============================================================================
// testLuaAPI is a mock implementation of LuaAPI for testing.
// Uses 1-indexed stack to match Lua semantics.
type testLuaAPI struct {
	stack []interface{}
}

func newTestLuaAPI(values ...float64) *testLuaAPI {
	stack := make([]interface{}, 0, 20)
	for _, v := range values {
		stack = append(stack, v)
	}
	return &testLuaAPI{stack: stack}
}

func (t *testLuaAPI) GetTop() int                    { return len(t.stack) }
func (t *testLuaAPI) SetTop(idx int)                  {
	for len(t.stack) < idx {
		t.stack = append(t.stack, nil)
	}
	t.stack = t.stack[:idx]
}
func (t *testLuaAPI) Pop()                            { t.stack = t.stack[:len(t.stack)-1] }
func (t *testLuaAPI) PushValue(idx int)               {}
func (t *testLuaAPI) AbsIndex(idx int) int            { return idx }
func (t *testLuaAPI) Rotate(idx, n int)               {}
func (t *testLuaAPI) Copy(fromidx, toidx int)         {}
func (t *testLuaAPI) CheckStack(n int) bool           { return true }
func (t *testLuaAPI) XMove(to luaapi.LuaAPI, n int)   {}

// Type Checking
func (t *testLuaAPI) Type(idx int) int                { return luaapi.LUA_TNUMBER }
func (t *testLuaAPI) TypeName(tp int) string          { return "number" }
func (t *testLuaAPI) IsNone(idx int) bool             { return idx > len(t.stack) }
func (t *testLuaAPI) IsNil(idx int) bool              { return false }
func (t *testLuaAPI) IsNoneOrNil(idx int) bool        { return idx > len(t.stack) }
func (t *testLuaAPI) IsBoolean(idx int) bool          { return false }
func (t *testLuaAPI) IsString(idx int) bool           { return false }
func (t *testLuaAPI) IsFunction(idx int) bool         { return false }
func (t *testLuaAPI) IsTable(idx int) bool            { return false }
func (t *testLuaAPI) IsLightUserData(idx int) bool    { return false }
func (t *testLuaAPI) IsThread(idx int) bool           { return false }
func (t *testLuaAPI) IsInteger(idx int) bool          { return true }
func (t *testLuaAPI) IsNumber(idx int) bool           { return true }

// Value Conversion - handle negative indices properly
func (t *testLuaAPI) ToInteger(idx int) (int64, bool) {
	v := t.getValue(idx)
	switch n := v.(type) {
	case int64:
		return n, true
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
func (t *testLuaAPI) getValue(idx int) interface{} {
	if idx < 0 {
		idx = len(t.stack) + idx + 1
	}
	if idx < 1 || idx > len(t.stack) {
		return nil
	}
	return t.stack[idx-1]
}
func (t *testLuaAPI) ToString(idx int) (string, bool) { return "", false }
func (t *testLuaAPI) ToBoolean(idx int) bool          { return true }
func (t *testLuaAPI) ToPointer(idx int) interface{}   { return nil }
func (t *testLuaAPI) ToThread(idx int) luaapi.LuaAPI  { return nil }

// Push Functions
func (t *testLuaAPI) PushNil()                            {}
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

// Internal
func (t *testLuaAPI) Stack() []interface{}              { return t.stack }
func (t *testLuaAPI) StackSize() int                   { return len(t.stack) }
func (t *testLuaAPI) GrowStack(n int)                  {}
func (t *testLuaAPI) CurrentCI() interface{}           { return nil }
func (t *testLuaAPI) PushCI(ci interface{})             {}
func (t *testLuaAPI) PopCI()                           {}

// =============================================================================
// Math Function Tests
// =============================================================================

// TestMathAbsFunction tests math.abs function returns correct values.
func TestMathAbsFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{-5.0, 5.0}, {5.0, 5.0}, {0.0, 0.0}, {-3.14, 3.14},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathAbs(L)
		got, _ := L.ToNumber(-1)
		if got != tc.expected {
			t.Errorf("math.abs(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathCeilFunction tests math.ceil function returns correct values.
func TestMathCeilFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{3.2, 4.0}, {3.8, 4.0}, {3.0, 3.0}, {-3.2, -3.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathCeil(L)
		got, _ := L.ToNumber(-1)
		if got != tc.expected {
			t.Errorf("math.ceil(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathFloorFunction tests math.floor function returns correct values.
func TestMathFloorFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{3.2, 3.0}, {3.8, 3.0}, {3.0, 3.0}, {-3.2, -4.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathFloor(L)
		got, _ := L.ToNumber(-1)
		if got != tc.expected {
			t.Errorf("math.floor(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathSqrtFunction tests math.sqrt function returns correct values.
func TestMathSqrtFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{16.0, 4.0}, {9.0, 3.0}, {2.0, math.Sqrt2}, {0.0, 0.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathSqrt(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.sqrt(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathPowFunction tests math.pow function returns correct values.
func TestMathPowFunction(t *testing.T) {
	testCases := []struct {
		x, y     float64
		expected float64
	}{
		{2.0, 3.0, 8.0}, {2.0, 0.0, 1.0}, {4.0, 0.5, 2.0}, {2.0, -1.0, 0.5},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.x, tc.y)
		mathPow(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.pow(%v, %v) = %v, want %v", tc.x, tc.y, got, tc.expected)
		}
	}
}

// TestMathSinFunction tests math.sin function returns correct values.
func TestMathSinFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{0.0, 0.0}, {math.Pi / 2, 1.0}, {math.Pi, 0.0}, {-math.Pi / 2, -1.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathSin(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.sin(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathCosFunction tests math.cos function returns correct values.
func TestMathCosFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{0.0, 1.0}, {math.Pi, -1.0}, {math.Pi / 2, 0.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathCos(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.cos(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathTanFunction tests math.tan function returns correct values.
func TestMathTanFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{0.0, 0.0}, {math.Pi / 4, 1.0}, {-math.Pi / 4, -1.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathTan(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.tan(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathAsinFunction tests math.asin function returns correct values.
func TestMathAsinFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{0.0, 0.0}, {1.0, math.Pi / 2}, {-1.0, -math.Pi / 2}, {0.5, math.Pi / 6},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathAsin(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.asin(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathAcosFunction tests math.acos function returns correct values.
func TestMathAcosFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{1.0, 0.0}, {0.0, math.Pi / 2}, {-1.0, math.Pi}, {0.5, math.Pi / 3},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathAcos(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.acos(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathAtanFunction tests math.atan function returns correct values.
func TestMathAtanFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{0.0, 0.0}, {1.0, math.Pi / 4}, {-1.0, -math.Pi / 4}, {math.Inf(1), math.Pi / 2},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathAtan(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.atan(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathAtan2Function tests math.atan2 function returns correct values.
func TestMathAtan2Function(t *testing.T) {
	testCases := []struct {
		y, x     float64
		expected float64
	}{
		{1.0, 1.0, math.Pi / 4}, {1.0, -1.0, 3 * math.Pi / 4}, {0.0, 1.0, 0.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.y, tc.x)
		mathAtan2(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.atan2(%v, %v) = %v, want %v", tc.y, tc.x, got, tc.expected)
		}
	}
}

// TestMathLogFunction tests math.log function returns correct values.
func TestMathLogFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{math.E, 1.0}, {1.0, 0.0}, {math.E * math.E, 2.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathLog(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.log(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathLogWithBaseFunction tests math.log(x, base) returns correct values.
func TestMathLogWithBaseFunction(t *testing.T) {
	testCases := []struct {
		x, base  float64
		expected float64
	}{
		{8.0, 2.0, 3.0}, {100.0, 10.0, 2.0}, {1.0, 2.0, 0.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.x, tc.base)
		mathLog(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.log(%v, %v) = %v, want %v", tc.x, tc.base, got, tc.expected)
		}
	}
}

// TestMathLog10Function tests math.log10 function returns correct values.
func TestMathLog10Function(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{100.0, 2.0}, {1000.0, 3.0}, {1.0, 0.0}, {10.0, 1.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathLog10(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.log10(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathExpFunction tests math.exp function returns correct values.
func TestMathExpFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{0.0, 1.0}, {1.0, math.E}, {2.0, math.E * math.E}, {-1.0, 1.0 / math.E},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathExp(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.exp(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathDegFunction tests math.deg function returns correct values.
func TestMathDegFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{math.Pi, 180.0}, {0.0, 0.0}, {math.Pi / 2, 90.0}, {2 * math.Pi, 360.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathDeg(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.deg(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathRadFunction tests math.rad function returns correct values.
func TestMathRadFunction(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{180.0, math.Pi}, {0.0, 0.0}, {90.0, math.Pi / 2}, {360.0, 2 * math.Pi},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.input)
		mathRad(L)
		got, _ := L.ToNumber(-1)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("math.rad(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// TestMathMaxFunction tests math.max function returns correct values.
func TestMathMaxFunction(t *testing.T) {
	testCases := []struct {
		args     []float64
		expected float64
	}{
		{[]float64{1.0, 2.0, 3.0}, 3.0},
		{[]float64{-5.0, -2.0, -1.0}, -1.0},
		{[]float64{42.0}, 42.0},
		{[]float64{5.0, 5.0, 5.0}, 5.0},
		{[]float64{10.0, 5.0, 15.0, 2.0}, 15.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.args...)
		mathMax(L)
		got, _ := L.ToNumber(-1)
		if got != tc.expected {
			t.Errorf("math.max(%v) = %v, want %v", tc.args, got, tc.expected)
		}
	}
}

// TestMathMinFunction tests math.min function returns correct values.
func TestMathMinFunction(t *testing.T) {
	testCases := []struct {
		args     []float64
		expected float64
	}{
		{[]float64{1.0, 2.0, 3.0}, 1.0},
		{[]float64{-5.0, -2.0, -1.0}, -5.0},
		{[]float64{42.0}, 42.0},
		{[]float64{5.0, 5.0, 5.0}, 5.0},
		{[]float64{10.0, 5.0, 15.0, 2.0}, 2.0},
	}
	for _, tc := range testCases {
		L := newTestLuaAPI(tc.args...)
		mathMin(L)
		got, _ := L.ToNumber(-1)
		if got != tc.expected {
			t.Errorf("math.min(%v) = %v, want %v", tc.args, got, tc.expected)
		}
	}
}

// TestMathRandomNoArgs tests math.random() with no arguments.
func TestMathRandomNoArgs(t *testing.T) {
	SetGlobalSeed(42)
	L := newTestLuaAPI()
	mathRandom(L)
	got, _ := L.ToNumber(-1)
	if got < 0 || got >= 1 {
		t.Errorf("math.random() = %v, expected [0, 1)", got)
	}
}

// TestMathRandomOneArg tests math.random(n) with one argument.
func TestMathRandomOneArg(t *testing.T) {
	SetGlobalSeed(42)
	L := newTestLuaAPI(10.0)
	mathRandom(L)
	got, _ := L.ToInteger(-1)
	if got < 1 || got > 10 {
		t.Errorf("math.random(10) = %v, expected [1, 10]", got)
	}
}

// TestMathRandomTwoArgs tests math.random(n, m) with two arguments.
func TestMathRandomTwoArgs(t *testing.T) {
	SetGlobalSeed(42)
	L := newTestLuaAPI(5.0, 15.0)
	mathRandom(L)
	got, _ := L.ToInteger(-1)
	if got < 5 || got > 15 {
		t.Errorf("math.random(5, 15) = %v, expected [5, 15]", got)
	}
}

// TestMathRandomseedFunction tests math.randomseed returns no value.
func TestMathRandomseedFunction(t *testing.T) {
	L := newTestLuaAPI(12345.0)
	result := mathRandomseed(L)
	if result != 0 {
		t.Errorf("math.randomseed returned %d, want 0", result)
	}
}

// TestMathRandomDeterministic tests that same seed produces same sequence.
func TestMathRandomDeterministic(t *testing.T) {
	SetGlobalSeed(100)
	L1 := newTestLuaAPI()
	mathRandom(L1)
	val1, _ := L1.ToNumber(-1)

	SetGlobalSeed(100)
	L2 := newTestLuaAPI()
	mathRandom(L2)
	val2, _ := L2.ToNumber(-1)

	if val1 != val2 {
		t.Errorf("Same seed should produce same sequence: %v != %v", val1, val2)
	}
}

// TestMathRandomSwappedArgs tests that swapped args produce same result.
func TestMathRandomSwappedArgs(t *testing.T) {
	SetGlobalSeed(42)
	L1 := newTestLuaAPI(5.0, 10.0)
	mathRandom(L1)
	val1, _ := L1.ToInteger(-1)

	SetGlobalSeed(42)
	L2 := newTestLuaAPI(10.0, 5.0)
	mathRandom(L2)
	val2, _ := L2.ToInteger(-1)

	if val1 != val2 {
		t.Errorf("Same seed with swapped args should produce same result: %v != %v", val1, val2)
	}
}

// TestReturnValues tests that all math functions return correct number of values.
func TestReturnValues(t *testing.T) {
	// Single-argument functions should return 1
	singleArgFuncs := []struct {
		name string
		fn   func(luaapi.LuaAPI) int
	}{
		{"abs", mathAbs}, {"ceil", mathCeil}, {"floor", mathFloor},
		{"sqrt", mathSqrt}, {"sin", mathSin}, {"cos", mathCos},
		{"tan", mathTan}, {"asin", mathAsin}, {"acos", mathAcos},
		{"atan", mathAtan}, {"log10", mathLog10}, {"exp", mathExp},
		{"deg", mathDeg}, {"rad", mathRad},
	}
	for _, tc := range singleArgFuncs {
		L := newTestLuaAPI(1.0)
		result := tc.fn(L)
		if result != 1 {
			t.Errorf("%s returned %d, want 1", tc.name, result)
		}
	}

	// Two-argument functions should return 1
	twoArgFuncs := []struct {
		name string
		fn   func(luaapi.LuaAPI) int
	}{
		{"pow", mathPow}, {"atan2", mathAtan2}, {"log", mathLog},
	}
	for _, tc := range twoArgFuncs {
		L := newTestLuaAPI(2.0, 3.0)
		result := tc.fn(L)
		if result != 1 {
			t.Errorf("%s returned %d, want 1", tc.name, result)
		}
	}

	// randomseed returns 0
	L := newTestLuaAPI(42.0)
	result := mathRandomseed(L)
	if result != 0 {
		t.Errorf("randomseed returned %d, want 0", result)
	}
}
