# Phase B: Bug Fix Design Document

## Overview

Three critical bugs in the Go-Lua VM need fixing. This document specifies the exact changes required, file by file, with code-level precision.

---

## Bug 1: Closure Upvalue Capture

### Problem
Nested functions cannot capture parent locals. `NewCodeGenerator()` creates isolated generators with no parent link. When a nested function references a parent local, it falls through to global lookup (`GETTABUP R(A), 0, K(name)`), but the nested prototype has zero `UpvalueDesc` entries, causing `"invalid upvalue index 0"`.

### Design

#### 1a. Add `Parent` field to `CodeGenerator` (`pkg/codegen/codegen.go`)

```go
type CodeGenerator struct {
    Prototype      *object.Prototype
    Parent         *CodeGenerator    // NEW: link to enclosing function's codegen
    PC             int
    StackTop       int
    MaxStackSize   int
    Upvalues       map[string]int
    Locals         [][]LocalVar
    Constants      map[string]int
    JumpList       []JumpEntry
    breakList      []int
}
```

#### 1b. Add `resolveUpvalue` method (`pkg/codegen/codegen.go`)

This method walks the parent chain (like PUC-Rio's `singlevaraux`):

```go
// resolveUpvalue resolves a variable as an upvalue by walking the parent chain.
// Returns the upvalue index in this generator's prototype and true if found.
func (cg *CodeGenerator) resolveUpvalue(name string) (int, bool) {
    if cg.Parent == nil {
        return -1, false
    }

    // Case 1: name is a local in the parent
    if idx, ok := cg.Parent.getLocal(name); ok {
        return cg.addUpvalue(name, object.UpvalueDesc{
            Index:   idx,
            IsLocal: true,
        }), true
    }

    // Case 2: name is already an upvalue in the parent
    if idx, ok := cg.Parent.getUpvalue(name); ok {
        return cg.addUpvalue(name, object.UpvalueDesc{
            Index:   idx,
            IsLocal: false,
        }), true
    }

    // Case 3: recurse — resolve in parent first, then reference parent's new upvalue
    if idx, ok := cg.Parent.resolveUpvalue(name); ok {
        return cg.addUpvalue(name, object.UpvalueDesc{
            Index:   idx,
            IsLocal: false,
        }), true
    }

    return -1, false
}
```

Note: `addUpvalue` already exists in `chunk.go` and deduplicates by name.

#### 1c. Wire parent in `genFunc` (`pkg/codegen/expr.go`, ~line 526)

```go
func (cg *CodeGenerator) genFunc(expr *parser.FuncExpr) int {
    nestedGen := NewCodeGenerator()
    nestedGen.Parent = cg  // NEW: link parent
    // ... rest unchanged
}
```

#### 1d. Wire parent in `genFuncDef` (`pkg/codegen/stmt.go`, ~line 432)

```go
func (cg *CodeGenerator) genFuncDef(stmt *parser.FuncDefStmt) {
    nestedGen := NewCodeGenerator()
    nestedGen.Parent = cg  // NEW: link parent
    // ...
}
```

**Critical for `local function fib(n)`**: The local must be registered in the parent scope BEFORE generating the nested body, so `resolveUpvalue("fib")` finds it:

```go
if stmt.IsLocal && len(stmt.Name) > 0 {
    // Register local BEFORE generating nested body so self-reference works
    reg := cg.allocRegister()
    cg.addLocal(stmt.Name[0].Name, reg, false)
    
    // Generate nested function with parent link
    nestedGen := NewCodeGenerator()
    nestedGen.Parent = cg
    // ... generate body ...
    
    // Emit CLOSURE into the pre-allocated register
    cg.EmitABx(vm.OP_CLOSURE, reg, protoIdx)
}
```

#### 1e. Update `genVar` to try upvalue resolution (`pkg/codegen/expr.go`, ~line 127)

Current flow: `getLocal` → `getUpvalue` → global fallthrough.
New flow: `getLocal` → `getUpvalue` → **`resolveUpvalue`** → global fallthrough.

```go
func (cg *CodeGenerator) genVar(expr *parser.VarExpr) int {
    reg := cg.allocRegister()
    if idx, ok := cg.getLocal(expr.Name); ok {
        cg.EmitABC(vm.OP_MOVE, reg, idx, 0)
    } else if upIdx, ok := cg.getUpvalue(expr.Name); ok {
        cg.EmitABC(vm.OP_GETUPVAL, reg, upIdx, 0)
    } else if upIdx, ok := cg.resolveUpvalue(expr.Name); ok {  // NEW
        cg.EmitABC(vm.OP_GETUPVAL, reg, upIdx, 0)              // NEW
    } else {
        // Global: GETTABUP R(A), 0, K(name)
        nameIdx := cg.addOrGetConstant(*object.NewString(expr.Name))
        cg.EmitABC(vm.OP_GETTABUP, reg, 0, nameIdx+256)
    }
    return reg
}
```

#### 1f. Update `assignToVar` for upvalue writes (`pkg/codegen/stmt.go`)

Same pattern in the `*parser.VarExpr` case of `assignToVar`:

```go
case *parser.VarExpr:
    if idx, ok := cg.getLocal(e.Name); ok {
        cg.EmitABC(vm.OP_MOVE, idx, valueReg, 0)
    } else if upIdx, ok := cg.getUpvalue(e.Name); ok {
        cg.EmitABC(vm.OP_SETUPVAL, valueReg, upIdx, 0)
    } else if upIdx, ok := cg.resolveUpvalue(e.Name); ok {  // NEW
        cg.EmitABC(vm.OP_SETUPVAL, valueReg, upIdx, 0)      // NEW
    } else {
        // Global
        nameIdx := cg.addOrGetConstant(*object.NewString(e.Name))
        cg.EmitABC(vm.OP_SETTABUP, 0, nameIdx+256, valueReg)
    }
```

#### 1g. VM OP_CLOSURE handler — already correct

The handler at `vm.go:887` already iterates `childProto.Upvalues` and handles both `IsLocal` (capture from stack) and `!IsLocal` (copy from parent closure). No changes needed here.

---

## Bug 2: Generic For Loop (ipairs)

### Problem
`for i,v in ipairs(t) do s=s+v end` — `s` stays 0.

### Root Cause Analysis

The `stdIpairs` iterator function uses a Go closure that captures `i` and increments it internally. When called via `FORGLOOP`:

1. `FORGLOOP` sets `vm.Base = vm.Base + a + 1` (so iterator sees state at stack index 1)
2. `vm.StackTop = vm.Base + 2` (2 args: state, control)
3. Iterator calls `L.PushNumber(float64(i))` and `L.vm.Push(*val)` — these push at `vm.StackTop` (absolute)
4. After call, `resultsTop = vm.StackTop` (which is now `oldBase + a + 3 + numResults`)
5. `firstResult = oldBase + a + 3`
6. `numResults = resultsTop - firstResult`

The stack math looks correct in theory. The actual issue is that the iterator function receives a `*State` wrapper created by `PushFunction`, and that wrapper calls `L.vm.GetStack(1)` which uses `vm.Base + 1 - 1 = vm.Base`. But `vm.Base` was set to `oldBase + a + 1` by FORGLOOP, so `GetStack(1)` returns `vm.Stack[oldBase + a + 1]` which is `R(A+1)` = the state (table). This is correct.

The issue is more subtle: **the iterator pushes results but `fn.GoFn(vm)` receives `vm` (the raw `*vm.VM`), not `L` (the `*State`)**. Looking at `PushFunction` in `api.go`:

```go
GoFn: func(vmInterface interface{}) error {
    vm, ok := vmInterface.(*vm.VM)
    // ...
    tempState := &State{vm: vm, global: s.global}
    nResults := fn(tempState)
    _ = nResults  // nResults is IGNORED!
```

The `nResults` return value from the Go function is discarded! The FORGLOOP handler calculates `numResults = resultsTop - (oldBase + a + 3)`. If the iterator pushes 2 values, `resultsTop` should be `oldBase + a + 3 + 2 = oldBase + a + 5`. Let's verify:

- Before call: `vm.StackTop = vm.Base + 2 = (oldBase + a + 1) + 2 = oldBase + a + 3`
- Iterator pushes 2 values: `vm.StackTop = oldBase + a + 5`
- `resultsTop = vm.StackTop = oldBase + a + 5`
- `firstResult = oldBase + a + 3`
- `numResults = 5 - 3 = 2` ✓

So the count is right. But wait — the iterator gets the table from `L.vm.GetStack(1)`. With `vm.Base = oldBase + a + 1`, `GetStack(1)` returns `vm.Stack[vm.Base + 0]` = `vm.Stack[oldBase + a + 1]` = R(A+1) = state. That's the table. ✓

Now the iterator does `t.GetI(i)` where `i` starts at 0 and increments to 1, 2, 3... But it also pushes `float64(i)` as the index. The control variable R(A+2) gets updated to the first result (the index). So R(A+2) = 1 after first iteration.

**Wait — the issue is in the `genForGeneric` codegen.** Let's look at the register allocation:

```go
baseReg := cg.StackTop  // e.g., 0
// funcReg = genExpr(funcExpr) -> allocates register, say reg 0
// argRegs = genExpr for state/control
// Then MOVE to baseReg, baseReg+1, baseReg+2
// Then:
for _, name := range stmt.Vars {
    reg := cg.allocRegister()  // allocates from current StackTop
    cg.addLocal(name.Name, reg, false)
}
```

The problem: after the MOVEs and freeRegisters, `cg.StackTop` may not be at `baseReg + 3`. The `freeRegister()` calls reduce StackTop. Let's trace:

1. `baseReg = cg.StackTop` (say 0)
2. `funcReg = cg.genExpr(funcExpr)` → allocates reg 0, StackTop=1
3. `argRegs[0] = cg.genExpr(args[0])` → allocates reg 1, StackTop=2
4. `MOVE baseReg(0), funcReg(0)` — noop, same reg
5. `cg.freeRegister()` → StackTop=1
6. `MOVE baseReg+1(1), argRegs[0](1)` — noop, same reg  
7. `cg.freeRegister()` → StackTop=0  ← **BUG! StackTop is now 0, not 3**

Then `allocRegister()` for loop vars starts at 0, not 3!

**The fix**: After setting up the 3 control registers, explicitly set `cg.StackTop = baseReg + 3` before allocating loop variable registers. Also ensure `MaxStackSize` is updated.

### Fix

In `genForGeneric` (`pkg/codegen/stmt.go`):

```go
// After moving iterator, state, control to baseReg, baseReg+1, baseReg+2:
// Ensure stack top is at baseReg+3 for loop variable allocation
cg.setStackTop(baseReg + 3)
if cg.StackTop > cg.MaxStackSize {
    cg.MaxStackSize = cg.StackTop
}

// Now allocate loop variables
for _, name := range stmt.Vars {
    reg := cg.allocRegister()  // Will be baseReg+3, baseReg+4, ...
    cg.addLocal(name.Name, reg, false)
}
```

Also, the `stdIpairs` iterator should read the control variable from the stack rather than using its own captured `i`, for correctness with FORGLOOP's control variable update. But since the Go closure increments `i` independently and FORGLOOP updates R(A+2) with the first result anyway, both stay in sync for sequential iteration. The captured `i` approach works but is fragile. **Fix**: read the control variable from the stack:

```go
L.PushFunction(func(L *State) int {
    // Get control variable from stack (arg 2 = control)
    ctrl := L.vm.GetStack(2)
    idx := 0
    if ctrl != nil && ctrl.IsNumber() {
        num, _ := ctrl.ToNumber()
        idx = int(num)
    }
    idx++ // next index
    
    // Get table from arg 1 (state)
    tv := L.vm.GetStack(1)
    t, ok := tv.ToTable()
    if !ok {
        L.PushNil()
        return 1
    }
    val := t.GetI(idx)
    if val == nil || val.IsNil() {
        L.PushNil()
        return 1
    }
    L.PushNumber(float64(idx))
    L.vm.Push(*val)
    return 2
})
```

---

## Bug 3: pcall Returns Function Instead of Bool

### Problem
`stdPcall` only handles `fn.IsGo`. When a Lua closure is passed, the `if fn.IsGo` block is skipped, leaving the function on the stack. The success path then collects whatever is on the stack (the original function + args).

### Design

#### 3a. Add `ProtectedCall` to VM (`pkg/vm/vm.go`)

```go
// ProtectedCall executes a function in protected mode with a nested Run loop.
// It saves/restores VM state and catches panics.
// funcIdx is relative to vm.Base.
func (vm *VM) ProtectedCall(funcIdx, nargs, nresults int) error {
    // Save VM state
    savedCI := vm.CI
    savedBase := vm.Base
    savedPC := vm.PC
    savedStackTop := vm.StackTop
    savedProto := vm.Prototype
    
    // Set up the call (pushes new CallInfo)
    if err := vm.Call(funcIdx, nargs, nresults); err != nil {
        // Restore on error
        vm.CI = savedCI
        vm.Base = savedBase
        vm.PC = savedPC
        vm.StackTop = savedStackTop
        vm.Prototype = savedProto
        return err
    }
    
    // Run nested execution loop until CI drops back to savedCI level
    for vm.CI > savedCI {
        if vm.PC >= len(vm.Prototype.Code) {
            break
        }
        instr := Instruction(vm.Prototype.Code[vm.PC])
        vm.PC++
        if err := vm.ExecuteInstruction(instr); err != nil {
            // Restore on error
            vm.CI = savedCI
            vm.Base = savedBase
            vm.PC = savedPC
            vm.StackTop = savedStackTop
            vm.Prototype = savedProto
            return err
        }
    }
    
    // Restore caller's execution context (PC, Prototype stay from RETURN handler)
    // But we need to restore PC and Prototype for the CALLER to continue
    vm.PC = savedPC
    vm.Prototype = savedProto
    
    return nil
}
```

**Key insight**: The nested `Run` loop must stop when `vm.CI` drops back to `savedCI` (meaning the called function returned). The `OP_RETURN` handler already decrements `vm.CI` and restores `vm.Base`, `vm.PC`, `vm.Prototype` from the CallInfo stack. But we need to make sure `ProtectedCall` restores the outer caller's state correctly after the nested function completes.

#### 3b. Update `stdPcall` (`pkg/api/stdlib.go`)

Add an `else` branch for Lua functions:

```go
fn, _ := funcVal.ToFunction()
if fn.IsGo {
    // ... existing Go function handling (unchanged)
} else {
    // Lua function: use ProtectedCall
    // Push function and args onto stack
    L.vm.SetTop(0)
    L.vm.Push(funcVal)
    for _, arg := range args {
        L.vm.Push(arg)
    }
    
    // Call with protected execution
    err := L.vm.ProtectedCall(0, len(args), -1)
    if err != nil {
        panic(err)  // Will be caught by the defer/recover above
    }
}
```

**Note**: The `ProtectedCall` approach means the Lua function executes synchronously within the Go `stdPcall` function. Results end up on the stack. The existing success path then collects them.

---

## File Change Summary

| File | Changes |
|------|---------|
| `pkg/codegen/codegen.go` | Add `Parent *CodeGenerator` field; add `resolveUpvalue` method |
| `pkg/codegen/expr.go` | Wire `Parent` in `genFunc`; add `resolveUpvalue` call in `genVar` |
| `pkg/codegen/stmt.go` | Wire `Parent` in `genFuncDef`; reorder local registration for `local function`; add `resolveUpvalue` in `assignToVar`; fix `genForGeneric` stack top |
| `pkg/vm/vm.go` | Add `ProtectedCall` method |
| `pkg/api/stdlib.go` | Fix `stdPcall` Lua function branch; fix `stdIpairs` to use control variable |

## Testing

A diagnostic Go test should verify all 6 acceptance criteria. All existing tests must continue to pass.