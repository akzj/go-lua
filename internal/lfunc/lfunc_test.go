package lfunc

import (
	"testing"

	"github.com/akzj/go-lua/internal/lstate"
)

func TestNewCClosure(t *testing.T) {
	L := &lstate.LuaState{}
	
	cl := NewCClosure(L, 3)
	if cl == nil {
		t.Fatal("NewCClosure should not return nil")
	}
	
	if cl.Nupvalues != 3 {
		t.Errorf("Expected 3 upvalues, got %d", cl.Nupvalues)
	}
}

func TestNewCClosureMaxUpval(t *testing.T) {
	L := &lstate.LuaState{}
	
	cl := NewCClosure(L, 300)
	if cl.Nupvalues != MAXUPVAL {
		t.Errorf("Expected %d upvalues (MAX), got %d", MAXUPVAL, cl.Nupvalues)
	}
}

func TestNewLClosure(t *testing.T) {
	L := &lstate.LuaState{}
	
	cl := NewLClosure(L, 2)
	if cl == nil {
		t.Fatal("NewLClosure should not return nil")
	}
	
	if cl.Nupvalues != 2 {
		t.Errorf("Expected 2 upvalues, got %d", cl.Nupvalues)
	}
	
	if len(cl.Upvals) != 2 {
		t.Errorf("Expected 2 upval slots, got %d", len(cl.Upvals))
	}
}

func TestNewProto(t *testing.T) {
	L := &lstate.LuaState{}
	
	p := NewProto(L)
	if p == nil {
		t.Fatal("NewProto should not return nil")
	}
}
