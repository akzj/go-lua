# 08 — Garbage Collector & Memory Allocation (lgc.c / lgc.h / lmem.c / lmem.h)

> **Source**: Lua 5.5.1 — `lgc.c` (1804 lines), `lgc.h` (268 lines), `lmem.c` (215 lines), `lmem.h` (96 lines)
> **Scope**: Incremental & generational GC, tri-color marking, write barriers, finalization, memory allocation, GC debt mechanism, Go reimplementation guidance.

---

## Table of Contents

1. [GC Architecture Overview](#1-gc-architecture-overview)
2. [Core Data Structures](#2-core-data-structures)
3. [Mark Phase](#3-mark-phase)
4. [Sweep Phase](#4-sweep-phase)
5. [Atomic Phase](#5-atomic-phase)
6. [Generational Mode](#6-generational-mode)
7. [Write Barriers](#7-write-barriers)
8. [Finalization](#8-finalization)
9. [lmem.c — Memory Allocation](#9-lmemc--memory-allocation)
10. [If I Were Building This in Go](#10-if-i-were-building-this-in-go)
11. [Edge Cases & Traps](#11-edge-cases--traps)
12. [Bug Pattern Guide](#12-bug-pattern-guide)

---

## 1. GC Architecture Overview

### 1.1 Two Collection Modes

Lua 5.5.1 supports two garbage collection modes that can be switched at runtime:

| Mode | Constant | Description |
|------|----------|-------------|
| **Incremental** | `KGC_INC` (0) | Classic tri-color mark-and-sweep, interleaved with mutator |
| **Generational Minor** | `KGC_GENMINOR` (1) | Young-generation collection (most collections) |
| **Generational Major** | `KGC_GENMAJOR` (2) | Full collection triggered from generational mode |

(`lstate.h:162-164`)

The generational collector is built **on top of** the incremental collector — they share the same mark, sweep, and atomic infrastructure. The generational mode adds age tracking and the concept of young vs. old objects.

### 1.2 Tri-Color Marking

From the comment in `lgc.h:18-30`:

> Collectable objects may have one of three colors:
> - **White**: not marked (candidate for collection)
> - **Gray**: marked, but its references may not be marked
> - **Black**: marked, and all its references are marked

**The Main Invariant** (enforced during mark phase): A black object can never point to a white one. Any gray object must be in a "gray list" (gray, grayagain, weak, ephemeron, allweak) so it can be visited before the cycle ends. Open upvalues are an exception — they're gray but attached to their thread via `twups`.

The invariant is only maintained when `keepinvariant(g)` is true (`lgc.h:61`):
```c
#define keepinvariant(g)  ((g)->gcstate <= GCSatomic)
```
During sweep phases, the invariant is broken (white objects may point to black ones) and restored when sweep ends.

### 1.3 The GC State Machine

The incremental collector cycles through these states (`lgc.h:42-50`):

```
                    ┌──────────────────────────────────────────────────────────────┐
                    │                                                              │
                    ▼                                                              │
              ┌──────────┐     ┌──────────────┐     ┌───────────┐                 │
              │ GCSpause │────▶│ GCSpropagate │────▶│GCSenter-  │                 │
              │   (8)    │     │     (0)      │     │ atomic(1) │                 │
              └──────────┘     └──────────────┘     └─────┬─────┘                 │
                    ▲                                      │                       │
                    │                                      ▼                       │
              ┌──────────┐                          ┌───────────┐                 │
              │GCScall-  │                          │ GCSatomic │                 │
              │  fin (7) │                          │    (2)    │                 │
              └────┬─────┘                          └─────┬─────┘                 │
                   │                                      │                       │
                   │                                      ▼                       │
              ┌──────────┐     ┌──────────────┐     ┌───────────┐                 │
              │ GCSswp-  │◀───│ GCSswp-      │◀───│GCSswp-    │                 │
              │  end (6) │     │ tobefnz (5)  │     │ finobj(4) │                 │
              └──────────┘     └──────────────┘     └─────┬─────┘                 │
                                                          │                       │
                                                    ┌───────────┐                 │
                                                    │GCSswp-    │─────────────────┘
                                                    │ allgc (3) │
                                                    └───────────┘
```

**State values** (`lgc.h:42-50`):

| State | Value | Purpose |
|-------|-------|---------|
| `GCSpropagate` | 0 | Incrementally traverse gray objects |
| `GCSenteratomic` | 1 | Transition: about to enter atomic phase |
| `GCSatomic` | 2 | Atomic (stop-the-world) mark completion |
| `GCSswpallgc` | 3 | Sweep `allgc` list |
| `GCSswpfinobj` | 4 | Sweep `finobj` list |
| `GCSswptobefnz` | 5 | Sweep `tobefnz` list |
| `GCSswpend` | 6 | Finish sweeps, shrink string table |
| `GCScallfin` | 7 | Call pending finalizers |
| `GCSpause` | 8 | Idle, waiting for next cycle |

**Key insight**: `issweepphase(g)` is `GCSswpallgc <= gcstate <= GCSswpend` (`lgc.h:53`).

### 1.4 Incremental vs Generational Flow

**Incremental** (`luaC_step` → `incstep` → `singlestep`, `lgc.c:1627-1658`):
Each allocation checks `GCdebt <= 0`. If so, `luaC_step` runs one or more `singlestep` calls, advancing the state machine. The `STEPMUL` parameter controls how many work units per step.

**Generational Minor** (`luaC_step` → `youngcollection`, `lgc.c:1668-1670`):
A complete young-generation collection runs in one shot (not incremental). It marks OLD1 objects, runs the atomic phase, sweeps young objects, and advances ages.

**Generational Major** (`luaC_step` → `incstep`, `lgc.c:1665-1666`):
Uses the same incremental machinery as `KGC_INC`, but after the atomic phase checks whether to shift back to minor mode via `checkmajorminor`.

---

## 2. Core Data Structures

### 2.1 GCObject Header — The `marked` Byte

Every collectable object starts with `CommonHeader` (`lobject.h`):
```c
#define CommonHeader  struct GCObject *next; lu_byte tt; lu_byte marked
```

The `marked` byte is a bitfield encoding color, age, and finalization status (`lgc.h:82-94`):

```
Bit layout of 'marked' byte:
┌───┬───┬───┬───┬───┬───┬───┬───┐
│ 7 │ 6 │ 5 │ 4 │ 3 │ 2 │ 1 │ 0 │
│TST│FIN│BLK│WH1│WH0│age│age│age│
└───┴───┴───┴───┴───┴───┴───┴───┘
```

| Bits | Name | Constant | Purpose |
|------|------|----------|---------|
| 0-2 | Age | `AGEBITS` (7) | Object age in generational mode (0-6) |
| 3 | White0 | `WHITE0BIT` (3) | White color, type 0 |
| 4 | White1 | `WHITE1BIT` (4) | White color, type 1 |
| 5 | Black | `BLACKBIT` (5) | Black color |
| 6 | Finalized | `FINALIZEDBIT` (6) | Object marked for finalization |
| 7 | Test | `TESTBIT` (7) | Used only by tests |

**Color determination** (`lgc.h:95-99`):
```c
#define iswhite(x)  testbits((x)->marked, WHITEBITS)    // either white bit set
#define isblack(x)  testbit((x)->marked, BLACKBIT)       // black bit set
#define isgray(x)   (!testbits((x)->marked, WHITEBITS | bitmask(BLACKBIT)))  // neither
```

**Critical**: An object is gray when **neither** white bits **nor** black bit is set. Gray is not a bit — it's the absence of both color bits.

**Two-white system**: There are two white values (`WHITE0BIT` and `WHITE1BIT`). The `currentwhite` in `global_State` holds the "current" white. At the end of the atomic phase, white is flipped (`lgc.c:1571`):
```c
g->currentwhite = cast_byte(otherwhite(g));  /* flip current white */
```
After the flip, objects still marked with the old white are "dead" — they weren't reached during marking. This is how the sweep phase identifies dead objects without clearing marks: `isdead(g,v)` checks if the object has the "other" white (`lgc.h:101`).

### 2.2 Object Ages (Generational Mode)

Ages occupy bits 0-2 of the `marked` byte (`lgc.h:112-118`):

| Age | Value | Meaning | Color | Can point to |
|-----|-------|---------|-------|--------------|
| `G_NEW` | 0 | Created in current cycle | White | Anything |
| `G_SURVIVAL` | 1 | Survived one cycle | White | Anything |
| `G_OLD0` | 2 | Marked old by forward barrier this cycle | Gray/Black | New, Survival |
| `G_OLD1` | 3 | First full cycle as old | Black | Survival (not New) |
| `G_OLD` | 4 | Really old | Black | Only old objects |
| `G_TOUCHED1` | 5 | Old object touched this cycle | Gray | Anything (re-traversed) |
| `G_TOUCHED2` | 6 | Old object touched in previous cycle | Black | Survival (not New) |

(`lgc.h:112-118`, with detailed explanation in `lgc.h:126-170`)

**Age macros** (`lgc.h:120-122`):
```c
#define getage(o)    ((o)->marked & AGEBITS)
#define setage(o,a)  ((o)->marked = cast_byte(((o)->marked & (~AGEBITS)) | a))
#define isold(o)     (getage(o) > G_SURVIVAL)
```

**Young objects** = `G_NEW` + `G_SURVIVAL` (both traversed every minor cycle).

### 2.3 global_State GC Fields

All GC-related fields in `global_State` (`lstate.h:330-363`):

#### Accounting Fields
| Field | Type | Line | Purpose |
|-------|------|------|---------|
| `GCtotalbytes` | `l_mem` | 330 | Total bytes allocated + debt |
| `GCdebt` | `l_mem` | 331 | Bytes counted but not yet allocated (controls GC pacing) |
| `GCmarked` | `l_mem` | 332 | Bytes marked in current cycle (multi-purpose, see §1) |
| `GCmajorminor` | `l_mem` | 333 | Auxiliary counter for major↔minor shifts |

**Actual bytes in use** = `GCtotalbytes - GCdebt` (`lstate.h:435`):
```c
#define gettotalbytes(g)  ((g)->GCtotalbytes - (g)->GCdebt)
```

#### Control Fields
| Field | Type | Line | Purpose |
|-------|------|------|---------|
| `gcparams[6]` | `lu_byte[6]` | 338 | Encoded GC parameters (pause, stepmul, etc.) |
| `currentwhite` | `lu_byte` | 339 | Current white bit (WHITE0BIT or WHITE1BIT) |
| `gcstate` | `lu_byte` | 340 | Current GC state (0-8) |
| `gckind` | `lu_byte` | 341 | GC mode: KGC_INC/KGC_GENMINOR/KGC_GENMAJOR |
| `gcstopem` | `lu_byte` | 342 | Stops emergency collections (prevents reentrance) |
| `gcstp` | `lu_byte` | 343 | Controls whether GC is running (GCSTPUSR/GCSTPGC/GCSTPCLS) |
| `gcemergency` | `lu_byte` | 344 | True during emergency collection |

#### Object Lists (Incremental)
| Field | Type | Line | Purpose |
|-------|------|------|---------|
| `allgc` | `GCObject*` | 345 | All collectable objects not marked for finalization |
| `sweepgc` | `GCObject**` | 346 | Current sweep position (pointer to `next` field) |
| `finobj` | `GCObject*` | 347 | Objects with `__gc` metamethod |
| `gray` | `GCObject*` | 348 | Gray objects waiting to be traversed |
| `grayagain` | `GCObject*` | 349 | Objects to re-traverse in atomic phase |
| `weak` | `GCObject*` | 350 | Tables with weak values (to clear) |
| `ephemeron` | `GCObject*` | 351 | Ephemeron tables (weak keys) |
| `allweak` | `GCObject*` | 352 | Tables with both weak keys and values |
| `tobefnz` | `GCObject*` | 353 | Objects ready to be finalized |
| `fixedgc` | `GCObject*` | 354 | Objects not to be collected (e.g., reserved strings) |

#### Generational Sub-lists (partition `allgc` by age)
| Field | Type | Line | Purpose |
|-------|------|------|---------|
| `survival` | `GCObject*` | 356 | Start of survival-age objects in `allgc` |
| `old1` | `GCObject*` | 357 | Start of OLD1-age objects in `allgc` |
| `reallyold` | `GCObject*` | 358 | Start of really-old objects in `allgc` |
| `firstold1` | `GCObject*` | 359 | First OLD1 object anywhere in the list (optimization) |
| `finobjsur` | `GCObject*` | 360 | Survival objects in `finobj` list |
| `finobjold1` | `GCObject*` | 361 | OLD1 objects in `finobj` list |
| `finobjrold` | `GCObject*` | 362 | Really-old objects in `finobj` list |
| `twups` | `lua_State*` | 363 | Threads with open upvalues |

**List structure in generational mode** (from `lstate.h:28-46`):
```
allgc → [NEW objects] → survival → [SURVIVAL objects] → old1 → [OLD1 objects] → reallyold → [OLD objects] → NULL
finobj → [NEW finobj] → finobjsur → [SURVIVAL finobj] → finobjold1 → [OLD1 finobj] → finobjrold → [OLD finobj] → NULL
```

The `firstold1` pointer is an optimization: it points to the first OLD1 object *anywhere* in the list (not necessarily after `old1`). This avoids scanning the entire list in `markold` (`lgc.c:1283-1291`).

### 2.4 GC Parameters

Encoded via `luaO_codeparam`/`luaO_applyparam` (`lgc.h:199-200`):

| Parameter | Index | Default | Purpose |
|-----------|-------|---------|---------|
| `LUA_GCPMINORMUL` | 0 | `LUAI_GENMINORMUL` (20) | Minor collection frequency (% of base) |
| `LUA_GCPMAJORMINOR` | 1 | `LUAI_MAJORMINOR` (50) | Threshold for major→minor shift |
| `LUA_GCPMINORMAJOR` | 2 | `LUAI_MINORMAJOR` (70) | Threshold for minor→major shift |
| `LUA_GCPPAUSE` | 3 | `LUAI_GCPAUSE` (250) | Pause between incremental cycles (%) |
| `LUA_GCPSTEPMUL` | 4 | `LUAI_GCMUL` (200) | Step multiplier (work units per word) |
| `LUA_GCPSTEPSIZE` | 5 | `LUAI_GCSTEPSIZE` (200*sizeof(Table)) | Step size in bytes |

(`lgc.h:173-196`, `lua.h:347-357`)

### 2.5 Gray Lists and `gclist`

Gray objects are linked through a `gclist` field that exists in different types at different offsets. The function `getgclist` (`lgc.c:145-158`) handles the polymorphism:

```c
static GCObject **getgclist (GCObject *o) {
  switch (o->tt) {
    case LUA_VTABLE: return &gco2t(o)->gclist;
    case LUA_VLCL: return &gco2lcl(o)->gclist;
    case LUA_VCCL: return &gco2ccl(o)->gclist;
    case LUA_VTHREAD: return &gco2th(o)->gclist;
    case LUA_VPROTO: return &gco2p(o)->gclist;
    case LUA_VUSERDATA: return &gco2u(o)->gclist;  // only if nuvalue > 0
    default: lua_assert(0); return 0;
  }
}
```

**Objects that can be gray**: Tables, Lua closures, C closures, threads, protos, userdata (with user values). Strings and upvalues **cannot** be gray (strings go directly to black; open upvalues are gray but not in gray lists).

The `linkgclist_` function (`lgc.c:167-172`) links an object into a gray list:
```c
static void linkgclist_ (GCObject *o, GCObject **pnext, GCObject **list) {
  lua_assert(!isgray(o));  /* cannot be in a gray list */
  *pnext = *list;      // o->gclist = old head
  *list = o;           // list head = o
  set2gray(o);         // now it is gray
}
```

---

## 3. Mark Phase

### 3.1 Starting a Collection — `restartcollection`

(`lgc.c:412-420`)

```c
static void restartcollection (global_State *g) {
  cleargraylists(g);       // reset gray, grayagain, weak, allweak, ephemeron
  g->GCmarked = 0;         // reset byte counter
  markobject(g, mainthread(g));    // mark the main thread (root)
  markvalue(g, &g->l_registry);    // mark the registry (root)
  markmt(g);                       // mark global metatables (roots)
  markbeingfnz(g);                 // mark objects being finalized (from previous cycle)
}
```

**Root set**: main thread + registry + global metatables + objects in `tobefnz`.

### 3.2 `reallymarkobject` — The Core Marking Function

(`lgc.c:326-358`)

This is the central dispatch for marking a single object. It first adds the object's size to `GCmarked`, then handles each type:

```
reallymarkobject(g, o):
  g->GCmarked += objsize(o)
  switch o->tt:
    case SHRSTR, LNGSTR:
      set2black(o)                    // strings have no references → black immediately
    
    case UPVAL:
      if upisopen(uv):
        set2gray(uv)                  // open upvalues stay gray (no barrier needed)
      else:
        set2black(uv)                 // closed upvalues → black
      markvalue(g, uv->v.p)          // mark the value the upvalue points to
    
    case USERDATA (no user values):
      markobjectN(g, u->metatable)   // mark metatable
      set2black(u)                    // no more references → black
    
    case USERDATA (with user values), LCL, CCL, TABLE, THREAD, PROTO:
      linkobjgclist(o, g->gray)      // add to gray list for later traversal
```

**Key insight**: Objects with no outgoing references (strings, closed upvalues, simple userdata) go directly to black. Complex objects go to the gray list for incremental traversal.

**Recursion depth**: The comment at `lgc.c:315-320` notes that `reallymarkobject` can recurse at most 2 levels deep: an upvalue can mark its value (which may be a userdata), and a userdata can mark its metatable (which is a table → goes to gray list). So the recursion is bounded.

### 3.3 Traversal Functions

Each complex object type has a traversal function that marks all references and returns an estimate of work done (in "slots"):

#### 3.3.1 `traversetable` (`lgc.c:608-621`)

```c
static l_mem traversetable (global_State *g, Table *h) {
  markobjectN(g, h->metatable);
  switch (getmode(g, h)) {
    case 0: traversestrongtable(g, h);    break;  // no weak refs
    case 1: traverseweakvalue(g, h);      break;  // weak values
    case 2: traverseephemeron(g, h, 0);   break;  // weak keys (ephemeron)
    case 3: /* all weak */
      if (g->gcstate == GCSpropagate)
        linkgclist(h, g->grayagain);    // revisit in atomic
      else
        linkgclist(h, g->allweak);      // clear later
      break;
  }
  return 1 + 2*sizenode(h) + h->asize;  // work estimate
}
```

The `getmode` function (`lgc.c:595-604`) checks the `__mode` metamethod: `'k'` = weak keys, `'v'` = weak values, `'kv'` = both.

**Strong table** (`traversestrongtable`, `lgc.c:582-593`): Marks all keys and values in both array and hash parts. Calls `genlink` for generational age tracking.

**Weak-value table** (`traverseweakvalue`, `lgc.c:466-486`): Marks keys but not values. During propagation, goes to `grayagain` for atomic revisit. In atomic phase, if any white value found, goes to `weak` list for clearing.

**Ephemeron table** (`traverseephemeron`, `lgc.c:510-550`): Complex — marks values only if their corresponding keys are already marked. Tracks white→white entries. Goes to `ephemeron` list if white→white entries exist (needs convergence iteration).

#### 3.3.2 `traverseproto` (`lgc.c:641-652`)

Marks: source string, all constants (`k[]`), upvalue names, nested protos, local variable names.
```
Work = 1 + sizek + sizeupvalues + sizep + sizelocvars
```

#### 3.3.3 `traverseLclosure` (`lgc.c:668-676`)

Marks: prototype, all upvalue objects.
```
Work = 1 + nupvalues
```

#### 3.3.4 `traverseCclosure` (`lgc.c:659-665`)

Marks: all upvalue TValues.
```
Work = 1 + nupvalues
```

#### 3.3.5 `traversethread` (`lgc.c:692-720`)

The most complex traversal:

```c
static l_mem traversethread (global_State *g, lua_State *th) {
  UpVal *uv;
  StkId o = th->stack.p;
  // Re-add to grayagain if old (gen mode) or propagating (inc mode)
  if (isold(th) || g->gcstate == GCSpropagate)
    linkgclist(th, g->grayagain);
  if (o == NULL) return 0;  // stack not built yet
  // Mark live stack elements
  for (; o < th->top.p; o++)
    markvalue(g, s2v(o));
  // Mark open upvalues (they can't be collected while thread lives)
  for (uv = th->openupval; uv != NULL; uv = uv->u.open.next)
    markobject(g, uv);
  // In atomic phase: shrink stack, clear dead slots, re-link to twups
  if (g->gcstate == GCSatomic) {
    if (!g->gcemergency)
      luaD_shrinkstack(th);
    for (o = th->top.p; o < th->stack_last.p + EXTRA_STACK; o++)
      setnilvalue(s2v(o));  // clear dead stack slice
    if (!isintwups(th) && th->openupval != NULL) {
      th->twups = g->twups;
      g->twups = th;
    }
  }
  return 1 + (th->top.p - th->stack.p);
}
```

**Critical behaviors**:
1. Threads are **always** re-added to `grayagain` during propagation (because the mutator can modify the stack before atomic phase) and when old (gen mode needs to re-traverse old threads).
2. During the atomic phase, dead stack slots are cleared to nil (prevents dangling references).
3. Stack shrinking only happens in atomic phase (not during incremental steps).
4. Open upvalues are marked to prevent premature collection.

#### 3.3.6 `traverseudata` (`lgc.c:632-638`)

Marks: metatable, all user values (`uv[]`). Calls `genlink` for age tracking.

### 3.4 `propagatemark` — Gray→Black Transition

(`lgc.c:726-739`)

```c
static l_mem propagatemark (global_State *g) {
  GCObject *o = g->gray;
  nw2black(o);                    // mark black (must not be white)
  g->gray = *getgclist(o);       // remove from gray list
  switch (o->tt) {
    case LUA_VTABLE: return traversetable(g, gco2t(o));
    case LUA_VUSERDATA: return traverseudata(g, gco2u(o));
    case LUA_VLCL: return traverseLclosure(g, gco2lcl(o));
    case LUA_VCCL: return traverseCclosure(g, gco2ccl(o));
    case LUA_VPROTO: return traverseproto(g, gco2p(o));
    case LUA_VTHREAD: return traversethread(g, gco2th(o));
  }
}
```

Each call to `propagatemark` processes ONE gray object: turns it black, removes it from the gray list, and traverses its references (which may add more objects to the gray list).

### 3.5 Mark Helpers

**`markmt`** (`lgc.c:361-365`): Marks all global metatables (`g->mt[0..LUA_NUMTYPES-1]`).

**`markbeingfnz`** (`lgc.c:371-376`): Marks all objects in the `tobefnz` list (objects whose finalizers are pending).

**`remarkupvals`** (`lgc.c:387-407`): For each non-marked thread in the `twups` list, simulates barriers between open upvalues and their values. Removes dead/upvalue-less threads from `twups`. This is critical: if a thread dies, its open upvalues will be closed, and the upvalue's value must be properly marked.

### 3.6 `genlink` — Generational Post-Mark Hook

(`lgc.c:440-448`)

Called after traversing an object in generational mode:
```c
static void genlink (global_State *g, GCObject *o) {
  lua_assert(isblack(o));
  if (getage(o) == G_TOUCHED1)       // touched this cycle?
    linkobjgclist(o, g->grayagain);  // must revisit in atomic
  else if (getage(o) == G_TOUCHED2)
    setage(o, G_OLD);                // advance to OLD
}
```

This is a no-op in incremental mode (objects are never TOUCHED1/TOUCHED2 in inc mode).

---

## 4. Sweep Phase

### 4.1 `sweeplist` — The Core Sweep Function

(`lgc.c:841-860`)

```c
static GCObject **sweeplist (lua_State *L, GCObject **p, l_mem countin) {
  global_State *g = G(L);
  int ow = otherwhite(g);
  int white = luaC_white(g);  /* current white */
  while (*p != NULL && countin-- > 0) {
    GCObject *curr = *p;
    int marked = curr->marked;
    if (isdeadm(ow, marked)) {  /* is 'curr' dead? */
      *p = curr->next;          /* remove 'curr' from list */
      freeobj(L, curr);         /* erase 'curr' */
    }
    else {  /* change mark to 'white' and age to 'new' */
      curr->marked = cast_byte((marked & ~maskgcbits) | white | G_NEW);
      p = &curr->next;  /* go to next element */
    }
  }
  return (*p == NULL) ? NULL : p;
}
```

**How dead objects are identified**: After the atomic phase flips `currentwhite`, any object still marked with the "other" white (the old current white) is dead. `isdeadm(ow, marked)` checks `marked & ow` — if the object has the old white bit set, it's dead.

**What happens to surviving objects**: Their `marked` byte is reset: all GC bits (color + age) are cleared, then the new current white and `G_NEW` age are set. This prepares them for the next cycle.

**Return value**: Returns a pointer-to-pointer for continuation, or `NULL` if the list is exhausted.

### 4.2 `sweeptolive` — Sweep Until a Live Object

(`lgc.c:866-872`)

```c
static GCObject **sweeptolive (lua_State *L, GCObject **p) {
  GCObject **old = p;
  do {
    p = sweeplist(L, p, 1);
  } while (p == old);
  return p;
}
```

Sweeps one object at a time until finding a live one. Used at the start of sweep phase to skip any dead objects at the head of the list.

### 4.3 `freeobj` — Object Deallocation

(`lgc.c:809-838`)

Dispatches to type-specific free functions:

| Type | Free Action |
|------|-------------|
| `LUA_VPROTO` | `luaF_freeproto` (frees all sub-arrays) |
| `LUA_VUPVAL` | `freeupval` (unlinks if open, then `luaM_free`) |
| `LUA_VLCL` | `luaM_freemem(cl, sizeLclosure(nupvalues))` |
| `LUA_VCCL` | `luaM_freemem(cl, sizeCclosure(nupvalues))` |
| `LUA_VTABLE` | `luaH_free` (frees array + hash parts) |
| `LUA_VTHREAD` | `luaE_freethread` (frees stack, call info chain) |
| `LUA_VUSERDATA` | `luaM_freemem(o, sizeudata(nuvalue, len))` |
| `LUA_VSHRSTR` | `luaS_remove` (remove from string table hash) + `luaM_freemem` |
| `LUA_VLNGSTR` | If external (`shrlen == LSTRMEM`): call `falloc` to free. Then `luaM_freemem` |

**Short string special handling**: `luaS_remove(L, ts)` removes the string from the global string hash table **before** freeing memory. This is critical — the string table uses the string's hash and contents for lookup.

**Long string external memory**: Lua 5.5 supports external long strings where the buffer is managed by a user-provided allocator (`ts->falloc`). The GC must call this allocator to free the external buffer.

### 4.4 Sweep State Machine Steps

The sweep phase processes three lists in sequence (`lgc.c:1590-1601`):

```
GCSswpallgc → sweep g->allgc
    ↓ (when allgc exhausted)
GCSswpfinobj → sweep g->finobj
    ↓ (when finobj exhausted)
GCSswptobefnz → sweep g->tobefnz
    ↓ (when tobefnz exhausted)
GCSswpend → checkSizes (shrink string table), then → GCScallfin
```

Each `sweepstep` (`lgc.c:1580-1587`) sweeps at most `GCSWEEPMAX` (20) objects per step, or the entire list if `fast` is true:

```c
static void sweepstep (lua_State *L, global_State *g,
                       lu_byte nextstate, GCObject **nextlist, int fast) {
  if (g->sweepgc)
    g->sweepgc = sweeplist(L, g->sweepgc, fast ? MAX_LMEM : GCSWEEPMAX);
  else {  /* enter next state */
    g->gcstate = nextstate;
    g->sweepgc = nextlist;
  }
}
```

### 4.5 `entersweep` — Starting the Sweep

(`lgc.c:1493-1498`)

```c
static void entersweep (lua_State *L) {
  global_State *g = G(L);
  g->gcstate = GCSswpallgc;
  lua_assert(g->sweepgc == NULL);
  g->sweepgc = sweeptolive(L, &g->allgc);
}
```

The `sweeptolive` call ensures the sweep pointer starts at a live object (skipping any dead objects at the head of `allgc`). This prevents newly-created objects (added to the head of `allgc` during sweep) from being swept.

### 4.6 White Flipping — The Key Insight

The two-white system is the mechanism that makes incremental sweep possible:

1. During marking, reachable objects are marked gray/black.
2. New objects created during marking get the **current** white.
3. At the end of atomic phase: `currentwhite` is flipped.
4. Now: old white = dead, new white = alive (or newly created).
5. Sweep checks `isdeadm(otherwhite(g), marked)` — objects with the OLD white are dead.
6. Surviving objects are re-painted with the NEW white.

This means objects created **during sweep** get the new white and won't be collected by the current sweep.

### 4.7 `objsize` — Object Size Calculation

(`lgc.c:105-140`)

Used for GC accounting. Returns the memory footprint of each object type:

| Type | Size |
|------|------|
| Table | `luaH_size(t)` (includes array + hash) |
| LClosure | `sizeLclosure(nupvalues)` |
| CClosure | `sizeCclosure(nupvalues)` |
| Userdata | `sizeudata(nuvalue, len)` |
| Proto | `luaF_protosize(p)` |
| Thread | `luaE_threadsize(th)` |
| Short string | `sizestrshr(shrlen)` |
| Long string | `luaS_sizelngstr(lnglen, shrlen)` |
| UpVal | `sizeof(UpVal)` |

---

## 5. Atomic Phase

### 5.1 Why Atomic?

The atomic phase (`lgc.c:1537-1575`) is the only part of the GC that must run without interleaving with the mutator. This is necessary because:

1. **Weak table clearing** must see a consistent snapshot of reachability.
2. **Finalization separation** (`separatetobefnz`) must see final reachability.
3. **Ephemeron convergence** requires iterating until a fixed point — mutator interference would prevent convergence.
4. **White flipping** must happen atomically with respect to object creation.

### 5.2 `atomic` — Step by Step

(`lgc.c:1537-1575`)

```c
static void atomic (lua_State *L) {
  global_State *g = G(L);
  GCObject *origweak, *origall;
  GCObject *grayagain = g->grayagain;  /* save original list */
  g->grayagain = NULL;
  lua_assert(g->ephemeron == NULL && g->weak == NULL);
  lua_assert(!iswhite(mainthread(g)));
  g->gcstate = GCSatomic;
```

**Phase 1: Re-mark roots that may have changed** (lines 1548-1553)
```c
  markobject(g, L);                  // mark running thread (may differ from main)
  markvalue(g, &g->l_registry);      // registry may change via API
  markmt(g);                         // global metatables may change via API
  propagateall(g);                   // empty the gray list
```

**Phase 2: Handle upvalues of dying threads** (lines 1554-1555)
```c
  remarkupvals(g);                   // mark values of upvalues on dying threads
  propagateall(g);                   // propagate those marks
```

**Phase 3: Re-traverse grayagain list** (lines 1556-1557)
```c
  g->gray = grayagain;              // restore saved grayagain as gray
  propagateall(g);                   // traverse all grayagain objects
```

The `grayagain` list contains:
- Threads (always re-added during propagation — stack may change)
- Weak-value tables (need atomic revisit to identify dead values)
- TOUCHED1 objects in generational mode

**Phase 4: Ephemeron convergence** (line 1558)
```c
  convergeephemerons(g);
```

At this point, **all strongly accessible objects are marked**.

**Phase 5: Clear weak values (first pass)** (lines 1560-1561)
```c
  clearbyvalues(g, g->weak, NULL);
  clearbyvalues(g, g->allweak, NULL);
  origweak = g->weak; origall = g->allweak;
```

**Phase 6: Separate finalizable objects** (lines 1563-1565)
```c
  separatetobefnz(g, 0);            // move unreachable finobj → tobefnz
  markbeingfnz(g);                   // mark objects to be finalized (resurrection!)
  propagateall(g);                   // propagate resurrection marks
  convergeephemerons(g);             // re-converge after resurrection
```

At this point, **all resurrected objects are marked**.

**Phase 7: Final weak table clearing** (lines 1567-1570)
```c
  clearbykeys(g, g->ephemeron);      // clear dead keys from ephemerons
  clearbykeys(g, g->allweak);        // clear dead keys from all-weak tables
  clearbyvalues(g, g->weak, origweak);    // clear new dead values
  clearbyvalues(g, g->allweak, origall);  // clear new dead values
```

**Phase 8: Cleanup** (lines 1571-1573)
```c
  luaS_clearcache(g);                // clear string cache
  g->currentwhite = cast_byte(otherwhite(g));  // FLIP WHITE
  lua_assert(g->gray == NULL);
```

### 5.3 Ephemeron Convergence

(`lgc.c:755-772`)

Ephemeron tables have weak keys — a value is only reachable if its key is reachable. But marking a value may make another key reachable (if the value is used as a key in another ephemeron table). This requires iteration until convergence:

```c
static void convergeephemerons (global_State *g) {
  int changed;
  int dir = 0;
  do {
    GCObject *w;
    GCObject *next = g->ephemeron;
    g->ephemeron = NULL;
    changed = 0;
    while ((w = next) != NULL) {
      Table *h = gco2t(w);
      next = h->gclist;
      nw2black(h);
      if (traverseephemeron(g, h, dir)) {  // marked something?
        propagateall(g);                     // propagate changes
        changed = 1;
      }
    }
    dir = !dir;  // alternate direction for faster convergence
  } while (changed);
}
```

The `dir` parameter alternates traversal direction (forward/backward through hash entries) to speed up convergence on chains within the same table.

### 5.4 Weak Table Clearing

**`clearbyvalues`** (`lgc.c:793-808`): For each table in the `weak` or `allweak` list, clears entries whose values are white (unmarked). Array entries are set to `LUA_VEMPTY`; hash entries are set to empty via `setempty`.

**`clearbykeys`** (`lgc.c:779-791`): For each table in the `ephemeron` or `allweak` list, clears entries whose keys are white. The entry's value is set to empty, and the key is marked dead (`setdeadkey`).

### 5.5 Resurrection

When `separatetobefnz` moves an unreachable object from `finobj` to `tobefnz`, that object needs to be finalized (its `__gc` will be called). But the finalizer receives the object as an argument — so the object must be kept alive. `markbeingfnz` marks all `tobefnz` objects, which may transitively mark other objects that were previously unreachable. This is **resurrection**: dead objects come back to life because a finalizer needs them.

After resurrection, weak tables must be re-cleared (the `origweak`/`origall` trick ensures only newly-resurrected entries are processed in the second pass).

---

## 6. Generational Mode

### 6.1 Overview

The generational GC exploits the **generational hypothesis**: most objects die young. Instead of scanning all objects every cycle, it focuses on young objects (NEW and SURVIVAL), only scanning old objects when they've been "touched" (written to by the mutator).

The generational mode uses the **same atomic phase** as the incremental collector but has its own sweep (`sweepgen`) and age-advancement logic.

### 6.2 Age Transitions

The `sweepgen` function (`lgc.c:1165-1212`) contains a static table defining age transitions:

```c
static const lu_byte nextage[] = {
  G_SURVIVAL,   /* from G_NEW */
  G_OLD1,       /* from G_SURVIVAL */
  G_OLD1,       /* from G_OLD0 */
  G_OLD,        /* from G_OLD1 */
  G_OLD,        /* from G_OLD (no change) */
  G_TOUCHED1,   /* from G_TOUCHED1 (no change) */
  G_TOUCHED2    /* from G_TOUCHED2 (no change) */
};
```

**Complete age lifecycle**:

```
Object creation → G_NEW (white)
                    │
            ┌───────┴────────────────────────────┐
            │ survived minor collection           │ died → freed
            ▼                                     
          G_SURVIVAL (white)                      
            │                                     
            ├── survived another minor → G_OLD1 (black)
            │                                     │
            │                              survived → G_OLD (black)
            │                                     │
            │                              (stays OLD forever unless touched)
            │
            └── caught in FORWARD barrier → G_OLD0 (gray/black)
                                              │
                                       next cycle → G_OLD1 → G_OLD
                                       
  Any OLD object caught in BACK barrier:
    G_OLD/G_OLD1/G_OLD0 → G_TOUCHED1 (gray, in grayagain)
                              │
                       next cycle → G_TOUCHED2 (black)
                              │
                       next cycle → G_OLD (black)
```

### 6.3 `youngcollection` — Minor Collection

(`lgc.c:1336-1394`)

This is the main entry point for generational minor collections. It runs as a complete cycle (not incremental):

```
youngcollection(L, g):
  1. MARK OLD1 OBJECTS
     If firstold1 exists:
       markold(g, firstold1, reallyold)  // mark OLD1 objects (they point to survival)
       firstold1 = NULL
     markold(g, finobj, finobjrold)      // mark OLD1 finobj
     markold(g, tobefnz, NULL)           // mark OLD1 tobefnz
  
  2. ATOMIC PHASE
     atomic(L)                           // full atomic: mark roots, propagate, clear weak, etc.
  
  3. SWEEP NURSERY (allgc → survival)
     gcstate = GCSswpallgc
     psurvival = sweepgen(L, g, &allgc, survival, &firstold1, &addedold1)
  
  4. SWEEP SURVIVAL (survival → old1)
     sweepgen(L, g, psurvival, old1, &firstold1, &addedold1)
  
  5. ADVANCE POINTERS
     reallyold = old1
     old1 = *psurvival        // survivals that survived become old1
     survival = allgc         // all new objects are now survivals
  
  6. REPEAT FOR FINOBJ LISTS
     (same sweep + pointer advancement for finobj/finobjsur/finobjold1/finobjrold)
  
  7. SWEEP TOBEFNZ
     sweepgen(L, g, &tobefnz, NULL, &dummy, &addedold1)
  
  8. UPDATE ACCOUNTING
     GCmarked = marked + addedold1
  
  9. CHECK MINOR→MAJOR SHIFT
     if checkminormajor(g):
       minor2inc(L, g, KGC_GENMAJOR)  // shift to major mode
     else:
       finishgencycle(L, g)            // stay in minor mode
```

### 6.4 `markold` — Marking OLD1 Objects

(`lgc.c:1283-1291`)

```c
static void markold (global_State *g, GCObject *from, GCObject *to) {
  GCObject *p;
  for (p = from; p != to; p = p->next) {
    if (getage(p) == G_OLD1) {
      lua_assert(!iswhite(p));
      setage(p, G_OLD);        // advance to OLD
      if (isblack(p))
        reallymarkobject(g, p); // re-mark to traverse references
    }
  }
}
```

OLD1 objects need marking because they may point to SURVIVAL objects (which need to be kept alive). After marking, they advance to G_OLD (truly old — won't be traversed again unless touched).

### 6.5 `sweepgen` — Generational Sweep

(`lgc.c:1165-1212`)

Unlike incremental `sweeplist` which resets all survivors to white/NEW, `sweepgen` advances ages:

```
For each object in [p, limit):
  if white (dead):
    assert(!isold && isdead)
    remove from list, freeobj
  else (alive):
    if G_NEW:
      clear GC bits, set G_SURVIVAL + current white
    else:
      advance age via nextage[] table
      if became G_OLD1:
        addedold += objsize(curr)
        track firstold1
```

**Key difference from incremental sweep**: NEW objects become SURVIVAL (white). All other ages advance but keep their existing color. This is crucial — old objects stay black because the generational invariant requires it.

### 6.6 `correctgraylist` — Post-Sweep Gray List Cleanup

(`lgc.c:1223-1253`)

After sweeping, the gray lists may contain stale entries (white objects that died, or TOUCHED2 objects that should advance). This function cleans them up:

```
For each object in gray list:
  if white → remove (dead object)
  if TOUCHED1 → make black, set TOUCHED2, keep in list
  if THREAD (non-white) → keep in list (threads always stay gray)
  if TOUCHED2 → advance to OLD, make black, remove
  else (other old) → make black, remove
```

`correctgraylists` (`lgc.c:1259-1266`) coalesces all gray lists (grayagain, weak, allweak, ephemeron) into one and runs `correctgraylist` on the combined list.

### 6.7 Minor→Major Shift (`minor2inc`)

(`lgc.c:1308-1316`)

When too many bytes become old (checked by `checkminormajor`), the collector shifts to major mode:

```c
static void minor2inc (lua_State *L, global_State *g, lu_byte kind) {
  g->GCmajorminor = g->GCmarked;     // save live bytes count
  g->gckind = kind;                    // KGC_GENMAJOR or KGC_INC
  g->reallyold = g->old1 = g->survival = NULL;  // clear gen pointers
  g->finobjrold = g->finobjold1 = g->finobjsur = NULL;
  entersweep(L);                       // start sweeping (clears all black objects)
  luaE_setdebt(g, applygcparam(g, STEPSIZE, 100));
}
```

This enters the incremental sweep phase. Since objects in generational mode are mostly black, the sweep will turn them all white, preparing for a full incremental collection.

### 6.8 Major→Minor Shift (`checkmajorminor` / `atomic2gen`)

After a major collection's atomic phase, `checkmajorminor` (`lgc.c:1436-1449`) checks if enough garbage was collected to justify returning to minor mode:

```c
static int checkmajorminor (lua_State *L, global_State *g) {
  if (g->gckind == KGC_GENMAJOR) {
    l_mem numbytes = gettotalbytes(g);
    l_mem addedbytes = numbytes - g->GCmajorminor;
    l_mem limit = applygcparam(g, MAJORMINOR, addedbytes);
    l_mem tobecollected = numbytes - g->GCmarked;
    if (tobecollected > limit) {
      atomic2gen(L, g);     // return to generational mode
      setminordebt(g);
      return 1;
    }
  }
  g->GCmajorminor = g->GCmarked;
  return 0;
}
```

`atomic2gen` (`lgc.c:1398-1417`) transitions back to generational mode by making all surviving objects OLD:

```c
static void atomic2gen (lua_State *L, global_State *g) {
  cleargraylists(g);
  g->gcstate = GCSswpallgc;
  sweep2old(L, &g->allgc);          // all survivors → OLD
  g->reallyold = g->old1 = g->survival = g->allgc;
  g->firstold1 = NULL;
  sweep2old(L, &g->finobj);
  g->finobjrold = g->finobjold1 = g->finobjsur = g->finobj;
  sweep2old(L, &g->tobefnz);
  g->gckind = KGC_GENMINOR;
  g->GCmajorminor = g->GCmarked;
  g->GCmarked = 0;
  finishgencycle(L, g);
}
```

### 6.9 `sweep2old` — Transitioning to Generational

(`lgc.c:1115-1136`)

Used when entering generational mode. Frees dead (white) objects and makes all survivors OLD:

```c
static void sweep2old (lua_State *L, GCObject **p) {
  GCObject *curr;
  global_State *g = G(L);
  while ((curr = *p) != NULL) {
    if (iswhite(curr)) {           // dead
      *p = curr->next;
      freeobj(L, curr);
    }
    else {                         // alive → make OLD
      setage(curr, G_OLD);
      if (curr->tt == LUA_VTHREAD) {
        lua_State *th = gco2th(curr);
        linkgclist(th, g->grayagain);  // threads stay gray
      }
      else if (curr->tt == LUA_VUPVAL && upisopen(gco2upv(curr)))
        set2gray(curr);            // open upvalues stay gray
      else
        nw2black(curr);            // everything else → black
      p = &curr->next;
    }
  }
}
```

### 6.10 `entergen` — Entering Generational Mode

(`lgc.c:1454-1461`)

```c
static void entergen (lua_State *L, global_State *g) {
  luaC_runtilstate(L, GCSpause, 1);    // finish any current cycle
  luaC_runtilstate(L, GCSpropagate, 1); // start new cycle
  atomic(L);                             // complete marking
  atomic2gen(L, g);                      // transition to gen mode
  setminordebt(g);                       // set debt for first minor cycle
}
```

### 6.11 The Generational Invariant

The generational invariant extends the tri-color invariant:

> **Old objects (G_OLD) cannot point to new objects (G_NEW).**

This is maintained through age transitions:
- When a NEW object is caught in a **forward barrier** (black old object → white new object), the new object becomes `G_OLD0` (not immediately OLD, because it may still point to other new objects).
- When an old object is caught in a **back barrier** (old object modified to point to a young object), the old object becomes `G_TOUCHED1` and goes to `grayagain` for re-traversal.

The multi-step aging (`OLD0→OLD1→OLD`) ensures that by the time an object reaches `G_OLD`, all objects it points to are also old.

### 6.12 GCmarked/GCmajorminor Multi-Purpose

(`lgc.c:1062-1082`)

These fields serve different purposes depending on the GC mode:

| Mode | `GCmarked` | `GCmajorminor` |
|------|-----------|-----------------|
| `KGC_INC` | Bytes marked during cycle | Not used |
| `KGC_GENMINOR` | Bytes that became old since last major | Bytes marked in last major |
| `KGC_GENMAJOR` | Bytes that became old since last major | Bytes marked in last major |

---

## 7. Write Barriers

### 7.1 Why Barriers?

The tri-color invariant states: **a black object must not point to a white object**. When the mutator (Lua code) writes a reference from object `o` to object `v`, this invariant may be violated if `o` is black and `v` is white. Write barriers detect and fix this violation.

There are two strategies:

### 7.2 Forward Barrier (`luaC_barrier_`)

(`lgc.c:236-252`)

"Move the collector forward" — mark the white object `v` (make it gray/black):

```c
void luaC_barrier_ (lua_State *L, GCObject *o, GCObject *v) {
  global_State *g = G(L);
  lua_assert(isblack(o) && iswhite(v) && !isdead(g, v) && !isdead(g, o));
  if (keepinvariant(g)) {           /* must keep invariant? */
    reallymarkobject(g, v);         /* restore invariant: mark v */
    if (isold(o)) {
      lua_assert(!isold(v));        /* white object can't be old */
      setage(v, G_OLD0);           /* restore generational invariant */
    }
  }
  else {  /* sweep phase */
    lua_assert(issweepphase(g));
    if (g->gckind != KGC_GENMINOR)  /* incremental mode? */
      makewhite(g, o);             /* mark 'o' as white to avoid other barriers */
  }
}
```

**Behavior by phase**:
- **During marking** (`keepinvariant`): Mark `v` immediately. In generational mode, if `o` is old, set `v` to `G_OLD0` (it can't become OLD immediately because it may point to young objects).
- **During sweep (incremental)**: Instead of marking `v`, paint `o` white. This "sweeps" `o` early — since the invariant is already broken during sweep, just make `o` consistent with the new state.
- **During sweep (generational)**: Do nothing — generational sweep doesn't distinguish white from dead by the other-white mechanism.

**Macro interface** (`lgc.h:215-220`):
```c
#define luaC_objbarrier(L,p,o) (  \
  (isblack(p) && iswhite(o)) ? \
  luaC_barrier_(L,obj2gco(p),obj2gco(o)) : cast_void(0))

#define luaC_barrier(L,p,v) (  \
  iscollectable(v) ? luaC_objbarrier(L,p,gcvalue(v)) : cast_void(0))
```

The check `isblack(p) && iswhite(o)` is done inline (fast path — usually false).

### 7.3 Back Barrier (`luaC_barrierback_`)

(`lgc.c:258-270`)

"Move the collector backward" — put the black object `o` back to gray:

```c
void luaC_barrierback_ (lua_State *L, GCObject *o) {
  global_State *g = G(L);
  lua_assert(isblack(o) && !isdead(g, o));
  lua_assert((g->gckind != KGC_GENMINOR)
          || (isold(o) && getage(o) != G_TOUCHED1));
  if (getage(o) == G_TOUCHED2)       /* already in gray list? */
    set2gray(o);                      /* make it gray to become touched1 */
  else                                /* link it in 'grayagain' and paint gray */
    linkobjgclist(o, g->grayagain);
  if (isold(o))                       /* generational mode? */
    setage(o, G_TOUCHED1);           /* touched in current cycle */
}
```

**Macro interface** (`lgc.h:222-227`):
```c
#define luaC_objbarrierback(L,p,o) (  \
  (isblack(p) && iswhite(o)) ? luaC_barrierback_(L,p) : cast_void(0))

#define luaC_barrierback(L,p,v) (  \
  iscollectable(v) ? luaC_objbarrierback(L, p, gcvalue(v)) : cast_void(0))
```

### 7.4 When to Use Which Barrier

| Barrier | Use Case | Rationale |
|---------|----------|-----------|
| **Forward** (`luaC_barrier`) | Single-reference writes: setting an upvalue, setting a userdata's metatable | Object `o` has few references; cheaper to mark `v` than re-traverse `o` |
| **Back** (`luaC_barrierback`) | Container writes: table assignments, stack pushes | Object `o` has many references; cheaper to re-traverse `o` later than mark each `v` individually |

**Specific uses in the codebase**:

| Location | Barrier Type | Why |
|----------|-------------|-----|
| Table rawset/rawseti | Back barrier | Tables are containers; many writes expected |
| Upvalue close/set | Forward barrier | Single value reference |
| Userdata set metatable | Forward barrier | Single reference |
| Closure creation (upvalue init) | Forward barrier | Each upvalue is a single reference |
| Thread stack push | No barrier needed | Threads are always gray (re-traversed in atomic) |
| Proto (during compilation) | No barrier needed | Protos are created during compilation when GC is paused |

### 7.5 The Critical Correctness Property

**Every write of a collectable reference into a GC-managed object must be followed by the appropriate barrier.** Missing a barrier can cause:

1. **In incremental mode**: A reachable object is collected (black object points to unmarked white → white object is swept as dead).
2. **In generational mode**: An old object points to a young object that gets collected in a minor cycle (generational invariant violated).

The barrier check (`isblack(p) && iswhite(o)`) is a fast-path optimization — in the common case, either `p` is not black or `o` is not white, and the barrier is a no-op.

### 7.6 TOUCHED2 Optimization

In `luaC_barrierback_`, if an object is already `G_TOUCHED2` (meaning it was touched in the previous cycle and is already in a gray list), the function just repaints it gray and sets it to `G_TOUCHED1`. It doesn't re-link it to `grayagain` because it's already in a gray list. This avoids duplicate entries.

### 7.7 Barrier-Free Objects

Some objects don't need barriers:
- **Threads**: Always kept in `grayagain` (re-traversed every atomic phase). Stack modifications don't need barriers.
- **Open upvalues**: Kept gray; their values are re-traversed via `remarkupvals`.
- **Fixed objects** (`fixedgc` list): Never collected, always gray/old.
- **Strings**: Immutable after creation — no writes, no barriers needed.

---

## 8. Finalization

### 8.1 The `__gc` Metamethod Lifecycle

Objects with a `__gc` metamethod follow a special lifecycle through three lists:

```
1. Object created → allgc list (normal)
2. Metatable with __gc set → luaC_checkfinalizer moves object: allgc → finobj
3. Object becomes unreachable → separatetobefnz moves: finobj → tobefnz
4. Finalizer called (GCTM) → udata2finalize moves: tobefnz → allgc (resurrected)
5. Object becomes unreachable again → collected normally (no more __gc)
```

### 8.2 `luaC_checkfinalizer` — Registering for Finalization

(`lgc.c:1019-1039`)

Called when a metatable with `__gc` is set on an object:

```c
void luaC_checkfinalizer (lua_State *L, GCObject *o, Table *mt) {
  global_State *g = G(L);
  if (tofinalize(o) ||                    // already marked
      gfasttm(g, mt, TM_GC) == NULL ||    // no __gc
      (g->gcstp & GCSTPCLS))              // closing state
    return;
  else {
    GCObject **p;
    if (issweepphase(g)) {
      makewhite(g, o);                     // "sweep" object
      if (g->sweepgc == &o->next)          // protect sweep pointer
        g->sweepgc = sweeptolive(L, g->sweepgc);
    }
    else
      correctpointers(g, o);              // fix generational pointers
    // Find and remove 'o' from allgc list
    for (p = &g->allgc; *p != o; p = &(*p)->next) { /* empty */ }
    *p = o->next;
    // Link into finobj list
    o->next = g->finobj;
    g->finobj = o;
    l_setbit(o->marked, FINALIZEDBIT);     // mark as finalizable
  }
}
```

**Key details**:
- The `FINALIZEDBIT` (bit 6) marks that the object has been registered for finalization.
- The linear search through `allgc` is O(n) — this is a known cost. It's mitigated by the fact that `luaC_checkfinalizer` is called rarely (only when setting a metatable with `__gc`).
- During sweep phase, the object is painted white (current white) to prevent the sweep from collecting it.
- `correctpointers` adjusts the generational sub-list pointers (`survival`, `old1`, `reallyold`, `firstold1`) if the removed object was one of those boundary markers.

### 8.3 `separatetobefnz` — Separating Unreachable Finalizable Objects

(`lgc.c:967-987`)

Called during the atomic phase to move unreachable objects from `finobj` to `tobefnz`:

```c
static void separatetobefnz (global_State *g, int all) {
  GCObject *curr;
  GCObject **p = &g->finobj;
  GCObject **lastnext = findlast(&g->tobefnz);
  while ((curr = *p) != g->finobjold1) {  // only traverse young finobj
    lua_assert(tofinalize(curr));
    if (!(iswhite(curr) || all))           // not dead and not 'all'?
      p = &curr->next;                     // skip
    else {
      if (curr == g->finobjsur)
        g->finobjsur = curr->next;         // fix pointer
      *p = curr->next;                     // remove from finobj
      curr->next = *lastnext;              // append to tobefnz
      *lastnext = curr;
      lastnext = &curr->next;
    }
  }
}
```

**Important optimization**: The traversal stops at `finobjold1` — old objects in `finobj` can't be white (they survived previous cycles), so they don't need checking. In incremental mode, `finobjold1` is NULL, so the whole list is traversed.

The `all` parameter is used during `luaC_freeallobjects` (state closing) to move ALL finalizable objects regardless of color.

### 8.4 `GCTM` — Calling a Finalizer

(`lgc.c:924-948`)

```c
static void GCTM (lua_State *L) {
  global_State *g = G(L);
  const TValue *tm;
  TValue v;
  lua_assert(!g->gcemergency);
  setgcovalue(L, &v, udata2finalize(g));   // move from tobefnz → allgc
  tm = luaT_gettmbyobj(L, &v, TM_GC);
  if (!notm(tm)) {
    TStatus status;
    lu_byte oldah = L->allowhook;
    lu_byte oldgcstp = g->gcstp;
    g->gcstp |= GCSTPGC;                   // prevent GC during finalizer
    L->allowhook = 0;                       // stop debug hooks
    setobj2s(L, L->top.p++, tm);            // push finalizer
    setobj2s(L, L->top.p++, &v);            // push object
    L->ci->callstatus |= CIST_FIN;
    status = luaD_pcall(L, dothecall, NULL, savestack(L, L->top.p - 2), 0);
    L->ci->callstatus &= ~CIST_FIN;
    L->allowhook = oldah;
    g->gcstp = oldgcstp;
    if (l_unlikely(status != LUA_OK)) {
      luaE_warnerror(L, "__gc");            // error in finalizer → warning
      L->top.p--;                           // pop error object
    }
  }
}
```

**Critical safety measures**:
1. GC is stopped during finalizer execution (`GCSTPGC`) to prevent reentrance.
2. Debug hooks are disabled.
3. Errors in finalizers are caught by `luaD_pcall` and converted to warnings (not propagated).
4. The `CIST_FIN` flag marks the call as a finalizer call.

### 8.5 `udata2finalize` — Resurrection Mechanics

(`lgc.c:903-916`)

```c
static GCObject *udata2finalize (global_State *g) {
  GCObject *o = g->tobefnz;               // get first element
  lua_assert(tofinalize(o));
  g->tobefnz = o->next;                   // remove from tobefnz
  o->next = g->allgc;                     // return to allgc
  g->allgc = o;
  resetbit(o->marked, FINALIZEDBIT);       // clear finalized bit
  if (issweepphase(g))
    makewhite(g, o);                       // "sweep" it
  else if (getage(o) == G_OLD1)
    g->firstold1 = o;                      // track in generational mode
  return o;
}
```

**After finalization**: The object is back in `allgc` with `FINALIZEDBIT` cleared. If it becomes unreachable again, it will be collected normally (no second finalization). This is the "call `__gc` at most once" guarantee.

### 8.6 `callallpendingfinalizers`

(`lgc.c:953-957`)

Simply loops calling `GCTM` until `tobefnz` is empty. Used in `finishgencycle` (generational mode) and `luaC_freeallobjects` (state closing).

### 8.7 `luaC_fix` — Making Objects Permanent

(`lgc.c:274-281`)

```c
void luaC_fix (lua_State *L, GCObject *o) {
  global_State *g = G(L);
  lua_assert(g->allgc == o);              // must be first in allgc
  set2gray(o);                             // gray forever
  setage(o, G_OLD);                        // old forever
  g->allgc = o->next;                     // remove from allgc
  o->next = g->fixedgc;                   // link to fixedgc
  g->fixedgc = o;
}
```

Used for reserved strings (keywords like `"and"`, `"or"`, etc.) and the main thread. Fixed objects are never collected.

---

## 9. lmem.c — Memory Allocation

### 9.1 Architecture

Lua's memory management is built on a single user-provided allocation function (`frealloc`) that handles all allocation, reallocation, and deallocation. All memory operations go through this function, and every allocation/deallocation updates the GC debt counter.

The allocation function signature (`lmem.c:39-49`):
```c
// void *frealloc (void *ud, void *ptr, size_t osize, size_t nsize);
// - frealloc(ud, p, x, 0)      → free block p, return NULL
// - frealloc(ud, NULL, x, s)    → allocate new block of size s
// - frealloc(ud, b, x, y)       → realloc block b from size x to size y
```

### 9.2 `luaM_realloc_` — Generic Allocation

(`lmem.c:169-180`)

```c
void *luaM_realloc_ (lua_State *L, void *block, size_t osize, size_t nsize) {
  void *newblock;
  global_State *g = G(L);
  lua_assert((osize == 0) == (block == NULL));
  newblock = firsttry(g, block, osize, nsize);
  if (l_unlikely(newblock == NULL && nsize > 0)) {
    newblock = tryagain(L, block, osize, nsize);
    if (newblock == NULL)
      return NULL;  /* do not update 'GCdebt' */
  }
  lua_assert((nsize == 0) == (newblock == NULL));
  g->GCdebt -= cast(l_mem, nsize) - cast(l_mem, osize);
  return newblock;
}
```

**GC Debt Update**: `GCdebt -= (nsize - osize)`. When allocating (nsize > osize), debt decreases (goes more negative). When freeing (nsize < osize), debt increases (goes toward zero or positive). When `GCdebt <= 0`, a GC step is triggered.

**Emergency collection** (`tryagain`, `lmem.c:157-164`): If allocation fails and the state is complete (`cantryagain`), runs a full emergency GC cycle and retries:
```c
static void *tryagain (lua_State *L, void *block, size_t osize, size_t nsize) {
  global_State *g = G(L);
  if (cantryagain(g)) {
    luaC_fullgc(L, 1);  /* try to free some memory... */
    return callfrealloc(g, block, osize, nsize);  /* try again */
  }
  else return NULL;
}
```

`cantryagain(g)` = `completestate(g) && !g->gcstopem` — can't retry during state construction or during a GC step (to prevent reentrance).

### 9.3 `luaM_saferealloc_` — Non-Failing Realloc

(`lmem.c:183-188`)

```c
void *luaM_saferealloc_ (lua_State *L, void *block, size_t osize, size_t nsize) {
  void *newblock = luaM_realloc_(L, block, osize, nsize);
  if (l_unlikely(newblock == NULL && nsize > 0))
    luaM_error(L);  /* throws LUA_ERRMEM */
  return newblock;
}
```

Used when allocation failure should throw an error (most internal allocations).

### 9.4 `luaM_malloc_` — New Allocation

(`lmem.c:191-203`)

```c
void *luaM_malloc_ (lua_State *L, size_t size, int tag) {
  if (size == 0)
    return NULL;
  else {
    global_State *g = G(L);
    void *newblock = firsttry(g, NULL, cast_sizet(tag), size);
    if (l_unlikely(newblock == NULL)) {
      newblock = tryagain(L, NULL, cast_sizet(tag), size);
      if (newblock == NULL)
        luaM_error(L);
    }
    g->GCdebt -= cast(l_mem, size);
    return newblock;
  }
}
```

**Tag parameter**: When allocating a new GC object, `tag` is the object type (passed as `osize` to `frealloc`). This allows the user-provided allocator to distinguish between object types for custom allocation strategies.

### 9.5 `luaM_free_` — Deallocation

(`lmem.c:149-154`)

```c
void luaM_free_ (lua_State *L, void *block, size_t osize) {
  global_State *g = G(L);
  lua_assert((osize == 0) == (block == NULL));
  callfrealloc(g, block, osize, 0);
  g->GCdebt += cast(l_mem, osize);  /* debt increases when freeing */
}
```

**Note**: Freeing **increases** `GCdebt` (makes it more positive / less negative). This is counterintuitive but correct: `GCdebt` tracks the "budget" — freeing gives back budget.

### 9.6 The GC Debt Mechanism

The debt mechanism is how allocation triggers GC steps:

```
GCdebt starts positive (set by setpause/setminordebt after a cycle)
    │
    ▼ Each allocation: GCdebt -= allocated_size
    │
    ▼ When GCdebt <= 0: luaC_condGC triggers luaC_step
    │
    ▼ After GC cycle: setpause sets GCdebt = threshold - totalbytes
    │
    ▼ Cycle repeats
```

The macro `luaC_condGC` (`lgc.h:209-211`):
```c
#define luaC_condGC(L,pre,pos) \
  { if (G(L)->GCdebt <= 0) { pre; luaC_step(L); pos;}; \
    condchangemem(L,pre,pos,0); }
```

**`setpause`** (`lgc.c:1093-1098`): After an incremental cycle, sets the debt so the next cycle starts when memory grows by `PAUSE`% of marked bytes:
```c
static void setpause (global_State *g) {
  l_mem threshold = applygcparam(g, PAUSE, g->GCmarked);
  l_mem debt = threshold - gettotalbytes(g);
  if (debt < 0) debt = 0;
  luaE_setdebt(g, debt);
}
```

**`setminordebt`** (`lgc.c:1420-1422`): After a minor collection, sets debt based on `MINORMUL`% of the base memory:
```c
static void setminordebt (global_State *g) {
  luaE_setdebt(g, applygcparam(g, MINORMUL, g->GCmajorminor));
}
```

### 9.7 `luaM_growaux_` — Dynamic Array Growth

(`lmem.c:100-118`)

The standard "grow vector" pattern used throughout Lua (parser arrays, stack, etc.):

```c
void *luaM_growaux_ (lua_State *L, void *block, int nelems, int *psize,
                     unsigned size_elems, int limit, const char *what) {
  void *newblock;
  int size = *psize;
  if (nelems + 1 <= size)          // still have room?
    return block;                   // nothing to do
  if (size >= limit / 2) {         // can't double?
    if (l_unlikely(size >= limit))
      luaG_runerror(L, "too many %s (limit is %d)", what, limit);
    size = limit;                   // grow to limit
  }
  else {
    size *= 2;
    if (size < MINSIZEARRAY)       // minimum size = 4
      size = MINSIZEARRAY;
  }
  lua_assert(nelems + 1 <= size && size <= limit);
  newblock = luaM_saferealloc_(L, block, cast_sizet(*psize) * size_elems,
                                         cast_sizet(size) * size_elems);
  *psize = size;
  return newblock;
}
```

**Growth strategy**: Double the size, with minimum of 4 elements. If doubling would exceed the limit, grow to the limit exactly.

### 9.8 `luaM_shrinkvector_` — Array Shrinking

(`lmem.c:125-133`)

Used after compilation to shrink parser arrays to their exact size:
```c
void *luaM_shrinkvector_ (lua_State *L, void *block, int *size,
                          int final_n, unsigned size_elem) {
  void *newblock;
  size_t oldsize = cast_sizet(*size) * size_elem;
  size_t newsize = cast_sizet(final_n) * size_elem;
  lua_assert(newsize <= oldsize);
  newblock = luaM_saferealloc_(L, block, oldsize, newsize);
  *size = final_n;
  return newblock;
}
```

### 9.9 Macro Interface (lmem.h)

| Macro | Expands To | Purpose |
|-------|-----------|---------|
| `luaM_new(L,t)` | `luaM_malloc_(L, sizeof(t), 0)` | Allocate single object |
| `luaM_newvector(L,n,t)` | `luaM_malloc_(L, n*sizeof(t), 0)` | Allocate array |
| `luaM_newobject(L,tag,s)` | `luaM_malloc_(L, s, tag)` | Allocate GC object (with type tag) |
| `luaM_free(L,b)` | `luaM_free_(L, b, sizeof(*b))` | Free single object |
| `luaM_freearray(L,b,n)` | `luaM_free_(L, b, n*sizeof(*b))` | Free array |
| `luaM_reallocvector(L,v,on,n,t)` | `luaM_realloc_(L, v, on*sizeof(t), n*sizeof(t))` | Resize array |
| `luaM_growvector(...)` | `luaM_growaux_(...)` | Grow array with doubling |
| `luaM_shrinkvector(...)` | `luaM_shrinkvector_(...)` | Shrink array to exact size |

### 9.10 `EMERGENCYGCTESTS` Mode

(`lmem.c:73-81`)

When `EMERGENCYGCTESTS` is defined, the `firsttry` function deliberately fails every allocation (except frees), forcing every allocation to go through `tryagain` → emergency GC. This tests the emergency collection path exhaustively.

---

## 10. If I Were Building This in Go

### 10.1 The Fundamental Difference

Go has its own garbage collector. A Go reimplementation of Lua does **NOT** need to reimplement the mark-and-sweep collector for managing Lua object lifetimes. Go's GC will handle that automatically — as long as Lua objects are represented as Go values (structs, slices, maps), they'll be collected when unreachable.

**What you DON'T need**:
- The tri-color marking algorithm
- The incremental state machine (GCSpause → ... → GCSpause)
- The sweep phase
- The `allgc` linked list for tracking all objects
- Write barriers for GC correctness (Go has its own)
- The two-white system
- `GCdebt` / `GCtotalbytes` for triggering collection

**What you DO need**:

### 10.2 `__gc` Metamethod Support (Finalization)

Lua's `__gc` metamethod is a **language feature**, not an implementation detail. Users expect:
1. When a userdata/table with `__gc` becomes unreachable, its `__gc` is called.
2. `__gc` is called at most once per object.
3. Finalizers run in a specific order (reverse order of creation).
4. Resurrected objects (referenced by a finalizer) stay alive until the finalizer completes.

**Go implementation approach**:
```go
// Use runtime.SetFinalizer on objects with __gc metamethods
type Userdata struct {
    // ... fields ...
    metatable *Table
    finalized bool
}

// When setting a metatable with __gc:
func setMetatable(L *LuaState, u *Userdata, mt *Table) {
    u.metatable = mt
    if mt.hasGCMetamethod() && !u.finalized {
        runtime.SetFinalizer(u, luaGCFinalizer)
    }
}

func luaGCFinalizer(u *Userdata) {
    u.finalized = true
    // Call __gc metamethod
    // Note: this runs in a separate goroutine — need careful synchronization
}
```

**Caveats**:
- `runtime.SetFinalizer` runs in an arbitrary goroutine — not in the Lua thread. You need synchronization.
- Go finalizers don't guarantee order. Lua requires reverse-creation order. You may need a finalization queue.
- Resurrection is tricky: if the finalizer stores the object somewhere reachable, Go's GC will keep it alive, but you must ensure `__gc` isn't called again.

### 10.3 Weak Tables

Weak tables are a **language feature** that must be supported. Go doesn't have native weak references (as of Go 1.22), but `weak` package was added in Go 1.24.

**Options**:
1. **Go 1.24+ `weak.Pointer`**: Use `weak.Pointer[T]` for weak references. This is the cleanest solution.
2. **Pre-1.24**: Use a polling approach — periodically scan weak tables and check if values are still reachable (expensive and imprecise).
3. **Weak keys (ephemeron tables)**: These are harder. You need to detect when a key becomes unreachable and clear the entry. `weak.Pointer` can help here too.

```go
// Weak-value table entry (Go 1.24+)
type WeakEntry struct {
    Key   Value
    Value weak.Pointer[GCObject]
}

// Reading a weak value:
func (t *WeakTable) Get(key Value) Value {
    entry := t.find(key)
    if entry == nil { return Nil }
    obj := entry.Value.Value()  // returns nil if collected
    if obj == nil {
        t.remove(key)  // clean up
        return Nil
    }
    return obj.ToValue()
}
```

### 10.4 Memory Accounting for `collectgarbage()`

Lua's `collectgarbage("count")` returns the total memory in use. Even though Go manages memory, you need to track how much memory Lua objects consume:

```go
type GCState struct {
    TotalBytes int64  // total bytes allocated for Lua objects
    // No need for GCdebt — Go handles collection timing
}

func (g *GCState) TrackAlloc(size int64) {
    atomic.AddInt64(&g.TotalBytes, size)
}

func (g *GCState) TrackFree(size int64) {
    atomic.AddInt64(&g.TotalBytes, -size)
}
```

`collectgarbage("collect")` can call `runtime.GC()`.
`collectgarbage("stop")`/`collectgarbage("restart")` can use `debug.SetGCPercent(-1)` / `debug.SetGCPercent(100)`.
`collectgarbage("incremental")` and `collectgarbage("generational")` are no-ops in Go (Go's GC is always concurrent).

### 10.5 The `luaC_fix` Equivalent

Fixed objects (reserved strings, main thread) should be stored in a global slice/map that prevents Go's GC from collecting them:

```go
type GlobalState struct {
    FixedObjects []GCObject  // prevents collection
    // ...
}
```

### 10.6 Summary: What to Implement

| C Lua Feature | Go Equivalent | Complexity |
|--------------|---------------|------------|
| allgc linked list | Not needed (Go GC tracks objects) | N/A |
| Mark phase | Not needed | N/A |
| Sweep phase | Not needed | N/A |
| Write barriers | Not needed (Go has its own) | N/A |
| Generational ages | Not needed | N/A |
| `__gc` metamethod | `runtime.SetFinalizer` + queue | Medium |
| Weak tables | `weak.Pointer` (Go 1.24+) | Medium |
| Ephemeron tables | `weak.Pointer` + custom logic | Hard |
| Memory accounting | Atomic counter | Easy |
| `collectgarbage()` API | `runtime.GC()` + counters | Easy |
| Emergency collection | Not needed (Go handles OOM) | N/A |
| String interning | `sync.Map` or similar | Easy |
| Fixed objects | Global slice | Easy |

### 10.7 The go-lua Project's Current Bugs

From `docs/gc-memory-bug-analysis.md`, the current go-lua implementation has fundamental issues:

1. **Bug 1 (P0)**: `Alloc()` returns `unsafe.Pointer(&slice[0])` — the slice becomes unreachable immediately, so Go's GC may collect the underlying array. **Root cause**: Using `unsafe.Pointer` to escape Go's type system without maintaining a reference.

2. **Bug 2 (P1)**: `Free()` is a no-op — it updates accounting but doesn't actually make the pointer unreachable.

3. **Bug 3 (P1)**: `Realloc()` leaks old memory — allocates new, copies, but old memory is never tracked.

4. **Bug 4 (P0)**: `GCCollector` may be nil due to init ordering — GC never triggers.

5. **Bug 5 (P2)**: `LinkObject()` is never called — the GC has no objects to manage.

**The fundamental design error**: The go-lua project tried to replicate C Lua's memory management model (manual allocation with `unsafe.Pointer`) instead of leveraging Go's GC. The correct approach is to use native Go types and let Go's GC handle lifetime management, only implementing the Lua-specific features (`__gc`, weak tables, memory accounting).

---

## 11. Edge Cases & Traps

### 11.1 Barrier Placement Correctness

**The Rule**: Every write of a GC-collectable reference into a GC-managed object must be followed by a barrier. The barrier must happen **after** the write, not before.

**Trap 1: Writing then reading**
```c
// WRONG: barrier before write
luaC_barrier(L, obj, value);
obj->field = value;  // ← if GC runs between barrier and write, invariant broken

// CORRECT: write then barrier
obj->field = value;
luaC_barrier(L, obj, value);
```

**Trap 2: Multiple writes without barriers**
```c
// WRONG: only one barrier for two writes
obj->field1 = value1;
obj->field2 = value2;
luaC_barrier(L, obj, value2);  // ← value1 has no barrier!

// CORRECT: barrier after each write, OR use back barrier
obj->field1 = value1;
obj->field2 = value2;
luaC_barrierback(L, obj, value1);  // back barrier covers all future reads
```

**Trap 3: Forgetting barrier for non-obvious writes**
Setting a metatable is a write. Closing an upvalue (copying value from stack to upvalue) is a write. Resizing a table (rehashing) doesn't need barriers because the values don't change — only their locations.

### 11.2 Ephemeron Tables

Ephemeron tables (weak-key tables) have the most complex GC interaction:

**The problem**: A value should be kept alive only if its key is alive. But marking a value might make another key alive (if the value is used as a key elsewhere). This creates a dependency cycle that requires iteration.

**The solution** (`convergeephemerons`, `lgc.c:755-772`): Iterate over all ephemeron tables, marking values whose keys are marked. Repeat until no new marks are produced. The `dir` flag alternates traversal direction to speed convergence on chains.

**Edge case**: An ephemeron table where key K1 maps to value V1, and V1 is used as key K2 in another ephemeron table mapping to V2, and V2 is used as key K1 in the first table. This circular dependency converges because each iteration marks at least one new object (or terminates).

**Trap**: Ephemeron semantics only apply to the hash part of a table. The array part is always strong (values in array part are always kept alive). This is noted in `traverseweakvalue` (`lgc.c:472`): `int hasclears = (h->asize > 0)` — array part is assumed to potentially have white values.

### 11.3 Weak Table Clearing During Sweep

Weak table entries are cleared in the **atomic phase**, not during sweep. This is important because:
1. During sweep, the "dead" status of objects changes as they're swept.
2. Clearing must happen based on the final reachability picture from marking.
3. After resurrection (finalization), weak tables must be re-cleared.

The two-pass clearing in `atomic` (`lgc.c:1560-1570`):
- First pass: clear based on initial reachability.
- After `separatetobefnz` + `markbeingfnz`: clear again for newly-resurrected entries.

**Trap**: The `origweak`/`origall` pointers ensure the second pass only processes tables that were added to the weak/allweak lists **after** resurrection marking. Without this, tables cleared in the first pass would be re-processed unnecessarily.

### 11.4 String Table Special Handling

Short strings are interned in a global hash table (`g->strt`). This creates special GC interactions:

**During sweep** (`freeobj`, `lgc.c:825-828`):
```c
case LUA_VSHRSTR: {
  TString *ts = gco2ts(o);
  luaS_remove(L, ts);  /* remove from hash table BEFORE freeing */
  luaM_freemem(L, ts, sizestrshr(cast_uint(ts->shrlen)));
  break;
}
```

The string must be removed from the hash table **before** its memory is freed. Otherwise, the hash table would contain a dangling pointer.

**String table resizing** (`checkSizes`, `lgc.c:880-885`): After sweep, if the string table is less than 25% full, it's shrunk by half. This happens in `GCSswpend` state.

**String cache** (`luaS_clearcache`, called in atomic phase): The string cache in `global_State` may hold references to dead strings. It's cleared at the end of the atomic phase.

### 11.5 Thread Traversal (Stack Scanning)

Threads are the most complex objects to traverse because:

1. **Stack is mutable**: The Lua stack changes constantly during execution. During incremental marking, the stack may change between the initial traversal and the atomic phase.

2. **Solution**: Threads are **always** added to `grayagain` during propagation (`lgc.c:697-698`):
   ```c
   if (isold(th) || g->gcstate == GCSpropagate)
     linkgclist(th, g->grayagain);
   ```
   This ensures they're re-traversed in the atomic phase.

3. **Dead stack slots**: In the atomic phase, stack slots above `top` are cleared to nil (`lgc.c:711-712`). This prevents dangling references from old stack frames.

4. **Stack shrinking**: Only done in atomic phase (`lgc.c:709-710`), not during incremental steps. Shrinking during incremental marking could invalidate pointers held by the mutator.

5. **Thread in `twups`**: Threads with open upvalues are tracked in the `twups` list. `remarkupvals` (`lgc.c:387-407`) handles the case where a thread dies but its upvalues' values still need marking.

### 11.6 Upvalue Traversal

Upvalues have unique GC behavior:

**Open upvalues** (pointing to a stack slot):
- Kept **gray** (not black) — they don't need barriers because their values are on the stack (which is re-scanned).
- Not in any gray list — they're tracked through their thread's `openupval` chain.
- When their thread is traversed, they're marked.
- `remarkupvals` handles the case where the thread is dying.

**Closed upvalues** (value copied from stack):
- Go directly to **black** in `reallymarkobject` (`lgc.c:336-339`).
- Their value is marked immediately.
- Need forward barriers when their value changes.

**Trap**: When an upvalue is closed (transitions from open to closed), its value is copied from the stack to the upvalue. This is a write that needs a forward barrier:
```c
// In luaF_closeupval:
setobj(L, uv->v.p, uv->u.open.value);  // copy value
luaC_barrier(L, uv, uv->v.p);           // barrier needed!
```

### 11.7 Object Creation During GC

New objects are created with the **current** white (`lgc.c:290`):
```c
o->marked = luaC_white(g);
```

This is safe because:
- During marking: new white objects may or may not be reached. If reached, they'll be marked. If not, they'll survive this cycle (current white = alive white).
- During sweep: new objects get the new white (post-flip), so they won't be swept.

**Trap**: `luaC_newobjdt` adds new objects to the **head** of `allgc` (`lgc.c:291-292`). In incremental mode, `entersweep` uses `sweeptolive` to skip past the head, ensuring new objects aren't swept. In generational mode, new objects are at the head (before `survival`), which is the "nursery."

### 11.8 Emergency Collection Reentrancy

The GC must not be reentrant. The `gcstopem` flag (`lstate.h:342`) prevents emergency collections during a GC step:

```c
// In singlestep (lgc.c:1608):
g->gcstopem = 1;    // no emergency collections while collecting
// ... do GC work ...
g->gcstopem = 0;

// In GCTM (lgc.c:935):
g->gcstp |= GCSTPGC;  // prevent GC during finalizer
```

**Trap**: Finalizers can allocate memory. If allocation triggers an emergency GC during a finalizer, and the emergency GC tries to call another finalizer, you get infinite recursion. `GCSTPGC` prevents this.

### 11.9 The `correctpointers` Function

(`lgc.c:1002-1007`)

When an object is removed from `allgc` (moved to `finobj`), the generational sub-list pointers must be adjusted:
```c
static void correctpointers (global_State *g, GCObject *o) {
  checkpointer(&g->survival, o);
  checkpointer(&g->old1, o);
  checkpointer(&g->reallyold, o);
  checkpointer(&g->firstold1, o);
}
```

If any pointer was pointing to the removed object, it's advanced to the next object. Missing this would corrupt the generational list structure.

### 11.10 `luaC_changemode` — Mode Switching

(`lgc.c:1467-1478`)

```c
void luaC_changemode (lua_State *L, int newmode) {
  global_State *g = G(L);
  if (g->gckind == KGC_GENMAJOR)    // doing major collections?
    g->gckind = KGC_INC;            // already incremental but in name
  if (newmode != g->gckind) {
    if (newmode == KGC_INC)
      minor2inc(L, g, KGC_INC);     // gen → inc
    else {
      lua_assert(newmode == KGC_GENMINOR);
      entergen(L, g);                // inc → gen
    }
  }
}
```

**Trap**: `KGC_GENMAJOR` is treated as `KGC_INC` for mode-switching purposes. The "major" mode is just an incremental collection that happens to check whether to return to minor mode.

---

## 12. Bug Pattern Guide

### 12.1 Missing Write Barriers

**Symptom**: Objects are collected while still reachable. Crash or data corruption, often intermittent and hard to reproduce.

**Pattern**:
```c
// BUG: writing a collectable reference without barrier
obj->field = newvalue;  // newvalue is a GCObject
// Missing: luaC_barrier(L, obj, newvalue) or luaC_barrierback(L, obj, newvalue)
```

**Detection**: Enable `HARDMEMTESTS` (`lgc.h:207-208`) — this forces a full GC cycle on every allocation, making barrier bugs deterministic:
```c
#define condchangemem(L,pre,pos,emg)  \
  { if (gcrunning(G(L))) { pre; luaC_fullgc(L, emg); pos; } }
```

**Common locations where barriers are forgotten**:
1. Setting a metatable on a userdata/table
2. Closing an upvalue (copying stack value to upvalue)
3. Setting a global variable through the registry
4. Any C API function that writes a collectable value into an existing object

### 12.2 Wrong Barrier Type

**Symptom**: Performance degradation (using forward barrier on containers) or correctness issues (using back barrier on single-reference objects in generational mode).

**Forward barrier on container (performance bug)**:
```c
// INEFFICIENT: forward barrier on a table that gets many writes
for (i = 0; i < n; i++) {
  table->array[i] = values[i];
  luaC_barrier(L, table, values[i]);  // marks each value individually
}
// BETTER: one back barrier after all writes
for (i = 0; i < n; i++)
  table->array[i] = values[i];
luaC_barrierback(L, table, values[0]);  // re-traverses table later
```

**Back barrier on single-reference (correctness concern in gen mode)**:
The back barrier puts the **parent** object back to gray. For a single-reference object (like an upvalue), this means the upvalue itself goes to grayagain, but the old value it pointed to might not be properly handled. Use forward barrier for upvalues.

### 12.3 Finalization Order Bugs

**Symptom**: Finalizer accesses an already-finalized object.

**Pattern**: Object A's `__gc` references object B. If B is finalized before A, A's finalizer sees a partially-destroyed B.

**How Lua handles it**: `separatetobefnz` appends objects to the **end** of `tobefnz` in list order. `GCTM` processes from the **front**. Objects created later appear earlier in `allgc` (prepended to head), so they're encountered first in `finobj` traversal and appended to `tobefnz` first. Combined with the `findlast` appending, this ensures **reverse creation order** for finalization.

**Trap in Go**: `runtime.SetFinalizer` does NOT guarantee order. If you need ordered finalization, you must implement a finalization queue that processes in the correct order.

### 12.4 Generational Age Transition Errors

**Symptom**: Old objects point to young objects that get collected, or young objects prematurely become old.

**Pattern 1: Skipping age steps**
```c
// BUG: setting an object directly to G_OLD when it might point to young objects
setage(obj, G_OLD);  // WRONG if obj has young references

// CORRECT: use G_OLD0 → G_OLD1 → G_OLD progression
setage(obj, G_OLD0);  // will become OLD1 next cycle, then OLD
```

**Pattern 2: Not handling TOUCHED objects**
```c
// BUG: forgetting to check TOUCHED1 in correctgraylist
// Results in TOUCHED1 objects never advancing to TOUCHED2 → OLD
```

**Pattern 3: Wrong age in sweepgen**
The `nextage[]` table in `sweepgen` (`lgc.c:1169-1177`) must be exactly right. Getting any entry wrong corrupts the age progression:
```c
static const lu_byte nextage[] = {
  G_SURVIVAL,   // G_NEW → G_SURVIVAL ✓
  G_OLD1,       // G_SURVIVAL → G_OLD1 ✓
  G_OLD1,       // G_OLD0 → G_OLD1 ✓
  G_OLD,        // G_OLD1 → G_OLD ✓
  G_OLD,        // G_OLD → G_OLD (no change) ✓
  G_TOUCHED1,   // G_TOUCHED1 → G_TOUCHED1 (no change) ✓
  G_TOUCHED2    // G_TOUCHED2 → G_TOUCHED2 (no change) ✓
};
```

Note: TOUCHED1 and TOUCHED2 don't advance here — they're handled by `correctgraylist` instead.

### 12.5 Resurrection Bugs

**Symptom**: Finalizer can't access the object's fields, or weak table entries disappear unexpectedly.

**Pattern**: Forgetting to re-mark after `separatetobefnz`:
```c
// In atomic():
separatetobefnz(g, 0);     // move unreachable finobj → tobefnz
// BUG: if you skip the next two lines, resurrected objects aren't marked
markbeingfnz(g);            // mark objects in tobefnz
propagateall(g);            // propagate resurrection marks
```

Without resurrection marking, the finalizer receives an object whose fields may have been cleared from weak tables or whose referenced objects have been freed.

### 12.6 Sweep Phase Barrier Bug

**Symptom**: Crash during sweep phase when a barrier is triggered.

**Pattern**: During incremental sweep, the invariant is broken (white objects can point to black). If a barrier is triggered on a black object during sweep, the forward barrier handles this by making the black object white (`makewhite`). But in generational mode, `makewhite` is NOT called during sweep:

```c
// In luaC_barrier_ (lgc.c:247-251):
else {  /* sweep phase */
  lua_assert(issweepphase(g));
  if (g->gckind != KGC_GENMINOR)  /* incremental mode? */
    makewhite(g, o);  /* only in incremental mode */
  // In generational mode: do nothing
}
```

**Why**: In generational mode, the sweep doesn't use the two-white system to distinguish dead from alive. Making an object white during generational sweep could cause it to be collected.

### 12.7 List Corruption Bugs

**Symptom**: Infinite loop during sweep or mark, or objects disappearing.

**Pattern 1: Moving object between lists without fixing pointers**
```c
// BUG: removing from allgc without fixing generational pointers
*p = o->next;  // remove from allgc
o->next = g->finobj;  // add to finobj
g->finobj = o;
// MISSING: correctpointers(g, o) — survival/old1/reallyold may point to o
```

**Pattern 2: Modifying list during traversal**
The sweep functions use `GCObject **p` (pointer to the `next` field of the previous object). This allows safe removal during traversal. But if external code modifies the list (e.g., creating new objects during sweep), the sweep pointer may become invalid. `entersweep`'s `sweeptolive` mitigates this.

### 12.8 GC Debt Accounting Errors

**Symptom**: GC runs too often or too rarely. Memory grows without bound or GC thrashes.

**Pattern**: Forgetting to update `GCdebt` after allocation/deallocation:
```c
// BUG: allocating without updating debt
void *ptr = malloc(size);  // bypasses luaM_* functions
// GCdebt not updated → GC doesn't know about this allocation

// CORRECT: always use luaM_* functions
void *ptr = luaM_malloc_(L, size, 0);  // updates GCdebt automatically
```

**In Go**: If you're tracking memory for `collectgarbage("count")`, every allocation and deallocation of Lua objects must update the counter. Missing any path leads to inaccurate reporting.

### 12.9 Emergency Collection During State Construction

**Symptom**: Crash during `lua_newstate` or early initialization.

**Pattern**: The state is not fully constructed (`completestate(g)` returns false), but an allocation triggers an emergency GC. The GC tries to traverse objects that aren't fully initialized.

**Protection**: `cantryagain(g)` checks `completestate(g)` before allowing emergency collection (`lmem.c:64`). During state construction, allocation failures return NULL instead of triggering emergency GC.

### 12.10 Summary: Barrier Placement Checklist

For any code that writes a GC-collectable reference:

| Question | Answer → Action |
|----------|-----------------|
| Is the target a table? | → Back barrier (`luaC_barrierback`) |
| Is the target a single-ref object (upvalue, userdata metatable)? | → Forward barrier (`luaC_barrier`) |
| Is the target a thread stack? | → No barrier (threads always re-traversed) |
| Is the target a string? | → N/A (strings are immutable) |
| Is the target a proto being compiled? | → No barrier (GC paused during compilation) |
| Is the value being written non-collectable (number, boolean, nil)? | → No barrier needed |
| Is the write happening during object creation (before any GC can run)? | → No barrier needed |

---

## Appendix: Function Index

| Function | File:Line | Purpose |
|----------|-----------|---------|
| `reallymarkobject` | lgc.c:326 | Mark a single object (dispatch by type) |
| `propagatemark` | lgc.c:726 | Process one gray object → black |
| `propagateall` | lgc.c:742 | Empty the gray list |
| `atomic` | lgc.c:1537 | Atomic phase (stop-the-world completion) |
| `convergeephemerons` | lgc.c:755 | Iterate ephemeron tables to fixed point |
| `sweeplist` | lgc.c:841 | Sweep objects, free dead, reset live |
| `sweepgen` | lgc.c:1165 | Generational sweep with age advancement |
| `sweep2old` | lgc.c:1115 | Transition all objects to OLD |
| `youngcollection` | lgc.c:1336 | Complete minor (young) collection |
| `atomic2gen` | lgc.c:1398 | Transition from incremental to generational |
| `minor2inc` | lgc.c:1308 | Transition from generational to incremental |
| `singlestep` | lgc.c:1607 | One incremental GC step |
| `incstep` | lgc.c:1627 | Incremental step with debt management |
| `luaC_step` | lgc.c:1655 | Main GC entry point (called on debt) |
| `luaC_fullgc` | lgc.c:1697 | Full GC cycle |
| `luaC_barrier_` | lgc.c:236 | Forward write barrier |
| `luaC_barrierback_` | lgc.c:258 | Back write barrier |
| `luaC_checkfinalizer` | lgc.c:1019 | Register object for finalization |
| `separatetobefnz` | lgc.c:967 | Move unreachable finobj → tobefnz |
| `GCTM` | lgc.c:924 | Call one finalizer |
| `luaC_newobj` | lgc.c:297 | Create new GC object |
| `luaC_fix` | lgc.c:274 | Make object permanent (never collected) |
| `luaC_changemode` | lgc.c:1467 | Switch between inc/gen modes |
| `luaM_realloc_` | lmem.c:169 | Generic reallocation |
| `luaM_malloc_` | lmem.c:191 | New allocation |
| `luaM_free_` | lmem.c:149 | Deallocation |
| `luaM_growaux_` | lmem.c:100 | Dynamic array growth |
| `luaM_shrinkvector_` | lmem.c:125 | Array shrinking |
