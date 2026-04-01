// Package vm implements the Lua 5.5.1 virtual machine execution engine.
//
// This package re-exports the public API from vm/api.
package vm

import (
	"github.com/akzj/go-lua/vm/api"
)

// Re-export types and interfaces
type Instruction = api.Instruction
type OpCode = api.OpCode
type VMExecutor = api.VMExecutor
type StackFrame = api.StackFrame

// Re-export helpers
var (
	GetOpCode = api.GetOpCode
	GetArgA   = api.GetArgA
	GetArgB   = api.GetArgB
	GetArgC   = api.GetArgC
	GetArgBx  = api.GetArgBx
	GetsBx    = api.GetsBx
	GetArgAx  = api.GetArgAx
	HasKBit   = api.HasKBit
)
