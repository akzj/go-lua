# go-lua API Reference

> Complete API reference for the `github.com/akzj/go-lua/pkg/lua` package.
> Maps every public Go function to its C Lua 5.5 equivalent.

```go
import "github.com/akzj/go-lua/pkg/lua"
```

---

## Quick Start

```go
L := lua.NewState()    // new state with all standard libraries
defer L.Close()

// Execute Lua code
if err := L.DoString(`print("hello from Lua")`); err != nil {
    log.Fatal(err)
}

// Register a Go function
add := func(L *lua.State) int {
    a := L.CheckInteger(1)
    b := L.CheckInteger(2)
    L.PushInteger(a + b)
    return 1 // number of return values
}
L.PushFunction(add)
L.SetGlobal("add")
```

---

## Type Definitions

```go
// The Lua interpreter state (opaque handle).
type State struct { ... }

// Go function callable from Lua. Receives the state, returns number of results pushed.
// This is the #1 type you need — every Go callback uses this signature.
type Function func(L *State) int

// Debug hook callback. event is one of the HookEvent* constants.
type HookFunc func(L *State, event int, currentLine int)

// Warning handler callback.
type WarnFunction func(ud interface{}, msg string, tocont bool)

// Lua value type (TypeNil, TypeBoolean, etc.)
type Type int

// Comparison operator for State.Compare (OpEQ, OpLT, OpLE)
type CompareOp int

// Arithmetic/bitwise operator for State.Arith (OpAdd, OpSub, ... OpBNot)
type ArithOp int

// GC operation for State.GC (GCStop, GCCollect, etc.)
type GCWhat int

// Debug information about a function activation record.
type DebugInfo struct {
    Source          string // source of the chunk (e.g. "@filename.lua")
    ShortSrc        string // short source for error messages (max 60 chars)
    LineDefined     int    // line where function definition starts
    LastLineDefined int    // line where function definition ends
    CurrentLine     int    // current line being executed
    Name            string // function name (if known)
    NameWhat        string // "global", "local", "method", "field", or ""
    What            string // "Lua", "C", or "main"
    NUps            int    // number of upvalues
    NParams         int    // number of fixed parameters
    IsVararg        bool   // true if function is variadic
    IsTailCall      bool   // true if this is a tail call
    ExtraArgs       int    // number of extra arguments (vararg)
    FTransfer       int    // index of first transferred value
    NTransfer       int    // number of transferred values
}
```

---

## Constants

### Status Codes

Returned by `PCall`, `Resume`, `Load`, `LoadFile`.

```go
lua.OK        = 0   // no errors
lua.Yield     = 1   // coroutine yielded (from Resume)
lua.ErrRun    = 2   // runtime error
lua.ErrSyntax = 3   // syntax error during compilation
lua.ErrMem    = 4   // memory allocation error
lua.ErrErr    = 5   // error while running the message handler
lua.ErrFile   = 6   // file I/O error (from LoadFile)
```

### Pseudo-Indices and Special Values

```go
lua.RegistryIndex = -1001000  // pseudo-index for the Lua registry table
lua.MultiRet      = -1        // accept all return values (for Call/PCall nResults)

lua.UpvalueIndex(i int) int   // pseudo-index for upvalue i (1-based)

lua.RIdxMainThread = 3        // registry key: main thread
lua.RIdxGlobals    = 2        // registry key: global environment table

lua.RefNil  = -1              // Ref() return for nil values
lua.NoRef   = -2              // invalid/freed reference sentinel
```

### Lua Value Types

```go
lua.TypeNone          = -1  // LUA_TNONE — invalid/absent stack index
lua.TypeNil           =  0  // LUA_TNIL
lua.TypeBoolean       =  1  // LUA_TBOOLEAN
lua.TypeLightUserdata =  2  // LUA_TLIGHTUSERDATA
lua.TypeNumber        =  3  // LUA_TNUMBER
lua.TypeString        =  4  // LUA_TSTRING
lua.TypeTable         =  5  // LUA_TTABLE
lua.TypeFunction      =  6  // LUA_TFUNCTION
lua.TypeUserdata      =  7  // LUA_TUSERDATA
lua.TypeThread        =  8  // LUA_TTHREAD
```

### Comparison Operators

```go
lua.OpEQ = 0  // == (may invoke __eq)
lua.OpLT = 1  // <  (may invoke __lt)
lua.OpLE = 2  // <= (may invoke __le)
```

### Arithmetic / Bitwise Operators

```go
lua.OpAdd  =  0  // +
lua.OpSub  =  1  // -
lua.OpMul  =  2  // *
lua.OpMod  =  3  // %
lua.OpPow  =  4  // ^
lua.OpDiv  =  5  // / (float division)
lua.OpIDiv =  6  // // (floor division)
lua.OpBAnd =  7  // & (bitwise AND)
lua.OpBOr  =  8  // | (bitwise OR)
lua.OpBXor =  9  // ~ (bitwise XOR)
lua.OpShl  = 10  // << (left shift)
lua.OpShr  = 11  // >> (right shift)
lua.OpUnm  = 12  // - (unary minus)
lua.OpBNot = 13  // ~ (bitwise NOT, unary)
```

### GC Operations

```go
lua.GCStop      =  0  // stop the garbage collector
lua.GCRestart   =  1  // restart the garbage collector
lua.GCCollect   =  2  // perform a full collection cycle
lua.GCCount     =  3  // return total memory in use (KBytes)
lua.GCCountB    =  4  // return remainder bytes
lua.GCStep      =  5  // perform an incremental GC step
lua.GCIsRunning =  9  // return 1 if GC is running
lua.GCGen       = 10  // switch to generational mode
lua.GCInc       = 11  // switch to incremental mode
```

### Hook Events and Masks

```go
// Hook events (passed to HookFunc)
lua.HookEventCall     // function call
lua.HookEventReturn   // function return
lua.HookEventLine     // new source line
lua.HookEventCount    // instruction count reached
lua.HookEventTailCall // tail call

// Hook masks (combine with | for SetHook)
lua.MaskCall  = 1 << 0  // trigger on function calls
lua.MaskRet   = 1 << 1  // trigger on function returns
lua.MaskLine  = 1 << 2  // trigger on new source lines
lua.MaskCount = 1 << 3  // trigger every N instructions
```

---

## State Lifecycle

| Go API | C Lua Equivalent | Description |
|--------|------------------|-------------|
| `lua.NewState() *State` | `luaL_newstate` + `luaL_openlibs` | New state with all standard libraries loaded |
| `lua.NewBareState() *State` | `lua_newstate` | New state WITHOUT standard libraries (for sandboxing) |
| `L.Close()` | `lua_close` | Release all resources |

---

## Stack — Push Values (Go → Lua)

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.PushNil()` | `lua_pushnil` | |
| `L.PushBoolean(b bool)` | `lua_pushboolean` | |
| `L.PushInteger(n int64)` | `lua_pushinteger` | |
| `L.PushNumber(n float64)` | `lua_pushnumber` | |
| `L.PushString(s string) string` | `lua_pushstring` | Returns the interned string |
| `L.PushFString(format string, args ...interface{}) string` | `lua_pushfstring` | Printf-style formatting |
| `L.PushFunction(f Function)` | `lua_pushcfunction` | `Function = func(L *State) int` |
| `L.PushClosure(f Function, n int)` | `lua_pushcclosure` | n upvalues must be on stack first |
| `L.PushLightUserdata(p interface{})` | `lua_pushlightuserdata` | Raw Go value, no metatable |
| `L.PushGlobalTable()` | `lua_pushglobaltable` | |
| `L.PushThread() bool` | `lua_pushthread` | Returns true if main thread |
| `L.PushFail()` | `luaL_pushfail` | Pushes nil (Lua 5.5 fail convention) |
| `L.PushValue(idx int)` | `lua_pushvalue` | Copy value at idx to top |

---

## Stack — Read Values (Lua → Go)

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.ToBoolean(idx int) bool` | `lua_toboolean` | false for nil/false, true for everything else |
| `L.ToInteger(idx int) (int64, bool)` | `lua_tointegerx` | bool = conversion success |
| `L.ToNumber(idx int) (float64, bool)` | `lua_tonumberx` | bool = conversion success |
| `L.ToString(idx int) (string, bool)` | `lua_tolstring` | Coerces numbers; bool = was string/number |
| `L.ToPointer(idx int) string` | `lua_topointer` | String representation of pointer |
| `L.ToThread(idx int) *State` | `lua_tothread` | nil if not a thread |
| `L.RawLen(idx int) int64` | `lua_rawlen` | Length without `__len` metamethod |
| `L.StringToNumber(s string) int` | `luaL_stringtonumber` | Returns len+1 on success, 0 on failure |

---

## Stack — Check Values (Raise Lua Error on Mismatch)

These are convenience wrappers for argument validation inside `Function` callbacks.

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.CheckString(idx int) string` | `luaL_checkstring` | Raises error if not a string |
| `L.CheckInteger(idx int) int64` | `luaL_checkinteger` | Raises error if not an integer |
| `L.CheckNumber(idx int) float64` | `luaL_checknumber` | Raises error if not a number |
| `L.CheckType(idx int, tp Type)` | `luaL_checktype` | Raises error if wrong type |
| `L.CheckAny(idx int)` | `luaL_checkany` | Raises error if index is invalid |
| `L.OptString(idx int, def string) string` | `luaL_optstring` | Returns `def` if nil/absent |
| `L.OptInteger(idx int, def int64) int64` | `luaL_optinteger` | Returns `def` if nil/absent |
| `L.OptNumber(idx int, def float64) float64` | `luaL_optnumber` | Returns `def` if nil/absent |
| `L.CheckOption(idx int, def string, opts []string) int` | `luaL_checkoption` | Returns index of matched option |
| `L.CheckUdata(idx int, tname string)` | `luaL_checkudata` | Raises type error if not matching userdata |
| `L.TestUdata(idx int, tname string) bool` | `luaL_testudata` | Returns true if userdata matches tname |

---

## Stack — Type Checking

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.Type(idx int) Type` | `lua_type` | Returns the type of value at idx |
| `L.TypeName(tp Type) string` | `lua_typename` | Returns type name string |
| `L.IsNil(idx int) bool` | `lua_isnil` | |
| `L.IsNone(idx int) bool` | `lua_isnone` | True if index is not valid |
| `L.IsNoneOrNil(idx int) bool` | `lua_isnoneornil` | |
| `L.IsBoolean(idx int) bool` | `lua_isboolean` | |
| `L.IsInteger(idx int) bool` | `lua_isinteger` | |
| `L.IsNumber(idx int) bool` | `lua_isnumber` | True for numbers and numeric strings |
| `L.IsString(idx int) bool` | `lua_isstring` | True for strings and numbers |
| `L.IsFunction(idx int) bool` | `lua_isfunction` | Lua or Go function |
| `L.IsTable(idx int) bool` | `lua_istable` | |
| `L.IsCFunction(idx int) bool` | `lua_iscfunction` | True if Go function |
| `L.IsUserdata(idx int) bool` | `lua_isuserdata` | Full or light userdata |

---

## Stack Manipulation

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.GetTop() int` | `lua_gettop` | Number of elements on the stack |
| `L.SetTop(idx int)` | `lua_settop` | Set stack top; 0 = clear all |
| `L.Pop(n int)` | `lua_pop` | Remove n elements from top |
| `L.AbsIndex(idx int) int` | `lua_absindex` | Convert negative index to absolute |
| `L.CheckStack(n int) bool` | `lua_checkstack` | Ensure space for n extra elements |
| `L.Copy(fromIdx, toIdx int)` | `lua_copy` | Copy value at fromIdx to toIdx |
| `L.Rotate(idx, n int)` | `lua_rotate` | Rotate elements between idx and top |
| `L.Insert(idx int)` | `lua_insert` | Move top element to idx |
| `L.Remove(idx int)` | `lua_remove` | Remove element at idx, shift down |
| `L.Replace(idx int)` | `lua_replace` | Replace value at idx with top, pop top |

---

## Table Operations

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.NewTable()` | `lua_newtable` | Push new empty table |
| `L.CreateTable(nArr, nRec int)` | `lua_createtable` | Pre-allocated table |
| `L.GetTable(idx int) Type` | `lua_gettable` | Push `t[k]` (k = top, pops key; metamethods) |
| `L.SetTable(idx int)` | `lua_settable` | `t[k] = v` (k = top-1, v = top; pops both; metamethods) |
| `L.GetField(idx int, key string) Type` | `lua_getfield` | Push `t[key]` (metamethods) |
| `L.SetField(idx int, key string)` | `lua_setfield` | `t[key] = top` (pops value; metamethods) |
| `L.GetI(idx int, n int64) Type` | `lua_geti` | Push `t[n]` (metamethods) |
| `L.SetI(idx int, n int64)` | `lua_seti` | `t[n] = top` (pops value; metamethods) |
| `L.GetGlobal(name string) Type` | `lua_getglobal` | Push `_G[name]` |
| `L.SetGlobal(name string)` | `lua_setglobal` | `_G[name] = top` (pops value) |
| `L.RawGet(idx int) Type` | `lua_rawget` | Like GetTable, no metamethods |
| `L.RawSet(idx int)` | `lua_rawset` | Like SetTable, no metamethods |
| `L.RawGetI(idx int, n int64) Type` | `lua_rawgeti` | Like GetI, no metamethods |
| `L.RawSetI(idx int, n int64)` | `lua_rawseti` | Like SetI, no metamethods |
| `L.RawGetP(idx int, p uintptr) Type` | `lua_rawgetp` | Get by pointer key, no metamethods |
| `L.RawSetP(idx int, p uintptr)` | `lua_rawsetp` | Set by pointer key, no metamethods |
| `L.GetMetatable(idx int) bool` | `lua_getmetatable` | Push metatable; false = none (nothing pushed) |
| `L.SetMetatable(idx int)` | `lua_setmetatable` | Pop table, set as metatable of value at idx |
| `L.GetMetafield(idx int, field string) bool` | `luaL_getmetafield` | Push metamethod; false = not found |
| `L.GetSubTable(idx int, fname string) bool` | `luaL_getsubtable` | Ensure `t[fname]` is table; true = already existed |
| `L.Next(idx int) bool` | `lua_next` | Pop key, push next key–value; false = end |
| `L.Len(idx int)` | `lua_len` | Push length (may trigger `__len`) |
| `L.RawLen(idx int) int64` | `lua_rawlen` | Length without metamethods |
| `L.RawEqual(idx1, idx2 int) bool` | `lua_rawequal` | Equality without metamethods |
| `L.Compare(idx1, idx2 int, op CompareOp) bool` | `lua_compare` | May trigger `__eq`/`__lt`/`__le` |
| `L.Concat(n int)` | `lua_concat` | Concatenate top n values (may trigger `__concat`) |
| `L.Arith(op ArithOp)` | `lua_arith` | Arithmetic on top values (may trigger metamethods) |

---

## Calling Functions

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.Call(nArgs, nResults int)` | `lua_call` | Unprotected call; panics on error |
| `L.PCall(nArgs, nResults, msgHandler int) int` | `lua_pcall` | Protected call; returns status code |
| `L.Load(code, name, mode string) int` | `luaL_loadbufferx` | Compile without executing; pushes chunk |
| `L.DoString(code string) error` | `luaL_dostring` | Load + execute; returns Go error |
| `L.LoadFile(filename, mode string) int` | `luaL_loadfilex` | Load file; mode: "t", "b", "bt", or "" |
| `L.DoFile(filename string) error` | `luaL_dofile` | Load + execute file; returns Go error |
| `L.Error() int` | `lua_error` | Raise Lua error (top = error object); does not return |
| `L.Dump(strip bool) []byte` | `lua_dump` | Dump function as binary chunk; nil if not a function |

---

## Userdata

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.NewUserdata(size, nUV int)` | `lua_newuserdatauv` | Create full userdata with nUV user values; pushes it |
| `L.UserdataValue(idx int) any` | `lua_touserdata` | Get Go value stored in userdata |
| `L.SetUserdataValue(idx int, v any)` | N/A (go-lua specific) | Store any Go value in full userdata |
| `L.GetIUserValue(idx int, n int) Type` | `lua_getiuservalue` | Push nth user value of userdata |
| `L.SetIUserValue(idx int, n int) bool` | `lua_setiuservalue` | Set nth user value from top; pops value |
| `L.GetUpvalue(funcIdx, n int) (string, bool)` | `lua_getupvalue` | Push upvalue n; returns (name, true) or ("", false) |
| `L.SetUpvalue(funcIdx, n int) (string, bool)` | `lua_setupvalue` | Set upvalue n from top; returns (name, true) or ("", false) |

---

## Coroutines

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.NewThread() *State` | `lua_newthread` | Create coroutine; pushes thread on L's stack |
| `L.Resume(from *State, nArgs int) (int, int)` | `lua_resume` | Returns (status, nResults) |
| `L.Yield(nResults int) int` | `lua_yield` | Yield from Go function |
| `L.YieldK(nResults, ctx int, k Function) int` | `lua_yieldk` | Yield with continuation function |
| `L.XMove(to *State, n int)` | `lua_xmove` | Move n values from L to `to` |
| `L.Status() int` | `lua_status` | OK, Yield, or error code |
| `L.IsYieldable() bool` | `lua_isyieldable` | True if coroutine can yield |
| `L.CloseThread(from *State) int` | `lua_closethread` | Close pending to-be-closed variables |

---

## Debug

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.SetHook(f HookFunc, mask, count int)` | `lua_sethook` | Set debug hook; nil f removes hook |
| `L.GetHook() (HookFunc, int, int)` | `lua_gethook` + `lua_gethookmask` + `lua_gethookcount` | Returns (func, mask, count) |
| `L.GetStack(level int) (*DebugInfo, bool)` | `lua_getstack` | Level 0 = current function |
| `L.GetInfo(what string, ar *DebugInfo) bool` | `lua_getinfo` | Fill fields per `what` chars (see below) |
| `L.GetLocal(ar *DebugInfo, n int) string` | `lua_getlocal` | Push local var n; returns name or "" |
| `L.SetLocal(ar *DebugInfo, n int) string` | `lua_setlocal` | Set local var n from top; returns name or "" |
| `L.HookMask() int` | `lua_gethookmask` | Current hook mask |
| `L.HookCount() int` | `lua_gethookcount` | Current base hook count |
| `L.HookActive() bool` | N/A (go-lua specific) | True if any hooks are set |
| `L.HasCallFrames() bool` | N/A (go-lua specific) | True if thread has call frames above base |
| `L.UpvalueId(funcIdx, n int) interface{}` | `lua_upvalueid` | Unique identifier for upvalue |
| `L.UpvalueJoin(funcIdx1, n1, funcIdx2, n2 int)` | `lua_upvaluejoin` | Make upvalue n1 of f1 share n2 of f2 |

### GetInfo `what` Characters

| Char | Fields Filled |
|------|---------------|
| `'n'` | `Name`, `NameWhat` |
| `'S'` | `Source`, `ShortSrc`, `What`, `LineDefined`, `LastLineDefined` |
| `'l'` | `CurrentLine` |
| `'u'` | `NUps`, `NParams`, `IsVararg` |
| `'f'` | pushes the function onto the stack |
| `'r'` | `FTransfer`, `NTransfer` |
| `'t'` | `IsTailCall`, `ExtraArgs` |

### Hook Example

```go
L.SetHook(func(L *lua.State, event int, line int) {
    if event == lua.HookEventLine {
        fmt.Printf("executing line %d\n", line)
    }
}, lua.MaskLine, 0)
```

---

## Garbage Collection

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.GC(what GCWhat, args ...int) int` | `lua_gc` | General GC control |
| `L.GCCollect()` | `lua_gc(L, LUA_GCCOLLECT)` | Force full collection cycle |
| `L.GCStepAPI() bool` | `lua_gc(L, LUA_GCSTEP)` | Incremental step; true if cycle completed |
| `L.GCTotalBytes() int64` | N/A (go-lua specific) | Total bytes tracked by Lua GC |
| `L.GetGCMode() string` | N/A (go-lua specific) | Returns `"incremental"` or `"generational"` |
| `L.SetGCMode(mode string) string` | N/A (go-lua specific) | Set mode; returns previous mode |
| `L.IsGCRunning() bool` | `lua_gc(L, LUA_GCISRUNNING)` | True if GC is not stopped |
| `L.SetGCStopped(stopped bool)` | N/A (go-lua specific) | Set/clear GC stopped flag |
| `L.GetGCParam(name string) int64` | N/A (go-lua specific) | Get GC parameter by name |
| `L.SetGCParam(name string, value int64) int64` | N/A (go-lua specific) | Set GC parameter; returns previous value |

### GC Parameter Names

| Name | Description |
|------|-------------|
| `"pause"` | GC pause (controls cycle frequency) |
| `"stepmul"` | Step multiplier (controls step size) |
| `"stepsize"` | Step size |
| `"minormul"` | Minor collection multiplier (generational) |
| `"majorminor"` | Major-to-minor threshold (generational) |
| `"minormajor"` | Minor-to-major threshold (generational) |

---

## Auxiliary / Library Building

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.SetFuncs(funcs map[string]Function, nUp int)` | `luaL_setfuncs` | Register functions into table at top |
| `L.NewLib(funcs map[string]Function)` | `luaL_newlib` | Create table + register functions |
| `L.Require(modname string, openf Function, global bool)` | `luaL_requiref` | Load/cache module |
| `L.NewMetatable(tname string) bool` | `luaL_newmetatable` | Create registry metatable; false = already exists |
| `L.Ref(t int) int` | `luaL_ref` | Pop top, store in table t; returns reference key |
| `L.Unref(t int, ref int)` | `luaL_unref` | Free reference in table t |
| `L.ArgError(arg int, extraMsg string) int` | `luaL_argerror` | Raise argument error (does not return) |
| `L.TypeError(arg int, tname string) int` | `luaL_typeerror` | Raise type error (does not return) |
| `L.Where(level int)` | `luaL_where` | Push `"source:line: "` string |
| `L.Errorf(format string, args ...interface{}) int` | `luaL_error` | Raise formatted error (does not return) |
| `L.LenI(idx int) int64` | `luaL_len` | Length as integer (may trigger `__len`) |
| `L.TolString(idx int) string` | `luaL_tolstring` | Convert via `__tostring`; pushes result |
| `L.ArgCheck(cond bool, arg int, extraMsg string)` | `luaL_argcheck` | Raise error if cond is false |
| `L.ArgExpected(cond bool, arg int, tname string)` | `luaL_argexpected` | Raise type error if cond is false |

---

## To-Be-Closed Variables

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.ToClose(idx int)` | `lua_toclose` | Mark value as to-be-closed (`<close>` equivalent) |
| `L.CloseSlot(idx int)` | `lua_closeslot` | Close slot and set to nil |

---

## Warning System

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.Warning(msg string, tocont bool)` | `lua_warning` | Emit warning; tocont = message continues |
| `L.SetWarnF(f WarnFunction, ud interface{})` | `lua_setwarnf` | Set warning handler |

---

## Thread Management

| Go API | C Lua Equivalent | Notes |
|--------|------------------|-------|
| `L.CloseThread(from *State) int` | `lua_closethread` | Reset thread, close to-be-closed vars |

---

## Deprecated API

These functions exist for backward compatibility. Use `SetHook`/`GetHook` instead.

| Go API | Replacement |
|--------|-------------|
| `L.SetHookFields(mask, count int)` | `L.SetHook(f, mask, count)` |
| `L.ClearHookFields()` | `L.SetHook(nil, 0, 0)` |
| `L.SetHookMarker()` | `L.SetHook(f, mask, count)` |

---

## Common Patterns

### Registering a Module

```go
func openMyLib(L *lua.State) int {
    L.NewLib(map[string]lua.Function{
        "greet": myGreet,
        "add":   myAdd,
    })
    return 1
}

L.Require("mylib", openMyLib, true) // true = set as global
```

### Creating Userdata with Metatable

```go
func newMyObject(L *lua.State) int {
    L.NewUserdata(0, 0)
    L.SetUserdataValue(-1, &MyGoStruct{})

    if L.NewMetatable("MyObject") {
        L.SetFuncs(map[string]lua.Function{
            "__index":    myIndex,
            "__gc":       myGC,
            "__tostring": myToString,
        }, 0)
    }
    L.SetMetatable(-2)
    return 1
}
```

### Protected Call with Error Handling

```go
status := L.Load(code, "=input", "t")
if status != lua.OK {
    msg, _ := L.ToString(-1)
    log.Printf("compile error: %s", msg)
    L.Pop(1)
    return
}

status = L.PCall(0, lua.MultiRet, 0)
if status != lua.OK {
    msg, _ := L.ToString(-1)
    log.Printf("runtime error: %s", msg)
    L.Pop(1)
}
```

### Driving a Coroutine from Go

```go
co := L.NewThread()
L.Pop(1) // pop the thread from L's stack

// Push function + args onto coroutine's stack
co.GetGlobal("myCoroutineFunc")
co.PushInteger(42)

status, nResults := co.Resume(L, 1)
for status == lua.Yield {
    // Process yielded values (nResults values on co's stack)
    val, _ := co.ToInteger(-1)
    fmt.Println("yielded:", val)
    co.Pop(nResults)

    status, nResults = co.Resume(L, 0)
}
```

### Table Iteration

```go
L.GetGlobal("myTable")
L.PushNil() // first key
for L.Next(-2) {
    key, _ := L.ToString(-2)
    val, _ := L.ToString(-1)
    fmt.Printf("%s = %s\n", key, val)
    L.Pop(1) // pop value, keep key for next iteration
}
L.Pop(1) // pop table
```

### Accessing Upvalues in a Closure

```go
myClosure := func(L *lua.State) int {
    // Access first upvalue (set when PushClosure was called)
    L.PushValue(lua.UpvalueIndex(1))
    counter, _ := L.ToInteger(-1)
    L.Pop(1)

    counter++
    L.PushInteger(counter)
    L.Replace(lua.UpvalueIndex(1)) // update upvalue

    L.PushInteger(counter)
    return 1
}

L.PushInteger(0)               // initial upvalue
L.PushClosure(myClosure, 1)    // 1 upvalue
L.SetGlobal("counter")
```
