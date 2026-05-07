package stdlib

import (
	"unicode/utf8"

	luaapi "github.com/akzj/go-lua/internal/api"
)

// ---------------------------------------------------------------------------
// UTF-8 library
// Reference: lua-master/lutf8lib.c
// ---------------------------------------------------------------------------

const (
	maxUnicode = 0x10FFFF   // maximum valid Unicode code point
	maxUTF     = 0x7FFFFFFF // maximum value encodable (non-strict mode)
)

// isCont returns true if byte is a UTF-8 continuation byte (10xxxxxx).
func isCont(c byte) bool {
	return (c & 0xC0) == 0x80
}

// uPosRelat converts a relative string position to absolute (1-based).
// Negative means back from end.
func uPosRelat(pos int64, slen int64) int64 {
	if pos >= 0 {
		return pos
	}
	if uint64(-pos) > uint64(slen) {
		return 0
	}
	return slen + pos + 1
}

// utf8Decode decodes one UTF-8 sequence starting at s[pos].
// Returns the decoded code point and number of bytes consumed, or ok=false on error.
// If strict is true, rejects surrogates and values > MAXUNICODE.
func utf8Decode(s string, pos int, strict bool) (code uint32, size int, ok bool) {
	if pos >= len(s) {
		return 0, 0, false
	}
	c := s[pos]
	if c < 0x80 { // ASCII
		return uint32(c), 1, true
	}
	if c >= 0xFE { // would need 6+ continuation bytes
		return 0, 0, false
	}

	var limits = [6]uint32{^uint32(0), 0x80, 0x800, 0x10000, 0x200000, 0x4000000}
	var res uint32
	count := 0
	cb := c
	for cb&0x40 != 0 { // while it needs continuation bytes
		count++
		if pos+count >= len(s) {
			return 0, 0, false
		}
		cc := s[pos+count]
		if !isCont(cc) {
			return 0, 0, false
		}
		res = (res << 6) | uint32(cc&0x3F)
		cb <<= 1
	}
	if count > 5 {
		return 0, 0, false
	}
	res |= uint32(cb&0x7F) << (uint(count) * 5)
	if res > maxUTF || res < limits[count] {
		return 0, 0, false
	}
	if strict {
		if res > maxUnicode || (0xD800 <= res && res <= 0xDFFF) {
			return 0, 0, false
		}
	}
	return res, count + 1, true
}

// utfLen implements utf8.len(s [, i [, j [, lax]]])
// Returns number of characters in range [i,j], or nil + position on invalid.
func utfLen(L *luaapi.State) int {
	s := L.CheckString(1)
	slen := int64(len(s))
	posi := uPosRelat(L.OptInteger(2, 1), slen)
	posj := uPosRelat(L.OptInteger(3, -1), slen)
	lax := L.ToBoolean(4)

	L.ArgCheck(1 <= posi && posi-1 <= slen, 2, "initial position out of bounds")
	posi-- // convert to 0-based
	L.ArgCheck(posj-1 < slen, 3, "final position out of bounds")
	posj-- // convert to 0-based

	var n int64
	for posi <= posj {
		_, size, ok := utf8Decode(s, int(posi), !lax)
		if !ok {
			L.PushFail()
			L.PushInteger(posi + 1) // back to 1-based
			return 2
		}
		posi += int64(size)
		n++
	}
	L.PushInteger(n)
	return 1
}

// utfCodepoint implements utf8.codepoint(s [, i [, j [, lax]]])
// Returns codepoints for all characters starting in [i,j].
func utfCodepoint(L *luaapi.State) int {
	s := L.CheckString(1)
	slen := int64(len(s))
	posi := uPosRelat(L.OptInteger(2, 1), slen)
	pose := uPosRelat(L.OptInteger(3, posi), slen)
	lax := L.ToBoolean(4)

	L.ArgCheck(posi >= 1, 2, "out of bounds")
	L.ArgCheck(pose <= slen, 3, "out of bounds")
	if posi > pose {
		return 0 // empty interval
	}
	n := int(pose - posi + 1)
	L.CheckStack(n)

	count := 0
	p := int(posi - 1) // 0-based
	se := int(pose)     // 0-based end (exclusive after converting from 1-based inclusive)
	for p < se {
		code, size, ok := utf8Decode(s, p, !lax)
		if !ok {
			L.PushFString("invalid UTF-8 code")
			L.Error()
			return 0
		}
		L.PushInteger(int64(code))
		count++
		p += size
	}
	return count
}

// encodeUTF8 encodes a single code point as UTF-8 bytes.
func encodeUTF8(code uint32) string {
	if code <= maxUnicode {
		// Use Go's standard library for valid Unicode
		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], rune(code))
		return string(buf[:n])
	}
	// For non-strict mode (code > maxUnicode, up to maxUTF),
	// manually encode using the extended UTF-8 scheme.
	var buf [6]byte
	if code < 0x80 {
		buf[0] = byte(code)
		return string(buf[:1])
	}
	mfb := 0x3F // max value for first byte payload
	n := 1
	for code > uint32(mfb) {
		buf[n] = byte(0x80 | (code & 0x3F))
		code >>= 6
		n++
		mfb >>= 1
	}
	buf[0] = byte((^mfb << 1) | int(code))
	// Reverse bytes 1..n-1 (they were written in reverse order)
	for i, j := 1, n-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf[:n])
}

// utfChar implements utf8.char(n1, n2, ...)
// Converts codepoints to UTF-8 string.
func utfChar(L *luaapi.State) int {
	n := L.GetTop()
	if n == 1 {
		code := uint64(L.CheckInteger(1))
		L.ArgCheck(code <= maxUTF, 1, "value out of range")
		L.PushString(encodeUTF8(uint32(code)))
	} else {
		var result []byte
		for i := 1; i <= n; i++ {
			code := uint64(L.CheckInteger(i))
			L.ArgCheck(code <= maxUTF, i, "value out of range")
			result = append(result, encodeUTF8(uint32(code))...)
		}
		L.PushString(string(result))
	}
	return 1
}

// utfOffset implements utf8.offset(s, n [, i])
// Returns byte position where n-th character (from position i) starts.
func utfOffset(L *luaapi.State) int {
	s := L.CheckString(1)
	slen := int64(len(s))
	n := L.CheckInteger(2)

	// Default i: if n >= 0 then 1, else len+1
	var defI int64
	if n >= 0 {
		defI = 1
	} else {
		defI = slen + 1
	}
	posi := uPosRelat(L.OptInteger(3, defI), slen)
	L.ArgCheck(1 <= posi && posi-1 <= slen, 3, "position out of bounds")
	posi-- // convert to 0-based

	if n == 0 {
		// Find beginning of current byte sequence
		for posi > 0 && isCont(s[posi]) {
			posi--
		}
	} else {
		if posi < slen && isCont(s[posi]) {
			L.PushFString("initial position is a continuation byte")
			L.Error()
			return 0
		}
		if n < 0 {
			for n < 0 && posi > 0 {
				posi--
				for posi > 0 && isCont(s[posi]) {
					posi--
				}
				n++
			}
		} else {
			n-- // do not move for 1st character
			for n > 0 && posi < slen {
				posi++
				for posi < slen && isCont(s[posi]) {
					posi++
				}
				n--
			}
		}
	}
	if n != 0 { // did not find given character
		L.PushFail()
		return 1
	}
	L.PushInteger(posi + 1) // initial position (1-based)
	// Also return end position of this character
	endPos := posi
	if posi < slen && (s[posi]&0x80) != 0 { // multi-byte character?
		if isCont(s[posi]) {
			L.PushFString("initial position is a continuation byte")
			L.Error()
			return 0
		}
		for endPos+1 < slen && isCont(s[endPos+1]) {
			endPos++ // skip to last continuation byte
		}
	}
	// else one-byte character: final position is the initial one
	L.PushInteger(endPos + 1) // final position (1-based)
	return 2
}

// iterAux is the core iterator function for utf8.codes.
// Matches C Lua's iter_aux exactly:
// - arg 1: the string s
// - arg 2: the byte offset from previous iteration (0 initially, then 1-based positions)
// The iterator skips continuation bytes to find the next character,
// decodes it, and returns (1-based position, codepoint).
func iterAux(L *luaapi.State, strict bool) int {
	s := L.CheckString(1)
	slen := len(s)
	n, _ := L.ToInteger(2)

	// C Lua casts to lua_Unsigned; negative values become huge → >= len → return 0
	if n < 0 || int(n) >= slen {
		return 0
	}
	pos := int(n)

	// Skip continuation bytes to find next character start
	for pos < slen && isCont(s[pos]) {
		pos++
	}
	if pos >= slen { // no more codepoints
		return 0
	}

	code, size, ok := utf8Decode(s, pos, strict)
	if !ok {
		L.PushFString("invalid UTF-8 code")
		L.Error()
		return 0
	}
	// Check that the byte after decoded sequence is not a continuation byte
	// (catches overlong sequences that utf8Decode might miss)
	nextPos := pos + size
	if nextPos < slen && isCont(s[nextPos]) {
		L.PushFString("invalid UTF-8 code")
		L.Error()
		return 0
	}
	L.PushInteger(int64(pos) + 1) // 1-based position
	L.PushInteger(int64(code))
	return 2
}

func iterAuxStrict(L *luaapi.State) int {
	return iterAux(L, true)
}

func iterAuxLax(L *luaapi.State) int {
	return iterAux(L, false)
}

// utfCodes implements utf8.codes(s [, lax])
// Returns an iterator function, the string, and 0.
func utfCodes(L *luaapi.State) int {
	lax := L.ToBoolean(2)
	s := L.CheckString(1)
	L.ArgCheck(len(s) == 0 || !isCont(s[0]), 1, "invalid UTF-8 code")
	if lax {
		L.PushCFunction(iterAuxLax)
	} else {
		L.PushCFunction(iterAuxStrict)
	}
	L.PushValue(1) // string
	L.PushInteger(0)
	return 3
}

// runeDisplayWidth returns the terminal display width of a rune.
// CJK/fullwidth characters are 2 columns; most others are 1.
func runeDisplayWidth(r rune) int {
	// East Asian Wide/Fullwidth ranges
	if (r >= 0x1100 && r <= 0x115F) || // Hangul Jamo
		r == 0x2329 || r == 0x232A ||
		(r >= 0x2E80 && r <= 0x303E) || // CJK Radicals..CJK Symbols
		(r >= 0x3040 && r <= 0x33BF) || // Hiragana..CJK Compatibility
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Unified Ext A
		(r >= 0x4E00 && r <= 0xA4CF) || // CJK Unified..Yi Radicals
		(r >= 0xA960 && r <= 0xA97C) || // Hangul Jamo Extended-A
		(r >= 0xAC00 && r <= 0xD7A3) || // Hangul Syllables
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0xFE10 && r <= 0xFE19) || // Vertical forms
		(r >= 0xFE30 && r <= 0xFE6F) || // CJK Compatibility Forms
		(r >= 0xFF01 && r <= 0xFF60) || // Fullwidth Forms
		(r >= 0xFFE0 && r <= 0xFFE6) || // Fullwidth Signs
		(r >= 0x1F300 && r <= 0x1F9FF) || // Emoji
		(r >= 0x20000 && r <= 0x2FFFD) || // CJK Unified Ext B..
		(r >= 0x30000 && r <= 0x3FFFD) { // CJK Unified Ext G..
		return 2
	}
	return 1
}

// utfWidth implements utf8.width(s [, i [, j]])
// Returns the display width of string s (or substring s[i..j]) in terminal columns.
// CJK/fullwidth characters count as 2, others as 1.
// i and j are byte positions (1-based, like string.sub).
func utfWidth(L *luaapi.State) int {
	s := L.CheckString(1)
	slen := int64(len(s))
	posi := int(uPosRelat(L.OptInteger(2, 1), slen)) - 1 // 0-based
	posj := int(uPosRelat(L.OptInteger(3, -1), slen)) - 1 // 0-based

	if posi < 0 {
		posi = 0
	}
	if posj >= len(s) {
		posj = len(s) - 1
	}

	width := 0
	for posi <= posj {
		code, size, ok := utf8Decode(s, posi, false)
		if !ok {
			posi++
			width++
			continue
		}
		width += runeDisplayWidth(rune(code))
		posi += size
	}
	L.PushInteger(int64(width))
	return 1
}

// utfSub implements utf8.sub(s, i [, j])
// Returns substring by codepoint indices (1-based, inclusive).
// Negative indices count from the end (-1 = last codepoint).
func utfSub(L *luaapi.State) int {
	s := L.CheckString(1)
	cpLen := int64(utf8.RuneCountInString(s))
	i := uPosRelat(L.CheckInteger(2), cpLen)
	j := uPosRelat(L.OptInteger(3, -1), cpLen)

	if i < 1 {
		i = 1
	}
	if j > cpLen {
		j = cpLen
	}
	if i > j {
		L.PushString("")
		return 1
	}

	// Find byte position for codepoint index i (1-based)
	byteStart := 0
	count := int64(1)
	pos := 0
	for pos < len(s) && count < i {
		_, size := utf8.DecodeRuneInString(s[pos:])
		pos += size
		count++
	}
	byteStart = pos

	// Find byte position for end of codepoint index j
	for pos < len(s) && count <= j {
		_, size := utf8.DecodeRuneInString(s[pos:])
		pos += size
		count++
	}
	byteEnd := pos

	L.PushString(s[byteStart:byteEnd])
	return 1
}

// OpenUTF8 opens the utf8 library.
func OpenUTF8(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{
		"offset":    utfOffset,
		"codepoint": utfCodepoint,
		"char":      utfChar,
		"len":       utfLen,
		"codes":     utfCodes,
		"width":     utfWidth,
		"sub":       utfSub,
	})
	// utf8.charpattern — pattern matching a single UTF-8 character
	// Mirrors: UTF8PATT in lutf8lib.c: "[\0-\x7F\xC2-\xFD][\x80-\xBF]*"
	L.PushString("[\x00-\x7F\xC2-\xFD][\x80-\xBF]*")
	L.SetField(-2, "charpattern")
	return 1
}
