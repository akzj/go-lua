package ldo

/*
** $Id: ldo.go $
** Stack and Call structure
** Ported from ldo.h and ldo.c
*/

import (
	"unsafe"

	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** Stack checking macros
 */
const LUAI_MAXCCALLS = 200

// Checkstack ensures there is extra stack space
func Checkstackaux(L *lstate.LuaState, n int) {
	if uintptr(unsafe.Pointer(L.StackLast.P))-uintptr(unsafe.Pointer(L.Top.P)) <= uintptr(n) {
		Growstack(L, n)
	}
}

// Savestack saves stack position as offset
func Savestack(L *lstate.LuaState, pt *lobject.TValue) int {
	return lstate.Savestack(L, pt)
}

// Restorestack restores stack position from offset
func Restorestack(L *lstate.LuaState, n int) *lobject.TValue {
	return lstate.Restorestack(L, n)
}

// Growstack grows the stack
func Growstack(L *lstate.LuaState, n int) {
	oldsize := len(L.Stack)
	newsize := oldsize + n
	if newsize < oldsize*3/2 {
		newsize = oldsize * 3 / 2
	}
	if newsize < lstate.BASIC_STACK_SIZE {
		newsize = lstate.BASIC_STACK_SIZE
	}
	newstack := make([]lobject.StackValue, newsize+lstate.EXTRA_STACK)
	copy(newstack, L.Stack)
	for i := len(L.Stack); i < len(newstack); i++ {
		lobject.SetNilValue(&newstack[i].Val)
	}
	L.Stack = newstack
	L.StackLast.P = &L.Stack[lstate.BASIC_STACK_SIZE].Val
}

// Reallocstack reallocates the stack
func Reallocstack(L *lstate.LuaState, size int) {
	Growstack(L, size)
}

// IncTop increments the top of stack
func IncTop(L *lstate.LuaState) {
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) + uintptr(unsafe.Sizeof(lobject.TValue{}))))
}

// Checkstack checks stack space
func Checkstack(L *lstate.LuaState, n int) {
	Checkstackaux(L, n)
}

// IsInMainThread checks if this is the main thread
func IsInMainThread(L *lstate.LuaState) bool {
	return lstate.MainThreadPtr(L.G) == L
}

// Yieldable checks if thread can yield
func Yieldable(L *lstate.LuaState) bool {
	return lstate.Yieldable(L)
}

// Incnny increments non-yieldable call counter
func Incnny(L *lstate.LuaState) {
	lstate.Incnny(L)
}

// Decnny decrements non-yieldable call counter
func Decnny(L *lstate.LuaState) {
	lstate.Decnny(L)
}

// Getnresults gets number of results from call status
func Getnresults(cs uint32) int {
	return lstate.Getnresults(cs)
}

// Seterrorobj sets the error object on the stack
func Seterrorobj(L *lstate.LuaState, errcode lobject.TStatus, oldtop *lobject.TValue) {
	if errcode == lobject.LUA_ERRMEM {
		// Reuse pre-registered message
	} else {
		lobject.SetObj(oldtop, L.Top.P)
	}
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(oldtop)) + uintptr(unsafe.Sizeof(lobject.TValue{}))))
}

// Throw raises an error
func Throw(L *lstate.LuaState, errcode lobject.TStatus) {
	panic(errcode)
}

// BaseCcalls returns base C call count
func BaseCcalls(L *lstate.LuaState) int {
	return L.BaseCcalls
}

// Setbasectx sets base context
func Setbasectx(L *lstate.LuaState, n int) {
	L.BaseCcalls = n
}

// CloseState closes the Lua state
func CloseState(L *lstate.LuaState) {
	// Free resources
}

// RawRunProtected runs a function in protected mode
func RawRunProtected(L *lstate.LuaState, f func(*lstate.LuaState, interface{}), ud interface{}) bool {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(lobject.TStatus); ok {
				L.Status = err
			}
		}
	}()
	L.Status = lobject.LUA_OK
	f(L, ud)
	return L.Status == lobject.LUA_OK
}
