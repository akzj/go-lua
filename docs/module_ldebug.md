# ldebug 模块规格书

## 模块职责

调试接口。提供行号信息获取、变量名查找、hook 支持、栈追踪等调试功能。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lobject | 所有类型 |
| lstate | 线程状态 |
| lvm | 字节码执行 |
| lfunc | 局部变量 |

## 公开 API

```c
/* 行号信息 */
LUAI_FUNC int luaG_getfuncline (const Proto *p, int pc);
LUAI_FUNC void luaGcondebuglg (lua_State *L, Addrbuff *buff, int line);
LUAI_FUNC void luaG_debugline (lua_State *L, int line);

/* 错误生成 */
LUAI_FUNC l_noret luaG_errormsg (lua_State *L);
LUAI_FUNC l_noret luaG_runerror (lua_State *L, const char *fmt, ...);
LUAI_FUNC l_noret luaG_concaterror (lua_State *L, int first, int last);
LUAI_FUNC l_noret luaG_tointerror (lua_State *L, const TValue *o);

/* 变量信息 */
LUAI_FUNC const char *luaG_findvar (lua_State *L, int n, int *stage);

/* 调用信息 */
LUAI_FUNC int luaG_checkopenop (const Instruction *i, int op);
LUAI_FUNC void luaG_checkexecmatch (lua_State *L, int from, int to, int 
                                    int n, int m);
LUAI_FUNC int luaG_checkcode (const Proto *pt);

/* 行hook */
LUAI_FUNC int luaG_traceexec (lua_State *L, const Instruction *pc);
```

## Hook 类型

```go
package lua

/* Hook 类型 */
const (
    LUA_HOOKCALL   = 0  /* 调用hook */
    LUA_HOOKRET    = 1  /* 返回hook */
    LUA_HOOKTAILCALL = 2 /* 尾调用hook */
    LUA_HOOKLINE   = 3  /* 行hook */
    LUA_HOOKCOUNT  = 4  /* 计数hook */
    LUA_HOOKANY    = 5  /* 任意hook */
)

/* Hook 事件 */
type HookEvent int

const (
    HOOK_EVENT_CALL HookEvent = iota
    HOOK_EVENT_RET
    HOOK_EVENT_TAILRET
    HOOK_EVENT_LINE
    HOOK_EVENT_COUNT
)
```

## lua_Debug 结构

```go
// lua_Debug 调试信息
type lua_Debug struct {
    Event    int           /* hook 事件 */
    Name     string        /* 变量名 */
    NameWhat string        /* "global", "local", "method", ... */
    What     string        /* "Lua", "C", "main" */
    Source   string        /* 源码 */
    CurrentLine int        /* 当前行 */
    LineDefined int        /* 函数定义行 */
    LastLineDefined int    /* 函数结束行 */
    Nups    int            /* upvalue 数量 */
    Nparams int            /* 参数数量 */
    IsVararg bool          /* 是否可变参数 */
    IsTailCall bool        /* 是否有尾调用 */
    ShortSrc string        /* 源码缩写 */
    
    // 内部
    ICI     *CallInfo     /* 调用信息 */
}
```

## Go 重写规格

### Hook 设置

```go
// lua_Hook 函数类型
type HookFunc func(*LuaState, *lua_Debug)

// 设置 hook
func (L *LuaState) SetHook(f HookFunc, mask, count int) {
    L.Lock()
    defer L.Unlock()
    
    L.Hook = f
    L.HookMask = mask
    L.HookCount = count
    
    // 更新 CI 的 trap 标志
    for ci := L.CI; ci != nil; ci = ci.Previous {
        if ci.IsLua() {
            ci.Trap = 1
        }
    }
}

func (L *LuaState) GetHook() HookFunc {
    return L.Hook
}

func (L *LuaState) GetHookMask() int {
    return int(L.HookMask)
}

func (L *LuaState) GetHookCount() int {
    return L.HookCount
}
```

### 行号查找

```go
// luaG_getfuncline: 从 PC 获取行号
func (L *LuaState) GetFuncLine(pc int) int {
    ci := L.CI
    if ci == nil {
        return -1
    }
    
    p := ci.Cl().Proto
    if p == nil {
        return -1
    }
    
    // 查找包含 pc 的行
    for i := 0; i < len(p.LineInfo); i++ {
        if p.LineInfo[i] == pc {
            // 需要计算绝对行号
            return getline(p, i)
        }
    }
    
    // 二分查找绝对行号
    return luaG-getfuncline(p, pc)
}

// getline: 计算绝对行号
func getline(p *Proto, idx int) int {
    line := 0
    for i := 0; i < idx; i++ {
        line += int(p.LineInfo[i])
    }
    
    // 加上基行号
    for i := 0; i < len(p.AbsLineInfo); i++ {
        if p.AbsLineInfo[i].PC <= idx {
            line = p.AbsLineInfo[i].Line
        }
    }
    
    return line
}
```

### 变量查找

```go
// luaG_findvar: 查找变量
func (L *LuaState) FindVar(n int) (string, *TValue) {
    ci := L.CI
    
    // 先查局部变量
    for i := 0; i < ci.nLocals; i++ {
        if n == 0 {
            return ci.Locals[i].Name, &L.Stack[ci.Locals[i].Idx]
        }
        n--
    }
    
    // 再查 upvalue
    cl := ci.Cl()
    for i := 0; i < cl.Nupvalues; i++ {
        if n == 0 {
            return cl.Upvals[i].Name, cl.Upvals[i].Get()
        }
        n--
    }
    
    // 全局变量
    env := L.GetEnv()
    return "", nil
}
```

### 错误生成

```go
// luaG_runerror: 生成运行时错误
func (L *LuaState) RunError(fmt string, args ...interface{}) {
    msg := fmt.Sprintf(fmt, args...)
    
    ci := L.CI
    if ci != nil && ci.IsLua() {
        // 添加位置信息
        p := ci.Cl().Proto
        line := L.GetFuncLine(ci.SavedPC)
        src := "?"
        if p.Source != nil {
            src = p.Source.String()
        }
        msg = fmt.Sprintf("%s:%d: %s", src, line, msg)
    }
    
    L.ErrorObject.SetString(L.NewString(msg))
    L.Error()
}

// luaG_errormsg: 抛出错误
func (L *LuaState) ErrorMsg() {
    L.Error()
}
```

### 栈追踪

```go
// lua_getinfo: 获取函数信息
func (L *LuaState) GetInfo(what string, ar *lua_Debug) int {
    var f *TValue
    
    if what[0] == '>' {
        // 函数在栈上
        idx := L.absIndex(int(what[1]))
        f = &L.Stack[idx]
        what = what[2:]
    } else {
        f = L.Stack[L.CI.Func : L.CI.Func+1]
    }
    
    for i := 0; i < len(what); i++ {
        switch what[i] {
        case 'S':
            L.fillSourceInfo(f, ar)
        case 'l':
            L.fillLineInfo(f, ar)
        case 'u':
            L.fillFuncInfo(f, ar)
        case 'n':
            L.fillNameInfo(f, ar)
        case 'L':
            L.fillLinesInfo(f, ar)
        case 'f':
            // 函数
        case 'C':
            // C 函数
        }
    }
    
    return 1
}

func (L *LuaState) fillSourceInfo(f *TValue, ar *lua_Debug) {
    var proto *Proto
    if f.IsLClosure() {
        proto = f.AsLClosure().Proto
    }
    
    if proto != nil {
        if proto.Source != nil {
            ar.Source = proto.Source.String()
        }
        ar.LineDefined = proto.LineDefined
        ar.LastLineDefined = proto.LastLineDefined
        ar.What = "Lua"
    } else if f.IsCClosure() {
        ar.What = "C"
        ar.Source = "=[C]"
    } else {
        ar.What = "main"
        ar.Source = "=[Main]"
    }
}
```

## 陷阱和注意事项

### 陷阱 1: Hook 中的安全点

```go
// Hook 只能在安全点被调用
// 安全点：指令边界、函数调用前

// luaG_traceexec 在每条指令前调用
// 但只在 ci.Trap 标志设置时才真正调用
```

### 陷阱 2: 局部变量索引

```go
// lua_getlocal 的索引是相对于 CallInfo 的
// 不是绝对栈索引

func (L *LuaState) GetLocal(ar *lua_Debug, n int) string {
    ci := ar.ICI
    if ci == nil {
        return ""
    }
    
    // 从 FuncInfo 获取局部变量
    fs := ci.FS
    if fs == nil {
        return ""
    }
    
    return fs.GetLocalName(n, ci.SavedPC)
}
```

### 陷阱 3: 行号的相对性

```go
// LineInfo 存储的是相对行号变化
// 需要累积才能得到绝对行号
```

## 验证测试

```lua
-- debug hook
local count = 0
debug.sethook(function(e, l)
    count = count + 1
    print("line", l)
end, "l")

-- 设置行 hook
debug.sethook(print, "l")

-- 获取调用栈
function traceback()
    local level = 1
    while true do
        local info = debug.getinfo(level, "nSl")
        if not info then break end
        print(level, info.source, info.currentline)
        level = level + 1
    end
end
```