// String↔number coercion and number formatting for Lua values.
//
// Reference: lua-master/lobject.c (luaO_str2num, tostringbuffFloat, l_str2int, l_str2d)
package api

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// ---------------------------------------------------------------------------
// Number formatting constants
// ---------------------------------------------------------------------------

// LuaNumberFmt is the default format for writing floats (%.15g for double).
const LuaNumberFmt = "%.15g"

// LuaNumberFmtN is the higher-precision format used when %.15g loses precision.
const LuaNumberFmtN = "%.17g"

// ---------------------------------------------------------------------------
// Number → String
// ---------------------------------------------------------------------------

// IntegerToString converts a Lua integer to its string representation.
// Equivalent to C's lua_integer2str: plain decimal format.
func IntegerToString(i int64) string {
	return strconv.FormatInt(i, 10)
}

// FloatToString converts a Lua float to its string representation.
//
// Algorithm (from C lobject.c tostringbuffFloat):
//  1. Format with %.15g
//  2. Parse back; if the value differs, re-format with %.17g
//  3. If the result looks like an integer (no '.', no 'e/E'), append ".0"
//
// Special cases: +Inf → "inf", -Inf → "-inf", NaN → "-nan" or "nan".
func FloatToString(f float64) string {
	// Handle special values like C Lua does
	if math.IsInf(f, 1) {
		return "inf"
	}
	if math.IsInf(f, -1) {
		return "-inf"
	}
	if math.IsNaN(f) {
		return "-nan"
	}

	// First try with default precision
	s := fmt.Sprintf(LuaNumberFmt, f)

	// Read it back to check for precision loss
	check, err := strconv.ParseFloat(s, 64)
	if err != nil || check != f {
		// Not enough precision, use higher precision
		s = fmt.Sprintf(LuaNumberFmtN, f)
	}

	// If result looks like an integer (no '.', no 'e', no 'E', no 'n', no 'i'),
	// append ".0" to distinguish from integer representation.
	if !strings.ContainsAny(s, ".eEinIN") {
		s += ".0"
	}

	return s
}

// ---------------------------------------------------------------------------
// String → Number
// ---------------------------------------------------------------------------

// StringToNumber attempts to convert a string to a Lua number.
// It tries integer first, then float (matching C's luaO_str2num).
// Returns the TValue and true on success, or (Nil, false) on failure.
func StringToNumber(s string) (TValue, bool) {
	// Try integer first (handles decimal and hex)
	if i, ok := StringToInteger(s); ok {
		return MakeInteger(i), true
	}
	// Then try float
	if f, ok := StringToFloat(s); ok {
		return MakeFloat(f), true
	}
	return Nil, false
}

// StringToInteger attempts to parse a string as a Lua integer.
// Supports decimal and hexadecimal (0x/0X prefix).
// Skips leading/trailing whitespace. Rejects overflow for decimal.
//
// Reference: lua-master/lobject.c l_str2int
func StringToInteger(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0, false
	}

	neg := false
	idx := 0

	// Check sign
	if s[idx] == '-' {
		neg = true
		idx++
	} else if s[idx] == '+' {
		idx++
	}

	if idx >= len(s) {
		return 0, false
	}

	var a uint64
	empty := true

	if idx+1 < len(s) && s[idx] == '0' && (s[idx+1] == 'x' || s[idx+1] == 'X') {
		// Hexadecimal
		idx += 2
		for idx < len(s) {
			c := s[idx]
			var d uint64
			switch {
			case c >= '0' && c <= '9':
				d = uint64(c - '0')
			case c >= 'a' && c <= 'f':
				d = uint64(c-'a') + 10
			case c >= 'A' && c <= 'F':
				d = uint64(c-'A') + 10
			default:
				goto done
			}
			a = a*16 + d
			empty = false
			idx++
		}
	} else {
		// Decimal — with overflow detection matching C
		const maxBy10 = uint64(math.MaxInt64) / 10
		const maxLastD = int(uint64(math.MaxInt64) % 10)

		for idx < len(s) {
			c := s[idx]
			if c < '0' || c > '9' {
				break
			}
			d := int(c - '0')
			if a >= maxBy10 && (a > maxBy10 || d > maxLastD+boolToInt(neg)) {
				return 0, false // overflow
			}
			a = a*10 + uint64(d)
			empty = false
			idx++
		}
	}

done:
	// Skip trailing whitespace
	for idx < len(s) && isLuaSpace(s[idx]) {
		idx++
	}

	if empty || idx != len(s) {
		return 0, false
	}

	if neg {
		return -int64(a), true
	}
	return int64(a), true
}

// StringToFloat attempts to parse a string as a Lua float.
// Supports decimal, hexadecimal floats (0x with optional p exponent).
// Rejects "inf" and "nan" (Lua does not accept these as valid number literals).
//
// Reference: lua-master/lobject.c l_str2d
func StringToFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0, false
	}

	// Check for special chars to determine mode
	lower := strings.ToLower(s)
	// Reject 'inf' and 'nan' — Lua does not accept these
	if strings.Contains(lower, "n") || strings.Contains(lower, "i") {
		// But allow hex floats which may contain these in the hex digits
		if !strings.HasPrefix(lower, "0x") && !strings.HasPrefix(lower, "-0x") && !strings.HasPrefix(lower, "+0x") {
			return 0, false
		}
	}

	// Use Go's strconv.ParseFloat which handles hex floats (0x with p exponent)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		// Overflow (e.g. 1e9999) → ±Inf is valid in Lua
		if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
			if math.IsInf(f, 0) {
				return f, true
			}
		}
		// Try hex float manually if Go doesn't support the exact format
		if f2, ok := parseHexFloat(s); ok {
			return f2, true
		}
		return 0, false
	}

	return f, true
}

// parseHexFloat parses a hexadecimal floating-point literal.
// Format: [+-]0x<hex>[.<hex>][p[+-]<dec>]
// This handles the C99 hex float format that Lua supports.
func parseHexFloat(s string) (float64, bool) {
	idx := 0
	neg := false

	if idx < len(s) && (s[idx] == '-' || s[idx] == '+') {
		neg = s[idx] == '-'
		idx++
	}

	if idx+1 >= len(s) || s[idx] != '0' || (s[idx+1] != 'x' && s[idx+1] != 'X') {
		return 0, false
	}
	idx += 2

	var r float64
	sigdig := 0
	nosigdig := 0
	hasdot := false
	e := 0

	for idx < len(s) {
		c := s[idx]
		if c == '.' {
			if hasdot {
				break
			}
			hasdot = true
			idx++
			continue
		}

		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		default:
			goto afterDigits
		}

		if sigdig == 0 && d == 0 { // non-significant zero
			nosigdig++
		} else if sigdig++; sigdig <= 30 { // MAXSIGDIG = 30
			r = r*16 + float64(d)
		} else {
			e++ // too many significant digits; ignore but count for exponent
		}
		if hasdot {
			e--
		}
		idx++
	}

afterDigits:
	if nosigdig+sigdig == 0 {
		return 0, false
	}

	e *= 4 // each hex digit = 4 binary digits

	// Exponent part: p or P
	if idx < len(s) && (s[idx] == 'p' || s[idx] == 'P') {
		idx++
		expNeg := false
		if idx < len(s) && (s[idx] == '-' || s[idx] == '+') {
			expNeg = s[idx] == '-'
			idx++
		}
		if idx >= len(s) || s[idx] < '0' || s[idx] > '9' {
			return 0, false
		}
		exp1 := 0
		for idx < len(s) && s[idx] >= '0' && s[idx] <= '9' {
			exp1 = exp1*10 + int(s[idx]-'0')
			idx++
		}
		if expNeg {
			exp1 = -exp1
		}
		e += exp1
	}

	// Skip trailing whitespace
	for idx < len(s) && isLuaSpace(s[idx]) {
		idx++
	}
	if idx != len(s) {
		return 0, false
	}

	if neg {
		r = -r
	}

	return math.Ldexp(r, e), true
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func isLuaSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// IsLuaSpace reports whether the rune is a Lua whitespace character.
func IsLuaSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v'
}

// IsLuaDigit reports whether the rune is an ASCII digit.
func IsLuaDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// IsLuaAlpha reports whether the rune is a Lua "alpha" character (letter or _).
func IsLuaAlpha(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

// IsLuaAlNum reports whether the rune is alphanumeric or underscore.
func IsLuaAlNum(r rune) bool {
	return IsLuaAlpha(r) || IsLuaDigit(r)
}

// FloatToInteger checks if float f has an integer representation
// (fits in int64 without loss of precision). Returns (int, true) if so,
// or (0, false) if f is not an integer or overflows int64.
// Mirrors: luaO_cast_number2int in lobject.c.
func FloatToInteger(f float64) (int64, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	i := int64(f)
	if float64(i) == f && i != 0 {
		return i, true
	}
	if f == 0 {
		return 0, true
	}
	return 0, false
}
