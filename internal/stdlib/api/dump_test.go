package api

import (
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

func TestStringDumpRoundtrip(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)

	code := `
print("testing string.dump")

-- Basic roundtrip
local f = function() return 42 end
local d = string.dump(f)
assert(type(d) == "string")
assert(#d > 0)
assert(d:byte(1) == 27)
print("dump ok, size=" .. #d)

-- Load binary back
local f2 = load(d, nil, "b")
assert(f2, "load binary failed")
assert(f2() == 42, "roundtrip value mismatch")
print("basic roundtrip ok")

-- Mode "t" rejects binary
local ok, err = load(d, "test", "t")
assert(not ok)
assert(string.find(err, "binary chunk"), "expected binary chunk error, got: " .. tostring(err))
print("mode t rejection ok")

-- pcall(string.dump, print) should fail (C function)
local ok2, err2 = pcall(string.dump, print)
assert(not ok2, "dumping C function should fail")
print("C function rejection ok")

-- Roundtrip with arguments and returns
local h = function(a, b) return a + b, a - b end
local dh = string.dump(h)
local h2 = load(dh, nil, "b")
local r1, r2 = h2(10, 3)
assert(r1 == 13 and r2 == 7, "multi-return roundtrip failed: " .. tostring(r1) .. ", " .. tostring(r2))
print("multi-arg roundtrip ok")

print("all string.dump tests passed")
`
	err := L.DoString(code)
	if err != nil {
		t.Fatalf("FAIL: %v", err)
	}
}
