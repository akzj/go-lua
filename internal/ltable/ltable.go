package ltable

import (
	"fmt"
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
		// For small tables, just set Lsizenode=1 directly
		if size <= 2 {
			t.Lsizenode = 1
		} else {
			t.Lsizenode = uint8(CeilLog2(size))
		}
		actualSize := 1 << int(t.Lsizenode)
		t.Node = make([]lobject.Node, actualSize)
		for i := range t.Node {
			lobject.SetNilValue(&t.Node[i].IVal)
			t.Node[i].Key.Tt = uint8(lobject.LUA_TNIL)
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
		fmt.Printf("MainPosition: size=0, returning nil (table=%p)\n", t)
		return nil
	}

	var idx int
	switch {
	case lobject.TtIsInteger(key):
		idx = HashInt(t, lobject.IntValue(key))
	case lobject.TtIsShrString(key):
		ts := lobject.Gco2Ts(lobject.GcValue(key))
		idx = int(ts.Hash) & (size - 1)
	case lobject.TtIsFalse(key):
		idx = 0
	case lobject.TtIsTrue(key):
		idx = 1
	default:
		ptr := uintptr(0)
		if lobject.GcValue(key) != nil {
			ptr = uintptr(unsafe.Pointer(lobject.GcValue(key)))
		}
		idx = int(ptr) & (size - 1)
	}
	fmt.Printf("MainPosition: table=%p key.Tt_=%d idx=%d size=%d mp=&Node[%d]=%p mp.Key.Tt=%d\n", t, key.Tt_, idx, size, idx, &t.Node[idx], t.Node[idx].Key.Tt)
	return &t.Node[idx]
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
	fmt.Printf("EqualKey: k1.Tt_=%d n2.Key.Tt=%d\n", k1.Tt_, n2.Key.Tt)
	if int(n2.Key.Tt) != int(k1.Tt_) {
		fmt.Println("EqualKey: type mismatch")
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
		fmt.Printf("EqualKey string: ts1=%p ts2=%p\n", ts1, ts2)
		// Compare string content, not pointer
		return lobject.TestStringValue(ts1, ts2)
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
	fmt.Printf("DEBUG ltable.Set: key=%p val=%p t=%p SizeNode=%d\n", key, value, t, SizeNode(t))
	result := SetGeneric(L, t, key, value)
	fmt.Printf("DEBUG ltable.Set: SetGeneric returned %d\n", result)
	if result != HOK {
		fmt.Println("DEBUG ltable.Set: calling FinishSet")
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
	fmt.Println("AAAA FinishSet ENTRY")
	if result == HOK {
		fmt.Println("AAAA result==HOK, returning")
		return
	}

	if lobject.TtIsNil(value) {
		fmt.Println("AAAA value is nil, returning")
		return
	}

	fmt.Println("AAAA about to call SizeNode")
	if SizeNode(t) == 0 {
		fmt.Println("AAAA SizeNode==0, calling SetNodevector")
		SetNodevector(L, t, 1)
	}

	fmt.Println("AAAA about to call MainPosition")
	mp := MainPosition(t, key)
	
	fmt.Printf("AAAA FinishSet ENTRY: mp=%p mp.Key.Tt=%d\n", mp, mp.Key.Tt)
	
	// Check if main position is empty - directly compare Key.Tt to nil type
	if mp.Key.Tt == uint8(lobject.LUA_TNIL) {
		// Empty slot - store key and value
		fmt.Println("FinishSet: STORING IN EMPTY SLOT")
		mp.IVal = *value
		mp.Key.Tt = key.Tt_
		mp.Key.Value = key.Value_
		mp.Key.Next = 0
		fmt.Printf("FinishSet: after store, mp.Key.Tt=%d\n", mp.Key.Tt)
		return
	}

	fmt.Println("DEBUG FinishSet: slot not empty, using free list")
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
