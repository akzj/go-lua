// coroutine_impl.go — Coroutine API implementation (NewThread, Resume, Yield, Status).
package api

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/akzj/go-lua/internal/object"

	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/vm"
)

// ---------------------------------------------------------------------------
// Coroutine API
// ---------------------------------------------------------------------------

// NewThread creates a new Lua thread (coroutine), pushes it on the stack,
// and returns a *State representing the new thread.
// Mirrors: lua_newthread in lstate.c
func (L *State) NewThread() *State {
	ls := L.ls()
	L1 := state.NewThread(ls)
	// Push the new thread onto the parent's stack
	L.push(object.TValue{Tt: object.TagThread, Obj: L1})
	return &State{Internal: L1}
}

// PushThread pushes the running thread onto its own stack.
// Returns true if the thread is the main thread.
func (L *State) PushThread() bool {
	ls := L.ls()
	L.push(object.TValue{Tt: object.TagThread, Obj: ls})
	return ls.Global.MainThread == ls || ls.Global.MainThread == nil
}

// Resume starts or resumes a coroutine.
// Returns (status, nresults) matching lua_resume in ldo.c.
// status is StatusOK (finished) or StatusYield (suspended) on success,
// or an error status on failure.
func (L *State) Resume(from *State, nArgs int) (int, int) {
	ls := L.ls()
	var fromLS *state.LuaState
	if from != nil {
		fromLS = from.ls()
	}
	status, nresults := vm.Resume(ls, fromLS, nArgs)
	return status, nresults
}

// YieldK yields a coroutine with a continuation function.
// Mirrors: lua_yieldk in ldo.c
func (L *State) YieldK(nResults int, ctx int, k CFunction) int {
	ls := L.ls()
	if k != nil {
		ci := ls.CI
		ci.K = func(innerL *state.LuaState, status int, context int) int {
			wrapper := &State{Internal: innerL}
			return k(wrapper)
		}
		ci.Ctx = ctx
	}
	vm.Yield(ls, nResults)
	return 0 // unreachable — Yield panics with LuaYield
}

// Yield yields a coroutine (no continuation).
func (L *State) Yield(nResults int) int {
	return L.YieldK(nResults, 0, nil)
}

// IsYieldable returns true if the running coroutine can yield.
func (L *State) IsYieldable() bool {
	return L.ls().Yieldable()
}

// XMove moves n values from L's stack to to's stack.
// Mirrors: lua_xmove in lapi.c
func (L *State) XMove(to *State, n int) {
	if L == to {
		return
	}
	fromLS := L.ls()
	toLS := to.ls()
	fromLS.Top -= n
	for i := 0; i < n; i++ {
		state.PushValue(toLS, fromLS.Stack[fromLS.Top+i].Val)
	}
}

// ToThread converts the value at the given index to a *State (thread).
// Returns nil if the value is not a thread.
func (L *State) ToThread(idx int) *State {
	v := L.index2val(idx)
	if v.Tt != object.TagThread {
		return nil
	}
	ls, ok := v.Obj.(*state.LuaState)
	if !ok {
		return nil
	}
	return &State{Internal: ls}
}

// Status returns the status of the coroutine L.
func (L *State) Status() int {
	return L.ls().Status
}

// SetStatus sets the thread status (used by test library for panic simulation).
func (L *State) SetStatus(status int) {
	L.ls().Status = status
}

// ---------------------------------------------------------------------------
// Userdata API (stubs)
// ---------------------------------------------------------------------------

func (L *State) ToUserdata(idx int) interface{} {
	v := L.index2val(idx)
	if v == nil {
		return nil
	}
	switch v.Tt {
	case object.TagUserdata:
		ud, ok := v.Obj.(*object.Userdata)
		if ok {
			return ud.Data
		}
		return nil
	case object.TagLightUserdata:
		return v.Obj
	default:
		return nil
	}
}

// GetUserdataObj returns the full Userdata struct at idx, or nil if not userdata.
func (L *State) GetUserdataObj(idx int) *object.Userdata {
	v := L.index2val(idx)
	if v == nil || v.Tt != object.TagUserdata {
		return nil
	}
	ud, ok := v.Obj.(*object.Userdata)
	if !ok {
		return nil
	}
	return ud
}

// GetIUserValue pushes the n-th user value of the userdata at idx onto the stack.
// Returns the type of the pushed value, or TypeNone if invalid.
// For non-full-userdata or invalid n, pushes nil and returns TypeNone.
// Mirrors: lua_getiuservalue in lapi.c
func (L *State) GetIUserValue(idx int, n int) object.Type {
	ls := L.ls()
	vm.CheckStack(ls, 1)
	v := L.index2val(idx)
	if v != nil && v.Tt == object.TagUserdata {
		ud, ok := v.Obj.(*object.Userdata)
		if ok && n >= 1 && n <= len(ud.UserVals) {
			val := ud.UserVals[n-1]
			ls.Stack[ls.Top].Val = val
			ls.Top++
			return object.Type(val.Tt & 0x0F)
		}
	}
	// Push nil for failure
	ls.Stack[ls.Top].Val = object.Nil
	ls.Top++
	return object.TypeNone
}

// SetIUserValue sets the n-th user value of the userdata at idx to the value
// at the top of the stack. Pops the value. Returns false if the operation fails.
// Mirrors: lua_setiuservalue in lapi.c
func (L *State) SetIUserValue(idx int, n int) bool {
	ls := L.ls()
	v := L.index2val(idx)
	if v != nil && v.Tt == object.TagUserdata {
		ud, ok := v.Obj.(*object.Userdata)
		if ok && n >= 1 && n <= len(ud.UserVals) {
			ls.Top--
			ud.UserVals[n-1] = ls.Stack[ls.Top].Val
			return true
		}
	}
	ls.Top-- // pop even on failure
	return false
}
func (L *State) ToPointer(idx int) string {
	v := L.index2val(idx)
	if v == nil || v.Obj == nil {
		return ""
	}
	rv := reflect.ValueOf(v.Obj)
	switch rv.Kind() {
	case reflect.Ptr:
		return fmt.Sprintf("0x%x", rv.Pointer())
	case reflect.Func:
		// reflect.Pointer() returns code entry point — same for all closures
		// of the same template. Use interface data word for unique pointer.
		type eface struct {
			_type uintptr
			data  uintptr
		}
		iface := v.Obj
		ef := (*eface)(unsafe.Pointer(&iface))
		return fmt.Sprintf("0x%x", ef.data)
	default:
		return ""
	}
}
