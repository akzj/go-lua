package api

import (
	"testing"
)

// TestClosureCaptureParentLocal tests that nested functions can capture parent locals.
func TestClosureCaptureParentLocal(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Test: nested function captures parent local
	err := L.LoadString(`
local x = 10
local function foo()
	return x
end
return foo()
`, "test")
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	err = L.Call(0, 1)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	result, _ := L.ToNumber(-1)
	if result != 10.0 {
		t.Errorf("Expected 10.0, got %v", result)
	}
}

// TestRecursiveLocalFunction tests that local function can call itself recursively.
func TestRecursiveLocalFunction(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Test: local function fib can call itself
	err := L.LoadString(`
local function fib(n)
	if n <= 1 then
		return n
	end
	return fib(n-1) + fib(n-2)
end
return fib(6)
`, "test")
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	err = L.Call(0, 1)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	result, _ := L.ToNumber(-1)
	if result != 8.0 {
		t.Errorf("Expected 8.0 (fib(6)), got %v", result)
	}
}

// TestDeeplyNestedClosure tests closures capturing variables through multiple levels.
func TestDeeplyNestedClosure(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Test: inner function captures both 'a' from outer scope and 'b' from immediate parent
	err := L.LoadString(`
local a = 1
local function outer()
	local b = 2
	local function inner()
		return a + b
	end
	return inner()
end
return outer()
`, "test")
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	err = L.Call(0, 1)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	result, _ := L.ToNumber(-1)
	if result != 3.0 {
		t.Errorf("Expected 3.0, got %v", result)
	}
}

// TestClosureWriteToUpvalue tests that closures can write to captured variables.
func TestClosureWriteToUpvalue(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.LoadString(`
local counter = 0
local function inc()
	counter = counter + 1
	return counter
end
local a = inc()
local b = inc()
local c = inc()
return a, b, c
`, "test")
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	err = L.Call(0, 3)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	a, _ := L.ToNumber(-3)
	b, _ := L.ToNumber(-2)
	c, _ := L.ToNumber(-1)

	if a != 1.0 || b != 2.0 || c != 3.0 {
		t.Errorf("Expected 1, 2, 3 got %v, %v, %v", a, b, c)
	}
}

// TestClosureReturningClosure tests a closure that returns another closure.
func TestClosureReturningClosure(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.LoadString(`
local function makeCounter()
	local count = 0
	return function()
		count = count + 1
		return count
	end
end
local c1 = makeCounter()
local c2 = makeCounter()
return c1(), c1(), c2()
`, "test")
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	err = L.Call(0, 3)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	r1, _ := L.ToNumber(-3)
	r2, _ := L.ToNumber(-2)
	r3, _ := L.ToNumber(-1)

	// c1() = 1, c1() = 2, c2() = 1 (different closure instance)
	if r1 != 1.0 || r2 != 2.0 || r3 != 1.0 {
		t.Errorf("Expected 1, 2, 1 got %v, %v, %v", r1, r2, r3)
	}
}
