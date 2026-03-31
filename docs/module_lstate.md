# lstate 模块规格书

## 模块职责

管理 Lua 全局状态（global_State）和每个线程状态（lua_State）。包括 GC 链表管理、字符串表、注册表、metatable 等全局资源。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lobject | 所有类型定义 |
| lgc | GC 初始化 |
| lstring | 字符串表初始化 |
| ltm | 元表初始化 |

## 公开 API

```c
/* 状态创建/销毁 */
LUAI_FUNC lua_State *lua_newstate (lua_Alloc f, void *ud);
LUAI_FUNC void lua_close (lua_State *L);

/* 线程操作 */
LUAI_FUNC lua_State *lua_newthread (lua_State *L);
LUAI_FUNC lua_State *luaE_freethread (lua_State *L, lua_State *L1);

/* CI 管理 */
LUAI_FUNC CallInfo *luaE_extendCI (lua_State *L, int err);
LUAI_FUNC void luaE_shrinkCI (lua_State *L);
LUAI_FUNC void luaE_checkcstack (lua_State *L);
LUAI_FUNC void luaE_incCstack (lua_State *L);

/* 警告 */
LUAI_FUNC void luaE_warning (lua_State *L, const char *msg, int tocont);
LUAI_FUNC void luaE_warnerror (lua_State *L, const char *where);

/* 线程大小 */
LUAI_FUNC lu_mem luaE_threadsize (lua_State *L);

/* GC */
LUAI_FUNC void luaE_setdebt (global_State *g, l_mem debt);
LUAI_FUNC TStatus luaE_resetthread (lua_State *L, TStatus status);
```

## 核心数据结构

### lua_State (线程状态)

```c
struct lua_State {
  CommonHeader;
  lu_byte allowhook;      /* 是否允许 hook */
  TStatus status;         /* 运行状态 */
  StkIdRel top;           /* 栈顶 */
  struct global_State *l_G;
  CallInfo *ci;          /* 当前调用帧 */
  StkIdRel stack_last;   /* 栈末尾 */
  StkIdRel stack;        /* 栈起始 */
  UpVal *openupval;      /* 开放的 upvalue 链表 */
  StkIdRel tbclist;      /* to-be-closed 变量 */
  GCObject *gclist;       /* GC 链表 */
  struct lua_State *twups; /* 有 open upvalue 的线程链表 */
  struct lua_longjmp *errorJmp; /* 错误处理帧 */
  CallInfo base_ci;       /* 基帧 */
  volatile lua_Hook hook;
  ptrdiff_t errfunc;     /* 错误处理函数 */
  l_uint32 nCcalls;       /* C 调用计数 */
  int oldpc;             /* 上次 trace 的 PC */
  int nci;               /* CI 数量 */
  int basehookcount;
  int hookcount;
  volatile l_signalT hookmask;
  struct { int ftransfer, ntransfer; } transferinfo;
};
```

### global_State (全局状态)

```c
typedef struct global_State {
  lua_Alloc frealloc;    /* 内存分配器 */
  void *ud;             /* 分配器用户数据 */
  l_mem GCtotalbytes;   /* 已分配内存 */
  l_mem GCdebt;         /* GC 预算 */
  l_mem GCmarked;       /* 本轮标记的对象数 */
  l_mem GCmajorminor;   /* 分代 GC 计数器 */
  stringtable strt;     /* 字符串表 */
  TValue l_registry;     /* 注册表 */
  TValue nilvalue;      /* 唯一 nil 值 */
  unsigned int seed;    /* 哈希种子 */
  lu_byte gcparams[LUA_GCPN];
  lu_byte currentwhite;  /* 当前白色 */
  lu_byte gcstate;      /* GC 状态 */
  lu_byte gckind;       /* GC 模式 */
  lu_byte gcstopem;     /* 停止紧急 GC */
  lu_byte gcstp;        /* GC 控制 */
  lu_byte gcemergency;  /* 紧急 GC */
  GCObject *allgc;      /* 所有对象链表 */
  GCObject **sweepgc;   /* sweep 位置 */
  GCObject *finobj;     /* 有终结器的对象 */
  GCObject *gray;        /* 灰色对象 */
  GCObject *grayagain;  /* 需要原子处理的灰色 */
  GCObject *weak;       /* 弱表 */
  GCObject *ephemeron;  /* 弱键表 */
  GCObject *allweak;    /* 所有弱表 */
  GCObject *tobefnz;    /* 待终结的对象 */
  GCObject *fixedgc;    /* 固定不回收的对象 */
  /* 分代 GC */
  GCObject *survival;   /* 存活对象 */
  GCObject *old1;       /* 老对象 */
  GCObject *reallyold;  /* 真正的老对象 */
  GCObject *firstold1;  /* 第一个 old1 */
  GCObject *finobjsur;
  GCObject *finobjold1;
  GCObject *finobjrold;
  struct lua_State *twups; /* 有 open upvalue 的线程 */
  lua_CFunction panic;  /* 恐慌函数 */
  TString *memerrmsg;  /* 内存错误消息 */
  TString *tmname[TM_N]; /* 元方法名 */
  struct Table *mt[LUA_NUMTYPES]; /* 基本类型的元表 */
  TString *strcache[STRCACHE_N][STRCACHE_M]; /* API 字符串缓存 */
  lua_WarnFunction warnf;
  void *ud_warn;
  LX mainth;            /* 主线程 */
} global_State;
```

## Go 重写规格

```go
package lua

// LuaState 单个线程/协程状态
type LuaState struct {
    Header     GCHeader
    AllowHook  bool
    Status     Status
    Top        int
    
    G          *GlobalState  // 全局状态指针
    CI         *CallInfo     // 当前调用帧
    Stack      []TValue      // 栈
    StackLast  int           // 栈末尾
    
    OpenUpval  *UpVal        // 开放的 upvalue
    TBCList    int           // to-be-closed 变量
    
    GCList     *GCHeader     // GC 链表
    Twups      *LuaState     // twups 链表
    
    ErrorJmp   *LJmp         // 错误处理帧
    BaseCI     *CallInfo     // 基帧
    
    Hook       HookFunc
    HookMask   uint8
    HookCount  int
    BaseHookCount int
    
    ErrFunc    int           // 错误处理函数
    NCcalls    uint32        // C 调用计数
    OldPC      int
    
    TransferInfo struct {
        FTransfer int
        NTransfer int
    }
}

// GlobalState 全局状态
type GlobalState struct {
    Frealloc   AllocFunc
    Ud          interface{}
    
    TotalBytes int64
    Debt       int64
    Marked      int64
    MajorMinor  int64
    
    Strt       StringTable    // 字符串表
    Registry   TValue         // 注册表
    NilValue   TValue         // nil 值
    
    Seed       uint32         // 哈希种子
    
    GCParams   [LUA_GCPN]uint8
    CurrentWhite uint8
    GCState    GCState
    GCKind     GCKind
    GCStopEM   bool
    GCStp      uint8
    GCEmergency bool
    
    // GC 链表
    AllGC      *GCHeader
    SweepGC    **GCHeader
    FinObj     *GCHeader
    Gray       *GCHeader
    GrayAgain  *GCHeader
    Weak       *GCHeader
    Ephemeron  *GCHeader
    AllWeak    *GCHeader
    ToBeFnz    *GCHeader
    FixedGC    *GCHeader
    
    // 分代 GC
    Survival   *GCHeader
    Old1       *GCHeader
    ReallyOld  *GCHeader
    FirstOld1  *GCHeader
    FinObjSur  *GCHeader
    FinObjOld1 *GCHeader
    FinObjROld *GCHeader
    
    Twups      *LuaState
    Panic      PanicFunc
    MemErrMsg  *TString
    
    TMName     [TM_N]*TString
    Mt         [LUA_NUMTYPES]*Table
    
    StrCache   [STRCACHE_N][STRCACHE_M]*TString
    
    WarnFn     WarnFunc
    WarnUd     interface{}
    
    MainThread *LuaState
}
```

## 注册表 (Registry)

```go
// 注册表是全局表，用于 C 和 Lua 之间的通信
// LUA_REGISTRYINDEX 引用此表

// 预定义注册表索引
const (
    LUA_RIDX_GLOBALS     = 2  // 全局环境表
    LUA_RIDX_MAINTHREAD  = 1  // 主线程
    LUA_RIDX_GC          = 3  // GC 参数
)

// 注册表初始化
func (g *GlobalState) initRegistry() {
    // 创建全局表
    g.Registry.SetTable(newTable())
    
    // 设置全局表到注册表
    reg := g.Registry.AsTable()
    reg.SetInt(LUA_RIDX_GLOBALS, newTable())
    reg.SetInt(LUA_RIDX_MAINTHREAD, g.MainThread)
}
```

## twups 链表（线程 upvalue 链）

```go
// twups 链表连接所有有 open upvalue 的线程
// 用于 GC 和栈收缩

func (L *LuaState) addToTwups() {
    L.Twups = L.G.Twups
    L.G.Twups = L
}

func (L *LuaState) removeFromTwups() {
    var prev **LuaState = &L.G.Twups
    for t := L.G.Twups; t != nil; t = t.Twups {
        if t == L {
            *prev = t.Twups
            return
        }
        prev = &t.Twups
    }
}
```

## 线程创建/销毁

```go
// lua_newstate
func NewState(alloc AllocFunc, ud interface{}) *LuaState {
    // 创建全局状态
    g := &GlobalState{
        Frealloc: alloc,
        Ud:       ud,
        Seed:     randUint32(),
    }
    
    // 创建主线程
    L := &LuaState{
        G:     g,
        Stack: make([]TValue, BASIC_STACK_SIZE),
    }
    L.Top = 0
    
    // 主线程指向自己
    L.Header.Next = nil
    L.Header.Tt = LUA_VTHREAD
    
    g.MainThread = L
    g.AllGC = &L.Header
    
    // 初始化注册表
    g.initRegistry()
    
    // 初始化字符串表
    initStringTable(&g.Strt)
    
    // 初始化元表
    initMetatables(g)
    
    // 初始化 GC
    initGC(g)
    
    return L
}

// lua_close
func (L *LuaState) Close() {
    g := L.G
    
    // 先关闭所有线程
    for th := g.Twups; th != nil; th = th.Twups {
        L.Close()
    }
    
    // GC 全部回收
    L.fullGC()
    
    // 释放字符串表
    freeStringTable(g)
    
    // 释放全局状态
    g.Frealloc(g.Ud, unsafe.Pointer(g), 0, 0)
}

// lua_newthread
func (L *LuaState) NewThread() *LuaState {
    g := L.G
    
    // 创建新线程
    L1 := &LuaState{
        G:     g,
        Stack: make([]TValue, BASIC_STACK_SIZE),
    }
    L1.Header.Tt = LUA_VTHREAD
    
    // 添加到 GC 链表
    L1.GCList = g.AllGC
    g.AllGC = &L1.Header
    
    // 添加到 twups
    L1.addToTwups()
    
    // 设置 __ENV 作为第一个 upvalue
    // ...
    
    return L1
}
```

## GC 链表操作

```go
// GC 对象链表是 Lua GC 的基础
// 所有可回收对象通过 CommonHeader.next 链接

type GCHeader struct {
    Next   *GCHeader
    Tt     LuaType
    Marked uint8
}

// 添加到 allgc 链表
func (L *LuaState) newObject(obj GCObject, tt LuaType) {
    h := obj.GetHeader()
    h.Tt = tt
    h.Marked = L.G.CurrentWhite
    h.Next = L.G.AllGC
    L.G.AllGC = h
}

// 移除对象（sweep 时）
func (g *GlobalState) removeFromGCList(h *GCHeader) {
    prev := &g.AllGC
    for curr := g.AllGC; curr != nil; curr = curr.Next {
        if curr == h {
            *prev = curr.Next
            return
        }
        prev = &curr.Next
    }
}
```

## 陷阱和注意事项

### 陷阱 1: 线程链表一致性

```go
// 所有有 open upvalue 的线程必须在 twups 链表中
// 否则 GC 可能无法正确遍历

func (L *LuaState) FindUpval(level int) *UpVal {
    // 创建/查找 upvalue
    // ...
    // 确保线程在 twups 中
    if L.Twups == nil {
        L.addToTwups()
    }
    return uv
}
```

### 陷阱 2: 主线程特殊处理

```go
// 主线程的 lua_State 是 global_State 的一部分
// 销毁时需要特殊处理

type LX struct {
    Extra [LUA_EXTRASPACE]byte
    L     LuaState
}

type GlobalState struct {
    // ...
    MainTh LX  // 主线程嵌入式存储
}
```

### 陷阱 3: 错误状态传播

```c
// luaE_resetthread 在错误时重置线程状态
TStatus luaE_resetthread (lua_State *L, TStatus status) {
    L->status = status;
    L->ci = &L->base_ci;  // 重置调用帧
    L->top.p = L->ci->func.p + 1;  // 重置栈
    luaF_closeupval(L, L->stack.p);  // 关闭 upvalue
    luaD_seterrorobj(L, status, L->top.p);  // 设置错误对象
}
```

## 验证测试

```lua
-- 基本线程操作
local co = coroutine.create(function()
    return coroutine.running()
end)
local main, isMain = coroutine.resume(co)
assert(main == co and isMain == false)

-- 注册表
assert(type(rawget(_G, "key")) == "nil")
_G.key = "value"
assert(rawget(_G, "key") == "value")

-- 全局环境
assert(_G == _ENV)
assert(getmetatable({}) == nil)  -- 普通表的元表是 nil
```