package internal

import (
	"testing"

	"github.com/akzj/go-lua/ast/api"
)

func TestExpKind(t *testing.T) {
	// Test that exp kinds are in expected order (iota)
	if api.EXP_VOID != 0 {
		t.Errorf("EXP_VOID should be 0, got %d", api.EXP_VOID)
	}
	if api.EXP_NIL != 1 {
		t.Errorf("EXP_NIL should be 1, got %d", api.EXP_NIL)
	}
	if api.EXP_TRUE != 2 {
		t.Errorf("EXP_TRUE should be 2, got %d", api.EXP_TRUE)
	}
	if api.EXP_FALSE != 3 {
		t.Errorf("EXP_FALSE should be 3, got %d", api.EXP_FALSE)
	}
	if api.EXP_K != 4 {
		t.Errorf("EXP_K should be 4, got %d", api.EXP_K)
	}
	if api.EXP_KINTEGER != 5 {
		t.Errorf("EXP_KINTEGER should be 5, got %d", api.EXP_KINTEGER)
	}
	if api.EXP_KFLOAT != 6 {
		t.Errorf("EXP_KFLOAT should be 6, got %d", api.EXP_KFLOAT)
	}
	if api.EXP_KSTRING != 7 {
		t.Errorf("EXP_KSTRING should be 7, got %d", api.EXP_KSTRING)
	}
	// New kinds from lua-master
	if api.EXP_NONRELOC != 8 {
		t.Errorf("EXP_NONRELOC should be 8, got %d", api.EXP_NONRELOC)
	}
	if api.EXP_VARARG != 10 {
		t.Errorf("EXP_VARARG should be 10, got %d", api.EXP_VARARG)
	}
	if api.EXP_VARARG_EXP != 22 {
		t.Errorf("EXP_VARARG_EXP should be 22, got %d", api.EXP_VARARG_EXP)
	}
}

func TestStatKind(t *testing.T) {
	// Test that stat kinds are in expected order
	if api.STAT_ASSIGN != 0 {
		t.Errorf("STAT_ASSIGN should be 0, got %d", api.STAT_ASSIGN)
	}
}

func TestBinopKind(t *testing.T) {
	// Test that binop kinds are in expected order
	if api.BINOP_ADD != 0 {
		t.Errorf("BINOP_ADD should be 0, got %d", api.BINOP_ADD)
	}
	if api.BINOP_CONCAT != 17 {
		t.Errorf("BINOP_CONCAT should be 17, got %d", api.BINOP_CONCAT)
	}
}

func TestUnopKind(t *testing.T) {
	if api.UNOP_NEG != 0 {
		t.Errorf("UNOP_NEG should be 0, got %d", api.UNOP_NEG)
	}
}

func TestNilExp(t *testing.T) {
	exp := &NilExp{}
	exp.Line, exp.Column = 1, 5

	if !exp.IsConstant() {
		t.Error("NilExp should be constant")
	}

	line, col := exp.Position()
	if line != 1 || col != 5 {
		t.Errorf("Position() = (%d, %d), want (1, 5)", line, col)
	}
}

func TestTrueExp(t *testing.T) {
	exp := &TrueExp{}
	exp.Line = 1

	if !exp.IsConstant() {
		t.Error("TrueExp should be constant")
	}
}

func TestFalseExp(t *testing.T) {
	exp := &FalseExp{}

	if !exp.IsConstant() {
		t.Error("FalseExp should be constant")
	}
}

func TestIntegerExp(t *testing.T) {
	exp := &IntegerExp{Value: 42}

	if !exp.IsConstant() {
		t.Error("IntegerExp should be constant")
	}
}

func TestFloatExp(t *testing.T) {
	exp := &FloatExp{Value: 3.14}

	if !exp.IsConstant() {
		t.Error("FloatExp should be constant")
	}
}

func TestStringExp(t *testing.T) {
	exp := &StringExp{Value: "hello"}

	if !exp.IsConstant() {
		t.Error("StringExp should be constant")
	}
}

func TestVarargExp(t *testing.T) {
	exp := &VarargExp{}

	if exp.IsConstant() {
		t.Error("VarargExp should not be constant")
	}
}

func TestNameExp(t *testing.T) {
	exp := &NameExp{Name: "x"}

	if exp.IsConstant() {
		t.Error("NameExp should not be constant")
	}
}

func TestExpDescImpl(t *testing.T) {
	desc := &ExpDescImpl{
		Kind_:    api.EXP_LOCAL,
		Reg_:     5,
		Info_:    10,
	}

	if desc.Kind() != api.EXP_LOCAL {
		t.Errorf("Kind() = %d, want %d", desc.Kind(), api.EXP_LOCAL)
	}

	if desc.Reg() != 5 {
		t.Errorf("Reg() = %d, want 5", desc.Reg())
	}

	if desc.Info() != 10 {
		t.Errorf("Info() = %d, want 10", desc.Info())
	}

	desc.SetKind(api.EXP_GLOBAL)
	if desc.Kind() != api.EXP_GLOBAL {
		t.Error("SetKind failed")
	}

	desc.SetReg(8)
	if desc.Reg() != 8 {
		t.Error("SetReg failed")
	}

	desc.SetTable(3, 4)
	tReg, kReg := desc.Table()
	if tReg != 3 || kReg != 4 {
		t.Errorf("Table() = (%d, %d), want (3, 4)", tReg, kReg)
	}

	desc.SetKeyIsString(true)
	if !desc.KeyIsString() {
		t.Error("KeyIsString() should be true after SetKeyIsString(true)")
	}

	desc.SetKeyIsString(false)
	if desc.KeyIsString() {
		t.Error("KeyIsString() should be false after SetKeyIsString(false)")
	}

	desc.SetTrueJump(100)
	if desc.TrueJump() != 100 {
		t.Error("SetTrueJump failed")
	}

	desc.SetFalseJump(200)
	if desc.FalseJump() != 200 {
		t.Error("SetFalseJump failed")
	}
}

func TestBinopExp(t *testing.T) {
	left := &IntegerExp{Value: 1}
	right := &IntegerExp{Value: 2}
	exp := &BinopExp{Op: api.BINOP_ADD, Left: left, Right: right}

	if !exp.IsConstant() {
		t.Error("BinopExp with constant operands should be constant")
	}
}

func TestUnopExp(t *testing.T) {
	sub := &IntegerExp{Value: -5}
	exp := &UnopExp{Op: api.UNOP_NEG, Exp: sub}

	if !exp.IsConstant() {
		t.Error("UnopExp with constant operand should be constant")
	}
}

func TestTableConstructorImpl(t *testing.T) {
	tc := &TableConstructorImpl{}
	tc.AddArrayField(&IntegerExp{Value: 1})
	tc.AddArrayField(&IntegerExp{Value: 2})
	tc.AddRecordField(&StringExp{Value: "key"}, &IntegerExp{Value: 3})

	if tc.NumFields() != 2 {
		t.Errorf("NumFields() = %d, want 2", tc.NumFields())
	}

	if tc.NumRecords() != 1 {
		t.Errorf("NumRecords() = %d, want 1", tc.NumRecords())
	}

	if !tc.IsConstant() {
		t.Error("TableConstructorImpl with constant fields should be constant")
	}
}

func TestFuncCallImpl(t *testing.T) {
	fc := &FuncCallImpl{
		Func_:       &NameExp{Name: "print"},
		Args_:       []api.ExpNode{&StringExp{Value: "hello"}},
		NumResults_: 0,
	}

	if fc.Func() == nil {
		t.Error("Func() should not be nil")
	}

	if len(fc.Args()) != 1 {
		t.Errorf("Args() length = %d, want 1", len(fc.Args()))
	}

	if fc.NumResults() != 0 {
		t.Error("NumResults() should be 0")
	}
}

func TestFuncDefImpl(t *testing.T) {
	fd := &FuncDefImpl{
		Line_:    1,
		LastLine_: 10,
		IsLocal_: true,
	}

	if !fd.IsLocal() {
		t.Error("IsLocal() should be true")
	}

	if fd.Line() != 1 {
		t.Error("Line() should be 1")
	}

	if fd.LastLine() != 10 {
		t.Error("LastLine() should be 10")
	}
}

func TestBlockImpl(t *testing.T) {
	block := &BlockImpl{
		Stats_:     []api.StatNode{&ReturnStat{}},
		ReturnExp_: []api.ExpNode{&IntegerExp{Value: 42}},
	}

	if len(block.Stats()) != 1 {
		t.Errorf("Stats() length = %d, want 1", len(block.Stats()))
	}

	if len(block.ReturnExp()) != 1 {
		t.Errorf("ReturnExp() length = %d, want 1", len(block.ReturnExp()))
	}
}

func TestChunkImpl(t *testing.T) {
	chunk := &ChunkImpl{
		Block_:      &BlockImpl{},
		SourceName_: "=(stdin)",
	}

	if chunk.SourceName() != "=(stdin)" {
		t.Error("SourceName() should be '=(stdin)'")
	}
}

func TestVarDescImpl(t *testing.T) {
	vd := &VarDescImpl{
		Name_:    "x",
		IsLocal_: true,
		Reg_:     5,
		Index_:   0,
	}

	if vd.Name() != "x" {
		t.Error("Name() should be 'x'")
	}

	if !vd.IsLocal() {
		t.Error("IsLocal() should be true")
	}

	if vd.IsGlobal() {
		t.Error("IsGlobal() should be false")
	}

	if vd.Reg() != 5 {
		t.Error("Reg() should be 5")
	}
}

func TestLocalVarsImpl(t *testing.T) {
	lv := &LocalVarsImpl{}
	lv.Add("a", 0)
	lv.Add("b", 1)

	if lv.Count() != 2 {
		t.Errorf("Count() = %d, want 2", lv.Count())
	}

	v0 := lv.Get(0)
	if v0 == nil || v0.Name() != "a" {
		t.Error("Get(0) should return 'a'")
	}

	lv.Reset()
	if lv.Count() != 0 {
		t.Error("After Reset(), Count() should be 0")
	}
}

func TestAssignStat(t *testing.T) {
	stat := &AssignStat{}
	if stat.IsScopeEnd() {
		t.Error("AssignStat should not end scope")
	}
}

func TestReturnStat(t *testing.T) {
	stat := &ReturnStat{}
	if !stat.IsScopeEnd() {
		t.Error("ReturnStat should end scope")
	}
}

func TestBreakStat(t *testing.T) {
	stat := &BreakStat{}
	if !stat.IsScopeEnd() {
		t.Error("BreakStat should end scope")
	}
}
