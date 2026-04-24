package lua_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

// ---------------------------------------------------------------------------
// CPU Limit Tests
// ---------------------------------------------------------------------------

func TestCPULimit_InfiniteLoop(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.SetCPULimit(100_000)

	err := L.DoString(`while true do end`)
	if err == nil {
		t.Fatal("expected CPU limit error, got nil")
	}
	if !strings.Contains(err.Error(), "CPU limit exceeded") {
		t.Fatalf("expected 'CPU limit exceeded' in error, got: %v", err)
	}
	t.Logf("Got expected error: %v", err)
}

func TestCPULimit_ShortCodeDoesNotTrigger(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.SetCPULimit(1_000_000)

	err := L.DoString(`local sum = 0; for i = 1, 100 do sum = sum + i end`)
	if err != nil {
		t.Fatalf("short code should not hit CPU limit: %v", err)
	}
}

func TestCPULimit_ResetCounter(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.SetCPULimit(1_000_000)

	// First run — use enough iterations to trigger the hook (>1000 instructions).
	err := L.DoString(`local sum = 0; for i = 1, 5000 do sum = sum + i end`)
	if err != nil {
		t.Fatalf("first run should not hit limit: %v", err)
	}

	used := L.CPUInstructionsUsed()
	if used <= 0 {
		t.Fatalf("expected positive instructions used, got %d", used)
	}
	t.Logf("Instructions used after first run: %d", used)

	// Reset and run again — counter should start from 0.
	L.ResetCPUCounter()
	if L.CPUInstructionsUsed() != 0 {
		t.Fatalf("expected 0 after reset, got %d", L.CPUInstructionsUsed())
	}

	err = L.DoString(`local sum = 0; for i = 1, 5000 do sum = sum + i end`)
	if err != nil {
		t.Fatalf("second run should not hit limit: %v", err)
	}

	used2 := L.CPUInstructionsUsed()
	if used2 <= 0 {
		t.Fatalf("expected positive instructions after second run, got %d", used2)
	}
	t.Logf("Instructions used after second run: %d", used2)
}

func TestCPULimit_RemoveLimit(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Set a very tight limit.
	L.SetCPULimit(1000)

	err := L.DoString(`for i = 1, 10000 do end`)
	if err == nil {
		t.Fatal("expected CPU limit error with tight limit")
	}

	// Remove the limit.
	L.SetCPULimit(0)

	// Now a big loop should work fine.
	err = L.DoString(`for i = 1, 10000 do end`)
	if err != nil {
		t.Fatalf("after removing limit, loop should work: %v", err)
	}
}

func TestCPULimit_CPUInstructionsUsed(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.SetCPULimit(1_000_000)

	err := L.DoString(`for i = 1, 5000 do end`)
	if err != nil {
		t.Fatal(err)
	}

	used := L.CPUInstructionsUsed()
	// The counter is approximate (increments in chunks of checkInterval).
	// A for-loop of 5000 iterations should use more than 5000 instructions
	// (each iteration has at least a few ops).
	if used < 1000 {
		t.Fatalf("expected at least 1000 instructions used, got %d", used)
	}
	t.Logf("Instructions used for 5000-iteration loop: %d", used)
}

func TestCPULimit_CatchableByPcall(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.SetCPULimit(100_000)

	// pcall should catch the CPU limit error.
	err := L.DoString(`
		local ok, msg = pcall(function()
			while true do end
		end)
		assert(not ok)
		assert(type(msg) == "string")
	`)
	if err != nil {
		t.Fatalf("pcall should catch CPU limit error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NewSandboxState Tests
// ---------------------------------------------------------------------------

func TestNewSandboxState_BasicLua(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{
		CPULimit: 1_000_000,
	})
	defer L.Close()

	err := L.DoString(`local x = 1 + 2; assert(x == 3)`)
	if err != nil {
		t.Fatalf("basic lua should work: %v", err)
	}
}

func TestNewSandboxState_StringLib(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{})
	defer L.Close()

	err := L.DoString(`assert(string.upper("hello") == "HELLO")`)
	if err != nil {
		t.Fatalf("string lib should work: %v", err)
	}
}

func TestNewSandboxState_MathLib(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{})
	defer L.Close()

	err := L.DoString(`assert(math.abs(-42) == 42)`)
	if err != nil {
		t.Fatalf("math lib should work: %v", err)
	}
}

func TestNewSandboxState_TableLib(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{})
	defer L.Close()

	err := L.DoString(`
		local t = {3, 1, 2}
		table.sort(t)
		assert(t[1] == 1 and t[2] == 2 and t[3] == 3)
	`)
	if err != nil {
		t.Fatalf("table lib should work: %v", err)
	}
}

func TestNewSandboxState_CoroutineLib(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{})
	defer L.Close()

	err := L.DoString(`
		local co = coroutine.create(function() return 42 end)
		local ok, val = coroutine.resume(co)
		assert(ok and val == 42)
	`)
	if err != nil {
		t.Fatalf("coroutine lib should work: %v", err)
	}
}

func TestNewSandboxState_IOBlocked(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{})
	defer L.Close()

	err := L.DoString(`io.open("test.txt")`)
	if err == nil {
		t.Fatal("io should not be available in sandbox")
	}
}

func TestNewSandboxState_OSBlocked(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{})
	defer L.Close()

	err := L.DoString(`os.execute("ls")`)
	if err == nil {
		t.Fatal("os should not be available in sandbox")
	}
}

func TestNewSandboxState_DebugBlocked(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{})
	defer L.Close()

	err := L.DoString(`debug.getinfo(1)`)
	if err == nil {
		t.Fatal("debug should not be available in sandbox")
	}
}

func TestNewSandboxState_DofileBlocked(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{})
	defer L.Close()

	err := L.DoString(`dofile("test.lua")`)
	if err == nil {
		t.Fatal("dofile should not be available in sandbox")
	}
}

func TestNewSandboxState_LoadfileBlocked(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{})
	defer L.Close()

	err := L.DoString(`loadfile("test.lua")`)
	if err == nil {
		t.Fatal("loadfile should not be available in sandbox")
	}
}

func TestNewSandboxState_LoadBlocked(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{})
	defer L.Close()

	err := L.DoString(`load("return 1")`)
	if err == nil {
		t.Fatal("load should not be available in sandbox")
	}
}

func TestNewSandboxState_RequireBlocked(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{})
	defer L.Close()

	err := L.DoString(`require("os")`)
	if err == nil {
		t.Fatal("require should not be available in sandbox")
	}
}

func TestNewSandboxState_AllowIO(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{
		AllowIO: true,
	})
	defer L.Close()

	// io table should exist (we don't actually open a file, just check the table).
	err := L.DoString(`assert(type(io) == "table")`)
	if err != nil {
		t.Fatalf("io should be available when AllowIO=true: %v", err)
	}

	err = L.DoString(`assert(type(os) == "table")`)
	if err != nil {
		t.Fatalf("os should be available when AllowIO=true: %v", err)
	}
}

func TestNewSandboxState_AllowDebug(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{
		AllowDebug: true,
	})
	defer L.Close()

	err := L.DoString(`assert(type(debug) == "table")`)
	if err != nil {
		t.Fatalf("debug should be available when AllowDebug=true: %v", err)
	}
}

func TestNewSandboxState_AllowPackage(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{
		AllowPackage: true,
	})
	defer L.Close()

	err := L.DoString(`assert(type(package) == "table")`)
	if err != nil {
		t.Fatalf("package should be available when AllowPackage=true: %v", err)
	}
}

func TestNewSandboxState_CPULimitApplied(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{
		CPULimit: 100_000,
	})
	defer L.Close()

	err := L.DoString(`while true do end`)
	if err == nil {
		t.Fatal("expected CPU limit error in sandbox")
	}
	if !strings.Contains(err.Error(), "CPU limit exceeded") {
		t.Fatalf("expected 'CPU limit exceeded' in error, got: %v", err)
	}
}

func TestNewSandboxState_MemoryLimitApplied(t *testing.T) {
	L := lua.NewSandboxState(lua.SandboxConfig{
		MemoryLimit: 256 * 1024, // 256 KB — very tight
		CPULimit:    10_000_000, // generous CPU to avoid CPU limit interference
	})
	defer L.Close()

	// Try to allocate a huge string — should hit memory limit.
	err := L.DoString(`
		local t = {}
		for i = 1, 1000000 do
			t[i] = string.rep("x", 1000)
		end
	`)
	if err == nil {
		t.Fatal("expected memory limit error in sandbox")
	}
	t.Logf("Got expected memory error: %v", err)
}

func TestNewSandboxState_ExtraLibs(t *testing.T) {
	myLib := func(L *lua.State) int {
		L.PushString("hello from custom lib")
		return 1
	}

	L := lua.NewSandboxState(lua.SandboxConfig{
		ExtraLibs: map[string]lua.Function{
			"mylib": func(L *lua.State) int {
				L.NewLib(map[string]lua.Function{
					"greet": myLib,
				})
				return 1
			},
		},
	})
	defer L.Close()

	err := L.DoString(`assert(mylib.greet() == "hello from custom lib")`)
	if err != nil {
		t.Fatalf("extra lib should work: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Runnable Examples (godoc)
// ---------------------------------------------------------------------------

func ExampleNewSandboxState() {
	L := lua.NewSandboxState(lua.SandboxConfig{
		MemoryLimit: 10 * 1024 * 1024, // 10MB
		CPULimit:    100_000,           // 100K instructions
	})
	defer L.Close()

	// Safe to execute untrusted code — io/os/debug are blocked.
	err := L.DoString(`print("Hello from sandbox!")`)
	if err != nil {
		fmt.Println("Error:", err)
	}

	// Dangerous functions are removed.
	err = L.DoString(`io.open("secret.txt")`)
	if err != nil {
		fmt.Println("io blocked:", err != nil)
	}
	// Output:
	// Hello from sandbox!
	// io blocked: true
}
