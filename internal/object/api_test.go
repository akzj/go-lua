package object

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// Tag tests
// ---------------------------------------------------------------------------

func TestTagBaseType(t *testing.T) {
	tests := []struct {
		tag  Tag
		want Type
	}{
		{TagNil, TypeNil},
		{TagEmpty, TypeNil},
		{TagAbstKey, TypeNil},
		{TagNotable, TypeNil},
		{TagFalse, TypeBoolean},
		{TagTrue, TypeBoolean},
		{TagInteger, TypeNumber},
		{TagFloat, TypeNumber},
		{TagShortStr, TypeString},
		{TagLongStr, TypeString},
		{TagTable, TypeTable},
		{TagLuaClosure, TypeFunction},
		{TagLightCFunc, TypeFunction},
		{TagCClosure, TypeFunction},
		{TagUserdata, TypeUserdata},
		{TagThread, TypeThread},
		{TagLightUserdata, TypeLightUserdata},
	}
	for _, tt := range tests {
		if got := tt.tag.BaseType(); got != tt.want {
			t.Errorf("Tag(0x%02x).BaseType() = %d, want %d", tt.tag, got, tt.want)
		}
	}
}

func TestTagIsNil(t *testing.T) {
	nilTags := []Tag{TagNil, TagEmpty, TagAbstKey, TagNotable}
	for _, tag := range nilTags {
		if !tag.IsNil() {
			t.Errorf("Tag(0x%02x).IsNil() = false, want true", tag)
		}
	}
	nonNilTags := []Tag{TagFalse, TagTrue, TagInteger, TagFloat, TagShortStr, TagTable}
	for _, tag := range nonNilTags {
		if tag.IsNil() {
			t.Errorf("Tag(0x%02x).IsNil() = true, want false", tag)
		}
	}
}

func TestTagIsStrictNil(t *testing.T) {
	if !TagNil.IsStrictNil() {
		t.Error("TagNil.IsStrictNil() = false, want true")
	}
	if TagEmpty.IsStrictNil() {
		t.Error("TagEmpty.IsStrictNil() = true, want false")
	}
	if TagAbstKey.IsStrictNil() {
		t.Error("TagAbstKey.IsStrictNil() = true, want false")
	}
}

func TestTagIsFalsy(t *testing.T) {
	falsyTags := []Tag{TagNil, TagEmpty, TagAbstKey, TagNotable, TagFalse}
	for _, tag := range falsyTags {
		if !tag.IsFalsy() {
			t.Errorf("Tag(0x%02x).IsFalsy() = false, want true", tag)
		}
	}
	truthyTags := []Tag{TagTrue, TagInteger, TagFloat, TagShortStr, TagTable, TagLuaClosure}
	for _, tag := range truthyTags {
		if tag.IsFalsy() {
			t.Errorf("Tag(0x%02x).IsFalsy() = true, want false", tag)
		}
	}
}

func TestTagVariant(t *testing.T) {
	tests := []struct {
		tag  Tag
		want byte
	}{
		{TagNil, 0},
		{TagEmpty, 1},
		{TagAbstKey, 2},
		{TagNotable, 3},
		{TagFalse, 0},
		{TagTrue, 1},
		{TagInteger, 0},
		{TagFloat, 1},
		{TagLuaClosure, 0},
		{TagLightCFunc, 1},
		{TagCClosure, 2},
	}
	for _, tt := range tests {
		if got := tt.tag.Variant(); got != tt.want {
			t.Errorf("Tag(0x%02x).Variant() = %d, want %d", tt.tag, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TValue creation and type checking
// ---------------------------------------------------------------------------

func TestTValueNil(t *testing.T) {
	v := Nil
	if !v.IsNil() {
		t.Error("Nil.IsNil() = false")
	}
	if !v.IsFalsy() {
		t.Error("Nil.IsFalsy() = false")
	}
	if v.Type() != TypeNil {
		t.Errorf("Nil.Type() = %d, want %d", v.Type(), TypeNil)
	}
}

func TestTValueBoolean(t *testing.T) {
	f := MakeBoolean(false)
	tr := MakeBoolean(true)

	if f.Tag() != TagFalse {
		t.Errorf("MakeBoolean(false).Tag() = 0x%02x, want 0x%02x", f.Tag(), TagFalse)
	}
	if tr.Tag() != TagTrue {
		t.Errorf("MakeBoolean(true).Tag() = 0x%02x, want 0x%02x", tr.Tag(), TagTrue)
	}
	if f.Boolean() {
		t.Error("MakeBoolean(false).Boolean() = true")
	}
	if !tr.Boolean() {
		t.Error("MakeBoolean(true).Boolean() = false")
	}
	if !f.IsFalsy() {
		t.Error("false.IsFalsy() = false")
	}
	if tr.IsFalsy() {
		t.Error("true.IsFalsy() = true")
	}
}

func TestTValueInteger(t *testing.T) {
	v := MakeInteger(42)
	if v.Tag() != TagInteger {
		t.Errorf("tag = 0x%02x, want 0x%02x", v.Tag(), TagInteger)
	}
	if !v.IsInteger() {
		t.Error("IsInteger() = false")
	}
	if !v.IsNumber() {
		t.Error("IsNumber() = false")
	}
	if v.Integer() != 42 {
		t.Errorf("Integer() = %d, want 42", v.Integer())
	}
	if v.IsFalsy() {
		t.Error("integer.IsFalsy() = true")
	}
}

func TestTValueFloat(t *testing.T) {
	v := MakeFloat(3.14)
	if v.Tag() != TagFloat {
		t.Errorf("tag = 0x%02x, want 0x%02x", v.Tag(), TagFloat)
	}
	if !v.IsFloat() {
		t.Error("IsFloat() = false")
	}
	if !v.IsNumber() {
		t.Error("IsNumber() = false")
	}
	if v.Float() != 3.14 {
		t.Errorf("Float() = %f, want 3.14", v.Float())
	}
}

func TestTValueString(t *testing.T) {
	s := &LuaString{Data: "hello", Hash_: 12345, IsShort: true}
	v := MakeString(s)
	if v.Tag() != TagShortStr {
		t.Errorf("tag = 0x%02x, want 0x%02x", v.Tag(), TagShortStr)
	}
	if !v.IsString() {
		t.Error("IsString() = false")
	}
	if v.StringVal() != s {
		t.Error("StringVal() returned different pointer")
	}
	if v.StringVal().String() != "hello" {
		t.Errorf("StringVal().String() = %q, want %q", v.StringVal().String(), "hello")
	}
}

func TestTValueLongString(t *testing.T) {
	s := &LuaString{Data: "a long string", Hash_: 0, IsShort: false}
	v := MakeString(s)
	if v.Tag() != TagLongStr {
		t.Errorf("tag = 0x%02x, want 0x%02x", v.Tag(), TagLongStr)
	}
}

func TestTValueGCTypes(t *testing.T) {
	// Table
	type fakeTable struct{ x int }
	tbl := &fakeTable{42}
	tv := MakeTable(tbl)
	if tv.Tag() != TagTable {
		t.Errorf("MakeTable tag = 0x%02x, want 0x%02x", tv.Tag(), TagTable)
	}
	if !tv.IsTable() {
		t.Error("IsTable() = false")
	}
	if tv.TableVal() != tbl {
		t.Error("TableVal() returned different pointer")
	}

	// Lua closure
	type fakeLClosure struct{ x int }
	lc := &fakeLClosure{1}
	lcv := MakeLuaClosure(lc)
	if lcv.Tag() != TagLuaClosure {
		t.Errorf("MakeLuaClosure tag = 0x%02x", lcv.Tag())
	}
	if !lcv.IsFunction() {
		t.Error("LuaClosure.IsFunction() = false")
	}

	// C closure
	type fakeCClosure struct{ x int }
	cc := &fakeCClosure{2}
	ccv := MakeCClosure(cc)
	if ccv.Tag() != TagCClosure {
		t.Errorf("MakeCClosure tag = 0x%02x", ccv.Tag())
	}

	// Light C function
	fn := func() {}
	lcf := MakeLightCFunc(fn)
	if lcf.Tag() != TagLightCFunc {
		t.Errorf("MakeLightCFunc tag = 0x%02x", lcf.Tag())
	}

	// Userdata
	ud := &Userdata{Data: []byte{1, 2, 3}}
	udv := MakeUserdata(ud)
	if udv.Tag() != TagUserdata {
		t.Errorf("MakeUserdata tag = 0x%02x", udv.Tag())
	}
	if udv.UserdataVal() != ud {
		t.Error("UserdataVal() returned different pointer")
	}

	// Light userdata
	p := &struct{}{}
	luv := MakeLightUserdata(p)
	if luv.Tag() != TagLightUserdata {
		t.Errorf("MakeLightUserdata tag = 0x%02x", luv.Tag())
	}

	// Thread
	type fakeThread struct{ x int }
	th := &fakeThread{3}
	thv := MakeThread(th)
	if thv.Tag() != TagThread {
		t.Errorf("MakeThread tag = 0x%02x", thv.Tag())
	}

	// Proto
	p2 := &Proto{NumParams: 2}
	pv := MakeProto(p2)
	if pv.Tag() != TagProto {
		t.Errorf("MakeProto tag = 0x%02x", pv.Tag())
	}
	if pv.ProtoVal().NumParams != 2 {
		t.Error("ProtoVal() wrong")
	}
}

// ---------------------------------------------------------------------------
// Number coercion
// ---------------------------------------------------------------------------

func TestToNumber(t *testing.T) {
	tests := []struct {
		name string
		val  TValue
		want float64
		ok   bool
	}{
		{"integer", MakeInteger(42), 42.0, true},
		{"float", MakeFloat(3.14), 3.14, true},
		{"nil", Nil, 0, false},
		{"string", MakeString(&LuaString{Data: "10"}), 0, false}, // TValue.ToNumber doesn't coerce strings
		{"boolean", True, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.val.ToNumber()
			if ok != tt.ok {
				t.Errorf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("value = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestToInteger(t *testing.T) {
	tests := []struct {
		name string
		val  TValue
		want int64
		ok   bool
	}{
		{"integer", MakeInteger(42), 42, true},
		{"float_exact", MakeFloat(42.0), 42, true},
		{"float_frac", MakeFloat(42.5), 0, false},
		{"float_nan", MakeFloat(math.NaN()), 0, false},
		{"float_inf", MakeFloat(math.Inf(1)), 0, false},
		{"nil", Nil, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.val.ToInteger()
			if ok != tt.ok {
				t.Errorf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("value = %d, want %d", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// String → Number conversion
// ---------------------------------------------------------------------------

func TestStringToInteger(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		ok    bool
	}{
		{"42", 42, true},
		{"-42", -42, true},
		{"+42", 42, true},
		{"0", 0, true},
		{"  42  ", 42, true},
		{"0x1A", 26, true},
		{"0X1a", 26, true},
		{"0xff", 255, true},
		{"-0xff", -255, true},
		{"0xFFFFFFFFFFFFFFFF", -1, true}, // wraps around
		{"", 0, false},
		{"abc", 0, false},
		{"42.0", 0, false},
		{"42e2", 0, false},
		{"0x", 0, false},
		{"  ", 0, false},
		{"--42", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := StringToInteger(tt.input)
			if ok != tt.ok {
				t.Errorf("StringToInteger(%q): ok = %v, want %v", tt.input, ok, tt.ok)
				return
			}
			if ok && got != tt.want {
				t.Errorf("StringToInteger(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestStringToFloat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
		ok    bool
	}{
		{"3.14", 3.14, true},
		{"-3.14", -3.14, true},
		{"3.14e2", 314.0, true},
		{"3.14E-2", 0.0314, true},
		{".5", 0.5, true},
		{"5.", 5.0, true},
		{"  3.14  ", 3.14, true},
		{"0x1.0p4", 16.0, true},
		{"0x1.8p1", 3.0, true},
		{"0xA", 10.0, true}, // hex integer as float
		{"inf", 0, false},   // Lua rejects inf
		{"nan", 0, false},   // Lua rejects nan
		{"-inf", 0, false},
		{"", 0, false},
		{"abc", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := StringToFloat(tt.input)
			if ok != tt.ok {
				t.Errorf("StringToFloat(%q): ok = %v, want %v", tt.input, ok, tt.ok)
				return
			}
			if ok && math.Abs(got-tt.want) > 1e-10 {
				t.Errorf("StringToFloat(%q) = %g, want %g", tt.input, got, tt.want)
			}
		})
	}
}

func TestStringToNumber(t *testing.T) {
	// Integer takes priority
	v, ok := StringToNumber("42")
	if !ok || v.Tag() != TagInteger || v.Integer() != 42 {
		t.Errorf("StringToNumber(\"42\") = %v, %v", v, ok)
	}

	// Float
	v, ok = StringToNumber("3.14")
	if !ok || v.Tag() != TagFloat {
		t.Errorf("StringToNumber(\"3.14\") = %v, %v", v, ok)
	}

	// Hex integer
	v, ok = StringToNumber("0xff")
	if !ok || v.Tag() != TagInteger || v.Integer() != 255 {
		t.Errorf("StringToNumber(\"0xff\") = %v, %v", v, ok)
	}

	// Failure
	_, ok = StringToNumber("abc")
	if ok {
		t.Error("StringToNumber(\"abc\") should fail")
	}
}

// ---------------------------------------------------------------------------
// Number → String conversion
// ---------------------------------------------------------------------------

func TestIntegerToString(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{42, "42"},
		{-42, "-42"},
		{math.MaxInt64, "9223372036854775807"},
		{math.MinInt64, "-9223372036854775808"},
	}
	for _, tt := range tests {
		got := IntegerToString(tt.input)
		if got != tt.want {
			t.Errorf("IntegerToString(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFloatToString(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0.0, "0.0"},
		{1.0, "1.0"},
		{-1.0, "-1.0"},
		{3.14, "3.14"},
		{1e10, "10000000000.0"},
		{1e20, "1e+20"},
		{math.Inf(1), "inf"},
		{math.Inf(-1), "-inf"},
	}
	for _, tt := range tests {
		got := FloatToString(tt.input)
		if got != tt.want {
			t.Errorf("FloatToString(%g) = %q, want %q", tt.input, got, tt.want)
		}
	}

	// NaN — just check it produces something (exact output may vary)
	nanStr := FloatToString(math.NaN())
	if nanStr != "-nan" && nanStr != "nan" {
		t.Errorf("FloatToString(NaN) = %q, want \"-nan\" or \"nan\"", nanStr)
	}

	// Check that ".0" is appended to integer-like floats
	got := FloatToString(42.0)
	if got != "42.0" {
		t.Errorf("FloatToString(42.0) = %q, want \"42.0\"", got)
	}
}

// ---------------------------------------------------------------------------
// RawEqual
// ---------------------------------------------------------------------------

func TestRawEqual(t *testing.T) {
	s1 := &LuaString{Data: "hello", Hash_: 1, IsShort: true}
	s2 := &LuaString{Data: "hello", Hash_: 1, IsShort: true}
	s3 := &LuaString{Data: "world", Hash_: 2, IsShort: true}
	longS := &LuaString{Data: "hello", Hash_: 1, IsShort: false}

	tests := []struct {
		name string
		a, b TValue
		want bool
	}{
		// Same type, same value
		{"nil==nil", Nil, Nil, true},
		{"false==false", False, False, true},
		{"true==true", True, True, true},
		{"int==int", MakeInteger(42), MakeInteger(42), true},
		{"int!=int", MakeInteger(42), MakeInteger(43), false},
		{"float==float", MakeFloat(3.14), MakeFloat(3.14), true},
		{"float!=float", MakeFloat(3.14), MakeFloat(2.71), false},

		// NaN != NaN
		{"nan!=nan", MakeFloat(math.NaN()), MakeFloat(math.NaN()), false},

		// Integer vs float cross-comparison
		{"int==float_exact", MakeInteger(42), MakeFloat(42.0), true},
		{"float==int_exact", MakeFloat(42.0), MakeInteger(42), true},
		{"int!=float_frac", MakeInteger(42), MakeFloat(42.5), false},

		// Short strings: pointer equality
		{"shortstr_same_ptr", MakeString(s1), MakeString(s1), true},
		{"shortstr_diff_ptr", MakeString(s1), MakeString(s2), false}, // different pointers, even same content

		// Short vs long string: content equality
		{"short_vs_long", MakeString(s1), MakeString(longS), true},

		// Different content
		{"str_diff", MakeString(s1), MakeString(s3), false},

		// Different base types
		{"int!=str", MakeInteger(42), MakeString(s1), false},
		{"nil!=false", Nil, False, false},
		{"int!=nil", MakeInteger(0), Nil, false},

		// Empty/AbstKey nil variants
		{"empty==empty", Empty, Empty, true},
		{"nil!=empty", Nil, Empty, false}, // same base type, different variant, not number/string

		// Tables: pointer identity (use non-zero-size structs to avoid Go optimization)
		{"table_diff", MakeTable(&[1]int{1}), MakeTable(&[1]int{2}), false}, // different pointers
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RawEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("RawEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}

	// Table same pointer
	tbl := &struct{}{}
	if !RawEqual(MakeTable(tbl), MakeTable(tbl)) {
		t.Error("RawEqual(same table, same table) = false")
	}
}

// ---------------------------------------------------------------------------
// Proto.IsVararg
// ---------------------------------------------------------------------------

func TestProtoIsVararg(t *testing.T) {
	tests := []struct {
		flag byte
		want bool
	}{
		{0, false},
		{PF_VAHID, true},
		{PF_VATAB, true},
		{PF_VAHID | PF_VATAB, true},
		{PF_FIXED, false},
		{PF_FIXED | PF_VAHID, true},
	}
	for _, tt := range tests {
		p := &Proto{Flag: tt.flag}
		if got := p.IsVararg(); got != tt.want {
			t.Errorf("Proto{Flag: %d}.IsVararg() = %v, want %v", tt.flag, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// StackValue
// ---------------------------------------------------------------------------

func TestStackValue(t *testing.T) {
	sv := StackValue{Val: MakeInteger(42), TBCDelta: 0}
	if sv.Val.Integer() != 42 {
		t.Error("StackValue.Val wrong")
	}
	if sv.TBCDelta != 0 {
		t.Error("TBCDelta should be 0")
	}

	sv.TBCDelta = 5
	if sv.TBCDelta != 5 {
		t.Error("TBCDelta not set")
	}
}

// ---------------------------------------------------------------------------
// LuaString
// ---------------------------------------------------------------------------

func TestLuaString(t *testing.T) {
	short := &LuaString{Data: "hi", Hash_: 42, IsShort: true, Extra: 0}
	if short.Tag() != TagShortStr {
		t.Error("short string tag wrong")
	}
	if short.String() != "hi" {
		t.Error("String() wrong")
	}
	if short.Hash() != 42 {
		t.Error("Hash() wrong")
	}
	if short.Len() != 2 {
		t.Error("Len() wrong")
	}

	long := &LuaString{Data: "a long string here", Hash_: 0, IsShort: false}
	if long.Tag() != TagLongStr {
		t.Error("long string tag wrong")
	}
}

// ---------------------------------------------------------------------------
// TypeNameOf
// ---------------------------------------------------------------------------

func TestTypeNameOf(t *testing.T) {
	tests := []struct {
		val  TValue
		want string
	}{
		{Nil, "nil"},
		{True, "boolean"},
		{False, "boolean"},
		{MakeInteger(1), "number"},
		{MakeFloat(1.0), "number"},
		{MakeString(&LuaString{Data: "x", IsShort: true}), "string"},
		{MakeTable(nil), "table"},
		{MakeLuaClosure(nil), "function"},
		{MakeUserdata(&Userdata{}), "userdata"},
		{MakeThread(nil), "thread"},
	}
	for _, tt := range tests {
		got := TypeNameOf(tt.val)
		if got != tt.want {
			t.Errorf("TypeNameOf(%v) = %q, want %q", tt.val, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// SetObj
// ---------------------------------------------------------------------------

func TestSetObj(t *testing.T) {
	var dst TValue
	src := MakeInteger(99)
	SetObj(&dst, src)
	if dst.Tag() != TagInteger || dst.Integer() != 99 {
		t.Errorf("SetObj failed: got %v", dst)
	}
}

// ---------------------------------------------------------------------------
// Character classification helpers
// ---------------------------------------------------------------------------

func TestIsLuaSpace(t *testing.T) {
	spaces := []rune{' ', '\t', '\n', '\r', '\f', '\v'}
	for _, r := range spaces {
		if !IsLuaSpace(r) {
			t.Errorf("IsLuaSpace(%q) = false", r)
		}
	}
	if IsLuaSpace('a') {
		t.Error("IsLuaSpace('a') = true")
	}
}

func TestIsLuaDigit(t *testing.T) {
	for r := '0'; r <= '9'; r++ {
		if !IsLuaDigit(r) {
			t.Errorf("IsLuaDigit(%q) = false", r)
		}
	}
	if IsLuaDigit('a') {
		t.Error("IsLuaDigit('a') = true")
	}
}

func TestIsLuaAlpha(t *testing.T) {
	if !IsLuaAlpha('a') {
		t.Error("IsLuaAlpha('a') = false")
	}
	if !IsLuaAlpha('_') {
		t.Error("IsLuaAlpha('_') = false")
	}
	if IsLuaAlpha('1') {
		t.Error("IsLuaAlpha('1') = true")
	}
}
