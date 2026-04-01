// Package internal implements the Lua table library.
package internal

import (
	"fmt"

	tableapi "github.com/akzj/go-lua/lib/table/api"
	luaapi "github.com/akzj/go-lua/api"
)

// TableLib is the implementation of the Lua table library.
type TableLib struct{}

// NewTableLib creates a new TableLib instance.
func NewTableLib() tableapi.TableLib {
	return &TableLib{}
}

// LuaFunc is the function signature for table library functions.
type LuaFunc func(L tableapi.LuaAPI) int

// checkfield checks if the metatable has a required field.
func checkfield(L tableapi.LuaAPI, key string) bool {
	L.PushString(key)
	L.GetTable(-2)
	isNil := L.IsNil(-1)
	L.Pop()
	return !isNil
}

// typeError raises a type error.
func typeError(L tableapi.LuaAPI, arg int, msg string) {
	panic(fmt.Sprintf("bad argument #%d to '%s'", arg, msg))
}

// checktab validates that arg either is a table or can behave like one.
func checktab(L tableapi.LuaAPI, arg int, what int) {
	arg = L.AbsIndex(arg)
	tp := L.Type(arg)
	if tp != luaapi.LUA_TTABLE {
		if L.GetMetatable(arg) {
			ok := true
			if (what&tableapi.TAB_R) != 0 {
				if !checkfield(L, "__index") {
					ok = false
				}
				L.Pop()
			}
			if ok && (what&tableapi.TAB_W) != 0 {
				if !checkfield(L, "__newindex") {
					ok = false
				}
				L.Pop()
			}
			if ok && (what&tableapi.TAB_L) != 0 && tp != luaapi.LUA_TSTRING {
				if !checkfield(L, "__len") {
					ok = false
				}
				L.Pop()
			}

			L.Pop()

			if !ok {
				typeError(L, arg, "table")
			}
		} else {
			typeError(L, arg, "table")
		}
	}
}

// aux_getn returns the length of the table at index n.
func aux_getn(L tableapi.LuaAPI, n int, what int) int64 {
	checktab(L, n, what)
	return int64(L.RawLen(n))
}

// Open implements tableapi.TableLib.Open.
func (t *TableLib) Open(L tableapi.LuaAPI) int {
	L.CreateTable(0, 10)

	L.PushGoFunction(tableapi.LuaFunc(tblConcat))
	L.SetField(-2, "concat")
	L.PushGoFunction(tableapi.LuaFunc(tblInsert))
	L.SetField(-2, "insert")
	L.PushGoFunction(tableapi.LuaFunc(tblMove))
	L.SetField(-2, "move")
	L.PushGoFunction(tableapi.LuaFunc(tblPack))
	L.SetField(-2, "pack")
	L.PushGoFunction(tableapi.LuaFunc(tblUnpack))
	L.SetField(-2, "unpack")
	L.PushGoFunction(tableapi.LuaFunc(tblRemove))
	L.SetField(-2, "remove")
	L.PushGoFunction(tableapi.LuaFunc(tblSort))
	L.SetField(-2, "sort")
	L.PushGoFunction(tableapi.LuaFunc(tblGetn))
	L.SetField(-2, "getn")
	L.PushGoFunction(tableapi.LuaFunc(tblForeachi))
	L.SetField(-2, "foreachi")
	L.PushGoFunction(tableapi.LuaFunc(tblForeach))
	L.SetField(-2, "foreach")

	L.SetGlobal("table")
	return 1
}

// Ensure types implement LuaFunc
var _ tableapi.LuaFunc = tblInsert
var _ tableapi.LuaFunc = tblRemove
var _ tableapi.LuaFunc = tblMove
var _ tableapi.LuaFunc = tblConcat
var _ tableapi.LuaFunc = tblSort
var _ tableapi.LuaFunc = tblPack
var _ tableapi.LuaFunc = tblUnpack
var _ tableapi.LuaFunc = tblGetn
var _ tableapi.LuaFunc = tblForeachi
var _ tableapi.LuaFunc = tblForeach

// =============================================================================
// Table Functions
// =============================================================================

// tblInsert inserts a value at position in the table.
func tblInsert(L tableapi.LuaAPI) int {
	t := 1

	e := int(aux_getn(L, t, tableapi.TAB_RW))
	e++

	switch L.GetTop() {
	case 2:
		L.PushValue(-1)
		L.SetI(t, int64(e))
	case 3:
		pos, _ := L.ToInteger(2)
		if pos-1 < 0 || pos-1 >= int64(e) {
			typeError(L, 2, tableapi.ErrPositionOutOfBounds)
		}
		for i := e; i > int(pos); i-- {
			L.GetI(t, int64(i-1))
			L.SetI(t, int64(i))
		}
		L.PushValue(-1)
		L.SetI(t, pos)
	default:
		typeError(L, 2, "wrong number of arguments to 'insert'")
	}
	return 0
}

// tblRemove removes and returns the element at position.
func tblRemove(L tableapi.LuaAPI) int {
	t := 1

	size := int(aux_getn(L, t, tableapi.TAB_RW))

	if L.GetTop() >= 2 {
		pos, _ := L.ToInteger(2)
		if pos < 1 || pos > int64(size)+1 {
			typeError(L, 2, tableapi.ErrPositionOutOfBounds)
		}
		L.GetI(t, pos)
		if pos != int64(size) {
			for ; pos < int64(size); pos++ {
				L.GetI(t, pos+1)
				L.SetI(t, pos)
			}
		}
		L.PushNil()
		L.SetI(t, pos)
	} else {
		if size > 0 {
			L.GetI(t, int64(size))
			L.PushNil()
			L.SetI(t, int64(size))
		} else {
			L.PushNil()
		}
	}

	return 1
}

// tblMove moves elements from one table to another.
func tblMove(L tableapi.LuaAPI) int {
	f, _ := L.ToInteger(2)
	e, _ := L.ToInteger(3)
	t, _ := L.ToInteger(4)

	tt := 1
	if !L.IsNoneOrNil(5) {
		tt = 5
	}

	checktab(L, 1, tableapi.TAB_R)
	checktab(L, tt, tableapi.TAB_W)

	if e >= f {
		n := e - f + 1

		sameTable := L.Compare(1, tt, luaapi.LUA_OPEQ)
		copyIncreasing := t > e || t < f || !sameTable

		if copyIncreasing {
			for i := int64(0); i < n; i++ {
				L.GetI(1, f+i)
				L.SetI(tt, t+i)
			}
		} else {
			for i := n - 1; i >= 0; i-- {
				L.GetI(1, f+i)
				L.SetI(tt, t+i)
			}
		}
	}

	L.PushValue(tt)
	return 1
}

// tblConcat concatenates table elements into a string.
func tblConcat(L tableapi.LuaAPI) int {
	t := 1
	last := int(aux_getn(L, t, tableapi.TAB_R))

	sep := ""
	if L.GetTop() >= 2 {
		sep, _ = L.ToString(2)
	}

	i := int64(1)
	if L.GetTop() >= 3 {
		i, _ = L.ToInteger(3)
	}

	j := int64(last)
	if L.GetTop() >= 4 {
		j, _ = L.ToInteger(4)
	}

	var result string
	for idx := i; idx <= j; idx++ {
		L.GetI(t, idx)
		if !L.IsString(-1) {
			tp := L.Type(-1)
			typeError(L, t, fmt.Sprintf(tableapi.ErrInvalidValueAtIndex, L.TypeName(tp), idx))
		}
		val, _ := L.ToString(-1)
		if idx > i {
			result += sep
		}
		result += val
		L.Pop()
	}

	L.PushString(result)
	return 1
}

// tblSort sorts table elements in-place.
func tblSort(L tableapi.LuaAPI) int {
	t := 1
	n := int(aux_getn(L, t, tableapi.TAB_RW))

	if n > 1 {
		if n > (1 << 31) - 1 {
			typeError(L, 1, tableapi.ErrArrayTooBig)
		}
		if !L.IsNoneOrNil(2) && !L.IsFunction(2) {
			typeError(L, 2, "function")
		}
		sortImpl(L, t, 1, int64(n))
	}

	return 0
}

// sortComp compares a and b using comparator at stack position 2.
func sortComp(L tableapi.LuaAPI, t int, a, b int64) bool {
	if L.IsNil(2) {
		return L.Compare(int(a), int(b), luaapi.LUA_OPLT)
	}
	L.PushValue(2)
	L.PushValue(int(a - 1))
	L.PushValue(int(b - 2))
	L.Call(2, 1)
	result := L.ToBoolean(-1)
	L.Pop()
	return result
}

// set2 swaps elements at positions i and j in table t.
func set2(L tableapi.LuaAPI, t int, i, j int64) {
	L.GetI(t, i)
	L.GetI(t, j)
	L.SetI(t, j)
	L.SetI(t, i)
}

// partition performs quicksort partition.
func partition(L tableapi.LuaAPI, t int, lo, up int64) int64 {
	i := lo
	j := up - 1

	for {
		for {
			i++
			L.GetI(t, i)
			if !sortComp(L, t, i, up-1) {
				L.Pop()
				break
			}
			L.Pop()
			if i >= up-1 {
				typeError(L, 1, tableapi.ErrInvalidOrderFunction)
			}
		}

		for {
			L.GetI(t, j)
			if !sortComp(L, t, lo, j) {
				L.Pop()
				break
			}
			L.Pop()
			if j <= i {
				typeError(L, 1, tableapi.ErrInvalidOrderFunction)
			}
		}

		if j < i {
			L.Pop()
			set2(L, t, up-1, i)
			return i
		}

		set2(L, t, i, j)
	}
}

// sortImpl is the quicksort implementation.
func sortImpl(L tableapi.LuaAPI, t int, lo, up int64) {
	for lo < up {
		L.GetI(t, lo)
		L.GetI(t, up)
		if sortComp(L, t, up, lo) {
			set2(L, t, lo, up)
		} else {
			L.Pop()
			L.Pop()
		}

		if up-lo == 1 {
			return
		}

		p := (lo + up) / 2

		L.GetI(t, p)
		L.GetI(t, lo)
		if sortComp(L, t, p, lo) {
			set2(L, t, p, lo)
		} else {
			L.Pop()
			L.GetI(t, up)
			if sortComp(L, t, up, p) {
				set2(L, t, p, up)
			} else {
				L.Pop()
				L.Pop()
			}
		}

		if up-lo == 2 {
			return
		}

		L.GetI(t, p)
		L.GetI(t, up-1)
		set2(L, t, p, up-1)

		i := partition(L, t, lo, up)

		if i-lo < up-i {
			sortImpl(L, t, lo, i-1)
			lo = i + 1
		} else {
			sortImpl(L, t, i+1, up)
			up = i - 1
		}
	}
}

// tblPack packs variadic arguments into a table.
func tblPack(L tableapi.LuaAPI) int {
	n := L.GetTop()
	L.CreateTable(n, 1)
	L.Rotate(1, n)

	for i := n; i >= 1; i-- {
		L.SetI(1, int64(i))
	}
	L.PushInteger(int64(n))
	L.SetField(1, "n")

	return 1
}

// tblUnpack unpacks table elements to variadic values.
func tblUnpack(L tableapi.LuaAPI) int {
	t := 1
	length := int(aux_getn(L, t, tableapi.TAB_R))

	i := int64(1)
	if L.GetTop() >= 2 {
		i, _ = L.ToInteger(2)
	}

	j := int64(length)
	if L.GetTop() >= 3 {
		j, _ = L.ToInteger(3)
	}

	if i > j {
		return 0
	}

	n := int(j - i + 1)
	if n >= (1<<31)-1 || !L.CheckStack(n+1) {
		typeError(L, 1, "too many results to unpack")
	}

	for idx := i; idx <= j; idx++ {
		L.GetI(t, idx)
	}

	return n
}

// tblGetn returns the length of the table.
func tblGetn(L tableapi.LuaAPI) int {
	t := 1
	n := aux_getn(L, t, tableapi.TAB_L)
	L.PushInteger(n)
	return 1
}

// tblForeachi iterates over array part with integer keys.
func tblForeachi(L tableapi.LuaAPI) int {
	t := 1
	f := 2

	if !L.IsFunction(f) {
		typeError(L, 2, "function")
	}

	for i := int64(1); ; i++ {
		L.GetI(t, i)
		if L.IsNil(-1) {
			L.Pop()
			break
		}
		L.PushValue(f)
		L.PushInteger(i)
		L.Insert(-2)
		L.Call(2, 0)
	}

	return 0
}

// tblForeach iterates over all key-value pairs.
func tblForeach(L tableapi.LuaAPI) int {
	f := 2

	if !L.IsFunction(f) {
		typeError(L, 2, "function")
	}

	if !L.IsTable(1) {
		typeError(L, 1, "table")
	}

	L.PushNil()
	for L.Next(1) {
		L.PushValue(f)
		L.PushValue(-3)
		L.PushValue(-2)
		L.Call(2, 0)
	}

	return 0
}
