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

var (
	lclosureSlabMu         sync.Mutex
	lclosureSlabs          [][]LClosure
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
		lclosureSlabMu.Unlock()
		return &lclosureSlabs[fe.slabIdx][fe.elemIdx]
	}
	if lclosureNextSlabIdx < int32(len(lclosureSlabs)) && lclosureNextSequential < lclosureSlabSize {
		i := lclosureNextSequential
		lclosureNextSequential++
		lclosureSlabMu.Unlock()
		lclosureSlabs[lclosureNextSlabIdx][i] = LClosure{}
		return &lclosureSlabs[lclosureNextSlabIdx][i]
	}
	slab := make([]LClosure, lclosureSlabSize)
	lclosureSlabs = append(lclosureSlabs, slab)
	lclosureNextSlabIdx = int32(len(lclosureSlabs) - 1)
	lclosureNextSequential = 1
	lclosureSlabMu.Unlock()
	return &slab[0]
}

// slabPutLClosure returns an LClosure to the slab allocator free list.
func slabPutLClosure(cl *LClosure) {
	ptr := unsafe.Pointer(cl)
	sz := unsafe.Sizeof(LClosure{})
	lclosureSlabMu.Lock()
	for i, slab := range lclosureSlabs {
		base := unsafe.SliceData(slab)
		diff := uintptr(ptr) - uintptr(unsafe.Pointer(base))
		if diff >= 0 && diff < uintptr(len(slab))*sz {
			elemIdx := int32(diff / sz)
			lclosureFreeList = append(lclosureFreeList, lclosureFreeEntry{slabIdx: int32(i), elemIdx: elemIdx})
			lclosureSlabMu.Unlock()
			return
		}
	}
	lclosureSlabMu.Unlock()
	// Fallback: not from any slab (shouldn't happen)
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

var (
	upvalSlabMu         sync.Mutex
	upvalSlabs          [][]UpVal
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
		upvalSlabMu.Unlock()
		return &upvalSlabs[fe.slabIdx][fe.elemIdx]
	}
	if upvalNextSlabIdx < int32(len(upvalSlabs)) && upvalNextSequential < upvalSlabSize {
		i := upvalNextSequential
		upvalNextSequential++
		upvalSlabMu.Unlock()
		upvalSlabs[upvalNextSlabIdx][i] = UpVal{}
		return &upvalSlabs[upvalNextSlabIdx][i]
	}
	slab := make([]UpVal, upvalSlabSize)
	upvalSlabs = append(upvalSlabs, slab)
	upvalNextSlabIdx = int32(len(upvalSlabs) - 1)
	upvalNextSequential = 1
	upvalSlabMu.Unlock()
	return &slab[0]
}

// slabPutUpVal returns an UpVal to the slab allocator free list.
func slabPutUpVal(uv *UpVal) {
	ptr := unsafe.Pointer(uv)
	sz := unsafe.Sizeof(UpVal{})
	upvalSlabMu.Lock()
	for i, slab := range upvalSlabs {
		base := unsafe.SliceData(slab)
		diff := uintptr(ptr) - uintptr(unsafe.Pointer(base))
		if diff >= 0 && diff < uintptr(len(slab))*sz {
			elemIdx := int32(diff / sz)
			upvalFreeList = append(upvalFreeList, upvalFreeEntry{slabIdx: int32(i), elemIdx: elemIdx})
			upvalSlabMu.Unlock()
			return
		}
	}
	upvalSlabMu.Unlock()
	// Fallback: not from any slab (shouldn't happen)
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
