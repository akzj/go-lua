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
// The returned Table is zero-valued (all fields cleared).
func getTable() *Table {
	t := tablePool.Get().(*Table)
	// CRITICAL: zero out ALL fields for safe reuse.
	// A reused table with stale pointers would corrupt the GC.
	*t = Table{}
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
