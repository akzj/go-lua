package lua_test

import (
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

// helper: create a state with a table at top containing test fields.
// Fields: name="hello", age=42, score=3.14, active=true
func newTestTable(t *testing.T) *lua.State {
	t.Helper()
	L := lua.NewState()
	if err := L.DoString(`
		testTable = {
			name   = "hello",
			age    = 42,
			score  = 3.14,
			active = true,
			flag   = false,
			nested = { x = 1, y = 2 },
			list   = { 10, 20, 30 },
		}
	`); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("testTable")
	return L
}

// ---------------------------------------------------------------------------
// GetFieldString
// ---------------------------------------------------------------------------

func TestGetFieldString(t *testing.T) {
	L := newTestTable(t)
	defer L.Close()

	// Normal string field.
	if got := L.GetFieldString(-1, "name"); got != "hello" {
		t.Fatalf("expected \"hello\", got %q", got)
	}

	// Missing key → "".
	if got := L.GetFieldString(-1, "nonexistent"); got != "" {
		t.Fatalf("expected \"\", got %q", got)
	}

	// Number field → coerced to string.
	if got := L.GetFieldString(-1, "age"); got == "" {
		t.Fatalf("expected number coerced to string, got empty")
	}

	// Stack should be unchanged (table still at -1).
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------------
// GetFieldInt
// ---------------------------------------------------------------------------

func TestGetFieldInt(t *testing.T) {
	L := newTestTable(t)
	defer L.Close()

	// Normal integer field.
	if got := L.GetFieldInt(-1, "age"); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}

	// Missing key → 0.
	if got := L.GetFieldInt(-1, "nonexistent"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}

	// Float field — score=3.14 has no exact int representation → 0.
	// (ToInteger returns 0, false for non-integer floats.)
	got := L.GetFieldInt(-1, "score")
	_ = got // just ensure it doesn't panic

	// Stack unchanged.
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------------
// GetFieldNumber
// ---------------------------------------------------------------------------

func TestGetFieldNumber(t *testing.T) {
	L := newTestTable(t)
	defer L.Close()

	// Float field.
	if got := L.GetFieldNumber(-1, "score"); got != 3.14 {
		t.Fatalf("expected 3.14, got %f", got)
	}

	// Integer field auto-converts.
	if got := L.GetFieldNumber(-1, "age"); got != 42.0 {
		t.Fatalf("expected 42.0, got %f", got)
	}

	// Missing key → 0.
	if got := L.GetFieldNumber(-1, "nonexistent"); got != 0 {
		t.Fatalf("expected 0, got %f", got)
	}

	// Stack unchanged.
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------------
// GetFieldBool
// ---------------------------------------------------------------------------

func TestGetFieldBool(t *testing.T) {
	L := newTestTable(t)
	defer L.Close()

	// True field.
	if got := L.GetFieldBool(-1, "active"); !got {
		t.Fatal("expected true")
	}

	// Explicit false field.
	if got := L.GetFieldBool(-1, "flag"); got {
		t.Fatal("expected false for 'flag'")
	}

	// Missing key → false (nil is falsy).
	if got := L.GetFieldBool(-1, "nonexistent"); got {
		t.Fatal("expected false for missing key")
	}

	// Stack unchanged.
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------------
// GetFieldAny
// ---------------------------------------------------------------------------

func TestGetFieldAny(t *testing.T) {
	L := newTestTable(t)
	defer L.Close()

	// String.
	if got, ok := L.GetFieldAny(-1, "name").(string); !ok || got != "hello" {
		t.Fatalf("expected string \"hello\", got %T(%v)", L.GetFieldAny(-1, "name"), L.GetFieldAny(-1, "name"))
	}

	// Integer (Lua integers come back as int64).
	if got, ok := L.GetFieldAny(-1, "age").(int64); !ok || got != 42 {
		t.Fatalf("expected int64(42), got %T(%v)", L.GetFieldAny(-1, "age"), L.GetFieldAny(-1, "age"))
	}

	// Boolean.
	if got, ok := L.GetFieldAny(-1, "active").(bool); !ok || !got {
		t.Fatalf("expected bool(true), got %T(%v)", L.GetFieldAny(-1, "active"), L.GetFieldAny(-1, "active"))
	}

	// Nil.
	if got := L.GetFieldAny(-1, "nonexistent"); got != nil {
		t.Fatalf("expected nil, got %T(%v)", got, got)
	}

	// Nested table (should come back as map or slice via ToAny).
	nested := L.GetFieldAny(-1, "nested")
	if nested == nil {
		t.Fatal("expected nested table, got nil")
	}

	// Stack unchanged.
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------------
// SetFields
// ---------------------------------------------------------------------------

func TestSetFields(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.NewTable()
	L.SetFields(-1, map[string]any{
		"x":    100,
		"name": "test",
		"ok":   true,
	})

	// Verify fields were set.
	if got := L.GetFieldInt(-1, "x"); got != 100 {
		t.Fatalf("expected x=100, got %d", got)
	}
	if got := L.GetFieldString(-1, "name"); got != "test" {
		t.Fatalf("expected name=\"test\", got %q", got)
	}
	if got := L.GetFieldBool(-1, "ok"); !got {
		t.Fatal("expected ok=true")
	}

	// Stack: just the table.
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

func TestSetFieldsNegativeIndex(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Push some values below the table to test negative index handling.
	L.PushInteger(999)
	L.NewTable()

	// Table is at -1 (top), 999 is at -2.
	L.SetFields(-1, map[string]any{
		"a": 1,
		"b": 2,
	})

	if got := L.GetFieldInt(-1, "a"); got != 1 {
		t.Fatalf("expected a=1, got %d", got)
	}
	if got := L.GetFieldInt(-1, "b"); got != 2 {
		t.Fatalf("expected b=2, got %d", got)
	}

	// Stack: [999, table].
	if L.GetTop() != 2 {
		t.Fatalf("stack leak: expected top=2, got %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------------
// NewTableFrom
// ---------------------------------------------------------------------------

func TestNewTableFrom(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.NewTableFrom(map[string]any{
		"host":  "localhost",
		"port":  8080,
		"debug": true,
	})

	if got := L.GetFieldString(-1, "host"); got != "localhost" {
		t.Fatalf("expected host=\"localhost\", got %q", got)
	}
	if got := L.GetFieldInt(-1, "port"); got != 8080 {
		t.Fatalf("expected port=8080, got %d", got)
	}
	if got := L.GetFieldBool(-1, "debug"); !got {
		t.Fatal("expected debug=true")
	}

	// Stack: just the new table.
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

func TestNewTableFromNested(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.NewTableFrom(map[string]any{
		"name": "config",
		"db": map[string]any{
			"host": "db.local",
			"port": 5432,
		},
		"tags": []any{"a", "b", "c"},
	})

	// Top-level.
	if got := L.GetFieldString(-1, "name"); got != "config" {
		t.Fatalf("expected name=\"config\", got %q", got)
	}

	// Nested map → subtable.
	L.GetField(-1, "db")
	if L.Type(-1) != lua.TypeTable {
		t.Fatalf("expected db to be a table, got %s", L.TypeName(L.Type(-1)))
	}
	if got := L.GetFieldString(-1, "host"); got != "db.local" {
		t.Fatalf("expected db.host=\"db.local\", got %q", got)
	}
	if got := L.GetFieldInt(-1, "port"); got != 5432 {
		t.Fatalf("expected db.port=5432, got %d", got)
	}
	L.Pop(1) // pop db table

	// Nested slice → array table.
	L.GetField(-1, "tags")
	if L.Type(-1) != lua.TypeTable {
		t.Fatalf("expected tags to be a table, got %s", L.TypeName(L.Type(-1)))
	}
	// tags[1] = "a"
	L.GetI(-1, 1)
	s, _ := L.ToString(-1)
	if s != "a" {
		t.Fatalf("expected tags[1]=\"a\", got %q", s)
	}
	L.Pop(1)
	// tags[3] = "c"
	L.GetI(-1, 3)
	s, _ = L.ToString(-1)
	if s != "c" {
		t.Fatalf("expected tags[3]=\"c\", got %q", s)
	}
	L.Pop(2) // pop string + tags table

	// Stack: just the root table.
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

func TestNewTableFromEmpty(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.NewTableFrom(map[string]any{})

	if L.Type(-1) != lua.TypeTable {
		t.Fatalf("expected table, got %s", L.TypeName(L.Type(-1)))
	}

	// Stack: just the empty table.
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------------
// ToStruct — recursive struct pointer slices
// ---------------------------------------------------------------------------

type testVNode struct {
	Type     string       `lua:"type"`
	Content  string       `lua:"content"`
	Children []*testVNode `lua:"children"`
	Focused  bool         `lua:"_focused"`
}

func TestToStructRecursiveSlicePtr(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		node = {
			type    = "box",
			content = "hello",
			_focused = true,
			children = {
				{ type = "text", content = "world", _focused = false },
				{ type = "span", content = "!", children = {
					{ type = "leaf", content = "nested" },
				}},
			},
		}
	`)
	if err != nil {
		t.Fatalf("DoString: %v", err)
	}

	L.GetGlobal("node")
	var vn testVNode
	if err := L.ToStruct(-1, &vn); err != nil {
		t.Fatalf("ToStruct: %v", err)
	}

	// Top-level fields.
	if vn.Type != "box" {
		t.Fatalf("expected type=\"box\", got %q", vn.Type)
	}
	if vn.Content != "hello" {
		t.Fatalf("expected content=\"hello\", got %q", vn.Content)
	}
	if !vn.Focused {
		t.Fatal("expected _focused=true")
	}

	// Children.
	if len(vn.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(vn.Children))
	}
	c0 := vn.Children[0]
	if c0 == nil {
		t.Fatal("children[0] is nil")
	}
	if c0.Type != "text" {
		t.Fatalf("expected children[0].type=\"text\", got %q", c0.Type)
	}
	if c0.Content != "world" {
		t.Fatalf("expected children[0].content=\"world\", got %q", c0.Content)
	}
	if c0.Focused {
		t.Fatal("expected children[0]._focused=false")
	}

	// Nested children (depth 2).
	c1 := vn.Children[1]
	if c1 == nil {
		t.Fatal("children[1] is nil")
	}
	if c1.Type != "span" {
		t.Fatalf("expected children[1].type=\"span\", got %q", c1.Type)
	}
	if len(c1.Children) != 1 {
		t.Fatalf("expected children[1] to have 1 child, got %d", len(c1.Children))
	}
	gc := c1.Children[0]
	if gc == nil {
		t.Fatal("grandchild is nil")
	}
	if gc.Type != "leaf" {
		t.Fatalf("expected grandchild.type=\"leaf\", got %q", gc.Type)
	}
	if gc.Content != "nested" {
		t.Fatalf("expected grandchild.content=\"nested\", got %q", gc.Content)
	}

	// Stack unchanged.
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

// Non-pointer struct slice: []testSimple
type testSimple struct {
	Name  string `lua:"name"`
	Value int64  `lua:"value"`
}

type testContainer struct {
	Items []testSimple `lua:"items"`
}

func TestToStructRecursiveSliceValue(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		container = {
			items = {
				{ name = "a", value = 1 },
				{ name = "b", value = 2 },
				{ name = "c", value = 3 },
			},
		}
	`)
	if err != nil {
		t.Fatalf("DoString: %v", err)
	}

	L.GetGlobal("container")
	var c testContainer
	if err := L.ToStruct(-1, &c); err != nil {
		t.Fatalf("ToStruct: %v", err)
	}

	if len(c.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(c.Items))
	}
	if c.Items[0].Name != "a" || c.Items[0].Value != 1 {
		t.Fatalf("items[0] = %+v, want {a, 1}", c.Items[0])
	}
	if c.Items[2].Name != "c" || c.Items[2].Value != 3 {
		t.Fatalf("items[2] = %+v, want {c, 3}", c.Items[2])
	}

	// Stack unchanged.
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

func TestToStructEmptyChildren(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`node = { type = "leaf", content = "x", children = {} }`)
	if err != nil {
		t.Fatalf("DoString: %v", err)
	}

	L.GetGlobal("node")
	var vn testVNode
	if err := L.ToStruct(-1, &vn); err != nil {
		t.Fatalf("ToStruct: %v", err)
	}

	if vn.Type != "leaf" {
		t.Fatalf("expected type=\"leaf\", got %q", vn.Type)
	}
	if vn.Children == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(vn.Children) != 0 {
		t.Fatalf("expected 0 children, got %d", len(vn.Children))
	}
}

// ---------------------------------------------------------------------------
// GetFieldRef
// ---------------------------------------------------------------------------

func TestGetFieldRef(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Create a table with a function field.
	err := L.DoString(`
		myTable = {
			greet = function() return "hello" end,
			name  = "test",
		}
	`)
	if err != nil {
		t.Fatalf("DoString: %v", err)
	}

	L.GetGlobal("myTable")

	// Get ref to function field.
	ref := L.GetFieldRef(-1, "greet")
	if ref == lua.RefNil {
		t.Fatal("expected valid ref, got RefNil")
	}

	// Retrieve the function from registry and verify it's a function.
	L.RawGetI(lua.RegistryIndex, int64(ref))
	if !L.IsFunction(-1) {
		t.Fatalf("expected function from registry, got %s", L.TypeName(L.Type(-1)))
	}
	L.Pop(1)

	// Non-function field → RefNil.
	ref2 := L.GetFieldRef(-1, "name")
	if ref2 != lua.RefNil {
		t.Fatalf("expected RefNil for string field, got %d", ref2)
	}

	// Missing field → RefNil.
	ref3 := L.GetFieldRef(-1, "nonexistent")
	if ref3 != lua.RefNil {
		t.Fatalf("expected RefNil for missing field, got %d", ref3)
	}

	// Clean up ref.
	L.Unref(lua.RegistryIndex, ref)

	// Stack: just the table.
	if L.GetTop() != 1 {
		t.Fatalf("stack leak: expected top=1, got %d", L.GetTop())
	}
}

func TestGetFieldRefCallFunction(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		myTable = {
			add = function(a, b) return a + b end,
		}
	`)
	if err != nil {
		t.Fatalf("DoString: %v", err)
	}

	L.GetGlobal("myTable")
	ref := L.GetFieldRef(-1, "add")
	if ref == lua.RefNil {
		t.Fatal("expected valid ref")
	}

	// Push function from registry, call it.
	L.RawGetI(lua.RegistryIndex, int64(ref))
	L.PushInteger(10)
	L.PushInteger(32)
	if status := L.PCall(2, 1, 0); status != lua.OK {
		msg, _ := L.ToString(-1)
		t.Fatalf("PCall failed (status %d): %s", status, msg)
	}
	result, ok := L.ToInteger(-1)
	if !ok || result != 42 {
		t.Fatalf("expected 42, got %d (ok=%v)", result, ok)
	}
	L.Pop(1)

	L.Unref(lua.RegistryIndex, ref)
}
