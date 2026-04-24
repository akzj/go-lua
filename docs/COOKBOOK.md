# go-lua Cookbook

Task-driven recipes for embedding go-lua in Go programs.
Find what you want to do → copy the code.

**Import**: All recipes assume `import "github.com/akzj/go-lua/pkg/lua"`.

---

## Table of Contents

1.  [Hello World](#1-hello-world)
2.  [Execute a Lua File](#2-execute-a-lua-file)
3.  [Register a Go Function](#3-register-a-go-function)
4.  [Variadic Go Function](#4-variadic-go-function)
5.  [Multiple Return Values](#5-multiple-return-values)
6.  [Optional Parameters](#6-optional-parameters)
7.  [Create a Module (Table of Functions)](#7-create-a-module-table-of-functions)
8.  [Create a Table from Go](#8-create-a-table-from-go)
9.  [Read a Lua Table from Go](#9-read-a-lua-table-from-go)
10. [Iterate a Table (pairs)](#10-iterate-a-table-pairs)
11. [Iterate an Array (ipairs)](#11-iterate-an-array-ipairs)
12. [Call a Lua Function from Go](#12-call-a-lua-function-from-go)
13. [Protected Call with Error Handling](#13-protected-call-with-error-handling)
14. [Userdata — Wrap a Go Struct](#14-userdata--wrap-a-go-struct)
15. [Userdata with Metatable (OOP)](#15-userdata-with-metatable-oop)
16. [Closures with Upvalues](#16-closures-with-upvalues)
17. [Coroutines — Drive from Go](#17-coroutines--drive-from-go)
18. [Yield from Go (Async Pattern)](#18-yield-from-go-async-pattern)
19. [Registry References](#19-registry-references)
20. [Sandboxing](#20-sandboxing)
21. [Debug Hooks](#21-debug-hooks)
22. [Execution Timeout](#22-execution-timeout)
23. [Require a Go Module](#23-require-a-go-module)
24. [Error Handling in Go Functions](#24-error-handling-in-go-functions)
25. [User Values on Userdata](#25-user-values-on-userdata)
26. [Push Any Go Value to Lua](#26-push-any-go-value-to-lua)
27. [Read Lua Values to Go (ToAny / ToStruct)](#27-read-lua-values-to-go-toany--tostruct)
28. [Convenience Table Access](#28-convenience-table-access)
29. [Safe Table Iteration (ForEach)](#29-safe-table-iteration-foreach)
30. [Auto-Bind Any Go Function (PushGoFunc)](#30-auto-bind-any-go-function-pushgofunc)
31. [Generic Wrappers (Type-Safe, No Reflection)](#31-generic-wrappers-type-safe-no-reflection)
32. [Sandbox — Run Untrusted Code](#32-sandbox--run-untrusted-code)
33. [CPU Instruction Limits](#33-cpu-instruction-limits)
34. [Context Cancellation / Timeout](#34-context-cancellation--timeout)
35. [Virtual Filesystem (embed.FS)](#35-virtual-filesystem-embedfs)
36. [Global Module Registry](#36-global-module-registry)
37. [Module Interface Pattern](#37-module-interface-pattern)
38. [State Pool for Concurrent Requests](#38-state-pool-for-concurrent-requests)
39. [Async Task Executor](#39-async-task-executor)
40. [Channels — Go ↔ Lua Communication](#40-channels--go--lua-communication)
41. [JSON — Encode and Decode](#41-json--encode-and-decode)
42. [HTTP Requests from Lua](#42-http-requests-from-lua)
43. [Async/Await with Futures](#43-asyncawait-with-futures)
44. [CallSafe and CallRef — Protected Calls](#44-callsafe-and-callref--protected-calls)

---

## 1. Hello World

Create a Lua state, run code, close it.

```go
L := lua.NewState()
defer L.Close()

err := L.DoString(`print("hello from Lua")`)
if err != nil {
    log.Fatal(err)
}
```

---

## 2. Execute a Lua File

Load and run a `.lua` file from disk.

```go
L := lua.NewState()
defer L.Close()

err := L.DoFile("script.lua")
if err != nil {
    log.Fatal(err)
}
```

For finer control (separate load from execute):

```go
status := L.LoadFile("script.lua", "t")  // "t" = text mode only
if status != lua.OK {
    msg, _ := L.ToString(-1)
    log.Fatalf("load error: %s", msg)
}
status = L.PCall(0, lua.MultiRet, 0)
if status != lua.OK {
    msg, _ := L.ToString(-1)
    log.Fatalf("runtime error: %s", msg)
}
```

---

## 3. Register a Go Function

Expose a Go function so Lua can call it by name.

```go
add := func(L *lua.State) int {
    a := L.CheckInteger(1)  // arg 1 (must be integer)
    b := L.CheckInteger(2)  // arg 2
    L.PushInteger(a + b)
    return 1  // number of return values
}

L.PushFunction(add)
L.SetGlobal("add")
```

**Lua side:**
```lua
print(add(40, 2))  --> 42
```

---

## 4. Variadic Go Function

Handle any number of arguments (like `print`).

```go
myPrint := func(L *lua.State) int {
    n := L.GetTop()  // number of arguments
    for i := 1; i <= n; i++ {
        if i > 1 {
            fmt.Print("\t")
        }
        fmt.Print(L.TolString(i))  // converts any value to string
    }
    fmt.Println()
    return 0
}

L.PushFunction(myPrint)
L.SetGlobal("myprint")
```

> `TolString` converts any Lua value to a string (like Lua's `tostring()`).
> `ToString` only works on actual string values.

---

## 5. Multiple Return Values

Return more than one value to Lua.

```go
divmod := func(L *lua.State) int {
    a := L.CheckInteger(1)
    b := L.CheckInteger(2)
    L.PushInteger(a / b)  // quotient
    L.PushInteger(a % b)  // remainder
    return 2  // two return values
}

L.PushFunction(divmod)
L.SetGlobal("divmod")
```

**Lua side:**
```lua
local q, r = divmod(17, 5)  --> q=3, r=2
```

---

## 6. Optional Parameters

Use `OptString` / `OptInteger` for arguments with defaults.

```go
greet := func(L *lua.State) int {
    name := L.OptString(1, "World")   // default "World"
    times := L.OptInteger(2, 1)       // default 1
    for i := int64(0); i < times; i++ {
        L.PushString(fmt.Sprintf("Hello, %s!", name))
    }
    return int(times)
}

L.PushFunction(greet)
L.SetGlobal("greet")
```

**Lua side:**
```lua
greet()            --> "Hello, World!"
greet("Alice")     --> "Hello, Alice!"
greet("Bob", 3)    --> "Hello, Bob!" (×3)
```

---

## 7. Create a Module (Table of Functions)

Group related Go functions into a Lua table.

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
```

**Lua side:**
```lua
print(mystr.upper("hello"))  --> "HELLO"
print(mystr.lower("WORLD"))  --> "world"
```

---

## 8. Create a Table from Go

Build a Lua table field-by-field from Go.

```go
L.NewTable()

L.PushString("localhost")
L.SetField(-2, "host")

L.PushInteger(8080)
L.SetField(-2, "port")

L.PushBoolean(true)
L.SetField(-2, "debug")

L.SetGlobal("config")
```

**Lua side:**
```lua
print(config.host)   --> "localhost"
print(config.port)   --> 8080
print(config.debug)  --> true
```

---

## 9. Read a Lua Table from Go

Extract values from a Lua table into Go variables.

```go
L.DoString(`settings = { width = 1920, height = 1080, debug = true }`)

L.GetGlobal("settings")

L.GetField(-1, "width")
w, _ := L.ToInteger(-1)
L.Pop(1)

L.GetField(-1, "height")
h, _ := L.ToInteger(-1)
L.Pop(1)

L.GetField(-1, "debug")
debug := L.ToBoolean(-1)
L.Pop(2)  // pop debug + settings table

fmt.Printf("%dx%d debug=%v\n", w, h, debug)
```

---

## 10. Iterate a Table (pairs)

Walk all key-value pairs in a Lua table.

```go
L.GetGlobal("mytable")
L.PushNil()  // first key
for L.Next(-2) {
    // key at index -2, value at index -1
    key, _ := L.ToString(-2)
    val := L.TolString(-1)
    fmt.Printf("%s = %s\n", key, val)
    L.Pop(1)  // pop value, keep key for next iteration
}
L.Pop(1)  // pop table
```

> **Warning**: Do not call `ToString` on the key if it might be a number —
> `ToString` modifies the stack value. Use `TolString` or check the type first.

---

## 11. Iterate an Array (ipairs)

Walk integer-indexed entries 1..N.

```go
L.GetGlobal("myarray")
length := L.LenI(-1)  // raw length (#myarray)
for i := int64(1); i <= length; i++ {
    L.GetI(-1, i)
    val, _ := L.ToString(-1)
    fmt.Printf("[%d] = %s\n", i, val)
    L.Pop(1)
}
L.Pop(1)  // pop table
```

---

## 12. Call a Lua Function from Go

Call a Lua-defined function and get its return value.

```go
L.DoString(`function greet(name) return "Hello, " .. name .. "!" end`)

L.GetGlobal("greet")       // push the function
L.PushString("Alice")      // push argument 1
status := L.PCall(1, 1, 0) // 1 arg, 1 result, no error handler
if status != lua.OK {
    msg, _ := L.ToString(-1)
    log.Fatal("error: ", msg)
}

result, _ := L.ToString(-1)
fmt.Println(result)  // "Hello, Alice!"
L.Pop(1)
```

---

## 13. Protected Call with Error Handling

Separate loading from execution for better error diagnostics.

```go
// Load without executing
status := L.Load(`return 6 * 7`, "=calc", "t")
if status != lua.OK {
    msg, _ := L.ToString(-1)
    log.Fatal("compile error: ", msg)
}

// Execute in protected mode
status = L.PCall(0, 1, 0)
if status != lua.OK {
    msg, _ := L.ToString(-1)
    fmt.Println("runtime error:", msg)
    L.Pop(1)
} else {
    result, _ := L.ToInteger(-1)
    fmt.Println(result)  // 42
    L.Pop(1)
}
```

**Status codes**: `lua.OK` (0), `lua.ErrRun` (2), `lua.ErrSyntax` (3), `lua.ErrMem` (4), `lua.ErrErr` (5), `lua.ErrFile` (6).

---

## 14. Userdata — Wrap a Go Struct

Attach any Go value to a Lua userdata.

```go
type Point struct{ X, Y int }

// Create userdata and attach Go value
L.NewUserdata(0, 0)
L.SetUserdataValue(-1, &Point{X: 10, Y: 20})
L.SetGlobal("pt")

// Later, read it back
L.GetGlobal("pt")
val := L.UserdataValue(-1)
p := val.(*Point)
fmt.Printf("x=%d y=%d\n", p.X, p.Y)
L.Pop(1)
```

---

## 15. Userdata with Metatable (OOP)

Full OOP pattern: metatable with methods, constructor, `__tostring`.

```go
type Point struct{ X, Y int }

// Step 1: Create the metatable (once, at setup time)
L.NewMetatable("Point")

// __index = metatable itself → method lookup
L.PushValue(-1)
L.SetField(-2, "__index")

// Add methods
L.SetFuncs(map[string]lua.Function{
    "getX": func(L *lua.State) int {
        L.CheckUdata(1, "Point")
        p := L.UserdataValue(1).(*Point)
        L.PushInteger(int64(p.X))
        return 1
    },
    "getY": func(L *lua.State) int {
        L.CheckUdata(1, "Point")
        p := L.UserdataValue(1).(*Point)
        L.PushInteger(int64(p.Y))
        return 1
    },
}, 0)

// __tostring metamethod
L.PushFunction(func(L *lua.State) int {
    p := L.UserdataValue(1).(*Point)
    L.PushString(fmt.Sprintf("Point(%d, %d)", p.X, p.Y))
    return 1
})
L.SetField(-2, "__tostring")
L.Pop(1)  // pop metatable

// Step 2: Constructor function
L.PushFunction(func(L *lua.State) int {
    x := L.CheckInteger(1)
    y := L.CheckInteger(2)
    L.NewUserdata(0, 0)
    L.SetUserdataValue(-1, &Point{X: int(x), Y: int(y)})
    L.GetField(lua.RegistryIndex, "Point")  // get metatable
    L.SetMetatable(-2)                       // attach to userdata
    return 1
})
L.SetGlobal("Point")
```

**Lua side:**
```lua
local p = Point(3, 4)
print(p:getX())      --> 3
print(p:getY())      --> 4
print(tostring(p))   --> "Point(3, 4)"
```

---

## 16. Closures with Upvalues

Create a Go function with persistent state via upvalues.

```go
L.PushInteger(0)  // initial counter value (upvalue 1)

counter := func(L *lua.State) int {
    val, _ := L.ToInteger(lua.UpvalueIndex(1))
    val++
    L.PushInteger(val)
    L.Copy(-1, lua.UpvalueIndex(1))  // update the upvalue
    return 1
}

L.PushClosure(counter, 1)  // 1 upvalue on stack
L.SetGlobal("counter")
```

**Lua side:**
```lua
print(counter())  --> 1
print(counter())  --> 2
print(counter())  --> 3
```

---

## 17. Coroutines — Drive from Go

Create a Lua coroutine and step through it from Go.

```go
L.DoString(`
    function squares(n)
        for i = 1, n do
            coroutine.yield(i * i)
        end
    end
`)

thread := L.NewThread()
thread.GetGlobal("squares")
thread.PushInteger(4)  // argument: n = 4

nArgs := 1  // first Resume passes function arguments
for {
    status, nresults := thread.Resume(L, nArgs)
    nArgs = 0  // subsequent Resumes pass 0 args

    if status == lua.OK {
        break  // coroutine finished
    }
    if status != lua.Yield {
        msg, _ := thread.ToString(-1)
        log.Fatal("coroutine error: ", msg)
    }

    val, _ := thread.ToInteger(-1)
    fmt.Println(val)  // 1, 4, 9, 16
    thread.Pop(nresults)
}
L.Pop(1)  // pop the thread
```

> **Key**: First `Resume` passes the function arguments (`nArgs` = number of args).
> Subsequent `Resume` calls pass values that become the return value of `coroutine.yield()` in Lua.

---

## 18. Yield from Go (Async Pattern)

A Go function can yield control back to the Go host, then receive a value when resumed.

```go
askUser := func(L *lua.State) int {
    // The argument (prompt) is already on the stack.
    return L.Yield(1)  // yield 1 value back to the Go host
}
L.PushFunction(askUser)
L.SetGlobal("ask_user")

L.DoString(`
    function chat()
        local name = ask_user("What is your name?")
        return "Hello, " .. name .. "!"
    end
`)

thread := L.NewThread()
thread.GetGlobal("chat")

// Start the coroutine — it will yield at ask_user()
status, _ := thread.Resume(L, 0)
if status == lua.Yield {
    prompt, _ := thread.ToString(-1)
    fmt.Println("Prompt:", prompt)  // "What is your name?"
    thread.Pop(1)

    // Resume with the "user's answer"
    thread.PushString("Alice")
    status, _ = thread.Resume(L, 1)
}
if status == lua.OK {
    result, _ := thread.ToString(-1)
    fmt.Println(result)  // "Hello, Alice!"
}
L.Pop(1)  // pop thread
```

This pattern is essential for async I/O, user input, or any operation where Go needs to provide a value mid-execution.

---

## 19. Registry References

Store a Lua value in the registry for later retrieval (useful for callbacks).

```go
// Store: pops value from stack, returns integer reference
L.PushString("my-callback")
ref := L.Ref(lua.RegistryIndex)

// Retrieve: pushes the stored value
L.RawGetI(lua.RegistryIndex, int64(ref))
s, _ := L.ToString(-1)
fmt.Println(s)  // "my-callback"
L.Pop(1)

// Free when no longer needed
L.Unref(lua.RegistryIndex, ref)
```

> References are integers. `lua.RefNil` (-1) means the value was nil.
> `lua.NoRef` (-2) is the "no reference" sentinel.

---

## 20. Sandboxing

Use `NewBareState` for a Lua environment with no standard libraries.

```go
L := lua.NewBareState()  // NO io, os, debug, string, table, etc.
defer L.Close()

// Whitelist only safe functions
L.PushFunction(safePrint)
L.SetGlobal("print")

// Dangerous operations are impossible
err := L.DoString(`io.open("secret.txt")`)  // error: io is nil
```

> `NewState()` opens all standard libraries. `NewBareState()` opens none.
> For sandboxing, start bare and register only what you need.

---

## 21. Debug Hooks

Trace execution line-by-line, on function calls, or on returns.

```go
L.SetHook(func(L *lua.State, event int, line int) {
    switch event {
    case lua.HookEventLine:
        fmt.Printf("line %d\n", line)
    case lua.HookEventCall:
        fmt.Println("call")
    case lua.HookEventReturn:
        fmt.Println("return")
    }
}, lua.MaskLine|lua.MaskCall|lua.MaskRet, 0)
```

Clear hooks:
```go
L.SetHook(nil, 0, 0)
```

Read current hook state:
```go
hookFn, mask, count := L.GetHook()
```

**Mask constants**: `lua.MaskCall`, `lua.MaskRet`, `lua.MaskLine`, `lua.MaskCount`.

**Event constants**: `lua.HookEventCall`, `lua.HookEventReturn`, `lua.HookEventLine`, `lua.HookEventCount`, `lua.HookEventTailCall`.

---

## 22. Execution Timeout

Limit CPU usage by counting instructions.

```go
const maxInstructions = 1_000_000
count := 0

L.SetHook(func(L *lua.State, event int, line int) {
    count++
    if count > maxInstructions {
        L.Errorf("execution limit exceeded")
    }
}, lua.MaskCount, 1000)  // fires every 1000 VM instructions
```

> The second argument to `MaskCount` controls granularity.
> `1000` means the hook fires every 1000 instructions — not on every single one.

---

## 23. Require a Go Module

Register a Go module so Lua can `require()` it.

**Option A: package.preload**
```go
L.GetGlobal("package")
L.GetField(-1, "preload")

L.PushFunction(func(L *lua.State) int {
    L.NewTable()
    L.SetFuncs(map[string]lua.Function{
        "hello": func(L *lua.State) int {
            L.PushString("hello from Go!")
            return 1
        },
    }, 0)
    return 1  // return the module table
})
L.SetField(-2, "mymodule")
L.Pop(2)  // pop preload + package
```

**Lua side:**
```lua
local m = require("mymodule")
print(m.hello())  --> "hello from Go!"
```

**Option B: L.Require**
```go
L.Require("mymodule", func(L *lua.State) int {
    L.NewTable()
    L.SetFuncs(map[string]lua.Function{
        "hello": func(L *lua.State) int {
            L.PushString("hello from Go!")
            return 1
        },
    }, 0)
    return 1
}, true)  // true = also set as global
L.Pop(1)  // Require leaves the module on the stack
```

---

## 24. Error Handling in Go Functions

Two conventions for reporting errors from Go back to Lua.

**Convention A: Return nil + error (Lua idiom)**
```go
readFile := func(L *lua.State) int {
    filename := L.CheckString(1)
    data, err := os.ReadFile(filename)
    if err != nil {
        L.PushNil()
        L.PushString(err.Error())
        return 2  // nil, "error message"
    }
    L.PushString(string(data))
    return 1
}
```

Lua side: `local data, err = readfile("x.txt")`

**Convention B: Raise a Lua error (stops execution)**
```go
readFile := func(L *lua.State) int {
    filename := L.CheckString(1)
    data, err := os.ReadFile(filename)
    if err != nil {
        return L.Errorf("cannot read %s: %s", filename, err)
    }
    L.PushString(string(data))
    return 1
}
```

Lua side: must use `pcall` to catch the error.

> Use Convention A for expected failures (file not found, parse error).
> Use Convention B for programming errors (wrong type, invalid state).

---

## 25. User Values on Userdata

Attach extra Lua values to a userdata (like slots for metadata).

```go
// Create userdata with 2 user value slots
L.NewUserdata(0, 2)
L.SetUserdataValue(-1, myGoObject)

// Set user value 1 (a string)
L.PushString("metadata")
L.SetIUserValue(-2, 1)

// Set user value 2 (a table)
L.NewTable()
L.SetIUserValue(-2, 2)

// Read user value 1
tp := L.GetIUserValue(-1, 1)  // pushes value, returns its type
if tp == lua.TypeString {
    s, _ := L.ToString(-1)
    fmt.Println(s)  // "metadata"
}
L.Pop(1)  // pop the user value
```

> The first argument to `NewUserdata(size, nuvalue)`: `size` is for C-style memory (use 0 in Go). `nuvalue` is the number of user value slots.

---

## 26. Push Any Go Value to Lua

Convert any Go value to its Lua equivalent automatically with `PushAny`.

```go
L.PushAny(42)                    // → Lua integer
L.PushAny(3.14)                  // → Lua number
L.PushAny("hello")               // → Lua string
L.PushAny(true)                  // → Lua boolean
L.PushAny(nil)                   // → Lua nil
L.PushAny([]any{1, "two", 3.0}) // → Lua table {1, "two", 3.0}
L.PushAny(map[string]any{        // → Lua table {name="Alice", age=30}
    "name": "Alice",
    "age":  30,
})

// Structs too:
type Config struct {
    Host string `lua:"host"`
    Port int    `lua:"port"`
}
L.PushAny(Config{Host: "localhost", Port: 8080})
// → Lua table {host="localhost", port=8080}
```

---

## 27. Read Lua Values to Go (ToAny / ToStruct)

`ToAny` converts any Lua value to a Go `any`. `ToStruct` maps a Lua table to a typed Go struct.

```go
// ToAny — generic conversion
L.DoString(`config = {host = "localhost", port = 8080, debug = true}`)
L.GetGlobal("config")
val := L.ToAny(-1) // → map[string]any{"host":"localhost", "port":int64(8080), "debug":true}
L.Pop(1)

// ToStruct — typed mapping
type Config struct {
    Host  string `lua:"host"`
    Port  int64  `lua:"port"`
    Debug bool   `lua:"debug"`
}
L.GetGlobal("config")
var cfg Config
err := L.ToStruct(-1, &cfg) // cfg.Host="localhost", cfg.Port=8080, cfg.Debug=true
L.Pop(1)
```

---

## 28. Convenience Table Access

Read and write table fields without manual stack management.

```go
// Read fields directly
L.GetGlobal("config")
host := L.GetFieldString(-1, "host")   // "localhost"
port := L.GetFieldInt(-1, "port")      // 8080
debug := L.GetFieldBool(-1, "debug")   // true
L.Pop(1)

// Set multiple fields at once
L.GetGlobal("config")
L.SetFields(-1, map[string]any{
    "host":  "0.0.0.0",
    "port":  9090,
    "debug": false,
})
L.Pop(1)

// Create a table in one call
L.NewTableFrom(map[string]any{
    "name":    "myapp",
    "version": "1.0",
})
L.SetGlobal("app")
```

---

## 29. Safe Table Iteration (ForEach)

`ForEach` iterates all key-value pairs, handling the stack automatically.

```go
L.GetGlobal("config")
L.ForEach(-1, func(L *lua.State) bool {
    key := L.TolString(-2)
    val := L.ToAny(-1)
    fmt.Printf("%s = %v\n", key, val)
    return true // continue iterating (false = stop early)
})
L.Pop(1)
```

---

## 30. Auto-Bind Any Go Function (PushGoFunc)

`PushGoFunc` uses reflection to bind **any** Go function signature — no manual stack operations needed.

```go
// Simple function
L.PushGoFunc(func(name string, age int) string {
    return fmt.Sprintf("Hello %s, age %d", name, age)
})
L.SetGlobal("greet")
// Lua: greet("Alice", 30) → "Hello Alice, age 30"

// Error returns become Lua errors automatically
L.PushGoFunc(func(path string) (string, error) {
    data, err := os.ReadFile(path)
    return string(data), err
})
L.SetGlobal("read_file")
// Lua: local content = read_file("test.txt")  -- errors raise Lua error

// Variadic functions work too
L.PushGoFunc(func(prefix string, items ...any) string {
    return fmt.Sprintf("%s: %v", prefix, items)
})
L.SetGlobal("log_items")
```

---

## 31. Generic Wrappers (Type-Safe, No Reflection)

`Wrap0R`, `Wrap1R`, `Wrap2R`, etc. use Go generics for compile-time type safety with zero reflection overhead.

```go
// Wrap2R: 2 args, 1 return — type-safe, zero reflection
lua.Wrap2R[string, int, string](L, func(name string, age int) string {
    return fmt.Sprintf("Hello %s, age %d", name, age)
})
L.SetGlobal("greet")

// Wrap1E: 1 arg, result + error
lua.Wrap1E[string, string](L, func(path string) (string, error) {
    data, err := os.ReadFile(path)
    return string(data), err
})
L.SetGlobal("read_file")

// Wrap0R: no args, 1 return
lua.Wrap0R[string](L, func() string {
    return time.Now().Format(time.RFC3339)
})
L.SetGlobal("now")
```

> **PushGoFunc vs Wrap**: `PushGoFunc` uses reflection (flexible, any signature). `Wrap` uses generics (faster, compile-time type safety, limited to 0–3 args).

---

## 32. Sandbox — Run Untrusted Code

`NewSandboxState` creates a restricted Lua state with configurable limits.

```go
L := lua.NewSandboxState(lua.SandboxConfig{
    CPULimit:     1_000_000, // max 1M VM instructions
    AllowIO:      false,     // no file/network access
    AllowDebug:   false,     // no debug library
    AllowPackage: false,     // no require()
})
defer L.Close()

// Safe libraries available: base (restricted), string, table, math, utf8, coroutine
// Removed from base: dofile, loadfile, load, require

err := L.DoString(untrustedCode)
if err != nil {
    fmt.Println("Script error:", err) // includes CPU limit exceeded
}
```

---

## 33. CPU Instruction Limits

Fine-grained CPU control on any state with `SetCPULimit`, `ResetCPUCounter`, `CPUInstructionsUsed`.

```go
L := lua.NewState()
defer L.Close()

L.SetCPULimit(500_000) // 500K instructions max

err := L.DoString(`while true do end`) // will error: "CPU limit exceeded"

// Reset for next execution
L.ResetCPUCounter()
err = L.DoString(`return 1 + 1`) // fresh budget

// Check usage
fmt.Println("Instructions used:", L.CPUInstructionsUsed())
```

---

## 34. Context Cancellation / Timeout

Attach a Go `context.Context` to a Lua state for cancellation and deadlines.

```go
L := lua.NewState()
defer L.Close()

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
L.SetContext(ctx)

err := L.DoString(`while true do end`)
// err: "context cancelled: context deadline exceeded"

// Remove context
L.SetContext(nil)
```

---

## 35. Virtual Filesystem (embed.FS)

Use Go's `embed.FS` or any `fs.FS` as the Lua file system for `DoFile`, `require`, and `dofile`.

```go
//go:embed lua/*
var luaFS embed.FS

func main() {
    L := lua.NewState()
    defer L.Close()

    sub, _ := fs.Sub(luaFS, "lua")
    L.SetFileSystem(sub)

    // All file operations now use the embedded FS
    L.DoFile("init.lua")                 // reads from embed.FS
    L.DoString(`require("mymodule")`)    // searchers use embed.FS
    L.DoString(`dofile("config.lua")`)   // dofile uses embed.FS
}
```

---

## 36. Global Module Registry

Register modules process-wide so every `State` can `require` them.

```go
// In package init() — available to ALL Lua States
func init() {
    lua.RegisterGlobal("mylib", func(L *lua.State) {
        L.NewLib(map[string]lua.Function{
            "hello": func(L *lua.State) int {
                L.PushString("hello from mylib!")
                return 1
            },
        })
    })
}

// Any State can now require it
L := lua.NewState()
defer L.Close()
L.DoString(`local m = require("mylib"); print(m.hello())`)
```

---

## 37. Module Interface Pattern

Implement the `Module` interface for reusable, self-contained library packages.

```go
// Define a module
type MathExtModule struct{}

func (MathExtModule) Name() string { return "mathext" }

func (MathExtModule) Open(L *lua.State) {
    L.NewLib(map[string]lua.Function{
        "clamp": func(L *lua.State) int {
            val := L.CheckNumber(1)
            min := L.CheckNumber(2)
            max := L.CheckNumber(3)
            if val < min { val = min }
            if val > max { val = max }
            L.PushNumber(val)
            return 1
        },
    })
}

// Load into specific States (not global)
L := lua.NewState()
lua.LoadModules(L, MathExtModule{})
L.DoString(`local m = require("mathext"); print(m.clamp(150, 0, 100))`) // 100
```

---

## 38. State Pool for Concurrent Requests

Reuse Lua states across goroutines with `StatePool` — ideal for HTTP servers.

```go
pool := lua.NewStatePool(lua.PoolConfig{
    MaxStates: 16,
    InitFunc: func(L *lua.State) {
        // Each State gets the same setup
        lua.LoadModules(L, MyModule{})
        L.DoString(`handler = require("handler")`)
    },
})
defer pool.Close()

http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
    L := pool.Get()
    defer pool.Put(L)

    L.GetGlobal("handler")
    L.GetField(-1, "process")
    L.PushString(r.URL.Path)
    if err := L.CallSafe(1, 1); err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    result, _ := L.ToString(-1)
    L.Pop(2) // pop result + handler table
    fmt.Fprint(w, result)
})
```

---

## 39. Async Task Executor

`Executor` runs Lua tasks in background goroutines with pooled states.

```go
exec := lua.NewExecutor(lua.ExecutorConfig{
    PoolConfig:   lua.PoolConfig{MaxStates: 4},
    ResultBuffer: 100,
})
defer exec.Shutdown()

// Submit tasks
exec.Submit(lua.Task{ID: "task1", Code: `return 40 + 2`})
exec.Submit(lua.Task{ID: "task2", Func: func(L *lua.State) (any, error) {
    L.DoString(`x = 0; for i=1,1000 do x = x + i end`)
    n, _ := L.ToInteger(-1)
    return n, nil
}})

// Collect results
for result := range exec.Results() {
    fmt.Printf("Task %s: value=%v, err=%v\n", result.ID, result.Value, result.Error)
}
```

---

## 40. Channels — Go ↔ Lua Communication

`Channel` provides a typed, buffered channel for passing values between Go goroutines and Lua scripts.

```go
ch := lua.NewChannel(10) // buffered channel

// Go producer goroutine
go func() {
    for i := 0; i < 5; i++ {
        ch.Send(fmt.Sprintf("message-%d", i))
    }
    ch.Close()
}()

// Lua consumer
L := lua.NewState()
defer L.Close()
L.PushUserdata(ch)
L.SetGlobal("ch")

L.DoString(`
    local channel = require("channel")
    while true do
        local val, ok = channel.recv(ch)
        if not ok then break end
        print("Got:", val)
    end
`)
```

---

## 41. JSON — Encode and Decode

Built-in JSON module, available via `require("json")`.

```go
L := lua.NewState()
defer L.Close()

L.DoString(`
    local json = require("json")

    -- Encode
    local data = {name = "Alice", scores = {95, 87, 92}}
    local s = json.encode(data)
    print(s)  -- {"name":"Alice","scores":[95,87,92]}

    -- Pretty print
    print(json.encode_pretty(data))

    -- Decode
    local t = json.decode('{"x":1,"y":2}')
    print(t.x, t.y)  -- 1  2
`)
```

---

## 42. HTTP Requests from Lua

Built-in HTTP module for GET, POST, and generic requests.

```go
L := lua.NewState()
defer L.Close()

L.DoString(`
    local http = require("http")

    -- Simple GET
    local resp = http.get("https://httpbin.org/get")
    print(resp.status, resp.body)

    -- POST with JSON body
    local json = require("json")
    local resp = http.post("https://httpbin.org/post", {
        body = json.encode({name = "Alice"}),
        headers = {["Content-Type"] = "application/json"},
        timeout = 10,  -- seconds
    })

    -- Generic request
    local resp = http.request({
        method = "PUT",
        url = "https://httpbin.org/put",
        body = "hello",
        headers = {["X-Custom"] = "value"},
        timeout = 30,
    })
    if resp then
        print(resp.status, resp.headers["content-type"])
    end
`)
```

> Response table: `{status=200, status_text="200 OK", body="...", headers={...}}`
> On error: returns `nil, error_string`.
> Default timeout: 30s. Max response body: 10MB. Respects State's context for cancellation.

---

## 43. Async/Await with Futures

Cooperative async using `Scheduler`, coroutines, and `Future` objects.

```go
L := lua.NewState()
defer L.Close()

sched := lua.NewScheduler(L)

// Define an async worker
L.DoString(`
    local async = require("async")

    function worker()
        local f1 = async.go("return 40 + 2")
        local f2 = async.go("return 'hello'")

        local v1 = async.await(f1)  -- yields until f1 resolves
        local v2 = async.await(f2)

        print(v1, v2)  -- 42  hello
    end
`)
L.GetGlobal("worker")
sched.Spawn(L)

// Drive the scheduler until all coroutines complete
sched.WaitAll(5 * time.Second)
```

> **Important**: `async.go` takes a **code string**, not a function. Lua closures are bound to their parent State and cannot safely run in another goroutine.

---

## 44. CallSafe and CallRef — Protected Calls

Convenience wrappers that return Go `error` instead of status codes.

```go
// CallSafe — PCall that returns a Go error
L.GetGlobal("process")
L.PushAny(inputData)
err := L.CallSafe(1, 1) // returns Go error instead of status code
if err != nil {
    log.Println("Lua error:", err)
}

// GetFieldRef — store a function reference for later calls
L.GetGlobal("callbacks")
ref := L.GetFieldRef(-1, "on_event") // stores function in registry, returns ref int
L.Pop(1)

// CallRef — call a stored function reference
L.PushAny(eventData)
err = L.CallRef(ref, 1, 0) // pushes ref'd function, calls with 1 arg, 0 results

// Clean up when done
L.Unref(lua.RegistryIndex, ref)
```

---

## Quick Reference: Stack Operations

| Operation | Effect |
|-----------|--------|
| `L.PushNil()` | Push nil |
| `L.PushBoolean(b)` | Push bool |
| `L.PushInteger(n)` | Push int64 |
| `L.PushNumber(n)` | Push float64 |
| `L.PushString(s)` | Push string |
| `L.PushFunction(f)` | Push Go function |
| `L.PushClosure(f, n)` | Push Go closure with n upvalues |
| `L.PushValue(idx)` | Copy value at idx to top |
| `L.Pop(n)` | Remove top n values |
| `L.GetTop()` | Number of values on stack |
| `L.SetTop(n)` | Set stack size (pops or pushes nils) |
| `L.Remove(idx)` | Remove value at idx, shift down |
| `L.Copy(from, to)` | Copy value between indices |

## Quick Reference: Type Checking

| Function | Returns |
|----------|---------|
| `L.Type(idx)` | `lua.Type` enum |
| `L.TypeName(tp)` | `string` (e.g. "nil", "number") |
| `L.IsNil(idx)` | `bool` |
| `L.IsBoolean(idx)` | `bool` |
| `L.IsInteger(idx)` | `bool` |
| `L.IsNumber(idx)` | `bool` |
| `L.IsString(idx)` | `bool` |
| `L.IsTable(idx)` | `bool` |
| `L.IsFunction(idx)` | `bool` |

## Quick Reference: Value Extraction

| Function | Returns | Notes |
|----------|---------|-------|
| `L.ToBoolean(idx)` | `bool` | false for nil/false, true otherwise |
| `L.ToInteger(idx)` | `(int64, bool)` | ok=false if not convertible |
| `L.ToNumber(idx)` | `(float64, bool)` | ok=false if not convertible |
| `L.ToString(idx)` | `(string, bool)` | only actual strings |
| `L.TolString(idx)` | `string` | converts any value (like `tostring()`) |
| `L.CheckInteger(n)` | `int64` | raises Lua error if arg n is not integer |
| `L.CheckNumber(n)` | `float64` | raises Lua error if arg n is not number |
| `L.CheckString(n)` | `string` | raises Lua error if arg n is not string |
| `L.OptInteger(n, d)` | `int64` | returns d if arg n is nil/absent |
| `L.OptString(n, d)` | `string` | returns d if arg n is nil/absent |

---

## Common Mistakes

### ❌ Forgetting to Pop

Every value you push or get must eventually be popped. Stack leaks cause crashes.

```go
// WRONG — leaks "settings" table on stack
L.GetGlobal("settings")
L.GetField(-1, "width")
w, _ := L.ToInteger(-1)
L.Pop(1)  // only popped width, forgot settings!

// RIGHT
L.GetGlobal("settings")
L.GetField(-1, "width")
w, _ := L.ToInteger(-1)
L.Pop(2)  // pop width + settings
```

### ❌ Using ToInteger Instead of CheckInteger

- `ToInteger` returns `(value, ok)` — silent failure, you must check `ok`.
- `CheckInteger` raises a Lua error on failure — usually what you want in Go functions that are called from Lua.

```go
// In a Go function called from Lua:
a := L.CheckInteger(1)  // ✅ raises clear error if arg 1 is not an integer

// In Go code reading a table field:
L.GetField(-1, "count")
n, ok := L.ToInteger(-1)  // ✅ check ok before using n
```

### ❌ Looking for bit32

Lua 5.4+ uses native bitwise operators: `&` `|` `~` `<<` `>>`.
There is no `bit32` library.

```lua
-- WRONG: bit32.band(a, b)
-- RIGHT:
local result = a & b
local shifted = x << 4
```

### ❌ Wrong Index After Push/Pop

After pushing a value, everything below shifts in relative terms. Use negative indices.

```go
L.GetGlobal("mytable")  // -1 = table
L.PushString("value")   // -1 = "value", -2 = table
L.SetField(-2, "key")   // use -2, not -1!
```

### ❌ Wrong nArgs on Resume

First `Resume`: `nArgs` = number of function arguments.
Subsequent `Resume`: `nArgs` = number of values to send as the return value of `yield()`.

```go
thread.GetGlobal("myfunc")
thread.PushInteger(42)
status, n := thread.Resume(L, 1)  // first call: 1 arg

// Later...
thread.PushString("answer")
status, n = thread.Resume(L, 1)   // send 1 value as yield's return
```

### ❌ Modifying the Key During Table Iteration

`L.Next` expects the key at the top of the stack. Don't call `ToString` on the key — it may convert a number key to a string, breaking iteration.

```go
// WRONG
for L.Next(-2) {
    key, _ := L.ToString(-2)  // ⚠️ may modify the key!
    L.Pop(1)
}

// RIGHT — use TolString or check type first
for L.Next(-2) {
    key := L.TolString(-2)  // safe: doesn't modify stack
    L.Pop(1)
}
```

### ❌ Forgetting defer L.Close()

Always close the Lua state to free resources.

```go
L := lua.NewState()
defer L.Close()  // don't forget!
```
