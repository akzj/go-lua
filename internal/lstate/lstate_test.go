package lstate

import (
	"testing"
)

func TestCallInfo(t *testing.T) {
	ci := &CallInfo{}
	
	// Initially 0 - should be Lua function
	if !ci.IsLua() {
		t.Error("Zero CallStatus should indicate Lua function")
	}
	
	// Set CIST_C flag - should indicate C function
	ci.CallStatus = CIST_C
	if ci.IsLua() {
		t.Error("CIST_C flag should indicate C function")
	}
}

func TestCallInfoIsLuacode(t *testing.T) {
	ci := &CallInfo{}
	
	// Lua code without hook
	if !ci.IsLuacode() {
		t.Error("Zero status should be Lua code")
	}
	
	// C function
	ci.CallStatus = CIST_C
	if ci.IsLuacode() {
		t.Error("C function should not be Lua code")
	}
}

func TestYieldable(t *testing.T) {
	L := &LuaState{
		NCcalls: 0,
	}
	
	if !Yieldable(L) {
		t.Error("New thread should be yieldable")
	}
	
	L.NCcalls = 0x10000 // Non-yieldable flag set
	if Yieldable(L) {
		t.Error("Thread with non-yieldable flag should not be yieldable")
	}
}

func TestGetCcalls(t *testing.T) {
	L := &LuaState{
		NCcalls: 100,
	}
	
	if GetCcalls(L) != 100 {
		t.Errorf("Expected 100 C calls, got %d", GetCcalls(L))
	}
	
	// With non-yieldable flag
	L.NCcalls = 0x10050 // 80 + non-yieldable
	if GetCcalls(L) != 80 {
		t.Errorf("Expected 80 C calls, got %d", GetCcalls(L))
	}
}

func TestIncnnyDecnny(t *testing.T) {
	L := &LuaState{NCcalls: 0}
	
	Incnny(L)
	if L.NCcalls != 0x10000 {
		t.Errorf("Expected 0x10000, got 0x%x", L.NCcalls)
	}
	
	Decnny(L)
	if L.NCcalls != 0 {
		t.Errorf("Expected 0, got 0x%x", L.NCcalls)
	}
}

func TestGetnresults(t *testing.T) {
	// nresults encoded as status + 1
	if Getnresults(1) != 0 {
		t.Errorf("Getnresults(1) should be 0")
	}
	if Getnresults(5) != 4 {
		t.Errorf("Getnresults(5) should be 4")
	}
}
