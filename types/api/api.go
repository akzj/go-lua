// Package api defines Lua 5.5.1 core type interfaces.
// NO dependencies - pure interface definitions.
package api

import "unsafe"

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
