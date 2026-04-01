// Package api defines the string table interface for Lua VM.
// No implementation details - only interfaces and constructors.
//
// Constraint: must NOT import internal/ to avoid circular dependency.
// Use NewStringTable() factory which returns the pre-initialized default.
package api

// =============================================================================
// String Table Interface
// =============================================================================

// MaxShortStringLen is the maximum length for short (interned) strings.
// Must be >= reserved words (e.g., "function"=8, "__newindex"=10).
// Why not configurable? Lua requires these strings to be interned.
const MaxShortStringLen = 40

// String variants (must match types/api constants)
const (
	LUA_VSHRSTR = 4 | (0 << 4) // LUA_TSTRING | 0
	LUA_VLNGSTR = 4 | (1 << 4) // LUA_TSTRING | 1
)

// TStringImpl is the concrete implementation of TString.
// TStringImpl layout (matches lua-master/lobject.h TString):
// - CommonHeader (next, tt, marked) - for GC
// - extra: reserved word flag (short) or has-hash flag (long)
// - shrlen: length (short) or LSTR* (long): -1=reg, -2=fix, -3=mem
// - hash: computed hash value
// - hnext: linked list chain for hash table
// - data: string content (flexible array member)
type TStringImpl struct {
	// CommonHeader for GC
	Next   *TStringImpl
	TT     uint8 // type tag (LUA_VSHRSTR or LUA_VLNGSTR)
	Marked uint8 // GC marked bits

	Extra  uint8 // reserved word (short) or has-hash (long)
	Shrlen int8  // length (short) or LSTR* (long)

	Hash uint32 // hash value

	// Hash chain for string table
	Hnext *TStringImpl

	// String data (flexible array - must be last)
	Data []byte
}

// IsShort returns true if this is a short string (<=MaxShortStringLen).
// Why int8 comparison? Shrlen >= 0 for short, < 0 for long.
func (ts *TStringImpl) IsShort() bool {
	return ts.Shrlen >= 0
}

// Len returns the string length.
func (ts *TStringImpl) Len() int {
	if ts.IsShort() {
		return int(ts.Shrlen)
	}
	// Long string: stored in Data
	return len(ts.Data)
}

// HashValue returns the precomputed hash value.
func (ts *TStringImpl) HashValue() uint32 {
	return ts.Hash
}

// Contents returns the string content.
func (ts *TStringImpl) Contents() string {
	return string(ts.Data)
}

// IsReservedWord returns true if this is a reserved word.
// Only valid for short strings.
func (ts *TStringImpl) IsReservedWord() bool {
	return ts.IsShort() && ts.Extra > 0
}

// StringTable manages Lua string interning.
// Short strings (<= MaxShortStringLen) are always interned.
// Long strings are created on demand without interning.
//
// Invariants:
// - NewString returns the same *TStringImpl for identical input (interning)
// - Interned and GetString return nil for non-existent strings
type StringTable interface {
	// NewString creates or retrieves an interned string.
	// For short strings (<=MaxShortStringLen): always interned
	// For long strings: always new (not interned)
	NewString(s string) *TStringImpl

	// Interned checks if a string exists in the string table.
	// Returns true only for short strings.
	Interned(s string) bool

	// GetString retrieves an existing interned string.
	// Returns nil if not found.
	GetString(s string) *TStringImpl
}

// =============================================================================
// Factory Function (avoids circular dependency)
// =============================================================================

// DefaultStringTable is the global string table instance.
// Initialized by internal.init() before any user code runs.
var DefaultStringTable StringTable

// NewStringTable returns the default string table instance.
// This factory pattern avoids importing internal/ from api/.
func NewStringTable() StringTable {
	return DefaultStringTable
}
