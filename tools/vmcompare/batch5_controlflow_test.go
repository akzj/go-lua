package vmcompare

import "testing"

var batch5ControlFlow = []vmTest{
	// --- OP_JMP ---
	{"JMP_if_true", `if true then print("y") else print("n") end`, []string{"OP_JMP"}},
	{"JMP_if_false", `if false then print("y") else print("n") end`, []string{"OP_JMP"}},
	{"JMP_elseif", `local a=2; if a==1 then print("one") elseif a==2 then print("two") else print("other") end`, nil},

	// --- OP_TEST / OP_TESTSET ---
	{"TEST_truthy_int", `local a=1; if a then print("y") else print("n") end`, []string{"OP_TEST"}},
	{"TEST_falsy_nil", `local a=nil; if a then print("y") else print("n") end`, []string{"OP_TEST"}},
	{"TEST_falsy_false", `local a=false; if a then print("y") else print("n") end`, []string{"OP_TEST"}},
	{"TEST_truthy_zero", `local a=0; if a then print("y") else print("n") end`, []string{"OP_TEST"}},
	{"TEST_truthy_emptystr", `local a=""; if a then print("y") else print("n") end`, []string{"OP_TEST"}},
	{"TESTSET_and", `local a=1; local b=2; print(a and b)`, []string{"OP_TESTSET"}},
	{"TESTSET_and_false", `local a=false; local b=2; print(a and b)`, []string{"OP_TESTSET"}},
	{"TESTSET_or", `local a=nil; local b=2; print(a or b)`, []string{"OP_TESTSET"}},
	{"TESTSET_or_true", `local a=1; local b=2; print(a or b)`, []string{"OP_TESTSET"}},
	{"AND_chain", `print(1 and 2 and 3)`, nil},
	{"OR_chain", `print(nil or false or 3)`, nil},
	{"AND_OR_mixed", `print(nil and 1 or 2)`, nil},

	// --- OP_NOT ---
	{"NOT_true", `print(not true)`, []string{"OP_NOT"}},
	{"NOT_false", `print(not false)`, []string{"OP_NOT"}},
	{"NOT_nil", `print(not nil)`, []string{"OP_NOT"}},
	{"NOT_number", `print(not 0)`, []string{"OP_NOT"}},
	{"NOT_string", `print(not "")`, []string{"OP_NOT"}},

	// --- OP_FORPREP / OP_FORLOOP (numeric for) ---
	{"FOR_basic", `for i=1,5 do print(i) end`, []string{"OP_FORPREP", "OP_FORLOOP"}},
	{"FOR_step", `for i=0,10,2 do print(i) end`, []string{"OP_FORPREP", "OP_FORLOOP"}},
	{"FOR_negative_step", `for i=5,1,-1 do print(i) end`, nil},
	{"FOR_float", `for i=0.0,1.0,0.5 do print(i) end`, nil},
	{"FOR_empty", `for i=5,1 do print(i) end`, nil},
	{"FOR_single", `for i=1,1 do print(i) end`, nil},

	// --- OP_TFORPREP / OP_TFORCALL / OP_TFORLOOP (generic for) ---
	{"TFOR_pairs", `local t={a=1,b=2}; local keys={}; for k,v in pairs(t) do keys[#keys+1]=k.."="..v end; table.sort(keys); for _,s in ipairs(keys) do print(s) end`, nil},
	{"TFOR_ipairs", `for i,v in ipairs({10,20,30}) do print(i,v) end`, nil},

	// --- OP_CALL ---
	{"CALL_noarg", `local function f() return 42 end; print(f())`, []string{"OP_CALL"}},
	{"CALL_args", `local function f(a,b) return a+b end; print(f(10,20))`, []string{"OP_CALL"}},
	{"CALL_multiret", `local function f() return 1,2,3 end; print(f())`, []string{"OP_CALL"}},
	{"CALL_nested", `local function f(x) return x*2 end; print(f(f(f(1))))`, nil},
	{"CALL_method", `local t={x=10}; function t:get() return self.x end; print(t:get())`, nil},

	// --- OP_RETURN / OP_RETURN0 / OP_RETURN1 ---
	{"RETURN_single", `local function f() return 42 end; print(f())`, []string{"OP_RETURN1"}},
	{"RETURN_multi", `local function f() return 1,2,3 end; local a,b,c=f(); print(a,b,c)`, []string{"OP_RETURN"}},
	{"RETURN_none", `local function f() end; print(f())`, []string{"OP_RETURN0"}},
	{"RETURN_discard", `local function f() return 1,2,3 end; local a=f(); print(a)`, nil},

	// --- OP_TAILCALL ---
	{"TAILCALL_basic", `local function f(n) if n<=0 then return "done" end return f(n-1) end; print(f(10))`, []string{"OP_TAILCALL"}},
	{"TAILCALL_deep", `local function f(n) if n<=0 then return n end return f(n-1) end; print(f(1000))`, nil},

	// --- OP_VARARG ---
	{"VARARG_basic", `local function f(...) print(...) end; f(1,2,3)`, []string{"OP_VARARG"}},
	{"VARARG_select", `local function f(...) return select("#",...) end; print(f(1,2,3))`, nil},
	{"VARARG_empty", `local function f(...) print(select("#",...)) end; f()`, nil},
	{"VARARG_table", `local function f(...) local t={...}; print(#t) end; f(1,2,3,4,5)`, nil},

	// --- while / repeat ---
	{"WHILE_basic", `local i=0; while i<5 do i=i+1 end; print(i)`, nil},
	{"REPEAT_basic", `local i=0; repeat i=i+1 until i>=5; print(i)`, nil},

	// --- break / goto ---
	{"BREAK_basic", `for i=1,10 do if i==5 then break end end; print("ok")`, nil},
	{"GOTO_basic", `do goto skip; print("no") ::skip:: end; print("yes")`, nil},
}

func TestBatch5ControlFlow(t *testing.T) {
	match, mismatch := 0, 0
	for _, tc := range batch5ControlFlow {
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
	t.Logf("Batch 5 Control Flow: %d match, %d mismatch out of %d", match, mismatch, len(batch5ControlFlow))
}
