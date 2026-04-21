package api

import (
	"strings"
	"testing"

	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// State creation
// ---------------------------------------------------------------------------

func TestNewState(t *testing.T) {
	L := NewState()
	if L == nil {
		t.Fatal("NewState returned nil")
	}
	if L.Internal == nil {
		t.Fatal("Internal state is nil")
	}
	L.Close()
}

func TestNewStateHasGlobalTable(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushGlobalTable()
	if !L.IsTable(-1) {
		t.Fatalf("global table is not a table, got type %v", L.Type(-1))
	}
	L.Pop(1)
}

// ---------------------------------------------------------------------------
// Stack basics
// ---------------------------------------------------------------------------

func TestGetTopEmpty(t *testing.T) {
	L := NewState()
	defer L.Close()
	if got := L.GetTop(); got != 0 {
		t.Fatalf("GetTop on empty stack = %d, want 0", got)
	}
}

func TestPushAndGetTop(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(42)
	L.PushInteger(43)
	L.PushInteger(44)
	if got := L.GetTop(); got != 3 {
		t.Fatalf("GetTop = %d, want 3", got)
	}
}

func TestSetTop(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)
	L.SetTop(2)
	if got := L.GetTop(); got != 2 {
		t.Fatalf("GetTop after SetTop(2) = %d, want 2", got)
	}
}

func TestSetTopGrows(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(1)
	L.SetTop(5)
	if got := L.GetTop(); got != 5 {
		t.Fatalf("GetTop after SetTop(5) = %d, want 5", got)
	}
	// Slots 2-5 should be nil
	if !L.IsNil(2) {
		t.Fatal("slot 2 should be nil")
	}
}

func TestSetTopNegative(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)
	L.SetTop(-2) // remove top 1 element
	if got := L.GetTop(); got != 2 {
		t.Fatalf("GetTop after SetTop(-2) = %d, want 2", got)
	}
}

func TestPop(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)
	L.Pop(2)
	if got := L.GetTop(); got != 1 {
		t.Fatalf("GetTop after Pop(2) = %d, want 1", got)
	}
}

func TestAbsIndex(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(10)
	L.PushInteger(20)
	L.PushInteger(30)
	if got := L.AbsIndex(-1); got != 3 {
		t.Fatalf("AbsIndex(-1) = %d, want 3", got)
	}
	if got := L.AbsIndex(-3); got != 1 {
		t.Fatalf("AbsIndex(-3) = %d, want 1", got)
	}
	if got := L.AbsIndex(2); got != 2 {
		t.Fatalf("AbsIndex(2) = %d, want 2", got)
	}
}

func TestPushValue(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(42)
	L.PushValue(1) // copy slot 1 to top
	n, ok := L.ToInteger(-1)
	if !ok || n != 42 {
		t.Fatalf("PushValue: got %d/%v, want 42/true", n, ok)
	}
}

func TestCopy(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(10)
	L.PushInteger(20)
	L.Copy(2, 1) // copy slot 2 to slot 1
	n, ok := L.ToInteger(1)
	if !ok || n != 20 {
		t.Fatalf("Copy: slot 1 = %d/%v, want 20/true", n, ok)
	}
}

func TestRotate(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)
	L.Rotate(1, 1) // rotate right by 1: [3,1,2]
	v1, _ := L.ToInteger(1)
	v2, _ := L.ToInteger(2)
	v3, _ := L.ToInteger(3)
	if v1 != 3 || v2 != 1 || v3 != 2 {
		t.Fatalf("Rotate: got [%d,%d,%d], want [3,1,2]", v1, v2, v3)
	}
}

func TestInsert(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3) // stack: [1,2,3]
	L.Insert(2)       // insert top at 2: [1,3,2]
	v1, _ := L.ToInteger(1)
	v2, _ := L.ToInteger(2)
	v3, _ := L.ToInteger(3)
	if v1 != 1 || v2 != 3 || v3 != 2 {
		t.Fatalf("Insert: got [%d,%d,%d], want [1,3,2]", v1, v2, v3)
	}
}

func TestRemove(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)
	L.Remove(2) // remove slot 2: [1,3]
	if got := L.GetTop(); got != 2 {
		t.Fatalf("Remove: top = %d, want 2", got)
	}
	v1, _ := L.ToInteger(1)
	v2, _ := L.ToInteger(2)
	if v1 != 1 || v2 != 3 {
		t.Fatalf("Remove: got [%d,%d], want [1,3]", v1, v2)
	}
}

func TestReplace(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(10)
	L.PushInteger(20)
	L.PushInteger(30)
	L.Replace(1) // replace slot 1 with top (30), pop top
	if got := L.GetTop(); got != 2 {
		t.Fatalf("Replace: top = %d, want 2", got)
	}
	v1, _ := L.ToInteger(1)
	if v1 != 30 {
		t.Fatalf("Replace: slot 1 = %d, want 30", v1)
	}
}

// ---------------------------------------------------------------------------
// Push/Type checking
// ---------------------------------------------------------------------------

func TestPushNil(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushNil()
	if !L.IsNil(-1) {
		t.Fatal("PushNil: not nil")
	}
	if L.Type(-1) != object.TypeNil {
		t.Fatalf("PushNil: type = %v, want nil", L.Type(-1))
	}
}

func TestPushBoolean(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushBoolean(true)
	L.PushBoolean(false)
	if !L.IsBoolean(1) || !L.IsBoolean(2) {
		t.Fatal("PushBoolean: not boolean")
	}
	if !L.ToBoolean(1) {
		t.Fatal("PushBoolean(true): ToBoolean = false")
	}
	if L.ToBoolean(2) {
		t.Fatal("PushBoolean(false): ToBoolean = true")
	}
}

func TestPushInteger(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(42)
	if !L.IsInteger(1) {
		t.Fatal("not integer")
	}
	if !L.IsNumber(1) {
		t.Fatal("integer should also be number")
	}
	n, ok := L.ToInteger(1)
	if !ok || n != 42 {
		t.Fatalf("ToInteger = %d/%v, want 42/true", n, ok)
	}
}

func TestPushNumber(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushNumber(3.14)
	if !L.IsNumber(1) {
		t.Fatal("not number")
	}
	n, ok := L.ToNumber(1)
	if !ok || n != 3.14 {
		t.Fatalf("ToNumber = %f/%v, want 3.14/true", n, ok)
	}
}

func TestPushString(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushString("hello")
	if !L.IsString(1) {
		t.Fatal("not string")
	}
	s, ok := L.ToString(1)
	if !ok || s != "hello" {
		t.Fatalf("ToString = %q/%v, want hello/true", s, ok)
	}
}

func TestPushFString(t *testing.T) {
	L := NewState()
	defer L.Close()
	s := L.PushFString("hello %d", 42)
	if s != "hello 42" {
		t.Fatalf("PushFString returned %q, want %q", s, "hello 42")
	}
	got, ok := L.ToString(-1)
	if !ok || got != "hello 42" {
		t.Fatalf("ToString = %q/%v", got, ok)
	}
}

func TestIsNone(t *testing.T) {
	L := NewState()
	defer L.Close()
	if !L.IsNone(1) {
		t.Fatal("empty stack: slot 1 should be none")
	}
	if !L.IsNoneOrNil(1) {
		t.Fatal("empty stack: slot 1 should be none or nil")
	}
}

func TestIsString_NumberIsString(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(42)
	if !L.IsString(1) {
		t.Fatal("integer should be considered string (auto-coerce)")
	}
}

func TestIsFunction(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushCFunction(func(L *State) int { return 0 })
	if !L.IsFunction(1) {
		t.Fatal("CFunction should be function")
	}
	if !L.IsCFunction(1) {
		t.Fatal("CFunction should be CFunction")
	}
}

// ---------------------------------------------------------------------------
// Conversion
// ---------------------------------------------------------------------------

func TestToBoolean(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushNil()
	L.PushBoolean(false)
	L.PushBoolean(true)
	L.PushInteger(0)
	L.PushString("")

	if L.ToBoolean(1) {
		t.Fatal("nil should be falsy")
	}
	if L.ToBoolean(2) {
		t.Fatal("false should be falsy")
	}
	if !L.ToBoolean(3) {
		t.Fatal("true should be truthy")
	}
	if !L.ToBoolean(4) {
		t.Fatal("0 should be truthy in Lua")
	}
	if !L.ToBoolean(5) {
		t.Fatal("empty string should be truthy in Lua")
	}
}

func TestToIntegerFromFloat(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushNumber(42.0)
	n, ok := L.ToInteger(1)
	if !ok || n != 42 {
		t.Fatalf("ToInteger(42.0) = %d/%v, want 42/true", n, ok)
	}
}

func TestToIntegerFromNonInteger(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushNumber(3.5)
	_, ok := L.ToInteger(1)
	if ok {
		t.Fatal("ToInteger(3.5) should fail")
	}
}

func TestToNumberFromInteger(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(42)
	n, ok := L.ToNumber(1)
	if !ok || n != 42.0 {
		t.Fatalf("ToNumber(int 42) = %f/%v", n, ok)
	}
}

func TestToStringFromInteger(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(42)
	s, ok := L.ToString(1)
	if !ok || s != "42" {
		t.Fatalf("ToString(42) = %q/%v", s, ok)
	}
}

func TestToStringFromNil(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushNil()
	_, ok := L.ToString(1)
	if ok {
		t.Fatal("ToString(nil) should fail")
	}
}

func TestRawLen(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushString("hello")
	if got := L.RawLen(1); got != 5 {
		t.Fatalf("RawLen(string) = %d, want 5", got)
	}
}

// ---------------------------------------------------------------------------
// Table operations
// ---------------------------------------------------------------------------

func TestCreateTableAndSetGet(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.NewTable()
	L.PushInteger(42)
	L.SetField(1, "x") // t.x = 42
	tp := L.GetField(1, "x")
	if tp != object.TypeNumber {
		t.Fatalf("GetField type = %v, want number", tp)
	}
	n, ok := L.ToInteger(-1)
	if !ok || n != 42 {
		t.Fatalf("GetField value = %d/%v, want 42/true", n, ok)
	}
}

func TestSetGetI(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.NewTable()
	L.PushString("hello")
	L.SetI(1, 1) // t[1] = "hello"
	tp := L.GetI(1, 1)
	if tp != object.TypeString {
		t.Fatalf("GetI type = %v, want string", tp)
	}
	s, ok := L.ToString(-1)
	if !ok || s != "hello" {
		t.Fatalf("GetI value = %q/%v", s, ok)
	}
}

func TestRawSetGet(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.NewTable()
	L.PushString("value")
	L.RawSetI(1, 5) // t[5] = "value"
	tp := L.RawGetI(1, 5)
	if tp != object.TypeString {
		t.Fatalf("RawGetI type = %v, want string", tp)
	}
	L.Pop(1)
}

func TestGetMetatable(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.NewTable() // table at 1
	// No metatable initially
	if L.GetMetatable(1) {
		t.Fatal("new table should not have metatable")
	}
	// Set a metatable
	L.NewTable() // metatable at 2
	L.SetMetatable(1)
	if !L.GetMetatable(1) {
		t.Fatal("should have metatable after SetMetatable")
	}
	if !L.IsTable(-1) {
		t.Fatal("metatable should be a table")
	}
}

func TestLen(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushString("hello")
	L.Len(1)
	n, ok := L.ToInteger(-1)
	if !ok || n != 5 {
		t.Fatalf("Len(string) = %d/%v, want 5", n, ok)
	}
}

func TestNext(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.NewTable()
	L.PushInteger(100)
	L.SetField(1, "a")
	L.PushInteger(200)
	L.SetField(1, "b")

	// Iterate
	count := 0
	L.PushNil() // first key
	for L.Next(1) {
		count++
		L.Pop(1) // pop value, keep key for next
	}
	if count != 2 {
		t.Fatalf("Next iterated %d times, want 2", count)
	}
}

// ---------------------------------------------------------------------------
// Globals
// ---------------------------------------------------------------------------

func TestSetGetGlobal(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(99)
	L.SetGlobal("myvar")
	tp := L.GetGlobal("myvar")
	if tp != object.TypeNumber {
		t.Fatalf("GetGlobal type = %v, want number", tp)
	}
	n, ok := L.ToInteger(-1)
	if !ok || n != 99 {
		t.Fatalf("GetGlobal value = %d/%v, want 99/true", n, ok)
	}
}

func TestGetGlobalNonExistent(t *testing.T) {
	L := NewState()
	defer L.Close()
	tp := L.GetGlobal("nonexistent")
	if tp != object.TypeNil {
		t.Fatalf("GetGlobal(nonexistent) type = %v, want nil", tp)
	}
	L.Pop(1)
}

// ---------------------------------------------------------------------------
// TypeName
// ---------------------------------------------------------------------------

func TestTypeName(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		tp   object.Type
		want string
	}{
		{object.TypeNil, "nil"},
		{object.TypeBoolean, "boolean"},
		{object.TypeNumber, "number"},
		{object.TypeString, "string"},
		{object.TypeTable, "table"},
		{object.TypeFunction, "function"},
		{TypeNone, "no value"},
	}
	for _, tt := range tests {
		got := L.TypeName(tt.tp)
		if got != tt.want {
			t.Errorf("TypeName(%v) = %q, want %q", tt.tp, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// CFunction / CClosure
// ---------------------------------------------------------------------------

func TestPushCFunction(t *testing.T) {
	L := NewState()
	defer L.Close()
	called := false
	f := func(L *State) int {
		called = true
		L.PushInteger(42)
		return 1
	}
	L.PushCFunction(f)
	if !L.IsFunction(1) {
		t.Fatal("CFunction should be function")
	}
	L.Call(0, 1)
	if !called {
		t.Fatal("CFunction was not called")
	}
	n, ok := L.ToInteger(-1)
	if !ok || n != 42 {
		t.Fatalf("CFunction result = %d/%v, want 42/true", n, ok)
	}
}

func TestPushCClosure(t *testing.T) {
	L := NewState()
	defer L.Close()
	// Push upvalue
	L.PushInteger(100)
	f := func(L *State) int {
		// Get upvalue 1
		n, _ := L.ToInteger(UpvalueIndex(1))
		L.PushInteger(n + 1)
		return 1
	}
	L.PushCClosure(f, 1)
	L.Call(0, 1)
	n, ok := L.ToInteger(-1)
	if !ok || n != 101 {
		t.Fatalf("CClosure result = %d/%v, want 101/true", n, ok)
	}
}

// ---------------------------------------------------------------------------
// DoString
// ---------------------------------------------------------------------------

func TestDoStringSimple(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString("x = 1 + 2")
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	tp := L.GetGlobal("x")
	if tp != object.TypeNumber {
		t.Fatalf("x type = %v, want number", tp)
	}
	n, ok := L.ToInteger(-1)
	if !ok || n != 3 {
		t.Fatalf("x = %d/%v, want 3/true", n, ok)
	}
}

func TestDoStringWithCFunction(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a Go function
	add := func(L *State) int {
		a := L.CheckInteger(1)
		b := L.CheckInteger(2)
		L.PushInteger(a + b)
		return 1
	}
	L.PushCFunction(add)
	L.SetGlobal("add")

	err := L.DoString("result = add(10, 20)")
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	L.GetGlobal("result")
	n, ok := L.ToInteger(-1)
	if !ok || n != 30 {
		t.Fatalf("result = %d/%v, want 30/true", n, ok)
	}
}

func TestDoStringSyntaxError(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString("if then end end")
	if err == nil {
		t.Fatal("expected syntax error")
	}
}

func TestDoStringRuntimeError(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString("error('boom')")
	if err == nil {
		t.Fatal("expected runtime error")
	}
	// error() is not registered yet, so this will be a different error
}

func TestDoStringMultipleStatements(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString(`
		a = 10
		b = 20
		c = a + b
	`)
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	L.GetGlobal("c")
	n, ok := L.ToInteger(-1)
	if !ok || n != 30 {
		t.Fatalf("c = %d/%v, want 30/true", n, ok)
	}
}

func TestDoStringStringConcat(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString(`s = "hello" .. " " .. "world"`)
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	L.GetGlobal("s")
	s, ok := L.ToString(-1)
	if !ok || s != "hello world" {
		t.Fatalf("s = %q/%v, want 'hello world'/true", s, ok)
	}
}

func TestDoStringLoop(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString(`
		sum = 0
		for i = 1, 10 do
			sum = sum + i
		end
	`)
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	L.GetGlobal("sum")
	n, ok := L.ToInteger(-1)
	if !ok || n != 55 {
		t.Fatalf("sum = %d/%v, want 55/true", n, ok)
	}
}

func TestDoStringTableConstructor(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString(`
		t = {1, 2, 3}
		n = #t
	`)
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	L.GetGlobal("n")
	n, ok := L.ToInteger(-1)
	if !ok || n != 3 {
		t.Fatalf("n = %d/%v, want 3/true", n, ok)
	}
}

func TestDoStringFunction(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString(`
		function double(x)
			return x * 2
		end
		result = double(21)
	`)
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	L.GetGlobal("result")
	n, ok := L.ToInteger(-1)
	if !ok || n != 42 {
		t.Fatalf("result = %d/%v, want 42/true", n, ok)
	}
}

// ---------------------------------------------------------------------------
// PCall
// ---------------------------------------------------------------------------

func TestPCallSuccess(t *testing.T) {
	L := NewState()
	defer L.Close()
	status := L.Load("return 1 + 2", "=test", "t")
	if status != StatusOK {
		t.Fatalf("Load failed: %d", status)
	}
	status = L.PCall(0, 1, 0)
	if status != StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("PCall failed: %d: %s", status, msg)
	}
	n, ok := L.ToInteger(-1)
	if !ok || n != 3 {
		t.Fatalf("result = %d/%v, want 3/true", n, ok)
	}
}

func TestPCallError(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register error function
	errFunc := func(L *State) int {
		msg := L.CheckString(1)
		L.PushString("ERR: " + msg)
		return 1
	}
	L.PushCFunction(errFunc)
	L.SetGlobal("myerror")

	// Push a function that errors
	boom := func(L *State) int {
		L.PushString("boom")
		L.Error()
		return 0
	}
	L.PushCFunction(boom)
	status := L.PCall(0, 0, 0)
	if status == StatusOK {
		t.Fatal("expected error from PCall")
	}
}

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

func TestLoad(t *testing.T) {
	L := NewState()
	defer L.Close()
	status := L.Load("return 42", "=test", "t")
	if status != StatusOK {
		msg, _ := L.ToString(-1)
		t.Fatalf("Load failed: %d: %s", status, msg)
	}
	if !L.IsFunction(-1) {
		t.Fatal("Load should push a function")
	}
}

func TestLoadSyntaxError(t *testing.T) {
	L := NewState()
	defer L.Close()
	status := L.Load("if then", "=test", "t")
	if status == StatusOK {
		t.Fatal("expected syntax error")
	}
	// Error message should be on stack
	msg, ok := L.ToString(-1)
	if !ok {
		t.Fatal("error message should be a string")
	}
	if !strings.Contains(msg, "expected") {
		t.Logf("syntax error message: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// Auxiliary functions
// ---------------------------------------------------------------------------

func TestNewLib(t *testing.T) {
	L := NewState()
	defer L.Close()
	funcs := map[string]CFunction{
		"add": func(L *State) int {
			a := L.CheckInteger(1)
			b := L.CheckInteger(2)
			L.PushInteger(a + b)
			return 1
		},
	}
	L.NewLib(funcs)
	if !L.IsTable(-1) {
		t.Fatal("NewLib should push a table")
	}
	tp := L.GetField(-1, "add")
	if tp != object.TypeFunction {
		t.Fatalf("add field type = %v, want function", tp)
	}
	L.Pop(1) // pop add function
}

func TestSetFuncsAndCall(t *testing.T) {
	L := NewState()
	defer L.Close()

	funcs := map[string]CFunction{
		"double": func(L *State) int {
			n := L.CheckInteger(1)
			L.PushInteger(n * 2)
			return 1
		},
	}
	L.NewLib(funcs)
	L.SetGlobal("mylib")

	err := L.DoString("result = mylib.double(21)")
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	L.GetGlobal("result")
	n, ok := L.ToInteger(-1)
	if !ok || n != 42 {
		t.Fatalf("result = %d/%v, want 42/true", n, ok)
	}
}

// ---------------------------------------------------------------------------
// CheckStack
// ---------------------------------------------------------------------------

func TestCheckStack(t *testing.T) {
	L := NewState()
	defer L.Close()
	if !L.CheckStack(100) {
		t.Fatal("CheckStack(100) should succeed")
	}
}

// ---------------------------------------------------------------------------
// RawEqual
// ---------------------------------------------------------------------------

func TestRawEqual(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(42)
	L.PushInteger(42)
	if !L.RawEqual(1, 2) {
		t.Fatal("42 == 42 should be true")
	}
	L.PushInteger(43)
	if L.RawEqual(1, 3) {
		t.Fatal("42 == 43 should be false")
	}
}

// ---------------------------------------------------------------------------
// UpvalueIndex
// ---------------------------------------------------------------------------

func TestUpvalueIndex(t *testing.T) {
	if UpvalueIndex(1) != RegistryIndex-1 {
		t.Fatalf("UpvalueIndex(1) = %d, want %d", UpvalueIndex(1), RegistryIndex-1)
	}
	if UpvalueIndex(2) != RegistryIndex-2 {
		t.Fatalf("UpvalueIndex(2) = %d, want %d", UpvalueIndex(2), RegistryIndex-2)
	}
}

// ---------------------------------------------------------------------------
// Integration: Go function library registered and called from Lua
// ---------------------------------------------------------------------------

func TestIntegration_GoLibFromLua(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a small math library
	mathLib := map[string]CFunction{
		"add": func(L *State) int {
			a := L.CheckNumber(1)
			b := L.CheckNumber(2)
			L.PushNumber(a + b)
			return 1
		},
		"mul": func(L *State) int {
			a := L.CheckNumber(1)
			b := L.CheckNumber(2)
			L.PushNumber(a * b)
			return 1
		},
	}
	L.NewLib(mathLib)
	L.SetGlobal("mymath")

	err := L.DoString(`
		x = mymath.add(10, 20)
		y = mymath.mul(3, 7)
		z = mymath.add(x, y)
	`)
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}

	L.GetGlobal("z")
	n, ok := L.ToNumber(-1)
	if !ok || n != 51.0 {
		t.Fatalf("z = %f/%v, want 51.0/true", n, ok)
	}
}

func TestIntegration_LuaCallsGoCallback(t *testing.T) {
	L := NewState()
	defer L.Close()

	var collected []int64
	collector := func(L *State) int {
		n := L.CheckInteger(1)
		collected = append(collected, n)
		return 0
	}
	L.PushCFunction(collector)
	L.SetGlobal("collect")

	err := L.DoString(`
		for i = 1, 5 do
			collect(i)
		end
	`)
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	if len(collected) != 5 {
		t.Fatalf("collected %d items, want 5", len(collected))
	}
	for i, v := range collected {
		if v != int64(i+1) {
			t.Fatalf("collected[%d] = %d, want %d", i, v, i+1)
		}
	}
}

func TestIntegration_RecursiveLua(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString(`
		function fib(n)
			if n <= 1 then return n end
			return fib(n-1) + fib(n-2)
		end
		result = fib(10)
	`)
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	L.GetGlobal("result")
	n, ok := L.ToInteger(-1)
	if !ok || n != 55 {
		t.Fatalf("fib(10) = %d/%v, want 55/true", n, ok)
	}
}

func TestIntegration_ClosureFromLua(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString(`
		function counter()
			local n = 0
			return function()
				n = n + 1
				return n
			end
		end
		c = counter()
		a = c()
		b = c()
		d = c()
	`)
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	L.GetGlobal("d")
	n, ok := L.ToInteger(-1)
	if !ok || n != 3 {
		t.Fatalf("d = %d/%v, want 3/true", n, ok)
	}
}
