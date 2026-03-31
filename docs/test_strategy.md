# Lua 5.5 测试策略

## 1. 测试套件结构

```
lua-master/testes/
├── 核心测试
│   ├── literals.lua      - 字面量测试
│   ├── main.lua          - 主测试入口
│   ├── events.lua        - 事件/hook 测试
│   ├── verybig.lua       - 大数据测试
│   ├── trace-cov.lua     - 覆盖率追踪
│   ├── db.lua            - 调试库测试
│   ├── errors.lua         - 错误处理测试
│   ├── math.lua          - 数学库测试
│   ├── strings.lua       - 字符串库测试
│   ├── tables.lua        - 表测试
│   ├── bitwise.lua       - 位运算测试
│   ├── sort.lua          - 排序测试
│   ├── api.lua           - C API 测试
│   ├── calls.lua         - 函数调用测试
│   ├── coros.lua         - 协程测试
│   ├── constructs.lua    - 控制流测试
│   ├── closure.lua       - 闭包测试
│   ├── gc.lua            - GC 测试
│   ├── nextvar.lua       - 变量测试
│   ├── pak.lua           - 包/require 测试
│   ├── pm.lua            - 模式匹配测试
│   ├── rebuild.lua       - 重建测试
│   ├── utf8.lua          - UTF8 测试
│   └── loclis.lua        - 局部函数测试
│
├── 测试工具
│   ├── readonly.lua      - 只读测试
│   ├── verysmall.lua     - 微小测试
│   ├── utffiles.lua      - UTF 文件测试
│   └──五人帮/
│       └──五人帮.lua     - 中文测试
│
└── 独立可执行文件
    └──宝宝.lua           - 独立测试
```

## 2. 模块到测试的映射

| 模块 | 对应测试文件 | 测试内容 |
|------|-------------|----------|
| **lobject** | (基础) | 所有测试都依赖类型系统 |
| **lstring** | strings.lua | 字符串操作、模式匹配、UTF8 |
| **ltable** | tables.lua, sort.lua | 表操作、迭代、排序 |
| **lfunc/closure** | closure.lua | 闭包、upvalue、变参 |
| **lvm** | literals.lua, constructs.lua | 字节码执行、语法构造 |
| **ldo** | calls.lua, errors.lua | 函数调用、错误处理、尾调用 |
| **lgc** | gc.lua | 垃圾回收、弱表、终结器 |
| **lcorolib** | coros.lua | 协程创建、yield、resume |
| **lmathlib** | math.lua | 数学函数 |
| **lbaselib** | main.lua | type(), pairs(), ipairs(), etc. |
| **liolib** | (外部 IO 测试) | 文件操作 |
| **lapi** | api.lua | C API 完整性 |
| **ldebug** | db.lua, events.lua | hook、debug info |

## 3. 每个模块的验证方案

### 阶段 1：lobject 类型系统

```lua
-- strings.lua 核心测试
-- 验证类型标签正确
assert(type(nil) == "nil")
assert(type(true) == "boolean")
assert(type(1) == "number")
assert(type("") == "string")
assert(type({}) == "table")
assert(type(function() end) == "function")
assert(type(coroutine.create(function() end)) == "thread")

-- 数值类型区分
assert(1 == 1.0)           -- 相同值
assert(math.type(1) == "integer")
assert(math.type(1.0) == "float")

-- 字符串操作
assert("hello" .. " world" == "hello world")
assert(#"hello" == 5)
```

**Go 验证代码：**
```go
func TestTypeSystem(t *testing.T) {
    L := NewLuaState()
    
    // nil
    L.PushNil()
    assert.Equal(t, LUA_TNIL, L.Type(-1))
    
    // boolean
    L.PushBoolean(true)
    assert.Equal(t, LUA_TBOOLEAN, L.Type(-1))
    assert.True(t, L.ToBoolean(-1))
    
    // integer
    L.PushInteger(42)
    assert.Equal(t, LUA_TNUMBER, L.Type(-1))
    assert.Equal(t, int64(42), L.ToInteger(-1))
    
    // float
    L.PushNumber(3.14)
    assert.Equal(t, LUA_TNUMBER, L.Type(-1))
    assert.InDelta(t, 3.14, L.ToNumber(-1), 0.001)
    
    // string
    L.PushString("hello")
    assert.Equal(t, LUA_TSTRING, L.Type(-1))
    assert.Equal(t, "hello", L.ToString(-1))
}
```

### 阶段 2：lstring

```lua
-- strings.lua 测试覆盖
local s = "Hello, World!"

-- 长度
assert(#s == 13)
assert(string.len(s) == 13)

-- 子串
assert(string.sub(s, 1, 5) == "Hello")
assert(string.sub(s, -6) == "World!")

-- 查找
assert(string.find(s, "World") == 8)
assert(string.find(s, "xyz") == nil)

-- 模式匹配
assert(string.match("hello123", "%d+") == "123")
assert(string.gsub("hello", "l", "L") == "heLLo")

-- 大小写
assert(string.upper("hello") == "HELLO")
assert(string.lower("HELLO") == "hello")

-- 格式化和转换
assert(string.format("%.2f", 3.14159) == "3.14")
assert(tonumber("42") == 42)
assert(tostring(42) == "42")

-- UTF8 (utf8.lua)
assert(utf8.len("你好") == 2)
assert(utf8.codepoint("A") == 65)
```

### 阶段 3：ltable

```lua
-- tables.lua 测试覆盖
local t = {1, 2, 3, a = "alpha", b = "beta"}

-- 长度
assert(#t == 3)
assert(t.a == "alpha")

-- 插入删除
table.insert(t, 4)
assert(t[4] == 4)
table.remove(t, 2)
assert(t[2] == 3)

-- 迭代
local sum = 0
for i, v in ipairs(t) do sum = sum + v end
assert(sum == 8)

-- next
for k, v in pairs(t) do
    -- k, v 应该被赋值
end

-- 表构造器
local t2 = {x = 1, y = 2, [1] = "one", [2] = "two"}
assert(t2.x == 1)
assert(t2[1] == "one")
```

### 阶段 4：lfunc/closure

```lua
-- closure.lua 测试覆盖

-- 基本闭包
local function counter()
    local count = 0
    return function()
        count = count + 1
        return count
    end
end
local c1, c2 = counter(), counter()
assert(c1() == 1)
assert(c1() == 2)
assert(c2() == 1)  -- 独立闭包

-- upvalue 正确性
local x = 10
local function getx() return x end
local function setx(v) x = v end
assert(getx() == 10)
setx(20)
assert(getx() == 20)

-- 变参
local function sum(...)
    local s = 0
    for _, v in ipairs({...}) do s = s + v end
    return s
end
assert(sum(1, 2, 3) == 6)
assert(sum()) == 0

-- 尾调用
local function tail(n)
    if n <= 1 then return n end
    return tail(n - 1)  -- 不消耗栈
end
assert(tail(1000000) == 1)
```

### 阶段 5：ldo/调用

```lua
-- calls.lua 测试覆盖

-- 函数调用
assert(math.max(1, 2, 3) == 3)

-- 多返回值
local function multi()
    return 1, 2, 3
end
local a, b, c = multi()
assert(a == 1 and b == 2 and c == 3)

-- pcall
local ok, err = pcall(error, "test error")
assert(not ok)
assert(err == "test error")

-- xpcall
local ok, msg = xpcall(
    function() error("msg") end,
    function(e) return "caught: " .. e end
)
assert(not ok)
assert(msg == "caught: msg")

-- 尾调用（不消耗栈空间）
local depth = 0
local function recurse(n)
    depth = depth + 1
    if n > 0 then return recurse(n - 1) end
    return depth
end
assert(recurse(10000) > 1000)  -- 不应该栈溢出
```

### 阶段 6：协程

```lua
-- coros.lua 测试覆盖

-- 基本协程
local co = coroutine.create(function(a, b)
    assert(a == 1 and b == 2)
    local r = coroutine.yield(3)
    return r
end)

local ok, val = coroutine.resume(co, 1, 2)
assert(ok and val == 3)

ok, val = coroutine.resume(co, 4)
assert(ok and val == 4)
assert(coroutine.status(co) == "dead")

-- 嵌套协程
local function producer()
    for i = 1, 5 do
        coroutine.yield(i)
    end
end

local function consumer(prod)
    local sum = 0
    while true do
        local ok, val = coroutine.resume(prod)
        if not ok then break end
        sum = sum + val
    end
    return sum
end

local p = coroutine.create(producer)
assert(consumer(p) == 15)

-- wrap
local wrap = coroutine.wrap(function()
    coroutine.yield(1)
    coroutine.yield(2)
end)
assert(wrap() == 1)
assert(wrap() == 2)
```

### 阶段 7：lgc/垃圾回收

```lua
-- gc.lua 测试覆盖

-- 弱表
local weak = setmetatable({}, {__mode = "v"})
local t = {}
weak[1] = t  -- t 现在是弱引用
t = nil
collectgarbage()
-- weak[1] 应该是 nil
assert(next(weak) == nil)

-- 终结器
local finalized = false
local obj = newproxy(true)
getmetatable(obj).__gc = function()
    finalized = true
end
obj = nil
collectgarbage()
collectgarbage()  -- 可能需要两次 GC
assert(finalized)

-- 手动 GC 控制
collectgarbage("stop")
local before = collectgarbage("count")
collectgarbage("restart")
collectgarbage("collect")
local after = collectgarbage("count")
assert(after <= before * 1.1)  -- 应该不会大幅增长
```

### 阶段 8：完整解释器

```bash
# 运行官方测试套件
cd lua-master
make clean && make

# 运行核心测试
lua testes/main.lua

# 运行单个测试
lua testes/tables.lua
lua testes/closure.lua
lua testes/coros.lua
```

## 4. 回归测试策略

### 4.1 单元测试级别

为每个 Go 模块编写单元测试：

```go
// lua/lobject_test.go
package lua

func TestTValueBasic(t *testing.T) {
    tests := []struct {
        push    interface{}
        expType int
    }{
        {nil, LUA_TNIL},
        {true, LUA_TBOOLEAN},
        {false, LUA_TBOOLEAN},
        {42, LUA_TNUMBER},
        {3.14, LUA_TNUMBER},
        {"hello", LUA_TSTRING},
        {make([]interface{}, 0), LUA_TTABLE},
        {func() {}, LUA_TFUNCTION},
    }
    
    for _, tt := range tests {
        L := NewState(10)
        switch v := tt.push.(type) {
        case nil:
            L.PushNil()
        case bool:
            L.PushBoolean(v)
        case int:
            L.PushInteger(int64(v))
        case float64:
            L.PushNumber(v)
        case string:
            L.PushString(v)
        }
        
        if L.Type(-1) != tt.expType {
            t.Errorf("type mismatch: got %d, want %d", L.Type(-1), tt.expType)
        }
    }
}
```

### 4.2 集成测试级别

运行 Lua 官方测试：

```go
// lua/integration_test.go
package lua

import (
    "os/exec"
    "testing"
)

func TestLuaTestSuite(t *testing.T) {
    // 确保 Go 实现能运行 Lua 测试文件
    // 需要先完成 lapi 的完整实现
}
```

### 4.3 模糊测试

```go
func FuzzLuaExecution(f *testing.F) {
    // 用随机 Lua 代码测试解释器稳定性
}
```

## 5. 性能基准

```lua
-- bench.lua
local function benchmark(name, fn, iterations)
    local start = os.clock()
    for i = 1, iterations do
        fn()
    end
    local elapsed = os.clock() - start
    print(string.format("%s: %.3f seconds", name, elapsed))
end

-- 表操作
benchmark("table insert", function()
    local t = {}
    for i = 1, 10000 do
        table.insert(t, i)
    end
end, 100)

-- 字符串拼接
benchmark("string concat", function()
    local s = ""
    for i = 1, 1000 do
        s = s .. "a"
    end
end, 100)

-- 函数调用
benchmark("function call", function()
    local function f() return 1 end
    for i = 1, 100000 do
        f()
    end
end, 100)

-- 闭包
benchmark("closure", function()
    local function make_counter()
        local c = 0
        return function()
            c = c + 1
            return c
        end
    end
    local c = make_counter()
    for i = 1, 10000 do
        c()
    end
end, 100)
```

## 6. 关键边界测试

```lua
-- 边界测试

-- 栈边界
local stk = {}
for i = 1, 1000000 do stk[i] = i end
assert(#stk == 1000000)

-- 递归深度
local function deep(n)
    if n <= 0 then return 0 end
    return 1 + deep(n - 1)
end
assert(deep(10000) == 10000)

-- 大整数
local large = 9007199254740992  -- 2^53
assert(large + 1 == large)  -- 精度丢失

-- 空表/空字符串
assert(#{} == 0)
assert(#"" == 0)
assert(next({}) == nil)
assert(next("") == nil)

-- 元表链
local mt1 = {__add = function() return "add" end}
local mt2 = {__add = function() return "mt2" end}
local t1 = setmetatable({}, mt1)
local t2 = setmetatable({}, mt2)
-- 元表不参与相加，只参与元方法查找
```

## 7. 测试执行顺序

```
第 1 轮：lobject, lmem
  → 运行: literals.lua (基础类型)

第 2 轮：lstring, ltable
  → 运行: strings.lua, tables.lua

第 3 轮：lfunc, closure
  → 运行: closure.lua

第 4 轮：lvm, ldo
  → 运行: calls.lua, constructs.lua

第 5 轮：lgc
  → 运行: gc.lua

第 6 轮：协程
  → 运行: coros.lua

第 7 轮：lapi
  → 运行: api.lua

第 8 轮：完整测试套件
  → 运行: testes/main.lua
```