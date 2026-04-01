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
	memapi "github.com/akzj/go-lua/mem/api"
	"github.com/akzj/go-lua/state"
	stateapi "github.com/akzj/go-lua/state/api"
	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"

	luaapi "github.com/akzj/go-lua/api/api"
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
	L.delegate.Call(nArgs, nResults)
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
		return luaapi.LUA_TNIL
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
	return L.Type(idx) == luaapi.LUA_TNIL
}

func (L *LuaState) IsNoneOrNil(idx int) bool {
	tp := L.Type(idx)
	return tp == luaapi.LUA_TNONE || tp == luaapi.LUA_TNIL
}

func (L *LuaState) IsBoolean(idx int) bool {
	return L.Type(idx) == luaapi.LUA_TBOOLEAN
}

func (L *LuaState) IsString(idx int) bool {
	tp := L.Type(idx)
	return tp == luaapi.LUA_TSTRING || tp == luaapi.LUA_TNUMBER
}

func (L *LuaState) IsFunction(idx int) bool {
	return L.Type(idx) == luaapi.LUA_TFUNCTION
}

func (L *LuaState) IsTable(idx int) bool {
	return L.Type(idx) == luaapi.LUA_TTABLE
}

func (L *LuaState) IsLightUserData(idx int) bool {
	return L.Type(idx) == luaapi.LUA_TLIGHTUSERDATA
}

func (L *LuaState) IsThread(idx int) bool {
	return L.Type(idx) == luaapi.LUA_TTHREAD
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
	return val.tp == luaapi.LUA_TNUMBER && ok
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
	return val.tp == luaapi.LUA_TNUMBER
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
	if val.tp == luaapi.LUA_TNUMBER {
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
	if val.tp == luaapi.LUA_TNUMBER {
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
	if val.tp == luaapi.LUA_TSTRING {
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
	if val.tp == luaapi.LUA_TBOOLEAN {
		if b, ok := val.data.(bool); ok {
			return b
		}
	}
	// In Lua, only false and nil are falsy
	return val.tp != luaapi.LUA_TNIL && val.tp != 0
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
	L.stack[L.top] = luaValue{tp: luaapi.LUA_TNIL, data: nil}
	L.top++
}

func (L *LuaState) PushInteger(n int64) {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: luaapi.LUA_TNUMBER, data: n}
	L.top++
}

func (L *LuaState) PushNumber(n float64) {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: luaapi.LUA_TNUMBER, data: n}
	L.top++
}

func (L *LuaState) PushString(s string) {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: luaapi.LUA_TSTRING, data: s}
	L.top++
}

func (L *LuaState) PushBoolean(b bool) {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: luaapi.LUA_TBOOLEAN, data: b}
	L.top++
}

func (L *LuaState) PushLightUserData(p interface{}) {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: luaapi.LUA_TLIGHTUSERDATA, data: p}
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

func (L *LuaState) GetTable(idx int) int {
	// TODO: implement table get with metamethods
	return luaapi.LUA_TNIL
}

func (L *LuaState) GetField(idx int, k string) int {
	// TODO: implement field get with metamethods
	return luaapi.LUA_TNIL
}

func (L *LuaState) GetI(idx int, n int64) int {
	// TODO: implement integer key get with metamethods
	return luaapi.LUA_TNIL
}

func (L *LuaState) RawGet(idx int) int {
	// TODO: implement raw table get without metamethods
	return luaapi.LUA_TNIL
}

func (L *LuaState) RawGetI(idx int, n int64) int {
	// TODO: implement raw table get with integer key
	return luaapi.LUA_TNIL
}

func (L *LuaState) CreateTable(narr, nrec int) {
	L.ensureCapacity(L.top + 1)
	L.stack[L.top] = luaValue{tp: luaapi.LUA_TTABLE, data: map[string]interface{}{}}
	L.top++
}

func (L *LuaState) SetTable(idx int) {
	// TODO: implement table set with metamethods
}

func (L *LuaState) SetField(idx int, k string) {
	// TODO: implement field set with metamethods
}

func (L *LuaState) SetI(idx int, n int64) {
	// TODO: implement integer key set with metamethods
}

func (L *LuaState) RawSet(idx int) {
	// TODO: implement raw table set without metamethods
}

func (L *LuaState) RawSetI(idx int, n int64) {
	// TODO: implement raw table set with integer key
}

func (L *LuaState) GetGlobal(name string) int {
	// TODO: implement global get
	return luaapi.LUA_TNIL
}

func (L *LuaState) SetGlobal(name string) {
	// TODO: implement global set
}

// =============================================================================
// Metatable Operations
// =============================================================================

func (L *LuaState) GetMetatable(idx int) bool {
	// TODO: implement metatable get
	return false
}

func (L *LuaState) SetMetatable(idx int) {
	// TODO: implement metatable set
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

func (L *LuaState) Error() int {
	// TODO: implement error raising
	return int(luaapi.LUA_ERRRUN)
}

func (L *LuaState) ErrorMessage() int {
	// TODO: implement error message
	return int(luaapi.LUA_ERRRUN)
}

func (L *LuaState) Where(level int) {
	// TODO: implement where
}

// =============================================================================
// GC Control
// =============================================================================

func (L *LuaState) GC(what int, args ...int) int {
	// TODO: implement GC control
	return 0
}

// =============================================================================
// Miscellaneous
// =============================================================================

func (L *LuaState) Next(idx int) bool {
	// TODO: implement next
	return false
}

func (L *LuaState) Concat(n int) {
	// TODO: implement concat
}

func (L *LuaState) Len(idx int) {
	// TODO: implement len
}

func (L *LuaState) Compare(idx1, idx2 int, op int) bool {
	// TODO: implement compare
	return false
}

func (L *LuaState) RawLen(idx int) uint {
	// TODO: implement raw len
	return 0
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
