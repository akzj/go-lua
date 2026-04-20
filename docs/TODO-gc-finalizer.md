# `__gc` Finalizer Support — V5 Architecture

## Status: ✅ Complete (V5 Mark-and-Sweep)

go-lua implements `__gc` finalizers for both tables and userdata using a native
mark-and-sweep GC. The system closely mirrors C Lua's `lgc.c` architecture.

## Architecture: V5 Mark-and-Sweep Finalization

```
┌─────────────────────────────────────────────────────────────┐
│ 1. REGISTRATION (SetMetatable → CheckFinalizer)             │
│    When setmetatable(obj, mt) is called and mt has __gc:    │
│    → Object is moved from allgc to finobj list              │
│    → gc/api/gc.go:CheckFinalizer()                          │
│    ⚠ Object stays on finobj until it becomes unreachable    │
└──────────────────────┬──────────────────────────────────────┘
                       │ (FullGC mark phase finds obj unmarked)
                       ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. SEPARATION (separateTobeFnz — atomic phase)              │
│    → Scans finobj list for unmarked (dead) objects          │
│    → Moves dead objects from finobj → tobefnz list          │
│    → markBeingFnz() marks them + propagates (resurrection)  │
│    → gc/api/gc.go:separateTobeFnz()                         │
└──────────────────────┬──────────────────────────────────────┘
                       │ (after sweep completes)
                       ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. FINALIZATION (callAllPendingFinalizers)                   │
│    → Pops objects from tobefnz one at a time                │
│    → Links object back to allgc (resurrection)              │
│    → Looks up __gc metamethod on the object                 │
│    → Calls __gc(obj) via PCall (errors silently discarded)  │
│    → Repeats until tobefnz is empty                         │
│    → api/api/impl.go:callAllPendingFinalizers()             │
└─────────────────────────────────────────────────────────────┘
```

## Key Functions

| Function | File | Purpose |
|----------|------|---------|
| `CheckFinalizer` | `gc/api/gc.go:535` | Moves object from allgc → finobj when __gc is set |
| `separateTobeFnz` | `gc/api/gc.go:512` | Moves dead finobj → tobefnz during atomic phase |
| `markBeingFnz` | `gc/api/gc.go` | Marks tobefnz objects (resurrection) |
| `Udata2Finalize` | `gc/api/gc.go` | Pops one object from tobefnz, links to allgc |
| `callAllPendingFinalizers` | `api/api/impl.go:1445` | Drains tobefnz, calls __gc via PCall |
| `callOneGCTM` | `api/api/impl.go` | Calls __gc for a single object |
| `GCCollect` | `api/api/impl.go:1404` | Full GC cycle entry point (mark + sweep + finalize) |

## Design Decisions

### Why native mark-and-sweep (not `runtime.SetFinalizer`)?
The original implementation used Go's `runtime.SetFinalizer` to bridge Go GC → Lua
`__gc`. This had fundamental limitations:
- Go finalizers run in arbitrary goroutines (unsafe for Lua state)
- No deterministic ordering
- Circular references prevented collection
- Weak table clearing and finalization had no ordering guarantees

The V5 GC manages object lifecycle directly: all collectable objects live on linked
lists (`allgc`, `finobj`, `tobefnz`), and the GC controls mark/sweep/finalize phases
in the correct order.

### Finalization order
Objects are finalized in the order they appear on the `tobefnz` list, which is
reverse creation order (LIFO) — matching C Lua's behavior.

### Re-entrancy protection
`callAllPendingFinalizers` checks `GCRunningFinalizer` to prevent re-entrant
finalization if a `__gc` handler triggers `collectgarbage()`.

### Object resurrection
When an object moves to `tobefnz`, `markBeingFnz` marks it and all objects
reachable from it. After `__gc` runs, the object is linked back to `allgc`.
It will be collected in a subsequent GC cycle (matching C Lua's resurrection
semantics).

### Error handling
Like C Lua's `GCTM()`, errors in `__gc` are silently discarded. The `PCall`
wrapper catches any error and continues to the next finalizer.

## Supported Types

| Type | `__gc` Support | Notes |
|------|:-:|-------|
| Table | ✅ | Via `CheckFinalizer` in `SetMetatable` |
| Userdata | ✅ | Via `CheckFinalizer` in `SetMetatable` |

## C Lua Reference

- `lgc.c` — `GCTM()`: runs one finalizer, pops from `tobefnz`
- `lgc.c` — `separateTobeFnz()`: moves dead finobj → tobefnz
- `lgc.c` — `luaC_checkfinalizer()`: marks objects for finalization
- `lgc.c` — `markbeingfnz()`: marks objects being finalized (resurrection)
- `lstate.c` — `luaC_freeallobjects()`: runs all pending finalizers at state close

## Historical Note

The original implementation (pre-V5) used a `runtime.SetFinalizer` + enqueue +
drain pattern. Go finalizer callbacks would enqueue objects into `GCFinalizerQueue`,
and `DrainGCFinalizers()` would process them during `collectgarbage()`. This was
replaced by the V5 mark-and-sweep architecture which provides correct ordering,
resurrection support, and deterministic behavior.
