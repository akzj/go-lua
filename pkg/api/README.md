# Go Lua API Package

This package provides the public Lua API for the Go Lua implementation, closely following the Lua 5.4 C API conventions.

## Overview

The `api` package exposes the Lua state and all standard Lua operations in a Go-friendly interface. It wraps the internal VM implementation and provides familiar functions for Lua programmers.

## Basic Usage

### Creating a State

```go
import "github.com/akzj/go-lua/pkg/api"

L := api.NewState()
defer L.Close()
```

### Executing Lua Code

```go
// Execute a string
err := L.DoString("print('Hello, World!')", "chunk")
if err != nil {
    log.Fatal(err)
}

// Execute a file
err = L.DoFile("script.lua")
if err != nil {
    log.Fatal(err)
}
```

### Stack Operations

```go
// Push values
L.PushNil()
L.PushBoolean(true)
L.PushNumber(42)
L.PushString("hello")
L.PushFunction(func(L *api.State) int {
    // Go function implementation
    return 0
})

// Get stack top
top := L.GetTop()

// Set stack top (pop or extend)
L.SetTop(0)  // Clear stack

// Pop values
L.Pop(2)  // Remove top 2 values
```

### Accessing Values

```go
// By positive index (1-based from bottom)
L.PushNumber(42)
n, ok := L.ToNumber(1)

// By negative index (-1 = top)
s, ok := L.ToString(-1)

// Type checking
if L.IsNil(1) {
    // Value is nil
}
if L.IsFunction(-1) {
    // Value is a function
}
if L.IsTruthy(1) {
    // Value is truthy (not nil or false)
}
```

### Registering Go Functions

```go
// Register as global
L.Register("myfunc", func(L *api.State) int {
    // Get arguments
    arg1, _ := L.ToNumber(1)
    arg2, _ := L.ToNumber(2)
    
    // Push result
    L.PushNumber(arg1 + arg2)
    
    // Return number of results
    return 1
})

// Call from Lua
L.DoString("result = myfunc(10, 20)", "")
```

### Tables

```go
// Create table
L.NewTable()

// Set fields
L.PushString("value")
L.SetField(-2, "key")  // table["key"] = "value"

// Set integer indices
L.PushNumber(42)
L.SetI(-2, 1)  // table[1] = 42

// Get fields
L.GetField(-1, "key")  // Push table["key"]

// Iterate
L.PushNil()  // First key
for L.Next(-2) {
    // Key at -2, value at -1
    L.Pop(1)  // Remove value, keep key
}
```

### Error Handling

```go
// Protected call
err := L.PCall(0, 1, 0)  // 0 args, 1 result, no message handler
if err != nil {
    if luaErr, ok := err.(*api.LuaError); ok {
        log.Printf("Lua error: %s", luaErr.Message)
    }
}

// Raise error from Go
L.PushString("something went wrong")
L.Error()  // Raises Lua error

// Or with formatting
L.Errorf("Error: %d", code)
```

## C API Mapping

| Go API | C API | Description |
|--------|-------|-------------|
| `NewState()` | `luaL_newstate()` | Create new state |
| `Close()` | `lua_close()` | Close state |
| `PushNil()` | `lua_pushnil()` | Push nil |
| `PushBoolean(b)` | `lua_pushboolean()` | Push boolean |
| `PushNumber(n)` | `lua_pushnumber()` | Push number |
| `PushString(s)` | `lua_pushstring()` | Push string |
| `PushFunction(fn)` | `lua_pushfunction()` | Push Go function |
| `GetTop()` | `lua_gettop()` | Get stack top |
| `SetTop(idx)` | `lua_settop()` | Set stack top |
| `Pop(n)` | `lua_pop()` | Pop n values |
| `ToNumber(idx)` | `lua_tonumber()` | Convert to number |
| `ToString(idx)` | `lua_tostring()` | Convert to string |
| `ToBoolean(idx)` | `lua_toboolean()` | Convert to boolean |
| `Type(idx)` | `lua_type()` | Get type |
| `TypeName(idx)` | `lua_typename()` | Get type name |
| `IsNil(idx)` | `lua_isnil()` | Check if nil |
| `IsBoolean(idx)` | `lua_isboolean()` | Check if boolean |
| `IsNumber(idx)` | `lua_isnumber()` | Check if number |
| `IsString(idx)` | `lua_isstring()` | Check if string |
| `IsFunction(idx)` | `lua_isfunction()` | Check if function |
| `IsTable(idx)` | `lua_istable()` | Check if table |
| `Len(idx)` | `lua_len()` | Get length |
| `Copy(from, to)` | `lua_copy()` | Copy value |
| `Call(nargs, nresults)` | `lua_call()` | Call function |
| `PCall(nargs, nresults, msgh)` | `lua_pcall()` | Protected call |
| `LoadString(code, name)` | `lua_load()` | Load code from string |
| `LoadFile(filename)` | `luaL_loadfile()` | Load code from file |
| `DoString(code, name)` | `luaL_dostring()` | Load and execute string |
| `DoFile(filename)` | `luaL_dofile()` | Load and execute file |
| `Register(name, fn)` | `lua_register()` | Register function |
| `SetGlobal(name)` | `lua_setglobal()` | Set global variable |
| `GetGlobal(name)` | `lua_getglobal()` | Get global variable |
| `SetField(idx, key)` | `lua_setfield()` | Set table field |
| `GetField(idx, key)` | `lua_getfield()` | Get table field |
| `SetI(idx, i)` | `lua_seti()` | Set integer index |
| `GetI(idx, i)` | `lua_geti()` | Get integer index |
| `RawSet(idx)` | `lua_rawset()` | Raw set (no metamethods) |
| `RawGet(idx)` | `lua_rawget()` | Raw get (no metamethods) |
| `RawSetI(idx, i)` | `lua_rawseti()` | Raw set integer |
| `RawGetI(idx, i)` | `lua_rawgeti()` | Raw get integer |
| `NewTable()` | `lua_newtable()` | Create new table |
| `CreateTable(narr, nrec)` | `lua_createtable()` | Create table with size |
| `Next(idx)` | `lua_next()` | Iterate table |
| `Error()` | `lua_error()` | Raise error |
| `AtPanic(fn)` | `lua_atpanic()` | Set panic function |
| `Version()` | `lua_version()` | Get version string |
| `GcControl(what, data)` | `lua_gc()` | Control GC |
| `Status()` | `lua_status()` | Get thread status |
| `Resume(from, nargs)` | `lua_resume()` | Resume coroutine |
| `Yield(nresults)` | `lua_yield()` | Yield coroutine |
| `IsYieldable()` | `lua_isyieldable()` | Check if can yield |
| `XMove(to, n)` | `lua_xmove()` | Move between states |

## Stack Index Conventions

Stack indices follow Lua conventions:

- **Positive indices**: Count from the bottom, 1-based
  - Index 1 = first element (bottom)
  - Index 2 = second element
  - etc.

- **Negative indices**: Count from the top, -1-based
  - Index -1 = top element
  - Index -2 = second from top
  - etc.

- **Index 0**: Invalid in Lua, treated as base

Example:
```
Stack: [A, B, C, D]
       1  2  3  4   (positive)
      -4 -3 -2 -1   (negative)

L.ToNumber(2)   // Returns B
L.ToNumber(-2)  // Returns C
```

## Error Types

### LuaError

All Lua runtime errors are returned as `*LuaError`:

```go
type LuaError struct {
    Message string
    Stack   []string  // Lua stack trace
    GoStack string    // Go stack trace (for debugging)
}

func (e *LuaError) Error() string
```

### Helper Functions

```go
// Create specific error types
api.SyntaxError(message, source, line)
api.RuntimeError(message)
api.FileError(filename, underlyingError)
```

## Threading

**Lua states are NOT thread-safe.** Each goroutine should have its own state:

```go
// WRONG: Sharing state between goroutines
L := api.NewState()
go func() { L.DoString("...", "") }()  // Race condition!
go func() { L.DoString("...", "") }()  // Race condition!

// CORRECT: One state per goroutine
go func() {
    L := api.NewState()
    defer L.Close()
    L.DoString("...", "")
}()
```

For communication between goroutines, use Go channels, not shared Lua states.

## Memory Management

Always call `Close()` when done with a state:

```go
L := api.NewState()
defer L.Close()

// ... use L ...
```

This ensures:
- Stack is cleared
- References are released
- Resources are freed

## Examples

### Complete Example

```go
package main

import (
    "fmt"
    "log"
    "github.com/akzj/go-lua/pkg/api"
)

func main() {
    L := api.NewState()
    defer L.Close()

    // Register a Go function
    L.Register("add", func(L *api.State) int {
        a, _ := L.ToNumber(1)
        b, _ := L.ToNumber(2)
        L.PushNumber(a + b)
        return 1
    })

    // Execute Lua code
    err := L.DoString(`
        result = add(10, 20)
        print("Result:", result)
    `, "main")
    
    if err != nil {
        log.Fatal(err)
    }

    // Get result
    L.GetGlobal("result")
    result, _ := L.ToNumber(-1)
    fmt.Printf("Final result: %f\n", result)
}
```

### Error Handling Example

```go
L := api.NewState()
defer L.Close()

err := L.DoString("error('Something went wrong!')", "test")
if err != nil {
    if luaErr, ok := err.(*api.LuaError); ok {
        fmt.Printf("Lua error: %s\n", luaErr.Message)
        fmt.Printf("Stack trace:\n%s\n", luaErr.Stack)
    } else {
        fmt.Printf("Error: %v\n", err)
    }
}
```

### Table Manipulation Example

```go
L := api.NewState()
defer L.Close()

// Create a table
L.NewTable()

// Add some values
L.PushString("hello")
L.SetField(-2, "greeting")

L.PushNumber(42)
L.SetI(-2, 1)

// Set as global
L.SetGlobal("mytable")

// Access from Lua
L.DoString(`
    print(mytable.greeting)  -- "hello"
    print(mytable[1])        -- 42
`, "")
```

## Limitations

This implementation currently has some limitations:

1. **Parser**: The Lua parser is a skeleton. Only simple `return <value>` statements are supported via `LoadString`/`DoString`.

2. **Coroutines**: Basic support via `Resume`/`Yield`, but full coroutine functionality is limited.

3. **Garbage Collection**: GC control via `GcControl` is stubbed.

4. **Upvalues**: Limited support for upvalue operations.

5. **Debug Library**: Not implemented.

## License

Same license as the main Go Lua project.