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

	// Step counter to prevent infinite loops (max 10M chars)
	stepCount int
	maxSteps  int
}

// NewLexer creates a new lexer for the given source.
func NewLexer(source, sourceName string) api.Lexer {
	return &lexer{
		source:     []byte(source),
		sourceName: sourceName,
		line:       1,
		column:     1,
		maxSteps:   10_000_000, // 10M chars max to prevent infinite loops
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
	// Guard against infinite loops
	l.stepCount++
	if l.stepCount > l.maxSteps {
		l.Error("lexer: maximum steps exceeded (possible infinite loop)")
		return
	}

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
		// Guard against infinite loops
		if l.stepCount > l.maxSteps {
			l.Error("lexer: maximum steps exceeded (possible infinite loop)")
			return
		}
		c := l.current()
		switch c {
		case ' ', '\t', '\f', '\v', '\n', '\r':
			l.advance()
		default:
			return
		}
	}
}

// inclinenumber increments line counter and resets column.
// Called when positioned AT a newline character (the newline has NOT been
// skipped yet by advance). Handles paired CRLF/LFLR sequences by skipping
// the second character, then increments line.
// Does NOT call advance() - the caller must advance past the newline.
func (l *lexer) inclinenumber() {
	// Check for paired CRLF or LFLR sequences
	// When this is called, we are AT the newline char (not past it)
	if l.pos < len(l.source) {
		c := l.source[l.pos]
		if l.pos+1 < len(l.source) {
			nc := l.source[l.pos+1]
			// Skip second char of CRLF (\r\n) or LFLR (\n\r)
			if (c == '\r' && nc == '\n') || (c == '\n' && nc == '\r') {
				l.pos += 2 // skip both chars of the pair
				l.line++
				l.column = 1
				return
			}
		}
	}
	// Single newline - advance past it
	l.advance()
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
		// Guard against infinite loops
		if l.stepCount > l.maxSteps {
			l.Error("lexer: maximum steps exceeded")
			return string(l.source[startPos:l.pos]), startColumn
		}
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
		// skipSep expects to start at '[', don't advance here
		sep, _ := l.skipSep()
		if sep >= 2 {
			// Valid long comment - discard the content
			l.readLongString(sep)
			return
		}
	}

	// Short comment - skip to end of line
	for {
		// Guard against infinite loops
		if l.stepCount > l.maxSteps {
			return
		}
		c := l.current()
		if c == -1 || c == '\n' || c == '\r' {
			return
		}
		l.advance()
	}
}

// skipSep returns the separator level (count of '=' + 2) and whether a matching
// closing bracket was found. When no match is found, position is restored to
// the original bracket so it can be reprocessed.
func (l *lexer) skipSep() (int, bool) {
	start := l.current()
	if start != '[' && start != ']' {
		return 0, false
	}
	// Save position of opening bracket
	savedPos := l.pos
	l.advance() // move past opening bracket
	
	count := 0
	// Count '=' signs WITHOUT advancing past the closing bracket
	for l.current() == '=' {
		l.advance()
		count++
	}
	
	
	// Check if closing bracket matches
	if l.current() == start {
		// Found matching closing bracket - advance past it
		l.advance()
		return count + 2, true
	}
	
	// No match - restore to opening bracket
	l.pos = savedPos
	if start == '[' && count == 0 {
		// [[ without ]] - return 1 for single [
		return 1, false
	}
	return 0, false
}

// skipSepForClose is used inside readLongString to find the closing delimiter.
// It looks for ']' followed by '=' signs and another ']'.
// Returns the sep level if matched (and advances past it).
// Returns 0 if not matched (position restored).
// skipSepForClose checks if current position starts a closing delimiter for the given sep.
// It looks for ']' followed by '=' signs and another ']'.
// Returns true if the closing delimiter matches the expected sep.
func (l *lexer) skipSepForClose(sep int) bool {
	if l.current() != ']' {
		return false
	}
	savedPos := l.pos
	l.advance() // skip past the ']'

	count := 0
	for l.current() == '=' {
		l.advance()
		count++
	}

	// Match only if '=' count matches exactly AND closing ']' follows.
	if l.current() == ']' && count+2 == sep {
		return true
	}

	// No match — restore position to before the ']'
	l.pos = savedPos
	return false
}

// Called after skipSep(), position is at the second bracket already.
// Returns the string content, stripping opening and closing delimiters.
func (l *lexer) readLongString(sep int) string {
	var sb strings.Builder

	// Determine delimiter string based on sep level
	// sep = 2 for [[, sep = 3 for [=[, etc.
	// Skip initial newline (Lua skips it but doesn't add '\n' to content)
	c := l.current()
	if c == '\n' || c == '\r' {
		l.inclinenumber()
	}

	for {
		c = l.current()
		if c == -1 {
			l.Error("unfinished long string")
			return ""
		}

		if c == ']' {
			// Check for closing delimiter
			if l.skipSepForClose(sep) {
				// Closing delimiter found! skipSepForClose already consumed it.
				return sb.String()
			}
			// Not a closing delimiter, treat ']' as content.
			sb.WriteByte(byte(c))
			l.advance()
			continue
		}

		if c == '\n' || c == '\r' {
			sb.WriteByte('\n')
			l.inclinenumber()
		} else {
			sb.WriteByte(byte(c))
			l.advance()
		}
	}
}
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
				// Handle case where . comes right after 0x (e.g., 0x.1 or 0x.F)
				for isHexDigit(l.current()) {
					l.advance()
				}
				// Check for exponent after decimal part
				if c := l.current(); c == 'p' || c == 'P' {
					l.advance()
					if l.current() == '+' || l.current() == '-' {
						l.advance()
					}
					for isHexDigit(l.current()) {
						l.advance()
					}
				}
				break
			} else if c == 'p' || c == 'P' {
				hasDecimal = true // p implies fractional part
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
		if hasDecimal {
			// Hex floats - Go's ParseFloat only supports hex floats with p/P exponent
			// For hex floats without exponent (like 0xF0.0 or 0x.FFFF), we validate format manually
			// Valid format: 0x[0-9a-fA-F]*\.?[0-9a-fA-F]+
			if !isValidHexFloat(numStr) {
				l.Error("malformed number")
			}
			return numStr, api.TOKEN_NUMBER
		}
		// Pure hex integer (no decimal, no exponent)
		// Lua allows arbitrarily large hex integers, so validate FORMAT only
		// Valid format: 0x[0-9a-fA-F]+
		if !isValidHexInt(numStr) {
			l.Error("malformed number")
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
		// Guard against infinite loops
		if l.stepCount > l.maxSteps {
			l.Error("lexer: maximum steps exceeded")
			return ""
		}
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
			case 'a':
				sb.WriteByte('\a')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'v':
				sb.WriteByte('\v')
			case '\\':
				sb.WriteByte('\\')
			case '"':
				sb.WriteByte('"')
			case '\'':
				sb.WriteByte('\'')
			case 'x':
				// Lua \xNN escape: exactly two hex digits
				l.advance()
				c1 := l.current()
				if !isHexDigit(c1) {
					l.Error("invalid escape sequence")
					return sb.String()
				}
				l.advance()
				c2 := l.current()
				if !isHexDigit(c2) {
					l.Error("invalid escape sequence")
					return sb.String()
				}
				l.advance()
				h1 := hexToInt(c1)
				h2 := hexToInt(c2)
				sb.WriteByte(byte((h1 << 4) | h2))
				continue
			case 'z':
				// \z skips whitespace including newlines (Lua 5.3+)
				l.advance()
				for {
					c := l.current()
					if c == '\n' || c == '\r' {
						l.inclinenumber()
					} else if isSpace(c) {
						l.advance()
					} else {
						break
					}
				}
				continue
			case 'u':
				l.advance()
				if l.current() != '{' {
					l.Error("invalid escape sequence")
				}
				l.advance()
				r, ok := l.readUnicodeEscape()
				if !ok {
					l.Error("invalid escape sequence")
				}
				if l.current() != '}' {
					l.Error("invalid escape sequence")
				}
				l.advance()
				sb.WriteRune(rune(r))
				continue
			case -1:
				l.Error("unfinished string")
				return ""
			case '\n', '\r': // Line continuation (backslash at end of line)
				l.inclinenumber()
				continue
			default:
				if c >= '0' && c <= '9' {
					// c is already the digit after the backslash (we advanced past \)
					// Don't backup - just call readDecimalEscape which will read from current pos
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
		return hexToInt(c)
	}
	l.Error("invalid escape sequence")
	return 0
}

// hexToInt converts a hex character to its integer value.
func hexToInt(c int) int {
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

// readUnicodeEscape reads \u{XXX}.
// Returns the Unicode code point and whether it was valid.
func (l *lexer) readUnicodeEscape() (int, bool) {
	r := 0
	for {
		c := l.current()
		if !isHexDigit(c) {
			break
		}
		d, ok := l.readHexDigitOK()
		if !ok {
			return 0, false
		}
		r = r*16 + d
	}
	return r, true
}

// readHexDigitOK reads a single hex digit and advances, returning (value, ok).
func (l *lexer) readHexDigitOK() (int, bool) {
	c := l.current()
	if isHexDigit(c) {
		l.advance()
		return hexToInt(c), true
	}
	return 0, false
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

// isValidHexFloat validates a hex float literal.
// Lua hex floats: 0x[0-9a-fA-F]*\.?[0-9a-fA-F]+([pP][+-]?[0-9a-fA-F]+)?
// Must have at least one hex digit somewhere (before or after the dot).
func isValidHexFloat(s string) bool {
	if len(s) < 3 || (s[0] != '0' && (s[1] != 'x' && s[1] != 'X')) {
		return false
	}
	hasDigit := false
	i := 2 // skip "0x" or "0X"
	// Read hex digits before decimal
	for i < len(s) && isHexDigit(int(s[i])) {
		hasDigit = true
		i++
	}
	// Check for decimal
	if i < len(s) && s[i] == '.' {
		i++
		// Read hex digits after decimal
		if !hasDigit {
			// Must have at least one digit before OR after decimal
			for i < len(s) && isHexDigit(int(s[i])) {
				hasDigit = true
				i++
			}
		} else {
			// Read digits after decimal (may be zero)
			for i < len(s) && isHexDigit(int(s[i])) {
				i++
			}
		}
	}
	// Check for exponent (p/P)
	if i < len(s) && (s[i] == 'p' || s[i] == 'P') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		for i < len(s) && isHexDigit(int(s[i])) {
			i++
		}
	}
	return hasDigit && i == len(s)
}

// isValidHexInt validates a hex integer literal format.
// Lua hex integers: 0x[0-9a-fA-F]+
// Must have at least one hex digit after 0x.
func isValidHexInt(s string) bool {
	if len(s) < 3 || s[0] != '0' || (s[1] != 'x' && s[1] != 'X') {
		return false
	}
	// Must have at least one hex digit
	if !isHexDigit(int(s[2])) {
		return false
	}
	// All remaining chars must be hex digits
	for i := 3; i < len(s); i++ {
		if !isHexDigit(int(s[i])) {
			return false
		}
	}
	return true
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
		// Save column BEFORE skipSep advances the position
		line2, column2 := l.saveColumn()
		sep, _ := l.skipSep()
		if sep >= 2 {
			// Valid long string [[...]] or [=[...]=] etc.
			value := l.readLongString(sep)
			return api.Token{Type: api.TOKEN_STRING, Value: value, Line: line2, Column: column2}
		}
		if sep == 0 {
			// [=... without matching ]=...
			l.Error("invalid long string delimiter")
		}
		// sep == 1: [[ without matching ]]
		// Return single TOKEN_LBRACK
		l.advance() // Consume the '[' character
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
