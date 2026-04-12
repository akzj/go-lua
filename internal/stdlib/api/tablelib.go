package api

import (
	luaapi "github.com/akzj/go-lua/internal/api/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
)

// ---------------------------------------------------------------------------
// Table library
// Reference: lua-master/ltablib.c
// ---------------------------------------------------------------------------

func auxGetN(L *luaapi.State, n int) int64 {
	L.CheckType(n, objectapi.TypeTable)
	return L.LenI(n)
}

func tabInsert(L *luaapi.State) int {
	e := auxGetN(L, 1) + 1 // first empty element
	switch L.GetTop() {
	case 2: // called with only 2 arguments: table, value
		// insert at end
	case 3: // table, pos, value
		pos := L.CheckInteger(2)
		L.ArgCheck(1 <= pos && pos <= e, 2, "position out of bounds")
		// shift elements up
		for i := e; i > pos; i-- {
			L.GetI(1, i-1)
			L.SetI(1, i)
		}
		e = pos
	default:
		L.Errorf("wrong number of arguments to 'insert'")
	}
	L.SetI(1, e)
	return 0
}

func tabRemove(L *luaapi.State) int {
	size := auxGetN(L, 1)
	pos := L.OptInteger(2, size)
	if pos != size {
		L.ArgCheck(1 <= pos && pos <= size, 2, "position out of bounds")
	}
	L.GetI(1, pos) // result = t[pos]
	// shift elements down
	for ; pos < size; pos++ {
		L.GetI(1, pos+1)
		L.SetI(1, pos)
	}
	L.PushNil()
	L.SetI(1, size) // t[size] = nil
	return 1
}

func tabMove(L *luaapi.State) int {
	f := L.CheckInteger(2)
	e := L.CheckInteger(3)
	t := L.CheckInteger(4)
	tt := 1 // destination table
	if !L.IsNoneOrNil(5) {
		tt = 5
	}
	L.CheckType(1, objectapi.TypeTable)
	if tt != 1 {
		L.CheckType(tt, objectapi.TypeTable)
	}
	if e >= f { // otherwise nothing to move
		n := e - f + 1
		L.ArgCheck(t <= 9007199254740991-n+1, 4, "destination wrap around")
		if t > f {
			for i := n - 1; i >= 0; i-- {
				L.GetI(1, f+i)
				L.SetI(tt, t+i)
			}
		} else {
			for i := int64(0); i < n; i++ {
				L.GetI(1, f+i)
				L.SetI(tt, t+i)
			}
		}
	}
	L.PushValue(tt) // return destination table
	return 1
}

func tabConcat(L *luaapi.State) int {
	sep := L.OptString(2, "")
	i := L.OptInteger(3, 1)
	last := L.OptInteger(4, L.LenI(1))

	var sb []string
	for ; i <= last; i++ {
		L.GetI(1, i)
		s, ok := L.ToString(-1)
		if !ok {
			L.Errorf("invalid value (table) at index %d in table for 'concat'", i)
		}
		sb = append(sb, s)
		L.Pop(1)
	}
	L.PushString(joinStrings(sb, sep))
	return 1
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for i := 1; i < len(ss); i++ {
		result += sep + ss[i]
	}
	return result
}

func tabPack(L *luaapi.State) int {
	n := L.GetTop()
	L.CreateTable(n, 1) // create result table
	L.Insert(1)         // put it at position 1
	for i := n; i >= 1; i-- {
		L.SetI(1, int64(i)) // t[i] = arg[i]
	}
	L.PushInteger(int64(n))
	L.SetField(1, "n")
	L.SetTop(1) // return table
	return 1
}

func tabUnpack(L *luaapi.State) int {
	L.CheckType(1, objectapi.TypeTable)
	i := L.OptInteger(2, 1)
	var e int64
	if L.IsNoneOrNil(3) {
		e = L.LenI(1)
	} else {
		e = L.CheckInteger(3)
	}
	if i > e {
		return 0 // empty range
	}
	n := e - i + 1
	if n <= 0 || !L.CheckStack(int(n)) {
		L.Errorf("too many results to unpack")
	}
	for ; i < e; i++ {
		L.GetI(1, i)
	}
	L.GetI(1, e) // last element
	return int(n)
}

// --- sort ---

func tabSort(L *luaapi.State) int {
	n := auxGetN(L, 1)
	hasComp := !L.IsNoneOrNil(2)
	if hasComp {
		L.CheckType(2, objectapi.TypeFunction)
	}
	L.SetTop(2) // ensure 2 slots (table + optional comparator)
	auxSort(L, 1, n, hasComp)
	return 0
}

func sortComp(L *luaapi.State, a, b int64, hasComp bool) bool {
	if hasComp { // use comparison function
		L.PushValue(2) // push comparator
		L.GetI(1, a)
		L.GetI(1, b)
		L.Call(2, 1)
		res := L.ToBoolean(-1)
		L.Pop(1)
		return res
	}
	// Default: use < operator
	L.GetI(1, a)
	L.GetI(1, b)
	res := L.Compare(-2, -1, luaapi.OpLT)
	L.Pop(2)
	return res
}

func auxSort(L *luaapi.State, lo, up int64, hasComp bool) {
	for lo < up {
		// small arrays use insertion sort
		if up-lo < 10 {
			for i := lo + 1; i <= up; i++ {
				for j := i; j > lo && sortComp(L, j, j-1, hasComp); j-- {
					// swap t[j] and t[j-1]
					L.GetI(1, j)
					L.GetI(1, j-1)
					L.SetI(1, j)
					L.SetI(1, j-1)
				}
			}
			return
		}
		// choose pivot as median of lo, mid, up
		mid := lo + (up-lo)/2
		// sort lo, mid, up
		if sortComp(L, mid, lo, hasComp) {
			swapI(L, lo, mid)
		}
		if sortComp(L, up, lo, hasComp) {
			swapI(L, lo, up)
		}
		if sortComp(L, up, mid, hasComp) {
			swapI(L, mid, up)
		}
		// pivot is at mid, move to up-1
		swapI(L, mid, up-1)
		pivot := up - 1
		i := lo
		j := up - 1
		for {
			i++
			for sortComp(L, i, pivot, hasComp) {
				i++
			}
			j--
			for sortComp(L, pivot, j, hasComp) {
				j--
			}
			if i >= j {
				break
			}
			swapI(L, i, j)
		}
		swapI(L, i, pivot)
		// recurse on smaller partition, iterate on larger
		if i-lo < up-i {
			auxSort(L, lo, i-1, hasComp)
			lo = i + 1
		} else {
			auxSort(L, i+1, up, hasComp)
			up = i - 1
		}
	}
}

func swapI(L *luaapi.State, i, j int64) {
	L.GetI(1, i)
	L.GetI(1, j)
	L.SetI(1, i)
	L.SetI(1, j)
}

// OpenTable opens the table library.
func OpenTable(L *luaapi.State) int {
	tabFuncs := map[string]luaapi.CFunction{
		"insert":  tabInsert,
		"remove":  tabRemove,
		"move":    tabMove,
		"concat":  tabConcat,
		"pack":    tabPack,
		"unpack":  tabUnpack,
		"sort":    tabSort,
	}
	L.NewLib(tabFuncs)
	return 1
}