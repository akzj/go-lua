package lcode

import (
	"testing"

	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lopcodes"
	"github.com/akzj/go-lua/internal/lparser"
)

func TestNO_JUMP(t *testing.T) {
	if NO_JUMP != -1 {
		t.Errorf("NO_JUMP should be -1, got %d", NO_JUMP)
	}
}

func TestBinOprConstants(t *testing.T) {
	// Test binary operator constants
	tests := []struct {
		name string
		got  BinOpr
		exp  BinOpr
	}{
		{"OPR_ADD", OPR_ADD, 0},
		{"OPR_SUB", OPR_SUB, 1},
		{"OPR_MUL", OPR_MUL, 2},
		{"OPR_MOD", OPR_MOD, 3},
		{"OPR_POW", OPR_POW, 4},
		{"OPR_DIV", OPR_DIV, 5},
		{"OPR_IDIV", OPR_IDIV, 6},
		{"OPR_BAND", OPR_BAND, 7},
		{"OPR_BOR", OPR_BOR, 8},
		{"OPR_BXOR", OPR_BXOR, 9},
		{"OPR_SHL", OPR_SHL, 10},
		{"OPR_SHR", OPR_SHR, 11},
		{"OPR_CONCAT", OPR_CONCAT, 12},
		{"OPR_EQ", OPR_EQ, 13},
		{"OPR_LT", OPR_LT, 14},
		{"OPR_LE", OPR_LE, 15},
		{"OPR_NE", OPR_NE, 16},
		{"OPR_GT", OPR_GT, 17},
		{"OPR_GE", OPR_GE, 18},
		{"OPR_AND", OPR_AND, 19},
		{"OPR_OR", OPR_OR, 20},
		{"OPR_NOBINOPR", OPR_NOBINOPR, 21},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.exp {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.exp)
			}
		})
	}
}

func TestUnOprConstants(t *testing.T) {
	tests := []struct {
		name string
		got  UnOpr
		exp  UnOpr
	}{
		{"OPR_MINUS", OPR_MINUS, 0},
		{"OPR_BNOT", OPR_BNOT, 1},
		{"OPR_NOT", OPR_NOT, 2},
		{"OPR_LEN", OPR_LEN, 3},
		{"OPR_NOUNOPR", OPR_NOUNOPR, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.exp {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.exp)
			}
		})
	}
}

func TestCodeInstruction(t *testing.T) {
	// Create a minimal FuncState for testing
	fs := &lparser.FuncState{}
	fs.F = &lobject.Proto{}
	fs.F.Code = make([]uint32, 0, 10)
	fs.Pc = 0

	// Test Code function
	idx := Code(fs, lopcodes.CREATE_ABCk(lopcodes.OP_LOADNIL, 0, 1, 0, 0))
	if idx != 0 {
		t.Errorf("Code returned %d, want 0", idx)
	}
	if fs.Pc != 1 {
		t.Errorf("fs.Pc = %d, want 1", fs.Pc)
	}
	if len(fs.F.Code) != 1 {
		t.Errorf("len(fs.F.Code) = %d, want 1", len(fs.F.Code))
	}
}

func TestCodeABC(t *testing.T) {
	fs := &lparser.FuncState{}
	fs.F = &lobject.Proto{}
	fs.F.Code = make([]uint32, 0, 10)
	fs.Pc = 0

	// Test CodeABC function
	idx := CodeABC(fs, lopcodes.OP_ADD, 0, 1, 2)
	if idx != 0 {
		t.Errorf("CodeABC returned %d, want 0", idx)
	}

	// Verify instruction encoding
	inst := fs.F.Code[0]
	if lopcodes.GETARG_A(inst) != 0 {
		t.Errorf("GETARG_A = %d, want 0", lopcodes.GETARG_A(inst))
	}
	if lopcodes.GETARG_B(inst) != 1 {
		t.Errorf("GETARG_B = %d, want 1", lopcodes.GETARG_B(inst))
	}
	if lopcodes.GETARG_C(inst) != 2 {
		t.Errorf("GETARG_C = %d, want 2", lopcodes.GETARG_C(inst))
	}
}

func TestCodeABx(t *testing.T) {
	fs := &lparser.FuncState{}
	fs.F = &lobject.Proto{}
	fs.F.Code = make([]uint32, 0, 10)
	fs.Pc = 0

	// Test CodeABx function
	idx := CodeABx(fs, lopcodes.OP_LOADK, 0, 100)
	if idx != 0 {
		t.Errorf("CodeABx returned %d, want 0", idx)
	}

	// Verify instruction encoding
	inst := fs.F.Code[0]
	if lopcodes.GETARG_A(inst) != 0 {
		t.Errorf("GETARG_A = %d, want 0", lopcodes.GETARG_A(inst))
	}
	if lopcodes.GETARG_Bx(inst) != 100 {
		t.Errorf("GETARG_Bx = %d, want 100", lopcodes.GETARG_Bx(inst))
	}
}

func TestJump(t *testing.T) {
	fs := &lparser.FuncState{}
	fs.F = &lobject.Proto{}
	fs.F.Code = make([]uint32, 10) // Pre-allocate to avoid index issues
	fs.Pc = 0

	// Test Jump function
	target := Jump(fs)
	if target != 0 {
		t.Errorf("Jump returned %d, want 0", target)
	}

	// Verify JMP instruction was generated
	inst := fs.F.Code[0]
	if lopcodes.GetOpCode(inst) != lopcodes.OP_JMP {
		t.Errorf("Expected OP_JMP, got %v", lopcodes.GetOpCode(inst))
	}
}

func TestRet(t *testing.T) {
	fs := &lparser.FuncState{}
	fs.F = &lobject.Proto{}
	fs.F.Code = make([]uint32, 0, 10)
	fs.Pc = 0

	// Test return 0
	Ret(fs, 0, 0)
	if lopcodes.GetOpCode(fs.F.Code[0]) != lopcodes.OP_RETURN0 {
		t.Errorf("Expected OP_RETURN0")
	}

	// Test return 1
	Ret(fs, 1, 1)
	if lopcodes.GetOpCode(fs.F.Code[1]) != lopcodes.OP_RETURN1 {
		t.Errorf("Expected OP_RETURN1")
	}

	// Test return multiple
	Ret(fs, 0, 3)
	if lopcodes.GetOpCode(fs.F.Code[2]) != lopcodes.OP_RETURN {
		t.Errorf("Expected OP_RETURN")
	}
}

func TestNil(t *testing.T) {
	fs := &lparser.FuncState{}
	fs.F = &lobject.Proto{}
	fs.F.Code = make([]uint32, 0, 10)
	fs.Pc = 0

	// Test Nil function
	Nil(fs, 0, 3)
	if lopcodes.GetOpCode(fs.F.Code[0]) != lopcodes.OP_LOADNIL {
		t.Errorf("Expected OP_LOADNIL")
	}
	if lopcodes.GETARG_A(fs.F.Code[0]) != 0 {
		t.Errorf("GETARG_A = %d, want 0", lopcodes.GETARG_A(fs.F.Code[0]))
	}
	if lopcodes.GETARG_B(fs.F.Code[0]) != 2 { // n-1 = 3-1 = 2
		t.Errorf("GETARG_B = %d, want 2", lopcodes.GETARG_B(fs.F.Code[0]))
	}
}

func TestCheckStack(t *testing.T) {
	fs := &lparser.FuncState{}
	fs.F = &lobject.Proto{}
	fs.F.Maxstacksize = 10
	fs.FreeReg = 5

	// Test CheckStack - should not grow
	CheckStack(fs, 2)
	if fs.F.Maxstacksize != 10 {
		t.Errorf("Maxstacksize = %d, want 10 (no grow needed)", fs.F.Maxstacksize)
	}

	// Test CheckStack - should grow
	CheckStack(fs, 8)
	if fs.F.Maxstacksize != 13 { // 5 + 8 = 13
		t.Errorf("Maxstacksize = %d, want 13", fs.F.Maxstacksize)
	}
}

func TestReserveRegs(t *testing.T) {
	fs := &lparser.FuncState{}
	fs.F = &lobject.Proto{}
	fs.F.Maxstacksize = 10
	fs.FreeReg = 5

	ReserveRegs(fs, 3)
	if fs.FreeReg != 8 {
		t.Errorf("FreeReg = %d, want 8", fs.FreeReg)
	}
}

func TestPatchToHere(t *testing.T) {
	fs := &lparser.FuncState{}
	fs.F = &lobject.Proto{}
	fs.F.Code = make([]uint32, 0, 20)
	fs.Pc = 0

	// Create a jump target
	jumpTarget := Jump(fs) // pc = 0
	fs.Pc = 5              // Advance past the jump

	// Patch the jump to here
	PatchToHere(fs, jumpTarget)

	// Verify the jump was patched
	offset := lopcodes.GETARG_sJ(fs.F.Code[jumpTarget])
	expectedOffset := 5 - (jumpTarget + 1) // target - (pc + 1)
	if offset != expectedOffset {
		t.Errorf("Jump offset = %d, want %d", offset, expectedOffset)
	}
}

func TestConcat(t *testing.T) {
	fs := &lparser.FuncState{}
	fs.F = &lobject.Proto{}
	fs.F.Code = make([]uint32, 10) // Pre-allocate to avoid index issues
	fs.Pc = 0

	// Create two jumps
	jump1 := Jump(fs) // pc = 0
	fs.Pc = 2
	jump2 := Jump(fs) // pc = 2
	fs.Pc = 4

	// Test Concat with first jump
	list := lparser.NO_JUMP
	Concat(fs, &list, jump2)
	if list != jump2 {
		t.Errorf("Concat with NO_JUMP: list = %d, want %d", list, jump2)
	}

	// Concat second jump - after this, list (jump2) should point to jump1
	Concat(fs, &list, jump1)
	// list is still jump2, but jump2's target should now be jump1 (via fixJump)
	// Verify the chain: jump2 -> jump1 -> NO_JUMP
	next1 := getJump(fs, jump2) // should be jump1
	next2 := getJump(fs, jump1)  // should be NO_JUMP
	if next1 != jump1 {
		t.Errorf("Chain broken: jump2 -> %d, want %d", next1, jump1)
	}
	if next2 != lparser.NO_JUMP {
		t.Errorf("Chain broken: jump1 -> %d, want NO_JUMP", next2)
	}
}

func TestMAXARG_sJ(t *testing.T) {
	// SIZE_sJ = SIZE_Bx + SIZE_A = 17 + 8 = 25
	if MAXARG_sJ != (1<<25)-1 {
		t.Errorf("MAXARG_sJ = %d, want %d", MAXARG_sJ, (1<<25)-1)
	}
}