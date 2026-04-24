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
