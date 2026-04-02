// Package internal provides the bytecode compiler implementation.
package internal

import (
	"fmt"

	astapi "github.com/akzj/go-lua/ast/api"
	bcapi "github.com/akzj/go-lua/bytecode/api"
)

// =============================================================================
// Compiler Implementation
// =============================================================================

// Compiler implements bcapi.Compiler.
// Transforms ast.Chunk into bytecode Prototype.
type Compiler struct {
	sourceName string
}

// NewCompiler creates a new bytecode compiler.
func NewCompiler(sourceName string) *Compiler {
	return &Compiler{sourceName: sourceName}
}

// Compile implements bcapi.Compiler.
func (c *Compiler) Compile(chunk astapi.Chunk) (bcapi.Prototype, error) {
	if chunk == nil {
		return nil, bcapi.NewCompileError(0, 0, "nil chunk")
	}

	block := chunk.Block()
	if block == nil {
		return nil, bcapi.NewCompileError(0, 0, "nil block")
	}

	proto := &Prototype{
		sourceName:      c.sourceName,
		lineDefined:    0,
		lastLineDefined: 0,
		numparams:      0,
		flag:            0,
		maxstacksize:   2,
	}

	_ = block
	_ = proto

	return proto, nil
}

// =============================================================================
// Prototype - Function Prototype (implements bcapi.Prototype)
// =============================================================================

// Prototype represents a compiled Lua function.
type Prototype struct {
	sourceName      string
	numparams       uint8
	flag            uint8
	maxstacksize    uint8
	sizeupvalues    int
	sizek           int
	sizecode        int
	sizelineinfo    int
	sizep           int
	sizelocvars     int
	sizeabslineinfo int
	lineDefined     int
	lastLineDefined int
	k               []*Constant
	code            []uint32
	p               []*Prototype
	upvalues        []*Upvaldesc
	lineinfo        []int32
	abslineinfo     []*AbsLineInfo
	locvars         []*LocVar
}

// Implement bcapi.Prototype interface
func (p *Prototype) SourceName() string      { return p.sourceName }
func (p *Prototype) LineDefined() int        { return p.lineDefined }
func (p *Prototype) LastLineDefined() int   { return p.lastLineDefined }
func (p *Prototype) NumParams() uint8       { return p.numparams }
func (p *Prototype) IsVararg() bool          { return p.flag&1 != 0 }
func (p *Prototype) MaxStackSize() uint8    { return p.maxstacksize }

// Getters for internal use (avoid interface overhead)
func (p *Prototype) GetCode() []uint32        { return p.code }
func (p *Prototype) GetConstants() []*Constant { return p.k }
func (p *Prototype) GetSubProtos() []*Prototype { return p.p }

// Constant represents a compile-time constant value.
type Constant struct {
	Type  ConstantType
	Int   int64
	Float float64
	Str   string
}

// ConstantType identifies the type of a constant.
type ConstantType uint8

const (
	ConstNil ConstantType = iota
	ConstInteger
	ConstFloat
	ConstString
	ConstBool
)

// Upvaldesc describes an upvalue.
type Upvaldesc struct {
	Name    string
	Instack uint8
	Idx     uint8
	Kind    uint8
}

// LocVar describes a local variable.
type LocVar struct {
	Varname string
	Startpc int
	Endpc   int
}

// AbsLineInfo maps instruction index to absolute line number.
type AbsLineInfo struct {
	Pc   int
	Line int
}

// =============================================================================
// FuncState - Per-Function Compilation State
// =============================================================================

// FuncState maintains compilation state for a single function.
type FuncState struct {
	Proto *Prototype
	pc    int
	Prev  *FuncState
	C     *Compiler
}

// NewFuncState creates a new FuncState.
func NewFuncState(c *Compiler, proto *Prototype) *FuncState {
	return &FuncState{
		Proto: proto,
		pc:    0,
		C:     c,
	}
}

// currentPC returns the current program counter.
func (fs *FuncState) currentPC() int {
	return fs.pc
}

// allocReg allocates a new register.
func (fs *FuncState) allocReg() int {
	reg := int(fs.Proto.maxstacksize)
	fs.Proto.maxstacksize++
	return reg
}

// freeReg frees a register.
func (fs *FuncState) freeReg(reg int) {
	// TODO: implement
}

// free_regs frees multiple registers.
func (fs *FuncState) free_regs(from, n int) {
	// TODO: implement
}

// =============================================================================
// Instruction Emission
// =============================================================================

// emit emits a single ABC instruction.
func (fs *FuncState) emit(opcode, a, b, c int) int {
	inst := encodeABC(opcode, a, b, c)
	fs.Proto.code = append(fs.Proto.code, inst)
	pc := fs.pc
	fs.pc++
	return pc
}

// emitABx emits an ABx instruction.
func (fs *FuncState) emitABx(opcode, a, bx int) int {
	inst := encodeABx(opcode, a, bx)
	fs.Proto.code = append(fs.Proto.code, inst)
	pc := fs.pc
	fs.pc++
	return pc
}

// emitAsBx emits an AsBx instruction (signed).
func (fs *FuncState) emitAsBx(opcode, a, sbx int) int {
	inst := encodeAsBx(opcode, a, sbx)
	fs.Proto.code = append(fs.Proto.code, inst)
	pc := fs.pc
	fs.pc++
	return pc
}

// encodeABC encodes an ABC format instruction.
func encodeABC(opcode, a, b, c int) uint32 {
	return uint32(opcode) | (uint32(a) << 7) | (uint32(b) << 14) | (uint32(c) << 23)
}

// encodeABx encodes an ABx format instruction.
func encodeABx(opcode, a, bx int) uint32 {
	return uint32(opcode) | (uint32(a) << 7) | (uint32(bx) << 14)
}

// encodeAsBx encodes an AsBx format instruction (signed Bx).
func encodeAsBx(opcode, a, sbx int) uint32 {
	return uint32(opcode) | (uint32(a) << 7) | (uint32(sbx+65535) << 14)
}

// =============================================================================
// Constant Management
// =============================================================================

// addConstant adds a constant to the constant table.
func (fs *FuncState) addConstant(c *Constant) int {
	// Simple linear search for now
	for i, k := range fs.Proto.k {
		if k.equals(c) {
			return i
		}
	}
	idx := len(fs.Proto.k)
	fs.Proto.k = append(fs.Proto.k, c)
	return idx
}

func (c *Constant) equals(other *Constant) bool {
	if c.Type != other.Type {
		return false
	}
	switch c.Type {
	case ConstNil:
		return true
	case ConstInteger:
		return c.Int == other.Int
	case ConstFloat:
		return c.Float == other.Float
	case ConstString:
		return c.Str == other.Str
	case ConstBool:
		return c.Int == other.Int
	}
	return false
}

// NewConstInteger creates an integer constant.
func NewConstInteger(i int64) *Constant {
	return &Constant{Type: ConstInteger, Int: i}
}

// NewConstFloat creates a float constant.
func NewConstFloat(f float64) *Constant {
	return &Constant{Type: ConstFloat, Float: f}
}

// NewConstString creates a string constant.
func NewConstString(s string) *Constant {
	return &Constant{Type: ConstString, Str: s}
}

// NewConstBool creates a boolean constant.
func NewConstBool(b bool) *Constant {
	i := int64(0)
	if b {
		i = 1
	}
	return &Constant{Type: ConstBool, Int: i}
}

// =============================================================================
// Helper Methods
// =============================================================================

// errorf reports a compilation error.
func (fs *FuncState) errorf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}

// =============================================================================
// Locals - Local Variable Management
// =============================================================================

// Locals tracks local variables during compilation.
type Locals struct {
	vars []VarInfo
}

// VarInfo describes a local variable.
type VarInfo struct {
	Name  string
	Reg   int
	Scope int
}

// NewLocals creates a new locals tracker.
func NewLocals() Locals {
	return Locals{
		vars: make([]VarInfo, 0),
	}
}

// Add registers a new local variable.
func (l *Locals) Add(name string, reg, scope int) {
	l.vars = append(l.vars, VarInfo{Name: name, Reg: reg, Scope: scope})
}

// Get returns the variable at the given index.
func (l *Locals) Get(index int) *VarInfo {
	if index < 0 || index >= len(l.vars) {
		return nil
	}
	return &l.vars[index]
}

// Count returns the number of local variables.
func (l *Locals) Count() int {
	return len(l.vars)
}
