package lua

/*
** $Id: table.go $
** Table library
** Ported from ltable.c
*/

import (
	"unsafe"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lauxlib"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** Table library functions
*/

// table_insert - insert element
func table_insert(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lauxlib.LuaL_checktype(LS, 1, lobject.LUA_TTABLE)
	
	pos := int(lapi.Lua_gettop(LS)) // default: at end
	if lapi.Lua_type(LS, 2) != lobject.LUA_TNUMBER {
		pos = int(lapi.Lua_gettop(LS)) + 1
	} else if int(lapi.Lua_gettop(LS)) == 2 {
		pos = int(lapi.Lua_gettop(LS)) + 1 // default: at end
	} else {
		pos = int(lapi.Lua_tointeger(LS, 2))
		if pos < 0 {
			pos = int(lapi.Lua_gettop(LS)) + pos + 2
		}
		if pos < 1 {
			pos = 1
		}
	}
	
	// Shift elements up
	for i := int(lapi.Lua_gettop(LS)) - 1; i >= pos-1; i-- {
		lapi.Lua_pushvalue(LS, i)
		lapi.Lua_pushvalue(LS, i+1)
		lapi.Lua_settable(LS, pos)
	}
	
	// Set new value
	lapi.Lua_pushvalue(LS, -1)
	lapi.Lua_settable(LS, pos)
	
	return 0
}

// table_remove - remove element
func table_remove(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lauxlib.LuaL_checktype(LS, 1, lobject.LUA_TTABLE)
	
	size := int(lapi.Lua_rawlen(LS, 1))
	pos := int(lauxlib.LuaL_optinteger(LS, 2, int64(size)))
	
	if pos == size+1 {
		pos = size
	}
	if pos < 0 {
		pos = size + pos + 1
	}
	if pos < 1 || pos > size {
		lapi.Lua_pushnil(LS)
		return 1
	}
	
	lapi.Lua_pushinteger(LS, int64(pos))
	lapi.Lua_gettable(LS, 1)
	
	// Shift elements down
	for i := pos; i < size; i++ {
		lapi.Lua_pushinteger(LS, int64(i))
		lapi.Lua_pushinteger(LS, int64(i+1))
		lapi.Lua_gettable(LS, 1)
		lapi.Lua_pushinteger(LS, int64(i))
		lapi.Lua_insert(LS, -2)
		lapi.Lua_settable(LS, 1)
	}
	
	// Remove last element
	lapi.Lua_pushnil(LS)
	lapi.Lua_pushinteger(LS, int64(size))
	lapi.Lua_settable(LS, 1)
	
	return 1
}

// table_concat - concatenate table elements
func table_concat(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lauxlib.LuaL_checktype(LS, 1, lobject.LUA_TTABLE)
	
	sep := ""
	if lapi.Lua_type(LS, 2) == lobject.LUA_TSTRING {
		sep = lapi.Lua_tolstring(LS, 2, nil)
	}
	i := int(lauxlib.LuaL_optinteger(LS, 3, 1))
	
	var j int64
	if lapi.Lua_isnoneornil(LS, 4) {
		j = int64(lapi.Lua_rawlen(LS, 1))
	} else {
		j = lapi.Lua_tointeger(LS, 4)
	}
	
	if i < 1 {
		i = 1
	}
	if int64(i) > j {
		lapi.Lua_pushstring(LS, "")
		return 1
	}
	
	result := ""
	for k := int64(i); k <= j; k++ {
		lapi.Lua_pushinteger(LS, k)
		lapi.Lua_gettable(LS, 1)
		if lapi.Lua_isstring(LS, -1) == 0 {
			lauxlib.LuaL_error(LS, "invalid value at index %d", k)
		}
		if k > int64(i) && sep != "" {
			result += sep
		}
		result += lapi.Lua_tolstring(LS, -1, nil)
		lapi.Lua_pop(LS, 1)
	}
	
	lapi.Lua_pushstring(LS, result)
	return 1
}

// table_sort - sort table (stub)
func table_sort(L *lobject.LuaState) int {
	return 0
}

// table_getn - get table length
func table_getn(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lauxlib.LuaL_checktype(LS, 1, lobject.LUA_TTABLE)
	n := int64(lapi.Lua_rawlen(LS, 1))
	lapi.Lua_pushinteger(LS, n)
	return 1
}

// table_maxn - get max numeric key (stub)
func table_maxn(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lauxlib.LuaL_checktype(LS, 1, lobject.LUA_TTABLE)
	lapi.Lua_pushinteger(LS, int64(lapi.Lua_rawlen(LS, 1)))
	return 1
}

// table_pack - pack arguments into table
func table_pack(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_gettop(LS)
	lapi.Lua_createtable(LS, n, 1)
	lapi.Lua_pushinteger(LS, int64(n))
	lapi.Lua_setfield(LS, -2, "n")
	
	for i := 1; i <= n; i++ {
		lapi.Lua_pushvalue(LS, i)
		lapi.Lua_seti(LS, -2, int64(i))
	}
	return 1
}

// table_unpack - unpack table into arguments
func table_unpack(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lauxlib.LuaL_checktype(LS, 1, lobject.LUA_TTABLE)
	
	i := int(lauxlib.LuaL_optinteger(LS, 2, 1))
	n := int64(lapi.Lua_rawlen(LS, 1))
	if !lapi.Lua_isnoneornil(LS, 4) {
		n = lapi.Lua_tointeger(LS, 4)
	}
	
	if int64(i) > n {
		return 0
	}
	
	for ; int64(i) <= n; i++ {
		lapi.Lua_pushinteger(LS, int64(i))
		lapi.Lua_gettable(LS, 1)
	}
	return int(n - int64(i) + 1)
}

/*
** Table library functions
*/
var tablelibs = []lauxlib.LuaL_Reg{
	{"insert", table_insert},
	{"remove", table_remove},
	{"concat", table_concat},
	{"sort", table_sort},
	{"getn", table_getn},
	{"maxn", table_maxn},
	{"pack", table_pack},
	{"unpack", table_unpack},
}

/*
** OpenTable - open table library
 */
func OpenTable(L *lstate.LuaState) {
	lauxlib.LuaL_newlib(L, tablelibs)
}
