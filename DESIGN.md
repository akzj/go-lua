# go-lua DESIGN.md — Lua 5.5.1 VM in Go

> **Version**: v4 (clean slate rewrite)
> **Methodology**: ZeroFAS interface-first, bottom-up, design-before-code
> **Knowledge Base**: .analysis/ (15,485 lines of Lua 5.5.1 reference analysis)

---

## §1 Overview & Goals

### What
A complete Lua 5.5.1 virtual machine implemented in Go, capable of running the official `lua-master/testes/` test suite (31 .lua files).

### Design Principles
1. **Faithfully follow the C reference** — same semantics, same edge cases, same opcodes
2. **Go-idiomatic where possible** — use Go's GC, slices, interfaces, error handling
3. **Interface-first** — every module has `api/api.go` defining its contract
4. **Small modules** — each module < 2000 lines, each file < 500 lines
5. **Bottom-up implementation** — strict dependency order, test each layer independently

### Philosophy
> This project may fail again. The goal is to find a strategy that maximizes
> the probability of success. Reducing difficulty IS a valid strategy.
> Performance is irrelevant if the project fails. Correctness first, simplicity second,
> performance never (until all 31 tests pass).

### Non-Goals (for v4)
- Performance optimization (correctness first, optimize after success)
- Full C API compatibility (we expose a Go API)
- Embedding API for external Go programs (internal focus for now)
- Web/WASM support (removed from scope)
- Custom GC implementation (use Go's GC, add __gc support later if needed)

---

## §2 Architecture

### Module Dependency Graph

```
Layer 0 — Foundation (no dependencies)
┌─────────────────────────────────────────────────────┐
│  object     — TValue, all Lua types, type tags      │
│  opcode     — instruction format, 77 opcodes        │
└─────────────────────────────────────────────────────┘
          │
          ▼
Layer 1 — Core Data Structures (depends on: object)
┌─────────────────────────────────────────────────────┐
│  luastring  — string interning, short/long strings  │
│  table      — hash+array hybrid table               │
└─────────────────────────────────────────────────────┘
          │
          ▼
Layer 2 — Runtime (depends on: object, luastring, table)
┌─────────────────────────────────────────────────────┐
│  state      — LuaState, GlobalState, CallInfo       │
│  closure    — LClosure, CClosure, UpVal management  │
│  metamethod — tag method dispatch, fasttm cache     │
└─────────────────────────────────────────────────────┘
          │
          ▼
Layer 3 — Compiler (depends on: object, opcode, luastring)
┌─────────────────────────────────────────────────────┐
│  lex        — lexer (source → tokens)               │
│  parse      — parser + code generator (tokens→Proto)│
└─────────────────────────────────────────────────────┘
          │
          ▼
Layer 4 — Execution Engine (depends on: all above)
┌─────────────────────────────────────────────────────┐
│  vm         — luaV_execute, all opcode handlers     │
│  do         — call/return, pcall, error, coroutines │
└─────────────────────────────────────────────────────┘
          │
          ▼
Layer 5 — API & Standard Libraries (depends on: all above)
┌─────────────────────────────────────────────────────┐
│  api        — Go API (lua_push*, lua_get*, etc.)    │
│  auxlib     — auxiliary helpers (luaL_check*, etc.)  │
│  baselib    — print, type, pairs, pcall, error...   │
│  strlib     — string.find, format, match, pack...   │
│  tablib     — table.insert, sort, concat, move...   │
│  mathlib    — math.*, random, maxinteger...         │
│  iolib      — io.open, read, write, lines...        │
│  oslib      — os.clock, date, execute...            │
│  dblib      — debug.getinfo, sethook, traceback...  │
│  utf8lib    — utf8.len, codes, char, codepoint...   │
│  corolib    — coroutine.create, resume, yield...    │
│  loadlib    — require, package.path, searchers...   │
└─────────────────────────────────────────────────────┘
          │
          ▼
Layer 6 — Composition Root
┌─────────────────────────────────────────────────────┐
│  lua        — NewState(), DoString(), DoFile()      │
│  testes     — test runner for lua-master/testes/    │
└─────────────────────────────────────────────────────┘
```

### Directory Structure

```
go-lua/
├── DESIGN.md                    # This file
├── .analysis/                   # Knowledge base (read-only reference)
├── lua-master/                  # C reference implementation
│
├── internal/
│   ├── object/                  # Layer 0: TValue, types
│   │   ├── api/api.go
│   │   └── object.go
│   │
│   ├── opcode/                  # Layer 0: instructions, opcodes
│   │   ├── api/api.go
│   │   └── opcode.go
│   │
│   ├── luastring/               # Layer 1: string interning
│   │   ├── api/api.go
│   │   └── luastring.go
│   │
│   ├── table/                   # Layer 1: Lua tables
│   │   ├── api/api.go
│   │   └── table.go
│   │
│   ├── state/                   # Layer 2: LuaState, GlobalState
│   │   ├── api/api.go
│   │   └── state.go
│   │
│   ├── closure/                 # Layer 2: closures, upvalues
│   │   ├── api/api.go
│   │   └── closure.go
│   │
│   ├── metamethod/              # Layer 2: tag methods
│   │   ├── api/api.go
│   │   └── metamethod.go
│   │
│   ├── lex/                     # Layer 3: lexer
│   │   ├── api/api.go
│   │   └── lex.go
│   │
│   ├── parse/                   # Layer 3: parser + codegen
│   │   ├── api/api.go
│   │   └── parse.go
│   │
│   ├── vm/                      # Layer 4: VM execution loop
│   │   ├── api/api.go
│   │   └── vm.go
│   │
│   ├── do/                      # Layer 4: call/return/error
│   │   ├── api/api.go
│   │   └── do.go
│   │
│   ├── api/                     # Layer 5: Go API
│   │   ├── api/api.go
│   │   └── api.go
│   │
│   └── stdlib/                  # Layer 5: all standard libraries
│       ├── api/api.go
│       ├── baselib.go
│       ├── strlib.go
│       ├── tablib.go
│       ├── mathlib.go
│       ├── iolib.go
│       ├── oslib.go
│       ├── dblib.go
│       ├── utf8lib.go
│       ├── corolib.go
│       └── loadlib.go
│
├── lua.go                       # Layer 6: composition root
├── lua_test.go                  # Integration tests
└── testes/                      # Layer 6: lua-master/testes runner
    └── testes_test.go
```

---

## §3 Key Design Decisions

### §3.1 TValue Representation

**C approach**: Tagged union (`Value` union + `tt_` byte) — 16 bytes on 64-bit.

**Go approach**: Go interface with type tag.

```go
// object/api/api.go
type Value interface {
    Type() Type        // base type (LUA_TNIL, LUA_TBOOLEAN, etc.)
    Tag() Tag          // full tag with variant bits
    // Type-specific accessors — panic if wrong type
    Integer() int64
    Number() float64
    Boolean() bool
    String() LuaString  // interface, not Go string
    Table() Table       // interface
    Closure() Closure   // interface
    // ...
}
```

**Decision**: Use whatever representation is simplest to implement correctly.
**Priority**: Correctness > simplicity > performance. If interface-based TValue is easier
to get right, use it. We can optimize later after all 31 tests pass.

**Option A** (simplest — interface-based):
```go
type TValue struct {
    tt  Tag   // type tag
    val any   // int64, float64, bool, *LuaString, *Table, *LClosure, *CClosure, etc.
}
```

**Option B** (more efficient — split fields):
```go
type TValue struct {
    tt  Tag
    i   int64  // integer OR float64 bits
    obj any    // GC objects only
}
```

**Guiding principle**: Choose whichever makes the VM opcode handlers simpler to write.
Performance optimization is a post-success concern.

**Reference**: .analysis/03-object-type-system.md §10

### §3.2 Stack Model

**C approach**: Contiguous C array (`StkIdRel` = pointer relative to stack base).

**Go approach**: `[]TValue` slice. Stack indices are `int` offsets.

```go
type LuaState struct {
    stack    []TValue    // the value stack
    top      int         // index of first free slot
    ci       *CallInfo   // current call frame
    // ...
}
```

**Key difference from C**: No `StkIdRel` pointer arithmetic. All stack access is `stack[index]`.
Stack growth via `append()` or manual reallocation with copy.

**Reference**: .analysis/04-call-return-error.md §11

### §3.3 Error Handling

**C approach**: `setjmp`/`longjmp` for protected calls.

**Go approach**: `panic`/`recover`.

```go
// do/internal: protected call wrapper
func pcall(L *LuaState, f func()) (err error) {
    defer func() {
        if r := recover(); r != nil {
            err = toLuaError(r)
        }
    }()
    f()
    return nil
}
```

**Decision**: Use `panic`/`recover` as the Go equivalent of `setjmp`/`longjmp`.
Lua errors (`luaG_runerror`, `luaD_throw`) become `panic(luaError{...})`.
Protected calls (`pcall`, `xpcall`) use `recover()`.

**Reference**: .analysis/04-call-return-error.md §6, §11

### §3.4 Garbage Collection Strategy

**C approach**: Custom tri-color incremental + generational GC with write barriers.

**Go approach**: **Rely on Go's GC entirely.** Do NOT reimplement mark-sweep.

**What we need**:
- `__gc` finalizer support → `runtime.SetFinalizer`
- Weak tables → `weak.Pointer` (Go 1.24+) or manual clearing
- Memory accounting → track allocations for `collectgarbage("count")`
- `collectgarbage()` API → `runtime.GC()` + accounting

**What we do NOT need**:
- Write barriers (Go GC handles this)
- Object lists (allgc, finobj, etc.)
- Tri-color marking
- Generational age tracking

**Reference**: .analysis/08-gc-memory.md §10

### §3.5 String Interning

**C approach**: Global hash table (`stringtable`) for short strings. Pointer equality.

**Go approach**: Use Go's built-in `string` type for most purposes. Intern short strings
via a `map[string]*LuaString` for pointer equality in table lookups.

**Decision**: `LuaString` wraps a Go `string` with optional interning for short strings.
This gives us Go string convenience (slicing, comparison) while preserving Lua's
pointer-equality optimization for table keys.

**Reference**: .analysis/07-runtime-infrastructure.md §4

### §3.6 Coroutines

**C approach**: Each coroutine is a `lua_State` with its own stack. Yield via `longjmp`.

**Go approach**: Each coroutine is a separate goroutine, communicating via channels.
OR: Each coroutine is a `LuaState` with its own stack, yield via `panic`/`recover`.

**Decision**: Use the `panic`/`recover` approach (same as C's longjmp model) — each
coroutine has its own `LuaState` and stack, but all run on the same goroutine.
This avoids goroutine overhead and matches C semantics exactly.

**Reference**: .analysis/04-call-return-error.md §8

---

## §4 Implementation Order

Strict bottom-up, each phase = one fork, each fork tests independently.

| Phase | Module(s) | Depends On | Estimated Lines | Test Strategy |
|-------|-----------|------------|-----------------|---------------|
| 1 | object, opcode | — | ~800 | Unit: type creation, tag checks, instruction encode/decode |
| 2 | luastring | object | ~300 | Unit: intern, hash, compare |
| 3 | table | object, luastring | ~600 | Unit: get/set, array/hash, resize, next |
| 4 | state, closure, metamethod | object, luastring, table | ~800 | Unit: state init, CallInfo, upvalue lifecycle, TM dispatch |
| 5 | lex | object, luastring | ~400 | Unit: tokenize Lua source snippets |
| 6 | parse | lex, object, opcode | ~1200 | Unit: parse → Proto, verify bytecode |
| 7 | vm, do | all Layer 0-3 | ~1500 | Unit: execute simple bytecode sequences |
| 8 | api, auxlib | all above | ~600 | Unit: stack manipulation, type checking |
| 9 | stdlib (baselib first) | all above | ~1500 | Integration: run simple Lua scripts |
| 10 | testes runner | all above | ~200 | Run lua-master/testes/, target 31/31 |

**Total estimated**: ~8,000 lines (vs previous 45,000 — the power of good design)

---

## §5 Module Specifications

Each module's complete interface is defined in its `api/api.go` file.
These files are the **source of truth** — implementation forks read them directly.
All 12 api packages compile cleanly: `go build ./internal/...` ✓

### Layer 0 — Foundation

| Module | Interface File | Key Types |
|--------|---------------|-----------|
| **§5.1 object** | `internal/object/api/api.go` | TValue, Tag, Type, LuaString, Proto, StackValue |
| **§5.2 opcode** | `internal/opcode/api/api.go` | Instruction (uint32), OpCode (85 opcodes), encode/decode |

### Layer 1 — Core Data Structures

| Module | Interface File | Key Types |
|--------|---------------|-----------|
| **§5.3 luastring** | `internal/luastring/api/api.go` | StringTable, Hash, NewShort/NewLong, Equal |
| **§5.4 table** | `internal/table/api/api.go` | Table (array+hash), Node, Get/Set/Next/RawLen |

### Layer 2 — Runtime

| Module | Interface File | Key Types |
|--------|---------------|-----------|
| **§5.5 state** | `internal/state/api/api.go` | LuaState, GlobalState, CallInfo, CFunction, LuaError |
| **§5.6 closure** | `internal/closure/api/api.go` | LClosure, CClosure, UpVal (open/closed duality) |
| **§5.7 metamethod** | `internal/metamethod/api/api.go` | TMS enum (25 methods), fasttm cache helpers |

### Layer 3 — Compiler

| Module | Interface File | Key Types |
|--------|---------------|-----------|
| **§5.8 lex** | `internal/lex/api/api.go` | TokenType, Token, LexState, LexReader, SyntaxError |
| **§5.9 parse** | `internal/parse/api/api.go` | ExpDesc, FuncState, BlockCnt, Dyndata, BinOpr/UnOpr |

### Layer 4 — Execution Engine (vm + do merged)

| Module | Interface File | Key Types |
|--------|---------------|-----------|
| **§5.10 vm** | `internal/vm/api/api.go` | LuaError, Status, CallStatus flags, MultiReturn |

> **Architecture note**: C's `lvm.c` and `ldo.c` are mutually recursive.
> In Go, circular imports are forbidden, so they are merged into one `internal/vm/` package.
> Separate files (`execute.go`, `call.go`, `arith.go`) maintain logical organization.

### Layer 5 — API & Standard Libraries

| Module | Interface File | Key Types |
|--------|---------------|-----------|
| **§5.12 api** | `internal/api/api/api.go` | State, CFunction, stack ops, table ops, auxiliary (luaL_*) |
| **§5.13 stdlib** | `internal/stdlib/api/api.go` | OpenFunc, Library, Open{Base,String,Table,Math,...} |

### Dependency Graph (verified: no cycles)

```
object ←── luastring, table, state, closure, parse
opcode ←── (standalone)
lex    ←── parse
closure ←── parse
vm     ←── api
api    ←── stdlib
```

---

## §6 Design Decisions Log

| Decision | Choice | Alternative | Why |
|----------|--------|-------------|-----|
| TValue representation | Struct with tag + any | Split int64/obj fields | Simplest to implement; optimize after success |
| Error handling | panic/recover | Error return values | Matches C's setjmp/longjmp model exactly |
| GC strategy | Use Go's GC entirely | Reimplement tri-color | Reduces difficulty massively; add __gc later if needed |
| String representation | Go string + interning map | Custom byte buffer | Go strings are immutable and GC'd — simplest |
| Coroutine model | Separate LuaState, same goroutine | Goroutine per coroutine | Matches C semantics, simplest correct approach |
| Module structure | internal/ with api/api.go | Flat package structure | ZeroFAS methodology: interface-first, fork-friendly |
| Compiler | Single-pass (like C) | AST-based two-pass | Faithful to reference, proven correct, less risk |
| **Overall priority** | **Correctness > Simplicity > Performance** | — | **Project may fail; maximize success probability** |

---

## §7 Test Strategy

### Unit Tests (per module)
Each module has `*_test.go` that tests its api/api.go contract independently.
Mock dependencies where needed (e.g., table tests don't need a full LuaState).

### Integration Tests
- `lua_test.go`: Run Lua code snippets via `DoString()`, verify output
- `testes/testes_test.go`: Run all 31 lua-master/testes/ files, track pass/fail

### Success Criteria
- `go build ./...` passes
- `go test ./...` passes (all unit tests)
- `testes/`: 31/31 lua-master/testes/ files pass

---

## §8 Risk Analysis

| Risk | Mitigation |
|------|------------|
| Parser complexity (2200 lines of C) | Follow C structure closely, use .analysis/06 as guide |
| VM opcode coverage (77 opcodes) | Implement incrementally, test each category |
| String pattern matching | Document full algorithm from .analysis/09 §4, test extensively |
| Coroutine yield/resume | Follow .analysis/04 §8 exactly, test C→Lua→C chains |
| setmetatable type checking | Previous bug — test explicitly with all type combinations |
| Call/return result count | Previous bug — test MULTRET, vararg, tail calls |
