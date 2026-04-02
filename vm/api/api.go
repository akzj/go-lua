// Package api defines the VM execution engine interface.
// NO dependencies on internal/ packages - pure interface definitions.
//
// Reference: lua-master/lvm.c, lua-master/lvm.h
//
// Design constraints:
// - VMExecutor is the only public interface
// - All implementation details live in vm/internal/
package api

import (
	"github.com/akzj/go-lua/opcodes/api"
	types "github.com/akzj/go-lua/types/api"
)

// Instruction is a 32-bit Lua VM instruction.
type Instruction = api.Instruction

// OpCode is a Lua VM opcode (0-84).
type OpCode = api.OpCode

// VMExecutor executes Lua bytecode instructions.
//
// Invariant: Execute() and Run() are mutually exclusive.
// Calling Execute() while Run() is in progress causes undefined behavior.
type VMExecutor interface {
	Execute(inst Instruction) bool
	Run() error
}

// VMFrameManager extends VMExecutor with frame management capabilities.
// Used by state package to integrate VM execution with Lua call frames.
type VMFrameManager interface {
	VMExecutor
	
	// SetStack shares a stack with the executor for integrated execution.
	SetStack(stack []types.TValue)
	
	// SetCode sets the bytecode instructions for the current frame.
	SetCode(code []Instruction)
	
	// SetKValues sets the constant pool for the current frame.
	SetKValues(kvalues []types.TValue)
	
	// PushFrame pushes a new stack frame for a function call.
	PushFrame(frame StackFrame)
	
	// PopFrame pops the current stack frame.
	PopFrame()
	
	// CurrentFrame returns the current frame without popping.
	CurrentFrame() StackFrame
	
	// FrameCount returns the number of frames on the stack.
	FrameCount() int
}

// StackFrame represents a single function call frame on the VM stack.
type StackFrame interface {
	Base() int
	Func() types.TValue
	Prev() StackFrame
	PC() int
	SetPC(pc int)
	Top() int
}

// NewVMFrameManager creates a new VM frame manager for integrated execution.
func NewVMFrameManager() VMFrameManager {
	panic("NewVMFrameManager must be implemented in vm/internal")
}

// =============================================================================
// Instruction helpers (pure, stateless)
// =============================================================================

func GetOpCode(inst Instruction) OpCode {
	return OpCode(inst >> api.POS_OP & api.MAXARG_OP)
}

func GetArgA(inst Instruction) int {
	return int(inst >> api.POS_A & api.MAXARG_A)
}

func GetArgB(inst Instruction) int {
	return int(inst >> api.POS_B & api.MAXARG_B)
}

func GetArgC(inst Instruction) int {
	c := int(inst >> api.POS_C & api.MAXARG_C)
	if (inst >> api.POS_k & 1) == 1 {
		c += 256
	}
	return c
}

func GetArgBx(inst Instruction) int {
	return int(inst >> api.POS_Bx & api.MAXARG_Bx)
}

func GetsBx(inst Instruction) int {
	return GetArgBx(inst) - api.OFFSET_sBx
}

func GetArgAx(inst Instruction) int {
	return int(inst >> api.POS_Ax & api.MAXARG_Ax)
}

func HasKBit(inst Instruction) bool {
	return (inst & (1 << api.POS_k)) != 0
}
