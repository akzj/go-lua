package bccompare

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/akzj/go-lua/internal/parse"
)

// stringReader implements lex.LexReader for test strings.
type stringReader struct {
	data string
	pos  int
}

func newStringReader(s string) *stringReader {
	return &stringReader{data: s}
}

func (r *stringReader) NextByte() int {
	if r.pos >= len(r.data) {
		return -1
	}
	b := r.data[r.pos]
	r.pos++
	return int(b)
}

// cLuaPath is the path to the reference C Lua 5.5.1 binary.
const cLuaPath = "/home/ubuntu/workspace/go-lua/lua-master/lua"

// disasmScript is the path to our C Lua disassembler script.
const disasmScript = "/home/ubuntu/workspace/go-lua/tools/disasm.lua"

// getCLuaDisasm compiles source with C Lua and returns disassembly text.
func getCLuaDisasm(t *testing.T, source string) string {
	t.Helper()
	cmd := exec.Command(cLuaPath, disasmScript, "-")
	cmd.Stdin = strings.NewReader(source)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("C Lua disasm failed: %v\n%s", err, string(out))
	}
	return string(out)
}

// getGoLuaDisasm compiles source with go-lua parser and returns disassembly text.
func getGoLuaDisasm(t *testing.T, source string) string {
	t.Helper()
	proto := parse.Parse("=input", newStringReader(source))
	return DumpProto(proto)
}

// normLines splits text into non-empty trimmed lines for comparison.
func normLines(s string) []string {
	raw := strings.Split(strings.TrimSpace(s), "\n")
	var out []string
	for _, line := range raw {
		line = strings.TrimRight(line, " \t\r")
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// diffLines returns a human-readable diff of two line slices.
func diffLines(a, b []string) string {
	var sb strings.Builder
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	diffs := 0
	for i := 0; i < maxLen; i++ {
		la, lb := "", ""
		if i < len(a) {
			la = a[i]
		}
		if i < len(b) {
			lb = b[i]
		}
		if la != lb {
			diffs++
			sb.WriteString("--- C:  ")
			sb.WriteString(la)
			sb.WriteString("\n+++ Go: ")
			sb.WriteString(lb)
			sb.WriteString("\n")
		}
	}
	if diffs == 0 {
		return ""
	}
	return sb.String()
}

var testCases = []struct {
	name   string
	source string
}{
	{
		name:   "basic_arith",
		source: "local a = 1 + 2; print(a)",
	},
	{
		name:   "string_concat",
		source: `local a, b = "hello", "world"; print(a..b)`,
	},
	{
		name:   "table_access",
		source: "local t = {1,2,3}; print(t[2])",
	},
	{
		name:   "function_call",
		source: "local function f(x) return x+1 end; print(f(5))",
	},
	{
		name:   "numeric_for",
		source: "for i=1,10 do end",
	},
	{
		name:   "generic_for",
		source: "local t={1,2,3}; for k,v in ipairs(t) do end",
	},
	{
		name:   "conditional",
		source: `local x=1; if x>0 then print("yes") else print("no") end`,
	},
	{
		name:   "closure_upvalue",
		source: "local function f(x) local function g() return x end; return g end",
	},
	{
		name:   "varargs",
		source: "local a,b,c = ...; print(a,b,c)",
	},
	{
		name:   "multi_return",
		source: "local function f() return 1,2,3 end; local a,b,c = f(); print(a,b,c)",
	},
	{
		name:   "global_const",
		source: "global <const> *; local x = 10; print(x)",
	},
	{
		name:   "while_loop",
		source: "local i = 0; while i < 10 do i = i + 1 end; print(i)",
	},
	{
		name:   "repeat_until",
		source: "local i = 0; repeat i = i + 1 until i >= 10; print(i)",
	},
	{
		name:   "nested_if",
		source: `local x = 5; if x > 10 then print("big") elseif x > 0 then print("pos") else print("neg") end`,
	},
	{
		name:   "method_call",
		source: `local t = {}; function t:foo(x) return self, x end; t:foo(1)`,
	},
	{
		name:   "multi_assign",
		source: "local a, b, c = 1, 2, 3; a, b, c = c, b, a; print(a, b, c)",
	},
	{
		name:   "table_constructor_mixed",
		source: `local t = {1, 2, x=3, ["y"]=4, [10]=5}; print(t.x)`,
	},
	{
		name:   "logical_operators",
		source: "local a, b = true, false; local c = a and b or a; print(c)",
	},
	{
		name:   "nested_closures",
		source: "local function f(a) return function(b) return function(c) return a+b+c end end end",
	},
	{
		name:   "string_arith_ops",
		source: `local a = 10; local b = a + 1; local c = a - 1; local d = a * 2; local e = a / 3; local f = a % 3; local g = a ^ 2; local h = a // 3`,
	},
	{
		name:   "comparison_ops",
		source: `local a, b = 1, 2; local c = a == b; local d = a ~= b; local e = a < b; local f = a <= b; local g = a > b; local h = a >= b`,
	},
	{
		name:   "bitwise_ops",
		source: "local a, b = 0xFF, 0x0F; local c = a & b; local d = a | b; local e = a ~ b; local f = ~a; local g = a << 2; local h = a >> 2",
	},
	{
		name:   "string_length",
		source: `local s = "hello"; local n = #s; print(n)`,
	},
	{
		name:   "tbc_variable",
		source: "local x <close> = setmetatable({}, {__close = function() end})",
	},
}

func TestBytecodeComparison(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cOut := getCLuaDisasm(t, tc.source)
			goOut := getGoLuaDisasm(t, tc.source)

			cLines := normLines(cOut)
			goLines := normLines(goOut)

			diff := diffLines(cLines, goLines)
			if diff != "" {
				t.Errorf("Bytecode mismatch for %q:\n%s\n\n--- Full C Lua output ---\n%s\n--- Full Go output ---\n%s",
					tc.name, diff, cOut, goOut)
			} else {
				t.Logf("✅ %s: %d lines match", tc.name, len(cLines))
			}
		})
	}
}

// TestBytecodeComparisonVerbose prints full output for manual inspection.
func TestBytecodeComparisonVerbose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping verbose output in short mode")
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cOut := getCLuaDisasm(t, tc.source)
			goOut := getGoLuaDisasm(t, tc.source)
			t.Logf("=== C Lua ===\n%s", cOut)
			t.Logf("=== Go Lua ===\n%s", goOut)
		})
	}
}