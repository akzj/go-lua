// Opcode metadata tables — populated from lua-master/lopcodes.c and lopnames.h.
//
// OpModes byte layout: bit7=mm, bit6=ot, bit5=it, bit4=t, bit3=a, bits[2:0]=format
// Format values: iABC=0, ivABC=1, iABx=2, iAsBx=3, iAx=4, isJ=5
//
// C macro: opmode(mm, ot, it, t, a, m) = (mm<<7)|(ot<<6)|(it<<5)|(t<<4)|(a<<3)|m
package api

// opmode computes the mode byte exactly as the C macro does.
func opmode(mm, ot, it, t, a int, m OpMode) byte {
	return byte(mm<<7 | ot<<6 | it<<5 | t<<4 | a<<3 | int(m))
}

func init() {
	// ORDER OP — from lua-master/lopnames.h
	OpNames = [NumOpcodes]string{
		"MOVE",       // 0
		"LOADI",      // 1
		"LOADF",      // 2
		"LOADK",      // 3
		"LOADKX",     // 4
		"LOADFALSE",  // 5
		"LFALSESKIP", // 6
		"LOADTRUE",   // 7
		"LOADNIL",    // 8
		"GETUPVAL",   // 9
		"SETUPVAL",   // 10
		"GETTABUP",   // 11
		"GETTABLE",   // 12
		"GETI",       // 13
		"GETFIELD",   // 14
		"SETTABUP",   // 15
		"SETTABLE",   // 16
		"SETI",       // 17
		"SETFIELD",   // 18
		"NEWTABLE",   // 19
		"SELF",       // 20
		"ADDI",       // 21
		"ADDK",       // 22
		"SUBK",       // 23
		"MULK",       // 24
		"MODK",       // 25
		"POWK",       // 26
		"DIVK",       // 27
		"IDIVK",      // 28
		"BANDK",      // 29
		"BORK",       // 30
		"BXORK",      // 31
		"SHLI",       // 32
		"SHRI",       // 33
		"ADD",        // 34
		"SUB",        // 35
		"MUL",        // 36
		"MOD",        // 37
		"POW",        // 38
		"DIV",        // 39
		"IDIV",       // 40
		"BAND",       // 41
		"BOR",        // 42
		"BXOR",       // 43
		"SHL",        // 44
		"SHR",        // 45
		"MMBIN",      // 46
		"MMBINI",     // 47
		"MMBINK",     // 48
		"UNM",        // 49
		"BNOT",       // 50
		"NOT",        // 51
		"LEN",        // 52
		"CONCAT",     // 53
		"CLOSE",      // 54
		"TBC",        // 55
		"JMP",        // 56
		"EQ",         // 57
		"LT",         // 58
		"LE",         // 59
		"EQK",        // 60
		"EQI",        // 61
		"LTI",        // 62
		"LEI",        // 63
		"GTI",        // 64
		"GEI",        // 65
		"TEST",       // 66
		"TESTSET",    // 67
		"CALL",       // 68
		"TAILCALL",   // 69
		"RETURN",     // 70
		"RETURN0",    // 71
		"RETURN1",    // 72
		"FORLOOP",    // 73
		"FORPREP",    // 74
		"TFORPREP",   // 75
		"TFORCALL",   // 76
		"TFORLOOP",   // 77
		"SETLIST",    // 78
		"CLOSURE",    // 79
		"VARARG",     // 80
		"GETVARG",    // 81
		"ERRNNIL",    // 82
		"VARARGPREP", // 83
		"EXTRAARG",   // 84
	}

	// ORDER OP — from lua-master/lopcodes.c luaP_opmodes[]
	//                    MM OT IT T  A  mode
	OpModes = [NumOpcodes]byte{
		opmode(0, 0, 0, 0, 1, IABC),  // OP_MOVE
		opmode(0, 0, 0, 0, 1, IAsBx), // OP_LOADI
		opmode(0, 0, 0, 0, 1, IAsBx), // OP_LOADF
		opmode(0, 0, 0, 0, 1, IABx),  // OP_LOADK
		opmode(0, 0, 0, 0, 1, IABx),  // OP_LOADKX
		opmode(0, 0, 0, 0, 1, IABC),  // OP_LOADFALSE
		opmode(0, 0, 0, 0, 1, IABC),  // OP_LFALSESKIP
		opmode(0, 0, 0, 0, 1, IABC),  // OP_LOADTRUE
		opmode(0, 0, 0, 0, 1, IABC),  // OP_LOADNIL
		opmode(0, 0, 0, 0, 1, IABC),  // OP_GETUPVAL
		opmode(0, 0, 0, 0, 0, IABC),  // OP_SETUPVAL
		opmode(0, 0, 0, 0, 1, IABC),  // OP_GETTABUP
		opmode(0, 0, 0, 0, 1, IABC),  // OP_GETTABLE
		opmode(0, 0, 0, 0, 1, IABC),  // OP_GETI
		opmode(0, 0, 0, 0, 1, IABC),  // OP_GETFIELD
		opmode(0, 0, 0, 0, 0, IABC),  // OP_SETTABUP
		opmode(0, 0, 0, 0, 0, IABC),  // OP_SETTABLE
		opmode(0, 0, 0, 0, 0, IABC),  // OP_SETI
		opmode(0, 0, 0, 0, 0, IABC),  // OP_SETFIELD
		opmode(0, 0, 0, 0, 1, IVABC), // OP_NEWTABLE
		opmode(0, 0, 0, 0, 1, IABC),  // OP_SELF
		opmode(0, 0, 0, 0, 1, IABC),  // OP_ADDI
		opmode(0, 0, 0, 0, 1, IABC),  // OP_ADDK
		opmode(0, 0, 0, 0, 1, IABC),  // OP_SUBK
		opmode(0, 0, 0, 0, 1, IABC),  // OP_MULK
		opmode(0, 0, 0, 0, 1, IABC),  // OP_MODK
		opmode(0, 0, 0, 0, 1, IABC),  // OP_POWK
		opmode(0, 0, 0, 0, 1, IABC),  // OP_DIVK
		opmode(0, 0, 0, 0, 1, IABC),  // OP_IDIVK
		opmode(0, 0, 0, 0, 1, IABC),  // OP_BANDK
		opmode(0, 0, 0, 0, 1, IABC),  // OP_BORK
		opmode(0, 0, 0, 0, 1, IABC),  // OP_BXORK
		opmode(0, 0, 0, 0, 1, IABC),  // OP_SHLI
		opmode(0, 0, 0, 0, 1, IABC),  // OP_SHRI
		opmode(0, 0, 0, 0, 1, IABC),  // OP_ADD
		opmode(0, 0, 0, 0, 1, IABC),  // OP_SUB
		opmode(0, 0, 0, 0, 1, IABC),  // OP_MUL
		opmode(0, 0, 0, 0, 1, IABC),  // OP_MOD
		opmode(0, 0, 0, 0, 1, IABC),  // OP_POW
		opmode(0, 0, 0, 0, 1, IABC),  // OP_DIV
		opmode(0, 0, 0, 0, 1, IABC),  // OP_IDIV
		opmode(0, 0, 0, 0, 1, IABC),  // OP_BAND
		opmode(0, 0, 0, 0, 1, IABC),  // OP_BOR
		opmode(0, 0, 0, 0, 1, IABC),  // OP_BXOR
		opmode(0, 0, 0, 0, 1, IABC),  // OP_SHL
		opmode(0, 0, 0, 0, 1, IABC),  // OP_SHR
		opmode(1, 0, 0, 0, 0, IABC),  // OP_MMBIN
		opmode(1, 0, 0, 0, 0, IABC),  // OP_MMBINI
		opmode(1, 0, 0, 0, 0, IABC),  // OP_MMBINK
		opmode(0, 0, 0, 0, 1, IABC),  // OP_UNM
		opmode(0, 0, 0, 0, 1, IABC),  // OP_BNOT
		opmode(0, 0, 0, 0, 1, IABC),  // OP_NOT
		opmode(0, 0, 0, 0, 1, IABC),  // OP_LEN
		opmode(0, 0, 0, 0, 1, IABC),  // OP_CONCAT
		opmode(0, 0, 0, 0, 0, IABC),  // OP_CLOSE
		opmode(0, 0, 0, 0, 0, IABC),  // OP_TBC
		opmode(0, 0, 0, 0, 0, ISJ),   // OP_JMP
		opmode(0, 0, 0, 1, 0, IABC),  // OP_EQ
		opmode(0, 0, 0, 1, 0, IABC),  // OP_LT
		opmode(0, 0, 0, 1, 0, IABC),  // OP_LE
		opmode(0, 0, 0, 1, 0, IABC),  // OP_EQK
		opmode(0, 0, 0, 1, 0, IABC),  // OP_EQI
		opmode(0, 0, 0, 1, 0, IABC),  // OP_LTI
		opmode(0, 0, 0, 1, 0, IABC),  // OP_LEI
		opmode(0, 0, 0, 1, 0, IABC),  // OP_GTI
		opmode(0, 0, 0, 1, 0, IABC),  // OP_GEI
		opmode(0, 0, 0, 1, 0, IABC),  // OP_TEST
		opmode(0, 0, 0, 1, 1, IABC),  // OP_TESTSET
		opmode(0, 1, 1, 0, 1, IABC),  // OP_CALL
		opmode(0, 1, 1, 0, 1, IABC),  // OP_TAILCALL
		opmode(0, 0, 1, 0, 0, IABC),  // OP_RETURN
		opmode(0, 0, 0, 0, 0, IABC),  // OP_RETURN0
		opmode(0, 0, 0, 0, 0, IABC),  // OP_RETURN1
		opmode(0, 0, 0, 0, 1, IABx),  // OP_FORLOOP
		opmode(0, 0, 0, 0, 1, IABx),  // OP_FORPREP
		opmode(0, 0, 0, 0, 0, IABx),  // OP_TFORPREP
		opmode(0, 0, 0, 0, 0, IABC),  // OP_TFORCALL
		opmode(0, 0, 0, 0, 1, IABx),  // OP_TFORLOOP
		opmode(0, 0, 1, 0, 0, IVABC), // OP_SETLIST
		opmode(0, 0, 0, 0, 1, IABx),  // OP_CLOSURE
		opmode(0, 1, 0, 0, 1, IABC),  // OP_VARARG
		opmode(0, 0, 0, 0, 1, IABC),  // OP_GETVARG
		opmode(0, 0, 0, 0, 0, IABx),  // OP_ERRNNIL
		opmode(0, 0, 1, 0, 0, IABC),  // OP_VARARGPREP
		opmode(0, 0, 0, 0, 0, IAx),   // OP_EXTRAARG
	}
}
