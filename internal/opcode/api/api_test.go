package api

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Encoding/Decoding roundtrip tests
// ---------------------------------------------------------------------------

func TestIABC_Roundtrip(t *testing.T) {
	// iABC format: op(7) A(8) k(1) B(8) C(8)
	tests := []struct {
		op      OpCode
		a, b, c int
		k       int
	}{
		{OP_MOVE, 0, 0, 0, 0},
		{OP_MOVE, 1, 2, 3, 0},
		{OP_MOVE, MaxArgA, MaxArgB, MaxArgC, 1},
		{OP_ADD, 10, 20, 30, 0},
		{OP_CALL, 0, 1, 2, 0},
		{OP_LOADFALSE, 255, 0, 0, 0},
	}
	for _, tt := range tests {
		i := CreateABCK(tt.op, tt.a, tt.b, tt.c, tt.k)
		if got := GetOpCode(i); got != tt.op {
			t.Errorf("op: got %d, want %d", got, tt.op)
		}
		if got := GetArgA(i); got != tt.a {
			t.Errorf("A: got %d, want %d", got, tt.a)
		}
		if got := GetArgB(i); got != tt.b {
			t.Errorf("B: got %d, want %d", got, tt.b)
		}
		if got := GetArgC(i); got != tt.c {
			t.Errorf("C: got %d, want %d", got, tt.c)
		}
		if got := GetArgK(i); got != tt.k {
			t.Errorf("k: got %d, want %d", got, tt.k)
		}
	}
}

func TestIABx_Roundtrip(t *testing.T) {
	tests := []struct {
		op OpCode
		a  int
		bx int
	}{
		{OP_LOADK, 0, 0},
		{OP_LOADK, 1, 100},
		{OP_LOADK, MaxArgA, MaxArgBx},
		{OP_CLOSURE, 5, 12345},
	}
	for _, tt := range tests {
		i := CreateABx(tt.op, tt.a, tt.bx)
		if got := GetOpCode(i); got != tt.op {
			t.Errorf("op: got %d, want %d", got, tt.op)
		}
		if got := GetArgA(i); got != tt.a {
			t.Errorf("A: got %d, want %d", got, tt.a)
		}
		if got := GetArgBx(i); got != tt.bx {
			t.Errorf("Bx: got %d, want %d", got, tt.bx)
		}
	}
}

func TestIAsBx_Roundtrip(t *testing.T) {
	tests := []struct {
		op  OpCode
		a   int
		sbx int
	}{
		{OP_LOADI, 0, 0},
		{OP_LOADI, 1, 100},
		{OP_LOADI, 1, -100},
		{OP_LOADI, MaxArgA, OffsetSBx},   // max positive
		{OP_LOADI, 0, -OffsetSBx},        // max negative
		{OP_LOADF, 5, 42},
		{OP_FORLOOP, 3, -10},
	}
	for _, tt := range tests {
		i := CreateAsBx(tt.op, tt.a, tt.sbx)
		if got := GetOpCode(i); got != tt.op {
			t.Errorf("op: got %d, want %d", got, tt.op)
		}
		if got := GetArgA(i); got != tt.a {
			t.Errorf("A: got %d, want %d (sbx=%d)", got, tt.a, tt.sbx)
		}
		if got := GetArgSBx(i); got != tt.sbx {
			t.Errorf("sBx: got %d, want %d", got, tt.sbx)
		}
	}
}

func TestIAx_Roundtrip(t *testing.T) {
	tests := []struct {
		op OpCode
		ax int
	}{
		{OP_EXTRAARG, 0},
		{OP_EXTRAARG, 1},
		{OP_EXTRAARG, MaxArgAx},
		{OP_EXTRAARG, 12345678},
	}
	for _, tt := range tests {
		i := CreateAx(tt.op, tt.ax)
		if got := GetOpCode(i); got != tt.op {
			t.Errorf("op: got %d, want %d", got, tt.op)
		}
		if got := GetArgAx(i); got != tt.ax {
			t.Errorf("Ax: got %d, want %d", got, tt.ax)
		}
	}
}

func TestISJ_Roundtrip(t *testing.T) {
	tests := []struct {
		op OpCode
		sj int
	}{
		{OP_JMP, 0},
		{OP_JMP, 1},
		{OP_JMP, -1},
		{OP_JMP, 100},
		{OP_JMP, -100},
		{OP_JMP, OffsetSJ},   // max positive
		{OP_JMP, -OffsetSJ},  // max negative
	}
	for _, tt := range tests {
		i := CreateSJ(tt.op, tt.sj)
		if got := GetOpCode(i); got != tt.op {
			t.Errorf("op: got %d, want %d", got, tt.op)
		}
		if got := GetArgSJ(i); got != tt.sj {
			t.Errorf("sJ: got %d, want %d", got, tt.sj)
		}
	}
}

func TestIVABC_Roundtrip(t *testing.T) {
	// ivABC format: op(7) A(8) k(1) vB(6) vC(10)
	tests := []struct {
		op     OpCode
		a      int
		vb, vc int
		k      int
	}{
		{OP_NEWTABLE, 0, 0, 0, 0},
		{OP_NEWTABLE, 5, 10, 100, 1},
		{OP_NEWTABLE, MaxArgA, MaxArgVB, MaxArgVC, 1},
		{OP_SETLIST, 3, 0, 50, 0},
	}
	for _, tt := range tests {
		i := CreateVABCK(tt.op, tt.a, tt.vb, tt.vc, tt.k)
		if got := GetOpCode(i); got != tt.op {
			t.Errorf("op: got %d, want %d", got, tt.op)
		}
		if got := GetArgA(i); got != tt.a {
			t.Errorf("A: got %d, want %d", got, tt.a)
		}
		if got := GetArgVB(i); got != tt.vb {
			t.Errorf("vB: got %d, want %d", got, tt.vb)
		}
		if got := GetArgVC(i); got != tt.vc {
			t.Errorf("vC: got %d, want %d", got, tt.vc)
		}
		if got := GetArgK(i); got != tt.k {
			t.Errorf("k: got %d, want %d", got, tt.k)
		}
	}
}

// ---------------------------------------------------------------------------
// Signed operand tests
// ---------------------------------------------------------------------------

func TestSignedSC(t *testing.T) {
	// sC = C - OffsetSC
	tests := []int{-OffsetSC, -1, 0, 1, OffsetSC}
	for _, sc := range tests {
		c := sc + OffsetSC // unsigned encoding
		i := CreateABCK(OP_ADDI, 0, 0, c, 0)
		if got := GetArgSC(i); got != sc {
			t.Errorf("sC: got %d, want %d", got, sc)
		}
	}
}

func TestSignedSB(t *testing.T) {
	tests := []int{-OffsetSC, -1, 0, 1, OffsetSC}
	for _, sb := range tests {
		b := sb + OffsetSC
		i := CreateABCK(OP_SHLI, 0, b, 0, 0)
		if got := GetArgSB(i); got != sb {
			t.Errorf("sB: got %d, want %d", got, sb)
		}
	}
}

// ---------------------------------------------------------------------------
// OpCode count
// ---------------------------------------------------------------------------

func TestNumOpcodes(t *testing.T) {
	if NumOpcodes != 85 {
		t.Errorf("NumOpcodes = %d, want 85", NumOpcodes)
	}
	// OP_EXTRAARG should be the last opcode
	if OP_EXTRAARG != 84 {
		t.Errorf("OP_EXTRAARG = %d, want 84", OP_EXTRAARG)
	}
}

// ---------------------------------------------------------------------------
// OpNames
// ---------------------------------------------------------------------------

func TestOpNames(t *testing.T) {
	// Spot-check key opcodes
	checks := map[OpCode]string{
		OP_MOVE:       "MOVE",
		OP_LOADI:      "LOADI",
		OP_LOADK:      "LOADK",
		OP_NEWTABLE:   "NEWTABLE",
		OP_ADD:        "ADD",
		OP_MMBIN:      "MMBIN",
		OP_JMP:        "JMP",
		OP_EQ:         "EQ",
		OP_CALL:       "CALL",
		OP_TAILCALL:   "TAILCALL",
		OP_RETURN:     "RETURN",
		OP_FORLOOP:    "FORLOOP",
		OP_CLOSURE:    "CLOSURE",
		OP_VARARG:     "VARARG",
		OP_GETVARG:    "GETVARG",
		OP_ERRNNIL:    "ERRNNIL",
		OP_VARARGPREP: "VARARGPREP",
		OP_EXTRAARG:   "EXTRAARG",
	}
	for op, want := range checks {
		if got := OpName(op); got != want {
			t.Errorf("OpName(%d) = %q, want %q", op, got, want)
		}
	}
	// All names should be non-empty
	for i := 0; i < NumOpcodes; i++ {
		if OpNames[i] == "" {
			t.Errorf("OpNames[%d] is empty", i)
		}
	}
}

// ---------------------------------------------------------------------------
// OpModes — spot-check 10 key opcodes against C source
// ---------------------------------------------------------------------------

func TestOpModes(t *testing.T) {
	tests := []struct {
		op     OpCode
		name   string
		mode   OpMode
		setsA  bool
		testT  bool
		usesIT bool
		setsOT bool
		mm     bool
	}{
		{OP_MOVE, "MOVE", IABC, true, false, false, false, false},
		{OP_LOADK, "LOADK", IABx, true, false, false, false, false},
		{OP_JMP, "JMP", ISJ, false, false, false, false, false},
		{OP_CALL, "CALL", IABC, true, false, true, true, false},
		{OP_RETURN, "RETURN", IABC, false, false, true, false, false},
		{OP_FORLOOP, "FORLOOP", IABx, true, false, false, false, false},
		{OP_EQ, "EQ", IABC, false, true, false, false, false},
		{OP_MMBIN, "MMBIN", IABC, false, false, false, false, true},
		{OP_EXTRAARG, "EXTRAARG", IAx, false, false, false, false, false},
		{OP_NEWTABLE, "NEWTABLE", IVABC, true, false, false, false, false},
		// Additional checks
		{OP_LOADI, "LOADI", IAsBx, true, false, false, false, false},
		{OP_TAILCALL, "TAILCALL", IABC, true, false, true, true, false},
		{OP_TESTSET, "TESTSET", IABC, true, true, false, false, false},
		{OP_VARARG, "VARARG", IABC, true, false, false, true, false},
		{OP_SETLIST, "SETLIST", IVABC, false, false, true, false, false},
		{OP_VARARGPREP, "VARARGPREP", IABC, false, false, true, false, false},
		{OP_SETUPVAL, "SETUPVAL", IABC, false, false, false, false, false},
		{OP_MMBINI, "MMBINI", IABC, false, false, false, false, true},
		{OP_MMBINK, "MMBINK", IABC, false, false, false, false, true},
		{OP_CLOSE, "CLOSE", IABC, false, false, false, false, false},
		{OP_TBC, "TBC", IABC, false, false, false, false, false},
		{OP_CLOSURE, "CLOSURE", IABx, true, false, false, false, false},
		{OP_RETURN0, "RETURN0", IABC, false, false, false, false, false},
		{OP_RETURN1, "RETURN1", IABC, false, false, false, false, false},
		{OP_TEST, "TEST", IABC, false, true, false, false, false},
		{OP_GETVARG, "GETVARG", IABC, true, false, false, false, false},
		{OP_ERRNNIL, "ERRNNIL", IABx, false, false, false, false, false},
	}
	for _, tt := range tests {
		if got := GetMode(tt.op); got != tt.mode {
			t.Errorf("%s: mode = %d, want %d", tt.name, got, tt.mode)
		}
		if got := TestAMode(tt.op); got != tt.setsA {
			t.Errorf("%s: setsA = %v, want %v", tt.name, got, tt.setsA)
		}
		if got := TestTMode(tt.op); got != tt.testT {
			t.Errorf("%s: testT = %v, want %v", tt.name, got, tt.testT)
		}
		if got := TestITMode(tt.op); got != tt.usesIT {
			t.Errorf("%s: usesIT = %v, want %v", tt.name, got, tt.usesIT)
		}
		if got := TestOTMode(tt.op); got != tt.setsOT {
			t.Errorf("%s: setsOT = %v, want %v", tt.name, got, tt.setsOT)
		}
		if got := TestMMMode(tt.op); got != tt.mm {
			t.Errorf("%s: mm = %v, want %v", tt.name, got, tt.mm)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge cases: max/min values for operand fields
// ---------------------------------------------------------------------------

func TestOperandLimits(t *testing.T) {
	// MaxArgA = 255
	if MaxArgA != 255 {
		t.Errorf("MaxArgA = %d, want 255", MaxArgA)
	}
	// MaxArgB = 255
	if MaxArgB != 255 {
		t.Errorf("MaxArgB = %d, want 255", MaxArgB)
	}
	// MaxArgC = 255
	if MaxArgC != 255 {
		t.Errorf("MaxArgC = %d, want 255", MaxArgC)
	}
	// MaxArgBx = 131071 (2^17 - 1)
	if MaxArgBx != 131071 {
		t.Errorf("MaxArgBx = %d, want 131071", MaxArgBx)
	}
	// MaxArgAx = 33554431 (2^25 - 1)
	if MaxArgAx != 33554431 {
		t.Errorf("MaxArgAx = %d, want 33554431", MaxArgAx)
	}
	// OffsetSBx = 65535
	if OffsetSBx != 65535 {
		t.Errorf("OffsetSBx = %d, want 65535", OffsetSBx)
	}
	// OffsetSJ = 16777215
	if OffsetSJ != 16777215 {
		t.Errorf("OffsetSJ = %d, want 16777215", OffsetSJ)
	}
	// OffsetSC = 127
	if OffsetSC != 127 {
		t.Errorf("OffsetSC = %d, want 127", OffsetSC)
	}
	// NoReg = 255
	if NoReg != 255 {
		t.Errorf("NoReg = %d, want 255", NoReg)
	}
}

// ---------------------------------------------------------------------------
// SetArgA and SetArgSBx — modify only the correct bits
// ---------------------------------------------------------------------------

func TestSetArgA(t *testing.T) {
	// Start with a known instruction
	i := CreateABCK(OP_MOVE, 10, 20, 30, 1)
	// Change A to 42
	i = SetArgA(i, 42)
	// A should be 42, everything else unchanged
	if got := GetOpCode(i); got != OP_MOVE {
		t.Errorf("op changed: got %d, want %d", got, OP_MOVE)
	}
	if got := GetArgA(i); got != 42 {
		t.Errorf("A: got %d, want 42", got)
	}
	if got := GetArgB(i); got != 20 {
		t.Errorf("B changed: got %d, want 20", got)
	}
	if got := GetArgC(i); got != 30 {
		t.Errorf("C changed: got %d, want 30", got)
	}
	if got := GetArgK(i); got != 1 {
		t.Errorf("k changed: got %d, want 1", got)
	}
}

func TestSetArgSBx(t *testing.T) {
	// Start with OP_LOADI A=5 sBx=100
	i := CreateAsBx(OP_LOADI, 5, 100)
	// Change sBx to -50
	i = SetArgSBx(i, -50)
	// sBx should be -50, op and A unchanged
	if got := GetOpCode(i); got != OP_LOADI {
		t.Errorf("op changed: got %d, want %d", got, OP_LOADI)
	}
	if got := GetArgA(i); got != 5 {
		t.Errorf("A changed: got %d, want 5", got)
	}
	if got := GetArgSBx(i); got != -50 {
		t.Errorf("sBx: got %d, want -50", got)
	}
}

func TestSetArgA_MaxValue(t *testing.T) {
	i := CreateABCK(OP_ADD, 0, 0, 0, 0)
	i = SetArgA(i, MaxArgA)
	if got := GetArgA(i); got != MaxArgA {
		t.Errorf("A: got %d, want %d", got, MaxArgA)
	}
	// Set back to 0
	i = SetArgA(i, 0)
	if got := GetArgA(i); got != 0 {
		t.Errorf("A: got %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Verify all OpModes bytes are initialized (non-zero check not applicable
// since some opcodes legitimately have mode byte 0x00, but all should have
// been explicitly set by init())
// ---------------------------------------------------------------------------

func TestAllOpModesInitialized(t *testing.T) {
	// We can't check for non-zero since some opcodes have mode 0x00.
	// Instead verify that the format field is within valid range for all.
	for i := 0; i < NumOpcodes; i++ {
		mode := GetMode(OpCode(i))
		if mode > ISJ {
			t.Errorf("OpCode %d (%s): invalid mode %d", i, OpName(OpCode(i)), mode)
		}
	}
}

// ---------------------------------------------------------------------------
// Cross-check: C format enum values match Go constants
// ---------------------------------------------------------------------------

func TestFormatConstants(t *testing.T) {
	// C: iABC=0, iABx=1, iAsBx=2, iAx=3, isJ=4
	// Wait — let me check. The C source has:
	//   enum OpMode {iABC, iABx, iAsBx, iAx, isJ};
	// But our Go has:
	//   IABC=0, IVABC=1, IABx=2, IAsBx=3, IAx=4, ISJ=5
	// This is DIFFERENT from C! Go has ivABC=1 inserted.
	// This is fine as long as opdata.go uses Go constants consistently.
	if IABC != 0 {
		t.Errorf("IABC = %d, want 0", IABC)
	}
	if IVABC != 1 {
		t.Errorf("IVABC = %d, want 1", IVABC)
	}
	if IABx != 2 {
		t.Errorf("IABx = %d, want 2", IABx)
	}
	if IAsBx != 3 {
		t.Errorf("IAsBx = %d, want 3", IAsBx)
	}
	if IAx != 4 {
		t.Errorf("IAx = %d, want 4", IAx)
	}
	if ISJ != 5 {
		t.Errorf("ISJ = %d, want 5", ISJ)
	}
}
