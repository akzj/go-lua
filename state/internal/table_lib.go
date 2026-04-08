// Table library implementation for Lua 5.4/5.5
package internal

import (
	"fmt"
	"sort"

	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
	vm "github.com/akzj/go-lua/vm/api"
)

// =============================================================================
// Helpers
// =============================================================================

// checkTable verifies that stack[base+argn] is a table and returns the internal table.
func checkTable(stack []types.TValue, base int, argn int, fname string) tableapi.TableInterface {
	idx := base + argn
	if idx >= len(stack) || stack[idx] == nil || stack[idx].IsNil() {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (table expected, got no value)", argn, fname))
		return nil
	}
	if !stack[idx].IsTable() {
		luaErrorString(fmt.Sprintf("bad argument #%d to '%s' (table expected, got %s)", argn, fname, luaTypeName(stack[idx])))
		return nil
	}
	// tableWrapper is in the same package, access .tbl directly
	tv := stack[idx]
	if tw, ok := tv.(*tableWrapper); ok {
		return tw.tbl
	}
	// VM-internal TValues store tables as *table.TableImpl in GetValue().
	// Try extracting via the tableapi.TableInterface interface.
	if val := tv.GetValue(); val != nil {
		if tbl, ok := val.(tableapi.TableInterface); ok {
			return tbl
		}
	}
	return nil
}

// tableSeqLen returns the length of the array part of a table.
func tableSeqLen(tbl tableapi.TableInterface) types.LuaInteger {
	var n types.LuaInteger = 1
	for {
		val := tbl.GetInt(n)
		if val == nil || val.IsNil() {
			return n - 1
		}
		n++
	}
}

// =============================================================================
// table.insert(t, [pos,] value)
// =============================================================================

func btableInsert(stack []types.TValue, base int) int {
	nargs := len(stack) - base - 1
	if nargs < 2 {
		luaErrorString("bad argument #1 to 'table.insert' (table expected, got no value)")
		return 0
	}

	tbl := checkTable(stack, base, 1, "table.insert")
	if tbl == nil {
		return 0
	}

	if nargs == 2 {
		// table.insert(t, value) — append at end
		pos := tableSeqLen(tbl) + 1
		val := stack[base+2]
		tbl.SetInt(pos, val)
		return 0
	}

	// table.insert(t, pos, value)
	pos := stack[base+2]
	if !pos.IsInteger() {
		luaErrorString("bad argument #2 to 'table.insert' (number has no integer representation)")
		return 0
	}
	posInt := pos.GetInteger()
	val := stack[base+3]

	// Clamp pos to [1, #t+1]
	seqLen := tableSeqLen(tbl)
	if posInt < 1 {
		posInt = 1
	}
	if posInt > seqLen+1 {
		posInt = seqLen + 1
	}

	// Shift elements from pos to end by 1
	for i := seqLen; i >= posInt; i-- {
		src := tbl.GetInt(i)
		tbl.SetInt(i+1, src)
	}

	// Insert at pos
	tbl.SetInt(posInt, val)
	return 0
}

// =============================================================================
// table.remove(t, [pos])
// =============================================================================

func btableRemove(stack []types.TValue, base int) int {
	nargs := len(stack) - base - 1
	if nargs < 1 {
		luaErrorString("bad argument #1 to 'table.remove' (table expected, got no value)")
		return 0
	}

	tbl := checkTable(stack, base, 1, "table.remove")
	if tbl == nil {
		return 0
	}

	seqLen := tableSeqLen(tbl)
	if seqLen == 0 {
		stack[base] = types.NewTValueNil()
		return 1
	}

	var pos types.LuaInteger
	if nargs >= 2 && !stack[base+2].IsNil() {
		if stack[base+2].IsInteger() {
			pos = stack[base+2].GetInteger()
		} else {
			pos = seqLen
		}
	} else {
		pos = seqLen
	}

	// Clamp pos to [1, #t]
	if pos < 1 {
		pos = 1
	}
	if pos > seqLen {
		stack[base] = types.NewTValueNil()
		return 1
	}

	// Get removed value
	removed := tbl.GetInt(pos)

	// Shift elements down
	for i := pos; i < seqLen; i++ {
		src := tbl.GetInt(i + 1)
		tbl.SetInt(i, src)
	}

	// Clear last element
	tbl.SetInt(seqLen, types.NewTValueNil())

	// Return removed value
	if removed != nil {
		stack[base] = removed
	} else {
		stack[base] = types.NewTValueNil()
	}
	return 1
}

// =============================================================================
// table.concat(t, [sep, [i, [j]]])
// =============================================================================

func btableConcat(stack []types.TValue, base int) int {
	nargs := len(stack) - base - 1
	if nargs < 1 {
		luaErrorString("bad argument #1 to 'table.concat' (table expected, got no value)")
		return 0
	}

	tbl := checkTable(stack, base, 1, "table.concat")
	if tbl == nil {
		return 0
	}

	// Separator (default "")
	sep := ""
	if nargs >= 2 && !stack[base+2].IsNil() {
		sepVal := stack[base+2]
		if s, ok := sepVal.GetValue().(string); ok {
			sep = s
		} else {
			luaErrorString("bad argument #2 to 'table.concat' (string expected)")
			return 0
		}
	}

	// Start index (default 1)
	seqLen := tableSeqLen(tbl)
	i := types.LuaInteger(1)
	if nargs >= 3 && !stack[base+3].IsNil() {
		if stack[base+3].IsInteger() {
			i = stack[base+3].GetInteger()
		} else {
			luaErrorString("bad argument #3 to 'table.concat' (number has no integer representation)")
			return 0
		}
	}

	// End index (default #t)
	j := seqLen
	if nargs >= 4 && !stack[base+4].IsNil() {
		if stack[base+4].IsInteger() {
			j = stack[base+4].GetInteger()
		} else {
			luaErrorString("bad argument #4 to 'table.concat' (number has no integer representation)")
			return 0
		}
	}

	// Clamp to valid range
	if i < 1 {
		i = 1
	}
	if j > seqLen {
		j = seqLen
	}
	if i > j {
		stack[base] = types.NewTValueString("")
		return 1
	}

	// Build result string
	result := ""
	for k := i; k <= j; k++ {
		if k > i {
			result += sep
		}
		val := tbl.GetInt(k)
		if val == nil || val.IsNil() {
			luaErrorString("invalid value (nil) at index " + fmt.Sprintf("%d", k) + " in table for 'concat'")
			return 0
		}
		if !val.IsString() && !val.IsNumber() {
			luaErrorString("invalid value (" + luaTypeName(val) + ") at index " + fmt.Sprintf("%d", k) + " in table for 'concat'")
			return 0
		}
		s := fmt.Sprintf("%v", val.GetValue())
		result += s
	}

	stack[base] = types.NewTValueString(result)
	return 1
}

// =============================================================================
// table.sort(t, [comp])
// =============================================================================

func btableSort(stack []types.TValue, base int) int {
	nargs := len(stack) - base - 1
	if nargs < 1 {
		luaErrorString("bad argument #1 to 'table.sort' (table expected, got no value)")
		return 0
	}

	tbl := checkTable(stack, base, 1, "table.sort")
	if tbl == nil {
		return 0
	}

	seqLen := int(tableSeqLen(tbl))
	if seqLen <= 1 {
		return 0
	}

	// Collect values into a slice
	vals := make([]types.TValue, seqLen)
	for i := 0; i < seqLen; i++ {
		vals[i] = tbl.GetInt(types.LuaInteger(i + 1))
	}

	// Default comparator: compare numerically if both are numbers
	defaultComp := func(a, b int) bool {
		va, vb := vals[a], vals[b]
		vaNum := func() types.LuaNumber {
			if va.IsInteger() {
				return types.LuaNumber(va.GetInteger())
			}
			if va.IsFloat() {
				return va.GetFloat()
			}
			return 0
		}()
		vbNum := func() types.LuaNumber {
			if vb.IsInteger() {
				return types.LuaNumber(vb.GetInteger())
			}
			if vb.IsFloat() {
				return vb.GetFloat()
			}
			return 0
		}()
		// If both are numeric, compare numerically
		if (va.IsInteger() || va.IsFloat()) && (vb.IsInteger() || vb.IsFloat()) {
			return vaNum < vbNum
		}
		// Fallback to string comparison
		return fmt.Sprintf("%v", va.GetValue()) < fmt.Sprintf("%v", vb.GetValue())
	}

	// Comparator function
	comp := defaultComp

	// If comp provided, use it via the GoFunc bridge
	if nargs >= 2 && !stack[base+2].IsNil() {
		compFn := stack[base+2]
		origComp := defaultComp
		comp = func(a, b int) bool {
			if gf, ok := compFn.GetValue().(vm.GoFunc); ok {
				tempStack := []types.TValue{compFn, vals[a], vals[b]}
				gf(tempStack, 0)
				return tempStack[0].IsTrue()
			}
			return origComp(a, b)
		}
	}

	sort.Slice(vals, comp)

	// Write sorted values back to table
	for i := 0; i < seqLen; i++ {
		tbl.SetInt(types.LuaInteger(i+1), vals[i])
	}

	return 0
}

// =============================================================================
// table.move(t, f, e, dest [, t2])
// =============================================================================

func btableMove(stack []types.TValue, base int) int {
	nargs := len(stack) - base - 1
	if nargs < 4 {
		luaErrorString("not enough arguments to 'table.move'")
		return 0
	}

	srcTbl := checkTable(stack, base, 1, "table.move")
	if srcTbl == nil {
		return 0
	}

	f := stack[base+2].GetInteger()
	e := stack[base+3].GetInteger()
	dest := stack[base+4].GetInteger()

	destTbl := srcTbl
	if nargs >= 5 && !stack[base+5].IsNil() {
		destTbl = checkTable(stack, base, 5, "table.move")
		if destTbl == nil {
			return 0
		}
	}

	for i := f; i <= e; i++ {
		src := srcTbl.GetInt(i)
		destTbl.SetInt(dest+(i-f), src)
	}

	return 0
}

// =============================================================================
// table.pack(...) — pack varargs into table with .n field
// =============================================================================

func btablePack(stack []types.TValue, base int) int {
	nargs := types.LuaInteger(len(stack) - base - 1)

	newTbl := createModuleTable()

	for i := types.LuaInteger(1); i <= nargs; i++ {
		idx := base + int(i)
		if idx < len(stack) {
			newTbl.SetInt(i, stack[idx])
		}
	}

	nKey := types.NewTValueString("n")
	nVal := types.NewTValueInteger(nargs)
	newTbl.Set(nKey, nVal)

	stack[base] = &tableWrapper{tbl: newTbl}
	return 1
}

// =============================================================================
// table.create(narr, nrec) — Lua 5.5: create table with pre-allocated sizes
// =============================================================================

func btableCreate(stack []types.TValue, base int) int {
	// Lua 5.5: table.create(narr, nrec) creates a table with hints for
	// array and record parts. Our implementation just creates a new table.
	newTbl := createModuleTable()
	stack[base] = &tableWrapper{tbl: newTbl}
	return 1
}

// =============================================================================
// table.maxn(t) — find largest positive integer key (deprecated in 5.5)
// =============================================================================

func btableMaxn(stack []types.TValue, base int) int {
	nargs := len(stack) - base - 1
	if nargs < 1 {
		luaErrorString("bad argument #1 to 'table.maxn' (table expected, got no value)")
		return 0
	}

	tbl := checkTable(stack, base, 1, "table.maxn")
	if tbl == nil {
		return 0
	}

	// Probe integer keys up to 100000
	maxKey := types.LuaInteger(0)
	for i := types.LuaInteger(1); i <= 100000; i++ {
		val := tbl.GetInt(i)
		if val != nil && !val.IsNil() {
			maxKey = i
		}
	}

	stack[base] = types.NewTValueInteger(maxKey)
	return 1
}

// =============================================================================
// Register all table library functions
// =============================================================================

// registerTableLib registers all table.* functions in the table module table.
func registerTableLib(tbl tableapi.TableInterface) {
	functions := []struct {
		name string
		fn   vm.GoFunc
	}{
		{"concat", btableConcat},
		{"remove", btableRemove},
		{"insert", btableInsert},
		{"move", btableMove},
		{"sort", btableSort},
		{"pack", btablePack},
		{"create", btableCreate},
		{"maxn", btableMaxn},
	}

	for _, f := range functions {
		key := types.NewTValueString(f.name)
		val := &goFuncWrapper{fn: f.fn}
		tbl.Set(key, val)
	}
}
