package lua_test

import (
	"strings"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

// ---------------------------------------------------------------------------
// 1. Basic: NewState → DoString → Close
// ---------------------------------------------------------------------------

func TestBasicDoString(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		-- basic Lua code
		local x = 40 + 2
		assert(x == 42, "expected 42")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestBasicPrint(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// print is available because NewState loads all stdlib
	err := L.DoString(`print("hello from Lua")`)
	if err != nil {
		t.Fatalf("DoString with print failed: %v", err)
	}
}

func TestBareStateNoStdlib(t *testing.T) {
	L := lua.NewBareState()
	defer L.Close()

	// print should NOT be available in bare state
	err := L.DoString(`print("hello")`)
	if err == nil {
		t.Fatal("expected error from bare state without stdlib")
	}
}

// ---------------------------------------------------------------------------
// 2. Go→Lua function registration
// ---------------------------------------------------------------------------

func TestGoFunctionRegistration(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Register a Go function that adds two numbers
	add := func(L *lua.State) int {
		a := L.CheckInteger(1)
		b := L.CheckInteger(2)
		L.PushInteger(a + b)
		return 1
	}

	L.PushFunction(add)
	L.SetGlobal("add")

	err := L.DoString(`
		local result = add(10, 32)
		assert(result == 42, "expected 42, got " .. tostring(result))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestGoFunctionMultipleReturns(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Register a function that returns multiple values
	divmod := func(L *lua.State) int {
		a := L.CheckInteger(1)
		b := L.CheckInteger(2)
		L.PushInteger(a / b)
		L.PushInteger(a % b)
		return 2
	}

	L.PushFunction(divmod)
	L.SetGlobal("divmod")

	err := L.DoString(`
		local q, r = divmod(17, 5)
		assert(q == 3, "quotient should be 3")
		assert(r == 2, "remainder should be 2")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestGoFunctionWithStrings(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	greet := func(L *lua.State) int {
		name := L.CheckString(1)
		L.PushString("Hello, " + name + "!")
		return 1
	}

	L.PushFunction(greet)
	L.SetGlobal("greet")

	err := L.DoString(`
		local msg = greet("World")
		assert(msg == "Hello, World!", "unexpected: " .. msg)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 3. Table creation and field access from Go side
// ---------------------------------------------------------------------------

func TestTableCreationAndAccess(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Create a table from Go
	L.NewTable()

	// Set fields
	L.PushString("bar")
	L.SetField(-2, "foo")

	L.PushInteger(42)
	L.SetField(-2, "num")

	L.SetGlobal("mytable")

	// Verify from Lua
	err := L.DoString(`
		assert(mytable.foo == "bar", "expected bar")
		assert(mytable.num == 42, "expected 42")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestTableReadFromGo(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Create table from Lua
	err := L.DoString(`
		config = {
			host = "localhost",
			port = 8080,
			debug = true,
		}
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Read from Go
	L.GetGlobal("config")

	L.GetField(-1, "host")
	host, ok := L.ToString(-1)
	if !ok || host != "localhost" {
		t.Fatalf("expected localhost, got %q (ok=%v)", host, ok)
	}
	L.Pop(1)

	L.GetField(-1, "port")
	port, ok := L.ToInteger(-1)
	if !ok || port != 8080 {
		t.Fatalf("expected 8080, got %d (ok=%v)", port, ok)
	}
	L.Pop(1)

	L.GetField(-1, "debug")
	debug := L.ToBoolean(-1)
	if !debug {
		t.Fatal("expected debug=true")
	}
	L.Pop(2) // pop debug value + config table
}

func TestTableArrayAccess(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`arr = {10, 20, 30}`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	L.GetGlobal("arr")
	for i := int64(1); i <= 3; i++ {
		L.GetI(-1, i)
		val, ok := L.ToInteger(-1)
		if !ok || val != i*10 {
			t.Fatalf("arr[%d]: expected %d, got %d", i, i*10, val)
		}
		L.Pop(1)
	}
	L.Pop(1)
}

// ---------------------------------------------------------------------------
// 4. Error handling
// ---------------------------------------------------------------------------

func TestSyntaxError(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`this is not valid lua!!!`)
	if err == nil {
		t.Fatal("expected syntax error")
	}
	if !strings.Contains(err.Error(), "error") {
		t.Fatalf("expected error message, got: %v", err)
	}
}

func TestRuntimeError(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`error("something went wrong")`)
	if err == nil {
		t.Fatal("expected runtime error")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Fatalf("expected 'something went wrong' in error, got: %v", err)
	}
}

func TestPCallProtectedError(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Load a chunk that will error
	status := L.Load(`error("oops")`, "=test", "t")
	if status != lua.OK {
		t.Fatalf("Load failed with status %d", status)
	}

	// PCall should catch the error
	status = L.PCall(0, 0, 0)
	if status == lua.OK {
		t.Fatal("expected PCall to return error status")
	}

	// Error message should be on the stack
	msg, ok := L.ToString(-1)
	if !ok {
		t.Fatal("expected error message on stack")
	}
	if !strings.Contains(msg, "oops") {
		t.Fatalf("expected 'oops' in error, got: %s", msg)
	}
	L.Pop(1)
}

func TestNilGlobalAccess(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local x = nonexistent_function()
	`)
	if err == nil {
		t.Fatal("expected error calling nil value")
	}
}

// ---------------------------------------------------------------------------
// 5. Coroutines
// ---------------------------------------------------------------------------

func TestCoroutineBasic(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Test coroutines from pure Lua
	err := L.DoString(`
		local co = coroutine.create(function()
			coroutine.yield(1)
			coroutine.yield(2)
			return 3
		end)

		local ok, v1 = coroutine.resume(co)
		assert(ok and v1 == 1, "first yield")

		local ok, v2 = coroutine.resume(co)
		assert(ok and v2 == 2, "second yield")

		local ok, v3 = coroutine.resume(co)
		assert(ok and v3 == 3, "return")

		local ok, err = coroutine.resume(co)
		assert(not ok, "dead coroutine")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestCoroutineFromGo(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Load a function that yields
	err := L.DoString(`
		function producer()
			for i = 1, 3 do
				coroutine.yield(i * 10)
			end
			return 999
		end
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Create a thread from Go
	thread := L.NewThread()

	// Push the function onto the thread's stack
	thread.GetGlobal("producer")

	// Resume the thread, collecting yielded values
	expected := []int64{10, 20, 30, 999}
	for i, exp := range expected {
		status, nresults := thread.Resume(L, 0)
		if i < 3 {
			if status != 1 { // Yield
				t.Fatalf("iteration %d: expected yield status, got %d", i, status)
			}
		} else {
			if status != lua.OK {
				t.Fatalf("iteration %d: expected OK status, got %d", i, status)
			}
		}
		if nresults < 1 {
			t.Fatalf("iteration %d: expected at least 1 result, got %d", i, nresults)
		}
		val, ok := thread.ToInteger(-1)
		if !ok || val != exp {
			t.Fatalf("iteration %d: expected %d, got %d (ok=%v)", i, exp, val, ok)
		}
		thread.Pop(nresults)
	}

	L.Pop(1) // pop the thread from L's stack
}

// ---------------------------------------------------------------------------
// 6. Additional coverage: type checking, stack manipulation
// ---------------------------------------------------------------------------

func TestTypeChecking(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushNil()
	if L.Type(-1) != lua.TypeNil {
		t.Error("expected TypeNil")
	}
	if L.TypeName(lua.TypeNil) != "nil" {
		t.Error("expected 'nil'")
	}
	L.Pop(1)

	L.PushBoolean(true)
	if !L.IsBoolean(-1) {
		t.Error("expected boolean")
	}
	L.Pop(1)

	L.PushInteger(42)
	if !L.IsInteger(-1) {
		t.Error("expected integer")
	}
	if !L.IsNumber(-1) {
		t.Error("expected number")
	}
	L.Pop(1)

	L.PushNumber(3.14)
	if !L.IsNumber(-1) {
		t.Error("expected number")
	}
	L.Pop(1)

	L.PushString("hello")
	if !L.IsString(-1) {
		t.Error("expected string")
	}
	if L.Type(-1) != lua.TypeString {
		t.Error("expected TypeString")
	}
	L.Pop(1)

	L.NewTable()
	if !L.IsTable(-1) {
		t.Error("expected table")
	}
	L.Pop(1)

	L.PushFunction(func(L *lua.State) int { return 0 })
	if !L.IsFunction(-1) {
		t.Error("expected function")
	}
	L.Pop(1)
}

func TestStackManipulation(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)

	if L.GetTop() != 3 {
		t.Fatalf("expected top=3, got %d", L.GetTop())
	}

	// Copy idx 1 to top
	L.PushValue(1)
	val, _ := L.ToInteger(-1)
	if val != 1 {
		t.Fatalf("expected 1, got %d", val)
	}
	L.Pop(1)

	// Remove middle element
	L.Remove(2) // removes '2', stack is now [1, 3]
	if L.GetTop() != 2 {
		t.Fatalf("expected top=2 after Remove, got %d", L.GetTop())
	}
	val, _ = L.ToInteger(2)
	if val != 3 {
		t.Fatalf("expected 3 at idx 2, got %d", val)
	}

	L.SetTop(0) // clear stack
	if L.GetTop() != 0 {
		t.Fatalf("expected empty stack, got %d", L.GetTop())
	}
}

func TestUserdata(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Create userdata and store a Go value in it
	L.NewUserdata(0, 1)
	L.SetUserdataValue(-1, "my-data")

	val := L.UserdataValue(-1)
	if val != "my-data" {
		t.Fatalf("expected 'my-data', got %v", val)
	}

	// Test user values
	L.PushString("uv1")
	ok := L.SetIUserValue(-2, 1)
	if !ok {
		t.Fatal("SetIUserValue failed")
	}

	tp := L.GetIUserValue(-1, 1)
	if tp != lua.TypeString {
		t.Fatalf("expected TypeString, got %v", tp)
	}
	s, _ := L.ToString(-1)
	if s != "uv1" {
		t.Fatalf("expected 'uv1', got %q", s)
	}
	L.Pop(2) // pop user value + userdata
}

func TestMetatable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local mt = {
			__tostring = function(self)
				return "Point(" .. self.x .. ", " .. self.y .. ")"
			end,
			__add = function(a, b)
				return {x = a.x + b.x, y = a.y + b.y, mt = getmetatable(a)}
			end,
		}
		mt.__index = mt

		function Point(x, y)
			return setmetatable({x=x, y=y}, mt)
		end

		local p = Point(1, 2)
		assert(tostring(p) == "Point(1, 2)")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestClosureUpvalues(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Test Go closure with upvalues
	L.PushInteger(100) // upvalue
	counter := func(L *lua.State) int {
		// Get upvalue (the counter)
		val, _ := L.ToInteger(lua.UpvalueIndex(1))
		val++
		L.PushInteger(val)
		L.Copy(-1, lua.UpvalueIndex(1)) // update upvalue
		return 1
	}
	L.PushClosure(counter, 1)
	L.SetGlobal("counter")

	err := L.DoString(`
		assert(counter() == 101)
		assert(counter() == 102)
		assert(counter() == 103)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestRefUnref(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Create a reference to a value in the registry
	L.PushString("stored-value")
	ref := L.Ref(lua.RegistryIndex)
	if ref == lua.RefNil || ref == lua.NoRef {
		t.Fatal("expected valid reference")
	}

	// Retrieve it
	L.RawGetI(lua.RegistryIndex, int64(ref))
	s, ok := L.ToString(-1)
	if !ok || s != "stored-value" {
		t.Fatalf("expected 'stored-value', got %q", s)
	}
	L.Pop(1)

	// Unref
	L.Unref(lua.RegistryIndex, ref)
}

func TestConstants(t *testing.T) {
	// Verify constants have expected values
	if lua.OK != 0 {
		t.Errorf("OK should be 0, got %d", lua.OK)
	}
	if lua.MultiRet != -1 {
		t.Errorf("MultiRet should be -1, got %d", lua.MultiRet)
	}
	if lua.RefNil != -1 {
		t.Errorf("RefNil should be -1, got %d", lua.RefNil)
	}
	if lua.NoRef != -2 {
		t.Errorf("NoRef should be -2, got %d", lua.NoRef)
	}
}
