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
// Aliased from opcodes.Instruction for convenience.
type Instruction = api.Instruction

// OpCode is a Lua VM opcode (0-84).
// Aliased from opcodes.OpCode for convenience.
type OpCode = api.OpCode

// VMExecutor executes Lua bytecode instructions.
//
// Invariant: Execute() and Run() are mutually exclusive.
// Calling Execute() while Run() is in progress causes undefined behavior.
//
// Why not just Run()?
// - Execute() allows single-step debugging
// - Execute() allows instruction interception
type VMExecutor interface {
	// Execute runs a single instruction.
	// Returns true to continue execution, false to suspend/stop.
	// May return error via the VM's error state (check after Run returns).
	Execute(inst Instruction) bool

	// Run executes instructions until completion or error.
	// Returns nil on normal completion, or error message on failure.
	Run() error
}

// =============================================================================
// Support interfaces (for implementation use)
// =============================================================================

// StackFrame represents a single function call frame on the VM stack.
//
// Invariant: base <= top (stack grows upward)
// Why not just use indices? Frames need saved state for non-local returns.
type StackFrame interface {
	// Base returns the base register index for this frame.
	Base() int

	// Func returns the function being executed in this frame.
	// Returns a TValue holding the closure/prototype.
	Func() types.TValue

	// Prev returns the previous frame, or nil if this is the bottom frame.
	Prev() StackFrame

	// PC returns the current instruction pointer.
	// Why PC instead of just nextPC? Callers need current PC for error reporting.
	PC() int

	// SetPC sets the instruction pointer (for jumps).
	SetPC(pc int)

	// Top returns one past the top of the active stack for this frame.
	Top() int
}

// =============================================================================
// Instruction helpers (pure, stateless)
// =============================================================================

// GetOpCode extracts the opcode from an instruction.
func GetOpCode(inst Instruction) OpCode {
	return OpCode(inst >> api.POS_OP & api.MAXARG_OP)
}

// GetArgA extracts the A argument (8 bits).
func GetArgA(inst Instruction) int {
	return int(inst >> api.POS_A & api.MAXARG_A)
}

// GetArgB extracts the B argument (8 bits).
func GetArgB(inst Instruction) int {
	return int(inst >> api.POS_B & api.MAXARG_B)
}

// GetArgC extracts the C argument (8 bits).
func GetArgC(inst Instruction) int {
	return int(inst >> api.POS_C & api.MAXARG_C)
}

// GetArgBx extracts the Bx argument (17 bits).
func GetArgBx(inst Instruction) int {
	return int(inst >> api.POS_Bx & api.MAXARG_Bx)
}

// GetsBx extracts the signed sBx argument.
func GetsBx(inst Instruction) int {
	return GetArgBx(inst) - api.OFFSET_sBx
}

// GetArgAx extracts the Ax argument (25 bits).
func GetArgAx(inst Instruction) int {
	return int(inst >> api.POS_Ax & api.MAXARG_Ax)
}

// HasKBit checks if the instruction uses a constant operand.
func HasKBit(inst Instruction) bool {
	return (inst & (1 << api.POS_k)) != 0
}
