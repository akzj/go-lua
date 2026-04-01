package llex

/*
** $Id: llex.go $
** Lexical Analyzer
** Ported from llex.h and llex.c
*/

import (
	"fmt"
	"unicode"

	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
	"github.com/akzj/go-lua/internal/lstring"
	"github.com/akzj/go-lua/internal/lzio"
)

/* lparser package import removed - FuncState will be used directly */

/*
** First reserved word code (after byte characters)
*/
const FirstReserved = 257

/*
** Reserved words tokens
 */
const (
	TK_AND    = FirstReserved + iota
	TK_BREAK
	TK_DO
	TK_ELSE
	TK_ELSEIF
	TK_END
	TK_FALSE
	TK_FOR
	TK_FUNCTION
	TK_GLOBAL
	TK_GOTO
	TK_IF
	TK_IN
	TK_LOCAL
	TK_NIL
	TK_NOT
	TK_OR
	TK_REPEAT
	TK_RETURN
	TK_THEN
	TK_TRUE
	TK_UNTIL
	TK_WHILE
	/* other terminal symbols */
	TK_IDIV
	TK_CONCAT
	TK_DOTS
	TK_EQ
	TK_GE
	TK_LE
	TK_NE
	TK_SHL
	TK_SHR
	TK_DBCOLON
	TK_EOS
	TK_FLT
	TK_INT
	TK_NAME
	TK_STRING
)

/* number of reserved words */
const NUM_RESERVED = TK_WHILE - FirstReserved + 1

/*
** SemInfo - semantic information for tokens
 */
type SemInfo struct {
	R  float64            // for TK_FLT
	I  int64              // for TK_INT
	Ts *lobject.TString // for TK_NAME, TK_STRING
}

/*
** Token structure
 */
type Token struct {
	Token   int
	SemInfo SemInfo
}

/*
** LexState - lexical analyzer state
** Fs is interface{} to hold *lparser.FuncState without import cycle
 */
type LexState struct {
	Current    int
	LineNum    int
	LastLine   int
	T          Token
	LookAhead  Token
	Fs         interface{} // *lparser.FuncState - set by lparser via SetFs
	L          *lstate.LuaState
	Z          *lzio.ZIO
	Buff       *lzio.Mbuffer
	H          *lobject.Table
	Dyd        interface{} // *lparser.Dyndata
	Source     *lobject.TString
	EnvN       *lobject.TString
	BrkN       *lobject.TString
	GlbN       *lobject.TString
	CurrIsLast bool
	NC         int
}

/*
** Token strings
 */
var luaX_tokens = []string{
	"and", "break", "do", "else", "elseif",
	"end", "false", "for", "function", "global", "goto", "if",
	"in", "local", "nil", "not", "or", "repeat",
	"return", "then", "true", "until", "while",
	"//", "..", "...", "==", ">=", "<=", "~=",
	"<<", ">>", "::", "<eof>",
	"<number>", "<integer>", "<name>", "<string>",
}

/*
** Current environment name
 */
const LUA_ENV = "_ENV"

/*
** Initialize lexer
 */
func Init(L *lstate.LuaState) {
	_ = lstring.NewString(L, LUA_ENV)
}

/*
** Set input for lexer
 */
func SetInput(L *lstate.LuaState, ls *LexState, z *lzio.ZIO, source *lobject.TString, firstChar int) {
	ls.T.Token = 0
	ls.L = L
	ls.Current = firstChar
	ls.LookAhead.Token = TK_EOS
	ls.Z = z
	ls.Fs = nil
	ls.LineNum = 1
	ls.LastLine = 1
	ls.Source = source
	ls.EnvN = lstring.NewString(L, LUA_ENV)
	ls.BrkN = lstring.NewString(L, "break")
	ls.GlbN = nil
	lzio.ResizeBuffer(ls.L, ls.Buff, lzio.MinBuffer)
}

/*
** Save character to buffer
 */
func Save(ls *LexState, c int) {
	buff := ls.Buff
	if lzio.BuffLen(buff)+1 > lzio.SizeBuffer(buff) {
		newsize := lzio.SizeBuffer(buff)
		if newsize >= MAX_SIZE/3*2 {
			SyntaxError(ls, "lexical element too long")
			return
		}
		newsize += newsize >> 1
		lzio.ResizeBuffer(ls.L, buff, newsize)
	}
	buff.Buffer[lzio.BuffLen(buff)] = byte(c)
	buff.N++
}

/*
** Save and advance
 */
func saveAndNext(ls *LexState) {
	Save(ls, ls.Current)
	next(ls)
}

/*
** Get next character
 */
func next(ls *LexState) {
	ls.Current = lzio.Zgetc(ls.Z)
}

/*
** Check if current char is newline
 */
func currIsNewline(ls *LexState) bool {
	return ls.Current == '\n' || ls.Current == '\r'
}

/*
** Increment line number and skip newline sequence
 */
func inclinenumber(ls *LexState) {
	intskip := ls.Current
	lua_assert(currIsNewline(ls))
	next(ls)
	if currIsNewline(ls) && ls.Current != intskip {
		next(ls)
	}
	ls.LineNum++
}

/*
** Create a new string
 */
func NewString(ls *LexState, str string, l int) *lobject.TString {
	return lstring.NewString(ls.L, str)
}

/*
** Token to string
 */
func Token2Str(ls *LexState, token int) string {
	if token < FirstReserved {
		if unicode.IsPrint(rune(token)) {
			return fmt.Sprintf("'%c'", token)
		}
		return fmt.Sprintf("'<\\%d>'", token)
	}
	s := luaX_tokens[token-FirstReserved]
	if token < TK_EOS {
		return fmt.Sprintf("'%s'", s)
	}
	return s
}

/*
** Syntax error
 */
func SyntaxError(ls *LexState, msg string) {
	panic(fmt.Sprintf("syntax error: %s", msg))
}

/*
** Semantic error
 */
func SemError(ls *LexState, msg string) {
	SyntaxError(ls, msg)
}

/*
** Next token
 */
func Next(ls *LexState) {
	ls.LastLine = ls.LineNum
	if ls.LookAhead.Token != TK_EOS {
		ls.T = ls.LookAhead
		ls.LookAhead.Token = TK_EOS
	} else {
		ls.T.Token = llex(ls, &ls.T.SemInfo)
	}
}

/*
** Look ahead token
 */
func LookAhead(ls *LexState) int {
	lua_assert(ls.LookAhead.Token == TK_EOS)
	ls.LookAhead.Token = llex(ls, &ls.LookAhead.SemInfo)
	return ls.LookAhead.Token
}

/*
** Main lexer function
 */
func llex(ls *LexState, seminfo *SemInfo) int {
	lzio.ResetBuffer(ls.Buff)
	for {
		switch ls.Current {
		case '\n', '\r':
			inclinenumber(ls)
		case ' ', '\f', '\t', '\v':
			next(ls)
		case '-':
			next(ls)
			if ls.Current != '-' {
				return '-'
			}
			next(ls)
			if ls.Current == '[' {
				sep := skipSep(ls)
				lzio.ResetBuffer(ls.Buff)
				if sep >= 2 {
					readLongString(ls, nil, sep)
					lzio.ResetBuffer(ls.Buff)
					break
				}
			}
			for !currIsNewline(ls) && ls.Current != lzio.EOZ {
				next(ls)
			}
		case '[':
			sep := skipSep(ls)
			if sep >= 2 {
				readLongString(ls, seminfo, sep)
				return TK_STRING
			} else if sep == 0 {
				SyntaxError(ls, "invalid long string delimiter")
			}
			return '['
		case '=':
			next(ls)
			if checkNext1(ls, '=') {
				return TK_EQ
			}
			return '='
		case '<':
			next(ls)
			if checkNext1(ls, '=') {
				return TK_LE
			} else if checkNext1(ls, '<') {
				return TK_SHL
			}
			return '<'
		case '>':
			next(ls)
			if checkNext1(ls, '=') {
				return TK_GE
			} else if checkNext1(ls, '>') {
				return TK_SHR
			}
			return '>'
		case '/':
			next(ls)
			if checkNext1(ls, '/') {
				return TK_IDIV
			}
			return '/'
		case '~':
			next(ls)
			if checkNext1(ls, '=') {
				return TK_NE
			}
			return '~'
		case ':':
			next(ls)
			if checkNext1(ls, ':') {
				return TK_DBCOLON
			}
			return ':'
		case '"', '\'':
			readString(ls, ls.Current, seminfo)
			return TK_STRING
		case '.':
			saveAndNext(ls)
			if checkNext1(ls, '.') {
				if checkNext1(ls, '.') {
					return TK_DOTS
				}
				return TK_CONCAT
			} else if isdigit(ls.Current) {
				return readNumeral(ls, seminfo)
			}
			return '.'
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return readNumeral(ls, seminfo)
		case lzio.EOZ:
			return TK_EOS
		default:
			if isalpha(ls.Current) {
				return readName(ls, seminfo)
			}
			c := ls.Current
			next(ls)
			return c
		}
	}
}

func checkNext1(ls *LexState, c int) bool {
	if ls.Current == c {
		next(ls)
		return true
	}
	return false
}

func checkNext2(ls *LexState, set string) bool {
	if ls.Current == int(set[0]) || ls.Current == int(set[1]) {
		saveAndNext(ls)
		return true
	}
	return false
}

func skipSep(ls *LexState) int64 {
	count := int64(0)
	s := ls.Current
	lua_assert(s == '[' || s == ']')
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

func readLongString(ls *LexState, seminfo *SemInfo, sep int64) {
	saveAndNext(ls)
	if currIsNewline(ls) {
		inclinenumber(ls)
	}
	for {
		switch ls.Current {
		case lzio.EOZ:
			msg := "unfinished long string"
			SyntaxError(ls, msg)
		case ']':
			if skipSep(ls) == sep {
				saveAndNext(ls)
				goto endloop
			}
		case '\n', '\r':
			Save(ls, '\n')
			inclinenumber(ls)
			if seminfo == nil {
				lzio.ResetBuffer(ls.Buff)
			}
		default:
			if seminfo != nil {
				saveAndNext(ls)
			} else {
				next(ls)
			}
		}
	}
endloop:
	if seminfo != nil {
		start := int(sep)
		length := int(lzio.BuffLen(ls.Buff)) - 2*int(sep)
		seminfo.Ts = NewString(ls, string(ls.Buff.Buffer[start:start+length]), length)
	}
}

func readNumeral(ls *LexState, seminfo *SemInfo) int {
	expo := "Ee"
	first := ls.Current
	lua_assert(isdigit(first))
	saveAndNext(ls)
	if first == '0' && checkNext2(ls, "xX") {
		expo = "Pp"
	}
	for {
		if checkNext2(ls, expo) {
			checkNext2(ls, "-+")
		} else if isxdigit(ls.Current) || ls.Current == '.' {
			saveAndNext(ls)
		} else {
			break
		}
	}
	if isalpha(ls.Current) {
		saveAndNext(ls)
	}
	Save(ls, 0)

	buff := string(ls.Buff.Buffer[:lzio.BuffLen(ls.Buff)-1])
	var val float64
	n, _ := fmt.Sscanf(buff, "%g", &val)
	if n == 0 {
		SyntaxError(ls, "malformed number")
	}
	if val == float64(int64(val)) {
		seminfo.I = int64(val)
		return TK_INT
	}
	seminfo.R = val
	return TK_FLT
}

func readString(ls *LexState, del int, seminfo *SemInfo) {
	saveAndNext(ls)
	for ls.Current != del {
		switch ls.Current {
		case lzio.EOZ, '\n', '\r':
			SyntaxError(ls, "unfinished string")
		case '\\':
			next(ls)
			c := readEscape(ls)
			Save(ls, c)
		default:
			saveAndNext(ls)
		}
	}
	saveAndNext(ls)
	length := int(lzio.BuffLen(ls.Buff)) - 2
	seminfo.Ts = NewString(ls, string(ls.Buff.Buffer[1:1+length]), length)
}

func readEscape(ls *LexState) int {
	switch ls.Current {
	case 'a':
		next(ls)
		return '\a'
	case 'b':
		next(ls)
		return '\b'
	case 'f':
		next(ls)
		return '\f'
	case 'n':
		next(ls)
		return '\n'
	case 'r':
		next(ls)
		return '\r'
	case 't':
		next(ls)
		return '\t'
	case 'v':
		next(ls)
		return '\v'
	case '\\':
		next(ls)
		return '\\'
	case '"':
		next(ls)
		return '"'
	case '\'':
		next(ls)
		return '\''
	case '\n', '\r':
		inclinenumber(ls)
		return '\n'
	case 'z':
		next(ls)
		for isspace(ls.Current) {
			if currIsNewline(ls) {
				inclinenumber(ls)
			} else {
				next(ls)
			}
		}
		return 0
	case 'x':
		return readHexaEscape(ls)
	case 'u':
		return readUTF8Escape(ls)
	default:
		if !isdigit(ls.Current) {
			SyntaxError(ls, "invalid escape sequence")
		}
		return readDecEscape(ls)
	}
}

func readHexaEscape(ls *LexState) int {
	r := getHexa(ls)
	r = r<<4 + getHexa(ls)
	lzio.BuffRemove(ls.Buff, 2)
	return r
}

func getHexa(ls *LexState) int {
	next(ls)
	if !isxdigit(ls.Current) {
		SyntaxError(ls, "hexadecimal digit expected")
	}
	return hexavalue(ls.Current)
}

func hexavalue(c int) int {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

func readUTF8Escape(ls *LexState) int {
	r := uint32(0)
	Save(ls, 'u')
	next(ls)
	if ls.Current != '{' {
		SyntaxError(ls, "missing '{'")
	}
	next(ls)
	for isxdigit(ls.Current) {
		r = r<<4 + uint32(hexavalue(ls.Current))
		next(ls)
	}
	if ls.Current != '}' {
		SyntaxError(ls, "missing '}'")
	}
	next(ls)
	buff := make([]byte, 4)
	n := encodeUTF8(r, buff)
	for i := 0; i < n; i++ {
		Save(ls, int(buff[i]))
	}
	return 0
}

func encodeUTF8(r uint32, buff []byte) int {
	if r <= 0x7F {
		buff[0] = byte(r)
		return 1
	} else if r <= 0x7FF {
		buff[0] = byte(0xC0 | (r >> 6))
		buff[1] = byte(0x80 | (r & 0x3F))
		return 2
	} else if r <= 0xFFFF {
		buff[0] = byte(0xE0 | (r >> 12))
		buff[1] = byte(0x80 | ((r >> 6) & 0x3F))
		buff[2] = byte(0x80 | (r & 0x3F))
		return 3
	}
	buff[0] = byte(0xF0 | (r >> 18))
	buff[1] = byte(0x80 | ((r >> 12) & 0x3F))
	buff[2] = byte(0x80 | ((r >> 6) & 0x3F))
	buff[3] = byte(0x80 | (r & 0x3F))
	return 4
}

func readDecEscape(ls *LexState) int {
	r := 0
	for i := 0; i < 3 && isdigit(ls.Current); i++ {
		r = 10*r + ls.Current - '0'
		next(ls)
	}
	if r > 0xFF {
		SyntaxError(ls, "decimal escape too large")
	}
	return r
}

func readName(ls *LexState, seminfo *SemInfo) int {
	for isalnum(ls.Current) || ls.Current == '_' {
		saveAndNext(ls)
	}
	Save(ls, 0)
	ts := NewString(ls, string(ls.Buff.Buffer[:lzio.BuffLen(ls.Buff)-1]), int(lzio.BuffLen(ls.Buff)-1))
	// Check for reserved keywords
	for i := 0; i < NUM_RESERVED; i++ {
		if ts.Shrlen == int8(len(luaX_tokens[i])) {
			match := true
			for j := 0; j < int(ts.Shrlen); j++ {
				c1 := int(luaX_tokens[i][j])
				// Compare case-insensitively
				c2 := int(ls.Buff.Buffer[j])
				if c1 >= 'A' && c1 <= 'Z' {
					c1 = c1 + 32
				}
				if c2 >= 'A' && c2 <= 'Z' {
					c2 = c2 + 32
				}
				if c1 != c2 {
					match = false
					break
				}
			}
			if match {
				return FirstReserved + i
			}
		}
	}
	seminfo.Ts = ts
	return TK_NAME
}

func isalpha(c int) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isdigit(c int) bool {
	return c >= '0' && c <= '9'
}

func isalnum(c int) bool {
	return isalpha(c) || isdigit(c)
}

func isxdigit(c int) bool {
	return isdigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isspace(c int) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func lua_assert(v bool) {
	if !v {
		panic("assertion failed")
	}
}

const MAX_SIZE = 1<<63 - 1

/*
** FuncStateSetter - interface that parser implements to allow llex to call parser methods
 */
type FuncStateSetter interface {
	GetFuncState() interface{}
}

/*
** SetFs - set FuncState on lexer (called by parser)
 */
func SetFs(ls *LexState, fs interface{}) {
	ls.Fs = fs
}

/*
** GetFs - get FuncState from lexer (for parser use)
 */
func GetFs(ls *LexState) interface{} {
	return ls.Fs
}

/*
** SetDyd - set Dyndata on lexer (called by parser)
 */
func SetDyd(ls *LexState, dyd interface{}) {
	ls.Dyd = dyd
}

/*
** GetDyd - get Dyndata from lexer (for parser use)
 */
func GetDyd(ls *LexState) interface{} {
	return ls.Dyd
}
