// Package mem provides self-managed memory allocation backed by mmap,
// bypassing Go's garbage collector for Lua-internal objects.
//
// SlabArena allocates large, fixed-size arrays that appear as a single GC root
// rather than thousands of individually-scanned objects. On Linux, this uses
// mmap anonymous memory; on other platforms it falls back to make().
package mem

import (
	"syscall"
	"unsafe"
)

// AllocSlab allocates n elements of type T using mmap anonymous memory.
// The returned slice is not visible to Go's GC — individual elements will not
// be scanned. This is safe because Lua manages its own object lifecycle via
// its own GC.
//
// On Linux, uses MAP_ANONYMOUS|MAP_PRIVATE with PROT_READ|PROT_WRITE.
// Returns an empty (non-nil) slice for n <= 0. Returns nil if mmap fails.
func AllocSlab[T any](n int) []T {
	if n <= 0 {
		return []T{}
	}
	sz := n * int(unsafe.Sizeof(*(*T)(nil)))
	data, err := syscall.Mmap(-1, 0, sz,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANONYMOUS|syscall.MAP_PRIVATE)
	if err != nil {
		return nil
	}
	// Convert raw byte slice to typed slice.
	ptr := (*T)(unsafe.Pointer(unsafe.SliceData(data)))
	return unsafe.Slice(ptr, n)[:n:n]
}

// FreeSlab unmaps a slab previously allocated by AllocSlab.
// The slice MUST have been returned by AllocSlab — freeing other memory is
// undefined behavior.
func FreeSlab[T any](slab []T) {
	if len(slab) == 0 {
		return
	}
	sz := cap(slab) * int(unsafe.Sizeof(*(*T)(nil)))
	ptr := unsafe.Pointer(unsafe.SliceData(slab))
	// Reinterpret as byte slice for munmap.
	b := unsafe.Slice((*byte)(ptr), sz)
	_ = syscall.Munmap(b)
}
