package codegen

import (
	"github.com/akzj/go-lua/pkg/object"
	"github.com/akzj/go-lua/pkg/parser"
	"github.com/akzj/go-lua/pkg/vm"
)

// genStmt generates code for a statement.
//
// Statements do not produce values; they consume expression results.
//
// Parameters:
//   - stmt: The statement AST node
func (cg *CodeGenerator) genStmt(stmt parser.Stmt) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *parser.BlockStmt:
		cg.genBlock(s)

	case *parser.AssignStmt:
		cg.genAssign(s)

	case *parser.LocalStmt:
		cg.genLocal(s)

	case *parser.IfStmt:
		cg.genIf(s)

	case *parser.WhileStmt:
		cg.genWhile(s)

	case *parser.RepeatStmt:
		cg.genRepeat(s)

	case *parser.ForNumericStmt:
		cg.genForNumeric(s)

	case *parser.ForGenericStmt:
		cg.genForGeneric(s)

	case *parser.ReturnStmt:
		cg.genReturn(s)

	case *parser.BreakStmt:
		cg.genBreak(s)

	case *parser.ExprStmt:
		cg.genExprStmt(s)

	case *parser.FuncDefStmt:
		cg.genFuncDef(s)

	case *parser.GotoStmt, *parser.LabelStmt:
		// TODO: Implement goto and labels
		// For now, ignore

	default:
		// Unknown statement type, ignore
	}
}

// genAssign generates code for an assignment statement.
// left1, left2, ... = right1, right2, ...
func (cg *CodeGenerator) genAssign(stmt *parser.AssignStmt) {
	if len(stmt.Left) == 0 || len(stmt.Right) == 0 {
		return
	}

	// Save current stack top
	savedStackTop := cg.StackTop

	// Generate all right-hand side values first
	rightRegs := make([]int, len(stmt.Right))
	for i, expr := range stmt.Right {
		rightRegs[i] = cg.genExpr(expr)
	}

	// Assign to left-hand side
	for i, left := range stmt.Left {
		var valueReg int
		if i < len(rightRegs) {
			valueReg = rightRegs[i]
		} else {
			// More variables than values, use nil
			valueReg = cg.genNil()
		}

		cg.assignToVar(left, valueReg)

		// Free value register if it's not used anymore
		if i < len(rightRegs)-1 {
			cg.freeRegister()
		}
	}

	// Free remaining right-hand side registers
	for range rightRegs {
		if cg.StackTop > savedStackTop {
			cg.freeRegister()
		}
	}

	// Restore stack top
	cg.setStackTop(savedStackTop)
}

// assignToVar assigns a value register to a variable expression.
func (cg *CodeGenerator) assignToVar(expr parser.Expr, valueReg int) {
	switch e := expr.(type) {
	case *parser.VarExpr:
		// Simple variable
		if idx, ok := cg.getLocal(e.Name); ok {
			// Local variable: MOVE
			cg.EmitABC(vm.OP_MOVE, idx, valueReg, 0)
		} else {
			// Global variable: SETTABLE on _ENV
			// Simplified: store in global table
			keyReg := cg.allocRegister()
			cg.emitLoadConstant(keyReg, *object.NewString(e.Name))
			cg.EmitABC(vm.OP_SETTABLE, 0, keyReg, valueReg) // 0 = _ENV (simplified)
			cg.freeRegister()
		}

	case *parser.IndexExpr:
		// table[index] = value
		tableReg := cg.genExpr(e.Table)
		indexReg := cg.genExpr(e.Index)
		cg.EmitABC(vm.OP_SETTABLE, tableReg, indexReg, valueReg)
		cg.freeRegisters(2)

	case *parser.FieldExpr:
		// table.field = value
		tableReg := cg.genExpr(e.Table)
		fieldIdx := cg.addOrGetConstant(*object.NewString(e.Field))
		if fieldIdx <= 255 {
			cg.EmitABC(vm.OP_SETFIELD, tableReg, fieldIdx, valueReg)
		} else {
			keyReg := cg.allocRegister()
			cg.EmitABx(vm.OP_LOADK, keyReg, fieldIdx)
			cg.EmitABC(vm.OP_SETTABLE, tableReg, keyReg, valueReg)
			cg.freeRegister()
		}
		cg.freeRegister()

	case *parser.CallExpr, *parser.MethodCallExpr:
		// Cannot assign to function call result
		// This should be caught by parser
	}
}

// genLocal generates code for a local variable declaration.
// local name1, name2, ... = value1, value2, ...
func (cg *CodeGenerator) genLocal(stmt *parser.LocalStmt) {
	// Generate values
	values := make([]int, len(stmt.Values))
	for i, expr := range stmt.Values {
		values[i] = cg.genExpr(expr)
	}

	// Add local variables and generate MOVE instructions
	for i, name := range stmt.Names {
		// Allocate register for this local
		reg := cg.allocRegister()

		// Add to local scope
		cg.addLocal(name.Name, reg, false)

		// Assign value if available
		if i < len(values) {
			if values[i] != reg {
				cg.EmitABC(vm.OP_MOVE, reg, values[i], 0)
			}
			cg.freeRegister() // Free value register
		} else {
			// No value, initialize to nil
			cg.EmitABC(vm.OP_LOADNIL, reg, 0, 0)
		}
	}

	// Free any remaining value registers
	for i := len(stmt.Values); i < len(values); i++ {
		cg.freeRegister()
	}
}

// genIf generates code for an if-then-elseif-else statement.
func (cg *CodeGenerator) genIf(stmt *parser.IfStmt) {
	// Generate condition
	condReg := cg.genExpr(stmt.Cond)

	// Test condition and jump to else/elseif if false
	// if not (R(A) ~= 0) then pc++
	cg.EmitABC(vm.OP_TEST, condReg, 0, 0)
	jumpPC := cg.EmitAsBx(vm.OP_JMP, 0, 0) // Forward jump placeholder
	cg.freeRegister()

	// Generate then block
	cg.genBlock(stmt.Then)

	// Jump over else/elseif blocks
	endJump := cg.EmitAsBx(vm.OP_JMP, 0, 0)

	// Patch condition jump to here (start of elseif/else)
	cg.patchJump(jumpPC, cg.GetCurrentPC())

	// Generate elseif blocks
	for _, elseif := range stmt.ElseIf {
		// Generate condition
		condReg := cg.genExpr(elseif.Cond)

		// Test and jump
		cg.EmitABC(vm.OP_TEST, condReg, 0, 0)
		jumpPC := cg.EmitAsBx(vm.OP_JMP, 0, 0)
		cg.freeRegister()

		// Generate then block
		cg.genBlock(elseif.Then)

		// Jump over remaining
		_ = cg.EmitAsBx(vm.OP_JMP, 0, 0)

		// Patch condition jump
		cg.patchJump(jumpPC, cg.GetCurrentPC())
	}

	// Generate else block if present
	if stmt.Else != nil {
		cg.genBlock(stmt.Else)
	}

	// Patch end jump
	cg.patchJump(endJump, cg.GetCurrentPC())
}

// genWhile generates code for a while-do-end loop.
func (cg *CodeGenerator) genWhile(stmt *parser.WhileStmt) {
	// Loop start label
	loopStart := cg.GetCurrentPC()

	// Generate condition
	condReg := cg.genExpr(stmt.Cond)

	// Test condition and jump to end if false
	cg.EmitABC(vm.OP_TEST, condReg, 0, 0)
	endJump := cg.EmitAsBx(vm.OP_JMP, 0, 0)
	cg.freeRegister()

	// Generate body
	cg.genBlock(stmt.Body)

	// Jump back to condition
	cg.EmitAsBx(vm.OP_JMP, 0, loopStart-cg.GetCurrentPC()-1)

	// Loop end - patch jump
	cg.patchJump(endJump, cg.GetCurrentPC())
}

// genRepeat generates code for a repeat-until loop.
func (cg *CodeGenerator) genRepeat(stmt *parser.RepeatStmt) {
	// Loop start label
	loopStart := cg.GetCurrentPC()

	// Generate body
	cg.genBlock(stmt.Body)

	// Generate condition
	condReg := cg.genExpr(stmt.Cond)

	// Test condition and jump back if false (repeat until true)
	cg.EmitABC(vm.OP_TEST, condReg, 0, 0)
	cg.EmitAsBx(vm.OP_JMP, 0, loopStart-cg.GetCurrentPC()-1)
	cg.freeRegister()
}

// genForNumeric generates code for a numeric for loop.
// for i = start, end, step do ... end
func (cg *CodeGenerator) genForNumeric(stmt *parser.ForNumericStmt) {
	// Allocate registers for for-loop control
	// R(A) = index, R(A+1) = limit, R(A+2) = step
	baseReg := cg.StackTop

	// Generate start, end, step values
	startReg := cg.genExpr(stmt.From)
	endReg := cg.genExpr(stmt.To)

	var stepReg int
	if stmt.Step != nil {
		stepReg = cg.genExpr(stmt.Step)
	} else {
		// Default step = 1
		stepReg = cg.allocRegister()
		cg.EmitABC(vm.OP_LOADI, stepReg, 1, 0)
	}

	// Move values to consecutive registers
	cg.EmitABC(vm.OP_MOVE, baseReg, startReg, 0)
	cg.EmitABC(vm.OP_MOVE, baseReg+1, endReg, 0)
	cg.EmitABC(vm.OP_MOVE, baseReg+2, stepReg, 0)

	cg.freeRegisters(3)

	// Add loop variable as local
	loopVarReg := cg.allocRegister()
	cg.addLocal(stmt.Var.Name, loopVarReg, false)

	// FORPREP R(A) sBx
	// Prepares the numeric for loop
	forPrepPC := cg.EmitAsBx(vm.OP_FORPREP, baseReg, 0) // Placeholder offset

	// Generate body
	cg.genBlock(stmt.Body)

	// FORLOOP R(A) sBx
	// Executes the numeric for loop
	forLoopPC := cg.EmitAsBx(vm.OP_FORLOOP, baseReg, 0) // Placeholder offset

	// Patch FORPREP offset (jump to after FORLOOP)
	forPrepOffset := cg.GetCurrentPC() - (forPrepPC + 1)
	cg.PatchInstruction(forPrepPC, object.Instruction(vm.MakeAsBx(vm.OP_FORPREP, baseReg, forPrepOffset)))

	// Patch FORLOOP offset (jump back to loop body)
	forLoopOffset := (forPrepPC + 1) - (forLoopPC + 1)
	cg.PatchInstruction(forLoopPC, object.Instruction(vm.MakeAsBx(vm.OP_FORLOOP, baseReg, forLoopOffset)))
}

// genForGeneric generates code for a generic for loop.
// for k, v in pairs(t) do ... end
func (cg *CodeGenerator) genForGeneric(stmt *parser.ForGenericStmt) {
	// Allocate registers for for-loop control
	// R(A) = iterator function, R(A+1) = state, R(A+2) = control variable
	baseReg := cg.StackTop

	// Split Exprs into iterator function and arguments
	// Exprs[0] is the iterator function, rest are arguments
	if len(stmt.Exprs) == 0 {
		return
	}

	funcExpr := stmt.Exprs[0]
	args := stmt.Exprs[1:]

	// Generate iterator function
	funcReg := cg.genExpr(funcExpr)

	// Generate arguments (state and initial value)
	argRegs := make([]int, len(args))
	for i, arg := range args {
		argRegs[i] = cg.genExpr(arg)
	}

	// Move to consecutive registers
	cg.EmitABC(vm.OP_MOVE, baseReg, funcReg, 0)
	cg.freeRegister()

	for i, argReg := range argRegs {
		if i < 2 { // FORGPREP expects up to 2 additional values
			cg.EmitABC(vm.OP_MOVE, baseReg+1+i, argReg, 0)
			cg.freeRegister()
		}
	}

	// Add loop variables as locals
	for _, name := range stmt.Vars {
		reg := cg.allocRegister()
		cg.addLocal(name.Name, reg, false)
	}

	// FORGPREP R(A) sBx
	// Prepares the generic for loop
	forGPrepPC := cg.EmitAsBx(vm.OP_FORGPREP, baseReg, 0)

	// Generate body
	cg.genBlock(stmt.Body)

	// FORGLOOP R(A) sBx
	// Executes the generic for loop
	forGLoopPC := cg.EmitAsBx(vm.OP_FORGLOOP, baseReg, 0)

	// Patch offsets
	forGPrepOffset := cg.GetCurrentPC() - (forGPrepPC + 1)
	cg.PatchInstruction(forGPrepPC, object.Instruction(vm.MakeAsBx(vm.OP_FORGPREP, baseReg, forGPrepOffset)))

	forGLoopOffset := (forGPrepPC + 1) - (forGLoopPC + 1)
	cg.PatchInstruction(forGLoopPC, object.Instruction(vm.MakeAsBx(vm.OP_FORGLOOP, baseReg, forGLoopOffset)))
}

// genReturn generates code for a return statement.
func (cg *CodeGenerator) genReturn(stmt *parser.ReturnStmt) {
	if len(stmt.Values) == 0 {
		// return with no values
		cg.emitReturn(0, 1) // A=0 (base), B=1 (0 results)
		return
	}

	// Generate return values
	baseReg := cg.StackTop
	for i, expr := range stmt.Values {
		reg := cg.genExpr(expr)
		if reg != baseReg+i {
			cg.EmitABC(vm.OP_MOVE, baseReg+i, reg, 0)
			cg.freeRegister()
		}
	}

	// RETURN R(A) B C
	// B = number of results + 1, 0 = all available
	cg.emitReturn(baseReg, len(stmt.Values)+1)
}

// genBreak generates code for a break statement.
func (cg *CodeGenerator) genBreak(stmt *parser.BreakStmt) {
	// Jump to end of innermost loop
	// This requires loop context tracking
	// Simplified: emit JMP with 0 offset (to be patched by loop)
	cg.EmitAsBx(vm.OP_JMP, 0, 0)
}

// genExprStmt generates code for an expression statement.
// Typically used for function calls where the result is discarded.
func (cg *CodeGenerator) genExprStmt(stmt *parser.ExprStmt) {
	_ = cg.genExpr(stmt.Expr)
	cg.freeRegister()
}

// genFuncDef generates code for a function definition statement.
func (cg *CodeGenerator) genFuncDef(stmt *parser.FuncDefStmt) {
	// Create nested function prototype
	nestedGen := NewCodeGenerator()
	
	var nestedProto *object.Prototype
	
	if stmt.Func != nil {
		// Test case: Func field contains nested Params + Body
		nestedGen.GenerateFunc(stmt.Func)
	} else {
		// Parser case: flat Params + Body
		nestedGen.GenerateFunc(&parser.FuncExpr{
			Params:   stmt.Params,
			Body:     stmt.Body,
			IsVarArg: stmt.IsVarArg,
		})
	}
	
	nestedProto = nestedGen.Prototype

	// Add to parent's prototype table
	protoIdx := len(cg.Prototype.Prototypes)
	cg.Prototype.Prototypes = append(cg.Prototype.Prototypes, nestedProto)

	// Generate code to assign the function to the target
	if stmt.IsLocal && len(stmt.Name) > 0 {
		// local function name() ... end
		reg := cg.allocRegister()
		cg.EmitABx(vm.OP_CLOSURE, reg, protoIdx)
		cg.addLocal(stmt.Name[0].Name, reg, false)
	} else if len(stmt.Name) > 0 {
		// function name() ... end (global or field)
		reg := cg.allocRegister()
		cg.EmitABx(vm.OP_CLOSURE, reg, protoIdx)

		// Assign to variable - use first name for simple cases
		nameExpr := stmt.Name[0]
		cg.assignToVar(nameExpr, reg)
		cg.freeRegister()
	}
}