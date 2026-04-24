package lua_test

import (
	"strings"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

func TestMemoryLimit(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Set a 1MB limit
	L.SetMemoryLimit(1 * 1024 * 1024)

	// This should succeed (small allocation)
	err := L.DoString(`local t = {}; for i = 1, 100 do t[i] = i end`)
	if err != nil {
		t.Fatalf("small allocation should succeed: %v", err)
	}

	// This should fail (try to allocate way more than 1MB)
	err = L.DoString(`
		local t = {}
		for i = 1, 1000000 do
			t[i] = string.rep("x", 100)
		end
	`)
	if err == nil {
		t.Fatal("expected out of memory error")
	}
	// Error message should mention memory
	if !strings.Contains(err.Error(), "memory") {
		t.Errorf("expected error to mention memory, got: %v", err)
	}
	t.Logf("Got expected error: %v", err)
}

func TestMemoryLimitZeroMeansNoLimit(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.SetMemoryLimit(0) // no limit

	err := L.DoString(`local t = {}; for i = 1, 10000 do t[i] = string.rep("x", 100) end`)
	if err != nil {
		t.Fatalf("no limit should not fail: %v", err)
	}
}

func TestMemoryUsed(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	before := L.MemoryUsed()
	err := L.DoString(`local t = {}; for i = 1, 1000 do t[i] = string.rep("x", 1000) end`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := L.MemoryUsed()

	if after <= before {
		t.Errorf("expected memory usage to increase: before=%d, after=%d", before, after)
	}
	t.Logf("Memory: before=%d, after=%d, delta=%d", before, after, after-before)
}

func TestMemoryLimitRecovery(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Set a small limit
	L.SetMemoryLimit(512 * 1024) // 512KB

	// Blow the limit with pcall — should get caught
	err := L.DoString(`
		local ok, msg = pcall(function()
			local t = {}
			for i = 1, 1000000 do
				t[i] = string.rep("x", 100)
			end
		end)
		if ok then
			error("expected pcall to fail")
		end
		-- After pcall catches the error, state should still be usable
		result = 42
	`)
	if err != nil {
		t.Fatalf("pcall should catch memory error: %v", err)
	}

	// Verify state is still usable
	L.GetGlobal("result")
	if !L.IsNumber(-1) {
		t.Fatal("expected result to be a number")
	}
	val, _ := L.ToNumber(-1)
	if val != 42 {
		t.Errorf("expected 42, got %v", val)
	}
	L.Pop(1)
}

func TestSetMemoryLimitReturnsPrevious(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Initially 0 (no limit)
	prev := L.SetMemoryLimit(1000)
	if prev != 0 {
		t.Errorf("expected initial limit 0, got %d", prev)
	}

	// Set again, should return previous
	prev = L.SetMemoryLimit(2000)
	if prev != 1000 {
		t.Errorf("expected previous limit 1000, got %d", prev)
	}

	// Clear limit
	prev = L.SetMemoryLimit(0)
	if prev != 2000 {
		t.Errorf("expected previous limit 2000, got %d", prev)
	}
}
