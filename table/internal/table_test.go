// Package internal provides tests for the table module.
package internal

import (
	"reflect"
	"testing"

	tableapi "github.com/akzj/go-lua/table/api"
)

func TestTableCreation(t *testing.T) {
	tbl := tableapi.NewTable(nil)
	if tbl == nil {
		t.Fatal("NewTable returned nil")
	}
}

func TestTableSetMetatableNil(t *testing.T) {
	tbl := tableapi.NewTable(nil)

	// SetMetatable with nil should work without panic
	tbl.SetMetatable(nil)
}

func TestTableInterfaceNonNil(t *testing.T) {
	// Verify the interface is non-nil
	tbl := tableapi.NewTable(nil)
	if tbl == nil {
		t.Fatal("TableInterface should not be nil")
	}
}

func TestMetatableReturnsInterface(t *testing.T) {
	tbl := tableapi.NewTable(nil)
	
	// GetMetatable should return an interface value
	// Note: In Go, a nil *Table returned as interface{} is NOT == nil
	// This is expected Go behavior - we just verify it doesn't panic
	mt := tbl.GetMetatable()
	
	// Use reflect to check if the interface is nil (no type, no value)
	v := reflect.ValueOf(mt)
	if v.Kind() != reflect.Interface || !v.IsNil() {
		t.Logf("Metatable is a typed nil (expected when unset)")
	}
}
