package lua

/*
** $Id: string.go $
** String library
** Ported from lstrlib.c
*/

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lauxlib"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** Maximum number of captures that a pattern can do
*/
const LUA_MAXCAPTURES = 32

/*
** Helper: convert position to absolute index
 */
func posrelat(pos int64, l int64) int64 {
	if pos > 0 {
		return pos
	} else if pos == 0 {
		return 1
	} else if pos < -l {
		return 1
	} else {
		return l + pos + 1
	}
}

/*
** String library functions
*/

// str_len - string length
func str_len(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	lapi.Lua_pushinteger(LS, int64(len(s)))
	return 1
}

// str_sub - substring
func str_sub(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	l := int64(len(s))
	start := lauxlib.LuaL_optinteger(LS, 2, 1)
	end := lauxlib.LuaL_optinteger(LS, 3, -1)
	
	start = posrelat(start, l)
	end = posrelat(end, l)
	
	if start < 1 {
		start = 1
	}
	if end > l {
		end = l
	}
	
	if start <= end {
		lapi.Lua_pushstring(LS, s[start-1:end])
	} else {
		lapi.Lua_pushstring(LS, "")
	}
	return 1
}

// str_reverse - reverse string
func str_reverse(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	lapi.Lua_pushstring(LS, string(runes))
	return 1
}

// str_lower - convert to lowercase
func str_lower(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	lapi.Lua_pushstring(LS, strings.ToLower(s))
	return 1
}

// str_upper - convert to uppercase
func str_upper(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	lapi.Lua_pushstring(LS, strings.ToUpper(s))
	return 1
}

// str_rep - repeat string
func str_rep(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	n := lapi.Lua_tointeger(LS, 2)
	sep := ""
	if lapi.Lua_gettop(LS) >= 3 {
		sep = lapi.Lua_tolstring(LS, 3, nil)
	}
	
	if n <= 0 {
		lapi.Lua_pushstring(LS, "")
		return 1
	}
	if len(s) == 0 && len(sep) == 0 {
		lapi.Lua_pushstring(LS, "")
		return 1
	}
	
	result := ""
	for i := int64(0); i < n; i++ {
		if i > 0 && len(sep) > 0 {
			result += sep
		}
		result += s
	}
	lapi.Lua_pushstring(LS, result)
	return 1
}

// str_byte - character to code
func str_byte(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	l := int64(len(s))
	i := lauxlib.LuaL_optinteger(LS, 2, 1)
	j := lauxlib.LuaL_optinteger(LS, 3, i)
	
	i = posrelat(i, l)
	j = posrelat(j, l)
	
	if i < 1 {
		i = 1
	}
	if j > l {
		j = l
	}
	
	if i > j {
		return 0
	}
	
	for k := i; k <= j; k++ {
		lapi.Lua_pushinteger(LS, int64(s[k-1]))
	}
	return int(j - i + 1)
}

// str_char - code to character
func str_char(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_gettop(LS)
	result := make([]byte, n)
	for i := 1; i <= n; i++ {
		c := lapi.Lua_tointeger(LS, i)
		if c < 0 || c > 255 {
			lauxlib.LuaL_error(LS, "invalid value")
		}
		result[i-1] = byte(c)
	}
	lapi.Lua_pushstring(LS, string(result))
	return 1
}

// str_dump - dump string representation
func str_dump(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	// Simplified: just return the string
	lapi.Lua_pushstring(LS, s)
	return 1
}

// str_format - format string
func str_format(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	format := lapi.Lua_tolstring(LS, 1, nil)
	args := make([]interface{}, 0)
	n := lapi.Lua_gettop(LS)
	for i := 2; i <= n; i++ {
		t := lapi.Lua_type(LS, i)
		switch t {
		case lobject.LUA_TNIL:
			args = append(args, "nil")
		case lobject.LUA_TNUMBER:
			if lapi.Lua_isinteger(LS, i) != 0 {
				args = append(args, lapi.Lua_tointeger(LS, i))
			} else {
				args = append(args, lapi.Lua_tonumberx(LS, i, nil))
			}
		case lobject.LUA_TSTRING:
			args = append(args, lapi.Lua_tolstring(LS, i, nil))
		case lobject.LUA_TBOOLEAN:
			if lapi.Lua_toboolean(LS, i) != 0 {
				args = append(args, "true")
			} else {
				args = append(args, "false")
			}
		default:
			args = append(args, lapi.Lua_typename(LS, t))
		}
	}
	
	// Simple format handling - just use fmt.Sprintf
	result := fmt.Sprintf(format, args...)
	lapi.Lua_pushstring(LS, result)
	return 1
}

// str_find - find pattern
func str_find(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	pattern := lapi.Lua_tolstring(LS, 2, nil)
	
	init := int64(1)
	if lapi.Lua_gettop(LS) >= 3 {
		init = lapi.Lua_tointeger(LS, 3)
	}
	plain := false
	if lapi.Lua_gettop(LS) >= 4 {
		plain = lapi.Lua_toboolean(LS, 4) != 0
	}
	
	if plain {
		// Plain find
		idx := strings.Index(s[init-1:], pattern)
		if idx >= 0 {
			lapi.Lua_pushinteger(LS, int64(idx+int(init)))
			lapi.Lua_pushinteger(LS, int64(idx+int(init)+len(pattern)-1))
			return 2
		}
	} else {
		// Pattern matching (simplified - just find substring)
		idx := strings.Index(s[init-1:], pattern)
		if idx >= 0 {
			lapi.Lua_pushinteger(LS, int64(idx+int(init)))
			lapi.Lua_pushinteger(LS, int64(idx+int(init)+len(pattern)-1))
			return 2
		}
	}
	return 0
}

// str_match - pattern match
func str_match(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	pattern := lapi.Lua_tolstring(LS, 2, nil)
	
	init := int64(1)
	if lapi.Lua_gettop(LS) >= 3 {
		init = lapi.Lua_tointeger(LS, 3)
	}
	
	// Simplified pattern matching
	if init < 1 {
		init = 1
	}
	if init > int64(len(s)) {
		return 0
	}
	
	// Find pattern in string
	idx := strings.Index(s[init-1:], pattern)
	if idx >= 0 {
		start := int(init) + idx
		end := start + len(pattern)
		lapi.Lua_pushstring(LS, s[start-1:end])
		return 1
	}
	return 0
}

// str_gmatch - global match iterator (simplified)
func str_gmatch(L *lobject.LuaState) int {
	// Simplified: just push the match function
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	lapi.Lua_pushstring(LS, "gmatch not fully implemented")
	return 1
}

// str_gsub - global substitute
func str_gsub(L *lobject.LuaState) int {
	LS := (*lstate.LuaState)(unsafe.Pointer(L))
	s := lapi.Lua_tolstring(LS, 1, nil)
	pattern := lapi.Lua_tolstring(LS, 2, nil)
	repl := lapi.Lua_tolstring(LS, 3, nil)
	n := int64(-1)
	if lapi.Lua_gettop(LS) >= 4 {
		n = lapi.Lua_tointeger(LS, 4)
	}
	
	result := strings.Replace(s, pattern, repl, int(n))
	lapi.Lua_pushstring(LS, result)
	
	// Count replacements
	count := int64(0)
	idx := 0
	for {
		if n >= 0 && count >= n {
			break
		}
		i := strings.Index(s[idx:], pattern)
		if i < 0 {
			break
		}
		count++
		idx += i + len(pattern)
	}
	lapi.Lua_pushinteger(LS, count)
	return 2
}

// str_concat - concatenation metamethod
func str_concat(L *lobject.LuaState) int {
	// Handled by Lua VM
	return 1
}

// str_len metamethod
func str_len_meta(L *lobject.LuaState) int {
	return str_len(L)
}

// str_sub metamethod
func str_sub_meta(L *lobject.LuaState) int {
	return str_sub(L)
}

// str_reverse metamethod
func str_reverse_meta(L *lobject.LuaState) int {
	return str_reverse(L)
}

// str_lower metamethod
func str_lower_meta(L *lobject.LuaState) int {
	return str_lower(L)
}

// str_upper metamethod
func str_upper_meta(L *lobject.LuaState) int {
	return str_upper(L)
}

// str_byte metamethod
func str_byte_meta(L *lobject.LuaState) int {
	return str_byte(L)
}

// str_char metamethod
func str_char_meta(L *lobject.LuaState) int {
	return str_char(L)
}

/*
** String library functions
*/
var stringlibs = []lauxlib.LuaL_Reg{
	{"len", str_len},
	{"sub", str_sub},
	{"reverse", str_reverse},
	{"lower", str_lower},
	{"upper", str_upper},
	{"rep", str_rep},
	{"byte", str_byte},
	{"char", str_char},
	{"dump", str_dump},
	{"format", str_format},
	{"find", str_find},
	{"match", str_match},
	{"gmatch", str_gmatch},
	{"gsub", str_gsub},
}

/*
** OpenString - open string library
 */
func OpenString(L *lstate.LuaState) {
	lauxlib.LuaL_newlib(L, stringlibs)
	
	// Set string metatable for __len, __sub, etc.
	// This is a simplified version
}
