# Phase D: Metatables and Polish - Design Document

## Overview

Phase D addresses three major areas:
1. String concatenation bug fix
2. Metatables with metamethods
3. CLI improvements (REPL, error messages)

## 1. String Concatenation Bug Fix

### Problem Analysis

The CONCAT instruction in Lua 5.4 has semantics: `CONCAT A B C` concatenates values in registers R[B] through R[C] and stores the result in R[A].

**Current Behavior (Broken):**
For `t[1] .. t[2] .. t[3]`, the codegen emits:
```
GETTABLE R3, R1, R2    ; t[1]
GETTABLE R6, R4, R5    ; t[2]
GETTABLE R9, R7, R8    ; t[3]
CONCAT R10, R6, R9     ; Wrong! Concatenates R6..R9 (non-contiguous, includes garbage)
CONCAT R9, R3, R10     ; Wrong! Includes previous result
```

**Root Cause:**
The `genArithmetic` function treats `..` like any binary operator, emitting one CONCAT per binary node. This fails because:
1. The parser creates left-associative tree: `BinOp("..", BinOp("..", t[1], t[2]), t[3])`
2. Each CONCAT is emitted with the wrong register range
3. Intermediate results corrupt the output

**Solution:**
Implement `genConcat` function that:
1. Flattens the `..` chain to collect all operands
2. Evaluates each operand into consecutive registers
3. Emits a single CONCAT instruction with the correct range

### Implementation

```go
// genConcat generates code for concatenation operator.
// It flattens the .. chain and emits a single CONCAT instruction.
func (cg *CodeGenerator) genConcat(expr *parser.BinOpExpr) int {
    // Collect all operands in the .. chain
    operands := cg.collectConcatOperands(expr)
    
    // Evaluate each operand into consecutive registers
    startReg := cg.StackTop
    for _, operand := range operands {
        reg := cg.genExpr(operand)
        // If not in the right position, move it
        expectedReg := startReg + len(evals)
        if reg != expectedReg {
            cg.EmitABC(vm.OP_MOVE, expectedReg, reg, 0)
        }
    }
    
    // Emit single CONCAT instruction
    resultReg := cg.allocRegister()
    endReg := startReg + len(operands) - 1
    cg.EmitABC(vm.OP_CONCAT, resultReg, startReg, endReg)
    
    // Free operand registers
    cg.setStackTop(startReg)
    
    return resultReg
}

// collectConcatOperands recursively collects all operands in a .. chain
func (cg *CodeGenerator) collectConcatOperands(expr parser.Expr) []parser.Expr {
    if binOp, ok := expr.(*parser.BinOpExpr); ok && binOp.Op == ".." {
        left := cg.collectConcatOperands(binOp.Left)
        right := cg.collectConcatOperands(binOp.Right)
        return append(left, right...)
    }
    return []parser.Expr{expr}
}
```

### Files to Modify
- `pkg/codegen/expr.go`: Add `genConcat` and `collectConcatOperands` functions, modify `genBinOp` to use it

## 2. Metatables Implementation

### Architecture

Metatables require changes across multiple packages:

```
pkg/object/types.go    - Add Metatable field to Table type
pkg/table/table.go     - Metatable get/set methods
pkg/vm/vm.go           - Metamethod lookup and invocation
pkg/api/stdlib.go      - setmetatable, getmetatable functions
```

### TValue and Table Changes

```go
// In pkg/object/types.go
type Table struct {
    array []TValue
    hash  map[string]TValue
    Metatable *Table  // NEW: metatable reference
}

// Metamethod constants
const (
    MetaIndex    = "__index"
    MetaNewIndex = "__newindex"
    MetaCall     = "__call"
    MetaAdd      = "__add"
    MetaSub      = "__sub"
    MetaMul      = "__mul"
    MetaDiv      = "__div"
    MetaMod      = "__mod"
    MetaPow      = "__pow"
    MetaUnm      = "__unm"
    MetaEq       = "__eq"
    MetaLt       = "__lt"
    MetaLe       = "__le"
    MetaToString = "__tostring"
    MetaConcat   = "__concat"
    MetaLen      = "__len"
    MetaPairs    = "__pairs"
    MetaIPairs   = "__ipairs"
)
```

### VM Metamethod Handling

For each operation that supports metamethods:

1. **GETTABLE (index access)**:
   - If key exists in table, return value
   - Else if metatable has `__index`:
     - If `__index` is table, recursively index it
     - If `__index` is function, call it with (table, key)

2. **SETTABLE (index assignment)**:
   - If key exists in table, assign value
   - Else if metatable has `__newindex`:
     - If `__newindex` is table, assign to it
     - If `__newindex` is function, call it with (table, key, value)

3. **Arithmetic operations**:
   - Check if either operand has metatable with corresponding metamethod
   - If found, call metamethod with operands

4. **Comparison operations**:
   - Check if both operands have same metatable with corresponding metamethod
   - If found, call metamethod with operands

5. **CALL (function call on table)**:
   - If table has metatable with `__call`, invoke it

### Stdlib Functions

```go
// setmetatable(table, metatable) -> table
func setmetatable(L *api.State) int {
    // Get table and metatable from stack
    // Set metatable on table
    // Return table
}

// getmetatable(table) -> metatable
func getmetatable(L *api.State) int {
    // Get table from stack
    // Return its metatable
}
```

### Implementation Order

1. Add `Metatable` field to `Table` struct
2. Implement `setmetatable` and `getmetatable` stdlib functions
3. Implement `__index` metamethod in GETTABLE
4. Implement `__newindex` metamethod in SETTABLE
5. Implement `__call` metamethod in CALL
6. Implement arithmetic metamethods
7. Implement comparison metamethods
8. Implement `__tostring`, `__concat`, `__len`
9. Implement `__pairs`, `__ipairs`

## 3. CLI Improvements

### REPL Mode

When `lua` is invoked without a file argument:
1. Print welcome banner
2. Enter read-eval-print loop:
   - Read line (handle multi-line for incomplete statements)
   - Compile and execute
   - Print result
3. Handle Ctrl+D for exit

### Error Messages with Line Numbers

1. Parser already tracks line numbers - ensure they're in error messages
2. Add source line context to runtime errors
3. Include function name in stack traces

### Implementation

```go
// In cmd/lua/main.go
func runREPL() {
    scanner := bufio.NewScanner(os.Stdin)
    L := api.NewState()
    L.OpenLibs()
    
    for {
        print("> ")
        if !scanner.Scan() {
            break
        }
        line := scanner.Text()
        
        // Handle multi-line input
        for !isComplete(line) {
            print(">> ")
            if !scanner.Scan() {
                break
            }
            line += "\n" + scanner.Text()
        }
        
        // Execute
        if err := L.DoString(line, "stdin"); err != nil {
            println(err.Error())
        }
    }
}

func isComplete(code string) bool {
    // Try to parse - if it fails with "unexpected EOF", it's incomplete
    l := lexer.NewLexer([]byte(code), "check")
    p := parser.NewParser(l)
    _, err := p.ParseChunk()
    if err == nil {
        return true
    }
    // Check for incomplete statement indicators
    return !strings.Contains(err.Error(), "unexpected EOF")
}
```

## Test Strategy

1. **String Concatenation Tests**:
   - `local t = {1, 2, 3}; return t[1] .. t[2] .. t[3]` → "123"
   - `"a" .. "b" .. "c"` → "abc"
   - Mixed types with `__concat` metamethod

2. **Metatable Tests**:
   - `setmetatable(t, {__index = {x = 10}}); return t.x` → 10
   - `setmetatable(t, {__newindex = {}}); t.x = 5; return t.x` → nil
   - `setmetatable(t, {__call = function() return 42 end}); return t()` → 42
   - Arithmetic: `setmetatable(t, {__add = ...})`
   - Comparison: `setmetatable(t, {__eq = ...})`

3. **CLI Tests**:
   - Run `./lua` without args → REPL prompt
   - Multi-line input: `function f() \n return 1 \n end`
   - Error messages include line numbers

## Constraints

- All existing tests must pass
- Follow Lua 5.4 semantics for metamethods
- No regression in existing functionality