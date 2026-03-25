// Package codegen generates bytecode from AST
package codegen

import (
	"github.com/akzj/go-lua/pkg/parser"
	"github.com/akzj/go-lua/pkg/vm"
	"github.com/akzj/go-lua/pkg/object"
)

// CodeGenerator generates bytecode from AST
type CodeGenerator struct {
	Prototype  *object.Prototype
	PC         int
	StackSize  int
	MaxStackSize int
	Upvalues   map[string]int
	Locals     []LocalVar
	Constants  map[string]int
}

// LocalVar represents a local variable
type LocalVar struct {
	Name  string
	Index int
	Active bool
}

// NewCodeGenerator creates a code generator
func NewCodeGenerator() *CodeGenerator {
	return &CodeGenerator{
		Prototype: &object.Prototype{
			Constants: make([]object.TValue, 0),
			Code:      make([]object.Instruction, 0),
			Upvalues:  make([]object.UpvalueDesc, 0),
			Prototypes: make([]*object.Prototype, 0),
		},
		Upvalues:  make(map[string]int),
		Constants: make(map[string]int),
	}
}

// Generate generates bytecode for a function
func (cg *CodeGenerator) Generate(funcDef *parser.FuncStmt) *object.Prototype {
	// TODO: Implement full code generation
	return cg.Prototype
}

// EmitABC emits an iABC format instruction
func (cg *CodeGenerator) EmitABC(op vm.Opcode, a, b, c int) int {
	pc := cg.PC
	instr := object.Instruction(vm.MakeABC(op, a, b, c))
	cg.Prototype.Code = append(cg.Prototype.Code, instr)
	cg.PC++
	return pc
}

// EmitABx emits an iABx format instruction
func (cg *CodeGenerator) EmitABx(op vm.Opcode, a, bx int) int {
	pc := cg.PC
	instr := object.Instruction(vm.MakeABx(op, a, bx))
	cg.Prototype.Code = append(cg.Prototype.Code, instr)
	cg.PC++
	return pc
}

// EmitAsBx emits an iAsBx format instruction
func (cg *CodeGenerator) EmitAsBx(op vm.Opcode, a, sbx int) int {
	pc := cg.PC
	instr := object.Instruction(vm.MakeAsBx(op, a, sbx))
	cg.Prototype.Code = append(cg.Prototype.Code, instr)
	cg.PC++
	return pc
}

// EmitAx emits an iAx format instruction
func (cg *CodeGenerator) EmitAx(op vm.Opcode, ax int) int {
	pc := cg.PC
	instr := object.Instruction(vm.MakeAx(op, ax))
	cg.Prototype.Code = append(cg.Prototype.Code, instr)
	cg.PC++
	return pc
}

// AddConstant adds a constant to the constant table
func (cg *CodeGenerator) AddConstant(value object.TValue) int {
	idx := len(cg.Prototype.Constants)
	cg.Prototype.Constants = append(cg.Prototype.Constants, value)
	return idx
}

// PatchInstruction patches an instruction at the given PC
func (cg *CodeGenerator) PatchInstruction(pc int, instr object.Instruction) {
	if pc >= 0 && pc < len(cg.Prototype.Code) {
		cg.Prototype.Code[pc] = instr
	}
}

// GetCurrentPC returns the current program counter
func (cg *CodeGenerator) GetCurrentPC() int {
	return cg.PC
}