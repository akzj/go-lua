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
package parse

import (
	"github.com/akzj/go-lua/internal/lex"
	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// expKind describes the kind of an expression descriptor.
// C5 FIX: Order matches C lparser.h exactly (23 values, was 20).
// ---------------------------------------------------------------------------
type expKind int

const (
	vVOID     expKind = iota // empty (no expression)
	vNIL                     // constant nil
	vTRUE                    // constant true
	vFALSE                   // constant false
	vK                       // constant in k[]; Info = index
	vKFLT                    // float constant; NVal = value
	vKINT                    // integer constant; IVal = value
	vKSTR                    // string constant; StrVal = value
	vNONRELOC                // value in fixed register; Info = register
	vLOCAL                   // local variable; Var.RegIdx, Var.VarIdx
	vVARGVAR                 // vararg parameter (Lua 5.5); Var.RegIdx, Var.VarIdx
	vGLOBAL                  // global variable (Lua 5.5); Var.RegIdx, Var.VarIdx
	vUPVAL                   // upvalue; Info = upvalue index
	vCONST                   // compile-time <const>; Info = actvar index
	vINDEXED                 // t[k]; Ind.Table, Ind.Idx
	vVARGIND                 // indexed vararg parameter (Lua 5.5); Ind.*
	vINDEXUP                 // upval[K]; Ind.Table (upval), Ind.Idx (K index)
	vINDEXI                  // t[integer]; Ind.Table, Ind.Idx (int value)
	vINDEXSTR                // t["string"]; Ind.Table, Ind.Idx (K index)
	vJMP                     // test/comparison; Info = pc of JMP
	vRELOC                   // result in any register; Info = instruction pc
	vCALL                    // function call; Info = instruction pc
	vVARARG                  // vararg expression; Info = instruction pc
)

// noJump is the sentinel for empty jump lists.
const noJump = -1

// ---------------------------------------------------------------------------
// expDesc is the expression descriptor — the central compiler abstraction.
//
// It enables delayed code generation: an expression is described abstractly
// and only materialized (discharged to a register) when needed.
//
// The T and F fields are jump patch lists threaded through JMP instructions.
// ---------------------------------------------------------------------------
type expDesc struct {
	Kind   expKind
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
func (e *expDesc) HasJumps() bool {
	return e.T != noJump || e.F != noJump
}

// ---------------------------------------------------------------------------
// funcState tracks the compilation state of one function.
// Forms a linked list via Prev for nested functions.
// ---------------------------------------------------------------------------
type funcState struct {
	Proto *object.Proto // the Proto being built
	Prev  *funcState    // enclosing function (nil for main)
	Lex   *lex.LexState // shared lexer state
	Block *blockCnt     // current block scope

	// StringCache deduplicates *LuaString objects across all FuncStates
	// in the same compilation unit. Shared via openFunc inheritance.
	// This ensures identical string literals (even long strings) get the
	// same *LuaString pointer, matching C Lua's behavior for %p identity.
	StringCache map[string]*object.LuaString

	KCache     map[any]int // constant dedup cache (key → index in Proto.K)
	PC         int         // next code position (= len(Proto.Code) effectively)
	LastTarget int         // pc of last jump target (for optimization guard)
	PrevLine   int         // last line saved in lineinfo

	FirstLocal int // index of first local in Dyndata.ActVar
	FirstLabel int // index of first label in Dyndata.Labels

	NProtos    int // number of nested prototypes created
	NDebugVars int // number of debug local variables registered

	NumActVar int16 // number of active variable declarations
	NumUps    byte  // number of upvalues
	FreeReg   byte  // first free register
	NeedClose bool  // function needs OP_CLOSE on return
	IWthAbs   byte  // instructions since last absolute line info
}

// ---------------------------------------------------------------------------
// blockCnt tracks a lexical block (scope).
// ---------------------------------------------------------------------------
type blockCnt struct {
	Prev       *blockCnt // enclosing block
	FirstLabel int       // index of first label in this block
	FirstGoto  int       // index of first pending goto
	NumActVar  int16     // active vars at block entry
	HasUpval   bool      // some variable captured as upvalue
	IsLoop     byte      // 0=no, 1=loop, 2=loop with pending breaks
	InsideTBC  bool      // inside a to-be-closed scope
}

// ---------------------------------------------------------------------------
// dyndata is the shared dynamic data for the compilation unit.
// Shared across all nested functions.
// ---------------------------------------------------------------------------
type dyndata struct {
	ActVar []varDesc   // active variable descriptors
	Gotos  []labelDesc // pending goto statements
	Labels []labelDesc // defined labels
}

// varDesc describes a local variable during compilation.
type varDesc struct {
	Name   string        // variable name
	Kind   byte          // VDKREG, RDKCONST, etc.
	RegIdx byte          // register index
	PIdx   int           // index into Proto.LocVars (debug info)
	K      object.TValue // compile-time constant value (for RDKCTC)
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

// labelDesc describes a label or pending goto.
type labelDesc struct {
	Name      string
	PC        int // position in code
	Line      int
	NumActVar int16 // active variables at this point
	Close     bool  // goto needs OP_CLOSE
}

// ---------------------------------------------------------------------------
// Binary and unary operator enums (used by code generator)
// ---------------------------------------------------------------------------

// binOpr is a binary operator for the code generator.
type binOpr int

const (
	oprADD binOpr = iota
	oprSUB
	oprMUL
	oprMOD
	oprPOW
	oprDIV
	oprIDIV
	oprBAND
	oprBOR
	oprBXOR
	oprSHL
	oprSHR
	oprCONCAT
	oprEQ
	oprLT
	oprLE
	oprNE
	oprGT
	oprGE
	oprAND
	oprOR
	oprNOBINOPR // sentinel
)

// unOpr is a unary operator for the code generator.
type unOpr int

const (
	oprMINUS unOpr = iota
	oprBNOT
	oprNOT
	oprLEN
	oprNOUNOPR // sentinel
)

// maxVars is the maximum number of local variables in a function.
const maxVars = 200
