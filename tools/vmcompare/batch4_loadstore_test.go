package vmcompare

import "testing"

var batch4LoadStore = []vmTest{
	// --- OP_MOVE ---
	{"MOVE_basic", `local a=42; local b=a; print(b)`, []string{"OP_MOVE"}},
	{"MOVE_nil", `local a=nil; local b=a; print(b)`, []string{"OP_MOVE"}},
	{"MOVE_string", `local a="hello"; local b=a; print(b)`, []string{"OP_MOVE"}},

	// --- OP_LOADI ---
	{"LOADI_zero", `local a=0; print(a)`, []string{"OP_LOADI"}},
	{"LOADI_positive", `local a=100; print(a)`, []string{"OP_LOADI"}},
	{"LOADI_negative", `local a=-100; print(a)`, []string{"OP_LOADI"}},

	// --- OP_LOADF ---
	{"LOADF_zero", `local a=0.0; print(a)`, []string{"OP_LOADF"}},
	{"LOADF_one", `local a=1.0; print(a)`, []string{"OP_LOADF"}},

	// --- OP_LOADK ---
	{"LOADK_string", `local a="hello world"; print(a)`, []string{"OP_LOADK"}},
	{"LOADK_large_int", `local a=1000000; print(a)`, []string{"OP_LOADK"}},
	{"LOADK_float", `local a=3.14159; print(a)`, []string{"OP_LOADK"}},

	// --- OP_LOADTRUE / OP_LOADFALSE / OP_LOADNIL ---
	{"LOADTRUE", `local a=true; print(a)`, []string{"OP_LOADTRUE"}},
	{"LOADFALSE", `local a=false; print(a)`, []string{"OP_LOADFALSE"}},
	{"LOADNIL_single", `local a; print(a)`, []string{"OP_LOADNIL"}},
	{"LOADNIL_multi", `local a,b,c; print(a,b,c)`, []string{"OP_LOADNIL"}},

	// --- OP_GETGLOBAL / OP_SETGLOBAL (via GETTABUP/SETTABUP on _ENV) ---
	{"GETGLOBAL_math", `print(math.pi)`, []string{"OP_GETTABUP"}},
	{"SETGLOBAL_basic", `x=42; print(x)`, []string{"OP_SETTABUP"}},
	{"SETGLOBAL_nil", `x=42; x=nil; print(x)`, []string{"OP_SETTABUP"}},

	// --- OP_GETTABLE / OP_SETTABLE ---
	{"GETTABLE_array", `local t={10,20,30}; print(t[2])`, []string{"OP_GETTABLE"}},
	{"SETTABLE_array", `local t={}; t[1]=42; print(t[1])`, []string{"OP_SETTABLE"}},
	{"GETTABLE_string_key", `local t={x=10}; local k="x"; print(t[k])`, []string{"OP_GETTABLE"}},

	// --- OP_GETI / OP_SETI ---
	{"GETI_basic", `local t={10,20,30}; print(t[1])`, []string{"OP_GETI"}},
	{"SETI_basic", `local t={}; t[1]=99; print(t[1])`, []string{"OP_SETI"}},
	{"GETI_outofrange", `local t={10,20}; print(t[5])`, []string{"OP_GETI"}},

	// --- OP_GETFIELD / OP_SETFIELD ---
	{"GETFIELD_basic", `local t={x=10}; print(t.x)`, []string{"OP_GETFIELD"}},
	{"SETFIELD_basic", `local t={}; t.x=42; print(t.x)`, []string{"OP_SETFIELD"}},
	{"GETFIELD_nil", `local t={}; print(t.x)`, []string{"OP_GETFIELD"}},
	{"SETFIELD_overwrite", `local t={x=1}; t.x=2; print(t.x)`, []string{"OP_SETFIELD"}},

	// --- OP_GETUPVAL / OP_SETUPVAL ---
	{"GETUPVAL_basic", `local x=10; local f=function() return x end; print(f())`, []string{"OP_GETUPVAL"}},
	{"SETUPVAL_basic", `local x=10; local f=function() x=20 end; f(); print(x)`, []string{"OP_SETUPVAL"}},
	{"UPVAL_chain", `local x=0; local f=function() x=x+1 end; f(); f(); f(); print(x)`, nil},

	// --- OP_NEWTABLE / OP_SETLIST ---
	{"NEWTABLE_empty", `local t={}; print(type(t))`, []string{"OP_NEWTABLE"}},
	{"SETLIST_array", `local t={1,2,3,4,5}; print(t[1],t[3],t[5])`, []string{"OP_SETLIST"}},
	{"SETLIST_mixed", `local t={1,2,x=3}; print(t[1],t[2],t.x)`, []string{"OP_SETLIST"}},
	{"NEWTABLE_large", `local t={}; for i=1,100 do t[i]=i end; print(t[50],t[100])`, nil},
}

func TestBatch4LoadStore(t *testing.T) {
	match, mismatch := 0, 0
	for _, tc := range batch4LoadStore {
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
	t.Logf("Batch 4 Load/Store: %d match, %d mismatch out of %d", match, mismatch, len(batch4LoadStore))
}
