# Parser Package

The `parser` package implements a recursive descent parser for Lua source code with precedence climbing for expressions. It follows the semantics from the official Lua C implementation (`lua-master/lparser.c`).

## Overview

The parser transforms Lua source code (tokens from the `lexer` package) into an Abstract Syntax Tree (AST). The AST can then be used by the code generator to produce bytecode.

## Parsing Strategy

### Recursive Descent
- Each statement type has its own parsing function
- Top-down parsing approach
- Easy to understand and maintain

### Precedence Climbing
- Used for expression parsing
- Handles operator precedence correctly
- Supports left and right associative operators

### Error Recovery
- Synchronization to next statement boundary
- Errors are collected and reported with line numbers
- Parser continues after errors to find more issues

## Usage

```go
package main

import (
    "github.com/akzj/go-lua/pkg/lexer"
    "github.com/akzj/go-lua/pkg/parser"
)

func main() {
    source := []byte("local x = 1 + 2")
    
    // Create lexer
    l := lexer.NewLexer(source, "chunk.lua")
    
    // Create parser
    p := parser.NewParser(l)
    
    // Parse the source
    proto, err := p.Parse()
    if err != nil {
        // Handle error
        return
    }
    
    // Use the prototype...
    _ = proto
}
```

## AST Structure

### Expression Interface

All expression nodes implement the `Expr` interface:

```go
type Expr interface {
    exprNode()
    Line() int
}
```

#### Expression Types

| Type | Description | Example |
|------|-------------|---------|
| `NilExpr` | nil literal | `nil` |
| `BooleanExpr` | boolean literal | `true`, `false` |
| `NumberExpr` | numeric literal | `42`, `3.14` |
| `StringExpr` | string literal | `"hello"` |
| `VarExpr` | variable reference | `x` |
| `FieldExpr` | field access | `obj.field` |
| `IndexExpr` | indexed access | `arr[1]` |
| `CallExpr` | function call | `func(1, 2)` |
| `MethodCallExpr` | method call | `obj:method(1)` |
| `BinOpExpr` | binary operation | `a + b` |
| `UnOpExpr` | unary operation | `-x`, `not a` |
| `TableExpr` | table constructor | `{x = 1, 2, 3}` |
| `FuncExpr` | anonymous function | `function(x) return x end` |
| `DotsExpr` | vararg expression | `...` |
| `ParenExpr` | parenthesized expression | `(a + b)` |

### Statement Interface

All statement nodes implement the `Stmt` interface:

```go
type Stmt interface {
    stmtNode()
    Line() int
}
```

#### Statement Types

| Type | Description | Example |
|------|-------------|---------|
| `BlockStmt` | block of statements | `do ... end` |
| `AssignStmt` | assignment | `x = 1` |
| `LocalStmt` | local declaration | `local x = 1` |
| `IfStmt` | conditional | `if x then ... end` |
| `WhileStmt` | while loop | `while x do ... end` |
| `RepeatStmt` | repeat loop | `repeat ... until x` |
| `ForNumericStmt` | numeric for loop | `for i = 1, 10 do ... end` |
| `ForGenericStmt` | generic for loop | `for k, v in pairs(t) do ... end` |
| `BreakStmt` | break statement | `break` |
| `ReturnStmt` | return statement | `return 1, 2` |
| `GotoStmt` | goto statement | `goto label` |
| `LabelStmt` | label | `::label::` |
| `FuncDefStmt` | function definition | `function foo() ... end` |
| `ExprStmt` | expression statement | `print("hello")` |

## Operator Precedence

Operators are parsed with the following precedence (highest to lowest):

1. `^` (power, right-associative)
2. `not`, `-`, `#`, `~` (unary)
3. `*`, `/`, `//`, `%` (multiplicative)
4. `+`, `-` (additive)
5. `..` (concatenation, right-associative)
6. `<<`, `>>` (bitwise shift)
7. `&` (bitwise AND)
8. `~` (bitwise XOR)
9. `|` (bitwise OR)
10. `<`, `>`, `<=`, `>=`, `~=`, `==` (comparison)
11. `and` (logical AND)
12. `or` (logical OR)

## Error Handling

The parser collects errors during parsing and continues to find more issues:

```go
p := parser.NewParser(lexer)
proto, err := p.Parse()
if err != nil {
    // err contains the first error encountered
    // Additional errors are in p.Errors
}
```

Error messages include the source name and line number:
```
chunk.lua:5: expected 'end' after if statement
```

## Files

- `ast.go` - AST node definitions (Expr and Stmt interfaces)
- `parser.go` - Parser core structure and main parsing logic
- `expr.go` - Expression parsing (precedence climbing)
- `stmt.go` - Statement parsing (recursive descent)
- `parser_test.go` - Comprehensive unit tests

## Testing

Run tests with coverage:

```bash
go test ./pkg/parser -v -cover
```

Current test coverage: ≥70%

## Dependencies

- `pkg/lexer` - Lexical analyzer (token stream)
- `pkg/object` - Object system (Prototype, TValue)

## References

- Lua 5.4 Reference Manual: https://www.lua.org/manual/5.4/
- Lua C Implementation: `lua-master/lparser.c`
- Architecture Design: `docs/architecture.md`