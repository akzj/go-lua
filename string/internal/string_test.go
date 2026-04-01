package internal

import (
	"testing"

	stringapi "github.com/akzj/go-lua/string/api"
)

func TestNewString_ShortString(t *testing.T) {
	st := NewStringTable()
	
	ts := st.NewString("hello")
	if ts == nil {
		t.Fatal("expected non-nil TString")
	}
	
	// Check length (excluding null terminator)
	if ts.Len() != 5 {
		t.Errorf("expected len 5, got %d", ts.Len())
	}
	if !ts.IsShort() {
		t.Error("expected IsShort() = true")
	}
}

func TestNewString_LongString(t *testing.T) {
	st := NewStringTable()
	
	longStr := "this is a very long string that exceeds the maximum short string length limit"
	ts := st.NewString(longStr)
	if ts == nil {
		t.Fatal("expected non-nil TString for long string")
	}
	if ts.IsShort() {
		t.Error("expected IsShort() = false for long string")
	}
}

func TestNewString_Interning(t *testing.T) {
	st := NewStringTable()
	
	// Same string should return same pointer (interning)
	ts1 := st.NewString("interned")
	ts2 := st.NewString("interned")
	
	if ts1 != ts2 {
		t.Errorf("expected same pointer for interned strings, got %p vs %p", ts1, ts2)
	}
	
	// Different string should return different pointer
	ts3 := st.NewString("different")
	if ts1 == ts3 {
		t.Error("expected different pointer for different strings")
	}
}

func TestInterned(t *testing.T) {
	st := NewStringTable()
	
	// Create and check interned
	ts := st.NewString("test")
	if ts == nil {
		t.Fatal("NewString returned nil")
	}
	
	// Check Interned
	if !st.Interned("test") {
		t.Error("expected 'test' to be interned")
	}
	
	if st.Interned("notexist") {
		t.Error("expected 'notexist' to not be interned")
	}
	
	// Long strings are never interned
	longStr := "this is a very long string that exceeds 40 characters"
	st.NewString(longStr)
	if st.Interned(longStr) {
		t.Error("expected long string to not be interned")
	}
}

func TestGetString(t *testing.T) {
	st := NewStringTable()
	
	original := st.NewString("getme")
	if original == nil {
		t.Fatal("NewString returned nil")
	}
	
	retrieved := st.GetString("getme")
	if retrieved == nil {
		t.Fatal("expected non-nil result")
	}
	if retrieved != original {
		t.Error("expected same pointer")
	}
	
	if st.GetString("notexist") != nil {
		t.Error("expected nil for non-existent string")
	}
}

func TestEqualStrings(t *testing.T) {
	st := NewStringTable()
	
	ts1 := st.NewString("equal")
	ts2 := st.NewString("equal")
	ts3 := st.NewString("notequal")
	
	if !EqualStrings(ts1, ts2) {
		t.Error("expected EqualStrings(ts1, ts2) = true")
	}
	
	if EqualStrings(ts1, ts3) {
		t.Error("expected EqualStrings(ts1, ts3) = false")
	}
}

func TestIsReservedWord(t *testing.T) {
	if !IsReservedWord("function") {
		t.Error("expected 'function' to be a reserved word")
	}
	if !IsReservedWord("local") {
		t.Error("expected 'local' to be a reserved word")
	}
	if IsReservedWord("notreserved") {
		t.Error("expected 'notreserved' to not be a reserved word")
	}
}

func TestDefaultStringTable(t *testing.T) {
	if stringapi.DefaultStringTable == nil {
		t.Error("expected DefaultStringTable to be initialized")
	}
	
	ts := stringapi.DefaultStringTable.NewString("default")
	if ts == nil {
		t.Error("expected non-nil from DefaultStringTable")
	}
}
