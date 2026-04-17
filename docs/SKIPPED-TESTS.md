# Skipped Test Sections in go-lua Testes

This document tracks all test sections that are skipped or commented out in go-lua's
copy of the Lua 5.5.1 testes. Each skip has a reason, the mechanism used, and what
would be needed to un-skip it.

---

## coroutine.lua

### Skip 1: Weak Table GC Assertion
- **Status**: ✅ RESOLVED / IMPLEMENTED
- **Location**: `coroutine.lua:478`
- **Mechanism**: Line commented out (`-- assert(C[1] == undef)`)
- **What it tests**: After `collectgarbage()`, a weak table entry should be collected
- **Why skipped**: go-lua **now has** weak table support. `__mode` metafield is fully implemented.
- **Resolution**: Weak table support implemented. Line un-commented and test passes.
- **Commit**: `93811bb`

### Skip 2: Yield Inside Metamethods + For-Iterators
- **Location**: `coroutine.lua:858-:1055`
- **Mechanism**: `if false then ... end` block
- **What it tests**: `coroutine.yield()` inside `__lt`, `__le`, `__eq`, `__add`, etc. metamethods, and inside generic `for` iterators
- **Why skipped**: Requires VM continuation support (`lua_callk` equivalent) at every metamethod dispatch point in `Execute`. ~20 opcodes need continuation functions.
- **To un-skip**: Implement yield-in-metamethods (see `docs/TODO-yield-in-metamethods.md`)
- **Commit**: `c466c3e`
- **Lines skipped**: ~197 lines

---

## nextvar.lua

### Skip 3: Yield Inside `__pairs`
- **Location**: `nextvar.lua:938-:957`
- **Mechanism**: `if not _port then ... end` guard (`_port` is set in go-lua's test environment)
- **What it tests**: `coroutine.yield()` inside a `__pairs` metamethod, then resuming the `for` loop
- **Why skipped**: Same root cause as coroutine.lua skip 2 — requires yieldable C calls (`lua_callk`). The `pairs()` C function would need to register a continuation.
- **To un-skip**: Implement `lua_callk` / continuation support in stdlib C functions
- **Commit**: `0f2a947`
- **Lines skipped**: ~19 lines

---

## Summary Table

| # | File | Lines | Mechanism | Root Cause | Dependency |
|---|------|-------|-----------|------------|------------|
| 1 | coroutine.lua:478 | 1 line | comment | ✅ RESOLVED | Commit `93811bb` |
| 2 | coroutine.lua:858-1055 | ~197 lines | `if false` | Yield-in-metamethods | `docs/TODO-yield-in-metamethods.md` |
| 3 | nextvar.lua:938-957 | ~19 lines | `_port` guard | Yield-in-C-calls | Same as #2 |

**Total skipped**: ~216 lines across 2 files (1 resolved)

---

## Notes

### `_port` vs `_soft` vs `if false`
- **`_port`**: Standard Lua test flag for "portable" mode — skips platform-specific tests. go-lua sets `_port = true` to skip tests requiring C API features (yield across C boundaries, testC functions, etc.)
- **`_soft`**: Standard Lua test flag for reduced stress — uses smaller iteration counts. go-lua sets `_soft = true` to avoid timeouts on heavy tests.
- **`if false`**: Used for go-lua-specific skips where `_port`/`_soft` don't apply (e.g., yield-in-metamethods is not a portability issue, it's a missing feature).

### Files NOT passing (not skipped, just failing)
These files are not yet included in the test suite (require unimplemented features):
- `gc.lua` — fails at :15 (GC mode switching)
- `gengc.lua` — fails at :122 (generational GC)
- `closure.lua` — OOM crash (GC pressure)
- `files.lua` — fails at :10 (io/os library missing)
- `cstack.lua` — fails at :29 (C stack overflow detection)

**Current status**: 21/21 testes files PASS. The 5 files above are not yet included in the test runner.
