package vmcompare

import "testing"

var batch3Bitwise = []vmTest{
	// --- OP_BAND / OP_BANDK ---
	{"BAND_basic", `local a,b=0xFF,0x0F; print(a&b)`, []string{"OP_BAND"}},
	{"BANDK_basic", `local a=0xFF; print(a&0x0F)`, []string{"OP_BANDK"}},
	{"BAND_zero", `local a=0xFF; print(a&0)`, []string{"OP_BAND"}},
	{"BAND_allones", `local a=0xFF; print(a&0xFF)`, []string{"OP_BAND"}},

	// --- OP_BOR / OP_BORK ---
	{"BOR_basic", `local a,b=0xF0,0x0F; print(a|b)`, []string{"OP_BOR"}},
	{"BORK_basic", `local a=0xF0; print(a|0x0F)`, []string{"OP_BORK"}},
	{"BOR_zero", `local a=0xFF; print(a|0)`, []string{"OP_BOR"}},

	// --- OP_BXOR / OP_BXORK ---
	{"BXOR_basic", `local a,b=0xFF,0x0F; print(a~b)`, []string{"OP_BXOR"}},
	{"BXORK_basic", `local a=0xFF; print(a~0x0F)`, []string{"OP_BXORK"}},
	{"BXOR_same", `local a=0xFF; print(a~a)`, []string{"OP_BXOR"}},
	{"BXOR_self_zero", `local a=42; print(a~a)`, []string{"OP_BXOR"}},

	// --- OP_SHL / OP_SHLI / OP_SHR / OP_SHRI ---
	{"SHL_basic", `local a=1; print(a<<4)`, []string{"OP_SHLI"}},
	{"SHL_var", `local a,b=1,4; print(a<<b)`, []string{"OP_SHL"}},
	{"SHR_basic", `local a=16; print(a>>2)`, []string{"OP_SHRI"}},
	{"SHR_var", `local a,b=16,2; print(a>>b)`, []string{"OP_SHR"}},
	{"SHL_large", `print(1<<62)`, nil},
	{"SHL_63", `print(1<<63)`, nil},
	{"SHR_negative", `print(-1>>1)`, nil},
	{"SHL_zero", `print(1<<0)`, nil},
	{"SHR_zero", `print(1>>0)`, nil},
	{"SHL_negative_shift", `print(1<<-1)`, nil},
	{"SHR_negative_shift", `print(1>>-1)`, nil},

	// --- OP_BNOT ---
	{"BNOT_basic", `local a=0; print(~a)`, []string{"OP_BNOT"}},
	{"BNOT_ff", `local a=0xFF; print(~a)`, []string{"OP_BNOT"}},
	{"BNOT_neg1", `local a=-1; print(~a)`, []string{"OP_BNOT"}},
	{"BNOT_maxint", `print(~math.maxinteger)`, nil},
	{"BNOT_minint", `print(~math.mininteger)`, nil},

	// --- Bitwise with string coercion (should error in 5.5) ---
	{"BAND_string_error", `print(pcall(function() return "15" & 0xFF end))`, nil},
	{"BOR_string_error", `print(pcall(function() return "15" | 0 end))`, nil},
}

func TestBatch3Bitwise(t *testing.T) {
	match, mismatch := 0, 0
	for _, tc := range batch3Bitwise {
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
	t.Logf("Batch 3 Bitwise: %d match, %d mismatch out of %d", match, mismatch, len(batch3Bitwise))
}
