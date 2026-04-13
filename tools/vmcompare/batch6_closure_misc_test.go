package vmcompare

import "testing"

var batch6ClosureMisc = []vmTest{
	// --- OP_CLOSURE ---
	{"CLOSURE_basic", `local function f() return 42 end; print(type(f))`, []string{"OP_CLOSURE"}},
	{"CLOSURE_upval", `local x=10; local f=function() return x end; print(f())`, []string{"OP_CLOSURE"}},
	{"CLOSURE_nested", `local function make(x) return function() return x end end; print(make(42)())`, nil},
	{"CLOSURE_shared_upval", `local x=0; local inc=function() x=x+1 end; local get=function() return x end; inc(); inc(); print(get())`, nil},
	{"CLOSURE_counter", `local function counter() local n=0; return function() n=n+1; return n end end; local c=counter(); print(c(),c(),c())`, nil},

	// --- OP_CONCAT ---
	{"CONCAT_two", `local a,b="hello"," world"; print(a..b)`, []string{"OP_CONCAT"}},
	{"CONCAT_multi", `print("a".."b".."c".."d")`, []string{"OP_CONCAT"}},
	{"CONCAT_int", `print("x="..42)`, []string{"OP_CONCAT"}},
	{"CONCAT_float", `print("x="..3.14)`, []string{"OP_CONCAT"}},
	{"CONCAT_empty", `print("".."".."")`	, nil},
	{"CONCAT_coerce_int", `print(10 ..20)`, nil},

	// --- OP_LEN ---
	{"LEN_string", `print(#"hello")`, []string{"OP_LEN"}},
	{"LEN_empty_string", `print(#"")`, []string{"OP_LEN"}},
	{"LEN_table_array", `print(#{1,2,3,4,5})`, []string{"OP_LEN"}},
	{"LEN_table_empty", `print(#{})`, []string{"OP_LEN"}},
	{"LEN_table_mixed", `local t={1,2,3,x=4}; print(#t)`, nil},
	{"LEN_utf8", `print(#"café")`, nil},

	// --- OP_VARARG (additional) ---
	{"VARARG_in_table", `local function f(...) return {...} end; local t=f(1,2,3); print(t[1],t[2],t[3])`, nil},
	{"VARARG_pass_through", `local function f(...) return ... end; print(f(1,2,3))`, nil},

	// --- OP_SELF ---
	{"SELF_method", `local t={x=10}; function t:getx() return self.x end; print(t:getx())`, []string{"OP_SELF"}},
	{"SELF_chain", `local t={x=10}; function t:add(n) self.x=self.x+n; return self end; t:add(5):add(3); print(t.x)`, nil},

	// --- OP_SETLIST (additional) ---
	{"SETLIST_large", `local t={1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20}; print(t[1],t[10],t[20])`, nil},
	{"SETLIST_vararg", `local function f(...) return {...} end; local t=f(1,2,3); print(#t)`, nil},

	// --- Type coercion edge cases ---
	{"CONCAT_bool_error", `print(pcall(function() return "x"..true end))`, nil},
	{"CONCAT_nil_error", `print(pcall(function() return "x"..nil end))`, nil},
	{"LEN_number_error", `print(pcall(function() return #42 end))`, nil},

	// --- Metamethods ---
	{"META_add", `local mt={__add=function(a,b) return 999 end}; local t=setmetatable({},mt); print(t+1)`, nil},
	{"META_concat", `local mt={__concat=function(a,b) return "custom" end}; local t=setmetatable({},mt); print(t.."x")`, nil},
	{"META_len", `local mt={__len=function() return 42 end}; local t=setmetatable({},mt); print(#t)`, nil},
	{"META_index", `local mt={__index=function(t,k) return k.."!" end}; local t=setmetatable({},mt); print(t.hello)`, nil},
	{"META_newindex", `local log=""; local mt={__newindex=function(t,k,v) rawset(t,k,v*2) end}; local t=setmetatable({},mt); t.x=5; print(t.x)`, nil},
	{"META_call", `local mt={__call=function(t,...) return "called" end}; local t=setmetatable({},mt); print(t())`, nil},
	{"META_eq", `local mt={__eq=function(a,b) return true end}; local a=setmetatable({},mt); local b=setmetatable({},mt); print(a==b)`, nil},
	{"META_lt", `local mt={__lt=function(a,b) return true end}; local a=setmetatable({},mt); local b=setmetatable({},mt); print(a<b)`, nil},
	{"META_le", `local mt={__le=function(a,b) return true end}; local a=setmetatable({},mt); local b=setmetatable({},mt); print(a<=b)`, nil},
	{"META_tostring", `local mt={__tostring=function() return "custom" end}; local t=setmetatable({},mt); print(tostring(t))`, nil},
	{"META_unm", `local mt={__unm=function(a) return 99 end}; local t=setmetatable({},mt); print(-t)`, nil},

	// --- pcall / xpcall ---
	{"PCALL_success", `print(pcall(function() return 42 end))`, nil},
	{"PCALL_error", `print(pcall(function() error("boom") end))`, nil},
	{"XPCALL_success", `print(xpcall(function() return 42 end, function(e) return "handled: "..e end))`, nil},
	{"XPCALL_error", `print(xpcall(function() error("boom") end, function(e) return "handled: "..e end))`, nil},

	// --- tostring / tonumber ---
	{"TOSTRING_int", `print(tostring(42))`, nil},
	{"TOSTRING_float", `print(tostring(3.14))`, nil},
	{"TOSTRING_bool", `print(tostring(true))`, nil},
	{"TOSTRING_nil", `print(tostring(nil))`, nil},
	{"TONUMBER_string", `print(tonumber("42"))`, nil},
	{"TONUMBER_float_str", `print(tonumber("3.14"))`, nil},
	{"TONUMBER_hex", `print(tonumber("0xff"))`, nil},
	{"TONUMBER_base", `print(tonumber("ff",16))`, nil},
	{"TONUMBER_fail", `print(tonumber("abc"))`, nil},

	// --- type() ---
	{"TYPE_nil", `print(type(nil))`, nil},
	{"TYPE_bool", `print(type(true))`, nil},
	{"TYPE_number_int", `print(type(42))`, nil},
	{"TYPE_number_float", `print(type(3.14))`, nil},
	{"TYPE_string", `print(type("hello"))`, nil},
	{"TYPE_table", `print(type({}))`, nil},
	{"TYPE_function", `print(type(print))`, nil},

	// --- String conversion in print ---
	{"PRINT_int_format", `print(42)`, nil},
	{"PRINT_float_format", `print(42.0)`, nil},
	{"PRINT_neg_zero", `print(-0.0)`, nil},
	{"PRINT_scientific", `print(1e10)`, nil},
	{"PRINT_small_float", `print(0.1)`, nil},
}

func TestBatch6ClosureMisc(t *testing.T) {
	match, mismatch := 0, 0
	for _, tc := range batch6ClosureMisc {
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
	t.Logf("Batch 6 Closure/Misc: %d match, %d mismatch out of %d", match, mismatch, len(batch6ClosureMisc))
}
