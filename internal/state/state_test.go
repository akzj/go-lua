package state

import (
	"testing"

	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/table"
)

// ---------------------------------------------------------------------------
// NewState tests
// ---------------------------------------------------------------------------

func TestNewState_Basic(t *testing.T) {
	L := NewState()
	if L == nil {
		t.Fatal("NewState returned nil")
	}
	if L.Global == nil {
		t.Fatal("Global is nil")
	}
	if L.Stack == nil {
		t.Fatal("Stack is nil")
	}
	if len(L.Stack) != BasicStackSize+ExtraStack {
		t.Errorf("Stack size = %d, want %d", len(L.Stack), BasicStackSize+ExtraStack)
	}
	if L.Status != StatusOK {
		t.Errorf("Status = %d, want StatusOK", L.Status)
	}
	if L.CI == nil {
		t.Fatal("CI is nil")
	}
	if L.CI != &L.BaseCI {
		t.Error("CI should point to BaseCI initially")
	}
}

func TestNewState_Registry(t *testing.T) {
	L := NewState()
	g := L.Global

	// Registry should be a table
	if g.Registry.Type() != object.TypeTable {
		t.Fatalf("Registry type = %v, want TypeTable", g.Registry.Type())
	}
	registry := g.Registry.Obj.(*table.Table)

	// registry[1] = false (C: setbfvalue)
	v, found := registry.GetInt(1)
	if !found {
		t.Fatal("registry[1] not found")
	}
	if v.Tt != object.TagFalse {
		t.Errorf("registry[1] tag = %v, want TagFalse", v.Tt)
	}

	// registry[LUA_RIDX_GLOBALS=2] = global table
	v, found = registry.GetInt(int64(RegistryIndexGlobals))
	if !found {
		t.Fatal("registry[GLOBALS] not found")
	}
	if v.Tt != object.TagTable {
		t.Errorf("registry[GLOBALS] tag = %v, want TagTable", v.Tt)
	}

	// registry[LUA_RIDX_MAINTHREAD=3] = main thread
	v, found = registry.GetInt(int64(RegistryIndexMainThread))
	if !found {
		t.Fatal("registry[MAINTHREAD] not found")
	}
	if v.Tt != object.TagThread {
		t.Errorf("registry[MAINTHREAD] tag = %v, want TagThread", v.Tt)
	}
	if v.Obj.(*LuaState) != L {
		t.Error("registry[MAINTHREAD] is not the main thread")
	}
}

func TestNewState_TMNames(t *testing.T) {
	L := NewState()
	g := L.Global

	expectedNames := [25]string{
		"__index", "__newindex", "__gc", "__mode", "__len", "__eq",
		"__add", "__sub", "__mul", "__mod", "__pow", "__div", "__idiv",
		"__band", "__bor", "__bxor", "__shl", "__shr",
		"__unm", "__bnot", "__lt", "__le",
		"__concat", "__call", "__close",
	}

	for i := 0; i < 25; i++ {
		if g.TMNames[i] == nil {
			t.Errorf("TMNames[%d] is nil", i)
			continue
		}
		if g.TMNames[i].Data != expectedNames[i] {
			t.Errorf("TMNames[%d] = %q, want %q", i, g.TMNames[i].Data, expectedNames[i])
		}
	}
}

func TestNewState_MainThread(t *testing.T) {
	L := NewState()
	if L.Global.MainThread != L {
		t.Error("MainThread should be L")
	}
}

func TestNewState_MemErrMsg(t *testing.T) {
	L := NewState()
	if L.Global.MemErrMsg == nil {
		t.Fatal("MemErrMsg is nil")
	}
	if L.Global.MemErrMsg.Data != "not enough memory" {
		t.Errorf("MemErrMsg = %q, want %q", L.Global.MemErrMsg.Data, "not enough memory")
	}
}

func TestNewState_NonYieldable(t *testing.T) {
	L := NewState()
	if L.Yieldable() {
		t.Error("Main thread should not be yieldable")
	}
}

func TestNewState_BaseCI(t *testing.T) {
	L := NewState()
	ci := &L.BaseCI
	if ci.Func != 0 {
		t.Errorf("BaseCI.Func = %d, want 0", ci.Func)
	}
	if ci.CallStatus&CISTC == 0 {
		t.Error("BaseCI should have CISTC flag (C frame)")
	}
	if ci.Prev != nil {
		t.Error("BaseCI.Prev should be nil")
	}
}

// ---------------------------------------------------------------------------
// Stack management tests
// ---------------------------------------------------------------------------

func TestGrowStack(t *testing.T) {
	L := NewState()

	// Push some values
	for i := 0; i < 10; i++ {
		PushValue(L, object.MakeInteger(int64(i+100)))
	}
	if L.Top != 11 { // 1 (base) + 10 pushed
		t.Errorf("Top = %d, want 11", L.Top)
	}

	// Verify values
	for i := 0; i < 10; i++ {
		v := L.Stack[i+1].Val
		if !v.IsInteger() || v.Integer() != int64(i+100) {
			t.Errorf("Stack[%d] = %v, want %d", i+1, v, i+100)
		}
	}

	// Grow the stack — request enough slots to exceed current capacity
	oldLen := len(L.Stack)
	GrowStack(L, BasicStackSize+100)
	if len(L.Stack) <= oldLen {
		t.Error("Stack should have grown")
	}

	// Verify values are preserved after growth
	for i := 0; i < 10; i++ {
		v := L.Stack[i+1].Val
		if !v.IsInteger() || v.Integer() != int64(i+100) {
			t.Errorf("After grow: Stack[%d] = %v, want %d", i+1, v, i+100)
		}
	}
}

func TestGrowStack_NoGrowIfEnoughSpace(t *testing.T) {
	L := NewState()
	oldLen := len(L.Stack)
	GrowStack(L, 5) // should not grow — plenty of space
	if len(L.Stack) != oldLen {
		t.Error("Stack should not have grown")
	}
}

func TestStackCheck(t *testing.T) {
	L := NewState()
	if !StackCheck(L, 10) {
		t.Error("StackCheck should return true for small n")
	}
	if StackCheck(L, 1000000) {
		t.Error("StackCheck should return false for huge n")
	}
}

func TestEnsureStack(t *testing.T) {
	L := NewState()
	L.Top = 30 // simulate nearly full stack
	EnsureStack(L, 20)
	// Should have grown to accommodate
	if L.StackLast()-L.Top < 20 {
		t.Error("EnsureStack did not provide enough space")
	}
}

func TestPushValue(t *testing.T) {
	L := NewState()
	initialTop := L.Top

	PushValue(L, object.MakeInteger(42))
	if L.Top != initialTop+1 {
		t.Errorf("Top = %d, want %d", L.Top, initialTop+1)
	}
	if v := L.Stack[initialTop].Val; !v.IsInteger() || v.Integer() != 42 {
		t.Errorf("Stack[%d] = %v, want 42", initialTop, v)
	}

	PushValue(L, object.MakeFloat(3.14))
	if L.Top != initialTop+2 {
		t.Errorf("Top = %d, want %d", L.Top, initialTop+2)
	}
	if v := L.Stack[initialTop+1].Val; !v.IsFloat() || v.Float() != 3.14 {
		t.Errorf("Stack[%d] = %v, want 3.14", initialTop+1, v)
	}
}

// ---------------------------------------------------------------------------
// CallInfo chain tests
// ---------------------------------------------------------------------------

func TestNewCI(t *testing.T) {
	L := NewState()

	// Initially at BaseCI
	if L.CI != &L.BaseCI {
		t.Fatal("Should start at BaseCI")
	}

	// Allocate first CI
	ci1 := NewCI(L)
	if ci1 == nil {
		t.Fatal("NewCI returned nil")
	}
	if L.CI != ci1 {
		t.Error("L.CI should point to new CI")
	}
	if ci1.Prev != &L.BaseCI {
		t.Error("ci1.Prev should be BaseCI")
	}
	if L.BaseCI.Next != ci1 {
		t.Error("BaseCI.Next should be ci1")
	}

	// Allocate second CI
	ci2 := NewCI(L)
	if ci2 == nil {
		t.Fatal("NewCI returned nil")
	}
	if L.CI != ci2 {
		t.Error("L.CI should point to ci2")
	}
	if ci2.Prev != ci1 {
		t.Error("ci2.Prev should be ci1")
	}
	if ci1.Next != ci2 {
		t.Error("ci1.Next should be ci2")
	}
}

func TestNewCI_Reuse(t *testing.T) {
	L := NewState()

	// Allocate two CIs
	ci1 := NewCI(L)
	ci2 := NewCI(L)
	_ = ci2

	// Go back to ci1
	L.CI = ci1

	// NewCI should reuse ci2
	reused := NewCI(L)
	if reused != ci2 {
		t.Error("NewCI should reuse existing next CI")
	}
}

func TestNewCI_NCI_Count(t *testing.T) {
	L := NewState()
	initialNCI := L.NCI

	NewCI(L)
	if L.NCI != initialNCI+1 {
		t.Errorf("NCI = %d, want %d", L.NCI, initialNCI+1)
	}

	NewCI(L)
	if L.NCI != initialNCI+2 {
		t.Errorf("NCI = %d, want %d", L.NCI, initialNCI+2)
	}
}

func TestFreeCI(t *testing.T) {
	L := NewState()
	NewCI(L)
	NewCI(L)

	// Go back to base
	L.CI = &L.BaseCI
	FreeCI(L)

	if L.BaseCI.Next != nil {
		t.Error("After FreeCI, BaseCI.Next should be nil")
	}
}

// ---------------------------------------------------------------------------
// NewThread tests
// ---------------------------------------------------------------------------

func TestNewThread(t *testing.T) {
	L := NewState()
	L1 := NewThread(L)

	if L1 == nil {
		t.Fatal("NewThread returned nil")
	}
	if L1.Global != L.Global {
		t.Error("Thread should share GlobalState")
	}
	if L1.Stack == nil {
		t.Fatal("Thread stack is nil")
	}
	if L1.Status != StatusOK {
		t.Errorf("Thread status = %d, want StatusOK", L1.Status)
	}
	// New thread should be yieldable (no incnny)
	if !L1.Yieldable() {
		t.Error("New thread should be yieldable")
	}
}

func TestNewThread_InheritsHooks(t *testing.T) {
	L := NewState()
	L.HookMask = MaskCall | MaskLine
	L.BaseHookCount = 100

	L1 := NewThread(L)
	if L1.HookMask != L.HookMask {
		t.Errorf("Thread HookMask = %d, want %d", L1.HookMask, L.HookMask)
	}
	if L1.BaseHookCount != L.BaseHookCount {
		t.Errorf("Thread BaseHookCount = %d, want %d", L1.BaseHookCount, L.BaseHookCount)
	}
}

// ---------------------------------------------------------------------------
// CloseState test
// ---------------------------------------------------------------------------

func TestCloseState(t *testing.T) {
	L := NewState()
	CloseState(L)

	if L.Stack != nil {
		t.Error("Stack should be nil after CloseState")
	}
	if L.Global != nil {
		t.Error("Global should be nil after CloseState")
	}
}
