# Code Generator Package

The `codegen` package generates Lua bytecode from parsed AST (Abstract Syntax Tree). It translates high-level Lua constructs into low-level VM instructions.

## Overview

This package implements the Lua code generator, following the semantics from `lua-master/lcode.c`. It traverses the AST produced by the parser and emits bytecode instructions for the VM.

## Architecture

### Core Components

```
pkg/codegen/
├── codegen.go       # Core generator, register allocation, instruction emission
├── expr.go          # Expression code generation
├── stmt.go          # Statement code generation
├── func.go          # Function prototype generation (TODO)
├── codegen_test.go  # Comprehensive tests
└── README.md        # This documentation
```

### CodeGenerator Structure

```go
type CodeGenerator struct {
    Prototype    *object.Prototype  // Being built
    PC           int                // Program counter
    StackTop     int                // Current stack top
    MaxStackSize int                // Maximum stack needed
    Locals       [][]LocalVar       // Local variable scopes
    Upvalues     map[string]int     // Upvalue tracking
    Constants    map[string]int     // Constant cache
}
```

## Code Generation Strategy

### Expression Code Generation

Expressions produce values that are left on the stack top.

#### Literals

```lua
-- Number literals
local x = 42      -- LOADI for small integers (-128 to 127)
local y = 1000.5  -- LOADK for other numbers

-- Boolean literals
local a = true    -- LOADBOOL
local b = false   -- LOADBOOL

-- String literals
local s = "hello" -- LOADK

-- Nil literal
local n = nil     -- LOADNIL
```

#### Variables

```lua
-- Local variables
local x = 10
local y = x       -- MOVE

-- Global variables
x = 10            -- SETTABLE on _ENV
y = x             -- GETTABLE from _ENV

-- Table access
local t = {}
local v = t[k]    -- GETTABLE
local w = t.field -- GETFIELD
```

#### Arithmetic Operations

```lua
local z = x + y   -- ADD
local z = x - y   -- SUB
local z = x * y   -- MUL
local z = x / y   -- DIV
local z = x % y   -- MOD
local z = x ^ y   -- POW
```

#### Comparison Operations

```lua
local r = x == y  -- EQ + conditional load
local r = x < y   -- LT + conditional load
local r = x <= y  -- LE + conditional load
```

#### Logical Operations (Short-Circuit)

```lua
-- AND: if left is false, result is left; else result is right
local r = a and b
-- Generates:
--   evaluate a
--   TEST a
--   JMP to end (if false)
--   evaluate b
--   end:

-- OR: if left is true, result is left; else result is right
local r = a or b
-- Generates:
--   evaluate a
--   JMP to end (if true)
--   evaluate b
--   end:
```

#### Unary Operations

```lua
local y = -x     -- UNM
local y = not x  -- NOT
local y = #x     -- LEN
local y = ~x     -- BNOT
```

#### Table Constructors

```lua
-- Array-style
local t = {1, 2, 3}
-- Generates:
--   NEWTABLE
--   SETI for each element

-- Field-style
local t = {x = 1, y = 2}
-- Generates:
--   NEWTABLE
--   SETFIELD for each field

-- Mixed
local t = {1, 2, x = 3}
-- Generates:
--   NEWTABLE
--   SETI for array elements
--   SETFIELD for fields
```

#### Function Calls

```lua
-- Regular call
result = func(a, b)
-- Generates:
--   load func
--   load arguments
--   CALL

-- Method call
result = obj:method(a, b)
-- Generates:
--   load obj
--   SELF (loads method and obj)
--   load arguments
--   CALL
```

### Statement Code Generation

Statements consume values without producing results.

#### Assignment

```lua
-- Simple assignment
x = 10

-- Multiple assignment
x, y = 1, 2

-- Table field assignment
t.field = value
t[key] = value
```

#### Local Variables

```lua
local x = 10
local x, y = 1, 2
```

#### Control Flow

```lua
-- If statement
if cond then
    -- then block
elseif cond2 then
    -- elseif block
else
    -- else block
end
-- Generates: TEST + JMP for conditionals

-- While loop
while cond do
    -- body
end
-- Generates: condition test + backward JMP

-- Repeat loop
repeat
    -- body
until cond
-- Generates: body + condition test + backward JMP

-- Numeric for loop
for i = start, end, step do
    -- body
end
-- Generates: FORPREP + body + FORLOOP

-- Generic for loop
for k, v in pairs(t) do
    -- body
end
-- Generates: FORGPREP + body + FORGLOOP
```

#### Return

```lua
return          -- RETURN with 0 results
return x        -- RETURN with 1 result
return x, y     -- RETURN with multiple results
```

## Register Allocation

The code generator uses **simple linear allocation**:

1. Each temporary value gets a new register
2. Stack top is tracked continuously
3. Temporaries are freed after statement completion
4. `MaxStackSize` is computed for the prototype

### Example

```lua
local x = 1 + 2 * 3
```

Generates:
```
R0 = LOADI 3      -- load 3
R1 = LOADI 2      -- load 2
R2 = MUL R1, R0   -- 2 * 3
R3 = LOADI 1      -- load 1
R4 = ADD R3, R2   -- 1 + (2 * 3)
R5 = MOVE R4      -- assign to x
FREE R0-R4        -- free temporaries
```

## Constant Management

Constants are **deduplicated** using a map:

1. Small integers (-128 to 127) use `LOADI` instruction (no constant table entry)
2. Other constants use `LOADK` with index into constant table
3. Duplicate constants are detected and reused

### Example

```lua
local x = 42      -- LOADI 42 (no constant)
local y = 1000    -- LOADK 0 (constant[0] = 1000)
local z = 1000    -- LOADK 0 (reuses constant[0])
```

## Short-Circuit Evaluation

Logical operators use **jump-based short-circuit evaluation**:

### AND Operation

```lua
result = a and b
```

Generates:
```
evaluate a
TEST a          -- if a is false, skip next
JMP to end      -- jump over b evaluation
evaluate b
MOVE b to result
end:
```

### OR Operation

```lua
result = a or b
```

Generates:
```
evaluate a
JMP to end      -- if a is true, skip b
evaluate b
MOVE b to result
end:
```

## Usage Example

```go
package main

import (
    "github.com/akzj/go-lua/pkg/codegen"
    "github.com/akzj/go-lua/pkg/parser"
    "github.com/akzj/go-lua/pkg/lexer"
)

func main() {
    // Parse source code
    source := `
    function add(a, b)
        return a + b
    end
    `
    
    lex := lexer.NewLexer([]byte(source), "test.lua")
    p := parser.NewParser(lex)
    funcDef, _ := p.Parse()
    
    // Generate bytecode
    cg := codegen.NewCodeGenerator()
    prototype := cg.Generate(funcDef)
    
    // Use prototype with VM
    // vm.Load(prototype)
}
```

## Testing

Run tests with:

```bash
go test ./pkg/codegen -v
go test ./pkg/codegen -cover
```

### Test Coverage

The package includes comprehensive tests for:

- Instruction emission (EmitABC, EmitABx, EmitAsBx, EmitAx)
- Constant table management
- Register allocation
- Local variable scoping
- Expression code generation
- Statement code generation
- Integration scenarios

## Dependencies

- `pkg/parser` - AST definitions
- `pkg/vm` - Instruction definitions (opcodes)
- `pkg/object` - TValue and Prototype definitions

## Implementation Status

### Completed

- [x] Core infrastructure (CodeGenerator struct, instruction emission)
- [x] Constant table management
- [x] Register allocation
- [x] Local variable management
- [x] Expression code generation (literals, variables, arithmetic, logic)
- [x] Statement code generation (assignment, local, if, while, for, return)
- [x] Function prototype generation
- [x] Unit tests

### TODO

- [ ] Upvalue resolution for closures
- [ ] Break/continue statement support
- [ ] Goto and label support
- [ ] Optimization passes
- [ ] Debug information (line numbers, local variable names)

## References

- Lua 5.4 Reference Manual: https://www.lua.org/manual/5.4/
- Lua Source Code: `lua-master/lcode.c`
- Architecture Design: `docs/architecture.md`