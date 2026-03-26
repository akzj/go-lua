# Phase G: Bitwise Operators Implementation

## Overview

This document describes the implementation of bitwise operators (`& | ~ << >>`) for the Go-based Lua 5.x VM. The goal is to enable at least 10 tests from the official Lua test suite to pass.

## Current State

### Already Implemented
- **VM opcodes**: `OP_BAND`, `OP_BOR`, `OP_BXOR`, `OP_SHL`, `OP_SHR`, `OP_BNOT` are defined in `pkg/vm/opcodes.go` and implemented in `pkg/vm/vm.go`
- **Lexer tokens**: `TK_SHL` and `TK_SHR` are defined in `pkg/lexer/token.go`
- **Parser precedence levels**: `precBitOr`, `precBitXor`, `precBitAnd`, `precShift` are defined
- **Codegen**: Already handles bitwise operators in `genArithmetic()` and `genUnOp()`

### Missing Components
1. **Lexer tokens**: `TK_BAND`, `TK_BOR`, `TK_BXOR` are not defined
2. **Lexer scanning**: `&`, `|`, `~` are not properly scanned
3. **Parser**: Missing handling for bitwise operators in precedence and expression creation

## Implementation Plan

### 1. Lexer Changes (`pkg/lexer/token.go`)

Add new tokens for bitwise operators:

```go
// Add to multi-character operator tokens section (around line 60):
const (
    TK_IDIV TokenType = iota + 512   // '//' Integer division
    TK_CONCAT                          // '..' Concatenation
    TK_DOTS                            // '...' Vararg
    TK_EQ                              // '==' Equality
    TK_GE                              // '>=' Greater than or equal
    TK_LE                              // '<=' Less than or equal
    TK_NE                              // '~=' Not equal
    TK_SHL                             // '<<' Left shift
    TK_SHR                             // '>>' Right shift
    TK_DBCOLON                         // '::' Label delimiter
    TK_BAND                            // '&' Bitwise AND (NEW)
    TK_BOR                             // '|' Bitwise OR (NEW)
    TK_BXOR                            // '~' Bitwise XOR (NEW)
)
```

### 2. Lexer Changes (`pkg/lexer/lexer.go`)

#### 2.1 Add scanning for `&` operator

Add a new case in the `NextToken()` switch statement (around line 150):

```go
case '&': // '&'
    l.Advance()
    return Token{Type: TK_BAND, Line: tokenLine, Column: tokenColumn}, nil
```

#### 2.2 Add scanning for `|` operator

Add a new case in the `NextToken()` switch statement:

```go
case '|': // '|'
    l.Advance()
    return Token{Type: TK_BOR, Line: tokenLine, Column: tokenColumn}, nil
```

#### 2.3 Fix scanning for `~` operator

The current implementation incorrectly returns `TK_CARET` for `~`. The `~` character should:
- Return `TK_BXOR` when used as binary XOR operator
- Return `TK_NE` when followed by `=` (already handled correctly)

Current code (line ~200):
```go
case '~': // '~' or '~='
    l.Advance()
    if l.Match('=') {
        return Token{Type: TK_NE, Line: tokenLine, Column: tokenColumn}, nil
    }
    return Token{Type: TK_CARET, Value: "~", Line: tokenLine, Column: tokenColumn}, nil
```

Should be changed to:
```go
case '~': // '~' or '~='
    l.Advance()
    if l.Match('=') {
        return Token{Type: TK_NE, Line: tokenLine, Column: tokenColumn}, nil
    }
    return Token{Type: TK_BXOR, Line: tokenLine, Column: tokenColumn}, nil
```

### 3. Parser Changes (`pkg/parser/expr.go`)

#### 3.1 Update `getOperatorPrecedence()`

Add cases for the new bitwise tokens:

```go
func (p *Parser) getOperatorPrecedence() precedenceLevel {
    switch p.Current.Type {
    case lexer.TK_OR:
        return precOr
    case lexer.TK_AND:
        return precAnd
    case lexer.TK_LT, lexer.TK_GT, lexer.TK_LE, lexer.TK_GE, lexer.TK_EQ, lexer.TK_NE:
        return precComparison
    case lexer.TK_BOR:  // NEW: bitwise OR (lowest bitwise precedence)
        return precBitOr
    case lexer.TK_BXOR: // NEW: bitwise XOR
        return precBitXor
    case lexer.TK_BAND: // NEW: bitwise AND
        return precBitAnd
    case lexer.TK_SHL, lexer.TK_SHR:
        return precShift
    case lexer.TK_CONCAT:
        return precConcat
    case lexer.TK_PLUS, lexer.TK_MINUS:
        return precAdd
    case lexer.TK_STAR, lexer.TK_SLASH, lexer.TK_IDIV, lexer.TK_PERCENT:
        return precMul
    case lexer.TK_CARET:
        return precPower
    default:
        return precNone
    }
}
```

#### 3.2 Update `createBinOpExpr()`

Add cases for bitwise operators:

```go
func (p *Parser) createBinOpExpr(left Expr, op lexer.Token, right Expr) Expr {
    var opStr string
    switch op.Type {
    // ... existing cases ...
    case lexer.TK_SHL:
        opStr = "<<"
    case lexer.TK_SHR:
        opStr = ">>"
    case lexer.TK_BAND:  // NEW
        opStr = "&"
    case lexer.TK_BOR:   // NEW
        opStr = "|"
    case lexer.TK_BXOR:  // NEW
        opStr = "~"
    }
    // ...
}
```

#### 3.3 Update `parseUnaryExpr()`

Add handling for unary `~` (bitwise NOT):

```go
func (p *Parser) parseUnaryExpr() Expr {
    line := p.Current.Line
    op := p.Current.Type
    p.advance()

    // ... existing code for '-' with numbers ...

    var opStr string
    switch op {
    case lexer.TK_MINUS:
        opStr = "-"
    case lexer.TK_NOT:
        opStr = "not"
    case lexer.TK_HASH:
        opStr = "#"
    case lexer.TK_BXOR:  // NEW: unary ~ for bitwise NOT
        opStr = "~"
    }

    // ...
}
```

Also update the switch in `parsePrefixExpr()` to include `lexer.TK_BXOR` for unary usage:

```go
case lexer.TK_MINUS, lexer.TK_NOT, lexer.TK_HASH, lexer.TK_BXOR:
    return p.parseUnaryExpr()
```

## Lua 5.3+ Operator Precedence

The correct precedence (lowest to highest):
```
or
and
< > <= >= ~= ==
| (bitwise OR)
~ (bitwise XOR)
& (bitwise AND)
<< >> (shifts)
..
+ - (arithmetic)
* / // %
unary operators (not - ~ #)
^
```

## Testing

### Unit Tests
1. Lexer tests for new tokens
2. Parser tests for precedence
3. Codegen tests for opcode generation
4. VM tests for execution

### Integration Tests
```lua
-- Basic bitwise operations
print(0xFF & 0x0F)  -- Expected: 15
print(0xF0 | 0x0F)  -- Expected: 255
print(0xFF ~ 0x0F)  -- Expected: 240
print(1 << 4)       -- Expected: 16
print(256 >> 4)     -- Expected: 16
print(~0)           -- Expected: -1 (all bits set)
```

### Official Test Suite
Run tests from `lua-master/testes` and verify at least 10 tests pass.

## Files to Modify

1. `pkg/lexer/token.go` - Add TK_BAND, TK_BOR, TK_BXOR tokens
2. `pkg/lexer/lexer.go` - Add scanning for &, |, ~
3. `pkg/parser/expr.go` - Add precedence and expression handling
4. `pkg/codegen/expr.go` - Already implemented (verify)
5. `pkg/vm/vm.go` - Already implemented (verify)

## Verification Checklist

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `print(0xFF & 0x0F)` returns 15
- [ ] `print(1 << 4)` returns 16
- [ ] At least 10 official tests pass