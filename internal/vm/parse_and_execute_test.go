package vm

import (
	"fmt"
	"testing"

	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/parse"
	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/table"
)

// stringReader implements lex.LexReader for test strings.
type stringReader struct {
	data string
	pos  int
}

func newStringReader(s string) *stringReader {
	return &stringReader{data: s}
}

func (r *stringReader) NextByte() int {
	if r.pos >= len(r.data) {
		return -1
	}
	b := r.data[r.pos]
	r.pos++
	return int(b)
}

// luaResult holds a Lua execution result.
type luaResult struct {
	L       *state.LuaState
	top     int // Top after execution
	funcIdx int // Where the function was placed (results start here)
}

// runLua compiles and executes Lua source code.
func runLua(t *testing.T, src string) luaResult {
	t.Helper()
	L := state.NewState()
	reader := newStringReader(src)
	proto := parse.Parse("test", reader)
	cl := closure.NewLClosure(proto, len(proto.Upvalues))
	if len(cl.UpVals) > 0 {
		gt := GetGlobalTable(L)
		uv := &closure.UpVal{}
		uv.Close(object.TValue{Tt: object.TagTable, Obj: gt})
		cl.UpVals[0] = uv
	}
	funcIdx := L.Top
	state.PushValue(L, object.TValue{Tt: object.TagLuaClosure, Obj: cl})
	Call(L, funcIdx, state.MultiRet)
	return luaResult{L: L, top: L.Top, funcIdx: funcIdx}
}

// get returns the i-th result (0-based).
// After Call with MultiRet, results start at funcIdx.
func (r luaResult) get(i int) object.TValue {
	idx := r.funcIdx + i
	if idx < r.top {
		return r.L.Stack[idx].Val
	}
	return object.Nil
}

func (r luaResult) nResults() int {
	return r.top - r.funcIdx
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
		t.Fatalf("expected 42, got tag=%d val=%v", v.Tt, v.Payload())
	}
}

func TestReturnFloat(t *testing.T) {
	r := runLua(t, "return 3.14")
	v := r.get(0)
	if !v.IsFloat() || v.Float() != 3.14 {
		t.Fatalf("expected 3.14, got tag=%d val=%v", v.Tt, v.Payload())
	}
}

func TestReturnString(t *testing.T) {
	r := runLua(t, `return "hello"`)
	v := r.get(0)
	if !v.IsString() || v.StringVal().Data != "hello" {
		t.Fatalf("expected 'hello', got tag=%d val=%v", v.Tt, v.Payload())
	}
}

func TestReturnTrue(t *testing.T) {
	r := runLua(t, "return true")
	if r.get(0).Tt != object.TagTrue {
		t.Fatalf("expected true, got tag %d", r.get(0).Tt)
	}
}

func TestReturnFalse(t *testing.T) {
	r := runLua(t, "return false")
	if r.get(0).Tt != object.TagFalse {
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
		t.Fatalf("expected 30, got %v (tag %d)", v.Payload(), v.Tt)
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
				t.Fatalf("expected %d, got %v (tag %d)", tt.expect, v.Payload(), v.Tt)
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
		t.Fatalf("expected 3, got %v (tag %d)", v.Payload(), v.Tt)
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
		t.Fatalf("expected 1, got %v", v.Payload())
	}
}

func TestIfFalse(t *testing.T) {
	r := runLua(t, `
		local x = 3
		if x > 5 then return 1 else return 2 end
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 2 {
		t.Fatalf("expected 2, got %v", v.Payload())
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
		t.Fatalf("expected 55, got %v (tag %d)", v.Payload(), v.Tt)
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
		t.Fatalf("expected 55, got %v (tag %d)", v.Payload(), v.Tt)
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
		t.Fatalf("expected 55, got %v (tag %d)", v.Payload(), v.Tt)
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
		t.Fatalf("expected 25, got %v (tag %d)", v.Payload(), v.Tt)
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
		t.Fatalf("expected 20, got %v (tag %d)", v.Payload(), v.Tt)
	}
}

func TestTableFieldAccess(t *testing.T) {
	r := runLua(t, `
		local t = {x = 42}
		return t.x
	`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 42 {
		t.Fatalf("expected 42, got %v (tag %d)", v.Payload(), v.Tt)
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
		t.Fatalf("expected 99, got %v (tag %d)", v.Payload(), v.Tt)
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
		t.Fatalf("expected 7, got %v (tag %d)", v.Payload(), v.Tt)
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
		t.Fatalf("expected 3, got %v (tag %d)", v.Payload(), v.Tt)
	}
}

// --- Test: String concatenation ---

func TestStringConcat(t *testing.T) {
	r := runLua(t, `return "hello" .. " " .. "world"`)
	v := r.get(0)
	if !v.IsString() || v.StringVal().Data != "hello world" {
		t.Fatalf("expected 'hello world', got %v", v.Payload())
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
				if v.Tt != object.TagTrue {
					t.Fatalf("expected true, got tag %d", v.Tt)
				}
			} else {
				if v.Tt != object.TagFalse {
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
			t.Fatalf("result[%d]: expected %d, got %v", i, want, v.Payload())
		}
	}
}

// --- Test: Not operator ---

func TestNotOperator(t *testing.T) {
	r := runLua(t, "return not false")
	if r.get(0).Tt != object.TagTrue {
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
		t.Fatalf("expected 30, got %v", v.Payload())
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
		t.Fatalf("expected 55, got %v (tag %d)", v.Payload(), v.Tt)
	}
}

// --- Test: Length operator ---

func TestLengthOperator(t *testing.T) {
	r := runLua(t, `return #"hello"`)
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 5 {
		t.Fatalf("expected 5, got %v (tag %d)", v.Payload(), v.Tt)
	}
}

// --- Test: Logical operators ---

func TestLogicalAnd(t *testing.T) {
	r := runLua(t, "return 1 and 2")
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 2 {
		t.Fatalf("expected 2, got %v (tag %d)", v.Payload(), v.Tt)
	}
}

func TestLogicalOr(t *testing.T) {
	r := runLua(t, "return false or 42")
	v := r.get(0)
	if !v.IsInteger() || v.Integer() != 42 {
		t.Fatalf("expected 42, got %v (tag %d)", v.Payload(), v.Tt)
	}
}

// --- Test: C function call ---

func TestCFunction(t *testing.T) {
	L := state.NewState()
	// Register a simple C function in globals
	gt := GetGlobalTable(L)
	addFn := func(L2 *state.LuaState) int {
		a := L2.Stack[L2.CI.Func+1].Val.Integer()
		b := L2.Stack[L2.CI.Func+2].Val.Integer()
		state.PushValue(L2, object.MakeInteger(a+b))
		return 1
	}
	gt.Set(object.MakeString(&object.LuaString{Data: "myadd", IsShort: true}),
		object.TValue{Tt: object.TagLightCFunc, Obj: state.CFunction(addFn)})

	reader := newStringReader("return myadd(10, 20)")
	proto := parse.Parse("test", reader)
	cl := closure.NewLClosure(proto, len(proto.Upvalues))
	if len(cl.UpVals) > 0 {
		uv := &closure.UpVal{}
		uv.Close(object.TValue{Tt: object.TagTable, Obj: gt})
		cl.UpVals[0] = uv
	}
	funcIdx := L.Top
	state.PushValue(L, object.TValue{Tt: object.TagLuaClosure, Obj: cl})
	Call(L, funcIdx, state.MultiRet)
	v := L.Stack[funcIdx].Val
	if !v.IsInteger() || v.Integer() != 30 {
		t.Fatalf("expected 30, got %v (tag %d)", v.Payload(), v.Tt)
	}
}

// Ensure unused imports don't cause build errors
var _ = table.New
var _ = fmt.Sprintf
