// Package bytecode provides Lua bytecode compilation functionality.
// Entry point for the bytecode module.
package bytecode

import (
	bcapi "github.com/akzj/go-lua/bytecode/api"
	bcinternal "github.com/akzj/go-lua/bytecode/internal"
)

// NewCompiler creates a new bytecode compiler for the given source.
// The compiler converts AST (ast.Chunk) into bytecode (bcapi.Prototype).
//
// Usage:
//   c := bytecode.NewCompiler("source name")
//   proto, err := c.Compile(chunk)
func NewCompiler(sourceName string) bcapi.Compiler {
	return bcinternal.NewCompiler(sourceName)
}

// GetCode extracts bytecode instructions from a prototype.
func GetCode(proto bcapi.Prototype) []uint32 {
	if p, ok := proto.(*bcinternal.Prototype); ok {
		return p.GetCode()
	}
	return nil
}

// GetConstants extracts the constant pool from a prototype.
func GetConstants(proto bcapi.Prototype) []*bcapi.Constant {
	if p, ok := proto.(*bcinternal.Prototype); ok {
		return p.GetConstants()
	}
	return nil
}
