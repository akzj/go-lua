# Closure, Vararg & Scope Opcodes — C ↔ Go Mapping

> Covers: OP_CLOSURE, OP_VARARG, OP_GETVARG, OP_VARARGPREP, OP_CLOSE, OP_TBC
>
> Source: `lua-master/lvm.c` + `lua-master/ltm.c` (C), `internal/vm/api/vm.go` (Go)

---

## OP_CLOSURE (C:1929 → Go:2441)

**Encoding**: iABx — `R[A] := closure(KPROTO[Bx])`

### C Implementation
```c
// lvm.c:1929
vmcase(OP_CLOSURE) {
  StkId ra = RA(i);
  Proto *p = cl->p->p[GETARG_Bx(i)];
  halfProtect(pushclosure(L, p, cl->upvals, base, ra));
  checkGC(L, ra + 1);
  vmbreak;
}
```

The `pushclosure` helper (lvm.c:834):
```c
static void pushclosure (lua_State *L, Proto *p, UpVal **encup,
                         StkId base, StkId ra) {
  int nup = p->sizeupvalues;
  Upvaldesc *uv = p->upvalues;
  int i;
  LClosure *ncl = luaF_newLclosure(L, nup);
  ncl->p = p;
  setclLvalue2s(L, ra, ncl);  /* anchor new closure in stack */
  for (i = 0; i < nup; i++) {
    if (uv[i].instack)  /* upvalue refers to local variable? */
      ncl->upvals[i] = luaF_findupval(L, base + uv[i].idx);
    else  /* get upvalue from enclosing function */
      ncl->upvals[i] = encup[uv[i].idx];
    luaC_objbarrier(L, ncl, ncl->upvals[i]);
  }
}
```

### Why Each Line
- `cl->p->p[Bx]`: Navigate from current closure → its Proto → child Proto at index Bx.
  Each function literal in source becomes a child Proto.
- `halfProtect(pushclosure(...))`: `halfProtect` saves PC before the call (so GC can
  find roots) but does NOT reload `base` afterward — safe because `pushclosure` doesn't
  move the stack.
- `luaF_newLclosure(L, nup)`: Allocate a new LClosure with `nup` upvalue slots.
- `setclLvalue2s(L, ra, ncl)`: Anchor closure in stack BEFORE filling upvalues.
  This prevents the GC from collecting the closure during upvalue resolution.
- **Upvalue capture loop**: For each upvalue declared in the child Proto:
  - `instack == true`: The variable is a local in the CURRENT frame. Call `luaF_findupval`
    to find or create an open UpVal pointing into the stack at `base + idx`.
  - `instack == false`: The variable is already captured by the enclosing closure.
    Reuse the parent's UpVal directly (`encup[idx]`).
- `luaC_objbarrier`: GC write barrier — the new closure references UpVal objects.
- `checkGC`: Closure allocation may trigger GC.

### Go Implementation
```go
// vm.go:2441 → calls PushClosure at vm.go:1314
case opcodeapi.OP_CLOSURE:
    bx := opcodeapi.GetArgBx(inst)
    p := cl.Proto.Protos[bx]
    PushClosure(L, p, cl.UpVals, base, ra)
```

`PushClosure` (vm.go:1314):
```go
func PushClosure(L *stateapi.LuaState, p *objectapi.Proto,
                 encup []*closureapi.UpVal, base, ra int) {
    nup := len(p.Upvalues)
    ncl := closureapi.NewLClosure(p, nup)
    L.Stack[ra].Val = objectapi.TValue{Tt: objectapi.TagLuaClosure, Val: ncl}
    for i := 0; i < nup; i++ {
        if p.Upvalues[i].InStack {
            ncl.UpVals[i] = closureapi.FindUpval(L, base+int(p.Upvalues[i].Idx))
        } else {
            ncl.UpVals[i] = encup[p.Upvalues[i].Idx]
        }
    }
}
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| GC barrier | Explicit `luaC_objbarrier` per upvalue | Not needed (Go GC traces references) |
| GC check | `checkGC` after closure creation | No explicit GC trigger |
| Stack anchor | `setclLvalue2s` before loop | Direct struct assignment before loop |
| Upvalue find | `luaF_findupval` (linked list walk) | `closureapi.FindUpval` (same logic) |

### How Upvalue Capture Works
```
Source:  local x = 1; function f() return x end

Proto of f declares: upvalues[0] = {instack=true, idx=<slot of x>}

At OP_CLOSURE time:
  instack=true → luaF_findupval(base + idx)
    → walks L->openupval linked list looking for matching stack level
    → if found, reuse it (sharing!)
    → if not found, create new UpVal pointing into the stack

When x's scope ends (OP_CLOSE):
  → luaF_close copies the stack value into UpVal.u.value
  → redirects UpVal.v.p to point at the internal copy
  → all closures sharing this UpVal now see the closed copy
```

---

## OP_VARARG (C:1936 → Go:2446)

**Encoding**: iABC — `R[A], R[A+1], ..., R[A+C-2] := vararg`

### C Implementation
```c
// lvm.c:1936
vmcase(OP_VARARG) {
  StkId ra = RA(i);
  int n = GETARG_C(i) - 1;  /* required results (-1 means all) */
  int vatab = GETARG_k(i) ? GETARG_B(i) : -1;
  Protect(luaT_getvarargs(L, ci, ra, n, vatab));
  vmbreak;
}
```

### Why Each Line
- `GETARG_C(i) - 1`: C=0 means "all varargs" (n=-1). C=1 means 0 results, etc.
  The -1 encoding avoids wasting C=0 as "zero results".
- `GETARG_k(i)`: The k bit indicates whether a vararg table exists (Lua 5.5 PF_VATAB).
  If k=1, B holds the register of the vararg table. If k=0, vatab=-1 (hidden args).
- `Protect(luaT_getvarargs(...))`: Full Protect because this may grow the stack
  (when n=-1 and there are many varargs). Protect saves/restores `pc` and `base`.
- **Two vararg storage modes** (Lua 5.5):
  - **PF_VAHID** (hidden): Extra args are stored below `ci->func` on the stack.
  - **PF_VATAB** (table): Extra args are stored in a table at a known register.

### Go Implementation
```go
// vm.go:2446
case opcodeapi.OP_VARARG:
    n := opcodeapi.GetArgC(inst) - 1
    vatab := -1
    if opcodeapi.GetArgK(inst) != 0 {
        vatab = opcodeapi.GetArgB(inst)
    }
    GetVarargs(L, ci, ra, n, vatab)
```

`GetVarargs` (vm.go:1394) mirrors `luaT_getvarargs` (ltm.c:338):
- If `vatab < 0`: reads from hidden stack slots below `ci.Func`
- If `vatab >= 0`: reads from the vararg table
- If `n < 0`: copies ALL available varargs, adjusts `L.Top`
- Pads with nil if fewer varargs available than requested

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Protect | Full `Protect` (may realloc stack) | `CheckStack` inside `GetVarargs` |
| Stack growth | `checkstackp` when n=-1 | `CheckStack` when n=-1 |
| Table read | `luaH_getint` for vatab path | `h.GetInt` for vatab path |

---

## OP_GETVARG (C:1943 → Go:2487)

**Encoding**: iABC — `R[A] := vararg[R[C]]` (Lua 5.5 single-value access)

### C Implementation
```c
// lvm.c:1943
vmcase(OP_GETVARG) {
  StkId ra = RA(i);
  TValue *rc = vRC(i);
  luaT_getvararg(ci, ra, rc);
  vmbreak;
}
```

`luaT_getvararg` (ltm.c:292):
```c
void luaT_getvararg (CallInfo *ci, StkId ra, TValue *rc) {
  int nextra = ci->u.l.nextraargs;
  lua_Integer n;
  if (tointegerns(rc, &n)) {          /* integral key? */
    if (l_castS2U(n) - 1 < cast_uint(nextra)) {
      StkId slot = ci->func.p - nextra + cast_int(n) - 1;
      setobjs2s(((lua_State*)NULL), ra, slot);
      return;
    }
  }
  else if (ttisstring(rc)) {          /* string key? */
    size_t len;
    const char *s = getlstr(tsvalue(rc), len);
    if (len == 1 && s[0] == 'n') {    /* key is "n"? */
      setivalue(s2v(ra), nextra);
      return;
    }
  }
  setnilvalue(s2v(ra));               /* else produce nil */
}
```

### Why Each Line
- WHY OP_GETVARG exists: New in Lua 5.5. Allows `arg[i]` to compile to a single
  instruction instead of VARARG + table index. Optimizes single-value vararg access.
- `tointegerns(rc, &n)`: Try to convert key to integer WITHOUT modifying it
  (the `ns` suffix = "no string coercion"). Float 2.0 → integer 2 is OK.
- `l_castS2U(n) - 1 < nextra`: Unsigned comparison handles negative indices
  (they wrap to huge unsigned values → out of range → nil).
- `key == "n"`: Returns the count of extra args, matching the vararg table convention.
- No `Protect` needed: GETVARG never allocates or moves the stack.

### Go Implementation
```go
// vm.go:2487
case opcodeapi.OP_GETVARG:
    rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
    switch rc.Tt {
    case objectapi.TagInteger:
        idx := rc.Val.(int64)
        nExtra := ci.NExtraArgs
        if uint64(idx-1) < uint64(nExtra) {
            varBase := ci.Func - nExtra
            L.Stack[ra].Val = L.Stack[varBase+int(idx)-1].Val
        } else {
            L.Stack[ra].Val = objectapi.Nil
        }
    case objectapi.TagFloat:
        // ... float-to-int conversion, same bounds check
    case objectapi.TagShortStr, objectapi.TagLongStr:
        s := rc.Val.(*objectapi.LuaString)
        if s.Data == "n" {
            L.Stack[ra].Val = objectapi.MakeInteger(int64(ci.NExtraArgs))
        } else {
            L.Stack[ra].Val = objectapi.Nil
        }
    default:
        L.Stack[ra].Val = objectapi.Nil
    }
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Type dispatch | `tointegerns` + `ttisstring` | Go `switch` on tag type |
| Float→int | `tointegerns` handles both int and float | Separate `TagFloat` case |
| Bounds check | `l_castS2U(n)-1 < nextra` (unsigned trick) | `uint64(idx-1) < uint64(nExtra)` (same trick) |
| Hidden args only | Always reads from stack (no vatab) | Same — GETVARG is hidden-args only |

---

## OP_VARARGPREP (C:1955 → Go:2454)

**Encoding**: iABC — Prepares vararg function frame

### C Implementation
```c
// lvm.c:1955
vmcase(OP_VARARGPREP) {
  ProtectNT(luaT_adjustvarargs(L, ci, cl->p));
  if (l_unlikely(trap)) {  /* previous "Protect" updated trap */
    luaD_hookcall(L, ci);
    L->oldpc = 1;  /* next opcode will be seen as a "new" line */
  }
  updatebase(ci);  /* function has new base after adjustment */
  vmbreak;
}
```

### Why Each Line
- `ProtectNT(luaT_adjustvarargs(...))`: `ProtectNT` = Protect but does NOT update
  `trap` from the call. The `NT` matters because `adjustvarargs` may trigger GC
  (stack reallocation), but we handle the trap check manually below.
- `luaT_adjustvarargs`: Two paths based on `Proto.flag`:
  - **PF_VATAB**: Creates a vararg table holding the extra args.
  - **PF_VAHID**: Shifts the stack — copies function + fixed params above the extra
    args, leaving the extras as "hidden" below the new `ci->func`.
- `if (trap)`: After adjustment, check if debug hooks are active.
  - `luaD_hookcall(L, ci)`: Fire the "call" hook (debugger notification).
  - `L->oldpc = 1`: Set OldPC to 1 (the VARARGPREP instruction itself). This ensures
    the NEXT instruction triggers a "new line" event in the line hook. Without this,
    the hook might think we're still on the same line as the caller.
- `updatebase(ci)`: Reload `base` from `ci->func + 1`. The hidden-args path moved
  `ci->func`, so the local `base` variable is stale.
- WHY VARARGPREP is always the FIRST instruction: The compiler places it at PC=0
  for every vararg function. It runs before any other opcode in the function body.

### Go Implementation
```go
// vm.go:2454
case opcodeapi.OP_VARARGPREP:
    AdjustVarargs(L, ci, cl.Proto)
    // Update base after adjustment
    base = ci.Func + 1
    // Set OldPC past VARARGPREP so the next instruction is seen as a
    // "new" line. Mirrors: OP_VARARGPREP in lvm.c sets L->oldpc = 1.
    if L.HookMask != 0 {
        L.OldPC = 1
    }
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Hook call | `luaD_hookcall` inside VARARGPREP | Hook already fired by `PreCall`/`CallHook` |
| Trap check | `if (trap)` after ProtectNT | `if L.HookMask != 0` (simpler check) |
| Base update | `updatebase(ci)` macro | Direct `base = ci.Func + 1` |

### Stack Layout (PF_VAHID Path)
```
BEFORE luaT_adjustvarargs:
  [func] [arg1] [arg2] [arg3] [extra1] [extra2]
   ^ ci->func                                    ^ L->top

AFTER (nfixparams=3, nextra=2):
  [func] [nil]  [nil]  [nil]  [extra1] [extra2] [func'] [arg1] [arg2] [arg3]
                                                  ^ ci->func (moved!)

  Hidden args (extra1, extra2) are below ci->func.
  ci->func now points to the copy of func.
  base = ci->func + 1 = slot of arg1 copy.
```

---

## OP_CLOSE (C:1634 → Go:1562)

**Encoding**: iABC — Close all upvalues and to-be-closed variables ≥ R[A]

### C Implementation
```c
// lvm.c:1634
vmcase(OP_CLOSE) {
  StkId ra = RA(i);
  lua_assert(!GETARG_B(i));  /* 'close must be alive */
  Protect(luaF_close(L, ra, LUA_OK, 1));
  vmbreak;
}
```

### Why Each Line
- `GETARG_B(i)` assertion: B=0 means the close is "alive" (not dead code).
- `luaF_close(L, ra, LUA_OK, 1)`: Close all open upvalues at or above stack level `ra`.
  - `LUA_OK`: Status code (no error — normal close).
  - `1`: "call close methods" flag — invoke `__close` metamethods on to-be-closed vars.
- `Protect`: `luaF_close` may call `__close` metamethods, which can trigger GC,
  errors, or stack reallocation. Full protection needed.
- WHY OP_CLOSE exists: Emitted at the end of blocks that contain local variables
  captured as upvalues or marked as to-be-closed. Without it, upvalues would remain
  "open" (pointing into dead stack slots) after the block exits.

### Go Implementation
```go
// vm.go:1562
case opcodeapi.OP_CLOSE:
    closureapi.CloseUpvals(L, ra)
    CloseTBC(L, ra)
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Implementation | Single `luaF_close` handles both upvals + TBC | Two separate calls |
| Error handling | `LUA_OK` status passed through | `CloseTBC` handles errors internally |
| Protect | Full `Protect` wrapper | No explicit protect (Go panics for errors) |

---

## OP_TBC (C:1640 → Go:1566)

**Encoding**: iABC — Mark R[A] as to-be-closed

### C Implementation
```c
// lvm.c:1640
vmcase(OP_TBC) {
  StkId ra = RA(i);
  /* create new to-be-closed upvalue */
  halfProtect(luaF_newtbcupval(L, ra));
  vmbreak;
}
```

### Why Each Line
- `luaF_newtbcupval(L, ra)`: Creates a new to-be-closed upvalue for the variable at
  stack slot `ra`. This links it into the TBC chain so `OP_CLOSE` (or function return)
  will call its `__close` metamethod.
- `halfProtect`: Saves PC before the call but does NOT reload `base` afterward.
  `luaF_newtbcupval` doesn't move the stack, so `base` stays valid.
- WHY TBC exists: Implements Lua 5.4+ `<close>` attribute:
  `local f <close> = io.open("file")` → compiler emits OP_TBC after the assignment.
  When the block exits, `__close` is called automatically (like Go's `defer`).

### Go Implementation
```go
// vm.go:1566
case opcodeapi.OP_TBC:
    // To-be-closed: mark the variable in the TBC linked list
    MarkTBC(L, ra)
```

`MarkTBC` is at `internal/vm/api/do.go:1220`.

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Mechanism | `luaF_newtbcupval` creates UpVal in TBC list | `MarkTBC` adds to TBC linked list |
| Protect | `halfProtect` (saves PC) | No explicit protect needed |
| Close trigger | `luaF_close` with `__close` metamethod | `CloseTBC` iterates TBC list |

---

## Summary Table

| Opcode | Format | C Line | Go Line | Semantics |
|--------|--------|--------|---------|-----------|
| OP_CLOSURE | iABx | 1929 | 2441 | R[A] := closure(KPROTO[Bx]) |
| OP_VARARG | iABC | 1936 | 2446 | R[A..A+C-2] := vararg |
| OP_GETVARG | iABC | 1943 | 2487 | R[A] := vararg[R[C]] (Lua 5.5) |
| OP_VARARGPREP | iABC | 1955 | 2454 | Prepare vararg frame (always PC=0) |
| OP_CLOSE | iABC | 1634 | 1562 | Close upvals + TBC vars ≥ R[A] |
| OP_TBC | iABC | 1640 | 1566 | Mark R[A] as to-be-closed |

## Key Architectural Differences

1. **Upvalue Capture**: Both C and Go use the same two-path algorithm (instack vs
   enclosing), but C requires explicit GC barriers (`luaC_objbarrier`) while Go
   relies on its concurrent GC.

2. **Vararg Storage**: Both support PF_VAHID (hidden args below ci.Func) and
   PF_VATAB (vararg table). The stack-shifting algorithm is identical.

3. **VARARGPREP Hook**: C fires `luaD_hookcall` inside VARARGPREP. Go fires the
   hook earlier during `PreCall`, so VARARGPREP only sets `OldPC = 1`.

4. **OP_CLOSE Split**: C uses a single `luaF_close` for both upvalue closing and
   TBC cleanup. Go splits this into `CloseUpvals` + `CloseTBC` for clarity.

5. **OP_GETVARG**: New in Lua 5.5. Both C and Go implement the same three-way
   dispatch (integer key → stack slot, "n" → count, else → nil). The unsigned
   bounds-check trick (`uint64(idx-1) < uint64(nExtra)`) is preserved exactly.
