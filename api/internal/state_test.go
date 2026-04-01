package internal

import (
	"testing"

	"github.com/akzj/go-lua/api/api"
)

// TestNewLuaState tests that we can create a new Lua state.
func TestNewLuaState(t *testing.T) {
	L := NewLuaState(nil)
	if L == nil {
		t.Fatal("NewLuaState returned nil")
	}
}

// TestLuaStateImplementsInterface verifies LuaState implements LuaAPI.
func TestLuaStateImplementsInterface(t *testing.T) {
	L := NewLuaState(nil)
	var _ api.LuaAPI = L
}

// TestPushAndTop tests basic push and top operations.
func TestPushAndTop(t *testing.T) {
	L := NewLuaState(nil)

	// Initially stack should be empty
	if top := L.Top(); top != 0 {
		t.Errorf("expected empty stack, got top=%d", top)
	}

	// Push nil and check
	L.PushNil()
	if top := L.Top(); top != 1 {
		t.Errorf("expected top=1 after PushNil, got top=%d", top)
	}
}

// TestTypeChecking tests type checking functions.
func TestTypeChecking(t *testing.T) {
	L := NewLuaState(nil)

	// Stack position 1 is out of range
	if L.Type(1) != api.LUA_TNONE {
		t.Errorf("expected LUA_TNONE for invalid index")
	}

	// Push nil and check
	L.PushNil()
	if L.Type(-1) != api.LUA_TNIL {
		t.Errorf("expected LUA_TNIL for nil value")
	}
	if !L.IsNil(-1) {
		t.Errorf("expected IsNil to return true for nil")
	}
}

// TestBooleanOperations tests boolean push and conversion.
func TestBooleanOperations(t *testing.T) {
	L := NewLuaState(nil)

	L.PushBoolean(true)
	if !L.ToBoolean(-1) {
		t.Error("expected true from ToBoolean")
	}
	if !L.IsBoolean(-1) {
		t.Error("expected IsBoolean to return true")
	}

	L.Pop()
	L.PushBoolean(false)
	if L.ToBoolean(-1) {
		t.Error("expected false from ToBoolean")
	}
}

// TestIntegerOperations tests integer push and conversion.
func TestIntegerOperations(t *testing.T) {
	L := NewLuaState(nil)

	L.PushInteger(42)
	val, ok := L.ToInteger(-1)
	if !ok {
		t.Error("expected ToInteger to succeed")
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
	if !L.IsInteger(-1) {
		t.Error("expected IsInteger to return true")
	}
}

// TestNumberOperations tests number (float) push and conversion.
func TestNumberOperations(t *testing.T) {
	L := NewLuaState(nil)

	L.PushNumber(3.14)
	val, ok := L.ToNumber(-1)
	if !ok {
		t.Error("expected ToNumber to succeed")
	}
	if val != 3.14 {
		t.Errorf("expected 3.14, got %f", val)
	}
	if !L.IsNumber(-1) {
		t.Error("expected IsNumber to return true")
	}
}

// TestStringOperations tests string push and conversion.
func TestStringOperations(t *testing.T) {
	L := NewLuaState(nil)

	L.PushString("hello")
	val, ok := L.ToString(-1)
	if !ok {
		t.Error("expected ToString to succeed")
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got '%s'", val)
	}
	if !L.IsString(-1) {
		t.Error("expected IsString to return true")
	}
}

// TestStatus tests the status methods.
func TestStatus(t *testing.T) {
	L := NewLuaState(nil)

	status := L.Status()
	if status != api.LUA_OK {
		t.Errorf("expected LUA_OK, got %d", status)
	}
}

// TestSetTop tests SetTop operation.
func TestSetTop(t *testing.T) {
	L := NewLuaState(nil)

	// SetTop with positive value
	L.SetTop(5)
	if top := L.Top(); top != 5 {
		t.Errorf("expected top=5, got top=%d", top)
	}

	// Shrink stack
	L.SetTop(2)
	if top := L.Top(); top != 2 {
		t.Errorf("expected top=2 after shrink, got top=%d", top)
	}
}

// TestCreateTable tests table creation.
func TestCreateTable(t *testing.T) {
	L := NewLuaState(nil)

	L.CreateTable(10, 5)
	if L.IsTable(-1) {
		t.Log("CreateTable pushed a table")
	} else {
		t.Error("expected table on stack after CreateTable")
	}
}

// TestAbsIndex tests absolute index conversion.
func TestAbsIndex(t *testing.T) {
	L := NewLuaState(nil)

	// Push a few values
	L.PushNil()
	L.PushInteger(1)
	L.PushInteger(2)

	// Negative indices
	abs := L.AbsIndex(-1)
	if abs != 3 {
		t.Errorf("expected AbsIndex(-1)=3, got %d", abs)
	}

	abs = L.AbsIndex(-2)
	if abs != 2 {
		t.Errorf("expected AbsIndex(-2)=2, got %d", abs)
	}

	// Already absolute
	abs = L.AbsIndex(1)
	if abs != 1 {
		t.Errorf("expected AbsIndex(1)=1, got %d", abs)
	}
}

// TestCheckStack tests stack capacity checking.
func TestCheckStack(t *testing.T) {
	L := NewLuaState(nil)

	// Should have room for basic operations
	if !L.CheckStack(10) {
		t.Error("expected CheckStack(10) to succeed")
	}
}

// TestRawLen tests raw length.
func TestRawLen(t *testing.T) {
	L := NewLuaState(nil)

	L.CreateTable(0, 0)
	// Empty table has length 0
	if len := L.RawLen(-1); len != 0 {
		t.Errorf("expected empty table len=0, got %d", len)
	}
}

// TestIsNoneOrNil tests the combined check.
func TestIsNoneOrNil(t *testing.T) {
	L := NewLuaState(nil)

	// Invalid index
	if !L.IsNoneOrNil(100) {
		t.Error("expected IsNoneOrNil to return true for invalid index")
	}

	// Nil value
	L.PushNil()
	if !L.IsNoneOrNil(-1) {
		t.Error("expected IsNoneOrNil to return true for nil")
	}

	// Non-nil value
	L.Pop()
	L.PushInteger(1)
	if L.IsNoneOrNil(-1) {
		t.Error("expected IsNoneOrNil to return false for non-nil")
	}
}

// TestTypename tests the typename helper.
func TestTypename(t *testing.T) {
	tests := []struct {
		tp    int
		name  string
	}{
		{api.LUA_TNIL, "nil"},
		{api.LUA_TBOOLEAN, "boolean"},
		{api.LUA_TNUMBER, "number"},
		{api.LUA_TSTRING, "string"},
		{api.LUA_TTABLE, "table"},
		{api.LUA_TFUNCTION, "function"},
		{api.LUA_TUSERDATA, "userdata"},
		{api.LUA_TTHREAD, "thread"},
	}

	for _, tc := range tests {
		name := api.Typename(tc.tp)
		if name != tc.name {
			t.Errorf("expected Typename(%d)='%s', got '%s'", tc.tp, tc.name, name)
		}
	}
}
