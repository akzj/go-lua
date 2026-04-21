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
	TypeNil           Type = Type(object.TypeNil)           // 0
	TypeBoolean       Type = Type(object.TypeBoolean)       // 1
	TypeLightUserdata Type = Type(object.TypeLightUserdata) // 2
	TypeNumber        Type = Type(object.TypeNumber)        // 3
	TypeString        Type = Type(object.TypeString)        // 4
	TypeTable         Type = Type(object.TypeTable)         // 5
	TypeFunction      Type = Type(object.TypeFunction)      // 6
	TypeUserdata      Type = Type(object.TypeUserdata)      // 7
	TypeThread        Type = Type(object.TypeThread)        // 8
	TypeNone          Type = -1                             // invalid/absent
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
type CompareOp = api.CompareOp

// Comparison operations.
const (
	OpEQ CompareOp = api.OpEQ // ==
	OpLT CompareOp = api.OpLT // <
	OpLE CompareOp = api.OpLE // <=
)

// ArithOp is the arithmetic operation type for [State.Arith].
type ArithOp = api.ArithOp

// Arithmetic operations.
const (
	OpAdd  ArithOp = api.OpAdd  // +
	OpSub  ArithOp = api.OpSub  // -
	OpMul  ArithOp = api.OpMul  // *
	OpMod  ArithOp = api.OpMod  // %
	OpPow  ArithOp = api.OpPow  // ^
	OpDiv  ArithOp = api.OpDiv  // /
	OpIDiv ArithOp = api.OpIDiv // //
	OpBAnd ArithOp = api.OpBAnd // &
	OpBOr  ArithOp = api.OpBOr  // |
	OpBXor ArithOp = api.OpBXor // ~
	OpShl  ArithOp = api.OpShl  // <<
	OpShr  ArithOp = api.OpShr  // >>
	OpUnm  ArithOp = api.OpUnm  // - (unary)
	OpBNot ArithOp = api.OpBNot // ~ (unary)
)

// GCWhat is the GC operation type for [State.GC].
type GCWhat = api.GCWhat

// GC operations.
const (
	GCStop      GCWhat = api.GCStop
	GCRestart   GCWhat = api.GCRestart
	GCCollect   GCWhat = api.GCCollect
	GCCount     GCWhat = api.GCCount
	GCCountB    GCWhat = api.GCCountB
	GCStep      GCWhat = api.GCStep
	GCIsRunning GCWhat = api.GCIsRunning
	GCGen       GCWhat = api.GCGen
	GCInc       GCWhat = api.GCInc
)

// DebugInfo holds debug information about a function activation.
// Returned by [State.GetStack] and filled by [State.GetInfo].
type DebugInfo = api.DebugInfo

// Pseudo-indices and special constants.
const (
	RegistryIndex = api.RegistryIndex // pseudo-index for the registry
	MultiRet      = api.MultiRet      // signals "return all results"
)

// UpvalueIndex returns the pseudo-index for upvalue i (1-based).
func UpvalueIndex(i int) int {
	return api.UpvalueIndex(i)
}

// Registry keys for well-known values.
const (
	RIdxMainThread = api.RIdxMainThread // registry index of main thread
	RIdxGlobals    = api.RIdxGlobals    // registry index of global table
)

// Reference constants for [State.Ref] / [State.Unref].
const (
	RefNil = api.RefNil // reference value for nil
	NoRef  = api.NoRef  // "no reference" sentinel
)

// Status codes returned by [State.PCall], [State.Resume], etc.
const (
	OK        = api.StatusOK
	ErrRun    = api.StatusErrRun
	ErrSyntax = api.StatusErrSyntax
	ErrMem    = api.StatusErrMem
	ErrErr    = api.StatusErrErr
)

// Hook event masks for debug hooks.
const (
	MaskCall  = 1 << 0 // call hook
	MaskRet   = 1 << 1 // return hook
	MaskLine  = 1 << 2 // line hook
	MaskCount = 1 << 3 // count hook
)
