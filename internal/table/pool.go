// Table object pool — reuses dead Table structs to reduce allocation pressure.
//
// Tables are the most frequently allocated GC objects in Lua programs (~96% of
// allocations in GC benchmarks). Using sync.Pool lets us reuse the struct
// memory for short-lived tables instead of going through Go's mallocgc each time.
package table

import (
	"sync"

	"github.com/akzj/go-lua/internal/object"
)

var tablePool = sync.Pool{
	New: func() any {
		return &Table{}
	},
}

// getTable gets a Table from the pool or allocates a new one.
// PutTable already clears reference fields (Array, Nodes, Metatable, GCHeader).
// We only need to zero the scalar fields here.
func getTable() *Table {
	t := tablePool.Get().(*Table)
	// Reference fields (Array, Nodes, Metatable, GCHeader) were cleared by PutTable.
	// Zero the remaining scalar fields for safe reuse.
	t.LsizeNode = 0
	t.LastFree = 0
	t.Flags = 0
	t.WeakMode = 0
	t.SizeDelta = 0
	return t
}

// PutTable returns a Table to the pool for reuse.
// Called by the GC sweep phase when a dead table is unlinked.
// Clears all reference fields before pooling to help Go's GC.
func PutTable(t *Table) {
	// Clear all reference-bearing fields to avoid retaining garbage
	t.Array = nil
	t.Nodes = nil
	t.Metatable = nil
	t.GCHeader = object.GCHeader{}
	tablePool.Put(t)
}
