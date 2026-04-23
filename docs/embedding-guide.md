# Embedding go-lua in Your Application

A practical guide for using go-lua as an embedded Lua 5.5 scripting engine in Go programs.

## Installation

```bash
go get github.com/akzj/go-lua@latest
```

## Basic Usage

```go
package main

import (
    "log"

    lua "github.com/akzj/go-lua/pkg/lua"
)

func main() {
    L := lua.NewState()
    defer L.Close()

    if err := L.DoString(`print("Hello from Lua!")`); err != nil {
        log.Fatal(err)
    }
}
```

`NewState()` creates a Lua state with all standard libraries loaded (string, table,
math, io, os, coroutine, debug, utf8, package). Call `Close()` when done to release
internal resources.

## Registering Go Functions

Any Go function with signature `func(L *lua.State) int` can be called from Lua.
The return value is the number of results pushed onto the stack.

```go
// A function that adds two integers.
add := func(L *lua.State) int {
    a := L.CheckInteger(1) // first argument
    b := L.CheckInteger(2) // second argument
    L.PushInteger(a + b)
    return 1 // one return value
}
L.PushFunction(add)
L.SetGlobal("add")

// Now callable from Lua: add(1, 2) → 3
```

### Multiple Return Values

```go
divmod := func(L *lua.State) int {
    a := L.CheckInteger(1)
    b := L.CheckInteger(2)
    L.PushInteger(a / b) // quotient
    L.PushInteger(a % b) // remainder
    return 2             // two return values
}
```

### Registering a Module

Use `SetFuncs` to register multiple functions at once:

```go
L.NewTable()
L.SetFuncs(map[string]lua.Function{
    "upper": func(L *lua.State) int {
        s := L.CheckString(1)
        L.PushString(strings.ToUpper(s))
        return 1
    },
    "lower": func(L *lua.State) int {
        s := L.CheckString(1)
        L.PushString(strings.ToLower(s))
        return 1
    },
}, 0)
L.SetGlobal("mystr")

// Lua: mystr.upper("hello") → "HELLO"
```

## Passing Data Between Go and Lua

### Go → Lua (Push)

```go
// Scalars
L.PushString("hello")
L.SetGlobal("greeting")

L.PushInteger(42)
L.SetGlobal("answer")

L.PushBoolean(true)
L.SetGlobal("debug_mode")

// Tables (Lua's primary data structure)
L.NewTable()
L.PushInteger(8080)
L.SetField(-2, "port")
L.PushString("localhost")
L.SetField(-2, "host")
L.SetGlobal("config")
// Lua: config.host → "localhost", config.port → 8080
```

### Lua → Go (Read)

```go
L.DoString(`result = "computed value"`)

L.GetGlobal("result")
if L.IsString(-1) {
    val, _ := L.ToString(-1)
    fmt.Println(val) // "computed value"
}
L.Pop(1) // always clean up the stack

// Reading table fields
L.DoString(`settings = { width = 1920, height = 1080 }`)
L.GetGlobal("settings")
L.GetField(-1, "width")
w, _ := L.ToInteger(-1)
L.Pop(2) // pop value + table
```

## Error Handling

### Simple: DoString / DoFile

```go
if err := L.DoString(code); err != nil {
    log.Printf("Lua error: %v", err)
}

if err := L.DoFile("script.lua"); err != nil {
    log.Printf("file error: %v", err)
}
```

### Advanced: Load + PCall

For finer control, load a chunk first, then call it in protected mode:

```go
status := L.Load(`return 6 * 7`, "=example", "t")
if status != lua.OK {
    msg, _ := L.ToString(-1)
    log.Printf("compile error: %s", msg)
    L.Pop(1)
    return
}

// Call with 0 args, 1 result, no error handler
status = L.PCall(0, 1, 0)
if status != lua.OK {
    msg, _ := L.ToString(-1)
    log.Printf("runtime error: %s", msg)
    L.Pop(1)
    return
}

result, _ := L.ToInteger(-1)
L.Pop(1)
fmt.Println(result) // 42
```

### Status Codes

| Constant | Value | Meaning |
|----------|-------|---------|
| `lua.OK` | 0 | Success |
| `lua.Yield` | 1 | Coroutine yielded |
| `lua.ErrRun` | 2 | Runtime error |
| `lua.ErrSyntax` | 3 | Syntax error |
| `lua.ErrMem` | 4 | Memory allocation error |
| `lua.ErrErr` | 5 | Error in error handler |
| `lua.ErrFile` | 6 | File I/O error |

## Coroutine-Based Interaction

The most powerful embedding pattern: Lua scripts call functions that **yield**
back to Go, and Go resumes with a result. This lets Lua code look synchronous
while Go handles async I/O.

```go
// Register a function that yields to Go
askUser := func(L *lua.State) int {
    // The prompt string (argument 1) is already on the stack.
    return L.Yield(1) // yield 1 value back to Go
}
L.PushFunction(askUser)
L.SetGlobal("ask")

// Lua script uses it like a normal blocking function
L.DoString(`
    function chat()
        local name = ask("What is your name?")
        local age  = ask("How old are you?")
        return "Hello, " .. name .. "! You are " .. age .. "."
    end
`)

// Drive the coroutine from Go
thread := L.NewThread()
thread.GetGlobal("chat")

nArgs := 0
for {
    status, nresults := thread.Resume(L, nArgs)

    if status == lua.Yield {
        // Lua yielded a prompt — read it
        prompt, _ := thread.ToString(-1)
        thread.Pop(nresults)

        // Get user input however you like (stdin, HTTP, WebSocket...)
        answer := readLine(prompt)

        // Push the answer and resume
        thread.PushString(answer)
        nArgs = 1
        continue
    }

    if status == lua.OK {
        // Coroutine finished — read the return value
        result, _ := thread.ToString(-1)
        fmt.Println(result)
        break
    }

    // Error
    msg, _ := thread.ToString(-1)
    log.Printf("coroutine error: %s", msg)
    break
}
L.Pop(1) // pop the thread
```

## Userdata: Wrapping Go Objects

Userdata lets you pass Go objects into Lua with type safety and metatables.

```go
// Create userdata wrapping a Go map
L.NewUserdata(0, 0) // 0 bytes Lua-side, 0 user values
L.SetUserdataValue(-1, map[string]int{"x": 10, "y": 20})

// Read it back in a Go callback
val := L.UserdataValue(-1)
m := val.(map[string]int)
fmt.Println(m["x"]) // 10
```

### With Metatables

```go
// Create a named metatable (stored in the registry)
L.NewMetatable("Point")

// Add a __tostring metamethod
L.PushFunction(func(L *lua.State) int {
    p := L.UserdataValue(1).([]float64)
    L.PushString(fmt.Sprintf("(%g, %g)", p[0], p[1]))
    return 1
})
L.SetField(-2, "__tostring")
L.Pop(1) // pop metatable

// Create a Point userdata
L.NewUserdata(0, 0)
L.SetUserdataValue(-1, []float64{3.0, 4.0})
L.GetField(lua.RegistryIndex, "Point") // retrieve metatable
L.SetMetatable(-2)                     // attach it
L.SetGlobal("pt")

L.DoString(`print(tostring(pt))`) // prints: (3, 4)
```

## Sandboxing

Use `NewBareState()` to create a state without standard libraries, then
register only the functions you want to expose:

```go
L := lua.NewBareState()
defer L.Close()

// Register only safe functions — no io, os, or debug
L.PushFunction(safePrint)
L.SetGlobal("print")

L.PushFunction(typeFunc)
L.SetGlobal("type")

// io.open, os.execute, etc. are NOT available
```

### Instruction Limits

Use a count hook to limit CPU usage:

```go
L.SetHook(func(L *lua.State, event int, line int) {
    if event == lua.HookEventCount {
        // Raise an error to stop execution
        L.Errorf("instruction limit exceeded")
    }
}, lua.MaskCount, 100000) // fire every 100,000 instructions
```

### Memory Monitoring

```go
// Check current memory usage
bytes := L.GCTotalBytes()
fmt.Printf("Lua memory: %d bytes\n", bytes)

// Force a garbage collection cycle
L.GCCollect()
```

## Concurrency

Each `lua.State` is **single-threaded** — never share a State across goroutines.
For concurrency, create one State per goroutine:

```go
var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        L := lua.NewState()
        defer L.Close()
        L.DoString(fmt.Sprintf(`result = %d * %d`, id, id))
    }(i)
}
wg.Wait()
```

## The Stack

go-lua uses a virtual stack to pass values between Go and Lua (matching
the C Lua API convention). Understanding the stack is essential:

- **Positive indices** count from the bottom: 1 = first element
- **Negative indices** count from the top: -1 = top element
- **Push** functions add values to the top
- **Pop(n)** removes n values from the top
- **GetTop()** returns the current stack size

```
Stack after PushInteger(10); PushString("hi"); PushBoolean(true):

  Index:  1       2       3       ← positive (from bottom)
         [10]   ["hi"]  [true]
  Index: -3      -2      -1      ← negative (from top)
```

## Storing Lua Values in Go (References)

Use the registry to store Lua values that you need to access later from Go:

```go
// Store a value
L.PushString("important data")
ref := L.Ref(lua.RegistryIndex)

// ... later, retrieve it
L.RawGetI(lua.RegistryIndex, int64(ref))
val, _ := L.ToString(-1)
L.Pop(1)

// Free when no longer needed
L.Unref(lua.RegistryIndex, ref)
```

## Debug Hooks

Monitor Lua execution with debug hooks:

```go
L.SetHook(func(L *lua.State, event int, line int) {
    switch event {
    case lua.HookEventCall:
        fmt.Println("function called")
    case lua.HookEventReturn:
        fmt.Println("function returned")
    case lua.HookEventLine:
        fmt.Printf("executing line %d\n", line)
    }
}, lua.MaskCall|lua.MaskRet|lua.MaskLine, 0)
```

Use `GetStack` and `GetInfo` inside hooks for deeper introspection:

```go
L.SetHook(func(L *lua.State, event int, line int) {
    if ar, ok := L.GetStack(0); ok {
        L.GetInfo("nSl", ar)
        fmt.Printf("%s:%d in %s\n", ar.ShortSrc, ar.CurrentLine, ar.Name)
    }
}, lua.MaskLine, 0)
```

## API Quick Reference

| Category | Key Methods |
|----------|------------|
| **State** | `NewState()`, `NewBareState()`, `Close()` |
| **Execute** | `DoString()`, `DoFile()`, `LoadFile()`, `Load()`, `PCall()`, `Call()` |
| **Stack Push** | `PushNil()`, `PushBoolean()`, `PushInteger()`, `PushNumber()`, `PushString()`, `PushFunction()`, `PushClosure()` |
| **Stack Read** | `ToBoolean()`, `ToInteger()`, `ToNumber()`, `ToString()`, `Type()`, `TypeName()` |
| **Stack Check** | `CheckString()`, `CheckInteger()`, `CheckNumber()`, `CheckType()` |
| **Stack Ops** | `Pop()`, `GetTop()`, `SetTop()`, `PushValue()`, `Copy()`, `Remove()`, `Rotate()` |
| **Tables** | `NewTable()`, `GetField()`, `SetField()`, `GetI()`, `SetI()`, `GetTable()`, `SetTable()`, `RawGet()`, `RawSet()`, `Next()`, `Len()` |
| **Globals** | `GetGlobal()`, `SetGlobal()` |
| **Metatables** | `NewMetatable()`, `GetMetatable()`, `SetMetatable()`, `SetFuncs()` |
| **Coroutines** | `NewThread()`, `Resume()`, `Yield()`, `YieldK()`, `Status()`, `IsYieldable()`, `XMove()` |
| **Userdata** | `NewUserdata()`, `UserdataValue()`, `SetUserdataValue()`, `GetIUserValue()`, `SetIUserValue()` |
| **References** | `Ref()`, `Unref()`, `RawGetI()` |
| **Debug** | `SetHook()`, `GetHook()`, `GetStack()`, `GetInfo()`, `GetLocal()`, `SetLocal()` |
| **GC** | `GCCollect()`, `GCTotalBytes()` |
| **Compare** | `Compare()`, `RawEqual()` |

## Further Reading

- [Lua 5.5 Reference Manual](https://www.lua.org/manual/5.5/)
- [pkg.go.dev documentation](https://pkg.go.dev/github.com/akzj/go-lua/pkg/lua) — full API docs with runnable examples
- `pkg/lua/example_test.go` — 18 runnable examples covering all major APIs
