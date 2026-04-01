package lundump

import (
	"testing"

	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
	"github.com/akzj/go-lua/internal/lzio"
)

func TestLoadConstants(t *testing.T) {
	// Create a minimal test setup
	L := &lstate.LuaState{}
	Z := &lzio.ZIO{}
	f := &lobject.Proto{}

	// Test LoadConstants
	err := LoadConstants(L, Z, f)
	if err != nil {
		t.Errorf("LoadConstants returned error: %v", err)
	}

	// Note: f.K is now []TValue (a slice), so it should be properly sized
	// The stub loadInt returns 0, so K should have length 0
	if len(f.K) != 0 {
		t.Errorf("f.K should have length 0 (stub returns 0), got %d", len(f.K))
	}
}

func TestLoadFunctions(t *testing.T) {
	L := &lstate.LuaState{}
	Z := &lzio.ZIO{}
	f := &lobject.Proto{}

	err := LoadFunctions(L, Z, f)
	if err != nil {
		t.Errorf("LoadFunctions returned error: %v", err)
	}

	// f.P should be allocated (even if stub returns 0 elements)
	if f.P == nil {
		t.Error("f.P should not be nil after LoadFunctions")
	}
}

func TestLoadDebug(t *testing.T) {
	L := &lstate.LuaState{}
	Z := &lzio.ZIO{}
	f := &lobject.Proto{}

	err := LoadDebug(L, Z, f)
	if err != nil {
		t.Errorf("LoadDebug returned error: %v", err)
	}
}

func TestUndump(t *testing.T) {
	L := &lstate.LuaState{}
	Z := &lzio.ZIO{}
	name := "test"

	// Undump is currently a stub that returns nil
	result := Undump(L, Z, name)
	if result != nil {
		t.Error("Undump stub should return nil")
	}
}