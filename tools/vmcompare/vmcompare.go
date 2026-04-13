// Package vmcompare provides systematic VM opcode verification by comparing
// go-lua execution output against C Lua 5.5.1 reference output.
//
// For each opcode, a minimal Lua snippet is run on both VMs and stdout is compared.
package vmcompare

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
	stdlibapi "github.com/akzj/go-lua/internal/stdlib/api"
)

// cLuaPath is the reference C Lua 5.5.1 binary.
const cLuaPath = "/home/ubuntu/workspace/go-lua/lua-master/lua"

// vmTest defines a single opcode verification test case.
type vmTest struct {
	Name string   // e.g. "ADD_int_int"
	Code string   // Lua source code using print() for output
	Ops  []string // primary opcodes exercised (documentation only)
}

// RunCLua executes Lua code with C Lua and returns stdout.
func RunCLua(t *testing.T, code string) string {
	t.Helper()
	cmd := exec.Command(cLuaPath, "-e", code)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		t.Fatalf("C Lua failed: %v\nstderr: %s\ncode: %s", err, stderr.String(), code)
	}
	return stdout.String()
}

// RunGoLua executes Lua code with go-lua and returns captured print output.
// Replaces the global print with a buffer-capturing version.
func RunGoLua(t *testing.T, code string) string {
	t.Helper()
	L := luaapi.NewState()
	stdlibapi.OpenAll(L)

	// Replace print with a buffer-capturing version
	var buf bytes.Buffer
	capturePrint := func(LL *luaapi.State) int {
		n := LL.GetTop()
		for i := 1; i <= n; i++ {
			if i > 1 {
				buf.WriteByte('\t')
			}
			s := LL.TolString(i)
			buf.WriteString(s)
			LL.Pop(1) // TolString pushes a string; pop it
		}
		buf.WriteByte('\n')
		return 0
	}
	L.PushCFunction(capturePrint)
	L.SetGlobal("print")

	// Use Load + PCall with source "=(command line)" to match C Lua's -e flag
	status := L.Load(code, "=(command line)", "t")
	if status != 0 {
		msg, _ := L.ToString(-1)
		t.Fatalf("go-lua load failed: %s\ncode: %s", msg, code)
	}
	status = L.PCall(0, -1, 0)
	if status != 0 {
		msg, _ := L.ToString(-1)
		t.Fatalf("go-lua failed: %s\ncode: %s", msg, code)
	}
	return buf.String()
}

// Normalize output for comparison: trim trailing whitespace per line,
// normalize NaN/inf representations, trim trailing newline.
func Normalize(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimRight(line, " \t\r")
		// Normalize NaN representations
		line = strings.ReplaceAll(line, "-nan", "nan")
		line = strings.ReplaceAll(line, "NaN", "nan")
		line = strings.ReplaceAll(line, "-nan(ind)", "nan")
		// Normalize inf representations  
		line = strings.ReplaceAll(line, "+Inf", "inf")
		line = strings.ReplaceAll(line, "+inf", "inf")
		line = strings.ReplaceAll(line, "Inf", "inf")
		out = append(out, line)
	}
	result := strings.Join(out, "\n")
	return strings.TrimRight(result, "\n")
}

// CompareVM runs a test case on both VMs and compares normalized output.
// Returns (match bool, cOutput string, goOutput string).
func CompareVM(t *testing.T, tc vmTest) (bool, string, string) {
	t.Helper()
	cOut := Normalize(RunCLua(t, tc.Code))
	goOut := Normalize(RunGoLua(t, tc.Code))
	return cOut == goOut, cOut, goOut
}
