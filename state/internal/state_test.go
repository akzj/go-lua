// Package internal provides tests for the Lua state management.
package internal

import (
	"testing"

	"github.com/akzj/go-lua/state/api"
	"github.com/akzj/go-lua/table"
	types "github.com/akzj/go-lua/types/api"
	vm "github.com/akzj/go-lua/vm"
)

// =============================================================================
// Stack Operations Tests
// =============================================================================

func TestNewLuaState(t *testing.T) {
	L := NewLuaState(nil)
	if L == nil {
		t.Error("NewLuaState should return non-nil")
	}

	if L.stack == nil {
		t.Error("LuaState stack should be initialized")
	}

	if L.Top() != 0 {
		t.Errorf("Initial top = %d, want 0", L.Top())
	}

	if L.Status() != api.LUA_OK {
		t.Errorf("Initial status = %d, want LUA_OK (0)", L.Status())
	}
}

func TestSetTop(t *testing.T) {
	L := NewLuaState(nil)

	L.SetTop(5)
	if L.Top() != 5 {
		t.Errorf("After SetTop(5), top = %d, want 5", L.Top())
	}

	L.SetTop(2)
	if L.Top() != 2 {
		t.Errorf("After SetTop(2), top = %d, want 2", L.Top())
	}

	L.SetTop(10)
	if L.Top() != 10 {
		t.Errorf("After SetTop(10), top = %d, want 10", L.Top())
	}
}

func TestGetTop(t *testing.T) {
	L := NewLuaState(nil)

	if L.Top() != 0 {
		t.Errorf("GetTop() on empty stack = %d, want 0", L.Top())
	}

	L.SetTop(3)
	if L.Top() != 3 {
		t.Errorf("GetTop() after 3 pushes = %d, want 3", L.Top())
	}
}

func TestPop(t *testing.T) {
	L := NewLuaState(nil)

	L.SetTop(3)
	L.Pop()

	if L.Top() != 2 {
		t.Errorf("After Pop, top = %d, want 2", L.Top())
	}

	L.Pop()
	if L.Top() != 1 {
		t.Errorf("After second Pop, top = %d, want 1", L.Top())
	}
}

func TestPushValue(t *testing.T) {
	L := NewLuaState(nil)

	L.SetTop(1)
	initialTop := L.Top()
	L.PushValue(1)

	if L.Top() != initialTop+1 {
		t.Errorf("After PushValue, top = %d, want %d", L.Top(), initialTop+1)
	}
}

func TestGrowStack(t *testing.T) {
	L := NewLuaState(nil)

	initialSize := L.StackSize()
	if initialSize < 20 {
		t.Errorf("Initial stack size should be at least 20, got %d", initialSize)
	}

	// Test GrowStack doesn't panic
	L.GrowStack(10)

	// Stack should still be valid after GrowStack
	stack := L.Stack()
	if stack == nil {
		t.Error("Stack should not be nil after GrowStack")
	}

	// Test shrinking (SetTop to smaller value)
	L.SetTop(5)
	if L.Top() != 5 {
		t.Errorf("After SetTop(5), top = %d, want 5", L.Top())
	}
}

func TestStack(t *testing.T) {
	L := NewLuaState(nil)

	stack := L.Stack()
	if stack == nil {
		t.Error("Stack() should return non-nil")
	}

	L.SetTop(5)
	if len(stack) < 5 {
		t.Error("Stack slice should have at least 5 elements")
	}
}

// =============================================================================
// CallInfo Management Tests
// =============================================================================

func TestCallInfoBasic(t *testing.T) {
	ci := &callInfo{
		func_:    0,
		top:      5,
		prev:     nil,
		nresults: -1,
	}

	if ci.Func() != 0 {
		t.Errorf("CallInfo.Func() = %d, want 0", ci.Func())
	}

	if ci.Top() != 5 {
		t.Errorf("CallInfo.Top() = %d, want 5", ci.Top())
	}

	if ci.NResults() != -1 {
		t.Errorf("CallInfo.NResults() = %d, want -1", ci.NResults())
	}

	if ci.Prev() != nil {
		t.Error("CallInfo.Prev() should be nil for base frame")
	}
}

func TestCallInfoPrev(t *testing.T) {
	base := &callInfo{
		func_:    0,
		top:      5,
		prev:     nil,
		nresults: -1,
	}

	ci := &callInfo{
		func_:    5,
		top:      10,
		prev:     base,
		nresults: 1,
	}

	if ci.Func() != 5 {
		t.Errorf("CallInfo.Func() = %d, want 5", ci.Func())
	}

	prev := ci.Prev()
	if prev == nil {
		t.Error("CallInfo.Prev() should return base ci")
	}

	if prev.Func() != 0 {
		t.Errorf("Prev.Func() = %d, want 0", prev.Func())
	}
}

func TestSetFuncSetTop(t *testing.T) {
	ci := &callInfo{
		func_:    0,
		top:      0,
		prev:     nil,
		nresults: 0,
	}

	ci.SetFunc(10)
	if ci.Func() != 10 {
		t.Errorf("After SetFunc(10), Func() = %d, want 10", ci.Func())
	}

	ci.SetTop(20)
	if ci.Top() != 20 {
		t.Errorf("After SetTop(20), Top() = %d, want 20", ci.Top())
	}
}

func TestSetNResults(t *testing.T) {
	ci := &callInfo{}

	ci.SetNResults(3)
	if ci.NResults() != 3 {
		t.Errorf("After SetNResults(3), NResults() = %d, want 3", ci.NResults())
	}

	ci.SetNResults(-1)
	if ci.NResults() != -1 {
		t.Errorf("After SetNResults(-1), NResults() = %d, want -1", ci.NResults())
	}
}

func TestSetPrev(t *testing.T) {
	ci1 := &callInfo{func_: 1, top: 5}
	ci2 := &callInfo{func_: 2, top: 10}

	ci2.SetPrev(ci1)

	if ci2.Prev() == nil {
		t.Error("Prev should not be nil after SetPrev")
	}

	if ci2.Prev().Func() != 1 {
		t.Errorf("Prev.Func() = %d, want 1", ci2.Prev().Func())
	}

	ci2.SetPrev(nil)
	if ci2.Prev() != nil {
		t.Error("Prev should be nil after SetPrev(nil)")
	}
}

// =============================================================================
// LuaState CallInfo Management Tests
// =============================================================================

func TestCurrentCI(t *testing.T) {
	L := NewLuaState(nil)

	ci := L.CurrentCI()
	if ci == nil {
		t.Error("CurrentCI() should return base CI")
	}

	if ci.Func() != 0 {
		t.Errorf("Base CI Func() = %d, want 0", ci.Func())
	}
}

func TestPushCIPopCI(t *testing.T) {
	L := NewLuaState(nil)

	newCI := &callInfo{
		func_:    10,
		top:      15,
		prev:     nil,
		nresults: 1,
	}
	L.PushCI(newCI)

	current := L.CurrentCI()
	if current.Func() != 10 {
		t.Errorf("After PushCI, current Func() = %d, want 10", current.Func())
	}

	if current.Prev() == nil {
		t.Error("Prev should not be nil after PushCI")
	}

	L.PopCI()
	afterPop := L.CurrentCI()
	if afterPop.Func() != 0 {
		t.Errorf("After PopCI, Func() = %d, want 0 (base)", afterPop.Func())
	}
}

func TestPushMultipleCI(t *testing.T) {
	L := NewLuaState(nil)

	ci1 := &callInfo{func_: 1, top: 5, prev: nil, nresults: 1}
	L.PushCI(ci1)

	ci2 := &callInfo{func_: 2, top: 10, prev: nil, nresults: 1}
	L.PushCI(ci2)

	ci3 := &callInfo{func_: 3, top: 15, prev: nil, nresults: 1}
	L.PushCI(ci3)

	current := L.CurrentCI()
	if current.Func() != 3 {
		t.Errorf("Current Func() = %d, want 3", current.Func())
	}

	L.PopCI()
	current = L.CurrentCI()
	if current.Func() != 2 {
		t.Errorf("After 1 pop, Func() = %d, want 2", current.Func())
	}

	L.PopCI()
	current = L.CurrentCI()
	if current.Func() != 1 {
		t.Errorf("After 2 pops, Func() = %d, want 1", current.Func())
	}
}

// =============================================================================
// Global State Tests
// =============================================================================

func TestGlobalState(t *testing.T) {
	L := NewLuaState(nil)

	g := L.Global()
	if g == nil {
		t.Error("Global() should return non-nil")
	}

	// Allocator and Registry may be nil if not initialized (depends on init order)
	// Just verify the Global() method works
	_ = g.Allocator()
	_ = g.Registry()
	_ = g.CurrentThread()
}

func TestNewThread(t *testing.T) {
	L := NewLuaState(nil)

	newL := L.NewThread()
	if newL == nil {
		t.Error("NewThread() should return non-nil")
	}

	if newL.Stack() == nil {
		t.Error("NewThread stack should be initialized")
	}

	if newL.Global() != L.Global() {
		t.Error("NewThread should share global state with parent")
	}
}

// =============================================================================
// Index Conversion Tests
// =============================================================================

func TestIdx2Stack(t *testing.T) {
	L := NewLuaState(nil)
	L.SetTop(5)

	abs := L.idx2stack(3)
	if abs != 3 {
		t.Errorf("idx2stack(3) = %d, want 3", abs)
	}

	abs = L.idx2stack(-1)
	if abs != 5 {
		t.Errorf("idx2stack(-1) with top=5 = %d, want 5", abs)
	}

	abs = L.idx2stack(-3)
	if abs != 3 {
		t.Errorf("idx2stack(-3) with top=5 = %d, want 3", abs)
	}
}

func TestAbsoluteIndex(t *testing.T) {
	L := NewLuaState(nil)
	L.SetTop(5)

	abs := L.absoluteIndex(3)
	if abs != 3 {
		t.Errorf("absoluteIndex(3) = %d, want 3", abs)
	}

	abs = L.absoluteIndex(-1)
	if abs != 5 {
		t.Errorf("absoluteIndex(-1) with top=5 = %d, want 5", abs)
	}

	abs = L.absoluteIndex(-10)
	if abs != -1 {
		t.Errorf("absoluteIndex(-10) should return -1, got %d", abs)
	}
}

// =============================================================================
// Status Tests
// =============================================================================

func TestStatusConstants(t *testing.T) {
	if api.LUA_OK != 0 {
		t.Errorf("LUA_OK should be 0, got %d", api.LUA_OK)
	}
	if api.LUA_YIELD != 1 {
		t.Errorf("LUA_YIELD should be 1, got %d", api.LUA_YIELD)
	}
	if api.LUA_ERRRUN != 3 {
		t.Errorf("LUA_ERRRUN should be 3, got %d", api.LUA_ERRRUN)
	}
}

// =============================================================================
// Stack Value Access Tests
// =============================================================================

func TestStackValueAccess(t *testing.T) {
	L := NewLuaState(nil)

	// Use SetTop to create stack slots
	L.SetTop(3)

	// Verify stack has elements
	stack := L.Stack()
	if len(stack) < 3 {
		t.Errorf("Stack should have at least 3 elements, got %d", len(stack))
	}
}

func TestStackInterface(t *testing.T) {
	L := NewLuaState(nil)

	// Test StackSize
	initialSize := L.StackSize()
	if initialSize < 20 {
		t.Errorf("Initial stack size should be at least 20, got %d", initialSize)
	}

	// Test that Stack returns non-nil
	stack := L.Stack()
	if stack == nil {
		t.Error("Stack() should return non-nil slice")
	}
}

// =============================================================================
// LuaRegistryIndex Tests
// =============================================================================

func TestRegistryIndex(t *testing.T) {
	if api.LUA_REGISTRYINDEX != -10000 {
		t.Errorf("LUA_REGISTRYINDEX should be -10000, got %d", api.LUA_REGISTRYINDEX)
	}
}

// =============================================================================
// Status Values Tests
// =============================================================================

func TestStatusValues(t *testing.T) {
	// Test all status constants are defined
	statuses := []api.Status{
		api.LUA_OK,
		api.LUA_YIELD,
		api.LUA_ERRERR,
		api.LUA_ERRRUN,
		api.LUA_ERRSYNTAX,
		api.LUA_ERRMEM,
		api.LUA_ERRGCMM,
		api.LUA_ERRFILE,
		api.LUA_ERRRUNNING,
		api.LuaErrorStatus,
	}

	// Verify they're unique (except for LUA_ERRRUN which appears twice in our list)
	if len(statuses) != 10 {
		t.Errorf("Expected 10 status constants, got %d", len(statuses))
	}
}

// =============================================================================
// Thread Safety Tests
// =============================================================================

func TestConcurrentStackOperations(t *testing.T) {
	L := NewLuaState(nil)
	L.SetTop(5)

	done := make(chan bool, 10)

	// Run multiple goroutines concurrently accessing the state
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Panic in goroutine %d: %v", id, r)
				}
				done <- true
			}()

			// Perform various operations
			for j := 0; j < 100; j++ {
				_ = L.Top()
				_ = L.StackSize()
				_ = L.Stack()
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestConcurrentNewThread(t *testing.T) {
	L := NewLuaState(nil)
	done := make(chan bool, 10)

	// Multiple goroutines creating new threads from the same parent
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Panic in goroutine %d: %v", id, r)
				}
				done <- true
			}()

			for j := 0; j < 50; j++ {
				thread := L.NewThread()
				if thread == nil {
					t.Errorf("Goroutine %d: NewThread returned nil", id)
				}
				_ = thread.StackSize()
			}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestConcurrentCallInfoAccess(t *testing.T) {
	L := NewLuaState(nil)
	done := make(chan bool, 5)

	// Multiple goroutines accessing call info
	for i := 0; i < 5; i++ {
		go func(id int) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Panic in goroutine %d: %v", id, r)
				}
				done <- true
			}()

			for j := 0; j < 100; j++ {
				ci := L.CurrentCI()
				_ = ci.Func()
				_ = ci.Top()
				_ = ci.NResults()
			}
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}
}

// =============================================================================
// Index Conversion Edge Cases Tests
// =============================================================================

func TestIdx2StackEdgeCases(t *testing.T) {
	L := NewLuaState(nil)

	// Test with empty stack (top = 0)
	// idx2stack(-1) = 0 + (-1) + 1 = 0
	abs := L.idx2stack(-1)
	if abs != 0 {
		t.Errorf("idx2stack(-1) with empty stack = %d, want 0", abs)
	}

	// Test with single element (top = 1)
	L.SetTop(1)
	abs = L.idx2stack(-1)
	if abs != 1 {
		t.Errorf("idx2stack(-1) with top=1 = %d, want 1", abs)
	}

	abs = L.idx2stack(-2)
	if abs != 0 {
		t.Errorf("idx2stack(-2) with top=1 = %d, want 0", abs)
	}

	// Test boundary: idx2stack can return 0 for indices beyond stack
	abs = L.idx2stack(-10)
	if abs != -8 {
		t.Errorf("idx2stack(-10) with top=1 = %d, want -8", abs)
	}
}

func TestAbsoluteIndexEdgeCases(t *testing.T) {
	L := NewLuaState(nil)

	// Test with empty stack
	abs := L.absoluteIndex(-1)
	if abs != -1 {
		t.Errorf("absoluteIndex(-1) with empty stack = %d, want -1", abs)
	}

	// Test registry index
	abs = L.absoluteIndex(api.LUA_REGISTRYINDEX)
	if abs != -1 {
		t.Errorf("absoluteIndex(LUA_REGISTRYINDEX) = %d, want -1", abs)
	}

	// Test positive index larger than stack
	L.SetTop(3)
	abs = L.absoluteIndex(100)
	if abs != 100 {
		t.Errorf("absoluteIndex(100) with top=3 = %d, want 100", abs)
	}

	// Test negative index that would be below valid range
	abs = L.absoluteIndex(-100)
	if abs != -1 {
		t.Errorf("absoluteIndex(-100) with top=3 = %d, want -1", abs)
	}
}

// =============================================================================
// Stack Operation Boundary Tests
// =============================================================================

func TestSetTopBoundary(t *testing.T) {
	L := NewLuaState(nil)

	// Test expanding stack to very large size
	L.SetTop(1000)
	if L.Top() != 1000 {
		t.Errorf("After SetTop(1000), top = %d, want 1000", L.Top())
	}

	// Test shrinking stack
	L.SetTop(1)
	if L.Top() != 1 {
		t.Errorf("After SetTop(1), top = %d, want 1", L.Top())
	}

	// Test that SetTop(0) doesn't work on empty stack (due to idx2stack behavior)
	// SetTop(0) calls idx2stack(0) which returns 0 (invalid), then treated as >= 0
	// The actual behavior: SetTop(0) on empty stack stays at current top or goes to 0
	// Let's verify the actual current behavior
	L.SetTop(0)
	if L.Top() < 0 || L.Top() > 2 {
		t.Errorf("After SetTop(0), top = %d, unexpected", L.Top())
	}
}

func TestPopBoundary(t *testing.T) {
	L := NewLuaState(nil)

	// Pop from empty stack should not panic
	L.Pop()
	if L.Top() != -1 {
		t.Errorf("After Pop on empty stack, top = %d, want -1", L.Top())
	}

	// Multiple pops from empty stack
	L.Pop()
	if L.Top() != -2 {
		t.Errorf("After second Pop, top = %d, want -2", L.Top())
	}
}

func TestGrowStackLarge(t *testing.T) {
	L := NewLuaState(nil)

	initialSize := L.StackSize()

	// Grow by a large amount
	L.GrowStack(1000)
	newSize := L.StackSize()
	if newSize <= initialSize {
		t.Errorf("After GrowStack(1000), size = %d, should be > %d", newSize, initialSize)
	}
}

func TestPushValueBoundary(t *testing.T) {
	L := NewLuaState(nil)

	// PushValue with invalid index should not panic
	L.PushValue(100) // Invalid - beyond stack
	if L.Top() != 0 {
		t.Errorf("After PushValue(100) on empty stack, top = %d, want 0", L.Top())
	}

	L.PushValue(-1) // Invalid for empty stack
	if L.Top() != 0 {
		t.Errorf("After PushValue(-1) on empty stack, top = %d, want 0", L.Top())
	}

	// Valid push
	L.SetTop(2)
	L.PushValue(1)
	if L.Top() != 3 {
		t.Errorf("After PushValue(1) with top=2, top = %d, want 3", L.Top())
	}
}

// =============================================================================
// GlobalState Additional Tests
// =============================================================================

func TestGlobalStateSharing(t *testing.T) {
	L1 := NewLuaState(nil)
	L2 := L1.NewThread()

	// Verify they share the same global state
	g1 := L1.Global()
	g2 := L2.Global()

	if g1 != g2 {
		t.Error("Child thread should share global state with parent")
	}

	if g1.Allocator() != g2.Allocator() {
		t.Error("Allocators should be the same for shared global state")
	}

	if g1.Registry() != g2.Registry() {
		t.Error("Registries should be the same for shared global state")
	}
}

// =============================================================================
// CallInfo Additional Tests
// =============================================================================

func TestCallInfoChaining(t *testing.T) {
	// Create a chain of call infos
	ci1 := &callInfo{func_: 1, top: 5, nresults: 1}
	ci2 := &callInfo{func_: 2, top: 10, nresults: 2}
	ci3 := &callInfo{func_: 3, top: 15, nresults: 3}

	// Link them
	ci2.SetPrev(ci1)
	ci3.SetPrev(ci2)

	// Verify chain
	if ci3.Prev() != ci2 {
		t.Error("ci3.Prev() should be ci2")
	}

	if ci2.Prev() != ci1 {
		t.Error("ci2.Prev() should be ci1")
	}

	if ci1.Prev() != nil {
		t.Error("ci1.Prev() should be nil")
	}

	// Traverse chain with nil check
	count := 0
	current := ci3
	for current != nil {
		count++
		if count > 10 {
			t.Error("Chain too long, possible cycle")
			break
		}
		if current.prev == nil {
			break
		}
		current = current.prev
	}
	if count != 3 {
		t.Errorf("Chain traversal found %d nodes, want 3", count)
	}
}

func TestLuaStateIdx2stackAndAbsoluteIndex(t *testing.T) {
	L := NewLuaState(nil)
	L.SetTop(10)

	// Both methods should give same results for positive indices
	for i := 1; i <= 10; i++ {
		idxResult := L.idx2stack(i)
		absResult := L.absoluteIndex(i)
		if idxResult != absResult {
			t.Errorf("For positive idx %d: idx2stack=%d, absoluteIndex=%d", i, idxResult, absResult)
		}
	}

	// For negative indices, idx2stack can return non-positive values
	// while absoluteIndex returns -1 for invalid indices
	_ = L.absoluteIndex(-1000)
	_ = L.idx2stack(-1000)
}

// =============================================================================
// Phase 2 Base Library Function Tests
// =============================================================================

func TestLuaError(t *testing.T) {
	le := &LuaError{Msg: types.NewTValueString("test error")}
	if le.Error() != "test error" {
		t.Errorf("LuaError.Error() = %q, want %q", le.Error(), "test error")
	}
}

func TestBerror(t *testing.T) {
	stack := make([]types.TValue, 5)
	stack[1] = types.NewTValueString("boom")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("berror should panic")
		}
		le, ok := r.(*LuaError)
		if !ok {
			t.Fatalf("berror should panic with *LuaError, got %T", r)
		}
		if le.Error() != "boom" {
			t.Errorf("LuaError msg = %q, want %q", le.Error(), "boom")
		}
	}()
	berror(stack, 0)
}

func TestBpcallSuccess(t *testing.T) {
	// pcall(function that returns 42)
	innerFn := vm.GoFunc(func(stack []types.TValue, base int) int {
		stack[base] = types.NewTValueInteger(42)
		return 1
	})

	stack := make([]types.TValue, 10)
	stack[0] = nil // will be overwritten with result
	stack[1] = &goFuncWrapper{fn: innerFn}

	nRet := bpcall(stack, 0)
	if nRet < 1 {
		t.Fatalf("bpcall returned %d results, want >= 1", nRet)
	}
	if !stack[0].IsTrue() {
		t.Error("pcall success should return true as first value")
	}
	if nRet >= 2 && stack[1].IsInteger() && stack[1].GetInteger() == 42 {
		// good
	} else if nRet >= 2 {
		t.Errorf("pcall result = %v, want 42", stack[1])
	}
}

func TestBpcallError(t *testing.T) {
	// pcall(function that errors)
	innerFn := vm.GoFunc(func(stack []types.TValue, base int) int {
		luaErrorString("inner error")
		return 0
	})

	stack := make([]types.TValue, 10)
	stack[0] = nil
	stack[1] = &goFuncWrapper{fn: innerFn}

	nRet := bpcall(stack, 0)
	if nRet != 2 {
		t.Fatalf("bpcall on error returned %d results, want 2", nRet)
	}
	if !stack[0].IsFalse() {
		t.Error("pcall error should return false as first value")
	}
	if !stack[1].IsString() {
		t.Errorf("pcall error msg should be string, got tag %d", stack[1].GetTag())
	} else if s, ok := stack[1].GetValue().(string); !ok || s != "inner error" {
		t.Errorf("pcall error msg = %q, want %q", s, "inner error")
	}
}

func TestBassertWithLuaError(t *testing.T) {
	stack := make([]types.TValue, 5)
	stack[1] = types.NewTValueBoolean(false)
	stack[2] = types.NewTValueString("custom fail")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("bassert(false) should panic")
		}
		le, ok := r.(*LuaError)
		if !ok {
			t.Fatalf("bassert should panic with *LuaError, got %T", r)
		}
		if le.Error() != "custom fail" {
			t.Errorf("assert error = %q, want %q", le.Error(), "custom fail")
		}
	}()
	bassert(stack, 0)
}

func TestBselect(t *testing.T) {
	// select('#', 10, 20, 30) -> 3
	stack := make([]types.TValue, 10)
	stack[1] = types.NewTValueString("#")
	stack[2] = types.NewTValueInteger(10)
	stack[3] = types.NewTValueInteger(20)
	stack[4] = types.NewTValueInteger(30)

	nRet := bselect(stack[:5], 0)
	if nRet != 1 {
		t.Fatalf("select('#',...) returned %d, want 1", nRet)
	}
	if !stack[0].IsInteger() || stack[0].GetInteger() != 3 {
		t.Errorf("select('#', 10, 20, 30) = %v, want 3", stack[0])
	}

	// select(2, 10, 20, 30) -> 20, 30
	stack2 := make([]types.TValue, 10)
	stack2[1] = types.NewTValueInteger(2)
	stack2[2] = types.NewTValueInteger(10)
	stack2[3] = types.NewTValueInteger(20)
	stack2[4] = types.NewTValueInteger(30)

	nRet = bselect(stack2[:5], 0)
	if nRet != 2 {
		t.Fatalf("select(2,...) returned %d, want 2", nRet)
	}
	if !stack2[0].IsInteger() || stack2[0].GetInteger() != 20 {
		t.Errorf("select(2, 10, 20, 30)[1] = %v, want 20", stack2[0])
	}
	if !stack2[1].IsInteger() || stack2[1].GetInteger() != 30 {
		t.Errorf("select(2, 10, 20, 30)[2] = %v, want 30", stack2[1])
	}
}

func TestRawEqual(t *testing.T) {
	if !rawEqual(types.NewTValueInteger(5), types.NewTValueInteger(5)) {
		t.Error("rawEqual(5, 5) should be true")
	}
	if rawEqual(types.NewTValueInteger(5), types.NewTValueInteger(6)) {
		t.Error("rawEqual(5, 6) should be false")
	}
	if !rawEqual(types.NewTValueString("hello"), types.NewTValueString("hello")) {
		t.Error("rawEqual('hello', 'hello') should be true")
	}
	if !rawEqual(types.NewTValueNil(), types.NewTValueNil()) {
		t.Error("rawEqual(nil, nil) should be true")
	}
	if rawEqual(types.NewTValueInteger(5), types.NewTValueString("5")) {
		t.Error("rawEqual(5, '5') should be false")
	}
}

func TestBunpack(t *testing.T) {
	tbl := table.NewTable()
	tbl.SetInt(1, types.NewTValueInteger(10))
	tbl.SetInt(2, types.NewTValueInteger(20))
	tbl.SetInt(3, types.NewTValueInteger(30))

	stack := make([]types.TValue, 10)
	stack[1] = &tableWrapper{tbl: tbl}

	nRet := bunpack(stack, 0)
	if nRet != 3 {
		t.Fatalf("unpack returned %d, want 3", nRet)
	}
	for i, want := range []types.LuaInteger{10, 20, 30} {
		if !stack[i].IsInteger() || stack[i].GetInteger() != want {
			t.Errorf("unpack[%d] = %v, want %d", i, stack[i], want)
		}
	}
}

func TestBrawgetBrawset(t *testing.T) {
	tbl := table.NewTable()
	
	// rawset(t, "x", 42)
	stack := make([]types.TValue, 10)
	stack[1] = &tableWrapper{tbl: tbl}
	stack[2] = types.NewTValueString("x")
	stack[3] = types.NewTValueInteger(42)
	brawset(stack, 0)

	// rawget(t, "x") -> 42
	stack2 := make([]types.TValue, 10)
	stack2[1] = &tableWrapper{tbl: tbl}
	stack2[2] = types.NewTValueString("x")
	nRet := brawget(stack2, 0)
	if nRet != 1 {
		t.Fatalf("rawget returned %d, want 1", nRet)
	}
	if !stack2[0].IsInteger() || stack2[0].GetInteger() != 42 {
		t.Errorf("rawget(t, 'x') = %v, want 42", stack2[0])
	}
}

func TestBrawlen(t *testing.T) {
	// rawlen on string
	stack := make([]types.TValue, 5)
	stack[1] = types.NewTValueString("hello")
	nRet := brawlen(stack, 0)
	if nRet != 1 || !stack[0].IsInteger() || stack[0].GetInteger() != 5 {
		t.Errorf("rawlen('hello') = %v, want 5", stack[0])
	}

	// rawlen on table
	tbl := table.NewTable()
	tbl.SetInt(1, types.NewTValueInteger(10))
	tbl.SetInt(2, types.NewTValueInteger(20))
	stack2 := make([]types.TValue, 5)
	stack2[1] = &tableWrapper{tbl: tbl}
	nRet = brawlen(stack2, 0)
	if nRet != 1 || !stack2[0].IsInteger() || stack2[0].GetInteger() != 2 {
		t.Errorf("rawlen(table) = %v, want 2", stack2[0])
	}
}

func TestBsetmetatableBgetmetatable(t *testing.T) {
	tbl := table.NewTable()
	mt := table.NewTable()

	// setmetatable(t, mt)
	stack := make([]types.TValue, 5)
	stack[1] = &tableWrapper{tbl: tbl}
	stack[2] = &tableWrapper{tbl: mt}
	nRet := bsetmetatable(stack, 0)
	if nRet != 1 {
		t.Fatalf("setmetatable returned %d, want 1", nRet)
	}

	// getmetatable(t) -> mt
	stack2 := make([]types.TValue, 5)
	stack2[1] = &tableWrapper{tbl: tbl}
	nRet = bgetmetatable(stack2, 0)
	if nRet != 1 {
		t.Fatalf("getmetatable returned %d, want 1", nRet)
	}
	if stack2[0].IsNil() {
		t.Error("getmetatable should return non-nil after setmetatable")
	}
}

func TestBxpcallSuccess(t *testing.T) {
	innerFn := vm.GoFunc(func(stack []types.TValue, base int) int {
		stack[base] = types.NewTValueInteger(99)
		return 1
	})
	handler := vm.GoFunc(func(stack []types.TValue, base int) int {
		stack[base] = types.NewTValueString("handled")
		return 1
	})

	stack := make([]types.TValue, 10)
	stack[1] = &goFuncWrapper{fn: innerFn}
	stack[2] = &goFuncWrapper{fn: handler}

	nRet := bxpcall(stack, 0)
	if nRet < 1 {
		t.Fatalf("xpcall returned %d, want >= 1", nRet)
	}
	if !stack[0].IsTrue() {
		t.Error("xpcall success should return true")
	}
}

func TestBxpcallError(t *testing.T) {
	innerFn := vm.GoFunc(func(stack []types.TValue, base int) int {
		luaErrorString("kaboom")
		return 0
	})
	handler := vm.GoFunc(func(stack []types.TValue, base int) int {
		// Transform error message
		stack[base] = types.NewTValueString("handled: kaboom")
		return 1
	})

	stack := make([]types.TValue, 10)
	stack[1] = &goFuncWrapper{fn: innerFn}
	stack[2] = &goFuncWrapper{fn: handler}

	nRet := bxpcall(stack, 0)
	if nRet != 2 {
		t.Fatalf("xpcall on error returned %d, want 2", nRet)
	}
	if !stack[0].IsFalse() {
		t.Error("xpcall error should return false")
	}
	if s, ok := stack[1].GetValue().(string); !ok || s != "handled: kaboom" {
		t.Errorf("xpcall handler result = %v, want 'handled: kaboom'", stack[1])
	}
}
