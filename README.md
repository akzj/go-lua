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
- **25,359 lines of source** across 13 internal packages, with **8,570 lines of tests**

## Performance

Benchmarked against C Lua 5.5.1 on the same hardware:

| Benchmark | Go-Lua | C Lua 5.5.1 | Ratio |
|---|---|---|---|
| ForLoop (1M iterations) | ~5ms | ~5ms | **1:1** |
| Fibonacci(20) | ~0.53ms | ~1ms | **Go faster** |
| Method call (100K) | ~8ms | ~7ms | ~1.1:1 |

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
