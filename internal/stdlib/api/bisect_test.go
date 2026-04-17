package api

import (
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

func TestBisectErrors(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)
	L.PushBoolean(true)
	L.SetGlobal("_port")
	
	// Run a Lua script that bisects checkmessage calls
	script := `
local debug = require"debug"

local function doit(s)
  local f, msg = load(s)
  if not f then return msg end
  local cond, msg = pcall(f)
  return (not cond) and msg
end

local tests = {
  {261, "local D=debug; local x=D.upvalueid(function() return debug end, 1); D.setuservalue(x, {})", "light userdata"},
  {266, "math.sin(io.input())", "(number expected, got FILE*)"},
  {269, "_G.XX=setmetatable({},{__name='My Type'}); io.input(XX)", "(FILE* expected, got My Type)"},
  {270, "_G.XX=setmetatable({},{__name='My Type'}); return XX + 1", "on a My Type value"},
  {271, "return ~io.stdin", "on a FILE* value"},
  {272, "_G.XX=setmetatable({},{__name='My Type'}); return XX < XX", "two My Type values"},
  {273, "_G.XX=setmetatable({},{__name='My Type'}); return {} < XX", "table with My Type"},
  {274, "_G.XX=setmetatable({},{__name='My Type'}); return XX < io.stdin", "My Type with FILE*"},
  {290, "(io.write or print){}", "io.write"},
  {291, "(collectgarbage or print){}", "collectgarbage"},
  {297, "local a,b = load('string x = \"hi\"'); print(a, b); assert(string.find(b, '?:?:'))", "?:?:"},
  {344, "math.sin('a')", "sin"},
  {353, "getmetatable(io.stdin).__gc()", "__gc"},
}

for _, t in ipairs(tests) do
  local line, prog, expected = t[1], t[2], t[3]
  local ok2, m = pcall(doit, prog)
  if not ok2 then
    print(string.format("L%d: CRASH - %s", line, tostring(m)))
  elseif m then
    local found = string.find(tostring(m), expected, 1, true)
    if found then
      print(string.format("L%d: PASS", line))
    else
      print(string.format("L%d: WRONG - expected '%s' got '%s'", line, expected, tostring(m)))
    end
  else
    print(string.format("L%d: NO_ERROR (code succeeded)", line))
  end
end
`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("bisect script error: %v", err)
	}
}
