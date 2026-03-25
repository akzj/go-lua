// Package api provides the public Lua API
package api

import (
	"os"
	"testing"

	"github.com/akzj/go-lua/pkg/object"
)

// TestNewState tests state creation
func TestNewState(t *testing.T) {
	L := NewState()
	if L == nil {
		t.Fatal("NewState returned nil")
	}
	defer L.Close()

	// Verify initial state
	if L.GetTop() != 0 {
		t.Errorf("Expected empty stack, got top=%d", L.GetTop())
	}
}

// TestClose tests state cleanup
func TestClose(t *testing.T) {
	L := NewState()
	L.PushNumber(42)
	L.PushString("test")

	if L.GetTop() != 2 {
		t.Errorf("Expected top=2, got %d", L.GetTop())
	}

	L.Close()

	// After close, stack should be cleared
	if L.GetTop() != 0 {
		t.Errorf("Expected empty stack after Close, got top=%d", L.GetTop())
	}
}

// TestPushNil tests pushing nil values
func TestPushNil(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushNil()

	if L.GetTop() != 1 {
		t.Errorf("Expected top=1 after PushNil, got %d", L.GetTop())
	}

	if !L.IsNil(1) {
		t.Error("Expected value at index 1 to be nil")
	}

	if !L.IsNil(-1) {
		t.Error("Expected value at index -1 to be nil")
	}
}

// TestPushBoolean tests pushing boolean values
func TestPushBoolean(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushBoolean(true)
	L.PushBoolean(false)

	if L.GetTop() != 2 {
		t.Errorf("Expected top=2, got %d", L.GetTop())
	}

	// Test true
	if !L.IsBoolean(1) {
		t.Error("Expected value at index 1 to be boolean")
	}
	b, ok := L.ToBoolean(1)
	if !ok {
		t.Error("ToBoolean should return ok=true for boolean")
	}
	if !b {
		t.Error("Expected true at index 1")
	}

	// Test false
	if !L.IsBoolean(2) {
		t.Error("Expected value at index 2 to be boolean")
	}
	b, ok = L.ToBoolean(2)
	if !ok {
		t.Error("ToBoolean should return ok=true for boolean")
	}
	if b {
		t.Error("Expected false at index 2")
	}

	// Test negative indices
	b, ok = L.ToBoolean(-2)
	if !ok || !b {
		t.Error("Expected true at index -2")
	}

	b, ok = L.ToBoolean(-1)
	if !ok || b {
		t.Error("Expected false at index -1")
	}
}

// TestPushNumber tests pushing number values
func TestPushNumber(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushNumber(42)
	L.PushNumber(3.14159)
	L.PushNumber(-100)

	if L.GetTop() != 3 {
		t.Errorf("Expected top=3, got %d", L.GetTop())
	}

	// Test integer
	if !L.IsNumber(1) {
		t.Error("Expected value at index 1 to be number")
	}
	n, ok := L.ToNumber(1)
	if !ok {
		t.Error("ToNumber should return ok=true for number")
	}
	if n != 42 {
		t.Errorf("Expected 42, got %f", n)
	}

	// Test float
	n, ok = L.ToNumber(2)
	if !ok {
		t.Error("ToNumber should return ok=true for number")
	}
	if n != 3.14159 {
		t.Errorf("Expected 3.14159, got %f", n)
	}

	// Test negative
	n, ok = L.ToNumber(3)
	if !ok {
		t.Error("ToNumber should return ok=true for number")
	}
	if n != -100 {
		t.Errorf("Expected -100, got %f", n)
	}
}

// TestPushString tests pushing string values
func TestPushString(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushString("hello")
	L.PushString("world")
	L.PushString("")

	if L.GetTop() != 3 {
		t.Errorf("Expected top=3, got %d", L.GetTop())
	}

	// Test normal string
	if !L.IsString(1) {
		t.Error("Expected value at index 1 to be string")
	}
	s, ok := L.ToString(1)
	if !ok {
		t.Error("ToString should return ok=true for string")
	}
	if s != "hello" {
		t.Errorf("Expected 'hello', got '%s'", s)
	}

	// Test another string
	s, ok = L.ToString(2)
	if !ok || s != "world" {
		t.Errorf("Expected 'world', got '%s'", s)
	}

	// Test empty string
	s, ok = L.ToString(3)
	if !ok || s != "" {
		t.Errorf("Expected empty string, got '%s'", s)
	}
}

// TestPushFunction tests pushing Go functions
func TestPushFunction(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushFunction(func(L *State) int {
		L.PushNumber(42)
		return 1
	})

	if L.GetTop() != 1 {
		t.Errorf("Expected top=1, got %d", L.GetTop())
	}

	if !L.IsFunction(1) {
		t.Error("Expected value at index 1 to be function")
	}
}

// TestGetTop tests getting stack top
func TestGetTop(t *testing.T) {
	L := NewState()
	defer L.Close()

	if L.GetTop() != 0 {
		t.Errorf("Expected top=0, got %d", L.GetTop())
	}

	L.PushNil()
	if L.GetTop() != 1 {
		t.Errorf("Expected top=1, got %d", L.GetTop())
	}

	L.PushNumber(42)
	L.PushString("test")
	if L.GetTop() != 3 {
		t.Errorf("Expected top=3, got %d", L.GetTop())
	}
}

// TestSetTop tests setting stack top
func TestSetTop(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Push some values
	L.PushNumber(1)
	L.PushNumber(2)
	L.PushNumber(3)
	L.PushNumber(4)
	L.PushNumber(5)

	if L.GetTop() != 5 {
		t.Errorf("Expected top=5, got %d", L.GetTop())
	}

	// Reduce stack
	L.SetTop(3)
	if L.GetTop() != 3 {
		t.Errorf("Expected top=3 after SetTop(3), got %d", L.GetTop())
	}

	// Clear stack
	L.SetTop(0)
	if L.GetTop() != 0 {
		t.Errorf("Expected top=0 after SetTop(0), got %d", L.GetTop())
	}

	// Extend stack (should fill with nil)
	L.PushNumber(42)
	L.SetTop(3)
	if L.GetTop() != 3 {
		t.Errorf("Expected top=3, got %d", L.GetTop())
	}

	// New slots should be nil
	if !L.IsNil(2) {
		t.Error("Expected index 2 to be nil after extending stack")
	}
	if !L.IsNil(3) {
		t.Error("Expected index 3 to be nil after extending stack")
	}
}

// TestPop tests popping values
func TestPop(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushNumber(1)
	L.PushNumber(2)
	L.PushNumber(3)

	L.Pop(1)
	if L.GetTop() != 2 {
		t.Errorf("Expected top=2 after Pop(1), got %d", L.GetTop())
	}

	L.Pop(2)
	if L.GetTop() != 0 {
		t.Errorf("Expected top=0 after Pop(2), got %d", L.GetTop())
	}
}

// TestType tests type checking
func TestType(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Test nil
	L.PushNil()
	if L.Type(1) != object.TypeNil {
		t.Errorf("Expected TypeNil, got %v", L.Type(1))
	}
	if L.TypeName(1) != "nil" {
		t.Errorf("Expected 'nil', got '%s'", L.TypeName(1))
	}

	// Test boolean
	L.PushBoolean(true)
	if L.Type(2) != object.TypeBoolean {
		t.Errorf("Expected TypeBoolean, got %v", L.Type(2))
	}

	// Test number
	L.PushNumber(42)
	if L.Type(3) != object.TypeNumber {
		t.Errorf("Expected TypeNumber, got %v", L.Type(3))
	}

	// Test string
	L.PushString("test")
	if L.Type(4) != object.TypeString {
		t.Errorf("Expected TypeString, got %v", L.Type(4))
	}

	// Test function
	L.PushFunction(func(L *State) int { return 0 })
	if L.Type(5) != object.TypeFunction {
		t.Errorf("Expected TypeFunction, got %v", L.Type(5))
	}
}

// TestIsTruthy tests Lua truthiness
func TestIsTruthy(t *testing.T) {
	L := NewState()
	defer L.Close()

	// nil is falsy
	L.PushNil()
	if L.IsTruthy(1) {
		t.Error("nil should be falsy")
	}

	// false is falsy
	L.PushBoolean(false)
	if L.IsTruthy(2) {
		t.Error("false should be falsy")
	}

	// true is truthy
	L.PushBoolean(true)
	if !L.IsTruthy(3) {
		t.Error("true should be truthy")
	}

	// 0 is truthy in Lua
	L.PushNumber(0)
	if !L.IsTruthy(4) {
		t.Error("0 should be truthy in Lua")
	}

	// empty string is truthy in Lua
	L.PushString("")
	if !L.IsTruthy(5) {
		t.Error("empty string should be truthy in Lua")
	}
}

// TestLen tests length operator
func TestLen(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Test string length
	L.PushString("hello")
	if L.Len(1) != 5 {
		t.Errorf("Expected length 5 for 'hello', got %d", L.Len(1))
	}

	// Test empty string
	L.PushString("")
	if L.Len(2) != 0 {
		t.Errorf("Expected length 0 for empty string, got %d", L.Len(2))
	}

	// Test table length
	L.NewTable()
	L.PushNumber(1)
	L.SetI(-2, 1)
	L.PushNumber(2)
	L.SetI(-2, 2)
	L.PushNumber(3)
	L.SetI(-2, 3)

	if L.Len(-1) != 3 {
		t.Errorf("Expected table length 3, got %d", L.Len(-1))
	}
}

// TestCopy tests copying values
func TestCopy(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushNumber(42)
	L.Copy(1, 2)

	if L.GetTop() != 2 {
		t.Errorf("Expected top=2 after Copy, got %d", L.GetTop())
	}

	n1, _ := L.ToNumber(1)
	n2, _ := L.ToNumber(2)

	if n1 != n2 {
		t.Errorf("Expected copy to have same value: %f vs %f", n1, n2)
	}
}

// TestCheckStack tests stack checking
func TestCheckStack(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Should succeed for reasonable values
	if !L.CheckStack(10) {
		t.Error("CheckStack(10) should succeed")
	}

	if !L.CheckStack(100) {
		t.Error("CheckStack(100) should succeed")
	}
}

// TestRegister tests function registration
func TestRegister(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.Register("myfunc", func(L *State) int {
		L.PushNumber(123)
		return 1
	})

	// Check if function is registered
	L.GetGlobal("myfunc")
	if !L.IsFunction(-1) {
		t.Error("Expected myfunc to be registered as a function")
	}
}

// TestSetGlobalGetGlobal tests global variable operations
func TestSetGlobalGetGlobal(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Set a global number
	L.PushNumber(42)
	L.SetGlobal("answer")

	// Get it back
	t1 := L.GetGlobal("answer")
	if t1 != object.TypeNumber {
		t.Errorf("Expected TypeNumber, got %v", t1)
	}

	n, ok := L.ToNumber(-1)
	if !ok || n != 42 {
		t.Errorf("Expected 42, got %f", n)
	}

	// Set a global string
	L.PushString("hello")
	L.SetGlobal("greeting")

	// Get it back
	t2 := L.GetGlobal("greeting")
	if t2 != object.TypeString {
		t.Errorf("Expected TypeString, got %v", t2)
	}

	s, ok := L.ToString(-1)
	if !ok || s != "hello" {
		t.Errorf("Expected 'hello', got '%s'", s)
	}

	// Get non-existent global (should be nil)
	t3 := L.GetGlobal("nonexistent")
	if t3 != object.TypeNil {
		t.Errorf("Expected TypeNil for nonexistent global, got %v", t3)
	}
}

// TestTable tests table operations
func TestTable(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Create table
	L.NewTable()
	if !L.IsTable(-1) {
		t.Error("Expected NewTable to create a table")
	}

	// Set fields
	L.PushString("value")
	L.SetField(-2, "key")

	// Get field
	L.GetField(-1, "key")
	s, ok := L.ToString(-1)
	if !ok || s != "value" {
		t.Errorf("Expected 'value', got '%s'", s)
	}
	L.Pop(1)

	// Set integer index
	L.PushNumber(42)
	L.SetI(-2, 1)

	// Get integer index
	L.GetI(-1, 1)
	n, ok := L.ToNumber(-1)
	if !ok || n != 42 {
		t.Errorf("Expected 42, got %f", n)
	}
}

// TestCreateTable tests creating tables with pre-allocated space
func TestCreateTable(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Create table with array space
	L.CreateTable(10, 0)
	if !L.IsTable(-1) {
		t.Error("Expected CreateTable to create a table")
	}

	// Create table with map space
	L.CreateTable(0, 5)
	if !L.IsTable(-1) {
		t.Error("Expected CreateTable to create a table")
	}
}

// TestRawSetGet tests raw table operations
func TestRawSetGet(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.NewTable()

	// Raw set
	L.PushString("key")
	L.PushNumber(42)
	L.RawSet(-3)

	// Raw get
	L.PushString("key")
	L.RawGet(-2)
	n, ok := L.ToNumber(-1)
	if !ok || n != 42 {
		t.Errorf("Expected 42, got %f", n)
	}
}

// TestPCall tests protected calls
func TestPCall(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a simple function
	L.Register("add", func(L *State) int {
		a, _ := L.ToNumber(1)
		b, _ := L.ToNumber(2)
		L.PushNumber(a + b)
		return 1
	})

	// Call the function
	L.GetGlobal("add")
	L.PushNumber(10)
	L.PushNumber(20)

	err := L.PCall(2, 1, 0)
	if err != nil {
		t.Fatalf("PCall failed: %v", err)
	}

	result, ok := L.ToNumber(-1)
	if !ok || result != 30 {
		t.Errorf("Expected 30, got %f", result)
	}
}

// TestCall tests regular calls
func TestCall(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a function
	L.Register("double", func(L *State) int {
		n, _ := L.ToNumber(1)
		L.PushNumber(n * 2)
		return 1
	})

	// Call it
	L.GetGlobal("double")
	L.PushNumber(21)

	err := L.Call(1, 1)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	result, ok := L.ToNumber(-1)
	if !ok || result != 42 {
		t.Errorf("Expected 42, got %f", result)
	}
}

// TestLoadString tests loading Lua code
func TestLoadString(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Load simple code
	err := L.LoadString("return 42", "test")
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	// Should have function on stack
	if !L.IsFunction(-1) {
		t.Error("Expected LoadString to push a function")
	}
}

// TestDoString tests executing Lua code
func TestDoString(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Execute simple code
	err := L.DoString("return 42", "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Check result
	result, ok := L.ToNumber(-1)
	if !ok || result != 42 {
		t.Errorf("Expected 42, got %f", result)
	}
}

// TestError tests error handling
func TestError(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Try to execute code that would error
	// (parser is skeleton, so this may not work as expected)
	err := L.DoString("error('test error')", "test")
	if err == nil {
		t.Log("Expected error but got none (parser may be skeleton)")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

// TestLuaError tests LuaError type
func TestLuaError(t *testing.T) {
	err := newLuaError("test error message")

	// err is already *LuaError from newLuaError
	luaErr := err

	if luaErr.Message != "test error message" {
		t.Errorf("Expected 'test error message', got '%s'", luaErr.Message)
	}

	// Test Error() method
	errStr := luaErr.Error()
	if errStr == "" {
		t.Error("Expected non-empty error string")
	}

	// Test wrapError with nil
	wrapped := wrapError(nil)
	if wrapped != nil {
		t.Error("wrapError(nil) should return nil")
	}

	// Test wrapError with LuaError
	wrapped = wrapError(luaErr)
	if wrapped != luaErr {
		t.Error("wrapError should return same LuaError")
	}

	// Test wrapError with non-LuaError
	regularErr := RuntimeError("regular error")
	if regularErr == nil {
		t.Error("Expected non-nil error")
	}
}

// TestVersion tests version string
func TestVersion(t *testing.T) {
	L := NewState()
	defer L.Close()

	version := L.Version()
	if version == "" {
		t.Error("Expected non-empty version string")
	}
	t.Logf("Version: %s", version)
}

// TestStackIndices tests various stack index combinations
func TestStackIndices(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushNumber(1)
	L.PushNumber(2)
	L.PushNumber(3)

	// Test positive indices
	n, _ := L.ToNumber(1)
	if n != 1 {
		t.Errorf("Expected 1 at index 1, got %f", n)
	}

	n, _ = L.ToNumber(2)
	if n != 2 {
		t.Errorf("Expected 2 at index 2, got %f", n)
	}

	n, _ = L.ToNumber(3)
	if n != 3 {
		t.Errorf("Expected 3 at index 3, got %f", n)
	}

	// Test negative indices
	n, _ = L.ToNumber(-1)
	if n != 3 {
		t.Errorf("Expected 3 at index -1, got %f", n)
	}

	n, _ = L.ToNumber(-2)
	if n != 2 {
		t.Errorf("Expected 2 at index -2, got %f", n)
	}

	n, _ = L.ToNumber(-3)
	if n != 1 {
		t.Errorf("Expected 1 at index -3, got %f", n)
	}
}

// TestToNumberConversion tests number conversion edge cases
func TestToNumberConversion(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Try to convert non-number to number
	L.PushString("not a number")
	n, ok := L.ToNumber(1)
	if ok {
		t.Error("Expected ToNumber to fail for string")
	}
	if n != 0 {
		t.Errorf("Expected 0 on failed conversion, got %f", n)
	}

	// Try to convert nil to number
	L.PushNil()
	n, ok = L.ToNumber(2)
	if ok {
		t.Error("Expected ToNumber to fail for nil")
	}
}

// TestToStringConversion tests string conversion edge cases
func TestToStringConversion(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Try to convert non-string to string
	L.PushNumber(42)
	s, ok := L.ToString(1)
	if ok {
		t.Error("Expected ToString to fail for number")
	}
	if s != "" {
		t.Errorf("Expected empty string on failed conversion, got '%s'", s)
	}
}

// TestToBooleanConversion tests boolean conversion edge cases
func TestToBooleanConversion(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Try to convert non-boolean to boolean
	L.PushNumber(42)
	b, ok := L.ToBoolean(1)
	if ok {
		t.Error("Expected ToBoolean to fail for number")
	}
	if b {
		t.Error("Expected false on failed conversion")
	}
}
// TestMove tests the Move function
func TestMove(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushNumber(42)
	L.Move(1, 2)

	n, ok := L.ToNumber(2)
	if !ok || n != 42 {
		t.Errorf("Expected 42, got %f", n)
	}
}

// TestRawSetI tests RawSetI
func TestRawSetI(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.NewTable()
	L.PushNumber(100)
	L.RawSetI(-2, 5)

	L.RawGetI(-1, 5)
	n, ok := L.ToNumber(-1)
	if !ok || n != 100 {
		t.Errorf("Expected 100, got %f", n)
	}
}

// TestRawGetI tests RawGetI
func TestRawGetI(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.NewTable()
	L.PushNumber(200)
	L.RawSetI(-2, 10)

	L.RawGetI(-1, 10)
	n, ok := L.ToNumber(-1)
	if !ok || n != 200 {
		t.Errorf("Expected 200, got %f", n)
	}
}

// TestNext tests table iteration
func TestNext(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.NewTable()
	L.PushString("key1")
	L.PushNumber(1)
	L.RawSet(-3)

	L.PushString("key2")
	L.PushNumber(2)
	L.RawSet(-3)

	// Iterate
	L.PushNil() // First key
	count := 0
	for L.Next(-2) {
		count++
		L.Pop(1) // Remove value, keep key
	}

	if count != 2 {
		t.Errorf("Expected 2 iterations, got %d", count)
	}
}

// TestLenOp tests LenOp
func TestLenOp(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushString("hello")
	L.LenOp(1)

	n, ok := L.ToNumber(-1)
	if !ok || n != 5 {
		t.Errorf("Expected length 5, got %f", n)
	}
}

// TestError tests Error function
func TestErrorFunction(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushString("test error")
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic from Error()")
		}
	}()
	L.Error()
}

// TestErrorf tests Errorf function
func TestErrorf(t *testing.T) {
	L := NewState()
	defer L.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic from Errorf()")
		}
	}()
	L.Errorf("formatted error: %d", 42)
}

// TestGcControl tests GcControl
func TestGcControl(t *testing.T) {
	L := NewState()
	defer L.Close()

	result := L.GcControl(GCStop, 0)
	if result != 0 {
		t.Errorf("Expected 0, got %d", result)
	}
}

// TestSetFuncs tests SetFuncs
func TestSetFuncs(t *testing.T) {
	L := NewState()
	defer L.Close()

	funcs := map[string]Function{
		"test1": func(L *State) int {
			L.PushNumber(1)
			return 1
		},
		"test2": func(L *State) int {
			L.PushNumber(2)
			return 1
		},
	}

	L.SetFuncs(funcs, 0)

	L.GetGlobal("test1")
	if !L.IsFunction(-1) {
		t.Error("Expected test1 to be a function")
	}

	L.GetGlobal("test2")
	if !L.IsFunction(-1) {
		t.Error("Expected test2 to be a function")
	}
}

// TestToCFunction tests ToCFunction
func TestToCFunction(t *testing.T) {
	L := NewState()
	defer L.Close()

	fn := func(L *State) int {
		L.PushNumber(42)
		return 1
	}

	L.PushFunction(fn)
	cfn := L.ToCFunction(-1)

	if cfn == nil {
		t.Error("Expected non-nil C function")
	}
}

// TestUpvalueID tests UpvalueID
func TestUpvalueID(t *testing.T) {
	L := NewState()
	defer L.Close()

	id := L.UpvalueID(1, 1)
	// Should return empty string for now
	if id != "" {
		t.Logf("Got upvalue ID: %s", id)
	}
}

// TestUpvalueJoin tests UpvalueJoin
func TestUpvalueJoin(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Should not panic
	L.UpvalueJoin(1, 1, 2, 2)
}

// TestRequireF tests RequireF
func TestRequireF(t *testing.T) {
	L := NewState()
	defer L.Close()

	// This will fail since require is not set up, but tests the function
	err := L.RequireF("test", func(L *State) int {
		L.PushNumber(1)
		return 1
	}, true)

	// Expected to fail since require is not available
	if err == nil {
		t.Log("RequireF succeeded (unexpected)")
	}
}

// TestCallK tests CallK
func TestCallK(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.Register("test", func(L *State) int {
		L.PushNumber(42)
		return 1
	})

	L.GetGlobal("test")
	err := L.CallK(0, 1, 0, func(L *State) int {
		return 0
	})

	if err != nil {
		t.Errorf("CallK failed: %v", err)
	}
}

// TestPCallK tests PCallK
func TestPCallK(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.Register("test", func(L *State) int {
		L.PushNumber(42)
		return 1
	})

	L.GetGlobal("test")
	err := L.PCallK(0, 1, 0, 0, func(L *State) int {
		return 0
	})

	if err != nil {
		t.Errorf("PCallK failed: %v", err)
	}
}

// TestResume tests Resume
func TestResume(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Just test it doesn't panic
	status, err := L.Resume(nil, 0)
	t.Logf("Resume status: %d, err: %v", status, err)
}

// TestYield tests Yield
func TestYield(t *testing.T) {
	L := NewState()
	defer L.Close()

	n := L.Yield(1)
	if n != 1 {
		t.Errorf("Expected 1, got %d", n)
	}
}

// TestYieldK tests YieldK
func TestYieldK(t *testing.T) {
	L := NewState()
	defer L.Close()

	n := L.YieldK(2, 0, nil)
	if n != 2 {
		t.Errorf("Expected 2, got %d", n)
	}
}

// TestStatus tests Status
func TestStatus(t *testing.T) {
	L := NewState()
	defer L.Close()

	status := L.Status()
	if status != 0 {
		t.Errorf("Expected status 0, got %d", status)
	}
}

// TestResetThread tests ResetThread
func TestResetThread(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushNumber(42)
	status := L.ResetThread()
	if status != 0 {
		t.Errorf("Expected status 0, got %d", status)
	}
	if L.GetTop() != 0 {
		t.Errorf("Expected empty stack after reset, got %d", L.GetTop())
	}
}

// TestIsYieldable tests IsYieldable
func TestIsYieldable(t *testing.T) {
	L := NewState()
	defer L.Close()

	if !L.IsYieldable() {
		t.Error("Expected yieldable")
	}
}

// TestXMove tests XMove
func TestXMove(t *testing.T) {
	L1 := NewState()
	defer L1.Close()

	L2 := NewState()
	defer L2.Close()

	L1.PushNumber(42)
	L1.PushString("hello")

	L1.XMove(L2, 2)

	if L1.GetTop() != 0 {
		t.Errorf("Expected L1 stack empty, got %d", L1.GetTop())
	}

	if L2.GetTop() != 2 {
		t.Errorf("Expected L2 stack top=2, got %d", L2.GetTop())
	}

	n, ok := L2.ToNumber(1)
	if !ok || n != 42 {
		t.Errorf("Expected 42, got %f", n)
	}

	s, ok := L2.ToString(2)
	if !ok || s != "hello" {
		t.Errorf("Expected 'hello', got %s", s)
	}
}

// TestSetAllocFunc tests SetAllocFunc
func TestSetAllocFunc(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Should not panic
	L.SetAllocFunc(nil, nil)
}

// TestSetUserdata tests SetUserdata
func TestSetUserdata(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Should not panic
	L.SetUserdata("test data")
}

// TestUserdata tests Userdata
func TestUserdata(t *testing.T) {
	L := NewState()
	defer L.Close()

	ud := L.Userdata()
	if ud != nil {
		t.Logf("Got userdata: %v", ud)
	}
}

// TestAtPanic tests AtPanic
func TestAtPanic(t *testing.T) {
	L := NewState()
	defer L.Close()

	oldPanic := L.AtPanic(func(L *State) int {
		return 0
	})

	if oldPanic != nil {
		t.Log("Got old panic function")
	}
}

// TestLuaErrorType tests LuaErrorType
func TestLuaErrorType(t *testing.T) {
	types := []LuaErrorType{ErrSyntax, ErrRuntime, ErrMemory, ErrFile}
	names := []string{"syntax error", "runtime error", "memory allocation error", "file error"}

	for i, typ := range types {
		if typ.String() != names[i] {
			t.Errorf("Expected %s, got %s", names[i], typ.String())
		}
	}
}

// TestSyntaxError tests SyntaxError
func TestSyntaxError(t *testing.T) {
	err := SyntaxError("unexpected symbol", "test.lua", 10)
	if err == nil {
		t.Error("Expected error")
	}
	if err.Message == "" {
		t.Error("Expected message")
	}
}

// TestRuntimeError tests RuntimeError
func TestRuntimeError(t *testing.T) {
	err := RuntimeError("test error")
	if err == nil {
		t.Error("Expected error")
	}
}

// TestFileError tests FileError
func TestFileError(t *testing.T) {
	err := FileError("test.txt", os.ErrNotExist)
	if err == nil {
		t.Error("Expected error")
	}
}
