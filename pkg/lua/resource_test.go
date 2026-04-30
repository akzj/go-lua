package lua_test

import (
	"errors"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

type mockResource struct {
	closed bool
	err    error
}

func (m *mockResource) Close() error {
	m.closed = true
	return m.err
}

func TestPushResourceGC(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	res := &mockResource{}
	L.PushResource(res)
	L.SetGlobal("myres")

	// Remove the reference
	L.PushNil()
	L.SetGlobal("myres")

	// Force GC — should trigger __gc
	L.GCCollect()
	L.GCCollect() // may need two cycles for finalization

	if !res.closed {
		t.Error("resource not closed after GC")
	}
}

func TestPushResourceExplicitClose(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	res := &mockResource{}
	L.PushResource(res)
	L.SetGlobal("myres")

	err := L.DoString(`myres:close()`)
	if err != nil {
		t.Fatal(err)
	}

	if !res.closed {
		t.Error("resource not closed after explicit close")
	}

	// Double close should be safe
	err = L.DoString(`myres:close()`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPushCloseableResourceTBC(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	res := &mockResource{}

	// Register a function that returns the resource
	L.PushFunction(func(L *lua.State) int {
		L.PushCloseableResource(res)
		return 1
	})
	L.SetGlobal("get_resource")

	// Use <close> syntax
	err := L.DoString(`
		do
			local r <close> = get_resource()
			-- r:close() will be called when leaving this block
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	if !res.closed {
		t.Error("resource not closed after <close> scope exit")
	}
}

func TestResourceCloseError(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	res := &mockResource{err: errors.New("close failed")}
	L.PushResource(res)
	L.SetGlobal("myres")

	err := L.DoString(`
		local ok, errmsg = myres:close()
		assert(ok == nil)
		assert(errmsg == "close failed")
	`)
	if err != nil {
		t.Fatal(err)
	}
}
