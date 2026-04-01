package ltable

import (
	"testing"

	"github.com/akzj/go-lua/internal/lobject"
)

/*
** Test basic table creation
 */
func TestNewTable(t *testing.T) {
	t.Run("New function exists", func(t *testing.T) {
		// Verify New is not nil (function exists)
		_ = New
	})
}

/*
** Test CeilLog2 function
 */
func TestCeilLog2(t *testing.T) {
	cases := []struct {
		input    int
		expected int
	}{
		{1, 0},
		{2, 1},
		{3, 2},
		{4, 2},
		{5, 3},
		{8, 3},
		{9, 4},
		{16, 4},
		{17, 5},
		{32, 5},
	}

	for _, c := range cases {
		result := CeilLog2(c.input)
		if result != c.expected {
			t.Errorf("CeilLog2(%d) = %d, want %d", c.input, result, c.expected)
		}
	}
}

/*
** Test result constants
 */
func TestConstants(t *testing.T) {
	if HOK != 0 {
		t.Errorf("HOK should be 0, got %d", HOK)
	}
	if HNOTFOUND != 1 {
		t.Errorf("HNOTFOUND should be 1, got %d", HNOTFOUND)
	}
	if HNOTATABLE != 2 {
		t.Errorf("HNOTATABLE should be 2, got %d", HNOTATABLE)
	}
	if HFIRSTNODE != 3 {
		t.Errorf("HFIRSTNODE should be 3, got %d", HFIRSTNODE)
	}
}

/*
** Test dummy node initialization
 */
func TestDummyNode(t *testing.T) {
	// Verify dummyNode exists and is initialized
	if dummyNode.IVal.Tt_ != uint8(lobject.LUA_VNIL) {
		t.Error("dummyNode should be initialized to nil value")
	}
}

/*
** Test SizeNode function
 */
func TestSizeNode(t *testing.T) {
	// Create a minimal table struct to test SizeNode
	tbl := &lobject.Table{
		Asize:     0,
		Lsizenode: 0,
		Array:     nil,
		Node:      nil,
	}

	// Empty table (Lsizenode = 0)
	if SizeNode(tbl) != 0 {
		t.Error("SizeNode should return 0 for empty table")
	}

	// Table with Lsizenode = 1 -> size = 2
	tbl.Lsizenode = 1
	tbl.Node = make([]lobject.Node, 2)
	if SizeNode(tbl) != 2 {
		t.Errorf("SizeNode with Lsizenode=1 should be 2, got %d", SizeNode(tbl))
	}

	// Table with Lsizenode = 3 -> size = 8
	tbl.Lsizenode = 3
	if SizeNode(tbl) != 8 {
		t.Errorf("SizeNode with Lsizenode=3 should be 8, got %d", SizeNode(tbl))
	}
}

/*
** Test GetFreePos on empty table
 */
func TestGetFreePos(t *testing.T) {
	tbl := &lobject.Table{
		Lsizenode: 0,
		Node:      nil,
	}

	// Empty table should return nil
	if GetFreePos(tbl) != nil {
		t.Error("GetFreePos on empty table should return nil")
	}
}

/*
** Test SetNodevector
 */
func TestSetNodevector(t *testing.T) {
	t.Run("SetNodevector with size 0", func(t *testing.T) {
		tbl := &lobject.Table{
			Lsizenode: 3,
			Node:      make([]lobject.Node, 8),
		}
		SetNodevector(nil, tbl, 0)
		if tbl.Node != nil {
			t.Error("SetNodevector(0) should set Node to nil")
		}
		if tbl.Lsizenode != 0 {
			t.Error("SetNodevector(0) should set Lsizenode to 0")
		}
	})

	t.Run("SetNodevector with size > 0", func(t *testing.T) {
		tbl := &lobject.Table{}
		SetNodevector(nil, tbl, 5)
		if tbl.Node == nil {
			t.Error("SetNodevector(5) should create node array")
		}
		if tbl.Lsizenode == 0 && len(tbl.Node) > 0 {
			t.Error("SetNodevector should set non-zero Lsizenode for size > 0")
		}
	})
}

/*
** Test HashInt
 */
func TestHashInt(t *testing.T) {
	tbl := &lobject.Table{
		Lsizenode: 2, // size = 4
		Node:      make([]lobject.Node, 4),
	}

	// Hash should be within bounds
	hash := HashInt(tbl, 10)
	if hash < 0 || hash >= 4 {
		t.Errorf("HashInt should return 0-3, got %d", hash)
	}

	// Same key should produce same hash
	hash2 := HashInt(tbl, 10)
	if hash != hash2 {
		t.Error("HashInt should be deterministic")
	}
}

/*
** Test Resize function
 */
func TestResize(t *testing.T) {
	t.Run("Resize array part", func(t *testing.T) {
		tbl := &lobject.Table{
			Lsizenode: 0,
			Asize:     0,
			Array:     nil,
			Node:      nil,
		}

		// Resize to 4 slots
		Resize(nil, tbl, 4, 0)
		if tbl.Asize != 4 {
			t.Errorf("Asize should be 4 after resize, got %d", tbl.Asize)
		}
		if len(tbl.Array) != 4 {
			t.Errorf("Array length should be 4, got %d", len(tbl.Array))
		}

		// Resize down to 2
		Resize(nil, tbl, 2, 0)
		if tbl.Asize != 2 {
			t.Errorf("Asize should be 2 after shrink, got %d", tbl.Asize)
		}
	})
}