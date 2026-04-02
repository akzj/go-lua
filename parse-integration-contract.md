# Parse Module Full Integration Contract

## Status

✅ **Step 1 Complete**: parse already uses `lex/api.Lexer` interface
❌ **Step 2 Pending**: Parser has stub implementations that skip content

## Problem Statement

The parser in `parse/internal/parser.go` already:
- Imports and uses `lexapi "github.com/akzj/go-lua/lex/api"`
- Creates lexer via `lexpackage.NewLexer(chunk, "=(parse)")`
- Calls `lexer.NextToken()`, `lexer.Lookahead()`, etc.

**BUT**: Many parsing functions are stubs that just skip to `end` without parsing nested content.

## What Works (Basic Unit Tests Pass)

From `parse/internal/parser_test.go`:
```go
// These pass because they only test stub parsing:
TestParserBasic          // "x = 1"
TestParserAssignment     // "x = 1"
TestParserLocalVar       // "local x = 1"
TestParserFunction       // "function foo() end"
TestParserIf             // "if true then end"
TestParserWhile          // "while true do end"
TestParserForNumeric     // "for i = 1, 10 do end"
```

## What Fails (lua-master/testes Files)

All lua-master/testes/*.lua files fail with "unexpected symbol" because:

1. **`parseIf`**: Creates dummy condition `trueExp` and empty `thenBlock`, skips content until `end`
2. **`parseWhile`**: Same as parseIf
3. **`parseFor`**: Skips to `do` then to `end`, creates stub for loop variables
4. **`parseTableConstructor`**: Skips everything between `{` and `}`
5. **`parseFunctionDef`**: Skips from `function` keyword to `end`
6. **Operators**: `parseAdd`/`parseMul` etc. exist but expression chaining may be incomplete

## Required Fixes (Delegate to Branch)

### Fix 1: parseIf - Parse Condition and Blocks

```go
// CURRENT (stub):
func (p *parser) parseIf() {
    p.next() // consume 'if'
    cond := &trueExp{...}  // DUMMY!
    for !p.peek(lexapi.TOKEN_THEN) { p.next() }
    p.next() // consume 'then'
    thenBlock := &blockImpl{...}  // EMPTY!
    for !p.peek(lexapi.TOKEN_END) { p.next() }
    p.next()
    // ...
}

// REQUIRED:
func (p *parser) parseIf() {
    p.next() // consume 'if'
    cond, err := p.parseExpr()  // REAL condition
    if err != nil { return }
    if !p.peek(lexapi.TOKEN_THEN) { p.errorAt(p.current(), "'then' expected") }
    p.next() // consume 'then'
    thenBlock, err := p.parseBlock()  // REAL block
    if err != nil { return }
    
    // Handle elseif/else recursively
    ...
}
```

### Fix 2: parseTableConstructor - Parse Fields

```go
// CURRENT (stub):
func (p *parser) parseTableConstructor() (astapi.ExpNode, error) {
    tok := p.current()
    p.next() // consume '{'
    tc := &tableConstructor{...}
    for !p.peek(lexapi.TOKEN_RBRACE) && !p.peek(lexapi.TOKEN_EOS) {
        p.next()  // SKIP content!
    }
    if p.peek(lexapi.TOKEN_RBRACE) { p.next() }
    return tc, nil
}

// REQUIRED: Parse array fields [expr] = value, record fields name = value, general fields
// Lua table syntax:
//   { }                           -- empty
//   { 1, 2, 3 }                   -- array: [1]=1, [2]=2, [3]=3
//   { a = 1, b = 2 }              -- record: ["a"]=1, ["b"]=2
//   { [1] = "one", [2] = "two" }  -- explicit index
//   { x = y, "value" }            -- mixed
```

### Fix 3: parseFor - Parse Numeric/Generic Loop Variables

```go
// CURRENT: Skips to 'do' and creates dummy start/stop/step
// REQUIRED:
//   Numeric: for <name> = <expr>, <expr> [, <expr>] do <block> end
//   Generic: for <name> [, <name>]* in <expr> [, <expr>]* do <block> end
```

### Fix 4: parseFunctionDef - Parse Parameters

```go
// CURRENT: Skips from 'function' to 'end'
// REQUIRED:
//   function foo() end
//   function foo(a, b, c) end
//   function obj:method(self, a) end
//   function foo(...) end
//   function foo(a = 1) end  -- Lua 5.4 default values
//   function foo(a, ...) end  -- named + vararg
```

### Fix 5: Expression Operators

Ensure complete precedence chain:
```
parseOr         → or
parseAnd        → and  
parseComparison → < > <= >= ~= ==
parseBitwiseOr  → |
parseBitwiseXor → ~
parseBitwiseAnd → &
parseShift     → << >>
parseConcat    → ..  (right associative!)
parseAdd       → + -
parseMul       → * / // %
parseUnary     → not # - ~
parsePower     → ^  (right associative, highest)
```

## Acceptance Criteria

1. ✅ `go build ./parse/...` compiles (already true)
2. ✅ `go test ./parse/...` unit tests pass (already true)
3. ⬜ `go test ./parse/internal/testes -v` can parse lua-master/testes files
4. ✅ parse uses lex/api.Lexer (already true)
5. ⬜ Progressively pass lua-master/testes test cases

## Verification Command

```bash
go test ./parse/internal/testes -v 2>&1 | grep -E "^(PASS|PARSE_ERROR|=== RUN)"
```

Expected after fixes: More "PASS" lines, fewer "PARSE_ERROR" lines.

## Contract File

The source of truth for the parse API is:
- `parse/api/api.go` - Parser interface
- `lex/api/api.go` - Lexer interface
- `ast/api/api.go` - AST node interfaces

Do NOT modify these files. Implementation must conform to these interfaces.
