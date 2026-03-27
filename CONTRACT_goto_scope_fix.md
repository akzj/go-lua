# Contract: Goto Scope Validation Fix

## Problem Statement

The goto scope validation incorrectly rejects valid jumps over local declarations to labels at the end of a block.

**Failing test case:**
```lua
do
  goto l1
  local a = 23
  ::l1::;
end
```

**Current behavior:** Error "jump into the scope of 'a'"
**Expected behavior:** Valid - jumping over local to end of block is allowed in Lua

## Root Cause Analysis

The current implementation in `genLabel()` checks:
```go
if currentLocals > gotoInfo.NumLocals {
    // Error: jump into scope
}
```

This is **WRONG** because it doesn't account for scope boundaries. A local variable declared between a goto and its target label is only "jumped into" if the label is WITHIN the local's scope, not at the END of it.

## Lua Semantics

In Lua:
- **VALID:** `goto label` jumping over `local x` to `::label::` at end of block
  - The local's scope ends at the same point as the label
  - Code after label cannot access the local
  
- **INVALID:** `goto label` jumping into scope of `local x`
  - Label is in the middle of block
  - Code after label could access the local

## Solution Design

### Invariant
```
A goto can jump over a local declaration if and only if:
  The label is at or past the end of the local's scope
```

### Data Structure Changes

```go
// ScopeEnd records where a scope ends
type ScopeEnd struct {
    EndPC    int      // PC where scope ends
    Locals   []string // Names of locals going out of scope
}

// Add to CodeGenerator:
scopeEnds []ScopeEnd  // Track scope boundaries
```

### Algorithm

1. In `endScope()`:
   - Record current PC as scope end
   - Record which locals are going out of scope
   
2. In `genLabel()` when checking forward gotos:
   - For each new local (local declared after goto):
     - Find the scope it belongs to
     - Check if label PC < scope end PC
     - If yes: ERROR (label is inside local's scope)
     - If no: OK (label is at/past scope end)

### Why Not Just Check Block Depth?

Block depth alone is insufficient because:
- A label at the end of a block has the same blockDepth as a label in the middle
- We need to know WHERE in the block the label is positioned
- Scope end PC gives us the exact boundary

## Implementation Contract

### Function: `recordScopeEnd(locals []string)`
- Called at the END of `endScope()`
- Records current PC and locals going out of scope
- Invariant: `len(scopeEnds)` increases by 1

### Function: `isLabelInLocalScope(localName string, labelPC int) bool`
- Returns true if label is INSIDE the local's scope
- Finds the scope containing the local
- Returns `labelPC < scopeEnd.EndPC`

### Modified `genLabel()` Logic
```go
// When checking forward gotos:
for _, localName := range newLocals {
    if cg.isLabelInLocalScope(localName, currentPC) {
        cg.setError("jump into the scope of '%s'", localName)
        return
    }
}
// If we get here, all new locals have scope ending at or before label
// This is VALID - do not error
```

## Test Cases

| Code | Valid? | Reason |
|------|--------|--------|
| `goto l; local a; ::l::` | YES | Label at scope end |
| `goto l; local a; ::l:: print(a)` | NO | Label uses local |
| `goto l; do local a; end ::l::` | YES | Local in inner scope |
| `do local a; ::l:: end goto l` | NO | Label inside inner block |

## Files to Modify

1. `pkg/codegen/codegen.go`:
   - Add `scopeEnds []ScopeEnd` field
   - Add `recordScopeEnd()` method
   - Add `isLabelInLocalScope()` method

2. `pkg/codegen/stmt.go`:
   - Modify `endScope()` to call `recordScopeEnd()`
   - Modify `genLabel()` to use `isLabelInLocalScope()`

## Non-Goals

- Do not change the parser
- Do not change the VM
- Do not modify backward goto handling (already correct)