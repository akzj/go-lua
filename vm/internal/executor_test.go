// Package internal implements tests for the VM execution engine.
package internal

import (
	"testing"

	opcodes "github.com/akzj/go-lua/opcodes/api"
	types "github.com/akzj/go-lua/types/api"
	vmapi "github.com/akzj/go-lua/vm/api"
)

// Helper to create an ABC instruction
func createABC(op opcodes.OpCode, a, b, c int) opcodes.Instruction {
	return opcodes.Instruction(op) |
		opcodes.Instruction(a<<opcodes.POS_A) |
		opcodes.Instruction(b<<opcodes.POS_B) |
		opcodes.Instruction(c<<opcodes.POS_C)
}

// Helper to create an AsBx instruction (for JMP, LOADI, LOADF)
func createAsBx(op opcodes.OpCode, a, sbx int) opcodes.Instruction {
	bx := sbx + opcodes.OFFSET_sBx
	return opcodes.Instruction(op) |
		opcodes.Instruction(a<<opcodes.POS_A) |
		opcodes.Instruction(bx<<opcodes.POS_Bx)
}

// newTestExecutor creates a properly initialized executor for testing
func newTestExecutor() *Executor {
	exec := NewExecutor().(*Executor)
	for i := 0; i < 32; i++ {
		exec.reg(i)
	}
	return exec
}

// =============================================================================
// Frame Interface Tests
// =============================================================================

func TestFrameInterface(t *testing.T) {
	frame := &Frame{
		base:    10,
		prev:    nil,
		savedPC: 5,
	}

	if frame.Base() != 10 {
		t.Errorf("Frame.Base() = %d, want 10", frame.Base())
	}

	if frame.PC() != 5 {
		t.Errorf("Frame.PC() = %d, want 5", frame.PC())
	}

	frame.SetPC(20)
	if frame.PC() != 20 {
		t.Errorf("Frame.SetPC(20); PC() = %d, want 20", frame.PC())
	}

	if frame.Top() != frame.base {
		t.Error("Frame.Top() should equal Frame.Base()")
	}
}

func TestFramePrevLinking(t *testing.T) {
	frame1 := &Frame{base: 0, prev: nil, savedPC: 0}
	frame2 := &Frame{base: 10, prev: frame1, savedPC: 5}

	// frame2.prev should point to frame1
	if frame2.prev != frame1 {
		t.Error("Frame2.prev should be frame1")
	}
}

// =============================================================================
// Executor Creation Tests
// =============================================================================

func TestNewExecutor(t *testing.T) {
	exec := NewExecutor()

	if exec == nil {
		t.Error("NewExecutor should return non-nil")
	}

	var _ vmapi.VMExecutor = exec
}

func TestExecutorStackGrowth(t *testing.T) {
	exec := NewExecutor().(*Executor)

	for i := 0; i < 100; i++ {
		_ = exec.reg(i)
	}

	if len(exec.stack) < 100 {
		t.Errorf("Stack should have grown to at least 100, got %d", len(exec.stack))
	}
}

// =============================================================================
// TValue Type Tests
// =============================================================================

func TestTValueTypeChecks(t *testing.T) {
	tv := &TValue{}

	tv.Tt = uint8(types.LUA_VNIL)
	if !tv.IsNil() {
		t.Error("IsNil should be true for nil value")
	}

	tv.Tt = uint8(types.LUA_VTRUE)
	if !tv.IsTrue() {
		t.Error("IsTrue should be true for true value")
	}

	tv.Tt = uint8(types.LUA_VFALSE)
	if !tv.IsFalse() {
		t.Error("IsFalse should be true for false value")
	}

	tv.Tt = uint8(types.LUA_VNUMINT)
	tv.Value.Variant = types.ValueInteger
	tv.Value.Data_ = types.LuaInteger(42)
	if !tv.IsInteger() {
		t.Error("IsInteger should be true for integer value")
	}
	if !tv.IsNumber() {
		t.Error("IsNumber should be true for integer value")
	}
	if tv.GetInteger() != 42 {
		t.Errorf("GetInteger = %d, want 42", tv.GetInteger())
	}

	tv.Tt = uint8(types.LUA_VNUMFLT)
	tv.Value.Variant = types.ValueFloat
	tv.Value.Data_ = types.LuaNumber(3.14)
	if !tv.IsFloat() {
		t.Error("IsFloat should be true for float value")
	}
	if !tv.IsNumber() {
		t.Error("IsNumber should be true for float value")
	}
}

func TestTValueIsEmpty(t *testing.T) {
	tv := &TValue{}

	tv.Tt = uint8(types.LUA_VNIL)
	if !tv.IsEmpty() {
		t.Error("IsEmpty should be true for nil")
	}

	tv.Tt = uint8(types.LUA_VTRUE)
	if tv.IsEmpty() {
		t.Error("IsEmpty should be false for true")
	}

	tv.Tt = uint8(types.LUA_VNUMINT)
	tv.Value.Variant = types.ValueInteger
	tv.Value.Data_ = types.LuaInteger(0)
	if tv.IsEmpty() {
		t.Error("IsEmpty should be false for integer")
	}
}

// =============================================================================
// Control Flow Tests
// =============================================================================

func TestControlFlowJmp(t *testing.T) {
	exec := newTestExecutor()
	exec.frames = append(exec.frames, &Frame{base: 0})
	exec.code = make([]opcodes.Instruction, 100)
	exec.pc = 10

	inst := createAsBx(opcodes.OP_JMP, 0, 5)
	exec.Execute(inst)

	if exec.pc != 15 {
		t.Errorf("OP_JMP pc after execution = %d, want 15", exec.pc)
	}
}

func TestControlFlowJmpNegative(t *testing.T) {
	exec := newTestExecutor()
	exec.frames = append(exec.frames, &Frame{base: 0})
	exec.code = make([]opcodes.Instruction, 100)
	exec.pc = 50

	inst := createAsBx(opcodes.OP_JMP, 0, -10)
	exec.Execute(inst)

	if exec.pc != 40 {
		t.Errorf("OP_JMP negative pc = %d, want 40", exec.pc)
	}
}

func TestControlFlowCall(t *testing.T) {
	exec := newTestExecutor()
	exec.frames = append(exec.frames, &Frame{base: 0})

	inst := createABC(opcodes.OP_CALL, 0, 1, 1)
	result := exec.Execute(inst)

	if result {
		t.Error("OP_CALL should return false (suspend)")
	}
}

// =============================================================================
// Opcode Names Test
// =============================================================================

func TestOpcodeNames(t *testing.T) {
	tests := []struct {
		op   opcodes.OpCode
		name string
	}{
		{opcodes.OP_MOVE, "MOVE"},
		{opcodes.OP_LOADI, "LOADI"},
		{opcodes.OP_LOADF, "LOADF"},
		{opcodes.OP_LOADK, "LOADK"},
		{opcodes.OP_LOADNIL, "LOADNIL"},
		{opcodes.OP_GETUPVAL, "GETUPVAL"},
		{opcodes.OP_GETTABLE, "GETTABLE"},
		{opcodes.OP_SETTABLE, "SETTABLE"},
		{opcodes.OP_NEWTABLE, "NEWTABLE"},
		{opcodes.OP_ADD, "ADD"},
		{opcodes.OP_SUB, "SUB"},
		{opcodes.OP_MUL, "MUL"},
		{opcodes.OP_EQ, "EQ"},
		{opcodes.OP_LT, "LT"},
		{opcodes.OP_LE, "LE"},
		{opcodes.OP_JMP, "JMP"},
		{opcodes.OP_CALL, "CALL"},
		{opcodes.OP_RETURN, "RETURN"},
	}

	for _, tt := range tests {
		name := opcodes.OpCodeName(tt.op)
		if name != tt.name {
			t.Errorf("OpCodeName(%d) = %s, want %s", tt.op, name, tt.name)
		}
	}
}

// =============================================================================
// Instruction Encoding Test
// =============================================================================

func TestInstructionEncodingABC(t *testing.T) {
	// Test that our helper creates correct ABC instructions
	inst := createABC(opcodes.OP_ADD, 10, 20, 30)

	op := vmapi.GetOpCode(inst)
	if op != opcodes.OP_ADD {
		t.Errorf("GetOpCode = %d, want OP_ADD (%d)", op, opcodes.OP_ADD)
	}

	a := vmapi.GetArgA(inst)
	if a != 10 {
		t.Errorf("GetArgA = %d, want 10", a)
	}

	b := vmapi.GetArgB(inst)
	if b != 20 {
		t.Errorf("GetArgB = %d, want 20", b)
	}

	c := vmapi.GetArgC(inst)
	if c != 30 {
		t.Errorf("GetArgC = %d, want 30", c)
	}
}

func TestInstructionEncodingAsBx(t *testing.T) {
	// Test JMP with positive offset
	inst := createAsBx(opcodes.OP_JMP, 0, 100)

	op := vmapi.GetOpCode(inst)
	if op != opcodes.OP_JMP {
		t.Errorf("GetOpCode = %d, want OP_JMP (%d)", op, opcodes.OP_JMP)
	}

	// Test that GetsBx decodes correctly
	sbx := vmapi.GetsBx(inst)
	if sbx != 100 {
		t.Errorf("GetsBx = %d, want 100", sbx)
	}

	// Test negative offset
	inst = createAsBx(opcodes.OP_JMP, 0, -50)
	sbx = vmapi.GetsBx(inst)
	if sbx != -50 {
		t.Errorf("GetsBx for negative = %d, want -50", sbx)
	}
}

// =============================================================================
// HasKBit Test
// =============================================================================

func TestHasKBit(t *testing.T) {
	// Create instruction with k bit set
	inst := opcodes.Instruction(opcodes.OP_EQ) | (1 << opcodes.POS_k)
	if !vmapi.HasKBit(inst) {
		t.Error("HasKBit should return true when k bit is set")
	}

	// Create instruction without k bit
	inst = createABC(opcodes.OP_EQ, 0, 1, 2)
	if vmapi.HasKBit(inst) {
		t.Error("HasKBit should return false when k bit is not set")
	}
}
