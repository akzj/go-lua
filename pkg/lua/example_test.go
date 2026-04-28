package lua_test

import (
	"fmt"
	"github.com/akzj/go-lua/pkg/lua"
)

func Example() {
	L := lua.NewState()
	defer L.Close()
	L.DoString(`print("hello from Lua")`)
	// Output:
	// hello from Lua
}

func ExampleState_DoString() {
	L := lua.NewState()
	defer L.Close()
	err := L.DoString(`result = 2 + 3`)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	L.GetGlobal("result")
	val, _ := L.ToInteger(-1)
	fmt.Println(val)
	// Output:
	// 5
}

func ExampleState_PushFunction() {
	L := lua.NewState()
	defer L.Close()
	
	add := func(L *lua.State) int {
		a := L.CheckInteger(1)
		b := L.CheckInteger(2)
		L.PushInteger(a + b)
		return 1
	}
	L.PushFunction(add)
	L.SetGlobal("add")
	
	L.DoString(`result = add(10, 32)`)
	L.GetGlobal("result")
	val, _ := L.ToInteger(-1)
	fmt.Println(val)
	// Output:
	// 42
}

func ExampleRegisterModule() {
	L := lua.NewState()
	defer L.Close()
	
	lua.RegisterModule(L, "greet", map[string]lua.Function{
		"hello": func(L *lua.State) int {
			name := L.CheckString(1)
			L.PushString(fmt.Sprintf("Hello, %s!", name))
			return 1
		},
	})
	
	L.DoString(`
		local g = require("greet")
		print(g.hello("World"))
	`)
	// Output:
	// Hello, World!
}

func ExampleState_ReloadModule() {
	L := lua.NewState()
	defer L.Close()
	
	// Load initial module
	L.DoString(`
		package.preload["mymod"] = function()
			local M = {}
			local count = 0
			function M.inc() count = count + 1; return count end
			function M.get() return count end
			return M
		end
	`)
	L.DoString(`
		local m = require("mymod")
		m.inc(); m.inc(); m.inc()
	`)
	
	// Update module (inc now adds 10)
	L.DoString(`
		package.preload["mymod"] = function()
			local M = {}
			local count = 0
			function M.inc() count = count + 10; return count end
			function M.get() return count end
			return M
		end
	`)
	
	result, _ := L.ReloadModule("mymod")
	fmt.Printf("replaced: %d\n", result.Replaced)
	
	// State preserved, new behavior active
	L.DoString(`
		local m = require("mymod")
		print("count:", m.get())  -- preserved: 3
		m.inc()
		print("after inc:", m.get())  -- 3 + 10 = 13
	`)
	// Output:
	// replaced: 2
	// count:	3
	// after inc:	13
}
