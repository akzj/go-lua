// Package internal provides the Lua lexical analyzer implementation.
package internal

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/akzj/go-lua/lex/api"
)

// lexer is the concrete implementation of api.Lexer.
type lexer struct {
	source     []byte
	sourceName string
	pos        int // current byte position
	line       int // current line (1-based)
	column     int // current column (1-based)

	// Lookahead token cache
	lookaheadToken api.Token
	lookaheadValid bool
}

// NewLexer creates a new lexer for the given source.
func NewLexer(source, sourceName string) api.Lexer {
	return &lexer{
		source:     []byte(source),
		sourceName: sourceName,
		line:       1,
		column:     1,
	}
}

// Current implements api.Lexer.
func (l *lexer) CurrentLine() int {
	return l.line
}

// CurrentColumn implements api.Lexer.
func (l *lexer) CurrentColumn() int {
	return l.column
}

// SourceName implements api.Lexer.
func (l *lexer) SourceName() string {
	return l.sourceName
}

// Error implements api.Lexer.
func (l *lexer) Error(msg string) {
	panic("lexer error: " + msg)
}

// current returns the current byte or -1 if at end.
func (l *lexer) current() int {
	if l.pos >= len(l.source) {
		return -1
	}
	return int(l.source[l.pos])
}

// advance moves to the next byte.
func (l *lexer) advance() {
	if l.pos >= len(l.source) {
		return
	}
	c := l.source[l.pos]
	l.pos++
	if c == '\n' || c == '\r' {
		// Handle \r\n, \n\r
		if l.pos < len(l.source) {
			next := l.source[l.pos]
			if (c == '\r' && next == '\n') || (c == '\n' && next == '\r') {
				l.pos++
			}
		}
		l.line++
		l.column = 1
	} else {
		l.column++
	}
}

// saveColumn saves the current position for token return.
func (l *lexer) saveColumn() (line, column int) {
	return l.line, l.column
}

// isAlphaNumeric reports whether byte is letter, underscore, or digit.
func isAlphaNumeric(c int) bool {
	return c == '_' || unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c))
}

// isAlpha reports whether byte is a letter or underscore.
func isAlpha(c int) bool {
	return c == '_' || unicode.IsLetter(rune(c))
}

// isDigit reports whether byte is a digit.
func isDigit(c int) bool {
	return c >= '0' && c <= '9'
}

// isHexDigit reports whether byte is a hex digit.
func isHexDigit(c int) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// skipWhitespace skips spaces, tabs, form feeds, and newlines.
func (l *lexer) skipWhitespace() {
	for {
		c := l.current()
		switch c {
		case ' ', '\t', '\f', '\v', '\n', '\r':
			l.advance()
		default:
			return
		}
	}
}

// inclinenumber handles newline incrementing.
func (l *lexer) inclinenumber() {
	c := l.current()
	if c == '\n' || c == '\r' {
		l.advance()
		// Handle \r\n or \n\r
		nc := l.current()
		if (c == '\r' && nc == '\n') || (c == '\n' && nc == '\r') {
			l.advance()
		}
		l.line++
		l.column = 1
	}
}

// checkNext1 checks if current char equals c and advances if so.
func (l *lexer) checkNext1(c int) bool {
	if l.current() == c {
		l.advance()
		return true
	}
	return false
}

// readIdentifier reads an identifier or keyword and returns start column.
func (l *lexer) readIdentifier() (string, int) {
	startPos := l.pos
	startColumn := l.column
	l.advance() // consume first character
	for {
		c := l.current()
		if isAlphaNumeric(c) {
			l.advance()
		} else {
			break
		}
	}
	return string(l.source[startPos:l.pos]), startColumn
}

// keywords map for fast lookup.
var keywords = map[string]api.TokenType{
	"and":      api.TOKEN_AND,
	"break":    api.TOKEN_BREAK,
	"do":       api.TOKEN_DO,
	"else":     api.TOKEN_ELSE,
	"elseif":   api.TOKEN_ELSEIF,
	"end":      api.TOKEN_END,
	"false":    api.TOKEN_FALSE,
	"for":      api.TOKEN_FOR,
	"function": api.TOKEN_FUNCTION,
	"global":   api.TOKEN_GLOBAL,
	"goto":     api.TOKEN_GOTO,
	"if":       api.TOKEN_IF,
	"in":       api.TOKEN_IN,
	"local":    api.TOKEN_LOCAL,
	"nil":      api.TOKEN_NIL,
	"not":      api.TOKEN_NOT,
	"or":       api.TOKEN_OR,
	"repeat":   api.TOKEN_REPEAT,
	"return":   api.TOKEN_RETURN,
	"then":     api.TOKEN_THEN,
	"true":     api.TOKEN_TRUE,
	"until":    api.TOKEN_UNTIL,
	"while":    api.TOKEN_WHILE,
}

// skipComment skips a comment.
func (l *lexer) skipComment() {
	if l.current() != '-' {
		return
	}
	l.advance() // skip second '-'

	if l.current() == '[' {
		// Long comment [[...]] or [=[...]=] etc.
		l.advance() // skip '['
		sep := l.skipSep()
		if sep >= 2 {
			// Valid long comment
			l.readLongString(nil, sep)
			return
		}
	}

	// Short comment - skip to end of line
	for {
		c := l.current()
		if c == -1 || c == '\n' || c == '\r' {
			return
		}
		l.advance()
	}
}

// skipSep returns the number of '=' signs between brackets plus 2.
func (l *lexer) skipSep() int {
	start := l.current()
	if start != '[' && start != ']' {
		return 0
	}
	l.advance()
	count := 0
	for l.current() == '=' {
		l.advance()
		count++
	}
	if l.current() == start {
		return count + 2
	}
	return 0 // unmatched
}

// readLongString reads a long string [[...]] or [=[...]=].
func (l *lexer) readLongString(seminfo *api.Token, sep int) {
	// Already consumed [[ or [= etc, just skip the second bracket
	l.advance()

	// Skip initial newline
	c := l.current()
	if c == '\n' || c == '\r' {
		l.inclinenumber()
	}

	var sb strings.Builder
	for {
		c := l.current()
		if c == -1 {
			l.Error("unfinished long string")
			return
		}

		if c == ']' {
			if l.skipSep() == sep {
				l.advance() // skip closing ]
				if seminfo != nil {
					seminfo.Value = sb.String()
				}
				return
			}
			sb.WriteByte(byte(c))
			l.advance()
			continue
		}

		if c == '\n' || c == '\r' {
			sb.WriteByte('\n')
			l.inclinenumber()
			continue
		}

		sb.WriteByte(byte(c))
		l.advance()
	}
}

// readNumber reads a numeric literal.
func (l *lexer) readNumber() (string, api.TokenType) {
	start := l.pos
	hasDecimal := false
	isHex := false

	c := l.current()
	if c == '0' {
		l.advance()
		nc := l.current()
		if nc == 'x' || nc == 'X' {
			isHex = true
			l.advance()
		} else if nc == '.' {
			hasDecimal = true
			l.advance() // consume the dot
		}
	}

	if isHex {
		// Hex number
		for {
			c = l.current()
			if isHexDigit(c) {
				l.advance()
			} else if c == '.' {
				hasDecimal = true
				l.advance()
				for isHexDigit(l.current()) {
					l.advance()
				}
				break
			} else if c == 'p' || c == 'P' {
				l.advance()
				if l.current() == '+' || l.current() == '-' {
					l.advance()
				}
				for isHexDigit(l.current()) {
					l.advance()
				}
				break
			} else {
				break
			}
		}
	} else {
		// Decimal number
		for {
			c = l.current()
			if isDigit(c) {
				l.advance()
			} else if c == '.' {
				if hasDecimal {
					break
				}
				hasDecimal = true
				l.advance()
			} else if c == 'e' || c == 'E' {
				l.advance()
				if l.current() == '+' || l.current() == '-' {
					l.advance()
				}
				for isDigit(l.current()) {
					l.advance()
				}
				break
			} else {
				break
			}
		}
	}

	// Check for trailing letter (invalid)
	if isAlpha(l.current()) {
		l.advance()
	}

	numStr := string(l.source[start:l.pos])

	// Try to parse to determine integer vs number
	if isHex {
		_, err := strconv.ParseInt(numStr, 0, 64)
		if err != nil {
			l.Error("malformed number")
		}
		if hasDecimal {
			return numStr, api.TOKEN_NUMBER
		}
		return numStr, api.TOKEN_INTEGER
	}

	// Try to parse as integer first
	if !hasDecimal {
		_, err := strconv.ParseInt(numStr, 10, 64)
		if err == nil {
			return numStr, api.TOKEN_INTEGER
		}
	}

	// Parse as float
	_, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		l.Error("malformed number")
	}
	return numStr, api.TOKEN_NUMBER
}

// readString reads a short string "..." or '...'.
func (l *lexer) readString() string {
	delimiter := l.current()
	l.advance() // consume opening delimiter

	var sb strings.Builder
	for {
		c := l.current()
		if c == -1 {
			l.Error("unfinished string")
			return ""
		}

		if c == '\n' || c == '\r' {
			l.Error("unfinished string")
			return ""
		}

		if c == delimiter {
			l.advance() // consume closing delimiter
			return sb.String()
		}

		if c == '\\' {
			l.advance()
			c = l.current()
			switch c {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '\\':
				sb.WriteByte('\\')
			case '"':
				sb.WriteByte('"')
			case '\'':
				sb.WriteByte('\'')
			case 'x':
				l.advance()
				h1 := l.readHexDigit()
				h2 := l.readHexDigit()
				sb.WriteByte(byte((h1 << 4) | h2))
			case 'z':
				l.advance()
				for isSpace(l.current()) {
					if l.current() == '\n' || l.current() == '\r' {
						l.inclinenumber()
					} else {
						l.advance()
					}
				}
				continue
			case 'u':
				l.advance()
				if l.current() != '{' {
					l.Error("invalid escape sequence")
				}
				l.advance()
				r := l.readUnicodeEscape()
				if l.current() != '}' {
					l.Error("invalid escape sequence")
				}
				l.advance()
				sb.WriteRune(rune(r))
				continue
			case -1:
				l.Error("unfinished string")
				return ""
			default:
				if c >= '0' && c <= '9' {
					l.pos--
					r := l.readDecimalEscape()
					sb.WriteByte(byte(r))
					continue
				}
				l.Error("invalid escape sequence")
			}
			l.advance()
			continue
		}

		sb.WriteByte(byte(c))
		l.advance()
	}
}

// readHexDigit reads a single hex digit.
func (l *lexer) readHexDigit() int {
	c := l.current()
	if isHexDigit(c) {
		l.advance()
		if c >= 'a' && c <= 'f' {
			return c - 'a' + 10
		}
		if c >= 'A' && c <= 'F' {
			return c - 'A' + 10
		}
		return c - '0'
	}
	l.Error("invalid escape sequence")
	return 0
}

// readUnicodeEscape reads \u{XXX}.
func (l *lexer) readUnicodeEscape() int {
	r := 0
	for isHexDigit(l.current()) {
		l.advance()
		r = r*16 + l.readHexDigit()
	}
	return r
}

// readDecimalEscape reads \ddd.
func (l *lexer) readDecimalEscape() int {
	r := 0
	for i := 0; i < 3; i++ {
		c := l.current()
		if c >= '0' && c <= '9' {
			r = r*10 + c - '0'
			l.advance()
		} else {
			break
		}
	}
	if r > 255 {
		l.Error("decimal escape too large")
	}
	return r
}

// isSpace reports whether byte is whitespace (not newline).
func isSpace(c int) bool {
	return c == ' ' || c == '\t' || c == '\f' || c == '\v'
}

// lexToken does the actual lexing and returns a token.
func (l *lexer) lexToken() api.Token {
	// Skip whitespace
	l.skipWhitespace()

	line, column := l.saveColumn()
	c := l.current()

	switch c {
	case -1:
		return api.Token{Type: api.TOKEN_EOS, Line: line, Column: column}
	case ' ', '\t', '\f', '\v', '\n', '\r':
		l.advance()
		return l.lexToken()
	case '-':
		l.advance()
		if l.current() == '-' {
			l.skipComment()
			return l.lexToken()
		}
		return api.Token{Type: api.TOKEN_MINUS, Line: line, Column: column}
	case '[':
		sep := l.skipSep()
		if sep >= 2 {
			line2, column2 := l.saveColumn()
			var sb strings.Builder
			// Read long string
			l.advance() // skip opening bracket
			// Skip initial newline
			if l.current() == '\n' || l.current() == '\r' {
				l.inclinenumber()
			}
			for {
				c := l.current()
				if c == -1 {
					l.Error("unfinished long string")
					return api.Token{}
				}
				if c == ']' {
					if l.skipSep() == sep {
						l.advance() // skip closing ]
						return api.Token{Type: api.TOKEN_STRING, Value: sb.String(), Line: line2, Column: column2}
					}
					sb.WriteByte(byte(c))
					l.advance()
					continue
				}
				if c == '\n' || c == '\r' {
					sb.WriteByte('\n')
					l.inclinenumber()
					continue
				}
				sb.WriteByte(byte(c))
				l.advance()
			}
		}
		if sep == 0 {
			// [=... missing second bracket
			// Check if this is just a regular '['
			if l.current() == '=' {
				// Invalid
				l.advance()
				return api.Token{Type: api.TOKEN_LBRACK, Line: line, Column: column}
			}
		}
		return api.Token{Type: api.TOKEN_LBRACK, Line: line, Column: column}
	case '=':
		l.advance()
		if l.checkNext1('=') {
			return api.Token{Type: api.TOKEN_EQ, Line: line, Column: column}
		}
		return api.Token{Type: api.TOKEN_ASSIGN, Line: line, Column: column}
	case '<':
		l.advance()
		if l.checkNext1('=') {
			return api.Token{Type: api.TOKEN_LE, Line: line, Column: column}
		}
		if l.checkNext1('<') {
			return api.Token{Type: api.TOKEN_SHL, Line: line, Column: column}
		}
		return api.Token{Type: api.TOKEN_LT, Line: line, Column: column}
	case '>':
		l.advance()
		if l.checkNext1('=') {
			return api.Token{Type: api.TOKEN_GE, Line: line, Column: column}
		}
		if l.checkNext1('>') {
			return api.Token{Type: api.TOKEN_SHR, Line: line, Column: column}
		}
		return api.Token{Type: api.TOKEN_GT, Line: line, Column: column}
	case '/':
		l.advance()
		if l.checkNext1('/') {
			return api.Token{Type: api.TOKEN_IDIV, Line: line, Column: column}
		}
		return api.Token{Type: api.TOKEN_DIV, Line: line, Column: column}
	case '~':
		l.advance()
		if l.checkNext1('=') {
			return api.Token{Type: api.TOKEN_NE, Line: line, Column: column}
		}
		return api.Token{Type: api.TOKEN_TILDE, Line: line, Column: column}
	case ':':
		l.advance()
		if l.checkNext1(':') {
			return api.Token{Type: api.TOKEN_DBCOLON, Line: line, Column: column}
		}
		return api.Token{Type: api.TOKEN_COLON, Line: line, Column: column}
	case '"', '\'':
		value := l.readString()
		return api.Token{Type: api.TOKEN_STRING, Value: value, Line: line, Column: column}
	case '.':
		l.advance()
		if l.checkNext1('.') {
			if l.checkNext1('.') {
				return api.Token{Type: api.TOKEN_DOTS, Line: line, Column: column}
			}
			return api.Token{Type: api.TOKEN_CONCAT, Line: line, Column: column}
		}
		if isDigit(l.current()) {
			l.pos--
			numStr, numType := l.readNumber()
			return api.Token{Type: numType, Value: numStr, Line: line, Column: column}
		}
		return api.Token{Type: api.TOKEN_DOT, Line: line, Column: column}
	case '+':
		l.advance()
		return api.Token{Type: api.TOKEN_PLUS, Line: line, Column: column}
	case '*':
		l.advance()
		return api.Token{Type: api.TOKEN_MUL, Line: line, Column: column}
	case '%':
		l.advance()
		return api.Token{Type: api.TOKEN_MOD, Line: line, Column: column}
	case '^':
		l.advance()
		return api.Token{Type: api.TOKEN_POW, Line: line, Column: column}
	case '#':
		l.advance()
		return api.Token{Type: api.TOKEN_HASH, Line: line, Column: column}
	case '&':
		l.advance()
		return api.Token{Type: api.TOKEN_AMP, Line: line, Column: column}
	case '|':
		l.advance()
		return api.Token{Type: api.TOKEN_PIPE, Line: line, Column: column}
	case '(':
		l.advance()
		return api.Token{Type: api.TOKEN_LPAREN, Line: line, Column: column}
	case ')':
		l.advance()
		return api.Token{Type: api.TOKEN_RPAREN, Line: line, Column: column}
	case '{':
		l.advance()
		return api.Token{Type: api.TOKEN_LBRACE, Line: line, Column: column}
	case '}':
		l.advance()
		return api.Token{Type: api.TOKEN_RBRACE, Line: line, Column: column}
	case ']':
		l.advance()
		return api.Token{Type: api.TOKEN_RBRACK, Line: line, Column: column}
	case ';':
		l.advance()
		return api.Token{Type: api.TOKEN_SEMICOLON, Line: line, Column: column}
	case ',':
		l.advance()
		return api.Token{Type: api.TOKEN_COMMA, Line: line, Column: column}
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		numStr, numType := l.readNumber()
		return api.Token{Type: numType, Value: numStr, Line: line, Column: column}
	default:
		if isAlpha(c) {
			ident, identColumn := l.readIdentifier()
			if kw, ok := keywords[ident]; ok {
				return api.Token{Type: kw, Line: line, Column: identColumn}
			}
			return api.Token{Type: api.TOKEN_NAME, Value: ident, Line: line, Column: identColumn}
		}
		// Unknown character - consume it
		l.advance()
		return api.Token{Type: api.TokenType(c), Line: line, Column: column}
	}
}

// NextToken implements api.Lexer.
func (l *lexer) NextToken() api.Token {
	if l.lookaheadValid {
		l.lookaheadValid = false
		return l.lookaheadToken
	}
	return l.lexToken()
}

// Lookahead implements api.Lexer.
func (l *lexer) Lookahead() api.Token {
	if !l.lookaheadValid {
		l.lookaheadToken = l.lexToken()
		l.lookaheadValid = true
	}
	return l.lookaheadToken
}
