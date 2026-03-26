# Phase D: CLI Error Messages & Line Numbers — Design Document

## Overview
The VM's error messages currently lack line number information. The infrastructure exists
(`Prototype.LineInfo []int`, `Prototype.Source string`) but is not populated by codegen
and not used by the VM when generating errors.

## Current State
- `pkg/object/value.go`: `Prototype.LineInfo []int` field exists but is always nil/empty
- `pkg/codegen/`: Never populates `LineInfo`
- `pkg/vm/vm.go`: Error messages use `fmt.Errorf("attempt to ...")` with no source location
- `pkg/api/error.go`: `captureStack()` is a stub returning `[G]:2: in function ?`
- Error format: `filename: attempt to index a non-table value`
- Desired format: `filename:3: attempt to index a non-table value`

## Design

### 1. Populate LineInfo in Codegen (`pkg/codegen/codegen.go`)

The `CodeGenerator` already tracks AST nodes which have position info from the lexer.
Each time an instruction is emitted via `EmitABC`, `EmitABx`, `EmitAsBx`, or `EmitAx`,
append the current line number to `Prototype.LineInfo`.

**Contract:**
```go
// In CodeGenerator, add a field to track current line:
currentLine int

// Before generating each statement, set currentLine from the AST node's position.
// In each Emit* method, append currentLine to cg.Prototype.LineInfo:
func (cg *CodeGenerator) EmitABC(op vm.Opcode, a, b, c int) int {
    // ... existing code ...
    cg.Prototype.LineInfo = append(cg.Prototype.LineInfo, cg.currentLine)
    return idx
}
```

**AST position info**: Check `parser.Node` types for `Line()` or position fields.
The lexer's `Token` has `Line int` and `Column int`. Statement nodes should carry
this info. If they don't, use the lexer's current position when entering each statement.

### 2. Use LineInfo in VM Errors (`pkg/vm/vm.go`)

Add a helper method to get the current source location:

```go
// getCurrentLine returns the current line number from LineInfo, or 0 if unavailable.
func (vm *VM) getCurrentLine() int {
    if vm.Prototype != nil && vm.PC > 0 && vm.PC-1 < len(vm.Prototype.LineInfo) {
        return vm.Prototype.LineInfo[vm.PC-1] // PC-1 because PC was already incremented
    }
    return 0
}

// getCurrentSource returns the current source file name.
func (vm *VM) getCurrentSource() string {
    if vm.Prototype != nil && vm.Prototype.Source != "" {
        return vm.Prototype.Source
    }
    return "?"
}

// runtimeError creates a formatted runtime error with source location.
func (vm *VM) runtimeError(format string, args ...interface{}) error {
    msg := fmt.Sprintf(format, args...)
    line := vm.getCurrentLine()
    source := vm.getCurrentSource()
    if line > 0 {
        return fmt.Errorf("%s:%d: %s", source, line, msg)
    }
    return fmt.Errorf("%s: %s", source, msg)
}
```

Then replace `fmt.Errorf("attempt to ...")` calls with `vm.runtimeError("attempt to ...")`.

### 3. Improve Stack Traces (`pkg/api/error.go`)

The `captureStack` function needs access to the VM's CallInfo stack.
Either:
- (a) Pass the VM to `newLuaError`, or
- (b) Have the VM build the stack trace and pass it to the error constructor.

Option (b) is cleaner:

```go
// In vm.go:
func (vm *VM) buildStackTrace() []string {
    var trace []string
    for i := vm.CI; i >= 0; i-- {
        ci := vm.CallInfo[i]
        if ci == nil { continue }
        source := "?"
        line := 0
        if ci.Closure != nil && ci.Closure.Proto != nil {
            proto := ci.Closure.Proto
            if proto.Source != "" { source = proto.Source }
            if ci.PC > 0 && ci.PC-1 < len(proto.LineInfo) {
                line = proto.LineInfo[ci.PC-1]
            }
        }
        if ci.Closure != nil && ci.Closure.IsGo {
            trace = append(trace, fmt.Sprintf("[C]: in function '%s'", "?"))
        } else if line > 0 {
            trace = append(trace, fmt.Sprintf("%s:%d: in function '%s'", source, line, "?"))
        } else {
            trace = append(trace, fmt.Sprintf("%s: in function '%s'", source, "?"))
        }
    }
    return trace
}
```

### 4. Files to Modify
1. `pkg/codegen/codegen.go` — Add `currentLine` field, populate `LineInfo` in Emit methods
2. `pkg/codegen/stmt.go` — Set `currentLine` from AST node positions before generating each statement
3. `pkg/codegen/expr.go` — Set `currentLine` from expression positions where relevant
4. `pkg/vm/vm.go` — Add `getCurrentLine()`, `getCurrentSource()`, `runtimeError()` helpers; replace `fmt.Errorf` calls
5. `pkg/api/error.go` — Update error creation to accept stack traces from VM

### 5. Constraints
- Do NOT break any existing tests
- Do NOT modify the metatable, concat, or REPL code that was just fixed
- Line numbers should be 1-based (matching Lua convention)
- If LineInfo is empty/unavailable, fall back to current behavior (no line number)
- `go build ./...` and `go test ./...` must pass after changes

### 6. Testing
- `go test ./...` must pass (zero failures)
- Manual test: a script with a runtime error on line 3 should show `:3:` in the error message