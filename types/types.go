// Package types provides Lua 5.5.1 core type definitions.
// Re-export from internal/api and internal/impl.
package types

import (
	"github.com/akzj/go-lua/types/api"
	impl "github.com/akzj/go-lua/types/internal"
)

// Type constants
const (
	LUA_TNIL           = api.LUA_TNIL
	LUA_TBOOLEAN       = api.LUA_TBOOLEAN
	LUA_TLIGHTUSERDATA = api.LUA_TLIGHTUSERDATA
	LUA_TNUMBER        = api.LUA_TNUMBER
	LUA_TSTRING        = api.LUA_TSTRING
	LUA_TTABLE         = api.LUA_TTABLE
	LUA_TFUNCTION      = api.LUA_TFUNCTION
	LUA_TUSERDATA      = api.LUA_TUSERDATA
	LUA_TTHREAD        = api.LUA_TTHREAD
	LUA_NUMTYPES       = api.LUA_NUMTYPES
	LUA_TUPVAL         = api.LUA_TUPVAL
	LUA_TPROTO         = api.LUA_TPROTO
	LUA_TDEADKEY       = api.LUA_TDEADKEY
	LUA_TOTALTYPES     = api.LUA_TOTALTYPES

	LUA_VNIL           = api.LUA_VNIL
	LUA_VEMPTY         = api.LUA_VEMPTY
	LUA_VABSTKEY       = api.LUA_VABSTKEY
	LUA_VNOTABLE       = api.LUA_VNOTABLE
	LUA_VFALSE         = api.LUA_VFALSE
	LUA_VTRUE          = api.LUA_VTRUE
	LUA_VNUMINT        = api.LUA_VNUMINT
	LUA_VNUMFLT        = api.LUA_VNUMFLT
	LUA_VSHRSTR        = api.LUA_VSHRSTR
	LUA_VLNGSTR        = api.LUA_VLNGSTR
	LUA_VLCL           = api.LUA_VLCL
	LUA_VLCF           = api.LUA_VLCF
	LUA_VCCL           = api.LUA_VCCL
	LUA_VTHREAD        = api.LUA_VTHREAD
	LUA_VLIGHTUSERDATA = api.LUA_VLIGHTUSERDATA
	LUA_VUSERDATA      = api.LUA_VUSERDATA
	LUA_VPROTO         = api.LUA_VPROTO
	LUA_VUPVAL         = api.LUA_VUPVAL
	LUA_VTABLE         = api.LUA_VTABLE

	PF_VARARG_HIDDEN = api.PF_VARARG_HIDDEN
	PF_VARARG_TABLE  = api.PF_VARARG_TABLE
	PF_FIXED         = api.PF_FIXED

	LUA_MININTEGER = api.LUA_MININTEGER
	LUA_MAXINTEGER = api.LUA_MAXINTEGER

	BIT_ISCOLLECTABLE = api.BIT_ISCOLLECTABLE
)

const (
	ValueGC        = api.ValueGC
	ValuePointer   = api.ValuePointer
	ValueCFunction = api.ValueCFunction
	ValueInteger   = api.ValueInteger
	ValueFloat     = api.ValueFloat
)

// Constructor functions
var (
	NewTValueNil           = impl.NewTValueNil
	NewTValueBoolean       = impl.NewTValueBoolean
	NewTValueInteger       = impl.NewTValueInteger
	NewTValueFloat         = impl.NewTValueFloat
	NewTValueLightUserData = impl.NewTValueLightUserData
	NewValueGC             = impl.NewValueGC
	NewValuePointer        = impl.NewValuePointer
	NewValueCFunction      = impl.NewValueCFunction
	NewValueInteger        = impl.NewValueInteger
	NewValueFloat          = impl.NewValueFloat
	NewTable               = impl.NewTable
	NewString              = impl.NewString
)

// Utility functions
var (
	MakeVariant = api.MakeVariant
	Ctb         = api.Ctb
	Novariant   = api.Novariant
	WithVariant = api.WithVariant
)
