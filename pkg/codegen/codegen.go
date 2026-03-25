// Package codegen generates bytecode from AST.
package codegen

import (
	"fmt"
	"github.com/akzj/go-lua/pkg/object"
	"github.com/akzj/go-lua/pkg/parser"
	"github.com/akzj/go-lua/pkg/vm"
)

// CodeGenerator generates bytecode from AST.
type CodeGenerator struct {
	Prototype      *object.Prototype
	PC             int
	StackTop       int
	MaxStackSize   int
	Upvalues       map[string]int
	Locals         [][]LocalVar
	Constants      map[string]int
	JumpList       []JumpEntry
	breakList      []int
	Parent         *CodeGenerator
}

// LocalVar represents a local variable.
type LocalVar struct {
	Name    string
	Index   int
	Active  bool
	IsParam bool
}

// JumpEntry represents a pending jump to patch.
type JumpEntry struct {
	PC     int
	Target int
}

// NewCodeGenerator creates a code generator.
func NewCodeGenerator() *CodeGenerator {
	return &CodeGenerator{
		Prototype: &object.Prototype{
			Constants:  make([]object.TValue, 0),
			Code:       make([]object.Instruction, 0),
			Upvalues:   make([]object.UpvalueDesc, 0),
			Prototypes: make([]*object.Prototype, 0),
		},
		Upvalues:  make(map[string]int),
		Constants: make(map[string]int),
		Locals:    make([][]LocalVar, 0),
		JumpList:  make([]JumpEntry, 0),
	}
}

// Generate generates bytecode for a function definition.
func (cg *CodeGenerator) Generate(funcDef *parser.FuncDefStmt) *object.Prototype {
	cg.beginScope()
	for i, param := range funcDef.Params {
		cg.addLocal(param.Name, i, true)
	}
	cg.Prototype.NumParams = len(funcDef.Params)
	cg.Prototype.IsVarArg = funcDef.IsVarArg
	cg.genBlock(funcDef.Body)
	cg.emitReturn(0, 1)
	cg.endScope()
	cg.Prototype.MaxStackSize = cg.MaxStackSize
	return cg.Prototype
}

// GenerateFunc generates bytecode for an anonymous function expression.
func (cg *CodeGenerator) GenerateFunc(funcExpr *parser.FuncExpr) *object.Prototype {
	cg.beginScope()
	for i, param := range funcExpr.Params {
		cg.addLocal(param.Name, i, true)
	}
	cg.Prototype.NumParams = len(funcExpr.Params)
	cg.Prototype.IsVarArg = funcExpr.IsVarArg
	cg.genBlock(funcExpr.Body)
	cg.emitReturn(0, 1)
	cg.endScope()
	cg.Prototype.MaxStackSize = cg.MaxStackSize
	return cg.Prototype
}

// beginScope starts a new local variable scope.
func (cg *CodeGenerator) beginScope() {
	cg.Locals = append(cg.Locals, make([]LocalVar, 0))
}

// endScope ends the current local variable scope.
func (cg *CodeGenerator) endScope() {
	if len(cg.Locals) > 0 {
		scope := &cg.Locals[len(cg.Locals)-1]
		for i := range *scope {
			(*scope)[i].Active = false
		}
		cg.Locals = cg.Locals[:len(cg.Locals)-1]
	}
}

// addLocal adds a local variable to the current scope.
func (cg *CodeGenerator) addLocal(name string, index int, isParam bool) {
	if len(cg.Locals) == 0 {
		cg.Locals = append(cg.Locals, make([]LocalVar, 0))
	}
	cg.Locals[len(cg.Locals)-1] = append(cg.Locals[len(cg.Locals)-1], LocalVar{
		Name:    name,
		Index:   index,
		Active:  true,
		IsParam: isParam,
	})
}

// getLocal looks up a local variable by name.
func (cg *CodeGenerator) getLocal(name string) (int, bool) {
	for i := len(cg.Locals) - 1; i >= 0; i-- {
		for _, local := range cg.Locals[i] {
			if local.Name == name && local.Active {
				return local.Index, true
			}
		}
	}
	return -1, false
}

// resolveUpvalue resolves a variable as an upvalue by walking the parent chain.
// Returns the upvalue index in this generator's prototype and true if found.
func (cg *CodeGenerator) resolveUpvalue(name string) (int, bool) {
	if cg.Parent == nil {
		return -1, false
	}

	// Case 1: name is a local in the parent
	if idx, ok := cg.Parent.getLocal(name); ok {
		return cg.addUpvalue(name, object.UpvalueDesc{
			Index:   idx,
			IsLocal: true,
		}), true
	}

	// Case 2: name is already an upvalue in the parent
	if idx, ok := cg.Parent.getUpvalue(name); ok {
		return cg.addUpvalue(name, object.UpvalueDesc{
			Index:   idx,
			IsLocal: false,
		}), true
	}

	// Case 3: recurse — resolve in parent first, then reference parent's new upvalue
	if idx, ok := cg.Parent.resolveUpvalue(name); ok {
		return cg.addUpvalue(name, object.UpvalueDesc{
			Index:   idx,
			IsLocal: false,
		}), true
	}

	return -1, false
}

// allocRegister allocates a register.
func (cg *CodeGenerator) allocRegister() int {
	reg := cg.StackTop
	cg.StackTop++
	if cg.StackTop > cg.MaxStackSize {
		cg.MaxStackSize = cg.StackTop
	}
	// Keep prototype in sync
	if cg.Prototype != nil && cg.MaxStackSize > cg.Prototype.MaxStackSize {
		cg.Prototype.MaxStackSize = cg.MaxStackSize
	}
	return reg
}

// freeRegister frees the top register.
func (cg *CodeGenerator) freeRegister() {
	if cg.StackTop > 0 {
		cg.StackTop--
	}
}

// freeRegisters frees multiple registers.
func (cg *CodeGenerator) freeRegisters(n int) {
	for i := 0; i < n; i++ {
		cg.freeRegister()
	}
}

// setStackTop sets the stack top.
func (cg *CodeGenerator) setStackTop(top int) {
	if top < 0 {
		top = 0
	}
	cg.StackTop = top
}

// EmitABC emits an iABC format instruction.
func (cg *CodeGenerator) EmitABC(op vm.Opcode, a, b, c int) int {
	pc := cg.PC
	instr := object.Instruction(vm.MakeABC(op, a, b, c))
	cg.Prototype.Code = append(cg.Prototype.Code, instr)
	cg.PC++
	return pc
}

// EmitABx emits an iABx format instruction.
func (cg *CodeGenerator) EmitABx(op vm.Opcode, a, bx int) int {
	pc := cg.PC
	instr := object.Instruction(vm.MakeABx(op, a, bx))
	cg.Prototype.Code = append(cg.Prototype.Code, instr)
	cg.PC++
	return pc
}

// EmitAsBx emits an iAsBx format instruction.
func (cg *CodeGenerator) EmitAsBx(op vm.Opcode, a, sbx int) int {
	pc := cg.PC
	instr := object.Instruction(vm.MakeAsBx(op, a, sbx))
	cg.Prototype.Code = append(cg.Prototype.Code, instr)
	cg.PC++
	return pc
}

// EmitAx emits an iAx format instruction.
func (cg *CodeGenerator) EmitAx(op vm.Opcode, ax int) int {
	pc := cg.PC
	instr := object.Instruction(vm.MakeAx(op, ax))
	cg.Prototype.Code = append(cg.Prototype.Code, instr)
	cg.PC++
	return pc
}

// emitJump emits a jump instruction.
func (cg *CodeGenerator) emitJump(target int) int {
	pc := cg.PC
	instr := object.Instruction(vm.MakeABC(vm.OP_JMP, 0, 0, target))
	cg.Prototype.Code = append(cg.Prototype.Code, instr)
	cg.PC++
	return pc
}

// PatchInstruction patches an instruction.
func (cg *CodeGenerator) PatchInstruction(pc int, instr object.Instruction) {
	if pc >= 0 && pc < len(cg.Prototype.Code) {
		cg.Prototype.Code[pc] = instr
	}
}

// patchJump patches a jump target.
func (cg *CodeGenerator) patchJump(pc int, target int) {
	offset := target - (pc + 1)
	cg.PatchInstruction(pc, object.Instruction(vm.MakeAsBx(vm.OP_JMP, 0, offset)))
}

// GetCurrentPC returns the current program counter.
func (cg *CodeGenerator) GetCurrentPC() int {
	return cg.PC
}

// AddConstant adds a constant.
func (cg *CodeGenerator) AddConstant(value object.TValue) int {
	idx := len(cg.Prototype.Constants)
	cg.Prototype.Constants = append(cg.Prototype.Constants, value)
	return idx
}

// addOrGetConstant adds or gets existing constant.
func (cg *CodeGenerator) addOrGetConstant(value object.TValue) int {
	key := cg.constantKey(value)
	if idx, ok := cg.Constants[key]; ok {
		return idx
	}
	idx := len(cg.Prototype.Constants)
	cg.Prototype.Constants = append(cg.Prototype.Constants, value)
	cg.Constants[key] = idx
	return idx
}

// constantKey generates a key for deduplication.
func (cg *CodeGenerator) constantKey(val object.TValue) string {
	switch val.Type {
	case object.TypeNumber:
		return fmt.Sprintf("num:%v", val.Value.Num)
	case object.TypeString:
		return fmt.Sprintf("str:%v", val.Value.Str)
	case object.TypeBoolean:
		return fmt.Sprintf("bool:%v", val.Value.Bool)
	default:
		return fmt.Sprintf("%T", val)
	}
}

// emitReturn emits RETURN: return B-1 values starting at register A (Lua B=0 means multret).
func (cg *CodeGenerator) emitReturn(a int, b int) {
	cg.EmitABC(vm.OP_RETURN, a, b, 0)
}

// genBlock generates code for a block of statements.
func (cg *CodeGenerator) genBlock(block *parser.BlockStmt) {
	for _, stmt := range block.Stmts {
		cg.genStmt(stmt)
	}
}

// emitLoadConstant loads a constant into a register with optimization.
// Small integers (-128 to 127) use LOADI, others use LOADK.
func (cg *CodeGenerator) emitLoadConstant(reg int, val object.TValue) {
	switch val.Type {
	case object.TypeNumber:
		// Check if it's an integer value within small range [-128, 127]
		num := val.Value.Num
		isInteger := num == float64(int(num))
		if isInteger {
			intNum := int(num)
			if intNum >= -128 && intNum <= 127 {
				// Use LOADI for small integers (sBx format)
				cg.EmitAsBx(vm.OP_LOADI, reg, intNum)
				return
			}
		}
		// Use LOADK for other numbers
		idx := cg.addOrGetConstant(val)
		cg.EmitABx(vm.OP_LOADK, reg, idx)
	case object.TypeString:
		idx := cg.addOrGetConstant(val)
		cg.EmitABx(vm.OP_LOADK, reg, idx)
	case object.TypeBoolean:
		cg.EmitABC(vm.OP_LOADBOOL, reg, boolToInt(val.Value.Bool), 0)
	case object.TypeNil:
		cg.EmitABC(vm.OP_LOADNIL, reg, 0, 0)
	default:
		// Fallback: treat as number
		idx := cg.addOrGetConstant(val)
		cg.EmitABx(vm.OP_LOADK, reg, idx)
	}
}

// boolToInt converts bool to int (0 or 1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}