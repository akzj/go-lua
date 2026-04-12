// Package api manages Lua string values with interning for short strings.
//
// Short strings (≤ MaxShortLen bytes) are interned in a global table.
// Two short strings with the same content share the same *LuaString pointer,
// enabling O(1) equality via pointer comparison.
//
// Long strings are not interned and compared by content.
//
// Reference: .analysis/07-runtime-infrastructure.md §4
package api

import objectapi "github.com/akzj/go-lua/internal/object/api"

// MaxShortLen is the maximum length for interned short strings (LUAI_MAXSHORTLEN = 40).
const MaxShortLen = 40

// ---------------------------------------------------------------------------
// StringTable — the global interning table
// ---------------------------------------------------------------------------

// StringTable interns short strings for pointer-equality lookups.
// It is owned by GlobalState and shared across all threads.
type StringTable struct {
	Buckets [][]*objectapi.LuaString // hash buckets (power-of-2 count)
	Count   int                      // number of interned strings
	Seed    uint32                   // hash seed (randomized per state)
}

// NewStringTable creates a string table with the given hash seed.
// Initial bucket count is 128 (MINSTRTABSIZE).
func NewStringTable(seed uint32) *StringTable {
	st := &StringTable{
		Buckets: make([][]*objectapi.LuaString, 128),
		Seed:    seed,
	}
	return st
}

// Intern returns the canonical *LuaString for the given Go string.
// If the string is short (≤ MaxShortLen), it is interned (deduplicated).
// If long, a new LuaString is created each time (not interned).
func (st *StringTable) Intern(s string) *objectapi.LuaString {
	// Implementation will be in luastring.go
	return nil
}

// InternBytes is like Intern but accepts a byte slice (avoids allocation
// when the caller already has bytes, e.g., from the lexer).
func (st *StringTable) InternBytes(b []byte) *objectapi.LuaString {
	return nil
}

// Count_ returns the number of interned short strings.
func (st *StringTable) Count_() int {
	return st.Count
}

// ---------------------------------------------------------------------------
// Hash function — faithful to C Lua's luaS_hash (lstring.c:53–58)
// ---------------------------------------------------------------------------

// Hash computes the Lua string hash for the given bytes with the given seed.
//
//	h = seed ^ len
//	for each byte b (reverse order):
//	    h ^= (h<<5) + (h>>2) + b
func Hash(data string, seed uint32) uint32 {
	h := seed ^ uint32(len(data))
	step := (len(data) >> 5) + 1
	for i := len(data); i >= step; i -= step {
		h ^= (h << 5) + (h >> 2) + uint32(data[i-1])
	}
	return h
}

// ---------------------------------------------------------------------------
// LuaString construction helpers
// ---------------------------------------------------------------------------

// NewShort creates an interned short string. Called by StringTable.Intern.
// The caller must ensure len(s) <= MaxShortLen.
func NewShort(s string, hash uint32) *objectapi.LuaString {
	return &objectapi.LuaString{
		Data:    s,
		Hash_:   hash,
		IsShort: true,
	}
}

// NewLong creates a non-interned long string.
// Hash is computed lazily on first use as a table key.
func NewLong(s string, seed uint32) *objectapi.LuaString {
	return &objectapi.LuaString{
		Data:    s,
		Hash_:   0, // computed lazily
		IsShort: false,
	}
}

// Equal compares two LuaStrings. For two short strings, this is pointer
// equality. For any other combination, it's content comparison.
func Equal(a, b *objectapi.LuaString) bool {
	if a == b {
		return true
	}
	if a.IsShort && b.IsShort {
		return false // interned: same content → same pointer
	}
	return a.Data == b.Data
}
