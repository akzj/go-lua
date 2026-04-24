package stdlib

import (
	"math"
	"strings"

	luaapi "github.com/akzj/go-lua/internal/api"
	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// Table library
// Reference: lua-master/ltablib.c
// ---------------------------------------------------------------------------

// Operations that an object must define to mimic a table.
// Matches C Lua's TAB_R, TAB_W, TAB_L constants in ltablib.c.
const (
	tabR  = 1 // read (__index)
	tabW  = 2 // write (__newindex)
	tabL  = 4 // length (__len)
	tabRW = tabR | tabW
)

// checkfield checks if the metatable (at stack depth n from top) has a
// given metamethod field. Returns true if found (non-nil). Pushes the
// result onto the stack (caller tracks count to pop).
// Matches C Lua's checkfield in ltablib.c.
func checkfield(L *luaapi.State, key string, n int) bool {
	L.PushString(key)
	return L.RawGet(-n) != object.TypeNil
}

// checktab checks that 'arg' either is a table or can behave like one
// (has a metatable with the required metamethods).
// Matches C Lua's checktab in ltablib.c.
func checktab(L *luaapi.State, arg int, what int) {
	tp := L.Type(arg)
	if tp == object.TypeTable {
		return
	}
	n := 1 // number of elements to pop
	ok := L.GetMetatable(arg) // must have metatable; pushes it
	if ok && what&tabR != 0 {
		n++
		ok = checkfield(L, "__index", n)
	}
	if ok && what&tabW != 0 {
		n++
		ok = checkfield(L, "__newindex", n)
	}
	if ok && what&tabL != 0 {
		// strings don't need '__len' to have a length
		if tp != object.TypeString {
			n++
			ok = checkfield(L, "__len", n)
		}
	}
	if ok {
		L.Pop(n) // pop metatable and tested metamethods
	} else {
		L.CheckType(arg, object.TypeTable) // force an error
	}
}

func auxGetN(L *luaapi.State, n int, what int) int64 {
	checktab(L, n, what|tabL)
	return L.LenI(n)
}

func tabInsert(L *luaapi.State) int {
	e := auxGetN(L, 1, tabRW) + 1 // first empty element
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
	size := auxGetN(L, 1, tabRW)
	pos := L.OptInteger(2, size)
	if pos != size {
		// C Lua uses unsigned: (lua_Unsigned)pos - 1u <= (lua_Unsigned)size
		// This allows pos in [1, size+1] — pos==size+1 is a no-op returning nil
		L.ArgCheck(1 <= pos && pos <= size+1, 2, "position out of bounds")
	}
	L.GetI(1, pos) // result = t[pos]
	// shift elements down
	for ; pos < size; pos++ {
		L.GetI(1, pos+1)
		L.SetI(1, pos)
	}
	L.PushNil()
	L.SetI(1, pos) // t[pos] = nil (pos was advanced to size by the loop)
	return 1
}

func tabMove(L *luaapi.State) int {
	const maxInt = int64(math.MaxInt64)
	f := L.CheckInteger(2)
	e := L.CheckInteger(3)
	t := L.CheckInteger(4)
	tt := 1 // destination table
	if !L.IsNoneOrNil(5) {
		tt = 5
	}
	checktab(L, 1, tabR)  // source needs read (__index)
	checktab(L, tt, tabW) // dest needs write (__newindex)
	if e >= f { // otherwise nothing to move
		L.ArgCheck(f > 0 || e < maxInt+f, 3, "too many elements to move")
		n := e - f + 1 // number of elements to move
		L.ArgCheck(t <= maxInt-n+1, 4, "destination wrap around")
		if t > e || t <= f || (tt != 1 && !L.RawEqual(1, tt)) {
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
	L.PushValue(tt) // return destination table
	return 1
}



func tabConcat(L *luaapi.State) int {
	last := auxGetN(L, 1, tabR)
	sep := L.OptString(2, "")
	i := L.OptInteger(3, 1)
	last = L.OptInteger(4, last)

	var sb []string
	for ; i <= last; i++ {
		L.GetI(1, i)
		s, ok := L.ToString(-1)
		if !ok {
			L.Errorf("invalid value (table) at index %d in table for 'concat'", i)
		}
		sb = append(sb, s)
		L.Pop(1)
		if i == last {
			break
		}
	}
	L.PushString(joinStrings(sb, sep))
	return 1
}

func joinStrings(ss []string, sep string) string {
	return strings.Join(ss, sep)
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
	L.CheckAny(1)
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
	// Use unsigned subtraction to avoid signed overflow (matches C Lua's tunpack).
	// C Lua checks n >= INT_MAX before lua_checkstack. We check against our MaxStack
	// (1_000_000) to avoid OOM from trying to allocate a huge Go slice.
	n := uint64(e) - uint64(i) // number of elements minus 1
	if n >= 1_000_000 || !L.CheckStack(int(n+1)) {
		L.Errorf("too many results to unpack")
	}
	n++ // now n = actual count
	for ; i < e; i++ {
		L.GetI(1, i)
	}
	L.GetI(1, e) // last element
	return int(n)
}

// --- sort ---

// tabCreate implements table.create(n [,m]).
// Creates a new table with pre-allocated array (n) and hash (m) slots.
// Lua 5.5 new function.
func tabCreate(L *luaapi.State) int {
	nArr := L.CheckInteger(1)
	nRec := L.OptInteger(2, 0)
	const intMax = int64(0x7FFFFFFF) // C INT_MAX — matches C Lua's check
	L.ArgCheck(nArr >= 0 && nArr <= intMax, 1, "out of range")
	L.ArgCheck(nRec >= 0 && nRec <= intMax, 2, "out of range")
	// C Lua also checks for table overflow (hash part too large)
	if nRec > 0 {
		const maxHash = 1 << 30
		if nRec > maxHash {
			L.PushString("table overflow")
			L.Error()
			return 0
		}
	}
	L.CreateTable(int(nArr), int(nRec))
	return 1
}

func tabSort(L *luaapi.State) int {
	n := auxGetN(L, 1, tabRW)
	L.ArgCheck(n < math.MaxInt32, 1, "array too big")
	hasComp := !L.IsNoneOrNil(2)
	if hasComp {
		L.CheckType(2, object.TypeFunction)
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
		// Sort elements lo, mid, up (median-of-three)
		if sortComp(L, up, lo, hasComp) {
			swapI(L, lo, up)
		}
		if up-lo == 1 { // only 2 elements
			return
		}
		mid := lo + (up-lo)/2
		// Sort lo, mid, up
		if sortComp(L, mid, lo, hasComp) {
			swapI(L, mid, lo)
		} else if sortComp(L, up, mid, hasComp) {
			swapI(L, mid, up)
		}
		if up-lo == 2 { // only 3 elements
			return
		}
		// Move pivot (mid) to up-1
		swapI(L, mid, up-1)
		// Partition [lo+1..up-2] around pivot at up-1
		i := lo
		j := up - 1
		for {
			i++
			for sortComp(L, i, up-1, hasComp) {
				if i >= up-1 {
					L.Errorf("invalid order function for sorting")
				}
				i++
			}
			j--
			for sortComp(L, up-1, j, hasComp) {
				if j < i {
					L.Errorf("invalid order function for sorting")
				}
				j--
			}
			if i >= j {
				break
			}
			swapI(L, i, j)
		}
		// Put pivot in final position
		swapI(L, i, up-1)
		// Recurse on smaller partition, iterate on larger
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

// ---------------------------------------------------------------------------
// Functional utilities: filter, map, reduce, keys, values, contains, slice, merge
// ---------------------------------------------------------------------------

// tabFilter implements table.filter(t, func(v, i) -> bool) -> table.
// Returns a new array containing only elements for which the callback returns true.
// Callback receives (value, index).
func tabFilter(L *luaapi.State) int {
	n := auxGetN(L, 1, tabR)
	L.CheckType(2, object.TypeFunction)
	L.CreateTable(int(n), 0) // result table at stack index 3
	resIdx := L.GetTop()
	var j int64 = 1 // result array index
	for i := int64(1); i <= n; i++ {
		L.PushValue(2) // push callback
		L.GetI(1, i)   // push t[i] (value)
		L.PushInteger(i) // push index
		L.Call(2, 1)     // call callback(value, index) -> result
		keep := L.ToBoolean(-1)
		L.Pop(1) // pop result
		if keep {
			L.GetI(1, i)      // push t[i]
			L.SetI(resIdx, j) // result[j] = t[i]
			j++
		}
	}
	L.PushValue(resIdx) // push result table
	return 1
}

// tabMap implements table.map(t, func(v, i) -> v2) -> table.
// Returns a new array with each element transformed by the callback.
// Callback receives (value, index) and should return one value.
func tabMap(L *luaapi.State) int {
	n := auxGetN(L, 1, tabR)
	L.CheckType(2, object.TypeFunction)
	L.CreateTable(int(n), 0) // result table at stack index 3
	resIdx := L.GetTop()
	for i := int64(1); i <= n; i++ {
		L.PushValue(2)   // push callback
		L.GetI(1, i)     // push t[i] (value)
		L.PushInteger(i) // push index
		L.Call(2, 1)     // call callback(value, index) -> result
		L.SetI(resIdx, i) // result[i] = callback result
	}
	L.PushValue(resIdx)
	return 1
}

// tabReduce implements table.reduce(t, func(acc, v, i) -> acc, init) -> value.
// Folds the array into a single value using the callback.
// Callback receives (accumulator, value, index).
func tabReduce(L *luaapi.State) int {
	n := auxGetN(L, 1, tabR)
	L.CheckType(2, object.TypeFunction)
	L.CheckAny(3) // initial value
	// Copy initial accumulator to top of stack (slot 4)
	L.PushValue(3)
	accIdx := L.GetTop()
	for i := int64(1); i <= n; i++ {
		L.PushValue(2)      // push callback
		L.PushValue(accIdx) // push current accumulator
		L.GetI(1, i)        // push t[i] (value)
		L.PushInteger(i)    // push index
		L.Call(3, 1)        // call callback(acc, value, index) -> new acc
		L.Replace(accIdx)   // replace accumulator with result
	}
	L.PushValue(accIdx)
	return 1
}

// tabKeys implements table.keys(t) -> table.
// Returns a new array of all keys in the table (order not guaranteed).
func tabKeys(L *luaapi.State) int {
	checktab(L, 1, tabR)
	L.CreateTable(0, 0) // result table at stack index 2
	resIdx := L.GetTop()
	var j int64 = 1
	L.PushNil() // initial key
	for L.Next(1) {
		// stack: ..., key, value
		L.Pop(1)               // pop value, keep key
		L.PushValue(-1)        // copy key (Next needs the original)
		L.SetI(resIdx, j)      // result[j] = key
		j++
	}
	L.PushValue(resIdx)
	return 1
}

// tabValues implements table.values(t) -> table.
// Returns a new array of all values in the table (order not guaranteed).
func tabValues(L *luaapi.State) int {
	checktab(L, 1, tabR)
	L.CreateTable(0, 0) // result table at stack index 2
	resIdx := L.GetTop()
	var j int64 = 1
	L.PushNil() // initial key
	for L.Next(1) {
		// stack: ..., key, value
		L.SetI(resIdx, j) // result[j] = value (pops value)
		j++
		// key remains for Next
	}
	L.PushValue(resIdx)
	return 1
}

// tabContains implements table.contains(t, value) -> bool.
// Returns true if any element in the array equals the given value.
// Uses raw equality for comparison.
func tabContains(L *luaapi.State) int {
	n := auxGetN(L, 1, tabR)
	L.CheckAny(2) // value to find
	for i := int64(1); i <= n; i++ {
		L.GetI(1, i) // push t[i]
		if L.RawEqual(-1, 2) {
			L.Pop(1)
			L.PushBoolean(true)
			return 1
		}
		L.Pop(1)
	}
	L.PushBoolean(false)
	return 1
}

// tabSlice implements table.slice(t, i [, j]) -> table.
// Returns a new array containing elements t[i] through t[j] (inclusive, 1-based).
// If j is omitted, defaults to #t. Negative indices count from end.
func tabSlice(L *luaapi.State) int {
	n := auxGetN(L, 1, tabR)
	i := L.CheckInteger(2)
	j := L.OptInteger(3, n)
	// Handle negative indices
	if i < 0 {
		i = n + i + 1
	}
	if j < 0 {
		j = n + j + 1
	}
	// Clamp to valid range
	if i < 1 {
		i = 1
	}
	if j > n {
		j = n
	}
	var count int
	if j >= i {
		count = int(j - i + 1)
	}
	L.CreateTable(count, 0) // result table
	resIdx := L.GetTop()
	var k int64 = 1
	for idx := i; idx <= j; idx++ {
		L.GetI(1, idx)
		L.SetI(resIdx, k)
		k++
	}
	L.PushValue(resIdx)
	return 1
}

// tabMerge implements table.merge(t1, t2, ...) -> table.
// Returns a new table that is a shallow merge of all input tables.
// Later tables override earlier ones for duplicate keys.
func tabMerge(L *luaapi.State) int {
	nArgs := L.GetTop()
	L.CreateTable(0, 0) // result table
	resIdx := L.GetTop()
	for arg := 1; arg <= nArgs; arg++ {
		checktab(L, arg, tabR)
		L.PushNil() // initial key for Next
		for L.Next(arg) {
			// stack: ..., key, value
			L.PushValue(-2)        // copy key
			L.PushValue(-2)        // copy value
			L.SetTable(resIdx)     // result[key] = value
			L.Pop(1)               // pop value, keep key for Next
		}
	}
	L.PushValue(resIdx)
	return 1
}

// OpenTable opens the table library.
func OpenTable(L *luaapi.State) int {
	tabFuncs := map[string]luaapi.CFunction{
		"create":   tabCreate,
		"insert":   tabInsert,
		"remove":   tabRemove,
		"move":     tabMove,
		"concat":   tabConcat,
		"pack":     tabPack,
		"unpack":   tabUnpack,
		"sort":     tabSort,
		"filter":   tabFilter,
		"map":      tabMap,
		"reduce":   tabReduce,
		"keys":     tabKeys,
		"values":   tabValues,
		"contains": tabContains,
		"slice":    tabSlice,
		"merge":    tabMerge,
	}
	L.NewLib(tabFuncs)
	return 1
}