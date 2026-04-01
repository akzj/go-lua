package main

/*
** golua - Lua interpreter in Go
** Main entry point
*/

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: golua <script.lua>")
		os.Exit(1)
	}

	filename := os.Args[1]

	// Read file contents
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("DEBUG: Read %d bytes from %s\n", len(data), filename)

	// Create Lua state
	L := lapi.Lua_newstate(nil, nil, 0)
	if L == nil {
		fmt.Fprintln(os.Stderr, "Error creating Lua state")
		os.Exit(1)
	}

	// Open standard libraries via internal call
	luaL_openlibs(L)

	fmt.Printf("DEBUG: First chars: %q\n", data)

	// Test with simple "return 1" first to isolate parser issue
	testCode := "return 1"
	fmt.Printf("DEBUG: Testing with: %s\n", testCode)
	testCl := testLuaLoad(L, testCode)
	if testCl != nil {
		fmt.Printf("DEBUG: testCl.Code len=%d, testCl.K len=%d\n", len(testCl.P.Code), len(testCl.P.K))
	}

	// Load the chunk
	ret := lapi.Lua_load(L, nil, string(data), "@"+filename, "")
	fmt.Printf("DEBUG: Lua_load returned %d\n", ret)

	if ret != 0 {
		fmt.Fprintf(os.Stderr, "Error loading script: %d\n", ret)
		lapi.Lua_close(L)
		os.Exit(1)
	}

	// Call the chunk
	fmt.Println("DEBUG: Calling lua_pcall")
	status := lapi.Lua_pcall(L, 0, lapi.LUA_MULTRET, 0)
	fmt.Printf("DEBUG: lua_pcall returned status=%d\n", status)

	if status != 0 {
		errMsg := lapi.Lua_tolstring(L, -1, nil)
		fmt.Fprintf(os.Stderr, "Runtime error code: %d\n", status)
		if errMsg != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", errMsg)
		}
		lapi.Lua_close(L)
		os.Exit(1)
	}

	lapi.Lua_close(L)
}

// luaL_openlibs - open standard libraries
func luaL_openlibs(L *lstate.LuaState) {
	// Open base library with print function
	lapi.Lua_pushcfunction(L, printFunction, 0)
	lapi.Lua_setglobal(L, "print")
}

// printFunction - C function implementing print
func printFunction(L *lobject.LuaState) int {
	// Cast to actual lstate.LuaState
	Lreal := (*lstate.LuaState)(unsafe.Pointer(L))
	n := lapi.Lua_gettop(Lreal)
	fmt.Printf("DEBUG printFunction: n=%d\n", n)
	for i := 1; i <= n; i++ {
		if i > 1 {
			fmt.Print("\t")
		}
		s := lapi.Lua_tolstring(Lreal, i, nil)
		fmt.Printf("DEBUG printFunction: arg[%d]=%q\n", i, s)
		fmt.Print(s)
	}
	fmt.Println()
	return 0
}

// testLuaLoad - test the parser directly
func testLuaLoad(L *lstate.LuaState, code string) *lobject.LClosure {
	// This is a simplified test - actual implementation would call parser directly
	_ = code
	return nil // Placeholder - actual test in VM
}
