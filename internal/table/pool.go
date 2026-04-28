// Table and slice pools — reuses dead Table structs and backing slices
// to reduce allocation pressure.
//
// Tables are the most frequently allocated GC objects in Lua programs (~96% of
// allocations in GC benchmarks). Using sync.Pool lets us reuse the struct
// memory for short-lived tables instead of going through Go's mallocgc each time.
//
// Slice pools index by log2(capacity) so that power-of-2 sized slices are
// reused across tables of similar sizes.
package table

import (
	"math/bits"
	"sync"

	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// Table struct pool
// ---------------------------------------------------------------------------

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
// Pools backing slices before clearing references.
func PutTable(t *Table) {
	// Pool the backing slices for reuse
	if t.Array != nil {
		putArraySlice(t.Array)
	}
	if t.Nodes != nil {
		putNodeSlice(t.Nodes)
	}
	// Clear all reference-bearing fields to avoid retaining garbage
	t.Array = nil
	t.Nodes = nil
	t.Metatable = nil
	t.GCHeader = object.GCHeader{}
	tablePool.Put(t)
}

// ---------------------------------------------------------------------------
// Slice pools — indexed by log2(capacity)
// ---------------------------------------------------------------------------

// arrayPools[i] pools []object.TValue slices with cap = 1<<i
// nodePools[i] pools []node slices with cap = 1<<i
var (
	arrayPools [32]sync.Pool
	nodePools  [32]sync.Pool
)

// getArraySlice returns a zeroed []TValue slice of the given size.
// The underlying capacity is rounded up to the next power of 2.
func getArraySlice(size int) []object.TValue {
	if size <= 0 {
		return nil
	}
	i := poolIndex(size)
	if v := arrayPools[i].Get(); v != nil {
		s := v.([]object.TValue)[:size]
		// Clear to zero (Nil = zero value, TagNil=0x00)
		for j := range s {
			s[j] = object.TValue{}
		}
		return s
	}
	return make([]object.TValue, size, 1<<i)
}

// putArraySlice returns a TValue slice to the pool for reuse.
// Clears all elements to release GC references before pooling.
func putArraySlice(s []object.TValue) {
	if cap(s) == 0 {
		return
	}
	// Clear all capacity to release GC references
	s = s[:cap(s)]
	for i := range s {
		s[i] = object.TValue{}
	}
	i := poolIndex(cap(s))
	arrayPools[i].Put(s)
}

// getNodeSlice returns a zeroed []node slice of the given size.
// The underlying capacity is rounded up to the next power of 2.
func getNodeSlice(size int) []node {
	if size <= 0 {
		return nil
	}
	i := poolIndex(size)
	if v := nodePools[i].Get(); v != nil {
		s := v.([]node)[:size]
		// Clear to zero
		for j := range s {
			s[j] = node{}
		}
		return s
	}
	return make([]node, size, 1<<i)
}

// putNodeSlice returns a node slice to the pool for reuse.
// Clears all elements to release GC references before pooling.
func putNodeSlice(s []node) {
	if cap(s) == 0 {
		return
	}
	// Clear all capacity to release GC references
	s = s[:cap(s)]
	for i := range s {
		s[i] = node{}
	}
	i := poolIndex(cap(s))
	nodePools[i].Put(s)
}

// poolIndex returns the pool bucket index for a given size.
// Returns ceil(log2(size)), so size=1→0, size=2→1, size=3→2, size=4→2, etc.
func poolIndex(size int) int {
	if size <= 1 {
		return 0
	}
	return bits.Len(uint(size - 1))
}
