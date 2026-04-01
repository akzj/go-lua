package ldo

import (
	"testing"

	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

func TestGrowstack(t *testing.T) {
	L := &lstate.LuaState{
		Stack: make([]lobject.StackValue, 10),
	}
	L.StackLast.P = &L.Stack[5].Val
	L.Top.P = &L.Stack[2].Val
	
	oldsize := len(L.Stack)
	Growstack(L, 10)
	
	if len(L.Stack) <= oldsize {
		t.Error("Stack should have grown")
	}
}

func TestCheckstack(t *testing.T) {
	L := &lstate.LuaState{
		Stack: make([]lobject.StackValue, 100),
	}
	L.StackLast.P = &L.Stack[80].Val
	L.Top.P = &L.Stack[50].Val
	
	// This should not panic
	Checkstack(L, 10)
}

func TestYieldable(t *testing.T) {
	L := &lstate.LuaState{
		NCcalls: 0,
	}
	
	if !Yieldable(L) {
		t.Error("Thread should be yieldable")
	}
	
	L.NCcalls = 0x10000
	if Yieldable(L) {
		t.Error("Thread should not be yieldable with non-yieldable flag")
	}
}

func TestRawRunProtected(t *testing.T) {
	L := &lstate.LuaState{
		NCcalls: 0,
		Stack:   make([]lobject.StackValue, 10),
	}
	L.Top.P = &L.Stack[0].Val
	L.Status = lobject.LUA_OK
	
	called := false
	f := func(L *lstate.LuaState, ud interface{}) {
		called = true
	}
	
	if !RawRunProtected(L, f, nil) {
		t.Error("Protected call should succeed")
	}
	
	if !called {
		t.Error("Function should have been called")
	}
}

func TestRawRunProtectedWithPanic(t *testing.T) {
	L := &lstate.LuaState{
		NCcalls: 0,
		Stack:   make([]lobject.StackValue, 10),
	}
	L.Top.P = &L.Stack[0].Val
	L.Status = lobject.LUA_OK
	
	f := func(L *lstate.LuaState, ud interface{}) {
		panic(lobject.LUA_ERRRUN)
	}
	
	if RawRunProtected(L, f, nil) {
		t.Error("Protected call should fail with panic")
	}
	
	if L.Status != lobject.LUA_ERRRUN {
		t.Errorf("Expected status ERR_RUN, got %d", L.Status)
	}
}
