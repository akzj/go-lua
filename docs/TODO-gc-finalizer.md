# __gc Finalizer Support — Design Document

## Overview

go-lua now supports `__gc` finalizers for tables. When a table with a `__gc`
metamethod becomes unreachable, Go's garbage collector triggers a callback that
enqueues the table for Lua-level finalization. The `__gc` metamethods are then
executed synchronously when `collectgarbage("collect")` or `collectgarbage("step")`
is called.

This bridges Go's GC → Lua's `__gc` metamethod system using `runtime.SetFinalizer`.

## Architecture: Three-Component System

```
┌─────────────────────────────────────────────────────────────┐
│ 1. REGISTRATION (SetMetatable)                              │
│    When setmetatable(t, mt) is called and mt has __gc:      │
│    → runtime.SetFinalizer(tbl, enqueueFunc)                 │
└──────────────────────┬──────────────────────────────────────┘
                       │ (Go GC collects table)
                       ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. ENQUEUE (Go finalizer callback — arbitrary goroutine)    │
│    → Lock GCFinalizerMu                                     │
│    → Check GCClosed (skip if closing)                       │
│    → Append table to GCFinalizerQueue                       │
│    → Unlock                                                 │
│    ⚠ NEVER calls Lua functions here — unsafe goroutine!     │
└──────────────────────┬──────────────────────────────────────┘
                       │ (next collectgarbage call)
                       ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. DRAIN (collectgarbage → DrainGCFinalizers)               │
│    → Lock mutex, copy queue, clear it, unlock               │
│    → For each table: look up __gc metamethod                │
│    → Push __gc function + table onto Lua stack              │
│    → L.PCall(1, 0, 0) — protected call, errors discarded   │
│    → Repeat until queue is empty                            │
└─────────────────────────────────────────────────────────────┘
```

## Files Modified

### 1. `internal/state/api/api.go` — GlobalState fields
Added to `GlobalState` struct:
- `GCFinalizerMu sync.Mutex` — protects the finalizer queue
- `GCFinalizerQueue []any` — pending tables/userdata for `__gc` calls
- `GCClosed bool` — set `true` when state is closing; prevents post-close enqueuing

### 2. `internal/api/api/impl.go` — SetMetatable hook + DrainGCFinalizers
- **SetMetatable**: After setting a metatable on a table, checks if the metatable
  has `__gc` via `metamethodapi.GetTM()`. If present, registers a Go finalizer
  via `runtime.SetFinalizer(tbl, ...)`.
- **DrainGCFinalizers()**: Public method on `*State`. Atomically grabs the queue,
  then for each table: looks up `__gc`, pushes it + the table, calls via `PCall`.
  Errors are silently discarded (matching C Lua's `GCTM()` behavior).

### 3. `internal/stdlib/api/baselib.go` — collectgarbage wiring
- `collectgarbage("collect")`: calls `runtime.GC()` twice (second pass ensures
  finalizers from first GC have run), then `L.DrainGCFinalizers()`.
- `collectgarbage("step")`: same pattern.

### 4. `internal/state/api/state.go` — CloseState safety
- Before nilling references, sets `GCClosed = true` under the mutex.
- This prevents Go finalizer callbacks (running in arbitrary goroutines) from
  enqueuing into a dead state.

### 5. `internal/stdlib/api/gc_finalizer_test.go` — Tests
Three tests:
- `TestGCFinalizer`: Basic __gc fires after collectgarbage
- `TestGCFinalizerMultiple`: 5 tables all get __gc called
- `TestGCFinalizerErrorSwallowed`: Error in __gc doesn't prevent other __gc calls

## Key Design Decisions

### Why `runtime.SetFinalizer` + enqueue (not direct call)?
Go finalizers run in an arbitrary goroutine. Calling Lua functions from a Go
finalizer would corrupt the Lua state (which is not thread-safe). The enqueue
pattern ensures all Lua calls happen in the main goroutine.

### Why `runtime.GC()` twice?
The first `runtime.GC()` may identify unreachable objects. Go runs finalizers
after the GC cycle completes, potentially in a separate goroutine. The second
`runtime.GC()` ensures those finalizer goroutines have had time to run and
enqueue their objects.

### Why loop in DrainGCFinalizers?
A `__gc` finalizer might create new objects with `__gc`, or trigger another
GC cycle. The drain loop continues until the queue is empty, ensuring all
pending finalizers are processed.

### Error handling
Like C Lua's `GCTM()`, errors in `__gc` are silently discarded. The `PCall`
wrapper catches any error and continues to the next finalizer.

## Known Limitations

1. **Userdata**: `NewUserdata` is currently a stub (returns nil). Userdata `__gc`
   support is structurally ready (queue accepts `any`) but untested.

2. **Timing**: Go's GC is non-deterministic. A single `collectgarbage("collect")`
   may not collect all unreachable objects. Multiple calls may be needed for
   complete finalization (the tests use 3 cycles for reliability).

3. **No weak tables**: `__mode` is not yet supported. Weak table entries are not
   collected by Go's GC.

4. **No incremental/generational modes**: `collectgarbage("incremental")` and
   `collectgarbage("generational")` return defaults but don't change behavior.

5. **Order of finalization**: C Lua finalizes objects in a specific order
   (reverse creation order within a GC cycle). Go's finalizer ordering is
   unspecified, so `__gc` call order may differ from C Lua.

6. **Resurrection**: C Lua supports object resurrection (a `__gc` can store `self`
   somewhere, keeping it alive). In go-lua, the Go finalizer fires once per
   `runtime.SetFinalizer` registration. If `__gc` resurrects the object, a new
   `runtime.SetFinalizer` would need to be registered (not currently implemented).

## C Lua Reference

- `lgc.c` — `GCTM()`: runs one finalizer, pops it from the `tobefnz` list
- `lgc.c` — `luaC_checkfinalizer()`: marks objects for finalization at `setmetatable` time
- `lstate.c:390` — `luaC_freeallobjects()`: runs all pending finalizers at state close
