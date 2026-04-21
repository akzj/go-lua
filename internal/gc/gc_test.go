package gc

import (
	"runtime"
	"testing"
	"weak"

	objectapi "github.com/akzj/go-lua/internal/object"
	stateapi "github.com/akzj/go-lua/internal/state"
)

func TestWeakPointerAfterSweep(t *testing.T) {
	// Create a minimal GlobalState with allgc chain
	g := &stateapi.GlobalState{}
	g.CurrentWhite = objectapi.WhiteBit0
	g.GCState = objectapi.GCSpause

	// Create an object and link it into allgc
	s := &objectapi.LuaString{Data: "hello"}
	g.LinkGC(s)

	// Create a weak pointer to track it
	wp := weak.Make(s)

	// Verify it's on the chain
	if g.Allgc == nil {
		t.Fatal("allgc chain is empty after LinkGC")
	}

	// Drop our local reference — only allgc holds it now
	s = nil

	// Run Lua GC — no roots set, so nothing is marked, everything is swept
	FullGC(g, nil)

	// Verify allgc is now empty (object was swept)
	if g.Allgc != nil {
		t.Error("allgc chain not empty after sweep — dead object not collected")
	}

	// Run Go GC to actually free the memory
	runtime.GC()
	runtime.GC()

	if wp.Value() != nil {
		t.Error("weak pointer still alive after Lua sweep + Go GC")
	}
}

func TestSweepPreservesLiveObjects(t *testing.T) {
	// Create a full Lua state
	L := stateapi.NewState()
	g := L.Global

	// Count objects on allgc chain
	count := 0
	for obj := g.Allgc; obj != nil; obj = obj.GC().Next {
		count++
	}
	if count == 0 {
		t.Fatal("no objects on allgc chain after NewState")
	}
	t.Logf("objects on allgc before GC: %d", count)

	// Run full GC — all objects should survive (they're reachable from roots)
	FullGC(g, L)

	// Count again
	countAfter := 0
	for obj := g.Allgc; obj != nil; obj = obj.GC().Next {
		countAfter++
	}
	t.Logf("objects on allgc after GC: %d", countAfter)

	if countAfter == 0 {
		t.Error("all objects swept — mark phase not working")
	}
	if countAfter < count/2 {
		t.Errorf("too many objects swept: before=%d after=%d", count, countAfter)
	}
}
