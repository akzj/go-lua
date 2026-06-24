// LuaString slab allocator — pre-allocated []object.LuaString slabs with
// O(1) get/put. Uses mmap-backed slab arrays to avoid Go GC scanning of
// individual LuaString structs (the single largest source of allocations).
//
// The slabSlot field in LuaString enables O(1) identity lookup (no linear
// pointer-range scan). A single mutex protects the slab metadata, but
// critical sections are minimal and O(1).
package luastring

import (
	"sync"

	"github.com/akzj/go-lua/internal/object"
)

// stringSlabSize is the number of LuaString elements per slab.
const stringSlabSize = 4096

// stringFreeEntry identifies a freed element within a slab.
type stringFreeEntry struct {
	slabIdx int32
	elemIdx int32
}

// stringSlabMeta holds a slab backing array and usage tracking.
type stringSlabMeta struct {
	data []object.LuaString
	used int32
}

// Global slab state for LuaString allocations.
var (
	stringSlabMu         sync.Mutex
	stringSlabs          []stringSlabMeta
	stringFreeList       []stringFreeEntry
	stringNextSlabIdx    int32
	stringNextSequential int32
)

// packStringSlot encodes slab index (1-based, upper 16 bits) and element index
// (1-based, lower 16 bits) into a uint32. A zero value means "not from slab."
func packStringSlot(slabIdx, elemIdx int32) uint32 {
	return uint32(slabIdx+1)<<16 | uint32(elemIdx+1)&0xFFFF
}

// unpackStringSlot decodes a uint32 slab slot into 0-based slab/element indices.
func unpackStringSlot(slot uint32) (int32, int32) {
	return int32(slot>>16) - 1, int32(slot&0xFFFF) - 1
}

// slabGetLuaString returns a *object.LuaString from the slab allocator.
// Prefers the free list, then sequential allocation, then allocates a new slab.
func slabGetLuaString() *object.LuaString {
	stringSlabMu.Lock()

	// 1. Try free list first (LIFO for cache locality)
	n := len(stringFreeList)
	if n > 0 {
		fe := stringFreeList[n-1]
		stringFreeList = stringFreeList[:n-1]
		stringSlabs[fe.slabIdx].used++
		stringSlabMu.Unlock()
		slab := &stringSlabs[fe.slabIdx].data[fe.elemIdx]
		*slab = object.LuaString{} // zero for safe reuse
		return slab
	}

	// 2. Sequential alloc from current slab
	if stringNextSlabIdx < int32(len(stringSlabs)) && stringNextSequential < stringSlabSize {
		i := stringNextSequential
		stringNextSequential++
		stringSlabs[stringNextSlabIdx].used++
		t := &stringSlabs[stringNextSlabIdx].data[i]
		t.SlabSlot = packStringSlot(stringNextSlabIdx, int32(i))
		stringSlabMu.Unlock()
		return t
	}

	// 3. Allocate new slab
	slab := make([]object.LuaString, stringSlabSize)
	stringSlabs = append(stringSlabs, stringSlabMeta{data: slab})
	stringNextSlabIdx = int32(len(stringSlabs) - 1)
	stringNextSequential = 1 // element 0 returned now
	t := &slab[0]
	t.SlabSlot = packStringSlot(stringNextSlabIdx, 0)
	stringSlabMu.Unlock()
	return t
}

// slabPutLuaString returns a LuaString to the slab allocator free list.
// Uses slabSlot for O(1) identity lookup — no linear scan.
func slabPutLuaString(ls *object.LuaString) {
	slot := ls.SlabSlot
	if slot == 0 {
		return // not from slab allocator
	}
	slabIdx, elemIdx := unpackStringSlot(slot)

	stringSlabMu.Lock()
	stringFreeList = append(stringFreeList, stringFreeEntry{slabIdx: slabIdx, elemIdx: elemIdx})
	stringSlabMu.Unlock()
}

// GetLuaString returns a zeroed *object.LuaString from the slab allocator.
func GetLuaString() *object.LuaString {
	return slabGetLuaString()
}

// PutLuaString returns a LuaString to the slab allocator for reuse.
func PutLuaString(ls *object.LuaString) {
	// Clear reference-bearing fields to avoid retaining garbage
	ls.Data = ""
	ls.GCHeader = object.GCHeader{}
	slabPutLuaString(ls)
}
