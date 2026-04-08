// Package internal provides concrete implementations of api interfaces.
package internal

import (
	"unsafe"

	bcapi "github.com/akzj/go-lua/bytecode/api"
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

// GetProto returns the function prototype as a bcapi.Prototype interface.
func (c *LClosure) GetProto() bcapi.Prototype {
	return c.Proto
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
	LineDefined_     int
	LastLineDefined_ int
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

// bcapi.Prototype interface implementation for Proto.
func (p *Proto) SourceName() string      { return "" }
func (p *Proto) LineDefined() int        { return p.LineDefined_ }
func (p *Proto) LastLineDefined() int    { return p.LastLineDefined_ }
func (p *Proto) NumParams() uint8        { return p.Numparams }
func (p *Proto) MaxStackSize() uint8     { return p.Maxstacksize }
func (p *Proto) GetCode() []uint32       { return p.Code }

func (p *Proto) GetConstants() []*bcapi.Constant {
	result := make([]*bcapi.Constant, len(p.K))
	for i, tv := range p.K {
		result[i] = tvalueToConstant(tv)
	}
	return result
}

func (p *Proto) GetSubProtos() []bcapi.Prototype {
	result := make([]bcapi.Prototype, len(p.P))
	for i, sub := range p.P {
		result[i] = sub
	}
	return result
}

func (p *Proto) GetUpvalues() []bcapi.UpvalueDesc {
	result := make([]bcapi.UpvalueDesc, len(p.Upvalues))
	for i, uv := range p.Upvalues {
		name := ""
		if uv.Name != nil {
			name = string(uv.Name.Contents_)
		}
		result[i] = bcapi.UpvalueDesc{
			Name:    name,
			Instack: uv.Instack,
			Idx:     uv.Idx,
			Kind:    uv.Kind,
		}
	}
	return result
}

func tvalueToConstant(tv *TValue) *bcapi.Constant {
	switch {
	case tv.IsNil():
		return &bcapi.Constant{Type: bcapi.ConstNil}
	case tv.IsInteger():
		return &bcapi.Constant{Type: bcapi.ConstInteger, Int: int64(tv.GetInteger())}
	case tv.IsFloat():
		return &bcapi.Constant{Type: bcapi.ConstFloat, Float: float64(tv.GetFloat())}
	case tv.IsString():
		if s, ok := tv.GetValue().(string); ok {
			return &bcapi.Constant{Type: bcapi.ConstString, Str: s}
		}
		return &bcapi.Constant{Type: bcapi.ConstNil}
	case tv.IsTrue():
		return &bcapi.Constant{Type: bcapi.ConstBool, Int: 1}
	case tv.IsFalse():
		return &bcapi.Constant{Type: bcapi.ConstBool, Int: 0}
	default:
		return &bcapi.Constant{Type: bcapi.ConstNil}
	}
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
