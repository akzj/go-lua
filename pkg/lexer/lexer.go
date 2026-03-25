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
}

// NewLexer creates a new lexer for the given source code.
// The name parameter is used for error messages (typically the filename).
func NewLexer(source []byte, name string) *Lexer {
	return &Lexer{
		Scanner: *NewScanner(source),
		name:    name,
	}
}

// NextToken returns the next token from the source.
// It scans whitespace, comments, literals, identifiers, and operators.
// Returns TK_EOF when the end of the source is reached.
func (l *Lexer) NextToken() (Token, error) {
	l.buffer.Reset()

	for {
		switch l.current {
		case 0: // EOF
			return Token{Type: TK_EOF, Line: l.line, Column: l.column}, nil

		case '\n', '\r': // Newlines
			l.skipNewline()
			continue

		case ' ', '\t', '\f', '\v': // Whitespace
			l.Advance()
			continue

		case '-': // '-' or '--' (comment)
			l.Advance()
			if l.current != '-' {
				return Token{Type: TK_MINUS, Line: l.line, Column: l.column}, nil
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
			for !isNewline(l.current) && l.current != 0 {
				l.Advance()
			}
			continue

		case '[': // Long string or '['
			sep := l.skipSep()
			if sep >= 2 {
				str := l.readLongString(sep)
				return Token{Type: TK_STRING, Value: str, Line: l.line, Column: l.column}, nil
			} else if sep == 0 {
				return Token{}, l.Error("invalid long string delimiter")
			}
			return Token{Type: TK_LBRACK, Line: l.line, Column: l.column}, nil

		case '=': // '=' or '=='
			l.Advance()
			if l.Match('=') {
				return Token{Type: TK_EQ, Line: l.line, Column: l.column}, nil
			}
			return Token{Type: TK_DOT, Value: "=", Line: l.line, Column: l.column}, nil

		case '<': // '<', '<=', or '<<'
			l.Advance()
			if l.Match('=') {
				return Token{Type: TK_LE, Line: l.line, Column: l.column}, nil
			}
			if l.Match('<') {
				return Token{Type: TK_SHL, Line: l.line, Column: l.column}, nil
			}
			return Token{Type: TK_LT, Line: l.line, Column: l.column}, nil

		case '>': // '>', '>=', or '>>'
			l.Advance()
			if l.Match('=') {
				return Token{Type: TK_GE, Line: l.line, Column: l.column}, nil
			}
			if l.Match('>') {
				return Token{Type: TK_SHR, Line: l.line, Column: l.column}, nil
			}
			return Token{Type: TK_GT, Line: l.line, Column: l.column}, nil

		case '/': // '/' or '//'
			l.Advance()
			if l.Match('/') {
				return Token{Type: TK_IDIV, Line: l.line, Column: l.column}, nil
			}
			return Token{Type: TK_SLASH, Line: l.line, Column: l.column}, nil

		case '~': // '~' or '~='
			l.Advance()
			if l.Match('=') {
				return Token{Type: TK_NE, Line: l.line, Column: l.column}, nil
			}
			return Token{Type: TK_CARET, Value: "~", Line: l.line, Column: l.column}, nil

		case ':': // ':' or '::'
			l.Advance()
			if l.Match(':') {
				return Token{Type: TK_DBCOLON, Line: l.line, Column: l.column}, nil
			}
			return Token{Type: TK_COLON, Line: l.line, Column: l.column}, nil

		case '"', '\'': // String literals
			delim := l.current
			str, err := l.readString(delim)
			if err != nil {
				return Token{}, err
			}
			return Token{Type: TK_STRING, Value: str, Line: l.line, Column: l.column}, nil

		case '.': // '.', '..', '...', or number
			startPos := l.pos
			l.Advance()
			if l.Match('.') {
				if l.Match('.') {
					return Token{Type: TK_DOTS, Line: l.line, Column: l.column}, nil
				}
				return Token{Type: TK_CONCAT, Line: l.line, Column: l.column}, nil
			}
			if isDigit(l.current) {
				// Number starting with .
				l.buffer.Write(l.Substring(startPos, l.pos))
				return l.readNumber()
			}
			return Token{Type: TK_DOT, Line: l.line, Column: l.column}, nil

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
				return Token{Type: tok, Line: l.line, Column: l.column}, nil
			}
			return Token{Type: TK_NAME, Value: name, Line: l.line, Column: l.column}, nil

		default: // Single-character tokens
			c := l.current
			l.Advance()
			return l.singleCharToken(c)
		}
	}
}

// singleCharToken returns the token type for a single character.
func (l *Lexer) singleCharToken(c byte) (Token, error) {
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
		tok = TK_DOT
		l.buffer.WriteString("=")
	default:
		return Token{}, l.Error("unexpected character '%c'", c)
	}
	return Token{Type: tok, Line: l.line, Column: l.column}, nil
}

// skipNewline skips a newline sequence (\n, \r, \n\r, or \r\n).
func (l *Lexer) skipNewline() {
	c := l.current
	l.Advance()
	if isNewline(l.current) && l.current != c {
		l.Advance()
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
		if l.current == 0 {
			return // EOF
		}
		if l.current == ']' {
			if l.checkSep(sep) {
				l.Advance() // Skip closing ]
				return
			}
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
	pos := l.pos
	l.Advance()
	count := 0
	for l.current == '=' {
		l.Advance()
		count++
	}
	result := l.current == ']' && count+2 == sep
	// Restore position
	l.pos = pos
	l.column = pos
	if pos < len(l.source) {
		l.current = l.source[pos]
	}
	return result
}

// readLongString reads a long string with the given separator level.
func (l *Lexer) readLongString(sep int) string {
	l.Advance() // Skip second [
	if isNewline(l.current) {
		l.skipNewline()
	}
	startPos := l.pos
	for {
		if l.current == 0 {
			return "" // Will be caught by error handling
		}
		if l.current == ']' {
			if l.checkSep(sep) {
				result := string(l.Substring(startPos, l.pos))
				l.Advance() // Skip ]
				l.Advance() // Skip =...=
				for i := 0; i < countFromSep(sep)-2; i++ {
					l.Advance()
				}
				l.Advance() // Skip final ]
				return result
			}
		} else if isNewline(l.current) {
			l.skipNewline()
		} else {
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
		if l.current == 0 {
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
				l.skipNewline()
			case 'x': // \xHH
				l.Advance()
				hex, err := l.readHex()
				if err != nil {
					return "", err
				}
				l.buffer.WriteByte(hex)
			case 'u': // \u{X}
				l.Advance()
				rune, err := l.readUTF8()
				if err != nil {
					return "", err
				}
				l.buffer.WriteRune(rune)
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

// readNumber reads a numeric literal (integer, float, or hex).
func (l *Lexer) readNumber() (Token, error) {
	startPos := l.pos - 1
	isHex := false

	// Check for hex prefix
	if l.current == '0' && (l.Peek1() == 'x' || l.Peek1() == 'X') {
		l.Advance() // Skip 0
		l.Advance() // Skip x/X
		isHex = true
	}

	// Read integer part
	l.SkipWhile(func(c byte) bool {
		if isHex {
			return isHexDigit(c)
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
		val, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return Token{}, l.Error("malformed number: %v", err)
		}
		return Token{Type: TK_FLOAT, Value: val, Line: l.line, Column: l.column}, nil
	}

	val, err := strconv.ParseInt(numStr, 0, 64)
	if err != nil {
		// Try parsing as float if int parsing fails
		fval, ferr := strconv.ParseFloat(numStr, 64)
		if ferr != nil {
			return Token{}, l.Error("malformed number: %v", err)
		}
		return Token{Type: TK_FLOAT, Value: fval, Line: l.line, Column: l.column}, nil
	}
	return Token{Type: TK_INT, Value: val, Line: l.line, Column: l.column}, nil
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
func (l *Lexer) Error(format string, args ...interface{}) error {
	return fmt.Errorf("%s:%d: %s", l.name, l.line, fmt.Sprintf(format, args...))
}