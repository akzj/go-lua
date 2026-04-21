package table

import (
	"testing"

	"github.com/akzj/go-lua/internal/object"
)

func TestComputeSizesDynamic(t *testing.T) {
	// Reproduce: add elements 1..k one at a time via SetInt
	// With ResizeArray fix in OP_SETLIST, this path still uses rehash.
	// But the nextvar.lua test uses {table.unpack(a,1,k)} which goes through
	// OP_SETLIST, which now pre-resizes. Test the direct SetInt path here
	// to ensure rehash still works correctly.
	sizes := []struct {
		k        int
		wantArr  int
		wantHash int
	}{
		{1, 1, 0},
		{2, 2, 0},
		{4, 4, 0},
		{8, 8, 0},
		{16, 16, 0},
	}
	for _, tc := range sizes {
		tbl := newTable(0, 0)
		for i := int64(1); i <= int64(tc.k); i++ {
			tbl.setInt(i, object.MakeInteger(i))
		}
		aLen := len(tbl.Array)
		hLen := tbl.HashLen()
		if aLen != tc.wantArr || hLen != tc.wantHash {
			t.Errorf("k=%d: got (%d, %d), want (%d, %d)", tc.k, aLen, hLen, tc.wantArr, tc.wantHash)
		}
	}
}

func TestResizeArray(t *testing.T) {
	// Test ResizeArray directly
	tbl := newTable(0, 0)
	tbl.ResizeArray(5)
	if len(tbl.Array) != 5 {
		t.Errorf("ResizeArray(5): got %d, want 5", len(tbl.Array))
	}
	// Set values 1..5
	for i := int64(1); i <= 5; i++ {
		tbl.setInt(i, object.MakeInteger(i))
	}
	if len(tbl.Array) != 5 {
		t.Errorf("After setting 1-5: got %d, want 5", len(tbl.Array))
	}
	// Verify values
	for i := int64(1); i <= 5; i++ {
		v, ok := tbl.getInt(i)
		if !ok || v.N != i {
			t.Errorf("a[%d]: got %v, ok=%v", i, v, ok)
		}
	}
}

func TestComputeSizesRehash(t *testing.T) {
	tbl := newTable(0, 0)
	for i := int64(1); i <= 100; i++ {
		tbl.setInt(i, object.MakeInteger(i))
	}
	for i := int64(5); i <= 95; i++ {
		tbl.setInt(i, object.Nil)
	}
	tbl.setInt(129, object.MakeInteger(1))

	aLen := len(tbl.Array)
	hLen := tbl.HashLen()
	if aLen != 4 || hLen != 8 {
		t.Errorf("Expected (4, 8), got (%d, %d)", aLen, hLen)
	}
}
