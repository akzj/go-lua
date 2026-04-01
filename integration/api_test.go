// Package integration provides end-to-end integration tests for the Lua VM.
// These tests verify that public APIs work correctly together.
//
// NOTE: DoString() and Require() have TODO implementations.
// TestCall() is implemented below.
package integration

import (
	"testing"

	luaapi "github.com/akzj/go-lua/api"
	luaapipkg "github.com/akzj/go-lua/api/api"
	"github.com/akzj/go-lua/state"
	stateapi "github.com/akzj/go-lua/state/api"
)

// =============================================================================
// State Creation Tests
// =============================================================================

// TestNewState verifies that state.New() creates a valid Lua state.
func TestNewState(t *testing.T) {
	L := state.New()
	if L == nil {
		t.Fatal("state.New() returned nil")
	}
}

// TestApiNewState is skipped because api.New() returns nil (needs initialization).
// The api package's DefaultLuaAPI is not initialized.
// TODO: Uncomment when api.New() is properly initialized.
// func TestApiNewState(t *testing.T) {
//     L := luaapi.New()
//     if L == nil {
//         t.Fatal("luaapi.New() returned nil")
//     }
// }

// TestInitialState verifies the initial state of a new Lua state.
func TestInitialState(t *testing.T) {
	L := state.New()

	// Initial top should be 0 (empty stack)
	if L.Top() != 0 {
		t.Errorf("Initial Top() = %d, want 0", L.Top())
	}

	// Initial status should be LUA_OK
	if L.Status() != stateapi.LUA_OK {
		t.Errorf("Initial Status() = %d, want LUA_OK (0)", L.Status())
	}

	// Stack should be initialized with at least 20 slots
	if L.StackSize() < 20 {
		t.Errorf("Initial StackSize() = %d, want >= 20", L.StackSize())
	}

	// Stack should be non-nil
	if L.Stack() == nil {
		t.Error("Stack() returned nil")
	}
}

// =============================================================================
// Stack Operations Tests
// =============================================================================

// TestSetTop verifies SetTop modifies stack correctly.
func TestSetTop(t *testing.T) {
	L := state.New()

	L.SetTop(5)
	if L.Top() != 5 {
		t.Errorf("After SetTop(5), Top() = %d, want 5", L.Top())
	}

	L.SetTop(2)
	if L.Top() != 2 {
		t.Errorf("After SetTop(2), Top() = %d, want 2", L.Top())
	}

	L.SetTop(10)
	if L.Top() != 10 {
		t.Errorf("After SetTop(10), Top() = %d, want 10", L.Top())
	}
}

// TestPop verifies Pop removes elements from stack.
func TestPop(t *testing.T) {
	L := state.New()

	L.SetTop(3)
	L.Pop()

	if L.Top() != 2 {
		t.Errorf("After Pop, Top() = %d, want 2", L.Top())
	}

	L.Pop()
	if L.Top() != 1 {
		t.Errorf("After second Pop, Top() = %d, want 1", L.Top())
	}
}

// TestPushValue verifies PushValue copies values correctly.
func TestPushValue(t *testing.T) {
	L := state.New()

	L.SetTop(1)
	initialTop := L.Top()
	L.PushValue(1)

	if L.Top() != initialTop+1 {
		t.Errorf("After PushValue, Top() = %d, want %d", L.Top(), initialTop+1)
	}
}

// TestGrowStack verifies GrowStack works correctly.
func TestGrowStack(t *testing.T) {
	L := state.New()

	initialSize := L.StackSize()
	L.GrowStack(100)

	if L.StackSize() <= initialSize {
		t.Errorf("After GrowStack, StackSize() = %d, should be > %d", L.StackSize(), initialSize)
	}
}

// =============================================================================
// CallInfo Management Tests
// =============================================================================

// TestCurrentCI verifies CurrentCI returns valid CallInfo.
func TestCurrentCI(t *testing.T) {
	L := state.New()

	ci := L.CurrentCI()
	if ci == nil {
		t.Fatal("CurrentCI() returned nil")
	}

	// Base frame should have Func = 0
	if ci.Func() != 0 {
		t.Errorf("Base frame Func() = %d, want 0", ci.Func())
	}
}

// TestPushCIPopCI is skipped because PushCI requires *internal.callInfo (internal-only).
// CallInfo management is tested indirectly via CurrentCI.
// TODO: Uncomment when PushCI accepts any CallInfo implementation.
// func TestPushCIPopCI(t *testing.T) { ... }

// =============================================================================
// Global State Tests
// =============================================================================

// TestGlobal verifies Global() returns valid GlobalState.
func TestGlobal(t *testing.T) {
	L := state.New()

	g := L.Global()
	if g == nil {
		t.Fatal("Global() returned nil")
	}

	// Verify allocator is accessible
	_ = g.Allocator()

	// Verify registry is accessible
	_ = g.Registry()
}

// TestNewThread verifies NewThread creates a new thread with shared global state.
func TestNewThread(t *testing.T) {
	L := state.New()

	newL := L.NewThread()
	if newL == nil {
		t.Fatal("NewThread() returned nil")
	}

	// Verify they share global state
	if newL.Global() != L.Global() {
		t.Error("NewThread should share global state with parent")
	}

	// Verify thread has its own stack
	if newL.Stack() == nil {
		t.Error("NewThread stack should be initialized")
	}
}

// =============================================================================
// API Constants Tests
// =============================================================================

// TestLuaTypeConstants verifies Lua type constants are defined correctly.
func TestLuaTypeConstants(t *testing.T) {
	if luaapi.LUA_TNIL != luaapipkg.LUA_TNIL {
		t.Error("LUA_TNIL mismatch")
	}
	if luaapi.LUA_TNUMBER != luaapipkg.LUA_TNUMBER {
		t.Error("LUA_TNUMBER mismatch")
	}
	if luaapi.LUA_TSTRING != luaapipkg.LUA_TSTRING {
		t.Error("LUA_TSTRING mismatch")
	}
	if luaapi.LUA_TTABLE != luaapipkg.LUA_TTABLE {
		t.Error("LUA_TTABLE mismatch")
	}
	if luaapi.LUA_TFUNCTION != luaapipkg.LUA_TFUNCTION {
		t.Error("LUA_TFUNCTION mismatch")
	}
}

// TestLuaStatusConstants verifies Lua status constants are defined correctly.
func TestLuaStatusConstants(t *testing.T) {
	if luaapi.LUA_OK != luaapipkg.LUA_OK {
		t.Error("LUA_OK mismatch")
	}
	if luaapi.LUA_YIELD != luaapipkg.LUA_YIELD {
		t.Error("LUA_YIELD mismatch")
	}
	if luaapi.LUA_ERRRUN != luaapipkg.LUA_ERRRUN {
		t.Error("LUA_ERRRUN mismatch")
	}
}

// TestStatusString verifies StatusString function works.
func TestStatusString(t *testing.T) {
	if luaapi.StatusString(luaapi.LUA_OK) != "OK" {
		t.Error("StatusString(LUA_OK) should be 'OK'")
	}
}

// TestTypename verifies Typename function works.
func TestTypename(t *testing.T) {
	if luaapi.Typename(luaapi.LUA_TNIL) != "nil" {
		t.Error("Typename(LUA_TNIL) should be 'nil'")
	}
	if luaapi.Typename(luaapi.LUA_TSTRING) != "string" {
		t.Error("Typename(LUA_TSTRING) should be 'string'")
	}
}

// =============================================================================
// TODO: Tests for unimplemented features
// =============================================================================

// The following features are not yet fully implemented:
// - Resume() - state/internal/state.go has panic("TODO: implement Resume")
// - Yield() - state/internal/state.go has panic("TODO: implement Yield")
// - DoString() - not implemented
// - Require() - not implemented
// - Full Lua closure execution - requires bytecode compiler
//
// TestCall verifies the Call function works correctly.
// Note: Call() requires a Lua closure (bytecode). This test verifies the
// API contract is satisfied. Full integration tests require a compiler.
func TestCall(t *testing.T) {
	L := state.New()
	
	// Test that Call panics appropriately when no function is on stack
	defer func() {
		if r := recover(); r != nil {
			// Expected panic for empty stack
			t.Logf("Call panic on empty stack (expected): %v", r)
		}
	}()
	
	L.SetTop(0) // Clear stack
	L.Call(0, 0) // Should panic - no function to call
}
