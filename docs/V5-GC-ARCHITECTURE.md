# V5 GC Architecture — Self-Managed Mark-and-Sweep in Go

## Status: ✅ Complete — All 26 Test Suites Pass

## Overview

V5 replaces Go GC delegation with a self-managed Lua GC. Every collectable Lua
object has a `GCHeader` and lives on a linked list (`allgc`). Lua GC controls
object lifecycle (mark/sweep/finalize). Go GC handles memory deallocation after
Lua GC unlinks dead objects.

## GCHeader Design

```go
// In object/api — the leaf package (no import cycles)

type GCObject interface {
    GC() *GCHeader
}

type GCHeader struct {
    Next   GCObject  // allgc/finobj/tobefnz chain
    GCList GCObject  // gray list link
    Marked byte      // GC color + age bits
}
```

Embedded in every collectable type:
- LuaString, Table, LClosure, CClosure, Userdata, LuaState, Proto, UpVal

## FullGC Phases

```
1. Flip currentwhite         (new objects get new white, survive sweep)
2. markRoots                 (registry, main thread, globals)
3. propagateAll              (drain gray list — mark reachable objects)
4. Atomic phase:
   a. Mark running thread + registry
   b. Process grayagain list (deferred weak tables)
   c. convergeEphemerons     (weak-key table convergence)
   d. clearByValues          (clear dead values from weak-value tables)
   e. separateTobeFnz        (move dead finobj → tobefnz)
   f. markBeingFnz           (resurrection — mark finalized objects)
   g. convergeEphemerons     (2nd pass for resurrected objects)
   h. clearDeadKeysAllEphemerons (TagDeadKey sentinel)
   i. clearByKeys            (clear dead keys from weak-key tables)
   j. clearByValues          (2nd pass for newly-added weak tables)
5. sweepList                 (unlink dead objects from allgc)
6. sweepList(finobj)         (sweep finobj list)
7. callAllPendingFinalizers  (run __gc via PCall)
```

## Sweep = Unlink

```go
func sweepObj(prev, curr GCObject) {
    prev.GC().Next = curr.GC().Next  // unlink from chain
    // Go GC handles deallocation (including cycles)
}
```

No nilling needed — Go's tracing GC handles cycles automatically.

## Write Barriers

~31 call sites. Go compiler inlines barrier functions.
Fast path: check two bit masks → return (3 instructions).

Forward barrier: marks white object black (used for most assignments).
Backward barrier: marks black object gray again (used for tables/threads).

## Object Creation Sites (13 total)

| Type       | Sites | Files                          |
|------------|-------|--------------------------------|
| LuaString  | 2     | luastring/api/api.go           |
| Table      | 3     | table/api, state/api, vm/api   |
| LClosure   | 3     | vm/api (OP_CLOSURE, load, undump) |
| CClosure   | 1     | api/api/impl.go                |
| Userdata   | 1     | api/api/impl.go                |
| LuaState   | 2     | state/api/state.go             |
| Proto      | 4     | vm/api/undump.go, parse/api    |
| UpVal      | 1     | closure/api/closure.go         |

All creation sites call `LinkGC()` to add the object to `allgc`.

## GlobalState GC Fields

### Object Chains
- `Allgc` — all collectable objects
- `FinObj` — objects with `__gc` (awaiting death)
- `TobeFnz` — dead objects pending finalization
- `FixedGC` — permanently pinned objects (not swept)

### Gray Lists
- `Gray` — objects to propagate
- `GrayAgain` — deferred objects (weak tables, threads)
- `Weak` — weak-value tables (for clearByValues)
- `AllWeak` — all-weak tables (for both clearing functions)
- `Ephemeron` — ephemeron tables (for convergence)

### GC State
- `GCState` — current phase (pause/propagate/atomic/sweep/callfin)
- `CurrentWhite` — which white bit is "current"
- `GCRunning` — re-entrancy guard
- `GCRunningFinalizer` — finalization re-entrancy guard
- `GCExplicit` — explicit vs periodic GC (affects traverseThread mode)

### Memory Tracking
- `GCTotalBytes` — estimated total Lua memory
- `GCDeallocDebt` — deallocation debt from runtime.AddCleanup callbacks

## Dual-Mode Thread Traversal

`traverseThread` operates in two modes:

- **Explicit GC** (`GCExplicit = true`): Uses `th.Top` — precise marking.
  Only marks stack slots up to the thread's current top. This allows weak
  tables to collect dead locals sitting in registers above Top.

- **Periodic GC** (`GCExplicit = false`): Uses `maxTop` (maximum of all
  CI.Top values). Conservative marking that won't prematurely collect values
  that are still in active call frames but above the current Top.

## Memory Deallocation

Objects are tracked with `runtime.AddCleanup` (Go 1.24+). When Go's GC
reclaims an unlinked Lua object, the cleanup callback decrements
`GCDeallocDebt`, which is applied to `GCTotalBytes` at the start of the
next Lua GC cycle.

## Key Files

| File | Purpose |
|------|---------|
| `internal/gc/gc.go` | Core GC: FullGC, mark, propagate, sweep, weak table clearing |
| `internal/api/gc_impl.go` | GCCollect, gcMarkSweep, callAllPendingFinalizers, clearStaleStack |
| `internal/state/api.go` | GlobalState struct with all GC fields |
| `internal/object/api.go` | GCHeader, GCObject interface, color constants |
