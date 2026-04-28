package lua

import (
	"github.com/akzj/go-lua/internal/api"
	"github.com/akzj/go-lua/internal/object"
)

// Function is the signature for Go functions callable from Lua.
//
// The function receives the Lua state and returns the number of results
// it pushed onto the stack. Arguments passed from Lua are on the stack
// at positive indices starting from 1.
//
// Example:
//
//	greet := func(L *lua.State) int {
//	    name := L.CheckString(1)
//	    L.PushString("Hello, " + name + "!")
//	    return 1
//	}
type Function func(L *State) int

// Type represents a Lua value type, corresponding to the LUA_T* constants
// in the C Lua API.
type Type int

// Lua value types. These match the C Lua LUA_T* constants.
const (
	TypeNil           Type = 0  // LUA_TNIL — the nil value
	TypeBoolean       Type = 1  // LUA_TBOOLEAN — true or false
	TypeLightUserdata Type = 2  // LUA_TLIGHTUSERDATA — raw Go interface{} without metatable
	TypeNumber        Type = 3  // LUA_TNUMBER — integer or floating-point number
	TypeString        Type = 4  // LUA_TSTRING — immutable string
	TypeTable         Type = 5  // LUA_TTABLE — associative array
	TypeFunction      Type = 6  // LUA_TFUNCTION — Lua or Go function
	TypeUserdata      Type = 7  // LUA_TUSERDATA — full userdata with metatable support
	TypeThread        Type = 8  // LUA_TTHREAD — coroutine
	TypeNone          Type = -1 // LUA_TNONE — invalid or absent stack index
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

// Comparison operations for [State.Compare].
const (
	OpEQ CompareOp = 0 // == (may invoke __eq metamethod)
	OpLT CompareOp = 1 // <  (may invoke __lt metamethod)
	OpLE CompareOp = 2 // <= (may invoke __le metamethod)
)

// ArithOp is the arithmetic operation type for [State.Arith].
type ArithOp int

// Arithmetic and bitwise operations for [State.Arith].
const (
	OpAdd  ArithOp = 0  // + addition
	OpSub  ArithOp = 1  // - subtraction
	OpMul  ArithOp = 2  // * multiplication
	OpMod  ArithOp = 3  // % modulo
	OpPow  ArithOp = 4  // ^ exponentiation
	OpDiv  ArithOp = 5  // / float division
	OpIDiv ArithOp = 6  // // floor division
	OpBAnd ArithOp = 7  // & bitwise AND
	OpBOr  ArithOp = 8  // | bitwise OR
	OpBXor ArithOp = 9  // ~ bitwise XOR
	OpShl  ArithOp = 10 // << left shift
	OpShr  ArithOp = 11 // >> right shift
	OpUnm  ArithOp = 12 // - (unary minus)
	OpBNot ArithOp = 13 // ~ (bitwise NOT, unary)
)

// GCWhat is the GC operation type for [State.GC].
type GCWhat int

// Garbage collection operations for [State.GC].
const (
	GCStop      GCWhat = 0  // stop the garbage collector
	GCRestart   GCWhat = 1  // restart the garbage collector
	GCCollect   GCWhat = 2  // perform a full collection cycle
	GCCount     GCWhat = 3  // return total memory in use (KBytes)
	GCCountB    GCWhat = 4  // return remainder bytes of memory in use
	GCStep      GCWhat = 5  // perform an incremental GC step
	GCIsRunning GCWhat = 9  // return 1 if GC is running, 0 if stopped
	GCGen       GCWhat = 10 // switch to generational GC mode
	GCInc       GCWhat = 11 // switch to incremental GC mode
)

// DebugInfo holds debug information about a function activation record.
// Returned by [State.GetStack] and filled by [State.GetInfo].
//
// Not all fields are filled by every call. The "what" parameter to
// [State.GetInfo] controls which fields are populated:
//
//   - 'n': Name, NameWhat
//   - 'S': Source, ShortSrc, What, LineDefined, LastLineDefined
//   - 'l': CurrentLine
//   - 'u': NUps, NParams, IsVararg
//   - 'f': pushes the function onto the stack
//   - 'r': FTransfer, NTransfer
//   - 't': IsTailCall, ExtraArgs
type DebugInfo struct {
	Source          string // source of the chunk (e.g. "@filename.lua" or "=stdin")
	ShortSrc        string // short source for error messages (max 60 chars)
	LineDefined     int    // line where the function definition starts
	LastLineDefined int    // line where the function definition ends
	CurrentLine     int    // current line being executed
	Name            string // function name (if known)
	NameWhat        string // "global", "local", "method", "field", or ""
	What            string // "Lua", "C", or "main"
	NUps            int    // number of upvalues
	NParams         int    // number of fixed parameters
	IsVararg        bool   // true if the function is variadic
	IsTailCall      bool   // true if this is a tail call
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
	// RegistryIndex is the pseudo-index for the Lua registry, a global table
	// used to store values that need to persist across function calls.
	RegistryIndex = -1001000

	// MultiRet signals that a function call should return all results,
	// used as the nResults argument to [State.Call] and [State.PCall].
	MultiRet = -1
)

// UpvalueIndex returns the pseudo-index for the i-th upvalue (1-based).
// Use this inside a [Function] to access closure upvalues.
func UpvalueIndex(i int) int {
	return RegistryIndex - i
}

// Well-known registry keys for accessing standard values.
const (
	// RIdxMainThread is the registry index of the main thread.
	RIdxMainThread = 3

	// RIdxGlobals is the registry index of the global environment table.
	RIdxGlobals = 2
)

// Reference constants for [State.Ref] and [State.Unref].
const (
	// RefNil is the reference returned by [State.Ref] when the value is nil.
	RefNil = -1

	// NoRef is the "no reference" sentinel, indicating an invalid reference.
	NoRef = -2
)

// Status codes returned by [State.PCall], [State.Resume], and related functions.
const (
	OK        = 0 // no errors
	Yield     = 1 // coroutine yielded (from [State.Resume])
	ErrRun    = 2 // runtime error
	ErrSyntax = 3 // syntax error during compilation
	ErrMem    = 4 // memory allocation error
	ErrErr    = 5 // error while running the message handler
	ErrFile   = 6 // file I/O error (from [State.LoadFile])
)

// Hook event masks for [State.SetHook].
// Combine with bitwise OR to set multiple hooks.
const (
	MaskCall  = 1 << 0 // call hook — triggered on every function call
	MaskRet   = 1 << 1 // return hook — triggered on every function return
	MaskLine  = 1 << 2 // line hook — triggered on every new source line
	MaskCount = 1 << 3 // count hook — triggered every N instructions
)
