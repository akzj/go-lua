// Package api manages Lua string values with interning for short strings.
//
// Short strings (≤ MaxShortLen bytes) are interned in a global table.
// Two short strings with the same content share the same *LuaString pointer,
// enabling O(1) equality via pointer comparison.
//
// Long strings are not interned and compared by content.
//
// The string table uses strong pointers. Dead interned strings are removed
// during GC sweep via RemoveString(), matching C Lua's approach where the
// sweep phase removes dead strings from the string table.
//
// The string table caps its bucket array at maxStrTabSize to prevent OOM
// from unbounded growth.
//
// Reference: .analysis/07-runtime-infrastructure.md §4
// C source: lua-master/lstring.c
package luastring

import (
	"github.com/akzj/go-lua/internal/object"
)

// MaxShortLen is the maximum length for interned short strings (LUAI_MAXSHORTLEN = 40).
const MaxShortLen = 40

// minStrTabSize is the initial/minimum bucket count (MINSTRTABSIZE = 128).
const minStrTabSize = 128

// maxStrTabSize caps the bucket array to prevent OOM on resize.
// 2^18 = 262,144 buckets. Each bucket is a slice header (24 bytes),
// so max bucket array is ~6MB. With load factor 1, this means at most
// ~262K interned strings before we stop growing the bucket array.
// Additional strings still get interned but with longer bucket chains.
const maxStrTabSize = 1 << 18

// ---------------------------------------------------------------------------
// Hash function — faithful to C Lua's luaS_hash (lstring.c:53–58)
//
// The C code iterates ALL bytes (no sampling/stepping for short strings):
//
//	static unsigned luaS_hash (const char *str, size_t l, unsigned seed) {
//	    unsigned int h = seed ^ cast_uint(l);
//	    for (; l > 0; l--)
//	        h ^= ((h<<5) + (h>>2) + cast_byte(str[l - 1]));
//	    return h;
//	}
// ---------------------------------------------------------------------------

// Hash computes the Lua string hash for the given bytes with the given seed.
// This matches C's luaS_hash exactly — iterates ALL bytes from end to start.
func Hash(data string, seed uint32) uint32 {
	l := len(data)
	h := seed ^ uint32(l)
	for l > 0 {
		l--
		h ^= (h << 5) + (h >> 2) + uint32(data[l])
	}
	return h
}

// hashBytes is like Hash but for a byte slice.
func hashBytes(data []byte, seed uint32) uint32 {
	l := len(data)
	h := seed ^ uint32(l)
	for l > 0 {
		l--
		h ^= (h << 5) + (h >> 2) + uint32(data[l])
	}
	return h
}

// ---------------------------------------------------------------------------
// StringTable — the global interning table
// ---------------------------------------------------------------------------

// stringEntry holds a strong reference to an interned LuaString.
// Dead strings are removed by RemoveString() during GC sweep.
type stringEntry struct {
	str *object.LuaString
}

// StringTable interns short strings for pointer-equality lookups.
// It is owned by GlobalState and shared across all threads.
type StringTable struct {
	buckets  [][]stringEntry       // hash buckets (power-of-2 count)
	count    int                   // number of interned strings
	seed     uint32                // hash seed (randomized per state)
	OnCreate func(object.GCObject) // V5: called when a new string is created (for GC linking)
}

// NewStringTable creates a string table with the given hash seed.
// Initial bucket count is 128 (MINSTRTABSIZE).
func NewStringTable(seed uint32) *StringTable {
	return &StringTable{
		buckets: make([][]stringEntry, minStrTabSize),
		seed:    seed,
	}
}

// Intern returns the canonical *LuaString for the given Go string.
// If the string is short (≤ MaxShortLen), it is interned (deduplicated).
// If long, a new LuaString is created each time (not interned).
func (st *StringTable) Intern(s string) *object.LuaString {
	if len(s) > MaxShortLen {
		ts := newLong(s)
		if st.OnCreate != nil {
			st.OnCreate(ts) // V5: register in allgc chain
		}
		return ts
	}
	return st.internShort(s, Hash(s, st.seed))
}

// InternBytes is like Intern but accepts a byte slice.
func (st *StringTable) InternBytes(b []byte) *object.LuaString {
	if len(b) > MaxShortLen {
		ts := newLong(string(b))
		if st.OnCreate != nil {
			st.OnCreate(ts) // V5: register in allgc chain
		}
		return ts
	}
	h := hashBytes(b, st.seed)
	// Convert to string for lookup/storage. For short strings (≤40 bytes),
	// this allocation is amortized by interning (only once per unique string).
	s := string(b)
	return st.internShort(s, h)
}

// Count returns the number of interned short strings.
func (st *StringTable) Count() int {
	return st.count
}

// Seed returns the hash seed.
func (st *StringTable) Seed() uint32 {
	return st.seed
}

// internShort looks up a short string in the table. If found, returns the
// existing *LuaString (pointer equality). If not found, creates a new one,
// inserts it, and returns it. May trigger resize.
//
// Uses strong pointers — dead strings are removed by RemoveString() during
// GC sweep, matching C Lua's approach.
func (st *StringTable) internShort(s string, h uint32) *object.LuaString {
	idx := h & uint32(len(st.buckets)-1) // lmod for power-of-2

	// Search existing bucket
	bucket := st.buckets[idx]
	for i := 0; i < len(bucket); i++ {
		p := bucket[i].str
		if p.Data == s { // Go string comparison (content)
			return p
		}
	}

	// Not found — create new
	ts := &object.LuaString{
		Data:    s,
		Hash_:   h,
		IsShort: true,
	}
	if st.OnCreate != nil {
		st.OnCreate(ts) // V5: register in allgc chain
	}

	// Insert into bucket
	st.buckets[idx] = append(st.buckets[idx], stringEntry{str: ts})
	st.count++

	// Resize if count exceeds bucket count (load factor > 1),
	// but only up to the cap to prevent OOM.
	if st.count > len(st.buckets) && len(st.buckets) < maxStrTabSize {
		st.resize(len(st.buckets) * 2)
	}

	return ts
}

// RemoveString removes a specific interned string from the string table.
// Called from GC sweep when a dead short string is found. This is the
// Go equivalent of C Lua's approach where sweep removes dead strings.
func (st *StringTable) RemoveString(ts *object.LuaString) {
	if !ts.IsShort {
		return // long strings are not in the table
	}
	idx := ts.Hash_ & uint32(len(st.buckets)-1)
	bucket := st.buckets[idx]
	for i := 0; i < len(bucket); i++ {
		if bucket[i].str == ts { // pointer equality — exact match
			// Swap-remove: replace with last element
			bucket[i] = bucket[len(bucket)-1]
			bucket[len(bucket)-1] = stringEntry{} // clear for GC
			st.buckets[idx] = bucket[:len(bucket)-1]
			st.count--
			return
		}
	}
}

// resize doubles (or changes) the bucket count and rehashes all entries.
func (st *StringTable) resize(newSize int) {
	if newSize < minStrTabSize {
		newSize = minStrTabSize
	}
	if newSize > maxStrTabSize {
		newSize = maxStrTabSize
	}
	newBuckets := make([][]stringEntry, newSize)
	mask := uint32(newSize - 1)
	for _, bucket := range st.buckets {
		for _, entry := range bucket {
			idx := entry.str.Hash_ & mask
			newBuckets[idx] = append(newBuckets[idx], entry)
		}
	}
	st.buckets = newBuckets
}

// SweepStrings optionally shrinks the bucket array if too sparse.
// With strong pointers, dead entries are removed individually by
// RemoveString() during GC sweep, so no dead-entry scanning is needed.
func (st *StringTable) SweepStrings() {
	// Shrink if too sparse (C Lua: nuse < size/4)
	size := len(st.buckets)
	if st.count < size/4 && size > minStrTabSize {
		newSize := size / 2
		if newSize < minStrTabSize {
			newSize = minStrTabSize
		}
		st.resize(newSize)
	}
}

// ---------------------------------------------------------------------------
// LuaString construction helpers
// ---------------------------------------------------------------------------

// newLong creates a non-interned long string.
// Hash is left at 0 — computed lazily when used as a table key.
func newLong(s string) *object.LuaString {
	return &object.LuaString{
		Data:    s,
		Hash_:   0,
		IsShort: false,
	}
}

// ---------------------------------------------------------------------------
// String equality
// ---------------------------------------------------------------------------

// equal compares two LuaStrings.
// For two short strings: pointer equality (both interned in the same table).
// For any other combination: content comparison.
func equal(a, b *object.LuaString) bool {
	if a == b {
		return true
	}
	// If both are short and interned in the same table, different pointers
	// means different content. But if they come from different tables
	// (shouldn't happen in practice), fall through to content compare.
	if a.IsShort && b.IsShort {
		return false // interned: same content → same pointer
	}
	// At least one is long — compare by content
	return a.Data == b.Data
}
