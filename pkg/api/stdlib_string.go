// Package api provides the public Lua API
// This file implements the string standard library module
package api

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unsafe"

	"github.com/akzj/go-lua/pkg/object"
)

// shortStringPtrs caches stable pointers for short strings (≤40 bytes)
// In Lua 5.4, short strings are interned and share the same pointer
var shortStringPtrs sync.Map // map[string]uintptr

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
		L.PushInteger(0)
		return 1
	}
	L.PushInteger(int64(len(str)))
	return 1
}

// stdStringSub implements string.sub(s, i [, j])
// Returns substring from i to j (inclusive). Negative indices count from end.
// stdStringSub implements string.sub(s, i [, j])
// Returns substring from i to j (inclusive). Negative indices count from end.
func stdStringSub(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushString("")
		return 1
	}

	n := len(str)

	// Get start index (required)
	i := 1.0
	if L.GetTop() >= 2 {
		if num, ok := L.ToNumber(2); ok {
			i = num
		}
	}

	// Get end index (optional, defaults to end of string)
	j := float64(n)
	if L.GetTop() >= 3 {
		if num, ok := L.ToNumber(3); ok {
			j = num
		}
	}

	// Handle negative indices (count from end)
	if i < 0 {
		i = float64(n) + i + 1
	}
	if j < 0 {
		j = float64(n) + j + 1
	}

	// Clamp to valid range [1, n] BEFORE converting to int to prevent overflow
	// Only clamp lower bound for i, upper bound for j
	// This ensures out-of-bounds start (i > n) results in i > j → empty string
	if i < 1 {
		i = 1
	}
	if j > float64(n) {
		j = float64(n)
	}

	// Now safe to convert to int
	iInt := int(i)
	jInt := int(j)

	// Return substring
	if iInt > jInt {
		L.PushString("")
	} else {
		L.PushString(str[iInt-1 : jInt])
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

// ============================================================================
// Lua Pattern Matching Implementation
// ============================================================================

// patternItemType represents the type of a pattern item
type patternItemType int

const (
	itemLiteral patternItemType = iota // literal character
	itemClass                          // %a, %d, etc.
	itemAny                            // . (any character)
	itemSet                            // [abc] or [^abc]
	itemAnchorStart                    // ^ at start
	itemAnchorEnd                      // $ at end
	itemCapture                        // () position capture
	itemGroupStart                     // ( start capture group
	itemGroupEnd                       // ) end capture group
)

// quantifierType represents how many times to match
type quantifierType int

const (
	quantOne quantifierType = iota // exactly one
	quantZeroPlus                   // * 0 or more (greedy)
	quantOnePlus                    // + 1 or more (greedy)
	quantZeroMinus                  // - 0 or more (non-greedy)
	quantOptional                   // ? 0 or 1
)

// patternItem represents a single pattern element
type patternItem struct {
	itemType   patternItemType
	quantifier quantifierType
	literal    byte           // for itemLiteral
	classChar  byte           // for itemClass (the char after %)
	setChars   []byte         // for itemSet
	setNegated bool           // for itemSet (true if [^...])
	classFunc  func(byte) bool // for itemClass
}

// matchResult holds the result of a pattern match
type matchResult struct {
	matched  bool
	start    int
	end      int
	captures []capture
}

// capture holds a captured substring or position
type capture struct {
	start, end int // positions in string
}

// Character class functions
func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isControl(c byte) bool {
	return c < 32 || c == 127
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isLower(c byte) bool {
	return c >= 'a' && c <= 'z'
}

func isPunct(c byte) bool {
	return unicode.IsPunct(rune(c))
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

func isUpper(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

func isAlnum(c byte) bool {
	return isAlpha(c) || isDigit(c)
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// getClassFunc returns the function for a character class
func getClassFunc(classChar byte) func(byte) bool {
	switch classChar {
	case 'a', 'A':
		return isAlpha
	case 'c', 'C':
		return isControl
	case 'd', 'D':
		return isDigit
	case 'l', 'L':
		return isLower
	case 'p', 'P':
		return isPunct
	case 's', 'S':
		return isSpace
	case 'u', 'U':
		return isUpper
	case 'w', 'W':
		return isAlnum
	case 'x', 'X':
		return isHex
	case 'z', 'Z':
		return func(c byte) bool { return c == 0 }
	default:
		// For unknown classes, match the literal character
		return func(c byte) bool { return c == classChar }
	}
}

// parsePattern parses a Lua pattern into a list of pattern items
func parsePattern(pattern string) []patternItem {
	items := make([]patternItem, 0, len(pattern))
	i := 0

	for i < len(pattern) {
		item := patternItem{itemType: itemLiteral, quantifier: quantOne}

		switch pattern[i] {
		case '^':
			if i == 0 {
				item.itemType = itemAnchorStart
			} else {
				item.literal = '^'
			}
			i++

		case '$':
			if i == len(pattern)-1 {
				item.itemType = itemAnchorEnd
				i++
			} else {
				item.literal = '$'
				i++
			}

		case '.':
			item.itemType = itemAny
			i++

		case '%':
			if i+1 >= len(pattern) {
				// % at end is literal %
				item.literal = '%'
				i++
				break
			}
			next := pattern[i+1]
			item.itemType = itemClass
			item.classChar = next
			item.classFunc = getClassFunc(next)
			i += 2

		case '[':
			// Character set
			item.itemType = itemSet
			i++
			if i < len(pattern) && pattern[i] == '^' {
				item.setNegated = true
				i++
			}
			item.setChars = parseCharSet(pattern, &i)

		case '(':
			if i+1 < len(pattern) && pattern[i+1] == ')' {
				item.itemType = itemCapture
				i += 2
			} else {
				item.itemType = itemGroupStart
				i++
			}

		case ')':
			item.itemType = itemGroupEnd
			i++

		default:
			item.literal = pattern[i]
			i++
		}

		// Check for quantifier
		if i < len(pattern) && item.itemType != itemAnchorStart && item.itemType != itemAnchorEnd {
			switch pattern[i] {
			case '*':
				item.quantifier = quantZeroPlus
				i++
			case '+':
				item.quantifier = quantOnePlus
				i++
			case '-':
				item.quantifier = quantZeroMinus
				i++
			case '?':
				item.quantifier = quantOptional
				i++
			}
		}

		items = append(items, item)
	}

	return items
}

// parseCharSet parses a character set [...] or [^...]
func parseCharSet(pattern string, i *int) []byte {
	chars := make([]byte, 0, 16)

	// Handle ] as first char (or after ^)
	if *i < len(pattern) && pattern[*i] == ']' {
		chars = append(chars, ']')
		*i++
	}

	for *i < len(pattern) && pattern[*i] != ']' {
		if pattern[*i] == '%' && *i+1 < len(pattern) {
			// %x in set
			classChar := pattern[*i+1]
			classFunc := getClassFunc(classChar)
			// Add all chars matching the class
			for c := byte(0); c < 128; c++ {
				if classFunc(c) {
					chars = append(chars, c)
				}
			}
			*i += 2
		} else if *i+2 < len(pattern) && pattern[*i+1] == '-' && pattern[*i+2] != ']' {
			// Range a-z
			startChar := pattern[*i]
			endChar := pattern[*i+2]
			for c := startChar; c <= endChar; c++ {
				chars = append(chars, c)
			}
			*i += 3
		} else {
			chars = append(chars, pattern[*i])
			*i++
		}
	}

	// Skip closing ]
	if *i < len(pattern) && pattern[*i] == ']' {
		*i++
	}

	return chars
}

// matchSingleChar checks if a single character matches a pattern item
func matchSingleChar(c byte, item patternItem) bool {
	switch item.itemType {
	case itemLiteral:
		return c == item.literal
	case itemClass:
		if item.classFunc == nil {
			// Fallback: use classChar to get the function
			item.classFunc = getClassFunc(item.classChar)
		}
		if item.classChar >= 'A' && item.classChar <= 'Z' {
			// Uppercase classes are negated
			baseFunc := getClassFunc(item.classChar + 32) // lowercase version
			return !baseFunc(c)
		}
		return item.classFunc(c)
	case itemAny:
		return true
	case itemSet:
		matches := byteInSet(c, item.setChars)
		if item.setNegated {
			return !matches
		}
		return matches
	default:
		return false
	}
}

// byteInSet checks if a byte is in a character set
func byteInSet(c byte, set []byte) bool {
	for _, b := range set {
		if c == b {
			return true
		}
	}
	return false
}

// matchPattern attempts to match a pattern against a string starting at startPos
// Returns: matched (bool), start, end, captures
func matchPattern(str string, startPos int, pattern []patternItem, anchored bool) matchResult {
	// Try matching at each position (unless anchored)
	for pos := startPos; pos <= len(str); pos++ {
		result := matchAtPosition(str, pos, pattern, 0)
		if result.matched {
			result.start = pos
			return result
		}
		if anchored {
			break
		}
	}

	return matchResult{matched: false}
}

// matchAtPosition recursively matches pattern starting at given string position and pattern index
func matchAtPosition(str string, strPos int, pattern []patternItem, patIdx int) matchResult {
	// Base case: all pattern items matched
	if patIdx >= len(pattern) {
		return matchResult{matched: true, end: strPos, captures: make([]capture, 0)}
	}

	item := pattern[patIdx]

	// Handle anchor start
	if item.itemType == itemAnchorStart {
		if strPos != 0 {
			return matchResult{matched: false}
		}
		return matchAtPosition(str, strPos, pattern, patIdx+1)
	}

	// Handle anchor end
	if item.itemType == itemAnchorEnd {
		if strPos != len(str) {
			return matchResult{matched: false}
		}
		return matchAtPosition(str, strPos, pattern, patIdx+1)
	}

	// Handle position capture ()
	if item.itemType == itemCapture {
		result := matchAtPosition(str, strPos, pattern, patIdx+1)
		if result.matched {
			result.captures = append([]capture{{start: strPos + 1, end: strPos}}, result.captures...)
		}
		return result
	}

	// Handle group start
	if item.itemType == itemGroupStart {
		// Find matching group end
		depth := 1
		endIdx := patIdx + 1
		for endIdx < len(pattern) {
			if pattern[endIdx].itemType == itemGroupStart {
				depth++
			} else if pattern[endIdx].itemType == itemGroupEnd {
				depth--
				if depth == 0 {
					break
				}
			}
			endIdx++
		}

		// Try to match the group content
		groupStart := strPos
		result := matchAtPosition(str, strPos, pattern, patIdx+1)
		if result.matched {
			// Add group capture
			result.captures = append([]capture{{start: groupStart, end: result.end}}, result.captures...)
			return result
		}
		return matchResult{matched: false}
	}

	// Handle group end
	if item.itemType == itemGroupEnd {
		return matchAtPosition(str, strPos, pattern, patIdx+1)
	}

	// Handle quantifiers
	switch item.quantifier {
	case quantOne:
		// Match exactly one
		if strPos >= len(str) {
			return matchResult{matched: false}
		}
		if !matchSingleChar(str[strPos], item) {
			return matchResult{matched: false}
		}
		return matchAtPosition(str, strPos+1, pattern, patIdx+1)

	case quantOptional:
		// Match zero or one
		// Try one first
		if strPos < len(str) && matchSingleChar(str[strPos], item) {
			result := matchAtPosition(str, strPos+1, pattern, patIdx+1)
			if result.matched {
				return result
			}
		}
		// Try zero
		return matchAtPosition(str, strPos, pattern, patIdx+1)

	case quantZeroPlus:
		// Match zero or more (greedy)
		return matchGreedy(str, strPos, pattern, patIdx, item)

	case quantOnePlus:
		// Match one or more (greedy)
		if strPos >= len(str) {
			return matchResult{matched: false}
		}
		if !matchSingleChar(str[strPos], item) {
			return matchResult{matched: false}
		}
		// Now match zero or more
		return matchGreedy(str, strPos+1, pattern, patIdx, item)

	case quantZeroMinus:
		// Match zero or more (non-greedy)
		return matchNonGreedy(str, strPos, pattern, patIdx, item)
	}

	return matchResult{matched: false}
}

// matchGreedy matches * quantifier (greedy)
func matchGreedy(str string, strPos int, pattern []patternItem, patIdx int, item patternItem) matchResult {
	// Collect all possible end positions
	positions := []int{strPos}
	pos := strPos
	for pos < len(str) {
		if !matchSingleChar(str[pos], item) {
			break
		}
		pos++
		positions = append(positions, pos)
	}

	// Try from longest to shortest (greedy)
	for i := len(positions) - 1; i >= 0; i-- {
		result := matchAtPosition(str, positions[i], pattern, patIdx+1)
		if result.matched {
			return result
		}
	}

	return matchResult{matched: false}
}

// matchNonGreedy matches - quantifier (non-greedy)
func matchNonGreedy(str string, strPos int, pattern []patternItem, patIdx int, item patternItem) matchResult {
	// Try from shortest to longest (non-greedy)
	pos := strPos
	for {
		// Try matching rest of pattern at current position
		result := matchAtPosition(str, pos, pattern, patIdx+1)
		if result.matched {
			return result
		}

		// Try to consume one more character
		if pos >= len(str) {
			break
		}
		if !matchSingleChar(str[pos], item) {
			break
		}
		pos++
	}

	return matchResult{matched: false}
}

// ============================================================================
// End of Pattern Matching Implementation
// ============================================================================

// stdStringFind implements string.find(s, pattern [, init [, plain]])
// Returns start and end indices of first match, or nil if not found.
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

	// Get plain flag
	plain := false
	if L.GetTop() >= 4 {
		plain = L.IsTruthy(4)
	}

	// Plain mode: simple string search
	if plain {
		if init > len(str)+1 {
			L.PushNil()
			return 1
		}
		if init > len(str) {
			if pattern == "" {
				L.PushInteger(int64(init))
				L.PushInteger(int64(init - 1))
				return 2
			}
			L.PushNil()
			return 1
		}

		idx := strings.Index(str[init-1:], pattern)
		if idx == -1 {
			L.PushNil()
			return 1
		}

		start := init + idx
		end := start + len(pattern) - 1
		L.PushInteger(int64(start))
		L.PushInteger(int64(end))
		return 2
	}

	// Pattern matching mode
	if init > len(str)+1 {
		L.PushNil()
		return 1
	}

	// Parse pattern
	items := parsePattern(pattern)
	if len(items) == 0 {
		// Empty pattern matches at init
		L.PushInteger(int64(init))
		L.PushInteger(int64(init - 1))
		return 2
	}

	// Check for anchor
	anchored := len(items) > 0 && items[0].itemType == itemAnchorStart

	// Try to match
	result := matchPattern(str, init-1, items, anchored)
	if !result.matched {
		L.PushNil()
		return 1
	}

	// Return start and end positions (1-based)
	L.PushInteger(int64(result.start + 1))
	L.PushInteger(int64(result.end))

	// Return captures (if any)
	for _, c := range result.captures {
		if c.start >= 0 && c.end >= 0 && c.start <= c.end {
			L.PushString(str[c.start:c.end])
		} else {
			L.PushInteger(int64(c.start))
		}
	}

	return 2 + len(result.captures)
}

// stdStringFormat implements string.format(formatstring, ...)
// Returns formatted string (sprintf-like).
// stdStringFormat implements string.format(formatstring, ...)
// Returns formatted string (sprintf-like).
//
// CONTRACT:
//   - Format spec: %[flags][width][.precision]conversion
//   - Conversion chars: %, p, s, q, d, i, f, c, x, X, o, u, a, A, e, E, g, G
//   - %% NEVER consumes an argument (invariant: argIdx only incremented for non-%% conversions)
//   - %q uses Lua escaping (\0 not \x00, \n not literal newline)
//   - %p: pointer types (table/function/thread/userdata) → GC address
//         strings ≤40 bytes → cached stable pointer (interned)
//         strings >40 bytes → unique pointer (not interned)
//         non-pointers → "(null)"
//   - %f default precision: 6 decimal places (matches C printf)
//   - Bare specs (%s) and modified specs (%10s) both supported
//
// PARSER INVARIANT:
//   - Outer `for i` loop owns `i`
//   - Inner scan advances `i` past flags/width/precision to the conversion char
//   - After inner scan, `i` points AT the conversion character
//   - Outer loop's `i++` is suppressed — inner scan already positioned correctly
//   - This prevents off-by-one bugs in format strings with multiple specs
//
// WHY NOT use fmt.Sprintf directly for %p?
//   - Go's %p expects a pointer, but we format pointer ADDRESS as a string
//   - Lua's %p has special semantics for short strings (interned) vs long strings
//   - We need manual control over the pointer representation
func stdStringFormat(L *State) int {
	format, ok := L.ToString(1)
	if !ok {
		L.PushString("")
		return 1
	}

	top := L.GetTop()
	argIdx := 2
	
	var result strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] == '%' && i+1 < len(format) {
			// Scan format spec: %[flags][width][.precision]conversion
			i++ // skip '%'
			start := i
			
			// Skip flags: - + # 0 space
			for i < len(format) && (format[i] == '-' || format[i] == '+' || format[i] == '#' || format[i] == '0' || format[i] == ' ') {
				i++
			}
			
			// Skip width (digits)
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
			
			// Skip precision (.digits)
			if i < len(format) && format[i] == '.' {
				i++
				for i < len(format) && format[i] >= '0' && format[i] <= '9' {
					i++
				}
			}
			
			// Now format[i] should be the conversion character
			if i >= len(format) {
				result.WriteByte('%')
				break
			}
			
			conversion := format[i]
			formatSpec := format[start:i] // everything between % and conversion (flags, width, precision)
			
			// Handle based on conversion type
			switch conversion {
			case '%':
				result.WriteByte('%')
			case 'p':
				// Get the argument value
				if argIdx <= top {
					v := L.vm.GetStack(argIdx)
					argIdx++
					ptrStr := getPointerString(*v)
					if formatSpec == "" {
						result.WriteString(ptrStr)
					} else {
						result.WriteString(applyFormatSpec(formatSpec, ptrStr))
					}
				} else {
					ptrStr := "(null)"
					if formatSpec == "" {
						result.WriteString(ptrStr)
					} else {
						result.WriteString(applyFormatSpec(formatSpec, ptrStr))
					}
				}
			case 's':
				if argIdx <= top {
					v := L.vm.GetStack(argIdx)
					argIdx++
					// %s should convert any value to string (Lua 5.4 behavior)
					str := object.ToStringRaw(v)
					if formatSpec == "" {
						result.WriteString(str)
					} else {
						// Lua 5.4+ raises error when using width/precision with strings containing null bytes
						if strings.Contains(str, "\x00") {
							L.PushString("bad argument #2 to 'format' (string contains zeros)")
							L.Error()
							return 0
						}
						result.WriteString(fmt.Sprintf("%"+formatSpec+"s", str))
					}
				}
			case 'q':
				if argIdx <= top {
					v := L.vm.GetStack(argIdx)
					argIdx++
					
					var quoted string
					switch {
					case v.IsNil():
						quoted = "nil"
					case v.IsBoolean():
						if v.Value.Bool {
							quoted = "true"
						} else {
							quoted = "false"
						}
					case v.IsNumber():
						num := v.Value.Num
						if math.IsNaN(num) {
							quoted = "(0/0)"
						} else if math.IsInf(num, 1) {
							quoted = "math.huge"
						} else if math.IsInf(num, -1) {
							quoted = "-math.huge"
						} else {
							// Format number as literal
							if v.IsInt {
								// Integer value - format as integer literal
								intVal := v.Value.Int
								// Special case: math.mininteger (-2^63) cannot be formatted
								// as a decimal literal because the lexer parses '-' as unary
								// minus, then the positive value overflows int64.
								// Use math.tointeger with hex format as workaround.
								if intVal == math.MinInt64 {
									quoted = "math.tointeger(-0x8000000000000000)"
								} else {
									quoted = strconv.FormatInt(intVal, 10)
								}
							} else {
								// Float value
								quoted = strconv.FormatFloat(num, 'g', -1, 64)
							}
						}
					case v.IsString():
						str := v.Value.Str
						quoted = luaQuote(str)
					default:
						// For other types (table, function, etc.), raise error
						L.PushString(fmt.Sprintf("bad argument #%d to 'format' (no literal for %s)", argIdx, v.Type))
						L.Error()
						return 0
					}
					
					if formatSpec == "" {
						result.WriteString(quoted)
					} else {
						result.WriteString(fmt.Sprintf("%"+formatSpec+"s", quoted))
					}
				}
			case 'd', 'i':
				if argIdx <= top {
					v := L.vm.GetStack(argIdx)
					argIdx++
					if num, ok := v.ToNumber(); ok {
						if formatSpec == "" {
							result.WriteString(strconv.Itoa(int(num)))
						} else {
							result.WriteString(fmt.Sprintf("%"+formatSpec+"d", int(num)))
						}
					}
				}
			case 'f':
				if argIdx <= top {
					v := L.vm.GetStack(argIdx)
					argIdx++
					if num, ok := v.ToNumber(); ok {
						if formatSpec == "" {
							// Default %f uses 6 decimal places (like C)
							result.WriteString(strconv.FormatFloat(num, 'f', 6, 64))
						} else {
							result.WriteString(fmt.Sprintf("%"+formatSpec+"f", num))
						}
					}
				}
			case 'c':
				if argIdx <= top {
					v := L.vm.GetStack(argIdx)
					argIdx++
					if num, ok := v.ToNumber(); ok {
						if formatSpec == "" {
							result.WriteByte(byte(int(num)))
						} else {
							result.WriteString(fmt.Sprintf("%"+formatSpec+"c", byte(int(num))))
						}
					}
				}
			case 'x', 'X':
				if argIdx <= top {
					v := L.vm.GetStack(argIdx)
					argIdx++
					if num, ok := v.ToNumber(); ok {
						if formatSpec == "" {
							if conversion == 'x' {
								result.WriteString(strconv.FormatInt(int64(num), 16))
							} else {
								result.WriteString(strings.ToUpper(strconv.FormatInt(int64(num), 16)))
							}
						} else {
							result.WriteString(fmt.Sprintf("%"+formatSpec+string(conversion), int(num)))
						}
					}
				}
			case 'o':
				if argIdx <= top {
					v := L.vm.GetStack(argIdx)
					argIdx++
					if num, ok := v.ToNumber(); ok {
						if formatSpec == "" {
							result.WriteString(strconv.FormatInt(int64(num), 8))
						} else {
							result.WriteString(fmt.Sprintf("%"+formatSpec+"o", int(num)))
						}
					}
				}
			case 'u':
				if argIdx <= top {
					v := L.vm.GetStack(argIdx)
					argIdx++
					if num, ok := v.ToNumber(); ok {
						if formatSpec == "" {
							result.WriteString(strconv.FormatUint(uint64(num), 10))
						} else {
							result.WriteString(fmt.Sprintf("%"+formatSpec+"d", int(num)))
						}
					}
				}
			case 'a', 'A':
				if argIdx <= top {
					v := L.vm.GetStack(argIdx)
					argIdx++
					if num, ok := v.ToNumber(); ok {
						if formatSpec == "" {
							if conversion == 'a' {
								result.WriteString(strconv.FormatFloat(num, 'x', -1, 64))
							} else {
								result.WriteString(strings.ToUpper(strconv.FormatFloat(num, 'x', -1, 64)))
							}
						} else {
							result.WriteString(fmt.Sprintf("%"+formatSpec+string(conversion), num))
						}
					}
				}
			case 'e', 'E', 'g', 'G':
				if argIdx <= top {
					v := L.vm.GetStack(argIdx)
					argIdx++
					if num, ok := v.ToNumber(); ok {
						if formatSpec == "" {
							result.WriteString(strconv.FormatFloat(num, byte(conversion), -1, 64))
						} else {
							result.WriteString(fmt.Sprintf("%"+formatSpec+string(conversion), num))
						}
					}
				}
			default:
				// Unknown conversion, output as-is
				result.WriteByte('%')
				result.WriteString(formatSpec)
				result.WriteByte(conversion)
			}
		} else {
			result.WriteByte(format[i])
		}
	}

	L.PushString(result.String())
	return 1
}

// applyFormatSpec applies width and justification to a string
// formatSpec contains flags and width (e.g., "90", "-60", "08")
func applyFormatSpec(formatSpec, value string) string {
	if formatSpec == "" {
		return value
	}
	
	// Parse format spec
	leftJustify := false
	width := 0
	
	i := 0
	// Check for '-' flag (left justify)
	if i < len(formatSpec) && formatSpec[i] == '-' {
		leftJustify = true
		i++
	}
	
	// Skip other flags (+, #, 0, space) - not relevant for strings
	for i < len(formatSpec) && (formatSpec[i] == '+' || formatSpec[i] == '#' || formatSpec[i] == '0' || formatSpec[i] == ' ') {
		i++
	}
	
	// Parse width
	for i < len(formatSpec) && formatSpec[i] >= '0' && formatSpec[i] <= '9' {
		width = width*10 + int(formatSpec[i]-'0')
		i++
	}
	
	if width <= len(value) {
		return value
	}
	
	padding := strings.Repeat(" ", width-len(value))
	if leftJustify {
		return value + padding
	}
	return padding + value
}

// getPointerString returns the pointer representation for a value
// For pointer types (table, function, thread, userdata, string): returns hex address
// For non-pointer types: returns "(null)"
func getPointerString(v object.TValue) string {
	if v.IsTable() || v.IsFunction() || v.IsThread() || v.IsUserData() {
		return fmt.Sprintf("%p", v.Value.GC)
	} else if v.IsString() {
		// Strings: Lua 5.4 semantics
		// Short strings (≤40 bytes) are interned - use cached stable pointer
		// Long strings (>40 bytes) are not interned - use unique pointer
		str, _ := v.ToString()
		if len(str) <= 40 {
			// Short string: use cached stable pointer
			if ptr, ok := shortStringPtrs.Load(str); ok {
				return fmt.Sprintf("0x%x", ptr.(uintptr))
			}
			// Create new stable pointer for this short string
			ptrVal := uintptr(unsafe.Pointer(unsafe.StringData(str)))
			shortStringPtrs.Store(str, ptrVal)
			return fmt.Sprintf("0x%x", ptrVal)
		}
		// Long string: use unique pointer (not interned)
		return fmt.Sprintf("%p", unsafe.StringData(str))
	}
	return "(null)"
}

// luaQuote returns a Lua-quoted string literal.
// Uses Lua escaping conventions: \0, \n, \t, \\, \" etc.
// WHY NOT fmt.Sprintf("%q")? Go uses \x00 for null, Lua uses \0.
func luaQuote(s string) string {
	var result strings.Builder
	result.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\a':
			result.WriteString("\\a")
		case '\b':
			result.WriteString("\\b")
		case '\f':
			result.WriteString("\\f")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		case '\v':
			result.WriteString("\\v")
		case '\\':
			result.WriteString("\\\\")
		case '"':
			result.WriteString("\\\"")
		case 0:
			// Lua uses \0 for null, not \x00
			// Check if next char is a digit - need to use \000 form
			if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
				result.WriteString("\\000")
			} else {
				result.WriteString("\\0")
			}
		default:
			if c >= 32 {
				// Printable ASCII and non-ASCII bytes (preserve as-is)
				result.WriteByte(c)
			} else {
				// Non-printable: use \ddd decimal escape
				result.WriteString(fmt.Sprintf("\\%03d", c))
			}
		}
	}
	result.WriteByte('"')
	return result.String()
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

	var n int
	if num, ok := L.ToNumber(2); ok {
		// Check if the number is too large for int
		if num > float64(1<<31-1) || num < 0 {
			L.PushString("bad argument #2 to 'rep' (resulting string too large)")
			L.Error()
			return 0
		}
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

	// Check for overflow: resulting string would be too large
	// Max reasonable size is around 1GB
	strLen := len(str)
	sepLen := len(sep)
	// Total size = n * strLen + (n-1) * sepLen
	// Check if this would overflow or be too large
	totalSize := int64(n) * int64(strLen)
	if sepLen > 0 && n > 1 {
		totalSize += int64(n-1) * int64(sepLen)
	}
	
	// Limit to ~1GB to prevent memory exhaustion
	maxSize := int64(1 << 30)
	if totalSize > maxSize || totalSize < 0 {
		L.PushString("bad argument #2 to 'rep' (resulting string too large)")
		L.Error()
		return 0
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

	bytes := []byte(str)
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}
	L.PushString(string(bytes))
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
	n := len(str)
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
			L.PushInteger(int64(str[k-1]))
			count++
		}
	}
	return count
}

// stdStringChar implements string.char(...)
// Returns string with given character codes.
func stdStringChar(L *State) int {
	top := L.GetTop()
	bytes := make([]byte, 0, top)

	for i := 1; i <= top; i++ {
		num, ok := L.ToNumber(i)
		if !ok {
			L.PushString(fmt.Sprintf("bad argument #%d to 'char' (number expected, got %s)", i, L.TypeName(i)))
			L.Error()
			return 0
		}

		// Validate range [0, 255]
		if num < 0 || num > 255 {
			L.PushString(fmt.Sprintf("bad argument #%d to 'char' (value out of range)", i))
			L.Error()
			return 0
		}

		bytes = append(bytes, byte(int(num)))
	}

	L.PushString(string(bytes))
	return 1
}

// stdStringGsub implements string.gsub(s, pattern, repl [, n])
// Returns copy of s with occurrences replaced, plus count.
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

	// Parse pattern
	items := parsePattern(pattern)

	// Perform replacements
	var result strings.Builder
	pos := 0
	count := 0

	for pos <= len(str) {
		if maxRepl >= 0 && count >= maxRepl {
			break
		}

		// Try to match at current position only (anchored search)
		matchRes := matchAtPosition(str, pos, items, 0)
		if matchRes.matched && matchRes.end > pos {
			// Replace
			result.WriteString(repl)
			pos = matchRes.end
			count++
		} else {
			// Copy character and advance
			if pos < len(str) {
				result.WriteByte(str[pos])
				pos++
			} else {
				break
			}
		}
	}

	// Copy remaining
	if pos < len(str) {
		result.WriteString(str[pos:])
	}

	L.PushString(result.String())
	L.PushInteger(int64(count))
	return 2
}

// stdStringMatch implements string.match(s, pattern [, init])
// Returns first match of pattern in s.
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

	if init > len(str) {
		L.PushNil()
		return 1
	}

	// Parse pattern
	items := parsePattern(pattern)
	anchored := len(items) > 0 && items[0].itemType == itemAnchorStart

	// Try to match
	result := matchPattern(str, init-1, items, anchored)
	if !result.matched {
		L.PushNil()
		return 1
	}

	// Return captures or full match
	if len(result.captures) > 0 {
		// Return captures
		for _, c := range result.captures {
			if c.start >= 0 && c.end <= len(str) {
				L.PushString(str[c.start:c.end])
			} else {
				L.PushInteger(int64(c.start))
			}
		}
		return len(result.captures)
	}

	// Return full match
	if result.start >= 0 && result.end <= len(str) {
		L.PushString(str[result.start:result.end])
	} else {
		L.PushNil()
	}
	return 1
}

// stdStringGmatch implements string.gmatch(s, pattern)
// Returns an iterator function for matches.
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

	// Parse pattern once
	items := parsePattern(pattern)

	// Create iterator with captured position
	pos := 0

	L.PushFunction(func(L *State) int {
		for pos <= len(str) {
			// Try to match at current position only (anchored)
			result := matchAtPosition(str, pos, items, 0)
			if result.matched && result.end > pos {
				// Store match start/end before updating pos
				matchStart := pos
				matchEnd := result.end

				// Update position for next iteration
				pos = result.end
				if pos == matchStart && pos < len(str) {
					pos++
				}

				// Return captures or full match
				if len(result.captures) > 0 {
					for _, c := range result.captures {
						if c.start >= 0 && c.end <= len(str) {
							L.PushString(str[c.start:c.end])
						} else {
							L.PushInteger(int64(c.start))
						}
					}
					return len(result.captures)
				}

				// Return full match
				if matchEnd <= len(str) {
					L.PushString(str[matchStart:matchEnd])
					return 1
				}
				return 0
			}

			// Move to next position
			pos++
		}

		return 0
	})

	return 1
}