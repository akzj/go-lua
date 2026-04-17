# luaD_precall — Function Call Preparation

## Overview

`luaD_precall` is the gateway for **every** function call in Lua. It inspects the
value at the call slot, determines whether it is a C function or a Lua closure,
and dispatches accordingly:

- **C function path** → creates a CI, executes the C function immediately,
  calls `luaD_poscall`, and returns `NULL`.
- **Lua function path** → creates a CI, sets up the frame (base, top, savedpc),
  pads missing arguments with nil, and returns the new `CallInfo*` so the
  caller (`ccall`) can invoke `luaV_execute`.
- **Neither** → tries the `__call` metamethod via `tryfuncTM` and retries.

The Go equivalent is `PreCall` in `internal/vm/api/do.go:316`.

---

## C Source Analysis

### Helper: `next_ci` (ldo.c:627)

```c
#define next_ci(L)  (L->ci->next ? L->ci->next : luaE_extendCI(L, 1))
```

**WHY**: CallInfo nodes form a **reusable linked list**. When a call returns,
the CI is not freed — it stays in the list for the next call. `next_ci` first
checks if there is already an allocated-but-unused node (`L->ci->next`). Only
if not does it allocate via `luaE_extendCI`. This avoids malloc/free churn on
every call/return cycle.

### Helper: `prepCallInfo` (ldo.c:636-645)

```c
l_sinline CallInfo *prepCallInfo (lua_State *L, StkId func, unsigned status,
                                                StkId top) {
  CallInfo *ci = L->ci = next_ci(L);  /* new frame */
  ci->func.p = func;
  lua_assert((status & ~(CIST_NRESULTS | CIST_C | MAX_CCMT)) == 0);
  ci->callstatus = status;
  ci->top.p = top;
  return ci;
}
```

**WHY each line**:
1. `L->ci = next_ci(L)` — push a new frame onto the call chain. `L->ci`
   always points to the **current** frame.
2. `ci->func.p = func` — record which stack slot holds the function value.
   This is a `StkIdRel` (offset-relative pointer; see §StkIdRel below).
3. `ci->callstatus = status` — the low 8 bits encode `nresults+1`. Higher
   bits may have `CIST_C` (for C functions) or `MAX_CCMT` bits (metamethod
   depth counter). The assert verifies no stray bits leak in.
4. `ci->top.p = top` — set the frame's stack ceiling. For Lua functions this
   is `func + 1 + maxstacksize`; for C functions it is `L->top + LUA_MINSTACK`.

### Helper: `precallC` (ldo.c:650-670)

```c
l_sinline int precallC (lua_State *L, StkId func, unsigned status,
                                            lua_CFunction f) {
  int n;
  CallInfo *ci;
  checkstackp(L, LUA_MINSTACK, func);          // [1]
  L->ci = ci = prepCallInfo(L, func,
                 status | CIST_C,               // [2]
                 L->top.p + LUA_MINSTACK);      // [3]
  lua_assert(ci->top.p <= L->stack_last.p);
  if (l_unlikely(L->hookmask & LUA_MASKCALL)) { // [4]
    int narg = cast_int(L->top.p - func) - 1;
    luaD_hook(L, LUA_HOOKCALL, -1, 1, narg);
  }
  lua_unlock(L);
  n = (*f)(L);                                   // [5]
  lua_lock(L);
  api_checknelems(L, n);
  luaD_poscall(L, ci, n);                        // [6]
  return n;
}
```

**WHY each line**:
1. Guarantee `LUA_MINSTACK` (20) free slots — C functions may push values.
2. `CIST_C` flag marks this as a C frame. The union `u.c` fields become valid.
3. Top is set to `L->top + LUA_MINSTACK` — generous ceiling for C code.
4. Fire call hook **before** the C function runs (if mask is set).
5. Actually call the C function. It returns the number of results it pushed.
6. `luaD_poscall` moves results and pops the CI.

**Key**: For C functions, `precallC` does the **entire** call cycle (create CI →
run function → poscall). The caller sees `NULL` returned from `luaD_precall`,
meaning "already handled."

### Main: `luaD_precall` (ldo.c:723-754)

```c
CallInfo *luaD_precall (lua_State *L, StkId func, int nresults) {
  unsigned status = cast_uint(nresults + 1);     // [1]
  lua_assert(status <= MAXRESULTS + 1);
 retry:
  switch (ttypetag(s2v(func))) {
    case LUA_VCCL:                                // [2a]
      precallC(L, func, status, clCvalue(s2v(func))->f);
      return NULL;
    case LUA_VLCF:                                // [2b]
      precallC(L, func, status, fvalue(s2v(func)));
      return NULL;
    case LUA_VLCL: {                              // [3]
      CallInfo *ci;
      Proto *p = clLvalue(s2v(func))->p;
      int narg = cast_int(L->top.p - func) - 1;  // [3a]
      int nfixparams = p->numparams;
      int fsize = p->maxstacksize;                // [3b]
      checkstackp(L, fsize, func);                // [3c]
      L->ci = ci = prepCallInfo(L, func, status,
                                func + 1 + fsize); // [3d]
      ci->u.l.savedpc = p->code;                  // [3e]
      for (; narg < nfixparams; narg++)            // [3f]
        setnilvalue(s2v(L->top.p++));
      lua_assert(ci->top.p <= L->stack_last.p);
      return ci;                                   // [3g]
    }
    default: {                                     // [4]
      checkstackp(L, 1, func);
      status = tryfuncTM(L, func, status);
      goto retry;
    }
  }
}
```

**WHY each line**:
1. `nresults + 1` encoding: the low 8 bits of `callstatus` store `nresults+1`
   so that `LUA_MULTRET` (-1) maps to 0, and 0 results maps to 1. This avoids
   special-casing -1 everywhere.
2. C closures (2a) and light C functions (2b) both call `precallC`. The only
   difference is how the function pointer `f` is extracted.
3. **Lua function path** — the critical path:
   - **(3a)** `narg = top - func - 1`: count real arguments (everything between
     the function slot and L->top).
   - **(3b)** `fsize = p->maxstacksize`: the compiler pre-computed how many
     registers this function needs.
   - **(3c)** `checkstackp`: ensure the stack can hold `fsize` slots. May
     reallocate the stack (invalidating all `StkId` pointers — see §StkIdRel).
   - **(3d)** `prepCallInfo(... func + 1 + fsize)`: the CI's `top` is set to
     `func + 1 + fsize`. The `+1` skips the function slot itself; `fsize`
     covers all registers. `base` is implicitly `func + 1` (not stored in CI
     in Lua 5.5 — it's computed as `ci->func.p + 1`).
   - **(3e)** `savedpc = p->code`: point to the first instruction. The VM will
     read `*savedpc` and advance.
   - **(3f)** Pad missing arguments with nil. If the caller passed fewer args
     than the function declares, fill the gap. `L->top.p++` for each nil
     ensures the stack is consistent.
   - **(3g)** Return the CI — the caller (`ccall`) will call `luaV_execute`.
4. **`__call` metamethod**: if the value isn't a function, look up `__call`.
   `tryfuncTM` shifts the stack, inserts the metamethod at `func`, increments
   the CCMT counter (to detect infinite `__call` chains), and we `goto retry`.

### Caller: `ccall` (ldo.c:765-780)

```c
l_sinline void ccall (lua_State *L, StkId func, int nResults, l_uint32 inc) {
  CallInfo *ci;
  L->nCcalls += inc;                              // [1]
  if (l_unlikely(getCcalls(L) >= LUAI_MAXCCALLS)) {
    checkstackp(L, 0, func);
    luaE_checkcstack(L);                          // [2]
  }
  if ((ci = luaD_precall(L, func, nResults)) != NULL) {
    ci->callstatus |= CIST_FRESH;                // [3]
    luaV_execute(L, ci);                          // [4]
  }
  L->nCcalls -= inc;                              // [5]
}
```

**WHY each line**:
1. `nCcalls` tracks C-stack depth. `inc` is 1 for normal calls, `nyci` for
   non-yieldable calls (which also increments the non-yieldable counter in
   the upper 16 bits).
2. If at the limit, check and potentially raise "C stack overflow."
3. `CIST_FRESH` marks this as a fresh entry into `luaV_execute`. The VM loop
   uses this to know when to stop (it exits when it pops back past the
   CIST_FRESH frame).
4. Execute the Lua function's bytecode.
5. Decrement on return. Symmetric with [1].

---

## StkIdRel: Offset-Based Stack Pointers

In C Lua 5.5, `StkId` is `StackValue*` (a raw pointer into the stack array).
Because `luaD_reallocstack` can move the array, C Lua introduced `StkIdRel`:

```c
typedef struct {
  StkId p;          /* actual pointer */
  ptrdiff_t offset; /* byte offset from stack base */
} StkIdRel;
```

Functions like `savestack`/`restorestack` convert between pointer and offset.
Every `checkstackp` call includes a `func` parameter so that after a potential
reallocation, `func` can be restored.

**In go-lua**: The stack is `[]StackValue` (a Go slice). Reallocation creates a
new backing array, but all references use **integer indices** (not pointers).
This means `StkIdRel` is unnecessary — indices survive reallocation naturally.
`ci.Func`, `ci.Top`, `L.Top` are all `int` indices.

---

## CallInfo Union Flattening

C Lua uses a union inside CallInfo to share memory between Lua and C fields:

```c
union {
  struct { const Instruction *savedpc; l_signalT trap; int nextraargs; } l;
  struct { lua_KFunction k; ptrdiff_t old_errfunc; lua_KContext ctx; } c;
} u;
```

**In go-lua** (`internal/state/api/api.go:87-108`), there is no union — all
fields coexist in a flat struct:

```go
type CallInfo struct {
    Func int; Top int; Prev *CallInfo; Next *CallInfo
    // Lua fields
    SavedPC int; Trap bool; NExtraArgs int
    // C fields
    K KFunction; OldErrFunc int; Ctx int
    // Ephemeral
    NYield int; NRes int; FuncIdx int
    CallStatus uint32
}
```

This wastes ~40 bytes per CI but eliminates unsafe pointer casts. The `CISTC`
flag in `CallStatus` determines which fields are semantically valid.

---

## Go Implementation Mapping

### `nextCI` → `next_ci` (do.go:258)

```go
func nextCI(L *stateapi.LuaState) *stateapi.CallInfo {
    if L.CI.Next != nil { return L.CI.Next }
    return stateapi.NewCI(L)
}
```

Identical logic. `stateapi.NewCI` (state.go:234) allocates a new `CallInfo`,
links it, and increments `L.NCI`.

### `prepCallInfo` → `prepCallInfo` (do.go:264)

```go
func prepCallInfo(L *stateapi.LuaState, funcIdx int, status uint32, top int) *stateapi.CallInfo {
    ci := nextCI(L)
    L.CI = ci
    ci.Func = funcIdx
    ci.CallStatus = status
    ci.Top = top
    return ci
}
```

Direct 1:1 mapping. Uses `int` indices instead of `StkId` pointers.

### `precallC` → `precallC` (do.go:275)

```go
func precallC(L *stateapi.LuaState, funcIdx int, status uint32, f stateapi.CFunction) int {
    CheckStack(L, luaMinStack)
    ci := prepCallInfo(L, funcIdx, status|stateapi.CISTC, L.Top+luaMinStack)
    if L.HookMask&stateapi.MaskCall != 0 {
        CallHook(L, ci)
    }
    n := f(L)
    PosCall(L, ci, n)
    return n
}
```

**Differences from C**:
- No `lua_unlock`/`lua_lock` (Go has no global interpreter lock).
- No `api_checknelems` assertion (Go relies on slice bounds checks).
- Hook check uses `MaskCall` directly, not `l_unlikely` hint.

### `PreCall` → `luaD_precall` (do.go:316)

```go
func PreCall(L *stateapi.LuaState, funcIdx int, nResults int) *stateapi.CallInfo {
    status := uint32(nResults + 1)
retry:
    fval := L.Stack[funcIdx].Val
    switch fval.Tt {
    case objectapi.TagLuaClosure:
        // ... Lua path (identical logic) ...
        ci := prepCallInfo(L, funcIdx, status, funcIdx+1+fsize)
        ci.SavedPC = 0  // starting point (index into Proto.Code)
        for ; narg < nfixparams; narg++ {
            L.Stack[L.Top].Val = objectapi.Nil
            L.Top++
        }
        if L.HookMask != 0 { CallHook(L, ci) }  // [*]
        return ci
    case objectapi.TagCClosure:
        precallC(L, funcIdx, status, cc.Fn)
        return nil
    case objectapi.TagLightCFunc:
        precallC(L, funcIdx, status, f)
        return nil
    default:
        CheckStack(L, 1)
        status = TryFuncTM(L, funcIdx, status)
        goto retry
    }
}
```

**Key difference [*]**: Go's `PreCall` calls `CallHook` for Lua functions
**inside** `PreCall` itself, whereas C Lua calls `luaD_hookcall` from the VM
loop entry (`luaV_execute`). This is a structural difference — the hook fires
at the same logical point (before first instruction) but from different call
sites.

### `Call` → `ccall` / `luaD_call` (do.go:625)

```go
func Call(L *stateapi.LuaState, funcIdx int, nResults int) {
    L.NCCalls++
    if L.CCalls() >= stateapi.MaxCCalls { /* overflow check */ }
    ci := PreCall(L, funcIdx, nResults)
    if ci != nil {
        ci.CallStatus |= stateapi.CISTFresh
        Execute(L, ci)
    }
    L.NCCalls--
}
```

Direct mapping. `inc` parameter is replaced by separate `Call` vs `CallNoYield`
functions. `CallNoYield` (do.go:650) increments `NCCalls` by `0x00010001`
(both counters packed into a single uint32).

### `TryFuncTM` → `tryfuncTM` (do.go:292)

```go
func TryFuncTM(L *stateapi.LuaState, funcIdx int, status uint32) uint32 {
    tm := mmapi.GetTMByObj(L.Global, L.Stack[funcIdx].Val, mmapi.TM_CALL)
    if tm.IsNil() {
        // Inlined error message (C calls luaG_callerror separately)
        typeName := objectapi.TypeNames[L.Stack[funcIdx].Val.Type()]
        extra := callErrorExtra(L, funcIdx)
        RunError(L, "attempt to call a "+typeName+" value"+extra)
    }
    for p := L.Top; p > funcIdx; p-- {
        L.Stack[p].Val = L.Stack[p-1].Val
    }
    L.Top++
    L.Stack[funcIdx].Val = tm
    if status&MaxCCMT == MaxCCMT { RunError(L, "'__call' chain too long") }
    return status + (1 << stateapi.CISTCCMTShift)
}
```

**Difference**: Go inlines the error message construction (`callErrorExtra`)
rather than calling a separate `luaG_callerror`. Functionally identical.

---

## Difference Table

| Area | C Lua (ldo.c) | go-lua (do.go) | Severity | Notes |
|---|---|---|---|---|
| Stack pointers | `StkIdRel` with `savestack`/`restorestack` | `int` indices | **None** | Go slices survive realloc; indices are stable |
| CI union | `u.l` / `u.c` union (shared memory) | Flat struct (all fields coexist) | **Low** | Wastes ~40 bytes/CI; no correctness impact |
| `savedpc` type | `const Instruction*` (pointer into code array) | `int` (index into `Proto.Code`) | **None** | Semantically equivalent |
| Hook call site | `luaD_hookcall` called from `luaV_execute` entry | `CallHook` called inside `PreCall` | **Medium** | Same logical point, different call site. If `PreCall` is called without going through `Call`, hook still fires. |
| `lua_lock`/`unlock` | Surrounds C function call | Absent | **None** | Go has no GIL equivalent |
| `api_checknelems` | Asserts C function didn't return too many | Absent | **Low** | Go relies on slice bounds; no explicit check |
| `__call` error | Calls `luaG_callerror` | Inlines error string construction | **None** | Same error message produced |
| `nCcalls` encoding | `inc` param (1 or `nyci`) | Separate `Call`/`CallNoYield` functions | **None** | Same packed-counter approach |
| `checkstackp` func param | Passes `func` StkId for restore after realloc | Not needed (indices stable) | **None** | Fundamental Go advantage |
| `CIST_FRESH` | Set in `ccall` after `luaD_precall` returns | Set in `Call` after `PreCall` returns | **None** | Identical placement |

---

## Verification Methods

### 1. Confirm nresults encoding
```lua
-- Call with 0, 1, LUA_MULTRET results and verify callstatus low bits
local function f() return 1, 2, 3 end
f()           -- nresults=0 → status=1
local x = f() -- nresults=1 → status=2
print(f())    -- nresults=MULTRET → status=0
```

### 2. Confirm nil-padding of missing arguments
```lua
local function f(a, b, c) return a, b, c end
assert(select(2, f(1)) == nil)  -- b and c should be nil
```

### 3. Confirm __call metamethod chain limit
```lua
local t = setmetatable({}, {})
getmetatable(t).__call = t  -- infinite __call loop
local ok, err = pcall(t)
assert(not ok and err:find("__call"))
```

### 4. Confirm hook fires before first instruction
```lua
local called = false
debug.sethook(function(event)
  if event == "call" then called = true end
end, "c")
local function f() end
f()
debug.sethook()
assert(called)
```

### 5. Confirm CI reuse (no allocation on repeated calls)
```
-- Requires instrumentation: call f() 1000 times, verify CI allocation
-- count doesn't grow linearly (nodes are reused from the linked list)
```

### 6. Go-specific: run test suite
```bash
cd /home/ubuntu/workspace/go-lua
go test ./internal/vm/api/... -run TestCall -count=1
go test ./internal/vm/api/... -run TestHook -count=1
```
