# Lua Interpreter Source Code Analysis

## Project Overview

**Lua** is the official reference implementation of the Lua programming language — a lightweight, embeddable scripting language designed for extending applications. This repository contains the complete C source code (version 5.5.1-dev), including:

- A standalone interpreter (`lua`)
- A VM-based bytecode executor
- A recursive-descent parser
- Incremental and generational garbage collectors
- Standard libraries (base, coroutines, I/O, OS, math, string, table, UTF-8, package/loader, debug)

The architecture follows the classic **three-phase compiler**: Lexer (llex) → Parser (lparser) → Code Generator (lcode), followed by a **stack-based VM** (lvm) for execution.

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                        lua.c (Standalone Interpreter)               │
│  main() → pmain() → doREPL() / handle_script() / runargs()          │
└─────────────────────────────┬───────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     lapi.c (Lua API Layer)                          │
│  Public C API: lua_newstate, lua_call, lua_pcall, lua_getfield, etc.│
└─────────────────────────────┬───────────────────────────────────────┘
                              │
          ┌───────────────────┼───────────────────┐
          │                   │                   │
          ▼                   ▼                   ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│  lstate.c/h    │  │     ldo.c/h     │  │    lgc.c/h      │
│  Thread State  │  │  Core Execution │  │ Garbage Collector│
│  Global State  │  │  luaD_call,     │  │ Incremental &    │
│  CallInfo      │  │  luaD_throw,    │  │ Generational GC  │
└─────────────────┘  │  luaD_rawrunctx│  └─────────────────┘
                     └─────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    lobject.c/h (Lua Objects)                        │
│  TValue (tagged union)  │  Table  │  Closure  │  Proto  │  TString  │
└────────────────────────┴─────────┴───────────┴────────┴────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        lvm.c (Virtual Machine)                       │
│  luaV_execute() — main VM loop dispatching opcodes via jump table   │
│  luaV_gettable, luaV_settable, luaV_concat, luaV_arith, etc.        │
└─────────────────────────────┬───────────────────────────────────────┘
                              │
          ┌───────────────────┼───────────────────┐
          │                   │                   │
          ▼                   ▼                   ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│   lcode.c/h     │  │    llex.c/h     │  │  lparser.c/h    │
│  Code Generator │  │  Lexical Analyz.│  │   Recursive-    │
│  Expression     │  │  Token stream   │  │   Descent       │
│  Optimizations  │  │  (lzio input)   │  │   Parser        │
└─────────────────┘  └─────────────────┘  └─────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                     lundump.c/h (Bytecode Loader)                   │
│  luaU_undump() — loads precompiled Lua chunks                        │
└─────────────────────────────┬───────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     ldump.c (Bytecode Dumper)                        │
│  luaU_dump() — serializes functions to bytecode                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Module Catalog

### Core VM & Execution

| Module | Purpose | Size |
|--------|---------|------|
| **lvm.c/h** | Virtual machine: opcode dispatch, arithmetic, table access, metamethods | 61KB |
| **ldo.c/h** | Core execution: call/return, stack management, error handling (longjmp) | 40KB |
| **lstate.c/h** | Thread state (lua_State), global state, CallInfo management | 16KB |

### Objects & Values

| Module | Purpose |
|--------|---------|
| **lobject.c/h** | Tagged values (TValue), GC objects, tables, functions, strings, prototypes |
| **ltable.c/h** | Hash-table implementation with array part + node part |
| **lstring.c/h** | String interning and management |
| **lfunc.c/h** | Function upvalues and closure handling |
| **lopcodes.c/h** | Opcode enumeration and instruction format macros |

### Compiler

| Module | Purpose |
|--------|---------|
| **llex.c/h** | Lexical analyzer producing token stream |
| **lparser.c/h** | Recursive-descent parser building AST into Proto |
| **lcode.c/h** | Expression code generation, constant folding |

### Garbage Collection

| Module | Purpose |
|--------|---------|
| **lgc.c/h** | Incremental and generational GC with tri-color marking |
| **lmem.c/h** | Memory allocation wrapper |

### I/O & Persistence

| Module | Purpose |
|--------|---------|
| **lundump.c/h** | Bytecode loader (binary chunk format) |
| **ldump.c** | Bytecode serializer |
| **lzio.c/h** | Generic buffered input stream abstraction |

### Standard Libraries (lua-master/*.c)

| Library | Purpose |
|---------|---------|
| **lbaselib.c** | _G, print, pairs, ipairs, type, pcall, error, etc. |
| **lcorolib.c** | coroutine.create, resume, yield, status, wrap |
| **ldblib.c** | debug library, hooks, info, getfenv |
| **liolib.c** | io.open, read, write, flush, close, lines |
| **loslib.c** | os.clock, date, time, difftime, execute, exit |
| **lmathlib.c** | math.* — sin, cos, sqrt, random, floor, etc. |
| **lstrlib.c** | string.* — sub, find, match, gmatch, format, etc. |
| **ltablib.c** | table.* — insert, remove, concat, sort, move, pack |
| **loadlib.c** | require, package.* — loaders, cpath, path |
| **lutf8lib.c** | utf8.* — len, char, codepoint, codes |
| **lauxlib.c** | Auxiliary library: luaL_newstate, luaL_loadfile, luaL_check* |
| **linit.c** | Standard library registration (luaL_openlibs) |

### API & Debug

| Module | Purpose |
|--------|---------|
| **lapi.c** | Public Lua C API implementation |
| **ldebug.c/h** | Debug API, hook management, stack introspection |
| **ltm.c/h** | Tag method (metamethod) dispatch: __add, __index, __gc, etc. |

### Interpreter

| Module | Purpose |
|--------|---------|
| **lua.c** | Standalone interpreter: main(), REPL, argument parsing |

### Configuration & Headers

| Module | Purpose |
|--------|---------|
| **lua.h** | Public API declarations, type definitions |
| **luaconf.h** | Build-time configuration (64-bit, types, options) |
| **lprefix.h** | Compiler-specific prefix (POSIX, Windows) |
| **llimits.h** | Size limits (MAXStack, LUAI_MAX*, etc.) |
| **ljumptab.h** | VM opcode jump table (generated) |

---

## Core Data Structures

### 1. TValue — Tagged Value
```c
typedef union Value {
  struct GCObject *gc;    // collectable: string, table, function, thread
  void *p;                // light userdata
  lua_CFunction f;        // light C function
  lua_Integer i;          // integer
  lua_Number n;           // float
} Value;

typedef struct TValue {
  Value value_;
  lu_byte tt_;            // type tag (bits 0-3: type, bits 4-5: variant, bit 6: collectable)
} TValue;
```
A tagged union representing all Lua values: nil, booleans, numbers (int/float), strings, tables, functions, userdata, threads.

### 2. Table — Hash + Array
```c
typedef struct Table {
  CommonHeader;           // GC metadata
  lu_byte flags;          // tagmethod cache
  lu_byte lsizenode;      // log2 of hash size
  unsigned int asize;      // array part size
  Value *array;            // dense array part
  Node *node;              // hash part (collision chains)
  struct Table *metatable;
  GCObject *gclist;
} Table;
```
Hybrid array+hash table. Integer keys 1..asize use array part; others use hash part.

### 3. Proto — Function Prototype
```c
typedef struct Proto {
  CommonHeader;
  lu_byte numparams, flag, maxstacksize;
  int sizek, sizecode, sizelineinfo, sizep, sizelocvars;
  int linedefined, lastlinedefined;
  TValue *k;                    // constants pool
  Instruction *code;             // bytecode
  struct Proto **p;             // nested functions
  Upvaldesc *upvalues;
  ls_byte *lineinfo;
  LocVar *locvars;
  TString *source;
  GCObject *gclist;
} Proto;
```
Compilable representation of a function; holds bytecode, constants, debug info.

### 4. lua_State — Thread/Execution State
```c
struct lua_State {
  CommonHeader;
  lu_byte allowhook;
  TStatus status;
  StkIdRel top;                  // stack top
  struct global_State *l_G;      // shared global state
  CallInfo *ci;                  // call stack
  StkIdRel stack_last;
  StkIdRel stack;
  UpVal *openupval;
  StkIdRel tbclist;              // to-be-closed variables
  GCObject *gclist;
  struct lua_State *twups;       // threads with open upvalues
  struct lua_longjmp *errorJmp;
  CallInfo base_ci;
  volatile lua_Hook hook;
  ptrdiff_t errfunc;
  l_uint32 nCcalls;              // nested C/non-yieldable calls
  // ...
};
```
Represents one Lua thread/coroutine. Multiple threads share a single `global_State`.

### 5. global_State — Shared State
```c
typedef struct global_State {
  lua_Alloc frealloc;
  void *ud;
  l_mem GCtotalbytes, GCdebt;
  stringtable strt;               // string interning
  TValue l_registry;
  unsigned int seed;              // randomized hash seed
  lu_byte gcparams[6];
  lu_byte currentwhite;
  lu_byte gcstate, gckind;
  GCObject *allgc, *survival, *old1, *reallyold;  // GC lists
  GCObject *gray, *grayagain, *weak, *ephemeron, *allweak;
  lua_CFunction panic;
  TString *tmname[TM_N];          // metamethod names
  struct Table *mt[9];            // metatables for basic types
  // ...
} global_State;
```
Shared across all threads in a single Lua state.

---

## Entry Points

### CLI Entry: `lua.c`
```
main()
  └─> luaL_newstate()           // create Lua state
      └─> lua_pcall(pmain)       // protected call
          └─> pmain()            // argument parsing
              ├─> luaL_openlibs()        // load standard libraries
              ├─> createargtable()       // build `arg` table
              ├─> handle_luainit()       // LUA_INIT
              ├─> runargs()              // -e, -l options
              ├─> handle_script()        // run main script
              └─> doREPL()               // interactive mode
```

### Public C API Entry: `lapi.c`
```
lua_call() → lua_callk() → luaD_call()
lua_pcallk() → [protected execution via luaD_pcall()]
lua_load() → luaU_undump() or llex/lparser for source
lua_newstate() → lua_newstatex() → lstate.c init
```

### VM Execution Entry: `lvm.c`
```
luaD_call()
  └─> luaV_execute()             // main VM loop
      └─> jumps via luaV_jumptbl[]  // opcode dispatch
          ├─> OP_LOAD*            // constants
          ├─> OP_GET* / OP_SET*   // table access
          ├─> OP_ADD / OP_SUB / ... // arithmetic
          ├─> OP_CALL / OP_TAILCALL
          ├─> OP_RETURN
          └─> ... (all 68 opcodes)
```

### Compilation Pipeline: `lua.c` → `lua_load()`
```
lua_load()
  └─> luaU_undump()              // try binary first
      └─> luaU_undumplay()
  OR
  └─> luaY_parser()              // source code
      └─> llex()                 // lexer: source → tokens
          └─> lparser()          // parser: tokens → Proto
              └─> lcode()        // codegen: expressions → bytecode
```

---

## Build System

**Makefile** (`lua-master/makefile`):
- Compiler: `gcc` with `-Wall -O2 -std=c99 -DLUA_USE_LINUX`
- Target: `liblua.a` (core) + `lua` (interpreter)
- Object groups:
  - `CORE_O`: VM, compiler, GC, core runtime (lapi, lvm, lgc, lcode, llex, lparser, etc.)
  - `AUX_O`: lauxlib
  - `LIB_O`: all standard libraries

**Build artifacts**: `.o` files, `liblua.a`, `lua` binary

---

## Complexity Assessment

### Complex Modules (warrant L2 analysis):

| Module | Lines | Reasons |
|--------|-------|---------|
| **lgc.c** | 1800+ | Dual-mode GC (incremental + generational), tri-color marking, write barriers, multiple object lists |
| **lvm.c** | 1900+ | 68 opcodes, complex dispatch, metamethod handling, coroutine execution |
| **lparser.c** | 2100+ | Recursive-descent parser with expression precedence, grammar is non-trivial |
| **lcode.c** | 1800+ | Expression codegen, constant folding, register allocation, temporary management |
| **ltable.c** | 1400+ | Hash-table with array part, rehash, resizing, key iteration |
| **lapi.c** | 1100+ | Full public C API, many functions, stack manipulation conventions |

### Moderate Complexity:
- **lstate.c** — state allocation, CallInfo management
- **ldo.c** — call/return mechanics, error handling via setjmp/longjmp
- **lstring.c** — string interning, hash table
- **loadlib.c** — dynamic library loading (platform-specific)
- **lstrlib.c** — complex string pattern matching (Lua patterns)

### Lower Complexity (utilities/wrappers):
- **lauxlib.c**, **linit.c**, **lfunc.c**, **lmem.c**, **lzio.c**, **ldump.c**, **lundump.c**, **lctype.c**, **ljumptab.h**

---

## Notes

- **Bytecode format**: 32-bit instructions with variable-width operands (iABC, iABx, iAsBx, iAx, isJ formats). Version marker: `\x1bLua` (ESC + "Lua").
- **Metamethods**: Dispatched via `ltm.c` based on type tag; supports `__add`, `__sub`, `__mul`, `__div`, `__mod`, `__pow`, `__unm`, `__idiv`, `__band`, `__bor`, `__bxor`, `__shl`, `__shr`, `__bnot`, `__eq`, `__lt`, `__le`, `__concat`, `__len`, `__index`, `__newindex`, `__call`, `__gc`, `__close`, `__tostring`, `__pairs`, `__ipairs`, `__mode`, `__len`, `__div`, etc.
- **Coroutine support**: Full coroutines via lua_State threads, yield/resume across C calls.
- **To-Be-Closed variables** (TBC): RAII-like semantics via `to-be-closed` keyword; handled at return/close.