// Package api provides Lua C API compatible interfaces.
// Main entry point - re-exports everything from api package.
package api

import (
	luaapi "github.com/akzj/go-lua/api/api"
)

type LuaAPI = luaapi.LuaAPI
type LuaLib = luaapi.LuaLib
type Status = luaapi.Status

// Re-export constants
const (
	LUA_TNIL           = luaapi.LUA_TNIL
	LUA_TBOOLEAN       = luaapi.LUA_TBOOLEAN
	LUA_TLIGHTUSERDATA = luaapi.LUA_TLIGHTUSERDATA
	LUA_TNUMBER        = luaapi.LUA_TNUMBER
	LUA_TSTRING        = luaapi.LUA_TSTRING
	LUA_TTABLE         = luaapi.LUA_TTABLE
	LUA_TFUNCTION      = luaapi.LUA_TFUNCTION
	LUA_TUSERDATA      = luaapi.LUA_TUSERDATA
	LUA_TTHREAD        = luaapi.LUA_TTHREAD
	LUA_TNONE          = luaapi.LUA_TNONE
	LUA_NUMTYPES       = luaapi.LUA_NUMTYPES
)

const (
	LUA_OK        = luaapi.LUA_OK
	LUA_YIELD     = luaapi.LUA_YIELD
	LUA_ERRRUN    = luaapi.LUA_ERRRUN
	LUA_ERRSYNTAX = luaapi.LUA_ERRSYNTAX
	LUA_ERRMEM    = luaapi.LUA_ERRMEM
	LUA_ERRERR    = luaapi.LUA_ERRERR
	LUA_MULTRET   = luaapi.LUA_MULTRET
)

const (
	LUA_OPADD  = luaapi.LUA_OPADD
	LUA_OPSUB  = luaapi.LUA_OPSUB
	LUA_OPMUL  = luaapi.LUA_OPMUL
	LUA_OPMOD  = luaapi.LUA_OPMOD
	LUA_OPPOW  = luaapi.LUA_OPPOW
	LUA_OPDIV  = luaapi.LUA_OPDIV
	LUA_OPIDIV = luaapi.LUA_OPIDIV
	LUA_OPBAND = luaapi.LUA_OPBAND
	LUA_OPBOR  = luaapi.LUA_OPBOR
	LUA_OPBXOR = luaapi.LUA_OPBXOR
	LUA_OPSHL  = luaapi.LUA_OPSHL
	LUA_OPSHR  = luaapi.LUA_OPSHR
	LUA_OPUNM  = luaapi.LUA_OPUNM
	LUA_OPBNOT = luaapi.LUA_OPBNOT
)

const (
	LUA_OPEQ = luaapi.LUA_OPEQ
	LUA_OPLT = luaapi.LUA_OPLT
	LUA_OPLE = luaapi.LUA_OPLE
)

const (
	LUA_GCSTOP     = luaapi.LUA_GCSTOP
	LUA_GCRESTART  = luaapi.LUA_GCRESTART
	LUA_GCCOLLECT  = luaapi.LUA_GCCOLLECT
	LUA_GCCOUNT    = luaapi.LUA_GCCOUNT
	LUA_GCCOUNTB   = luaapi.LUA_GCCOUNTB
	LUA_GCSTEP     = luaapi.LUA_GCSTEP
	LUA_GCISRUNNING = luaapi.LUA_GCISRUNNING
)

const (
	LUA_REGISTRYINDEX = luaapi.LUA_REGISTRYINDEX
	LUA_RIDX_GLOBALS  = luaapi.LUA_RIDX_GLOBALS
	LUA_RIDX_MAINTHREAD = luaapi.LUA_RIDX_MAINTHREAD
	LUA_RIDX_LAST     = luaapi.LUA_RIDX_LAST
	LUA_MINSTACK      = luaapi.LUA_MINSTACK
	LUA_NOREF         = -2
	LUA_REFNIL        = -1
)

// Re-export helper functions
var Typename = luaapi.Typename
var StatusString = luaapi.StatusString
var New = luaapi.New
var NewWithAllocator = luaapi.NewWithAllocator

// Re-export LuaL_Reg
type LuaL_Reg = luaapi.LuaL_Reg
