package api

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	luaapi "github.com/akzj/go-lua/internal/api/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	vmapi "github.com/akzj/go-lua/internal/vm/api"
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

// ---------------------------------------------------------------------------
// Pack/unpack format parser — matches C Lua's getoption/getdetails/getnum/getnumlimit
// Reference: lua-master/lstrlib.c lines 1440-1570
// ---------------------------------------------------------------------------

// maxIntSize is the maximum size for pack/unpack integer formats (matches C Lua).
const maxIntSize = 16

// maxSize is the maximum total size for pack results.
// On 64-bit: MAX_SIZE = LUA_MAXINTEGER (matches C Lua's llimits.h).
const maxSize = math.MaxInt64

// kOption classifies a pack/unpack format option.
type kOption int

const (
	kInt       kOption = iota // signed integers
	kUint                     // unsigned integers
	kFloat                    // C float
	kNumber                   // Lua number (float64)
	kDouble                   // C double
	kChar                     // fixed-size string (cn)
	kString                   // strings with length count (sn)
	kZstr                     // zero-terminated strings
	kPadding                  // padding byte (x)
	kPaddalign                // padding for alignment (X)
	kNop                      // no-op (configuration or spaces)
)

// packHeader holds pack/unpack state — matches C Lua's Header struct.
type packHeader struct {
	L        *luaapi.State
	islittle bool
	maxalign int
}

// initHeader initializes a packHeader with native defaults.
func initHeader(L *luaapi.State) packHeader {
	return packHeader{L: L, islittle: true, maxalign: 1} // assume little-endian native
}

// getnum reads an integer numeral from fmtStr[*pos] or returns df if no digit.
// Matches C Lua's getnum() in lstrlib.c.
func getnum(fmtStr string, pos *int, df int) int {
	if *pos >= len(fmtStr) || fmtStr[*pos] < '0' || fmtStr[*pos] > '9' {
		return df
	}
	a := 0
	for *pos < len(fmtStr) && fmtStr[*pos] >= '0' && fmtStr[*pos] <= '9' && a <= (maxSize-9)/10 {
		a = a*10 + int(fmtStr[*pos]-'0')
		*pos++
	}
	return a
}

// getnumlimit reads an integer numeral and errors if outside [1, maxIntSize].
// Matches C Lua's getnumlimit() in lstrlib.c.
func getnumlimit(h *packHeader, fmtStr string, pos *int, df int) int {
	sz := getnum(fmtStr, pos, df)
	if sz < 1 || sz > maxIntSize { // must be in [1, maxIntSize]
		h.L.ArgError(1, fmt.Sprintf("integral size (%d) out of limits [1,%d]", sz, maxIntSize))
	}
	return sz
}

// getoption reads and classifies the next format option.
// Returns the option kind and fills *size with the option's data size.
// Advances *pos past the option character and any following digits.
// Matches C Lua's getoption() in lstrlib.c.
func getoption(h *packHeader, fmtStr string, pos *int, size *int) kOption {
	if *pos >= len(fmtStr) {
		// shouldn't happen — caller checks
		return kNop
	}
	opt := fmtStr[*pos]
	*pos++
	*size = 0 // default
	switch opt {
	case 'b':
		*size = 1
		return kInt
	case 'B':
		*size = 1
		return kUint
	case 'h':
		*size = 2
		return kInt
	case 'H':
		*size = 2
		return kUint
	case 'l':
		*size = 8
		return kInt // sizeof(long) on 64-bit
	case 'L':
		*size = 8
		return kUint
	case 'j':
		*size = 8
		return kInt // sizeof(lua_Integer)
	case 'J':
		*size = 8
		return kUint
	case 'T':
		*size = 8
		return kUint // sizeof(size_t)
	case 'f':
		*size = 4
		return kFloat
	case 'n':
		*size = 8
		return kNumber
	case 'd':
		*size = 8
		return kDouble
	case 'i':
		*size = getnumlimit(h, fmtStr, pos, 4)
		return kInt
	case 'I':
		*size = getnumlimit(h, fmtStr, pos, 4)
		return kUint
	case 's':
		*size = getnumlimit(h, fmtStr, pos, 8)
		return kString
	case 'c':
		*size = getnum(fmtStr, pos, -1)
		if *size == -1 {
			h.L.Errorf("missing size for format option 'c'")
		}
		return kChar
	case 'z':
		return kZstr
	case 'x':
		*size = 1
		return kPadding
	case 'X':
		return kPaddalign
	case ' ':
		// skip
	case '<':
		h.islittle = true
	case '>':
		h.islittle = false
	case '=':
		h.islittle = true // native = little-endian on our platform
	case '!':
		h.maxalign = getnumlimit(h, fmtStr, pos, 8) // default maxalign = 8 (offsetof(struct cD, u) on 64-bit)
	default:
		h.L.Errorf("invalid format option '%c'", opt)
	}
	return kNop
}

// ispow2 checks if n is a power of 2.
func ispow2(n int) bool {
	return n > 0 && (n&(n-1)) == 0
}

// getdetails reads, classifies, and computes alignment for the next option.
// Returns the option kind, fills *psize with data size, *ntoalign with alignment padding.
// Matches C Lua's getdetails() in lstrlib.c.
func getdetails(h *packHeader, totalsize int, fmtStr string, pos *int, psize *int, ntoalign *int) kOption {
	opt := getoption(h, fmtStr, pos, psize)
	align := *psize // usually, alignment follows size
	if opt == kPaddalign {
		// 'X' gets alignment from following option
		if *pos >= len(fmtStr) {
			h.L.ArgError(1, "invalid next option for option 'X'")
		}
		nextOpt := getoption(h, fmtStr, pos, &align)
		if nextOpt == kChar || align == 0 {
			h.L.ArgError(1, "invalid next option for option 'X'")
		}
	}
	if align <= 1 || opt == kChar { // need no alignment?
		*ntoalign = 0
	} else {
		if align > h.maxalign { // enforce maximum alignment
			align = h.maxalign
		}
		if !ispow2(align) {
			*ntoalign = 0
			h.L.ArgError(1, "format asks for alignment not power of 2")
		} else {
			// szmoda = totalsize % align
			szmoda := totalsize & (align - 1)
			*ntoalign = (align - szmoda) & (align - 1)
		}
	}
	return opt
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
		if arg > L.GetTop() {
			L.ArgError(arg, "no value")
		}
		if len(spec) > 32 {
			L.Errorf("invalid format (too long)")
		}

		// checkformat: validates flags, width, precision per conversion
		// Mirrors C Lua's checkformat(L, form, flags, precision)
		checkFmt := func(validFlags string, allowPrec bool) {
			p := 1 // skip '%'
			// skip only valid flags
			for p < len(spec)-1 && strings.ContainsRune(validFlags, rune(spec[p])) {
				p++
			}
			// width cannot start with '0' (0 as flag must be in validFlags)
			if p < len(spec)-1 && spec[p] != '0' {
				// skip up to 2 width digits
				for nd := 0; nd < 2 && p < len(spec)-1 && spec[p] >= '0' && spec[p] <= '9'; nd++ {
					p++
				}
				if allowPrec && p < len(spec)-1 && spec[p] == '.' {
					p++
					// skip up to 2 precision digits
					for nd := 0; nd < 2 && p < len(spec)-1 && spec[p] >= '0' && spec[p] <= '9'; nd++ {
						p++
					}
				}
			}
			// must have consumed everything up to the conversion letter
			if p != len(spec)-1 {
				L.Errorf("invalid conversion '%s' to 'format'", spec)
			}
		}

		switch conv {
		case 'c':
			checkFmt("-", false)
		case 'd', 'i':
			checkFmt("-+0 ", true)
		case 'u':
			checkFmt("-0", true)
		case 'o', 'x', 'X':
			checkFmt("-#0", true)
		case 'a', 'A':
			checkFmt("-+#0 ", true)
		case 'f', 'e', 'E', 'g', 'G':
			checkFmt("-+#0 ", true)
		case 'p':
			checkFmt("-", false)
		case 'q':
			if spec != "%q" {
				L.Errorf("specifier '%%q' cannot have modifiers")
			}
		case 's':
			// s with modifiers: checkFmt with "-" flags and precision allowed
			if spec != "%s" {
				checkFmt("-", true)
			}
		default:
			L.Errorf("invalid conversion '%s' to 'format'", spec)
		}

		switch conv {
		case 'd', 'i':
			n := L.CheckInteger(arg)
			// Go doesn't support %i; replace trailing 'i' with 'd'
			goSpec := spec
			if conv == 'i' {
				goSpec = spec[:len(spec)-1] + "d"
			}
			sb.WriteString(fmt.Sprintf(goSpec, n))
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
		case 'f', 'e', 'E', 'g', 'G':
			n := L.CheckNumber(arg)
			sb.WriteString(fmt.Sprintf(spec, n))
		case 'a', 'A':
			n := L.CheckNumber(arg)
			// Parse flags, width, precision from spec
			upper := conv == 'A'
			prec := -1 // default: full precision
			hasPlus := strings.ContainsRune(spec, '+')
			hasSpace := strings.ContainsRune(spec, ' ')
			width := 0
			hasMinus := strings.ContainsRune(spec, '-')

			// Parse width from spec (between flags and '.' or conv)
			j := 1 // skip '%'
			for j < len(spec) && strings.ContainsRune("-+ #0", rune(spec[j])) {
				j++
			}
			wStart := j
			for j < len(spec) && spec[j] >= '0' && spec[j] <= '9' {
				j++
			}
			if j > wStart {
				for _, ch := range spec[wStart:j] {
					width = width*10 + int(ch-'0')
				}
			}

			// Parse precision
			if j < len(spec) && spec[j] == '.' {
				j++
				prec = 0
				for j < len(spec) && spec[j] >= '0' && spec[j] <= '9' {
					prec = prec*10 + int(spec[j]-'0')
					j++
				}
			}

			hexStr := formatHexFloat(n, prec, upper)

			// Apply + or space flag for non-negative numbers
			if !math.IsNaN(n) && !math.Signbit(n) {
				if hasPlus {
					hexStr = "+" + hexStr
				} else if hasSpace {
					hexStr = " " + hexStr
				}
			}

			// Apply width padding
			if width > 0 && len(hexStr) < width {
				pad := strings.Repeat(" ", width-len(hexStr))
				if hasMinus {
					hexStr = hexStr + pad // left-aligned
				} else {
					hexStr = pad + hexStr // right-aligned
				}
			}

			sb.WriteString(hexStr)
		case 'x', 'X':
			n := L.CheckInteger(arg)
			// C printf treats %x/%X as unsigned; Go's %x on negative int64 produces "-1"
			sb.WriteString(fmt.Sprintf(spec, uint64(n)))
		case 'o':
			n := L.CheckInteger(arg)
			// C printf treats %o as unsigned; Go's %o on negative int64 produces "-1"
			sb.WriteString(fmt.Sprintf(spec, uint64(n)))
		case 'c':
			n := L.CheckInteger(arg)
			ch := string([]byte{byte(n)})
			sSpec := spec[:len(spec)-1] + "s"
			sb.WriteString(fmt.Sprintf(sSpec, ch))
		case 's':
			s := L.TolString(arg)
			L.Pop(1) // pop string from TolString
			// Check if we need to apply format (width/precision)
			if spec == "%s" {
				sb.WriteString(s)
			} else {
				// C Lua: strings with embedded zeros can't be formatted with width/precision
				if strings.ContainsRune(s, 0) {
					L.ArgCheck(false, arg, "string contains zeros")
				}
				sb.WriteString(fmt.Sprintf(spec, s))
			}
		case 'q':
			// C Lua addliteral: handles string, number, nil, boolean
			switch L.Type(arg) {
			case objectapi.TypeString:
				s := L.CheckString(arg)
				sb.WriteString(quoteString(s))
			case objectapi.TypeNumber:
				if L.IsInteger(arg) {
					n, _ := L.ToInteger(arg)
					// Corner case: mininteger uses hex to avoid overflow
					if n == math.MinInt64 {
						sb.WriteString(fmt.Sprintf("0x%x", uint64(n)))
					} else {
						sb.WriteString(fmt.Sprintf("%d", n))
					}
				} else {
					n, _ := L.ToNumber(arg)
					sb.WriteString(quoteFloat(n))
				}
			case objectapi.TypeNil:
				sb.WriteString("nil")
			case objectapi.TypeBoolean:
				b := L.ToBoolean(arg)
				if b {
					sb.WriteString("true")
				} else {
					sb.WriteString("false")
				}
			default:
				L.ArgError(arg, "value has no literal form")
			}
		case 'p':
			// pointer representation — mirrors luaO_pushfstring %p
			// nil, boolean, number → "(null)"
			// table, function, userdata, thread, string → "0x..." hex address
			v := L.Type(arg)
			var pstr string
			switch v {
			case objectapi.TypeNil, objectapi.TypeBoolean, objectapi.TypeNumber:
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
			L.Errorf("invalid conversion '%s' to 'format'", spec)
		}
	}
	L.PushString(sb.String())
	return 1
}

// formatHexFloat formats a float as hex (%a/%A) matching C Lua output.
// Returns lowercase hex float string. Caller uppercases for %A.
func formatHexFloat(n float64, prec int, upper bool) string {
	// Handle special values
	if math.IsInf(n, 1) {
		if upper {
			return "INF"
		}
		return "inf"
	}
	if math.IsInf(n, -1) {
		if upper {
			return "-INF"
		}
		return "-inf"
	}
	if math.IsNaN(n) {
		if upper {
			return "NAN"
		}
		return "nan"
	}

	var s string
	if prec >= 0 {
		s = strconv.FormatFloat(n, 'x', prec, 64)
	} else {
		s = strconv.FormatFloat(n, 'x', -1, 64)
	}

	// Normalize exponent: strip leading zeros (Go: p+01 → C: p+1)
	if idx := strings.Index(s, "p+"); idx >= 0 {
		exp := s[idx+2:]
		for len(exp) > 1 && exp[0] == '0' {
			exp = exp[1:]
		}
		s = s[:idx+2] + exp
	} else if idx := strings.Index(s, "p-"); idx >= 0 {
		exp := s[idx+2:]
		for len(exp) > 1 && exp[0] == '0' {
			exp = exp[1:]
		}
		s = s[:idx+2] + exp
	}

	if upper {
		s = strings.ToUpper(s)
	}
	return s
}

// quoteFloat formats a float for %q — matches C Lua's quotefloat.
// Uses hex float format for precision, special strings for inf/nan.
func quoteFloat(n float64) string {
	if math.IsInf(n, 1) {
		return "1e9999"
	}
	if math.IsInf(n, -1) {
		return "-1e9999"
	}
	if math.IsNaN(n) {
		return "(0/0)"
	}
	return formatHexFloat(n, -1, false)
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
			sb.WriteString("\\\n")
		default:
			// C Lua: iscntrl(cast_uchar(c)) in C locale — 0x00-0x1F and 0x7F
			// Bytes >= 0x80 pass through as-is (not control chars in C locale)
			if c < ' ' || c == 0x7F {
				// If next char is a digit, use 3-digit format to avoid ambiguity
				if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
					sb.WriteString(fmt.Sprintf("\\%03d", c))
				} else {
					sb.WriteString(fmt.Sprintf("\\%d", c))
				}
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
	L       *luaapi.State
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

func classEnd(ms *matchState, p int) int {
	pat := ms.pat
	c := pat[p]
	p++
	switch c {
	case '%':
		if p >= len(pat) {
			ms.L.Errorf("malformed pattern (ends with '%%')")
		}
		return p + 1
	case '[':
		if p < len(pat) && pat[p] == '^' {
			p++
		}
		// ']' right after '[' or '[^' is a literal member of the class
		if p < len(pat) && pat[p] == ']' {
			p++
		}
		// skip until closing ] — matches C Lua's do...while loop
		for {
			if p >= len(pat) {
				ms.L.Errorf("malformed pattern (missing ']')")
			}
			c = pat[p]
			p++
			if c == '%' && p < len(pat) {
				p++ // skip escaped char (e.g. '%]')
				continue
			}
			if c == ']' {
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
	case 'g':
		// printable non-space (isgraph): printable chars except space
		res = c > 0x20 && c < 0x7f
	case 'l':
		res = c >= 'a' && c <= 'z'
	case 'p':
		// C ispunct: printable non-alnum non-space (ASCII 0x21-0x7E minus alnum)
		res = c > 0x20 && c < 0x7f && !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'))
	case 's':
		res = c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
	case 'u':
		res = c >= 'A' && c <= 'Z'
	case 'w':
		res = (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
	case 'x':
		res = (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
	case 'z':
		res = c == 0
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
		ms.L.Errorf("malformed pattern (missing arguments to '%%b')")
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
				if pi >= len(ms.pat) || ms.pat[pi] != '[' {
					ms.L.Errorf("missing '[' after '%%f' in pattern")
				}
				ep := classEnd(ms, pi)
				var prev byte
				if si > 0 {
					prev = ms.src[si-1]
				}
				var cur byte
				if si < len(ms.src) {
					cur = ms.src[si]
				}
				// Check frontier boundary: prev NOT in set, cur IN set
				if singlematchByte(ms, prev, pi, ep) || !singlematchByte(ms, cur, pi, ep) {
					return -1
				}
				pi = ep
				continue
			}
			// Backreference: %0-%9 (C Lua's match_capture + check_capture)
			if pi+1 < len(ms.pat) {
				c := ms.pat[pi+1]
				if c >= '0' && c <= '9' {
					idx := int(c) - int('1') // '0' → -1, '1' → 0, etc.
					// check_capture: error if idx < 0 or idx >= level or unfinished
					if idx < 0 || idx >= ms.level || ms.capture[idx].len == capUnfinished {
						ms.L.Errorf("invalid capture index %%%d", idx+1)
					}
					capLen := ms.capture[idx].len
					if capLen < 0 {
						return -1 // position capture can't be backreferenced
					}
					capStart := ms.capture[idx].init
					if si+capLen <= len(ms.src) && ms.src[si:si+capLen] == ms.src[capStart:capStart+capLen] {
						si += capLen
						pi += 2
						continue
					}
					return -1
				}
			}
			goto dflt
		default:
			goto dflt
		}
	dflt:
		ep := classEnd(ms, pi)
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
		ms.L.Errorf("invalid pattern capture")
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
		ms.pushOneCapture(L, i, si, ei)
	}
	return nlevels
}

// pushOneCapture pushes capture at index i (0-based).
// Mirrors C Lua's push_onecapture.
func (ms *matchState) pushOneCapture(L *luaapi.State, i int, si, ei int) {
	if i >= ms.level {
		if i != 0 {
			ms.L.Errorf("invalid capture index %%%d", i+1)
		}
		// No captures: capture 0 = whole match
		L.PushString(ms.src[si:ei])
		return
	}
	cap := ms.capture[i]
	if cap.len == capUnfinished {
		ms.L.Errorf("unfinished capture")
	} else if cap.len == capPosition {
		L.PushInteger(int64(cap.init + 1)) // 1-based
	} else {
		start := cap.init
		end := start + cap.len
		if end > len(ms.src) {
			end = len(ms.src)
		}
		L.PushString(ms.src[start:end])
	}
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

	ms := &matchState{L: L, src: s, pat: pat}
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
		if anchor || si > len(s) {
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
	lastmatch := -1 // tracks last match end to skip duplicate empty matches

	iter := func(L *luaapi.State) int {
		if done {
			return 0
		}
		ms := &matchState{L: L, src: s, pat: pat}
		for pos <= len(s) {
			ms.level = 0
			res := ms.match(pos, 0)
			if res >= 0 && res != lastmatch {
				lastmatch = res
				si := pos
				pos = res
				if res == si {
					pos++ // empty match: advance for next iteration
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

	// Push as CClosure with 1 upvalue (the source string) so that
	// debug.getupvalue sees it as a C closure (upvalues named "").
	// C Lua's gmatch pushes a CClosure with upvalues for state.
	L.PushString(s)
	L.PushCClosure(func(L *luaapi.State) int {
		return iter(L)
	}, 1)
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
	changed := false // tracks whether any actual substitution occurred
	lastmatch := -1 // end of last match (Lua 5.3.3+ empty match semantics)
	for n < maxn {
		ms := &matchState{L: L, src: s, pat: pat}
		ms.level = 0
		res := ms.match(si, 0)
		if res >= 0 && res != lastmatch { // match, not same end as last
			n++
			switch replType {
			case objectapi.TypeString:
				repl := L.CheckString(3)
				sb.WriteString(gsubReplace(L, repl, ms, si, res))
				changed = true
			case objectapi.TypeTable:
				ms.pushOneCapture(L, 0, si, res) // first capture is the index
				L.GetTable(3)
				if addReplacementChanged(L, &sb, s, si, res) {
					changed = true
				}
			case objectapi.TypeFunction:
				nCap := ms.pushCapture(L, si, res)
				L.PushValue(3)
				L.Insert(-(nCap + 1))
				L.Call(nCap, 1)
				if addReplacementChanged(L, &sb, s, si, res) {
					changed = true
				}
			default:
				L.ArgError(3, "string/function/table expected")
			}
			si = res
			lastmatch = res
		} else if si < len(s) {
			sb.WriteByte(s[si])
			si++
		} else {
			break
		}
		if anchor {
			break
		}
	}
	// Append remainder
	if si <= len(s) {
		sb.WriteString(s[si:])
	}
	if !changed {
		// No actual substitutions — return the original string object
		L.PushValue(1)
	} else {
		L.PushString(sb.String())
	}
	L.PushInteger(int64(n))
	return 2
}

// addReplacementChanged is like addReplacement but returns true if an actual
// substitution occurred (replacement value was truthy).
func addReplacementChanged(L *luaapi.State, sb *strings.Builder, s string, si, ei int) bool {
	// Value is on top of stack
	if !L.ToBoolean(-1) {
		L.Pop(1)
		sb.WriteString(s[si:ei]) // keep original
		return false
	}
	r, ok := L.ToString(-1)
	if !ok {
		L.Errorf("invalid replacement value (a %s)", L.TypeName(L.Type(-1)))
	}
	sb.WriteString(r)
	L.Pop(1)
	return true
}

func gsubReplace(L *luaapi.State, repl string, ms *matchState, si, ei int) string {
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
				if cap.len == capUnfinished {
					L.Errorf("unfinished capture")
				} else if cap.len == capPosition {
					sb.WriteString(fmt.Sprintf("%d", cap.init+1))
				} else if cap.len >= 0 {
					sb.WriteString(ms.src[cap.init : cap.init+cap.len])
				}
			} else if ms.level == 0 && idx == 1 {
				// No explicit captures: %1 = whole match (C Lua compat)
				sb.WriteString(ms.src[si:ei])
			} else {
				L.Errorf("invalid capture index %%%d", idx)
			}
		} else if c == '%' {
			sb.WriteByte('%')
		} else {
			L.Errorf("invalid use of '%%' in replacement string")
		}
	}
	return sb.String()
}

// (packOptSize removed — replaced by getoption/getdetails infrastructure above)

// str_packsize implements string.packsize(fmt).
// Matches C Lua's str_packsize in lstrlib.c.
func str_packsize(L *luaapi.State) int {
	fmtStr := L.CheckString(1)
	h := initHeader(L)
	totalsize := 0
	pos := 0
	for pos < len(fmtStr) {
		var size, ntoalign int
		opt := getdetails(&h, totalsize, fmtStr, &pos, &size, &ntoalign)
		L.ArgCheck(opt != kString && opt != kZstr, 1, "variable-length format")
		size += ntoalign
		L.ArgCheck(totalsize <= maxSize-size, 1, "format result too large")
		totalsize += size
	}
	L.PushInteger(int64(totalsize))
	return 1
}

// str_pack implements string.pack(fmt, v1, v2, ...).
// Matches C Lua's str_pack in lstrlib.c.
func str_pack(L *luaapi.State) int {
	fmtStr := L.CheckString(1)
	h := initHeader(L)
	var buf []byte
	arg := 1 // current argument to pack (will be incremented before use)
	totalsize := 0
	pos := 0
	for pos < len(fmtStr) {
		var size, ntoalign int
		opt := getdetails(&h, totalsize, fmtStr, &pos, &size, &ntoalign)
		L.ArgCheck(size+ntoalign <= maxSize-totalsize, arg, "result too long")
		totalsize += ntoalign + size
		// Fill alignment padding
		for ntoalign > 0 {
			buf = append(buf, 0)
			ntoalign--
		}
		arg++
		switch opt {
		case kInt: // signed integers
			v := L.CheckInteger(arg)
			if size < 8 { // need overflow check?
				lim := int64(1) << (uint(size)*8 - 1)
				L.ArgCheck(-lim <= v && v < lim, arg, "integer overflow")
			}
			buf = appendInt(buf, uint64(v), size, h.islittle)
		case kUint: // unsigned integers
			v := L.CheckInteger(arg)
			if size < 8 {
				L.ArgCheck(uint64(v) < (uint64(1) << (uint(size) * 8)), arg, "unsigned overflow")
			} else if size == 8 {
				// 8-byte unsigned: allow all bit patterns (C Lua does too)
			}
			buf = appendInt(buf, uint64(v), size, h.islittle)
		case kFloat: // C float
			v := float32(L.CheckNumber(arg))
			bits := math.Float32bits(v)
			buf = appendInt(buf, uint64(bits), 4, h.islittle)
		case kNumber, kDouble: // Lua number / C double
			v := L.CheckNumber(arg)
			bits := math.Float64bits(v)
			buf = appendInt(buf, bits, 8, h.islittle)
		case kChar: // fixed-size string (cn)
			s := L.CheckString(arg)
			L.ArgCheck(len(s) <= size, arg, "string longer than given size")
			buf = append(buf, s...)
			// Pad with zeros if shorter
			for i := len(s); i < size; i++ {
				buf = append(buf, 0)
			}
		case kString: // strings with length count (sn)
			s := L.CheckString(arg)
			L.ArgCheck(size >= 8 || uint64(len(s)) < (uint64(1)<<(uint(size)*8)),
				arg, "string length does not fit in given size")
			buf = appendInt(buf, uint64(len(s)), size, h.islittle)
			buf = append(buf, s...)
			totalsize += len(s)
		case kZstr: // zero-terminated string
			s := L.CheckString(arg)
			// Check that string doesn't contain embedded zeros
			L.ArgCheck(strings.IndexByte(s, 0) == -1, arg, "string contains zeros")
			buf = append(buf, s...)
			buf = append(buf, 0)
			totalsize += len(s) + 1
		case kPadding:
			buf = append(buf, 0)
			arg-- // undo increment (no argument consumed)
		case kPaddalign, kNop:
			arg-- // undo increment (no argument consumed)
		}
	}
	L.PushString(string(buf))
	return 1
}

// str_unpack implements string.unpack(fmt, s [, pos]).
// Matches C Lua's str_unpack in lstrlib.c.
func str_unpack(L *luaapi.State) int {
	fmtStr := L.CheckString(1)
	data := L.CheckString(2)
	ld := len(data)
	pos := posRelat(L.OptInteger(3, 1), ld) - 1 // posRelat returns 1-based, convert to 0-based
	if pos < 0 {
		pos = 0
	}
	L.ArgCheck(pos <= ld, 3, "initial position out of string")
	h := initHeader(L)
	n := 0 // number of results
	fmtPos := 0
	for fmtPos < len(fmtStr) {
		var size, ntoalign int
		opt := getdetails(&h, pos, fmtStr, &fmtPos, &size, &ntoalign)
		L.ArgCheck(ntoalign+size <= ld-pos, 2, "data string too short")
		pos += ntoalign // skip alignment
		n++
		switch opt {
		case kInt:
			v := readInt(data, pos, size, h.islittle, true)
			if size > 8 && !checkIntOverflow(data, pos, size, h.islittle, true) {
				L.ArgError(2, fmt.Sprintf("%d-byte integer does not fit into Lua Integer", size))
			}
			L.PushInteger(v)
		case kUint:
			v := readInt(data, pos, size, h.islittle, false)
			if size > 8 && !checkIntOverflow(data, pos, size, h.islittle, false) {
				L.ArgError(2, fmt.Sprintf("%d-byte integer does not fit into Lua Integer", size))
			}
			L.PushInteger(v)
		case kFloat:
			bits := uint32(readUint(data, pos, 4, h.islittle))
			L.PushNumber(float64(math.Float32frombits(bits)))
		case kNumber, kDouble:
			bits := readUint(data, pos, 8, h.islittle)
			L.PushNumber(math.Float64frombits(bits))
		case kChar:
			L.PushString(data[pos : pos+size])
		case kString:
			slen := int(readUint(data, pos, size, h.islittle))
			L.ArgCheck(slen <= ld-pos-size, 2, "data string too short")
			L.PushString(data[pos+size : pos+size+slen])
			pos += slen // skip string content
		case kZstr:
			end := pos
			for end < ld && data[end] != 0 {
				end++
			}
			L.ArgCheck(end < ld, 2, "unfinished string for format 'z'")
			L.PushString(data[pos:end])
			pos = end + 1 - size // adjust: size=0 for kZstr, but pos += size below
			// Actually kZstr has size=0, so pos = end+1 after pos += size
			pos = end + 1
			size = 0 // ensure pos += size doesn't double-count
		case kPaddalign, kPadding, kNop:
			n-- // undo increment (no result pushed)
		}
		pos += size
	}
	L.PushInteger(int64(pos + 1)) // return final position (1-based)
	return n + 1
}

func appendInt(buf []byte, v uint64, size int, little bool) []byte {
	b := make([]byte, size)
	// Sign extension byte: if the value is negative (bit 63 set), extend with 0xff
	var ext byte
	if int64(v) < 0 {
		ext = 0xff
	}
	if little {
		for i := 0; i < size; i++ {
			if i < 8 {
				b[i] = byte(v >> (uint(i) * 8))
			} else {
				b[i] = ext
			}
		}
	} else {
		// Big-endian: high bytes first
		for i := 0; i < size; i++ {
			byteIdx := size - 1 - i // byte position from LSB
			if byteIdx < 8 {
				b[i] = byte(v >> (uint(byteIdx) * 8))
			} else {
				b[i] = ext
			}
		}
	}
	return append(buf, b...)
}

func readUint(data string, pos, size int, little bool) uint64 {
	var v uint64
	// Only read up to 8 bytes into uint64; extra bytes are sign extension
	readSize := size
	if readSize > 8 {
		readSize = 8
	}
	if little {
		for i := readSize - 1; i >= 0; i-- {
			v = (v << 8) | uint64(data[pos+i])
		}
	} else {
		// Big-endian: skip leading (size-readSize) bytes, read last readSize
		offset := size - readSize
		for i := 0; i < readSize; i++ {
			v = (v << 8) | uint64(data[pos+offset+i])
		}
	}
	return v
}

// checkIntOverflow checks that extra bytes (beyond 8) are valid sign extension.
// For signed: extra bytes must match the sign of the 8-byte value.
// For unsigned: extra bytes must be 0x00.
// Returns true if the value fits, false if overflow.
func checkIntOverflow(data string, pos, size int, little, signed bool) bool {
	if size <= 8 {
		return true // no extra bytes
	}
	// Determine what the extra bytes should be
	var expected byte
	if signed {
		// Check sign bit of the 8-byte value
		var signByte byte
		if little {
			signByte = data[pos+7] // MSB of the 8-byte value in little-endian
		} else {
			signByte = data[pos+size-8] // MSB of the 8-byte value in big-endian
		}
		if signByte&0x80 != 0 {
			expected = 0xff
		}
	}
	// Check extra bytes
	if little {
		// Extra bytes are at positions [8, size)
		for i := 8; i < size; i++ {
			if data[pos+i] != expected {
				return false
			}
		}
	} else {
		// Extra bytes are at positions [0, size-8)
		for i := 0; i < size-8; i++ {
			if data[pos+i] != expected {
				return false
			}
		}
	}
	return true
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

// str_dump implements string.dump(function [, strip])
// Mirrors: str_dump in lstrlib.c
func str_dump(L *luaapi.State) int {
	L.CheckType(1, objectapi.TypeFunction)
	cl := L.GetLClosure(1)
	if cl == nil {
		L.ArgError(1, "Lua function expected")
	}
	strip := L.ToBoolean(2)
	data := vmapi.DumpProto(cl.Proto, strip)
	L.PushString(string(data))
	return 1
}

// OpenString opens the string library.
func OpenString(L *luaapi.State) int {
	strFuncs := map[string]luaapi.CFunction{
		"byte":     str_byte,
		"char":     str_char,
		"dump":     str_dump,
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
