# 标准库模块规格书

## 概述

Lua 5.5 标准库包括 10 个模块，提供与 C 标准库类似的功能。

| 模块 | 文件 | 行数 | 功能 |
|------|------|------|------|
| **lbaselib** | lbaselib.c | 559 | 基础函数 |
| **lstrlib** | lstrlib.c | 1894 | 字符串操作 |
| **ltablib** | ltablib.c | 429 | 表操作 |
| **lmathlib** | lmathlib.c | 765 | 数学函数 |
| **liolib** | liolib.c | 841 | I/O 操作 |
| **loslib** | loslib.c | 432 | OS 操作 |
| **lcorolib** | lcorolib.c | 225 | 协程操作 |
| **ldblib** | ldblib.c | 477 | 调试接口 |
| **lutf8lib** | lutf8lib.c | 294 | UTF-8 支持 |
| **loadlib** | loadlib.c | 748 | 动态库加载 |

---

## 1. lbaselib — 基础库

### 函数列表

```go
var baseLib = []lua.Reg{
    {"assert",         assert},
    {"collectgarbage",  collectgarbage},
    {"dofile",          dofile},
    {"error",           error},
    {"getmetatable",    getmetatable},
    {"ipairs",          ipairs},
    {"load",            load},
    {"loadfile",        loadfile},
    {"next",            next},
    {"pairs",           pairs},
    {"pcall",           pcall},
    {"print",           print},
    {"rawequal",       rawequal},
    {"rawlen",          rawlen},
    {"rawget",          rawget},
    {"rawset",          rawset},
    {"select",          select},
    {"setmetatable",    setmetatable},
    {"tonumber",        tonumber},
    {"tostring",        tostring},
    {"type",            type_},
    {"xpcall",          xpcall},
    {"_G",              getglobalenv},
    {"_VERSION",        getversion},
    {"_LOADED",          getloaded},
    {"_PRELOAD",        getpreload},
}
```

### 关键实现

```go
// assert
func assert(L *LuaState) int {
    if !L.ToBoolean(1) {
        L.PushString(L.GetOptString(2, "assertion failed!"))
        L.Error()
    }
    return L.GetTop()  // 返回所有参数
}

// pcall
func pcall(L *LuaState) int {
    status := L.PCall(1, LUA_MULTRET, 0)
    if status != 0 {
        L.PushBoolean(false)
        L.Insert(-1)  // 移动错误消息
        return 2
    }
    L.Insert(L.NewBoolean(true), 1)
    return L.GetTop()
}

// select
func select(L *LuaState) int {
    if L.CheckString(1) == "#" {
        L.PushInteger(int64(L.GetTop() - 1))
        return 1
    }
    idx := L.CheckInteger(1)
    if idx < 0 {
        idx = L.GetTop() + idx + 1
    }
    L.PushInteger(int64(L.GetTop() - idx))
    return 1
}

// print
func print(L *LuaState) int {
    n := L.GetTop()
    for i := 1; i <= n; i++ {
        if i > 1 {
            fmt.Print("\t")
        }
        L.GetGlobal("tostring")
        L.PushValue(i)
        L.Call(1, 1)
        fmt.Print(L.CheckString(-1))
        L.Pop(1)
    }
    fmt.Println()
    return 0
}
```

---

## 2. lstrlib — 字符串库

### 函数列表

```go
var stringLib = []lua.Reg{
    {"byte",       byte},
    {"char",       char},
    {"dump",       dump},
    {"find",       find},
    {"format",     format},
    {"gmatch",     gmatch},
    {"gsub",       gsub},
    {"len",        len},
    {"lower",      lower},
    {"match",      match},
    {"rep",        rep},
    {"reverse",    reverse},
    {"sub",        sub},
    {"upper",      upper},
}
```

### 模式匹配

```go
// find
func find(L *LuaState) int {
    s := L.CheckString(1)
    pattern := L.CheckString(2)
    init := L.OptInteger(3, 1)
    
    // 编译模式
    regex := compilePattern(pattern)
    
    // 搜索
    match := regex.Find(s, init-1)
    if match == nil {
        L.PushNil()
        return 1
    }
    
    // 返回位置
    L.PushInteger(int64(match.Start + 1))
    L.PushInteger(int64(match.End))
    
    // 捕获组
    for _, cap := range match.Captures {
        if cap.Start >= 0 {
            L.PushString(s[cap.Start:cap.End])
        } else {
            L.PushNil()
        }
    }
    
    return 2 + len(match.Captures)
}

// gmatch 迭代器
type gmatchState struct {
    S        string
    P        *Pattern
    LastPos  int
}

func gmatch(L *LuaState) int {
    s := L.CheckString(1)
    p := compilePattern(L.CheckString(2))
    
    // 创建用户数据存储状态
    ud := L.NewUserData(sizeof(gmatchState))
    state := getGmatchState(ud)
    *state = gmatchState{S: s, P: p}
    
    L.PushGoFunction(gmatchIter)
    L.PushUserData(ud)
    return 1
}

func gmatchIter(L *LuaState) int {
    state := getGmatchState(L.CheckUserData(1))
    
    for state.LastPos < len(state.S) {
        match := state.P.Find(state.S, state.LastPos)
        if match == nil {
            return 0
        }
        state.LastPos = match.End
        
        // 返回捕获
        if len(match.Captures) == 0 {
            L.PushString(match.Str)
            return 1
        }
        for _, cap := range match.Captures {
            if cap.Start >= 0 {
                L.PushString(state.S[cap.Start:cap.End])
            } else {
                L.PushNil()
            }
        }
        return len(match.Captures)
    }
    return 0
}
```

---

## 3. ltablib — 表库

### 函数列表

```go
var tableLib = []lua.Reg{
    {"concat",  concat},
    {"insert",  insert},
    {"move",    move},
    {"pack",    pack},
    {"unpack",  unpack},
    {"remove",  remove},
    {"sort",    sort_},
}
```

### 实现

```go
// concat
func concat(L *LuaState) int {
    t := L.CheckTable(1)
    sep := L.OptString(2, "")
    i := L.OptInteger(3, 1)
    j := L.OptInteger(4, int64(t.Len()))
    
    if i > j {
        L.PushString("")
        return 1
    }
    
    var result bytes.Buffer
    for n := i; n <= j; n++ {
        if n > i {
            result.WriteString(sep)
        }
        L.PushInteger(n)
        L.GetTable(t)
        result.WriteString(L.CheckString(-1))
        L.Pop(1)
    }
    
    L.PushString(result.String())
    return 1
}

// insert (可变参数版本)
func insert(L *LuaState) int {
    if L.GetTop() == 1 {
        // insert(t, x) -> insert(t, #t+1, x)
        L.PushInteger(int64(L.GetTop() - 1))
        L.Rotate(1, -1)
    }
    // insert(t, pos, x)
    L.Rotate(-1, -2)
    return 0  // 已修改表
}

// sort
func sort_(L *LuaState) int {
    t := L.CheckTable(1)
    n := int64(t.Len())
    
    // 获取比较函数
    comp := func(i, j int) bool {
        L.PushValue(2)  // 比较函数
        L.PushInteger(int64(i))
        L.PushInteger(int64(j))
        L.Call(2, 1)
        result := L.ToBoolean(-1)
        L.Pop(1)
        return result
    }
    
    // 快速排序
    sort.SliceStable(t, comp)
    return 0
}
```

---

## 4. lmathlib — 数学库

### 函数列表

```go
var mathLib = []lua.Reg{
    {"abs",    abs},
    {"acos",   acos},
    {"asin",   asin},
    {"atan",   atan},
    {"ceil",   ceil},
    {"cos",    cos},
    {"cosh",   cosh},
    {"deg",    deg},
    {"exp",    exp},
    {"floor",  floor},
    {"fmod",   fmod},
    {"frexp",  frexp},
    {"hypot",  hypot},
    {"ldexp",  ldexp},
    {"log",    log},
    {"log10",  log10},
    {"max",    max},
    {"min",    min},
    {"modf",   modf},
    {"pi",     pi},
    {"pow",    pow},
    {"rad",    rad},
    {"random", random},
    {"randomseed", randomseed},
    {"sin",    sin},
    {"sinh",   sinh},
    {"sqrt",   sqrt},
    {"tan",    tan},
    {"tanh",   tanh},
    {"type",   mathtype},
}
```

---

## 5. liolib — I/O 库

### 文件操作

```go
var ioLib = []lua.Reg{
    {"close",   close},
    {"flush",   flush},
    {"input",   input},
    {"lines",   lines},
    {"open",    open},
    {"output",  output},
    {"popen",   popen},
    {"read",    read},
    {"tmpfile", tmpfile},
    {"type",    iotype},
    {"write",   write},
}
```

### 内部文件

```go
var defaultOutput File = os.Stdout

// 文件方法
var fileMethods = []lua.Reg{
    {"close",   fileClose},
    {"flush",   fileFlush},
    {"lines",   fileLines},
    {"read",    fileRead},
    {"seek",    fileSeek},
    {"setvbuf", fileSetVBuf},
    {"write",   fileWrite},
}
```

---

## 6. loslib — OS 库

### 函数列表

```go
var osLib = []lua.Reg{
    {"clock",      clock},
    {"date",       date},
    {"difftime",   difftime},
    {"execute",    execute},
    {"exit",       exit},
    {"getenv",     getenv},
    {"remove",     remove},
    {"rename",     rename},
    {"setlocale",  setlocale},
    {"time",       time},
    {"tmpname",    tmpname},
}
```

---

## 7. lcorolib — 协程库

### 函数列表

```go
var coLib = []lua.Reg{
    {"create",  create},
    {"resume",  resume},
    {"running", running},
    {"status",  status},
    {"wrap",    wrap},
    {"yield",   yield},
}
```

### 实现

```go
// create
func create(L *LuaState) int {
    L.CheckType(1, LUA_TFUNCTION)
    
    // 创建协程
    co := L.NewThread()
    
    // 复制函数到协程
    L.PushValue(1)
    L.XMove(co, 1)
    
    L.PushThread()
    return 1
}

// resume
func resume(L *LuaState) int {
    co := L.CheckThread(1)
    
    nargs := L.GetTop() - 1
    if !co.Resume(L, nargs) {
        L.PushBoolean(false)
        L.PushString(co.Status())
        return 2
    }
    
    // 返回结果
    nres := co.GetTop()
    L.XMove(co, nres)
    L.Insert(L.NewBoolean(true), 1)
    return L.GetTop()
}

// wrap
func wrap(L *LuaState) int {
    create(L)  // 创建协程
    
    // 创建 wrapper 函数
    L.PushGoFunction(wrapFunc)
    L.Remove(1)  // 协程作为 upvalue
    L.PushValue(1)
    return 1
}

func wrapFunc(L *LuaState) int {
    co := L.ToThread(L.UpValue(1))
    nargs := L.GetTop()
    
    L.XMove(co, nargs)
    co.Resume(L, nargs)
    
    nres := co.GetTop()
    L.XMove(co, nres)
    return nres
}
```

---

## 8. ldblib — 调试库

### 函数列表

```go
var debugLib = []lua.Reg{
    {"debug",      debug},
    {"getfenv",    getfenv},
    {"gethook",    gethook},
    {"getinfo",    getinfo},
    {"getlocal",   getlocal},
    {"getmetatable", getmetatableDebug},
    {"getregistry", getregistry},
    {"getupvalue", getupvalue},
    {"getuservalue", getuservalue},
    {"setfenv",    setfenv},
    {"sethook",    sethook},
    {"setlocal",   setlocal},
    {"setmetatable", setmetatableDebug},
    {"setupvalue", setupvalue},
    {"setuservalue", setuservalue},
    {"traceback",  traceback},
    {"upvalueid",  upvalueid},
    {"upvaluejoin", upvaluejoin},
}
```

---

## 9. lutf8lib — UTF-8 库

### 函数列表

```go
var utf8Lib = []lua.Reg{
    {"char",     utf8Char},
    {"codes",    utf8Codes},
    {"len",      utf8Len},
    {"lenb",     utf8Lenb},
    {"offset",   utf8Offset},
    {"sub",      utf8Sub},
    {"codepoint", utf8Codepoint},
    {"charpattern", utf8CharPattern},
}
```

### UTF-8 实现

```go
// len
func utf8Len(L *LuaState) int {
    s := L.CheckString(1)
    i := L.OptInteger(3, 1)
    j := L.OptInteger(4, -1)
    
    count := 0
    pos := 0
    for _, r := range s {
        pos += utf8.RuneLen(r)
        count++
        if pos >= int(j) {
            break
        }
    }
    
    L.PushInteger(int64(count))
    return 1
}

// codepoint
func utf8Codepoint(L *LuaState) int {
    s := L.CheckString(1)
    i := L.OptInteger(3, 1)
    j := L.OptInteger(4, -1)
    
    n := 0
    pos := 0
    for _, r := range s {
        pos += utf8.RuneLen(r)
        if pos >= int(i) && (j < 0 || pos <= int(j)) {
            L.PushInteger(int64(r))
            n++
        }
        if j > 0 && pos >= int(j) {
            break
        }
    }
    return n
}
```

---

## 10. loadlib — 动态库加载

### 函数列表

```go
var loadLib = []lua.Reg{
    {"searchpath", searchpath},
    {"preload",   preload},
    {"loadlib",   loadlib},
    {"seeall",    seeall},
}
```

### 实现

```go
// searchpath
func searchpath(L *LuaState) int {
    name := L.CheckString(1)
    path := L.CheckString(2)
    sep := L.OptString(3, ".")
    rep := L.OptString(4, "/")
    
    // 替换分隔符
    name = strings.ReplaceAll(name, sep, rep)
    
    // 搜索路径
    for _, pattern := range strings.Split(path, ";") {
        filename := strings.Replace(pattern, "?", name, 1)
        if _, err := os.Stat(filename); err == nil {
            L.PushString(filename)
            return 1
        }
    }
    
    L.PushFString("module '%s' not found", name)
    return L.Error()
}

// loadlib
func loadlib(L *LuaState) int {
    filename := L.CheckString(1)
    funcname := L.CheckString(2)
    
    // 加载动态库
    handle, err := dlopen(filename)
    if err != nil {
        L.PushNil()
        L.PushString(err.Error())
        return 2
    }
    
    // 查找函数
    sym, err := dlsym(handle, funcname)
    if err != nil {
        L.PushNil()
        L.PushString(err.Error())
        return 2
    }
    
    // 注册为 C 函数
    L.PushGoFunction(sym.(GoFunction))
    return 1
}
```

---

## 库初始化

```go
// luaL_openlibs: 打开所有标准库
func Openlibs(L *LuaState) {
    libs := []struct{
        name string
        fn   func(*LuaState) int
    }{
        {"_G", Open},
        {LUA_COLIBNAME, OpenCore},
        {LUA_TABLIBNAME, OpenTable},
        {LUA_IOLIBNAME, OpenIO},
        {LUA_OSLIBNAME, OpenOS},
        {LUA_STRLIBNAME, OpenString},
        {LUA_MATHLIBNAME, OpenMath},
        {LUA_UTF8LIBNAME, OpenUTF8},
        {LUA_DBLIBNAME, OpenDebug},
        {LUA_LOADLIBNAME, OpenPackage},
    }
    
    for _, lib := range libs {
        L.RequireF(lib.name, lib.fn, true)
    }
}

// RequireF: 加载模块（支持 preload）
func (L *LuaState) RequireF(name string, openf func(*LuaState) int, glb bool) {
    L.PushGlobalTable()
    L.PushString(name)
    L.RawGet(-2)
    if L.IsNil(-1) {
        L.Pop(1)
        
        // 调用加载函数
        L.RequireFprepare(name, openf)
        if glb {
            L.PushValue(-1)
            L.SetField(-3, name)
        }
    }
}
```