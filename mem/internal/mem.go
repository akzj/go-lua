// Package internal implements the memory allocator.
// Implementation details hidden from external packages.
package internal

import (
	"unsafe"

	"github.com/akzj/go-lua/mem/api"
)

// allocator implements the Allocator interface using Go's built-in allocator.
type allocator struct {
	onOOM func()
	gc    gcCollector
}

// gcCollector interface for GC triggering (matches GCCollector in gc/api)
type gcCollector interface {
	AllocateBytes(bytes uint64)
	BytesInUse() uint64
	BytesThreshold() uint64
	Step() bool
}

// NewAllocator creates a new Allocator with default settings.
func NewAllocator() api.Allocator {
	return newAllocator(nil)
}

// NewAllocatorWithConfig creates a new Allocator with custom configuration.
func NewAllocatorWithConfig(config *api.AllocatorConfig) api.Allocator {
	return newAllocator(config)
}

func newAllocator(config *api.AllocatorConfig) api.Allocator {
	if config == nil {
		config = &api.AllocatorConfig{}
	}
	onOOM := config.OnOutOfMemory
	if onOOM == nil {
		onOOM = func() {
			panic("memory allocation error: block too big")
		}
	}
	return &allocator{onOOM: onOOM, gc: config.GCCollector}
}

func (a *allocator) Alloc(size api.LuaMem) unsafe.Pointer {
	if size == 0 {
		return nil
	}
	// Use make to allocate a byte slice, then get pointer to first element.
	slice := make([]byte, size)

	// Trigger GC check if collector is configured
	if a.gc != nil {
		a.gc.AllocateBytes(uint64(size))
		// If memory usage exceeds threshold, perform incremental GC step
		if a.gc.BytesInUse() >= a.gc.BytesThreshold() {
			a.gc.Step()
		}
	}

	return unsafe.Pointer(&slice[0])
}

func (a *allocator) Free(ptr unsafe.Pointer, size api.LuaMem) {
	if ptr == nil || size == 0 {
		return
	}
	// Update GC accounting
	if a.gc != nil {
		a.gc.AllocateBytes(^uint64(size) + 1) // equivalent to -int64(size)
	}
	// Go's GC handles deallocation; we just need to make the pointer unreachable.
}

func (a *allocator) Realloc(ptr unsafe.Pointer, oldSize, newSize api.LuaMem) unsafe.Pointer {
	if newSize == 0 {
		a.Free(ptr, oldSize)
		return nil
	}
	if ptr == nil {
		return a.Alloc(newSize)
	}

	// Allocate new block
	slice := make([]byte, newSize)
	newPtr := unsafe.Pointer(&slice[0])

	// Copy existing data
	if oldSize > 0 && newSize > 0 {
		copyBytes(newPtr, ptr, min(oldSize, newSize))
	}

	// Update GC accounting: net change = newSize - oldSize
	if a.gc != nil {
		if newSize >= oldSize {
			a.gc.AllocateBytes(uint64(newSize - oldSize))
		} else {
			a.gc.AllocateBytes(^uint64(oldSize-newSize) + 1) // subtract
		}
		// Check threshold after realloc
		if a.gc.BytesInUse() >= a.gc.BytesThreshold() {
			a.gc.Step()
		}
	}

	return newPtr
}

func (a *allocator) SafeRealloc(ptr unsafe.Pointer, oldSize, newSize api.LuaMem) unsafe.Pointer {
	result := a.Realloc(ptr, oldSize, newSize)
	if result == nil && newSize > 0 {
		a.onOOM()
	}
	return result
}

func copyBytes(dst, src unsafe.Pointer, n api.LuaMem) {
	if n <= 0 {
		return
	}
	srcSlice := unsafe.Slice((*byte)(src), n)
	dstSlice := unsafe.Slice((*byte)(dst), n)
	copy(dstSlice, srcSlice)
}

func min(a, b api.LuaMem) api.LuaMem {
	if a < b {
		return a
	}
	return b
}

// verify allocator implements Allocator at compile time.
var _ api.Allocator = (*allocator)(nil)
