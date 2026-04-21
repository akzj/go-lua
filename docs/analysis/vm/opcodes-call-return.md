# Call/Return Opcodes — Deep Analysis

> C source: `lua-master/lvm.c` | Go source: `internal/vm/vm.go`, `internal/vm/do.go`

## Encoding Conventions

| Field | Meaning in CALL/RETURN context |
|-------|-------------------------------|
| `B` | **nargs + 1** (CALL/TAILCALL) or **nresults + 1** (RETURN). `B==0` → variable (use stack top) |
| `C` | **nresults + 1** (CALL) or **nparams1** (TAILCALL/RETURN, for vararg adjustment) |
| `k` | If set on TAILCALL/RETURN: upvalues need closing before transfer |

The `+1` encoding allows 0 to mean "variable" while still representing 0 actual args/results.

## Key Helper Functions

| C (ldo.c) | Go (do.go) | Purpose |
|-----------|-----------|---------|
| `luaD_precall` (ldo.c:723) | `PreCall` (do.go:316) | Set up new CallInfo for call target |
| `luaD_poscall` (ldo.c:613) | `PosCall` (do.go:361) | Move results, unwind CallInfo |
| `luaD_pretailcall` (ldo.c:677) | `PreTailCall` (do.go:580) | Reuse current frame for tail call |

---

## OP_CALL (C:1720 → Go:2329)

### C Implementation
```c
// lvm.c:1720
vmcase(OP_CALL) {
    StkId ra = RA(i);
    CallInfo *newci;
    int b = GETARG_B(i);
    int nresults = GETARG_C(i) - 1;
    if (b != 0)                          // fixed number of arguments?
        L->top.p = ra + b;              // top signals number of arguments
    /* else previous instruction set top */
    savepc(ci);                          // in case of errors
    if ((newci = luaD_precall(L, ra, nresults)) == NULL)
        updatetrap(ci);                  // C call; nothing else to be done
    else {                               // Lua call: run function in this same C frame
        ci = newci;
        goto startfunc;
    }
    vmbreak;
}
```

### Why Each Line
- **`b = GETARG_B(i)`**: B encodes nargs+1. If B==0, arg count is variable (top already set by previous VARARG/CALL).
- **`nresults = GETARG_C(i) - 1`**: C encodes nresults+1. Subtract 1 to get actual count. C==1 → 0 results, C==0 → LUA_MULTRET (-1).
- **`L->top.p = ra + b`**: When B≠0, set top to `func + nargs + 1` (ra points to func slot). This tells precall how many args exist.
- **`savepc(ci)`**: Store current PC so error messages show correct line.
- **`luaD_precall` returns NULL for C functions**: C function already executed inside precall; just update trap and continue.
- **`luaD_precall` returns new CallInfo for Lua functions**: Switch to new frame and jump to `startfunc` to begin executing the callee.
- **`updatetrap(ci)`**: After C call returns, hooks may have changed; refresh trap flag.

### Go Implementation
```go
// vm.go:2329
case opcodeapi.OP_CALL:
    b := opcodeapi.GetArgB(inst)
    nresults := opcodeapi.GetArgC(inst) - 1
    if b != 0 {
        L.Top = ra + b
    }
    ci.SavedPC = ci.SavedPC // save PC
    newci := PreCall(L, ra, nresults)
    if newci != nil {
        ci = newci
        goto startfunc
    }
    // C function already executed
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| precall return | NULL = C func done | nil = C func done |
| trap update | `updatetrap(ci)` after C call | No explicit trap — no hook-trap mechanism |
| savepc | `savepc(ci)` macro stores PC offset | `ci.SavedPC` already an int index |
| Stack top | `L->top.p` (pointer) | `L.Top` (int index) |

---

## OP_TAILCALL (C:1737 → Go:2343)

### C Implementation
```c
// lvm.c:1737
vmcase(OP_TAILCALL) {
    StkId ra = RA(i);
    int b = GETARG_B(i);               // number of arguments + 1 (function)
    int n;                               // number of results when calling a C function
    int nparams1 = GETARG_C(i);
    /* delta is virtual 'func' - real 'func' (vararg functions) */
    int delta = (nparams1) ? ci->u.l.nextraargs + nparams1 : 0;
    if (b != 0)
        L->top.p = ra + b;
    else                                 // previous instruction set top
        b = cast_int(L->top.p - ra);
    savepc(ci);
    if (TESTARG_k(i)) {
        luaF_closeupval(L, base);        // close upvalues from current call
        lua_assert(L->tbclist.p < base); // no pending tbc variables
        lua_assert(base == ci->func.p + 1);
    }
    if ((n = luaD_pretailcall(L, ci, ra, b, delta)) < 0)
        goto startfunc;                  // Lua function: execute the callee
    else {                               // C function?
        ci->func.p -= delta;            // restore 'func' (if vararg)
        luaD_poscall(L, ci, n);         // finish caller
        updatetrap(ci);
        goto ret;                        // caller returns after the tail call
    }
}
```

### Why Each Line
- **`nparams1 = GETARG_C(i)`**: For vararg functions, C field holds nparams+1 (non-zero). For non-vararg, it's 0.
- **`delta`**: Vararg functions shift `func` up by `nextraargs + nparams1` slots. Delta tracks this shift so tail call can undo it, reusing the frame at the original position.
- **`b = cast_int(L->top.p - ra)`**: When B==0, compute actual arg count from stack top.
- **`TESTARG_k(i)` → close upvalues**: If the `k` bit is set, current function has upvalues that must be closed before frame reuse. The compiler sets `k` when the function creates closures that capture locals.
- **`luaD_pretailcall` returns <0 for Lua**: Reuses current CallInfo frame — moves args down, sets up new function. Returns negative to signal "Lua function, go execute."
- **`luaD_pretailcall` returns ≥0 for C**: C function already ran. `n` = number of results. Must restore func pointer and do poscall.
- **`goto ret`**: After C tail call completes, return from current frame entirely.

### Go Implementation
```go
// vm.go:2343
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
        closureapi.CloseUpvals(L, base)
    }
    n := PreTailCall(L, ci, ra, b, delta)
    if n < 0 {
        goto startfunc                   // Lua function
    }
    ci.Func -= delta                     // restore func
    PosCall(L, ci, n)
    goto ret
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| delta field | `ci->u.l.nextraargs` (union) | `ci.NExtraArgs` (direct field) |
| upval close | `luaF_closeupval(L, base)` | `closureapi.CloseUpvals(L, base)` |
| TBC assertion | `lua_assert(L->tbclist.p < base)` | No assertion (Go doesn't have TBC list pointer) |
| pretailcall return | `< 0` = Lua, `≥ 0` = C (n=results) | Same convention |
| trap update | `updatetrap(ci)` after C path | Omitted — no trap mechanism |

---

## OP_RETURN (C:1763 → Go:2368)

### C Implementation
```c
// lvm.c:1763
vmcase(OP_RETURN) {
    StkId ra = RA(i);
    int n = GETARG_B(i) - 1;            // number of results
    int nparams1 = GETARG_C(i);
    if (n < 0)                           // not fixed?
        n = cast_int(L->top.p - ra);    // get what is available
    savepc(ci);
    if (TESTARG_k(i)) {                 // may there be open upvalues?
        ci->u2.nres = n;               // save number of returns
        if (L->top.p < ci->top.p)
            L->top.p = ci->top.p;
        luaF_close(L, base, CLOSEKTOP, 1);
        updatetrap(ci);
        updatestack(ci);
    }
    if (nparams1)                        // vararg function?
        ci->func.p -= ci->u.l.nextraargs + nparams1;
    L->top.p = ra + n;                  // set call for 'luaD_poscall'
    luaD_poscall(L, ci, n);
    updatetrap(ci);
    goto ret;
}
```

### Why Each Line
- **`n = GETARG_B(i) - 1`**: B encodes nresults+1. B==0 → n=-1 → variable results.
- **`n = cast_int(L->top.p - ra)`**: For variable results, count everything from ra to current top.
- **`TESTARG_k(i)` → close upvalues + TBC**: The `k` bit signals that this function has upvalues or to-be-closed variables that need closing. `luaF_close` handles both upvalue closing and TBC `__close` metamethod calls.
- **`ci->u2.nres = n`**: Save result count before close — `__close` metamethods may use the stack.
- **`L->top.p = ci->top.p`**: Ensure stack has enough space for `__close` calls.
- **`updatestack(ci)`**: After `luaF_close`, stack may have been reallocated; refresh local pointers.
- **`ci->func.p -= nextraargs + nparams1`**: For vararg functions, restore `func` to its original position (before the vararg shift) so poscall places results correctly.
- **`luaD_poscall`**: Moves results from `ra..ra+n` down to caller's expected position, unwinds CallInfo.

### Go Implementation
```go
// vm.go:2368
case opcodeapi.OP_RETURN:
    b := opcodeapi.GetArgB(inst)
    n := b - 1
    nparams1 := opcodeapi.GetArgC(inst)
    if n < 0 {
        n = L.Top - ra
    }
    if opcodeapi.GetArgK(inst) != 0 {
        ci.NRes = n
        if L.Top < ci.Top {
            L.Top = ci.Top
        }
        closureapi.CloseUpvals(L, base)
        CloseTBCWithError(L, base, stateapi.StatusCloseKTop, objectapi.Nil, true)
        base = ci.Func + 1              // refresh after possible stack realloc
        ra = base + opcodeapi.GetArgA(inst)
    }
    if nparams1 != 0 {
        ci.Func -= ci.NExtraArgs + nparams1
    }
    L.Top = ra + n
    PosCall(L, ci, n)
    goto ret
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Close mechanism | `luaF_close(L, base, CLOSEKTOP, 1)` — single call handles upvals + TBC | Two calls: `CloseUpvals` + `CloseTBCWithError` — separated |
| Stack refresh | `updatestack(ci)` macro | Manual `base = ci.Func + 1; ra = base + GetArgA(inst)` |
| nres storage | `ci->u2.nres` (union field) | `ci.NRes` (direct field) |
| trap update | Two `updatetrap(ci)` calls | Omitted |

---

## OP_RETURN0 (C:1785 → Go:2397)

### C Implementation
```c
// lvm.c:1785
vmcase(OP_RETURN0) {
    if (l_unlikely(L->hookmask)) {
        StkId ra = RA(i);
        L->top.p = ra;
        savepc(ci);
        luaD_poscall(L, ci, 0);         // no hurry...
        trap = 1;
    }
    else {                               // do the 'poscall' here
        int nres = get_nresults(ci->callstatus);
        L->ci = ci->previous;           // back to caller
        L->top.p = base - 1;
        for (; l_unlikely(nres > 0); nres--)
            setnilvalue(s2v(L->top.p++)); // all results are nil
    }
    goto ret;
}
```

### Why Each Line
- **Hook check first**: If hooks are active, fall back to full `luaD_poscall` which fires return hooks. This is the slow path.
- **Fast path (no hooks)**: Inlines the poscall logic directly — avoids function call overhead. This is the common case.
- **`get_nresults(ci->callstatus)`**: The caller's expected result count is stored in the upper bits of `callstatus`.
- **`L->ci = ci->previous`**: Manually unwind the CallInfo stack.
- **`L->top.p = base - 1`**: Set top to just before the function slot (the caller's result destination).
- **`setnilvalue` loop**: If caller expects N results but we return 0, fill with nils. The `l_unlikely` hints this rarely runs (callers usually expect 0 results from void-like calls).

### Go Implementation
```go
// vm.go:2397
case opcodeapi.OP_RETURN0:
    if L.HookMask != 0 {
        L.Top = ra
        PosCall(L, ci, 0)
    } else {
        nres := ci.NResults()
        L.CI = ci.Prev
        L.Top = base - 1
        for i := 0; i < nres; i++ {
            L.Stack[L.Top].Val = objectapi.Nil
            L.Top++
        }
        if nres < 0 {
            L.Top = base - 1
        }
    }
    goto ret
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Hook check | `l_unlikely(L->hookmask)` | `L.HookMask != 0` (no branch hint) |
| nres < 0 handling | Implicit: `nres > 0` loop doesn't run | Explicit: `if nres < 0 { L.Top = base - 1 }` — handles MULTRET |
| nil fill | `setnilvalue(s2v(L->top.p++))` | `L.Stack[L.Top].Val = objectapi.Nil; L.Top++` |
| trap flag | `trap = 1` after hook path | No trap mechanism |

---

## OP_RETURN1 (C:1802 → Go:2417)

### C Implementation
```c
// lvm.c:1802
vmcase(OP_RETURN1) {
    if (l_unlikely(L->hookmask)) {
        StkId ra = RA(i);
        L->top.p = ra + 1;
        savepc(ci);
        luaD_poscall(L, ci, 1);
        trap = 1;
    }
    else {
        int nres = get_nresults(ci->callstatus);
        L->ci = ci->previous;
        if (nres == 0)
            L->top.p = base - 1;        // asked for no results
        else {
            StkId ra = RA(i);
            setobjs2s(L, base - 1, ra); // at least this result
            L->top.p = base;
            for (; l_unlikely(nres > 1); nres--)
                setnilvalue(s2v(L->top.p++)); // complete missing results
        }
    }
   ret:  /* return from a Lua function */
    if (ci->callstatus & CIST_FRESH)
        return;                          // end this frame
    else {
        ci = ci->previous;
        goto returning;                  // continue running caller in this frame
    }
```

### Why Each Line
- **Same hook-check pattern as RETURN0**: Slow path with hooks, fast path without.
- **`nres == 0`**: Caller doesn't want any results — just set top below func slot.
- **`setobjs2s(L, base - 1, ra)`**: Copy the single return value to `base - 1` (the caller's result slot = the function slot).
- **`L->top.p = base`**: Top is now one past the result.
- **nil fill for `nres > 1`**: Caller wanted more than 1 result; fill extras with nil.
- **`ret:` label**: This is where ALL return opcodes converge (RETURN, RETURN0, RETURN1 all `goto ret`).
- **`CIST_FRESH`**: If this CallInfo was the entry point from C (fresh C frame), return from `luaV_execute` entirely.
- **Otherwise**: The caller is also a Lua function in the same C frame — jump to `returning` to resume it.

### Go Implementation
```go
// vm.go:2417
case opcodeapi.OP_RETURN1:
    if L.HookMask != 0 {
        L.Top = ra + 1
        PosCall(L, ci, 1)
    } else {
        nres := ci.NResults()
        L.CI = ci.Prev
        if nres == 0 {
            L.Top = base - 1
        } else {
            L.Stack[base-1].Val = L.Stack[ra].Val
            L.Top = base
            for i := 1; i < nres; i++ {
                L.Stack[L.Top].Val = objectapi.Nil
                L.Top++
            }
        }
    }
    goto ret
```

The `ret:` label in Go (vm.go:2574):
```go
ret:
    if ci.CallStatus&stateapi.CISTFresh != 0 {
        return // end this frame
    }
    ci = L.CI
    if ci.IsLua() {
        L.OldPC = ci.SavedPC - 1
    }
    goto startfunc
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| ret label | `returning:` resumes caller in same C frame | `goto startfunc` — re-enters main loop |
| CIST_FRESH check | `ci->callstatus & CIST_FRESH` | `ci.CallStatus & stateapi.CISTFresh` |
| OldPC restore | Done inside `rethook` in ldo.c | Done at `ret:` label + in PosCall |
| Value copy | `setobjs2s(L, base-1, ra)` (macro) | `L.Stack[base-1].Val = L.Stack[ra].Val` |

---

## The Call/Return Flow

### Call Flow: `OP_CALL` → `luaD_precall` → `startfunc`

```
Caller frame (ci)          New frame (newci)
┌─────────────────┐        ┌─────────────────┐
│ ... code ...    │        │ callee code     │
│ OP_CALL ra,B,C  │───→    │ (starts at PC 0)│
│ (saves PC here) │        │                 │
└─────────────────┘        └─────────────────┘
         │                          │
    B!=0: set top=ra+B         precall: push CI
    B==0: top already set      fill missing params with nil
         │                     set savedpc=0
    precall(ra, nresults)      return newci
         │                          │
    NULL → C func done         ci=newci; goto startfunc
    !NULL → Lua func
```

### Return Flow: `OP_RETURN` → `luaD_poscall` → `ret:`

```
Callee frame (ci)          Caller frame (ci->previous)
┌─────────────────┐        ┌─────────────────┐
│ ... code ...    │        │ ... code ...    │
│ OP_RETURN ra,B,C│───→    │ (resumes after  │
│                 │        │  the OP_CALL)   │
└─────────────────┘        └─────────────────┘
         │                          │
    B!=0: n=B-1                poscall: move results
    B==0: n=top-ra             to func slot (base-1)
         │                     fill missing with nil
    k bit? close upvals/TBC   L->ci = ci->previous
    vararg? restore func ptr        │
         │                     CIST_FRESH? return
    poscall(ci, n)             else: goto returning
```

---

## Cross-Reference

- **Detailed precall/poscall internals**: `.analysis/04-call-return-error.md`
- **PreCall** (do.go:316): Handles Lua closures, C closures, light C functions, `__call` metamethod
- **PosCall** (do.go:361): Fires return hook, restores OldPC, moves results via `moveResults`
- **PreTailCall** (do.go:580): Reuses frame for Lua, executes immediately for C
