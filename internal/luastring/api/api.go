// Package api manages Lua string values with interning for short strings.
//
// Short strings (≤ MaxShortLen bytes) are interned in a global table.
// Two short strings with the same content share the same *LuaString pointer,
// enabling O(1) equality via pointer comparison.
//
// Long strings are not interned and compared by content.
//
// The string table caps its bucket array at maxStrTabSize to prevent OOM
// from unbounded growth. A Sweep method allows periodic cleanup of dead
// entries, matching C Lua's checkSizes behavior.
//
// Reference: .analysis/07-runtime-infrastructure.md §4
// C source: lua-master/lstring.c
package api

import objectapi "github.com/akzj/go-lua/internal/object/api"

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

// HashBytes is like Hash but for a byte slice.
func HashBytes(data []byte, seed uint32) uint32 {
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

// StringTable interns short strings for pointer-equality lookups.
// It is owned by GlobalState and shared across all threads.
type StringTable struct {
	buckets [][]*objectapi.LuaString // hash buckets (power-of-2 count)
	count   int                      // number of interned strings
	seed    uint32                   // hash seed (randomized per state)
}

// NewStringTable creates a string table with the given hash seed.
// Initial bucket count is 128 (MINSTRTABSIZE).
func NewStringTable(seed uint32) *StringTable {
	return &StringTable{
		buckets: make([][]*objectapi.LuaString, minStrTabSize),
		seed:    seed,
	}
}

// Intern returns the canonical *LuaString for the given Go string.
// If the string is short (≤ MaxShortLen), it is interned (deduplicated).
// If long, a new LuaString is created each time (not interned).
func (st *StringTable) Intern(s string) *objectapi.LuaString {
	if len(s) > MaxShortLen {
		return newLong(s)
	}
	return st.internShort(s, Hash(s, st.seed))
}

// InternBytes is like Intern but accepts a byte slice.
func (st *StringTable) InternBytes(b []byte) *objectapi.LuaString {
	if len(b) > MaxShortLen {
		return newLong(string(b))
	}
	h := HashBytes(b, st.seed)
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
func (st *StringTable) internShort(s string, h uint32) *objectapi.LuaString {
	idx := h & uint32(len(st.buckets)-1) // lmod for power-of-2

	// Search existing bucket
	for _, ts := range st.buckets[idx] {
		if ts.Data == s { // Go string comparison (content)
			return ts
		}
	}

	// Not found — create new
	ts := &objectapi.LuaString{
		Data:    s,
		Hash_:   h,
		IsShort: true,
	}

	// Insert at head of bucket
	st.buckets[idx] = append(st.buckets[idx], ts)
	st.count++

	// Resize if count exceeds bucket count (load factor > 1),
	// but only up to the cap to prevent OOM.
	if st.count > len(st.buckets) && len(st.buckets) < maxStrTabSize {
		st.resize(len(st.buckets) * 2)
	}

	return ts
}

// resize doubles (or changes) the bucket count and rehashes all entries.
func (st *StringTable) resize(newSize int) {
	if newSize < minStrTabSize {
		newSize = minStrTabSize
	}
	if newSize > maxStrTabSize {
		newSize = maxStrTabSize
	}
	newBuckets := make([][]*objectapi.LuaString, newSize)
	mask := uint32(newSize - 1)
	for _, bucket := range st.buckets {
		for _, ts := range bucket {
			idx := ts.Hash_ & mask
			newBuckets[idx] = append(newBuckets[idx], ts)
		}
	}
	st.buckets = newBuckets
}

// Sweep removes dead entries from the string table and optionally shrinks it.
// This matches C Lua's checkSizes behavior: shrink when nuse < size/4.
// Currently a no-op for entry removal (Go GC handles memory), but handles
// bucket array shrinking.
func (st *StringTable) Sweep() {
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
func newLong(s string) *objectapi.LuaString {
	return &objectapi.LuaString{
		Data:    s,
		Hash_:   0,
		IsShort: false,
	}
}

// ---------------------------------------------------------------------------
// String equality
// ---------------------------------------------------------------------------

// Equal compares two LuaStrings.
// For two short strings: pointer equality (both interned in the same table).
// For any other combination: content comparison.
func Equal(a, b *objectapi.LuaString) bool {
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
