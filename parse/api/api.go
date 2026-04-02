// Package api defines the Lua parser interface.
// NO dependencies - pure interface definitions.
package api

import (
	"fmt"

	astapi "github.com/akzj/go-lua/ast/api"
	lexapi "github.com/akzj/go-lua/lex/api"
)

// =============================================================================
// Parser Interface
// =============================================================================

// Parser converts lex.Token streams into ast.Chunk.
// Implementation uses recursive descent parsing.
//
// Why not just return AST? Parser may need state for incremental parsing,
// error recovery, and source position tracking.
type Parser interface {
	// Parse parses a complete Lua chunk (file or string).
	// Returns the root AST node or error on syntax failure.
	Parse(chunk string) (astapi.Chunk, error)

	// ParseExpression parses a single expression.
	// Used for -e flag, loadstring, and REPL.
	// Returns the expression node or error.
	ParseExpression(expr string) (astapi.ExpNode, error)
}

// =============================================================================
// Token Helper (for error reporting)
// =============================================================================

// TokenHelper provides token access for position reporting.
type TokenHelper interface {
	// Token returns the current token.
	Token() lexapi.Token
	// NextToken returns the next token (lookahead).
	NextToken() lexapi.Token
}

// =============================================================================
// Parse Errors
// =============================================================================

// ParseError represents a syntax error with position information.
type ParseError struct {
	Message string
	Line    int
	Column  int
}

func (e *ParseError) Error() string {
	return e.Message
}

// NewParseError creates a parse error at the given position.
func NewParseError(line, column int, format string, args ...interface{}) *ParseError {
	return &ParseError{
		Message: fmt.Sprintf(format, args...),
		Line:    line,
		Column:  column,
	}
}
