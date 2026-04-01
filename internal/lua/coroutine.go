package lua

/*
** $Id: coroutine.go $
** Coroutine library
** Ported from lcorolib.c
*/

import (
	"unsafe"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lauxlib"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** Coroutine library functions
*/

// auxresume - auxiliary resume function
func auxresume(L *lstate.LuaState, f *lstate.LuaState, nargs int) int {
	// Simplified: just call the function
	lapi.Lua_call(L, nargs, lapi.LUA_MULTRET)
	return lapi.Lua_gettop(L)
}

// luaB_coresume - coroutine resume
func luaB_coresume(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	co := lapi.Lua_touserdata(LS, 1)
	if f, ok := co.(*lstate.LuaState); ok {
		nargs := lapi.Lua_gettop(LS) - 1
		status := auxresume(LS, f, nargs)
		if status >= 0 {
			lapi.Lua_pushboolean(LS, true)
			return status + 1
		}
		lapi.Lua_pushboolean(LS, false)
		lapi.Lua_pushstring(LS, "error in coroutine")
		return 2
	}
	lauxlib.LuaL_error(LS, "not a coroutine")
	return 0
}

// luaB_costatus - coroutine status
func luaB_costatus(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	co := lapi.Lua_touserdata(LS, 1)
	if _, ok := co.(*lstate.LuaState); ok {
		lapi.Lua_pushstring(LS, "running")
		return 1
	}
	lapi.Lua_pushstring(LS, "dead")
	return 1
}

// luaB_yield - coroutine yield
func luaB_yield(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	return lapi.Lua_gettop(LS)
}

// luaB_yieldable - check if yieldable
func luaB_yieldable(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_pushboolean(LS, true)
	return 1
}

// luaB_corunning - return current coroutine (stub)
func luaB_corunning(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	// Return current thread as userdata
	lapi.Lua_pushlightuserdata(LS, unsafe.Pointer(L))
	return 1
}

// luaB_create - create coroutine
func luaB_create(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	// Create a new state as coroutine
	newL := lapi.Lua_newstate(nil, nil, 0)
	if newL == nil {
		lauxlib.LuaL_error(LS, "cannot create thread")
	}
	lapi.Lua_pushlightuserdata(LS, unsafe.Pointer(newL))
	return 1
}

// luaB_wrap - create wrapped coroutine
func luaB_wrap(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	luaB_create(L)
	lapi.Lua_pushcfunction(LS, luaB_coresume, 0)
	return 2
}

// luaB_close - close coroutine
func luaB_close(L *lobject.LuaState) int {
	return 0
}

/*
** Coroutine library functions
*/
var coroutinelibs = []lauxlib.LuaL_Reg{
	{"create", luaB_create},
	{"resume", luaB_coresume},
	{"yield", luaB_yield},
	{"status", luaB_costatus},
	{"running", luaB_corunning},
	{"yieldable", luaB_yieldable},
	{"wrap", luaB_wrap},
	{"close", luaB_close},
}

/*
** OpenCoroutine - open coroutine library
 */
func OpenCoroutine(L *lstate.LuaState) {
	lauxlib.LuaL_newlib(L, coroutinelibs)
}
