package vmcompare

import (
	"testing"
)

// ===========================================================================
// Smoke test — verify framework works
// ===========================================================================

func TestSmoke(t *testing.T) {
	tests := []vmTest{
		{"print_int", `print(42)`, nil},
		{"print_float", `print(3.14)`, nil},
		{"print_string", `print("hello")`, nil},
	}
	match, mismatch := 0, 0
	for _, tc := range tests {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			ok, cOut, goOut := CompareVM(t, tc)
			if ok {
				match++
			} else {
				mismatch++
				t.Errorf("MISMATCH\n  C:  %q\n  Go: %q", cOut, goOut)
			}
		})
	}
	t.Logf("Smoke: %d match, %d mismatch", match, mismatch)
}

// ===========================================================================
// Batch 1: Arithmetic opcodes
// ===========================================================================

var batch1Arithmetic = []vmTest{
	// --- OP_ADD / OP_ADDK / OP_ADDI ---
	{"ADD_int_int", `local a,b=10,20; print(a+b)`, []string{"OP_ADD"}},
	{"ADD_float_float", `local a,b=1.5,2.5; print(a+b)`, []string{"OP_ADD"}},
	{"ADD_int_float", `local a,b=10,1.5; print(a+b)`, []string{"OP_ADD"}},
	{"ADD_string_coerce", `print("10"+5)`, []string{"OP_ADD"}},
	{"ADDI_basic", `local a=10; a=a+1; print(a)`, []string{"OP_ADDI"}},
	{"ADDK_float_const", `local a=10; print(a+3.5)`, []string{"OP_ADDK"}},

	// --- OP_SUB / OP_SUBK ---
	{"SUB_int_int", `local a,b=10,3; print(a-b)`, []string{"OP_SUB"}},
	{"SUB_float_float", `local a,b=1.5,0.5; print(a-b)`, []string{"OP_SUB"}},
	{"SUBK_basic", `local a=10; print(a-3)`, []string{"OP_SUBK"}},

	// --- OP_MUL / OP_MULK ---
	{"MUL_int_int", `local a,b=6,7; print(a*b)`, []string{"OP_MUL"}},
	{"MULK_basic", `local a=6; print(a*7)`, []string{"OP_MULK"}},
	{"MUL_float", `local a,b=2.5,4.0; print(a*b)`, []string{"OP_MUL"}},

	// --- OP_MOD / OP_MODK ---
	{"MOD_int_int", `local a,b=10,3; print(a%b)`, []string{"OP_MOD"}},
	{"MOD_negative", `local a,b=-7,3; print(a%b)`, []string{"OP_MOD"}},
	{"MODK_basic", `local a=10; print(a%3)`, []string{"OP_MODK"}},
	{"MOD_float", `local a,b=5.5,2.0; print(a%b)`, []string{"OP_MOD"}},

	// --- OP_POW / OP_POWK ---
	{"POW_int_int", `local a,b=2,10; print(a^b)`, []string{"OP_POW"}},
	{"POWK_basic", `local a=2; print(a^10)`, []string{"OP_POWK"}},
	{"POW_float", `local a,b=2.0,0.5; print(a^b)`, []string{"OP_POW"}},

	// --- OP_DIV / OP_DIVK ---
	{"DIV_int_int", `local a,b=10,3; print(a/b)`, []string{"OP_DIV"}},
	{"DIVK_basic", `local a=10; print(a/3)`, []string{"OP_DIVK"}},
	{"DIV_exact", `local a,b=10,2; print(a/b)`, []string{"OP_DIV"}},

	// --- OP_IDIV / OP_IDIVK ---
	{"IDIV_int_int", `local a,b=10,3; print(a//b)`, []string{"OP_IDIV"}},
	{"IDIV_negative", `local a,b=-7,2; print(a//b)`, []string{"OP_IDIV"}},
	{"IDIVK_basic", `local a=10; print(a//3)`, []string{"OP_IDIVK"}},
	{"IDIV_float", `local a,b=7.5,2.0; print(a//b)`, []string{"OP_IDIV"}},

	// --- OP_UNM ---
	{"UNM_int", `local a=10; print(-a)`, []string{"OP_UNM"}},
	{"UNM_float", `local a=1.5; print(-a)`, []string{"OP_UNM"}},
	{"UNM_zero", `local a=0; print(-a)`, []string{"OP_UNM"}},
	{"UNM_string_coerce", `print(-"10")`, []string{"OP_UNM"}},

	// --- Overflow / edge cases ---
	{"ADD_maxint_1", `print(math.maxinteger + 1)`, nil},
	{"ADD_minint_neg1", `print(math.mininteger - 1)`, nil},
	{"DIV_by_zero_float", `print(1/0)`, nil},
	{"DIV_zero_zero", `print(0/0)`, nil},
	{"POW_large", `print(2^53)`, nil},
	{"POW_large_plus1", `print(2^53 + 1)`, nil},
	{"INT_literal_large", `print(9007199254740993)`, nil},
	{"MOD_by_zero_int", `print(pcall(function() return 1%0 end))`, nil},
	{"IDIV_by_zero_int", `print(pcall(function() return 1//0 end))`, nil},
	{"MININT_negate", `print(-math.mininteger)`, nil},
}

func TestBatch1Arithmetic(t *testing.T) {
	match, mismatch := 0, 0
	for _, tc := range batch1Arithmetic {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			ok, cOut, goOut := CompareVM(t, tc)
			if ok {
				match++
				t.Logf("MATCH: %s", cOut)
			} else {
				mismatch++
				t.Errorf("MISMATCH\n  C:  %q\n  Go: %q", cOut, goOut)
			}
		})
	}
	t.Logf("Batch 1 Arithmetic: %d match, %d mismatch out of %d", match, mismatch, len(batch1Arithmetic))
}

// ===========================================================================
// Batch 2: Comparison opcodes
// ===========================================================================

var batch2Comparison = []vmTest{
	// --- OP_EQ ---
	{"EQ_int_int_true", `print(1 == 1)`, []string{"OP_EQ"}},
	{"EQ_int_int_false", `print(1 == 2)`, []string{"OP_EQ"}},
	{"EQ_float_int_true", `print(1.0 == 1)`, []string{"OP_EQ"}},
	{"EQ_float_float_false", `print(1.0 == 1.1)`, []string{"OP_EQ"}},
	{"EQ_string_true", `print("a" == "a")`, []string{"OP_EQ"}},
	{"EQ_string_false", `print("a" == "b")`, []string{"OP_EQ"}},
	{"EQ_nil_nil", `print(nil == nil)`, []string{"OP_EQ"}},
	{"EQ_nil_false", `print(nil == false)`, []string{"OP_EQ"}},
	{"EQ_bool_true", `print(true == true)`, []string{"OP_EQ"}},
	{"EQ_bool_mixed", `print(true == false)`, []string{"OP_EQ"}},
	{"NE_basic", `print(1 ~= 2)`, []string{"OP_EQ"}},

	// --- OP_EQK / OP_EQI ---
	{"EQK_const", `local a=10; if a==10 then print("y") else print("n") end`, []string{"OP_EQK"}},
	{"EQI_imm", `local a=5; if a==5 then print("y") else print("n") end`, []string{"OP_EQI"}},
	{"EQI_false", `local a=5; if a==6 then print("y") else print("n") end`, []string{"OP_EQI"}},

	// --- OP_LT ---
	{"LT_int_true", `print(1 < 2)`, []string{"OP_LT"}},
	{"LT_int_false", `print(2 < 1)`, []string{"OP_LT"}},
	{"LT_int_equal", `print(1 < 1)`, []string{"OP_LT"}},
	{"LT_float", `print(1.5 < 2.5)`, []string{"OP_LT"}},
	{"LT_int_float_mixed", `print(1 < 1.5)`, []string{"OP_LT"}},
	{"LT_string", `print("abc" < "abd")`, []string{"OP_LT"}},
	{"LT_string_prefix", `print("ab" < "abc")`, []string{"OP_LT"}},

	// --- OP_LE ---
	{"LE_int_equal", `print(1 <= 1)`, []string{"OP_LE"}},
	{"LE_int_less", `print(1 <= 2)`, []string{"OP_LE"}},
	{"LE_int_greater", `print(2 <= 1)`, []string{"OP_LE"}},
	{"LE_float", `print(1.5 <= 1.5)`, []string{"OP_LE"}},

	// --- OP_LTI / OP_LEI / OP_GTI / OP_GEI ---
	{"LTI_true", `local a=5; if a<10 then print("y") else print("n") end`, []string{"OP_LTI"}},
	{"LTI_false", `local a=15; if a<10 then print("y") else print("n") end`, []string{"OP_LTI"}},
	{"LEI_equal", `local a=5; if a<=5 then print("y") else print("n") end`, []string{"OP_LEI"}},
	{"LEI_less", `local a=3; if a<=5 then print("y") else print("n") end`, []string{"OP_LEI"}},
	{"GTI_true", `local a=5; if a>3 then print("y") else print("n") end`, []string{"OP_GTI"}},
	{"GTI_false", `local a=1; if a>3 then print("y") else print("n") end`, []string{"OP_GTI"}},
	{"GEI_equal", `local a=5; if a>=5 then print("y") else print("n") end`, []string{"OP_GEI"}},
	{"GEI_greater", `local a=7; if a>=5 then print("y") else print("n") end`, []string{"OP_GEI"}},

	// --- Float-int precision edge cases (the bug we just fixed) ---
	{"EQ_precision_exact", `print(9007199254740992 == 9007199254740992.0)`, nil},
	{"EQ_precision_off", `print(9007199254740993 == 9007199254740993.0)`, nil},
	{"LT_precision", `print(9007199254740993 < 9007199254740993.0)`, nil},
	{"LE_precision", `print(9007199254740993 <= 9007199254740992.0)`, nil},
	{"GT_precision", `print(9007199254740993 > 9007199254740992.0)`, nil},

	// --- NaN / inf comparisons ---
	{"EQ_nan", `print(0/0 == 0/0)`, nil},
	{"NE_nan", `print(0/0 ~= 0/0)`, nil},
	{"EQ_inf", `print(math.huge == math.huge)`, nil},
	{"LT_inf", `print(math.huge > 1e308)`, nil},
	{"LT_neg_inf", `print(-math.huge < -1e308)`, nil},
	{"EQ_neg_zero", `print(0.0 == -0.0)`, nil},
}

func TestBatch2Comparison(t *testing.T) {
	match, mismatch := 0, 0
	for _, tc := range batch2Comparison {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			ok, cOut, goOut := CompareVM(t, tc)
			if ok {
				match++
				t.Logf("MATCH: %s", cOut)
			} else {
				mismatch++
				t.Errorf("MISMATCH\n  C:  %q\n  Go: %q", cOut, goOut)
			}
		})
	}
	t.Logf("Batch 2 Comparison: %d match, %d mismatch out of %d", match, mismatch, len(batch2Comparison))
}
