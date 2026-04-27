// Hash part internals for Lua tables.
//
// Implements open-addressing hash table with Brent's variation for
// collision resolution. Reference: lua-master/ltable.c
package table

import (
	"math"
	"math/bits"
	"unsafe"

	"github.com/akzj/go-lua/internal/object"
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
func hashStr(s *object.LuaString, hmask int) int {
	return int(s.Hash()) & hmask
}

// ---------------------------------------------------------------------------
// Main position
// ---------------------------------------------------------------------------

// mainPosition returns the hash bucket index for a key.
func mainPosition(t *Table, key object.TValue) int {
	hmask := (1 << t.LsizeNode) - 1
	switch key.Tt {
	case object.TagInteger:
		return hashInt(key.N, hmask)
	case object.TagFloat:
		h := hashFloat(key.Float())
		return int(h % uint(hmask|1))
	case object.TagShortStr:
		return hashStr(key.Obj.(*object.LuaString), hmask)
	case object.TagLongStr:
		s := key.Obj.(*object.LuaString)
		return int(s.Hash()) & hmask
	case object.TagFalse:
		return 0 & hmask
	case object.TagTrue:
		return 1 & hmask
	case object.TagLightCFunc:
		// Use interface data word (unique per closure instance) for hashing.
		ptr := object.FuncDataPtr(key.Obj)
		return hashInt(int64(ptr), hmask)
	default:
		// For other types, use tag as hash (simple but correct).
		return int(key.Tt) & hmask
	}
}

// mainPositionFromNode returns the main position for a node's key.
// Dead keys (TagDeadKey) are handled by nodeKey which uses KeyOldTT.
func mainPositionFromNode(t *Table, nd *node) int {
	return mainPosition(t, nodeKey(nd))
}

// ---------------------------------------------------------------------------
// Node helpers
// ---------------------------------------------------------------------------

func nodeKey(nd *node) object.TValue {
	tt := nd.KeyTT
	if tt == object.TagDeadKey {
		// Dead keys: reconstruct using the preserved original tag.
		// This is needed for next() iteration and getFromHashDeadOk.
		tt = nd.KeyOldTT
	}
	return nodeKeyFromParts(tt, nd.KeyN, nd.KeyPtr)
}

// nodeKeyFromParts reconstructs a TValue from a tag and the split key fields.
func nodeKeyFromParts(tt object.Tag, keyN int64, keyPtr unsafe.Pointer) object.TValue {
	switch tt {
	case object.TagInteger:
		return object.MakeInteger(keyN)
	case object.TagFloat:
		return object.TValue{Tt: object.TagFloat, N: keyN}
	case object.TagShortStr, object.TagLongStr:
		s := (*object.LuaString)(keyPtr)
		return object.MakeString(s)
	case object.TagFalse:
		return object.False
	case object.TagTrue:
		return object.True
	case object.TagNil:
		return object.TValue{Tt: object.TagNil}
	default:
		// Rare collectable types (table-as-key, userdata-as-key, etc.)
		// Use the registered callback to reconstruct a properly-typed any.
		if object.ReconstructObj != nil && keyPtr != nil {
			obj := object.ReconstructObj(tt, keyPtr)
			return object.TValue{Tt: tt, Obj: obj}
		}
		return object.TValue{Tt: tt}
	}
}

func setNodeKey(nd *node, key object.TValue) {
	nd.KeyTT = key.Tt
	switch key.Tt {
	case object.TagInteger:
		nd.KeyN = key.N
		nd.KeyPtr = nil
	case object.TagFloat:
		nd.KeyN = key.N
		nd.KeyPtr = nil
	case object.TagShortStr, object.TagLongStr:
		nd.KeyPtr = unsafe.Pointer(key.Obj.(*object.LuaString))
		nd.KeyN = 0
	case object.TagFalse:
		nd.KeyN = 0
		nd.KeyPtr = nil
	case object.TagTrue:
		nd.KeyN = 1
		nd.KeyPtr = nil
	default:
		// For other collectable types (rare: tables as keys, userdata, closures, threads)
		// Extract the data pointer from the any interface.
		nd.KeyPtr = object.AnyDataPtr(key.Obj)
		nd.KeyN = 0
	}
}

func nodeIsEmpty(nd *node) bool {
	return nd.Val.Tt.IsNil()
}

func keyIsNil(nd *node) bool {
	return nd.KeyTT == object.TagNil
}

func keyIsDead(nd *node) bool {
	return nd.KeyTT == object.TagDeadKey
}

// ---------------------------------------------------------------------------
// Key equality
// ---------------------------------------------------------------------------

func equalKey(k1 object.TValue, n2 *node, deadOk bool) bool {
	if k1.Tt != n2.KeyTT {
		if deadOk && keyIsDead(n2) {
			// Dead key: compare by value identity using original tag
			return equalKeyDead(k1, n2)
		}
		// Cross-type string comparison (short vs long or long vs short)
		if k1.IsString() && (n2.KeyTT == object.TagShortStr || n2.KeyTT == object.TagLongStr) {
			s1 := k1.Obj.(*object.LuaString)
			s2 := (*object.LuaString)(n2.KeyPtr)
			return s1.Data == s2.Data
		}
		return false
	}
	switch n2.KeyTT {
	case object.TagNil, object.TagFalse, object.TagTrue:
		return true
	case object.TagInteger:
		return k1.N == n2.KeyN
	case object.TagFloat:
		return k1.Float() == math.Float64frombits(uint64(n2.KeyN))
	case object.TagShortStr:
		// Pointer equality for interned short strings
		return unsafe.Pointer(k1.Obj.(*object.LuaString)) == n2.KeyPtr
	case object.TagLongStr:
		s1 := k1.Obj.(*object.LuaString)
		s2 := (*object.LuaString)(n2.KeyPtr)
		return s1.Data == s2.Data
	default:
		if n2.KeyTT == object.TagLightCFunc {
			// Light C functions: compare by data pointer identity
			return object.AnyDataPtr(k1.Obj) == n2.KeyPtr
		}
		// Other GC objects (table, userdata, etc.): pointer identity
		return object.AnyDataPtr(k1.Obj) == n2.KeyPtr
	}
}

// equalKeyDead compares a live key against a dead node key.
// Dead keys preserve their original KeyN/KeyPtr and KeyOldTT.
func equalKeyDead(k1 object.TValue, n2 *node) bool {
	origTT := n2.KeyOldTT
	if k1.Tt != origTT {
		return false
	}
	switch origTT {
	case object.TagInteger:
		return k1.N == n2.KeyN
	case object.TagFloat:
		return k1.Float() == math.Float64frombits(uint64(n2.KeyN))
	case object.TagShortStr:
		return unsafe.Pointer(k1.Obj.(*object.LuaString)) == n2.KeyPtr
	case object.TagLongStr:
		s1 := k1.Obj.(*object.LuaString)
		s2 := (*object.LuaString)(n2.KeyPtr)
		return s1.Data == s2.Data
	case object.TagLightCFunc:
		return object.AnyDataPtr(k1.Obj) == n2.KeyPtr
	default:
		// Other collectable types: pointer identity
		return object.AnyDataPtr(k1.Obj) == n2.KeyPtr
	}
}

// ---------------------------------------------------------------------------
// Hash part lookup
// ---------------------------------------------------------------------------

// getFromHashLoop searches the chain starting at position mp.
func getFromHashLoop(t *Table, key object.TValue, mp int) (object.TValue, bool) {
	idx := mp
	for {
		nd := &t.Nodes[idx]
		if equalKey(key, nd, false) {
			if !nodeIsEmpty(nd) {
				return nd.Val, true
			}
			return object.Nil, false
		}
		nx := nd.Next
		if nx == 0 {
			return object.Nil, false
		}
		idx += int(nx)
	}
}

// getFromHashDeadOk is like getFromHashLoop but accepts dead keys (for Next).
func getFromHashDeadOk(t *Table, key object.TValue) (int, bool) {
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
func getIntFromHash(t *Table, key int64) (object.TValue, bool) {
	if len(t.Nodes) == 0 {
		return object.Nil, false
	}
	hmask := (1 << t.LsizeNode) - 1
	idx := hashInt(key, hmask)
	for {
		nd := &t.Nodes[idx]
		if nd.KeyTT == object.TagInteger && nd.KeyN == key {
			if !nodeIsEmpty(nd) {
				return nd.Val, true
			}
			return object.Nil, false
		}
		nx := nd.Next
		if nx == 0 {
			return object.Nil, false
		}
		idx += int(nx)
	}
}

// getStrFromHash searches the hash part for a short string key.
func getStrFromHash(t *Table, key *object.LuaString) (object.TValue, bool) {
	if len(t.Nodes) == 0 {
		return object.Nil, false
	}
	hmask := (1 << t.LsizeNode) - 1
	idx := hashStr(key, hmask)
	for {
		nd := &t.Nodes[idx]
		if nd.KeyTT == object.TagShortStr {
			ndKey := (*object.LuaString)(nd.KeyPtr)
			if ndKey == key || ndKey.Data == key.Data {
				if !nodeIsEmpty(nd) {
					return nd.Val, true
				}
				return object.Nil, false
			}
		}
		nx := nd.Next
		if nx == 0 {
			return object.Nil, false
		}
		idx += int(nx)
	}
}

// ---------------------------------------------------------------------------
// SetIfExists helpers — combined lookup + overwrite (no insertion)
// ---------------------------------------------------------------------------

// setInHashLoopIfExists walks the hash chain and overwrites the value if found.
func setInHashLoopIfExists(t *Table, key object.TValue, mp int, value object.TValue) bool {
	idx := mp
	for {
		nd := &t.Nodes[idx]
		if equalKey(key, nd, false) {
			if !nodeIsEmpty(nd) {
				nd.Val = value
				return true
			}
			return false
		}
		nx := nd.Next
		if nx == 0 {
			return false
		}
		idx += int(nx)
	}
}

// setIntInHashIfExists searches the hash part for an integer key and overwrites if found.
func setIntInHashIfExists(t *Table, key int64, value object.TValue) bool {
	if len(t.Nodes) == 0 {
		return false
	}
	hmask := (1 << t.LsizeNode) - 1
	idx := hashInt(key, hmask)
	for {
		nd := &t.Nodes[idx]
		if nd.KeyTT == object.TagInteger && nd.KeyN == key {
			if !nodeIsEmpty(nd) {
				nd.Val = value
				return true
			}
			return false
		}
		nx := nd.Next
		if nx == 0 {
			return false
		}
		idx += int(nx)
	}
}

// setStrInHashIfExists searches the hash part for a short string key and overwrites if found.
func setStrInHashIfExists(t *Table, key *object.LuaString, value object.TValue) bool {
	if len(t.Nodes) == 0 {
		return false
	}
	hmask := (1 << t.LsizeNode) - 1
	idx := hashStr(key, hmask)
	for {
		nd := &t.Nodes[idx]
		if nd.KeyTT == object.TagShortStr {
			ndKey := (*object.LuaString)(nd.KeyPtr)
			if ndKey == key || ndKey.Data == key.Data {
				if !nodeIsEmpty(nd) {
					nd.Val = value
					return true
				}
				return false
			}
		}
		nx := nd.Next
		if nx == 0 {
			return false
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
func insertKey(t *Table, key object.TValue, value object.TValue) bool {
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
			t.Nodes[mp].Val = object.Nil
			t.Nodes[mp].KeyTT = object.TagNil
			t.Nodes[mp].KeyN = 0
			t.Nodes[mp].KeyPtr = nil
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

func rehash(t *Table, extraKey object.TValue) {
	var nums [32]uint
	var totalNA uint
	var total uint
	var hasDeleted bool

	total = 1 // count extra key
	if extraKey.Tt == object.TagInteger {
		k := extraKey.N
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
		if nd.KeyTT == object.TagInteger {
			k := nd.KeyN
			if ai := arrayIndex(k); ai > 0 {
				nums[ceilLog2(uint(ai))]++
				totalNA++
			}
		}
	}

	// Matches C Lua (ltable.c rehash): if no integer keys were found in
	// the hash part or extra key that could migrate to the array, keep the
	// existing array size unchanged.  This prevents a pre-allocated but
	// empty array (e.g. table.create(1000)) from collapsing to size 0 when
	// only a hash key is added.
	var optimalASize uint
	var naInArray uint
	if totalNA == 0 {
		// No new keys to enter array part; keep it with the same size.
		optimalASize = uint(len(t.Array))
	} else {
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
		optimalASize, naInArray = computeSizes(nums[:], totalNA)
	}

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
	oldSize := t.GCHeader.ObjSize // capture before resize

	// Allocate new array
	var newArray []object.TValue
	if newASize > 0 {
		// make() returns zeroed memory; object.Nil is the zero value (TagNil=0x00)
		newArray = make([]object.TValue, newASize)
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
				key := object.MakeInteger(int64(i + 1))
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
		if k.Tt == object.TagInteger {
			ik := k.N
			if ik >= 1 && int(ik) <= newASize {
				t.Array[ik-1] = nd.Val
				continue
			}
		}
		insertKey(t, k, nd.Val)
	}

	// Update pre-computed size for GC accounting and track delta
	t.GCHeader.ObjSize = t.EstimateBytes()
	delta := t.GCHeader.ObjSize - oldSize
	if delta != 0 {
		t.SizeDelta += delta
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
	t.Nodes = make([]node, actualSize)
	t.LsizeNode = lsize
	t.LastFree = actualSize
	// No explicit zeroing needed: make() returns zeroed memory, and
	// all default values are zero: TagNil=0x00, Nil=TValue{Tt:0x00}, Next=0.
}
