// Package lua provides a public Go API for the Lua 5.5.1 interpreter.
//
// This package wraps the internal implementation, providing a clean,
// stable API for embedding Lua in Go applications.
//
// Basic usage:
//
//	L := lua.NewState()
//	defer L.Close()
//	if err := L.DoString(`print("hello")`); err != nil {
//	    log.Fatal(err)
//	}
package lua

import (
	"github.com/akzj/go-lua/internal/api"
	"github.com/akzj/go-lua/internal/object"
)

// Function is the signature for Go functions callable from Lua.
// The function receives the Lua state and returns the number of results
// it pushed onto the stack.
type Function func(L *State) int

// Type represents a Lua value type.
type Type int

// Lua value types. These match the C Lua LUA_T* constants.
const (
	TypeNil           Type = 0    // LUA_TNIL
	TypeBoolean       Type = 1    // LUA_TBOOLEAN
	TypeLightUserdata Type = 2    // LUA_TLIGHTUSERDATA
	TypeNumber        Type = 3    // LUA_TNUMBER
	TypeString        Type = 4    // LUA_TSTRING
	TypeTable         Type = 5    // LUA_TTABLE
	TypeFunction      Type = 6    // LUA_TFUNCTION
	TypeUserdata      Type = 7    // LUA_TUSERDATA
	TypeThread        Type = 8    // LUA_TTHREAD
	TypeNone          Type = -1   // LUA_TNONE — invalid/absent
)

// toPublicType converts internal object.Type to public Type.
func toPublicType(t object.Type) Type {
	if t == object.TypeNone {
		return TypeNone
	}
	return Type(t)
}

// toInternalType converts public Type to internal object.Type.
func toInternalType(t Type) object.Type {
	if t == TypeNone {
		return object.TypeNone
	}
	return object.Type(t)
}

// CompareOp is the comparison operation type for [State.Compare].
type CompareOp int

// Comparison operations.
const (
	OpEQ CompareOp = 0 // ==
	OpLT CompareOp = 1 // <
	OpLE CompareOp = 2 // <=
)

// ArithOp is the arithmetic operation type for [State.Arith].
type ArithOp int

// Arithmetic operations.
const (
	OpAdd  ArithOp = 0  // +
	OpSub  ArithOp = 1  // -
	OpMul  ArithOp = 2  // *
	OpMod  ArithOp = 3  // %
	OpPow  ArithOp = 4  // ^
	OpDiv  ArithOp = 5  // /
	OpIDiv ArithOp = 6  // //
	OpBAnd ArithOp = 7  // &
	OpBOr  ArithOp = 8  // |
	OpBXor ArithOp = 9  // ~
	OpShl  ArithOp = 10 // <<
	OpShr  ArithOp = 11 // >>
	OpUnm  ArithOp = 12 // - (unary)
	OpBNot ArithOp = 13 // ~ (unary)
)

// GCWhat is the GC operation type for [State.GC].
type GCWhat int

// GC operations.
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

// DebugInfo holds debug information about a function activation.
// Returned by [State.GetStack] and filled by [State.GetInfo].
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

	internal *api.DebugInfo // opaque handle for GetInfo/GetLocal/SetLocal
}

// copyFromInternal copies public fields from the internal DebugInfo.
func (d *DebugInfo) copyFromInternal() {
	if d.internal == nil {
		return
	}
	i := d.internal
	d.Source = i.Source
	d.ShortSrc = i.ShortSrc
	d.LineDefined = i.LineDefined
	d.LastLineDefined = i.LastLineDefined
	d.CurrentLine = i.CurrentLine
	d.Name = i.Name
	d.NameWhat = i.NameWhat
	d.What = i.What
	d.NUps = i.NUps
	d.NParams = i.NParams
	d.IsVararg = i.IsVararg
	d.IsTailCall = i.IsTailCall
	d.ExtraArgs = i.ExtraArgs
	d.FTransfer = i.FTransfer
	d.NTransfer = i.NTransfer
}

// Pseudo-indices and special constants.
const (
	RegistryIndex = -1001000 // pseudo-index for the registry
	MultiRet      = -1       // signals "return all results"
)

// UpvalueIndex returns the pseudo-index for upvalue i (1-based).
func UpvalueIndex(i int) int {
	return RegistryIndex - i
}

// Registry keys for well-known values.
const (
	RIdxMainThread = 1 // registry index of main thread
	RIdxGlobals    = 2 // registry index of global table
)

// Reference constants for [State.Ref] / [State.Unref].
const (
	RefNil = -1 // reference value for nil
	NoRef  = -2 // "no reference" sentinel
)

// Status codes returned by [State.PCall], [State.Resume], etc.
const (
	OK        = 0 // no errors
	Yield     = 1 // coroutine yielded
	ErrRun    = 2 // runtime error
	ErrSyntax = 3 // syntax error during compilation
	ErrMem    = 4 // memory allocation error
	ErrErr    = 5 // error in error handler
)

// Hook event masks for debug hooks.
const (
	MaskCall  = 1 << 0 // call hook
	MaskRet   = 1 << 1 // return hook
	MaskLine  = 1 << 2 // line hook
	MaskCount = 1 << 3 // count hook
)
