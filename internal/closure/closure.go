// Upvalue management: find, close, and lifecycle operations.
//
// Reference: lua-master/lfunc.c
package closure

import (
	objectapi "github.com/akzj/go-lua/internal/object"
	stateapi "github.com/akzj/go-lua/internal/state"
)

// ---------------------------------------------------------------------------
// FindUpval — find or create an open upvalue at the given stack level.
//
// The open upvalue list (L.OpenUpval) is sorted by StackIdx in descending
// order. If an upvalue already exists at this level, it is returned
// (upvalue sharing). Otherwise, a new one is created and inserted in
// sorted position.
//
// Mirrors: luaF_findupval in lfunc.c
// ---------------------------------------------------------------------------

// FindUpval finds or creates an open upvalue at the given stack level.
func FindUpval(L *stateapi.LuaState, level int) *UpVal {
	// Walk the open upvalue list (sorted descending by StackIdx)
	var prev *UpVal
	p := openUpvalHead(L)

	for p != nil && p.StackIdx >= level {
		if p.StackIdx == level {
			return p // found existing — share it
		}
		prev = p
		p = p.Next
	}

	// Not found — create new upvalue at this level
	uv := &UpVal{
		StackIdx: level,
		Own:      objectapi.Nil,
		Next:     p,     // link to rest of list (lower levels)
		Stack:    &L.Stack, // reference to owning thread's stack for cross-thread access
	}
	L.Global.LinkGC(uv) // V5: register in allgc chain

	// Insert into the list
	if prev == nil {
		// Insert at head
		L.OpenUpval = uv
	} else {
		prev.Next = uv
	}

	return uv
}

// ---------------------------------------------------------------------------
// CloseUpvals — close all open upvalues at or above the given stack level.
//
// Walks from the head of the open upvalue list. For each upvalue with
// StackIdx >= level, copies the stack value into the upvalue's Own field
// and marks it as closed (StackIdx = -1).
//
// Mirrors: luaF_closeupval in lfunc.c
// ---------------------------------------------------------------------------

// CloseUpvals closes all open upvalues at or above the given stack level.
func CloseUpvals(L *stateapi.LuaState, level int) {
	for {
		uv := openUpvalHead(L)
		if uv == nil || uv.StackIdx < level {
			break
		}

		// Remove from open list
		if uv.Next == nil {
			// Last upvalue — set to untyped nil to avoid typed-nil-in-interface trap
			L.OpenUpval = nil
		} else {
			L.OpenUpval = uv.Next
		}

		// Close: capture the current stack value
		if uv.StackIdx >= 0 && uv.StackIdx < len(L.Stack) {
			uv.Close(L.Stack[uv.StackIdx].Val)
		} else {
			uv.Close(objectapi.Nil)
		}
	}
}

// ---------------------------------------------------------------------------
// InitUpvals — fill a closure with new closed (nil) upvalues.
//
// Used when creating the main closure or when loading bytecode.
// Each upvalue is created in closed state with nil value.
//
// Mirrors: luaF_initupvals in lfunc.c
// ---------------------------------------------------------------------------

// InitUpvals fills all nil upvalue slots in the closure with new closed upvalues.
// The caller is responsible for linking upvalues into allgc after this returns.
func InitUpvals(cl *LClosure) {
	for i := range cl.UpVals {
		if cl.UpVals[i] == nil {
			cl.UpVals[i] = &UpVal{
				StackIdx: -1, // closed
				Own:      objectapi.Nil,
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Helper: access the open upvalue list head with type assertion.
// L.OpenUpval is typed as `any` to avoid import cycles.
// ---------------------------------------------------------------------------

// openUpvalHead returns the head of the open upvalue list, or nil.
func openUpvalHead(L *stateapi.LuaState) *UpVal {
	if L.OpenUpval == nil {
		return nil
	}
	return L.OpenUpval.(*UpVal)
}
