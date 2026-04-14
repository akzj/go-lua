// Package api implements Lua's hybrid array+hash table.
//
// A Lua table has two parts:
//   - Array part: integer keys 1..N stored in a Go slice (O(1) access)
//   - Hash part: all other keys stored in an open-addressing hash table
//     using Brent's variation for collision resolution
//
// The # (length) operator finds a "boundary" — an index i where t[i] ~= nil
// and t[i+1] == nil. This is NOT the same as Go's len().
//
// Reference: .analysis/07-runtime-infrastructure.md §3
package api

import objectapi "github.com/akzj/go-lua/internal/object/api"

// ---------------------------------------------------------------------------
// Table — the core Lua data structure
// ---------------------------------------------------------------------------

// Table is a Lua table with hybrid array + hash storage.
type Table struct {
	Array     []objectapi.TValue // array part: indices 0..len-1 map to Lua keys 1..len
	Nodes     []Node             // hash part: open-addressing with Brent's variation
	LsizeNode uint8              // log2(len(nodes)), 0 if nodes == nil
	LastFree  int                // index for free-slot backward scan
	Flags     byte               // metamethod absence cache (bit p = TM p absent)
	Metatable *Table             // metatable or nil
}

// Node is a hash table entry (key + value + chain offset).
type Node struct {
	Val    objectapi.TValue // value
	KeyTT  objectapi.Tag    // key type tag
	KeyVal any              // key value (int64, float64, *objectapi.LuaString, etc.)
	Next   int32            // offset to next node in chain (0 = end)
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// New creates an empty Lua table.
// arraySize and hashSize are hints for pre-allocation.
// hashSize will be rounded up to the next power of 2.
func New(arraySize, hashSize int) *Table {
	return newTable(arraySize, hashSize)
}

// ---------------------------------------------------------------------------
// Get operations — never modify the table
// ---------------------------------------------------------------------------

// Get retrieves the value for the given key. Returns (value, found).
// If the key is not present, returns (object.Nil, false).
// For integer keys in the array range, accesses the array part directly.
func (t *Table) Get(key objectapi.TValue) (objectapi.TValue, bool) {
	return t.get(key)
}

// GetInt retrieves the value for an integer key.
// Checks the array part first, then the hash part.
func (t *Table) GetInt(key int64) (objectapi.TValue, bool) {
	return t.getInt(key)
}

// GetStr retrieves the value for a short string key.
// Uses pointer equality for interned short strings (O(1) amortized).
func (t *Table) GetStr(key *objectapi.LuaString) (objectapi.TValue, bool) {
	return t.getStr(key)
}

// RawLen returns the "raw" length of the table (the # operator result).
// This finds a boundary: an index i where t[i] ~= nil and t[i+1] == nil.
func (t *Table) RawLen() int64 {
	return t.rawLen()
}

// ---------------------------------------------------------------------------
// Set operations — may trigger rehash
// ---------------------------------------------------------------------------

// Set sets the value for the given key. Panics if key is nil or NaN.
// If the key is new and the hash part is full, triggers a rehash.
func (t *Table) Set(key, value objectapi.TValue) {
	t.set(key, value)
}

// SetInt sets the value for an integer key.
func (t *Table) SetInt(key int64, value objectapi.TValue) {
	t.setInt(key, value)
}

// SetStr sets the value for a short string key.
func (t *Table) SetStr(key *objectapi.LuaString, value objectapi.TValue) {
	t.setStr(key, value)
}

// ---------------------------------------------------------------------------
// Iteration
// ---------------------------------------------------------------------------

// Next advances the iterator past the given key and returns the next (key, value) pair.
// To start iteration, pass object.Nil as key.
// Returns (key, value, true) for the next entry, or (Nil, Nil, false) when done.
// Iteration order: array part first (keys 1..N), then hash part.
func (t *Table) Next(key objectapi.TValue) (nextKey, nextVal objectapi.TValue, ok bool) {
	return t.next(key)
}

// ---------------------------------------------------------------------------
// Metatable
// ---------------------------------------------------------------------------

// GetMetatable returns the table's metatable (may be nil).
func (t *Table) GetMetatable() *Table {
	return t.Metatable
}

// SetMetatable sets the table's metatable.
func (t *Table) SetMetatable(mt *Table) {
	t.Metatable = mt
}

// ---------------------------------------------------------------------------
// Metamethod cache (fasttm optimization)
// ---------------------------------------------------------------------------

// InvalidateFlags clears the metamethod absence cache.
// Must be called when the metatable changes.
func (t *Table) InvalidateFlags() {
	t.Flags &^= 0x3F
}

// HasTagMethod returns true if the metamethod at index tm might be present.
// false = definitely absent (cached). true = might be present (check metatable).
// tm is a TMS enum value (0 = TM_INDEX, ..., 5 = TM_EQ are fast-cached).
func (t *Table) HasTagMethod(tm byte) bool {
	return t.Flags&(1<<tm) == 0
}

// SetNoTagMethod marks the given tag method as absent in the cache.
func (t *Table) SetNoTagMethod(tm byte) {
	t.Flags |= 1 << tm
}

// ---------------------------------------------------------------------------
// Array/Hash size info (used by GC, debug, collectgarbage)
// ---------------------------------------------------------------------------

// ArrayLen returns the allocated array part length.
func (t *Table) ArrayLen() int {
	return len(t.Array)
}

// HashLen returns the hash part capacity (always a power of 2, or 0).
func (t *Table) HashLen() int {
	if t.LsizeNode == 0 {
		return 0
	}
	return 1 << t.LsizeNode
}

// EstimateBytes returns an approximate byte size for this table,
// mirroring C Lua's allocation tracking for collectgarbage("count").
// sizeof(Table)=80, sizeof(TValue)=24, sizeof(Node)=56 on 64-bit Go.
func (t *Table) EstimateBytes() int64 {
	const tableOverhead = 80
	const tvalueSize = 24
	const nodeSize = 56
	return int64(tableOverhead) + int64(len(t.Array))*tvalueSize + int64(t.HashLen())*nodeSize
}
