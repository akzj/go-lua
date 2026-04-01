package lopcodes

/*
** $Id: lopcodes.go $
** Opcodes for Lua virtual machine
** Ported from lopcodes.h
*/

import "github.com/akzj/go-lua/internal/lobject"

/*
** We assume that instructions are unsigned 32-bit integers.
** All instructions have an opcode in the first 7 bits.
** Instructions can have the following formats:
**
**       3 3 2 2 2 2 2 2 2 2 2 2 1 1 1 1 1 1 1 1 1 1 0 0 0 0 0 0 0 0 0 0
**       1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0
** iABC          C(8)     |      B(8)     |k|     A(8)      |   Op(7)     |
** ivABC         vC(10)    |     vB(6)   |k|     A(8)      |   Op(7)     |
** iABx                Bx(17)               |     A(8)      |   Op(7)     |
** iAsBx              sBx (signed)(17)      |     A(8)      |   Op(7)     |
** iAx                           Ax(25)                     |   Op(7)     |
** isJ                           sJ (signed)(25)            |   Op(7)     |
 */

/*
** basic instruction formats
 */
type OpMode uint8

const (
	IABC  OpMode = iota // iABC format
	IvABC               // ivABC format
	IABx                // iABx format
	IAsBx               // iAsBx format
	IAx                 // iAx format
	IsJ                 // isJ format
)

/*
** size and position of opcode arguments.
 */
const (
	SIZE_OP  = 7
	SIZE_A   = 8
	SIZE_B   = 8
	SIZE_C   = 8
	SIZE_vB  = 6
	SIZE_vC  = 10
	SIZE_Bx  = SIZE_C + SIZE_B + 1
	SIZE_Ax  = SIZE_Bx + SIZE_A
	SIZE_sJ  = SIZE_Bx + SIZE_A
)

const (
	POS_OP  = 0
	POS_A   = POS_OP + SIZE_OP
	POS_k   = POS_A + SIZE_A
	POS_B   = POS_k + 1
	POS_vB  = POS_k + 1
	POS_C   = POS_B + SIZE_B
	POS_vC  = POS_vB + SIZE_vB
	POS_Bx  = POS_k
	POS_Ax  = POS_A
	POS_sJ  = POS_A
)

/*
** limits for opcode arguments.
 */
const MAXARG_A = (1 << SIZE_A) - 1
const MAXARG_B = (1 << SIZE_B) - 1
const MAXARG_C = (1 << SIZE_C) - 1
const MAXARG_vB = (1 << SIZE_vB) - 1
const MAXARG_vC = (1 << SIZE_vC) - 1
const MAXARG_Bx = (1 << SIZE_Bx) - 1
const MAXARG_Ax = (1 << SIZE_Ax) - 1
const MAXARG_sJ = (1 << SIZE_sJ) - 1

const OFFSET_sBx = MAXARG_Bx >> 1      // 'sBx' is signed
const OFFSET_sJ  = MAXARG_sJ >> 1
const OFFSET_sC  = MAXARG_C >> 1

/*
** creates a mask with 'n' 1 bits at position 'p'
 */
func mask1(n uint, p uint) lobject.LUInt32 {
	return ^(((^lobject.LUInt32(0)) << n) << p)
}

/*
** creates a mask with 'n' 0 bits at position 'p'
 */
func mask0(n uint, p uint) lobject.LUInt32 {
	return ^mask1(n, p)
}

/*
** OpCode enumeration - must match C source exactly
 */
type OpCode uint8

const (
	OP_MOVE OpCode = iota
	OP_LOADI
	OP_LOADF
	OP_LOADK
	OP_LOADKX
	OP_LOADFALSE
	OP_LFALSESKIP
	OP_LOADTRUE
	OP_LOADNIL
	OP_GETUPVAL
	OP_SETUPVAL

	OP_GETTABUP
	OP_GETTABLE
	OP_GETI
	OP_GETFIELD

	OP_SETTABUP
	OP_SETTABLE
	OP_SETI
	OP_SETFIELD

	OP_NEWTABLE

	OP_SELF

	OP_ADDI

	OP_ADDK
	OP_SUBK
	OP_MULK
	OP_MODK
	OP_POWK
	OP_DIVK
	OP_IDIVK

	OP_BANDK
	OP_BORK
	OP_BXORK

	OP_SHLI
	OP_SHRI

	OP_ADD
	OP_SUB
	OP_MUL
	OP_MOD
	OP_POW
	OP_DIV
	OP_IDIV

	OP_BAND
	OP_BOR
	OP_BXOR
	OP_SHL
	OP_SHR

	OP_MMBIN
	OP_MMBINI
	OP_MMBINK

	OP_UNM
	OP_BNOT
	OP_NOT
	OP_LEN

	OP_CONCAT

	OP_CLOSE
	OP_TBC
	OP_JMP
	OP_EQ
	OP_LT
	OP_LE

	OP_EQK
	OP_EQI
	OP_LTI
	OP_LEI
	OP_GTI
	OP_GEI

	OP_TEST
	OP_TESTSET

	OP_CALL
	OP_TAILCALL

	OP_RETURN
	OP_RETURN0
	OP_RETURN1

	OP_FORLOOP
	OP_FORPREP

	OP_TFORPREP
	OP_TFORCALL
	OP_TFORLOOP

	OP_SETLIST

	OP_CLOSURE

	OP_VARARG

	OP_GETVARG

	OP_ERRNNIL

	OP_VARARGPREP

	OP_EXTRAARG

	NUM_OPCODES
)

/*
** Maximum size for the stack of a Lua function.
 */
const MAX_STACK = MAXARG_A

/*
** Invalid register (one more than last valid register).
 */
const NO_REG = MAXARG_A

/*
** Get/Set macros for instruction fields
 */
func GetOpCode(i lobject.LUInt32) OpCode {
	return OpCode((i >> POS_OP) & mask1(SIZE_OP, 0))
}

func SetOpCode(i *lobject.LUInt32, o OpCode) {
	*i = (*i & mask0(SIZE_OP, POS_OP)) | ((lobject.LUInt32(o) << POS_OP) & mask1(SIZE_OP, POS_OP))
}

func getarg(i lobject.LUInt32, pos uint, size uint) int {
	return int((i >> pos) & mask1(size, 0))
}

func setarg(i *lobject.LUInt32, v int, pos uint, size uint) {
	*i = (*i & mask0(size, pos)) | ((lobject.LUInt32(v) << pos) & mask1(size, pos))
}

func GETARG_A(i lobject.LUInt32) int {
	return getarg(i, POS_A, SIZE_A)
}

func SETARG_A(i *lobject.LUInt32, v int) {
	setarg(i, v, POS_A, SIZE_A)
}

func GETARG_B(i lobject.LUInt32) int {
	return getarg(i, POS_B, SIZE_B)
}

func SETARG_B(i *lobject.LUInt32, v int) {
	setarg(i, v, POS_B, SIZE_B)
}

func GETARG_C(i lobject.LUInt32) int {
	return getarg(i, POS_C, SIZE_C)
}

func SETARG_C(i *lobject.LUInt32, v int) {
	setarg(i, v, POS_C, SIZE_C)
}

func GETARG_Bx(i lobject.LUInt32) int {
	return getarg(i, POS_Bx, SIZE_Bx)
}

func SETARG_Bx(i *lobject.LUInt32, v int) {
	setarg(i, v, POS_Bx, SIZE_Bx)
}

func GETARG_Ax(i lobject.LUInt32) int {
	return getarg(i, POS_Ax, SIZE_Ax)
}

func SETARG_Ax(i *lobject.LUInt32, v int) {
	setarg(i, v, POS_Ax, SIZE_Ax)
}

func GETARG_sBx(i lobject.LUInt32) int {
	return getarg(i, POS_Bx, SIZE_Bx) - OFFSET_sBx
}

func SETARG_sBx(i *lobject.LUInt32, b int) {
	SETARG_Bx(i, b+OFFSET_sBx)
}

func GETARG_sJ(i lobject.LUInt32) int {
	return getarg(i, POS_sJ, SIZE_sJ) - OFFSET_sJ
}

func SETARG_sJ(i *lobject.LUInt32, j int) {
	setarg(i, j+OFFSET_sJ, POS_sJ, SIZE_sJ)
}

func GETARG_k(i lobject.LUInt32) int {
	return getarg(i, POS_k, 1)
}

func SETARG_k(i *lobject.LUInt32, v int) {
	setarg(i, v, POS_k, 1)
}

func GETARG_vB(i lobject.LUInt32) int {
	return getarg(i, POS_vB, SIZE_vB)
}

func GETARG_vC(i lobject.LUInt32) int {
	return getarg(i, POS_vC, SIZE_vC)
}

func Int2sC(i int) int {
	return i + OFFSET_sC
}

func sC2Int(i int) int {
	return i - OFFSET_sC
}

func GETARG_sC(i lobject.LUInt32) int {
	return sC2Int(GETARG_C(i))
}

func TESTARG_k(i lobject.LUInt32) int {
	return int(i & (1 << POS_k))
}

/*
** Create instruction macros
 */
func CREATE_ABCk(o OpCode, a, b, c, k int) lobject.LUInt32 {
	return (lobject.LUInt32(o) << POS_OP) |
		(lobject.LUInt32(a) << POS_A) |
		(lobject.LUInt32(b) << POS_B) |
		(lobject.LUInt32(c) << POS_C) |
		(lobject.LUInt32(k) << POS_k)
}

func CREATE_vABCk(o OpCode, a, b, c, k int) lobject.LUInt32 {
	return (lobject.LUInt32(o) << POS_OP) |
		(lobject.LUInt32(a) << POS_A) |
		(lobject.LUInt32(b) << POS_vB) |
		(lobject.LUInt32(c) << POS_vC) |
		(lobject.LUInt32(k) << POS_k)
}

func CREATE_ABx(o OpCode, a, bc int) lobject.LUInt32 {
	return (lobject.LUInt32(o) << POS_OP) |
		(lobject.LUInt32(a) << POS_A) |
		(lobject.LUInt32(bc) << POS_Bx)
}

func CREATE_Ax(o OpCode, a int) lobject.LUInt32 {
	return (lobject.LUInt32(o) << POS_OP) |
		(lobject.LUInt32(a) << POS_Ax)
}

func CREATE_sJ(o OpCode, j, k int) lobject.LUInt32 {
	return (lobject.LUInt32(o) << POS_OP) |
		(lobject.LUInt32(j+OFFSET_sJ) << POS_sJ) |
		(lobject.LUInt32(k) << POS_k)
}

/*
** opmode macro from C:
** #define opmode(mm,ot,it,t,a, m)    (((mm) << 7) | ((ot) << 6) | ((it) << 5) | ((t) << 4) | ((a) << 3) | (m))
** Format: MM<<7 | OT<<6 | IT<<5 | T<<4 | A<<3 | mode
 */
func opmode(mm, ot, it, t, a int, m OpMode) uint8 {
	return uint8((mm << 7) | (ot << 6) | (it << 5) | (t << 4) | (a << 3) | int(m))
}

/*
** OpMode array - instruction formats from lopcodes.c
** Format: bits 0-2 = mode, bit 3 = A, bit 4 = T, bit 5 = IT, bit 6 = OT, bit 7 = MM
 */
var luaP_opmodes = [NUM_OPCODES]uint8{
	/* OP_MOVE      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_LOADI     */ opmode(0, 0, 0, 0, 0, IAsBx),
	/* OP_LOADF     */ opmode(0, 0, 0, 0, 0, IAsBx),
	/* OP_LOADK     */ opmode(0, 0, 0, 0, 0, IABx),
	/* OP_LOADKX    */ opmode(0, 0, 0, 0, 0, IABx),
	/* OP_LOADFALSE */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_LFALSESKIP */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_LOADTRUE  */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_LOADNIL   */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_GETUPVAL  */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_SETUPVAL  */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_GETTABUP  */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_GETTABLE  */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_GETI      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_GETFIELD  */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_SETTABUP  */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_SETTABLE  */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_SETI      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_SETFIELD  */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_NEWTABLE  */ opmode(0, 0, 0, 0, 0, IvABC),
	/* OP_SELF      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_ADDI      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_ADDK      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_SUBK      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_MULK      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_MODK      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_POWK      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_DIVK      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_IDIVK     */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_BANDK     */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_BORK      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_BXORK     */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_SHLI      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_SHRI      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_ADD       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_SUB       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_MUL       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_MOD       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_POW       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_DIV       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_IDIV      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_BAND      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_BOR       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_BXOR      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_SHL       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_SHR       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_MMBIN     */ opmode(1, 0, 0, 0, 0, IABC),
	/* OP_MMBINI    */ opmode(1, 0, 0, 0, 0, IABC),
	/* OP_MMBINK    */ opmode(1, 0, 0, 0, 0, IABC),
	/* OP_UNM       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_BNOT      */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_NOT       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_LEN       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_CONCAT    */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_CLOSE     */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_TBC       */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_JMP       */ opmode(0, 0, 0, 0, 0, IsJ),
	/* OP_EQ        */ opmode(0, 0, 0, 1, 0, IABC),
	/* OP_LT        */ opmode(0, 0, 0, 1, 0, IABC),
	/* OP_LE        */ opmode(0, 0, 0, 1, 0, IABC),
	/* OP_EQK       */ opmode(0, 0, 0, 1, 0, IABC),
	/* OP_EQI       */ opmode(0, 0, 0, 1, 0, IABC),
	/* OP_LTI       */ opmode(0, 0, 0, 1, 0, IABC),
	/* OP_LEI       */ opmode(0, 0, 0, 1, 0, IABC),
	/* OP_GTI       */ opmode(0, 0, 0, 1, 0, IABC),
	/* OP_GEI       */ opmode(0, 0, 0, 1, 0, IABC),
	/* OP_TEST      */ opmode(0, 0, 0, 1, 0, IABC),
	/* OP_TESTSET   */ opmode(0, 0, 0, 1, 0, IABC),
	/* OP_CALL      */ opmode(0, 1, 1, 0, 1, IABC),
	/* OP_TAILCALL  */ opmode(0, 1, 1, 0, 1, IABC),
	/* OP_RETURN    */ opmode(0, 0, 1, 0, 0, IABC),
	/* OP_RETURN0   */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_RETURN1   */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_FORLOOP   */ opmode(0, 0, 0, 0, 0, IABx),
	/* OP_FORPREP   */ opmode(0, 0, 0, 0, 0, IABx),
	/* OP_TFORPREP  */ opmode(0, 0, 0, 0, 0, IABx),
	/* OP_TFORCALL  */ opmode(0, 0, 0, 0, 0, IABC),
	/* OP_TFORLOOP  */ opmode(0, 0, 0, 0, 0, IABx),
	/* OP_SETLIST   */ opmode(0, 1, 1, 0, 0, IvABC),
	/* OP_CLOSURE   */ opmode(0, 0, 0, 0, 0, IABx),
	/* OP_VARARG    */ opmode(0, 1, 0, 0, 1, IABC),
	/* OP_GETVARG   */ opmode(0, 0, 0, 0, 1, IABC),
	/* OP_ERRNNIL   */ opmode(0, 0, 0, 0, 0, IABx),
	/* OP_VARARGPREP */ opmode(0, 0, 1, 0, 0, IABC),
	/* OP_EXTRAARG  */ opmode(0, 0, 0, 0, 0, IAx),
}

func getOpMode(m OpCode) OpMode {
	return OpMode(luaP_opmodes[m] & 7)
}

func testAMode(m OpCode) bool {
	return (luaP_opmodes[m] & (1 << 3)) != 0
}

func testTMode(m OpCode) bool {
	return (luaP_opmodes[m] & (1 << 4)) != 0
}

func testITMode(m OpCode) bool {
	return (luaP_opmodes[m] & (1 << 5)) != 0
}

func testOTMode(m OpCode) bool {
	return (luaP_opmodes[m] & (1 << 6)) != 0
}

func testMMMode(m OpCode) bool {
	return (luaP_opmodes[m] & (1 << 7)) != 0
}

/*
** OpCode names for debugging
 */
var opNames = [NUM_OPCODES]string{
	"OP_MOVE",
	"OP_LOADI",
	"OP_LOADF",
	"OP_LOADK",
	"OP_LOADKX",
	"OP_LOADFALSE",
	"OP_LFALSESKIP",
	"OP_LOADTRUE",
	"OP_LOADNIL",
	"OP_GETUPVAL",
	"OP_SETUPVAL",
	"OP_GETTABUP",
	"OP_GETTABLE",
	"OP_GETI",
	"OP_GETFIELD",
	"OP_SETTABUP",
	"OP_SETTABLE",
	"OP_SETI",
	"OP_SETFIELD",
	"OP_NEWTABLE",
	"OP_SELF",
	"OP_ADDI",
	"OP_ADDK",
	"OP_SUBK",
	"OP_MULK",
	"OP_MODK",
	"OP_POWK",
	"OP_DIVK",
	"OP_IDIVK",
	"OP_BANDK",
	"OP_BORK",
	"OP_BXORK",
	"OP_SHLI",
	"OP_SHRI",
	"OP_ADD",
	"OP_SUB",
	"OP_MUL",
	"OP_MOD",
	"OP_POW",
	"OP_DIV",
	"OP_IDIV",
	"OP_BAND",
	"OP_BOR",
	"OP_BXOR",
	"OP_SHL",
	"OP_SHR",
	"OP_MMBIN",
	"OP_MMBINI",
	"OP_MMBINK",
	"OP_UNM",
	"OP_BNOT",
	"OP_NOT",
	"OP_LEN",
	"OP_CONCAT",
	"OP_CLOSE",
	"OP_TBC",
	"OP_JMP",
	"OP_EQ",
	"OP_LT",
	"OP_LE",
	"OP_EQK",
	"OP_EQI",
	"OP_LTI",
	"OP_LEI",
	"OP_GTI",
	"OP_GEI",
	"OP_TEST",
	"OP_TESTSET",
	"OP_CALL",
	"OP_TAILCALL",
	"OP_RETURN",
	"OP_RETURN0",
	"OP_RETURN1",
	"OP_FORLOOP",
	"OP_FORPREP",
	"OP_TFORPREP",
	"OP_TFORCALL",
	"OP_TFORLOOP",
	"OP_SETLIST",
	"OP_CLOSURE",
	"OP_VARARG",
	"OP_GETVARG",
	"OP_ERRNNIL",
	"OP_VARARGPREP",
	"OP_EXTRAARG",
}

/*
** Check whether type 'int' has at least 'b' + 1 bits.
 */
func hasBits(b int) bool {
	return (^uint(0) >> b) >= 1
}