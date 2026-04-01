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
	"fmt"
	"regexp"
	"strings"
	"unicode"

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
	// Create "string" table: 0 array elements, 12 predefined fields
	L.CreateTable(0, 12)

	// Register all string functions using PushGoFunction + SetField
	register := func(name string, fn stringlib.LuaFunc) {
		L.PushGoFunction(fn)
		L.SetField(-2, name)
	}

	register("len", strLen)
	register("sub", strSub)
	register("upper", strUpper)
	register("lower", strLower)
	register("reverse", strReverse)
	register("rep", strRep)
	register("byte", strByte)
	register("char", strChar)
	register("find", strFind)
	register("match", strMatch)
	register("gsub", strGsub)
	register("format", strFormat)

	// luaopen_string convention: return 1 (the module table stays on stack)
	return 1
}

// Ensure StringLib implements StringLib interface
var _ stringlib.StringLib = (*StringLib)(nil)

// Ensure types implement LuaFunc (compile-time check)
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
	s, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}
	// Use rune count for proper UTF-8 handling
	runes := []rune(s)
	L.PushInteger(int64(len(runes)))
	return 1
}

// strSub returns a substring.
// string.sub(s, i [, j]) -> string
// Lua string indices are 1-based. Negative indices wrap from end.
// i=1 returns from first char, j=-1 returns to last char.
func strSub(L stringlib.LuaAPI) int {
	s, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	runes := []rune(s)
	length := len(runes)

	// Get start position (default 1)
	starti := toIntegerOpt(L, 2, 1)
	start := posrelat(int64(starti), int64(length))
	if start < 1 {
		start = 1
	}

	// Get end position (default -1)
	endi := toIntegerOpt(L, 3, -1)
	end := posrelat(int64(endi), int64(length))
	if end > int64(length) {
		end = int64(length)
	}

	// Handle empty result
	if start > end || start > int64(length) {
		L.PushString("")
		return 1
	}

	// Extract substring (convert back to 0-based indices)
	result := string(runes[start-1 : end])
	L.PushString(result)
	return 1
}

// strUpper returns a copy of the string with all characters uppercased.
// string.upper(s) -> string
func strUpper(L stringlib.LuaAPI) int {
	s, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}
	L.PushString(strings.ToUpper(s))
	return 1
}

// strLower returns a copy of the string with all characters lowercased.
// string.lower(s) -> string
func strLower(L stringlib.LuaAPI) int {
	s, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}
	L.PushString(strings.ToLower(s))
	return 1
}

// strReverse returns a copy of the string with characters in reverse order.
// string.reverse(s) -> string
func strReverse(L stringlib.LuaAPI) int {
	s, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	runes := []rune(s)
	// Reverse the runes
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	L.PushString(string(runes))
	return 1
}

// strRep returns a string that is the concatenation of n copies of the string s.
// string.rep(s, n [, sep]) -> string
func strRep(L stringlib.LuaAPI) int {
	s, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	n := toIntegerOpt(L, 2, 0)
	sep := toStringOpt(L, 3, "")

	if n < 0 {
		n = 0
	}

	var builder strings.Builder
	for i := int64(0); i < n; i++ {
		if i > 0 && sep != "" {
			builder.WriteString(sep)
		}
		builder.WriteString(s)
	}
	L.PushString(builder.String())
	return 1
}

// strByte returns the internal numeric codes of the characters s[i], s[i+1], ..., s[j].
// string.byte(s [, i [, j]]) -> integer...
// Returns values in range 0-255.
func strByte(L stringlib.LuaAPI) int {
	s, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	runes := []rune(s)
	length := len(runes)
	if length == 0 {
		return 0
	}

	// Get start position (default 1)
	starti := toIntegerOpt(L, 2, 1)
	start := posrelat(int64(starti), int64(length))
	if start < 1 {
		start = 1
	}

	// Get end position (default start)
	endi := toIntegerOpt(L, 3, int64(starti))
	end := posrelat(int64(endi), int64(length))
	if end > int64(length) {
		end = int64(length)
	}

	// Empty interval
	if start > end {
		return 0
	}

	// Push each byte value (rune as int)
	// start and end are 1-based, convert to 0-based for slice
	for i := int(start); i <= int(end); i++ {
		if i >= 1 && i <= length {
			L.PushInteger(int64(runes[i-1]))
		}
	}
	return int(end - start + 1)
}

// strChar returns a string with length equal to the number of arguments.
// string.char(...) -> string
// Each argument should be an integer in range 0-255.
func strChar(L stringlib.LuaAPI) int {
	n := L.GetTop()
	if n == 0 {
		L.PushString("")
		return 1
	}

	var builder strings.Builder
	for i := 1; i <= n; i++ {
		c := toInteger(L, i)
		// Clamp to valid range
		if c < 0 {
			c = 0
		}
		if c > 255 {
			c = 255
		}
		builder.WriteRune(rune(c))
	}
	L.PushString(builder.String())
	return 1
}

// strFind looks for the first match of pattern in the string s.
// string.find(s, pattern [, init [, plain]]) -> start, end | nil
// When plain is true, pattern is treated as literal string.
func strFind(L stringlib.LuaAPI) int {
	s, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	pattern, ok := L.ToString(2)
	if !ok {
		L.PushNil()
		return 1
	}

	runes := []rune(s)
	patternRunes := []rune(pattern)
	length := len(runes)
	patternLen := len(patternRunes)

	// Get init position (default 1)
	init := toIntegerOpt(L, 3, 1)
	pos := posrelat(int64(init), int64(length))
	if pos < 1 {
		pos = 1
	}

	// Check for plain mode
	plain := toBooleanOpt(L, 4, false)

	if plain {
		// Plain string search (no pattern matching)
		if patternLen == 0 {
			// Empty pattern matches at start position
			L.PushInteger(pos)
			L.PushInteger(pos - 1)
			return 2
		}

		// Search for pattern in string (use int for loop indices)
		// Convert to 0-based starting position
		start := int(pos) - 1
		lengthInt := int(length)
		patternLenInt := int(patternLen)
		
		// Bounds check
		if start < 0 {
			start = 0
		}
		
		for i := start; i < lengthInt; i++ {
			// Check if pattern fits at position i
			if i + patternLenInt > lengthInt {
				break
			}
			found := true
			for j := 0; j < patternLenInt; j++ {
				if runes[i+j] != patternRunes[j] {
					found = false
					break
				}
			}
			if found {
				// Return 1-based indices
				L.PushInteger(int64(i + 1))
				L.PushInteger(int64(i + patternLenInt))
				return 2
			}
		}
		L.PushNil()
		return 1
	}

	// Pattern matching mode - convert Lua pattern to regex
	regexPattern := luaPatternToRegex(string(pattern))
	
	// Find the match position within the substring starting at pos
	searchStr := string(runes[pos-1:])
	
	// Use regex to find match (don't add ^ anchor)
	re := regexp.MustCompile(regexPattern)
	loc := re.FindStringIndex(searchStr)
	if loc == nil {
		L.PushNil()
		return 1
	}

	// Return 1-based indices (offset by pos-1)
	startPos := int64(pos) + int64(loc[0])
	endPos := int64(pos) + int64(loc[1]) - 1
	L.PushInteger(startPos)
	L.PushInteger(endPos)
	return 2
}

// strMatch looks for the first match of pattern in the string s.
// string.match(s, pattern [, init]) -> string | nil
func strMatch(L stringlib.LuaAPI) int {
	s, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	pattern, ok := L.ToString(2)
	if !ok {
		L.PushNil()
		return 1
	}

	runes := []rune(s)
	length := len(runes)

	// Get init position (default 1)
	init := toIntegerOpt(L, 3, 1)
	pos := posrelat(int64(init), int64(length))
	if pos < 1 {
		pos = 1
	}

	// Convert Lua pattern to regex and find match
	regexPattern := luaPatternToRegex(pattern)
	searchIn := string(runes[pos-1:])
	re := regexp.MustCompile(regexPattern)
	match := re.FindString(searchIn)

	if match == "" {
		L.PushNil()
		return 1
	}

	L.PushString(match)
	return 1
}

// strGsub returns a copy of s in which all occurrences of the pattern have been replaced.
// string.gsub(s, pattern, repl [, n]) -> string, count
func strGsub(L stringlib.LuaAPI) int {
	s, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		L.PushInteger(0)
		return 2
	}

	pattern, ok := L.ToString(2)
	if !ok {
		L.PushNil()
		L.PushInteger(0)
		return 2
	}

	repl, ok := L.ToString(3)
	if !ok {
		// Replacement can be a number or function too, but for simplicity handle string
		L.PushNil()
		L.PushInteger(0)
		return 2
	}

	// Get max replacements (default: all)
	n := toIntegerOpt(L, 4, -1)
	if n < 0 {
		n = -1
	}

	// Convert Lua pattern to regex
	regexPattern := luaPatternToRegex(pattern)
	re := regexp.MustCompile(regexPattern)

	// Count matches
	count := len(re.FindAllStringIndex(s, -1))
	if n >= 0 && count > int(n) {
		count = int(n)
	}

	// Replace (handle % escapes in replacement)
	result := luaGsub(s, regexPattern, repl, int(n))

	L.PushString(result)
	L.PushInteger(int64(count))
	return 2
}

// strFormat formats a string.
// string.format(formatstring, ...) -> string
// Supports: %s, %d, %i, %f, %.nf, %c, %x, %X, %o, %e, %E, %g, %G, %a, %A
func strFormat(L stringlib.LuaAPI) int {
	formatStr, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	result, err := luaFormat(L, formatStr)
	if err != nil {
		L.PushNil()
		return 1
	}

	L.PushString(result)
	return 1
}

// =============================================================================
// Helper Functions
// =============================================================================

// posrelat converts a Lua 1-based position to a 0-based index.
// Handles negative indices (wrapping from end).
func posrelat(pos, length int64) int64 {
	if pos > 0 {
		return pos
	}
	if pos == 0 {
		return 1
	}
	// Negative: wrap from end
	if pos < -length {
		return 1
	}
	return length + pos + 1
}

// luaPatternToRegex converts a Lua pattern to a Go regular expression.
// Handles: % escape, . any, %a alpha, %d digit, %s space, %l lower, %u upper, %w word
// Also handles character classes: [abc], [^abc], [a-z]
func luaPatternToRegex(pattern string) string {
	var result strings.Builder
	result.WriteString("(?s)") // Enable dotall mode

	i := 0
	for i < len(pattern) {
		c := pattern[i]
		switch c {
		case '%':
			if i+1 < len(pattern) {
				next := pattern[i+1]
				// Handle pattern classes that can be followed by modifiers
				canHaveModifier := false
				switch next {
				case '.':
					result.WriteString(".")
					canHaveModifier = true
				case 'a':
					result.WriteString("[A-Za-z]")
					canHaveModifier = true
				case 'c':
					result.WriteString("[\\x00-\\x1F\\x7F]")
					canHaveModifier = true
				case 'd':
					result.WriteString("[0-9]")
					canHaveModifier = true
				case 'g':
					result.WriteString("[!-~]")
					canHaveModifier = true
				case 'l':
					result.WriteString("[a-z]")
					canHaveModifier = true
				case 'p':
					result.WriteString("[\\x21-\\x2F\\x3A-\\x40\\x5B-\\x60\\x7B-\\x7E]")
					canHaveModifier = true
				case 's':
					result.WriteString("[ \\t\\n\\r\\f\\v]")
					canHaveModifier = true
				case 'u':
					result.WriteString("[A-Z]")
					canHaveModifier = true
				case 'w':
					result.WriteString("[A-Za-z0-9]")
					canHaveModifier = true
				case 'x':
					result.WriteString("[0-9A-Fa-f]")
					canHaveModifier = true
				case 'A':
					result.WriteString("[^A-Za-z]")
				case 'C':
					result.WriteString("[^\\x00-\\x1F\\x7F]")
				case 'D':
					result.WriteString("[^0-9]")
				case 'G':
					result.WriteString("[^!-~]")
				case 'L':
					result.WriteString("[^a-z]")
				case 'P':
					result.WriteString("[^\\x21-\\x2F\\x3A-\\x40\\x5B-\\x60\\x7B-\\x7E]")
				case 'S':
					result.WriteString("[^ \\t\\n\\r\\f\\v]")
				case 'U':
					result.WriteString("[^A-Z]")
				case 'W':
					result.WriteString("[^A-Za-z0-9]")
				case 'X':
					result.WriteString("[^0-9A-Fa-f]")
				case 'z':
					result.WriteString("\\x00")
				case 'Z':
					result.WriteString("[^\\x00]")
				case '(':
					result.WriteString("(")
				case ')':
					result.WriteString(")")
				case '[':
					// Character class
					result.WriteString("[")
					i++
					for i < len(pattern) && pattern[i] != ']' {
						if pattern[i] == '%' && i+1 < len(pattern) {
							i++
							result.WriteString("\\")
							result.WriteByte(pattern[i])
						} else {
							result.WriteByte(pattern[i])
						}
						i++
					}
					result.WriteString("]")
				default:
					// Escape special character
					result.WriteString(regexp.QuoteMeta(string(next)))
				}
				
				// Check for modifiers after pattern class
				if canHaveModifier && i+2 < len(pattern) {
					mod := pattern[i+2]
					switch mod {
					case '*':
						result.WriteString("*")
						i += 2
					case '+':
						result.WriteString("+")
						i += 2
					case '?':
						result.WriteString("?")
						i += 2
					}
				}
				i += 2
			} else {
				result.WriteByte(c)
				i++
			}
		case '[':
			// Character class - copy as-is
			result.WriteByte(c)
			i++
			for i < len(pattern) && pattern[i] != ']' {
				result.WriteByte(pattern[i])
				i++
			}
			if i < len(pattern) {
				result.WriteByte(pattern[i])
				i++
			}
		case '(':
			// Capture group
			result.WriteString("(")
			i++
		case ')':
			// End capture group
			result.WriteString(")")
			i++
		case '$':
			// End of string anchor
			result.WriteString("$")
			i++
		default:
			// Escape regex special characters
			result.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}

	return result.String()
}

// luaGsub performs pattern replacement with Lua-style % escapes.
func luaGsub(s, pattern, repl string, n int) string {
	re := regexp.MustCompile(pattern)

	if n < 0 {
		return re.ReplaceAllStringFunc(s, func(match string) string {
			return processLuaReplacement(match, repl)
		})
	}

	count := 0
	return re.ReplaceAllStringFunc(s, func(match string) string {
		if count >= n {
			return match
		}
		count++
		return processLuaReplacement(match, repl)
	})
}

// processLuaReplacement handles % escapes in replacement string.
func processLuaReplacement(match, repl string) string {
	var result strings.Builder

	i := 0
	for i < len(repl) {
		c := repl[i]
		if c == '%' && i+1 < len(repl) {
			next := repl[i+1]
			switch next {
			case '%':
				result.WriteByte('%')
				i += 2
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
				// Capture group reference - simplified, just return match for now
				result.WriteString(match)
				i += 2
			default:
				result.WriteByte(next)
				i += 2
			}
		} else {
			result.WriteByte(c)
			i++
		}
	}

	return result.String()
}

// luaFormat implements Lua's string.format function.
func luaFormat(L stringlib.LuaAPI, formatStr string) (string, error) {
	var result strings.Builder
	i := 0
	argIndex := 2 // First format argument is at index 2

	for i < len(formatStr) {
		c := formatStr[i]
		if c != '%' {
			result.WriteByte(c)
			i++
			continue
		}

		// Check for %%
		if i+1 < len(formatStr) && formatStr[i+1] == '%' {
			result.WriteByte('%')
			i += 2
			continue
		}

		// Parse format specifier
		format, consumed, err := parseLuaFormat(formatStr[i:])
		if err != nil {
			return "", err
		}

		// Get the argument
		if argIndex > L.GetTop() {
			return "", fmt.Errorf("no value for argument %d", argIndex)
		}

		// Format the argument
		formatted, err := formatLuaValue(L, argIndex, format)
		if err != nil {
			return "", err
		}
		result.WriteString(formatted)

		i += consumed
		argIndex++
	}

	return result.String(), nil
}

// parseLuaFormat parses a Lua format specifier and returns the Go format string.
func parseLuaFormat(s string) (string, int, error) {
	if len(s) == 0 || s[0] != '%' {
		return "", 0, fmt.Errorf("invalid format")
	}

	// Find the specifier character
	specIndex := 1
	for specIndex < len(s) {
		c := rune(s[specIndex])
		if unicode.IsLetter(c) || c == '%' {
			break
		}
		specIndex++
	}

	if specIndex >= len(s) {
		return "", 0, fmt.Errorf("invalid format")
	}

	formatSpec := s[:specIndex+1]

	// Convert Lua format spec to Go format
	goFormat, err := luaFormatToGo(formatSpec)
	if err != nil {
		return "", 0, err
	}

	return goFormat, specIndex + 1, nil
}

// luaFormatToGo converts Lua format specifier to Go fmt format.
func luaFormatToGo(luaFormat string) (string, error) {
	if len(luaFormat) < 2 {
		return "", fmt.Errorf("invalid format")
	}

	// Get the specifier character
	spec := luaFormat[len(luaFormat)-1]

	// Get flags, width, precision
	flags := luaFormat[1 : len(luaFormat)-1]

	switch spec {
	case 's':
		return "%" + flags + "s", nil
	case 'd', 'i':
		return "%" + flags + "d", nil
	case 'u':
		return "%" + flags + "d", nil // Lua uses %u but we treat as int
	case 'o':
		return "%" + flags + "o", nil
	case 'x':
		return "%" + flags + "x", nil
	case 'X':
		return "%" + flags + "X", nil
	case 'f', 'F':
		return "%" + flags + "f", nil
	case 'e':
		return "%" + flags + "e", nil
	case 'E':
		return "%" + flags + "E", nil
	case 'g':
		return "%" + flags + "g", nil
	case 'G':
		return "%" + flags + "G", nil
	case 'a':
		return "%" + flags + "a", nil
	case 'A':
		return "%" + flags + "A", nil
	case 'c':
		return "%" + flags + "c", nil
	case 'p':
		return "%" + flags + "p", nil
	case 'q':
		// %q is not directly supported, convert to %s with quoting
		return "%" + flags + "q", nil
	default:
		return "", fmt.Errorf("invalid format specifier: %c", spec)
	}
}

// formatLuaValue formats a Lua value according to the format string.
func formatLuaValue(L stringlib.LuaAPI, idx int, format string) (string, error) {
	tp := L.Type(idx)

	switch tp {
	case 0: // LUA_TNIL
		return "nil", nil
	case 1: // LUA_TBOOLEAN
		if L.ToBoolean(idx) {
			return "true", nil
		}
		return "false", nil
	case 4: // LUA_TSTRING
		s, _ := L.ToString(idx)
		return fmt.Sprintf(format, s), nil
	case 3: // LUA_TNUMBER
		if L.IsInteger(idx) {
			i, _ := L.ToInteger(idx)
			return fmt.Sprintf(format, i), nil
		}
		f, _ := L.ToNumber(idx)
		return fmt.Sprintf(format, f), nil
	default:
		// For other types, convert to string
		s, ok := L.ToString(idx)
		if ok {
			return s, nil
		}
		return "", fmt.Errorf("bad argument")
	}
}

// =============================================================================
// Helper Functions (using raw LuaAPI methods)
// =============================================================================

// toString extracts a string at the given index.
func toString(L stringlib.LuaAPI, idx int) string {
	s, ok := L.ToString(idx)
	if !ok {
		return ""
	}
	return s
}

// toStringOpt returns string at idx, or def if nil/absent.
func toStringOpt(L stringlib.LuaAPI, idx int, def string) string {
	if L.IsNoneOrNil(idx) {
		return def
	}
	return toString(L, idx)
}

// toInteger extracts an integer at the given index.
func toInteger(L stringlib.LuaAPI, idx int) int64 {
	if L.IsInteger(idx) {
		i, _ := L.ToInteger(idx)
		return i
	}
	// Try as number
	if L.IsNumber(idx) {
		n, _ := L.ToNumber(idx)
		return int64(n)
	}
	return 0
}

// toIntegerOpt returns integer at idx, or def if nil/absent.
func toIntegerOpt(L stringlib.LuaAPI, idx int, def int64) int64 {
	if L.IsNoneOrNil(idx) {
		return def
	}
	return toInteger(L, idx)
}

// toNumber extracts a number at the given index.
func toNumber(L stringlib.LuaAPI, idx int) float64 {
	n, _ := L.ToNumber(idx)
	return n
}

// toBoolean extracts a boolean at the given index.
func toBoolean(L stringlib.LuaAPI, idx int) bool {
	return L.ToBoolean(idx)
}

// toBooleanOpt returns boolean at idx, or def if nil/absent.
func toBooleanOpt(L stringlib.LuaAPI, idx int, def bool) bool {
	if L.IsNoneOrNil(idx) {
		return def
	}
	return toBoolean(L, idx)
}
