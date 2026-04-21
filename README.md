# go-lua

**Pure Go implementation of Lua 5.5.1 | 纯 Go 实现的 Lua 5.5.1 虚拟机**

![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)
![Tests](https://img.shields.io/badge/tests-911%20passed-brightgreen)
![Lua 5.5](https://img.shields.io/badge/Lua-5.5.1-blue?logo=lua&logoColor=white)
![Zero Dependencies](https://img.shields.io/badge/dependencies-zero-success)
![License](https://img.shields.io/badge/license-MIT-green)

A complete, production-quality Lua 5.5.1 virtual machine written entirely in Go — no CGo, no external dependencies. Passes **26 official Lua 5.5 test suites** and achieves **near-parity performance** with the C reference implementation.

---

## Features

- **Full Lua 5.5.1 language** — all operators, control flow, closures, metatables, varargs
- **10 standard libraries** — base, string, table, math, io, os, coroutine, debug, utf8, package
- **Lua 5.5 additions** — to-be-closed variables (`<close>`), generational GC, integer/float types, bitwise operators, floor division (`//`)
- **Coroutine yield across metamethods** — full support for yielding inside `__index`, `__newindex`, `__call`, etc.
- **Mark-and-sweep GC** with generational mode, integrated with Go's garbage collector
- **String interning** via `weak.Pointer` for memory-efficient string handling
- **~26,000 lines of source** across 13 internal packages, with **8,570 lines of tests**

## Performance

### Computation — At Parity with C Lua 5.5.1

Benchmarked against C Lua 5.5.1 on the same hardware (Intel i7-14700KF):

| Benchmark | Go-Lua | C Lua 5.5.1 | Ratio |
|---|---|---|---|
| Numeric for-loop (1M iterations) | ~5ms | ~5ms | **1:1** |
| Fibonacci(20) recursive | ~0.54ms | ~1ms | **Go faster** |
| Method call (100K OOP dispatch) | ~7.8ms | ~7ms | **~1.1:1** |
| Table ops (10K read/write) | ~1.6ms | — | — |
| Closure creation (10K) | ~3.7ms | — | — |

### GC Performance

| Metric | Go-Lua | C Lua 5.5.1 | Notes |
|---|---|---|---|
| GC collect (50K live objects) | 780µs | 500µs | **1.6x** — close |
| GC weak table clearing | 1.1ms | 1.2ms | **~1:1** — parity |
| Object creation (100K tables) | ~97ms | ~15ms | 6.5x — Go allocator overhead |
| Memory usage | 7042 KB | 5289 KB | 1.33x — TValue 32B vs C's 16B |

### Optimization Highlights

- **Zero-alloc numeric operations** — TValue uses dual-field struct (`int64` + `any`) instead of `interface{}` boxing. Numeric for-loops dropped from 3M allocations to ~1K (3000× reduction).
- **Table object pool** — `sync.Pool` reuses dead Table structs, cutting GC benchmark allocations by 48%.
- **Slim GC headers** — GCHeader reduced from 48 to 32 bytes by replacing intrusive linked lists with slice-based gray lists.
- **Ephemeron fast-path** — Skips O(N) allgc chain walk when no ephemeron tables exist.
- **Pre-computed object sizes** — Eliminates type assertions during GC sweep.
- **Capacity-based stack growth** — Stack grows within existing capacity without reallocation.
- **CallInfo slab allocation** — Batch-allocates 32 CallInfo structs at a time.

## Quick Start

```go
package main

import (
    luaapi "github.com/akzj/go-lua/internal/api"
    "github.com/akzj/go-lua/internal/stdlib"
    "fmt"
)

func main() {
    L := luaapi.NewState()
    defer L.Close()
    stdlib.OpenAll(L)

    // Execute Lua code
    if err := L.DoString(`print("Hello from Go-Lua!")`); err != nil {
        fmt.Println("Error:", err)
    }

    // Register a Go function and call it from Lua
    L.PushCFunction(func(L *luaapi.State) int {
        name := L.CheckString(1)
        L.PushString(fmt.Sprintf("Hello, %s!", name))
        return 1
    })
    L.SetGlobal("greet")

    L.DoString(`print(greet("World"))`)
}
```

## Standard Libraries

| Library | Highlights |
|---|---|
| **base** | `print`, `type`, `tostring`, `tonumber`, `assert`, `error`, `pcall`, `xpcall`, `select`, `ipairs`, `pairs`, `next`, `rawget`, `rawset`, `rawlen`, `rawequal`, `setmetatable`, `getmetatable`, `require`, `load`, `dofile`, `collectgarbage` |
| **string** | `find`, `gsub`, `gmatch`, `match`, `format`, `rep`, `sub`, `byte`, `char`, `len`, `reverse`, `upper`, `lower`, `dump`, `pack`, `unpack`, `packsize` |
| **table** | `concat`, `insert`, `remove`, `sort`, `move`, `unpack`, `pack`, `create` |
| **math** | Full library including integer/float operations, `random` |
| **io** | Full file I/O |
| **os** | `clock`, `date`, `time`, `difftime`, `exit`, `getenv`, `remove`, `rename`, `tmpname` |
| **coroutine** | `create`, `resume`, `yield`, `status`, `wrap`, `isyieldable`, `close` |
| **debug** | `getinfo`, `getlocal`, `setlocal`, `getupvalue`, `setupvalue`, `upvaluejoin`, `upvalueid`, `sethook`, `gethook`, `getuservalue`, `setuservalue`, `traceback` |
| **utf8** | `char`, `codepoint`, `codes`, `len`, `offset`, `charpattern` |
| **package** | `require`, `searchpath` |

## Architecture

```
internal/
├── api/        — Public Lua API (State, stack ops, type checks)
├── closure/    — Closures and upvalues
├── gc/         — Mark-and-sweep garbage collector (V5)
├── lex/        — Lexer/scanner
├── luastring/  — String interning (weak.Pointer based)
├── metamethod/ — Metamethod dispatch and tag method cache
├── object/     — Core types (TValue, LuaString, Proto, etc.)
├── opcode/     — Bytecode opcodes and instruction encoding
├── parse/      — Parser and code generator
├── state/      — Lua state and CallInfo
├── stdlib/     — Standard library implementations
├── table/      — Hash table with Brent's algorithm
└── vm/         — Virtual machine (execute, do, debug)
```

## Test Suite

**911 Go unit tests** plus **26 official Lua 5.5 test suites** from the reference implementation:

`strings` · `math` · `sort` · `vararg` · `constructs` · `events` · `calls` · `locals` · `bitwise` · `tpack` · `code` · `api` · `nextvar` · `pm` · `db` · `attrib` · `coroutine` · `errors` · `goto` · `literals` · `utf8` · `closure` · `gc` · `gengc` · `files` · `cstack`

### Running Tests

```bash
# All Go unit tests
go test ./... -count=1 -timeout 300s

# Official Lua 5.5 test suite
go test ./internal/stdlib/ -run TestTestesWide -v

# Performance benchmarks
go test ./internal/stdlib/ -bench=. -benchmem
```

## Requirements

- **Go 1.24+**
- No external dependencies

## Installation

```bash
go get github.com/akzj/go-lua
```

## License

[MIT](LICENSE) © akzj
