// Hash part internals for Lua tables.
//
// Implements open-addressing hash table with Brent's variation for
// collision resolution. Reference: lua-master/ltable.c
package api

import (
	"math"
	"math/bits"

	obj "github.com/akzj/go-lua/internal/object/api"
)

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

// ceilLog2 returns ⌈log2(x)⌉. Returns 0 for x <= 1.
func ceilLog2(x uint) uint8 {
	if x <= 1 {
		return 0
	}
	return uint8(bits.Len(x - 1))
}

// ---------------------------------------------------------------------------
// Hash functions — faithful to C Lua's hashing
// ---------------------------------------------------------------------------

// hashInt hashes an integer key. Uses modulo with (size-1)|1.
func hashInt(i int64, hmask int) int {
	ui := uint64(i)
	return int(ui % uint64(hmask|1))
}

// hashFloat hashes a float key. Matches C: l_hashfloat.
func hashFloat(n float64) uint {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	frac, exp := math.Frexp(n)
	ni := int64(frac * float64(-math.MinInt32))
	u := uint(int(exp)) + uint(ni)
	if u <= uint(math.MaxInt32) {
		return u
	}
	return ^u
}

// hashStr hashes a string key using its cached hash. Power-of-2 modulo.
func hashStr(s *obj.LuaString, hmask int) int {
	return int(s.Hash()) & hmask
}

// ---------------------------------------------------------------------------
// Main position
// ---------------------------------------------------------------------------

// mainPosition returns the hash bucket index for a key.
func mainPosition(t *Table, key obj.TValue) int {
	hmask := (1 << t.LsizeNode) - 1
	switch key.Tt {
	case obj.TagInteger:
		return hashInt(key.Val.(int64), hmask)
	case obj.TagFloat:
		h := hashFloat(key.Val.(float64))
		return int(h % uint(hmask|1))
	case obj.TagShortStr:
		return hashStr(key.Val.(*obj.LuaString), hmask)
	case obj.TagLongStr:
		s := key.Val.(*obj.LuaString)
		return int(s.Hash()) & hmask
	case obj.TagFalse:
		return 0 & hmask
	case obj.TagTrue:
		return 1 & hmask
	case obj.TagLightCFunc:
		// Use interface data word (unique per closure instance) for hashing.
		ptr := obj.FuncDataPtr(key.Val)
		return hashInt(int64(ptr), hmask)
	default:
		// For other types, use tag as hash (simple but correct).
		return int(key.Tt) & hmask
	}
}

// mainPositionFromNode returns the main position for a node's key.
// Dead keys (TagDeadKey) preserve their original KeyVal, so we reconstruct
// the original tag from the Go type of KeyVal to hash correctly.
func mainPositionFromNode(t *Table, nd *Node) int {
	key := nodeKey(nd)
	if key.Tt == obj.TagDeadKey {
		// Reconstruct original tag from the preserved KeyVal
		switch v := nd.KeyVal.(type) {
		case int64:
			key.Tt = obj.TagInteger
		case float64:
			key.Tt = obj.TagFloat
		case *obj.LuaString:
			if v.IsShort {
				key.Tt = obj.TagShortStr
			} else {
				key.Tt = obj.TagLongStr
			}
		case bool:
			if v {
				key.Tt = obj.TagTrue
			} else {
				key.Tt = obj.TagFalse
			}
		default:
			// Other collectable types — use pointer identity hash
			hmask := (1 << t.LsizeNode) - 1
			return int(uintptr(0)) & hmask
		}
	}
	return mainPosition(t, key)
}

// ---------------------------------------------------------------------------
// Node helpers
// ---------------------------------------------------------------------------

func nodeKey(nd *Node) obj.TValue {
	return obj.TValue{Tt: nd.KeyTT, Val: nd.KeyVal}
}

func setNodeKey(nd *Node, key obj.TValue) {
	nd.KeyTT = key.Tt
	nd.KeyVal = key.Val
}

func nodeIsEmpty(nd *Node) bool {
	return nd.Val.Tt.IsNil()
}

func keyIsNil(nd *Node) bool {
	return nd.KeyTT == obj.TagNil
}

func keyIsDead(nd *Node) bool {
	return nd.KeyTT == obj.TagDeadKey
}

// ---------------------------------------------------------------------------
// Key equality
// ---------------------------------------------------------------------------

func equalKey(k1 obj.TValue, n2 *Node, deadOk bool) bool {
	if k1.Tt != n2.KeyTT {
		if deadOk && keyIsDead(n2) {
			// Dead key: compare by value identity for collectable types
			// Go func values are not comparable; use reflect for those.
			if k1.Tt == obj.TagLightCFunc {
				return obj.LightCFuncEqual(k1.Val, n2.KeyVal)
			}
			return k1.Val == n2.KeyVal
		}
		if n2.KeyTT == obj.TagShortStr && k1.Tt == obj.TagLongStr {
			s1 := k1.Val.(*obj.LuaString)
			s2 := n2.KeyVal.(*obj.LuaString)
			return s1.Data == s2.Data
		}
		return false
	}
	switch n2.KeyTT {
	case obj.TagNil, obj.TagFalse, obj.TagTrue:
		return true
	case obj.TagInteger:
		return k1.Val.(int64) == n2.KeyVal.(int64)
	case obj.TagFloat:
		return k1.Val.(float64) == n2.KeyVal.(float64)
	case obj.TagShortStr:
		return k1.Val == n2.KeyVal // pointer equality for interned strings
	case obj.TagLongStr:
		s1 := k1.Val.(*obj.LuaString)
		s2 := n2.KeyVal.(*obj.LuaString)
		return s1.Data == s2.Data
	default:
		// Go func values are not comparable with ==; use reflect for those.
		if n2.KeyTT == obj.TagLightCFunc {
			return obj.LightCFuncEqual(k1.Val, n2.KeyVal)
		}
		return k1.Val == n2.KeyVal // identity for GC objects (pointers)
	}
}

// ---------------------------------------------------------------------------
// Hash part lookup
// ---------------------------------------------------------------------------

// getFromHashLoop searches the chain starting at position mp.
func getFromHashLoop(t *Table, key obj.TValue, mp int) (obj.TValue, bool) {
	idx := mp
	for {
		nd := &t.Nodes[idx]
		if equalKey(key, nd, false) {
			if !nodeIsEmpty(nd) {
				return nd.Val, true
			}
			return obj.Nil, false
		}
		nx := nd.Next
		if nx == 0 {
			return obj.Nil, false
		}
		idx += int(nx)
	}
}

// getFromHashDeadOk is like getFromHashLoop but accepts dead keys (for Next).
func getFromHashDeadOk(t *Table, key obj.TValue) (int, bool) {
	if len(t.Nodes) == 0 {
		return -1, false
	}
	mp := mainPosition(t, key)
	idx := mp
	for {
		nd := &t.Nodes[idx]
		if equalKey(key, nd, true) {
			return idx, true
		}
		nx := nd.Next
		if nx == 0 {
			return -1, false
		}
		idx += int(nx)
	}
}

// getIntFromHash searches the hash part for an integer key.
func getIntFromHash(t *Table, key int64) (obj.TValue, bool) {
	if len(t.Nodes) == 0 {
		return obj.Nil, false
	}
	hmask := (1 << t.LsizeNode) - 1
	idx := hashInt(key, hmask)
	for {
		nd := &t.Nodes[idx]
		if nd.KeyTT == obj.TagInteger && nd.KeyVal.(int64) == key {
			if !nodeIsEmpty(nd) {
				return nd.Val, true
			}
			return obj.Nil, false
		}
		nx := nd.Next
		if nx == 0 {
			return obj.Nil, false
		}
		idx += int(nx)
	}
}

// getStrFromHash searches the hash part for a short string key.
func getStrFromHash(t *Table, key *obj.LuaString) (obj.TValue, bool) {
	if len(t.Nodes) == 0 {
		return obj.Nil, false
	}
	hmask := (1 << t.LsizeNode) - 1
	idx := hashStr(key, hmask)
	for {
		nd := &t.Nodes[idx]
		if nd.KeyTT == obj.TagShortStr {
			ndKey := nd.KeyVal.(*obj.LuaString)
			if ndKey == key || ndKey.Data == key.Data {
				if !nodeIsEmpty(nd) {
					return nd.Val, true
				}
				return obj.Nil, false
			}
		}
		nx := nd.Next
		if nx == 0 {
			return obj.Nil, false
		}
		idx += int(nx)
	}
}

// ---------------------------------------------------------------------------
// Free slot management
// ---------------------------------------------------------------------------

func getFreePos(t *Table) int {
	for t.LastFree > 0 {
		t.LastFree--
		if keyIsNil(&t.Nodes[t.LastFree]) {
			return t.LastFree
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Insert key (Brent's variation)
// ---------------------------------------------------------------------------

// insertKey inserts a new key-value pair into the hash part.
// The key must NOT already exist as a live key.
// Returns true if successful, false if no free slot (caller must rehash).
func insertKey(t *Table, key obj.TValue, value obj.TValue) bool {
	if len(t.Nodes) == 0 {
		return false
	}
	mp := mainPosition(t, key)
	nd := &t.Nodes[mp]

	// Check if main position is taken.
	// Matches C Lua: !isempty(gval(mp)) — check VALUE, not key.
	// A dead key (value nil) means the slot is available for reuse.
	if !nodeIsEmpty(nd) {
		f := getFreePos(t)
		if f < 0 {
			return false
		}

		othern := mainPositionFromNode(t, nd)
		if othern != mp {
			// Colliding node is NOT in its main position — steal mp.
			// Walk the chain from othern to find the node pointing to mp.
			prev := othern
			for {
				nextIdx := prev + int(t.Nodes[prev].Next)
				if nextIdx == mp {
					break
				}
				prev = nextIdx
			}
			// Rechain: prev → f instead of prev → mp
			t.Nodes[prev].Next = int32(f - prev)
			// Copy mp to f
			t.Nodes[f] = t.Nodes[mp]
			// Fix f's Next (was relative to mp, now relative to f)
			if t.Nodes[mp].Next != 0 {
				t.Nodes[f].Next += int32(mp - f)
			}
			// Clear mp for the new key
			t.Nodes[mp].Next = 0
			t.Nodes[mp].Val = obj.Nil
			t.Nodes[mp].KeyTT = obj.TagNil
			t.Nodes[mp].KeyVal = nil
		} else {
			// Colliding node IS in its main position — put new key in f.
			if nd.Next != 0 {
				t.Nodes[f].Next = int32(mp+int(nd.Next)) - int32(f)
			} else {
				t.Nodes[f].Next = 0
			}
			nd.Next = int32(f - mp)
			mp = f
		}
	}

	// Set the key and value at position mp
	setNodeKey(&t.Nodes[mp], key)
	t.Nodes[mp].Val = value
	return true
}

// ---------------------------------------------------------------------------
// Rehash
// ---------------------------------------------------------------------------

func rehash(t *Table, extraKey obj.TValue) {
	var nums [32]uint
	var totalNA uint
	var total uint
	var hasDeleted bool

	total = 1 // count extra key
	if extraKey.Tt == obj.TagInteger {
		k := extraKey.Val.(int64)
		if ai := arrayIndex(k); ai > 0 {
			nums[ceilLog2(uint(ai))]++
			totalNA++
		}
	}

	// Count keys in hash part
	for i := range t.Nodes {
		nd := &t.Nodes[i]
		if nodeIsEmpty(nd) {
			if !keyIsNil(nd) {
				hasDeleted = true
			}
			continue
		}
		total++
		if nd.KeyTT == obj.TagInteger {
			k := nd.KeyVal.(int64)
			if ai := arrayIndex(k); ai > 0 {
				nums[ceilLog2(uint(ai))]++
				totalNA++
			}
		}
	}

	// Count keys in array part
	if len(t.Array) > 0 {
		asize := uint(len(t.Array))
		var i uint = 1
		for lg := uint8(0); lg < 32; lg++ {
			lim := uint(1) << lg
			if lim > asize {
				lim = asize
			}
			if i > lim {
				break
			}
			var count uint
			for ; i <= lim; i++ {
				if !t.Array[i-1].Tt.IsNil() {
					count++
				}
			}
			nums[lg] += count
			totalNA += count
			total += count
		}
	}

	// Compute optimal array size
	optimalASize, naInArray := computeSizes(nums[:], totalNA)

	// Hash part gets everything not in array
	nhsize := total - naInArray
	if hasDeleted {
		nhsize += nhsize / 4
	}
	// Ensure at least 1 hash slot if there are hash-bound keys
	if nhsize == 0 && total > naInArray {
		nhsize = 1
	}

	resizeTable(t, int(optimalASize), int(nhsize))
}

func computeSizes(nums []uint, totalNA uint) (uint, uint) {
	var a uint
	var na uint
	var optimal uint

	for i := uint(0); i < 32; i++ {
		twotoi := uint(1) << i
		// arrayXhash: is it worth putting these in array vs hash?
		// array slot costs 1 unit, hash slot costs ~3 units
		// So array is worth it if: twotoi <= totalNA * 3
		if twotoi > 0 && twotoi <= totalNA*3 {
			a += nums[i]
			if nums[i] > 0 && twotoi <= a*3 {
				optimal = twotoi
				na = a
			}
		} else {
			break
		}
	}
	return optimal, na
}

func arrayIndex(k int64) uint {
	if k >= 1 && k <= (1<<31) {
		return uint(k)
	}
	return 0
}

func resizeTable(t *Table, newASize, newHSize int) {
	oldArray := t.Array
	oldNodes := t.Nodes

	// Allocate new array
	var newArray []obj.TValue
	if newASize > 0 {
		newArray = make([]obj.TValue, newASize)
		for i := range newArray {
			newArray[i] = obj.Nil
		}
	}

	// Copy common elements from old array
	commonLen := len(oldArray)
	if commonLen > newASize {
		commonLen = newASize
	}
	for i := 0; i < commonLen; i++ {
		newArray[i] = oldArray[i]
	}

	// Set up new hash part
	initHashPart(t, newHSize)
	t.Array = newArray

	// Re-insert array elements that no longer fit
	if len(oldArray) > newASize {
		for i := newASize; i < len(oldArray); i++ {
			if !oldArray[i].Tt.IsNil() {
				key := obj.MakeInteger(int64(i + 1))
				insertKey(t, key, oldArray[i])
			}
		}
	}

	// Re-insert all hash entries from old hash part
	for i := range oldNodes {
		nd := &oldNodes[i]
		if nodeIsEmpty(nd) {
			continue
		}
		k := nodeKey(nd)
		if k.Tt == obj.TagInteger {
			ik := k.Val.(int64)
			if ik >= 1 && int(ik) <= newASize {
				t.Array[ik-1] = nd.Val
				continue
			}
		}
		insertKey(t, k, nd.Val)
	}
}

func initHashPart(t *Table, size int) {
	if size <= 0 {
		t.Nodes = nil
		t.LsizeNode = 0
		t.LastFree = 0
		return
	}
	lsize := ceilLog2(uint(size))
	if lsize > 30 {
		lsize = 30
	}
	actualSize := 1 << lsize
	t.Nodes = make([]Node, actualSize)
	t.LsizeNode = lsize
	t.LastFree = actualSize
	for i := range t.Nodes {
		t.Nodes[i].KeyTT = obj.TagNil
		t.Nodes[i].Val = obj.Nil
		t.Nodes[i].Next = 0
	}
}
