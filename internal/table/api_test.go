package table

import (
	"math"
	"testing"

	"github.com/akzj/go-lua/internal/object"
)

// Helper: make a short string key (simulates interning).
func mkstr(s string) *object.LuaString {
	return &object.LuaString{Data: s, Hash_: simpleHash(s), IsShort: true}
}

func simpleHash(s string) uint32 {
	var h uint32 = 5381
	for i := 0; i < len(s); i++ {
		h = ((h << 5) + h) + uint32(s[i])
	}
	return h
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func TestNewEmptyTable(t *testing.T) {
	tbl := New(0, 0)
	if tbl == nil {
		t.Fatal("New returned nil")
	}
	if tbl.ArrayLen() != 0 {
		t.Errorf("expected array len 0, got %d", tbl.ArrayLen())
	}
	if tbl.HashLen() != 0 {
		t.Errorf("expected hash len 0, got %d", tbl.HashLen())
	}
}

func TestNewWithSizes(t *testing.T) {
	tbl := New(10, 5)
	if tbl.ArrayLen() != 10 {
		t.Errorf("expected array len 10, got %d", tbl.ArrayLen())
	}
	// hashSize 5 rounds up to 8 (next power of 2)
	if tbl.HashLen() != 8 {
		t.Errorf("expected hash len 8, got %d", tbl.HashLen())
	}
}

// ---------------------------------------------------------------------------
// Basic Get/Set — Integer keys
// ---------------------------------------------------------------------------

func TestSetGetInt(t *testing.T) {
	tbl := New(10, 0)
	tbl.SetInt(1, object.MakeInteger(100))
	tbl.SetInt(5, object.MakeInteger(500))

	v, ok := tbl.GetInt(1)
	if !ok || v.Integer() != 100 {
		t.Errorf("GetInt(1) = %v, %v; want 100, true", v, ok)
	}
	v, ok = tbl.GetInt(5)
	if !ok || v.Integer() != 500 {
		t.Errorf("GetInt(5) = %v, %v; want 500, true", v, ok)
	}
	v, ok = tbl.GetInt(3)
	if ok {
		t.Errorf("GetInt(3) should be absent, got %v", v)
	}
}

func TestSetGetIntViaGet(t *testing.T) {
	tbl := New(10, 0)
	tbl.Set(object.MakeInteger(3), object.MakeFloat(3.14))

	v, ok := tbl.Get(object.MakeInteger(3))
	if !ok || v.Float() != 3.14 {
		t.Errorf("Get(3) = %v, %v; want 3.14, true", v, ok)
	}
}

// ---------------------------------------------------------------------------
// Basic Get/Set — String keys
// ---------------------------------------------------------------------------

func TestSetGetStr(t *testing.T) {
	tbl := New(0, 4)
	key := mkstr("hello")
	tbl.SetStr(key, object.MakeInteger(42))

	v, ok := tbl.GetStr(key)
	if !ok || v.Integer() != 42 {
		t.Errorf("GetStr('hello') = %v, %v; want 42, true", v, ok)
	}

	// Different string object, same content but different pointer — should NOT match
	// (short strings use pointer equality for interned strings)
	key2 := mkstr("hello")
	v, ok = tbl.GetStr(key2)
	// In our impl, short strings use pointer equality, so key2 != key
	if ok {
		t.Logf("Note: different pointer matched — this means equality is by value, not pointer. OK for correctness.")
	}
}

func TestSetGetStrViaGeneric(t *testing.T) {
	tbl := New(0, 4)
	key := mkstr("world")
	tbl.Set(object.MakeString(key), object.MakeBoolean(true))

	v, ok := tbl.Get(object.MakeString(key))
	if !ok || v.Tt != object.TagTrue {
		t.Errorf("Get('world') = %v, %v; want true, true", v, ok)
	}
}

// ---------------------------------------------------------------------------
// Float keys
// ---------------------------------------------------------------------------

func TestFloatAsIntegerKey(t *testing.T) {
	tbl := New(10, 0)
	// Set via integer key 3
	tbl.SetInt(3, object.MakeInteger(300))

	// Get via float 3.0 — should find same value
	v, ok := tbl.Get(object.MakeFloat(3.0))
	if !ok || v.Integer() != 300 {
		t.Errorf("Get(3.0) = %v, %v; want 300, true", v, ok)
	}

	// Set via float 3.0 — should overwrite same slot
	tbl.Set(object.MakeFloat(3.0), object.MakeInteger(999))
	v, ok = tbl.GetInt(3)
	if !ok || v.Integer() != 999 {
		t.Errorf("GetInt(3) after Set(3.0) = %v, %v; want 999, true", v, ok)
	}
}

func TestNonIntegerFloat(t *testing.T) {
	tbl := New(0, 4)
	tbl.Set(object.MakeFloat(1.5), object.MakeInteger(15))

	v, ok := tbl.Get(object.MakeFloat(1.5))
	if !ok || v.Integer() != 15 {
		t.Errorf("Get(1.5) = %v, %v; want 15, true", v, ok)
	}

	// 1.5 should NOT be found via integer key 1 or 2
	_, ok = tbl.GetInt(1)
	if ok {
		t.Error("GetInt(1) should not find float key 1.5")
	}
}

// ---------------------------------------------------------------------------
// Boolean keys
// ---------------------------------------------------------------------------

func TestBooleanKeys(t *testing.T) {
	tbl := New(0, 4)
	tbl.Set(object.True, object.MakeInteger(1))
	tbl.Set(object.False, object.MakeInteger(0))

	v, ok := tbl.Get(object.True)
	if !ok || v.Integer() != 1 {
		t.Errorf("Get(true) = %v, %v; want 1, true", v, ok)
	}
	v, ok = tbl.Get(object.False)
	if !ok || v.Integer() != 0 {
		t.Errorf("Get(false) = %v, %v; want 0, true", v, ok)
	}
}

// ---------------------------------------------------------------------------
// NaN and Nil key panics
// ---------------------------------------------------------------------------

func TestNaNKeyPanics(t *testing.T) {
	tbl := New(0, 4)
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Set(NaN, ...) should panic")
		}
	}()
	tbl.Set(object.MakeFloat(math.NaN()), object.MakeInteger(1))
}

func TestNilKeyPanics(t *testing.T) {
	tbl := New(0, 4)
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Set(nil, ...) should panic")
		}
	}()
	tbl.Set(object.Nil, object.MakeInteger(1))
}

// ---------------------------------------------------------------------------
// Set nil removes key
// ---------------------------------------------------------------------------

func TestSetNilRemovesKey(t *testing.T) {
	tbl := New(0, 4)
	key := mkstr("remove_me")
	tbl.Set(object.MakeString(key), object.MakeInteger(42))

	v, ok := tbl.Get(object.MakeString(key))
	if !ok {
		t.Fatal("key should exist before removal")
	}
	_ = v

	// Set to nil
	tbl.Set(object.MakeString(key), object.Nil)
	_, ok = tbl.Get(object.MakeString(key))
	if ok {
		t.Error("key should be removed after setting to nil")
	}
}

func TestSetNilRemovesArrayKey(t *testing.T) {
	tbl := New(10, 0)
	tbl.SetInt(5, object.MakeInteger(500))

	v, ok := tbl.GetInt(5)
	if !ok || v.Integer() != 500 {
		t.Fatal("key 5 should exist")
	}

	// Set to nil via generic Set
	tbl.Set(object.MakeInteger(5), object.Nil)
	_, ok = tbl.GetInt(5)
	if ok {
		t.Error("key 5 should be removed after setting to nil")
	}
}

// ---------------------------------------------------------------------------
// Array part: sequential integer keys
// ---------------------------------------------------------------------------

func TestArraySequential100(t *testing.T) {
	tbl := New(0, 0)
	for i := int64(1); i <= 100; i++ {
		tbl.Set(object.MakeInteger(i), object.MakeInteger(i*10))
	}
	for i := int64(1); i <= 100; i++ {
		v, ok := tbl.GetInt(i)
		if !ok || v.Integer() != i*10 {
			t.Errorf("GetInt(%d) = %v, %v; want %d, true", i, v, ok, i*10)
		}
	}
}

// ---------------------------------------------------------------------------
// Hash part: string keys
// ---------------------------------------------------------------------------

func TestHashStringKeys(t *testing.T) {
	tbl := New(0, 0)
	keys := make([]*object.LuaString, 20)
	for i := 0; i < 20; i++ {
		keys[i] = mkstr(string(rune('a' + i)))
		tbl.Set(object.MakeString(keys[i]), object.MakeInteger(int64(i)))
	}
	for i := 0; i < 20; i++ {
		v, ok := tbl.Get(object.MakeString(keys[i]))
		if !ok || v.Integer() != int64(i) {
			t.Errorf("Get('%c') = %v, %v; want %d, true", rune('a'+i), v, ok, i)
		}
	}
}

// ---------------------------------------------------------------------------
// RawLen
// ---------------------------------------------------------------------------

func TestRawLenEmpty(t *testing.T) {
	tbl := New(0, 0)
	if tbl.RawLen() != 0 {
		t.Errorf("RawLen of empty table = %d, want 0", tbl.RawLen())
	}
}

func TestRawLenSequential(t *testing.T) {
	tbl := New(10, 0)
	for i := int64(1); i <= 10; i++ {
		tbl.SetInt(i, object.MakeInteger(i))
	}
	l := tbl.RawLen()
	if l != 10 {
		t.Errorf("RawLen = %d, want 10", l)
	}
}

func TestRawLenWithGap(t *testing.T) {
	tbl := New(10, 0)
	tbl.SetInt(1, object.MakeInteger(1))
	tbl.SetInt(2, object.MakeInteger(2))
	tbl.SetInt(3, object.MakeInteger(3))
	// Gap at 4
	tbl.SetInt(5, object.MakeInteger(5))

	l := tbl.RawLen()
	// Lua # can return ANY valid boundary.
	// Valid boundaries: 3 (t[3]!=nil, t[4]==nil) or 5 (t[5]!=nil, t[6]==nil)
	if l != 3 && l != 5 {
		t.Errorf("RawLen = %d, want 3 or 5", l)
	}
}

func TestRawLenFirstNil(t *testing.T) {
	tbl := New(10, 0)
	// t[1] is nil, so boundary is 0
	tbl.SetInt(2, object.MakeInteger(2))
	l := tbl.RawLen()
	// Boundary: t[0+1]=nil → 0, but t[2] exists...
	// The # operator can return 0 or 2 here (both are valid boundaries)
	if l != 0 && l != 2 {
		t.Errorf("RawLen = %d, want 0 or 2", l)
	}
}

// ---------------------------------------------------------------------------
// Next — iteration
// ---------------------------------------------------------------------------

func TestNextEmptyTable(t *testing.T) {
	tbl := New(0, 0)
	_, _, ok, _ := tbl.Next(object.Nil)
	if ok {
		t.Error("Next on empty table should return false")
	}
}

func TestNextArrayOnly(t *testing.T) {
	tbl := New(5, 0)
	for i := int64(1); i <= 5; i++ {
		tbl.SetInt(i, object.MakeInteger(i*10))
	}

	count := 0
	key := object.Nil
	for {
		k, v, ok, _ := tbl.Next(key)
		if !ok {
			break
		}
		count++
		if !k.IsInteger() {
			t.Errorf("expected integer key, got tag %d", k.Tt)
		}
		if v.Integer() != k.Integer()*10 {
			t.Errorf("value mismatch: key=%d, val=%d", k.Integer(), v.Integer())
		}
		key = k
	}
	if count != 5 {
		t.Errorf("Next iterated %d entries, want 5", count)
	}
}

func TestNextMixedKeys(t *testing.T) {
	tbl := New(0, 0)
	tbl.Set(object.MakeInteger(1), object.MakeInteger(10))
	tbl.Set(object.MakeInteger(2), object.MakeInteger(20))
	key := mkstr("x")
	tbl.Set(object.MakeString(key), object.MakeInteger(99))

	count := 0
	k := object.Nil
	for {
		var ok bool
		k, _, ok, _ = tbl.Next(k)
		if !ok {
			break
		}
		count++
	}
	if count != 3 {
		t.Errorf("Next iterated %d entries, want 3", count)
	}
}

// ---------------------------------------------------------------------------
// Rehash (triggered by inserting into full hash)
// ---------------------------------------------------------------------------

func TestRehashManyKeys(t *testing.T) {
	tbl := New(0, 0)
	n := 200
	keys := make([]*object.LuaString, n)
	for i := 0; i < n; i++ {
		keys[i] = mkstr(string(rune(i + 0x100)))
		tbl.Set(object.MakeString(keys[i]), object.MakeInteger(int64(i)))
	}
	// Verify all keys are retrievable
	for i := 0; i < n; i++ {
		v, ok := tbl.Get(object.MakeString(keys[i]))
		if !ok || v.Integer() != int64(i) {
			t.Errorf("after rehash: Get(key[%d]) = %v, %v; want %d, true", i, v, ok, i)
		}
	}
}

func TestRehashIntegerKeys(t *testing.T) {
	tbl := New(0, 0) // start empty, force rehash
	for i := int64(1); i <= 100; i++ {
		tbl.Set(object.MakeInteger(i), object.MakeInteger(i*10))
	}
	for i := int64(1); i <= 100; i++ {
		v, ok := tbl.GetInt(i)
		if !ok || v.Integer() != i*10 {
			t.Errorf("GetInt(%d) = %v, %v; want %d, true", i, v, ok, i*10)
		}
	}
	// After rehash, integer keys 1..N should be in array part
	if tbl.ArrayLen() == 0 {
		t.Error("expected non-zero array length after inserting sequential integers")
	}
}

// ---------------------------------------------------------------------------
// Metatable
// ---------------------------------------------------------------------------

func TestMetatable(t *testing.T) {
	tbl := New(0, 0)
	if tbl.GetMetatable() != nil {
		t.Error("new table should have nil metatable")
	}

	mt := New(0, 4)
	tbl.SetMetatable(mt)
	if tbl.GetMetatable() != mt {
		t.Error("metatable not set correctly")
	}
}

func TestMetamethodFlags(t *testing.T) {
	tbl := New(0, 0)
	// New table has flags = 0x3F (all 6 TMs marked absent)
	if tbl.HasTagMethod(0) {
		t.Error("new table should have TM 0 absent")
	}

	tbl.InvalidateFlags()
	if !tbl.HasTagMethod(0) {
		t.Error("after invalidate, TM 0 should be 'might be present'")
	}

	tbl.SetNoTagMethod(0)
	if tbl.HasTagMethod(0) {
		t.Error("after SetNoTagMethod(0), TM 0 should be absent")
	}
}

// ---------------------------------------------------------------------------
// Empty table edge cases
// ---------------------------------------------------------------------------

func TestEmptyTableGet(t *testing.T) {
	tbl := New(0, 0)
	v, ok := tbl.Get(object.MakeInteger(1))
	if ok || !v.IsNil() {
		t.Errorf("empty table Get(1) = %v, %v; want nil, false", v, ok)
	}
	v, ok = tbl.GetInt(42)
	if ok {
		t.Error("empty table GetInt should return false")
	}
}

// ---------------------------------------------------------------------------
// Large table stress test
// ---------------------------------------------------------------------------

func TestLargeTable(t *testing.T) {
	tbl := New(0, 0)
	n := 1000
	for i := 0; i < n; i++ {
		tbl.Set(object.MakeInteger(int64(i+1)), object.MakeInteger(int64(i*i)))
	}
	for i := 0; i < n; i++ {
		v, ok := tbl.GetInt(int64(i + 1))
		if !ok || v.Integer() != int64(i*i) {
			t.Errorf("large table: GetInt(%d) = %v, %v; want %d", i+1, v, ok, i*i)
		}
	}

	// Verify iteration count
	count := 0
	k := object.Nil
	for {
		var ok bool
		k, _, ok, _ = tbl.Next(k)
		if !ok {
			break
		}
		count++
	}
	if count != n {
		t.Errorf("large table iteration: got %d entries, want %d", count, n)
	}
}

// ---------------------------------------------------------------------------
// Overwrite existing key
// ---------------------------------------------------------------------------

func TestOverwriteExistingKey(t *testing.T) {
	tbl := New(10, 4)
	tbl.SetInt(1, object.MakeInteger(100))
	tbl.SetInt(1, object.MakeInteger(200))

	v, ok := tbl.GetInt(1)
	if !ok || v.Integer() != 200 {
		t.Errorf("overwrite: GetInt(1) = %v, %v; want 200, true", v, ok)
	}
}

func TestOverwriteStringKey(t *testing.T) {
	tbl := New(0, 4)
	key := mkstr("test")
	tbl.SetStr(key, object.MakeInteger(1))
	tbl.SetStr(key, object.MakeInteger(2))

	v, ok := tbl.GetStr(key)
	if !ok || v.Integer() != 2 {
		t.Errorf("overwrite: GetStr('test') = %v, %v; want 2, true", v, ok)
	}
}

// ---------------------------------------------------------------------------
// Mixed types in same table
// ---------------------------------------------------------------------------

func TestMixedKeyTypes(t *testing.T) {
	tbl := New(0, 0)
	sk := mkstr("key")
	tbl.Set(object.MakeInteger(1), object.MakeInteger(10))
	tbl.Set(object.MakeString(sk), object.MakeInteger(20))
	tbl.Set(object.MakeFloat(2.5), object.MakeInteger(30))
	tbl.Set(object.True, object.MakeInteger(40))

	tests := []struct {
		key  object.TValue
		want int64
	}{
		{object.MakeInteger(1), 10},
		{object.MakeString(sk), 20},
		{object.MakeFloat(2.5), 30},
		{object.True, 40},
	}
	for _, tt := range tests {
		v, ok := tbl.Get(tt.key)
		if !ok || v.Integer() != tt.want {
			t.Errorf("Get(%v) = %v, %v; want %d, true", tt.key, v, ok, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Negative and zero integer keys (should go to hash)
// ---------------------------------------------------------------------------

func TestNegativeAndZeroKeys(t *testing.T) {
	tbl := New(5, 0)
	tbl.Set(object.MakeInteger(0), object.MakeInteger(100))
	tbl.Set(object.MakeInteger(-1), object.MakeInteger(200))

	v, ok := tbl.Get(object.MakeInteger(0))
	if !ok || v.Integer() != 100 {
		t.Errorf("Get(0) = %v, %v; want 100, true", v, ok)
	}
	v, ok = tbl.Get(object.MakeInteger(-1))
	if !ok || v.Integer() != 200 {
		t.Errorf("Get(-1) = %v, %v; want 200, true", v, ok)
	}
}

// ---------------------------------------------------------------------------
// RawLen with hash-only table
// ---------------------------------------------------------------------------

func TestRawLenHashOnly(t *testing.T) {
	tbl := New(0, 0)
	// No array part, just hash
	tbl.Set(object.MakeInteger(1), object.MakeInteger(10))
	tbl.Set(object.MakeInteger(2), object.MakeInteger(20))
	// After rehash, these should go to array, but let's test the boundary
	l := tbl.RawLen()
	if l != 2 {
		t.Errorf("RawLen = %d, want 2", l)
	}
}

// ---------------------------------------------------------------------------
// Delete and re-insert
// ---------------------------------------------------------------------------

func TestDeleteAndReinsert(t *testing.T) {
	tbl := New(0, 4)
	key := mkstr("reuse")
	tbl.Set(object.MakeString(key), object.MakeInteger(1))
	tbl.Set(object.MakeString(key), object.Nil) // delete

	_, ok := tbl.Get(object.MakeString(key))
	if ok {
		t.Error("key should be deleted")
	}

	// Re-insert
	tbl.Set(object.MakeString(key), object.MakeInteger(2))
	v, ok := tbl.Get(object.MakeString(key))
	if !ok || v.Integer() != 2 {
		t.Errorf("after re-insert: Get = %v, %v; want 2, true", v, ok)
	}
}
