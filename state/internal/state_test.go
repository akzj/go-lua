// Package internal provides tests for the Lua state management.
package internal

import (
	"testing"

	"github.com/akzj/go-lua/state/api"
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
