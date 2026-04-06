package internal

import (
	"testing"
	"github.com/akzj/go-lua/state/api"
	"github.com/akzj/go-lua/types"
)

func TestRegistrySetGet(t *testing.T) {
	var L api.LuaStateInterface = NewLuaState(nil)
	registry := L.Global().Registry()
	
	if registry == nil {
		t.Fatal("Registry is nil!")
	}
	
	// Test setting a string key
	key := types.NewTValueString("x")
	val := types.NewTValueInteger(42)
	
	registry.Set(key, val)
	
	result := registry.Get(key)
	if result.IsNil() {
		t.Fatal("Get returned nil after Set")
	}
	if !result.IsInteger() {
		t.Fatalf("Expected integer, got type %d", result.GetTag())
	}
	if result.GetInteger() != 42 {
		t.Fatalf("Expected 42, got %d", result.GetInteger())
	}
	
	t.Logf("Registry Set/Get works: key='x' value=%d", result.GetInteger())
}
