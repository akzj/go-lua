// Package api defines the memory allocator interface for Lua VM.
// No implementation details - only interfaces and constructors.
//
// Constraint: must import from types module, not define locally.
package api

import (
	"unsafe"

	"github.com/akzj/go-lua/types/api"
)

// Re-export LuaMem and LuaUMem from types for convenience.
// Consumers of this package can use api.LuaMem instead of types/api.LuaMem.
type LuaMem = api.LuaMem
type LuaUMem = api.LuaUMem

// Allocator defines the memory allocation interface for Lua VM.
//
// Design rationale:
// - Go's allocator uses interface + function pointer (like lua_State.frealloc)
// - This interface mirrors luaM_malloc_/luaM_free_/luaM_realloc_
// - SafeRealloc panics on OOM (Lua semantics, unlike Go which returns nil)
//
// Invariants:
// - Alloc(0) returns nil
// - Free(nil, 0) is a no-op (matches ISO C free(NULL))
// - Realloc(nil, 0, n) is equivalent to Alloc(n)
type Allocator interface {
	// Alloc allocates n bytes of memory.
	// Returns nil if size == 0.
	// Post: result != nil || size == 0
	Alloc(size LuaMem) unsafe.Pointer

	// Free releases memory pointed by ptr.
	// Pre: ptr != nil && size > 0 (same invariant as luaM_free_)
	// Why not accept nil? luaM_free_ asserts (osize == 0) == (block == NULL)
	Free(ptr unsafe.Pointer, size LuaMem)

	// Realloc reallocates memory from oldSize to newSize.
	// If ptr is nil, behaves like Alloc(newSize).
	// If newSize is 0, behaves like Free and returns nil.
	// Post: result != nil || newSize == 0
	Realloc(ptr unsafe.Pointer, oldSize, newSize LuaMem) unsafe.Pointer

	// SafeRealloc is like Realloc but panics on OOM instead of returning nil.
	// Used by luaM_saferealloc_ pattern.
	// Why not combine with Realloc? Lua semantics distinguish recoverable
	// (try GC then retry) vs unrecoverable (panic) errors.
	SafeRealloc(ptr unsafe.Pointer, oldSize, newSize LuaMem) unsafe.Pointer
}

// AllocatorConfig holds configuration for creating an Allocator.
type AllocatorConfig struct {
	// OnOutOfMemory is called when allocation fails and cannot be recovered.
	// If nil, defaults to panic with "memory allocation error".
	OnOutOfMemory func()

	// GCCollector is an optional garbage collector to trigger on allocations.
	// When set, the allocator will call GC step checks after each allocation.
	// This enables automatic GC triggering for long-running services.
	// Must implement: AllocateBytes, BytesInUse, BytesThreshold, Step
	GCCollector interface {
		AllocateBytes(bytes uint64)
		BytesInUse() uint64
		BytesThreshold() uint64
		Step() bool
		Start()
	}
}

// NewAllocator creates an Allocator with the given configuration.
// If config is nil, uses the default system allocator (DefaultAllocator).
// The DefaultAllocator is initialized by internal.init() before any user code runs.
func NewAllocator(config *AllocatorConfig) Allocator {
	// DefaultAllocator is set by internal.init() before user code runs.
	return DefaultAllocator
}

// DefaultAllocator is the system default allocator.
// Initialized by internal.init() before any user code runs.
var DefaultAllocator Allocator
