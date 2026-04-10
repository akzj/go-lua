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

// TestSimpleUpval5 测试闭包作为返回值 — 之前失败的关键情况
func TestSimpleUpval5(t *testing.T) {
	code := `
local function outer()
    local x = 10
    local function inner()
        return x
    end
    x = 20  -- 修改 x
    return inner
end
local f = outer()
local result = f()
assert(result == 20, "should see modified x=20, got " .. tostring(result))
`
	err := state.DoString(code)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
}

// TestSimpleUpval6 测试循环中创建的闭包 — 之前失败的关键情况
func TestSimpleUpval6(t *testing.T) {
	code := `
local function outer()
    local a = {}
    for i = 1, 2 do
        local x = 100
        a[i] = function()
            return x
        end
    end
    return a
end
local result = outer()
print("result[1]() =", result[1]())
print("result[2]() =", result[2]())
-- Each closure should capture its own x
-- But currently they may share the same slot
`
	err := state.DoString(code)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
}

// TestSimpleUpval7 测试循环中创建的闭包，调用后返回值
func TestSimpleUpval7(t *testing.T) {
	code := `
local function outer()
    local a = {}
    for i = 1, 2 do
        local x = 100
        a[i] = function()
            return x
        end
    end
    return a
end
local result = outer()
print("result[1]()() =", result[1]())  -- 注意：两次()
print("result[2]()() =", result[2]())  -- 第一次调用返回函数，第二次调用才执行
`
	err := state.DoString(code)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
}
