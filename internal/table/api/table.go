// Core Table operations for Lua tables.
//
// Implements Get, Set, RawLen, Next, and constructor.
// Reference: lua-master/ltable.c
package api

import (
	"errors"
	"math"

	obj "github.com/akzj/go-lua/internal/object/api"
)

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// newTable creates a Table with pre-allocated array and hash parts.
// hashSize is rounded up to the next power of 2.
func newTable(arraySize, hashSize int) *Table {
	t := &Table{}
	if arraySize > 0 {
		t.Array = make([]obj.TValue, arraySize)
		for i := range t.Array {
			t.Array[i] = obj.Nil
		}
	}
	initHashPart(t, hashSize)
	t.Flags = 0x3F // all 6 fast TM bits set = all absent (empty table)
	return t
}

// ---------------------------------------------------------------------------
// Get operations
// ---------------------------------------------------------------------------

// get retrieves the value for a key. Handles all key types.
func (t *Table) get(key obj.TValue) (obj.TValue, bool) {
	switch key.Tt {
	case obj.TagInteger:
		return t.getInt(key.Val.(int64))
	case obj.TagFloat:
		f := key.Val.(float64)
		if i, ok := floatToInteger(f); ok {
			return t.getInt(i)
		}
		if len(t.Nodes) == 0 {
			return obj.Nil, false
		}
		return getFromHashLoop(t, key, mainPosition(t, key))
	case obj.TagShortStr:
		return t.getStr(key.Val.(*obj.LuaString))
	case obj.TagNil:
		return obj.Nil, false
	default:
		if len(t.Nodes) == 0 {
			return obj.Nil, false
		}
		return getFromHashLoop(t, key, mainPosition(t, key))
	}
}

// getInt retrieves the value for an integer key.
func (t *Table) getInt(key int64) (obj.TValue, bool) {
	if key >= 1 && int(key) <= len(t.Array) {
		v := t.Array[key-1]
		if !v.Tt.IsNil() {
			return v, true
		}
		return obj.Nil, false
	}
	return getIntFromHash(t, key)
}

// getStr retrieves the value for a string key.
func (t *Table) getStr(key *obj.LuaString) (obj.TValue, bool) {
	if key.IsShort {
		return getStrFromHash(t, key)
	}
	k := obj.MakeString(key)
	if len(t.Nodes) == 0 {
		return obj.Nil, false
	}
	return getFromHashLoop(t, k, mainPosition(t, k))
}

// ---------------------------------------------------------------------------
// Set operations
// ---------------------------------------------------------------------------

// set sets the value for a key. Panics on nil/NaN keys.
func (t *Table) set(key, value obj.TValue) {
	switch key.Tt {
	case obj.TagNil:
		panic("table index is nil")
	case obj.TagFloat:
		f := key.Val.(float64)
		if math.IsNaN(f) {
			panic("table index is NaN")
		}
		if i, ok := floatToInteger(f); ok {
			t.setInt(i, value)
			return
		}
		t.setHash(key, value)
	case obj.TagInteger:
		t.setInt(key.Val.(int64), value)
	case obj.TagShortStr:
		t.setStr(key.Val.(*obj.LuaString), value)
	default:
		t.setHash(key, value)
	}
}

// setInt sets the value for an integer key.
func (t *Table) setInt(key int64, value obj.TValue) {
	if key >= 1 && int(key) <= len(t.Array) {
		t.Array[key-1] = value
		return
	}
	intKey := obj.MakeInteger(key)
	t.setHash(intKey, value)
}

// setStr sets the value for a string key.
func (t *Table) setStr(key *obj.LuaString, value obj.TValue) {
	k := obj.MakeString(key)
	t.setHash(k, value)
}

// setHash sets a key-value in the hash part. Handles existing keys,
// dead key reuse, new key insertion, and rehash.
func (t *Table) setHash(key, value obj.TValue) {
	// Try to find existing key (including dead keys for reuse)
	if len(t.Nodes) > 0 {
		mp := mainPosition(t, key)
		idx := mp
		for {
			nd := &t.Nodes[idx]
			if equalKey(key, nd, false) {
				// Live key match — update or delete
				if value.Tt.IsNil() {
					nd.Val = obj.Nil
					nd.KeyTT = obj.TagDeadKey
				} else {
					nd.Val = value
				}
				return
			}
			// Check for dead key with same original value — reuse slot
			if keyIsDead(nd) && equalKey(key, nd, true) {
				if value.Tt.IsNil() {
					return // already dead, setting nil is no-op
				}
				// Resurrect the dead key
				setNodeKey(nd, key)
				nd.Val = value
				return
			}
			nx := nd.Next
			if nx == 0 {
				break
			}
			idx += int(nx)
		}
	}

	// Key not found — insert new
	if value.Tt.IsNil() {
		return // don't insert nil values
	}

	if !insertKey(t, key, value) {
		rehash(t, key)
		// After rehash, key might go to array part
		if key.Tt == obj.TagInteger {
			ik := key.Val.(int64)
			if ik >= 1 && int(ik) <= len(t.Array) {
				t.Array[ik-1] = value
				return
			}
		}
		if !insertKey(t, key, value) {
			panic("table overflow: insert failed after rehash")
		}
	}
	t.InvalidateFlags()
}

// ---------------------------------------------------------------------------
// RawLen — the # operator
// ---------------------------------------------------------------------------

// rawLen finds a boundary: an index i where t[i] ~= nil and t[i+1] == nil.
func (t *Table) rawLen() int64 {
	asize := len(t.Array)
	if asize == 0 {
		return t.hashBoundary()
	}

	// Check if last array element is nil
	if t.Array[asize-1].Tt.IsNil() {
		// Boundary is within array. Binary search for it.
		// We need: t[result] is non-nil (or result==0), t[result+1] is nil.
		// Use 1-based Lua indices for the search.
		return int64(binsearch(t.Array, 0, uint(asize)))
	}

	// Last array element is non-nil — boundary might be beyond array
	if _, found := getIntFromHash(t, int64(asize+1)); !found {
		return int64(asize)
	}

	return t.hashSearchBoundary(int64(asize))
}

// binsearch does binary search in the array for a boundary.
// lo and hi are 1-based Lua indices.
// Invariant: t[lo] is present (or lo==0), t[hi+1] is absent (or hi==asize).
// Actually: lo is a present index (0 means "before start"), hi is an absent index.
// We find the largest i in [lo, hi) such that arr[i-1] is non-nil.
func binsearch(arr []obj.TValue, lo, hi uint) uint {
	// lo: 1-based index known present (or 0 = before array)
	// hi: 1-based index known absent
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if mid == 0 || arr[mid-1].Tt.IsNil() {
			hi = mid
		} else {
			lo = mid
		}
	}
	return lo
}

// hashBoundary finds a boundary when there's no array part.
func (t *Table) hashBoundary() int64 {
	if _, found := getIntFromHash(t, 1); !found {
		return 0
	}
	return t.hashSearchBoundary(0)
}

// hashSearchBoundary finds boundary starting from known-present asize.
func (t *Table) hashSearchBoundary(asize int64) int64 {
	i := asize + 1 // known present
	j := i * 2
	if j < i+1 {
		j = i + 1 // overflow guard
	}
	for {
		if _, found := getIntFromHash(t, j); !found {
			break
		}
		i = j
		if j > math.MaxInt64/2 {
			return j
		}
		j = j*2 + 1
	}
	for j-i > 1 {
		m := i + (j-i)/2
		if _, found := getIntFromHash(t, m); found {
			i = m
		} else {
			j = m
		}
	}
	return i
}

// ---------------------------------------------------------------------------
// Next — iteration
// ---------------------------------------------------------------------------

// ErrInvalidKey is returned by Next when the given key is not in the table.
var ErrInvalidKey = errors.New("invalid key to 'next'")

// next advances the iterator. Pass Nil to start.
// Returns ErrInvalidKey if the key is not found in the table.
func (t *Table) next(key obj.TValue) (obj.TValue, obj.TValue, bool, error) {
	asize := len(t.Array)

	var startIdx int // 0-based unified index (array then hash)
	if key.Tt.IsNil() {
		startIdx = 0
	} else if key.Tt == obj.TagInteger {
		k := key.Val.(int64)
		if k >= 1 && int(k) <= asize {
			startIdx = int(k) // next position after array index k
		} else {
			ni, found := getFromHashDeadOk(t, key)
			if !found {
				return obj.Nil, obj.Nil, false, ErrInvalidKey
			}
			startIdx = asize + ni + 1
		}
	} else {
		ni, found := getFromHashDeadOk(t, key)
		if !found {
			return obj.Nil, obj.Nil, false, ErrInvalidKey
		}
		startIdx = asize + ni + 1
	}

	// Scan array part
	for i := startIdx; i < asize; i++ {
		if !t.Array[i].Tt.IsNil() {
			return obj.MakeInteger(int64(i + 1)), t.Array[i], true, nil
		}
	}

	// Scan hash part
	hashStart := startIdx - asize
	if hashStart < 0 {
		hashStart = 0
	}
	for i := hashStart; i < len(t.Nodes); i++ {
		nd := &t.Nodes[i]
		if !nodeIsEmpty(nd) {
			return nodeKey(nd), nd.Val, true, nil
		}
	}

	return obj.Nil, obj.Nil, false, nil
}

// ---------------------------------------------------------------------------
// Float-to-integer conversion (for table keys)
// ---------------------------------------------------------------------------

func floatToInteger(f float64) (int64, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	i := int64(f)
	if float64(i) == f {
		return i, true
	}
	return 0, false
}
