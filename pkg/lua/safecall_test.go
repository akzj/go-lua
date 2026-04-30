package lua_test

import (
	"strings"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

func TestSafeCall(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Define a function that errors
	err := L.DoString(`
		function bad()
			error("something went wrong")
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("bad")
	callErr := L.SafeCall(0, 0)
	if callErr == nil {
		t.Fatal("expected error")
	}

	// Should contain traceback
	if !strings.Contains(callErr.Error(), "something went wrong") {
		t.Errorf("error missing message: %s", callErr)
	}
	if !strings.Contains(callErr.Error(), "stack traceback") {
		t.Errorf("error missing traceback: %s", callErr)
	}
}

func TestSafeCallSuccess(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`function add(a,b) return a+b end`)
	if err != nil {
		t.Fatal(err)
	}

	L.GetGlobal("add")
	L.PushInteger(3)
	L.PushInteger(4)
	callErr := L.SafeCall(2, 1)
	if callErr != nil {
		t.Fatal(callErr)
	}

	result, _ := L.ToInteger(-1)
	L.Pop(1)
	if result != 7 {
		t.Errorf("result = %d, want 7", result)
	}
}

func TestWrapSafe(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Register a function that panics
	L.PushFunction(lua.WrapSafe(func(L *lua.State) int {
		panic("intentional panic")
	}))
	L.SetGlobal("panicker")

	// Should not crash, should return Lua error
	err := L.DoString(`
		local ok, msg = pcall(panicker)
		assert(not ok)
		assert(msg:find("Go panic"))
		assert(msg:find("intentional panic"))
	`)
	if err != nil {
		t.Fatal(err)
	}
}
