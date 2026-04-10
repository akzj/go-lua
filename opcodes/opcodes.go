// Package opcodes defines Lua 5.5.1 VM opcodes.
// This package re-exports types and constants from subpackages.
//
// Structure:
//   - api: Opcode constants, OpMode, OpArgMask, instruction encoding
//   - internal: Opcode metadata and instruction manipulation helpers
package opcodes

import "github.com/akzj/go-lua/opcodes/api"

// Re-export commonly used types and constants
type (
	OpCode      = api.OpCode
	OpMode      = api.OpMode
	OpArgMask   = api.OpArgMask
	OpProperty  = api.OpProperty
	Instruction = uint32
)

const (
	NUM_OPCODES = api.NUM_OPCODES

	// OpModes
	OpModeABC  = api.OpModeABC
	OpModeVABC = api.OpModeVABC
	OpModeABx  = api.OpModeABx
	OpModeAsBx = api.OpModeAsBx
	OpModeAx   = api.OpModeAx
	OpModeSJ   = api.OpModeSJ

	// OpArgMasks
	OpArgN = api.OpArgN
	OpArgU = api.OpArgU
	OpArgR = api.OpArgR
	OpArgK = api.OpArgK

	// Argument limits
	MAXARG_A    = api.MAXARG_A
	MAXARG_B    = api.MAXARG_B
	MAXARG_C    = api.MAXARG_C
	MAXARG_Bx   = api.MAXARG_Bx
	MAXARG_Ax   = api.MAXARG_Ax
	MAXARG_sJ   = api.MAXARG_sJ
	OFFSET_sBx  = api.OFFSET_sBx
	OFFSET_sJ   = api.OFFSET_sJ

	// Stack limits
	MAX_FSTACK  = api.MAX_FSTACK
	NO_REG      = api.NO_REG
	MAXINDEXRK  = api.MAXINDEXRK

	// All 85 Opcodes (0-84)
	OP_MOVE       = api.OP_MOVE
	OP_LOADI      = api.OP_LOADI
	OP_LOADF      = api.OP_LOADF
	OP_LOADK      = api.OP_LOADK
	OP_LOADKX     = api.OP_LOADKX
	OP_LOADFALSE  = api.OP_LOADFALSE
	OP_LFALSESKIP = api.OP_LFALSESKIP
	OP_LOADTRUE   = api.OP_LOADTRUE
	OP_LOADNIL    = api.OP_LOADNIL
	OP_GETUPVAL   = api.OP_GETUPVAL
	OP_SETUPVAL   = api.OP_SETUPVAL
	OP_GETTABUP   = api.OP_GETTABUP
	OP_GETTABLE   = api.OP_GETTABLE
	OP_GETI       = api.OP_GETI
	OP_GETFIELD   = api.OP_GETFIELD
	OP_SETTABUP   = api.OP_SETTABUP
	OP_SETTABLE   = api.OP_SETTABLE
	OP_SETI       = api.OP_SETI
	OP_SETFIELD   = api.OP_SETFIELD
	OP_NEWTABLE   = api.OP_NEWTABLE
	OP_SELF       = api.OP_SELF
	OP_ADDI       = api.OP_ADDI
	OP_ADDK       = api.OP_ADDK
	OP_SUBK       = api.OP_SUBK
	OP_MULK       = api.OP_MULK
	OP_MODK       = api.OP_MODK
	OP_POWK       = api.OP_POWK
	OP_DIVK       = api.OP_DIVK
	OP_IDIVK      = api.OP_IDIVK
	OP_BANDK      = api.OP_BANDK
	OP_BORK       = api.OP_BORK
	OP_BXORK      = api.OP_BXORK
	OP_SHLI       = api.OP_SHLI
	OP_SHRI       = api.OP_SHRI
	OP_ADD        = api.OP_ADD
	OP_SUB        = api.OP_SUB
	OP_MUL        = api.OP_MUL
	OP_MOD        = api.OP_MOD
	OP_POW        = api.OP_POW
	OP_DIV        = api.OP_DIV
	OP_IDIV       = api.OP_IDIV
	OP_BAND       = api.OP_BAND
	OP_BOR        = api.OP_BOR
	OP_BXOR       = api.OP_BXOR
	OP_SHL        = api.OP_SHL
	OP_SHR        = api.OP_SHR
	OP_MMBIN      = api.OP_MMBIN
	OP_MMBINI     = api.OP_MMBINI
	OP_MMBINK     = api.OP_MMBINK
	OP_UNM        = api.OP_UNM
	OP_BNOT       = api.OP_BNOT
	OP_NOT        = api.OP_NOT
	OP_LEN        = api.OP_LEN
	OP_CONCAT     = api.OP_CONCAT
	OP_CLOSE      = api.OP_CLOSE
	OP_TBC        = api.OP_TBC
	OP_JMP        = api.OP_JMP
	OP_EQ         = api.OP_EQ
	OP_LT         = api.OP_LT
	OP_LE         = api.OP_LE
	OP_EQK        = api.OP_EQK
	OP_EQI        = api.OP_EQI
	OP_LTI        = api.OP_LTI
	OP_LEI        = api.OP_LEI
	OP_GTI        = api.OP_GTI
	OP_GEI        = api.OP_GEI
	OP_TEST       = api.OP_TEST
	OP_TESTSET    = api.OP_TESTSET
	OP_CALL       = api.OP_CALL
	OP_TAILCALL   = api.OP_TAILCALL
	OP_RETURN     = api.OP_RETURN
	OP_RETURN0    = api.OP_RETURN0
	OP_RETURN1    = api.OP_RETURN1
	OP_FORLOOP    = api.OP_FORLOOP
	OP_FORPREP    = api.OP_FORPREP
	OP_TFORPREP   = api.OP_TFORPREP
	OP_TFORCALL   = api.OP_TFORCALL
	OP_TFORLOOP   = api.OP_TFORLOOP
	OP_SETLIST    = api.OP_SETLIST
	OP_CLOSURE    = api.OP_CLOSURE
	OP_VARARG     = api.OP_VARARG
	OP_GETVARG    = api.OP_GETVARG
	OP_ERRNNIL    = api.OP_ERRNNIL
	OP_VARARGPREP = api.OP_VARARGPREP
	OP_EXTRAARG   = api.OP_EXTRAARG
	OP_SETTABLEN  = api.OP_SETTABLEN
)

// Name returns the string name of an opcode.
func Name(op OpCode) string {
	return api.OpCodeName(op)
}
