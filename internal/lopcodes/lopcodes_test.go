package lopcodes

/*
** $Id: lopcodes_test.go $
** Unit tests for lopcodes
*/

import (
	"testing"

	"github.com/akzj/go-lua/internal/lobject"
)

func TestOpCodeConstants(t *testing.T) {
	// Verify OpCode enum has correct number of opcodes
	if NUM_OPCODES != 85 {
		t.Errorf("NUM_OPCODES expected 85, got %d", NUM_OPCODES)
	}

	// Verify first and last opcodes
	if OP_MOVE != 0 {
		t.Errorf("OP_MOVE expected 0, got %d", OP_MOVE)
	}
	if OP_EXTRAARG != 84 {
		t.Errorf("OP_EXTRAARG expected 84, got %d", OP_EXTRAARG)
	}

	// Verify specific opcodes exist and are in order
	if OP_GETTABLE != 12 {
		t.Errorf("OP_GETTABLE expected 12, got %d", OP_GETTABLE)
	}
	if OP_SETTABLE != 16 {
		t.Errorf("OP_SETTABLE expected 16, got %d", OP_SETTABLE)
	}
	if OP_ADD != 34 {
		t.Errorf("OP_ADD expected 34, got %d", OP_ADD)
	}
	if OP_MOVE >= OP_LOADI || OP_LOADI >= OP_LOADF {
		t.Error("First few opcodes should be in order")
	}
}

func TestNUMOPCODES(t *testing.T) {
	if NUM_OPCODES != 85 {
		t.Errorf("NUM_OPCODES expected 85, got %d", NUM_OPCODES)
	}
}

func TestGETARG_A(t *testing.T) {
	inst := CREATE_ABCk(OP_MOVE, 10, 5, 0, 0)
	if GETARG_A(inst) != 10 {
		t.Errorf("GETARG_A expected 10, got %d", GETARG_A(inst))
	}
}

func TestGETARG_B(t *testing.T) {
	inst := CREATE_ABCk(OP_MOVE, 3, 20, 0, 0)
	if GETARG_B(inst) != 20 {
		t.Errorf("GETARG_B expected 20, got %d", GETARG_B(inst))
	}
}

func TestGETARG_C(t *testing.T) {
	inst := CREATE_ABCk(OP_ADD, 1, 2, 3, 0)
	if GETARG_C(inst) != 3 {
		t.Errorf("GETARG_C expected 3, got %d", GETARG_C(inst))
	}
}

func TestSETARG_A(t *testing.T) {
	inst := lobject.LUInt32(0)
	SETARG_A(&inst, 42)
	if GETARG_A(inst) != 42 {
		t.Errorf("SETARG_A/GETARG_A expected 42, got %d", GETARG_A(inst))
	}
}

func TestGETARG_k(t *testing.T) {
	inst := CREATE_ABCk(OP_EQ, 0, 1, 1, 1)
	if GETARG_k(inst) != 1 {
		t.Errorf("GETARG_k expected 1, got %d", GETARG_k(inst))
	}
}

func TestGETARG_Bx(t *testing.T) {
	inst := CREATE_ABx(OP_LOADK, 5, 100)
	if GETARG_Bx(inst) != 100 {
		t.Errorf("GETARG_Bx expected 100, got %d", GETARG_Bx(inst))
	}
}

func TestGETARG_Ax(t *testing.T) {
	inst := CREATE_Ax(OP_EXTRAARG, 12345)
	if GETARG_Ax(inst) != 12345 {
		t.Errorf("GETARG_Ax expected 12345, got %d", GETARG_Ax(inst))
	}
}

func TestGETARG_sBx(t *testing.T) {
	inst := lobject.LUInt32(0)
	SETARG_sBx(&inst, -50)
	if GETARG_sBx(inst) != -50 {
		t.Errorf("GETARG_sBx expected -50, got %d", GETARG_sBx(inst))
	}
}

func TestGETARG_sJ(t *testing.T) {
	inst := lobject.LUInt32(0)
	SETARG_sJ(&inst, -25)
	if GETARG_sJ(inst) != -25 {
		t.Errorf("GETARG_sJ expected -25, got %d", GETARG_sJ(inst))
	}
}

func TestGetOpCode(t *testing.T) {
	inst := CREATE_ABCk(OP_ADD, 1, 2, 3, 0)
	if GetOpCode(inst) != OP_ADD {
		t.Errorf("GetOpCode expected OP_ADD")
	}
}

func TestSetOpCode(t *testing.T) {
	inst := lobject.LUInt32(0)
	SETARG_A(&inst, 5)
	SetOpCode(&inst, OP_SUB)
	if GetOpCode(inst) != OP_SUB {
		t.Errorf("SetOpCode failed")
	}
}

func TestCREATE_ABx(t *testing.T) {
	inst := CREATE_ABx(OP_LOADK, 3, 500)
	if GetOpCode(inst) != OP_LOADK {
		t.Error("CREATE_ABx: wrong opcode")
	}
	if GETARG_A(inst) != 3 {
		t.Errorf("CREATE_ABx: wrong A, got %d", GETARG_A(inst))
	}
}

func TestCREATE_ABCk(t *testing.T) {
	inst := CREATE_ABCk(OP_CALL, 1, 2, 3, 1)
	if GETARG_A(inst) != 1 || GETARG_B(inst) != 2 || GETARG_C(inst) != 3 || GETARG_k(inst) != 1 {
		t.Error("CREATE_ABCk: wrong args")
	}
}

func TestCREATE_sJ(t *testing.T) {
	inst := CREATE_sJ(OP_JMP, -10, 1)
	if GetOpCode(inst) != OP_JMP {
		t.Error("CREATE_sJ: wrong opcode")
	}
	if GETARG_sJ(inst) != -10 {
		t.Errorf("CREATE_sJ: wrong sJ, got %d", GETARG_sJ(inst))
	}
}

func TestOpMode(t *testing.T) {
	if getOpMode(OP_LOADK) != IABx {
		t.Errorf("OP_LOADK should be IABx mode")
	}
	if getOpMode(OP_MOVE) != IABC {
		t.Errorf("OP_MOVE should be IABC mode")
	}
	if getOpMode(OP_EXTRAARG) != IAx {
		t.Errorf("OP_EXTRAARG should be IAx mode")
	}
	if getOpMode(OP_JMP) != IsJ {
		t.Errorf("OP_JMP should be IsJ mode")
	}
}

func TestTestTMode(t *testing.T) {
	if !testTMode(OP_EQ) || !testTMode(OP_TEST) {
		t.Error("Test opcodes should be T mode")
	}
	if testTMode(OP_ADD) {
		t.Error("ADD should not be T mode")
	}
}

func TestMaxStack(t *testing.T) {
	if MAX_STACK != 255 {
		t.Errorf("MAX_STACK expected 255, got %d", MAX_STACK)
	}
}

func TestMaskOperations(t *testing.T) {
	m1 := mask1(8, 0)
	if m1 != 0xFF {
		t.Errorf("mask1(8, 0) expected 0xFF, got %x", m1)
	}
}

func TestOpNames(t *testing.T) {
	if len(opNames) != int(NUM_OPCODES) {
		t.Errorf("opNames length mismatch")
	}
	if opNames[OP_MOVE] != "OP_MOVE" {
		t.Errorf("opNames[OP_MOVE] wrong")
	}
}

func TestLuaPOpModes(t *testing.T) {
	if len(luaP_opmodes) != int(NUM_OPCODES) {
		t.Errorf("luaP_opmodes length mismatch")
	}
}

func TestInstructionBitLayout(t *testing.T) {
	inst := CREATE_ABCk(OP_MOVE, 10, 20, 30, 0)
	if GetOpCode(inst) != OP_MOVE {
		t.Error("opcode mismatch")
	}
	if GETARG_A(inst) != 10 || GETARG_B(inst) != 20 || GETARG_C(inst) != 30 {
		t.Error("args mismatch")
	}
}

func TestCreateAx(t *testing.T) {
	inst := CREATE_Ax(OP_EXTRAARG, 999)
	if GETARG_Ax(inst) != 999 {
		t.Error("CREATE_Ax failed")
	}
}
