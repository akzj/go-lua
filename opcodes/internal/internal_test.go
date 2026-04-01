package internal

import (
	"testing"

	"github.com/akzj/go-lua/opcodes/api"
)

func TestOpcodeCount(t *testing.T) {
	// Verify we have exactly 85 opcodes
	if len(luaP_opmodes) != 85 {
		t.Errorf("luaP_opmodes has %d entries, expected 85", len(luaP_opmodes))
	}
	if api.NUM_OPCODES != 85 {
		t.Errorf("NUM_OPCODES is %d, expected 85", api.NUM_OPCODES)
	}
}

func TestOpModes(t *testing.T) {
	tests := []struct {
		op       api.OpCode
		mode     api.OpMode
		name     string
	}{
		{api.OP_MOVE, api.OpModeABC, "OP_MOVE"},
		{api.OP_LOADI, api.OpModeAsBx, "OP_LOADI"},
		{api.OP_LOADF, api.OpModeAsBx, "OP_LOADF"},
		{api.OP_LOADK, api.OpModeABx, "OP_LOADK"},
		{api.OP_LOADKX, api.OpModeABx, "OP_LOADKX"},
		{api.OP_LOADFALSE, api.OpModeABC, "OP_LOADFALSE"},
		{api.OP_LFALSESKIP, api.OpModeABC, "OP_LFALSESKIP"},
		{api.OP_LOADTRUE, api.OpModeABC, "OP_LOADTRUE"},
		{api.OP_LOADNIL, api.OpModeABC, "OP_LOADNIL"},
		{api.OP_GETUPVAL, api.OpModeABC, "OP_GETUPVAL"},
		{api.OP_SETUPVAL, api.OpModeABC, "OP_SETUPVAL"},
		{api.OP_GETTABUP, api.OpModeABC, "OP_GETTABUP"},
		{api.OP_GETTABLE, api.OpModeABC, "OP_GETTABLE"},
		{api.OP_GETI, api.OpModeABC, "OP_GETI"},
		{api.OP_GETFIELD, api.OpModeABC, "OP_GETFIELD"},
		{api.OP_SETTABUP, api.OpModeABC, "OP_SETTABUP"},
		{api.OP_SETTABLE, api.OpModeABC, "OP_SETTABLE"},
		{api.OP_SETI, api.OpModeABC, "OP_SETI"},
		{api.OP_SETFIELD, api.OpModeABC, "OP_SETFIELD"},
		{api.OP_NEWTABLE, api.OpModeVABC, "OP_NEWTABLE"},
		{api.OP_SELF, api.OpModeABC, "OP_SELF"},
		{api.OP_ADDI, api.OpModeABC, "OP_ADDI"},
		{api.OP_ADDK, api.OpModeABC, "OP_ADDK"},
		{api.OP_SUBK, api.OpModeABC, "OP_SUBK"},
		{api.OP_MULK, api.OpModeABC, "OP_MULK"},
		{api.OP_MODK, api.OpModeABC, "OP_MODK"},
		{api.OP_POWK, api.OpModeABC, "OP_POWK"},
		{api.OP_DIVK, api.OpModeABC, "OP_DIVK"},
		{api.OP_IDIVK, api.OpModeABC, "OP_IDIVK"},
		{api.OP_BANDK, api.OpModeABC, "OP_BANDK"},
		{api.OP_BORK, api.OpModeABC, "OP_BORK"},
		{api.OP_BXORK, api.OpModeABC, "OP_BXORK"},
		{api.OP_SHLI, api.OpModeABC, "OP_SHLI"},
		{api.OP_SHRI, api.OpModeABC, "OP_SHRI"},
		{api.OP_ADD, api.OpModeABC, "OP_ADD"},
		{api.OP_SUB, api.OpModeABC, "OP_SUB"},
		{api.OP_MUL, api.OpModeABC, "OP_MUL"},
		{api.OP_MOD, api.OpModeABC, "OP_MOD"},
		{api.OP_POW, api.OpModeABC, "OP_POW"},
		{api.OP_DIV, api.OpModeABC, "OP_DIV"},
		{api.OP_IDIV, api.OpModeABC, "OP_IDIV"},
		{api.OP_BAND, api.OpModeABC, "OP_BAND"},
		{api.OP_BOR, api.OpModeABC, "OP_BOR"},
		{api.OP_BXOR, api.OpModeABC, "OP_BXOR"},
		{api.OP_SHL, api.OpModeABC, "OP_SHL"},
		{api.OP_SHR, api.OpModeABC, "OP_SHR"},
		{api.OP_MMBIN, api.OpModeABC, "OP_MMBIN"},
		{api.OP_MMBINI, api.OpModeABC, "OP_MMBINI"},
		{api.OP_MMBINK, api.OpModeABC, "OP_MMBINK"},
		{api.OP_UNM, api.OpModeABC, "OP_UNM"},
		{api.OP_BNOT, api.OpModeABC, "OP_BNOT"},
		{api.OP_NOT, api.OpModeABC, "OP_NOT"},
		{api.OP_LEN, api.OpModeABC, "OP_LEN"},
		{api.OP_CONCAT, api.OpModeABC, "OP_CONCAT"},
		{api.OP_CLOSE, api.OpModeABC, "OP_CLOSE"},
		{api.OP_TBC, api.OpModeABC, "OP_TBC"},
		{api.OP_JMP, api.OpModeSJ, "OP_JMP"},
		{api.OP_EQ, api.OpModeABC, "OP_EQ"},
		{api.OP_LT, api.OpModeABC, "OP_LT"},
		{api.OP_LE, api.OpModeABC, "OP_LE"},
		{api.OP_EQK, api.OpModeABC, "OP_EQK"},
		{api.OP_EQI, api.OpModeABC, "OP_EQI"},
		{api.OP_LTI, api.OpModeABC, "OP_LTI"},
		{api.OP_LEI, api.OpModeABC, "OP_LEI"},
		{api.OP_GTI, api.OpModeABC, "OP_GTI"},
		{api.OP_GEI, api.OpModeABC, "OP_GEI"},
		{api.OP_TEST, api.OpModeABC, "OP_TEST"},
		{api.OP_TESTSET, api.OpModeABC, "OP_TESTSET"},
		{api.OP_CALL, api.OpModeABC, "OP_CALL"},
		{api.OP_TAILCALL, api.OpModeABC, "OP_TAILCALL"},
		{api.OP_RETURN, api.OpModeABC, "OP_RETURN"},
		{api.OP_RETURN0, api.OpModeABC, "OP_RETURN0"},
		{api.OP_RETURN1, api.OpModeABC, "OP_RETURN1"},
		{api.OP_FORLOOP, api.OpModeABx, "OP_FORLOOP"},
		{api.OP_FORPREP, api.OpModeABx, "OP_FORPREP"},
		{api.OP_TFORPREP, api.OpModeABx, "OP_TFORPREP"},
		{api.OP_TFORCALL, api.OpModeABC, "OP_TFORCALL"},
		{api.OP_TFORLOOP, api.OpModeABx, "OP_TFORLOOP"},
		{api.OP_SETLIST, api.OpModeVABC, "OP_SETLIST"},
		{api.OP_CLOSURE, api.OpModeABx, "OP_CLOSURE"},
		{api.OP_VARARG, api.OpModeABC, "OP_VARARG"},
		{api.OP_GETVARG, api.OpModeABC, "OP_GETVARG"},
		{api.OP_ERRNNIL, api.OpModeABx, "OP_ERRNNIL"},
		{api.OP_VARARGPREP, api.OpModeABC, "OP_VARARGPREP"},
		{api.OP_EXTRAARG, api.OpModeAx, "OP_EXTRAARG"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if GetOpMode(tt.op) != tt.mode {
				t.Errorf("GetOpMode(%s) = %v, want %v", tt.name, GetOpMode(tt.op), tt.mode)
			}
		})
	}
}

func TestModeBits(t *testing.T) {
	// Test A mode bits for some opcodes
	tests := []struct {
		op       api.OpCode
		testAMode bool
		testTMode bool
		testITMode bool
		testOTMode bool
		testMMMode bool
		name      string
	}{
		{api.OP_MOVE, true, false, false, false, false, "OP_MOVE"},
		{api.OP_SETUPVAL, false, false, false, false, false, "OP_SETUPVAL"},
		{api.OP_JMP, false, false, false, false, false, "OP_JMP"},
		{api.OP_EQ, false, true, false, false, false, "OP_EQ"},
		{api.OP_TESTSET, true, true, false, false, false, "OP_TESTSET"},
		{api.OP_CALL, true, false, true, true, false, "OP_CALL"},
		{api.OP_MMBIN, false, false, false, false, true, "OP_MMBIN"},
		{api.OP_RETURN, false, false, true, false, false, "OP_RETURN"},
		{api.OP_VARARG, true, false, false, true, false, "OP_VARARG"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if TestAMode(tt.op) != tt.testAMode {
				t.Errorf("TestAMode(%s) = %v, want %v", tt.name, TestAMode(tt.op), tt.testAMode)
			}
			if TestTMode(tt.op) != tt.testTMode {
				t.Errorf("TestTMode(%s) = %v, want %v", tt.name, TestTMode(tt.op), tt.testTMode)
			}
			if TestITMode(tt.op) != tt.testITMode {
				t.Errorf("TestITMode(%s) = %v, want %v", tt.name, TestITMode(tt.op), tt.testITMode)
			}
			if TestOTMode(tt.op) != tt.testOTMode {
				t.Errorf("TestOTMode(%s) = %v, want %v", tt.name, TestOTMode(tt.op), tt.testOTMode)
			}
			if TestMMMode(tt.op) != tt.testMMMode {
				t.Errorf("TestMMMode(%s) = %v, want %v", tt.name, TestMMMode(tt.op), tt.testMMMode)
			}
		})
	}
}

func TestCreateABC(t *testing.T) {
	// Test ABC instruction creation and extraction
	instr := CreateABC(api.OP_MOVE, 10, 20, 30)
	
	if GetOpCode(instr) != api.OP_MOVE {
		t.Errorf("GetOpCode() = %v, want OP_MOVE", GetOpCode(instr))
	}
	if GetArgA(instr) != 10 {
		t.Errorf("GetArgA() = %v, want 10", GetArgA(instr))
	}
	if GetArgB(instr) != 20 {
		t.Errorf("GetArgB() = %v, want 20", GetArgB(instr))
	}
	if GetArgC(instr) != 30 {
		t.Errorf("GetArgC() = %v, want 30", GetArgC(instr))
	}
}

func TestCreateABCk(t *testing.T) {
	// Test ABC with k bit
	instr := CreateABCk(api.OP_ADDK, 5, 1, 10, 20)
	
	if GetOpCode(instr) != api.OP_ADDK {
		t.Errorf("GetOpCode() = %v, want OP_ADDK", GetOpCode(instr))
	}
	if GetArgA(instr) != 5 {
		t.Errorf("GetArgA() = %v, want 5", GetArgA(instr))
	}
	if GetArgK(instr) != 1 {
		t.Errorf("GetArgK() = %v, want 1", GetArgK(instr))
	}
	if GetArgB(instr) != 10 {
		t.Errorf("GetArgB() = %v, want 10", GetArgB(instr))
	}
	if GetArgC(instr) != 20 {
		t.Errorf("GetArgC() = %v, want 20", GetArgC(instr))
	}
}

func TestCreatevABCk(t *testing.T) {
	// Test variant ABC (vB=6 bits, vC=10 bits)
	instr := CreatevABCk(api.OP_NEWTABLE, 1, 0, 30, 500)
	
	if GetOpCode(instr) != api.OP_NEWTABLE {
		t.Errorf("GetOpCode() = %v, want OP_NEWTABLE", GetOpCode(instr))
	}
	if GetArgA(instr) != 1 {
		t.Errorf("GetArgA() = %v, want 1", GetArgA(instr))
	}
	if GetArgK(instr) != 0 {
		t.Errorf("GetArgK() = %v, want 0", GetArgK(instr))
	}
	if GetArgvB(instr) != 30 {
		t.Errorf("GetArgvB() = %v, want 30", GetArgvB(instr))
	}
	if GetArgvC(instr) != 500 {
		t.Errorf("GetArgvC() = %v, want 500", GetArgvC(instr))
	}
}

func TestCreateABx(t *testing.T) {
	// Test ABx instruction
	instr := CreateABx(api.OP_LOADK, 10, 1000)
	
	if GetOpCode(instr) != api.OP_LOADK {
		t.Errorf("GetOpCode() = %v, want OP_LOADK", GetOpCode(instr))
	}
	if GetArgA(instr) != 10 {
		t.Errorf("GetArgA() = %v, want 10", GetArgA(instr))
	}
	if GetArgBx(instr) != 1000 {
		t.Errorf("GetArgBx() = %v, want 1000", GetArgBx(instr))
	}
}

func TestCreateAsBx(t *testing.T) {
	// Test AsBx with signed operand
	instr := CreateAsBx(api.OP_LOADI, 10, -100)
	
	if GetOpCode(instr) != api.OP_LOADI {
		t.Errorf("GetOpCode() = %v, want OP_LOADI", GetOpCode(instr))
	}
	if GetArgsBx(instr) != -100 {
		t.Errorf("GetArgsBx() = %v, want -100", GetArgsBx(instr))
	}
	
	// Test positive value
	instr2 := CreateAsBx(api.OP_LOADI, 5, 200)
	if GetArgsBx(instr2) != 200 {
		t.Errorf("GetArgsBx() = %v, want 200", GetArgsBx(instr2))
	}
}

func TestCreateAx(t *testing.T) {
	// Test Ax instruction
	instr := CreateAx(api.OP_EXTRAARG, 1000000)
	
	if GetOpCode(instr) != api.OP_EXTRAARG {
		t.Errorf("GetOpCode() = %v, want OP_EXTRAARG", GetOpCode(instr))
	}
	if GetArgAx(instr) != 1000000 {
		t.Errorf("GetArgAx() = %v, want 1000000", GetArgAx(instr))
	}
}

func TestCreateSJ(t *testing.T) {
	// Test SJ instruction
	instr := CreateSJ(api.OP_JMP, -50)
	
	if GetOpCode(instr) != api.OP_JMP {
		t.Errorf("GetOpCode() = %v, want OP_JMP", GetOpCode(instr))
	}
	if GetArgsJ(instr) != -50 {
		t.Errorf("GetArgsJ() = %v, want -50", GetArgsJ(instr))
	}
	
	// Test positive jump
	instr2 := CreateSJ(api.OP_JMP, 100)
	if GetArgsJ(instr2) != 100 {
		t.Errorf("GetArgsJ() = %v, want 100", GetArgsJ(instr2))
	}
}

func TestLuaP_IsOT(t *testing.T) {
	// OP_TAILCALL always sets OT
	tailcallInstr := CreateABC(api.OP_TAILCALL, 0, 0, 0)
	if !luaP_isOT(tailcallInstr) {
		t.Error("luaP_isOT(OP_TAILCALL) should be true")
	}
	
	// Regular call with C=0 should set OT
	callInstr := CreateABC(api.OP_CALL, 0, 1, 0)
	if !luaP_isOT(callInstr) {
		t.Error("luaP_isOT(OP_CALL with C=0) should be true")
	}
	
	// Call with C>0 should not set OT
	callInstr2 := CreateABC(api.OP_CALL, 0, 1, 2)
	if luaP_isOT(callInstr2) {
		t.Error("luaP_isOT(OP_CALL with C>0) should be false")
	}
}

func TestLuaP_IsIT(t *testing.T) {
	// OP_RETURN with B=0 should set IT
	returnInstr := CreateABC(api.OP_RETURN, 0, 0, 0)
	if !luaP_isIT(returnInstr) {
		t.Error("luaP_isIT(OP_RETURN with B=0) should be true")
	}
	
	// OP_SETLIST with vB=0 should set IT
	setlistInstr := CreatevABCk(api.OP_SETLIST, 0, 0, 0, 1)
	if !luaP_isIT(setlistInstr) {
		t.Error("luaP_isIT(OP_SETLIST with vB=0) should be true")
	}
	
	// Regular instruction with B=0
	moveInstr := CreateABC(api.OP_MOVE, 0, 0, 0)
	if luaP_isIT(moveInstr) {
		// OP_MOVE doesn't have IT mode set
		t.Error("luaP_isIT(OP_MOVE) should be false")
	}
}

func TestGETALIASFunctions(t *testing.T) {
	// Test that GET_* alias functions work correctly
	instr := CreateABCk(api.OP_ADDK, 5, 1, 10, 20)
	
	if GET_OPCODE(instr) != api.OP_ADDK {
		t.Errorf("GET_OPCODE() = %v, want OP_ADDK", GET_OPCODE(instr))
	}
	if GETARG_A(instr) != 5 {
		t.Errorf("GETARG_A() = %v, want 5", GETARG_A(instr))
	}
	if GETARG_B(instr) != 10 {
		t.Errorf("GETARG_B() = %v, want 10", GETARG_B(instr))
	}
	if GETARG_C(instr) != 20 {
		t.Errorf("GETARG_C() = %v, want 20", GETARG_C(instr))
	}
	if GETARG_K(instr) != 1 {
		t.Errorf("GETARG_K() = %v, want 1", GETARG_K(instr))
	}
}
