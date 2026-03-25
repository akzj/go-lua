# VM Package - Lua Virtual Machine

This package implements a register-based Lua virtual machine that executes Lua bytecode.

## Architecture Overview

The VM is the core execution engine for Lua bytecode. It features:

- **Register-based execution**: Uses a stack with register windows for function calls
- **RK mode**: Efficient constant/register operand resolution
- **CallInfo management**: Stack frames for function calls with proper base/top tracking
- **Upvalue support**: Closures with access to enclosing function variables

## Instruction Formats

The VM uses four instruction formats (all 32-bit):

### iABC Format
```
+--------+--------+--------+--------+
| Opcode |   A    |   C    |   B    |
|  7bit  |  8bit  |  9bit  |  8bit  |
+--------+--------+--------+--------+
Bits:    [0-6]   [7-14]  [15-23] [24-31]
```

### iABx Format
```
+--------+--------+-----------------+
| Opcode |   A    |      Bx         |
|  7bit  |  8bit  |     17bit       |
+--------+--------+-----------------+
Bits:    [0-6]   [7-14]   [15-31]
```

### iAsBx Format (signed Bx)
```
+--------+--------+-----------------+
| Opcode |   A    |      sBx        |
|  7bit  |  8bit  |     17bit       |
+--------+--------+-----------------+
sBx = Bx - 0xFFFF (bias for signed values)
```

### iAx Format
```
+--------+-------------------------+
| Opcode |          Ax             |
|  7bit  |         25bit           |
+--------+-------------------------+
Bits:    [0-6]        [7-31]
```

## RK Mode

Many instructions use RK (Register/Constant) mode for B/C operands:

- **If value < 256**: Register index `R(value)`
- **If value >= 256**: Constant index `K(value - 256)`

Example:
```go
// In instruction: B=256 means K(0), B=10 means R(10)
value := vm.getRKValue(256)  // Returns Constants[0]
value := vm.getRKValue(10)   // Returns Stack[Base+10]
```

## Stack Layout

Each function call has a stack frame:

```
[0..Base-1]    : Caller's frame
[Base]         : Function value
[Base+1..]     : Function parameters and locals
[StackTop]     : First free slot
```

## Usage Examples

### Creating a VM

```go
import (
    "github.com/akzj/go-lua/pkg/state"
    "github.com/akzj/go-lua/pkg/vm"
)

global := state.NewGlobalState()
vm := vm.NewVM(global)
```

### Executing Instructions

```go
// Set up prototype with constants
vm.Prototype = &object.Prototype{
    Constants: []object.TValue{
        *object.NewNumber(100.0),
    },
}

// Set up stack
vm.Stack[vm.Base+0].SetNumber(10.0)
vm.Stack[vm.Base+1].SetNumber(20.0)

// Execute ADD R(2), R(0), R(1)
instr := vm.MakeABC(vm.OP_ADD, 2, 0, 1)
err := vm.ExecuteInstruction(instr)
if err != nil {
    log.Fatal(err)
}

// Result: vm.Stack[vm.Base+2] = 30.0
```

### Running Bytecode

```go
// Set up bytecode
vm.Prototype = &object.Prototype{
    Code: []object.Instruction{
        vm.MakeAsBx(vm.OP_LOADI, 0, 42),  // R(0) = 42
        vm.MakeAsBx(vm.OP_LOADI, 1, 58),  // R(1) = 58
        vm.MakeABC(vm.OP_ADD, 2, 0, 1),   // R(2) = R(0) + R(1)
    },
}
vm.PC = 0

// Run the bytecode
err := vm.Run()
if err != nil {
    log.Fatal(err)
}
```

## Instruction Categories

### Data Loading
- `MOVE`, `LOADI`, `LOADF`, `LOADK`, `LOADKX`, `LOADBOOL`, `LOADNIL`

### Arithmetic
- `ADD`, `SUB`, `MUL`, `DIV`, `MOD`, `POW`, `IDIV`
- `UNM` (unary minus)
- `BAND`, `BOR`, `BXOR`, `SHL`, `SHR`, `BNOT` (bitwise)

### Comparison
- `EQ`, `LT`, `LE`
- `EQI`, `LTI`, `LEI`, `GTI` (immediate with constant)

### Control Flow
- `JMP`, `TEST`, `CLOSE`, `TBC`

### Table Operations
- `NEWTABLE`, `GETTABLE`, `SETTABLE`
- `GETI`, `SETI` (integer index)
- `GETFIELD`, `SETFIELD` (field access)

### Other
- `CONCAT`, `LEN`, `NOT`, `SELF`, `ADDI`

## Error Handling

The VM returns errors for:
- Invalid instructions
- Type errors (e.g., indexing non-table)
- Stack overflow conditions

```go
err := vm.ExecuteInstruction(instr)
if err != nil {
    // Handle error
}
```

## Testing

Run tests with coverage:

```bash
go test ./pkg/vm -v -cover
```

Current test coverage: ≥87%

## References

- Lua 5.4 Reference Manual: https://www.lua.org/manual/5.4/
- Lua VM source code: https://github.com/lua/lua
