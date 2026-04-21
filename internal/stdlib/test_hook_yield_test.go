package stdlib_test

import (
	"fmt"
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api"
	"github.com/akzj/go-lua/internal/stdlib"
)

func TestHookYieldLine(t *testing.T) {
	L := luaapi.NewState()
	stdlib.OpenAll(L)
	stdlib.OpenTestLib(L)

	// Matches coroutine.lua lines 651-668: yields in line hook
	src := `
local line = debug.getinfo(1, "l").currentline + 2
local function foo()
  local x = 10
  x = x + 10
  _G.XX = x
end

local co = coroutine.wrap(function()
  T.sethook("setglobal X; yield 0", "l", 0); foo(); return 10 end)

_G.XX = nil;
_G.X = nil; co(); assert(_G.X == line, "step1: X=" .. tostring(_G.X) .. " expected=" .. line)
_G.X = nil; co(); assert(_G.X == line + 1, "step2: X=" .. tostring(_G.X) .. " expected=" .. (line+1))
_G.X = nil; co(); assert(_G.X == line + 2 and _G.XX == nil, "step3")
_G.X = nil; co(); assert(_G.X == line + 3 and _G.XX == 20, "step4")
assert(co() == 10, "step5: co() should return 10")
return "OK"
`
	status := L.Load(src, "=test", "bt")
	if status != 0 {
		msg, _ := L.ToString(-1)
		t.Fatalf("load error: %s", msg)
	}
	status = L.PCall(0, 1, 0)
	if status != 0 {
		msg, _ := L.ToString(-1)
		t.Fatalf("pcall error: %s", msg)
	}
	result, _ := L.ToString(-1)
	fmt.Printf("Line hook yield: %s\n", result)
}

func TestHookYieldCount(t *testing.T) {
	L := luaapi.NewState()
	stdlib.OpenAll(L)
	stdlib.OpenTestLib(L)

	// Matches coroutine.lua lines 670-677: yields in count hook
	src := `
local function foo()
  local x = 10
  x = x + 10
  _G.XX = x
end

local co = coroutine.wrap(function()
  T.sethook("yield 0", "", 1); foo(); return 10 end)

_G.XX = nil;
local c = 0
repeat c = c + 1; local a = co() until a == 10
assert(_G.XX == 20 and c >= 5, "count hook: XX=" .. tostring(_G.XX) .. " c=" .. c)
return "OK"
`
	status := L.Load(src, "=test", "bt")
	if status != 0 {
		msg, _ := L.ToString(-1)
		t.Fatalf("load error: %s", msg)
	}
	status = L.PCall(0, 1, 0)
	if status != 0 {
		msg, _ := L.ToString(-1)
		t.Fatalf("pcall error: %s", msg)
	}
	result, _ := L.ToString(-1)
	fmt.Printf("Count hook yield: %s\n", result)
}
