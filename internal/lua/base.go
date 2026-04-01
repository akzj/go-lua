package lua

/*
** $Id: base.go $
** Basic library
** Ported from lbaselib.c
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
** Constants
 */
const (
	LUA_GNAME   = "_G"
	LUA_VERSION = "Lua 5.4"
)

// printOutput is the output function for print()
var printOutput func(string) = func(s string) { fmt.Println(s) }

// SetPrintOutput sets the output function for print()
func SetPrintOutput(f func(string)) {
	printOutput = f
}

// luaB_print - print function
func luaB_print(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_gettop(LS)
	for i := 1; i <= n; i++ {
		if i > 1 {
			fmt.Print("\t")
		}
		s := lapi.Lua_tolstring(LS, i, nil)
		fmt.Print(s)
	}
	fmt.Println()
	return 0
}

// luaB_tonumber - convert to number
func luaB_tonumber(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	if lapi.Lua_type(LS, 1) == lobject.LUA_TNUMBER {
		lapi.Lua_pushnumber(LS, lapi.Lua_tonumberx(LS, 1, nil))
		return 1
	}
	s := lapi.Lua_tolstring(LS, 1, nil)
	var n lobject.LuaNumber
	_, err := fmt.Sscan(s, &n)
	if err == nil {
		lapi.Lua_pushnumber(LS, n)
		return 1
	}
	return 0
}

// luaB_error - error function
func luaB_error(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	msg := lapi.Lua_tolstring(LS, -1, nil)
	panic(msg)
}

// luaB_getmetatable - get metatable
func luaB_getmetatable(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	if lapi.Lua_getmetatable(LS, 1) == 0 {
		lapi.Lua_pushnil(LS)
		return 1
	}
	return 1
}

// luaB_setmetatable - set metatable
func luaB_setmetatable(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_setmetatable(LS, 1)
	return 1
}

// luaB_rawequal - raw equal
func luaB_rawequal(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_pushboolean(LS, lapi.Lua_rawequal(LS, 1, 2) != 0)
	return 1
}

// luaB_rawlen - raw length
func luaB_rawlen(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_rawlen(LS, 1)
	lapi.Lua_pushinteger(LS, lobject.LuaInteger(n))
	return 1
}

// luaB_rawget - raw get
func luaB_rawget(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_settop(LS, 2)
	lapi.Lua_rawget(LS, 1)
	return 1
}

// luaB_rawset - raw set
func luaB_rawset(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_settop(LS, 3)
	lapi.Lua_rawset(LS, 1)
	return 1
}

// luaB_type - get type
func luaB_type(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	t := lapi.Lua_type(LS, 1)
	lapi.Lua_pushstring(LS, lapi.Lua_typename(LS, t))
	return 1
}

// luaB_tostring - tostring function
func luaB_tostring(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	if s != "" {
		return 1
	}
	lapi.Lua_pushstring(LS, lapi.Lua_typename(LS, lapi.Lua_type(LS, 1)))
	return 1
}

// luaB_assert - assert function
func luaB_assert(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	if lapi.Lua_toboolean(LS, 1) == 0 {
		panic("assertion failed!")
	}
	return lapi.Lua_gettop(LS)
}

// luaB_collectgarbage - garbage collection (stub)
func luaB_collectgarbage(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_pushinteger(LS, 0)
	return 1
}

// luaB_next - next in table (stub)
func luaB_next(L *lobject.LuaState) int {
	return 0
}

// luaB_pairs - pairs iterator
func luaB_pairs(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_pushcfunction(LS, luaB_next, 0)
	lapi.Lua_pushvalue(LS, 1)
	lapi.Lua_pushnil(LS)
	return 3
}

// luaB_ipairs_aux - ipairs auxiliary
func luaB_ipairs_aux(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	i := lapi.Lua_tointeger(LS, 2) + 1
	lapi.Lua_pushinteger(LS, i)
	if lapi.Lua_geti(LS, 1, i) == lobject.LUA_TNIL {
		lapi.Lua_pop(LS, 1)
		return 1
	}
	return 2
}

// luaB_ipairs - ipairs iterator
func luaB_ipairs(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_pushcfunction(LS, luaB_ipairs_aux, 0)
	lapi.Lua_pushvalue(LS, 1)
	lapi.Lua_pushinteger(LS, 0)
	return 3
}

// luaB_select - select function
func luaB_select(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_gettop(LS)
	if lapi.Lua_type(LS, 1) == lobject.LUA_TSTRING {
		lapi.Lua_pushinteger(LS, int64(n-1))
		return 1
	}
	i := lapi.Lua_tointeger(LS, 1)
	if i < 0 {
		i = int64(n) + i
	}
	lapi.Lua_insert(LS, 1)
	return n - int(i)
}

// luaB_pcall - protected call (stub)
func luaB_pcall(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_gettop(LS)
	lapi.Lua_call(LS, n-1, lapi.LUA_MULTRET)
	return lapi.Lua_gettop(LS)
}

// luaB_xpcall - xpcall (stub)
func luaB_xpcall(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_gettop(LS)
	lapi.Lua_call(LS, n-2, lapi.LUA_MULTRET)
	return lapi.Lua_gettop(LS)
}

// baselibs - base library functions
var baselibs = []lauxlib.LuaL_Reg{
	{"print", luaB_print},
	{"tonumber", luaB_tonumber},
	{"tostring", luaB_tostring},
	{"error", luaB_error},
	{"type", luaB_type},
	{"getmetatable", luaB_getmetatable},
	{"setmetatable", luaB_setmetatable},
	{"rawequal", luaB_rawequal},
	{"rawlen", luaB_rawlen},
	{"rawget", luaB_rawget},
	{"rawset", luaB_rawset},
	{"collectgarbage", luaB_collectgarbage},
	{"pairs", luaB_pairs},
	{"next", luaB_next},
	{"ipairs", luaB_ipairs},
	{"select", luaB_select},
	{"pcall", luaB_pcall},
	{"xpcall", luaB_xpcall},
	{"assert", luaB_assert},
}

// OpenBase - open base library
func OpenBase(L *lstate.LuaState) {
	lauxlib.LuaL_newlib(L, baselibs)
	lapi.Lua_pushglobaltable(L)
	lauxlib.LuaL_setfuncs(L, baselibs, 0)
	lapi.Lua_pushvalue(L, -1)
	lapi.Lua_setfield(L, lapi.LUA_REGISTRYINDEX, LUA_GNAME)
}
