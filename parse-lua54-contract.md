# Lua 5.4+ Parser Features Contract

## Overview
Add support for Lua 5.4+ features to the parse module:
1. goto statements and label statements
2. global <const> * modifiers
3. bitwise operators (& | ~ << >>)
4. // integer division (already tokenized)

## 1. Lexer Changes

### File: lex/api/api.go

**ADD** `TOKEN_CONST` after `TOKEN_GLOBAL` in the reserved keywords section:
```go
// In the const block starting at TOKEN_AND:
TOKEN_GLOBAL // Lua 5.4 compatibility
TOKEN_GOTO
TOKEN_CONST // Lua 5.4 compatibility - NEW
```

**Why not TOKEN_GLOBAL + TOKEN_CONST separately?** Lua 5.4 uses `global const` as a combined modifier. Having TOKEN_CONST allows the parser to distinguish `global const x` from just `global`.

## 2. AST Changes

### File: ast/api/api.go

**ADD** to BinopKind constants:
```go
const (
    BINOP_ADD BinopKind = iota //
    BINOP_SUB                   //
    // ... existing ...
    BINOP_SHL                   // <<
    BINOP_SHR                   // >>
    BINOP_BAND                  // & NEW
    BINOP_BOR                   // | NEW
    BINOP_BXOR                  // ~ NEW
    BINOP_CONCAT                // ..
)
```

**Why not just use TOKEN_* directly?** The AST should be language-agnostic. BinopKind is the canonical representation independent of lexer tokens.

## 3. Parser Changes

### File: parse/internal/parser.go

#### 3.1 Goto Statement
```go
case lexapi.TOKEN_GOTO:
    p.parseGoto()
```
**Parser function:**
```go
func (p *parser) parseGoto() {
    p.next() // consume 'goto'
    name := p.current().Value
    tok := p.current()
    p.next()
    stat := &gotoStat{
        baseNode: baseNode{line: tok.Line, column: tok.Column},
        name:     name,
    }
    p.block.stats = append(p.block.stats, stat)
}
```

**Why not resolve labels immediately?** Goto resolution requires scope analysis. Labels must be in the same block or an enclosing block. Goto cannot jump into a function.

#### 3.2 Label Statement
```go
case lexapi.TOKEN_DBCOLON:
    p.parseLabel()
```
**Parser function:**
```go
func (p *parser) parseLabel() {
    p.next() // consume first ':'
    if !p.peek(lexapi.TOKEN_DBCOLON) {
        // Not a label, might be an error or part of another construct
        return
    }
    p.next() // consume second ':'
    name := p.current().Value
    nameTok := p.current()
    p.next()
    stat := &labelStat{
        baseNode: baseNode{line: nameTok.Line, column: nameTok.Column},
        name:     name,
    }
    p.block.stats = append(p.block.stats, stat)
}
```

#### 3.3 Global Statement
```go
case lexapi.TOKEN_GLOBAL:
    p.parseGlobal()
```
**Parser function:**
```go
func (p *parser) parseGlobal() {
    p.next() // consume 'global'
    parentBlock := p.block
    
    // Check for modifiers
    isConst := p.peek(lexapi.TOKEN_CONST)
    if isConst {
        p.next() // consume 'const'
    }
    
    // Check for '*' export-all modifier
    isExportAll := p.peek(lexapi.TOKEN_MUL)
    if isExportAll {
        p.next() // consume '*'
        // Handle global const * = expr
        if p.peek(lexapi.TOKEN_ASSIGN) {
            p.next()
            _, err := p.parseExprList()
            if err != nil {
                return
            }
        }
        return
    }
    
    // Regular global var = expr
    if p.peek(lexapi.TOKEN_NAME) {
        name := p.current().Value
        nameTok := p.current()
        p.next()
        
        if p.peek(lexapi.TOKEN_ASSIGN) {
            p.next()
            exprs, err := p.parseExprList()
            if err != nil {
                return
            }
            stat := &globalVarStat{
                baseNode: baseNode{line: nameTok.Line, column: nameTok.Column},
                name:     name,
                isConst:  isConst,
                exprs:    exprs,
            }
            parentBlock.stats = append(parentBlock.stats, stat)
        }
    }
}
```

**New statement type for global variables:**
```go
type globalVarStat struct {
    baseNode
    name    string
    isConst bool
    exprs   []astapi.ExpNode
}

func (s *globalVarStat) IsScopeEnd() bool   { return false }
func (s *globalVarStat) Kind() astapi.StatKind { return astapi.STAT_GLOBAL_VAR }
func (s *globalVarStat) GetName() string   { return s.name }
func (s *globalVarStat) IsConst() bool      { return s.isConst }
func (s *globalVarStat) GetExprs() []astapi.ExpNode { return s.exprs }
```

#### 3.4 Bitwise Operators
Add to parseExpr precedence chain (after parseComparison, before parseAdd):

```go
func (p *parser) parseBitwiseOr() (astapi.ExpNode, error) {
    left, err := p.parseBitwiseXor()
    if err != nil {
        return nil, err
    }
    for p.peek(lexapi.TOKEN_PIPE) { // |
        tok := p.current()
        p.next()
        right, err := p.parseBitwiseXor()
        if err != nil {
            return nil, err
        }
        left = &binopExp{op: astapi.BINOP_BOR, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
    }
    return left, nil
}

func (p *parser) parseBitwiseXor() (astapi.ExpNode, error) {
    left, err := p.parseBitwiseAnd()
    if err != nil {
        return nil, err
    }
    for p.peek(lexapi.TOKEN_TILDE) { // ~ (bxor in Lua 5.3+)
        tok := p.current()
        p.next()
        right, err := p.parseBitwiseAnd()
        if err != nil {
            return nil, err
        }
        left = &binopExp{op: astapi.BINOP_BXOR, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
    }
    return left, nil
}

func (p *parser) parseBitwiseAnd() (astapi.ExpNode, error) {
    left, err := p.parseShift()
    if err != nil {
        return nil, err
    }
    for p.peek(lexapi.TOKEN_AMP) { // &
        tok := p.current()
        p.next()
        right, err := p.parseShift()
        if err != nil {
            return nil, err
        }
        left = &binopExp{op: astapi.BINOP_BAND, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
    }
    return left, nil
}

func (p *parser) parseShift() (astapi.ExpNode, error) {
    left, err := p.parseAdd()
    if err != nil {
        return nil, err
    }
    for p.peek(lexapi.TOKEN_SHL) || p.peek(lexapi.TOKEN_SHR) { // << >>
        var op astapi.BinopKind
        if p.peek(lexapi.TOKEN_SHL) {
            op = astapi.BINOP_SHL
        } else {
            op = astapi.BINOP_SHR
        }
        tok := p.current()
        p.next()
        right, err := p.parseAdd()
        if err != nil {
            return nil, err
        }
        left = &binopExp{op: op, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
    }
    return left, nil
}
```

**Update precedence chain:**
```
parseExpr -> parseOr -> parseAnd -> parseComparison -> 
parseBitwiseOr -> parseBitwiseXor -> parseBitwiseAnd -> parseShift -> 
parseAdd -> parseMul -> parseUnary -> parsePrimary
```

## 4. AST API Update

**ADD** to StatKind constants:
```go
const (
    STAT_ASSIGN StatKind = iota
    // ... existing ...
    STAT_GLOBAL_VAR // NEW - global variable declaration
    STAT_GOTO       // NEW
    STAT_LABEL      // NEW
)
```

## 5. Known Traps

1. **TOKEN_TILDE is BXOR, not NOT**: In Lua 5.3+, `~` is bitwise XOR. `not` is TOKEN_NOT.
2. **Label scope**: Labels are block-scoped. A label named `foo` is visible from its definition until the end of the block.
3. **Goto restrictions**: Goto cannot jump into the scope of a local variable (including function parameters).
4. **Global modifiers**: `global const` and `global const *` are Lua 5.4 features. Plain `global` may not be supported.

## 6. Verification

```bash
go build ./parse/...
go test ./parse/...
go test ./parse/internal/testes
```

## 7. Test Cases

```lua
-- goto and labels
::label::
goto label

-- global modifiers
global const x = 1
global y = 2
global const * = expr

-- bitwise operators
a & b  -- band
a | b  -- bor
a ~ b  -- bxor (Lua 5.3+)
a << b -- shl
a >> b -- shr

-- integer division
a // b
```
