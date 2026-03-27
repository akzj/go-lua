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

	// Track source line for error messages
	if line := stmt.Line(); line > 0 {
		cg.currentLine = line
	}

	switch s := stmt.(type) {
	case *parser.BlockStmt:
		cg.genBlock(s)

	case *parser.DoStmt:
		cg.genDo(s)

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
		// Handle goto and labels
		switch s := stmt.(type) {
		case *parser.GotoStmt:
			cg.genGoto(s)
		case *parser.LabelStmt:
			cg.genLabel(s)
		}

	case *parser.GlobalStmt:
		cg.genGlobal(s)

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
		} else if upIdx, ok := cg.getUpvalue(e.Name); ok {
			// Upvalue: SETUPVAL
			cg.EmitABC(vm.OP_SETUPVAL, valueReg, upIdx, 0)
		} else if upIdx, ok := cg.resolveUpvalue(e.Name); ok {
			cg.EmitABC(vm.OP_SETUPVAL, valueReg, upIdx, 0)
		} else {
			// Global: SETTABUP A=0, B=constIdx, C=RK(value)
			// B is constant index directly (K[B] format), C is RK mode
			nameIdx := cg.addOrGetConstant(*object.NewString(e.Name))
			cg.EmitABC(vm.OP_SETTABUP, 0, nameIdx, valueReg)
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
	// Special case: if there are more names than values and the last value is a call expression,
	// the call should return all its results to fill the remaining variables
	numNames := len(stmt.Names)
	numValues := len(stmt.Values)

	// Check if the last value is a call expression or vararg
	var lastIsCall bool
	var lastIsDots bool
	if numValues > 0 {
		lastVal := stmt.Values[numValues-1]
		_, lastIsCall = lastVal.(*parser.CallExpr)
		if !lastIsCall {
			_, lastIsCall = lastVal.(*parser.MethodCallExpr)
		}
		if !lastIsCall {
			_, lastIsDots = lastVal.(*parser.DotsExpr)
		}
	}

	// If the last value is a call/vararg and we have more names than values,
	// we need to set ExpectedResults to get all results
	if (lastIsCall || lastIsDots) && numNames > numValues {
		// Save old ExpectedResults
		oldExpected := cg.ExpectedResults
		
		// Calculate how many results we need from the call/vararg
		numResultsNeeded := numNames - (numValues - 1)
		cg.ExpectedResults = numResultsNeeded // Exact number of results needed

		// Process all values except the last one normally
		for i := 0; i < numValues-1; i++ {
			valueReg := cg.genExpr(stmt.Values[i])
			cg.addLocal(stmt.Names[i].Name, valueReg, false)
		}

		// Generate the last value (the call/vararg) which will return multiple results
		lastValueReg := cg.genExpr(stmt.Values[numValues-1])
		
		// Update StackTop to account for all the results
		// The results are at lastValueReg, lastValueReg+1, ..., lastValueReg+numResultsNeeded-1
		cg.setStackTop(lastValueReg + numResultsNeeded)
		if cg.StackTop > cg.MaxStackSize {
			cg.MaxStackSize = cg.StackTop
		}

		// The call/vararg results are in consecutive registers starting at lastValueReg
		// Assign them to the remaining names
		for i := numValues - 1; i < numNames; i++ {
			cg.addLocal(stmt.Names[i].Name, lastValueReg+(i-(numValues-1)), false)
		}

		// Restore ExpectedResults
		cg.ExpectedResults = oldExpected
		return
	}

	// Normal case: one value per name (or fewer values than names, with nil for missing)
	for i, name := range stmt.Names {
		if i < len(stmt.Values) {
			// Generate value — this allocates a register at StackTop
			valueReg := cg.genExpr(stmt.Values[i])
			// The value is already in valueReg which is at the right position
			// Register valueReg as the local variable
			cg.addLocal(name.Name, valueReg, false)
		} else {
			// No value, initialize to nil
			reg := cg.allocRegister()
			cg.EmitABC(vm.OP_LOADNIL, reg, reg, 0)
			cg.addLocal(name.Name, reg, false)
		}
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
	cg.beginScope()
	cg.genBlock(stmt.Then)
	cg.endScope()

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
		cg.beginScope()
		cg.genBlock(elseif.Then)
		cg.endScope()

		// Jump over remaining
		_ = cg.EmitAsBx(vm.OP_JMP, 0, 0)

		// Patch condition jump
		cg.patchJump(jumpPC, cg.GetCurrentPC())
	}

	// Generate else block if present
	if stmt.Else != nil {
		cg.beginScope()
		cg.genBlock(stmt.Else)
		cg.endScope()
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
	cg.beginScope()
	cg.genBlock(stmt.Body)
	cg.endScope()

	// Jump back to condition
	cg.EmitAsBx(vm.OP_JMP, 0, loopStart-cg.GetCurrentPC()-1)

	// Loop end - patch jump
	cg.patchJump(endJump, cg.GetCurrentPC())
}

// genRepeat generates code for a repeat-until loop.
// Note: In Lua, the until condition can reference locals declared in the body,
// so we must keep the scope open until after the condition is evaluated.
func (cg *CodeGenerator) genRepeat(stmt *parser.RepeatStmt) {
	// Loop start label
	loopStart := cg.GetCurrentPC()

	// Generate body - begin scope for locals
	cg.beginScope()
	cg.genBlock(stmt.Body)

	// Generate condition WHILE STILL IN SCOPE (Lua semantics)
	condReg := cg.genExpr(stmt.Cond)

	// Test condition and jump back if false (repeat until true)
	cg.EmitABC(vm.OP_TEST, condReg, 0, 0)
	cg.EmitAsBx(vm.OP_JMP, 0, loopStart-cg.GetCurrentPC()-1)
	cg.freeRegister()

	// End scope AFTER condition evaluation
	cg.endScope()
}

// genForNumeric generates code for a numeric for loop.
// for i = start, end, step do ... end
func (cg *CodeGenerator) genForNumeric(stmt *parser.ForNumericStmt) {
	// Allocate registers for for-loop control
	// R(A) = index, R(A+1) = limit, R(A+2) = step, R(A+3) = external loop var
	baseReg := cg.StackTop

	// Generate start, end, step values directly into consecutive registers
	startReg := cg.genExpr(stmt.From)
	endReg := cg.genExpr(stmt.To)

	var stepReg int
	if stmt.Step != nil {
		stepReg = cg.genExpr(stmt.Step)
	} else {
		// Default step = 1
		stepReg = cg.allocRegister()
		cg.EmitAsBx(vm.OP_LOADI, stepReg, 1)
	}

	// Ensure values are in consecutive registers starting at baseReg
	// If genExpr already allocated them consecutively (which it should),
	// no MOVEs are needed. But if not, we need to move them.
	if startReg != baseReg {
		cg.EmitABC(vm.OP_MOVE, baseReg, startReg, 0)
	}
	if endReg != baseReg+1 {
		cg.EmitABC(vm.OP_MOVE, baseReg+1, endReg, 0)
	}
	if stepReg != baseReg+2 {
		cg.EmitABC(vm.OP_MOVE, baseReg+2, stepReg, 0)
	}

	// Set stack top to baseReg+3 (after the 3 control registers)
	cg.setStackTop(baseReg + 3)
	if cg.StackTop > cg.MaxStackSize {
		cg.MaxStackSize = cg.StackTop
	}

	// Allocate register for external loop variable R(A+3)
	loopVarReg := cg.allocRegister() // baseReg+3
	// Begin scope for loop variable — it must not leak to outer scope
	cg.beginScope()
	cg.addLocal(stmt.Var.Name, loopVarReg, false)

	// FORPREP R(A) sBx — jump forward to FORLOOP
	forPrepPC := cg.EmitAsBx(vm.OP_FORPREP, baseReg, 0) // Placeholder offset

	// Loop body start
	bodyStart := cg.GetCurrentPC()

	// Begin scope for loop body - each iteration gets fresh locals
	cg.beginScope()

	// Generate body
	cg.genBlock(stmt.Body)

	// End scope for loop body - mark locals as inactive
	cg.endScope()

	// Close any upvalues captured by closures in the body.
	// This moves captured locals from the stack to the heap,
	// allowing the stack slots to be reused in the next iteration.
	// CLOSE all upvalues at or above baseReg+4 (first body register).
	cg.EmitABC(vm.OP_CLOSE, baseReg+4, 0, 0)

	// Reset stack top to just after loop variable for next iteration
	// This ensures temp registers are freed between iterations
	cg.setStackTop(baseReg + 4)

	// FORLOOP R(A) sBx — jump back to loop body start
	forLoopPC := cg.EmitAsBx(vm.OP_FORLOOP, baseReg, 0) // Placeholder offset

	// Patch FORPREP: jump from forPrepPC to forLoopPC (the FORLOOP instruction)
	forPrepOffset := forLoopPC - forPrepPC - 1
	cg.PatchInstruction(forPrepPC, object.Instruction(vm.MakeAsBx(vm.OP_FORPREP, baseReg, forPrepOffset)))

	// Patch FORLOOP: jump back to bodyStart
	forLoopOffset := bodyStart - forLoopPC - 1
	cg.PatchInstruction(forLoopPC, object.Instruction(vm.MakeAsBx(vm.OP_FORLOOP, baseReg, forLoopOffset)))

	// End scope for loop variable
	cg.endScope()

	// Restore stack top (free loop registers)
	cg.setStackTop(baseReg)
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

	// Generate iterator function call with ALL results (c=0)
	// This is critical: ipairs/pairs return 3 values (func, state, control)
	var funcReg int
	if callExpr, ok := funcExpr.(*parser.CallExpr); ok {
		// Generate function expression
		funcReg = cg.genExpr(callExpr.Func)

		// Generate arguments
		argCount := len(callExpr.Args)
		for i, arg := range callExpr.Args {
			argReg := cg.genExpr(arg)
			// Move argument to correct position if needed
			if argReg != funcReg+1+i {
				cg.EmitABC(vm.OP_MOVE, funcReg+1+i, argReg, 0)
				cg.freeRegister()
			}
		}

		// Emit CALL with c=0 (all results)
		// R(A) := R(A)(R(A+1), ..., R(A+C-1)) where C=0 means all available
		cg.EmitABC(vm.OP_CALL, funcReg, argCount+1, 0)

		// After CALL with c=0, results are in funcReg, funcReg+1, funcReg+2, ...
		// Set stack top to preserve all results
		cg.setStackTop(funcReg + 3)
		if cg.StackTop > cg.MaxStackSize {
			cg.MaxStackSize = cg.StackTop
		}
	} else {
		// Not a call expression, use regular genExpr
		funcReg = cg.genExpr(funcExpr)
	}

	// Generate arguments (state and initial value) from Exprs[1:]
	argRegs := make([]int, len(args))
	for i, arg := range args {
		argRegs[i] = cg.genExpr(arg)
	}

	// Move to consecutive registers at baseReg
	// If funcReg is already at baseReg, no move needed for the first value
	if funcReg != baseReg {
		cg.EmitABC(vm.OP_MOVE, baseReg, funcReg, 0)
		// Don't free funcReg - it may have multiple results
	}
	// For call expressions, results are already in funcReg, funcReg+1, funcReg+2
	// which should be baseReg, baseReg+1, baseReg+2 after the move above

	for i, argReg := range argRegs {
		if i < 2 { // FORGPREP expects up to 2 additional values
			cg.EmitABC(vm.OP_MOVE, baseReg+1+i, argReg, 0)
			cg.freeRegister()
		}
	}

	// Ensure stack top is at baseReg+3 for loop variable allocation
	cg.setStackTop(baseReg + 3)
	if cg.StackTop > cg.MaxStackSize {
		cg.MaxStackSize = cg.StackTop
	}

	// Add loop variables as locals — in their own scope so they don't leak
	numVars := len(stmt.Vars)
	cg.beginScope()
	for _, name := range stmt.Vars {
		reg := cg.allocRegister()
		cg.addLocal(name.Name, reg, false)
	}

	// FORGPREP R(A) sBx
	// Prepares the generic for loop
	forGPrepPC := cg.EmitAsBx(vm.OP_FORGPREP, baseReg, 0)

	// Begin scope for loop body - each iteration gets fresh locals
	cg.beginScope()

	// Generate body
	cg.genBlock(stmt.Body)

	// End scope for loop body - mark locals as inactive
	cg.endScope()

	// Close any upvalues captured by closures in the body.
	// This moves captured locals from the stack to the heap,
	// allowing the stack slots to be reused in the next iteration.
	// CLOSE all upvalues at or above first body register.
	cg.EmitABC(vm.OP_CLOSE, baseReg+3+numVars, 0, 0)

	// Reset stack top to just after loop variables for next iteration
	// This ensures temp registers are freed between iterations
	cg.setStackTop(baseReg + 3 + numVars)

	// FORGLOOP R(A) sBx
	// Executes the generic for loop
	forGLoopPC := cg.EmitAsBx(vm.OP_FORGLOOP, baseReg, 0)

	// Patch offsets
	// FORGPREP should jump to FORGLOOP
	forGPrepOffset := forGLoopPC - (forGPrepPC + 1)
	cg.PatchInstruction(forGPrepPC, object.Instruction(vm.MakeAsBx(vm.OP_FORGPREP, baseReg, forGPrepOffset)))

	// FORGLOOP should jump back to loop body (after FORGPREP)
	forGLoopOffset := (forGPrepPC + 1) - (forGLoopPC + 1)
	cg.PatchInstruction(forGLoopPC, object.Instruction(vm.MakeAsBx(vm.OP_FORGLOOP, baseReg, forGLoopOffset)))

	// End scope for loop variables
	cg.endScope()
}

// genReturn generates code for a return statement.
func (cg *CodeGenerator) genReturn(stmt *parser.ReturnStmt) {
	if len(stmt.Values) == 0 {
		// return with no values
		cg.emitReturn(0, 1) // A=0 (base), B=1 (0 results)
		return
	}

	// Check if the last expression is a call expression (for tail call optimization)
	lastIdx := len(stmt.Values) - 1
	_, lastIsCall := stmt.Values[lastIdx].(*parser.CallExpr)
	_, lastIsMethodCall := stmt.Values[lastIdx].(*parser.MethodCallExpr)
	isLastCall := lastIsCall || lastIsMethodCall

	// Generate return values
	baseReg := cg.StackTop

	// First, collect all source registers
	sources := make([]int, len(stmt.Values))
	for i, expr := range stmt.Values {
		// For the last expression, if it's a call, set ExpectedResults = 0 (all results)
		if i == lastIdx && isLastCall {
			cg.ExpectedResults = 0 // 0 = all results
		}
		reg := cg.genExpr(expr)
		sources[i] = reg
		// Reset ExpectedResults after generating the expression
		cg.ExpectedResults = -1
	}

	// Special case: single call expression returning all values
	if len(stmt.Values) == 1 && isLastCall {
		// The call results are already in the right place (starting at sources[0])
		// Return all results from the call
		cg.emitReturn(sources[0], 0)
		return
	}

	// Check if any destination overlaps with any source that would be overwritten
	hasOverlap := false
	for i := range sources {
		dest := baseReg + i
		for j := range sources {
			if i != j && dest == sources[j] {
				hasOverlap = true
				break
			}
		}
		if hasOverlap {
			break
		}
	}

	if hasOverlap {
		// Use temporary registers to avoid overlap issues
		temps := make([]int, len(sources))
		for i, src := range sources {
			temps[i] = cg.allocRegister()
			cg.EmitABC(vm.OP_MOVE, temps[i], src, 0)
		}
		for i, tmp := range temps {
			dest := baseReg + i
			if tmp != dest {
				cg.EmitABC(vm.OP_MOVE, dest, tmp, 0)
			}
		}
		for range temps {
			cg.freeRegister()
		}
	} else {
		// No overlap - check if we need forward or reverse order
		maxSource := 0
		for _, s := range sources {
			if s > maxSource {
				maxSource = s
			}
		}

		if maxSource >= baseReg {
			// Move in reverse order
			for i := len(stmt.Values) - 1; i >= 0; i-- {
				if sources[i] != baseReg+i {
					cg.EmitABC(vm.OP_MOVE, baseReg+i, sources[i], 0)
				}
			}
		} else {
			// Move in forward order (no overlap)
			for i := range stmt.Values {
				if sources[i] != baseReg+i {
					cg.EmitABC(vm.OP_MOVE, baseReg+i, sources[i], 0)
				}
			}
		}
	}

	// Free source registers
	for range sources {
		cg.freeRegister()
	}

	// RETURN R(A) B C
	// B = number of results + 1, 0 = all available
	// If the last expression is a call, use B=0 to return all results from the call
	if isLastCall {
		cg.emitReturn(baseReg, 0)
	} else {
		cg.emitReturn(baseReg, len(stmt.Values)+1)
	}
}

// genBreak generates code for a break statement.
func (cg *CodeGenerator) genBreak(stmt *parser.BreakStmt) {
	// Jump to end of innermost loop
	// This requires loop context tracking
	// Simplified: emit JMP with 0 offset (to be patched by loop)
	cg.EmitAsBx(vm.OP_JMP, 0, 0)
}


// genDo generates code for a do...end block.
// A do block creates a new scope for local variables.
func (cg *CodeGenerator) genDo(stmt *parser.DoStmt) {
	cg.beginScope()
	if stmt.Body != nil {
		cg.genBlock(stmt.Body)
	}
	cg.endScope()
}

// genExprStmt generates code for an expression statement.
// Typically used for function calls where the result is discarded.
func (cg *CodeGenerator) genExprStmt(stmt *parser.ExprStmt) {
	_ = cg.genExpr(stmt.Expr)
	cg.freeRegister()
}

// genFuncDef generates code for a function definition statement.
func (cg *CodeGenerator) genFuncDef(stmt *parser.FuncDefStmt) {
	if stmt.IsLocal && len(stmt.Name) > 0 {
		// local function name() ... end
		// Register local BEFORE generating nested body so self-reference works
		reg := cg.allocRegister()
		cg.addLocal(stmt.Name[0].Name, reg, false)

		// Create nested function prototype with parent link
		nestedGen := NewCodeGenerator()
		nestedGen.Parent = cg

		// Set up _ENV as upvalue[0] for the nested function
		// Use resolveUpvalue to correctly handle local _ENV in parent scope
		if _, ok := nestedGen.resolveUpvalue("_ENV"); !ok {
			// Fallback: inherit from parent's upvalue[0]
			nestedGen.Upvalues["_ENV"] = 0
			nestedGen.Prototype.Upvalues = append(nestedGen.Prototype.Upvalues, object.UpvalueDesc{
				Index:   0,     // Parent's upvalue[0] (_ENV)
				IsLocal: false, // Inherited from parent's upvalues
			})
		}

		if stmt.Func != nil {
			nestedGen.GenerateFunc(stmt.Func)
		} else {
			nestedGen.GenerateFunc(&parser.FuncExpr{
				Params:   stmt.Params,
				Body:     stmt.Body,
				IsVarArg: stmt.IsVarArg,
			})
		}

		nestedProto := nestedGen.Prototype

		// Add to parent's prototype table
		protoIdx := len(cg.Prototype.Prototypes)
		cg.Prototype.Prototypes = append(cg.Prototype.Prototypes, nestedProto)

		// Emit CLOSURE into the pre-allocated register
		cg.EmitABx(vm.OP_CLOSURE, reg, protoIdx)
	} else {
		// Non-local function: function name() ... end
		nestedGen := NewCodeGenerator()
		nestedGen.Parent = cg

		// Set up _ENV as upvalue[0] for the nested function
		// Use resolveUpvalue to correctly handle local _ENV in parent scope
		if _, ok := nestedGen.resolveUpvalue("_ENV"); !ok {
			// Fallback: inherit from parent's upvalue[0]
			nestedGen.Upvalues["_ENV"] = 0
			nestedGen.Prototype.Upvalues = append(nestedGen.Prototype.Upvalues, object.UpvalueDesc{
				Index:   0,
				IsLocal: false,
			})
		}

		if stmt.Func != nil {
			nestedGen.GenerateFunc(stmt.Func)
		} else {
			nestedGen.GenerateFunc(&parser.FuncExpr{
				Params:   stmt.Params,
				Body:     stmt.Body,
				IsVarArg: stmt.IsVarArg,
			})
		}

		nestedProto := nestedGen.Prototype

		protoIdx := len(cg.Prototype.Prototypes)
		cg.Prototype.Prototypes = append(cg.Prototype.Prototypes, nestedProto)

		if len(stmt.Name) > 0 {
			reg := cg.allocRegister()
			cg.EmitABx(vm.OP_CLOSURE, reg, protoIdx)
			// Use FullName for dotted function definitions (a.f), otherwise use Name[0]
			var nameExpr parser.Expr = stmt.Name[0]
			if stmt.FullName != nil {
				nameExpr = stmt.FullName
			}
			cg.assignToVar(nameExpr, reg)
			cg.freeRegister()
		}
	}
}

// genGlobal generates code for a global declaration statement.
// global [attr] ('*' | namelist) [= exprlist]
func (cg *CodeGenerator) genGlobal(stmt *parser.GlobalStmt) {
	// Track `global *` (wildcard) declarations for goto scope checking
	// When Names is nil or empty, it's the `global *` form
	if len(stmt.Names) == 0 {
		// Record the source line number as a `global *` declaration point
		cg.globalAlls = append(cg.globalAlls, stmt.Line())
	}
	// Note: `global *` doesn't generate bytecode, it's a declaration marker
	// The actual global variable assignments are handled separately
}

// genGoto generates code for a goto statement.
// goto label
func (cg *CodeGenerator) genGoto(stmt *parser.GotoStmt) {
	if cg.hasError() {
		return
	}
	
	labelName := stmt.Label
	currentPC := cg.GetCurrentPC()
	
	// Check if label is already defined (backward jump)
	if labelInfo, exists := cg.labels[labelName]; exists {
		// Backward jump: check that we're not jumping into an inner block
		// A goto can only jump to a label in the same or outer scope
		if labelInfo.BlockDepth > cg.blockDepth {
			cg.setError("label '%s' is inside an inner block", labelName)
			return
		}
		
		// Emit JMP directly to target
		// JMP uses sBx offset: PC = PC + 1 + sBx
		// We want to jump to targetPC, so: sBx = targetPC - (currentPC + 1)
		offset := labelInfo.PC - (currentPC + 1)
		cg.EmitAsBx(vm.OP_JMP, 0, offset)
	} else {
		// Forward jump: emit JMP with placeholder, will be patched when label is found
		jumpPC := cg.EmitAsBx(vm.OP_JMP, 0, 0) // Placeholder offset
		
		// Record this jump for later patching
		cg.forwardGotos[labelName] = append(cg.forwardGotos[labelName], GotoInfo{
			PC:            jumpPC,
			Label:         labelName,
			BlockDepth:    cg.blockDepth,
			Line:          stmt.Line(),
			NumLocals:     cg.countLocals(),
			NumGlobalAlls: len(cg.globalAlls),
			ActiveLocals:  cg.getActiveLocals(),
		})
	}
}

// genLabel generates code for a label statement.
// ::label::
func (cg *CodeGenerator) genLabel(stmt *parser.LabelStmt) {
	if cg.hasError() {
		return
	}
	
	labelName := stmt.Name
	currentPC := cg.GetCurrentPC()
	labelBlockDepth := cg.blockDepth
	
	// Check for duplicate labels
	// A label is visible if it's in the same or outer block (blockDepth <= current)
	// Labels in outer blocks are visible in inner blocks, so same-name labels in inner blocks are duplicates
	if existing, exists := cg.labels[labelName]; exists {
		// If the existing label is at the same or outer block depth, it's visible and this is a duplicate
		if existing.BlockDepth <= cg.blockDepth {
			cg.setError("duplicate label '%s'", labelName)
			return
		}
	}
	
	// Record the label position with current scope info
	cg.labels[labelName] = LabelInfo{
		PC:           currentPC,
		BlockDepth:   cg.blockDepth,
		Line:         stmt.Line(),
		ScopeLevel:   len(cg.Locals),
		AtBlockEnd:   false, // Will be set to true at genBlock end if this is the last statement
		PCAfterLabel: cg.GetCurrentPC(), // PC right after the label
	}
	
	// Patch any forward jumps to this label
	if jumps, exists := cg.forwardGotos[labelName]; exists {
		for _, gotoInfo := range jumps {
			// Check that the label is not in a deeper block than the goto
			// A goto can only jump to a label in the same or outer scope
			// If label's blockDepth > goto's blockDepth, the label is inside an inner block (invisible)
			if labelBlockDepth > gotoInfo.BlockDepth {
				cg.setError("label '%s' is inside an inner block", labelName)
				return
			}
			
			// Check if the goto jumps into the scope of a local variable
			// A goto may jump over a local declaration IF the label is at or past the end of the local's scope.
			// 
			// Get locals that are active at the label but were NOT active at the goto
			currentActiveLocals := cg.getActiveLocals()
			gotoActiveLocals := gotoInfo.ActiveLocals
			
			// Find new active locals (in scope at label but not at goto)
			for _, localName := range currentActiveLocals {
				wasActiveAtGoto := false
				for _, oldLocal := range gotoActiveLocals {
					if oldLocal == localName {
						wasActiveAtGoto = true
						break
					}
				}
				
				if !wasActiveAtGoto {
					// This local is new - check if the label is at the end of its scope
					localBlockDepth := cg.getLocalBlockDepth(localName)
					
					// If local's block depth > label's block depth, the local was in a deeper block
					// that has since closed - the label is past the end of the local's scope
					if localBlockDepth > labelBlockDepth {
						// Local was in a deeper block that has closed - label is past its scope
						// This is allowed
						continue
					}
					
					// If local's block depth == label's block depth, need to check if label is at block end
					// This is determined at endScope time, so defer the check
					if localBlockDepth == labelBlockDepth {
						// Same block - defer check until endScope
						cg.pendingScopeChecks = append(cg.pendingScopeChecks, PendingScopeCheck{
							LabelName:        labelName,
							GotoInfo:         gotoInfo,
							LocalName:        localName,
							LocalBlockDepth:  localBlockDepth,
						})
						continue
					}
					
					// The local is in an outer block that's still open - jumping into its scope
					cg.setError("jump into the scope of '%s'", localName)
					return
				}
			}
			
			// Check if the goto jumps over a `global *` declaration
			// Compare the count of `global *` declarations at goto time vs now
			currentGlobalAlls := len(cg.globalAlls)
			if currentGlobalAlls > gotoInfo.NumGlobalAlls {
				cg.setError("scope of '*'")
				return
			}
			
			// Patch the jump: sBx = targetPC - (jumpPC + 1)
			offset := currentPC - (gotoInfo.PC + 1)
			cg.PatchInstruction(gotoInfo.PC, object.Instruction(vm.MakeAsBx(vm.OP_JMP, 0, offset)))
		}
		// Remove the patched jumps
		delete(cg.forwardGotos, labelName)
	}
	
	// Labels don't generate any code themselves - they're just markers
}
