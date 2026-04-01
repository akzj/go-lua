package lua

/*
** $Id: os.go $
** OS library
** Ported from loslib.c
*/

import (
	"os"
	"strconv"
	"time"
	"unsafe"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lauxlib"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** OS library functions
*/

// os_clock - get CPU time
func os_clock(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	// Return seconds as float
	start := time.Now()
	lapi.Lua_pushnumber(LS, float64(start.UnixNano())/1e9)
	return 1
}

// os_date - get date/time
func os_date(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	format := "%c"
	if lapi.Lua_gettop(LS) >= 1 && !lapi.Lua_isnoneornil(LS, 1) {
		format = lapi.Lua_tolstring(LS, 1, nil)
	}
	t := time.Now()
	if !lapi.Lua_isnoneornil(LS, 2) {
		t = time.Unix(lapi.Lua_tointeger(LS, 2), 0)
	}
	
	result := formatDate(format, t)
	lapi.Lua_pushstring(LS, result)
	return 1
}

// formatDate - format time (simplified)
func formatDate(format string, t time.Time) string {
	result := format
	result = replaceAll(result, "%Y", strconv.FormatInt(int64(t.Year()), 10))
	result = replaceAll(result, "%m", padInt(int(t.Month()), 2))
	result = replaceAll(result, "%d", padInt(t.Day(), 2))
	result = replaceAll(result, "%H", padInt(t.Hour(), 2))
	result = replaceAll(result, "%M", padInt(t.Minute(), 2))
	result = replaceAll(result, "%S", padInt(t.Second(), 2))
	result = replaceAll(result, "%a", t.Weekday().String()[:3])
	result = replaceAll(result, "%A", t.Weekday().String())
	result = replaceAll(result, "%b", t.Month().String()[:3])
	result = replaceAll(result, "%B", t.Month().String())
	result = replaceAll(result, "%c", t.Format("Mon Jan 02 15:04:05 2006"))
	result = replaceAll(result, "%x", t.Format("01/02/2006"))
	result = replaceAll(result, "%X", t.Format("15:04:05"))
	return result
}

func padInt(n int, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func replaceAll(s, old, new string) string {
	result := ""
	for {
		i := indexOf(s, old)
		if i < 0 {
			result += s
			break
		}
		result += s[:i] + new
		s = s[i+len(old):]
	}
	return result
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// os_time - get current time
func os_time(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	if lapi.Lua_isnoneornil(LS, 1) {
		lapi.Lua_pushinteger(LS, time.Now().Unix())
	} else {
		lauxlib.LuaL_checktype(LS, 1, lobject.LUA_TTABLE)
		year := int(lapi.Lua_getfield(LS, 1, "year"))
		month := int(lapi.Lua_getfield(LS, 1, "month"))
		day := int(lapi.Lua_getfield(LS, 1, "day"))
		hour := 0
		if lapi.Lua_getfield(LS, 1, "hour") != lobject.LUA_TNIL {
			hour = int(lapi.Lua_tointeger(LS, -1))
		}
		min := 0
		if lapi.Lua_getfield(LS, 1, "min") != lobject.LUA_TNIL {
			min = int(lapi.Lua_tointeger(LS, -1))
		}
		sec := 0
		if lapi.Lua_getfield(LS, 1, "sec") != lobject.LUA_TNIL {
			sec = int(lapi.Lua_tointeger(LS, -1))
		}
		t := time.Date(year, time.Month(month), day, hour, min, sec, 0, time.Local)
		lapi.Lua_pushinteger(LS, t.Unix())
	}
	return 1
}

// os_difftime - time difference
func os_difftime(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	t2 := lapi.Lua_tointeger(LS, 1)
	t1 := lapi.Lua_tointeger(LS, 2)
	lapi.Lua_pushnumber(LS, float64(t2-t1))
	return 1
}

// os_getenv - get environment variable
func os_getenv(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	name := lauxlib.LuaL_checkstring(LS, 1)
	value := os.Getenv(name)
	if value != "" {
		lapi.Lua_pushstring(LS, value)
	} else {
		lapi.Lua_pushnil(LS)
	}
	return 1
}

// os_setenv - set environment variable
func os_setenv(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	name := lauxlib.LuaL_checkstring(LS, 1)
	value := lauxlib.LuaL_checkstring(LS, 2)
	os.Setenv(name, value)
	return 0
}

// os_exit - exit program
func os_exit(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	code := 0
	if !lapi.Lua_isnoneornil(LS, 1) {
		code = int(lapi.Lua_tointeger(LS, 1))
	}
	os.Exit(code)
	return 0
}

// os_remove - delete file
func os_remove(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	filename := lauxlib.LuaL_checkstring(LS, 1)
	err := os.Remove(filename)
	if err != nil {
		lauxlib.LuaL_error(LS, "cannot remove file: %s", err.Error())
	}
	return 0
}

// os_rename - rename file
func os_rename(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	oldname := lauxlib.LuaL_checkstring(LS, 1)
	newname := lauxlib.LuaL_checkstring(LS, 2)
	err := os.Rename(oldname, newname)
	if err != nil {
		lauxlib.LuaL_error(LS, "cannot rename file: %s", err.Error())
	}
	return 0
}

// os_tmpname - temporary file name
func os_tmpname(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_pushstring(LS, os.TempDir()+"/lua_temp")
	return 1
}

// os_execute - execute shell command (stub)
func os_execute(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	if !lapi.Lua_isnoneornil(LS, 1) {
		lauxlib.LuaL_error(LS, "os.execute not fully supported")
	}
	lapi.Lua_pushinteger(LS, 0)
	return 1
}

// os_getcwd - get current directory
func os_getcwd(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	dir, err := os.Getwd()
	if err != nil {
		lauxlib.LuaL_error(LS, "cannot get current directory: %s", err.Error())
	}
	lapi.Lua_pushstring(LS, dir)
	return 1
}

// os_chdir - change directory
func os_chdir(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	dir := lauxlib.LuaL_checkstring(LS, 1)
	err := os.Chdir(dir)
	if err != nil {
		lauxlib.LuaL_error(LS, "cannot change directory: %s", err.Error())
	}
	return 0
}

// os_popen - pipe (stub)
func os_popen(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lauxlib.LuaL_error(LS, "os.popen not supported")
	return 0
}

// os_rename2 - rename (alias)
func os_rename2(L *lobject.LuaState) int {
	return os_rename(L)
}

// os_tmpname - temporary name
func os_tmpname2(L *lobject.LuaState) int {
	return os_tmpname(L)
}

/*
** OS library functions
*/
var oslibs = []lauxlib.LuaL_Reg{
	{"clock", os_clock},
	{"date", os_date},
	{"difftime", os_difftime},
	{"getenv", os_getenv},
	{"setenv", os_setenv},
	{"exit", os_exit},
	{"remove", os_remove},
	{"rename", os_rename},
	{"tmpname", os_tmpname},
	{"execute", os_execute},
	{"getcwd", os_getcwd},
	{"chdir", os_chdir},
	{"popen", os_popen},
	{"time", os_time},
}

/*
** OpenOS - open OS library
 */
func OpenOS(L *lstate.LuaState) {
	lauxlib.LuaL_newlib(L, oslibs)
}
