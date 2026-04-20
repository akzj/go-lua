package api

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// StringReader — simple LexReader from a string
// ---------------------------------------------------------------------------

type StringReader struct {
	data string
	pos  int
}

func NewStringReader(s string) *StringReader {
	return &StringReader{data: s}
}

func (r *StringReader) ReadByte() int {
	if r.pos >= len(r.data) {
		return EOZ
	}
	b := int(r.data[r.pos])
	r.pos++
	return b
}

// helper: scan all tokens from a string
func scanAll(t *testing.T, input string) []Token {
	t.Helper()
	ls := NewLexState(NewStringReader(input), "test")
	SetInput(ls)
	var tokens []Token
	for {
		Next(ls)
		tokens = append(tokens, ls.Token)
		if ls.Token.Type == TK_EOS {
			break
		}
	}
	return tokens
}

// helper: scan first non-EOS token
func scanOne(t *testing.T, input string) Token {
	t.Helper()
	ls := NewLexState(NewStringReader(input), "test")
	SetInput(ls)
	Next(ls)
	return ls.Token
}

// helper: expect panic with SyntaxError
func expectError(t *testing.T, input string, substr string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic for input %q, got none", input)
		}
		se, ok := r.(*SyntaxError)
		if !ok {
			t.Fatalf("expected *SyntaxError, got %T: %v", r, r)
		}
		if substr != "" && !strings.Contains(se.Message, substr) {
			t.Errorf("error message %q does not contain %q", se.Message, substr)
		}
	}()
	scanAll(t, input)
}

// ---------------------------------------------------------------------------
// Reserved words
// ---------------------------------------------------------------------------

func TestReservedWords(t *testing.T) {
	words := []struct {
		word string
		tk   TokenType
	}{
		{"and", TK_AND}, {"break", TK_BREAK}, {"do", TK_DO},
		{"else", TK_ELSE}, {"elseif", TK_ELSEIF}, {"end", TK_END},
		{"false", TK_FALSE}, {"for", TK_FOR}, {"function", TK_FUNCTION},
		{"goto", TK_GOTO}, {"if", TK_IF},
		{"in", TK_IN}, {"local", TK_LOCAL}, {"nil", TK_NIL},
		{"not", TK_NOT}, {"or", TK_OR}, {"repeat", TK_REPEAT},
		{"return", TK_RETURN}, {"then", TK_THEN}, {"true", TK_TRUE},
		{"until", TK_UNTIL}, {"while", TK_WHILE},
	}
	for _, w := range words {
		tok := scanOne(t, w.word)
		if tok.Type != w.tk {
			t.Errorf("%q: got type %d, want %d", w.word, tok.Type, w.tk)
		}
	}
}

func TestReservedWordCount(t *testing.T) {
	if NumReservedCount != 22 {
		t.Errorf("NumReservedCount = %d, want 22", NumReservedCount)
	}
}

// TestGlobalIsSoftKeyword verifies that "global" scans as TK_NAME (identifier),
// not as a reserved word. The parser handles "global" as a context-sensitive
// keyword at statement start, but it must remain usable as a variable name.
func TestGlobalIsSoftKeyword(t *testing.T) {
	tok := scanOne(t, "global")
	if tok.Type != TK_NAME {
		t.Errorf("\"global\" should scan as TK_NAME (%d), got %d", TK_NAME, tok.Type)
	}
	if tok.StrVal != "global" {
		t.Errorf("\"global\" StrVal = %q, want \"global\"", tok.StrVal)
	}
}

// ---------------------------------------------------------------------------
// Operators
// ---------------------------------------------------------------------------

func TestSingleCharOperators(t *testing.T) {
	ops := "+-*%^#&|(){}[];,@"
	for _, c := range ops {
		tok := scanOne(t, string(c))
		if tok.Type != TokenType(c) {
			t.Errorf("%c: got type %d, want %d", c, tok.Type, TokenType(c))
		}
	}
}

func TestMultiCharOperators(t *testing.T) {
	cases := []struct {
		input string
		tk    TokenType
	}{
		{"//", TK_IDIV},
		{"..", TK_CONCAT},
		{"...", TK_DOTS},
		{"==", TK_EQ},
		{">=", TK_GE},
		{"<=", TK_LE},
		{"~=", TK_NE},
		{"<<", TK_SHL},
		{">>", TK_SHR},
		{"::", TK_DBCOLON},
	}
	for _, c := range cases {
		tok := scanOne(t, c.input)
		if tok.Type != c.tk {
			t.Errorf("%q: got type %d, want %d (%s)", c.input, tok.Type, c.tk, Token2Str(c.tk))
		}
	}
}

func TestOperatorDisambiguation(t *testing.T) {
	// '=' alone
	tok := scanOne(t, "=")
	if tok.Type != TokenType('=') {
		t.Errorf("'=': got %d, want %d", tok.Type, TokenType('='))
	}
	// '<' alone
	tok = scanOne(t, "< ")
	if tok.Type != TokenType('<') {
		t.Errorf("'<': got %d, want %d", tok.Type, TokenType('<'))
	}
	// '>' alone
	tok = scanOne(t, "> ")
	if tok.Type != TokenType('>') {
		t.Errorf("'>': got %d, want %d", tok.Type, TokenType('>'))
	}
	// '~' alone
	tok = scanOne(t, "~ ")
	if tok.Type != TokenType('~') {
		t.Errorf("'~': got %d, want %d", tok.Type, TokenType('~'))
	}
	// '/' alone
	tok = scanOne(t, "/ ")
	if tok.Type != TokenType('/') {
		t.Errorf("'/': got %d, want %d", tok.Type, TokenType('/'))
	}
	// ':' alone
	tok = scanOne(t, ": ")
	if tok.Type != TokenType(':') {
		t.Errorf("':': got %d, want %d", tok.Type, TokenType(':'))
	}
	// '.' alone
	tok = scanOne(t, ". ")
	if tok.Type != TokenType('.') {
		t.Errorf("'.': got %d, want %d", tok.Type, TokenType('.'))
	}
}

// ---------------------------------------------------------------------------
// Numbers
// ---------------------------------------------------------------------------

func TestIntegers(t *testing.T) {
	cases := []struct {
		input string
		val   int64
	}{
		{"42", 42},
		{"0", 0},
		{"123456789", 123456789},
	}
	for _, c := range cases {
		tok := scanOne(t, c.input)
		if tok.Type != TK_INT {
			t.Errorf("%q: got type %d, want TK_INT", c.input, tok.Type)
			continue
		}
		if tok.IntVal != c.val {
			t.Errorf("%q: got %d, want %d", c.input, tok.IntVal, c.val)
		}
	}
}

func TestHexIntegers(t *testing.T) {
	cases := []struct {
		input string
		val   int64
	}{
		{"0xFF", 255},
		{"0XA0", 160},
		{"0x0", 0},
	}
	for _, c := range cases {
		tok := scanOne(t, c.input)
		if tok.Type != TK_INT {
			t.Errorf("%q: got type %d, want TK_INT", c.input, tok.Type)
			continue
		}
		if tok.IntVal != c.val {
			t.Errorf("%q: got %d, want %d", c.input, tok.IntVal, c.val)
		}
	}
}

func TestFloats(t *testing.T) {
	cases := []struct {
		input string
		val   float64
	}{
		{"3.14", 3.14},
		{"0.5", 0.5},
		{".5", 0.5},
		{"5.", 5.0},
		{"1e10", 1e10},
		{"1E10", 1e10},
		{"1e+10", 1e10},
		{"1e-2", 0.01},
		{"3.14e2", 314.0},
	}
	for _, c := range cases {
		tok := scanOne(t, c.input)
		if tok.Type != TK_FLT {
			t.Errorf("%q: got type %d, want TK_FLT", c.input, tok.Type)
			continue
		}
		if tok.FltVal != c.val {
			t.Errorf("%q: got %g, want %g", c.input, tok.FltVal, c.val)
		}
	}
}

func TestHexFloats(t *testing.T) {
	cases := []struct {
		input string
		val   float64
	}{
		{"0x1p4", 16.0},
		{"0x1.8p1", 3.0},
	}
	for _, c := range cases {
		tok := scanOne(t, c.input)
		if tok.Type != TK_FLT {
			t.Errorf("%q: got type %d, want TK_FLT", c.input, tok.Type)
			continue
		}
		if tok.FltVal != c.val {
			t.Errorf("%q: got %g, want %g", c.input, tok.FltVal, c.val)
		}
	}
}

// ---------------------------------------------------------------------------
// Strings
// ---------------------------------------------------------------------------

func TestSimpleStrings(t *testing.T) {
	cases := []struct {
		input string
		val   string
	}{
		{`"hello"`, "hello"},
		{`'world'`, "world"},
		{`""`, ""},
		{`''`, ""},
		{`"abc def"`, "abc def"},
	}
	for _, c := range cases {
		tok := scanOne(t, c.input)
		if tok.Type != TK_STRING {
			t.Errorf("%q: got type %d, want TK_STRING", c.input, tok.Type)
			continue
		}
		if tok.StrVal != c.val {
			t.Errorf("%q: got %q, want %q", c.input, tok.StrVal, c.val)
		}
	}
}

func TestStringEscapes(t *testing.T) {
	cases := []struct {
		input string
		val   string
	}{
		{`"a\nb"`, "a\nb"},
		{`"a\tb"`, "a\tb"},
		{`"a\\b"`, "a\\b"},
		{`"a\"b"`, `a"b`},
		{`'a\'b'`, "a'b"},
		{`"a\rb"`, "a\rb"},
		{`"\a"`, "\a"},
		{`"\b"`, "\b"},
		{`"\f"`, "\f"},
		{`"\v"`, "\v"},
	}
	for _, c := range cases {
		tok := scanOne(t, c.input)
		if tok.Type != TK_STRING {
			t.Errorf("%q: got type %d, want TK_STRING", c.input, tok.Type)
			continue
		}
		if tok.StrVal != c.val {
			t.Errorf("%q: got %q, want %q", c.input, tok.StrVal, c.val)
		}
	}
}

func TestHexEscape(t *testing.T) {
	tok := scanOne(t, `"\x41"`) // 0x41 = 'A'
	if tok.Type != TK_STRING || tok.StrVal != "A" {
		t.Errorf(`"\x41": got type=%d val=%q, want TK_STRING "A"`, tok.Type, tok.StrVal)
	}

	tok = scanOne(t, `"\x00"`) // null byte
	if tok.Type != TK_STRING || tok.StrVal != "\x00" {
		t.Errorf(`"\x00": got type=%d val=%q, want TK_STRING "\x00"`, tok.Type, tok.StrVal)
	}
}

func TestDecimalEscape(t *testing.T) {
	tok := scanOne(t, `"\65"`) // 65 = 'A'
	if tok.Type != TK_STRING || tok.StrVal != "A" {
		t.Errorf(`"\65": got type=%d val=%q, want TK_STRING "A"`, tok.Type, tok.StrVal)
	}

	tok = scanOne(t, `"\097"`) // 97 = 'a'
	if tok.Type != TK_STRING || tok.StrVal != "a" {
		t.Errorf(`"\097": got type=%d val=%q, want TK_STRING "a"`, tok.Type, tok.StrVal)
	}
}

func TestUTF8Escape(t *testing.T) {
	tok := scanOne(t, `"\u{41}"`) // U+0041 = 'A'
	if tok.Type != TK_STRING || tok.StrVal != "A" {
		t.Errorf(`"\u{41}": got type=%d val=%q, want TK_STRING "A"`, tok.Type, tok.StrVal)
	}

	tok = scanOne(t, `"\u{4e16}"`) // U+4E16 = '世'
	if tok.Type != TK_STRING || tok.StrVal != "世" {
		t.Errorf(`"\u{4e16}": got type=%d val=%q, want TK_STRING "世"`, tok.Type, tok.StrVal)
	}

	tok = scanOne(t, `"\u{1F600}"`) // U+1F600 = 😀
	if tok.Type != TK_STRING || tok.StrVal != "😀" {
		t.Errorf(`"\u{1F600}": got type=%d val=%q, want TK_STRING "😀"`, tok.Type, tok.StrVal)
	}
}

func TestZEscape(t *testing.T) {
	// \z skips following whitespace
	tok := scanOne(t, "\"ab\\z    cd\"")
	if tok.Type != TK_STRING || tok.StrVal != "abcd" {
		t.Errorf(`\z escape: got type=%d val=%q, want "abcd"`, tok.Type, tok.StrVal)
	}

	// \z across newlines
	tok = scanOne(t, "\"ab\\z\n  \n  cd\"")
	if tok.Type != TK_STRING || tok.StrVal != "abcd" {
		t.Errorf(`\z across newlines: got type=%d val=%q, want "abcd"`, tok.Type, tok.StrVal)
	}
}

func TestEscapedNewline(t *testing.T) {
	// Escaped newline in string
	tok := scanOne(t, "\"ab\\\ncd\"")
	if tok.Type != TK_STRING || tok.StrVal != "ab\ncd" {
		t.Errorf(`escaped newline: got type=%d val=%q, want "ab\ncd"`, tok.Type, tok.StrVal)
	}
}

// ---------------------------------------------------------------------------
// Long strings
// ---------------------------------------------------------------------------

func TestLongStrings(t *testing.T) {
	cases := []struct {
		input string
		val   string
	}{
		{"[[hello]]", "hello"},
		{"[=[hello]=]", "hello"},
		{"[==[hello]==]", "hello"},
		{"[[multi\nline]]", "multi\nline"},
		{"[[\nhello]]", "hello"},       // leading newline stripped
		{"[[\rhello]]", "hello"},       // leading \r stripped
		{"[[\r\nhello]]", "hello"},     // leading \r\n stripped
		{"[[]]", ""},                   // empty
		{"[=[]=]", ""},                 // empty level 1
	}
	for _, c := range cases {
		tok := scanOne(t, c.input)
		if tok.Type != TK_STRING {
			t.Errorf("%q: got type %d, want TK_STRING", c.input, tok.Type)
			continue
		}
		if tok.StrVal != c.val {
			t.Errorf("%q: got %q, want %q", c.input, tok.StrVal, c.val)
		}
	}
}

func TestLongStringNoEscapes(t *testing.T) {
	// Long strings don't process escape sequences
	tok := scanOne(t, `[[hello\nworld]]`)
	if tok.StrVal != `hello\nworld` {
		t.Errorf(`long string escapes: got %q, want %q`, tok.StrVal, `hello\nworld`)
	}
}

func TestLongStringNested(t *testing.T) {
	// [=[ can contain ]] without closing
	tok := scanOne(t, "[=[a]]b]=]")
	if tok.StrVal != "a]]b" {
		t.Errorf("nested brackets: got %q, want %q", tok.StrVal, "a]]b")
	}
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

func TestShortComment(t *testing.T) {
	tokens := scanAll(t, "-- this is a comment\n42")
	// Should get: TK_INT(42), TK_EOS
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[0].Type != TK_INT || tokens[0].IntVal != 42 {
		t.Errorf("expected TK_INT(42), got %v", tokens[0])
	}
}

func TestLongComment(t *testing.T) {
	tokens := scanAll(t, "--[[ long comment ]]42")
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[0].Type != TK_INT || tokens[0].IntVal != 42 {
		t.Errorf("expected TK_INT(42), got %v", tokens[0])
	}
}

func TestLongCommentMultiline(t *testing.T) {
	tokens := scanAll(t, "--[[\nmulti\nline\ncomment\n]]42")
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[0].Type != TK_INT || tokens[0].IntVal != 42 {
		t.Errorf("expected TK_INT(42), got %v", tokens[0])
	}
}

func TestLongCommentLevel(t *testing.T) {
	// Level 3 comment: ]===] closes it, ]==] does NOT
	tokens := scanAll(t, "--[===[comment]==]still comment]===]42")
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[0].Type != TK_INT {
		t.Errorf("expected TK_INT, got %v", tokens[0])
	}
}

func TestCommentNotLong(t *testing.T) {
	// --[ is just a short comment (single bracket)
	tokens := scanAll(t, "--[ not long\n42")
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[0].Type != TK_INT || tokens[0].IntVal != 42 {
		t.Errorf("expected TK_INT(42), got %v", tokens[0])
	}
}

// ---------------------------------------------------------------------------
// Identifiers
// ---------------------------------------------------------------------------

func TestIdentifiers(t *testing.T) {
	cases := []struct {
		input string
		name  string
	}{
		{"foo", "foo"},
		{"_bar", "_bar"},
		{"__init__", "__init__"},
		{"abc123", "abc123"},
		{"x", "x"},
	}
	for _, c := range cases {
		tok := scanOne(t, c.input)
		if tok.Type != TK_NAME {
			t.Errorf("%q: got type %d, want TK_NAME", c.input, tok.Type)
			continue
		}
		if tok.StrVal != c.name {
			t.Errorf("%q: got %q, want %q", c.input, tok.StrVal, c.name)
		}
	}
}

func TestIdentifierNotReserved(t *testing.T) {
	// "andd" should be an identifier, not "and"
	tok := scanOne(t, "andd")
	if tok.Type != TK_NAME {
		t.Errorf("andd: got type %d, want TK_NAME", tok.Type)
	}
	if tok.StrVal != "andd" {
		t.Errorf("andd: got %q, want %q", tok.StrVal, "andd")
	}
}

// ---------------------------------------------------------------------------
// Line counting
// ---------------------------------------------------------------------------

func TestLineNumbers(t *testing.T) {
	ls := NewLexState(NewStringReader("a\nb\nc"), "test")
	SetInput(ls)

	Next(ls) // 'a' on line 1
	if ls.Line != 1 {
		t.Errorf("after 'a': line = %d, want 1", ls.Line)
	}

	Next(ls) // 'b' on line 2
	if ls.Line != 2 {
		t.Errorf("after 'b': line = %d, want 2", ls.Line)
	}

	Next(ls) // 'c' on line 3
	if ls.Line != 3 {
		t.Errorf("after 'c': line = %d, want 3", ls.Line)
	}
}

func TestLineNumbersCRLF(t *testing.T) {
	ls := NewLexState(NewStringReader("a\r\nb\r\nc"), "test")
	SetInput(ls)

	Next(ls) // 'a' on line 1
	if ls.Line != 1 {
		t.Errorf("after 'a': line = %d, want 1", ls.Line)
	}

	Next(ls) // 'b' on line 2
	if ls.Line != 2 {
		t.Errorf("after 'b': line = %d, want 2", ls.Line)
	}

	Next(ls) // 'c' on line 3
	if ls.Line != 3 {
		t.Errorf("after 'c': line = %d, want 3", ls.Line)
	}
}

func TestLineNumbersInLongString(t *testing.T) {
	ls := NewLexState(NewStringReader("[[\nline2\nline3\n]]42"), "test")
	SetInput(ls)

	Next(ls) // long string
	if ls.Token.Type != TK_STRING {
		t.Fatalf("expected TK_STRING, got %d", ls.Token.Type)
	}

	Next(ls) // 42
	if ls.Line != 4 {
		t.Errorf("after long string: line = %d, want 4", ls.Line)
	}
}

// ---------------------------------------------------------------------------
// Error cases
// ---------------------------------------------------------------------------

func TestUnterminatedString(t *testing.T) {
	expectError(t, `"hello`, "unfinished string")
}

func TestUnterminatedStringNewline(t *testing.T) {
	expectError(t, "\"hello\n\"", "unfinished string")
}

func TestUnterminatedLongString(t *testing.T) {
	expectError(t, "[[hello", "unfinished long string")
}

func TestInvalidEscape(t *testing.T) {
	expectError(t, `"\q"`, "invalid escape sequence")
}

func TestMalformedNumber(t *testing.T) {
	expectError(t, "0xGG", "malformed number")
}

func TestInvalidLongStringDelimiter(t *testing.T) {
	expectError(t, "[=hello", "invalid long string delimiter")
}

func TestDecimalEscapeTooLarge(t *testing.T) {
	expectError(t, `"\256"`, "decimal escape too large")
}

// ---------------------------------------------------------------------------
// Lookahead
// ---------------------------------------------------------------------------

func TestLookahead(t *testing.T) {
	ls := NewLexState(NewStringReader("a b c"), "test")
	SetInput(ls)

	Next(ls) // current = 'a'
	if ls.Token.Type != TK_NAME || ls.Token.StrVal != "a" {
		t.Fatalf("expected 'a', got %v", ls.Token)
	}

	// Peek at next
	ahead := Lookahead(ls)
	if ahead != TK_NAME {
		t.Errorf("lookahead type = %d, want TK_NAME", ahead)
	}
	if ls.Lookahead.StrVal != "b" {
		t.Errorf("lookahead val = %q, want %q", ls.Lookahead.StrVal, "b")
	}

	// Current should still be 'a'
	if ls.Token.StrVal != "a" {
		t.Errorf("current should still be 'a', got %q", ls.Token.StrVal)
	}

	// Next should consume the lookahead
	Next(ls) // should be 'b'
	if ls.Token.StrVal != "b" {
		t.Errorf("after Next: expected 'b', got %q", ls.Token.StrVal)
	}

	Next(ls) // should be 'c'
	if ls.Token.StrVal != "c" {
		t.Errorf("after Next: expected 'c', got %q", ls.Token.StrVal)
	}
}

// ---------------------------------------------------------------------------
// Full token stream — small Lua program
// ---------------------------------------------------------------------------

func TestFullTokenStream(t *testing.T) {
	input := `local x = 42
if x > 0 then
  print("hello")
end`

	expected := []TokenType{
		TK_LOCAL,            // local
		TK_NAME,             // x
		TokenType('='),      // =
		TK_INT,              // 42
		TK_IF,               // if
		TK_NAME,             // x
		TokenType('>'),      // >
		TK_INT,              // 0
		TK_THEN,             // then
		TK_NAME,             // print
		TokenType('('),      // (
		TK_STRING,           // "hello"
		TokenType(')'),      // )
		TK_END,              // end
		TK_EOS,              // <eof>
	}

	tokens := scanAll(t, input)
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, exp := range expected {
		if tokens[i].Type != exp {
			t.Errorf("token[%d]: got %d (%s), want %d (%s)",
				i, tokens[i].Type, Token2Str(tokens[i].Type),
				exp, Token2Str(exp))
		}
	}

	// Check specific values
	if tokens[1].StrVal != "x" {
		t.Errorf("token[1] name = %q, want 'x'", tokens[1].StrVal)
	}
	if tokens[3].IntVal != 42 {
		t.Errorf("token[3] int = %d, want 42", tokens[3].IntVal)
	}
	if tokens[11].StrVal != "hello" {
		t.Errorf("token[11] string = %q, want 'hello'", tokens[11].StrVal)
	}
}

func TestComplexProgram(t *testing.T) {
	input := `-- Fibonacci
local function fib(n)
  if n <= 1 then return n end
  return fib(n - 1) + fib(n - 2)
end
for i = 1, 10 do
  print(fib(i))
end`

	tokens := scanAll(t, input)
	// Just verify it scans without error and ends with EOS
	last := tokens[len(tokens)-1]
	if last.Type != TK_EOS {
		t.Errorf("last token should be TK_EOS, got %d", last.Type)
	}
	// Count tokens (excluding EOS)
	count := len(tokens) - 1
	if count < 30 {
		t.Errorf("expected at least 30 tokens, got %d", count)
	}
}

func TestAllOperatorsInExpression(t *testing.T) {
	input := "a + b - c * d / e // f % g ^ h .. i << j >> k & l | m ~ n"
	tokens := scanAll(t, input)
	// Should have: a + b - c * d / e // f % g ^ h .. i << j >> k & l | m ~ n EOS
	// = 15 names + 14 operators + EOS = 30
	if len(tokens) < 20 {
		t.Errorf("expected many tokens, got %d", len(tokens))
	}
}

// ---------------------------------------------------------------------------
// Token2Str
// ---------------------------------------------------------------------------

func TestToken2Str(t *testing.T) {
	cases := []struct {
		tk   TokenType
		want string
	}{
		{TokenType('+'), "'+'"},
		{TK_AND, "'and'"},
		{TK_EQ, "'=='"},
		{TK_EOS, "<eof>"},
		{TK_NAME, "<name>"},
		{TK_STRING, "<string>"},
		{TK_INT, "<integer>"},
		{TK_FLT, "<number>"},
	}
	for _, c := range cases {
		got := Token2Str(c.tk)
		if got != c.want {
			t.Errorf("Token2Str(%d) = %q, want %q", c.tk, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestEmptyInput(t *testing.T) {
	tokens := scanAll(t, "")
	if len(tokens) != 1 || tokens[0].Type != TK_EOS {
		t.Errorf("empty input should produce only TK_EOS")
	}
}

func TestOnlyWhitespace(t *testing.T) {
	tokens := scanAll(t, "   \t\n\n  ")
	if len(tokens) != 1 || tokens[0].Type != TK_EOS {
		t.Errorf("whitespace-only input should produce only TK_EOS")
	}
}

func TestOnlyComment(t *testing.T) {
	tokens := scanAll(t, "-- just a comment")
	if len(tokens) != 1 || tokens[0].Type != TK_EOS {
		t.Errorf("comment-only input should produce only TK_EOS")
	}
}

func TestMinusNotComment(t *testing.T) {
	tok := scanOne(t, "-42")
	if tok.Type != TokenType('-') {
		t.Errorf("'-42': first token should be '-', got %d", tok.Type)
	}
}

func TestDotNumber(t *testing.T) {
	tok := scanOne(t, ".5")
	if tok.Type != TK_FLT {
		t.Errorf("'.5': got type %d, want TK_FLT", tok.Type)
	}
	if tok.FltVal != 0.5 {
		t.Errorf("'.5': got %g, want 0.5", tok.FltVal)
	}
}

func TestLastLine(t *testing.T) {
	ls := NewLexState(NewStringReader("a\nb"), "test")
	SetInput(ls)

	Next(ls) // 'a' on line 1
	if ls.LastLine != 1 {
		t.Errorf("after first Next: LastLine = %d, want 1", ls.LastLine)
	}

	Next(ls) // 'b' on line 2
	if ls.LastLine != 1 {
		t.Errorf("after second Next: LastLine = %d, want 1 (line of 'a')", ls.LastLine)
	}

	Next(ls) // EOS
	if ls.LastLine != 2 {
		t.Errorf("after third Next: LastLine = %d, want 2 (line of 'b')", ls.LastLine)
	}
}
