package api

import (
	"fmt"
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

func TestChunkidFormat(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	// Run a Lua snippet that prints error messages
	code := `
local function doit (s)
  local f, msg = load(s)
  if not f then return msg end
  local cond, msg = pcall(f)
  return (not cond) and msg
end

-- syntax error from load()
print("syntax:", doit("syntax error"))

-- eof error from load()
print("eof:", doit([[
  local a = {4

]]))

-- runtime error from load()()
print("runtime:", doit("a = math.sin()"))

-- Check the pattern that checksyntax expects
local msg = doit("syntax error")
local pt = string.format([[^%%[string ".*"%%]:%d: .- near %s$]], 1, "'error'")
print("pattern:", pt)
print("match:", string.find(msg, pt) ~= nil)
`
	err := L.DoString(code)
	if err != nil {
		fmt.Printf("DoString error: %v\n", err)
	}
}
