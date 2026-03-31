# 模块规格书：lgc — 垃圾回收器

## 模块概述

**文件名**: `lua-master/lgc.c` (1804 行) + `lua-master/lgc.h` (268 行)

**核心职责**: Lua 5.5 的垃圾回收器实现，采用**三色增量式标记-清除算法**，支持**分代模式**。管理所有可回收对象（GCObject）的生命周期，包括标记、遍历、清除和终结器调用。

**目标读者**: 能写 Go 但不熟悉 Lua 内部实现的开发者

---

## 1. GC 状态机

### 1.1 状态定义（lgc.h 第 21-29 行）

```c
#define GCSpropagate    0   // 传播/标记阶段
#define GCSenteratomic  1   // 进入原子阶段（增量GC）
#define GCSatomic       2   // 原子阶段
#define GCSswpallgc     3   // 扫描 allgc 列表
#define GCSswpfinobj    4   // 扫描 finobj 列表（有终结器的对象）
#define GCSswptobefnz   5   // 扫描待终结列表
#define GCSswpend       6   // 扫描结束
#define GCScallfin      7   // 调用终结器
#define GCSpause        8   // 暂停（新周期开始）
```

### 1.2 状态转换图

```
                        ┌─────────────────────────────────────────────────────┐
                        │                      KGC_GENMINOR                   │
                        │  ┌──────────┐    ┌──────────────┐                  │
                        │  │GCSpropagate────▶│ GCSenteratomic │                 │
                        │  └──────────┘    └───────┬───────┘                  │
                        │         ▲                │                          │
                        │         │                ▼                          │
                        │         │         ┌──────────┐    minor2inc        │
                        │         │         │GCSswpallgc │────────────────────┘
                        │         │         └────┬─────┘                      │
                        │         │              │ sweepgen()                  │
                        │         │              ▼                             │
                        │         │         ┌──────────┐                      │
                        │         └─────────│GCSpause │                      │
                        │                   └──────────┘                      │
                        │                         ▲                            │
                        └─────────────────────────┼────────────────────────────┘
                                                  │
                          ┌───────────────────────┼───────────────────────┐
                          │      KGC_INC / KGC_GENMAJOR                  │
                          │                                               │
    ┌──────────┐    ┌─────▼───────┐    ┌─────────┐    ┌──────────┐    ┌──▼──┐
    │GCSpause  │────▶│GCSpropagate│───▶│GCSatomic│───▶│GCSswpallgc│──▶│GCSpause│
    └──────────┘    └─────────────┘    └─────────┘    └────┬─────┘    └──┬──┘
       ▲                  │                              │              │
       │                  ▼                              ▼              │
       │           ┌──────────┐              ┌─────────────────────┐      │
       │           │propagatemark│           │ GCSswpfinobj       │      │
       │           │(遍历灰色) │              │ GCSswptobefnz      │      │
       │           └──────────┘              │ GCSswpend          │      │
       │                                      └─────────────────────┘      │
       │                                                                  │
       └──────────────────────────────────────────────────────────────────┘
```

### 1.3 核心步骤函数（lgc.c 第 1623-1700 行）

```c
// singlestep 函数是增量GC的核心
static l_mem singlestep (lua_State *L, int fast) {
  global_State *g = G(L);
  switch (g->gcstate) {
    case GCSpause: {
      restartcollection(g);      // 重启：标记根集合
      g->gcstate = GCSpropagate;
      return 1;
    }
    case GCSpropagate: {
      // 如果没有灰色对象或fast模式，进入原子阶段
      if (fast || g->gray == NULL) {
        g->gcstate = GCSenteratomic;
        return 1;
      }
      return propagatemark(g);   // 遍历一个灰色对象
    }
    case GCSenteratomic: {
      atomic(L);                 // 执行原子操作
      if (checkmajorminor(L, g))
        return step2minor;      // 切换到分代minor
      entersweep(L);            // 开始扫描
      return atomicstep;
    }
    case GCSswpallgc:
      return sweepstep(L, g, GCSswpfinobj, &g->finobj, fast);
    case GCSswpfinobj:
      return sweepstep(L, g, GCSswptobefnz, &g->tobefnz, fast);
    case GCSswptobefnz:
      return sweepstep(L, g, GCSswpend, NULL, fast);
    case GCSswpend:
      checkSizes(L, g);         // 可能收缩字符串表
      g->gcstate = GCScallfin;
      return GCSWEEPMAX;
    case GCScallfin:
      if (g->tobefnz && luaD_checkminstack(L)) {
        GCTM(L);                 // 调用一个终结器
        return CWUFIN;
      }
      g->gcstate = GCSpause;    // 完成一个周期
      return step2pause;
  }
}
```

---

## 2. 三色标记算法

### 2.1 颜色定义（lgc.h 第 36-56 行）

```c
// marked 字段的位布局：
// bits 0-2: 分代年龄 (age)
// bit 3:    WHITE0BIT (白色0)
// bit 4:    WHITE1BIT (白色1)
// bit 5:    BLACKBIT  (黑色)
// bit 6:    FINALIZEDBIT (已终结)
// bit 7:    TESTBIT (测试用)

#define WHITE0BIT   3
#define WHITE1BIT   4
#define BLACKBIT    5
#define FINALIZEDBIT 6

#define WHITEBITS   bit2mask(WHITE0BIT, WHITE1BIT)

// 颜色判断宏
#define iswhite(x)    testbits((x)->marked, WHITEBITS)
#define isblack(x)    testbit((x)->marked, BLACKBIT)
#define isgray(x)     (!testbits((x)->marked, WHITEBITS | bitmask(BLACKBIT)))
// isgray = 既不是白色也不是黑色 = 灰色
```

### 2.2 两色白色系统

Lua 使用**两色白色系统**（white0 和 white1）实现**颜色翻转**：

```c
// 当前"活动白色"是哪个
#define luaC_white(g)  cast_byte((g)->currentwhite & WHITEBITS)

// 翻转白色（在 atomic 阶段结束时调用）
#define otherwhite(g)  ((g)->currentwhite ^ WHITEBITS)

// 翻转操作（lgc.c atomic 函数末尾）
g->currentwhite = cast_byte(otherwhite(g));  // white0 <-> white1
```

**为什么需要两个白色？**
- 增量 GC 中，标记和扫描交替进行
- sweep 阶段会把对象变成"当前白色"
- 但标记阶段可能还没完成
- 颜色翻转后，之前 sweep 产生的"旧白色"变成"旧黑色"，不会再被错误回收

### 2.3 三色不变量

> **核心不变量**: 黑色对象**永远不能**指向白色对象

如果违反这个不变量：
- 白色对象可能是垃圾但不会被回收
- 或者程序访问已删除的对象

### 2.4 标记过程

```c
// lgc.c reallymarkobject (第 339-386 行)
static void reallymarkobject (global_State *g, GCObject *o) {
  g->GCmarked += objsize(o);  // 累计已标记字节数
  switch (o->tt) {
    case LUA_VSHRSTR:
    case LUA_VLNGSTR: {
      set2black(o);            // 字符串直接变黑，无子节点
      break;
    }
    case LUA_VUPVAL: {
      UpVal *uv = gco2upv(o);
      if (upisopen(uv))
        set2gray(uv);          // 开放的 upvalue 保持灰色
      else
        set2black(uv);         // 关闭的 upvalue 变黑
      markvalue(g, uv->v.p);   // 标记其值
      break;
    }
    case LUA_VUSERDATA: {
      // ... 可能有 user values
    }  // FALLTHROUGH
    case LUA_VLCL: case LUA_VCCL: case LUA_VTABLE:
    case LUA_VTHREAD: case LUA_VPROTO: {
      linkobjgclist(o, g->gray);  // 加入灰色链表，待后续遍历
      break;
    }
  }
}

// propagatemark: 从灰色链表中取出一个对象遍历
// lgc.c 第 727-745 行
static l_mem propagatemark (global_State *g) {
  GCObject *o = g->gray;
  nw2black(o);                 // 变黑（no-white-to-black）
  g->gray = *getgclist(o);    // 从灰色链表移除
  switch (o->tt) {
    case LUA_VTABLE: return traversetable(g, gco2t(o));
    case LUA_VUSERDATA: return traverseudata(g, gco2u(o));
    // ... 其他类型
  }
}
```

### 2.5 原子阶段（atomic 函数，lgc.c 第 1543-1595 行）

atomic 阶段是增量 GC 中**不可中断的部分**，必须一次性完成：

```c
static void atomic (lua_State *L) {
  global_State *g = G(L);
  g->gcstate = GCSatomic;
  
  markobject(g, L);           // 标记正在运行的线程
  markvalue(g, &g->l_registry);  // 标记注册表
  markmt(g);                  // 标记全局元表
  propagateall(g);            // 完成传播阶段
  
  remarkupvals(g);            // 重新标记 upvalues
  propagateall(g);            // 再次传播
  
  g->gray = grayagain;
  propagateall(g);            // 遍历 grayagain
  convergeephemerons(g);      // 处理 ephemeron 表
  
  // 此时所有强可达对象都已标记
  clearbyvalues(g, g->weak, NULL);    // 清除弱值表
  clearbyvalues(g, g->allweak, NULL); // 清除所有弱表
  
  separatetobefnz(g, 0);      // 分离待终结对象
  markbeingfnz(g);            // 标记将被终结的对象
  propagateall(g);            // 传播复活
  convergeephemerons(g);
  
  clearbykeys(g, g->ephemeron);     // 清除 ephemeron 键
  clearbykeys(g, g->allweak);
  
  g->currentwhite = cast_byte(otherwhite(g));  // 翻转白色
}
```

---

## 3. 增量 GC

### 3.1 luaC_step 函数（lgc.c 第 1740-1768 行）

```c
void luaC_step (lua_State *L) {
  global_State *g = G(L);
  if (!gcrunning(g)) {
    if (g->gcstp & GCSTPUSR)  // 用户停止GC？
      luaE_setdebt(g, 20000);
    return;
  }
  
  switch (g->gckind) {
    case KGC_INC: case KGC_GENMAJOR:
      incstep(L, g);          // 增量/重大GC一步
      break;
    case KGC_GENMINOR:
      youngcollection(L, g);  // 年轻代收集
      setminordebt(g);
      break;
  }
}
```

### 3.2 GC 债务系统

Lua 使用**债务（debt）系统**控制 GC 何时运行：

```c
// 全局状态中的债务字段
l_mem GCdebt;        // 已分配但未"偿还"的字节数

// 每次分配后调用（lmem.c）
#define luaC_condGC(L,pre,pos) \
  { if (G(L)->GCdebt <= 0) { pre; luaC_step(L); pos;}; }

// 每分配一个字节，债务增加
void *luaM_realloc_(lua_State *L, void *block, size_t osize, size_t nsize) {
  // ...
  g->GCdebt -= cast(l_mem, nsize) - cast(l_mem, osize);  // 债务变化
  // ...
}
```

### 3.3 incstep 增量步进

```c
// lgc.c 第 1724 行附近
#define LUAI_GCSTEPSIZE (200 * sizeof(Table))

void luaC_fullgc (lua_State *L, int isemergency) {
  // ...
  if (g->gckind == KGC_INC)
    fullinc(L, g);
  // ...
}

static void fullinc (lua_State *L, global_State *g) {
  if (keepinvariant(g))
    entersweep(L);           // 如果有黑色对象，先扫描成白色
  luaC_runtilstate(L, GCSpause, 1);   // 完成到暂停
  luaC_runtilstate(L, GCScallfin, 1); // 执行到终结器
  luaC_runtilstate(L, GCSpause, 1);   // 完成
  setpause(g);
}
```

---

## 4. Write Barrier

### 4.1 为什么要 Write Barrier？

增量 GC 中，**程序和 GC 同时运行**。如果程序修改了对象引用：

```
场景1: 黑色对象 → 白色对象（新引用）
问题: 白色对象不会被遍历到，可能被错误回收

场景2: 年老对象 → 年轻对象（分代GC）
问题: 年轻对象可能还没有被扫描
```

Write Barrier 确保这两种情况被正确处理。

### 4.2 luaC_barrier_（前向屏障，lgc.c 第 5573-5690 行）

```c
// 触发时机: 黑色对象写入对白色对象的引用
void luaC_barrier_ (lua_State *L, GCObject *o, GCObject *v) {
  global_State *g = G(L);
  lua_assert(isblack(o) && iswhite(v) && !isdead(g, v) && !isdead(g, o));
  
  if (keepinvariant(g)) {           // 正在标记阶段？
    reallymarkobject(g, v);         // 把白色对象变灰！
    if (isold(o)) {
      // 分代模式：白色对象标记为 OLD0
      setage(v, G_OLD0);
    }
  }
  else {                            // sweep 阶段
    lua_assert(issweepphase(g));
    if (g->gckind != KGC_GENMINOR)
      makewhite(g, o);              // 把黑色变白，避免再次触发屏障
  }
}

// 使用宏
#define luaC_barrier(L,p,v) (  \
  iscollectable(v) ? luaC_objbarrier(L,p,gcvalue(v)) : cast_void(0))

#define luaC_objbarrier(L,p,o) (  \
  (isblack(p) && iswhite(o)) ? \
  luaC_barrier_(L,obj2gco(p),obj2gco(o)) : cast_void(0))
```

### 4.3 luaC_barrierback_（后向屏障，lgc.c 第 6692-6710 行）

```c
// 触发时机: 黑色对象被修改（可能是老对象）
void luaC_barrierback_ (lua_State *L, GCObject *o) {
  global_State *g = G(L);
  lua_assert(isblack(o) && !isdead(g, o));
  
  if (getage(o) == G_TOUCHED2)     // 已经在 grayagain？
    set2gray(o);
  else
    linkobjgclist(o, g->grayagain);  // 加入 grayagain 链表
  
  if (isold(o))
    setage(o, G_TOUCHED1);
}

// 使用宏
#define luaC_barrierback(L,p,v) (  \
  iscollectable(v) ? luaC_objbarrierback(L, p, gcvalue(v)) : cast_void(0))
```

### 4.4 屏障时机（实际调用位置）

```c
// table 设置值时
luaH_finishset() → luaC_barrierback()

// closure 设置 upvalue
luaF_closeupval() → luaC_barrierback()

// 全局表设置
lua_setupvalue() → luaC_barrierback()

// userdata 元表设置
lua_setmetatable() → luaC_barrier()
```

---

## 5. 分代 GC

### 5.1 对象年龄（lgc.h 第 72-88 行）

```c
// 分代模式的年龄定义
#define G_NEW         0   // 当前周期创建
#define G_SURVIVAL    1   // 上个周期创建（年轻）
#define G_OLD0        2   // 本周期被前向屏障捕获
#define G_OLD1        3   // 经过一个完整周期
#define G_OLD         4   // 真正年老（不再遍历）
#define G_TOUCHED1    5   // 本周期被后向屏障捕获
#define G_TOUCHED2    6   // 上个周期被后向屏障捕获

#define AGEBITS       7    // 所有年龄位

#define getage(o)     ((o)->marked & AGEBITS)
#define setage(o,a)   ((o)->marked = cast_byte(((o)->marked & (~AGEBITS)) | a))
#define isold(o)      (getage(o) > G_SURVIVAL)

// 年龄关系
// G_NEW < G_SURVIVAL < G_OLD0/G_OLD1 < G_OLD
// G_TOUCHED1, G_TOUCHED2 是特殊状态
```

### 5.2 分代列表结构（lstate.h 第 36-59 行）

```c
// allgc 链表的分代分割：
// allgc → survival → old1 → reallyold → NULL

GCObject *allgc;       // 所有对象
GCObject *survival;     // 上个周期幸存的对象（年轻）
GCObject *old1;         // 刚变成老年的对象
GCObject *reallyold;    // 真正老年的对象（超过一个周期）
GCObject *firstold1;   // 第一个 OLD1 对象的指针（优化用）

// 终结器相关
GCObject *finobj;       // 有终结器的对象
GCObject *finobjsur;    // finobj 的 survival
GCObject *finobjold1;   // finobj 的 old1
GCObject *finobjrold;   // finobj 的 reallyold
```

### 5.3 Minor GC（youngcollection，lgc.c 第 1335-1410 行）

```c
static void youngcollection (lua_State *L, global_State *g) {
  l_mem addedold1 = 0;
  // ...
  if (g->firstold1) {           // 有 OLD1 对象？
    markold(g, g->firstold1, g->reallyold);  // 标记它们
    g->firstold1 = NULL;
  }
  markold(g, g->finobj, g->finobjrold);      // 标记 finobj
  markold(g, g->tobefnz, NULL);
  
  atomic(L);                    // 执行原子阶段
  
  // 扫描 allgc：活的变 OLD，不活的释放
  g->gcstate = GCSswpallgc;
  psurvival = sweepgen(L, g, &g->allgc, g->survival, &g->firstold1, &addedold1);
  // sweep survival：活的变成 OLD1
  sweepgen(L, g, psurvival, g->old1, &g->firstold1, &addedold1);
  
  // 更新链表指针
  g->reallyold = g->old1;
  g->old1 = *psurvival;
  g->survival = g->allgc;      // 新对象都是 survival
  
  // 决定是否切换到 major 模式
  if (checkminormajor(g))
    minor2inc(L, g, KGC_GENMAJOR);
  else
    finishgencycle(L, g);
}
```

### 5.4 Major GC（atomic2gen，lgc.c 第 1389-1430 行）

```c
static void atomic2gen (lua_State *L, global_State *g) {
  cleargraylists(g);
  
  g->gcstate = GCSswpallgc;
  sweep2old(L, &g->allgc);     // 所有活对象变 OLD
  
  // 所有列表合并到 allgc
  g->reallyold = g->old1 = g->survival = g->allgc;
  g->firstold1 = NULL;
  
  // 切换到分代模式
  g->gckind = KGC_GENMINOR;
  g->GCmajorminor = g->GCmarked;  // 记录基准内存
  g->GCmarked = 0;
  
  finishgencycle(L, g);
}
```

### 5.5 Minor/Major 切换条件

```c
// lgc.c checkminormajor
#define LUAI_MINORMAJOR 70   // 70%: 超过这个比例的内存变老时，切到 major
#define LUAI_MAJORMINOR 50  // 50%: major GC 后回收超过50%时，切到 minor
#define LUAI_GENMINORMUL 20 // 20%: 分配超过基准的20%时，触发 minor GC
```

---

## 6. 字符串 GC

### 6.1 字符串不参与普通 GC

```c
// reallymarkobject 中的处理
case LUA_VSHRSTR:
case LUA_VLNGSTR: {
  set2black(o);  // 字符串直接变黑，不加入灰色链表
  break;
}
```

**原因**: 字符串是**不可变对象**，一旦创建就不会被修改。因此：
- 字符串不需要 write barrier
- 字符串可以通过 hash 直接访问

### 6.2 字符串表 GC

```c
// lgc.c checkSizes
static void checkSizes (lua_State *L, global_State *g) {
  if (!g->gcemergency) {
    if (g->strt.nuse < g->strt.size / 4)  // 使用率 < 25%
      luaS_resize(L, g->strt.size / 2);   // 收缩字符串表
  }
}
```

### 6.3 字符串缓存清理

```c
// lgc.c luaS_clearcache
void luaS_clearcache (global_State *g) {
  int i, j;
  for (i = 0; i < STRCACHE_N; i++)
    for (j = 0; j < STRCACHE_M; j++) {
      if (iswhite(g->strcache[i][j]))      // 将被回收？
        g->strcache[i][j] = g->memerrmsg;  // 替换为固定字符串
    }
}
```

---

## 7. 弱表处理

### 7.1 弱表类型

```c
// 元表中的 __mode 字段
__mode = "k"    // 弱键
__mode = "v"    // 弱值  
__mode = "kv"   // 键和值都弱
```

### 7.2 弱表处理流程

```c
// atomic 阶段
clearbyvalues(g, g->weak, NULL);     // 清除弱值
clearbyvalues(g, g->allweak, NULL);

// 标记待终结对象后
clearbykeys(g, g->ephemeron);         // 清除 ephemeron 键
clearbykeys(g, g->allweak);

// 复活传播后
clearbyvalues(g, g->weak, origweak);
clearbyvalues(g, g->allweak, origall);
```

### 7.3 Ephemeron 表的特殊处理

Ephemeron 表：键是弱引用，只有当键可达时，值才可达。

```c
// lgc.c convergeephemerons
// 迭代直到收敛
do {
  changed = 0;
  while ((w = next) != NULL) {
    Table *h = gco2t(w);
    if (traverseephemeron(g, h, dir)) {  // 标记了某个值？
      propagateall(g);                    // 重新传播
      changed = 1;
    }
  }
  dir = !dir;
} while (changed);
```

---

## 8. Go 重写要点

### 8.1 核心数据结构

```go
// GC 颜色
type GCColor uint8

const (
    GCWhite0  GCColor = 1 << 3  // 0b001000
    GCWhite1  GCColor = 1 << 4  // 0b010000
    GCBlack   GCColor = 1 << 5  // 0b100000
)

// 对象年龄（分代GC）
type GCAge uint8

const (
    GcNew       GCAge = 0
    GcSurvival  GCAge = 1
    GcOld0      GCAge = 2
    GcOld1      GCAge = 3
    GcOld       GCAge = 4
    GcTouched1  GCAge = 5
    GcTouched2  GCAge = 6
)

// GC 对象头
type GCHeader struct {
    Next   uintptr    // GC 链表（用 uintptr 避免 Go GC）
    Tt     LuaType    // 类型标签
    Marked uint8      // 标记位 = 颜色 | 年龄 | FINALIZEDBIT
}

// 灰色链表
type GrayList struct {
    Gray      *GCHeader  // 主灰色链表
    GrayAgain *GCHeader  // 需要再次遍历
    Weak      *GCHeader  // 弱值表
    Ephemeron *GCHeader  // ephemeron 表
    AllWeak   *GCHeader  // 所有弱表
}

// GC 状态
type GCState uint8

const (
    GCSpropagate  GCState = 0
    GCSenteratomic GCState = 1
    GCSatomic      GCState = 2
    GCSswpallgc    GCState = 3
    GCSswpfinobj   GCState = 4
    GCSswptobefnz  GCState = 5
    GCSswpend      GCState = 6
    GCScallfin     GCState = 7
    GCSpause       GCState = 8
)

// 全局 GC 状态
type GC struct {
    State        GCState
    Kind         GCKind         // KGC_INC, KGC_GENMINOR, KGC_GENMAJOR
    Debt         int64          // GC 债务（负数=需要运行）
    CurrentWhite GCColor        // 当前白色
    
    // 内存统计
    TotalBytes   int64          // 总分配字节
    GCmarked     int64          // 已标记字节
    Majorminor   int64          // major 基准
    
    // 对象链表
    AllGC        *GCHeader      // 所有对象
    Survival     *GCHeader      // 幸存者
    Old1         *GCHeader      // 刚变老
    ReallyOld    *GCHeader      // 真正年老
    FixedGC      *GCHeader      // 固定对象（luaC_fix）
    
    // 灰色链表
    Gray         *GCHeader
    GrayAgain    *GCHeader
    Weak         *GCHeader
    Ephemeron    *GCHeader
    AllWeak      *GCHeader
    
    // 终结器
    FinObj       *GCHeader
    ToBeFnz      *GCHeader      // 待终结
    
    // 扫描位置
    SweepGC      *GCHeader      // 当前扫描位置
    
    // 分代优化
    FirstOld1    *GCHeader      // 第一个 OLD1 对象
}
```

### 8.2 颜色判断方法

```go
func (h *GCHeader) Color() GCColor {
    return GCColor(h.Marked & 0b011000)
}

func (h *GCHeader) IsWhite() bool {
    return h.Color() == G(L).gc.CurrentWhite
}

func (h *GCHeader) IsBlack() bool {
    return h.Marked&0b100000 != 0
}

func (h *GCHeader) IsGray() bool {
    return h.Color() == 0 && !h.IsBlack()
}

func (h *GCHeader) Age() GCAge {
    return GCAge(h.Marked & 0b111)
}

func (h *GCHeader) IsOld() bool {
    return h.Age() > GcSurvival
}
```

### 8.3 标记实现

```go
func (g *GC) MarkObject(o *GCHeader) {
    if !o.IsWhite() {
        return  // 不是白色，无需处理
    }
    
    g.GCmarked += int64(objsize(o))
    
    switch o.Tt {
    case LUA_VSHRSTR, LUA_VLNGSTR:
        o.setBlack()
        
    case LUA_VUPVAL:
        uv := (*UpVal)(unsafe.Pointer(o))
        if uv.IsOpen() {
            o.setGray()    // 开放upvalue保持灰色
        } else {
            o.setBlack()
        }
        g.MarkValue(uv.Value)
        
    case LUA_VTABLE, LUA_VUSERDATA, LUA_VLCL, 
         LUA_VCCL, LUA_VTHREAD, LUA_VPROTO:
        // 加入灰色链表
        o.setGray()
        o.GClist = g.Gray
        g.Gray = o
    }
}
```

### 8.4 Write Barrier 实现

```go
// 前向屏障：黑色对象 → 白色对象
func (g *GC) Barrier(parent, value *GCHeader) {
    if parent.IsBlack() && value.IsWhite() && !value.IsDead() {
        if g.keepInvariant() {
            g.MarkObject(value)           // 把白色变灰
            if parent.IsOld() {
                value.SetAge(GcOld0)      // 分代：标记为 OLD0
            }
        } else if g.Kind != KGcGenMinor {
            value.MakeWhite()              // sweep阶段：变白
        }
    }
}

// 后向屏障：黑色对象被修改
func (g *GC) BarrierBack(o *GCHeader) {
    if o.IsBlack() && !o.IsDead() {
        if o.Age() == GcTouched2 {
            o.setGray()
        } else {
            // 加入 grayagain
            o.GClist = g.GrayAgain
            g.GrayAgain = o
        }
        if o.IsOld() {
            o.SetAge(GcTouched1)
        }
    }
}
```

### 8.5 增量步进

```go
func (L *LuaState) GCStep() {
    g := L.G
    if !gcrunning(g) {
        if g.GCstp&GCSTPUSR != 0 {
            g.setDebt(20000)
        }
        return
    }
    
    switch g.Kind {
    case KGcInc, KGcGenMajor:
        g.incStep()
    case KGcGenMinor:
        g.youngCollection()
        g.setMinorDebt()
    }
}

func (g *GC) incStep() {
    for !g.stepInvariant() {
        switch g.State {
        case GCSpropagate:
            if g.Gray == nil {
                g.State = GCSenteratomic
                continue
            }
            g.propagateMark()
            
        case GCSenteratomic:
            g.atomic()
            g.enterSweep()
            
        case GCSswpallgc:
            g.sweepStep(&g.AllGC, GCSswpfinobj)
            
        // ... 其他状态
        }
    }
}
```

### 8.6 关键注意事项

1. **使用 uintptr 而非指针**: Go GC 会扫描指针，用 `uintptr` 可以避免 Lua 对象被 Go GC 追踪

2. **避免逃逸**: Lua 对象必须保持在 Go 栈上或显式管理的内存中

3. **线程安全**: 如果 Go 实现支持多线程 Lua 状态，需要加锁

4. **内存分配器集成**: 
   ```go
   func (L *LuaState) Alloc(size int) unsafe.Pointer {
       g := L.G
       ptr := g.frealloc(g.ud, nil, 0, size)
       g.TotalBytes += int64(size)
       g.Debt += int64(size)
       if g.Debt > 0 {
           L.GCStep()
       }
       return ptr
   }
   ```

5. **与 Go GC 的关系**: 
   - 选项A：完全自己管理内存（最快，但最复杂）
   - 选项B：用 Go 分配器，但用 `runtime.SetFinalizer` 配合 Lua GC（推荐）

---

## 9. API 清单

| 函数 | 签名 | 说明 |
|------|------|------|
| luaC_step | `void luaC_step(lua_State *L)` | 增量 GC 一步 |
| luaC_fullgc | `void luaC_fullgc(lua_State *L, int isemergency)` | 完整 GC |
| luaC_newobj | `GCObject *luaC_newobj(lua_State *L, lu_byte tt, size_t sz)` | 创建 GC 对象 |
| luaC_barrier_ | `void luaC_barrier_(lua_State *L, GCObject *o, GCObject *v)` | 前向屏障 |
| luaC_barrierback_ | `void luaC_barrierback_(lua_State *L, GCObject *o)` | 后向屏障 |
| luaC_fix | `void luaC_fix(lua_State *L, GCObject *o)` | 固定对象（永不回收） |
| luaC_freeallobjects | `void luaC_freeallobjects(lua_State *L)` | 释放所有对象 |
| luaC_changemode | `void luaC_changemode(lua_State *L, int newmode)` | 切换 GC 模式 |
| luaC_checkfinalizer | `void luaC_checkfinalizer(lua_State *L, GCObject *o, Table *mt)` | 检查终结器 |

---

## 10. 依赖关系

```
lgc.c 依赖:
├── lobject.h   (GCHeader, GCObject, Novariant)
├── lstate.h   (global_State, GC 链表定义)
├── lmem.h     (luaM_newobject)
├── lstring.h  (luaS_clearcache)
├── ltable.h   (遍历 table)
├── lfunc.h    (遍历 closure, upvalue)
├── ltm.h      (luaT_gettmbyobj)
└── ldo.h     (luaD_checkminstack)
```

## 11. 常见陷阱

| 陷阱 | 后果 | 解决方案 |
|------|------|----------|
| 忘记 write barrier | 内存泄漏（白色对象被回收） | 每次写入可回收引用时调用 barrier |
| sweep 时不 makewhite | 同一周期内对象被回收两次 | sweep 时把对象变当前白色 |
| 颜色翻转时机错误 | 对象被过早回收 | 在 atomic 阶段结束时翻转 |
| ephemeron 处理不当 | 弱表键可达但值不可达 | 迭代直到收敛 |
| 分代 age 计算错误 | 对象被过早/过晚晋升 | 严格按照年龄位操作 |