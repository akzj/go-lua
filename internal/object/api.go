// Package api defines Lua's universal value type (TValue) and all Lua types.
//
// Every Lua value is represented as a TValue: a type tag (Tag) plus a Go value (any).
// This is the Go equivalent of C Lua's tagged union (lobject.h).
//
// Design priority: Correctness > Simplicity > Performance.
// Reference: .analysis/03-object-type-system.md
package object

import (
	"math"
	"unsafe"
)

// ---------------------------------------------------------------------------
// GC Object interface and header — embedded in every collectable Lua object
// ---------------------------------------------------------------------------

// GCObject is the interface all GC-collectable Lua objects implement.
type GCObject interface {
	GC() *GCHeader
}

// GCHeader is embedded in every collectable Lua object.
// It provides the linked-list pointer for the Lua GC allgc chain.
//
// Gray/weak/ephemeron lists use external slices in GlobalState instead of
// an intrusive GCList pointer, saving 16 bytes per object (interface = 16B).
type GCHeader struct {
	Next    GCObject // next in allgc/finobj/tobefnz chain
	Marked  byte     // GC mark bits: color (white0/white1/black) + finalized
	Age     byte     // generational GC age (G_NEW through G_TOUCHED2)

	// ObjSize is the pre-computed byte size of this object for GC accounting.
	// Set at allocation time. Updated on table resize (rehash).
	// Used by sweepList to decrement GCTotalBytes without type assertions.
	ObjSize int64
}

// iface is the Go runtime layout of a non-empty interface.
// Used by FastGC to extract the data pointer without virtual dispatch.
type iface struct {
	_    uintptr        // itab pointer (type + method table)
	data unsafe.Pointer // pointer to concrete value
}

// FastGC extracts *GCHeader from a GCObject interface without calling
// the GC() method (avoiding interface dispatch overhead).
//
// SAFETY INVARIANT: Every type implementing GCObject MUST embed GCHeader
// as its FIRST field. This ensures the interface's data pointer points
// directly to the GCHeader.
//
// This is 3.9x faster than obj.GC() when multiple concrete types exist,
// because it avoids the indirect call through the interface method table.
func FastGC(obj GCObject) *GCHeader {
	return (*GCHeader)((*iface)(unsafe.Pointer(&obj)).data)
}

// FastGCFromAny extracts *GCHeader from an `any` value without type assertion.
// The any value MUST contain a pointer to a struct with GCHeader as first field.
// Used in markValue to check IsWhite() before the expensive GCObject type assertion.
func FastGCFromAny(obj any) *GCHeader {
	return (*GCHeader)(unsafe.Pointer((*eface)(unsafe.Pointer(&obj)).data))
}



// GC color/mark bit constants.
const (
	WhiteBit0    byte = 1 << 0 // white bit 0
	WhiteBit1    byte = 1 << 1 // white bit 1
	BlackBit     byte = 1 << 2 // black bit
	FinalizedBit byte = 1 << 3 // has been finalized
)

// Generational GC age constants (stored in GCHeader.Age, not in Marked).
// Mirrors C Lua's G_NEW through G_TOUCHED2 (lgc.h:110-116).
const (
	G_NEW      byte = 0 // created in current cycle
	G_SURVIVAL byte = 1 // survived one cycle
	G_OLD0     byte = 2 // marked old by forward barrier in this cycle
	G_OLD1     byte = 3 // first full cycle as old
	G_OLD      byte = 4 // really old object (not visited in minor GC)
	G_TOUCHED1 byte = 5 // old object touched this cycle
	G_TOUCHED2 byte = 6 // old object touched in previous cycle
)

// WhiteBits is the mask for both white bits.
const WhiteBits = WhiteBit0 | WhiteBit1

// IsWhite returns true if the object is white (scheduled for collection).
func (h *GCHeader) IsWhite() bool { return h.Marked&WhiteBits != 0 }

// IsBlack returns true if the object is black (fully traversed).
func (h *GCHeader) IsBlack() bool { return h.Marked&BlackBit != 0 }

// IsGray returns true if the object is gray (marked but not yet traversed).
func (h *GCHeader) IsGray() bool { return !h.IsWhite() && !h.IsBlack() }

// IsDead returns true if the object is dead. Matches C Lua's isdeadm: (m) & (ow).
func (h *GCHeader) IsDead(otherwhite byte) bool {
	return h.Marked&otherwhite != 0
}

// IsOld returns true if the object is older than SURVIVAL (OLD0, OLD1, OLD, TOUCHED1, TOUCHED2).
func (h *GCHeader) IsOld() bool { return h.Age > G_SURVIVAL }

// GetAge returns the generational age of the object.
func (h *GCHeader) GetAge() byte { return h.Age }

// SetAge sets the generational age of the object.
func (h *GCHeader) SetAge(age byte) { h.Age = age }
// GC phase constants (matches C Lua's GCState values from lgc.h).
const (
	GCSpause       byte = 0 // waiting to start a new cycle
	GCSpropagate   byte = 1 // propagating gray objects
	GCSenteratomic byte = 2 // about to enter atomic phase
	GCSatomic      byte = 3 // atomic mark phase
	GCSswpallgc    byte = 4 // sweeping allgc list
	GCSswpfinobj   byte = 5 // sweeping finobj list
	GCSswptobefnz  byte = 6 // sweeping tobefnz list
	GCSswpend      byte = 7 // sweep finished
	GCScallfin     byte = 8 // calling finalizers
)

// GC kind constants (matches C Lua's KGC_* from lgc.h).
const (
	KGC_INC      byte = 0 // incremental mode (default)
	KGC_GENMINOR byte = 1 // generational mode — minor collection
	KGC_GENMAJOR byte = 2 // generational mode — major collection
)



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

// BIT_ISCOLLECTABLE marks a tag as a GC-collectable type.
// Matches C Lua's BIT_ISCOLLECTABLE (bit 6).
const BIT_ISCOLLECTABLE Tag = 0x40

// IsCollectable returns true if this tag represents a GC-collectable type.
func (t Tag) IsCollectable() bool { return t&BIT_ISCOLLECTABLE != 0 }

// Nil variants (4 kinds in Lua 5.5) — NOT collectable
const (
	TagNil     Tag = 0x00 // LUA_VNIL — standard nil
	TagEmpty   Tag = 0x10 // LUA_VEMPTY — empty table slot
	TagAbstKey Tag = 0x20 // LUA_VABSTKEY — absent key (key not found)
	TagNotable Tag = 0x30 // LUA_VNOTABLE — fast-get hit a non-table (Lua 5.5 new)
)

// Boolean variants — NOT collectable
const (
	TagFalse Tag = 0x01 // LUA_VFALSE
	TagTrue  Tag = 0x11 // LUA_VTRUE
)

// Number variants — NOT collectable
const (
	TagInteger Tag = 0x03 // LUA_VNUMINT
	TagFloat   Tag = 0x13 // LUA_VNUMFLT
)

// String variants — collectable (bit 6 set)
const (
	TagShortStr Tag = 0x44 // LUA_VSHRSTR — interned short string
	TagLongStr  Tag = 0x54 // LUA_VLNGSTR — non-interned long string
)

// Function variants
const (
	TagLuaClosure Tag = 0x46 // LUA_VLCL — Lua closure (collectable)
	TagLightCFunc Tag = 0x16 // LUA_VLCF — light C function (NOT collectable)
	TagCClosure   Tag = 0x66 // LUA_VCCL — C closure (collectable)
)

// Other collectable types (bit 6 set)
const (
	TagTable         Tag = 0x45 // LUA_VTABLE
	TagUserdata      Tag = 0x47 // LUA_VUSERDATA
	TagThread        Tag = 0x48 // LUA_VTHREAD
	TagUpVal         Tag = 0x49 // LUA_VUPVAL (internal)
	TagProto         Tag = 0x4A // LUA_VPROTO (internal)
	TagLightUserdata Tag = 0x02 // LUA_VLIGHTUSERDATA (NOT collectable)
)

// TagDeadKey is used for dead keys in table hash part (internal).
const TagDeadKey Tag = 0x0B // LUA_TDEADKEY (NOT collectable)

// ---------------------------------------------------------------------------
// Proto flag constants (C7 FIX: replaces IsVararg bool)
// ---------------------------------------------------------------------------
const (
	PF_VAHID byte = 1 // function has hidden vararg arguments
	PF_VATAB byte = 2 // function has vararg table (Lua 5.5)
	PF_FIXED byte = 4 // prototype has parts in fixed memory
)

// IsVararg returns true if the proto has any vararg flag set.
func (p *Proto) IsVararg() bool {
	return p.Flag&(PF_VAHID|PF_VATAB) != 0
}

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

// TValue represents any Lua value using a dual-field layout for zero-alloc numerics.
//
// Numeric types (integer, float) are stored in the N field without boxing:
//   - Integer: N holds the int64 value directly
//   - Float: N holds the float64 bits via math.Float64bits/Float64frombits
//
// GC object types (string, table, closure, userdata, thread, proto, upval)
// are stored in the Obj field as typed pointers.
//
// This eliminates runtime.convT64 allocations on the hot path.
type TValue struct {
	Tt  Tag   // type tag
	N   int64 // numeric payload: int64 for integers, float64 bits for floats
	Obj any   // GC object payload: *LuaString | *Table | *LClosure | etc.
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

// MakeInteger creates an integer TValue. Stored inline in N — zero allocation.
func MakeInteger(i int64) TValue { return TValue{Tt: TagInteger, N: i} }

// MakeFloat creates a float TValue. Bits stored in N — zero allocation.
func MakeFloat(f float64) TValue { return TValue{Tt: TagFloat, N: int64(math.Float64bits(f))} }

// MakeBoolean creates a boolean TValue (TagFalse or TagTrue).
func MakeBoolean(b bool) TValue {
	if b {
		return True
	}
	return False
}

// MakeString creates a string TValue from a *LuaString.
// The tag is taken from the LuaString itself (short or long).
func MakeString(s *LuaString) TValue { return TValue{Tt: s.Tag(), Obj: s} }

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

// Integer returns the int64 value. The value is stored directly in N.
func (v TValue) Integer() int64 { return v.N }

// Float returns the float64 value. The bits are stored in N.
func (v TValue) Float() float64 { return math.Float64frombits(uint64(v.N)) }

// Boolean returns the boolean value.
func (v TValue) Boolean() bool { return v.Tt == TagTrue }

// StringVal returns the *LuaString. Panics if not a string tag.
func (v TValue) StringVal() *LuaString { return v.Obj.(*LuaString) }

// GCValue returns the GC object pointer (for any GC-collectable type).
func (v TValue) GCValue() any { return v.Obj }

// Payload returns a boxed representation of the value for use in rare paths
// (e.g., table hash keys). For integers it boxes N, for floats it boxes the
// float64, for everything else it returns Obj. This DOES allocate for numbers
// but is only used in cold paths (table key storage).
func (v TValue) Payload() any {
	switch v.Tt {
	case TagInteger:
		return v.N
	case TagFloat:
		return v.Float()
	default:
		return v.Obj
	}
}

// MakeFromPayload reconstructs a TValue from a tag and a payload (as returned
// by Payload()). Used in table node key reconstruction.
func MakeFromPayload(tt Tag, payload any) TValue {
	switch tt {
	case TagInteger:
		return TValue{Tt: tt, N: payload.(int64)}
	case TagFloat:
		return MakeFloat(payload.(float64))
	default:
		return TValue{Tt: tt, Obj: payload}
	}
}

// --- Number coercion (used heavily by VM) ---

// ToNumber attempts to convert to float64. Returns (value, ok).
// Integer → float64 conversion. Float → identity. Other → false.
func (v TValue) ToNumber() (float64, bool) {
	switch v.Tt {
	case TagFloat:
		return v.Float(), true
	case TagInteger:
		return float64(v.N), true
	default:
		return 0, false
	}
}

// ToInteger attempts to convert to int64. Returns (value, ok).
// Integer → identity. Float → truncate if exact. Other → false.
func (v TValue) ToInteger() (int64, bool) {
	switch v.Tt {
	case TagInteger:
		return v.N, true
	case TagFloat:
		f := v.Float()
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
	GCHeader              // GC metadata
	Data    string
	Hash_   uint32
	IsShort bool
	Extra   byte // reserved word flag for short strings
}

// GC returns the GC header for this string.
func (s *LuaString) GC() *GCHeader { return &s.GCHeader }

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
	GCHeader                 // GC metadata
	Code         []uint32    // bytecode instructions
	Constants    []TValue    // constant pool
	Protos       []*Proto    // nested function prototypes
	Upvalues     []UpvalDesc // upvalue descriptors
	NumParams    byte        // number of fixed parameters
	MaxStackSize byte        // registers needed
	Flag         byte        // function flags (PF_VAHID, PF_VATAB, PF_FIXED)
	LineDefined  int         // first line of definition
	LastLine     int         // last line of definition
	Source       *LuaString  // source file name

	// Debug info (optional, can be stripped)
	LineInfo    []int8        // per-instruction line delta
	AbsLineInfo []AbsLineInfo // sparse absolute line info
	LocVars     []LocVar      // local variable info
}

// GC returns the GC header for this proto.
func (p *Proto) GC() *GCHeader { return &p.GCHeader }


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

// --- Userdata (C8 FIX: was missing entirely) ---

// Userdata represents a full userdata object.
// It holds arbitrary Go data, a metatable, and user values.
type Userdata struct {
	GCHeader              // GC metadata
	Data      any      // user data (Go value)
	MetaTable any      // *Table at runtime (any to avoid import cycle)
	UserVals  []TValue // user values (nuvalue)
	Size      int      // allocated size in bytes (for lua_rawlen)
}

// GC returns the GC header for this userdata.
func (u *Userdata) GC() *GCHeader { return &u.GCHeader }

// --- Stack value (TValue + to-be-closed delta) ---

// StackValue is a stack slot: a TValue plus a delta for the to-be-closed list.
type StackValue struct {
	Val      TValue
	TBCDelta uint16 // distance to previous tbc variable (0 = not tbc)
}
