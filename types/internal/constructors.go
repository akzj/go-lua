// Package internal provides concrete implementations of api interfaces.
package internal

import (
	"unsafe"

	"github.com/akzj/go-lua/types/api"
)

// Value constructors
func NewValueGC(gc *api.GCObject) api.Value {
	return &Value{Variant: api.ValueGC, Data_: gc}
}

func NewValuePointer(p unsafe.Pointer) api.Value {
	return &Value{Variant: api.ValuePointer, Data_: p}
}

func NewValueCFunction(f unsafe.Pointer) api.Value {
	return &Value{Variant: api.ValueCFunction, Data_: f}
}

func NewValueInteger(i api.LuaInteger) api.Value {
	return &Value{Variant: api.ValueInteger, Data_: i}
}

func NewValueFloat(n api.LuaNumber) api.Value {
	return &Value{Variant: api.ValueFloat, Data_: n}
}

// TValue constructors
func NewTValueNil() api.TValue {
	return &TValue{Tt: uint8(api.LUA_VNIL)}
}

func NewTValueBoolean(b bool) api.TValue {
	tag := api.LUA_VFALSE
	if b {
		tag = api.LUA_VTRUE
	}
	return &TValue{Tt: uint8(tag)}
}

func NewTValueInteger(i api.LuaInteger) api.TValue {
	return &TValue{Value: Value{Variant: api.ValueInteger, Data_: i}, Tt: uint8(api.LUA_VNUMINT)}
}

func NewTValueFloat(n api.LuaNumber) api.TValue {
	return &TValue{Value: Value{Variant: api.ValueFloat, Data_: n}, Tt: uint8(api.LUA_VNUMFLT)}
}

func NewTValueLightUserData(p unsafe.Pointer) api.TValue {
	return &TValue{Value: Value{Variant: api.ValuePointer, Data_: p}, Tt: uint8(api.LUA_VLIGHTUSERDATA)}
}
