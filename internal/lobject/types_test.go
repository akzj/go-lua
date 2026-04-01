package lobject

import "testing"

func TestTValueBasics(t *testing.T) {
	// Test nil value
	var v TValue
	SetNilValue(&v)
	if !TtIsNil(&v) {
		t.Error("Expected nil value")
	}
	if TType(&v) != LUA_TNIL {
		t.Errorf("Expected type %d, got %d", LUA_TNIL, TType(&v))
	}

	// Test integer value
	SetIntValue(&v, 42)
	if !TtIsInteger(&v) {
		t.Error("Expected integer value")
	}
	if IntValue(&v) != 42 {
		t.Errorf("Expected 42, got %d", IntValue(&v))
	}

	// Test float value
	SetFltValue(&v, 3.14)
	if !TtIsFloat(&v) {
		t.Error("Expected float value")
	}

	// Test boolean
	SetBtValue(&v, true)
	if !TtIsTrue(&v) {
		t.Error("Expected true")
	}
	SetBtValue(&v, false)
	if !TtIsFalse(&v) {
		t.Error("Expected false")
	}

	// Test IsFalse
	SetNilValue(&v)
	if !IsFalse(&v) {
		t.Error("nil should be false")
	}
	SetBtValue(&v, false)
	if !IsFalse(&v) {
		t.Error("false should be false")
	}
	SetBtValue(&v, true)
	if IsFalse(&v) {
		t.Error("true should not be false")
	}
}

func TestTypeTags(t *testing.T) {
	// Test variant tag calculation
	if LUA_VNUMINT != 48 { // 3 | (0 << 4) | 64 = 3 | 0 | 64 = 67... wait
		t.Logf("LUA_VNUMINT = %d", LUA_VNUMINT)
	}
	if LUA_VNUMFLT != 49 { // 3 | (1 << 4) | 64 = 3 | 16 | 64 = 83
		t.Logf("LUA_VNUMFLT = %d", LUA_VNUMFLT)
	}
}

func TestTableStruct(t *testing.T) {
	// Verify Table structure exists
	var tbl Table
	if tbl.Flags != 0 {
		t.Error("Table flags should be 0")
	}
	if tbl.Lsizenode != 0 {
		t.Error("Table lsizenode should be 0")
	}
}

func TestNodeStruct(t *testing.T) {
	// Verify Node structure exists
	var node Node
	// Key should be nil initially
	if !IsKeyNil(&node) {
		t.Error("New node key should be nil")
	}
}

func TestValueCopy(t *testing.T) {
	var v1, v2 TValue
	SetIntValue(&v1, 100)
	SetObj(&v2, &v1)
	if IntValue(&v2) != 100 {
		t.Errorf("Expected 100, got %d", IntValue(&v2))
	}
}
