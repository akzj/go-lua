package main

import (
	"fmt"
	"github.com/akzj/go-lua/pkg/api"
)

func main() {
	L := api.NewState()
	defer L.Close()
	L.OpenLibs()

	script := `
local pack = string.pack
local unpack = string.unpack
local packsize = string.packsize

local function assert_eq(actual, expected, msg)
	if actual ~= expected then
		error(msg .. ": expected " .. tostring(expected) .. " but got " .. tostring(actual))
	end
end

print("Test1: pack B")
local s = pack("B", 0xff)
assert_eq(#s, 1, "B size")
assert_eq(unpack("B", s), 0xff, "B value")

print("Test2: pack b (signed)")
local s = pack("b", -128)
assert_eq(unpack("b", s), -128, "b value")

print("Test3: pack h")
local s = pack("h", 0x1234)
assert_eq(#s, 2, "h size")
assert_eq(unpack("h", s), 0x1234, "h value")

print("Test4: pack h signed")
local s = pack("h", -0x8000)
assert_eq(unpack("h", s), -0x8000, "h signed value")

print("Test5: pack <i4")
local s = pack("<i4", 0x12345678)
assert_eq(#s, 4, "<i4 size")
assert_eq(unpack("<i4", s), 0x12345678, "<i4 value")

print("Test6: pack >i4")
local s = pack(">i4", 0x12345678)
assert_eq(unpack(">i4", s), 0x12345678, ">i4 value")

print("Test7: packsize")
assert_eq(packsize("i4"), 4, "packsize i4")

print("Test8: multiple values")
local s = pack("<bb", 1, 2)
local a, b = unpack("<bb", s)
assert_eq(a, 1, "first value")
assert_eq(b, 2, "second value")

print("Test9: c5 format")
local s = pack("c5", "abc")
assert_eq(#s, 5, "c5 size")

print("Test10: z format")
local s = pack("z", "hello")
local r = unpack("z", s)
assert_eq(r, "hello", "z value")

print("Test11: i3 format")
local s = pack("i3", 0x123456)
assert_eq(#s, 3, "i3 size")

print("Test12: j format")
local s = pack("j", 42)
local v = unpack("j", s)
assert_eq(v, 42, "j value")

print("Test13: f format")
local s = pack("f", 1.5)
assert_eq(#s, 4, "f size")

print("Test14: unpack position")
local s = pack("<bb", 1, 2)
local a, pos = unpack("<bb", s, 1)
assert_eq(a, 1, "first at pos1")
local b, pos2 = unpack("<bb", s, 2)
assert_eq(b, 2, "second at pos2")

print("ALL PACK/UNPACK TESTS PASSED!")
`
	err := L.DoString(script, "<verify>")
	if err != nil {
		fmt.Println("ERROR:", err)
	}
}
