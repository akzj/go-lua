// Package internal implements tests for the VM execution engine.
package internal

import (
	"fmt"
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

// =============================================================================
// For Loop Opcode Tests
// =============================================================================

func TestForLoopOpcodes(t *testing.T) {
	exec := newTestExecutor()
	exec.frames = append(exec.frames, &Frame{base: 0})
	exec.code = make([]opcodes.Instruction, 100)

	// Test OP_FORPREP - should add offset to pc (jump forward for setup)
	exec.pc = 10
	inst := createAsBx(opcodes.OP_FORPREP, 0, 5)
	exec.Execute(inst)

	// FORPREP should add 5 to pc (jumping to the loop body)
	if exec.pc != 15 {
		t.Errorf("OP_FORPREP pc after execution = %d, want 15", exec.pc)
	}

	// Test OP_FORPREP with negative offset
	exec.pc = 20
	inst = createAsBx(opcodes.OP_FORPREP, 0, -3)
	exec.Execute(inst)

	if exec.pc != 17 {
		t.Errorf("OP_FORPREP negative pc = %d, want 17", exec.pc)
	}

	// Test OP_TFORPREP (generic for loop prep)
	exec.pc = 30
	inst = createAsBx(opcodes.OP_TFORPREP, 0, 10)
	exec.Execute(inst)

	if exec.pc != 40 {
		t.Errorf("OP_TFORPREP pc = %d, want 40", exec.pc)
	}
}

// =============================================================================
// Control Flow Additional Tests
// =============================================================================

func TestControlFlowFurther(t *testing.T) {
	exec := newTestExecutor()
	exec.frames = append(exec.frames, &Frame{base: 0})
	exec.code = make([]opcodes.Instruction, 100)

	// Test multiple sequential JMPs
	exec.pc = 0
	inst := createAsBx(opcodes.OP_JMP, 0, 5)
	exec.Execute(inst)
	if exec.pc != 5 {
		t.Errorf("First JMP pc = %d, want 5", exec.pc)
	}

	exec.pc = 5
	inst = createAsBx(opcodes.OP_JMP, 0, 10)
	exec.Execute(inst)
	if exec.pc != 15 {
		t.Errorf("Second JMP pc = %d, want 15", exec.pc)
	}

	// Test JMP from different starting positions
	exec.pc = 100
	inst = createAsBx(opcodes.OP_JMP, 0, -50)
	exec.Execute(inst)
	if exec.pc != 50 {
		t.Errorf("Negative JMP pc = %d, want 50", exec.pc)
	}
}

// =============================================================================
// Frame and Stack Tests
// =============================================================================

func TestFrameStackIntegrity(t *testing.T) {
	exec := newTestExecutor()
	exec.frames = append(exec.frames, &Frame{base: 0})
	exec.frames = append(exec.frames, &Frame{base: 10, prev: exec.frames[0]})

	// Test currentFrame returns correct frame
	frame := exec.currentFrame()
	if frame.Base() != 10 {
		t.Errorf("currentFrame Base = %d, want 10", frame.Base())
	}

	// Test frameBase with multiple frames
	base := frameBase(exec)
	if base != 10 {
		t.Errorf("frameBase = %d, want 10", base)
	}
}

// =============================================================================
// Run Method Tests
// =============================================================================

func TestRunMethod(t *testing.T) {
	exec := NewExecutor().(*Executor)

	// Test Run with no code - should complete without error
	err := exec.Run()
	if err != nil {
		t.Errorf("Run() with no code returned error: %v", err)
	}

	// Test Run with code but no frames
	exec.code = []opcodes.Instruction{createAsBx(opcodes.OP_JMP, 0, 1)}
	exec.pc = 0
	err = exec.Run()
	// Should stop when pc reaches end of code
	if err != nil {
		t.Errorf("Run() with code returned unexpected error: %v", err)
	}
}

func TestExecuteNext(t *testing.T) {
	exec := newTestExecutor()
	exec.frames = append(exec.frames, &Frame{base: 0})
	exec.code = []opcodes.Instruction{
		createAsBx(opcodes.OP_JMP, 0, 1), // at idx 0, will set pc=2 after
		createAsBx(opcodes.OP_JMP, 0, 1), // at idx 1, will set pc=3 after
		createAsBx(opcodes.OP_JMP, 0, 0), // self-loop
	}
	exec.pc = 0

	// Execute first instruction: executeNext increments pc to 1, then JMP(1) adds 1 -> pc=2
	more := exec.executeNext()
	if !more {
		t.Error("executeNext should return true for valid instruction")
	}
	if exec.pc != 2 {
		t.Errorf("After first JMP pc = %d, want 2", exec.pc)
	}

	// Execute second instruction: pc increments to 2, then JMP(1) adds 1 -> pc=3
	more = exec.executeNext()
	if !more {
		t.Error("executeNext should return true for valid instruction")
	}
	if exec.pc != 3 {
		t.Errorf("After second JMP pc = %d, want 3", exec.pc)
	}

	// Test executeNext when pc >= len(code) - should return false
	exec.pc = len(exec.code)
	more = exec.executeNext()
	if more {
		t.Error("executeNext should return false when pc >= len(code)")
	}

	// Test executeNext with no frames - should return false
	exec2 := NewExecutor().(*Executor)
	exec2.code = []opcodes.Instruction{createAsBx(opcodes.OP_JMP, 0, 1)}
	more = exec2.executeNext()
	if more {
		t.Error("executeNext should return false when no frames")
	}
}

// =============================================================================
// Executor State Tests
// =============================================================================

func TestExecutorState(t *testing.T) {
	exec := NewExecutor().(*Executor)

	// Test initial state
	if exec.err != nil {
		t.Error("new executor should have nil error")
	}

	// Test error state
	exec.err = fmt.Errorf("test error")
	if err := exec.Run(); err == nil || err.Error() != "test error" {
		t.Errorf("Run should return stored error, got: %v", err)
	}

	// Test stack is initialized
	if len(exec.stack) == 0 {
		t.Error("stack should be initialized with capacity")
	}
}
