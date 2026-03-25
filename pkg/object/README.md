# Package object

Package `object` implements Lua's object system, providing the core data structures for representing Lua values.

## Overview

This package provides:
- **TValue**: The fundamental tagged value type in Lua
- **Type System**: Support for all Lua types (nil, boolean, number, string, table, function, userdata, thread)
- **GC Interfaces**: Garbage collection interfaces for collectable objects
- **Function Prototypes**: Structures for representing compiled Lua functions
- **Upvalues**: Support for closure upvalues

## TValue Type System

`TValue` is the core value representation in the Lua VM. Every value on the stack, in tables, or in registers is a `TValue`.

### Creating Values

```go
// Create nil value
v := object.NewNil()

// Create boolean values
v = object.NewBoolean(true)
v = object.NewBoolean(false)

// Create number values
v = object.NewNumber(42.5)    // Float
v = object.NewInteger(100)     // Integer

// Create string value
v = object.NewString("hello")

// Create table value
table := object.NewTable()
v = object.NewTableValue(table)

// Create function value
closure := &object.Closure{IsGo: true}
v = object.NewFunction(closure)

// Create userdata value
ud := &object.UserData{Value: myGoValue}
v = object.NewUserData(ud)

// Create thread value
thread := &object.Thread{}
v = object.NewThread(thread)

// Create light userdata value
v = object.NewLightUserData(unsafe.Pointer(ptr))
```

### Type Checking

```go
v := object.NewNumber(42.0)

// Check type
if v.IsNumber() {
    fmt.Println("It's a number")
}

if v.IsNil() {
    fmt.Println("It's nil")
}

// Check if collectable
if v.IsCollectable() {
    fmt.Println("GC will handle this")
}
```

### Type Conversion

```go
// Convert to number
v := object.NewNumber(42.0)
n, ok := v.ToNumber()
if ok {
    fmt.Println("Number:", n)
}

// Convert to string
v = object.NewString("hello")
s, ok := v.ToString()
if ok {
    fmt.Println("String:", s)
}

// Convert to boolean
v = object.NewBoolean(true)
b, ok := v.ToBoolean()
if ok {
    fmt.Println("Boolean:", b)
}

// Convert to table
table := object.NewTable()
v = object.NewTableValue(table)
t, ok := v.ToTable()
if ok {
    fmt.Println("Table:", t)
}

// Convert to function
closure := &object.Closure{}
v = object.NewFunction(closure)
fn, ok := v.ToFunction()
if ok {
    fmt.Println("Function:", fn)
}
```

### Modifying Values

```go
v := object.NewNil()

// Set different types
v.SetBoolean(true)
v.SetNumber(42.0)
v.SetInteger(100)
v.SetString("hello")
v.SetTable(table)
v.SetFunction(closure)

// Copy from another value
src := object.NewString("world")
v.CopyFrom(src)

// Clear to nil
v.Clear()
```

### Lua Truthiness

In Lua, only `nil` and `false` are "falsy" values. All other values (including `0`, `""`, and empty tables) are truthy.

```go
// Check if value is falsy in Lua terms
v := object.NewNumber(0)
if object.IsFalse(v) {
    // This won't execute - 0 is truthy in Lua
}

v = object.NewNil()
if object.IsFalse(v) {
    // This will execute - nil is falsy
}

v = object.NewBoolean(false)
if object.IsFalse(v) {
    // This will execute - false is falsy
}
```

### Value Comparison

```go
v1 := object.NewNumber(42.0)
v2 := object.NewNumber(42.0)

if object.Equal(v1, v2) {
    fmt.Println("Values are equal")
}

// Note: Equal performs shallow comparison
// For tables and functions, it compares by reference
```

### String Representation

```go
// Get type name
v := object.NewNumber(42.0)
typeName := object.TypeName(v)  // "number"

// Convert any value to string representation
v = object.NewNumber(42.0)
s := object.ToStringRaw(v)  // "42"

v = object.NewBoolean(true)
s = object.ToStringRaw(v)  // "true"

v = object.NewTableValue(object.NewTable())
s = object.ToStringRaw(v)  // "table: 0x..."
```

## Supported Types

### TypeNil
Represents the Lua `nil` value.

### TypeBoolean
Represents boolean values (`true` or `false`).

### TypeNumber
Represents numeric values. Internally stored as `float64`, with optional `int64` for integer optimization.

### TypeString
Represents string values. Strings are interned for memory efficiency.

### TypeTable
Represents Lua tables - the primary data structure in Lua.

### TypeFunction
Represents function values. Can be either:
- **Lua closures**: Functions defined in Lua code
- **Go closures**: Functions defined in Go

### TypeUserData
Represents userdata - arbitrary Go data stored in Lua.

### TypeThread
Represents Lua threads (coroutines).

### TypeLightUserData
Represents light userdata - pointer-like values without GC.

### TypeProto
Represents function prototypes (pre-compiled functions).

### TypeUpValue
Represents upvalue references in closures.

## Garbage Collection

All collectable objects implement the `GCObject` interface:

```go
type GCObject interface {
    gcObject()
}
```

### Collectable Types

The following types implement `GCObject`:
- `*Table`
- `*Closure`
- `*UserData`
- `*Thread`
- `*Prototype`
- `*Upvalue`
- `*GCString`

### Example

```go
// Check if a value is collectable
v := object.NewTable(object.NewTable())
if v.IsCollectable() {
    fmt.Println("This value will be garbage collected")
}
```

## Function Prototypes

`Prototype` represents a compiled Lua function:

```go
proto := &object.Prototype{
    Code:         []object.Instruction{...},  // Bytecode
    Constants:    []object.TValue{...},       // Constant table
    Upvalues:     []object.UpvalueDesc{...},  // Upvalue info
    Prototypes:   []*object.Prototype{...},   // Nested prototypes
    Source:       "main.lua",                 // Source file
    LineInfo:     []int{...},                 // Line numbers
    NumParams:    2,                          // Parameter count
    IsVarArg:     false,                      // Vararg function
    MaxStackSize: 10,                         // Stack size
}
```

## Upvalues

`Upvalue` represents a reference to a local variable from an enclosing function:

```go
// Create an upvalue
v := object.NewNumber(42.0)
upval := object.NewUpvalue(0, v)

// Get the value
current := upval.Get()

// Set the value
upval.Set(object.NewNumber(100.0))

// Close the upvalue (cache the value)
upval.Close()
```

## Tables

`Table` represents a Lua table:

```go
// Create a new table
table := object.NewTable()

// Tables are GC objects
var _ object.GCObject = table
```

## Closures

`Closure` represents a Lua function closure:

```go
// Create a Go closure
goFn := func(vm interface{}) error {
    // Go function implementation
    return nil
}

closure := &object.Closure{
    IsGo:   true,
    GoFn:   goFn,
    Upvalues: []*object.Upvalue{...},
}

// Create a Lua closure
closure := &object.Closure{
    IsGo:   false,
    Proto:  proto,  // Function prototype
    Upvalues: []*object.Upvalue{...},
}
```

## Best Practices

1. **Use factory functions**: Prefer `NewNumber()`, `NewString()`, etc. over manual construction
2. **Check conversion results**: Always check the `ok` return value from conversion methods
3. **Understand Lua truthiness**: Use `IsFalse()` for Lua-style boolean conversion
4. **Manage upvalues**: Remember to close upvalues when variables go out of scope
5. **Type safety**: Use type checking methods before conversion

## Examples

### Complete Example

```go
package main

import (
    "fmt"
    "your-module/pkg/object"
)

func main() {
    // Create values
    num := object.NewNumber(42.0)
    str := object.NewString("hello")
    tbl := object.NewTable(object.NewTable())
    
    // Type checking
    fmt.Println("Is number:", num.IsNumber())  // true
    fmt.Println("Is string:", num.IsString())  // false
    
    // Type conversion
    n, ok := num.ToNumber()
    if ok {
        fmt.Println("Number value:", n)
    }
    
    // Lua truthiness
    fmt.Println("Is false (num):", object.IsFalse(num))  // false
    fmt.Println("Is false (nil):", object.IsFalse(object.NewNil()))  // true
    
    // Value comparison
    num2 := object.NewNumber(42.0)
    fmt.Println("Equal:", object.Equal(num, num2))  // true
    
    // String representation
    fmt.Println("Type name:", object.TypeName(num))  // "number"
    fmt.Println("String repr:", object.ToStringRaw(num))  // "42"
}
```

## Testing

Run tests with:

```bash
go test ./pkg/object -v
go test ./pkg/object -cover
```

## References

- Architecture design: `docs/architecture.md`
- Lua reference: `lua-master/lobject.h`, `lua-master/lobject.c`
- Lua 5.4 Reference Manual: https://www.lua.org/manual/5.4/