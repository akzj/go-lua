# go-lua

**Pure Go implementation of Lua 5.5.1 | 纯 Go 实现的 Lua 5.5.1 虚拟机**

![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)
![Tests](https://img.shields.io/badge/tests-29%2F29_passing-brightgreen)
![Lua 5.5](https://img.shields.io/badge/Lua-5.5.1-blue?logo=lua&logoColor=white)
![Zero Dependencies](https://img.shields.io/badge/dependencies-zero-success)
![License](https://img.shields.io/badge/license-MIT-green)

A complete, production-quality Lua 5.5.1 virtual machine written entirely in Go — no CGo, no external dependencies. Passes **all 29 official Lua 5.5.1 test suites** (including the C-API test suite and generational GC tests) with a **2.86× geometric mean** vs C Lua on computation benchmarks.

**Lua 5.5 compatible. Go-native.**

---

## Features

- **Full Lua 5.5.1 language** — all operators, control flow, closures, metatables, varargs
- **10 standard libraries** — base, string, table, math, io, os, coroutine, debug, utf8, package
- **Lua 5.5 additions** — to-be-closed variables (`<close>`), generational GC, integer/float types, bitwise operators, floor division (`//`)
- **Coroutine yield across metamethods** — full support for yielding inside `__index`, `__newindex`, `__call`, etc.
- **Go-native API** — auto-bind Go functions with `PushGoFunc`, push any Go value with `PushAny`, type-safe generics with `Wrap2R`
- **Built-in sandbox** — `NewSandboxState` with CPU limits, memory limits, and library restrictions
- **Context integration** — `SetContext(ctx)` for deadline/cancellation via standard Go `context.Context`
- **Resource safety** — `PushResource` auto-closes Go objects via `__gc`; `PushCloseableResource` adds Lua 5.5 `<close>` support
- **Safe calls** — `SafeCall` provides PCall + automatic `debug.traceback`; `WrapSafe` converts Go panics to Lua errors
- **Leak detection** — `RefTracker` finds orphaned registry references during development
- **Virtual filesystem** — `SetFileSystem(fs.FS)` for `go:embed` and custom file sources
- **Mark-and-sweep GC** with generational mode, integrated with Go's garbage collector
- **String interning** via `weak.Pointer` for memory-efficient string handling
- **testC testing library** — 97 C-API-level instructions with multi-state testing (`newstate`/`closestate`/`doremote`)
- **Public embedding API** — clean `pkg/lua/` package for external use
- **~31,500 lines of source** across 13 internal packages, with **~10,800 lines of tests**

## Why go-lua?

Most Lua-in-Go libraries expose a raw C-style stack API. go-lua gives you that *and* a Go-native layer on top:

| | C-style Lua bindings | go-lua |
|---|---|---|
| Register a Go function | 8 lines of stack manipulation | `L.PushGoFunc(myFunc)` — 1 line |
| Pass a struct to Lua | Manual Push+SetField per field | `L.PushAny(myStruct)` — 1 line |
| Read a Lua table | GetField+ToString+Pop per field | `L.GetFieldString(idx, "key")` — 1 line |
| Safe function call | PCall + manual error extraction | `L.SafeCall(nArgs, nResults)` → Go error with traceback |
| Execute untrusted code | Build your own sandbox | `lua.NewSandboxState(config)` — built-in |
| Timeout control | Not possible | `L.SetContext(ctx)` — standard Go context |
| Load embedded files | Not possible | `L.SetFileSystem(embedFS)` — Go embed support |

## Quick Start

```go
package main

import (
    "fmt"
    lua "github.com/akzj/go-lua/pkg/lua"
)

func main() {
    L := lua.NewState()
    defer L.Close()

    // Execute Lua code
    L.DoString(`print("Hello from Go-Lua!")`)

    // Register Go functions — no stack manipulation needed
    L.PushGoFunc(func(name string, age int) string {
        return fmt.Sprintf("Hello %s, you are %d!", name, age)
    })
    L.SetGlobal("greet")

    L.DoString(`print(greet("World", 42))`)
    // Output: Hello World, you are 42!
}
```

## Three API Layers

go-lua provides three ways to register Go functions, from simplest to most flexible:

### Layer 1: Auto-binding (recommended for most cases)

```go
// Reflect-based — works with any Go function signature
L.PushGoFunc(func(name string, count int) (string, error) {
    if count < 0 {
        return "", fmt.Errorf("count must be positive")
    }
    return strings.Repeat(name, count), nil
})
L.SetGlobal("repeat_str")
// Lua: repeat_str("ha", 3) → "hahaha"
// Lua: repeat_str("x", -1) → raises Lua error
```

### Layer 2: Type-safe generics (zero reflection overhead)

```go
// For performance-sensitive paths — no reflect, just generics
lua.Wrap2R(L, func(a, b int) int { return a + b })
L.SetGlobal("add")
// Lua: add(10, 20) → 30
```

### Layer 3: Raw stack API (full control)

```go
// When you need direct Lua stack access
L.PushFunction(func(L *lua.State) int {
    n := L.GetTop() // number of arguments
    sum := 0.0
    for i := 1; i <= n; i++ {
        v, _ := L.ToNumber(i)
        sum += v
    }
    L.PushNumber(sum)
    return 1 // one return value
})
L.SetGlobal("sum")
// Lua: sum(1, 2, 3, 4) → 10.0
```

## Built-in Sandbox

Execute untrusted Lua code safely with CPU limits, memory limits, and restricted library access:

```go
L := lua.NewSandboxState(lua.SandboxConfig{
    MemoryLimit: 10 * 1024 * 1024,  // 10MB
    CPULimit:    1_000_000,          // 1M VM instructions
    AllowIO:     false,              // block io/os libraries
    AllowDebug:  false,              // block debug library
})
defer L.Close()

err := L.DoString(untrustedCode) // safe!
```

The sandbox disables `dofile`, `loadfile`, `load`, and `require` by default. Only safe libraries (base, string, table, math, utf8, coroutine) are loaded. Enable `AllowIO`, `AllowDebug`, or `AllowPackage` selectively as needed.

## Resource Safety

Prevent Go resource leaks when Lua code forgets to close handles:

```go
// Go resources auto-close when Lua GC collects them
conn := db.Open(dsn)
L.PushResource(conn)         // __gc calls conn.Close()
L.SetGlobal("conn")

// Or with Lua 5.5 <close> support (auto-close on scope exit)
L.PushCloseableResource(conn)
```

```lua
-- Lua side: <close> variable auto-closes on scope exit
local conn <close> = db.open(dsn)
-- conn:close() called automatically, even on error

-- Or explicit close (idempotent)
conn:close()
```

Safe function calls with automatic traceback:

```go
L.GetGlobal("handler")
L.PushString(request)
err := L.SafeCall(1, 1) // error includes full Lua stack trace

// Wrap Go functions: panics become Lua errors (not crashes)
L.PushFunction(lua.WrapSafe(riskyHandler))
L.SetGlobal("handle")
```

Development-time leak detection:

```go
tracker := lua.NewRefTracker()
ref := tracker.Ref(L, lua.RegistryIndex)
// ... if you forget Unref ...
fmt.Println(tracker.Leaks()) // shows file:line where ref was created
```

## Performance

Benchmarked against C Lua 5.5.1 (`lua-master/lua`) using `tools/luabench.sh` (median of 5 runs, `os.clock()` CPU time):

| Benchmark | C Lua (ms) | go-lua (ms) | Ratio |
|-----------|----------:|------------:|------:|
| Concat Multi | 8.06 | 8.47 | **1.05×** |
| Method Call | 36.64 | 53.73 | **1.47×** |
| Pattern Match | 23.49 | 37.89 | **1.61×** |
| Fibonacci (recursive) | 13.37 | 24.15 | **1.81×** |
| For Loop | 120.69 | 227.21 | **1.88×** |
| String Concat | 15.77 | 32.09 | **2.04×** |
| Closure Creation | 33.77 | 72.89 | **2.16×** |
| Coroutine Create/Resume/Finish | 75.82 | 323.31 | **4.26×** |
| Coroutine Create | 43.48 | 222.85 | **5.13×** |
| Table Ops | 13.29 | 70.78 | **5.33×** |
| Coroutine Yield/Resume | 35.14 | 188.69 | **5.37×** |
| GC | 26.23 | 142.81 | **5.44×** |
| Concat Operator | 3.16 | 21.16 | **6.70×** |
| **Geometric Mean** | | | **2.86×** |

> Pure computation (fibonacci, pattern matching, for-loops, method calls) runs within **1.0–2.1×** of C Lua.
> Allocation-heavy workloads (coroutines, tables, GC) are **4–7×** due to Go runtime overhead.

### Optimization Highlights

- **Zero-alloc numeric operations** — TValue uses dual-field struct (`int64` + `any`) instead of `interface{}` boxing. Numeric for-loops dropped from 3M allocations to ~1K (3000× reduction).
- **Table object pool** — `sync.Pool` reuses dead Table structs, cutting GC benchmark allocations by 48%.
- **Slim GC headers** — GCHeader reduced from 48 to 32 bytes by replacing intrusive linked lists with slice-based gray lists.
- **Ephemeron fast-path** — Skips O(N) allgc chain walk when no ephemeron tables exist.
- **Pre-computed object sizes** — Eliminates type assertions during GC sweep.
- **Capacity-based stack growth** — Stack grows within existing capacity without reallocation.
- **CallInfo slab allocation** — Batch-allocates 32 CallInfo structs at a time.
- **`strings.Join` for concat** — Multi-value `..` operator uses `strings.Join` instead of repeated allocation (Concat Multi 1.05×).
- **Stack-alloc string parts** — Small concat operations use stack-allocated arrays to avoid heap escapes.
- **Table fast-path updates** — `Table.SetIfExists` skips hash insertion when the key already exists.
- **Inline `checkGC`** — Countdown counter with slow-path split reduces per-instruction GC check overhead.
- **Integer for-loop fast path** — Extracted `forLoopInt` inlines at cost 43, eliminating function call overhead.
- **Array fast paths** — OP_GETI/OP_SETI bypass `tableSetWithMeta` for direct array access.
- **Non-interned concat strings** — Skip string interning for concat results, reducing hash table pressure.
- **Direct GetStr lookups** — OP_GETFIELD/GETTABUP/SELF use `GetStr` to skip type dispatch on string keys.
- **OP_POW fast paths** — x², x³, √x computed directly without `math.Pow`.

## Documentation

| Document | Description |
|----------|-------------|
| [API Reference](docs/API_REFERENCE.md) | Complete Go API with C Lua equivalents |
| [Capabilities](docs/CAPABILITIES.md) | Standard library completeness, language features |
| [Cookbook](docs/COOKBOOK.md) | 25 task-driven recipes for common tasks |
| [Embedding Guide](docs/embedding-guide.md) | Getting started with go-lua |
| [go doc](https://pkg.go.dev/github.com/akzj/go-lua/pkg/lua) | AI-friendly API reference via `go doc ./pkg/lua/` |

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
| `-l name` | Preload a library (`require("name")`) before executing scripts |
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
