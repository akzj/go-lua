# C → Go 翻译陷阱详解

本文档详细分析 Lua 5.5 C 源码中最容易导致重写失败的陷阱，给出精确的 Go 等价实现方案。

---

## 1. setjmp/longjmp 错误处理机制

### 问题描述

Lua 使用 `setjmp/longjmp` 实现：
1. **Protected call** — 在 C API `lua_pcall` 中保护函数调用
2. **协程 yield/resume** — `lua_yieldk` 需要从深层调用栈直接返回
3. **错误传播** — `luaD_throw` 跳转到最近的错误处理点

### C 实现分析（ldo.c）

```c
// lua_longjmp 结构体 — 错误处理帧
typedef struct lua_longjmp {
  struct lua_longjmp *previous;  // 链表链接
  jmp_buf b;                      // setjmp 缓冲区
  volatile TStatus status;        // 错误码
} lua_longjmp;

// 核心 protected call 实现
TStatus luaD_rawrunprotected (lua_State *L, Pfunc f, void *ud) {
  l_uint32 oldnCcalls = L->nCcalls;
  lua_longjmp lj;
  lj.status = LUA_OK;
  lj.previous = L->errorJmp;     // 链入错误处理栈
  L->errorJmp = &lj;
  
  LUAI_TRY(L, &lj, f, ud);       // setjmp/try 块
  // 如果 f() 正常返回，继续执行这里
  L->errorJmp = lj.previous;
  L->nCcalls = oldnCcalls;
  return lj.status;              // 返回 OK 或错误码
}

// 抛出错误 — longjmp 到最近的 errorJmp
l_noret luaD_throw (lua_State *L, TStatus errcode) {
  if (L->errorJmp) {
    L->errorJmp->status = errcode;
    LUAI_THROW(L, L->errorJmp);  // longjmp
  }
  // ... 向上传播到主线程
}
```

### 关键宏展开

```c
// POSIX 实现
#if defined(LUA_USE_POSIX)
#define LUAI_THROW(L,c)    _longjmp((c)->b, 1)
#define LUAI_TRY(L,c,f,ud) if (_setjmp((c)->b) == 0) ((f)(L, ud))
#endif
```

### Go 等价实现方案

**方案 A：Goroutine + Channel（推荐）**

```go
// 用 goroutine 实现每个 protected call
func (L *LuaState) protectedCall(f func(*LuaState), args []Value) (results []Value, err error) {
    done := make(chan []Value, 1)
    errCh := make(chan error, 1)
    
    go func() {
        defer func() {
            if r := recover(); r != nil {
                errCh <- r.(error)
            }
        }()
        results = f(L)
        done <- results
    }()
    
    select {
    case results = <-done:
        return results, nil
    case err = <-errCh:
        return nil, err
    }
}

// lua_pcall 等价实现
func (L *LuaState) PCall(nArgs, nResults int) error {
    // 保存当前栈状态
    funcIdx := L.top - nArgs - 1
    
    err := L.protectedCall(func(L *LuaState) []Value {
        L.call(funcIdx, nResults)
        return nil  // call 已经处理了结果
    }, nil)
    
    return err
}
```

**方案 B：Panic/Recover（适用于单线程栈）**

```go
// luaD_rawrunprotected 等价
func (L *LuaState) rawRunProtected(f func(), ud interface{}) Status {
    defer func() {
        if r := recover(); r != nil {
            if err, ok := r.(LuaError); ok {
                L.errorStatus = err.Status
            } else {
                L.errorStatus = StatusErrRun
            }
        }
    }()
    
    f()
    return LUA_OK
}

// luaD_throw 等价
func (L *LuaState) throw(errcode Status) {
    panic(LuaError{Status: errcode, State: L})
}
```

### 陷阱 1.1：Coroutine Yield 的特殊情况

```c
// lua_yieldk 不只是抛错，而是保存状态后返回
LUA_API int lua_yieldk (lua_State *L, int nresults, lua_KContext ctx,
                        lua_KFunction k) {
    // ... 验证 ...
    L->status = LUA_YIELD;
    ci->u2.nyield = nresults;
    
    if (isLua(ci)) {
        // 在 hook 中 yield，直接返回到 hook 调用点
    } else {
        // 普通 yield，longjmp 到 resume 的调用点
        if (ci->u.c.k = k) != NULL)
            ci->u.c.ctx = ctx;
        luaD_throw(L, LUA_YIELD);  // 特殊处理
    }
    lua_assert(ci->callstatus & CIST_HOOKED);
    lua_unlock(L);
    return 0;  // 返回 0 给 hook
}
```

**Go 方案：Goroutine 天然支持**

```go
// lua_yield 等价
func (L *LuaState) Yield(nResults int) {
    L.status = StatusYield
    
    // 暂停当前 goroutine，恢复到 resume 点
    runtime.Gosched()  // 让出调度
    
    // 注意：需要特殊处理才能 resume
    // 推荐使用 channel 传递控制权
}
```

---

## 2. Tagged Union → Go 接口

### 问题描述

Lua 用 C union 实现类型安全的 variant：

```c
typedef union Value {
  struct GCObject *gc;
  void *p;
  lua_CFunction f;
  lua_Integer i;
  lua_Number n;
} Value;

typedef struct TValue {
  Value value_;
  lu_byte tt_;  // type tag
} TValue;
```

Go 没有 union，必须用其他方式实现。

### Go 等价实现

**方案：Interface + Type Switch**

```go
// LuaValue 是所有 Lua 值的接口
type LuaValue interface {
    // 获取类型
    Type() LuaType
    // 获取原始值（用于内部操作）
    rawGet() (GCObject | uintptr | luaGoFunction | int64 | float64, LuaType)
}

// 基础实现
type nilValue struct{}
type booleanValue bool
type integerValue int64
type floatValue float64
type lightUserdata unsafe.Pointer
type stringValue struct { s string }       // Go string 自动 intern
type tableValue *Table
type closureValue *Closure
type threadValue *LuaState
type userdataValue struct { data unsafe.Pointer; size int }
```

**更实用的实现（性能优化）**

```go
// 用 struct + type tag 实现类似效果
type TValue struct {
    tt LuaType  // 8 bytes
    // 根据 tt 的值，以下某个字段有效
    gc    uintptr      // 可回收对象指针
    p     unsafe.Pointer // light userdata
    f     luaGoFunction // C function (Go version)
    i     int64        // integer
    n     float64      // number (float)
}

// 类型常量
type LuaType uint8

const (
    LUA_TNIL          LuaType = 0
    LUA_TBOOLEAN      LuaType = 1
    LUA_TLIGHTUSERDATA LuaType = 2
    LUA_TNUMBER       LuaType = 3
    LUA_TSTRING       LuaType = 4
    LUA_TTABLE        LuaType = 5
    LUA_TFUNCTION     LuaType = 6
    LUA_TUSERDATA     LuaType = 7
    LUA_TTHREAD       LuaType = 8
    // 变体
    LUA_VNUMINT  LuaType = 0x10  // 整数
    LUA_VNUMFLT  LuaType = 0x11  // 浮点
    LUA_VSHRSTR  LuaType = 0x14  // 短字符串
    LUA_VLNGSTR  LuaType = 0x15  // 长字符串
)
```

### 陷阱 2.1：可回收对象标记

```c
// C 中，可回收对象通过 GCObject header 识别
typedef struct GCObject {
  struct GCObject *next;
  lu_byte tt;
  lu_byte marked;
} GCObject;

// 检查是否是可回收对象
#define iscollectable(o)    (rawtt(o) & BIT_ISCOLLECTABLE)
#define BIT_ISCOLLECTABLE  (1 << 6)
```

**Go 方案：统一 GCObject 接口**

```go
// 所有可回收对象实现此接口
type GCObject interface {
    GetHeader() *GCHeader
    Mark()
    Traverse(visitor GCVisitor)
}

type GCHeader struct {
    Next *GCHeader  // GC 链表
    Tt   LuaType    // 类型标签
    Marked uint8    // 标记位
}

// 对象类型断言
func AsTable(v LuaValue) *Table {
    if t, ok := v.(*Table); ok {
        return t
    }
    return nil
}
```

---

## 3. 栈操作：指针算术 → Slice Index

### 问题描述

C 中，栈是数组，指针算术操作：

```c
typedef StackValue *StkId;  // 栈元素指针

// 栈操作
#define s2v(o)          (&(o)->val)           // StackValue → TValue
#define savestack(L,pt) (cast_charp(pt) - cast_charp(L->stack.p))  // 指针 → 偏移
#define restorestack(L,n) cast(StkId, cast_charp(L->stack.p) + (n))  // 偏移 → 指针

// push/pop
L->top.p++;                // push
TValue *obj = --L->top.p;  // pop

// 取值
#define setobj(L, obj1, obj2) \
  { TValue *io1=(obj1); const TValue *io2=(obj2); \
    io1->value_ = io2->value_; settt_(io1, io2->tt_); \
    checkliveness(L,io1); }
```

### Go 等价实现

```go
type LuaState struct {
    stack     []TValue   // 栈
    top       int        // 栈顶索引（下一个写入位置）
    stackLast int        // 栈底（已分配）
}

// push 操作
func (L *LuaState) push(value TValue) {
    L.stack[L.top] = value
    L.top++
}

// pop 操作
func (L *LuaState) pop() TValue {
    L.top--
    return L.stack[L.top]
}

// 获取栈顶元素（不弹出）
func (L *LuaState) stackTop() int {
    return L.top
}

// 取相对索引（Lua 索引可以是负数）
func (L *LuaState) index2addr(idx int) int {
    if idx > 0 {
        // 相对于函数帧基址
        base := L.ci.Base() // CallInfo 中的函数位置
        return base + idx - 1
    } else {
        // 负索引相对于栈顶
        return L.top + idx
    }
}

// savestack/restostack 等价（用于 GC 或栈重分配）
func (L *LuaState) saveStack() int {
    return L.top
}

func (L *LuaState) restoreStack(saved int) {
    L.top = saved
}
```

### 陷阱 3.1：栈重分配时的指针更新

C 中栈可以增长，所有指针必须转换为偏移量：

```c
// relstack：重分配前，将所有指针转为偏移
static void relstack (lua_State *L) {
  L->top.offset = savestack(L, L->top.p);
  L->tbclist.offset = savestack(L, L->tbclist.p);
  // ...
}

// correctstack：重分配后，从偏移恢复指针
static void correctstack (lua_State *L, StkId oldstack) {
  L->top.p = restorestack(L, L->top.offset);
  // ...
}
```

**Go 方案：天然处理**

```go
// Go slice 会自动处理重新分配，index 天然正确
func (L *LuaState) growStack(needed int) {
    newSize := len(L.stack) * 2
    if newSize < needed {
        newSize = needed
    }
    // slice 复制后，原来的 []TValue 位置无关
    newStack := make([]TValue, newSize)
    copy(newStack, L.stack)
    L.stack = newStack
}
```

### 陷阱 3.2：Off-by-One 错误

**C 中容易出错的地方：**

```c
// lua_gettop 返回元素个数，不是最后一个索引
LUA_API int lua_gettop (lua_State *L) {
  return cast_int(L->top.p - (L->ci->func.p + 1));
}

// lua_settop 可以收缩或扩展栈
LUA_API void lua_settop (lua_State *L, int idx) {
  StkId func = L->ci->func.p;
  if (idx >= 0) {
    // ...
    L->top.p = func + 1 + idx;  // top = func + 1 + idx
  }
}
```

**Go 必须正确对应：**

```go
func (L *LuaState) GetTop() int {
    base := L.ci.Base()
    return L.top - base - 1  // top - base - 1 = 元素个数
}

func (L *LuaState) SetTop(idx int) {
    base := L.ci.Base()
    if idx >= 0 {
        L.top = base + 1 + idx
    } else {
        L.top = L.top + idx + 1
    }
}
```

---

## 4. 关键宏展开分析

### 4.1 setobj 系列

```c
// 最基础的赋值宏
#define setobj(L,obj1,obj2) \
  { TValue *io1=(obj1); const TValue *io2=(obj2); \
    io1->value_ = io2->value_;  /* 复制值 */ \
    settt_(io1, io2->tt_);       /* 复制类型 */ \
    checkliveness(L,io1); }      /* GC 检查 */

// 变体：栈到栈
#define setobjs2s(L,o1,o2)  setobj(L,s2v(o1),s2v(o2))

// 变体：到栈（源不是同栈）
#define setobj2s(L,o1,o2)   setobj(L,s2v(o1),o2)
```

### 4.2 luaH_get 系列

```c
// luaH_fastgeti 宏展开
#define luaH_fastgeti(t,k,res,tag) \
  { Table *h = t; \
    lua_Unsigned u = l_castS2U(k) - 1u;  /* k 是 1-based，转 0-based */ \
    if ((u < h->asize)) {  /* 在数组部分？ */ \
      tag = *getArrTag(h, u); \
      if (!tagisempty(tag)) { farr2val(h, u, tag, res); }} \
    else { tag = luaH_getint(h, (k), res); }}  /* 查哈希部分 */

// getArrTag 宏展开
#define getArrTag(t,k)  (cast(lu_byte*, (t)->array) + sizeof(unsigned) + (k))
```

### 4.3 指令解码宏

```c
// 从指令中提取 opcode
#define GET_OPCODE(i)  (cast(OpCode, ((i)>>POS_OP) & MASK1(SIZE_OP,0)))

// 从指令中提取参数 A
#define GETARG_A(i)  getarg(i, POS_A, SIZE_A)

// 解码例
#define getarg(i,pos,size)  (cast_int(((i)>>(pos)) & MASK1(size,0)))

// MASK1 定义
#define MASK1(n,p)  ((~((~(Instruction)0)<<(n)))<<(p))
```

---

## 5. GC Write Barrier

### 问题描述

Lua 的 GC 是三色增量式，写屏障必须正确实现：

```c
// 基本写屏障
#define luaC_barrier(L,t,v) \
  { if (iscollectable(v) && iswhite(obj2gco(t))) \
        luaC_barrier_(L,obj2gco(t),gcvalue(v)); }

// 写屏障：父对象是黑色，子对象要变成灰色
void luaC_barrier_ (lua_State *L, GCObject *t, GCObject *v) {
  global_State *g = G(L);
  lua_assert(isblack(t) && iswhite(v));
  if (keepinvariant(g))
    reallymarkobject(g, v);  // 变灰
}
```

### Go 中的挑战

Go 有自己的 GC，会干扰 Lua 对象的管理。必须：

1. **不要让 Go GC 扫描 Lua 对象**
2. **Lua 内部 GC 完全自己实现**
3. **Lua 对象使用 unsafe.Pointer 避免 Go GC**

```go
// 方案：使用 Go 的内存分配器，但管理自己的 GC

type GCObject interface {
    Header() *GCHeader
}

type GCHeader struct {
    Next   uintptr  // GC 链表（作为 uintptr 避免 GC）
    Tt     LuaType
    Marked uint8
}

// 标记阶段
func (g *globalState) mark() {
    for _, obj := range g.allgc {
        if !isBlack(obj) {
            markObject(obj)
        }
    }
}

// 并发安全：使用 uintptr 避免 GC
```

---

## 6. 字符串 Interning

### 问题描述

Lua 短字符串（≤ 40 字符）全局唯一：

```c
// intern 查找
static TString *internshrstr (lua_State *L, const char *str, size_t l) {
  global_State *g = G(L);
  stringtable *tb = &g->strt;
  
  unsigned int h = luaS_hash(str, l, g->seed);  // hash
  TString **list = &tb->hash[lmod(h, tb->size)];  // bucket
  
  // 遍历链表查找
  for (ts = *list; ts != NULL; ts = ts->u.hnext) {
    if (l == cast_uint(ts->shrlen) &&
        (memcmp(str, getshrstr(ts), l) == 0)) {
      if (isdead(g, ts))  // 复活
        changewhite(ts);
      return ts;
    }
  }
  // 没找到，创建新的
  // ...
}
```

### Go 等价方案

```go
// Go 的 string 已经是 interned 的，但 Lua 需要区分短/长字符串

type TString struct {
    GCHeader
    Extra    uint8     // 短字符串的 hash 标记
    Shrlen   int8      // ≥0 短字符串长度，<0 长字符串标记
    Hash     uint32    // hash 值
    // 长字符串专用
    Contents string    // 实际字符串内容
}

type stringTable struct {
    hash []*TString    // 哈希桶
    nuse int           // 元素数量
    size int           // 桶数量
}

// 短字符串 intern
func (L *LuaState) internShortString(s string) *TString {
    g := L.G
    h := hashString(s, g.seed)
    bucket := h % uint(g.strt.size)
    
    for ts := g.strt.hash[bucket]; ts != nil; ts = ts.Next {
        if ts.IsShort() && ts.Contents == s {
            if ts.IsDead() {
                ts.Resurrect()
            }
            return ts
        }
    }
    // 创建新字符串
    ts := &TString{
        Shrlen:   int8(len(s)),
        Hash:     h,
        Contents: s,
    }
    // 添加到哈希表
    // ...
    return ts
}
```

---

## 7. UpValue 的 Open/Close 语义

### 问题描述

UpValue 可以指向栈上的变量（open）或已经关闭的值（closed）：

```c
typedef struct UpVal {
  CommonHeader;
  union {
    TValue *p;           // 指向栈或自己的 value
    ptrdiff_t offset;   // 栈重分配时用
  } v;
  union {
    struct {  // open 时
      struct UpVal *next;
      struct UpVal **previous;
    } open;
    TValue value;        // closed 时的值
  } u;
} UpVal;

// 检查是否 open
#define upisopen(up)  ((up)->v.p != &(up)->u.value)

// 关闭 upvalue：将栈上的值复制到 UpVal 中
StkId luaF_close (lua_State *L, StkId level) {
  UpVal *uv;
  while ((uv = L->openupval) != NULL && uplevel(uv) >= level) {
    // 复制栈上的值到 UpVal
    setobj(L, &uv->u.value, uv->v.p);
    // 从 open 链表移除
    luaF_unlinkupval(uv);
    // 改为 closed 状态
    uv->v.p = &uv->u.value;  // 现在指向自己的 value
  }
}
```

### Go 等价方案

```go
type UpVal struct {
    Header GCHeader
    V      *TValue       // 指向栈上的值（open 时）
    Closed bool          // 是否已关闭
    Value  TValue        // closed 时的值
    
    // open 链表（用于 lua_State.openupval）
    Next   *UpVal
}

func (L *LuaState) CloseUpvals(level int) {
    for uv := L.openupval; uv != nil; uv = uv.Next {
        if uv.StackIndex() >= level {
            uv.Close()  // 关闭
        }
    }
}

func (uv *UpVal) Close() {
    if !uv.Closed {
        uv.Value = *uv.V  // 复制值
        uv.Closed = true
        uv.V = nil
    }
}
```

---

## 8. Table 的 Mainposition 哈希算法

### 问题描述

```c
// 获取 key 的主位置
static Node *mainposition (const Table *t, const TValue *key) {
  unsigned int h = luaV_hash(key, t->gcing);  // hash
  int mp = lmod(h, sizenode(t));  // 取模（2 的幂次）
  return &t->node[mp];
}

// 冲突时使用线性探测
#define gnode(t,i)  (&(t)->node[i])

static Node *getfreepos (Table *t) {
  while (t->freelist != NULL) {
    Node *n = t->freelist;
    t->freelist = n->u.next;
    return n;
  }
  // 正常探测
  for (int i = t->sizenode; i < 2*t->sizenode; i++) {
    if (keyisnil(&t->node[i]))
      return &t->node[i];
  }
  return NULL;
}
```

### Go 等价实现

```go
type Table struct {
    Header  GCHeader
    Flags   uint8
    LSizenode uint8    // log2(哈希大小)
    Asize   uint32    // 数组部分大小
    Array   []TValue  // 数组部分
    Node    []Node    // 哈希部分
    
    Freelist int      // 空闲节点链表头索引，-1 表示空
}

type Node struct {
    Key     TValue
    Val     TValue
    Next    int       // 下一个冲突节点的索引，-1 表示链表结束
}

func (t *Table) mainPosition(key *TValue) int {
    h := hashValue(key, t.G)
    size := 1 << t.LSizenode
    return int(h) & (size - 1)  // h % size，但更快
}

func (t *Table) Get(key *TValue) (TValue, bool) {
    // 先检查数组部分（整数键）
    if key.IsInteger() {
        idx := key.I - 1  // Lua 索引从 1 开始
        if idx >= 0 && idx < int(t.Asize) {
            if !t.Array[idx].IsNil() {
                return t.Array[idx], true
            }
        }
    }
    
    // 查哈希部分：⚠️修正：使用链式冲突遍历，不是线性探测
    mp := t.mainPosition(key)
    for i := mp; i != -1; i = t.Node[i].Next {
        if equalValue(&t.Node[i].Key, key) {
            return t.Node[i].Val, true
        }
    }
    return TValue{}, false  // 未找到
}
```

---

## 9. 协程 Yield/Resume

### ⚠️ 修正：原方案有误

**原方案问题**：每个 Lua 协程一个 goroutine 的方案有语义不匹配：
1. Lua 协程是**协作式**（用户控制何时 yield）
2. Go goroutine 是**抢占式**（运行时调度）
3. 无法从外部检查/修改协程栈

### 正确方案：单 Goroutine + 手动栈复制

**核心思想**：模拟 C 的行为 — `lua_yield` 通过 longjmp 保存栈状态并返回，`lua_resume` 恢复栈并继续。

```go
// LuaState 协程相关字段
type LuaState struct {
    status   Status
    parent   *LuaState       // 主线程或创建者
    stack    []TValue        // 栈（同一 goroutine）
    top      int
    savedCI  *CallInfo        // yield 时保存的调用帧
    savedTop int             // yield 时保存的栈顶
    savedPC  int             // yield 时保存的 PC
    statusBeforeYield Status  // yield 前的状态
    
    // Resume 通道（所有协程共享一个 goroutine）
    resumeCh chan struct {
        args   []Value
        result chan []Value
        err    chan error
    }
}

// lua_resume 等价
func (L *LuaState) Resume(args []Value) ([]Value, error) {
    // 检查状态
    if L.status != StatusSuspended && L.status != StatusOK {
        return nil, fmt.Errorf("cannot resume %s coroutine", L.status)
    }
    
    // 分配结果通道
    resultCh := make(chan []Value, 1)
    errCh := make(chan error, 1)
    
    // 发送 resume 请求
    L.resumeCh <- resumeRequest{
        target:   L,
        args:     args,
        resultCh: resultCh,
        errCh:    errCh,
    }
    
    select {
    case result := <-resultCh:
        return result, nil
    case err := <-errCh:
        return nil, err
    }
}

// lua_yield 等价
func (L *LuaState) Yield(nResults int) {
    // 保存当前执行状态
    L.savedCI = L.ci
    L.savedTop = L.top - nResults
    if L.ci.IsLua() {
        L.savedPC = L.ci.SavedPC
    }
    L.statusBeforeYield = L.status
    L.status = StatusSuspended
    
    // 发送 yield 结果
    results := L.stack[L.top-nResults : L.top]
    L.parent.yieldResult <- yieldResult{
        from:    L,
        results: results,
    }
    
    // 暂停，等待下次 Resume
    // 使用 channel 阻塞而非 goroutine 暂停
    yieldAck := <-L.yieldAckCh
    if yieldAck.err != nil {
        panic(yieldAck.err)
    }
}

// 协程调度器（运行在单一 goroutine 中）
func (L *LuaState) runCoroutineScheduler() {
    for {
        select {
        case req := <-L.resumeCh:
            // 处理 resume 请求
            target := req.target
            if target.status == StatusOK {
                // 首次调用
                target.pushFunction(req.args[0])
                target.pushArgs(req.args[1:]...)
                target.call(len(req.args)-1, LUA_MULTRET)
            } else {
                // 从 yield 恢复
                target.status = StatusRunning
                // 恢复栈状态
                // ...
                target.continueExecution()
            }
            
        case yr := <-L.yieldResult:
            // 协程 yield
            yr.from.status = StatusSuspended
            // 将控制权交给 resume 的调用者
            yr.from.yieldAckCh <- yieldAck{resume: yr}
        }
    }
}
```

### 关键：C 语义模拟

```c
// C 中的 lua_yieldk 本质上做了：
// 1. 保存寄存器状态 (longjmp)
// 2. 保存 PC (ci->savedpc)
// 3. 保存栈顶
// 4. 从 lua_resume 的 setjmp 点继续执行
```

**Go 中必须模拟的状态：**
1. **CallInfo 链表** — yield 时的调用帧
2. **PC** — 如果在 Lua 函数中 yield
3. **栈内容** — yield 返回的值
4. **UpValue** — open upvalue 必须正确追踪

### 陷阱 9.1：协程 vs 线程

```lua
-- Lua 协程是"半协程"，可以在 C 和 Lua 层 yield
local co = coroutine.create(function()
    coroutine.yield(1)  -- Lua yield
end)

-- 可以在 C 函数中 yield
lua_yield(L, 0);  -- C 代码
```

**注意**：C 函数中的 yield 需要 continuation 机制：
```go
// C 函数 yield 后恢复点
func cFunctionWithYield(L *LuaState) int {
    if needsYield {
        L.ci.K = resumePoint  // 设置继续函数
        L.Yield(nResults)
        // 不会执行到这里
    }
resumePoint:
    // 从这里继续
    return results
}
```

---

## 10. 数值表示

### 问题描述

Lua 5.3+ 支持 integer 和 float 两种数值类型：

```c
// variant tags
#define LUA_VNUMINT   makevariant(LUA_TNUMBER, 0)  // 整数
#define LUA_VNUMFLT   makevariant(LUA_TNUMBER, 1)  // 浮点

// 访问宏
#define ttisinteger(o)    checktag((o), LUA_VNUMINT)
#define ttisfloat(o)      checktag((o), LUA_VNUMFLT)

#define ivalue(o)   check_exp(ttisinteger(o), (o)->value_.i)
#define fltvalue(o) check_exp(ttisfloat(o), (o)->value_.n)

// 设置宏
#define setfltvalue(obj,x) \
  { (obj)->value_.n=(x); settt_((obj), LUA_VNUMFLT); }

#define setivalue(obj,x) \
  { (obj)->value_.i=(x); settt_((obj), LUA_VNUMINT); }
```

### Go 等价实现

```go
type Number struct {
    tt LuaType  // LUA_VNUMINT 或 LUA_VNUMFLT
    i  int64
    n  float64
}

func (n *Number) Int() int64 {
    if n.tt == LUA_VNUMINT {
        return n.i
    }
    return int64(n.n)
}

func (n *Number) Float() float64 {
    if n.tt == LUA_VNUMFLT {
        return n.n
    }
    return float64(n.i)
}

func MakeInteger(i int64) *Number {
    return &Number{tt: LUA_VNUMINT, i: i}
}

func MakeFloat(f float64) *Number {
    return &Number{tt: LUA_VNUMFLT, n: f}
}

// 自动类型转换
func (n *Number) Add(other *Number) *Number {
    if n.tt == LUA_VNUMINT && other.tt == LUA_VNUMINT {
        // ⚠️修正：整数溢出 wrap around，不转浮点
        // C 中使用 uint64 计算后再转回 int64（溢出是未定义，但实际是 wrap）
        result := int64(uint64(n.i) + uint64(other.i))
        return MakeInteger(result)
    }
    // 其他情况转浮点
    return MakeFloat(n.Float() + other.Float())
}
```

---

## 11. 常见陷阱总结

| 陷阱 | 错误后果 | 解决方案 |
|------|----------|----------|
| setjmp/longjmp | 协程 yield/resume 无法工作 | panic/recover 模拟 |
| Tagged union | 类型混淆、内存错乱 | Go struct + type tag |
| 指针算术 | Off-by-one、段错误 | Go slice index |
| GC write barrier | 对象泄漏、过早回收 | Lua 自己管理 GC |
| 字符串 intern | 内存浪费、比较错误 | 短字符串手动 intern |
| UpValue lifecycle | 闭包捕获错误值 | Go closure 天然处理 |
| Table hash | 死循环、性能差 | 正确实现探测算法 |
| 整数溢出 | 数值错误 | wrap around，不是转浮点 |

---

## 12. 推荐的重写策略

1. **先实现 lobject**：定义 TValue、GCObject 等核心类型
2. **然后 lmem**：实现内存分配器（可以使用 Go 的但包装）
3. **lstring 和 lfunc**：先完成这两个，它们是其他模块的基础
4. **lvm 和 ldo**：同时实现，它们互相依赖最深
5. **lgc**：最后实现，因为它需要理解所有对象类型

**测试驱动**：每个模块完成后，立即运行对应的 Lua 测试用例。