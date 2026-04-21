# Debug Hooks — Complete Lifecycle Analysis

## Overview

The Lua debug hook system allows user code to intercept **call**, **return**, **line**, and **count** events during execution. This document traces the complete lifecycle from hook registration through dispatch, covering every C function and its go-lua mapping.

**C Source:** `lua-master/ldo.c` (lines 444–535), `lua-master/ldebug.c` (lines 116–143, 900–978)
**Go Source:** `internal/vm/do.go` (lines 390–535), `internal/state/api.go` (lines 155–172)

---

## 1. Hook Registration: `lua_sethook`

### C Source (ldebug.c:133–143)

```c
LUA_API void lua_sethook (lua_State *L, lua_Hook func, int mask, int count) {
  if (func == NULL || mask == 0) {  /* turn off hooks? */
    mask = 0;
    func = NULL;
  }
  L->hook = func;              // store the hook function pointer
  L->basehookcount = count;    // base count (reset value)
  resethookcount(L);           // L->hookcount = L->basehookcount
  L->hookmask = cast_byte(mask);  // which events to fire
  if (mask)
    settraps(L->ci);           // activate trap on ALL Lua frames
}
```

**WHY each line:**
- **func/mask NULL check**: Passing NULL or mask=0 means "turn off hooks". Both must be zeroed together for consistency.
- **L->hook**: The actual C function pointer called when a hook fires.
- **L->basehookcount**: The "reset value" for count hooks. After each count hook fires, `hookcount` is reset to this value.
- **resethookcount**: Sets `L->hookcount = L->basehookcount`. Without this, a newly-set count hook wouldn't fire until the old countdown expired.
- **L->hookmask**: Bitmask of `LUA_MASKCALL|LUA_MASKRET|LUA_MASKLINE|LUA_MASKCOUNT`.
- **settraps**: Critical — sets `ci->u.l.trap = 1` on ALL active Lua frames. This is what makes the VM check for hooks on every instruction.

### C: `settraps` (ldebug.c:116–120)

```c
static void settraps (CallInfo *ci) {
  for (; ci != NULL; ci = ci->previous)
    if (isLua(ci))
      ci->u.l.trap = 1;
}
```

**WHY**: The VM only calls `luaG_traceexec` when `trap` is set. Without this walk, existing frames would never fire hooks. New frames get `trap` set by `luaD_hookcall`.

### Go Mapping

| C Field | Go Field | Location |
|---------|----------|----------|
| `L->hook` | `L.Hook` (type `any`) | `internal/state/api.go:169` |
| `L->basehookcount` | `L.BaseHookCount` | `internal/state/api.go:170` |
| `L->hookcount` | `L.HookCount` | `internal/state/api.go:171` |
| `L->hookmask` | `L.HookMask` | `internal/state/api.go:172` |
| `L->allowhook` | `L.AllowHook` | `internal/state/api.go:161` |
| `L->oldpc` | `L.OldPC` | `internal/state/api.go:166` |
| `ci->u.l.trap` | `ci.Trap` (bool) | `internal/state/api.go:95` |

**Key difference**: C `trap` is `int` (0/1); Go `Trap` is `bool`. C uses `ci->u.l.trap` (union member); Go uses `ci.Trap` (flat struct field).

---

## 2. The `trap` Flag — When Set, By Whom

The `trap` flag on `CallInfo` is the **gatekeeper** for all hook dispatch. The VM checks it before every instruction.

### Who sets `trap = 1`:

| Who | When | C Location | Go Location |
|-----|------|------------|-------------|
| `lua_sethook` → `settraps` | Hook registration | ldebug.c:143 | N/A (API layer) |
| `luaG_tracecall` | Function entry | ldebug.c:913 | vm.go:1505 (inline) |
| `correctstack` | Stack reallocation | ldo.c:289, 319 | N/A (Go uses indices) |
| `luaD_precall` | New Lua call | ldo.c:738 (via hookcall) | do.go:340 (via CallHook) |

### Who sets `trap = 0`:

| Who | When | C Location |
|-----|------|------------|
| `luaG_traceexec` | No hooks active | ldebug.c:942 |

**WHY trap exists**: Without `trap`, the VM would need to check `hookmask` on every instruction fetch. With `trap`, the fast path (no hooks) has zero overhead — just a single flag check in the instruction dispatch loop.

---

## 3. The Core Dispatcher: `luaD_hook`

### C Source (ldo.c:447–481)

```c
void luaD_hook (lua_State *L, int event, int line,
                              int ftransfer, int ntransfer) {
  lua_Hook hook = L->hook;
  if (hook && L->allowhook) {           // [1] Guard: hook exists + not recursive
    CallInfo *ci = L->ci;
    ptrdiff_t top = savestack(L, L->top.p);      // [2] Save top
    ptrdiff_t ci_top = savestack(L, ci->top.p);  // [3] Save ci->top
    lua_Debug ar;
    ar.event = event;                    // [4] Fill debug info
    ar.currentline = line;
    ar.i_ci = ci;
    L->transferinfo.ftransfer = ftransfer;  // [5] Transfer info for getinfo
    L->transferinfo.ntransfer = ntransfer;
    if (isLua(ci) && L->top.p < ci->top.p)
      L->top.p = ci->top.p;             // [6] Protect activation registers
    luaD_checkstack(L, LUA_MINSTACK);   // [7] Ensure stack space
    if (ci->top.p < L->top.p + LUA_MINSTACK)
      ci->top.p = L->top.p + LUA_MINSTACK;  // [8] Extend ci->top
    L->allowhook = 0;                   // [9] Prevent recursive hooks
    ci->callstatus |= CIST_HOOKED;      // [10] Mark frame as in-hook
    lua_unlock(L);
    (*hook)(L, &ar);                     // [11] CALL THE HOOK
    lua_lock(L);
    lua_assert(!L->allowhook);
    L->allowhook = 1;                   // [12] Re-enable hooks
    ci->top.p = restorestack(L, ci_top);  // [13] Restore ci->top
    L->top.p = restorestack(L, top);      // [14] Restore top
    ci->callstatus &= ~CIST_HOOKED;     // [15] Clear hook mark
  }
}
```

**WHY each section:**
- **[1]**: Double guard — `hook` can be NULL (cleared asynchronously), `allowhook` prevents recursion.
- **[2-3]**: `savestack` converts pointers to offsets — the hook may trigger GC/stack reallocation.
- **[4]**: `lua_Debug` struct passed to hook. `event` = LUA_HOOKCALL/RET/LINE/COUNT.
- **[5]**: `transferinfo` lets `lua_getinfo` report transferred values for call/return hooks.
- **[6]**: If `top < ci->top`, activation registers could be overwritten by hook args. Push top up.
- **[7-8]**: Hook needs `LUA_MINSTACK` (20) slots for its own use.
- **[9]**: `allowhook = 0` — a hook calling code that triggers another hook would cause infinite recursion.
- **[10]**: `CIST_HOOKED` marks this CI so `funcnamefromcall` knows the context.
- **[11]**: The actual hook invocation. In C, this is a function pointer call.
- **[12-15]**: Restore everything. The hook must not permanently affect the caller's state.

### Go Mapping: `hookDispatch` (do.go:390–447)

```go
func hookDispatch(L *stateapi.LuaState, event string, line int) {
    hookVal, ok := L.Hook.(objectapi.TValue)  // [1] Get hook as TValue
    if !ok || hookVal.Tt == objectapi.TagNil { return }
    if !L.AllowHook { return }                // [1] Guard
    ci := L.CI
    savedTop := L.Top                         // [2] Save top
    savedCITop := ci.Top                      // [3] Save ci->top
    L.AllowHook = false                       // [9] Prevent recursion
    ci.CallStatus |= stateapi.CISTHooked      // [10] Mark
    if ci.IsLua() && L.Top < ci.Top {
        L.Top = ci.Top                        // [6] Protect registers
    }
    defer func() {                            // [12-15] Restore
        L.AllowHook = savedAllowHook
        ci.CallStatus &^= stateapi.CISTHooked
        ci.Top = savedCITop
        L.Top = savedTop
    }()
    CheckStack(L, 4)                          // [7] Ensure stack
    // Push hook function + event string + optional line
    L.Stack[L.Top].Val = hookVal; L.Top++
    L.Stack[L.Top].Val = MakeString(event); L.Top++
    if line >= 0 { L.Stack[L.Top].Val = MakeInteger(line); L.Top++ }
    Call(L, L.Top-nargs-1, 0)                 // [11] Call hook
}
```

**Critical differences from C:**
1. **C passes `lua_Debug` struct** to hook as a C pointer. **Go passes event string + line as Lua args** — the hook is called as a regular Lua function with `Call()`.
2. **C uses `transferinfo`** for `lua_getinfo` queries. **Go does not implement `transferinfo`** (`L.FTransfer`/`L.NTransfer` exist but are unused).
3. **C has `lua_unlock/lua_lock`** around hook call (thread safety). Go has no equivalent.
4. **C uses `savestack`/`restorestack`** (pointer→offset for GC safety). Go uses integer indices — no conversion needed.

---

## 4. Call Hook: `luaD_hookcall`

### C Source (ldo.c:487–498)

```c
void luaD_hookcall (lua_State *L, CallInfo *ci) {
  L->oldpc = 0;                              // [1] Reset oldpc for new function
  if (L->hookmask & LUA_MASKCALL) {          // [2] Check if call hook enabled
    int event = (ci->callstatus & CIST_TAIL) ? LUA_HOOKTAILCALL
                                             : LUA_HOOKCALL;  // [3]
    Proto *p = ci_func(ci)->p;
    ci->u.l.savedpc++;                       // [4] Hooks assume pc is incremented
    luaD_hook(L, event, -1, 1, p->numparams);  // [5] Fire hook
    ci->u.l.savedpc--;                       // [6] Correct pc back
  }
}
```

**WHY each line:**
- **[1]**: `oldpc = 0` ensures the first instruction of the new function will always trigger a line hook (if enabled). Without this, stale `oldpc` from the caller could suppress the first line event.
- **[2]**: Even when `hookmask != 0`, call hooks might be off (only line/count active). Must check `LUA_MASKCALL` specifically.
- **[3]**: Tail calls get a different event type so the hook can distinguish them.
- **[4-6]**: The hook protocol assumes `savedpc` points to the NEXT instruction. Since we're at function entry (`savedpc` = first instruction), we increment temporarily. This matters because `lua_getinfo` uses `savedpc` to compute the current line.
- **[5]**: `ftransfer=1, ntransfer=p->numparams` — tells `lua_getinfo` where the function arguments are on the stack.

### Go Mapping: `CallHook` (do.go:457–475)

```go
func CallHook(L *stateapi.LuaState, ci *stateapi.CallInfo) {
    L.OldPC = 0                               // [1]
    if L.HookMask&stateapi.MaskCall == 0 { return }  // [2]
    event := "call"
    if ci.CallStatus&stateapi.CISTTail != 0 { event = "tail call" }  // [3]
    if ci.IsLua() {
        ci.SavedPC++                          // [4]
        hookDispatch(L, event, -1)            // [5]
        ci.SavedPC--                          // [6]
    } else {
        hookDispatch(L, event, -1)
    }
}
```

**Difference**: Go adds an `isLua` check before the savedpc++/-- dance. C doesn't need this because `luaD_hookcall` is only called for Lua functions. Go's `CallHook` can be called for C functions too (the `else` branch).

---

## 5. Return Hook: `rethook`

### C Source (ldo.c:504–535)

```c
static void rethook (lua_State *L, CallInfo *ci, int nres) {
  if (L->hookmask & LUA_MASKRET) {           // [1] Return hook enabled?
    StkId firstres = L->top.p - nres;        // [2] First result
    int delta = 0;
    int ftransfer;
    if (isLua(ci)) {
      Proto *p = ci_func(ci)->p;
      if (p->flag & PF_VAHID)
        delta = ci->u.l.nextraargs + p->numparams + 1;  // [3] Vararg delta
    }
    ci->func.p += delta;                     // [4] Back to virtual 'func'
    ftransfer = cast_int(firstres - ci->func.p);  // [5] Transfer offset
    luaD_hook(L, LUA_HOOKRET, -1, ftransfer, nres);  // [6] Fire hook
    ci->func.p -= delta;                     // [7] Restore func
  }
  if (isLua(ci = ci->previous))              // [8] UNCONDITIONAL: set oldpc
    L->oldpc = pcRel(ci->u.l.savedpc, ci_func(ci)->p);
}
```

**WHY each line:**
- **[1]**: Only fire if `LUA_MASKRET` is in the mask.
- **[2-7]**: Vararg handling. Vararg functions have a hidden prefix on the stack. The hook needs to see the "virtual" function position, not the actual one. `delta` adjusts for this.
- **[5]**: `ftransfer` tells `lua_getinfo` where results start relative to the function slot.
- **[8]**: **CRITICAL** — this line runs even when `LUA_MASKRET` is off! It restores `oldpc` for the **caller's** frame. Without this, `changedline()` in `luaG_traceexec` would see stale `oldpc` and produce wrong line events.

### Go Mapping: `retHook` + PosCall hook block (do.go:366–381, 451–453)

```go
// In PosCall (do.go:366-381):
if L.HookMask != 0 {
    if L.AllowHook && L.HookMask&stateapi.MaskRet != 0 {
        retHook(L, ci, nres)                  // [1,6]
    }
    // UNCONDITIONAL oldpc restore [8]
    if prev := ci.Prev; prev != nil && prev.IsLua() {
        L.OldPC = prev.SavedPC - 1
    }
}

// retHook itself (do.go:451-453):
func retHook(L *stateapi.LuaState, ci *stateapi.CallInfo, nres int) {
    hookDispatch(L, "return", -1)
}
```

**Critical differences:**
1. **Go `retHook` is trivial** — no vararg delta, no transfer info. C's complex vararg adjustment is absent.
2. **Go's oldpc restore uses `SavedPC - 1`** while C uses `pcRel(savedpc, p)`. In Go, `SavedPC` is already an index into `Proto.Code`, so `-1` converts from "next instruction" to "current instruction" (equivalent to `pcRel`).
3. **Go checks `L.AllowHook`** before calling `retHook`, but the oldpc restore is unconditional — matching C behavior.

---

## 6. Count Hook Mechanism

### C Source (ldebug.c:948–950 in `luaG_traceexec`)

```c
counthook = (mask & LUA_MASKCOUNT) && (--L->hookcount == 0);
if (counthook)
    resethookcount(L);  /* reset count */
```

Then later (ldebug.c:961):
```c
if (counthook)
    luaD_hook(L, LUA_HOOKCOUNT, -1, 0, 0);
```

**WHY**: Count hooks fire every N instructions. `hookcount` counts down from `basehookcount`. When it hits 0, fire the hook and reset. The decrement happens in `luaG_traceexec`, which runs on every instruction when `trap` is set.

### Go Mapping (do.go:490–499)

```go
countHook := false
if mask&stateapi.MaskCount != 0 {
    L.HookCount--
    if L.HookCount == 0 {
        L.HookCount = L.BaseHookCount  // reset
        countHook = true
    }
}
```

**Difference**: Structurally identical. Go uses explicit if-chain instead of C's compound expression.

---

## 7. Line Hook Mechanism

### C Source (ldebug.c:963–972 in `luaG_traceexec`)

```c
if (mask & LUA_MASKLINE) {
    int oldpc = (L->oldpc < p->sizecode) ? L->oldpc : 0;  // [1]
    int npci = pcRel(pc, p);                                // [2]
    if (npci <= oldpc ||                                     // [3]
        changedline(p, oldpc, npci)) {                       // [4]
      int newline = luaG_getfuncline(p, npci);
      luaD_hook(L, LUA_HOOKLINE, newline, 0, 0);            // [5]
    }
    L->oldpc = npci;                                         // [6]
}
```

**WHY each line:**
- **[1]**: `oldpc` may be invalid (e.g., after returning from a different function). Clamp to 0 — a wrong but valid value at most causes one extra line hook.
- **[2]**: `npci` = PC relative to proto start = index into code array.
- **[3]**: `npci <= oldpc` means backward jump (loop). ALWAYS fire line hook on backward jumps — this is how loop iterations get line events.
- **[4]**: `changedline` checks if the source line changed between `oldpc` and `npci`.
- **[5]**: Fire line hook with the actual line number.
- **[6]**: Update `oldpc` for next comparison.

### Go Mapping (do.go:507–533)

```go
if npci <= oldpc || GetFuncLine(p, oldpc) != GetFuncLine(p, npci) {
    newline := GetFuncLine(p, npci)
    if newline >= 0 {
        hookDispatch(L, "line", newline)
    }
}
L.OldPC = npci
```

**Critical difference**: Go uses `GetFuncLine(oldpc) != GetFuncLine(npci)` instead of C's `changedline()`. C's `changedline` is an optimized walk through delta-encoded lineinfo; Go calls `GetFuncLine` twice. Functionally equivalent but Go may be slower for large functions. Go also adds a `newline >= 0` guard that C doesn't have.

---

## 8. The `allowhook` Flag — Recursive Hook Prevention

### Lifecycle

```
Normal state:     L->allowhook = 1 (true)
During hook call: L->allowhook = 0 (false)
After hook call:  L->allowhook = 1 (true)
```

**WHY**: If a hook function triggers another hook event (e.g., a line hook calls a function, which triggers a call hook), infinite recursion would occur. `allowhook = 0` during hook execution prevents this.

**Set to false**: `luaD_hook` (ldo.c:468) / `hookDispatch` (do.go:409)
**Restored to true**: `luaD_hook` (ldo.c:474) / `hookDispatch` defer block (do.go:419)

### C vs Go

C checks `L->allowhook` inside `luaD_hook`. Go checks it inside `hookDispatch`. Both are equivalent — the check happens at the single dispatch point.

---

## 9. The Hook CallInfo (C-specific)

### C Behavior

In C, `luaD_hook` does NOT create a new CallInfo for the hook itself. Instead:
1. It marks the CURRENT `ci` with `CIST_HOOKED`
2. Saves/restores `top` and `ci->top` around the hook call
3. The hook function pointer is called directly: `(*hook)(L, &ar)`

The hook receives a `lua_Debug*` pointer, not Lua values on the stack. The hook can then call `lua_getinfo`, `lua_getlocal`, etc. using this pointer.

### Go Behavior

Go's `hookDispatch` calls the hook as a **regular Lua function call** via `Call()`:
1. Pushes the hook function onto the stack
2. Pushes event name string
3. Pushes line number (for line hooks)
4. Calls `Call(L, ...)` which creates a new CallInfo

This is a fundamental architectural difference: C hooks are C function pointer callbacks; Go hooks are Lua-callable values invoked through the standard call machinery.

---

## 10. `luaG_tracecall` — Function Entry Hook Gate

### C Source (ldebug.c:900–921)

```c
int luaG_tracecall (lua_State *L) {
  CallInfo *ci = L->ci;
  Proto *p = ci_func(ci)->p;
  ci->u.l.trap = 1;                          // [1] Ensure hooks checked
  if (ci->u.l.savedpc == p->code) {           // [2] First instruction?
    if (isvararg(p))
      return 0;                               // [3] Vararg: defer to VARARGPREP
    else if (!(ci->callstatus & CIST_HOOKYIELD))
      luaD_hookcall(L, ci);                   // [4] Fire call hook
  }
  return 1;                                   // [5] Keep trap on
}
```

**WHY:**
- **[1]**: Always set trap — even if we don't fire a call hook, line/count hooks may be active.
- **[2]**: Only fire call hook on the FIRST instruction. Resuming after yield should not re-fire.
- **[3]**: Vararg functions start with `OP_VARARGPREP`. The call hook fires AFTER that instruction adjusts arguments. Firing before would give wrong parameter info.
- **[4]**: `CIST_HOOKYIELD` check: if the hook yielded last time, don't call it again on resume.
- **[5]**: Return 1 = keep `trap` active for subsequent line/count hooks.

### Go Mapping (vm.go:1502–1505, inline in VM loop)

Go handles this inline in the VM execution loop rather than as a separate function. The vararg skip and hook-yield check are handled at the `OP_VARARGPREP` instruction.

---

## Difference Table

| Area | C Lua | go-lua | Severity | Notes |
|------|-------|--------|----------|-------|
| Hook type | C function pointer `lua_Hook` | Lua-callable TValue via `Call()` | **HIGH** | Different calling convention; Go hooks are Lua functions |
| Debug info | `lua_Debug` struct passed to hook | Event string + line as Lua args | **HIGH** | Go hooks can't call `lua_getinfo` on the debug struct |
| `transferinfo` | `ftransfer`/`ntransfer` set for getinfo | Fields exist but unused | **MEDIUM** | Affects `lua_getinfo` "u" option for call/return hooks |
| `trap` type | `int` in CI union (`u.l.trap`) | `bool` in flat struct (`ci.Trap`) | **LOW** | Semantically identical |
| Vararg return hook | Complex delta adjustment | Not implemented in `retHook` | **MEDIUM** | May affect hook info for vararg function returns |
| `changedline` | Optimized delta walk | `GetFuncLine` called twice | **LOW** | Performance only; functionally equivalent |
| `CIST_HOOKYIELD` | Checked in `luaG_traceexec` | `CISTHookYield` defined but not checked in `TraceExec` | **HIGH** | Go may re-fire hooks after yield incorrectly |
| `luaP_isIT` top fix | Corrects `top` if instruction doesn't use top | Not implemented | **MEDIUM** | May leave `top` wrong during hook calls |
| Stack save/restore | `savestack`/`restorestack` (ptr→offset) | Direct index save/restore | **NONE** | Go uses indices natively |
| `settraps` on registration | Walks all CIs setting trap=1 | Not visible in analyzed code | **MEDIUM** | Must exist in Go's sethook equivalent |
| `correctstack` sets trap | Stack realloc sets all traps | N/A (Go uses indices) | **NONE** | Not needed in Go |

---

## Verification Methods

1. **Hook firing order**: `debug.sethook(fn, "crl")` → call function → verify events = call→line(s)→return
2. **Count hook**: Set count=5, run 100-iteration loop, verify hook fired ~20 times
3. **Recursive prevention**: Inside hook, trigger events — verify `allowhook` prevents re-entry
4. **Tail call event**: `return g()` should produce "tail call" event, not "call"
5. **OldPC after return**: Line hooks must fire on line after `f()` returns (tests oldpc restoration)

### Source Verification
```bash
grep -n 'LUA_MASKCALL\|LUA_MASKRET\|LUA_MASKLINE\|LUA_MASKCOUNT' lua-master/lua.h
grep -n 'MaskCall\|MaskRet\|MaskLine\|MaskCount' internal/state/api.go
grep -n 'CIST_HOOKED\|CIST_TAIL\|CIST_HOOKYIELD' lua-master/lstate.h
grep -n 'CISTHooked\|CISTTail\|CISTHookYield' internal/state/api.go
grep -n 'HookYield\|HOOKYIELD' internal/vm/do.go  # Verify CIST_HOOKYIELD checked
```
