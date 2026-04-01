// Package lex provides Lua lexical analysis functionality.
package lex

import (
	"github.com/akzj/go-lua/lex/api"
	"github.com/akzj/go-lua/lex/internal"
)

// NewLexer creates a new Lexer from the given source code.
// Source must be non-nil. First line starts at line 1, column 1.
func NewLexer(source, sourceName string) api.Lexer {
	return internal.NewLexer(source, sourceName)
}
