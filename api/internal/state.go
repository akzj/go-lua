// Package internal provides the concrete implementation of api.LuaAPI.
// Implements lua.h functions (lua_push*, lua_pop*, lua_get*, lua_set*, lua_call, etc.)
//
// Reference: lua-master/lapi.c
//
// Design constraints:
// - Implements api.LuaAPI interface
// - Delegates to state/internal for thread management
// - Wraps state.LuaStateInterface with C API semantics
package internal

import (
	"fmt"
	"unsafe"

	luaapi "github.com/akzj/go-lua/api/api"
	memapi "github.com/akzj/go-lua/mem/api"
	"github.com/akzj/go-lua/state"
	stateapi "github.com/akzj/go-lua/state/api"
	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
)

// LuaState is the concrete implementation of api.LuaAPI.
// It wraps state.LuaStateInterface and adds C API methods.
type LuaState struct {
	delegate stateapi.LuaStateInterface
	registry tableapi.TableInterface
	// Local stack for API-level values (mirrors delegate stack but with proper types)
	stack []luaValue
	top   int
}

// luaValue represents a Lua value on the API stack.
type luaValue struct {
	tp   int    // type tag
	data interface{} // actual value
}

// =============================================================================
// Value Conversion Helpers (between luaValue and types.TValue)
// =============================================================================

// typesTValue is a concrete implementation of types.TValue for conversions
type typesTValue struct {
	tt    uint8
	data_ interface{}
}

func (t *typesTValue) IsNil() bool              { return types.Novariant(int(t.tt)) == types.LUA_TNIL }
func (t *typesTValue) IsBoolean() bool           { return types.Novariant(int(t.tt)) == types.LUA_TBOOLEAN }
func (t *typesTValue) IsNumber() bool            { return types.Novariant(int(t.tt)) == types.LUA_TNUMBER }
func (t *typesTValue) IsInteger() bool           { return int(t.tt) == types.LUA_VNUMINT }
func (t *typesTValue) IsFloat() bool             { return int(t.tt) == types.LUA_VNUMFLT }
func (t *typesTValue) IsString() bool            { return types.Novariant(int(t.tt)) == types.LUA_TSTRING }
func (t *typesTValue) IsTable() bool             { return int(t.tt) == types.Ctb(int(types.LUA_VTABLE)) }
func (t *typesTValue) IsFunction() bool          { return types.Novariant(int(t.tt)) == types.LUA_TFUNCTION }
func (t *typesTValue) IsThread() bool            { return int(t.tt) == types.Ctb(int(types.LUA_VTHREAD)) }
func (t *typesTValue) IsLightUserData() bool     { return int(t.tt) == types.LUA_VLIGHTUSERDATA }
func (t *typesTValue) IsUserData() bool          { return int(t.tt) == types.Ctb(int(types.LUA_VUSERDATA)) }
func (t *typesTValue) IsCollectable() bool       { return int(t.tt)&types.BIT_ISCOLLECTABLE != 0 }
func (t *typesTValue) IsTrue() bool              { return int(t.tt) == types.LUA_VTRUE }
func (t *typesTValue) IsFalse() bool             { return int(t.tt) == types.LUA_VFALSE }
func (t *typesTValue) IsLClosure() bool          { return int(t.tt) == types.Ctb(int(types.LUA_VLCL)) }
func (t *typesTValue) IsCClosure() bool          { return int(t.tt) == types.Ctb(int(types.LUA_VCCL)) }
func (t *typesTValue) IsLightCFunction() bool    { return int(t.tt) == types.LUA_VLCF }
func (t *typesTValue) IsClosure() bool            { return t.IsLClosure() || t.IsCClosure() }
func (t *typesTValue) IsProto() bool              { return int(t.tt) == types.Ctb(int(types.LUA_VPROTO)) }
func (t *typesTValue) IsUpval() bool             { return int(t.tt) == types.Ctb(int(types.LUA_VUPVAL)) }
func (t *typesTValue) IsShortString() bool       { return int(t.tt) == types.Ctb(int(types.LUA_VSHRSTR)) }
func (t *typesTValue) IsLongString() bool        { return int(t.tt) == types.Ctb(int(types.LUA_VLNGSTR)) }
func (t *typesTValue) IsEmpty() bool             { return types.Novariant(int(t.tt)) == types.LUA_TNIL }
func (t *typesTValue) GetTag() int               { return int(t.tt) }
func (t *typesTValue) GetBaseType() int          { return types.Novariant(int(t.tt)) }
func (t *typesTValue) GetValue() interface{}     { return t.data_ }
func (t *typesTValue) GetGC() *types.GCObject    { return nil }
func (t *typesTValue) GetInteger() types.LuaInteger { return t.data_.(types.LuaInteger) }
func (t *typesTValue) GetFloat() types.LuaNumber  { return t.data_.(types.LuaNumber) }
func (t *typesTValue) GetPointer() unsafe.Pointer { return t.data_.(unsafe.Pointer) }

// compile-time interface check
var _ types.TValue = (*typesTValue)(nil)

// luaValueToTValue converts a luaValue to types.TValue
func (L *LuaState) luaValueToTValue(lv luaValue) types.TValue {
	tv := &typesTValue{
		tt:    uint8(lv.tp),
		data_: lv.data,
	}
	return tv
}

// tValueToLuaValue converts a types.TValue to luaValue
func (L *LuaState) tValueToLuaValue(tv types.TValue) luaValue {
	if tv.IsNil() {
		return luaValue{tp: types.LUA_TNIL, data: nil}
	}
	if tv.IsTrue() {
		return luaValue{tp: types.LUA_TBOOLEAN, data: true}
	}
	if tv.IsFalse() {
		return luaValue{tp: types.LUA_TBOOLEAN, data: false}
	}
	if tv.IsInteger() {
		return luaValue{tp: types.LUA_TNUMBER, data: tv.GetInteger()}
	}
	if tv.IsFloat() {
		return luaValue{tp: types.LUA_TNUMBER, data: float64(tv.GetFloat())}
	}
	if tv.IsLightUserData() {
		return luaValue{tp: types.LUA_TLIGHTUSERDATA, data: tv.GetPointer()}
	}
	if tv.IsString() {
		return luaValue{tp: types.LUA_TSTRING, data: tv.GetValue()}
	}
	if tv.IsTable() {
		return luaValue{tp: types.LUA_TTABLE, data: tv.GetValue()}
	}
	if tv.IsFunction() {
		return luaValue{tp: types.LUA_TFUNCTION, data: tv.GetValue()}
	}
	if tv.IsThread() {
		return luaValue{tp: types.LUA_TTHREAD, data: tv.GetValue()}
	}
	if tv.IsUserData() {
		return luaValue{tp: types.LUA_TUSERDATA, data: tv.GetValue()}
	}
	return luaValue{tp: types.LUA_TNIL, data: nil}
}

// NewLuaState creates a new LuaState with the given allocator.
func NewLuaState(alloc memapi.Allocator) *LuaState {
	// Use the state module's New which creates a proper LuaState
	// The allocator is handled internally by the state module
	_ = alloc // allocator currently not used - state module handles it
	return &LuaState{
		delegate: state.New(),
		registry:  nil,
		stack:    make([]luaValue, 20),
		top:      0,
	}
}

// syncStackToDelegate syncs the API stack to the delegate's stack.
// This is needed before calling state functions that expect values on the stack.
func (L *LuaState) syncStackToDelegate() {
	delegateStack := L.delegate.Stack()
	// Grow delegate stack if needed
	needed := L.top - len(delegateStack)
	if needed > 0 {
		L.delegate.GrowStack(needed)
		delegateStack = L.delegate.Stack() // Get the potentially new stack
	}
	// Convert each luaValue to TValue using index assignment only
	for i := 0; i < L.top; i++ {
		delegateStack[i] = L.luaValueToTValue(L.stack[i])
	}
	// Set delegate's top to match
	L.delegate.SetTop(L.top)
}

// syncStackFromDelegate syncs the delegate's stack back to the API stack.
// This is needed after state functions that modify the stack.
func (L *LuaState) syncStackFromDelegate() {
	delegateStack := L.delegate.Stack()
	delegateTop := L.delegate.StackSize()
	
	// Ensure API stack is large enough
	L.ensureCapacity(delegateTop)
	
	// Convert each TValue to luaValue
	for i := 0; i < delegateTop && i < cap(L.stack); i++ {
		L.stack[i] = L.tValueToLuaValue(delegateStack[i])
	}
	L.top = delegateTop
}

// Compile-time interface check
var _ luaapi.LuaAPI = (*LuaState)(nil)

// =============================================================================
// Thread Management (delegates to state)
// =============================================================================

func (L *LuaState) NewThread() luaapi.LuaAPI {
	// TODO: wrap the new thread with LuaState wrapper
	return nil
}

func (L *LuaState) Status() luaapi.Status {
	return L.delegate.Status()
}

// =============================================================================
// Stack Operations (delegates to state)
// =============================================================================

func (L *LuaState) PushValue(idx int) {
	L.delegate.PushValue(idx)
}

func (L *LuaState) Pop() {
	L.delegate.Pop()
}

func (L *LuaState) Top() int {
	return L.top
}

// GetTop returns the index of the top element in the stack.
// Matches lua_gettop from C API.
func (L *LuaState) GetTop() int {
	return L.Top()
}

func (L *LuaState) SetTop(idx int) {
	if idx < 0 {
		idx = 0
	}
	// Grow stack if needed
	for cap(L.stack) < idx {
		newStack := make([]luaValue, cap(L.stack)*2)
		copy(newStack, L.stack)
		L.stack = newStack
	}
	L.top = idx
}

// =============================================================================
// Function Calls
// =============================================================================

func (L *LuaState) Call(nArgs, nResults int) {
	// Get the function position
	funcIdx := L.absIndex(-nArgs - 1)
	if funcIdx < 1 || funcIdx > L.top {
		return
	}

	// Get the function
	fn := L.stack[funcIdx-1]

	// Check if this is a Go function (stored in our local stack)
	if goFn, ok := fn.data.(func(L luaapi.LuaAPI) int); ok && fn.tp == types.LUA_TFUNCTION {
		// Shift arguments to the front of the stack (where function was)
		// Stack before: [func, arg1, arg2]  (indices 0, 1, 2; funcIdx=1, top=3)
		// Stack after:  [arg1, arg2]       (indices 0, 1; top=2)
		// Formula: stack[i-1] = stack[funcIdx+i-1]
		for i := 1; i <= nArgs; i++ {
			L.stack[i-1] = L.stack[funcIdx+i-1]
		}
		L.top = funcIdx + nArgs - 1

		// Call the Go function - arguments now at positions 1, 2, ...
		nr := goFn(L)

		// Handle nResults:
		// If nResults >= 0, ensure exactly nResults values are on stack
		// If nResults == LUA_MULTRET (-1), keep all returned values
		if nResults >= 0 && nResults != luaapi.LUA_MULTRET {
			if nr < nResults {
				// Pad with nil
				for L.top < nResults {
					L.PushNil()
				}
			} else if nr > nResults {
				// Remove extra values from bottom
				L.top = nResults
			}
		}
		return
	}

	// For non-Go functions, sync stack and delegate to state
	L.syncStackToDelegate()
	L.delegate.Call(nArgs, nResults)
	// Sync back from delegate to maintain consistency
	L.syncStackFromDelegate()
}

func (L *LuaState) Resume() error {
	return L.delegate.Resume()
}

func (L *LuaState) Yield(nResults int) error {
	return L.delegate.Yield(nResults)
}

// =============================================================================
// Global State
// =============================================================================

func (L *LuaState) Global() stateapi.GlobalState {
	return L.delegate.Global()
}

// PushGlobalTable pushes the global table onto the stack.
// Matches lua_pushglobaltable from C API.
func (L *LuaState) PushGlobalTable() {
	L.PushInteger(0) // TODO: get global table properly - placeholder
}

// =============================================================================
// Internal (for VM integration)
// =============================================================================

func (L *LuaState) Stack() []types.TValue {
	return L.delegate.Stack()
}

func (L *LuaState) StackSize() int {
	return L.delegate.StackSize()
}

func (L *LuaState) GrowStack(n int) {
	L.delegate.GrowStack(n)
}

func (L *LuaState) CurrentCI() stateapi.CallInfo {
	return L.delegate.CurrentCI()
}

func (L *LuaState) PushCI(ci stateapi.CallInfo) {
	L.delegate.PushCI(ci)
}

func (L *LuaState) PopCI() {
	L.delegate.PopCI()
}

// =============================================================================
// Basic Stack Operations
// =============================================================================

func (L *LuaState) AbsIndex(idx int) int {
	return L.absIndex(idx)
}

func (L *LuaState) Rotate(idx int, n int) {
	// TODO: implement rotation
}

// Insert moves the top element to position idx.
// Implemented as Rotate(idx, 1) per lua_insert semantics.
func (L *LuaState) Insert(pos int) {
	L.Rotate(pos, 1)
}

func (L *LuaState) Copy(fromidx, toidx int) {
	// TODO: implement copy
}

func (L *LuaState) CheckStack(n int) bool {
	// TODO: implement proper check
	return true
}

func (L *LuaState) XMove(to luaapi.LuaAPI, n int) {
	// TODO: implement cross-state move
}

// =============================================================================
// Type Checking
// =============================================================================

func (L *LuaState) Type(idx int) int {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		return luaapi.LUA_TNONE
	}
	if absIdx > len(L.stack) {
		return luaapi.LUA_TNONE
	}
	val := L.stack[absIdx-1]
	if val.tp == 0 && val.data == nil {
		return types.LUA_TNIL
	}
	return val.tp
}

// absIndex converts a stack index to absolute position.
func (L *LuaState) absIndex(idx int) int {
	if idx > 0 {
		return idx
	}
	if idx < 0 {
		return L.top + idx + 1
	}
	return idx
}

func (L *LuaState) TypeName(tp int) string {
	return luaapi.Typename(tp)
}

func (L *LuaState) IsNone(idx int) bool {
	return L.Type(idx) == luaapi.LUA_TNONE
}

func (L *LuaState) IsNil(idx int) bool {
	return L.Type(idx) == types.LUA_TNIL
}

func (L *LuaState) IsNoneOrNil(idx int) bool {
	tp := L.Type(idx)
	return tp == luaapi.LUA_TNONE || tp == types.LUA_TNIL
}

func (L *LuaState) IsBoolean(idx int) bool {
	return L.Type(idx) == types.LUA_TBOOLEAN
}

func (L *LuaState) IsString(idx int) bool {
	tp := L.Type(idx)
	return tp == types.LUA_TSTRING || tp == types.LUA_TNUMBER
}

func (L *LuaState) IsFunction(idx int) bool {
	return L.Type(idx) == types.LUA_TFUNCTION
}

func (L *LuaState) IsTable(idx int) bool {
	return L.Type(idx) == types.LUA_TTABLE
}

func (L *LuaState) IsLightUserData(idx int) bool {
	return L.Type(idx) == types.LUA_TLIGHTUSERDATA
}

func (L *LuaState) IsThread(idx int) bool {
	return L.Type(idx) == types.LUA_TTHREAD
}

func (L *LuaState) IsInteger(idx int) bool {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		return false
	}
	if absIdx > len(L.stack) {
		return false
	}
	val := L.stack[absIdx-1]
	if val.tp == 0 && val.data == nil {
		return false
	}
	// Check if data is int64
	_, ok := val.data.(int64)
	return val.tp == types.LUA_TNUMBER && ok
}

func (L *LuaState) IsNumber(idx int) bool {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		return false
	}
	if absIdx > len(L.stack) {
		return false
	}
	val := L.stack[absIdx-1]
	if val.tp == 0 && val.data == nil {
		return false
	}
	return val.tp == types.LUA_TNUMBER
}

// =============================================================================
// Value Conversion
// =============================================================================

func (L *LuaState) ToInteger(idx int) (int64, bool) {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		return 0, false
	}
	if absIdx > len(L.stack) {
		return 0, false
	}
	val := L.stack[absIdx-1]
	if val.tp == 0 && val.data == nil {
		return 0, false
	}
	if val.tp == types.LUA_TNUMBER {
		if i, ok := val.data.(int64); ok {
			return i, true
		}
	}
	return 0, false
}

func (L *LuaState) ToNumber(idx int) (float64, bool) {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		return 0, false
	}
	if absIdx > len(L.stack) {
		return 0, false
	}
	val := L.stack[absIdx-1]
	if val.tp == 0 && val.data == nil {
		return 0, false
	}
	if val.tp == types.LUA_TNUMBER {
		switch v := val.data.(type) {
		case int64:
			return float64(v), true
		case float64:
			return v, true
		}
	}
	return 0, false
}

func (L *LuaState) ToString(idx int) (string, bool) {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		return "", false
	}
	if absIdx > len(L.stack) {
		return "", false
	}
	val := L.stack[absIdx-1]
	if val.tp == 0 && val.data == nil {
		return "", false
	}
	if val.tp == types.LUA_TSTRING {
		if s, ok := val.data.(string); ok {
			return s, true
		}
	}
	return "", false
}

func (L *LuaState) ToBoolean(idx int) bool {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		return false
	}
	if absIdx > len(L.stack) {
		return false
	}
	val := L.stack[absIdx-1]
	if val.tp == 0 && val.data == nil {
		return false
	}
	if val.tp == types.LUA_TBOOLEAN {
		if b, ok := val.data.(bool); ok {
			return b
		}
	}
	// In Lua, only false and nil are falsy
	return val.tp != types.LUA_TNIL && val.tp != 0
}

func (L *LuaState) ToPointer(idx int) interface{} {
	// Not implemented for debugging only
	return nil
}

func (L *LuaState) ToThread(idx int) luaapi.LuaAPI {
	// TODO: implement thread conversion
	return nil
}

// =============================================================================
// Push Functions
// =============================================================================

func (L *LuaState) PushNil() {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: types.LUA_TNIL, data: nil}
	L.top++
}

func (L *LuaState) PushInteger(n int64) {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: types.LUA_TNUMBER, data: n}
	L.top++
}

func (L *LuaState) PushNumber(n float64) {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: types.LUA_TNUMBER, data: n}
	L.top++
}

func (L *LuaState) PushString(s string) {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: types.LUA_TSTRING, data: s}
	L.top++
}

func (L *LuaState) PushBoolean(b bool) {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: types.LUA_TBOOLEAN, data: b}
	L.top++
}

func (L *LuaState) PushLightUserData(p interface{}) {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: types.LUA_TLIGHTUSERDATA, data: p}
	L.top++
}

// GoFunction wraps a Go function to be called from Lua.
// It implements CFunction semantics (func(*State) int).
type GoFunction struct {
	Fn func(L luaapi.LuaAPI) int
}

// PushGoFunction pushes a Go function onto the stack.
// The function can then be called by Lua code.
func (L *LuaState) PushGoFunction(fn func(L luaapi.LuaAPI) int) {
	L.ensureCapacity(L.top + 1)
	// Store function with CClosure tag so the VM recognizes it as callable
	L.stack[L.top] = luaValue{
		tp:   types.LUA_TFUNCTION, // Base type is FUNCTION
		data: fn,                   // Store the Go function directly
	}
	L.top++
}

// ensureCapacity ensures the stack has capacity for n elements.
func (L *LuaState) ensureCapacity(n int) {
	if cap(L.stack) < n {
		newStack := make([]luaValue, n, n*2)
		copy(newStack, L.stack)
		L.stack = newStack
	}
}

// =============================================================================
// Table Operations
// =============================================================================

// getTableInterface extracts a TableInterface from a stack position
func (L *LuaState) getTableInterface(idx int) (tableapi.TableInterface, bool) {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		return nil, false
	}
	if absIdx > len(L.stack) {
		return nil, false
	}
	val := L.stack[absIdx-1]
	if val.tp != types.LUA_TTABLE {
		return nil, false
	}
	tbl, ok := val.data.(tableapi.TableInterface)
	return tbl, ok
}

// PushTValue pushes a types.TValue onto the stack
func (L *LuaState) pushTValue(tv types.TValue) {
	result := L.tValueToLuaValue(tv)
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = result
	L.top++
}

// NewTValueNil creates a new nil TValue for use as placeholder
func NewTValueNil() types.TValue {
	return &typesTValue{tt: uint8(types.LUA_VNIL), data_: nil}
}

// RawGet gets value at table[key] without metamethods.
// Returns type of the pushed value.
func (L *LuaState) RawGet(idx int) int {
	tbl, ok := L.getTableInterface(idx)
	if !ok {
		L.PushNil()
		return types.LUA_TNIL
	}
	// Key is at stack position -1
	if L.top < 1 {
		L.PushNil()
		return types.LUA_TNIL
	}
	key := L.luaValueToTValue(L.stack[L.top-1])
	L.top-- // Pop the key
	result := tbl.Get(key)
	L.pushTValue(result)
	return L.Type(-1)
}

// RawGetI gets value at table[n] without metamethods.
// Returns type of the pushed value.
func (L *LuaState) RawGetI(idx int, n int64) int {
	tbl, ok := L.getTableInterface(idx)
	if !ok {
		L.PushNil()
		return types.LUA_TNIL
	}
	result := tbl.GetInt(types.LuaInteger(n))
	L.pushTValue(result)
	return L.Type(-1)
}

// RawSet sets table[key] = value without metamethods.
// Key and value are popped from the stack.
func (L *LuaState) RawSet(idx int) {
	tbl, ok := L.getTableInterface(idx)
	if !ok {
		return
	}
	// Stack: ..., table, key, value
	if L.top < 3 {
		return
	}
	value := L.luaValueToTValue(L.stack[L.top-1])
	key := L.luaValueToTValue(L.stack[L.top-2])
	L.top -= 2 // Pop key and value
	tbl.Set(key, value)
}

// RawSetI sets table[n] = value without metamethods.
// Value is popped from the stack.
func (L *LuaState) RawSetI(idx int, n int64) {
	tbl, ok := L.getTableInterface(idx)
	if !ok {
		return
	}
	if L.top < 2 {
		return
	}
	value := L.luaValueToTValue(L.stack[L.top-1])
	L.top-- // Pop value
	tbl.SetInt(types.LuaInteger(n), value)
}

// GetTable gets value at table[key] with metamethods.
// Returns type of the pushed value.
func (L *LuaState) GetTable(idx int) int {
	// For now, implement without metamethods
	return L.RawGet(idx)
}

// GetField gets value at table.key, pushes result onto stack.
// Returns type of the pushed value.
func (L *LuaState) GetField(idx int, k string) int {
	tbl, ok := L.getTableInterface(idx)
	if !ok {
		L.PushNil()
		return types.LUA_TNIL
	}
	// Create string key as TValue
	key := &typesTValue{
		tt:    uint8(types.Ctb(int(types.LUA_VSHRSTR))),
		data_: k,
	}
	result := tbl.Get(key)
	L.pushTValue(result)
	return L.Type(-1)
}

// GetI gets value at table[n], pushes result onto stack.
// Returns type of the pushed value.
func (L *LuaState) GetI(idx int, n int64) int {
	// For now, implement without metamethods
	return L.RawGetI(idx, n)
}

// SetTable pops key and value from stack, sets table[key] = value.
// Uses metamethods.
func (L *LuaState) SetTable(idx int) {
	// For now, implement without metamethods
	L.RawSet(idx)
}

// SetField sets table.key = value (value on stack).
// Uses metamethods.
func (L *LuaState) SetField(idx int, k string) {
	tbl, ok := L.getTableInterface(idx)
	if !ok {
		return
	}
	if L.top < 2 {
		return
	}
	value := L.luaValueToTValue(L.stack[L.top-1])
	// Create string key as TValue
	key := &typesTValue{
		tt:    uint8(types.Ctb(int(types.LUA_VSHRSTR))),
		data_: k,
	}
	L.top-- // Pop value
	tbl.Set(key, value)
}

// SetI sets table[n] = value (value on stack).
// Uses metamethods.
func (L *LuaState) SetI(idx int, n int64) {
	tbl, ok := L.getTableInterface(idx)
	if !ok {
		return
	}
	if L.top < 2 {
		return
	}
	value := L.luaValueToTValue(L.stack[L.top-1])
	L.top-- // Pop value
	tbl.SetInt(types.LuaInteger(n), value)
}

// CreateTable creates a new table and pushes it onto stack.
func (L *LuaState) CreateTable(narr, nrec int) {
	L.ensureCapacity(L.top + 1)
	tbl := tableapi.NewTable(nil)
	L.stack[L.top] = luaValue{tp: types.LUA_TTABLE, data: tbl}
	L.top++
}

// GetGlobal gets global variable value, pushes onto stack.
// Returns type of the pushed value.
func (L *LuaState) GetGlobal(name string) int {
	// Get the global table from registry
	globalTbl := L.delegate.Global().Registry()
	// Create string key
	key := &typesTValue{
		tt:    uint8(types.Ctb(int(types.LUA_VSHRSTR))),
		data_: name,
	}
	result := globalTbl.Get(key)
	L.pushTValue(result)
	return L.Type(-1)
}

// SetGlobal sets a global variable. Pops value from stack.
func (L *LuaState) SetGlobal(name string) {
	// Get the global table from registry
	globalTbl := L.delegate.Global().Registry()
	if L.top < 1 {
		return
	}
	value := L.luaValueToTValue(L.stack[L.top-1])
	// Create string key
	key := &typesTValue{
		tt:    uint8(types.Ctb(int(types.LUA_VSHRSTR))),
		data_: name,
	}
	L.top-- // Pop value
	globalTbl.Set(key, value)
}

// Next pops key from stack, pushes next key-value pair from table at idx.
// Returns true if there is a next pair, false if iteration is complete.
func (L *LuaState) Next(idx int) bool {
	tbl, ok := L.getTableInterface(idx)
	if !ok {
		return false
	}
	// Get the key from stack (pop it)
	var key types.TValue
	if L.top < 1 {
		// No key on stack, start from beginning
		key = NewTValueNil()
	} else {
		key = L.luaValueToTValue(L.stack[L.top-1])
		L.top-- // Pop the key
	}
	// Get next key-value pair
	nextKey, nextValue, ok := tbl.Next(key)
	if !ok {
		// Push nil to indicate end of iteration
		L.PushNil()
		return false
	}
	// Push the key and value onto stack
	L.pushTValue(nextKey)
	L.pushTValue(nextValue)
	return true
}

// Concat concatenates n values from the top of the stack.
// Pops n values, pushes their concatenation.
func (L *LuaState) Concat(n int) {
	if n < 1 {
		return
	}
	if L.top < n {
		n = L.top
	}
	// Collect the values
	var result string
	for i := L.top - n; i < L.top; i++ {
		val := L.stack[i]
		switch v := val.data.(type) {
		case string:
			result += v
		case int64:
			result += fmt.Sprintf("%d", v)
		case float64:
			result += fmt.Sprintf("%g", v)
		default:
			// Skip non-stringable values for now
		}
	}
	// Pop the original values
	L.top -= n
	// Push the result
	L.PushString(result)
}

// Len pushes the length of the value at idx onto the stack.
// Works with __len metamethod (simplified: raw length).
func (L *LuaState) Len(idx int) {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		L.PushInteger(0)
		return
	}
	val := L.stack[absIdx-1]
	switch v := val.data.(type) {
	case string:
		L.PushInteger(int64(len(v)))
	case tableapi.TableInterface:
		L.PushInteger(int64(v.Len()))
	default:
		L.PushInteger(0)
	}
}

// Compare compares two values.
// op: LUA_OPEQ (0), LUA_OPLT (1), LUA_OPLE (2)
// Returns true if comparison succeeds.
func (L *LuaState) Compare(idx1, idx2 int, op int) bool {
	absIdx1 := L.absIndex(idx1)
	absIdx2 := L.absIndex(idx2)
	if absIdx1 < 1 || absIdx1 > L.top || absIdx2 < 1 || absIdx2 > L.top {
		return false
	}
	if absIdx1 > len(L.stack) || absIdx2 > len(L.stack) {
		return false
	}
	val1 := L.stack[absIdx1-1]
	val2 := L.stack[absIdx2-1]
	switch op {
	case luaapi.LUA_OPEQ:
		return L.compareEqual(val1, val2)
	case luaapi.LUA_OPLT:
		return L.compareLess(val1, val2)
	case luaapi.LUA_OPLE:
		return L.compareLessOrEqual(val1, val2)
	}
	return false
}

// compareEqual checks if two luaValues are equal
func (L *LuaState) compareEqual(a, b luaValue) bool {
	// Different types are not equal
	if a.tp != b.tp {
		return false
	}
	// Nil
	if a.tp == types.LUA_TNIL {
		return true
	}
	// Boolean
	if a.tp == types.LUA_TBOOLEAN {
		return a.data == b.data
	}
	// Number
	if a.tp == types.LUA_TNUMBER {
		ai, aok := a.data.(int64)
		bi, bok := b.data.(int64)
		if aok && bok {
			return ai == bi
		}
		af, _ := a.data.(float64)
		bf, _ := b.data.(float64)
		return af == bf
	}
	// For other types, compare by value
	return a.data == b.data
}

// compareLess compares a < b for numbers
func (L *LuaState) compareLess(a, b luaValue) bool {
	if a.tp != types.LUA_TNUMBER || b.tp != types.LUA_TNUMBER {
		return false
	}
	ai, aok := a.data.(int64)
	bi, bok := b.data.(int64)
	if aok && bok {
		return ai < bi
	}
	af := float64(0)
	bf := float64(0)
	if aok {
		af = float64(ai)
	} else {
		af, _ = a.data.(float64)
	}
	if bok {
		bf = float64(bi)
	} else {
		bf, _ = b.data.(float64)
	}
	return af < bf
}

// compareLessOrEqual compares a <= b for numbers
func (L *LuaState) compareLessOrEqual(a, b luaValue) bool {
	if a.tp != types.LUA_TNUMBER || b.tp != types.LUA_TNUMBER {
		return false
	}
	ai, aok := a.data.(int64)
	bi, bok := b.data.(int64)
	if aok && bok {
		return ai <= bi
	}
	af := float64(0)
	bf := float64(0)
	if aok {
		af = float64(ai)
	} else {
		af, _ = a.data.(float64)
	}
	if bok {
		bf = float64(bi)
	} else {
		bf, _ = b.data.(float64)
	}
	return af <= bf
}

// RawLen returns the raw length of the object at idx.
// For strings: byte length
// For tables: array part size (without metamethods)
func (L *LuaState) RawLen(idx int) uint {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		return 0
	}
	if absIdx > len(L.stack) {
		return 0
	}
	val := L.stack[absIdx-1]
	switch v := val.data.(type) {
	case string:
		return uint(len(v))
	case tableapi.TableInterface:
		return uint(v.Len())
	default:
		return 0
	}
}

// =============================================================================
// Metatable Operations
// =============================================================================

func (L *LuaState) GetMetatable(idx int) bool {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		return false
	}
	if absIdx > len(L.stack) {
		return false
	}
	
	val := L.stack[absIdx-1]
	if val.tp == 0 && val.data == nil {
		return false
	}
	
	// Only tables and userdata can have metatables
	if val.tp != types.LUA_TTABLE && val.tp != types.LUA_TUSERDATA {
		return false
	}
	
	// Try to get TableInterface
	tbl, ok := val.data.(tableapi.TableInterface)
	if !ok {
		return false
	}
	
	mt := tbl.GetMetatable()
	if mt == nil {
		return false
	}
	
	// Push the metatable onto the stack
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: types.LUA_TTABLE, data: mt}
	L.top++
	return true
}

func (L *LuaState) SetMetatable(idx int) {
	absIdx := L.absIndex(idx)
	if absIdx < 1 || absIdx > L.top {
		return
	}
	if absIdx > len(L.stack) {
		return
	}
	
	// The metatable should be at the top of the stack
	if L.top < 1 {
		return
	}
	
	// Get the metatable value (must be a table or nil)
	mtVal := L.stack[L.top-1]
	if mtVal.tp != types.LUA_TTABLE && mtVal.tp != types.LUA_TNIL {
		return
	}
	
	// Get the target value
	val := L.stack[absIdx-1]
	
	// Only tables and userdata can have metatables
	if val.tp != types.LUA_TTABLE && val.tp != types.LUA_TUSERDATA {
		return
	}
	
	// Get the target as TableInterface
	tbl, ok := val.data.(tableapi.TableInterface)
	if !ok {
		return
	}
	
	// Set the metatable
	if mtVal.tp == types.LUA_TNIL {
		tbl.SetMetatable(nil)
	} else {
		// Cast the metatable value to types.Table
		// Both TableImpl and types.Table work here through the interface
		if mt, ok := mtVal.data.(types.Table); ok {
			tbl.SetMetatable(mt)
		}
	}
	
	// Pop the metatable from the stack
	L.top--
}

// =============================================================================
// Call Operations
// =============================================================================

func (L *LuaState) PCall(nArgs, nResults, errfunc int) int {
	// TODO: implement protected call
	L.delegate.Call(nArgs, nResults)
	return int(luaapi.LUA_OK)
}

// =============================================================================
// Error Handling
// =============================================================================

// Error raises a Lua error.
// The error value is the value at the top of the stack.
// This function never returns (longjmp in C).
// Returns the error code (LUA_ERRRUN).
func (L *LuaState) Error() int {
	// Get the error value from the top of the stack
	if L.top < 1 {
		// No error value on stack, create a generic error
		L.PushString("runtime error")
	}
	// Return the error code - caller should longjmp
	// In Go, this means the caller should recover and handle the error
	return int(luaapi.LUA_ERRRUN)
}

// ErrorMessage raises an error with the value at the top of the stack.
// This is like Error() but keeps the error message on the stack.
func (L *LuaState) ErrorMessage() int {
	// Just return the error code - error stays on stack
	return int(luaapi.LUA_ERRRUN)
}

// Where prepends location information to the error message.
// lvl: call level (1 = where the current function is)
// Pushes a location string onto the stack.
func (L *LuaState) Where(level int) {
	var location string
	
	// Walk up the call stack
	ci := L.CurrentCI()
	for i := 1; i <= level; i++ {
		if ci == nil {
			break
		}
		ci = ci.Prev()
	}
	
	if ci == nil {
		// No more call info, provide a generic location
		location = "[Golang]"
	} else {
		// Get the function at this call level
		funcIdx := ci.Func()
		if funcIdx >= 0 && funcIdx < len(L.stack) {
			tv := L.stack[funcIdx]
			// Try to get source/line info from closure
			location = L.getFuncLocation(tv)
		} else {
			location = "[Golang]"
		}
	}
	
	// Push location string onto stack
	L.PushString(location)
}

// getFuncLocation returns a source:line string for a function value.
func (L *LuaState) getFuncLocation(tv luaValue) string {
	// Currently return a placeholder - full line info requires
	// more integration with the VM's closure tracking
	return "[Golang]"
}

// =============================================================================
// GC Control
// =============================================================================

// GC implements the lua_gc function for garbage collector control.
// what: LUA_GC* constant specifying the operation
// args: additional arguments for specific operations
// Returns: operation-specific result
func (L *LuaState) GC(what int, args ...int) int {
	gc := L.delegate.Global().GC()
	if gc == nil {
		return 0
	}

	switch what {
	case luaapi.LUA_GCSTOP:
		// Stop the garbage collector
		gc.Stop()
		return 1

	case luaapi.LUA_GCRESTART:
		// Restart the garbage collector
		gc.Start()
		return 1

	case luaapi.LUA_GCCOLLECT:
		// Perform a full garbage collection cycle
		gc.Collect()
		return 1

	case luaapi.LUA_GCCOUNT:
		// Return the number of kilobytes of memory in use
		// GCCollector.BytesInUse returns bytes, convert to KB
		var bytes uint64
		if gs, ok := gc.(interface{ BytesInUse() uint64 }); ok {
			bytes = gs.BytesInUse()
		}
		return int(bytes / 1024)

	case luaapi.LUA_GCCOUNTB:
		// Return the remainder of bytes after dividing by 1024
		var bytes uint64
		if gs, ok := gc.(interface{ BytesInUse() uint64 }); ok {
			bytes = gs.BytesInUse()
		}
		return int(bytes % 1024)

	case luaapi.LUA_GCSTEP:
		// Perform an incremental garbage collection step
		// args[0] contains the step multiplier (data/size multiplier)
		if len(args) > 0 && args[0] > 0 {
			// Step multiplier affects how much work is done
			// Multiple calls may be needed based on the multiplier
			steps := args[0] / 10
			if steps < 1 {
				steps = 1
			}
			for i := 0; i < steps; i++ {
				if !gc.Step() {
					// GC cycle completed
					return 1 // true in Lua (non-zero means completed)
				}
			}
			return 0 // more work to do
		}
		// Perform one step
		if gc.Step() {
			return 0 // more work to do
		}
		return 1 // cycle completed

	case luaapi.LUA_GCISRUNNING:
		// Return whether the GC is running
		if gs, ok := gc.(interface{ IsRunning() bool }); ok {
			if gs.IsRunning() {
				return 1
			}
		}
		return 0

	case 7: // LUA_GCSETPAUSE (not in standard but needed)
		// Set GC pause - args[0] is pause percentage (0-100)
		// In our implementation, this is handled by the collector
		if len(args) > 0 && args[0] > 0 {
			if gs, ok := gc.(interface{ SetPause(int) }); ok {
				gs.SetPause(args[0])
			}
			return args[0]
		}
		return 100

	case 8: // LUA_GCSETSTEPMUL (not in standard but needed)
		// Set GC step multiplier - args[0] is step multiplier (0-100000%)
		if len(args) > 0 && args[0] > 0 {
			if gs, ok := gc.(interface{ SetStepMul(int) }); ok {
				gs.SetStepMul(args[0])
			}
			return args[0]
		}
		return 100

	default:
		return 0
	}
}

// GCCount returns the number of kilobytes of memory in use by the garbage collector.
// This is a convenience method wrapping GC(LUA_GCCOUNT).
func (L *LuaState) GCCount() int {
	return L.GC(luaapi.LUA_GCCOUNT)
}

// GCCount64 returns the exact number of bytes of memory in use by the garbage collector.
func (L *LuaState) GCCount64() int64 {
	gc := L.delegate.Global().GC()
	if gc == nil {
		return 0
	}
	if gs, ok := gc.(interface{ BytesInUse() uint64 }); ok {
		return int64(gs.BytesInUse())
	}
	return 0
}

// GCStep performs an incremental garbage collection step.
// data: step size multiplier
// Returns true if the garbage collection cycle has completed.
func (L *LuaState) GCStep(data int) bool {
	if data <= 0 {
		data = 1
	}
	return L.GC(luaapi.LUA_GCSTEP, data) != 0
}

// GCStop stops the garbage collector.
func (L *LuaState) GCStop() {
	L.GC(luaapi.LUA_GCSTOP)
}

// GCRestart restarts the garbage collector.
func (L *LuaState) GCRestart() {
	L.GC(luaapi.LUA_GCRESTART)
}

// SetGCThreshold sets the GC threshold in bytes.
func (L *LuaState) SetGCThreshold(bytes int64) {
	gc := L.delegate.Global().GC()
	if gc == nil {
		return
	}
	if gs, ok := gc.(interface{ SetThreshold(uint64) }); ok {
		gs.SetThreshold(uint64(bytes))
	}
}

// SetPause sets the GC pause percentage.
// The pause controls how long the GC waits between cycles.
// value: pause percentage (e.g., 100 means wait until memory doubles)
func (L *LuaState) SetPause(value int) {
	if value <= 0 {
		value = 1
	}
	// Store in GC collector if it supports this
	gc := L.delegate.Global().GC()
	if gc == nil {
		return
	}
	if gs, ok := gc.(interface{ SetPause(int) }); ok {
		gs.SetPause(value)
	}
}

// SetStepMul sets the GC step multiplier.
// The step multiplier controls how much work is done per incremental step.
// value: step multiplier (e.g., 100 means normal speed)
func (L *LuaState) SetStepMul(value int) {
	if value <= 0 {
		value = 1
	}
	gc := L.delegate.Global().GC()
	if gc == nil {
		return
	}
	if gs, ok := gc.(interface{ SetStepMul(int) }); ok {
		gs.SetStepMul(value)
	}
}

// =============================================================================
// Registry Access
// =============================================================================

func (L *LuaState) Registry() tableapi.TableInterface {
	// TODO: implement registry access
	return nil
}

func (L *LuaState) Ref(t tableapi.TableInterface) int {
	// TODO: implement ref
	return -1 // LUA_REFNIL
}

func (L *LuaState) UnRef(t tableapi.TableInterface, ref int) {
	// TODO: implement unref
}
