package types

import (
	"testing"
)

func TestTValueTypeChecks(t *testing.T) {
	// Test nil value
	tv := NewTValueNil()
	if !tv.IsNil() {
		t.Error("expected IsNil() to be true")
	}

	// Test boolean values
	tvTrue := NewTValueBoolean(true)
	if !tvTrue.IsTrue() {
		t.Error("expected IsTrue() to be true")
	}
	if tvTrue.IsFalse() {
		t.Error("expected IsFalse() to be false for true value")
	}

	tvFalse := NewTValueBoolean(false)
	if !tvFalse.IsFalse() {
		t.Error("expected IsFalse() to be true")
	}

	// Test integer
	tvInt := NewTValueInteger(42)
	if !tvInt.IsInteger() {
		t.Error("expected IsInteger() to be true")
	}
	if !tvInt.IsNumber() {
		t.Error("expected IsNumber() to be true")
	}

	// Test float
	tvFloat := NewTValueFloat(3.14)
	if !tvFloat.IsFloat() {
		t.Error("expected IsFloat() to be true")
	}
}

func TestTableCreation(t *testing.T) {
	table := NewTable()
	if table == nil {
		t.Error("expected NewTable() to return non-nil")
	}
	if table.SizeNode() != 1 {
		t.Error("expected empty table to have SizeNode() == 0")
	}
}

func TestStringCreation(t *testing.T) {
	// Short string
	s := NewString("hello")
	if s == nil {
		t.Error("expected NewString() to return non-nil")
	}
	if s.Len() != 5 {
		t.Errorf("expected Len() == 5, got %d", s.Len())
	}
	if !s.IsShort() {
		t.Error("expected short string to return IsShort() == true")
	}

	// Long string
	longStr := NewString("this is a much longer string that exceeds short string limit")
	if longStr.IsShort() {
		t.Error("expected long string to return IsShort() == false")
	}
}

func TestTypeConstants(t *testing.T) {
	// Verify type constants are correctly defined
	if LUA_TNIL != 0 {
		t.Errorf("expected LUA_TNIL == 0, got %d", LUA_TNIL)
	}
	if LUA_TBOOLEAN != 1 {
		t.Errorf("expected LUA_TBOOLEAN == 1, got %d", LUA_TBOOLEAN)
	}
	if LUA_NUMTYPES != 8 {
		t.Errorf("expected LUA_NUMTYPES == 8, got %d", LUA_NUMTYPES)
	}

	// Verify variant constants
	if LUA_VFALSE != LUA_TBOOLEAN|(0<<4) {
		t.Errorf("LUA_VFALSE incorrect")
	}
	if LUA_VTRUE != LUA_TBOOLEAN|(1<<4) {
		t.Errorf("LUA_VTRUE incorrect")
	}
}

func TestUtilityFunctions(t *testing.T) {
	// Test MakeVariant
	v := MakeVariant(LUA_TNUMBER, 1)
	if v != LUA_VNUMFLT {
		t.Errorf("MakeVariant(LUA_TNUMBER, 1) = %d, want %d", v, LUA_VNUMFLT)
	}

	// Test Ctb (mark as collectable)
	ct := Ctb(LUA_VTABLE)
	if ct != LUA_VTABLE|BIT_ISCOLLECTABLE {
		t.Error("Ctb incorrect")
	}

	// Test Novariant
	nv := Novariant(LUA_VNUMFLT)
	if nv != LUA_TNUMBER {
		t.Errorf("Novariant(LUA_VNUMFLT) = %d, want %d", nv, LUA_TNUMBER)
	}
}
