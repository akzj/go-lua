// Table and slice pools — reuses dead Table structs and backing slices
// to reduce allocation pressure.
//
// Tables are the most frequently allocated GC objects in Lua programs (~96% of
// allocations in GC benchmarks). Using slab-backed arrays lets us reuse struct
// memory instead of going through Go's mallocgc each time.
//
// Free list is a lock-free ring buffer (atomic head/tail) — no mutex on the
// hot alloc/free path. A fallback mutex protects the sequential allocator and
// new slab creation, which are rare (only when the ring is empty/exhausted).
//
// Slice pools index by log2(capacity) so that power-of-2 sized slices are
// reused across tables of similar sizes.
package table

import (
	"math/bits"
	"sync"
	"sync/atomic"

	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// Table struct pool — slab allocator
// ---------------------------------------------------------------------------

// slabSize is the number of Table elements per slab.
const slabSize = 4096

// entry is a free entry (slab index + element index) for the lock-free ring.
type entry struct {
	slabIdx int32
	elemIdx int32
}

// ringSize must be a power of 2 for fast modulo.
const ringSize = 16384 // 128KB — holds up to 16K freed table slots

// freeRing is a lock-free ring buffer for free list operations.
// Both head and tail are monotonically increasing uint64 counters;
// the actual index is counter % ringSize.
// Head = next write position, Tail = next read position.
// Empty: head == tail. Full: head-tail == ringSize.
var (
	freeRing [ringSize]entry
	ringHead atomic.Uint64
	ringTail atomic.Uint64
)

// pushFree adds a free entry to the ring. Returns false if ring is full.
// Lock-free via CAS on ringHead.
func pushFree(slabIdx, elemIdx int32) bool {
	for {
		h := ringHead.Load()
		t := ringTail.Load()
		if h-t >= ringSize {
			return false // ring full
		}
		if ringHead.CompareAndSwap(h, h+1) {
			freeRing[h%ringSize] = entry{slabIdx: slabIdx, elemIdx: elemIdx}
			return true
		}
	}
}

// popFree removes a free entry from the ring. Returns false if ring is empty.
// Lock-free via CAS on ringTail.
func popFree() (entry, bool) {
	for {
		h := ringHead.Load()
		t := ringTail.Load()
		if t >= h {
			return entry{}, false // empty
		}
		if ringTail.CompareAndSwap(t, t+1) {
			e := freeRing[t%ringSize]
			return e, true
		}
	}
}

// slabMu protects the sequential allocator and new slab creation
// (cold path — only used when the ring is empty or exhausted).
var (
	slabMu         sync.Mutex
	tableSlabs     []tableSlabMeta
	nextSlabIdx    int32
	nextSequential int32
)

// tableSlabMeta holds a slab backing array.
type tableSlabMeta struct {
	data []Table
}

// packSlot encodes slab index (1-based, upper 16 bits) and element index
// (1-based, lower 16 bits) into a uint32. A zero value means "not from slab."
func packSlot(slabIdx, elemIdx int32) uint32 {
	return uint32(slabIdx+1)<<16 | uint32(elemIdx+1)&0xFFFF
}

// unpackSlot decodes a uint32 slab slot into 0-based slab/element indices.
func unpackSlot(slot uint32) (int32, int32) {
	return int32(slot>>16) - 1, int32(slot&0xFFFF) - 1
}

// slabGetTable returns a *Table from the slab allocator.
// Hot path: lock-free pop from ring. Cold path: locked sequential alloc or new slab.
func slabGetTable() *Table {
	// Hot path: try lock-free ring
	if e, ok := popFree(); ok {
		t := &tableSlabs[e.slabIdx].data[e.elemIdx]
		t.slabSlot = packSlot(e.slabIdx, e.elemIdx)
		return t
	}

	// Cold path: locked — sequential alloc or new slab
	slabMu.Lock()

	// Sequential alloc from current slab
	if nextSlabIdx < int32(len(tableSlabs)) && nextSequential < slabSize {
		i := nextSequential
		nextSequential++
		t := &tableSlabs[nextSlabIdx].data[i]
		t.slabSlot = packSlot(nextSlabIdx, i)
		slabMu.Unlock()
		return t
	}

	// Allocate new slab
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
// Hot path: lock-free push to ring. Cold path: drop (rare overflow).
func slabPutTable(t *Table) {
	si, ei := unpackSlot(t.slabSlot)
	if !pushFree(si, ei) {
		// Ring full — rare. Drop the entry.
		// The slab's sequential allocator provides an unbounded fallback.
	}
}

// getTable gets a Table from the slab allocator.
func getTable() *Table {
	t := slabGetTable()
	t.LsizeNode = 0
	t.LastFree = 0
	t.Flags = 0
	t.WeakMode = 0
	t.SizeDelta = 0
	return t
}

// PutTable returns a Table to the slab allocator for reuse.
func PutTable(t *Table) {
	if t.Array != nil {
		if !isInlineSlice(t, t.Array) {
			putArraySlice(t.Array)
		}
	}
	if t.Nodes != nil {
		putNodeSlice(t.Nodes)
	}
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

func getArraySlice(size int) []object.TValue {
	if size <= 0 {
		return nil
	}
	i := poolIndex(size)
	if v := arrayPools[i].Get(); v != nil {
		s := v.([]object.TValue)[:size]
		clear(s)
		return s
	}
	return make([]object.TValue, size, 1<<i)
}

func putArraySlice(s []object.TValue) {
	if cap(s) == 0 {
		return
	}
	s = s[:cap(s)]
	clear(s)
	i := poolIndex(cap(s))
	arrayPools[i].Put(s)
}

func getNodeSlice(size int) []node {
	if size <= 0 {
		return nil
	}
	i := poolIndex(size)
	if v := nodePools[i].Get(); v != nil {
		s := v.([]node)[:size]
		clear(s)
		return s
	}
	return make([]node, size, 1<<i)
}

func putNodeSlice(s []node) {
	if cap(s) == 0 {
		return
	}
	s = s[:cap(s)]
	clear(s)
	i := poolIndex(cap(s))
	nodePools[i].Put(s)
}

func poolIndex(size int) int {
	if size <= 1 {
		return 0
	}
	return bits.Len(uint(size - 1))
}
