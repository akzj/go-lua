# lfunc 模块规格书

## 模块职责

管理 Lua 函数原型（Proto）、Lua 闭包（LClosure）、C 闭包（CClosure）和 UpValue。这是闭包语义的核心实现。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lobject | 所有类型定义 |
| lgc | GCObject 创建 |
| lmem | 内存分配 |
| lstate | lua_State |
| ldo | 栈操作 |

## 公开 API

```c
/* 创建函数/闭包 */
LUAI_FUNC Proto *luaF_newproto (lua_State *L);
LUAI_FUNC CClosure *luaF_newCclosure (lua_State *L, int nupvals);
LUAI_FUNC LClosure *luaF_newLclosure (lua_State *L, int nupvals);
LUAI_FUNC void luaF_initupvals (lua_State *L, LClosure *cl);

/* UpValue 管理 */
LUAI_FUNC UpVal *luaF_findupval (lua_State *L, StkId level);
LUAI_FUNC void luaF_newtbcupval (lua_State *L, StkId level);
LUAI_FUNC void luaF_closeupval (lua_State *L, StkId level);
LUAI_FUNC StkId luaF_close (lua_State *L, StkId level, TStatus status, int yy);
LUAI_FUNC void luaF_unlinkupval (UpVal *uv);

/* 调试 */
LUAI_FUNC const char *luaF_getlocalname (const Proto *func, int local_number, int pc);

/* 内存 */
LUAI_FUNC lu_mem luaF_protosize (Proto *p);
LUAI_FUNC void luaF_freeproto (lua_State *L, Proto *f);
```

## 核心数据结构

### C 定义

```c
/* UpVal 描述（在 Proto 中，用于调试） */
typedef struct Upvaldesc {
  TString *name;        /* upvalue 名称 */
  lu_byte instack;      /* 是否在栈上 */
  lu_byte idx;          /* 栈索引或外层 upvalue 索引 */
  lu_byte kind;         /* 变量种类 */
} Upvaldesc;

/* 函数原型 */
typedef struct Proto {
  CommonHeader;
  lu_byte numparams;    /* 固定参数个数 */
  lu_byte flag;         /* 标志位 */
  lu_byte maxstacksize; /* 需要的最大栈寄存器 */
  int sizek, sizecode, sizep;
  int sizelineinfo, sizelocvars, sizeabslineinfo;
  int linedefined, lastlinedefined;
  
  TValue *k;            /* 常量表 */
  Instruction *code;     /* 字节码 */
  struct Proto **p;      /* 嵌套函数 */
  Upvaldesc *upvalues;  /* upvalue 描述 */
  ls_byte *lineinfo;     /* 行号信息 */
  AbsLineInfo *abslineinfo;
  LocVar *locvars;       /* 局部变量信息 */
  TString *source;       /* 源码标识 */
  GCObject *gclist;
} Proto;

/* UpValue */
typedef struct UpVal {
  CommonHeader;
  union {
    TValue *p;           /* 指向栈上的值（open 时） */
    ptrdiff_t offset;    /* 栈重分配时用 */
  } v;
  union {
    struct {             /* open 时 */
      struct UpVal *next;
      struct UpVal **previous;
    } open;
    TValue value;         /* closed 时的值 */
  } u;
} UpVal;

/* Lua 闭包 */
typedef struct LClosure {
  ClosureHeader;
  struct Proto *p;
  UpVal *upvals[1];      /* 可变长度 */
} LClosure;

/* C 闭包 */
typedef struct CClosure {
  ClosureHeader;
  lua_CFunction f;
  TValue upvalue[1];      /* 可变长度 */
} CClosure;

/* 闭包联合体 */
typedef union Closure {
  CClosure c;
  LClosure l;
} Closure;
```

## Go 重写规格

### 类型定义

```go
package lua

// UpVal 描述（用于调试信息）
type UpvalDesc struct {
    Name    *TString
    InStack uint8     // 是否在栈上
    Idx     uint8     // 栈索引或外层索引
    Kind    uint8     // 变量种类
}

// LocVar 局部变量信息（用于调试）
type LocVar struct {
    Varname *TString
    StartPC int       // 开始有效的 PC
    EndPC   int       // 失效的 PC
}

// Proto 函数原型（编译时生成）
type Proto struct {
    Header      GCHeader
    NumParams   uint8
    Flag        uint8
    MaxStackSize uint8
    Sizek       int
    SizeCode    int
    SizeP       int
    SizeLineInfo int
    SizeLocVars int
    SizeAbsLineInfo int
    LineDefined int
    LastLineDefined int
    
    // 数据
    K          []TValue      // 常量表
    Code       []Instruction // 字节码
    P          []*Proto      // 嵌套函数
    Upvalues   []UpvalDesc   // upvalue 描述
    LineInfo   []int8        // 行号增量
    AbsLineInfo []AbsLineInfo
    LocVars    []LocVar
    Source     *TString
    Gclist     *GCHeader
}

// AbsLineInfo 绝对行号
type AbsLineInfo struct {
    PC   int
    Line int
}

// UpValue
type UpVal struct {
    Header GCHeader
    V      *TValue  // open 时指向栈，closed 时指向自己的 Value
    
    // open 时用的链表
    OpenNext *UpVal
    OpenPrev **UpVal
    
    Closed bool    // 是否已关闭
    Value  TValue  // closed 时的值
}

// LClosure Lua 闭包
type LClosure struct {
    Header GCHeader
    Nupvalues uint8
    Gclist    *GCHeader
    Proto     *Proto
    Upvals    []*UpVal
}

// CClosure C 闭包
type CClosure struct {
    Header GCHeader
    Nupvalues uint8
    Gclist    *GCHeader
    Fn       GoFunction  // Go 实现的函数
    Upvals    []TValue   // upvalue 值
}

// Closure 闭包联合
type Closure interface {
    IsC() bool
}

func (c *LClosure) IsC() bool { return false }
func (c *CClosure) IsC() bool { return true }
```

### 关键实现

#### 1. 创建闭包

```go
// luaF_newLclosure
func (L *LuaState) NewLClosure(proto *Proto) *LClosure {
    nup := proto.SizeUpvalues
    cl := &LClosure{
        Proto: proto,
        Upvals: make([]*UpVal, nup),
    }
    // 设置 GC header
    L.GC.NewObj(cl, LUA_VLCL)
    return cl
}

// luaF_newCclosure
func (L *LuaState) NewCClosure(fn GoFunction, nup int) *CClosure {
    cl := &CClosure{
        Fn: fn,
        Upvals: make([]TValue, nup),
    }
    L.GC.NewObj(cl, LUA_VCCL)
    return cl
}
```

#### 2. UpValue 查找

UpValue 的核心语义：
- **open upvalue**：指向栈上的变量，可以追踪栈的变化
- **closed upvalue**：值已经"关闭"，不再依赖栈

```go
// luaF_findupval: 查找或创建 upvalue
func (L *LuaState) FindUpval(level int) *UpVal {
    // level 是栈索引（相对于栈底）
    // 查找 open upvalue 链表
    for uv := L.OpenUpval; uv != nil; uv = uv.OpenNext {
        if uv.StackIndex() == level {
            return uv
        }
    }
    
    // 没找到，创建新的
    uv := &UpVal{
        V: &L.Stack[level],  // 指向栈
    }
    
    // 插入链表头部
    uv.OpenNext = L.OpenUpval
    if L.OpenUpval != nil {
        L.OpenUpval.OpenPrev = &uv.OpenNext
    }
    L.OpenUpval = uv
    uv.OpenPrev = &L.OpenUpval
    
    return uv
}

// luaF_close: 关闭所有 >= level 的 upvalue
func (L *LuaState) CloseUpvals(level int) {
    for uv := L.OpenUpval; uv != nil; uv = uv.OpenNext {
        if uv.StackIndex() >= level {
            uv.Close()  // 关闭
        }
    }
}

func (uv *UpVal) Close() {
    if !uv.Closed {
        // 复制当前值
        uv.Value = *uv.V
        uv.Closed = true
        // 从链表中移除
        *uv.OpenPrev = uv.OpenNext
        if uv.OpenNext != nil {
            uv.OpenNext.OpenPrev = uv.OpenPrev
        }
        uv.V = &uv.Value  // 现在指向自己的值
    }
}
```

#### 3. UpValue 生命周期

```
闭包创建时:
┌──────────────────────────────────────┐
│  function outer()                   │
│    local x = 10                     │
│    function inner() return x end    │ ──┐ Create upvalue for x
│    return inner                     │    │
└──────────────────────────────────────┘    │
                                           │
闭包返回后:                               │
┌──────────────────────────────────────┐    │
│  inner = outer()                     │    │
│  -- x 在栈上已经不存在了             │    │
│  -- upvalue 变成 closed              │    │
│  assert(inner() == 10)               │    │
└──────────────────────────────────────┘    │
                                           │
UpValue 关闭过程:                         │
  open upvalue: uv.V → &stack[x] ──────────┘
       ↓ Close()
  closed upvalue: uv.V → &uv.Value
```

#### 4. 函数调用时的 UpValue 处理

```go
// luaV_execute 中的闭包创建（OP_CLOSURE）
case OP_CLOSURE: {
    LClosure *cl;
    Proto *p = cl->p->p[GETARG_Bx(i)];  // 获取子函数原型
    cl = luaF_newLclosure(L, p->sizeupvalues);
    cl->p = p;
    
    // 初始化 upvalue
    for (int n = 0; n < p->sizeupvalues; n++) {
        Instruction av = cl->p->p[base + n];
        if (GET_OPCODE(av) == OP_GETUPVAL) {
            // 从外层复制 upvalue
            cl->upvals[n] = base[GETARG_B(av)].cl->upvals[GETARG_C(av)];
        } else {
            // 从栈上引用 (OP_MOVE)
            cl->upvals[n] = luaF_findupval(L, base + GETARG_B(av));
        }
    }
    // ...
}
```

### 陷阱和注意事项

#### 陷阱 1: UpValue 栈索引 vs 指针

C 中 UpVal.v.p 是指针，栈重分配时需要修正：

```c
// 栈重分配前
static void relstack (lua_State *L) {
  for (up = L->openupval; up != NULL; up = up->u.open.next)
    up->v.offset = savestack(L, uplevel(up));  // 转偏移
}

// 栈重分配后
static void correctstack (lua_State *L) {
  for (up = L->openupval; up != NULL; up = up->u.open.next)
    up->v.p = s2v(restorestack(L, up->v.offset));  // 恢复指针
}
```

**Go 方案：使用 slice index，不需要修正**
```go
type UpVal struct {
    StackIdx int  // 栈索引（open 时）
    // 不需要 offset，因为 slice 会自动处理
}
```

#### 陷阱 2: 多层闭包的 UpValue 链

```lua
-- 三层闭包
local function a()
    local x = 1
    local function b()
        local function c()
            return x  -- c 的 upvalue 指向 b 的 upvalue，后者指向 a 的栈
        end
        return c
    end
    return b
end
```

**Go 方案：**
```go
// upvalue 始终指向栈上变量（open）或自己的值（closed）
// 闭包间的 upvalue 共享通过 LuaClosure 的 Upvals 数组实现
```

#### 陷阱 3: to-be-closed 变量

Lua 5.4+ 支持 `to-be-closed` 变量（类似 defer）：

```lua
local f <close> = io.open("file")
-- 当作用域结束时，自动调用 f:close()
```

**Go 方案：**
```go
// CallInfo 中记录 tbclist
type CallInfo struct {
    Base     int
    Top      int
    SavedPC  int
    TBCList  []int  // to-be-closed 变量索引
    
    // 其他字段...
}

func (L *LuaState) ReturnTo(level int) {
    // 执行所有 to-be-closed 变量的 __close
    for _, idx := range L.TBCList {
        if idx >= level {
            L.GetFromStack(idx).CallMethod("close")
        }
    }
}
```

### 验证测试

```lua
-- closure.lua 关键测试

-- 基本闭包
local function counter()
    local count = 0
    return function()
        count = count + 1
        return count
    end
end
local c = counter()
assert(c() == 1)
assert(c() == 2)

-- 多层闭包
local function outer()
    local x = 10
    local function middle()
        local function inner()
            return x  -- 访问外层变量
        end
        return inner
    end
    return middle
end
local m = outer()
local i = m()
assert(i() == 10)

-- upvalue 独立性
local function make_adder(n)
    return function(x)
        return x + n
    end
end
local add5 = make_adder(5)
local add10 = make_adder(10)
assert(add5(1) == 6)
assert(add10(1) == 11)

-- to-be-closed
local function test_tbc()
    local opened = false
    local function with_resource()
        opened = true
        return function()
            return "closed"
        end
    end
    local r <close> = with_resource()
    assert(opened == true)
end
test_tbc()  -- r 的 close 方法被调用
```

### 性能考虑

1. **闭包创建开销**：每次创建闭包都要分配 UpVal 数组
2. **UpValue 查找**：维护 open upvalue 链表，查找 O(n)
3. **优化方案**：大多数函数 upvalue 数量固定且较少，可以缓存

### 与 lvm 的交互

```go
// luaV_execute 中 OP_GETUPVAL
case OP_GETUPVAL: {
    TValue *base = L.Base()
    int a = GETARG_A(i);
    int b = GETARG_B(i);  // upvalue 索引
    LClosure *cl = base[0].Cl()
    setobj(L, &base[a], cl.Upvals[b].Get())  // 获取 upvalue 的值
    break;
}

// luaV_execute 中 OP_SETUPVAL
case OP_SETUPVAL: {
    int a = GETARG_A(i);
    int b = GETARG_B(i);
    LClosure *cl = base[0].Cl()
    cl.Upvals[b].Set(&base[a])  // 设置 upvalue 的值
    break;
}

// luaV_execute 中 OP_CLOSURE
case OP_CLOSURE: {
    Proto *p = L.Protos()[GETARG_Bx(i)]
    LClosure *cl = L.NewLClosure(p)
    // 根据 upvalue 描述初始化每个 upvalue
    for n := 0; n < p.NumUpvalues; n++ {
        inst := L.CurrentInstruction()
        if GET_OPCODE(inst) == OP_GETUPVAL {
            // 从外层复制
            cl.Upvals[n] = outer.Upvals[GETARG_C(inst)]
        } else {
            // 从当前栈引用
            cl.Upvals[n] = L.FindUpval(base + GETARG_B(inst))
        }
    }
    base[a].SetClosure(cl)
    break;
}
```