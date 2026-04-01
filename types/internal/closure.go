// Package internal provides concrete implementations of api interfaces.
package internal

import (
	"unsafe"

	"github.com/akzj/go-lua/types/api"
)

// UpVal is the concrete UpVal implementation.
type UpVal struct {
	GCObject
	V struct {
		P unsafe.Pointer
	}
	U struct {
		Open struct {
			Next     *UpVal
			Previous **UpVal
		}
		Value TValue
	}
}

// ClosureHeader shared by both closure types.
type ClosureHeader struct {
	GCObject
	Nupvalues uint8
	GClist    *GCObject
}

// CClosure is the concrete CClosure implementation.
type CClosure struct {
	ClosureHeader
	F unsafe.Pointer
}

// LClosure is the concrete LClosure implementation.
type LClosure struct {
	ClosureHeader
	Proto  *Proto
	Upvals []*UpVal
}

// Closure is the concrete Closure implementation.
type Closure struct {
	IsCClosure bool
	C          *CClosure
	L          *LClosure
}

func (c *Closure) IsC() bool { return c.IsCClosure }

// Proto is the concrete Proto implementation.
type Proto struct {
	GCObject
	Numparams       uint8
	Flag            uint8
	Maxstacksize    uint8
	Sizeupvalues    int
	Sizek           int
	Sizecode        int
	Sizelineinfo    int
	Sizep           int
	Sizelocvars     int
	Sizeabslineinfo int
	LineDefined     int
	LastLineDefined int
	K               []*TValue
	Code            []uint32
	P               []*Proto
	Upvalues        []*Upvaldesc
	Lineinfo        []int8
	Abslineinfo     []*AbsLineInfo
	Locvars         []*LocVar
	Source          *TString
	GClist          *GCObject
}

func (p *Proto) IsVararg() bool {
	return p.Flag&(api.PF_VARARG_HIDDEN|api.PF_VARARG_TABLE) != 0
}

// Upvaldesc is the concrete Upvaldesc implementation.
type Upvaldesc struct {
	Name    *TString
	Instack uint8
	Idx     uint8
	Kind    uint8
}

// LocVar is the concrete LocVar implementation.
type LocVar struct {
	Varname *TString
	Startpc int
	Endpc   int
}

// AbsLineInfo is the concrete AbsLineInfo implementation.
type AbsLineInfo struct {
	Pc   int
	Line int
}

// Udata is the concrete Udata implementation.
type Udata struct {
	GCObject
	Nuvalue   uint16
	Len       uintptr
	Metatable *Table
	GClist    *GCObject
}
