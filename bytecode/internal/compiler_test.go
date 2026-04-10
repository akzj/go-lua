// Package internal provides bytecode compiler tests.
package internal

import (
	"testing"

	"github.com/akzj/go-lua/bytecode/api"
)

func TestNewCompiler(t *testing.T) {
	c := NewCompiler("test")
	if c == nil {
		t.Fatal("NewCompiler returned nil")
	}
}

func TestCompilerCompileNil(t *testing.T) {
	c := NewCompiler("test")
	_, err := c.Compile(nil)
	if err == nil {
		t.Fatal("expected error for nil chunk")
	}
}

func TestPrototypeInterface(t *testing.T) {
	proto := &Prototype{
		sourceName:      "test",
		lineDefined:     1,
		lastLineDefined: 10,
		numparams:       2,
		flag:            0,
		maxstacksize:    16,
	}

	// Test interface methods
	if proto.SourceName() != "test" {
		t.Error("SourceName() wrong")
	}
	if proto.LineDefined() != 1 {
		t.Error("LineDefined() wrong")
	}
	if proto.LastLineDefined() != 10 {
		t.Error("LastLineDefined() wrong")
	}
	if proto.NumParams() != 2 {
		t.Error("NumParams() wrong")
	}
	if proto.IsVararg() {
		t.Error("IsVararg() should be false")
	}
	if proto.MaxStackSize() != 16 {
		t.Error("MaxStackSize() wrong")
	}

	// Test internal getters
	if proto.GetCode() != nil {
		t.Error("GetCode() should be nil initially")
	}
	if proto.GetConstants() != nil {
		t.Error("GetConstants() should be nil initially")
	}
}

func TestLocals(t *testing.T) {
	locals := NewLocals()
	if locals.Count() != 0 {
		t.Fatalf("expected 0 locals, got %d", locals.Count())
	}

	locals.Add("x", 0, 0)
	if locals.Count() != 1 {
		t.Fatalf("expected 1 local, got %d", locals.Count())
	}

	v := locals.Get(0)
	if v == nil || v.Name != "x" {
		t.Fatal("Get(0) returned wrong value")
	}

	// Test Get out of bounds
	if locals.Get(-1) != nil {
		t.Fatal("Get(-1) should return nil")
	}
	if locals.Get(100) != nil {
		t.Fatal("Get(100) should return nil")
	}
}

func TestConstantEquals(t *testing.T) {
	c1 := NewConstInteger(42)
	c2 := NewConstInteger(42)
	c3 := NewConstInteger(100)

	if !c1.equals(c2) {
		t.Error("same integers should be equal")
	}
	if c1.equals(c3) {
		t.Error("different integers should not be equal")
	}

	s1 := NewConstString("hello")
	s2 := NewConstString("hello")
	s3 := NewConstString("world")
	if !s1.equals(s2) {
		t.Error("same strings should be equal")
	}
	if s1.equals(s3) {
		t.Error("different strings should not be equal")
	}

	b1 := NewConstBool(true)
	b2 := NewConstBool(true)
	b3 := NewConstBool(false)
	if !b1.equals(b2) {
		t.Error("same bools should be equal")
	}
	if b1.equals(b3) {
		t.Error("different bools should not be equal")
	}
}

func TestAddConstant(t *testing.T) {
	proto := &Prototype{}
	fs := &FuncState{Proto: proto}

	idx1 := fs.addConstant(NewConstInteger(42))
	if idx1 != 0 {
		t.Errorf("first constant should be at index 0, got %d", idx1)
	}

	idx2 := fs.addConstant(NewConstInteger(100))
	if idx2 != 1 {
		t.Errorf("second constant should be at index 1, got %d", idx2)
	}

	// Duplicate should return same index
	idx3 := fs.addConstant(NewConstInteger(42))
	if idx3 != 0 {
		t.Errorf("duplicate constant should return index 0, got %d", idx3)
	}

	if len(proto.k) != 2 {
		t.Errorf("expected 2 constants, got %d", len(proto.k))
	}
}

func TestFuncStateAllocReg(t *testing.T) {
	proto := &Prototype{maxstacksize: 2}
	fs := &FuncState{Proto: proto}

	reg := fs.allocReg()
	if reg != 2 {
		t.Errorf("first alloc should return 2, got %d", reg)
	}
	if proto.maxstacksize != 3 {
		t.Errorf("maxstacksize should be 3, got %d", proto.maxstacksize)
	}

	reg2 := fs.allocReg()
	if reg2 != 3 {
		t.Errorf("second alloc should return 3, got %d", reg2)
	}
}

func TestCompileError(t *testing.T) {
	err := api.NewCompileError(1, 2, "test error")
	if err.Line != 1 || err.Column != 2 {
		t.Fatal("CompileError has wrong line/column")
	}
	if err.Error() != "test error" {
		t.Fatal("CompileError.Error() wrong")
	}
}

// =============================================================================
// Table Construction Tests
// =============================================================================

func TestTableConstruction(t *testing.T) {
	proto := &Prototype{
		sourceName:   "test",
		maxstacksize: 3,
		k:            make([]*api.Constant, 0),
		code:         make([]uint32, 0),
	}
	fs := &FuncState{Proto: proto}

	// Test NEWTABLE instruction emission
	// Format: OP_NEWTABLE A B C K (A=dest reg, B=array size, C=nhash, K=has array)
	// NEWTABLE is opcode 37 in standard Lua
	pc := fs.emit(37, 0, 0, 0)

	if pc != 0 {
		t.Errorf("first instruction should be at pc 0, got %d", pc)
	}
	if len(proto.code) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(proto.code))
	}

	// Verify NEWTABLE encoding
	expected := uint32(37) // opcode
	if proto.code[0]&0x7F != expected {
		t.Errorf("opcode mismatch: expected %d, got %d", expected, proto.code[0]&0x7F)
	}
}

func TestNewTableInstructionEncoding(t *testing.T) {
	// Test ABC instruction encoding for table operations
	proto := &Prototype{maxstacksize: 1}

	// Test encodeABC for NEWTABLE (opcode 37)
	// A=register 0, B=array size 10, C=hash size 5
	inst := encodeABC(37, 0, 10, 5)
	_ = proto.maxstacksize // suppress unused warning for proto

	// Verify opcode (bits 0-6)
	opcode := inst & 0x7F
	if opcode != 37 {
		t.Errorf("opcode should be 37, got %d", opcode)
	}

	// Verify A register (bits 7-14)
	a := (inst >> 7) & 0xFF
	if a != 0 {
		t.Errorf("A should be 0, got %d", a)
	}
}

func TestSetListInstruction(t *testing.T) {
	// Test SETLIST instruction for table field assignment
	// Format: OP_SETLIST A B C K
	proto := &Prototype{maxstacksize: 3}
	fs := &FuncState{Proto: proto}

	// Allocate registers (only tracks maxstacksize, no code emission)
	_ = fs.allocReg() // table register
	_ = fs.allocReg() // value register

	// Emit SETLIST instruction (opcode 43 in standard Lua)
	// C field holds the number of registers to set (or 0 = use next instruction)
	fs.emitABC(43, 0, 1, 0)

	// Only emitABC adds to code
	if len(proto.code) != 1 {
		t.Errorf("expected 1 SETLIST instruction, got %d", len(proto.code))
	}
}

// =============================================================================
// Function Definition Tests
// =============================================================================

func TestSubPrototypeCreation(t *testing.T) {
	parentProto := &Prototype{
		sourceName:   "parent",
		maxstacksize: 8,
		k:            make([]*api.Constant, 0),
		code:         make([]uint32, 0),
		p:            make([]*Prototype, 0),
	}

	// Create a sub-prototype (nested function)
	subProto := &Prototype{
		sourceName:      "child",
		lineDefined:     1,
		lastLineDefined: 10,
		numparams:       1,
		maxstacksize:    4,
		k:               make([]*api.Constant, 0),
		code:            make([]uint32, 0),
	}

	// Add sub-prototype to parent
	parentProto.p = append(parentProto.p, subProto)

	if len(parentProto.p) != 1 {
		t.Fatalf("expected 1 sub-prototype, got %d", len(parentProto.p))
	}
	if parentProto.p[0].SourceName() != "child" {
		t.Error("sub-prototype source name mismatch")
	}
	if parentProto.p[0].NumParams() != 1 {
		t.Error("sub-prototype numparams mismatch")
	}
}

func TestFunctionWithMultipleConstants(t *testing.T) {
	proto := &Prototype{
		sourceName:   "multiconst",
		maxstacksize: 5,
		k:            make([]*api.Constant, 0),
		code:         make([]uint32, 0),
	}
	fs := &FuncState{Proto: proto}

	// Add multiple constants of different types
	intIdx := fs.addConstant(NewConstInteger(100))
	floatIdx := fs.addConstant(NewConstFloat(3.14))
	strIdx := fs.addConstant(NewConstString("test"))

	if intIdx != 0 {
		t.Errorf("integer constant should be at index 0, got %d", intIdx)
	}
	if floatIdx != 1 {
		t.Errorf("float constant should be at index 1, got %d", floatIdx)
	}
	if strIdx != 2 {
		t.Errorf("string constant should be at index 2, got %d", strIdx)
	}

	// Emit LOADK instructions to load constants
	fs.emitABx(1, 0, intIdx)   // LOADK R(0), K(intIdx)
	fs.emitABx(1, 1, floatIdx) // LOADK R(1), K(floatIdx)
	fs.emitABx(1, 2, strIdx)   // LOADK R(2), K(strIdx)

	if len(proto.code) != 3 {
		t.Errorf("expected 3 LOADK instructions, got %d", len(proto.code))
	}
}

// =============================================================================
// Control Flow Tests
// =============================================================================

func TestJumpInstruction(t *testing.T) {
	proto := &Prototype{maxstacksize: 2}
	fs := &FuncState{Proto: proto}

	// Emit JMP instruction (opcode 44 in standard Lua)
	// Format: OP_JMP A sBx (sBx is signed offset)
	pc1 := fs.currentPC()
	jmpPc := fs.emitAsBx(44, 0, 5) // JMP +5

	// Verify JMP was emitted
	if len(proto.code) != 1 {
		t.Errorf("expected 1 instruction, got %d", len(proto.code))
	}

	// JMP should return the PC of the instruction itself
	if jmpPc != pc1 {
		t.Errorf("JMP should return current PC, expected %d, got %d", pc1, jmpPc)
	}

	// Verify the JMP was recorded in the right place
	if fs.currentPC() != 1 {
		t.Errorf("PC should be 1 after JMP, got %d", fs.currentPC())
	}
}

func TestConditionalJump(t *testing.T) {
	proto := &Prototype{maxstacksize: 4}
	fs := &FuncState{Proto: proto}

	// Allocate registers for comparison (allocReg doesn't emit code)
	_ = fs.allocReg()
	_ = fs.allocReg()

	// Test EQ (opcode 39): EQ A B C -> if ((RK(B) == RK(C)) ~= A) then pc++
	// First instruction, so pc = 0
	eqPc := fs.emitABC(39, 0, 1, 2)

	if eqPc != 0 {
		t.Errorf("EQ should be at pc 0, got %d", eqPc)
	}

	// Test JMP when comparison is true
	pcBeforeJmp := fs.currentPC()
	jmpPc := fs.emitAsBx(44, 0, 10)

	if jmpPc != pcBeforeJmp {
		t.Errorf("JMP should be at pc %d, got %d", pcBeforeJmp, jmpPc)
	}
}

func TestLoopWithJump(t *testing.T) {
	proto := &Prototype{maxstacksize: 3}
	fs := &FuncState{Proto: proto}

	// Simulate a simple loop: while true do end
	// The loop uses JMP to jump back

	// Allocate register for condition check
	_ = fs.allocReg()

	// Emit loop body (simplified) - one instruction
	_ = fs.emitABC(1, 0, 0, 0) // LOADNIL placeholder

	// Emit JMP back to loop start (negative offset)
	_ = fs.emitAsBx(44, 0, 0)

	// Should have 2 instructions
	if len(proto.code) != 2 {
		t.Errorf("expected 2 instructions, got %d", len(proto.code))
	}
}

func TestIfThenElseControlFlow(t *testing.T) {
	proto := &Prototype{maxstacksize: 4}
	fs := &FuncState{Proto: proto}

	// Simulate: if cond then stmt1 else stmt2 end
	// Pattern: LOAD condition -> JMP if false -> statement -> JMP to end -> else statement

	// Load condition into register 0
	_ = fs.allocReg()

	// Jump to else (or end) if condition is false
	// In Lua bytecode, this uses a test-and-jump instruction
	_ = fs.emitAsBx(44, 0, 0) // Placeholder for JMP

	// Then branch
	thenReg := fs.allocReg()
	_ = fs.emitABC(1, thenReg, 0, 0) // LOADNIL (placeholder)

	// Jump past else
	_ = fs.emitAsBx(44, 0, 0) // Placeholder

	// Else branch
	elseReg := fs.allocReg()
	_ = fs.emitABC(1, elseReg, 0, 0) // LOADNIL (placeholder)

	// Instruction after else
	_ = fs.emitABC(1, 0, 0, 0)

	if len(proto.code) != 5 {
		t.Errorf("expected 5 instructions for if-else, got %d", len(proto.code))
	}
}

// =============================================================================
// Operator Tests
// =============================================================================

func TestArithmeticOperators(t *testing.T) {
	proto := &Prototype{maxstacksize: 5}
	fs := &FuncState{Proto: proto}

	// Test ADD (opcode 13): ADD A B C -> R(A) = RK(B) + RK(C)
	_ = fs.emitABC(13, 0, 1, 2)

	// Test SUB (opcode 14): SUB A B C -> R(A) = RK(B) - RK(C)
	_ = fs.emitABC(14, 0, 1, 2)

	// Test MUL (opcode 15): MUL A B C -> R(A) = RK(B) * RK(C)
	_ = fs.emitABC(15, 0, 1, 2)

	// Test DIV (opcode 16): DIV A B C -> R(A) = RK(B) / RK(C)
	_ = fs.emitABC(16, 0, 1, 2)

	// Test MOD (opcode 17): MOD A B C -> R(A) = RK(B) % RK(C)
	_ = fs.emitABC(17, 0, 1, 2)

	// Test POW (opcode 18): POW A B C -> R(A) = RK(B) ^ RK(C)
	_ = fs.emitABC(18, 0, 1, 2)

	// Test IDIV (opcode 19): IDIV A B C -> R(A) = RK(B) // RK(C)
	_ = fs.emitABC(19, 0, 1, 2)

	if len(proto.code) != 7 {
		t.Errorf("expected 7 arithmetic instructions, got %d", len(proto.code))
	}
}

func TestUnaryOperators(t *testing.T) {
	proto := &Prototype{maxstacksize: 3}
	fs := &FuncState{Proto: proto}

	// Test UNM (opcode 12): UNM A B -> R(A) = -R(B)
	unmReg := fs.allocReg()
	_ = fs.emitABC(12, unmReg, 0, 0)

	// Test NOT (opcode 20): NOT A B -> R(A) = not R(B)
	notReg := fs.allocReg()
	_ = fs.emitABC(20, notReg, 0, 0)

	// Test LEN (opcode 21): LEN A B -> R(A) = length of R(B)
	lenReg := fs.allocReg()
	_ = fs.emitABC(21, lenReg, 0, 0)

	// Test BNOT (opcode 22): BNOT A B -> R(A) = ~R(B)
	bnotReg := fs.allocReg()
	_ = fs.emitABC(22, bnotReg, 0, 0)

	if len(proto.code) != 4 {
		t.Errorf("expected 4 unary instructions, got %d", len(proto.code))
	}
}

func TestComparisonOperators(t *testing.T) {
	proto := &Prototype{maxstacksize: 5}
	fs := &FuncState{Proto: proto}

	// Allocate registers
	_ = fs.allocReg()

	// Test LT (opcode 7): LT A B C -> if (RK(B) < RK(C)) then pc++
	_ = fs.emitABC(7, 0, 1, 2)

	// Test GT (opcode 8): GT A B C -> if (RK(B) > RK(C)) then pc++
	_ = fs.emitABC(8, 0, 1, 2)

	// Test LE (opcode 9): LE A B C -> if (RK(B) <= RK(C)) then pc++
	_ = fs.emitABC(9, 0, 1, 2)

	// Test EQ (opcode 39): EQ A B C -> if (RK(B) == RK(C)) ~= A then pc++
	_ = fs.emitABC(39, 0, 1, 2)

	if len(proto.code) != 4 {
		t.Errorf("expected 4 comparison instructions, got %d", len(proto.code))
	}
}

func TestLogicalOperators(t *testing.T) {
	proto := &Prototype{maxstacksize: 4}
	fs := &FuncState{Proto: proto}

	// Test AND (opcode 23): AND A B C -> R(A) = R(B) and R(C)
	andReg := fs.allocReg()
	_ = fs.emitABC(23, andReg, 1, 2)

	// Test OR (opcode 24): OR A B C -> R(A) = R(B) or R(C)
	orReg := fs.allocReg()
	_ = fs.emitABC(24, orReg, 1, 2)

	// Test JMP for short-circuit (opcode 44)
	_ = fs.emitAsBx(44, 0, 5)

	if len(proto.code) != 3 {
		t.Errorf("expected 3 logical instructions, got %d", len(proto.code))
	}
}

func TestBitwiseOperators(t *testing.T) {
	proto := &Prototype{maxstacksize: 4}
	fs := &FuncState{Proto: proto}

	// Test SHL (opcode 25): SHL A B C -> R(A) = R(B) << R(C)
	shlReg := fs.allocReg()
	_ = fs.emitABC(25, shlReg, 1, 2)

	// Test SHR (opcode 26): SHR A B C -> R(A) = R(B) >> R(C)
	shrReg := fs.allocReg()
	_ = fs.emitABC(26, shrReg, 1, 2)

	// Test BAND (opcode 27): BAND A B C -> R(A) = R(B) & R(C)
	bandReg := fs.allocReg()
	_ = fs.emitABC(27, bandReg, 1, 2)

	// Test BOR (opcode 28): BOR A B C -> R(A) = R(B) | R(C)
	borReg := fs.allocReg()
	_ = fs.emitABC(28, borReg, 1, 2)

	// Test BXOR (opcode 29): BXOR A B C -> R(A) = R(B) ~ R(C)
	bxorReg := fs.allocReg()
	_ = fs.emitABC(29, bxorReg, 1, 2)

	if len(proto.code) != 5 {
		t.Errorf("expected 5 bitwise instructions, got %d", len(proto.code))
	}
}

func TestConcatOperator(t *testing.T) {
	proto := &Prototype{maxstacksize: 4}
	fs := &FuncState{Proto: proto}

	// Test CONCAT (opcode 40): CONCAT A B C -> R(A) = R(B) .. R(C) .. R(C-1)
	concatReg := fs.allocReg()
	_ = fs.emitABC(40, concatReg, 1, 2)

	if len(proto.code) != 1 {
		t.Errorf("expected 1 CONCAT instruction, got %d", len(proto.code))
	}
}

// =============================================================================
// Instruction Encoding Tests
// =============================================================================

func TestEncodeABC(t *testing.T) {
	// Test ABC encoding format
	inst := encodeABC(37, 5, 10, 15)

	// Opcode: bits 0-6
	if inst&0x7F != 37 {
		t.Errorf("opcode bits wrong")
	}

	// A: bits 7-14 (8 bits)
	a := (inst >> 7) & 0xFF
	if a != 5 {
		t.Errorf("A should be 5, got %d", a)
	}

	// C: bits 24-31 (8 bits)
	c := inst >> 24
	if c != 15 {
		t.Errorf("C should be 15, got %d", c)
	}
}

func TestEncodeABx(t *testing.T) {
	// Test ABx encoding format
	inst := encodeABx(1, 3, 1000) // LOADK instruction

	// Opcode: bits 0-6
	if inst&0x7F != 1 {
		t.Errorf("opcode bits wrong")
	}

	// A: bits 7-14
	a := (inst >> 7) & 0xFF
	if a != 3 {
		t.Errorf("A should be 3, got %d", a)
	}

	// Bx: bits 15+
	bx := inst >> 15
	if bx != 1000 {
		t.Errorf("Bx should be 1000, got %d", bx)
	}
}

func TestEncodeAsBx(t *testing.T) {
	// Test AsBx encoding format (signed Bx)
	// Positive offset
	instPos := encodeAsBx(44, 0, 100)
	if instPos == 0 {
		t.Error("positive AsBx encoding failed")
	}

	// Negative offset
	instNeg := encodeAsBx(44, 0, -50)
	if instNeg == 0 {
		t.Error("negative AsBx encoding failed")
	}

	// Verify positive > negative
	if instPos <= instNeg {
		t.Error("positive offset should encode to larger value than negative")
	}
}

// =============================================================================
// Constant Management Tests
// =============================================================================

func TestAddMultipleConstants(t *testing.T) {
	proto := &Prototype{k: make([]*api.Constant, 0)}
	fs := &FuncState{Proto: proto}

	// Add many constants - testing deduplication
	// Integer 1 -> idx 0
	idx0 := fs.addConstant(NewConstInteger(1))
	if idx0 != 0 {
		t.Errorf("integer 1 should have index 0, got %d", idx0)
	}

	// Integer 2 -> idx 1
	idx1 := fs.addConstant(NewConstInteger(2))
	if idx1 != 1 {
		t.Errorf("integer 2 should have index 1, got %d", idx1)
	}

	// Float 1.5 -> idx 2
	idx2 := fs.addConstant(NewConstFloat(1.5))
	if idx2 != 2 {
		t.Errorf("float 1.5 should have index 2, got %d", idx2)
	}

	// String "hello" -> idx 3
	idx3 := fs.addConstant(NewConstString("hello"))
	if idx3 != 3 {
		t.Errorf("string hello should have index 3, got %d", idx3)
	}

	// Integer 1 (duplicate of idx0) -> idx 0
	idx4 := fs.addConstant(NewConstInteger(1))
	if idx4 != 0 {
		t.Errorf("duplicate integer 1 should return index 0, got %d", idx4)
	}

	// String "world" -> idx 4 (new unique string)
	idx5 := fs.addConstant(NewConstString("world"))
	if idx5 != 4 {
		t.Errorf("string world should have index 4, got %d", idx5)
	}

	// Should have 5 unique constants (deduplicated integer 1)
	if len(proto.k) != 5 {
		t.Errorf("expected 5 constants after deduplication, got %d", len(proto.k))
	}
}

func TestConstantTypes(t *testing.T) {
	// Test all constant types
	nilConst := &Constant{Type: ConstNil}
	intConst := NewConstInteger(42)
	floatConst := NewConstFloat(3.14)
	strConst := NewConstString("test")
	boolConst := NewConstBool(true)

	if nilConst.Type != ConstNil {
		t.Error("nil constant type wrong")
	}
	if intConst.Type != ConstInteger {
		t.Error("integer constant type wrong")
	}
	if floatConst.Type != ConstFloat {
		t.Error("float constant type wrong")
	}
	if strConst.Type != ConstString {
		t.Error("string constant type wrong")
	}
	if boolConst.Type != ConstBool {
		t.Error("bool constant type wrong")
	}

	// Test boolean value storage
	if boolConst.Int != 1 {
		t.Errorf("true bool should have Int=1, got %d", boolConst.Int)
	}

	falseConst := NewConstBool(false)
	if falseConst.Int != 0 {
		t.Errorf("false bool should have Int=0, got %d", falseConst.Int)
	}
}

// =============================================================================
// Prototype Method Tests
// =============================================================================

func TestPrototypeGetters(t *testing.T) {
	proto := &Prototype{
		sourceName:      "getter_test",
		lineDefined:     5,
		lastLineDefined: 20,
		numparams:       3,
		flag:            1, // Vararg flag set
		maxstacksize:    10,
		k:               make([]*api.Constant, 0),
		code:            []uint32{1, 2, 3},
	}

	if proto.SourceName() != "getter_test" {
		t.Error("SourceName() wrong")
	}
	if proto.LineDefined() != 5 {
		t.Error("LineDefined() wrong")
	}
	if proto.LastLineDefined() != 20 {
		t.Error("LastLineDefined() wrong")
	}
	if proto.NumParams() != 3 {
		t.Error("NumParams() wrong")
	}
	if !proto.IsVararg() {
		t.Error("IsVararg() should be true")
	}
	if proto.MaxStackSize() != 10 {
		t.Error("MaxStackSize() wrong")
	}
	if len(proto.GetCode()) != 3 {
		t.Error("GetCode() wrong")
	}
	if proto.GetConstants() == nil {
		t.Error("GetConstants() should not be nil")
	}
}

func TestPrototypeSubProtos(t *testing.T) {
	proto := &Prototype{p: make([]*Prototype, 0)}

	sub1 := &Prototype{sourceName: "sub1"}
	sub2 := &Prototype{sourceName: "sub2"}

	proto.p = append(proto.p, sub1)
	proto.p = append(proto.p, sub2)

	subs := proto.GetSubProtos()
	if len(subs) != 2 {
		t.Errorf("expected 2 sub-prototypes, got %d", len(subs))
	}
	if subs[0].SourceName() != "sub1" {
		t.Error("first sub-prototype name wrong")
	}
	if subs[1].SourceName() != "sub2" {
		t.Error("second sub-prototype name wrong")
	}
}
