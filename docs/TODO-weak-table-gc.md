# Weak Table GC Support — V5 Architecture

## Status: ✅ Complete (V5 Native Weak Table Clearing)

Weak tables with `__mode="k"/"v"/"kv"` are fully supported via the V5
mark-and-sweep GC. Weak table clearing happens natively during the GC's
atomic phase — no Go `weak.Pointer` involvement.

## Architecture: Native Mark-and-Sweep Clearing

The V5 GC handles weak tables as part of its atomic phase in `FullGC()`,
mirroring C Lua's `lgc.c` approach:

```
FullGC atomic phase:
  1. markRoots → propagateAll        (mark all reachable objects)
  2. Process grayagain list          (deferred weak tables from propagate)
  3. convergeEphemerons              (weak-key table convergence)
  4. clearByValues(Weak, AllWeak)    (clear unmarked values from weak-value tables)
  5. separateTobeFnz + markBeingFnz (finalization — may resurrect objects)
  6. convergeEphemerons (2nd pass)   (for resurrected objects)
  7. clearDeadKeysAllEphemerons      (clear dead keys from ephemeron tables)
  8. clearByKeys(AllWeak)            (clear dead keys from weak-key tables)
  9. clearByValues (2nd pass)        (clear values from newly-added weak tables)
  10. sweepList                      (remove dead objects from allgc)
```

### Key Functions

| Function | File | Purpose |
|----------|------|---------|
| `clearByValues` | `gc/api/gc.go:654` | Clears unmarked values from weak-value tables |
| `clearByKeys` | `gc/api/gc.go:627` | Clears unmarked keys from weak-key tables |
| `convergeEphemerons` | `gc/api/gc.go:691` | Iterative convergence for ephemeron tables |
| `clearDeadKeysAllEphemerons` | `gc/api/gc.go` | Clears dead keys + uses `TagDeadKey` sentinel |
| `traverseWeakValue` | `gc/api/gc.go` | Links weak-value tables to `g.Weak` list |
| `traverseEphemeron` | `gc/api/gc.go` | Links ephemeron tables to `g.Ephemeron` list |

### Weak Table Classification (during traversal)

| `__mode` | Classification | GC List | Clearing |
|----------|---------------|---------|----------|
| `"v"` | Weak-value | `g.Weak` | `clearByValues` |
| `"k"` | Ephemeron | `g.Ephemeron` | `convergeEphemerons` + `clearByKeys` |
| `"kv"` | All-weak | `g.AllWeak` | Both `clearByValues` + `clearByKeys` |

### Dead Key Handling

When a key in a weak-key table is collected, the key is replaced with a
`TagDeadKey` sentinel (not simply removed). This preserves hash chain
integrity — other entries in the same chain remain findable. The sentinel
is a special tag value that:
- Cannot match any lookup (dead keys are never found)
- Preserves the hash chain structure
- Gets cleaned up during table resize

This mirrors C Lua's `setdeadkey()` / `keyisdead()` mechanism.

### Ephemeron Convergence

Ephemeron tables (weak-key tables where values may reference keys) require
iterative convergence:

1. First pass: for each entry, if the key is marked, mark the value
2. If any new objects were marked, repeat (values may make other keys reachable)
3. Converge until no new marks in a pass
4. After convergence, any remaining unmarked keys are dead

This runs twice in `FullGC`: once before finalization, once after (to handle
resurrected objects).

## Design Decisions

### Why native clearing (not Go `weak.Pointer`)?

The original implementation used Go's `weak.Pointer[T]` with a two-phase
prepare/sweep approach. This was replaced because:

1. **Ordering**: V5 GC controls the exact order of clearing vs finalization
2. **Ephemerons**: Go has no ephemeron concept; native GC implements convergence
3. **Correctness**: `weak.Pointer` was restoring entries that the Lua GC had
   already correctly cleared (the two systems fought each other)
4. **Simplicity**: One GC system instead of two

`SweepWeakTables()` is now disabled in `GCCollect()` — the V5 GC's
`clearByValues`/`clearByKeys` handles everything during the atomic phase.

### Non-collectable types persist forever

Integers, floats, booleans, and strings are not GC-managed objects and persist
in weak tables indefinitely. This matches C Lua behavior.

## Pointer-Backed Types (can be weak-collected)

| Type | Go Type |
|------|---------|
| Table | `*tableapi.Table` |
| Lua Closure | `*closureapi.LClosure` |
| C Closure | `*closureapi.CClosure` |
| Userdata | `*objectapi.Userdata` |
| Thread | `*stateapi.LuaState` |

## C Lua Reference

- `lgc.c:596` — `getmode()` reads `__mode`
- `lgc.c:808` — `clearbyvalues()` clears unmarked weak value entries
- `lgc.c:789` — `clearbykeys()` clears unmarked weak key entries
- `lgc.c:530-570` — ephemeron convergence
- `lgc.c:1542` — `atomic()` phase calls clear functions

## Test Coverage

- `testes/gc.lua` — passes unpatched (all 4 patches removed)
- `testes/coroutine.lua:478` — weak table + coroutine wrap assertion
- All 26 Lua test suites pass with zero patches

## Historical Note

The original implementation (pre-V5) used Go's `weak.Pointer[T]` (Go 1.24+)
with a two-phase sweep: Phase 1 created weak pointers and nil'd strong refs,
Phase 2 (after `runtime.GC()`) checked which pointers survived. This was
replaced by the V5 native mark-and-sweep approach.
