package lua_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

// ---------------------------------------------------------------------------
// Wrap0 tests
// ---------------------------------------------------------------------------

func TestWrap0(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	called := false
	lua.Wrap0(L, func() { called = true })
	L.SetGlobal("myfn")

	if err := L.DoString(`myfn()`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	if !called {
		t.Fatal("function was not called")
	}
}

func TestWrap0R(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap0R(L, func() int { return 42 })
	L.SetGlobal("answer")

	if err := L.DoString(`assert(answer() == 42)`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestWrap0R_String(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap0R(L, func() string { return "hello" })
	L.SetGlobal("greet")

	if err := L.DoString(`assert(greet() == "hello")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestWrap0E_Success(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap0E(L, func() error { return nil })
	L.SetGlobal("ok")

	if err := L.DoString(`ok()`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestWrap0E_Error(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap0E(L, func() error { return errors.New("fail") })
	L.SetGlobal("bad")

	err := L.DoString(`bad()`)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "fail") {
		t.Fatalf("expected 'fail' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Wrap1 tests
// ---------------------------------------------------------------------------

func TestWrap1(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	var saved string
	lua.Wrap1(L, func(s string) { saved = s })
	L.SetGlobal("save")

	if err := L.DoString(`save("hello")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	if saved != "hello" {
		t.Fatalf("expected 'hello', got %q", saved)
	}
}

func TestWrap1R(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap1R(L, func(n int64) string { return fmt.Sprintf("%d", n) })
	L.SetGlobal("str")

	if err := L.DoString(`assert(str(99) == "99")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestWrap1E_Success(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap1E(L, func(s string) (string, error) { return "ok:" + s, nil })
	L.SetGlobal("safe")

	if err := L.DoString(`assert(safe("test") == "ok:test")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestWrap1E_Error(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap1E(L, func(s string) (string, error) { return "", errors.New("bad:" + s) })
	L.SetGlobal("fail")

	err := L.DoString(`fail("input")`)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bad:input") {
		t.Fatalf("expected 'bad:input' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Wrap2 tests
// ---------------------------------------------------------------------------

func TestWrap2(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	var saved string
	lua.Wrap2(L, func(a, b string) { saved = a + b })
	L.SetGlobal("cat")

	if err := L.DoString(`cat("foo", "bar")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	if saved != "foobar" {
		t.Fatalf("expected 'foobar', got %q", saved)
	}
}

func TestWrap2R(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap2R(L, func(a, b int64) int64 { return a + b })
	L.SetGlobal("add")

	if err := L.DoString(`assert(add(10, 20) == 30)`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestWrap2E_Success(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap2E(L, func(a, b float64) (float64, error) { return a / b, nil })
	L.SetGlobal("div")

	if err := L.DoString(`assert(div(10.0, 2.0) == 5.0)`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestWrap2E_Error(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap2E(L, func(a, b float64) (float64, error) {
		if b == 0 {
			return 0, errors.New("division by zero")
		}
		return a / b, nil
	})
	L.SetGlobal("div")

	err := L.DoString(`div(10.0, 0.0)`)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected 'division by zero' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Wrap3 tests
// ---------------------------------------------------------------------------

func TestWrap3(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	var saved string
	lua.Wrap3(L, func(a, b, c string) { saved = a + b + c })
	L.SetGlobal("cat3")

	if err := L.DoString(`cat3("a", "b", "c")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	if saved != "abc" {
		t.Fatalf("expected 'abc', got %q", saved)
	}
}

func TestWrap3R(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap3R(L, func(a, b, c string) string { return a + b + c })
	L.SetGlobal("cat3")

	if err := L.DoString(`assert(cat3("x", "y", "z") == "xyz")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestWrap1R_Bool(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap1R(L, func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	})
	L.SetGlobal("yesno")

	if err := L.DoString(`
		assert(yesno(true) == "yes")
		assert(yesno(false) == "no")
	`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestWrap2R_MixedTypes(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap2R(L, func(name string, count int64) string {
		return fmt.Sprintf("%s:%d", name, count)
	})
	L.SetGlobal("label")

	if err := L.DoString(`assert(label("item", 5) == "item:5")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Runnable Examples (godoc)
// ---------------------------------------------------------------------------

func ExampleWrap1R() {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap1R(L, func(n int) int { return n * n })
	L.SetGlobal("square")

	L.DoString(`print(square(7))`)
	// Output:
	// 49
}

func ExampleWrap2R() {
	L := lua.NewState()
	defer L.Close()

	lua.Wrap2R(L, func(a, b int) int { return a + b })
	L.SetGlobal("add")

	L.DoString(`print(add(3, 4))`)
	// Output:
	// 7
}
