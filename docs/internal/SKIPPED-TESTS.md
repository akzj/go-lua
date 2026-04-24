# Test Status — go-lua Testes

## Current Status: ✅ ALL 26 Test Suites PASS

**All 26 Lua 5.5.1 test files pass in `TestTestesWide` with:**
- **Zero runtime patches** — no string-replacement patches in the test runner
- **Zero skipped test files**
- **Zero failures**

The test runner (`testes_wide_test.go`) executes every official Lua test file with
`_port = true` and `_soft = true` flags set.

### gc.lua — Fully Unpatched ✅

`gc.lua` previously required 4 runtime string-replacement patches to work around
GC timing differences. With the V5 mark-and-sweep GC implementation, **all patches
have been removed** and gc.lua passes unpatched. Key V5 features that enabled this:

- Native weak table clearing (`clearByValues`/`clearByKeys`)
- Ephemeron convergence (`convergeEphemerons`)
- Dead key clearing with `TagDeadKey` sentinel
- `separateTobeFnz` + `callAllPendingFinalizers` for `__gc` finalization
- Object resurrection via `markBeingFnz`

### Remaining `_port` Guards

The test runner still uses `_port = true`, which causes the Lua test files themselves
to skip certain platform-specific sections. These are **not** go-lua limitations —
they are the standard Lua test suite's own guards for:

- `os.date` edge cases and format specifiers
- POSIX-specific modifiers
- Locale-dependent behavior
- C-level test API (`T` / `testC`) sections
- Interpreter-specific features (standalone `lua` binary)

These guards are part of the official Lua test suite design and are used by all
non-reference Lua implementations.

### Files with `_port` Guards Previously Removed

Several test files had `_port` guards removed because go-lua implements the features
they were guarding:

- `nextvar.lua` — yield-in-`__pairs` (CallK implemented)
- `strings.lua` — `%a` format for inf/nan/-0.0, locale tests
- `errors.lua` — global function name resolution
- `files.lua` — yield across dofile, binary chunk loading

### Interpreter Harness Files (Not Tested)

`main.lua` and `all.lua` are interpreter harness files that start with `#!` shebang
lines. They are skipped in `TestParseAllTestes` because shebang stripping is handled
by the file loader (`DoFile`), not the parser.

---

## Historical Notes

Previously, multiple test files required patches or did not pass:
- `gc.lua` — required 4 runtime patches (Patch 0-3) for GC timing differences
- `closure.lua` — required patch for weak table auto-sweep
- `files.lua` — required registry LinkGC fix
- `cstack.lua` — required tracegc module

All issues were resolved by the V5 GC rewrite. As of commit `173a9ea`, all 26
test suites pass cleanly.
