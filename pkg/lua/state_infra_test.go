package lua_test

import (
	"bytes"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

// ---------------------------------------------------------------------------
// UserValue tests
// ---------------------------------------------------------------------------

func TestSetUserValue(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.SetUserValue("greeting", "hello")
	got := L.UserValue("greeting")
	if got != "hello" {
		t.Errorf("expected 'hello', got %v", got)
	}
}

func TestUserValueNil(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	got := L.UserValue("nonexistent")
	if got != nil {
		t.Errorf("expected nil for nonexistent key, got %v", got)
	}
}

func TestDeleteUserValue(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.SetUserValue("key", 42)
	L.DeleteUserValue("key")

	got := L.UserValue("key")
	if got != nil {
		t.Errorf("expected nil after delete, got %v", got)
	}
}

func TestDeleteUserValueNonexistent(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Should not panic on deleting a key that was never set
	L.DeleteUserValue("never_set")
}

func TestUserValueTypes(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Store various Go types
	type myStruct struct {
		Name string
		Age  int
	}

	L.SetUserValue("int", 42)
	L.SetUserValue("string", "hello")
	L.SetUserValue("slice", []int{1, 2, 3})
	L.SetUserValue("struct", myStruct{Name: "test", Age: 30})
	L.SetUserValue("func", func() string { return "ok" })
	L.SetUserValue("nil_value", nil)

	if v := L.UserValue("int"); v != 42 {
		t.Errorf("int: expected 42, got %v", v)
	}
	if v := L.UserValue("string"); v != "hello" {
		t.Errorf("string: expected 'hello', got %v", v)
	}
	if v, ok := L.UserValue("slice").([]int); !ok || len(v) != 3 {
		t.Errorf("slice: expected []int{1,2,3}, got %v", L.UserValue("slice"))
	}
	if v, ok := L.UserValue("struct").(myStruct); !ok || v.Name != "test" {
		t.Errorf("struct: expected myStruct{test,30}, got %v", L.UserValue("struct"))
	}
	if v, ok := L.UserValue("func").(func() string); !ok || v() != "ok" {
		t.Errorf("func: expected func returning 'ok'")
	}
	// nil_value was stored — key exists but value is nil
	if v := L.UserValue("nil_value"); v != nil {
		t.Errorf("nil_value: expected nil, got %v", v)
	}
}

func TestUserValueOverwrite(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.SetUserValue("key", "first")
	L.SetUserValue("key", "second")

	got := L.UserValue("key")
	if got != "second" {
		t.Errorf("expected 'second' after overwrite, got %v", got)
	}
}

func TestUserValueIsolation(t *testing.T) {
	// Two separate states should have independent user values
	L1 := lua.NewState()
	defer L1.Close()
	L2 := lua.NewState()
	defer L2.Close()

	L1.SetUserValue("key", "from_L1")
	L2.SetUserValue("key", "from_L2")

	if v := L1.UserValue("key"); v != "from_L1" {
		t.Errorf("L1: expected 'from_L1', got %v", v)
	}
	if v := L2.UserValue("key"); v != "from_L2" {
		t.Errorf("L2: expected 'from_L2', got %v", v)
	}
}

// ---------------------------------------------------------------------------
// SetWriter tests
// ---------------------------------------------------------------------------

func TestSetWriter(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	var buf bytes.Buffer
	L.SetWriter(&buf)

	err := L.DoString(`print("hello", "world")`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	got := buf.String()
	expected := "hello\tworld\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSetWriterNil(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	var buf bytes.Buffer
	L.SetWriter(&buf)
	L.SetWriter(nil) // revert to default

	// Writer() should return os.Stdout when nil
	w := L.Writer()
	if w == nil {
		t.Error("Writer() should never return nil")
	}
}

func TestWriterDefault(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	w := L.Writer()
	if w == nil {
		t.Error("Writer() should return os.Stdout by default, got nil")
	}
}

func TestSetWriterMultiplePrints(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	var buf bytes.Buffer
	L.SetWriter(&buf)

	err := L.DoString(`
		print("line1")
		print("line2")
		print("a", "b", "c")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	got := buf.String()
	expected := "line1\nline2\na\tb\tc\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSetWriterIsolation(t *testing.T) {
	// Two states with different writers should be independent
	L1 := lua.NewState()
	defer L1.Close()
	L2 := lua.NewState()
	defer L2.Close()

	var buf1, buf2 bytes.Buffer
	L1.SetWriter(&buf1)
	L2.SetWriter(&buf2)

	L1.DoString(`print("from_L1")`)
	L2.DoString(`print("from_L2")`)

	if got := buf1.String(); got != "from_L1\n" {
		t.Errorf("L1 buf: expected 'from_L1\\n', got %q", got)
	}
	if got := buf2.String(); got != "from_L2\n" {
		t.Errorf("L2 buf: expected 'from_L2\\n', got %q", got)
	}
}

func TestSetWriterFromCallback(t *testing.T) {
	// Verify writer works when print() is called from a Go callback
	L := lua.NewState()
	defer L.Close()

	var buf bytes.Buffer
	L.SetWriter(&buf)

	// Register a Go function that calls print via Lua
	L.PushFunction(func(L *lua.State) int {
		// The callback gets a fresh State wrapper, but the Writer
		// is on the shared api.State — so print should still go to buf
		L.DoString(`print("from_callback")`)
		return 0
	})
	L.SetGlobal("go_call_print")

	err := L.DoString(`go_call_print()`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	got := buf.String()
	if got != "from_callback\n" {
		t.Errorf("expected 'from_callback\\n', got %q", got)
	}
}
