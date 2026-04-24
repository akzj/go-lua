package lua_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

// ---------------------------------------------------------------------------
// PushGoFunc tests
// ---------------------------------------------------------------------------

func TestPushGoFunc_NoArgs_NoReturn(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	called := false
	L.PushGoFunc(func() { called = true })
	L.SetGlobal("myfn")

	if err := L.DoString(`myfn()`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	if !called {
		t.Fatal("function was not called")
	}
}

func TestPushGoFunc_StringArg_StringReturn(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushGoFunc(func(s string) string { return "hi " + s })
	L.SetGlobal("greet")

	if err := L.DoString(`assert(greet("world") == "hi world")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushGoFunc_MultipleArgs(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushGoFunc(func(a string, b int, c float64) string {
		return fmt.Sprintf("%s:%d:%.1f", a, b, c)
	})
	L.SetGlobal("combine")

	if err := L.DoString(`assert(combine("x", 5, 3.5) == "x:5:3.5")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushGoFunc_ErrorReturn_Nil(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushGoFunc(func(s string) (string, error) { return "ok:" + s, nil })
	L.SetGlobal("safe")

	if err := L.DoString(`assert(safe("test") == "ok:test")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushGoFunc_ErrorReturn_NonNil(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushGoFunc(func() (string, error) { return "", errors.New("boom") })
	L.SetGlobal("fail")

	err := L.DoString(`fail()`)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected 'boom' in error, got: %v", err)
	}
}

func TestPushGoFunc_MapArg(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushGoFunc(func(m map[string]any) int { return len(m) })
	L.SetGlobal("maplen")

	if err := L.DoString(`assert(maplen({a=1, b=2, c=3}) == 3)`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushGoFunc_BoolReturn(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushGoFunc(func(s string) bool { return s == "yes" })
	L.SetGlobal("check")

	if err := L.DoString(`
		assert(check("yes") == true)
		assert(check("no") == false)
	`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushGoFunc_OptionalArgs(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushGoFunc(func(a, b string) string {
		if b == "" {
			return a + ":default"
		}
		return a + ":" + b
	})
	L.SetGlobal("opt")

	if err := L.DoString(`assert(opt("hello") == "hello:default")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushGoFunc_Variadic(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushGoFunc(func(prefix string, rest ...any) string {
		parts := []string{prefix}
		for _, v := range rest {
			parts = append(parts, fmt.Sprintf("%v", v))
		}
		return strings.Join(parts, "-")
	})
	L.SetGlobal("join")

	if err := L.DoString(`assert(join("a", "b", "c") == "a-b-c")`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushGoFunc_MultiReturn(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushGoFunc(func() (string, int64, bool) { return "hello", 42, true })
	L.SetGlobal("multi")

	if err := L.DoString(`
		local a, b, c = multi()
		assert(a == "hello", "a=" .. tostring(a))
		assert(b == 42, "b=" .. tostring(b))
		assert(c == true, "c=" .. tostring(c))
	`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushGoFunc_NotAFunc_Panics(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for non-function argument")
		}
	}()

	L.PushGoFunc(42)
}

func TestPushGoFunc_IntArgs(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushGoFunc(func(a int, b int) int { return a + b })
	L.SetGlobal("add")

	if err := L.DoString(`assert(add(10, 20) == 30)`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushGoFunc_Float64Return(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushGoFunc(func(a, b float64) float64 { return a * b })
	L.SetGlobal("mul")

	if err := L.DoString(`
		local r = mul(2.5, 4.0)
		assert(r == 10.0, "got " .. tostring(r))
	`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}
