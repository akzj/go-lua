// Closure and UpVal object pools — reuses dead structs to reduce allocation pressure.
//
// LClosures and UpVals are the second and third most frequently allocated GC
// objects (after tables). Using sync.Pool lets us reuse the struct memory for
// short-lived closures/upvals instead of going through Go's mallocgc each time.
package closure

import (
	"sync"

	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// LClosure pool
// ---------------------------------------------------------------------------

var lclosurePool = sync.Pool{
	New: func() any {
		return &LClosure{}
	},
}

// getLClosure gets an LClosure from the pool or allocates a new one.
// The returned closure has zeroed GCHeader and nil Proto/UpVals.
func getLClosure() *LClosure {
	cl := lclosurePool.Get().(*LClosure)
	cl.GCHeader = object.GCHeader{}
	cl.Proto = nil
	cl.UpVals = nil
	return cl
}

// PutLClosure returns an LClosure to the pool for reuse.
// Called by the GC sweep phase when a dead closure is unlinked.
// Clears all reference fields before pooling to help Go's GC.
func PutLClosure(cl *LClosure) {
	cl.Proto = nil
	cl.UpVals = nil
	cl.GCHeader = object.GCHeader{}
	lclosurePool.Put(cl)
}

// ---------------------------------------------------------------------------
// UpVal pool
// ---------------------------------------------------------------------------

var upvalPool = sync.Pool{
	New: func() any {
		return &UpVal{}
	},
}

// getUpVal gets an UpVal from the pool or allocates a new one.
// The returned upval is fully zeroed.
func getUpVal() *UpVal {
	uv := upvalPool.Get().(*UpVal)
	uv.GCHeader = object.GCHeader{}
	uv.StackIdx = 0
	uv.Own = object.Nil
	uv.Next = nil
	uv.Stack = nil
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
	upvalPool.Put(uv)
}
