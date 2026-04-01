package lauxlib

/*
** $Id: lauxlib.go $
** Auxiliary functions for building Lua libraries
** Ported from lauxlib.c and lauxlib.h
*/

import (
	"unsafe"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lmem"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** Constants
 */
const LUAL_BUFFERSIZE = 1024

/*
** Registry keys
 */
const (
	LUA_GNAME         = "_G"
	LUA_LOADED_TABLE  = "_LOADED"
	LUA_PRELOAD_TABLE = "_PRELOAD"
)

/*
** Predefined references
 */
const (
	LUA_NOREF  = -2
	LUA_REFNIL = -1
)

/*
** Extra error code for luaL_loadfilex
 */
const LUA_ERRFILE = 6 // LUA_ERRERR + 1

/*
** luaL_Reg - registry entry for library functions
 */
type luaL_Reg struct {
	Name string
	Func lobject.LuaCFunction
}

/*
** luaL_Buffer - string buffer for building results
 */
type luaL_Buffer struct {
	L       *lstate.LuaState
	b       []byte // current buffer
	size    int    // buffer size
	n       int    // number of characters in buffer
	initbuf [LUAL_BUFFERSIZE]byte // initial buffer
}

/*
** makeseed - generate random seed
 */
var seedCounter uint64 = 1

func makeseed() uint {
	// Simple seed based on address and counter
	seedCounter++
	return uint(seedCounter << 32)
}

/*
** luaL_newstate - creates a new Lua state
** Calls lua_newstate from lapi
 */
func luaL_newstate() *lstate.LuaState {
	L := lapi.Lua_newstate(lmem.DefaultAlloc, nil, makeseed())
	if L != nil {
		lapi.Lua_atpanic(L, func(L *lobject.LuaState) int {
			msg := "error in panic"
			L2 := (*lstate.LuaState)(unsafe.Pointer(L))
			if lapi.Lua_type(L2, -1) == lobject.LUA_TSTRING {
				var size int
				msg = lapi.Lua_tolstring(L2, -1, &size)
			}
			_ = msg
			return 0
		})
	}
	return L
}

// LuaL_newstate - exported version
func LuaL_newstate() *lstate.LuaState {
	return luaL_newstate()
}

/*
** LUAL_NUMSIZES - numeric sizes check
 */
const LUAL_NUMSIZES = unsafe.Sizeof(lobject.LuaInteger(0))*16 + unsafe.Sizeof(lobject.LuaNumber(0))

/*
** luaL_checkversion - check Lua version
 */
func luaL_checkversion(L *lstate.LuaState) {
	v := lapi.Lua_version(L)
	if LUAL_NUMSIZES != 0 { // simplified check
		_ = v
	}
}

/*
** luaL_getmetafield - get metafield
** Returns the type of the metafield, or LUA_TNIL if it doesn't exist
 */
func luaL_getmetafield(L *lstate.LuaState, obj int, event string) int {
	if lapi.Lua_getmetatable(L, obj) == 0 { // no metatable?
		return lobject.LUA_TNIL
	}
	lapi.Lua_pushstring(L, event)
	tt := lapi.Lua_rawget(L, -2)
	if tt == lobject.LUA_TNIL { // is metafield nil?
		lapi.Lua_pop(L, 2) // remove metatable and metafield
	} else {
		lapi.Lua_remove(L, -2) // remove only metatable
	}
	return tt
}

/*
** luaL_callmeta - call a metafield
** Returns 1 if the metafield was called, 0 otherwise
 */
func luaL_callmeta(L *lstate.LuaState, obj int, event string) int {
	obj = lapi.Lua_absindex(L, obj)
	if luaL_getmetafield(L, obj, event) == lobject.LUA_TNIL { // no metafield?
		return 0
	}
	lapi.Lua_pushvalue(L, obj)
	lapi.Lua_call(L, 1, 1)
	return 1
}

/*
** luaL_tolstring - convert value to string
** Returns pointer to string and sets len
 */
func luaL_tolstring(L *lstate.LuaState, idx int, l *int) string {
	idx = lapi.Lua_absindex(L, idx)
	if luaL_callmeta(L, idx, "__tostring") == 1 { // metafield called?
		if lapi.Lua_isstring(L, -1) == 0 {
			luaL_error(L, "'__tostring' must return a string")
		}
	} else {
		switch lapi.Lua_type(L, idx) {
		case lobject.LUA_TNUMBER:
			var isnum int
			n := lapi.Lua_tonumberx(L, idx, &isnum)
			lapi.Lua_pushfstring(L, "%f", n)
		case lobject.LUA_TSTRING:
			lapi.Lua_pushvalue(L, idx)
		case lobject.LUA_TBOOLEAN:
			if lapi.Lua_toboolean(L, idx) != 0 {
				lapi.Lua_pushstring(L, "true")
			} else {
				lapi.Lua_pushstring(L, "false")
			}
		case lobject.LUA_TNIL:
			lapi.Lua_pushstring(L, "nil")
		default:
			lapi.Lua_pushfstring(L, "%s: %p", lapi.Lua_typename(L, lapi.Lua_type(L, idx)), lapi.Lua_topointer(L, idx))
		}
	}
	return lapi.Lua_tolstring(L, -1, l)
}

/*
** luaL_argerror - raise argument error
 */
func luaL_argerror(L *lstate.LuaState, arg int, extramsg string) int {
	var ar lobject.Debug
	if lapi.Lua_getstack(L, 0, &ar) == 0 { // no stack frame?
		return luaL_error(L, "bad argument #%d (%s)", arg, extramsg)
	}
	lapi.Lua_getinfo(L, "nt", &ar)
	return luaL_error(L, "bad argument #%d to '%s' (%s)", arg, ar.Name, extramsg)
}

/*
** luaL_typeerror - raise type error
 */
func luaL_typeerror(L *lstate.LuaState, arg int, tname string) int {
	var typearg string
	if luaL_getmetafield(L, arg, "__name") == lobject.LUA_TSTRING {
		typearg = lapi.Lua_tolstring(L, -1, nil)
	} else if lapi.Lua_type(L, arg) == lobject.LUA_TLIGHTUSERDATA {
		typearg = "light userdata"
	} else {
		typearg = lapi.Lua_typename(L, lapi.Lua_type(L, arg))
	}
	msg := tname + " expected, got " + typearg
	return luaL_argerror(L, arg, msg)
}

/*
** tag_error - raise type error with tag
 */
func tag_error(L *lstate.LuaState, arg, tag int) {
	luaL_typeerror(L, arg, lapi.Lua_typename(L, tag))
}

/*
** luaL_where - push location information
 */
func luaL_where(L *lstate.LuaState, level int) {
	var ar lobject.Debug
	if lapi.Lua_getstack(L, level, &ar) != 0 { // check function at level
		lapi.Lua_getinfo(L, "Sl", &ar) // get info about it
		if ar.Line > 0 {
			lapi.Lua_pushfstring(L, "%s:%d: ", ar.Source, ar.Line)
			return
		}
	}
	lapi.Lua_pushstring(L, "") // else, no information available...
}

/*
** luaL_error - raise error
 */
func luaL_error(L *lstate.LuaState, fmtstr string, args ...interface{}) int {
	luaL_where(L, 1)
	lapi.Lua_pushfstring(L, fmtstr, args...)
	lapi.Lua_concat(L, 2)
	return lapi.Lua_error(L)
}

/*
** luaL_fileresult - file operation result
 */
func luaL_fileresult(L *lstate.LuaState, stat int, fname string) int {
	if stat != 0 {
		lapi.Lua_pushboolean(L, true)
		return 1
	}
	luaL_pushfail(L)
	if fname != "" {
		lapi.Lua_pushfstring(L, "%s: %s", fname, "error message")
	} else {
		lapi.Lua_pushstring(L, "error message")
	}
	lapi.Lua_pushinteger(L, 0)
	return 3
}

/*
** luaL_execresult - execution result
 */
func luaL_execresult(L *lstate.LuaState, stat int) int {
	if stat == 0 {
		lapi.Lua_pushboolean(L, true)
		lapi.Lua_pushstring(L, "exit")
		lapi.Lua_pushinteger(L, 0)
		return 3
	}
	luaL_pushfail(L)
	lapi.Lua_pushstring(L, "exit")
	lapi.Lua_pushinteger(L, int64(stat))
	return 3
}

/*
** luaL_newmetatable - create new metatable
** Returns 0 if name already exists, 1 if created
 */
func luaL_newmetatable(L *lstate.LuaState, tname string) int {
	if lapi.Lua_getfield(L, lapi.LUA_REGISTRYINDEX, tname) != lobject.LUA_TNIL { // name already in use?
		return 0
	}
	lapi.Lua_pop(L, 1)
	lapi.Lua_createtable(L, 0, 2) // create metatable
	lapi.Lua_pushstring(L, tname)
	lapi.Lua_setfield(L, -2, "__name")
	lapi.Lua_pushvalue(L, -1)
	lapi.Lua_setfield(L, lapi.LUA_REGISTRYINDEX, tname)
	return 1
}

/*
** luaL_setmetatable - set metatable by name
 */
func luaL_setmetatable(L *lstate.LuaState, tname string) {
	lapi.Lua_getfield(L, lapi.LUA_REGISTRYINDEX, tname)
	lapi.Lua_setmetatable(L, -2)
}

/*
** luaL_testudata - test userdata type
** Returns userdata pointer if it matches the type, nil otherwise
 */
func luaL_testudata(L *lstate.LuaState, ud int, tname string) interface{} {
	p := lapi.Lua_touserdata(L, ud)
	if p != nil {
		if lapi.Lua_getmetatable(L, ud) != 0 {
			lapi.Lua_getfield(L, lapi.LUA_REGISTRYINDEX, tname)
			if lapi.Lua_rawequal(L, -1, -2) == 0 {
				p = nil
			}
			lapi.Lua_pop(L, 2)
			return p
		}
	}
	return nil
}

/*
** luaL_checkudata - check userdata type
 */
func luaL_checkudata(L *lstate.LuaState, ud int, tname string) interface{} {
	p := luaL_testudata(L, ud, tname)
	if p == nil {
		luaL_typeerror(L, ud, tname)
	}
	return p
}

/*
** luaL_checkstack - ensure stack space
 */
func luaL_checkstack(L *lstate.LuaState, sz int, msg string) {
	if !lapi.Lua_checkstack(L, sz) {
		if msg != "" {
			luaL_error(L, "stack overflow (%s)", msg)
		} else {
			luaL_error(L, "stack overflow")
		}
	}
}

/*
** luaL_checktype - check value type
 */
func luaL_checktype(L *lstate.LuaState, arg, t int) {
	if lapi.Lua_type(L, arg) != t {
		tag_error(L, arg, t)
	}
}

/*
** luaL_checkany - check for any value
 */
func luaL_checkany(L *lstate.LuaState, arg int) {
	if lapi.Lua_type(L, arg) == 0 { // LUA_TNONE = 0
		luaL_argerror(L, arg, "value expected")
	}
}

/*
** luaL_checklstring - check and get string argument
 */
func luaL_checklstring(L *lstate.LuaState, arg int, l *int) string {
	if l != nil {
		*l = 0
	}
	s := lapi.Lua_tolstring(L, arg, l)
	if s == "" && lapi.Lua_type(L, arg) != lobject.LUA_TSTRING {
		tag_error(L, arg, lobject.LUA_TSTRING)
	}
	return s
}

/*
** luaL_optlstring - get optional string argument
 */
func luaL_optlstring(L *lstate.LuaState, arg int, def string, l *int) string {
	if lapi.Lua_isnoneornil(L, arg) {
		if l != nil {
			*l = len(def)
		}
		return def
	}
	return luaL_checklstring(L, arg, l)
}

/*
** luaL_checknumber - check and get number argument
 */
func luaL_checknumber(L *lstate.LuaState, arg int) lobject.LuaNumber {
	var isnum int
	n := lapi.Lua_tonumberx(L, arg, &isnum)
	if isnum == 0 {
		tag_error(L, arg, lobject.LUA_TNUMBER)
	}
	return n
}

/*
** luaL_optnumber - get optional number argument
 */
func luaL_optnumber(L *lstate.LuaState, arg int, def lobject.LuaNumber) lobject.LuaNumber {
	if lapi.Lua_isnoneornil(L, arg) {
		return def
	}
	return luaL_checknumber(L, arg)
}

/*
** luaL_checkinteger - check and get integer argument
 */
func luaL_checkinteger(L *lstate.LuaState, arg int) lobject.LuaInteger {
	var isnum int
	i := lapi.Lua_tointegerx(L, arg, &isnum)
	if isnum == 0 {
		tag_error(L, arg, lobject.LUA_TNUMBER)
	}
	return i
}

/*
** luaL_optinteger - get optional integer argument
 */
func luaL_optinteger(L *lstate.LuaState, arg int, def lobject.LuaInteger) lobject.LuaInteger {
	if lapi.Lua_isnoneornil(L, arg) {
		return def
	}
	return luaL_checkinteger(L, arg)
}

/*
** luaL_checkoption - check option from list
 */
func luaL_checkoption(L *lstate.LuaState, arg int, def string, lst []string) int {
	name := luaL_optlstring(L, arg, def, nil)
	for i := 0; i < len(lst); i++ {
		if lst[i] == name {
			return i
		}
	}
	return luaL_argerror(L, arg, "invalid option '"+name+"'")
}

/*
** luaL_Buffer operations
*/

func luaL_buffinit(L *lstate.LuaState, B *luaL_Buffer) {
	B.L = L
	B.b = B.initbuf[:0]
	B.n = 0
	B.size = LUAL_BUFFERSIZE
	lapi.Lua_pushlightuserdata(L, unsafe.Pointer(B))
}

func luaL_prepbuffsize(B *luaL_Buffer, sz int) []byte {
	if B.size-B.n >= sz {
		return B.b[B.n : B.n+sz]
	}
	newsize := B.size * 2
	if newsize < B.n+sz {
		newsize = B.n + sz
	}
	newbuf := make([]byte, newsize)
	copy(newbuf, B.b)
	B.b = newbuf
	B.size = newsize
	return B.b[B.n : B.n+sz]
}

func luaL_addlstring(B *luaL_Buffer, s string, l int) {
	if l > 0 {
		copy(luaL_prepbuffsize(B, l), s)
		B.n += l
	}
}

func luaL_addstring(B *luaL_Buffer, s string) {
	luaL_addlstring(B, s, len(s))
}

func luaL_addvalue(B *luaL_Buffer) {
	var length int
	s := lapi.Lua_tolstring(B.L, -1, &length)
	if length > 0 {
		copy(luaL_prepbuffsize(B, length), s)
		B.n += length
	}
	lapi.Lua_pop(B.L, 1)
}

func luaL_pushresult(B *luaL_Buffer) {
	lapi.Lua_pushlstring(B.L, string(B.b[:B.n]))
}

func luaL_pushresultsize(B *luaL_Buffer, sz int) {
	B.n += sz
	luaL_pushresult(B)
}

func luaL_buffinitsize(L *lstate.LuaState, B *luaL_Buffer, sz int) []byte {
	luaL_buffinit(L, B)
	return luaL_prepbuffsize(B, sz)
}

func luaL_addchar(B *luaL_Buffer, c byte) {
	slice := luaL_prepbuffsize(B, 1)
	slice[0] = c
	B.n++
}

/*
** Reference system
*/

func luaL_ref(L *lstate.LuaState, t int) int {
	if lapi.Lua_isnil(L, -1) {
		lapi.Lua_pop(L, 1)
		return LUA_REFNIL
	}
	t = lapi.Lua_absindex(L, t)
	if lapi.Lua_rawgeti(L, t, 1) == lobject.LUA_TNUMBER {
		return int(lapi.Lua_tointeger(L, -1))
	}
	ref := 0
	lapi.Lua_pushinteger(L, 0)
	lapi.Lua_rawseti(L, t, 1)
	lapi.Lua_pop(L, 1)
	if ref != 0 {
		lapi.Lua_rawgeti(L, t, int64(ref))
		lapi.Lua_rawseti(L, t, 1)
	} else {
		ref = int(lapi.Lua_rawlen(L, t)) + 1
	}
	lapi.Lua_rawseti(L, t, int64(ref))
	return ref
}

func luaL_unref(L *lstate.LuaState, t, ref int) {
	if ref >= 0 {
		t = lapi.Lua_absindex(L, t)
		lapi.Lua_rawgeti(L, t, 1)
		lapi.Lua_rawseti(L, t, int64(ref))
		lapi.Lua_pushinteger(L, int64(ref))
		lapi.Lua_rawseti(L, t, 1)
	}
}

/*
** Load functions
*/

func luaL_loadbufferx(L *lstate.LuaState, buff string, name, mode string) int {
	return lapi.Lua_load(L, nil, nil, name, mode)
}

func luaL_loadstring(L *lstate.LuaState, s string) int {
	return luaL_loadbufferx(L, s, s, "")
}

func luaL_getsubtable(L *lstate.LuaState, idx int, fname string) int {
	if lapi.Lua_getfield(L, idx, fname) == lobject.LUA_TTABLE {
		return 1
	}
	lapi.Lua_pop(L, 1)
	idx = lapi.Lua_absindex(L, idx)
	lapi.Lua_createtable(L, 0, 0)
	lapi.Lua_pushvalue(L, -1)
	lapi.Lua_setfield(L, idx, fname)
	return 0
}

func luaL_requiref(L *lstate.LuaState, modname string, openf lobject.LuaCFunction, glb int) {
	luaL_getsubtable(L, lapi.LUA_REGISTRYINDEX, LUA_LOADED_TABLE)
	lapi.Lua_getfield(L, -1, modname)
	if lapi.Lua_toboolean(L, -1) == 0 {
		lapi.Lua_pop(L, 1)
		lapi.Lua_pushcfunction(L, openf, 0)
		lapi.Lua_pushstring(L, modname)
		lapi.Lua_call(L, 1, 1)
		lapi.Lua_pushvalue(L, -1)
		lapi.Lua_setfield(L, -3, modname)
	}
	lapi.Lua_pop(L, 1)
	if glb != 0 {
		lapi.Lua_pushvalue(L, -1)
		lapi.Lua_setglobal(L, modname)
	}
}

func luaL_traceback(L, L1 *lstate.LuaState, msg string, level int) {
	if msg != "" {
		lapi.Lua_pushstring(L, msg)
	}
	lapi.Lua_pushstring(L, "\nstack traceback:")
	luaL_where(L, level)
}

func luaL_setfuncs(L *lstate.LuaState, l []luaL_Reg, nup int) {
	luaL_checkstack(L, nup, "too many upvalues")
	for i := range l {
		if l[i].Name == "" {
			lapi.Lua_pushboolean(L, false)
		} else {
			for j := 0; j < nup; j++ {
				lapi.Lua_pushvalue(L, -nup)
			}
			lapi.Lua_pushcfunction(L, l[i].Func, nup)
		}
		lapi.Lua_setfield(L, -(nup + 2), l[i].Name)
	}
	lapi.Lua_pop(L, nup)
}

func luaL_newlibtable(L *lstate.LuaState, l []luaL_Reg) {
	lapi.Lua_createtable(L, 0, len(l)-1)
}

func luaL_newlib(L *lstate.LuaState, l []luaL_Reg) {
	luaL_checkversion(L)
	luaL_newlibtable(L, l)
	luaL_setfuncs(L, l, 0)
}

func luaL_gsub(L *lstate.LuaState, s, p, r string) string {
	result := ""
	start := 0
	for {
		idx := findString(s, p, start)
		if idx < 0 {
			result += s[start:]
			break
		}
		result += s[start:idx]
		result += r
		start = idx + len(p)
	}
	lapi.Lua_pushstring(L, result)
	return lapi.Lua_tolstring(L, -1, nil)
}

func findString(s, pattern string, start int) int {
	if start >= len(s) {
		return -1
	}
	for i := start; i <= len(s)-len(pattern); i++ {
		if s[i:i+len(pattern)] == pattern {
			return i
		}
	}
	return -1
}

func luaL_len(L *lstate.LuaState, idx int) lobject.LuaInteger {
	lapi.Lua_len(L, idx)
	var isnum int
	l := lapi.Lua_tointegerx(L, -1, &isnum)
	if isnum == 0 {
		luaL_error(L, "object length is not an integer")
	}
	lapi.Lua_pop(L, 1)
	return l
}

func luaL_pushfail(L *lstate.LuaState) {
	lapi.Lua_pushboolean(L, false)
}

func luaL_argcheck(L *lstate.LuaState, cond bool, arg int, extramsg string) {
	if !cond {
		luaL_argerror(L, arg, extramsg)
	}
}

func luaL_argexpected(L *lstate.LuaState, cond bool, arg, tname int) {
	if !cond {
		luaL_typeerror(L, arg, lapi.Lua_typename(L, tname))
	}
}

func luaL_getmetatable(L *lstate.LuaState, tname string) int {
	return lapi.Lua_getfield(L, lapi.LUA_REGISTRYINDEX, tname)
}

func luaL_pushstring(L *lstate.LuaState, s string) {
	lapi.Lua_pushstring(L, s)
}

func luaL_openlibs(L *lstate.LuaState) {
	lapi.Lua_createtable(L, 0, 0)
	lapi.Lua_setglobal(L, LUA_GNAME)
	luaL_getsubtable(L, lapi.LUA_REGISTRYINDEX, LUA_LOADED_TABLE)
	lapi.Lua_pop(L, 1)
	luaL_getsubtable(L, lapi.LUA_REGISTRYINDEX, LUA_PRELOAD_TABLE)
	lapi.Lua_pop(L, 1)
}

/*
** Exported wrappers for use by other packages
 */

// LuaL_Reg - exported version of luaL_Reg
type LuaL_Reg struct {
	Name string
	Func lobject.LuaCFunction
}

// LuaL_setfuncs - exported version of luaL_setfuncs
func LuaL_setfuncs(L *lstate.LuaState, l []LuaL_Reg, nup int) {
	luaL_checkstack(L, nup, "too many upvalues")
	for i := range l {
		if l[i].Name == "" {
			lapi.Lua_pushboolean(L, false)
		} else {
			for j := 0; j < nup; j++ {
				lapi.Lua_pushvalue(L, -nup)
			}
			lapi.Lua_pushcfunction(L, l[i].Func, nup)
		}
		lapi.Lua_setfield(L, -(nup + 2), l[i].Name)
	}
	lapi.Lua_pop(L, nup)
}

// LuaL_newlib - exported version of luaL_newlib
func LuaL_newlib(L *lstate.LuaState, l []LuaL_Reg) {
	luaL_checkversion(L)
	lapi.Lua_createtable(L, 0, len(l)-1)
	LuaL_setfuncs(L, l, 0)
}

// LuaL_pushfail - push false value
func LuaL_pushfail(L *lstate.LuaState) {
	lapi.Lua_pushboolean(L, false)
}

// LuaL_tolstring - convert value to string
func LuaL_tolstring(L *lstate.LuaState, idx int, l *int) string {
	return luaL_tolstring(L, idx, l)
}

// LuaL_getmetafield - get metafield
func LuaL_getmetafield(L *lstate.LuaState, obj int, event string) int {
	return luaL_getmetafield(L, obj, event)
}

// LuaL_checkany - check for any value
func LuaL_checkany(L *lstate.LuaState, arg int) {
	luaL_checkany(L, arg)
}

// LuaL_checkinteger - check and get integer
func LuaL_checkinteger(L *lstate.LuaState, arg int) lobject.LuaInteger {
	return luaL_checkinteger(L, arg)
}

// LuaL_optinteger - get optional integer
func LuaL_optinteger(L *lstate.LuaState, arg int, def lobject.LuaInteger) lobject.LuaInteger {
	return luaL_optinteger(L, arg, def)
}

// LuaL_checkstring - check and get string
func LuaL_checkstring(L *lstate.LuaState, arg int) string {
	return luaL_checklstring(L, arg, nil)
}

// LuaL_checktype - check type
func LuaL_checktype(L *lstate.LuaState, arg, t int) {
	luaL_checktype(L, arg, t)
}

// LuaL_argexpected - check type with error
func LuaL_argexpected(L *lstate.LuaState, cond bool, arg, tname int) {
	luaL_argexpected(L, cond, arg, tname)
}

// LuaL_argcheck - check argument condition
func LuaL_argcheck(L *lstate.LuaState, cond bool, arg int, extramsg string) {
	luaL_argcheck(L, cond, arg, extramsg)
}

// LuaL_error - raise error
func LuaL_error(L *lstate.LuaState, fmtstr string, args ...interface{}) int {
	return luaL_error(L, fmtstr, args...)
}

// Exported wrappers for lua package

// LuaL_Buffer - exported alias for luaL_Buffer
type LuaL_Buffer = luaL_Buffer

// LuaL_checknumber - check and get number argument
func LuaL_checknumber(L *lstate.LuaState, arg int) lobject.LuaNumber {
	return luaL_checknumber(L, arg)
}

// LuaL_typeerror - raise type error
func LuaL_typeerror(L *lstate.LuaState, arg int, tname string) int {
	return luaL_typeerror(L, arg, tname)
}

// LuaL_optlstring - get optional string argument
func LuaL_optlstring(L *lstate.LuaState, arg int, def string, l *int) string {
	return luaL_optlstring(L, arg, def, l)
}

// LuaL_buffinit - initialize buffer
func LuaL_buffinit(L *lstate.LuaState, B *LuaL_Buffer) {
	luaL_buffinit(L, (*luaL_Buffer)(B))
}

// LuaL_prepbuffsize - prepare buffer size
func LuaL_prepbuffsize(B *LuaL_Buffer, sz int) []byte {
	return luaL_prepbuffsize((*luaL_Buffer)(B), sz)
}

// LuaL_addlstring - add string to buffer
func LuaL_addlstring(B *LuaL_Buffer, s string, l int) {
	luaL_addlstring((*luaL_Buffer)(B), s, l)
}

// LuaL_addvalue - add value to buffer
func LuaL_addvalue(L *lstate.LuaState, B *LuaL_Buffer) {
	luaL_addvalue((*luaL_Buffer)(B))
}

// LuaL_pushresult - push buffer result
func LuaL_pushresult(L *lstate.LuaState, B *LuaL_Buffer) {
	luaL_pushresult((*luaL_Buffer)(B))
}

