package integration

import (
	"testing"
	"github.com/akzj/go-lua/state"
)

// TestSimpleUpval1 最简单的 upvalue 测试：闭包返回捕获的变量
func TestSimpleUpval1(t *testing.T) {
	code := `
local x = 10
local function f()
    return x
end
assert(f() == 10, "should capture x=10")
`
	err := state.DoString(code)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
}

// TestSimpleUpval2 测试 SETUPVAL - 修改 upvalue
func TestSimpleUpval2(t *testing.T) {
	code := `
local x = 10
local function f()
    x = 20
end
f()
assert(x == 20, "should modify x to 20")
`
	err := state.DoString(code)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
}

// TestSimpleUpval3 测试闭包返回捕获的变量
func TestSimpleUpval3(t *testing.T) {
	code := `
local x = 10
local function f()
    return x
end
local result = f()
assert(result == 10, "closure should return x=10, got " .. tostring(result))
`
	err := state.DoString(code)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
}

// TestSimpleUpval4 测试闭包修改并返回 upvalue
func TestSimpleUpval4(t *testing.T) {
	code := `
local x = 10
local function f()
    x = x + 5
    return x
end
assert(f() == 15, "first call should return 15")
assert(f() == 20, "second call should return 20")
`
	err := state.DoString(code)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
}
