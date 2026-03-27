# Contract: goto.lua Complete Fix

## Mission
Fix ALL blockers in goto.lua to achieve end-to-end test pass.

## Verification Command
```bash
go test ./tests/ -run TestLuaTestSuite -timeout 120s -v 2>&1 | grep 'goto.lua'
# Expected: [pass]
```

---

## Blocker Inventory (from bisect analysis)

### BLOCKER 0: Prerequisite - stdlib_string.go syntax error
- **Status**: FIXED (backup in .backups/)
- **What was wrong**: Invalid rune literal with embedded newline at line 1107
- **Fix applied**: Corrected the escape sequence handling

### BLOCKER 1: Scope-end jump validation (Lines 80-102)
- **Observed error**: `jump into the scope of 'a'` (line 83) and `jump into the scope of 'x'` (line 99)
- **Test case**:
  ```lua
  do
    goto l1
    local a = 23
    ::l1::;  -- Label at END of block
  end
  ```
- **Current behavior**: ERROR (incorrectly rejects valid jump)
- **Expected**: VALID - jumping over local to end of block is allowed
- **Root cause**: `genLabel()` checks `currentLocals > gotoInfo.NumLocals` without considering scope boundaries
- **Fix location**: `pkg/codegen/stmt.go` `genLabel()` function

### BLOCKER 2: Label visibility after scope exit (Lines 40-41)
- **Observed error**: `assertion failed!` (errmsg expects "label 'l1'" error but doesn't get it)
- **Test case**:
  ```lua
  errmsg([[ do ::l1:: end goto l1 ]], "label 'l1'")
  ```
- **Expected**: ERROR "label 'l1'" (label inside do/end is invisible after scope ends)
- **Root cause**: Labels may not be properly removed from visibility when scope ends
- **Fix location**: `pkg/codegen/codegen.go` `endScope()` - verify label cleanup

### BLOCKER 3: Repeat-until scope checking (Lines 44-50)
- **Observed error**: `assertion failed!`
- **Test case**:
  ```lua
  errmsg([[
    repeat
      if x then goto cont end
      local xuxu = 10
      ::cont::
    until xuxu < x
  ]], "scope of 'xuxu'")
  ```
- **Expected**: ERROR "scope of 'xuxu'" (goto jumps into scope of local in repeat body)
- **Root cause**: Scope tracking in repeat-until may not correctly identify locals

### BLOCKER 4: Parser "expected 'end'" errors (Lines 100, 150, 250, 350)
- **Observed error**: `expected 'end', got 0`
- **Root cause**: Unknown - may be related to if/while/for block handling with goto
- **Investigation needed**: Run isolated tests to identify pattern

### BLOCKER 5: Nested function goto (Lines 108-120)
- **Test case**: Gotos inside local function with multiple labels
- **Status**: Unknown - need bisect after fixing blockers 1-4

### BLOCKER 6: Upvalue closing on goto (Lines 172+)
- **Test case**: Gotos that jump over closures with upvalues
- **Status**: Unknown - need bisect after fixing earlier blockers

### BLOCKER 7: To-be-closed variables with goto (Lines 230+)
- **Test case**: `global *` with `__close` metamethod and goto
- **Status**: Unknown - need bisect after fixing earlier blockers

### BLOCKER 8: Global declaration sections (Lines 340+)
- **Test case**: `global *` and `global<const>` declarations
- **Status**: Unknown - need bisect after fixing earlier blockers

---

## Core Invariant: Label Scope Validation

### The Correct Rule (from Lua manual)
> A goto may jump over a local variable declaration if the label is at or past the end of the local's scope.

### Current Implementation (WRONG)
```go
// In genLabel():
if currentLocals > gotoInfo.NumLocals {
    cg.setError("jump into the scope of '%s'", newLocals[0])
}
```
This rejects ANY jump that crosses a local declaration, even if the label is at scope end.

### Correct Implementation Approach
Track scope boundaries, not just local counts:

```go
// Data structure needed:
type ScopeBoundary struct {
    EndPC     int      // PC where this scope ends
    StartPC   int      // PC where this scope starts  
    Locals    []string // Locals declared in this scope
    Parent    *ScopeBoundary // Nil for top-level
}

// Check for each new local:
// Is the label PC < local's scope end PC?
// If yes: ERROR (label inside scope)
// If no: OK (label at/past scope end)
```

### Why Not Block Depth Alone?
Block depth is the same for:
- Label at END of block (valid jump target)
- Label in MIDDLE of block (invalid if local declared before)

We need the PC position to distinguish these cases.

---

## Label Visibility After Scope Exit

### Invariant
When `endScope()` runs, all labels defined in that scope must become INVISIBLE to subsequent gotos.

### Current Implementation
```go
// In endScope():
currentScopeLevel := len(cg.Locals)
for name, labelInfo := range cg.labels {
    if labelInfo.ScopeLevel > currentScopeLevel {
        delete(cg.labels, name)
    }
}
```

### Potential Issue
- `ScopeLevel` is set to `len(cg.Locals)` at label definition time
- After `endScope()`, `len(cg.Locals)` decreases
- The comparison `labelInfo.ScopeLevel > currentScopeLevel` should work
- **VERIFY**: Is `ScopeLevel` being set correctly in `genLabel()`?

---

## Iteration Protocol

Branch MUST follow this workflow:

```
1. BISECT at current failure line
   BISECT_FILE=goto.lua BISECT_LINE=<N> go test ./tests/ -run TestBisect -timeout 3s

2. DIAGNOSE the specific error
   - Run isolated test case if needed
   - Identify root cause in codegen/parser

3. FIX the identified issue
   - Make minimal targeted fix
   - Verify fix doesn't break earlier lines

4. RE-BISECT to verify progress
   - Re-run bisect at same line
   - If pass, advance to next failure line
   
5. REPEAT until line 450+ passes
```

### Progress Tracking
After each fix, update working memory with:
- Last passing bisect line
- Current failure line
- Error message at current failure

---

## Files to Modify

1. `pkg/codegen/stmt.go` - `genLabel()`, `genGoto()`
2. `pkg/codegen/codegen.go` - `endScope()`, scope tracking structures
3. `pkg/parser/stmt.go` - If parser issues found (BLOCKER 4)

---

## Non-Goals

- Do NOT modify VM
- Do NOT modify test files
- Do NOT change Lua semantics (follow Lua 5.4 spec)

---

## Acceptance Criteria

1. `go test ./tests/ -run TestLuaTestSuite -timeout 120s -v 2>&1 | grep 'goto.lua'` shows `[pass]`
2. All fixes committed to git
3. Working memory updated with final status