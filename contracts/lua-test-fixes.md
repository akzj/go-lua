# Lua Tests Fix Contract

## Mission
Fix lua-master/testes test failures. Target: 15+ passed, 0 failed. DO NOT skip tests.

## Failure Analysis Summary

### 1. stdlib Gap Failures (Easy Fixes)
| Test | Error | Missing Function |
|------|-------|------------------|
| strings.lua:432 | `attempt to call a non-function value` | `os.setlocale` |
| cstack.lua:27 | `attempt to call a non-function value` | `debug.debug` |
| gengc.lua:102 | `attempt to call a non-function value` | `collectgarbage("generational")` |

**Fix**: Add stub implementations in pkg/api/stdlib_*.go

### 2. Named Vararg Bug (Medium Fix)
- **Test**: vararg.lua:8 - `attempt to index a non-table value`
- **Root cause**: Parser sees `...t` but discards the name `t`. Codegen doesn't create the vararg table.
- **Lua 5.4 spec**: `local function f(...t)` means `t` is a table containing all varargs

**Fix locations**:
- pkg/parser/ast.go: Add VarargName field to FuncExpr/FuncDefStmt
- pkg/parser/expr.go: Store vararg name instead of discarding
- pkg/codegen/codegen.go: Build vararg table and assign to name

### 3. Hex Integer Overflow (Medium Fix)
- **Test**: tpack.lua:74
- **Root cause**: `0x13121110090807060504030201` exceeds int64 range
- **Fix**: pkg/vm/lexer.go - parse overflow as float

### 4. Assertion Failures (Various)
Tests with "assertion failed!" need individual debugging:
attrib.lua, calls.lua, coroutine.lua, db.lua, errors.lua, files.lua, gc.lua, goto.lua, locals.lua, math.lua, nextvar.lua, pm.lua, sort.lua, strings.lua, utf8.lua

## Required Fixes

### Phase 1: stdlib Stubs
1. os.setlocale - returns false
2. debug.debug - empty loop
3. collectgarbage("generational") - returns false

### Phase 2: Named Vararg
1. Store vararg name in AST
2. Emit table construction code

### Phase 3: Hex Overflow
1. Parse as float on overflow

### Phase 4: Assertion Debug
1. Run each test to find specific failures
2. Fix each assertion

## Verification
go test -v ./tests/... -run TestLuaTestSuite 2>&1 | grep -E "Lua test suite files: (1[5-9]|2[0-5]) passed, 0 failed"

## Implementation Details

### Named Vararg Table Construction
When a function has `...name` (e.g., `local function f(...t)`), the parser must:
1. Store the vararg name in AST (`FuncExpr.VarargName`, `FuncDefStmt.VarargName`)
2. Codegen must emit: build table `{[1]=arg1, [2]=arg2, ..., n=#varargs}` and assign to VarargName

Codegen approach:
1. Create new table: `OP_NEWTABLE A B C` (A=reg, B=0, C=0 for array part)
2. Copy varargs into table: loop with `OP_VARARG` and `OP_SETTABLE`
3. Or simpler: emit `OP_SETLIST` after building array manually

### stdlib Function Signatures
- `os.setlocale(locale string, category? string) -> string|false`
- `debug.debug() -> never returns` (enters interactive REPL, can stub as empty)
- `collectgarbage(opt string, ...) -> various` - "generational" option returns false (not supported)

### Coroutine Note
`coroutine.resume` currently returns `(false, "not implemented")`. Tests gengc.lua and coroutine.lua will fail until coroutines are properly implemented. Focus on other fixes first.

### Parent Clarification
- coroutine.lua and gengc.lua should NOT be skipped - attempt to fix them
- Priority: simpler fixes first, then coroutines, only skip if truly impossible
- Target: 15+ tests pass, 0 fail
