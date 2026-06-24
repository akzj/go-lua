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

// ---------------------------------------------------------------------------
// Slab allocator — pre-allocated []Table slabs with per-P free batches.
//
// Slabs eliminate Go GC scanning of individual Table structs by allocating them
// within large fixed backing arrays that act as a single GC root.
//
// To eliminate the global mutex contention that cost ~30% CPU in GC-heavy
// workloads, we use a two-tier approach:
//   1. Per-P free batches (via sync.Pool) — lock-free hot path
//   2. Global free list + sequential alloc — locked fallback
//
// Each Table carries a slabSlot (packed slab index + element index) so
// slabPutTable is O(1) instead of O(n), eliminating the pointer-range scan.
// ---------------------------------------------------------------------------

// slabSize is the number of Table elements per slab.
const slabSize = 4096

// batchSize is the number of free entries cached per P in a perPFreeBatch.
// Kept small (64) to bound per-P memory overhead to ~2KB per P.
const batchSize = 64

// freeEntry identifies a freed element within a slab.
type freeEntry struct {
	slabIdx int32
	elemIdx int32
}

// perPFreeBatch caches a small LIFO stack of free table slots per P.
// Using sync.Pool, each P gets its own batch, eliminating lock contention
// on the hot alloc/free path.
type perPFreeBatch struct {
	entries [batchSize]freeEntry
	count   int
}

// tableSlabMeta holds a slab backing array.
// Slabs are never reclaimed (bounded by peak usage), so no used-tracking needed.
type tableSlabMeta struct {
	data []Table
}

// Global slab state.
var (
	slabMu         sync.Mutex      // guards all slab state (concurrent-safe)
	tableSlabs     []tableSlabMeta // slab backing arrays (never shrinks)
	tableFreeList  []freeEntry     // LIFO free list of (slab, elem) pairs
	nextSlabIdx    int32           // current slab index for sequential alloc
	nextSequential int32           // next sequential index within current slab
)

// freeBatchPool caches per-P free batches. Each Get returns a batch
// from the current P's cache (lock-free), or a new one is created.
var freeBatchPool = sync.Pool{
	New: func() any { return &perPFreeBatch{} },
}

// packSlot encodes slab index (upper 16 bits) and element index (lower 16 bits)
// into a uint32 for O(1) slab lookups.
func packSlot(slabIdx, elemIdx int32) uint32 {
	return uint32(slabIdx)<<16 | uint32(elemIdx)&0xFFFF
}

// unpackSlot decodes a uint32 slab slot into slab index and element index.
func unpackSlot(slot uint32) (int32, int32) {
	return int32(slot >> 16), int32(slot & 0xFFFF)
}

// slabGetTable returns a *Table from the slab allocator.
//
// Fast path (lock-free): pops from the per-P free batch.
// Slow path (locked): falls through to global free list → sequential alloc → new slab.
//
// The returned Table has slabSlot set for O(1) future slabPutTable calls.
func slabGetTable() *Table {
	// Fast path: try per-P batch (lock-free)
	batch := freeBatchPool.Get().(*perPFreeBatch)
	if batch.count > 0 {
		batch.count--
		fe := batch.entries[batch.count]
		t := &tableSlabs[fe.slabIdx].data[fe.elemIdx]
		t.slabSlot = packSlot(fe.slabIdx, fe.elemIdx)
		freeBatchPool.Put(batch)
		return t
	}
	freeBatchPool.Put(batch)

	// Slow path: locked
	slabMu.Lock()

	// 1. Try global free list first
	n := len(tableFreeList)
	if n > 0 {
		fe := tableFreeList[n-1]
		tableFreeList = tableFreeList[:n-1]
		t := &tableSlabs[fe.slabIdx].data[fe.elemIdx]
		t.slabSlot = packSlot(fe.slabIdx, fe.elemIdx)
		slabMu.Unlock()
		return t
	}

	// 2. Sequential alloc from current slab
	if nextSlabIdx < int32(len(tableSlabs)) && nextSequential < slabSize {
		i := nextSequential
		nextSequential++
		t := &tableSlabs[nextSlabIdx].data[i]
		t.slabSlot = packSlot(nextSlabIdx, i)
		slabMu.Unlock()
		return t
	}

	// 3. Allocate new slab
	slab := make([]Table, slabSize)
	tableSlabs = append(tableSlabs, tableSlabMeta{data: slab})
	nextSlabIdx = int32(len(tableSlabs) - 1)
	nextSequential = 1 // element 0 returned now
	t := &slab[0]
	t.slabSlot = packSlot(nextSlabIdx, 0)
	slabMu.Unlock()
	return t
}

// slabPutTable returns a Table to the slab allocator free list.
// O(1): uses the Table's slabSlot field to determine slab/element identity
// without scanning all slabs.
//
// Fast path (lock-free): pushes to the per-P batch.
// If the batch is full, all entries are flushed to the global free list (locked).
func slabPutTable(t *Table) {
	si, ei := unpackSlot(t.slabSlot)

	batch := freeBatchPool.Get().(*perPFreeBatch)
	if batch.count < batchSize {
		// Fast path: push to per-P batch (lock-free)
		batch.entries[batch.count] = freeEntry{slabIdx: si, elemIdx: ei}
		batch.count++
		freeBatchPool.Put(batch)
		return
	}

	// Batch full: flush all entries to global free list (locked)
	slabMu.Lock()
	for i := 0; i < batch.count; i++ {
		tableFreeList = append(tableFreeList, batch.entries[i])
	}
	tableFreeList = append(tableFreeList, freeEntry{slabIdx: si, elemIdx: ei})
	slabMu.Unlock()

	batch.count = 0
	freeBatchPool.Put(batch)
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
	// slabGetTable sets slabSlot and returns a zero-initialized Table element
	// (slab backing arrays are created by make(), which zeroes all fields).
	// Re-zero scalar fields for safety on recycled entries.
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
	// Clear all reference-bearing fields to avoid retaining garbage.
	// slabSlot is intentionally preserved — slabPutTable needs it for O(1) lookup.
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
