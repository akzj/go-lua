package api

import "testing"

// ---------------------------------------------------------------------------
// Ref / Unref (luaL_ref / luaL_unref)
// ---------------------------------------------------------------------------

// TestRefBasicCycle pushes a value, refs it into the registry, retrieves it
// via RawGetI, and unrefs it.
func TestRefBasicCycle(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushString("hello")
	ref := L.Ref(RegistryIndex)
	if ref <= 0 {
		t.Fatalf("Ref returned %d, want > 0", ref)
	}

	// Retrieve the value via registry[ref]
	L.RawGetI(RegistryIndex, int64(ref))
	got, ok := L.ToString(-1)
	if !ok || got != "hello" {
		t.Fatalf("registry[%d] = %q (ok=%v), want \"hello\"", ref, got, ok)
	}
	L.Pop(1)

	// Unref should not panic
	L.Unref(RegistryIndex, ref)
}

// TestRefMultipleUnique creates several refs and verifies each returns a
// distinct key that maps to the correct value.
func TestRefMultipleUnique(t *testing.T) {
	L := NewState()
	defer L.Close()

	values := []string{"alpha", "beta", "gamma", "delta"}
	refs := make([]int, len(values))

	for i, v := range values {
		L.PushString(v)
		refs[i] = L.Ref(RegistryIndex)
		if refs[i] <= 0 {
			t.Fatalf("Ref for %q returned %d, want > 0", v, refs[i])
		}
	}

	// All refs must be unique
	seen := map[int]bool{}
	for _, r := range refs {
		if seen[r] {
			t.Fatalf("duplicate ref key %d", r)
		}
		seen[r] = true
	}

	// Each ref must retrieve the correct value
	for i, v := range values {
		L.RawGetI(RegistryIndex, int64(refs[i]))
		got, ok := L.ToString(-1)
		if !ok || got != v {
			t.Fatalf("registry[%d] = %q (ok=%v), want %q", refs[i], got, ok, v)
		}
		L.Pop(1)
	}
}

// TestRefReuseAfterUnref verifies that Unref returns a slot to the free list
// and a subsequent Ref reuses it.
func TestRefReuseAfterUnref(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Create two refs
	L.PushString("first")
	ref1 := L.Ref(RegistryIndex)
	L.PushString("second")
	ref2 := L.Ref(RegistryIndex)

	// Free the first ref
	L.Unref(RegistryIndex, ref1)

	// Next Ref should reuse ref1's slot
	L.PushString("third")
	ref3 := L.Ref(RegistryIndex)
	if ref3 != ref1 {
		t.Fatalf("expected reuse of slot %d, got %d", ref1, ref3)
	}

	// ref2 should still be intact
	L.RawGetI(RegistryIndex, int64(ref2))
	got, ok := L.ToString(-1)
	if !ok || got != "second" {
		t.Fatalf("registry[%d] = %q (ok=%v), want \"second\"", ref2, got, ok)
	}
	L.Pop(1)

	// ref3 should hold the new value
	L.RawGetI(RegistryIndex, int64(ref3))
	got, ok = L.ToString(-1)
	if !ok || got != "third" {
		t.Fatalf("registry[%d] = %q (ok=%v), want \"third\"", ref3, got, ok)
	}
	L.Pop(1)
}

// TestRefNilReturnsRefNil verifies that Ref on a nil value returns RefNil
// and does not store anything.
func TestRefNilReturnsRefNil(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushNil()
	ref := L.Ref(RegistryIndex)
	if ref != RefNil {
		t.Fatalf("Ref(nil) = %d, want RefNil (%d)", ref, RefNil)
	}
}

// TestUnrefNoRefAndRefNilAreNoOps verifies that calling Unref with NoRef or
// RefNil does not panic or corrupt state.
func TestUnrefNoRefAndRefNilAreNoOps(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Create a real ref first so the registry has some state
	L.PushString("sentinel")
	ref := L.Ref(RegistryIndex)

	// These must not panic
	L.Unref(RegistryIndex, NoRef)
	L.Unref(RegistryIndex, RefNil)
	L.Unref(RegistryIndex, 0) // zero is also a no-op (free list head)

	// The real ref should be unaffected
	L.RawGetI(RegistryIndex, int64(ref))
	got, ok := L.ToString(-1)
	if !ok || got != "sentinel" {
		t.Fatalf("registry[%d] = %q (ok=%v), want \"sentinel\"", ref, got, ok)
	}
	L.Pop(1)
}

// ---------------------------------------------------------------------------
// Double-free / corruption guards
// ---------------------------------------------------------------------------

// TestRefDoubleUnref verifies that calling Unref twice with the same ref
// does not corrupt the free list (no self-loop causing duplicate Ref returns).
func TestRefDoubleUnref(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushString("val1")
	r1 := L.Ref(RegistryIndex)
	L.PushString("val2")
	_ = L.Ref(RegistryIndex) // r2, keep it alive

	// Double unref r1 — should not corrupt free list
	L.Unref(RegistryIndex, r1)
	L.Unref(RegistryIndex, r1) // double-free!

	// Two new refs should get DIFFERENT slots
	L.PushString("new1")
	nr1 := L.Ref(RegistryIndex)
	L.PushString("new2")
	nr2 := L.Ref(RegistryIndex)

	if nr1 == nr2 {
		t.Fatalf("double Unref caused duplicate ref: nr1=%d nr2=%d", nr1, nr2)
	}

	// Verify values are correct
	L.RawGetI(RegistryIndex, int64(nr1))
	v1, _ := L.ToString(-1)
	L.Pop(1)
	L.RawGetI(RegistryIndex, int64(nr2))
	v2, _ := L.ToString(-1)
	L.Pop(1)

	if v1 != "new1" {
		t.Fatalf("expected 'new1', got %q", v1)
	}
	if v2 != "new2" {
		t.Fatalf("expected 'new2', got %q", v2)
	}
}

// TestRefDoubleUnrefNonConsecutive verifies the self-loop guard in Ref
// catches corruption even if the double-free wasn't caught by Unref's
// head-check (e.g., A freed, B freed, A freed again → cycle).
func TestRefDoubleUnrefNonConsecutive(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushString("a")
	rA := L.Ref(RegistryIndex)
	L.PushString("b")
	rB := L.Ref(RegistryIndex)
	L.PushString("c")
	_ = L.Ref(RegistryIndex) // rC, keep alive

	// Free A, then B, then A again (non-consecutive double-free)
	L.Unref(RegistryIndex, rA)
	L.Unref(RegistryIndex, rB)
	L.Unref(RegistryIndex, rA) // A is freed again — creates cycle A→B→A→B...

	// Allocate several refs — they must all be unique
	seen := map[int]bool{}
	for i := 0; i < 5; i++ {
		L.PushString("x")
		ref := L.Ref(RegistryIndex)
		if seen[ref] {
			t.Fatalf("got duplicate ref %d at iteration %d", ref, i)
		}
		seen[ref] = true
	}

	_ = rB // suppress unused warning
}

// TestRefConsecutiveRefsUnique allocates many refs without any Unref and
// verifies all are unique.
func TestRefConsecutiveRefsUnique(t *testing.T) {
	L := NewState()
	defer L.Close()

	refs := make(map[int]bool)
	for i := 0; i < 100; i++ {
		L.PushInteger(int64(i))
		ref := L.Ref(RegistryIndex)
		if refs[ref] {
			t.Fatalf("duplicate ref %d at iteration %d", ref, i)
		}
		refs[ref] = true
	}
}

// TestRefUnrefAndReuseAll frees all refs and then re-allocates, verifying
// no duplicates occur.
func TestRefUnrefAndReuseAll(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Allocate 5 refs
	var allRefs []int
	for i := 0; i < 5; i++ {
		L.PushInteger(int64(i))
		ref := L.Ref(RegistryIndex)
		allRefs = append(allRefs, ref)
	}

	// Free all
	for _, ref := range allRefs {
		L.Unref(RegistryIndex, ref)
	}

	// Allocate 5 more — should reuse freed slots, all unique
	newRefs := make(map[int]bool)
	for i := 0; i < 5; i++ {
		L.PushInteger(int64(i + 100))
		ref := L.Ref(RegistryIndex)
		if newRefs[ref] {
			t.Fatalf("duplicate ref %d at iteration %d", ref, i)
		}
		newRefs[ref] = true
	}
}
