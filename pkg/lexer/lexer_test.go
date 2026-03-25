// Package lexer_test provides comprehensive unit tests for the Lua lexer.
//
// This test file covers all token types, number/string parsing,
// error handling, and line/column tracking as specified in the
// Lua language reference implementation.
package lexer_test

import (
	"testing"

	"github.com/akzj/go-lua/pkg/lexer"
)

// TestEmptySource tests that empty input produces TK_EOF.
func TestEmptySource(t *testing.T) {
	l := lexer.NewLexer([]byte(""), "test")
	tok, err := l.NextToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != lexer.TK_EOF {
		t.Errorf("expected TK_EOF, got %v", tok.Type)
	}
}


// TestIdentifiers tests identifier tokenization.
func TestIdentifiers(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"simple", "foo", "foo"},
		{"underscore", "_var", "_var"},
		{"mixed", "myVar123", "myVar123"},
		{"underscore_num", "_123abc", "_123abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer([]byte(tt.input), "test")
			tok, err := l.NextToken()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.Type != lexer.TK_NAME {
				t.Errorf("expected TK_NAME, got %v", tok.Type)
			}
			if tok.Value != tt.expect {
				t.Errorf("expected value %q, got %q", tt.expect, tok.Value)
			}
		})
	}
}

// TestKeywords tests reserved word tokenization.
func TestKeywords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected lexer.TokenType
	}{
		{"if", "if", lexer.TK_IF},
		{"then", "then", lexer.TK_THEN},
		{"else", "else", lexer.TK_ELSE},
		{"function", "function", lexer.TK_FUNCTION},
		{"end", "end", lexer.TK_END},
		{"for", "for", lexer.TK_FOR},
		{"while", "while", lexer.TK_WHILE},
		{"return", "return", lexer.TK_RETURN},
		{"local", "local", lexer.TK_LOCAL},
		{"true", "true", lexer.TK_TRUE},
		{"false", "false", lexer.TK_FALSE},
		{"nil", "nil", lexer.TK_NIL},
		{"and", "and", lexer.TK_AND},
		{"or", "or", lexer.TK_OR},
		{"not", "not", lexer.TK_NOT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer([]byte(tt.input), "test")
			tok, err := l.NextToken()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.Type != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tok.Type)
			}
		})
	}
}

// TestIntegers tests integer literal tokenization.
func TestIntegers(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect int64
	}{
		{"zero", "0", 0},
		{"single", "42", 42},
		{"large", "123456789", 123456789},
		{"hex", "0xFF", 255},
		{"hex_upper", "0XFF", 255},
		{"hex_mixed", "0x1A2B", 6699},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer([]byte(tt.input), "test")
			tok, err := l.NextToken()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.Type != lexer.TK_INT {
				t.Errorf("expected TK_INT, got %v", tok.Type)
			}
			if tok.Value != tt.expect {
				t.Errorf("expected value %d, got %v", tt.expect, tok.Value)
			}
		})
	}
}

// TestFloats tests floating-point literal tokenization.
func TestFloats(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect float64
	}{
		{"simple", "3.14", 3.14},
		{"dot_start", ".5", 0.5},
		{"with_exp", "1e10", 1e10},
		{"exp_neg", "1e-5", 1e-5},
		{"exp_plus", "1e+5", 1e+5},
		{"hex_float", "0x1p2", 4.0},
		{"hex_float_frac", "0x1.5p2", 5.25},
		{"complex", "3.14e-2", 0.0314},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer([]byte(tt.input), "test")
			tok, err := l.NextToken()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.Type != lexer.TK_FLOAT {
				t.Errorf("expected TK_FLOAT, got %v", tok.Type)
			}
			if tok.Value != tt.expect {
				t.Errorf("expected value %v, got %v", tt.expect, tok.Value)
			}
		})
	}
}

// TestStrings tests string literal tokenization.
func TestStrings(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"double", `"hello"`, "hello"},
		{"single", `'hello'`, "hello"},
		{"empty_double", `""`, ""},
		{"empty_single", `''`, ""},
		{"escape_n", `"line1\nline2"`, "line1\nline2"},
		{"escape_t", `"tab\there"`, "tab\there"},
		{"escape_bslash", `"back\\slash"`, "back\\slash"},
		{"escape_quote", `"say \"hi\""`, `say "hi"`},
		{"escape_squote", `'it\'s'`, "it's"},
		{"escape_x", `"\x41"`, "A"},
		{"escape_u", `"\u{41}"`, "A"},
		{"escape_u_large", `"\u{1F600}"`, "😀"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer([]byte(tt.input), "test")
			tok, err := l.NextToken()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.Type != lexer.TK_STRING {
				t.Errorf("expected TK_STRING, got %v", tok.Type)
			}
			if tok.Value != tt.expect {
				t.Errorf("expected value %q, got %q", tt.expect, tok.Value)
			}
		})
	}
}

// TestLongStrings tests long string literal tokenization.
func TestLongStrings(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"simple", `[[hello]]`, "hello"},
		{"multiline", "[[line1\nline2]]", "line1\nline2"},
		{"with_equals", `[=[hello]=]`, "hello"},
		{"nested_bracket", `[=[hello]=]`, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer([]byte(tt.input), "test")
			tok, err := l.NextToken()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.Type != lexer.TK_STRING {
				t.Errorf("expected TK_STRING, got %v", tok.Type)
			}
			if tok.Value != tt.expect {
				t.Errorf("expected value %q, got %q", tt.expect, tok.Value)
			}
		})
	}
}

// TestOperators tests operator tokenization.
func TestOperators(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected lexer.TokenType
	}{
		{"plus", "+", lexer.TK_PLUS},
		{"minus", "-", lexer.TK_MINUS},
		{"star", "*", lexer.TK_STAR},
		{"slash", "/", lexer.TK_SLASH},
		{"percent", "%", lexer.TK_PERCENT},
		{"caret", "^", lexer.TK_CARET},
		{"hash", "#", lexer.TK_HASH},
		{"eq", "==", lexer.TK_EQ},
		{"ne", "~=", lexer.TK_NE},
		{"le", "<=", lexer.TK_LE},
		{"ge", ">=", lexer.TK_GE},
		{"lt", "<", lexer.TK_LT},
		{"gt", ">", lexer.TK_GT},
		{"shl", "<<", lexer.TK_SHL},
		{"shr", ">>", lexer.TK_SHR},
		{"idiv", "//", lexer.TK_IDIV},
		{"concat", "..", lexer.TK_CONCAT},
		{"dots", "...", lexer.TK_DOTS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer([]byte(tt.input), "test")
			tok, err := l.NextToken()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.Type != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tok.Type)
			}
		})
	}
}

// TestDelimiters tests delimiter tokenization.
func TestDelimiters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected lexer.TokenType
	}{
		{"lparen", "(", lexer.TK_LPAREN},
		{"rparen", ")", lexer.TK_RPAREN},
		{"lbrace", "{", lexer.TK_LBRACE},
		{"rbrace", "}", lexer.TK_RBRACE},
		{"lbrack", "[", lexer.TK_LBRACK},
		{"rbrack", "]", lexer.TK_RBRACK},
		{"comma", ",", lexer.TK_COMMA},
		{"semicolon", ";", lexer.TK_SEMICOLON},
		{"colon", ":", lexer.TK_COLON},
		{"dot", ".", lexer.TK_DOT},
		{"dbcolon", "::", lexer.TK_DBCOLON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer([]byte(tt.input), "test")
			tok, err := l.NextToken()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.Type != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tok.Type)
			}
		})
	}
}

// TestComments tests that comments are skipped.
func TestComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected lexer.TokenType
	}{
		{"short_comment", "-- comment\nreturn", lexer.TK_RETURN},
		{"long_comment", "--[[comment]]return", lexer.TK_RETURN},
		{"long_comment_eq", "--[=[comment]=]return", lexer.TK_RETURN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer([]byte(tt.input), "test")
			tok, err := l.NextToken()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.Type != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tok.Type)
			}
		})
	}
}

// TestLineColumnTracking tests line and column number tracking.
func TestLineColumnTracking(t *testing.T) {
	l := lexer.NewLexer([]byte("a\nb\nc"), "test")

	// Token 'a' at line 1, column 0
	tok, err := l.NextToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Line != 1 || tok.Column != 0 {
		t.Errorf("expected line 1, column 0, got line %d, column %d", tok.Line, tok.Column)
	}

	// Token 'b' at line 2, column 0
	tok, err = l.NextToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Line != 2 || tok.Column != 0 {
		t.Errorf("expected line 2, column 0, got line %d, column %d", tok.Line, tok.Column)
	}

	// Token 'c' at line 3, column 0
	tok, err = l.NextToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Line != 3 || tok.Column != 0 {
		t.Errorf("expected line 3, column 0, got line %d, column %d", tok.Line, tok.Column)
	}
}

// TestLineColumnWithSpaces tests column tracking with leading spaces.
func TestLineColumnWithSpaces(t *testing.T) {
	l := lexer.NewLexer([]byte("  x"), "test")
	tok, err := l.NextToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Line != 1 || tok.Column != 2 {
		t.Errorf("expected line 1, column 2, got line %d, column %d", tok.Line, tok.Column)
	}
}

// TestUnterminatedString tests error handling for unterminated strings.
func TestUnterminatedString(t *testing.T) {
	l := lexer.NewLexer([]byte(`"unterminated`), "test")
	_, err := l.NextToken()
	if err == nil {
		t.Error("expected error for unterminated string, got nil")
	}
}

// TestUnterminatedStringNewline tests error for newline in string.
func TestUnterminatedStringNewline(t *testing.T) {
	l := lexer.NewLexer([]byte("\"line1\nline2\""), "test")
	_, err := l.NextToken()
	if err == nil {
		t.Error("expected error for newline in string, got nil")
	}
}

// TestInvalidCharacter tests error handling for invalid characters.
func TestInvalidCharacter(t *testing.T) {
	l := lexer.NewLexer([]byte("@invalid"), "test")
	_, err := l.NextToken()
	if err == nil {
		t.Error("expected error for invalid character, got nil")
	}
}

// TestMalformedNumber tests error handling for malformed numbers.
func TestMalformedNumber(t *testing.T) {
	l := lexer.NewLexer([]byte("123abc"), "test")
	_, err := l.NextToken()
	if err == nil {
		t.Error("expected error for malformed number, got nil")
	}
}

// TestInvalidHexEscape tests error handling for invalid hex escape.
func TestInvalidHexEscape(t *testing.T) {
	l := lexer.NewLexer([]byte(`"\xGG"`), "test")
	_, err := l.NextToken()
	if err == nil {
		t.Error("expected error for invalid hex escape, got nil")
	}
}

// TestInvalidUTF8Escape tests error handling for invalid UTF-8 escape.
func TestInvalidUTF8Escape(t *testing.T) {
	l := lexer.NewLexer([]byte(`"\u{GG}"`), "test")
	_, err := l.NextToken()
	if err == nil {
		t.Error("expected error for invalid UTF-8 escape, got nil")
	}
}

// TestMultipleTokens tests tokenization of multiple tokens.
func TestMultipleTokens(t *testing.T) {
	l := lexer.NewLexer([]byte("if x > 0 then return x end"), "test")

	expected := []lexer.TokenType{
		lexer.TK_IF,
		lexer.TK_NAME,
		lexer.TK_GT,
		lexer.TK_INT,
		lexer.TK_THEN,
		lexer.TK_RETURN,
		lexer.TK_NAME,
		lexer.TK_END,
		lexer.TK_EOF,
	}

	for i, exp := range expected {
		tok, err := l.NextToken()
		if err != nil {
			t.Fatalf("unexpected error at token %d: %v", i, err)
		}
		if tok.Type != exp {
			t.Errorf("token %d: expected %v, got %v", i, exp, tok.Type)
		}
	}
}

// TestFunctionDefinition tests a complete function definition.
func TestFunctionDefinition(t *testing.T) {
	input := `function add(a, b)
    return a + b
end`
	l := lexer.NewLexer([]byte(input), "test")

	tokens := []lexer.TokenType{
		lexer.TK_FUNCTION,
		lexer.TK_NAME,
		lexer.TK_LPAREN,
		lexer.TK_NAME,
		lexer.TK_COMMA,
		lexer.TK_NAME,
		lexer.TK_RPAREN,
		lexer.TK_RETURN,
		lexer.TK_NAME,
		lexer.TK_PLUS,
		lexer.TK_NAME,
		lexer.TK_END,
		lexer.TK_EOF,
	}

	for i, exp := range tokens {
		tok, err := l.NextToken()
		if err != nil {
			t.Fatalf("unexpected error at token %d: %v", i, err)
		}
		if tok.Type != exp {
			t.Errorf("token %d: expected %v, got %v", i, exp, tok.Type)
		}
	}
}

