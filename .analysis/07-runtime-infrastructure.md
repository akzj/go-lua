# 07 — Runtime Infrastructure Deep Analysis

> **Scope**: `lstate.c/h` (state management), `lfunc.c/h` (closures/upvalues), `ltable.c/h` (tables), `lstring.c/h` (string interning), `ltm.c/h` (metamethods)
> **Source**: Lua 5.5.1 reference C implementation at `lua-master/`
> **Prereqs**: [03-object-type-system](03-object-type-system.md), [04-call-return-error](04-call-return-error.md), [05-vm-execution-loop](05-vm-execution-loop.md)

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [lstate — State Management](#2-lstate--state-management)
3. [lfunc — Closures and Upvalues](#3-lfunc--closures-and-upvalues)
4. [ltable — Tables](#4-ltable--tables)
5. [lstring — String Interning](#5-lstring--string-interning)
6. [ltm — Tag Methods (Metamethods)](#6-ltm--tag-methods-metamethods)
7. [Module Interconnections](#7-module-interconnections)
8. [If I Were Building This in Go](#8-if-i-were-building-this-in-go)
9. [Edge Cases & Traps](#9-edge-cases--traps)
10. [Bug Pattern Guide](#10-bug-pattern-guide)

---

## 1. Architecture Overview

These five modules form the runtime substrate that the VM (lvm.c) operates on:

```
┌──────────────────────────────────────────────────────────────────┐
│                        VM (lvm.c)                                │
│  Executes opcodes, calls metamethods, manipulates stack          │
├──────────┬──────────┬──────────┬──────────┬─────────────────────┤
│ lstate   │ lfunc    │ ltable   │ lstring  │ ltm                 │
│ State &  │ Closures │ Tables   │ String   │ Metamethod          │
│ threads  │ Upvalues │ Hash+Arr │ Interning│ Dispatch            │
├──────────┴──────────┴──────────┴──────────┴─────────────────────┤
│                    lobject.h (TValue, GCObject)                  │
│                    lgc.c (Garbage Collection)                    │
└──────────────────────────────────────────────────────────────────┘
```

**Dependency flow**:
- `lstate` creates everything: it calls `luaS_init`, `luaT_init`, creates the registry table
- `ltable` uses `lstring` for key hashing (short strings compared by pointer identity)
- `ltm` uses `ltable` to look up metamethods and `lstring` for metamethod names
- `lfunc` uses `ltm` for `__close` metamethods, links upvalues to the stack in `lstate`
- The VM calls into all five modules during execution

---

## 2. lstate — State Management

### 2.1 Core Data Structures

#### `global_State` (lstate.h:327-372)

The shared state for all threads (coroutines) in a Lua instance:

```c
typedef struct global_State {
  lua_Alloc frealloc;          // memory allocator function
  void *ud;                    // auxiliary data for allocator
  l_mem GCtotalbytes;          // bytes allocated + debt
  l_mem GCdebt;                // bytes counted but not yet allocated
  l_mem GCmarked;              // objects marked in current GC cycle
  stringtable strt;            // GLOBAL string intern table
  TValue l_registry;           // the registry table
  TValue nilvalue;             // shared nil (also signals state completeness)
  unsigned int seed;           // randomized hash seed
  // ... GC fields (allgc, finobj, gray lists, generational markers) ...
  struct lua_State *twups;     // list of threads with open upvalues
  lua_CFunction panic;         // unprotected error handler
  TString *memerrmsg;          // pre-allocated "not enough memory"
  TString *tmname[TM_N];      // cached metamethod name strings (25 entries)
  struct Table *mt[LUA_NUMTYPES]; // metatables for basic types
  TString *strcache[STRCACHE_N][STRCACHE_M]; // API string cache (53×2)
  LX mainth;                   // main thread (embedded, not heap-allocated)
} global_State;
```

**Key insight**: `global_State` is allocated as a single block. The main thread (`mainth`) is embedded directly inside it — not separately allocated. This means `lua_newstate` does exactly ONE allocation for the entire initial state.

#### `lua_State` (lstate.h:285-312)

Per-thread (coroutine) state:

```c
struct lua_State {
  CommonHeader;                // GC header (all Lua objects have this)
  lu_byte allowhook;           // hook enable flag
  TStatus status;              // thread status (OK, YIELD, error codes)
  StkIdRel top;                // first free stack slot
  struct global_State *l_G;    // pointer to shared global state
  CallInfo *ci;                // current call info
  StkIdRel stack_last;         // end of usable stack
  StkIdRel stack;              // stack base
  UpVal *openupval;            // linked list of open upvalues (sorted by level)
  StkIdRel tbclist;            // to-be-closed variable list
  GCObject *gclist;            // for GC traversal
  struct lua_State *twups;     // thread list link (threads with open upvalues)
  struct lua_longjmp *errorJmp; // current error recovery point (longjmp)
  CallInfo base_ci;            // embedded base CallInfo (for C host level)
  volatile lua_Hook hook;      // debug hook function
  ptrdiff_t errfunc;           // error handler stack index
  l_uint32 nCcalls;            // C call depth (lower 16) + non-yieldable count (upper 16)
  int oldpc;                   // last traced PC
  int nci;                     // number of CallInfo nodes in the list
  // ... hook fields, transfer info ...
};
```

#### `CallInfo` (lstate.h:187-209)

```c
struct CallInfo {
  StkIdRel func;               // function index in stack
  StkIdRel top;                // top for this function
  struct CallInfo *previous, *next; // doubly-linked list
  union {
    struct { /* Lua functions */
      const Instruction *savedpc;
      volatile l_signalT trap;
      int nextraargs;          // extra args in vararg functions
    } l;
    struct { /* C functions */
      lua_KFunction k;         // continuation for yields
      ptrdiff_t old_errfunc;
      lua_KContext ctx;
    } c;
  } u;
  union {
    int funcidx;               // C protected call
    int nyield;                // values yielded
    int nres;                  // values returned
  } u2;
  l_uint32 callstatus;        // bit flags (CIST_C, CIST_FRESH, etc.)
};
```

**CallInfo pool**: CallInfo nodes form a doubly-linked list. Nodes beyond the current `ci` are "free" — cached for reuse. This avoids allocation on every function call.

#### `nCcalls` encoding (lstate.h:94-117)

```c
#define yieldable(L)     (((L)->nCcalls & 0xffff0000) == 0)
#define getCcalls(L)     ((L)->nCcalls & 0xffff)
#define incnny(L)        ((L)->nCcalls += 0x10000)  // increment non-yieldable
#define decnny(L)        ((L)->nCcalls -= 0x10000)
#define nyci             (0x10000 | 1)  // increment both at once
```

The lower 16 bits count recursive C calls; the upper 16 bits count non-yieldable frames. Both are packed into one `l_uint32` so they can be saved/restored atomically.

### 2.2 Initialization Sequence: `lua_newstate`

`lua_newstate` (lstate.c:341-393) performs a carefully ordered bootstrap:

```
lua_newstate(allocator, ud, seed)
│
├─ 1. Allocate global_State (single block, sizeof(global_State))
│     The main thread L = &g->mainth.l (embedded in global_State)
│
├─ 2. Set L->tt = LUA_VTHREAD, mark white, preinit_thread(L, g)
│     preinit_thread zeros everything: stack=NULL, ci=NULL, nCcalls=0,
│     openupval=NULL, twups=L (self = no upvalues), etc.
│
├─ 3. g->allgc = obj2gco(L)  — main thread is first GC object
│     incnny(L)                — main thread is always non-yieldable
│
├─ 4. Initialize global_State fields:
│     g->seed = seed           — hash randomization
│     g->gcstp = GCSTPGC      — DISABLE GC during bootstrap
│     g->strt = {NULL, 0, 0}  — empty string table
│     g->nilvalue = integer 0  — NOT nil yet (signals incomplete state)
│     g->mt[i] = NULL          — no type metatables
│     GC params set to defaults
│
├─ 5. luaD_rawrunprotected(L, f_luaopen, NULL)
│     Protected call — if any allocation fails, close_state cleans up
│
│     f_luaopen(L) sequence:
│     ├─ stack_init(L, L)      — allocate stack (BASIC_STACK_SIZE + EXTRA_STACK)
│     │   └─ All slots set to nil, base_ci initialized (CIST_C)
│     ├─ init_registry(L, g)   — create registry table
│     │   ├─ registry[1] = false
│     │   ├─ registry[LUA_RIDX_MAINTHREAD] = L
│     │   └─ registry[LUA_RIDX_GLOBALS] = new table (global env)
│     ├─ luaS_init(L)          — init string table (128 buckets)
│     │   └─ Pre-create "not enough memory" string, fix it in GC
│     ├─ luaT_init(L)          — create all 25 metamethod name strings
│     │   └─ Fix them in GC (never collected)
│     ├─ luaX_init(L)          — create reserved word strings
│     ├─ g->gcstp = 0          — ENABLE GC
│     └─ g->nilvalue = nil     — signals state is COMPLETE
│
└─ 6. Return L (or NULL if f_luaopen failed)
```

**Critical ordering**: The `nilvalue` trick (lstate.h:382):
```c
#define completestate(g)  ttisnil(&g->nilvalue)
```
During bootstrap, `g->nilvalue` is set to integer 0 (not nil). Only after `f_luaopen` succeeds is it set to nil. This lets `close_state` know whether to do a full cleanup or a partial one.

### 2.3 Thread (Coroutine) Creation: `lua_newthread`

`lua_newthread` (lstate.c:278-302):

```c
LUA_API lua_State *lua_newthread (lua_State *L) {
  // 1. Trigger GC check
  // 2. Allocate LX (lua_State + extra space) via luaC_newobjdt
  // 3. Anchor new thread on parent's stack (prevents GC collection)
  // 4. preinit_thread(L1, g) — zero all fields
  // 5. Copy hook settings from parent
  // 6. Copy extra space from main thread
  // 7. stack_init(L1, L) — allocate stack using parent's allocator
  return L1;
}
```

**Key detail**: New threads inherit hook settings from the creating thread, and extra space from the *main* thread (not the creating thread).

### 2.4 CallInfo Pool Management

**Extend** — `luaE_extendCI` (lstate.c:71-87):
```c
CallInfo *luaE_extendCI (lua_State *L, int err) {
  ci = luaM_reallocvector(L, NULL, 0, 1, CallInfo);  // allocate one node
  // Insert after L->ci in the doubly-linked list
  ci->next = L->ci->next;
  ci->previous = L->ci;
  L->ci->next = ci;
  L->nci++;
  return ci;
}
```

**Shrink** — `luaE_shrinkCI` (lstate.c:109-126): Frees every OTHER free CallInfo node (keeps half). Called by GC to reclaim unused nodes without destroying the pool entirely.

**Free** — `freeCI` (lstate.c:93-102): Frees ALL CallInfo nodes after the current one. Called during `freestack`.

### 2.5 C Stack Overflow Protection

`luaE_checkcstack` (lstate.c:136-141):
- At exactly `LUAI_MAXCCALLS`: raises "C stack overflow"
- At `LUAI_MAXCCALLS * 1.1`: calls `luaD_errerr` (error while handling error)
- The gap between the two thresholds allows error handling code to run

### 2.6 Thread Cleanup

`luaE_freethread` (lstate.c:305-312):
```c
void luaE_freethread (lua_State *L, lua_State *L1) {
  luaF_closeupval(L1, L1->stack.p);  // close ALL open upvalues
  lua_assert(L1->openupval == NULL);
  freestack(L1);                      // free stack + all CallInfo nodes
  luaM_free(L, fromstate(L1));        // free the LX block
}
```

`close_state` (lstate.c:260-275) for the main thread:
1. Reset CI, close all upvalues via `luaD_closeprotected`
2. Empty the stack, run finalizers via `luaC_freeallobjects`
3. Free string table hash array
4. Free stack
5. Assert total bytes == sizeof(global_State)
6. Free the global_State block itself

---

## 3. lfunc — Closures and Upvalues

### 3.1 Core Data Structures

#### `UpVal` (lobject.h:679-694)

```c
typedef struct UpVal {
  CommonHeader;
  union {
    TValue *p;           // points to stack slot (open) or to u.value (closed)
    ptrdiff_t offset;    // used during stack reallocation
  } v;
  union {
    struct {             // when OPEN:
      struct UpVal *next;      // next in open list
      struct UpVal **previous; // pointer to previous node's next pointer
    } open;
    TValue value;        // when CLOSED: the captured value lives here
  } u;
} UpVal;
```

**The open/closed duality**: An UpVal's `v.p` pointer determines its state:
- **Open**: `v.p` points to a stack slot → the variable is still alive on the stack
- **Closed**: `v.p` points to `&u.value` → the value has been copied into the UpVal itself

```c
#define upisopen(up)  ((up)->v.p != &(up)->u.value)  // lfunc.h:32
#define uplevel(up)   check_exp(upisopen(up), cast(StkId, (up)->v.p))  // lfunc.h:35
```

#### `LClosure` / `CClosure` (lobject.h:699-713)

```c
typedef struct CClosure {
  ClosureHeader;           // CommonHeader + nupvalues + gclist
  lua_CFunction f;
  TValue upvalue[1];       // upvalues stored INLINE as TValues
} CClosure;

typedef struct LClosure {
  ClosureHeader;
  struct Proto *p;         // function prototype
  UpVal *upvals[1];        // upvalues stored as POINTERS to UpVal objects
} LClosure;
```

**Critical difference**: CClosure stores upvalues inline as TValues (no sharing). LClosure stores pointers to UpVal objects (enabling sharing between closures).

### 3.2 Closure Creation

**C Closure** — `luaF_newCclosure` (lfunc.c:27-32):
```c
CClosure *luaF_newCclosure (lua_State *L, int nupvals) {
  GCObject *o = luaC_newobj(L, LUA_VCCL, sizeCclosure(nupvals));
  CClosure *c = gco2ccl(o);
  c->nupvalues = cast_byte(nupvals);
  return c;
}
```
Size: `offsetof(CClosure, upvalue) + sizeof(TValue) * nupvals`

**Lua Closure** — `luaF_newLclosure` (lfunc.c:35-42):
```c
LClosure *luaF_newLclosure (lua_State *L, int nupvals) {
  GCObject *o = luaC_newobj(L, LUA_VLCL, sizeLclosure(nupvals));
  LClosure *c = gco2lcl(o);
  c->p = NULL;
  c->nupvalues = cast_byte(nupvals);
  while (nupvals--) c->upvals[nupvals] = NULL;  // NULL all upval pointers
  return c;
}
```
Size: `offsetof(LClosure, upvals) + sizeof(UpVal*) * nupvals`

**Init closed upvalues** — `luaF_initupvals` (lfunc.c:48-58): Creates new UpVal objects in closed state (v.p = &u.value, value = nil). Used for top-level closures.

### 3.3 The Open Upvalue List

Each `lua_State` maintains `L->openupval`, a singly-linked list of open upvalues **sorted by stack level in descending order** (highest stack position first).

#### `luaF_findupval` (lfunc.c:87-99) — Find or Create

```c
UpVal *luaF_findupval (lua_State *L, StkId level) {
  UpVal **pp = &L->openupval;
  UpVal *p;
  // Walk the list (sorted descending by stack level)
  while ((p = *pp) != NULL && uplevel(p) >= level) {
    if (uplevel(p) == level)  // found existing upvalue at this level
      return p;               // REUSE it (sharing!)
    pp = &p->u.open.next;
  }
  // Not found: create new upvalue, insert after 'pp'
  return newupval(L, level, pp);
}
```

**Sharing mechanism**: When two closures capture the same local variable, `luaF_findupval` returns the same UpVal object. This is how closures share state.

#### `newupval` (lfunc.c:65-80) — Create and Link

```c
static UpVal *newupval (lua_State *L, StkId level, UpVal **prev) {
  UpVal *uv = ...;  // allocate
  uv->v.p = s2v(level);           // point to stack slot
  uv->u.open.next = *prev;        // link into list
  uv->u.open.previous = prev;     // doubly-linked for O(1) removal
  if (*prev) (*prev)->u.open.previous = &uv->u.open.next;
  *prev = uv;
  
  // Add thread to global "threads with upvalues" list
  if (!isintwups(L)) {
    L->twups = G(L)->twups;
    G(L)->twups = L;
  }
  return uv;
}
```

**Thread tracking**: `G(L)->twups` is a linked list of all threads that have open upvalues. This is used by GC to find and traverse open upvalues.

### 3.4 Closing Upvalues: The Open→Closed Transition

#### `luaF_closeupval` (lfunc.c:197-210) — Close upvalues down to a stack level

```c
void luaF_closeupval (lua_State *L, StkId level) {
  UpVal *uv;
  while ((uv = L->openupval) != NULL && uplevel(uv) >= level) {
    TValue *slot = &uv->u.value;           // destination: UpVal's own storage
    luaF_unlinkupval(uv);                  // remove from open list
    setobj(L, slot, uv->v.p);             // COPY value from stack to UpVal
    uv->v.p = slot;                        // redirect pointer to own storage
    if (!iswhite(uv)) {                    // GC color maintenance
      nw2black(uv);                        // closed upvalues cannot be gray
      luaC_barrier(L, uv, slot);
    }
  }
}
```

**The transition in detail**:
1. Before: `uv->v.p` → stack slot (open)
2. Copy the value from the stack slot into `uv->u.value`
3. After: `uv->v.p` → `&uv->u.value` (closed)
4. Any code reading `*uv->v.p` now reads the captured copy

**GC subtlety**: Closed upvalues are set to black (not gray). Open upvalues are kept gray intentionally to avoid write barriers on stack modifications.

#### `luaF_unlinkupval` (lfunc.c:186-191) — O(1) removal from doubly-linked list

```c
void luaF_unlinkupval (UpVal *uv) {
  lua_assert(upisopen(uv));
  *uv->u.open.previous = uv->u.open.next;
  if (uv->u.open.next)
    uv->u.open.next->u.open.previous = uv->u.open.previous;
}
```

### 3.5 The Full Close: `luaF_close`

`luaF_close` (lfunc.c:230-240) handles both upvalues AND to-be-closed (tbc) variables:

```c
StkId luaF_close (lua_State *L, StkId level, TStatus status, int yy) {
  ptrdiff_t levelrel = savestack(L, level);
  luaF_closeupval(L, level);              // 1. Close upvalues first
  while (L->tbclist.p >= level) {         // 2. Then close tbc variables
    StkId tbc = L->tbclist.p;
    poptbclist(L);                         //    Remove from tbc list
    prepcallclosemth(L, tbc, status, yy); //    Call __close metamethod
    level = restorestack(L, levelrel);     //    Stack may have moved!
  }
  return level;
}
```

**Ordering matters**: Upvalues are closed BEFORE tbc variables. This ensures that `__close` methods see the captured values (not stack garbage).

### 3.6 To-Be-Closed Variables

`luaF_newtbcupval` (lfunc.c:172-183):
```c
void luaF_newtbcupval (lua_State *L, StkId level) {
  if (l_isfalse(s2v(level)))
    return;                    // false doesn't need closing
  checkclosemth(L, level);     // must have __close metamethod
  // Handle large gaps with dummy nodes (delta encoding)
  while (cast_uint(level - L->tbclist.p) > MAXDELTA) {
    L->tbclist.p += MAXDELTA;
    L->tbclist.p->tbclist.delta = 0;  // dummy node
  }
  level->tbclist.delta = cast(unsigned short, level - L->tbclist.p);
  L->tbclist.p = level;
}
```

The tbc list uses **delta encoding**: each entry stores its distance from the previous entry as a `unsigned short`. If the gap exceeds `USHRT_MAX` (65535), dummy nodes with delta=0 are inserted.

### 3.7 Proto (Function Prototype)

`luaF_newproto` (lfunc.c:243-267): Creates a new Proto with all arrays NULL and sizes 0. Protos are created by the parser/compiler, not at runtime.

`luaF_freeproto` (lfunc.c:285-296): Frees all sub-arrays (code, lineinfo, constants, etc.). If `PF_FIXED` flag is set, code/lineinfo are in fixed memory and not freed.


---

## 4. ltable — Tables

### 4.1 Core Data Structures

#### `Table` (lobject.h:776-785)

```c
typedef struct Table {
  CommonHeader;
  lu_byte flags;           // metamethod cache bits + BITDUMMY flag
  lu_byte lsizenode;       // log2 of hash part size
  unsigned int asize;      // number of slots in array part
  Value *array;            // pointer INTO the array (between values and tags)
  Node *node;              // hash part node array
  struct Table *metatable; // metatable (or NULL)
  GCObject *gclist;        // for GC traversal
} Table;
```

#### `Node` (lobject.h:751-762)

```c
typedef union Node {
  struct NodeKey {
    TValuefields;          // fields for VALUE (tt_ and value_)
    lu_byte key_tt;        // key type tag
    int next;              // OFFSET to next node in chain (not pointer!)
    Value key_val;         // key value
  } u;
  TValue i_val;            // direct access to value as TValue
} Node;
```

**Key design**: `next` is a signed integer OFFSET, not a pointer. `n + gnext(n)` gives the next node. This saves memory on 64-bit systems (4 bytes vs 8 bytes for a pointer). A `next` of 0 means end of chain.

#### Array Part Layout (ltable.h:95-124)

The array part uses an **inverted layout** — values grow downward, tags grow upward, with an unsigned hint in between:

```
Memory layout:
  ...  |  Value[1]  |  Value[0]  | unsigned hint | Tag[0] | Tag[1] | ...
                                  ^ t->array points here

  Values:  getArrVal(t, k) = t->array - 1 - k    (negative indexing)
  Tags:    getArrTag(t, k) = (lu_byte*)(t->array) + sizeof(unsigned) + k
```

**Why inverted?** This avoids padding waste. Values (8 bytes) and tags (1 byte) would normally require 7 bytes of padding per entry. By separating them, the array uses `sizeof(Value) + 1` bytes per entry plus one `sizeof(unsigned)` for the length hint.

The `unsigned` between the two arrays is a **length hint** for `#t` (the `luaH_getn` operation), stored via `lenhint(t)`.

### 4.2 Hash Algorithm

#### Main Position (ltable.c:188-223)

`mainpositionTV` determines where a key should be placed in the hash part:

```c
static Node *mainpositionTV (const Table *t, const TValue *key) {
  switch (ttypetag(key)) {
    case LUA_VNUMINT:    return hashint(t, ivalue(key));
    case LUA_VNUMFLT:    return hashmod(t, l_hashfloat(fltvalue(key)));
    case LUA_VSHRSTR:    return hashstr(t, tsvalue(key));   // hashpow2
    case LUA_VLNGSTR:    return hashpow2(t, luaS_hashlongstr(ts));
    case LUA_VFALSE:     return hashboolean(t, 0);
    case LUA_VTRUE:      return hashboolean(t, 1);
    case LUA_VLIGHTUSERDATA: return hashpointer(t, pvalue(key));
    case LUA_VLCF:       return hashpointer(t, fvalue(key));
    default:             return hashpointer(t, gcvalue(key)); // identity hash
  }
}
```

Two hash strategies:
- `hashpow2(t, n)` = `gnode(t, lmod(n, sizenode(t)))` — for good hashes (strings, booleans)
- `hashmod(t, n)` = `gnode(t, n % ((sizenode(t)-1)|1))` — for potentially biased hashes (pointers, floats)

The `|(1)` ensures the modulus is odd, avoiding patterns when hash values have many factors of 2.

#### Integer Hashing (ltable.c:145-151)

```c
static Node *hashint (const Table *t, lua_Integer i) {
  lua_Unsigned ui = l_castS2U(i);
  if (ui <= cast_uint(INT_MAX))
    return gnode(t, cast_int(ui) % cast_int((sizenode(t)-1) | 1));
  else
    return hashmod(t, ui);
}
```

Small non-negative integers use a faster signed modulo; larger values use unsigned modulo.

#### Float Hashing (ltable.c:168-181)

```c
static unsigned l_hashfloat (lua_Number n) {
  int i;
  lua_Integer ni;
  n = frexp(n, &i) * -cast_num(INT_MIN);
  if (!lua_numbertointeger(n, &ni))  // inf/NaN
    return 0;
  else {
    unsigned u = cast_uint(i) + cast_uint(ni);
    return (u <= cast_uint(INT_MAX) ? u : ~u);
  }
}
```

### 4.3 The Main Invariant (Brent's Variation)

From ltable.c:19-24:
> *A main invariant of these tables is that, if an element is not in its main position (i.e. the 'original' position that its hash gives to it), then the colliding element is in its own main position.*

This means: if you find a key NOT in its main position, the key that IS in that position belongs there. This is enforced by `insertkey`.

### 4.4 Key Operations

#### `luaH_new` (ltable.c:798-807) — Create empty table

```c
Table *luaH_new (lua_State *L) {
  Table *t = ...;
  t->metatable = NULL;
  t->flags = maskflags;      // all metamethod-absent bits set (no metamethods)
  t->array = NULL;
  t->asize = 0;
  setnodevector(L, t, 0);   // use dummynode for hash part
  return t;
}
```

Empty tables use a shared `dummynode` (ltable.c:130-133) — a single static Node with an empty value and a DEADKEY key. The `BITDUMMY` flag in `t->flags` (bit 6) signals this.

#### `luaH_get` (ltable.c:1019-1041) — Main search function

```c
lu_byte luaH_get (Table *t, const TValue *key, TValue *res) {
  switch (ttypetag(key)) {
    case LUA_VSHRSTR:
      slot = luaH_Hgetshortstr(t, tsvalue(key));  // pointer comparison
      break;
    case LUA_VNUMINT:
      return luaH_getint(t, ivalue(key), res);    // check array first
      break;
    case LUA_VNIL:
      slot = &absentkey;                           // nil key = absent
      break;
    case LUA_VNUMFLT: {
      lua_Integer k;
      if (luaV_flttointeger(fltvalue(key), &k, F2Ieq))
        return luaH_getint(t, k, res);             // float with int value → array
      // else fall through to generic
    }
    default:
      slot = getgeneric(t, key, 0);
  }
  return finishnodeget(slot, res);
}
```

**Optimization path for integers**: `luaH_getint` (ltable.c:958-968) checks the array part first:
```c
lu_byte luaH_getint (Table *t, lua_Integer key, TValue *res) {
  unsigned k = ikeyinarray(t, key);     // is key in [1, asize]?
  if (k > 0) {                          // yes → array lookup
    lu_byte tag = *getArrTag(t, k - 1); // O(1) array access
    if (!tagisempty(tag))
      farr2val(t, k - 1, tag, res);
    return tag;
  }
  else
    return finishnodeget(getintfromhash(t, key), res);  // hash lookup
}
```

**Optimization path for short strings**: `luaH_Hgetshortstr` (ltable.c:974-987) uses pointer equality:
```c
const TValue *luaH_Hgetshortstr (Table *t, TString *key) {
  Node *n = hashstr(t, key);
  for (;;) {
    if (keyisshrstr(n) && eqshrstr(keystrval(n), key))  // pointer ==
      return gval(n);
    int nx = gnext(n);
    if (nx == 0) return &absentkey;
    n += nx;
  }
}
```

Since short strings are interned, `eqshrstr` is just a pointer comparison (`a == b`).

#### The pset/finishset Pattern

Lua 5.5 uses a two-phase set pattern to handle metamethods:

1. **`luaH_pset*`** (pre-set): Try to set the value. Returns:
   - `HOK` (0): success, value was set
   - `HNOTFOUND` (1): key is truly absent
   - `HFIRSTNODE + index`: slot exists but value is nil (needs metamethod check)
   - `~array_index`: empty array slot (needs metamethod check)

2. **`luaH_finishset`** (ltable.c:1153-1188): Completes the set when pset couldn't:
   - `HNOTFOUND`: calls `luaH_newkey` (may trigger rehash)
   - Positive hres: set value in existing hash node
   - Negative hres: set value in array slot

**Why two phases?** The VM needs to check for `__newindex` metamethod between finding the slot and writing the value. The pset phase is called in the fast path (no metamethod), and finishset is called only when the fast path fails.

#### `insertkey` (ltable.c:858-894) — Brent's variation

```c
static int insertkey (Table *t, const TValue *key, TValue *value) {
  Node *mp = mainpositionTV(t, key);
  if (!isempty(gval(mp)) || isdummy(t)) {  // main position taken?
    Node *f = getfreepos(t);               // find free slot
    if (f == NULL) return 0;               // no free slot → need rehash
    
    Node *othern = mainpositionfromnode(t, mp);
    if (othern != mp) {
      // Colliding node is NOT in its main position → MOVE it
      // Find previous in chain, relink to point to free slot
      // Copy colliding node to free slot
      // Put new key in main position (mp)
    } else {
      // Colliding node IS in its main position
      // Put new key in free slot, chain it after mp
    }
  }
  setnodekey(mp, key);
  setobj2t(..., gval(mp), value);
  return 1;
}
```

**Brent's optimization**: When a collision occurs and the existing node is NOT in its main position, move it to a free slot and put the new key in the main position. This keeps frequently-accessed keys in their main positions, reducing chain length.

#### `getfreepos` (ltable.c:828-846) — Find free hash slot

```c
static Node *getfreepos (Table *t) {
  if (haslastfree(t)) {           // hash size >= 2^LIMFORLAST (8)?
    while (getlastfree(t) > t->node) {
      Node *free = --getlastfree(t);
      if (keyisnil(free)) return free;
    }
  } else {                         // small hash: linear search
    unsigned i = sizenode(t);
    while (i--) {
      if (keyisnil(gnode(t, i))) return gnode(t, i);
    }
  }
  return NULL;
}
```

For hash parts ≥ 8 nodes, a `lastfree` pointer is stored just before the node array (in a `Limbox` union). It scans backwards from the last free position. For smaller hash parts, a full linear search is done.

### 4.5 Rehash Strategy

#### `rehash` (ltable.c:761-791) — Triggered when `insertkey` fails

```c
static void rehash (lua_State *L, Table *t, const TValue *ek) {
  Counters ct;
  // 1. Count all keys (array + hash) into logarithmic bins
  numusehash(t, &ct);
  if (ct.na > 0) numusearray(t, &ct);
  
  // 2. Compute optimal array size
  unsigned asize = computesizes(&ct);
  
  // 3. Hash part gets all remaining keys
  unsigned nsize = ct.total - ct.na;
  if (ct.deleted) nsize += nsize >> 2;  // 25% extra for deletion patterns
  
  // 4. Resize
  luaH_resize(L, t, asize, nsize);
}
```

#### `computesizes` (ltable.c:446-467) — Optimal array size

The algorithm uses logarithmic bins. For each power of 2 (`twotoi`), it checks:
- Does the number of array-eligible keys justify this array size?
- The condition `arrayXhash(twotoi, a)` checks: `twotoi <= a * 3`
  (a hash node costs ~3× more memory than an array entry)

The optimal size is the largest power of 2 where array entries are "worth it" compared to putting them in the hash.

#### `luaH_resize` (ltable.c:715-747) — The actual resize

```
luaH_resize(L, t, newasize, nhsize)
│
├─ 1. Create new hash part in temporary 'newt'
│     setnodevector(L, &newt, nhsize)
│
├─ 2. If array is SHRINKING:
│     ├─ exchangehashpart(t, &newt)  // temporarily use new hash
│     ├─ reinsertOldSlice(t, oldasize, newasize)  // move vanishing array→hash
│     └─ exchangehashpart(t, &newt)  // restore old hash
│
├─ 3. Allocate new array part
│     newarray = resizearray(L, t, oldasize, newasize)
│     If allocation fails → free new hash, raise error
│
├─ 4. Install new hash, new array
│     exchangehashpart(t, &newt)  // t gets new hash, newt gets old
│     t->array = newarray
│     t->asize = newasize
│     *lenhint(t) = newasize / 2  // initial length hint
│
├─ 5. Clear new array slots
│     clearNewSlice(t, oldasize, newasize)
│
├─ 6. Reinsert old hash elements
│     reinserthash(L, &newt, t)  // from old hash into new table
│
└─ 7. Free old hash
      freehash(L, &newt)
```

**Error safety**: If the array allocation fails, the table is still in its original state (old hash was preserved). The new hash is freed and the error is raised.

### 4.6 Table Iteration: `luaH_next`

`luaH_next` (ltable.c:361-381):

```c
int luaH_next (lua_State *L, Table *t, StkId key) {
  unsigned asize = t->asize;
  unsigned i = findindex(L, t, s2v(key), asize);  // find current key's index
  
  // Try array part first
  for (; i < asize; i++) {
    lu_byte tag = *getArrTag(t, i);
    if (!tagisempty(tag)) {
      setivalue(s2v(key), cast_int(i) + 1);  // key = i+1 (1-based)
      farr2val(t, i, tag, s2v(key + 1));     // value
      return 1;
    }
  }
  
  // Then hash part
  for (i -= asize; i < sizenode(t); i++) {
    if (!isempty(gval(gnode(t, i)))) {
      getnodekey(L, s2v(key), gnode(t, i));   // key
      setobj2s(L, key + 1, gval(gnode(t, i))); // value
      return 1;
    }
  }
  return 0;  // no more elements
}
```

**Iteration order**: Array part first (indices 1..asize), then hash part (node 0..sizenode-1). Empty slots and dead keys are skipped.

`findindex` (ltable.c:343-358) locates the current key: if it's an integer in the array range, return its array index; otherwise search the hash part with `getgeneric(t, key, 1)` (deadok=1 to handle keys that became dead during iteration).

### 4.7 Length Operator: `luaH_getn`

`luaH_getn` (ltable.c:1301-1343) implements the `#` operator:

1. If array part exists, start from the **length hint** (`*lenhint(t)`)
2. Check vicinity of hint (±4 positions) for a boundary
3. If not found nearby, binary search in the array part
4. If the last array element is non-empty, check hash part for `asize+1`
5. If `asize+1` is also present, do `hash_search` (randomized doubling + binary search)

The **length hint** is stored in the `unsigned` between the value and tag arrays, updated on each `#t` call. This makes repeated `#t` calls O(1) when the boundary hasn't moved.

`hash_search` (ltable.c:1239-1268) uses randomized probing to prevent adversarial inputs from causing O(n) behavior.


---

## 5. lstring — String Interning

### 5.1 Core Data Structures

#### `TString` (lobject.h:405-419)

```c
typedef struct TString {
  CommonHeader;
  lu_byte extra;         // reserved words (short) or "has hash" flag (long)
  ls_byte shrlen;        // length for short strings; negative for long strings
  unsigned int hash;     // hash value
  union {
    size_t lnglen;       // length for long strings
    struct TString *hnext; // next in hash bucket (short strings only)
  } u;
  char *contents;        // pointer to string data (long strings)
  lua_Alloc falloc;      // deallocation function (external strings)
  void *ud;              // user data (external strings)
} TString;
```

**Short vs Long distinction**:
- `shrlen >= 0` → short string (length ≤ LUAI_MAXSHORTLEN = 40)
- `shrlen < 0` → long string (length stored in `u.lnglen`)

```c
#define strisshr(ts)    ((ts)->shrlen >= 0)
#define getshrstr(ts)   cast_charp(&(ts)->contents)  // data follows header
#define getlngstr(ts)   (ts)->contents               // data at pointer
```

**Short string data layout**: For short strings, the actual character data is stored immediately after the `contents` field in the TString struct itself (via `rawgetshrstr`). For long strings, `contents` is a pointer to separately allocated data (or external data).

#### `stringtable` (lstate.h:167-171)

```c
typedef struct stringtable {
  TString **hash;    // array of bucket heads (linked lists)
  int nuse;          // number of interned strings
  int size;          // number of buckets (always power of 2)
} stringtable;
```

### 5.2 String Kinds (Lua 5.5 novelty)

Lua 5.5 introduces three kinds of long strings:

```c
#define LSTRREG  0   // regular: data follows the header
#define LSTRFIX  (-1) // fixed external: data at external pointer, never freed
#define LSTRMEM  (-2) // external with dealloc: data at external pointer, freed via falloc
```

The `shrlen` field doubles as the kind indicator for long strings (negative values).

### 5.3 Hash Computation

`luaS_hash` (lstring.c:53-58):
```c
static unsigned luaS_hash (const char *str, size_t l, unsigned seed) {
  unsigned int h = seed ^ cast_uint(l);
  for (; l > 0; l--)
    h ^= ((h<<5) + (h>>2) + cast_byte(str[l - 1]));
  return h;
}
```

**Properties**:
- Seeded with `g->seed` (randomized per state) — prevents hash-flooding attacks
- Processes ALL bytes (no skip step like Lua 5.3 had for long strings)
- The shift pattern `(h<<5) + (h>>2)` provides good avalanche behavior

**Long string lazy hashing** — `luaS_hashlongstr` (lstring.c:61-69):
```c
unsigned luaS_hashlongstr (TString *ts) {
  if (ts->extra == 0) {           // not yet hashed?
    ts->hash = luaS_hash(getlngstr(ts), ts->u.lnglen, ts->hash);
    ts->extra = 1;                // mark as hashed
  }
  return ts->hash;
}
```

Long strings start with `hash = g->seed` and `extra = 0`. The full hash is computed lazily on first use as a table key. This avoids hashing large strings that are never used as keys.

### 5.4 String Interning: `internshrstr`

`internshrstr` (lstring.c:214-243) — the core of short string management:

```c
static TString *internshrstr (lua_State *L, const char *str, size_t l) {
  global_State *g = G(L);
  stringtable *tb = &g->strt;
  unsigned h = luaS_hash(str, l, g->seed);
  TString **list = &tb->hash[lmod(h, tb->size)];
  
  // 1. Search the bucket for an existing match
  for (ts = *list; ts != NULL; ts = ts->u.hnext) {
    if (l == cast_uint(ts->shrlen) &&
        memcmp(str, getshrstr(ts), l) == 0) {
      // FOUND! Resurrect if dead (but not yet collected)
      if (isdead(g, ts)) changewhite(ts);
      return ts;
    }
  }
  
  // 2. Not found: grow table if needed
  if (tb->nuse >= tb->size)
    growstrtab(L, tb);  // doubles the table size
  
  // 3. Create new string object
  ts = createstrobj(L, sizestrshr(l), LUA_VSHRSTR, h);
  ts->shrlen = cast(ls_byte, l);
  memcpy(getshrstr(ts), str, l);
  getshrstr(ts)[l] = '\0';
  
  // 4. Insert at head of bucket
  ts->u.hnext = *list;
  *list = ts;
  tb->nuse++;
  return ts;
}
```

**Resurrection**: If a string is found but marked dead (pending GC collection), it's resurrected by flipping its white bit. This prevents creating a duplicate.

**Growth policy**: When `nuse >= size`, the table doubles. `growstrtab` (lstring.c:200-208) also tries a full GC if `nuse == INT_MAX`.

### 5.5 The API String Cache

`luaS_new` (lstring.c:269-283) — for zero-terminated C strings:

```c
TString *luaS_new (lua_State *L, const char *str) {
  unsigned i = point2uint(str) % STRCACHE_N;  // hash by POINTER address
  TString **p = G(L)->strcache[i];
  
  // Check cache (2 entries per bucket)
  for (j = 0; j < STRCACHE_M; j++) {
    if (strcmp(str, getstr(p[j])) == 0)
      return p[j];  // cache hit!
  }
  
  // Cache miss: shift entries, insert new at front
  for (j = STRCACHE_M - 1; j > 0; j--)
    p[j] = p[j - 1];
  p[0] = luaS_newlstr(L, str, strlen(str));
  return p[0];
}
```

**Design**: 53 sets × 2 entries. Hashed by the C pointer address (not the string content). This is optimized for the common case where the same C string literal is passed repeatedly (same pointer = same hash bucket).

### 5.6 String Table Resize

`luaS_resize` (lstring.c:95-113):
```c
void luaS_resize (lua_State *L, int nsize) {
  stringtable *tb = &G(L)->strt;
  int osize = tb->size;
  if (nsize < osize)              // shrinking?
    tablerehash(tb->hash, osize, nsize);  // rehash BEFORE realloc
  
  TString **newvect = luaM_reallocvector(L, tb->hash, osize, nsize, TString*);
  if (newvect == NULL) {          // allocation failed?
    if (nsize < osize)
      tablerehash(tb->hash, nsize, osize);  // UNDO the rehash
    return;                        // keep old table
  }
  
  tb->hash = newvect;
  tb->size = nsize;
  if (nsize > osize)
    tablerehash(newvect, osize, nsize);  // rehash AFTER realloc
}
```

**Error safety**: When shrinking, strings are rehashed into the smaller range BEFORE reallocation. If realloc fails, the rehash is undone. When growing, new buckets are cleared and strings are rehashed AFTER successful reallocation.

### 5.7 String Comparison

`luaS_eqstr` (lstring.c:44-50):
```c
int luaS_eqstr (TString *a, TString *b) {
  size_t len1, len2;
  const char *s1 = getlstr(a, len1);
  const char *s2 = getlstr(b, len2);
  return ((len1 == len2) && (memcmp(s1, s2, len1) == 0));
}
```

For short strings, this is only called when comparing with external/long strings. Between two short strings, pointer equality (`a == b`) suffices due to interning:
```c
#define eqshrstr(a,b)  check_exp((a)->tt == LUA_VSHRSTR, (a) == (b))
```

### 5.8 Initialization

`luaS_init` (lstring.c:133-146):
1. Allocate 128 buckets (`MINSTRTABSIZE`)
2. Pre-create `"not enough memory"` string and fix it in GC (never collected)
3. Fill the string cache with the memerrmsg string (entries cannot be NULL)

### 5.9 External Strings (Lua 5.5 novelty)

`luaS_newextlstr` (lstring.c:318-338): Creates a long string that points to externally-managed data. Two variants:
- **Fixed** (`LSTRFIX`): data pointer is never freed (e.g., string literals in C code)
- **Managed** (`LSTRMEM`): data is freed via a custom allocator when the string is collected

`luaS_normstr` (lstring.c:344-352): If an external string is short enough (≤ 40 bytes), it gets internalized (converted to a regular short string). This is needed before using it as a table key.

---

## 6. ltm — Tag Methods (Metamethods)

### 6.1 The TMS Enumeration

`ltm.h:18-45` defines 25 metamethods:

```c
typedef enum {
  TM_INDEX,      // __index        (0)
  TM_NEWINDEX,   // __newindex     (1)
  TM_GC,         // __gc           (2)
  TM_MODE,       // __mode         (3)
  TM_LEN,        // __len          (4)
  TM_EQ,         // __eq           (5)  ← last "fast access" method
  TM_ADD,        // __add          (6)
  TM_SUB, TM_MUL, TM_MOD, TM_POW, TM_DIV, TM_IDIV,
  TM_BAND, TM_BOR, TM_BXOR, TM_SHL, TM_SHR,
  TM_UNM,        // __unm          (18)
  TM_BNOT,       // __bnot         (19)
  TM_LT, TM_LE,  // __lt, __le    (20, 21)
  TM_CONCAT,     // __concat       (22)
  TM_CALL,       // __call         (23)
  TM_CLOSE,      // __close        (24)
  TM_N           // count = 25
} TMS;
```

### 6.2 The fasttm Optimization

The first 6 metamethods (TM_INDEX through TM_EQ) have **fast-access** caching in the table's `flags` byte:

```c
#define maskflags  cast_byte(~(~0u << (TM_EQ + 1)))  // 0x3F = bits 0-5

// Check if metamethod is ABSENT (bit set = absent)
#define checknoTM(mt, e)  ((mt) == NULL || (mt)->flags & (1u<<(e)))

// Fast path: check cache first, then do full lookup only if needed
#define gfasttm(g, mt, e) \
  (checknoTM(mt, e) ? NULL : luaT_gettm(mt, e, (g)->tmname[e]))

#define fasttm(l, mt, e)  gfasttm(G(l), mt, e)
```

**How it works**:
1. When a table is created, `flags = maskflags` (all 6 bits set = "no metamethods")
2. When `luaT_gettm` is called and the metamethod is NOT found, it sets the corresponding bit: `events->flags |= (1u << event)` (lstring.c:64)
3. When `luaT_gettm` finds the metamethod, it returns it (bit stays clear)
4. When a table's metatable changes, `invalidateTMcache(t)` clears all 6 bits: `t->flags &= ~maskflags`

**Bit 6** (`BITDUMMY`) is reserved for the dummy node flag and is NOT part of the metamethod cache.

```
flags byte layout:
  Bit 0: TM_INDEX absent?
  Bit 1: TM_NEWINDEX absent?
  Bit 2: TM_GC absent?
  Bit 3: TM_MODE absent?
  Bit 4: TM_LEN absent?
  Bit 5: TM_EQ absent?
  Bit 6: BITDUMMY (hash part uses dummy node)
  Bit 7: unused
```

### 6.3 Metamethod Lookup

#### `luaT_gettm` (ltm.c:60-68) — Lookup in metatable with caching

```c
const TValue *luaT_gettm (Table *events, TMS event, TString *ename) {
  const TValue *tm = luaH_Hgetshortstr(events, ename);  // raw table lookup
  lua_assert(event <= TM_EQ);  // only called for fast-access methods
  if (notm(tm)) {
    events->flags |= cast_byte(1u << event);  // CACHE the absence
    return NULL;
  }
  return tm;
}
```

#### `luaT_gettmbyobj` (ltm.c:71-84) — Get metamethod for any value

```c
const TValue *luaT_gettmbyobj (lua_State *L, const TValue *o, TMS event) {
  Table *mt;
  switch (ttype(o)) {
    case LUA_TTABLE:    mt = hvalue(o)->metatable; break;
    case LUA_TUSERDATA: mt = uvalue(o)->metatable; break;
    default:            mt = G(L)->mt[ttype(o)];   break;  // type metatable
  }
  return (mt ? luaH_Hgetshortstr(mt, G(L)->tmname[event]) : &G(L)->nilvalue);
}
```

**Three sources of metatables**:
1. Tables have their own `metatable` field
2. Userdata have their own `metatable` field
3. All other types use the global type metatable `G(L)->mt[type]`

### 6.4 Metamethod Dispatch Functions

#### `luaT_callTM` (ltm.c:103-116) — Call with no return value

```c
void luaT_callTM (lua_State *L, const TValue *f, const TValue *p1,
                  const TValue *p2, const TValue *p3) {
  StkId func = L->top.p;
  setobj2s(L, func, f);       // push metamethod
  setobj2s(L, func + 1, p1);  // 1st arg
  setobj2s(L, func + 2, p2);  // 2nd arg
  setobj2s(L, func + 3, p3);  // 3rd arg
  L->top.p = func + 4;
  // Yield-aware dispatch:
  if (isLuacode(L->ci))
    luaD_call(L, func, 0);      // yieldable
  else
    luaD_callnoyield(L, func, 0); // not yieldable
}
```

Used for `__newindex` (3 args: table, key, value) and `__close` (2 args + error).

#### `luaT_callTMres` (ltm.c:119-135) — Call with one return value

```c
lu_byte luaT_callTMres (lua_State *L, const TValue *f, const TValue *p1,
                        const TValue *p2, StkId res) {
  ptrdiff_t result = savestack(L, res);  // save position (stack may move)
  // Push function + 2 args, call with 1 result
  // ...
  res = restorestack(L, result);         // restore after possible stack move
  setobjs2s(L, res, --L->top.p);        // move result to destination
  return ttypetag(s2v(res));             // return tag of result
}
```

Used for arithmetic/comparison metamethods. Returns the type tag of the result.

#### Binary Metamethod Dispatch (ltm.c:138-147)

```c
static int callbinTM (lua_State *L, const TValue *p1, const TValue *p2,
                      StkId res, TMS event) {
  const TValue *tm = luaT_gettmbyobj(L, p1, event);  // try first operand
  if (notm(tm))
    tm = luaT_gettmbyobj(L, p2, event);               // try second operand
  if (notm(tm))
    return -1;                                          // not found
  return luaT_callTMres(L, tm, p1, p2, res);
}
```

**Operand priority**: First operand's metamethod is tried first. If absent, second operand's metamethod is tried. This matches Lua semantics.

#### Associative Binary TM (ltm.c:180-186)

```c
void luaT_trybinassocTM (lua_State *L, const TValue *p1, const TValue *p2,
                          int flip, StkId res, TMS event) {
  if (flip)
    luaT_trybinTM(L, p2, p1, res, event);  // swap operands
  else
    luaT_trybinTM(L, p1, p2, res, event);
}
```

The `flip` flag handles cases where the VM has swapped operands for optimization (e.g., `OP_ADDI` with immediate operand).

#### Order Metamethods (ltm.c:200-207)

```c
int luaT_callorderTM (lua_State *L, const TValue *p1, const TValue *p2,
                      TMS event) {
  int tag = callbinTM(L, p1, p2, L->top.p, event);
  if (tag >= 0)
    return !tagisfalse(tag);   // convert result to boolean
  luaG_ordererror(L, p1, p2);  // no metamethod → error
}
```

### 6.5 Vararg Handling (in ltm.c)

Lua 5.5 has two vararg strategies:

1. **Hidden args** (`PF_VAHID`): Extra arguments are stored before the function on the stack. `luaT_adjustvarargs` calls `buildhiddenargs` to rearrange the stack.

2. **Vararg table** (`PF_VATAB`): Extra arguments are packed into a table with an `n` field. `createvarargtab` (ltm.c:231-246) creates this table.

`luaT_getvarargs` (ltm.c:338-363) retrieves vararg values from either source.

### 6.6 Initialization

`luaT_init` (ltm.c:38-53):
```c
void luaT_init (lua_State *L) {
  static const char *const luaT_eventname[] = {
    "__index", "__newindex", "__gc", "__mode", "__len", "__eq",
    "__add", "__sub", "__mul", "__mod", "__pow", "__div", "__idiv",
    "__band", "__bor", "__bxor", "__shl", "__shr",
    "__unm", "__bnot", "__lt", "__le",
    "__concat", "__call", "__close"
  };
  for (i = 0; i < TM_N; i++) {
    G(L)->tmname[i] = luaS_new(L, luaT_eventname[i]);
    luaC_fix(L, obj2gco(G(L)->tmname[i]));  // never collect
  }
}
```

All 25 metamethod name strings are created and **fixed** in GC (moved to `fixedgc` list). They are interned short strings, so metamethod lookup in tables is a pointer comparison.

---

## 7. Module Interconnections

### 7.1 Dependency Graph

```
lua_newstate (lstate.c)
  │
  ├── creates global_State with embedded main thread
  ├── calls luaS_init (lstring.c)
  │     └── creates string table, pre-allocates memerrmsg
  ├── calls luaT_init (ltm.c)
  │     └── creates 25 metamethod name strings (uses lstring)
  ├── calls init_registry (lstate.c)
  │     └── creates registry Table (uses ltable)
  │         └── creates globals Table (uses ltable)
  └── calls luaX_init (llex.c)
        └── creates reserved word strings (uses lstring)
```

### 7.2 Runtime Interaction Patterns

**VM → ltable**: Every `GETTABLE`, `SETTABLE`, `GETTABUP`, etc. opcode calls `luaH_get*` / `luaH_pset*`.

**VM → ltm**: When a table operation fails (key absent, no value), the VM checks for metamethods via `fasttm` or `luaT_gettmbyobj`.

**ltm → ltable**: `luaT_gettm` calls `luaH_Hgetshortstr` to look up metamethod names in metatables.

**ltm → lstring**: Metamethod names are pre-interned `TString*` stored in `G(L)->tmname[]`. Lookup is by pointer identity.

**lfunc → lstate**: Open upvalues link to stack slots in `lua_State`. The `twups` list connects threads with open upvalues.

**lfunc → ltm**: `callclosemethod` (lfunc.c:107) uses `luaT_gettmbyobj` to find `__close` metamethods.

**ltable → lstring**: Short string keys use pointer equality (`eqshrstr`). Long string keys use `luaS_eqstr`. External strings must be normalized via `luaS_normstr` before use as table keys.

### 7.3 The String→Table Fast Path

When the VM does `t.name` (string key access):
1. The string `"name"` is already interned (short string, pointer-unique)
2. `luaH_Hgetshortstr` hashes the string's pre-computed hash
3. Walks the chain comparing pointers only (`eqshrstr` = `==`)
4. No memcmp, no length check — just pointer comparison

This is why string interning is critical for table performance.

---

## 8. If I Were Building This in Go

### 8.1 State Management (lstate)

```go
type LuaState struct {
    global    *GlobalState      // shared state
    stack     []StackValue      // Go slice (auto-growing)
    ci        *CallInfo         // current call info
    ciList    []*CallInfo       // pool as slice (append-friendly)
    openUpval *UpVal            // linked list (same as C)
    status    int
    // ...
}

type GlobalState struct {
    registry  *Table
    seed      uint32
    strt      StringTable       // string interning
    tmName    [TM_N]*TString    // metamethod names
    mt        [LUA_NUMTYPES]*Table // type metatables
    // No GC fields — Go GC handles this
}
```

**Key differences**:
- Stack as a Go slice (no manual `stack_last` tracking)
- CallInfo pool as a slice with append (no manual linked-list management)
- No GC fields — Go's garbage collector handles all object lifecycle
- `nCcalls` can be a simple `int` (Go doesn't need the yield-tracking hack)

### 8.2 Tables (ltable)

**Option A: Go map (simple but lossy)**
```go
type Table struct {
    hash      map[Value]Value   // Go's built-in map
    array     []Value           // separate array part
    metatable *Table
    flags     byte              // metamethod cache
}
```

**Option B: Custom implementation (faithful)**
```go
type Table struct {
    array     []TValue          // array part (simple slice)
    arrTags   []byte            // separate tag array (match C layout)
    nodes     []Node            // hash part
    lsizenode uint8             // log2 of node count
    flags     byte              // metamethod cache + dummy flag
    metatable *Table
    lastfree  int               // index for free-slot search
    lenHint   uint              // length hint for # operator
}

type Node struct {
    val    TValue
    keyTT  byte
    keyVal Value
    next   int32               // offset (matching C design)
}
```

**Recommendation**: Option B for correctness. Go maps don't preserve:
- The array/hash split optimization
- Brent's collision resolution invariant
- The `#` operator semantics (boundary finding)
- The exact iteration order (Go maps randomize iteration)
- The pset/finishset two-phase pattern for metamethods

### 8.3 String Interning (lstring)

```go
type StringTable struct {
    mu      sync.RWMutex       // thread safety for concurrent access
    buckets [][]* TString      // or use sync.Map for lock-free reads
    count   int
}

// Short strings: interned, compared by pointer
type TString struct {
    hash    uint32
    data    string             // Go strings are immutable — natural interning candidate
    isShort bool
    extra   byte               // reserved word flag
}
```

**Go advantage**: Go strings are immutable and the runtime already has string interning for small strings. However, we still need explicit interning for Lua semantics (pointer equality for table keys).

**Alternative**: Use `sync.Map` for the intern table:
```go
var internTable sync.Map  // map[string]*TString

func InternString(s string) *TString {
    if ts, ok := internTable.Load(s); ok {
        return ts.(*TString)
    }
    ts := &TString{data: s, hash: hashString(s)}
    actual, _ := internTable.LoadOrStore(s, ts)
    return actual.(*TString)
}
```

### 8.4 Upvalues (lfunc)

```go
type UpVal struct {
    value *TValue              // points to stack slot (open) or own value (closed)
    own   TValue               // storage for closed value
    next  *UpVal               // open list link
    // No 'previous' needed if we use a different list structure
}

func (uv *UpVal) IsOpen() bool {
    return uv.value != &uv.own
}

func (uv *UpVal) Close() {
    uv.own = *uv.value         // copy value
    uv.value = &uv.own         // redirect
}
```

**Go trap**: The `value *TValue` pointer must point into the stack slice. If the stack slice grows (via `append`), all pointers are invalidated. Solutions:
1. Use indices instead of pointers: `stackIndex int` + reference to the stack
2. Pre-allocate stack to max size (wasteful)
3. Use a stable-address stack (e.g., linked list of chunks)

### 8.5 Metamethods (ltm)

```go
type TMS int

const (
    TM_INDEX TMS = iota
    TM_NEWINDEX
    // ...
    TM_N
)

var tmNames [TM_N]*TString  // initialized once

func GetTMByObj(L *LuaState, o *TValue, event TMS) *TValue {
    var mt *Table
    switch o.Type() {
    case LUA_TTABLE:
        mt = o.TableValue().metatable
    case LUA_TUSERDATA:
        mt = o.UserdataValue().metatable
    default:
        mt = L.global.mt[o.Type()]
    }
    if mt == nil {
        return NilValue
    }
    return mt.GetShortStr(tmNames[event])
}
```

The fasttm optimization translates directly — the `flags` byte cache works the same way.

---

## 9. Edge Cases & Traps

### 9.1 Table Array/Hash Boundary

**Trap**: `t->asize` is the allocated array size, but not all slots may be occupied. The `#` operator finds a "boundary" (present key followed by absent key), which may not equal `asize`.

**Off-by-one source**: Array indices in C are 0-based (`getArrTag(t, k)` uses 0-based `k`), but Lua keys are 1-based. The conversion: `ikeyinarray(t, key)` returns 0 if not in array, or the 1-based index. The C-index is `k - 1`.

**Float-to-int coercion**: `luaH_get` converts float keys with integer values to integers before lookup. This means `t[3.0]` and `t[3]` access the same slot. If your Go implementation doesn't do this, `t[3.0] = "x"; print(t[3])` will fail.

### 9.2 Dead Keys in Hash Iteration

**Trap**: During `luaH_next`, a key may have been collected by GC (becoming a "dead key" with `LUA_TDEADKEY` type). `findindex` uses `getgeneric(t, key, 1)` with `deadok=1` to match dead keys by identity.

**Why it's safe**: Dead keys can produce false positives (a new object at the same address), but this only causes `next` to return a different valid entry or nil — never a crash or infinite loop.

### 9.3 Upvalue Close Ordering

**Trap**: `luaF_closeupval` processes upvalues from highest stack level to lowest (the list is sorted descending). If you close in the wrong order, a `__close` method might access an already-closed upvalue.

**The ordering guarantee**: `luaF_close` closes upvalues FIRST (via `luaF_closeupval`), THEN processes tbc variables. This ensures `__close` methods can still read captured values.

**Stack movement**: During `__close` calls, the stack may be reallocated. `luaF_close` uses `savestack`/`restorestack` to handle this.

### 9.4 String Hash Seed Randomization

**Trap**: `g->seed` is passed to `lua_newstate` and used in ALL hash computations. Different seeds produce different hash values, which means:
- Table iteration order varies between runs
- String table bucket assignment varies
- The `#` operator may return different valid boundaries

**Testing trap**: If your tests depend on iteration order or specific hash distributions, they'll break with different seeds. Use seed=0 for deterministic testing.

### 9.5 The fasttm Cache Invalidation

**Trap**: The `flags` cache in Table must be invalidated whenever:
1. A metatable is SET on the table (`invalidateTMcache(t)`)
2. A key is ADDED to a metatable (the metatable's own flags are invalidated)
3. A metatable is REMOVED (flags should be reset to `maskflags`)

**Missing invalidation** = metamethods silently ignored. This is a subtle bug because:
- It only manifests when a metamethod is added AFTER the table was first accessed
- The first access caches "no metamethod", and subsequent accesses use the stale cache

### 9.6 The dummynode Trap

**Trap**: Empty tables share a single static `dummynode`. You must NEVER write to it. The `isdummy(t)` check (via `BITDUMMY` flag) guards against this.

**In Go**: If you use a shared sentinel node, ensure it's truly immutable. In Go, there's no `const` for structs, so use a function that returns a copy or a package-level unexported var.

### 9.7 External String Normalization

**Trap**: External strings (`LSTRFIX`, `LSTRMEM`) are long strings even if their content is short. Before using them as table keys, they MUST be normalized via `luaS_normstr` (ltable.c:1171-1177 in `luaH_finishset`). Without normalization, a short external string and an interned short string with the same content would be different table keys.

---

## 10. Bug Pattern Guide

### 10.1 Table Implementation Bugs

| Bug Pattern | Symptom | Root Cause |
|---|---|---|
| Missing float→int coercion in get/set | `t[3.0]` and `t[3]` are different keys | Forgot `luaV_flttointeger` check |
| Wrong hash function for type | Keys cluster in few buckets, slow lookup | Using `hashpow2` for pointer types (should use `hashmod`) |
| Not checking `isdummy` in insert | Writes to shared dummynode, corrupts all empty tables | Missing guard in `insertkey` |
| Wrong `next` offset arithmetic | Infinite loops or wrong chain traversal | `gnext` is an offset, not an index |
| Missing `invalidateTMcache` | Metamethods silently ignored after metatable change | Forgot to clear flags on metatable set |
| Array index off-by-one | Wrong values returned for `t[1]` | Confusing 0-based C index with 1-based Lua key |
| Not handling nil key in set | Crash or silent corruption | Must error on `t[nil] = value` |
| Not handling NaN key in set | Silent data loss | Must error on `t[NaN] = value` |
| Rehash doesn't count extra key | Table too small after rehash | `rehash` must include the triggering key in count |

### 10.2 Upvalue Lifecycle Bugs

| Bug Pattern | Symptom | Root Cause |
|---|---|---|
| Not sharing upvalues | Closures don't share state | `findupval` must return existing UpVal for same level |
| Wrong close ordering | `__close` sees garbage values | Must close upvalues before tbc variables |
| Not updating `twups` list | GC misses open upvalues, premature collection | Forgot to add thread to `G(L)->twups` |
| Stack realloc invalidates upvalue pointers | Dangling pointer crash | Open upvalues use `v.p` pointing to stack; must update on realloc |
| Closing already-closed upvalue | Double-free or corruption | Must check `upisopen` before closing |
| Not handling stack movement in `__close` | Wrong stack slot accessed | Must use `savestack`/`restorestack` around metamethod calls |

### 10.3 Metamethod Dispatch Bugs

| Bug Pattern | Symptom | Root Cause |
|---|---|---|
| Wrong operand priority | `a + b` uses `b`'s metamethod when `a` has one | Must check first operand first |
| Not flipping operands for commutative ops | Wrong argument order in metamethod call | `flip` parameter ignored |
| fasttm cache not invalidated | Metamethods silently skipped | Missing `invalidateTMcache` on metatable change |
| Using `luaT_gettm` for non-fast methods | Assertion failure (event > TM_EQ) | `luaT_gettm` only works for events 0-5; use `luaT_gettmbyobj` for others |
| Not checking type metatable | Metamethods on numbers/strings don't work | Must fall through to `G(L)->mt[type]` for non-table/userdata |
| Yield in non-yieldable context | Crash or assertion | Must check `isLuacode(L->ci)` before using `luaD_call` vs `luaD_callnoyield` |

### 10.4 String Interning Bugs

| Bug Pattern | Symptom | Root Cause |
|---|---|---|
| Not interning short strings | Pointer comparison fails for equal strings | All short strings must go through `internshrstr` |
| Hash seed mismatch | Strings not found in table after state transfer | Using wrong seed or no seed |
| Not resurrecting dead strings | Duplicate interned strings | Must `changewhite` on dead-but-found strings |
| String table not resized | O(n) lookup as table fills | Must grow when `nuse >= size` |
| External string used as table key without normalization | Two equal strings map to different keys | Must call `luaS_normstr` for external strings |
| Cache not cleared on GC | Dangling pointers in `strcache` | Must replace collected entries with `memerrmsg` |

### 10.5 State Management Bugs

| Bug Pattern | Symptom | Root Cause |
|---|---|---|
| GC enabled during bootstrap | Crash accessing incomplete state | Must set `gcstp = GCSTPGC` until `f_luaopen` completes |
| Main thread not marked non-yieldable | Yield from main thread | Must `incnny(L)` for main thread |
| CallInfo pool leak | Memory grows unbounded | `luaE_shrinkCI` must be called periodically (by GC) |
| Not copying extra space from main thread | Extra space uninitialized in new threads | `lua_newthread` must `memcpy` from `mainthread(g)` |
| `completestate` check missing in close | Crash closing partial state | `close_state` must check `completestate(g)` |

---

## Appendix: Key Constants

| Constant | Value | Defined In | Purpose |
|---|---|---|---|
| `LUAI_MAXSHORTLEN` | 40 | lstring.h:29 | Max length for interned strings |
| `MINSTRTABSIZE` | 128 | lstring.c:37 | Initial string table buckets |
| `STRCACHE_N` | 53 | lstate.h:151 | String cache sets (prime) |
| `STRCACHE_M` | 2 | lstate.h:152 | Entries per cache set |
| `EXTRA_STACK` | 5 | lstate.h:142 | Extra stack for TM calls |
| `BASIC_STACK_SIZE` | 2*LUA_MINSTACK | lstate.h:156 | Initial stack size |
| `MAXUPVAL` | 255 | lfunc.h:29 | Max upvalues per closure |
| `MAXDELTA` | USHRT_MAX | lfunc.c:166 | Max tbc list delta |
| `LIMFORLAST` | 3 | ltable.c:49 | Min hash log2 size for lastfree |
| `MAXABITS` | 31 (on 32-bit) | ltable.c:70 | Max array size as power of 2 |
| `MAXHBITS` | 30 (on 32-bit) | ltable.c:91 | Max hash size as power of 2 |
| `TM_N` | 25 | ltm.h:44 | Number of metamethods |
| `maskflags` | 0x3F | ltm.h:54 | Fast-access metamethod mask (6 bits) |
| `BITDUMMY` | 0x40 | ltable.h:31 | Dummy node flag in table flags |
