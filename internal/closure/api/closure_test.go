package api

import (
	"testing"

	objectapi "github.com/akzj/go-lua/internal/object/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
)

// ---------------------------------------------------------------------------
// Constructor tests (already defined in api.go, but verify behavior)
// ---------------------------------------------------------------------------

func TestNewLClosure(t *testing.T) {
	proto := &objectapi.Proto{
		NumParams:    2,
		MaxStackSize: 10,
	}
	cl := NewLClosure(proto, 3)
	if cl == nil {
		t.Fatal("NewLClosure returned nil")
	}
	if cl.Proto != proto {
		t.Error("Proto mismatch")
	}
	if len(cl.UpVals) != 3 {
		t.Errorf("UpVals len = %d, want 3", len(cl.UpVals))
	}
	// All upvals should be nil initially
	for i, uv := range cl.UpVals {
		if uv != nil {
			t.Errorf("UpVals[%d] should be nil, got %v", i, uv)
		}
	}
}

func TestNewCClosure(t *testing.T) {
	fn := func(L *stateapi.LuaState) int { return 0 }
	cl := NewCClosure(fn, 2)
	if cl == nil {
		t.Fatal("NewCClosure returned nil")
	}
	if cl.Fn == nil {
		t.Error("Fn is nil")
	}
	if len(cl.UpVals) != 2 {
		t.Errorf("UpVals len = %d, want 2", len(cl.UpVals))
	}
	// All upvals should be nil TValue initially
	for i, uv := range cl.UpVals {
		if !uv.IsNil() {
			t.Errorf("UpVals[%d] should be Nil, got %v", i, uv)
		}
	}
}

func TestNewCClosure_NumUpvals(t *testing.T) {
	fn := func(L *stateapi.LuaState) int { return 0 }
	cl := NewCClosure(fn, 5)
	if cl.NumUpvals() != 5 {
		t.Errorf("NumUpvals = %d, want 5", cl.NumUpvals())
	}
}

// ---------------------------------------------------------------------------
// UpVal lifecycle tests
// ---------------------------------------------------------------------------

func TestUpVal_OpenClose(t *testing.T) {
	uv := &UpVal{
		StackIdx: 5,
		Own:      objectapi.Nil,
	}

	if !uv.IsOpen() {
		t.Error("Should be open")
	}

	// Create a mock stack
	stack := make([]objectapi.StackValue, 10)
	stack[5].Val = objectapi.MakeInteger(42)

	// Get should return stack value
	val := uv.Get(stack)
	if !val.IsInteger() || val.Integer() != 42 {
		t.Errorf("Get = %v, want 42", val)
	}

	// Set should modify stack
	uv.Set(stack, objectapi.MakeInteger(99))
	if stack[5].Val.Integer() != 99 {
		t.Errorf("After Set, stack[5] = %v, want 99", stack[5].Val)
	}

	// Close the upvalue
	uv.Close(objectapi.MakeInteger(99))
	if uv.IsOpen() {
		t.Error("Should be closed after Close()")
	}
	if uv.StackIdx != -1 {
		t.Errorf("StackIdx = %d, want -1", uv.StackIdx)
	}

	// Get should return Own value
	val = uv.Get(nil)
	if !val.IsInteger() || val.Integer() != 99 {
		t.Errorf("After close, Get = %v, want 99", val)
	}

	// Set should modify Own
	uv.Set(nil, objectapi.MakeFloat(3.14))
	if uv.Own.Float() != 3.14 {
		t.Errorf("After close Set, Own = %v, want 3.14", uv.Own)
	}
}

// ---------------------------------------------------------------------------
// FindUpval tests
// ---------------------------------------------------------------------------

func newTestState() *stateapi.LuaState {
	return stateapi.NewState()
}

func TestFindUpval_CreateNew(t *testing.T) {
	L := newTestState()

	// Set up some stack values
	L.Stack[3].Val = objectapi.MakeInteger(30)
	L.Stack[5].Val = objectapi.MakeInteger(50)

	// Find upvalue at level 5 (no existing upvalues)
	uv := FindUpval(L, 5)
	if uv == nil {
		t.Fatal("FindUpval returned nil")
	}
	if uv.StackIdx != 5 {
		t.Errorf("StackIdx = %d, want 5", uv.StackIdx)
	}
	if !uv.IsOpen() {
		t.Error("Should be open")
	}

	// Verify it's in the open list
	head := L.OpenUpval.(*UpVal)
	if head != uv {
		t.Error("Should be head of open list")
	}
}

func TestFindUpval_Sharing(t *testing.T) {
	L := newTestState()
	L.Stack[5].Val = objectapi.MakeInteger(50)

	// Create first upvalue at level 5
	uv1 := FindUpval(L, 5)

	// Find again at same level — should return same upvalue
	uv2 := FindUpval(L, 5)
	if uv1 != uv2 {
		t.Error("FindUpval should return same upvalue for same level (sharing)")
	}
}

func TestFindUpval_SortedOrder(t *testing.T) {
	L := newTestState()
	L.Stack[3].Val = objectapi.MakeInteger(30)
	L.Stack[5].Val = objectapi.MakeInteger(50)
	L.Stack[7].Val = objectapi.MakeInteger(70)

	// Create upvalues in non-sorted order
	uv5 := FindUpval(L, 5)
	uv7 := FindUpval(L, 7)
	uv3 := FindUpval(L, 3)

	// List should be sorted descending: 7 → 5 → 3
	head := L.OpenUpval.(*UpVal)
	if head != uv7 {
		t.Errorf("Head should be uv7 (level 7), got level %d", head.StackIdx)
	}
	if head.Next != uv5 {
		t.Error("Second should be uv5 (level 5)")
	}
	if head.Next.Next != uv3 {
		t.Error("Third should be uv3 (level 3)")
	}
	if uv3.Next != nil {
		t.Error("Last should have nil Next")
	}
}

func TestFindUpval_InsertMiddle(t *testing.T) {
	L := newTestState()

	// Create at 7 and 3
	FindUpval(L, 7)
	FindUpval(L, 3)

	// Now insert at 5 — should go between 7 and 3
	uv5 := FindUpval(L, 5)

	head := L.OpenUpval.(*UpVal)
	if head.StackIdx != 7 {
		t.Errorf("Head level = %d, want 7", head.StackIdx)
	}
	if head.Next != uv5 {
		t.Error("Second should be uv5")
	}
	if uv5.Next.StackIdx != 3 {
		t.Errorf("Third level = %d, want 3", uv5.Next.StackIdx)
	}
}

// ---------------------------------------------------------------------------
// CloseUpvals tests
// ---------------------------------------------------------------------------

func TestCloseUpvals_Basic(t *testing.T) {
	L := newTestState()
	L.Stack[3].Val = objectapi.MakeInteger(30)
	L.Stack[5].Val = objectapi.MakeInteger(50)
	L.Stack[7].Val = objectapi.MakeInteger(70)

	uv3 := FindUpval(L, 3)
	FindUpval(L, 5)
	uv7 := FindUpval(L, 7)

	// Close all upvalues at level >= 5
	CloseUpvals(L, 5)

	// uv7 should be closed with value 70
	if uv7.IsOpen() {
		t.Error("uv7 should be closed")
	}
	if uv7.Own.Integer() != 70 {
		t.Errorf("uv7.Own = %v, want 70", uv7.Own)
	}

	// uv3 should still be open
	if !uv3.IsOpen() {
		t.Error("uv3 should still be open")
	}

	// Open list should only contain uv3
	head := L.OpenUpval.(*UpVal)
	if head != uv3 {
		t.Error("Open list head should be uv3")
	}
	if head.Next != nil {
		t.Error("uv3 should be the only open upvalue")
	}
}

func TestCloseUpvals_All(t *testing.T) {
	L := newTestState()
	L.Stack[3].Val = objectapi.MakeInteger(30)
	L.Stack[5].Val = objectapi.MakeInteger(50)

	uv3 := FindUpval(L, 3)
	uv5 := FindUpval(L, 5)

	// Close all (level 0)
	CloseUpvals(L, 0)

	if uv3.IsOpen() || uv5.IsOpen() {
		t.Error("All upvalues should be closed")
	}
	if uv3.Own.Integer() != 30 {
		t.Errorf("uv3.Own = %v, want 30", uv3.Own)
	}
	if uv5.Own.Integer() != 50 {
		t.Errorf("uv5.Own = %v, want 50", uv5.Own)
	}

	// Open list should be empty
	if L.OpenUpval != nil {
		t.Error("Open list should be nil")
	}
}

func TestCloseUpvals_None(t *testing.T) {
	L := newTestState()
	L.Stack[3].Val = objectapi.MakeInteger(30)

	uv3 := FindUpval(L, 3)

	// Close at level 10 — nothing to close
	CloseUpvals(L, 10)

	if !uv3.IsOpen() {
		t.Error("uv3 should still be open")
	}
}

func TestCloseUpvals_CapturesCurrentValue(t *testing.T) {
	L := newTestState()
	L.Stack[5].Val = objectapi.MakeInteger(100)

	uv := FindUpval(L, 5)

	// Modify the stack value before closing
	L.Stack[5].Val = objectapi.MakeInteger(200)

	CloseUpvals(L, 5)

	// Should have captured the value at time of close (200)
	if uv.Own.Integer() != 200 {
		t.Errorf("Captured value = %v, want 200", uv.Own)
	}
}

// ---------------------------------------------------------------------------
// InitUpvals tests
// ---------------------------------------------------------------------------

func TestInitUpvals(t *testing.T) {
	proto := &objectapi.Proto{}
	cl := NewLClosure(proto, 3)

	// Before init, all nil
	for i, uv := range cl.UpVals {
		if uv != nil {
			t.Errorf("Before init: UpVals[%d] should be nil", i)
		}
	}

	InitUpvals(cl)

	// After init, all should be closed UpVals with nil value
	for i, uv := range cl.UpVals {
		if uv == nil {
			t.Errorf("After init: UpVals[%d] should not be nil", i)
			continue
		}
		if uv.IsOpen() {
			t.Errorf("After init: UpVals[%d] should be closed", i)
		}
		if !uv.Own.IsNil() {
			t.Errorf("After init: UpVals[%d].Own should be Nil", i)
		}
	}
}

func TestInitUpvals_PreservesExisting(t *testing.T) {
	proto := &objectapi.Proto{}
	cl := NewLClosure(proto, 3)

	// Set one upvalue manually
	existing := &UpVal{StackIdx: 5, Own: objectapi.MakeInteger(42)}
	cl.UpVals[1] = existing

	InitUpvals(cl)

	// Slot 1 should still be the existing one
	if cl.UpVals[1] != existing {
		t.Error("InitUpvals should not overwrite existing upvalues")
	}
	// Slots 0 and 2 should be initialized
	if cl.UpVals[0] == nil || cl.UpVals[2] == nil {
		t.Error("InitUpvals should fill nil slots")
	}
}
