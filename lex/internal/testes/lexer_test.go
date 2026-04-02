package testes

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akzj/go-lua/lex"
	"github.com/akzj/go-lua/lex/api"
)

// TestAllTestesFiles tests that the lexer can parse all files in lua-master/testes/
func TestAllTestesFiles(t *testing.T) {
	testDir := "../../../lua-master/testes"
	files, err := filepath.Glob(filepath.Join(testDir, "*.lua"))
	if err != nil {
		t.Skipf("skipping: %v", err)
	}

	failed := []string{}
	for _, f := range files {
		name := filepath.Base(f)
		data, err := ioutil.ReadFile(f)
		if err != nil {
			t.Logf("SKIP %s: %v", name, err)
			continue
		}

		lexer := lex.NewLexer(string(data), f)

		func() {
			defer func() {
				if r := recover(); r != nil {
					failed = append(failed, fmt.Sprintf("%s: %v", name, r))
				}
			}()

			count := 0
			for {
				tok := lexer.NextToken()
				count++
				if tok.Type == api.TOKEN_EOS {
					t.Logf("PASS %s (%d tokens)", name, count)
					return
				}
				if count > 100000 {
					failed = append(failed, fmt.Sprintf("%s: too many tokens (>100000)", name))
					return
				}
			}
		}()
	}

	if len(failed) > 0 {
		for _, f := range failed {
			t.Errorf("FAILED: %s", f)
		}
		t.Fatalf("%d/%d files failed", len(failed), len(files))
	}
}

// TestLineContinuation tests the line continuation escape sequence
func TestLineContinuation(t *testing.T) {
	// Lua allows backslash followed by newline as line continuation
	source := `a = "test\
more"`

	lexer := lex.NewLexer(source, "test")
	tokens := []api.TokenType{api.TOKEN_NAME, api.TOKEN_ASSIGN, api.TOKEN_STRING, api.TOKEN_EOS}
	values := []string{"a", "", "testmore", ""}

	for i, expected := range tokens {
		tok := lexer.NextToken()
		if tok.Type != expected {
			t.Errorf("token %d: expected %s, got %s", i, api.TokenTypeName(expected), api.TokenTypeName(tok.Type))
		}
		if tok.Value != values[i] {
			t.Errorf("token %d value: expected %q, got %q", i, values[i], tok.Value)
		}
	}
}

// TestApiLua specifically tests api.lua which has line continuation
func TestApiLua(t *testing.T) {
	data, err := ioutil.ReadFile("../../../lua-master/testes/api.lua")
	if err != nil {
		t.Skipf("skipping: %v", err)
	}

	lexer := lex.NewLexer(string(data), "api.lua")

	count := 0
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC at token %d: %v", count, r)
		}
	}()

	for {
		tok := lexer.NextToken()
		count++
		// Debug: print tokens around failure
		if count >= 3020 && count <= 3035 {
			t.Logf("Token %d: type=%s line=%d col=%d value_len=%d", 
				count, api.TokenTypeName(tok.Type), tok.Line, tok.Column, len(tok.Value))
		}
		if tok.Type == api.TOKEN_EOS {
			t.Logf("SUCCESS: %d tokens", count)
			return
		}
		if count > 100000 {
			t.Errorf("too many tokens (>100000)")
			return
		}
	}
}

// TestStringsLua tests strings.lua
func TestStringsLua(t *testing.T) {
	data, err := ioutil.ReadFile("../../../lua-master/testes/strings.lua")
	if err != nil {
		t.Skipf("skipping: %v", err)
	}

	lexer := lex.NewLexer(string(data), "strings.lua")

	count := 0
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: %v", r)
		}
	}()

	for {
		tok := lexer.NextToken()
		count++
		if tok.Type == api.TOKEN_EOS {
			t.Logf("SUCCESS: %d tokens", count)
			return
		}
		if count > 100000 {
			t.Errorf("too many tokens")
			return
		}
	}
}

// ParseFile parses a file and returns token info for diff comparison
func ParseFile(filename string) (*ParseResult, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	lexer := lex.NewLexer(string(data), filename)
	result := &ParseResult{
		Filename: filename,
		Tokens:   []api.Token{},
	}

	for {
		tok := lexer.NextToken()
		result.Tokens = append(result.Tokens, tok)
		result.TokenCount++
		if tok.Type == api.TOKEN_EOS {
			break
		}
		if result.TokenCount > 100000 {
			return nil, fmt.Errorf("too many tokens (>100000)")
		}
	}

	return result, nil
}

// ParseResult holds the result of parsing a file
type ParseResult struct {
	Filename   string
	Tokens     []api.Token
	TokenCount int
}

// TokenSummary returns a summary string for diff comparison
func (r *ParseResult) TokenSummary() string {
	var b strings.Builder
	for i, tok := range r.Tokens {
		b.WriteString(fmt.Sprintf("%d\t%s\t%d\t%d\t%q\n",
			i, api.TokenTypeName(tok.Type), tok.Line, tok.Column, tok.Value))
	}
	return b.String()
}
