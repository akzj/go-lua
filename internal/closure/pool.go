// Closure and UpVal object pools — reuses dead structs to reduce allocation pressure.
//
// LClosures and UpVals are the second and third most frequently allocated GC
// objects (after tables). Using slab allocation replaces individual heap
// allocations with pre-allocated slabs, eliminating Go GC scan overhead.
package closure

import (
	"sync"
	"unsafe"

	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// LClosure slab allocator — pre-allocated []LClosure slabs
// LClosure is ~64 bytes, slab of 4096 = ~262KB.
// ---------------------------------------------------------------------------

const lclosureSlabSize = 4096

type lclosureFreeEntry struct {
	slabIdx int32
	elemIdx int32
}

// lclosureSlabMeta holds a slab backing array and tracks live element count.
type lclosureSlabMeta struct {
	data []LClosure
	used int32
}

var (
	lclosureSlabMu         sync.Mutex
	lclosureSlabs          []lclosureSlabMeta
	lclosureFreeList       []lclosureFreeEntry
	lclosureNextSlabIdx    int32
	lclosureNextSequential int32
)

// slabGetLClosure returns an *LClosure from the slab allocator.
func slabGetLClosure() *LClosure {
	lclosureSlabMu.Lock()
	n := len(lclosureFreeList)
	if n > 0 {
		fe := lclosureFreeList[n-1]
		lclosureFreeList = lclosureFreeList[:n-1]
		lclosureSlabs[fe.slabIdx].used++
		lclosureSlabMu.Unlock()
		return &lclosureSlabs[fe.slabIdx].data[fe.elemIdx]
	}
	if lclosureNextSlabIdx < int32(len(lclosureSlabs)) && lclosureNextSequential < lclosureSlabSize {
		i := lclosureNextSequential
		lclosureNextSequential++
		lclosureSlabs[lclosureNextSlabIdx].used++
		lclosureSlabMu.Unlock()
		lclosureSlabs[lclosureNextSlabIdx].data[i] = LClosure{}
		return &lclosureSlabs[lclosureNextSlabIdx].data[i]
	}
	slab := make([]LClosure, lclosureSlabSize)
	lclosureSlabs = append(lclosureSlabs, lclosureSlabMeta{data: slab, used: 1})
	lclosureNextSlabIdx = int32(len(lclosureSlabs) - 1)
	lclosureNextSequential = 1
	lclosureSlabMu.Unlock()
	return &slab[0]
}

// slabPutLClosure returns an LClosure to the slab allocator free list.
// If the slab becomes completely empty, it is reclaimed to prevent unbounded growth.
func slabPutLClosure(cl *LClosure) {
	ptr := unsafe.Pointer(cl)
	sz := unsafe.Sizeof(LClosure{})
	lclosureSlabMu.Lock()
	for i := range lclosureSlabs {
		meta := &lclosureSlabs[i]
		base := unsafe.SliceData(meta.data)
		diff := uintptr(ptr) - uintptr(unsafe.Pointer(base))
		if diff >= 0 && diff < uintptr(len(meta.data))*sz {
			elemIdx := int32(diff / sz)
			meta.used--
			if meta.used == 0 && len(lclosureSlabs) > 1 {
				lclosureSlabs = append(lclosureSlabs[:i], lclosureSlabs[i+1:]...)
				compactFreeListLClosure(i)
			} else {
				lclosureFreeList = append(lclosureFreeList, lclosureFreeEntry{slabIdx: int32(i), elemIdx: elemIdx})
			}
			lclosureSlabMu.Unlock()
			return
		}
	}
	lclosureSlabMu.Unlock()
	// Fallback: not from any slab (shouldn't happen)
}

// compactFreeListLClosure adjusts LClosure slab indices after a slab is removed.
func compactFreeListLClosure(removedIdx int) {
	j := 0
	for i := range lclosureFreeList {
		if lclosureFreeList[i].slabIdx == int32(removedIdx) {
			continue
		}
		if lclosureFreeList[i].slabIdx > int32(removedIdx) {
			lclosureFreeList[i].slabIdx--
		}
		lclosureFreeList[j] = lclosureFreeList[i]
		j++
	}
	lclosureFreeList = lclosureFreeList[:j]
	if lclosureNextSlabIdx > int32(removedIdx) {
		lclosureNextSlabIdx--
	} else if lclosureNextSlabIdx == int32(removedIdx) {
		lclosureNextSequential = lclosureSlabSize
	}
}

// ---------------------------------------------------------------------------
// UpVal slab allocator — pre-allocated []UpVal slabs
// UpVal is ~88 bytes, slab of 4096 = ~360KB.
// ---------------------------------------------------------------------------

const upvalSlabSize = 4096

type upvalFreeEntry struct {
	slabIdx int32
	elemIdx int32
}

// upvalSlabMeta holds a slab backing array and tracks live element count.
type upvalSlabMeta struct {
	data []UpVal
	used int32
}

var (
	upvalSlabMu         sync.Mutex
	upvalSlabs          []upvalSlabMeta
	upvalFreeList       []upvalFreeEntry
	upvalNextSlabIdx    int32
	upvalNextSequential int32
)

// slabGetUpVal returns an *UpVal from the slab allocator.
func slabGetUpVal() *UpVal {
	upvalSlabMu.Lock()
	n := len(upvalFreeList)
	if n > 0 {
		fe := upvalFreeList[n-1]
		upvalFreeList = upvalFreeList[:n-1]
		upvalSlabs[fe.slabIdx].used++
		upvalSlabMu.Unlock()
		return &upvalSlabs[fe.slabIdx].data[fe.elemIdx]
	}
	if upvalNextSlabIdx < int32(len(upvalSlabs)) && upvalNextSequential < upvalSlabSize {
		i := upvalNextSequential
		upvalNextSequential++
		upvalSlabs[upvalNextSlabIdx].used++
		upvalSlabMu.Unlock()
		upvalSlabs[upvalNextSlabIdx].data[i] = UpVal{}
		return &upvalSlabs[upvalNextSlabIdx].data[i]
	}
	slab := make([]UpVal, upvalSlabSize)
	upvalSlabs = append(upvalSlabs, upvalSlabMeta{data: slab, used: 1})
	upvalNextSlabIdx = int32(len(upvalSlabs) - 1)
	upvalNextSequential = 1
	upvalSlabMu.Unlock()
	return &slab[0]
}

// slabPutUpVal returns an UpVal to the slab allocator free list.
// If the slab becomes completely empty, it is reclaimed to prevent unbounded growth.
func slabPutUpVal(uv *UpVal) {
	ptr := unsafe.Pointer(uv)
	sz := unsafe.Sizeof(UpVal{})
	upvalSlabMu.Lock()
	for i := range upvalSlabs {
		meta := &upvalSlabs[i]
		base := unsafe.SliceData(meta.data)
		diff := uintptr(ptr) - uintptr(unsafe.Pointer(base))
		if diff >= 0 && diff < uintptr(len(meta.data))*sz {
			elemIdx := int32(diff / sz)
			meta.used--
			if meta.used == 0 && len(upvalSlabs) > 1 {
				upvalSlabs = append(upvalSlabs[:i], upvalSlabs[i+1:]...)
				compactFreeListUpVal(i)
			} else {
				upvalFreeList = append(upvalFreeList, upvalFreeEntry{slabIdx: int32(i), elemIdx: elemIdx})
			}
			upvalSlabMu.Unlock()
			return
		}
	}
	upvalSlabMu.Unlock()
	// Fallback: not from any slab (shouldn't happen)
}

// compactFreeListUpVal adjusts UpVal slab indices after a slab is removed.
func compactFreeListUpVal(removedIdx int) {
	j := 0
	for i := range upvalFreeList {
		if upvalFreeList[i].slabIdx == int32(removedIdx) {
			continue
		}
		if upvalFreeList[i].slabIdx > int32(removedIdx) {
			upvalFreeList[i].slabIdx--
		}
		upvalFreeList[j] = upvalFreeList[i]
		j++
	}
	upvalFreeList = upvalFreeList[:j]
	if upvalNextSlabIdx > int32(removedIdx) {
		upvalNextSlabIdx--
	} else if upvalNextSlabIdx == int32(removedIdx) {
		upvalNextSequential = upvalSlabSize
	}
}

// ---------------------------------------------------------------------------
// LClosure pool
// ---------------------------------------------------------------------------

// getLClosure gets an LClosure from the slab allocator.
// The returned closure has zeroed GCHeader and nil Proto/UpVals.
func getLClosure() *LClosure {
	cl := slabGetLClosure()
	cl.GCHeader = object.GCHeader{}
	cl.Proto = nil
	cl.UpVals = nil
	return cl
}

// PutLClosure returns an LClosure to the slab allocator for reuse.
// Called by the GC sweep phase when a dead closure is unlinked.
// Clears all reference fields before returning to slab.
func PutLClosure(cl *LClosure) {
	cl.Proto = nil
	cl.UpVals = nil
	cl.GCHeader = object.GCHeader{}
	slabPutLClosure(cl)
}

// ---------------------------------------------------------------------------
// UpVal pool
// ---------------------------------------------------------------------------

// getUpVal gets an UpVal from the slab allocator.
// The returned upval is fully zeroed.
func getUpVal() *UpVal {
	uv := slabGetUpVal()
	uv.GCHeader = object.GCHeader{}
	uv.StackIdx = 0
	uv.Own = object.Nil
	uv.Next = nil
	uv.Stack = nil
	return uv
}

// PutUpVal returns an UpVal to the slab allocator for reuse.
// Called by the GC sweep phase when a dead upval is unlinked.
// Clears all reference fields before returning to slab.
func PutUpVal(uv *UpVal) {
	uv.Own = object.Nil
	uv.Next = nil
	uv.Stack = nil
	uv.GCHeader = object.GCHeader{}
	slabPutUpVal(uv)
}
