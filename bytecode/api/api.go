// Package api defines the bytecode compiler interface.
// NO dependencies - pure interface definitions.
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
	// GetCode returns the instruction list.
	GetCode() []uint32
	// GetConstants returns the constant list.
	GetConstants() []*Constant
	// GetSubProtos returns the list of nested function prototypes.
	GetSubProtos() []Prototype
	// GetUpvalues returns the upvalue descriptors for this prototype.
	GetUpvalues() []UpvalueDesc
}

// UpvalueDesc describes an upvalue captured by a closure.
type UpvalueDesc struct {
	Name    string
	Instack uint8 // 1 = capture from enclosing function's register, 0 = copy from enclosing function's upvalue
	Idx     uint8 // register index (if Instack==1) or upvalue index (if Instack==0)
	Kind    uint8
}

// Constant represents a compile-time constant value.
type Constant struct {
	Type  ConstantType
	Int   int64
	Float float64
	Str   string
}

// ConstantType identifies the type of a constant.
type ConstantType uint8

const (
	ConstNil ConstantType = iota
	ConstInteger
	ConstFloat
	ConstString
	ConstBool
)

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
