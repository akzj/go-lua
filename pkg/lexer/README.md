# Lua Lexer Package

The `lexer` package implements a lexical analyzer for the Lua programming language. It scans Lua source code and produces a stream of tokens for the parser.

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/your-org/go-lua/pkg/lexer"
)

func main() {
    source := []byte(`x = 42 + 3.14`)
    lex := lexer.NewLexer(source, "example.lua")
    
    for {
        tok, err := lex.NextToken()
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            return
        }
        if tok.Type == lexer.TK_EOF {
            break
        }
        fmt.Printf("Type: %v, Value: %v, Line: %d, Col: %d\n", 
            tok.Type, tok.Value, tok.Line, tok.Column)
    }
}
```

## Token Structure

### Token Type

```go
type Token struct {
    Type   TokenType   // Token category
    Value  interface{} // Token value (string, int64, float64)
    Line   int         // Line number (1-based)
    Column int         // Column number (0-based)
}
```

### Token Value Types

| Token Type | Value Type | Description |
|------------|------------|-------------|
| `TK_NAME` | `string` | Identifier name |
| `TK_STRING` | `string` | String literal (unquoted) |
| `TK_INT` | `int64` | Integer literal |
| `TK_FLOAT` | `float64` | Floating-point literal |
| Others | `nil` | Keywords and operators have no value |

## Token Types

### Literals

| Constant | Description | Example |
|----------|-------------|---------|
| `TK_EOF` | End of file | - |
| `TK_NAME` | Identifier | `myVar`, `function_name` |
| `TK_STRING` | String literal | `"hello"`, `'world'` |
| `TK_INT` | Integer literal | `42`, `0xFF`, `0b1010` |
| `TK_FLOAT` | Float literal | `3.14`, `1e-5`, `0x1.5p+2` |

### Reserved Words (Keywords)

| Keywords | | | |
|----------|-|-|-|
| `TK_AND` | `TK_BREAK` | `TK_DO` | `TK_ELSE` |
| `TK_ELSEIF` | `TK_END` | `TK_FALSE` | `TK_FOR` |
| `TK_FUNCTION` | `TK_GLOBAL` | `TK_GOTO` | `TK_IF` |
| `TK_IN` | `TK_LOCAL` | `TK_NIL` | `TK_NOT` |
| `TK_OR` | `TK_REPEAT` | `TK_RETURN` | `TK_THEN` |
| `TK_TRUE` | `TK_UNTIL` | `TK_WHILE` | |

### Operators

| Constant | Token | Description |
|----------|-------|-------------|
| `TK_PLUS` | `+` | Addition |
| `TK_MINUS` | `-` | Subtraction |
| `TK_STAR` | `*` | Multiplication |
| `TK_SLASH` | `/` | Division |
| `TK_PERCENT` | `%` | Modulo |
| `TK_CARET` | `^` | Exponentiation |
| `TK_HASH` | `#` | Length operator |
| `TK_IDIV` | `//` | Integer division |
| `TK_CONCAT` | `..` | String concatenation |
| `TK_DOTS` | `...` | Vararg |
| `TK_EQ` | `==` | Equality |
| `TK_NE` | `~=` | Not equal |
| `TK_LT` | `<` | Less than |
| `TK_LE` | `<=` | Less than or equal |
| `TK_GT` | `>` | Greater than |
| `TK_GE` | `>=` | Greater than or equal |
| `TK_SHL` | `<<` | Left shift |
| `TK_SHR` | `>>` | Right shift |

### Delimiters

| Constant | Token | Description |
|----------|-------|-------------|
| `TK_LPAREN` | `(` | Left parenthesis |
| `TK_RPAREN` | `)` | Right parenthesis |
| `TK_LBRACE` | `{` | Left brace |
| `TK_RBRACE` | `}` | Right brace |
| `TK_LBRACK` | `[` | Left bracket |
| `TK_RBRACK` | `]` | Right bracket |
| `TK_SEMICOLON` | `;` | Semicolon |
| `TK_COLON` | `:` | Colon |
| `TK_DBCOLON` | `::` | Label delimiter |
| `TK_COMMA` | `,` | Comma |
| `TK_DOT` | `.` | Dot/period |

## API Reference

### NewLexer

```go
func NewLexer(source []byte, name string) *Lexer
```

Creates a new lexer for the given source code.

**Parameters:**
- `source`: The Lua source code as a byte slice
- `name`: Source name for error messages (typically filename)

**Returns:** A new `*Lexer` instance

### NextToken

```go
func (l *Lexer) NextToken() (Token, error)
```

Returns the next token from the source. Scans whitespace and comments automatically.

**Returns:**
- `Token`: The next token (TK_EOF at end of source)
- `error`: Lexical error if encountered (e.g., unfinished string)

**Example:**
```go
lex := lexer.NewLexer([]byte("x = 1"), "test.lua")
tok, err := lex.NextToken()
if err != nil {
    // Handle error
}
```

### Peek

```go
func (l *Lexer) Peek() Token
```

Returns the next token without consuming it. Useful for lookahead.

**Note:** This is a simplified implementation that buffers the token internally.

### Error

```go
func (l *Lexer) Error(format string, args ...interface{}) error
```

Creates a lexer error with the current position. Format: `filename:line: message`

## Line and Column Tracking

The lexer tracks position information for every token:

- **Line**: 1-based line number (starts at 1)
- **Column**: 0-based column offset within the line

```go
source := []byte(`x = 1
y = 2`)
lex := lexer.NewLexer(source, "test.lua")

tok1, _ := lex.NextToken() // TK_NAME "x" at Line: 1, Column: 0
tok2, _ := lex.NextToken() // TK_PLUS "=" at Line: 1, Column: 2
tok3, _ := lex.NextToken() // TK_INT 1 at Line: 1, Column: 4
tok4, _ := lex.NextToken() // TK_NAME "y" at Line: 2, Column: 0
```

## Error Handling

The lexer returns errors for malformed input:

```go
source := []byte(`"unfinished string`)
lex := lexer.NewLexer(source, "bad.lua")

tok, err := lex.NextToken()
if err != nil {
    // Error: bad.lua:1: unfinished string
    fmt.Println(err)
}
```

**Common Errors:**
- `unfinished string` - String literal not closed
- `unfinished string (newline in string)` - Newline in short string
- `invalid escape sequence` - Unknown escape in string
- `hexadecimal digit expected` - Invalid hex escape
- `malformed number` - Invalid numeric literal
- `unexpected character 'X'` - Unrecognized character

## Complete Example: Tokenize Lua Function

```go
package main

import (
    "fmt"
    "github.com/your-org/go-lua/pkg/lexer"
)

func main() {
    source := []byte(`
function factorial(n)
    if n <= 1 then
        return 1
    end
    return n * factorial(n - 1)
end
`)
    lex := lexer.NewLexer(source, "factorial.lua")
    
    fmt.Println("Tokens:")
    for {
        tok, err := lex.NextToken()
        if err != nil {
            fmt.Printf("Error at line %d: %v\n", tok.Line, err)
            return
        }
        if tok.Type == lexer.TK_EOF {
            break
        }
        
        // Format value display
        value := ""
        if tok.Value != nil {
            value = fmt.Sprintf(" (%v)", tok.Value)
        }
        
        fmt.Printf("Line %2d, Col %2d: %-10s%s\n", 
            tok.Line, tok.Column, tok.Type, value)
    }
}
```

**Output:**
```
Tokens:
Line  2, Col  0: TK_FUNCTION 
Line  2, Col 10: TK_NAME     (factorial)
Line  2, Col 20: TK_LPAREN   
Line  2, Col 21: TK_NAME     (n)
Line  2, Col 22: TK_RPAREN   
Line  2, Col 23: TK_LBRACE   
Line  3, Col  4: TK_IF       
Line  3, Col  7: TK_NAME     (n)
Line  3, Col  9: TK_LE       
Line  3, Col 12: TK_INT      (1)
Line  3, Col 14: TK_THEN     
Line  4, Col  8: TK_RETURN   
Line  4, Col 15: TK_INT      (1)
...
```

## Related Packages

- `pkg/parser` - Syntax analyzer that consumes lexer tokens
- `pkg/lexer/keywords.go` - Reserved word definitions