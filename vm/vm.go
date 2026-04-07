// Package vm implements the Lua 5.5.1 virtual machine execution engine.
package vm

import (
	"github.com/akzj/go-lua/vm/api"
	"github.com/akzj/go-lua/vm/internal"
)

// Re-export types and interfaces
type Instruction = api.Instruction
type OpCode = api.OpCode
type VMExecutor = api.VMExecutor
type StackFrame = api.StackFrame
type VMFrameManager = api.VMFrameManager
type GoFunc = api.GoFunc

// NewVMFrameManager creates a new VM frame manager for integrated execution.
var NewVMFrameManager = internal.NewVMFrameManager

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
