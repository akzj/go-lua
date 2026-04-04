# go-lua

A Lua 5.5.1 virtual machine implementation in Go.

## Features

- **Parser & Lexer**: Full Lua 5.5.1 syntax support
- **Bytecode Compiler**: Compile Lua source to VM instructions
- **VM Executor**: Execute Lua bytecode with register-based VM
- **Coroutines**: Full Resume/Yield support for cooperative multitasking
- **Metatables**: Object-oriented programming with metamethods
- **Tables**: Arrays and dictionaries with length operator
- **Garbage Collection**: Incremental GC with generational support
- **Error Handling**: Protected calls and error propagation
- **C Function Integration**: Register Go functions callable from Lua

## Quick Start

### Installation

```bash
go get github.com/akzj/go-lua
```

### Basic Usage

```go
package main

import (
    "fmt"
    "github.com/akzj/go-lua/state"
)

func main() {
    L := state.New()
    defer L.Close()

    // Execute Lua code
    if err := L.DoString(`print("Hello, World!")`); err != nil {
        fmt.Println("Error:", err)
    }
}
```

### Working with the Stack

```go
L := state.New()
defer L.Close()

// Push values onto the stack
L.PushInteger(10)
L.PushString("hello")

// Check stack top
fmt.Println("Stack top:", L.Top()) // 2

// Get values from stack
val, ok := L.ToInteger(1)  // index 1 = first pushed value
fmt.Println("Integer:", val, ok) // 10, true

// Pop values
L.Pop()
fmt.Println("Stack top after pop:", L.Top()) // 1
```

### Global Variables

```go
L := state.New()
defer L.Close()

// Set a global variable
L.PushInteger(42)
L.SetGlobal("myNumber")

// Get a global variable
L.GetGlobal("myNumber")
val, _ := L.ToInteger(-1)
L.Pop()
fmt.Println("myNumber:", val) // 42
```

### Tables

```go
L := state.New()
defer L.Close()

// Create a table: t = {1, 2, 3}
L.CreateTable(0, 0)  // 0 array, 0 hash slots
L.PushInteger(1)
L.RawSetInt(-2, 1)   // t[1] = 1
L.PushInteger(2)
L.RawSetInt(-2, 2)   // t[2] = 2
L.PushInteger(3)
L.RawSetInt(-2, 3)   // t[3] = 3

// Get table length
fmt.Println("Table length:", L.Len(-1)) // 3

// Access table field
L.GetField(-1, 1)  // Get t[1]
val, _ := L.ToInteger(-1)
fmt.Println("t[1]:", val) // 1
L.Pop() // Pop returned value
```

### Functions

```go
L := state.New()
defer L.Close()

// Define a Lua function
L.DoString(`
    function add(a, b)
        return a + b
    end
`)

// Call the function
L.GetGlobal("add")
L.PushInteger(10)
L.PushInteger(20)
L.Call(2, 1)  // 2 args, 1 result

// Get result
result, _ := L.ToInteger(-1)
fmt.Println("add(10, 20) =", result) // 30
L.Pop()
```

### Metatables & Metamethods

```go
L := state.New()
defer L.Close()

// Create a table
L.CreateTable(0, 0)
tableIndex := L.Top()

// Create metatable
L.CreateTable(0, 1)

// Set __add metamethod
L.PushGoFunction(func(L state.LuaStateInterface) int {
    a, _ := L.ToInteger(1)
    b, _ := L.ToInteger(2)
    L.PushInteger(a + b)
    return 1
})
L.SetField(-2, "__add")

// Set metatable on table
L.SetMetatable(tableIndex)

// Now we can "add" tables (if table has numeric value)
// This requires more setup, but demonstrates the pattern
```

### Coroutines

```go
L := state.New()
defer L.Close()

// Create a coroutine
co := L.NewThread()
if co == nil {
    panic("failed to create coroutine")
}

// Define a generator function
L.DoString(`
    function generator()
        for i = 1, 5 do
            coroutine.yield(i)
        end
    end
`)

// Move function to coroutine
L.GetGlobal("generator")
state.Move(co, L, 1)
L.Pop()

// Resume coroutine multiple times
for i := 0; i < 6; i++ {
    status := co.Resume(co, 0)
    if status == state.LUA_YIELD {
        if co.Top() > 0 {
            val, _ := co.ToInteger(1)
            fmt.Printf("Yield %d: %d\n", i+1, val)
        }
        co.SetTop(0)
    } else if status == state.LUA_OK {
        fmt.Println("Coroutine finished")
        break
    }
}
```

### Error Handling

```go
L := state.New()
defer L.Close()

// Use protected call
L.GetGlobal("nonExistent")
err := L.PCall(0, 0, nil)
if err != nil {
    fmt.Println("Error caught:", err)
}

// Manual error with error()
L.DoString(`error("something went wrong")`) // Returns error
```

### Registering Go Functions

```go
L := state.New()
defer L.Close()

// Register a Go function callable from Lua
L.PushGoFunction(func(L state.LuaStateInterface) int {
    // Get arguments from stack
    a, _ := L.ToInteger(1)
    b, _ := L.ToInteger(2)
    
    // Return result
    L.PushInteger(a + b)
    return 1  // number of return values
})
L.SetGlobal("goAdd")

// Now call it from Lua
L.DoString(`print(goAdd(5, 3))`) // Prints: 8
```

## API Overview

### State Functions

| Function | Description |
|----------|-------------|
| `New()` | Create a new Lua state |
| `Close()` | Close and release a Lua state |
| `DoString(code)` | Execute Lua source code |
| `DoFile(filename)` | Execute Lua file |
| `NewThread()` | Create a new coroutine |

### Stack Operations

| Function | Description |
|----------|-------------|
| `GetTop()` | Get current stack top index |
| `SetTop(index)` | Set stack top |
| `PushValue(index)` | Push a copy of value at index |
| `Pop()` | Pop top value |
| `PushInteger(n)` | Push integer onto stack |
| `PushString(s)` | Push string onto stack |
| `ToInteger(index)` | Convert value at index to integer |
| `ToString(index)` | Convert value at index to string |
| `ToBoolean(index)` | Convert value at index to boolean |

### Global Variables

| Function | Description |
|----------|-------------|
| `GetGlobal(name)` | Push global variable onto stack |
| `SetGlobal(name)` | Pop and set global variable |

### Tables

| Function | Description |
|----------|-------------|
| `CreateTable(narr, nrec)` | Create new table |
| `GetField(index, key)` | Get table field |
| `SetField(index, key)` | Set table field |
| `RawGet(index, key)` | Raw table get |
| `RawSet(index, key, value)` | Raw table set |
| `RawGetInt(index, key)` | Get integer key |
| `RawSetInt(index, key, value)` | Set integer key |
| `Len(index)` | Get table/string length |
| `GetMetatable(index)` | Get metatable |
| `SetMetatable(index)` | Set metatable |

### Function Calls

| Function | Description |
|----------|-------------|
| `Call(nArgs, nResults)` | Call a function |
| `PCall(nArgs, nResults, errfunc)` | Protected call |
| `PushGoFunction(f)` | Push Go function as Lua function |

### Coroutines

| Function | Description |
|----------|-------------|
| `Resume(co, nArgs)` | Resume a coroutine |
| `Yield(nResults)` | Yield from coroutine |

### Type Checking

| Function | Description |
|----------|-------------|
| `Type(index)` | Get type of value at index |
| `IsNumber(index)` | Check if value is number |
| `IsString(index)` | Check if value is string |
| `IsFunction(index)` | Check if value is function |
| `IsTable(index)` | Check if value is table |
| `IsNil(index)` | Check if value is nil |
| `IsBoolean(index)` | Check if value is boolean |

## Project Structure

```
go-lua/
├── api/              # Public API packages
├── ast/              # Abstract syntax tree types
├── bytecode/         # Bytecode definitions
├── examples/         # Usage examples
├── gc/               # Garbage collector
├── integration/      # Integration tests
├── lex/              # Lexical analyzer
├── lib/              # Standard library bindings
├── mem/              # Memory allocator
├── opcodes/          # VM opcode definitions
├── parse/            # Lua parser
├── state/            # Lua state management
├── string/           # String interning
├── table/            # Table implementation
├── types/            # Core Lua type definitions
└── vm/               # Virtual machine executor
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific test
go test -v ./integration -run TestLuaMasterExecution
```

## Compatibility

- Targets Lua 5.5.1 specification
- Compatible with Lua 5.4 syntax (global const, goto)
- Go 1.26.1 or later

## Limitations

- Standard library (io, os, math, etc.) not yet implemented
- Debug API not yet implemented
- String formatting (`string.format`) not yet implemented

## License

MIT License - see LICENSE file for details.
