# Call / Return / Error / Stack — ldo.c Deep Analysis

**Lua 5.5.1** | Source: `lua-master/ldo.c` (1172 lines), `lua-master/ldo.h` (99 lines), `lua-master/lstate.h` (451 lines), `lua-master/lvm.c`

---

## Table of Contents

1. [CallInfo Structure](#1-callinfo-structure-from-lstateh)
2. [luaD_precall](#2-luad_precall-ldoc723-754)
3. [luaD_poscall](#3-luad_poscall-ldoc613-623)
4. [luaD_call / luaD_callnoyield](#4-luad_call--luad_callnoyield-ldoc783-793)
5. [Stack Management](#5-stack-management)
6. [Error Handling](#6-error-handling)
7. [Protected Calls: luaD_pcall](#7-protected-calls-luad_pcall-ldoc1089-1106)
8. [Coroutine Mechanics](#8-coroutine-mechanics)
9. [The CRITICAL ldo.c ↔ lvm.c Interaction](#9-the-critical-ldoc--lvmc-interaction)
10. [Complete Call Sequence Pseudocode](#10-complete-call-sequence-pseudocode)
11. [If I Were Building This in Go](#11-if-i-were-building-this-in-go)
12. [Edge Cases](#12-edge-cases)
13. [Bug Pattern Guide](#13-bug-pattern-guide)

---

## 1. CallInfo Structure (from lstate.h)

**Location**: `lstate.h:187-209`

`CallInfo` is the activation record for a single function call. It forms a **doubly-linked list** on the Lua stack's call chain.

```c
// lstate.h:187-209
struct CallInfo {
  StkIdRel func;              /* function index in the stack */
  StkIdRel top;                /* top for this function */
  struct CallInfo *previous;   /* dynamic call link — caller */
  struct CallInfo *next;       /* dynamic call link — callee (allocated on demand) */
  union {
    struct {                   /* only for Lua functions */
      const Instruction *savedpc;  /* current program counter */
      volatile l_signalT trap;    /* function is tracing lines/counts */
      int nextraargs;             /* # of extra arguments in vararg functions */
    } l;
    struct {                   /* only for C functions */
      lua_KFunction k;            /* continuation in case of yields */
      ptrdiff_t old_errfunc;      /* saved error function */
      lua_KContext ctx;           /* context info. in case of yields */
    } c;
  } u;
  union {
    int funcidx;   /* called-function index (protected call) */
    int nyield;    /* number of values yielded */
    int nres;     /* number of values returned */
  } u2;
  l_uint32 callstatus;   /* status flags */
};
```

### Field-by-Field Breakdown

| Field | Type | Purpose |
|---|---|---|
| `func` | `StkIdRel` | Index of the function slot on the Lua stack. `func.p` is the actual pointer (offset-based for realloc safety). For Lua functions, `base = func.p + 1`. |
| `top` | `StkIdRel` | One-past-the-last valid slot for this activation. Used to validate `L->top <= ci->top`. |
| `previous` | `CallInfo*` | Pointer to caller's `CallInfo`. `base_ci` (the root) has `previous = NULL`. |
| `next` | `CallInfo*` | Pointer to callee's `CallInfo`. Usually `NULL` (allocated lazily via `luaE_extendCI`). |

### Union `u` — Function-Type Specific Data

**For Lua functions** (`u.l`):
- `savedpc`: The instruction pointer. Points to the **next** instruction to execute. Critical for debugging, error messages, and yield resumption.
- `trap`: Flag set by `luaG_tracecall` when hooks are active. Triggers line-tracing inside `luaV_execute`.
- `nextraargs`: Number of vararg arguments beyond the fixed parameters. Only non-zero for vararg functions.

**For C functions** (`u.c`):
- `k`: Continuation function pointer. When a C function calls `lua_yieldk`, the coroutine can later resume by calling `k`. NULL for non-resumable C functions.
- `old_errfunc`: The error-handler stack index that was active when this C call started. Restored by `finishCcall`.
- `ctx`: Opaque context passed to `k` on continuation.

### Union `u2` — Ephemeral State

- `funcidx`: Used only by protected calls (`CIST_YPCALL`). Stores the stack offset of the called function so it can be recovered after an unwind.
- `nyield`: Number of values returned by the last `lua_yieldk`. Set by `lua_yieldk:1028`, read by `lua_resume:1002`.
- `nres`: Number of values returned by the function. Used by `CIST_CLSRET` (close-to-be-closed return) and vararg adjustments.

### callstatus Flags (lstate.h:221-251)

These 32 bits encode everything about the call's state:

```c
// lstate.h:222-251
#define CIST_NRESULTS  0xffu              /* bits 0-7: nresults + 1 (max 255) */

// bits 8-11: count of __call metamethods invoked (for overflow detection)
#define CIST_CCMT      8                   /* the offset, not the mask */
#define MAX_CCMT       (0xfu << CIST_CCMT) /* max 15 metamethod calls */

// bits 12-14: CIST_RECST — 3-bit error status for coroutine recovery
#define CIST_RECST     12

#define CIST_C         (1u << (CIST_RECST + 3))     /* call is running a C function */
#define CIST_FRESH     (CIST_C << 1)                 /* fresh luaV_execute frame */
#define CIST_CLSRET    (CIST_FRESH << 1)             /* closing tbc variables on return */
#define CIST_TBC       (CIST_CLSRET << 1)            /* has to-be-closed variables */
#define CIST_OAH       (CIST_TBC << 1)               /* original allowhook value */
#define CIST_HOOKED    (CIST_OAH << 1)               /* running a debug hook */
#define CIST_YPCALL    (CIST_HOOKED << 1)            /* yieldable protected call */
#define CIST_TAIL      (CIST_YPCALL << 1)            /* call was tail called */
#define CIST_HOOKYIELD (CIST_TAIL << 1)              /* last hook call yielded */
#define CIST_FIN       (CIST_HOOKYIELD << 1)          /* function called a finalizer */
```

**Key flag interactions**:
- `isLua(ci)` = `!((ci)->callstatus & CIST_C)` — true if Lua function.
- `isLuacode(ci)` = `!((ci)->callstatus & (CIST_C | CIST_HOOKED))` — true if running Lua bytecode (not C and not a hook).
- `CIST_FRESH` + `CIST_TAIL` are **mutually exclusive** in practice: a fresh call is never a tail call.
- `CIST_TBC | CIST_NRESULTS` encodes what the **caller** wanted — used by `luaD_poscall` to decide how many results to keep.

### The CallInfo Linked List

```
┌─────────────────────────────────────────────────────────┐
│  lua_State                                               │
│  ┌──────────┐                                           │
│  │ base_ci  │◄── root CI (C host level)                 │
│  └────┬─────┘     previous = NULL                        │
│       │                                                │
│       ▼                                                │
│  ┌──────────┐                                           │
│  │ ci[0]    │ ◄── L->ci (current call)                  │
│  │ func ─────────► [func slot on stack]                  │
│  │ top ───────────► [slot past last valid for this call]│
│  │ previous ───────► base_ci                             │
│  │ next    │ NULL  (not allocated until needed)           │
│  └────┬─────┘                                           │
│       │                                                │
│       ▼ (allocated lazily when luaD_precall is called)  │
│  ┌──────────┐                                           │
│  │ ci[1]    │ ◄── next CI (callee, if any)              │
│  │ ...      │                                           │
│  └──────────┘                                           │
└─────────────────────────────────────────────────────────┘
```

- `L->ci` always points to the **current** CallInfo.
- `base_ci` is the root — always present, allocated as part of `lua_State`. It has `previous = NULL`.
- `ci->next` is **lazily allocated** via `luaE_extendCI` when needed (ldo.c:627).
- `nci` (lstate.h:304) counts total items in the CI list.

---

## 2. luaD_precall (ldo.c:723-754)

**Purpose**: Prepare a function call. For C functions, it also **executes** them. For Lua functions, it allocates a CallInfo and returns it for `luaV_execute` to handle.

```c
// ldo.c:723-754
CallInfo *luaD_precall (lua_State *L, StkId func, int nresults) {
  unsigned status = cast_uint(nresults + 1);    // encode nresults in callstatus
  lua_assert(status <= MAXRESULTS + 1);
 retry:
  switch (ttypetag(s2v(func))) {
    case LUA_VCCL:    // C closure
      precallC(L, func, status, clCvalue(s2v(func))->f);
      return NULL;
    case LUA_VLCF:    // light C function
      precallC(L, func, status, fvalue(s2v(func)));
      return NULL;
    case LUA_VLCL: {  // Lua function
      CallInfo *ci;
      Proto *p = clLvalue(s2v(func))->p;
      int narg = cast_int(L->top.p - func) - 1;     // number of arguments
      int nfixparams = p->numparams;
      int fsize = p->maxstacksize;                   // frame size
      checkstackp(L, fsize, func);
      L->ci = ci = prepCallInfo(L, func, status, func + 1 + fsize);
      ci->u.l.savedpc = p->code;                     // start at first instruction
      for (; narg < nfixparams; narg++)
        setnilvalue(s2v(L->top.p++));                 // pad missing fixed args with nil
      lua_assert(ci->top.p <= L->stack_last.p);
      return ci;                                     // caller must call luaV_execute
    }
    default: {                                        // not a function
      checkstackp(L, 1, func);
      status = tryfuncTM(L, func, status);            // try __call metamethod
      goto retry;
    }
  }
}
```

### Three Execution Paths

**Path 1 — C closure (`LUA_VCCL`) or Light C function (`LUA_VLCF`)**:
1. Calls `precallC` (ldo.c:650), which:
   - Ensures minimum stack space (`LUA_MINSTACK`).
   - Allocates a new CallInfo via `prepCallInfo` with `CIST_C` flag set.
   - Calls the C function directly: `n = (*f)(L)`.
   - Calls `luaD_poscall(L, ci, n)`.
2. Returns `NULL` — no further action needed from caller.

**Path 2 — Lua function (`LUA_VLCL`)**:
1. Gets `Proto*` from the closure.
2. Computes frame requirements: `fsize = p->maxstacksize`.
3. Calls `prepCallInfo` which:
   - Calls `next_ci(L)` to allocate a new CallInfo if needed.
   - Sets `func`, `top`, and `callstatus`.
4. Sets `savedpc = p->code` (entry point).
5. Nil-pads missing fixed arguments.
6. Returns the `ci` pointer — **caller must invoke `luaV_execute(L, ci)`**.

**Path 3 — Not callable (default)**:
1. Calls `tryfuncTM` to look up `__call` metamethod.
2. `goto retry` to re-check the type — the metamethod is now on the stack where `func` was.
3. Tracks metamethod depth in `status` bits (`CIST_CCMT`). Overflow → " '__call' chain too long".

### Key Helper: `prepCallInfo` (ldo.c:636-644)

```c
// ldo.c:636-644
l_sinline CallInfo *prepCallInfo (lua_State *L, StkId func, unsigned status,
                                  StkId top) {
  CallInfo *ci = L->ci = next_ci(L);    // allocate new frame, update L->ci
  ci->func.p = func;
  lua_assert((status & ~(CIST_NRESULTS | CIST_C | MAX_CCMT)) == 0);
  ci->callstatus = status;
  ci->top.p = top;
  return ci;
}
```

### Key Helper: `next_ci` Macro (ldo.c:627)

```c
// ldo.c:627
#define next_ci(L)  (L->ci->next ? L->ci->next : luaE_extendCI(L, 1))
```

If `ci->next` is already allocated, reuse it. Otherwise, call `luaE_extendCI` (lstate.c) to allocate a new CallInfo node.

---

## 3. luaD_poscall (ldo.c:613-623)

**Purpose**: Called **after** a function returns. Moves results to the caller's frame, handles hooks, and unwinds the CallInfo chain.

```c
// ldo.c:613-623
void luaD_poscall (lua_State *L, CallInfo *ci, int nres) {
  l_uint32 fwanted = ci->callstatus & (CIST_TBC | CIST_NRESULTS);
  if (l_unlikely(L->hookmask) && !(fwanted & CIST_TBC))
    rethook(L, ci, nres);
  moveresults(L, ci->func.p, nres, fwanted);     // move results to caller's frame
  lua_assert(!(ci->callstatus &
        (CIST_HOOKED | CIST_YPCALL | CIST_FIN | CIST_CLSRET)));
  L->ci = ci->previous;                           // unwind CI chain
}
```

### How moveresults Works (ldo.c:547-604)

`moveresults` dispatches on `fwanted` (what the caller wanted):

| fwanted value | Meaning | Action |
|---|---|---|
| `0 + 1` | Caller wants 0 results | `L->top.p = res` (discard all) |
| `1 + 1` | Caller wants 1 result | Move 1 result or nil; `top = res + 1` |
| `LUA_MULTRET + 1` | Caller wants all results (`...`) | `genmoveresults` with `nres` |
| `default` (≥2+1) | Caller wants 2+ results | Handle CIST_TBC closing, `genmoveresults` |

The `+1` offset is because `nresults` of `0` must be distinguishable from "not set", so nresults+1 is stored.

```c
// ldo.c:569-604 — moveresults dispatch
l_sinline void moveresults (lua_State *L, StkId res, int nres,
                                          l_uint32 fwanted) {
  switch (fwanted) {
    case 0 + 1:
      L->top.p = res;
      return;
    case 1 + 1:
      if (nres == 0)
        setnilvalue(s2v(res));
      else
        setobjs2s(L, res, L->top.p - nres);
      L->top.p = res + 1;
      return;
    case LUA_MULTRET + 1:
      genmoveresults(L, res, nres, nres);
      break;
    default: {
      int wanted = get_nresults(fwanted);
      if (fwanted & CIST_TBC) {
        L->ci->u2.nres = nres;
        L->ci->callstatus |= CIST_CLSRET;
        res = luaF_close(L, res, CLOSEKTOP, 1);
        L->ci->callstatus &= ~CIST_CLSRET;
        if (L->hookmask) {
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

`genmoveresults` (ldo.c:548-559) copies at most `wanted` results from the returned position to the expected position, filling missing ones with nil.

---

## 4. luaD_call / luaD_callnoyield (ldo.c:783-793)

These are thin wrappers around `ccall`:

```c
// ldo.c:783-793
void luaD_call (lua_State *L, StkId func, int nResults) {
  ccall(L, func, nResults, 1);
}

void luaD_callnoyield (lua_State *L, StkId func, int nResults) {
  ccall(L, func, nResults, nyci);    // nyci = 0x10000 | 1
}
```

### ccall — The Core (ldo.c:765-777)

```c
// ldo.c:765-777
l_sinline void ccall (lua_State *L, StkId func, int nResults, l_uint32 inc) {
  CallInfo *ci;
  L->nCcalls += inc;
  if (l_unlikely(getCcalls(L) >= LUAI_MAXCCALLS)) {
    checkstackp(L, 0, func);
    luaE_checkcstack(L);
  }
  if ((ci = luaD_precall(L, func, nResults)) != NULL) {  // Lua function
    ci->callstatus |= CIST_FRESH;                        // mark fresh frame
    luaV_execute(L, ci);                                 // execute
  }
  L->nCcalls -= inc;
}
```

**Critical invariant**: `luaV_execute` **never returns normally** via its `ret` label when `CIST_FRESH` is set — it returns to `ccall`. When an error occurs inside `luaV_execute`, it longjmps out via `luaD_throw`, bypassing the normal return path entirely.

The `inc` parameter controls yieldability:
- `inc = 1`: normal call, `nCcalls` incremented by 1. C stack depth tracked in low 16 bits.
- `inc = nyci = 0x10000 | 1`: non-yieldable call. Sets high 16 bits (making `yieldable()` return false) AND increments C stack depth. Used by `luaD_callnoyield` and protected parsers.

---

## 5. Stack Management

### Stack Layout (lstate.h, ldo.h)

```c
// lobject.h:158 — StkId is a pointer to StackValue
typedef StackValue *StkId;

// lobject.h:166 — StkIdRel stores pointer as offset for realloc safety
typedef struct {
  StkId p;  /* actual pointer */
} StkIdRel;
```

The Lua stack is a **dynamically allocated array of `StackValue`** (each 16 bytes — a `TValue`):

```
┌─────────────────────────────────────────────────────────────┐
│  L->stack.p                                                │
├────────────┬─────────────────────────────┬──────────────────┤
│  slots     │  used by current call       │  L->stack_last  │
│  [0..base) │  [base .. L->top.p - 1]    │  L->top.p        │
│  (caller's │                            │  (first free)    │
│   args)    │                            │                  │
└────────────┴─────────────────────────────┴──────────────────┘
```

- `L->stack.p`: base pointer (returned by allocator).
- `L->stack_last.p`: one-past-last usable slot (`L->stack.p + size`).
- `L->top.p`: first **free** slot.
- `BASIC_STACK_SIZE = 2 * LUA_MINSTACK` (ldo.h:156).
- `EXTRA_STACK = 5` (lstate.h:142): reserved space for error handling beyond `stack_last`.
- `LUAI_MAXSTACK = 1000000` (or `INT_MAX/2`) — the maximum logical stack size (ldo.c:192).

### Growing: luaD_growstack (ldo.c:361-388)

```c
int luaD_growstack (lua_State *L, int n, int raiseerror) {
  int size = stacksize(L);
  if (l_unlikely(size > MAXSTACK)) {
    // already at ERRORSTACKSIZE, cannot grow further
    if (raiseerror) luaD_errerr(L);
    return 0;
  }
  else if (n < MAXSTACK) {
    int newsize = size + (size >> 1);  // size * 1.5
    int needed = cast_int(L->top.p - L->stack.p) + n;
    if (newsize > MAXSTACK) newsize = MAXSTACK;
    if (newsize < needed) newsize = needed;
    if (l_likely(newsize <= MAXSTACK))
      return luaD_reallocstack(L, newsize, raiseerror);
  }
  // stack overflow
  luaD_reallocstack(L, ERRORSTACKSIZE, raiseerror);
  if (raiseerror) luaG_runerror(L, "stack overflow");
  return 0;
}
```

Growth strategy:
1. **Normal growth**: `size * 1.5`, but at least `needed`.
2. **Overflow**: grow to `ERRORSTACKSIZE` (MAXSTACK + 200), then raise `"stack overflow"`.
3. The `raiseerror=0` path is used internally (e.g., GC shrink path) to check if growth would succeed.

### Reallocating: luaD_reallocstack (ldo.c:330-354)

```c
int luaD_reallocstack (lua_State *L, int newsize, int raiseerror) {
  int oldsize = stacksize(L);
  StkId oldstack = L->stack.p;
  lu_byte oldgcstop = G(L)->gcstopem;
  lua_assert(newsize <= MAXSTACK || newsize == ERRORSTACKSIZE);
  relstack(L);                              // pointers → offsets
  G(L)->gcstopem = 1;                       // stop emergency GC
  newstack = luaM_reallocvector(L, oldstack, oldsize + EXTRA_STACK,
                                 newsize + EXTRA_STACK, StackValue);
  G(L)->gcstopem = oldgcstop;               // restore GC
  if (l_unlikely(newstack == NULL)) {
    correctstack(L, oldstack);              // restore pointers on failure
    if (raiseerror) luaM_error(L);
    else return 0;
  }
  L->stack.p = newstack;
  correctstack(L, oldstack);                // offsets → pointers
  L->stack_last.p = L->stack.p + newsize;
  // erase new segment with nil
  for (i = oldsize + EXTRA_STACK; i < newsize + EXTRA_STACK; i++)
    setnilvalue(s2v(newstack + i));
  return 1;
}
```

### Pointer Safety: relstack / correctstack (ldo.c:256-321)

The **critical** mechanism that makes stack reallocation safe:

```c
// ldo.c:260-271 — convert all pointers to offsets
static void relstack (lua_State *L) {
  L->top.offset = savestack(L, L->top.p);
  L->tbclist.offset = savestack(L, L->tbclist.p);
  for (up = L->openupval; up != NULL; up = up->u.open.next)
    up->v.offset = savestack(L, uplevel(up));
  for (ci = L->ci; ci != NULL; ci = ci->previous) {
    ci->top.offset = savestack(L, ci->top.p);
    ci->func.offset = savestack(L, ci->func.p);
  }
}

// ldo.c:277-291 — convert offsets back to pointers
static void correctstack (lua_State *L, StkId oldstack) {
  L->top.p = restorestack(L, L->top.offset);
  L->tbclist.p = restorestack(L, L->tbclist.offset);
  for (up = L->openupval; ...)
    up->v.p = s2v(restorestack(L, up->v.offset));
  for (ci = L->ci; ...)
    ci->top.p = restorestack(L, ci->top.offset);
    ci->func.p = restorestack(L, ci->func.offset);
    if (isLua(ci))
      ci->u.l.trap = 1;     // signal luaV_execute to update trap
}
```

`LUAI_STRICT_ADDRESS=1` (default) converts to proper offsets. When `0`, uses `pointer - oldstack + newstack` arithmetic which is faster but not strictly ISO C compliant.

### Shrinking: luaD_shrinkstack (ldo.c:419-431)

```c
void luaD_shrinkstack (lua_State *L) {
  int inuse = stackinuse(L);    // max of all ci->top and current L->top
  int max = (inuse > MAXSTACK / 3) ? MAXSTACK : inuse * 3;
  if (inuse <= MAXSTACK && stacksize(L) > max)
    luaD_reallocstack(L, (inuse > MAXSTACK / 2) ? MAXSTACK : inuse * 2, 0);
  else
    condmovestack(L,(void)0,(void)0);   // debugging only
  luaE_shrinkCI(L);                     // also shrink CallInfo list
}
```

Shrinks if the stack is more than **3×** the current usage. `luaE_shrinkCI` frees excess CallInfo nodes.

### Savestack / Restorestack Macros (ldo.h:45-46)

```c
// ldo.h:45-46
#define savestack(L,pt)      (cast_charp(pt) - cast_charp(L->stack.p))
#define restorestack(L,n)    cast(StkId, cast_charp(L->stack.p) + (n))
```

Convert pointer ↔ integer offset. Used everywhere stack reallocation can happen.

---

## 6. Error Handling

### lua_longjmp — Chained Jump Buffers (ldo.c:61-65)

```c
// ldo.c:61-65
typedef struct lua_longjmp {
  struct lua_longjmp *previous;   /* chain of error handlers */
  jmp_buf b;                      /* platform jump buffer */
  volatile TStatus status;         /* error code */
} lua_longjmp;
```

Forms a **stack of error handlers**. `L->errorJmp` points to the current handler.

### LUAI_THROW / LUAI_TRY (ldo.c:74-109)

Three implementations depending on platform:

```c
// POSIX (most efficient — uses _setjmp which saves signal context)
#define LUAI_THROW(L,c)    _longjmp((c)->b, 1)
#define LUAI_TRY(L,c,f,ud) if (_setjmp((c)->b) == 0) ((f)(L, ud))

// ISO C (uses setjmp, less efficient)
#define LUAI_THROW(L,c)    longjmp((c)->b, 1)
#define LUAI_TRY(L,c,f,ud) if (setjmp((c)->b) == 0) ((f)(L, ud))

// C++ (uses C++ exceptions)
#define LUAI_THROW(L,c)    throw(c)
```

### luaD_throw (ldo.c:125-147) — The Throw Entry Point

```c
l_noret luaD_throw (lua_State *L, TStatus errcode) {
  if (L->errorJmp) {                    // has error handler?
    L->errorJmp->status = errcode;
    LUAI_THROW(L, L->errorJmp);         // longjmp to it
  }
  // No handler — propagate to thread's creator
  global_State *g = G(L);
  lua_State *mainth = mainthread(g);
  errcode = luaE_resetthread(L, errcode);  // close upvalues, mark thread dead
  L->status = errcode;
  if (mainth->errorJmp) {
    setobjs2s(L, mainth->top.p++, L->top.p - 1);
    luaD_throw(mainth, errcode);
  }
  else {
    if (g->panic) { g->panic(L); }      // last resort
    abort();
  }
}
```

Three levels of escalation:
1. **Local handler** (`L->errorJmp`): longjmp to the protected frame.
2. **Thread's main thread**: if thread has no handler, close its upvalues and re-throw on `mainthread`.
3. **Panic / abort**: no handler anywhere → call `g->panic`, then `abort()`.

### luaD_rawrunprotected (ldo.c:160-170) — The Protected Call Wrapper

```c
TStatus luaD_rawrunprotected (lua_State *L, Pfunc f, void *ud) {
  l_uint32 oldnCcalls = L->nCcalls;
  lua_longjmp lj;
  lj.status = LUA_OK;
  lj.previous = L->errorJmp;        // chain new handler
  L->errorJmp = &lj;
  LUAI_TRY(L, &lj, f, ud);          // call f(L, ud) with setjmp
  L->errorJmp = lj.previous;         // restore old handler
  L->nCcalls = oldnCcalls;
  return lj.status;                 // OK or error code
}
```

This is the **foundation** of all protected execution in Lua. Creates a local `lua_longjmp`, chains it onto `L->errorJmp`, calls the function inside `LUAI_TRY` (which calls `setjmp`/`_setjmp`), and restores state on both success and error.

---

## 7. Protected Calls: luaD_pcall (ldo.c:1089-1106)

```c
TStatus luaD_pcall (lua_State *L, Pfunc func, void *u, ptrdiff_t old_top,
                                  ptrdiff_t ef) {
  TStatus status;
  CallInfo *old_ci = L->ci;
  lu_byte old_allowhooks = L->allowhook;
  ptrdiff_t old_errfunc = L->errfunc;
  L->errfunc = ef;
  status = luaD_rawrunprotected(L, func, u);
  if (l_unlikely(status != LUA_OK)) {
    L->ci = old_ci;
    L->allowhook = old_allowhooks;
    status = luaD_closeprotected(L, old_top, status);
    luaD_seterrorobj(L, status, restorestack(L, old_top));
    luaD_shrinkstack(L);
  }
  L->errfunc = old_errfunc;
  return status;
}
```

**Saved state** (restored on error):
- `L->ci`: back to the caller's frame.
- `L->allowhook`: restore hook permission.
- `L->errfunc`: restore error handler function index.
- `L->top`: restored by `luaD_seterrorobj`.

`luaD_closeprotected` (ldo.c:1067-1081) calls `luaF_close` on all to-be-closed variables in a loop, protecting each close call. If a `__close` method itself errors, it loops and tries again with the next variable.

---

## 8. Coroutine Mechanics

### lua_yieldk (ldo.c:1014-1042)

```c
LUA_API int lua_yieldk (lua_State *L, int nresults, lua_KContext ctx,
                        lua_KFunction k) {
  CallInfo *ci;
  luai_userstateyield(L, nresults);
  lua_lock(L);
  ci = L->ci;
  api_checkpop(L, nresults);
  if (l_unlikely(!yieldable(L))) {
    if (L != mainthread(G(L)))
      luaG_runerror(L, "attempt to yield across a C-call boundary");
    else
      luaG_runerror(L, "attempt to yield from outside a coroutine");
  }
  L->status = LUA_YIELD;
  ci->u2.nyield = nresults;
  if (isLua(ci)) {                    // inside hook
    lua_assert(!isLuacode(ci));
    api_check(L, nresults == 0, "hooks cannot yield values");
    api_check(L, k == NULL, "hooks cannot continue after yielding");
  }
  else {
    if ((ci->u.c.k = k) != NULL)      // save continuation
      ci->u.c.ctx = ctx;
    luaD_throw(L, LUA_YIELD);        // longjmp out
  }
  lua_assert(ci->callstatus & CIST_HOOKED);
  lua_unlock(L);
  return 0;                           // return to luaD_hook
}
```

**Two yield paths**:
1. **Inside a hook** (`isLua(ci)`): Returns to `luaD_hook`. No continuation possible. `nresults` must be 0.
2. **Normal yield**: Saves continuation `k`/`ctx`, calls `luaD_throw(L, LUA_YIELD)` which longjmps to the nearest `luaD_rawrunprotected` frame (the `lua_resume` wrapper).

### lua_resume (ldo.c:974-1006)

```c
LUA_API int lua_resume (lua_State *L, lua_State *from, int nargs, int *nresults) {
  TStatus status;
  lua_lock(L);
  if (L->status == LUA_OK) {               // initial start
    if (L->ci != &L->base_ci)              // not at base level?
      return resume_error(...);
    else if (L->top.p - (L->ci->func.p + 1) == nargs)
      return resume_error(...);            // no function to call
    ccall(L, firstArg - 1, LUA_MULTRET, 0);// start coroutine body
  }
  else {                                    // resuming from yield
    lua_assert(L->status == LUA_YIELD);
    L->status = LUA_OK;
    if (isLua(ci)) {                        // yielded in hook
      ci->u.l.savedpc--;
      L->top.p = firstArg;
      luaV_execute(L, ci);                 // continue
    }
    else {                                  // common yield
      if (ci->u.c.k != NULL) {
        lua_unlock(L);
        n = (*ci->u.c.k)(L, LUA_YIELD, ci->u.c.ctx);
        lua_lock(L);
        api_checknelems(L, n);
      }
      luaD_poscall(L, ci, n);
      unroll(L, NULL);
    }
  }
  status = luaD_rawrunprotected(L, resume, &nargs);
  status = precover(L, status);            // handle recoverable errors
  // ...
}
```

### unroll (ldo.c:874-885)

```c
static void unroll (lua_State *L, void *ud) {
  CallInfo *ci;
  UNUSED(ud);
  while ((ci = L->ci) != &L->base_ci) {
    if (!isLua(ci))
      finishCcall(L, ci);                  // complete C function
    else {
      luaV_finishOp(L);                    // finish interrupted instruction
      luaV_execute(L, ci);                // run to next C boundary
    }
  }
}
```

Unwinds the **entire** call stack from the point of yield/error. For Lua frames, it finishes any partially-executed instruction first.

### luaV_finishOp (lvm.c:855-912)

Completes **partially executed** VM instructions after yield:

```c
void luaV_finishOp (lua_State *L) {
  CallInfo *ci = L->ci;
  StkId base = ci->func.p + 1;
  Instruction inst = *(ci->u.l.savedpc - 1);
  OpCode op = GET_OPCODE(inst);
  switch (op) {
    case OP_MMBIN ... OP_MMBINK:   // binary metamethods
      setobjs2s(L, base + GETARG_A(...), --L->top.p);
      break;
    case OP_UNM, OP_BNOT, OP_LEN:
    case OP_GETTABUP, OP_GETTABLE, OP_GETI, OP_GETFIELD, OP_SELF:
      setobjs2s(L, base + GETARG_A(inst), --L->top.p);
      break;
    case OP_LT, OP_LE, OP_EQ ...:
      // restore stack top, possibly skip the JMP
      break;
    case OP_CONCAT:
      // complete concatenation
      break;
    case OP_CLOSE, OP_RETURN:
      // repeat instruction for remaining closes/returns
      break;
    default: // OP_TFORCALL, OP_CALL, OP_TAILCALL, OP_SETTABUP/SETTABLE/SETI/FIELD
      break;  // continuation handles these
  }
}
```

Only these opcodes can yield mid-execution (lvm.c:852-854). Any reimplementation that adds yieldable opcodes **must** update `luaV_finishOp`.

---

## 9. The CRITICAL ldo.c ↔ lvm.c Interaction

This is the most important section for reimplementors. The VM and call mechanism are deeply entangled.

### luaV_execute — The Main Interpreter Loop (lvm.c:1198-1826)

```c
void luaV_execute (lua_State *L, CallInfo *ci) {
  LClosure *cl;
  TValue *k;
  StkId base;
  const Instruction *pc;
  int trap;
 startfunc:
  trap = L->hookmask;
 returning:
  cl = ci_func(ci);
  k = cl->p->k;
  pc = ci->u.l.savedpc;
  if (l_unlikely(trap))
    trap = luaG_tracecall(L);
  base = ci->func.p + 1;
  for (;;) {
    Instruction i;
    vmfetch();
    // ... execute instruction ...
  ret:                                   // lvm.c:1823
    if (ci->callstatus & CIST_FRESH)
      return;                            // ← return to ccall
    else {
      ci = ci->previous;
      goto returning;                    // ← continue in same luaV_execute
    }
  }
}
```

**Key insight**: `luaV_execute` is **NOT** re-entrant per call. A single invocation handles the **entire call stack** via gotos. `CIST_FRESH` distinguishes "returning from the top-level luaV_execute" (return to `ccall`) from "returning from a nested Lua call" (goto `returning`).

### OP_CALL (lvm.c:1721-1716)

```c
vmcase(OP_CALL) {
  StkId ra = RA(i);
  int b = GETARG_B(i);          // number of arguments
  int nresults = GETARG_C(i) - 1;
  if (b != 0)
    L->top.p = ra + b;         // top signals argument count
  savepc(ci);                   // for error reporting
  if ((newci = luaD_precall(L, ra, nresults)) == NULL)
    updatetrap(ci);            // C function — poscall already done
  else {
    ci = newci;
    goto startfunc;            // Lua function — continue in same luaV_execute
  }
}
```

### OP_TAILCALL (lvm.c:1735-1742)

```c
vmcase(OP_TAILCALL) {
  StkId ra = RA(i);
  int b = GETARG_B(i);
  int n, nparams1 = GETARG_C(i);
  int delta = (nparams1) ? ci->u.l.nextraargs + nparams1 : 0;
  if (TESTARG_k(i)) {
    luaF_closeupval(L, base);
    lua_assert(L->tbclist.p < base);
  }
  if ((n = luaD_pretailcall(L, ci, ra, b, delta)) < 0)
    goto startfunc;            // Lua tail call — reuse frame!
  else {
    ci->func.p -= delta;
    luaD_poscall(L, ci, n);
    updatetrap(ci);
    goto ret;                  // C tail call — return from this luaV_execute
  }
}
```

### OP_RETURN (lvm.c:1761-1764)

```c
vmcase(OP_RETURN) {
  StkId ra = RA(i);
  int n = GETARG_B(i) - 1;
  if (n < 0) n = cast_int(L->top.p - ra);
  savepc(ci);
  if (TESTARG_k(i)) {
    ci->u2.nres = n;
    if (L->top.p < ci->top.p) L->top.p = ci->top.p;
    luaF_close(L, base, CLOSEKTOP, 1);
    updatetrap(ci);
    updatestack(ci);
  }
  if (GETARG_C(i)) ci->func.p -= ci->u.l.nextraargs + GETARG_C(i);
  L->top.p = ra + n;
  luaD_poscall(L, ci, n);
  updatetrap(ci);
  goto ret;
}
```

### The Dance Summary

```
ccall
  └─ luaD_precall
       └─ (for Lua) returns ci pointer
  └─ luaV_execute(L, ci)          ← ONE luaV_execute per ccall
       │
       ├─ startfunc: read ci, set base
       ├─ for(;;): execute opcodes
       │    ├─ OP_CALL → luaD_precall → goto startfunc (Lua) or break (C)
       │    ├─ OP_TAILCALL → luaD_pretailcall → goto startfunc (Lua) or goto ret (C)
       │    └─ OP_RETURN → luaD_poscall → goto ret
       │
       └─ ret: if CIST_FRESH → return to ccall
               else → ci=previous, goto returning
```

---

## 10. Complete Call Sequence Pseudocode

### From C API (lua_call equivalent)

```
luaD_call(L, func, nResults):
    ccall(L, func, nResults, 1)
        L->nCcalls += 1
        ci = luaD_precall(L, func, nResults)
        if ci != NULL:                        // Lua function
            ci->callstatus |= CIST_FRESH
            luaV_execute(L, ci)               // never returns normally
        else:                                 // C function
            // precallC already called the C function
            // and already called luaD_poscall
        L->nCcalls -= 1
```

### Inside luaV_execute — Full Loop

```
luaV_execute(L, ci):
    startfunc:
    cl      = ci_func(ci)                      // closure
    k       = cl->p->k                         // constants
    pc      = ci->u.l.savedpc                  // instruction pointer
    base    = ci->func.p + 1                   // local variable base
    trap    = L->hookmask

    returning:
    for ;;:
        i = fetch_instruction(pc++)
        execute(i)

        // What OP_CALL does:
        OP_CALL:
            ra      = register(i)
            nargs   = GETARG_B(i)
            nres    = GETARG_C(i) - 1
            if nargs != 0: L->top = ra + nargs
            savepc(ci)
            newci = luaD_precall(L, ra, nres)
            if newci == NULL:
                // C function ran to completion inside precallC
                // luaD_poscall already called inside precallC
                updatetrap(ci)
            else:
                // Lua function: ci = newci, continue in same luaV_execute
                ci = newci
                goto startfunc

        // What OP_RETURN does:
        OP_RETURN:
            ra      = register(i)
            n       = GETARG_B(i) - 1          // number of results
            if n < 0: n = L->top - ra         // "up to top"
            savepc(ci)
            if k-flag:                         // "K" bit
                luaF_close(L, base, CLOSEKTOP, 1)
            L->top = ra + n
            luaD_poscall(L, ci, n)
            goto ret

        // The return label:
        ret:
            if ci->callstatus & CIST_FRESH:
                return                         // back to ccall
            else:
                ci = ci->previous              // back to caller
                goto returning

luaD_poscall(L, ci, nres):
    fwanted = ci->callstatus & (CIST_TBC | CIST_NRESULTS)
    if L->hookmask && !(fwanted & CIST_TBC):
        rethook(L, ci, nres)
    moveresults(L, ci->func.p, nres, fwanted)  // move to caller's frame
    L->ci = ci->previous                        // unwind CI
```

---

## 11. If I Were Building This in Go

### The Stack: []lua.Value with Integer Offsets

Lua stores `TValue*` pointers directly. In Go, use a slice with integer indices:

```go
type luaState struct {
    stack    []lua.Value        // the Lua value stack
    top      int                // index of first free slot (not pointer!)
    stackLast int               // stack_last = len(stack) (one-past-end)
}

// savestack: convert index to offset
func savestack(L *luaState, idx int) int {
    return idx
}

// restorestack: convert offset back to index
func restorestack(L *luaState, offset int) int {
    return offset
}

// But for actual reallocation, we need true offsets from base pointer:
type luaState struct {
    stack []lua.Value
    top   int           // true index
    base  int           // saved offset for reallocation
}
```

The key insight: in Go, `L->top.p` is an `int` index into `[]lua.Value`, not a pointer. When reallocating the slice, compute `savedIdx = int(unsafe.Pointer(oldPtr))` and restore as `newIdx = int(uintptr(newBase) + savedOffset)`. Or use a simpler model where `top` is always a true array index and every pointer-based access goes through `slice[idx]`.

### CallInfo in Go

```go
type CallInfo struct {
    Func  int           // stack index of function slot (was StkIdRel)
    Top   int           // stack index of top for this call
    Prev  *CallInfo     // caller's CallInfo
    Next  *CallInfo     // callee's CallInfo (allocated lazily)

    // Lua-specific
    SavedPC   uintptr   // index into bytecode instructions
    Trap      bool      // tracing active
    NExtraArgs int      // vararg extra args

    // C-specific
    K       luaKFunction
    OldErrFunc int
    Ctx     luaKContext

    // Ephemeral
    FuncIdx int        // for CIST_YPCALL
    NYield  int
    NRes    int

    CallStatus uint32
}
```

### Handling setjmp/longjmp (Go Has No Equivalent)

Go's `panic/recover` cannot cross goroutine boundaries cleanly. Options:

**Option A — Panic/Recover with locked goroutines (recommended)**:

```go
func RawRunProtected(L *luaState, f func(*luaState, unsafe.Pointer), ud unsafe.Pointer) TStatus {
    lj := &longjmp{prev: L.errorJmp, status: LUA_OK}
    L.errorJmp = lj
    defer func() {
        L.errorJmp = lj.prev
        if r := recover(); r != nil {
            if lj2, ok := r.(*longjmp); ok {
                lj.status = lj2.status
            } else {
                lj.status = LUA_ERRRUN
            }
        }
    }()
    runtime.LockOSThread()    // ensure goroutine stays on same OS thread
    f(L, ud)
    return LUA_OK
}
```

**Option B — Explicit error propagation (state machine)**:

```go
// Instead of longjmp, every function returns an error
func executeVM(L *luaState, ci *CallInfo) TStatus {
    for {
        inst := fetch(ci.SavedPC)
        err := dispatch(L, ci, inst)
        if err != nil {
            return err   // explicit error return instead of longjmp
        }
        if ci.CallStatus&CIST_FRESH != 0 {
            return LUA_OK
        }
        ci = ci.Prev
    }
}
```

Option B is cleaner for Go but requires propagating errors through **every** VM instruction — a significant refactoring. Option A is closer to the C semantics.

### Coroutine Yield as State Machine

In Go, `lua_yield` does **NOT** block a goroutine. It suspends the **state machine**:

```go
type coroutineState int
const (
    CoroutineRunning coroutineState = iota
    CoroutineSuspended
    CoroutineDead
)

type luaState struct {
    status coroutineState
    // ... other fields ...
}

func (L *luaState) Yield(nresults int) int {
    if !L.yieldable() {
        panic(runtimeError("cannot yield"))
    }
    L.status = CoroutineSuspended
    L.ci.NYield = nresults
    runtime.Goexit()   // exit the goroutine
    // never reached — Resume() picks up from here
    return 0
}

func Resume(L *luaState, args []lua.Value) (int, error) {
    if L.status == CoroutineDead {
        return 0, errors.New("cannot resume dead coroutine")
    }
    L.status = CoroutineRunning
    // push args to stack, restore savedPC, continue luaV_execute
    // use runtime.LockOSThread + channel for synchronization
}
```

The critical realization: a Lua coroutine is NOT a Go goroutine. It is a **state machine** on a locked OS thread. `lua_yield` suspends the state machine, and `lua_resume` resumes it. The Go goroutine model maps naturally here, but `runtime.Goexit()` (not blocking) is the yield mechanism.

### luaV_execute in Go

```go
func executeVM(L *luaState, ci *CallInfo) TStatus {
    for {
        // Load the function's closure
        base := ci.Func + 1
        cl := L.stack[ci.Func].(*LClosure)

        // Main dispatch loop
        for ci.SavedPC < uintptr(len(cl.Proto.Code)) {
            inst := cl.Proto.Code[ci.SavedPC]
            ci.SavedPC++
            
            op := inst.Opcode()
            switch op {
            case OP_CALL:
                nresults := int(inst.C() - 1)
                newCI := luaD_precall(L, inst.A(), nresults)
                if newCI != nil {
                    // Lua function: push new frame
                    ci = newCI
                    goto newFrame
                }
                // C function: already completed via precallC
            case OP_RETURN:
                n := int(inst.B() - 1)
                if n < 0 { n = L.top - inst.A() }
                if inst.KBit() {
                    luaF_close(L, base, CLOSEKTOP)
                }
                L.top = inst.A() + n
                luaD_poscall(L, ci, n)
                if ci.CallStatus&CIST_FRESH != 0 {
                    return LUA_OK
                }
                ci = ci.Prev
            // ... all other opcodes ...
            }
        }
        // Fell off the bytecode — implicit return nil
        luaD_poscall(L, ci, 0)
        if ci.CallStatus&CIST_FRESH != 0 {
            return LUA_OK
        }
        ci = ci.Prev
    newFrame:
        // recalculate base from new ci, continue loop
    }
}
```

### CIST_FRESH Equivalent

In Go, we track whether the current `luaV_execute` is the **top-level** call or a continuation:

```go
const CIST_FRESH uint32 = 1 << 16  // or any unused bit

// When starting luaV_execute from ccall:
ci.CallStatus |= CIST_FRESH
executeVM(L, ci)

// At OP_RETURN:
if ci.CallStatus&CIST_FRESH != 0 {
    // This is the top-level call — we're done
    return
}
// Otherwise, ci = ci.Prev, continue in same executeVM
```

---

## 12. Edge Cases

### Tail Calls

A tail call is a call in **disguise as a return**. The callee **reuses** the caller's CallInfo frame:

```c
// ldo.c:677-712 — luaD_pretailcall for Lua functions
luaD_pretailcall(L, CallInfo *ci, StkId func, int narg1, int delta):
    // delta = ci->func - virtual func (for vararg functions)
    ci->func.p -= delta           // restore func position
    for i = 0; i < narg1; i++:    // move function+args DOWN
        setobjs2s(L, ci->func.p + i, func + i)
    func = ci->func.p             // new func position
    for ; narg1 <= nfixparams; narg1++:
        setnilvalue(s2v(func + narg1))
    ci->top.p = func + 1 + fsize  // set frame top
    ci->callstatus |= CIST_TAIL   // mark as tail call
    L->top.p = func + narg1       // set stack top
    return -1                     // signal: Lua function, continue
```

**In Go**: Tail call = reset the `ci` fields in-place. Do NOT allocate a new `CallInfo`. Do NOT push onto a call stack. Just update `ci.Func`, `ci.Top`, `ci.SavedPC`, and `ci.CallStatus`. The return from a tail-called function goes back to the **original caller** of the tail-call sequence.

### Vararg Functions

Vararg functions receive a variable number of arguments. The frame management is trickier:

- `nextraargs`: Count of extra vararg args beyond the fixed parameters. Set during `OP_VARARGPREP`.
- `delta` in tail calls: The `func` slot may be shifted to accommodate vararg args. `ci->func.p -= delta` restores it.
- `OP_VARARGPREP`: The **first instruction** of any vararg function. Calls `luaT_adjustvarargs` which sets up the vararg environment.

```c
// ldo.c:1760 — OP_RETURN adjusts func for vararg
if (GETARG_C(i))    // vararg function (C bit set)
    ci->func.p -= ci->u.l.nextraargs + GETARG_C(i);
```

### C Function Calling Lua Calling C

```
C code calls luaD_call
  └─ ccall (inc=1)
       └─ luaD_precall → luaV_execute
            └─ OP_CALL → luaD_precall → precallC
                 └─ C function runs
                      └─ calls luaD_call (nestable!)
                           └─ ...
```

The `nCcalls` counter (lstate.h:302) tracks both C stack depth (low 16 bits) and non-yieldable call depth (high 16 bits). A coroutine can only yield if `yieldable(L)` is true — i.e., no non-yieldable calls are active.

### Stack Overflow Detection

Three distinct overflow checks:

| Check | Limit | Action |
|---|---|---|
| C stack depth | `LUAI_MAXCCALLS = 200` | `luaE_checkcstack` raises `LUA_ERRERR` |
| Lua stack size | `MAXSTACK = 1,000,000` | `luaD_reallocstack` fails |
| Lua stack overflow | After reaching MAXSTACK | `luaG_runerror("stack overflow")` |

`luaE_checkcstack` (lstate.c) is called when `getCcalls(L) >= LUAI_MAXCCALLS`. It raises an error if the C stack is truly exhausted (platform-specific check).

---

## 13. Bug Pattern Guide

### Wrong Number of Results

**Symptom**: "stack underflow", wrong values returned, silent data corruption.

**Root cause**: `moveresults` truncates or pads based on `fwanted`, but `fwanted` was set by `luaD_precall` from the **caller's** expected `nresults`. If the callee returns a different number than expected, and the caller requested a specific count, the mismatch corrupts the result area.

**Specific failure modes**:
- `luaD_precall` encodes `nresults + 1` in `callstatus & CIST_NRESULTS`. If this is wrong, `moveresults` will write to wrong stack positions.
- `LUA_MULTRET` (`-1`) is stored as `MAXRESULTS + 1` (`-1 + 1 = 0`, which is valid). But `MAXRESULTS = 250`, so a function cannot legally return more than 250 values.
- `CIST_TBC` in `fwanted` forces `genmoveresults` even for single-result cases — failing to account for this causes wrong truncation.

**Fix**: Trace the `nresults` value from the opcode (`GETARG_C(i) - 1`) through `luaD_precall` (stored as `+1`) through `moveresults` (decoded as `-1`). Verify the match at every boundary.

---

### Stack Pointer Corruption

**Symptom**: Random `nil` values, crashes at unexpected addresses, `L->top` in wrong position.

**Root cause**: After stack reallocation (`luaD_reallocstack`), all stack pointers must be updated. The critical list:
1. `L->top.p`
2. `L->tbclist.p`
3. All `ci->func.p` and `ci->top.p` for every `CallInfo` in the chain
4. All open upvalue `v.p` pointers

**Specific failure modes**:
- Missing a pointer in `relstack`/`correctstack` → dangling pointer → crash after next realloc.
- `relstack`/`correctstack` must traverse the **entire** CI chain (not just `L->ci`).
- The `ci->func.p` for all Lua frames must be restored, or `base = ci->func.p + 1` will point to garbage.

**Fix**: Every time you add a new field that holds a `StkId` or `TValue*`, add it to both `relstack` and `correctstack`.

---

### CI Chain Bugs

**Symptom**: "attempt to call nil value", crashes in `luaV_execute` with wrong `base`, wrong line numbers in error messages.

**Root cause**: The `CallInfo` linked list (`ci->previous` → `ci->next`) must match the actual call stack.

**Specific failure modes**:
- `L->ci = ci->previous` in `luaD_poscall` happens **after** `moveresults`. Reversing the order corrupts the stack before results are moved.
- `base_ci` (lstate.h:299) is the root. It always has `previous = NULL`. If you lose track of `base_ci`, you cannot unwind to it.
- `ci->next` being `NULL` when `luaD_precall` is called causes `luaE_extendCI` to allocate — if this allocation fails, the call fails.
- `luaE_shrinkCI` frees `ci->next` nodes that were allocated but are no longer needed. If you free a node that's still in use, you lose the callee's `CallInfo`.

**Fix**: Draw the CI chain before and after every call/return. Verify `L->ci` points to the current frame. Verify every frame has a valid `func` pointing to a slot on the stack.

---

### CIST_FRESH / CIST_TAIL Confusion

**Symptom**: `luaV_execute` returns prematurely (before the call stack is unwound), or it never returns (looping forever).

The `ret` label is the decision point:

```c
ret:
    if (ci->callstatus & CIST_FRESH)
        return;                     // This is a fresh luaV_execute — return
    else {
        ci = ci->previous;
        goto returning;             // Nested Lua call — continue in same frame
    }
```

**Specific failure modes**:
- `CIST_FRESH` is set by `ccall` (ldo.c:773) **only** when calling a Lua function from C. It should **never** be set when `luaV_execute` calls itself via `goto startfunc`.
- `CIST_TAIL` is set by `luaD_pretailcall` to mark a frame as tail-called. It does **not** prevent `goto returning` — it's just for the return hook (`luaD_hookcall` checks it).
- If `CIST_FRESH` is accidentally set during a nested call (e.g., after `OP_CALL`), `luaV_execute` will return to `ccall` while the call stack still has frames → corrupted state.
- If `CIST_FRESH` is **not** set when it should be (e.g., `ccall` didn't set it), `luaV_execute` will never return to `ccall` → infinite loop or stack overflow.

**Fix**: `CIST_FRESH` is set in exactly **one place**: `ccall` (ldo.c:773), right before `luaV_execute`. Verify it is NOT set anywhere else.

---

### luaV_finishOp Omissions

**Symptom**: After `lua_yield`, resuming produces wrong values (e.g., wrong operands for metamethods, wrong stack state).

When a coroutine yields, the current instruction may be **partially executed**. `luaV_finishOp` must undo or complete the partial effect.

**Known yieldable opcodes** (lvm.c:852-854):
- `OP_TFORCALL`, `OP_CALL`, `OP_TAILCALL`, `OP_SETTABUP`, `OP_SETTABLE`, `OP_SETI`, `OP_SETFIELD` — continuation handles these.
- `OP_MMBIN`, `OP_MMBINI`, `OP_MMBINK` — must pop the temporary from `L->top`.
- `OP_UNM`, `OP_BNOT`, `OP_LEN`, `OP_GETTABUP`, `OP_GETTABLE`, `OP_GETI`, `OP_GETFIELD`, `OP_SELF` — must pop the temp.
- `OP_LT`, `OP_LE`, `OP_LTI`, `OP_LEI`, `OP_GTI`, `OP_GEI`, `OP_EQ`, `OP_EQI`, `OP_EQK` — must restore stack top and possibly skip JMP.
- `OP_CONCAT` — must finalize the concatenation.
- `OP_CLOSE`, `OP_RETURN` — must repeat to close remaining variables.

**Fix**: When adding a new yieldable opcode, implement its finish handler in `luaV_finishOp`. Test by yielding mid-execution of the new opcode and resuming.

---

### Protected Call / Panic Recovery

**Symptom**: Errors during `lua_pcall` don't restore `L->ci`, `L->top`, or `L->allowhook`. Panic function called with wrong state.

`luaD_pcall` saves three pieces of state before the protected call and restores them on error:

```c
// ldo.c:1093-1099
CallInfo *old_ci = L->ci;
lu_byte old_allowhooks = L->allowhook;
ptrdiff_t old_errfunc = L->errfunc;
// ... protected call ...
if (status != LUA_OK) {
    L->ci = old_ci;           // restore call chain
    L->allowhook = old_allowhooks;
    status = luaD_closeprotected(L, old_top, status);
    luaD_seterrorobj(L, status, restorestack(L, old_top));
    luaD_shrinkstack(L);
}
```

**Specific failure modes**:
- Forgetting to restore `L->ci` → error message shows wrong call stack.
- Forgetting to restore `L->allowhook` → hooks may be permanently disabled.
- `luaD_seterrorobj` places the error object at `old_top`. If `old_top` was not correctly computed (using `savestack`), and the stack was reallocated during the protected call, the error object goes to a garbage address.
- `luaD_closeprotected` loops until no more errors. If `__close` methods keep erroring, this is an infinite loop risk — but the loop handles it by repeatedly calling `luaF_close` until success.

---

### CIST_YPCALL / Coroutine Recovery

**Symptom**: Error in a coroutine with `__close` methods causes crash or corrupted state on resume.

When a coroutine yields inside a `lua_pcallk` with active `__close` variables:

1. `CIST_YPCALL` marks the protected call frame.
2. `CIST_RECST` (bits 12-14) stores the 3-bit error status.
3. `precover` finds `CIST_YPCALL` frames and calls `unroll` in protected mode.
4. `finishpcallk` uses `CIST_RECST` to restore the original error status.

```c
// ldo.c:963-971
static TStatus precover (lua_State *L, TStatus status) {
  CallInfo *ci;
  while (errorstatus(status) && (ci = findpcall(L)) != NULL) {
    L->ci = ci;
    setcistrecst(ci, status);              // store error status
    status = luaD_rawrunprotected(L, unroll, NULL);
  }
  return status;
}
```

**Fix**: In a Go reimplementation, `CIST_YPCALL` and `CIST_RECST` must be tracked as part of the `CallInfo`. `precover` scans the CI chain for `CIST_YPCALL`, stores the error in the frame's `u2` union (as `funcidx` or `nres`), and runs the unwind in a protected call.

---

### __call Metamethod Chain

**Symptom**: "attempt to call nil value" when calling an object with a `__call` metamethod, or " '__call' chain too long".

```c
// ldo.c:531-544
static unsigned tryfuncTM (lua_State *L, StkId func, unsigned status) {
  const TValue *tm = luaT_gettmbyobj(L, s2v(func), TM_CALL);
  if (l_unlikely(ttisnil(tm)))
    luaG_callerror(L, s2v(func));          // "attempt to call nil value"
  for (p = L->top.p; p > func; p--)
    setobjs2s(L, p, p-1);                  // shift stack to make room
  L->top.p++;
  setobj2s(L, func, tm);                   // metamethod is new function
  if ((status & MAX_CCMT) == MAX_CCMT)
    luaG_runerror(L, "'__call' chain too long");
  return status + (1u << CIST_CCMT);       // increment metamethod count
}
```

**Bug patterns**:
- `__call` resolution shifts the stack **upward**. If the stack must grow to accommodate this shift, all `StkId` values become invalid unless `relstack`/`correctstack` is called.
- The metamethod count (`CIST_CCMT`) limits chains to **15**. This prevents infinite `__call` loops but requires tracking.
- `tryfuncTM` is called from both `luaD_precall` (with retry) and `luaD_pretailcall` (with retry). Both must correctly handle the stack shift.

**Fix**: Every time you modify the stack layout (push, pop, shift), verify `relstack`/`correctstack` is called if reallocation is possible.

---

## Quick Reference

### Key Constants

| Constant | Value | Location |
|---|---|---|
| `LUAI_MAXCCALLS` | 200 | ldo.h:63 |
| `LUAI_MAXSTACK` | 1,000,000 | ldo.c:192 |
| `BASIC_STACK_SIZE` | `2*LUA_MINSTACK` | lstate.h:156 |
| `EXTRA_STACK` | 5 | lstate.h:142 |
| `MAXSTACK` | min(LUAI_MAXSTACK, MAXSTACK_BYSIZET) | ldo.c:206 |
| `ERRORSTACKSIZE` | MAXSTACK + 200 | ldo.h:211 |
| `MAXRESULTS` | 250 | lstate.h:216 |
| `nyci` | `0x10000 \| 1` | lstate.h:117 |

### Key Macros

| Macro | Definition | Purpose |
|---|---|---|
| `savestack(L, pt)` | `(cast_charp(pt) - cast_charp(L->stack.p))` | Pointer → offset |
| `restorestack(L, n)` | `cast(StkId, cast_charp(L->stack.p) + (n))` | Offset → pointer |
| `isLua(ci)` | `!((ci)->callstatus & CIST_C)` | Is Lua function? |
| `isLuacode(ci)` | `!((ci)->callstatus & (CIST_C \| CIST_HOOKED))` | Is Lua bytecode? |
| `yieldable(L)` | `((L)->nCcalls & 0xffff0000) == 0` | Can coroutine yield? |
| `getCcalls(L)` | `((L)->nCcalls & 0xffff)` | C stack depth |
| `get_nresults(cs)` | `cast_int((cs) & CIST_NRESULTS) - 1` | Decode nresults |
| `ci_func(ci)` | `clLvalue(s2v((ci)->func.p))` | Get closure from CI |

### Source File Index

| File | Lines | Key Content |
|---|---|---|
| `lua-master/ldo.c` | 1172 | All runtime execution: call, return, yield, error |
| `lua-master/ldo.h` | 99 | Stack macros, function declarations |
| `lua-master/lstate.h` | 187-312 | `CallInfo` struct, `lua_State` struct |
| `lua-master/lvm.c` | 1198-1826 | `luaV_execute` main loop, `luaV_finishOp` |
| `lua-master/ltm.c` | 272-338 | Vararg adjustment functions |
| `lua-master/ldebug.h` | 18 | `ci_func` macro |