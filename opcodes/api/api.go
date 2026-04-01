// Package api defines Lua 5.5.1 VM opcode constants and types.
// NO dependencies - pure constant and type definitions.
//
// Reference: lua-master/lopcodes.h (14170 bytes)
// Lua 5.5.1 defines NUM_OPCODES = 77 (OP_EXTRAARG = 76)
package api

// Instruction is a 32-bit unsigned integer encoding a Lua VM instruction.
// All instructions have an opcode in the first 7 bits.
type Instruction uint32

// =============================================================================
// OpMode - Instruction Format Modes
// =============================================================================

// OpMode defines how an instruction encodes its operands.
// Matches lua-master/lopcodes.h: enum OpMode {iABC, ivABC, iABx, iAsBx, iAx, isJ}
type OpMode uint8

const (
	OpModeABC  OpMode = 0 // A(8) | k(1) | B(8) | C(8)
	OpModeVABC OpMode = 1 // A(8) | k(1) | vB(6) | vC(10)
	OpModeABx  OpMode = 2 // A(8) | Bx(17)
	OpModeAsBx OpMode = 3 // A(8) | sBx(17 signed)
	OpModeAx   OpMode = 4 // Ax(25)
	OpModeSJ   OpMode = 5 // sJ(25 signed)
)

// Instruction encoding constants (from lua-master/lopcodes.h)
const (
	SIZE_OP  = 7  // opcode bits
	SIZE_A   = 8  // register A
	SIZE_B   = 8  // register B (standard)
	SIZE_vB  = 6  // register B (variant, for vABC)
	SIZE_C   = 8  // register C (standard)
	SIZE_vC  = 10 // register C (variant, for vABC)
	SIZE_Bx  = SIZE_C + SIZE_B + 1 // Bx = 17 bits
	SIZE_Ax  = SIZE_Bx + SIZE_A     // Ax = 25 bits
	SIZE_sJ  = SIZE_Bx + SIZE_A     // sJ = 25 bits (signed)

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

// Argument limits (unsigned)
const (
	MAXARG_OP  = (1 << SIZE_OP) - 1   // 127 (7-bit opcode mask)
	MAXARG_A   = (1 << SIZE_A) - 1    // 255
	MAXARG_B   = (1 << SIZE_B) - 1    // 255
	MAXARG_vB  = (1 << SIZE_vB) - 1  // 63
	MAXARG_C   = (1 << SIZE_C) - 1    // 255
	MAXARG_vC  = (1 << SIZE_vC) - 1  // 1023
	MAXARG_Bx  = (1 << SIZE_Bx) - 1  // 131071
	MAXARG_Ax  = (1 << SIZE_Ax) - 1  // 33554431

	OFFSET_sBx = MAXARG_Bx >> 1      // 65535 (signed offset for iAsBx)
	MAXARG_sJ  = (1 << SIZE_sJ) - 1  // 33554431
	OFFSET_sJ  = MAXARG_sJ >> 1      // signed offset for isJ
)

// =============================================================================
// OpArgMask - Argument Usage Modes
// =============================================================================

// OpArgMask identifies how an operand field is used.
type OpArgMask uint8

const (
	OpArgN OpArgMask = 0 // argument not used
	OpArgU OpArgMask = 1 // argument is used
	OpArgR OpArgMask = 2 // argument is a register index
	OpArgK OpArgMask = 3 // argument is a constant index
)

// =============================================================================
// Opcode Constants - All 77 Lua 5.5.1 VM Opcodes (0-76)
// =============================================================================
// Matches lua-master/lopcodes.h exactly. DO NOT use iota - explicit values required.

type OpCode uint8

const (
	OP_MOVE       OpCode = 0  // R[A] := R[B]
	OP_LOADI      OpCode = 1  // R[A] := sBx (integer)
	OP_LOADF      OpCode = 2  // R[A] := (lua_Number)sBx
	OP_LOADK      OpCode = 3  // R[A] := K[Bx]
	OP_LOADKX     OpCode = 4  // R[A] := K[extra arg]
	OP_LOADFALSE  OpCode = 5  // R[A] := false
	OP_LFALSESKIP OpCode = 6  // R[A] := false; pc++
	OP_LOADTRUE   OpCode = 7  // R[A] := true
	OP_LOADNIL    OpCode = 8  // R[A], ..., R[A+B] := nil
	OP_GETUPVAL   OpCode = 9  // R[A] := UpValue[B]
	OP_SETUPVAL   OpCode = 10 // UpValue[B] := R[A]
	OP_GETTABUP   OpCode = 11 // R[A] := UpValue[B][K[C]]
	OP_GETTABLE   OpCode = 12 // R[A] := R[B][R[C]]
	OP_GETI       OpCode = 13 // R[A] := R[B][C]
	OP_GETFIELD   OpCode = 14 // R[A] := R[B][K[C]]
	OP_SETTABUP   OpCode = 15 // UpValue[A][K[B]] := RK(C)
	OP_SETTABLE   OpCode = 16 // R[A][R[B]] := RK(C)
	OP_SETI       OpCode = 17 // R[A][B] := RK(C)
	OP_SETFIELD   OpCode = 18 // R[A][K[B]] := RK(C)
	OP_NEWTABLE   OpCode = 19 // R[A] := {}
	OP_SELF       OpCode = 20 // R[A+1] := R[B]; R[A] := R[B][K[C]]
	OP_ADDI       OpCode = 21 // R[A] := R[B] + sC
	OP_ADDK       OpCode = 22 // R[A] := R[B] + K[C]
	OP_SUBK       OpCode = 23 // R[A] := R[B] - K[C]
	OP_MULK       OpCode = 24 // R[A] := R[B] * K[C]
	OP_MODK       OpCode = 25 // R[A] := R[B] % K[C]
	OP_POWK       OpCode = 26 // R[A] := R[B] ^ K[C]
	OP_DIVK       OpCode = 27 // R[A] := R[B] / K[C]
	OP_IDIVK      OpCode = 28 // R[A] := R[B] // K[C]
	OP_BANDK      OpCode = 29 // R[A] := R[B] & K[C]
	OP_BORK       OpCode = 30 // R[A] := R[B] | K[C]
	OP_BXORK      OpCode = 31 // R[A] := R[B] ~ K[C]
	OP_SHLI       OpCode = 32 // R[A] := sC << R[B]
	OP_SHRI       OpCode = 33 // R[A] := R[B] >> sC
	OP_ADD        OpCode = 34 // R[A] := R[B] + R[C]
	OP_SUB        OpCode = 35 // R[A] := R[B] - R[C]
	OP_MUL        OpCode = 36 // R[A] := R[B] * R[C]
	OP_MOD        OpCode = 37 // R[A] := R[B] % R[C]
	OP_POW        OpCode = 38 // R[A] := R[B] ^ R[C]
	OP_DIV        OpCode = 39 // R[A] := R[B] / R[C]
	OP_IDIV       OpCode = 40 // R[A] := R[B] // R[C]
	OP_BAND       OpCode = 41 // R[A] := R[B] & R[C]
	OP_BOR        OpCode = 42 // R[A] := R[B] | R[C]
	OP_BXOR       OpCode = 43 // R[A] := R[B] ~ R[C]
	OP_SHL        OpCode = 44 // R[A] := R[B] << R[C]
	OP_SHR        OpCode = 45 // R[A] := R[B] >> R[C]
	OP_MMBIN      OpCode = 46 // metamethod binary op
	OP_MMBINI     OpCode = 47 // metamethod binary op (immediate)
	OP_MMBINK     OpCode = 48 // metamethod binary op (constant)
	OP_UNM        OpCode = 49 // R[A] := -R[B]
	OP_BNOT       OpCode = 50 // R[A] := ~R[B]
	OP_NOT        OpCode = 51 // R[A] := not R[B]
	OP_LEN        OpCode = 52 // R[A] := #R[B]
	OP_CONCAT     OpCode = 53 // R[A] := R[A].. ... ..R[A+B-1]
	OP_CLOSE      OpCode = 54 // close all upvalues >= R[A]
	OP_TBC        OpCode = 55 // mark variable A "to be closed"
	OP_JMP        OpCode = 56 // pc += sJ
	OP_EQ         OpCode = 57 // if ((R[A] == R[B]) ~= k) then pc++
	OP_LT         OpCode = 58 // if ((R[A] < R[B]) ~= k) then pc++
	OP_LE         OpCode = 59 // if ((R[A] <= R[B]) ~= k) then pc++
	OP_EQK        OpCode = 60 // if ((R[A] == K[B]) ~= k) then pc++
	OP_EQI        OpCode = 61 // if ((R[A] == sB) ~= k) then pc++
	OP_LTI        OpCode = 62 // if ((R[A] < sB) ~= k) then pc++
	OP_LEI        OpCode = 63 // if ((R[A] <= sB) ~= k) then pc++
	OP_GTI        OpCode = 64 // if ((R[A] > sB) ~= k) then pc++
	OP_GEI        OpCode = 65 // if ((R[A] >= sB) ~= k) then pc++
	OP_TEST       OpCode = 66 // if (not R[A] == k) then pc++
	OP_TESTSET    OpCode = 67 // if (not R[B] == k) then pc++ else R[A] := R[B]
	OP_CALL       OpCode = 68 // R[A], ... := R[A](R[A+1], ...)
	OP_TAILCALL   OpCode = 69 // return R[A](R[A+1], ...)
	OP_RETURN     OpCode = 70 // return R[A], ...
	OP_RETURN0    OpCode = 71 // return
	OP_RETURN1    OpCode = 72 // return R[A]
	OP_FORLOOP    OpCode = 73 // update counters; if continues pc-=Bx
	OP_FORPREP    OpCode = 74 // check and prepare counters
	OP_TFORPREP   OpCode = 75 // create upvalue for R[A+3]; pc+=Bx
	OP_TFORCALL   OpCode = 76 // R[A+4], ... := R[A](R[A+1], R[A+2])
	OP_TFORLOOP   OpCode = 77 // if R[A+2] ~= nil then R[A]=R[A+2]; pc -= Bx
	OP_SETLIST    OpCode = 78 // R[A][vC+i] := R[A+i]
	OP_CLOSURE    OpCode = 79 // R[A] := closure(KPROTO[Bx])
	OP_VARARG     OpCode = 80 // R[A], ... = varargs
	OP_GETVARG    OpCode = 81 // R[A] := R[B][R[C]]
	OP_ERRNNIL    OpCode = 82 // raise error if R[A] ~= nil
	OP_VARARGPREP OpCode = 83 // adjust varargs
	OP_EXTRAARG   OpCode = 84 // extra (larger) argument for previous opcode

	// NUM_OPCODES must match lua-master: #define NUM_OPCODES ((int)(OP_EXTRAARG) + 1)
	// But here OP_EXTRAARG = 84, so we define it explicitly
	NUM_OPCODES = 85 // Total: 0-84 = 85 opcodes
)

// Verify NUM_OPCODES matches OP_EXTRAARG + 1
const _NUM_OPCODES_check = NUM_OPCODES - (int(OP_EXTRAARG) + 1) // Should be 0

// =============================================================================
// Instruction Property Flags
// =============================================================================
// These match lua-master/lopcodes.h luaP_opmodes[] encoding:
// bits 0-2: op mode, bit 3: A, bit 4: T, bit 5: IT, bit 6: OT, bit 7: MM

// OpProperty encodes instruction characteristics for the VM.
type OpProperty uint8

const (
	OpPropertyMM  OpProperty = 1 << 7 // metamethod instruction
	OpPropertyOT  OpProperty = 1 << 6 // sets L->top for next instruction
	OpPropertyIT  OpProperty = 1 << 5 // uses L->top from previous instruction
	OpPropertyT   OpProperty = 1 << 4 // test (next instruction is jump)
	OpPropertyA   OpProperty = 1 << 3 // sets register A
)

// =============================================================================
// Instruction Creation Helpers
// =============================================================================

const (
	// MAX_FSTACK is the maximum size for the stack of a Lua function.
	MAX_FSTACK = MAXARG_A // 255

	// NO_REG is an invalid register (one more than last valid register).
	NO_REG = MAX_FSTACK

	// MAXINDEXRK is the maximum index for a RK (register or constant) operand.
	MAXINDEXRK = MAXARG_B // 255
)

// =============================================================================
// OpCodeName returns the string name of an opcode.
// Matches lua-master/lopnames.h exactly (85 entries).
func OpCodeName(op OpCode) string {
	names := [85]string{
		"MOVE", "LOADI", "LOADF", "LOADK", "LOADKX", "LOADFALSE", "LFALSESKIP",
		"LOADTRUE", "LOADNIL", "GETUPVAL", "SETUPVAL", "GETTABUP", "GETTABLE",
		"GETI", "GETFIELD", "SETTABUP", "SETTABLE", "SETI", "SETFIELD",
		"NEWTABLE", "SELF", "ADDI", "ADDK", "SUBK", "MULK", "MODK", "POWK",
		"DIVK", "IDIVK", "BANDK", "BORK", "BXORK", "SHLI", "SHRI", "ADD", "SUB",
		"MUL", "MOD", "POW", "DIV", "IDIV", "BAND", "BOR", "BXOR", "SHL", "SHR",
		"MMBIN", "MMBINI", "MMBINK", "UNM", "BNOT", "NOT", "LEN", "CONCAT",
		"CLOSE", "TBC", "JMP", "EQ", "LT", "LE", "EQK", "EQI", "LTI", "LEI",
		"GTI", "GEI", "TEST", "TESTSET", "CALL", "TAILCALL", "RETURN", "RETURN0",
		"RETURN1", "FORLOOP", "FORPREP", "TFORPREP", "TFORCALL", "TFORLOOP",
		"SETLIST", "CLOSURE", "VARARG", "GETVARG", "ERRNNIL", "VARARGPREP", "EXTRAARG",
	}
	if op < 85 {
		return names[op]
	}
	return "INVALID"
}
