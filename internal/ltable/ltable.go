package ltable

import (
	"unsafe"

	"github.com/akzj/go-lua/internal/lmem"
	"github.com/akzj/go-lua/internal/lobject"
)

/*
** Table results
 */
const (
	HOK        = 0
	HNOTFOUND  = 1
	HNOTATABLE = 2
	HFIRSTNODE = 3
)

/*
** Dummy node for empty tables
 */
var dummyNode lobject.Node

func init() {
	lobject.SetNilValue(&dummyNode.IVal)
}

/*
** Create new table
 */
func New(L *lobject.LuaState) *lobject.Table {
	mem := lmem.NewObject(L, uint64(unsafe.Sizeof(lobject.Table{})), uint8(lobject.LUA_VTABLE))
	t := mem.(*lobject.Table)

	t.CommonHeader.Next = nil
	t.CommonHeader.Tt = uint8(lobject.LUA_VTABLE)
	t.CommonHeader.Marked = 0

	t.Flags = 0
	t.Metatable = nil
	t.Asize = 0
	t.Array = nil
	t.Gclist = nil
	t.Node = nil

	SetNodevector(L, t, 0)
	return t
}

/*
** Set node vector (hash array)
 */
func SetNodevector(L *lobject.LuaState, t *lobject.Table, size int) {
	if size == 0 {
		t.Node = nil
		t.Lsizenode = 0
	} else {
		t.Lsizenode = uint8(CeilLog2(size))
		t.Node = make([]lobject.Node, size)
		for i := range t.Node {
			lobject.SetNilValue(&t.Node[i].IVal)
		}
	}
}

/*
** Ceiling log2
 */
func CeilLog2(x int) int {
	n := 0
	pow := 1
	for pow < x {
		pow *= 2
		n++
	}
	return n
}

/*
** Size of node array (power of 2)
 */
func SizeNode(t *lobject.Table) int {
	if t.Node == nil {
		return 0
	}
	if t.Lsizenode == 0 {
		return len(t.Node)
	}
	return 1 << int(t.Lsizenode)
}

/*
** Hash function for integers
 */
func HashInt(t *lobject.Table, i lobject.LuaInteger) int {
	size := SizeNode(t)
	if size == 0 {
		return 0
	}
	return int(uint64(i) & uint64(size-1))
}

/*
** Get main position for key
 */
func MainPosition(t *lobject.Table, key *lobject.TValue) *lobject.Node {
	size := SizeNode(t)
	if size == 0 {
		return nil
	}

	switch {
	case lobject.TtIsInteger(key):
		idx := HashInt(t, lobject.IntValue(key))
		return &t.Node[idx]
	case lobject.TtIsShrString(key):
		ts := lobject.Gco2Ts(lobject.GcValue(key))
		idx := int(ts.Hash) & (size - 1)
		return &t.Node[idx]
	case lobject.TtIsFalse(key):
		return &t.Node[0]
	case lobject.TtIsTrue(key):
		return &t.Node[1]
	default:
		ptr := uintptr(0)
		if lobject.GcValue(key) != nil {
			ptr = uintptr(unsafe.Pointer(lobject.GcValue(key)))
		}
		return &t.Node[int(ptr)&(size-1)]
	}
}

/*
** Get free position in hash table
 */
func GetFreePos(t *lobject.Table) *lobject.Node {
	size := SizeNode(t)
	for i := 0; i < size; i++ {
		if lobject.IsKeyNil(&t.Node[i]) {
			return &t.Node[i]
		}
	}
	return nil
}

/*
** Check if two keys are equal
 */
func EqualKey(k1 *lobject.TValue, n2 *lobject.Node) bool {
	if int(n2.Key.Tt) != int(k1.Tt_) {
		return false
	}
	switch {
	case lobject.TtIsNil(k1):
		return true
	case lobject.TtIsInteger(k1):
		return lobject.IntValue(k1) == n2.Key.Value.I
	case lobject.TtIsShrString(k1):
		ts1 := lobject.Gco2Ts(lobject.GcValue(k1))
		ts2 := lobject.Gco2Ts(n2.Key.Value.Gc)
		return ts1 == ts2
	default:
		return lobject.GcValue(k1) == n2.Key.Value.Gc
	}
}

/*
** Generic search in hash chain
 */
func SearchGeneric(t *lobject.Table, key *lobject.TValue) *lobject.TValue {
	mp := MainPosition(t, key)
	if mp == nil {
		return nil
	}

	if EqualKey(key, mp) {
		return &mp.IVal
	}

	for mp.Key.Next != 0 {
		idx := mp.Key.Next - 1
		if idx >= SizeNode(t) {
			break
		}
		n := &t.Node[idx]
		if EqualKey(key, n) {
			return &n.IVal
		}
		mp = n
	}

	return nil
}

/*
** luaH_get - generic get (returns tag)
 */
func Get(t *lobject.Table, key *lobject.TValue) (int, *lobject.TValue) {
	if lobject.TtIsInteger(key) {
		idx := lobject.IntValue(key)
		if idx > 0 && int64(idx) <= int64(t.Asize) {
			return int(t.Array[idx-1].Tt_), &t.Array[idx-1]
		}
	}
	v := SearchGeneric(t, key)
	if v != nil {
		return int(v.Tt_), v
	}
	return int(lobject.LUA_VNIL), nil
}

/*
** luaH_getint - get by integer key (returns tag)
 */
func GetInt(t *lobject.Table, key lobject.LuaInteger) (int, *lobject.TValue) {
	if key > 0 && int64(key) <= int64(t.Asize) {
		return int(t.Array[key-1].Tt_), &t.Array[key-1]
	}

	idx := HashInt(t, key)
	n := &t.Node[idx]

	for {
		if lobject.IsKeyInteger(n) && lobject.KeyInt(n) == key {
			return int(n.IVal.Tt_), &n.IVal
		}
		if n.Key.Next == 0 {
			break
		}
		idx = n.Key.Next - 1
		if idx >= SizeNode(t) {
			break
		}
		n = &t.Node[idx]
	}

	return int(lobject.LUA_VNIL), nil
}

/*
** luaH_set - generic set
 */
func Set(L *lobject.LuaState, t *lobject.Table, key, value *lobject.TValue) {
	result := SetGeneric(L, t, key, value)
	if result != HOK {
		FinishSet(L, t, key, value, result)
	}
}

/*
** luaH_pset - pre-set (returns hint)
 */
func SetGeneric(L *lobject.LuaState, t *lobject.Table, key, value *lobject.TValue) int {
	if lobject.TtIsInteger(key) {
		idx := lobject.IntValue(key)
		if idx > 0 && int64(idx) <= int64(t.Asize) {
			t.Array[idx-1] = *value
			return HOK
		}
	}

	if SizeNode(t) == 0 {
		SetNodevector(L, t, 1)
	}

	mp := MainPosition(t, key)

	if EqualKey(key, mp) {
		mp.IVal = *value
		return HOK
	}

	for mp.Key.Next != 0 {
		idx := mp.Key.Next - 1
		if idx >= SizeNode(t) {
			break
		}
		n := &t.Node[idx]
		if EqualKey(key, n) {
			n.IVal = *value
			return HOK
		}
		mp = n
	}

	return HNOTFOUND
}

/*
** Finish set operation
 */
func FinishSet(L *lobject.LuaState, t *lobject.Table, key, value *lobject.TValue, result int) {
	if result == HOK {
		return
	}

	if lobject.TtIsNil(value) {
		return
	}

	if SizeNode(t) == 0 {
		SetNodevector(L, t, 1)
	}

	mp := MainPosition(t, key)

	if lobject.IsKeyNil(mp) {
		mp.IVal = *value
		mp.Key.Tt = key.Tt_
		mp.Key.Value = key.Value_
		mp.Key.Next = 0
		return
	}

	free := GetFreePos(t)
	if free == nil {
		Rehash(L, t, key)
		SetGeneric(L, t, key, value)
		return
	}

	*free = *mp
	mp.IVal = *value
	mp.Key.Tt = key.Tt_
	mp.Key.Value = key.Value_
	mp.Key.Next = 0
}

/*
** Rehash table
 */
func Rehash(L *lobject.LuaState, t *lobject.Table, ek *lobject.TValue) {
	nasize, nhsize := CountKeys(t, ek)
	newAsize := ComputeArraySize(nasize)
	newHsize := ComputeHashSize(nasize + nhsize)
	Resize(L, t, newAsize, newHsize)
}

/*
** Count keys in table
 */
func CountKeys(t *lobject.Table, ek *lobject.TValue) (int, int) {
	nasize := 0
	nhsize := 0

	for i := uint32(0); i < t.Asize; i++ {
		if !lobject.TtIsNil(&t.Array[i]) {
			nasize++
		}
	}

	size := SizeNode(t)
	for i := 0; i < size; i++ {
		if !lobject.IsKeyNil(&t.Node[i]) {
			nhsize++
		}
	}

	if ek != nil && lobject.TtIsInteger(ek) {
		nasize++
	}

	return nasize, nhsize
}

/*
** Compute optimal array size
 */
func ComputeArraySize(na int) int {
	if na == 0 {
		return 0
	}
	n := 1
	for n < na {
		n *= 2
	}
	return n
}

/*
** Compute optimal hash size
 */
func ComputeHashSize(n int) int {
	if n == 0 {
		return 0
	}
	n *= 2
	size := 1
	for size < n {
		size *= 2
	}
	return size
}

/*
** Resize table
 */
func Resize(L *lobject.LuaState, t *lobject.Table, nasize, nhsize int) {
	if nasize != int(t.Asize) {
		newArray := make([]lobject.TValue, nasize)
		oldSize := int(t.Asize)
		if nasize < oldSize {
			copy(newArray, t.Array[:nasize])
		} else if nasize > oldSize && oldSize > 0 {
			copy(newArray, t.Array)
		}
		t.Array = newArray
		t.Asize = uint32(nasize)
	}

	if nhsize != SizeNode(t) {
		oldNode := t.Node
		SetNodevector(L, t, nhsize)

		if oldNode != nil {
			for i := range oldNode {
				if !lobject.IsKeyNil(&oldNode[i]) {
					InsertNode(t, &oldNode[i])
				}
			}
		}
	}
}

/*
** Insert node into table
 */
func InsertNode(t *lobject.Table, node *lobject.Node) {
	if SizeNode(t) == 0 {
		SetNodevector(nil, t, 1)
	}

	mp := MainPosition(t, &node.IVal)

	if lobject.IsKeyNil(mp) {
		*mp = *node
		return
	}

	free := GetFreePos(t)
	if free != nil {
		*free = *node
	}
}

/*
** Free table
 */
func Free(L *lobject.LuaState, t *lobject.Table) {
	t.Node = nil
	t.Array = nil
}
