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

type UpVal interface{}

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
