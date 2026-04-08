// Package internal provides the bytecode compiler implementation.
package internal

import (
	"fmt"

	astapi "github.com/akzj/go-lua/ast/api"
	bcapi "github.com/akzj/go-lua/bytecode/api"
	opcodes "github.com/akzj/go-lua/opcodes/api"
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
		maxstacksize:   0,
		k:               make([]*bcapi.Constant, 0),
		code:            make([]uint32, 0),
	}

	fs := &FuncState{
		Proto:        proto,
		pc:           0,
		C:            c,
		labelScopes:  []map[string]int{make(map[string]int)},
		pendingGotos: make(map[string][]int),
		gotoScopes:   make(map[int]int),
	}

	// Compile each statement in the block
	for _, stat := range block.Stats() {
		if err := fs.compileStat(stat); err != nil {
			return nil, err
		}
	}

	// Check for undefined labels referenced by gotos
	if len(fs.pendingGotos) > 0 {
		var undefinedLabels []string
		for labelName := range fs.pendingGotos {
			undefinedLabels = append(undefinedLabels, labelName)
		}
		if len(undefinedLabels) > 0 {
			return nil, bcapi.NewCompileError(0, 0, "labels not defined: %v", undefinedLabels)
		}
	}

	// Handle block-level return expressions (e.g., "return 42" at end of chunk)
	// In Lua, return at the end of a block is stored in Block.ReturnExp(),
	// NOT as a STAT_RETURN statement.
	retExps := block.ReturnExp()
	if len(retExps) > 0 {
		// Compile each return expression to consecutive registers
		firstReg := -1
		for _, exp := range retExps {
			reg := fs.allocReg()
			if firstReg == -1 {
				firstReg = reg
			}
			fs.expToReg(exp, reg)
		}
		// Emit RETURN with first register and number of results
		n := len(retExps)
		if firstReg == -1 {
			firstReg = 0
		}
		fs.emitABC(int(opcodes.OP_RETURN), firstReg, n+1, 0)
	} else {
		// Add RETURN0 at end of function block (no return values)
		if len(proto.code) == 0 || (proto.code[len(proto.code)-1]>>6)&0x3F != uint32(opcodes.OP_RETURN0) {
			fs.emit(int(opcodes.OP_RETURN0), 0, 0, 0)
		}
	}

	return proto, nil
}

// compileStat compiles a single statement.
func (fs *FuncState) compileStat(stat astapi.StatNode) error {
	switch stat.Kind() {
	case astapi.STAT_CALL:
		return fs.compileCallStat(stat)
	case astapi.STAT_ASSIGN:
		return fs.compileAssignStat(stat)
	case astapi.STAT_LOCAL_FUNC:
		return fs.compileLocalFuncStat(stat)
	case astapi.STAT_GLOBAL_FUNC:
		return fs.compileGlobalFuncStat(stat)
	case astapi.STAT_LOCAL_VAR:
		return fs.compileLocalVarStat(stat)
	case astapi.STAT_GLOBAL_VAR:
		return fs.compileGlobalVarStat(stat)
	case astapi.STAT_RETURN:
		return fs.compileReturnStat(stat)
	case astapi.STAT_IF:
		return fs.compileIfStat(stat)
	case astapi.STAT_WHILE:
		return fs.compileWhileStat(stat)
	case astapi.STAT_DO:
		return fs.compileDoStat(stat)
	case astapi.STAT_REPEAT:
		return fs.compileRepeatStat(stat)
	case astapi.STAT_FOR_NUM, astapi.STAT_FOR_IN, astapi.STAT_BREAK:
		return fs.compileBlockStat(stat)
	case astapi.STAT_GOTO:
		return fs.compileGotoStat(stat)
	case astapi.STAT_LABEL:
		return fs.compileLabelStat(stat)
	default:
		return fmt.Errorf("unsupported statement kind: %v (type %T)", stat.Kind(), stat)
	}
}

// compileCallStat compiles a function call statement.
func (fs *FuncState) compileCallStat(stat astapi.StatNode) error {
	var call astapi.FuncCall
	
	// Try GetExpr first (expressionStat from parser)
	if exprStat, ok := stat.(interface{ GetExpr() astapi.ExpNode }); ok {
		exp := exprStat.GetExpr()
		if exp != nil {
			// Check if it is a FuncCall
			if fc, ok := exp.(astapi.FuncCall); ok {
				call = fc
			} else {
				// Not a FuncCall - skip silently (e.g., tableConstructor expression)
				return nil
			}
		} else {
			// Nil expression - skip silently
			return nil
		}
	} else {
		// No GetExpr - not an expression statement
		return nil
	}
	
	if call == nil {
		return nil
	}
	
	// Get function expression
	funcExp := call.Func()
	args := call.Args()
	
	// Determine if this is a method call (indexExpr) or global call (nameExp)
	var funcReg int
	
	// Check if funcExp is a function definition (anonymous function)
	if funcDef, ok := funcExp.(interface{ Kind() astapi.ExpKind }); ok && funcDef.Kind() == astapi.EXP_FUNC {
		// Anonymous function call: (function() end)(args)
		// Compile the function definition to a register
		funcReg = fs.allocReg()
		fs.expToReg(funcExp, funcReg)
		// Emit arguments
		for i, arg := range args {
			argReg := funcReg + 1 + i
			fs.expToReg(arg, argReg)
		}
		// Update maxstacksize
		if len(args) > 0 {
			fs.Proto.maxstacksize = uint8(funcReg + 1 + len(args))
		} else {
			fs.Proto.maxstacksize = uint8(funcReg + 2)
		}
		// Emit CALL
		fs.emitABC(int(opcodes.OP_CALL), funcReg, len(args)+1, 1)
		fs.freeReg(funcReg)
		return nil
	}
	
	if idx, ok := funcExp.(indexAccess); ok {
		// Method call: obj:method(args) -> compiles to:
		// 1. Load obj to register
		// 2. GETTABLE obj.method to register (SELF would be better but requires MOVE first)
		// 3. MOVE obj to R(A+1) for self
		// 4. Load args starting at R(A+2)
		// 5. CALL R(A), nArgs+2, 1
		
		table := idx.GetTable()
		key := idx.GetKey()
		
		// Allocate register for method result
		funcReg = fs.allocReg()
		
		// Compile table (object) to a temp register
		tableReg := fs.allocReg()
		fs.expToReg(table, tableReg)
		
		// Get method from table: GETTABLE R(funcReg), R(tableReg), K(methodName)
		if s, ok := key.(interface{ GetValue() string }); ok {
			methodIdx := fs.addConstant(&Constant{Type: ConstString, Str: s.GetValue()})
			fs.emitABC(int(opcodes.OP_GETFIELD), funcReg, tableReg, methodIdx)
		} else {
			keyReg := fs.allocReg()
			fs.expToReg(key, keyReg)
			fs.emitABC(int(opcodes.OP_GETTABLE), funcReg, tableReg, keyReg)
		}
		
		// Self argument: MOVE R(funcReg+1), R(tableReg) - object is now self
		fs.emitABC(int(opcodes.OP_MOVE), funcReg+1, tableReg, 0)
		
		// Update maxstacksize
		fs.Proto.maxstacksize = uint8(funcReg + 2)
		
		// Emit arguments starting at R(funcReg+2)
		for i, arg := range args {
			argReg := funcReg + 2 + i
			fs.expToReg(arg, argReg)
			if argReg+1 > int(fs.Proto.maxstacksize) {
				fs.Proto.maxstacksize = uint8(argReg + 1)
			}
		}
		
		// Emit CALL R(funcReg), nArgs+2, 1
		// +2 because: function + self + args
		fs.emitABC(int(opcodes.OP_CALL), funcReg, len(args)+2, 1)
		
	} else if name, ok := funcExp.(nameAccess); ok {
		funcName := name.GetName()
		
		// Allocate a fresh register for the function
		funcReg = fs.allocReg()
		
		// Check if it's a local variable first
		if localReg := fs.locals.Find(funcName); localReg >= 0 {
			// Local variable: use MOVE to copy from local register
			fs.emitABC(int(opcodes.OP_MOVE), funcReg, localReg, 0)
		} else {
			// Global variable: emit GETTABUP to load from upvalue[0]
			nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: funcName})
			fs.emitABC(int(opcodes.OP_GETTABUP), funcReg, 0, nameIdx)
		}
		
		// Now emit arguments starting at R[funcReg+1]
		for i, arg := range args {
			argReg := funcReg + 1 + i
			fs.expToReg(arg, argReg)
			if argReg+1 > int(fs.Proto.maxstacksize) {
				fs.Proto.maxstacksize = uint8(argReg + 1)
			}
		}
		
		// Update maxstacksize
		if len(args) > 0 {
			if funcReg+1+len(args) > int(fs.Proto.maxstacksize) {
				fs.Proto.maxstacksize = uint8(funcReg + 1 + len(args))
			}
		} else {
			if funcReg+2 > int(fs.Proto.maxstacksize) {
				fs.Proto.maxstacksize = uint8(funcReg + 2)
			}
		}
		
		// Emit CALL R(funcReg), nArgs+1, 1
		// B includes the function itself: if 1 arg, B=2 (function + 1 arg)
		fs.emitABC(int(opcodes.OP_CALL), funcReg, len(args)+1, 1)
		
	} else {
		// Fallback: treat as any expression (handles binopExp, etc.)
		funcReg = fs.allocReg()
		fs.expToReg(funcExp, funcReg)
		
		// Emit arguments
		for i, arg := range args {
			argReg := funcReg + 1 + i
			fs.expToReg(arg, argReg)
			if argReg+1 > int(fs.Proto.maxstacksize) {
				fs.Proto.maxstacksize = uint8(argReg + 1)
			}
		}
		
		// Update maxstacksize
		if len(args) > 0 {
			if funcReg+1+len(args) > int(fs.Proto.maxstacksize) {
				fs.Proto.maxstacksize = uint8(funcReg + 1 + len(args))
			}
		} else {
			if funcReg+2 > int(fs.Proto.maxstacksize) {
				fs.Proto.maxstacksize = uint8(funcReg + 2)
			}
		}
		
		// Emit CALL
		fs.emitABC(int(opcodes.OP_CALL), funcReg, len(args)+1, 1)
		fs.freeReg(funcReg)
	}
	
	return nil
}

// compileGlobalFuncStat compiles global function declaration: function name(args) body end
func (fs *FuncState) compileGlobalFuncStat(stat astapi.StatNode) error {
	// Get the FuncDef from the stat
	gf, ok := stat.(interface{ GetFuncDef() astapi.FuncDef })
	if !ok || gf == nil {
		return fs.errorf("invalid global function statement") // Should not happen in normal flow
	}
	
	funcDef := gf.GetFuncDef()
	if funcDef == nil {
		return fs.errorf("nil FuncDef in global function")
	}
	
	// Get the function name
	name := ""
	if gn, ok := stat.(interface{ GetName() string }); ok {
		name = gn.GetName()
	}
	
	// Compile the function to get its prototype
	funcProto, err := fs.compileFuncDef(funcDef)
	if err != nil {
		return err
	}
	
	// Add the prototype as a constant
	funcIdx := fs.addConstant(&Constant{Type: ConstFunction, Func: funcProto})
	
	// Emit CLOSURE to load function into a register
	reg := fs.allocReg()
	fs.emitABx(int(opcodes.OP_CLOSURE), reg, funcIdx)
	
	// Emit SETTABUP to store the function in the global environment (_ENV is upvalue 0)
	nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: name})
	fs.emitABC(int(opcodes.OP_SETTABUP), 0, nameIdx, reg)
	
	fs.freeReg(reg)
	return nil
}

// compileGlobalVarStat compiles global variable declaration (Lua 5.4): global name = expr
func (fs *FuncState) compileGlobalVarStat(stat astapi.StatNode) error {
	gv, ok := stat.(interface{ GetName() string; GetExprs() []astapi.ExpNode })
	if !ok {
		return fs.errorf("invalid global var statement")
	}
	
	name := gv.GetName()
	exps := gv.GetExprs()
	
	// Compile each expression to a register
	for _, exp := range exps {
		reg := fs.allocReg()
		if exp != nil {
			fs.expToReg(exp, reg)
		} else {
			fs.emitABC(int(opcodes.OP_LOADNIL), reg, 0, 0)
		}
		// Emit SETTABUP to store in global environment (_ENV is upvalue 0)
		nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: name})
		fs.emitABC(int(opcodes.OP_SETTABUP), 0, nameIdx, reg)
		fs.freeReg(reg)
	}
	
	return nil
}

// compileLocalFuncStat compiles local function declaration: local function name(args) body end
// compileLocalFuncStat compiles local function declaration: local function name(args) body end
func (fs *FuncState) compileLocalFuncStat(stat astapi.StatNode) error {
	lf, ok := stat.(interface{ GetFuncDef() astapi.FuncDef })
	if !ok || lf == nil {
		return fs.errorf("invalid local function statement") // Should not happen in normal flow
	}
	
	funcDef := lf.GetFuncDef()
	if funcDef == nil {
		return fs.errorf("nil FuncDef in local function")
	}
	
	// Get the function name
	var funcName string
	if namer, ok := stat.(interface{ GetName() string }); ok {
		funcName = namer.GetName()
	}
	

	
	// Compile the function to get its prototype
	funcProto, err := fs.compileFuncDef(funcDef)
	if err != nil {
		return err
	}
	
	// Add the prototype as a constant
	funcIdx := fs.addConstant(&Constant{Type: ConstFunction, Func: funcProto})
	
	// Allocate a register for the function
	reg := fs.allocReg()

	
	// Emit CLOSURE to load function into register
	fs.emitABx(int(opcodes.OP_CLOSURE), reg, funcIdx)
	
	// Register the local variable
	if funcName != "" {
		fs.locals.Add(funcName, reg, 0)
		// Also store in _ENV so nested functions can find it via GETTABUP
		nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: funcName})
		fs.emitABC(int(opcodes.OP_SETTABUP), 0, nameIdx, reg)
	}
	
	return nil
}
func (fs *FuncState) compileLocalVarStat(stat astapi.StatNode) error {
	// Use interface to access concrete type fields
	var names []string
	var exps []astapi.ExpNode
	
	switch lv := stat.(type) {
	case interface{ GetNames() []string }:
		names = lv.GetNames()
	case interface{ Names() []string }:
		names = lv.Names()
	}
	
	switch lv := stat.(type) {
	case interface{ GetExprs() []astapi.ExpNode }:
		exps = lv.GetExprs()
	case interface{ Exprs() []astapi.ExpNode }:
		exps = lv.Exprs()
	}
	
	if names == nil && exps == nil {
		return fs.errorf("invalid local var statement")
	}
	
	if names == nil {
		names = make([]string, len(exps))
	}
	if exps == nil {
		exps = make([]astapi.ExpNode, len(names))
	}
	
	// Handle case where names and exps may have different lengths
	nVars := len(names)
	nExps := len(exps)
	
	// Compile expressions to registers
	for i, exp := range exps {
		if exp == nil {
			reg := fs.allocReg()
			fs.emitABC(int(opcodes.OP_LOADNIL), reg, 0, 0)
			if i < nVars && names[i] != "" {
				fs.locals.Add(names[i], reg, fs.pc)
			}
			continue
		}
		// Check if this is the last expression AND it's a function call
		// AND there are more variables than expressions — use multi-return
		if i == nExps-1 && nVars > i+1 {
			if fc, ok := exp.(astapi.FuncCall); ok {
				nResults := nVars - i
				funcReg, err := fs.compileFuncCallToVars(fc, nResults)
				if err != nil {
					return err
				}
				// Register all remaining locals from funcReg onwards
				for j := i; j < nVars; j++ {
					if names[j] != "" {
						fs.locals.Add(names[j], funcReg+j-i, fs.pc)
					}
				}
				// Allocate registers for the extra results (funcReg+1..funcReg+nResults-1)
				// compileFuncCallToVars already allocated funcReg, allocate the rest
				for j := 1; j < nResults; j++ {
					fs.allocReg()
				}
				return nil // All vars handled by multi-return
			}
		}
		// Normal single-value expression
		reg := fs.allocReg()
		fs.expToReg(exp, reg)
		if i < nVars && names[i] != "" {
			fs.locals.Add(names[i], reg, fs.pc)
		}
	}
	
	// Handle extra variables without expressions (local x, y)
	for i := nExps; i < nVars; i++ {
		reg := fs.allocReg()
		fs.emitABC(int(opcodes.OP_LOADNIL), reg, 0, 0)
		if names[i] != "" {
			fs.locals.Add(names[i], reg, fs.pc)
		}
	}
	
	return nil
}

// compileReturnStat compiles return statement: return expr1, expr2, ...
func (fs *FuncState) compileReturnStat(stat astapi.StatNode) error {
	ret, ok := stat.(interface{ GetExprs() []astapi.ExpNode })
	if !ok {
		return fs.errorf("invalid return statement")
	}
	
	exps := ret.GetExprs()
	if exps == nil || len(exps) == 0 {
		// Return no values
		fs.emitABC(int(opcodes.OP_RETURN), 0, 1, 0)
		return nil
	}
	
	// Compile each expression to consecutive registers
	firstReg := -1
	for _, exp := range exps {
		reg := fs.allocReg()
		if firstReg == -1 {
			firstReg = reg
		}
		fs.expToReg(exp, reg)
	}
	
	// Emit RETURN with first register and number of results
	n := len(exps)
	if firstReg == -1 {
		firstReg = 0
	}
	fs.emitABC(int(opcodes.OP_RETURN), firstReg, n+1, 0)
	
	return nil
}

// compileBlock compiles a block of statements
func (fs *FuncState) compileBlock(block astapi.Block) error {
	if block == nil {
		return nil
	}
	for _, stat := range block.Stats() {
		if stat != nil {
			if err := fs.compileStat(stat); err != nil {
				return err
			}
		}
	}
	// Handle block-level return expressions
	retExps := block.ReturnExp()
	if len(retExps) > 0 {
		firstReg := -1
		for _, exp := range retExps {
			reg := fs.allocReg()
			if firstReg == -1 {
				firstReg = reg
			}
			fs.expToReg(exp, reg)
		}
		n := len(retExps)
		if firstReg == -1 {
			firstReg = 0
		}
		fs.emitABC(int(opcodes.OP_RETURN), firstReg, n+1, 0)
	}
	return nil
}

// compileIfStat compiles if statement: if cond then thenBlock [else elseBlock] end
func (fs *FuncState) compileIfStat(stat astapi.StatNode) error {
	ifStmt, ok := stat.(interface {
		GetCondition() astapi.ExpNode
		GetThenBlock() astapi.Block
		GetElseBlock() astapi.Block
	})
	if !ok {
		return fs.errorf("invalid if statement")
	}

	// Compile condition
	condReg := fs.allocReg()
	fs.expToReg(ifStmt.GetCondition(), condReg)

	// TEST condReg, if false JMP to else (c=0 means jump if false)
	fs.emitABC(int(opcodes.OP_TEST), condReg, 0, 0)
	fs.freeReg(condReg) // Free condition register after TEST
	jmpToElseIdx := fs.pc
	fs.emitSJ(0) // placeholder jump

	// Compile then block with new label scope
	if thenBlock := ifStmt.GetThenBlock(); thenBlock != nil {
		fs.labelScopes = append(fs.labelScopes, make(map[string]int))
		err := fs.compileBlock(thenBlock)
		fs.labelScopes = fs.labelScopes[:len(fs.labelScopes)-1]
		if err != nil {
			return err
		}
	}

	// JMP to end (skip else block)
	jmpToEndIdx := fs.pc
	fs.emitSJ(0) // placeholder jump

	// Patch else jump target
	fs.patchSJ(jmpToElseIdx, fs.pc)

	// Compile else block with new label scope
	if elseBlock := ifStmt.GetElseBlock(); elseBlock != nil {
		fs.labelScopes = append(fs.labelScopes, make(map[string]int))
		err := fs.compileBlock(elseBlock)
		fs.labelScopes = fs.labelScopes[:len(fs.labelScopes)-1]
		if err != nil {
			return err
		}
	}

	// Patch end jump target
	fs.patchSJ(jmpToEndIdx, fs.pc)

	return nil
}

// compileDoStat compiles do...end block statement
func (fs *FuncState) compileDoStat(stat astapi.StatNode) error {
	doStmt, ok := stat.(interface{ GetBlock() astapi.Block })
	if !ok {
		return fs.errorf("invalid do statement")
	}
	if block := doStmt.GetBlock(); block != nil {
		// Push a new label scope for the do...end block
		fs.labelScopes = append(fs.labelScopes, make(map[string]int))
		err := fs.compileBlock(block)
		// Pop the label scope when exiting the block
		fs.labelScopes = fs.labelScopes[:len(fs.labelScopes)-1]
		return err
	}
	return nil
}

// compileWhileStat compiles while statement: while cond do block end
func (fs *FuncState) compileWhileStat(stat astapi.StatNode) error {
	whileStmt, ok := stat.(interface {
		GetCondition() astapi.ExpNode
		GetBlock() astapi.Block
	})
	if !ok {
		return fs.errorf("invalid while statement")
	}

	// Mark loop start position
	loopStart := fs.pc

	// Compile condition
	condReg := fs.allocReg()
	fs.expToReg(whileStmt.GetCondition(), condReg)

	// TEST condReg, if false JMP to end (c=0 means jump if false)
	fs.emitABC(int(opcodes.OP_TEST), condReg, 0, 0)
	jmpToEndIdx := fs.pc
	fs.emitSJ(0) // placeholder jump

	// Save previous pending breaks and loop exit index
	savedPendingBreaks := fs.pendingBreaks
	savedLoopExitIdx := fs.loopExitIdx
	fs.pendingBreaks = make([]int, 0)
	fs.loopExitIdx = jmpToEndIdx // break jumps will patch to this index

	// Compile loop body with new label scope
	if block := whileStmt.GetBlock(); block != nil {
		fs.labelScopes = append(fs.labelScopes, make(map[string]int))
		err := fs.compileBlock(block)
		fs.labelScopes = fs.labelScopes[:len(fs.labelScopes)-1]
		if err != nil {
			fs.pendingBreaks = savedPendingBreaks
			fs.loopExitIdx = savedLoopExitIdx
			return err
		}
	}

	// JMP back to condition
	fs.emitSJ(loopStart - fs.pc - 1)

	// Patch end jump target - this is the loop exit point
	fs.patchSJ(jmpToEndIdx, fs.pc)

	// Patch all pending break jumps to loop exit
	for _, breakIdx := range fs.pendingBreaks {
		fs.patchSJ(breakIdx, fs.pc)
	}

	// Restore previous pending breaks and loop exit index
	fs.pendingBreaks = savedPendingBreaks
	fs.loopExitIdx = savedLoopExitIdx

	return nil
}

// compileRepeatStat compiles repeat...until statement: repeat block until condition
// The body executes first, then condition is tested.
// Loop continues while condition is FALSE, exits when TRUE.
// Fix: negate the condition (false→true, true→false) and use TEST to check.
// If negated is TRUE (original was FALSE), we JMP back (continue loop).
func (fs *FuncState) compileRepeatStat(stat astapi.StatNode) error {
	repeatStmt, ok := stat.(interface {
		GetCondition() astapi.ExpNode
		GetBlock() astapi.Block
	})
	if !ok {
		return fs.errorf("invalid repeat statement")
	}

	// Mark loop start position
	loopStart := fs.pc

	// Save previous pending breaks and loop exit index
	savedPendingBreaks := fs.pendingBreaks
	savedLoopExitIdx := fs.loopExitIdx
	fs.pendingBreaks = make([]int, 0)
	fs.loopExitIdx = -1

	// Compile loop body first (repeat...until executes body before testing condition)
	if block := repeatStmt.GetBlock(); block != nil {
		if err := fs.compileBlock(block); err != nil {
			fs.pendingBreaks = savedPendingBreaks
			fs.loopExitIdx = savedLoopExitIdx
			return err
		}
	}

	// Negate the condition using conditional jumps:
	// negReg = NOT(IsTrue(condReg))
	// 1. LOADFALSE negReg    ; default: negReg = false
	// 2. TEST condReg, 0     ; JMP if condReg is FALSE (to setTrue)
	// 3. JMP skipTrue        ; skip over LOADTRUE
	// setTrue:
	// 4. LOADTRUE negReg     ; cond was false → set negReg = true
	// skipTrue:
	// (negReg now has negated boolean)

	negReg := fs.allocReg()
	// Step 1: default negReg = false
	fs.emit(int(opcodes.OP_LOADFALSE), negReg, 0, 0)
	// Step 2: TEST condReg, 0 — JMP if condReg is FALSE
	condReg := fs.allocReg()
	fs.expToReg(repeatStmt.GetCondition(), condReg)
	fs.emitABC(int(opcodes.OP_TEST), condReg, 0, 0)
	jmpToSetTrue := fs.pc
	fs.emitSJ(0) // placeholder
	// Step 3: JMP to skip LOADTRUE
	jmpSkipTrue := fs.pc
	fs.emitSJ(0) // placeholder
	// Step 4: setTrue — LOADTRUE negReg
	setTrueIdx := fs.pc
	fs.emit(int(opcodes.OP_LOADTRUE), negReg, 0, 0)
	// Patch TEST JMP to setTrue, patch skip JMP to here
	fs.patchSJ(jmpToSetTrue, setTrueIdx)
	fs.patchSJ(jmpSkipTrue, setTrueIdx)

	// Now TEST negReg, 1 — JMP if negReg is TRUE (original was FALSE → continue)
	fs.emitABC(int(opcodes.OP_TEST), negReg, 1, 0)
	jmpBackIdx := fs.pc
	fs.emitSJ(0) // placeholder jump

	// Set loop exit index for break
	fs.loopExitIdx = fs.pc

	// Patch jump to loop start
	fs.patchSJ(jmpBackIdx, loopStart)

	// Patch pending break jumps
	for _, breakIdx := range fs.pendingBreaks {
		fs.patchSJ(breakIdx, fs.pc)
	}

	// Restore
	fs.pendingBreaks = savedPendingBreaks
	fs.loopExitIdx = savedLoopExitIdx

	return nil
}


// patchAsBx patches the last AsBx instruction's sBx field to jump to current PC
func (fs *FuncState) patchAsBx(instrIdx int) {
	if instrIdx >= 0 && instrIdx < len(fs.Proto.code) {
		// Get opcode and A from existing instruction
		oldInst := fs.Proto.code[instrIdx]
		op := int(oldInst >> 6 & 0x7F) // Opcode is 7 bits, mask with 0x7F
		a := int(oldInst >> 14 & 0x1FF)
		// Re-encode with new sBx
		fs.Proto.code[instrIdx] = encodeAsBx(op, a, fs.pc-instrIdx-1)
	}
}

// compileBlockStat compiles control flow statements (if, while, for, return, break)
// These delegate to specific compile methods based on statement type
func (fs *FuncState) compileBlockStat(stat astapi.StatNode) error {
	switch stat.Kind() {
	case astapi.STAT_FOR_NUM:
		return fs.compileForNum(stat)
	case astapi.STAT_FOR_IN:
		return fs.compileForIn(stat)
	case astapi.STAT_BREAK:
		return fs.compileBreakStat(stat)
	default:
		return fs.errorf("unsupported control flow statement: %v", stat.Kind())
	}
}

// forNumAccess interface for accessing for-num statement fields
type forNumAccess interface {
	GetName() string
	GetStart() astapi.ExpNode
	GetStop() astapi.ExpNode
	GetStep() astapi.ExpNode
	GetBlock() astapi.Block
}

// compileForNum compiles numeric for-loop: for var = start, stop [, step] do body end
// Generates:
//   LOADK R[A] = limit
//   LOADK R[A+1] = step
//   LOADK R[A+2] = init (loop var exposed to user)
//   FORPREP R[A] -> R[A+2] -= R[A+1]; jump to FORLOOP
//   [loop body]
//   FORLOOP R[A] -> R[A+2] += R[A+1]; if in range pc += sBx
//
// Lua 5.5 register layout (A points to base):
//   R[A]   = limit
//   R[A+1] = step
//   R[A+2] = var (loop variable exposed to user, FORLOOP updates this)
//
// FORPREP: R[A+2] -= R[A+1] (or R[A+2] = init - step), then jump
// FORLOOP: R[A+2] += R[A+1]; if (step > 0 && idx <= limit) || (step < 0 && idx >= limit) then pc += sBx
func (fs *FuncState) compileForNum(stat astapi.StatNode) error {
	forNum, ok := stat.(forNumAccess)
	if !ok {
		return fs.errorf("invalid for-num statement")
	}

	varName := forNum.GetName()
	startExp := forNum.GetStart()
	stopExp := forNum.GetStop()
	stepExp := forNum.GetStep()
	bodyBlock := forNum.GetBlock()

	// Allocate 3 consecutive registers:
	// R[baseReg]   = limit
	// R[baseReg+1] = step
	// R[baseReg+2] = var (loop variable exposed to user, FORLOOP updates this)
	baseReg := fs.allocReg()
	fs.allocReg()
	fs.allocReg()

	// Update maxstacksize
	fs.Proto.maxstacksize = uint8(baseReg + 3)

	// Register the loop variable (at R[baseReg+2]) - BEFORE body so it's visible
	fs.locals.Add(varName, baseReg+2, fs.pc)

	// Compile limit expression to R[baseReg]
	fs.expToReg(stopExp, baseReg)

	// Compile step expression to R[baseReg+1], or default to 1
	if stepExp != nil {
		fs.expToReg(stepExp, baseReg+1)
	} else {
		stepIdx := fs.addConstant(&Constant{Type: ConstInteger, Int: 1})
		fs.emitABx(int(opcodes.OP_LOADK), baseReg+1, stepIdx)
	}

	// Compile init expression to R[baseReg+2] (var)
	fs.expToReg(startExp, baseReg+2)

	// Emit FORPREP: R[A+2] -= R[A+1]; jump to FORLOOP
	forPrepIdx := fs.emitAsBx(int(opcodes.OP_FORPREP), baseReg, 0)

	// Save previous pending breaks and loop exit index
	savedPendingBreaks := fs.pendingBreaks
	savedLoopExitIdx := fs.loopExitIdx
	fs.pendingBreaks = make([]int, 0)
	// loopExitIdx will be set after FORLOOP

	// Compile loop body with new label scope
	if bodyBlock != nil {
		fs.labelScopes = append(fs.labelScopes, make(map[string]int))
		err := fs.compileBlock(bodyBlock)
		fs.labelScopes = fs.labelScopes[:len(fs.labelScopes)-1]
		if err != nil {
			fs.pendingBreaks = savedPendingBreaks
			fs.loopExitIdx = savedLoopExitIdx
			return err
		}
	}

	// Emit FORLOOP: R[A+2] += R[A+1]; if in range pc += sBx
	forLoopIdx := fs.emitAsBx(int(opcodes.OP_FORLOOP), baseReg, 0)

	// Patch FORPREP to jump to FORLOOP (which checks condition and loops back)
	fs.patchAsBxJump(forPrepIdx, forLoopIdx)

	// Patch FORLOOP to jump back to FORPREP+1 (right after FORPREP)
	fs.patchAsBxJump(forLoopIdx, forPrepIdx+1)

	// Set loop exit index and patch pending break jumps
	fs.loopExitIdx = fs.pc
	for _, breakIdx := range fs.pendingBreaks {
		fs.patchSJ(breakIdx, fs.pc)
	}

	// Restore previous pending breaks and loop exit index
	fs.pendingBreaks = savedPendingBreaks
	fs.loopExitIdx = savedLoopExitIdx

	return nil
}

// forInAccess interface for accessing for-in statement fields
type forInAccess interface {
	GetNames() []string
	GetExprs() []astapi.ExpNode
	GetBlock() astapi.Block
}

// compileForIn compiles generic for-loop: for vars in exprs do body end
// Lua 5.4 register layout:
//   R[A]   = iterator function
//   R[A+1] = state (invariant)
//   R[A+2] = control variable
//   R[A+3] = first loop variable (e.g., k)
//   R[A+4] = second loop variable (e.g., v)
// Generates:
//   [setup: compile exprs, CALL to get iterator/state/control into R[A..A+2]]
//   TFORPREP A sBx  → jump forward to TFORLOOP
//   [loop body]
//   TFORCALL A C    → R[A+4..A+3+C] := R[A](R[A+1], R[A+2])
//   TFORLOOP A sBx  → if R[A+2] ~= nil then pc -= sBx (back to body)
func (fs *FuncState) compileForIn(stat astapi.StatNode) error {
	forIn, ok := stat.(forInAccess)
	if !ok {
		return fs.errorf("invalid for-in statement")
	}

	names := forIn.GetNames()
	exprs := forIn.GetExprs()
	bodyBlock := forIn.GetBlock()

	if names == nil || len(names) == 0 {
		return fs.errorf("for-in requires at least one variable")
	}
	if exprs == nil || len(exprs) == 0 {
		return fs.errorf("for-in requires at least one expression")
	}

	nVars := len(names)

	// Layout: R[baseReg] = iterator, R[baseReg+1] = state, R[baseReg+2] = control
	//         R[baseReg+3..baseReg+2+nVars] = loop variables
	baseReg := fs.allocReg() // R[baseReg] = iterator function

	// Allocate registers for state, control, and loop variables
	for i := 1; i < 3+nVars; i++ {
		fs.allocReg()
	}

	// Update maxstacksize: need baseReg + 3 + nVars + extra for TFORCALL temp
	needed := baseReg + 3 + nVars + 3
	if needed > int(fs.Proto.maxstacksize) {
		fs.Proto.maxstacksize = uint8(needed)
	}

	// Compile the expression list.
	// For "pairs(t)", this is a single function call that returns 3 values.
	// We need 3 results: iterator, state, control → R[baseReg], R[baseReg+1], R[baseReg+2]
	if len(exprs) == 1 {
		// Single expression (common case: pairs(t) or ipairs(t))
		// Compile as a function call with 3 results
		if fc, ok := exprs[0].(astapi.FuncCall); ok {
			// Compile function call with 3 results into baseReg
			funcExp := fc.Func()
			args := fc.Args()
			fs.expToReg(funcExp, baseReg)
			for i, arg := range args {
				fs.expToReg(arg, baseReg+1+i)
			}
			nArgs := len(args)
			// CALL baseReg, nArgs+1, 4 (3 results: iterator, state, control)
			fs.emitABC(int(opcodes.OP_CALL), baseReg, nArgs+1, 4)
		} else {
			// Non-call expression: evaluate to baseReg, nil state and control
			fs.expToReg(exprs[0], baseReg)
		}
	} else {
		// Multiple expressions: first goes to baseReg, second to baseReg+1, etc.
		for i, expr := range exprs {
			if i < 3 {
				fs.expToReg(expr, baseReg+i)
			}
		}
	}

	// Emit TFORPREP: jump forward past the body to TFORLOOP
	tforPrepIdx := fs.emitAsBx(int(opcodes.OP_TFORPREP), baseReg, 0)

	// Register loop variables (they start at baseReg+3)
	for i, name := range names {
		fs.locals.Add(name, baseReg+3+i, fs.pc)
	}

	// Save break state
	savedPendingBreaks := fs.pendingBreaks
	savedLoopExitIdx := fs.loopExitIdx
	fs.pendingBreaks = make([]int, 0)

	// Compile loop body
	if bodyBlock != nil {
		fs.labelScopes = append(fs.labelScopes, make(map[string]int))
		err := fs.compileBlock(bodyBlock)
		fs.labelScopes = fs.labelScopes[:len(fs.labelScopes)-1]
		if err != nil {
			fs.pendingBreaks = savedPendingBreaks
			fs.loopExitIdx = savedLoopExitIdx
			return err
		}
	}

	// Emit TFORCALL: call R[A](R[A+1], R[A+2]), results to R[A+3..A+3+C-1]
	tforcallIdx := len(fs.Proto.code)
	fs.emitABC(int(opcodes.OP_TFORCALL), baseReg, 0, nVars)

	// Emit TFORLOOP: if R[A+3] ~= nil then jump back to body start
	tforLoopIdx := fs.emitAsBx(int(opcodes.OP_TFORLOOP), baseReg, 0)

	// Patch jumps:
	// TFORPREP jumps forward to TFORCALL (so iterator is called before first check)
	fs.patchAsBxJump(tforPrepIdx, tforcallIdx)

	// TFORLOOP jumps back to the start of the body (tforPrepIdx + 1)
	fs.patchAsBxJump(tforLoopIdx, tforPrepIdx+1)

	// Patch break jumps
	fs.loopExitIdx = fs.pc
	for _, breakIdx := range fs.pendingBreaks {
		fs.patchSJ(breakIdx, fs.pc)
	}

	fs.pendingBreaks = savedPendingBreaks
	fs.loopExitIdx = savedLoopExitIdx

	return nil
}

// compileBreakStat compiles break statement
func (fs *FuncState) compileBreakStat(stat astapi.StatNode) error {
	// Check if we're inside a loop (pendingBreaks != nil means we're in a loop)
	// We use nil check since pendingBreaks is set to make([]int, 0) in loops
	if fs.pendingBreaks == nil {
		return fs.errorf("break statement must be inside a loop")
	}
	// Emit JMP with 0 offset (will be patched to actual exit)
	jmpIdx := fs.emitSJ(0)
	fs.pendingBreaks = append(fs.pendingBreaks, jmpIdx)
	return nil
}

// compileGotoStat compiles goto statement: goto label
// Goto can jump forward (to a label not yet seen) or backward (to an existing label).
// For forward jumps, we emit a placeholder JMP and patch it when we encounter the label.
// Labels have block scope - a label is only visible within its block.
// A goto cannot jump into or out of a block.
func (fs *FuncState) compileGotoStat(stat astapi.StatNode) error {
	gt, ok := stat.(interface{ GetName() string })
	if !ok {
		return fs.errorf("invalid goto statement")
	}

	labelName := gt.GetName()
	currentScope := len(fs.labelScopes) - 1

	// Search only scopes at or above current level (not nested inner scopes)
	// This prevents jumping into or out of blocks
	for i := currentScope; i >= 0; i-- {
		if labelPC, exists := fs.labelScopes[i][labelName]; exists {
			// Label exists in an accessible scope - emit JMP to that position
			jmpIdx := fs.emitSJ(0)
			fs.patchSJ(jmpIdx, labelPC)
			return nil
		}
	}

	// Label not yet seen - emit placeholder JMP and record it for later patching
	// Store in the current scope
	jmpIdx := fs.emitSJ(0)
	fs.pendingGotos[labelName] = append(fs.pendingGotos[labelName], jmpIdx)
	fs.gotoScopes[jmpIdx] = currentScope

	return nil
}

// compileLabelStat compiles label statement: ::label::
// Labels have function-wide scope - no duplicate names allowed anywhere in the function.
func (fs *FuncState) compileLabelStat(stat astapi.StatNode) error {
	lbl, ok := stat.(interface{ GetName() string })
	if !ok {
		return fs.errorf("invalid label statement")
	}

	labelName := lbl.GetName()
	currentScope := len(fs.labelScopes) - 1

	// Check for duplicate label in ALL scopes (labels are function-wide unique)
	for i := 0; i < len(fs.labelScopes); i++ {
		if _, exists := fs.labelScopes[i][labelName]; exists {
			return fs.errorf("label '%s' already defined", labelName)
		}
	}

	// Record the label position in the current scope
	fs.labelScopes[currentScope][labelName] = fs.pc

	// Patch any pending gotos that reference this label
	// Validate that the goto can reach this label (same or outer scope)
	if pendingJumps, exists := fs.pendingGotos[labelName]; exists {
		for _, jmpIdx := range pendingJumps {
			gotoScope := fs.gotoScopes[jmpIdx]
			// Goto can only jump to same scope or outer scope (not into inner blocks)
			if gotoScope < currentScope {
				return fs.errorf("label '%s' not visible due to scope", labelName)
			}
			fs.patchSJ(jmpIdx, fs.pc)
		}
		delete(fs.pendingGotos, labelName)
	}

	return nil
}

// patchAsBxJump patches the sBx field of an AsBx instruction to jump to targetPC
func (fs *FuncState) patchAsBxJump(instrIdx int, targetPC int) {
	if instrIdx >= 0 && instrIdx < len(fs.Proto.code) {
		oldInst := fs.Proto.code[instrIdx]
		// Lua 5.4 instruction format: bits 0-6 = opcode, bits 7-14 = A
		op := int(oldInst & 0x7F)
		a := int((oldInst >> 7) & 0xFF)
		offset := targetPC - instrIdx - 1
		fs.Proto.code[instrIdx] = encodeAsBx(op, a, offset)
	}
}

// compileAssignStat compiles assignment statement: var = expr
func (fs *FuncState) compileAssignStat(stat astapi.StatNode) error {
	if as, ok := stat.(interface{ GetVars() []astapi.ExpNode; GetExprs() []astapi.ExpNode }); ok {
		vars := as.GetVars()
		exprs := as.GetExprs()
		
		nVars := len(vars)
		nExprs := len(exprs)
		
		// Special handling for multi-value assignment: a,b = f()
		// When len(vars) < len(exprs) and last expr is a function call
		// the function call may return multiple values
		if nVars > 0 && nExprs > 0 && nVars < nExprs {
			// Check if last expression is a function call
			lastExp := exprs[nExprs-1]
			if _, isFuncCall := lastExp.(astapi.FuncCall); isFuncCall {
				return fs.compileMultiAssign(vars, exprs)
			}
		}
		
		// Simple 1:1 assignment
		for i, v := range vars {
			if i < nExprs && exprs[i] != nil {
				if err := fs.compileSingleAssign(v, exprs[i]); err != nil {
					return err
				}
			} else {
				// Variable without expression - set to nil
				if name, ok := v.(nameAccess); ok {
					nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: name.GetName()})
					fs.emitABC(int(opcodes.OP_SETTABUP), 0, nameIdx, NO_REG)
				} else if idx, ok := v.(indexAccess); ok {
					tableReg := fs.allocReg()
					fs.expToReg(idx.GetTable(), tableReg)
					keyReg := fs.allocReg()
					fs.expToReg(idx.GetKey(), keyReg)
					fs.emitABC(int(opcodes.OP_LOADNIL), NO_REG, 0, 0)
					fs.emitABC(int(opcodes.OP_SETTABLE), tableReg, keyReg, NO_REG)
					fs.freeReg(keyReg)
					fs.freeReg(tableReg)
				}
			}
		}
		return nil
	}
	// Statement doesn't have GetVars/GetExprs - skip (e.g., break/continue in compiler)
	return nil
}

// compileMultiAssign handles multi-value assignment like a,b = f()
// where the number of variables may not match the number of expressions
func (fs *FuncState) compileMultiAssign(vars []astapi.ExpNode, exprs []astapi.ExpNode) error {
	nVars := len(vars)
	nExprs := len(exprs)
	
	// Compile all expressions except the last one to consecutive registers
	for i := 0; i < nExprs-1; i++ {
		reg := fs.allocReg()
		fs.expToReg(exprs[i], reg)
	}
	
	// For the last expression (function call), we need to:
	// 1. Compile function to a register
	// 2. Call with expected results = nVars
	// 3. The results will be placed starting at that register
	lastExp := exprs[nExprs-1]
	
	var funcReg int
	// Handle function call specially
	if funcCall, ok := lastExp.(astapi.FuncCall); ok {
		var err error
		funcReg, err = fs.compileFuncCallToVars(funcCall, nVars)
		if err != nil {
			return err
		}
	} else {
		// Non-function-call expression: compile to a register
		funcReg = fs.allocReg()
		fs.expToReg(lastExp, funcReg)
	}
	
	// Now assign results to variables using MOVE or SETUPVAL
	// Results are at funcReg..funcReg+nVars-1
	for i, v := range vars {
		if err := fs.assignToVar(v, funcReg+i); err != nil {
			return err
		}
	}
	
	return nil
}

// compileFuncCallToVars compiles a function call that returns multiple values
// The expected number of results is nVars
// Returns the register where the function was placed (results are at funcReg..funcReg+nVars-1)
func (fs *FuncState) compileFuncCallToVars(call astapi.FuncCall, nVars int) (int, error) {
	funcExp := call.Func()
	args := call.Args()
	
	// Allocate register for the function
	funcReg := fs.allocReg()
	fs.expToReg(funcExp, funcReg)
	
	// Emit arguments
	for i, arg := range args {
		argReg := funcReg + 1 + i
		fs.expToReg(arg, argReg)
		if argReg+1 > int(fs.Proto.maxstacksize) {
			fs.Proto.maxstacksize = uint8(argReg + 1)
		}
	}
	
	// Update maxstacksize for arguments
	nArgs := len(args)
	if nArgs > 0 {
		if funcReg+1+nArgs > int(fs.Proto.maxstacksize) {
			fs.Proto.maxstacksize = uint8(funcReg + 1 + nArgs)
		}
	} else {
		if funcReg+2 > int(fs.Proto.maxstacksize) {
			fs.Proto.maxstacksize = uint8(funcReg + 2)
		}
	}
	
	// Emit CALL: R[funcReg], nArgs+1, nVars+1
	// B = nArgs + 1 (includes function itself)
	// C = nVars + 1 (number of results including function slot)
	fs.emitABC(int(opcodes.OP_CALL), funcReg, nArgs+1, nVars+1)
	
	return funcReg, nil
}

// assignToVar assigns the value at srcReg to variable v
func (fs *FuncState) assignToVar(v astapi.ExpNode, srcReg int) error {
	if name, ok := v.(nameAccess); ok {
		// Check if it's a local variable
		if localReg := fs.locals.Find(name.GetName()); localReg >= 0 {
			// Local variable: use MOVE to copy
			fs.emitABC(int(opcodes.OP_MOVE), localReg, srcReg, 0)
		} else {
			// Global variable: use SETTABUP
			nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: name.GetName()})
			fs.emitABC(int(opcodes.OP_SETTABUP), 0, nameIdx, srcReg)
		}
	} else if idx, ok := v.(indexAccess); ok {
		// Table assignment: t[k] = v
		tableReg := fs.allocReg()
		fs.expToReg(idx.GetTable(), tableReg)
		// Check if key is a string constant — use SETFIELD for efficiency
		key := idx.GetKey()
		if strKey, ok := key.(interface{ GetValue() string }); ok {
			keyIdx := fs.addConstant(&Constant{Type: ConstString, Str: strKey.GetValue()})
			fs.emitABC(int(opcodes.OP_SETFIELD), tableReg, keyIdx, srcReg)
		} else {
			keyReg := fs.allocReg()
			fs.expToReg(key, keyReg)
			fs.emitABC(int(opcodes.OP_SETTABLE), tableReg, keyReg, srcReg)
			fs.freeReg(keyReg)
		}
		fs.freeReg(tableReg)
	}
	return nil
}

// NO_REG constant for cases where we need MAX_FSTACK (no register)
const NO_REG = 0x1FF

// compileSingleAssign compiles a single assignment: var = expr
func (fs *FuncState) compileSingleAssign(v astapi.ExpNode, e astapi.ExpNode) error {
	// Compile the expression to a register
	exprReg := fs.allocReg()
	fs.expToReg(e, exprReg)
	// Delegate to assignToVar which correctly checks locals first
	err := fs.assignToVar(v, exprReg)
	fs.freeReg(exprReg)
	return err
}

// compileFuncDef compiles a FuncDef to a Prototype
func (fs *FuncState) compileFuncDef(funcDef astapi.FuncDef) (*Prototype, error) {
	if funcDef == nil {
		return nil, fs.errorf("nil FuncDef")
	}
	
	// Create a new FuncState for the nested function
	nParams := uint8(len(funcDef.GetParams()))
	nestedProto := &Prototype{
		maxstacksize: nParams, // start at number of parameters
		k:            make([]*bcapi.Constant, 0),
		code:          make([]uint32, 0),
	}
	
	// Get function info from interface
	nestedProto.lineDefined = funcDef.Line()
	nestedProto.lastLineDefined = funcDef.LastLine()
	nestedProto.numparams = uint8(len(funcDef.GetParams()))
	if funcDef.IsVarArg() {
		nestedProto.numparams = nestedProto.numparams | 0x80 // Vararg flag
	}
	
	nestedFs := &FuncState{
		Proto:        nestedProto,
		pc:           0,
		C:            fs.C,
		labelScopes:  []map[string]int{make(map[string]int)},
		pendingGotos: make(map[string][]int),
		gotoScopes:   make(map[int]int),
	}
	
	// Compile the function body
	block := funcDef.GetBlock()
	if block != nil {
		for _, stat := range block.Stats() {
			if err := nestedFs.compileStat(stat); err != nil {
				return nil, err
			}
		}
		
		// Handle return statement if present (stored in block.returnExp by parser)
		if retExp := block.ReturnExp(); retExp != nil {
			firstReg := -1
			for _, exp := range retExp {
				reg := nestedFs.allocReg()
				if firstReg == -1 {
					firstReg = reg
				}
				nestedFs.expToReg(exp, reg)
			}
			n := len(retExp)
			if firstReg == -1 {
				firstReg = 0
			}
			nestedFs.emitABC(int(opcodes.OP_RETURN), firstReg, n+1, 0)
		}
		
		// Add implicit return if last instruction is not a return
		if len(nestedProto.code) == 0 || ((nestedProto.code[len(nestedProto.code)-1]>>6)&0x3F) != uint32(opcodes.OP_RETURN0) {
			nestedFs.emit(int(opcodes.OP_RETURN0), 0, 1, 0)
		}
	}
	
	return nestedProto, nil
}

// addArgConstant adds an argument as a constant
func (fs *FuncState) addArgConstant(arg astapi.ExpNode) {
	if s, ok := arg.(interface{ GetValue() string }); ok {
		fs.addConstant(&Constant{Type: ConstString, Str: s.GetValue()})
	} else if i, ok := arg.(interface{ GetValue() int64 }); ok {
		fs.addConstant(&Constant{Type: ConstInteger, Int: i.GetValue()})
	} else if f, ok := arg.(interface{ GetValue() float64 }); ok {
		fs.addConstant(&Constant{Type: ConstFloat, Float: f.GetValue()})
	}
}

// binopAccess interface for accessing binop fields
// unaryOpAccess matches UnopExp which has public fields Op and Exp.
type unaryOpAccess interface {
	GetUnaryOp() (astapi.UnopKind, astapi.ExpNode)
}

type binopAccess interface {
	GetOp() astapi.BinopKind
	GetLeft() astapi.ExpNode
	GetRight() astapi.ExpNode
}

// indexAccess interface for accessing indexed expressions (obj.key)
type indexAccess interface {
	GetTable() astapi.ExpNode
	GetKey() astapi.ExpNode
}

// nameAccess interface for accessing name expressions
type nameAccess interface {
	GetName() string
}

// addArgLoad emits code to load an argument into a register
func (fs *FuncState) addArgLoad(arg astapi.ExpNode, reg int) {
	// Delegate to expToReg which handles all expression types correctly.
	// Previously this was a parallel incomplete dispatch that missed FuncCall,
	// TableConstructor, Name lookups, etc. — causing them to emit LOADNIL.
	fs.expToReg(arg, reg)
}

// expToReg compiles an expression to a register.
// Handles constants, binary expressions, and indexed expressions.
// Returns the register index where the result is stored.
func (fs *FuncState) expToReg(exp astapi.ExpNode, destReg int) int {
	switch e := exp.(type) {
	case interface{ GetValue() string }:
		idx := fs.addConstant(&Constant{Type: ConstString, Str: e.GetValue()})
		fs.emitABx(int(opcodes.OP_LOADK), destReg, idx)
	case interface{ GetValue() int64 }:
		idx := fs.addConstant(&Constant{Type: ConstInteger, Int: e.GetValue()})
		fs.emitABx(int(opcodes.OP_LOADK), destReg, idx)
	case interface{ GetValue() float64 }:
		idx := fs.addConstant(&Constant{Type: ConstFloat, Float: e.GetValue()})
		fs.emitABx(int(opcodes.OP_LOADK), destReg, idx)
	case unaryOpAccess:
		// Unary operators: #, -, ~, not
		fs.compileUnaryOp(e, destReg)
	case binopAccess:
		// Must check binopAccess BEFORE Kind() since binopExp implements both
		fs.compileBinop(e, destReg)
	case indexAccess:
		// Must check indexAccess BEFORE Kind() since indexExpr may implement both
		fs.compileIndexExpr(e, destReg)
	case interface{ Name() string }:
		// First check if it's a local variable (MUST be before Kind() since nameExp implements both)
		name := e.Name()
		reg := fs.locals.Find(name)
		if reg >= 0 {
			// Local variable: emit MOVE to copy from local register
			fs.emitABC(int(opcodes.OP_MOVE), destReg, reg, 0)
		} else {
			// Global variable: emit GETTABUP to load from upvalue[0]
			nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: name})
			fs.emitABC(int(opcodes.OP_GETTABUP), destReg, 0, nameIdx)
		}
	case astapi.FuncCall:
		// Function call as expression — MUST be before Kind() since FuncCallImpl has Kind()
		funcExp := e.Func()
		args := e.Args()

		// Compile function to destReg
		fs.expToReg(funcExp, destReg)

		// Compile arguments starting at destReg+1
		for i, arg := range args {
			argReg := destReg + 1 + i
			fs.expToReg(arg, argReg)
			if argReg+1 > int(fs.Proto.maxstacksize) {
				fs.Proto.maxstacksize = uint8(argReg + 1)
			}
		}

		// Update maxstacksize for arguments
		nArgs := len(args)
		if nArgs > 0 {
			if destReg+1+nArgs > int(fs.Proto.maxstacksize) {
				fs.Proto.maxstacksize = uint8(destReg + 1 + nArgs)
			}
		} else {
			if destReg+2 > int(fs.Proto.maxstacksize) {
				fs.Proto.maxstacksize = uint8(destReg + 2)
			}
		}

		// Emit CALL R(destReg), nArgs+1, 2 (1 result into destReg)
		// C=2 means 1 result (C-1), since expToReg is called when the value is needed
		fs.emitABC(int(opcodes.OP_CALL), destReg, nArgs+1, 2)
	case interface{ NumFields() int; NumRecords() int }:
		// Table constructor — MUST be before Kind() since TableConstructorImpl has Kind()
		fs.compileTableConstructor(e, destReg)
	case interface{ Kind() astapi.ExpKind }:
		// Handle boolean and nil constants — catch-all for remaining ExpKind types
		switch e.Kind() {
		case astapi.EXP_TRUE:
			fs.emit(int(opcodes.OP_LOADTRUE), destReg, 0, 0)
		case astapi.EXP_FALSE:
			fs.emit(int(opcodes.OP_LOADFALSE), destReg, 0, 0)
		case astapi.EXP_NIL:
			fs.emitABC(int(opcodes.OP_LOADNIL), destReg, 0, 0)
		default:
			fs.emitABC(int(opcodes.OP_LOADNIL), destReg, 0, 0)
		}
	default:
		fs.emitABC(int(opcodes.OP_LOADNIL), destReg, 0, 0)
	}
	return destReg
}

// compileTableConstructor compiles a table constructor expression {fields}.
// Emits NEWTABLE followed by SETI/SETFIELD/SETTABLE for each field, and SETLIST for array batch.
func (fs *FuncState) compileTableConstructor(tc interface{ NumFields() int; NumRecords() int }, destReg int) {
	nArray := tc.NumFields()
	nHash := tc.NumRecords()

	// Emit NEWTABLE A B C k — A=dest, B=array size hint, C=hash size hint
	// For Lua 5.4, NEWTABLE uses log-encoded sizes. For simplicity, use 0 (VM will grow as needed).
	fs.emitABC(int(opcodes.OP_NEWTABLE), destReg, nArray, nHash)
	// NEWTABLE is followed by an EXTRAARG in Lua 5.4 when k=1; we skip this for now
	// (the VM should handle NEWTABLE without EXTRAARG for small tables)

	// If there are array fields, compile them with SETI
	if nArray > 0 {
		if iter, ok := tc.(interface {
			GetArrayField(int) astapi.ExpNode
		}); ok {
			for i := 0; i < nArray; i++ {
				field := iter.GetArrayField(i)
				tmpReg := fs.allocReg()
				fs.expToReg(field, tmpReg)
				// SETI A B C: R[A][B] = R[C]  (B is integer key, 1-based)
				fs.emitABC(int(opcodes.OP_SETI), destReg, i+1, tmpReg)
			}
		}
	}

	// If there are record fields, compile them with SETFIELD
	if nHash > 0 {
		if iter, ok := tc.(interface {
			GetRecordField(int) (astapi.ExpNode, astapi.ExpNode)
		}); ok {
			for i := 0; i < nHash; i++ {
				key, val := iter.GetRecordField(i)
				// Compile value to temp register
				valReg := destReg + 1
				fs.expToReg(val, valReg)
				if valReg+1 > int(fs.Proto.maxstacksize) {
					fs.Proto.maxstacksize = uint8(valReg + 1)
				}
				// If key is a string constant, use SETFIELD
				if strKey, ok := key.(interface{ GetValue() string }); ok {
					keyIdx := fs.addConstant(&Constant{Type: ConstString, Str: strKey.GetValue()})
					fs.emitABC(int(opcodes.OP_SETFIELD), destReg, keyIdx, valReg)
				} else {
					// General key: compile key, use SETTABLE
					keyReg := destReg + 2
					fs.expToReg(key, keyReg)
					if keyReg+1 > int(fs.Proto.maxstacksize) {
						fs.Proto.maxstacksize = uint8(keyReg + 1)
					}
					fs.emitABC(int(opcodes.OP_SETTABLE), destReg, keyReg, valReg)
				}
			}
		}
	}
}


// compileUnaryOp compiles unary operators: -, #, ~, not
func (fs *FuncState) compileUnaryOp(e unaryOpAccess, destReg int) {
	op, operand := e.GetUnaryOp()
	// Compile operand to destReg first
	operandReg := fs.allocReg()
	fs.expToReg(operand, operandReg)
	
	switch op {
	case astapi.UNOP_NEG:
		fs.emitABC(int(opcodes.OP_UNM), destReg, operandReg, 0)
	case astapi.UNOP_LEN:
		fs.emitABC(int(opcodes.OP_LEN), destReg, operandReg, 0)
	case astapi.UNOP_BNOT:
		fs.emitABC(int(opcodes.OP_BNOT), destReg, operandReg, 0)
	case astapi.UNOP_NOT:
		fs.emitABC(int(opcodes.OP_NOT), destReg, operandReg, 0)
	default:
		fs.emitABC(int(opcodes.OP_LOADNIL), destReg, 0, 0)
	}
	fs.freeReg(operandReg)
}

// compileIndexExpr compiles an indexed expression (table.key) to a register.
// Generates: GETTABLE R(dest), R(tableReg), K(keyIdx)
func (fs *FuncState) compileIndexExpr(idx indexAccess, destReg int) {
	table := idx.GetTable()
	key := idx.GetKey()

	// Compile table to a register
	tableReg := fs.expToReg(table, destReg+1)
	if tableReg != destReg+1 {
		// Table was already compiled elsewhere, need to move
		fs.emitABC(int(opcodes.OP_MOVE), destReg+1, tableReg, 0)
	}

	// Compile key (usually a string constant)
	keyReg := destReg + 2
	if s, ok := key.(interface{ GetValue() string }); ok {
		keyIdx := fs.addConstant(&Constant{Type: ConstString, Str: s.GetValue()})
		// Use GETTABLE with constant key encoded in B (for short strings)
		fs.emitABC(int(opcodes.OP_GETFIELD), destReg, tableReg, keyIdx)
	} else {
		// Non-constant key: compile key to register
		fs.expToReg(key, keyReg)
		fs.emitABC(int(opcodes.OP_GETTABLE), destReg, tableReg, keyReg)
	}
}

// compileAndOr compiles short-circuit and/or expressions.
// "a and b": if a is falsy, result = a; else result = b
// "a or b": if a is truthy, result = a; else result = b
// Uses TESTSET + JMP pattern for short-circuit evaluation.
func (fs *FuncState) compileAndOr(left, right astapi.ExpNode, op astapi.BinopKind, destReg int) {
	// Compile left operand to destReg
	fs.expToReg(left, destReg)

	// For "and": skip right side if left is falsy (k=0)
	// For "or": skip right side if left is truthy (k=1)
	var k int
	if op == astapi.BINOP_AND {
		k = 0 // TEST R[A], 0 → skip next if R[A] is falsy
	} else {
		k = 1 // TEST R[A], 1 → skip next if R[A] is truthy
	}

	// Emit TEST: if (not R[destReg] == k) then pc++ (skip the JMP)
	fs.emitABCk(int(opcodes.OP_TEST), destReg, k, 0, 0)

	// Emit JMP to skip the right-side evaluation (placeholder, patched below)
	jmpIdx := fs.emitSJ(0)

	// Compile right operand to destReg (overwrites left if we reach here)
	fs.expToReg(right, destReg)

	// Patch the JMP to skip over the right-side code
	fs.patchSJ(jmpIdx, fs.pc)
}

// compileConcat compiles string concatenation (a .. b).
// OP_CONCAT A B: R[A] := R[A] .. ... .. R[A+B-1]
func (fs *FuncState) compileConcat(left, right astapi.ExpNode, destReg int) {
	// Compile left to destReg, right to destReg+1
	fs.expToReg(left, destReg)
	fs.expToReg(right, destReg+1)
	if destReg+2 > int(fs.Proto.maxstacksize) {
		fs.Proto.maxstacksize = uint8(destReg + 2)
	}
	// OP_CONCAT A B: R[A] := R[A] .. ... .. R[A+B-1], B=2 for binary concat
	fs.emitABC(int(opcodes.OP_CONCAT), destReg, 2, 0)
}

// compileBinop compiles a binary expression.
// The result is stored in destReg. Operands use registers after the result.
func (fs *FuncState) compileBinop(binop binopAccess, destReg int) {
	left := binop.GetLeft()
	right := binop.GetRight()
	op := binop.GetOp()

	// Handle short-circuit operators (and/or) specially — they need TEST+JMP
	if op == astapi.BINOP_AND || op == astapi.BINOP_OR {
		fs.compileAndOr(left, right, op, destReg)
		return
	}

	// Handle concat specially — OP_CONCAT has different operand layout
	if op == astapi.BINOP_CONCAT {
		fs.compileConcat(left, right, destReg)
		return
	}

	// Allocate registers for operands - result at destReg, operands after
	leftReg := destReg + 1
	rightReg := destReg + 2

	// Update maxstacksize to account for operand registers
	fs.Proto.maxstacksize = uint8(rightReg + 1)

	// Map binopKind to opcode
	opcode := fs.binopToOpcode(op)

	// For comparison operators, swap operands for > and >=
	// a > b becomes b < a, a >= b becomes b <= a
	swap := (op == astapi.BINOP_GT || op == astapi.BINOP_GE)
	actualLeft := right
	actualRight := left
	if !swap {
		actualLeft = left
		actualRight = right
	}

	// Load operands (in swapped order for > and >=)
	fs.addArgLoad(actualLeft, leftReg)
	fs.addArgLoad(actualRight, rightReg)

	// Emit comparison or arithmetic op
	// For comparison ops (OP_LT, OP_LE): executor compares R[A] vs R[C]
	// So we put left operand in A (via MOVE), right operand in C, and B=0
	// For arithmetic ops: executor uses R[B] and R[C], result in R[A]
	if fs.isComparisonOp(opcode) {
		// Lua 5.4 comparison pattern: EQ/LT/LE A B k
		// VM: if (cond != k) then pc++ (skip next instruction)
		// Followed by boolean materialization: LFALSESKIP + LOADTRUE
		//
		// For ==: k=0 → skip LFALSESKIP when equal → LOADTRUE → true
		// For ~=: k=1 → skip LFALSESKIP when NOT equal → LOADTRUE → true
		// For < and <=: k=0 → skip LFALSESKIP when condition true → LOADTRUE → true
		k := 0
		if op == astapi.BINOP_NE {
			k = 1
		}
		fs.emitABCk(opcode, leftReg, k, rightReg, 0)
		fs.emitABC(int(opcodes.OP_LFALSESKIP), destReg, 0, 0)
		fs.emitABC(int(opcodes.OP_LOADTRUE), destReg, 0, 0)
	} else {
		// Arithmetic: result in destReg, operands in leftReg and rightReg
		fs.emitABC(opcode, destReg, leftReg, rightReg)
	}
}

// binopToOpcode converts BinopKind to opcode
// For > and >=, we emit OP_LT/OP_LE with swapped operands in compileBinop
func (fs *FuncState) binopToOpcode(op astapi.BinopKind) int {
	switch op {
	case astapi.BINOP_ADD:
		return int(opcodes.OP_ADD)
	case astapi.BINOP_SUB:
		return int(opcodes.OP_SUB)
	case astapi.BINOP_MUL:
		return int(opcodes.OP_MUL)
	case astapi.BINOP_DIV:
		return int(opcodes.OP_DIV)
	case astapi.BINOP_MOD:
		return int(opcodes.OP_MOD)
	case astapi.BINOP_POW:
		return int(opcodes.OP_POW)
	case astapi.BINOP_IDIV:
		return int(opcodes.OP_IDIV)
	case astapi.BINOP_BAND:
		return int(opcodes.OP_BAND)
	case astapi.BINOP_BOR:
		return int(opcodes.OP_BOR)
	case astapi.BINOP_BXOR:
		return int(opcodes.OP_BXOR)
	case astapi.BINOP_SHL:
		return int(opcodes.OP_SHL)
	case astapi.BINOP_SHR:
		return int(opcodes.OP_SHR)
	case astapi.BINOP_LT:
		return int(opcodes.OP_LT)
	case astapi.BINOP_LE:
		return int(opcodes.OP_LE)
	case astapi.BINOP_GT:
		return int(opcodes.OP_LT) // For >, use LT with swapped operands
	case astapi.BINOP_GE:
		return int(opcodes.OP_LE) // For >=, use LE with swapped operands
	case astapi.BINOP_EQ:
		return int(opcodes.OP_EQ)
	case astapi.BINOP_NE:
		return int(opcodes.OP_EQ) // OP_NE not available; use OP_EQ with K=1 for negation
	default:
		return int(opcodes.OP_ADD) // default to ADD
	}
}

// isComparisonOp returns true if the opcode is a comparison operator
func (fs *FuncState) isComparisonOp(opcode int) bool {
	return opcode == int(opcodes.OP_LT) || opcode == int(opcodes.OP_LE) ||
		opcode == int(opcodes.OP_EQ)
}

// emitABC emits an ABC format instruction (alias for emit).
func (fs *FuncState) emitABC(opcode, a, b, c int) int {
	return fs.emit(opcode, a, b, c)
}

// emitABCk emits an ABC instruction with explicit k-bit control.
func (fs *FuncState) emitABCk(opcode, a, k, b, c int) int {
	inst := uint32(opcode) | (uint32(a) << 7) | (uint32(k) << 15) | (uint32(b) << 16) | (uint32(c) << 24)
	fs.Proto.code = append(fs.Proto.code, inst)
	pc := fs.pc
	fs.pc++
	return pc
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
	k               []*bcapi.Constant
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
func (p *Prototype) GetCode() []uint32        { return p.code }
func (p *Prototype) GetConstants() []*bcapi.Constant { return p.k }
func (p *Prototype) GetSubProtos() []bcapi.Prototype {
	result := make([]bcapi.Prototype, len(p.p))
	for i, proto := range p.p {
		result[i] = proto
	}
	return result
}

func (p *Prototype) GetUpvalues() []bcapi.UpvalueDesc {
	result := make([]bcapi.UpvalueDesc, len(p.upvalues))
	for i, uv := range p.upvalues {
		result[i] = bcapi.UpvalueDesc{
			Name:    uv.Name,
			Instack: uv.Instack,
			Idx:     uv.Idx,
			Kind:    uv.Kind,
		}
	}
	return result
}

// Constant represents a compile-time constant value.
type Constant struct {
	Type  ConstantType
	Int   int64
	Float float64
	Str   string
	Func  *Prototype // Function prototype for closure constants
}

// ConstantType identifies the type of a constant.
type ConstantType uint8

const (
	ConstNil ConstantType = iota
	ConstInteger
	ConstFloat
	ConstString
	ConstBool
	ConstFunction
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
	locals        Locals
	pc            int
	Prev          *FuncState
	C             *Compiler
	labelScopes   []map[string]int // stack of scope maps for labels
	pendingGotos  map[string][]int // label name -> list of goto instruction positions
	gotoScopes    map[int]int      // goto instruction PC -> scope index where it was created
	loopExitIdx   int              // instruction index of loop exit point (for break statement)
	pendingBreaks []int            // pending break jump indices to patch to loop exit
}

// NewFuncState creates a new FuncState.
func NewFuncState(c *Compiler, proto *Prototype) *FuncState {
	return &FuncState{
		Proto:        proto,
		locals:       NewLocals(),
		pc:           0,
		C:            c,
		labelScopes:  []map[string]int{make(map[string]int)},
		pendingGotos: make(map[string][]int),
		gotoScopes:   make(map[int]int),
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

// freeReg frees a register by decrementing maxstacksize.
// Note: This assumes registers are freed in LIFO order (last allocated = first freed).
// For non-LIFO freeing, a more sophisticated register tracking system would be needed.
func (fs *FuncState) freeReg(reg int) {
	if int(fs.Proto.maxstacksize) > 0 && reg == int(fs.Proto.maxstacksize)-1 {
		fs.Proto.maxstacksize--
	}
}

// free_regs frees multiple consecutive registers starting from 'from' for 'n' count.
func (fs *FuncState) free_regs(from, n int) {
	for i := 0; i < n; i++ {
		fs.freeReg(from + i)
	}
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

// emitSJ emits a JMP instruction using sBx format (compatible with GetsBx).
// addBoolConstant emits a LOADBOOL instruction and returns the register index.
// This creates a dedicated register for true/false that can be reused.
func (fs *FuncState) addBoolConstant(val bool) int {
	reg := fs.allocReg()
	if val {
		fs.emit(int(opcodes.OP_LOADTRUE), reg, 0, 0)
	} else {
		fs.emit(int(opcodes.OP_LOADFALSE), reg, 0, 0)
	}
	return reg
}

func (fs *FuncState) emitSJ(sbx int) int {
	// Use AsBx format: opcode | A | sBx
	return fs.emitAsBx(int(opcodes.OP_JMP), 0, sbx)
}

// patchSJ patches the sBx field of a JMP instruction to jump to targetPC.
func (fs *FuncState) patchSJ(instrIdx int, targetPC int) {
	if instrIdx >= 0 && instrIdx < len(fs.Proto.code) {
		offset := targetPC - instrIdx - 1
		oldInst := fs.Proto.code[instrIdx]
		// Lua 5.4 instruction format: bits 0-6 = opcode, bits 7-14 = A
		op := int(oldInst & 0x7F)
		a := int((oldInst >> 7) & 0xFF)
		fs.Proto.code[instrIdx] = encodeAsBx(op, a, offset)
	}
}

// encodeABC encodes an ABC format instruction.
func encodeABC(opcode, a, b, c int) uint32 {
	var k int
	if c >= 256 {
		k = 1
		c -= 256
	}
	return uint32(opcode) | (uint32(a) << 7) | (uint32(k) << 15) | (uint32(b) << 16) | (uint32(c) << 24)
}

// encodeABx encodes an ABx format instruction.
// Layout: [opcode(7) | A(8) | Bx(17)]
// Bx starts at bit 15 (same as k bit position in ABC format)
func encodeABx(opcode, a, bx int) uint32 {
	return uint32(opcode) | (uint32(a) << 7) | (uint32(bx) << 15)
}

// encodeAsBx encodes an AsBx format instruction (signed Bx).
func encodeAsBx(opcode, a, sbx int) uint32 {
	// sBx is a signed 16-bit value (range -65535 to 65535), stored as unsigned with offset
	// OFFSET_sBx = 65535 (MAXARG_Bx >> 1)
	// Mask AFTER shift to keep only bits 15-31 of sBx field, preventing overflow
	return uint32(opcode) | (uint32(a) << 7) | ((uint32(sbx+65535) << 15) & 0xFFFF8000)
}

// =============================================================================
// Constant Management
// =============================================================================

// addConstant adds a constant to the constant table.
func (fs *FuncState) addConstant(c *Constant) int {
	// Handle ConstFunction specially - store in p[] (sub-prototypes)
	if c.Type == ConstFunction && c.Func != nil {
		// Check if this prototype already exists
		for i, p := range fs.Proto.p {
			if p == c.Func {
				return i // Return index into p[]
			}
		}
		// Add to sub-prototypes
		idx := len(fs.Proto.p)
		fs.Proto.p = append(fs.Proto.p, c.Func)
		return idx
	}

	// Simple linear search for regular constants
	for i, k := range fs.Proto.k {
		if k.Type != bcapi.ConstantType(c.Type) {
			continue
		}
		switch c.Type {
		case ConstNil:
			return i
		case ConstInteger:
			if k.Int == c.Int {
				return i
			}
		case ConstFloat:
			if k.Float == c.Float {
				return i
			}
		case ConstString:
			if k.Str == c.Str {
				return i
			}
		case ConstBool:
			if k.Int == c.Int {
				return i
			}
		}
	}
	idx := len(fs.Proto.k)
	fs.Proto.k = append(fs.Proto.k, &bcapi.Constant{
		Type:  bcapi.ConstantType(c.Type),
		Int:   c.Int,
		Float: c.Float,
		Str:   c.Str,
	})
	return idx
}

// equals compares two constants for equality.
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

// Find returns the register index for a variable name, or -1 if not found.
func (l *Locals) Find(name string) int {
	for i := len(l.vars) - 1; i >= 0; i-- {
		if l.vars[i].Name == name {
			return l.vars[i].Reg
		}
	}
	return -1
}

// Count returns the number of local variables.
func (l *Locals) Count() int {
	return len(l.vars)
}
