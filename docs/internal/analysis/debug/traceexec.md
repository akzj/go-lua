# luaG_traceexec — Line-by-Line Analysis

## Overview

`luaG_traceexec` (ldebug.c:936-978) is the **per-opcode hook dispatcher** called from the VM
fetch loop whenever `trap` is non-zero. It handles count hooks, line hooks, and yield recovery.
It is the single most bug-prone function in go-lua (5 bugs found here).

**Call site (C):** `vmfetch()` macro in lvm.c:1186-1187 — `trap = luaG_traceexec(L, pc)`
**Call site (Go):** `internal/vm/vm.go:1508` — `TraceExec(L, ci)`

**Go equivalent:** `TraceExec` in `internal/vm/do.go:478`

### Key Semantic Contract

1. Called **before** each opcode when hooks are active
2. Returns 0 to turn off trap (no hooks needed), 1 to keep trap active
3. Must save `pc` into `savedpc` so hooks can read current position
4. Must update `L->oldpc` so the next call knows where we were
5. Must correct `L->top` before calling hooks (hooks may trigger GC)

---

## C Source Line-by-Line Analysis

### Function Signature (line 936)

```c
int luaG_traceexec (lua_State *L, const Instruction *pc) {
```

- **What:** Takes the lua state and the **current** instruction pointer.
- **WHY:** `pc` points to the instruction about to execute. The VM passes its local `pc` copy.
- **Go (do.go:478):** `func TraceExec(L *stateapi.LuaState, ci *stateapi.CallInfo) bool`
- **Difference:** Go passes `ci` instead of `pc`. Go reads `ci.SavedPC` directly. C passes a raw
  pointer because `pc` is a local variable in `luaV_execute`, not yet saved to `ci->u.l.savedpc`.

### Line 937: Get current CallInfo

```c
  CallInfo *ci = L->ci;
```

- **What:** Cache the current call frame.
- **WHY:** Needed to access `savedpc`, `trap`, `callstatus`, and `top`.
- **Go (do.go:478):** Passed as parameter `ci`.

### Line 938: Read hook mask

```c
  lu_byte mask = cast_byte(L->hookmask);
```

- **What:** Snapshot the hook mask into a local variable.
- **WHY:** `hookmask` can be changed asynchronously by signals. Reading once ensures consistent
  decisions throughout the function. Without this, a race could enable line hooks after the count
  check but before the line check, causing inconsistent behavior.
- **Go (do.go:479):** `mask := L.HookMask`

### Line 939: Get the Proto

```c
  const Proto *p = ci_func(ci)->p;
```

- **What:** Get the prototype (bytecode + debug info) of the current Lua function.
- **WHY:** Needed for `pcRel`, `changedline`, `luaG_getfuncline`, and `sizecode` validation.
- **Go (do.go:482-486):** Extracts `cl.Proto` from `L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)`.
  Go adds a nil/type check that C doesn't need (C guarantees Lua function via `isLua` at call site).

### Line 940: Declare counthook

```c
  int counthook;
```

- **What:** Flag: will we fire the count hook this time?
- **Go (do.go:491):** `countHook := false`

### Lines 941-943: Early exit — no hooks needed

```c
  if (!(mask & (LUA_MASKLINE | LUA_MASKCOUNT))) {  /* no hooks? */
    ci->u.l.trap = 0;  /* don't need to stop again */
    return 0;  /* turn off 'trap' */
  }
```

- **What:** If neither line nor count hooks are enabled, clear trap and return 0.
- **WHY:** This is the **trap clearing path**. Once hooks are removed (e.g., user called
  `lua_sethook(L, NULL, 0, 0)`), the VM was still trapping because `ci->u.l.trap` was set.
  This clears it, so subsequent opcodes skip the trap check entirely. Without this, every
  opcode would call `luaG_traceexec` forever even with no hooks.
- **Go (do.go:480-481):** `if mask&(stateapi.MaskLine|stateapi.MaskCount) == 0 { return false }`
- **Difference:** Go doesn't set `ci.Trap = false` here. Instead, the VM loop checks
  `L.HookMask` directly each iteration (vm.go:1507), so there's no persistent trap flag to clear.
  **Severity: LOW** — behavioral equivalent since Go re-checks mask every iteration.

### Line 944: Advance PC ★★★ CRITICAL ★★★

```c
  pc++;  /* reference is always next instruction */
```

- **What:** Increment `pc` to point to the **next** instruction (the one after the one about to execute).
- **WHY:** This is one of the most subtle lines. In C Lua, `savedpc` always points to the **next**
  instruction to execute. The VM does `i = *(pc++)` in `vmfetch`, so after fetch, `pc` already
  points past the current instruction. But `luaG_traceexec` is called **before** `vmfetch` does
  `i = *(pc++)`, so `pc` points to the current instruction. The `pc++` here makes `savedpc`
  consistent with C Lua's convention that `savedpc` = next instruction.
  Without this: `savedpc` would be off by one, causing `pcRel` to return wrong values,
  `getcurrentline` to report wrong line numbers in hooks, and `changedline` to compare wrong PCs.
- **Go:** Not needed. Go calls `TraceExec` before `ci.SavedPC++` (vm.go:1510), and TraceExec
  reads `ci.SavedPC` which already points to the current instruction index. Go uses `npci = ci.SavedPC`
  (do.go:508) as the current instruction's PC index directly. The Go `pcRel` equivalent is just
  the index itself. **The Go convention differs: SavedPC = current instruction, not next.**

### Line 945: Save PC into CallInfo ★★★ CRITICAL ★★★

```c
  ci->u.l.savedpc = pc;  /* save 'pc' */
```

- **What:** Store the (now incremented) `pc` back to the call frame.
- **WHY:** Hooks need to read the current position. `getcurrentline(ci)` uses `savedpc` to
  compute the source line. If we don't save `pc` here, hooks would see a stale `savedpc` from
  the previous instruction, reporting the wrong line number.
  Also critical for error messages — if a hook throws an error, the error handler uses `savedpc`
  to report the source location.
- **Go:** Not needed as a separate step. Go's `ci.SavedPC` is always current because the VM
  loop reads `code[ci.SavedPC]` directly (vm.go:1499) rather than using a local pointer copy.

### Lines 946-948: Count hook check and reset

```c
  counthook = (mask & LUA_MASKCOUNT) && (--L->hookcount == 0);
  if (counthook)
    resethookcount(L);  /* reset count */
```

- **What:** If count hook is enabled, decrement `hookcount`. If it reaches zero, set `counthook`
  flag and reset count to `basehookcount`.
- **WHY:** Count hooks fire every N instructions. `hookcount` counts down from `basehookcount`.
  The reset must happen immediately (not after firing) because if the hook yields, the count
  should already be reset for when execution resumes.
- **Go (do.go:492-497):**
  ```go
  if mask&stateapi.MaskCount != 0 {
      L.HookCount--
      if L.HookCount == 0 {
          L.HookCount = L.BaseHookCount
          countHook = true
      }
  }
  ```
- **Difference:** Equivalent logic. Go uses explicit if-block instead of C's short-circuit `&&`.

### Lines 949-950: Early exit — no line hook, count not triggered

```c
  else if (!(mask & LUA_MASKLINE))
    return 1;  /* no line hook and count != 0; nothing to be done now */
```

- **What:** If count hook didn't fire AND line hook is not enabled, return early but keep trap on.
- **WHY:** We still need trap active for future count decrements. But there's nothing to do this
  iteration — count didn't reach zero and there's no line hook to check. This is a performance
  optimization: avoids the expensive `changedline` / `luaG_getfuncline` calls when only count
  hooks are active and count hasn't reached zero yet.
- **Go (do.go:499-500):** `if !countHook && mask&stateapi.MaskLine == 0 { return true }`

### Lines 951-953: CIST_HOOKYIELD recovery

```c
  if (ci->callstatus & CIST_HOOKYIELD) {  /* hook yielded last time? */
    ci->callstatus &= ~CIST_HOOKYIELD;  /* erase mark */
    return 1;  /* do not call hook again (VM yielded, so it did not move) */
  }
```

- **What:** If the previous hook call yielded (via `coroutine.yield` inside a hook), clear the
  flag and skip all hook dispatch this time.
- **WHY:** When a hook yields, the VM stops mid-instruction. When resumed, the VM re-enters
  `luaG_traceexec` for the **same** instruction. Without this guard, the hook would fire again
  for the same instruction, causing an infinite yield loop. The flag says "we already handled
  this instruction's hooks — just continue execution."
- **Go:** **NOT IMPLEMENTED.** Go's TraceExec has no `CIST_HOOKYIELD` check. go-lua doesn't
  support yielding from hooks (no coroutine yield support in hooks).
  **Severity: MEDIUM** — not a bug now, but will matter when coroutine yield-from-hook is added.

### Lines 954-955: Correct top for GC safety ★★★

```c
  if (!luaP_isIT(*(ci->u.l.savedpc - 1)))  /* top not being used? */
    L->top.p = ci->top.p;  /* correct top */
```

- **What:** If the current instruction doesn't use a variable-length result from the previous
  instruction (the "IT" = Inherit Top pattern), reset `L->top` to the frame's expected top.
- **WHY:** During execution, `L->top` may be anywhere (some opcodes push temps). Before calling
  hooks, we need `L->top` correct because hooks can trigger GC, and GC scans `base..top`.
  If top is too high, GC sees garbage. If too low, GC misses live values.
  BUT: if the previous instruction left variable results on the stack (e.g., `OP_CALL` with
  B=0), the current instruction needs those results, so top must NOT be reset.
  `luaP_isIT` checks if the instruction accepts inherited top (B operand = 0 in SETLIST, CALL, etc.).
- **Go:** **NOT IMPLEMENTED** in TraceExec. Go manages `L.Top` differently — it's always kept
  consistent because Go doesn't use the "IT" optimization the same way. The `hookDispatch`
  function (do.go:414-416) does protect the activation register: `if ci.IsLua() && L.Top < ci.Top`.
  **Severity: LOW** — Go's stack management is different enough that this isn't needed.

### Lines 956-957: Fire count hook

```c
  if (counthook)
    luaD_hook(L, LUA_HOOKCOUNT, -1, 0, 0);  /* call count hook */
```

- **What:** If count reached zero, fire the count hook.
- **WHY:** Count hook fires with event=HOOKCOUNT, line=-1 (no line info), no transfer info.
  This is done BEFORE the line hook check because: (1) count and line are independent events,
  (2) if both trigger, both should fire, (3) count fires first by convention.
- **Go (do.go:503-504):** `if countHook { hookDispatch(L, "count", -1) }`
- **Difference:** Go fires count hook BEFORE the line hook section, same as C. But Go doesn't
  pass transfer info (ftransfer/ntransfer). **Severity: LOW** — transfer info is only used
  for return hooks.

### Lines 958-968: Line hook section ★★★ CRITICAL ★★★

```c
  if (mask & LUA_MASKLINE) {
```

- **What:** Enter line hook processing only if line hooks are enabled.
- **Go (do.go:507):** `if mask&stateapi.MaskLine != 0 {`

### Line 961: Validate oldpc ★★★

```c
    /* 'L->oldpc' may be invalid; use zero in this case */
    int oldpc = (L->oldpc < p->sizecode) ? L->oldpc : 0;
```

- **What:** Read `L->oldpc` but clamp to 0 if it's out of bounds for this proto.
- **WHY:** `L->oldpc` is a global (per-thread) value. When returning from a called function,
  `rethook` sets `L->oldpc` to the caller's PC. But in some edge cases (error recovery,
  coroutine resume, first entry), `oldpc` may be stale or invalid for the current proto.
  Using an invalid index would cause array-out-of-bounds in `changedline` or `luaG_getfuncline`.
  Clamping to 0 is safe: worst case it causes one extra line hook call (line 0 != current line).
  The comment says "a wrong but valid oldpc at most causes an extra call to a line hook."
- **Go (do.go:515-520):**
  ```go
  oldpc := L.OldPC
  if oldpc < 0 || oldpc >= len(p.Code) {
      oldpc = 0
  }
  ```
- **Difference:** Go also checks `< 0` (Go ints can be negative). C's `L->oldpc` is unsigned
  (`int` but always set to non-negative values), so only the upper bound check is needed.

### Line 963: Compute npci (new PC index) ★★★

```c
    int npci = pcRel(pc, p);
```

- **What:** Convert the raw instruction pointer to a 0-based index relative to `p->code`.
  `pcRel` is defined as `(pc - p->code) - 1` (ldebug.h:14).
- **WHY:** Since `pc` was incremented on line 944, `pcRel` subtracts 1, giving us the index
  of the **current** instruction (the one about to execute). This is the PC we want to check
  for line changes. `npci` = "new PC index" = the instruction we're about to run.
- **Go (do.go:508):** `npci := ci.SavedPC` — directly uses the SavedPC which already holds
  the 0-based index of the current instruction. No pointer arithmetic needed.
- **Go also clamps (do.go:509-513):** Bounds-checks npci against `[0, len(p.Code)-1]`.
  C doesn't need this because pcRel always produces valid indices in normal operation.

### Lines 964-965: Line hook trigger condition ★★★ CRITICAL ★★★

```c
    if (npci <= oldpc ||  /* call hook when jump back (loop), */
        changedline(p, oldpc, npci)) {  /* or when enter new line */
```

- **What:** Fire line hook if EITHER:
  1. `npci <= oldpc` — we jumped backward (loop) or stayed on same instruction
  2. `changedline(p, oldpc, npci)` — we moved to a different source line
- **WHY for condition 1:** Backward jumps indicate loops. Even if the loop body is on the
  same line (e.g., `while true do end`), we want the line hook to fire each iteration.
  Without this, tight single-line loops would never trigger line hooks, making debugging
  impossible. The `<=` (not `<`) also catches the case where `npci == oldpc`, which can
  happen when an instruction re-executes (e.g., after a yield-resume on the same instruction).
- **WHY for condition 2:** Forward jumps that cross line boundaries should fire the hook.
  `changedline` is an optimization — it avoids calling `luaG_getfuncline` when possible
  by walking the lineinfo delta array.
- **WHY short-circuit:** `changedline` is expensive. The backward-jump check is O(1) and
  catches the most common case (loops). Only if we moved forward do we need `changedline`.
  Also, `changedline` requires `oldpc < npci` (see its comment "it must be after"), so the
  `npci <= oldpc` check acts as a guard.
- **Go (do.go:525):**
  ```go
  if npci <= oldpc || GetFuncLine(p, oldpc) != GetFuncLine(p, npci) {
  ```
- **CRITICAL DIFFERENCE:** Go does NOT use `changedline`. Instead it calls `GetFuncLine` twice.
  This is functionally correct but:
  1. **Performance:** `GetFuncLine` is O(n) where n = distance from baseline. `changedline` is
     O(delta) for small deltas, falling back to `GetFuncLine` only for large gaps.
  2. **Correctness:** Identical — both check if the source line changed.
  **Severity: LOW** (performance only, not correctness).

### Lines 966-967: Fire line hook

```c
      int newline = luaG_getfuncline(p, npci);
      luaD_hook(L, LUA_HOOKLINE, newline, 0, 0);  /* call line hook */
```

- **What:** Compute the source line for the current instruction and fire the line hook.
- **WHY:** The hook callback receives the line number so it can report "now executing line N".
- **Go (do.go:526-529):**
  ```go
  newline := GetFuncLine(p, npci)
  if newline >= 0 {
      hookDispatch(L, "line", newline)
  }
  ```
- **Difference:** Go adds a `newline >= 0` guard. C fires the hook even if `luaG_getfuncline`
  returns -1 (no debug info). In practice this shouldn't happen because `changedline` returns 0
  when `lineinfo == NULL`. **Severity: MINIMAL.**

### Line 969: Update oldpc ★★★

```c
    L->oldpc = npci;  /* 'pc' of last call to line hook */
```

- **What:** Save the current PC as `oldpc` for the next traceexec call.
- **WHY:** This is UNCONDITIONAL within the `mask & LUA_MASKLINE` block. Even if the line hook
  didn't fire (because the line didn't change), we update `oldpc`. This is correct because
  `oldpc` tracks "the last PC we checked", not "the last PC where we fired a hook".
  Without this update, the next call would compare against a stale `oldpc`, potentially
  missing a line change or firing spurious hooks.
- **Go (do.go:532):** `L.OldPC = npci`
- **Note:** Updated AFTER the hook fires. If the hook modifies `L->oldpc` (e.g., by calling
  another function which sets oldpc), this line overwrites it. This is intentional — the
  traceexec function owns `oldpc` for the current frame.

### Lines 970-975: Yield handling

```c
  if (L->status == LUA_YIELD) {  /* did hook yield? */
    if (counthook)
      L->hookcount = 1;  /* undo decrement to zero */
    ci->callstatus |= CIST_HOOKYIELD;  /* mark that it yielded */
    luaD_throw(L, LUA_YIELD);
  }
```

- **What:** After all hooks have fired, check if any hook yielded (set `L->status` to `LUA_YIELD`).
  If so: undo the count decrement (so the count hook fires again on resume), mark the CI so
  the next traceexec call skips hooks (lines 951-953), and throw a yield.
- **WHY for hookcount undo:** The count was decremented and reset on line 946-948. But the
  instruction didn't actually execute (we're yielding before it runs). If we don't undo, the
  count would be off by one when resumed. Setting to 1 means "fire count hook on the very
  next instruction after resume" — which is correct because the resume will re-enter traceexec
  for the same instruction.
- **WHY for CIST_HOOKYIELD:** Prevents infinite hook-yield loops. Without this flag, resume →
  traceexec → hooks fire → yield → resume → traceexec → hooks fire → yield → ...
- **Go:** **NOT IMPLEMENTED.** go-lua doesn't support yielding from hooks.
  **Severity: MEDIUM** — will need implementation when coroutine hooks are supported.

### Line 976: Return — keep trap active

```c
  return 1;  /* keep 'trap' on */
```

- **What:** Return 1 to keep the trap flag active for the next opcode.
- **WHY:** As long as any hook is enabled, we need to keep trapping. The only path that returns
  0 (clearing trap) is lines 941-943 when no hooks are enabled at all.
- **Go (do.go:535):** `return true`

---

## Go Implementation Mapping

| C Function/Line | Go Function | Go File:Line | Notes |
|---|---|---|---|
| `luaG_traceexec` (936) | `TraceExec` | do.go:478 | Main entry |
| `vmfetch` trap check (lvm.c:1186) | Hook check in loop | vm.go:1507-1509 | Different: Go checks mask+AllowHook |
| `luaG_tracecall` (lvm.c:1214) | Inline in OP_VARARGPREP | vm.go:2454-2463 | Go skips VARARGPREP via opcode check |
| `changedline` (881) | Inlined as `GetFuncLine` comparison | do.go:525 | Go uses full line lookup |
| `luaG_getfuncline` (88) | `GetFuncLine` | debug.go:47 | Direct equivalent |
| `getbaseline` (62) | `getBaseLine` | debug.go:28 | Direct equivalent |
| `currentpc` (43) | `ci.SavedPC - 1` | debug.go:73 | Inline computation |
| `getcurrentline` (100) | `getCurrentLine` | debug.go:65 | Direct equivalent |
| `luaD_hook` (447) | `hookDispatch` | do.go:395 | Different name, same role |
| `rethook` (501) | `retHook` + inline OldPC | do.go:451 + vm.go:2587 | Split across two sites |
| `luaD_hookcall` (484) | `CallHook` | do.go:457 | Direct equivalent |

---

## Difference Table

| Area | C Lua | go-lua | Severity | Notes |
|---|---|---|---|---|
| PC convention | `savedpc` = next instruction | `SavedPC` = current instruction | **HIGH** | Fundamental difference; Go increments AFTER TraceExec |
| `pc++` in traceexec | Line 944: explicit increment | Not needed | **INFO** | Consequence of PC convention difference |
| Trap flag | `ci->u.l.trap` persists, cleared in traceexec | No persistent flag; checks `HookMask` each loop | **LOW** | Behavioral equivalent |
| `changedline` optimization | Walks lineinfo deltas for small gaps | Always calls `GetFuncLine` twice | **LOW** | Performance only |
| `CIST_HOOKYIELD` | Full yield-from-hook support | Not implemented | **MEDIUM** | Needed for coroutine hook yield |
| `luaP_isIT` top correction | Resets top unless IT instruction | Not done; hookDispatch protects registers | **LOW** | Different stack management |
| `newline >= 0` guard | Fires hook even if line = -1 | Skips hook if line < 0 | **MINIMAL** | Unreachable in practice |
| OldPC restore on return | In `rethook` (ldo.c:518) | Split: PosCall (do.go:379) + vm.go:2587 | **MEDIUM** | Two restore sites in Go vs one in C |
| Count hook yield undo | `L->hookcount = 1` | Not implemented | **MEDIUM** | Needed for yield support |

---

## OldPC Lifecycle Summary

| Event | C Lua | go-lua | File:Line |
|---|---|---|---|
| New function call | `luaD_hookcall`: `L->oldpc = 0` | `CallHook`: `L.OldPC = 0` | ldo.c:485 / do.go:458 |
| OP_VARARGPREP | `L->oldpc = 1` | `L.OldPC = 1` | lvm.c:1959 / vm.go:2462 |
| Each traceexec | `L->oldpc = npci` | `L.OldPC = npci` | ldebug.c:969 / do.go:532 |
| Function return | `rethook`: `L->oldpc = pcRel(ci->savedpc)` | PosCall: `L.OldPC = prev.SavedPC - 1` | ldo.c:518 / do.go:379 |
| Return in VM loop | (handled by rethook above) | `L.OldPC = ci.SavedPC - 1` | — / vm.go:2587 |

---

## Verification Methods

1. **Line hook accuracy:** Set line hook, run multi-line function, verify hook fires exactly
   once per new source line and on every backward jump.
2. **Count hook accuracy:** Set count hook with N=5, run 20-instruction function, verify hook
   fires exactly 4 times.
3. **OldPC consistency:** After each return, verify `L.OldPC` equals caller's current PC.
   Test with nested calls 3+ levels deep.
4. **No-hook performance:** Set hooks, then clear them. Verify TraceExec returns false and
   subsequent opcodes don't call TraceExec (check via count).
5. **Edge case — first instruction:** New function with OldPC=0, npci=0. Verify `npci <= oldpc`
   triggers line hook for the first line.
6. **Edge case — tight loop:** `while true do end` on single line. Verify line hook fires
   each iteration (backward jump triggers `npci <= oldpc`).
7. **Regression test:** All 5 historical TraceExec bugs should have test cases in the test suite.

```bash
# Verify no source modifications
cd /home/ubuntu/workspace/go-lua && git diff --stat
# Check file length
wc -l docs/analysis/debug/traceexec.md
```
