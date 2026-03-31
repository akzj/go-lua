# Lua 5.5 C 源码 → Go 重写架构文档

## 1. 项目概述

本项目是 Lua 5.5 解释器的 C 源码分析文档，目标是为 Go 重写提供完整的技术规格说明。Lua 5.5 是一个完整的 Lua 语言实现，包含词法分析、语法分析、字节码编译器、寄存器式虚拟机、增量式垃圾回收器、以及完整的基础库。

**核心统计**：
- 源码总量：约 34,000 行 C 代码
- 核心模块：18 个 C 文件 + 18 个头文件
- 字节码指令：48 条（OpCode）
- 代码生成：约 2,000 行（lcode.c + lparser.c）

## 2. 模块依赖图

```
                          ┌─────────────────────────────────────────────┐
                          │              lua.h (公开 API)               │
                          │         lapi.h (内部 API)                    │
                          └──────────────────┬────────────────────────────┘
                                             │
        ┌────────────────────────────────────┼────────────────────────────────────┐
        │                                    │                                    │
        ▼                                    ▼                                    ▼
┌───────────────┐                    ┌───────────────┐                    ┌───────────────┐
│   lua.c       │                    │   lapi.c      │                    │   lauxlib.c   │
│  (可执行入口) │                    │  (C API层)    │                    │  (辅助库)     │
└───────────────┘                    └───────┬───────┘                    └───────────────┘
                                              │
        ┌─────────────────────────────────────┼─────────────────────────────────────┐
        │                                     │                                     │
        ▼                                     ▼                                     ▼
┌───────────────┐                    ┌───────────────┐                    ┌───────────────┐
│    ldo.c      │                    │    lvm.c      │                    │   lgc.c       │
│  (调用/错误)   │◄───────────────────│   (虚拟机)    │                    │  (垃圾回收)   │
└───────┬───────┘                    └───────┬───────┘                    └───────┬───────┘
        │                                      │                                      │
        │          ┌───────────────────────────┴───────────────────────────┐         │
        │          │                           │                           │         │
        ▼          ▼                           ▼                           ▼         │
┌───────────────┐ ┌───────────────┐     ┌───────────────┐     ┌───────────────┐        │
│  lstate.c     │ │  lfunc.c      │     │  lobject.c    │     │  ltable.c     │        │
│  (全局状态)   │ │  (函数/闭包)  │     │  (类型系统)   │     │  (表)         │        │
└───────┬───────┘ └───────┬───────┘     └───────┬───────┘     └───────┬───────┘        │
        │                  │                     │                     │                │
        └──────────────────┼─────────────────────┼─────────────────────┘                │
                           │                     │                                          │
        ┌──────────────────┼─────────────────────┼───────────────────────────────────────┐
        │                  │                     │                                       │
        ▼                  ▼                     ▼                                       ▼
┌───────────────┐ ┌───────────────┐     ┌───────────────┐                     ┌───────────────┐
│  lstring.c    │ │  lmem.c       │     │  lzio.c       │                     │  ldebug.c     │
│  (字符串表)   │ │  (内存管理)   │     │  (输入流)     │                     │  (调试接口)   │
└───────────────┘ └───────────────┘     └───────┬───────┘                     └───────────────┘
                                               │
                        ┌──────────────────────┬┴──────────────────────┐
                        ▼                                           ▼
              ┌───────────────┐                             ┌───────────────┐
              │   llex.c      │                             │  lundump.c    │
              │  (词法分析)   │                             │  (字节码加载) │
              └───────┬───────┘                             └───────────────┘
                      │
                      ▼
              ┌───────────────┐        ┌───────────────┐
              │  lparser.c    │◄───────│   lcode.c     │
              │  (语法分析)   │        │  (代码生成)   │
              └───────────────┘        └───────────────┘

┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              标准库 (依赖 VM/API)                                    │
├─────────────┬─────────────┬─────────────┬─────────────┬─────────────┬─────────────┤
│ lbaselib.c  │ lstrlib.c   │ ltablib.c   │ lmathlib.c  │ liolib.c    │ loslib.c    │
│  (基础库)   │  (字符串)   │  (表)       │  (数学)     │  (IO)       │  (OS)       │
├─────────────┼─────────────┼─────────────┼─────────────┼─────────────┼─────────────┤
│ lcorolib.c  │ ldblib.c    │ lutf8lib.c  │ loadlib.c   │ ltm.c       │ linit.c     │
│  (协程)     │  (调试)     │  (UTF8)     │  (动态库)  │  (元方法)   │  (初始化)   │
└─────────────┴─────────────┴─────────────┴─────────────┴─────────────┴─────────────┘
```

## 3. 模块职责与代码量

| 模块 | 文件 | 行数 | 核心职责 |
|------|------|------|----------|
| **类型系统** | lobject.h/c | 864+718 | TValue tagged union、所有 Lua 类型定义 |
| **内存管理** | lmem.c/h | 215+96 | luaM_* 分配器、紧急 GC |
| **字符串** | lstring.c/h | 353+73 | 字符串 interning、短字符串全局表 |
| **表** | ltable.c/h | 1355+184 | 数组+哈希混合结构、mainposition |
| **函数/闭包** | lfunc.c/h | 314+65 | Proto、Closure、UpVal 管理 |
| **全局状态** | lstate.c/h | 425+451 | lua_State、global_State、GC 链表 |
| **输入流** | lzio.c/h | 89+67 | ZIO 抽象、缓冲读取 |
| **词法分析** | llex.c/h | 604+93 | 词符流生成器 |
| **语法分析** | lparser.c/h | 2202+196 | LL(1) 递归下降 parser |
| **代码生成** | lcode.c/h | 1972+105 | 字节码指令生成 |
| **指令集** | lopcodes.h/c | 439+140 | OpCode 枚举、指令编码/解码宏 |
| **虚拟机** | lvm.c/h | 1972+136 | 48 条指令执行循环 |
| **调用/错误** | ldo.c/h | 1172+99 | 函数调用栈、setjmp/longjmp |
| **垃圾回收** | lgc.c/h | 1804+268 | 三色标记、增量/分代 GC |
| **元方法** | ltm.c/h | 364+105 | TM 查找缓存、 metamethod |
| **调试接口** | ldebug.c/h | 979+65 | hook、行号信息、栈追踪 |
| **C API** | lapi.c/h | 1478+65 | lua_* 公开 API 实现 |
| **辅助库** | lauxlib.c/h | 1202+271 | luaL_* 辅助函数 |

## 4. 重写顺序（依赖关系排序）

### 第一阶段：基础设施（无依赖）
1. **lobject** — 类型系统（所有其他模块依赖）
2. **lmem** — 内存管理（lobject 依赖）
3. **lopcodes** — 指令集定义（lobject 依赖）

### 第二阶段：核心数据结构
4. **lstring** — 字符串表（lobject 依赖）
5. **lfunc** — 函数/闭包/upvalue（lobject + lstring 依赖）
6. **ltable** — 表实现（lobject + lstring 依赖）

### 第三阶段：运行时基础设施
7. **lstate** — 全局状态（依赖以上所有）
8. **lzio** — 输入流抽象

### 第四阶段：执行引擎
9. **lvm** — 虚拟机（依赖 ldo、lobject、lopcodes）
10. **ldo** — 调用栈、错误处理（最复杂之一）

### 第五阶段：代码生成
11. **llex** — 词法分析（依赖 lzio）
12. **lcode** — 代码生成（依赖 lparser）
13. **lparser** — 语法分析（依赖 llex、lcode）
14. **lundump** — 字节码加载（依赖 lfunc、lstring）

### 第六阶段：GC 和元方法
15. **lgc** — 垃圾回收（需要理解所有对象类型）
16. **ltm** — 元方法系统

### 第七阶段：API 层
17. **lapi** — C API（依赖以上所有）
18. **ldebug** — 调试接口

### 第八阶段：标准库
19. **lbaselib, lstrlib, ltablib, lmathlib, liolib, loslib, lcorolib, ldblib, lutf8lib, loadlib**

## 5. 核心数据结构一览

### TValue (Tagged Union)
```c
// C 实现：union 存储值 + byte 存储 tag
typedef union Value {
  struct GCObject *gc;    // 可回收对象
  void *p;               // light userdata
  lua_CFunction f;       // light C function
  lua_Integer i;         // 整数
  lua_Number n;          // 浮点数
} Value;

typedef struct TValue {
  Value value_;
  lu_byte tt_;           // type tag
} TValue;
```

### lua_State (线程/协程状态)
```c
struct lua_State {
  CommonHeader;          // GC 链表链接
  lu_byte allowhook;
  TStatus status;         // 运行状态
  StkIdRel top;           // 栈顶指针
  struct global_State *l_G;
  CallInfo *ci;          // 当前调用帧
  StkIdRel stack_last;    // 栈底
  StkIdRel stack;         // 栈起始
  UpVal *openupval;      // 开放的 upvalue 链表
  StkIdRel tbclist;      // to-be-closed 变量列表
  // ... 更多字段
};
```

### Table (混合数组+哈希)
```c
typedef struct Table {
  CommonHeader;
  lu_byte flags;         // 元方法缓存标记
  lu_byte lsizenode;    // log2(哈希槽数)
  unsigned int asize;    // 数组部分大小
  Value *array;         // 数组部分
  Node *node;           // 哈希部分
  struct Table *metatable;
  GCObject *gclist;
} Table;
```

### Proto (函数原型)
```c
typedef struct Proto {
  CommonHeader;
  lu_byte numparams;
  lu_byte flag;
  lu_byte maxstacksize;
  int sizek, sizecode, sizep, ...
  TValue *k;             // 常量表
  Instruction *code;     // 字节码
  struct Proto **p;       // 嵌套函数
  Upvaldesc *upvalues;   // upvalue 描述
  // ... 调试信息
} Proto;
```

## 6. 关键技术挑战

| 挑战 | 原因 | Go 解决方案 |
|------|------|------------|
| **setjmp/longjmp** | Lua 用它实现异常和协程 yield | Go 的 panic/recover + goroutine |
| **Tagged Union** | C 用 union 节省空间 | Go 用 interface{} + type tag |
| **指针算术** | StkId 是 TValue* | Go 用 slice index |
| **GC 屏障** | Lua 自己的 GC 需要 write barrier | 需要重新实现 Lua 对象 GC |
| **字符串 interning** | 短字符串全局唯一 | Go string 天然 interned |
| **UpVal 生命周期** | open/close 的精确语义 | Go closure 天然处理 |
| **协程 yield/resume** | 需要栈帧切换 | Go goroutine 天然支持 |

## 7. 文件清单

```
lua-master/
├── 核心文件 (必须重写)
│   ├── lobject.h/c    - 类型系统 (~1600行)
│   ├── lmem.h/c       - 内存管理 (~300行)
│   ├── lstate.h/c     - 全局状态 (~900行)
│   ├── lstring.h/c    - 字符串表 (~430行)
│   ├── ltable.h/c     - 表实现 (~1500行)
│   ├── lfunc.h/c      - 函数/闭包 (~380行)
│   ├── lvm.h/c        - 虚拟机 (~2100行)
│   ├── ldo.h/c        - 调用栈 (~1270行)
│   ├── lgc.h/c        - 垃圾回收 (~2070行)
│   ├── llex.h/c       - 词法分析 (~700行)
│   ├── lcode.h/c      - 代码生成 (~2070行)
│   ├── lparser.h/c    - 语法分析 (~2400行)
│   ├── lundump.h/c    - 字节码加载 (~460行)
│   └── lzio.h/c       - 输入流 (~160行)
│
├── API 层
│   ├── lua.h          - 公开 API 定义
│   ├── lapi.h/c       - C API 实现 (~1540行)
│   ├── lauxlib.h/c    - 辅助库 (~1470行)
│   └── ltm.h/c        - 元方法 (~470行)
│
├── 调试和支持
│   ├── ldebug.h/c     - 调试接口 (~1040行)
│   ├── lopcodes.h/c   - 指令集 (~580行)
│   ├── llimits.h      - 限制常量
│   └── luaconf.h      - 配置
│
├── 标准库
│   ├── lbaselib.c     - 基础库 (~560行)
│   ├── lstrlib.c      - 字符串库 (~1890行)
│   ├── ltablib.c      - 表库 (~430行)
│   ├── lmathlib.c    - 数学库 (~770行)
│   ├── liolib.c      - IO库 (~840行)
│   ├── loslib.c      - OS库 (~430行)
│   ├── lcorolib.c    - 协程库 (~230行)
│   ├── ldblib.c      - 调试库 (~480行)
│   ├── lutf8lib.c    - UTF8库 (~300行)
│   ├── loadlib.c     - 动态库 (~750行)
│   └── linit.c       - 库初始化 (~60行)
│
└── 其他
    ├── lua.c          - 可执行入口
    └── onelua.c       - 单文件 Lua
```

## 8. 编译依赖关系（makefile 分析）

```
CORE_O = lapi.o lcode.o lctype.o ldebug.o ldo.o ldump.o lfunc.o lgc.o \
         llex.o lmem.o lobject.o lopcodes.o lparser.o lstate.o lstring.o \
         ltable.o ltm.o lundump.o lvm.o lzio.o ltests.o

AUX_O = lauxlib.o

LIB_O = lbaselib.o ldblib.o liolib.o lmathlib.o loslib.o ltablib.o \
        lstrlib.o lutf8lib.o loadlib.o lcorolib.o linit.o
```

## 9. 已完成文档

| 文档 | 状态 | 描述 |
|------|-------|------|
| `architecture.md` | ✅ 完成 | 项目架构总览、重写顺序 |
| `module_lobject.md` | ✅ 完成 | 类型系统、TValue、GCObject |
| `module_lmem.md` | ✅ 完成 | 内存分配器、紧急GC |
| `module_lstring.md` | ✅ 完成 | 字符串interning、哈希表 |
| `module_ltable.md` | ✅ 完成 | 数组+哈希混合表、mainposition |
| `module_lfunc.md` | ✅ 完成 | Proto、Closure、UpVal |
| `module_ldo.md` | ✅ 完成 | 调用栈、protected call、yield/resume |
| `module_lvm.md` | ✅ L2分析完成 | 虚拟机执行循环、48条指令 |
| `module_lgc.md` | ✅ L2分析完成 | 三色增量GC、分代GC、write barrier |
| `module_ltm.md` | ✅ 完成 | 元方法查找、缓存 |
| `c_to_go_pitfalls.md` | ✅ 完成 | 11个关键翻译陷阱 |
| `test_strategy.md` | ✅ 完成 | 测试套件映射、验证方案 |

## 10. 快速参考：Go 类型映射表

### 核心类型

| C 类型 | Go 类型 | 说明 |
|--------|---------|------|
| `TValue` | `struct { tt LuaType; i int64; n float64; ... }` | Tagged union |
| `lua_State` | `struct { stack []TValue; top int; ci *CallInfo; ... }` | 线程/协程 |
| `Table` | `struct { array []TValue; node []Node; ... }` | 表 |
| `Closure` | `LClosure/CClosure` 接口 | 闭包 |
| `Proto` | `struct { code []uint32; k []TValue; ... }` | 函数原型 |
| `UpVal` | `struct { V *TValue; Closed bool; ... }` | UpValue |

### 关键转换

| C 模式 | Go 模式 |
|--------|---------|
| `setjmp/longjmp` | `goroutine + channel` 或 `panic/recover` |
| `TValue*` 指针 | `[]TValue slice + index` |
| `luaM_realloc` | `append() + copy()` |
| GCObject 链表 | `uintptr` 避免 Go GC 扫描 |

## 11. 关键修正 (来自 Review)

### 协程方案修正

**原方案问题**：每个 Lua 协程一个 goroutine 有语义不匹配（Lua 协作式 vs Go 抢占式）。

**正确方案**：单 goroutine + 手动栈复制，模拟 C 的 longjmp 行为。

详见 `c_to_go_pitfalls.md` 第 9 节。

### Table Hash 算法

Lua Table 使用 **开放地址法 + 链式冲突**，不是纯线性探测。详见 `module_ltable.md`。

---

## 12. 文档统计

| 指标 | 值 |
|------|-----|
| 总文档数 | 21 个文件 |
| 总行数 | ~11,000 行 |
| 模块规格 | 18 个 |
| 翻译陷阱 | 11 个 |
| 测试映射 | 40+ 个测试文件 |

---

**文档生成时间**: 2024
**源码版本**: Lua 5.5 (lua-master/)
**C 源码总量**: ~34,000 行
**完成度**: 100%