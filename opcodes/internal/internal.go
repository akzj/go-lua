// Package internal implements Lua VM instruction creation, extraction,
// and opcode metadata matching lua-master/lopcodes.c exactly.
//
// Reference: lua-master/lopcodes.c, lua-master/lopcodes.h
package internal

import (
	"github.com/akzj/go-lua/opcodes/api"
)

// =============================================================================
// Instruction Creation Functions
// =============================================================================

// CreateABC creates an ABC format instruction: [opcode(7) | A(8) | k(1) | B(8) | C(8)]
func CreateABC(op api.OpCode, a, b, c uint32) api.Instruction {
	return (api.Instruction(op) << api.POS_OP) |
		(api.Instruction(a) << api.POS_A) |
		(api.Instruction(b) << api.POS_B) |
		(api.Instruction(c) << api.POS_C)
}

// CreateABCk creates an ABC format instruction with k bit: [opcode(7) | A(8) | k(1) | B(8) | C(8)]
func CreateABCk(op api.OpCode, a, k, b, c uint32) api.Instruction {
	return (api.Instruction(op) << api.POS_OP) |
		(api.Instruction(a) << api.POS_A) |
		(api.Instruction(k) << api.POS_k) |
		(api.Instruction(b) << api.POS_B) |
		(api.Instruction(c) << api.POS_C)
}

// CreatevABCk creates a variant ABC format instruction with variable B and C: [opcode(7) | A(8) | k(1) | vB(6) | vC(10)]
func CreatevABCk(op api.OpCode, a, k, vb, vc uint32) api.Instruction {
	return (api.Instruction(op) << api.POS_OP) |
		(api.Instruction(a) << api.POS_A) |
		(api.Instruction(k) << api.POS_k) |
		(api.Instruction(vb) << api.POS_vB) |
		(api.Instruction(vc) << api.POS_vC)
}

// CreateABx creates an ABx format instruction: [opcode(7) | A(8) | Bx(17)]
func CreateABx(op api.OpCode, a, bx uint32) api.Instruction {
	return (api.Instruction(op) << api.POS_OP) |
		(api.Instruction(a) << api.POS_A) |
		(api.Instruction(bx) << api.POS_Bx)
}

// CreateAsBx creates an AsBx format instruction with signed Bx: [opcode(7) | A(8) | sBx(17 signed)]
func CreateAsBx(op api.OpCode, a, sbx int32) api.Instruction {
	// Convert signed to unsigned representation
	bx := uint32(sbx + api.OFFSET_sBx)
	return (api.Instruction(op) << api.POS_OP) |
		(api.Instruction(a) << api.POS_A) |
		(api.Instruction(bx) << api.POS_Bx)
}

// CreateAx creates an Ax format instruction: [opcode(7) | Ax(25)]
func CreateAx(op api.OpCode, ax uint32) api.Instruction {
	return (api.Instruction(op) << api.POS_OP) |
		(api.Instruction(ax) << api.POS_Ax)
}

// CreateSJ creates an SJ format instruction with signed J: [opcode(7) | sJ(25 signed)]
func CreateSJ(op api.OpCode, sj int32) api.Instruction {
	// Convert signed to unsigned representation
	sj_unsigned := uint32(sj + api.OFFSET_sJ)
	return (api.Instruction(op) << api.POS_OP) |
		(api.Instruction(sj_unsigned) << api.POS_sJ)
}

// =============================================================================
// Instruction Extraction Functions
// =============================================================================

// GetOpCode extracts the opcode from an instruction.
func GetOpCode(i api.Instruction) api.OpCode {
	return api.OpCode((i >> api.POS_OP) & api.MAXARG_OP)
}

// GetArgA extracts argument A from an instruction.
func GetArgA(i api.Instruction) uint32 {
	return uint32((i >> api.POS_A) & api.MAXARG_A)
}

// GetArgB extracts argument B from an instruction (standard 8-bit format).
func GetArgB(i api.Instruction) uint32 {
	return uint32((i >> api.POS_B) & api.MAXARG_B)
}

// GetArgC extracts argument C from an instruction (standard 8-bit format).
func GetArgC(i api.Instruction) uint32 {
	return uint32((i >> api.POS_C) & api.MAXARG_C)
}

// GetArgBx extracts argument Bx from an instruction (17-bit format).
func GetArgBx(i api.Instruction) uint32 {
	return uint32((i >> api.POS_Bx) & api.MAXARG_Bx)
}

// GetArgAx extracts argument Ax from an instruction (25-bit format).
func GetArgAx(i api.Instruction) uint32 {
	return uint32((i >> api.POS_Ax) & api.MAXARG_Ax)
}

// GetArgsBx extracts signed argument sBx from an instruction.
func GetArgsBx(i api.Instruction) int32 {
	bx := GetArgBx(i)
	return int32(bx) - api.OFFSET_sBx
}

// GetArgsJ extracts signed argument sJ from an instruction.
func GetArgsJ(i api.Instruction) int32 {
	sj := (i >> api.POS_sJ) & api.MAXARG_sJ
	return int32(sj) - api.OFFSET_sJ
}

// GetArgK extracts the k (constant) bit from an instruction.
func GetArgK(i api.Instruction) uint32 {
	return uint32((i >> api.POS_k) & 1)
}

// GetArgvB extracts variant argument vB from an instruction.
func GetArgvB(i api.Instruction) uint32 {
	return uint32((i >> api.POS_vB) & api.MAXARG_vB)
}

// GetArgvC extracts variant argument vC from an instruction.
func GetArgvC(i api.Instruction) uint32 {
	return uint32((i >> api.POS_vC) & api.MAXARG_vC)
}

// =============================================================================
// Opcode Modes - luaP_opmodes array matching lua-master/lopcodes.c
// =============================================================================

// luaP_opmodes encodes instruction modes for all 85 opcodes.
// Each entry encodes: MM(1) | OT(1) | IT(1) | T(1) | A(1) | mode(3)
// Matches lua-master/lopcodes.c exactly.
var luaP_opmodes = [api.NUM_OPCODES]uint8{
	/*       MM OT IT T  A  mode	   opcode  */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)     OP_MOVE = 0 */
	0x0B, /* opmode(0, 0, 0, 0, 1, iAsBx)   OP_LOADI = 1 */
	0x0B, /* opmode(0, 0, 0, 0, 1, iAsBx)   OP_LOADF = 2 */
	0x0A, /* opmode(0, 0, 0, 0, 1, iABx)    OP_LOADK = 3 */
	0x0A, /* opmode(0, 0, 0, 0, 1, iABx)    OP_LOADKX = 4 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_LOADFALSE = 5 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_LFALSESKIP = 6 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_LOADTRUE = 7 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_LOADNIL = 8 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_GETUPVAL = 9 */
	0x00, /* opmode(0, 0, 0, 0, 0, iABC)    OP_SETUPVAL = 10 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_GETTABUP = 11 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_GETTABLE = 12 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_GETI = 13 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_GETFIELD = 14 */
	0x00, /* opmode(0, 0, 0, 0, 0, iABC)    OP_SETTABUP = 15 */
	0x00, /* opmode(0, 0, 0, 0, 0, iABC)    OP_SETTABLE = 16 */
	0x00, /* opmode(0, 0, 0, 0, 0, iABC)    OP_SETI = 17 */
	0x00, /* opmode(0, 0, 0, 0, 0, iABC)    OP_SETFIELD = 18 */
	0x09, /* opmode(0, 0, 0, 0, 1, ivABC)   OP_NEWTABLE = 19 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_SELF = 20 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_ADDI = 21 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_ADDK = 22 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_SUBK = 23 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_MULK = 24 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_MODK = 25 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_POWK = 26 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_DIVK = 27 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_IDIVK = 28 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_BANDK = 29 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_BORK = 30 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_BXORK = 31 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_SHLI = 32 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_SHRI = 33 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_ADD = 34 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_SUB = 35 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_MUL = 36 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_MOD = 37 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_POW = 38 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_DIV = 39 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_IDIV = 40 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_BAND = 41 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_BOR = 42 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_BXOR = 43 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_SHL = 44 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_SHR = 45 */
	0x80, /* opmode(1, 0, 0, 0, 0, iABC)    OP_MMBIN = 46 */
	0x80, /* opmode(1, 0, 0, 0, 0, iABC)    OP_MMBINI = 47 */
	0x80, /* opmode(1, 0, 0, 0, 0, iABC)    OP_MMBINK = 48 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_UNM = 49 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_BNOT = 50 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_NOT = 51 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_LEN = 52 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_CONCAT = 53 */
	0x00, /* opmode(0, 0, 0, 0, 0, iABC)    OP_CLOSE = 54 */
	0x00, /* opmode(0, 0, 0, 0, 0, iABC)    OP_TBC = 55 */
	0x05, /* opmode(0, 0, 0, 0, 0, isJ)    OP_JMP = 56 */
	0x10, /* opmode(0, 0, 0, 1, 0, iABC)    OP_EQ = 57 */
	0x10, /* opmode(0, 0, 0, 1, 0, iABC)    OP_LT = 58 */
	0x10, /* opmode(0, 0, 0, 1, 0, iABC)    OP_LE = 59 */
	0x10, /* opmode(0, 0, 0, 1, 0, iABC)    OP_EQK = 60 */
	0x10, /* opmode(0, 0, 0, 1, 0, iABC)    OP_EQI = 61 */
	0x10, /* opmode(0, 0, 0, 1, 0, iABC)    OP_LTI = 62 */
	0x10, /* opmode(0, 0, 0, 1, 0, iABC)    OP_LEI = 63 */
	0x10, /* opmode(0, 0, 0, 1, 0, iABC)    OP_GTI = 64 */
	0x10, /* opmode(0, 0, 0, 1, 0, iABC)    OP_GEI = 65 */
	0x10, /* opmode(0, 0, 0, 1, 0, iABC)    OP_TEST = 66 */
	0x18, /* opmode(0, 0, 0, 1, 1, iABC)    OP_TESTSET = 67 */
	0x68, /* opmode(0, 1, 1, 0, 1, iABC)    OP_CALL = 68 */
	0x68, /* opmode(0, 1, 1, 0, 1, iABC)    OP_TAILCALL = 69 */
	0x20, /* opmode(0, 0, 1, 0, 0, iABC)    OP_RETURN = 70 */
	0x00, /* opmode(0, 0, 0, 0, 0, iABC)    OP_RETURN0 = 71 */
	0x00, /* opmode(0, 0, 0, 0, 0, iABC)    OP_RETURN1 = 72 */
	0x0A, /* opmode(0, 0, 0, 0, 1, iABx)    OP_FORLOOP = 73 */
	0x0A, /* opmode(0, 0, 0, 0, 1, iABx)    OP_FORPREP = 74 */
	0x02, /* opmode(0, 0, 0, 0, 0, iABx)    OP_TFORPREP = 75 */
	0x00, /* opmode(0, 0, 0, 0, 0, iABC)    OP_TFORCALL = 76 */
	0x0A, /* opmode(0, 0, 0, 0, 1, iABx)    OP_TFORLOOP = 77 */
	0x21, /* opmode(0, 0, 1, 0, 0, ivABC)   OP_SETLIST = 78 */
	0x0A, /* opmode(0, 0, 0, 0, 1, iABx)    OP_CLOSURE = 79 */
	0x48, /* opmode(0, 1, 0, 0, 1, iABC)    OP_VARARG = 80 */
	0x08, /* opmode(0, 0, 0, 0, 1, iABC)    OP_GETVARG = 81 */
	0x02, /* opmode(0, 0, 0, 0, 0, iABx)    OP_ERRNNIL = 82 */
	0x20, /* opmode(0, 0, 1, 0, 0, iABC)    OP_VARARGPREP = 83 */
	0x04, /* opmode(0, 0, 0, 0, 0, iAx)     OP_EXTRAARG = 84 */
}

// =============================================================================
// Mode Helper Functions
// =============================================================================

// GetOpMode returns the instruction format mode for an opcode.
func GetOpMode(op api.OpCode) api.OpMode {
	return api.OpMode(luaP_opmodes[op] & 7)
}

// TestAMode tests if the A bit is set for an opcode.
func TestAMode(op api.OpCode) bool {
	return (luaP_opmodes[op] & 0x08) != 0
}

// TestTMode tests if the T bit is set for an opcode.
func TestTMode(op api.OpCode) bool {
	return (luaP_opmodes[op] & 0x10) != 0
}

// TestITMode tests if the IT bit is set for an opcode.
func TestITMode(op api.OpCode) bool {
	return (luaP_opmodes[op] & 0x20) != 0
}

// TestOTMode tests if the OT bit is set for an opcode.
func TestOTMode(op api.OpCode) bool {
	return (luaP_opmodes[op] & 0x40) != 0
}

// TestMMMode tests if the MM (metamethod) bit is set for an opcode.
func TestMMMode(op api.OpCode) bool {
	return (luaP_opmodes[op] & 0x80) != 0
}

// =============================================================================
// Instruction Property Checks (per lua-master/lopcodes.c)
// =============================================================================

// luaP_isOT checks whether instruction sets top for next instruction.
// That is, it results in multiple values.
func luaP_isOT(i api.Instruction) bool {
	op := GetOpCode(i)
	switch op {
	case api.OP_TAILCALL:
		return true
	default:
		return TestOTMode(op) && GetArgC(i) == 0
	}
}

// luaP_isIT checks whether instruction uses top from previous instruction.
// That is, it accepts multiple results.
func luaP_isIT(i api.Instruction) bool {
	op := GetOpCode(i)
	switch op {
	case api.OP_SETLIST:
		return TestITMode(op) && GetArgvB(i) == 0
	default:
		return TestITMode(op) && GetArgB(i) == 0
	}
}

// GET_OPCODE is an alias for GetOpCode for compatibility with lua-master naming.
func GET_OPCODE(i api.Instruction) api.OpCode {
	return GetOpCode(i)
}

// GETARG_A is an alias for GetArgA.
func GETARG_A(i api.Instruction) uint32 {
	return GetArgA(i)
}

// GETARG_B is an alias for GetArgB.
func GETARG_B(i api.Instruction) uint32 {
	return GetArgB(i)
}

// GETARG_C is an alias for GetArgC.
func GETARG_C(i api.Instruction) uint32 {
	return GetArgC(i)
}

// GETARG_Bx is an alias for GetArgBx.
func GETARG_Bx(i api.Instruction) uint32 {
	return GetArgBx(i)
}

// GETARG_Ax is an alias for GetArgAx.
func GETARG_Ax(i api.Instruction) uint32 {
	return GetArgAx(i)
}

// GETARG_sBx is an alias for GetArgsBx.
func GETARG_sBx(i api.Instruction) int32 {
	return GetArgsBx(i)
}

// GETARG_sJ is an alias for GetArgsJ.
func GETARG_sJ(i api.Instruction) int32 {
	return GetArgsJ(i)
}

// GETARG_K is an alias for GetArgK.
func GETARG_K(i api.Instruction) uint32 {
	return GetArgK(i)
}

// GETARG_vB is an alias for GetArgvB.
func GETARG_vB(i api.Instruction) uint32 {
	return GetArgvB(i)
}

// GETARG_vC is an alias for GetArgvC.
func GETARG_vC(i api.Instruction) uint32 {
	return GetArgvC(i)
}
