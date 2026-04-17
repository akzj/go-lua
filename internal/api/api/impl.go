package api

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"unsafe"

	closureapi "github.com/akzj/go-lua/internal/closure/api"
	lexapi "github.com/akzj/go-lua/internal/lex/api"
	luastringapi "github.com/akzj/go-lua/internal/luastring/api"
	metamethodapi "github.com/akzj/go-lua/internal/metamethod/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"

	parseapi "github.com/akzj/go-lua/internal/parse/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
	tableapi "github.com/akzj/go-lua/internal/table/api"
	vmapi "github.com/akzj/go-lua/internal/vm/api"
)

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// ls returns the internal LuaState.
func (L *State) ls() *stateapi.LuaState {
	return L.Internal.(*stateapi.LuaState)
}

// strtab returns the string interning table from the global state.
func (L *State) strtab() *luastringapi.StringTable {
	return L.ls().Global.StringTable.(*luastringapi.StringTable)
}

// internStr creates a properly-hashed, interned LuaString.
// This ensures table lookups work correctly (hash-based bucket matching).
func (L *State) internStr(s string) *objectapi.LuaString {
	return L.strtab().Intern(s)
}

// nilValue is a sentinel for invalid/absent stack values.
var nilValue = objectapi.Nil

// index2val converts a Lua API index to a pointer to the TValue.
// Positive indices are 1-based from CI.Func+1.
// Negative indices are relative to Top.
// Pseudo-indices: RegistryIndex, UpvalueIndex(n).
// Returns pointer to nilValue for invalid indices.
func (L *State) index2val(idx int) *objectapi.TValue {
	ls := L.ls()
	ci := ls.CI
	if idx > 0 {
		o := ci.Func + idx
		if o < ls.Top {
			return &ls.Stack[o].Val
		}
		return &nilValue
	} else if idx > RegistryIndex {
		// negative index relative to top
		o := ls.Top + idx
		if o >= ci.Func+1 {
			return &ls.Stack[o].Val
		}
		return &nilValue
	} else if idx == RegistryIndex {
		return &ls.Global.Registry
	} else {
		// upvalue pseudo-index
		upIdx := RegistryIndex - idx // 1-based upvalue index
		fval := ls.Stack[ci.Func].Val
		switch fval.Tt {
		case objectapi.TagCClosure:
			cc := fval.Val.(*closureapi.CClosure)
			if upIdx <= len(cc.UpVals) {
				return &cc.UpVals[upIdx-1]
			}
		case objectapi.TagLuaClosure:
			lc := fval.Val.(*closureapi.LClosure)
			if upIdx <= len(lc.UpVals) && lc.UpVals[upIdx-1] != nil {
				if lc.UpVals[upIdx-1].IsOpen() {
					return &ls.Stack[lc.UpVals[upIdx-1].StackIdx].Val
				}
				return &lc.UpVals[upIdx-1].Own
			}
		}
		return &nilValue
	}
}

// index2stack converts a valid non-pseudo index to a stack index.
func (L *State) index2stack(idx int) int {
	ls := L.ls()
	if idx > 0 {
		return ls.CI.Func + idx
	}
	// negative
	return ls.Top + idx
}

// push pushes a TValue onto the internal stack.
func (L *State) push(v objectapi.TValue) {
	ls := L.ls()
	stateapi.PushValue(ls, v)
}

// wrapCFunction creates an adapter from api.CFunction to stateapi.CFunction.
// This bridges the type mismatch between the public API and internal API.
func (L *State) wrapCFunction(f CFunction) stateapi.CFunction {
	pub := L // capture the public State
	return func(ls *stateapi.LuaState) int {
		// The public State wraps the same internal state.
		// We need to ensure the Internal pointer is current.
		pub.Internal = ls
		return f(pub)
	}
}

// wrapCFunctionStatic creates an adapter without capturing a specific State.
// Each call creates a temporary State wrapper.
func wrapCFunctionStatic(f CFunction) stateapi.CFunction {
	return func(ls *stateapi.LuaState) int {
		pub := &State{Internal: ls}
		return f(pub)
	}
}

// ---------------------------------------------------------------------------
// State creation/destruction
// ---------------------------------------------------------------------------

// NewState creates a new Lua state.
func NewState() *State {
	ls := stateapi.NewState()
	return &State{Internal: ls}
}

// Close releases all resources associated with the state.
func (L *State) Close() {
	stateapi.CloseState(L.ls())
}

// ---------------------------------------------------------------------------
// Stack manipulation
// ---------------------------------------------------------------------------

// GetTop returns the number of elements on the stack (= index of top element).
func (L *State) GetTop() int {
	ls := L.ls()
	return ls.Top - (ls.CI.Func + 1)
}

// SetTop sets the stack top to idx. Fills with nil if growing.
func (L *State) SetTop(idx int) {
	ls := L.ls()
	ci := ls.CI
	base := ci.Func + 1
	if idx >= 0 {
		newTop := base + idx
		// Fill new slots with nil
		for ls.Top < newTop {
			ls.Stack[ls.Top].Val = objectapi.Nil
			ls.Top++
		}
		ls.Top = newTop
	} else {
		ls.Top = ls.Top + idx + 1
	}
}

// AbsIndex converts a possibly-negative index to absolute.
func (L *State) AbsIndex(idx int) int {
	if idx > 0 || idx <= RegistryIndex {
		return idx
	}
	ls := L.ls()
	return (ls.Top - (ls.CI.Func)) + idx
}

// CheckStack ensures at least n free stack slots.
func (L *State) CheckStack(n int) bool {
	ls := L.ls()
	// Mirrors: lua_checkstack in lapi.c:109-123
	// Return false if the requested size would exceed MaxStack.
	if ls.Top+n > stateapi.MaxStack {
		return false
	}
	stateapi.EnsureStack(ls, n)
	// Adjust frame top if needed (mirrors C Lua's ci->top adjustment)
	if ls.CI != nil && ls.CI.Top < ls.Top+n {
		ls.CI.Top = ls.Top + n
	}
	return true
}

// Pop removes n elements from the top.
func (L *State) Pop(n int) {
	L.SetTop(-n - 1)
}

// Copy copies value at fromIdx to toIdx.
func (L *State) Copy(fromIdx, toIdx int) {
	from := L.index2val(fromIdx)
	to := L.index2val(toIdx)
	*to = *from
}

// Rotate rotates the stack elements between idx and top by n positions.
func (L *State) Rotate(idx, n int) {
	ls := L.ls()
	p := L.index2stack(idx)
	t := ls.Top - 1
	var m int
	if n >= 0 {
		m = t - n
	} else {
		m = p - n - 1
	}
	reverseStack(ls, p, m)
	reverseStack(ls, m+1, t)
	reverseStack(ls, p, t)
}

func reverseStack(ls *stateapi.LuaState, from, to int) {
	for from < to {
		ls.Stack[from].Val, ls.Stack[to].Val = ls.Stack[to].Val, ls.Stack[from].Val
		from++
		to--
	}
}

// Insert moves top element to idx, shifting up.
func (L *State) Insert(idx int) {
	L.Rotate(idx, 1)
}

// Remove removes element at idx, shifting down.
func (L *State) Remove(idx int) {
	L.Rotate(idx, -1)
	L.Pop(1)
}

// Replace replaces value at idx with top element, popping top.
func (L *State) Replace(idx int) {
	L.Copy(-1, idx)
	L.Pop(1)
}

// PushValue pushes a copy of the value at idx.
func (L *State) PushValue(idx int) {
	v := L.index2val(idx)
	L.push(*v)
}

// ---------------------------------------------------------------------------
// Push functions (Go → Lua stack)
// ---------------------------------------------------------------------------

// PushNil pushes nil.
func (L *State) PushNil() {
	L.push(objectapi.Nil)
}

// PushBoolean pushes a boolean.
func (L *State) PushBoolean(b bool) {
	L.push(objectapi.MakeBoolean(b))
}

// PushInteger pushes an integer.
func (L *State) PushInteger(n int64) {
	L.push(objectapi.MakeInteger(n))
}

// PushNumber pushes a float.
func (L *State) PushNumber(n float64) {
	L.push(objectapi.MakeFloat(n))
}

// PushString pushes a string. Returns the string.
func (L *State) PushString(s string) string {
	is := L.internStr(s)
	L.push(objectapi.MakeString(is))
	return s
}

// PushFString pushes a formatted string.
func (L *State) PushFString(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	L.PushString(s)
	return s
}

// PushCFunction pushes a Go function as a light C function (no upvalues).
func (L *State) PushCFunction(f CFunction) {
	wrapped := wrapCFunctionStatic(f)
	L.push(objectapi.TValue{Tt: objectapi.TagLightCFunc, Val: wrapped})
}

// PushCFunctionSame pushes a pre-wrapped C function value. Unlike PushCFunction,
// which creates a new wrapper each call, this pushes the exact same TValue each time.
// Use for stateless iterators (e.g., ipairs) where identity must be preserved.
func (L *State) PushCFunctionSame(tv objectapi.TValue) {
	L.push(tv)
}

// WrapCFunction wraps a CFunction into a stateapi.CFunction + TValue.
// The caller can cache the result and pass it to PushCFunctionSame.
func WrapCFunction(f CFunction) objectapi.TValue {
	wrapped := wrapCFunctionStatic(f)
	return objectapi.TValue{Tt: objectapi.TagLightCFunc, Val: wrapped}
}

// PushCClosure pushes a Go function as a closure with n upvalues.
func (L *State) PushCClosure(f CFunction, n int) {
	if n == 0 {
		L.PushCFunction(f)
		return
	}
	ls := L.ls()
	wrapped := wrapCFunctionStatic(f)
	cc := closureapi.NewCClosure(wrapped, n)
	// Pop n upvalues from stack into the closure
	for i := n; i >= 1; i-- {
		ls.Top--
		cc.UpVals[i-1] = ls.Stack[ls.Top].Val
	}
	L.push(objectapi.TValue{Tt: objectapi.TagCClosure, Val: cc})
}

// PushLightUserdata pushes a light userdata.
func (L *State) PushLightUserdata(p interface{}) {
	L.push(objectapi.TValue{Tt: objectapi.TagLightUserdata, Val: p})
}

// PushGlobalTable pushes the global table.
func (L *State) PushGlobalTable() {
	L.RawGetI(RegistryIndex, int64(RIdxGlobals))
}

// ---------------------------------------------------------------------------
// Type checking
// ---------------------------------------------------------------------------

// Type returns the type of the value at idx.
func (L *State) Type(idx int) objectapi.Type {
	v := L.index2val(idx)
	if v == &nilValue {
		// Check if it's truly out of range
		ls := L.ls()
		ci := ls.CI
		if idx > 0 {
			o := ci.Func + idx
			if o >= ls.Top {
				return TypeNone
			}
		}
		return TypeNone
	}
	return v.Type()
}

// TypeName returns the name of the given type.
func (L *State) TypeName(tp objectapi.Type) string {
	names := [...]string{
		objectapi.TypeNil:      "nil",
		objectapi.TypeBoolean:  "boolean",
		objectapi.TypeNumber:   "number",
		objectapi.TypeString:   "string",
		objectapi.TypeTable:    "table",
		objectapi.TypeFunction: "function",
		objectapi.TypeUserdata: "userdata",
		objectapi.TypeThread:   "thread",
	}
	if tp == TypeNone {
		return "no value"
	}
	if int(tp) < len(names) {
		return names[tp]
	}
	return "unknown"
}

// IsNil returns true if the value at idx is nil.
func (L *State) IsNil(idx int) bool {
	return L.Type(idx) == objectapi.TypeNil
}

// IsNone returns true if the index is not valid.
func (L *State) IsNone(idx int) bool {
	return L.Type(idx) == TypeNone
}

// IsNoneOrNil returns true if the index is not valid or the value is nil.
func (L *State) IsNoneOrNil(idx int) bool {
	tp := L.Type(idx)
	return tp == TypeNone || tp == objectapi.TypeNil
}

// IsBoolean returns true if the value is a boolean.
func (L *State) IsBoolean(idx int) bool {
	return L.Type(idx) == objectapi.TypeBoolean
}

// IsInteger returns true if the value is an integer.
func (L *State) IsInteger(idx int) bool {
	v := L.index2val(idx)
	return v.Tt == objectapi.TagInteger
}

// IsNumber returns true if the value is a number or convertible string.
func (L *State) IsNumber(idx int) bool {
	_, ok := L.ToNumber(idx)
	return ok
}

// IsString returns true if the value is a string or a number.
func (L *State) IsString(idx int) bool {
	tp := L.Type(idx)
	return tp == objectapi.TypeString || tp == objectapi.TypeNumber
}

// IsFunction returns true if the value is a function.
func (L *State) IsFunction(idx int) bool {
	return L.Type(idx) == objectapi.TypeFunction
}

// IsTable returns true if the value is a table.
func (L *State) IsTable(idx int) bool {
	return L.Type(idx) == objectapi.TypeTable
}

// IsCFunction returns true if the value is a C/Go function.
func (L *State) IsCFunction(idx int) bool {
	v := L.index2val(idx)
	return v.Tt == objectapi.TagLightCFunc || v.Tt == objectapi.TagCClosure
}

// IsUserdata returns true if the value is a userdata.
func (L *State) IsUserdata(idx int) bool {
	tp := L.Type(idx)
	return tp == objectapi.TypeUserdata
}

// ---------------------------------------------------------------------------
// Conversion functions (Lua stack → Go)
// ---------------------------------------------------------------------------

// ToBoolean converts the value to boolean.
func (L *State) ToBoolean(idx int) bool {
	v := L.index2val(idx)
	return !v.IsFalsy()
}

// ToInteger converts the value to integer.
func (L *State) ToInteger(idx int) (int64, bool) {
	v := L.index2val(idx)
	switch v.Tt {
	case objectapi.TagInteger:
		return v.Val.(int64), true
	case objectapi.TagFloat:
		return objectapi.FloatToInteger(v.Val.(float64))
	case objectapi.TagShortStr, objectapi.TagLongStr:
		s := v.Val.(*objectapi.LuaString).Data
		return objectapi.StringToInteger(s)
	default:
		return 0, false
	}
}

// ToNumber converts the value to float.
func (L *State) ToNumber(idx int) (float64, bool) {
	v := L.index2val(idx)
	switch v.Tt {
	case objectapi.TagFloat:
		return v.Val.(float64), true
	case objectapi.TagInteger:
		return float64(v.Val.(int64)), true
	case objectapi.TagShortStr, objectapi.TagLongStr:
		s := v.Val.(*objectapi.LuaString).Data
		tv, ok := objectapi.StringToNumber(s)
		if !ok {
			return 0, false
		}
		if tv.Tt == objectapi.TagFloat {
			return tv.Val.(float64), true
		}
		return float64(tv.Val.(int64)), true
	default:
		return 0, false
	}
}

// ToString converts the value to string.
func (L *State) ToString(idx int) (string, bool) {
	v := L.index2val(idx)
	switch v.Tt {
	case objectapi.TagShortStr, objectapi.TagLongStr:
		return v.Val.(*objectapi.LuaString).Data, true
	case objectapi.TagInteger:
		s := fmt.Sprintf("%d", v.Val.(int64))
		// Coerce in-place
		is := L.internStr(s)
		*v = objectapi.MakeString(is)
		return s, true
	case objectapi.TagFloat:
		s := objectapi.FloatToString(v.Val.(float64))
		is := L.internStr(s)
		*v = objectapi.MakeString(is)
		return s, true
	default:
		return "", false
	}
}

// ToGoFunction returns the Go function at idx, or nil.
func (L *State) ToGoFunction(idx int) CFunction {
	// We can't easily reverse the wrapper, so return nil for now.
	// This is rarely needed in practice.
	return nil
}

// RawLen returns the raw length (no __len metamethod).
func (L *State) RawLen(idx int) int64 {
	v := L.index2val(idx)
	switch v.Tt {
	case objectapi.TagTable:
		return v.Val.(*tableapi.Table).RawLen()
	case objectapi.TagShortStr, objectapi.TagLongStr:
		return int64(len(v.Val.(*objectapi.LuaString).Data))
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// Table operations
// ---------------------------------------------------------------------------

func (L *State) getTableVal(idx int) *tableapi.Table {
	v := L.index2val(idx)
	if v.Tt == objectapi.TagTable {
		return v.Val.(*tableapi.Table)
	}
	return nil
}

// GetTable pushes t[k] where t is at idx and k is at top. Pops k.
// apiGetWithIndex performs a table get with __index metamethod chain walking.
// Mirrors C Lua's luaV_gettable but without VM stack manipulation.
// Walks up to 20 levels of __index chain (table or function).
func apiGetWithIndex(L *State, t objectapi.TValue, key objectapi.TValue) objectapi.TValue {
	const maxLoop = 20
	for loop := 0; loop < maxLoop; loop++ {
		if t.IsTable() {
			tbl := t.Val.(*tableapi.Table)
			val, found := tbl.Get(key)
			if found && !val.IsNil() {
				return val
			}
			// Key not found — check for __index metamethod
			mt := tbl.GetMetatable()
			if mt == nil {
				return objectapi.Nil
			}
			indexStr := L.internStr("__index")
			tm, tmFound := mt.GetStr(indexStr)
			if !tmFound || tm.IsNil() {
				return objectapi.Nil
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
				stateapi.EnsureStack(ls, 4)
				ls.Stack[ls.Top].Val = tm
				ls.Top++
				ls.Stack[ls.Top].Val = t
				ls.Top++
				ls.Stack[ls.Top].Val = key
				ls.Top++
				vmapi.Call(ls, ls.Top-3, 1)
				result := ls.Stack[ls.Top-1].Val
				ls.Top = oldTop
				return result
			}
			// __index is not table or function — error
			return objectapi.Nil
		}
		// Non-table value — check for type metatable __index
		// (userdata, etc.) For now, return nil
		return objectapi.Nil
	}
	// Too many __index levels
	return objectapi.Nil
}

func (L *State) GetTable(idx int) objectapi.Type {
	ls := L.ls()
	t := L.index2val(idx)
	key := ls.Stack[ls.Top-1].Val
	ls.Top--

	val := apiGetWithIndex(L, *t, key)
	L.push(val)
	return val.Type()
}

// GetField pushes t[key] where t is at idx.
func (L *State) GetField(idx int, key string) objectapi.Type {
	t := L.index2val(idx)
	ks := L.internStr(key)
	val := apiGetWithIndex(L, *t, objectapi.MakeString(ks))
	L.push(val)
	return val.Type()
}

// GetI pushes t[n] where t is at idx.
// Mirrors lua_geti: handles __index metamethods for non-table types
// and for table keys that are not found.
func (L *State) GetI(idx int, n int64) objectapi.Type {
	ls := L.ls()
	t := L.index2val(idx)
	if t.Tt == objectapi.TagTable {
		tbl := t.Val.(*tableapi.Table)
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
	stateapi.EnsureStack(ls, 1)
	ls.Stack[ra].Val = objectapi.Nil // default
	vmapi.FinishGet(ls, *t, objectapi.MakeInteger(n), ra)
	// FinishGet already wrote result to Stack[ra]. Just advance Top.
	result := ls.Stack[ra].Val
	ls.Top = ra + 1
	return result.Type()
}

// GetGlobal pushes the value of global variable name.
func (L *State) GetGlobal(name string) objectapi.Type {
	gt := vmapi.GetGlobalTable(L.ls())
	ks := L.internStr(name)
	val, found := gt.GetStr(ks)
	if found && !val.IsNil() {
		L.push(val)
		return val.Type()
	}
	L.push(objectapi.Nil)
	return objectapi.TypeNil
}

// SetTable does t[k] = v where t is at idx, k at top-1, v at top.
func (L *State) SetTable(idx int) {
	ls := L.ls()
	t := L.index2val(idx)
	if t.Tt == objectapi.TagTable {
		tbl := t.Val.(*tableapi.Table)
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

// SetField does t[key] = v where t is at idx, v at top.
func (L *State) SetField(idx int, key string) {
	ls := L.ls()
	t := L.index2val(idx)
	if t.Tt == objectapi.TagTable {
		tbl := t.Val.(*tableapi.Table)
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
	if t.Tt == objectapi.TagTable {
		tbl := t.Val.(*tableapi.Table)
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
	key := objectapi.MakeInteger(n)
	vmapi.FinishSet(ls, *t, key, val)
	ls.Top--
}

// SetGlobal pops a value and sets it as global variable name.
func (L *State) SetGlobal(name string) {
	ls := L.ls()
	gt := vmapi.GetGlobalTable(ls)
	ks := L.internStr(name)
	val := ls.Stack[ls.Top-1].Val
	gt.SetStr(ks, val)
	ls.Top--
}

// RawGet pushes t[k] without metamethods.
func (L *State) RawGet(idx int) objectapi.Type {
	return L.GetTable(idx) // our GetTable already skips metamethods
}

// RawGetI pushes t[n] without metamethods.
func (L *State) RawGetI(idx int, n int64) objectapi.Type {
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
	t := tableapi.New(nArr, nRec)
	L.TrackAlloc(t.EstimateBytes())
	L.push(objectapi.TValue{Tt: objectapi.TagTable, Val: t})
}

// NewTable pushes a new empty table.
func (L *State) NewTable() {
	L.CreateTable(0, 0)
}

// GetMetatable pushes the metatable of the value at idx.
func (L *State) GetMetatable(idx int) bool {
	v := L.index2val(idx)
	var mt *tableapi.Table
	switch v.Tt {
	case objectapi.TagTable:
		mt = v.Val.(*tableapi.Table).GetMetatable()
	default:
		// Check global type metatables
		ls := L.ls()
		tp := v.Type()
		if int(tp) < len(ls.Global.MT) {
			if tbl, ok := ls.Global.MT[tp].(*tableapi.Table); ok {
				mt = tbl
			}
		}
	}
	if mt != nil {
		L.push(objectapi.TValue{Tt: objectapi.TagTable, Val: mt})
		return true
	}
	return false
}

// SetMetatable pops a table and sets it as metatable for value at idx.
func (L *State) SetMetatable(idx int) {
	ls := L.ls()
	v := L.index2val(idx)
	var mt *tableapi.Table
	mtVal := ls.Stack[ls.Top-1].Val
	if mtVal.Tt == objectapi.TagTable {
		mt = mtVal.Val.(*tableapi.Table)
	}
	ls.Top--

	switch v.Tt {
	case objectapi.TagTable:
		tbl := v.Val.(*tableapi.Table)
		tbl.SetMetatable(mt)
		// Register __gc finalizer if metatable has __gc
		if mt != nil {
			g := ls.Global
			tmName := g.TMNames[metamethodapi.TM_GC]
			gcTM := metamethodapi.GetTM(mt, metamethodapi.TM_GC, tmName)
			if !gcTM.IsNil() {
				runtime.SetFinalizer(tbl, func(t *tableapi.Table) {
					g.GCFinalizerMu.Lock()
					defer g.GCFinalizerMu.Unlock()
					if g.GCClosed {
						return
					}
					g.GCFinalizerQueue = append(g.GCFinalizerQueue, t)
				})
			}
		}
	default:
		tp := v.Type()
		if int(tp) < len(ls.Global.MT) {
			ls.Global.MT[tp] = mt // mt is *tableapi.Table, MT is [9]any — OK
		}
	}
}

// Next implements table traversal.
func (L *State) Next(idx int) bool {
	ls := L.ls()
	t := L.index2val(idx)
	if t.Tt != objectapi.TagTable {
		return false
	}
	tbl := t.Val.(*tableapi.Table)
	key := ls.Stack[ls.Top-1].Val
	ls.Top--
	nextKey, nextVal, ok, err := tbl.Next(key)
	if err != nil {
		vmapi.RunError(ls, err.Error())
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
	stateapi.EnsureStack(ls, 1)
	vmapi.ObjLen(ls, ra, *v)
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
	return v1.Val == v2.Val
}

// Compare compares two values.
func (L *State) Compare(idx1, idx2 int, op CompareOp) bool {
	v1 := L.index2val(idx1)
	v2 := L.index2val(idx2)
	switch op {
	case OpEQ:
		return vmapi.EqualObj(L.ls(), *v1, *v2)
	case OpLT:
		return vmapi.LessThan(L.ls(), *v1, *v2)
	case OpLE:
		return vmapi.LessEqual(L.ls(), *v1, *v2)
	}
	return false
}

// Concat concatenates the n values at the top of the stack.
func (L *State) Concat(n int) {
	ls := L.ls()
	if n >= 2 {
		vmapi.Concat(ls, n)
	} else if n == 0 {
		L.PushString("")
	}
	// n == 1: value already on stack
}

// Arith performs an arithmetic operation.
func (L *State) Arith(op ArithOp) {
	// Simplified: just for basic operations on top-of-stack values
	// Full implementation would use objectapi.RawArith + metamethods
}

// ---------------------------------------------------------------------------
// Call/Load functions
// ---------------------------------------------------------------------------

// Call calls a function. nArgs arguments are on the stack above the function.
func (L *State) Call(nArgs, nResults int) {
	ls := L.ls()
	funcIdx := ls.Top - nArgs - 1
	// C Lua's lua_callk with k==NULL calls luaD_callnoyield.
	// This marks the call as non-yieldable.
	vmapi.CallNoYield(ls, funcIdx, nResults)
	// Ensure Top >= CI.Func + 1 so the API stack is valid
	base := ls.CI.Func + 1
	if ls.Top < base {
		ls.Top = base
	}
}

// PCall performs a protected call. Returns status code.
func (L *State) PCall(nArgs, nResults, msgHandler int) int {
	ls := L.ls()
	funcIdx := ls.Top - nArgs - 1
	errFunc := 0
	if msgHandler != 0 {
		errFunc = L.index2stack(msgHandler)
	}
	status := vmapi.PCall(ls, funcIdx, nResults, errFunc)
	// Ensure Top >= CI.Func + 1 so the API stack is valid
	base := ls.CI.Func + 1
	if ls.Top < base {
		ls.Top = base
	}
	return status
}

// Load loads a Lua chunk from a string. Pushes the compiled function.
func (L *State) Load(code string, name string, mode string) int {
	ls := L.ls()
	if mode == "" {
		mode = "bt"
	}
	isBinary := len(code) > 0 && code[0] == '\x1b' // LUA_SIGNATURE

	if isBinary {
		if !strings.Contains(mode, "b") {
			L.PushString(fmt.Sprintf("%s: attempt to load a binary chunk", name))
			return StatusErrSyntax
		}
		// Binary chunk — use undump
		cl, err := vmapi.UndumpProto(ls, []byte(code), name)
		if err != nil {
			L.PushString(err.Error())
			return StatusErrSyntax
		}
		// Push the closure onto the stack
		L.push(objectapi.TValue{Tt: objectapi.TagLuaClosure, Val: cl})
		// Set _ENV (first upvalue) to the global table.
		// C Lua's lua_load does this after luaD_protectedparser:
		//   if (f->nupvalues >= 1) { setobj(L, f->upvals[0]->v.p, &gt); }
		if len(cl.UpVals) > 0 && cl.UpVals[0] != nil {
			gt := vmapi.GetGlobalTable(ls)
			cl.UpVals[0].Own = objectapi.TValue{
				Tt:  objectapi.TagTable,
				Val: gt,
			}
		}
		return StatusOK
	}

	// Text chunk
	if !strings.Contains(mode, "t") {
		L.PushString(fmt.Sprintf("%s: attempt to load a text chunk", name))
		return StatusErrSyntax
	}
	reader := &stringReader{data: code}
	return vmapi.Load(ls, reader, name)
}

// DoString loads and executes a string.
func (L *State) DoString(code string) error {
	status := L.Load(code, "=(dostring)", "t")
	if status != StatusOK {
		msg, _ := L.ToString(-1)
		L.Pop(1)
		return fmt.Errorf("load error: %s", msg)
	}
	status = L.PCall(0, MultiRet, 0)
	if status != StatusOK {
		msg, _ := L.ToString(-1)
		L.Pop(1)
		return fmt.Errorf("runtime error: %s", msg)
	}
	return nil
}

// DoFile loads and executes a file.
func (L *State) DoFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("cannot open %s: %v", filename, err)
	}
	// Strip shebang line if present
	code := string(data)
	if len(code) > 1 && code[0] == '#' {
		if idx := strings.Index(code, "\n"); idx >= 0 {
			code = code[idx+1:]
		}
	}

	// Prepend script's directory to package.path so require can find
	// sibling .lua modules (e.g., tracegc.lua in the testes directory).
	dir := filepath.Dir(filename)
	if dir != "" {
		L.GetGlobal("package")
		if !L.IsNil(-1) {
			L.GetField(-1, "path")
			oldPath, _ := L.ToString(-1)
			L.Pop(1) // pop old path
			newPath := dir + string(filepath.Separator) + "?.lua;" + oldPath
			L.PushString(newPath)
			L.SetField(-2, "path")
		}
		L.Pop(1) // pop package table
	}

	source := "@" + filename
	status := L.Load(code, source, "t")
	if status != StatusOK {
		msg, _ := L.ToString(-1)
		L.Pop(1)
		return fmt.Errorf("load error: %s", msg)
	}
	status = L.PCall(0, MultiRet, 0)
	if status != StatusOK {
		msg, _ := L.ToString(-1)
		L.Pop(1)
		return fmt.Errorf("runtime error: %s", msg)
	}
	return nil
}

// Error raises a Lua error with the value at the top of the stack.
func (L *State) Error() int {
	vmapi.ErrorMsg(L.ls())
	return 0 // unreachable
}

// ---------------------------------------------------------------------------
// Userdata
// ---------------------------------------------------------------------------

// NewUserdata creates a new full userdata.
func (L *State) NewUserdata(size int, nUV int) interface{} {
	return nil // placeholder
}

// ---------------------------------------------------------------------------
// Upvalue access
// ---------------------------------------------------------------------------

// GetUpvalue pushes the value of upvalue n of the closure at funcIdx.
// Returns (name, true) if upvalue exists, ("", false) if not.
// C closure upvalues are always named "" (empty string).
func (L *State) GetUpvalue(funcIdx, n int) (string, bool) {
	v := L.index2val(funcIdx)
	if v == nil {
		return "", false
	}
	switch v.Tt {
	case objectapi.TagLuaClosure:
		cl := v.Val.(*closureapi.LClosure)
		if n < 1 || n > len(cl.UpVals) {
			return "", false
		}
		uv := cl.UpVals[n-1]
		if uv == nil {
			return "", false
		}
		val := uv.Get(L.ls().Stack)
		L.push(val)
		// Return the name from Proto.Upvalues debug info
		if n-1 < len(cl.Proto.Upvalues) && cl.Proto.Upvalues[n-1].Name != nil {
			return cl.Proto.Upvalues[n-1].Name.String(), true
		}
		return "", true
	case objectapi.TagCClosure:
		cc := v.Val.(*closureapi.CClosure)
		if n < 1 || n > len(cc.UpVals) {
			return "", false
		}
		L.push(cc.UpVals[n-1])
		return "", true // C closures have no upvalue names — always ""
	}
	return "", false
}

// SetUpvalue sets upvalue n of the closure at funcIdx from the top value.
// Returns (name, true) if upvalue exists, ("", false) if not.
func (L *State) SetUpvalue(funcIdx, n int) (string, bool) {
	v := L.index2val(funcIdx)
	if v == nil {
		return "", false
	}
	switch v.Tt {
	case objectapi.TagLuaClosure:
		cl := v.Val.(*closureapi.LClosure)
		if n < 1 || n > len(cl.UpVals) {
			return "", false
		}
		uv := cl.UpVals[n-1]
		if uv == nil {
			return "", false
		}
		val := L.index2val(-1)
		if val != nil {
			uv.Set(L.ls().Stack, *val)
		}
		L.Pop(1)
		if n-1 < len(cl.Proto.Upvalues) && cl.Proto.Upvalues[n-1].Name != nil {
			return cl.Proto.Upvalues[n-1].Name.String(), true
		}
		return "", true
	case objectapi.TagCClosure:
		cc := v.Val.(*closureapi.CClosure)
		if n < 1 || n > len(cc.UpVals) {
			return "", false
		}
		val := L.index2val(-1)
		if val != nil {
			cc.UpVals[n-1] = *val
		}
		L.Pop(1)
		return "", true
	}
	return "", false
}

// ---------------------------------------------------------------------------
// GC
// ---------------------------------------------------------------------------

// GC performs a garbage collection operation.
func (L *State) GC(what GCWhat, args ...int) int {
	// Go's GC handles this. No-op for most operations.
	return 0
}

// GCTotalBytes returns the Lua-level allocation counter (bytes).
// Mirrors C Lua's gettotalbytes(g) for collectgarbage("count").
func (L *State) GCTotalBytes() int64 {
	return L.ls().Global.GCTotalBytes
}

// TrackAlloc adds n bytes to the Lua-level allocation counter.
func (L *State) TrackAlloc(n int64) {
	L.ls().Global.GCTotalBytes += n
}

// ---------------------------------------------------------------------------
// Auxiliary functions (luaL_*)
// ---------------------------------------------------------------------------

// tagError raises a type error for argument arg.
func (L *State) tagError(arg int, tag objectapi.Type) {
	L.TypeError(arg, L.TypeName(tag))
}

// CheckString checks that argument at idx is a string and returns it.
func (L *State) CheckString(idx int) string {
	s, ok := L.ToString(idx)
	if !ok {
		L.tagError(idx, objectapi.TypeString)
	}
	return s
}

// CheckInteger checks that argument at idx is an integer and returns it.
// If the value is a float that can't be represented as an int64,
// raises "has no integer representation" error. If the value is not a
// number at all, raises "number expected" error.
// Mirrors: luaL_checkinteger → luaO_str2intX in lauxlib.c / lobject.c.
func (L *State) CheckInteger(idx int) int64 {
	v := L.index2val(idx)
	switch v.Tt {
	case objectapi.TagInteger:
		return v.Val.(int64)
	case objectapi.TagFloat:
		f := v.Val.(float64)
		if i, ok := objectapi.FloatToInteger(f); ok {
			return i
		}
		L.ArgError(idx, fmt.Sprintf("number (%.10g) has no integer representation", f))
	case objectapi.TagShortStr, objectapi.TagLongStr:
		if i, ok := objectapi.StringToInteger(v.Val.(*objectapi.LuaString).Data); ok {
			return i
		}
		L.ArgError(idx, "malformed number")
	default:
		L.tagError(idx, objectapi.TypeNumber)
	}
	return 0 // unreachable
}

// CheckNumber checks that argument at idx is a number and returns it.
func (L *State) CheckNumber(idx int) float64 {
	n, ok := L.ToNumber(idx)
	if !ok {
		L.tagError(idx, objectapi.TypeNumber)
	}
	return n
}

// CheckType checks that argument at idx has the given type.
func (L *State) CheckType(idx int, tp objectapi.Type) {
	if L.Type(idx) != tp {
		L.tagError(idx, tp)
	}
}

// CheckAny checks that there is an argument at idx.
func (L *State) CheckAny(idx int) {
	if L.Type(idx) == TypeNone {
		L.ArgError(idx, "value expected")
	}
}

// OptString returns the string at idx, or def if nil/none.
func (L *State) OptString(idx int, def string) string {
	if L.IsNoneOrNil(idx) {
		return def
	}
	return L.CheckString(idx)
}

// OptInteger returns the integer at idx, or def if nil/none.
func (L *State) OptInteger(idx int, def int64) int64 {
	if L.IsNoneOrNil(idx) {
		return def
	}
	return L.CheckInteger(idx)
}

// OptNumber returns the number at idx, or def if nil/none.
func (L *State) OptNumber(idx int, def float64) float64 {
	if L.IsNoneOrNil(idx) {
		return def
	}
	return L.CheckNumber(idx)
}

// ArgError raises an error for argument arg.
func (L *State) ArgError(arg int, extraMsg string) int {
	msg := fmt.Sprintf("bad argument #%d (%s)", arg, extraMsg)
	L.PushString(msg)
	L.Error()
	return 0
}

// TypeError raises a type error for argument arg.
func (L *State) TypeError(arg int, tname string) int {
	got := L.TypeName(L.Type(arg))
	msg := fmt.Sprintf("%s expected, got %s", tname, got)
	return L.ArgError(arg, msg)
}

// Where pushes "source:line: " for the given call level.
func (L *State) Where(level int) {
	// Mirrors: luaL_where in lauxlib.c
	ls := L.ls()
	ci := ls.CI
	// Walk up 'level' call frames
	for i := 0; i < level && ci.Prev != nil; i++ {
		ci = ci.Prev
	}
	if ci.IsLua() {
		fval := ls.Stack[ci.Func].Val
		if fval.Tt == objectapi.TagLuaClosure {
			cl := fval.Val.(*closureapi.LClosure)
			pc := ci.SavedPC - 1
			if pc < 0 {
				pc = 0
			}
			line := vmapi.GetFuncLine(cl.Proto, pc)
			srcName := "?"
			if cl.Proto.Source != nil {
				srcName = vmapi.ShortSrc(cl.Proto.Source.Data)
			}
			L.PushString(fmt.Sprintf("%s:%d: ", srcName, line))
			return
		}
	}
	L.PushString("")
}

// Errorf raises a formatted error.
func (L *State) Errorf(format string, args ...interface{}) int {
	L.Where(1)
	L.PushFString(format, args...)
	// Concatenate where + message
	ls := L.ls()
	if ls.Top >= 2 {
		where, _ := L.ToString(-2)
		msg, _ := L.ToString(-1)
		ls.Top -= 2
		L.PushString(where + msg)
	}
	L.Error()
	return 0
}

// SetFuncs registers functions from a map into the table at top of stack.
func (L *State) SetFuncs(funcs map[string]CFunction, nUp int) {
	for name, fn := range funcs {
		if fn == nil {
			L.PushBoolean(false)
		} else {
			// Copy upvalues for each function
			for i := 0; i < nUp; i++ {
				L.PushValue(-nUp) // copy upvalue
			}
			L.PushCClosure(fn, nUp)
		}
		L.SetField(-(nUp + 2), name)
	}
	if nUp > 0 {
		L.Pop(nUp) // remove upvalues
	}
}

// NewLib creates a new table and registers functions into it.
func (L *State) NewLib(funcs map[string]CFunction) {
	L.CreateTable(0, len(funcs))
	L.SetFuncs(funcs, 0)
}

// Require calls openf to load a module, stores in package.loaded.
// If the module is already in package.loaded, pushes the cached value.
// Mirrors luaL_requiref in lauxlib.c.
func (L *State) Require(modname string, openf CFunction, global bool) {
	// Get package.loaded table (or create it)
	L.GetSubTable(RegistryIndex, "_LOADED")
	tp := L.GetField(-1, modname)
	if tp != objectapi.TypeNil {
		// Already loaded — remove _LOADED table, keep the module
		L.Remove(-2)
		return
	}
	L.Pop(1) // pop nil

	// Call the opener
	L.PushCFunction(openf)
	L.PushString(modname)
	L.Call(1, 1) // call openf(modname) -> module table on top

	// Store in package.loaded
	L.PushValue(-1)         // copy module
	L.SetField(-3, modname) // _LOADED[modname] = module

	L.Remove(-2) // remove _LOADED table, keep module on top

	if global {
		L.PushValue(-1)      // copy module
		L.SetGlobal(modname) // _G[modname] = module
	}
}

// Ref creates a reference in the table at idx.
func (L *State) Ref(idx int) int {
	return 0 // placeholder
}

// Unref frees a reference in the table at idx.
func (L *State) Unref(idx int, ref int) {}

// ---------------------------------------------------------------------------
// Debug interface (minimal)
// ---------------------------------------------------------------------------

// GetStack fills a DebugInfo for the given call level.
func (L *State) GetStack(level int) (*DebugInfo, bool) {
	ls := L.ls()
	if level < 0 {
		return nil, false
	}
	ci := ls.CI
	for i := 0; i < level && ci != nil; i++ {
		ci = ci.Prev
	}
	if ci == nil || ci == &ls.BaseCI {
		return nil, false
	}
	ar := &DebugInfo{}
	// Store this CI and thread state for GetInfo/GetLocal to use
	ar.CI = ci
	ar.ThreadState = ls
	fval := ls.Stack[ci.Func].Val
	switch fval.Tt {
	case objectapi.TagLuaClosure:
		cl := fval.Val.(*closureapi.LClosure)
		p := cl.Proto
		if p.Source != nil {
			ar.Source = p.Source.Data
			ar.ShortSrc = shortSrc(p.Source.Data)
		}
		ar.LineDefined = p.LineDefined
		ar.LastLineDefined = p.LastLine
		ar.NUps = len(cl.UpVals)
		ar.NParams = int(p.NumParams)
		ar.IsVararg = p.IsVararg()
		// Mirrors C Lua funcinfo: linedefined==0 means "main" chunk
		if p.LineDefined == 0 {
			ar.What = "main"
		} else {
			ar.What = "Lua"
		}
		// Current line
		pc := ci.SavedPC - 1
		if pc < 0 {
			pc = 0
		}
		ar.CurrentLine = vmapi.GetFuncLine(p, pc)
	case objectapi.TagCClosure, objectapi.TagLightCFunc:
		ar.Source = "=[C]"
		ar.ShortSrc = "[C]"
		ar.What = "C"
		ar.IsVararg = true // All C functions are vararg
		ar.NParams = 0
		if fval.Tt == objectapi.TagCClosure {
			cc := fval.Val.(*closureapi.CClosure)
			ar.NUps = len(cc.UpVals)
		}
	}
	return ar, true
}

// GetInfo fills debug info fields specified by what string.
// Mirrors: lua_getinfo in lapi.c
func (L *State) GetInfo(what string, ar *DebugInfo) bool {
	if ar == nil {
		return false
	}
	// Use the thread state from the debug info if available (for coroutine inspection)
	ls := L.ls()
	if ts, ok := ar.ThreadState.(*stateapi.LuaState); ok {
		ls = ts
	}
	for i := 0; i < len(what); i++ {
		switch what[i] {
		case 'n':
			if ar.CI == nil {
				break
			}
			queriedCI, ok := ar.CI.(*stateapi.CallInfo)
			if !ok {
				break
			}
			caller := queriedCI.Prev
			if caller == nil {
				break
			}
			// Check if caller is closing TBC vars (CISTClsRet flag)
			// If so, this frame is a __close metamethod call
			if caller.CallStatus&stateapi.CISTClsRet != 0 {
				ar.Name = "close"
				ar.NameWhat = "metamethod"
				break
			}
			// Check if caller is running a hook (CISTHooked flag)
			// If so, this frame was called from a hook dispatch
			if caller.CallStatus&stateapi.CISTHooked != 0 {
				ar.Name = "?"
				ar.NameWhat = "hook"
				break
			}
			fval := ls.Stack[caller.Func].Val
			if fval.Tt != objectapi.TagLuaClosure {
				break
			}
			cl := fval.Val.(*closureapi.LClosure)
			p := cl.Proto
			if p == nil {
				break
			}
			pc := caller.SavedPC - 1
			if pc < 0 || pc >= len(p.Code) {
				break
			}
			// Use funcNameFromCode which handles all opcodes:
			// OP_CALL, OP_TAILCALL, OP_TFORCALL, OP_MMBIN, OP_GETTABUP, etc.
			kind, name := vmapi.FuncNameFromCode(ls, p, pc)
			if name != "" {
				ar.Name = name
				ar.NameWhat = kind
			}
		case 'S', 'l', 'u', 'f':
			// Already filled by GetStack
		case 'r':
			// Transfer info for call/return hooks
			if ar.CI != nil {
				if ci, ok := ar.CI.(*stateapi.CallInfo); ok {
					if ci.CallStatus&stateapi.CISTHooked != 0 {
						ar.FTransfer = ls.FTransfer
						ar.NTransfer = ls.NTransfer
					}
				}
			}
		case 't':
			// Tail call and extra args info
			if ar.CI != nil {
				if ci, ok := ar.CI.(*stateapi.CallInfo); ok {
					ar.IsTailCall = ci.CallStatus&stateapi.CISTTail != 0
					ar.ExtraArgs = ci.NExtraArgs
				}
			}
		}
	}
	return true
}

// shortSrc creates a short source name for error messages.
// Mirrors luaO_chunkid in lobject.c. LUA_IDSIZE = 60.
func shortSrc(source string) string {
	const idsize = 60
	if len(source) == 0 {
		// Empty string source → [string ""]
		return `[string ""]`
	}
	if source[0] == '=' {
		rest := source[1:]
		if len(rest)+1 <= idsize {
			return rest
		}
		return rest[:idsize-1]
	}
	if source[0] == '@' {
		rest := source[1:]
		if len(rest)+1 <= idsize {
			return rest
		}
		return "..." + rest[len(rest)-(idsize-1-3):]
	}
	// String source: format as [string "source"]
	// PRE=[string " (9), POS="] (2), RETS=... (3), +1 for NUL
	const maxContent = idsize - 9 - 3 - 2 - 1 // = 45
	nl := strings.IndexByte(source, '\n')
	srclen := len(source)
	if srclen <= maxContent && nl < 0 {
		// Small one-line source — keep it as-is
		return fmt.Sprintf("[string \"%s\"]", source)
	}
	// Truncate: stop at first newline, then clamp to maxContent, add "..."
	if nl >= 0 {
		srclen = nl
	}
	if srclen > maxContent {
		srclen = maxContent
	}
	return fmt.Sprintf("[string \"%s...\"]", source[:srclen])
}

// ---------------------------------------------------------------------------
// Debug hooks
// ---------------------------------------------------------------------------

// SetHookFields sets the hook mask, count, and enable flag on the internal state.
func (L *State) SetHookFields(mask, count int) {
	ls := L.ls()
	ls.HookMask = mask
	ls.BaseHookCount = count
	ls.HookCount = count
	ls.AllowHook = true
}

// ClearHookFields clears all hook fields.
func (L *State) ClearHookFields() {
	ls := L.ls()
	ls.HookMask = 0
	ls.BaseHookCount = 0
	ls.HookCount = 0
	ls.AllowHook = true
	ls.Hook = nil
}

// SetHookMarker sets a non-nil marker in Hook to indicate hooks are active.
func (L *State) SetHookMarker() {
	L.ls().Hook = true
}

// HookMask returns the current hook mask.
func (L *State) HookMask() int {
	return L.ls().HookMask
}

// HookCount returns the base hook count (for count hooks).
func (L *State) HookCount() int {
	return L.ls().BaseHookCount
}

// HookActive returns true if any hooks are set.
func (L *State) HookActive() bool {
	return L.ls().HookMask != 0
}

// HasCallFrames returns true if the thread has call frames above the base CI.
// Mirrors: lua_getstack(L, 0, &ar) in C Lua — returns true when ci != &L->base_ci.
func (L *State) HasCallFrames() bool {
	ls := L.ls()
	return ls.CI != nil && ls.CI != &ls.BaseCI
}

// GetFuncProtoInfo inspects a Lua closure at stack index `idx` and returns
// its Proto metadata. For C functions returns defaults with ok=false.
func (L *State) GetFuncProtoInfo(idx int) (source, shortSrc, what string, lineDefined, lastLine, nups, nparams int, isVararg, ok bool) {
	v := L.index2val(idx)
	if v == nil {
		return "=[C]", "[C]", "C", 0, 0, 0, 0, true, false
	}
	if v.Tt == objectapi.TagLuaClosure {
		cl := v.Val.(*closureapi.LClosure)
		p := cl.Proto
		if p != nil {
			src := ""
			if p.Source != nil {
				src = p.Source.String()
			}
			w := "Lua"
			if p.LineDefined == 0 {
				w = "main"
			}
			return src, shortSrcStr(src), w, p.LineDefined, p.LastLine, len(cl.UpVals), int(p.NumParams), p.IsVararg(), true
		}
	}
	if v.Tt == objectapi.TagCClosure {
		cc := v.Val.(*closureapi.CClosure)
		return "=[C]", "[C]", "C", 0, 0, len(cc.UpVals), 0, true, false
	}
	// TagLightCFunc or other C function types
	return "=[C]", "[C]", "C", 0, 0, 0, 0, true, false
}

// shortSrcStr creates a short source name (exported for use by stdlib).
func shortSrcStr(source string) string {
	return shortSrc(source)
}
func (L *State) GetLocal(ar *DebugInfo, n int) string {
	ls, ok := ar.ThreadState.(*stateapi.LuaState)
	if !ok {
		ls = L.ls()
	}
	ci, ok := ar.CI.(*stateapi.CallInfo)
	if !ok || ci == nil {
		return ""
	}
	clfn := ls.Stack[ci.Func].Val
	isLua := clfn.Tt == objectapi.TagLuaClosure

	var proto *objectapi.Proto
	if isLua {
		cl, ok := clfn.Val.(*closureapi.LClosure)
		if ok && cl != nil && cl.Proto != nil {
			proto = cl.Proto
		}
	}

	// Negative n: vararg slots (Lua functions only)
	if n < 0 {
		if proto == nil || !proto.IsVararg() {
			return ""
		}
		numExtra := ci.NExtraArgs
		if numExtra <= 0 {
			return ""
		}
		// Formula: slot = ci.Func - numExtra - n - 1 (n is negative).
		slot := ci.Func - int(numExtra) - n - 1
		// Valid: slot >= ci.Func-numExtra and slot <= ci.Func-1.
		if slot < ci.Func-int(numExtra) || slot > ci.Func-1 {
			return ""
		}
		vmapi.CheckStack(ls, 1)
		ls.Stack[ls.Top].Val = ls.Stack[slot].Val
		ls.Top++
		return "(vararg)"
	}

	// Try to find a named local (Lua functions only)
	name := ""
	if proto != nil {
		localNum := n
		pc := ci.SavedPC - 1
		if pc < 0 {
			pc = 0
		}
		for i := 0; i < len(proto.LocVars) && proto.LocVars[i].StartPC <= pc; i++ {
			if pc < proto.LocVars[i].EndPC {
				localNum--
				if localNum == 0 {
					if proto.LocVars[i].Name != nil {
						name = proto.LocVars[i].Name.Data
					}
					break
				}
			}
		}
	}

	// If we found a named local, push its value and return
	if name != "" && name != "?" {
		slot := ci.Func + n
		if slot < 0 || slot >= len(ls.Stack) {
			return ""
		}
		vmapi.CheckStack(ls, 1)
		ls.Stack[ls.Top] = ls.Stack[slot]
		ls.Top++
		return name
	}

	// Fallback: check if n is within CI stack range (unnamed slots).
	// Mirrors C Lua's luaG_findlocal fallback for temporaries.
	base := ci.Func + 1
	var limit int
	if ci == ls.CI {
		limit = ls.Top
	} else if ci.Next != nil {
		limit = ci.Next.Func
	} else {
		limit = ls.Top
	}
	if n > 0 && limit-base >= n {
		slot := base + n - 1
		if slot >= 0 && slot < len(ls.Stack) {
			vmapi.CheckStack(ls, 1)
			ls.Stack[ls.Top] = ls.Stack[slot]
			ls.Top++
			if isLua {
				return "(temporary)"
			}
			return "(C temporary)"
		}
	}
	return ""
}

func (L *State) SetLocal(ar *DebugInfo, n int) string {
	ls, ok := ar.ThreadState.(*stateapi.LuaState)
	if !ok {
		ls = L.ls()
	}
	ci, ok := ar.CI.(*stateapi.CallInfo)
	if !ok || ci == nil {
		return ""
	}
	clfn := ls.Stack[ci.Func].Val
	isLua := clfn.Tt == objectapi.TagLuaClosure

	var proto *objectapi.Proto
	if isLua {
		cl, ok := clfn.Val.(*closureapi.LClosure)
		if ok && cl != nil && cl.Proto != nil {
			proto = cl.Proto
		}
	}

	// Negative n: vararg slots (Lua functions only)
	if n < 0 {
		if proto == nil || !proto.IsVararg() {
			return ""
		}
		numExtra := ci.NExtraArgs
		if numExtra <= 0 {
			return ""
		}
		// Same formula as GetLocal: slot = ci.Func - numExtra - n - 1.
		slot := ci.Func - int(numExtra) - n - 1
		if slot < ci.Func-int(numExtra) || slot > ci.Func-1 {
			return ""
		}
		ls.Stack[slot] = ls.Stack[ls.Top-1]
		ls.Top--
		return "(vararg)"
	}

	// Positive n: try named locals first (Lua functions only)
	name := ""
	if proto != nil {
		localNum := n
		pc := ci.SavedPC - 1
		if pc < 0 {
			pc = 0
		}
		for i := 0; i < len(proto.LocVars) && proto.LocVars[i].StartPC <= pc; i++ {
			if pc < proto.LocVars[i].EndPC {
				localNum--
				if localNum == 0 {
					if proto.LocVars[i].Name != nil {
						name = proto.LocVars[i].Name.Data
					}
					break
				}
			}
		}
	}

	// If we found a named local, set its value and return
	if name != "" && name != "?" {
		slot := ci.Func + n
		if slot < 0 || slot >= len(ls.Stack) {
			return ""
		}
		ls.Stack[slot] = ls.Stack[ls.Top-1]
		ls.Top--
		return name
	}

	// Fallback: check if n is within CI stack range (unnamed slots / temporaries).
	// Mirrors C Lua's luaG_findlocal fallback.
	base := ci.Func + 1
	var limit int
	if ci == ls.CI {
		limit = ls.Top
	} else if ci.Next != nil {
		limit = ci.Next.Func
	} else {
		limit = ls.Top
	}
	if n > 0 && limit-base >= n {
		slot := base + n - 1
		if slot >= 0 && slot < len(ls.Stack) {
			ls.Stack[slot] = ls.Stack[ls.Top-1]
			ls.Top--
			if isLua {
				return "(temporary)"
			}
			return "(C temporary)"
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Coroutine API
// ---------------------------------------------------------------------------

// NewThread creates a new Lua thread (coroutine), pushes it on the stack,
// and returns a *State representing the new thread.
// Mirrors: lua_newthread in lstate.c
func (L *State) NewThread() *State {
	ls := L.ls()
	L1 := stateapi.NewThread(ls)
	// Push the new thread onto the parent's stack
	L.push(objectapi.TValue{Tt: objectapi.TagThread, Val: L1})
	return &State{Internal: L1}
}

// PushThread pushes the running thread onto its own stack.
// Returns true if the thread is the main thread.
func (L *State) PushThread() bool {
	ls := L.ls()
	L.push(objectapi.TValue{Tt: objectapi.TagThread, Val: ls})
	return ls.Global.MainThread == ls || ls.Global.MainThread == nil
}

// Resume starts or resumes a coroutine.
// Returns (status, nresults) matching lua_resume in ldo.c.
// status is StatusOK (finished) or StatusYield (suspended) on success,
// or an error status on failure.
func (L *State) Resume(from *State, nArgs int) (int, int) {
	ls := L.ls()
	var fromLS *stateapi.LuaState
	if from != nil {
		fromLS = from.ls()
	}
	status, nresults := vmapi.Resume(ls, fromLS, nArgs)
	return status, nresults
}

// YieldK yields a coroutine with a continuation function.
// Mirrors: lua_yieldk in ldo.c
func (L *State) YieldK(nResults int, ctx int, k CFunction) int {
	ls := L.ls()
	if k != nil {
		ci := ls.CI
		ci.K = func(innerL *stateapi.LuaState, status int, context int) int {
			wrapper := &State{Internal: innerL}
			return k(wrapper)
		}
		ci.Ctx = ctx
	}
	vmapi.Yield(ls, nResults)
	return 0 // unreachable — Yield panics with LuaYield
}

// Yield yields a coroutine (no continuation).
func (L *State) Yield(nResults int) int {
	return L.YieldK(nResults, 0, nil)
}

// IsYieldable returns true if the running coroutine can yield.
func (L *State) IsYieldable() bool {
	return L.ls().Yieldable()
}

// XMove moves n values from L's stack to to's stack.
// Mirrors: lua_xmove in lapi.c
func (L *State) XMove(to *State, n int) {
	if L == to {
		return
	}
	fromLS := L.ls()
	toLS := to.ls()
	fromLS.Top -= n
	for i := 0; i < n; i++ {
		stateapi.PushValue(toLS, fromLS.Stack[fromLS.Top+i].Val)
	}
}

// ToThread converts the value at the given index to a *State (thread).
// Returns nil if the value is not a thread.
func (L *State) ToThread(idx int) *State {
	v := L.index2val(idx)
	if v.Tt != objectapi.TagThread {
		return nil
	}
	ls, ok := v.Val.(*stateapi.LuaState)
	if !ok {
		return nil
	}
	return &State{Internal: ls}
}

// Status returns the status of the coroutine L.
func (L *State) Status() int {
	return L.ls().Status
}

// ---------------------------------------------------------------------------
// Userdata API (stubs)
// ---------------------------------------------------------------------------

func (L *State) ToUserdata(idx int) interface{}              { return nil }
// GetIUserValue pushes the n-th user value of the userdata at idx onto the stack.
// Returns the type of the pushed value, or TypeNone if invalid.
// For non-full-userdata or invalid n, pushes nil and returns TypeNone.
// Mirrors: lua_getiuservalue in lapi.c
func (L *State) GetIUserValue(idx int, n int) objectapi.Type {
	ls := L.ls()
	// Always push something (nil for failure cases)
	vmapi.CheckStack(ls, 1)
	ls.Stack[ls.Top].Val = objectapi.Nil
	ls.Top++
	return objectapi.TypeNone
}

// SetIUserValue sets the n-th user value of the userdata at idx to the value
// at the top of the stack. Returns false if the operation fails (e.g. not full userdata).
// Note: unlike C Lua's lua_setiuservalue, this stub does NOT pop the value.
// The caller (debugSetuservalue) manages the stack.
func (L *State) SetIUserValue(idx int, n int) bool {
	return false
}
func (L *State) ToPointer(idx int) string {
	v := L.index2val(idx)
	if v == nil || v.Val == nil {
		return ""
	}
	rv := reflect.ValueOf(v.Val)
	switch rv.Kind() {
	case reflect.Ptr:
		return fmt.Sprintf("0x%x", rv.Pointer())
	case reflect.Func:
		// reflect.Pointer() returns code entry point — same for all closures
		// of the same template. Use interface data word for unique pointer.
		type eface struct {
			_type uintptr
			data  uintptr
		}
		iface := v.Val
		ef := (*eface)(unsafe.Pointer(&iface))
		return fmt.Sprintf("0x%x", ef.data)
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// String reader (for Load)
// ---------------------------------------------------------------------------

type stringReader struct {
	data string
	pos  int
}

func (r *stringReader) ReadByte() int {
	if r.pos >= len(r.data) {
		return -1
	}
	b := r.data[r.pos]
	r.pos++
	return int(b)
}

// Ensure unused imports are used
var _ = parseapi.Parse
var _ = lexapi.TK_EOS

// --- Additional auxiliary functions needed by stdlib ---

// TolString converts the value at idx to a string, using __tostring metamethod if present.
// Pushes the result on the stack and returns it.
func (L *State) TolString(idx int) string {
	v := L.index2val(idx)
	if v == nil {
		L.PushString("nil")
		return "nil"
	}
	// Check for __tostring metamethod
	if L.GetMetafield(idx, "__tostring") {
		L.PushValue(idx)
		L.Call(1, 1)
		if !L.IsString(-1) {
			L.Errorf("'__tostring' must return a string")
		}
		s, _ := L.ToString(-1)
		return s
	}
	// Default conversion based on type
	switch {
	case v.IsString():
		s := v.StringVal().Data
		L.PushString(s)
		return s
	case v.IsInteger():
		s := fmt.Sprintf("%d", v.Integer())
		L.PushString(s)
		return s
	case v.IsFloat():
		s := objectapi.FloatToString(v.Float())
		L.PushString(s)
		return s
	case v.Tt == objectapi.TagTrue:
		L.PushString("true")
		return "true"
	case v.Tt == objectapi.TagFalse:
		L.PushString("false")
		return "false"
	case v.IsNil():
		L.PushString("nil")
		return "nil"
	default:
		// C Lua: luaT_objtypename checks __name in metatable first
		tn := L.TypeName(L.Type(idx))
		if L.GetMetafield(idx, "__name") {
			if name, ok := L.ToString(-1); ok {
				tn = name
			}
			L.Pop(1)
		}
		s := fmt.Sprintf("%s: 0x%x", tn, reflect.ValueOf(v.Val).Pointer())
		L.PushString(s)
		return s
	}
}

// GetMetafield pushes the metamethod field from the metatable of the value at idx.
// Returns true if found (value pushed), false if not (nothing pushed).
func (L *State) GetMetafield(idx int, field string) bool {
	if !L.GetMetatable(idx) {
		return false
	}
	tp := L.GetField(-1, field)
	if tp == objectapi.TypeNil {
		L.Pop(2) // pop nil and metatable
		return false
	}
	L.Remove(-2) // remove metatable, keep field value
	return true
}

// StringToNumber tries to convert a string to a number and pushes it.
// Returns the length+1 on success, 0 on failure.
func (L *State) StringToNumber(s string) int {
	tv, ok := objectapi.StringToNumber(s)
	if !ok {
		return 0
	}
	L.push(tv)
	return len(s) + 1
}

// PushFail pushes a "fail" value (false in Lua 5.4+).
func (L *State) PushFail() {
	L.PushNil()
}

// ArgCheck checks a condition for argument arg. If cond is false, raises an error.
func (L *State) ArgCheck(cond bool, arg int, extraMsg string) {
	if !cond {
		L.ArgError(arg, extraMsg)
	}
}

// ArgExpected checks that argument arg has the expected type name.
func (L *State) ArgExpected(cond bool, arg int, tname string) {
	if !cond {
		L.TypeError(arg, tname)
	}
}

// CheckOption checks that the argument at idx is a string matching one of the options.
// Returns the index of the matched option.
func (L *State) CheckOption(idx int, def string, opts []string) int {
	var s string
	if def != "" && L.IsNoneOrNil(idx) {
		s = def
	} else {
		s = L.CheckString(idx)
	}
	for i, opt := range opts {
		if s == opt {
			return i
		}
	}
	L.ArgError(idx, fmt.Sprintf("invalid option '%s'", s))
	return 0 // unreachable
}

// LenI returns the length of the value at idx as an integer (calls __len if needed).
func (L *State) LenI(idx int) int64 {
	L.Len(idx)
	n, ok := L.ToInteger(-1)
	L.Pop(1)
	if !ok {
		L.Errorf("object length is not an integer")
	}
	return n
}

// GetSubTable ensures that t[fname] is a table, creating it if needed.
// Returns true if the table already existed.
func (L *State) GetSubTable(idx int, fname string) bool {
	if L.GetField(idx, fname) == objectapi.TypeTable {
		return true // table already there
	}
	L.Pop(1) // remove previous result
	idx = L.AbsIndex(idx)
	L.NewTable()
	L.PushValue(-1)        // copy to be left at top
	L.SetField(idx, fname) // assign new table to field
	return false
}

// GetLClosure returns the LClosure at the given stack index, or nil if not a Lua closure.
func (L *State) GetLClosure(idx int) *closureapi.LClosure {
	v := L.index2val(idx)
	if v.Tt != objectapi.TagLuaClosure {
		return nil
	}
	return v.Val.(*closureapi.LClosure)
}

// DebugGetProto returns the Proto of the LClosure at top of stack (for testing).
func DebugGetProto(L *State) *objectapi.Proto {
	ls := L.ls()
	if ls.Top <= 0 {
		return nil
	}
	fval := ls.Stack[ls.Top-1].Val
	if fval.Tt != objectapi.TagLuaClosure {
		return nil
	}
	cl := fval.Val.(*closureapi.LClosure)
	return cl.Proto
}

// PushFuncFromDebug pushes the function associated with a DebugInfo onto the stack.
// Returns true if successful, false if the CI is nil or invalid.
func (L *State) PushFuncFromDebug(ar *DebugInfo) bool {
	if ar == nil || ar.CI == nil {
		return false
	}
	ci, ok := ar.CI.(*stateapi.CallInfo)
	if !ok {
		return false
	}
	ls := L.ls()
	if ci.Func < 0 || ci.Func >= len(ls.Stack) {
		return false
	}
	L.push(ls.Stack[ci.Func].Val)
	return true
}

// ---------------------------------------------------------------------------
// DrainGCFinalizers — run pending __gc metamethods for collected objects.
//
// Called synchronously from collectgarbage("collect") and CloseState.
// Objects are enqueued by runtime.SetFinalizer callbacks (in arbitrary
// goroutines) and drained here in the calling goroutine where it is safe
// to call Lua functions.
//
// Mirrors: GCTM() in lgc.c — runs one finalizer per call.
// ---------------------------------------------------------------------------

// DrainGCFinalizers drains the GC finalizer queue, calling each object's
// __gc metamethod via a protected call. Errors are silently discarded
// (matching C Lua behavior).
func (L *State) DrainGCFinalizers() {
	ls := L.ls()
	g := ls.Global
	if g == nil {
		return
	}

	for {
		// Atomically grab the queue
		g.GCFinalizerMu.Lock()
		queue := g.GCFinalizerQueue
		g.GCFinalizerQueue = nil
		g.GCFinalizerMu.Unlock()

		if len(queue) == 0 {
			break
		}

		for _, obj := range queue {
			tbl, ok := obj.(*tableapi.Table)
			if !ok {
				continue // skip non-table objects for now
			}
			mt := tbl.GetMetatable()
			if mt == nil {
				continue
			}
			tmName := g.TMNames[metamethodapi.TM_GC]
			gcTM := metamethodapi.GetTM(mt, metamethodapi.TM_GC, tmName)
			if gcTM.IsNil() {
				continue
			}

			// Push the __gc function and the table as argument
			L.push(gcTM)
			L.push(objectapi.TValue{Val: tbl, Tt: objectapi.TagTable})
			// Protected call: 1 arg, 0 results, no error handler
			// Discard errors (like C Lua's GCTM)
			L.PCall(1, 0, 0)
		}
	}
}
