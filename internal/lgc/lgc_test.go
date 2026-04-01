package lgc

/*
** $Id: lgc_test.go $
** Unit tests for lgc
*/

import (
	"testing"

	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

func TestBitmask(t *testing.T) {
	tests := []struct {
		input    int
		expected lobject.LuByte
	}{
		{0, 1},
		{1, 2},
		{3, 8},
		{4, 16},
		{5, 32},
		{6, 64},
		{7, 128},
	}

	for _, tc := range tests {
		result := bitmask(tc.input)
		if result != tc.expected {
			t.Errorf("bitmask(%d) expected %d, got %d", tc.input, tc.expected, result)
		}
	}
}

func TestBit2mask(t *testing.T) {
	tests := []struct {
		b1, b2   int
		expected lobject.LuByte
	}{
		{3, 4, 8 | 16},   // WHITE0BIT | WHITE1BIT
		{0, 1, 1 | 2},
		{5, 6, 32 | 64},
	}

	for _, tc := range tests {
		result := bit2mask(tc.b1, tc.b2)
		if result != tc.expected {
			t.Errorf("bit2mask(%d, %d) expected %d, got %d", tc.b1, tc.b2, tc.expected, result)
		}
	}
}

func TestColorConstants(t *testing.T) {
	// Verify color bit positions
	if WHITE0BIT != 3 {
		t.Errorf("WHITE0BIT expected 3, got %d", WHITE0BIT)
	}
	if WHITE1BIT != 4 {
		t.Errorf("WHITE1BIT expected 4, got %d", WHITE1BIT)
	}
	if BLACKBIT != 5 {
		t.Errorf("BLACKBIT expected 5, got %d", BLACKBIT)
	}
	if FINALIZEDBIT != 6 {
		t.Errorf("FINALIZEDBIT expected 6, got %d", FINALIZEDBIT)
	}
}

func TestGCConstants(t *testing.T) {
	// Verify GC state constants
	if GCSpause != 8 {
		t.Errorf("GCSpause expected 8, got %d", GCSpause)
	}
	if GCSpropagate != 0 {
		t.Errorf("GCSpropagate expected 0, got %d", GCSpropagate)
	}
	if GCSswpend != 6 {
		t.Errorf("GCSswpend expected 6, got %d", GCSswpend)
	}
}

func TestIswhite(t *testing.T) {
	// An object is white if it has WHITE0BIT or WHITE1BIT set
	o := &lobject.GCObject{Marked: 8} // WHITE0BIT set
	if !iswhite(o) {
		t.Error("Object with WHITE0BIT should be white")
	}

	o = &lobject.GCObject{Marked: 16} // WHITE1BIT set
	if !iswhite(o) {
		t.Error("Object with WHITE1BIT should be white")
	}

	o = &lobject.GCObject{Marked: 0}
	if iswhite(o) {
		t.Error("Object with no white bits should not be white")
	}
}

func TestIsblack(t *testing.T) {
	o := &lobject.GCObject{Marked: 32} // BLACKBIT set
	if !isblack(o) {
		t.Error("Object with BLACKBIT should be black")
	}

	o = &lobject.GCObject{Marked: 0}
	if isblack(o) {
		t.Error("Object without BLACKBIT should not be black")
	}
}

func TestIsgray(t *testing.T) {
	// Gray is neither white nor black
	o := &lobject.GCObject{Marked: 0}
	if !isgray(o) {
		t.Error("Object with no white or black bits should be gray")
	}

	o = &lobject.GCObject{Marked: 8} // white
	if isgray(o) {
		t.Error("White object should not be gray")
	}

	o = &lobject.GCObject{Marked: 32} // black
	if isgray(o) {
		t.Error("Black object should not be gray")
	}
}

func TestTofinalize(t *testing.T) {
	o := &lobject.GCObject{Marked: 64} // FINALIZEDBIT set
	if !tofinalize(o) {
		t.Error("Object with FINALIZEDBIT should be finalized")
	}

	o = &lobject.GCObject{Marked: 0}
	if tofinalize(o) {
		t.Error("Object without FINALIZEDBIT should not be finalized")
	}
}

func TestChangewhite(t *testing.T) {
	o := &lobject.GCObject{Marked: 8} // WHITE0BIT
	changewhite(o)
	// After change, it should have WHITE1BIT instead
	if o.Marked&16 == 0 {
		t.Error("changewhite should toggle white bits")
	}
}

func TestMakewhite(t *testing.T) {
	g := &lstate.GlobalState{CurrentWhite: 8} // WHITE0BIT
	o := &lobject.GCObject{Marked: 32}      // BLACK

	makewhite(g, o)
	// Should now be white (WHITE0BIT)
	if o.Marked&8 == 0 {
		t.Error("makewhite should set current white bit")
	}
}

func TestSet2gray(t *testing.T) {
	o := &lobject.GCObject{Marked: 8} // WHITE0BIT
	set2gray(o)
	if o.Marked&8 != 0 || o.Marked&16 != 0 {
		t.Error("set2gray should clear white bits")
	}
}

func TestSet2black(t *testing.T) {
	o := &lobject.GCObject{Marked: 8} // WHITE0BIT
	set2black(o)
	if o.Marked&32 == 0 {
		t.Error("set2black should set BLACKBIT")
	}
}

func TestValiswhite(t *testing.T) {
	// Test with collectable white object
	o := &lobject.TValue{}
	o.Value_.Gc = &lobject.GCObject{Marked: 8} // white
	o.Tt_ = lobject.LuByte(lobject.CTb(lobject.LUA_TTABLE))

	if !valiswhite(o) {
		t.Error("Collectable white value should be valiswhite")
	}

	// Test with non-collectable
	o2 := &lobject.TValue{}
	lobject.SetIntValue(o2, 42)
	if valiswhite(o2) {
		t.Error("Non-collectable value should not be valiswhite")
	}

	// Test with black object
	o.Value_.Gc = &lobject.GCObject{Marked: 32} // black
	if valiswhite(o) {
		t.Error("Collectable black value should not be valiswhite")
	}
}

func TestLinkobjgclist(t *testing.T) {
	o := &lobject.GCObject{Tt: lobject.LUA_VTABLE}
	list := &lobject.GCObject{Tt: lobject.LUA_VTHREAD}

	linkobjgclist(o, &list)

	// o should now be first in list
	if list != o {
		t.Error("linkobjgclist should link object to front of list")
	}

	// o should be gray
	if !isgray(o) {
		t.Error("linked object should be gray")
	}
}

func TestReallymarkobject(t *testing.T) {
	g := &lstate.GlobalState{
		Allgc:     nil,
		Gray:      nil,
		GrayAgain: nil,
	}

	// Mark a table
	o := &lobject.GCObject{Tt: lobject.LUA_VTABLE}
	reallymarkobject(g, o)

	if g.Gray != o {
		t.Error("Table should be added to gray list")
	}
}

func TestOtherwhite(t *testing.T) {
	// otherwhite toggles between WHITE0BIT and WHITE1BIT
	g := &lstate.GlobalState{CurrentWhite: 8} // WHITE0BIT
	result := otherwhite(g)
	if result != 16 {
		t.Errorf("otherwhite with WHITE0BIT should return WHITE1BIT, got %d", result)
	}

	g.CurrentWhite = 16 // WHITE1BIT
	result = otherwhite(g)
	if result != 8 {
		t.Errorf("otherwhite with WHITE1BIT should return WHITE0BIT, got %d", result)
	}
}

func TestIsdeadm(t *testing.T) {
	// isdeadm(ow, m) returns true if m & ow != 0
	if !isdeadm(8, 8) {
		t.Error("isdeadm(8, 8) should be true")
	}
	if isdeadm(8, 16) {
		t.Error("isdeadm(8, 16) should be false")
	}
	if !isdeadm(24, 8) {
		t.Error("isdeadm(24, 8) should be true (24 includes 8)")
	}
}

func TestNwb2black(t *testing.T) {
	o := &lobject.GCObject{Marked: 8} // white
	nw2black(o)
	if o.Marked&32 == 0 {
		t.Error("nw2black should set BLACKBIT")
	}
}

func TestLuaC_White(t *testing.T) {
	g := &lstate.GlobalState{CurrentWhite: 8} // WHITE0BIT
	result := luaC_white(g)
	if result != 8 {
		t.Errorf("luaC_white expected 8, got %d", result)
	}

	g.CurrentWhite = 16 // WHITE1BIT
	result = luaC_white(g)
	if result != 16 {
		t.Errorf("luaC_white expected 16, got %d", result)
	}
}

func TestMarkvalue(t *testing.T) {
	g := &lstate.GlobalState{
		Allgc:     nil,
		Gray:      nil,
		GrayAgain: nil,
	}

	// Create a collectable value
	o := &lobject.GCObject{Tt: lobject.LUA_VTABLE, Marked: 8} // white
	val := &lobject.TValue{}
	val.Value_.Gc = o
	val.Tt_ = lobject.LuByte(lobject.CTb(lobject.LUA_TTABLE))

	markvalue(g, val)

	if g.Gray != o {
		t.Error("markvalue should add white collectable to gray list")
	}
}

func TestMarkobject(t *testing.T) {
	g := &lstate.GlobalState{
		Allgc:     nil,
		Gray:      nil,
		GrayAgain: nil,
	}

	o := &lobject.GCObject{Tt: lobject.LUA_VTABLE, Marked: 8} // white
	markobject(g, o)

	if g.Gray != o {
		t.Error("markobject should add white object to gray list")
	}

	// Reset
	g.Gray = nil
	o2 := &lobject.GCObject{Tt: lobject.LUA_VTABLE, Marked: 32} // black
	markobject(g, o2)

	if g.Gray != nil {
		t.Error("markobject should not add black object to gray list")
	}
}

func TestCWUFIN(t *testing.T) {
	if CWUFIN != 10 {
		t.Errorf("CWUFIN expected 10, got %d", CWUFIN)
	}
}

func TestGCSWEEPMAX(t *testing.T) {
	if GCSWEEPMAX != 20 {
		t.Errorf("GCSWEEPMAX expected 20, got %d", GCSWEEPMAX)
	}
}