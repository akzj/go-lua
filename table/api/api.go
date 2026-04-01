// Package api defines the table interface for Lua VM.
// No implementation details - only interfaces and constructors.
//
// Constraint: must NOT import internal/ to avoid circular dependency.
// Use NewTable() factory which returns the pre-initialized default.
package api

import (
	memapi "github.com/akzj/go-lua/mem/api"
	"github.com/akzj/go-lua/types/api"
)

// =============================================================================
// Table Interface
// =============================================================================

// TableInterface manages Lua tables (array part + hash part).
//
// Invariants:
// - Array indices are 1-based (Lua semantics)
// - Hash part uses Brent's variation of chained scatter table
// - Keys in array part are positive integers [1, asize]
// - Negative integer keys and non-integer keys go to hash part
//
// Why separate array and hash?
// - Array access is O(1), hash access is O(1) average
// - Lua tables optimize for sequential integer keys (common pattern)
type TableInterface interface {
	// Get retrieves a value by key.
	// Returns nil TValue if key is absent.
	// For integer keys in array range, uses fast path.
	Get(key api.TValue) api.TValue

	// Set assigns a value to key.
	// If value is nil, deletes the key (like Lua's t[k] = nil).
	// May trigger rehash if table grows.
	Set(key, value api.TValue)

	// Len returns the length of the array part (#t in Lua).
	// Uses hint from previous operation if available.
	// Returns 0 if table is empty.
	Len() int

	// GetInt retrieves value by integer key.
	// Fast path for t[n] where n is integer.
	// Returns nil TValue if absent.
	GetInt(key api.LuaInteger) api.TValue

	// SetInt assigns value by integer key.
	// Fast path for t[n] = v where n is integer.
	// May trigger rehash if key is out of bounds.
	SetInt(key api.LuaInteger, value api.TValue)

	// GetMetatable returns the table's metatable, or nil.
	GetMetatable() api.Table

	// SetMetatable sets the table's metatable.
	// Previous metatable, if any, is replaced.
	SetMetatable(t api.Table)

	// Next returns the next key-value pair after 'key'.
	// For first iteration, pass nil key.
	// Returns (nextKey, nextValue, true) or (nil, nil, false) if exhausted.
	// Used to implement Lua's next() function.
	Next(key api.TValue) (api.TValue, api.TValue, bool)

	// Resize changes the array part size to nasize.
	// Hash part is resized proportionally.
	// Used during rehash operations.
	Resize(nasize int)
}

// =============================================================================
// Factory Function (avoids circular dependency)
// =============================================================================

// DefaultTable is the default table instance.
// Initialized by internal.init() before any user code runs.
var DefaultTable TableInterface

// NewTable returns a new TableInterface instance.
// This factory pattern avoids importing internal/ from api/.
func NewTable(alloc memapi.Allocator) TableInterface {
	return DefaultTable
}
