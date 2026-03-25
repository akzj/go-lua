# Pipeline Wiring Design: Parser→Codegen→VM

## Overview

This document specifies the exact changes needed to wire the full compilation pipeline
and implement missing VM opcodes. It is the authoritative spec for all branch tasks.

---

## 1. Parser→Codegen Wiring

### Current State
- `Parser.Parse()` returns an empty `object.Prototype` with `Code: []object.Instruction{}`
- The codegen package exists and works for individual AST constructs
- The codegen is never called from the parser

### Target Design

**`pkg/parser/parser.go` changes:**

The `Parse()` method must:
1. Parse the chunk into a `*BlockStmt` (already done)
2. Create a `codegen.CodeGenerator` for the main chunk
3. Set up the _ENV upvalue as upvalue index 0
4. Call `cg.genBlock(body)` to generate bytecode
5. Emit a trailing RETURN instruction
6. Return `cg.Prototype`

**Key**: The codegen's `GenerateChunk(block *parser.BlockStmt, source string) *object.Prototype`
is a NEW method we add to `CodeGenerator` that:
- Sets `IsVarArg = true` (main chunk is always vararg)
- Adds `_ENV` as upvalue index 0 with `UpvalueDesc{Index: 0, IsLocal: true}`
- Calls `beginScope()`, `genBlock()`, `emitReturn(0, 1)`, `endScope()`
- Sets `MaxStackSize`

**Import**: parser.go will import `codegen` package. This is fine since codegen already
imports parser (for AST types), but parser imports codegen only for the top-level call.
Wait - this creates a **circular dependency**! parser imports codegen, codegen imports parser.

**Solution**: Move the wiring to `pkg/api/load.go` instead. The parser returns the AST
(the `*BlockStmt`), and `LoadString` calls codegen on it. This avoids circular deps.

**Revised approach:**
- `Parser.Parse()` returns `(*parser.BlockStmt, error)` instead of `(*object.Prototype, error)`
- `LoadString` in `pkg/api/load.go` does: parse → codegen → closure
- OR: `Parser.Parse()` still returns `*object.Prototype` but we add a `Compile` function
  in the codegen package that the parser calls... but that's still circular.

**Final approach**: Add a `CompileChunk` function in `pkg/codegen/codegen.go`:
```go
func CompileChunk(block *parser.BlockStmt, source string) *object.Prototype
```

And call it from `pkg/api/load.go` after parsing. The parser's `Parse()` method
will return `(*parser.BlockStmt, error)` - we change its signature. But this breaks
existing tests that expect `*object.Prototype`.

**Simplest non-breaking approach**: Keep `Parse()` returning `*object.Prototype` but
have `LoadString` call codegen directly. Parse() returns the AST wrapped in a way
that LoadString can detect and use codegen on.

**ACTUALLY SIMPLEST**: Modify `LoadString` to:
1. Create lexer + parser
2. Call `p.ParseChunk()` (new exported method) which returns `*parser.BlockStmt`
3. Call `codegen.CompileChunk(block, name)` which returns `*object.Prototype`
4. Create closure from prototype

We add `ParseChunk()` as a new exported method on Parser (the existing `parseChunk()`
just needs to be exported or wrapped). The existing `Parse()` can remain for backward
compat (tests call it). We also add `CompileChunk()` to codegen.

---

## 2. _ENV and Global Variable Access

### Current State
- Codegen uses `OP_GETTABLE R(A), 0, R(key)` for globals (simplified, broken)
- No _ENV upvalue is set up

### Target Design

**Codegen changes for global access:**
- The main chunk's upvalue[0] is `_ENV`
- Global variable READ: `GETTABUP R(A), 0, K(name)` — get from UpValue[0][K(name)]
- Global variable WRITE: `SETTABUP 0, K(name), RK(value)` — set UpValue[0][K(name)]

In `codegen/expr.go` `genVar()`:
```go
// Instead of the current broken GETTABLE approach:
if idx, ok := cg.getLocal(expr.Name); ok {
    cg.EmitABC(vm.OP_MOVE, reg, idx, 0)
} else if upIdx, ok := cg.getUpvalue(expr.Name); ok {
    cg.EmitABC(vm.OP_GETUPVAL, reg, upIdx, 0)
} else {
    // Global: GETTABUP R(A), 0, K(name)
    nameIdx := cg.addOrGetConstant(*object.NewString(expr.Name))
    cg.EmitABC(vm.OP_GETTABUP, reg, 0, nameIdx)
}
```

In `codegen/stmt.go` `assignToVar()` for globals:
```go
// Instead of SETTABLE:
nameIdx := cg.addOrGetConstant(*object.NewString(e.Name))
cg.EmitABC(vm.OP_SETTABUP, 0, nameIdx, valueReg)
```

**LoadString changes for _ENV setup:**
After creating the closure, set its first upvalue to point to the global table:
```go
closure.Upvalues = []*object.Upvalue{
    {Value: globalTableTValue, Closed: false},
}
```

The global table is `s.global.Registry` (the registry IS the global table in this impl).

---

## 3. Missing VM Opcodes

All opcodes go in `pkg/vm/vm.go` `ExecuteInstruction` switch.

### OP_GETUPVAL (7): R(A) := UpValue[B]
```
Get the current closure's upvalue at index B, put in R(A).
The closure is stored in the current CallInfo.
```

### OP_SETUPVAL (8): UpValue[B] := R(A)
```
Set the current closure's upvalue at index B to R(A).
```

### OP_GETTABUP (9): R(A) := UpValue[B][K(C)]
```
Get upvalue[B] (must be a table), index it with constant K(C).
This is the primary mechanism for global variable access.
```

### OP_SETTABUP (13): UpValue[A][K(B)] := RK(C)
```
Get upvalue[A] (must be a table), set key K(B) to RK(C).
This is the primary mechanism for global variable assignment.
```

### OP_CALL (58): R(A)(R(A+1), ..., R(A+B-1))
```
Call function at R(A) with B-1 arguments (B=0 means use top).
C-1 results expected (C=0 means all results).

For Go functions:
  1. Get closure from R(A)
  2. Call closure.GoFn with VM
  3. Handle results

For Lua functions:
  1. Save current PC and prototype in CallInfo
  2. Push new CallInfo
  3. Set Base = A+1, PC = 0, Prototype = closure.Proto
  4. Continue execution (the Run loop will execute the new function)
```

### OP_FORPREP (48): Prepare numeric for loop
```
R(A) = initial value, R(A+1) = limit, R(A+2) = step
R(A+3) = loop variable (external)

Subtract step from initial (so first FORLOOP iteration adds it back).
Jump forward by sBx to the FORLOOP instruction.
```

### OP_FORLOOP (49): Numeric for loop iteration
```
R(A) += R(A+2)  (add step)
if step > 0: if R(A) <= R(A+1) then { R(A+3) = R(A); pc += sBx (jump back) }
if step < 0: if R(A) >= R(A+1) then { R(A+3) = R(A); pc += sBx (jump back) }
```

### OP_FORGPREP (50): Prepare generic for loop
```
Just jump forward by sBx to the FORGLOOP check.
```

### OP_FORGLOOP (51): Generic for loop iteration
```
Call R(A)(R(A+1), R(A+2)) — the iterator function
If first result is not nil:
  R(A+2) = first result (update control variable)
  Copy results to loop variables R(A+3), R(A+4), ...
  Jump back by sBx
```

### OP_SETLIST (52): R(A)[C+i] := R(A+i), 1 <= i <= B
```
Initialize table array part. B is count, C is offset.
If B == 0, use everything from R(A+1) to top.
```

### OP_CLOSURE (53): R(A) := closure(KPROTO[Bx])
```
Create a new closure from the Bx-th child prototype.
Set up upvalues based on the prototype's UpvalueDesc:
  - IsLocal=true: capture from current stack
  - IsLocal=false: copy from current closure's upvalues
```

### OP_VARARG (54): Copy varargs to registers
```
Copy C-1 varargs to R(A), R(A+1), ... (C=0 means all).
Varargs are stored below the function's fixed parameters.
```

### OP_VARARGPREP (55): Prepare vararg function
```
A = number of fixed parameters.
Move fixed params to their correct positions.
Store excess args as varargs.
```

---

## 4. Closure and Upvalue Architecture

### VM needs a "current closure" reference

Currently the VM has `Prototype` but not the closure. We need the closure to access upvalues.

**Change**: Add `Closure *object.Closure` to `CallInfo` struct, and track the current closure.

When executing OP_GETUPVAL/OP_SETUPVAL/OP_GETTABUP/OP_SETTABUP, get the closure from
`vm.CallInfo[vm.CI].Closure` (or a cached `vm.CurrentClosure`).

### CallInfo changes
```go
type CallInfo struct {
    Func      *object.TValue
    Closure   *object.Closure  // NEW: direct reference to closure
    Base      int
    Top       int
    PC        int
    NResults  int
    Status    CallStatus
}
```

---

## 5. Standard Library Functions

New file: `pkg/api/stdlib.go`

```go
func (s *State) OpenLibs() {
    s.registerBaseLib()
}

func (s *State) registerBaseLib() {
    // Register in global table
    s.Register("print", luaPrint)
    s.Register("type", luaType)
    s.Register("tostring", luaTostring)
    s.Register("tonumber", luaTonumber)
    s.Register("assert", luaAssert)
    s.Register("error", luaError)
    s.Register("pcall", luaPcall)
    s.Register("pairs", luaPairs)
    s.Register("ipairs", luaIpairs)
    s.Register("next", luaNext)
    s.Register("select", luaSelect)
    s.Register("unpack", luaUnpack)
}
```

Each function is a `func(L *State) int` that reads args from stack and pushes results.

---

## 6. CLI Entry Point

New file: `cmd/lua/main.go`

```go
func main() {
    L := api.NewState()
    defer L.Close()
    L.OpenLibs()
    
    if len(os.Args) > 1 {
        // Execute file
        if err := L.DoFile(os.Args[1]); err != nil {
            fmt.Fprintf(os.Stderr, "%v\n", err)
            os.Exit(1)
        }
    } else {
        // Read from stdin
        data, _ := io.ReadAll(os.Stdin)
        if err := L.DoString(string(data), "stdin"); err != nil {
            fmt.Fprintf(os.Stderr, "%v\n", err)
            os.Exit(1)
        }
    }
}
```

---

## 7. Integration: How LoadString Works After Changes

```
LoadString(code, name):
  1. l = lexer.NewLexer(code, name)
  2. p = parser.NewParser(l)
  3. block, err = p.ParseChunk()  // returns *parser.BlockStmt
  4. proto = codegen.CompileChunk(block, name)  // returns *object.Prototype
  5. closure = &object.Closure{Proto: proto}
  6. closure.Upvalues = []*object.Upvalue{{Value: &globalTableTValue}}  // _ENV
  7. Push closure onto stack
```

---

## 8. File Change Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `pkg/parser/parser.go` | MODIFY | Add `ParseChunk()` exported method |
| `pkg/codegen/codegen.go` | MODIFY | Add `CompileChunk()` function, add upvalue tracking |
| `pkg/codegen/expr.go` | MODIFY | Fix `genVar()` to use GETTABUP for globals |
| `pkg/codegen/stmt.go` | MODIFY | Fix `assignToVar()` to use SETTABUP for globals |
| `pkg/api/load.go` | MODIFY | Remove compileSimpleCode, wire codegen, set up _ENV |
| `pkg/vm/vm.go` | MODIFY | Add 13 missing opcodes, add Closure to CallInfo |
| `pkg/api/stdlib.go` | NEW | Standard library functions |
| `cmd/lua/main.go` | NEW | CLI entry point |
| `pkg/api/api_test.go` | MODIFY (optional) | Add end-to-end tests |

---

## 9. Dependency Order

1. **Wave 1** (no deps): Codegen fixes (GETTABUP/SETTABUP for globals, CompileChunk, upvalue tracking)
2. **Wave 2** (depends on Wave 1): VM opcodes (all 13 missing opcodes)
3. **Wave 3** (depends on Wave 1+2): Pipeline wiring (LoadString, _ENV setup) + stdlib + CLI + tests

Actually, since these all touch different files mostly, we can do:
- **Branch A**: Codegen fixes + Parser.ParseChunk() + LoadString wiring + _ENV setup
- **Branch B**: VM opcodes (all 13)
- **Branch C**: stdlib + CLI + end-to-end tests (depends on A and B)

But A and B both modify vm.go (A needs CallInfo.Closure, B adds opcodes). Better:
- **Branch 1**: ALL code changes (codegen, parser, vm opcodes, load.go, stdlib, CLI)

Given the complexity and interdependence, a single branch with clear instructions is best.