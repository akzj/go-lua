package api

import (
	"fmt"
	"testing"

	closureapi "github.com/akzj/go-lua/internal/closure/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	parseapi "github.com/akzj/go-lua/internal/parse/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
	tableapi "github.com/akzj/go-lua/internal/table/api"
)

// stringReader implements lexapi.LexReader for test strings.
type stringReader struct {
	data string
	pos  int
}

func newStringReader(s string) *stringReader {
	return &stringReader{data: s}
}

func (r *stringReader) ReadByte() int {
	if r.pos >= len(r.data) {
		return -1
	}
	b := r.data[r.pos]
	r.pos++
	return int(b)
}

// luaResult holds a Lua execution result.
type luaResult struct {
	L   *stateapi.LuaState
	top int // Top after execution
}

// runLua compiles and executes Lua source code.
func runLua(t *testing.T, src string) luaResult {
	t.Helper()
	L := stateapi.NewState()
	reader := newStringReader(src)
	proto := parseapi.Parse("test", reader)
	cl := closureapi.NewLClosure(proto, len(proto.Upvalues))
	if len(cl.UpVals) > 0 {
		gt := GetGlobalTable(L)
		uv := &closureapi.UpVal{}
		uv.Close(objectapi.TValue{Tt: objectapi.TagTable, Val: gt})
		cl.UpVals[0] = uv
	}
	funcIdx := L.Top
	stateapi.PushValue(L, objectapi.TValue{Tt: objectapi.TagLuaClosure, Val: cl})
	Call(L, funcIdx, stateapi.MultiRet)
	return luaResult{L: L, top: L.Top}
}

// get returns the i-th result (0-based from end of stack).
// Results are placed contiguously ending at Top.
// For i=0 returns first result, i=1 second, etc.
func (r luaResult) get(i int) objectapi.TValue {
	// After Call with MultiRet on a main chunk (vararg):
	// Results are at Stack[0..Top-1] because RETURN undoes VARARGPREP shift.
	// nResults = Top (since results start at 0 for main chunks).
	if i < r.top {
		return r.L.Stack[i].Val
	}
	return objectapi.Nil
}

func (r luaResult) nResults() int {
	return r.top
}

// --- Test: Empty program ---

func TestEmptyProgram(t *testing.T) {
	r := runLua(t, "")
	_ = r
}

// --- Test: Return values ---

func TestReturnInteger(t *testing.T) {
	r := runLua(t, "return 42")
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 42 {
		t.Fatalf("expected 42, got tag=%d val=%v", v.Tt, v.Val)
	}
}

func TestReturnFloat(t *testing.T) {
	r := runLua(t, "return 3.14")
	v := r.get(0)
	if !v.IsFloat() || v.Float() != 3.14 {
		t.Fatalf("expected 3.14, got tag=%d val=%v", v.Tt, v.Val)
	}
}

func TestReturnString(t *testing.T) {
	r := runLua(t, `return "hello"`)
	v := r.get(0)
	if !v.IsString() || v.StringVal().Data != "hello" {
		t.Fatalf("expected 'hello', got tag=%d val=%v", v.Tt, v.Val)
	}
}

func TestReturnTrue(t *testing.T) {
	r := runLua(t, "return true")
	if r.get(0).Tt != objectapi.TagTrue {
		t.Fatalf("expected true, got tag %d", r.get(0).Tt)
	}
}

func TestReturnFalse(t *testing.T) {
	r := runLua(t, "return false")
	if r.get(0).Tt != objectapi.TagFalse {
		t.Fatalf("expected false, got tag %d", r.get(0).Tt)
	}
}

func TestReturnNil(t *testing.T) {
	r := runLua(t, "return nil")
	if !r.get(0).IsNil() {
		t.Fatalf("expected nil, got tag %d", r.get(0).Tt)
	}
}

// --- Test: Local variables and arithmetic ---

func TestLocalArithmetic(t *testing.T) {
	r := runLua(t, `
		local a = 10
		local b = 20
		return a + b
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 30 {
		t.Fatalf("expected 30, got %v (tag %d)", v.Val, v.Tt)
	}
}

func TestArithmeticOperators(t *testing.T) {
	tests := []struct {
		src    string
		expect int64
	}{
		{"return 10 + 3", 13},
		{"return 10 - 3", 7},
		{"return 10 * 3", 30},
		{"return 10 % 3", 1},
		{"return -5", -5},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			r := runLua(t, tt.src)
			v := r.get(0)
			if !v.IsInteger() || v.Integer() != tt.expect {
				t.Fatalf("expected %d, got %v (tag %d)", tt.expect, v.Val, v.Tt)
			}
		})
	}
}

func TestFloatArithmetic(t *testing.T) {
	r := runLua(t, "return 10 / 3")
	v := r.get(0)
	if !v.IsFloat() {
		t.Fatalf("expected float, got tag %d", v.Tt)
	}
	expected := 10.0 / 3.0
	if v.Float() != expected {
		t.Fatalf("expected %f, got %f", expected, v.Float())
	}
}

func TestFloorDivision(t *testing.T) {
	r := runLua(t, "return 10 // 3")
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 3 {
		t.Fatalf("expected 3, got %v (tag %d)", v.Val, v.Tt)
	}
}

// --- Test: If/else ---

func TestIfTrue(t *testing.T) {
	r := runLua(t, `
		local x = 10
		if x > 5 then return 1 else return 2 end
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 1 {
		t.Fatalf("expected 1, got %v", v.Val)
	}
}

func TestIfFalse(t *testing.T) {
	r := runLua(t, `
		local x = 3
		if x > 5 then return 1 else return 2 end
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 2 {
		t.Fatalf("expected 2, got %v", v.Val)
	}
}

// --- Test: While loop ---

func TestWhileLoop(t *testing.T) {
	r := runLua(t, `
		local sum = 0
		local i = 1
		while i <= 10 do
			sum = sum + i
			i = i + 1
		end
		return sum
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 55 {
		t.Fatalf("expected 55, got %v (tag %d)", v.Val, v.Tt)
	}
}

// --- Test: Repeat loop ---

func TestRepeatLoop(t *testing.T) {
	r := runLua(t, `
		local sum = 0
		local i = 1
		repeat
			sum = sum + i
			i = i + 1
		until i > 10
		return sum
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 55 {
		t.Fatalf("expected 55, got %v (tag %d)", v.Val, v.Tt)
	}
}

// --- Test: Numeric for loop ---

func TestNumericFor(t *testing.T) {
	r := runLua(t, `
		local sum = 0
		for i = 1, 10 do
			sum = sum + i
		end
		return sum
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 55 {
		t.Fatalf("expected 55, got %v (tag %d)", v.Val, v.Tt)
	}
}

func TestNumericForWithStep(t *testing.T) {
	r := runLua(t, `
		local sum = 0
		for i = 1, 10, 2 do
			sum = sum + i
		end
		return sum
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 25 {
		t.Fatalf("expected 25, got %v (tag %d)", v.Val, v.Tt)
	}
}

// --- Test: Table constructor and access ---

func TestTableConstructor(t *testing.T) {
	r := runLua(t, `
		local t = {10, 20, 30}
		return t[2]
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 20 {
		t.Fatalf("expected 20, got %v (tag %d)", v.Val, v.Tt)
	}
}

func TestTableFieldAccess(t *testing.T) {
	r := runLua(t, `
		local t = {x = 42}
		return t.x
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 42 {
		t.Fatalf("expected 42, got %v (tag %d)", v.Val, v.Tt)
	}
}

func TestTableFieldSet(t *testing.T) {
	r := runLua(t, `
		local t = {}
		t.x = 99
		return t.x
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 99 {
		t.Fatalf("expected 99, got %v (tag %d)", v.Val, v.Tt)
	}
}

// --- Test: Function calls ---

func TestLocalFunction(t *testing.T) {
	r := runLua(t, `
		local function add(a, b)
			return a + b
		end
		return add(3, 4)
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 7 {
		t.Fatalf("expected 7, got %v (tag %d)", v.Val, v.Tt)
	}
}

// --- Test: Closures and upvalues ---

func TestClosure(t *testing.T) {
	r := runLua(t, `
		local function make_counter()
			local count = 0
			return function()
				count = count + 1
				return count
			end
		end
		local c = make_counter()
		c()
		c()
		return c()
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 3 {
		t.Fatalf("expected 3, got %v (tag %d)", v.Val, v.Tt)
	}
}

// --- Test: String concatenation ---

func TestStringConcat(t *testing.T) {
	r := runLua(t, `return "hello" .. " " .. "world"`)
	v := r.get(0)
	if !v.IsString() || v.StringVal().Data != "hello world" {
		t.Fatalf("expected 'hello world', got %v", v.Val)
	}
}

// --- Test: Comparison operators ---

func TestComparisons(t *testing.T) {
	tests := []struct {
		src    string
		expect bool
	}{
		{"return 1 < 2", true},
		{"return 2 < 1", false},
		{"return 1 <= 1", true},
		{"return 2 <= 1", false},
		{"return 1 == 1", true},
		{"return 1 == 2", false},
		{"return 1 ~= 2", true},
		{"return 1 ~= 1", false},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			r := runLua(t, tt.src)
			v := r.get(0)
			if tt.expect {
				if v.Tt != objectapi.TagTrue {
					t.Fatalf("expected true, got tag %d", v.Tt)
				}
			} else {
				if v.Tt != objectapi.TagFalse {
					t.Fatalf("expected false, got tag %d", v.Tt)
				}
			}
		})
	}
}

// --- Test: Multiple return values ---

func TestMultipleReturns(t *testing.T) {
	r := runLua(t, "return 1, 2, 3")
	if r.nResults() < 3 {
		t.Fatalf("expected 3 results, got %d", r.nResults())
	}
	for i, want := range []int64{1, 2, 3} {
		v := r.get(i)
		if !v.IsInteger() || v.Integer() != want {
			t.Fatalf("result[%d]: expected %d, got %v", i, want, v.Val)
		}
	}
}

// --- Test: Not operator ---

func TestNotOperator(t *testing.T) {
	r := runLua(t, "return not false")
	if r.get(0).Tt != objectapi.TagTrue {
		t.Fatalf("expected true, got tag %d", r.get(0).Tt)
	}
}

// --- Test: Nested function calls ---

func TestNestedCalls(t *testing.T) {
	r := runLua(t, `
		local function double(x) return x * 2 end
		local function triple(x) return x * 3 end
		return double(triple(5))
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 30 {
		t.Fatalf("expected 30, got %v", v.Val)
	}
}

// --- Test: Recursive fibonacci ---

func TestFibonacci(t *testing.T) {
	r := runLua(t, `
		local function fib(n)
			if n <= 1 then return n end
			return fib(n - 1) + fib(n - 2)
		end
		return fib(10)
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 55 {
		t.Fatalf("expected 55, got %v (tag %d)", v.Val, v.Tt)
	}
}

// --- Test: Length operator ---

func TestLengthOperator(t *testing.T) {
	r := runLua(t, `return #"hello"`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 5 {
		t.Fatalf("expected 5, got %v (tag %d)", v.Val, v.Tt)
	}
}

// --- Test: Logical operators ---

func TestLogicalAnd(t *testing.T) {
	r := runLua(t, "return 1 and 2")
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 2 {
		t.Fatalf("expected 2, got %v (tag %d)", v.Val, v.Tt)
	}
}

func TestLogicalOr(t *testing.T) {
	r := runLua(t, "return false or 42")
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 42 {
		t.Fatalf("expected 42, got %v (tag %d)", v.Val, v.Tt)
	}
}

// --- Test: C function call ---

func TestCFunction(t *testing.T) {
	L := stateapi.NewState()
	// Register a simple C function in globals
	gt := GetGlobalTable(L)
	addFn := func(L2 *stateapi.LuaState) int {
		a := L2.Stack[L2.CI.Func+1].Val.Integer()
		b := L2.Stack[L2.CI.Func+2].Val.Integer()
		stateapi.PushValue(L2, objectapi.MakeInteger(a+b))
		return 1
	}
	gt.Set(objectapi.MakeString(&objectapi.LuaString{Data: "myadd", IsShort: true}),
		objectapi.TValue{Tt: objectapi.TagLightCFunc, Val: stateapi.CFunction(addFn)})

	reader := newStringReader("return myadd(10, 20)")
	proto := parseapi.Parse("test", reader)
	cl := closureapi.NewLClosure(proto, len(proto.Upvalues))
	if len(cl.UpVals) > 0 {
		uv := &closureapi.UpVal{}
		uv.Close(objectapi.TValue{Tt: objectapi.TagTable, Val: gt})
		cl.UpVals[0] = uv
	}
	funcIdx := L.Top
	stateapi.PushValue(L, objectapi.TValue{Tt: objectapi.TagLuaClosure, Val: cl})
	Call(L, funcIdx, stateapi.MultiRet)
	v := L.Stack[0].Val
	if !v.IsInteger() || v.Integer() != 30 {
		t.Fatalf("expected 30, got %v (tag %d)", v.Val, v.Tt)
	}
}

// Ensure unused imports don't cause build errors
var _ = tableapi.New
var _ = fmt.Sprintf
