# Phase F: Official Test Suite Fixes - Design Document v2

## Critical Bug Found: Instruction Encoding Mismatch

### Root Cause Analysis

The instruction encoding in our VM doesn't match Lua 5.4's actual format. Looking at Lua 5.4's `lopcodes.h`:

```
OP_GETTABUP,/* A B C   R[A] := UpValue[B][K[C]:shortstring]        */
OP_SETTABUP,/* A B C   UpValue[A][K[B]:shortstring] := RK(C)       */
OP_GETFIELD,/* A B C   R[A] := R[B][K[C]:shortstring]              */
OP_SETFIELD,/* A B C   R[A][K[B]:shortstring] := RK(C)             */
```

**Key insight**: 
- `K[X]` means X is ALWAYS a constant index (0-255)
- `RK(X)` means X uses RK mode (0-255 = register, 256-511 = constant)

### Current (Wrong) Implementation

**Codegen** (`pkg/codegen/stmt.go`):
```go
// Global: SETTABUP 0, K(name), RK(value)
nameIdx := cg.addOrGetConstant(*object.NewString(e.Name))
cg.EmitABC(vm.OP_SETTABUP, 0, nameIdx+256, valueReg)  // WRONG!
```

**VM** (`pkg/vm/vm.go`):
```go
case OP_SETTABUP:
    key := vm.getRKValue(b)  // WRONG! B is not RK mode
    val := vm.getRKValue(c)  // Correct
```

### Correct Implementation

For **SETTABUP** `UpValue[A][K[B]] := RK(C)`:
- A = upvalue index (0 for _ENV)
- B = constant index (directly, no +256)
- C = value (RK mode: 0-255 = register, 256-511 = constant)

For **GETTABUP** `R[A] := UpValue[B][K[C]]`:
- A = destination register
- B = upvalue index (0 for _ENV)
- C = constant index (directly, no +256)

## Files to Fix

### 1. `pkg/codegen/stmt.go` - `assignToVar`

Current:
```go
// Global: SETTABUP 0, K(name), RK(value)
nameIdx := cg.addOrGetConstant(*object.NewString(e.Name))
cg.EmitABC(vm.OP_SETTABUP, 0, nameIdx+256, valueReg)
```

Fixed:
```go
// Global: SETTABUP A=0, B=constIdx, C=RK(value)
nameIdx := cg.addOrGetConstant(*object.NewString(e.Name))
// B is constant index directly (K[B] format)
// C is RK mode - add 256 if value is in a register, or use directly if constant
cg.EmitABC(vm.OP_SETTABUP, 0, nameIdx, valueReg)  // valueReg is already a register
```

### 2. `pkg/codegen/expr.go` - `genVar`

Current:
```go
// Global: GETTABUP R(A), 0, K(name) — get from UpValue[0][K(name)]
nameIdx := cg.addOrGetConstant(*object.NewString(expr.Name))
cg.EmitABC(vm.OP_GETTABUP, reg, 0, nameIdx+256)
```

Fixed:
```go
// Global: GETTABUP R(A), 0, K(C) — R(A) := UpValue[0][K(C)]
nameIdx := cg.addOrGetConstant(*object.NewString(expr.Name))
cg.EmitABC(vm.OP_GETTABUP, reg, 0, nameIdx)  // C is constant index directly
```

### 3. `pkg/vm/vm.go` - `OP_SETTABUP`

Current:
```go
key := vm.getRKValue(b)  // Wrong
val := vm.getRKValue(c)  // Correct
```

Fixed:
```go
// B is constant index directly (K[B] format)
key := &vm.Prototype.Constants[b]
// C is RK mode
val := vm.getRKValue(c)
```

### 4. `pkg/vm/vm.go` - `OP_GETTABUP`

Current:
```go
key := vm.getRKValue(c)  // Wrong
```

Fixed:
```go
// C is constant index directly (K[C] format)
key := &vm.Prototype.Constants[c]
```

### 5. Similar fixes for SETFIELD/GETFIELD

Same pattern applies - check opcode definition and use correct encoding.

## Implementation Order

1. **Fix VM instruction handling** (pkg/vm/vm.go)
   - OP_SETTABUP: B = constant, C = RK
   - OP_GETTABUP: C = constant
   - OP_SETFIELD: B = constant, C = RK
   - OP_GETFIELD: C = constant

2. **Fix codegen** (pkg/codegen/stmt.go, expr.go)
   - Remove +256 offset for constant indices in K[] operands
   - Keep +256 offset for RK operands (C field in SET* instructions)

3. **Add require function** (pkg/api/stdlib.go)
   - Simple implementation returning empty table

4. **Test and verify**
   - Run `x = 42; print(x)`
   - Run `function f() end; f()`
   - Run official test suite

## Acceptance Criteria

1. ✓ `x = 42; print(x)` prints 42
2. ✓ `function f() print("hello") end; f()` works
3. ✓ `require "debug"` returns a table
4. ✓ At least 10 official test files pass
5. ✓ All existing tests still pass