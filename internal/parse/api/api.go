// Package api defines the parser and code generator public interface.
//
// The parser is a single-pass recursive descent compiler that converts
// Lua source into Proto (compiled function prototypes). The public API
// is minimal: Parse() is the sole entry point.
//
// FuncState, ExpDesc, and BlockCnt are compiler-internal types. They are
// defined here for documentation but will be used only within the parse
// and codegen implementation.
//
// Reference: .analysis/06-compiler-pipeline.md §3-§4
package api

import (
	objectapi "github.com/akzj/go-lua/internal/object/api"
	lexapi "github.com/akzj/go-lua/internal/lex/api"
	opcodeapi "github.com/akzj/go-lua/internal/opcode/api"
)

// ---------------------------------------------------------------------------
// ExpKind describes the kind of an expression descriptor.
// C5 FIX: Order matches C lparser.h exactly (23 values, was 20).
// ---------------------------------------------------------------------------
type ExpKind int

const (
	VVOID     ExpKind = iota // empty (no expression)
	VNIL                     // constant nil
	VTRUE                    // constant true
	VFALSE                   // constant false
	VK                       // constant in k[]; Info = index
	VKFLT                    // float constant; NVal = value
	VKINT                    // integer constant; IVal = value
	VKSTR                    // string constant; StrVal = value
	VNONRELOC                // value in fixed register; Info = register
	VLOCAL                   // local variable; Var.RegIdx, Var.VarIdx
	VVARGVAR                 // vararg parameter (Lua 5.5); Var.RegIdx, Var.VarIdx
	VGLOBAL                  // global variable (Lua 5.5); Var.RegIdx, Var.VarIdx
	VUPVAL                   // upvalue; Info = upvalue index
	VCONST                   // compile-time <const>; Info = actvar index
	VINDEXED                 // t[k]; Ind.Table, Ind.Idx
	VVARGIND                 // indexed vararg parameter (Lua 5.5); Ind.*
	VINDEXUP                 // upval[K]; Ind.Table (upval), Ind.Idx (K index)
	VINDEXI                  // t[integer]; Ind.Table, Ind.Idx (int value)
	VINDEXSTR                // t["string"]; Ind.Table, Ind.Idx (K index)
	VJMP                     // test/comparison; Info = pc of JMP
	VRELOC                   // result in any register; Info = instruction pc
	VCALL                    // function call; Info = instruction pc
	VVARARG                  // vararg expression; Info = instruction pc
)

// NoJump is the sentinel for empty jump lists.
const NoJump = -1

// ---------------------------------------------------------------------------
// ExpDesc is the expression descriptor — the central compiler abstraction.
//
// It enables delayed code generation: an expression is described abstractly
// and only materialized (discharged to a register) when needed.
//
// The T and F fields are jump patch lists threaded through JMP instructions.
// ---------------------------------------------------------------------------
type ExpDesc struct {
	Kind   ExpKind
	Info   int     // generic: register, pc, upvalue index, K index
	IVal   int64   // for VKINT
	NVal   float64 // for VKFLT
	StrVal string  // for VKSTR

	// For indexed variables (VINDEXED, VINDEXUP, VINDEXI, VINDEXSTR)
	Ind struct {
		Table    byte // table register or upvalue index
		Idx      int  // key index (register, K index, or int value)
		ReadOnly bool // true if variable is read-only
		KeyStr   int  // K index of string key, or -1
	}

	// For local variables (VLOCAL)
	Var struct {
		RegIdx byte  // register holding the variable
		VarIdx int16 // index in actvar array
	}

	T int // patch list of "exit when true"
	F int // patch list of "exit when false"
}

// HasJumps returns true if the expression has pending jump lists.
func (e *ExpDesc) HasJumps() bool {
	return e.T != NoJump || e.F != NoJump
}

// ---------------------------------------------------------------------------
// FuncState tracks the compilation state of one function.
// Forms a linked list via Prev for nested functions.
// ---------------------------------------------------------------------------
type FuncState struct {
	Proto      *objectapi.Proto // the Proto being built
	Prev       *FuncState       // enclosing function (nil for main)
	Lex        *lexapi.LexState // shared lexer state
	Block      *BlockCnt        // current block scope

	KCache     map[any]int // constant dedup cache (key → index in Proto.K)
	PC         int         // next code position (= len(Proto.Code) effectively)
	LastTarget int         // pc of last jump target (for optimization guard)
	PrevLine   int         // last line saved in lineinfo

	FirstLocal int // index of first local in Dyndata.ActVar
	FirstLabel int // index of first label in Dyndata.Labels

	NProtos   int   // number of nested prototypes created
	NDebugVars int  // number of debug local variables registered

	NumActVar int16 // number of active variable declarations
	NumUps    byte  // number of upvalues
	FreeReg   byte  // first free register
	NeedClose bool  // function needs OP_CLOSE on return
	IWthAbs   byte  // instructions since last absolute line info
}

// ---------------------------------------------------------------------------
// BlockCnt tracks a lexical block (scope).
// ---------------------------------------------------------------------------
type BlockCnt struct {
	Prev       *BlockCnt // enclosing block
	FirstLabel int       // index of first label in this block
	FirstGoto  int       // index of first pending goto
	NumActVar  int16     // active vars at block entry
	HasUpval   bool      // some variable captured as upvalue
	IsLoop     byte      // 0=no, 1=loop, 2=loop with pending breaks
	InsideTBC  bool      // inside a to-be-closed scope
}

// ---------------------------------------------------------------------------
// Dyndata is the shared dynamic data for the compilation unit.
// Shared across all nested functions.
// ---------------------------------------------------------------------------
type Dyndata struct {
	ActVar []VarDesc   // active variable descriptors
	Gotos  []LabelDesc // pending goto statements
	Labels []LabelDesc // defined labels
}

// VarDesc describes a local variable during compilation.
type VarDesc struct {
	Name   string          // variable name
	Kind   byte            // VDKREG, RDKCONST, etc.
	RegIdx byte            // register index
	PIdx   int             // index into Proto.LocVars (debug info)
	K      objectapi.TValue // compile-time constant value (for RDKCTC)
}

// Variable kinds (C6 FIX: matches C lparser.h:102-108 exactly)
const (
	VDKREG     byte = 0 // regular local
	RDKCONST   byte = 1 // local constant
	RDKVAVAR   byte = 2 // vararg parameter (was incorrectly 3)
	RDKTOCLOSE byte = 3 // to-be-closed (was incorrectly 2)
	RDKCTC     byte = 4 // local compile-time constant (was missing)
	GDKREG     byte = 5 // regular global (was incorrectly 4)
	GDKCONST   byte = 6 // global constant (was incorrectly 5)
)

// LabelDesc describes a label or pending goto.
type LabelDesc struct {
	Name      string
	PC        int   // position in code
	Line      int
	NumActVar int16 // active variables at this point
	Close     bool  // goto needs OP_CLOSE
}

// ---------------------------------------------------------------------------
// Binary and unary operator enums (used by code generator)
// ---------------------------------------------------------------------------

// BinOpr is a binary operator for the code generator.
type BinOpr int

const (
	OPR_ADD BinOpr = iota
	OPR_SUB
	OPR_MUL
	OPR_MOD
	OPR_POW
	OPR_DIV
	OPR_IDIV
	OPR_BAND
	OPR_BOR
	OPR_BXOR
	OPR_SHL
	OPR_SHR
	OPR_CONCAT
	OPR_EQ
	OPR_LT
	OPR_LE
	OPR_NE
	OPR_GT
	OPR_GE
	OPR_AND
	OPR_OR
	OPR_NOBINOPR // sentinel
)

// UnOpr is a unary operator for the code generator.
type UnOpr int

const (
	OPR_MINUS UnOpr = iota
	OPR_BNOT
	OPR_NOT
	OPR_LEN
	OPR_NOUNOPR // sentinel
)

// MaxVars is the maximum number of local variables in a function.
const MaxVars = 200

// MaxFStack is the maximum register index (= MaxArgA = 255).
const MaxFStack = opcodeapi.MaxArgA

// LuaMultRet is the multi-return sentinel (LUA_MULTRET = -1).
const LuaMultRet = -1
