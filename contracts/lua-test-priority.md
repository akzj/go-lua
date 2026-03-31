# Lua Test Fixes - Priority Contract

## Mission
Fix lua-master/testes test failures. Target: **15+ passed, 0 failed**. DO NOT skip tests.

## Current Baseline (After Timeout Fix)
- 4 passed: api.lua, code.lua, pm.lua, tracegc.lua
- 21 failed (see below)
- 9 skipped (keep these)

## Critical Failures to Fix

### 1. gengc.lua:102 - collectgarbage stub missing
```
Error: attempt to call a non-function value
```
**Root cause**: `collectgarbage("generational")` returns `false` but Lua expects it to work
**Fix**: Return true for "generational" - it's a no-op but should succeed

### 2. tpack.lua:13 - table.pack missing
```
Error: attempt to call a non-function value
```
**Root cause**: `table.pack` not registered in stdlib
**Fix**: Add `table.pack` to pkg/api/stdlib_table.go

### 3. vararg.lua:8 - Named vararg not implemented
```
Error: attempt to index a non-table value
```
**Root cause**: `...name` syntax needs table construction
**Fix**: Convert `...name` to just `...` in preprocessor (already done), or implement table creation

### 4. cstack.lua - VM depth tracking missing
```
Error: test timed out after 10 seconds
```
**Root cause**: xpcall doesn't track call depth, causing infinite recursion
**Fix**: Add call depth limit to xpcall implementation

### 5. main.lua:41 - arg table issue
```
Error: attempt to index a non-table value
```
**Root cause**: `_ARG` table not properly initialized
**Fix**: Check arg table initialization in pkg/api/stdlib.go

## Secondary Failures (Assertion Failed)
attrib.lua, calls.lua, coroutine.lua, db.lua, errors.lua, files.lua, gc.lua, goto.lua, locals.lua, math.lua, nextvar.lua, sort.lua, strings.lua, utf8.lua

These need individual debugging. Run:
```bash
cd /home/ubuntu/workspace/go-lua && go run ./cmd/lua lua-master/testes/<file>.lua
```

## Implementation Order

### Phase 1: Quick Wins
1. Add `table.pack` to stdlib_table.go
2. Fix `collectgarbage("generational")` to return true
3. Verify `_ARG` table initialization

### Phase 2: cstack.lua Fix
1. Add call depth limit to xpcall (max ~1000 calls)
2. Return "error in error handling" when limit exceeded

### Phase 3: Named Vararg
1. Already converted to `...` in preprocessor
2. Need to ensure vararg works without name

## Verification
```bash
cd /home/ubuntu/workspace/go-lua
go clean -testcache
timeout 180 go test -v ./tests/... -run TestLuaTestSuite 2>&1 | grep -E "Lua test suite files: (1[5-9]|2[0-5]) passed, 0 failed"
```

## Acceptance Criteria
- Lua test suite files: 15-25 passed, 0 failed
- cstack.lua completes (passes or fails gracefully, not timeout)