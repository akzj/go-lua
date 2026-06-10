// Closure and UpVal object pools — reuses dead structs to reduce allocation pressure.
//
// LClosures and UpVals are the second and third most frequently allocated GC
// objects (after tables). Using slice-based freelists is safe because Lua is
// single-threaded — no locks or atomics needed.
package closure

import (
	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// LClosure pool
// ---------------------------------------------------------------------------

var lclosureFreeList []*LClosure

// getLClosure gets an LClosure from the freelist or allocates a new one.
// The returned closure has zeroed GCHeader and nil Proto/UpVals.
func getLClosure() *LClosure {
	n := len(lclosureFreeList)
	if n > 0 {
		cl := lclosureFreeList[n-1]
		lclosureFreeList = lclosureFreeList[:n-1]
		cl.GCHeader = object.GCHeader{}
		cl.Proto = nil
		cl.UpVals = nil
		return cl
	}
	cl := &LClosure{}
	cl.GCHeader = object.GCHeader{}
	return cl
}

// PutLClosure returns an LClosure to the pool for reuse.
// Called by the GC sweep phase when a dead closure is unlinked.
// Clears all reference fields before pooling to help Go's GC.
func PutLClosure(cl *LClosure) {
	cl.Proto = nil
	cl.UpVals = nil
	cl.GCHeader = object.GCHeader{}
	lclosureFreeList = append(lclosureFreeList, cl)
}

// ---------------------------------------------------------------------------
// UpVal pool
// ---------------------------------------------------------------------------

var upvalFreeList []*UpVal

// getUpVal gets an UpVal from the freelist or allocates a new one.
// The returned upval is fully zeroed.
func getUpVal() *UpVal {
	n := len(upvalFreeList)
	if n > 0 {
		uv := upvalFreeList[n-1]
		upvalFreeList = upvalFreeList[:n-1]
		uv.GCHeader = object.GCHeader{}
		uv.StackIdx = 0
		uv.Own = object.Nil
		uv.Next = nil
		uv.Stack = nil
		return uv
	}
	uv := &UpVal{}
	uv.GCHeader = object.GCHeader{}
	return uv
}

// PutUpVal returns an UpVal to the pool for reuse.
// Called by the GC sweep phase when a dead upval is unlinked.
// Clears all reference fields before pooling to help Go's GC.
func PutUpVal(uv *UpVal) {
	uv.Own = object.Nil
	uv.Next = nil
	uv.Stack = nil
	uv.GCHeader = object.GCHeader{}
	upvalFreeList = append(upvalFreeList, uv)
}
