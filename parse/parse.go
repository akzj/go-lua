// Package parse provides Lua syntax parsing functionality.
// Entry point for the parse module.
package parse

import (
	parseapi "github.com/akzj/go-lua/parse/api"
	parseinternal "github.com/akzj/go-lua/parse/internal"
)

// NewParser creates a new Lua parser instance.
// The parser converts Lua source code into an AST (ast.Chunk).
//
// Usage:
//   p := parse.NewParser()
//   chunk, err := p.Parse("local x = 1")
func NewParser() parseapi.Parser {
	return parseinternal.NewParser()
}
