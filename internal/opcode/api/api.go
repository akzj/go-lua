// Package api defines Lua 5.5.1 VM instruction encoding and all 85 opcodes.
//
// Every instruction is a uint32 with a 7-bit opcode and operands in one of
// six formats: iABC, ivABC, iABx, iAsBx, iAx, isJ.
//
// Reference: .analysis/02-opcodes-instruction-format.md
package api

// ---------------------------------------------------------------------------
// Instruction type and field layout constants
// ---------------------------------------------------------------------------

// Instruction is a 32-bit encoded Lua VM instruction.
type Instruction = uint32

// Field sizes (bits)
const (
	SizeOP = 7
	SizeA  = 8
	SizeB  = 8
	SizeC  = 8
	SizeBx = SizeC + SizeB + 1 // 17
	SizeAx = SizeBx + SizeA    // 25
	SizeSJ = SizeBx + SizeA    // 25
	SizeVB = 6
	SizeVC = 10
)

// Field positions (bit offset from LSB)
const (
	PosOP = 0
	PosA  = PosOP + SizeOP // 7
	PosK  = PosA + SizeA   // 15
	PosB  = PosK + 1       // 16
	PosC  = PosB + SizeB   // 24
	PosBx = PosK           // 15
	PosAx = PosA           // 7
	PosSJ = PosA           // 7
	PosVB = PosK + 1       // 16
	PosVC = PosVB + SizeVB // 22
)

// Operand limits
const (
	MaxArgA  = (1 << SizeA) - 1  // 255
	MaxArgB  = (1 << SizeB) - 1  // 255
	MaxArgC  = (1 << SizeC) - 1  // 255
	MaxArgBx = (1 << SizeBx) - 1 // 131071
	MaxArgAx = (1 << SizeAx) - 1 // 33554431
	MaxArgSJ = (1 << SizeSJ) - 1 // 33554431
	MaxArgVB = (1 << SizeVB) - 1 // 63
	MaxArgVC = (1 << SizeVC) - 1 // 1023
)

// Signed encoding offsets (excess-K bias)
const (
	OffsetSBx = MaxArgBx >> 1 // 65535
	OffsetSJ  = MaxArgSJ >> 1 // 16777215
	OffsetSC  = MaxArgC >> 1  // 127
)

// NoReg is the invalid register sentinel.
const NoReg = MaxArgA // 255

// ---------------------------------------------------------------------------
// Instruction decoding
// ---------------------------------------------------------------------------

// GetOpCode extracts the opcode from an instruction.
func GetOpCode(i Instruction) OpCode { return OpCode(i >> PosOP & ((1 << SizeOP) - 1)) }

// GetArgA extracts the A operand.
func GetArgA(i Instruction) int { return int(i >> PosA & ((1 << SizeA) - 1)) }

// GetArgB extracts the B operand.
func GetArgB(i Instruction) int { return int(i >> PosB & ((1 << SizeB) - 1)) }

// GetArgC extracts the C operand.
func GetArgC(i Instruction) int { return int(i >> PosC & ((1 << SizeC) - 1)) }

// GetArgK extracts the k bit.
func GetArgK(i Instruction) int { return int(i >> PosK & 1) }

// GetArgBx extracts the Bx operand (unsigned).
func GetArgBx(i Instruction) int { return int(i >> PosBx & ((1 << SizeBx) - 1)) }

// GetArgAx extracts the Ax operand (unsigned).
func GetArgAx(i Instruction) int { return int(i >> PosAx & ((1 << SizeAx) - 1)) }

// GetArgVB extracts the vB operand (6-bit).
func GetArgVB(i Instruction) int { return int(i >> PosVB & ((1 << SizeVB) - 1)) }

// GetArgVC extracts the vC operand (10-bit).
func GetArgVC(i Instruction) int { return int(i >> PosVC & ((1 << SizeVC) - 1)) }

// Signed decodings

// GetArgSBx extracts the signed sBx operand.
func GetArgSBx(i Instruction) int { return GetArgBx(i) - OffsetSBx }

// GetArgSJ extracts the signed sJ operand.
func GetArgSJ(i Instruction) int {
	return int(i>>PosSJ&((1<<SizeSJ)-1)) - OffsetSJ
}

// GetArgSC extracts the signed sC operand.
func GetArgSC(i Instruction) int { return GetArgC(i) - OffsetSC }

// GetArgSB extracts the signed sB operand.
func GetArgSB(i Instruction) int { return GetArgB(i) - OffsetSC }

// ---------------------------------------------------------------------------
// Instruction encoding
// ---------------------------------------------------------------------------

// CreateABCK creates an iABC format instruction.
func CreateABCK(op OpCode, a, b, c, k int) Instruction {
	return Instruction(op)<<PosOP | uint32(a)<<PosA | uint32(b)<<PosB |
		uint32(c)<<PosC | uint32(k)<<PosK
}

// CreateABx creates an iABx format instruction.
func CreateABx(op OpCode, a, bx int) Instruction {
	return Instruction(op)<<PosOP | uint32(a)<<PosA | uint32(bx)<<PosBx
}

// CreateAsBx creates an iAsBx format instruction (signed Bx).
func CreateAsBx(op OpCode, a, sbx int) Instruction {
	return CreateABx(op, a, sbx+OffsetSBx)
}

// CreateAx creates an iAx format instruction.
func CreateAx(op OpCode, ax int) Instruction {
	return Instruction(op)<<PosOP | uint32(ax)<<PosAx
}

// CreateSJ creates an isJ format instruction (signed jump).
func CreateSJ(op OpCode, sj int) Instruction {
	return Instruction(op)<<PosOP | uint32(sj+OffsetSJ)<<PosSJ
}

// CreateVABCK creates an ivABC format instruction.
func CreateVABCK(op OpCode, a, vb, vc, k int) Instruction {
	return Instruction(op)<<PosOP | uint32(a)<<PosA | uint32(vb)<<PosVB |
		uint32(vc)<<PosVC | uint32(k)<<PosK
}

// SetArgA replaces the A field in an existing instruction.
func SetArgA(i Instruction, v int) Instruction {
	mask := uint32(((1 << SizeA) - 1) << PosA)
	return (i &^ mask) | (uint32(v) << PosA & mask)
}

// SetArgB replaces the B field in an existing instruction.
func SetArgB(i Instruction, v int) Instruction {
	mask := uint32(((1 << SizeB) - 1) << PosB)
	return (i &^ mask) | (uint32(v) << PosB & mask)
}

// SetArgC replaces the C field in an existing instruction.
func SetArgC(i Instruction, v int) Instruction {
	mask := uint32(((1 << SizeC) - 1) << PosC)
	return (i &^ mask) | (uint32(v) << PosC & mask)
}

// SetArgK replaces the k bit in an existing instruction.
func SetArgK(i Instruction, v int) Instruction {
	mask := uint32(1 << PosK)
	return (i &^ mask) | (uint32(v) << PosK & mask)
}

// SetArgSBx replaces the sBx field in an existing instruction.
func SetArgSBx(i Instruction, v int) Instruction {
	mask := uint32(((1 << SizeBx) - 1) << PosBx)
	return (i &^ mask) | (uint32(v+OffsetSBx) << PosBx & mask)
}

// SetArgBx replaces the unsigned Bx field in an existing instruction.
func SetArgBx(i Instruction, v int) Instruction {
	mask := uint32(((1 << SizeBx) - 1) << PosBx)
	return (i &^ mask) | (uint32(v) << PosBx & mask)
}

// SetArgSJ replaces the sJ field in an existing instruction.
func SetArgSJ(i Instruction, v int) Instruction {
	mask := uint32(((1 << SizeSJ) - 1) << PosSJ)
	return (i &^ mask) | (uint32(v+OffsetSJ) << PosSJ & mask)
}

// SetOpCode replaces the opcode field in an existing instruction.
func SetOpCode(i Instruction, op OpCode) Instruction {
	mask := uint32((1 << SizeOP) - 1)
	return (i &^ mask) | uint32(op)
}

// CreateSJK creates an isJ format instruction with a k bit.
// Used by codesJ in codegen when the sJ format needs a k flag.
func CreateSJK(op OpCode, sj, k int) Instruction {
	return Instruction(op)<<PosOP | uint32(sj+OffsetSJ)<<PosSJ | uint32(k)<<PosK
}

// ---------------------------------------------------------------------------
// OpCode enumeration — all 85 Lua 5.5.1 opcodes
// ---------------------------------------------------------------------------

// OpCode is the type for Lua VM opcodes.
type OpCode = uint8

const (
	OP_MOVE       OpCode = 0
	OP_LOADI      OpCode = 1
	OP_LOADF      OpCode = 2
	OP_LOADK      OpCode = 3
	OP_LOADKX     OpCode = 4
	OP_LOADFALSE  OpCode = 5
	OP_LFALSESKIP OpCode = 6
	OP_LOADTRUE   OpCode = 7
	OP_LOADNIL    OpCode = 8
	OP_GETUPVAL   OpCode = 9
	OP_SETUPVAL   OpCode = 10
	OP_GETTABUP   OpCode = 11
	OP_GETTABLE   OpCode = 12
	OP_GETI       OpCode = 13
	OP_GETFIELD   OpCode = 14
	OP_SETTABUP   OpCode = 15
	OP_SETTABLE   OpCode = 16
	OP_SETI       OpCode = 17
	OP_SETFIELD   OpCode = 18
	OP_NEWTABLE   OpCode = 19
	OP_SELF       OpCode = 20
	OP_ADDI       OpCode = 21
	OP_ADDK       OpCode = 22
	OP_SUBK       OpCode = 23
	OP_MULK       OpCode = 24
	OP_MODK       OpCode = 25
	OP_POWK       OpCode = 26
	OP_DIVK       OpCode = 27
	OP_IDIVK      OpCode = 28
	OP_BANDK      OpCode = 29
	OP_BORK       OpCode = 30
	OP_BXORK      OpCode = 31
	OP_SHLI       OpCode = 32
	OP_SHRI       OpCode = 33
	OP_ADD        OpCode = 34
	OP_SUB        OpCode = 35
	OP_MUL        OpCode = 36
	OP_MOD        OpCode = 37
	OP_POW        OpCode = 38
	OP_DIV        OpCode = 39
	OP_IDIV       OpCode = 40
	OP_BAND       OpCode = 41
	OP_BOR        OpCode = 42
	OP_BXOR       OpCode = 43
	OP_SHL        OpCode = 44
	OP_SHR        OpCode = 45
	OP_MMBIN      OpCode = 46
	OP_MMBINI     OpCode = 47
	OP_MMBINK     OpCode = 48
	OP_UNM        OpCode = 49
	OP_BNOT       OpCode = 50
	OP_NOT        OpCode = 51
	OP_LEN        OpCode = 52
	OP_CONCAT     OpCode = 53
	OP_CLOSE      OpCode = 54
	OP_TBC        OpCode = 55
	OP_JMP        OpCode = 56
	OP_EQ         OpCode = 57
	OP_LT         OpCode = 58
	OP_LE         OpCode = 59
	OP_EQK        OpCode = 60
	OP_EQI        OpCode = 61
	OP_LTI        OpCode = 62
	OP_LEI        OpCode = 63
	OP_GTI        OpCode = 64
	OP_GEI        OpCode = 65
	OP_TEST       OpCode = 66
	OP_TESTSET    OpCode = 67
	OP_CALL       OpCode = 68
	OP_TAILCALL   OpCode = 69
	OP_RETURN     OpCode = 70
	OP_RETURN0    OpCode = 71
	OP_RETURN1    OpCode = 72
	OP_FORLOOP    OpCode = 73
	OP_FORPREP    OpCode = 74
	OP_TFORPREP   OpCode = 75
	OP_TFORCALL   OpCode = 76
	OP_TFORLOOP   OpCode = 77
	OP_SETLIST    OpCode = 78
	OP_CLOSURE    OpCode = 79
	OP_VARARG     OpCode = 80
	OP_GETVARG    OpCode = 81
	OP_ERRNNIL    OpCode = 82
	OP_VARARGPREP OpCode = 83
	OP_EXTRAARG   OpCode = 84

	NumOpcodes = 85
)

// ---------------------------------------------------------------------------
// Instruction format metadata
// ---------------------------------------------------------------------------

// OpMode identifies the operand encoding format.
type OpMode byte

const (
	IABC  OpMode = 0 // iABC:  A(8) k(1) B(8) C(8)
	IVABC OpMode = 1 // ivABC: A(8) k(1) vB(6) vC(10)
	IABx  OpMode = 2 // iABx:  A(8) Bx(17)
	IAsBx OpMode = 3 // iAsBx: A(8) sBx(17 signed)
	IAx   OpMode = 4 // iAx:   Ax(25)
	ISJ   OpMode = 5 // isJ:   sJ(25 signed)
)

// GetMode returns the instruction format for the given opcode.
func GetMode(op OpCode) OpMode { return OpMode(OpModes[op] & 0x07) }

// TestAMode returns true if the opcode sets register A.
func TestAMode(op OpCode) bool { return OpModes[op]&(1<<3) != 0 }

// TestTMode returns true if the opcode is a test (next instruction is a jump).
func TestTMode(op OpCode) bool { return OpModes[op]&(1<<4) != 0 }

// TestITMode returns true if the opcode uses L->top from the previous instruction (B==0).
func TestITMode(op OpCode) bool { return OpModes[op]&(1<<5) != 0 }

// TestOTMode returns true if the opcode sets L->top for the next instruction (C==0).
func TestOTMode(op OpCode) bool { return OpModes[op]&(1<<6) != 0 }

// TestMMMode returns true if the opcode is a metamethod fallback instruction.
func TestMMMode(op OpCode) bool { return OpModes[op]&(1<<7) != 0 }

// OpName returns the human-readable name of an opcode.
func OpName(op OpCode) string {
	if int(op) < len(OpNames) {
		return OpNames[op]
	}
	return "???"
}

// OpModes holds the mode flags for each opcode.
// Populated by init() in the implementation file.
var OpModes [NumOpcodes]byte

// OpNames holds the human-readable name for each opcode.
// Populated by init() in the implementation file.
var OpNames [NumOpcodes]string
