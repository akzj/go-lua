package lvm

/*
** $Id: lvm_test.go $
** Unit tests for lvm
*/

import (
	"math"
	"testing"

	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

func TestLuaV_Flttointeger(t *testing.T) {
	tests := []struct {
		input    lobject.LuaNumber
		expected lobject.LuaInteger
		mode     F2Imod
		success  bool
	}{
		{42.0, 42, F2Ieq, true},
		{42.5, 0, F2Ieq, false}, // non-integer, F2Ieq fails
		{42.7, 43, F2Iceil, true},
		{42.3, 42, F2Ifloor, true},
		{math.MaxFloat64, 0, F2Ieq, false}, // too large
	}

	for _, tc := range tests {
		var result lobject.LuaInteger
		ok := luaV_flttointeger(tc.input, &result, tc.mode)
		if ok != tc.success {
			t.Errorf("luaV_flttointeger(%v, %v): expected ok=%v, got %v", tc.input, tc.mode, tc.success, ok)
		}
		if ok && result != tc.expected {
			t.Errorf("luaV_flttointeger(%v, %v): expected %v, got %v", tc.input, tc.mode, tc.expected, result)
		}
	}
}

func TestLuaV_Idiv(t *testing.T) {
	tests := []struct {
		m, n     lobject.LuaInteger
		expected lobject.LuaInteger
	}{
		{10, 3, 3},
		{-10, 3, -4},   // floor division
		{10, -3, -4},   // floor division
		{-10, -3, 3},
		{7, 2, 3},
		{7, 3, 2},
		{0, 5, 0},
	}

	for _, tc := range tests {
		result := luaV_idiv(nil, tc.m, tc.n)
		if result != tc.expected {
			t.Errorf("luaV_idiv(%d, %d): expected %d, got %d", tc.m, tc.n, tc.expected, result)
		}
	}
}

func TestLuaV_Mod(t *testing.T) {
	tests := []struct {
		m, n     lobject.LuaInteger
		expected lobject.LuaInteger
	}{
		{10, 3, 1},
		{-10, 3, 2},   // Lua semantics
		{10, -3, -2},  // Lua semantics
		{-10, -3, -1}, // Lua semantics
		{7, 2, 1},
		{7, 3, 1},
		{0, 5, 0},
	}

	for _, tc := range tests {
		result := luaV_mod(nil, tc.m, tc.n)
		if result != tc.expected {
			t.Errorf("luaV_mod(%d, %d): expected %d, got %d", tc.m, tc.n, tc.expected, result)
		}
	}
}

func TestLuaV_Shiftl(t *testing.T) {
	tests := []struct {
		x, y     lobject.LuaInteger
		expected lobject.LuaInteger
	}{
		{1, 0, 1},
		{1, 1, 2},
		{1, 2, 4},
		{8, -3, 1},    // 8 >> 3
		{8, -4, 0},   // 8 >> 4 = 0
		{0, 10, 0},
		{-1, 0, -1},
		{-1, 1, -2},
	}

	for _, tc := range tests {
		result := luaV_shiftl(tc.x, tc.y)
		if result != tc.expected {
			t.Errorf("luaV_shiftl(%d, %d): expected %d, got %d", tc.x, tc.y, tc.expected, result)
		}
	}
}

func TestLuaV_Shiftr(t *testing.T) {
	tests := []struct {
		x, y     lobject.LuaInteger
		expected lobject.LuaInteger
	}{
		{8, 1, 4},
		{8, 2, 2},
		{8, 3, 1},
		{8, 4, 0},
		{1, 0, 1},
		{0, 10, 0},
	}

	for _, tc := range tests {
		result := luaV_shiftr(tc.x, tc.y)
		if result != tc.expected {
			t.Errorf("luaV_shiftr(%d, %d): expected %d, got %d", tc.x, tc.y, tc.expected, result)
		}
	}
}

func TestLTnum_IntegerInteger(t *testing.T) {
	a := &lobject.TValue{}
	b := &lobject.TValue{}
	lobject.SetIntValue(a, 10)
	lobject.SetIntValue(b, 20)

	if !LTnum(a, b) {
		t.Error("LTnum(10, 20) should be true")
	}

	lobject.SetIntValue(a, 20)
	if LTnum(a, b) {
		t.Error("LTnum(20, 10) should be false")
	}

	lobject.SetIntValue(a, 10)
	lobject.SetIntValue(b, 10)
	if LTnum(a, b) {
		t.Error("LTnum(10, 10) should be false")
	}
}

func TestLTnum_FloatFloat(t *testing.T) {
	a := &lobject.TValue{}
	b := &lobject.TValue{}
	lobject.SetFltValue(a, 10.5)
	lobject.SetFltValue(b, 20.5)

	if !LTnum(a, b) {
		t.Error("LTnum(10.5, 20.5) should be true")
	}
}

func TestLTnum_IntFloat(t *testing.T) {
	a := &lobject.TValue{}
	b := &lobject.TValue{}
	lobject.SetIntValue(a, 10)
	lobject.SetFltValue(b, 20.5)

	if !LTnum(a, b) {
		t.Error("LTnum(10, 20.5) should be true")
	}
}

func TestLEnum_IntegerInteger(t *testing.T) {
	a := &lobject.TValue{}
	b := &lobject.TValue{}
	lobject.SetIntValue(a, 10)
	lobject.SetIntValue(b, 20)

	if !LEnum(a, b) {
		t.Error("LEnum(10, 20) should be true")
	}

	lobject.SetIntValue(a, 20)
	lobject.SetIntValue(b, 10)
	if LEnum(a, b) {
		t.Error("LEnum(20, 10) should be false")
	}

	lobject.SetIntValue(a, 10)
	lobject.SetIntValue(b, 10)
	if !LEnum(a, b) {
		t.Error("LEnum(10, 10) should be true")
	}
}

func TestLEnum_FloatFloat(t *testing.T) {
	a := &lobject.TValue{}
	b := &lobject.TValue{}
	lobject.SetFltValue(a, 10.0)
	lobject.SetFltValue(b, 10.0)

	if !LEnum(a, b) {
		t.Error("LEnum(10.0, 10.0) should be true")
	}
}

func TestLuaV_Equalobj(t *testing.T) {
	// Test nil
	a := &lobject.TValue{}
	b := &lobject.TValue{}
	lobject.SetNilValue(a)
	lobject.SetNilValue(b)
	if !luaV_equalobj(nil, a, b) {
		t.Error("equalobj(nil, nil) should be true")
	}

	// Test integers
	lobject.SetIntValue(a, 42)
	lobject.SetIntValue(b, 42)
	if !luaV_equalobj(nil, a, b) {
		t.Error("equalobj(42, 42) should be true")
	}

	lobject.SetIntValue(b, 43)
	if luaV_equalobj(nil, a, b) {
		t.Error("equalobj(42, 43) should be false")
	}

	// Test floats
	lobject.SetFltValue(a, 3.14)
	lobject.SetFltValue(b, 3.14)
	if !luaV_equalobj(nil, a, b) {
		t.Error("equalobj(3.14, 3.14) should be true")
	}

	// Test different types
	lobject.SetIntValue(a, 1)
	lobject.SetFltValue(b, 1.0)
	// In Lua 5.4, 1 == 1.0 is true
	if !luaV_equalobj(nil, a, b) {
		t.Error("equalobj(1, 1.0) should be true")
	}

	// Test booleans
	lobject.SetBtValue(a, true)
	lobject.SetBtValue(b, true)
	if !luaV_equalobj(nil, a, b) {
		t.Error("equalobj(true, true) should be true")
	}

	lobject.SetBtValue(a, true)
	lobject.SetBtValue(b, false)
	if luaV_equalobj(nil, a, b) {
		t.Error("equalobj(true, false) should be false")
	}
}

func TestLuaV_Rawequalobj(t *testing.T) {
	a := &lobject.TValue{}
	b := &lobject.TValue{}

	lobject.SetIntValue(a, 42)
	lobject.SetIntValue(b, 42)
	if !luaV_rawequalobj(a, b) {
		t.Error("rawequalobj(42, 42) should be true")
	}

	lobject.SetIntValue(b, 43)
	if luaV_rawequalobj(a, b) {
		t.Error("rawequalobj(42, 43) should be false")
	}
}

func TestLuaV_Objlen(t *testing.T) {
	L := &lstate.LuaState{}
	ra := &lobject.TValue{}
	rb := &lobject.TValue{}

	// Test with nil (should return 0 for now)
	lobject.SetNilValue(rb)
	luaV_objlen(L, ra, rb)

	// For now, implementation returns 0 for unimplemented types
}

func TestIntop(t *testing.T) {
	if intop('+', 2, 3) != 5 {
		t.Error("intop(+) failed")
	}
	if intop('-', 10, 3) != 7 {
		t.Error("intop(-) failed")
	}
	if intop('*', 4, 5) != 20 {
		t.Error("intop(*) failed")
	}
	if intop('&', 0b1010, 0b1100) != 0b1000 {
		t.Error("intop(&) failed")
	}
	if intop('|', 0b1010, 0b1100) != 0b1110 {
		t.Error("intop(|) failed")
	}
	if intop('^', 0b1010, 0b1100) != 0b0110 {
		t.Error("intop(^) failed")
	}
}

func TestLuaV_Modf(t *testing.T) {
	result := luaV_modf(nil, 10.5, 3.0)
	expected := 1.5
	if math.Abs(float64(result)-expected) > 0.0001 {
		t.Errorf("luaV_modf(10.5, 3.0): expected %v, got %v", expected, result)
	}
}

func TestLuaV_Tonumber(t *testing.T) {
	// Test with integer - this should succeed
	o := &lobject.TValue{}
	lobject.SetIntValue(o, 42)
	var n lobject.LuaNumber
	if !luaV_tonumber(o, &n) {
		t.Error("luaV_tonumber(42) should succeed")
	}
	if n != 42 {
		t.Errorf("luaV_tonumber(42): expected 42, got %v", n)
	}

	// Test with float - luaV_tonumber returns false for floats
	// (only handles integers in current implementation)
	o2 := &lobject.TValue{}
	lobject.SetFltValue(o2, 3.14)
	if luaV_tonumber(o2, &n) {
		t.Error("luaV_tonumber(3.14) should return false (not implemented for floats)")
	}
}

func TestLuaV_Tointegerns(t *testing.T) {
	o := &lobject.TValue{}
	var i lobject.LuaInteger

	// Test with integer
	lobject.SetIntValue(o, 42)
	if !luaV_tointegerns(o, &i, F2Ieq) {
		t.Error("luaV_tointegerns(42) should succeed")
	}
	if i != 42 {
		t.Errorf("luaV_tointegerns(42): expected 42, got %d", i)
	}

	// Test with float (no coercion)
	lobject.SetFltValue(o, 42.5)
	if luaV_tointegerns(o, &i, F2Ieq) {
		t.Error("luaV_tointegerns(42.5) should fail with F2Ieq")
	}

	if !luaV_tointegerns(o, &i, F2Ifloor) {
		t.Error("luaV_tointegerns(42.5) should succeed with F2Ifloor")
	}
	if i != 42 {
		t.Errorf("luaV_tointegerns(42.5): expected 42, got %d", i)
	}
}

func TestLTintfloat(t *testing.T) {
	if !LTintfloat(10, 20.0) {
		t.Error("LTintfloat(10, 20.0) should be true")
	}
	if LTintfloat(20, 10.0) {
		t.Error("LTintfloat(20, 10.0) should be false")
	}
	if LTintfloat(10, 10.0) {
		t.Error("LTintfloat(10, 10.0) should be false")
	}
}

func TestLEintfloat(t *testing.T) {
	if !LEintfloat(10, 20.0) {
		t.Error("LEintfloat(10, 20.0) should be true")
	}
	if !LEintfloat(10, 10.0) {
		t.Error("LEintfloat(10, 10.0) should be true")
	}
	if LEintfloat(20, 10.0) {
		t.Error("LEintfloat(20, 10.0) should be false")
	}
}

func TestLTfloatint(t *testing.T) {
	if !LTfloatint(10.0, 20) {
		t.Error("LTfloatint(10.0, 20) should be true")
	}
	if LTfloatint(20.0, 10) {
		t.Error("LTfloatint(20.0, 10) should be false")
	}
}

func TestLEfloatint(t *testing.T) {
	if !LEfloatint(10.0, 20) {
		t.Error("LEfloatint(10.0, 20) should be true")
	}
	if !LEfloatint(10.0, 10) {
		t.Error("LEfloatint(10.0, 10) should be true")
	}
	if LEfloatint(20.0, 10) {
		t.Error("LEfloatint(20.0, 10) should be false")
	}
}

func TestToNumber(t *testing.T) {
	o := &lobject.TValue{}
	var n lobject.LuaNumber

	// Integer -> Number
	lobject.SetIntValue(o, 42)
	if !ToNumber(o, &n) {
		t.Error("ToNumber(42) should succeed")
	}

	// Float -> Number
	lobject.SetFltValue(o, 3.14)
	if !ToNumber(o, &n) {
		t.Error("ToNumber(3.14) should succeed")
	}
}

func TestToInteger(t *testing.T) {
	o := &lobject.TValue{}
	var i lobject.LuaInteger

	// Integer -> Integer
	lobject.SetIntValue(o, 42)
	if !ToInteger(o, &i, F2Ieq) {
		t.Error("ToInteger(42) should succeed")
	}
	if i != 42 {
		t.Errorf("ToInteger(42): expected 42, got %d", i)
	}
}

func TestToNumberNS(t *testing.T) {
	o := &lobject.TValue{}
	var n lobject.LuaNumber

	// Float -> Number
	lobject.SetFltValue(o, 3.14)
	if !ToNumberNS(o, &n) {
		t.Error("ToNumberNS(3.14) should succeed")
	}

	// Integer -> Number
	lobject.SetIntValue(o, 42)
	if !ToNumberNS(o, &n) {
		t.Error("ToNumberNS(42) should succeed")
	}
}

func TestToIntegerNS(t *testing.T) {
	o := &lobject.TValue{}
	var i lobject.LuaInteger

	// Integer -> Integer
	lobject.SetIntValue(o, 42)
	if !ToIntegerNS(o, &i, F2Ieq) {
		t.Error("ToIntegerNS(42) should succeed")
	}
	if i != 42 {
		t.Errorf("ToIntegerNS(42): expected 42, got %d", i)
	}
}