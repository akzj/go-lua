// Package api defines the public Go API for the Lua VM.
//
// This is the Go equivalent of C's lua_* functions (lapi.c) and
// luaL_* auxiliary functions (lauxlib.c).
// It provides a stack-based API for manipulating Lua state from Go.
// All standard library implementations use this API.
//
// Reference: .analysis/09-standard-libraries.md §1-§2
package api

import (
	"github.com/akzj/go-lua/internal/object"
)

// CFunction is the type for Go functions callable from Lua.
// It receives the Lua state and returns the number of results pushed.
type CFunction func(L *State) int

// State is the public Lua state handle.
// It wraps the internal LuaState and provides the stack-based API.
type State struct {
	// Internal fields set during construction — not exported.
	// The implementation file will define these.
	Internal any // *state.LuaState (avoids circular import)
}

// --- Pseudo-Indices ---

const (
	RegistryIndex = -1000000 - 1000 // LUA_REGISTRYINDEX
)

// UpvalueIndex returns the pseudo-index for upvalue i (1-based).
func UpvalueIndex(i int) int {
	return RegistryIndex - i
}

// Registry keys for well-known values.
const (
	RIdxMainThread = 1 // LUA_RIDX_MAINTHREAD
	RIdxGlobals    = 2 // LUA_RIDX_GLOBALS — the _G table
)

// CompareOp is the comparison operation type.
type CompareOp int

const (
	OpEQ CompareOp = 0 // ==
	OpLT CompareOp = 1 // <
	OpLE CompareOp = 2 // <=
)

// ArithOp is the arithmetic operation type.
type ArithOp int

const (
	OpAdd  ArithOp = 0
	OpSub  ArithOp = 1
	OpMul  ArithOp = 2
	OpMod  ArithOp = 3
	OpPow  ArithOp = 4
	OpDiv  ArithOp = 5
	OpIDiv ArithOp = 6
	OpBAnd ArithOp = 7
	OpBOr  ArithOp = 8
	OpBXor ArithOp = 9
	OpShl  ArithOp = 10
	OpShr  ArithOp = 11
	OpUnm  ArithOp = 12
	OpBNot ArithOp = 13
)

// GCWhat is the GC operation type.
type GCWhat int

const (
	GCStop      GCWhat = 0
	GCRestart   GCWhat = 1
	GCCollect   GCWhat = 2
	GCCount     GCWhat = 3
	GCCountB    GCWhat = 4
	GCStep      GCWhat = 5
	GCIsRunning GCWhat = 9
	GCGen       GCWhat = 10
	GCInc       GCWhat = 11
)

// RefNil is the reference value for nil.
const RefNil = -1

// NoRef is the "no reference" value.
const NoRef = -2

// DebugInfo holds debug information about a function activation.
type DebugInfo struct {
	Source          string // source of the chunk
	ShortSrc        string // short source (for error messages)
	LineDefined     int    // line where definition starts
	LastLineDefined int    // line where definition ends
	CurrentLine     int    // current line
	Name            string // function name (if known)
	NameWhat        string // "global", "local", "method", "field", ""
	What            string // "Lua", "C", "main"
	NUps            int    // number of upvalues
	NParams         int    // number of parameters
	IsVararg        bool   // is a vararg function
	IsTailCall      bool   // is a tail call
	ExtraArgs       int    // number of extra arguments (vararg)
	FTransfer       int    // index of first transferred value (for call/return hooks)
	NTransfer       int    // number of transferred values (for call/return hooks)
	CI              interface{} // internal: *state.CallInfo
	ThreadState     interface{} // internal: *state.LuaState (set by GetStack)
}

// Status codes
const (
	StatusOK     = 0
	StatusYield  = 1
	StatusErrRun = 2
	StatusErrSyntax = 3
	StatusErrMem = 4
	StatusErrErr = 5
)

// MultiRet signals "return all results".
const MultiRet = -1

// TypeNone is the type for invalid indices.
const TypeNone object.Type = 0xFF
