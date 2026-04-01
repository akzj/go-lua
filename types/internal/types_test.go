package internal

import (
	"testing"

	"github.com/akzj/go-lua/types/api"
)

func TestTValueTypeChecks(t *testing.T) {
	// Test nil value
	tv := NewTValueNil()
	if !tv.IsNil() {
		t.Error("expected IsNil() to be true")
	}

	// Test boolean values
	tvTrue := NewTValueBoolean(true)
	if !tvTrue.IsTrue() {
		t.Error("expected IsTrue() to be true")
	}
	if tvTrue.IsFalse() {
		t.Error("expected IsFalse() to be false for true value")
	}

	tvFalse := NewTValueBoolean(false)
	if !tvFalse.IsFalse() {
		t.Error("expected IsFalse() to be true")
	}

	// Test integer
	tvInt := NewTValueInteger(42)
	if !tvInt.IsInteger() {
		t.Error("expected IsInteger() to be true")
	}
	if !tvInt.IsNumber() {
		t.Error("expected IsNumber() to be true")
	}

	// Test float
	tvFloat := NewTValueFloat(3.14)
	if !tvFloat.IsFloat() {
		t.Error("expected IsFloat() to be true")
	}
}

func TestTableCreation(t *testing.T) {
	table := NewTable()
	if table == nil {
		t.Error("expected NewTable() to return non-nil")
	}
	if table.SizeNode() != 1 {
		t.Error("expected empty table to have SizeNode() == 0")
	}
}

func TestStringCreation(t *testing.T) {
	// Short string
	s := NewString("hello")
	if s == nil {
		t.Error("expected NewString() to return non-nil")
	}
	if s.Len() != 5 {
		t.Errorf("expected Len() == 5, got %d", s.Len())
	}
	if !s.IsShort() {
		t.Error("expected short string to return IsShort() == true")
	}

	// Long string
	longStr := NewString("this is a much longer string that exceeds short string limit")
	if longStr.IsShort() {
		t.Error("expected long string to return IsShort() == false")
	}
}

func TestTypeConstants(t *testing.T) {
	// Verify type constants are correctly defined
	if api.LUA_TNIL != 0 {
		t.Errorf("expected LUA_TNIL == 0, got %d", api.LUA_TNIL)
	}
	if api.LUA_TBOOLEAN != 1 {
		t.Errorf("expected LUA_TBOOLEAN == 1, got %d", api.LUA_TBOOLEAN)
	}
	if api.LUA_NUMTYPES != 8 {
		t.Errorf("expected LUA_NUMTYPES == 8, got %d", api.LUA_NUMTYPES)
	}

	// Verify variant constants
	if api.LUA_VFALSE != api.LUA_TBOOLEAN|(0<<4) {
		t.Errorf("LUA_VFALSE incorrect")
	}
	if api.LUA_VTRUE != api.LUA_TBOOLEAN|(1<<4) {
		t.Errorf("LUA_VTRUE incorrect")
	}
}

func TestUtilityFunctions(t *testing.T) {
	// Test MakeVariant
	v := api.MakeVariant(api.LUA_TNUMBER, 1)
	if v != api.LUA_VNUMFLT {
		t.Errorf("MakeVariant(LUA_TNUMBER, 1) = %d, want %d", v, api.LUA_VNUMFLT)
	}

	// Test Ctb (mark as collectable)
	ct := api.Ctb(api.LUA_VTABLE)
	if ct != api.LUA_VTABLE|api.BIT_ISCOLLECTABLE {
		t.Error("Ctb incorrect")
	}

	// Test Novariant
	nv := api.Novariant(api.LUA_VNUMFLT)
	if nv != api.LUA_TNUMBER {
		t.Errorf("Novariant(LUA_VNUMFLT) = %d, want %d", nv, api.LUA_TNUMBER)
	}
}

func TestNodeKeyTypes(t *testing.T) {
	node := &Node{}

	// Test KeyIsNil - default is 0, which is LUA_TNIL
	if !node.KeyIsNil() {
		t.Error("expected KeyIsNil() to be true for default node")
	}
	if node.KeyIsDead() {
		t.Error("expected KeyIsDead() to be false for nil key")
	}
	if node.KeyIsCollectable() {
		t.Error("expected KeyIsCollectable() to be false for nil key")
	}

	// Test with dead key
	node.KeyTt = uint8(api.LUA_TDEADKEY)
	if node.KeyIsNil() {
		t.Error("expected KeyIsNil() to be false for dead key")
	}
	if !node.KeyIsDead() {
		t.Error("expected KeyIsDead() to be true")
	}
	if node.KeyIsCollectable() {
		t.Error("expected KeyIsCollectable() to be false for dead key")
	}

	// Test with collectable key (table)
	node.KeyTt = uint8(api.Ctb(int(api.LUA_VTABLE)))
	if node.KeyIsNil() {
		t.Error("expected KeyIsNil() to be false for table key")
	}
	if node.KeyIsDead() {
		t.Error("expected KeyIsDead() to be false for table key")
	}
	if !node.KeyIsCollectable() {
		t.Error("expected KeyIsCollectable() to be true for table key")
	}
}

func TestTableFlagsAndMetatable(t *testing.T) {
	table := NewTable().(*Table)

	// Test default Flags
	if table.Flags != 0 {
		t.Errorf("expected Flags == 0, got %d", table.Flags)
	}

	// Set and verify Flags
	table.Flags = 5
	if table.Flags != 5 {
		t.Errorf("expected Flags == 5, got %d", table.Flags)
	}

	// Test default Metatable
	if table.Metatable != nil {
		t.Error("expected default Metatable to be nil")
	}

	// Set and verify Metatable
	mt := NewTable().(*Table)
	table.Metatable = mt
	if table.Metatable != mt {
		t.Error("expected Metatable to be set")
	}
}

func TestClosureTypes(t *testing.T) {
	// Test CClosure
	cClosure := &CClosure{
		ClosureHeader: ClosureHeader{
			GCObject:  GCObject{Tt: uint8(api.Ctb(int(api.LUA_VCCL)))},
			Nupvalues: 2,
		},
	}
	if int(cClosure.Nupvalues) != 2 {
		t.Errorf("expected Nupvalues == 2, got %d", cClosure.Nupvalues)
	}

	// Test LClosure
	proto := &Proto{
		GCObject:      GCObject{Tt: uint8(api.Ctb(int(api.LUA_VPROTO)))},
		Numparams:     1,
		Maxstacksize: 10,
	}
	lClosure := &LClosure{
		ClosureHeader: ClosureHeader{
			GCObject:  GCObject{Tt: uint8(api.Ctb(int(api.LUA_VLCL)))},
			Nupvalues: 3,
		},
		Proto:  proto,
		Upvals: []*UpVal{},
	}
	if lClosure.Nupvalues != 3 {
		t.Errorf("expected Nupvalues == 3, got %d", lClosure.Nupvalues)
	}
	if lClosure.Proto != proto {
		t.Error("expected Proto to be set")
	}

	// Test Closure wrapper
	closure := &Closure{
		IsCClosure: true,
		C:          cClosure,
	}
	if !closure.IsC() {
		t.Error("expected IsC() to be true for CClosure")
	}

	closure.IsCClosure = false
	closure.L = lClosure
	if closure.IsC() {
		t.Error("expected IsC() to be false for LClosure")
	}
}

func TestProtoType(t *testing.T) {
	proto := &Proto{
		GCObject:      GCObject{Tt: uint8(api.Ctb(int(api.LUA_VPROTO)))},
		Numparams:     2,
		Flag:          0,
		Maxstacksize: 20,
	}

	// Test IsVararg with no flags
	if proto.IsVararg() {
		t.Error("expected IsVararg() to be false with no flags")
	}

	// Test IsVararg with VARARG_HIDDEN
	proto.Flag = api.PF_VARARG_HIDDEN
	if !proto.IsVararg() {
		t.Error("expected IsVararg() to be true with VARARG_HIDDEN")
	}

	// Test IsVararg with VARARG_TABLE
	proto.Flag = api.PF_VARARG_TABLE
	if !proto.IsVararg() {
		t.Error("expected IsVararg() to be true with VARARG_TABLE")
	}

	// Test Proto fields
	if proto.Numparams != 2 {
		t.Errorf("expected Numparams == 2, got %d", proto.Numparams)
	}
	if proto.Maxstacksize != 20 {
		t.Errorf("expected Maxstacksize == 20, got %d", proto.Maxstacksize)
	}
}

func TestMoreTValueTypeChecks(t *testing.T) {
	// Test IsLightUserData
	tvLightUD := &TValue{
		Tt: uint8(api.LUA_VLIGHTUSERDATA),
	}
	if !tvLightUD.IsLightUserData() {
		t.Error("expected IsLightUserData() to be true")
	}
	if tvLightUD.IsCollectable() {
		t.Error("expected IsCollectable() to be false for light userdata")
	}

	// Test IsThread
	tvThread := &TValue{
		Tt: uint8(api.Ctb(int(api.LUA_VTHREAD))),
	}
	if !tvThread.IsThread() {
		t.Error("expected IsThread() to be true")
	}
	if !tvThread.IsCollectable() {
		t.Error("expected IsCollectable() to be true for thread")
	}

	// Test IsProto
	tvProto := &TValue{
		Tt: uint8(api.Ctb(int(api.LUA_VPROTO))),
	}
	if !tvProto.IsProto() {
		t.Error("expected IsProto() to be true")
	}

	// Test IsUpval
	tvUpval := &TValue{
		Tt: uint8(api.Ctb(int(api.LUA_VUPVAL))),
	}
	if !tvUpval.IsUpval() {
		t.Error("expected IsUpval() to be true")
	}

	// Test IsEmpty
	tvNil := NewTValueNil()
	if !tvNil.IsEmpty() {
		t.Error("expected IsEmpty() to be true for nil value")
	}

	// Test GetTag and GetBaseType
	if tvThread.GetTag() != int(api.Ctb(int(api.LUA_VTHREAD))) {
		t.Error("GetTag returned unexpected value for thread")
	}
	if tvThread.GetBaseType() != api.LUA_TTHREAD {
		t.Errorf("GetBaseType = %d, want %d", tvThread.GetBaseType(), api.LUA_TTHREAD)
	}
}

func TestUpValType(t *testing.T) {
	upval := &UpVal{
		GCObject: GCObject{Tt: uint8(api.Ctb(int(api.LUA_VUPVAL)))},
	}
	if int(upval.Tt) != api.Ctb(int(api.LUA_VUPVAL)) {
		t.Error("UpVal Tt not set correctly")
	}

	// Test open UpVal structure
	upval.U.Open.Next = nil
	upval.U.Open.Previous = nil
	if upval.U.Open.Next != nil {
		t.Error("UpVal Open.Next should be nil")
	}
}

func TestGCObjectIsTable(t *testing.T) {
	// Test GCObject.IsTable() for Table
	table := NewTable().(*Table)
	if !table.IsTable() {
		t.Error("expected IsTable() to be true for Table")
	}

	// Test GCObject.IsTable() for non-table
	gcObj := &GCObject{Tt: uint8(api.LUA_TNIL)}
	if gcObj.IsTable() {
		t.Error("expected IsTable() to be false for non-table")
	}
}
