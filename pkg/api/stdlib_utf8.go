// Package api provides the public Lua API
// This file implements the utf8 standard library module
package api

import (
	"unicode/utf8"

	"github.com/akzj/go-lua/pkg/object"
)

// openUtf8Lib registers the utf8 module
func (s *State) openUtf8Lib() {
	// Create module table with all functions
	funcs := map[string]Function{
		"len":       stdUtf8Len,
		"char":      stdUtf8Char,
		"codes":     stdUtf8Codes,
		"offset":    stdUtf8Offset,
		"codepoint": stdUtf8Codepoint,
	}

	s.RegisterModule("utf8", funcs)

	// Add charpattern as a constant string
	// This is the official Lua 5.3 pattern for UTF-8:
	// - Single byte 0x00-0x7F (ASCII)
	// - OR leading byte 0xC2-0xFD followed by continuation bytes 0x80-0xBF
	globalTable := s.getGlobalTable()
	key := object.TValue{Type: object.TypeString, Value: object.Value{Str: "utf8"}}
	utf8Module := globalTable.Get(key)
	if utf8Module != nil && utf8Module.IsTable() {
		utf8ModuleTable, _ := utf8Module.ToTable()
		patternValue := object.NewString("[\x00-\x7F\xC2-\xFD][\x80-\xBF]*")
		patternKey := object.TValue{Type: object.TypeString, Value: object.Value{Str: "charpattern"}}
		utf8ModuleTable.Set(patternKey, *patternValue)
	}
}

// stdUtf8Len implements utf8.len(s [, i [, j [, lax]]])
// Returns the number of UTF-8 characters in string s between byte positions i and j.
// Returns nil and an error position if s contains invalid UTF-8.
// lax=true allows codepoints up to 0x7FFFFFFF.
func stdUtf8Len(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	// Default range: 1 to len(s)
	i := 1
	j := -1

	if L.GetTop() >= 2 {
		if num, ok := L.ToNumber(2); ok {
			i = int(num)
		}
	}
	if L.GetTop() >= 3 {
		if num, ok := L.ToNumber(3); ok {
			j = int(num)
		}
	}

	// lax mode allows codepoints up to 0x7FFFFFFF
	lax := false
	if L.GetTop() >= 4 {
		lax = L.IsTruthy(4)
	}

	n := len(str)

	// Normalize negative indices
	if i < 0 {
		i = n + i + 1
	}
	if j < 0 {
		j = n + j + 1
	}

	// Validate i (must be in range [1, n+1])
	// i=0 or negative after normalization should error
	if i < 1 || i > n+1 {
		L.PushString("out of bounds")
		L.Error()
		return 0
	}

	// Validate j (must be in range [1, n])
	if j < 1 || j > n {
		L.PushString("out of bounds")
		L.Error()
		return 0
	}

	// Count UTF-8 characters
	count := 0
	pos := i - 1
	_ = j
	for pos < j {
		if pos >= n {
			break
		}
		r, size := utf8.DecodeRuneInString(str[pos:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8
			if lax {
				// In lax mode, accept continuation bytes as individual bytes
				pos++
				count++
			} else {
				L.PushNil()
				L.PushInteger(int64(pos + 1))
				return 2
			}
		} else if r >= 0xD800 && r <= 0xDFFF && !lax {
			// Surrogate pair range - invalid in strict UTF-8
			L.PushNil()
			L.PushInteger(int64(pos + 1))
			return 2
		} else if r > 0x10FFFF && !lax {
			// Codepoint out of valid range
			L.PushNil()
			L.PushInteger(int64(pos + 1))
			return 2
		} else {
			pos += size
			count++
		}
	}

	L.PushInteger(int64(count))
	return 1
}

// stdUtf8Char implements utf8.char(...)
// Returns a string with the given codepoints as UTF-8 bytes.
func stdUtf8Char(L *State) int {
	top := L.GetTop()
	runes := make([]rune, 0, top)

	for i := 1; i <= top; i++ {
		v := L.vm.GetStack(i)
		var num int64
		if v.IsInt {
			num = v.Value.Int
		} else if v.IsNumber() {
			num = int64(v.Value.Num)
		} else {
			L.PushString("bad argument #" + itoa(i) + " to 'char' (number expected)")
			L.Error()
			return 0
		}

		// Validate range: 0 to 0x10FFFF (or 0x7FFFFFFF in lax mode)
		if num < 0 || num > 0x10FFFF {
			L.PushString("value out of range")
			L.Error()
			return 0
		}

		runes = append(runes, rune(num))
	}

	L.PushString(string(runes))
	return 1
}

// stdUtf8Offset implements utf8.offset(s, n [, i])
// Returns the byte position of the n-th UTF-8 character in string s.
// n can be negative to count from the end.
// Returns nil if n is out of bounds.
func stdUtf8Offset(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	var n int
	if num, ok := L.ToNumber(2); ok {
		n = int(num)
	}

	nStr := len(str)

	// Default start position depends on n (matching reference: posi = (n >= 0) ? 1 : len + 1)
	var startPos int
	if n >= 0 {
		startPos = 1
	} else {
		startPos = nStr + 1
	}
	if L.GetTop() >= 3 {
		if num, ok := L.ToNumber(3); ok {
			startPos = int(num)
		}
	}

	// Normalize negative startPos
	if startPos < 0 {
		startPos = nStr + startPos + 1
	}

	// Clamp to valid range [1, nStr]
	if startPos < 1 {
		L.PushString("position out of bounds")
		L.Error()
		return 0
	}
	if startPos > nStr+1 {
		L.PushString("position out of bounds")
		L.Error()
		return 0
	}

	// Convert to 0-based index
	pos := startPos - 1

	// Handle n == 0: return the start and end byte of the character at position pos+1
	if n == 0 {
		// If start position is a continuation byte, find the start of the character
		// (unlike n>0 which errors on continuation bytes at start position)
		for pos > 0 && (str[pos]&0xC0) == 0x80 {
			pos--
		}
		// Check if we're at a valid start byte
		if pos < 0 || pos >= nStr || (str[pos]&0xC0) == 0x80 {
			L.PushNil()
			return 1
		}
		// Decode the character
		charStart := pos + 1 // 1-based
		c := str[pos]
		// Find the end position (last byte of this character)
		charEnd := charStart
		if (c & 0x80) != 0 {
			// Multi-byte character, skip continuation bytes
			for i := pos + 1; i < nStr && (str[i]&0xC0) == 0x80; i++ {
				charEnd = i + 1
			}
		}
		L.PushInteger(int64(charStart))
		L.PushInteger(int64(charEnd))
		return 2
	}

	// Find the n-th character from current position
	if n > 0 {
		// Check if start position is a continuation byte - should error
		if pos < nStr && (str[pos]&0xC0) == 0x80 {
			L.PushString("continuation byte")
			L.Error()
			return 0
		}
		// Skip n-1 characters forward
		for i := 1; i < n; i++ {
			// Skip continuation bytes to find next character start
			for pos < nStr && (str[pos]&0xC0) == 0x80 {
				pos++
			}
			if pos >= nStr {
				// No more characters, return nil
				L.PushNil()
				return 1
			}
			// Move to next character
			r, size := utf8.DecodeRuneInString(str[pos:])
			if r == utf8.RuneError && size == 1 {
				L.PushString("continuation byte")
				L.Error()
				return 0
			}
			pos += size
		}
		// Now at the start of the n-th character
		// Check if we're at end of string - return one past the end (Lua 5.3 behavior)
		if pos >= nStr {
			L.PushInteger(int64(nStr + 1))
			return 1
		}
		// Check if we're at a continuation byte
		if (str[pos]&0xC0) == 0x80 {
			L.PushNil()
			return 1
		}
		charStart := pos + 1 // 1-based
		L.PushInteger(int64(charStart))
		return 1
	} else {
		// n < 0: count backwards from startPos
		// For n=-1, find character at startPos; for n=-2, find character before startPos, etc.
		targetFromEnd := -n

		// Count characters backwards from startPos
		count := 0
		currentPos := pos

		for count < targetFromEnd {
			// Move back to find start of previous character
			currentPos--
			// If we went past the start, we can't find more characters
			if currentPos < 0 {
				break
			}
			// Skip back over continuation bytes to find character start
			for currentPos > 0 && (str[currentPos]&0xC0) == 0x80 {
				currentPos--
			}
			// Now currentPos >= 0, check if we went past start
			if currentPos < 0 {
				break
			}
			count++
		}

		// If we don't have enough characters, return nil
		if count < targetFromEnd {
			L.PushNil()
			return 1
		}

		// If currentPos is out of valid range
		if currentPos < 0 || currentPos >= nStr {
			L.PushNil()
			return 1
		}

		// Check if we're at a valid start byte
		c := str[currentPos]
		if (c & 0xC0) == 0x80 {
			// Lone continuation byte - return its position
			L.PushInteger(int64(currentPos + 1))
			return 1
		}
		L.PushInteger(int64(currentPos + 1))
		return 1
	}
}

// stdUtf8Codepoint implements utf8.codepoint(s [, i [, j [, lax]]])
// Returns codepoints as multiple values.
func stdUtf8Codepoint(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		return 0
	}

	// Default range: 1 to -1 (entire string)
	i := 1
	j := -1

	if L.GetTop() >= 2 {
		if num, ok := L.ToNumber(2); ok {
			i = int(num)
		}
	}
	if L.GetTop() >= 3 {
		if num, ok := L.ToNumber(3); ok {
			j = int(num)
		}
	}

	// lax mode allows codepoints up to 0x7FFFFFFF
	lax := false
	if L.GetTop() >= 4 {
		lax = L.IsTruthy(4)
	}

	n := len(str)

	// Normalize negative indices
	if i < 0 {
		i = n + i + 1
	}
	if j < 0 {
		j = n + j + 1
	}

	// Clamp i to valid range [1, n+1]
	if i < 1 {
		L.PushString("out of bounds")
		L.Error()
		return 0
	}
	if i > n+1 {
		L.PushString("out of bounds")
		L.Error()
		return 0
	}

	// Clamp j to valid range [i, n]
	if j > n {
		L.PushString("out of bounds")
		L.Error()
		return 0
	}
	if j < i {
		return 0
	}

	// Convert to 0-based indices
	start := i - 1
	end := j

	// Extract codepoints
	count := 0
	pos := start
	for pos < end {
		if pos >= n {
			break
		}
		r, size := utf8.DecodeRuneInString(str[pos:])
		if r == utf8.RuneError && size == 1 {
			if lax {
				// In lax mode, treat as individual byte
				L.PushInteger(int64(str[pos]))
				count++
				pos++
			} else {
				L.PushString("invalid UTF-8 code")
				L.Error()
				return 0
			}
		} else if r >= 0xD800 && r <= 0xDFFF && !lax {
			// Surrogate - invalid in strict UTF-8
			L.PushString("invalid UTF-8 code")
			L.Error()
			return 0
		} else if r > 0x10FFFF && !lax {
			L.PushString("invalid UTF-8 code")
			L.Error()
			return 0
		} else {
			L.PushInteger(int64(r))
			count++
			pos += size
		}
	}

	return count
}

// utf8CodesState holds iteration state for utf8.codes
type utf8CodesState struct {
	str  string
	pos  int
	end  int
	lax  bool
}

// utf8CodesIterator is the iterator function for utf8.codes
// Lua generic for calls: iter(state, nil) for subsequent iterations
func utf8CodesIterator(L *State) int {
	// Get state from first argument (position 1)
	stateVal := L.vm.GetStack(1)
	if stateVal == nil || !stateVal.IsTable() {
		L.PushNil()
		return 1
	}

	// Get current position and end from the state table
	posKey := object.TValue{Type: object.TypeString, Value: object.Value{Str: "pos"}}
	endKey := object.TValue{Type: object.TypeString, Value: object.Value{Str: "end"}}
	strKey := object.TValue{Type: object.TypeString, Value: object.Value{Str: "str"}}
	laxKey := object.TValue{Type: object.TypeString, Value: object.Value{Str: "lax"}}

	t, _ := stateVal.ToTable()
	posVal := t.Get(posKey)
	endVal := t.Get(endKey)
	strVal := t.Get(strKey)
	laxVal := t.Get(laxKey)

	if posVal == nil || endVal == nil || strVal == nil {
		L.PushNil()
		return 1
	}

	str, _ := strVal.ToString()
	pos := int(posVal.Value.Num)
	end := int(endVal.Value.Num)
	lax := laxVal != nil && laxVal.IsBoolean() && laxVal.Value.Bool

	if pos > end || pos >= len(str) {
		L.PushNil()
		return 1
	}

	// Decode current UTF-8 character
	r, size := utf8.DecodeRuneInString(str[pos:])
	if r == utf8.RuneError && size == 1 {
		if lax {
			// Return byte position and byte value
			L.PushInteger(int64(pos + 1))
			L.PushInteger(int64(str[pos]))
			// Update position
			pos++
			t.Set(posKey, object.TValue{Type: object.TypeNumber, Value: object.Value{Num: float64(pos)}})
			return 2
		}
		L.PushString("invalid UTF-8 code")
		L.Error()
		return 0
	}

	// Return byte position (1-based) and codepoint
	L.PushInteger(int64(pos + 1))
	L.PushInteger(int64(r))

	// Update position
	pos += size
	t.Set(posKey, object.TValue{Type: object.TypeNumber, Value: object.Value{Num: float64(pos)}})

	return 2
}

// stdUtf8Codes implements utf8.codes(s [, i [, j [, lax]]])
// Returns an iterator function, the string, and initial state.
func stdUtf8Codes(L *State) int {
	str, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	// Default range: 1 to -1 (entire string)
	i := 1
	j := -1

	if L.GetTop() >= 2 {
		if num, ok := L.ToNumber(2); ok {
			i = int(num)
		}
	}
	if L.GetTop() >= 3 {
		if num, ok := L.ToNumber(3); ok {
			j = int(num)
		}
	}

	// lax mode allows codepoints up to 0x7FFFFFFF
	lax := false
	if L.GetTop() >= 4 {
		lax = L.IsTruthy(4)
	}

	n := len(str)

	// Normalize negative indices
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

	// Create state table
	state := object.NewTable()
	state.Set(object.TValue{Type: object.TypeString, Value: object.Value{Str: "str"}},
		object.TValue{Type: object.TypeString, Value: object.Value{Str: str}})
	state.Set(object.TValue{Type: object.TypeString, Value: object.Value{Str: "pos"}},
		object.TValue{Type: object.TypeNumber, Value: object.Value{Num: float64(i - 1)}})
	state.Set(object.TValue{Type: object.TypeString, Value: object.Value{Str: "end"}},
		object.TValue{Type: object.TypeNumber, Value: object.Value{Num: float64(j)}})
	if lax {
		state.Set(object.TValue{Type: object.TypeString, Value: object.Value{Str: "lax"}},
			object.TValue{Type: object.TypeBoolean, Value: object.Value{Bool: true}})
	}

	// Create iterator closure
	L.PushFunction(utf8CodesIterator)

	// Push state table
	stateVal := object.TValue{Type: object.TypeTable, Value: object.Value{GC: state}}
	L.vm.Push(stateVal)

	// Push nil as initial value
	L.PushNil()

	return 3
}

// itoa converts int to string
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}