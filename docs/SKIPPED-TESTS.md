# Skipped Test Sections in go-lua Testes

## Current Status

**All 27 Lua 5.5.1 test files pass in `TestTestesWide`.**

The test runner (`testes_wide_test.go`) executes every official Lua test file with
`_port = true` and `_soft = true` flags set, which skip platform-specific and
stress-heavy sections as intended by the Lua test suite design.

### Runtime Patches (gc.lua only)

`gc.lua` requires runtime string-replacement patches to guard assertions that depend
on Go GC timing differences (weak table collection order, ephemeron clearing in a
single pass, etc.). These are being addressed by the GC rewrite work. No feature-level
skips remain — all patches are about GC timing semantics, not missing functionality.

### Files with _port Guards Removed

Several test files have had `_port` guards removed because go-lua now implements the
features they were guarding:

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

Previously, 5 test files did not pass (gc.lua, gengc.lua, closure.lua, files.lua,
cstack.lua). All now pass in `TestTestesWide` as of the current implementation.
