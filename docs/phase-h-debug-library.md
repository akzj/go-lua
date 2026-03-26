# Phase H: Debug Library Design Document

## Overview

This document describes the architecture for implementing the Lua 5.4 debug library in go-lua. The debug library provides introspection capabilities for debugging Lua programs.

## Goals

1. Implement core debug functions: `debug.getinfo()`, `debug.getlocal()`, `debug.setlocal()`, `debug.traceback()`, `debug.debug()`
2. Optionally implement hook functions: `debug.sethook()`, `debug.gethook()`
3. Enable at least 5 additional official Lua tests to pass
4. Maintain backward compatibility and minimal performance impact

## Current Architecture Analysis

### Existing Components

1. **`pkg/object/value.go`**:
   - `Prototype` already has debug info fields: `Source`, `LineInfo`, `LocVars`
   - `LocVar` struct: `Name`, `Start`, `End` (PC range for variable lifetime)
   - `Closure` references `Prototype` and `Upvalues`

2. **`pkg/vm/vm.go`**:
   - `CallInfo` tracks call frames: `Func`, `Closure`, `Base`, `PC`, `NResults`
   - `VM` has `CallInfo` stack and `CI` index
   - Stack access via `Stack[]`, `Base`, `StackTop`

3. **`pkg/codegen/codegen.go`**:
   - `LocalVar` tracks variables during compilation: `Name`, `Index`, `Active`, `IsParam`
   - `Locals` is scoped (array of scopes)
   - Already emits `LineInfo` per instruction

### Missing Components

1. **Local variable debug info in Prototype**:
   - `codegen.LocalVar` is not persisted to `object.LocVar`
   - Need to capture variable name, register index, and PC range

2. **Debug library registration**:
   - No `debug` module exists

3. **Hook mechanism**:
   - No hook support in VM

## Design

### 1. Debug Info Storage

#### LocVar Enhancement

The `object.LocVar` already exists but needs to be populated during compilation:

```go
// In object/value.go (already exists)
type LocVar struct {
    Name      string  // Variable name
    Start     int     // Start PC (instruction index)
    End       int     // End PC (instruction index)
    RegIndex  int     // NEW: Register index for stack access
}
```

**Why add RegIndex?**
- `debug.getlocal()` needs to know which register holds the variable
- Without it, we can't map variable names to stack positions

#### Codegen Changes

During compilation, capture local variable debug info:

```go
// In codegen/codegen.go
func (cg *CodeGenerator) finalizeLocVars() {
    for _, scope := range cg.Locals {
        for _, local := range scope {
            cg.Prototype.LocVars = append(cg.Prototype.LocVars, object.LocVar{
                Name:     local.Name,
                Start:    local.StartPC,  // Track when variable becomes active
                End:     cg.PC,            // Current PC is end
                RegIndex: local.Index,     // Register assignment
            })
        }
    }
}
```

**Invariant**: Each LocVar entry must have valid Start <= End and a valid RegIndex.

### 2. Debug Library API

#### debug.getinfo([thread,] f [, what])

Returns a table with function information.

**Stack levels**:
- Level 1 = current function
- Level 2 = caller of current function
- etc.

**Fields** (selected from Lua 5.4 spec):
- `source`: Source name (e.g., "@file.lua" or "[string]")
- `short_src`: Short source name for error messages
- `linedefined`: First line of function definition
- `lastlinedefined`: Last line of function definition
- `what`: "Lua", "C", or "main"
- `name`: Function name (if available)
- `namewhat`: "global", "local", "method", "field", or ""
- `nparams`: Number of parameters
- `isvararg`: Whether function is vararg
- `func`: Function value

**Implementation**:

```go
func debugGetinfo(L *State) int {
    // Parse arguments
    level := 1
    what := "flnStu"
    
    if L.IsNumber(1) {
        level, _ = L.ToNumber(1)
        if L.IsString(2) {
            what, _ = L.ToString(2)
        }
    } else if L.IsFunction(1) {
        // Direct function argument
        // ...
    }
    
    // Get CallInfo at level
    ci := L.vm.getCallInfoAtLevel(level)
    if ci == nil {
        L.PushNil()
        return 1
    }
    
    // Build info table
    L.NewTable()
    // ... populate fields based on 'what'
    
    return 1
}
```

**Invariant**: `debug.getinfo(n)` returns nil if level n doesn't exist.

#### debug.getlocal([thread,] f, local)

Returns name and value of local variable at given index.

**Implementation**:

```go
func debugGetlocal(L *State) int {
    level, _ := L.ToNumber(1)
    localIdx, _ := L.ToNumber(2)
    
    ci := L.vm.getCallInfoAtLevel(int(level))
    if ci == nil {
        L.PushNil()
        return 1
    }
    
    // Get prototype for Lua function
    if ci.Closure == nil || ci.Closure.IsGo {
        L.PushNil()
        return 1
    }
    
    proto := ci.Closure.Proto
    
    // Find local variable at given index active at current PC
    varName, regIdx := findLocalAtPC(proto, ci.PC, int(localIdx))
    if varName == "" {
        L.PushNil()
        return 1
    }
    
    // Get value from stack
    value := L.vm.Stack[ci.Base + regIdx]
    
    L.PushString(varName)
    L.vm.Push(value)
    return 2
}
```

**Invariant**: Returns (name, value) or nil if not found.

#### debug.setlocal([thread,] f, local, value)

Sets local variable value.

**Implementation**: Similar to `getlocal` but sets the stack value.

**Invariant**: Returns "no variable" if variable doesn't exist.

#### debug.traceback([thread,] [message [, level]])

Returns stack traceback string.

**Implementation**:

```go
func debugTraceback(L *State) int {
    message := ""
    level := 1
    
    if L.IsString(1) {
        message, _ = L.ToString(1)
    }
    if L.IsNumber(2) {
        level, _ = L.ToNumber(2)
    }
    
    var sb strings.Builder
    if message != "" {
        sb.WriteString(message)
        sb.WriteString("\n")
    }
    sb.WriteString("stack traceback:")
    
    // Walk call stack
    for i := level; ; i++ {
        ci := L.vm.getCallInfoAtLevel(i)
        if ci == nil {
            break
        }
        
        sb.WriteString("\n\t")
        sb.WriteString(formatFrame(ci, L.vm))
    }
    
    L.PushString(sb.String())
    return 1
}
```

**Invariant**: Always returns a string, even for empty stack.

#### debug.debug()

Simple interactive debugger loop.

**Implementation**: Read-eval-print loop until user types "cont".

### 3. VM Enhancements

#### getCallInfoAtLevel

```go
// In vm/vm.go
func (vm *VM) getCallInfoAtLevel(level int) *CallInfo {
    // Level 1 = current (CI)
    // Level 2 = caller (CI-1)
    // etc.
    idx := vm.CI - level + 1
    if idx < 0 || idx > vm.CI {
        return nil
    }
    return vm.CallInfo[idx]
}
```

**Invariant**: Level 1 always returns current frame if CI >= 0.

#### Hook Mechanism (Optional)

For `debug.sethook()`:

```go
type Hook struct {
    Function *object.Closure
    Mask     uint8  // 'c' = call, 'r' = return, 'l' = line
    Count    int    // Count interval for count hooks
}

// In VM
type VM struct {
    // ... existing fields
    Hook      *Hook
    HookCount int
}
```

**Why optional?**
- Hook mechanism is complex and may not be needed for basic tests
- Can be added as a follow-up if tests require it

### 4. File Structure

```
pkg/api/
  stdlib_debug.go      # Debug library implementation
  stdlib_debug_test.go # Debug library tests

pkg/vm/
  vm.go                # Add getCallInfoAtLevel, expose CallInfo
  debug.go             # Debug helpers (optional, can be in vm.go)

pkg/codegen/
  codegen.go           # Populate LocVars in Prototype

pkg/object/
  value.go             # Add RegIndex to LocVar (if needed)
```

## Implementation Order

1. **Phase 1: Infrastructure**
   - Add `RegIndex` to `LocVar` (if needed)
   - Populate `LocVars` in codegen
   - Add `getCallInfoAtLevel` to VM
   - Expose necessary VM internals via API

2. **Phase 2: Core Functions**
   - Implement `debug.getinfo()`
   - Implement `debug.getlocal()`
   - Implement `debug.setlocal()`
   - Implement `debug.traceback()`

3. **Phase 3: Testing**
   - Run official Lua tests
   - Verify at least 5 additional tests pass

4. **Phase 4: Optional Features**
   - Implement `debug.debug()`
   - Implement `debug.sethook()` / `debug.gethook()` if needed

## Invariants

1. **CallInfo Consistency**: `vm.CallInfo[vm.CI]` always points to current frame
2. **LocVar Validity**: Each LocVar has Start <= End and valid RegIndex
3. **Stack Alignment**: `ci.Base + regIdx` must be within valid stack range
4. **Debug Safety**: Debug functions must not crash on invalid input

## Why Not Alternatives

### Why not store locals in a separate map?
- `LocVars` array is already defined in `Prototype`
- Matches Lua's reference implementation
- Efficient lookup by PC range

### Why not use reflection for debug info?
- Performance impact
- Doesn't match Lua's design
- Harder to maintain

### Why expose VM internals to debug library?
- Debug library by nature needs low-level access
- Lua's C API exposes similar internals
- Alternative (reflection) is slower and more complex

## Acceptance Criteria

1. `debug.getinfo()` returns valid table with `source`, `linedefined`, `lastlinedefined`, `what` fields
2. `debug.getlocal()` returns correct local variable names and values
3. `debug.traceback()` produces readable stack trace
4. At least 5 additional official Lua tests pass
5. All existing tests continue to pass
6. `go test ./...` succeeds

## Test Files Expected to Pass

After implementation:
- `db.lua` - Primary debug library tests
- `events.lua` - May pass with hook support
- `gc.lua` - May pass with debug info
- Additional tests that use debug functions