// LuaState object pool — reuses dead thread structs to reduce allocation pressure.
//
// Coroutines are the most expensive objects to create in go-lua because each
// LuaState includes a large stack slice (~200 StackValues). Using sync.Pool
// lets us reuse both the struct and the stack slice for short-lived coroutines.
package state

import (
	"sync"

	"github.com/akzj/go-lua/internal/object"
)

var luaStatePool = sync.Pool{
	New: func() any {
		return &LuaState{}
	},
}

// getLuaState gets a LuaState from the pool or allocates a new one.
// The returned state has zeroed fields except Stack which may retain
// capacity from a previous use (reused in stackInit).
func getLuaState() *LuaState {
	L := luaStatePool.Get().(*LuaState)
	// Zero the GCHeader
	L.GCHeader = object.GCHeader{}
	// Scalar fields
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
	L.CISlabIdx = 0
	// Pointer fields — clear to prevent GC retention
	L.CI = nil
	L.Global = nil
	L.OpenUpval = nil
	L.Hook = nil
	L.APIState = nil
	L.CISlab = nil
	// BaseCI is embedded — zero it
	L.BaseCI = CallInfo{}
	// NOTE: L.Stack is intentionally NOT cleared here.
	// stackInit will reuse its capacity if sufficient, or allocate new.
	return L
}

// PutLuaState returns a LuaState to the pool for reuse.
// Called by the GC sweep phase when a dead thread is unlinked.
// Clears all reference fields before pooling to help Go's GC,
// but retains the Stack slice capacity for reuse.
func PutLuaState(L *LuaState) {
	// Clear all reference-bearing fields to avoid retaining garbage
	L.GCHeader = object.GCHeader{}
	L.Global = nil
	L.OpenUpval = nil
	L.Hook = nil
	L.APIState = nil
	L.CI = nil
	L.CISlab = nil
	L.BaseCI = CallInfo{}
	// Clear stack values but keep the backing array
	for i := range L.Stack {
		L.Stack[i] = object.StackValue{}
	}
	// Reslice to zero length but keep capacity
	L.Stack = L.Stack[:0]
	luaStatePool.Put(L)
}
