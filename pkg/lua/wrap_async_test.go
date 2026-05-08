package lua_test

import (
	"testing"
	"time"

	"github.com/akzj/go-lua/pkg/lua"
)

// syncAdd is a simple synchronous Go function: adds two numbers.
func syncAdd(L *lua.State) int {
	a, _ := L.ToNumber(1)
	b, _ := L.ToNumber(2)
	L.PushNumber(a + b)
	return 1
}

// syncError returns nil + error string (standard error convention).
func syncError(L *lua.State) int {
	L.PushNil()
	L.PushString("something went wrong")
	return 2
}

// syncPanic panics inside the function.
func syncPanic(L *lua.State) int {
	panic("intentional panic")
}

// syncTableArg reads a table argument and returns a field from it.
func syncTableArg(L *lua.State) int {
	// Expect a table at position 1 with field "name"
	L.GetField(1, "name")
	name, _ := L.ToString(-1)
	L.Pop(1)
	L.PushString("hello " + name)
	return 1
}

// syncNoReturn returns nothing.
func syncNoReturn(L *lua.State) int {
	return 0
}

// syncValueAndNil returns (value, nil) — success pattern.
func syncValueAndNil(L *lua.State) int {
	L.PushInteger(42)
	L.PushNil()
	return 2
}

func TestWrapAsync_BasicNumber(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	sched := lua.NewScheduler(L)

	// Register the async function
	L.PushFunction(lua.WrapAsync(syncAdd))
	L.SetGlobal("addAsync")

	// Define coroutine function that calls addAsync and awaits it
	err := L.DoString(`
		local async = require("async")
		function test_fn()
			local future = addAsync(10, 20)
			local result = async.await(future)
			_G.test_result = result
		end
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	L.GetGlobal("test_fn")
	_, err = sched.Spawn(L)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	// Tick until done
	if err := sched.WaitAll(2 * time.Second); err != nil {
		t.Fatalf("WaitAll: %v", err)
	}

	// Check result
	L.GetGlobal("test_result")
	result, _ := L.ToNumber(-1)
	L.Pop(1)

	if result != 30 {
		t.Errorf("expected 30, got %v", result)
	}
}

func TestWrapAsync_ErrorConvention(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	sched := lua.NewScheduler(L)

	L.PushFunction(lua.WrapAsync(syncError))
	L.SetGlobal("errorAsync")

	err := L.DoString(`
		local async = require("async")
		function test_fn()
			local future = errorAsync()
			local val, err = async.await(future)
			_G.test_val = val
			_G.test_err = err
		end
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	L.GetGlobal("test_fn")
	_, err = sched.Spawn(L)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if err := sched.WaitAll(2 * time.Second); err != nil {
		t.Fatalf("WaitAll: %v", err)
	}

	// When Future rejects, async.await returns (nil, errString)
	L.GetGlobal("test_val")
	if !L.IsNil(-1) {
		t.Errorf("expected test_val to be nil, got %v", L.ToAny(-1))
	}
	L.Pop(1)

	L.GetGlobal("test_err")
	errMsg, _ := L.ToString(-1)
	L.Pop(1)
	if errMsg != "something went wrong" {
		t.Errorf("expected 'something went wrong', got %q", errMsg)
	}
}

func TestWrapAsync_Panic(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	sched := lua.NewScheduler(L)

	L.PushFunction(lua.WrapAsync(syncPanic))
	L.SetGlobal("panicAsync")

	err := L.DoString(`
		local async = require("async")
		function test_fn()
			local future = panicAsync()
			local val, err = async.await(future)
			_G.test_val = val
			_G.test_err = err
		end
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	L.GetGlobal("test_fn")
	_, err = sched.Spawn(L)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if err := sched.WaitAll(2 * time.Second); err != nil {
		t.Fatalf("WaitAll: %v", err)
	}

	L.GetGlobal("test_val")
	if !L.IsNil(-1) {
		t.Errorf("expected test_val to be nil")
	}
	L.Pop(1)

	L.GetGlobal("test_err")
	errMsg, _ := L.ToString(-1)
	L.Pop(1)
	if errMsg == "" {
		t.Errorf("expected non-empty error message from panic")
	}
	// Should contain "panic" somewhere
	if !containsSubstring(errMsg, "panic") {
		t.Errorf("expected error to mention 'panic', got %q", errMsg)
	}
}

func TestWrapAsync_TableArg(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	sched := lua.NewScheduler(L)

	L.PushFunction(lua.WrapAsync(syncTableArg))
	L.SetGlobal("greetAsync")

	err := L.DoString(`
		local async = require("async")
		function test_fn()
			local future = greetAsync({name = "world"})
			local result = async.await(future)
			_G.test_result = result
		end
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	L.GetGlobal("test_fn")
	_, err = sched.Spawn(L)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if err := sched.WaitAll(2 * time.Second); err != nil {
		t.Fatalf("WaitAll: %v", err)
	}

	L.GetGlobal("test_result")
	result, _ := L.ToString(-1)
	L.Pop(1)

	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestWrapAsync_NoReturn(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	sched := lua.NewScheduler(L)

	L.PushFunction(lua.WrapAsync(syncNoReturn))
	L.SetGlobal("noReturnAsync")

	err := L.DoString(`
		local async = require("async")
		function test_fn()
			local future = noReturnAsync()
			local result = async.await(future)
			_G.test_result = result
			_G.test_ran = true
		end
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	L.GetGlobal("test_fn")
	_, err = sched.Spawn(L)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if err := sched.WaitAll(2 * time.Second); err != nil {
		t.Fatalf("WaitAll: %v", err)
	}

	L.GetGlobal("test_ran")
	if !L.ToBoolean(-1) {
		t.Errorf("coroutine did not complete")
	}
	L.Pop(1)

	L.GetGlobal("test_result")
	if !L.IsNil(-1) {
		t.Errorf("expected nil result, got %v", L.ToAny(-1))
	}
	L.Pop(1)
}

func TestWrapAsync_ValueAndNil(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	sched := lua.NewScheduler(L)

	L.PushFunction(lua.WrapAsync(syncValueAndNil))
	L.SetGlobal("valueAsync")

	err := L.DoString(`
		local async = require("async")
		function test_fn()
			local future = valueAsync()
			local result = async.await(future)
			_G.test_result = result
		end
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	L.GetGlobal("test_fn")
	_, err = sched.Spawn(L)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if err := sched.WaitAll(2 * time.Second); err != nil {
		t.Fatalf("WaitAll: %v", err)
	}

	L.GetGlobal("test_result")
	result, _ := L.ToInteger(-1)
	L.Pop(1)

	if result != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestWrapAsyncWithContext_Basic(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	sched := lua.NewScheduler(L)

	// A function that checks it has a context
	contextFn := func(L *lua.State) int {
		ctx := L.Context()
		if ctx == nil {
			L.PushNil()
			L.PushString("no context")
			return 2
		}
		L.PushString("has context")
		return 1
	}

	L.PushFunction(lua.WrapAsyncWithContext(contextFn))
	L.SetGlobal("ctxAsync")

	err := L.DoString(`
		local async = require("async")
		function test_fn()
			local future = ctxAsync()
			local result = async.await(future)
			_G.test_result = result
		end
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	L.GetGlobal("test_fn")
	_, err = sched.Spawn(L)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if err := sched.WaitAll(2 * time.Second); err != nil {
		t.Fatalf("WaitAll: %v", err)
	}

	L.GetGlobal("test_result")
	result, _ := L.ToString(-1)
	L.Pop(1)

	if result != "has context" {
		t.Errorf("expected 'has context', got %q", result)
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
