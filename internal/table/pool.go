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
	"unsafe"

	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// Table struct pool
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Slab allocator — pre-allocated []Table slabs with O(1) get/put.
// Eliminates Go GC scanning of individual Table structs by allocating them
// within large fixed backing arrays that act as a single GC root.
// ---------------------------------------------------------------------------

// slabSize is the number of Table elements per slab.
const slabSize = 4096

// freeEntry identifies a freed element within a slab.
type freeEntry struct {
	slabIdx int32
	elemIdx int32
}

// tableSlabMeta holds a slab backing array and tracks how many of its
// elements are currently live. Used for empty slab reclamation.
type tableSlabMeta struct {
	data []Table
	used int32
}

// Global slab state.
var (
	slabMu         sync.Mutex      // guards all slab state (concurrent-safe)
	tableSlabs     []tableSlabMeta // slab backing arrays with usage tracking
	tableFreeList  []freeEntry     // LIFO free list of (slab, elem) pairs
	nextSlabIdx    int32           // current slab index for sequential alloc
	nextSequential int32           // next sequential index within current slab
)

// slabGetTable returns a *Table from the slab allocator.
// Prefers the free list (LIFO), then sequential allocation within the current
// slab, then allocates a new slab if exhausted.
func slabGetTable() *Table {
	slabMu.Lock()
	// 1. Try free list first (LIFO for cache locality)
	n := len(tableFreeList)
	if n > 0 {
		fe := tableFreeList[n-1]
		tableFreeList = tableFreeList[:n-1]
		tableSlabs[fe.slabIdx].used++
		slabMu.Unlock()
		return &tableSlabs[fe.slabIdx].data[fe.elemIdx]
	}
	// 2. Sequential alloc from current slab
	if nextSlabIdx < int32(len(tableSlabs)) && nextSequential < slabSize {
		i := nextSequential
		nextSequential++
		tableSlabs[nextSlabIdx].used++
		slabMu.Unlock()
		// Return directly — getTable() zeroes scalar fields. Reference fields
		// (Array, Nodes, Metatable, GCHeader) are cleared by PutTable.
		return &tableSlabs[nextSlabIdx].data[i]
	}
	// 3. Allocate new slab
	slab := make([]Table, slabSize)
	tableSlabs = append(tableSlabs, tableSlabMeta{data: slab, used: 1})
	nextSlabIdx = int32(len(tableSlabs) - 1)
	nextSequential = 1 // element 0 returned now
	slabMu.Unlock()
	return &slab[0]
}

// slabPutTable returns a Table to the slab allocator free list.
// Uses pointer range scanning to determine which slab t belongs to.
// If the slab becomes completely empty (all elements freed), the slab
// is removed and its backing array freed to prevent unbounded growth.
func slabPutTable(t *Table) {
	ptr := unsafe.Pointer(t)
	sz := unsafe.Sizeof(Table{})
	slabMu.Lock()
	for i := range tableSlabs {
		meta := &tableSlabs[i]
		base := unsafe.SliceData(meta.data)
		diff := uintptr(ptr) - uintptr(unsafe.Pointer(base))
		if diff >= 0 && diff < uintptr(len(meta.data))*sz {
			elemIdx := int32(diff / sz)
			meta.used--
			if meta.used == 0 && len(tableSlabs) > 1 {
				// Reclaim this empty slab
				tableSlabs = append(tableSlabs[:i], tableSlabs[i+1:]...)
				compactFreeList(i)
			} else {
				tableFreeList = append(tableFreeList, freeEntry{slabIdx: int32(i), elemIdx: elemIdx})
			}
			slabMu.Unlock()
			return
		}
	}
	slabMu.Unlock()
	// Fallback: t is not from any slab (shouldn't happen in normal operation)
}

// compactFreeList adjusts slab indices after a slab is removed.
// Removes stale free list entries that pointed to the removed slab,
// decrements slabIdx for entries beyond the removed index, and updates
// nextSlabIdx if needed.
func compactFreeList(removedIdx int) {
	// Single pass: filter stale entries and adjust indices
	j := 0
	for i := range tableFreeList {
		if tableFreeList[i].slabIdx == int32(removedIdx) {
			continue // drop stale entry pointing to the removed slab
		}
		if tableFreeList[i].slabIdx > int32(removedIdx) {
			tableFreeList[i].slabIdx--
		}
		tableFreeList[j] = tableFreeList[i]
		j++
	}
	tableFreeList = tableFreeList[:j]
	if nextSlabIdx > int32(removedIdx) {
		nextSlabIdx--
	} else if nextSlabIdx == int32(removedIdx) {
		// Lost sequential position context; force new slab allocation on next alloc
		nextSequential = slabSize
	}
}

var tablePool = sync.Pool{
	New: func() any {
		return &Table{}
	},
}

// getTable gets a Table from the slab allocator.
// The returned Table has zeroed scalar fields; callers must initialize as needed.
func getTable() *Table {
	t := slabGetTable()
	// SlabGetTable zeros the struct, but re-zero scalar fields for safety.
	t.LsizeNode = 0
	t.LastFree = 0
	t.Flags = 0
	t.WeakMode = 0
	t.SizeDelta = 0
	return t
}

// PutTable returns a Table to the slab allocator for reuse.
// Called by the GC sweep phase when a dead table is unlinked.
// Pools backing slices (unless they're inline arrays) before clearing references.
func PutTable(t *Table) {
	// Pool backing slices only if NOT inline arrays
	if t.Array != nil {
		if !isInlineSlice(t, t.Array) {
			putArraySlice(t.Array)
		}
	}
	if t.Nodes != nil {
		putNodeSlice(t.Nodes)
	}
	// Clear all reference-bearing fields to avoid retaining garbage
	t.Array = nil
	t.Nodes = nil
	t.Metatable = nil
	t.GCHeader = object.GCHeader{}
	slabPutTable(t)
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
		clear(s)
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
	clear(s)
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
		clear(s)
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
	clear(s)
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
