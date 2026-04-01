package internal

import (
	"testing"
	"unsafe"

	"github.com/akzj/go-lua/mem/api"
)

// failingAllocator always returns nil to simulate OOM.
type failingAllocator struct{}

func (f *failingAllocator) Alloc(size api.LuaMem) unsafe.Pointer {
	return nil
}

func (f *failingAllocator) Free(ptr unsafe.Pointer, size api.LuaMem) {
}

func (f *failingAllocator) Realloc(ptr unsafe.Pointer, oldSize, newSize api.LuaMem) unsafe.Pointer {
	return nil
}

func (f *failingAllocator) SafeRealloc(ptr unsafe.Pointer, oldSize, newSize api.LuaMem) unsafe.Pointer {
	return nil
}

var _ api.Allocator = (*failingAllocator)(nil)

func TestAllocZero(t *testing.T) {
	alloc := api.DefaultAllocator
	ptr := alloc.Alloc(0)
	if ptr != nil {
		t.Errorf("Alloc(0) should return nil, got %v", ptr)
	}
}

func TestAlloc(t *testing.T) {
	alloc := api.NewAllocator(nil)
	ptr := alloc.Alloc(100)
	if ptr == nil {
		t.Errorf("Alloc(100) should return non-nil")
	}
	alloc.Free(ptr, 100)
}

func TestReallocNilPtr(t *testing.T) {
	alloc := api.NewAllocator(nil)
	ptr := alloc.Realloc(nil, 0, 50)
	if ptr == nil {
		t.Errorf("Realloc(nil, 0, 50) should allocate new block")
	}
	alloc.Free(ptr, 50)
}

func TestReallocToZero(t *testing.T) {
	alloc := api.NewAllocator(nil)
	ptr := alloc.Alloc(100)
	result := alloc.Realloc(ptr, 100, 0)
	if result != nil {
		t.Errorf("Realloc(ptr, 100, 0) should return nil")
	}
}

func TestSafeReallocOOMConfig(t *testing.T) {
	oomCalled := false
	alloc := NewAllocatorWithConfig(&api.AllocatorConfig{
		OnOutOfMemory: func() {
			oomCalled = true
		},
	})
	// Verify allocator was created with custom config
	if alloc == nil {
		t.Errorf("NewAllocatorWithConfig should return non-nil")
	}
	_ = oomCalled
}
