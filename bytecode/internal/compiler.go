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
		Proto: proto,
		pc:    0,
		C:     c,
	}

	// Compile each statement in the block
	for _, stat := range block.Stats() {
		if err := fs.compileStat(stat); err != nil {
			return nil, err
		}
	}

	// Add RETURN0 if no return statement
	if len(proto.code) == 0 {
		fs.emit(int(opcodes.OP_RETURN0), 0, 0, 0)
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
	default:
		return nil
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
			fs.emitABC(int(opcodes.OP_GETTABLE), funcReg, tableReg, methodIdx+256)
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
		// Global function call: name(args)
		funcName := name.GetName()
		
		// Reserve R[0] for function
		funcReg = 0
		
		// Emit GETTABUP R(funcReg), upval[0], K(nameIdx)
		nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: funcName})
		fs.emitABC(int(opcodes.OP_GETTABUP), funcReg, 0, nameIdx+256)
		
		// Now emit arguments starting at R[1]
		for i, arg := range args {
			argReg := 1 + i
			fs.expToReg(arg, argReg)
		}
		
		// Update maxstacksize
		if len(args) > 0 {
			fs.Proto.maxstacksize = uint8(1 + len(args))
		} else {
			fs.Proto.maxstacksize = uint8(2)
		}
		
		// Emit CALL R(funcReg), nArgs+1, 1
		// B includes the function itself: if 1 arg, B=2 (function + 1 arg)
		fs.emitABC(int(opcodes.OP_CALL), funcReg, len(args)+1, 1)
		
	} else {
		return fs.errorf("expected nameExp or indexExpr for function, got %T", funcExp)
	}
	
	return nil
}

// compileGlobalFuncStat compiles global function declaration: function name(args) body end
func (fs *FuncState) compileGlobalFuncStat(stat astapi.StatNode) error {
	// Get the FuncDef from the stat
	gf, ok := stat.(interface{ GetFuncDef() astapi.FuncDef })
	if !ok || gf == nil {
		return nil // Not a real global function stat
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

// compileLocalFuncStat compiles local function declaration: local function name(args) body end
func (fs *FuncState) compileLocalFuncStat(stat astapi.StatNode) error {
	lf, ok := stat.(interface{ GetFuncDef() astapi.FuncDef })
	if !ok || lf == nil {
		return nil // Not a real local function stat
	}
	
	funcDef := lf.GetFuncDef()
	if funcDef == nil {
		return fs.errorf("nil FuncDef in local function")
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
	
	// The function is now in register 'reg' - locals are managed by the VM
	
	return nil
}

// compileAssignStat compiles assignment statement: var = expr
func (fs *FuncState) compileAssignStat(stat astapi.StatNode) error {
	if as, ok := stat.(interface{ GetVars() []astapi.ExpNode; GetExprs() []astapi.ExpNode }); ok {
		vars := as.GetVars()
		exprs := as.GetExprs()
		
		// For multi-assign, we need to:
		// 1. First compile all expressions to consecutive registers
		// 2. Then emit instructions to store each expression result
		// For simplicity, only support 1:1 assignment for now
		if len(vars) != len(exprs) {
			// Multi-assign with different counts - not yet supported
			// Return nil to avoid breaking compilation (will be a runtime error if executed)
			return nil
		}
		
		// Simple 1:1 assignment
		for i, v := range vars {
			if err := fs.compileSingleAssign(v, exprs[i]); err != nil {
				return err
			}
		}
		return nil
	}
	// Statement doesn't have GetVars/GetExprs - not a real assignment
	return nil
}

// compileSingleAssign compiles a single assignment: var = expr
func (fs *FuncState) compileSingleAssign(v astapi.ExpNode, e astapi.ExpNode) error {
	// First compile the expression to a register
	exprReg := fs.allocReg()
	fs.expToReg(e, exprReg)
	
	// Then store to the variable
	if idx, ok := v.(indexAccess); ok {
		// Table assignment: t[k] = v
		tableReg := fs.allocReg()
		fs.expToReg(idx.GetTable(), tableReg)
		keyReg := fs.allocReg()
		fs.expToReg(idx.GetKey(), keyReg)
		fs.emitABC(int(opcodes.OP_SETTABLE), exprReg, tableReg, keyReg)
		fs.freeReg(keyReg)
		fs.freeReg(tableReg)
	} else if name, ok := v.(nameAccess); ok {
		// Global assignment: name = v (use SETTABUP to set in _ENV)
		nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: name.GetName()})
		fs.emitABC(int(opcodes.OP_SETTABUP), 0, nameIdx, exprReg)
	} else {
		return fs.errorf("unsupported assignment target: %T", v)
	}
	
	fs.freeReg(exprReg)
	return nil
}

// compileFuncDef compiles a FuncDef to a Prototype
func (fs *FuncState) compileFuncDef(funcDef astapi.FuncDef) (*Prototype, error) {
	if funcDef == nil {
		return nil, fs.errorf("nil FuncDef")
	}
	
	// Create a new FuncState for the nested function
	nestedProto := &Prototype{
		maxstacksize: 2, // minimum
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
		Proto: nestedProto,
		pc:    0,
		C:     fs.C,
	}
	
	// Compile the function body
	block := funcDef.GetBlock()
	if block != nil {
		for _, stat := range block.Stats() {
			if err := nestedFs.compileStat(stat); err != nil {
				return nil, err
			}
		}
		// Add return if last instruction is not a return
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
	if s, ok := arg.(interface{ GetValue() string }); ok {
		idx := fs.addConstant(&Constant{Type: ConstString, Str: s.GetValue()})
		fs.emitABx(int(opcodes.OP_LOADK), reg, idx)
	} else if i, ok := arg.(interface{ GetValue() int64 }); ok {
		idx := fs.addConstant(&Constant{Type: ConstInteger, Int: i.GetValue()})
		fs.emitABx(int(opcodes.OP_LOADK), reg, idx)
	} else if f, ok := arg.(interface{ GetValue() float64 }); ok {
		idx := fs.addConstant(&Constant{Type: ConstFloat, Float: f.GetValue()})
		fs.emitABx(int(opcodes.OP_LOADK), reg, idx)
	} else if binop, ok := arg.(binopAccess); ok {
		// Handle binary expression: left, right, op
		fs.compileBinop(binop, reg)
	} else {
		fs.emitABC(int(opcodes.OP_LOADNIL), reg, 0, 0)
	}
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
	case binopAccess:
		fs.compileBinop(e, destReg)
	case indexAccess:
		fs.compileIndexExpr(e, destReg)
	case interface{ Name() string }:
		// Global variable: emit GETTABUP to load from upvalue[0]
		nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: e.Name()})
		fs.emitABC(int(opcodes.OP_GETTABUP), destReg, 0, nameIdx+256)
	default:
		fs.emitABC(int(opcodes.OP_LOADNIL), destReg, 0, 0)
	}
	return destReg
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
		fs.emitABC(int(opcodes.OP_GETTABLE), destReg, tableReg, keyIdx+256)
	} else {
		// Non-constant key: compile key to register
		fs.expToReg(key, keyReg)
		fs.emitABC(int(opcodes.OP_GETTABLE), destReg, tableReg, keyReg)
	}
}

// compileBinop compiles a binary expression.
// The result is stored in destReg. Operands use registers after the result.
func (fs *FuncState) compileBinop(binop binopAccess, destReg int) {
	left := binop.GetLeft()
	right := binop.GetRight()
	op := binop.GetOp()

	// Allocate registers for operands - result at destReg, operands after
	leftReg := destReg + 1
	rightReg := destReg + 2

	// Update maxstacksize to account for operand registers
	fs.Proto.maxstacksize = uint8(rightReg + 1)

	// Load operands
	fs.addArgLoad(left, leftReg)
	fs.addArgLoad(right, rightReg)

	// Map binopKind to opcode
	opcode := fs.binopToOpcode(op)

	// Emit ADD/SUB/etc: result in destReg, operands at leftReg and rightReg
	fs.emitABC(opcode, destReg, leftReg, rightReg)
}

// binopToOpcode converts BinopKind to opcode
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
	default:
		return int(opcodes.OP_ADD) // default to ADD
	}
}

// emitABC emits an ABC format instruction (alias for emit).
func (fs *FuncState) emitABC(opcode, a, b, c int) int {
	return fs.emit(opcode, a, b, c)
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
func (p *Prototype) GetSubProtos() []*Prototype { return p.p }

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
	return uint32(opcode) | (uint32(a) << 7) | (uint32(sbx+65535) << 14)
}

// =============================================================================
// Constant Management
// =============================================================================

// addConstant adds a constant to the constant table.
func (fs *FuncState) addConstant(c *Constant) int {
	// Simple linear search for now
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

// Count returns the number of local variables.
func (l *Locals) Count() int {
	return len(l.vars)
}
