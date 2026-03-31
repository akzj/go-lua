# lapi 模块规格书

## 模块职责

Lua 的公开 C API 实现（1478行）。提供 C 程序与 Lua 交互的接口，包括栈操作、类型查询、函数调用等。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lstate | lua_State |
| lvm | luaV_execute |
| lfunc | luaF_newCclosure |
| lgc | luaC_checkGC |
| lauxlib | 辅助宏 |

## 核心 API 分类

### 1. 状态管理

```go
// lua_newstate: 创建新状态
func NewState(alloc AllocFunc, ud interface{}) *LuaState

// lua_close: 关闭状态
func (L *LuaState) Close()

// lua_newthread: 创建新线程
func (L *LuaState) NewThread() *LuaState
```

### 2. 栈操作

```go
// lua_gettop: 获取栈顶索引
func (L *LuaState) GetTop() int

// lua_settop: 设置栈顶
func (L *LuaState) SetTop(idx int)

// lua_pushvalue: 压入值
func (L *LuaState) PushValue(idx int)

// lua_rotate: 旋转栈
func (L *LuaState) Rotate(idx, n int)

// lua_copy: 复制值
func (L *LuaState) Copy(fromidx, toidx int)

// lua_checkstack: 检查栈空间
func (L *LuaState) CheckStack(n int) bool
```

### 3. 类型查询

```go
// lua_type: 获取类型
func (L *LuaState) Type(idx int) LuaType

// lua_typename: 类型名
func (L *LuaState) TypeName(tp LuaType) string

// lua_is*: 类型检查
func (L *LuaState) IsNil(idx int) bool
func (L *LuaState) IsBoolean(idx int) bool
func (L *LuaState) IsNumber(idx int) bool
func (L *LuaState) IsString(idx int) bool
func (L *LuaState) IsTable(idx int) bool
func (L *LuaState) IsFunction(idx int) bool
func (L *LuaState) IsCFunction(idx int) bool
func (L *LuaState) IsUserdata(idx int) bool
func (L *LuaState) IsThread(idx int) bool
```

### 4. 值转换

```go
// lua_to*: 值转换
func (L *LuaState) ToBoolean(idx int) bool
func (L *LuaState) ToNumber(idx int) float64
func (L *LuaState) ToInteger(idx int) int64
func (L *LuaState) ToString(idx int) string
func (L *LuaState) ToPointer(idx int) unsafe.Pointer
func (L *LuaState) ToThread(idx int) *LuaState
func (L *LuaState) ToUserData(idx int) unsafe.Pointer

// lua_tonumberx: 安全数值转换
func (L *LuaState) ToNumberX(idx int) (float64, bool)
func (L *LuaState) ToIntegerX(idx int) (int64, bool)
```

### 5. 压入值

```go
// lua_push*: 压入值
func (L *LuaState) PushNil()
func (L *LuaState) PushBoolean(b bool)
func (L *LuaState) PushInteger(n int64)
func (L *LuaState) PushNumber(n float64)
func (L *LuaState) PushString(s string)
func (L *LuaState) PushCFunction(f GoFunction)
func (L *LuaState) PushCClosure(f GoFunction, n int)
func (L *LuaState) PushLightUserData(p unsafe.Pointer)
func (L *LuaState) PushThread() bool
```

### 6. 表操作

```go
// 创建
func (L *LuaState) NewTable()
func (L *LuaState) CreateTable(narr, nrec int)

// 访问
func (L *LuaState) GetTable(idx int) LuaType
func (L *LuaState) GetField(idx int, k string) LuaType
func (L *LuaState) GetI(idx int, n int64) LuaType
func (L *LuaState) RawGet(idx int) LuaType
func (L *LuaState) RawGetI(idx, n int) LuaType

// 设置
func (L *LuaState) SetTable(idx int)
func (L *LuaState) SetField(idx int, k string)
func (L *LuaState) SetI(idx int, n int64)
func (L *LuaState) RawSet(idx int)
func (L *LuaState) RawSetI(idx, n int)
```

### 7. 元表

```go
func (L *LuaState) GetMetatable(idx int) bool
func (L *LuaState) SetMetatable(idx int) int
```

### 8. 调用

```go
// lua_call: 调用函数
func (L *LuaState) Call(nargs, nresults int)

// lua_pcall: 保护调用
func (L *LuaState) PCall(nargs, nresults, errfunc int) error

// lua_load: 加载代码
func (L *LuaState) Load(reader Reader, data interface{}, chunkname, mode string) error

// lua_dump: 导出字节码
func (L *LuaState) Dump(writer Writer, data interface{}, strip int) int
```

### 9. 协程

```go
func (L *LuaState) Resume(from *LuaState, nargs int) ([]Value, Status, error)
func (L *LuaState) Yield(nresults int) int
func (L *LuaState) IsYieldable() bool
```

### 10. GC

```go
func (L *LuaState) GC(what int, args ...int) int
```

## Go 重写规格

```go
package lua

// GoFunction Go 函数类型
type GoFunction func(*LuaState) int

// LuaState 公开 API
type LuaState struct {
    // 内部状态
    internal *luaInternal
}

// lua_newstate 等价
func NewState() *LuaState {
    return NewStateWithAlloc(defaultAlloc, nil)
}

func NewStateWithAlloc(alloc AllocFunc, ud interface{}) *LuaState {
    L := newStateInternal(alloc, ud)
    // 初始化标准库
    Openlibs(L)
    return L
}

// lua_gettop
func (L *LuaState) GetTop() int {
    L.Lock()
    defer L.Unlock()
    return L.internal.GetTop()
}

// lua_settop
func (L *LuaState) SetTop(idx int) {
    L.Lock()
    defer L.Unlock()
    L.internal.SetTop(idx)
}

// lua_pushnil
func (L *LuaState) PushNil() {
    L.Lock()
    defer L.Unlock()
    L.internal.PushNil()
}

// lua_pushinteger
func (L *LuaState) PushInteger(n int64) {
    L.Lock()
    defer L.Unlock()
    L.internal.PushInteger(n)
}

// lua_pushstring
func (L *LuaState) PushString(s string) {
    L.Lock()
    defer L.Unlock()
    L.internal.PushString(s)
}

// lua_pushcfunction
func (L *LuaState) PushCFunction(f GoFunction) {
    L.Lock()
    defer L.Unlock()
    L.internal.PushCFunction(f)
}

// lua_pushcclosure
func (L *LuaState) PushCClosure(f GoFunction, n int) {
    L.Lock()
    defer L.Unlock()
    L.internal.PushCClosure(f, n)
}

// lua_call
func (L *LuaState) Call(nargs, nresults int) {
    L.Lock()
    defer L.Unlock()
    
    funcIdx := L.internal.top - nargs - 1
    L.internal.Call(funcIdx, nresults)
}

// lua_pcall
func (L *LuaState) PCall(nargs, nresults, errfunc int) error {
    L.Lock()
    defer L.Unlock()
    
    funcIdx := L.internal.top - nargs - 1
    err := L.internal.PCall(funcIdx, nresults, errfunc)
    return err
}

// lua_gettable
func (L *LuaState) GetTable(idx int) LuaType {
    L.Lock()
    defer L.Unlock()
    
    t := L.internal.index2addr(idx)
    k := L.internal.stack[L.internal.top-1]
    L.internal.top--
    
    return L.internal.getTable(t, &k)
}

// lua_settable
func (L *LuaState) SetTable(idx int) {
    L.Lock()
    defer L.Unlock()
    
    t := L.internal.index2addr(idx)
    k := L.internal.stack[L.internal.top-2]
    v := L.internal.stack[L.internal.top-1]
    L.internal.top -= 2
    
    L.internal.setTable(t, &k, &v)
}

// lua_newtable
func (L *LuaState) NewTable() {
    L.Lock()
    defer L.Unlock()
    
    L.internal.NewTable()
    L.internal.CheckGC()
}

// lua_createtable
func (L *LuaState) CreateTable(narr, nrec int) {
    L.Lock()
    defer L.Unlock()
    
    L.internal.CreateTable(narr, nrec)
    L.internal.CheckGC()
}

// lua_load
func (L *LuaState) Load(reader Reader, data interface{}, chunkname, mode string) error {
    L.Lock()
    defer L.Unlock()
    
    return L.internal.Load(reader, data, chunkname, mode)
}

// lua_resume
func (L *LuaState) Resume(from *LuaState, nargs int) ([]Value, Status, error) {
    // 线程安全处理
    return L.internal.Resume(from.internal, nargs)
}

// lua_yield
func (L *LuaState) Yield(nresults int) int {
    return L.internal.Yield(nresults)
}

// lua_close
func (L *LuaState) Close() {
    L.Lock()
    defer L.Unlock()
    
    L.internal.Close()
}

// lua_newthread
func (L *LuaState) NewThread() *LuaState {
    L.Lock()
    defer L.Unlock()
    
    t := L.internal.NewThread()
    return wrapState(t)
}
```

### 索引转换

```go
// lua_absindex: 转换索引为绝对索引
func (L *LuaState) absIndex(idx int) int {
    if idx > 0 || idx <= LUA_REGISTRYINDEX {
        return idx
    }
    base := L.internal.ci.Base()
    return base + idx + 1
}

// luaL_checkstack 内部使用
func (L *LuaState) index2addr(idx int) *TValue {
    if idx > 0 {
        base := L.internal.ci.Base()
        return &L.internal.stack[base+idx-1]
    } else if idx <= LUA_REGISTRYINDEX {
        if idx == LUA_REGISTRYINDEX {
            return &L.internal.G.Registry
        }
        // upvalue
    }
    return &L.internal.stack[L.internal.top+idx]
}
```

### 函数注册

```go
// RegisterGoFunction: 注册 Go 函数到全局表
func (L *LuaState) Register(name string, f GoFunction) {
    L.PushCFunction(f)
    L.SetGlobal(name)
}

// 注册多个函数
type Reg struct {
    Name string
    Fn   GoFunction
}

func (L *LuaState) RegisterFunctions(regs []Reg) {
    L.PushGlobalTable()
    for _, r := range regs {
        L.PushCFunction(r.Fn)
        L.SetField(-2, r.Name)
    }
    L.Pop(1)
}
```

## 陷阱和注意事项

### 陷阱 1: 栈索引 vs API 索引

```go
// API 索引从 1 开始
// 栈顶是 lua_gettop()
// 正索引从 1 开始，负索引从 -1 开始

// 错误示例
L.stack[L.GetTop()]  // ❌

// 正确示例
L.stack[L.GetTop()-1]  // ✅
// 或者
L.Get(-1)  // ✅
```

### 陷阱 2: 多返回值

```go
// lua_call 不处理多返回值
// 结果数量由 nresults 控制

// lua_pcallk 支持 continuation
```

### 陷阱 3: C 函数返回值

```go
// C 函数返回压入栈的返回值数量
// 不是返回值本身

func myFunction(L *LuaState) int {
    L.PushInteger(42)  // 压入返回值
    return 1           // 返回值数量
}
```

### 陷阱 4: Userdata

```go
// Light userdata: 直接指针
// Full userdata: 带元表和用户值
```

## lauxlib 辅助库

```go
package lua

// lauxlib 提供的高级 API

// CheckString: 获取字符串参数
func (L *LuaState) CheckString(arg int) string {
    if !L.IsString(arg) {
        L.TypeError(arg, "string")
    }
    return L.ToString(arg)
}

// OptString: 获取可选字符串
func (L *LuaState) OptString(arg int, def string) string {
    if L.IsNil(arg) {
        return def
    }
    return L.CheckString(arg)
}

// CheckInt: 获取整数参数
func (L *LuaState) CheckInt(arg int) int64 {
    if !L.IsNumber(arg) {
        L.TypeError(arg, "number")
    }
    return L.ToInteger(arg)
}

// LoadFile: 加载文件
func (L *LuaState) LoadFile(filename string) error {
    f, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer f.Close()
    return L.Load(func(L *LuaState) ([]byte, error) {
        return io.ReadAll(f)
    }, nil, filename, "bt")
}

// DoFile: 执行文件
func (L *LuaState) DoFile(filename string) error {
    if err := L.LoadFile(filename); err != nil {
        return err
    }
    L.Call(0, LUA_MULTRET)
    return nil
}

// DoString: 执行字符串
func (L *LuaState) DoString(chunk string) error {
    return L.Load(func(L *LuaState) ([]byte, error) {
        return []byte(chunk), nil
    }, nil, "=(load)", "t")
}
```

## 验证测试

```go
func TestLuaAPI(t *testing.T) {
    L := NewState()
    defer L.Close()
    
    // 栈操作
    L.PushInteger(1)
    L.PushString("hello")
    L.PushBoolean(true)
    assert.Equal(t, 3, L.GetTop())
    
    // 类型检查
    assert.True(t, L.IsInteger(-3))
    assert.True(t, L.IsString(-2))
    assert.True(t, L.IsBoolean(-1))
    
    // 值获取
    assert.Equal(t, int64(1), L.ToInteger(-3))
    assert.Equal(t, "hello", L.ToString(-2))
    
    // 表操作
    L.NewTable()
    L.PushString("key")
    L.PushInteger(100)
    L.SetTable(-3)
    
    // 函数调用
    L.GetGlobal("print")
    L.PushString("test")
    L.Call(1, 0)
}
```