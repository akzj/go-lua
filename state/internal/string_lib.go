// String library implementation for Lua 5.4/5.5
package internal

import (
	"fmt"
	"math"
	"strings"
	"strconv"
	"encoding/binary"
	"bytes"
	"reflect"
	"unsafe"

	types "github.com/akzj/go-lua/types/api"
	tableapi "github.com/akzj/go-lua/table/api"
)

// Helper: extract string argument from stack
func checkString(stack []types.TValue, base int, argn int, fname string) string {
	idx := base + argn
	if idx >= len(stack) {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (string expected, got no value)", argn, fname))
	}
	v := stack[idx]
	if v == nil || v.IsNil() {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (string expected, got nil)", argn, fname))
	}
	if v.IsString() {
		if s, ok := v.GetValue().(string); ok {
			return s
		}
	}
	// Try number coercion to string (Lua does this)
	if v.IsInteger() {
		return fmt.Sprintf("%d", v.GetInteger())
	}
	if v.IsFloat() {
		return fmt.Sprintf("%g", v.GetFloat())
	}
	luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (string expected, got %s)", argn, fname, luaTypeName(v)))
	return ""
}

// Helper: extract optional integer argument
func optInt(stack []types.TValue, base int, argn int, def types.LuaInteger) types.LuaInteger {
	idx := base + argn
	if idx >= len(stack) {
		return def
	}
	v := stack[idx]
	if v == nil || v.IsNil() {
		return def
	}
	if v.IsInteger() {
		return v.GetInteger()
	}
	if v.IsFloat() {
		return types.LuaInteger(v.GetFloat())
	}
	if v.IsString() {
		if s, ok := v.GetValue().(string); ok {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return types.LuaInteger(n)
			}
		}
	}
	return def
}

// Helper: check integer argument
func checkInt(stack []types.TValue, base int, argn int, fname string) types.LuaInteger {
	idx := base + argn
	if idx >= len(stack) {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected, got no value)", argn, fname))
	}
	v := stack[idx]
	if v == nil || v.IsNil() {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected, got nil)", argn, fname))
	}
	if v.IsInteger() {
		return v.GetInteger()
	}
	if v.IsFloat() {
		return types.LuaInteger(v.GetFloat())
	}
	if v.IsString() {
		if s, ok := v.GetValue().(string); ok {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return types.LuaInteger(n)
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return types.LuaInteger(f)
			}
		}
	}
	luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected)", argn, fname))
	return 0
}

func luaTypeName(v types.TValue) string {
	if v == nil || v.IsNil() {
		return "nil"
	}
	if v.IsBoolean() {
		return "boolean"
	}
	if v.IsNumber() {
		return "number"
	}
	if v.IsString() {
		return "string"
	}
	if v.IsTable() {
		return "table"
	}
	if v.IsFunction() {
		return "function"
	}
	return "userdata"
}

// nArgs returns the number of arguments passed to a Go function.
func nArgs(stack []types.TValue, base int) int {
	return realArgCount(stack, base)
}

// isTruthy checks if a TValue is truthy (not nil and not false).
func isTruthy(v types.TValue) bool {
	if v == nil || v.IsNil() || v.IsFalse() {
		return false
	}
	return true
}

// =============================================================================
// string.len(s)
// =============================================================================
func bstringLen(stack []types.TValue, base int) int {
	s := checkString(stack, base, 1, "len")
	stack[base] = types.NewTValueInteger(types.LuaInteger(len(s)))
	return 1
}

// =============================================================================
// string.byte(s [, i [, j]])
// =============================================================================
func bstringByte(stack []types.TValue, base int) int {
	s := checkString(stack, base, 1, "byte")
	i := int(optInt(stack, base, 2, 1))
	j := int(optInt(stack, base, 3, types.LuaInteger(i)))

	n := len(s)
	// Convert 1-based Lua indices to 0-based, handling negatives
	if i < 0 { i = n + i + 1 }
	if j < 0 { j = n + j + 1 }
	if i < 1 { i = 1 }
	if j > n { j = n }

	if i > j {
		return 0
	}

	count := j - i + 1
	// Ensure we have enough stack space
	for k := 0; k < count; k++ {
		if base+k < len(stack) {
			stack[base+k] = types.NewTValueInteger(types.LuaInteger(s[i-1+k]))
		}
	}
	return count
}

// =============================================================================
// string.char(...)
// =============================================================================
func bstringChar(stack []types.TValue, base int) int {
	n := nArgs(stack, base)
	var buf strings.Builder
	buf.Grow(n)
	for i := 1; i <= n; i++ {
		c := checkInt(stack, base, i, "char")
		if c < 0 || c > 255 {
			luaErrorString(fmt.Sprintf("bad argument #%d to 'char' (value out of range)", i))
		}
		buf.WriteByte(byte(c))
	}
	stack[base] = types.NewTValueString(buf.String())
	return 1
}

// =============================================================================
// string.sub(s, i [, j])
// =============================================================================
func bstringSub(stack []types.TValue, base int) int {
	s := checkString(stack, base, 1, "sub")
	i := int(checkInt(stack, base, 2, "sub"))
	j := int(optInt(stack, base, 3, -1))

	n := len(s)
	if i < 0 { i = n + i + 1 }
	if j < 0 { j = n + j + 1 }
	if i < 1 { i = 1 }
	if j > n { j = n }

	if i > j {
		stack[base] = types.NewTValueString("")
		return 1
	}

	stack[base] = types.NewTValueString(s[i-1 : j])
	return 1
}

// =============================================================================
// string.rep(s, n [, sep])
// =============================================================================
func bstringRep(stack []types.TValue, base int) int {
	s := checkString(stack, base, 1, "rep")
	n := int(checkInt(stack, base, 2, "rep"))
	sep := ""
	if nArgs(stack, base) >= 3 {
		v := stack[base+3]
		if v != nil && v.IsString() {
			sep, _ = v.GetValue().(string)
		}
	}

	if n <= 0 {
		stack[base] = types.NewTValueString("")
		return 1
	}

	// Check for resulting string too large
	sLen := int64(len(s))
	sepLen := int64(len(sep))
	resultLen := sLen * int64(n)
	if sep != "" {
		resultLen += sepLen * int64(n-1)
	}
	if resultLen > 1<<30 || resultLen < 0 {
		luaErrorString("resulting string too large")
	}

	if sep == "" {
		stack[base] = types.NewTValueString(strings.Repeat(s, n))
		return 1
	}

	// With separator
	var buf strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			buf.WriteString(sep)
		}
		buf.WriteString(s)
	}
	stack[base] = types.NewTValueString(buf.String())
	return 1
}

// =============================================================================
// string.reverse(s)
// =============================================================================
func bstringReverse(stack []types.TValue, base int) int {
	s := checkString(stack, base, 1, "reverse")
	stack[base] = types.NewTValueString(luaStringReverse(s))
	return 1
}

// =============================================================================
// string.upper(s)
// =============================================================================
func bstringUpper(stack []types.TValue, base int) int {
	s := checkString(stack, base, 1, "upper")
	stack[base] = types.NewTValueString(luaToUpper(s))
	return 1
}

// =============================================================================
// string.lower(s)
// =============================================================================
func bstringLower(stack []types.TValue, base int) int {
	s := checkString(stack, base, 1, "lower")
	stack[base] = types.NewTValueString(luaToLower(s))
	return 1
}

// =============================================================================
// string.find(s, pattern [, init [, plain]])
// =============================================================================
func bstringFind(stack []types.TValue, base int) int {
	s := checkString(stack, base, 1, "find")
	pattern := checkString(stack, base, 2, "find")
	init := int(optInt(stack, base, 3, 1))
	plain := false
	if nArgs(stack, base) >= 4 {
		v := stack[base+4]
		if v != nil {
			plain = isTruthy(v)
		}
	}

	n := len(s)
	// Adjust init (Lua 5.4 semantics)
	if init < 0 { init = n + init + 1 }
	if init < 1 { init = 1 }

	// Empty pattern: always matches at init position (if within bounds)
	if len(pattern) == 0 {
		// Empty pattern matches at init position if within bounds
		// init <= n means within string, init == n+1 means end-of-string
		if init > n+1 {
			stack[base] = types.NewTValueNil()
			return 1
		}
		// Empty pattern matches at init position; end = init-1
		stack[base] = types.NewTValueInteger(types.LuaInteger(init))
		return 1
	}

	// If init is past the end of string (+1 for empty match at end), fail
	if init > n+1 {
		stack[base] = types.NewTValueNil()
		return 1
	}

	si := init - 1 // convert to 0-based

	if plain {
		// Plain string search
		idx := strings.Index(s[si:], pattern)
		if idx == -1 {
			stack[base] = types.NewTValueNil()
			return 1
		}
		start := si + idx
		stack[base] = types.NewTValueInteger(types.LuaInteger(start + 1))     // 1-based
		if base+1 < len(stack) {
			stack[base+1] = types.NewTValueInteger(types.LuaInteger(start + len(pattern))) // 1-based end
		}
		return 2
	}

	// Pattern matching
	anchor := false
	pat := pattern
	if len(pat) > 0 && pat[0] == '^' {
		anchor = true
		pat = pat[1:]
	}

	start, end, caps, ncap, found := luaPatternFind(s, pat, si, anchor)
	if !found {
		stack[base] = types.NewTValueNil()
		return 1
	}

	// Return start, end (1-based) + captures
	stack[base] = types.NewTValueInteger(types.LuaInteger(start + 1))
	nret := 2
	if base+1 < len(stack) {
		stack[base+1] = types.NewTValueInteger(types.LuaInteger(end))
	}

	// Add captures
	for i := 0; i < ncap; i++ {
		idx := base + 2 + i
		if idx >= len(stack) {
			break
		}
		c := caps[i]
		if c.len == capPosition {
			stack[idx] = types.NewTValueInteger(types.LuaInteger(c.init + 1))
		} else {
			stack[idx] = types.NewTValueString(s[c.init : c.init+c.len])
		}
		nret++
	}

	return nret
}

// =============================================================================
// string.match(s, pattern [, init])
// =============================================================================
func bstringMatch(stack []types.TValue, base int) int {
	s := checkString(stack, base, 1, "match")
	pattern := checkString(stack, base, 2, "match")
	init := int(optInt(stack, base, 3, 1))

	n := len(s)
	if init < 0 { init = n + init + 1 }
	if init < 1 { init = 1 }
	if init > n+1 { init = n + 1 }

	si := init - 1

	anchor := false
	pat := pattern
	if len(pat) > 0 && pat[0] == '^' {
		anchor = true
		pat = pat[1:]
	}

	start, end, caps, ncap, found := luaPatternFind(s, pat, si, anchor)
	if !found {
		stack[base] = types.NewTValueNil()
		return 1
	}

	if ncap == 0 {
		// No captures — return the whole match
		stack[base] = types.NewTValueString(s[start:end])
		return 1
	}

	// Return captures
	for i := 0; i < ncap; i++ {
		idx := base + i
		if idx >= len(stack) {
			break
		}
		c := caps[i]
		if c.len == capPosition {
			stack[idx] = types.NewTValueInteger(types.LuaInteger(c.init + 1))
		} else {
			stack[idx] = types.NewTValueString(s[c.init : c.init+c.len])
		}
	}
	return ncap
}

// =============================================================================
// string.gmatch(s, pattern)
// Returns an iterator function
// =============================================================================
func bstringGmatch(stack []types.TValue, base int) int {
	s := checkString(stack, base, 1, "gmatch")
	pattern := checkString(stack, base, 2, "gmatch")

	pat := pattern
	anchor := false
	if len(pat) > 0 && pat[0] == '^' {
		anchor = true
		pat = pat[1:]
	}

	pos := 0 // current search position (0-based)

	// Create iterator as a Go function
	iterFn := func(iterStack []types.TValue, iterBase int) int {
		for pos <= len(s) {
			start, end, caps, ncap, found := luaPatternFind(s, pat, pos, anchor)
			if !found {
				iterStack[iterBase] = types.NewTValueNil()
				return 1
			}

			// Advance position for next iteration
			if end == start {
				pos = end + 1 // avoid infinite loop on empty match
			} else {
				pos = end
			}

			if ncap == 0 {
				iterStack[iterBase] = types.NewTValueString(s[start:end])
				return 1
			}

			for i := 0; i < ncap; i++ {
				idx := iterBase + i
				if idx >= len(iterStack) {
					break
				}
				c := caps[i]
				if c.len == capPosition {
					iterStack[idx] = types.NewTValueInteger(types.LuaInteger(c.init + 1))
				} else {
					iterStack[idx] = types.NewTValueString(s[c.init : c.init+c.len])
				}
			}
			return ncap
		}
		iterStack[iterBase] = types.NewTValueNil()
		return 1
	}

	stack[base] = &goFuncWrapper{fn: iterFn}
	return 1
}

// =============================================================================
// string.gsub(s, pattern, repl [, n])
// =============================================================================
func bstringGsub(stack []types.TValue, base int) int {
	s := checkString(stack, base, 1, "gsub")
	pattern := checkString(stack, base, 2, "gsub")
	// repl can be string, table, or function
	replVal := stack[base+3]
	maxRepl := int(optInt(stack, base, 4, types.LuaInteger(len(s)+1)))

	pat := pattern
	anchor := false
	if len(pat) > 0 && pat[0] == '^' {
		anchor = true
		pat = pat[1:]
	}

	var buf strings.Builder
	count := 0
	si := 0

	for (count < maxRepl) && si <= len(s) {
		ms := &luaMatchState{
			src: s,
			pat: pat,
		}
		ms.level = 0
		ms.matchDepth = luaMaxMatchCalls
		res := ms.match(si, 0)
		if res == -1 {
			if si < len(s) && !anchor {
				buf.WriteByte(s[si])
				si++
				continue
			}
			break
		}

		count++

		// Get replacement based on type
		if replVal != nil && replVal.IsString() {
			replStr, _ := replVal.GetValue().(string)
			buf.WriteString(luaPatternReplaceCaptures(replStr, s, ms.captures[:ms.level], ms.level, si, res))
		} else if replVal != nil && replVal.IsTable() {
			// Table replacement: use first capture (or whole match) as key
			capStr := getCaptureString(s, ms.captures[:ms.level], ms.level, 0, si, res)
			tbl := extractTable(replVal)
			if tbl != nil {
				val := tbl.Get(types.NewTValueString(capStr))
				if val != nil && !val.IsNil() && !val.IsFalse() {
					if val.IsString() {
						if sv, ok := val.GetValue().(string); ok {
							buf.WriteString(sv)
						}
					} else if val.IsInteger() {
						buf.WriteString(fmt.Sprintf("%d", val.GetInteger()))
					} else if val.IsFloat() {
						buf.WriteString(fmt.Sprintf("%g", val.GetFloat()))
					} else {
						buf.WriteString(s[si:res])
					}
				} else {
					buf.WriteString(s[si:res])
				}
			} else {
				buf.WriteString(s[si:res])
			}
		} else {
			// Not a string or table replacement — just keep original
			buf.WriteString(s[si:res])
		}

		if res == si {
			if si < len(s) {
				buf.WriteByte(s[si])
			}
			si++
		} else {
			si = res
		}

		if anchor {
			break
		}
	}

	// Append remaining
	if si <= len(s) {
		buf.WriteString(s[si:])
	}

	stack[base] = types.NewTValueString(buf.String())
	if base+1 < len(stack) {
		stack[base+1] = types.NewTValueInteger(types.LuaInteger(count))
	}
	return 2
}

// =============================================================================
// string.format(fmt, ...)
// =============================================================================
func bstringFormat(stack []types.TValue, base int) int {
	fmtStr := checkString(stack, base, 1, "format")
	argIdx := 2 // next argument index (1-based relative to base)

	var buf strings.Builder
	i := 0
	for i < len(fmtStr) {
		if fmtStr[i] != '%' {
			buf.WriteByte(fmtStr[i])
			i++
			continue
		}
		i++ // skip '%'
		if i >= len(fmtStr) {
			luaErrorString("invalid format string (ends with '%%')")
		}

		// Handle %%
		if fmtStr[i] == '%' {
			buf.WriteByte('%')
			i++
			continue
		}

		// Parse format specifier: %[flags][width][.precision]specifier
		specStart := i - 1 // includes the '%'
		
		// Flags
		for i < len(fmtStr) && (fmtStr[i] == '-' || fmtStr[i] == '+' || fmtStr[i] == ' ' || fmtStr[i] == '0' || fmtStr[i] == '#') {
			i++
		}
		// Width
		for i < len(fmtStr) && fmtStr[i] >= '0' && fmtStr[i] <= '9' {
			i++
		}
		// Precision
		if i < len(fmtStr) && fmtStr[i] == '.' {
			i++
			for i < len(fmtStr) && fmtStr[i] >= '0' && fmtStr[i] <= '9' {
				i++
			}
		}

		if i >= len(fmtStr) {
			luaErrorString("invalid format string")
		}

		spec := fmtStr[i]
		i++
		fmtSpec := fmtStr[specStart:i]

		// Lua format validation
		// 1. Total format spec length check (Lua limits to ~50 chars)
		if len(fmtSpec) > 50 {
			luaErrorString("invalid format (too long)")
		}

		// 2. Parse flags, width, precision from fmtSpec for validation
		fmtBody := fmtSpec[1 : len(fmtSpec)-1] // strip % and specifier
		hasFlags := ""
		fb := fmtBody
		for len(fb) > 0 && (fb[0] == '-' || fb[0] == '+' || fb[0] == ' ' || fb[0] == '0' || fb[0] == '#') {
			hasFlags += string(fb[0])
			fb = fb[1:]
		}
		widthStr := ""
		for len(fb) > 0 && fb[0] >= '0' && fb[0] <= '9' {
			widthStr += string(fb[0])
			fb = fb[1:]
		}
		precStr := ""
		hasDot := false
		if len(fb) > 0 && fb[0] == '.' {
			hasDot = true
			fb = fb[1:]
			for len(fb) > 0 && fb[0] >= '0' && fb[0] <= '9' {
				precStr += string(fb[0])
				fb = fb[1:]
			}
		}

		// Parse numeric values
		widthVal := 0
		if widthStr != "" {
			for _, c := range widthStr {
				widthVal = widthVal*10 + int(c-'0')
			}
		}
		precVal := 0
		if precStr != "" {
			for _, c := range precStr {
				precVal = precVal*10 + int(c-'0')
			}
		}

		// 3. Width or precision > 99 → "invalid conversion"
		if widthVal > 99 || precVal > 99 {
			luaErrorString(fmt.Sprintf("invalid conversion '%%%c'", spec))
		}

		// 4. %q cannot have any modifiers
		if spec == 'q' && len(fmtBody) > 0 {
			luaErrorString("cannot have modifiers with '%q'")
		}

		// 5. %c: no '0' flag, no '#' flag, no precision
		if spec == 'c' && (strings.ContainsAny(hasFlags, "0#") || hasDot) {
			luaErrorString(fmt.Sprintf("invalid conversion '%%%c'", spec))
		}

		// 6. %s: no '0' flag; precision with '0' flag invalid
		if spec == 's' && strings.Contains(hasFlags, "0") {
			luaErrorString(fmt.Sprintf("invalid conversion '%%%c'", spec))
		}

		// 7. Integer types: no '#' flag
		if (spec == 'd' || spec == 'i' || spec == 'u') && strings.Contains(hasFlags, "#") {
			luaErrorString(fmt.Sprintf("invalid conversion '%%%c'", spec))
		}

		// 8. %p: no precision allowed, but width and alignment flags are OK
		if spec == 'p' && hasDot {
			luaErrorString(fmt.Sprintf("invalid conversion '%%%c'", spec))
		}

		// 9. Unknown specifier
		validSpecs := "diouxXeEfgGaAcspq"
		if !strings.ContainsRune(validSpecs, rune(spec)) {
			luaErrorString(fmt.Sprintf("invalid conversion '%%%c'", spec))
		}

		// 10. Check argument availability (except %% which is already handled)
		{
			nArgs := realArgCount(stack, base)
			if argIdx > nArgs {
				luaErrorString(fmt.Sprintf("bad argument #%d to 'format' (no value)", argIdx))
			}
		}

		switch spec {
		case 'd', 'i':
			val := getFormatInt(stack, base, argIdx, "format")
			argIdx++
			buf.WriteString(fmt.Sprintf(strings.Replace(fmtSpec, string(spec), "d", 1), val))
		case 'u':
			val := getFormatInt(stack, base, argIdx, "format")
			argIdx++
			if val < 0 {
				// Unsigned interpretation
				buf.WriteString(fmt.Sprintf(strings.Replace(fmtSpec, "u", "d", 1), uint64(val)))
			} else {
				buf.WriteString(fmt.Sprintf(strings.Replace(fmtSpec, "u", "d", 1), val))
			}
		case 'f', 'F':
			val := getFormatFloat(stack, base, argIdx, "format")
			argIdx++
			buf.WriteString(fmt.Sprintf(fmtSpec, val))
		case 'e', 'E':
			val := getFormatFloat(stack, base, argIdx, "format")
			argIdx++
			buf.WriteString(fmt.Sprintf(fmtSpec, val))
		case 'g', 'G':
			val := getFormatFloat(stack, base, argIdx, "format")
			argIdx++
			buf.WriteString(fmt.Sprintf(fmtSpec, val))
		case 'a':
			val := getFormatFloat(stack, base, argIdx, "format")
			argIdx++
			if math.IsInf(val, 1) {
				buf.WriteString("inf")
			} else if math.IsInf(val, -1) {
				buf.WriteString("-inf")
			} else if math.IsNaN(val) {
				buf.WriteString("-nan")
			} else {
				goSpec := strings.Replace(fmtSpec, "a", "x", 1)
				s := fmt.Sprintf(goSpec, val)
				s = stripHexFloatExponentZeros(s, 'p')
				buf.WriteString(s)
			}
		case 'A':
			val := getFormatFloat(stack, base, argIdx, "format")
			argIdx++
			if math.IsInf(val, 1) {
				buf.WriteString("INF")
			} else if math.IsInf(val, -1) {
				buf.WriteString("-INF")
			} else if math.IsNaN(val) {
				buf.WriteString("-NAN")
			} else {
				goSpec := strings.Replace(fmtSpec, "A", "X", 1)
				s := fmt.Sprintf(goSpec, val)
				s = stripHexFloatExponentZeros(s, 'P')
				buf.WriteString(s)
			}
		case 'o':
			val := getFormatInt(stack, base, argIdx, "format")
			argIdx++
			// Lua treats %o as unsigned
			buf.WriteString(fmt.Sprintf(strings.Replace(fmtSpec, "o", "o", 1), uint64(val)))
		case 'x':
			val := getFormatInt(stack, base, argIdx, "format")
			argIdx++
			// Lua treats %x as unsigned
			buf.WriteString(fmt.Sprintf(strings.Replace(fmtSpec, "x", "x", 1), uint64(val)))
		case 'X':
			val := getFormatInt(stack, base, argIdx, "format")
			argIdx++
			// Lua treats %X as unsigned
			buf.WriteString(fmt.Sprintf(strings.Replace(fmtSpec, "X", "X", 1), uint64(val)))
		case 'c':
			val := getFormatInt(stack, base, argIdx, "format")
			argIdx++
			// %c converts integer to single character, but respects width/flags
			ch := string([]byte{byte(val & 0xff)})
			// Replace %...c with %...s to use Go's string formatting for width/alignment
			sfmt := fmtSpec[:len(fmtSpec)-1] + "s"
			buf.WriteString(fmt.Sprintf(sfmt, ch))
		case 's':
			sval := getFormatString(stack, base, argIdx, "format")
			argIdx++
			// Lua 5.4+: error if %s has width/precision and string contains zeros
			if len(fmtSpec) > 2 && strings.Contains(sval, "\x00") {
				luaErrorString("string format: string contains zeros")
			}
			buf.WriteString(fmt.Sprintf(fmtSpec, sval))
		case 'q':
			// In Lua 5.4+, %q handles non-string types:
			// strings → quoted with escapes
			// integers → decimal representation
			// floats → exact representation (handles inf, nan)
			// nil → "nil"
			// booleans → "true"/"false"
			// anything else → error "no literal"
			v := stack[base+argIdx]
			argIdx++
			if v == nil || v.IsNil() {
				buf.WriteString("nil")
			} else if v.IsBoolean() {
				if v.IsTrue() {
					buf.WriteString("true")
				} else {
					buf.WriteString("false")
				}
			} else if v.IsInteger() {
				n := int64(v.GetInteger())
				if n == math.MinInt64 {
					// math.mininteger can't round-trip as decimal because
					// -9223372036854775808 overflows when parsed as -(9223372036854775808).
					// Lua 5.4+ emits (-1 << 63) for this case.
					buf.WriteString("(-1 << 63)")
				} else {
					buf.WriteString(fmt.Sprintf("%d", n))
				}
			} else if v.IsFloat() {
				fv := float64(v.GetFloat())
				if math.IsInf(fv, 1) {
					buf.WriteString("1e9999")
				} else if math.IsInf(fv, -1) {
					buf.WriteString("-1e9999")
				} else if math.IsNaN(fv) {
					buf.WriteString("(0/0)")
				} else {
					// Use hex float format (%a) for exact precision roundtrip
					// This matches Lua 5.4+ quotefloat behavior
					s := fmt.Sprintf("%x", fv)
					// Go's %a uses 'p' for exponent, Lua uses 'p' too — compatible
					buf.WriteString(s)
				}
			} else if v.IsString() {
				sval := v.GetValue().(string)
				buf.WriteString(quoteString(sval))
			} else {
				luaErrorString(fmt.Sprintf("no literal"))
			}
		case 'p':
			// %p — format any value as a pointer (hex address)
			// In Lua 5.4, %p only returns addresses for GC objects
			// (tables, functions, coroutines, userdata, strings).
			// For non-GC types (nil, booleans, numbers), returns "(null)".
			v := stack[base+argIdx]
			argIdx++
			pResult := "(null)"
			if v != nil && !v.IsNil() && !v.IsBoolean() && !v.IsInteger() && !v.IsFloat() {
				if v.IsString() {
					// For strings: use unsafe to get the string data pointer
					s := v.GetValue().(string)
					sh := (*[2]uintptr)(unsafe.Pointer(&s))
					pResult = fmt.Sprintf("0x%014x", sh[0])
				} else {
					// For other GC objects: use reflect to get pointer
					val := v.GetValue()
					rv := reflect.ValueOf(val)
					switch rv.Kind() {
					case reflect.Ptr, reflect.UnsafePointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
						pResult = fmt.Sprintf("0x%014x", rv.Pointer())
					default:
						pResult = fmt.Sprintf("0x%014x", reflect.ValueOf(&val).Pointer())
					}
				}
			}
			// Handle width/alignment for %p using the fmtSpec
			// fmtSpec is like "%p", "%-60p", "%10p", etc.
			// Extract flags+width from fmtSpec, apply to pResult as a string
			if len(fmtSpec) > 2 { // more than just "%p"
				// Replace 'p' with 's' and use Go fmt for padding
				sFmt := fmtSpec[:len(fmtSpec)-1] + "s"
				pResult = fmt.Sprintf(sFmt, pResult)
			}
			buf.WriteString(pResult)
		default:
			luaErrorString(fmt.Sprintf("invalid conversion '%%%c'", spec))
		}
	}

	stack[base] = types.NewTValueString(buf.String())
	return 1
}

// getFormatInt extracts an integer for string.format
func getFormatInt(stack []types.TValue, base, argIdx int, fname string) int64 {
	idx := base + argIdx
	if idx >= len(stack) {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected, got no value)", argIdx, fname))
	}
	v := stack[idx]
	if v == nil || v.IsNil() {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected, got nil)", argIdx, fname))
	}
	if v.IsInteger() {
		return int64(v.GetInteger())
	}
	if v.IsFloat() {
		return int64(v.GetFloat())
	}
	if v.IsString() {
		if s, ok := v.GetValue().(string); ok {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return n
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return int64(f)
			}
		}
	}
	luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected)", argIdx, fname))
	return 0
}

// getFormatFloat extracts a float for string.format
func getFormatFloat(stack []types.TValue, base, argIdx int, fname string) float64 {
	idx := base + argIdx
	if idx >= len(stack) {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected, got no value)", argIdx, fname))
	}
	v := stack[idx]
	if v == nil || v.IsNil() {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected, got nil)", argIdx, fname))
	}
	if v.IsFloat() {
		return float64(v.GetFloat())
	}
	if v.IsInteger() {
		return float64(v.GetInteger())
	}
	if v.IsString() {
		if s, ok := v.GetValue().(string); ok {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f
			}
		}
	}
	luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (number expected)", argIdx, fname))
	return 0
}

// getFormatString extracts a string for string.format %s
func getFormatString(stack []types.TValue, base, argIdx int, fname string) string {
	idx := base + argIdx
	if idx >= len(stack) {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (string expected, got no value)", argIdx, fname))
	}
	v := stack[idx]
	if v == nil || v.IsNil() {
		return "nil"
	}
	if v.IsString() {
		if s, ok := v.GetValue().(string); ok {
			return s
		}
	}
	if v.IsInteger() {
		return fmt.Sprintf("%d", v.GetInteger())
	}
	if v.IsFloat() {
		return fmt.Sprintf("%g", v.GetFloat())
	}
	if v.IsBoolean() {
		if v.IsTrue() {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%s: %p", luaTypeName(v), v)
}

// stripHexFloatExponentZeros removes leading zeros from the exponent part
// of a hex float string. Go produces "0x1.8p+03" but Lua expects "0x1.8p+3".
func stripHexFloatExponentZeros(s string, pChar byte) string {
	idx := strings.IndexByte(s, pChar)
	if idx < 0 {
		return s
	}
	// Find the start of digits after p/P and optional +/-
	expStart := idx + 1
	if expStart < len(s) && (s[expStart] == '+' || s[expStart] == '-') {
		expStart++
	}
	// Find first non-zero digit
	digitStart := expStart
	for digitStart < len(s)-1 && s[digitStart] == '0' {
		digitStart++
	}
	if digitStart > expStart {
		s = s[:expStart] + s[digitStart:]
	}
	return s
}

// quoteString produces a Lua %q quoted string
func quoteString(s string) string {
	var buf strings.Builder
	buf.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			buf.WriteString("\\\\")
		case '"':
			buf.WriteString("\\\"")
		case '\n':
			buf.WriteString("\\\n")
		case '\r':
			buf.WriteString("\\r")
		case '\x00':
			// If next char is a digit, use 3-digit form to avoid ambiguity
			if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
				buf.WriteString("\\000")
			} else {
				buf.WriteString("\\0")
			}
		case '\x1a':
			buf.WriteString("\\26")
		default:
			if c < 0x20 || c == 0x7f {
				// Escape control chars and non-ASCII bytes as \ddd
				// If next char is a digit, always use 3-digit form
				if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
					buf.WriteString(fmt.Sprintf("\\%03d", c))
				} else {
					buf.WriteString(fmt.Sprintf("\\%d", c))
				}
			} else {
				buf.WriteByte(c)
			}
		}
	}
	buf.WriteByte('"')
	return buf.String()
}

// =============================================================================
// string.dump(f) — stub
// =============================================================================
func bstringDump(stack []types.TValue, base int) int {
	// Stub: return an empty string (not critical)
	stack[base] = types.NewTValueString("")
	return 1
}

// =============================================================================
// string.pack(fmt, v1, v2, ...)
// =============================================================================
func bstringPack(stack []types.TValue, base int) int {
	fmtStr := checkString(stack, base, 1, "pack")
	argIdx := 2
	var buf bytes.Buffer

	i := 0
	for i < len(fmtStr) {
		opt := fmtStr[i]
		i++

		switch opt {
		case '<', '>', '=', '!':
			// Endianness/alignment markers — skip for basic impl
			continue
		case 'b': // signed byte
			val := getFormatInt(stack, base, argIdx, "pack")
			argIdx++
			buf.WriteByte(byte(val))
		case 'B': // unsigned byte
			val := getFormatInt(stack, base, argIdx, "pack")
			argIdx++
			buf.WriteByte(byte(val))
		case 'h': // signed short (2 bytes, little-endian)
			val := getFormatInt(stack, base, argIdx, "pack")
			argIdx++
			var b [2]byte
			binary.LittleEndian.PutUint16(b[:], uint16(val))
			buf.Write(b[:])
		case 'H': // unsigned short
			val := getFormatInt(stack, base, argIdx, "pack")
			argIdx++
			var b [2]byte
			binary.LittleEndian.PutUint16(b[:], uint16(val))
			buf.Write(b[:])
		case 'i', 'I': // int (default 4 bytes)
			// Check for size suffix
			size := 4
			if i < len(fmtStr) && fmtStr[i] >= '1' && fmtStr[i] <= '8' {
				size = int(fmtStr[i] - '0')
				i++
			}
			val := getFormatInt(stack, base, argIdx, "pack")
			argIdx++
			packIntLE(&buf, val, size)
		case 'l': // signed long (8 bytes)
			val := getFormatInt(stack, base, argIdx, "pack")
			argIdx++
			var b [8]byte
			binary.LittleEndian.PutUint64(b[:], uint64(val))
			buf.Write(b[:])
		case 'L': // unsigned long (8 bytes)
			val := getFormatInt(stack, base, argIdx, "pack")
			argIdx++
			var b [8]byte
			binary.LittleEndian.PutUint64(b[:], uint64(val))
			buf.Write(b[:])
		case 'j', 'J': // lua integer (8 bytes)
			val := getFormatInt(stack, base, argIdx, "pack")
			argIdx++
			var b [8]byte
			binary.LittleEndian.PutUint64(b[:], uint64(val))
			buf.Write(b[:])
		case 'T': // size_t (8 bytes on 64-bit)
			val := getFormatInt(stack, base, argIdx, "pack")
			argIdx++
			var b [8]byte
			binary.LittleEndian.PutUint64(b[:], uint64(val))
			buf.Write(b[:])
		case 'f': // float (4 bytes)
			val := getFormatFloat(stack, base, argIdx, "pack")
			argIdx++
			var b [4]byte
			binary.LittleEndian.PutUint32(b[:], math.Float32bits(float32(val)))
			buf.Write(b[:])
		case 'd', 'n': // double (8 bytes)
			val := getFormatFloat(stack, base, argIdx, "pack")
			argIdx++
			var b [8]byte
			binary.LittleEndian.PutUint64(b[:], math.Float64bits(val))
			buf.Write(b[:])
		case 's': // string with length prefix
			sval := checkString(stack, base, argIdx, "pack")
			argIdx++
			// Default: 8-byte length prefix
			size := 8
			if i < len(fmtStr) && fmtStr[i] >= '1' && fmtStr[i] <= '8' {
				size = int(fmtStr[i] - '0')
				i++
			}
			packIntLE(&buf, int64(len(sval)), size)
			buf.WriteString(sval)
		case 'z': // zero-terminated string
			sval := checkString(stack, base, argIdx, "pack")
			argIdx++
			buf.WriteString(sval)
			buf.WriteByte(0)
		case 'x': // padding byte
			buf.WriteByte(0)
		case ' ':
			continue
		default:
			// Skip digit suffixes for alignment
			if opt >= '0' && opt <= '9' {
				continue
			}
			luaErrorString(fmt.Sprintf("invalid format option '%c'", opt))
		}
	}

	stack[base] = types.NewTValueString(buf.String())
	return 1
}

func packIntLE(buf *bytes.Buffer, val int64, size int) {
	for j := 0; j < size; j++ {
		buf.WriteByte(byte(val >> (uint(j) * 8)))
	}
}

// =============================================================================
// string.unpack(fmt, s [, pos])
// =============================================================================
func bstringUnpack(stack []types.TValue, base int) int {
	fmtStr := checkString(stack, base, 1, "unpack")
	s := checkString(stack, base, 2, "unpack")
	pos := int(optInt(stack, base, 3, 1)) - 1 // convert to 0-based

	resultIdx := 0
	i := 0
	for i < len(fmtStr) {
		opt := fmtStr[i]
		i++

		switch opt {
		case '<', '>', '=', '!':
			continue
		case 'b': // signed byte
			if pos >= len(s) {
				luaErrorString("data string too short")
			}
			val := int8(s[pos])
			pos++
			stack[base+resultIdx] = types.NewTValueInteger(types.LuaInteger(val))
			resultIdx++
		case 'B': // unsigned byte
			if pos >= len(s) {
				luaErrorString("data string too short")
			}
			val := s[pos]
			pos++
			stack[base+resultIdx] = types.NewTValueInteger(types.LuaInteger(val))
			resultIdx++
		case 'h': // signed short
			if pos+2 > len(s) {
				luaErrorString("data string too short")
			}
			val := int16(binary.LittleEndian.Uint16([]byte(s[pos:])))
			pos += 2
			stack[base+resultIdx] = types.NewTValueInteger(types.LuaInteger(val))
			resultIdx++
		case 'H': // unsigned short
			if pos+2 > len(s) {
				luaErrorString("data string too short")
			}
			val := binary.LittleEndian.Uint16([]byte(s[pos:]))
			pos += 2
			stack[base+resultIdx] = types.NewTValueInteger(types.LuaInteger(val))
			resultIdx++
		case 'i', 'I': // int
			size := 4
			if i < len(fmtStr) && fmtStr[i] >= '1' && fmtStr[i] <= '8' {
				size = int(fmtStr[i] - '0')
				i++
			}
			if pos+size > len(s) {
				luaErrorString("data string too short")
			}
			val := unpackIntLE(s, pos, size, opt == 'i')
			pos += size
			stack[base+resultIdx] = types.NewTValueInteger(types.LuaInteger(val))
			resultIdx++
		case 'l': // signed long
			if pos+8 > len(s) {
				luaErrorString("data string too short")
			}
			val := int64(binary.LittleEndian.Uint64([]byte(s[pos:])))
			pos += 8
			stack[base+resultIdx] = types.NewTValueInteger(types.LuaInteger(val))
			resultIdx++
		case 'L', 'j', 'J', 'T': // unsigned long, lua integer, size_t
			if pos+8 > len(s) {
				luaErrorString("data string too short")
			}
			val := binary.LittleEndian.Uint64([]byte(s[pos:]))
			pos += 8
			stack[base+resultIdx] = types.NewTValueInteger(types.LuaInteger(val))
			resultIdx++
		case 'f': // float
			if pos+4 > len(s) {
				luaErrorString("data string too short")
			}
			val := math.Float32frombits(binary.LittleEndian.Uint32([]byte(s[pos:])))
			pos += 4
			stack[base+resultIdx] = types.NewTValueFloat(types.LuaNumber(val))
			resultIdx++
		case 'd', 'n': // double
			if pos+8 > len(s) {
				luaErrorString("data string too short")
			}
			val := math.Float64frombits(binary.LittleEndian.Uint64([]byte(s[pos:])))
			pos += 8
			stack[base+resultIdx] = types.NewTValueFloat(types.LuaNumber(val))
			resultIdx++
		case 's': // string with length prefix
			size := 8
			if i < len(fmtStr) && fmtStr[i] >= '1' && fmtStr[i] <= '8' {
				size = int(fmtStr[i] - '0')
				i++
			}
			if pos+size > len(s) {
				luaErrorString("data string too short")
			}
			slen := int(unpackIntLE(s, pos, size, false))
			pos += size
			if pos+slen > len(s) {
				luaErrorString("data string too short")
			}
			stack[base+resultIdx] = types.NewTValueString(s[pos : pos+slen])
			pos += slen
			resultIdx++
		case 'z': // zero-terminated string
			end := strings.IndexByte(s[pos:], 0)
			if end == -1 {
				luaErrorString("unfinished string for format 'z'")
			}
			stack[base+resultIdx] = types.NewTValueString(s[pos : pos+end])
			pos += end + 1
			resultIdx++
		case 'x': // padding byte
			pos++
		case ' ':
			continue
		default:
			if opt >= '0' && opt <= '9' {
				continue
			}
			luaErrorString(fmt.Sprintf("invalid format option '%c'", opt))
		}
	}

	// Last return value is the position after the last read item (1-based)
	stack[base+resultIdx] = types.NewTValueInteger(types.LuaInteger(pos + 1))
	resultIdx++
	return resultIdx
}

func unpackIntLE(s string, pos, size int, signed bool) int64 {
	var val uint64
	for j := 0; j < size; j++ {
		val |= uint64(s[pos+j]) << (uint(j) * 8)
	}
	if signed && size < 8 {
		// Sign extend
		signBit := uint64(1) << (uint(size)*8 - 1)
		if val&signBit != 0 {
			val |= ^((uint64(1) << (uint(size) * 8)) - 1)
		}
	}
	return int64(val)
}

// =============================================================================
// string.packsize(fmt)
// =============================================================================
func bstringPacksize(stack []types.TValue, base int) int {
	fmtStr := checkString(stack, base, 1, "packsize")
	size := 0
	i := 0
	for i < len(fmtStr) {
		opt := fmtStr[i]
		i++
		switch opt {
		case '<', '>', '=', '!', ' ':
			continue
		case 'b', 'B':
			size++
		case 'h', 'H':
			size += 2
		case 'i', 'I':
			sz := 4
			if i < len(fmtStr) && fmtStr[i] >= '1' && fmtStr[i] <= '8' {
				sz = int(fmtStr[i] - '0')
				i++
			}
			size += sz
		case 'l', 'L', 'j', 'J', 'T':
			size += 8
		case 'f':
			size += 4
		case 'd', 'n':
			size += 8
		case 'x':
			size++
		case 's', 'z':
			luaErrorString("variable-size format in packsize")
		default:
			if opt >= '0' && opt <= '9' {
				continue
			}
			luaErrorString(fmt.Sprintf("invalid format option '%c'", opt))
		}
	}
	stack[base] = types.NewTValueInteger(types.LuaInteger(size))
	return 1
}

// =============================================================================
// registerStringLib populates the string module table with all functions.
// =============================================================================
func registerStringLib(stringMod tableapi.TableInterface) {
	funcs := map[string]func([]types.TValue, int) int{
		"len":      bstringLen,
		"byte":     bstringByte,
		"char":     bstringChar,
		"sub":      bstringSub,
		"rep":      bstringRep,
		"reverse":  bstringReverse,
		"upper":    bstringUpper,
		"lower":    bstringLower,
		"find":     bstringFind,
		"match":    bstringMatch,
		"gmatch":   bstringGmatch,
		"gsub":     bstringGsub,
		"format":   bstringFormat,
		"dump":     bstringDump,
		"pack":     bstringPack,
		"unpack":   bstringUnpack,
		"packsize": bstringPacksize,
	}
	for name, fn := range funcs {
		key := types.NewTValueString(name)
		stringMod.Set(key, &goFuncWrapper{fn: fn})
	}
}
