package lua

/*
** $Id: io.go $
** I/O library
** Ported from liolib.c
*/

import (
	"os"
	"unsafe"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lauxlib"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** I/O library state
*/
var (
	currentInput  = os.Stdin
	currentOutput = os.Stdout
	currentError  = os.Stderr
)

// io_open - open file
func io_open(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	filename := lauxlib.LuaL_checkstring(LS, 1)
	mode := "r"
	if lapi.Lua_gettop(LS) >= 2 {
		mode = lapi.Lua_tolstring(LS, 2, nil)
	}
	
	file, err := os.OpenFile(filename, modeToFlag(mode), 0644)
	if err != nil {
		lapi.Lua_pushnil(LS)
		lapi.Lua_pushstring(LS, err.Error())
		return 2
	}
	
	lapi.Lua_pushlightuserdata(LS, file)
	return 1
}

// modeToFlag - convert mode string to os flags
func modeToFlag(mode string) int {
	switch mode {
	case "r":
		return int(os.O_RDONLY)
	case "w":
		return int(os.O_WRONLY | os.O_CREATE | os.O_TRUNC)
	case "a":
		return int(os.O_WRONLY | os.O_CREATE | os.O_APPEND)
	case "r+":
		return int(os.O_RDWR)
	case "w+":
		return int(os.O_RDWR | os.O_CREATE | os.O_TRUNC)
	case "a+":
		return int(os.O_RDWR | os.O_CREATE | os.O_APPEND)
	default:
		return int(os.O_RDONLY)
	}
}

// io_close - close file
func io_close(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	file := lapi.Lua_touserdata(LS, 1)
	if f, ok := file.(*os.File); ok {
		f.Close()
	}
	return 0
}

// io_read - read from input
func io_read(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	data := make([]byte, 1024)
	n, err := currentInput.Read(data)
	if err != nil && n == 0 {
		return 0
	}
	lapi.Lua_pushstring(LS, string(data[:n]))
	return 1
}

// io_write - write to output
func io_write(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_gettop(LS)
	for i := 1; i <= n; i++ {
		s := lapi.Lua_tolstring(LS, i, nil)
		currentOutput.WriteString(s)
	}
	return 0
}

// io_flush - flush output
func io_flush(L *lobject.LuaState) int {
	return 0
}

// io_input - get/set input
func io_input(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	if lapi.Lua_isnoneornil(LS, 1) {
		lapi.Lua_pushlightuserdata(LS, currentInput)
		return 1
	}
	if lapi.Lua_isstring(LS, 1) != 0 {
		filename := lapi.Lua_tolstring(LS, 1, nil)
		f, err := os.Open(filename)
		if err != nil {
			lauxlib.LuaL_error(LS, "cannot open file: %s", err.Error())
		}
		currentInput = f
	}
	return 0
}

// io_output - get/set output
func io_output(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	if lapi.Lua_isnoneornil(LS, 1) {
		lapi.Lua_pushlightuserdata(LS, currentOutput)
		return 1
	}
	return 0
}

// io_type - type of file handle
func io_type(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	file := lapi.Lua_touserdata(LS, 1)
	if file == nil {
		lapi.Lua_pushnil(LS)
	} else if _, ok := file.(*os.File); ok {
		lapi.Lua_pushstring(LS, "file")
	} else {
		lapi.Lua_pushnil(LS)
	}
	return 1
}

// io_lines - iterator for file lines
func io_lines(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	filename := ""
	if !lapi.Lua_isnoneornil(LS, 1) {
		filename = lapi.Lua_tolstring(LS, 1, nil)
	}
	
	if filename != "" {
		file, err := os.Open(filename)
		if err != nil {
			lauxlib.LuaL_error(LS, "cannot open file: %s", err.Error())
		}
		lapi.Lua_pushcfunction(LS, io_lines_aux, 0)
		lapi.Lua_pushlightuserdata(LS, file)
		return 2
	}
	
	lapi.Lua_pushcfunction(LS, io_read, 0)
	return 1
}

// io_lines_aux - auxiliary for lines
func io_lines_aux(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	file := lapi.Lua_touserdata(LS, lapi.LUA_REGISTRYINDEX-1)
	if f, ok := file.(*os.File); ok {
		data := make([]byte, 1024)
		n, err := f.Read(data)
		if n > 0 {
			lapi.Lua_pushstring(LS, string(data[:n]))
			return 1
		}
		if err != nil {
			return 0
		}
	}
	return 0
}

// io_popen - open process (stub)
func io_popen(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lauxlib.LuaL_error(LS, "popen not supported")
	return 0
}

// io_tmpfile - open temporary file (stub)
func io_tmpfile(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	file, err := os.CreateTemp("", "lua")
	if err != nil {
		lauxlib.LuaL_error(LS, "cannot create temp file: %s", err.Error())
	}
	lapi.Lua_pushlightuserdata(LS, file)
	return 1
}

/*
** I/O library functions
*/
var iolibs = []lauxlib.LuaL_Reg{
	{"open", io_open},
	{"close", io_close},
	{"read", io_read},
	{"write", io_write},
	{"flush", io_flush},
	{"input", io_input},
	{"output", io_output},
	{"type", io_type},
	{"lines", io_lines},
	{"popen", io_popen},
	{"tmpfile", io_tmpfile},
}

/*
** OpenIO - open I/O library
 */
func OpenIO(L *lstate.LuaState) {
	lauxlib.LuaL_newlib(L, iolibs)
	
	lapi.Lua_pushlightuserdata(L, os.Stdin)
	lapi.Lua_setfield(L, -2, "stdin")
	lapi.Lua_pushlightuserdata(L, os.Stdout)
	lapi.Lua_setfield(L, -2, "stdout")
	lapi.Lua_pushlightuserdata(L, os.Stderr)
	lapi.Lua_setfield(L, -2, "stderr")
}
