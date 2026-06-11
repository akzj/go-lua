// LuaString slab allocator — pre-allocated []object.LuaString slabs with
// O(1) get/put. Reduces Go heap allocation pressure by keeping LuaString
// structs in large fixed backing arrays instead of allocating individually
// on the Go heap.
//
// Strings are the single largest allocation source in go-lua (~57% of heap
// allocations per profile). Since LuaString is exactly 56 bytes, fixed-size
// slab allocation eliminates per-struct Go GC scanning overhead.
package luastring

import (
	"sync"
	"unsafe"

	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// Slab allocator — pre-allocated []object.LuaString slabs with O(1) get/put.
// ---------------------------------------------------------------------------

// stringSlabSize is the number of LuaString elements per slab.
const stringSlabSize = 4096

// stringFreeEntry identifies a freed element within a slab.
type stringFreeEntry struct {
	slabIdx int32
	elemIdx int32
}

// stringSlabMeta holds a slab backing array and tracks how many of its
// elements are currently live. Used for empty slab reclamation.
type stringSlabMeta struct {
	data []object.LuaString
	used int32
}

// Global slab state for LuaString allocations.
var (
	stringSlabMu         sync.Mutex           // guards all slab state
	stringSlabs          []stringSlabMeta     // slab backing arrays with usage tracking
	stringFreeList       []stringFreeEntry    // LIFO free list of (slab, elem) pairs
	stringNextSlabIdx    int32                // current slab index for sequential alloc
	stringNextSequential int32                // next sequential index within current slab
)

// GetLuaString returns a zeroed *object.LuaString from the slab allocator.
// Prefers the free list (LIFO for cache locality), then sequential allocation
// within the current slab, then allocates a new slab if exhausted.
func GetLuaString() *object.LuaString {
	stringSlabMu.Lock()
	// 1. Try free list first (LIFO for cache locality)
	n := len(stringFreeList)
	if n > 0 {
		fe := stringFreeList[n-1]
		stringFreeList = stringFreeList[:n-1]
		stringSlabs[fe.slabIdx].used++
		stringSlabMu.Unlock()
		return &stringSlabs[fe.slabIdx].data[fe.elemIdx]
	}
	// 2. Sequential alloc from current slab
	if stringNextSlabIdx < int32(len(stringSlabs)) && stringNextSequential < stringSlabSize {
		i := stringNextSequential
		stringNextSequential++
		stringSlabs[stringNextSlabIdx].used++
		stringSlabMu.Unlock()
		// Zero the struct for safe reuse
		stringSlabs[stringNextSlabIdx].data[i] = object.LuaString{}
		return &stringSlabs[stringNextSlabIdx].data[i]
	}
	// 3. Allocate new slab
	slab := make([]object.LuaString, stringSlabSize)
	stringSlabs = append(stringSlabs, stringSlabMeta{data: slab, used: 1})
	stringNextSlabIdx = int32(len(stringSlabs) - 1)
	stringNextSequential = 1 // element 0 returned now
	stringSlabMu.Unlock()
	return &slab[0]
}

// PutLuaString returns a LuaString to the slab allocator free list.
// Clears all reference-bearing fields before returning to the pool.
// Uses pointer range scanning to determine which slab t belongs to.
// If the slab becomes completely empty (all elements freed), the slab
// is removed and its backing array freed to prevent unbounded growth.
func PutLuaString(ls *object.LuaString) {
	// Clear references to avoid retaining garbage
	ls.Data = ""
	ls.GCHeader = object.GCHeader{}

	ptr := unsafe.Pointer(ls)
	sz := unsafe.Sizeof(object.LuaString{})
	stringSlabMu.Lock()
	for i := range stringSlabs {
		meta := &stringSlabs[i]
		base := unsafe.SliceData(meta.data)
		diff := uintptr(ptr) - uintptr(unsafe.Pointer(base))
		if diff >= 0 && diff < uintptr(len(meta.data))*sz {
			elemIdx := int32(diff / sz)
			meta.used--
			if meta.used == 0 && len(stringSlabs) > 1 {
				// Reclaim this empty slab
				stringSlabs = append(stringSlabs[:i], stringSlabs[i+1:]...)
				compactStringFreeList(i)
			} else {
				stringFreeList = append(stringFreeList, stringFreeEntry{slabIdx: int32(i), elemIdx: elemIdx})
			}
			stringSlabMu.Unlock()
			return
		}
	}
	stringSlabMu.Unlock()
	// Fallback: ls is not from any slab (shouldn't happen in normal operation)
}

// compactStringFreeList adjusts slab indices after a slab is removed.
// Removes stale free list entries that pointed to the removed slab,
// decrements slabIdx for entries beyond the removed index, and updates
// nextSlabIdx if needed.
func compactStringFreeList(removedIdx int) {
	j := 0
	for i := range stringFreeList {
		if stringFreeList[i].slabIdx == int32(removedIdx) {
			continue // drop stale entry pointing to the removed slab
		}
		if stringFreeList[i].slabIdx > int32(removedIdx) {
			stringFreeList[i].slabIdx--
		}
		stringFreeList[j] = stringFreeList[i]
		j++
	}
	stringFreeList = stringFreeList[:j]
	if stringNextSlabIdx > int32(removedIdx) {
		stringNextSlabIdx--
	} else if stringNextSlabIdx == int32(removedIdx) {
		// Lost sequential position context; force new slab allocation on next alloc
		stringNextSequential = stringSlabSize
	}
}
