// Package vm implements the Lua virtual machine
package vm

// Opcode defines the instruction type
type Opcode uint8

const (
	OP_MOVE      Opcode = 0
	OP_LOADI     Opcode = 1
	OP_LOADF     Opcode = 2
	OP_LOADK     Opcode = 3
	OP_LOADKX    Opcode = 4
	OP_LOADBOOL  Opcode = 5
	OP_LOADNIL   Opcode = 6
	OP_GETUPVAL  Opcode = 7
	OP_SETUPVAL  Opcode = 8
	OP_GETTABUP  Opcode = 9
	OP_GETTABLE  Opcode = 10
	OP_GETI      Opcode = 11
	OP_GETFIELD  Opcode = 12
	OP_SETTABUP  Opcode = 13
	OP_SETTABLE  Opcode = 14
	OP_SETI      Opcode = 15
	OP_SETFIELD  Opcode = 16
	OP_NEWTABLE  Opcode = 17
	OP_SELF      Opcode = 18
	OP_ADDI      Opcode = 19
	OP_ADD       Opcode = 20
	OP_SUB       Opcode = 21
	OP_MUL       Opcode = 22
	OP_MOD       Opcode = 23
	OP_POW       Opcode = 24
	OP_DIV       Opcode = 25
	OP_IDIV      Opcode = 26
	OP_BAND      Opcode = 27
	OP_BOR       Opcode = 28
	OP_BXOR      Opcode = 29
	OP_SHL       Opcode = 30
	OP_SHR       Opcode = 31
	OP_UNM       Opcode = 32
	OP_BNOT      Opcode = 33
	OP_NOT       Opcode = 34
	OP_LEN       Opcode = 35
	OP_CONCAT    Opcode = 36
	OP_CLOSE     Opcode = 37
	OP_TBC       Opcode = 38
	OP_JMP       Opcode = 39
	OP_EQ        Opcode = 40
	OP_LT        Opcode = 41
	OP_LE        Opcode = 42
	OP_EQI       Opcode = 43
	OP_LEI       Opcode = 44
	OP_LTI       Opcode = 45
	OP_GTI       Opcode = 46
	OP_TEST      Opcode = 47
	OP_FORPREP   Opcode = 48
	OP_FORLOOP   Opcode = 49
	OP_FORGPREP  Opcode = 50
	OP_FORGLOOP  Opcode = 51
	OP_SETLIST   Opcode = 52
	OP_CLOSURE   Opcode = 53
	OP_VARARG    Opcode = 54
	OP_VARARGPREP Opcode = 55
	OP_EXTRAARG  Opcode = 56
	OP_RETURN    Opcode = 57
)

// Instruction is a 32-bit bytecode instruction
type Instruction uint32

// Opcode returns the opcode of the instruction
func (i Instruction) Opcode() Opcode {
	return Opcode(i & 0x7F)
}

// A returns the A field of the instruction
func (i Instruction) A() int {
	return int((i >> 7) & 0xFF)
}

// B returns the B field of the instruction (8 bits)
func (i Instruction) B() int {
	return int((i >> 24) & 0xFF)
}

// C returns the C field of the instruction (9 bits)
func (i Instruction) C() int {
	return int((i >> 15) & 0x1FF)
}

// Bx returns the Bx field of the instruction (17 bits)
func (i Instruction) Bx() int {
	return int((i >> 15) & 0x1FFFF)
}

// sBx returns the signed Bx field of the instruction (17 bits, bias 0xFFFF)
func (i Instruction) sBx() int {
	return int((i >> 15) & 0x1FFFF) - 0xFFFF
}

// Ax returns the Ax field of the instruction
func (i Instruction) Ax() int {
	return int((i >> 7) & 0x1FFFFFF)
}

// MakeABC creates an iABC format instruction
func MakeABC(op Opcode, a, b, c int) Instruction {
	return Instruction(uint32(op) | uint32(a)<<7 | uint32(c)<<15 | uint32(b)<<24)
}

// MakeABx creates an iABx format instruction
func MakeABx(op Opcode, a, bx int) Instruction {
	return Instruction(uint32(op) | uint32(a)<<7 | uint32(bx)<<15)
}

// MakeAsBx creates an iAsBx format instruction
func MakeAsBx(op Opcode, a, sbx int) Instruction {
	return MakeABx(op, a, sbx+0xFFFF)
}

// MakeAx creates an iAx format instruction
func MakeAx(op Opcode, ax int) Instruction {
	return Instruction(uint32(op) | uint32(ax)<<7)
}