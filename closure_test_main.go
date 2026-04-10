//go:build ignore
package main

import "github.com/akzj/go-lua/state"

func main() {
    code := `
local function f() return f end
local r = f()
print("self-ref:", r ~= nil and "OK" or "FAIL", "f =", f)
`
    err := state.DoString(code)
    if err != nil { println("Error:", err) }
}
