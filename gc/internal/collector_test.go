package internal

import (
	"testing"

	"github.com/akzj/go-lua/gc/api"
)

func TestGCConstants(t *testing.T) {
	// Verify GC state constants are in expected order
	if api.GCSpropagate != 0 {
		t.Errorf("GCSpropagate = %d, want 0", api.GCSpropagate)
	}
	if api.GCSpause != 8 {
		t.Errorf("GCSpause = %d, want 8", api.GCSpause)
	}
}

func TestColorConstants(t *testing.T) {
	// White = 1 (bit 0), Black = 2 (bit 1) per Lua lgc.h
	if api.White != 1 {
		t.Errorf("White = %d, want 1", api.White)
	}
	if api.Black != 2 {
		t.Errorf("Black = %d, want 2", api.Black)
	}
}

func TestAgeConstants(t *testing.T) {
	if api.GNew != 0 {
		t.Errorf("GNew = %d, want 0", api.GNew)
	}
	if api.GOld != 4 {
		t.Errorf("GOld = %d, want 4", api.GOld)
	}
}

func TestHelperFunctions(t *testing.T) {
	// Test IsWhite - White = 1 (bit 0)
	if !api.IsWhite(1) { // marked with White bit
		t.Error("IsWhite(1) = false, want true")
	}
	if api.IsWhite(2) { // Black doesn't have White bit
		t.Error("IsWhite(2) = true, want false")
	}

	// Test IsBlack - Black = 2 (bit 1)
	if !api.IsBlack(2) { // marked with Black bit
		t.Error("IsBlack(2) = false, want true")
	}
	if api.IsBlack(1) { // White doesn't have Black bit
		t.Error("IsBlack(1) = true, want false")
	}

	// Test IsGray - neither white nor black
	if !api.IsGray(0) { // Gray = 0
		t.Error("IsGray(0) = false, want true")
	}
	if api.IsGray(1) { // White is not gray
		t.Error("IsGray(1) = true, want false")
	}

	// Test GetAge/SetAge
	var marked uint8 = 0
	marked = api.SetAge(marked, api.GOld)
	if api.GetAge(marked) != api.GOld {
		t.Errorf("GetAge(SetAge(0, GOld)) = %d, want GOld", api.GetAge(marked))
	}
}

func TestCollectorInterface(t *testing.T) {
	// Verify Collector implements GCCollector
	var _ api.GCCollector = (*Collector)(nil)
}
