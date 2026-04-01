package lcode

/*
** $Id: lcode.go $
** Code Generator - Thin wrapper re-exporting from lparser
** Types and functions are implemented in lparser package
*/

import (
	"github.com/akzj/go-lua/internal/lopcodes"
	"github.com/akzj/go-lua/internal/lparser"
)

// Re-export constants
const (
	NO_JUMP      = lparser.NO_JUMP
	LUA_MULTRET  = lparser.LUA_MULTRET
	MAXVARS      = lparser.MAXVARS
	MAXARG_sJ    = lopcodes.MAXARG_sJ
	_MAXSTACK    = lopcodes.MAX_STACK
	_MAXARG_Bx   = lopcodes.MAXARG_Bx
	_MAX_FSTACK  = lopcodes.MAX_STACK
)

// Re-export types
type (
	BinOpr = lparser.BinOpr
	UnOpr  = lparser.UnOpr
	Expdesc = lparser.Expdesc
)

// Binary operators
const (
	OPR_ADD    BinOpr = lparser.OPR_ADD
	OPR_SUB    BinOpr = lparser.OPR_SUB
	OPR_MUL    BinOpr = lparser.OPR_MUL
	OPR_MOD    BinOpr = lparser.OPR_MOD
	OPR_POW    BinOpr = lparser.OPR_POW
	OPR_DIV    BinOpr = lparser.OPR_DIV
	OPR_IDIV   BinOpr = lparser.OPR_IDIV
	OPR_BAND   BinOpr = lparser.OPR_BAND
	OPR_BOR    BinOpr = lparser.OPR_BOR
	OPR_BXOR   BinOpr = lparser.OPR_BXOR
	OPR_SHL    BinOpr = lparser.OPR_SHL
	OPR_SHR    BinOpr = lparser.OPR_SHR
	OPR_CONCAT BinOpr = lparser.OPR_CONCAT
	OPR_EQ     BinOpr = lparser.OPR_EQ
	OPR_LT     BinOpr = lparser.OPR_LT
	OPR_LE     BinOpr = lparser.OPR_LE
	OPR_NE     BinOpr = lparser.OPR_NE
	OPR_GT     BinOpr = lparser.OPR_GT
	OPR_GE     BinOpr = lparser.OPR_GE
	OPR_AND    BinOpr = lparser.OPR_AND
	OPR_OR     BinOpr = lparser.OPR_OR
	OPR_NOBINOPR BinOpr = lparser.OPR_NOBINOPR
)

// Unary operators
const (
	OPR_MINUS   UnOpr = lparser.OPR_MINUS
	OPR_BNOT    UnOpr = lparser.OPR_BNOT
	OPR_NOT     UnOpr = lparser.OPR_NOT
	OPR_LEN     UnOpr = lparser.OPR_LEN
	OPR_NOUNOPR UnOpr = lparser.OPR_NOUNOPR
)

// Re-export functions
var (
	Code         = lparser.Code
	CodeABC      = lparser.CodeABC
	CodeABx      = lparser.CodeABx
	Jump         = lparser.Jump
	PatchList    = lparser.PatchList
	PatchToHere  = lparser.PatchToHere
	Concat       = lparser.Concat
	CheckStack   = lparser.CheckStack
	ReserveRegs  = lparser.ReserveRegs
	Nil          = lparser.Nil
	Ret          = lparser.Ret
)

// Additional helpers needed by tests
func getJump(fs *lparser.FuncState, pc int) int {
	offset := lopcodes.GETARG_sJ(fs.F.Code[pc])
	if offset == NO_JUMP {
		return NO_JUMP
	}
	return pc + 1 + offset
}

func CodeABCk(fs *lparser.FuncState, o lopcodes.OpCode, a, b, c, k int) int {
	return lparser.Code(fs, lopcodes.CREATE_ABCk(o, a, b, c, k))
}

func CodesJ(fs *lparser.FuncState, o lopcodes.OpCode, sj, k int) int {
	return lparser.Code(fs, lopcodes.CREATE_sJ(o, sj, k))
}

func LoadK(fs *lparser.FuncState, reg, k int) int {
	if k <= lopcodes.MAXARG_Bx {
		return CodeABx(fs, lopcodes.OP_LOADK, reg, k)
	}
	p := CodeABx(fs, lopcodes.OP_LOADKX, reg, 0)
	CodeABx(fs, lopcodes.OP_EXTRAARG, 0, k)
	return p
}

func LoadBool(fs *lparser.FuncState, reg, b int) {
	if b != 0 {
		CodeABC(fs, lopcodes.OP_LOADTRUE, reg, 0, 0)
	} else {
		CodeABC(fs, lopcodes.OP_LOADFALSE, reg, 0, 0)
	}
}

// MAXINDEXRK - maximum index that can be in a RK register
const MAXINDEXRK = lopcodes.MAXARG_Bx

func fitsC(i int64) bool {
	return i >= 0 && i <= lopcodes.MAXARG_C
}

func fitsBx(i int64) bool {
	return i >= -lopcodes.OFFSET_sBx && i <= lopcodes.MAXARG_Bx-lopcodes.OFFSET_sBx
}

// Number operations
func numadd(a, b float64) float64 { return a + b }
func numsub(a, b float64) float64 { return a - b }
func nummul(a, b float64) float64 { return a * b }
func numdiv(a, b float64) float64 { return a / b }
func nummod(a, b float64) float64 { return a - b }
func numpow(a, b float64) float64 { return a * b }
func numunm(a float64) float64    { return -a }
func numisnan(a float64) bool      { return false }