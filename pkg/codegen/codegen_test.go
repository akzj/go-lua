package codegen

import (
	"testing"

	"github.com/akzj/go-lua/pkg/object"
	"github.com/akzj/go-lua/pkg/parser"
	"github.com/akzj/go-lua/pkg/vm"
)

// TestCodeGenerator_Basic tests basic code generation functionality.
func TestCodeGenerator_Basic(t *testing.T) {
	cg := NewCodeGenerator()

	if cg == nil {
		t.Fatal("Failed to create CodeGenerator")
	}

	if cg.Prototype == nil {
		t.Fatal("Prototype should be initialized")
	}

	if cg.Constants == nil {
		t.Fatal("Constants map should be initialized")
	}

	if cg.Upvalues == nil {
		t.Fatal("Upvalues map should be initialized")
	}
}

// TestCodeGenerator_EmitInstructions tests instruction emission.
func TestCodeGenerator_EmitInstructions(t *testing.T) {
	cg := NewCodeGenerator()

	// Test EmitABC
	pc := cg.EmitABC(vm.OP_ADD, 1, 2, 3)
	if pc != 0 {
		t.Errorf("Expected PC 0, got %d", pc)
	}
	if len(cg.Prototype.Code) != 1 {
		t.Errorf("Expected 1 instruction, got %d", len(cg.Prototype.Code))
	}

	// Verify instruction (cast to vm.Instruction to access methods)
	instr := vm.Instruction(cg.Prototype.Code[0])
	if instr.Opcode() != vm.OP_ADD {
		t.Errorf("Expected OP_ADD, got %v", instr.Opcode())
	}
	if instr.A() != 1 {
		t.Errorf("Expected A=1, got %d", instr.A())
	}
	if instr.B() != 2 {
		t.Errorf("Expected B=2, got %d", instr.B())
	}
	if instr.C() != 3 {
		t.Errorf("Expected C=3, got %d", instr.C())
	}

	// Test EmitABx
	pc = cg.EmitABx(vm.OP_LOADK, 1, 100)
	if pc != 1 {
		t.Errorf("Expected PC 1, got %d", pc)
	}

	// Test EmitAsBx
	pc = cg.EmitAsBx(vm.OP_JMP, 0, 10)
	if pc != 2 {
		t.Errorf("Expected PC 2, got %d", pc)
	}

	// Test EmitAx
	pc = cg.EmitAx(vm.OP_EXTRAARG, 1000)
	if pc != 3 {
		t.Errorf("Expected PC 3, got %d", pc)
	}
}

// TestCodeGenerator_AddConstant tests constant table management.
func TestCodeGenerator_AddConstant(t *testing.T) {
	cg := NewCodeGenerator()

	// Add first constant
	idx1 := cg.AddConstant(*object.NewNumber(42.0))
	if idx1 != 0 {
		t.Errorf("Expected index 0, got %d", idx1)
	}
	if len(cg.Prototype.Constants) != 1 {
		t.Errorf("Expected 1 constant, got %d", len(cg.Prototype.Constants))
	}

	// Add second constant
	idx2 := cg.AddConstant(*object.NewString("hello"))
	if idx2 != 1 {
		t.Errorf("Expected index 1, got %d", idx2)
	}

	// Add duplicate number
	idx3 := cg.AddConstant(*object.NewNumber(42.0))
	if idx3 != 2 {
		t.Errorf("Expected index 2 for duplicate, got %d", idx3)
	}
}

// TestCodeGenerator_RegisterAllocation tests register allocation.
func TestCodeGenerator_RegisterAllocation(t *testing.T) {
	cg := NewCodeGenerator()

	// Allocate registers
	reg1 := cg.allocRegister()
	if reg1 != 0 {
		t.Errorf("Expected register 0, got %d", reg1)
	}
	if cg.StackTop != 1 {
		t.Errorf("Expected StackTop 1, got %d", cg.StackTop)
	}

	reg2 := cg.allocRegister()
	if reg2 != 1 {
		t.Errorf("Expected register 1, got %d", reg2)
	}
	if cg.StackTop != 2 {
		t.Errorf("Expected StackTop 2, got %d", cg.StackTop)
	}
	if cg.MaxStackSize != 2 {
		t.Errorf("Expected MaxStackSize 2, got %d", cg.MaxStackSize)
	}

	// Free register
	cg.freeRegister()
	if cg.StackTop != 1 {
		t.Errorf("Expected StackTop 1 after free, got %d", cg.StackTop)
	}

	// Allocate again
	reg3 := cg.allocRegister()
	if reg3 != 1 {
		t.Errorf("Expected register 1 after free, got %d", reg3)
	}
}

// TestCodeGenerator_LocalVariables tests local variable management.
func TestCodeGenerator_LocalVariables(t *testing.T) {
	cg := NewCodeGenerator()

	// Begin scope
	cg.beginScope()

	// Add local variable
	cg.addLocal("x", 0, false)

	// Get local variable
	_, ok := cg.getLocal("x")
	if !ok {
		t.Fatal("Failed to find local variable 'x'")
	}

	// Add another local
	cg.addLocal("y", 1, false)

	_, ok = cg.getLocal("y")
	if !ok {
		t.Fatal("Failed to find local variable 'y'")
	}

	// End scope
	cg.endScope()

	// Variable should be out of scope
	_, ok = cg.getLocal("x")
	if ok {
		t.Error("Variable 'x' should be out of scope")
	}
}

// TestCodeGenerator_PatchInstruction tests instruction patching.
func TestCodeGenerator_PatchInstruction(t *testing.T) {
	cg := NewCodeGenerator()

	// Emit a placeholder instruction
	pc := cg.EmitAsBx(vm.OP_JMP, 0, 0)

	// Patch it
	cg.patchJump(pc, 10)

	// Verify the patch (cast to vm.Instruction)
	instr := vm.Instruction(cg.Prototype.Code[pc])
	if instr.Opcode() != vm.OP_JMP {
		t.Errorf("Expected OP_JMP, got %v", instr.Opcode())
	}

	// The offset should be 10 - (pc + 1) = 10 - 1 = 9
	expectedOffset := 10 - (pc + 1)
	if instr.SBx() != expectedOffset {
		t.Errorf("Expected offset %d, got %d", expectedOffset, instr.SBx())
	}
}

// TestCodeGenerator_LoadConstant tests constant loading with optimization.
func TestCodeGenerator_LoadConstant(t *testing.T) {
	cg := NewCodeGenerator()

	// Test LOADI for small integer
	cg.emitLoadConstant(0, *object.NewNumber(42.0))
	instr := vm.Instruction(cg.Prototype.Code[0])
	if instr.Opcode() != vm.OP_LOADI {
		t.Errorf("Expected LOADI for small int, got %v", instr.Opcode())
	}

	// Test LOADK for large number
	cg2 := NewCodeGenerator()
	cg2.emitLoadConstant(0, *object.NewNumber(1000.0))
	instr = vm.Instruction(cg2.Prototype.Code[0])
	if instr.Opcode() != vm.OP_LOADK {
		t.Errorf("Expected LOADK for large number, got %v", instr.Opcode())
	}
}

// TestCodeGenerator_GenNil tests nil literal generation.
func TestCodeGenerator_GenNil(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	_ = cg.genNil()

	if len(cg.Prototype.Code) != 1 {
		t.Fatalf("Expected 1 instruction, got %d", len(cg.Prototype.Code))
	}

	instr := vm.Instruction(cg.Prototype.Code[0])
	if instr.Opcode() != vm.OP_LOADNIL {
		t.Errorf("Expected OP_LOADNIL, got %v", instr.Opcode())
	}
}

// TestCodeGenerator_GenBoolean tests boolean literal generation.
func TestCodeGenerator_GenBoolean(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Test true
	_ = cg.genBoolean(true)
	if len(cg.Prototype.Code) != 1 {
		t.Fatalf("Expected 1 instruction, got %d", len(cg.Prototype.Code))
	}

	instr := vm.Instruction(cg.Prototype.Code[0])
	if instr.Opcode() != vm.OP_LOADBOOL {
		t.Errorf("Expected OP_LOADBOOL, got %v", instr.Opcode())
	}
	if instr.B() != 1 {
		t.Errorf("Expected B=1 for true, got %d", instr.B())
	}

	// Test false
	cg2 := NewCodeGenerator()
	cg2.beginScope()
	_ = cg2.genBoolean(false)

	instr = vm.Instruction(cg2.Prototype.Code[0])
	if instr.B() != 0 {
		t.Errorf("Expected B=0 for false, got %d", instr.B())
	}
}

// TestCodeGenerator_GenNumber tests number literal generation.
func TestCodeGenerator_GenNumber(t *testing.T) {
	// Test small integer (should use LOADI)
	cg := NewCodeGenerator()
	cg.beginScope()

	expr := &parser.NumberExpr{
		Value: 42.0,
		Int:   42,
		IsInt: true,
	}
	_ = cg.genNumber(expr)

	if len(cg.Prototype.Code) != 1 {
		t.Fatalf("Expected 1 instruction, got %d", len(cg.Prototype.Code))
	}

	instr := vm.Instruction(cg.Prototype.Code[0])
	if instr.Opcode() != vm.OP_LOADI {
		t.Errorf("Expected OP_LOADI for small int, got %v", instr.Opcode())
	}

	// Test large number (should use LOADK)
	cg2 := NewCodeGenerator()
	cg2.beginScope()

	expr2 := &parser.NumberExpr{
		Value: 1000.5,
		IsInt: false,
	}
	_ = cg2.genNumber(expr2)

	instr = vm.Instruction(cg2.Prototype.Code[0])
	if instr.Opcode() != vm.OP_LOADK {
		t.Errorf("Expected OP_LOADK for float, got %v", instr.Opcode())
	}
}

// TestCodeGenerator_GenString tests string literal generation.
func TestCodeGenerator_GenString(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	_ = cg.genString("hello")

	if len(cg.Prototype.Code) != 1 {
		t.Fatalf("Expected 1 instruction, got %d", len(cg.Prototype.Code))
	}

	instr := vm.Instruction(cg.Prototype.Code[0])
	if instr.Opcode() != vm.OP_LOADK {
		t.Errorf("Expected OP_LOADK, got %v", instr.Opcode())
	}

	// Verify constant was added
	if len(cg.Prototype.Constants) != 1 {
		t.Errorf("Expected 1 constant, got %d", len(cg.Prototype.Constants))
	}

	str, ok := cg.Prototype.Constants[0].ToString()
	if !ok || str != "hello" {
		t.Errorf("Expected constant 'hello', got %v", cg.Prototype.Constants[0])
	}
}

// TestCodeGenerator_GenArithmetic tests arithmetic operation generation.
func TestCodeGenerator_GenArithmetic(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Create addition expression: 1 + 2
	left := &parser.NumberExpr{Value: 1.0, Int: 1, IsInt: true}
	right := &parser.NumberExpr{Value: 2.0, Int: 2, IsInt: true}
	expr := &parser.BinOpExpr{
		Left:  left,
		Op:    "+",
		Right: right,
	}

	_ = cg.genArithmetic(expr)

	// Should have: LOADI, LOADI, ADD
	if len(cg.Prototype.Code) < 3 {
		t.Fatalf("Expected at least 3 instructions, got %d", len(cg.Prototype.Code))
	}

	// Last instruction should be ADD
	lastInstr := vm.Instruction(cg.Prototype.Code[len(cg.Prototype.Code)-1])
	if lastInstr.Opcode() != vm.OP_ADD {
		t.Errorf("Expected OP_ADD, got %v", lastInstr.Opcode())
	}
}

// TestCodeGenerator_GenUnary tests unary operation generation.
func TestCodeGenerator_GenUnary(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Create negation expression: -5
	operand := &parser.NumberExpr{Value: 5.0, Int: 5, IsInt: true}
	expr := &parser.UnOpExpr{
		Op:   "-",
		Expr: operand,
	}

	_ = cg.genUnOp(expr)

	// Should have: LOADI, UNM
	if len(cg.Prototype.Code) < 2 {
		t.Fatalf("Expected at least 2 instructions, got %d", len(cg.Prototype.Code))
	}

	// Last instruction should be UNM
	lastInstr := vm.Instruction(cg.Prototype.Code[len(cg.Prototype.Code)-1])
	if lastInstr.Opcode() != vm.OP_UNM {
		t.Errorf("Expected OP_UNM, got %v", lastInstr.Opcode())
	}
}

// TestCodeGenerator_GenTable tests table constructor generation.
func TestCodeGenerator_GenTable(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Create simple table: {1, 2, 3}
	expr := &parser.TableExpr{
		Entries: []parser.TableEntry{
			{Kind: parser.TableEntryValue, Value: &parser.NumberExpr{Value: 1.0, Int: 1, IsInt: true}},
			{Kind: parser.TableEntryValue, Value: &parser.NumberExpr{Value: 2.0, Int: 2, IsInt: true}},
			{Kind: parser.TableEntryValue, Value: &parser.NumberExpr{Value: 3.0, Int: 3, IsInt: true}},
		},
	}

	_ = cg.genTable(expr)

	// Should have: NEWTABLE, SETI, SETI, SETI
	if len(cg.Prototype.Code) < 4 {
		t.Fatalf("Expected at least 4 instructions, got %d", len(cg.Prototype.Code))
	}

	// First instruction should be NEWTABLE
	firstInstr := vm.Instruction(cg.Prototype.Code[0])
	if firstInstr.Opcode() != vm.OP_NEWTABLE {
		t.Errorf("Expected OP_NEWTABLE, got %v", firstInstr.Opcode())
	}
}

// TestCodeGenerator_ScopeManagement tests scope management.
func TestCodeGenerator_ScopeManagement(t *testing.T) {
	cg := NewCodeGenerator()

	// Outer scope
	cg.beginScope()
	cg.addLocal("x", 0, false)

	// Inner scope
	cg.beginScope()
	cg.addLocal("y", 1, false)

	// Should find both
	_, okX := cg.getLocal("x")
	_, okY := cg.getLocal("y")

	if !okX {
		t.Error("Should find 'x' in outer scope")
	}
	if !okY {
		t.Error("Should find 'y' in inner scope")
	}

	// End inner scope
	cg.endScope()

	// Should only find x
	_, okX = cg.getLocal("x")
	_, okY = cg.getLocal("y")

	if !okX {
		t.Error("Should still find 'x'")
	}
	if okY {
		t.Error("Should not find 'y' after scope end")
	}

	// End outer scope
	cg.endScope()

	_, okX = cg.getLocal("x")
	if okX {
		t.Error("Should not find 'x' after all scopes end")
	}
}

// TestCodeGenerator_ConstantDeduplication tests constant deduplication.
func TestCodeGenerator_ConstantDeduplication(t *testing.T) {
	cg := NewCodeGenerator()

	// Add same constant twice
	idx1 := cg.addOrGetConstant(*object.NewNumber(42.0))
	idx2 := cg.addOrGetConstant(*object.NewNumber(42.0))

	if idx1 != idx2 {
		t.Errorf("Expected same index for duplicate constant, got %d and %d", idx1, idx2)
	}

	if len(cg.Prototype.Constants) != 1 {
		t.Errorf("Expected 1 constant after deduplication, got %d", len(cg.Prototype.Constants))
	}
}

// TestCodeGenerator_MaxStackSize tests max stack size tracking.
func TestCodeGenerator_MaxStackSize(t *testing.T) {
	cg := NewCodeGenerator()

	// Allocate several registers
	cg.allocRegister()
	cg.allocRegister()
	cg.allocRegister()

	if cg.MaxStackSize != 3 {
		t.Errorf("Expected MaxStackSize 3, got %d", cg.MaxStackSize)
	}

	// Free and reallocate
	cg.freeRegister()
	cg.allocRegister()

	if cg.MaxStackSize != 3 {
		t.Errorf("Expected MaxStackSize to remain 3, got %d", cg.MaxStackSize)
	}
}

// TestCodeGenerator_GenLocal tests local variable declaration.
func TestCodeGenerator_GenLocal(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	stmt := &parser.LocalStmt{
		Names: []*parser.VarExpr{
			{Name: "x"},
			{Name: "y"},
		},
		Values: []parser.Expr{
			&parser.NumberExpr{Value: 1.0, Int: 1, IsInt: true},
			&parser.NumberExpr{Value: 2.0, Int: 2, IsInt: true},
		},
	}

	cg.genLocal(stmt)

	// Should have generated code for values and moves
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected some instructions to be generated")
	}

	// Verify locals were added
	_, ok := cg.getLocal("x")
	if !ok {
		t.Error("Local 'x' should be added")
	}

	_, ok = cg.getLocal("y")
	if !ok {
		t.Error("Local 'y' should be added")
	}
}

// TestCodeGenerator_GenReturn tests return statement generation.
func TestCodeGenerator_GenReturn(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	stmt := &parser.ReturnStmt{
		Values: []parser.Expr{
			&parser.NumberExpr{Value: 42.0, Int: 42, IsInt: true},
		},
	}

	cg.genReturn(stmt)

	// Should have generated code for value and RETURN
	if len(cg.Prototype.Code) < 2 {
		t.Fatalf("Expected at least 2 instructions, got %d", len(cg.Prototype.Code))
	}

	// Last instruction should be RETURN
	lastInstr := vm.Instruction(cg.Prototype.Code[len(cg.Prototype.Code)-1])
	if lastInstr.Opcode() != vm.OP_RETURN {
		t.Errorf("Expected OP_RETURN, got %v", lastInstr.Opcode())
	}
}

// TestCodeGenerator_GenIf tests if statement generation.
func TestCodeGenerator_GenIf(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	stmt := &parser.IfStmt{
		Cond: &parser.BooleanExpr{Value: true},
		Then: &parser.BlockStmt{
			Stmts: []parser.Stmt{
				&parser.ExprStmt{Expr: &parser.NumberExpr{Value: 1.0}},
			},
		},
	}

	cg.genIf(stmt)

	// Should have generated conditional code
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenWhile tests while loop generation.
func TestCodeGenerator_GenWhile(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	stmt := &parser.WhileStmt{
		Cond: &parser.BooleanExpr{Value: true},
		Body: &parser.BlockStmt{
			Stmts: []parser.Stmt{
				&parser.ExprStmt{Expr: &parser.NumberExpr{Value: 1.0}},
			},
		},
	}

	cg.genWhile(stmt)

	// Should have generated loop code with JMP
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}

	// Should have at least one JMP for loop back
	hasJMP := false
	for _, instr := range cg.Prototype.Code {
		if vm.Instruction(instr).Opcode() == vm.OP_JMP {
			hasJMP = true
			break
		}
	}

	if !hasJMP {
		t.Error("Expected JMP instruction for loop")
	}
}

// TestCodeGenerator_GenAssign tests assignment statement generation.
func TestCodeGenerator_GenAssign(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Add a local variable first
	cg.addLocal("x", 0, false)

	stmt := &parser.AssignStmt{
		Left: []parser.Expr{
			&parser.VarExpr{Name: "x"},
		},
		Right: []parser.Expr{
			&parser.NumberExpr{Value: 42.0, Int: 42, IsInt: true},
		},
	}

	cg.genAssign(stmt)

	// Should have generated assignment code
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_Integration tests a simple integration scenario.
func TestCodeGenerator_Integration(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Simulate: local x = 10; local y = 20; return x + y

	// local x = 10
	local1 := &parser.LocalStmt{
		Names:  []*parser.VarExpr{{Name: "x"}},
		Values: []parser.Expr{&parser.NumberExpr{Value: 10.0, Int: 10, IsInt: true}},
	}
	cg.genLocal(local1)

	// local y = 20
	local2 := &parser.LocalStmt{
		Names:  []*parser.VarExpr{{Name: "y"}},
		Values: []parser.Expr{&parser.NumberExpr{Value: 20.0, Int: 20, IsInt: true}},
	}
	cg.genLocal(local2)

	// return x + y
	xVar := &parser.VarExpr{Name: "x"}
	yVar := &parser.VarExpr{Name: "y"}
	addExpr := &parser.BinOpExpr{Left: xVar, Op: "+", Right: yVar}
	returnStmt := &parser.ReturnStmt{Values: []parser.Expr{addExpr}}
	cg.genReturn(returnStmt)

	// Verify code was generated
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected code to be generated")
	}

	if cg.Prototype.MaxStackSize == 0 {
		t.Error("Expected MaxStackSize to be set")
	}
}

// TestCodeGenerator_GenFunc tests function generation.
func TestCodeGenerator_GenFunc(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Create a simple function expression: function() return 42 end
	funcExpr := &parser.FuncExpr{
		Params: []*parser.VarExpr{},
		Body: &parser.BlockStmt{
			Stmts: []parser.Stmt{
				&parser.ReturnStmt{
					Values: []parser.Expr{
						&parser.NumberExpr{Value: 42.0, Int: 42, IsInt: true},
					},
				},
			},
		},
	}

	_ = cg.genFunc(funcExpr)

	// Should have generated CLOSURE instruction
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected CLOSURE instruction")
	}

	lastInstr := vm.Instruction(cg.Prototype.Code[len(cg.Prototype.Code)-1])
	if lastInstr.Opcode() != vm.OP_CLOSURE {
		t.Errorf("Expected OP_CLOSURE, got %v", lastInstr.Opcode())
	}

	// Should have nested prototype
	if len(cg.Prototype.Prototypes) != 1 {
		t.Errorf("Expected 1 nested prototype, got %d", len(cg.Prototype.Prototypes))
	}
}
// TestCodeGenerator_GenIndex tests index expression generation.
func TestCodeGenerator_GenIndex(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Create index expression: t[1]
	table := &parser.VarExpr{Name: "t"}
	index := &parser.NumberExpr{Value: 1.0, Int: 1, IsInt: true}
	expr := &parser.IndexExpr{
		Table: table,
		Index: index,
	}

	_ = cg.genIndex(expr)

	// Should have generated code
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenField tests field expression generation.
func TestCodeGenerator_GenField(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Create field expression: t.field
	table := &parser.VarExpr{Name: "t"}
	expr := &parser.FieldExpr{
		Table: table,
		Field: "field",
	}

	_ = cg.genField(expr)

	// Should have generated code
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenCall tests function call generation.
func TestCodeGenerator_GenCall(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Create call expression: func(1, 2)
	funcExpr := &parser.VarExpr{Name: "func"}
	args := []parser.Expr{
		&parser.NumberExpr{Value: 1.0, Int: 1, IsInt: true},
		&parser.NumberExpr{Value: 2.0, Int: 2, IsInt: true},
	}
	expr := &parser.CallExpr{
		Func: funcExpr,
		Args: args,
	}

	_ = cg.genCall(expr)

	// Should have generated CALL instruction
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenMethodCall tests method call generation.
func TestCodeGenerator_GenMethodCall(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Create method call: obj:method(1, 2)
	obj := &parser.VarExpr{Name: "obj"}
	args := []parser.Expr{
		&parser.NumberExpr{Value: 1.0, Int: 1, IsInt: true},
		&parser.NumberExpr{Value: 2.0, Int: 2, IsInt: true},
	}
	expr := &parser.MethodCallExpr{
		Object: obj,
		Method: "method",
		Args:   args,
	}

	_ = cg.genMethodCall(expr)

	// Should have generated code
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenAnd tests logical AND generation.
func TestCodeGenerator_GenAnd(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Create AND expression: a and b
	left := &parser.VarExpr{Name: "a"}
	right := &parser.VarExpr{Name: "b"}
	expr := &parser.BinOpExpr{
		Left:  left,
		Op:    "and",
		Right: right,
	}

	_ = cg.genAnd(expr)

	// Should have generated code with jumps
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenOr tests logical OR generation.
func TestCodeGenerator_GenOr(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Create OR expression: a or b
	left := &parser.VarExpr{Name: "a"}
	right := &parser.VarExpr{Name: "b"}
	expr := &parser.BinOpExpr{
		Left:  left,
		Op:    "or",
		Right: right,
	}

	_ = cg.genOr(expr)

	// Should have generated code with jumps
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenRepeat tests repeat loop generation.
func TestCodeGenerator_GenRepeat(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	stmt := &parser.RepeatStmt{
		Cond: &parser.BooleanExpr{Value: false},
		Body: &parser.BlockStmt{
			Stmts: []parser.Stmt{
				&parser.ExprStmt{Expr: &parser.NumberExpr{Value: 1.0}},
			},
		},
	}

	cg.genRepeat(stmt)

	// Should have generated loop code
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenForNumeric tests numeric for loop generation.
func TestCodeGenerator_GenForNumeric(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	stmt := &parser.ForNumericStmt{
		Var:  &parser.VarExpr{Name: "i"},
		From: &parser.NumberExpr{Value: 1.0, Int: 1, IsInt: true},
		To:   &parser.NumberExpr{Value: 10.0, Int: 10, IsInt: true},
		Step: nil,
		Body: &parser.BlockStmt{
			Stmts: []parser.Stmt{
				&parser.ExprStmt{Expr: &parser.NumberExpr{Value: 1.0}},
			},
		},
	}

	cg.genForNumeric(stmt)

	// Should have FORPREP and FORLOOP instructions
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}

	hasForPrep := false
	hasForLoop := false
	for _, instr := range cg.Prototype.Code {
		op := vm.Instruction(instr).Opcode()
		if op == vm.OP_FORPREP {
			hasForPrep = true
		}
		if op == vm.OP_FORLOOP {
			hasForLoop = true
		}
	}

	if !hasForPrep {
		t.Error("Expected FORPREP instruction")
	}
	if !hasForLoop {
		t.Error("Expected FORLOOP instruction")
	}
}

// TestCodeGenerator_GenForGeneric tests generic for loop generation.
func TestCodeGenerator_GenForGeneric(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	stmt := &parser.ForGenericStmt{
		Vars: []*parser.VarExpr{{Name: "k"}, {Name: "v"}},
		Exprs: []parser.Expr{
			&parser.VarExpr{Name: "pairs"},
			&parser.VarExpr{Name: "t"},
		},
		Body: &parser.BlockStmt{
			Stmts: []parser.Stmt{
				&parser.ExprStmt{Expr: &parser.NumberExpr{Value: 1.0}},
			},
		},
	}

	cg.genForGeneric(stmt)

	// Should have FORGPREP and FORGLOOP instructions
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}

	hasForGPrep := false
	hasForGLoop := false
	for _, instr := range cg.Prototype.Code {
		op := vm.Instruction(instr).Opcode()
		if op == vm.OP_FORGPREP {
			hasForGPrep = true
		}
		if op == vm.OP_FORGLOOP {
			hasForGLoop = true
		}
	}

	if !hasForGPrep {
		t.Error("Expected FORGPREP instruction")
	}
	if !hasForGLoop {
		t.Error("Expected FORGLOOP instruction")
	}
}

// TestCodeGenerator_GenBreak tests break statement generation.
func TestCodeGenerator_GenBreak(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Need to be in a loop context
	cg.breakList = append(cg.breakList, 0)

	stmt := &parser.BreakStmt{}
	cg.genBreak(stmt)

	// Should have generated JMP instruction
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenFuncDef tests function definition statement generation.
func TestCodeGenerator_GenFuncDef(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	stmt := &parser.FuncDefStmt{
		Name: []*parser.VarExpr{{Name: "f"}},
		Func: &parser.FuncExpr{
			Params: []*parser.VarExpr{},
			Body: &parser.BlockStmt{
				Stmts: []parser.Stmt{
					&parser.ReturnStmt{
						Values: []parser.Expr{
							&parser.NumberExpr{Value: 42.0, Int: 42, IsInt: true},
						},
					},
				},
			},
		},
	}

	cg.genFuncDef(stmt)

	// Should have generated code
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_FreeRegisters tests freeRegisters function.
func TestCodeGenerator_FreeRegisters(t *testing.T) {
	cg := NewCodeGenerator()

	// Allocate some registers
	cg.allocRegister()
	cg.allocRegister()
	cg.allocRegister()

	if cg.StackTop != 3 {
		t.Fatalf("Expected StackTop 3, got %d", cg.StackTop)
	}

	// Free 2 registers
	cg.freeRegisters(2)

	if cg.StackTop != 1 {
		t.Errorf("Expected StackTop 1 after freeing 2, got %d", cg.StackTop)
	}
}

// TestCodeGenerator_EmitJump tests emitJump function.
func TestCodeGenerator_EmitJump(t *testing.T) {
	cg := NewCodeGenerator()

	// Emit a jump
	pc := cg.emitJump(0)

	if pc != 0 {
		t.Errorf("Expected PC 0, got %d", pc)
	}

	if len(cg.Prototype.Code) != 1 {
		t.Errorf("Expected 1 instruction, got %d", len(cg.Prototype.Code))
	}

	instr := vm.Instruction(cg.Prototype.Code[0])
	if instr.Opcode() != vm.OP_JMP {
		t.Errorf("Expected OP_JMP, got %v", instr.Opcode())
	}
}

// TestCodeGenerator_AssignToVar tests assignToVar function.
func TestCodeGenerator_AssignToVar(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Add a local variable
	cg.addLocal("x", 0, false)

	// Create variable expression
	varExpr := &parser.VarExpr{Name: "x"}
	valueReg := cg.allocRegister()

	cg.assignToVar(varExpr, valueReg)

	// Should have generated MOVE instruction
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenExpr tests genExpr dispatch.
func TestCodeGenerator_GenExpr(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Test with different expression types
	exprs := []parser.Expr{
		&parser.NumberExpr{Value: 42.0, Int: 42, IsInt: true},
		&parser.StringExpr{Value: "hello"},
		&parser.BooleanExpr{Value: true},
		&parser.NilExpr{},
	}

	for _, expr := range exprs {
		cg2 := NewCodeGenerator()
		cg2.beginScope()
		reg := cg2.genExpr(expr)
		if reg < 0 {
			t.Errorf("genExpr returned negative register for %T", expr)
		}
	}
}

// TestCodeGenerator_GenStmt tests genStmt dispatch.
func TestCodeGenerator_GenStmt(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Test with different statement types
	stmts := []parser.Stmt{
		&parser.ExprStmt{Expr: &parser.NumberExpr{Value: 1.0}},
		&parser.LocalStmt{
			Names:  []*parser.VarExpr{{Name: "x"}},
			Values: []parser.Expr{&parser.NumberExpr{Value: 1.0}},
		},
	}

	for _, stmt := range stmts {
		cg2 := NewCodeGenerator()
		cg2.beginScope()
		cg2.genStmt(stmt)
		// Just verify it doesn't panic
	}
}

// TestCodeGenerator_BoolToInt tests boolToInt helper.
func TestCodeGenerator_BoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("boolToInt(true) should return 1")
	}
	if boolToInt(false) != 0 {
		t.Error("boolToInt(false) should return 0")
	}
}

// TestCodeGenerator_GenBinOp tests genBinOp dispatch.
func TestCodeGenerator_GenBinOp(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Test different binary operators
	ops := []string{"+", "-", "*", "/", "%", "^", "//", "..", "==", "~=", "<=", ">=", "<", ">", "&", "|", "~", "<<", ">>"}

	for _, op := range ops {
		cg2 := NewCodeGenerator()
		cg2.beginScope()

		left := &parser.NumberExpr{Value: 1.0, Int: 1, IsInt: true}
		right := &parser.NumberExpr{Value: 2.0, Int: 2, IsInt: true}
		expr := &parser.BinOpExpr{
			Left:  left,
			Op:    op,
			Right: right,
		}

		reg := cg2.genBinOp(expr)
		if reg < 0 {
			t.Errorf("genBinOp returned negative register for op %s", op)
		}
	}
}

// TestCodeGenerator_GenUnOp tests genUnOp with all operators.
func TestCodeGenerator_GenUnOp(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	ops := []string{"-", "not", "#", "~"}

	for _, op := range ops {
		cg2 := NewCodeGenerator()
		cg2.beginScope()

		operand := &parser.NumberExpr{Value: 5.0, Int: 5, IsInt: true}
		expr := &parser.UnOpExpr{
			Op:   op,
			Expr: operand,
		}

		reg := cg2.genUnOp(expr)
		if reg < 0 {
			t.Errorf("genUnOp returned negative register for op %s", op)
		}
	}
}

// TestCodeGenerator_GenTableComprehensive tests comprehensive table generation.
func TestCodeGenerator_GenTableComprehensive(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Create table with mixed entries
	expr := &parser.TableExpr{
		Entries: []parser.TableEntry{
			{Kind: parser.TableEntryValue, Value: &parser.NumberExpr{Value: 1.0}},
			{Kind: parser.TableEntryKey, Key: &parser.StringExpr{Value: "key"}, Value: &parser.NumberExpr{Value: 2.0}},
			{Kind: parser.TableEntryField, Key: &parser.StringExpr{Value: "field"}, Value: &parser.NumberExpr{Value: 3.0}},
		},
	}

	_ = cg.genTable(expr)

	// Should have generated code
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenIfElse tests if-else statement generation.
func TestCodeGenerator_GenIfElse(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	stmt := &parser.IfStmt{
		Cond: &parser.BooleanExpr{Value: true},
		Then: &parser.BlockStmt{
			Stmts: []parser.Stmt{
				&parser.ExprStmt{Expr: &parser.NumberExpr{Value: 1.0}},
			},
		},
		Else: &parser.BlockStmt{
			Stmts: []parser.Stmt{
				&parser.ExprStmt{Expr: &parser.NumberExpr{Value: 2.0}},
			},
		},
	}

	cg.genIf(stmt)

	// Should have generated code
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GenIfElseIf tests if-elseif-else statement generation.
func TestCodeGenerator_GenIfElseIf(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	stmt := &parser.IfStmt{
		Cond: &parser.BooleanExpr{Value: true},
		Then: &parser.BlockStmt{
			Stmts: []parser.Stmt{
				&parser.ExprStmt{Expr: &parser.NumberExpr{Value: 1.0}},
			},
		},
		ElseIf: []parser.ElseIfClause{
			{
				Cond: &parser.BooleanExpr{Value: false},
				Then: &parser.BlockStmt{
					Stmts: []parser.Stmt{
						&parser.ExprStmt{Expr: &parser.NumberExpr{Value: 2.0}},
					},
				},
			},
		},
		Else: &parser.BlockStmt{
			Stmts: []parser.Stmt{
				&parser.ExprStmt{Expr: &parser.NumberExpr{Value: 3.0}},
			},
		},
	}

	cg.genIf(stmt)

	// Should have generated code
	if len(cg.Prototype.Code) == 0 {
		t.Fatal("Expected instructions to be generated")
	}
}

// TestCodeGenerator_GetLocalNotFound tests getLocal when variable not found.
func TestCodeGenerator_GetLocalNotFound(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	_, ok := cg.getLocal("nonexistent")
	if ok {
		t.Error("getLocal should return false for nonexistent variable")
	}
}

// TestCodeGenerator_EndScopeNoLocals tests endScope with no locals.
func TestCodeGenerator_EndScopeNoLocals(t *testing.T) {
	cg := NewCodeGenerator()
	cg.beginScope()

	// Don't add any locals, just end scope
	cg.endScope()

	// Should not panic
}

// TestCodeGenerator_SetStackTop tests setStackTop function.
func TestCodeGenerator_SetStackTop(t *testing.T) {
	cg := NewCodeGenerator()

	// Allocate some registers
	cg.allocRegister()
	cg.allocRegister()

	// Set stack top lower
	cg.setStackTop(1)

	if cg.StackTop != 1 {
		t.Errorf("Expected StackTop 1, got %d", cg.StackTop)
	}
}

// TestCodeGenerator_ConstantKey tests constantKey function coverage.
func TestCodeGenerator_ConstantKey(t *testing.T) {
	cg := NewCodeGenerator()

	// Test with different value types
	values := []object.TValue{
		*object.NewNumber(42.0),
		*object.NewString("hello"),
		*object.NewBoolean(true),
		*object.NewNil(),
	}

	for _, val := range values {
		key := cg.constantKey(val)
		if key == "" {
			t.Errorf("constantKey returned empty string for %T", val)
		}
	}
}

// TestCodeGenerator_PatchInstructionBounds tests patching with bounds checking.
func TestCodeGenerator_PatchInstructionBounds(t *testing.T) {
	cg := NewCodeGenerator()

	// Emit an instruction
	cg.EmitABC(vm.OP_ADD, 1, 2, 3)

	// Try to patch out of bounds (should not panic)
	cg.PatchInstruction(100, 0)

	// Patch valid PC
	cg.PatchInstruction(0, object.Instruction(vm.MakeABC(vm.OP_SUB, 1, 2, 3)))

	// Verify the patch
	instr := object.Instruction(cg.Prototype.Code[0])
	if instr.Opcode() != uint8(vm.OP_SUB) {
		t.Errorf("Expected OP_SUB after patch, got %v", instr.Opcode())
	}
}

// TestCodeGenerator_GetCurrentPC tests GetCurrentPC function.
func TestCodeGenerator_GetCurrentPC(t *testing.T) {
	cg := NewCodeGenerator()

	pc := cg.GetCurrentPC()
	if pc != 0 {
		t.Errorf("Expected PC 0, got %d", pc)
	}

	cg.EmitABC(vm.OP_ADD, 1, 2, 3)

	pc = cg.GetCurrentPC()
	if pc != 1 {
		t.Errorf("Expected PC 1, got %d", pc)
	}
}
