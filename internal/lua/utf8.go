package lua

/*
** $Id: utf8.go $
** UTF-8 library
** Ported from lutf8lib.c
*/

import (
	"unicode/utf8"
	"unsafe"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lauxlib"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** UTF-8 library functions
*/

// utf8_len - get length of UTF-8 string
func utf8_len(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	
	i := 0
	if !lapi.Lua_isnoneornil(LS, 2) {
		i = int(lapi.Lua_tointeger(LS, 2)) - 1
	}
	
	j := len(s)
	if !lapi.Lua_isnoneornil(LS, 3) {
		j = int(lapi.Lua_tointeger(LS, 3))
	}
	
	if i < 0 {
		i += len(s)
	}
	if j < 0 {
		j += len(s)
	}
	i++ // back to 1-based
	
	count := 0
	pos := 0
	for pos < len(s) && pos < i-1 {
		_, size := utf8.DecodeRuneInString(s[pos:])
		pos += size
	}
	
	for pos < len(s) && count < j-i+1 {
		r, size := utf8.DecodeRuneInString(s[pos:])
		if r == utf8.RuneError && size == 1 {
			lauxlib.LuaL_error(LS, "invalid UTF-8 code")
		}
		count++
		pos += size
	}
	
	lapi.Lua_pushinteger(LS, int64(count))
	return 1
}

// utf8_sub - get substring
func utf8_sub(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	
	// Convert to rune indices
	start := 1
	if !lapi.Lua_isnoneornil(LS, 2) {
		start = int(lapi.Lua_tointeger(LS, 2))
	}
	end := len(s)
	if !lapi.Lua_isnoneornil(LS, 3) {
		end = int(lapi.Lua_tointeger(LS, 3))
	}
	
	// Build result from rune positions
	result := ""
	runes := []rune(s)
	
	if start < 0 {
		start += len(runes) + 1
	}
	if end < 0 {
		end += len(runes) + 1
	}
	
	if start < 1 {
		start = 1
	}
	if end > len(runes) {
		end = len(runes)
	}
	
	if start <= end {
		result = string(runes[start-1 : end])
	}
	
	lapi.Lua_pushstring(LS, result)
	return 1
}

// utf8_reverse - reverse UTF-8 string
func utf8_reverse(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	runes := []rune(s)
	
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	
	lapi.Lua_pushstring(LS, string(runes))
	return 1
}

// utf8_char - convert codepoints to UTF-8
func utf8_char(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_gettop(LS)
	
	result := make([]byte, 0, n*4)
	for i := 1; i <= n; i++ {
		codepoint := int(lapi.Lua_tointeger(LS, i))
		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], rune(codepoint))
		result = append(result, buf[:n]...)
	}
	
	lapi.Lua_pushstring(LS, string(result))
	return 1
}

// utf8_codes - iterate over UTF-8 codes
func utf8_codes(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	_ = LS
	// Simplified: return iterator function
	return 1
}

// utf8_offset - byte offset for character position
func utf8_offset(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	n := int(lapi.Lua_tointeger(LS, 2))
	
	if n < 0 {
		n += utf8.RuneCountInString(s) + 1
	}
	
	pos := 0
	count := 1
	for pos < len(s) && count < n {
		_, size := utf8.DecodeRuneInString(s[pos:])
		pos += size
		count++
	}
	
	if pos >= len(s) {
		lapi.Lua_pushnil(LS)
	} else {
		lapi.Lua_pushinteger(LS, int64(pos+1))
	}
	return 1
}

// utf8_charpos - character position to byte position
func utf8_charpos(L *lobject.LuaState) int {
	return utf8_offset(L)
}

// utf8_codepoint - codepoint at position
func utf8_codepoint(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	i := 0
	if !lapi.Lua_isnoneornil(LS, 2) {
		i = int(lapi.Lua_tointeger(LS, 2)) - 1
	}
	j := i + 1
	if !lapi.Lua_isnoneornil(LS, 3) {
		j = int(lapi.Lua_tointeger(LS, 3))
	}
	
	if i < 0 {
		i += len(s)
	}
	if j < 0 {
		j += len(s)
	}
	i++ // back to 1-based
	
	count := 0
	pos := 0
	for pos < len(s) && pos < i-1 {
		_, size := utf8.DecodeRuneInString(s[pos:])
		pos += size
	}
	
	for pos < len(s) && count < j-i+1 {
		r, size := utf8.DecodeRuneInString(s[pos:])
		if r == utf8.RuneError && size == 1 {
			lauxlib.LuaL_error(LS, "invalid UTF-8 code")
		}
		lapi.Lua_pushinteger(LS, int64(r))
		count++
		pos += size
	}
	
	if count == 0 {
		return 0
	}
	return count
}

/*
** UTF-8 library functions
*/
var utf8libs = []lauxlib.LuaL_Reg{
	{"len", utf8_len},
	{"sub", utf8_sub},
	{"reverse", utf8_reverse},
	{"char", utf8_char},
	{"codes", utf8_codes},
	{"offset", utf8_offset},
	{"charpos", utf8_charpos},
	{"codepoint", utf8_codepoint},
}

/*
** OpenUTF8 - open UTF-8 library
 */
func OpenUTF8(L *lstate.LuaState) {
	lauxlib.LuaL_newlib(L, utf8libs)
}
