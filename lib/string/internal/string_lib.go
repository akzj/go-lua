// Package internal implements the Lua string library.
// This package provides implementations for:
//   - string.len(s): string length
//   - string.sub(s, i, j): substring
//   - string.upper(s) / string.lower(s): case conversion
//   - string.find(s, pattern): find substring with pattern
//   - string.match(s, pattern): match pattern
//   - string.gsub(s, pattern, repl): global substitution
//   - string.format(fmt, ...): string formatting
//   - string.byte(s, i, j) / string.char(...): byte conversion
//   - string.rep(s, n): string repetition
//   - string.reverse(s): string reversal
//
// Reference: lua-master/lstrlib.c
package internal

import (
	stringlib "github.com/akzj/go-lua/lib/string/api"
)

// StringLib is the implementation of the Lua string library.
type StringLib struct{}

// NewStringLib creates a new StringLib instance.
func NewStringLib() stringlib.StringLib {
	return &StringLib{}
}

// Open implements stringlib.StringLib.Open.
// Registers all string library functions in the global table under "string".
func (s *StringLib) Open(L stringlib.LuaAPI) int {
	panic("TODO: implement string library Open")
}

// Ensure types implement LuaFunc
var _ stringlib.LuaFunc = strLen
var _ stringlib.LuaFunc = strSub
var _ stringlib.LuaFunc = strUpper
var _ stringlib.LuaFunc = strLower
var _ stringlib.LuaFunc = strReverse
var _ stringlib.LuaFunc = strRep
var _ stringlib.LuaFunc = strByte
var _ stringlib.LuaFunc = strChar
var _ stringlib.LuaFunc = strFind
var _ stringlib.LuaFunc = strMatch
var _ stringlib.LuaFunc = strGsub
var _ stringlib.LuaFunc = strFormat

// =============================================================================
// String Functions
// =============================================================================

// strLen returns the length of a string.
// string.len(s) -> integer
func strLen(L stringlib.LuaAPI) int {
	panic("TODO: implement strLen")
}

// strSub returns a substring.
// string.sub(s, i [, j]) -> string
func strSub(L stringlib.LuaAPI) int {
	panic("TODO: implement strSub")
}

// strUpper returns a copy of the string with all characters uppercased.
// string.upper(s) -> string
func strUpper(L stringlib.LuaAPI) int {
	panic("TODO: implement strUpper")
}

// strLower returns a copy of the string with all characters lowercased.
// string.lower(s) -> string
func strLower(L stringlib.LuaAPI) int {
	panic("TODO: implement strLower")
}

// strReverse returns a copy of the string with characters in reverse order.
// string.reverse(s) -> string
func strReverse(L stringlib.LuaAPI) int {
	panic("TODO: implement strReverse")
}

// strRep returns a string that is the concatenation of n copies of the string s.
// string.rep(s, n [, sep]) -> string
func strRep(L stringlib.LuaAPI) int {
	panic("TODO: implement strRep")
}

// strByte returns the internal numeric codes of the characters s[i], s[i+1], ..., s[j].
// string.byte(s [, i [, j]]) -> integer...
func strByte(L stringlib.LuaAPI) int {
	panic("TODO: implement strByte")
}

// strChar returns a string with length equal to the number of arguments.
// string.char(...) -> string
func strChar(L stringlib.LuaAPI) int {
	panic("TODO: implement strChar")
}

// strFind looks for the first match of pattern in the string s.
// string.find(s, pattern [, init [, plain]]) -> start, end | nil
func strFind(L stringlib.LuaAPI) int {
	panic("TODO: implement strFind")
}

// strMatch looks for the first match of pattern in the string s.
// string.match(s, pattern [, init]) -> string | nil
func strMatch(L stringlib.LuaAPI) int {
	panic("TODO: implement strMatch")
}

// strGsub returns a copy of s in which all occurrences of the pattern have been replaced.
// string.gsub(s, pattern, repl [, n]) -> string, count
func strGsub(L stringlib.LuaAPI) int {
	panic("TODO: implement strGsub")
}

// strFormat formats a string.
// string.format(formatstring, ...) -> string
func strFormat(L stringlib.LuaAPI) int {
	panic("TODO: implement strFormat")
}
