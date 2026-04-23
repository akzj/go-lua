package lua_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

// ---------------------------------------------------------------------------
// Runnable Examples (godoc + go test -run Example)
// ---------------------------------------------------------------------------

func ExampleNewState() {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`print("hello from Lua")`)
	if err != nil {
		fmt.Println("error:", err)
	}
	// Output:
	// hello from Lua
}

func ExampleNewBareState() {
	L := lua.NewBareState()
	defer L.Close()

	// Bare state has no standard libraries loaded.
	// Attempting to call print will fail:
	err := L.DoString(`print("hello")`)
	if err != nil {
		fmt.Println("error: standard library not available")
	}
	// Output:
	// error: standard library not available
}

func ExampleState_PushFunction() {
	L := lua.NewState()
	defer L.Close()

	// Register a Go function that adds two integers.
	add := func(L *lua.State) int {
		a := L.CheckInteger(1)
		b := L.CheckInteger(2)
		L.PushInteger(a + b)
		return 1 // one return value
	}

	L.PushFunction(add)
	L.SetGlobal("add")

	err := L.DoString(`print(add(40, 2))`)
	if err != nil {
		fmt.Println("error:", err)
	}
	// Output:
	// 42
}

func ExampleState_PushClosure() {
	L := lua.NewState()
	defer L.Close()

	// Create a counter closure with an upvalue.
	L.PushInteger(0) // initial counter value (upvalue 1)

	counter := func(L *lua.State) int {
		val, _ := L.ToInteger(lua.UpvalueIndex(1))
		val++
		L.PushInteger(val)
		L.Copy(-1, lua.UpvalueIndex(1)) // update the upvalue
		return 1
	}

	L.PushClosure(counter, 1) // 1 upvalue
	L.SetGlobal("counter")

	L.DoString(`
		print(counter())
		print(counter())
		print(counter())
	`)
	// Output:
	// 1
	// 2
	// 3
}

func ExampleState_NewTable() {
	L := lua.NewState()
	defer L.Close()

	// Create a table from Go and pass it to Lua.
	L.NewTable()

	L.PushString("localhost")
	L.SetField(-2, "host")

	L.PushInteger(8080)
	L.SetField(-2, "port")

	L.SetGlobal("config")

	L.DoString(`print(config.host .. ":" .. config.port)`)
	// Output:
	// localhost:8080
}

func ExampleState_GetField() {
	L := lua.NewState()
	defer L.Close()

	// Create a table in Lua and read it from Go.
	L.DoString(`settings = { width = 1920, height = 1080 }`)

	L.GetGlobal("settings")

	L.GetField(-1, "width")
	w, _ := L.ToInteger(-1)
	L.Pop(1)

	L.GetField(-1, "height")
	h, _ := L.ToInteger(-1)
	L.Pop(2) // pop height + settings table

	fmt.Printf("%dx%d\n", w, h)
	// Output:
	// 1920x1080
}

func ExampleState_DoString() {
	L := lua.NewState()
	defer L.Close()

	// DoString returns a Go error on failure.
	err := L.DoString(`error("something went wrong")`)
	if err != nil {
		fmt.Println("caught error:", err != nil)
	}
	// Output:
	// caught error: true
}

func ExampleState_PCall() {
	L := lua.NewState()
	defer L.Close()

	// Load a chunk without executing it.
	status := L.Load(`return 6 * 7`, "=example", "t")
	if status != lua.OK {
		fmt.Println("load error")
		return
	}

	// Call it in protected mode.
	status = L.PCall(0, 1, 0)
	if status != lua.OK {
		msg, _ := L.ToString(-1)
		fmt.Println("error:", msg)
		L.Pop(1)
		return
	}

	result, _ := L.ToInteger(-1)
	L.Pop(1)
	fmt.Println(result)
	// Output:
	// 42
}

func ExampleState_NewThread() {
	L := lua.NewState()
	defer L.Close()

	// Define a generator function in Lua.
	L.DoString(`
		function squares(n)
			for i = 1, n do
				coroutine.yield(i * i)
			end
		end
	`)

	// Create a coroutine thread from Go.
	thread := L.NewThread()
	thread.GetGlobal("squares")
	thread.PushInteger(4) // n = 4

	// Drive the coroutine from Go, collecting yielded values.
	for {
		status, nresults := thread.Resume(L, 1)
		if status == lua.OK {
			break // coroutine finished
		}
		if status != lua.Yield {
			msg, _ := thread.ToString(-1)
			fmt.Println("error:", msg)
			break
		}
		val, _ := thread.ToInteger(-1)
		fmt.Println(val)
		thread.Pop(nresults)
	}
	L.Pop(1) // pop the thread

	// Output:
	// 1
	// 4
	// 9
	// 16
}

func ExampleState_SetFuncs() {
	L := lua.NewState()
	defer L.Close()

	// Create a module table with multiple Go functions.
	L.NewTable()
	L.SetFuncs(map[string]lua.Function{
		"upper": func(L *lua.State) int {
			s := L.CheckString(1)
			L.PushString(strings.ToUpper(s))
			return 1
		},
		"repeat": func(L *lua.State) int {
			s := L.CheckString(1)
			n := L.CheckInteger(2)
			L.PushString(strings.Repeat(s, int(n)))
			return 1
		},
	}, 0)
	L.SetGlobal("mystr")

	L.DoString(`
		print(mystr.upper("hello"))
		print(mystr["repeat"]("ab", 3))
	`)
	// Output:
	// HELLO
	// ababab
}

func ExampleState_Ref() {
	L := lua.NewState()
	defer L.Close()

	// Store a Lua value in the registry and retrieve it later.
	L.PushString("stored-value")
	ref := L.Ref(lua.RegistryIndex)

	// ... later, retrieve it by reference:
	L.RawGetI(lua.RegistryIndex, int64(ref))
	s, _ := L.ToString(-1)
	fmt.Println(s)
	L.Pop(1)

	// Free the reference when no longer needed.
	L.Unref(lua.RegistryIndex, ref)
	// Output:
	// stored-value
}

func ExampleState_NewUserdata() {
	L := lua.NewState()
	defer L.Close()

	// Create a userdata wrapping a Go value.
	L.NewUserdata(0, 0)
	L.SetUserdataValue(-1, map[string]int{"x": 10, "y": 20})

	// Read it back.
	val := L.UserdataValue(-1)
	m := val.(map[string]int)
	fmt.Printf("x=%d y=%d\n", m["x"], m["y"])
	L.Pop(1)
	// Output:
	// x=10 y=20
}


func ExampleState_Resume() {
	L := lua.NewState()
	defer L.Close()

	// Lua generator that yields values.
	L.DoString(`
		function generate()
			coroutine.yield("hello")
			coroutine.yield("world")
			return "done"
		end
	`)

	thread := L.NewThread()
	thread.GetGlobal("generate")

	for {
		status, nresults := thread.Resume(L, 0)
		if nresults > 0 {
			val, _ := thread.ToString(-1)
			fmt.Println(val)
			thread.Pop(nresults)
		}
		if status == lua.OK {
			break
		}
	}
	L.Pop(1) // pop the thread
	// Output:
	// hello
	// world
	// done
}

func ExampleState_Yield() {
	L := lua.NewState()
	defer L.Close()

	// Register a Go function that yields back to the Go host.
	askUser := func(L *lua.State) int {
		// The argument (prompt string) is already on the stack.
		return L.Yield(1) // yield 1 value (the prompt)
	}
	L.PushFunction(askUser)
	L.SetGlobal("ask_user")

	L.DoString(`
		function chat()
			local name = ask_user("What is your name?")
			return "Hello, " .. name .. "!"
		end
	`)

	thread := L.NewThread()
	thread.GetGlobal("chat")

	// First resume: starts the coroutine, which yields at ask_user.
	status, _ := thread.Resume(L, 0)
	if status == lua.Yield {
		prompt, _ := thread.ToString(-1)
		fmt.Println("Prompt:", prompt)
		thread.Pop(1)

		// Resume with the "user's answer".
		thread.PushString("Alice")
		status, _ = thread.Resume(L, 1)
	}
	if status == lua.OK {
		result, _ := thread.ToString(-1)
		fmt.Println("Result:", result)
	}
	L.Pop(1) // pop the thread
	// Output:
	// Prompt: What is your name?
	// Result: Hello, Alice!
}

func ExampleState_SetHook() {
	L := lua.NewState()
	defer L.Close()

	var lines []int
	L.SetHook(func(L *lua.State, event int, currentLine int) {
		if event == lua.HookEventLine {
			lines = append(lines, currentLine)
		}
	}, lua.MaskLine, 0)

	L.DoString(`
		local x = 1
		local y = 2
		local z = x + y
	`)

	fmt.Println("Lines executed:", len(lines))
	// Output:
	// Lines executed: 3
}

func ExampleState_DoFile() {
	L := lua.NewState()
	defer L.Close()

	// DoFile loads and executes a Lua file.
	// Returns a Go error on failure.
	err := L.DoFile("nonexistent.lua")
	if err != nil {
		fmt.Println("Error loading file (expected)")
	}
	// Output:
	// Error loading file (expected)
}

func ExampleNewBareState_sandbox() {
	L := lua.NewBareState()
	defer L.Close()

	// Register only safe functions — no io, os, or debug.
	L.PushFunction(func(L *lua.State) int {
		s := L.CheckString(1)
		fmt.Println(s)
		return 0
	})
	L.SetGlobal("safe_print")

	// This works: safe_print is explicitly registered.
	L.DoString(`safe_print("sandboxed hello")`)

	// io/os/debug are NOT available in a bare state.
	err := L.DoString(`io.open("secret.txt")`)
	if err != nil {
		fmt.Println("io blocked (expected)")
	}
	// Output:
	// sandboxed hello
	// io blocked (expected)
}

func ExampleState_NewMetatable() {
	L := lua.NewState()
	defer L.Close()

	// Create a "Point" metatable with a __tostring metamethod.
	L.NewMetatable("Point")
	L.PushFunction(func(L *lua.State) int {
		p := L.UserdataValue(1).([]int64)
		L.PushString(fmt.Sprintf("(%d, %d)", p[0], p[1]))
		return 1
	})
	L.SetField(-2, "__tostring")
	L.Pop(1) // pop the metatable

	// Create a userdata and attach the metatable.
	L.NewUserdata(0, 0)
	L.SetUserdataValue(-1, []int64{3, 4})
	L.GetField(lua.RegistryIndex, "Point") // retrieve the metatable
	L.SetMetatable(-2)
	L.SetGlobal("pt")

	L.DoString(`print(tostring(pt))`)
	// Output:
	// (3, 4)
}

// ---------------------------------------------------------------------------
// Test functions (more thorough coverage)
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

	err := L.DoString(`print("hello from Lua")`)
	if err != nil {
		t.Fatalf("DoString with print failed: %v", err)
	}
}

func TestBareStateNoStdlib(t *testing.T) {
	L := lua.NewBareState()
	defer L.Close()

	err := L.DoString(`print("hello")`)
	if err == nil {
		t.Fatal("expected error from bare state without stdlib")
	}
}

func TestGoFunctionRegistration(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

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

func TestTableCreationAndAccess(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.NewTable()
	L.PushString("bar")
	L.SetField(-2, "foo")
	L.PushInteger(42)
	L.SetField(-2, "num")
	L.SetGlobal("mytable")

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
	L.Pop(2)
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

func TestSyntaxError(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`this is not valid lua!!!`)
	if err == nil {
		t.Fatal("expected syntax error")
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

	status := L.Load(`error("oops")`, "=test", "t")
	if status != lua.OK {
		t.Fatalf("Load failed with status %d", status)
	}

	status = L.PCall(0, 0, 0)
	if status == lua.OK {
		t.Fatal("expected PCall to return error status")
	}

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

	err := L.DoString(`local x = nonexistent_function()`)
	if err == nil {
		t.Fatal("expected error calling nil value")
	}
}

func TestCoroutineBasic(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

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

	thread := L.NewThread()
	thread.GetGlobal("producer")

	expected := []int64{10, 20, 30, 999}
	for i, exp := range expected {
		status, nresults := thread.Resume(L, 0)
		if i < 3 {
			if status != 1 {
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

	L.Pop(1)
}

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

	L.PushValue(1)
	val, _ := L.ToInteger(-1)
	if val != 1 {
		t.Fatalf("expected 1, got %d", val)
	}
	L.Pop(1)

	L.Remove(2)
	if L.GetTop() != 2 {
		t.Fatalf("expected top=2 after Remove, got %d", L.GetTop())
	}
	val, _ = L.ToInteger(2)
	if val != 3 {
		t.Fatalf("expected 3 at idx 2, got %d", val)
	}

	L.SetTop(0)
	if L.GetTop() != 0 {
		t.Fatalf("expected empty stack, got %d", L.GetTop())
	}
}

func TestUserdata(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.NewUserdata(0, 1)
	L.SetUserdataValue(-1, "my-data")

	val := L.UserdataValue(-1)
	if val != "my-data" {
		t.Fatalf("expected 'my-data', got %v", val)
	}

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
	L.Pop(2)
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

	L.PushInteger(100)
	counter := func(L *lua.State) int {
		val, _ := L.ToInteger(lua.UpvalueIndex(1))
		val++
		L.PushInteger(val)
		L.Copy(-1, lua.UpvalueIndex(1))
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

	L.PushString("stored-value")
	ref := L.Ref(lua.RegistryIndex)
	if ref == lua.RefNil || ref == lua.NoRef {
		t.Fatal("expected valid reference")
	}

	L.RawGetI(lua.RegistryIndex, int64(ref))
	s, ok := L.ToString(-1)
	if !ok || s != "stored-value" {
		t.Fatalf("expected 'stored-value', got %q", s)
	}
	L.Pop(1)

	L.Unref(lua.RegistryIndex, ref)
}

func TestConstants(t *testing.T) {
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

// ---------------------------------------------------------------------------
// SetHook / GetHook / LoadFile tests
// ---------------------------------------------------------------------------

func TestSetHookLine(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	var lines []int
	L.SetHook(func(L *lua.State, event int, line int) {
		if event == lua.HookEventLine {
			lines = append(lines, line)
		}
	}, lua.MaskLine, 0)
	if err := L.DoString("local x = 1\nlocal y = 2\nlocal z = x + y\n"); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	if len(lines) == 0 {
		t.Error("expected line hooks to fire, got none")
	}
}

func TestSetHookCount(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	countHits := 0
	L.SetHook(func(L *lua.State, event int, line int) {
		if event == lua.HookEventCount {
			countHits++
		}
	}, lua.MaskCount, 10)
	if err := L.DoString("for i = 1, 100 do end"); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	if countHits == 0 {
		t.Error("expected count hooks to fire, got none")
	}
}

func TestSetHookCall(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	callHits := 0
	L.SetHook(func(L *lua.State, event int, line int) {
		if event == lua.HookEventCall {
			callHits++
		}
	}, lua.MaskCall, 0)
	if err := L.DoString("local function f() end\nf()\nf()\n"); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	if callHits < 2 {
		t.Errorf("expected at least 2 call hooks, got %d", callHits)
	}
}

func TestSetHookClear(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	hits := 0
	L.SetHook(func(L *lua.State, event int, line int) {
		hits++
	}, lua.MaskLine|lua.MaskCall, 0)
	_ = L.DoString("local x = 1")
	before := hits
	L.SetHook(nil, 0, 0) // clear hook
	_ = L.DoString("local y = 2")
	if hits != before {
		t.Errorf("hooks should not fire after SetHook(nil), got %d more hits", hits-before)
	}
}

func TestGetHook(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	f, mask, count := L.GetHook()
	if f != nil || mask != 0 || count != 0 {
		t.Error("expected nil hook initially")
	}
	myHook := func(L *lua.State, event int, line int) {}
	L.SetHook(myHook, lua.MaskLine|lua.MaskCall, 42)
	f, mask, count = L.GetHook()
	if f == nil {
		t.Error("expected non-nil hook after SetHook")
	}
	if mask != lua.MaskLine|lua.MaskCall {
		t.Errorf("expected mask %d, got %d", lua.MaskLine|lua.MaskCall, mask)
	}
	if count != 42 {
		t.Errorf("expected count 42, got %d", count)
	}
}

func TestLoadFile(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	tmpFile := t.TempDir() + "/test.lua"
	if err := os.WriteFile(tmpFile, []byte("return 42\n"), 0644); err != nil {
		t.Fatal(err)
	}
	status := L.LoadFile(tmpFile, "t")
	if status != lua.OK {
		msg, _ := L.ToString(-1)
		t.Fatalf("LoadFile failed: %s", msg)
	}
	callStatus := L.PCall(0, 1, 0)
	if callStatus != lua.OK {
		msg, _ := L.ToString(-1)
		t.Fatalf("PCall failed: %s", msg)
	}
	val, ok := L.ToInteger(-1)
	if !ok || val != 42 {
		t.Errorf("expected 42, got %d (ok=%v)", val, ok)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	status := L.LoadFile("/nonexistent/path.lua", "t")
	if status != lua.ErrFile {
		t.Errorf("expected ErrFile (%d), got %d", lua.ErrFile, status)
	}
	msg, _ := L.ToString(-1)
	if msg == "" {
		t.Error("expected error message on stack")
	}
}
