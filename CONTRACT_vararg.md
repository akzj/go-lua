# Vararg Implementation Contract

## Problem
`OP_VARARG` always returns nil, breaking all variadic function calls like `checkerror(msg, f, ...)`.

## Root Cause
1. `CallInfo` doesn't track varargs (where they start, how many)
2. `OP_VARARGPREP` is a no-op
3. `OP_VARARG` just sets nil instead of copying varargs

## Stack Layout for Vararg Functions

When a function `f(a, b, ...)` is called with `f(1, 2, 3, 4, 5)`:
```
Stack:
  [Base]     = 'a' (param 1) = 1
  [Base+1]   = 'b' (param 2) = 2
  [Base+2]   = vararg[0] = 3
  [Base+3]   = vararg[1] = 4
  [Base+4]   = vararg[2] = 5
```

- `NumParams = 2` (fixed params a, b)
- `VarargBase = Base + NumParams = Base + 2`
- `VarargCount = nargs - NumParams = 5 - 2 = 3`

## Implementation Contract

### 1. CallInfo Extension
```go
type CallInfo struct {
    // ... existing fields ...
    VarargBase  int  // Stack index where varargs start (Base + NumParams)
    VarargCount int  // Number of varargs (nargs - NumParams, if > 0)
}
```

**Invariant**: `VarargCount >= 0` always. If `nargs <= NumParams`, `VarargCount = 0`.

### 2. OP_VARARGPREP Implementation
```go
case OP_VARARGPREP:
    // A = number of fixed parameters
    // Calculate varargs: nargs - A
    // Store in current CallInfo
```

**Why not calculate at call time?** The caller doesn't know if callee is vararg. Callee must set up varargs after checking its prototype.

### 3. OP_VARARG Implementation
```go
case OP_VARARG:
    // VARARG R(A), C
    // Copy C-1 varargs to R(A), R(A+1), ...
    // C=0 means copy all varargs
    a, c := instr.A(), instr.C()
    numWanted := c - 1  // C=0 means all (numWanted = -1)
    
    ci := vm.CallInfo[vm.CI]
    available := ci.VarargCount
    
    if numWanted < 0 {
        numWanted = available  // Copy all
    }
    
    // Copy varargs to destination
    for i := 0; i < numWanted; i++ {
        if i < available {
            vm.Stack[vm.Base+a+i].CopyFrom(&vm.Stack[ci.VarargBase+i])
        } else {
            vm.Stack[vm.Base+a+i].SetNil()
        }
    }
```

**Invariant**: `vm.Base + a + numWanted` must not exceed current stack frame.

### 4. OP_CALL Modification
When setting up CallInfo for Lua function:
```go
// Calculate varargs
varargCount := nargs - proto.NumParams
if varargCount < 0 {
    varargCount = 0
}

vm.CallInfo[vm.CI] = &CallInfo{
    // ... existing fields ...
    VarargBase:  newBase + proto.NumParams,
    VarargCount: varargCount,
}
```

**Why not use OP_VARARGPREP?** OP_VARARGPREP is at function entry. But we need vararg info available before executing VARARGPREP for the edge case where varargs are used before VARARGPREP (though Lua typically puts VARARGPREP first).

**Decision**: Set VarargBase/VarargCount in OP_CALL, OP_VARARGPREP becomes a no-op (already is). This is simpler and correct.

## Why Not Alternative Approaches

1. **Store varargs separately**: Would require copying, inefficient.
2. **Calculate on demand**: Would need to track nargs in CallInfo anyway.
3. **Use stack markers**: Complex, error-prone.

The chosen approach: track vararg position/count in CallInfo, varargs stay in place on stack.

## Testing
After fix, `checkerror("out of range", string.char, 256)` should work:
1. `checkerror` receives `... = (256,)` as varargs
2. `pcall(f, ...)` expands varargs
3. `string.char(256)` gets called with 256
4. Error message contains "out of range"