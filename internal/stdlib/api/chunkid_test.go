package api

import (
	"fmt"
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

func TestChunkidFormat(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	// Run errors.lua with debug flag on checkmessage to find which call fails
	code := `
local function doit (s)
  local f, msg = load(s)
  if not f then return msg end
  local cond, msg = pcall(f)
  return (not cond) and msg
end

local function checkmessage (prog, msg, debug)
  local m = doit(prog)
  if not string.find(m, msg, 1, true) then
    print("FAILED checkmessage:")
    print("  prog:", prog)
    print("  expected substring:", msg)
    print("  actual message:", m)
    error("checkmessage failed")
  end
end

-- Test the calls from errors.lua around line 134-160
checkmessage("a = {} + 1", "arithmetic")
print("PASS: arithmetic")
checkmessage("a = {} | 1", "bitwise operation")
print("PASS: bitwise")
checkmessage("a = {} < 1", "attempt to compare")
print("PASS: compare")
checkmessage("aaa=1; bbbb=2; aaa=math.sin(3)+bbbb(3)", "global 'bbbb'")
print("PASS: global bbbb")
checkmessage("aaa={}; do local aaa=1 end aaa:bbbb(3)", "method 'bbbb'")
print("PASS: method bbbb")
checkmessage("local a={}; a.bbbb(3)", "field 'bbbb'")
print("PASS: field bbbb")
`
	err := L.DoString(code)
	if err != nil {
		fmt.Printf("DoString error: %v\n", err)
	}
}
