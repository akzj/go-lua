package metamethod

import (
	"testing"

	"github.com/akzj/go-lua/internal/luastring"
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/table"
)

// ---------------------------------------------------------------------------
// TMS enum tests
// ---------------------------------------------------------------------------

func TestTMS_Values(t *testing.T) {
	if TM_INDEX != 0 {
		t.Errorf("TM_INDEX = %d, want 0", TM_INDEX)
	}
	if TM_NEWINDEX != 1 {
		t.Errorf("TM_NEWINDEX = %d, want 1", TM_NEWINDEX)
	}
	if TM_EQ != 5 {
		t.Errorf("TM_EQ = %d, want 5", TM_EQ)
	}
	if TM_ADD != 6 {
		t.Errorf("TM_ADD = %d, want 6", TM_ADD)
	}
	if TM_CLOSE != 24 {
		t.Errorf("TM_CLOSE = %d, want 24", TM_CLOSE)
	}
	if TM_N != 25 {
		t.Errorf("TM_N = %d, want 25", TM_N)
	}
}

func TestTMNames_Complete(t *testing.T) {
	expected := [25]string{
		"__index", "__newindex", "__gc", "__mode", "__len", "__eq",
		"__add", "__sub", "__mul", "__mod", "__pow", "__div", "__idiv",
		"__band", "__bor", "__bxor", "__shl", "__shr",
		"__unm", "__bnot", "__lt", "__le",
		"__concat", "__call", "__close",
	}
	for i := TMS(0); i < TM_N; i++ {
		if TMNames[i] != expected[i] {
			t.Errorf("TMNames[%d] = %q, want %q", i, TMNames[i], expected[i])
		}
	}
}

// ---------------------------------------------------------------------------
// fasttm cache tests
// ---------------------------------------------------------------------------

func TestHasFastTM(t *testing.T) {
	// All bits clear = all might be present
	var flags byte = 0
	for i := TMS(0); i <= TM_EQ; i++ {
		if !hasFastTM(flags, i) {
			t.Errorf("HasFastTM(0, %d) should be true", i)
		}
	}

	// Set TM_INDEX as absent
	setAbsent(&flags, TM_INDEX)
	if hasFastTM(flags, TM_INDEX) {
		t.Error("After SetAbsent, TM_INDEX should be absent")
	}
	// Others still present
	if !hasFastTM(flags, TM_NEWINDEX) {
		t.Error("TM_NEWINDEX should still be present")
	}

	// Invalidate cache
	invalidateCache(&flags)
	if !hasFastTM(flags, TM_INDEX) {
		t.Error("After InvalidateCache, TM_INDEX should be present again")
	}
}

// ---------------------------------------------------------------------------
// InitTMNames test
// ---------------------------------------------------------------------------

func TestInitTMNames(t *testing.T) {
	g := &state.GlobalState{}
	strtab := luastring.NewStringTable(12345)
	initTMNames(g, strtab)

	for i := TMS(0); i < TM_N; i++ {
		if g.TMNames[i] == nil {
			t.Errorf("TMNames[%d] is nil after InitTMNames", i)
			continue
		}
		if g.TMNames[i].Data != TMNames[i] {
			t.Errorf("TMNames[%d] = %q, want %q", i, g.TMNames[i].Data, TMNames[i])
		}
		if !g.TMNames[i].IsShort {
			t.Errorf("TMNames[%d] should be short string", i)
		}
	}
}

// ---------------------------------------------------------------------------
// GetTM tests
// ---------------------------------------------------------------------------

func newTestGlobal() (*state.GlobalState, *luastring.StringTable) {
	g := &state.GlobalState{}
	strtab := luastring.NewStringTable(42)
	g.StringTable = strtab
	initTMNames(g, strtab)
	return g, strtab
}

func TestGetTM_NilMetatable(t *testing.T) {
	g, _ := newTestGlobal()
	result := GetTM(nil, TM_INDEX, g.TMNames[TM_INDEX])
	if !result.IsNil() {
		t.Error("GetTM with nil metatable should return Nil")
	}
}

func TestGetTM_Absent_Cached(t *testing.T) {
	g, _ := newTestGlobal()
	mt := table.New(0, 4)

	// First lookup — TM_INDEX not in table
	result := GetTM(mt, TM_INDEX, g.TMNames[TM_INDEX])
	if !result.IsNil() {
		t.Error("GetTM should return Nil for absent metamethod")
	}

	// Check that absence is now cached
	if mt.HasTagMethod(byte(TM_INDEX)) {
		t.Error("TM_INDEX absence should be cached after lookup")
	}

	// Second lookup — should hit cache (fast path)
	result = GetTM(mt, TM_INDEX, g.TMNames[TM_INDEX])
	if !result.IsNil() {
		t.Error("Cached GetTM should return Nil")
	}
}

func TestGetTM_Present(t *testing.T) {
	g, _ := newTestGlobal()
	mt := table.New(0, 4)

	// Set __index metamethod
	indexFn := object.MakeInteger(42) // any non-nil value works
	mt.SetStr(g.TMNames[TM_INDEX], indexFn)

	result := GetTM(mt, TM_INDEX, g.TMNames[TM_INDEX])
	if result.IsNil() {
		t.Error("GetTM should return the metamethod value")
	}
	if result.Integer() != 42 {
		t.Errorf("GetTM result = %v, want 42", result)
	}
}

func TestGetTM_NonFast_Absent(t *testing.T) {
	g, _ := newTestGlobal()
	mt := table.New(0, 4)

	// TM_ADD (index 6) is not a fast TM
	result := GetTM(mt, TM_ADD, g.TMNames[TM_ADD])
	if !result.IsNil() {
		t.Error("GetTM should return Nil for absent non-fast metamethod")
	}
}

func TestGetTM_NonFast_Present(t *testing.T) {
	g, _ := newTestGlobal()
	mt := table.New(0, 4)

	// Set __add metamethod
	mt.SetStr(g.TMNames[TM_ADD], object.MakeInteger(99))

	result := GetTM(mt, TM_ADD, g.TMNames[TM_ADD])
	if result.IsNil() {
		t.Error("GetTM should return the metamethod value")
	}
	if result.Integer() != 99 {
		t.Errorf("GetTM result = %v, want 99", result)
	}
}

func TestGetTM_CacheInvalidation(t *testing.T) {
	g, _ := newTestGlobal()
	mt := table.New(0, 4)

	// First: absent → cached
	GetTM(mt, TM_INDEX, g.TMNames[TM_INDEX])
	if mt.HasTagMethod(byte(TM_INDEX)) {
		t.Error("Should be cached as absent")
	}

	// Invalidate cache (simulates metatable change)
	mt.InvalidateFlags()

	// Now it should check again
	if !mt.HasTagMethod(byte(TM_INDEX)) {
		t.Error("After invalidation, should report as possibly present")
	}
}

// ---------------------------------------------------------------------------
// GetTMByObj tests
// ---------------------------------------------------------------------------

func TestGetTMByObj_TableWithMetatable(t *testing.T) {
	g, _ := newTestGlobal()

	// Create a table with a metatable that has __index
	tbl := table.New(0, 0)
	mt := table.New(0, 4)
	mt.SetStr(g.TMNames[TM_INDEX], object.MakeInteger(42))
	tbl.SetMetatable(mt)

	obj := object.TValue{Tt: object.TagTable, Val: tbl}
	result := GetTMByObj(g, obj, TM_INDEX)
	if result.IsNil() {
		t.Error("Should find __index in table's metatable")
	}
	if result.Integer() != 42 {
		t.Errorf("result = %v, want 42", result)
	}
}

func TestGetTMByObj_TableNoMetatable(t *testing.T) {
	g, _ := newTestGlobal()

	tbl := table.New(0, 0)
	obj := object.TValue{Tt: object.TagTable, Val: tbl}

	result := GetTMByObj(g, obj, TM_INDEX)
	if !result.IsNil() {
		t.Error("Table without metatable should return Nil")
	}
}

func TestGetTMByObj_NumberWithGlobalMT(t *testing.T) {
	g, _ := newTestGlobal()

	// Set global metatable for numbers (type 3)
	numMT := table.New(0, 4)
	numMT.SetStr(g.TMNames[TM_ADD], object.MakeInteger(77))
	g.MT[object.TypeNumber] = numMT

	obj := object.MakeInteger(10)
	result := GetTMByObj(g, obj, TM_ADD)
	if result.IsNil() {
		t.Error("Should find __add in global number metatable")
	}
	if result.Integer() != 77 {
		t.Errorf("result = %v, want 77", result)
	}
}

func TestGetTMByObj_NumberNoGlobalMT(t *testing.T) {
	g, _ := newTestGlobal()

	obj := object.MakeInteger(10)
	result := GetTMByObj(g, obj, TM_ADD)
	if !result.IsNil() {
		t.Error("Without global metatable, should return Nil")
	}
}

func TestGetTMByObj_StringWithGlobalMT(t *testing.T) {
	g, strtab := newTestGlobal()

	// Set global metatable for strings (type 4)
	strMT := table.New(0, 4)
	strMT.SetStr(g.TMNames[TM_INDEX], object.MakeInteger(55))
	g.MT[object.TypeString] = strMT

	s := strtab.Intern("hello")
	obj := object.MakeString(s)
	result := GetTMByObj(g, obj, TM_INDEX)
	if result.IsNil() {
		t.Error("Should find __index in global string metatable")
	}
	if result.Integer() != 55 {
		t.Errorf("result = %v, want 55", result)
	}
}

func TestGetTMByObj_NilValue(t *testing.T) {
	g, _ := newTestGlobal()

	result := GetTMByObj(g, object.Nil, TM_INDEX)
	if !result.IsNil() {
		t.Error("Nil value should have no metatable")
	}
}

// ---------------------------------------------------------------------------
// ObjTypeName tests
// ---------------------------------------------------------------------------

func TestObjTypeName_Basic(t *testing.T) {
	L := state.NewState()
	g := L.Global

	tests := []struct {
		val  object.TValue
		want string
	}{
		{object.Nil, "nil"},
		{object.True, "boolean"},
		{object.False, "boolean"},
		{object.MakeInteger(42), "number"},
		{object.MakeFloat(3.14), "number"},
	}

	for _, tt := range tests {
		got := ObjTypeName(g, tt.val)
		if got != tt.want {
			t.Errorf("ObjTypeName(%v) = %q, want %q", tt.val, got, tt.want)
		}
	}
}

func TestObjTypeName_TableWithName(t *testing.T) {
	L := state.NewState()
	g := L.Global
	strtab := g.StringTable.(*luastring.StringTable)

	tbl := table.New(0, 0)
	mt := table.New(0, 4)
	nameKey := strtab.Intern("__name")
	nameVal := strtab.Intern("MyClass")
	mt.SetStr(nameKey, object.MakeString(nameVal))
	tbl.SetMetatable(mt)

	obj := object.TValue{Tt: object.TagTable, Val: tbl}
	got := ObjTypeName(g, obj)
	if got != "MyClass" {
		t.Errorf("ObjTypeName = %q, want %q", got, "MyClass")
	}
}

func TestObjTypeName_TableWithoutName(t *testing.T) {
	L := state.NewState()
	g := L.Global

	tbl := table.New(0, 0)
	obj := object.TValue{Tt: object.TagTable, Val: tbl}
	got := ObjTypeName(g, obj)
	if got != "table" {
		t.Errorf("ObjTypeName = %q, want %q", got, "table")
	}
}
