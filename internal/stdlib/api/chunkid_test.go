package api

import (
	"fmt"
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

func TestMetamethodError(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	// Test what error message we get for failed metamethod call
	code := `
local function doit (s)
  local f, msg = load(s)
  if not f then return msg end
  local cond, msg = pcall(f)
  return (not cond) and msg
end

-- This is the test that fails at line 172-175 of errors.lua
local m = doit([[
  local a = setmetatable({}, {__add = 34})
  a = a + 1
]])
print("metamethod error:", m)
-- Expected: should contain "metamethod 'add'"
-- C Lua: [string "..."]:2: attempt to call a number value (metamethod 'add')
`
	err := L.DoString(code)
	if err != nil {
		fmt.Printf("DoString error: %v\n", err)
	}
}
