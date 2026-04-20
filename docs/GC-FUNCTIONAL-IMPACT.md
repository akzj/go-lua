# GC Functional Impact — Post-V5 Status

## Status: ✅ V5 GC Complete — All Major Issues Resolved

With the V5 mark-and-sweep GC, all previously identified functional impact areas
have been resolved. This document is retained for historical reference.

## Current Functional Status

| Feature | Status | Notes |
|---------|:------:|-------|
| `__gc` finalizers (tables) | ✅ | Native mark-and-sweep finalization |
| `__gc` finalizers (userdata) | ✅ | Same pathway as tables |
| Weak tables (`__mode="k"/"v"/"kv"`) | ✅ | Native clearing in atomic phase |
| Ephemeron convergence | ✅ | Iterative convergence implemented |
| `collectgarbage("collect")` | ✅ | Full mark-sweep + finalization |
| `collectgarbage("stop"/"restart")` | ✅ | GCStopped flag |
| `collectgarbage("count")` | ✅ | Counter-based tracking |
| `collectgarbage("isrunning")` | ✅ | Returns !GCStopped |
| Object resurrection | ✅ | markBeingFnz after separateTobeFnz |
| Finalization re-entrancy | ✅ | GCRunningFinalizer guard |
| CloseState finalization | ✅ | Drains finalizers before closing |
| `<close>` variables | ✅ | Independent of GC (always worked) |

## Impact by Use Case

| Use Case | Completeness | Notes |
|----------|:------------:|-------|
| Script execution | ~100% | All core language features work |
| Configuration files | 100% | No GC dependency |
| Go embedding | ~98% | Full GC support |
| File processing (io library) | ~100% | `__gc` works for userdata |
| Long-running services | ~98% | Native weak table clearing |
| C library bindings (userdata) | ~98% | `__gc` works for userdata |
| GC tuning | ~90% | Single-mode GC (no incremental/generational) |

## Known Differences from C Lua

| Aspect | Impact |
|--------|--------|
| No incremental GC mode | `collectgarbage("step", n)` runs full cycle |
| No generational GC mode | Both modes store params but use same full-cycle GC |
| Memory counting approximate | `collectgarbage("count")` uses counter, not exact bytes |
| GC pacing | Allocation-count threshold vs debt-based |

These differences do not affect correctness — all 26 Lua 5.5.1 test suites pass.

## Historical Note

This document originally analyzed the pre-V5 GC architecture's functional gaps,
including: userdata `__gc` not firing, weak table caches leaking, ephemeron
convergence missing, and `collectgarbage()` options being no-ops. All of these
were resolved by the V5 mark-and-sweep rewrite.
