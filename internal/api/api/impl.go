package api

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	closureapi "github.com/akzj/go-lua/internal/closure/api"
	lexapi "github.com/akzj/go-lua/internal/lex/api"
	luastringapi "github.com/akzj/go-lua/internal/luastring/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	opcodeapi "github.com/akzj/go-lua/internal/opcode/api"
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
	stateapi.EnsureStack(ls, n)
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
		f := v.Val.(float64)
		i := int64(f)
		if float64(i) == f {
			return i, true
		}
		return 0, false
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
func (L *State) GetTable(idx int) objectapi.Type {
	ls := L.ls()
	t := L.index2val(idx)
	key := ls.Stack[ls.Top-1].Val
	ls.Top--

	if t.Tt == objectapi.TagTable {
		tbl := t.Val.(*tableapi.Table)
		val, found := tbl.Get(key)
		if found && !val.IsNil() {
			L.push(val)
			return val.Type()
		}
	}
	// For simplicity, push nil if not found (skip metamethods for now)
	L.push(objectapi.Nil)
	return objectapi.TypeNil
}

// GetField pushes t[key] where t is at idx.
func (L *State) GetField(idx int, key string) objectapi.Type {
	t := L.index2val(idx)
	if t.Tt == objectapi.TagTable {
		tbl := t.Val.(*tableapi.Table)
		ks := L.internStr(key)
		val, found := tbl.GetStr(ks)
		if found && !val.IsNil() {
			L.push(val)
			return val.Type()
		}
	}
	L.push(objectapi.Nil)
	return objectapi.TypeNil
}

// GetI pushes t[n] where t is at idx.
func (L *State) GetI(idx int, n int64) objectapi.Type {
	t := L.index2val(idx)
	if t.Tt == objectapi.TagTable {
		tbl := t.Val.(*tableapi.Table)
		val, found := tbl.GetInt(n)
		if found && !val.IsNil() {
			L.push(val)
			return val.Type()
		}
	}
	L.push(objectapi.Nil)
	return objectapi.TypeNil
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
	if t.Tt == objectapi.TagTable {
		tbl := t.Val.(*tableapi.Table)
		val := ls.Stack[ls.Top-1].Val
		tbl.SetInt(n, val)
	}
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
		v.Val.(*tableapi.Table).SetMetatable(mt)
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
	nextKey, nextVal, ok := tbl.Next(key)
	if ok {
		L.push(nextKey)
		L.push(nextVal)
		return true
	}
	return false
}

// Len pushes the length of the value at idx.
func (L *State) Len(idx int) {
	v := L.index2val(idx)
	switch v.Tt {
	case objectapi.TagTable:
		L.push(objectapi.MakeInteger(v.Val.(*tableapi.Table).RawLen()))
	case objectapi.TagShortStr, objectapi.TagLongStr:
		L.push(objectapi.MakeInteger(int64(len(v.Val.(*objectapi.LuaString).Data))))
	default:
		L.push(objectapi.MakeInteger(0))
	}
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
	vmapi.Call(ls, funcIdx, nResults)
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
	reader := &stringReader{data: code}
	if name == "" {
		name = "=(load)"
	}
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
func (L *State) GetUpvalue(funcIdx, n int) string {
	v := L.index2val(funcIdx)
	if v == nil {
		return ""
	}
	switch v.Tt {
	case objectapi.TagLuaClosure:
		cl := v.Val.(*closureapi.LClosure)
		if n < 1 || n > len(cl.UpVals) {
			return ""
		}
		uv := cl.UpVals[n-1]
		if uv == nil {
			return ""
		}
		val := uv.Get(L.ls().Stack)
		L.push(val)
		// Return the name from Proto.Upvalues debug info
		if n-1 < len(cl.Proto.Upvalues) && cl.Proto.Upvalues[n-1].Name != nil {
			return cl.Proto.Upvalues[n-1].Name.String()
		}
		return ""
	case objectapi.TagCClosure:
		cc := v.Val.(*closureapi.CClosure)
		if n < 1 || n > len(cc.UpVals) {
			return ""
		}
		L.push(cc.UpVals[n-1])
		return "" // C closures have no upvalue names
	}
	return ""
}

// SetUpvalue sets upvalue n of the closure at funcIdx from the top value.
func (L *State) SetUpvalue(funcIdx, n int) string {
	v := L.index2val(funcIdx)
	if v == nil {
		return ""
	}
	switch v.Tt {
	case objectapi.TagLuaClosure:
		cl := v.Val.(*closureapi.LClosure)
		if n < 1 || n > len(cl.UpVals) {
			return ""
		}
		uv := cl.UpVals[n-1]
		if uv == nil {
			return ""
		}
		val := L.index2val(-1)
		if val != nil {
			uv.Set(L.ls().Stack, *val)
		}
		L.Pop(1)
		if n-1 < len(cl.Proto.Upvalues) && cl.Proto.Upvalues[n-1].Name != nil {
			return cl.Proto.Upvalues[n-1].Name.String()
		}
		return ""
	case objectapi.TagCClosure:
		cc := v.Val.(*closureapi.CClosure)
		if n < 1 || n > len(cc.UpVals) {
			return ""
		}
		val := L.index2val(-1)
		if val != nil {
			cc.UpVals[n-1] = *val
		}
		L.Pop(1)
		return ""
	}
	return ""
}

// ---------------------------------------------------------------------------
// GC
// ---------------------------------------------------------------------------

// GC performs a garbage collection operation.
func (L *State) GC(what GCWhat, args ...int) int {
	// Go's GC handles this. No-op for most operations.
	return 0
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
func (L *State) CheckInteger(idx int) int64 {
	n, ok := L.ToInteger(idx)
	if !ok {
		L.tagError(idx, objectapi.TypeNumber)
	}
	return n
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
	L.PushValue(-1)          // copy module
	L.SetField(-3, modname)  // _LOADED[modname] = module

	L.Remove(-2) // remove _LOADED table, keep module on top

	if global {
		L.PushValue(-1)       // copy module
		L.SetGlobal(modname)  // _G[modname] = module
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
	ci := ls.CI
	for i := 0; i < level && ci != nil; i++ {
		ci = ci.Prev
	}
	if ci == nil {
		return nil, false
	}
	ar := &DebugInfo{}
	// Store this CI for GetInfo to use
	ar.ci = ci
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
		if ci == &ls.BaseCI {
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
	}
	return ar, true
}

// GetInfo fills debug info fields specified by what string.
// Mirrors: lua_getinfo in lapi.c
func (L *State) GetInfo(what string, ar *DebugInfo) bool {
	if ar == nil {
		return false
	}
	ls := L.ls()
	for i := 0; i < len(what); i++ {
		switch what[i] {
		case 'n':
			if ar.ci == nil {
				break
			}
			queriedCI, ok := ar.ci.(*stateapi.CallInfo)
			if !ok {
				break
			}
			caller := queriedCI.Prev
			if caller == nil {
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
			inst := p.Code[pc]
			op := opcodeapi.GetOpCode(inst)
			if op == opcodeapi.OP_CALL || op == opcodeapi.OP_TAILCALL {
				reg := int(opcodeapi.GetArgA(inst))
				kind, name := vmapi.BasicGetObjName(p, pc, reg)
				if name != "" {
					ar.Name = name
					ar.NameWhat = kind
				}
			}
		case 'S', 'l', 'u', 'f':
			// Already filled by GetStack
		}
	}
	return true
}

// shortSrc creates a short source name for error messages.
func shortSrc(source string) string {
	if len(source) == 0 {
		return "[string \"?\"]"
	}
	if source[0] == '=' {
		if len(source) <= 60 {
			return source[1:]
		}
		return source[1:60]
	}
	if source[0] == '@' {
		if len(source) <= 60 {
			return source[1:]
		}
		return "..." + source[len(source)-57:]
	}
	// String source
	first := strings.SplitN(source, "\n", 2)[0]
	if len(first) > 45 {
		first = first[:45]
	}
	return fmt.Sprintf("[string \"%s\"]", first)
}

// ---------------------------------------------------------------------------
// Debug hooks (stubs)
// ---------------------------------------------------------------------------

func (L *State) SetHook(f interface{}, mask, count int) {}
func (L *State) GetHook() interface{}                    { return nil }
func (L *State) GetHookMask() int                        { return 0 }
func (L *State) GetHookCount() int                       { return 0 }
func (L *State) GetLocal(ar *DebugInfo, n int) string    { return "" }
func (L *State) SetLocal(ar *DebugInfo, n int) string    { return "" }

// ---------------------------------------------------------------------------
// Coroutine API (stubs)
// ---------------------------------------------------------------------------

func (L *State) NewThread() *State     { return nil }
func (L *State) PushThread() bool      { return true }
func (L *State) Resume(from *State, nArgs int) (int, bool) { return 0, false }
func (L *State) YieldK(nResults int, ctx int, k CFunction) int { return 0 }
func (L *State) Yield(nResults int) int { return 0 }
func (L *State) IsYieldable() bool     { return false }
func (L *State) XMove(to *State, n int) {}
func (L *State) ToThread(idx int) *State { return nil }

// ---------------------------------------------------------------------------
// Userdata API (stubs)
// ---------------------------------------------------------------------------

func (L *State) ToUserdata(idx int) interface{}                    { return nil }
func (L *State) GetIUserValue(idx int, n int) objectapi.Type       { return 0 }
func (L *State) SetIUserValue(idx int, n int) bool                 { return false }
func (L *State) ToPointer(idx int) interface{}                     { return nil }

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
		tn := L.TypeName(L.Type(idx))
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
	L.PushValue(-1)         // copy to be left at top
	L.SetField(idx, fname)  // assign new table to field
	return false
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
