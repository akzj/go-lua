// Package table implements Lua tables.
//
// Lua tables are hybrid array-map structures that serve as the primary
// data structure in Lua. They support:
//   - Array-like access with integer keys (1-indexed)
//   - Hash-like access with arbitrary keys
//   - Metatables for operator overloading
//   - Iteration via pairs() and ipairs()
//
// # Implementation Details
//
// The Table implementation uses a hybrid structure:
//   - Array part: Stores values for integer keys 1..N efficiently
//   - Map part: Stores values for all other keys (strings, floats, objects)
//
// # Example Usage
//
//	// Create a new table
//	t := table.NewTable(0, 0)
//
//	// Array access
//	t.Set(object.NewInteger(1), object.NewString("hello"))
//	v := t.Get(object.NewInteger(1))  // Returns "hello"
//
//	// Hash access
//	t.Set(object.NewString("key"), object.NewNumber(42.0))
//	v = t.Get(object.NewString("key"))  // Returns 42.0
//
//	// Length operator
//	len := t.Len()  // Returns array length
//
//	// Iteration
//	for k, v := t.Next(nil); k != nil; k, v = t.Next(k) {
//	    // Process key-value pair
//	}
package table

import (
	"sort"
	"unsafe"

	"github.com/akzj/go-lua/pkg/object"
)

// Table represents a Lua table (hybrid array/map).
//
// Tables are the primary data structure in Lua, used for arrays,
// dictionaries, objects, and modules. This implementation uses a
// hybrid array-map structure for performance:
//   - Array part: Efficient storage for integer keys 1..N
//   - Map part: Flexible storage for all other key types
//
// The Table struct implements the object.GCObject interface for
// garbage collection.
type Table struct {
	// Array part for integer keys 1..N
	Array []object.TValue

	// Map part for other keys
	Map map[valueKey]*object.TValue

	// Metatable for operator overloading
	Metatable *Table

	// Flags
	IsArray bool // True if only has array part
	Length  int  // Cached length (may be stale, use Len() to get accurate value)
}

// valueKey is a comparable key for the map.
//
// This struct serves as a hash key for the map part of the table.
// It captures the essential information needed to compare Lua values:
//   - Type: The Lua type tag
//   - Num: Numeric value (for numbers and type discrimination)
//   - Str: String value (for string keys)
//   - Ptr: Pointer value (for light userdata and GC objects)
//
// The struct is designed to be comparable (no slices, maps, or functions).
type valueKey struct {
	Type  object.Type
	Num   float64
	Str   string
	Ptr   uintptr
}

// gcObjectPtr returns a unique pointer value for a GC object.
//
// This function extracts a unique identifier for GC objects by
// using their pointer address. This allows GC objects to be used
// as map keys.
func gcObjectPtr(obj object.GCObject) uintptr {
	if obj == nil {
		return 0
	}
	// Use type assertion to get the concrete pointer
	// All GC objects are pointers, so we can use their address
	switch o := obj.(type) {
	case *object.Table:
		return uintptr(*(*uintptr)(unsafe.Pointer(&o)))
	case *object.Closure:
		return uintptr(*(*uintptr)(unsafe.Pointer(&o)))
	case *object.UserData:
		return uintptr(*(*uintptr)(unsafe.Pointer(&o)))
	case *object.Thread:
		return uintptr(*(*uintptr)(unsafe.Pointer(&o)))
	case *object.Prototype:
		return uintptr(*(*uintptr)(unsafe.Pointer(&o)))
	case *object.Upvalue:
		return uintptr(*(*uintptr)(unsafe.Pointer(&o)))
	case *object.GCString:
		return uintptr(*(*uintptr)(unsafe.Pointer(&o)))
	default:
		return 0
	}
}

// newValueKey creates a valueKey from a TValue.
//
// This function extracts the essential information from a TValue
// to create a comparable key for the map. For GC objects, it uses
// the pointer address as the unique identifier.
func newValueKey(key object.TValue) valueKey {
	vk := valueKey{
		Type: key.Type,
	}

	switch key.Type {
	case object.TypeNil:
		// Nil keys are not typically used, but handle gracefully
		vk.Num = 0
	case object.TypeBoolean:
		if key.Value.Bool {
			vk.Num = 1
		} else {
			vk.Num = 0
		}
	case object.TypeNumber:
		vk.Num = key.Value.Num
	case object.TypeString:
		vk.Str = key.Value.Str
	case object.TypeLightUserData:
		// For light userdata, we store the pointer directly
		// Note: This is a simplification
		vk.Ptr = 0
	case object.TypeTable, object.TypeFunction, object.TypeUserData,
		object.TypeThread, object.TypeProto, object.TypeUpValue:
		// Use the GC object pointer as unique identifier
		if key.Value.GC != nil {
			vk.Ptr = gcObjectPtr(key.Value.GC)
		}
	}

	return vk
}

// NewTable creates a new table with optional pre-allocation.
//
// Parameters:
//   - arraySize: Initial capacity for the array part (for integer keys)
//   - mapSize: Initial capacity for the map part (for other keys)
//
// Returns a pointer to a new Table instance.
//
// Example:
//
//	// Create a table optimized for array use
//	t := table.NewTable(10, 0)
//
//	// Create a table optimized for hash use
//	t := table.NewTable(0, 10)
//
//	// Create an empty table
//	t := table.NewTable(0, 0)
func NewTable(arraySize, mapSize int) *Table {
	t := &Table{
		Array:   make([]object.TValue, 0, arraySize),
		Map:     make(map[valueKey]*object.TValue, mapSize),
		IsArray: mapSize == 0,
		Length:  0,
	}
	return t
}

// Get retrieves a value from the table by key.
//
// This method implements Lua's table access semantics:
//   - For integer keys in range [1, len(Array)], accesses the array part
//   - For all other keys, accesses the map part
//   - Returns nil if the key is not found
//
// Parameters:
//   - key: The key to look up (any Lua value type)
//
// Returns:
//   - *object.TValue: The value associated with the key, or nil if not found
//
// Note: This method does not invoke metamethods. For metamethod support,
// use the VM's table access operations.
func (t *Table) Get(key object.TValue) *object.TValue {
	// Check if key is an integer suitable for array access
	if key.Type == object.TypeNumber {
		// Check if it's an integer value
		num := key.Value.Num
		intVal := int64(num)
		
		// Check if the number is actually an integer
		if float64(intVal) == num {
			// Check if it's in the valid array range (1-indexed)
			if intVal >= 1 && int(intVal) <= len(t.Array) {
				// Array access (1-indexed, so subtract 1 for 0-indexed slice)
				return &t.Array[intVal-1]
			}
		}
	}
	
	// Map access for all other cases
	vk := newValueKey(key)
	if val, ok := t.Map[vk]; ok {
		return val
	}
	
	// Key not found
	return nil
}

// GetI retrieves a value from the table by integer index.
//
// This is an optimized version of Get for integer keys.
// It directly accesses the array part without type checking.
//
// Parameters:
//   - idx: The 1-based integer index
//
// Returns:
//   - *object.TValue: The value at the index, or nil if out of bounds
func (t *Table) GetI(idx int) *object.TValue {
	if idx >= 1 && idx <= len(t.Array) {
		return &t.Array[idx-1]
	}
	// Check map part for integer keys outside array range
	key := object.NewInteger(int64(idx))
	vk := newValueKey(*key)
	if v, ok := t.Map[vk]; ok {
		return v
	}
	return nil
}

// Set sets a value in the table for the given key.
//
// This method implements Lua's table assignment semantics:
//   - For integer keys in range [1, len(Array)+1], uses the array part
//   - For all other keys, uses the map part
//   - Updates existing values or inserts new ones
//
// Parameters:
//   - key: The key to set (any Lua value type)
//   - value: The value to associate with the key
//
// Note: This method does not invoke metamethods. For metamethod support,
// use the VM's table assignment operations.
//
// Note: Setting nil values does not remove keys. Use explicit removal
// if needed (future enhancement).
func (t *Table) Set(key, value object.TValue) {
	// Check if key is an integer suitable for array access
	if key.Type == object.TypeNumber {
		num := key.Value.Num
		intVal := int64(num)
		
		// Check if the number is actually an integer
		if float64(intVal) == num && intVal >= 1 {
			idx := int(intVal)
			
			// Extend array if necessary
			if idx > len(t.Array) {
				// Grow array to accommodate the new index
				newArray := make([]object.TValue, idx)
				copy(newArray, t.Array)
				t.Array = newArray
			}
			
			// Set value in array (1-indexed, so subtract 1)
			t.Array[idx-1] = value
			t.IsArray = len(t.Map) == 0
			return
		}
	}
	
	// Map access for all other cases
	vk := newValueKey(key)
	
	// Create a copy of the value to store
	valCopy := value
	t.Map[vk] = &valCopy
	t.IsArray = false
}

// SetI sets a value in the table at the given integer index.
//
// This is an optimized version of Set for integer keys.
// It directly accesses the array part without type checking.
//
// Parameters:
//   - idx: The 1-based integer index
//   - value: The value to set
func (t *Table) SetI(idx int, value object.TValue) {
	if idx < 1 {
		// Invalid index, store in map instead
		key := object.NewInteger(int64(idx))
		t.Set(*key, value)
		return
	}
	
	// Extend array if necessary
	if idx > len(t.Array) {
		newArray := make([]object.TValue, idx)
		copy(newArray, t.Array)
		t.Array = newArray
	}
	
	t.Array[idx-1] = value
	t.IsArray = len(t.Map) == 0
}

// Len returns the length of the table (# operator semantics).
//
// This method implements Lua's length operator (#) for tables:
//   - For array-like tables, returns the largest n such that
//     t[n] is not nil and t[n+1] is nil (or n is the last index)
//   - For tables with holes (nil values in the array), finds the
//     boundary between present and absent values
//
// Returns:
//   - int: The length of the array part
//
// Note: This implementation assumes the array part has no holes.
// For tables with holes, a more sophisticated algorithm would be
// needed (binary search for the boundary).
func (t *Table) Len() int {
	// Find the actual length by checking for nil values
	// Lua's # operator finds the boundary where array elements stop
	length := 0
	for i := len(t.Array); i > 0; i-- {
		if t.Array[i-1].Type != object.TypeNil {
			length = i
			break
		}
	}
	t.Length = length
	return length
}

// Next returns the next key-value pair for iteration.
//
// This method implements Lua's next() function for table iteration:
//   - When key is nil, returns the first key-value pair
//   - Otherwise, returns the next key-value pair after the given key
//   - Returns (nil, nil) when there are no more pairs
//
// The iteration order is:
//   1. Array part (integer keys 1, 2, 3, ...)
//   2. Map part (arbitrary order, not guaranteed)
//
// Parameters:
//   - key: The current key (nil to start iteration)
//
// Returns:
//   - *object.TValue: The next key, or nil if iteration is complete
//   - *object.TValue: The next value, or nil if iteration is complete
//
// Example (implementing pairs):
//
//	for k, v := t.Next(nil); k != nil; k, v = t.Next(k) {
//	    // Process key-value pair
//	}
//
// Example (implementing ipairs):
//
//	for i := 1; i <= t.Len(); i++ {
//	    v := t.GetI(i)
//	    if v != nil && v.Type != object.TypeNil {
//	        // Process array element
//	    }
//	}
func (t *Table) Next(key *object.TValue) (*object.TValue, *object.TValue) {
	// Collect all map keys in a deterministic order
	mapKeys := make([]valueKey, 0, len(t.Map))
	for vk := range t.Map {
		mapKeys = append(mapKeys, vk)
	}
	// Sort map keys for deterministic iteration
	sort.Slice(mapKeys, func(i, j int) bool {
		if mapKeys[i].Type != mapKeys[j].Type {
			return mapKeys[i].Type < mapKeys[j].Type
		}
		if mapKeys[i].Str != mapKeys[j].Str {
			return mapKeys[i].Str < mapKeys[j].Str
		}
		if mapKeys[i].Num != mapKeys[j].Num {
			return mapKeys[i].Num < mapKeys[j].Num
		}
		return mapKeys[i].Ptr < mapKeys[j].Ptr
	})

	// If key is nil, start from the beginning
	if key == nil || key.Type == object.TypeNil {
		// Return first array element if array is not empty
		if len(t.Array) > 0 {
			k := object.NewInteger(1)
			return k, &t.Array[0]
		}

		// Return first map element if map is not empty
		if len(mapKeys) > 0 {
			k := keyFromValueKey(mapKeys[0])
			return k, t.Map[mapKeys[0]]
		}

		// Table is empty
		return nil, nil
	}

	// Continue iteration from the given key

	// Check if we're in the array part
	if key.Type == object.TypeNumber {
		num := key.Value.Num
		intVal := int64(num)

		if float64(intVal) == num && intVal >= 1 {
			idx := int(intVal)

			// Try next array element
			if idx+1 <= len(t.Array) {
				k := object.NewInteger(int64(idx + 1))
				return k, &t.Array[idx]
			}

			// Array part exhausted, move to map part
			// Return first map element
			if len(mapKeys) > 0 {
				k := keyFromValueKey(mapKeys[0])
				return k, t.Map[mapKeys[0]]
			}

			// No more elements
			return nil, nil
		}
	}

	// We're in the map part or key is not an array index
	// Find the current key in the sorted map keys
	currentVK := newValueKey(*key)
	foundIndex := -1

	for i, vk := range mapKeys {
		if vk == currentVK {
			foundIndex = i
			break
		}
	}

	// If current key not found, it might have been removed
	// Just return the first element
	if foundIndex == -1 {
		if len(mapKeys) > 0 {
			k := keyFromValueKey(mapKeys[0])
			return k, t.Map[mapKeys[0]]
		}
		return nil, nil
	}

	// Return the next element
	if foundIndex+1 < len(mapKeys) {
		nextVK := mapKeys[foundIndex+1]
		k := keyFromValueKey(nextVK)
		return k, t.Map[nextVK]
	}

	// No more elements
	return nil, nil
}


// keyFromValueKey creates a TValue from a valueKey.
//
// This is the inverse of newValueKey, reconstructing a TValue
// from its key representation.
func keyFromValueKey(vk valueKey) *object.TValue {
	switch vk.Type {
	case object.TypeNil:
		return object.NewNil()
	case object.TypeBoolean:
		return object.NewBoolean(vk.Num != 0)
	case object.TypeNumber:
		return object.NewNumber(vk.Num)
	case object.TypeString:
		return object.NewString(vk.Str)
	case object.TypeLightUserData:
		// Simplified: return nil for light userdata
		return object.NewNil()
	default:
		// For GC objects, we can't reconstruct the original
		// This is a limitation of the current implementation
		return object.NewNil()
	}
}

// GetMetatable returns the metatable of the table.
//
// Metatables allow customizing table behavior through metamethods
// such as __index, __newindex, __add, etc.
//
// Returns:
//   - *Table: The metatable, or nil if no metatable is set
func (t *Table) GetMetatable() *Table {
	return t.Metatable
}

// SetMetatable sets the metatable of the table.
//
// Parameters:
//   - mt: The metatable to set (can be nil to remove metatable)
//
// Note: In Lua, setting a metatable may be restricted if the table
// already has a metatable with a __metatable field. This method
// does not enforce such restrictions; the VM should handle that.
func (t *Table) SetMetatable(mt *Table) {
	t.Metatable = mt
}

// gcObject marks Table as a GC object.
//
// This method implements the object.GCObject interface,
// allowing Table to be tracked by the garbage collector.
func (t *Table) gcObject() {}

// IsEmpty returns true if the table has no elements.
//
// Returns:
//   - bool: True if both array and map parts are empty
func (t *Table) IsEmpty() bool {
	return len(t.Array) == 0 && len(t.Map) == 0
}

// Clear removes all elements from the table.
//
// This method resets both the array and map parts,
// but preserves the metatable.
func (t *Table) Clear() {
	t.Array = make([]object.TValue, 0, cap(t.Array))
	t.Map = make(map[valueKey]*object.TValue)
	t.Length = 0
	t.IsArray = true
}