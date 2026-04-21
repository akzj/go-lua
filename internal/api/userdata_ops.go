// userdata_ops.go — Userdata and upvalue access operations.
package api

import (
	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// Userdata
// ---------------------------------------------------------------------------

// NewUserdata creates a new full userdata with nUV user values and pushes it.
// Returns the Userdata object. size is ignored (Go manages memory).
func (L *State) NewUserdata(size int, nUV int) *object.Userdata {
	ud := &object.Userdata{
		UserVals: make([]object.TValue, nUV),
	}
	// Initialize user values to nil
	for i := range ud.UserVals {
		ud.UserVals[i] = object.Nil
	}
	L.ls().Global.LinkGC(ud) // V5: register in allgc chain
	// Track allocation: base Userdata struct (~80 bytes) + UserVals slice
	estimateSize := int64(80 + nUV*24)
	L.TrackAlloc(estimateSize)
	L.push(object.TValue{Tt: object.TagUserdata, Obj: ud})
	return ud
}

// ---------------------------------------------------------------------------
// Upvalue access
// ---------------------------------------------------------------------------

// GetUpvalue pushes the value of upvalue n of the closure at funcIdx.
// Returns (name, true) if upvalue exists, ("", false) if not.
// C closure upvalues are always named "" (empty string).
func (L *State) GetUpvalue(funcIdx, n int) (string, bool) {
	v := L.index2val(funcIdx)
	if v == nil {
		return "", false
	}
	switch v.Tt {
	case object.TagLuaClosure:
		cl := v.Obj.(*closure.LClosure)
		if n < 1 || n > len(cl.UpVals) {
			return "", false
		}
		uv := cl.UpVals[n-1]
		if uv == nil {
			return "", false
		}
		val := uv.Get(L.ls().Stack)
		L.push(val)
		// Return the name from Proto.Upvalues debug info
		if n-1 < len(cl.Proto.Upvalues) && cl.Proto.Upvalues[n-1].Name != nil {
			return cl.Proto.Upvalues[n-1].Name.String(), true
		}
		return "(no name)", true
	case object.TagCClosure:
		cc := v.Obj.(*closure.CClosure)
		if n < 1 || n > len(cc.UpVals) {
			return "", false
		}
		L.push(cc.UpVals[n-1])
		return "", true // C closures have no upvalue names — always ""
	}
	return "", false
}

// SetUpvalue sets upvalue n of the closure at funcIdx from the top value.
// Returns (name, true) if upvalue exists, ("", false) if not.
func (L *State) SetUpvalue(funcIdx, n int) (string, bool) {
	v := L.index2val(funcIdx)
	if v == nil {
		return "", false
	}
	switch v.Tt {
	case object.TagLuaClosure:
		cl := v.Obj.(*closure.LClosure)
		if n < 1 || n > len(cl.UpVals) {
			return "", false
		}
		uv := cl.UpVals[n-1]
		if uv == nil {
			return "", false
		}
		val := L.index2val(-1)
		if val != nil {
			uv.Set(L.ls().Stack, *val)
		}
		L.Pop(1)
		if n-1 < len(cl.Proto.Upvalues) && cl.Proto.Upvalues[n-1].Name != nil {
			return cl.Proto.Upvalues[n-1].Name.String(), true
		}
		return "(no name)", true
	case object.TagCClosure:
		cc := v.Obj.(*closure.CClosure)
		if n < 1 || n > len(cc.UpVals) {
			return "", false
		}
		val := L.index2val(-1)
		if val != nil {
			cc.UpVals[n-1] = *val
		}
		L.Pop(1)
		return "", true
	}
	return "", false
}
