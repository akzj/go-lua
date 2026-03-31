# ldo 模块规格书

## 模块职责

管理 Lua 函数调用栈、处理 protected call、实现错误传播。这是 Lua 运行时最核心的模块之一，与虚拟机（lvm）紧密耦合。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lstate | lua_State, CallInfo |
| lfunc | luaF_close |
| lvm | luaV_execute |
| lobject | TValue |
| lgc | GC |
| ldebug | 错误消息 |

## 公开 API

```c
/* 栈管理 */
LUAI_FUNC int luaD_growstack (lua_State *L, int n, int raiseerror);
LUAI_FUNC int luaD_reallocstack (lua_State *L, int newsize, int raiseerror);
LUAI_FUNC void luaD_shrinkstack (lua_State *L);
LUAI_FUNC int luaD_checkminstack (lua_State *L);
LUAI_FUNC void luaD_inctop (lua_State *L);

/* 调用 */
LUAI_FUNC CallInfo *luaD_precall (lua_State *L, StkId func, int nResults);
LUAI_FUNC void luaD_poscall (lua_State *L, CallInfo *ci, int nres);
LUAI_FUNC int luaD_pretailcall (lua_State *L, CallInfo *ci, StkId func,
                                 int narg1, int delta);
LUAI_FUNC void luaD_call (lua_State *L, StkId func, int nResults);
LUAI_FUNC void luaD_callnoyield (lua_State *L, StkId func, int nResults);

/* 错误处理 */
LUAI_FUNC TStatus luaD_rawrunprotected (lua_State *L, Pfunc f, void *ud);
LUAI_FUNC void luaD_seterrorobj (lua_State *L, TStatus errcode, StkId oldtop);
LUAI_FUNC l_noret luaD_throw (lua_State *L, TStatus errcode);
LUAI_FUNC l_noret luaD_throwbaselevel (lua_State *L, TStatus errcode);
LUAI_FUNC TStatus luaD_pcall (lua_State *L, Pfunc func, void *u,
                               ptrdiff_t old_top, ptrdiff_t ef);
LUAI_FUNC TStatus luaD_closeprotected (lua_State *L, ptrdiff_t level, TStatus status);

/* Hook */
LUAI_FUNC void luaD_hook (lua_State *L, int event, int line,
                           int fTransfer, int nTransfer);
LUAI_FUNC void luaD_hookcall (lua_State *L, CallInfo *ci);

/* 解析器 */
LUAI_FUNC TStatus luaD_protectedparser (lua_State *L, ZIO *z,
                                          const char *name, const char *mode);
```

## 核心数据结构

### CallInfo 结构

```c
struct CallInfo {
  StkIdRel func;      /* 函数在栈上的位置 */
  StkIdRel top;       /* 此调用的栈顶 */
  struct CallInfo *previous, *next;  /* 调用链 */
  
  union {
    struct {  /* Lua 函数 */
      const Instruction *savedpc;   /* 保存的 PC */
      volatile l_signalT trap;       /* 调试陷阱 */
      int nextraargs;               /* 额外参数 */
    } l;
    struct {  /* C 函数 */
      lua_KFunction k;              /* yield 继续函数 */
      ptrdiff_t old_errfunc;
      lua_KContext ctx;             /* 上下文 */
    } c;
  } u;
  
  union {
    int funcidx;   /* 被调用函数的索引（pcall 用） */
    int nyield;    /* yield 的值数量 */
    int nres;      /* 返回值数量 */
  } u2;
  
  l_uint32 callstatus;  /* 调用状态标志 */
};

/* 关键状态标志 */
#define CIST_C       (1 << 15)   /* C 函数 */
#define CIST_FRESH   (1 << 16)  /* 新的执行帧 */
#define CIST_TBC     (1 << 17)  /* 有 to-be-closed 变量 */
#define CIST_OAH     (1 << 18)  /* allowhook 原值 */
#define CIST_HOOKED  (1 << 19)  /* 在 hook 中 */
#define CIST_YPCALL  (1 << 20)  /* yieldable protected call */
#define CIST_TAIL    (1 << 21)  /* 尾调用 */
#define CIST_HOOKYIELD (1 << 22) /* hook yield */
```

### Protected Call 机制

```c
/* lua_longjmp - setjmp/longjmp 的替代品 */
typedef struct lua_longjmp {
  struct lua_longjmp *previous;
  jmp_buf b;
  volatile TStatus status;
} lua_longjmp;

/* Protected call 实现 */
TStatus luaD_rawrunprotected (lua_State *L, Pfunc f, void *ud) {
  lj.previous = L->errorJmp;
  L->errorJmp = &lj;
  
  if (setjmp(lj.b) == 0) {
    f(L, ud);  /* 正常执行 */
  } else {
    /* 发生错误/yield */
  }
  
  L->errorJmp = lj.previous;
  return lj.status;
}
```

## Go 重写规格

### 类型定义

```go
package lua

// CallInfo 函数调用帧
type CallInfo struct {
    Func    int     // 函数在栈上的索引（相对）
    Top     int     // 此帧的栈顶
    
    // 链表
    Previous *CallInfo
    Next     *CallInfo
    
    // Lua 函数专用
    SavedPC int       // 保存的 PC
    Trap    uint32   // 调试陷阱
    nExtraArgs int   // 额外参数
    
    // C 函数专用
    K      GoFunction // 继续函数
    ErrFunc int       // 错误处理函数
    Ctx    interface{} // 上下文
    
    // 多用途
    FuncIdx int       // pcall 用
    NYield  int       // yield 数量
    NRes    int       // 返回值数量
    
    Status  uint32    // 状态标志
}

// 状态常量
const (
    CIST_C        uint32 = 1 << 15
    CIST_FRESH     uint32 = 1 << 16
    CIST_TBC       uint32 = 1 << 17
    CIST_OAH       uint32 = 1 << 18
    CIST_HOOKED    uint32 = 1 << 19
    CIST_YPCALL    uint32 = 1 << 20
    CIST_TAIL      uint32 = 1 << 21
    CIST_HOOKYIELD uint32 = 1 << 22
)

func (ci *CallInfo) IsC() bool       { return ci.Status&CIST_C != 0 }
func (ci *CallInfo) IsLua() bool     { return ci.Status&CIST_C == 0 }
func (ci *CallInfo) IsFresh() bool   { return ci.Status&CIST_FRESH != 0 }
func (ci *CallInfo) HasTBC() bool    { return ci.Status&CIST_TBC != 0 }
```

### 栈管理

```go
type LuaState struct {
    // ... 其他字段
    stack    []TValue  // 栈
    top      int       // 当前栈顶
    stackLast int      // 栈末尾
    
    ci       *CallInfo  // 当前调用帧
    baseCi   *CallInfo  // 基帧（主线程）
    
    openUpval *UpVal    // 开放的 upvalue 链表
    tbcList  int        // to-be-closed 变量
    
    errorJmp  *LJmp     // 错误处理帧
}

const (
    LUAI_MAXSTACK    = 1000000
    BASIC_STACK_SIZE = 2 * LUA_MINSTACK  // 2 * 13 = 26
    STACK_ERR_SPACE  = 200
)

// luaD_growstack
func (L *LuaState) GrowStack(need int) bool {
    size := len(L.stack)
    
    if size > LUAI_MAXSTACK {
        // 已经达到最大
        return false
    }
    
    // 增长因子 1.5
    newSize := size + size/2
    if newSize < size + need {
        newSize = size + need
    }
    if newSize > LUAI_MAXSTACK {
        newSize = LUAI_MAXSTACK
    }
    
    return L.reallocStack(newSize)
}

// luaD_reallocstack
func (L *LuaState) reallocStack(newSize int) bool {
    oldStack := L.stack
    oldSize := len(oldStack)
    
    // 分配新栈
    newStack := make([]TValue, newSize + STACK_ERR_SPACE)
    
    // 复制旧内容
    copy(newStack, oldStack)
    
    // 修正指针（Go 不需要，但需要修正 CallInfo 中的索引）
    // 在 Go 中，slice 已经自动处理
    
    L.stack = newStack
    
    // 初始化新空间为 nil
    for i := oldSize; i < len(newStack); i++ {
        L.stack[i].SetNil()
    }
    
    return true
}
```

### 函数调用

```go
// luaD_call: 调用函数
func (L *LuaState) Call(funcIdx, nResults int) {
    if !L.isFunction(L.stack[funcIdx]) {
        // 尝试 __call 元方法
        if !L.tryCallMetamethod(funcIdx, 1) {
            L.runError("attempt to call a non-function")
            return
        }
    }
    
    ci := L.precall(funcIdx, nResults)
    if ci != nil {
        // Lua 函数
        L.execute(ci)
    }
    // C 函数在 precall 中已经执行
}

// luaD_precall: 准备调用
func (L *LuaState) precall(funcIdx, nResults int) *CallInfo {
    f := &L.stack[funcIdx]
    
    switch f.TypeTag() {
    case LUA_VCCL:  // C 闭包
        return L.precallC(f.AsCClosure(), funcIdx, nResults)
        
    case LUA_VLCF:  // light C function
        return L.precallC(f.AsLightC(), funcIdx, nResults)
        
    case LUA_VLCL:  // Lua 闭包
        return L.precallLua(f.AsLClosure(), funcIdx, nResults)
        
    default:
        return nil
    }
}

// precall Lua 函数
func (L *LuaState) precallLua(cl *LClosure, funcIdx, nResults int) *CallInfo {
    proto := cl.Proto
    
    // 准备新 CallInfo
    ci := &CallInfo{
        Func: funcIdx,
        Top:  funcIdx + 1 + int(proto.MaxStackSize),
        Previous: L.ci,
    }
    ci.Status = uint32(nResults + 1)
    
    // 检查栈空间
    if ci.Top > len(L.stack) {
        L.GrowStack(ci.Top - len(L.stack))
    }
    
    // 初始化参数
    narg := L.top - funcIdx - 1
    nfixparams := int(proto.NumParams)
    
    for i := narg; i < nfixparams; i++ {
        L.stack[L.top].SetNil()
        L.top++
    }
    
    // 保存 PC
    ci.SavedPC = 0
    
    L.ci = ci
    return ci
}

// precall C 函数
func (L *LuaState) precallC(fn interface{}, funcIdx, nResults int) {
    ci := &CallInfo{
        Func: funcIdx,
        Top:  funcIdx + 1 + LUA_MINSTACK,
        Previous: L.ci,
    }
    ci.Status = uint32(nResults+1) | CIST_C
    
    L.ci = ci
    
    // 执行 C 函数
    L.execCFunction(fn)
}
```

### Protected Call (pcall)

```go
// luaD_rawrunprotected 等价
type LJmp struct {
    Previous *LJmp
    Status   Status
    Ch       chan struct{}
}

func (L *LuaState) rawRunProtected(f func(), ud interface{}) Status {
    lj := &LJmp{
        Previous: L.errorJmp,
        Ch:       make(chan struct{}),
    }
    L.errorJmp = lj
    
    done := make(chan Status, 1)
    
    go func() {
        defer func() {
            if r := recover(); r != nil {
                if err, ok := r.(LuaError); ok {
                    lj.Status = err.Status
                } else {
                    lj.Status = LUA_ERRRUN
                }
            }
            done <- lj.Status
        }()
        f()
        done <- LUA_OK
    }()
    
    status := <-done
    L.errorJmp = lj.Previous
    return status
}

// luaD_pcall 等价
func (L *LuaState) pcall(funcIdx, nResults, errFuncIdx int) Status {
    oldTop := L.top
    
    // 查找错误处理函数
    errFunc := 0
    if errFuncIdx != 0 {
        errFunc = L.absIndex(errFuncIdx)
    }
    
    status := L.rawRunProtected(func() {
        L.Call(funcIdx, nResults)
    }, nil)
    
    if status != LUA_OK {
        // 恢复栈
        L.top = oldTop
        // 调用错误处理
        if errFunc != 0 {
            L.Call(errFunc, 1)
        }
    }
    
    return status
}

// luaD_throw 等价
func (L *LuaState) throw(status Status) {
    if L.errorJmp != nil {
        L.errorJmp.Status = status
        // longjmp 到错误处理点
        // Go 中用 panic
        panic(LuaError{Status: status, L: L})
    }
    
    // 向上传播
    if L.IsMainThread() {
        if L.G.Panic != nil {
            L.G.Panic(L)
        }
        panic("Lua panic")
    }
    
    // 传播到父线程
    if L.Parent != nil {
        L.Parent.throw(status)
    }
}
```

### 尾调用优化

```go
// luaD_pretailcall: 尾调用优化
// 尾调用：return f(...) 不需要返回
func (L *LuaState) preTailCall(funcIdx, nArgs, delta int) int {
    // 检查是否是 Lua 函数
    f := &L.stack[funcIdx]
    
    if f.TypeTag() != LUA_VLCL {
        // C 函数或非函数，无法尾调用优化
        return -1
    }
    
    cl := f.AsLClosure()
    proto := cl.Proto
    
    // 复用当前 CallInfo
    ci := L.ci
    ci.Status |= CIST_TAIL
    
    // 移动函数和参数
    for i := 0; i < nArgs; i++ {
        L.stack[ci.Func + i] = L.stack[funcIdx + i]
    }
    
    // 重置参数
    nfixparams := int(proto.NumParams)
    for i := nArgs; i < nfixparams; i++ {
        L.stack[ci.Func + i].SetNil()
    }
    
    // 设置新 top
    ci.Top = ci.Func + 1 + int(proto.MaxStackSize)
    L.top = ci.Func + nArgs
    
    // 重置 PC
    ci.SavedPC = 0
    
    return 0  // 继续执行
}
```

### to-be-closed 变量

```go
// luaF_newtbcupval: 标记为 to-be-closed
func (L *LuaState) markTBC(slot int) {
    // 创建 upvalue（用于追踪栈位置）
    uv := L.newUpVal()
    uv.V = &L.stack[slot]
    
    // 添加到 tbc 链表
    uv.TBCNext = L.tbcUpvals
    L.tbcUpvals = uv
    
    // 标记当前函数有 tbc
    L.ci.Status |= CIST_TBC
}

// luaF_close: 关闭变量
func (L *LuaState) CloseUpvals(slot int) {
    // 关闭所有 >= slot 的 upvalue
    for uv := L.openUpval; uv != nil; uv = uv.Next {
        if uv.StackIdx >= slot {
            uv.Close()
        }
    }
    
    // 关闭所有 tbc 变量
    for uv := L.tbcUpvals; uv != nil; uv = uv.Next {
        if uv.StackIdx >= slot {
            // 调用 __close
            if mt := uv.GetMetatable(); mt != nil {
                if close := mt.Get("__close"); close != nil {
                    L.CallMeta(uv, "close")
                }
            }
            uv.Close()
        }
    }
}
```

### lua_yield / lua_resume

```go
// lua_yieldk 等价
func (L *LuaState) Yield(nResults int) {
    if !L.IsYieldable() {
        L.runError("cannot yield")
        return
    }
    
    L.status = StatusYield
    L.ci.NYield = nResults
    
    if L.ci.IsLua() {
        // 在 Lua 函数中 yield
        // 保存状态，panic 到 resume 点
        panic(YieldError{
            Status:  LUA_YIELD,
            NYield:  nResults,
            Continuation: nil,
        })
    } else {
        // 在 C 函数中 yield
        // panic 给 resume 处理
        panic(YieldError{
            Status:  LUA_YIELD,
            NYield:  nResults,
            Continuation: L.ci.Cont,
        })
    }
}

// lua_resume 等价
func (L *LuaState) Resume(args []Value) ([]Value, Status) {
    switch L.status {
    case StatusOK:
        // 首次 resume，执行函数
        if L.ci != L.baseCi {
            return nil, StatusError
        }
        
    case StatusYield:
        // 从 yield 恢复
        
    default:
        return nil, StatusError
    }
    
    // 保护调用
    status := L.rawRunProtected(func() {
        L.continueExecution(args)
    }, nil)
    
    results := L.getReturnValues()
    return results, status
}

func (L *LuaState) continueExecution(args []Value) {
    if L.status == StatusOK {
        // 首次调用
        funcIdx := L.top - len(args) - 1
        L.Call(funcIdx, LUA_MULTRET)
    } else {
        // 从 yield 恢复
        ci := L.ci
        
        if ci.IsLua() {
            // 继续 Lua 执行
            L.execute(ci)
        } else {
            // 调用 continuation
            if ci.Cont != nil {
                L.Call(ci.Cont, LUA_MULTRET)
            }
            L.poscall(ci, L.ci.NYield)
        }
        
        // 继续执行栈上的函数
        for L.ci != L.baseCi {
            L.continueCall()
        }
    }
}
```

## 陷阱和注意事项

### 陷阱 1: setjmp/longjmp vs Go panic

C 中 longjmp 会**跳过栈展开**，直接跳到 setjmp 点。Go 的 panic 会执行 defer，但 Lua 期望：

1. 调用栈上的局部变量**不被 cleanup**
2. UpValue **不自动关闭**
3. 资源**不自动释放**（除非明确调用 luaF_close）

**Go 方案：**
```go
// 创建一个特殊的 panic 类型
type LuaLongJmp struct {
    Status Status
    From   *LJmp
}

func (L *LuaState) throw(status Status) {
    // 不调用任何 defer
    panic(LuaLongJmp{Status: status, From: L.errorJmp})
}

// 在 rawRunProtected 中恢复
func (L *LuaState) rawRunProtected(f func()) Status {
    defer func() {
        if r := recover(); r != nil {
            if lj, ok := r.(LuaLongJmp); ok {
                // longjmp 语义：直接跳到这里
                L.errorJmp = lj.From
                L.recoverFromLuaJmp()
                // 不运行其他 defer
                runtime.Goexit()
            } else {
                panic(r)
            }
        }
    }()
    
    f()
    return LUA_OK
}
```

### 陷阱 2: 栈重分配时的 CallInfo

C 中 CallInfo 的 func/top 使用 StkIdRel（偏移量），栈重分配时需要修正：

```go
// Go 方案：使用绝对索引，不需要修正
// 因为 Go slice 重分配后，所有引用失效，必须重新获取
type LuaState struct {
    stack []TValue
}

type CallInfo struct {
    Func int  // 绝对索引
    Top  int
}

// 当栈重分配时
func (L *LuaState) reallocStack(newSize int) {
    oldStack := L.stack
    oldTop := L.top
    
    L.stack = make([]TValue, newSize)
    copy(L.stack, oldStack)
    
    // 修正 CallInfo
    for ci := L.ci; ci != nil; ci = ci.Previous {
        // 索引不需要修正，因为用的是绝对位置
        // 但需要确保在范围内
        if ci.Top > newSize {
            ci.Top = newSize
        }
    }
}
```

### 陷阱 3: yield 时的状态保存

```c
// C 中 yield 需要保存：
// 1. 当前 PC（用于恢复执行）
// 2. 调用帧状态
// 3. 栈上的返回值

// lua_yieldk 关键代码
L->status = LUA_YIELD;
ci->u2.nyield = nresults;
luaD_throw(L, LUA_YIELD);  // longjmp 到 resume 点
```

**Go 方案：**
```go
type LuaState struct {
    status   Status
    savedPC  int       // yield 时保存的 PC
    savedCI  *CallInfo // yield 时的调用帧
    savedTop int       // yield 时的栈顶
}

func (L *LuaState) Yield(n int) {
    L.status = StatusYield
    L.savedPC = L.ci.SavedPC
    L.savedCI = L.ci
    L.savedTop = L.top - n  // 返回值位置
    
    panic(YieldError{})
}

func (L *LuaState) Resume(args []Value) {
    // 从 saved 状态恢复
    L.status = StatusRunning
    // 继续执行
}
```

### 陷阱 4: C 调用计数限制

```c
// C 调用深度有限制，防止栈溢出
#define LUAI_MAXCCALLS 200

// nCcalls 分为两部分：
// 低 16 位：C 调用计数
// 高 16 位：非可中断调用计数

#define incnny(L)   ((L)->nCcalls += 0x10000)
#define decnny(L)   ((L)->nCcalls -= 0x10000)
```

**Go 方案：**
```go
type LuaState struct {
    nCcalls uint32
}

const LUAI_MAXCCALLS = 200

func (L *LuaState) incCcalls() bool {
    if L.nCcalls & 0xFFFF >= LUAI_MAXCCALLS {
        return false
    }
    L.nCcalls++
    return true
}

func (L *LuaState) incNny() {
    L.nCcalls += 0x10000
}
```

## 验证测试

```lua
-- calls.lua 关键测试

-- 基本调用
assert(math.max(1, 2, 3) == 3)

-- 多返回值
local function multi() return 1, 2, 3 end
local a, b, c = multi()
assert(a == 1 and b == 2 and c == 3)

-- 尾调用
local depth = 0
local function tail_recursive(n)
    depth = depth + 1
    if n > 1 then return tail_recursive(n - 1) end
    return depth
end
assert(tail_recursive(10000) == 10000)  -- 不应该栈溢出

-- pcall
local ok, err = pcall(error, "test")
assert(not ok and err == "test")

-- xpcall
local ok, msg = xpcall(
    function() error("fail") end,
    function(e) return "caught: " .. e end
)
assert(not ok and msg == "caught: fail")

-- 变参
local function vararg(...)
    local args = {...}
    return #args
end
assert(vararg(1, 2, 3) == 3)
assert(vararg() == 0)
```

```lua
-- coroutine.lua 测试

-- 基本 yield/resume
local co = coroutine.create(function()
    coroutine.yield(1)
    return 2
end)

local ok, v = coroutine.resume(co)
assert(ok and v == 1)

ok, v = coroutine.resume(co)
assert(ok and v == 2)
assert(coroutine.status(co) == "dead")

-- 嵌套 yield
local function producer()
    for i = 1, 5 do
        coroutine.yield(i)
    end
end

local p = coroutine.wrap(producer)
assert(p() == 1)
assert(p() == 2)
assert(p() == 3)

-- 尾调用 yield
local function tail_yield()
    return coroutine.yield(42)
end
local co = coroutine.create(tail_yield)
assert(coroutine.resume(co) == 42)
```