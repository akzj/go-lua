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
package table

import (
	"unsafe"

	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// Table — the core Lua data structure
// ---------------------------------------------------------------------------

// Weak table mode constants.
const (
	WeakKey   byte = 1 // bit 0: weak keys
	WeakValue byte = 2 // bit 1: weak values
)

// Table is a Lua table with hybrid array + hash storage.
type Table struct {
	object.GCHeader                 // GC metadata
	Array           []object.TValue // array part: indices 0..len-1 map to Lua keys 1..len
	Nodes           []node          // hash part: open-addressing with Brent's variation
	LsizeNode       uint8           // log2(len(nodes)), 0 if nodes == nil
	LastFree        int             // index for free-slot backward scan
	Flags           byte            // metamethod absence cache (bit p = TM p absent)
	Metatable       *Table          // metatable or nil

	// Weak table support (__mode metafield)
	WeakMode byte // bit 0 = weak keys, bit 1 = weak values

	// SizeDelta accumulates the net change in ObjSize from table resizes.
	// The VM/API layer checks this after table mutations and calls
	// TrackAllocation to update GCDebt. Reset to 0 after consumption.
	SizeDelta int64
}

// GC returns the GC header for this table.
func (t *Table) GC() *object.GCHeader { return &t.GCHeader }

// HasWeakKeys returns true if this table has weak keys (__mode contains "k").
func (t *Table) HasWeakKeys() bool { return t.WeakMode&WeakKey != 0 }

// HasWeakValues returns true if this table has weak values (__mode contains "v").
func (t *Table) HasWeakValues() bool { return t.WeakMode&WeakValue != 0 }

// node is a hash table entry (key + value + chain offset).
//
// Key storage uses split fields to avoid boxing allocations:
//   - KeyN: numeric payload (int64 for integers, float64 bits for floats)
//   - KeyPtr: pointer payload for GC objects (*LuaString, *Table, etc.)
//   - KeyTT: type tag (packed after Next for alignment)
//   - KeyOldTT: preserved original tag when key is dead (TagDeadKey)
//
// This eliminates heap allocations for integer/float key insertions
// that were previously caused by boxing into the `any` interface.
type node struct {
	Val      object.TValue    // 32 bytes — value
	KeyN     int64            // 8 bytes — numeric key (int64 directly, float64 via bits)
	KeyPtr   unsafe.Pointer   // 8 bytes — GC object key (*LuaString, *Table, etc.)
	Next     int32            // 4 bytes — offset to next node in chain (0 = end)
	KeyTT    object.Tag       // 1 byte — key type tag
	KeyOldTT object.Tag       // 1 byte — original tag preserved when KeyTT = TagDeadKey
	_pad     [2]byte          // 2 bytes padding to align to 8
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
func (t *Table) Get(key object.TValue) (object.TValue, bool) {
	return t.get(key)
}

// GetInt retrieves the value for an integer key.
// Checks the array part first, then the hash part.
func (t *Table) GetInt(key int64) (object.TValue, bool) {
	return t.getInt(key)
}

// GetStr retrieves the value for a short string key.
// Uses pointer equality for interned short strings (O(1) amortized).
func (t *Table) GetStr(key *object.LuaString) (object.TValue, bool) {
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

// SetIfExists overwrites the value for an existing key and returns true.
// If the key is not found, returns false without modifying the table.
// This is the "fast set" path — no insertion, no rehash, single hash lookup.
func (t *Table) SetIfExists(key, value object.TValue) bool {
	return t.setIfExists(key, value)
}

// Set sets the value for the given key. Panics if key is nil or NaN.
// If the key is new and the hash part is full, triggers a rehash.
func (t *Table) Set(key, value object.TValue) {
	t.set(key, value)
}

// SetInt sets the value for an integer key.
func (t *Table) SetInt(key int64, value object.TValue) {
	t.setInt(key, value)
}

// SetStr sets the value for a short string key.
func (t *Table) SetStr(key *object.LuaString, value object.TValue) {
	t.setStr(key, value)
}

// ---------------------------------------------------------------------------
// Iteration
// ---------------------------------------------------------------------------

// Next advances the iterator past the given key and returns the next (key, value) pair.
// To start iteration, pass object.Nil as key.
// Returns (key, value, true) for the next entry, or (Nil, Nil, false) when done.
// Iteration order: array part first (keys 1..N), then hash part.
func (t *Table) Next(key object.TValue) (nextKey, nextVal object.TValue, ok bool, err error) {
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
	if len(t.Nodes) == 0 {
		return 0
	}
	return 1 << t.LsizeNode
}

// EstimateBytes returns an approximate byte size for this table,
// mirroring C Lua's allocation tracking for collectgarbage("count").
func (t *Table) EstimateBytes() int64 {
	const tableOverhead = 80
	const tvalueSize = 24
	const nodeSize = int64(unsafe.Sizeof(node{}))
	return int64(tableOverhead) + int64(len(t.Array))*tvalueSize + int64(t.HashLen())*nodeSize
}

// ResizeArray grows or shrinks the array part to exactly newSize.
// Matches C Lua's luaH_resizearray: keeps the hash part unchanged,
// migrates elements between array and hash as needed.
// Used by OP_SETLIST to pre-allocate the exact array size.
func (t *Table) ResizeArray(newSize int) {
	resizeTable(t, newSize, t.HashLen())
}

// ---------------------------------------------------------------------------
// GC node key access helpers (exported for internal/gc)
// ---------------------------------------------------------------------------

// NodeKeyGCHeader returns the GCHeader for a node key at index i.
// Precondition: the node's KeyTT has BIT_ISCOLLECTABLE set and KeyPtr is non-nil.
// For string keys, KeyPtr points directly to the LuaString (GCHeader is first field).
// For rare collectable types, KeyPtr also points to the object (GCHeader first field).
func (t *Table) NodeKeyGCHeader(i int) *object.GCHeader {
	ptr := t.Nodes[i].KeyPtr
	if ptr == nil {
		return nil
	}
	return (*object.GCHeader)(ptr)
}

// NodeKeyPtr returns the raw key pointer for a node at index i.
// Used by GC to reconstruct GCObject for rare collectable key types.
func (t *Table) NodeKeyPtr(i int) unsafe.Pointer {
	return t.Nodes[i].KeyPtr
}

// MarkNodeKeyDead marks the key at node index i as dead.
// Preserves KeyN/KeyPtr for hash chain probing.
func (t *Table) MarkNodeKeyDead(i int) {
	nd := &t.Nodes[i]
	nd.KeyOldTT = nd.KeyTT
	nd.KeyTT = object.TagDeadKey
}
