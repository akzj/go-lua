# Standard Library Implementation Design

## Overview

This document describes the architecture for implementing the Lua 5.x standard library modules (string, table, math, io) in the go-lua VM.

## Current State

- `pkg/api/stdlib.go` contains 12 basic functions registered directly as globals
- Functions use `State.Register(name, fn)` to register in the global environment
- Each function follows the `func(L *State) int` signature (returns number of results)

## Architecture

### Module Registration Pattern

Lua 5.x organizes stdlib as module tables. Each module (e.g., `string`) is a table with functions as fields:

```lua
-- Lua code
print(string.len("hello"))  -- 5
print(string.sub("hello", 2, 4))  -- "ell"
```

We need a `RegisterModule` function that:
1. Creates a new table for the module
2. Populates it with function fields
3. Registers the table as a global

### API Design

```go
// RegisterModule creates a module table and registers it globally
func (s *State) RegisterModule(name string, funcs map[string]Function) {
    s.NewTable()  // Create module table on stack
    for fname, fn := range funcs {
        s.PushFunction(fn)
        s.SetField(-2, fname)  // table.fname = fn
    }
    s.SetGlobal(name)  // Register module table as global
}
```

### File Organization

```
pkg/api/
├── stdlib.go      # Existing basic functions + OpenLibs
├── stdlib_string.go  # string module functions
├── stdlib_table.go   # table module functions  
├── stdlib_math.go    # math module functions
├── stdlib_io.go      # io module functions + file handle type
```

## Module Implementations

### 1. String Module (`stdlib_string.go`)

Functions to implement:
- `len(s)` - string length
- `sub(s, i, j)` - substring
- `upper(s)` - uppercase
- `lower(s)` - lowercase
- `find(s, pattern, init, plain)` - find pattern (simple string match for now)
- `format(format, ...)` - sprintf-like formatting
- `rep(s, n)` - repeat string
- `reverse(s)` - reverse string
- `byte(s, i, j)` - character codes
- `char(...)` - characters from codes
- `gsub(s, pattern, repl, n)` - global substitution (simple match for now)
- `match(s, pattern, init)` - pattern match (simple match for now)
- `gmatch(s, pattern)` - pattern iterator (simple match for now)

**Implementation notes:**
- Use Go's `strings` package for most operations
- Simple string matching (not full Lua patterns) per constraints
- Lua uses 1-based indexing

### 2. Table Module (`stdlib_table.go`)

Functions to implement:
- `insert(t, pos, value)` / `insert(t, value)` - insert element
- `remove(t, pos)` - remove element
- `sort(t, comp)` - sort in place
- `concat(t, sep, i, j)` - join elements
- `pack(...)` - pack arguments into table with n field
- `unpack(t, i, j)` - unpack table to multiple values

**Implementation notes:**
- Use `t.Len()` for array length
- Use `t.GetI()` / `t.SetI()` for array access
- `sort` needs a comparison function (default: `<` operator)

### 3. Math Module (`stdlib_math.go`)

Functions to implement:
- `abs(x)` - absolute value
- `ceil(x)` - ceiling
- `floor(x)` - floor
- `max(x, ...)` - maximum
- `min(x, ...)` - minimum
- `sqrt(x)` - square root
- `pow(x, y)` - power
- `random()` / `random(n)` / `random(m, n)` - random number
- `sin(x)`, `cos(x)`, `tan(x)` - trigonometric

Constants:
- `pi` - π value
- `huge` - +Inf

**Implementation notes:**
- Use Go's `math` package
- Use `math/rand` for random (seed with time)
- `huge` = `math.MaxFloat64`

### 4. IO Module (`stdlib_io.go`)

Functions to implement:
- `open(filename, mode)` - open file, returns file handle or nil, err
- `lines(filename)` - iterator over file lines
- `input(file)` - get/set default input file
- `output(file)` - get/set default output file

File handle methods:
- `read(...)` - read from file
- `write(...)` - write to file
- `close()` - close file
- `lines()` - iterator

**File Handle Implementation:**

```go
// FileHandle wraps os.File for Lua
type FileHandle struct {
    file   *os.File
    closed bool
}

// Open functions register file handles as userdata
func openFile(L *State, filename, mode string) {
    // Create UserData with *FileHandle
    // Set metatable with __index for methods
}
```

**Implementation notes:**
- Use Go's `os` and `bufio` packages
- File handles are userdata with metatables
- Default input/output files stored in io module table
- `io.input()` and `io.output()` return default files

## Integration with OpenLibs

Update `OpenLibs()` in `stdlib.go`:

```go
func (s *State) OpenLibs() {
    // Basic functions (existing)
    s.Register("print", stdPrint)
    s.Register("type", stdType)
    // ... existing functions ...
    
    // Module tables (new)
    s.openStringLib()
    s.openTableLib()
    s.openMathLib()
    s.openIOLib()
}
```

## Testing Strategy

Each module should have corresponding tests:
- `stdlib_string_test.go`
- `stdlib_table_test.go`
- `stdlib_math_test.go`
- `stdlib_io_test.go`

Tests should verify acceptance criteria from the task specification.

## Error Handling

- Use `L.PushNil()` + `L.PushString(errMsg)` for errors
- Return 2 for error results
- Use `L.Error()` for fatal errors

## Lua 5.4 Semantics

Key differences to handle:
- 1-based indexing for strings and arrays
- `string.sub(s, -1)` means last character
- `table.insert(t, value)` appends to end
- `math.random()` returns [0, 1)
- File handles have `:method()` syntax (metatable __index)

## Implementation Order

1. **string module** - most commonly used, straightforward implementation
2. **table module** - depends on table operations already in place
3. **math module** - simple wrappers around Go math package
4. **io module** - most complex, requires userdata and file handling