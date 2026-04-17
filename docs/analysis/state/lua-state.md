# lua_State Fields — Deep Analysis

> C Lua 5.5.1 `lstate.h:284-312` → go-lua `internal/state/api/api.go:148-180`

## Overview

`lua_State` (C) / `LuaState` (go-lua) represents a single Lua thread (coroutine).
Each thread has its own stack, call chain, hook state, and error handler, but shares
the global state (`global_State` / `GlobalState`) with all other threads.

The main thread is created by `lua_newstate()` / `NewState()`. Child threads are
created by `lua_newthread()` / `NewThread()` and inherit hook settings from the parent.

## Hook System Fields

### `hookmask` — Which hooks are enabled
| | C Lua | go-lua |
|---|---|---|
| **Type** | `volatile l_signalT` (`sig_atomic_t`) | `int` |
| **Location** | `lstate.h:307` | `api.go:173` (`HookMask`) |

- **WHY**: Bitmask controlling which debug events fire. Without this, there's no way to
  selectively enable call/return/line/count hooks.
- **Bits**: `LUA_MASKCALL` (1), `LUA_MASKRET` (2), `LUA_MASKLINE` (4), `LUA_MASKCOUNT` (8).
  go-lua: `MaskCall`, `MaskRet`, `MaskLine`, `MaskCount` (`api.go:255-259`).
- **WHEN changes**: Set in `lua_sethook()` (`ldebug.c:141`). Initialized to 0
  (`lstate.c:237`). Inherited by child threads (`lstate.c:280`).
- **WHO reads**: `luaV_execute()` (`lvm.c:1208`) sets per-frame trap. `luaG_traceexec()`
  (`ldebug.c:938`). `luaD_hookcall/ret()` check specific bits.
- **WHAT breaks**: Wrong mask → hooks fire when shouldn't (perf hit) or don't fire when
  should (debugger blind).
- **C Lua note**: `volatile` + `sig_atomic_t` because `lua_sethook` can be called from
  a signal handler. go-lua uses plain `int` — Go doesn't have this concern.

### `allowhook` — Recursive hook guard
| | C Lua | go-lua |
|---|---|---|
| **Type** | `lu_byte` | `bool` |
| **Location** | `lstate.h:285` | `api.go:162` (`AllowHook`) |

- **WHY**: Prevents infinite hook recursion. When a hook executes, `allowhook = 0/false`
  so operations inside the hook don't trigger another hook.
- **WHEN changes**: Set to 0/false in `luaD_hook()` (`ldo.c:465`). Restored to 1/true
  after hook returns (`ldo.c:471`). Saved/restored via `CIST_OAH` flag across protected
  calls (`ldo.c:818`). Initialized to 1/true (`lstate.c:239`).
- **Gate check**: `luaD_hook()` (`ldo.c:450`): `if (hook && L->allowhook)` — the single
  point that prevents recursive hooks.
- **WHAT breaks**: Stuck at 0/false → no hooks ever fire. Stuck at 1/true during hook →
  infinite recursion → stack overflow.

### `oldpc` — Last hooked program counter
| | C Lua | go-lua |
|---|---|---|
| **Type** | `int` (PC offset) | `int` |
| **Location** | `lstate.h:304` | `api.go:167` (`OldPC`) |

- **WHY**: Stores the last PC where a line hook fired. `luaG_traceexec()` compares
  current PC's line to `oldpc`'s line — the hook only fires when the **line changes**.
  Without this, line hooks would fire on every instruction.
- **WHEN changes**: Set to 0 in `luaD_hookcall()` (`ldo.c:485`) — new function starts
  fresh. Updated in `luaG_traceexec()` (`ldebug.c:969`). Set in `luaD_hookret()`
  (`ldo.c:518`). Initialized to 0 (`lstate.c:243`).
- **Robustness**: `luaG_traceexec()` validates: `oldpc < p->sizecode ? oldpc : 0`
  (`ldebug.c:962`) — invalid values default to 0, preventing crashes.
- **WHAT breaks**: Wrong value → line hook fires on wrong lines or not at all.

### `basehookcount` — User-configured hook interval
| | C Lua | go-lua |
|---|---|---|
| **Type** | `int` | `int` |
| **Location** | `lstate.h:305` | `api.go:171` (`BaseHookCount`) |

- **WHY**: When `LUA_MASKCOUNT` is set, the count hook fires every `basehookcount`
  instructions. This is the "original" interval set by the user via `lua_sethook`.
- **WHEN changes**: Set in `lua_sethook()` (`ldebug.c:139`). Inherited by child threads
  (`lstate.c:281`). Initialized to 0 (`lstate.c:238`).
- **WHAT breaks**: If 0 when count mask is set → `hookcount` resets to 0 every time →
  hook fires every instruction (massive slowdown).

### `hookcount` — Countdown timer
| | C Lua | go-lua |
|---|---|---|
| **Type** | `int` | `int` |
| **Location** | `lstate.h:306` | `api.go:172` (`HookCount`) |

- **WHY**: Decremented each instruction in `luaG_traceexec()`. When it reaches 0, the
  count hook fires and `hookcount` resets to `basehookcount`.
- **WHEN changes**: Decremented (`ldebug.c:947`). Reset via `resethookcount(L)` macro
  (`ldebug.h:21`). Set to 1 in edge case when line hook yields (`ldebug.c:973`).
- **WHAT breaks**: Not decremented → count hook never fires.

### `hook` — Hook function
| | C Lua | go-lua |
|---|---|---|
| **Type** | `volatile lua_Hook` (C function pointer) | `any` (stores `TValue`) |
| **Location** | `lstate.h:300` | `api.go:170` (`Hook`) |

- **WHY**: The user-installed debug hook function. Called for enabled events.
- **go-lua difference**: Typed as `any` to avoid import cycles. Actual type is
  `objectapi.TValue`, checked in `hookDispatch()` (`do.go:336`).

## Execution State Fields

### `status` — Thread/coroutine status
| | C Lua | go-lua |
|---|---|---|
| **Type** | `TStatus` (`lu_byte`) | `int` |
| **Location** | `lstate.h:286` | `api.go:161` (`Status`) |

- **WHY**: Determines whether a coroutine can be resumed and what happened when it
  last stopped.
- **Values**: `LUA_OK` (0), `LUA_YIELD` (1), `LUA_ERRRUN` (2), `LUA_ERRSYNTAX` (3),
  `LUA_ERRMEM` (4), `LUA_ERRERR` (5). go-lua adds `StatusCloseKTop` (6) matching
  C Lua's `CLOSEKTOP`.
- **WHEN changes**: Set to `LUA_YIELD` on yield (`ldo.c:1027`). Set to error codes in
  `luaD_throw()` (`ldo.c:134`). Cleared to `LUA_OK` on resume (`ldo.c:932`).
- **WHAT breaks**: Wrong status → resume when shouldn't (undefined behavior) or can't
  resume when should (coroutine stuck).

### `nCcalls` — Dual-purpose call depth counter
| | C Lua | go-lua |
|---|---|---|
| **Type** | `l_uint32` | `uint32` |
| **Location** | `lstate.h:303` | `api.go:163` (`NCCalls`) |

- **WHY**: Packed counter preventing C stack overflow AND tracking yieldability.
- **Encoding**: Low 16 bits = C call depth. High 16 bits = non-yieldable count.
- **Macros/Methods**:
  - `getCcalls(L)` / `CCalls()` = `nCcalls & 0xFFFF`
  - `yieldable(L)` / `Yieldable()` = `(nCcalls & 0xFFFF0000) == 0`
  - `incnny(L)` = `nCcalls += 0x10000` — mark non-yieldable
  - `nyci` = `0x10001` — increment both (non-yieldable C call)
- **WHEN changes**: Incremented in `ccall()` (`ldo.c:767`). Main thread starts
  non-yieldable (`lstate.c:364` / `state.go:61`). On resume: set from caller's depth
  (`ldo.c:986`).
- **Limits**: `LUAI_MAXCCALLS = 200` (`ldo.h:63`). Exceeding → "C stack overflow".
- **WHAT breaks**: Low bits overflow → C stack overflow crash. High bits wrong →
  coroutine yields when it shouldn't or can't yield when it should.

### `errfunc` — Error handler stack position
| | C Lua | go-lua |
|---|---|---|
| **Type** | `ptrdiff_t` (stack byte offset) | `int` (stack index) |
| **Location** | `lstate.h:302` | `api.go:166` (`ErrFunc`) |

- **WHY**: Points to the current message handler function on the stack. When an error
  occurs, Lua calls this function to transform the error (e.g., add traceback).
- **WHY offset/index**: Stack can be reallocated, so raw pointers would dangle.
  C Lua uses byte offset from stack base. go-lua uses integer index.
- **WHEN changes**: Set in `luaD_pcall()` (`ldo.c:1095`). Restored after pcall
  (`ldo.c:1104`). Saved per-frame in `ci.OldErrFunc`. 0 = no handler.
- **WHAT breaks**: Wrong value → error handler is wrong function or garbage.

## Stack and Thread Fields

### `stack` — The stack array
| | C Lua | go-lua |
|---|---|---|
| **Type** | `StkIdRel` (pointer to `StackValue` array) | `[]objectapi.StackValue` |
| **Location** | `lstate.h:289` | `api.go:152` (`Stack`) |

- **WHY**: ALL values (locals, temps, args, results) live on this array. Every stack
  access is relative to this base.
- See `stack-layout.md` for full details on growth, reallocation, and StkIdRel.

### `top` — First free stack slot
| | C Lua | go-lua |
|---|---|---|
| **Type** | `StkIdRel` | `int` |
| **Location** | `lstate.h:287` | `api.go:153` (`Top`) |

- **WHY**: The "stack pointer" — API functions push/pop relative to this. NOT the same
  as `ci.Top` (frame ceiling).
- **WHEN changes**: Every push increments, every pop decrements. Adjusted on call/return.

### `stack_last` — Usable stack limit
| | C Lua | go-lua |
|---|---|---|
| **Type** | `StkIdRel` (stored field) | Computed: `StackLast()` method |
| **Location** | `lstate.h:288` | `api.go:188-190` |

- **go-lua difference**: Not stored — computed as `len(Stack) - ExtraStack`. Natural
  for Go slices where `len()` is always available.

### `ci` / `base_ci` — Call chain
| | C Lua | go-lua |
|---|---|---|
| **Type** | `CallInfo *` / `CallInfo` (embedded) | `*CallInfo` / `CallInfo` (embedded) |
| **Location** | `lstate.h` | `api.go:154-155` (`CI` / `BaseCI`) |

- `ci` points to current active CallInfo. `base_ci` is the embedded root node.
- See `callinfo.md` for full details.

### `tbclist` — To-be-closed variable chain
| | C Lua | go-lua |
|---|---|---|
| **Type** | `StkIdRel` | `int` (-1 = none) |
| **Location** | `lstate.h:291` | `api.go:159` (`TBCList`) |

- **WHY**: Head of linked list of TBC (`<close>`) variables embedded in the stack.
  When scope closes, `__close` metamethods are called in reverse order.
- See `stack-layout.md` for TBC list mechanics.

### `openupval` — Open upvalue list
| | C Lua | go-lua |
|---|---|---|
| **Type** | `UpVal *` | `any` (actual: `*closure.UpVal`) |
| **Location** | `lstate.h:290` | `api.go:158` (`OpenUpval`) |

- **WHY**: Head of linked list of upvalues still pointing into this thread's stack.
  When scope closes, open upvalues must be "closed" — value copied from stack into
  the upvalue itself.
- **go-lua note**: Typed as `any` to avoid import cycle between `state` and `closure`
  packages.

### `errorJmp` — Error recovery point
| | C Lua | go-lua |
|---|---|---|
| **Type** | `struct lua_longjmp *` | N/A (Go uses `panic`/`recover`) |
| **Location** | `lstate.h:294` | — |

- **WHY**: C Lua uses `setjmp`/`longjmp` for error recovery. Each protected call pushes
  a new recovery point. go-lua uses Go's `panic`/`recover` instead — see `LuaError`,
  `LuaYield` types (`api.go:224-250`).

### Transfer info fields
| | C Lua | go-lua |
|---|---|---|
| **Type** | `struct { ftransfer, ntransfer }` | `FTransfer int`, `NTransfer int` |
| **Location** | `lstate.h:308-311` | `api.go:174-175` |

- **WHY**: Communicates transferred value positions to call/return hooks. `ftransfer` =
  offset of first transferred value from `ci.Func`. `ntransfer` = count.

## GC-Related Fields (C Lua only)

These fields exist in C Lua but are **absent** in go-lua because Go's garbage collector
handles object traversal, marking, and finalization.

| C Lua Field | Location | Purpose | go-lua Status |
|-------------|----------|---------|---------------|
| `gclist` | `lstate.h:292` | Links thread into GC gray lists | ❌ Missing — Go GC |
| `twups` | `lstate.h:293` | "Threads with upvalues" list for GC | ❌ Missing — Go GC |
| `marked` | CommonHeader | GC mark bits (white/gray/black) | ❌ Missing — Go GC |

**Why `twups` matters in C Lua**: The GC needs to find all threads with open upvalues
to mark upvalue values. Without the `twups` list, it would scan ALL threads. Sentinel:
`L->twups == L` means "not in list" (`lfunc.h:22`).

**Why go-lua doesn't need these**: Go's GC traces all reachable objects automatically.
No manual gray lists, no mark bits, no special thread-with-upvalues tracking needed.

## Difference Table

| Field | C Lua | go-lua | Severity |
|-------|-------|--------|----------|
| `hookmask` | `volatile sig_atomic_t` | `int` | 🟢 Trivial (no signal safety needed) |
| `allowhook` | `lu_byte` | `bool` | 🟢 Trivial (type difference only) |
| `oldpc` | `int` | `int` | 🟢 Identical |
| `basehookcount` | `int` | `int` | 🟢 Identical |
| `hookcount` | `int` | `int` | 🟢 Identical |
| `hook` | `volatile lua_Hook` (C func ptr) | `any` (TValue) | 🟡 Medium (type erasure) |
| `status` | `lu_byte` | `int` | 🟢 Trivial |
| `nCcalls` | `l_uint32` (dual-encoded) | `uint32` (dual-encoded) | 🟢 Identical encoding |
| `errfunc` | `ptrdiff_t` (byte offset) | `int` (stack index) | 🟡 Medium (offset vs index) |
| `stack` | `StkIdRel` (ptr) | `[]StackValue` (slice) | 🔴 Structural |
| `top` | `StkIdRel` (ptr) | `int` (index) | 🔴 Structural |
| `stack_last` | `StkIdRel` (stored) | Computed method | 🟡 Medium |
| `tbclist` | `StkIdRel` (ptr) | `int` (-1 = none) | 🟡 Medium |
| `openupval` | `UpVal *` (typed) | `any` (type-erased) | 🟡 Medium |
| `errorJmp` | `lua_longjmp *` (setjmp) | N/A (panic/recover) | 🔴 Structural |
| `gclist` | Present | Missing | 🟢 Expected (Go GC) |
| `twups` | Present | Missing | 🟢 Expected (Go GC) |
| `marked` | Present | Missing | 🟢 Expected (Go GC) |
| `nci` | `int` | `int` | 🟢 Identical |
| `ftransfer/ntransfer` | Nested struct | Flat fields | 🟢 Trivial |

## Verification

```bash
# Verify LuaState struct
grep -n "type LuaState struct" internal/state/api/api.go
# Expected: line ~148

# Verify hook fields exist
grep -n "HookMask\|AllowHook\|OldPC\|BaseHookCount\|HookCount" internal/state/api/api.go

# Verify NCCalls dual encoding
grep -n "NCCalls\|Yieldable\|CCalls" internal/state/api/api.go

# Verify no gclist/twups/marked
grep -n "gclist\|twups\|marked" internal/state/api/api.go
# Expected: no matches

# Verify error handling uses panic/recover (not longjmp)
grep -rn "LuaError\|LuaYield" internal/state/api/api.go | head -5
```
