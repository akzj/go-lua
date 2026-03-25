# pkg/table - Lua Table Implementation

This package implements Lua tables, the primary data structure in Lua. Tables are hybrid array-map structures that support both integer-indexed array access and arbitrary-key hash access.

## Features

- **Hybrid Storage**: Efficient array storage for integer keys (1, 2, 3, ...) and flexible map storage for all other key types
- **Array Access**: O(1) access for integer keys using the array part
- **Hash Access**: O(1) average access for string, number, boolean, and other keys using the map part
- **Metatables**: Support for metatables to customize table behavior
- **Iteration**: Support for `pairs()` and `ipairs()` style iteration via the `Next()` method
- **Length Operator**: Implements Lua's `#` operator semantics

## Basic Usage

### Creating Tables

```go
import "github.com/akzj/go-lua/pkg/table"
import "github.com/akzj/go-lua/pkg/object"

// Create an empty table
t := table.NewTable(0, 0)

// Create a table with pre-allocated array space (for sequential integer keys)
t := table.NewTable(10, 0)

// Create a table with pre-allocated map space (for hash access)
t := table.NewTable(0, 10)
```

### Array Access

```go
// Set values at integer indices (1-based, Lua-style)
t.SetI(1, *object.NewString("hello"))
t.SetI(2, *object.NewString("world"))

// Get values by integer index
val := t.GetI(1)  // Returns *object.TValue containing "hello"

// Array access through Get/Set also works
t.Set(*object.NewInteger(3), *object.NewNumber(42.0))
val = t.Get(*object.NewInteger(3))
```

### Hash Access

```go
// String keys
t.Set(*object.NewString("name"), *object.NewString("Lua"))
t.Set(*object.NewString("version"), *object.NewNumber(5.4))

// Boolean keys
t.Set(*object.NewBoolean(true), *object.NewString("truthy"))

// Number keys (non-integer)
t.Set(*object.NewNumber(3.14), *object.NewString("pi"))

// Retrieve values
name := t.Get(*object.NewString("name"))
```

### Mixed Access

```go
// Tables can mix array and hash access
t := table.NewTable(0, 0)

// Array part
t.SetI(1, *object.NewString("first"))
t.SetI(2, *object.NewString("second"))

// Hash part
t.Set(*object.NewString("key"), *object.NewNumber(100.0))

// Check if table is array-only
if t.IsArray {
    // Table only has array part
}
```

### Length Operator

```go
// Get the length of the array part (# operator semantics)
t := table.NewTable(0, 0)
t.SetI(1, *object.NewInteger(10))
t.SetI(2, *object.NewInteger(20))
t.SetI(3, *object.NewInteger(30))

length := t.Len()  // Returns 3
```

### Iteration

```go
// Iterate over all key-value pairs (pairs semantics)
for k, v := t.Next(nil); k != nil; k, v = t.Next(k) {
    // Process key k and value v
    // k and v are *object.TValue
}

// Iterate over array part only (ipairs semantics)
for i := 1; i <= t.Len(); i++ {
    v := t.GetI(i)
    if v != nil && v.Type != object.TypeNil {
        // Process array element
    }
}
```

### Metatables

```go
// Create a metatable
mt := table.NewTable(0, 0)
mt.Set(*object.NewString("__index"), *object.NewString("fallback"))

// Set metatable on a table
t := table.NewTable(0, 0)
t.SetMetatable(mt)

// Get metatable
currentMt := t.GetMetatable()

// Remove metatable
t.SetMetatable(nil)
```

### Utility Methods

```go
// Check if table is empty
if t.IsEmpty() {
    // Table has no elements
}

// Clear all elements (preserves metatable)
t.Clear()

// After Clear(), table is empty but metatable is preserved
```

## Implementation Details

### Hybrid Array-Map Structure

The Table implementation uses a hybrid structure for optimal performance:

- **Array Part**: A Go slice (`[]object.TValue`) stores values for integer keys 1..N. This provides O(1) access for array-like usage.
- **Map Part**: A Go map (`map[valueKey]*object.TValue`) stores values for all other keys (strings, floats, booleans, objects).

### Key Types

The map part supports all Lua value types as keys:

- `nil`
- Booleans (`true`, `false`)
- Numbers (integers and floats)
- Strings
- Light userdata
- GC objects (tables, functions, userdata, threads, prototypes, upvalues)

### Iteration Order

- **Array Part**: Iterated in sequential order (1, 2, 3, ...)
- **Map Part**: Iterated in deterministic order based on key type and value (sorted for consistency)

### Memory Management

The Table struct implements the `object.GCObject` interface, allowing it to be tracked by the garbage collector.

## API Reference

### Types

```go
type Table struct {
    Array     []object.TValue      // Array part for integer keys
    Map       map[valueKey]*object.TValue  // Map part for other keys
    Metatable *Table               // Optional metatable
    IsArray   bool                 // True if only has array part
    Length    int                  // Cached length (may be stale)
}
```

### Functions

```go
// NewTable creates a new table with optional pre-allocation
func NewTable(arraySize, mapSize int) *Table
```

### Methods

```go
// Get retrieves a value from the table by key
func (t *Table) Get(key object.TValue) *object.TValue

// GetI retrieves a value by integer index (optimized for array access)
func (t *Table) GetI(idx int) *object.TValue

// Set sets a value in the table for the given key
func (t *Table) Set(key, value object.TValue)

// SetI sets a value at the given integer index (optimized for array access)
func (t *Table) SetI(idx int, value object.TValue)

// Len returns the length of the array part (# operator semantics)
func (t *Table) Len() int

// Next returns the next key-value pair for iteration
// Pass nil to start iteration
func (t *Table) Next(key *object.TValue) (*object.TValue, *object.TValue)

// GetMetatable returns the metatable
func (t *Table) GetMetatable() *Table

// SetMetatable sets the metatable
func (t *Table) SetMetatable(mt *Table)

// IsEmpty returns true if the table has no elements
func (t *Table) IsEmpty() bool

// Clear removes all elements from the table (preserves metatable)
func (t *Table) Clear()
```

## Testing

Run tests with:

```bash
go test -v ./pkg/table
```

Check test coverage:

```bash
go test -cover ./pkg/table
```

## Performance Considerations

- **Array Access**: O(1) for integer keys in the array part
- **Hash Access**: O(1) average case for map part keys
- **Length Calculation**: O(n) in worst case (finds last non-nil element)
- **Iteration**: O(n) to iterate all elements

### Optimization Tips

1. **Pre-allocate**: If you know the approximate size, use `NewTable(arraySize, mapSize)` to pre-allocate space
2. **Use SetI/GetI**: For integer keys, use `SetI`/`GetI` instead of `Set`/`Get` for better performance
3. **Avoid Holes**: For array-like tables, avoid setting nil values in the middle of the array

## References

- [Lua 5.4 Reference Manual - Tables](https://www.lua.org/manual/5.4/manual.html#3.4.9)
- Lua C implementation: `lua-master/ltable.c`, `lua-master/ltable.h`
- Architecture design: `docs/architecture.md` - "Table Implementation" section