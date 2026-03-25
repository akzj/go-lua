package table

import (
	"testing"

	"github.com/akzj/go-lua/pkg/object"
)

// TestTable_NewTable tests table creation
func TestTable_NewTable(t *testing.T) {
	t.Run("empty table", func(t *testing.T) {
		tab := NewTable(0, 0)
		if tab == nil {
			t.Fatal("NewTable returned nil")
		}
		if len(tab.Array) != 0 {
			t.Errorf("Expected empty array, got length %d", len(tab.Array))
		}
		if len(tab.Map) != 0 {
			t.Errorf("Expected empty map, got length %d", len(tab.Map))
		}
		if !tab.IsArray {
			t.Error("Expected IsArray to be true for empty table")
		}
	})

	t.Run("pre-allocated array", func(t *testing.T) {
		tab := NewTable(10, 0)
		if cap(tab.Array) != 10 {
			t.Errorf("Expected array capacity 10, got %d", cap(tab.Array))
		}
	})

	t.Run("pre-allocated map", func(t *testing.T) {
		tab := NewTable(0, 10)
		if len(tab.Map) != 0 {
			t.Errorf("Expected empty map, got length %d", len(tab.Map))
		}
		if tab.IsArray {
			t.Error("Expected IsArray to be false when map is pre-allocated")
		}
	})
}

// TestTable_GetI_SetI tests array access operations
func TestTable_GetI_SetI(t *testing.T) {
	tab := NewTable(0, 0)

	t.Run("set and get single value", func(t *testing.T) {
		val := object.NewString("hello")
		tab.SetI(1, *val)

		got := tab.GetI(1)
		if got == nil {
			t.Fatal("GetI returned nil")
		}
		if got.Type != object.TypeString {
			t.Errorf("Expected string type, got %v", got.Type)
		}
		s, ok := got.ToString()
		if !ok || s != "hello" {
			t.Errorf("Expected 'hello', got '%v'", s)
		}
	})

	t.Run("set multiple values", func(t *testing.T) {
		tab := NewTable(0, 0)
		for i := 1; i <= 5; i++ {
			tab.SetI(i, *object.NewInteger(int64(i*10)))
		}

		for i := 1; i <= 5; i++ {
			got := tab.GetI(i)
			if got == nil {
				t.Errorf("GetI(%d) returned nil", i)
				continue
			}
			n, ok := got.ToNumber()
			if !ok || n != float64(i*10) {
				t.Errorf("GetI(%d) expected %d, got %v", i, i*10, n)
			}
		}
	})

	t.Run("get out of bounds", func(t *testing.T) {
		tab := NewTable(0, 0)
		tab.SetI(1, *object.NewString("test"))

		got := tab.GetI(0)
		if got != nil {
			t.Error("GetI(0) should return nil")
		}

		got = tab.GetI(2)
		if got != nil {
			t.Error("GetI(2) should return nil for unset index")
		}
	})

	t.Run("set negative index", func(t *testing.T) {
		tab := NewTable(0, 0)
		val := object.NewString("negative")
		tab.SetI(-1, *val)

		// Negative indices should be stored in map
		got := tab.Get(*object.NewInteger(-1))
		if got == nil {
			t.Error("Get(-1) should return value stored in map")
		}
	})
}

// TestTable_Get_Set tests hash access operations
func TestTable_Get_Set(t *testing.T) {
	tab := NewTable(0, 0)

	t.Run("string key", func(t *testing.T) {
		key := object.NewString("key1")
		val := object.NewNumber(42.0)
		tab.Set(*key, *val)

		got := tab.Get(*key)
		if got == nil {
			t.Fatal("Get returned nil for string key")
		}
		n, ok := got.ToNumber()
		if !ok || n != 42.0 {
			t.Errorf("Expected 42.0, got %v", n)
		}
	})

	t.Run("update existing key", func(t *testing.T) {
		key := object.NewString("key2")
		tab.Set(*key, *object.NewNumber(100.0))
		tab.Set(*key, *object.NewNumber(200.0))

		got := tab.Get(*key)
		if got == nil {
			t.Fatal("Get returned nil")
		}
		n, ok := got.ToNumber()
		if !ok || n != 200.0 {
			t.Errorf("Expected 200.0, got %v", n)
		}
	})

	t.Run("boolean key", func(t *testing.T) {
		keyTrue := object.NewBoolean(true)
		keyFalse := object.NewBoolean(false)
		tab.Set(*keyTrue, *object.NewString("true"))
		tab.Set(*keyFalse, *object.NewString("false"))

		gotTrue := tab.Get(*keyTrue)
		if gotTrue == nil {
			t.Error("Get(true) returned nil")
		}

		gotFalse := tab.Get(*keyFalse)
		if gotFalse == nil {
			t.Error("Get(false) returned nil")
		}
	})

	t.Run("number key (non-integer)", func(t *testing.T) {
		key := object.NewNumber(3.14)
		val := object.NewString("pi")
		tab.Set(*key, *val)

		got := tab.Get(*key)
		if got == nil {
			t.Error("Get(3.14) returned nil")
		}
	})

	t.Run("get non-existent key", func(t *testing.T) {
		got := tab.Get(*object.NewString("nonexistent"))
		if got != nil {
			t.Error("Get should return nil for non-existent key")
		}
	})
}

// TestTable_Get_Set_Array tests array access through Get/Set
func TestTable_Get_Set_Array(t *testing.T) {
	tab := NewTable(0, 0)

	t.Run("set integer key 1", func(t *testing.T) {
		key := object.NewInteger(1)
		val := object.NewString("first")
		tab.Set(*key, *val)

		got := tab.Get(*key)
		if got == nil {
			t.Fatal("Get(1) returned nil")
		}
		s, ok := got.ToString()
		if !ok || s != "first" {
			t.Errorf("Expected 'first', got '%v'", s)
		}
	})

	t.Run("set integer keys sequentially", func(t *testing.T) {
		tab := NewTable(0, 0)
		for i := 1; i <= 10; i++ {
			key := object.NewInteger(int64(i))
			val := object.NewInteger(int64(i * 100))
			tab.Set(*key, *val)
		}

		// Verify array was used
		if len(tab.Array) != 10 {
			t.Errorf("Expected array length 10, got %d", len(tab.Array))
		}

		// Verify values
		for i := 1; i <= 10; i++ {
			key := object.NewInteger(int64(i))
			got := tab.Get(*key)
			if got == nil {
				t.Errorf("Get(%d) returned nil", i)
				continue
			}
			n, ok := got.ToNumber()
			if !ok || n != float64(i*100) {
				t.Errorf("Get(%d) expected %d, got %v", i, i*100, n)
			}
		}
	})

	t.Run("array with gap", func(t *testing.T) {
		tab := NewTable(0, 0)
		tab.Set(*object.NewInteger(1), *object.NewString("a"))
		tab.Set(*object.NewInteger(3), *object.NewString("c"))

		// Index 2 should be nil (zero value)
		got2 := tab.Get(*object.NewInteger(2))
		if got2 == nil {
			t.Error("Get(2) should return a value (possibly nil TValue)")
		}
	})
}

// TestTable_Get_Set_Mixed tests mixed array and hash access
func TestTable_Get_Set_Mixed(t *testing.T) {
	tab := NewTable(0, 0)

	t.Run("mixed integer and string keys", func(t *testing.T) {
		// Set array values
		tab.Set(*object.NewInteger(1), *object.NewString("array1"))
		tab.Set(*object.NewInteger(2), *object.NewString("array2"))

		// Set hash values
		tab.Set(*object.NewString("key"), *object.NewNumber(123.0))
		tab.Set(*object.NewString("name"), *object.NewString("test"))

		// Verify array access
		got1 := tab.Get(*object.NewInteger(1))
		if got1 == nil {
			t.Error("Get(1) returned nil")
		}

		got2 := tab.Get(*object.NewInteger(2))
		if got2 == nil {
			t.Error("Get(2) returned nil")
		}

		// Verify hash access
		gotKey := tab.Get(*object.NewString("key"))
		if gotKey == nil {
			t.Error("Get('key') returned nil")
		}

		gotName := tab.Get(*object.NewString("name"))
		if gotName == nil {
			t.Error("Get('name') returned nil")
		}

		// Verify IsArray flag
		if tab.IsArray {
			t.Error("Expected IsArray to be false for mixed table")
		}
	})

	t.Run("integer keys beyond array", func(t *testing.T) {
		tab := NewTable(0, 0)
		// Set consecutive array indices
		for i := 1; i <= 5; i++ {
			tab.Set(*object.NewInteger(int64(i)), *object.NewInteger(int64(i)))
		}

		// Set non-consecutive index (should still go to array)
		tab.Set(*object.NewInteger(10), *object.NewInteger(10))

		// Verify all values
		for i := 1; i <= 5; i++ {
			got := tab.Get(*object.NewInteger(int64(i)))
			if got == nil {
				t.Errorf("Get(%d) returned nil", i)
			}
		}

		got10 := tab.Get(*object.NewInteger(10))
		if got10 == nil {
			t.Error("Get(10) returned nil")
		}
	})
}

// TestTable_Len tests length calculation
func TestTable_Len(t *testing.T) {
	t.Run("empty table", func(t *testing.T) {
		tab := NewTable(0, 0)
		if tab.Len() != 0 {
			t.Errorf("Expected length 0, got %d", tab.Len())
		}
	})

	t.Run("consecutive array", func(t *testing.T) {
		tab := NewTable(0, 0)
		for i := 1; i <= 5; i++ {
			tab.SetI(i, *object.NewInteger(int64(i)))
		}
		if tab.Len() != 5 {
			t.Errorf("Expected length 5, got %d", tab.Len())
		}
	})

	t.Run("array with nil at end", func(t *testing.T) {
		tab := NewTable(0, 0)
		for i := 1; i <= 5; i++ {
			tab.SetI(i, *object.NewInteger(int64(i)))
		}
		// Set index 5 to nil
		tab.SetI(5, *object.NewNil())

		if tab.Len() != 4 {
			t.Errorf("Expected length 4 after setting index 5 to nil, got %d", tab.Len())
		}
	})

	t.Run("array with gap", func(t *testing.T) {
		tab := NewTable(0, 0)
		tab.SetI(1, *object.NewInteger(1))
		tab.SetI(2, *object.NewInteger(2))
		tab.SetI(3, *object.NewNil())
		tab.SetI(4, *object.NewInteger(4))

		// Lua's # operator finds the boundary; our implementation
		// finds the last non-nil value
		if tab.Len() != 4 {
			t.Errorf("Expected length 4, got %d", tab.Len())
		}
	})

	t.Run("hash only table", func(t *testing.T) {
		tab := NewTable(0, 0)
		tab.Set(*object.NewString("a"), *object.NewInteger(1))
		tab.Set(*object.NewString("b"), *object.NewInteger(2))

		// Length operator only counts array part
		if tab.Len() != 0 {
			t.Errorf("Expected length 0 for hash-only table, got %d", tab.Len())
		}
	})
}

// TestTable_Next tests iteration
func TestTable_Next(t *testing.T) {
	t.Run("empty table", func(t *testing.T) {
		tab := NewTable(0, 0)
		k, v := tab.Next(nil)
		if k != nil || v != nil {
			t.Error("Next on empty table should return (nil, nil)")
		}
	})

	t.Run("array iteration", func(t *testing.T) {
		tab := NewTable(0, 0)
		for i := 1; i <= 3; i++ {
			tab.SetI(i, *object.NewInteger(int64(i*10)))
		}

		count := 0
		for k, v := tab.Next(nil); k != nil; k, v = tab.Next(k) {
			count++
			if k.Type != object.TypeNumber {
				t.Errorf("Expected number key, got %v", k.Type)
			}
			if v == nil {
				t.Error("Expected non-nil value")
			}
		}

		if count != 3 {
			t.Errorf("Expected 3 iterations, got %d", count)
		}
	})

	t.Run("hash iteration", func(t *testing.T) {
		tab := NewTable(0, 0)
		tab.Set(*object.NewString("a"), *object.NewInteger(1))
		tab.Set(*object.NewString("b"), *object.NewInteger(2))
		tab.Set(*object.NewString("c"), *object.NewInteger(3))

		count := 0
		seenKeys := make(map[string]bool)
		for k, v := tab.Next(nil); k != nil; k, v = tab.Next(k) {
			count++
			if k.Type != object.TypeString {
				t.Errorf("Expected string key, got %v", k.Type)
			}
			s, ok := k.ToString()
			if ok {
				seenKeys[s] = true
			}
			if v == nil {
				t.Error("Expected non-nil value")
			}
		}

		if count != 3 {
			t.Errorf("Expected 3 iterations, got %d", count)
		}

		// Verify all keys were seen
		expectedKeys := map[string]bool{"a": true, "b": true, "c": true}
		for k := range expectedKeys {
			if !seenKeys[k] {
				t.Errorf("Key '%s' was not iterated", k)
			}
		}
	})

	t.Run("mixed iteration", func(t *testing.T) {
		tab := NewTable(0, 0)
		tab.SetI(1, *object.NewString("array1"))
		tab.SetI(2, *object.NewString("array2"))
		tab.Set(*object.NewString("key"), *object.NewNumber(100.0))

		count := 0
		for k, v := tab.Next(nil); k != nil; k, v = tab.Next(k) {
			count++
			if v == nil {
				t.Error("Expected non-nil value")
			}
		}

		if count != 3 {
			t.Errorf("Expected 3 iterations, got %d", count)
		}
	})

	t.Run("iteration starting from middle", func(t *testing.T) {
		tab := NewTable(0, 0)
		for i := 1; i <= 5; i++ {
			tab.SetI(i, *object.NewInteger(int64(i)))
		}

		// Start from key 2
		startKey := object.NewInteger(2)
		count := 0
		for k, _ := tab.Next(startKey); k != nil; k, _ = tab.Next(k) {
			count++
		}

		// Should iterate from 3 to 5 (3 elements)
		if count != 3 {
			t.Errorf("Expected 3 iterations starting from key 2, got %d", count)
		}
	})
}

// TestTable_Metatable tests metatable operations
func TestTable_Metatable(t *testing.T) {
	t.Run("initial metatable is nil", func(t *testing.T) {
		tab := NewTable(0, 0)
		if tab.GetMetatable() != nil {
			t.Error("Initial metatable should be nil")
		}
	})

	t.Run("set and get metatable", func(t *testing.T) {
		tab := NewTable(0, 0)
		mt := NewTable(0, 0)

		tab.SetMetatable(mt)
		got := tab.GetMetatable()

		if got == nil {
			t.Fatal("GetMetatable returned nil after SetMetatable")
		}
		if got != mt {
			t.Error("GetMetatable returned different table than set")
		}
	})

	t.Run("set nil metatable", func(t *testing.T) {
		tab := NewTable(0, 0)
		mt := NewTable(0, 0)
		tab.SetMetatable(mt)
		tab.SetMetatable(nil)

		if tab.GetMetatable() != nil {
			t.Error("Metatable should be nil after setting to nil")
		}
	})
}

// TestTable_IsEmpty tests IsEmpty method
func TestTable_IsEmpty(t *testing.T) {
	t.Run("empty table", func(t *testing.T) {
		tab := NewTable(0, 0)
		if !tab.IsEmpty() {
			t.Error("Expected table to be empty")
		}
	})

	t.Run("array with elements", func(t *testing.T) {
		tab := NewTable(0, 0)
		tab.SetI(1, *object.NewInteger(1))
		if tab.IsEmpty() {
			t.Error("Expected table to not be empty")
		}
	})

	t.Run("hash with elements", func(t *testing.T) {
		tab := NewTable(0, 0)
		tab.Set(*object.NewString("key"), *object.NewInteger(1))
		if tab.IsEmpty() {
			t.Error("Expected table to not be empty")
		}
	})
}

// TestTable_Clear tests Clear method
func TestTable_Clear(t *testing.T) {
	tab := NewTable(0, 0)
	tab.SetI(1, *object.NewInteger(1))
	tab.SetI(2, *object.NewInteger(2))
	tab.Set(*object.NewString("key"), *object.NewString("value"))

	mt := NewTable(0, 0)
	tab.SetMetatable(mt)

	tab.Clear()

	if !tab.IsEmpty() {
		t.Error("Expected table to be empty after Clear")
	}

	// Metatable should be preserved
	if tab.GetMetatable() != mt {
		t.Error("Expected metatable to be preserved after Clear")
	}
}

// TestTable_ValueKey tests valueKey creation and comparison
func TestTable_ValueKey(t *testing.T) {
	t.Run("same string keys produce same valueKey", func(t *testing.T) {
		key1 := object.NewString("test")
		key2 := object.NewString("test")

		vk1 := newValueKey(*key1)
		vk2 := newValueKey(*key2)

		if vk1 != vk2 {
			t.Error("Same string keys should produce same valueKey")
		}
	})

	t.Run("different string keys produce different valueKey", func(t *testing.T) {
		key1 := object.NewString("test1")
		key2 := object.NewString("test2")

		vk1 := newValueKey(*key1)
		vk2 := newValueKey(*key2)

		if vk1 == vk2 {
			t.Error("Different string keys should produce different valueKey")
		}
	})

	t.Run("same number keys produce same valueKey", func(t *testing.T) {
		key1 := object.NewNumber(42.0)
		key2 := object.NewNumber(42.0)

		vk1 := newValueKey(*key1)
		vk2 := newValueKey(*key2)

		if vk1 != vk2 {
			t.Error("Same number keys should produce same valueKey")
		}
	})

	t.Run("same boolean keys produce same valueKey", func(t *testing.T) {
		key1 := object.NewBoolean(true)
		key2 := object.NewBoolean(true)

		vk1 := newValueKey(*key1)
		vk2 := newValueKey(*key2)

		if vk1 != vk2 {
			t.Error("Same boolean keys should produce same valueKey")
		}
	})
}

// TestTable_KeyFromValueKey tests keyFromValueKey function
func TestTable_KeyFromValueKey(t *testing.T) {
	t.Run("nil key", func(t *testing.T) {
		vk := valueKey{Type: object.TypeNil}
		k := keyFromValueKey(vk)
		if k == nil || k.Type != object.TypeNil {
			t.Error("Expected nil key")
		}
	})

	t.Run("boolean key", func(t *testing.T) {
		vk := valueKey{Type: object.TypeBoolean, Num: 1}
		k := keyFromValueKey(vk)
		if k == nil || k.Type != object.TypeBoolean {
			t.Error("Expected boolean key")
		}
		b, ok := k.ToBoolean()
		if !ok || !b {
			t.Error("Expected true")
		}
	})

	t.Run("number key", func(t *testing.T) {
		vk := valueKey{Type: object.TypeNumber, Num: 3.14}
		k := keyFromValueKey(vk)
		if k == nil || k.Type != object.TypeNumber {
			t.Error("Expected number key")
		}
		n, ok := k.ToNumber()
		if !ok || n != 3.14 {
			t.Errorf("Expected 3.14, got %v", n)
		}
	})

	t.Run("string key", func(t *testing.T) {
		vk := valueKey{Type: object.TypeString, Str: "hello"}
		k := keyFromValueKey(vk)
		if k == nil || k.Type != object.TypeString {
			t.Error("Expected string key")
		}
		s, ok := k.ToString()
		if !ok || s != "hello" {
			t.Errorf("Expected 'hello', got '%v'", s)
		}
	})
}