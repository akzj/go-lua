// Package vm tests
package vm

import (
	"testing"

	"github.com/akzj/go-lua/pkg/object"
	"github.com/akzj/go-lua/pkg/state"
)

func TestRKResolution(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	// Set up stack with test values
	vm.Stack[vm.Base+0].SetNumber(10.0)
	vm.Stack[vm.Base+1].SetNumber(20.0)
	vm.Stack[vm.Base+2].SetNumber(30.0)

	// Set up constants
	vm.Prototype = &object.Prototype{
		Constants: []object.TValue{
			*object.NewNumber(100.0),
			*object.NewNumber(200.0),
		},
	}

	// Test register access (index < 256)
	val := vm.getRKValue(0)
	if !val.IsNumber() || val.Value.Num != 10.0 {
		t.Errorf("Expected register 0 to be 10.0, got %v", val.Value.Num)
	}

	val = vm.getRKValue(1)
	if !val.IsNumber() || val.Value.Num != 20.0 {
		t.Errorf("Expected register 1 to be 20.0, got %v", val.Value.Num)
	}

	// Test constant access (index >= 256)
	val = vm.getRKValue(256) // K(0)
	if !val.IsNumber() || val.Value.Num != 100.0 {
		t.Errorf("Expected constant 0 to be 100.0, got %v", val.Value.Num)
	}

	val = vm.getRKValue(257) // K(1)
	if !val.IsNumber() || val.Value.Num != 200.0 {
		t.Errorf("Expected constant 1 to be 200.0, got %v", val.Value.Num)
	}
}

func TestMoveInstruction(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	vm.Stack[vm.Base+0].SetNumber(42.0)
	vm.Prototype = &object.Prototype{}

	// MOVE R(2), R(0)
	instr := MakeABC(OP_MOVE, 2, 0, 0)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("MOVE instruction failed: %v", err)
	}

	if !vm.Stack[vm.Base+2].IsNumber() || vm.Stack[vm.Base+2].Value.Num != 42.0 {
		t.Errorf("Expected R(2) to be 42.0, got %v", vm.Stack[vm.Base+2].Value.Num)
	}
}

func TestLoadInstructions(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{
		Constants: []object.TValue{
			*object.NewNumber(100.0),
			*object.NewString("hello"),
		},
	}

	// LOADI R(0), 42
	instr := MakeAsBx(OP_LOADI, 0, 42)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LOADI instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+0].IsNumber() || vm.Stack[vm.Base+0].Value.Num != 42.0 {
		t.Errorf("Expected R(0) to be 42.0, got %v", vm.Stack[vm.Base+0].Value.Num)
	}

	// LOADK R(1), K(0)
	instr = MakeABx(OP_LOADK, 1, 0)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LOADK instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+1].IsNumber() || vm.Stack[vm.Base+1].Value.Num != 100.0 {
		t.Errorf("Expected R(1) to be 100.0, got %v", vm.Stack[vm.Base+1].Value.Num)
	}

	// LOADNIL R(2), R(3)
	instr = MakeABC(OP_LOADNIL, 2, 3, 0)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LOADNIL instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+2].IsNil() || !vm.Stack[vm.Base+3].IsNil() {
		t.Error("Expected R(2) and R(3) to be nil")
	}
}

func TestArithmeticInstructions(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	// Set up operands
	vm.Stack[vm.Base+0].SetNumber(10.0)
	vm.Stack[vm.Base+1].SetNumber(3.0)
	vm.Prototype = &object.Prototype{}

	// ADD R(2), R(0), R(1)
	instr := MakeABC(OP_ADD, 2, 0, 1)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("ADD instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+2].IsNumber() || vm.Stack[vm.Base+2].Value.Num != 13.0 {
		t.Errorf("Expected ADD result to be 13.0, got %v", vm.Stack[vm.Base+2].Value.Num)
	}

	// SUB R(3), R(0), R(1)
	instr = MakeABC(OP_SUB, 3, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("SUB instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+3].IsNumber() || vm.Stack[vm.Base+3].Value.Num != 7.0 {
		t.Errorf("Expected SUB result to be 7.0, got %v", vm.Stack[vm.Base+3].Value.Num)
	}

	// MUL R(4), R(0), R(1)
	instr = MakeABC(OP_MUL, 4, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("MUL instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+4].IsNumber() || vm.Stack[vm.Base+4].Value.Num != 30.0 {
		t.Errorf("Expected MUL result to be 30.0, got %v", vm.Stack[vm.Base+4].Value.Num)
	}

	// DIV R(5), R(0), R(1)
	instr = MakeABC(OP_DIV, 5, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("DIV instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+5].IsNumber() {
		t.Error("Expected DIV result to be a number")
	} else if vm.Stack[vm.Base+5].Value.Num < 3.33 || vm.Stack[vm.Base+5].Value.Num > 3.34 {
		t.Errorf("Expected DIV result to be ~3.33, got %v", vm.Stack[vm.Base+5].Value.Num)
	}
}

func TestComparisonInstructions(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	vm.Stack[vm.Base+0].SetNumber(10.0)
	vm.Stack[vm.Base+1].SetNumber(20.0)
	vm.Prototype = &object.Prototype{}

	// Test EQ - equal values
	vm.PC = 0
	instr := MakeABC(OP_EQ, 0, 0, 0) // if (R(0) == R(0)) ~= 0 then pc++
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("EQ instruction failed: %v", err)
	}
	if vm.PC != 1 {
		t.Errorf("Expected PC to stay at 1 (no skip), got %d", vm.PC)
	}

	// Test EQ - different values with skip
	vm.PC = 0
	instr = MakeABC(OP_EQ, 1, 0, 1) // if (R(0) == R(1)) ~= 1 then pc++
	// R(0)=10, R(1)=20, not equal, so (false ~= true) = true, should skip
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("EQ instruction failed: %v", err)
	}
	if vm.PC != 1 {
		t.Errorf("Expected PC to be 1 (skip), got %d", vm.PC)
	}

	// Test LT
	vm.PC = 0
	instr = MakeABC(OP_LT, 0, 0, 1) // if (R(0) < R(1)) ~= 0 then pc++
	// 10 < 20 is true, (true ~= false) = true, should skip
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LT instruction failed: %v", err)
	}
	if vm.PC != 1 {
		t.Errorf("Expected PC to be 1 (skip), got %d", vm.PC)
	}
}

func TestJmpInstruction(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{}
	vm.PC = 10

	// JMP +5
	instr := MakeAsBx(OP_JMP, 0, 5)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("JMP instruction failed: %v", err)
	}
	expectedPC := 15
	if vm.PC != expectedPC {
		t.Errorf("Expected PC to be %d after JMP +5, got %d", expectedPC, vm.PC)
	}

	// JMP -3
	instr = MakeAsBx(OP_JMP, 0, -3)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("JMP instruction failed: %v", err)
	}
	expectedPC = 12
	if vm.PC != expectedPC {
		t.Errorf("Expected PC to be %d after JMP -3, got %d", expectedPC, vm.PC)
	}
}

func TestTableInstructions(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{
		Constants: []object.TValue{
			*object.NewString("key"),
		},
	}

	// NEWTABLE R(0)
	instr := MakeABC(OP_NEWTABLE, 0, 0, 0)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("NEWTABLE instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+0].IsTable() {
		t.Error("Expected R(0) to be a table")
	}

	// SETTABLE R(0)[R(1)], R(2)
	vm.Stack[vm.Base+1].SetNumber(1.0)
	vm.Stack[vm.Base+2].SetNumber(100.0)
	instr = MakeABC(OP_SETTABLE, 0, 1, 2)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("SETTABLE instruction failed: %v", err)
	}

	// GETTABLE R(3), R(0), R(1)
	instr = MakeABC(OP_GETTABLE, 3, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("GETTABLE instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+3].IsNumber() || vm.Stack[vm.Base+3].Value.Num != 100.0 {
		t.Errorf("Expected GETTABLE result to be 100.0, got %v", vm.Stack[vm.Base+3].Value.Num)
	}
}

func TestStackOperations(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	// Test Push
	val := object.NewNumber(42.0)
	vm.Push(*val)
	if vm.StackTop != 1 {
		t.Errorf("Expected StackTop to be 1, got %d", vm.StackTop)
	}

	// Test Pop
	popped := vm.Pop()
	if !popped.IsNumber() || popped.Value.Num != 42.0 {
		t.Errorf("Expected popped value to be 42.0, got %v", popped.Value.Num)
	}
	if vm.StackTop != 0 {
		t.Errorf("Expected StackTop to be 0 after pop, got %d", vm.StackTop)
	}

	// Test GetStack/SetStack
	vm.Base = 0
	vm.Stack[0].SetNumber(100.0)
	stackVal := vm.GetStack(0)
	if !stackVal.IsNumber() || stackVal.Value.Num != 100.0 {
		t.Errorf("Expected GetStack(0) to be 100.0, got %v", stackVal.Value.Num)
	}

	// Positive indices are Lua 1-based from Base: slot Stack[1] is index 2 when Base=0.
	vm.SetStack(2, *object.NewNumber(200.0))
	if !vm.Stack[1].IsNumber() || vm.Stack[1].Value.Num != 200.0 {
		t.Errorf("Expected Stack[1] to be 200.0, got %v", vm.Stack[1].Value.Num)
	}
}

func TestBitwiseInstructions(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	vm.Stack[vm.Base+0].SetNumber(12.0) // 1100 in binary
	vm.Stack[vm.Base+1].SetNumber(5.0)  // 0101 in binary
	vm.Prototype = &object.Prototype{}

	// BAND R(2), R(0), R(1) - 1100 & 0101 = 0100 = 4
	instr := MakeABC(OP_BAND, 2, 0, 1)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("BAND instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+2].IsNumber() || vm.Stack[vm.Base+2].Value.Num != 4.0 {
		t.Errorf("Expected BAND result to be 4.0, got %v", vm.Stack[vm.Base+2].Value.Num)
	}

	// BOR R(3), R(0), R(1) - 1100 | 0101 = 1101 = 13
	instr = MakeABC(OP_BOR, 3, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("BOR instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+3].IsNumber() || vm.Stack[vm.Base+3].Value.Num != 13.0 {
		t.Errorf("Expected BOR result to be 13.0, got %v", vm.Stack[vm.Base+3].Value.Num)
	}

	// BNOT R(4), R(0) - ~12 = -13 (two's complement)
	instr = MakeABC(OP_BNOT, 4, 0, 0)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("BNOT instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+4].IsNumber() || vm.Stack[vm.Base+4].Value.Num != -13.0 {
		t.Errorf("Expected BNOT result to be -13.0, got %v", vm.Stack[vm.Base+4].Value.Num)
	}
}

func TestStringConcat(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	vm.Stack[vm.Base+0].SetString("Hello")
	vm.Stack[vm.Base+1].SetString(" ")
	vm.Stack[vm.Base+2].SetString("World")
	vm.Prototype = &object.Prototype{}

	// CONCAT R(3), R(0), R(2)
	instr := MakeABC(OP_CONCAT, 3, 0, 2)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("CONCAT instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+3].IsString() {
		t.Error("Expected CONCAT result to be a string")
	} else {
		s, _ := vm.Stack[vm.Base+3].ToString()
		if s != "Hello World" {
			t.Errorf("Expected CONCAT result to be 'Hello World', got '%s'", s)
		}
	}
}

func TestLenInstruction(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{}

	// Test with string
	vm.Stack[vm.Base+0].SetString("Hello")
	instr := MakeABC(OP_LEN, 1, 0, 0)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LEN instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+1].IsNumber() || vm.Stack[vm.Base+1].Value.Num != 5.0 {
		t.Errorf("Expected string length to be 5.0, got %v", vm.Stack[vm.Base+1].Value.Num)
	}

	// Test with table
	vm.Stack[vm.Base+2].SetTable(object.NewTableWithSize(3, 0))
	vm.Stack[vm.Base+2].Value.GC.(*object.Table).Array = []object.TValue{
		*object.NewNumber(1),
		*object.NewNumber(2),
		*object.NewNumber(3),
	}
	instr = MakeABC(OP_LEN, 3, 2, 0)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LEN instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+3].IsNumber() || vm.Stack[vm.Base+3].Value.Num != 3.0 {
		t.Errorf("Expected table length to be 3.0, got %v", vm.Stack[vm.Base+3].Value.Num)
	}
}

func TestNotInstruction(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{}

	// Test with true
	vm.Stack[vm.Base+0].SetBoolean(true)
	instr := MakeABC(OP_NOT, 1, 0, 0)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("NOT instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+1].IsBoolean() || vm.Stack[vm.Base+1].Value.Bool != false {
		t.Errorf("Expected NOT true to be false")
	}

	// Test with nil (should be true in Lua)
	vm.Stack[vm.Base+2].SetNil()
	instr = MakeABC(OP_NOT, 3, 2, 0)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("NOT instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+3].IsBoolean() || vm.Stack[vm.Base+3].Value.Bool != true {
		t.Errorf("Expected NOT nil to be true")
	}
}

func TestInstructionDecoding(t *testing.T) {
	// Test iABC format
	instr := MakeABC(OP_ADD, 1, 2, 3)
	if instr.Opcode() != OP_ADD {
		t.Errorf("Expected opcode to be OP_ADD, got %d", instr.Opcode())
	}
	if instr.A() != 1 {
		t.Errorf("Expected A to be 1, got %d", instr.A())
	}
	if instr.B() != 2 {
		t.Errorf("Expected B to be 2, got %d", instr.B())
	}
	if instr.C() != 3 {
		t.Errorf("Expected C to be 3, got %d", instr.C())
	}

	// Test iABx format with Bx=0
	instr = MakeABx(OP_LOADK, 5, 0)
	if instr.Opcode() != OP_LOADK {
		t.Errorf("Expected opcode to be OP_LOADK, got %d", instr.Opcode())
	}
	if instr.A() != 5 {
		t.Errorf("Expected A to be 5, got %d", instr.A())
	}
	if instr.Bx() != 0 {
		t.Errorf("Expected Bx to be 0, got %d", instr.Bx())
	}

	// Test iAsBx format with sBx=0
	instr = MakeAsBx(OP_JMP, 0, 0)
	if instr.Opcode() != OP_JMP {
		t.Errorf("Expected opcode to be OP_JMP, got %d", instr.Opcode())
	}
	if instr.SBx() != 0 {
		t.Errorf("Expected sBx to be 0, got %d", instr.SBx())
	}

	// Test iAx format
	instr = MakeAx(OP_EXTRAARG, 0)
	if instr.Opcode() != OP_EXTRAARG {
		t.Errorf("Expected opcode to be OP_EXTRAARG, got %d", instr.Opcode())
	}
	if instr.Ax() != 0 {
		t.Errorf("Expected Ax to be 0, got %d", instr.Ax())
	}
}

func TestTableGetI(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{}

	// Create table with array values
	tbl := object.NewTableWithSize(5, 0)
	tbl.Array = []object.TValue{
		*object.NewNumber(10),
		*object.NewNumber(20),
		*object.NewNumber(30),
	}
	vm.Stack[vm.Base+0].SetTable(tbl)

	// GETI R(1), R(0), 2
	instr := MakeABC(OP_GETI, 1, 0, 2)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("GETI instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+1].IsNumber() || vm.Stack[vm.Base+1].Value.Num != 20.0 {
		t.Errorf("Expected GETI result to be 20.0, got %v", vm.Stack[vm.Base+1].Value.Num)
	}
}

func TestTableSetI(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{}

	// Create table
	tbl := object.NewTableWithSize(3, 0)
	vm.Stack[vm.Base+0].SetTable(tbl)
	vm.Stack[vm.Base+1].SetNumber(999.0)

	// SETI R(0), 2, R(1)
	instr := MakeABC(OP_SETI, 0, 1, 2)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("SETI instruction failed: %v", err)
	}

	// Verify
	tbl2, _ := vm.Stack[vm.Base+0].ToTable()
	val := tbl2.GetI(2)
	if val == nil || !val.IsNumber() || val.Value.Num != 999.0 {
		t.Errorf("Expected table[2] to be 999.0, got %v", val)
	}
}

func TestLoadBoolInstruction(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{}

	// LOADBOOL R(0), true, 0
	instr := MakeABC(OP_LOADBOOL, 0, 1, 0)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LOADBOOL instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+0].IsBoolean() || vm.Stack[vm.Base+0].Value.Bool != true {
		t.Errorf("Expected R(0) to be true, got %v", vm.Stack[vm.Base+0].Value.Bool)
	}

	// LOADBOOL R(1), false, 0
	instr = MakeABC(OP_LOADBOOL, 1, 0, 0)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LOADBOOL instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+1].IsBoolean() || vm.Stack[vm.Base+1].Value.Bool != false {
		t.Errorf("Expected R(1) to be false, got %v", vm.Stack[vm.Base+1].Value.Bool)
	}

	// LOADBOOL with skip (c=1)
	vm.PC = 10
	instr = MakeABC(OP_LOADBOOL, 2, 1, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LOADBOOL instruction failed: %v", err)
	}
	if vm.PC != 11 {
		t.Errorf("Expected PC to skip to 11, got %d", vm.PC)
	}
}

func TestLoadKXInstruction(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{
		Constants: []object.TValue{
			*object.NewNumber(999.0),
		},
		Code: []object.Instruction{
			object.Instruction(MakeAx(OP_EXTRAARG, 0)), // Extra arg for constant index 0
		},
	}
	vm.PC = 0

	// LOADKX R(0)
	instr := MakeABC(OP_LOADKX, 0, 0, 0)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LOADKX instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+0].IsNumber() || vm.Stack[vm.Base+0].Value.Num != 999.0 {
		t.Errorf("Expected R(0) to be 999.0, got %v", vm.Stack[vm.Base+0].Value.Num)
	}
}

func TestMoreArithmeticInstructions(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	vm.Stack[vm.Base+0].SetNumber(10.0)
	vm.Stack[vm.Base+1].SetNumber(3.0)
	vm.Prototype = &object.Prototype{}

	// MOD R(2), R(0), R(1)
	instr := MakeABC(OP_MOD, 2, 0, 1)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("MOD instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+2].IsNumber() || vm.Stack[vm.Base+2].Value.Num != 1.0 {
		t.Errorf("Expected MOD result to be 1.0, got %v", vm.Stack[vm.Base+2].Value.Num)
	}

	// POW R(3), R(0), R(1) - 10^3 = 1000
	instr = MakeABC(OP_POW, 3, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("POW instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+3].IsNumber() || vm.Stack[vm.Base+3].Value.Num != 1000.0 {
		t.Errorf("Expected POW result to be 1000.0, got %v", vm.Stack[vm.Base+3].Value.Num)
	}

	// IDIV R(4), R(0), R(1) - floor(10/3) = 3
	instr = MakeABC(OP_IDIV, 4, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("IDIV instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+4].IsNumber() || vm.Stack[vm.Base+4].Value.Num != 3.0 {
		t.Errorf("Expected IDIV result to be 3.0, got %v", vm.Stack[vm.Base+4].Value.Num)
	}

	// UNM R(5), R(0)
	instr = MakeABC(OP_UNM, 5, 0, 0)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("UNM instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+5].IsNumber() || vm.Stack[vm.Base+5].Value.Num != -10.0 {
		t.Errorf("Expected UNM result to be -10.0, got %v", vm.Stack[vm.Base+5].Value.Num)
	}

	// SHL R(6), R(0), R(1) - 10 << 3 = 80
	instr = MakeABC(OP_SHL, 6, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("SHL instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+6].IsNumber() || vm.Stack[vm.Base+6].Value.Num != 80.0 {
		t.Errorf("Expected SHL result to be 80.0, got %v", vm.Stack[vm.Base+6].Value.Num)
	}

	// SHR R(7), R(0), R(1) - 10 >> 3 = 1
	instr = MakeABC(OP_SHR, 7, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("SHR instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+7].IsNumber() || vm.Stack[vm.Base+7].Value.Num != 1.0 {
		t.Errorf("Expected SHR result to be 1.0, got %v", vm.Stack[vm.Base+7].Value.Num)
	}

	// BXOR R(8), R(0), R(1) - 10 ^ 3 = 9
	instr = MakeABC(OP_BXOR, 8, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("BXOR instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+8].IsNumber() || vm.Stack[vm.Base+8].Value.Num != 9.0 {
		t.Errorf("Expected BXOR result to be 9.0, got %v", vm.Stack[vm.Base+8].Value.Num)
	}
}

func TestComparisonInstructionsMore(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	vm.Stack[vm.Base+0].SetNumber(10.0)
	vm.Stack[vm.Base+1].SetNumber(20.0)
	vm.Prototype = &object.Prototype{
		Constants: []object.TValue{
			*object.NewNumber(15.0),
		},
	}

	// LE - less or equal
	vm.PC = 0
	instr := MakeABC(OP_LE, 0, 0, 1) // if (R(0) <= R(1)) ~= 0 then pc++
	// 10 <= 20 is true, (true ~= false) = true, should skip
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LE instruction failed: %v", err)
	}
	if vm.PC != 1 {
		t.Errorf("Expected PC to be 1 (skip), got %d", vm.PC)
	}

	// EQI - equal immediate
	vm.PC = 0
	instr = MakeABC(OP_EQI, 0, 0, 256) // if (R(0) == K(0)) ~= 0 then pc++
	// R(0)=10, K(0)=15, not equal, (false ~= false) = false, no skip
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("EQI instruction failed: %v", err)
	}
	if vm.PC != 0 {
		t.Errorf("Expected PC to be 0 (no skip), got %d", vm.PC)
	}

	// LTI - less than immediate
	vm.PC = 0
	instr = MakeABC(OP_LTI, 0, 0, 256) // if (R(0) < K(0)) ~= 0 then pc++
	// 10 < 15 is true, (true ~= false) = true, should skip
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LTI instruction failed: %v", err)
	}
	if vm.PC != 1 {
		t.Errorf("Expected PC to be 1 (skip), got %d", vm.PC)
	}

	// LEI - less or equal immediate
	vm.PC = 0
	instr = MakeABC(OP_LEI, 0, 0, 256) // if (R(0) <= K(0)) ~= 0 then pc++
	// 10 <= 15 is true, (true ~= false) = true, should skip
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("LEI instruction failed: %v", err)
	}
	if vm.PC != 1 {
		t.Errorf("Expected PC to be 1 (skip), got %d", vm.PC)
	}

	// GTI - greater than immediate
	vm.PC = 0
	instr = MakeABC(OP_GTI, 0, 1, 256) // if (R(1) > K(0)) ~= 0 then pc++
	// 20 > 15 is true, (true ~= false) = true, should skip
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("GTI instruction failed: %v", err)
	}
	if vm.PC != 1 {
		t.Errorf("Expected PC to be 1 (skip), got %d", vm.PC)
	}
}

func TestTestInstruction(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{}

	// TEST with truthy value, c=0
	// isTruthy=true, c!=0=false, true!=false=true → SKIP
	vm.Stack[vm.Base+0].SetNumber(10.0)
	vm.PC = 0
	instr := MakeABC(OP_TEST, 0, 0, 0)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("TEST instruction failed: %v", err)
	}
	if vm.PC != 1 {
		t.Errorf("Expected PC to be 1 (skip), got %d", vm.PC)
	}

	// TEST with truthy value, c=1
	// isTruthy=true, c!=0=true, true!=true=false → NO SKIP
	vm.PC = 0
	instr = MakeABC(OP_TEST, 0, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("TEST instruction failed: %v", err)
	}
	if vm.PC != 0 {
		t.Errorf("Expected PC to be 0 (no skip), got %d", vm.PC)
	}

	// TEST with falsy value (nil), c=0
	// isTruthy=false, c!=0=false, false!=false=false → NO SKIP
	vm.Stack[vm.Base+0].SetNil()
	vm.PC = 0
	instr = MakeABC(OP_TEST, 0, 0, 0)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("TEST instruction failed: %v", err)
	}
	if vm.PC != 0 {
		t.Errorf("Expected PC to be 0 (no skip), got %d", vm.PC)
	}

	// TEST with false value, c=0
	// isTruthy=false, c!=0=false, false!=false=false → NO SKIP
	vm.Stack[vm.Base+0].SetBoolean(false)
	vm.PC = 0
	instr = MakeABC(OP_TEST, 0, 0, 0)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("TEST instruction failed: %v", err)
	}
	if vm.PC != 0 {
		t.Errorf("Expected PC to be 0 (no skip), got %d", vm.PC)
	}
}

func TestCloseAndTBCInstructions(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{}
	vm.OpenUpvalues = make(map[int]*Upvalue)

	// Create an open upvalue
	upvalue := &Upvalue{Index: vm.Base + 5}
	vm.OpenUpvalues[vm.Base+5] = upvalue

	// CLOSE R(5)
	instr := MakeABC(OP_CLOSE, 5, 0, 0)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("CLOSE instruction failed: %v", err)
	}
	// Upvalue should be closed
	if upvalue.Value != nil {
		t.Error("Expected upvalue to be closed")
	}

	// TBC R(3)
	vm.TBCList = []int{}
	instr = MakeABC(OP_TBC, 3, 0, 0)
	err = vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("TBC instruction failed: %v", err)
	}
	if len(vm.TBCList) != 1 || vm.TBCList[0] != vm.Base+3 {
		t.Errorf("Expected TBC list to contain base+3, got %v", vm.TBCList)
	}
}

func TestGetFieldInstruction(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{
		Constants: []object.TValue{
			*object.NewString("key"),
		},
	}

	// Create table with field
	tbl := object.NewTable()
	tbl.Set(*object.NewString("key"), *object.NewNumber(42.0))
	vm.Stack[vm.Base+0].SetTable(tbl)

	// GETFIELD R(1), R(0), K(0)
	instr := MakeABC(OP_GETFIELD, 1, 0, 0)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("GETFIELD instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+1].IsNumber() || vm.Stack[vm.Base+1].Value.Num != 42.0 {
		t.Errorf("Expected GETFIELD result to be 42.0, got %v", vm.Stack[vm.Base+1].Value.Num)
	}
}

func TestSetFieldInstruction(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{
		Constants: []object.TValue{
			*object.NewString("field"),
		},
	}

	// Create table
	tbl := object.NewTable()
	vm.Stack[vm.Base+0].SetTable(tbl)
	vm.Stack[vm.Base+1].SetNumber(100.0)

	// SETFIELD R(0), K(0), R(1)
	instr := MakeABC(OP_SETFIELD, 0, 0, 1)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("SETFIELD instruction failed: %v", err)
	}

	// Verify
	val := tbl.Get(*object.NewString("field"))
	if val == nil || !val.IsNumber() || val.Value.Num != 100.0 {
		t.Errorf("Expected table.field to be 100.0, got %v", val)
	}
}

func TestSelfInstruction(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{
		Constants: []object.TValue{
			*object.NewString("method"),
		},
	}

	// Create table with method
	tbl := object.NewTable()
	tbl.Set(*object.NewString("method"), *object.NewNumber(999.0))
	vm.Stack[vm.Base+1].SetTable(tbl)

	// SELF R(0), R(1), K(0)
	instr := MakeABC(OP_SELF, 0, 1, 256)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("SELF instruction failed: %v", err)
	}

	// R(0) should be the method value
	if !vm.Stack[vm.Base+0].IsNumber() || vm.Stack[vm.Base+0].Value.Num != 999.0 {
		t.Errorf("Expected R(0) to be 999.0, got %v", vm.Stack[vm.Base+0].Value.Num)
	}
	// R(1) should be the table
	if !vm.Stack[vm.Base+1].IsTable() {
		t.Error("Expected R(1) to be the table")
	}
}

func TestAddIInstruction(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{}

	vm.Stack[vm.Base+0].SetNumber(10.0)

	// ADDI R(1), R(0), 5
	instr := MakeABC(OP_ADDI, 1, 0, 5)
	err := vm.ExecuteInstruction(instr)
	if err != nil {
		t.Fatalf("ADDI instruction failed: %v", err)
	}
	if !vm.Stack[vm.Base+1].IsNumber() || vm.Stack[vm.Base+1].Value.Num != 15.0 {
		t.Errorf("Expected ADDI result to be 15.0, got %v", vm.Stack[vm.Base+1].Value.Num)
	}
}

func TestTableErrors(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{}

	// Try to index a non-table
	vm.Stack[vm.Base+0].SetNumber(10.0)
	vm.Stack[vm.Base+1].SetNumber(1.0)
	vm.Stack[vm.Base+2].SetNumber(2.0)

	// GETTABLE on non-table
	instr := MakeABC(OP_GETTABLE, 3, 0, 1)
	err := vm.ExecuteInstruction(instr)
	if err == nil {
		t.Error("Expected GETTABLE on non-table to fail")
	}

	// SETTABLE on non-table
	instr = MakeABC(OP_SETTABLE, 0, 1, 2)
	err = vm.ExecuteInstruction(instr)
	if err == nil {
		t.Error("Expected SETTABLE on non-table to fail")
	}

	// GETI on non-table
	instr = MakeABC(OP_GETI, 3, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err == nil {
		t.Error("Expected GETI on non-table to fail")
	}

	// SETI on non-table
	instr = MakeABC(OP_SETI, 0, 1, 2)
	err = vm.ExecuteInstruction(instr)
	if err == nil {
		t.Error("Expected SETI on non-table to fail")
	}

	// GETFIELD on non-table
	vm.Prototype.Constants = []object.TValue{*object.NewString("key")}
	instr = MakeABC(OP_GETFIELD, 3, 0, 0)
	err = vm.ExecuteInstruction(instr)
	if err == nil {
		t.Error("Expected GETFIELD on non-table to fail")
	}

	// SETFIELD on non-table
	instr = MakeABC(OP_SETFIELD, 0, 0, 1)
	err = vm.ExecuteInstruction(instr)
	if err == nil {
		t.Error("Expected SETFIELD on non-table to fail")
	}

	// SELF on non-table
	instr = MakeABC(OP_SELF, 0, 0, 0)
	err = vm.ExecuteInstruction(instr)
	if err == nil {
		t.Error("Expected SELF on non-table to fail")
	}
}

func TestLenErrors(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Prototype = &object.Prototype{}

	// Try to get length of invalid type
	vm.Stack[vm.Base+0].SetNumber(10.0)

	instr := MakeABC(OP_LEN, 1, 0, 0)
	err := vm.ExecuteInstruction(instr)
	if err == nil {
		t.Error("Expected LEN on number to fail")
	}
}

func TestRKValue(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	// Set up test data
	vm.Stack[vm.Base+10].SetNumber(100.0)
	vm.Prototype = &object.Prototype{
		Constants: []object.TValue{
			*object.NewNumber(200.0),
		},
	}

	// Test register access
	val := vm.getRKValue(10)
	if !val.IsNumber() || val.Value.Num != 100.0 {
		t.Errorf("Expected getRKValue(10) to be 100.0, got %v", val.Value.Num)
	}

	// Test constant access
	val = vm.getRKValue(256)
	if !val.IsNumber() || val.Value.Num != 200.0 {
		t.Errorf("Expected getRKValue(256) to be 200.0, got %v", val.Value.Num)
	}
}

func TestGetStackValue(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	vm.Stack[100].SetNumber(42.0)

	val := vm.getStackValue(100)
	if !val.IsNumber() || val.Value.Num != 42.0 {
		t.Errorf("Expected getStackValue(100) to be 42.0, got %v", val.Value.Num)
	}
}

func TestDecodeSize(t *testing.T) {
	// Test decodeSize function
	tests := []struct {
		input    int
		expected int
	}{
		{0, 0},
		{1, 1},
		{10, 10},
		{100, 100},
	}

	for _, tt := range tests {
		result := decodeSize(tt.input)
		if result != tt.expected {
			t.Errorf("decodeSize(%d) = %d, expected %d", tt.input, result, tt.expected)
		}
	}
}

func TestOpcodeString(t *testing.T) {
	// Test that String() method works for all opcodes
	opcodes := []Opcode{
		OP_MOVE, OP_LOADI, OP_LOADF, OP_LOADK, OP_LOADKX,
		OP_LOADBOOL, OP_LOADNIL, OP_GETUPVAL, OP_SETUPVAL,
		OP_GETTABUP, OP_GETTABLE, OP_GETI, OP_GETFIELD,
		OP_SETTABUP, OP_SETTABLE, OP_SETI, OP_SETFIELD,
		OP_NEWTABLE, OP_SELF, OP_ADDI, OP_ADD, OP_SUB,
		OP_MUL, OP_MOD, OP_POW, OP_DIV, OP_IDIV,
		OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR,
		OP_UNM, OP_BNOT, OP_NOT, OP_LEN, OP_CONCAT,
		OP_CLOSE, OP_TBC, OP_JMP, OP_EQ, OP_LT,
		OP_LE, OP_EQI, OP_LEI, OP_LTI, OP_GTI,
		OP_TEST, OP_FORPREP, OP_FORLOOP, OP_FORGPREP, OP_FORGLOOP,
		OP_SETLIST, OP_CLOSURE, OP_VARARG, OP_VARARGPREP, OP_EXTRAARG,
	}

	for _, op := range opcodes {
		s := op.String()
		if s == "" {
			t.Errorf("Opcode %d has empty String()", op)
		}
	}
}

func TestCallInfoStruct(t *testing.T) {
	// Test CallInfo struct
	ci := &CallInfo{
		Func:     object.NewNumber(1.0),
		Base:     10,
		Top:      20,
		PC:       5,
		NResults: 2,
		Status:   CallOK,
	}

	if ci.Base != 10 {
		t.Errorf("Expected Base to be 10, got %d", ci.Base)
	}
	if ci.Top != 20 {
		t.Errorf("Expected Top to be 20, got %d", ci.Top)
	}
	if ci.PC != 5 {
		t.Errorf("Expected PC to be 5, got %d", ci.PC)
	}
}

func TestCallStatus(t *testing.T) {
	// Test CallStatus constants
	if CallOK != 0 {
		t.Errorf("Expected CallOK to be 0, got %d", CallOK)
	}
	if CallYield != 1 {
		t.Errorf("Expected CallYield to be 1, got %d", CallYield)
	}
	if CallError != 2 {
		t.Errorf("Expected CallError to be 2, got %d", CallError)
	}
}

func TestUpvalueClose(t *testing.T) {
	val := object.NewNumber(42.0)
	upvalue := &Upvalue{
		Index: 10,
		Value: val,
	}

	upvalue.Close()

	if upvalue.Value != nil {
		t.Error("Expected Value to be nil after Close")
	}
	if !upvalue.Closed.IsNumber() || upvalue.Closed.Value.Num != 42.0 {
		t.Errorf("Expected Closed to be 42.0, got %v", upvalue.Closed.Value.Num)
	}
}

func TestNewVM(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	if vm.Stack == nil {
		t.Error("Expected Stack to be initialized")
	}
	if vm.StackSize != 2048 {
		t.Errorf("Expected StackSize to be 2048, got %d", vm.StackSize)
	}
	if vm.CallInfo == nil {
		t.Error("Expected CallInfo to be initialized")
	}
	if vm.Global != global {
		t.Error("Expected Global to be set")
	}
	if vm.OpenUpvalues == nil {
		t.Error("Expected OpenUpvalues to be initialized")
	}
}

func TestPushPop(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)

	// Test Push
	val := object.NewNumber(100.0)
	vm.Push(*val)
	if vm.StackTop != 1 {
		t.Errorf("Expected StackTop to be 1, got %d", vm.StackTop)
	}
	if !vm.Stack[vm.StackTop-1].IsNumber() || vm.Stack[vm.StackTop-1].Value.Num != 100.0 {
		t.Errorf("Expected pushed value to be 100.0")
	}

	// Test Pop
	popped := vm.Pop()
	if !popped.IsNumber() || popped.Value.Num != 100.0 {
		t.Errorf("Expected popped value to be 100.0")
	}
	if vm.StackTop != 0 {
		t.Errorf("Expected StackTop to be 0 after pop, got %d", vm.StackTop)
	}
}

func TestSetStack(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Base = 0

	vm.SetStack(6, *object.NewNumber(42.0))
	if !vm.Stack[5].IsNumber() || vm.Stack[5].Value.Num != 42.0 {
		t.Errorf("Expected Stack[5] to be 42.0, got %v", vm.Stack[5].Value.Num)
	}
}

func TestGetStack(t *testing.T) {
	global := state.NewGlobalState()
	vm := NewVM(global)
	vm.Base = 0
	vm.Stack[10].SetNumber(99.0)

	val := vm.GetStack(11)
	if !val.IsNumber() || val.Value.Num != 99.0 {
		t.Errorf("Expected GetStack(11) to be 99.0, got %v", val.Value.Num)
	}
}
