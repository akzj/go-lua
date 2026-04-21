// impl.go — Core Lua API implementation (State, stack ops, push, type checks, conversions).
package api

import (
	"fmt"
	"reflect"

	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/gc"
	"github.com/akzj/go-lua/internal/lex"
	"github.com/akzj/go-lua/internal/luastring"
	"github.com/akzj/go-lua/internal/object"

	"github.com/akzj/go-lua/internal/parse"
	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/table"
)

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// ls returns the internal LuaState.
func (L *State) ls() *state.LuaState {
	return L.Internal.(*state.LuaState)
}

// strtab returns the string interning table from the global state.
func (L *State) strtab() *luastring.StringTable {
	return L.ls().Global.StringTable.(*luastring.StringTable)
}

// internStr creates a properly-hashed, interned LuaString.
// This ensures table lookups work correctly (hash-based bucket matching).
func (L *State) internStr(s string) *object.LuaString {
	return L.strtab().Intern(s)
}

// nilValue is a sentinel for invalid/absent stack values.
var nilValue = object.Nil

// index2val converts a Lua API index to a pointer to the TValue.
// Positive indices are 1-based from CI.Func+1.
// Negative indices are relative to Top.
// Pseudo-indices: RegistryIndex, UpvalueIndex(n).
// Returns pointer to nilValue for invalid indices.
func (L *State) index2val(idx int) *object.TValue {
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
		case object.TagCClosure:
			cc := fval.Val.(*closure.CClosure)
			if upIdx <= len(cc.UpVals) {
				return &cc.UpVals[upIdx-1]
			}
		case object.TagLuaClosure:
			lc := fval.Val.(*closure.LClosure)
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
func (L *State) push(v object.TValue) {
	ls := L.ls()
	state.PushValue(ls, v)
}

// wrapCFunctionStatic creates an adapter without capturing a specific State.
// Each call creates a temporary State wrapper.
func wrapCFunctionStatic(f CFunction) state.CFunction {
	return func(ls *state.LuaState) int {
		pub := &State{Internal: ls}
		return f(pub)
	}
}

// ---------------------------------------------------------------------------
// State creation/destruction
// ---------------------------------------------------------------------------

// NewState creates a new Lua state.
func NewState() *State {
	ls := state.NewState()
	L := &State{Internal: ls}

	// Wire up GC step functions so the VM can trigger periodic GC
	// without importing this package.

	// GCStepFn: periodic GC during VM allocation loops.
	// Runs FullGC (mark/sweep + weak table clearing) AND drains
	// pending finalizers (__gc). This is required for gc.lua tests
	// where allocation loops depend on __gc setting a finish flag.
	// The traverseThread function marks ALL allocated stack slots,
	// preventing premature collection of live registers.
	ls.Global.GCStepFn = func(thread *state.LuaState) {
		g := thread.Global
		if g.GCRunning || g.GCRunningFinalizer || g.GCStopped {
			return
		}
		g.GCRunning = true
		// NOTE: No clearStaleStack for periodic GC — traverseThread uses
		// maxTop (conservative) which limits marking to active frames.
		// clearStaleStack is only needed for explicit GC (collectgarbage()).
		gc.FullGC(g, thread)
		g.GCRunning = false
		// Drain pending finalizers — objects moved to tobefnz by
		// separateTobeFnz in FullGC need their __gc called.
		wrapper := &State{Internal: thread}
		wrapper.callAllPendingFinalizers()
		// V5 GC handles weak tables natively via clearByValues/clearByKeys.
	}

	// GCDrainFn: just drain pending finalizers.
	ls.Global.GCDrainFn = func(thread *state.LuaState) {
		wrapper := &State{Internal: thread}
		wrapper.callAllPendingFinalizers()
	}

	return L
}

// Close releases all resources associated with the state.
func (L *State) Close() {
	state.CloseState(L.ls())
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
			ls.Stack[ls.Top].Val = object.Nil
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
	if ls.Top+n > state.MaxStack {
		return false
	}
	state.EnsureStack(ls, n)
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

func reverseStack(ls *state.LuaState, from, to int) {
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
	L.push(object.Nil)
}

// PushBoolean pushes a boolean.
func (L *State) PushBoolean(b bool) {
	L.push(object.MakeBoolean(b))
}

// PushInteger pushes an integer.
func (L *State) PushInteger(n int64) {
	L.push(object.MakeInteger(n))
}

// PushNumber pushes a float.
func (L *State) PushNumber(n float64) {
	L.push(object.MakeFloat(n))
}

// PushString pushes a string. Returns the string.
func (L *State) PushString(s string) string {
	is := L.internStr(s)
	L.push(object.MakeString(is))
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
	L.push(object.TValue{Tt: object.TagLightCFunc, Val: wrapped})
}

// PushCFunctionSame pushes a pre-wrapped C function value. Unlike PushCFunction,
// which creates a new wrapper each call, this pushes the exact same TValue each time.
// Use for stateless iterators (e.g., ipairs) where identity must be preserved.
func (L *State) PushCFunctionSame(tv object.TValue) {
	L.push(tv)
}

// WrapCFunction wraps a CFunction into a state.CFunction + TValue.
// The caller can cache the result and pass it to PushCFunctionSame.
func WrapCFunction(f CFunction) object.TValue {
	wrapped := wrapCFunctionStatic(f)
	return object.TValue{Tt: object.TagLightCFunc, Val: wrapped}
}

// PushCClosure pushes a Go function as a closure with n upvalues.
func (L *State) PushCClosure(f CFunction, n int) {
	if n == 0 {
		L.PushCFunction(f)
		return
	}
	ls := L.ls()
	wrapped := wrapCFunctionStatic(f)
	cc := closure.NewCClosure(wrapped, n)
	ls.Global.LinkGC(cc) // V5: register in allgc chain
	// Pop n upvalues from stack into the closure
	for i := n; i >= 1; i-- {
		ls.Top--
		cc.UpVals[i-1] = ls.Stack[ls.Top].Val
	}
	L.push(object.TValue{Tt: object.TagCClosure, Val: cc})
}

// PushLightUserdata pushes a light userdata.
func (L *State) PushLightUserdata(p interface{}) {
	L.push(object.TValue{Tt: object.TagLightUserdata, Val: p})
}

// PushGlobalTable pushes the global table.
func (L *State) PushGlobalTable() {
	L.RawGetI(RegistryIndex, int64(RIdxGlobals))
}

// ---------------------------------------------------------------------------
// Type checking
// ---------------------------------------------------------------------------

// Type returns the type of the value at idx.
func (L *State) Type(idx int) object.Type {
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
func (L *State) TypeName(tp object.Type) string {
	names := [...]string{
		object.TypeNil:           "nil",
		object.TypeBoolean:       "boolean",
		object.TypeLightUserdata: "userdata",
		object.TypeNumber:        "number",
		object.TypeString:        "string",
		object.TypeTable:         "table",
		object.TypeFunction:      "function",
		object.TypeUserdata:      "userdata",
		object.TypeThread:        "thread",
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
	return L.Type(idx) == object.TypeNil
}

// IsNone returns true if the index is not valid.
func (L *State) IsNone(idx int) bool {
	return L.Type(idx) == TypeNone
}

// IsNoneOrNil returns true if the index is not valid or the value is nil.
func (L *State) IsNoneOrNil(idx int) bool {
	tp := L.Type(idx)
	return tp == TypeNone || tp == object.TypeNil
}

// IsBoolean returns true if the value is a boolean.
func (L *State) IsBoolean(idx int) bool {
	return L.Type(idx) == object.TypeBoolean
}

// IsInteger returns true if the value is an integer.
func (L *State) IsInteger(idx int) bool {
	v := L.index2val(idx)
	return v.Tt == object.TagInteger
}

// IsNumber returns true if the value is a number or convertible string.
func (L *State) IsNumber(idx int) bool {
	_, ok := L.ToNumber(idx)
	return ok
}

// IsString returns true if the value is a string or a number.
func (L *State) IsString(idx int) bool {
	tp := L.Type(idx)
	return tp == object.TypeString || tp == object.TypeNumber
}

// IsFunction returns true if the value is a function.
func (L *State) IsFunction(idx int) bool {
	return L.Type(idx) == object.TypeFunction
}

// IsTable returns true if the value is a table.
func (L *State) IsTable(idx int) bool {
	return L.Type(idx) == object.TypeTable
}

// IsCFunction returns true if the value is a C/Go function.
func (L *State) IsCFunction(idx int) bool {
	v := L.index2val(idx)
	return v.Tt == object.TagLightCFunc || v.Tt == object.TagCClosure
}

// IsUserdata returns true if the value is a userdata.
func (L *State) IsUserdata(idx int) bool {
	tp := L.Type(idx)
	return tp == object.TypeUserdata
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
	case object.TagInteger:
		return v.Val.(int64), true
	case object.TagFloat:
		return object.FloatToInteger(v.Val.(float64))
	case object.TagShortStr, object.TagLongStr:
		s := v.Val.(*object.LuaString).Data
		return object.StringToInteger(s)
	default:
		return 0, false
	}
}

// ToNumber converts the value to float.
func (L *State) ToNumber(idx int) (float64, bool) {
	v := L.index2val(idx)
	switch v.Tt {
	case object.TagFloat:
		return v.Val.(float64), true
	case object.TagInteger:
		return float64(v.Val.(int64)), true
	case object.TagShortStr, object.TagLongStr:
		s := v.Val.(*object.LuaString).Data
		tv, ok := object.StringToNumber(s)
		if !ok {
			return 0, false
		}
		if tv.Tt == object.TagFloat {
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
	case object.TagShortStr, object.TagLongStr:
		return v.Val.(*object.LuaString).Data, true
	case object.TagInteger:
		s := fmt.Sprintf("%d", v.Val.(int64))
		// Coerce in-place
		is := L.internStr(s)
		*v = object.MakeString(is)
		return s, true
	case object.TagFloat:
		s := object.FloatToString(v.Val.(float64))
		is := L.internStr(s)
		*v = object.MakeString(is)
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
	case object.TagTable:
		return v.Val.(*table.Table).RawLen()
	case object.TagShortStr, object.TagLongStr:
		return int64(len(v.Val.(*object.LuaString).Data))
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// String reader (for Load)
// ---------------------------------------------------------------------------

type stringReader struct {
	data string
	pos  int
}

func (r *stringReader) NextByte() int {
	if r.pos >= len(r.data) {
		return -1
	}
	b := r.data[r.pos]
	r.pos++
	return int(b)
}

// Ensure unused imports are used
var _ = parse.Parse
var _ = lex.TK_EOS

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
		s := object.FloatToString(v.Float())
		L.PushString(s)
		return s
	case v.Tt == object.TagTrue:
		L.PushString("true")
		return "true"
	case v.Tt == object.TagFalse:
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
	if tp == object.TypeNil {
		L.Pop(2) // pop nil and metatable
		return false
	}
	L.Remove(-2) // remove metatable, keep field value
	return true
}

// StringToNumber tries to convert a string to a number and pushes it.
// Returns the length+1 on success, 0 on failure.
func (L *State) StringToNumber(s string) int {
	tv, ok := object.StringToNumber(s)
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
	if L.GetField(idx, fname) == object.TypeTable {
		return true // table already there
	}
	L.Pop(1) // remove previous result
	idx = L.AbsIndex(idx)
	L.NewTable()
	L.PushValue(-1)        // copy to be left at top
	L.SetField(idx, fname) // assign new table to field
	return false
}

// NewMetatable creates a new metatable in the registry with the given name.
// If the registry already has a table with that name, pushes it and returns false.
// Otherwise creates a new table, stores it in registry[tname], and returns true.
// Mirrors: luaL_newmetatable in lauxlib.c
func (L *State) NewMetatable(tname string) bool {
	if L.GetField(RegistryIndex, tname) != object.TypeNil {
		// Already exists — it's on the stack
		return false
	}
	L.Pop(1) // remove nil
	L.NewTable()
	L.PushValue(-1)                  // dup the table
	L.SetField(RegistryIndex, tname) // registry[tname] = table
	return true
}

// TestUdata checks if the value at idx is a userdata with metatable matching registry[tname].
// Returns true if it matches, false otherwise. Does not modify the stack.
// Mirrors: luaL_testudata in lauxlib.c
func (L *State) TestUdata(idx int, tname string) bool {
	v := L.index2val(idx)
	if v.Tt != object.TagUserdata {
		return false
	}
	ud, ok := v.Val.(*object.Userdata)
	if !ok || ud.MetaTable == nil {
		return false
	}
	// Get the expected metatable from registry
	L.GetField(RegistryIndex, tname)
	expectedMT := L.index2val(-1)
	L.Pop(1)
	// Compare metatable pointers
	if expectedMT.Tt != object.TagTable {
		return false
	}
	mt, ok := ud.MetaTable.(*table.Table)
	if !ok {
		return false
	}
	return mt == expectedMT.Val.(*table.Table)
}

// CheckUdata checks that the value at idx is a userdata with metatable matching registry[tname].
// Raises a type error if not. Mirrors: luaL_checkudata in lauxlib.c
func (L *State) CheckUdata(idx int, tname string) {
	if !L.TestUdata(idx, tname) {
		L.TypeError(idx, tname)
	}
}

// GetLClosure returns the LClosure at the given stack index, or nil if not a Lua closure.
func (L *State) GetLClosure(idx int) *closure.LClosure {
	v := L.index2val(idx)
	if v.Tt != object.TagLuaClosure {
		return nil
	}
	return v.Val.(*closure.LClosure)
}

// DebugGetProto returns the Proto of the LClosure at top of stack (for testing).
func DebugGetProto(L *State) *object.Proto {
	ls := L.ls()
	if ls.Top <= 0 {
		return nil
	}
	fval := ls.Stack[ls.Top-1].Val
	if fval.Tt != object.TagLuaClosure {
		return nil
	}
	cl := fval.Val.(*closure.LClosure)
	return cl.Proto
}

// PushFuncFromDebug pushes the function associated with a DebugInfo onto the stack.
// Returns true if successful, false if the CI is nil or invalid.
func (L *State) PushFuncFromDebug(ar *DebugInfo) bool {
	if ar == nil || ar.CI == nil {
		return false
	}
	ci, ok := ar.CI.(*state.CallInfo)
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
