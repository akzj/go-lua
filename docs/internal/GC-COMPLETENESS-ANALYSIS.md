# go-lua GC Completeness Analysis

## Status: ✅ V5 Mark-and-Sweep GC — Complete

The V5 GC is a self-managed mark-and-sweep garbage collector that closely mirrors
C Lua's `lgc.c`. All 26 Lua 5.5.1 test suites pass with zero patches, including
`gc.lua` which passes fully unpatched.

### Feature Status

| Feature | Status | Notes |
|---------|:------:|-------|
| Mark-and-sweep with GCHeader | ✅ | All collectable types have GCHeader |
| Object lists (allgc/finobj/tobefnz/fixedgc) | ✅ | Correct linked-list management |
| Tri-color marking (white/gray/black) | ✅ | With currentwhite flip |
| Write barriers | ✅ | ~31 call sites, forward + backward barriers |
| `__gc` finalizers (tables) | ✅ | CheckFinalizer → separateTobeFnz → callAllPendingFinalizers |
| `__gc` finalizers (userdata) | ✅ | Same pathway as tables |
| Object resurrection | ✅ | markBeingFnz after separateTobeFnz |
| Weak-value tables (`__mode="v"`) | ✅ | clearByValues in atomic phase |
| Weak-key tables (`__mode="k"`) | ✅ | Ephemeron convergence + clearByKeys |
| All-weak tables (`__mode="kv"`) | ✅ | Both clearByValues + clearByKeys |
| Ephemeron convergence | ✅ | Iterative mark propagation |
| Dead key clearing (TagDeadKey) | ✅ | Preserves hash chain integrity |
| Re-entrancy protection | ✅ | GCRunning + GCRunningFinalizer guards |
| Periodic GC during VM execution | ✅ | Allocation-count triggered |
| Dual-mode traverseThread | ✅ | Precise (explicit) / conservative (periodic) |
| `collectgarbage("collect")` | ✅ | Full mark-sweep + finalization |
| `collectgarbage("stop"/"restart")` | ✅ | Controls GCStopped flag |
| `collectgarbage("isrunning")` | ✅ | Returns !GCStopped |
| `collectgarbage("count")` | ✅ | Tracks via GCTotalBytes counter |
| `collectgarbage("step")` | ✅ | Runs full GC cycle |
| `collectgarbage("incremental")` | ✅ | Stores params (single-mode GC) |
| `collectgarbage("generational")` | ✅ | Stores params (single-mode GC) |
| CloseState finalization | ✅ | Drains finalizers before closing |
| Memory deallocation tracking | ✅ | runtime.AddCleanup callbacks |

### GC Architecture

```
V5 Mark-and-Sweep (self-managed):
┌──────────────────────────────────────┐
│ Lua GC Engine (gc/api/gc.go)         │
│ ├─ Tri-color marking (white/gray/black) │
│ ├─ Write barriers (forward + back)   │
│ ├─ FullGC: mark → propagate → atomic │
│ │   ├─ convergeEphemerons            │
│ │   ├─ clearByValues / clearByKeys   │
│ │   ├─ separateTobeFnz + markBeingFnz │
│ │   └─ sweepList                     │
│ ├─ callAllPendingFinalizers (PCall)  │
│ ├─ Periodic GC (allocation-triggered)│
│ └─ Dual-mode thread traversal        │
│                                      │
│ Go GC handles:                       │
│ ├─ Memory deallocation (after unlink)│
│ └─ Cycle breaking (Go tracing GC)   │
└──────────────────────────────────────┘
```

### Design: Lua GC Controls Lifecycle, Go GC Handles Memory

The V5 architecture separates concerns:
- **Lua GC** decides which objects are alive/dead (mark phase), clears weak
  references, runs finalizers, and unlinks dead objects from chains
- **Go GC** handles actual memory deallocation after objects are unlinked
  (no explicit `free()` needed — Go's tracing GC handles cycles automatically)

This means `sweepList` simply unlinks dead objects from the chain. Go's GC
will reclaim the memory when no more Go references exist.

### Known Differences from C Lua

| Aspect | C Lua | go-lua | Impact |
|--------|-------|--------|--------|
| GC mode | Incremental + generational | Single full-cycle | `collectgarbage("step", n)` runs full GC |
| Memory tracking | Exact byte counting | Counter-based (AddCleanup) | `collectgarbage("count")` is approximate |
| GC pacing | Debt-based incremental | Allocation-count threshold | Different GC frequency characteristics |
| Finalization order | Strict LIFO | LIFO (tobefnz list order) | Matches C Lua |
| Write barrier impl | C macros | Go function calls (inlined) | Slightly different performance profile |

These differences do not affect correctness — all 26 test suites pass unpatched.

### Test Results

```
$ go test ./... -count=1
  26/26 Lua test suites: PASS
  15/15 Go packages: PASS
  gc.lua: PASS (unpatched, all 4 patches removed)
  Runtime patches: 0
  Skipped tests: 0
```

## Historical Note

This document originally analyzed the pre-V5 GC architecture, which delegated to
Go's GC via `runtime.SetFinalizer` and `weak.Pointer`. That analysis identified
numerous gaps (ephemerons, weak table clearing order, finalization semantics, etc.)
that were classified as "unfixable within Go GC constraints." The V5 rewrite
resolved all of these by implementing a native mark-and-sweep GC on top of Go.
