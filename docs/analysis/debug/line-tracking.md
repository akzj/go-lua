# Line Tracking System — changedline, currentline, luaG_getfuncline

## Overview

Lua's line tracking system maps bytecode instruction indices (PCs) to source line numbers
using a **delta-encoded** `lineinfo` array with periodic `abslineinfo` checkpoints. This
system powers line hooks, error messages, and debug.getinfo().

Three key functions form the pipeline:
- **`luaG_getfuncline`** (ldebug.c:88) — core: PC → source line number
- **`getbaseline`** (ldebug.c:62) — find the nearest absolute checkpoint
- **`changedline`** (ldebug.c:881) — optimization: did the line change between two PCs?
- **`getcurrentline`** (ldebug.c:100) — convenience: current CI → source line

**Go equivalents:**
- `GetFuncLine` — `internal/vm/api/debug.go:47`
- `getBaseLine` — `internal/vm/api/debug.go:28`
- `getCurrentLine` — `internal/vm/api/debug.go:65`
- `changedline` — **not implemented as separate function** (inlined as two `GetFuncLine` calls)

---

## The Delta Encoding Scheme

### Data Structures

**C (lobject.h):**
```c
typedef struct Proto {
  ls_byte *lineinfo;       // per-instruction line delta (int8), size = sizecode
  AbsLineInfo *abslineinfo; // periodic checkpoints, size = sizeabslineinfo
  int linedefined;          // first line of function definition
  int sizecode;             // number of instructions
  // ...
} Proto;

typedef struct AbsLineInfo {
  int pc;    // instruction index
  int line;  // absolute line number at that PC
} AbsLineInfo;
```

**Go (internal/object/api/proto.go):**
```go
type Proto struct {
    LineInfo    []int8         // per-instruction delta
    AbsLineInfo []AbsLineInfo // checkpoints
    LineDefined int           // first line
    Code        []uint32      // instructions
}
type AbsLineInfo struct {
    PC   int
    Line int
}
```

### How It Works

Each instruction `i` has a `lineinfo[i]` value (int8, range -127 to +127):
- **Normal value:** delta from previous instruction's line. `line[i] = line[i-1] + lineinfo[i]`
- **Sentinel value -128 (`ABSLINEINFO`):** marks that an absolute checkpoint exists here.
  The delta is NOT added; instead, look up the checkpoint in `abslineinfo[]`.

Checkpoints are placed every `MAXIWTHABS` (128) instructions by the compiler. They provide
O(1) random access to approximate positions, avoiding O(n) walks from the start.

**Constants:**
- `MAXIWTHABS = 128` (ldebug.h:35) → Go: `maxIWthAbs = 128` (debug.go:18)
- `ABSLINEINFO = -128` (ldebug.h:27) → Go: `absLineInfo = int8(-0x80)` (debug.go:22)

---

## C Source Analysis: getbaseline (ldebug.c:62-80)

```c
static int getbaseline (const Proto *f, int pc, int *basepc) {
```

### Line 63-65: No checkpoints or PC before first checkpoint

```c
  if (f->sizeabslineinfo == 0 || pc < f->abslineinfo[0].pc) {
    *basepc = -1;  /* start from the beginning */
    return f->linedefined;
  }
```

- **What:** If no absolute line info exists, or PC is before the first checkpoint, use the
  function's definition line as baseline and start walking from PC -1 (beginning).
- **WHY:** Small functions (< 128 instructions) may have no checkpoints. The function's
  `linedefined` is always known from the parser.

### Lines 68-69: Estimate checkpoint index

```c
    int i = pc / MAXIWTHABS - 1;  /* get an estimate */
```

- **What:** Checkpoints are placed ~every 128 instructions. `pc/128 - 1` gives a lower bound.
- **WHY:** The `-1` ensures we never overshoot. Large empty/comment sequences may cause extra
  checkpoint entries, so the estimate is conservative.

### Lines 72-73: Adjust upward

```c
    while (i + 1 < f->sizeabslineinfo && pc >= f->abslineinfo[i + 1].pc)
      i++;  /* low estimate; adjust it */
```

- **What:** Walk forward to find the last checkpoint at or before `pc`.
- **WHY:** The estimate may be too low if there are extra checkpoints. This loop is typically
  0-2 iterations.

### Lines 74-75: Return checkpoint

```c
    *basepc = f->abslineinfo[i].pc;
    return f->abslineinfo[i].line;
```

**Go equivalent (debug.go:28-42):** `getBaseLine` — structurally identical logic. Returns
`(baseline int, basepc int)` as a tuple instead of using an output pointer.

---

## C Source Analysis: luaG_getfuncline (ldebug.c:88-97)

```c
int luaG_getfuncline (const Proto *f, int pc) {
  if (f->lineinfo == NULL)  /* no debug information? */
    return -1;
```

- **What:** Return -1 if the proto was stripped of debug info.
- **Go (debug.go:48-49):** `if len(f.LineInfo) == 0 { return -1 }`

```c
  else {
    int basepc;
    int baseline = getbaseline(f, pc, &basepc);
    while (basepc++ < pc) {  /* walk until given instruction */
      lua_assert(f->lineinfo[basepc] != ABSLINEINFO);
      baseline += f->lineinfo[basepc];  /* correct line */
    }
    return baseline;
  }
```

### The Walk Algorithm

1. Find the nearest checkpoint before `pc` → gives `(baseline, basepc)`
2. Walk from `basepc+1` to `pc`, adding each `lineinfo[i]` delta to `baseline`
3. The assertion confirms no ABSLINEINFO sentinels between checkpoints (they only appear
   AT checkpoint boundaries)

**Complexity:** O(distance from checkpoint) = O(MAXIWTHABS) = O(128) worst case.

**Go (debug.go:50-57):**
```go
baseline, basepc := getBaseLine(f, pc)
for basepc++; basepc <= pc; basepc++ {
    if f.LineInfo[basepc] != absLineInfo {
        baseline += int(f.LineInfo[basepc])
    }
}
return baseline
```

**Key Difference:** Go **skips** ABSLINEINFO sentinels with an if-check instead of asserting
they don't exist. This is more defensive but masks potential data corruption. C asserts
because the compiler guarantees no sentinel between checkpoints.
**Severity: LOW** — defensive coding, functionally equivalent for valid protos.

---

## C Source Analysis: getcurrentline (ldebug.c:100-102)

```c
static int getcurrentline (CallInfo *ci) {
  return luaG_getfuncline(ci_func(ci)->p, currentpc(ci));
}
```

Where `currentpc` (line 43) is:
```c
static int currentpc (CallInfo *ci) {
  return pcRel(ci->u.l.savedpc, ci_func(ci)->p);
}
```

And `pcRel` (ldebug.h:14) is:
```c
#define pcRel(pc, p)  (cast_int((pc) - (p)->code) - 1)
```

- **What:** `savedpc` points to the NEXT instruction. `pcRel` subtracts 1 to get the CURRENT
  instruction's index. Then `luaG_getfuncline` maps that to a source line.
- **Go (debug.go:65-76):** `getCurrentLine` does `pc := ci.SavedPC - 1` (same -1 adjustment
  because Go's SavedPC has already been incremented past the current instruction during execution).

---

## C Source Analysis: changedline (ldebug.c:881-898)

### Comment block (lines 874-880)

```
Check whether new instruction 'newpc' is in a different line from
previous instruction 'oldpc'. More often than not, 'newpc' is only
one or a few instructions after 'oldpc' (it must be after, see caller),
so try to avoid calling 'luaG_getfuncline'. If they are too far apart,
there is a good chance of a ABSLINEINFO in the way, so it goes directly
to 'luaG_getfuncline'.
```

**Key insight:** `changedline` is a **performance optimization**. It answers "did the line
change?" without computing the actual line numbers when possible.

### Line 881-882: No debug info

```c
static int changedline (const Proto *p, int oldpc, int newpc) {
  if (p->lineinfo == NULL)  /* no debug information? */
    return 0;
```

- **WHY:** No lineinfo = no line changes to report. Return false.

### Lines 883-892: Fast path — walk deltas

```c
  if (newpc - oldpc < MAXIWTHABS / 2) {  /* not too far apart? */
    int delta = 0;  /* line difference */
    int pc = oldpc;
    for (;;) {
      int lineinfo = p->lineinfo[++pc];
      if (lineinfo == ABSLINEINFO)
        break;  /* cannot compute delta; fall through */
      delta += lineinfo;
      if (pc == newpc)
        return (delta != 0);  /* delta computed successfully */
    }
  }
```

- **What:** If `newpc` is within 64 instructions of `oldpc`, walk the lineinfo deltas from
  `oldpc+1` to `newpc`. If the accumulated delta is non-zero, the line changed.
- **WHY `MAXIWTHABS / 2`:** If they're close, walking deltas is O(n) with small n and avoids
  two full `luaG_getfuncline` calls. The threshold of 64 is a heuristic — beyond that, the
  chance of hitting an ABSLINEINFO sentinel increases, and the walk may not complete.
- **WHY break on ABSLINEINFO:** The sentinel means the delta encoding resets here. We can't
  simply add it — fall through to the slow path.
- **WHY `delta != 0`:** If all deltas sum to zero, we're on the same line (multiple instructions
  on one line). Return false — no line hook needed.

### Lines 896-898: Slow path — full lookup

```c
  return (luaG_getfuncline(p, oldpc) != luaG_getfuncline(p, newpc));
```

- **What:** Compute both line numbers from scratch and compare.
- **WHY:** Either the PCs are far apart (> 64 instructions) or an ABSLINEINFO sentinel was
  encountered. Full lookup is always correct.

### Go: changedline is NOT a separate function

Go's `TraceExec` (do.go:525) inlines this as:
```go
if npci <= oldpc || GetFuncLine(p, oldpc) != GetFuncLine(p, npci) {
```

This always takes the "slow path" — two full `GetFuncLine` calls. The fast delta-walk
optimization is not implemented.

**Severity: LOW** — performance difference only. For most Lua programs, `GetFuncLine` is fast
enough (walks ≤128 deltas). Could matter for programs with very frequent line hooks.

---

## OldPC Lifecycle

`OldPC` tracks "the PC index we last checked for line changes." It is a **per-thread** value
(not per-CallInfo), which means it must be carefully managed across function calls and returns.

### When OldPC is Set

| Event | C Code | Go Code | Value Set |
|---|---|---|---|
| **New function call** | `luaD_hookcall` (ldo.c:485) | `CallHook` (do.go:458) | `0` |
| **OP_VARARGPREP** | lvm.c:1959 | vm.go:2462 | `1` |
| **Each traceexec** | ldebug.c:969 | do.go:532 | `npci` (current PC) |
| **Function return** | `rethook` (ldo.c:518) | PosCall (do.go:379) + vm.go:2587 | Caller's `savedpc` as index |

### Why OldPC = 0 on New Call

When entering a new function, `npci` will be 0 (first instruction). With `oldpc = 0`,
the condition `npci <= oldpc` (0 ≤ 0) is TRUE, so the line hook fires for the first
instruction. This ensures every function entry triggers a line hook.

### Why OldPC = 1 after VARARGPREP

VARARGPREP is instruction 0 for vararg functions. After it executes, the next instruction
is at PC 1. Setting `oldpc = 1` means `npci(1) <= oldpc(1)` is TRUE, triggering the line
hook for instruction 1. This is correct because the call hook already fired during
VARARGPREP processing.

### Why OldPC is Restored on Return

When function B returns to function A, B's execution has been overwriting `L->oldpc` with
B's PCs. Function A needs `oldpc` to reflect where A was when it called B (the CALL
instruction's PC). Without restoration, the line hook would compare A's current PC against
B's last PC, potentially missing a line change or firing a spurious hook.

**C (ldo.c:518):** `L->oldpc = pcRel(ci->u.l.savedpc, ci_func(ci)->p)` — uses the
**caller's** savedpc (which points to the instruction after CALL).

**Go has TWO restore sites:**
1. `PosCall` (do.go:379): `L.OldPC = prev.SavedPC - 1` — runs during normal PosCall
2. `vm.go:2587`: `L.OldPC = ci.SavedPC - 1` — runs at the `ret:` label in Execute

**Severity: MEDIUM** — Having two restore sites increases maintenance risk. C has one
canonical site (`rethook`). The Go duplication exists because PosCall handles the general
case (including C-function returns) while the vm.go site handles the fast-path return
within the VM loop.

---

## Difference Table

| Area | C Lua | go-lua | Severity | Notes |
|---|---|---|---|---|
| `changedline` function | Separate optimized function with fast delta-walk | Inlined as two `GetFuncLine` calls | **LOW** | Performance only |
| ABSLINEINFO in walk | Assert: sentinels don't appear between checkpoints | Skip with if-check (defensive) | **LOW** | Masks potential data corruption |
| `getbaseline` estimate | `pc/MAXIWTHABS - 1` with while-loop adjust | Same algorithm | **NONE** | Identical |
| `luaG_getfuncline` | Walks deltas with assert | Walks deltas with skip | **LOW** | See ABSLINEINFO note |
| `getcurrentline` | `luaG_getfuncline(p, currentpc(ci))` | `GetFuncLine(p, ci.SavedPC-1)` | **NONE** | Identical semantics |
| OldPC restore sites | Single site: `rethook` (ldo.c:518) | Two sites: PosCall + vm.go ret label | **MEDIUM** | Duplication risk |
| `changedline` caller contract | Requires `newpc > oldpc` | No such requirement (handled by `npci <= oldpc` guard) | **NONE** | Guard is in traceexec |
| lineinfo type | `ls_byte` (signed char) | `int8` | **NONE** | Identical |

---

## Verification Methods

1. **Delta encoding correctness:** Create a proto with known lineinfo deltas and abslineinfo
   checkpoints. Call `GetFuncLine` for every PC and verify against expected line numbers.

2. **Checkpoint boundary:** Test `GetFuncLine` at PC values exactly at, before, and after
   abslineinfo checkpoint boundaries (multiples of 128).

3. **No debug info:** Call `GetFuncLine` on a stripped proto (empty LineInfo). Verify returns -1.

4. **OldPC lifecycle integration:** Set a line hook, call a function that calls another function.
   Verify line hook fires correct lines on entry, during execution, and after return.

5. **changedline equivalence:** For a representative proto, compare the result of:
   - C's `changedline(p, oldpc, newpc)` for all valid (oldpc, newpc) pairs
   - Go's `GetFuncLine(p, oldpc) != GetFuncLine(p, newpc)` for the same pairs
   Verify they always agree.

6. **Large function:** Test with a function > 128 instructions to exercise abslineinfo lookups.

7. **Single-line function:** Multiple instructions on one line. Verify `changedline` returns
   false (delta sums to zero) and line hook does NOT fire between them.

```bash
# Verify no source modifications
cd /home/ubuntu/workspace/go-lua && git diff --stat
# Check file length
wc -l docs/analysis/debug/line-tracking.md
```
