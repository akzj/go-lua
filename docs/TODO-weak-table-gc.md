# Weak Table GC Support — IMPLEMENTED

## Status: ✅ Implemented

Weak tables with `__mode="k"/"v"/"kv"` are now fully supported.
`coroutine.lua:478` assertion is enabled and passes.

## Architecture

### Two-Phase Sweep Approach

Unlike C Lua which integrates weak table clearing into its mark-and-sweep GC,
go-lua uses Go's GC via `weak.Pointer[T]` (Go 1.24+). The implementation uses
a **two-phase sweep** triggered by `collectgarbage()`:

**Phase 1 (before GC):** For each registered weak table, scan all entries.
For pointer-backed values/keys, create a `weak.Pointer[T]` and nil out the
strong reference in the table. Non-pointer values (int64, float64, bool,
strings) are left in place — they persist forever (matches C Lua semantics).

**Phase 2 (after GC):** Check each `weak.Pointer`. If `.Value()` returns
non-nil, the object is still alive — restore it. If nil, the object was
collected — leave the entry as nil.

### Key Design Decisions

1. **Callback pattern for circular import avoidance**: `table/api` cannot
   import `closure/api` or `state/api` (dependency cycle). Instead, `table/api`
   defines `WeakRefMake`/`WeakRefCheck` function variables that the API layer
   (`internal/api/api/weak.go`) registers at init time with concrete type
   handlers for all 5 pointer-backed types.

2. **No Get/Set interception**: Values are stored normally (strong refs) in
   the table. Weak behavior only manifests during `collectgarbage()` sweep.
   This avoids modifying the hot Get/Set/Next paths.

3. **Non-pointer types persist forever**: Integers, floats, booleans, and
   strings cannot use `weak.Pointer` and are never collected from weak tables.
   This matches C Lua behavior where non-collectable values survive in weak
   tables.

4. **No strong refs in sweep closures**: The `PrepareWeakSweep` closure
   captures only weak refs (containing `weak.Pointer`) and non-pointer values.
   No TValues holding GC-collectable pointers are captured — this is critical
   for the GC to actually collect unreferenced objects during the sweep.

### Pointer-Backed Types (can be weak)
| Type | Tag | Go Type |
|------|-----|---------|
| Table | 0x05 | `*tableapi.Table` |
| Lua Closure | 0x06 | `*closureapi.LClosure` |
| C Closure | 0x26 | `*closureapi.CClosure` |
| Userdata | 0x07 | `*objectapi.Userdata` |
| Thread | 0x08 | `*stateapi.LuaState` |

### Non-Pointer Types (persist forever)
- `int64`, `float64`, `bool`, `nil`
- `*objectapi.LuaString` (interned, value-semantic)
- Light C functions (`stateapi.CFunction`)

## Files Modified

| File | Changes |
|------|---------|
| `internal/table/api/api.go` | `WeakMode byte` field, `WeakKey`/`WeakValue` constants, `HasWeakKeys()`/`HasWeakValues()` helpers, `WeakArrayRefs`/`WeakKeyRefs`/`WeakValRefs` parallel storage |
| `internal/table/api/weak.go` | **NEW** — `WeakRefMake`/`WeakRefCheck` callback vars, `PrepareWeakSweep()` two-phase sweep |
| `internal/api/api/weak.go` | **NEW** — Registers callbacks with concrete `weak.Pointer[T]` handlers, `SweepWeakTables()` method |
| `internal/api/api/impl.go` | `SetMetatable()` now parses `__mode` from metatable, sets `WeakMode`, registers in `GlobalState.WeakTables` |
| `internal/state/api/api.go` | `WeakTables []any` field on `GlobalState` |
| `internal/state/api/state.go` | `RegisterWeakTable()` method |
| `internal/stdlib/api/baselib.go` | `collectgarbage("collect"/"step")` calls `L.SweepWeakTables()` |
| `lua-master/testes/coroutine.lua` | Line 478: un-skipped `assert(C[1] == undef)` |

## C Lua Reference
- `lgc.c:596` — `getmode()` reads `__mode`
- `lgc.c:808` — `clearbyvalues()` clears unmarked weak value entries
- `lgc.c:789` — `clearbykeys()` clears unmarked weak key entries
- `lgc.c:1542` — `atomic()` phase calls clear functions

## Test Coverage
- `internal/stdlib/api/weak_test.go` — Go tests for weak values, weak keys, weak kv
- `lua-master/testes/coroutine.lua:478` — Lua test for weak table with coroutine wrap
- All 21 testes pass (regression verified)
