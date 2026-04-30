package lua_test

import (
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

func TestRefTrackerNoLeaks(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tracker := lua.NewRefTracker()

	L.PushString("hello")
	ref := tracker.Ref(L, lua.RegistryIndex)

	if tracker.Count() != 1 {
		t.Fatalf("count = %d, want 1", tracker.Count())
	}

	tracker.Unref(L, lua.RegistryIndex, ref)

	if tracker.Count() != 0 {
		t.Fatalf("count = %d, want 0", tracker.Count())
	}

	leaks := tracker.Leaks()
	if len(leaks) != 0 {
		t.Fatalf("unexpected leaks: %v", leaks)
	}
}

func TestRefTrackerDetectsLeak(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tracker := lua.NewRefTracker()

	L.PushString("leaked")
	_ = tracker.Ref(L, lua.RegistryIndex) // intentionally not Unref'd

	L.PushString("also leaked")
	_ = tracker.Ref(L, lua.RegistryIndex)

	leaks := tracker.Leaks()
	if len(leaks) != 2 {
		t.Fatalf("leaks = %d, want 2", len(leaks))
	}

	// Each leak should have caller info
	for _, leak := range leaks {
		if leak == "" {
			t.Error("empty leak entry")
		}
	}
}
