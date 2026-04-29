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
// PutLuaState guarantees all fields are zeroed before returning to the pool,
// so no per-field clearing is needed here. Stack and CISlab retain capacity
// from a previous use (reused in stackInit and NewCallInfo).
func getLuaState() *LuaState {
	return luaStatePool.Get().(*LuaState)
}

// PutLuaState returns a LuaState to the pool for reuse.
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
	luaStatePool.Put(L)
}
