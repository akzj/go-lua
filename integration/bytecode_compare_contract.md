# Bytecode Comparison Test Contract

## Overview

Design bytecode comparison tests in `integration/bytecode_test.go` that:
1. Parse Lua source code → AST (using `parse.NewParser()`)
2. Compile AST → bytecode Prototype (using `bytecode.NewCompiler()`)
3. Extract instruction sequences and constant pools
4. Verify against lua-master reference patterns

## Architecture

```
integration/bytecode_test.go
├── TestBytecode_compileBasics       // Simple expressions → opcode sequence
├── TestBytecode_constantPool        // Integer/float/string constants
├── TestBytecode_functionClosure     // Function definitions
└── TestBytecode_controlFlow         // If/for/while opcodes
```

## Dependencies

```go
import (
    "testing"
    bc "github.com/akzj/go-lua/bytecode"
    parse "github.com/akzj/go-lua/parse"
    "github.com/akzj/go-lua/opcodes/api"
)
```

## Key Types

```go
// InstructionInfo represents a decoded VM instruction.
// Why not just compare raw uint32? Opcode names are human-readable
// and match lua-master's T.listcode() output format.
type InstructionInfo struct {
    Opcode opcodes.OpCode
    A      int
    B      int
    C      int
    Bx     int
}

// CompareInstructions compares two instruction sequences.
// Returns nil if equivalent, error describing first difference.
// Why not byte-compare? go-lua and lua-master may use different
// register allocation - opcode sequence must match exactly.
func CompareInstructions(want, got []uint32) error

// CompareConstantPools compares constant pools by type and value.
// Why not index-by-index? Constant pool ordering may differ between
// implementations. Compare by semantic equivalence.
func CompareConstantPools(want, got []*bc.Constant) error

// ExtractInstructions decodes raw uint32 instructions to human-readable form.
// Matches lua-master's T.listcode() format.
func ExtractInstructions(code []uint32) []InstructionInfo

// ExtractConstants converts Prototype constants to comparable form.
func ExtractConstants(constants []*bc.Constant) []ConstantInfo
```

## Invariants

1. **Instruction count invariant**: If lua-master emits N opcodes for X.lua,
   go-lua must emit exactly N opcodes for the same source.

2. **Opcode sequence invariant**: The opcode sequence must match lua-master's
   output from T.listcode() (line numbers stripped).

3. **Constant pool invariant**: Constants must be semantically equivalent
   (same type and value), though index may differ.

## Why Not Alternatives

- **Why not run luac?** Lua is not installed in this environment. Use embedded
  reference data instead.

- **Why not just test execution?** The goal is to verify bytecode generation
  correctness, not runtime behavior. lua-master/testes only tested parsing.

- **Why not compare raw uint32?** go-lua may use different register allocation
  than lua-master. Opcode sequence comparison is more robust.

## Reference Data Format

```go
// Test fixtures follow lua-master's code.lua patterns:
// check(function() ... end, "OP1", "OP2", ...)
//
// Example:
//   check(function() local a end end, "LOADNIL", "RETURN0")
//
// For constants, use checkKlist patterns:
//   checkKlist(foo, {1, 2.0, "str"})
```

## Acceptance

```bash
ls bytecode_test.go 2>/dev/null && head -50 bytecode_test.go
go build ./... && go test ./... -run Bytecode -v
```
