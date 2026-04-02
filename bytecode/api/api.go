// Package api defines the bytecode compiler interface.
// NO dependencies - pure interface definitions.
//
// Reference: lua-master/lcode.c (1972 lines)
package api

import (
	"fmt"

	astapi "github.com/akzj/go-lua/ast/api"
)

// =============================================================================
// Compiler Interface
// =============================================================================

// Prototype represents a compiled Lua function prototype.
// Returned by Compiler.Compile after AST compilation.
type Prototype interface {
	// SourceName returns the source name.
	SourceName() string
	// LineDefined returns the first line number.
	LineDefined() int
	// LastLineDefined returns the last line number.
	LastLineDefined() int
	// NumParams returns the number of parameters.
	NumParams() uint8
	// IsVararg returns true if function is vararg.
	IsVararg() bool
	// MaxStackSize returns the maximum stack size needed.
	MaxStackSize() uint8
}

// Compiler converts AST (ast.Chunk) to bytecode (Prototype).
type Compiler interface {
	// Compile converts an AST chunk to a bytecode prototype.
	Compile(chunk astapi.Chunk) (Prototype, error)
}

// =============================================================================
// Compilation Error
// =============================================================================

// CompileError represents a compilation error with source position.
type CompileError struct {
	Message string
	Line    int
	Column  int
}

func (e *CompileError) Error() string {
	return e.Message
}

// NewCompileError creates a compile error at the given position.
func NewCompileError(line, column int, format string, args ...interface{}) *CompileError {
	return &CompileError{
		Message: fmt.Sprintf(format, args...),
		Line:    line,
		Column:  column,
	}
}
