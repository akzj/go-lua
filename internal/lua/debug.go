package lua

/*
** $Id: debug.go $
** Debug library
** Ported from ldblib.c
*/

import (
	"fmt"
	"unsafe"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lauxlib"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** Debug library functions
*/

// luaB_debug - debug hook
func luaB_debug(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	for i := 1; i <= lapi.Lua_gettop(LS); i++ {
		s := lapi.Lua_tolstring(LS, i, nil)
		if i > 1 {
			fmt.Print("\t")
		}
		fmt.Print(s)
	}
	fmt.Println()
	return 0
}

// luaB_traceback - get traceback
func luaB_traceback(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	msg := ""
	if !lapi.Lua_isnoneornil(LS, 1) {
		msg = lapi.Lua_tolstring(LS, 1, nil)
	}
	
	trace := fmt.Sprintf("stack traceback:\n%s\n", msg)
	lapi.Lua_pushstring(LS, trace)
	return 1
}

// luaB_getlocal - get local variable
func luaB_getlocal(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	_ = LS
	lapi.Lua_pushnil(LS)
	return 1
}

// luaB_setlocal - set local variable
func luaB_setlocal(L *lobject.LuaState) int {
	return 0
}

// luaB_getinfo - get function info
func luaB_getinfo(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	what := lapi.Lua_tolstring(LS, 2, nil)
	
	lapi.Lua_createtable(LS, 0, 0)
	
	for i := 0; i < len(what); i++ {
		switch what[i] {
		case 'n':
			lapi.Lua_pushstring(LS, "")
			lapi.Lua_setfield(LS, -2, "name")
		case 'f':
			if !lapi.Lua_isnoneornil(LS, 1) {
				lapi.Lua_pushvalue(LS, 1)
			} else {
				lapi.Lua_pushnil(LS)
			}
			lapi.Lua_setfield(LS, -2, "func")
		case 'S':
			lapi.Lua_pushstring(LS, "Lua")
			lapi.Lua_setfield(LS, -2, "what")
			lapi.Lua_pushstring(LS, "[string]")
			lapi.Lua_setfield(LS, -2, "source")
		case 'l':
			lapi.Lua_pushinteger(LS, 0)
			lapi.Lua_setfield(LS, -2, "currentline")
		case 'u':
			lapi.Lua_pushinteger(LS, 0)
			lapi.Lua_setfield(LS, -2, "nups")
		}
	}
	return 1
}

// luaB_getupvalue - get upvalue
func luaB_getupvalue(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	_ = LS
	lapi.Lua_pushnil(LS)
	return 1
}

// luaB_setupvalue - set upvalue
func luaB_setupvalue(L *lobject.LuaState) int {
	return 0
}

// luaB_upvalueid - get unique id for upvalue
func luaB_upvalueid(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_pushlightuserdata(LS, nil)
	return 1
}

// luaB_upvaluejoin - join upvalues
func luaB_upvaluejoin(L *lobject.LuaState) int {
	return 0
}

// luaB_setuservalue - set user value
func luaB_setuservalue(L *lobject.LuaState) int {
	return 0
}

// luaB_getuservalue - get user value
func luaB_getuservalue(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	_ = LS
	lapi.Lua_pushnil(LS)
	return 1
}

// luaB_sethook - set debug hook
func luaB_sethook(L *lobject.LuaState) int {
	return 0
}

// luaB_gethook - get debug hook
func luaB_gethook(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	_ = LS
	lapi.Lua_pushnil(LS)
	return 1
}

// luaB_hookcount - get hook count
func luaB_hookcount(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_pushinteger(LS, 0)
	return 1
}

// luaB_hookmask - get hook mask
func luaB_hookmask(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_pushinteger(LS, 0)
	return 1
}

/*
** Debug library functions
*/
var debuglibs = []lauxlib.LuaL_Reg{
	{"debug", luaB_debug},
	{"traceback", luaB_traceback},
	{"getlocal", luaB_getlocal},
	{"setlocal", luaB_setlocal},
	{"getinfo", luaB_getinfo},
	{"getupvalue", luaB_getupvalue},
	{"setupvalue", luaB_setupvalue},
	{"upvalueid", luaB_upvalueid},
	{"upvaluejoin", luaB_upvaluejoin},
	{"setuservalue", luaB_setuservalue},
	{"getuservalue", luaB_getuservalue},
	{"sethook", luaB_sethook},
	{"gethook", luaB_gethook},
	{"hookcount", luaB_hookcount},
	{"hookmask", luaB_hookmask},
}

/*
** OpenDebug - open debug library
 */
func OpenDebug(L *lstate.LuaState) {
	lauxlib.LuaL_newlib(L, debuglibs)
}
