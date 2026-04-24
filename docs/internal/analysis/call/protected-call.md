# Protected Calls: luaD_pcall & luaD_rawrunprotected

## Overview

Protected calls let Lua execute code that might error without crashing the host.
In C Lua, this uses `setjmp`/`longjmp`. In go-lua, it uses Go's `panic`/`recover`.

The architecture has two layers:
1. **`luaD_rawrunprotected`** — low-level: establishes an error-catching frame
2. **`luaD_pcall`** — high-level: saves/restores state, calls rawrunprotected, handles cleanup

The API entry point `lua_pcallk` (C) / `State.PCall` (Go) adds argument handling
and the two-path yieldable/non-yieldable split.

---

## C Source Analysis

### 1. Error Recovery Primitives (ldo.c:55-110)

#### lua_longjmp — Chained Jump Buffers (ldo.c:61-65)

```c
typedef struct lua_longjmp {
  struct lua_longjmp *previous;   // chain → forms a STACK of handlers
  jmp_buf b;                      // platform jump buffer
  volatile TStatus status;        // error code (volatile: survives longjmp)
} lua_longjmp;
```

**WHY chained?** Each `rawrunprotected` call pushes a handler. Nested protected
calls create a chain. `luaD_throw` jumps to the innermost (most recent) handler.
The chain is a stack implemented as a linked list on the C stack.

#### LUAI_THROW / LUAI_TRY (ldo.c:74-109)

Three platform variants, all semantically identical:

| Platform | THROW | TRY |
|----------|-------|-----|
| C++ | `throw(c)` | `try { f(L,ud); } catch(...)` |
| POSIX | `_longjmp(c->b, 1)` | `if (_setjmp(c->b) == 0) f(L,ud)` |
| ISO C | `longjmp(c->b, 1)` | `if (setjmp(c->b) == 0) f(L,ud)` |

**WHY `_setjmp` on POSIX?** It skips saving/restoring signal masks — faster.
The `volatile` on `status` is critical: without it, the compiler could optimize
away the status write that happens between `setjmp` and `longjmp`.

### 2. luaD_seterrorobj (ldo.c:112-122)

```c
void luaD_seterrorobj (lua_State *L, TStatus errcode, StkId oldtop) {
  if (errcode == LUA_ERRMEM)
    setsvalue2s(L, oldtop, G(L)->memerrmsg);  // pre-registered string
  else
    setobjs2s(L, oldtop, L->top.p - 1);       // move error from top
  L->top.p = oldtop + 1;  // stack = [oldtop: error_obj]
}
```

**WHY special-case ERRMEM?** Memory errors can't allocate a new error string.
C Lua pre-creates `memerrmsg` ("not enough memory") at state creation.

**WHY `oldtop + 1`?** Restores the stack to exactly: old function position + error object.
This is the contract: after pcall failure, stack has exactly one value (the error).

### 3. luaD_throw (ldo.c:125-147)

```c
l_noret luaD_throw (lua_State *L, TStatus errcode) {
  if (L->errorJmp) {                         // (line 126) handler exists?
    L->errorJmp->status = errcode;            // (line 127) set error code
    LUAI_THROW(L, L->errorJmp);              // (line 128) longjmp!
  }
  else {                                      // (line 130) no handler
    global_State *g = G(L);
    lua_State *mainth = mainthread(g);
    errcode = luaE_resetthread(L, errcode);   // (line 133) close upvalues
    L->status = errcode;                      // (line 134) mark thread dead
    if (mainth->errorJmp) {                   // (line 135) main has handler?
      setobjs2s(L, mainth->top.p++, L->top.p - 1);  // copy error
      luaD_throw(mainth, errcode);            // (line 137) re-throw on main
    }
    else {
      if (g->panic) g->panic(L);             // (line 140) last resort
      abort();                                // (line 142) truly fatal
    }
  }
}
```

**WHY three escalation levels?**
1. Local handler → longjmp to nearest `rawrunprotected`
2. No local handler → thread is dead, propagate to main thread
3. No handler anywhere → panic callback, then abort

### 4. luaD_rawrunprotected (ldo.c:160-170)

```c
TStatus luaD_rawrunprotected (lua_State *L, Pfunc f, void *ud) {
  l_uint32 oldnCcalls = L->nCcalls;          // (line 161) save C call depth
  lua_longjmp lj;
  lj.status = LUA_OK;                        // (line 163) assume success
  lj.previous = L->errorJmp;                 // (line 164) chain handler
  L->errorJmp = &lj;                         // (line 165) install handler
  LUAI_TRY(L, &lj, f, ud);                   // (line 166) setjmp + call f
  L->errorJmp = lj.previous;                 // (line 167) unchain handler
  L->nCcalls = oldnCcalls;                   // (line 168) restore depth
  return lj.status;                          // (line 169) OK or error code
}
```

**WHY save/restore nCcalls?** If an error occurs deep in nested C calls,
the longjmp skips all intermediate frames. Without restoration, `nCcalls`
would be permanently incremented, eventually triggering stack overflow.

**WHY is `lj` on the C stack?** The `setjmp` saves the C stack frame.
When `longjmp` fires, it restores this frame — `lj` is still alive.
After the jump, `lj.status` contains the error code set by `luaD_throw`.

### 5. luaD_pcall (ldo.c:1089-1106)

```c
TStatus luaD_pcall (lua_State *L, Pfunc func, void *u,
                    ptrdiff_t old_top, ptrdiff_t ef) {
  TStatus status;
  CallInfo *old_ci = L->ci;                   // (1093) save call chain
  lu_byte old_allowhooks = L->allowhook;      // (1094) save hook permission
  ptrdiff_t old_errfunc = L->errfunc;         // (1095) save error handler
  L->errfunc = ef;                            // (1096) install new handler
  status = luaD_rawrunprotected(L, func, u);  // (1097) EXECUTE
  if (l_unlikely(status != LUA_OK)) {         // (1098) error occurred?
    L->ci = old_ci;                           // (1099) restore call chain
    L->allowhook = old_allowhooks;            // (1100) restore hooks
    status = luaD_closeprotected(L, old_top, status);  // (1101) close TBC
    luaD_seterrorobj(L, status, restorestack(L, old_top));  // (1102) set err
    luaD_shrinkstack(L);                      // (1103) shrink if overflow
  }
  L->errfunc = old_errfunc;                   // (1105) ALWAYS restore errfunc
  return status;
}
```

**WHY save `old_ci`?** On error, the longjmp skips all `luaD_poscall` calls
that would normally unwind the call chain. Manual restoration is required.

**WHY save `allowhook`?** Hooks are disabled during hook execution. If an error
occurs inside a hook, `allowhook` would be stuck at false without restoration.

**WHY `closeprotected` before `seterrorobj`?** To-be-closed variables must run
their `__close` methods. These methods might themselves error, so they run in
a protected loop. Only after all closes does the final error get placed on stack.

**WHY `shrinkstack`?** Stack overflow errors grow the stack. After recovery,
shrink it back to prevent permanent memory bloat.

**WHY is `errfunc` restored unconditionally (outside the if)?** Even on success,
the error handler must be restored to the caller's handler.

### 6. luaD_closeprotected (ldo.c:1067-1081)

```c
TStatus luaD_closeprotected (lua_State *L, ptrdiff_t level, TStatus status) {
  CallInfo *old_ci = L->ci;
  lu_byte old_allowhooks = L->allowhook;
  for (;;) {
    struct CloseP pcl;
    pcl.level = restorestack(L, level);
    pcl.status = status;
    status = luaD_rawrunprotected(L, &closepaux, &pcl);
    if (l_likely(status == LUA_OK))
      return pcl.status;            // all closed successfully
    else {
      L->ci = old_ci;              // __close errored — restore and retry
      L->allowhook = old_allowhooks;
    }
  }
}
```

**WHY a loop?** Each `__close` method runs protected. If one errors, the new
error replaces the old, and the loop continues closing remaining variables.
This ensures ALL to-be-closed variables get closed even if some error.

### 7. lua_pcallk — The API Entry Point (lapi.c:1076-1118)

Two paths based on yieldability:

**Path A (non-yieldable):** `luaD_pcall(L, f_call, &c, savestack(L, c.func), func)`
- Standard protected call with setjmp/longjmp error recovery.

**Path B (yieldable with continuation):** `luaD_call(L, c.func, nresults)`
- UNPROTECTED call! Error recovery happens at the `lua_resume` level.
- Saves continuation `k`, context `ctx`, `funcidx`, `old_errfunc`, `allowhook` on the CI.
- Sets `CIST_YPCALL` flag so `precover` (in resume) knows to run `finishpcallk`.

**WHY two paths?** Path B allows yields inside pcall. If the called function
yields, the continuation `k` resumes it later. Errors propagate up to `resume`'s
`rawrunprotected`, where `precover` handles them using the saved state.

---

## Go Implementation Mapping

### RunProtected (do.go:674-710)

```go
func RunProtected(L *stateapi.LuaState, f func()) (status int) {
    oldNCCalls := L.NCCalls
    status = stateapi.StatusOK
    defer func() {
        if r := recover(); r != nil {
            switch e := r.(type) {
            case stateapi.LuaBaseLevel:
                panic(e)                    // re-panic for base-level throws
            case stateapi.LuaError:
                status = e.Status
                L.NCCalls = oldNCCalls
            case stateapi.LuaYield:
                status = stateapi.StatusYield
                L.NCCalls = oldNCCalls
            case *lexapi.SyntaxError:
                // ... push error string, set StatusErrSyntax
            default:
                panic(r)                    // non-Lua errors propagate
            }
        }
    }()
    f()
    return status
}
```

**Key mapping:** C's `lua_longjmp` chain → Go's `defer`/`recover` stack.
Each `RunProtected` call creates a `defer` frame that catches panics.
Go's runtime manages the "chain" implicitly — nested `recover()` calls
catch at the nearest `defer`, just like nested `setjmp` catch at the nearest buffer.

### PCall (do.go:751-810)

```go
func PCall(L *stateapi.LuaState, funcIdx int, nResults int, errFunc int) int {
    oldCI := L.CI
    oldAllowHook := L.AllowHook
    oldErrFunc := L.ErrFunc
    oldTop := funcIdx                         // C: savestack(L, c.func)

    L.ErrFunc = errFunc

    // PATH B: Yieldable with continuation
    if L.Yieldable() && oldCI.K != nil {
        oldCI.CallStatus |= stateapi.CISTYPCall
        oldCI.FuncIdx = funcIdx
        oldCI.OldErrFunc = oldErrFunc
        Call(L, funcIdx, nResults)             // PLAIN call
        oldCI.CallStatus &^= stateapi.CISTYPCall
        L.ErrFunc = oldErrFunc
        return stateapi.StatusOK
    }

    // PATH A: Non-yieldable — RunProtected
    status := RunProtected(L, func() {
        Call(L, funcIdx, nResults)
    })

    if status != stateapi.StatusOK {
        L.CI = oldCI
        L.AllowHook = oldAllowHook
        // Close TBC vars
        if L.TBCList >= oldTop {
            status, errObj = CloseProtected(L, oldTop, status, errObj)
        }
        SetErrorObj(L, status, oldTop)
        ShrinkStack(L)
    }
    L.ErrFunc = oldErrFunc
    return status
}
```

### State.PCall — API Layer (impl.go:886-900)

```go
func (L *State) PCall(nArgs, nResults, msgHandler int) int {
    ls := L.ls()
    funcIdx := ls.Top - nArgs - 1
    errFunc := 0
    if msgHandler != 0 {
        errFunc = L.index2stack(msgHandler)
    }
    status := vmapi.PCall(ls, funcIdx, nResults, errFunc)
    // Ensure Top >= CI.Func + 1
    base := ls.CI.Func + 1
    if ls.Top < base { ls.Top = base }
    return status
}
```

### Throw (do.go:44-46)

```go
func Throw(L *stateapi.LuaState, status int) {
    panic(stateapi.LuaError{Status: status})
}
```

C's `luaD_throw` has three escalation levels. Go's `Throw` always panics.
The escalation (thread → main thread → abort) is handled differently:
`LuaBaseLevel` panic type propagates past inner `recover()` frames.

### SetErrorObj (do.go:50-68)

Maps directly to C's `luaD_seterrorobj`. Same ERRMEM special case using
pre-registered `MemErrMsg`. Same `Top = oldtop + 1` contract.

### CloseProtected (do.go:1375-1440)

More complex than C due to `LuaBaseLevel` handling. Uses nested
`func() { defer recover()... { RunProtected(...) } }()` to catch
base-level panics that `RunProtected` re-panics.

---

## Difference Table

| Area | C Lua | go-lua | Severity | Notes |
|------|-------|--------|----------|-------|
| Error mechanism | `setjmp`/`longjmp` | `panic`/`recover` | 🟢 Low | Semantically equivalent; Go's is type-safe |
| Handler chain | Explicit `lua_longjmp` linked list | Implicit `defer` stack | 🟢 Low | Go runtime manages the chain |
| Error types | Single `TStatus` integer | Typed panics: `LuaError`, `LuaYield`, `LuaBaseLevel`, `*SyntaxError` | 🟡 Medium | Go distinguishes error kinds at catch site |
| Base-level throw | `luaD_throwbaselevel` unwinds chain then longjmps | `LuaBaseLevel` panic re-panicked by `RunProtected` | 🟡 Medium | Different mechanism, same effect — reaches outermost handler |
| `nCcalls` save | `l_uint32` saved/restored | Same field, same logic | 🟢 Low | Direct mapping |
| `nny` (non-yieldable) | Encoded in `nCcalls` upper bits | Encoded in `NCCalls` upper bits | 🟢 Low | Same encoding scheme |
| `old_top` type | `ptrdiff_t` (byte offset from stack base) | `int` (direct index) | 🟡 Medium | Go uses direct indices — no `savestack`/`restorestack` needed |
| Stack reallocation safety | `StkIdRel` + `restorestack` after any realloc | Direct indices always valid (Go slice realloc preserves indices) | 🟢 Low | Go's model is simpler |
| `closeprotected` loop | `for(;;)` with `rawrunprotected` | `for` loop with nested `func()/defer/recover` | 🟡 Medium | Go adds `LuaBaseLevel` catch layer |
| `errfunc` type | `ptrdiff_t` (byte offset) | `int` (stack index) | 🟢 Low | Same concept, different representation |
| Path B yield handling | `CIST_YPCALL` + `luaD_call` | Same: `CISTYPCall` + `Call` | 🟢 Low | Direct mapping |
| `SyntaxError` handling | Part of normal error flow | Separate panic type caught in `RunProtected` | 🟡 Medium | Go catches parser errors distinctly |
| `shrinkstack` | Called on error path | Same | 🟢 Low | Direct mapping |
| TBC close with error obj | `closepaux` passes status | `CloseProtected` passes both status and errObj | 🟡 Medium | Go threads error object explicitly |

---

## Verification Methods

### 1. Basic pcall error recovery
```lua
local ok, err = pcall(function() error("test") end)
assert(not ok and err:find("test"))
```
Verifies: `RunProtected` catches error, `SetErrorObj` places it correctly.

### 2. Nested pcall
```lua
local ok1, err1 = pcall(function()
    local ok2, err2 = pcall(function() error("inner") end)
    assert(not ok2)
    error("outer")
end)
assert(not ok1 and err1:find("outer"))
```
Verifies: handler chain nesting works (multiple `defer`/`recover` frames).

### 3. Error handler (message handler)
```lua
local ok, err = xpcall(function() error("raw") end,
    function(e) return "handled: " .. e end)
assert(not ok and err:find("handled"))
```
Verifies: `errfunc` is called to transform the error message.

### 4. Stack restoration after error
```lua
local before = select("#", 1, 2, 3)  -- push some values
local ok = pcall(function()
    local t = {}
    for i = 1, 1000 do t[i] = i end  -- grow stack
    error("boom")
end)
-- Stack should be restored to old_top + 1 (error object)
```
Verifies: `SetErrorObj` restores `Top = oldtop + 1`.

### 5. allowhook restoration
```lua
-- Set a hook, trigger error inside hook callback
-- After pcall recovery, hooks should still fire
debug.sethook(function() end, "l")
pcall(function() error("in hook test") end)
-- Hook should still be active (allowhook restored)
```

### 6. To-be-closed variable cleanup
```lua
local closed = false
local ok = pcall(function()
    local x <close> = setmetatable({}, {__close = function() closed = true end})
    error("boom")
end)
assert(not ok and closed)
```
Verifies: `CloseProtected` runs `__close` methods on error path.

### 7. Memory error uses pre-registered message
Test by exhausting memory allocation — error message should be "not enough memory"
(the pre-registered `memerrmsg`/`MemErrMsg`).

### 8. C function pcall (Path B — yieldable)
```lua
-- In a coroutine with continuation, pcall uses plain Call
-- Error recovery happens at resume level via precover/finishPCallK
```
Verifies: `CISTYPCall` path works correctly in coroutine context.
