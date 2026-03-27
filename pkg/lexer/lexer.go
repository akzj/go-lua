// Package lexer implements the Lua lexical analyzer.
//
// This file provides the main Lexer struct and NextToken implementation,
// following the semantics from lua-master/llex.c.

package lexer

import (
	"fmt"
	"strconv"
	"strings"
)

// Token represents a lexical token produced by the lexer.
type Token struct {
	Type   TokenType
	Value  interface{} // string, int64, float64
	Line   int
	Column int
}

// Lexer performs lexical analysis on Lua source code.
// It scans the source and produces a stream of tokens.
type Lexer struct {
	Scanner              // Embed scanner for character operations
	name     string      // Source name for error messages
	buffer   strings.Builder // Buffer for building token strings
	atStart  bool        // Track if we're at the start of the file (for shebang handling)
}

// NewLexer creates a new lexer for the given source code.
// The name parameter is used for error messages (typically the filename).
func NewLexer(source []byte, name string) *Lexer {
	return &Lexer{
		Scanner: *NewScanner(source),
		name:    name,
		atStart: true,
	}
}

// NextToken returns the next token from the source.
// It scans whitespace, comments, literals, identifiers, and operators.
// Returns TK_EOF when the end of the source is reached.
func (l *Lexer) NextToken() (Token, error) {
	l.buffer.Reset()

	// Handle first line starting with '#' (shebang or comment)
	// Per Lua spec, if the first line starts with '#', the entire line is skipped
	if l.atStart && l.current == '#' {
		l.atStart = false
		// Skip the entire shebang line
		for !isNewline(l.current) && !l.AtEnd() {
			l.Advance()
		}
		// Skip the newline if present
		if isNewline(l.current) {
			l.skipNewline()
		}
	}
	l.atStart = false // Mark that we've started parsing

	for {
		// Save start position for this token
		tokenLine := l.line
		tokenColumn := l.column

		switch l.current {
		case 0: // EOF or NUL byte
			if l.AtEnd() {
				return Token{Type: TK_EOF, Line: tokenLine, Column: tokenColumn}, nil
			}
			// NUL byte in source - treat as unexpected character
			return Token{}, l.Error("unexpected character '\\x00'")

		case '\n', '\r': // Newlines
			l.skipNewline()
			continue

		case ' ', '\t', '\f', '\v': // Whitespace
			l.Advance()
			continue

		case '-': // '-' or '--' (comment)
			l.Advance()
			if l.current != '-' {
				return Token{Type: TK_MINUS, Line: tokenLine, Column: tokenColumn}, nil
			}
			// Comment
			l.Advance()
			if l.current == '[' {
				sep := l.skipSep()
				if sep >= 2 {
					l.skipLongString(sep) // Skip long comment
					l.buffer.Reset()
					continue
				}
			}
			// Short comment - skip until end of line
			for !isNewline(l.current) && !l.AtEnd() {
				l.Advance()
			}
			continue

		case '[': // Long string or '['
			sep := l.skipSep()
			if sep >= 2 {
				str := l.readLongString(sep)
				return Token{Type: TK_STRING, Value: str, Line: tokenLine, Column: tokenColumn}, nil
			} else if sep == 0 {
				return Token{}, l.Error("invalid long string delimiter")
			}
			return Token{Type: TK_LBRACK, Line: tokenLine, Column: tokenColumn}, nil

		case '=': // '=' or '=='
			l.Advance()
			if l.Match('=') {
				return Token{Type: TK_EQ, Line: tokenLine, Column: tokenColumn}, nil
			}
			return Token{Type: TK_ASSIGN, Line: tokenLine, Column: tokenColumn}, nil

		case '<': // '<', '<=', or '<<'
			l.Advance()
			if l.Match('=') {
				return Token{Type: TK_LE, Line: tokenLine, Column: tokenColumn}, nil
			}
			if l.Match('<') {
				return Token{Type: TK_SHL, Line: tokenLine, Column: tokenColumn}, nil
			}
			return Token{Type: TK_LT, Line: tokenLine, Column: tokenColumn}, nil

		case '>': // '>', '>=', or '>>'
			l.Advance()
			if l.Match('=') {
				return Token{Type: TK_GE, Line: tokenLine, Column: tokenColumn}, nil
			}
			if l.Match('>') {
				return Token{Type: TK_SHR, Line: tokenLine, Column: tokenColumn}, nil
			}
			return Token{Type: TK_GT, Line: tokenLine, Column: tokenColumn}, nil

		case '/': // '/' or '//'
			l.Advance()
			if l.Match('/') {
				return Token{Type: TK_IDIV, Line: tokenLine, Column: tokenColumn}, nil
			}
			return Token{Type: TK_SLASH, Line: tokenLine, Column: tokenColumn}, nil

		case '~': // '~' or '~='
			l.Advance()
			if l.Match('=') {
				return Token{Type: TK_NE, Line: tokenLine, Column: tokenColumn}, nil
			}
			return Token{Type: TK_BXOR, Line: tokenLine, Column: tokenColumn}, nil

		case '&': // '&'
			l.Advance()
			return Token{Type: TK_BAND, Line: tokenLine, Column: tokenColumn}, nil

		case '|': // '|'
			l.Advance()
			return Token{Type: TK_BOR, Line: tokenLine, Column: tokenColumn}, nil

		case ':': // ':' or '::'
			l.Advance()
			if l.Match(':') {
				return Token{Type: TK_DBCOLON, Line: tokenLine, Column: tokenColumn}, nil
			}
			return Token{Type: TK_COLON, Line: tokenLine, Column: tokenColumn}, nil

		case '"', '\'': // String literals
			delim := l.current
			str, err := l.readString(delim)
			if err != nil {
				return Token{}, err
			}
			return Token{Type: TK_STRING, Value: str, Line: tokenLine, Column: tokenColumn}, nil

		case '.': // '.', '..', '...', or number
			l.Advance()
			if l.Match('.') {
				if l.Match('.') {
					return Token{Type: TK_DOTS, Line: tokenLine, Column: tokenColumn}, nil
				}
				return Token{Type: TK_CONCAT, Line: tokenLine, Column: tokenColumn}, nil
			}
			if isDigit(l.current) {
				// Number starting with .
				return l.readNumberStartingWithDot()
			}
			return Token{Type: TK_DOT, Line: tokenLine, Column: tokenColumn}, nil

		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9': // Numbers
			return l.readNumber()

		case 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
		     'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		     'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
		     'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z', '_':
			// Identifier or keyword
			startPos := l.pos
			l.SkipWhile(isAlphaNum)
			name := string(l.Substring(startPos, l.pos))
			if tok, ok := Keywords[name]; ok {
				return Token{Type: tok, Line: tokenLine, Column: tokenColumn}, nil
			}
			return Token{Type: TK_NAME, Value: name, Line: tokenLine, Column: tokenColumn}, nil

		default: // Single-character tokens
			c := l.current
			l.Advance()
			return l.singleCharToken(c, tokenLine, tokenColumn)
		}
	}
}

// singleCharToken returns the token type for a single character.
func (l *Lexer) singleCharToken(c byte, line, column int) (Token, error) {
	var tok TokenType
	switch c {
	case '+':
		tok = TK_PLUS
	case '-':
		tok = TK_MINUS
	case '*':
		tok = TK_STAR
	case '/':
		tok = TK_SLASH
	case '%':
		tok = TK_PERCENT
	case '^':
		tok = TK_CARET
	case '#':
		tok = TK_HASH
	case '(':
		tok = TK_LPAREN
	case ')':
		tok = TK_RPAREN
	case '{':
		tok = TK_LBRACE
	case '}':
		tok = TK_RBRACE
	case '[':
		tok = TK_LBRACK
	case ']':
		tok = TK_RBRACK
	case ';':
		tok = TK_SEMICOLON
	case ':':
		tok = TK_COLON
	case ',':
		tok = TK_COMMA
	case '.':
		tok = TK_DOT
	case '=':
		tok = TK_ASSIGN
	default:
		return Token{}, l.Error("unexpected character '%c'", c)
	}
	return Token{Type: tok, Line: line, Column: column}, nil
}

// skipNewline skips a newline sequence (\n, \r, \n\r, or \r\n).
// It always increments the line counter once.
func (l *Lexer) skipNewline() {
	c := l.current
	// Always count one line for the newline
	l.line++
	l.column = 0
	// Advance past the first newline character
	l.pos++
	if l.pos < len(l.source) {
		l.current = l.source[l.pos]
	} else {
		l.current = 0
	}
	// If the next char is also a newline but different type, skip it too (\r\n or \n\r)
	if isNewline(l.current) && l.current != c {
		l.pos++
		if l.pos < len(l.source) {
			l.current = l.source[l.pos]
		} else {
			l.current = 0
		}
	}
}

// skipSep skips a separator [=*[ or ]=*] and returns the level (number of = + 2).
// Returns 0 if malformed, 1 if single bracket.
func (l *Lexer) skipSep() int {
	l.Advance() // Skip initial [ or ]
	count := 0
	for l.current == '=' {
		l.Advance()
		count++
	}
	if l.current == '[' || l.current == ']' {
		return count + 2
	}
	if count == 0 {
		return 1
	}
	return 0
}

// skipLongString skips a long string/comment with the given separator level.
func (l *Lexer) skipLongString(sep int) {
	l.Advance() // Skip second [
	if isNewline(l.current) {
		l.skipNewline()
	}
	for {
		if l.AtEnd() {
			return // EOF
		}
		if l.current == ']' {
			if l.checkSep(sep) {
				// Skip closing separator ]=...=]
				for i := 0; i < sep; i++ {
					l.Advance()
				}
				return
			}
			// Not the closing separator, just a ] character - advance
			l.Advance()
		} else if isNewline(l.current) {
			l.skipNewline()
		} else {
			l.Advance()
		}
	}
}

// checkSep checks if the current position has a closing separator of the given level.
// Does not advance the scanner.
func (l *Lexer) checkSep(sep int) bool {
	// Save state
	savedPos := l.pos
	savedColumn := l.column
	savedCurrent := l.current

	l.Advance()
	count := 0
	for l.current == '=' {
		l.Advance()
		count++
	}
	result := l.current == ']' && count+2 == sep

	// Restore state
	l.pos = savedPos
	l.column = savedColumn
	l.current = savedCurrent
	return result
}

// readLongString reads a long string with the given separator level.
func (l *Lexer) readLongString(sep int) string {
	l.Advance() // Skip second [
	if isNewline(l.current) {
		l.skipNewline()
	}
	// Build the string with \r normalization
	var result strings.Builder
	for {
		if l.AtEnd() {
			return "" // Will be caught by error handling
		}
		if l.current == ']' {
			if l.checkSep(sep) {
				// Skip the closing separator: ]=...=]
				for i := 0; i < sep; i++ {
					l.Advance()
				}
				return result.String()
			}
			// Not the closing separator, just a ] character
			result.WriteByte(']')
			l.Advance()
		} else if isNewline(l.current) {
			// Normalize all newline sequences to \n
			result.WriteByte('\n')
			l.skipNewline()
		} else {
			result.WriteByte(l.current)
			l.Advance()
		}
	}
}

// countFromSep returns the number of = signs from the separator level.
func countFromSep(sep int) int {
	return sep - 2
}

// readString reads a short string literal with the given delimiter.
func (l *Lexer) readString(delim byte) (string, error) {
	l.Advance() // Skip opening delimiter

	for l.current != delim {
		if l.AtEnd() {
			return "", l.Error("unfinished string")
		}
		if isNewline(l.current) {
			return "", l.Error("unfinished string (newline in string)")
		}
		if l.current == '\\' {
			// Handle escape sequences
			l.Advance()
			switch l.current {
			case 'a':
				l.buffer.WriteByte('\a')
				l.Advance()
			case 'b':
				l.buffer.WriteByte('\b')
				l.Advance()
			case 'f':
				l.buffer.WriteByte('\f')
				l.Advance()
			case 'n':
				l.buffer.WriteByte('\n')
				l.Advance()
			case 'r':
				l.buffer.WriteByte('\r')
				l.Advance()
			case 't':
				l.buffer.WriteByte('\t')
				l.Advance()
			case 'v':
				l.buffer.WriteByte('\v')
				l.Advance()
			case '\\':
				l.buffer.WriteByte('\\')
				l.Advance()
			case '"':
				l.buffer.WriteByte('"')
				l.Advance()
			case '\'':
				l.buffer.WriteByte('\'')
				l.Advance()
			case '\n', '\r':
				// \<newline> is an escape sequence that produces a newline character
				// (not line continuation - Lua 5.1/5.5 semantics)
				l.buffer.WriteByte('\n')
				l.skipNewline()
			case 'x': // \xHH
				l.Advance()
				// Check if we have hex digits
				if !isHexDigit(l.current) {
					// Build the near context including \x and whatever follows
					near := "\\x"
					if l.current != 0 && l.current != '\n' && l.current != '\r' {
						near += string(l.current)
					}
					return "", l.ErrorNear(near, "hexadecimal digit expected")
				}
				hex, err := l.readHex()
				if err != nil {
					return "", err
				}
				l.buffer.WriteByte(hex)
			case 'u': // \u{X}
				l.Advance()
				r, err := l.readUTF8()
				if err != nil {
					return "", err
				}
				// Write UTF-8 bytes directly, even for invalid code points
				// Lua allows code points up to 0x7FFFFFFF for raw byte sequences
				l.writeUTF8Bytes(r)
			case 'z': // \z (skip whitespace)
				l.Advance()
				for isSpace(l.current) || isNewline(l.current) {
					if isNewline(l.current) {
						l.skipNewline()
					} else {
						l.Advance()
					}
				}
			default:
				if isDigit(l.current) {
					// \ddd decimal escape
					val, err := l.readDecimal()
					if err != nil {
						return "", err
					}
					l.buffer.WriteByte(byte(val))
				} else {
					return "", l.Error("invalid escape sequence")
				}
			}
		} else {
			l.buffer.WriteByte(l.current)
			l.Advance()
		}
	}
	l.Advance() // Skip closing delimiter
	return l.buffer.String(), nil
}

// readHex reads a two-digit hexadecimal number.
func (l *Lexer) readHex() (byte, error) {
	if !isHexDigit(l.current) {
		return 0, l.Error("hexadecimal digit expected")
	}
	val := hexValue(l.current) << 4
	l.Advance()
	if !isHexDigit(l.current) {
		return 0, l.Error("hexadecimal digit expected")
	}
	val |= hexValue(l.current)
	l.Advance()
	return val, nil
}

// readUTF8 reads a UTF-8 escape sequence \u{X}.
func (l *Lexer) readUTF8() (rune, error) {
	if l.current != '{' {
		return 0, l.Error("missing '{' in UTF-8 escape")
	}
	l.Advance()
	var val rune
	for isHexDigit(l.current) {
		val = (val << 4) | rune(hexValue(l.current))
		if val > 0x7FFFFFFF {
			return 0, l.Error("UTF-8 value too large")
		}
		l.Advance()
	}
	if l.current != '}' {
		return 0, l.Error("missing '}' in UTF-8 escape")
	}
	l.Advance()
	return val, nil
}

// writeUTF8Bytes writes a code point as UTF-8 bytes to the buffer.
// This handles the full range of code points (0 to 0x7FFFFFFF),
// including invalid ones beyond Unicode's U+10FFFF limit.
// Lua uses this for testing raw UTF-8 byte sequences.
func (l *Lexer) writeUTF8Bytes(r rune) {
	switch {
	case r <= 0x7F:
		l.buffer.WriteByte(byte(r))
	case r <= 0x7FF:
		l.buffer.WriteByte(byte(0xC0 | (r >> 6)))
		l.buffer.WriteByte(byte(0x80 | (r & 0x3F)))
	case r <= 0xFFFF:
		l.buffer.WriteByte(byte(0xE0 | (r >> 12)))
		l.buffer.WriteByte(byte(0x80 | ((r >> 6) & 0x3F)))
		l.buffer.WriteByte(byte(0x80 | (r & 0x3F)))
	case r <= 0x1FFFFF:
		l.buffer.WriteByte(byte(0xF0 | (r >> 18)))
		l.buffer.WriteByte(byte(0x80 | ((r >> 12) & 0x3F)))
		l.buffer.WriteByte(byte(0x80 | ((r >> 6) & 0x3F)))
		l.buffer.WriteByte(byte(0x80 | (r & 0x3F)))
	case r <= 0x3FFFFFF:
		l.buffer.WriteByte(byte(0xF8 | (r >> 24)))
		l.buffer.WriteByte(byte(0x80 | ((r >> 18) & 0x3F)))
		l.buffer.WriteByte(byte(0x80 | ((r >> 12) & 0x3F)))
		l.buffer.WriteByte(byte(0x80 | ((r >> 6) & 0x3F)))
		l.buffer.WriteByte(byte(0x80 | (r & 0x3F)))
	default: // up to 0x7FFFFFFF
		l.buffer.WriteByte(byte(0xFC | (r >> 30)))
		l.buffer.WriteByte(byte(0x80 | ((r >> 24) & 0x3F)))
		l.buffer.WriteByte(byte(0x80 | ((r >> 18) & 0x3F)))
		l.buffer.WriteByte(byte(0x80 | ((r >> 12) & 0x3F)))
		l.buffer.WriteByte(byte(0x80 | ((r >> 6) & 0x3F)))
		l.buffer.WriteByte(byte(0x80 | (r & 0x3F)))
	}
}

// readDecimal reads a decimal escape sequence (up to 3 digits).
func (l *Lexer) readDecimal() (int, error) {
	val := 0
	for i := 0; i < 3 && isDigit(l.current); i++ {
		val = val*10 + int(l.current-'0')
		l.Advance()
	}
	if val > 255 {
		return 0, l.Error("decimal escape too large")
	}
	return val, nil
}

// hexValue returns the numeric value of a hex digit.
func hexValue(c byte) byte {
	if c >= '0' && c <= '9' {
		return c - '0'
	}
	if c >= 'a' && c <= 'f' {
		return c - 'a' + 10
	}
	if c >= 'A' && c <= 'F' {
		return c - 'A' + 10
	}
	return 0
}


// readNumberStartingWithDot reads a numeric literal that starts with a decimal point.
func (l *Lexer) readNumberStartingWithDot() (Token, error) {
	tokenLine := l.line
	tokenColumn := l.column
	startPos := l.pos - 1 // Include the '.'

	// Read fractional part
	l.SkipWhile(isDigit)

	// Check for exponent
	if l.current == 'e' || l.current == 'E' {
		l.Advance()
		if l.current == '+' || l.current == '-' {
			l.Advance()
		}
		l.SkipWhile(isDigit)
	}

	numStr := string(l.Substring(startPos, l.pos))

	// Check for invalid suffix (letter after number)
	if isAlpha(l.current) {
		return Token{}, l.Error("malformed number")
	}

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return Token{}, l.Error("malformed number: %v", err)
	}
	return Token{Type: TK_FLOAT, Value: val, Line: tokenLine, Column: tokenColumn}, nil
}

// readNumber reads a numeric literal (integer, float, or hex).
func (l *Lexer) readNumber() (Token, error) {
	tokenLine := l.line
	tokenColumn := l.column
	startPos := l.pos
	isHex := false
	isOctal := false

	// Check for hex prefix (0x or 0X)
	if l.current == '0' && (l.Peek1() == 'x' || l.Peek1() == 'X') {
		l.Advance() // Skip 0
		l.Advance() // Skip x/X
		isHex = true
	} else if l.current == '0' && (l.Peek1() == 'o' || l.Peek1() == 'O') {
		// Check for octal prefix (0o or 0O) - Lua 5.4+ syntax
		l.Advance() // Skip 0
		l.Advance() // Skip o/O
		isOctal = true
	}

	// Read integer part
	l.SkipWhile(func(c byte) bool {
		if isHex {
			return isHexDigit(c)
		}
		if isOctal {
			return c >= '0' && c <= '7'
		}
		return isDigit(c)
	})

	// Check for decimal point
	isFloat := false
	if l.current == '.' {
		isFloat = true
		l.Advance()
		l.SkipWhile(func(c byte) bool {
			if isHex {
				return isHexDigit(c)
			}
			return isDigit(c)
		})
	}

	// Check for exponent
	if isHex {
		if l.current == 'p' || l.current == 'P' {
			isFloat = true
			l.Advance()
			if l.current == '+' || l.current == '-' {
				l.Advance()
			}
			l.SkipWhile(isDigit)
		}
	} else {
		if l.current == 'e' || l.current == 'E' {
			isFloat = true
			l.Advance()
			if l.current == '+' || l.current == '-' {
				l.Advance()
			}
			l.SkipWhile(isDigit)
		}
	}

	numStr := string(l.Substring(startPos, l.pos))

	// Check for invalid suffix (letter after number)
	if isAlpha(l.current) {
		return Token{}, l.Error("malformed number")
	}

	if isFloat {
		// For hex floats without exponent, add p0 (Lua allows this, Go requires p)
		if isHex && !strings.ContainsAny(numStr, "pP") {
			numStr = numStr + "p0"
		}
		val, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return Token{}, l.Error("malformed number: %v", err)
		}
		return Token{Type: TK_FLOAT, Value: val, Line: tokenLine, Column: tokenColumn}, nil
	}

	// Parse the number with the appropriate base
	var val int64
	var err error
	if isHex {
		// Hex: strip 0x prefix and parse with base 16
		hexStr := strings.TrimPrefix(strings.TrimPrefix(numStr, "0x"), "0X")
		val, err = strconv.ParseInt(hexStr, 16, 64)
		if err != nil {
			// Try as uint64 for large values
			uval, uerr := strconv.ParseUint(hexStr, 16, 64)
			if uerr == nil {
				return Token{Type: TK_FLOAT, Value: float64(uval), Line: tokenLine, Column: tokenColumn}, nil
			}
		}
	} else if isOctal {
		// Octal: strip 0o prefix and parse with base 8
		octStr := strings.TrimPrefix(strings.TrimPrefix(numStr, "0o"), "0O")
		val, err = strconv.ParseInt(octStr, 8, 64)
	} else {
		// Decimal: use base 10 (NOT auto-detect, to avoid treating 010 as octal)
		val, err = strconv.ParseInt(numStr, 10, 64)
	}
	
	if err != nil {
		// Try parsing as float if int parsing fails
		fval, ferr := strconv.ParseFloat(numStr, 64)
		if ferr != nil {
			return Token{}, l.Error("malformed number: %v", err)
		}
		return Token{Type: TK_FLOAT, Value: fval, Line: tokenLine, Column: tokenColumn}, nil
	}
	
	return Token{Type: TK_INT, Value: val, Line: tokenLine, Column: tokenColumn}, nil
}

// Peek returns the next token without consuming it.
// Note: This is a simplified implementation that just calls NextToken.
// A full implementation would need to buffer the token.
func (l *Lexer) Peek() Token {
	// Save state
	savedPos := l.pos
	savedLine := l.line
	savedColumn := l.column
	savedCurrent := l.current

	tok, _ := l.NextToken()

	// Restore state
	l.pos = savedPos
	l.line = savedLine
	l.column = savedColumn
	l.current = savedCurrent
	if l.pos < len(l.Scanner.source) {
		l.current = l.Scanner.source[l.pos]
	}

	return tok
}

// Error creates a lexer error with the current position.
// Lua error format: "source:line: message near 'token'"
func (l *Lexer) Error(format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	// Get the current token context for "near" part
	var near string
	if l.current != 0 {
		// Show the current character or context
		if l.current == '\n' || l.current == '\r' {
			near = "<newline>"
		} else if l.current == '\\' {
			// For escape sequence errors, show the escape
			near = "\\"
			if l.pos < len(l.source) {
				next := l.source[l.pos]
				if next != 0 {
					near = "\\" + string(next)
				}
			}
		} else {
			near = string(l.current)
		}
		return fmt.Errorf("%s:%d: %s near '%s'", l.name, l.line, msg, near)
	}
	return fmt.Errorf("%s:%d: %s", l.name, l.line, msg)
}

// ErrorNear creates a lexer error with a specific near context.
func (l *Lexer) ErrorNear(near, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s:%d: %s near '%s'", l.name, l.line, msg, near)
}

// Name returns the source name (filename) for error messages.
func (l *Lexer) Name() string {
	return l.name
}