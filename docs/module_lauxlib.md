# lauxlib 模块规格书

## 模块职责

Lua 辅助库（lauxlib）提供 C API 的高级封装，简化常用操作。包括参数检查、内存分配辅助、栈操作辅助、错误处理等。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lapi | C API |
| lua.h | 类型定义 |

## 核心 API

### 1. 栈操作辅助

```go
// luaL_checkstack: 确保栈空间
func (L *LuaState) CheckStack(n int) {
    if !L.CheckStack(n) {
        L.Error("stack overflow")
    }
}

// luaL_checkany: 检查至少有一个值
func (L *LuaState) CheckAny(arg int) {
    if L.Type(arg) == LUA_TNONE {
        L.TypeError(arg, "value")
    }
}
```

### 2. 类型检查

```go
// luaL_checktype: 严格类型检查
func (L *LuaState) CheckType(arg int, t LuaType) {
    if L.Type(arg) != t {
        L.TypeError(arg, L.TypeName(t))
    }
}

// luaL_checkinteger: 获取整数参数
func (L *LuaState) CheckInteger(arg int) int64 {
    if !L.IsInteger(arg) {
        L.TypeError(arg, "integer")
    }
    return L.ToInteger(arg)
}

// luaL_checknumber: 获取数值参数
func (L *LuaState) CheckNumber(arg int) float64 {
    if !L.IsNumber(arg) {
        L.TypeError(arg, "number")
    }
    return L.ToNumber(arg)
}

// luaL_checkstring: 获取字符串参数
func (L *LuaState) CheckString(arg int) string {
    if !L.IsString(arg) {
        L.TypeError(arg, "string")
    }
    return L.ToString(arg)
}

// luaL_checklstring: 获取字符串+长度
func (L *LuaState) CheckLString(arg int) (string, int) {
    return L.CheckString(arg), int(L.RawLen(arg))
}

// luaL_checkint: int 类型
func (L *LuaState) CheckInt(arg int) int {
    return int(L.CheckInteger(arg))
}

// luaL_checklong: long 类型
func (L *LuaState) CheckLong(arg int) int64 {
    return L.CheckInteger(arg)
}
```

### 3. 可选参数

```go
// luaL_optinteger: 可选整数
func (L *LuaState) OptInteger(arg int, def int64) int64 {
    if L.IsNone(arg) || L.IsNil(arg) {
        return def
    }
    return L.CheckInteger(arg)
}

// luaL_optnumber: 可选数值
func (L *LuaState) OptNumber(arg int, def float64) float64 {
    if L.IsNone(arg) || L.IsNil(arg) {
        return def
    }
    return L.CheckNumber(arg)
}

// luaL_optstring: 可选字符串
func (L *LuaState) OptString(arg int, def string) string {
    if L.IsNone(arg) || L.IsNil(arg) {
        return def
    }
    return L.CheckString(arg)
}
```

### 4. 表操作

```go
// luaL_createtable: 创建表（带预分配）
func (L *LuaState) CreateTable(narr, nrec int) {
    L.CreateTable(narr, nrec)
}

// luaL_newlibtable: 创建库表（预分配）
func (L *LuaState) NewLibTable(lib []Reg) {
    L.CreateTable(0, len(lib))
}

// luaL_newlib: 创建库表并设置元表
func (L *LuaState) NewLib(lib []Reg) {
    L.NewLibTable(lib)
    L.SetMetatable(-1)
}

// luaL_setfuncs: 注册库函数
func (L *LuaState) SetFuncs(lib []Reg, nupvalue int) {
    for _, fn := range lib {
        if nupvalue > 0 {
            // 复制 upvalue
            L.PushValue(-nupvalue)
        } else {
            L.PushNil()
        }
        L.PushGoFunction(fn.Fn)
        L.SetField(-(nupvalue + 2), fn.Name)
    }
    L.Pop(nupvalue)
}
```

### 5. 字符串操作

```go
// luaL_gsub: 全局替换
func (L *LuaState) Gsub(s, p, r string) string {
    L.PushString(s)
    
    result := ""
    pos := 0
    
    for {
        idx := strings.Index(s[pos:], p)
        if idx == -1 {
            result += s[pos:]
            break
        }
        result += s[pos:pos+idx] + r
        pos += idx + len(p)
    }
    
    L.Pop(1)
    L.PushString(result)
    return result
}

// luaL_addchar: 添加字符到缓冲区
func (L *LuaState) AddChar(c byte) {
    // ...
}

// luaL_addstring: 添加字符串
func (L *LuaState) AddString(s string) {
    // ...
}

// luaL_pushresult: 完成缓冲区并压入
func (L *LuaState) PushResult() {
    // ...
}
```

### 6. 错误处理

```go
// luaL_argerror: 参数错误
func (L *LuaState) ArgError(arg int, msg string) {
    L.Pop(1)
    L.PushFString("bad argument #%d: %s", arg, msg)
    L.Error()
}

// luaL_typeerror: 类型错误
func (L *LuaState) TypeError(arg int, tname string) {
    L.PushFString("bad argument #%d: %s expected, got %s",
        arg, tname, L.TypeName(L.Type(arg)))
    L.Error()
}

// luaL_error: 一般错误
func (L *LuaState) Error(msg string) {
    L.PushString(msg)
    L.Error()
}

// luaL_fileresult: 文件操作结果
func (L *LuaState) FileResult(ok bool, filename string) int {
    if ok {
        L.PushBoolean(true)
        return 1
    }
    L.PushNil()
    L.PushString(filename)
    L.PushString(strerror())
    return 2
}
```

### 7. 文件和代码加载

```go
// luaL_loadfile: 加载文件
func (L *LuaState) LoadFile(filename string) error {
    if filename == "" {
        // stdin
        return L.Load(readStdin, nil, "=stdin", "t")
    }
    
    f, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer f.Close()
    
    return L.Load(func(L *LuaState) ([]byte, error) {
        return io.ReadAll(f)
    }, nil, "@"+filename, "t")
}

// luaL_loadstring: 加载字符串
func (L *LuaState) LoadString(s string) error {
    return L.Load(func(L *LuaState) ([]byte, error) {
        return []byte(s), nil
    }, nil, "=(load)", "t")
}

// luaL_dofile: 执行文件
func (L *LuaState) DoFile(filename string) error {
    if err := L.LoadFile(filename); err != nil {
        return err
    }
    L.Call(0, LUA_MULTRET)
    return nil
}

// luaL_dostring: 执行字符串
func (L *LuaState) DoString(s string) error {
    if err := L.LoadString(s); err != nil {
        return err
    }
    L.Call(0, LUA_MULTRET)
    return nil
}
```

### 8. Buffer

```go
// luaL_Buffer 字符串缓冲区
type Buffer struct {
    L       *LuaState
    Size    int
    N       int
    Init    [LUAL_BUFFERSIZE]byte
    Data    []byte
}

const LUAL_BUFFERSIZE = 1024

func (L *LuaState) NewBuffer() *Buffer {
    return &Buffer{L: L}
}

func (b *Buffer) AddChar(c byte) {
    if b.N >= b.Size {
        b.AddSize()
    }
    b.Data[b.N] = c
    b.N++
}

func (b *Buffer) AddString(s string) {
    for _, c := range s {
        b.AddChar(byte(c))
    }
}

func (b *Buffer) PushResult() {
    b.L.PushString(string(b.Data[:b.N]))
}
```

## Reg 结构

```go
// luaL_Reg 函数注册表
type Reg struct {
    Name string
    Fn  GoFunction
}

// 示例
var mathLib = []lua.Reg{
    {"abs", abs},
    {"sin", sin},
    {"cos", cos},
    {"tan", tan},
    {"floor", floor},
    {"ceil", ceil},
    {"sqrt", sqrt},
    {"pow", pow},
    {"log", log},
    {"exp", exp},
}

// 注册库
func OpenMath(L *LuaState) int {
    L.NewLib(mathLib)
    return 1
}
```

## 错误码

```go
const (
    LUA_ERRRUN    = 1
    LUA_ERRMEM    = 2
    LUA_ERRERR    = 3
    LUA_ERRSYNTAX = 4
    LUA_ERRFILE   = 5
)
```

## 验证测试

```go
func TestAuxLib(t *testing.T) {
    L := NewState()
    defer L.Close()
    
    // 测试 Check 函数
    L.PushInteger(42)
    assert.Equal(t, int64(42), L.CheckInteger(1))
    
    // 测试 Opt 函数
    L.Pop(1)
    L.PushNil()
    assert.Equal(t, int64(10), L.OptInteger(1, 10))
    
    // 测试错误
    assert.Panics(t, func() {
        L.CheckString(1)  // nil 不是字符串
    })
    
    // 测试 Buffer
    buf := L.NewBuffer()
    buf.AddString("hello")
    buf.AddString(" world")
    buf.PushResult()
    assert.Equal(t, "hello world", L.ToString(-1))
}
```