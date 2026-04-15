// Full Lua 5.5.1 lexer implementation.
//
// Converts Lua source text into a stream of tokens. Handles all escape
// sequences, long strings, comments, number parsing, and reserved word detection.
//
// Reference: lua-master/llex.c
package api

import (
	"fmt"
	"strings"
	"unicode/utf8"

	objectapi "github.com/akzj/go-lua/internal/object/api"
)

// chunkid formats a source name for error messages.
// Mirrors luaO_chunkid in lobject.c. LUA_IDSIZE = 60.
func chunkid(source string) string {
	const idsize = 60
	if len(source) == 0 {
		return `[string ""]`
	}
	if source[0] == '=' {
		rest := source[1:]
		if len(rest)+1 <= idsize {
			return rest
		}
		return rest[:idsize-1]
	}
	if source[0] == '@' {
		rest := source[1:]
		if len(rest)+1 <= idsize {
			return rest
		}
		return "..." + rest[len(rest)-(idsize-1-3):]
	}
	// String source: format as [string "source"] or [string "source..."]
	// PRE=[string " (9), POST="] (2), RETS=... (3), +1 for NUL
	const maxContent = idsize - 9 - 3 - 2 - 1 // = 45
	nl := strings.IndexByte(source, '\n')
	srclen := len(source)
	if srclen <= maxContent && nl < 0 {
		return fmt.Sprintf(`[string "%s"]`, source)
	}
	if nl >= 0 && nl < srclen {
		srclen = nl
	}
	if srclen > maxContent {
		srclen = maxContent
	}
	return fmt.Sprintf(`[string "%s..."]`, source[:srclen])
}

// EOZ signals end of input (-1, matching C's EOZ).
const EOZ = -1

// ---------------------------------------------------------------------------
// Token name table — ORDER RESERVED (matches C's luaX_tokens)
// ---------------------------------------------------------------------------

var tokenNames = []string{
	// Reserved words (FirstReserved + 0..22)
	"and", "break", "do", "else", "elseif",
	"end", "false", "for", "function", "global", "goto", "if",
	"in", "local", "nil", "not", "or", "repeat",
	"return", "then", "true", "until", "while",
	// Multi-char operators and value tokens
	"//", "..", "...", "==", ">=", "<=", "~=",
	"<<", ">>", "::", "<eof>",
	"<number>", "<integer>", "<name>", "<string>",
}

// Token2Str returns a human-readable name for a token type.
func Token2Str(token TokenType) string {
	if token < FirstReserved {
		if token >= 32 && token < 127 {
			return fmt.Sprintf("'%c'", rune(token))
		}
		return fmt.Sprintf("'<\\%d>'", token)
	}
	idx := int(token - FirstReserved)
	if idx >= 0 && idx < len(tokenNames) {
		s := tokenNames[idx]
		if token < TK_EOS { // fixed format (symbols and reserved words)
			return fmt.Sprintf("'%s'", s)
		}
		return s // names, strings, numerals
	}
	return fmt.Sprintf("<token %d>", token)
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewLexState creates a new lexer state from a reader and source name.
func NewLexState(reader LexReader, source string) *LexState {
	ls := &LexState{
		Reader:    reader,
		Source:    source,
		Line:      1,
		LastLine:  1,
		EnvName:   "_ENV",
		BreakName: "break",
		Buf:       make([]byte, 0, 64),
	}
	ls.Lookahead.Type = TK_EOS
	return ls
}

// SetInput reads the first character and prepares the lexer for scanning.
func SetInput(ls *LexState) {
	ls.Current = ls.Reader.ReadByte()
}

// SkipShebang skips a Unix shebang line (#! or # comment) at the start of
// source. Must be called after SetInput and before Next.
// Mirrors skipcomment in lauxlib.c.
func SkipShebang(ls *LexState) {
	if ls.Current == '#' {
		for ls.Current != '\n' && ls.Current != EOZ {
			next(ls)
		}
		if ls.Current == '\n' {
			next(ls) // skip the newline itself
			ls.Line++
		}
	}
}

// ---------------------------------------------------------------------------
// Character-level helpers
// ---------------------------------------------------------------------------

func next(ls *LexState) {
	ls.Current = ls.Reader.ReadByte()
}

func save(ls *LexState, c int) {
	ls.Buf = append(ls.Buf, byte(c))
}

func saveAndNext(ls *LexState) {
	save(ls, ls.Current)
	next(ls)
}

func currIsNewline(ls *LexState) bool {
	return ls.Current == '\n' || ls.Current == '\r'
}

func isDigit(c int) bool {
	return c >= '0' && c <= '9'
}

func isHexDigit(c int) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isAlpha(c int) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isAlNum(c int) bool {
	return isAlpha(c) || isDigit(c)
}

func isSpace(c int) bool {
	return c == ' ' || c == '\f' || c == '\t' || c == '\v' || c == '\n' || c == '\r'
}

// incLineNumber handles newlines (both \r\n and \n\r pairs).
// Mirrors: inclinenumber in llex.c
func incLineNumber(ls *LexState) {
	old := ls.Current
	next(ls) // skip '\n' or '\r'
	if currIsNewline(ls) && ls.Current != old {
		next(ls) // skip '\n\r' or '\r\n'
	}
	ls.Line++
	if ls.Line >= 0x7FFFFFFF { // INT_MAX
		LexError(ls, "chunk has too many lines", 0)
	}
}

// checkNext1: if current==c, advance and return true.
func checkNext1(ls *LexState, c int) bool {
	if ls.Current == c {
		next(ls)
		return true
	}
	return false
}

// checkNext2: if current is c1 or c2, save and advance, return true.
func checkNext2(ls *LexState, c1, c2 int) bool {
	if ls.Current == c1 || ls.Current == c2 {
		saveAndNext(ls)
		return true
	}
	return false
}

// resetBuf clears the token buffer.
func resetBuf(ls *LexState) {
	ls.Buf = ls.Buf[:0]
}

// bufString returns the current buffer contents as a string.
func bufString(ls *LexState) string {
	return string(ls.Buf)
}

// ---------------------------------------------------------------------------
// Error reporting
// ---------------------------------------------------------------------------

// LexError raises a SyntaxError via panic.
func LexError(ls *LexState, msg string, token TokenType) {
	tokStr := ""
	if token != 0 {
		switch token {
		case TK_NAME, TK_STRING, TK_FLT, TK_INT:
			tokStr = fmt.Sprintf("'%s'", bufString(ls))
		default:
			tokStr = Token2Str(token)
		}
	}

	fullMsg := fmt.Sprintf("%s:%d: %s", chunkid(ls.Source), ls.Line, msg)
	if tokStr != "" {
		fullMsg = fmt.Sprintf("%s near %s", fullMsg, tokStr)
	}

	panic(&SyntaxError{
		Source:  ls.Source,
		Line:    ls.Line,
		Token:   tokStr,
		Message: fullMsg,
	})
}

// SyntaxErr is a convenience wrapper — error at current token.
func SyntaxErr(ls *LexState, msg string) {
	LexError(ls, msg, ls.Token.Type)
}

// ---------------------------------------------------------------------------
// Number scanning
// Mirrors: read_numeral in llex.c
// ---------------------------------------------------------------------------

func readNumeral(ls *LexState) Token {
	expo1, expo2 := 'E', 'e'
	first := ls.Current
	saveAndNext(ls)
	if first == '0' && checkNext2(ls, 'x', 'X') {
		expo1, expo2 = 'P', 'p'
	}
	for {
		if checkNext2(ls, int(expo1), int(expo2)) { // exponent mark
			checkNext2(ls, '-', '+') // optional sign
		} else if isHexDigit(ls.Current) || ls.Current == '.' {
			saveAndNext(ls)
		} else {
			break
		}
	}
	// If numeral touches a letter, force an error
	if isAlpha(ls.Current) {
		saveAndNext(ls)
	}

	s := bufString(ls)

	// Try integer first, then float (matches C's luaO_str2num)
	if iv, ok := objectapi.StringToInteger(s); ok {
		return Token{Type: TK_INT, IntVal: iv}
	}
	if fv, ok := objectapi.StringToFloat(s); ok {
		return Token{Type: TK_FLT, FltVal: fv}
	}
	LexError(ls, "malformed number", TK_FLT)
	return Token{} // unreachable
}

// ---------------------------------------------------------------------------
// Long bracket scanning
// Mirrors: skip_sep, read_long_string in llex.c
// ---------------------------------------------------------------------------

// skipSep reads a sequence '[=*[' or ']=*]', leaving the last bracket.
// Returns count+2 if well-formed, 1 if single bracket, 0 if unfinished.
func skipSep(ls *LexState) int {
	count := 0
	s := ls.Current
	saveAndNext(ls)
	for ls.Current == '=' {
		saveAndNext(ls)
		count++
	}
	if ls.Current == s {
		return count + 2
	}
	if count == 0 {
		return 1
	}
	return 0
}

// readLongString reads a long string or long comment.
// If isString is true, captures content; if false (comment), discards.
func readLongString(ls *LexState, isString bool, sep int) Token {
	line := ls.Line // save start line for error messages
	saveAndNext(ls) // skip 2nd '['
	if currIsNewline(ls) {
		incLineNumber(ls) // skip leading newline
	}
	for {
		switch ls.Current {
		case EOZ:
			what := "string"
			if !isString {
				what = "comment"
			}
			LexError(ls, fmt.Sprintf("unfinished long %s (starting at line %d)", what, line), TK_EOS)
		case ']':
			if skipSep(ls) == sep {
				saveAndNext(ls) // skip 2nd ']'
				if isString {
					// Extract content between opening and closing brackets
					content := string(ls.Buf[sep : len(ls.Buf)-sep])
					return Token{Type: TK_STRING, StrVal: content}
				}
				return Token{}
			}
		case '\n', '\r':
			save(ls, '\n')
			incLineNumber(ls)
			if !isString {
				resetBuf(ls) // avoid wasting space for comments
			}
		default:
			if isString {
				saveAndNext(ls)
			} else {
				next(ls)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// String scanning
// Mirrors: read_string in llex.c
// ---------------------------------------------------------------------------

func readString(ls *LexState, delimiter int) Token {
	saveAndNext(ls) // save opening delimiter
	for ls.Current != delimiter {
		switch ls.Current {
		case EOZ:
			LexError(ls, "unfinished string", TK_EOS)
		case '\n', '\r':
			LexError(ls, "unfinished string", TK_STRING)
		case '\\': // escape sequences
			saveAndNext(ls) // save '\' for error messages
			var c int
			handled := false
			switch ls.Current {
			case 'a':
				c = '\a'
			case 'b':
				c = '\b'
			case 'f':
				c = '\f'
			case 'n':
				c = '\n'
			case 'r':
				c = '\r'
			case 't':
				c = '\t'
			case 'v':
				c = '\v'
			case '\\':
				c = '\\'
			case '"':
				c = '"'
			case '\'':
				c = '\''
			case '\n', '\r':
				// Escaped newline — save as \n
				incLineNumber(ls)
				// Remove the saved '\' from buffer
				ls.Buf = ls.Buf[:len(ls.Buf)-1]
				save(ls, '\n')
				handled = true
			case 'x':
				// \xHH — hex byte
				r := readHexaEsc(ls)
				// Remove the saved '\' from buffer
				ls.Buf = ls.Buf[:len(ls.Buf)-1]
				save(ls, r)
				handled = true
			case 'u':
				// \u{XXXX} — UTF-8 escape
				readUTF8Esc(ls)
				handled = true
			case 'z':
				// \z — skip whitespace
				ls.Buf = ls.Buf[:len(ls.Buf)-1] // remove '\'
				next(ls)                          // skip 'z'
				for isSpace(ls.Current) {
					if currIsNewline(ls) {
						incLineNumber(ls)
					} else {
						next(ls)
					}
				}
				handled = true
			case EOZ:
				handled = true // will error on next loop iteration
			default:
				if !isDigit(ls.Current) {
					escError(ls, "invalid escape sequence")
				}
				// \ddd — decimal byte
				r := readDecEsc(ls)
				// Remove the saved '\' from buffer
				ls.Buf = ls.Buf[:len(ls.Buf)-1]
				save(ls, r)
				handled = true
			}
			if !handled {
				// read_save pattern: next(), remove '\', save c
				next(ls)
				ls.Buf = ls.Buf[:len(ls.Buf)-1] // remove '\'
				save(ls, c)
			}
		default:
			saveAndNext(ls)
		}
	}
	saveAndNext(ls) // skip closing delimiter

	// Extract content (strip delimiters)
	content := string(ls.Buf[1 : len(ls.Buf)-1])
	return Token{Type: TK_STRING, StrVal: content}
}

// getHexa mirrors C Lua's gethexa: save_and_next, then check hex digit.
func getHexa(ls *LexState) int {
	saveAndNext(ls)
	if !isHexDigit(ls.Current) {
		escError(ls, "hexadecimal digit expected")
	}
	return hexaValue(ls.Current)
}

// hexaValue returns the numeric value of a hex digit character.
func hexaValue(c int) int {
	if c >= '0' && c <= '9' {
		return c - '0'
	}
	if c >= 'a' && c <= 'f' {
		return c - 'a' + 10
	}
	return c - 'A' + 10
}

// readHexaEsc reads \xHH escape. Returns the byte value.
// Mirrors C Lua's readhexaesc: uses getHexa to save chars for error messages.
func readHexaEsc(ls *LexState) int {
	r := getHexa(ls)
	r = (r << 4) | getHexa(ls)
	// Remove the 2 saved chars ('x' and first hex digit) from buffer.
	ls.Buf = ls.Buf[:len(ls.Buf)-2]
	next(ls) // advance past second hex digit
	return r
}


// readDecEsc reads \ddd escape (up to 3 decimal digits). Returns the byte value.
func readDecEsc(ls *LexState) int {
	r := 0
	for i := 0; i < 3 && isDigit(ls.Current); i++ {
		r = 10*r + (ls.Current - '0')
		saveAndNext(ls)
	}
	if r > 255 {
		escError(ls, "decimal escape too large")
	}
	// Remove the saved digits from buffer
	// We saved up to 3 digits; need to remove them
	// Count how many we saved
	digitsSaved := 0
	for i := 0; i < 3 && i < len(ls.Buf); i++ {
		b := ls.Buf[len(ls.Buf)-1-i]
		if b >= '0' && b <= '9' {
			digitsSaved++
		} else {
			break
		}
	}
	ls.Buf = ls.Buf[:len(ls.Buf)-digitsSaved]
	return r
}

// readUTF8Esc reads \u{XXXX} escape and saves UTF-8 bytes.
// Mirrors C Lua's utf8esc + readutf8esc: saves everything to buffer
// for error reporting, counts saved chars, removes them on success.
func readUTF8Esc(ls *LexState) {
	// Phase 1: read the codepoint value, saving chars for error messages
	// i counts chars to remove on success; starts at 4 for "\u{X"
	i := 4
	saveAndNext(ls) // save 'u', advance to '{'
	if ls.Current != '{' {
		escError(ls, "missing '{'")
	}
	r := uint32(getHexa(ls)) // save '{', advance, check first hex digit
	for {
		saveAndNext(ls) // save current digit (or non-digit), advance
		if !isHexDigit(ls.Current) {
			break
		}
		i++
		if r > (0x7FFFFFFF >> 4) {
			escError(ls, "UTF-8 value too large")
		}
		r = (r << 4) + uint32(hexaValue(ls.Current))
	}
	if ls.Current != '}' {
		escError(ls, "missing '}'")
	}
	next(ls) // skip '}'
	// Remove i saved chars from buffer (includes the '\' saved by caller)
	ls.Buf = ls.Buf[:len(ls.Buf)-i]

	// Phase 2: encode UTF-8 and save to buffer
	const utf8BufSz = 8
	var buf [utf8BufSz]byte
	n := 1
	if r < 0x80 {
		buf[utf8BufSz-1] = byte(r)
	} else {
		mfb := uint32(0x3F)
		for {
			buf[utf8BufSz-n] = byte(0x80 | (r & 0x3F))
			n++
			r >>= 6
			mfb >>= 1
			if r <= mfb {
				break
			}
		}
		buf[utf8BufSz-n] = byte((^mfb << 1) | r)
	}
	for j := utf8BufSz - n; j < utf8BufSz; j++ {
		save(ls, int(buf[j]))
	}
}


func escError(ls *LexState, msg string) {
	// Save current char for error message if not EOF
	if ls.Current != EOZ {
		saveAndNext(ls)
	}
	LexError(ls, msg, TK_STRING)
}

// ---------------------------------------------------------------------------
// UTF-8 encoding helper (for \u{} escapes)
// ---------------------------------------------------------------------------

// utf8Encode encodes a Unicode code point to UTF-8 bytes.
func utf8Encode(r rune) []byte {
	buf := make([]byte, utf8.UTFMax)
	n := utf8.EncodeRune(buf, r)
	return buf[:n]
}

// ---------------------------------------------------------------------------
// Main lexer — llex
// Mirrors: llex() in llex.c
// ---------------------------------------------------------------------------

func llex(ls *LexState) Token {
	resetBuf(ls)
	for {
		switch ls.Current {
		case '\n', '\r': // line breaks
			incLineNumber(ls)

		case ' ', '\f', '\t', '\v': // whitespace
			next(ls)

		case '-': // '-' or '--' (comment)
			next(ls)
			if ls.Current != '-' {
				return Token{Type: TokenType('-')}
			}
			// Comment
			next(ls)
			if ls.Current == '[' { // long comment?
				sep := skipSep(ls)
				resetBuf(ls) // skipSep may dirty the buffer
				if sep >= 2 {
					readLongString(ls, false, sep)
					resetBuf(ls)
					continue // restart main loop
				}
			}
			// Short comment — skip to end of line
			for !currIsNewline(ls) && ls.Current != EOZ {
				next(ls)
			}

		case '[': // long string or simply '['
			sep := skipSep(ls)
			if sep >= 2 {
				tok := readLongString(ls, true, sep)
				return tok
			} else if sep == 0 {
				LexError(ls, "invalid long string delimiter", TK_STRING)
			}
			return Token{Type: TokenType('[')}

		case '=':
			next(ls)
			if checkNext1(ls, '=') {
				return Token{Type: TK_EQ}
			}
			return Token{Type: TokenType('=')}

		case '<':
			next(ls)
			if checkNext1(ls, '=') {
				return Token{Type: TK_LE}
			}
			if checkNext1(ls, '<') {
				return Token{Type: TK_SHL}
			}
			return Token{Type: TokenType('<')}

		case '>':
			next(ls)
			if checkNext1(ls, '=') {
				return Token{Type: TK_GE}
			}
			if checkNext1(ls, '>') {
				return Token{Type: TK_SHR}
			}
			return Token{Type: TokenType('>')}

		case '/':
			next(ls)
			if checkNext1(ls, '/') {
				return Token{Type: TK_IDIV}
			}
			return Token{Type: TokenType('/')}

		case '~':
			next(ls)
			if checkNext1(ls, '=') {
				return Token{Type: TK_NE}
			}
			return Token{Type: TokenType('~')}

		case ':':
			next(ls)
			if checkNext1(ls, ':') {
				return Token{Type: TK_DBCOLON}
			}
			return Token{Type: TokenType(':')}

		case '"', '\'': // short literal strings
			return readString(ls, ls.Current)

		case '.': // '.', '..', '...', or number
			saveAndNext(ls)
			if checkNext1(ls, '.') {
				if checkNext1(ls, '.') {
					return Token{Type: TK_DOTS}
				}
				return Token{Type: TK_CONCAT}
			}
			if !isDigit(ls.Current) {
				return Token{Type: TokenType('.')}
			}
			// Number starting with '.'
			return readNumeral2(ls)

		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return readNumeral(ls)

		case EOZ:
			return Token{Type: TK_EOS}

		default:
			if isAlpha(ls.Current) { // identifier or reserved word
				for {
					saveAndNext(ls)
					if !isAlNum(ls.Current) {
						break
					}
				}
				s := bufString(ls)
				if tkType, ok := ReservedWords[s]; ok {
					return Token{Type: tkType}
				}
				return Token{Type: TK_NAME, StrVal: s}
			}
			// Single-char token
			c := ls.Current
			next(ls)
			return Token{Type: TokenType(c)}
		}
	}
}

// readNumeral2 is for numbers that start with '.' (already saved).
// The '.' is already in the buffer.
func readNumeral2(ls *LexState) Token {
	// The '.' was already saved. Now continue reading the number.
	for isDigit(ls.Current) || ls.Current == '.' {
		saveAndNext(ls)
	}
	// Check for exponent
	if ls.Current == 'e' || ls.Current == 'E' {
		saveAndNext(ls)
		if ls.Current == '+' || ls.Current == '-' {
			saveAndNext(ls)
		}
	}
	for isDigit(ls.Current) || isHexDigit(ls.Current) {
		saveAndNext(ls)
	}
	if isAlpha(ls.Current) {
		saveAndNext(ls) // force error
	}

	s := bufString(ls)
	if iv, ok := objectapi.StringToInteger(s); ok {
		return Token{Type: TK_INT, IntVal: iv}
	}
	if fv, ok := objectapi.StringToFloat(s); ok {
		return Token{Type: TK_FLT, FltVal: fv}
	}
	LexError(ls, "malformed number", TK_FLT)
	return Token{} // unreachable
}

// ---------------------------------------------------------------------------
// Public API — Next and Lookahead
// Mirrors: luaX_next and luaX_lookahead in llex.c
// ---------------------------------------------------------------------------

// Next advances to the next token.
func Next(ls *LexState) {
	ls.LastLine = ls.Line
	if ls.HasAhead {
		// Use lookahead token
		ls.Token = ls.Lookahead
		ls.HasAhead = false
		ls.Lookahead.Type = TK_EOS
	} else {
		ls.Token = llex(ls)
	}
}

// Lookahead peeks at the next token without consuming it.
func Lookahead(ls *LexState) TokenType {
	ls.Lookahead = llex(ls)
	ls.HasAhead = true
	return ls.Lookahead.Type
}
