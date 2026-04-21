// table_ops.go — Table access operations (GetTable, SetTable, GetField, SetField, Next, Len).
package api

import (
	"math"

	"github.com/akzj/go-lua/internal/gc"
	"github.com/akzj/go-lua/internal/metamethod"
	"github.com/akzj/go-lua/internal/object"

	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/table"
	"github.com/akzj/go-lua/internal/vm"
)

// ---------------------------------------------------------------------------
// Table operations
// ---------------------------------------------------------------------------

func (L *State) getTableVal(idx int) *table.Table {
	v := L.index2val(idx)
	if v.Tt == object.TagTable {
		return v.Obj.(*table.Table)
	}
	return nil
}

// GetTable pushes t[k] where t is at idx and k is at top. Pops k.
// apiGetWithIndex performs a table get with __index metamethod chain walking.
// Mirrors C Lua's luaV_gettable but without VM stack manipulation.
// Walks up to 20 levels of __index chain (table or function).
func apiGetWithIndex(L *State, t object.TValue, key object.TValue) object.TValue {
	const maxLoop = 20
	for loop := 0; loop < maxLoop; loop++ {
		if t.IsTable() {
			tbl := t.Obj.(*table.Table)
			val, found := tbl.Get(key)
			if found && !val.IsNil() {
				return val
			}
			// Key not found — check for __index metamethod
			mt := tbl.GetMetatable()
			if mt == nil {
				return object.Nil
			}
			indexStr := L.internStr("__index")
			tm, tmFound := mt.GetStr(indexStr)
			if !tmFound || tm.IsNil() {
				return object.Nil
			}
			if tm.IsTable() {
				// __index is a table — recurse into it
				t = tm
				continue
			}
			if tm.IsFunction() {
				// __index is a function — call it with (table, key)
				ls := L.ls()
				oldTop := ls.Top
				state.EnsureStack(ls, 4)
				ls.Stack[ls.Top].Val = tm
				ls.Top++
				ls.Stack[ls.Top].Val = t
				ls.Top++
				ls.Stack[ls.Top].Val = key
				ls.Top++
				vm.Call(ls, ls.Top-3, 1)
				result := ls.Stack[ls.Top-1].Val
				ls.Top = oldTop
				return result
			}
			// __index is not table or function — error
			return object.Nil
		}
		// Non-table value — check for type metatable __index
		// (userdata, etc.) For now, return nil
		return object.Nil
	}
	// Too many __index levels
	return object.Nil
}

func (L *State) GetTable(idx int) object.Type {
	ls := L.ls()
	t := L.index2val(idx)
	key := ls.Stack[ls.Top-1].Val
	ls.Top--

	val := apiGetWithIndex(L, *t, key)
	L.push(val)
	return val.Type()
}

// GetField pushes t[key] where t is at idx.
func (L *State) GetField(idx int, key string) object.Type {
	t := L.index2val(idx)
	ks := L.internStr(key)
	val := apiGetWithIndex(L, *t, object.MakeString(ks))
	L.push(val)
	return val.Type()
}

// GetI pushes t[n] where t is at idx.
// Mirrors lua_geti: handles __index metamethods for non-table types
// and for table keys that are not found.
func (L *State) GetI(idx int, n int64) object.Type {
	ls := L.ls()
	t := L.index2val(idx)
	if t.Tt == object.TagTable {
		tbl := t.Obj.(*table.Table)
		val, found := tbl.GetInt(n)
		if found && !val.IsNil() {
			L.push(val)
			return val.Type()
		}
		// Table key not found — fall through to metamethod
	}
	// Non-table or key not found: use FinishGet for __index metamethod chain.
	// FinishGet writes result to Stack[ra].
	ra := ls.Top
	state.EnsureStack(ls, 1)
	ls.Stack[ra].Val = object.Nil // default
	vm.FinishGet(ls, *t, object.MakeInteger(n), ra)
	// FinishGet already wrote result to Stack[ra]. Just advance Top.
	result := ls.Stack[ra].Val
	ls.Top = ra + 1
	return result.Type()
}

// GetGlobal pushes the value of global variable name.
func (L *State) GetGlobal(name string) object.Type {
	gt := vm.GetGlobalTable(L.ls())
	ks := L.internStr(name)
	val, found := gt.GetStr(ks)
	if found && !val.IsNil() {
		L.push(val)
		return val.Type()
	}
	L.push(object.Nil)
	return object.TypeNil
}

// SetTable does t[k] = v where t is at idx, k at top-1, v at top.
func (L *State) SetTable(idx int) {
	ls := L.ls()
	t := L.index2val(idx)
	if t.Tt == object.TagTable {
		tbl := t.Obj.(*table.Table)
		key := ls.Stack[ls.Top-2].Val
		val := ls.Stack[ls.Top-1].Val
		// Check for NaN key — C Lua raises error, not panic
		if key.IsFloat() && math.IsNaN(key.Float()) {
			L.Errorf("table index is NaN")
		}
		tbl.Set(key, val)
	}
	ls.Top -= 2
}

// SetTableMeta does t[k] = v with __newindex metamethod support.
// Used by testC's settable instruction. Separate from SetTable to avoid
// Go call stack overflow in deep metamethod chains (events.lua).
func (L *State) SetTableMeta(idx int) {
	ls := L.ls()
	t := L.index2val(idx)
	key := ls.Stack[ls.Top-2].Val
	val := ls.Stack[ls.Top-1].Val
	ls.Top -= 2
	vm.APISetTable(ls, *t, key, val)
}

// SetField does t[key] = v where t is at idx, v at top.
func (L *State) SetField(idx int, key string) {
	ls := L.ls()
	t := L.index2val(idx)
	if t.Tt == object.TagTable {
		tbl := t.Obj.(*table.Table)
		ks := L.internStr(key)
		val := ls.Stack[ls.Top-1].Val
		tbl.SetStr(ks, val)
	}
	ls.Top--
}

// SetI does t[n] = v where t is at idx, v at top.
func (L *State) SetI(idx int, n int64) {
	ls := L.ls()
	t := L.index2val(idx)
	val := ls.Stack[ls.Top-1].Val
	if t.Tt == object.TagTable {
		tbl := t.Obj.(*table.Table)
		// Fast path: if key already exists in table, do raw set (no metamethod).
		// Matches C Lua's luaV_fastseti in lua_seti.
		if _, found := tbl.GetInt(n); found {
			tbl.SetInt(n, val)
			ls.Top--
			return
		}
	}
	// Non-table or key not found in table: go through full metamethod chain.
	// FinishSet handles __newindex for tables and non-tables.
	key := object.MakeInteger(n)
	vm.FinishSet(ls, *t, key, val)
	ls.Top--
}

// SetGlobal pops a value and sets it as global variable name.
func (L *State) SetGlobal(name string) {
	ls := L.ls()
	gt := vm.GetGlobalTable(ls)
	ks := L.internStr(name)
	val := ls.Stack[ls.Top-1].Val
	gt.SetStr(ks, val)
	ls.Top--
}

// RawGet pushes t[k] without metamethods.
func (L *State) RawGet(idx int) object.Type {
	return L.GetTable(idx) // our GetTable already skips metamethods
}

// RawGetI pushes t[n] without metamethods.
func (L *State) RawGetI(idx int, n int64) object.Type {
	return L.GetI(idx, n)
}

// RawSet does t[k] = v without metamethods.
func (L *State) RawSet(idx int) {
	L.SetTable(idx)
}

// RawSetI does t[n] = v without metamethods.
func (L *State) RawSetI(idx int, n int64) {
	L.SetI(idx, n)
}

// CreateTable pushes a new table with pre-allocated space.
func (L *State) CreateTable(nArr, nRec int) {
	t := table.New(nArr, nRec)
	L.ls().Global.LinkGC(t) // V5: register in allgc chain
	size := t.EstimateBytes()
	t.GCHeader.ObjSize = size
	L.TrackAlloc(size)
	L.push(object.TValue{Tt: object.TagTable, Obj: t})

	// V5 GC sweep handles dealloc accounting — no AddCleanup needed.
	// Periodic GC is handled by checkPeriodicGC in the VM dispatch loop.
}

// NewTable pushes a new empty table.
func (L *State) NewTable() {
	L.CreateTable(0, 0)
}

// GetMetatable pushes the metatable of the value at idx.
func (L *State) GetMetatable(idx int) bool {
	v := L.index2val(idx)
	var mt *table.Table
	switch v.Tt {
	case object.TagTable:
		mt = v.Obj.(*table.Table).GetMetatable()
	case object.TagUserdata:
		if ud, ok := v.Obj.(*object.Userdata); ok {
			if tbl, ok := ud.MetaTable.(*table.Table); ok {
				mt = tbl
			}
		}
	default:
		// Check global type metatables
		ls := L.ls()
		tp := v.Type()
		if int(tp) < len(ls.Global.MT) {
			if tbl, ok := ls.Global.MT[tp].(*table.Table); ok {
				mt = tbl
			}
		}
	}
	if mt != nil {
		L.push(object.TValue{Tt: object.TagTable, Obj: mt})
		return true
	}
	return false
}

// SetMetatable pops a table and sets it as metatable for value at idx.
func (L *State) SetMetatable(idx int) {
	ls := L.ls()
	v := L.index2val(idx)
	var mt *table.Table
	mtVal := ls.Stack[ls.Top-1].Val
	if mtVal.Tt == object.TagTable {
		mt = mtVal.Obj.(*table.Table)
	}
	ls.Top--

	switch v.Tt {
	case object.TagTable:
		tbl := v.Obj.(*table.Table)
		tbl.SetMetatable(mt)
		// V5 GC: Move object from allgc to finobj if __gc detected.
		// Dealloc tracking is handled by V5 GC sweep.
		if mt != nil {
			g := ls.Global
			tmName := g.TMNames[metamethod.TM_GC]
			gcTM := metamethod.GetTM(mt, metamethod.TM_GC, tmName)
			if !gcTM.IsNil() {
				gc.CheckFinalizer(g, tbl)
			}
		}
		// Parse __mode from metatable for weak table support
		if mt != nil {
			modeName := ls.Global.TMNames[metamethod.TM_MODE]
			modeVal, found := mt.GetStr(modeName)
			if found && (modeVal.Tt == object.TagShortStr || modeVal.Tt == object.TagLongStr) {
				modeStr := modeVal.Obj.(*object.LuaString).Data
				var mode byte
				for _, c := range modeStr {
					if c == 'k' {
						mode |= table.WeakKey
					}
					if c == 'v' {
						mode |= table.WeakValue
					}
				}
				tbl.WeakMode = mode
			}
		} else {
			tbl.WeakMode = 0
		}
	case object.TagUserdata:
		if ud, ok := v.Obj.(*object.Userdata); ok {
			ud.MetaTable = mt
			// V5 GC: Move object from allgc to finobj if __gc detected.
			if mt != nil {
				g := ls.Global
				tmName := g.TMNames[metamethod.TM_GC]
				gcTM := metamethod.GetTM(mt, metamethod.TM_GC, tmName)
				if !gcTM.IsNil() {
					gc.CheckFinalizer(g, ud)
				}
			}
		}
	default:
		tp := v.Type()
		if int(tp) < len(ls.Global.MT) {
			ls.Global.MT[tp] = mt // mt is *table.Table, MT is [9]any — OK
		}
	}
}

// Next implements table traversal.
func (L *State) Next(idx int) bool {
	ls := L.ls()
	t := L.index2val(idx)
	if t.Tt != object.TagTable {
		return false
	}
	tbl := t.Obj.(*table.Table)
	key := ls.Stack[ls.Top-1].Val
	ls.Top--
	nextKey, nextVal, ok, err := tbl.Next(key)
	if err != nil {
		vm.RunError(ls, err.Error())
	}
	if ok {
		L.push(nextKey)
		L.push(nextVal)
		return true
	}
	return false
}

// Len pushes the length of the value at idx.
// Mirrors lua_len: calls luaV_objlen which handles __len metamethods.
func (L *State) Len(idx int) {
	ls := L.ls()
	v := L.index2val(idx)
	// Use ObjLen which handles metamethods for all types (including tables).
	// ObjLen writes result to Stack[ra].
	ra := ls.Top
	state.EnsureStack(ls, 1)
	vm.ObjLen(ls, ra, *v)
	// ObjLen already wrote result to Stack[ra]. Just advance Top.
	ls.Top = ra + 1
}

// RawEqual compares two values without metamethods.
func (L *State) RawEqual(idx1, idx2 int) bool {
	v1 := L.index2val(idx1)
	v2 := L.index2val(idx2)
	if v1.Tt != v2.Tt {
		return false
	}
	if v1.Obj == nil && v2.Obj == nil {
		// Both non-GC types with same tag — check numeric equality
		if v1.Tt == object.TagInteger || v1.Tt == object.TagFloat {
			return v1.N == v2.N
		}
		return true
	}
	// Light C functions: use interface data-word comparison (unique per
	// closure instance). reflect.Pointer returns the shared code address
	// which is identical for all closures of the same function literal.
	if v1.Tt == object.TagLightCFunc {
		return object.LightCFuncEqual(v1.Obj, v2.Obj)
	}
	return v1.Obj == v2.Obj
}

// Concat concatenates the n values at the top of the stack.
func (L *State) Concat(n int) {
	ls := L.ls()
	if n >= 2 {
		vm.Concat(ls, n)
	} else if n == 0 {
		L.PushString("")
	}
	// n == 1: value already on stack
}

// Arith performs an arithmetic operation.
func (L *State) Arith(op ArithOp) {
	ls := L.ls()
	// For unary ops (UNM, BNOT), both operands are the same (top of stack).
	// For binary ops, operands are top-2 and top-1.
	var p1, p2 object.TValue
	if op == OpUnm || op == OpBNot {
		p1 = ls.Stack[ls.Top-1].Val
		p2 = p1
	} else {
		p1 = ls.Stack[ls.Top-2].Val
		p2 = ls.Stack[ls.Top-1].Val
	}

	// lua_arith coerces strings to numbers (mirrors luaO_arith in C Lua).
	cp1 := arithCoerceToNumber(p1)
	cp2 := arithCoerceToNumber(p2)

	// Try raw arithmetic first
	result, ok := object.RawArith(int(op), cp1, cp2)
	if ok {
		if op == OpUnm || op == OpBNot {
			ls.Top--
		} else {
			ls.Top -= 2
		}
		ls.Stack[ls.Top].Val = result
		ls.Top++
		return
	}

	// Raw arithmetic failed — try metamethods (mirrors luaT_trybinTM).
	event := metamethod.TMS(int(metamethod.TM_ADD) + int(op))
	tm := metamethod.GetTMByObj(ls.Global, p1, event)
	if tm.IsNil() {
		tm = metamethod.GetTMByObj(ls.Global, p2, event)
	}
	if tm.IsNil() {
		if op == OpIDiv {
			vm.RunError(ls, "attempt to divide by zero")
		}
		if op == OpMod {
			vm.RunError(ls, "attempt to perform 'n%%0'")
		}
		vm.RunError(ls, "attempt to perform arithmetic on incompatible types")
	}

	// Call metamethod: tm(p1, p2) → result
	top := ls.Top
	ls.Stack[top].Val = tm
	ls.Stack[top+1].Val = p1
	ls.Stack[top+2].Val = p2
	ls.Top = top + 3
	vm.Call(ls, top, 1)
	tmResult := ls.Stack[top].Val
	ls.Top = top

	if op == OpUnm || op == OpBNot {
		ls.Top--
	} else {
		ls.Top -= 2
	}
	ls.Stack[ls.Top].Val = tmResult
	ls.Top++
}

// arithCoerceToNumber converts a string TValue to numeric if possible.
func arithCoerceToNumber(v object.TValue) object.TValue {
	if v.Tt == object.TagShortStr || v.Tt == object.TagLongStr {
		if s, ok := v.Obj.(*object.LuaString); ok {
			if tv, cvt := object.StringToNumber(s.Data); cvt {
				return tv
			}
		}
	}
	return v
}

// Compare compares two values.
func (L *State) Compare(idx1, idx2 int, op CompareOp) bool {
	// C Lua: lua_compare returns 0 for non-valid indices.
	if !L.isValidIndex(idx1) || !L.isValidIndex(idx2) {
		return false
	}
	v1 := L.index2val(idx1)
	v2 := L.index2val(idx2)
	switch op {
	case OpEQ:
		return vm.EqualObj(L.ls(), *v1, *v2)
	case OpLT:
		return vm.LessThan(L.ls(), *v1, *v2)
	case OpLE:
		return vm.LessEqual(L.ls(), *v1, *v2)
	}
	return false
}

// isValidIndex checks if an index refers to an acceptable stack position.
func (L *State) isValidIndex(idx int) bool {
	if idx == RegistryIndex {
		return true
	}
	ls := L.ls()
	ci := ls.CI
	if idx > 0 {
		return ci.Func+idx < ls.Top
	} else if idx > RegistryIndex {
		return ls.Top+idx >= ci.Func+1
	}
	return true // upvalue pseudo-index
}
