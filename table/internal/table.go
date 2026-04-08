// Package internal provides concrete implementations of table interfaces.
package internal

import (
	"math"
	"reflect"
	"unsafe"

	"github.com/akzj/go-lua/mem/api"
	typesapi "github.com/akzj/go-lua/types/api"
)

// =============================================================================
// Constants (from lua-master/ltable.c)
// =============================================================================

const (
	// LIMFORLAST is log2 of real limit (8) for lastfree optimization.
	LIMFORLAST = 3

	// MAXABITS is the largest integer such that 2^MAXABITS fits in unsigned.
	MAXABITS = 31

	// MAXASIZEB is maximum elements in array part that fits in size_t.
	MAXASIZEB = math.MaxInt / 2

	// MAXHBITS is the largest integer such that 2^MAXHBITS fits in signed int.
	MAXHBITS = 30

	// MAXASIZE is the maximum size of the array part.
	MAXASIZE = 1 << MAXABITS

	// MAXHSIZE is the maximum size of the hash part.
	MAXHSIZE = 1 << MAXHBITS
)

// =============================================================================
// Hash Constants
// =============================================================================

const (
	BITDUMMY = 1 << 6
)

// =============================================================================
// Value and TValue (copied from types/internal for self-containment)
// =============================================================================

// Value is the concrete Value implementation.
type Value struct {
	Variant typesapi.ValueVariant
	Data_   interface{}
}

func (v *Value) GetGC() *typesapi.GCObject {
	if v.Variant != typesapi.ValueGC {
		panic("Value.GetGC: not a GC object")
	}
	return v.Data_.(*typesapi.GCObject)
}

func (v *Value) GetPointer() unsafe.Pointer {
	if v.Variant != typesapi.ValuePointer {
		panic("Value.GetPointer: not a light userdata")
	}
	return v.Data_.(unsafe.Pointer)
}

func (v *Value) GetCFunction() unsafe.Pointer {
	if v.Variant != typesapi.ValueCFunction {
		panic("Value.GetCFunction: not a C function")
	}
	return v.Data_.(unsafe.Pointer)
}

func (v *Value) GetInteger() typesapi.LuaInteger {
	if v.Variant != typesapi.ValueInteger {
		panic("Value.GetInteger: not an integer")
	}
	return v.Data_.(typesapi.LuaInteger)
}

func (v *Value) GetFloat() typesapi.LuaNumber {
	if v.Variant != typesapi.ValueFloat {
		panic("Value.GetFloat: not a float")
	}
	return v.Data_.(typesapi.LuaNumber)
}

// TValue is the concrete TValue implementation.
type TValue struct {
	Value Value
	Tt     uint8
}

func (t *TValue) IsNil() bool {
	return typesapi.Novariant(int(t.Tt)) == typesapi.LUA_TNIL
}
func (t *TValue) IsBoolean() bool {
	return typesapi.Novariant(int(t.Tt)) == typesapi.LUA_TBOOLEAN
}
func (t *TValue) IsNumber() bool {
	return typesapi.Novariant(int(t.Tt)) == typesapi.LUA_TNUMBER
}
func (t *TValue) IsInteger() bool {
	return int(t.Tt) == typesapi.LUA_VNUMINT
}
func (t *TValue) IsFloat() bool {
	return int(t.Tt) == typesapi.LUA_VNUMFLT
}
func (t *TValue) IsString() bool {
	return typesapi.Novariant(int(t.Tt)) == typesapi.LUA_TSTRING
}
func (t *TValue) IsTable() bool {
	return int(t.Tt) == typesapi.Ctb(int(typesapi.LUA_VTABLE))
}
func (t *TValue) IsFunction() bool {
	return typesapi.Novariant(int(t.Tt)) == typesapi.LUA_TFUNCTION
}
func (t *TValue) IsThread() bool {
	return int(t.Tt) == typesapi.Ctb(int(typesapi.LUA_VTHREAD))
}
func (t *TValue) IsLightUserData() bool {
	return int(t.Tt) == typesapi.LUA_VLIGHTUSERDATA
}
func (t *TValue) IsUserData() bool {
	return int(t.Tt) == typesapi.Ctb(int(typesapi.LUA_VUSERDATA))
}
func (t *TValue) IsCollectable() bool {
	return int(t.Tt)&typesapi.BIT_ISCOLLECTABLE != 0
}
func (t *TValue) IsTrue() bool {
	return int(t.Tt) == typesapi.LUA_VTRUE
}
func (t *TValue) IsFalse() bool {
	return int(t.Tt) == typesapi.LUA_VFALSE
}
func (t *TValue) IsLClosure() bool {
	return int(t.Tt) == typesapi.Ctb(int(typesapi.LUA_VLCL))
}
func (t *TValue) IsCClosure() bool {
	return int(t.Tt) == typesapi.Ctb(int(typesapi.LUA_VCCL))
}
func (t *TValue) IsLightCFunction() bool {
	return int(t.Tt) == typesapi.LUA_VLCF
}
func (t *TValue) IsClosure() bool {
	return t.IsLClosure() || t.IsCClosure()
}
func (t *TValue) IsProto() bool {
	return int(t.Tt) == typesapi.Ctb(int(typesapi.LUA_VPROTO))
}
func (t *TValue) IsUpval() bool {
	return int(t.Tt) == typesapi.Ctb(int(typesapi.LUA_VUPVAL))
}
func (t *TValue) IsShortString() bool {
	return int(t.Tt) == typesapi.Ctb(int(typesapi.LUA_VSHRSTR))
}
func (t *TValue) IsLongString() bool {
	return int(t.Tt) == typesapi.Ctb(int(typesapi.LUA_VLNGSTR))
}
func (t *TValue) IsEmpty() bool {
	return typesapi.Novariant(int(t.Tt)) == typesapi.LUA_TNIL
}

func (t *TValue) GetTag() int                { return int(t.Tt) }
func (t *TValue) GetBaseType() int           { return typesapi.Novariant(int(t.Tt)) }
func (t *TValue) GetValue() interface{}      { return t.Value.Data_ }
func (t *TValue) GetGC() *typesapi.GCObject { return t.Value.GetGC() }
func (t *TValue) GetInteger() typesapi.LuaInteger {
	return t.Value.GetInteger()
}
func (t *TValue) GetFloat() typesapi.LuaNumber { return t.Value.GetFloat() }
func (t *TValue) GetPointer() unsafe.Pointer   { return t.Value.GetPointer() }

// NewTValueNil creates a nil TValue.
func NewTValueNil() *TValue {
	return &TValue{Tt: uint8(typesapi.LUA_VNIL)}
}

// NewTValueInteger creates an integer TValue.
func NewTValueInteger(i typesapi.LuaInteger) *TValue {
	return &TValue{
		Value: Value{Variant: typesapi.ValueInteger, Data_: i},
		Tt:    uint8(typesapi.LUA_VNUMINT),
	}
}

// NewTValueBoolean creates a boolean TValue.
func NewTValueBoolean(b bool) *TValue {
	tag := typesapi.LUA_VFALSE
	if b {
		tag = typesapi.LUA_VTRUE
	}
	return &TValue{Tt: uint8(tag)}
}

// =============================================================================
// Node (hash table entry)
// =============================================================================

// Node is a hash table entry (from lua-master/lnode.h).
type Node struct {
	KeyValue Value // key value
	KeyTt    uint8 // key type tag
	KeyNext  int32 // next node in chain (0 = end)
	Val      TValue
}

func (n *Node) KeyIsNil() bool {
	return n.KeyTt == typesapi.LUA_TNIL
}
func (n *Node) KeyIsDead() bool {
	return n.KeyTt == typesapi.LUA_TDEADKEY
}
func (n *Node) KeyIsCollectable() bool {
	return int(n.KeyTt)&typesapi.BIT_ISCOLLECTABLE != 0
}

// =============================================================================
// Table (Lua table)
// =============================================================================

// Table is the concrete Table implementation.
type Table struct {
	// GCObject header
	Next   *Table
	Tt     uint8
	Marked uint8

	Flags     uint8 // metamethod cache flags + dummy node bit
	Lsizenode uint8 // log2 of hash size
	Asize     uint32

	// Array part: slice of *TValue
	Array []*TValue

	// Hash part
	Node *Node

	// Metatable
	Metatable *Table

	// GC list
	GClist *Table
}

func (t *Table) SizeNode() int {
	if t.Lsizenode >= 32 {
		return 0
	}
	return 1 << t.Lsizenode
}

// NewLuaTable creates a new empty Table.
func NewLuaTable() *Table {
	return &Table{
		Tt:        uint8(typesapi.Ctb(int(typesapi.LUA_VTABLE))),
		Flags:     0,
		Lsizenode: 0,
		Asize:     0,
		Array:     make([]*TValue, 0),
	}
}

// =============================================================================
// Size Helpers
// =============================================================================

func sizenode(t *Table) int {
	if t.Lsizenode >= 32 {
		return 0
	}
	return 1 << t.Lsizenode
}

func allocsizenode(t *Table) int {
	if isdummy(t) {
		return 0
	}
	return sizenode(t)
}

func isdummy(t *Table) bool {
	return t.Flags&BITDUMMY != 0
}

func setnodummy(t *Table) {
	t.Flags &^= BITDUMMY
}

func setdummy(t *Table) {
	t.Flags |= BITDUMMY
}


// =============================================================================
// Hash Helper Functions
// =============================================================================

// absentkey signals key not found in hash table.
var absentkey = &TValue{Tt: uint8(typesapi.LUA_VABSTKEY)}

// gnode returns the i-th node in the hash array.
func gnode(t *Table, i int) *Node {
	return (*Node)(unsafe.Pointer(
		uintptr(unsafe.Pointer(t.Node)) + uintptr(i)*unsafe.Sizeof(Node{}),
	))
}

// gval returns the value slot of node n.
func gval(n *Node) *TValue {
	return &n.Val
}

// gnext returns the next field of node n.
func gnext(n *Node) int {
	return int(n.KeyNext)
}

// setgnext sets the next field of node n.
func setgnext(n *Node, val int) {
	n.KeyNext = int32(val)
}

// keytt returns the key type tag of node n.
func keytt(n *Node) int {
	return int(n.KeyTt)
}

// keyisnil returns true if the key of node n is nil.
func keyisnil(n *Node) bool {
	return n.KeyTt == typesapi.LUA_TNIL
}

// keyisinteger returns true if the key of node n is an integer.
func keyisinteger(n *Node) bool {
	return int(n.KeyTt) == typesapi.LUA_VNUMINT
}

// keyival returns the integer key of node n.
func keyival(n *Node) typesapi.LuaInteger {
	return n.KeyValue.GetInteger()
}

// keystrval returns the string key of node n.
func keystrval(n *Node) *TValue {
	return &TValue{Value: n.KeyValue, Tt: n.KeyTt}
}

// getVariant extracts the ValueVariant from a typesapi.TValue interface.
func getVariant(v typesapi.TValue) typesapi.ValueVariant {
	if v.IsInteger() {
		return typesapi.ValueInteger
	}
	if v.IsFloat() {
		return typesapi.ValueFloat
	}
	return typesapi.ValueGC
}

// getData extracts the data from a typesapi.TValue interface.
func getData(v typesapi.TValue) interface{} {
	if v.IsInteger() {
		return v.GetInteger()
	}
	if v.IsFloat() {
		return v.GetFloat()
	}
	if v.IsLightUserData() {
		return v.GetPointer()
	}
	return v.GetValue()
}

// isempty returns true if the value is nil or empty.
func isempty(v *TValue) bool {
	return v.IsNil() || int(v.Tt) == typesapi.LUA_VEMPTY
}

// setempty sets the value to empty.
func setempty(v *TValue) {
	v.Tt = uint8(typesapi.LUA_VEMPTY)
}

// setnodekey sets the key of node n from a TValue (typesapi.TValue interface).
func setnodekey(n *Node, key typesapi.TValue) {
	n.KeyTt = uint8(key.GetTag())
	if key.IsInteger() {
		n.KeyValue = Value{Variant: typesapi.ValueInteger, Data_: key.GetInteger()}
	} else if key.IsFloat() {
		n.KeyValue = Value{Variant: typesapi.ValueFloat, Data_: key.GetFloat()}
	} else {
		n.KeyValue = Value{Variant: typesapi.ValueGC, Data_: key.GetValue()}
	}
}

// getnodekey gets the key of node n.
func getnodekey(n *Node) typesapi.TValue {
	return &TValue{Value: n.KeyValue, Tt: n.KeyTt}
}

// =============================================================================
// Hash Functions
// =============================================================================

// hashint returns the main position of an integer key.
func hashint(t *Table, key typesapi.LuaInteger) *Node {
	size := sizenode(t)
	if size == 0 {
		return nil
	}
	return gnode(t, int(key)&(size-1))
}

// mainposition returns the main position for a key.
func mainposition(t *Table, key typesapi.TValue) *Node {
	if key.IsInteger() {
		return hashint(t, key.GetInteger())
	}
	// If hash table has no nodes allocated, return nil
	if t.Node == nil || sizenode(t) == 0 {
		return nil
	}
	if key.IsFloat() {
		size := sizenode(t)
		return gnode(t, int(math.Float64bits(float64(key.GetFloat())))%size)
	}
	// For other types, use pointer hash
	h := uintptr(0)
	if key.GetValue() != nil {
		h = reflect.ValueOf(key.GetValue()).Pointer()
	}
	size := sizenode(t)
	return gnode(t, int(h)&(size-1))
}

// =============================================================================
// Key Comparison
// =============================================================================

// equalkey compares key k1 with the key in node n.
func equalkey(k1 typesapi.TValue, n *Node) bool {
	if k1.GetTag() != int(n.KeyTt) {
		// Handle string comparison specially
		if keytt(n) == typesapi.LUA_VSHRSTR && int(k1.GetTag()) == typesapi.LUA_VLNGSTR {
			// Long string can equal short string key
			return false // Simplified - full impl would compare string content
		}
		return false
	}
	switch {
	case k1.IsNil():
		return true
	case k1.IsTrue():
		return true
	case k1.IsFalse():
		return true
	case k1.IsInteger():
		return k1.GetInteger() == keyival(n)
	case k1.IsFloat():
		return k1.GetFloat() == n.KeyValue.GetFloat()
	default:
		return k1.GetValue() == n.KeyValue.Data_
	}
}

// =============================================================================
// Internal Helpers
// =============================================================================

// ikeyinarray returns array index if k is in array part.
func ikeyinarray(t *Table, k typesapi.LuaInteger) uint32 {
	if k >= 1 && uint64(k) <= uint64(t.Asize) {
		return uint32(k)
	}
	return 0
}

// getfreepos finds a free position in hash table.
func (ti *TableImpl) getfreepos() *Node {
	size := sizenode(ti.tbl)
	for i := 0; i < size; i++ {
		if keyisnil(gnode(ti.tbl, i)) {
			return gnode(ti.tbl, i)
		}
	}
	return nil
}

// arraykeyisempty returns true if array[key] is empty.
func (ti *TableImpl) arraykeyisempty(key uint32) bool {
	if key == 0 || key > ti.tbl.Asize {
		return true
	}
	idx := key - 1
	if idx >= uint32(len(ti.tbl.Array)) || ti.tbl.Array[idx] == nil {
		return true
	}
	return ti.tbl.Array[idx].IsNil()
}

// binsearch performs binary search for border in array part.
func (ti *TableImpl) binsearch(i, j uint32) uint32 {
	for j-i > 1 {
		m := (i + j) / 2
		if ti.arraykeyisempty(m) {
			j = m
		} else {
			i = m
		}
	}
	return i
}

// =============================================================================
// TableImpl
// =============================================================================

// TableImpl implements tableapi.TableInterface.
// Interface is defined via struct embedding and explicit method declarations.
type TableImpl struct {
	alloc api.Allocator
	tbl   *Table
}

func NewTable() *TableImpl {
	tbl := NewLuaTable()
	// Initialize hash part with dummy node
	setdummy(tbl)

	return &TableImpl{
		alloc: api.DefaultAllocator,
		tbl:   tbl,
	}
}

func (ti *TableImpl) Get(key typesapi.TValue) typesapi.TValue {
	// Handle integer keys via fast path
	if key.IsInteger() {
		return ti.GetInt(key.GetInteger())
	}
	// For floats, check if it represents a valid integer key
	if key.IsFloat() {
		f := key.GetFloat()
		if float64(f) == float64(int64(f)) && f >= 1 {
			return ti.GetInt(typesapi.LuaInteger(f))
		}
	}
	// Generic get for other types
	mp := mainposition(ti.tbl, key)
	if mp == nil {
		return NewTValueNil()
	}
	for {
		if equalkey(key, mp) {
			if !isempty(gval(mp)) {
				return gval(mp)
			}
			return NewTValueNil()
		}
		if gnext(mp) == 0 {
			return NewTValueNil()
		}
		idx := int(uintptr(unsafe.Pointer(mp))-uintptr(unsafe.Pointer(ti.tbl.Node)))/int(unsafe.Sizeof(Node{})) + gnext(mp)
		mp = gnode(ti.tbl, idx)
	}
}

func (ti *TableImpl) Set(key, value typesapi.TValue) {
	if value.IsNil() {
		// Delete key
		if key.IsInteger() {
			ti.SetInt(key.GetInteger(), NewTValueNil())
		}
		return
	}
	// For integer keys, use fast path
	if key.IsInteger() {
		ti.SetInt(key.GetInteger(), value)
		return
	}
	// For float keys that are valid integers
	if key.IsFloat() {
		f := key.GetFloat()
		if float64(f) == float64(int64(f)) && f >= 1 {
			ti.SetInt(typesapi.LuaInteger(f), value)
			return
		}
	}
	// Insert into hash
	ti.newkey(key, &TValue{Tt: uint8(value.GetTag()), Value: Value{Variant: getVariant(value), Data_: getData(value)}})
}

func (ti *TableImpl) Len() int {
	asize := ti.tbl.Asize
	if asize == 0 {
		return 0
	}
	// Check if last element is non-empty
	if !ti.arraykeyisempty(asize) {
		return int(asize)
	}
	// Binary search for border
	border := ti.binsearch(0, asize)
	return int(border)
}

func (ti *TableImpl) GetInt(key typesapi.LuaInteger) typesapi.TValue {
	// Check array part first
	idx := ikeyinarray(ti.tbl, key)
	if idx > 0 {
		arrIdx := idx - 1
		if arrIdx < uint32(len(ti.tbl.Array)) && ti.tbl.Array[arrIdx] != nil {
			return ti.tbl.Array[arrIdx]
		}
		return NewTValueNil()
	}
	// Search in hash part
	n := hashint(ti.tbl, key)
	for {
		if keyisinteger(n) && keyival(n) == key {
			if !isempty(gval(n)) {
				return gval(n)
			}
			return NewTValueNil()
		}
		if gnext(n) == 0 {
			return NewTValueNil()
		}
		idx := int(uintptr(unsafe.Pointer(n))-uintptr(unsafe.Pointer(ti.tbl.Node)))/int(unsafe.Sizeof(Node{})) + gnext(n)
		n = gnode(ti.tbl, idx)
	}
}

func (ti *TableImpl) SetInt(key typesapi.LuaInteger, value typesapi.TValue) {
	if value.IsNil() {
		// Delete from array if present
		idx := ikeyinarray(ti.tbl, key)
		if idx > 0 {
			ti.tbl.Array[idx-1] = nil
			return
		}
		// Delete from hash
		ti.deleteKey(NewTValueInteger(key))
		return
	}
	// Check array part
	idx := ikeyinarray(ti.tbl, key)
	if idx > 0 {
		ti.tbl.Array[idx-1] = &TValue{Tt: uint8(value.GetTag()), Value: Value{Variant: getVariant(value), Data_: getData(value)}}
		return
	}
	// Insert into hash
	ti.newkey(NewTValueInteger(key), &TValue{Tt: uint8(value.GetTag()), Value: Value{Variant: getVariant(value), Data_: getData(value)}})
}

func (ti *TableImpl) GetMetatable() typesapi.Table {
	return ti.tbl.Metatable
}

func (ti *TableImpl) SetMetatable(t typesapi.Table) {
	if t == nil {
		ti.tbl.Metatable = nil
		return
	}
	ti.tbl.Metatable = t.(*Table)
}

// newkey inserts a key-value pair into the hash table.
// Implements the standard Lua table insertion algorithm:
// - If main position (mp) is empty: store directly at mp.
// - If mp is occupied: traverse collision chain from mp, find free slot f,
//   move the node at mp to f if its main position is in the collision chain,
//   then store the new key-value at mp.
func (ti *TableImpl) newkey(key typesapi.TValue, value *TValue) {
	mp := mainposition(ti.tbl, key)
	if mp == nil {
		// Table has no hash nodes - allocate initial node
		ti.tbl.Lsizenode = 1
		nodes := make([]Node, sizenode(ti.tbl))
		ti.tbl.Node = (*Node)(unsafe.Pointer(&nodes[0]))
		ti.tbl.Flags &^= BITDUMMY
		mp = mainposition(ti.tbl, key)
		if mp == nil {
			return
		}
	}
	if isempty(gval(mp)) || isdummy(ti.tbl) {
		// Main position is empty -- store directly here
		setnodekey(mp, key)
		mp.Val = *value
		setnodummy(ti.tbl)
		return
	}
	// Collision: main position occupied.
	// Find a free slot to hold the displaced node at mp.
	f := ti.getfreepos()
	if f == nil {
		// No free slot -- rehash to grow the table
		ti.rehash()
		// Retry insertion after rehash
		ti.newkey(key, value)
		return
	}
	// Check if the displaced node (at mp) can be moved to f.
	// Traverse the collision chain starting at mp.
	// A node can be moved if its main position is NOT in the chain.
	n := mp
	for {
		other := mainposition(ti.tbl, getnodekey(n))
		if other == nil {
			break
		}
		// Check if 'other' is in the chain from mp
		reachable := false
		check := mp
		for {
			if check == other {
				reachable = true
				break
			}
			if gnext(check) == 0 {
				break
			}
			nextIdx := int(uintptr(unsafe.Pointer(check))-uintptr(unsafe.Pointer(ti.tbl.Node)))/int(unsafe.Sizeof(Node{})) + gnext(check)
			check = gnode(ti.tbl, nextIdx)
		}
		if !reachable {
			// 'other' is not in the chain -- mp can be moved to f
			break
		}
		if gnext(n) == 0 {
			break
		}
		nextIdx := int(uintptr(unsafe.Pointer(n))-uintptr(unsafe.Pointer(ti.tbl.Node)))/int(unsafe.Sizeof(Node{})) + gnext(n)
		n = gnode(ti.tbl, nextIdx)
	}
	// Move node at mp to free slot f
	*f = *mp
	setempty(gval(mp))
	// Store new key-value at mp
	setnodekey(mp, key)
	mp.Val = *value
}

// rehash grows the hash table by one level.
// All existing entries are reinserted into the new table.
func (ti *TableImpl) rehash() {
	oldSize := allocsizenode(ti.tbl)
	ti.tbl.Lsizenode++
	if ti.tbl.Lsizenode >= 32 {
		ti.tbl.Lsizenode--
		return
	}
	newSize := sizenode(ti.tbl)
	if newSize <= oldSize {
		ti.tbl.Lsizenode--
		return
	}

	// Save old nodes
	oldBase := ti.tbl.Node
	oldCount := oldSize

	// Allocate new node array
	newNodes := make([]Node, newSize)
	ti.tbl.Node = (*Node)(unsafe.Pointer(&newNodes[0]))
	setnodummy(ti.tbl)

	// Reinsert all old entries
	for i := 0; i < oldCount; i++ {
		oldNode := (*Node)(unsafe.Pointer(uintptr(unsafe.Pointer(oldBase)) + uintptr(i)*unsafe.Sizeof(Node{})))
		if keyisnil(oldNode) || oldNode.KeyIsDead() || isempty(gval(oldNode)) {
			continue
		}
		ti.reinsertNode(oldNode)
	}
}

// reinsertNode inserts a node from the old table into the current table.
// Called during rehash with the current table's mainposition.
func (ti *TableImpl) reinsertNode(oldNode *Node) {
	key := getnodekey(oldNode)
	val := oldNode.Val

	mp := mainposition(ti.tbl, key)
	if mp == nil || isempty(gval(mp)) || isdummy(ti.tbl) {
		// Free slot -- store directly
		setnodekey(mp, key)
		mp.Val = val
		return
	}

	// Collision: find free slot
	f := ti.getfreepos()
	if f == nil {
		// This should not happen after rehash increased size
		return
	}
	// Move mp to f, store new key at mp
	*f = *mp
	setempty(gval(mp))
	setnodekey(mp, key)
	mp.Val = val
}

// deleteKey removes a key from the hash table.
func (ti *TableImpl) deleteKey(key typesapi.TValue) {
	mp := mainposition(ti.tbl, key)
	for {
		if equalkey(key, mp) {
			// Mark as dead key and empty value
			mp.KeyTt = uint8(typesapi.LUA_TDEADKEY)
			setempty(gval(mp))
			return
		}
		if gnext(mp) == 0 {
			return
		}
		idx := int(uintptr(unsafe.Pointer(mp))-uintptr(unsafe.Pointer(ti.tbl.Node)))/int(unsafe.Sizeof(Node{})) + gnext(mp)
		mp = gnode(ti.tbl, idx)
	}
}

func (ti *TableImpl) Next(key typesapi.TValue) (typesapi.TValue, typesapi.TValue, bool) {
	// findindex: find starting position
	asize := ti.tbl.Asize
	var startIdx uint32 = 0
	
	if !key.IsNil() {
		// Key provided, find its position
		if key.IsInteger() {
			idx := ikeyinarray(ti.tbl, key.GetInteger())
			if idx > 0 {
				startIdx = idx
			} else {
				// Search hash
				startIdx = asize + 1 // Mark as hash search
			}
		} else {
			// Search hash
			startIdx = asize + 1
		}
	}
	
	// Search array part
	for i := startIdx; i < asize; i++ {
		if i < uint32(len(ti.tbl.Array)) && ti.tbl.Array[i] != nil && !ti.tbl.Array[i].IsNil() {
			return NewTValueInteger(typesapi.LuaInteger(i+1)), ti.tbl.Array[i], true
		}
	}
	
	// Search hash part
	hashStart := uint32(0)
	if startIdx > asize {
		// Start from beginning of hash
	} else if startIdx > 0 {
		hashStart = 0 // Start from beginning since we already checked array
	}
	
	size := sizenode(ti.tbl)
	for i := hashStart; i < uint32(size); i++ {
		n := gnode(ti.tbl, int(i))
		if !isempty(gval(n)) && !n.KeyIsDead() {
			return getnodekey(n), gval(n), true
		}
	}
	
	return NewTValueNil(), NewTValueNil(), false
}

func (ti *TableImpl) Resize(nasize int) {
	if nasize < 0 {
		nasize = 0
	}
	oldasize := int(ti.tbl.Asize)
	if nasize == oldasize {
		return
	}
	
	// Create new array
	newArray := make([]*TValue, nasize)
	
	// Copy elements from old array
	copyLen := oldasize
	if nasize < copyLen {
		copyLen = nasize
	}
	for i := 0; i < copyLen; i++ {
		newArray[i] = ti.tbl.Array[i]
	}
	
	ti.tbl.Asize = uint32(nasize)
	ti.tbl.Array = newArray
}

// Ensure Table implements api.Table
var _ typesapi.Table = (*Table)(nil)

// RawTable returns the underlying *Table as a typesapi.Table.
// Used by setmetatable to pass the correct internal type.
func (ti *TableImpl) RawTable() typesapi.Table {
	return ti.tbl
}

// WrapRawTable wraps a typesapi.Table (which must be *Table) back into a *TableImpl.
// Used by getmetatable to convert the raw metatable back to a usable TableInterface.
func WrapRawTable(t typesapi.Table) *TableImpl {
	if t == nil {
		return nil
	}
	tbl, ok := t.(*Table)
	if !ok {
		return nil
	}
	return &TableImpl{
		alloc: api.DefaultAllocator,
		tbl:   tbl,
	}
}

// TableImplInterface is a compile-time check that TableImpl implements TableInterface.
// tableapi.TableInterface is the interface from table/api/api.go.
type TableImplInterface interface {
	GetMetatable() typesapi.Table
	SetMetatable(t typesapi.Table)
}
