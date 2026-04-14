package api

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	luaapi "github.com/akzj/go-lua/internal/api/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
)

// ---------------------------------------------------------------------------
// String library
// Reference: lua-master/lstrlib.c
// ---------------------------------------------------------------------------

// posRelat converts a relative position (negative = from end) to absolute.
// Lua strings are 1-based.
func posRelat(pos int64, length int) int {
	if pos >= 0 {
		return int(pos)
	}
	if -pos > int64(length) {
		return 0
	}
	return length + int(pos) + 1
}

func str_byte(L *luaapi.State) int {
	s := L.CheckString(1)
	i := posRelat(L.OptInteger(2, 1), len(s))
	j := posRelat(L.OptInteger(3, int64(i)), len(s))
	if i < 1 {
		i = 1
	}
	if j > len(s) {
		j = len(s)
	}
	if i > j {
		return 0
	}
	n := j - i + 1
	L.CheckStack(n)
	for k := i; k <= j; k++ {
		L.PushInteger(int64(s[k-1]))
	}
	return n
}

func str_char(L *luaapi.State) int {
	n := L.GetTop()
	buf := make([]byte, n)
	for i := 1; i <= n; i++ {
		c := L.CheckInteger(i)
		L.ArgCheck(c >= 0 && c <= 255, i, "value out of range")
		buf[i-1] = byte(c)
	}
	L.PushString(string(buf))
	return 1
}

func str_len(L *luaapi.State) int {
	s := L.CheckString(1)
	L.PushInteger(int64(len(s)))
	return 1
}

func str_lower(L *luaapi.State) int {
	s := L.CheckString(1)
	L.PushString(strings.ToLower(s))
	return 1
}

func str_upper(L *luaapi.State) int {
	s := L.CheckString(1)
	L.PushString(strings.ToUpper(s))
	return 1
}

func str_rep(L *luaapi.State) int {
	s := L.CheckString(1)
	n := L.CheckInteger(2)
	sep := L.OptString(3, "")
	if n <= 0 {
		L.PushString("")
		return 1
	}
	ls := int64(len(s))
	lsep := int64(len(sep))
	// Check for overflow: total = n*ls + (n-1)*lsep
	const maxSize = 1 << 30 // 1GB limit
	if ls+lsep > 0 && n > maxSize/(ls+lsep) {
		L.Errorf("resulting string too large")
	}
	if n == 1 {
		L.PushString(s)
		return 1
	}
	// Build with separator
	var sb strings.Builder
	total := n*ls + (n-1)*lsep
	if total > maxSize {
		L.Errorf("resulting string too large")
	}
	sb.Grow(int(total))
	for i := int64(0); i < n; i++ {
		if i > 0 && sep != "" {
			sb.WriteString(sep)
		}
		sb.WriteString(s)
	}
	L.PushString(sb.String())
	return 1
}

func str_reverse(L *luaapi.State) int {
	s := L.CheckString(1)
	bs := []byte(s)
	for i, j := 0, len(bs)-1; i < j; i, j = i+1, j-1 {
		bs[i], bs[j] = bs[j], bs[i]
	}
	L.PushString(string(bs))
	return 1
}

func str_sub(L *luaapi.State) int {
	s := L.CheckString(1)
	l := len(s)
	i := posRelat(L.CheckInteger(2), l)
	j := posRelat(L.OptInteger(3, -1), l)
	if i < 1 {
		i = 1
	}
	if j > l {
		j = l
	}
	if i > j {
		L.PushString("")
	} else {
		L.PushString(s[i-1 : j])
	}
	return 1
}

// ---------------------------------------------------------------------------
// string.format — simplified printf-style formatting
// ---------------------------------------------------------------------------

func str_format(L *luaapi.State) int {
	fmtStr := L.CheckString(1)
	arg := 1 // current argument index (starts at 2)
	var sb strings.Builder
	i := 0
	for i < len(fmtStr) {
		if fmtStr[i] != '%' {
			sb.WriteByte(fmtStr[i])
			i++
			continue
		}
		i++ // skip '%'
		if i >= len(fmtStr) {
			break
		}
		if fmtStr[i] == '%' {
			sb.WriteByte('%')
			i++
			continue
		}
		// Parse format specifier: flags, width, precision, conversion
		start := i - 1 // include the '%'
		// Skip flags
		for i < len(fmtStr) && strings.ContainsRune("-+ #0", rune(fmtStr[i])) {
			i++
		}
		// Skip width
		for i < len(fmtStr) && fmtStr[i] >= '0' && fmtStr[i] <= '9' {
			i++
		}
		// Skip precision
		if i < len(fmtStr) && fmtStr[i] == '.' {
			i++
			for i < len(fmtStr) && fmtStr[i] >= '0' && fmtStr[i] <= '9' {
				i++
			}
		}
		if i >= len(fmtStr) {
			break
		}
		conv := fmtStr[i]
		i++
		spec := fmtStr[start:i]
		arg++

		switch conv {
		case 'd', 'i':
			n := L.CheckInteger(arg)
			sb.WriteString(fmt.Sprintf(spec, n))
		case 'u':
			n := L.CheckInteger(arg)
			// Replace %u with %d for Go (Lua integers are signed)
			goSpec := strings.Replace(spec, "u", "d", 1)
			if n < 0 {
				// Treat as unsigned
				sb.WriteString(fmt.Sprintf(goSpec, uint64(n)))
			} else {
				sb.WriteString(fmt.Sprintf(goSpec, n))
			}
		case 'f', 'F', 'e', 'E', 'g', 'G':
			n := L.CheckNumber(arg)
			sb.WriteString(fmt.Sprintf(spec, n))
		case 'x', 'X':
			n := L.CheckInteger(arg)
			sb.WriteString(fmt.Sprintf(spec, n))
		case 'o':
			n := L.CheckInteger(arg)
			sb.WriteString(fmt.Sprintf(spec, n))
		case 'c':
			n := L.CheckInteger(arg)
			sb.WriteByte(byte(n))
		case 's':
			s := L.TolString(arg)
			L.Pop(1) // pop string from TolString
			// Check if we need to apply format (width/precision)
			if spec == "%s" {
				sb.WriteString(s)
			} else {
				sb.WriteString(fmt.Sprintf(spec, s))
			}
		case 'q':
			s := L.CheckString(arg)
			sb.WriteString(quoteString(s))
		case 'p':
			// pointer representation — mirrors luaO_pushfstring %p
			// nil, boolean, number, string → "(null)"
			// table, function, userdata, thread → "0x..." hex address
			v := L.Type(arg)
			var pstr string
			switch v {
			case objectapi.TypeNil, objectapi.TypeBoolean, objectapi.TypeNumber, objectapi.TypeString:
				pstr = "(null)"
			default:
				ptr := L.ToPointer(arg)
				if ptr == "" {
					pstr = "(null)"
				} else {
					pstr = ptr
				}
			}
			// Apply width/padding from format spec
			if spec != "%p" {
				fmtS := strings.Replace(spec, "p", "s", 1)
				pstr = fmt.Sprintf(fmtS, pstr)
			}
			sb.WriteString(pstr)
		default:
			sb.WriteString(spec) // unknown, pass through
		}
	}
	L.PushString(sb.String())
	return 1
}

func quoteString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			sb.WriteString("\\\\")
		case '"':
			sb.WriteString("\\\"")
		case '\n':
			sb.WriteString("\\n")
		case '\r':
			sb.WriteString("\\r")
		case '\x00':
			sb.WriteString("\\0")
		case '\x1a': // ^Z
			sb.WriteString("\\26")
		default:
			if c < ' ' {
				sb.WriteString(fmt.Sprintf("\\%d", c))
			} else {
				sb.WriteByte(c)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// ---------------------------------------------------------------------------
// Pattern matching engine — Lua patterns (NOT regex)
// ---------------------------------------------------------------------------

// matchState holds pattern matching state
type matchState struct {
	src     string
	pat     string
	level   int // number of active captures
	capture [32]captureInfo
}

type captureInfo struct {
	init int // start position
	len  int // length (-1 = position capture, -2 = unfinished)
}

const (
	capUnfinished = -1
	capPosition   = -2
)

func classEnd(pat string, p int) int {
	c := pat[p]
	p++
	switch c {
	case '%':
		if p >= len(pat) {
			return p
		}
		return p + 1
	case '[':
		if p < len(pat) && pat[p] == '^' {
			p++
		}
		// skip until closing ]
		for {
			if p >= len(pat) {
				return p
			}
			c = pat[p]
			p++
			if c == '%' && p < len(pat) {
				p++ // skip escaped char
			} else if c == ']' {
				return p
			}
		}
	default:
		return p
	}
}

func matchClass(c byte, cl byte) bool {
	var res bool
	lcl := cl | 0x20 // lowercase
	switch lcl {
	case 'a':
		res = (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
	case 'c':
		res = c < 32 || c == 127
	case 'd':
		res = c >= '0' && c <= '9'
	case 'l':
		res = c >= 'a' && c <= 'z'
	case 'p':
		res = unicode.IsPunct(rune(c))
	case 's':
		res = c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
	case 'u':
		res = c >= 'A' && c <= 'Z'
	case 'w':
		res = (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
	case 'x':
		res = (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
	default:
		return c == cl
	}
	if cl >= 'A' && cl <= 'Z' { // uppercase = complement
		return !res
	}
	return res
}

func singlematch(ms *matchState, si int, pi int, ep int) bool {
	if si >= len(ms.src) {
		return false
	}
	c := ms.src[si]
	p := ms.pat[pi]
	switch p {
	case '.':
		return true
	case '%':
		if pi+1 < len(ms.pat) {
			return matchClass(c, ms.pat[pi+1])
		}
		return false
	case '[':
		// match character set
		ep2 := ep - 1 // ep points past ']'
		pi++
		negate := false
		if pi < ep2 && ms.pat[pi] == '^' {
			negate = true
			pi++
		}
		found := false
		for pi < ep2 {
			if ms.pat[pi] == '%' && pi+1 < ep2 {
				pi++
				if matchClass(c, ms.pat[pi]) {
					found = true
				}
				pi++
			} else if pi+2 < ep2 && ms.pat[pi+1] == '-' {
				if c >= ms.pat[pi] && c <= ms.pat[pi+2] {
					found = true
				}
				pi += 3
			} else {
				if c == ms.pat[pi] {
					found = true
				}
				pi++
			}
		}
		if negate {
			return !found
		}
		return found
	default:
		return c == p
	}
}

func (ms *matchState) matchBalance(si, pi int) int {
	if pi >= len(ms.pat)-1 {
		return -1
	}
	if si >= len(ms.src) || ms.src[si] != ms.pat[pi] {
		return -1
	}
	b := ms.pat[pi]
	e := ms.pat[pi+1]
	cont := 1
	for si++; si < len(ms.src); si++ {
		if ms.src[si] == e {
			cont--
			if cont == 0 {
				return si + 1
			}
		} else if ms.src[si] == b {
			cont++
		}
	}
	return -1
}

func (ms *matchState) match(si, pi int) int {
	for pi < len(ms.pat) {
		switch ms.pat[pi] {
		case '(':
			if pi+1 < len(ms.pat) && ms.pat[pi+1] == ')' {
				return ms.matchCapture(si, pi+2, capPosition)
			}
			return ms.matchCapture(si, pi+1, capUnfinished)
		case ')':
			return ms.matchClose(si, pi+1)
		case '$':
			if pi+1 == len(ms.pat) {
				if si == len(ms.src) {
					return si
				}
				return -1
			}
			// '$' is literal if not at end of pattern
			goto dflt
		case '%':
			if pi+1 < len(ms.pat) && ms.pat[pi+1] == 'b' {
				s := ms.matchBalance(si, pi+2)
				if s >= 0 {
					pi = pi + 4
					si = s
					continue
				}
				return -1
			}
			if pi+1 < len(ms.pat) && ms.pat[pi+1] == 'f' {
				// frontier pattern
				pi += 2
				ep := classEnd(ms.pat, pi)
				var prev byte
				if si > 0 {
					prev = ms.src[si-1]
				}
				var cur byte
				if si < len(ms.src) {
					cur = ms.src[si]
				}
				if !singlematch(ms, si-1, pi, ep) || singlematch(ms, si, pi, ep) {
					// Check: prev NOT in set, cur IN set
					_ = prev
					_ = cur
				}
				// Simplified: just check the boundary
				if singlematchByte(ms, prev, pi, ep) || !singlematchByte(ms, cur, pi, ep) {
					return -1
				}
				pi = ep
				continue
			}
			goto dflt
		default:
			goto dflt
		}
	dflt:
		ep := classEnd(ms.pat, pi)
		// Check for repetition
		if ep < len(ms.pat) {
			switch ms.pat[ep] {
			case '?':
				if singlematch(ms, si, pi, ep) {
					if res := ms.match(si+1, ep+1); res >= 0 {
						return res
					}
				}
				pi = ep + 1
				continue
			case '*':
				return ms.matchStar(si, pi, ep)
			case '+':
				if singlematch(ms, si, pi, ep) {
					return ms.matchStar(si+1, pi, ep)
				}
				return -1
			case '-':
				return ms.matchMinus(si, pi, ep)
			}
		}
		// No repetition
		if !singlematch(ms, si, pi, ep) {
			return -1
		}
		si++
		pi = ep
	}
	return si
}

func singlematchByte(ms *matchState, c byte, pi int, ep int) bool {
	p := ms.pat[pi]
	switch p {
	case '.':
		return true
	case '%':
		if pi+1 < len(ms.pat) {
			return matchClass(c, ms.pat[pi+1])
		}
		return false
	case '[':
		ep2 := ep - 1
		pi++
		negate := false
		if pi < ep2 && ms.pat[pi] == '^' {
			negate = true
			pi++
		}
		found := false
		for pi < ep2 {
			if ms.pat[pi] == '%' && pi+1 < ep2 {
				pi++
				if matchClass(c, ms.pat[pi]) {
					found = true
				}
				pi++
			} else if pi+2 < ep2 && ms.pat[pi+1] == '-' {
				if c >= ms.pat[pi] && c <= ms.pat[pi+2] {
					found = true
				}
				pi += 3
			} else {
				if c == ms.pat[pi] {
					found = true
				}
				pi++
			}
		}
		if negate {
			return !found
		}
		return found
	default:
		return c == p
	}
}

func (ms *matchState) matchStar(si, pi, ep int) int {
	// greedy: match as many as possible, then backtrack
	i := si
	for singlematch(ms, i, pi, ep) {
		i++
	}
	for i >= si {
		if res := ms.match(i, ep+1); res >= 0 {
			return res
		}
		i--
	}
	return -1
}

func (ms *matchState) matchMinus(si, pi, ep int) int {
	// lazy: try 0 matches first, then expand
	for {
		if res := ms.match(si, ep+1); res >= 0 {
			return res
		}
		if !singlematch(ms, si, pi, ep) {
			return -1
		}
		si++
	}
}

func (ms *matchState) matchCapture(si, pi int, what int) int {
	level := ms.level
	if level >= 32 {
		return -1
	}
	ms.capture[level].init = si
	ms.capture[level].len = what
	ms.level = level + 1
	res := ms.match(si, pi)
	if res < 0 {
		ms.level = level // undo capture
	}
	return res
}

func (ms *matchState) matchClose(si, pi int) int {
	level := ms.level - 1
	for level >= 0 && ms.capture[level].len != capUnfinished {
		level--
	}
	if level < 0 {
		return -1
	}
	ms.capture[level].len = si - ms.capture[level].init
	res := ms.match(si, pi)
	if res < 0 {
		ms.capture[level].len = capUnfinished // undo
	}
	return res
}

func (ms *matchState) pushCapture(L *luaapi.State, si, ei int) int {
	nlevels := ms.level
	if nlevels == 0 {
		// no captures: push whole match
		L.PushString(ms.src[si:ei])
		return 1
	}
	for i := 0; i < nlevels; i++ {
		if ms.capture[i].len == capPosition {
			L.PushInteger(int64(ms.capture[i].init + 1)) // 1-based
		} else {
			start := ms.capture[i].init
			end := start + ms.capture[i].len
			if end > len(ms.src) {
				end = len(ms.src)
			}
			L.PushString(ms.src[start:end])
		}
	}
	return nlevels
}

func str_find(L *luaapi.State) int {
	return str_find_aux(L, true)
}

func str_match(L *luaapi.State) int {
	return str_find_aux(L, false)
}

func str_find_aux(L *luaapi.State, find bool) int {
	s := L.CheckString(1)
	p := L.CheckString(2)
	init := posRelat(L.OptInteger(3, 1), len(s))
	if init < 1 {
		init = 1
	} else if init > len(s)+1 {
		// init past end of string — no match possible
		L.PushNil()
		return 1
	}

	// Plain find?
	if find && (L.ToBoolean(4) || !hasPatternSpecials(p)) {
		// plain search
		idx := strings.Index(s[init-1:], p)
		if idx < 0 {
			L.PushNil()
			return 1
		}
		L.PushInteger(int64(init + idx))
		L.PushInteger(int64(init + idx + len(p) - 1))
		return 2
	}

	// Pattern search
	anchor := false
	pat := p
	if len(pat) > 0 && pat[0] == '^' {
		anchor = true
		pat = pat[1:]
	}

	ms := &matchState{src: s, pat: pat}
	si := init - 1
	for {
		ms.level = 0
		res := ms.match(si, 0)
		if res >= 0 {
			if find {
				L.PushInteger(int64(si + 1))
				L.PushInteger(int64(res))
				return ms.pushCapture(L, si, res) + 2
			}
			return ms.pushCapture(L, si, res)
		}
		si++
		if anchor || si >= len(s) {
			break
		}
	}
	L.PushNil()
	return 1
}

func hasPatternSpecials(p string) bool {
	for _, c := range p {
		switch c {
		case '^', '$', '*', '+', '-', '?', '.', '(', ')', '[', ']', '%':
			return true
		}
	}
	return false
}

// gmatch — returns an iterator function
func str_gmatch(L *luaapi.State) int {
	s := L.CheckString(1)
	p := L.CheckString(2)
	init := posRelat(L.OptInteger(3, 1), len(s)) - 1
	if init < 0 {
		init = 0
	}

	anchor := false
	pat := p
	if len(pat) > 0 && pat[0] == '^' {
		anchor = true
		pat = pat[1:]
	}

	// Capture state in upvalues via closure
	pos := init
	done := false

	iter := func(L *luaapi.State) int {
		if done {
			return 0
		}
		ms := &matchState{src: s, pat: pat}
		for pos <= len(s) {
			ms.level = 0
			res := ms.match(pos, 0)
			if res >= 0 {
				si := pos
				if res == pos {
					pos++ // empty match: advance
				} else {
					pos = res
				}
				return ms.pushCapture(L, si, res)
			}
			if anchor {
				break
			}
			pos++
		}
		done = true
		return 0
	}

	L.PushCFunction(func(L *luaapi.State) int {
		return iter(L)
	})
	return 1
}

// gsub — global substitution
func str_gsub(L *luaapi.State) int {
	s := L.CheckString(1)
	p := L.CheckString(2)
	maxn := int(L.OptInteger(4, int64(len(s)+1)))

	// repl can be string, table, or function
	replType := L.Type(3)

	anchor := false
	pat := p
	if len(pat) > 0 && pat[0] == '^' {
		anchor = true
		pat = pat[1:]
	}

	var sb strings.Builder
	n := 0
	si := 0
	for n < maxn {
		ms := &matchState{src: s, pat: pat}
		ms.level = 0
		res := ms.match(si, 0)
		if res < 0 {
			if anchor || si >= len(s) {
				break
			}
			sb.WriteByte(s[si])
			si++
			continue
		}
		n++
		// Get replacement
		switch replType {
		case objectapi.TypeString:
			repl := L.CheckString(3)
			sb.WriteString(gsubReplace(repl, ms, si, res))
		case objectapi.TypeTable:
			// Use first capture (or whole match) as key
			ms.pushCapture(L, si, res)
			L.GetTable(3) // table[capture]
			addReplacement(L, &sb, s, si, res)
		case objectapi.TypeFunction:
			nCap := ms.pushCapture(L, si, res)
			L.PushValue(3) // push function
			L.Insert(-(nCap + 1))
			L.Call(nCap, 1)
			addReplacement(L, &sb, s, si, res)
		default:
			L.ArgError(3, "string/function/table expected")
		}
		if res == si {
			if si < len(s) {
				sb.WriteByte(s[si])
			}
			si++
		} else {
			si = res
		}
		if anchor {
			break
		}
	}
	// Append remainder
	if si <= len(s) {
		sb.WriteString(s[si:])
	}
	L.PushString(sb.String())
	L.PushInteger(int64(n))
	return 2
}

func addReplacement(L *luaapi.State, sb *strings.Builder, s string, si, ei int) {
	// Value is on top of stack
	if !L.ToBoolean(-1) {
		L.Pop(1)
		sb.WriteString(s[si:ei]) // keep original
	} else {
		r, ok := L.ToString(-1)
		if !ok {
			L.Errorf("invalid replacement value (a %s)", L.TypeName(L.Type(-1)))
		}
		sb.WriteString(r)
		L.Pop(1)
	}
}

func gsubReplace(repl string, ms *matchState, si, ei int) string {
	var sb strings.Builder
	for i := 0; i < len(repl); i++ {
		c := repl[i]
		if c != '%' {
			sb.WriteByte(c)
			continue
		}
		i++
		if i >= len(repl) {
			break
		}
		c = repl[i]
		if c >= '0' && c <= '9' {
			idx := int(c - '0')
			if idx == 0 {
				// %0 = whole match
				sb.WriteString(ms.src[si:ei])
			} else if idx <= ms.level {
				cap := ms.capture[idx-1]
				if cap.len >= 0 {
					sb.WriteString(ms.src[cap.init : cap.init+cap.len])
				}
			}
		} else if c == '%' {
			sb.WriteByte('%')
		} else {
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// packOptSize returns the byte size for a single pack format character.
// Matches C Lua's getdetails() in lstrlib.c.
// On 64-bit: lua_Integer=int64(8), lua_Number=float64(8), size_t=8, int=4, short=2, long=8.
func packOptSize(fmt byte) int {
	switch fmt {
	case 'b', 'B':
		return 1
	case 'h', 'H':
		return 2
	case 'l', 'L':
		return 8 // sizeof(long) on 64-bit
	case 'j', 'J':
		return 8 // sizeof(lua_Integer)
	case 'T':
		return 8 // sizeof(size_t)
	case 'f':
		return 4 // sizeof(float)
	case 'n':
		return 8 // sizeof(lua_Number)
	case 'd':
		return 8 // sizeof(double)
	default:
		return 0 // unknown
	}
}

// str_packsize implements string.packsize(fmt).
func str_packsize(L *luaapi.State) int {
	fmtStr := L.CheckString(1)
	total := 0
	i := 0
	for i < len(fmtStr) {
		c := fmtStr[i]
		i++
		switch {
		case c == '<' || c == '>' || c == '=' || c == '!':
			// endianness/alignment markers — skip
			continue
		case c >= '0' && c <= '9':
			// skip digits (alignment specifier after !)
			continue
		case c == ' ':
			continue
		case c == 'x':
			total++ // one padding byte
		case c == 'X':
			// empty alignment item — skip
			continue
		case c == 'i', c == 'I':
			// i[n] / I[n] — optional size follows
			sz := 4 // default = sizeof(int)
			if i < len(fmtStr) && fmtStr[i] >= '1' && fmtStr[i] <= '9' {
				sz = int(fmtStr[i] - '0')
				i++
				for i < len(fmtStr) && fmtStr[i] >= '0' && fmtStr[i] <= '9' {
					sz = sz*10 + int(fmtStr[i]-'0')
					i++
				}
			}
			total += sz
		case c == 's':
			L.ArgError(1, "variable-length format")
			return 0
		case c == 'z':
			L.ArgError(1, "variable-length format")
			return 0
		default:
			sz := packOptSize(c)
			if sz == 0 {
				L.ArgError(1, fmt.Sprintf("invalid format option '%c'", c))
				return 0
			}
			total += sz
		}
	}
	L.PushInteger(int64(total))
	return 1
}

// str_pack implements string.pack(fmt, v1, v2, ...).
func str_pack(L *luaapi.State) int {
	fmtStr := L.CheckString(1)
	var buf []byte
	arg := 2
	i := 0
	isLittle := true // default: native (assume little-endian)
	for i < len(fmtStr) {
		c := fmtStr[i]
		i++
		switch {
		case c == '<':
			isLittle = true
			continue
		case c == '>':
			isLittle = false
			continue
		case c == '=', c == '!':
			continue
		case c >= '0' && c <= '9', c == ' ':
			continue
		case c == 'x':
			buf = append(buf, 0)
		case c == 'b':
			v := L.CheckInteger(arg)
			arg++
			buf = append(buf, byte(int8(v)))
		case c == 'B':
			v := L.CheckInteger(arg)
			arg++
			buf = append(buf, byte(v))
		case c == 'i', c == 'I':
			sz := 4
			if i < len(fmtStr) && fmtStr[i] >= '1' && fmtStr[i] <= '9' {
				sz = int(fmtStr[i] - '0')
				i++
				for i < len(fmtStr) && fmtStr[i] >= '0' && fmtStr[i] <= '9' {
					sz = sz*10 + int(fmtStr[i]-'0')
					i++
				}
			}
			v := L.CheckInteger(arg)
			arg++
			buf = appendInt(buf, uint64(v), sz, isLittle)
		case c == 'h', c == 'H':
			v := L.CheckInteger(arg)
			arg++
			buf = appendInt(buf, uint64(v), 2, isLittle)
		case c == 'l', c == 'L':
			v := L.CheckInteger(arg)
			arg++
			buf = appendInt(buf, uint64(v), 8, isLittle)
		case c == 'j', c == 'J':
			v := L.CheckInteger(arg)
			arg++
			buf = appendInt(buf, uint64(v), 8, isLittle)
		case c == 'f':
			v := float32(L.CheckNumber(arg))
			arg++
			bits := math.Float32bits(v)
			buf = appendInt(buf, uint64(bits), 4, isLittle)
		case c == 'd', c == 'n':
			v := L.CheckNumber(arg)
			arg++
			bits := math.Float64bits(v)
			buf = appendInt(buf, bits, 8, isLittle)
		case c == 's':
			sz := 8 // default s size
			if i < len(fmtStr) && fmtStr[i] >= '1' && fmtStr[i] <= '9' {
				sz = int(fmtStr[i] - '0')
				i++
			}
			s := L.CheckString(arg)
			arg++
			buf = appendInt(buf, uint64(len(s)), sz, isLittle)
			buf = append(buf, s...)
		case c == 'z':
			s := L.CheckString(arg)
			arg++
			buf = append(buf, s...)
			buf = append(buf, 0)
		default:
			// skip unknown
		}
	}
	L.PushString(string(buf))
	return 1
}

// str_unpack implements string.unpack(fmt, s [, pos]).
func str_unpack(L *luaapi.State) int {
	fmtStr := L.CheckString(1)
	data := L.CheckString(2)
	pos := int(L.OptInteger(3, 1)) - 1 // Lua 1-based to 0-based
	if pos < 0 {
		pos = 0
	}

	isLittle := true
	i := 0
	nret := 0
	for i < len(fmtStr) {
		c := fmtStr[i]
		i++
		switch {
		case c == '<':
			isLittle = true
			continue
		case c == '>':
			isLittle = false
			continue
		case c == '=', c == '!':
			continue
		case c >= '0' && c <= '9', c == ' ':
			continue
		case c == 'x':
			pos++
		case c == 'b':
			if pos >= len(data) {
				L.ArgError(2, "data string too short")
			}
			L.PushInteger(int64(int8(data[pos])))
			pos++
			nret++
		case c == 'B':
			if pos >= len(data) {
				L.ArgError(2, "data string too short")
			}
			L.PushInteger(int64(data[pos]))
			pos++
			nret++
		case c == 'i', c == 'I':
			sz := 4
			signed := c == 'i'
			if i < len(fmtStr) && fmtStr[i] >= '1' && fmtStr[i] <= '9' {
				sz = int(fmtStr[i] - '0')
				i++
				for i < len(fmtStr) && fmtStr[i] >= '0' && fmtStr[i] <= '9' {
					sz = sz*10 + int(fmtStr[i]-'0')
					i++
				}
			}
			v := readInt(data, pos, sz, isLittle, signed)
			L.PushInteger(v)
			pos += sz
			nret++
		case c == 'h':
			v := readInt(data, pos, 2, isLittle, true)
			L.PushInteger(v)
			pos += 2
			nret++
		case c == 'H':
			v := readInt(data, pos, 2, isLittle, false)
			L.PushInteger(v)
			pos += 2
			nret++
		case c == 'l', c == 'j':
			v := readInt(data, pos, 8, isLittle, true)
			L.PushInteger(v)
			pos += 8
			nret++
		case c == 'L', c == 'J':
			v := readInt(data, pos, 8, isLittle, false)
			L.PushInteger(v)
			pos += 8
			nret++
		case c == 'f':
			bits := uint32(readUint(data, pos, 4, isLittle))
			L.PushNumber(float64(math.Float32frombits(bits)))
			pos += 4
			nret++
		case c == 'd', c == 'n':
			bits := readUint(data, pos, 8, isLittle)
			L.PushNumber(math.Float64frombits(bits))
			pos += 8
			nret++
		case c == 's':
			sz := 8
			if i < len(fmtStr) && fmtStr[i] >= '1' && fmtStr[i] <= '9' {
				sz = int(fmtStr[i] - '0')
				i++
			}
			slen := int(readUint(data, pos, sz, isLittle))
			pos += sz
			L.PushString(data[pos : pos+slen])
			pos += slen
			nret++
		case c == 'z':
			end := pos
			for end < len(data) && data[end] != 0 {
				end++
			}
			L.PushString(data[pos:end])
			pos = end + 1
			nret++
		default:
			// skip
		}
	}
	L.PushInteger(int64(pos + 1)) // return final position (1-based)
	return nret + 1
}

func appendInt(buf []byte, v uint64, size int, little bool) []byte {
	b := make([]byte, size)
	if little {
		for i := 0; i < size; i++ {
			b[i] = byte(v >> (uint(i) * 8))
		}
	} else {
		for i := size - 1; i >= 0; i-- {
			b[i] = byte(v)
			v >>= 8
		}
	}
	return append(buf, b...)
}

func readUint(data string, pos, size int, little bool) uint64 {
	var v uint64
	if little {
		for i := size - 1; i >= 0; i-- {
			v = (v << 8) | uint64(data[pos+i])
		}
	} else {
		for i := 0; i < size; i++ {
			v = (v << 8) | uint64(data[pos+i])
		}
	}
	return v
}

func readInt(data string, pos, size int, little, signed bool) int64 {
	v := readUint(data, pos, size, little)
	if signed && size < 8 {
		mask := uint64(1) << (uint(size)*8 - 1)
		if v&mask != 0 {
			v |= ^((uint64(1) << (uint(size) * 8)) - 1)
		}
	}
	return int64(v)
}

// OpenString opens the string library.
func OpenString(L *luaapi.State) int {
	strFuncs := map[string]luaapi.CFunction{
		"byte":     str_byte,
		"char":     str_char,
		"find":     str_find,
		"format":   str_format,
		"gmatch":   str_gmatch,
		"gsub":     str_gsub,
		"len":      str_len,
		"lower":    str_lower,
		"match":    str_match,
		"pack":     str_pack,
		"packsize": str_packsize,
		"rep":      str_rep,
		"reverse":  str_reverse,
		"sub":      str_sub,
		"unpack":   str_unpack,
		"upper":    str_upper,
	}
	L.NewLib(strFuncs)

	// Set string metatable so methods work on string values
	L.CreateTable(0, 1)
	L.PushValue(-2) // push string library table
	L.SetField(-2, "__index")
	L.PushString("") // push a string to get its metatable slot
	L.PushValue(-2)  // push the metatable
	L.SetMetatable(-2)
	L.Pop(2) // pop string and metatable

	return 1
}
