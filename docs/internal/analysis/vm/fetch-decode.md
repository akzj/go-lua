# VM Fetch-Decode-Dispatch Mechanism

> C: `lua-master/lvm.c` lines 1098–1232 | Go: `internal/vm/vm.go` Execute() at line 1488

## Overview

The Lua VM is a register-based interpreter. Each iteration: fetch one 32-bit instruction,
decode the opcode, dispatch to the handler. C Lua uses macros for this; go-lua uses a
plain `for`/`switch` loop. The critical difference: C Lua has a **trap** mechanism for
hooks/signals that go-lua replaces with inline hook checks.

---

## 1. Function Entry — `luaV_execute` / `Execute`

### C Implementation

```c
// lvm.c:1198
void luaV_execute (lua_State *L, CallInfo *ci) {
  LClosure *cl;
  TValue *k;
  StkId base;
  const Instruction *pc;
  int trap;
#if LUA_USE_JUMPTABLE
#include "ljumptab.h"
#endif
 startfunc:                          // line 1207
  trap = L->hookmask;               // line 1208
 returning:  /* trap already set */  // line 1209
  cl = ci_func(ci);                  // line 1210
  k = cl->p->k;                     // line 1211
  pc = ci->u.l.savedpc;             // line 1212
  if (l_unlikely(trap))              // line 1213
    trap = luaG_tracecall(L);        // line 1214
  base = ci->func.p + 1;            // line 1215
```

### Why Each Line

| Line | Purpose |
|------|---------|
| `trap = L->hookmask` | Copy hook mask into local var. Non-zero = hooks active. Local copy avoids re-reading global state every iteration. |
| `startfunc:` label | Jumped to when entering a NEW Lua function (e.g., after OP_CALL). Resets trap from hookmask. |
| `returning:` label | Jumped to when RETURNING INTO this function. Trap is already set by caller — skip re-reading hookmask. |
| `cl = ci_func(ci)` | Extract LClosure from current CallInfo. |
| `k = cl->p->k` | Cache constant table pointer in local var for fast access. |
| `pc = ci->u.l.savedpc` | Load program counter from CallInfo into local. PC is only "saved" back when needed (see savepc). |
| `luaG_tracecall(L)` | Fire call hook if hooks active. Returns updated trap value (0 if hooks turned off by hook function). |
| `base = ci->func.p + 1` | Stack base = slot after function object. Register 0 = base+0. |

### Go Implementation

```go
// vm.go:1488
func Execute(L *stateapi.LuaState, ci *stateapi.CallInfo) {
    gState = L.Global                                          // 1490

startfunc:                                                     // 1492
    cl := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)     // 1493
    k := cl.Proto.Constants                                    // 1494
    code := cl.Proto.Code                                      // 1495
    base := ci.Func + 1                                        // 1496
```

### Key Differences — Function Entry

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| PC storage | Local `pc` pointer, copied from `ci->u.l.savedpc` | Uses `ci.SavedPC` index directly — no local copy |
| Trap variable | Local `int trap`, copied from `L->hookmask` | No trap variable; checks `L.HookMask` inline each iteration |
| Code access | Via `pc` pointer arithmetic (`*(pc++)`) | Via `code[ci.SavedPC]` slice indexing |
| `returning:` label | Separate entry point (trap already set) | No equivalent — no trap to preserve |
| `startfunc:` label | Goto target for new function entry | Go `goto startfunc` label at line 1492 |

---

## 2. The Main Loop — vmfetch / vmdispatch

### C Implementation

```c
// lvm.c:1185–1191  vmfetch macro
#define vmfetch()  { \
  if (l_unlikely(trap)) {                   /* hooks active? */  \
    trap = luaG_traceexec(L, pc);           /* fire line/count hook */ \
    updatebase(ci);                         /* stack may have moved */ \
  } \
  i = *(pc++);                              /* fetch and advance PC */ \
}

// lvm.c:1193–1195  dispatch macros
#define vmdispatch(o)   switch(o)
#define vmcase(l)       case l:
#define vmbreak         break

// lvm.c:1217–1232  main loop
  for (;;) {                                // line 1217
    Instruction i;                          // line 1218
    vmfetch();                              // line 1219
    vmdispatch (GET_OPCODE(i)) {            // line 1231
      vmcase(OP_MOVE) {                     // line 1232
```

### Why Each Line — vmfetch

| Line | Purpose |
|------|---------|
| `if (l_unlikely(trap))` | Branch hint: trap is usually 0. Only enter hook path when hooks are active. This keeps the fast path (no hooks) as a single `if` + fetch. |
| `luaG_traceexec(L, pc)` | Fire line hook (if PC crossed a line boundary) and count hook (decrement counter). Returns new trap value — hooks can disable themselves. |
| `updatebase(ci)` | `base = ci->func.p + 1`. Hook callbacks can trigger GC which can reallocate the stack, invalidating `base`. Must refresh after any hook call. |
| `i = *(pc++)` | Fetch instruction at current PC, then advance PC. Post-increment means PC points to NEXT instruction during handler execution. |

### Go Implementation

```go
// vm.go:1498–1515
for {
    inst := code[ci.SavedPC]                                   // 1499

    // Hook dispatch: fire line/count hooks if active.
    if L.HookMask&(stateapi.MaskLine|stateapi.MaskCount) != 0 && L.AllowHook &&
        opcodeapi.GetOpCode(inst) != opcodeapi.OP_VARARGPREP { // 1506-1508
        TraceExec(L, ci)                                       // 1509
    }
    ci.SavedPC++                                               // 1511
    op := opcodeapi.GetOpCode(inst)                            // 1512
    ra := base + opcodeapi.GetArgA(inst)                       // 1513

    switch op {                                                // 1515
```

### Key Differences — Fetch-Decode Loop

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Instruction fetch | `i = *(pc++)` — pointer deref + post-increment | `inst := code[ci.SavedPC]` — slice index |
| PC advance | Implicit in `*(pc++)` | Explicit `ci.SavedPC++` after hook check |
| Hook check | `if (trap)` — single local variable test | `if L.HookMask&(...) != 0 && L.AllowHook` — reads global state each time |
| Hook timing | Before fetch (hooks see current PC) | After fetch, before advance (same semantic PC) |
| VARARGPREP skip | Implicit: `luaG_tracecall` returns trap=0 for vararg | Explicit: `opcodeapi.GetOpCode(inst) != OP_VARARGPREP` |
| Dispatch | `switch(GET_OPCODE(i))` (or computed goto via `ljumptab.h`) | `switch op` (standard Go switch) |
| Computed goto | `#if LUA_USE_JUMPTABLE` enables GCC computed goto for ~15% speedup | Not available in Go — always uses switch |
| RA decode | Decoded per-opcode: `StkId ra = RA(i)` | Decoded once before switch: `ra := base + GetArgA(inst)` |

---

## 3. Trap Mechanism

The **trap** is C Lua's optimization for hook dispatch. It avoids checking `L->hookmask`
(a global struct field) on every instruction by caching it in a local variable.

### How Trap Works

```
1. Function entry: trap = L->hookmask        (lvm.c:1208)
2. Each vmfetch: if (trap) → call hooks      (lvm.c:1186)
3. After any Protect: updatetrap(ci)          (lvm.c:1113 — reads ci->u.l.trap)
4. After jumps: updatetrap(ci)                (lvm.c:1127 — dojump macro)
```

The `ci->u.l.trap` field is set by the debug system when hooks change. The local `trap`
variable is refreshed via `updatetrap` after any operation that might change hook state.

### Why go-lua Has No Trap

Go doesn't have local variables that persist across loop iterations in the same way C does.
The Go compiler already optimizes field access well. Checking `L.HookMask` directly is
simpler and the performance difference is negligible in Go's execution model.

```c
// lvm.c:1113
#define updatetrap(ci)  (trap = ci->u.l.trap)
```

In go-lua, the equivalent is simply re-reading `L.HookMask` each iteration (line 1506).

---

## 4. savepc / savestate Macros

### C Implementation

```c
// lvm.c:1140
#define savepc(ci)      (ci->u.l.savedpc = pc)

// lvm.c:1147
#define savestate(L,ci) (savepc(ci), L->top.p = ci->top.p)
```

### Why They Exist

C Lua keeps `pc` in a local variable for speed. But when calling into C functions (which
may raise errors), the error handler needs the current PC to generate error messages with
line numbers. `savepc` writes the local `pc` back to `ci->u.l.savedpc`.

`savestate` additionally sets `L->top` from `ci->top`. Some operations (like table access
metamethods) need a correct `top` to push arguments.

### Go Equivalent

go-lua has **no savepc** — it uses `ci.SavedPC` directly, so the global state is always
current. There is no local PC copy to sync.

`L.Top` management is explicit in go-lua — set before each operation that needs it.

---

## 5. Protect / ProtectNT / halfProtect

These macros wrap operations that can trigger errors, stack reallocation, or hook changes.

### C Implementation

```c
// lvm.c:1158
#define Protect(exp)     (savestate(L,ci), (exp), updatetrap(ci))

// lvm.c:1161
#define ProtectNT(exp)   (savepc(ci), (exp), updatetrap(ci))

// lvm.c:1167
#define halfProtect(exp) (savestate(L,ci), (exp))
```

### When Each Is Used

| Macro | Before | After | Used For |
|-------|--------|-------|----------|
| `Protect` | savepc + set top | updatetrap | Most operations: metamethods, finishget/finishset, close, varargs. Can raise errors, reallocate stack, change hooks. |
| `ProtectNT` | savepc only | updatetrap | Operations that don't change top: `luaV_concat`, `luaD_call`. |
| `halfProtect` | savepc + set top | nothing | Operations that can error but won't change hooks: `pushclosure`, `newtbcupval`, `errnnil`. |

### Usage Counts in lvm.c

```
Protect:     ~20 uses (finishget, finishset, trybinTM, close, equalobj, varargs...)
ProtectNT:   3 uses  (concat at 1630, call at 1888, adjustvarargs at 1956)
halfProtect: 4 uses  (newtbcupval at 1643/1869, pushclosure at 1932, errnnil at 1952)
```

### Go Equivalent

go-lua has **none of these macros**. Since `ci.SavedPC` is always current (no local PC),
and `L.Top` is managed explicitly, there's no need to "save state" before operations.
The trap update is unnecessary because go-lua checks `L.HookMask` each iteration.

| C Pattern | Go Equivalent |
|-----------|---------------|
| `Protect(luaV_finishget(...))` | Direct call: `FinishGet(L, ...)` |
| `ProtectNT(luaV_concat(...))` | Direct call: `Concat(L, n)` |
| `halfProtect(pushclosure(...))` | Direct call: `pushClosure(L, ...)` |

---

## 6. checkGC Macro

```c
// lvm.c:1178
#define checkGC(L,c)  \
    { luaC_condGC(L, (savepc(ci), L->top.p = (c)), \
                       updatetrap(ci)); \
      luai_threadyield(L); }
```

Triggers GC if needed. The `savepc` and `top=(c)` happen inside the GC condition
(only executed if GC runs). After GC, `updatetrap` refreshes hook state.
`luai_threadyield` is a no-op in standard Lua (unlock/lock for thread safety).

### Go Equivalent

go-lua calls `gc.CheckGC(L)` or `gc.Step(L)` explicitly where needed. No macro wrapping.

---

## 7. Jump Macros

```c
// lvm.c:1127
#define dojump(ci,i,e)  { pc += GETARG_sJ(i) + e; updatetrap(ci); }

// lvm.c:1130
#define donextjump(ci)  { Instruction ni = *pc; dojump(ci, ni, 1); }

// lvm.c:1137
#define docondjump()    if (cond != GETARG_k(i)) pc++; else donextjump(ci);
```

| Macro | Purpose |
|-------|---------|
| `dojump` | Advance PC by signed offset + extra `e`. Refreshes trap (tight loops need hook checks). |
| `donextjump` | Fetch the NEXT instruction (which must be a jump) and execute it. Used after conditional tests. |
| `docondjump` | If condition doesn't match expected `k` bit, skip the jump (pc++). Otherwise, do the jump. |

### Go Equivalent

```go
// vm.go — conditional jump pattern (e.g., OP_EQ at line 2133)
if cond != (opcodeapi.GetArgK(inst) != 0) {
    ci.SavedPC++                    // skip jump
} else {
    ni := code[ci.SavedPC]
    ci.SavedPC += opcodeapi.GetArgSJ(ni) + 1  // do jump
}
```

No `updatetrap` needed — go-lua checks hooks at loop top every iteration.

---

## 8. Register Access Macros

```c
// lvm.c:1102–1110
#define RA(i)     (base+GETARG_A(i))
#define RB(i)     (base+GETARG_B(i))
#define KB(i)     (k+GETARG_B(i))
#define RC(i)     (base+GETARG_C(i))
#define KC(i)     (k+GETARG_C(i))
#define RKC(i)    ((TESTARG_k(i)) ? k + GETARG_C(i) : s2v(base + GETARG_C(i)))
```

| Macro | Meaning |
|-------|---------|
| `RA(i)` | Register A — destination for most ops |
| `RB(i)` / `RC(i)` | Register B/C — source operands |
| `KB(i)` / `KC(i)` | Constant B/C — index into constant table |
| `RKC(i)` | Register-or-Constant C — if k-bit set, use constant; else register |

### Go Equivalent

```go
ra := base + opcodeapi.GetArgA(inst)          // decoded once before switch
rb := base + opcodeapi.GetArgB(inst)          // decoded per-case
// For RKC pattern:
if opcodeapi.GetArgK(inst) != 0 {
    rc = k[opcodeapi.GetArgC(inst)]           // constant
} else {
    rc = L.Stack[base+opcodeapi.GetArgC(inst)].Val  // register
}
```

---

## Summary: C vs Go VM Loop Architecture

| Aspect | C Lua (`lvm.c`) | go-lua (`vm.go`) |
|--------|-----------------|------------------|
| PC storage | Local `const Instruction *pc` | `ci.SavedPC` (int index) |
| Instruction fetch | `*(pc++)` pointer deref | `code[ci.SavedPC]` slice index |
| Dispatch | `switch` or computed goto | `switch` only |
| Hook optimization | `trap` local variable | Direct `L.HookMask` check |
| State save | `savepc`/`savestate` macros | Not needed (PC always in ci) |
| Error protection | `Protect`/`ProtectNT`/`halfProtect` | Direct function calls |
| Stack base | `StkId base` (pointer) | `base int` (index) |
| GC trigger | `checkGC` macro | Explicit `gc.CheckGC()` calls |
| Jump execution | `dojump`/`donextjump`/`docondjump` | Inline `ci.SavedPC +=` arithmetic |
