# Contract: lua-master/testes Runner

## Context

go-lua is a Lua 5.5.1 VM implementation. Goal: run `lua-master/testes` suite to verify VM correctness.

## Current State

| Module | Status | Notes |
|--------|--------|-------|
| `go build ./...` | ✅ Pass | Compiles |
| `go test ./...` | ❌ FAIL | `TestBinopKind` expects BINOP_CONCAT=17, got 20 |
| `lua-master/testes` | 🔲 TODO | 34 .lua files need to run |

## Issue 1: BinopKind Ordering

### Problem
`ast/api/api.go` BinopKind ordering doesn't match Lua 5.5 reference.

### Lua 5.5 Order (from `lua-master/lcode.h`)
```go
BINOP_ADD = 0, BINOP_SUB, BINOP_MUL, BINOP_MOD, BINOP_POW,
BINOP_DIV, BINOP_IDIV,
// bitwise
BINOP_BAND, BINOP_BOR, BINOP_BXOR, BINOP_SHL, BINOP_SHR,
// string
BINOP_CONCAT = 12,
// comparison (note: EQ, LT, LE before NE, GT, GE)
BINOP_EQ, BINOP_LT, BINOP_LE, BINOP_NE, BINOP_GT, BINOP_GE,
// logical
BINOP_AND, BINOP_OR
```

### Fix Required
Reorder `ast/api/api.go` BinopKind constants to match Lua 5.5 order.
Update `parse/internal/parser.go` if it depends on ordering.

## Issue 2: Test Runner

### Contract
```go
// TestesRunner runs lua-master/testes suite
type TestesRunner interface {
    // Run executes the testes suite, returns pass/fail counts
    Run() (passed int, failed int, err error)
    
    // RunFile executes a single .lua test file
    RunFile(path string) error
    
    // Results returns test results summary
    Results() TestResults
}

type TestResults struct {
    Passed  []string
    Failed  []string
    Errors  map[string]error
}
```

### Test Format
lua-master/testes files contain:
- Lua code
- Expected output in `-- output:` comments
- Test比对: 执行结果与注释期望输出一致则通过

## Acceptance Criteria

1. ✅ `go build ./...` compiles
2. ✅ `go test ./...` all tests pass (fix BINOP_CONCAT ordering)
3. 🔲 Create testes runner that runs lua-master/testes
4. 🔲 Record pass/fail counts for each .lua file
5. 🔲 Progressive fixes to pass more tests

## Constraints

- lua-master/testes约定：每个 .lua 文件包含测试代码 + 输出注释
- VM opcode 共 85 个 (opcodes/opcodes.go)
- Reference: `lua-master/testes/main.lua` for test framework

## Implementation Plan

1. Fix `ast/api/api.go` BinopKind ordering → `go test ./...` must pass
2. Create `testes/testes.go` with `TestesRunner` interface
3. Implement runner: parse lua file → execute via `state.DoString()` → compare output
4. Run all 34 .lua files, collect results
5. Fix VM bugs as discovered
