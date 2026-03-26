// Package api provides the public Lua API
// This file implements the string standard library module
package api

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/akzj/go-lua/pkg/object"
)

// openStringLib registers the string module
func (s *State) openStringLib() {
	funcs := map[string]Function{
		"len":     stdStringLength,
		"sub":     stdStringSub,
		"upper":   stdStringUpper,
		"lower":   stdStringLower,
		"find":    stdStringFind,
		"format":  stdStringFormat,
		"rep":     stdStringRep,
		"reverse": stdStringReverse,
		"byte":    stdStringByte,
		"char":    stdStringChar,
		"gsub":    stdStringGsub,
		"match":   stdStringMatch,
		"gmatch":  stdStringGmatch,
	}
	s.RegisterModule("string", funcs)

	// Create string metatable with __index pointing to string module
	// Get the string module from globals (it was just registered)
	globalTable := s.getGlobalTable()
	key := object.TValue{Type: object.TypeString, Value: object.Value{Str: "string"}}
	stringModule := globalTable.Get(key)
	if stringModule != nil && stringModule.IsTable() {
		stringModuleTable, _ := stringModule.ToTable()
		metatable := object.NewTable()
		indexPath := object.NewTableValue(stringModuleTable)
		metatable.Set(object.TValue{Type: object.TypeString, Value: object.Value{Str: "__index"}}, *indexPath)
		s.vm.Global.StringMetatable = metatable
	}
}

// stdStringLength implements string.len(s)
// Returns the length of string s.
func stdStringLength(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushNumber(0)
		return 1
	}
	L.PushNumber(float64(utf8.RuneCountInString(str)))
	return 1
}

// stdStringSub implements string.sub(s, i [, j])
// Returns substring from i to j (inclusive). Negative indices count from end.
func stdStringSub(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushString("")
		return 1
	}

	// Convert to runes for proper Unicode handling
	runes := []rune(str)
	n := len(runes)

	// Get start index (required)
	i := 1
	if L.GetTop() >= 2 {
		if num, ok := L.ToNumber(2); ok {
			i = int(num)
		}
	}

	// Get end index (optional, defaults to -1 = end)
	j := n
	if L.GetTop() >= 3 {
		if num, ok := L.ToNumber(3); ok {
			j = int(num)
		}
	}

	// Handle negative indices
	if i < 0 {
		i = n + i + 1
	}
	if j < 0 {
		j = n + j + 1
	}

	// Clamp to valid range
	if i < 1 {
		i = 1
	}
	if j > n {
		j = n
	}

	// Return substring
	if i > j {
		L.PushString("")
	} else {
		L.PushString(string(runes[i-1 : j]))
	}
	return 1
}

// stdStringUpper implements string.upper(s)
// Returns uppercase version of s.
func stdStringUpper(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushString("")
		return 1
	}
	L.PushString(strings.ToUpper(str))
	return 1
}

// stdStringLower implements string.lower(s)
// Returns lowercase version of s.
func stdStringLower(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushString("")
		return 1
	}
	L.PushString(strings.ToLower(str))
	return 1
}

// stdStringFind implements string.find(s, pattern [, init [, plain]])
// Returns start and end indices of first match, or nil if not found.
// Note: Simple string matching only (no full Lua patterns).
func stdStringFind(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	pattern, ok := L.ToString(2)
	if !ok {
		L.PushNil()
		return 1
	}

	// Get start position (default 1)
	init := 1
	if L.GetTop() >= 3 {
		if num, ok := L.ToNumber(3); ok {
			init = int(num)
		}
	}

	// Handle negative init
	if init < 0 {
		init = len(str) + init + 1
	}
	if init < 1 {
		init = 1
	}

	// Get plain flag (default false for patterns, but we do simple matching)
	plain := false
	if L.GetTop() >= 4 {
		plain = L.IsTruthy(4)
	}
	_ = plain // We always do simple string matching per constraints

	// Simple string find (no pattern matching)
	if init > len(str) {
		L.PushNil()
		return 1
	}

	idx := strings.Index(str[init-1:], pattern)
	if idx == -1 {
		L.PushNil()
		return 1
	}

	// Return 1-based indices
	start := init + idx
	end := start + len(pattern) - 1
	L.PushNumber(float64(start))
	L.PushNumber(float64(end))
	return 2
}

// stdStringFormat implements string.format(formatstring, ...)
// Returns formatted string (sprintf-like).
func stdStringFormat(L *State) int {
	format, ok := L.ToString(1)
	if !ok {
		L.PushString("")
		return 1
	}

	// Simple implementation: handle %s, %d, %f, %%
	args := make([]interface{}, 0)
	top := L.GetTop()
	for i := 2; i <= top; i++ {
		v := L.vm.GetStack(i)
		switch v.Type {
		case object.TypeNumber:
			num, _ := v.ToNumber()
			// Check if integer for %d
			if num == float64(int(num)) {
				args = append(args, int(num))
			} else {
				args = append(args, num)
			}
		case object.TypeString:
			str, _ := v.ToString()
			args = append(args, str)
		case object.TypeBoolean:
			b, _ := v.ToBoolean()
			args = append(args, b)
		case object.TypeNil:
			args = append(args, "nil")
		default:
			args = append(args, fmt.Sprintf("%s: %p", v.Type.String(), v.Value.GC))
		}
	}

	result, err := formatString(format, args...)
	if err != nil {
		L.PushString(err.Error())
		return 1
	}

	L.PushString(result)
	return 1
}

// formatString performs simple string formatting
func formatString(format string, args ...interface{}) (string, error) {
	// Simple implementation using fmt.Sprintf-like behavior
	// This handles basic %s, %d, %f, %%, %q formats
	var result strings.Builder
	argIdx := 0

	for i := 0; i < len(format); i++ {
		if format[i] == '%' && i+1 < len(format) {
			spec := format[i+1]
			switch spec {
			case '%':
				result.WriteByte('%')
				i++
			case 's', 'q':
				if argIdx < len(args) {
					if spec == 'q' {
						result.WriteString(fmt.Sprintf("%q", args[argIdx]))
					} else {
						result.WriteString(fmt.Sprint(args[argIdx]))
					}
					argIdx++
				}
				i++
			case 'd', 'i':
				if argIdx < len(args) {
					switch v := args[argIdx].(type) {
					case int:
						result.WriteString(strconv.Itoa(v))
					case float64:
						result.WriteString(strconv.Itoa(int(v)))
					default:
						result.WriteString(fmt.Sprint(v))
					}
					argIdx++
				}
				i++
			case 'f':
				if argIdx < len(args) {
					switch v := args[argIdx].(type) {
					case float64:
						result.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
					case int:
						result.WriteString(strconv.FormatFloat(float64(v), 'f', -1, 64))
					default:
						result.WriteString(fmt.Sprint(v))
					}
					argIdx++
				}
				i++
			default:
				result.WriteByte(format[i])
			}
		} else {
			result.WriteByte(format[i])
		}
	}

	return result.String(), nil
}

// stdStringRep implements string.rep(s, n [, sep])
// Returns s repeated n times, separated by sep.
func stdStringRep(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushString("")
		return 1
	}

	n := 0
	if num, ok := L.ToNumber(2); ok {
		n = int(num)
	}

	sep := ""
	if L.GetTop() >= 3 {
		sep, _ = L.ToString(3)
	}

	if n <= 0 {
		L.PushString("")
		return 1
	}

	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = str
	}
	L.PushString(strings.Join(parts, sep))
	return 1
}

// stdStringReverse implements string.reverse(s)
// Returns s reversed.
func stdStringReverse(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushString("")
		return 1
	}

	runes := []rune(str)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	L.PushString(string(runes))
	return 1
}

// stdStringByte implements string.byte(s [, i [, j]])
// Returns internal numeric codes of characters s[i] through s[j].
func stdStringByte(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		return 0
	}

	if len(str) == 0 {
		return 0
	}

	// Default: first character
	i := 1
	j := i

	if L.GetTop() >= 2 {
		if num, ok := L.ToNumber(2); ok {
			i = int(num)
		}
		if L.GetTop() >= 3 {
			if num, ok := L.ToNumber(3); ok {
				j = int(num)
			}
		} else {
			j = i // Only i specified
		}
	}

	// Handle negative indices
	runes := []rune(str)
	n := len(runes)
	if i < 0 {
		i = n + i + 1
	}
	if j < 0 {
		j = n + j + 1
	}

	// Clamp to valid range
	if i < 1 {
		i = 1
	}
	if j > n {
		j = n
	}

	// Return byte values
	count := 0
	for k := i; k <= j; k++ {
		if k >= 1 && k <= n {
			L.PushNumber(float64(runes[k-1]))
			count++
		}
	}
	return count
}

// stdStringChar implements string.char(...)
// Returns string with given character codes.
func stdStringChar(L *State) int {
	top := L.GetTop()
	runes := make([]rune, 0, top)

	for i := 1; i <= top; i++ {
		if num, ok := L.ToNumber(i); ok {
			runes = append(runes, rune(int(num)))
		}
	}

	L.PushString(string(runes))
	return 1
}

// stdStringGsub implements string.gsub(s, pattern, repl [, n])
// Returns copy of s with occurrences replaced, plus count.
// Note: Simple string matching only (no full Lua patterns).
func stdStringGsub(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushString("")
		L.PushNumber(0)
		return 2
	}

	pattern, ok := L.ToString(2)
	if !ok {
		L.PushString(str)
		L.PushNumber(0)
		return 2
	}

	repl := ""
	if L.GetTop() >= 3 {
		repl, _ = L.ToString(3)
	}

	maxRepl := -1
	if L.GetTop() >= 4 {
		if num, ok := L.ToNumber(4); ok {
			maxRepl = int(num)
		}
	}

	// Simple string replace
	result := str
	count := 0
	if maxRepl < 0 {
		result = strings.ReplaceAll(str, pattern, repl)
		count = strings.Count(str, pattern)
	} else {
		result = strings.Replace(str, pattern, repl, maxRepl)
		count = strings.Count(str, pattern)
		if count > maxRepl {
			count = maxRepl
		}
	}

	L.PushString(result)
	L.PushNumber(float64(count))
	return 2
}

// stdStringMatch implements string.match(s, pattern [, init])
// Returns first match of pattern in s.
// Note: Simple string matching only (no full Lua patterns).
func stdStringMatch(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	pattern, ok := L.ToString(2)
	if !ok {
		L.PushNil()
		return 1
	}

	init := 1
	if L.GetTop() >= 3 {
		if num, ok := L.ToNumber(3); ok {
			init = int(num)
		}
	}

	// Handle negative init
	if init < 0 {
		init = len(str) + init + 1
	}
	if init < 1 {
		init = 1
	}

	// Simple string match
	if init > len(str) {
		L.PushNil()
		return 1
	}

	idx := strings.Index(str[init-1:], pattern)
	if idx == -1 {
		L.PushNil()
		return 1
	}

	// Return the matched string
	L.PushString(pattern)
	return 1
}

// stdStringGmatch implements string.gmatch(s, pattern)
// Returns an iterator function for matches.
// Note: Simple string matching only (no full Lua patterns).
func stdStringGmatch(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	pattern, ok := L.ToString(2)
	if !ok {
		L.PushNil()
		return 1
	}

	// Create iterator function
	// We need to capture the current position
	pos := 0

	L.PushFunction(func(L *State) int {
		if pos > len(str) {
			return 0
		}

		idx := strings.Index(str[pos:], pattern)
		if idx == -1 {
			return 0
		}

		// Return the match
		L.PushString(pattern)
		pos = pos + idx + len(pattern)
		return 1
	})

	return 1
}