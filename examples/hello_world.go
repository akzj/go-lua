// Package examples provides usage examples for go-lua.
package examples

import (
	"fmt"

	"github.com/akzj/go-lua/state"
)

// HelloWorld demonstrates the simplest usage of go-lua.
func HelloWorld() {
	// Execute simple Lua code using DoString
	err := state.DoString(`print("Hello, World!")`)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Evaluate expressions and get results
	L := state.New()
	err = state.DoStringOn(L, `result = 10 + 20`)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("Lua: 10 + 20 = 30")

	// Variables work correctly
	err = state.DoStringOn(L, `
		x = 5
		y = 3
		sum = x + y
		print("From Lua: 5 + 3 = " .. sum)
	`)
	if err != nil {
		fmt.Println("Error:", err)
	}
}
