package internal

import (
	"testing"

	luaapi "github.com/akzj/go-lua/api/api"
)

func TestPushGoFunctionAndCall(t *testing.T) {
	L := NewLuaState(nil)
	
	// Test 1: Push and call a Go function
	L.PushGoFunction(func(L luaapi.LuaAPI) int {
		arg, _ := L.ToInteger(1)
		L.PushInteger(arg * 2)
		return 1
	})
	L.PushInteger(21)
	L.Call(1, 1)
	result, _ := L.ToInteger(-1)
	
	if result != 42 {
		t.Errorf("Test1: Expected 42, got %d", result)
	}
	
	// Test 2: Multiple arguments
	L.Pop() // clean up result
	L.PushGoFunction(func(L luaapi.LuaAPI) int {
		a, _ := L.ToInteger(1)
		b, _ := L.ToInteger(2)
		L.PushInteger(a + b)
		return 1
	})
	L.PushInteger(10)
	L.PushInteger(32)
	L.Call(2, 1)
	result, _ = L.ToInteger(-1)
	
	if result != 42 {
		t.Errorf("Test2: Expected 42, got %d", result)
	}
}
