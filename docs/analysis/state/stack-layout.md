# Stack Layout — StkIdRel, Frame Geometry, Growth

> C Lua 5.5.1 `lstate.h`, `ldo.c`, `ldo.h` → go-lua `internal/state/api/`, `internal/vm/api/do.go`

## The Fundamental Difference: Pointers vs Indices

C Lua uses raw pointers (`StkId = StackValue *`) into a `realloc`-able array. Since
`realloc` can move the array, all pointers become dangling. C Lua solves this with
`StkIdRel` — a union that stores either a pointer (normal use) or a byte offset
(during reallocation).

go-lua uses **integer indices** into a Go slice (`[]StackValue`). When the slice grows,
Go allocates a new backing array and copies — but integer indices remain valid. This
eliminates the entire `StkIdRel` machinery.

| | C Lua | go-lua |
|---|---|---|
| Stack type | `StackValue *` (C array via malloc) | `[]objectapi.StackValue` (Go slice) |
| Pointer type | `StkId` = `StackValue *` | `int` (index) |
| Realloc-safe type | `StkIdRel` (ptr/offset union) | `int` (inherently safe) |
| Pointer fixup | `relstack()`/`correctstack()` | Not needed |
| Location | `lobject.h:158-168`, `ldo.c:261-316` | `api.go:152`, `state.go:177-186` |

### StkIdRel in Detail (C Lua only)

```c
// lobject.h:162-168
typedef union {
  StkId p;           // actual pointer — normal execution
  ptrdiff_t offset;  // byte offset from L->stack.p — during realloc
} StkIdRel;

// ldo.h:48-49
#define savestack(L,pt)   (cast_charp(pt) - cast_charp(L->stack.p))  // ptr → offset
#define restorestack(L,n) cast(StkId, cast_charp(L->stack.p) + (n))  // offset → ptr
```

**Conversion flow** (strict mode, `LUAI_STRICT_ADDRESS=1`):
1. `relstack()` (`ldo.c:261-271`): Before realloc, converts ALL pointers to offsets —
   `L->top`, `L->tbclist`, all open upvalues, all CallInfo `func`/`top` fields.
2. `realloc()` executes — array may move to new address.
3. `correctstack()` (`ldo.c:278-291`): After realloc, converts ALL offsets back to
   pointers. Also sets `ci->u.l.trap = 1` for all Lua frames (forces VM to re-read).

**go-lua equivalent**: `reallocStack()` (`state.go:177-186`) — just `make` + `copy` +
nil-fill. Comment at `state.go:178`: *"Since upvalues use StackIdx (not pointers), no
upvalue fixup is needed."*

## StackValue — The Stack Slot

| | C Lua | go-lua |
|---|---|---|
| **Type** | `union StackValue` (`lobject.h:148-155`) | `struct StackValue` (`object/api/api.go:335-338`) |
| **Value** | `TValue val` | `Val TValue` |
| **TBC link** | `tbclist.delta` (`unsigned short`) | `TBCDelta uint16` |

Each stack slot holds a Lua value plus a `delta` field used to form the TBC
(to-be-closed) variable linked list embedded in the stack itself.

## Frame Geometry: func / base / top

### Regular Lua Call (non-vararg)

After `luaD_precall()` (`ldo.c:731-745`) / go-lua `Precall()` (`do.go`):

```
Stack:  ... | func | arg1 | arg2 | ... | argN | nil-pad | [registers] |
             ^      ^                                      ^
             ci.Func ci.Func+1                             ci.Top
                     (= "base")                            (= func+1+maxstacksize)
```

- **`ci.Func`**: Stack slot of the closure. Set in `prepCallInfo()`.
- **base** = `ci.Func + 1`: First argument = register 0. **Not stored** — computed
  as needed. The VM caches it in a local variable for speed.
- **`ci.Top`**: Frame ceiling = `func + 1 + Proto.maxstacksize`. The VM must not
  write beyond this.
- **`L.Top`**: First free slot after live values. May be < `ci.Top` if fewer args
  than registers. Missing args are nil-filled (`ldo.c:743`).

### C Function Call

After `precallC()` (`ldo.c:649-666`) / go-lua equivalent in `do.go`:

```
Stack:  ... | func | arg1 | ... | argN | [LUA_MINSTACK free slots] |
             ^                    ^      ^
             ci.Func              L.Top  ci.Top (= L.Top + 20)
```

- C functions get at least `LUA_MINSTACK` (20) free slots guaranteed.
- `checkstackp(L, LUA_MINSTACK, func)` (`ldo.c:653`) ensures space exists.
- C reads args relative to `func`, pushes results onto `L.Top`.
- `CIST_C` flag is set in `callstatus`.

### Vararg Function Call (Lua 5.5 hidden args)

Lua 5.5 hides extra args below `func` rather than using a separate structure.
Handled by `buildhiddenargs()` (`ltm.c:255-271`) / go-lua `AdjustVarargs()`.

**Before** `buildhiddenargs`:
```
Stack:  ... | func | fix1 | fix2 | extra1 | extra2 |
             ^                                       ^
             ci.Func                                 L.Top
```

**After** `buildhiddenargs`:
```
Stack:  ... | func | nil  | nil  | extra1 | extra2 | func' | fix1 | fix2 | [regs] |
             ^old                                     ^new                  ^
                                                      ci.Func (shifted)    ci.Top (shifted)
```

- `ci.Func += totalargs + 1` — jumps past hidden args (`ltm.c:270`)
- `ci.Top += totalargs + 1` — frame ceiling also shifts (`ltm.c:271`)
- `ci.NExtraArgs = nextra` — count of hidden extra args (`ltm.c:259`)
- Extra args accessible at `ci.Func - nextra + index` (`ltm.c:297`)
- Original fixed params nil-ed for GC safety (`ltm.c:268`)
- On return, `ci.Func` restored by subtracting delta (`ldo.c:692`, `lvm.c:1779`)

**go-lua also supports PF_VATAB** (vararg table mode): creates a table
`{1=arg1, 2=arg2, ..., n=count}` instead of shifting the stack. No `ci.Func` change.

## Stack Growth

### Initial Size

| | C Lua | go-lua |
|---|---|---|
| Basic size | `BASIC_STACK_SIZE = 40` (`lstate.h:156`) | `BasicStackSize = 40` (`api.go:71`) |
| Extra slots | `EXTRA_STACK = 5` (`lstate.h:136`) | `ExtraStack = 5` (`api.go:74`) |
| Total alloc | 45 slots (`lstate.c:164`) | 45 slots (`state.go:71`) |
| Usable limit | `stack_last = stack + 40` (`lstate.c:169`) | `StackLast() = len(Stack) - 5` (`api.go:189`) |
| Max size | `MAXSTACK = 1,000,000` (`ldo.c:192`) | `MaxStack = 1,000,000` (`api.go:68`) |

### Growth Algorithm

**C Lua** `luaD_growstack()` (`ldo.c:361-387`):
1. New size = `currentSize * 1.5` (tentative)
2. Minimum = `L->top - L->stack + needed`
3. Cap at `MAXSTACK`
4. On overflow: allocate `ERRORSTACKSIZE = MAXSTACK + 200` for error handling space

**go-lua** has two growth paths:
1. `stateapi.GrowStack()` (`state.go:137-173`): Simple doubling. Used by `PushValue`.
2. `vmapi.GrowStack()` (`do.go:152-184`): 1.5x growth with error handling. Used by VM.

Both call `reallocStack()`: `make` new slice → `copy` old → nil-fill → assign.

### Reallocation Safety

| Step | C Lua | go-lua |
|------|-------|--------|
| 1. Save pointers | `relstack()` converts to offsets | Not needed (int indices) |
| 2. Block GC | `G(L)->gcstopem = 1` | Not needed (Go GC safe) |
| 3. Reallocate | `luaM_reallocvector()` | `make` + `copy` |
| 4. Handle failure | Restore old pointers, raise error | Go doesn't fail on `make` |
| 5. Fix pointers | `correctstack()` converts back | Not needed |
| 6. Set traps | `ci->u.l.trap = 1` for all frames | Not needed |

**Key insight**: go-lua's reallocation is ~6 lines vs C Lua's ~60 lines. The entire
`relstack`/`correctstack` machinery (pointer↔offset conversion, upvalue fixup, trap
setting) is eliminated by using integer indices.

### Shrinking

`luaD_shrinkstack()` (`ldo.c:389+`): If stack > 3× current use, shrink to 2× use.
Called by GC. `stackinuse()` computes max of all `ci->top` values and `L->top`.

go-lua: Similar logic in `ShrinkStack()`.

### Stack Check Macros (C Lua)

```c
// ldo.h:37-42
#define luaD_checkstackaux(L,n,pre,pos)  \
    if (L->stack_last.p - L->top.p <= (n))
      { pre; luaD_growstack(L, n, 1); pos; }
```

The `pre`/`pos` mechanism uses `savestack`/`restorestack` to preserve a single pointer
across potential reallocation. go-lua doesn't need this — just check and grow.

## EXTRA_STACK — The Emergency Buffer

```
Allocated:  [slot 0] ... [slot 39] [slot 40] [slot 41] [slot 42] [slot 43] [slot 44]
                         ^                                                   ^
                         stack_last (usable limit)                           true end
                         |<--- EXTRA_STACK = 5 slots --->|
```

- **WHY**: 5 slots beyond `stack_last` for temporary use — pushing TM arguments,
  error handling — without triggering a full stack check.
- **Contract**: Values pushed into EXTRA_STACK must be promptly popped. No frame
  should rely on this space.
- **WHAT breaks without it**: Every metamethod call, every error push would need a
  prior stack check — massive overhead on the hottest paths.
- **Both C Lua and go-lua**: Identical constant (5), identical purpose.

## TBC (To-Be-Closed) Variable List

The TBC list is embedded in the stack using `StackValue.delta`/`TBCDelta`:

- `L.tbclist` points to the most recent TBC variable (C: `StkIdRel`, go-lua: `int`)
- Each TBC slot's `delta` stores distance (in slots) to the previous TBC variable
- Walking: `tbc -= tbc->tbclist.delta` (`lfunc.c:219`)
- Dummy nodes inserted when gap > `MAXDELTA` (`lfunc.c:177-179`)
- go-lua: `TBCList int` field in `LuaState` (`api.go:159`), -1 = none

## Difference Table

| Aspect | C Lua | go-lua | Severity |
|--------|-------|--------|----------|
| Stack storage | `malloc`'d C array | Go slice `[]StackValue` | 🔴 Structural |
| Stack pointers | `StkIdRel` (ptr/offset union) | `int` indices | 🔴 Structural |
| Realloc fixup | `relstack()`/`correctstack()` ~60 lines | `make`+`copy` ~6 lines | 🔴 Structural |
| Upvalue fixup on realloc | Required (traverse open list) | Not needed | 🔴 Structural |
| Trap setting on realloc | Required (all Lua frames) | Not needed | 🟡 Medium |
| `stack_last` | Stored as `StkIdRel` field | Computed: `len(Stack)-ExtraStack` | 🟡 Medium |
| `tbclist` | `StkIdRel` (pointer) | `int` index (-1 = none) | 🟡 Medium |
| StackValue layout | C union with `tbclist` variant | Go struct with `TBCDelta` | 🟢 Trivial |
| EXTRA_STACK | 5 | 5 | 🟢 Identical |
| BASIC_STACK_SIZE | 40 | 40 | 🟢 Identical |
| MAXSTACK | 1,000,000 | 1,000,000 | 🟢 Identical |
| Frame geometry | func/base(computed)/top | Func/base(computed)/Top | 🟢 Identical |
| Vararg hidden args | `buildhiddenargs()` shifts func | Same + PF_VATAB table mode | 🟡 Medium |

## Verification

```bash
# Verify stack type
grep -n "Stack \[\]" internal/state/api/api.go
# Verify StackValue struct
grep -n "type StackValue struct" internal/object/api/api.go
# Verify no StkIdRel equivalent
grep -rn "StkIdRel" internal/ --include="*.go"
# Expected: no matches
# Verify EXTRA_STACK constant
grep -n "ExtraStack" internal/state/api/api.go
# Verify reallocStack simplicity
grep -n -A5 "func.*reallocStack" internal/state/api/state.go
```
