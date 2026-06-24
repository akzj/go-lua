//go:build !linux

package mem

import "unsafe"

// AllocSlab allocates n elements of type T via make() on non-Linux platforms.
// This is a safe fallback — elements will be visible to Go's GC, which means
// slightly higher GC overhead, but correctness is preserved.
func AllocSlab[T any](n int) []T {
	return make([]T, n)
}

// FreeSlab is a no-op on non-Linux platforms — Go's GC handles the memory.
func FreeSlab[T any](slab []T) {}
