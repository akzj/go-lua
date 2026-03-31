//go:build ignore

package main

import (
	"fmt"
	"github.com/akzj/go-lua/pkg/api"
)

func main() {
	L := api.NewState()
	L.OpenLibs()

	scripts := []struct {
		name string
		code string
	}{
		{"Test 1: pack < i1 i2", `
local pack = string.pack
local result = pack(" < i1 i2 ", 2, 3)
print("Result bytes:", #result)
for i=1,#result do print("  ", i, string.format("%02x", string.byte(result, i))) end
print("Expected: 02 03 00")
`},
		{"Test 2: packsize !8 xXi8", `
print("packsize('!8 xXi8')=", string.packsize("!8 xXi8"))
`},
		{"Test 3: unpack !8 xXi8", `
local pos = string.unpack("!8 xXi8", "0123456701234567")
print("unpack('!8 xXi8', ...) =", pos, "(expected 9)")
`},
		{"Test 4: packsize !2 xXi2", `
print("packsize('!2 xXi2')=", string.packsize("!2 xXi2"))
`},
		{"Test 5: unpack !2 xXi2", `
local pos = string.unpack("!2 xXi2", "0123456701234567")
print("unpack('!2 xXi2', ...) =", pos, "(expected 3)")
`},
		{"Test 6: pack b b Xd b Xb x", `
local x = string.pack(" b b Xd b Xb x", 1, 2, 3)
print("packsize(' b b Xd b Xb x')=", string.packsize(" b b Xd b Xb x"))
print("Result size:", #x)
for i=1,#x do print("  ", i, string.format("%02x", string.byte(x, i))) end
`},
	}
	for _, s := range scripts {
		fmt.Printf("\n=== %s ===\n", s.name)
		if err := L.DoString(s.code, "=(test)"); err != nil {
			fmt.Printf("ERROR: %v\n", err)
		}
	}
}
