# Tail Calls: luaD_pretailcall

## Overview

Tail calls reuse the current call frame instead of creating a new one.
This is critical for Lua's guarantee of unlimited tail recursion without
stack overflow. The function `luaD_pretailcall` prepares a tail call by
moving the new function and arguments down into the current frame's slot.

The VM instruction `OP_TAILCALL` handles upvalue closing, then delegates
to `luaD_pretailcall` for the actual frame reuse. For Lua functions,
execution restarts at `startfunc`. For C functions, the result is returned
through `luaD_poscall` as if the caller returned.

---

## C Source Analysis

### 1. OP_TAILCALL in the VM Loop (lvm.c:1737-1762)

```c
vmcase(OP_TAILCALL) {
    StkId ra = RA(i);
    int b = GETARG_B(i);                          // (1739) nargs + 1
    int nparams1 = GETARG_C(i);                   // (1741) for vararg delta
    int delta = (nparams1) ? ci->u.l.nextraargs + nparams1 : 0;  // (1743)
    if (b != 0)
        L->top.p = ra + b;                        // (1745) set exact top
    else
        b = cast_int(L->top.p - ra);              // (1746) use existing top
    savepc(ci);                                    // (1748) save PC for errors
    if (TESTARG_k(i)) {
        luaF_closeupval(L, base);                  // (1750) close upvalues
        lua_assert(L->tbclist.p < base);           // (1751) no pending TBC
        lua_assert(base == ci->func.p + 1);        // (1752) sanity check
    }
    if ((n = luaD_pretailcall(L, ci, ra, b, delta)) < 0)
        goto startfunc;                            // (1754) Lua fn → restart
    else {
        ci->func.p -= delta;                       // (1756) restore func
        luaD_poscall(L, ci, n);                    // (1757) finish caller
        updatetrap(ci);                            // (1758) hooks may change
        goto ret;                                  // (1759) caller returns
    }
}
```

**WHY `delta`?** Vararg functions shift `ci->func` forward by `nextraargs + nparams1`
to make room for extra args below the frame. `delta` reverses this shift so the
tail-called function occupies the original frame position.

**WHY close upvalues before pretailcall?** The current frame's local variables
are about to be overwritten. Any open upvalues pointing to those locals must be
closed (captured to heap) first. `TESTARG_k` is set by the compiler when the
function has upvalues that need closing.

**WHY `luaF_closeupval` not `luaF_close`?** Tail calls only close upvalues,
not to-be-closed variables. TBC vars (`<close>`) cannot exist at a tail call
site — the compiler ensures this (the assert on line 1751 verifies it).

**WHY `savepc` before the call?** `pretailcall` may trigger `__call` metamethod
resolution or stack reallocation, both of which can raise errors. The saved PC
ensures error messages point to the tail call instruction.

### 2. luaD_pretailcall (ldo.c:677-713)

```c
int luaD_pretailcall (lua_State *L, CallInfo *ci, StkId func,
                                    int narg1, int delta) {
  unsigned status = LUA_MULTRET + 1;              // (679) sentinel for nresults
 retry:
  switch (ttypetag(s2v(func))) {
```

**WHY `LUA_MULTRET + 1`?** Tail calls always return all results to the caller's
caller. `status` encodes `nresults + 1` in the CI. `MULTRET + 1` means "return
everything" — the ultimate caller's `wanted` determines how many to keep.

#### Case: C Closure (ldo.c:682-683)

```c
    case LUA_VCCL:
      return precallC(L, func, status, clCvalue(s2v(func))->f);
```

C function tail calls create a NEW frame via `precallC`, execute immediately,
and return the result count. The caller then does `poscall` + `goto ret`.
**C functions cannot reuse the frame** because they need their own C stack frame.

#### Case: Light C Function (ldo.c:684-685)

```c
    case LUA_VLCF:
      return precallC(L, func, status, fvalue(s2v(func)));
```

Same as C closure — light C functions also get a fresh frame.

#### Case: Lua Function — The Core (ldo.c:686-705)

```c
    case LUA_VLCL: {
      Proto *p = clLvalue(s2v(func))->p;
      int fsize = p->maxstacksize;                 // (688) new frame size
      int nfixparams = p->numparams;               // (689)
      int i;
      checkstackp(L, fsize - delta, func);         // (691) ensure space
      ci->func.p -= delta;                         // (692) undo vararg shift
      for (i = 0; i < narg1; i++)                  // (693-694)
        setobjs2s(L, ci->func.p + i, func + i);   //   move fn+args DOWN
      func = ci->func.p;                           // (695) update func ptr
      for (; narg1 <= nfixparams; narg1++)          // (696-697)
        setnilvalue(s2v(func + narg1));            //   pad missing args
      ci->top.p = func + 1 + fsize;               // (698) new frame top
      lua_assert(ci->top.p <= L->stack_last.p);    // (699) bounds check
      ci->u.l.savedpc = p->code;                   // (700) reset PC to start
      ci->callstatus |= CIST_TAIL;                 // (701) mark as tail call
      L->top.p = func + narg1;                     // (702) set stack top
      return -1;                                   // (703) signal: Lua function
    }
```

**Line-by-line WHY:**

**Line 691 — `checkstackp(L, fsize - delta, func)`:**
The new function might need more stack than the old one. `fsize - delta` accounts
for the space saved by the vararg shift. Stack reallocation may occur here.

**Line 692 — `ci->func.p -= delta`:**
Undo the vararg shift. For non-vararg functions, delta=0 (no-op). For vararg
functions, this moves `func.p` back to the original frame start, reclaiming
the extra-args space.

**Lines 693-694 — Move function + arguments down:**
The new function object and its arguments are at `func` (which may be higher
on the stack). Copy them down to `ci->func.p` to reuse the frame slot.
This is the key operation that avoids allocating a new CallInfo.

**Line 695 — `func = ci->func.p`:**
Update the local `func` pointer to the new (moved-down) position.

**Lines 696-697 — Pad missing arguments with nil:**
If the caller passed fewer arguments than the callee expects, fill with nil.
Same logic as `luaD_precall`.

**Line 698 — `ci->top.p = func + 1 + fsize`:**
Set the frame's top to accommodate the new function's stack needs.
`func + 1` is the base (first local), `+ fsize` is `maxstacksize`.

**Line 700 — `ci->u.l.savedpc = p->code`:**
Reset the program counter to the start of the new function's bytecode.
The VM will restart execution from instruction 0.

**Line 701 — `ci->callstatus |= CIST_TAIL`:**
Mark this frame as a tail call. Used by:
- `luaG_traceexec` to fire `LUA_HOOKTAILCALL` instead of `LUA_HOOKCALL`
- Debug info (`debug.getinfo`) to report tail calls in the call stack
- Error messages to show `(...tail calls...)` in tracebacks

**Line 702 — `L->top.p = func + narg1`:**
Set the actual stack top to just above the last argument. The VM uses this
to know where live values end.

#### Case: Not a Function (ldo.c:706-711)

```c
    default:
      checkstackp(L, 1, func);                    // space for metamethod
      status = tryfuncTM(L, func, status);         // try __call
      narg1++;                                     // metamethod shifts args
      goto retry;
```

**WHY `narg1++`?** `tryfuncTM` inserts the metamethod at `func` and shifts
all arguments up by one. The original "function" becomes the first argument.

**WHY `goto retry`?** The metamethod result might itself be a table with
`__call`, requiring another round. The `status` counter tracks depth to
prevent infinite `__call` chains.

---

## Go Implementation Mapping

### PreTailCall (do.go:580-623)

```go
func PreTailCall(L *stateapi.LuaState, ci *stateapi.CallInfo,
                 funcIdx int, narg1 int, delta int) int {
    status := uint32(stateapi.MultiRet + 1)
retry:
    fval := L.Stack[funcIdx].Val
    switch fval.Tt {
    case objectapi.TagCClosure:
        cc := fval.Val.(*closureapi.CClosure)
        return precallC(L, funcIdx, status, cc.Fn)

    case objectapi.TagLightCFunc:
        f := fval.Val.(stateapi.CFunction)
        return precallC(L, funcIdx, status, f)

    case objectapi.TagLuaClosure:
        cl := fval.Val.(*closureapi.LClosure)
        p := cl.Proto
        fsize := int(p.MaxStackSize)
        nfixparams := int(p.NumParams)
        CheckStack(L, fsize-delta)
        ci.Func -= delta                    // undo vararg shift
        for i := 0; i < narg1; i++ {        // move fn+args down
            L.Stack[ci.Func+i].Val = L.Stack[funcIdx+i].Val
        }
        funcIdx = ci.Func                   // update to moved position
        for ; narg1 <= nfixparams; narg1++ { // pad missing args
            L.Stack[funcIdx+narg1].Val = objectapi.Nil
        }
        ci.Top = funcIdx + 1 + fsize        // new frame top
        ci.SavedPC = 0                      // reset PC to start
        ci.CallStatus |= stateapi.CISTTail  // mark as tail call
        L.Top = funcIdx + narg1             // set stack top
        return -1

    default:
        CheckStack(L, 1)
        status = TryFuncTM(L, funcIdx, status)
        narg1++
        goto retry
    }
}
```

### OP_TAILCALL in Go VM (vm.go:2343-2370)

```go
case opcodeapi.OP_TAILCALL:
    b := opcodeapi.GetArgB(inst)
    nparams1 := opcodeapi.GetArgC(inst)
    delta := 0
    if nparams1 != 0 {
        delta = ci.NExtraArgs + nparams1
    }
    if b != 0 {
        L.Top = ra + b
    } else {
        b = L.Top - ra
    }
    if opcodeapi.GetArgK(inst) != 0 {
        closureapi.CloseUpvals(L, base)      // close upvalues
    }
    n := PreTailCall(L, ci, ra, b, delta)
    if n < 0 {
        goto startfunc                       // Lua function
    }
    ci.Func -= delta                         // C function: restore func
    PosCall(L, ci, n)
    goto ret
```

### Key Go Types and Constants

| Go | C | Location |
|----|---|----------|
| `stateapi.CISTTail` | `CIST_TAIL` | internal/state/api/state.go |
| `stateapi.MultiRet` | `LUA_MULTRET` | internal/state/api/state.go |
| `closureapi.CloseUpvals` | `luaF_closeupval` | internal/closure/api/closure.go:66 |
| `ci.NExtraArgs` | `ci->u.l.nextraargs` | internal/state/api/state.go |
| `ci.SavedPC` | `ci->u.l.savedpc` | internal/state/api/state.go |

---

## Difference Table

| Area | C Lua | go-lua | Severity | Notes |
|------|-------|--------|----------|-------|
| Stack pointers | `StkId` (pointer into stack array) | `int` (direct index) | 🟢 Low | Go indices survive realloc; C needs `StkIdRel` |
| `ci->func.p -= delta` | Pointer arithmetic on `StkIdRel` | `ci.Func -= delta` (integer subtraction) | 🟢 Low | Semantically identical |
| Move loop | `setobjs2s` (macro, copies TValue) | `L.Stack[dst].Val = L.Stack[src].Val` | 🟢 Low | Direct assignment in Go |
| Nil padding | `setnilvalue(s2v(...))` | `= objectapi.Nil` | 🟢 Low | Same semantics |
| `savedpc` reset | `ci->u.l.savedpc = p->code` (pointer to first instruction) | `ci.SavedPC = 0` (index 0) | 🟢 Low | Go uses index, C uses pointer |
| C function tail call | Creates new CI via `precallC`, returns n | Same: `precallC` creates CI, returns n | 🟢 Low | Direct mapping |
| `__call` metamethod | `tryfuncTM` + `goto retry` | `TryFuncTM` + `goto retry` | 🟢 Low | Direct mapping |
| `checkstackp` | May realloc stack, invalidates pointers | `CheckStack` may grow slice, indices stable | 🟢 Low | Go model simpler |
| Upvalue closing | `luaF_closeupval(L, base)` in VM | `closureapi.CloseUpvals(L, base)` in VM | 🟢 Low | Direct mapping |
| TBC assertion | `lua_assert(L->tbclist.p < base)` | No assertion | 🟡 Medium | Go trusts compiler; missing runtime check |
| `base == ci->func.p + 1` assert | Present in C | Not present in Go | 🟡 Medium | Missing sanity check |
| `ci->top <= stack_last` assert | Present in C (line 699) | Not present in Go | 🟡 Medium | Missing bounds assertion |
| Union flattening | `ci->u.l.savedpc`, `ci->u.l.nextraargs` | `ci.SavedPC`, `ci.NExtraArgs` (flat fields) | 🟢 Low | Go has no unions; all fields always present |
| `CIST_TAIL` usage | Checked by hook dispatch + debug | Same: checked by `TraceExec` + debug info | 🟢 Low | Direct mapping |

---

## Verification Methods

### 1. Basic tail call — no stack growth
```lua
-- Should not overflow even with millions of tail calls
function f(n) if n <= 0 then return "done" end return f(n - 1) end
assert(f(1000000) == "done")
```
Verifies: frame reuse works, no new CI allocated per call.

### 2. Tail call with vararg function
```lua
function vararg(...)
    local n = select("#", ...)
    if n <= 0 then return "ok" end
    return vararg(select(2, ...))  -- tail call with fewer args
end
assert(vararg(1, 2, 3, 4, 5) == "ok")
```
Verifies: `delta` calculation correct, vararg shift properly undone.

### 3. Tail call to C function
```lua
function f() return tostring(42) end  -- tail call to C function
assert(f() == "42")
```
Verifies: C function path in `pretailcall` works (creates new CI, returns n).

### 4. Tail call with __call metamethod
```lua
local mt = {__call = function(self, n)
    if n <= 0 then return "meta" end
    return self(n - 1)  -- tail call through __call
end}
local obj = setmetatable({}, mt)
assert(obj(100) == "meta")
```
Verifies: `tryfuncTM` + `goto retry` path, `narg1++` correct.

### 5. CIST_TAIL in debug info
```lua
function a() return b() end  -- tail call
function b() return debug.traceback() end
local tb = a()
assert(tb:find("tail call"))
```
Verifies: `CIST_TAIL` flag set, debug system reports tail calls.

### 6. Upvalue closing before tail call
```lua
local captured
function f()
    local x = 42
    local function g() return x end  -- creates upvalue to x
    captured = g
    return other()  -- tail call closes x's upvalue
end
function other() return captured() end
f()
assert(captured() == 42)  -- upvalue was properly closed
```
Verifies: `CloseUpvals` runs before frame reuse, upvalue captured to heap.

### 7. Missing argument padding
```lua
function f(a, b, c)
    if c == nil and b == nil then return a end
    return f(a)  -- tail call with fewer args than params
end
assert(f(1, 2, 3) == 1)
```
Verifies: nil-padding loop fills missing parameters.

### 8. Stack top correctness after C tail call
```lua
function f()
    return pcall(function() return 1, 2, 3 end)  -- C function tail call
end
local a, b, c, d = f()
assert(a == true and b == 1 and c == 2 and d == 3)
```
Verifies: `PosCall` after C tail call correctly moves results.
