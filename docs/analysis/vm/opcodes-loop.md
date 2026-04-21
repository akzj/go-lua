# Loop Opcodes — Deep Analysis

> C source: `lua-master/lvm.c` | Go source: `internal/vm/vm.go`

## Stack Layout for Numeric For

```
Before OP_FORPREP:           After OP_FORPREP (integer):
  ra   : initial value         ra   : iteration count (unsigned)
  ra+1 : limit                 ra+1 : step
  ra+2 : step                  ra+2 : control variable (= init)
  ra+3 : (user-visible var)    ra+3 : (user-visible copy)
```

For **float** loops: `ra=limit, ra+1=step, ra+2=index`.

---

## forlimit — Integer Limit Conversion (C:181, Go:1137)

```c
// lvm.c:181
static int forlimit (lua_State *L, lua_Integer init, const TValue *lim,
                                   lua_Integer *p, lua_Integer step) {
  if (!luaV_tointeger(lim, p, (step < 0 ? F2Iceil : F2Ifloor))) {
    lua_Number flim;
    if (!tonumber(lim, &flim)) luaG_forerror(L, lim, "limit");
    if (luai_numlt(0, flim)) {
      if (step < 0) return 1;  *p = LUA_MAXINTEGER;
    } else {
      if (step > 0) return 1;  *p = LUA_MININTEGER;
    }
  }
  return (step > 0 ? init > *p : init < *p);
}
```

**Key logic**: Try converting limit to integer (floor for ascending, ceil for descending). If limit is a float outside int64 range: if unreachable → skip loop; otherwise truncate to MAX/MIN. Final check: if init already past limit → skip.

Go equivalent: `forLimit` at vm.go:1137 — same logic using `flttointeger` helper instead of `luaV_tointeger`.

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Integer conversion | `luaV_tointeger` with F2I mode enum | `flttointeger` helper with step sign |
| Error | `luaG_forerror(L, lim, "limit")` | `RunError(L, "'for' limit must be a number")` |

---

## forprep — Loop Preparation (C:214, Go:1211)

```c
// lvm.c:214 (integer path only — float path similar)
static int forprep (lua_State *L, StkId ra) {
  TValue *pinit = s2v(ra), *plimit = s2v(ra + 1), *pstep = s2v(ra + 2);
  if (ttisinteger(pinit) && ttisinteger(pstep)) {
    lua_Integer init = ivalue(pinit), step = ivalue(pstep), limit;
    if (step == 0) luaG_runerror(L, "'for' step is zero");
    if (forlimit(L, init, plimit, &limit, step)) return 1;
    lua_Unsigned count;
    if (step > 0) {
      count = l_castS2U(limit) - l_castS2U(init);
      if (step != 1) count /= l_castS2U(step);
    } else {
      count = l_castS2U(init) - l_castS2U(limit);
      count /= l_castS2U(-(step + 1)) + 1u;
    }
    chgivalue(s2v(ra), l_castU2S(count));   // ra = count
    setivalue(s2v(ra + 1), step);            // ra+1 = step
    chgivalue(s2v(ra + 2), init);            // ra+2 = init
  }
  else { /* float: convert all to float, check step==0, store limit/step/init */ }
  return 0;
}
```

**Why count-based**: Instead of `idx <= limit` each iteration (overflow-prone with signed ints), C Lua precomputes total iterations as unsigned. The `-(step+1)+1` trick avoids negating MIN_INTEGER.

Go equivalent: `ForPrep` at vm.go:1211 — identical algorithm. Uses `uint64()` casts instead of `l_castS2U`, `objectapi.MakeInteger()` instead of `chgivalue`.

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Type check | `ttisinteger(pinit)` macro | `pinit.IsInteger()` method |
| Value mutation | `chgivalue`/`setivalue` (in-place) | `MakeInteger()` creates new TValue |
| Unsigned cast | `l_castS2U` macro | `uint64()` cast |

---

## OP_FORPREP (C:1849 → Go:2292)

### C Implementation
```c
// lvm.c:1849
vmcase(OP_FORPREP) {
    StkId ra = RA(i);
    savestate(L, ci);                    // in case of errors
    if (forprep(L, ra))
        pc += GETARG_Bx(i) + 1;         // skip the loop
    vmbreak;
}
```

### Why Each Line
- **`savestate`**: Saves PC + stack state — `forprep` may raise errors (step==0, bad types).
- **`forprep` returns true**: Loop should be skipped. Jump past body + FORLOOP instruction (`+1`).
- **`GETARG_Bx`**: Unsigned offset to end of loop body.

### Go Implementation
```go
// vm.go:2292
case opcodeapi.OP_FORPREP:
    if ForPrep(L, ra) {
        ci.SavedPC += opcodeapi.GetArgBx(inst) + 1
    }
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| State save | `savestate(L, ci)` macro | Implicit — Go error recovery via panic/recover |
| PC advance | `pc += ...` (pointer arith) | `ci.SavedPC += ...` (index arith) |

---

## OP_FORLOOP (C:1831 → Go:2297)

### C Implementation
```c
// lvm.c:1831
vmcase(OP_FORLOOP) {
    StkId ra = RA(i);
    if (ttisinteger(s2v(ra + 1))) {      // integer loop?
        lua_Unsigned count = l_castS2U(ivalue(s2v(ra)));
        if (count > 0) {
            lua_Integer step = ivalue(s2v(ra + 1));
            lua_Integer idx = ivalue(s2v(ra + 2));
            chgivalue(s2v(ra), l_castU2S(count - 1));
            idx = intop(+, idx, step);
            chgivalue(s2v(ra + 2), idx);
            pc -= GETARG_Bx(i);          // jump back
        }
    }
    else if (floatforloop(L, ra))        // float loop
        pc -= GETARG_Bx(i);
    updatetrap(ci);
    vmbreak;
}
```

### Why Each Line
- **Type check on `ra+1` (step)**: Step type determines integer vs float (set by forprep).
- **Integer fast path**: Decrement counter, advance index. No comparison — counter tracks iterations exactly.
- **`intop(+, idx, step)`**: Wrapping unsigned addition — avoids UB on signed overflow.
- **`pc -= GETARG_Bx(i)`**: Backward jump to loop body start.
- **Float path**: `floatforloop` (C:273) does `idx += step` and checks `idx <= limit` (or `limit <= idx`).
- **`updatetrap`**: Allows debug hooks to break the loop.

### Go Implementation
```go
// vm.go:2297
case opcodeapi.OP_FORLOOP:
    if ForLoop(L, ra) {
        ci.SavedPC -= opcodeapi.GetArgBx(inst)
    }
```

`ForLoop` at vm.go:1276 — integer and float paths both inlined:
```go
func ForLoop(L *stateapi.LuaState, ra int) bool {
    if L.Stack[ra+1].Val.IsInteger() {
        count := uint64(L.Stack[ra].Val.Integer())
        if count > 0 {
            step := L.Stack[ra+1].Val.Integer()
            idx := L.Stack[ra+2].Val.Integer()
            L.Stack[ra].Val = objectapi.MakeInteger(int64(count - 1))
            L.Stack[ra+2].Val = objectapi.MakeInteger(idx + step)
            return true
        }
    } else { /* float: idx+=step, check bounds, return true if continuing */ }
    return false
}
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Integer loop | Inlined in vmcase | Extracted to `ForLoop` function |
| Float loop | Separate `floatforloop` (C:273) | Inlined in `ForLoop` |
| Wrapping add | `intop(+, idx, step)` macro | `idx + step` (Go int64 wraps naturally) |
| trap update | `updatetrap(ci)` | No trap mechanism |

---

## OP_TFORPREP (C:1856 → Go:2302)

### C Implementation
```c
// lvm.c:1856
vmcase(OP_TFORPREP) {
    StkId ra = RA(i);
    TValue temp;
    setobj(L, &temp, s2v(ra + 3));       // save closing var
    setobjs2s(L, ra + 3, ra + 2);        // ra+3 = control
    setobj2s(L, ra + 2, &temp);           // ra+2 = closing
    halfProtect(luaF_newtbcupval(L, ra + 2)); // mark as to-be-closed
    pc += GETARG_Bx(i);                  // jump to end of loop
    i = *(pc++);                          // fetch TFORCALL
    lua_assert(GET_OPCODE(i) == OP_TFORCALL && ra == RA(i));
    goto l_tforcall;
}
```

### Why Each Line
- **Slot swap**: Compiler places `ra+2=control, ra+3=closing`. TFORPREP swaps them so the closing variable is at ra+2 where `luaF_newtbcupval` expects it for TBC marking.
- **`luaF_newtbcupval`**: Creates a to-be-closed upvalue. On loop exit/error, `__close` is called.
- **`pc += GETARG_Bx`**: Jump to TFORCALL at end of loop. First iteration starts by calling iterator.
- **Fetch + goto**: Falls through to TFORCALL without re-entering dispatch.

### Go Implementation
```go
// vm.go:2302
case opcodeapi.OP_TFORPREP:
    temp := L.Stack[ra+3].Val
    L.Stack[ra+3].Val = L.Stack[ra+2].Val
    L.Stack[ra+2].Val = temp
    MarkTBC(L, ra+2)
    ci.SavedPC += opcodeapi.GetArgBx(inst)
    inst = code[ci.SavedPC]
    ci.SavedPC++
    ra = base + opcodeapi.GetArgA(inst)
    goto tforcall
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| TBC creation | `luaF_newtbcupval` — creates upvalue object | `MarkTBC` — marks slot (do.go:1220) |
| Assertion | `lua_assert(OP_TFORCALL)` | None (trusts compiler) |
| Fall-through | `goto l_tforcall` (label inside vmcase) | `goto tforcall` (label at vm.go:2554) |

---

## OP_TFORCALL (C:1875 → Go:2319/2554)

### C Implementation
```c
// lvm.c:1875
vmcase(OP_TFORCALL) {
  l_tforcall: {
    StkId ra = RA(i);
    setobjs2s(L, ra + 5, ra + 3);       // copy control variable
    setobjs2s(L, ra + 4, ra + 1);       // copy state
    setobjs2s(L, ra + 3, ra);           // copy function
    L->top.p = ra + 3 + 3;
    ProtectNT(luaD_call(L, ra + 3, GETARG_C(i)));
    updatestack(ci);
    i = *(pc++);
    lua_assert(GET_OPCODE(i) == OP_TFORLOOP && ra == RA(i));
    goto l_tforloop;
  }}
```

### Why Each Line
- **Copy to ra+3..ra+5**: Iterator call needs `func, state, control`. Permanent copies at ra+0..ra+2 are preserved; call uses copies at ra+3..ra+5.
- **`L->top.p = ra + 3 + 3`**: 3 arguments for the call (function + 2 args).
- **`luaD_call`**: Call the iterator. C field = expected results.
- **`updatestack`**: Stack may have been reallocated during call.
- **Fall-through to TFORLOOP**: Check if iterator returned nil.

### Go Implementation
```go
// vm.go:2554 (tforcall label)
tforcall:
    L.Stack[ra+5].Val = L.Stack[ra+3].Val // copy control
    L.Stack[ra+4].Val = L.Stack[ra+1].Val // copy state
    L.Stack[ra+3].Val = L.Stack[ra].Val   // copy function
    L.Top = ra + 3 + 3
    Call(L, ra+3, opcodeapi.GetArgC(inst))
    L.Top = ci.Top
    // Inline TFORLOOP check:
    inst = code[ci.SavedPC]; ci.SavedPC++
    ra = base + opcodeapi.GetArgA(inst)
    if !L.Stack[ra+3].Val.IsNil() {
        ci.SavedPC -= opcodeapi.GetArgBx(inst)
    }
    continue
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Call | `luaD_call` | `Call` (do.go) |
| Stack refresh | `updatestack(ci)` | `L.Top = ci.Top` |
| TFORLOOP | Separate `goto l_tforloop` | Inlined — checks nil + jumps in same block |

---

## OP_TFORLOOP (C:1894 → Go:2322)

### C Implementation
```c
// lvm.c:1894
vmcase(OP_TFORLOOP) {
  l_tforloop: {
    StkId ra = RA(i);
    if (!ttisnil(s2v(ra + 3)))           // continue loop?
        pc -= GETARG_Bx(i);             // jump back
    vmbreak;
  }}
```

### Why Each Line
- **`ra + 3`**: First iterator return value. Nil = loop terminates.
- **`pc -= GETARG_Bx`**: Backward jump to loop body start.

### Go Implementation
```go
// vm.go:2322
case opcodeapi.OP_TFORLOOP:
    if !L.Stack[ra+3].Val.IsNil() {
        ci.SavedPC -= opcodeapi.GetArgBx(inst)
    }
```

**Note**: In go-lua, the `tforcall` label (vm.go:2554) already inlines the TFORLOOP nil-check, so `case OP_TFORLOOP` is only reached if TFORLOOP appears without preceding TFORCALL (shouldn't happen in valid bytecode).

---

## OP_ERRNNIL (C:1949 → Go:2528)

### C Implementation
```c
// lvm.c:1949
vmcase(OP_ERRNNIL) {
    TValue *ra = vRA(i);
    if (!ttisnil(ra))
        halfProtect(luaG_errnnil(L, cl, GETARG_Bx(i)));
    vmbreak;
}
```

### Why Each Line
- **Purpose**: Lua 5.5 global redefinition check. If value at `ra` is NOT nil, a global was already defined — raise error.
- **`luaG_errnnil`**: Generates error using constant at Bx index for the variable name.

### Go Implementation
```go
// vm.go:2528
case opcodeapi.OP_ERRNNIL:
    if !L.Stack[ra].Val.IsNil() {
        bx := opcodeapi.GetArgBx(inst)
        if bx > 0 {
            name := k[bx-1]
            if name.IsString() {
                RunError(L, "global '"+name.StringVal().String()+"' already defined")
            } else { RunError(L, "global already defined") }
        } else { RunError(L, "global already defined") }
    }
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Error generation | `luaG_errnnil` helper in ldebug.c | Inlined error message construction |
| Name lookup | Inside helper using Bx | `k[bx-1]` with bounds check |

---

## Generic For Loop Flow

```
OP_TFORPREP (once): swap ra+2↔ra+3, mark TBC, jump to TFORCALL
     │
     ▼
OP_TFORCALL: copy iter/state/control → ra+3..5, call iterator
     │
     ▼
OP_TFORLOOP: ra+3 == nil? → exit : jump back to loop body
     │                                    │
     └── loop body ◄──────────────────────┘
```

## Numeric For Loop Flow

```
OP_FORPREP (once): validate types, step≠0, compute count, rearrange slots
     │  (skip → jump past FORLOOP)
     ▼
     ┌── loop body ◄──────────┐
     │                        │
     ▼                        │
OP_FORLOOP: count--; idx+=step│
     continue? ───────────────┘
     exit → fall through
```

---

## Cross-Reference

| Function | C Location | Go Location |
|----------|-----------|-------------|
| `forlimit` | lvm.c:181 | vm.go:1137 (`forLimit`) |
| `forprep` | lvm.c:214 | vm.go:1211 (`ForPrep`) |
| `floatforloop` | lvm.c:273 | vm.go:1276 (inside `ForLoop`) |
| `MarkTBC` | — | do.go:1220 |
