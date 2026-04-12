// Package api defines Lua's universal value type (TValue) and all Lua types.
//
// Every Lua value is represented as a TValue: a type tag (Tag) plus a Go value (any).
// This is the Go equivalent of C Lua's tagged union (lobject.h).
//
// Design priority: Correctness > Simplicity > Performance.
// Reference: .analysis/03-object-type-system.md
package api

import "math"

// ---------------------------------------------------------------------------
// Type tags — faithful to C Lua's encoding (lobject.h:37–42)
// ---------------------------------------------------------------------------

// Type is the base Lua type (bits 0–3 of the tag byte).
type Type byte

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
	TypeNone          Type = 0xFF // LUA_TNONE — invalid/absent (API only)
)

// NumTypes is the count of public Lua types (LUA_NUMTYPES = 9).
const NumTypes = 9

// TypeNames maps base types to their display names.
var TypeNames = [NumTypes]string{
	"nil", "boolean", "userdata", "number",
	"string", "table", "function", "userdata", "thread",
}

// Tag is the full type tag with variant bits (bits 0–5 of the tag byte).
// Bit layout: [7:unused][6:collectable][5:variant1][4:variant0][3:0:base_type]
type Tag byte

// Nil variants (4 kinds in Lua 5.5)
const (
	TagNil     Tag = 0x00 // LUA_VNIL — standard nil
	TagEmpty   Tag = 0x10 // LUA_VEMPTY — empty table slot
	TagAbstKey Tag = 0x20 // LUA_VABSTKEY — absent key (key not found)
	TagNotable Tag = 0x30 // LUA_VNOTABLE — fast-get hit a non-table (Lua 5.5 new)
)

// Boolean variants
const (
	TagFalse Tag = 0x01 // LUA_VFALSE
	TagTrue  Tag = 0x11 // LUA_VTRUE
)

// Number variants
const (
	TagInteger Tag = 0x03 // LUA_VNUMINT
	TagFloat   Tag = 0x13 // LUA_VNUMFLT
)

// String variants
const (
	TagShortStr Tag = 0x04 // LUA_VSHRSTR — interned short string
	TagLongStr  Tag = 0x14 // LUA_VLNGSTR — non-interned long string
)

// Function variants
const (
	TagLuaClosure Tag = 0x06 // LUA_VLCL — Lua closure
	TagLightCFunc Tag = 0x16 // LUA_VLCF — light C function (no upvalues)
	TagCClosure   Tag = 0x26 // LUA_VCCL — C closure (with upvalues)
)

// Other collectable types
const (
	TagTable         Tag = 0x05 // LUA_VTABLE
	TagUserdata      Tag = 0x07 // LUA_VUSERDATA
	TagThread        Tag = 0x08 // LUA_VTHREAD
	TagUpVal         Tag = 0x09 // LUA_VUPVAL (internal)
	TagProto         Tag = 0x0A // LUA_VPROTO (internal)
	TagLightUserdata Tag = 0x02 // LUA_VLIGHTUSERDATA
)

// TagDeadKey is used for dead keys in table hash part (internal).
const TagDeadKey Tag = 0x0B // LUA_TDEADKEY

// BaseType extracts the base type (bits 0–3) from a tag.
func (t Tag) BaseType() Type { return Type(t & 0x0F) }

// Variant extracts the variant bits (bits 4–5) from a tag.
func (t Tag) Variant() byte { return byte(t>>4) & 0x03 }

// IsNil returns true for any nil variant (nil, empty, abstkey, notable).
func (t Tag) IsNil() bool { return t.BaseType() == TypeNil }

// IsStrictNil returns true only for standard nil (not empty/abstkey/notable).
func (t Tag) IsStrictNil() bool { return t == TagNil }

// IsFalsy returns true for nil (any variant) and false.
func (t Tag) IsFalsy() bool { return t.IsNil() || t == TagFalse }

// ---------------------------------------------------------------------------
// TValue — the universal Lua value container
// ---------------------------------------------------------------------------

// TValue represents any Lua value: a tag identifying the type, plus the value.
//
// For non-GC types (nil, boolean, integer, float, light C function, light userdata),
// the value is stored inline.
// For GC types (string, table, closure, userdata, thread, proto, upval),
// the value is a pointer stored in the 'any' field.
//
// This is intentionally the simplest correct representation.
type TValue struct {
	Tt  Tag // type tag
	Val any // nil | int64 | float64 | bool | *LuaString | *Table | ...
}

// Nil is the singleton nil TValue.
var Nil = TValue{Tt: TagNil}

// Empty is the singleton empty-slot TValue.
var Empty = TValue{Tt: TagEmpty}

// AbsentKey is the singleton absent-key TValue.
var AbsentKey = TValue{Tt: TagAbstKey}

// False is the singleton false TValue.
var False = TValue{Tt: TagFalse}

// True is the singleton true TValue.
var True = TValue{Tt: TagTrue}

// --- Constructors ---

// MakeInteger creates an integer TValue.
func MakeInteger(i int64) TValue { return TValue{Tt: TagInteger, Val: i} }

// MakeFloat creates a float TValue.
func MakeFloat(f float64) TValue { return TValue{Tt: TagFloat, Val: f} }

// MakeBoolean creates a boolean TValue (TagFalse or TagTrue).
func MakeBoolean(b bool) TValue {
	if b {
		return True
	}
	return False
}

// MakeString creates a string TValue from a *LuaString.
// The tag is taken from the LuaString itself (short or long).
func MakeString(s *LuaString) TValue { return TValue{Tt: s.Tag(), Val: s} }

// --- Tag accessors ---

// Tag returns the full type tag.
func (v TValue) Tag() Tag { return v.Tt }

// Type returns the base type.
func (v TValue) Type() Type { return v.Tt.BaseType() }

// IsNil returns true for any nil variant.
func (v TValue) IsNil() bool { return v.Tt.IsNil() }

// IsFalsy returns true for nil or false.
func (v TValue) IsFalsy() bool { return v.Tt.IsFalsy() }

// IsInteger returns true if this is an integer number.
func (v TValue) IsInteger() bool { return v.Tt == TagInteger }

// IsFloat returns true if this is a float number.
func (v TValue) IsFloat() bool { return v.Tt == TagFloat }

// IsNumber returns true if this is any number (integer or float).
func (v TValue) IsNumber() bool { return v.Tt.BaseType() == TypeNumber }

// IsString returns true if this is any string (short or long).
func (v TValue) IsString() bool { return v.Tt.BaseType() == TypeString }

// IsFunction returns true if this is any function variant.
func (v TValue) IsFunction() bool { return v.Tt.BaseType() == TypeFunction }

// IsTable returns true if this is a table.
func (v TValue) IsTable() bool { return v.Tt == TagTable }

// --- Value accessors (panic on type mismatch) ---

// Integer returns the int64 value. Panics if not TagInteger.
func (v TValue) Integer() int64 { return v.Val.(int64) }

// Float returns the float64 value. Panics if not TagFloat.
func (v TValue) Float() float64 { return v.Val.(float64) }

// Boolean returns the boolean value.
func (v TValue) Boolean() bool { return v.Tt == TagTrue }

// StringVal returns the *LuaString. Panics if not a string tag.
func (v TValue) StringVal() *LuaString { return v.Val.(*LuaString) }

// --- Number coercion (used heavily by VM) ---

// ToNumber attempts to convert to float64. Returns (value, ok).
// Integer → float64 conversion. Float → identity. Other → false.
func (v TValue) ToNumber() (float64, bool) {
	switch v.Tt {
	case TagFloat:
		return v.Val.(float64), true
	case TagInteger:
		return float64(v.Val.(int64)), true
	default:
		return 0, false
	}
}

// ToInteger attempts to convert to int64. Returns (value, ok).
// Integer → identity. Float → truncate if exact. Other → false.
func (v TValue) ToInteger() (int64, bool) {
	switch v.Tt {
	case TagInteger:
		return v.Val.(int64), true
	case TagFloat:
		f := v.Val.(float64)
		if i := int64(f); float64(i) == f && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return i, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// --- Forward-declared types (implemented in other packages) ---
// These are placeholder types that will be defined in their respective packages.
// The object package declares them so TValue can reference them without import cycles.

// LuaString wraps a Go string with interning support and hash caching.
type LuaString struct {
	Data    string
	Hash_   uint32
	IsShort bool
	Extra   byte // reserved word flag for short strings
}

// Tag returns TagShortStr or TagLongStr based on the string kind.
func (s *LuaString) Tag() Tag {
	if s.IsShort {
		return TagShortStr
	}
	return TagLongStr
}

// String returns the underlying Go string.
func (s *LuaString) String() string { return s.Data }

// Hash returns the cached hash value.
func (s *LuaString) Hash() uint32 { return s.Hash_ }

// Len returns the string length in bytes.
func (s *LuaString) Len() int { return len(s.Data) }

// --- Proto (function prototype — compiled bytecode) ---

// Proto represents a compiled Lua function (the bytecode + metadata).
// This is the Go equivalent of C Lua's Proto struct (lobject.h:492–515).
type Proto struct {
	Code         []uint32    // bytecode instructions
	Constants    []TValue    // constant pool
	Protos       []*Proto    // nested function prototypes
	Upvalues     []UpvalDesc // upvalue descriptors
	NumParams    byte        // number of fixed parameters
	MaxStackSize byte        // registers needed
	IsVararg     bool        // has ... parameter
	LineDefined  int         // first line of definition
	LastLine     int         // last line of definition
	Source       *LuaString  // source file name

	// Debug info (optional, can be stripped)
	LineInfo    []int8        // per-instruction line delta
	AbsLineInfo []AbsLineInfo // sparse absolute line info
	LocVars     []LocVar      // local variable info
}

// UpvalDesc describes how an upvalue is captured.
type UpvalDesc struct {
	Name    *LuaString // upvalue name (debug)
	InStack bool       // true = in enclosing function's stack (register)
	Idx     byte       // index in stack or in outer function's upvalue list
	Kind    byte       // kind of corresponding variable
}

// AbsLineInfo maps a PC to an absolute source line number.
type AbsLineInfo struct {
	PC   int
	Line int
}

// LocVar describes a local variable's lifetime.
type LocVar struct {
	Name    *LuaString
	StartPC int // first active instruction
	EndPC   int // first dead instruction
}

// --- Stack value (TValue + to-be-closed delta) ---

// StackValue is a stack slot: a TValue plus a delta for the to-be-closed list.
type StackValue struct {
	Val      TValue
	TBCDelta uint16 // distance to previous tbc variable (0 = not tbc)
}
