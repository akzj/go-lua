# go-lua Design Philosophy

This document explores the architectural thinking and design philosophy behind go-lua.

## Core Philosophy

**Faithful implementation with pragmatic adaptation.** We aim for semantic compatibility with Lua 5.5.1 while embracing Go's strengths.

### Three Tenets

1. **Correctness over performance** — A bug-free slow VM beats a fast buggy one
2. **Simplicity over cleverness** — Code is read more than written
3. **Interfaces over implementations** — Depend on abstractions, not concretions

---

## Language Choice: Why Go?

### Arguments For

| Factor | Go Advantage |
|--------|--------------|
| Garbage Collection | Native GC integration, no manual memory management |
| Interface System | Duck typing enables clean API design |
| Concurrency | Native coroutine support (goroutines) |
| Portability | Single binary, no external dependencies |
| Performance | Fast enough for most scripting needs |

### Arguments Against / Trade-offs

| Factor | Consideration |
|--------|---------------|
| No Pointer Arithmetic | Limits direct memory layout control |
| No Tail Call Optimization | Lua semantics preserved, but stack grows |
| GC Pauses | Lua's incremental GC mitigates this |
| Runtime Reflection | Limited, but Lua values are already typed |

### Decision

Go's strengths outweigh limitations for a scripting VM. The interface system enables clean API design, and Go's GC eliminates a major implementation complexity.

---

## Architecture Decisions

### Layered Architecture

```
┌─────────────────────────────────────────┐
│            User Code                     │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│  lex/ parse/ compile → bytecode         │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│  vm/ → execution                         │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│  state/ → stack, calls, globals         │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│  gc/ → memory management                 │
└─────────────────────────────────────────┘
```

**Why layers?**
- Each layer has a single responsibility
- Changes in one layer don't ripple to others
- Testing is easier at layer boundaries
- The compiler catches cross-layer errors

### API Package Pattern

Each subsystem has an `api/` subpackage:

```
state/
├── api/
│   └── api.go      ← Interface definitions
└── internal/
    └── state.go    ← Implementation
```

**Why this pattern?**
- Clear separation of "what" (interface) from "how" (implementation)
- Prevents circular dependencies
- Enables interface-based testing
- Makes dependencies explicit in imports

**Why not put interfaces in the parent?**
- Would create import cycles between packages
- Implementation packages need their api, creating bidirectional deps
- Subpackage structure is idiomatic Go (`encoding/json`, `net/http`)

---

## Implementation Trade-offs

### Tagged Union vs Interface{}

**Option A: Go interface{}**
```go
type TValue struct {
    value interface{}
}
```

**Option B: Tagged union**
```go
type TValue struct {
    value    interface{}
    gcObject interface{}
    extra    uintptr  // packed type info
}
```

**Decision: Tagged union**

**Rationale:**
- Lua types are a closed set; the tag is known at compile time
- Type packing enables fast type checks (bitwise ops vs type assertions)
- GC needs to know if a value is collectable
- Memory layout is explicit and predictable

### Register-Based VM vs Stack-Based

**Option A: Stack-based (Lua 4.0)**
```lua
LOADK 0, 1      -- push constant 1
LOADK 1, 2      -- push constant 2
ADD 0, 0, 1    -- pop 2, push result
```
Accumulator-based, one implicit register.

**Option B: Register-based (Lua 5.x)**
```lua
LOADK R(0), 1   -- R[0] = 1
LOADK R(1), 2   -- R[1] = 2
ADD R(0), R(0), R(1)  -- R[0] = R[0] + R[1]
```
Explicit registers, no stack push/pop.

**Decision: Register-based**

**Rationale:**
- Matches Lua 5.x specification (must be compatible)
- Fewer LOAD/STORE operations (operands in registers)
- Better register allocation
- Cleaner code generation (expressions map directly)

### Incremental vs Stop-the-World GC

**Option A: Stop-the-world**
- Simple implementation
- Long pauses on large heaps
- Good for batch processing

**Option B: Incremental**
- Complex implementation (tri-color marking)
- Short, predictable pauses
- Better for interactive applications

**Decision: Incremental**

**Rationale:**
- Go runtime is already incremental
- User-facing scripts need responsive GC
- Matches Lua 5.x behavior

---

## Error Handling Philosophy

### Lua's Error Model

Lua uses exceptions (longjmp in C):

```lua
-- Error propagates up call stack
function dangerous()
    error("oops!")
end

function safe()
    pcall(dangerous)  -- Caught here
end
```

### Go Implementation

```go
func (ls *LuaState) PCall(nArgs, nResults int, errfunc func(err error)) {
    defer func() {
        if r := recover(); r != nil {
            // Handle error
            ls.pushError(r)
        }
    }()
    ls.call(nArgs, nResults)
}
```

**Design decisions:**
- Use `panic/recover` for control flow (matches Lua semantics)
- `PCall` establishes protected execution context
- Errors are values at API boundary (`error` return type)

---

## Closures and Upvalues

### The Closure Problem

```lua
function outer()
    local x = 10
    return function()
        return x  -- Captures 'x'
    end
end
```

The inner function must access `x` from outer scope even after `outer()` returns.

### Implementation Approach

```
┌─────────────────────────────────────┐
│ Closure                             │
│ ┌─────────────────────────────────┐ │
│ │ Prototype (immutable)          │ │
│ │   - Instructions               │ │
│ │   - Constants                   │ │
│ │   - Upvalue descriptions         │ │
│ └─────────────────────────────────┘ │
│ ┌─────────────────────────────────┐ │
│ │ Upvalue[] (captured variables) │ │
│ └─────────────────────────────────┘ │
└─────────────────────────────────────┘
```

**Key design:**
- Upvalues point to stack slots or heap storage
- Closed over variables move to heap when outer function returns
- Single-level upvalue resolution (matches Lua semantics)

---

## Coroutines: Symmetric vs Asymmetric

### Lua's Choice: Asymmetric

```lua
-- Resume/yield are paired
co = coroutine.create(function()
    local x = 1
    while true do
        x = coroutine.yield(x * 2)
    end
end)

coroutine.resume(co)     -- x = 1, yields 2
coroutine.resume(co, 5)  -- x = 5, yields 10
```

**Why asymmetric?**
- Clear ownership: `resume` caller is not the same as `yield` caller
- Simpler mental model
- `yield` returns control to `resume` caller, not arbitrary coroutine

### Implementation

Each thread (LuaState) has:
- Its own stack
- Its own call frame chain
- Shared global state (registry, allocator, GC)

```
GlobalState
├── Registry
├── Allocator
├── GC
└── Threads[]
    ├── Thread 1: stack, frames, code
    └── Thread 2: stack, frames, code
```

---

## Metatables: Object Orientation via Tables

### Lua's Approach

Objects are just tables with special behavior:

```lua
Vector = {}
Vector.__index = Vector

function Vector.new(x, y)
    return setmetatable({x = x, y = y}, Vector)
end

function Vector:add(other)
    return Vector.new(self.x + other.x, self.y + other.y)
end

v1 = Vector.new(1, 2)
v2 = Vector.new(3, 4)
v3 = v1:add(v2)  -- v3 = {x=4, y=6}
```

### Metamethod Dispatch

| Operation | Metamethod |
|-----------|------------|
| `a + b` | `__add` |
| `a == b` | `__eq` |
| `t[k]` | `__index` / `__newindex` |
| `f()` | `__call` |
| `#t` | `__len` |
| GC finalization | `__gc` |

### Why Not Classes?

Lua chooses tables over classes:
- Simpler core (no inheritance implementation)
- Multiple inheritance via metamethods
- Prototypes (not classes) in Lua 5.1
- Freedom to implement class systems as libraries

---

## Performance Characteristics

### Strengths

| Feature | Performance |
|---------|-------------|
| Function calls | Fast via register VM |
| Table access | O(1) average via hash tables |
| String operations | Fast via interning |
| Arithmetic | Fast via Go's native int/float |

### Weaknesses

| Feature | Consideration |
|---------|---------------|
| GC pauses | Incremental mitigates, but not zero |
| Goroutine integration | Not transparent (no async I/O) |
| Reflection | Limited compared to native Go |
| Large tables | Hash collisions can occur |

---

## Testing Philosophy

### Test Categories

1. **Unit tests**: Per-package, testing interfaces
2. **Integration tests**: Cross-package, testing interactions
3. **Contract tests**: Against lua-master reference implementation
4. **GC stress tests**: Memory pressure scenarios

### Contract Testing

```bash
# Extract patterns from lua-master/testes/
lua-master/testes/
├── constructs.lua   # Control flow
├── literals.lua     # Literals and escape sequences
├── closure.lua      # Closures and upvalues
├── nextvar.lua      # Variables and tables
└── ...
```

These tests verify semantic compatibility with reference Lua.

---

## Future Considerations

### What's Missing

- Debug API (`debug.getlocal`, `debug.setlocal`, etc.)
- Full standard library (`io`, `os`, `math`, `string.format`)
- C extension API (not planned for Go version)

### Potential Enhancements

- WASM compilation target
- Async/await integration with Go
- Foreign Function Interface (FFI)
- JIT compilation for hot paths

### Won't Do

- Lua 5.1 compatibility mode (different API)
- LuaJIT-specific extensions
- Coroutine preemption (goroutine integration)

---

## Conclusion

go-lua aims to be a faithful Lua 5.5.1 implementation in Go. The design prioritizes:

1. **Correctness**: Full test coverage, contract testing against lua-master
2. **Simplicity**: Clean interfaces, minimal magic, readable code
3. **Pragmatism**: Go idioms where they improve the design, Lua semantics where they matter

The result is a scripting VM that's easy to integrate, extend, and reason about.
