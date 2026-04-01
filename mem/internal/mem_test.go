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

func TestAllocSize1(t *testing.T) {
	alloc := api.NewAllocator(nil)
	ptr := alloc.Alloc(1)
	if ptr == nil {
		t.Errorf("Alloc(1) should return non-nil")
	}
	alloc.Free(ptr, 1)
}

func TestAllocLarge(t *testing.T) {
	alloc := api.NewAllocator(nil)
	ptr := alloc.Alloc(1024 * 1024) // 1MB
	if ptr == nil {
		t.Errorf("Alloc(1MB) should return non-nil")
	}
	alloc.Free(ptr, 1024*1024)
}

func TestReallocSameSize(t *testing.T) {
	alloc := api.NewAllocator(nil)
	ptr := alloc.Alloc(100)
	if ptr == nil {
		t.Fatalf("Alloc(100) should return non-nil")
	}
	result := alloc.Realloc(ptr, 100, 100)
	if result == nil {
		t.Errorf("Realloc(ptr, 100, 100) should return non-nil")
	}
	alloc.Free(result, 100)
}

func TestReallocLarger(t *testing.T) {
	alloc := api.NewAllocator(nil)
	ptr := alloc.Alloc(50)
	if ptr == nil {
		t.Fatalf("Alloc(50) should return non-nil")
	}
	// Write data to original allocation
	data := unsafe.Slice((*byte)(ptr), 50)
	for i := range data {
		data[i] = byte(i % 256)
	}
	result := alloc.Realloc(ptr, 50, 100)
	if result == nil {
		t.Fatalf("Realloc(ptr, 50, 100) should return non-nil")
	}
	// Verify original data is preserved
	resultData := unsafe.Slice((*byte)(result), 50)
	for i := range resultData {
		if resultData[i] != byte(i%256) {
			t.Errorf("Data at index %d: expected %d, got %d", i, byte(i%256), resultData[i])
		}
	}
	alloc.Free(result, 100)
}

func TestReallocSmaller(t *testing.T) {
	alloc := api.NewAllocator(nil)
	ptr := alloc.Alloc(100)
	if ptr == nil {
		t.Fatalf("Alloc(100) should return non-nil")
	}
	// Write data to original allocation
	data := unsafe.Slice((*byte)(ptr), 100)
	for i := range data {
		data[i] = byte(i % 256)
	}
	result := alloc.Realloc(ptr, 100, 50)
	if result == nil {
		t.Fatalf("Realloc(ptr, 100, 50) should return non-nil")
	}
	// Verify first 50 bytes are preserved
	resultData := unsafe.Slice((*byte)(result), 50)
	for i := range resultData {
		if resultData[i] != byte(i%256) {
			t.Errorf("Data at index %d: expected %d, got %d", i, byte(i%256), resultData[i])
		}
	}
	alloc.Free(result, 50)
}

func TestCopyBytesZero(t *testing.T) {
	alloc := api.NewAllocator(nil)
	src := alloc.Alloc(100)
	dst := alloc.Alloc(100)
	if src == nil || dst == nil {
		t.Fatalf("Alloc should return non-nil")
	}
	// copyBytes with n=0 should not panic and do nothing
	copyBytes(dst, src, 0)
	alloc.Free(src, 100)
	alloc.Free(dst, 100)
}

func TestCopyBytesNormal(t *testing.T) {
	alloc := api.NewAllocator(nil)
	src := alloc.Alloc(100)
	dst := alloc.Alloc(100)
	if src == nil || dst == nil {
		t.Fatalf("Alloc should return non-nil")
	}
	// Write data to source
	srcData := unsafe.Slice((*byte)(src), 100)
	for i := range srcData {
		srcData[i] = byte((i + 42) % 256)
	}
	// Copy using copyBytes
	copyBytes(dst, src, 100)
	// Verify destination has copied data
	dstData := unsafe.Slice((*byte)(dst), 100)
	for i := range dstData {
		if dstData[i] != byte((i+42)%256) {
			t.Errorf("Copied data at index %d: expected %d, got %d", i, byte((i+42)%256), dstData[i])
		}
	}
	alloc.Free(src, 100)
	alloc.Free(dst, 100)
}

func TestSafeReallocSuccess(t *testing.T) {
	oomCalled := 0
	alloc := NewAllocatorWithConfig(&api.AllocatorConfig{
		OnOutOfMemory: func() {
			oomCalled++
		},
	})
	// Normal realloc should succeed without calling OOM
	result := alloc.SafeRealloc(nil, 0, 100)
	if result == nil {
		t.Errorf("SafeRealloc should return non-nil on success")
	}
	if oomCalled != 0 {
		t.Errorf("OnOutOfMemory should not be called on success")
	}
	alloc.Free(result, 100)
}

func TestFreeNilPtr(t *testing.T) {
	alloc := api.NewAllocator(nil)
	// Free with nil pointer should not panic
	alloc.Free(nil, 100)
}

func TestFreeZeroSize(t *testing.T) {
	alloc := api.NewAllocator(nil)
	ptr := alloc.Alloc(100)
	// Free with zero size should not panic
	alloc.Free(ptr, 0)
}
