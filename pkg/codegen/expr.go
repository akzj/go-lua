package codegen

import (
	"github.com/akzj/go-lua/pkg/object"
	"github.com/akzj/go-lua/pkg/parser"
	"github.com/akzj/go-lua/pkg/vm"
)

// genExpr generates code for an expression and returns the result register.
//
// The result of the expression is left in the returned register.
// Callers should free the register when done.
//
// Parameters:
//   - expr: The expression AST node
//
// Returns:
//   - int: The register containing the result
func (cg *CodeGenerator) genExpr(expr parser.Expr) int {
	if expr == nil {
		return 0
	}

	// Track source line for error messages
	if line := expr.Line(); line > 0 {
		cg.currentLine = line
	}

	switch e := expr.(type) {
	case *parser.NilExpr:
		return cg.genNil()

	case *parser.BooleanExpr:
		return cg.genBoolean(e.Value)

	case *parser.NumberExpr:
		return cg.genNumber(e)

	case *parser.StringExpr:
		return cg.genString(e.Value)

	case *parser.VarExpr:
		return cg.genVar(e)

	case *parser.IndexExpr:
		return cg.genIndex(e)

	case *parser.FieldExpr:
		return cg.genField(e)

	case *parser.CallExpr:
		return cg.genCall(e)

	case *parser.MethodCallExpr:
		return cg.genMethodCall(e)

	case *parser.BinOpExpr:
		return cg.genBinOp(e)

	case *parser.UnOpExpr:
		return cg.genUnOp(e)

	case *parser.TableExpr:
		return cg.genTable(e)

	case *parser.FuncExpr:
		return cg.genFunc(e)

	case *parser.DotsExpr:
		return cg.genDots()

	case *parser.ParenExpr:
		return cg.genExpr(e.Expr)

	default:
		// Unknown expression type, return nil
		return cg.genNil()
	}
}

// genNil generates code for a nil literal.
func (cg *CodeGenerator) genNil() int {
	reg := cg.allocRegister()
	cg.EmitABC(vm.OP_LOADNIL, reg, reg, 0)
	return reg
}

// genBoolean generates code for a boolean literal.
func (cg *CodeGenerator) genBoolean(value bool) int {
	reg := cg.allocRegister()
	cg.EmitABC(vm.OP_LOADBOOL, reg, boolToInt(value), 0)
	return reg
}

// genNumber generates code for a number literal.
func (cg *CodeGenerator) genNumber(expr *parser.NumberExpr) int {
	reg := cg.allocRegister()

	if expr.IsInt && expr.Int >= -128 && expr.Int <= 127 {
		// Use LOADI for small integers (sBx format)
		cg.EmitAsBx(vm.OP_LOADI, reg, int(expr.Int))
	} else if expr.IsInt {
		// Use LOADK for large integers - preserve IsInt flag
		value := object.NewInteger(expr.Int)
		idx := cg.addOrGetConstant(*value)
		if idx <= 255 {
			cg.EmitABx(vm.OP_LOADK, reg, idx)
		} else {
			cg.EmitABx(vm.OP_LOADKX, reg, 0)
			cg.EmitAx(vm.OP_EXTRAARG, idx)
		}
	} else {
		// Use LOADK for floats
		value := object.NewNumber(expr.Value)
		idx := cg.addOrGetConstant(*value)
		if idx <= 255 {
			cg.EmitABx(vm.OP_LOADK, reg, idx)
		} else {
			cg.EmitABx(vm.OP_LOADKX, reg, 0)
			cg.EmitAx(vm.OP_EXTRAARG, idx)
		}
	}

	return reg
}

// genString generates code for a string literal.
func (cg *CodeGenerator) genString(value string) int {
	reg := cg.allocRegister()
	tvalue := object.NewString(value)
	idx := cg.addOrGetConstant(*tvalue)
	if idx <= 255 {
		cg.EmitABx(vm.OP_LOADK, reg, idx)
	} else {
		cg.EmitABx(vm.OP_LOADKX, reg, 0)
		cg.EmitAx(vm.OP_EXTRAARG, idx)
	}
	return reg
}

// genVar generates code for a variable reference.
func (cg *CodeGenerator) genVar(expr *parser.VarExpr) int {
	reg := cg.allocRegister()

	// Check if it's a local variable
	if idx, ok := cg.getLocal(expr.Name); ok {
		// Use MOVE to copy from local
		cg.EmitABC(vm.OP_MOVE, reg, idx, 0)
	} else if upIdx, ok := cg.getUpvalue(expr.Name); ok {
		// Upvalue access
		cg.EmitABC(vm.OP_GETUPVAL, reg, upIdx, 0)
	} else if upIdx, ok := cg.resolveUpvalue(expr.Name); ok {
		cg.EmitABC(vm.OP_GETUPVAL, reg, upIdx, 0)
	} else {
		// Global: GETTABUP R(A), 0, K(C) — R(A) := UpValue[0][K(C)]
		// C is constant index directly (K[C] format), not RK mode
		nameIdx := cg.addOrGetConstant(*object.NewString(expr.Name))
		cg.EmitABC(vm.OP_GETTABUP, reg, 0, nameIdx)
	}

	return reg
}

// genIndex generates code for table[index] access.
func (cg *CodeGenerator) genIndex(expr *parser.IndexExpr) int {
	// Generate table expression
	tableReg := cg.genExpr(expr.Table)

	// Generate index expression
	indexReg := cg.genExpr(expr.Index)

	// Free both operands, then allocate result in tableReg's slot.
	// For GETTABLE R(A) := R(B)[R(C)], VM reads B and C before writing A.
	cg.freeRegisters(2)
	resultReg := cg.allocRegister()

	// Emit GETTABLE: R(A) := R(B)[R(C)]
	cg.EmitABC(vm.OP_GETTABLE, resultReg, tableReg, indexReg)

	return resultReg
}

// genField generates code for table.field access.
func (cg *CodeGenerator) genField(expr *parser.FieldExpr) int {
	// Generate table expression
	tableReg := cg.genExpr(expr.Table)

	// Free table operand, allocate result in its slot.
	// For GETFIELD R(A) := R(B)[K(C)], VM reads B before writing A, so A == B is safe.
	cg.freeRegister()
	resultReg := cg.allocRegister()

	// Load field name as constant
	fieldIdx := cg.addOrGetConstant(*object.NewString(expr.Field))

	// Emit GETFIELD: R(A) := R(B)[K(C)]
	if fieldIdx <= 255 {
		cg.EmitABC(vm.OP_GETFIELD, resultReg, tableReg, fieldIdx)
	} else {
		// For large indices, use GETTABLE with loaded key
		keyReg := cg.allocRegister()
		cg.EmitABx(vm.OP_LOADK, keyReg, fieldIdx)
		cg.EmitABC(vm.OP_GETTABLE, resultReg, tableReg, keyReg)
		cg.freeRegister() // keyReg
	}

	return resultReg
}

// genCall generates code for a function call.
func (cg *CodeGenerator) genCall(expr *parser.CallExpr) int {
	// Generate function expression
	funcReg := cg.genExpr(expr.Func)

	// Check if the last argument is a call expression (for multi-return propagation)
	lastArgIsCall := false
	lastArgIsDots := false
	if len(expr.Args) > 0 {
		lastArg := expr.Args[len(expr.Args)-1]
		_, lastArgIsCall = lastArg.(*parser.CallExpr)
		if !lastArgIsCall {
			_, lastArgIsCall = lastArg.(*parser.MethodCallExpr)
		}
		if !lastArgIsCall {
			_, lastArgIsDots = lastArg.(*parser.DotsExpr)
		}
	}

	// Generate arguments
	argCount := len(expr.Args)
	for i, arg := range expr.Args {
		isLast := (i == len(expr.Args)-1)
		if isLast && lastArgIsCall {
			// Last argument is a call - generate with ExpectedResults=0 (all results)
			// The inner call's result will be at funcReg+1+i
			argFuncReg := funcReg + 1 + i
			// Handle both CallExpr and MethodCallExpr
			if callExpr, ok := arg.(*parser.CallExpr); ok {
				// Generate the inner call's function at the correct position
				innerFuncReg := cg.genExpr(callExpr.Func)
				if innerFuncReg != argFuncReg {
					cg.EmitABC(vm.OP_MOVE, argFuncReg, innerFuncReg, 0)
					cg.freeRegister()
				}
				// Generate inner call's arguments at argFuncReg+1, argFuncReg+2, ...
				for j, innerArg := range callExpr.Args {
					innerArgReg := cg.genExpr(innerArg)
					targetReg := argFuncReg + 1 + j
					if innerArgReg != targetReg {
						cg.EmitABC(vm.OP_MOVE, targetReg, innerArgReg, 0)
						cg.freeRegister()
					}
				}
				// Emit inner CALL with C=0 (all results), result at argFuncReg
				cg.EmitABC(vm.OP_CALL, argFuncReg, len(callExpr.Args)+1, 0)
			} else if methodExpr, ok := arg.(*parser.MethodCallExpr); ok {
				// Method call: obj:method(args)
				// Generate object at argFuncReg (for SELF)
				objReg := cg.genExpr(methodExpr.Object)
				if objReg != argFuncReg {
					cg.EmitABC(vm.OP_MOVE, argFuncReg, objReg, 0)
					cg.freeRegister()
				}
				// Get method constant
				methodIdx := cg.addOrGetConstant(*object.NewString(methodExpr.Method))
				// Use SELF: R(A+1) := R(B); R(A) := R(B)[RK(C)]
				// This loads the method and puts object in R(A+1)
				if methodIdx <= 255 {
					cg.EmitABC(vm.OP_SELF, argFuncReg, argFuncReg, methodIdx+256)
				} else {
					keyReg := cg.allocRegister()
					cg.EmitABx(vm.OP_LOADK, keyReg, methodIdx)
					cg.EmitABC(vm.OP_MOVE, argFuncReg+1, argFuncReg, 0)
					cg.EmitABC(vm.OP_GETTABLE, argFuncReg, argFuncReg, keyReg)
					cg.freeRegister()
				}
				// Now argFuncReg contains method, argFuncReg+1 contains self
				cg.setStackTop(argFuncReg + 2)
				// Generate arguments at argFuncReg+2, argFuncReg+3, ...
				for j, innerArg := range methodExpr.Args {
					innerArgReg := cg.genExpr(innerArg)
					targetReg := argFuncReg + 2 + j
					if innerArgReg != targetReg {
						cg.EmitABC(vm.OP_MOVE, targetReg, innerArgReg, 0)
						cg.freeRegister()
					}
				}
				// Emit inner CALL with C=0 (all results), arg count includes self
				cg.EmitABC(vm.OP_CALL, argFuncReg, len(methodExpr.Args)+2, 0)
			}
			// For C=0, the VM sets StackTop; don't adjust here
		} else if isLast && lastArgIsDots {
			// Last argument is ... (vararg) - emit VARARG with C=0 (all varargs)
			targetReg := funcReg + 1 + i
			cg.EmitABC(vm.OP_VARARG, targetReg, 1, 0) // C=0 means all varargs
		} else {
			argReg := cg.genExpr(arg)
			targetReg := funcReg + 1 + i
			// Move argument to correct position if needed
			if argReg != targetReg {
				cg.EmitABC(vm.OP_MOVE, targetReg, argReg, 0)
				cg.freeRegister()
			}
			// Ensure StackTop accounts for this argument
			// This prevents the next argument's expression from overwriting previous args or the function
			if cg.StackTop <= targetReg {
				cg.setStackTop(targetReg + 1)
			}
		}
	}

	// Emit CALL: R(A) := R(A)(R(A+1), ..., R(A+C-1))
	// B = argCount + 1 (including function), 0 = vararg (when last arg is multi-return call)
	// C = nresults + 1, or 0 = multiple results
	resultReg := funcReg
	cField := 2 // default: 1 result (c = nresults + 1)
	if cg.ExpectedResults > 0 {
		cField = cg.ExpectedResults + 1
	} else if cg.ExpectedResults == 0 {
		// ExpectedResults == 0 means "all results" (for return f() or similar)
		cField = 0
	}
	// ExpectedResults == -1 means "default" (1 result, cField = 2)
	
	bField := argCount + 1
	if lastArgIsCall || lastArgIsDots {
		bField = 0 // Variable number of args from stack top
	}
	cg.EmitABC(vm.OP_CALL, resultReg, bField, cField)

	// After CALL, result is in funcReg
	// Set stack top based on expected results
	// For cField == 0 (all results), the VM handles stack top
	if cField > 0 {
		// Fixed number of results
		cg.setStackTop(funcReg + cField - 1)
	}
	// For cField == 0, don't adjust stack top - the VM will handle it

	return resultReg
}

// genMethodCall generates code for obj:method(args) call.
func (cg *CodeGenerator) genMethodCall(expr *parser.MethodCallExpr) int {
	// Generate object expression
	objReg := cg.genExpr(expr.Object)

	// Get the method
	methodIdx := cg.addOrGetConstant(*object.NewString(expr.Method))

	// Allocate result register for the function
	funcReg := cg.allocRegister()

	// Use SELF: R(A+1) := R(B); R(A) := R(B)[RK(C)]
	// This loads the method and puts object in R(A+1)
	// RK encoding: if C >= 256, it's a constant index (C - 256)
	if methodIdx <= 255 {
		cg.EmitABC(vm.OP_SELF, funcReg, objReg, methodIdx+256) // +256 for RK constant encoding
	} else {
		// For large indices, load key first
		keyReg := cg.allocRegister()
		cg.EmitABx(vm.OP_LOADK, keyReg, methodIdx)
		cg.EmitABC(vm.OP_MOVE, funcReg+1, objReg, 0)
		cg.EmitABC(vm.OP_GETTABLE, funcReg, objReg, keyReg)
		cg.freeRegister() // keyReg
	}

	// Now funcReg contains the method, funcReg+1 contains the object (self)
	// IMPORTANT: Update StackTop to reserve space for self at funcReg+1
	// Otherwise genExpr for arguments might allocate funcReg+1 and overwrite self
	cg.setStackTop(funcReg + 2)
	if cg.StackTop > cg.MaxStackSize {
		cg.MaxStackSize = cg.StackTop
	}

	// Check if the last argument is a call expression (for multi-return propagation)
	lastArgIsCall := false
	lastArgIsDots := false
	if len(expr.Args) > 0 {
		lastArg := expr.Args[len(expr.Args)-1]
		_, lastArgIsCall = lastArg.(*parser.CallExpr)
		if !lastArgIsCall {
			_, lastArgIsCall = lastArg.(*parser.MethodCallExpr)
		}
		if !lastArgIsCall {
			_, lastArgIsDots = lastArg.(*parser.DotsExpr)
		}
	}

	// Generate arguments (starting at funcReg+2)
	argCount := len(expr.Args)
	for i, arg := range expr.Args {
		isLast := (i == len(expr.Args)-1)
		if isLast && lastArgIsCall {
			// Last argument is a call - generate with ExpectedResults=0 (all results)
			// The inner call's result will be at funcReg+2+i
			argFuncReg := funcReg + 2 + i
			// Handle both CallExpr and MethodCallExpr
			if callExpr, ok := arg.(*parser.CallExpr); ok {
				// Generate the inner call's function at the correct position
				innerFuncReg := cg.genExpr(callExpr.Func)
				if innerFuncReg != argFuncReg {
					cg.EmitABC(vm.OP_MOVE, argFuncReg, innerFuncReg, 0)
					cg.freeRegister()
				}
				// Generate inner call's arguments at argFuncReg+1, argFuncReg+2, ...
				for j, innerArg := range callExpr.Args {
					innerArgReg := cg.genExpr(innerArg)
					targetReg := argFuncReg + 1 + j
					if innerArgReg != targetReg {
						cg.EmitABC(vm.OP_MOVE, targetReg, innerArgReg, 0)
						cg.freeRegister()
					}
				}
				// Emit inner CALL with C=0 (all results), result at argFuncReg
				cg.EmitABC(vm.OP_CALL, argFuncReg, len(callExpr.Args)+1, 0)
			} else if methodExpr, ok := arg.(*parser.MethodCallExpr); ok {
				// Method call: obj:method(args)
				// Generate object at argFuncReg (for SELF)
				objReg := cg.genExpr(methodExpr.Object)
				if objReg != argFuncReg {
					cg.EmitABC(vm.OP_MOVE, argFuncReg, objReg, 0)
					cg.freeRegister()
				}
				// Get method constant
				methodIdx := cg.addOrGetConstant(*object.NewString(methodExpr.Method))
				// Use SELF: R(A+1) := R(B); R(A) := R(B)[RK(C)]
				if methodIdx <= 255 {
					cg.EmitABC(vm.OP_SELF, argFuncReg, argFuncReg, methodIdx+256)
				} else {
					keyReg := cg.allocRegister()
					cg.EmitABx(vm.OP_LOADK, keyReg, methodIdx)
					cg.EmitABC(vm.OP_MOVE, argFuncReg+1, argFuncReg, 0)
					cg.EmitABC(vm.OP_GETTABLE, argFuncReg, argFuncReg, keyReg)
					cg.freeRegister()
				}
				// Now argFuncReg contains method, argFuncReg+1 contains self
				cg.setStackTop(argFuncReg + 2)
				// Generate arguments at argFuncReg+2, argFuncReg+3, ...
				for j, innerArg := range methodExpr.Args {
					innerArgReg := cg.genExpr(innerArg)
					targetReg := argFuncReg + 2 + j
					if innerArgReg != targetReg {
						cg.EmitABC(vm.OP_MOVE, targetReg, innerArgReg, 0)
						cg.freeRegister()
					}
				}
				// Emit inner CALL with C=0 (all results), arg count includes self
				cg.EmitABC(vm.OP_CALL, argFuncReg, len(methodExpr.Args)+2, 0)
			}
			// For C=0, the VM sets StackTop; don't adjust here
		} else if isLast && lastArgIsDots {
			// Last argument is ... (vararg) - emit VARARG with C=0 (all varargs)
			targetReg := funcReg + 2 + i
			cg.EmitABC(vm.OP_VARARG, targetReg, 1, 0) // C=0 means all varargs
		} else {
			argReg := cg.genExpr(arg)
			targetReg := funcReg + 2 + i
			if argReg != targetReg {
				cg.EmitABC(vm.OP_MOVE, targetReg, argReg, 0)
				cg.freeRegister()
			}
		}
	}

	// Emit CALL
	cField := 2 // default: 1 result (c = nresults + 1)
	if cg.ExpectedResults > 0 {
		cField = cg.ExpectedResults + 1
	} else if cg.ExpectedResults == 0 {
		// ExpectedResults == 0 means "all results" (for return f() or similar)
		cField = 0
	}
	// ExpectedResults == -1 means "default" (1 result, cField = 2)
	
	bField := argCount + 2 // +2 for self
	if lastArgIsCall || lastArgIsDots {
		bField = 0 // Variable number of args from stack top
	}
	cg.EmitABC(vm.OP_CALL, funcReg, bField, cField)

	// Set stack top based on expected results
	// For cField == 0 (all results), the VM handles stack top
	if cField > 0 {
		// Fixed number of results
		cg.setStackTop(funcReg + cField - 1)
	}
	// For cField == 0, don't adjust stack top - the VM will handle it

	// NOTE: Do NOT free object register here - the result is at funcReg,
	// and we need StackTop to be funcReg + 1 so next allocation doesn't overwrite result.
	// The object was copied to funcReg+1 by SELF, and CALL overwrites that area.

	return funcReg
}

// genBinOp generates code for a binary operation.
func (cg *CodeGenerator) genBinOp(expr *parser.BinOpExpr) int {
	switch expr.Op {
	case "and":
		return cg.genAnd(expr)
	case "or":
		return cg.genOr(expr)
	case "..":
		return cg.genConcat(expr)
	default:
		return cg.genArithmetic(expr)
	}
}

// genAnd generates code for short-circuit AND.
// Result is first operand if false, second operand otherwise.
func (cg *CodeGenerator) genAnd(expr *parser.BinOpExpr) int {
	// Generate left operand
	leftReg := cg.genExpr(expr.Left)

	// Test if left is false
	// If false, jump to end (result is left)
	// If true, continue to evaluate right
	cg.EmitABC(vm.OP_TEST, leftReg, 0, 0) // if not (R(A) ~= 0) then pc++
	jumpInstr := cg.EmitAsBx(vm.OP_JMP, 0, 0)        // forward jump placeholder

	// Generate right operand
	rightReg := cg.genExpr(expr.Right)

	// Move right to left's register if different
	if rightReg != leftReg {
		cg.EmitABC(vm.OP_MOVE, leftReg, rightReg, 0)
		cg.freeRegister()
	}

	// Patch jump to skip right evaluation
	cg.patchJump(jumpInstr, cg.GetCurrentPC())

	return leftReg
}

// genOr generates code for short-circuit OR.
// Result is first operand if true, second operand otherwise.
func (cg *CodeGenerator) genOr(expr *parser.BinOpExpr) int {
	// Generate left operand
	leftReg := cg.genExpr(expr.Left)

	// Test if left is true
	// If true, jump to end (result is left)
	// If false, continue to evaluate right
	// OP_TEST with c=1: skip next instruction if value is falsy
	cg.EmitABC(vm.OP_TEST, leftReg, 0, 1)
	jumpInstr := cg.EmitAsBx(vm.OP_JMP, 0, 0) // forward jump placeholder

	// Generate right operand
	rightReg := cg.genExpr(expr.Right)

	// Move right to left's register if different
	if rightReg != leftReg {
		cg.EmitABC(vm.OP_MOVE, leftReg, rightReg, 0)
		cg.freeRegister()
	}

	// Patch jump to skip right evaluation
	cg.patchJump(jumpInstr, cg.GetCurrentPC())

	return leftReg
}

// collectConcatOperands recursively collects all operands in a .. chain.
// This flattens left-associative concatenation expressions.
func (cg *CodeGenerator) collectConcatOperands(expr parser.Expr) []parser.Expr {
	if binOp, ok := expr.(*parser.BinOpExpr); ok && binOp.Op == ".." {
		left := cg.collectConcatOperands(binOp.Left)
		right := cg.collectConcatOperands(binOp.Right)
		return append(left, right...)
	}
	return []parser.Expr{expr}
}

// genConcat generates code for concatenation operator.
// It flattens the .. chain and emits a single CONCAT instruction.
// Lua 5.4 CONCAT semantics: R(A) := R(B) .. R(B+1) .. ... .. R(C)
func (cg *CodeGenerator) genConcat(expr *parser.BinOpExpr) int {
	// Collect all operands in the .. chain
	operands := cg.collectConcatOperands(expr)

	// Remember where we start so we can free registers later
	startStackTop := cg.StackTop

	// Evaluate each operand into consecutive registers
	for i, operand := range operands {
		reg := cg.genExpr(operand)
		expectedReg := startStackTop + i
		// If the result is not in the expected position, move it
		if reg != expectedReg {
			cg.EmitABC(vm.OP_MOVE, expectedReg, reg, 0)
		}
		// Ensure stack top is at the next position for the next operand
		if cg.StackTop < expectedReg+1 {
			cg.StackTop = expectedReg + 1
			if cg.StackTop > cg.MaxStackSize {
				cg.MaxStackSize = cg.StackTop
			}
		}
	}

	// Emit single CONCAT instruction
	// CONCAT A B C: R(A) := R(B) .. R(B+1) .. ... .. R(C)
	startReg := startStackTop
	endReg := startStackTop + len(operands) - 1
	// Result goes into startReg (reuses first operand's slot).
	// VM reads B..C before writing A, so A == B is safe.
	cg.EmitABC(vm.OP_CONCAT, startReg, startReg, endReg)

	// Free all operand registers except the result
	cg.setStackTop(startStackTop + 1)

	return startReg
}

// genArithmetic generates code for arithmetic operations.
func (cg *CodeGenerator) genArithmetic(expr *parser.BinOpExpr) int {
	// Generate left operand
	leftReg := cg.genExpr(expr.Left)

	// Generate right operand
	rightReg := cg.genExpr(expr.Right)

	// Determine opcode
	var op vm.Opcode
	switch expr.Op {
	case "+":
		op = vm.OP_ADD
	case "-":
		op = vm.OP_SUB
	case "*":
		op = vm.OP_MUL
	case "/":
		op = vm.OP_DIV
	case "%":
		op = vm.OP_MOD
	case "^":
		op = vm.OP_POW
	case "//":
		op = vm.OP_IDIV
	case "&":
		op = vm.OP_BAND
	case "|":
		op = vm.OP_BOR
	case "~":
		op = vm.OP_BXOR
	case "<<":
		op = vm.OP_SHL
	case ">>":
		op = vm.OP_SHR
	case "==":
		op = vm.OP_EQ
		// Comparisons: free operands, allocate result, emit
		cg.freeRegisters(2)
		resultReg := cg.allocRegister()
		cg.EmitABC(op, resultReg, leftReg, rightReg)
		return resultReg
	case "~=":
		// Not equal: compare and then NOT the result
		cg.freeRegisters(2)
		resultReg := cg.allocRegister()
		cg.EmitABC(vm.OP_EQ, resultReg, leftReg, rightReg)
		cg.EmitABC(vm.OP_NOT, resultReg, resultReg, 0)
		return resultReg
	case "<":
		op = vm.OP_LT
		cg.freeRegisters(2)
		resultReg := cg.allocRegister()
		cg.EmitABC(op, resultReg, leftReg, rightReg)
		return resultReg
	case ">":
		// Greater than: swap operands for <
		cg.freeRegisters(2)
		resultReg := cg.allocRegister()
		cg.EmitABC(vm.OP_LT, resultReg, rightReg, leftReg)
		return resultReg
	case "<=":
		op = vm.OP_LE
		cg.freeRegisters(2)
		resultReg := cg.allocRegister()
		cg.EmitABC(op, resultReg, leftReg, rightReg)
		return resultReg
	case ">=":
		// Greater or equal: swap operands for <=
		cg.freeRegisters(2)
		resultReg := cg.allocRegister()
		cg.EmitABC(vm.OP_LE, resultReg, rightReg, leftReg)
		return resultReg
	default:
		// Unknown operator, default to ADD
		op = vm.OP_ADD
	}

	// Free both operand registers, then allocate result.
	// Result reuses leftReg's slot. For R(A) := R(B) op R(C),
	// the VM reads B and C before writing A, so A == B is safe.
	cg.freeRegisters(2)
	resultReg := cg.allocRegister()

	// Emit arithmetic instruction: R(A) := R(B) op R(C)
	cg.EmitABC(op, resultReg, leftReg, rightReg)

	return resultReg
}

// genUnOp generates code for a unary operation.
func (cg *CodeGenerator) genUnOp(expr *parser.UnOpExpr) int {
	// Generate operand
	operandReg := cg.genExpr(expr.Expr)

	// Determine opcode
	var op vm.Opcode
	switch expr.Op {
	case "-":
		op = vm.OP_UNM
	case "not":
		op = vm.OP_NOT
	case "#":
		op = vm.OP_LEN
	case "~":
		op = vm.OP_BNOT
	default:
		op = vm.OP_UNM
	}

	// Free operand and allocate result — this reuses the operand's register slot.
	// For unary ops R(A) := op R(B), A == B is safe because VM reads B before writing A.
	cg.freeRegister()
	resultReg := cg.allocRegister()

	// Emit unary instruction: R(A) := op R(B)
	cg.EmitABC(op, resultReg, operandReg, 0)

	return resultReg
}

// genTable generates code for a table constructor.
func (cg *CodeGenerator) genTable(expr *parser.TableExpr) int {
	// Allocate result register
	resultReg := cg.allocRegister()

	// Create empty table
	// NEWTABLE R(A) sizearray=sizeB sizehash=sizeC
	// For simplicity, use 0, 0 and let VM grow as needed
	cg.EmitABC(vm.OP_NEWTABLE, resultReg, 0, 0)

	// Track array index for sequential entries
	arrayIndex := 1

	// Process entries
	for i, entry := range expr.Entries {
		switch entry.Kind {
		case parser.TableEntryValue:
			// Array-style entry: value
			// Check if this is a call expression that's the last entry
			isLast := (i == len(expr.Entries)-1)
			callExpr, isCall := entry.Value.(*parser.CallExpr)
			methodExpr, isMethodCall := entry.Value.(*parser.MethodCallExpr)

			if isLast && (isCall || isMethodCall) {
				// Last entry is a call - capture all return values using SETLIST
				// We need to ensure the function is called at resultReg+1 so that
				// results are at resultReg+1, resultReg+2, etc. for SETLIST

				// Ensure stack has space for function at resultReg+1
				cg.setStackTop(resultReg + 2)

				if isCall {
					// Generate function expression
					funcReg := cg.genExpr(callExpr.Func)

					// Move function to resultReg+1 if needed
					if funcReg != resultReg+1 {
						cg.EmitABC(vm.OP_MOVE, resultReg+1, funcReg, 0)
						cg.freeRegister()
					}

					// Generate arguments at resultReg+2...
					for j, arg := range callExpr.Args {
						argReg := cg.genExpr(arg)
						targetReg := resultReg + 2 + j
						if argReg != targetReg {
							cg.EmitABC(vm.OP_MOVE, targetReg, argReg, 0)
							cg.freeRegister()
						}
					}

					// Emit CALL with C=0 (all results)
					cg.EmitABC(vm.OP_CALL, resultReg+1, len(callExpr.Args)+1, 0)

				} else { // isMethodCall
					// Generate object expression
					objReg := cg.genExpr(methodExpr.Object)

					// Move object to resultReg+1 (for SELF)
					if objReg != resultReg+1 {
						cg.EmitABC(vm.OP_MOVE, resultReg+1, objReg, 0)
						cg.freeRegister()
					}

					// Get method constant
					methodIdx := cg.addOrGetConstant(*object.NewString(methodExpr.Method))

					// Use SELF: R(A+1) := R(B); R(A) := R(B)[RK(C)]
					// This loads the method and puts object in R(A+1)
					if methodIdx <= 255 {
						cg.EmitABC(vm.OP_SELF, resultReg+1, resultReg+1, methodIdx+256) // +256 for RK constant encoding
					} else {
						// For large indices, load key first
						keyReg := cg.allocRegister()
						cg.EmitABx(vm.OP_LOADK, keyReg, methodIdx)
						cg.EmitABC(vm.OP_MOVE, resultReg+2, resultReg+1, 0)
						cg.EmitABC(vm.OP_GETTABLE, resultReg+1, resultReg+1, keyReg)
						cg.freeRegister()
					}

					// Now resultReg+1 contains the method, resultReg+2 contains self
					// Generate arguments at resultReg+3...
					for j, arg := range methodExpr.Args {
						argReg := cg.genExpr(arg)
						targetReg := resultReg + 3 + j
						if argReg != targetReg {
							cg.EmitABC(vm.OP_MOVE, targetReg, argReg, 0)
							cg.freeRegister()
						}
					}

					// Emit CALL with C=0 (all results), arg count includes self
					cg.EmitABC(vm.OP_CALL, resultReg+1, len(methodExpr.Args)+2, 0)
				}

				// SETLIST: R(resultReg)[arrayIndex...] = R(resultReg+1...StackTop)
				// C field is the starting index - 1 (offset)
				cg.EmitABC(vm.OP_SETLIST, resultReg, 0, arrayIndex-1)
				// arrayIndex update not needed since this is the last entry
			} else {
				// Not the last entry, or not a call - just add single value
				valueReg := cg.genExpr(entry.Value)
				// SETI R(A)[C] := RK(B) — B=value, C=integer index
				cg.EmitABC(vm.OP_SETI, resultReg, valueReg, arrayIndex)
				cg.freeRegister()
				arrayIndex++
			}

		case parser.TableEntryField:
			// Field entry: key = value
			var keyIdx int
			switch k := entry.Key.(type) {
			case *parser.StringExpr:
				keyIdx = cg.addOrGetConstant(*object.NewString(k.Value))
			case *parser.VarExpr:
				keyIdx = cg.addOrGetConstant(*object.NewString(k.Name))
			default:
				// Fallback for other expression types - treat as generic string
				keyIdx = cg.addOrGetConstant(object.TValue{Type: object.TypeString, Value: object.Value{Str: "field"}})
			}
			valueReg := cg.genExpr(entry.Value)

			if keyIdx <= 255 {
				cg.EmitABC(vm.OP_SETFIELD, resultReg, keyIdx, valueReg)
			} else {
				keyReg := cg.allocRegister()
				cg.EmitABx(vm.OP_LOADK, keyReg, keyIdx)
				cg.EmitABC(vm.OP_SETTABLE, resultReg, keyReg, valueReg)
				cg.freeRegister()
			}
			cg.freeRegister()

		case parser.TableEntryIndex:
			// Index entry: [key] = value
			keyReg := cg.genExpr(entry.Key)
			valueReg := cg.genExpr(entry.Value)
			cg.EmitABC(vm.OP_SETTABLE, resultReg, keyReg, valueReg)
			cg.freeRegisters(2)
		}
	}

	return resultReg
}

// genFunc generates code for an anonymous function.
func (cg *CodeGenerator) genFunc(expr *parser.FuncExpr) int {
	// Create a new code generator for the nested function
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

	// Generate nested function
	nestedProto := nestedGen.GenerateFunc(&parser.FuncExpr{
		Params:   expr.Params,
		Body:     expr.Body,
		IsVarArg: expr.IsVarArg,
	})

	// Add nested prototype to parent
	protoIdx := len(cg.Prototype.Prototypes)
	cg.Prototype.Prototypes = append(cg.Prototype.Prototypes, nestedProto)

	// Allocate result register
	resultReg := cg.allocRegister()

	// Emit CLOSURE: R(A) := closure(KPROTO[Bx])
	cg.EmitABx(vm.OP_CLOSURE, resultReg, protoIdx)

	// TODO: Handle upvalues (OPEN_UPVAL instructions)
	// For now, simplified implementation

	return resultReg
}

// genDots generates code for vararg (...).
func (cg *CodeGenerator) genDots() int {
	reg := cg.allocRegister()
	// VARARG R(A) := R(A+1), ..., R(A+C-1)
	// C = numResults + 1, or C=0 for all results
	
	cField := 2 // default: 1 result (c = nresults + 1)
	if cg.ExpectedResults > 0 {
		cField = cg.ExpectedResults + 1
	} else if cg.ExpectedResults == 0 {
		// ExpectedResults == 0 means "all results"
		cField = 0
	}
	// ExpectedResults == -1 means "default" (1 result, cField = 2)
	
	cg.EmitABC(vm.OP_VARARG, reg, 1, cField)
	return reg
}

