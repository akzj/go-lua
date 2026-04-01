package internal

import (
	"testing"

	"github.com/akzj/go-lua/lex/api"
)

func TestLexerKeywords(t *testing.T) {
	source := "and break do else elseif end false for function goto if in local nil not or repeat return then true until while"
	lexer := NewLexer(source, "test")

	expectedTokens := []api.TokenType{
		api.TOKEN_AND, api.TOKEN_BREAK, api.TOKEN_DO, api.TOKEN_ELSE, api.TOKEN_ELSEIF,
		api.TOKEN_END, api.TOKEN_FALSE, api.TOKEN_FOR, api.TOKEN_FUNCTION, api.TOKEN_GOTO,
		api.TOKEN_IF, api.TOKEN_IN, api.TOKEN_LOCAL, api.TOKEN_NIL, api.TOKEN_NOT,
		api.TOKEN_OR, api.TOKEN_REPEAT, api.TOKEN_RETURN, api.TOKEN_THEN, api.TOKEN_TRUE,
		api.TOKEN_UNTIL, api.TOKEN_WHILE,
	}

	for i, expected := range expectedTokens {
		token := lexer.NextToken()
		if token.Type != expected {
			t.Errorf("token %d: expected %s, got %s", i, api.TokenTypeName(expected), api.TokenTypeName(token.Type))
		}
	}
}

func TestLexerIdentifiers(t *testing.T) {
	source := "foo bar _test myVar"
	lexer := NewLexer(source, "test")

	for _, expectedName := range []string{"foo", "bar", "_test", "myVar"} {
		token := lexer.NextToken()
		if token.Type != api.TOKEN_NAME {
			t.Errorf("expected TOKEN_NAME, got %s", api.TokenTypeName(token.Type))
		}
		if token.Value != expectedName {
			t.Errorf("expected value %q, got %q", expectedName, token.Value)
		}
	}
}

func TestLexerNumbers(t *testing.T) {
	cases := []struct {
		source   string
		expected api.TokenType
	}{
		{"42", api.TOKEN_INTEGER},
		{"3.14", api.TOKEN_NUMBER},
		{"0xFF", api.TOKEN_INTEGER},
		{"1e10", api.TOKEN_NUMBER},
		{"0.5e-3", api.TOKEN_NUMBER},
	}

	for _, c := range cases {
		lexer := NewLexer(c.source, "test")
		token := lexer.NextToken()
		if token.Type != c.expected {
			t.Errorf("source %q: expected %s, got %s", c.source, api.TokenTypeName(c.expected), api.TokenTypeName(token.Type))
		}
		if token.Value != c.source {
			t.Errorf("source %q: expected value %q, got %q", c.source, c.source, token.Value)
		}
	}
}

func TestLexerStrings(t *testing.T) {
	cases := []struct {
		source     string
		value      string
	}{
		{`"hello"`, "hello"},
		{"'world'", "world"},
		{`"\n\t\\"`, "\n\t\\"},
	}

	for _, c := range cases {
		lexer := NewLexer(c.source, "test")
		token := lexer.NextToken()
		if token.Type != api.TOKEN_STRING {
			t.Errorf("source %q: expected TOKEN_STRING, got %s", c.source, api.TokenTypeName(token.Type))
		}
		if token.Value != c.value {
			t.Errorf("source %q: expected value %q, got %q", c.source, c.value, token.Value)
		}
	}
}

func TestLexerOperators(t *testing.T) {
	cases := []struct {
		source     string
		expected   api.TokenType
	}{
		{"==", api.TOKEN_EQ},
		{"<=", api.TOKEN_LE},
		{">=", api.TOKEN_GE},
		{"~=", api.TOKEN_NE},
		{"..", api.TOKEN_CONCAT},
		{"...", api.TOKEN_DOTS},
		{"//", api.TOKEN_IDIV},
		{"<<", api.TOKEN_SHL},
		{">>", api.TOKEN_SHR},
		{"::", api.TOKEN_DBCOLON},
	}

	for _, c := range cases {
		lexer := NewLexer(c.source, "test")
		token := lexer.NextToken()
		if token.Type != c.expected {
			t.Errorf("source %q: expected %s, got %s", c.source, api.TokenTypeName(c.expected), api.TokenTypeName(token.Type))
		}
	}
}

func TestLexerLineColumn(t *testing.T) {
	source := "line1\nline2\nline3"
	lexer := NewLexer(source, "test")

	token := lexer.NextToken()
	if token.Line != 1 || token.Column != 1 {
		t.Errorf("first token: expected (1,1), got (%d,%d)", token.Line, token.Column)
	}

	token = lexer.NextToken()
	if token.Line != 2 || token.Column != 1 {
		t.Errorf("second token: expected (2,1), got (%d,%d)", token.Line, token.Column)
	}

	token = lexer.NextToken()
	if token.Line != 3 || token.Column != 1 {
		t.Errorf("third token: expected (3,1), got (%d,%d)", token.Line, token.Column)
	}
}

func TestLexerComments(t *testing.T) {
	source := "x -- comment\ny"
	lexer := NewLexer(source, "test")

	token := lexer.NextToken()
	if token.Type != api.TOKEN_NAME || token.Value != "x" {
		t.Errorf("first token: expected NAME 'x', got %s '%s'", api.TokenTypeName(token.Type), token.Value)
	}

	token = lexer.NextToken()
	if token.Type != api.TOKEN_NAME || token.Value != "y" {
		t.Errorf("second token after comment: expected NAME 'y', got %s '%s'", api.TokenTypeName(token.Type), token.Value)
	}
}

func TestLexerEOS(t *testing.T) {
	lexer := NewLexer("x", "test")
	lexer.NextToken()
	token := lexer.NextToken()
	if token.Type != api.TOKEN_EOS {
		t.Errorf("expected TOKEN_EOS, got %s", api.TokenTypeName(token.Type))
	}
}

func TestLexerLookahead(t *testing.T) {
	lexer := NewLexer("x + y", "test")

	first := lexer.Lookahead()
	second := lexer.Lookahead()

	if first.Type != second.Type || first.Value != second.Value {
		t.Errorf("Lookahead should return same token: first=%v, second=%v", first, second)
	}

	consumed := lexer.NextToken()
	if consumed.Type != first.Type || consumed.Value != first.Value {
		t.Errorf("NextToken after Lookahead should return the looked-ahead token")
	}
}

func TestLexerLongStrings(t *testing.T) {
	source := "[[hello]]"
	lexer := NewLexer(source, "test")
	token := lexer.NextToken()

	if token.Type != api.TOKEN_STRING {
		t.Errorf("expected TOKEN_STRING for long string, got %s", api.TokenTypeName(token.Type))
	}
	if token.Value != "hello" {
		t.Errorf("expected value 'hello', got %q", token.Value)
	}
}

func TestLexerLongStringsWithEquals(t *testing.T) {
	source := "[=[hello]=]"
	lexer := NewLexer(source, "test")
	token := lexer.NextToken()

	if token.Type != api.TOKEN_STRING {
		t.Errorf("expected TOKEN_STRING for long string [=[...]=], got %s", api.TokenTypeName(token.Type))
	}
	if token.Value != "hello" {
		t.Errorf("expected value 'hello', got %q", token.Value)
	}
}
