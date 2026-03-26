# Phase E: Lua Test Suite Compatibility

## Overview

This document describes the design for making the go-lua implementation compatible with the official Lua test suite at `lua-master/testes/`.

## Goals

1. Add `do...end` block parsing support
2. Handle shebang lines (`#!`) at the start of files
3. Create test infrastructure to run and track Lua test suite results
4. Achieve at least 5 passing test files

## Design

### 1. Parser Fix: `do...end` Block Parsing

**Problem**: Standalone `do...end` blocks are not parsed. The parser only handles `do` in the context of `while...do`, `for...do`, etc.

**Location**: `pkg/parser/parser.go` and `pkg/parser/stmt.go`

**Solution**: Add a new case in `parseStmt()` for `TK_DO` and implement `parseDoBlock()`.

**AST Node**: Use existing `BlockStmt` - a standalone `do...end` is just a block with its own scope.

**Implementation**:

```go
// In pkg/parser/parser.go, add to parseStmt() switch:
case lexer.TK_DO:
    return p.parseDoBlock()

// In pkg/parser/stmt.go, add new function:
func (p *Parser) parseDoBlock() Stmt {
    line := p.Current.Line
    p.advance() // Skip 'do'
    
    body := p.parseBlock()
    
    if !p.expect(lexer.TK_END, "'end'") {
        p.sync()
        return nil
    }
    
    return &DoStmt{
        baseStmt: baseStmt{line: line},
        Body:     body,
    }
}
```

**AST Node Addition** (in `pkg/parser/ast.go`):

```go
// DoStmt represents a standalone do...end block.
type DoStmt struct {
    baseStmt
    Body *BlockStmt
}

func (s *DoStmt) stmtNode() {}
```

### 2. Lexer Fix: Shebang Line Handling

**Problem**: Files starting with `#!` (shebang) cause parse errors because `#` is treated as the length operator.

**Location**: `pkg/lexer/lexer.go`

**Solution**: At the start of lexing (first token only), check if the source starts with `#!` and skip to the end of the line.

**Implementation**:

```go
// In Lexer struct, add a field:
type Lexer struct {
    Scanner
    name       string
    buffer     strings.Builder
    atStart    bool  // NEW: track if we're at the start of the file
}

// In NewLexer:
func NewLexer(source []byte, name string) *Lexer {
    return &Lexer{
        Scanner: *NewScanner(source),
        name:    name,
        atStart: true,  // NEW
    }
}

// In NextToken(), at the beginning of the main loop:
func (l *Lexer) NextToken() (Token, error) {
    l.buffer.Reset()

    // NEW: Handle shebang at start of file
    if l.atStart && l.current == '#' && l.Peek1() == '!' {
        l.atStart = false
        // Skip the entire shebang line
        for !isNewline(l.current) && l.current != 0 {
            l.Advance()
        }
        // Skip the newline
        if isNewline(l.current) {
            l.skipNewline()
        }
    }
    l.atStart = false  // Mark that we've started

    // ... rest of existing code
}
```

### 3. Lua 5.5 Syntax Handling

**Problem**: Test files use Lua 5.5 syntax like `global <const> *` declarations.

**Solution**: Preprocess test files to strip `global` declarations before running.

**Implementation**: Create a preprocessor function in the test runner.

```go
// Strip Lua 5.5 global declarations from source
func preprocessLua55(source string) string {
    lines := strings.Split(source, "\n")
    var result []string
    for _, line := range lines {
        // Skip global declarations
        trimmed := strings.TrimSpace(line)
        if strings.HasPrefix(trimmed, "global ") {
            continue
        }
        result = append(result, line)
    }
    return strings.Join(result, "\n")
}
```

### 4. Test Infrastructure

**Location**: Create `tests/suite_test.go`

**Design**:
- Test runner that loads each test file from `lua-master/testes/`
- Preprocesses Lua 5.5 syntax
- Runs tests and captures pass/fail status
- Reports results in a structured format

**Test Runner Structure**:

```go
// tests/suite_test.go
package tests

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
    
    "github.com/akzj/go-lua/pkg/api"
)

// TestFile represents a single test file result
type TestFile struct {
    Name   string
    Status string // "pass", "fail", "skip"
    Error  string
}

// RunLuaTestSuite runs all test files
func RunLuaTestSuite(t *testing.T) map[string]TestFile {
    testDir := "../lua-master/testes"
    results := make(map[string]TestFile)
    
    files, _ := filepath.Glob(filepath.Join(testDir, "*.lua"))
    for _, file := range files {
        name := filepath.Base(file)
        // Skip all.lua (test runner itself)
        if name == "all.lua" {
            continue
        }
        
        result := runTestFile(file)
        results[name] = result
    }
    
    return results
}

func runTestFile(path string) TestFile {
    // 1. Read file
    // 2. Preprocess Lua 5.5 syntax
    // 3. Run with API
    // 4. Capture result
}
```

## Test Files Priority Order

Start with simpler tests and work up:

1. `literals.lua` - Basic literal parsing
2. `vararg.lua` - Vararg handling
3. `goto.lua` - Goto/label support
4. `strings.lua` - String operations
5. `math.lua` - Math library

Skip for now (require features not in scope):
- `coroutine.lua` - Requires coroutine support
- `db.lua` - Requires debug library
- `gc.lua` - Requires GC hooks
- `bitwise.lua` - May require bitwise ops

## Acceptance Criteria

1. Parser: `do print("x") end` parses without error
2. Lexer: Files starting with `#!` parse without error
3. Test runner: Can run individual test files and report pass/fail
4. At least 5 test files pass
5. All existing tests continue to pass

## Files to Modify

1. `pkg/parser/parser.go` - Add `TK_DO` case to `parseStmt()`
2. `pkg/parser/stmt.go` - Add `parseDoBlock()` function
3. `pkg/parser/ast.go` - Add `DoStmt` struct
4. `pkg/lexer/lexer.go` - Add shebang handling in `NextToken()`
5. `tests/suite_test.go` - Create test runner (new file)

## Implementation Order

1. **Lexer fix first** - Shebang handling (simple, low risk)
2. **Parser fix second** - `do...end` blocks (moderate complexity)
3. **Test infrastructure third** - Test runner with preprocessing
4. **Run tests** - Identify and track passing/failing tests