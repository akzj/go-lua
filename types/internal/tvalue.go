// Package internal provides concrete implementations of api interfaces.
package internal

import (
	"unsafe"

	"github.com/akzj/go-lua/types/api"
)

// Value is the concrete Value implementation.
type Value struct {
	Variant api.ValueVariant
	Data_   interface{}
}

func (v *Value) GetGC() *api.GCObject {
	if v.Variant != api.ValueGC {
		panic("Value.GetGC: not a GC object")
	}
	return v.Data_.(*api.GCObject)
}

func (v *Value) GetPointer() unsafe.Pointer {
	if v.Variant != api.ValuePointer {
		panic("Value.GetPointer: not a light userdata")
	}
	return v.Data_.(unsafe.Pointer)
}

func (v *Value) GetCFunction() unsafe.Pointer {
	if v.Variant != api.ValueCFunction {
		panic("Value.GetCFunction: not a C function")
	}
	return v.Data_.(unsafe.Pointer)
}

func (v *Value) GetInteger() api.LuaInteger {
	if v.Variant != api.ValueInteger {
		panic("Value.GetInteger: not an integer")
	}
	return v.Data_.(api.LuaInteger)
}

func (v *Value) GetFloat() api.LuaNumber {
	if v.Variant != api.ValueFloat {
		panic("Value.GetFloat: not a float")
	}
	return v.Data_.(api.LuaNumber)
}

// TValue is the concrete TValue implementation.
type TValue struct {
	Value Value
	Tt    uint8
}

func (t *TValue) IsNil() bool              { return api.Novariant(int(t.Tt)) == api.LUA_TNIL }
func (t *TValue) IsBoolean() bool           { return api.Novariant(int(t.Tt)) == api.LUA_TBOOLEAN }
func (t *TValue) IsNumber() bool            { return api.Novariant(int(t.Tt)) == api.LUA_TNUMBER }
func (t *TValue) IsInteger() bool           { return int(t.Tt) == api.LUA_VNUMINT }
func (t *TValue) IsFloat() bool            { return int(t.Tt) == api.LUA_VNUMFLT }
func (t *TValue) IsString() bool            { return api.Novariant(int(t.Tt)) == api.LUA_TSTRING }
func (t *TValue) IsTable() bool            { return int(t.Tt) == api.Ctb(int(api.LUA_VTABLE)) }
func (t *TValue) IsFunction() bool         { return api.Novariant(int(t.Tt)) == api.LUA_TFUNCTION }
func (t *TValue) IsThread() bool           { return int(t.Tt) == api.Ctb(int(api.LUA_VTHREAD)) }
func (t *TValue) IsLightUserData() bool    { return int(t.Tt) == api.LUA_VLIGHTUSERDATA }
func (t *TValue) IsUserData() bool         { return int(t.Tt) == api.Ctb(int(api.LUA_VUSERDATA)) }
func (t *TValue) IsCollectable() bool      { return int(t.Tt)&api.BIT_ISCOLLECTABLE != 0 }
func (t *TValue) IsTrue() bool            { return !t.IsNil() && !t.IsFalse() }
func (t *TValue) IsFalse() bool           { return int(t.Tt) == api.LUA_VFALSE }
func (t *TValue) IsLClosure() bool         { return int(t.Tt) == api.Ctb(int(api.LUA_VLCL)) }
func (t *TValue) IsCClosure() bool         { return int(t.Tt) == api.Ctb(int(api.LUA_VCCL)) }
func (t *TValue) IsLightCFunction() bool   { return int(t.Tt) == api.LUA_VLCF }
func (t *TValue) IsClosure() bool          { return t.IsLClosure() || t.IsCClosure() }
func (t *TValue) IsProto() bool           { return int(t.Tt) == api.Ctb(int(api.LUA_VPROTO)) }
func (t *TValue) IsUpval() bool           { return int(t.Tt) == api.Ctb(int(api.LUA_VUPVAL)) }
func (t *TValue) IsShortString() bool     { return int(t.Tt) == api.Ctb(int(api.LUA_VSHRSTR)) }
func (t *TValue) IsLongString() bool      { return int(t.Tt) == api.Ctb(int(api.LUA_VLNGSTR)) }
func (t *TValue) IsEmpty() bool           { return api.Novariant(int(t.Tt)) == api.LUA_TNIL }

func (t *TValue) GetTag() int           { return int(t.Tt) }
func (t *TValue) GetBaseType() int      { return api.Novariant(int(t.Tt)) }
func (t *TValue) GetValue() interface{} { return t.Value.Data_ }
func (t *TValue) GetGC() *api.GCObject  { return t.Value.GetGC() }
func (t *TValue) GetInteger() api.LuaInteger { return t.Value.GetInteger() }
func (t *TValue) GetFloat() api.LuaNumber    { return t.Value.GetFloat() }
func (t *TValue) GetPointer() unsafe.Pointer  { return t.Value.GetPointer() }
