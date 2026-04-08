// Lua pattern matching engine.
// Implements Lua 5.4 pattern matching (NOT Go regex).
// Patterns: . %a %d %w %s %l %u %p %x %c [set] [^set] * + - ? ^ $ () %bxy
package internal

import (
	"strings"
	"unicode"
)

const luaMaxCaptures = 32

// luaCapture represents a single capture in a pattern match.
type luaCapture struct {
	init int // start position in subject string (0-based)
	len  int // length of capture, or capUnfinished or capPosition
}

const (
	capUnfinished = -1
	capPosition   = -2
)

// luaMatchState holds the state for a single pattern match attempt.
type luaMatchState struct {
	src        string
	pat        string
	level      int
	captures   [luaMaxCaptures]luaCapture
	matchDepth int
}

const luaMaxMatchCalls = 200

// classEnd returns the index past the end of a character class in pattern p starting at i.
func classEnd(p string, i int) int {
	c := p[i]
	i++
	switch c {
	case '%':
		if i >= len(p) {
			luaErrorString("malformed pattern (ends with '%%')")
		}
		return i + 1
	case '[':
		if i < len(p) && p[i] == '^' {
			i++
		}
		if i < len(p) {
			i++ // skip first char (handles ']' as first)
		}
		for i < len(p) && p[i] != ']' {
			if p[i] == '%' {
				i++
				if i >= len(p) {
					luaErrorString("malformed pattern (missing ']')")
				}
			}
			i++
		}
		if i >= len(p) {
			luaErrorString("malformed pattern (missing ']')")
		}
		return i + 1
	default:
		return i
	}
}

// isClassChar checks if byte ch matches the Lua character class cl.
func isClassChar(cl byte, ch byte) bool {
	var res bool
	lowerCl := cl | 0x20
	switch lowerCl {
	case 'a':
		res = isAlpha(ch)
	case 'c':
		res = isCntrl(ch)
	case 'd':
		res = isDigit(ch)
	case 'l':
		res = isLower(ch)
	case 'p':
		res = isPunct(ch)
	case 's':
		res = isSpace(ch)
	case 'u':
		res = isUpper(ch)
	case 'w':
		res = isAlnum(ch)
	case 'x':
		res = isXDigit(ch)
	case 'g':
		res = isPrint(ch) && !isSpace(ch)
	default:
		return cl == ch
	}
	if isUpper(cl) {
		res = !res
	}
	return res
}

// matchClass checks if byte ch matches the class starting at pattern position pi.
func matchClass(ch byte, p string, pi int) bool {
	switch p[pi] {
	case '.':
		return true
	case '%':
		return isClassChar(p[pi+1], ch)
	case '[':
		pi++
		negate := false
		if pi < len(p) && p[pi] == '^' {
			negate = true
			pi++
		}
		found := false
		for pi < len(p) && p[pi] != ']' {
			if p[pi] == '%' && pi+1 < len(p) {
				pi++
				if isClassChar(p[pi], ch) {
					found = true
				}
				pi++
			} else if pi+2 < len(p) && p[pi+1] == '-' && p[pi+2] != ']' {
				lo := p[pi]
				hi := p[pi+2]
				if lo <= ch && ch <= hi {
					found = true
				}
				pi += 3
			} else {
				if p[pi] == ch {
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
		return p[pi] == ch
	}
}

// match is the core recursive pattern matching function.
// Returns end position (0-based, exclusive) or -1 if no match.
func (ms *luaMatchState) match(si, pi int) int {
	ms.matchDepth--
	if ms.matchDepth <= 0 {
		luaErrorString("pattern too complex")
	}
	defer func() { ms.matchDepth++ }()

	for pi < len(ms.pat) {
		switch ms.pat[pi] {
		case '(':
			if pi+1 < len(ms.pat) && ms.pat[pi+1] == ')' {
				return ms.matchPosCapture(si, pi)
			}
			return ms.matchCapOpen(si, pi)
		case ')':
			return ms.matchCapClose(si, pi)
		case '$':
			if pi+1 == len(ms.pat) {
				if si == len(ms.src) {
					return si
				}
				return -1
			}
			goto dflt
		case '%':
			if pi+1 < len(ms.pat) && ms.pat[pi+1] == 'b' {
				return ms.matchBalancePair(si, pi)
			}
			if pi+1 < len(ms.pat) && ms.pat[pi+1] == 'f' {
				return ms.matchFrontier(si, pi)
			}
			goto dflt
		default:
			goto dflt
		}
	dflt:
		ep := classEnd(ms.pat, pi)
		if ep < len(ms.pat) {
			switch ms.pat[ep] {
			case '*':
				return ms.matchQuantGreedy(si, pi, ep)
			case '+':
				return ms.matchQuantPlus(si, pi, ep)
			case '-':
				return ms.matchQuantLazy(si, pi, ep)
			case '?':
				return ms.matchQuantOpt(si, pi, ep)
			}
		}
		if si < len(ms.src) && matchClass(ms.src[si], ms.pat, pi) {
			si++
			pi = ep
			continue
		}
		return -1
	}
	return si
}

// matchPosCapture handles position captures: ()
func (ms *luaMatchState) matchPosCapture(si, pi int) int {
	if ms.level >= luaMaxCaptures {
		luaErrorString("too many captures")
	}
	l := ms.level
	ms.captures[l].init = si
	ms.captures[l].len = capPosition
	ms.level++
	res := ms.match(si, pi+2)
	if res == -1 {
		ms.level--
	}
	return res
}

func (ms *luaMatchState) matchCapOpen(si, pi int) int {
	if ms.level >= luaMaxCaptures {
		luaErrorString("too many captures")
	}
	l := ms.level
	ms.captures[l].init = si
	ms.captures[l].len = capUnfinished
	ms.level++
	res := ms.match(si, pi+1)
	if res == -1 {
		ms.level--
	}
	return res
}

func (ms *luaMatchState) matchCapClose(si, pi int) int {
	l := ms.findOpenCapture()
	ms.captures[l].len = si - ms.captures[l].init
	res := ms.match(si, pi+1)
	if res == -1 {
		ms.captures[l].len = capUnfinished
	}
	return res
}

func (ms *luaMatchState) findOpenCapture() int {
	for l := ms.level - 1; l >= 0; l-- {
		if ms.captures[l].len == capUnfinished {
			return l
		}
	}
	luaErrorString("invalid pattern capture")
	return -1
}

func (ms *luaMatchState) matchQuantGreedy(si, pi, ep int) int {
	count := 0
	for si+count < len(ms.src) && matchClass(ms.src[si+count], ms.pat, pi) {
		count++
	}
	for count >= 0 {
		res := ms.match(si+count, ep+1)
		if res != -1 {
			return res
		}
		count--
	}
	return -1
}

func (ms *luaMatchState) matchQuantPlus(si, pi, ep int) int {
	count := 0
	for si+count < len(ms.src) && matchClass(ms.src[si+count], ms.pat, pi) {
		count++
	}
	for count >= 1 {
		res := ms.match(si+count, ep+1)
		if res != -1 {
			return res
		}
		count--
	}
	return -1
}

func (ms *luaMatchState) matchQuantLazy(si, pi, ep int) int {
	for {
		res := ms.match(si, ep+1)
		if res != -1 {
			return res
		}
		if si < len(ms.src) && matchClass(ms.src[si], ms.pat, pi) {
			si++
		} else {
			return -1
		}
	}
}

func (ms *luaMatchState) matchQuantOpt(si, pi, ep int) int {
	if si < len(ms.src) && matchClass(ms.src[si], ms.pat, pi) {
		res := ms.match(si+1, ep+1)
		if res != -1 {
			return res
		}
	}
	return ms.match(si, ep+1)
}

func (ms *luaMatchState) matchBalancePair(si, pi int) int {
	if pi+3 >= len(ms.pat) {
		luaErrorString("malformed pattern (missing arguments to '%%b')")
	}
	open := ms.pat[pi+2]
	close := ms.pat[pi+3]
	if si >= len(ms.src) || ms.src[si] != open {
		return -1
	}
	count := 1
	si++
	for si < len(ms.src) && count > 0 {
		if ms.src[si] == open {
			count++
		} else if ms.src[si] == close {
			count--
		}
		si++
	}
	if count != 0 {
		return -1
	}
	return ms.match(si, pi+4)
}

func (ms *luaMatchState) matchFrontier(si, pi int) int {
	if pi+2 >= len(ms.pat) || ms.pat[pi+2] != '[' {
		luaErrorString("missing '[' after '%%f' in pattern")
	}
	ep := classEnd(ms.pat, pi+2)
	var prev byte
	if si > 0 {
		prev = ms.src[si-1]
	}
	if !matchClass(prev, ms.pat, pi+2) {
		var cur byte
		if si < len(ms.src) {
			cur = ms.src[si]
		}
		if matchClass(cur, ms.pat, pi+2) {
			return ms.match(si, ep)
		}
	}
	return -1
}

// Character classification helpers (ASCII-based, matching Lua's C locale)
func isAlpha(c byte) bool  { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isAlnum(c byte) bool  { return isAlpha(c) || isDigit(c) }
func isLower(c byte) bool  { return c >= 'a' && c <= 'z' }
func isUpper(c byte) bool  { return c >= 'A' && c <= 'Z' }
func isSpace(c byte) bool  { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' }
func isPrint(c byte) bool  { return c >= 0x20 && c <= 0x7e }
func isCntrl(c byte) bool  { return c < 0x20 || c == 0x7f }
func isPunct(c byte) bool  {
	return (c >= 0x21 && c <= 0x2f) || (c >= 0x3a && c <= 0x40) || (c >= 0x5b && c <= 0x60) || (c >= 0x7b && c <= 0x7e)
}
func isXDigit(c byte) bool { return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }

// luaPatternFind searches for pattern in subject starting from init (0-based).
// Returns (start, end, captures, ncaptures, found). start/end are 0-based.
func luaPatternFind(src, pat string, init int, anchor bool) (int, int, []luaCapture, int, bool) {
	ms := &luaMatchState{
		src: src,
		pat: pat,
	}

	for si := init; si <= len(src); si++ {
		ms.level = 0
		ms.matchDepth = luaMaxMatchCalls
		res := ms.match(si, 0)
		if res != -1 {
			caps := make([]luaCapture, ms.level)
			copy(caps, ms.captures[:ms.level])
			return si, res, caps, ms.level, true
		}
		if anchor {
			break
		}
	}
	return 0, 0, nil, 0, false
}

// luaPatternReplaceCaptures replaces %0-%9 in repl with captured strings.
func luaPatternReplaceCaptures(repl, src string, caps []luaCapture, ncap int, matchStart, matchEnd int) string {
	var buf strings.Builder
	for i := 0; i < len(repl); i++ {
		if repl[i] == '%' && i+1 < len(repl) {
			i++
			if repl[i] >= '0' && repl[i] <= '9' {
				idx := int(repl[i] - '0')
				if idx == 0 {
					buf.WriteString(src[matchStart:matchEnd])
				} else if idx <= ncap {
					c := caps[idx-1]
					if c.len == capPosition {
						// position capture: write the 1-based position as string
						buf.WriteString(strings.Repeat("", 0)) // placeholder
					} else if c.len >= 0 {
						buf.WriteString(src[c.init : c.init+c.len])
					}
				} else {
					luaErrorString("invalid capture index %" + string(repl[i]))
				}
			} else if repl[i] == '%' {
				buf.WriteByte('%')
			} else {
				buf.WriteByte('%')
				buf.WriteByte(repl[i])
			}
		} else {
			buf.WriteByte(repl[i])
		}
	}
	return buf.String()
}

// getCaptureString returns the string for capture index i (0-based).
func getCaptureString(src string, caps []luaCapture, ncap int, i int, matchStart, matchEnd int) string {
	if ncap == 0 && i == 0 {
		return src[matchStart:matchEnd]
	}
	if i >= ncap {
		return ""
	}
	c := caps[i]
	if c.len == capPosition {
		return ""
	}
	if c.len == capUnfinished {
		luaErrorString("unfinished capture")
	}
	return src[c.init : c.init+c.len]
}

// String utility functions
func luaToLower(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + 32
		}
		buf.WriteByte(c)
	}
	return buf.String()
}

func luaToUpper(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c = c - 32
		}
		buf.WriteByte(c)
	}
	return buf.String()
}

func luaStringReverse(s string) string {
	n := len(s)
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		buf[n-1-i] = s[i]
	}
	return string(buf)
}

var _ = unicode.IsLetter // keep import
