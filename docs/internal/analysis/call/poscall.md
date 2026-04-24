# luaD_poscall — Post-Call Cleanup

## Overview

`luaD_poscall` is called after every function returns. It has three jobs:

1. **Fire the return hook** (via `rethook`) if hooks are active.
2. **Move results** from the callee's stack area to the caller's expected
   destination (the slot where the function value was).
3. **Pop the CallInfo** — restore `L->ci` to the caller's frame.

The function is deceptively short (11 lines in C) but delegates to two complex
helpers: `rethook` (hook dispatch + OldPC restoration) and `moveresults`
(optimized result placement with TBC variable handling).

The Go equivalent is `PosCall` in `internal/vm/do.go:361`.

---

## C Source Analysis

### `rethook` (ldo.c:502-519)

```c
static void rethook (lua_State *L, CallInfo *ci, int nres) {
  if (L->hookmask & LUA_MASKRET) {                    // [1]
    StkId firstres = L->top.p - nres;                  // [2]
    int delta = 0;
    int ftransfer;
    if (isLua(ci)) {                                   // [3]
      Proto *p = ci_func(ci)->p;
      if (p->flag & PF_VAHID)
        delta = ci->u.l.nextraargs + p->numparams + 1;
    }
    ci->func.p += delta;                               // [4]
    ftransfer = cast_int(firstres - ci->func.p);       // [5]
    luaD_hook(L, LUA_HOOKRET, -1, ftransfer, nres);   // [6]
    ci->func.p -= delta;                               // [7]
  }
  if (isLua(ci = ci->previous))                        // [8]
    L->oldpc = pcRel(ci->u.l.savedpc, ci_func(ci)->p); // [9]
}
```

**WHY each line**:

1. **Guard**: Only fire the return hook if `LUA_MASKRET` is set. But note:
   lines [8-9] execute **unconditionally** (outside the if). The OldPC
   restoration is needed by the **line hook**, not the return hook.

2. **`firstres`**: Results sit at the top of the stack. `L->top - nres` is the
   first result's position.

3. **Vararg correction**: For vararg functions with hidden parameters
   (`PF_VAHID`), `ci->func.p` points to the "real" function slot (after the
   hidden varargs). The hook needs to see the "virtual" function position
   (before the varargs) to compute `ftransfer` correctly.

4. **Adjust func forward**: Temporarily move `func` past the hidden varargs so
   that `ftransfer` (the offset from func to first result) reflects what the
   hook user expects.

5. **`ftransfer`**: Distance from function slot to first result. Passed to the
   hook via `L->transferinfo` so `debug.getinfo` can report transfer info.

6. **Fire the hook**: `LUA_HOOKRET` event, line=-1 (not a line event).

7. **Restore func**: Undo the vararg adjustment. The rest of poscall needs the
   real `func` position.

8. **Move to caller**: `ci = ci->previous` — now `ci` points to the **caller**.
   Check if the caller is a Lua function.

9. **Restore OldPC**: Set `L->oldpc` to the caller's current PC (relative to
   the proto's code base). **This is critical for the line hook**: when
   execution resumes in the caller after the call returns, `changedline()`
   compares `oldpc` to the current PC to decide if a line event should fire.
   Without this restoration, `oldpc` would be stale from before the call,
   causing spurious or missed line events.

### `genmoveresults` (ldo.c:548-558)

```c
l_sinline void genmoveresults (lua_State *L, StkId res, int nres,
                                             int wanted) {
  StkId firstresult = L->top.p - nres;               // [1]
  int i;
  if (nres > wanted)                                   // [2]
    nres = wanted;
  for (i = 0; i < nres; i++)                           // [3]
    setobjs2s(L, res + i, firstresult + i);
  for (; i < wanted; i++)                              // [4]
    setnilvalue(s2v(res + i));
  L->top.p = res + wanted;                             // [5]
}
```

**WHY each line**:
1. Results are at the top of the stack; compute their start position.
2. If more results than wanted, ignore the extras.
3. Copy available results from their source position to `res` (the function
   slot). Results overwrite the function value and arguments.
4. If fewer results than wanted, pad with nil.
5. Set `L->top` to just past the last result. The caller's view of the stack
   is now clean.

### `moveresults` (ldo.c:569-612)

This is the **optimized dispatcher** that handles common cases with fast paths:

```c
l_sinline void moveresults (lua_State *L, StkId res, int nres,
                                          l_uint32 fwanted) {
  switch (fwanted) {
    case 0 + 1:                                        // [A] no values needed
      L->top.p = res;
      return;
    case 1 + 1:                                        // [B] one value needed
      if (nres == 0)
        setnilvalue(s2v(res));
      else
        setobjs2s(L, res, L->top.p - nres);
      L->top.p = res + 1;
      return;
    case LUA_MULTRET + 1:                              // [C] all results
      genmoveresults(L, res, nres, nres);
      break;
    default: {                                         // [D] 2+ results or TBC
      int wanted = get_nresults(fwanted);
      if (fwanted & CIST_TBC) {                        // [D1]
        L->ci->u2.nres = nres;
        L->ci->callstatus |= CIST_CLSRET;
        res = luaF_close(L, res, CLOSEKTOP, 1);        // [D2]
        L->ci->callstatus &= ~CIST_CLSRET;
        if (L->hookmask) {                             // [D3]
          ptrdiff_t savedres = savestack(L, res);
          rethook(L, L->ci, nres);
          res = restorestack(L, savedres);
        }
        if (wanted == LUA_MULTRET)
          wanted = nres;
      }
      genmoveresults(L, res, nres, wanted);
      break;
    }
  }
}
```

**WHY each case**:

- **[A] `fwanted = 0+1` (nresults=0)**: Statement calls like `f()` where the
  return value is discarded. Just reset `top` to the function slot — cheapest
  possible path.

- **[B] `fwanted = 1+1` (nresults=1)**: Single-result expressions like
  `x = f()`. If no results, set nil. If results exist, copy just the first
  one. This is the most common case in Lua code.

- **[C] `fwanted = LUA_MULTRET+1` (nresults=-1)**: Tail calls, `return f()`,
  `print(f())`. Keep all results — delegate to `genmoveresults(nres, nres)`.

- **[D] Default**: Two or more specific results, OR to-be-closed variables.
  - **[D1]** `CIST_TBC` flag: If the function has to-be-closed variables
    (declared with `<close>`), they must be closed **before** moving results.
  - **[D2]** `luaF_close`: Close all TBC variables from `res` upward. This
    may call `__close` metamethods which can yield or error.
  - **[D3]** If hooks are active, fire `rethook` **after** closing TBC
    variables (the hook should see the post-close state). Must
    `savestack`/`restorestack` because the hook can trigger stack reallocation.

**Key insight about `fwanted` encoding**: The C code passes `fwanted` as a
`l_uint32` that packs both `nresults+1` (low 8 bits) and the `CIST_TBC` flag
(bit 18). The `CIST_TBC` bit forces the switch into the default case even for
0 or 1 results, because TBC closing must happen.

### `luaD_poscall` (ldo.c:613-623)

```c
void luaD_poscall (lua_State *L, CallInfo *ci, int nres) {
  l_uint32 fwanted = ci->callstatus & (CIST_TBC | CIST_NRESULTS); // [1]
  if (l_unlikely(L->hookmask) && !(fwanted & CIST_TBC))           // [2]
    rethook(L, ci, nres);
  moveresults(L, ci->func.p, nres, fwanted);                      // [3]
  lua_assert(!(ci->callstatus &                                    // [4]
        (CIST_HOOKED | CIST_YPCALL | CIST_FIN | CIST_CLSRET)));
  L->ci = ci->previous;                                           // [5]
}
```

**WHY each line**:

1. **Extract fwanted**: Mask out just the result count (low 8 bits) and TBC
   flag (bit 18) from `callstatus`. Everything else is irrelevant for result
   handling.

2. **Conditional rethook**: Fire `rethook` ONLY if:
   - Hooks are active (`L->hookmask != 0`), AND
   - There are NO TBC variables (`!(fwanted & CIST_TBC)`).
   **WHY the TBC exclusion?** When TBC variables exist, `rethook` is called
   **inside** `moveresults` (at [D3]) — after the `__close` metamethods run.
   Calling it here too would fire the hook twice.

3. **Move results**: `ci->func.p` is the destination — results overwrite the
   function slot and everything above it. This is why the function value is
   "consumed" by the call.

4. **Assert clean state**: By the time we reach poscall, none of these flags
   should be set. If they are, something went wrong in the call lifecycle.

5. **Pop CI**: Restore `L->ci` to the caller's frame. The callee's CI remains
   in the linked list for reuse (see `next_ci` in precall.md).

---

## Go Implementation Mapping

### `PosCall` → `luaD_poscall` (do.go:361)

```go
func PosCall(L *stateapi.LuaState, ci *stateapi.CallInfo, nres int) {
    wanted := ci.NResults()                              // [1]
    res := ci.Func                                       // [2]

    if L.HookMask != 0 {                                 // [3]
        if L.AllowHook && L.HookMask&stateapi.MaskRet != 0 {
            retHook(L, ci, nres)                         // [3a]
        }
        if prev := ci.Prev; prev != nil && prev.IsLua() { // [3b]
            L.OldPC = prev.SavedPC - 1
        }
    }

    moveResults(L, res, nres, wanted)                    // [4]
    L.CI = ci.Prev                                       // [5]
}
```

**Structural differences from C**:

- **[1]** Go calls `ci.NResults()` which decodes `(CallStatus & 0xFF) - 1`.
  C extracts `fwanted` as the raw packed value including CIST_TBC. **Go does
  not handle TBC in moveResults** — see difference table.

- **[2]** `res = ci.Func` is an `int` index. C uses `ci->func.p` (a pointer).

- **[3]** Go splits the rethook logic differently from C:
  - **[3a]** `retHook` is guarded by **both** `AllowHook` and `MaskRet`. In C,
    `rethook` only checks `MaskRet` (AllowHook is checked inside `luaD_hook`).
    Functionally equivalent but the check is at a different level.
  - **[3b]** OldPC restoration is done **inline in PosCall**, not inside
    `rethook`. This is a deliberate go-lua design: the OldPC restoration must
    happen even when `AllowHook` is false (during hook dispatch), so it cannot
    be inside `retHook` which is guarded by `AllowHook`.

- **[4-5]** Identical to C (move results, pop CI).

### `retHook` → `rethook` (do.go:451)

```go
func retHook(L *stateapi.LuaState, ci *stateapi.CallInfo, nres int) {
    hookDispatch(L, "return", -1)
}
```

**Major simplification**: Go's `retHook` does NOT compute `ftransfer`/`delta`
for vararg functions. It simply dispatches the "return" event. This means
`debug.getinfo`'s transfer info is not available in go-lua's return hook.

### `moveResults` → `moveresults` (do.go:539)

```go
func moveResults(L *stateapi.LuaState, res int, nres int, wanted int) {
    switch wanted {
    case 0:
        L.Top = res
    case 1:
        if nres == 0 {
            L.Stack[res].Val = objectapi.Nil
        } else {
            L.Stack[res].Val = L.Stack[L.Top-nres].Val
        }
        L.Top = res + 1
    case stateapi.MultiRet:
        genMoveResults(L, res, nres, nres)
    default:
        genMoveResults(L, res, nres, wanted)
    }
}
```

**Key differences from C**:

1. **`wanted` is a plain `int`**, not a packed `l_uint32` with TBC flag. The
   Go code does not handle `CIST_TBC` in moveResults at all.

2. **Switch cases use decoded values** (0, 1, MultiRet) instead of the C
   encoding (0+1, 1+1, MULTRET+1). The decoding happens in `ci.NResults()`.

3. **No TBC handling**: The default case simply calls `genMoveResults` without
   checking for to-be-closed variables. TBC support would need to be added
   here if go-lua implements `<close>` variables.

### `genMoveResults` → `genmoveresults` (do.go:562)

```go
func genMoveResults(L *stateapi.LuaState, res int, nres int, wanted int) {
    firstResult := L.Top - nres
    if nres > wanted { nres = wanted }
    for i := 0; i < nres; i++ {
        L.Stack[res+i].Val = L.Stack[firstResult+i].Val
    }
    for i := nres; i < wanted; i++ {
        L.Stack[res+i].Val = objectapi.Nil
    }
    L.Top = res + wanted
}
```

**1:1 mapping** with C's `genmoveresults`. Uses `objectapi.Nil` instead of
`setnilvalue`. Uses direct slice indexing instead of pointer arithmetic.

---

## The OldPC Restoration — Why It Matters

The single most critical line in the entire poscall system is:

```c
// C (inside rethook, ldo.c:518-519):
if (isLua(ci = ci->previous))
    L->oldpc = pcRel(ci->u.l.savedpc, ci_func(ci)->p);
```

```go
// Go (inside PosCall, do.go:379-381):
if prev := ci.Prev; prev != nil && prev.IsLua() {
    L.OldPC = prev.SavedPC - 1
}
```

**WHY this exists**: After a function call returns, execution resumes at the
caller's `savedpc`. The line hook uses `changedline(oldpc, newpc)` to decide
if a line event should fire. If `oldpc` is not restored to match the caller's
current position, `changedline` will compare against a stale value from before
the call was made, causing:

- **Missed line events**: If the stale oldpc happens to be on the same line
  as the return point.
- **Spurious line events**: If the stale oldpc is on a different line but
  the return point is the same line as the next instruction.

**The go-lua difference**: In C, this runs inside `rethook` (which is called
from `luaD_poscall`). In Go, it's inlined directly in `PosCall` and runs
**even when AllowHook is false**. This is correct — the C version also runs
unconditionally (it's outside the `if (L->hookmask & LUA_MASKRET)` guard).

**SavedPC - 1 vs pcRel**: In C, `pcRel` computes `savedpc - p->code` (an
offset from the proto's code array start). In Go, `SavedPC` is already an
index into `Proto.Code`, so `SavedPC - 1` gives the equivalent "current
instruction" offset (savedpc points to the **next** instruction).

---

## Result Movement Lifecycle

```
Before poscall:
  Stack: [... func | arg1 | arg2 | ... | local1 | ... | res1 | res2 | res3]
                                                         ^--- L.Top - nres
                                                                          ^--- L.Top

After moveResults (wanted=2):
  Stack: [... res1 | res2 | ...]
         ^--- res (= ci.Func)    ^--- L.Top = res + 2

After moveResults (wanted=0):
  Stack: [...]
         ^--- L.Top = res

After moveResults (wanted=MULTRET, nres=3):
  Stack: [... res1 | res2 | res3 | ...]
         ^--- res                  ^--- L.Top = res + 3
```

Results are always moved to `ci->func.p` (the slot where the function value
was). This overwrites the function, its arguments, and any locals — they're
no longer needed. The caller sees results starting at what was the function
slot.

---

## Difference Table

| Area | C Lua (ldo.c) | go-lua (do.go) | Severity | Notes |
|---|---|---|---|---|
| `fwanted` encoding | Packed `l_uint32` with TBC flag + nresults | Plain `int` from `NResults()` | **Medium** | Go cannot handle TBC in moveResults |
| TBC variable closing | Full `CIST_TBC` path in moveresults default case | Not implemented | **High** | `<close>` variables won't close on return. Blocks `<close>` feature. |
| `rethook` vararg delta | Computes `delta` for `PF_VAHID` vararg funcs | Not computed | **Medium** | `debug.getinfo` transfer info wrong for vararg returns |
| `rethook` ftransfer | Passes `ftransfer`/`nres` to `luaD_hook` | Not passed (hookDispatch takes event+line only) | **Medium** | Transfer info unavailable in return hooks |
| OldPC restoration location | Inside `rethook` (unconditional, outside MaskRet guard) | Inline in `PosCall` (unconditional) | **None** | Same semantics, different code location |
| OldPC when AllowHook=false | Runs (outside the MaskRet guard) | Runs (outside the AllowHook guard) | **None** | Both correctly unconditional |
| `savestack`/`restorestack` | Used in TBC path (hook can move stack) | Not needed (int indices) | **None** | Go advantage |
| Assert on return | Checks no stray CIST flags | No equivalent assertion | **Low** | Missing safety check |
| `moveresults` case 1+1 | Uses `setobjs2s` (copies with write barrier) | Direct `.Val` assignment | **None** | Go GC handles this differently |
| `moveresults` MULTRET | `genmoveresults(nres, nres)` | `genMoveResults(nres, nres)` | **None** | Identical logic |

---

## Verification Methods

### 1. Confirm result count handling
```lua
-- Test 0, 1, 2, MULTRET results
local function f() return 10, 20, 30 end

-- 0 results (statement call)
f()

-- 1 result
local x = f()
assert(x == 10)

-- 2 results
local a, b = f()
assert(a == 10 and b == 20)

-- MULTRET (tail position)
local function g() return f() end
local p, q, r = g()
assert(p == 10 and q == 20 and r == 30)
```

### 2. Confirm nil-padding when fewer results than wanted
```lua
local function f() return 1 end
local a, b, c = f()
assert(a == 1 and b == nil and c == nil)
```

### 3. Confirm nil when zero results but one wanted
```lua
local function f() end
local x = f()
assert(x == nil)
```

### 4. Confirm return hook fires
```lua
local events = {}
debug.sethook(function(event)
  events[#events+1] = event
end, "r")
local function f() return 1 end
f()
debug.sethook()
-- Should contain "return" events
local found = false
for _, e in ipairs(events) do
  if e == "return" then found = true; break end
end
assert(found)
```

### 5. Confirm OldPC restoration (line hook after call)
```lua
local lines = {}
debug.sethook(function(_, line) lines[#lines+1] = line end, "l")
local function f() return 1 end
local x = f()  -- line A
local y = 2    -- line B (should fire line event)
debug.sethook()
-- Verify line B appears in the trace
```

### 6. Go-specific test suite
```bash
cd /home/ubuntu/workspace/go-lua
go test ./internal/vm/... -run TestPosCall -count=1
go test ./internal/vm/... -run TestReturn -count=1
go test ./... -run TestHookReturn -count=1
```

### 7. Confirm CI is popped (not freed)
```
After poscall: L.CI should equal ci.Prev
ci should still be reachable via L.CI.Next (for reuse by next call)
```
