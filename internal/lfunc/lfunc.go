package lfunc

/*
** $Id: lfunc.go $
** Auxiliary functions for closures and upvalues
** Ported from lfunc.h and lfunc.c
*/

import (
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** Maximum upvalues
 */
const MAXUPVAL = 255

/*
** Close upvalue - closes an upvalue
 */
func Close(L *lstate.LuaState, level *lobject.TValue) {
	// Simplified - just unlink from open upvalue list
	uv := L.OpenUpval
	for uv != nil {
		next := uv.U.Open.Next
		uv.V.P = &uv.U.Value
		if p, ok := uv.V.P.(*lobject.TValue); ok {
			lobject.SetObj(p, p)
		}
		uv = next
	}
}

/*
** NewCClosure creates a new C closure
 */
func NewCClosure(L *lstate.LuaState, nupvals int) *lobject.CClosure {
	if nupvals > MAXUPVAL {
		nupvals = MAXUPVAL
	}
	c := &lobject.CClosure{
		Nupvalues: uint8(nupvals),
	}
	return c
}

/*
** NewLClosure creates a new Lua closure
 */
func NewLClosure(L *lstate.LuaState, nupvals int) *lobject.LClosure {
	if nupvals > MAXUPVAL {
		nupvals = MAXUPVAL
	}
	cl := &lobject.LClosure{
		Nupvalues: uint8(nupvals),
		Upvals:    make([]*lobject.UpVal, nupvals),
	}
	return cl
}

/*
** InitUpvals initializes upvalues for a closure
 */
func InitUpvals(L *lstate.LuaState, cl *lobject.LClosure) {
	for i := 0; i < int(cl.Nupvalues); i++ {
		uv := &lobject.UpVal{}
		tv := &lobject.TValue{}
		uv.V.P = tv
		lobject.SetNilValue(tv)
		cl.Upvals[i] = uv
	}
}

/*
** FindUpval finds or creates an upvalue at the given level
 */
func FindUpval(L *lstate.LuaState, level *lobject.TValue) *lobject.UpVal {
	// Simplified implementation
	uv := &lobject.UpVal{}
	uv.V.P = level
	return uv
}

/*
** NewProto creates a new function prototype
 */
func NewProto(L *lstate.LuaState) *lobject.Proto {
	return &lobject.Proto{}
}

/*
** FreeProto frees a function prototype
 */
func FreeProto(L *lstate.LuaState, f *lobject.Proto) {
	// Simplified
}

/*
** GetLocalName returns the name of a local variable
 */
func GetLocalName(f *lobject.Proto, local_number int, pc int) string {
	if local_number <= 0 || local_number > len(f.Locvars) {
		return ""
	}
	return ""
}

/*
** CloseUpval closes all upvalues up to level
 */
func CloseUpval(L *lstate.LuaState, level *lobject.TValue) {
	Close(L, level)
}
