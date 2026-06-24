package mem

import (
	"testing"
)

func TestAllocSlab(t *testing.T) {
	type testStruct struct {
		A int64
		B int64
		C [4]byte
	}

	n := 4096
	slab := AllocSlab[testStruct](n)
	if slab == nil {
		t.Fatal("AllocSlab returned nil")
	}
	if len(slab) != n {
		t.Fatalf("len = %d, want %d", len(slab), n)
	}
	if cap(slab) != n {
		t.Fatalf("cap = %d, want %d", cap(slab), n)
	}

	// Verify elements are accessible and writable.
	for i := 0; i < n; i++ {
		slab[i].A = int64(i)
		slab[i].B = int64(i * 2)
	}

	// Verify values persist.
	for i := 0; i < n; i++ {
		if slab[i].A != int64(i) {
			t.Fatalf("slab[%d].A = %d, want %d", i, slab[i].A, i)
		}
	}

	// Free and re-allocate to verify no panic.
	FreeSlab(slab)
	slab2 := AllocSlab[testStruct](100)
	if slab2 == nil {
		t.Fatal("AllocSlab returned nil on second alloc")
	}
	FreeSlab(slab2)
}

func TestAllocSlabZero(t *testing.T) {
	slab := AllocSlab[byte](0)
	if slab == nil {
		t.Fatal("AllocSlab(0) returned nil")
	}
	if len(slab) != 0 {
		t.Fatalf("len = %d, want 0", len(slab))
	}
	FreeSlab(slab) // should not panic
}

func TestAllocSlabLarge(t *testing.T) {
	// Allocate a slab of 100k structs (~2.4MB) to stress test.
	type bigStruct struct {
		Data [32]byte
		ID   int64
	}
	n := 100_000
	slab := AllocSlab[bigStruct](n)
	if slab == nil {
		// mmap might fail on constrained systems — skip, don't fail.
		t.Skip("AllocSlab returned nil (mmap may not be available)")
	}
	if len(slab) != n {
		t.Fatalf("len = %d, want %d", len(slab), n)
	}
	// Touch all pages to verify mapping is valid.
	for i := 0; i < n; i++ {
		slab[i].ID = int64(i)
	}
	FreeSlab(slab)
}
