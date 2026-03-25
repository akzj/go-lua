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
	cg.EmitABC(vm.OP_LOADNIL, reg, 0, 0)
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
		// Use LOADI for small integers
		cg.EmitABC(vm.OP_LOADI, reg, int(expr.Int), 0)
	} else {
		// Use LOADK for other numbers
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
	} else {
		// It's a global variable (get from _ENV)
		// For now, treat as GETTABLE from _ENV
		// This is simplified; proper implementation needs _ENV handling
		cg.emitLoadConstant(reg, *object.NewString(expr.Name))
		cg.EmitABC(vm.OP_GETTABLE, reg, 0, reg) // Simplified
	}

	return reg
}

// genIndex generates code for table[index] access.
func (cg *CodeGenerator) genIndex(expr *parser.IndexExpr) int {
	// Generate table expression
	tableReg := cg.genExpr(expr.Table)

	// Generate index expression
	indexReg := cg.genExpr(expr.Index)

	// Allocate result register
	resultReg := cg.allocRegister()

	// Emit GETTABLE: R(A) := R(B)[R(C)]
	cg.EmitABC(vm.OP_GETTABLE, resultReg, tableReg, indexReg)

	// Free temporaries
	cg.freeRegister() // indexReg
	cg.freeRegister() // tableReg

	return resultReg
}

// genField generates code for table.field access.
func (cg *CodeGenerator) genField(expr *parser.FieldExpr) int {
	// Generate table expression
	tableReg := cg.genExpr(expr.Table)

	// Allocate result register
	resultReg := cg.allocRegister()

	// Load field name as constant
	fieldIdx := cg.addOrGetConstant(*object.NewString(expr.Field))

	// Emit GETFIELD: R(A) := R(B)[K(C)]
	if fieldIdx <= 255 {
		cg.EmitABC(vm.OP_GETFIELD, resultReg, tableReg, fieldIdx)
	} else {
		// For large indices, use GETTABUP or similar
		// Simplified: use GETTABLE with loaded key
		keyReg := cg.allocRegister()
		cg.EmitABx(vm.OP_LOADK, keyReg, fieldIdx)
		cg.EmitABC(vm.OP_GETTABLE, resultReg, tableReg, keyReg)
		cg.freeRegister() // keyReg
	}

	// Free table register
	cg.freeRegister() // tableReg

	return resultReg
}

// genCall generates code for a function call.
func (cg *CodeGenerator) genCall(expr *parser.CallExpr) int {
	// Generate function expression
	funcReg := cg.genExpr(expr.Func)

	// Check if it's actually a method call in disguise (table:method)
	// For regular calls, arguments start at funcReg + 1

	// Generate arguments
	argCount := len(expr.Args)
	for i, arg := range expr.Args {
		argReg := cg.genExpr(arg)
		// Move argument to correct position if needed
		if argReg != funcReg+1+i {
			cg.EmitABC(vm.OP_MOVE, funcReg+1+i, argReg, 0)
			cg.freeRegister()
		}
	}

	// Emit CALL: R(A) := R(A)(R(A+1), ..., R(A+C-1))
	// B = argCount + 1 (including function), 0 = vararg
	// C = 2 (1 result), 0 = multiple results
	resultReg := funcReg
	cg.EmitABC(vm.OP_CALL, resultReg, argCount+1, 2)

	// After CALL, result is in funcReg
	// Free the argument registers (they were consumed by CALL)
	cg.setStackTop(funcReg + 1)

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
	if methodIdx <= 255 {
		cg.EmitABC(vm.OP_SELF, funcReg, objReg, methodIdx)
	} else {
		// For large indices, load key first
		keyReg := cg.allocRegister()
		cg.EmitABx(vm.OP_LOADK, keyReg, methodIdx)
		cg.EmitABC(vm.OP_MOVE, funcReg+1, objReg, 0)
		cg.EmitABC(vm.OP_GETTABLE, funcReg, objReg, keyReg)
		cg.freeRegister() // keyReg
	}

	// Now funcReg contains the method, funcReg+1 contains the object (self)

	// Generate arguments (starting at funcReg+2)
	argCount := len(expr.Args)
	for i, arg := range expr.Args {
		argReg := cg.genExpr(arg)
		targetReg := funcReg + 2 + i
		if argReg != targetReg {
			cg.EmitABC(vm.OP_MOVE, targetReg, argReg, 0)
			cg.freeRegister()
		}
	}

	// Emit CALL
	cg.EmitABC(vm.OP_CALL, funcReg, argCount+2, 2) // +2 for self

	// Set stack top
	cg.setStackTop(funcReg + 1)

	// Free object register
	cg.freeRegister()

	return funcReg
}

// genBinOp generates code for a binary operation.
func (cg *CodeGenerator) genBinOp(expr *parser.BinOpExpr) int {
	switch expr.Op {
	case "and":
		return cg.genAnd(expr)
	case "or":
		return cg.genOr(expr)
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

// genArithmetic generates code for arithmetic operations.
func (cg *CodeGenerator) genArithmetic(expr *parser.BinOpExpr) int {
	// Generate left operand
	leftReg := cg.genExpr(expr.Left)

	// Generate right operand
	rightReg := cg.genExpr(expr.Right)

	// Allocate result register
	resultReg := cg.allocRegister()

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
	case "~=":
		// Not equal is equal with inverted result
		cg.EmitABC(vm.OP_EQ, 1, leftReg, rightReg) // if (R(B) ~= R(C)) then pc++
		cg.EmitAsBx(vm.OP_JMP, 0, 1)     // skip to load false
		cg.EmitABC(vm.OP_LOADBOOL, resultReg, 1, 0) // load true
		cg.EmitAsBx(vm.OP_JMP, 0, 1)     // skip over false
		cg.EmitABC(vm.OP_LOADBOOL, resultReg, 0, 0) // load false
		cg.freeRegisters(2)
		return resultReg
	case "<":
		op = vm.OP_LT
	case ">":
		// Swap operands for >
		cg.EmitABC(vm.OP_LT, 0, rightReg, leftReg)
		cg.EmitAsBx(vm.OP_JMP, 0, 1)
		cg.EmitABC(vm.OP_LOADBOOL, resultReg, 1, 0)
		cg.EmitAsBx(vm.OP_JMP, 0, 1)
		cg.EmitABC(vm.OP_LOADBOOL, resultReg, 0, 0)
		cg.freeRegisters(2)
		return resultReg
	case "<=":
		op = vm.OP_LE
	case ">=":
		// Swap operands for >=
		cg.EmitABC(vm.OP_LE, 0, rightReg, leftReg)
		cg.EmitAsBx(vm.OP_JMP, 0, 1)
		cg.EmitABC(vm.OP_LOADBOOL, resultReg, 1, 0)
		cg.EmitAsBx(vm.OP_JMP, 0, 1)
		cg.EmitABC(vm.OP_LOADBOOL, resultReg, 0, 0)
		cg.freeRegisters(2)
		return resultReg
	case "..":
		// Concatenation - simplified
		cg.EmitABC(vm.OP_CONCAT, resultReg, leftReg, rightReg)
		cg.freeRegisters(2)
		return resultReg
	default:
		// Unknown operator, default to ADD
		op = vm.OP_ADD
	}

	// Emit arithmetic instruction: R(A) := R(B) op R(C)
	cg.EmitABC(op, resultReg, leftReg, rightReg)

	// Free operand registers
	cg.freeRegister() // rightReg
	cg.freeRegister() // leftReg

	return resultReg
}

// genUnOp generates code for a unary operation.
func (cg *CodeGenerator) genUnOp(expr *parser.UnOpExpr) int {
	// Generate operand
	operandReg := cg.genExpr(expr.Expr)

	// Allocate result register
	resultReg := cg.allocRegister()

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

	// Emit unary instruction: R(A) := op R(B)
	cg.EmitABC(op, resultReg, operandReg, 0)

	// Free operand register
	cg.freeRegister()

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
	for _, entry := range expr.Entries {
		switch entry.Kind {
		case parser.TableEntryValue:
			// Array-style entry: value
			valueReg := cg.genExpr(entry.Value)
			// SETI R(A)[B] := RK(C)
			cg.EmitABC(vm.OP_SETI, resultReg, arrayIndex, valueReg)
			cg.freeRegister()
			arrayIndex++

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

	// Generate nested function
	nestedProto := nestedGen.Generate(&parser.FuncDefStmt{
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
	cg.EmitABC(vm.OP_VARARG, reg, 1, 2)
	return reg
}

