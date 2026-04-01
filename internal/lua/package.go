package lua

/*
** $Id: package.go $
** Package library
** Ported from lloadlib.c / lpacklib.c
*/

import (
	"unsafe"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lauxlib"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** Package library functions
*/

// modular_require - require function
func modular_require(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	name := lapi.Lua_tolstring(LS, 1, nil)
	
	// Check if already loaded
	lapi.Lua_pushglobaltable(LS)
	lapi.Lua_pushstring(LS, "loaded")
	lapi.Lua_gettable(LS, -2)
	lapi.Lua_pushstring(LS, name)
	lapi.Lua_gettable(LS, -2)
	
	if !lapi.Lua_isnil(LS, -1) {
		return 1
	}
	lapi.Lua_pop(LS, 1)
	
	// Try preload
	lapi.Lua_pushglobaltable(LS)
	lapi.Lua_pushstring(LS, "preload")
	lapi.Lua_gettable(LS, -2)
	lapi.Lua_pushstring(LS, name)
	lapi.Lua_gettable(LS, -2)
	
	if lapi.Lua_isnil(LS, -1) {
		lauxlib.LuaL_error(LS, "module '%s' not found", name)
	}
	
	// Load module
	lapi.Lua_pushstring(LS, name)
	lapi.Lua_call(LS, 1, 1)
	
	return 1
}

// modular_searchpath - search for file
func modular_searchpath(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	name := lapi.Lua_tolstring(LS, 1, nil)
	// Simplified: just return the name
	lapi.Lua_pushstring(LS, name)
	return 1
}

// modular_config - package config
func modular_config(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_pushstring(LS, "\n.\n;\n?\n!\n-\n")
	return 1
}

// modular_seeall - set metatable for modules
func modular_seeall(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lauxlib.LuaL_checktype(LS, 1, lobject.LUA_TTABLE)
	return 1
}

/*
** Package library functions
*/
var packagelibs = []lauxlib.LuaL_Reg{
	{"config", modular_config},
	{"searchpath", modular_searchpath},
	{"seeall", modular_seeall},
}

/*
** OpenPackage - open package library
 */
func OpenPackage(L *lstate.LuaState) {
	lauxlib.LuaL_newlib(L, packagelibs)
	
	// Create loaded table
	lapi.Lua_createtable(L, 0, 1)
	lapi.Lua_setfield(L, -2, "loaded")
	
	// Create preload table
	lapi.Lua_createtable(L, 0, 1)
	lapi.Lua_setfield(L, -2, "preload")
	
	// Create searchers table
	lapi.Lua_createtable(L, 2, 0)
	lapi.Lua_pushcfunction(L, modular_require, 0)
	lapi.Lua_seti(L, -2, 1)
	lapi.Lua_setfield(L, -2, "searchers")
	
	// Create path and cpath
	lapi.Lua_pushstring(L, "./?.lua")
	lapi.Lua_setfield(L, -2, "path")
	lapi.Lua_pushstring(L, "./?.so")
	lapi.Lua_setfield(L, -2, "cpath")
}

// OpenAll - open all standard libraries (convenience function)
func OpenAll(L *lstate.LuaState) {
	OpenBase(L)
	OpenMath(L)
	OpenString(L)
	OpenTable(L)
	OpenIO(L)
	OpenOS(L)
	OpenCoroutine(L)
	OpenDebug(L)
	OpenUTF8(L)
	OpenPackage(L)
}
