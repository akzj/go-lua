package lua_test

import (
	"sync"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

// ---------------------------------------------------------------------------
// RegisterGlobal tests
// ---------------------------------------------------------------------------

func TestRegisterGlobal_Basic(t *testing.T) {
	// Register a module globally
	lua.RegisterGlobal("testmod", func(L *lua.State) {
		L.NewTableFrom(map[string]any{
			"greeting": "hello from testmod",
		})
	})
	defer lua.UnregisterGlobal("testmod")

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local m = require("testmod")
		assert(m.greeting == "hello from testmod", "expected greeting, got: " .. tostring(m.greeting))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestRegisterGlobal_FunctionModule(t *testing.T) {
	// Register a module that exposes callable functions
	lua.RegisterGlobal("mymath", func(L *lua.State) {
		L.CreateTable(0, 2)
		L.PushFunction(func(L *lua.State) int {
			a := L.CheckNumber(1)
			b := L.CheckNumber(2)
			L.PushNumber(a + b)
			return 1
		})
		L.SetField(-2, "add")
	})
	defer lua.UnregisterGlobal("mymath")

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local m = require("mymath")
		local result = m.add(10, 20)
		assert(result == 30, "expected 30, got: " .. tostring(result))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestRegisterGlobal_Cached(t *testing.T) {
	// Verify that require() caches the module (second require returns same table)
	callCount := 0
	lua.RegisterGlobal("counted", func(L *lua.State) {
		callCount++
		L.NewTableFrom(map[string]any{"count": callCount})
	})
	defer lua.UnregisterGlobal("counted")

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local m1 = require("counted")
		local m2 = require("counted")
		-- require() caches, so m1 and m2 should be the same table
		assert(rawequal(m1, m2), "expected same table from repeated require()")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// The opener should only be called once
	if callCount != 1 {
		t.Fatalf("expected opener called once, got %d", callCount)
	}
}

func TestRegisterGlobal_Unregister(t *testing.T) {
	lua.RegisterGlobal("ephemeral", func(L *lua.State) {
		L.NewTableFrom(map[string]any{"value": 42})
	})

	// First verify it works
	L1 := lua.NewState()
	defer L1.Close()

	err := L1.DoString(`
		local m = require("ephemeral")
		assert(m.value == 42)
	`)
	if err != nil {
		t.Fatalf("first require failed: %v", err)
	}

	// Unregister
	lua.UnregisterGlobal("ephemeral")

	// New state should NOT find it
	L2 := lua.NewState()
	defer L2.Close()

	err = L2.DoString(`
		local ok, err = pcall(require, "ephemeral")
		assert(not ok, "expected require to fail after unregister")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestRegisterGlobal_NotFound(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local ok, err = pcall(require, "nonexistent_module_xyz")
		assert(not ok, "expected require to fail for nonexistent module")
		assert(type(err) == "string", "expected error string")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestRegisterGlobal_MultipleStates(t *testing.T) {
	// Same global module should work from multiple independent States
	lua.RegisterGlobal("shared", func(L *lua.State) {
		L.NewTableFrom(map[string]any{"name": "shared-module"})
	})
	defer lua.UnregisterGlobal("shared")

	for i := 0; i < 5; i++ {
		L := lua.NewState()
		err := L.DoString(`
			local m = require("shared")
			assert(m.name == "shared-module")
		`)
		if err != nil {
			t.Fatalf("state %d: DoString failed: %v", i, err)
		}
		L.Close()
	}
}

func TestRegisterGlobal_ThreadSafety(t *testing.T) {
	// Multiple goroutines registering and looking up concurrently
	var wg sync.WaitGroup

	// Register some modules concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := "tsmod" + string(rune('A'+idx))
			lua.RegisterGlobal(name, func(L *lua.State) {
				L.NewTableFrom(map[string]any{"idx": idx})
			})
		}(i)
	}
	wg.Wait()

	// Clean up after test
	defer func() {
		for i := 0; i < 10; i++ {
			lua.UnregisterGlobal("tsmod" + string(rune('A'+i)))
		}
	}()

	// Concurrently read GlobalModules
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			names := lua.GlobalModules()
			if len(names) < 1 {
				t.Errorf("expected at least 1 module, got %d", len(names))
			}
		}()
	}
	wg.Wait()

	// Concurrently create States and require modules
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := "tsmod" + string(rune('A'+idx))
			L := lua.NewState()
			defer L.Close()
			err := L.DoString(`
				local m = require("` + name + `")
				assert(type(m) == "table", "expected table")
			`)
			if err != nil {
				t.Errorf("goroutine %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestGlobalModules(t *testing.T) {
	// Start with a known set
	lua.RegisterGlobal("listmod_a", func(L *lua.State) {
		L.PushBoolean(true)
	})
	lua.RegisterGlobal("listmod_b", func(L *lua.State) {
		L.PushBoolean(true)
	})
	defer lua.UnregisterGlobal("listmod_a")
	defer lua.UnregisterGlobal("listmod_b")

	names := lua.GlobalModules()
	found := make(map[string]bool)
	for _, n := range names {
		found[n] = true
	}
	if !found["listmod_a"] || !found["listmod_b"] {
		t.Fatalf("expected listmod_a and listmod_b in %v", names)
	}
}

func TestRegisterGlobal_PreloadTakesPriority(t *testing.T) {
	// Preload (searcher #1) should take priority over global registry (searcher #3)
	lua.RegisterGlobal("priority_test", func(L *lua.State) {
		L.NewTableFrom(map[string]any{"source": "global"})
	})
	defer lua.UnregisterGlobal("priority_test")

	L := lua.NewState()
	defer L.Close()

	// Register in preload (which takes priority)
	err := L.DoString(`
		package.preload["priority_test"] = function()
			return { source = "preload" }
		end
		local m = require("priority_test")
		assert(m.source == "preload", "expected preload to win, got: " .. m.source)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestRegisterGlobal_InSandbox(t *testing.T) {
	lua.RegisterGlobal("sandbox_mod", func(L *lua.State) {
		L.NewTableFrom(map[string]any{"safe": true})
	})
	defer lua.UnregisterGlobal("sandbox_mod")

	// Sandbox with package library enabled
	L := lua.NewSandboxState(lua.SandboxConfig{
		AllowPackage: true,
	})
	defer L.Close()

	err := L.DoString(`
		local m = require("sandbox_mod")
		assert(m.safe == true, "expected safe=true")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestRegisterGlobal_EmptyOpener(t *testing.T) {
	// An opener that pushes nothing should still work (returns true)
	lua.RegisterGlobal("empty_mod", func(L *lua.State) {
		// Push nothing — installGlobalSearcher should push true
	})
	defer lua.UnregisterGlobal("empty_mod")

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local m = require("empty_mod")
		assert(m == true, "expected true for empty opener, got: " .. tostring(m))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Module interface tests
// ---------------------------------------------------------------------------

// testModule implements the Module interface for testing.
type testModule struct {
	name  string
	value string
}

func (m testModule) Name() string { return m.name }
func (m testModule) Open(L *lua.State) {
	L.NewTableFrom(map[string]any{"value": m.value})
}

func TestModule_Interface(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mod := testModule{name: "iface_test", value: "hello"}
	lua.LoadModules(L, mod)

	err := L.DoString(`
		local m = require("iface_test")
		assert(m.value == "hello", "expected hello, got: " .. tostring(m.value))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestLoadModules_Multiple(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mod1 := testModule{name: "multi_a", value: "alpha"}
	mod2 := testModule{name: "multi_b", value: "beta"}
	lua.LoadModules(L, mod1, mod2)

	err := L.DoString(`
		local a = require("multi_a")
		local b = require("multi_b")
		assert(a.value == "alpha", "expected alpha")
		assert(b.value == "beta", "expected beta")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestLoadModules_Cached(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	callCount := 0
	lua.LoadModules(L, &countingModule{count: &callCount})

	err := L.DoString(`
		local m1 = require("counting")
		local m2 = require("counting")
		assert(rawequal(m1, m2), "expected same table from repeated require()")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected opener called once, got %d", callCount)
	}
}

type countingModule struct {
	count *int
}

func (m *countingModule) Name() string { return "counting" }
func (m *countingModule) Open(L *lua.State) {
	*m.count++
	L.NewTableFrom(map[string]any{"opened": *m.count})
}
