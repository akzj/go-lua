package lua_test

import (
	"math"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

// ---------------------------------------------------------------------------
// PushAny tests
// ---------------------------------------------------------------------------

func TestPushAnyNil(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushAny(nil)
	if !L.IsNil(-1) {
		t.Fatalf("expected nil, got type %s", L.TypeName(L.Type(-1)))
	}
	L.Pop(1)
}

func TestPushAnyBool(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushAny(true)
	if !L.IsBoolean(-1) || !L.ToBoolean(-1) {
		t.Fatal("expected true")
	}
	L.Pop(1)

	L.PushAny(false)
	if !L.IsBoolean(-1) || L.ToBoolean(-1) {
		t.Fatal("expected false")
	}
	L.Pop(1)
}

func TestPushAnyIntegers(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cases := []struct {
		name string
		val  any
		want int64
	}{
		{"int", int(42), 42},
		{"int8", int8(8), 8},
		{"int16", int16(16), 16},
		{"int32", int32(32), 32},
		{"int64", int64(64), 64},
		{"uint", uint(10), 10},
		{"uint8", uint8(8), 8},
		{"uint16", uint16(16), 16},
		{"uint32", uint32(32), 32},
		{"uint64-small", uint64(100), 100},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L.PushAny(tc.val)
			if !L.IsInteger(-1) {
				t.Fatalf("expected integer, got type %s", L.TypeName(L.Type(-1)))
			}
			got, ok := L.ToInteger(-1)
			if !ok || got != tc.want {
				t.Fatalf("expected %d, got %d (ok=%v)", tc.want, got, ok)
			}
			L.Pop(1)
		})
	}
}

func TestPushAnyUint64Large(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	big := uint64(math.MaxInt64) + 1
	L.PushAny(big)
	if !L.IsNumber(-1) {
		t.Fatalf("expected number for large uint64, got %s", L.TypeName(L.Type(-1)))
	}
	v, ok := L.ToNumber(-1)
	if !ok {
		t.Fatal("ToNumber failed")
	}
	// float64 can't represent this exactly, but should be close.
	if v < float64(math.MaxInt64) {
		t.Fatalf("expected large number, got %f", v)
	}
	L.Pop(1)
}

func TestPushAnyFloats(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushAny(float64(3.14))
	v, ok := L.ToNumber(-1)
	if !ok || v != 3.14 {
		t.Fatalf("expected 3.14, got %f", v)
	}
	L.Pop(1)

	L.PushAny(float32(2.5))
	v, ok = L.ToNumber(-1)
	if !ok || v != 2.5 {
		t.Fatalf("expected 2.5, got %f", v)
	}
	L.Pop(1)
}

func TestPushAnyString(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushAny("hello")
	s, ok := L.ToString(-1)
	if !ok || s != "hello" {
		t.Fatalf("expected 'hello', got %q", s)
	}
	L.Pop(1)
}

func TestPushAnyBytes(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushAny([]byte("binary"))
	s, ok := L.ToString(-1)
	if !ok || s != "binary" {
		t.Fatalf("expected 'binary', got %q", s)
	}
	L.Pop(1)
}

func TestPushAnyFunction(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	fn := lua.Function(func(L *lua.State) int {
		L.PushInteger(99)
		return 1
	})
	L.PushAny(fn)
	if !L.IsFunction(-1) {
		t.Fatal("expected function")
	}
	L.SetGlobal("myfn")

	err := L.DoString(`assert(myfn() == 99)`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushAnyMapStringAny(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	m := map[string]any{
		"name": "Alice",
		"age":  int64(30),
	}
	L.PushAny(m)
	L.SetGlobal("t")

	err := L.DoString(`
		assert(t.name == "Alice", "name mismatch: " .. tostring(t.name))
		assert(t.age == 30, "age mismatch: " .. tostring(t.age))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushAnyMapStringString(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	m := map[string]string{"key": "value"}
	L.PushAny(m)
	L.SetGlobal("t")

	err := L.DoString(`assert(t.key == "value")`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushAnySlice(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	s := []any{int64(10), "hello", true}
	L.PushAny(s)
	L.SetGlobal("arr")

	err := L.DoString(`
		assert(#arr == 3, "length: " .. #arr)
		assert(arr[1] == 10, "arr[1]: " .. tostring(arr[1]))
		assert(arr[2] == "hello", "arr[2]: " .. tostring(arr[2]))
		assert(arr[3] == true, "arr[3]: " .. tostring(arr[3]))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushAnyTypedSlice(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// []int — not []any, must go through reflection.
	s := []int{10, 20, 30}
	L.PushAny(s)
	L.SetGlobal("arr")

	err := L.DoString(`
		assert(#arr == 3)
		assert(arr[1] == 10)
		assert(arr[2] == 20)
		assert(arr[3] == 30)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushAnyNestedMapSlice(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	m := map[string]any{
		"items": []any{int64(1), int64(2), int64(3)},
		"meta": map[string]any{
			"count": int64(3),
		},
	}
	L.PushAny(m)
	L.SetGlobal("data")

	err := L.DoString(`
		assert(#data.items == 3)
		assert(data.items[2] == 2)
		assert(data.meta.count == 3)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushAnyStruct(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	type Point struct {
		X int
		Y int
	}
	p := Point{X: 10, Y: 20}
	L.PushAny(p)
	L.SetGlobal("pt")

	// Default naming: lowercase first letter → "x", "y"
	err := L.DoString(`
		assert(pt.x == 10, "x: " .. tostring(pt.x))
		assert(pt.y == 20, "y: " .. tostring(pt.y))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushAnyStructWithTags(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	type Config struct {
		Host    string  `lua:"host"`
		Port    int64   `lua:"port"`
		Debug   bool    `lua:"debug"`
		Weight  float64 `lua:"weight"`
		Ignored string  `lua:"-"`
		NoTag   string
	}
	cfg := Config{
		Host:    "localhost",
		Port:    8080,
		Debug:   true,
		Weight:  1.5,
		Ignored: "should-not-appear",
		NoTag:   "visible",
	}
	L.PushAny(cfg)
	L.SetGlobal("cfg")

	err := L.DoString(`
		assert(cfg.host == "localhost", "host: " .. tostring(cfg.host))
		assert(cfg.port == 8080, "port: " .. tostring(cfg.port))
		assert(cfg.debug == true, "debug: " .. tostring(cfg.debug))
		assert(cfg.weight == 1.5, "weight: " .. tostring(cfg.weight))
		assert(cfg.noTag == "visible", "noTag: " .. tostring(cfg.noTag))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Verify the ignored field is not present.
	err = L.DoString(`
		for k, v in pairs(cfg) do
			if k == "-" then error("found ignored field with key '-'") end
		end
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushAnyStructPointer(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	type Item struct {
		Name  string `lua:"name"`
		Value int    `lua:"value"`
	}
	item := &Item{Name: "sword", Value: 42}
	L.PushAny(item)
	L.SetGlobal("item")

	err := L.DoString(`
		assert(item.name == "sword")
		assert(item.value == 42)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestPushAnyNilPointer(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	var p *int
	L.PushAny(p)
	if !L.IsNil(-1) {
		t.Fatalf("expected nil for nil pointer, got %s", L.TypeName(L.Type(-1)))
	}
	L.Pop(1)
}

func TestPushAnyMapIntKey(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	m := map[int]string{1: "one", 2: "two"}
	L.PushAny(m)
	L.SetGlobal("t")

	err := L.DoString(`
		assert(t[1] == "one", "t[1]: " .. tostring(t[1]))
		assert(t[2] == "two", "t[2]: " .. tostring(t[2]))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ToAny tests
// ---------------------------------------------------------------------------

func TestToAnyNil(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushNil()
	v := L.ToAny(-1)
	if v != nil {
		t.Fatalf("expected nil, got %v", v)
	}
	L.Pop(1)
}

func TestToAnyBool(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushBoolean(true)
	v := L.ToAny(-1)
	b, ok := v.(bool)
	if !ok || !b {
		t.Fatalf("expected true, got %v", v)
	}
	L.Pop(1)
}

func TestToAnyInteger(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushInteger(42)
	v := L.ToAny(-1)
	n, ok := v.(int64)
	if !ok || n != 42 {
		t.Fatalf("expected int64(42), got %v (%T)", v, v)
	}
	L.Pop(1)
}

func TestToAnyFloat(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushNumber(3.14)
	v := L.ToAny(-1)
	f, ok := v.(float64)
	if !ok || f != 3.14 {
		t.Fatalf("expected float64(3.14), got %v (%T)", v, v)
	}
	L.Pop(1)
}

func TestToAnyString(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushString("hello")
	v := L.ToAny(-1)
	s, ok := v.(string)
	if !ok || s != "hello" {
		t.Fatalf("expected 'hello', got %v", v)
	}
	L.Pop(1)
}

func TestToAnyArrayTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`arr = {10, 20, 30}`)
	if err != nil {
		t.Fatal(err)
	}
	L.GetGlobal("arr")
	v := L.ToAny(-1)
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", v)
	}
	if len(arr) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr))
	}
	for i, want := range []int64{10, 20, 30} {
		got, ok := arr[i].(int64)
		if !ok || got != want {
			t.Fatalf("arr[%d]: expected %d, got %v", i, want, arr[i])
		}
	}
	L.Pop(1)
}

func TestToAnyHashTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`t = {name = "Alice", age = 30}`)
	if err != nil {
		t.Fatal(err)
	}
	L.GetGlobal("t")
	v := L.ToAny(-1)
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", v)
	}
	if m["name"] != "Alice" {
		t.Fatalf("name: expected Alice, got %v", m["name"])
	}
	if m["age"] != int64(30) {
		t.Fatalf("age: expected 30, got %v", m["age"])
	}
	L.Pop(1)
}

func TestToAnyMixedTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// A table with both integer and string keys → map[string]any.
	err := L.DoString(`t = {10, 20, name = "mixed"}`)
	if err != nil {
		t.Fatal(err)
	}
	L.GetGlobal("t")
	v := L.ToAny(-1)
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any for mixed table, got %T", v)
	}
	if m["name"] != "mixed" {
		t.Fatalf("name: expected 'mixed', got %v", m["name"])
	}
	// Integer keys are converted to string keys.
	if m["1"] != int64(10) {
		t.Fatalf("key '1': expected 10, got %v", m["1"])
	}
	L.Pop(1)
}

func TestToAnyNestedTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`t = {items = {1, 2, 3}, meta = {count = 3}}`)
	if err != nil {
		t.Fatal(err)
	}
	L.GetGlobal("t")
	v := L.ToAny(-1)
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", v)
	}

	items, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("items: expected []any, got %T", m["items"])
	}
	if len(items) != 3 {
		t.Fatalf("items: expected 3 elements, got %d", len(items))
	}

	meta, ok := m["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta: expected map[string]any, got %T", m["meta"])
	}
	if meta["count"] != int64(3) {
		t.Fatalf("meta.count: expected 3, got %v", meta["count"])
	}
	L.Pop(1)
}

func TestToAnyEmptyTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`t = {}`)
	if err != nil {
		t.Fatal(err)
	}
	L.GetGlobal("t")
	v := L.ToAny(-1)
	// Empty table: LenI returns 0, so it goes to the map path.
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any for empty table, got %T", v)
	}
	if len(m) != 0 {
		t.Fatalf("expected empty map, got %v", m)
	}
	L.Pop(1)
}

func TestToAnyUserdata(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.NewUserdata(0, 0)
	L.SetUserdataValue(-1, "my-go-value")
	v := L.ToAny(-1)
	s, ok := v.(string)
	if !ok || s != "my-go-value" {
		t.Fatalf("expected 'my-go-value', got %v (%T)", v, v)
	}
	L.Pop(1)
}

// ---------------------------------------------------------------------------
// ToStruct tests
// ---------------------------------------------------------------------------

func TestToStruct(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`cfg = {host = "localhost", port = 8080, debug = true, weight = 1.5}`)
	if err != nil {
		t.Fatal(err)
	}

	type Config struct {
		Host   string  `lua:"host"`
		Port   int64   `lua:"port"`
		Debug  bool    `lua:"debug"`
		Weight float64 `lua:"weight"`
	}

	L.GetGlobal("cfg")
	var cfg Config
	if err := L.ToStruct(-1, &cfg); err != nil {
		t.Fatalf("ToStruct failed: %v", err)
	}
	L.Pop(1)

	if cfg.Host != "localhost" {
		t.Errorf("Host: expected 'localhost', got %q", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port: expected 8080, got %d", cfg.Port)
	}
	if !cfg.Debug {
		t.Error("Debug: expected true")
	}
	if cfg.Weight != 1.5 {
		t.Errorf("Weight: expected 1.5, got %f", cfg.Weight)
	}
}

func TestToStructDefaultNaming(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`pt = {x = 10, y = 20}`)
	if err != nil {
		t.Fatal(err)
	}

	type Point struct {
		X int64
		Y int64
	}

	L.GetGlobal("pt")
	var pt Point
	if err := L.ToStruct(-1, &pt); err != nil {
		t.Fatalf("ToStruct failed: %v", err)
	}
	L.Pop(1)

	if pt.X != 10 {
		t.Errorf("X: expected 10, got %d", pt.X)
	}
	if pt.Y != 20 {
		t.Errorf("Y: expected 20, got %d", pt.Y)
	}
}

func TestToStructSkipsIgnored(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`t = {name = "test", secret = "hidden"}`)
	if err != nil {
		t.Fatal(err)
	}

	type Item struct {
		Name   string `lua:"name"`
		Secret string `lua:"-"`
	}

	L.GetGlobal("t")
	var item Item
	item.Secret = "original" // should not be overwritten
	if err := L.ToStruct(-1, &item); err != nil {
		t.Fatalf("ToStruct failed: %v", err)
	}
	L.Pop(1)

	if item.Name != "test" {
		t.Errorf("Name: expected 'test', got %q", item.Name)
	}
	if item.Secret != "original" {
		t.Errorf("Secret: should not be overwritten, got %q", item.Secret)
	}
}

func TestToStructMissingFields(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`t = {name = "partial"}`)
	if err != nil {
		t.Fatal(err)
	}

	type Full struct {
		Name  string `lua:"name"`
		Value int64  `lua:"value"`
	}

	L.GetGlobal("t")
	var f Full
	f.Value = 99 // should remain unchanged
	if err := L.ToStruct(-1, &f); err != nil {
		t.Fatalf("ToStruct failed: %v", err)
	}
	L.Pop(1)

	if f.Name != "partial" {
		t.Errorf("Name: expected 'partial', got %q", f.Name)
	}
	if f.Value != 99 {
		t.Errorf("Value: should remain 99, got %d", f.Value)
	}
}

func TestToStructNotTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.PushString("not a table")
	type S struct{ X int }
	var s S
	err := L.ToStruct(-1, &s)
	if err == nil {
		t.Fatal("expected error for non-table value")
	}
	L.Pop(1)
}

func TestToStructNotPointer(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.NewTable()
	type S struct{ X int }
	var s S
	err := L.ToStruct(-1, s) // not a pointer
	if err == nil {
		t.Fatal("expected error for non-pointer dest")
	}
	L.Pop(1)
}

func TestToStructNested(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`t = {name = "outer", inner = {value = 42}}`)
	if err != nil {
		t.Fatal(err)
	}

	type Inner struct {
		Value int64 `lua:"value"`
	}
	type Outer struct {
		Name  string `lua:"name"`
		Inner Inner  `lua:"inner"`
	}

	L.GetGlobal("t")
	var o Outer
	if err := L.ToStruct(-1, &o); err != nil {
		t.Fatalf("ToStruct failed: %v", err)
	}
	L.Pop(1)

	if o.Name != "outer" {
		t.Errorf("Name: expected 'outer', got %q", o.Name)
	}
	if o.Inner.Value != 42 {
		t.Errorf("Inner.Value: expected 42, got %d", o.Inner.Value)
	}
}

func TestToStructUintFields(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`t = {count = 100, small = 5}`)
	if err != nil {
		t.Fatal(err)
	}

	type S struct {
		Count uint64 `lua:"count"`
		Small uint8  `lua:"small"`
	}

	L.GetGlobal("t")
	var s S
	if err := L.ToStruct(-1, &s); err != nil {
		t.Fatalf("ToStruct failed: %v", err)
	}
	L.Pop(1)

	if s.Count != 100 {
		t.Errorf("Count: expected 100, got %d", s.Count)
	}
	if s.Small != 5 {
		t.Errorf("Small: expected 5, got %d", s.Small)
	}
}

// ---------------------------------------------------------------------------
// RegisterModule tests
// ---------------------------------------------------------------------------

func TestRegisterModule(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.RegisterModule(L, "mymod", map[string]lua.Function{
		"greet": func(L *lua.State) int {
			name := L.CheckString(1)
			L.PushString("Hello, " + name + "!")
			return 1
		},
		"add": func(L *lua.State) int {
			a := L.CheckInteger(1)
			b := L.CheckInteger(2)
			L.PushInteger(a + b)
			return 1
		},
	})

	err := L.DoString(`
		local m = require("mymod")
		assert(m.greet("World") == "Hello, World!", "greet failed")
		assert(m.add(10, 32) == 42, "add failed")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestRegisterModuleMultipleRequire(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	lua.RegisterModule(L, "counter", map[string]lua.Function{
		"new": func(L *lua.State) int {
			L.PushInteger(0)
			return 1
		},
	})

	err := L.DoString(`
		local c1 = require("counter")
		local c2 = require("counter")
		assert(c1 == c2, "require should return same table")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Round-trip tests
// ---------------------------------------------------------------------------

func TestPushAnyToAnyRoundTrip(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cases := []struct {
		name string
		val  any
	}{
		{"nil", nil},
		{"bool-true", true},
		{"bool-false", false},
		{"int64", int64(42)},
		{"float64", float64(3.14)},
		{"string", "hello"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L.PushAny(tc.val)
			got := L.ToAny(-1)
			L.Pop(1)

			if got != tc.val {
				t.Fatalf("round trip: pushed %v (%T), got %v (%T)",
					tc.val, tc.val, got, got)
			}
		})
	}
}

func TestPushAnyToAnyMapRoundTrip(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	original := map[string]any{
		"name": "Alice",
		"age":  int64(30),
	}
	L.PushAny(original)
	got := L.ToAny(-1)
	L.Pop(1)

	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", got)
	}
	if m["name"] != "Alice" {
		t.Errorf("name: expected 'Alice', got %v", m["name"])
	}
	if m["age"] != int64(30) {
		t.Errorf("age: expected 30, got %v", m["age"])
	}
}

func TestPushAnyToAnySliceRoundTrip(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	original := []any{int64(1), "two", true}
	L.PushAny(original)
	got := L.ToAny(-1)
	L.Pop(1)

	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", got)
	}
	if len(arr) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr))
	}
	if arr[0] != int64(1) {
		t.Errorf("[0]: expected 1, got %v", arr[0])
	}
	if arr[1] != "two" {
		t.Errorf("[1]: expected 'two', got %v", arr[1])
	}
	if arr[2] != true {
		t.Errorf("[2]: expected true, got %v", arr[2])
	}
}

func TestPushAnyToStructRoundTrip(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	type Config struct {
		Host  string  `lua:"host"`
		Port  int64   `lua:"port"`
		Debug bool    `lua:"debug"`
		Rate  float64 `lua:"rate"`
	}

	original := Config{Host: "localhost", Port: 8080, Debug: true, Rate: 0.5}
	L.PushAny(original)

	var got Config
	err := L.ToStruct(-1, &got)
	if err != nil {
		t.Fatalf("ToStruct failed: %v", err)
	}
	L.Pop(1)

	if got != original {
		t.Fatalf("round trip mismatch: %+v != %+v", got, original)
	}
}
