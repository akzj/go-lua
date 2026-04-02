# lex module Bug Fix Specification

## Problem
11/34 lua-master/testes files fail with lexer errors:
- `malformed number` — bitwise.lua, code.lua, files.lua, math.lua
- `unfinished string` — events.lua, gengc.lua, main.lua, strings.lua, tpack.lua, utf8.lua
- `invalid escape sequence` — literals.lua

## Root Causes

### 1. Missing Escape Sequences
lua-master/llex.c `read_string()` supports: `\a`, `\b`, `\f`, `\v`

### 2. Invalid Long String Delimiter
When `[=...` without matching `]=...`, lua-master errors with "invalid long string delimiter"

### 3. Hex Float Edge Cases
Hex floats like `0x1.2p3`, `0xABCp-3`

## Files to Modify
- `lex/internal/lexer.go`

## Verification
```bash
go test ./lex/...  # All 34 files must pass
```
