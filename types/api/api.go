// Package api defines Lua 5.5.1 core type interfaces.
// NO dependencies - pure interface definitions.
package api

import (
	"sync"
	"unsafe"
)

// =============================================================================
// Type Tag Constants
// =============================================================================

const (
	LUA_TNIL           = 0
	LUA_TBOOLEAN       = 1
	LUA_TLIGHTUSERDATA = 2
	LUA_TNUMBER        = 3
	LUA_TSTRING        = 4
	LUA_TTABLE         = 5
	LUA_TFUNCTION      = 6
	LUA_TUSERDATA      = 7
	LUA_TTHREAD        = 8
	LUA_NUMTYPES       = 8

	LUA_TUPVAL   = LUA_NUMTYPES
	LUA_TPROTO   = LUA_NUMTYPES + 1
	LUA_TDEADKEY = LUA_NUMTYPES + 2
	LUA_TOTALTYPES = LUA_TPROTO + 2
)

const (
	variantShift      = 4
	variantMask       = 0x30
	BIT_ISCOLLECTABLE = 1 << 6
)

// Nil variants
const (
	LUA_VNIL     = LUA_TNIL | (0 << variantShift)
	LUA_VEMPTY   = LUA_TNIL | (1 << variantShift)
	LUA_VABSTKEY = LUA_TNIL | (2 << variantShift)
	LUA_VNOTABLE = LUA_TNIL | (3 << variantShift)
)

// Boolean variants
const (
	LUA_VFALSE = LUA_TBOOLEAN | (0 << variantShift)
	LUA_VTRUE  = LUA_TBOOLEAN | (1 << variantShift)
)

// Number variants
const (
	LUA_VNUMINT = LUA_TNUMBER | (0 << variantShift)
	LUA_VNUMFLT = LUA_TNUMBER | (1 << variantShift)
)

// String variants
const (
	LUA_VSHRSTR = LUA_TSTRING | (0 << variantShift)
	LUA_VLNGSTR = LUA_TSTRING | (1 << variantShift)
)

// Function variants
const (
	LUA_VLCL = LUA_TFUNCTION | (0 << variantShift)
	LUA_VLCF = LUA_TFUNCTION | (1 << variantShift)
	LUA_VCCL = LUA_TFUNCTION | (2 << variantShift)
)

// Other variants
const (
	LUA_VTHREAD        = LUA_TTHREAD | (0 << variantShift)
	LUA_VLIGHTUSERDATA = LUA_TLIGHTUSERDATA | (0 << variantShift)
	LUA_VUSERDATA      = LUA_TUSERDATA | (0 << variantShift)
	LUA_VPROTO         = LUA_TPROTO | (0 << variantShift)
	LUA_VUPVAL         = LUA_TUPVAL | (0 << variantShift)
	LUA_VTABLE         = LUA_TTABLE | (0 << variantShift)
)

const (
	PF_VARARG_HIDDEN = 1
	PF_VARARG_TABLE  = 2
	PF_FIXED         = 4
)

const (
	LUA_MAXINTEGER = LuaInteger(0x7FFFFFFFFFFFFFFF)
	LUA_MININTEGER = -LUA_MAXINTEGER - 1
)

// =============================================================================
// Type Aliases
// =============================================================================

type LuaNumber  float64
type LuaInteger int64
type LuaByte    uint8
type LuaSByte   int8
type LuaMem     int
type LuaUMem    uint
type TStatus    uint8
type CFunction  unsafe.Pointer
type Instruction uint32

type ValueVariant uint8

const (
	ValueGC        ValueVariant = iota
	ValuePointer
	ValueCFunction
	ValueInteger
	ValueFloat
)

// =============================================================================
// Interfaces
// =============================================================================

type GCObject interface {
	IsTable() bool
}

type Value interface {
	GetGC() *GCObject
	GetPointer() unsafe.Pointer
	GetCFunction() unsafe.Pointer
	GetInteger() LuaInteger
	GetFloat() LuaNumber
}

type TValue interface {
	IsNil() bool
	IsBoolean() bool
	IsNumber() bool
	IsInteger() bool
	IsFloat() bool
	IsString() bool
	IsTable() bool
	IsFunction() bool
	IsThread() bool
	IsLightUserData() bool
	IsUserData() bool
	IsCollectable() bool
	IsTrue() bool
	IsFalse() bool
	IsLClosure() bool
	IsCClosure() bool
	IsLightCFunction() bool
	IsClosure() bool
	IsProto() bool
	IsUpval() bool
	IsShortString() bool
	IsLongString() bool
	IsEmpty() bool
	GetTag() int
	GetBaseType() int
	GetValue() interface{}
	GetGC() *GCObject
	GetInteger() LuaInteger
	GetFloat() LuaNumber
	GetPointer() unsafe.Pointer
}

type TString interface {
	IsShort() bool
	Len() int
	Hash() uint32
}

type Table interface {
	SizeNode() int
}

type Node interface {
	KeyIsNil() bool
	KeyIsDead() bool
	KeyIsCollectable() bool
}

// UpVal represents a Lua upvalue — a local variable from an outer scope
// that has been captured by a closure. Upvalues can be in one of two states:
//
//   - Open: the upvalue points to a slot on the Lua stack (the variable is
//     still in scope). The V.P field holds a pointer to that stack slot.
//     The U.Open fields form a doubly-linked list of all open upvalues.
//
//   - Closed: the variable has gone out of scope. The upvalue holds an
//     independent copy of the value in U.Value.
//
// An upvalue transitions from open to closed via luaF_close(), which copies
// the value out of the stack slot into U.Value and unlinks it from the list.
//
// The UpVal interface is implemented by *internal.UpVal (types/internal/closure.go).
// Use NewOpenUpval() and NewClosedUpval() factory functions to create instances.
type UpVal interface {
	// IsOpen returns true if this upvalue is currently open (pointing to a
	// stack slot). Returns false if closed.
	IsOpen() bool

	// GetStackPtr returns the pointer to the stack slot this upvalue refers to.
	// Only valid when IsOpen() is true. Returns nil for closed upvalues.
	GetStackPtr() unsafe.Pointer

	// GetValue returns the captured value. For open upvalues, this reads
	// through to the stack slot. For closed upvalues, this returns U.Value.
	GetValue() TValue

	// SetValue sets the captured value. For open upvalues, this writes through
	// to the stack slot. For closed upvalues, this stores the value in U.Value.
	SetValue(v TValue)

	// Open returns the doubly-linked list entry for open upvalues.
	// Only valid when IsOpen() is true.
	Open() UpValOpen
}

// UpValOpen represents the open-state fields of an upvalue.
// This is embedded in the internal UpVal struct; the interface allows
// state/internal to access it without importing types/internal.
type UpValOpen interface {
	Next() UpVal
	// Previous returns a pointer to the head-pointer for this upvalue's position
	// in the list. This enables O(1) unlinking without traversing from head.
	// - For the head upvalue: Previous() == &headPointer
	// - For other upvalues: Previous() == &prev.Open()
	Previous() *UpVal
	// SetNextPtr updates the Next field. Used for unlinking from the open list.
	SetNextPtr(next UpVal)
	// SetPreviousPtr updates the Previous pointer. Used for linking/unlinking.
	SetPreviousPtr(prev *UpVal)
}

type Closure interface {
	IsC() bool
}

type Proto interface {
	IsVararg() bool
}

type Upvaldesc interface{}
type LocVar interface{}
type AbsLineInfo interface{}
type Udata interface{}
type StackValue interface{}

// Utility functions (pure, no dependencies)
func MakeVariant(t, v int) int { return t | (v << variantShift) }
func Ctb(t int) int { return t | BIT_ISCOLLECTABLE }
func Novariant(t int) int { return t & 0x0F }
func WithVariant(t int) int { return t & 0x3F }

// =============================================================================
// TValue Constructors (inline implementations)
// =============================================================================

// NewTValueNil creates a nil TValue.
func NewTValueNil() TValue { return &tvalueStruct{Tt: uint8(LUA_VNIL)} }

// NewTValueBoolean creates a boolean TValue.
func NewTValueBoolean(b bool) TValue {
	tt := LUA_VFALSE
	if b {
		tt = LUA_VTRUE
	}
	return &tvalueStruct{Tt: uint8(tt)}
}

// NewTValueInteger creates an integer TValue.
func NewTValueInteger(i LuaInteger) TValue {
	return &tvalueStruct{Value: &valueStruct{Variant: ValueInteger, Data_: i}, Tt: uint8(LUA_VNUMINT)}
}

// NewTValueFloat creates a float TValue.
func NewTValueFloat(n LuaNumber) TValue {
	return &tvalueStruct{Value: &valueStruct{Variant: ValueFloat, Data_: n}, Tt: uint8(LUA_VNUMFLT)}
}

// NewTValueLightUserData creates a light userdata TValue.
func NewTValueLightUserData(p unsafe.Pointer) TValue {
	return &tvalueStruct{Value: &valueStruct{Variant: ValuePointer, Data_: p}, Tt: uint8(LUA_VLIGHTUSERDATA)}
}

// internedStrings is a global intern map for short strings (≤40 bytes).
// This ensures that identical short strings share the same Go string backing,
// making their unsafe.Pointer addresses identical (needed for %p format).
var internedStrings sync.Map

const maxInternLen = 40

// InternString returns the canonical copy of a short string.
func InternString(s string) string {
	if len(s) > maxInternLen {
		return s
	}
	if v, ok := internedStrings.Load(s); ok {
		return v.(string)
	}
	internedStrings.Store(s, s)
	return s
}

// NewTValueString creates a string TValue.
// Short strings (≤40 bytes) are interned so identical strings share
// the same backing pointer (important for %p format correctness).
func NewTValueString(s string) TValue {
	s = InternString(s)
	tt := uint8(Ctb(int(LUA_VSHRSTR)))
	if len(s) > maxInternLen {
		tt = uint8(Ctb(int(LUA_VLNGSTR)))
	}
	return &tvalueStruct{Value: &valueStruct{Variant: ValueGC, Data_: s}, Tt: tt}
}

// NewTValueLightCFunction creates a light C function TValue.
func NewTValueLightCFunction(fn unsafe.Pointer) TValue {
	return &tvalueStruct{Value: &valueStruct{Variant: ValueCFunction, Data_: fn}, Tt: uint8(LUA_VLCF)}
}

// NewTValueThread creates a thread TValue.
// Must use Ctb() wrapper like NewTValueString/NewTValueTable for collectable types.
func NewTValueThread(data unsafe.Pointer) TValue {
	return &tvalueStruct{Value: &valueStruct{Variant: ValueGC, Data_: data}, Tt: uint8(Ctb(int(LUA_VTHREAD)))}
}

// NewDoStringMarker creates a special marker for DoString closures.
func NewDoStringMarker(id int) TValue {
	return &tvalueStruct{Value: &valueStruct{Variant: ValuePointer, Data_: id}, Tt: uint8(LUA_VLIGHTUSERDATA)}
}

// tvalueStruct and valueStruct are the concrete implementations referenced by
// the public TValue and Value interfaces in api. Defined here to avoid import cycles.
type tvalueStruct struct {
	Value *valueStruct
	Tt    uint8
}
type valueStruct struct {
	Variant ValueVariant
	Data_   interface{}
}

func (v *valueStruct) GetGC() *GCObject {
	if v.Variant != ValueGC {
		panic("not GC")
	}
	return v.Data_.(*GCObject)
}
func (v *valueStruct) GetPointer() unsafe.Pointer { return v.Data_.(unsafe.Pointer) }
func (v *valueStruct) GetCFunction() unsafe.Pointer { return v.Data_.(unsafe.Pointer) }
func (v *valueStruct) GetInteger() LuaInteger { return v.Data_.(LuaInteger) }
func (v *valueStruct) GetFloat() LuaNumber { return v.Data_.(LuaNumber) }

func (t *tvalueStruct) GetTag() int               { return int(t.Tt) }
func (t *tvalueStruct) GetBaseType() int          { return int(t.Tt) & 0x0F }
func (t *tvalueStruct) GetValue() interface{}      { return t.Value.Data_ }
func (t *tvalueStruct) GetGC() *GCObject           { return t.Value.GetGC() }
func (t *tvalueStruct) GetInteger() LuaInteger     { return t.Value.GetInteger() }
func (t *tvalueStruct) GetFloat() LuaNumber        { return t.Value.GetFloat() }
func (t *tvalueStruct) GetPointer() unsafe.Pointer { return t.Value.GetPointer() }
func (t *tvalueStruct) IsNil() bool                { return t.GetBaseType() == LUA_TNIL }
func (t *tvalueStruct) IsBoolean() bool            { return t.GetBaseType() == LUA_TBOOLEAN }
func (t *tvalueStruct) IsNumber() bool             { return t.GetBaseType() == LUA_TNUMBER }
func (t *tvalueStruct) IsInteger() bool            { return int(t.Tt) == LUA_VNUMINT }
func (t *tvalueStruct) IsFloat() bool              { return int(t.Tt) == LUA_VNUMFLT }
func (t *tvalueStruct) IsString() bool             { return t.GetBaseType() == LUA_TSTRING }
func (t *tvalueStruct) IsTable() bool              { return int(t.Tt) == Ctb(int(LUA_VTABLE)) }
func (t *tvalueStruct) IsFunction() bool           { return t.GetBaseType() == LUA_TFUNCTION }
func (t *tvalueStruct) IsThread() bool            { return int(t.Tt) == LUA_VTHREAD }
func (t *tvalueStruct) IsLightUserData() bool      { return int(t.Tt) == LUA_VLIGHTUSERDATA }
func (t *tvalueStruct) IsUserData() bool           { return int(t.Tt) == Ctb(int(LUA_VUSERDATA)) }
func (t *tvalueStruct) IsCollectable() bool       { return int(t.Tt)&(1<<6) != 0 }
func (t *tvalueStruct) IsTrue() bool              { return int(t.Tt) == LUA_VTRUE }
func (t *tvalueStruct) IsFalse() bool             { return int(t.Tt) == LUA_VFALSE }
func (t *tvalueStruct) IsLClosure() bool          { return int(t.Tt) == Ctb(int(LUA_VLCL)) }
func (t *tvalueStruct) IsCClosure() bool         { return int(t.Tt) == Ctb(int(LUA_VCCL)) }
func (t *tvalueStruct) IsLightCFunction() bool    { return int(t.Tt) == LUA_VLCF }
func (t *tvalueStruct) IsClosure() bool            { return t.IsLClosure() || t.IsCClosure() || t.IsLightCFunction() }
func (t *tvalueStruct) IsProto() bool              { return int(t.Tt) == Ctb(int(LUA_VPROTO)) }
func (t *tvalueStruct) IsUpval() bool             { return int(t.Tt) == Ctb(int(LUA_VUPVAL)) }
func (t *tvalueStruct) IsShortString() bool       { return int(t.Tt) == Ctb(int(LUA_VSHRSTR)) }
func (t *tvalueStruct) IsLongString() bool        { return int(t.Tt) == Ctb(int(LUA_VLNGSTR)) }
func (t *tvalueStruct) IsEmpty() bool             { return t.GetBaseType() == LUA_TNIL && t.Value == nil }


// =============================================================================
// UpVal Factory Functions (implemented in types/internal/closure.go)
// =============================================================================

// upvalOpenEntry is the concrete open-list entry embedded in internal.UpVal.
type upvalOpenEntry struct {
	Fwd  UpVal
	Prev *UpVal
}

// NewOpenUpval creates a new open upvalue pointing to the given stack slot.
// The upvalue is inserted at the head of the global open-upvalue list via
// the prev pointer. After calling NewOpenUpval, the caller must link the
// upvalue into the list by setting uv.U.Open.Previous = &openUpval (the
// state's head pointer) and updating openUpval.U.Open.Previous.
//
// This function is exported so state/internal can create upvalues without
// importing types/internal (which would violate Go's internal package rule).
func NewOpenUpval(stackPtr unsafe.Pointer) UpVal {
	return &openUpvalImpl{stackPtr: stackPtr}
}

// NewClosedUpval creates a new closed upvalue holding the given value.
// The upvalue is NOT linked into any list (closed upvalues are GC-tracked).
func NewClosedUpval(v TValue) UpVal {
	return &closedUpvalImpl{value: v}
}

// openUpvalImpl is the concrete UpVal implementation for open upvalues.
// Defined here so state/internal can construct it without importing types/internal.
type openUpvalImpl struct {
	stackPtr unsafe.Pointer
	list     upvalOpenEntry
}

func (u *openUpvalImpl) IsOpen() bool   { return true }
func (u *openUpvalImpl) GetStackPtr() unsafe.Pointer { return u.stackPtr }
func (u *openUpvalImpl) GetValue() TValue {
	// Read through to the stack slot — caller must handle out-of-bounds.
	return *(*TValue)(u.stackPtr)
}
func (u *openUpvalImpl) SetValue(v TValue) {
	*(*TValue)(u.stackPtr) = v
}
func (u *openUpvalImpl) Open() UpValOpen { return &u.list }

// closedUpvalImpl is the concrete UpVal implementation for closed upvalues.
type closedUpvalImpl struct {
	value TValue
}

func (u *closedUpvalImpl) IsOpen() bool                  { return false }
func (u *closedUpvalImpl) GetStackPtr() unsafe.Pointer   { return nil }
func (u *closedUpvalImpl) GetValue() TValue              { return u.value }
func (u *closedUpvalImpl) SetValue(v TValue)              { u.value = v }
func (u *closedUpvalImpl) Open() UpValOpen               { return nil }

// Ensure both implementations satisfy UpVal and UpValOpen.
var _ UpVal = (*openUpvalImpl)(nil)
var _ UpVal = (*closedUpvalImpl)(nil)
var _ UpValOpen = (*upvalOpenEntry)(nil)

// upvalOpenEntry methods for the linked list.
func (e *upvalOpenEntry) Next() UpVal       { return e.Fwd }
func (e *upvalOpenEntry) Previous() *UpVal   { return e.Prev }
func (e *upvalOpenEntry) SetNextPtr(next UpVal)    { e.Fwd = next }
func (e *upvalOpenEntry) SetPreviousPtr(prev *UpVal){ e.Prev = prev }
