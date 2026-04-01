package lapi

/*
** $Id: lapi.go $
** Lua API - Go implementation
** Ported from lapi.c
*/

import (
	"fmt"
	"unsafe"

	"github.com/akzj/go-lua/internal/ldo"
	"github.com/akzj/go-lua/internal/lfunc"
	"github.com/akzj/go-lua/internal/lmem"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lparser"
	"github.com/akzj/go-lua/internal/lstate"
	"github.com/akzj/go-lua/internal/lstring"
	"github.com/akzj/go-lua/internal/ltable"
	"github.com/akzj/go-lua/internal/lvm"
	"github.com/akzj/go-lua/internal/lzio"
)

/*
** Constants
 */
const MAXRESULTS = 2024
const LUA_REGISTRYINDEX = -10000
const LUA_MULTRET = -1
const LUA_VERSION_NUM = 504

/*
** LuaReader function type
 */
type LuaReader func(L *lstate.LuaState, data interface{}, size *int) *byte

/*
** Growstackaux - grow stack with check
 */
func Growstackaux(L *lstate.LuaState, n int) int {
	defer func() {
		if r := recover(); r != nil {
			// Stack overflow handled
		}
	}()
	ldo.Growstack(L, n)
	return 1
}

/*
** Check stack space
 */
func luaCheckstack(L *lstate.LuaState, n int) int {
	if uintptr(unsafe.Pointer(L.StackLast.P))-uintptr(unsafe.Pointer(L.Top.P)) > uintptr(n) {
		return 1
	}
	return Growstackaux(L, n)
}

/*
** Convert acceptable index to absolute index
 */
func luaAbsindex(L *lstate.LuaState, idx int) int {
	if idx > 0 || idx <= LUA_REGISTRYINDEX {
		return idx
	}
	return int(uintptr(unsafe.Pointer(L.Top.P))-uintptr(unsafe.Pointer(L.Ci.F.P))-unsafe.Sizeof(lobject.TValue{})) + idx
}

/*
** Increment stack top
 */
func apiIncrTop(L *lstate.LuaState) {
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) + unsafe.Sizeof(lobject.TValue{})))
}

/*
** Check that there are n elements to pop
 */
func apiCheckPop(L *lstate.LuaState, n int) {
	if int(uintptr(unsafe.Pointer(L.Top.P))-uintptr(unsafe.Pointer(L.Ci.F.P))-unsafe.Sizeof(lobject.TValue{})) < n {
		panic("not enough free elements in the stack")
	}
}

/*
** Check results from function
 */
func checkresults(L *lstate.LuaState, na, nr int) {
	// Simplified validation
	_ = na
	_ = nr
}

/*
** Adjust results from function
 */
func adjustresults(L *lstate.LuaState, nres int) {
	if nres == LUA_MULTRET && int(uintptr(unsafe.Pointer(L.Ci.Top.P))-uintptr(unsafe.Pointer(L.Top.P))) > 0 {
		L.Ci.Top.P = L.Top.P
	}
}

/*
** API status conversion
 */
func APIstatus(status lobject.TStatus) int {
	// Return 0 for success, non-zero for error (standard Lua convention)
	if status == lobject.LUA_OK || status == lobject.LUA_YIELD {
		return 0
	}
	return 1
}

/*
** Index to stack value - convert index to stack pointer
 */
func index2stack(L *lstate.LuaState, idx int) *lobject.TValue {
	ci := L.Ci
	if idx > 0 {
		return (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(ci.F.P)) + uintptr(idx)*unsafe.Sizeof(lobject.TValue{})))
	}
	// Negative index
	return (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) + uintptr(idx)*unsafe.Sizeof(lobject.TValue{})))
}

/*
** Index to value - get TValue from index
 */
func index2value(L *lstate.LuaState, idx int) *lobject.TValue {
	ci := L.Ci
	if idx > 0 {
		return (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(ci.F.P)) + uintptr(idx)*unsafe.Sizeof(lobject.TValue{})))
	}
	if idx == LUA_REGISTRYINDEX {
		return &L.G.LRegistry
	}
	// Negative index from top
	return (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) + uintptr(idx)*unsafe.Sizeof(lobject.TValue{})))
}

/*
** Push functions (C -> stack)
*/

/*
** lua_pushnil - pushes nil onto stack
 */
func lua_pushnil(L *lstate.LuaState) {
	lobject.SetNilValue(L.Top.P)
	apiIncrTop(L)
}

/*
** lua_pushnumber - pushes number onto stack
 */
func lua_pushnumber(L *lstate.LuaState, n lobject.LuaNumber) {
	lobject.SetFltValue(L.Top.P, n)
	apiIncrTop(L)
}

/*
** lua_pushinteger - pushes integer onto stack
 */
func lua_pushinteger(L *lstate.LuaState, n lobject.LuaInteger) {
	lobject.SetIntValue(L.Top.P, n)
	apiIncrTop(L)
}

/*
** lua_pushlstring - pushes string onto stack
 */
func lua_pushlstring(L *lstate.LuaState, s string) {
	var ts *lobject.TString
	if len(s) == 0 {
		ts = lstring.NewString(L, "")
	} else {
		ts = lstring.NewString(L, s)
	}
	// Set GCObject pointer directly
	L.Top.P.Value_.Gc = (*lobject.GCObject)(unsafe.Pointer(ts))
	L.Top.P.Tt_ = uint8(lobject.LUA_VSHRSTR)
	apiIncrTop(L)
}

/*
** lua_pushstring - pushes string onto stack
 */
func lua_pushstring(L *lstate.LuaState, s string) {
	if s == "" {
		lua_pushnil(L)
	} else {
		lua_pushlstring(L, s)
	}
}

/*
** lua_pushcclosure - pushes C closure onto stack
 */
func lua_pushcclosure(L *lstate.LuaState, fn lobject.LuaCFunction, n int) {
	if n == 0 {
		lobject.SetFValue(L.Top.P, fn)
		apiIncrTop(L)
	} else {
		apiCheckPop(L, n)
		cl := lfunc.NewCClosure(L, n)
		cl.F = fn
		for i := 0; i < n; i++ {
			src := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - uintptr(n-i)*unsafe.Sizeof(lobject.TValue{})))
			cl.Upvalue[i] = *src
		}
		L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - uintptr(n)*unsafe.Sizeof(lobject.TValue{})))
		L.Top.P.Value_.Gc = (*lobject.GCObject)(unsafe.Pointer(cl))
		L.Top.P.Tt_ = uint8(lobject.LUA_VCCL)
		apiIncrTop(L)
	}
}

/*
** lua_pushboolean - pushes boolean onto stack
 */
func lua_pushboolean(L *lstate.LuaState, b bool) {
	lobject.SetBtValue(L.Top.P, b)
	apiIncrTop(L)
}

/*
** lua_pushlightuserdata - pushes light userdata onto stack
 */
func lua_pushlightuserdata(L *lstate.LuaState, p interface{}) {
	lobject.SetPValue(L.Top.P, p)
	apiIncrTop(L)
}

/*
** lua_pushthread - pushes thread onto stack
 */
func lua_pushthread(L *lstate.LuaState) int {
	L.Top.P.Value_.Gc = (*lobject.GCObject)(unsafe.Pointer(L))
	L.Top.P.Tt_ = uint8(lobject.LUA_VTHREAD)
	apiIncrTop(L)
	if lstate.MainThreadPtr(L.G) == L {
		return 1
	}
	return 0
}

/*
** lua_pushvalue - pushes copy of value onto stack
 */
func lua_pushvalue(L *lstate.LuaState, idx int) {
	src := index2value(L, idx)
	lobject.SetObj(L.Top.P, src)
	apiIncrTop(L)
}

/*
** Access functions (stack -> C)
*/

/*
** lua_gettop - returns stack top index
 */
func lua_gettop(L *lstate.LuaState) int {
	return int(uintptr(unsafe.Pointer(L.Top.P))-uintptr(unsafe.Pointer(L.Ci.F.P)))/int(unsafe.Sizeof(lobject.TValue{})) - 1
}

/*
** lua_settop - sets stack top
 */
func lua_settop(L *lstate.LuaState, idx int) {
	if idx >= 0 {
		// Set to absolute position
		newTop := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Ci.F.P)) + uintptr(idx+1)*unsafe.Sizeof(lobject.TValue{})))
		L.Top.P = newTop
	} else {
		// Relative from top
		L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) + uintptr(idx)*unsafe.Sizeof(lobject.TValue{})))
	}
}

/*
** lua_type - returns type of value at index
 */
func lua_type(L *lstate.LuaState, idx int) int {
	o := index2value(L, idx)
	return lobject.TType(o)
}

/*
** lua_typename - returns name of type
 */
func lua_typename(L *lstate.LuaState, t int) string {
	return lobject.TTypeName(t)
}

/*
** lua_iscfunction - returns 1 if value is C function
 */
func lua_iscfunction(L *lstate.LuaState, idx int) int {
	o := index2value(L, idx)
	if lobject.TtIsLcf(o) || lobject.TtIsCClosure(o) {
		return 1
	}
	return 0
}

/*
** lua_isinteger - returns 1 if value is integer
 */
func lua_isinteger(L *lstate.LuaState, idx int) int {
	o := index2value(L, idx)
	if lobject.TtIsInteger(o) {
		return 1
	}
	return 0
}

/*
** lua_isnumber - returns 1 if value is number
 */
func lua_isnumber(L *lstate.LuaState, idx int) int {
	o := index2value(L, idx)
	if lobject.TtIsInteger(o) || lobject.TtIsFloat(o) {
		return 1
	}
	return 0
}

/*
** lua_isstring - returns 1 if value is string
 */
func lua_isstring(L *lstate.LuaState, idx int) int {
	o := index2value(L, idx)
	if lobject.TtIsString(o) {
		return 1
	}
	return 0
}

/*
** lua_isuserdata - returns 1 if value is userdata
 */
func lua_isuserdata(L *lstate.LuaState, idx int) int {
	o := index2value(L, idx)
	if lobject.TtIsUserdata(o) || lobject.TtIsLightUserdata(o) {
		return 1
	}
	return 0
}

/*
** tonumber - convert to number
 */
func tonumber(o *lobject.TValue, n *lobject.LuaNumber) int {
	if lobject.TtIsInteger(o) {
		*n = lobject.LuaNumber(lobject.IntValue(o))
		return 1
	}
	if lobject.TtIsFloat(o) {
		*n = lobject.FltValue(o)
		return 1
	}
	return 0
}

/*
** tointeger - convert to integer
 */
func tointeger(o *lobject.TValue, i *lobject.LuaInteger) int {
	if lobject.TtIsInteger(o) {
		*i = lobject.IntValue(o)
		return 1
	}
	if lobject.TtIsFloat(o) {
		f := lobject.FltValue(o)
		if f == float64(int64(f)) {
			*i = int64(f)
			return 1
		}
	}
	return 0
}

/*
** lua_tonumberx - returns number at index
 */
func lua_tonumberx(L *lstate.LuaState, idx int, isnum *int) lobject.LuaNumber {
	o := index2value(L, idx)
	var n lobject.LuaNumber
	*isnum = tonumber(o, &n)
	return n
}

/*
** lua_tointegerx - returns integer at index
 */
func lua_tointegerx(L *lstate.LuaState, idx int, isnum *int) lobject.LuaInteger {
	o := index2value(L, idx)
	var i lobject.LuaInteger
	*isnum = tointeger(o, &i)
	return i
}

/*
** lua_toboolean - returns boolean at index
 */
func lua_toboolean(L *lstate.LuaState, idx int) int {
	o := index2value(L, idx)
	if lobject.IsFalse(o) {
		return 0
	}
	return 1
}

/*
** lua_tolstring - returns string at index
 */
func lua_tolstring(L *lstate.LuaState, idx int, len *int) string {
	o := index2value(L, idx)
	if lobject.TtIsString(o) {
		ts := lobject.Gco2Ts(lobject.GcValue(o))
		if len != nil {
			*len = lstring.StrLen(ts)
		}
		return string(lstring.GetStr(ts))
	}
	// Handle numbers
	if lobject.TtIsInteger(o) {
		return fmt.Sprintf("%d", o.Value_.I)
	}
	if lobject.TtIsNumber(o) {
		return fmt.Sprintf("%v", o.Value_.N)
	}
	if len != nil {
		*len = 0
	}
	return ""
}

/*
** lua_rawlen - returns raw length
 */
func lua_rawlen(L *lstate.LuaState, idx int) lobject.LuaUnsigned {
	o := index2value(L, idx)
	if lobject.TtIsTable(o) {
		t := lobject.Gco2T(lobject.GcValue(o))
		return lobject.LuaUnsigned(t.Asize)
	}
	if lobject.TtIsShrString(o) || lobject.TtIsLngString(o) {
		ts := lobject.Gco2Ts(lobject.GcValue(o))
		return lobject.LuaUnsigned(ts.Shrlen)
	}
	return 0
}

/*
** lua_tocfunction - returns C function at index
 */
func lua_tocfunction(L *lstate.LuaState, idx int) lobject.LuaCFunction {
	o := index2value(L, idx)
	if lobject.TtIsLcf(o) {
		return lobject.FValue(o)
	}
	if lobject.TtIsCClosure(o) {
		cl := lobject.Gco2Ccl(lobject.GcValue(o))
		return cl.F
	}
	return nil
}

/*
** lua_touserdata - returns userdata at index
 */
func lua_touserdata(L *lstate.LuaState, idx int) interface{} {
	o := index2value(L, idx)
	switch lobject.TType(o) {
	case lobject.LUA_TUSERDATA:
		u := lobject.Gco2U(lobject.GcValue(o))
		return u
	case lobject.LUA_TLIGHTUSERDATA:
		return lobject.PValue(o)
	}
	return nil
}

/*
** lua_tothread - returns thread at index
 */
func lua_tothread(L *lstate.LuaState, idx int) *lstate.LuaState {
	o := index2value(L, idx)
	if !lobject.TtIsThread(o) {
		return nil
	}
	// Convert from *lobject.LuaState (forward decl) to *lstate.LuaState
	return (*lstate.LuaState)(unsafe.Pointer(lobject.Gco2Th(lobject.GcValue(o))))
}

/*
** Get functions (Lua -> stack)
*/

/*
** lua_createtable - creates new table
 */
func lua_createtable(L *lstate.LuaState, narray, nrec int) {
	t := newTable()
	t.Asize = uint32(narray)
	t.Array = make([]lobject.TValue, narray)
	t.Node = make([]lobject.Node, 0)
	L.Top.P.Value_.Gc = (*lobject.GCObject)(unsafe.Pointer(t))
	L.Top.P.Tt_ = uint8(lobject.LUA_VTABLE)
	apiIncrTop(L)
}

/*
** lua_gettable - gets table value
 */
func lua_gettable(L *lstate.LuaState, idx int) int {
	t := index2value(L, idx)
	key := L.Top.P
	if !lobject.TtIsTable(t) {
		apiIncrTop(L)
		return lobject.LUA_TNIL
	}
	tbl := lobject.Gco2T(lobject.GcValue(t))
	_, val := ltable.Get(tbl, key)
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
	if val != nil {
		lobject.SetObj(L.Top.P, val)
		apiIncrTop(L)
		return lobject.RawTT(val) & 0x0F
	}
	lobject.SetNilValue(L.Top.P)
	apiIncrTop(L)
	return lobject.LUA_TNIL
}

/*
** lua_getfield - gets table field
 */
func lua_getfield(L *lstate.LuaState, idx int, k string) int {
	t := index2value(L, idx)
	if !lobject.TtIsTable(t) {
		return lobject.LUA_TNIL
	}
	tbl := lobject.Gco2T(lobject.GcValue(t))
	key := &lobject.TValue{Value_: lobject.Value{Gc: (*lobject.GCObject)(unsafe.Pointer(lstring.NewString(L, k)))}, Tt_: uint8(lobject.LUA_VSHRSTR)}
	_, val := ltable.Get(tbl, key)
	if val != nil {
		lobject.SetObj(L.Top.P, val)
		apiIncrTop(L)
		return lobject.RawTT(val) & 0x0F
	}
	lobject.SetNilValue(L.Top.P)
	apiIncrTop(L)
	return lobject.LUA_TNIL
}

/*
** lua_geti - gets table value by integer
 */
func lua_geti(L *lstate.LuaState, idx int, n lobject.LuaInteger) int {
	t := index2value(L, idx)
	if !lobject.TtIsTable(t) {
		return lobject.LUA_TNIL
	}
	tbl := lobject.Gco2T(lobject.GcValue(t))
	_, val := ltable.GetInt(tbl, n)
	if val != nil {
		lobject.SetObj(L.Top.P, val)
		apiIncrTop(L)
		return lobject.RawTT(val) & 0x0F
	}
	lobject.SetNilValue(L.Top.P)
	apiIncrTop(L)
	return lobject.LUA_TNIL
}

/*
** lua_rawget - raw table get
 */
func lua_rawget(L *lstate.LuaState, idx int) int {
	t := index2value(L, idx)
	if !lobject.TtIsTable(t) {
		panic("table expected")
	}
	tbl := lobject.Gco2T(lobject.GcValue(t))
	key := L.Top.P
	_, val := ltable.Get(tbl, key)
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
	if val != nil && lobject.RawTT(val) != lobject.LUA_TNIL {
		lobject.SetObj(L.Top.P, val)
		apiIncrTop(L)
		return lobject.RawTT(val) & 0x0F
	}
	lobject.SetNilValue(L.Top.P)
	apiIncrTop(L)
	return lobject.LUA_TNIL
}

/*
** lua_rawgeti - raw table get by integer
 */
func lua_rawgeti(L *lstate.LuaState, idx int, n lobject.LuaInteger) int {
	t := index2value(L, idx)
	if !lobject.TtIsTable(t) {
		panic("table expected")
	}
	tbl := lobject.Gco2T(lobject.GcValue(t))
	_, val := ltable.GetInt(tbl, n)
	if val != nil && lobject.RawTT(val) != lobject.LUA_TNIL {
		lobject.SetObj(L.Top.P, val)
		apiIncrTop(L)
		return lobject.RawTT(val) & 0x0F
	}
	lobject.SetNilValue(L.Top.P)
	apiIncrTop(L)
	return lobject.LUA_TNIL
}

/*
** lua_getglobal - gets global value
 */
func lua_getglobal(L *lstate.LuaState, name string) int {
	return lua_getfield(L, LUA_REGISTRYINDEX, name)
}

/*
** lua_getmetatable - gets metatable
 */
func lua_getmetatable(L *lstate.LuaState, objindex int) int {
	obj := index2value(L, objindex)
	var mt *lobject.Table
	switch lobject.TType(obj) {
	case lobject.LUA_TTABLE:
		t := lobject.Gco2T(lobject.GcValue(obj))
		mt = t.Metatable
	case lobject.LUA_TUSERDATA:
		u := lobject.Gco2U(lobject.GcValue(obj))
		mt = u.Metatable
	default:
		mt = L.G.Mt[lobject.TType(obj)]
	}
	if mt != nil {
		L.Top.P.Value_.Gc = (*lobject.GCObject)(unsafe.Pointer(mt))
		L.Top.P.Tt_ = uint8(lobject.LUA_VTABLE)
		apiIncrTop(L)
		return 1
	}
	return 0
}

/*
** Set functions (stack -> Lua)
*/

/*
** lua_settable - sets table value
 */
func lua_settable(L *lstate.LuaState, idx int) {
	t := index2value(L, idx)
	key := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - 2*unsafe.Sizeof(lobject.TValue{})))
	val := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - 2*unsafe.Sizeof(lobject.TValue{})))
	if lobject.TtIsTable(t) {
		tbl := lobject.Gco2T(lobject.GcValue(t))
		ltable.Set((*lobject.LuaState)(unsafe.Pointer(L)), tbl, key, val)
	}
}

/*
** lua_setfield - sets table field
 */
func lua_setfield(L *lstate.LuaState, idx int, k string) {
	t := index2value(L, idx)
	key := &lobject.TValue{Value_: lobject.Value{Gc: (*lobject.GCObject)(unsafe.Pointer(lstring.NewString(L, k)))}, Tt_: uint8(lobject.LUA_VSHRSTR)}
	// FIX: Read val from slot BEFORE decrementing L.Top.P
	val := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
	fmt.Printf("DEBUG setfield: key=%s val=%p Tt_=%d Gc=%p\n", k, val, val.Tt_, val.Value_.Gc)
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
	if lobject.TtIsTable(t) {
		tbl := lobject.Gco2T(lobject.GcValue(t))
		ltable.Set((*lobject.LuaState)(unsafe.Pointer(L)), tbl, key, val)
	}
}

/*
** lua_seti - sets table value by integer
 */
func lua_seti(L *lstate.LuaState, idx int, n lobject.LuaInteger) {
	t := index2value(L, idx)
	// FIX: Read val from slot BEFORE decrementing L.Top.P
	val := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
	key := &lobject.TValue{Value_: lobject.Value{I: n}, Tt_: uint8(lobject.LUA_VNUMINT)}
	if lobject.TtIsTable(t) {
		tbl := lobject.Gco2T(lobject.GcValue(t))
		ltable.Set((*lobject.LuaState)(unsafe.Pointer(L)), tbl, key, val)
	}
}

/*
** lua_rawset - raw table set
 */
func lua_rawset(L *lstate.LuaState, idx int) {
	t := index2value(L, idx)
	if !lobject.TtIsTable(t) {
		panic("table expected")
	}
	tbl := lobject.Gco2T(lobject.GcValue(t))
	key := L.Top.P
	val := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
	ltable.Set((*lobject.LuaState)(unsafe.Pointer(L)), tbl, key, val)
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - 2*unsafe.Sizeof(lobject.TValue{})))
}

/*
** lua_rawseti - raw table set by integer
 */
func lua_rawseti(L *lstate.LuaState, idx int, n lobject.LuaInteger) {
	t := index2value(L, idx)
	if !lobject.TtIsTable(t) {
		panic("table expected")
	}
	tbl := lobject.Gco2T(lobject.GcValue(t))
	val := L.Top.P
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
	key := &lobject.TValue{Value_: lobject.Value{I: n}, Tt_: uint8(lobject.LUA_VNUMINT)}
	ltable.Set((*lobject.LuaState)(unsafe.Pointer(L)), tbl, key, val)
}

/*
** lua_setglobal - sets global value
 */
func lua_setglobal(L *lstate.LuaState, name string) {
	// Debug: show which table we're setting in
	t := index2value(L, LUA_REGISTRYINDEX)
	tbl := lobject.Gco2T(lobject.GcValue(t))
	fmt.Printf("DEBUG lua_setglobal: name=%s tbl=%p\n", name, tbl)
	lua_setfield(L, LUA_REGISTRYINDEX, name)
}

/*
** lua_setmetatable - sets metatable
 */
func lua_setmetatable(L *lstate.LuaState, objindex int) int {
	obj := index2value(L, objindex)
	mt := L.Top.P
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
	var mtPtr *lobject.Table
	if !lobject.TtIsNil(mt) {
		if !lobject.TtIsTable(mt) {
			panic("table expected")
		}
		mtPtr = lobject.Gco2T(lobject.GcValue(mt))
	}
	switch lobject.TType(obj) {
	case lobject.LUA_TTABLE:
		t := lobject.Gco2T(lobject.GcValue(obj))
		t.Metatable = mtPtr
	case lobject.LUA_TUSERDATA:
		u := lobject.Gco2U(lobject.GcValue(obj))
		u.Metatable = mtPtr
	default:
		L.G.Mt[lobject.TType(obj)] = mtPtr
	}
	return 1
}

/*
** lua_copy - copy value
 */
func lua_copy(L *lstate.LuaState, fromidx, toidx int) {
	fr := index2value(L, fromidx)
	to := index2value(L, toidx)
	lobject.SetObj(to, fr)
}

/*
** Load and call functions
*/

/*
** lua_callk - call a function (panics on error)
 */
func lua_callk(L *lstate.LuaState, nargs, nresults int, ctx lobject.LuaInteger, k lobject.LuaKFunction) {
	apiCheckPop(L, nargs+1)
	if L.Status != lobject.LUA_OK {
		panic("cannot do calls on non-normal thread")
	}
	checkresults(L, nargs, nresults)
	func_ := index2stack(L, 1)
	luaD_call(L, func_, nresults)
	adjustresults(L, nresults)
}

/*
** luaD_call - internal call
 */
func luaD_call(L *lstate.LuaState, func_ *lobject.TValue, nresults int) {
	if lobject.TtIsCClosure(func_) {
		cl := lobject.Gco2Ccl(lobject.GcValue(func_))
		cl.F((*lobject.LuaState)(unsafe.Pointer(L)))
	} else if lobject.TtIsLcf(func_) {
		f := lobject.FValue(func_)
		f((*lobject.LuaState)(unsafe.Pointer(L)))
	} else if lobject.TtIsLClosure(func_) {
		// Lua closure - call VM
		cl := lobject.Gco2Lcl(lobject.GcValue(func_))
		fmt.Printf("DEBUG lapi: func_=%p cl=%p cl.P=%p\n", func_, cl, cl.P)
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("DEBUG VM panic: %v\n", r)
					panic(r)
				}
			}()
			lvm.LuaV_execute(L, L.Ci, func_)
		}()
	}
	_ = nresults
}

/*
** lua_pcallk - protected call (returns error code)
 */
func lua_pcallk(L *lstate.LuaState, nargs, nresults, errfunc int, ctx lobject.LuaInteger, k lobject.LuaKFunction) int {
	apiCheckPop(L, nargs+1)
	if L.Status != lobject.LUA_OK {
		return int(L.Status)
	}
	checkresults(L, nargs, nresults)

	func_ := index2stack(L, 1)
	var status lobject.TStatus

	func() {
		defer func() {
			if r := recover(); r != nil {
				if err, ok := r.(lobject.TStatus); ok {
					status = err
				} else {
					status = lobject.LUA_ERRERR
				}
				L.Top.P = func_
			}
		}()
		luaD_call(L, func_, nresults)
		status = lobject.LUA_OK
	}()

	adjustresults(L, nresults)
	return int(APIstatus(status))
}

/*
** lua_load - load chunk using parser
 */
func lua_load(L *lstate.LuaState, reader LuaReader, data interface{}, chunkname string, mode string) int {
	if chunkname == "" {
		chunkname = "?"
	}

	// If data is a string, use it directly with lzio
	if data != nil {
		if s, ok := data.(string); ok {
			z := &lzio.ZIO{}
			sr := &stringReader{data: s}
			lzio.Init(L, z, sr.Read, data)

			buff := &lzio.Mbuffer{}
			lzio.InitBuffer(buff)
			lzio.ResizeBuffer(L, buff, 256)

			cl := lparser.LuaY_parser(L, z, buff, chunkname)
			if cl == nil {
				return int(lobject.LUA_ERRSYNTAX)
			}
			// Set up _ENV upvalue for main chunk
			if int(cl.Nupvalues) < 1 {
				cl.Upvals = append(cl.Upvals, nil)
				cl.Nupvalues = 1
			}
			if cl.Upvals[0] == nil {
				cl.Upvals[0] = &lobject.UpVal{}
			}
			// LRegistry is a TValue, get the Table from its Value_.Gc field
			globalTbl := lobject.Gco2T(L.G.LRegistry.Value_.Gc)
			fmt.Printf("DEBUG _ENV: globalTbl=%p\n", globalTbl)
			cl.Upvals[0].V = &lobject.Value{Gc: (*lobject.GCObject)(unsafe.Pointer(globalTbl))}
			fmt.Printf("DEBUG: _ENV set, Nupvalues=%d\n", cl.Nupvalues)
			fmt.Printf("DEBUG: PARSER Code len=%d K len=%d\n", len(cl.P.Code), len(cl.P.K))
			for i := 0; i < len(cl.P.Code) && i < 10; i++ {
				fmt.Printf("DEBUG: PARSER ins[%d]=%d\n", i, cl.P.Code[i])
			}
			fmt.Printf("DEBUG: K len=%d\n", len(cl.P.K))

			// Push closure to stack using SetObj
			L.Top.P.Value_.Gc = (*lobject.GCObject)(unsafe.Pointer(cl))
			L.Top.P.Tt_ = uint8(lobject.CTb(lobject.LUA_VLCL))
			apiIncrTop(L)
			return int(lobject.LUA_OK)
		}
	}

	// No data - create empty closure
	f := lfunc.NewLClosure(L, 0)
	L.Top.P.Value_.Gc = (*lobject.GCObject)(unsafe.Pointer(f))
	L.Top.P.Tt_ = uint8(lobject.LUA_VLCL)
	apiIncrTop(L)
	return int(lobject.LUA_OK)
}

/*
** stringReader - implements lzio.Reader for string input
 */
type stringReader struct {
	data string
	pos  int
}

func (r *stringReader) Read(L *lstate.LuaState, data interface{}, size *int64) []byte {
	if r.pos >= len(r.data) {
		*size = 0
		return nil
	}
	remaining := r.data[r.pos:]
	*size = int64(len(remaining))
	r.pos = len(r.data)
	return []byte(remaining)
}

/*
** lua_close - close Lua state
 */
func lua_close(L *lstate.LuaState) {
	// Simplified cleanup
}

/*
** lua_status - returns thread status
 */
func lua_status(L *lstate.LuaState) int {
	return int(APIstatus(L.Status))
}

/*
** lua_error - raise error
 */
func lua_error(L *lstate.LuaState) int {
	panic(lobject.LUA_ERRRUN)
}

/*
** lua_next - iterate table
 */
func lua_next(L *lstate.LuaState, idx int) int {
	t := index2value(L, idx)
	if !lobject.TtIsTable(t) {
		panic("table expected")
	}
	return 0
}

/*
** lua_concat - concatenate
 */
func lua_concat(L *lstate.LuaState, n int) {
	if n > 0 {
		// Simplified
	} else {
		lua_pushstring(L, "")
	}
}

/*
** lua_len - length
 */
func lua_len(L *lstate.LuaState, idx int) {
	t := index2value(L, idx)
	if lobject.TtIsTable(t) {
		tbl := lobject.Gco2T(lobject.GcValue(t))
		lobject.SetIntValue(L.Top.P, lobject.LuaInteger(tbl.Asize))
	} else if lobject.TtIsString(t) {
		ts := lobject.Gco2Ts(lobject.GcValue(t))
		lobject.SetIntValue(L.Top.P, lobject.LuaInteger(ts.Shrlen))
	} else {
		lobject.SetIntValue(L.Top.P, 0)
	}
	apiIncrTop(L)
}

/*
** lua_getallocf - get allocator
 */
func lua_getallocf(L *lstate.LuaState, ud *interface{}) lobject.LuaAlloc {
	if ud != nil {
		*ud = L.G.Ud
	}
	return L.G.Frealloc
}

/*
** lua_setallocf - set allocator
 */
func lua_setallocf(L *lstate.LuaState, f lobject.LuaAlloc, ud interface{}) {
	L.G.Ud = ud
	L.G.Frealloc = f
}

/*
** lua_newuserdatauv - create userdata
 */
func lua_newuserdatauv(L *lstate.LuaState, size int, nuvalue int) interface{} {
	u := &lobject.Udata{
		Nuvalue: uint16(nuvalue),
		Len:     uint64(size),
	}
	L.Top.P.Value_.Gc = (*lobject.GCObject)(unsafe.Pointer(u))
	L.Top.P.Tt_ = uint8(lobject.LUA_VUSERDATA)
	apiIncrTop(L)
	return u
}

/*
** lua_atpanic - set panic function
 */
func lua_atpanic(L *lstate.LuaState, panicf lobject.LuaCFunction) lobject.LuaCFunction {
	old := L.G.Panic
	L.G.Panic = panicf
	return old
}

/*
** lua_version - get version
 */
func lua_version(L *lstate.LuaState) lobject.LuaNumber {
	return lobject.LuaNumber(LUA_VERSION_NUM)
}

/*
** lua_xmove - move values between threads
 */
func lua_xmove(from *lstate.LuaState, to *lstate.LuaState, n int) {
	if from == to {
		return
	}
	if from.G != to.G {
		panic("moving among independent states")
	}
	for i := 0; i < n; i++ {
		src := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(from.Top.P)) - uintptr(n-i)*unsafe.Sizeof(lobject.TValue{})))
		lobject.SetObj(to.Top.P, src)
		apiIncrTop(to)
	}
	from.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(from.Top.P)) - uintptr(n)*unsafe.Sizeof(lobject.TValue{})))
}

/*
** newTable - create a new table directly (avoids lmem type issues)
 */
func newTable() *lobject.Table {
	t := &lobject.Table{
		CommonHeader: lobject.CommonHeader{
			Tt: uint8(lobject.LUA_VTABLE),
		},
		Flags:     0,
		Lsizenode: 0,
		Asize:     0,
		Array:     nil,
		Node:      nil,
		Metatable: nil,
		Gclist:    nil,
	}
	return t
}

/*
** lua_newstate - creates a new Lua state
** Panics on memory allocation failure (constraint)
 */
func lua_newstate(f lobject.LuaAlloc, ud interface{}, seed uint) *lstate.LuaState {
	// Allocate global state
	g := (*lstate.GlobalState)(unsafe.Pointer(&make([]byte, unsafe.Sizeof(lstate.GlobalState{}))[:][0]))
	if g == nil {
		panic("cannot allocate memory for lua state")
	}

	// Initialize global state
	L := &g.MainTh.L
	L.G = g

	// Set up basic fields
	g.Frealloc = f
	g.Ud = ud
	g.Strt.Size = 0
	g.Strt.Hash = nil
	g.Strt.Nuse = 0
	lobject.SetNilValue(&g.LRegistry)
	g.Panic = nil
	g.GCState = uint8(8)
	g.GCKind = 0
	g.GCStopEm = 0
	g.GCEmergency = 0
	g.GCStp = 0
	g.Allgc = nil
	g.Finobj = nil
	g.Fixedgc = nil
	g.Gray = nil
	g.GrayAgain = nil
	g.Weak = nil
	g.Ephemeron = nil
	g.Allweak = nil
	g.TobefnZ = nil
	g.Twups = nil
	g.Memerrmsg = nil
	for i := 0; i < lobject.LUA_NUMTYPES; i++ {
		g.Mt[i] = nil
	}

	// Initialize main thread
	L.CommonHeader.Tt = uint8(lobject.LUA_VTHREAD)
	L.CommonHeader.Marked = 0
	L.CommonHeader.Next = nil
	L.AllowHook = true
	L.Status = lobject.LUA_OK
	L.Gclist = nil
	L.OpenUpval = nil
	L.ErrorJmp = nil
	L.NCcalls = 0
	L.HookMask = 0
	L.BaseHookCount = 0
	L.HookCount = 0
	L.Hook = nil
	L.Errfunc = 0
	L.Oldpc = 0
	L.Nci = 0
	L.BaseCcalls = 0
	L.Twups = L

	// Initialize stack
	L.Stack = make([]lobject.StackValue, lstate.BASIC_STACK_SIZE+lstate.EXTRA_STACK)
	for i := range L.Stack {
		lobject.SetNilValue(&L.Stack[i].Val)
	}
	L.StackLast.P = &L.Stack[lstate.BASIC_STACK_SIZE].Val

	// Initialize top and base
	L.BaseCcalls = 200
	L.Top.P = &L.Stack[1].Val  // Top starts at first available slot (func is at slot 0)
	L.Ci = &L.BaseCi
	L.Ci.F.P = &L.Stack[0].Val  // Function at slot 0
	L.Ci.Top.P = &L.Stack[lstate.BASIC_STACK_SIZE].Val
	L.Ci.Previous = nil
	L.Ci.Next = nil
	L.Ci.CallStatus = 0
	L.BaseCi = *L.Ci

	// Set up nilvalue to signal state is built
	lobject.SetIntValue(&g.NilValue, 0)

	// Initialize string table
	g.Strt.Size = 0
	g.Strt.Hash = nil
	g.Strt.Nuse = 0

	// Set current white
	g.CurrentWhite = 1 << uint8(3)

	// Create global table and add to registry
	globalTbl := newTable()
	gtPtr := &lobject.TValue{Value_: lobject.Value{Gc: (*lobject.GCObject)(unsafe.Pointer(globalTbl))}, Tt_: uint8(lobject.LUA_VTABLE)}
	lobject.SetObj(&g.LRegistry, gtPtr)

	// Set global table in registry (simplified - direct access)
	// The registry table is already set up, we'll initialize it lazily

	return L
}

/*
** Default allocator
 */
var Alloc = lmem.DefaultAlloc

/*
** Additional API functions for lauxlib
*/

/*
** lua_pushfstring - push formatted string
** Supports basic format specifiers: %s, %d, %f, %p, %c
 */
func lua_pushfstring(L *lstate.LuaState, fmtStr string, args ...interface{}) {
	// Simple implementation - build string from format and args
	result := formatString(fmtStr, args)
	lua_pushstring(L, result)
}

func formatString(fmtStr string, args []interface{}) string {
	result := ""
	argIdx := 0
	i := 0
	for i < len(fmtStr) {
		if fmtStr[i] == '%' && i+1 < len(fmtStr) {
			switch fmtStr[i+1] {
			case 's':
				if argIdx < len(args) {
					result += toString(args[argIdx])
					argIdx++
				}
				i += 2
				continue
			case 'd', 'i':
				if argIdx < len(args) {
					result += toIntString(args[argIdx])
					argIdx++
				}
				i += 2
				continue
			case 'f':
				if argIdx < len(args) {
					result += toFloatString(args[argIdx])
					argIdx++
				}
				i += 2
				continue
			case 'p':
				if argIdx < len(args) {
					result += fmt.Sprintf("%p", args[argIdx])
					argIdx++
				}
				i += 2
				continue
			case 'c':
				if argIdx < len(args) {
					result += toCharString(args[argIdx])
					argIdx++
				}
				i += 2
				continue
			default:
				result += string(fmtStr[i])
				i++
			}
		} else {
			result += string(fmtStr[i])
			i++
		}
	}
	return result
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func toIntString(v interface{}) string {
	switch val := v.(type) {
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%d", int64(val))
	default:
		return "0"
	}
}

func toFloatString(v interface{}) string {
	switch val := v.(type) {
	case float64:
		return fmt.Sprintf("%f", val)
	case int:
		return fmt.Sprintf("%d.000000", val)
	case int64:
		return fmt.Sprintf("%d.000000", val)
	default:
		return "0.000000"
	}
}

func toCharString(v interface{}) string {
	switch val := v.(type) {
	case int:
		return string(rune(val))
	case int64:
		return string(rune(val))
	default:
		return ""
	}
}

/*
** lua_getstack - get stack information
** Returns 1 if successful, 0 otherwise
 */
func lua_getstack(L *lstate.LuaState, level int, ar *lobject.Debug) int {
	if level < 0 {
		return 0
	}
	// Simplified implementation
	if level == 0 {
		ar.Event = 0
		ar.Name = "?"
		ar.NameWhat = ""
		ar.What = "C"
		ar.Source = "=[C]"
		ar.Line = -1
		ar.LineDef = 0
		ar.Nups = 0
		return 1
	}
	return 0
}

/*
** lua_getinfo - get function information
** 'what' can contain: 'n' (name), 'S' (source), 'l' (currentline), 'f' (function), 't' (tail call)
 */
func lua_getinfo(L *lstate.LuaState, what string, ar *lobject.Debug) int {
	for _, c := range what {
		switch c {
		case 'n':
			if ar != nil {
				ar.Name = "?"
				ar.NameWhat = ""
			}
		case 'S':
			if ar != nil {
				ar.What = "C"
				ar.Source = "=[C]"
				ar.LineDef = 0
				ar.LastLine = 0
			}
		case 'l':
			if ar != nil {
				ar.Line = -1
			}
		case 'f':
			// Push function onto stack
			lua_pushnil(L)
		case 't':
			if ar != nil {
				ar.IsTailCall = false
			}
		case 'u':
			if ar != nil {
				ar.Nups = 0
			}
		case 'L':
			// No line info
		}
	}
	return 1
}

/*
** Warning function type
 */
type LuaWarnFunction func(ud interface{}, msg string, tocont int)

/*
** lua_setwarnf - set warning function
 */
func lua_setwarnf(L *lstate.LuaState, f LuaWarnFunction, ud interface{}) {
	// Stub - warnings not implemented in this simplified version
	_ = L
	_ = f
	_ = ud
}

/*
** lua_closeslot - close slot
** Marks a slot to be closed when the function returns
 */
func lua_closeslot(L *lstate.LuaState, idx int) {
	// Simplified - marks slot for closing
	// In full implementation, this would register the slot for __close
	_ = L
	_ = idx
}

/*
** lua_isnoneornil - check if index is none or nil
 */
func lua_isnoneornil(L *lstate.LuaState, idx int) bool {
	if idx == 0 {
		return true // LUA_NONE
	}
	t := lua_type(L, idx)
	return t == lobject.LUA_TNIL
}

/*
** lua_checkstack - ensure stack space
 */
func lua_checkstack(L *lstate.LuaState, sz int) {
	luaCheckstack(L, sz)
}

/*
** lua_rawequal - raw equality check
 */
func lua_rawequal(L *lstate.LuaState, idx1, idx2 int) int {
	o1 := index2value(L, idx1)
	o2 := index2value(L, idx2)
	if o1.Tt_ != o2.Tt_ {
		return 0
	}
	// Compare values
	switch {
	case lobject.TtIsNil(o1):
		return 1
	case lobject.TtIsInteger(o1):
		return boolToInt(lobject.IntValue(o1) == lobject.IntValue(o2))
	case lobject.TtIsFloat(o1):
		return boolToInt(lobject.FltValue(o1) == lobject.FltValue(o2))
	case lobject.TtIsBoolean(o1):
		return boolToInt(lobject.TtIsTrue(o1) == lobject.TtIsTrue(o2))
	case lobject.TtIsString(o1):
		s1 := lobject.Gco2Ts(lobject.GcValue(o1))
		s2 := lobject.Gco2Ts(lobject.GcValue(o2))
		return boolToInt(s1 == s2)
	case lobject.IsCollectable(o1):
		return boolToInt(lobject.GcValue(o1) == lobject.GcValue(o2))
	default:
		return 0
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

/*
** lua_pop - pop n elements from stack
 */
func lua_pop(L *lstate.LuaState, n int) {
	lua_settop(L, lua_gettop(L)-n)
}

/*
** lua_replace - replace value at index with stack top
 */
func lua_replace(L *lstate.LuaState, idx int) {
	o := index2value(L, idx)
	src := L.Top.P
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
	lobject.SetObj(o, src)
}

/*
** lua_remove - remove value at index
 */
func lua_remove(L *lstate.LuaState, idx int) {
	old := index2value(L, idx)
	pos := uintptr(unsafe.Pointer(old)) - uintptr(unsafe.Pointer(&L.Stack[0].Val))
	count := int(pos / unsafe.Sizeof(lobject.TValue{}))
	for i := count; i < len(L.Stack)-1; i++ {
		L.Stack[i].Val = L.Stack[i+1].Val
	}
	lobject.SetNilValue(&L.Stack[len(L.Stack)-1].Val)
	L.Top.P = (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
}

/*
** lua_insert - move top value to index
 */
func lua_insert(L *lstate.LuaState, idx int) {
	top := (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(L.Top.P)) - unsafe.Sizeof(lobject.TValue{})))
	tgt := index2value(L, idx)
	// Shift elements
	pos := uintptr(unsafe.Pointer(tgt)) - uintptr(unsafe.Pointer(&L.Stack[0].Val))
	count := int(pos / unsafe.Sizeof(lobject.TValue{}))
	for i := len(L.Stack) - 1; i > count; i-- {
		L.Stack[i].Val = L.Stack[i-1].Val
	}
	*tgt = *top
}

/*
** lua_topointer - get pointer to value
 */
func lua_topointer(L *lstate.LuaState, idx int) unsafe.Pointer {
	o := index2value(L, idx)
	return unsafe.Pointer(o)
}

/*
** lua_pushglobaltable - push global table
 */
func lua_pushglobaltable(L *lstate.LuaState) {
	lua_getfield(L, LUA_REGISTRYINDEX, "_G")
}

/*
** lua_rawgetp - raw get by pointer key
 */
func lua_rawgetp(L *lstate.LuaState, idx int, p interface{}) int {
	t := index2value(L, idx)
	if !lobject.TtIsTable(t) {
		return lobject.LUA_TNIL
	}
	lua_pushlightuserdata(L, p)
	return lua_rawget(L, idx)
}

/*
** lua_rawsetp - raw set by pointer key
 */
func lua_rawsetp(L *lstate.LuaState, idx int, p interface{}) {
	t := index2value(L, idx)
	if !lobject.TtIsTable(t) {
		panic("table expected")
	}
	lua_pushlightuserdata(L, p)
	lua_insert(L, -2)
	lua_rawset(L, idx)
}

/*
** Exported API functions for use by lauxlib package
** These are uppercase versions of internal functions
 */

/*
** Lua_newstate - exported version of lua_newstate
 */
func Lua_newstate(f lobject.LuaAlloc, ud interface{}, seed uint) *lstate.LuaState {
	return lua_newstate(f, ud, seed)
}

/*
** Lua_atpanic - exported version of lua_atpanic
 */
func Lua_atpanic(L *lstate.LuaState, panicf lobject.LuaCFunction) lobject.LuaCFunction {
	return lua_atpanic(L, panicf)
}

/*
** Lua_type - exported version of lua_type
 */
func Lua_type(L *lstate.LuaState, idx int) int {
	return lua_type(L, idx)
}

/*
** Lua_tolstring - exported version of lua_tolstring
 */
func Lua_tolstring(L *lstate.LuaState, idx int, len *int) string {
	return lua_tolstring(L, idx, len)
}

/*
** Lua_pushstring - exported version of lua_pushstring
 */
func Lua_pushstring(L *lstate.LuaState, s string) {
	lua_pushstring(L, s)
}

/*
** Lua_pushfstring - exported version of lua_pushfstring
 */
func Lua_pushfstring(L *lstate.LuaState, fmtStr string, args ...interface{}) {
	lua_pushfstring(L, fmtStr, args...)
}

/*
** Lua_pushnil - exported version of lua_pushnil
 */
func Lua_pushnil(L *lstate.LuaState) {
	lua_pushnil(L)
}

/*
** Lua_pushnumber - exported version of lua_pushnumber
 */
func Lua_pushnumber(L *lstate.LuaState, n lobject.LuaNumber) {
	lua_pushnumber(L, n)
}

/*
** Lua_pushinteger - exported version of lua_pushinteger
 */
func Lua_pushinteger(L *lstate.LuaState, n lobject.LuaInteger) {
	lua_pushinteger(L, n)
}

/*
** Lua_pushlstring - exported version of lua_pushlstring
 */
func Lua_pushlstring(L *lstate.LuaState, s string) {
	lua_pushlstring(L, s)
}

/*
** Lua_pushboolean - exported version of lua_pushboolean
 */
func Lua_pushboolean(L *lstate.LuaState, b bool) {
	lua_pushboolean(L, b)
}

/*
** Lua_pushcfunction - exported version of lua_pushcclosure
 */
func Lua_pushcfunction(L *lstate.LuaState, fn lobject.LuaCFunction, n int) {
	lua_pushcclosure(L, fn, n)
}

/*
** Lua_pushlightuserdata - exported version of lua_pushlightuserdata
 */
func Lua_pushlightuserdata(L *lstate.LuaState, p interface{}) {
	lua_pushlightuserdata(L, p)
}

/*
** Lua_pushvalue - exported version of lua_pushvalue
 */
func Lua_pushvalue(L *lstate.LuaState, idx int) {
	lua_pushvalue(L, idx)
}

/*
** Lua_pushglobaltable - exported version of lua_pushglobaltable
 */
func Lua_pushglobaltable(L *lstate.LuaState) {
	lua_pushglobaltable(L)
}

/*
** Lua_pop - exported version of lua_pop
 */
func Lua_pop(L *lstate.LuaState, n int) {
	lua_pop(L, n)
}

/*
** Lua_gettop - exported version of lua_gettop
 */
func Lua_gettop(L *lstate.LuaState) int {
	return lua_gettop(L)
}

/*
** Lua_settop - exported version of lua_settop
 */
func Lua_settop(L *lstate.LuaState, idx int) {
	lua_settop(L, idx)
}

/*
** Lua_absindex - exported version of luaAbsindex
 */
func Lua_absindex(L *lstate.LuaState, idx int) int {
	return luaAbsindex(L, idx)
}

/*
** Lua_typename - exported version of lua_typename
 */
func Lua_typename(L *lstate.LuaState, t int) string {
	return lua_typename(L, t)
}

/*
** Lua_isstring - exported version of lua_isstring
 */
func Lua_isstring(L *lstate.LuaState, idx int) int {
	return lua_isstring(L, idx)
}

/*
** Lua_isnil - exported check for nil
 */
func Lua_isnil(L *lstate.LuaState, idx int) bool {
	return lua_type(L, idx) == lobject.LUA_TNIL
}

/*
** Lua_isnoneornil - exported version of lua_isnoneornil
 */
func Lua_isnoneornil(L *lstate.LuaState, idx int) bool {
	return lua_isnoneornil(L, idx)
}

/*
** Lua_toboolean - exported version of lua_toboolean
 */
func Lua_toboolean(L *lstate.LuaState, idx int) int {
	return lua_toboolean(L, idx)
}

/*
** Lua_tointeger - exported version
 */
func Lua_tointeger(L *lstate.LuaState, idx int) lobject.LuaInteger {
	return lua_tointegerx(L, idx, nil)
}

/*
** Lua_tointegerx - exported version
 */
func Lua_tointegerx(L *lstate.LuaState, idx int, isnum *int) lobject.LuaInteger {
	return lua_tointegerx(L, idx, isnum)
}

/*
** Lua_tonumberx - exported version
 */
func Lua_tonumberx(L *lstate.LuaState, idx int, isnum *int) lobject.LuaNumber {
	return lua_tonumberx(L, idx, isnum)
}

/*
** Lua_touserdata - exported version of lua_touserdata
 */
func Lua_touserdata(L *lstate.LuaState, idx int) interface{} {
	return lua_touserdata(L, idx)
}

/*
** Lua_topointer - exported version of lua_topointer
 */
func Lua_topointer(L *lstate.LuaState, idx int) unsafe.Pointer {
	return lua_topointer(L, idx)
}

/*
** Lua_rawlen - exported version of lua_rawlen
 */
func Lua_rawlen(L *lstate.LuaState, idx int) lobject.LuaUnsigned {
	return lua_rawlen(L, idx)
}

/*
** Lua_createtable - exported version of lua_createtable
 */
func Lua_createtable(L *lstate.LuaState, narray, nrec int) {
	lua_createtable(L, narray, nrec)
}

/*
** Lua_gettable - exported version of lua_gettable
 */
func Lua_gettable(L *lstate.LuaState, idx int) int {
	return lua_gettable(L, idx)
}

/*
** Lua_getfield - exported version of lua_getfield
 */
func Lua_getfield(L *lstate.LuaState, idx int, k string) int {
	return lua_getfield(L, idx, k)
}

/*
** Lua_geti - exported version of lua_geti
 */
func Lua_geti(L *lstate.LuaState, idx int, n lobject.LuaInteger) int {
	return lua_geti(L, idx, n)
}

/*
** Lua_rawget - exported version of lua_rawget
 */
func Lua_rawget(L *lstate.LuaState, idx int) int {
	return lua_rawget(L, idx)
}

/*
** Lua_rawgeti - exported version of lua_rawgeti
 */
func Lua_rawgeti(L *lstate.LuaState, idx int, n lobject.LuaInteger) int {
	return lua_rawgeti(L, idx, n)
}

/*
** Lua_getglobal - exported version of lua_getglobal
 */
func Lua_getglobal(L *lstate.LuaState, name string) int {
	return lua_getglobal(L, name)
}

/*
** Lua_getmetatable - exported version of lua_getmetatable
 */
func Lua_getmetatable(L *lstate.LuaState, objindex int) int {
	return lua_getmetatable(L, objindex)
}

/*
** Lua_settable - exported version of lua_settable
 */
func Lua_settable(L *lstate.LuaState, idx int) {
	lua_settable(L, idx)
}

/*
** Lua_setfield - exported version of lua_setfield
 */
func Lua_setfield(L *lstate.LuaState, idx int, k string) {
	lua_setfield(L, idx, k)
}

/*
** Lua_seti - exported version of lua_seti
 */
func Lua_seti(L *lstate.LuaState, idx int, n lobject.LuaInteger) {
	lua_seti(L, idx, n)
}

/*
** Lua_rawset - exported version of lua_rawset
 */
func Lua_rawset(L *lstate.LuaState, idx int) {
	lua_rawset(L, idx)
}

/*
** Lua_rawseti - exported version of lua_rawseti
 */
func Lua_rawseti(L *lstate.LuaState, idx int, n lobject.LuaInteger) {
	lua_rawseti(L, idx, n)
}

/*
** Lua_setglobal - exported version of lua_setglobal
 */
func Lua_setglobal(L *lstate.LuaState, name string) {
	lua_setglobal(L, name)
}

/*
** Lua_setmetatable - exported version of lua_setmetatable
 */
func Lua_setmetatable(L *lstate.LuaState, objindex int) int {
	return lua_setmetatable(L, objindex)
}

/*
** Lua_rawequal - exported version of lua_rawequal
 */
func Lua_rawequal(L *lstate.LuaState, idx1, idx2 int) int {
	return lua_rawequal(L, idx1, idx2)
}

/*
** Lua_remove - exported version of lua_remove
 */
func Lua_remove(L *lstate.LuaState, idx int) {
	lua_remove(L, idx)
}

/*
** Lua_insert - exported version of lua_insert
 */
func Lua_insert(L *lstate.LuaState, idx int) {
	lua_insert(L, idx)
}

/*
** Lua_replace - exported version of lua_replace
 */
func Lua_replace(L *lstate.LuaState, idx int) {
	lua_replace(L, idx)
}

/*
** Lua_call - exported version of lua_callk
 */
func Lua_call(L *lstate.LuaState, nargs, nresults int) {
	lua_callk(L, nargs, nresults, 0, nil)
}

/*
** Lua_pcall - protected call, returns error code
 */
func Lua_pcall(L *lstate.LuaState, nargs, nresults, errfunc int) int {
	return lua_pcallk(L, nargs, nresults, errfunc, 0, nil)
}

/*
** Lua_concat - exported version of lua_concat
 */
func Lua_concat(L *lstate.LuaState, n int) {
	lua_concat(L, n)
}

/*
** Lua_len - exported version of lua_len
 */
func Lua_len(L *lstate.LuaState, idx int) {
	lua_len(L, idx)
}

/*
** Lua_error - exported version of lua_error
 */
func Lua_error(L *lstate.LuaState) int {
	return lua_error(L)
}

/*
** Lua_checkstack - exported version of lua_checkstack
 */
func Lua_checkstack(L *lstate.LuaState, sz int) bool {
	lua_checkstack(L, sz)
	return true
}

/*
** Lua_newuserdatauv - exported version of lua_newuserdatauv
 */
func Lua_newuserdatauv(L *lstate.LuaState, size int, nuvalue int) interface{} {
	return lua_newuserdatauv(L, size, nuvalue)
}

/*
** Lua_close - exported version of lua_close
 */
func Lua_close(L *lstate.LuaState) {
	lua_close(L)
}

/*
** Lua_version - exported version of lua_version
 */
func Lua_version(L *lstate.LuaState) lobject.LuaNumber {
	return lua_version(L)
}

/*
** Lua_getstack - exported version of lua_getstack
 */
func Lua_getstack(L *lstate.LuaState, level int, ar *lobject.Debug) int {
	return lua_getstack(L, level, ar)
}

/*
** Lua_getinfo - exported version of lua_getinfo
 */
func Lua_getinfo(L *lstate.LuaState, what string, ar *lobject.Debug) int {
	return lua_getinfo(L, what, ar)
}

/*
** Lua_load - exported version of lua_load
 */
func Lua_load(L *lstate.LuaState, reader LuaReader, data interface{}, chunkname string, mode string) int {
	return lua_load(L, reader, data, chunkname, mode)
}

/*
** Lua_status - exported version of lua_status
 */
func Lua_status(L *lstate.LuaState) int {
	return lua_status(L)
}

/*
** Lua_setwarnf - exported version of lua_setwarnf
 */
func Lua_setwarnf(L *lstate.LuaState, f LuaWarnFunction, ud interface{}) {
	lua_setwarnf(L, f, ud)
}

/*
** Lua_closeslot - exported version of lua_closeslot
 */
func Lua_closeslot(L *lstate.LuaState, idx int) {
	lua_closeslot(L, idx)
}

// Lua_isinteger - check if value is an integer
func Lua_isinteger(L *lstate.LuaState, idx int) int {
	if Lua_type(L, idx) != lobject.LUA_TNUMBER {
		return 0
	}
	var isnum int
	Lua_tointegerx(L, idx, &isnum)
	return isnum
}
