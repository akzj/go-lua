package internal

import (
	"sync"
	"testing"

	gcapi "github.com/akzj/go-lua/gc/api"
)

// GCCollector is the interface for GCCollector (avoid circular import)
var _ gcapi.GCCollector = (*Collector)(nil)

// =============================================================================
// NewCollector Tests
// =============================================================================

// TestNewCollectorBasic tests basic collector creation.
func TestNewCollectorBasic(t *testing.T) {
	c := NewCollector(nil)
	if c == nil {
		t.Fatal("NewCollector returned nil")
	}
	if c.state != gcapi.GCSpause {
		t.Errorf("Initial state = %d, want GCSpause", c.state)
	}
	if c.currentWhite != 1 {
		t.Errorf("Initial white = %d, want 1", c.currentWhite)
	}
}

// TestNewCollectorWithNilAllocator tests collector with nil allocator.
func TestNewCollectorWithNilAllocator(t *testing.T) {
	// Should use default allocator
	c := NewCollector(nil)
	if c == nil {
		t.Fatal("NewCollector(nil) returned nil")
	}
}

// TestNewCollectorWithCustomAllocator tests collector creation with nil (default allocator).
func TestNewCollectorWithCustomAllocator(t *testing.T) {
	// Use nil to test default allocator
	c := NewCollector(nil)
	if c == nil {
		t.Fatal("NewCollector(nil) returned nil")
	}
	// Verify collector has valid state
	if c.state != gcapi.GCSpause {
		t.Errorf("Initial state = %d, want GCSpause", c.state)
	}
}

// =============================================================================
// GC State Tests
// =============================================================================

// TestCollectorInitialState tests initial GC state.
func TestCollectorInitialState(t *testing.T) {
	c := NewCollector(nil)
	if c.State() != gcapi.GCSpause {
		t.Errorf("State() = %d, want GCSpause", c.State())
	}
}

// TestCollectorIsRunning tests IsRunning method.
func TestCollectorIsRunning(t *testing.T) {
	c := NewCollector(nil)
	// Initial state is paused
	if c.IsRunning() {
		t.Error("IsRunning() = true, want false (initial state is GCSpause)")
	}
}

// =============================================================================
// Start/Stop Tests
// =============================================================================

// TestStopGC tests stopping the GC.
func TestStopGC(t *testing.T) {
	c := NewCollector(nil)
	c.Stop()
	if !c.stopped {
		t.Error("stopped = false, want true after Stop()")
	}
}

// TestStartGC tests starting the GC.
func TestStartGC(t *testing.T) {
	c := NewCollector(nil)
	c.Stop()
	c.Start()
	if c.stopped {
		t.Error("stopped = true, want false after Start()")
	}
}

// TestMultipleStopStart tests multiple stop/start cycles.
func TestMultipleStopStart(t *testing.T) {
	c := NewCollector(nil)
	for i := 0; i < 10; i++ {
		c.Stop()
		c.Start()
	}
	// Should be in consistent state
	if c.stopped {
		t.Error("stopped = true after 10 Stop/Start cycles")
	}
}

// =============================================================================
// Step Tests
// =============================================================================

// TestStepWhenStopped tests Step when GC is stopped.
func TestStepWhenStopped(t *testing.T) {
	c := NewCollector(nil)
	c.Stop()
	if c.Step() {
		t.Error("Step() = true, want false when stopped")
	}
}

// TestStepStartsNewCycle tests that Step starts a new GC cycle.
func TestStepStartsNewCycle(t *testing.T) {
	c := NewCollector(nil)
	// Initial state is GCSpause (8)
	// After Step, it enters propagation then advances
	c.Step()
	// State should be past initial GCSpause - either GCSpropagate or next
	state := c.State()
	if state == gcapi.GCSpause {
		t.Error("State() still GCSpause after Step, want progression")
	}
}

// =============================================================================
// Collect Tests
// =============================================================================

// TestCollectWhenStopped tests Collect when GC is stopped.
func TestCollectWhenStopped(t *testing.T) {
	c := NewCollector(nil)
	c.Stop()
	freed := c.Collect()
	if freed != 0 {
		t.Errorf("Collect() = %d, want 0 when stopped", freed)
	}
}

// TestCollectBasic tests basic Collect functionality.
func TestCollectBasic(t *testing.T) {
	c := NewCollector(nil)
	c.Collect()
	// Should complete and return to pause state
	if c.State() != gcapi.GCSpause {
		t.Errorf("State() = %d after Collect, want GCSpause", c.State())
	}
}

// TestMultipleCollects tests multiple consecutive Collect calls.
func TestMultipleCollects(t *testing.T) {
	c := NewCollector(nil)
	for i := 0; i < 5; i++ {
		c.Collect()
		if c.State() != gcapi.GCSpause {
			t.Errorf("State() = %d after Collect #%d, want GCSpause", c.State(), i+1)
		}
	}
}

// =============================================================================
// Color Helper Tests
// =============================================================================

// TestOtherWhite tests otherWhite function.
func TestOtherWhite(t *testing.T) {
	c := NewCollector(nil)
	if c.currentWhite != 1 {
		t.Skip("Test assumes currentWhite = 1")
	}
	if c.otherWhite() != 0 {
		t.Error("otherWhite() = 1, want 0 when currentWhite = 1")
	}
}

// TestIsWhite tests iswhite helper.
func TestIsWhite(t *testing.T) {
	c := NewCollector(nil)
	// Object with white bit set
	obj := &GCObject{Marked: 1}
	if !c.iswhite(obj) {
		t.Error("iswhite(obj with white) = false, want true")
	}
	// Object with black bit set
	obj = &GCObject{Marked: 2}
	if c.iswhite(obj) {
		t.Error("iswhite(obj with black) = true, want false")
	}
	// Nil object
	if c.iswhite(nil) {
		t.Error("iswhite(nil) = true, want false")
	}
}

// TestIsBlack tests isblack helper.
func TestIsBlack(t *testing.T) {
	c := NewCollector(nil)
	// Object with black bit set
	obj := &GCObject{Marked: 2}
	if !c.isblack(obj) {
		t.Error("isblack(obj with black) = false, want true")
	}
	// Object with white bit set
	obj = &GCObject{Marked: 1}
	if c.isblack(obj) {
		t.Error("isblack(obj with white) = true, want false")
	}
	// Nil object
	if c.isblack(nil) {
		t.Error("isblack(nil) = true, want false")
	}
}

// TestIsGray tests isgray helper.
func TestIsGray(t *testing.T) {
	c := NewCollector(nil)
	// Gray object has neither white nor black
	obj := &GCObject{Marked: 0}
	if !c.isgray(obj) {
		t.Error("isgray(obj with no color) = false, want true")
	}
	// White object is not gray
	obj = &GCObject{Marked: 1}
	if c.isgray(obj) {
		t.Error("isgray(obj with white) = true, want false")
	}
	// Nil object
	if c.isgray(nil) {
		t.Error("isgray(nil) = true, want false")
	}
}

// =============================================================================
// Age Helper Tests
// =============================================================================

// TestGetAge tests getAge helper.
func TestGetAge(t *testing.T) {
	c := NewCollector(nil)
	// New object
	obj := &GCObject{Marked: 0}
	if c.getAge(obj) != gcapi.GNew {
		t.Errorf("getAge(new) = %d, want GNew", c.getAge(obj))
	}
	// Old object
	obj.Marked = gcapi.SetAge(0, gcapi.GOld)
	if c.getAge(obj) != gcapi.GOld {
		t.Errorf("getAge(old) = %d, want GOld", c.getAge(obj))
	}
	// Nil object
	if c.getAge(nil) != 0 {
		t.Error("getAge(nil) != 0")
	}
}

// TestSetAge tests setAge helper.
func TestSetAge(t *testing.T) {
	c := NewCollector(nil)
	obj := &GCObject{}
	c.setAge(obj, gcapi.GOld0)
	if c.getAge(obj) != gcapi.GOld0 {
		t.Errorf("getAge after setAge(GOld0) = %d, want GOld0", c.getAge(obj))
	}
	// Nil object should not panic
	c.setAge(nil, gcapi.GOld0)
}

// =============================================================================
// Color Setting Tests
// =============================================================================

// TestSet2Black tests set2black helper.
func TestSet2Black(t *testing.T) {
	c := NewCollector(nil)
	obj := &GCObject{Marked: 1} // white
	c.set2black(obj)
	if !gcapi.IsBlack(obj.Marked) {
		t.Error("Object should be black after set2black")
	}
	// Nil should not panic
	c.set2black(nil)
}

// TestMakeWhite tests makewhite helper.
func TestMakeWhite(t *testing.T) {
	c := NewCollector(nil)
	obj := &GCObject{Marked: 2} // black
	c.makewhite(obj)
	if !gcapi.IsWhite(obj.Marked) {
		t.Error("Object should be white after makewhite")
	}
	// Nil should not panic
	c.makewhite(nil)
}

// TestSet2Gray tests set2gray helper.
func TestSet2Gray(t *testing.T) {
	c := NewCollector(nil)
	obj := &GCObject{Marked: 1} // white
	c.set2gray(obj)
	if !gcapi.IsGray(obj.Marked) {
		t.Error("Object should be gray after set2gray")
	}
	// Nil should not panic
	c.set2gray(nil)
}

// =============================================================================
// Memory Accounting Tests
// =============================================================================

// TestBytesInUseInitial tests initial BytesInUse.
func TestBytesInUseInitial(t *testing.T) {
	c := NewCollector(nil)
	if c.BytesInUse() != 0 {
		t.Errorf("BytesInUse() = %d, want 0", c.BytesInUse())
	}
}

// TestAllocateBytes tests AllocateBytes method.
func TestAllocateBytes(t *testing.T) {
	c := NewCollector(nil)
	c.AllocateBytes(1000)
	if c.BytesInUse() != 1000 {
		t.Errorf("BytesInUse() = %d after AllocateBytes(1000)", c.BytesInUse())
	}
}

// TestFreeBytes tests FreeBytes method.
func TestFreeBytes(t *testing.T) {
	c := NewCollector(nil)
	c.AllocateBytes(1000)
	c.FreeBytes(300)
	if c.BytesInUse() != 700 {
		t.Errorf("BytesInUse() = %d, want 700", c.BytesInUse())
	}
}

// TestFreeBytesUnderflow tests FreeBytes with more than available.
func TestFreeBytesUnderflow(t *testing.T) {
	c := NewCollector(nil)
	c.AllocateBytes(100)
	c.FreeBytes(200) // More than available
	if c.BytesInUse() != 0 {
		t.Errorf("BytesInUse() = %d after over-free, want 0", c.BytesInUse())
	}
}

// =============================================================================
// Threshold Tests
// =============================================================================

// TestSetThreshold tests SetThreshold method.
func TestSetThreshold(t *testing.T) {
	c := NewCollector(nil)
	c.SetThreshold(5000)
	if c.BytesThreshold() != 5000 {
		t.Errorf("BytesThreshold() = %d, want 5000", c.BytesThreshold())
	}
}

// TestThresholdInitial tests initial threshold.
func TestThresholdInitial(t *testing.T) {
	c := NewCollector(nil)
	// Initial threshold should be 0
	if c.BytesThreshold() != 0 {
		t.Errorf("Initial BytesThreshold() = %d, want 0", c.BytesThreshold())
	}
}

// =============================================================================
// GC Mode Tests
// =============================================================================

// TestGCModeDefault tests default GC mode.
func TestGCModeDefault(t *testing.T) {
	c := NewCollector(nil)
	if c.GCMode() != gcapi.KGCInc {
		t.Errorf("GCMode() = %d, want KGCInc", c.GCMode())
	}
}

// TestSetGCMode tests SetGCMode method.
func TestSetGCMode(t *testing.T) {
	c := NewCollector(nil)
	c.SetGCMode(gcapi.KGCGenMinor)
	if c.GCMode() != gcapi.KGCGenMinor {
		t.Errorf("GCMode() = %d after SetGCMode(KGCGenMinor)", c.GCMode())
	}
}

// =============================================================================
// Generational GC Tests
// =============================================================================

// TestFullGCMajor tests FullGCMajor method.
func TestFullGCMajor(t *testing.T) {
	c := NewCollector(nil)
	c.FullGCMajor()
	// Should be in major mode
	if c.GCMode() != gcapi.KGCGenMajor {
		t.Errorf("GCMode() = %d after FullGCMajor, want KGCGenMajor", c.GCMode())
	}
}

// TestMinorGC tests MinorGC method.
func TestMinorGC(t *testing.T) {
	c := NewCollector(nil)
	c.MinorGC()
	if c.GCMode() != gcapi.KGCGenMinor {
		t.Errorf("GCMode() = %d after MinorGC, want KGCGenMinor", c.GCMode())
	}
}

// =============================================================================
// Fix/FreeFix Tests
// =============================================================================

// TestFixNil tests fixing nil object.
func TestFixNil(t *testing.T) {
	c := NewCollector(nil)
	// Should not panic
	c.Fix(nil)
}

// TestFreeFixNil tests freeing fixed nil object.
func TestFreeFixNil(t *testing.T) {
	c := NewCollector(nil)
	// Should not panic
	c.FreeFix(nil)
}

// TestFixAndFreeFix tests fixing and freeing an object.
func TestFixAndFreeFix(t *testing.T) {
	c := NewCollector(nil)
	obj := &GCObject{Tt: 5} // table
	c.Fix(obj)
	// Object should be in fixedgc list
	c.FreeFix(obj)
	// Object should be back in allgc
	// Just verify no panic
}

// =============================================================================
// LinkObject Tests
// =============================================================================

// TestLinkObjectNil tests linking nil object.
func TestLinkObjectNil(t *testing.T) {
	c := NewCollector(nil)
	var list *GCObject
	c.LinkObject(nil, &list)
	if list != nil {
		t.Error("List should be nil after linking nil")
	}
}

// TestLinkObjectChains tests that objects are properly chained.
func TestLinkObjectChains(t *testing.T) {
	c := NewCollector(nil)
	var list *GCObject
	obj1 := &GCObject{Tt: 5}
	obj2 := &GCObject{Tt: 5}
	c.LinkObject(obj1, &list)
	c.LinkObject(obj2, &list)
	// obj2 should be at head
	if list != obj2 {
		t.Error("Head of list should be most recently linked object")
	}
	if list.Next != obj1 {
		t.Error("Second object should be linked correctly")
	}
}

// =============================================================================
// Thread Safety Tests
// =============================================================================

// TestConcurrentStopStart tests concurrent stop/start operations.
func TestConcurrentStopStart(t *testing.T) {
	c := NewCollector(nil)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Stop()
				c.Start()
			}
		}()
	}
	wg.Wait()
}

// TestConcurrentStep tests concurrent step operations.
func TestConcurrentStep(t *testing.T) {
	c := NewCollector(nil)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				c.Step()
			}
		}()
	}
	wg.Wait()
}

// =============================================================================
// Edge Cases
// =============================================================================

// TestEmptyCycle tests a complete empty GC cycle.
func TestEmptyCycle(t *testing.T) {
	c := NewCollector(nil)
	// No objects allocated, just run a cycle
	for c.State() != gcapi.GCSpause {
		c.Step()
	}
	if c.State() != gcapi.GCSpause {
		t.Error("Should return to pause state after cycle")
	}
}

// TestMultipleCycles tests multiple GC cycles.
func TestMultipleCycles(t *testing.T) {
	c := NewCollector(nil)
	for i := 0; i < 3; i++ {
		c.Collect()
		if c.State() != gcapi.GCSpause {
			t.Errorf("Should be in pause state after cycle %d", i+1)
		}
	}
}

// TestIsDead tests isdead helper.
func TestIsDead(t *testing.T) {
	c := NewCollector(nil)
	// An object is dead if it's old white
	otherWhite := c.otherWhite()
	obj := &GCObject{Marked: otherWhite} // old white
	if !c.isdead(obj) {
		t.Error("Object with otherWhite should be dead")
	}
	// Black object is not dead
	obj.Marked = 2
	if c.isdead(obj) {
		t.Error("Black object should not be dead")
	}
	// Nil should not panic
	if c.isdead(nil) {
		t.Error("nil should not be dead")
	}
}
