package internal

import (
	"testing"

	tableapi "github.com/akzj/go-lua/lib/table/api"
)

// TestNewTableLib tests creating a new TableLib instance.
func TestNewTableLib(t *testing.T) {
	lib := NewTableLib()
	if lib == nil {
		t.Error("NewTableLib() returned nil")
	}
}

// TestTableLibImplementsInterface tests that TableLib implements TableLib interface.
func TestTableLibImplementsInterface(t *testing.T) {
	var lib tableapi.TableLib = NewTableLib()
	if lib == nil {
		t.Error("TableLib does not implement tableapi.TableLib")
	}
}

// TestLuaFuncTypes tests that all functions have correct signature.
// These compile-time checks ensure all functions match LuaFunc signature
func TestLuaFuncTypes(t *testing.T) {
	var _ tableapi.LuaFunc = tblInsert
	var _ tableapi.LuaFunc = tblRemove
	var _ tableapi.LuaFunc = tblMove
	var _ tableapi.LuaFunc = tblConcat
	var _ tableapi.LuaFunc = tblSort
	var _ tableapi.LuaFunc = tblPack
	var _ tableapi.LuaFunc = tblUnpack
	var _ tableapi.LuaFunc = tblGetn
	var _ tableapi.LuaFunc = tblForeachi
	var _ tableapi.LuaFunc = tblForeach
}

// TestHelperFunctionsExist verifies helper functions exist with correct signatures.
func TestHelperFunctionsExist(t *testing.T) {
	// These are compile-time checks for helper function signatures
	// checkfield, checktab, aux_getn, typeError, sortComp, set2, partition, sortImpl
	_ = checkfield
	_ = checktab
	_ = aux_getn
	_ = typeError
	_ = sortComp
	_ = set2
	_ = partition
	_ = sortImpl
}
