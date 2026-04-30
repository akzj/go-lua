package lua

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// RefTracker tracks Lua registry references for leak detection.
// Use during development/testing to find Ref() calls without matching Unref().
//
// Example:
//
//	tracker := lua.NewRefTracker()
//	ref := tracker.Ref(L, lua.RegistryIndex)
//	// ... use ref ...
//	tracker.Unref(L, lua.RegistryIndex, ref)
//
//	// At shutdown:
//	leaks := tracker.Leaks()
//	if len(leaks) > 0 {
//	    t.Errorf("ref leaks:\n%s", strings.Join(leaks, "\n"))
//	}
type RefTracker struct {
	mu     sync.Mutex
	active map[int]refInfo
}

type refInfo struct {
	caller string // file:line where Ref was called
}

// NewRefTracker creates a new RefTracker.
func NewRefTracker() *RefTracker {
	return &RefTracker{
		active: make(map[int]refInfo),
	}
}

// Ref creates a registry reference and tracks it.
// Use instead of L.Ref(RegistryIndex).
func (t *RefTracker) Ref(L *State, idx int) int {
	ref := L.Ref(idx)
	if ref > 0 { // don't track RefNil
		t.mu.Lock()
		t.active[ref] = refInfo{caller: callerInfo(2)}
		t.mu.Unlock()
	}
	return ref
}

// Unref frees a registry reference and removes it from tracking.
// Use instead of L.Unref(RegistryIndex, ref).
func (t *RefTracker) Unref(L *State, idx int, ref int) {
	t.mu.Lock()
	delete(t.active, ref)
	t.mu.Unlock()
	L.Unref(idx, ref)
}

// Leaks returns a list of leaked references (Ref'd but never Unref'd).
// Each entry includes the caller location where Ref was called.
func (t *RefTracker) Leaks() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.active) == 0 {
		return nil
	}

	// Sort by ref ID for deterministic output
	refs := make([]int, 0, len(t.active))
	for ref := range t.active {
		refs = append(refs, ref)
	}
	sort.Ints(refs)

	leaks := make([]string, 0, len(refs))
	for _, ref := range refs {
		info := t.active[ref]
		leaks = append(leaks, fmt.Sprintf("  ref=%d created at %s", ref, info.caller))
	}
	return leaks
}

// Count returns the number of currently active (unfreed) references.
func (t *RefTracker) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.active)
}

// Reset clears all tracking state.
func (t *RefTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active = make(map[int]refInfo)
}

// callerInfo returns "file:line" of the caller at the given skip level.
func callerInfo(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}
	// Shorten path to just filename
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		file = file[idx+1:]
	}
	return fmt.Sprintf("%s:%d", file, line)
}
