// LuaState object pool — reuses dead thread structs to reduce allocation pressure.
//
// Coroutines are the most expensive objects to create in go-lua because each
// LuaState includes a large stack slice (~200 StackValues). Using slab allocation
// replaces individual heap allocations with pre-allocated slabs, eliminating
// Go GC scan overhead for LuaState objects.
package state

import (
	"sync"
	"unsafe"

	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// Slab allocator for LuaState — pre-allocated slabs with O(1) get/put.
// ---------------------------------------------------------------------------

// luaStateSlabSize is the number of LuaState elements per slab.
// LuaState is ~400 bytes → 256 elements per slab = ~100KB.
const luaStateSlabSize = 256

// luaStateFreeEntry identifies a freed LuaState within a slab.
type luaStateFreeEntry struct {
	slabIdx int32
	elemIdx int32
}

// Global LuaState slab state.
var (
	luaStateSlabMu         sync.Mutex     // guards all LuaState slab state
	luaStateSlabs          [][]LuaState   // slab backing arrays
	luaStateFreeList       []luaStateFreeEntry // LIFO free list
	luaStateNextSlabIdx    int32          // current slab for sequential alloc
	luaStateNextSequential int32          // next sequential index in current slab
)

// slabGetLuaState returns a *LuaState from the slab allocator.
// Prefers free list (LIFO), then sequential bump, then allocates a new slab.
func slabGetLuaState() *LuaState {
	luaStateSlabMu.Lock()
	// 1. Try free list first (LIFO for cache locality)
	n := len(luaStateFreeList)
	if n > 0 {
		fe := luaStateFreeList[n-1]
		luaStateFreeList = luaStateFreeList[:n-1]
		luaStateSlabMu.Unlock()
		return &luaStateSlabs[fe.slabIdx][fe.elemIdx]
	}
	// 2. Sequential alloc from current slab
	if luaStateNextSlabIdx < int32(len(luaStateSlabs)) && luaStateNextSequential < luaStateSlabSize {
		i := luaStateNextSequential
		luaStateNextSequential++
		luaStateSlabMu.Unlock()
		// Zero the struct for safe reuse
		luaStateSlabs[luaStateNextSlabIdx][i] = LuaState{}
		return &luaStateSlabs[luaStateNextSlabIdx][i]
	}
	// 3. Allocate new slab
	slab := make([]LuaState, luaStateSlabSize)
	luaStateSlabs = append(luaStateSlabs, slab)
	luaStateNextSlabIdx = int32(len(luaStateSlabs) - 1)
	luaStateNextSequential = 1 // element 0 returned now
	luaStateSlabMu.Unlock()
	return &slab[0]
}

// slabPutLuaState returns a LuaState to the slab allocator free list.
// Uses pointer range scanning (diff-based, checkptr-safe).
func slabPutLuaState(L *LuaState) {
	ptr := unsafe.Pointer(L)
	sz := unsafe.Sizeof(LuaState{})
	luaStateSlabMu.Lock()
	for i, slab := range luaStateSlabs {
		base := unsafe.SliceData(slab)
		diff := uintptr(ptr) - uintptr(unsafe.Pointer(base))
		if diff >= 0 && diff < uintptr(len(slab))*sz {
			elemIdx := int32(diff / sz)
			luaStateFreeList = append(luaStateFreeList, luaStateFreeEntry{slabIdx: int32(i), elemIdx: elemIdx})
			luaStateSlabMu.Unlock()
			return
		}
	}
	luaStateSlabMu.Unlock()
	// Fallback: not from any slab (shouldn't happen in normal operation)
}

// getLuaState gets a LuaState from the slab allocator.
// PutLuaState guarantees all fields are cleared before returning to the slab,
// so the returned struct is ready for reuse. Stack and CISlab retain capacity
// from a previous use (reused in stackInit and NewCallInfo).
func getLuaState() *LuaState {
	return slabGetLuaState()
}

// PutLuaState returns a LuaState to the slab allocator for reuse.
// Called by the GC sweep phase when a dead thread is unlinked.
// Zeroes ALL fields (scalar, pointer, embedded) so getLuaState can return
// the struct directly without per-field clearing. Stack and CISlab backing
// arrays are retained for capacity reuse.
func PutLuaState(L *LuaState) {
	// GC header and pointer fields — prevent retaining dead objects
	L.GCHeader = object.GCHeader{}
	L.Global = nil
	L.OpenUpval = nil
	L.Hook = nil
	L.APIState = nil
	L.CI = nil
	L.BaseCI = CallInfo{}
	// Scalar fields — zero for clean reuse by NewThread
	L.Top = 0
	L.Status = 0
	L.AllowHook = false
	L.NCCalls = 0
	L.NCI = 0
	L.ErrFunc = 0
	L.OldPC = 0
	L.TBCList = 0
	L.HookMask = 0
	L.BaseHookCount = 0
	L.HookCount = 0
	L.FTransfer = 0
	L.NTransfer = 0
	L.HookEvent = 0
	L.HookLine = 0
	L.HookSavedTop = 0
	L.HookSavedCITop = 0
	L.YieldFlag = false
	// Clear only used CI slab entries (CISlabIdx tells us how many were used)
	for i := 0; i < L.CISlabIdx; i++ {
		L.CISlab[i] = CallInfo{}
	}
	L.CISlabIdx = 0
	// Zero all stack slots. Uses Go's built-in clear() which the compiler
	// optimizes to memclr (a single fast memset-like operation) instead of
	// a per-element loop. Both Tt (for Lua GC safety) and Obj (for Go GC
	// safety) must be zeroed; clear() handles the full StackValue.
	clear(L.Stack)
	// Reslice to zero length but keep capacity
	L.Stack = L.Stack[:0]
	slabPutLuaState(L)
}
