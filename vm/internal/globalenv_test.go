package internal

import (
	"testing"
	"github.com/akzj/go-lua/table"
	tableapi "github.com/akzj/go-lua/table/api"
	"github.com/akzj/go-lua/types"
)

var _ = table.Lib

func TestExecutorGlobalEnvSetGet(t *testing.T) {
	registry := tableapi.NewTable(nil)
	if registry == nil {
		t.Fatal("Registry is nil!")
	}
	
	exec := NewVMFrameManager()
	exec.SetGlobalEnv(registry)
	
	keyInterface := types.NewTValueString("x")
	
	// Create internal TValue for key using extractVariantAndData pattern
	keyPtr := &TValue{}
	keyPtr.Tt = uint8(keyInterface.GetTag())
	keyVariant, keyData := extractVariantAndData(keyInterface)
	keyPtr.Value.Variant = keyVariant
	keyPtr.Value.Data_ = keyData
	
	// Create table pointer
	tablePtr := &TValue{
		Tt:    uint8(types.Ctb(int(types.LUA_VTABLE))),
		Value: Value{Variant: types.ValueGC, Data_: registry},
	}
	
	// Create integer value using the same pattern as extractVariantAndData
	valInterface := types.NewTValueInteger(42)
	valPtr := &TValue{}
	valPtr.Tt = uint8(valInterface.GetTag())
	valVariant, valData := extractVariantAndData(valInterface)
	valPtr.Value.Variant = valVariant
	valPtr.Value.Data_ = valData
	
	exec.PushFrame(&Frame{kvalues: make([]TValue, 0)})
	
	// Call finishSet
	exec.(*Executor).finishSet(tablePtr, keyPtr, valPtr)
	
	// Verify via direct registry access
	resultDirect := registry.Get(keyInterface)
	if resultDirect.IsNil() {
		t.Fatal("Direct registry.Get returned nil after finishSet")
	}
	t.Logf("Direct registry.Get works: key='x' value=%d", resultDirect.GetInteger())
	
	// Call finishGet
	resultPtr := &TValue{}
	exec.(*Executor).finishGet(resultPtr, tablePtr, keyPtr)
	
	if resultPtr.IsNil() {
		t.Fatal("finishGet returned nil!")
	}
	if !resultPtr.IsInteger() {
		t.Fatalf("Expected integer, got type %d", resultPtr.Tt)
	}
	if resultPtr.GetInteger() != 42 {
		t.Fatalf("Expected 42, got %d", resultPtr.GetInteger())
	}
	
	t.Logf("SUCCESS: finishSet/finishGet works: key='x' value=%d", resultPtr.GetInteger())
}
