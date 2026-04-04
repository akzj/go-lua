# go-lua Design Documentation

This document describes the internal architecture and design decisions of the go-lua implementation.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        User Code                            │
│                  (Lua source / bytecode)                    │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                       Lexer (lex/)                          │
│         Tokenizes Lua source into token stream              │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                       Parser (parse/)                       │
│        Converts tokens into AST (lua-master compatible)     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Compiler (compile/)                      │
│         Transforms AST into bytecode instructions           │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   Virtual Machine (vm/)                     │
│                 Executes bytecode instructions              │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│              State Management (state/)                       │
│              Stack, call frames, globals                     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                Garbage Collector (gc/)                      │
│         Incremental + generational GC for memory           │
└─────────────────────────────────────────────────────────────┘
```

## Package Organization

### API Contracts

Each major subsystem exposes its interface through an `api/` subpackage:

| Package | Purpose | Public Interface |
|---------|---------|------------------|
| `types/api` | Core Lua types | `TValue`, type tags, constants |
| `lex/api` | Lexer interface | `TokenType`, lexer functions |
| `state/api` | State management | `LuaStateInterface`, `GlobalState` |
| `vm/api` | VM execution | `VMExecutor`, `VMFrameManager` |
| `gc/api` | Garbage collection | `GCCollector` |

This design:
- Separates interface from implementation
- Enables testing against interfaces
- Prevents circular dependencies
- Makes dependencies explicit

### Type System (types/)

#### TValue - Tagged Union

Lua values are represented as tagged unions:

```go
type TValue struct {
    value    interface{}  // Actual Go value
    gcObject interface{}  // GC-managed object reference
    extra    uintptr      // Extra info (variant bits)
}
```

The tag is encoded in the variant bits of `extra`, following Lua 5.5.1 conventions:

- `LUA_TNIL` (0): nil, empty, abstkey, notable
- `LUA_TBOOLEAN` (1): true, false
- `LUA_TNUMBER` (3): integer, float
- `LUA_TSTRING` (4): short string, long string
- `LUA_TTABLE` (5): table
- `LUA_TFUNCTION` (6): local closure, C function, C closure
- `LUA_TUSERDATA` (7): userdata
- `LUA_TTHREAD` (8): thread/coroutine

#### Why Tagged Union?

Lua's dynamic typing requires a single type that can hold any value. The tagged union pattern:
- Stores the actual value in `interface{}`
- Uses bit packing in `extra` for type info and variants
- Enables efficient type checking via bitwise operations

### Lexer (lex/)

The lexer implements Lua 5.5.1 tokenization:

```
Source Text → Tokens → Parser
```

Key components:
- **Scanner**: Reads source bytes, handles UTF-8
- **Keywords**: Lua keywords detected by identifier scan
- **Numbers**: Supports decimal, hex (0x), scientific notation
- **Strings**: Supports single/double quotes, long brackets `[[...]]`
- **Escape sequences**: `\n`, `\t`, `\\`, `\"`, `\xNN`, `\u{NNNN}`

#### Token Type Design

```go
type TokenType int

const (
    TOKEN_AND  TokenType = 200 + iota  // Keywords start at 200
    TOKEN_BREAK
    ...
    TOKEN_EQ     // Multi-char: ==
    TOKEN_CONCAT // Multi-char: ..
)
```

Single-char tokens use their ASCII values (e.g., `TOKEN_PLUS = '+'`), while keywords and multi-char operators use values > 255.

### Parser (parse/)

The parser builds an AST compatible with lua-master:

```go
type FuncState struct {
    f *prototype  // Function prototype (AST node)
    ...
}
```

#### Expression Types

- **ExpDesc**: Union type for expressions
  - `VK`: Constant value (nil, bool, number, string)
  - `VLOCAL`: Local variable
  - `VGLOBAL`: Global variable (upvalue handling)
  - `VINDEXED`: Table index access
  - `VCALL`: Function call result
  - `VVARARG`: Vararg expression

#### Instruction Generation

Expressions generate instructions into a buffer:
- Register allocation for temporary values
- Constant pool management
- Jump target patching for branches

### Compiler (bytecode/)

The compiler transforms AST into bytecode:

#### Instruction Format

Lua 5.5.1 uses 32-bit instructions with 3 address formats:

```
[opcode:6][A:8][B:9][C:9]   - ABC format
[opcode:6][A:8][Bx:18]      - ABx format
[opcode:6][A:8][sBx:18]     - AsBx format (signed)
[opcode:6][A:8][Ax:26]      - AAx format
```

#### Opcode Categories

| Category | Opcodes | Purpose |
|----------|---------|---------|
| Move | `MOVE`, `LOADNIL` | Register transfers |
| Arithmetic | `ADD`, `SUB`, `MUL`, `DIV` | Binary operations |
| Bitwise | `BAND`, `BOR`, `BXOR` | Bit manipulation |
| Comparison | `EQ`, `LT`, `LE` | Conditional jumps |
| Logical | `AND`, `OR`, `NOT` | Boolean operations |
| Strings | `CONCAT` | String concatenation |
| Calls | `CALL`, `TAILCALL` | Function invocation |
| Returns | `RETURN` | Function return |
| Loops | `FORLOOP`, `FORPREP` | Numeric for-loops |
| Tables | `SETTABLE`, `GETTABLE` | Table access |
| Closures | `CLOSURE`, `VARARG` | Function creation |

### Virtual Machine (vm/)

The VM executes bytecode with a register-based architecture:

```go
type VMExecutor interface {
    Execute(inst Instruction) bool
    Run() error
}
```

#### Stack Layout

```
┌─────────────────────────────────────────┐
│  CallInfo 1: [func][arg1][arg2]...     │
│  CallInfo 2: [func][local1][local2]... │
│  CallInfo 3: [func][local1]...         │
│  ...                                   │
│  Base (base_ci)                        │
└─────────────────────────────────────────┘
```

- Stack grows upward
- Each CallInfo tracks frame boundaries
- Virtual registers are stack slots

#### Execution Loop

```go
func (vm *Executor) Run() error {
    for {
        inst := vm.Fetch()
        if !vm.Execute(inst) {
            return vm.err  // Error or halt
        }
        if vm.pc >= len(vm.code) {
            return nil  // Normal exit
        }
    }
}
```

### State Management (state/)

The `LuaStateInterface` manages execution state:

```go
type LuaStateInterface interface {
    NewThread() LuaStateInterface
    Status() Status
    
    // Stack operations
    PushValue(idx int)
    Pop()
    Top() int
    SetTop(idx int)
    
    // Function calls
    Call(nArgs, nResults int)
    Resume() error
    Yield(nResults int) error
    
    // Table operations
    GetField(idx int, key string)
    SetField(idx int, key string)
    ...
}
```

#### Thread Model

- Main thread created via `New()`
- Coroutines via `NewThread()`
- All threads share `GlobalState` (registry, allocator, GC)
- `Resume`/`Yield` for coroutine control

#### Call Frames

CallInfo linked list tracks nested calls:
- `Func()`: Stack index of function
- `Top()`: One-past last register for this frame
- `Prev()`: Caller's frame

### Garbage Collector (gc/)

Implements Lua's incremental GC with generational support:

#### GC Phases

1. **Stop-the-World Pause**: Mark roots
2. **Incremental Propagation**: Traverse reachable objects
3. **Atomic Phase**: Handle weak tables, finalize
4. **Sweep**: Collect unreachable objects
5. **Finalization**: Call `__gc` metamethods

#### Object Colors

Objects are marked with colors:
- **White**: Newly allocated, not yet traced
- **Gray**: Traced but references not followed
- **Black**: Fully traced, references followed

#### Generational Mode

Minor GC collects young objects:
- Objects survive minor GC → promoted to old
- Old objects accumulate until major GC

### Memory Management (mem/)

Custom allocator integrates with GC:

```go
type Allocator interface {
    Alloc(size uint64) unsafe.Pointer
    Realloc(ptr unsafe.Pointer, oldSize, newSize uint64) unsafe.Pointer
    Free(ptr unsafe.Pointer, size uint64)
}
```

Key features:
- Allocation tracked for GC
- Realloc updates GC bookkeeping
- Finalizers registered via GC

## Design Patterns

### 1. Factory Functions over Constructors

```go
// In api package (interface)
var DefaultLuaState LuaStateInterface

// In internal package (implementation)
func init() {
    api.DefaultLuaState = newLuaState(nil)
}
```

This avoids circular imports while maintaining a clean public API.

### 2. Interface Segregation

Each subsystem exposes minimal interfaces:
- `vm/api`: Only `Execute()` and `Run()`
- `state/api`: Only `LuaStateInterface`
- `gc/api`: Only `GCCollector`

Internal details stay internal.

### 3. Bit Packing for Type Tags

Lua's value representation uses bit packing:

```
[variant:4][type:4][collectable:1][reserved:3]
```

This allows:
- Fast type checking via bit masks
- Compact storage (2 bytes per tag)
- Variant info in type field

### 4. Double Dispatch for Operations

Table operations use double dispatch:

```
VM → Table → GC
```

The table layer delegates to GC for object marking, avoiding tight coupling.

## Performance Considerations

### Register-Based VM

Lua 5.x uses registers instead of stack:
- Fewer LOAD/STORE operations
- Better register allocation
- Matches AST expression evaluation order

### String Interning

Short strings are interned:
- Fast equality comparison
- Reduced memory allocation
- Hash table lookup

### Incremental GC

GC runs in small increments:
- No long stop-the-world pauses
- Predictable pause times
- Background collection

### Instruction Caching

Hot code paths:
- Tight execution loop
- Minimal bounds checking
- Inline fast paths

## Reference Implementation

This implementation references:
- `lua-master/`: Reference C implementation (Lua 5.5.1 development)
- `testes/`: Reference test suite for compatibility

Key differences from C implementation:
- Go idioms (goroutines, channels, interfaces)
- No C extension API
- Simplified memory model
