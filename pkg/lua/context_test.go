package lua_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/akzj/go-lua/pkg/lua"
)

func TestSetContext_Timeout(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	L.SetContext(ctx)

	start := time.Now()
	err := L.DoString(`while true do end`)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from infinite loop with timeout context")
	}
	if !strings.Contains(err.Error(), "context cancelled") {
		t.Fatalf("expected 'context cancelled' error, got: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("timeout took too long: %v (expected ~100ms)", elapsed)
	}
	t.Logf("Context timeout fired after %v, error: %v", elapsed, err)
}

func TestSetContext_Cancel(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	ctx, cancel := context.WithCancel(context.Background())
	L.SetContext(ctx)

	// Cancel after 100ms from a goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := L.DoString(`while true do end`)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from infinite loop with cancelled context")
	}
	if !strings.Contains(err.Error(), "context cancelled") {
		t.Fatalf("expected 'context cancelled' error, got: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("cancellation took too long: %v (expected ~100ms)", elapsed)
	}
	t.Logf("Context cancel fired after %v", elapsed)
}

func TestSetContext_Nil(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// SetContext(nil) should not panic and normal execution should work
	L.SetContext(nil)
	err := L.DoString(`return 1 + 2`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Context() should return background when nil
	ctx := L.Context()
	if ctx == nil {
		t.Fatal("Context() returned nil, expected context.Background()")
	}
}

func TestSetContext_NormalExecution(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Context with long timeout should not interfere with normal execution
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	L.SetContext(ctx)

	err := L.DoString(`
		local sum = 0
		for i = 1, 1000 do
			sum = sum + i
		end
		return sum
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	n, ok := L.ToInteger(-1)
	if !ok || n != 500500 {
		t.Fatalf("expected 500500, got %d (ok=%v)", n, ok)
	}
}

func TestSetContext_WithCPULimit(t *testing.T) {
	// Test that context and CPU limit coexist: whichever fires first wins.

	t.Run("CPU fires first", func(t *testing.T) {
		L := lua.NewState()
		defer L.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		L.SetContext(ctx)
		L.SetCPULimit(10000) // very low CPU limit

		err := L.DoString(`while true do end`)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "CPU limit exceeded") {
			t.Fatalf("expected CPU limit error, got: %v", err)
		}
	})

	t.Run("Context fires first", func(t *testing.T) {
		L := lua.NewState()
		defer L.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		L.SetContext(ctx)
		L.SetCPULimit(1_000_000_000) // very high CPU limit

		err := L.DoString(`while true do end`)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "context cancelled") {
			t.Fatalf("expected context cancelled error, got: %v", err)
		}
	})
}

func TestSetContext_CatchableByPcall(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	L.SetContext(ctx)

	// pcall should catch the context cancellation error
	err := L.DoString(`
		local ok, msg = pcall(function()
			while true do end
		end)
		assert(not ok)
		assert(type(msg) == "string")
		assert(msg:find("context cancelled"))
	`)
	if err != nil {
		t.Fatalf("pcall should catch context error, got outer error: %v", err)
	}
}

func TestSetContext_RemoveContext(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Set a context, then remove it
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	L.SetContext(ctx)

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	// Remove context
	L.SetContext(nil)

	// Should execute normally now (no context check)
	err := L.DoString(`return 42`)
	if err != nil {
		t.Fatalf("unexpected error after removing context: %v", err)
	}
}
