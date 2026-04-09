// Debug library implementation for Lua 5.4/5.5
package internal

import (
	"strings"

	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
)

// debugHookFn stores the current hook function
var debugHookFn types.TValue
var debugHookMask string

// bdebugGethook implements debug.gethook()
// Returns nil, nil when no hook is set.
func bdebugGethook(stack []types.TValue, base int) int {
	if debugHookFn == nil || debugHookFn.IsNil() {
		stack[base] = types.NewTValueNil()
		stack[base+1] = types.NewTValueNil()
		return 2
	}
	stack[base] = debugHookFn
	stack[base+1] = types.NewTValueString(debugHookMask)
	return 2
}

// bdebugSethook implements debug.sethook(hook, mask, count)
// If hook is nil or no args, disables the hook.
func bdebugSethook(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs == 0 || stack[base+1] == nil || stack[base+1].IsNil() {
		debugHookFn = types.NewTValueNil()
		debugHookMask = ""
		return 0
	}
	debugHookFn = stack[base+1]
	if nArgs >= 2 && stack[base+2] != nil && stack[base+2].IsString() {
		debugHookMask, _ = stack[base+2].GetValue().(string)
	} else {
		debugHookMask = ""
	}
	return 0
}

// bdebugGetinfo implements debug.getinfo(thread, level, what)
// Returns a table with function information.
func bdebugGetinfo(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 1 || stack[base+1] == nil || stack[base+1].IsNil() {
		stack[base] = types.NewTValueNil()
		return 1
	}

	// Get what string
	what := "nSl"
	if nArgs >= 2 && stack[base+2] != nil && stack[base+2].IsString() {
		what, _ = stack[base+2].GetValue().(string)
	}

	// Create result table
	tbl := createModuleTable()

	// Determine function type
	var fn types.TValue
	isCFunc := false

	if nArgs >= 1 && stack[base+1] != nil && !stack[base+1].IsNil() {
		fn = stack[base+1]
		isCFunc = fn.IsCClosure() || fn.IsLightCFunction()
	}

	// S option - source info
	if strings.Contains(what, "S") {
		if isCFunc {
			tbl.Set(types.NewTValueString("source"), types.NewTValueString("=[C]"))
			tbl.Set(types.NewTValueString("short_src"), types.NewTValueString("[C]"))
			tbl.Set(types.NewTValueString("linedefined"), types.NewTValueInteger(-1))
			tbl.Set(types.NewTValueString("what"), types.NewTValueString("C"))
		} else {
			tbl.Set(types.NewTValueString("source"), types.NewTValueString(""))
			tbl.Set(types.NewTValueString("short_src"), types.NewTValueString("[string \"...\"]"))
			tbl.Set(types.NewTValueString("linedefined"), types.NewTValueInteger(0))
			tbl.Set(types.NewTValueString("what"), types.NewTValueString("main"))
		}
	}

	// n option - name info
	if strings.Contains(what, "n") {
		if isCFunc {
			tbl.Set(types.NewTValueString("name"), types.NewTValueString("?"))
			tbl.Set(types.NewTValueString("namewhat"), types.NewTValueString(""))
		}
		tbl.Set(types.NewTValueString("nups"), types.NewTValueInteger(0))
	}

	// f option - func
	if strings.Contains(what, "f") && fn != nil {
		tbl.Set(types.NewTValueString("func"), fn)
	}

	// L option - activelines
	if strings.Contains(what, "L") && !isCFunc {
		activelines := createModuleTable()
		tbl.Set(types.NewTValueString("activelines"), &tableWrapper{tbl: activelines})
	}

	stack[base] = &tableWrapper{tbl: tbl}
	return 1
}

// bdebugTraceback implements debug.traceback(thread, message, level)
func bdebugTraceback(stack []types.TValue, base int) int {
	stack[base] = types.NewTValueString("stack traceback:\n")
	return 1
}

// bdebugGetlocal implements debug.getlocal(thread, level, local)
// Returns the name and value of a local variable.
func bdebugGetlocal(stack []types.TValue, base int) int {
	stack[base] = types.NewTValueNil()
	return 1
}

// bdebugSetlocal implements debug.setlocal(thread, level, local, value)
// Sets the value of a local variable.
func bdebugSetlocal(stack []types.TValue, base int) int {
	return 0
}

// bdebugUpvalueid implements debug.upvalueid(f, n)
// Returns unique identifier for the n-th upvalue of f.
func bdebugUpvalueid(stack []types.TValue, base int) int {
	stack[base] = types.NewTValueInteger(0)
	return 1
}

// bdebugUpvaluejoin implements debug.upvaluejoin(f1, n1, f2, n2)
// Make the n1-th upvalue of f1 refer to the n2-th upvalue of f2.
func bdebugUpvaluejoin(stack []types.TValue, base int) int {
	return 0
}

// registerDebugLib registers debug library functions in the module table
func registerDebugLib(debugMod tableapi.TableInterface) {
	debugMod.Set(types.NewTValueString("gethook"), &goFuncWrapper{fn: bdebugGethook})
	debugMod.Set(types.NewTValueString("sethook"), &goFuncWrapper{fn: bdebugSethook})
	debugMod.Set(types.NewTValueString("getinfo"), &goFuncWrapper{fn: bdebugGetinfo})
	debugMod.Set(types.NewTValueString("traceback"), &goFuncWrapper{fn: bdebugTraceback})
	debugMod.Set(types.NewTValueString("getlocal"), &goFuncWrapper{fn: bdebugGetlocal})
	debugMod.Set(types.NewTValueString("setlocal"), &goFuncWrapper{fn: bdebugSetlocal})
	debugMod.Set(types.NewTValueString("upvalueid"), &goFuncWrapper{fn: bdebugUpvalueid})
	debugMod.Set(types.NewTValueString("upvaluejoin"), &goFuncWrapper{fn: bdebugUpvaluejoin})
}

// =============================================================================
// utf8 library
// =============================================================================

// utf8CharPattern is the Lua UTF-8 character class pattern
// Matches valid UTF-8 encoded Unicode codepoints
const utf8CharPattern = "[\x80-\xBF]?[\x00-\x7F]|[\xC2-\xDF][\x80-\xBF]|[\xE0-\xEF][\x80-\xBF]{2}|[\xF0-\xF4][\x80-\xBF]{3}"

// getCharBytes returns the number of bytes for a UTF-8 character starting at s[i] (0-based).
// Returns 0 if the byte is an invalid or incomplete UTF-8 start byte.
func getUtf8CharBytes(s string, i int) int {
	if i >= len(s) {
		return 0
	}
	b := s[i]
	// ASCII: 0x00-0x7F
	if int(b) < 0x80 {
		return 1
	}
	// Continuation bytes (0x80-0xBF) are invalid as start bytes
	if int(b) >= 0x80 && int(b) < 0xC2 {
		return 0
	}
	// 2-byte: 0xC2-0xDF
	if int(b) >= 0xC2 && int(b) < 0xE0 {
		if i+1 < len(s) && int(s[i+1]) >= 0x80 && int(s[i+1]) < 0xC0 {
			return 2
		}
		return 0
	}
	// 3-byte: 0xE0-0xEF
	if int(b) >= 0xE0 && int(b) < 0xF0 {
		if i+2 < len(s) && int(s[i+1]) >= 0x80 && int(s[i+1]) < 0xC0 && int(s[i+2]) >= 0x80 && int(s[i+2]) < 0xC0 {
			return 3
		}
		return 0
	}
	// 4-byte: 0xF0-0xF4
	if int(b) >= 0xF0 && int(b) < 0xF5 {
		if i+3 < len(s) && int(s[i+1]) >= 0x80 && int(s[i+1]) < 0xC0 && int(s[i+2]) >= 0x80 && int(s[i+2]) < 0xC0 && int(s[i+3]) >= 0x80 && int(s[i+3]) < 0xC0 {
			// Validate codepoint range: must be <= U+10FFFF
			codepoint := ((int(b) & 0x07) << 18) | ((int(s[i+1]) & 0x3F) << 12) | ((int(s[i+2]) & 0x3F) << 6) | (int(s[i+3]) & 0x3F)
			if codepoint > 0x10FFFF {
				return 0
			}
			return 4
		}
		return 0
	}
	// Invalid start bytes: 0xF5-0xFF
	return 0
}

// decodeUtf8 decodes a UTF-8 codepoint starting at position pos (1-based) in string s.
// Returns (codepoint, numBytes). Returns (0, 0) on invalid UTF-8.
func decodeUtf8(s string, pos int) (codepoint int, n int) {
	if pos < 1 || pos > len(s) {
		return 0, 0
	}
	b := int(s[pos-1])
	// ASCII: 0x00-0x7F
	if b < 0x80 {
		return b, 1
	}
	// Continuation bytes (0x80-0xBF) are invalid as start bytes
	if b >= 0x80 && b < 0xC2 {
		return 0, 0
	}
	// 2-byte: 0xC2-0xDF
	if b >= 0xC2 && b < 0xE0 {
		if pos+1 <= len(s) && int(s[pos]) >= 0x80 && int(s[pos]) < 0xC0 {
			return ((b & 0x1F) << 6) | (int(s[pos]) & 0x3F), 2
		}
		return 0, 0
	}
	// 3-byte: 0xE0-0xEF
	if b >= 0xE0 && b < 0xF0 {
		if pos+2 <= len(s) && int(s[pos]) >= 0x80 && int(s[pos]) < 0xC0 && int(s[pos+1]) >= 0x80 && int(s[pos+1]) < 0xC0 {
			codepoint = ((b & 0x0F) << 12) | ((int(s[pos]) & 0x3F) << 6) | (int(s[pos+1]) & 0x3F)
			if codepoint > 0x10FFFF {
				return 0, 0
			}
			return codepoint, 3
		}
		return 0, 0
	}
	// 4-byte: 0xF0-0xF4
	if b >= 0xF0 && b < 0xF5 {
		if pos+3 <= len(s) && int(s[pos]) >= 0x80 && int(s[pos]) < 0xC0 && int(s[pos+1]) >= 0x80 && int(s[pos+1]) < 0xC0 && int(s[pos+2]) >= 0x80 && int(s[pos+2]) < 0xC0 {
			codepoint = ((b & 0x07) << 18) | ((int(s[pos]) & 0x3F) << 12) | ((int(s[pos+1]) & 0x3F) << 6) | (int(s[pos+2]) & 0x3F)
			if codepoint > 0x10FFFF {
				return 0, 0
			}
			return codepoint, 4
		}
		return 0, 0
	}
	// Invalid start bytes: 0xF5-0xFF
	return 0, 0
}

// butf8Len implements utf8.len(s [, init [, end [, lax]]])
// Returns the number of UTF-8 characters in s, or nil+position on invalid UTF-8.
func butf8Len(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 1 {
		stack[base] = types.NewTValueNil()
		return 1
	}
	v := stack[base+1]
	if !v.IsString() {
		stack[base] = types.NewTValueNil()
		stack[base+1] = types.NewTValueInteger(1)
		return 2
	}
	s := v.GetValue().(string)
	if len(s) == 0 {
		stack[base] = types.NewTValueInteger(0)
		return 1
	}

	// Parse init (default 1)
	init := 1
	if nArgs >= 2 {
		v2 := stack[base+2]
		if !(v2 == nil || v2.IsNil()) {
			init = int(checkInt(stack, base, 2, "len"))
		}
	}
	// Parse end (default -1)
	end := -1
	if nArgs >= 3 {
		v3 := stack[base+3]
		if !(v3 == nil || v3.IsNil()) {
			end = int(checkInt(stack, base, 3, "len"))
		}
	}

	// Normalize to 1-based
	if init < 0 {
		init = len(s) + init + 1
	}
	if end < 0 {
		end = len(s) + end + 1
	}
	// Clamp
	if init < 1 {
		init = 1
	}
	if end > len(s) {
		end = len(s)
	}
	if init > end || init > len(s) {
		stack[base] = types.NewTValueNil()
		stack[base+1] = types.NewTValueInteger(types.LuaInteger(init))
		return 2
	}

	runeCount := 0
	pos := 1
	for pos <= len(s) {
		n := getUtf8CharBytes(s, pos-1)
		if n == 0 {
			stack[base] = types.NewTValueNil()
			stack[base+1] = types.NewTValueInteger(types.LuaInteger(pos))
			return 2
		}
		if pos >= init && pos <= end {
			runeCount++
		}
		if pos > end {
			break
		}
		pos += n
	}

	stack[base] = types.NewTValueInteger(types.LuaInteger(runeCount))
	return 1
}

// butf8Codes implements utf8.codes(s) — returns iterator for (byte-offset, codepoint)
func butf8Codes(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 1 {
		stack[base] = types.NewTValueNil()
		return 1
	}
	v := stack[base+1]
	if !v.IsString() {
		stack[base] = types.NewTValueNil()
		return 1
	}
	s := v.GetValue().(string)

	pos := 1 // current byte position, 1-based

	iterFn := func(iterStack []types.TValue, iterBase int) int {
		if pos < 1 || pos > len(s) {
			iterStack[iterBase] = types.NewTValueNil()
			return 1
		}

		codepoint, n := decodeUtf8(s, pos)
		if n == 0 {
			iterStack[iterBase] = types.NewTValueNil()
			return 1
		}

		currentPos := pos
		iterStack[iterBase] = types.NewTValueInteger(types.LuaInteger(currentPos))
		iterStack[iterBase+1] = types.NewTValueInteger(types.LuaInteger(codepoint))
		pos = pos + n
		return 2
	}

	stack[base] = &goFuncWrapper{fn: iterFn}
	return 1
}

// butf8Offset implements utf8.offset(s, n [, init])
// When n == 0: returns (start-byte, end-byte) of character at position init
// Otherwise: returns byte position of the n-th UTF-8 character from init
func butf8Offset(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 2 {
		stack[base] = types.NewTValueNil()
		return 1
	}
	v := stack[base+1]
	if !v.IsString() {
		stack[base] = types.NewTValueNil()
		return 1
	}
	s := v.GetValue().(string)
	n := int(checkInt(stack, base, 2, "offset"))

	init := 1
	if nArgs >= 3 {
		v3 := stack[base+3]
		if !(v3 == nil || v3.IsNil()) {
			init = int(checkInt(stack, base, 3, "offset"))
		}
	}

	if init < 0 {
		init = len(s) + init + 1
	}
	if init < 1 {
		stack[base] = types.NewTValueNil()
		return 1
	}
	if init > len(s) {
		if len(s) == 0 && (n == 1 || n == 0) {
			stack[base] = types.NewTValueInteger(1)
			return 1
		}
		stack[base] = types.NewTValueNil()
		return 1
	}

	pos := init

	if n == 0 {
		// Return (start, end) byte positions of character at pos
		nb := getUtf8CharBytes(s, pos-1)
		if nb == 0 || pos+nb-1 > len(s) {
			stack[base] = types.NewTValueNil()
			return 1
		}
		stack[base] = types.NewTValueInteger(types.LuaInteger(pos))
		stack[base+1] = types.NewTValueInteger(types.LuaInteger(pos + nb - 1))
		return 2
	}

	if n > 0 {
		for i := 1; i < n; i++ {
			if pos > len(s) {
				stack[base] = types.NewTValueNil()
				return 1
			}
			nb := getUtf8CharBytes(s, pos-1)
			if nb == 0 {
				stack[base] = types.NewTValueNil()
				return 1
			}
			pos += nb
		}
	} else if n < 0 {
		for i := 0; i < -n; i++ {
			if pos <= 1 {
				stack[base] = types.NewTValueNil()
				return 1
			}
			pos--
			for pos > 1 {
				nb := getUtf8CharBytes(s, pos-1)
				if nb > 0 {
					break
				}
				pos--
			}
		}
	}

	if pos < 1 || pos > len(s) {
		stack[base] = types.NewTValueNil()
		return 1
	}
	stack[base] = types.NewTValueInteger(types.LuaInteger(pos))
	return 1
}

// butf8Char// butf8Char implements utf8.char(...) — converts codepoints to UTF-8 string
func butf8Char(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	var buf strings.Builder
	for i := 1; i <= nArgs; i++ {
		codepoint := int(checkInt(stack, base, i, "char"))
		if codepoint < 0 || codepoint > 0x10FFFF {
			stack[base] = types.NewTValueNil()
			return 1
		}
		if codepoint < 0x80 {
			buf.WriteByte(byte(codepoint))
		} else if codepoint < 0x800 {
			buf.WriteByte(byte(0xC0 | (codepoint >> 6)))
			buf.WriteByte(byte(0x80 | (codepoint & 0x3F)))
		} else if codepoint < 0x10000 {
			buf.WriteByte(byte(0xE0 | (codepoint >> 12)))
			buf.WriteByte(byte(0x80 | ((codepoint >> 6) & 0x3F)))
			buf.WriteByte(byte(0x80 | (codepoint & 0x3F)))
		} else {
			buf.WriteByte(byte(0xF0 | (codepoint >> 18)))
			buf.WriteByte(byte(0x80 | ((codepoint >> 12) & 0x3F)))
			buf.WriteByte(byte(0x80 | ((codepoint >> 6) & 0x3F)))
			buf.WriteByte(byte(0x80 | (codepoint & 0x3F)))
		}
	}
	stack[base] = types.NewTValueString(buf.String())
	return 1
}

// butf8Codepoint implements utf8.codepoint(s [, init [, end [, lax]]])
// Returns codepoints as multiple values.
func butf8Codepoint(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 1 {
		stack[base] = types.NewTValueNil()
		return 1
	}
	v := stack[base+1]
	if !v.IsString() {
		stack[base] = types.NewTValueNil()
		return 1
	}
	s := v.GetValue().(string)

	init := 1
	end := -1
	if nArgs >= 2 {
		v2 := stack[base+2]
		if !(v2 == nil || v2.IsNil()) {
			init = int(checkInt(stack, base, 2, "codepoint"))
		}
	}
	if nArgs >= 3 {
		v3 := stack[base+3]
		if !(v3 == nil || v3.IsNil()) {
			end = int(checkInt(stack, base, 3, "codepoint"))
		}
	}

	if init < 0 {
		init = len(s) + init + 1
	}
	if end < 0 {
		end = len(s) + end + 1
	}

	count := 0
	pos := init
	if init < 1 {
		pos = 1
	}

	for pos <= len(s) && (end < 0 || pos <= end) {
		codepoint, n := decodeUtf8(s, pos)
		if n == 0 {
			stack[base] = types.NewTValueNil()
			return 1
		}
		stack[base+count] = types.NewTValueInteger(types.LuaInteger(codepoint))
		count++
		pos += n
	}

	return count
}

// registerUtf8Lib registers utf8 library functions
func registerUtf8Lib(utf8Mod tableapi.TableInterface) {
	// charpattern — Lua UTF-8 character class
	utf8Mod.Set(types.NewTValueString("charpattern"), types.NewTValueString(utf8CharPattern))
	// Functions
	utf8Mod.Set(types.NewTValueString("len"), &goFuncWrapper{fn: butf8Len})
	utf8Mod.Set(types.NewTValueString("codes"), &goFuncWrapper{fn: butf8Codes})
	utf8Mod.Set(types.NewTValueString("offset"), &goFuncWrapper{fn: butf8Offset})
	utf8Mod.Set(types.NewTValueString("char"), &goFuncWrapper{fn: butf8Char})
	utf8Mod.Set(types.NewTValueString("codepoint"), &goFuncWrapper{fn: butf8Codepoint})
}
