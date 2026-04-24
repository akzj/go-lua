# CallInfo Struct — Field-by-Field Analysis

> C Lua 5.5.1 `lstate.h:187-209` → go-lua `internal/state/api.go:87-108`

## Overview

CallInfo represents a single call frame in the Lua call stack. It is a **doubly-linked
list** in both C Lua and go-lua (not an array). Each node tracks the function being
called, its stack window, program counter (Lua) or continuation (C), and status flags.

**Why a linked list?** Reuses freed nodes as a cache — `next` beyond `L->ci` points to
pre-allocated nodes. `luaE_shrinkCI()` halves the cache during GC. This avoids
malloc/free on every call/return.

## Core Fields

### `func` — Function slot on stack
| | C Lua | go-lua |
|---|---|---|
| **Type** | `StkIdRel` (pointer/offset union) | `int` (stack index) |
| **Location** | `lstate.h:188` | `api.go:88` (`Func`) |

- **WHY**: Every call frame needs to know which function it's executing. The function
  value sits on the stack below the arguments.
- **WHEN changes**: Set in `prepCallInfo()` (`ldo.c:639`). For vararg functions, shifted
  forward past hidden args in `buildhiddenargs()` (`ltm.c:270`). Restored on return by
  subtracting delta (`ldo.c:692`, `lvm.c:1779`). In C Lua, converted to/from offset
  during stack realloc (`relstack`/`correctstack` in `ldo.c:264-282`).
- **WHAT breaks**: Wrong value → VM dereferences wrong closure → wrong prototype, wrong
  upvalues, wrong everything. Vararg access (`func - nextra`) reads garbage.
- **go-lua difference**: Integer index — no pointer fixup needed on stack realloc.

### `top` — Frame ceiling
| | C Lua | go-lua |
|---|---|---|
| **Type** | `StkIdRel` | `int` |
| **Location** | `lstate.h:189` | `api.go:89` (`Top`) |

- **WHY**: Maximum stack slot this frame may use. NOT `L->top` (current top of live
  values). The VM checks `ci->top <= L->stack_last` to ensure the frame fits.
- **WHEN changes**: Set in `prepCallInfo()`. Lua frames: `func + 1 + maxstacksize`
  (`ldo.c:741`). C frames: `L->top + LUA_MINSTACK` (`ldo.c:655`). Vararg: shifted
  forward with `func` (`ltm.c:271`).
- **WHAT breaks**: Too small → writes past frame into next frame's data. Too large →
  stack overflow checks pass incorrectly.

### `previous` / `next` — Linked list pointers
| | C Lua | go-lua |
|---|---|---|
| **Type** | `struct CallInfo *` | `*CallInfo` |
| **Location** | `lstate.h:190` | `api.go:90-91` (`Prev`/`Next`) |

- **WHY**: Doubly-linked list enables: (1) reusing freed nodes, (2) walking call chain
  for debug/error, (3) no reallocation on call/return.
- **Cache behavior**: Nodes beyond `L->ci` are a free list. `luaE_extendCI()`
  (`lstate.c:73-78`) allocates new; `luaE_shrinkCI()` (`lstate.c:96-112`) frees every
  other unused node during GC.
- **go-lua**: Same design. `NewCI()` (`state.go:213-227`), `FreeCI()` (`state.go:231-240`),
  `ShrinkCI()` (`state.go:244-261`).

### `base` — NOT a stored field
- **Critical**: Neither C Lua nor go-lua stores `base` in CallInfo. It is always
  **computed** as `ci->func + 1`. The VM keeps a local `base` variable for performance
  (`lvm.c` / `vm.go:1579`).

## Union `u` — Lua vs C Call Data

C Lua uses a C union to share memory between Lua-call and C-call fields.
go-lua **flattens all fields onto a single struct** — no union, no interface.
The active variant is determined by `callstatus & CIST_C`.

### Lua variant: `u.l` (C Lua `lstate.h:192-196`)

#### `savedpc` — Program counter
| | C Lua | go-lua |
|---|---|---|
| **Type** | `const Instruction *` | `int` (index into `Proto.Code`) |
| **Location** | `lstate.h:193` | `api.go:94` (`SavedPC`) |

- **WHY**: When the VM yields, calls a hook, or calls another function, the current PC
  must be saved so execution can resume at the right instruction.
- **WHEN changes**: Set at call start (`ldo.c:742`). Saved/restored continuously in
  `luaV_execute()` via `savepc(ci)` (`lvm.c:1144`).
- **WHAT breaks**: Wrong PC → executing wrong instruction → stack corruption or crash.

#### `trap` — Debug hook signal
| | C Lua | go-lua |
|---|---|---|
| **Type** | `volatile l_signalT` (`sig_atomic_t`) | `bool` |
| **Location** | `lstate.h:194` | `api.go:95` (`Trap`) |

- **WHY**: When nonzero, VM checks for hooks at every instruction. `volatile` +
  `sig_atomic_t` in C because `lua_sethook` can be called from a signal handler.
- **WHEN changes**: Set to 1 on stack realloc (`ldo.c:289`), hook install, debug events.
  Cleared in `luaE_extendCI()` (`lstate.c:80`).
- **go-lua difference**: Plain `bool` — Go doesn't need signal safety for hooks.

#### `nextraargs` — Vararg extra count
| | C Lua | go-lua |
|---|---|---|
| **Type** | `int` | `int` |
| **Location** | `lstate.h:195` | `api.go:96` (`NExtraArgs`) |

- **WHY**: For vararg functions, stores count of extra arguments beyond fixed parameters.
  Extra args are hidden below `ci->func` on the stack.
- **WHEN changes**: Set in `buildhiddenargs()` (`ltm.c:259`). Read by `OP_VARARG`,
  `OP_RETURN`, and `luaT_getvararg()`.
- **WHAT breaks**: Wrong count → vararg access reads wrong slots; return corrupts caller.

### C variant: `u.c` (C Lua `lstate.h:197-201`)

#### `k` — Continuation function
| | C Lua | go-lua |
|---|---|---|
| **Type** | `lua_KFunction` | `KFunction` |
| **Location** | `lstate.h:198` | `api.go:99` (`K`) |

- **WHY**: When a C function yields via `lua_yieldk`, `k` is the function that resumes
  execution. Without this, C functions couldn't yield.

#### `old_errfunc` — Saved error handler
| | C Lua | go-lua |
|---|---|---|
| **Type** | `ptrdiff_t` (stack offset) | `int` (stack index) |
| **Location** | `lstate.h:199` | `api.go:100` (`OldErrFunc`) |

- **WHY**: Protected calls (`lua_pcallk`) install a new error handler. This saves the
  previous one for restoration on completion/error.

#### `ctx` — Continuation context
| | C Lua | go-lua |
|---|---|---|
| **Type** | `lua_KContext` | `int` |
| **Location** | `lstate.h:200` | `api.go:101` (`Ctx`) |

- **WHY**: Opaque value passed to continuation `k`. Since C locals are lost on yield,
  this carries state across the yield boundary.

## Union `u2` — Multiplexed Per-Phase Data

C Lua reuses memory for three mutually exclusive values. go-lua stores all three.

| Field | C Lua | go-lua | When valid |
|---|---|---|---|
| `funcidx` | `lstate.h:204` | `api.go:106` (`FuncIdx`) | During `CIST_YPCALL` |
| `nyield` | `lstate.h:205` | `api.go:104` (`NYield`) | Between yield and resume |
| `nres` | `lstate.h:206` | `api.go:105` (`NRes`) | During `CIST_CLSRET` |

## `callstatus` Flags

`callstatus` is a `uint32` bitfield encoding orthogonal properties of the call frame.
Both C Lua and go-lua use identical bit positions.

| Bits | C Lua | go-lua | Purpose |
|------|-------|--------|---------|
| 0-7 | `CIST_NRESULTS` | `CISTNResults` (`0xFF`) | Expected results + 1 (0 = MULTRET) |
| 8-11 | `CIST_CCMT` | `CISTCCMTShift=8` | `__call` metamethod depth (max 15) |
| 12-14 | `CIST_RECST` | `CISTRecstShift=12` | Recovery status for TBC close |
| 15 | `CIST_C` (`1<<15`) | `CISTC` | C function — **most critical flag** |
| 16 | `CIST_FRESH` (`1<<16`) | `CISTFresh` | Fresh `luaV_execute` frame |
| 17 | `CIST_CLSRET` (`1<<17`) | `CISTClsRet` | Closing TBC vars during return |
| 18 | `CIST_TBC` (`1<<18`) | `CISTTBC` | Frame has TBC variables |
| 19 | `CIST_OAH` (`1<<19`) | `CISTOAH` | Saved `allowhook` value |
| 20 | `CIST_HOOKED` (`1<<20`) | `CISTHooked` | Running inside debug hook |
| 21 | `CIST_YPCALL` (`1<<21`) | `CISTYPCall` | Yieldable protected call |
| 22 | `CIST_TAIL` (`1<<22`) | `CISTTail` | Tail call |
| 23 | `CIST_HOOKYIELD` (`1<<23`) | `CISTHookYield` | Last hook yielded |
| 24 | `CIST_FIN` (`1<<24`) | `CISTFin` | Finalizer (`__gc`) call |

**Helper methods** (go-lua `api.go:112-142`):
- `IsLua()` → `CallStatus & CISTC == 0`
- `NResults()` → extracts bits 0-7
- `SetNResults()` → encodes into bits 0-7
- `GetRecst()` / `SetRecst()` → bits 12-14

## Difference Table

| Aspect | C Lua | go-lua | Severity |
|--------|-------|--------|----------|
| Stack pointers | `StkIdRel` (ptr/offset union) | `int` index | 🔴 Structural |
| Union `u` | C union (Lua/C share memory) | Flat struct (all fields present) | 🟡 Minor |
| Union `u2` | C union (funcidx/nyield/nres) | Flat struct | 🟡 Minor |
| `savedpc` type | `const Instruction *` (pointer) | `int` (index into Code) | 🟡 Medium |
| `trap` type | `volatile sig_atomic_t` | `bool` | 🟢 Trivial |
| `nresults` storage | Encoded in callstatus bits 0-7 | Same encoding | 🟢 Identical |
| `base` field | Not stored (computed) | Not stored (computed) | 🟢 Identical |
| Linked list | Doubly-linked with cache | Doubly-linked with cache | 🟢 Identical |
| Flag bit layout | `lstate.h:216-248` | `api.go:46-61` | 🟢 Identical |

## Verification

```bash
# Verify CallInfo struct exists in go-lua
grep -n "type CallInfo struct" internal/state/api.go
# Verify linked list (Prev/Next fields)
grep -n "Prev\|Next" internal/state/api.go | head -5
# Verify callstatus flags match
grep -n "CIST" internal/state/api.go | head -20
# Verify no base field in CallInfo
grep -n "Base " internal/state/api.go
```
