# go-lua

**Pure Go implementation of Lua 5.5.1 | 纯 Go 实现的 Lua 5.5.1 虚拟机**

![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)
![Tests](https://img.shields.io/badge/tests-29%2F29_passing-brightgreen)
![Lua 5.5](https://img.shields.io/badge/Lua-5.5.1-blue?logo=lua&logoColor=white)
![Zero Dependencies](https://img.shields.io/badge/dependencies-zero-success)
![License](https://img.shields.io/badge/license-MIT-green)

A complete, production-quality Lua 5.5.1 virtual machine written entirely in Go — no CGo, no external dependencies. Passes **all 29 official Lua 5.5.1 test suites** (including the C-API test suite and generational GC tests) with a **3.1× geometric mean** vs C Lua on computation benchmarks.

---

## Features

- **Full Lua 5.5.1 language** — all operators, control flow, closures, metatables, varargs
- **10 standard libraries** — base, string, table, math, io, os, coroutine, debug, utf8, package
- **Lua 5.5 additions** — to-be-closed variables (`<close>`), generational GC, integer/float types, bitwise operators, floor division (`//`)
- **Coroutine yield across metamethods** — full support for yielding inside `__index`, `__newindex`, `__call`, etc.
- **Mark-and-sweep GC** with generational mode, integrated with Go's garbage collector
- **String interning** via `weak.Pointer` for memory-efficient string handling
- **testC testing library** — 97 C-API-level instructions with multi-state testing (`newstate`/`closestate`/`doremote`)
- **Public embedding API** — clean `pkg/lua/` package for external use
- **~30,500 lines of source** across 13 internal packages, with **~9,700 lines of tests**

## Performance

Benchmarked against C Lua 5.5.1 (`lua-master/lua`) using `tools/luabench.sh` (median of 5 runs, `os.clock()` CPU time):

| Benchmark | C Lua (ms) | go-lua (ms) | Ratio |
|-----------|----------:|------------:|------:|
| Fibonacci (recursive) | 19.52 | 24.98 | **1.28×** |
| Pattern Match | 22.90 | 38.11 | **1.66×** |
| Concat Multi | 5.67 | 11.12 | **1.96×** |
| For Loop | 121.23 | 250.30 | **2.06×** |
| Method Call | 35.44 | 76.63 | **2.16×** |
| Closure Creation | 33.03 | 71.69 | **2.17×** |
| Concat Operator | 5.85 | 13.46 | **2.30×** |
| String Concat | 10.22 | 37.15 | **3.63×** |
| Coroutine Create/Resume/Finish | 72.50 | 337.42 | **4.65×** |
| Coroutine Create | 44.84 | 222.50 | **4.96×** |
| Coroutine Yield/Resume | 34.66 | 198.95 | **5.74×** |
| GC | 26.33 | 167.50 | **6.36×** |
| Table Ops | 10.45 | 88.94 | **8.51×** |
| **Geometric Mean** | | | **3.10×** |

> Pure computation (fibonacci, pattern matching, for-loops) runs within **1.3–2.1×** of C Lua.
> Allocation-heavy workloads (coroutines, tables, string concat) are **4–9×** due to Go runtime overhead.

### Optimization Highlights

- **Zero-alloc numeric operations** — TValue uses dual-field struct (`int64` + `any`) instead of `interface{}` boxing. Numeric for-loops dropped from 3M allocations to ~1K (3000× reduction).
- **Table object pool** — `sync.Pool` reuses dead Table structs, cutting GC benchmark allocations by 48%.
- **Slim GC headers** — GCHeader reduced from 48 to 32 bytes by replacing intrusive linked lists with slice-based gray lists.
- **Ephemeron fast-path** — Skips O(N) allgc chain walk when no ephemeron tables exist.
- **Pre-computed object sizes** — Eliminates type assertions during GC sweep.
- **Capacity-based stack growth** — Stack grows within existing capacity without reallocation.
- **CallInfo slab allocation** — Batch-allocates 32 CallInfo structs at a time.
- **`strings.Join` for concat** — Multi-value `..` operator uses `strings.Join` instead of repeated allocation (Concat Multi 1.96×).
- **Stack-alloc string parts** — Small concat operations use stack-allocated arrays to avoid heap escapes.
- **Table fast-path updates** — `Table.SetIfExists` skips hash insertion when the key already exists.

## Quick Start

```go
package main

import (
    "fmt"
    lua "github.com/akzj/go-lua/pkg/lua"
)

func main() {
    L := lua.NewState()  // Opens all standard libraries
    defer L.Close()

    // Execute Lua code
    if err := L.DoString(`print("Hello from Go-Lua!")`); err != nil {
        fmt.Println("Error:", err)
    }

    // Register a Go function and call it from Lua
    L.PushCFunction(func(L *lua.State) int {
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

## testC Testing Library

The official Lua 5.5 test suite includes `api.lua`, which tests the C API through a mini-language called `T.testC`. This project implements the full `T` library in pure Go:

- **97 testC instructions** mapping to Lua C API functions (`pushvalue`, `gettable`, `call`, `yield`, `resume`, etc.)
- **Multi-state testing** — `newstate`, `closestate`, and `doremote` for testing independent Lua states
- **53 auxiliary functions** — `checkmemory`, `totalmem`, `udataval`, `makeCfunc`, `coresume`, and more
- **~1,450 lines** in `internal/stdlib/testlib.go`

A few C-specific test sections are skipped due to Go runtime differences:
- `alloccount` — Go does not expose per-allocation counting
- GC finalizer ordering — Go finalizers run non-deterministically
- Certain `debug.sethook` edge cases involving C-level hook callbacks

## Architecture

```
pkg/lua/            — Public embedding API (State, stack ops, type checks)
internal/
├── api/            — Internal Lua API implementation
├── closure/        — Closures and upvalues
├── gc/             — Mark-and-sweep + generational garbage collector
├── lex/            — Lexer/scanner
├── luastring/      — String interning (weak.Pointer based)
├── metamethod/     — Metamethod dispatch and tag method cache
├── object/         — Core types (TValue, LuaString, Proto, etc.)
├── opcode/         — Bytecode opcodes and instruction encoding
├── parse/          — Parser and code generator
├── state/          — Lua state and CallInfo
├── stdlib/         — Standard library implementations + testC library
├── table/          — Hash table with Brent's algorithm
└── vm/             — Virtual machine (execute, do, debug)
```

## Command-Line Interpreter (`cmd/glua`)

A standalone Lua 5.5 interpreter powered by go-lua, with interactive REPL support.

### Install

```bash
go install github.com/akzj/go-lua/cmd/glua@latest
```

### Usage

```
glua [options] [script [args...]]
```

| Flag | Description |
|------|-------------|
| `-e "code"` | Execute a Lua string |
| `-i` | Enter interactive REPL after executing scripts |
| `-v` | Print version and exit |
| `-` | Read script from stdin |

When invoked with no arguments, `glua` enters an interactive REPL.

### Examples

```bash
# Run a Lua script
glua hello.lua

# One-liner
glua -e 'print("Hello from go-lua!")'

# Pipe from stdin
echo 'print(1+2)' | glua -

# Interactive REPL
glua
> 1 + 2
3
> for i = 1, 3 do print(i) end
1
2
3
```

## Testing

Passes **29 of 29** official Lua 5.5.1 test suites (`lua-master/testes/`):

`strings` · `math` · `sort` · `vararg` · `constructs` · `events` · `calls` · `locals` · `bitwise` · `tpack` · `code` · `api` · `big` · `bwcoercion` · `verybig` · `nextvar` · `pm` · `db` · `attrib` · `coroutine` · `errors` · `goto` · `literals` · `utf8` · `closure` · `gc` · `gengc` · `files` · `cstack`

### Running Tests

```bash
# All Go unit tests
go test ./... -count=1 -timeout 300s

# Official Lua 5.5 test suite
go test -run TestTestesWide -v ./internal/stdlib/

# Performance benchmarks (internal)
go test ./internal/stdlib/ -bench=. -benchmem

# Performance comparison vs C Lua
./tools/luabench.sh 3
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
