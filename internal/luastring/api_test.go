package luastring

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// Hash tests
// ---------------------------------------------------------------------------

func TestHashDeterministic(t *testing.T) {
	// Same input + seed → same hash
	h1 := Hash("hello", 42)
	h2 := Hash("hello", 42)
	if h1 != h2 {
		t.Errorf("Hash not deterministic: %d != %d", h1, h2)
	}
}

func TestHashSeedMatters(t *testing.T) {
	h1 := Hash("hello", 0)
	h2 := Hash("hello", 12345)
	if h1 == h2 {
		t.Errorf("Different seeds produced same hash: %d", h1)
	}
}

func TestHashDifferentStrings(t *testing.T) {
	h1 := Hash("hello", 42)
	h2 := Hash("world", 42)
	if h1 == h2 {
		t.Errorf("Different strings produced same hash: %d", h1)
	}
}

func TestHashEmptyString(t *testing.T) {
	// h = seed ^ 0 = seed, then no iterations
	h := Hash("", 42)
	if h != 42 {
		t.Errorf("Hash of empty string with seed 42 = %d, want 42", h)
	}
}

func TestHashSingleByte(t *testing.T) {
	// h = seed ^ 1, then one iteration: h ^= (h<<5) + (h>>2) + byte
	seed := uint32(0)
	h := seed ^ 1
	h ^= (h << 5) + (h >> 2) + uint32('A')
	expected := h
	got := Hash("A", 0)
	if got != expected {
		t.Errorf("Hash('A', 0) = %d, want %d", got, expected)
	}
}

func TestHashMatchesC(t *testing.T) {
	// Manually compute C's luaS_hash for "abc" with seed=0:
	// h = 0 ^ 3 = 3
	// l=3: l-- → l=2, h ^= (3<<5)+(3>>2)+'c' = 96+0+99 = 195, h = 3^195 = 192
	// l=2: l-- → l=1, h ^= (192<<5)+(192>>2)+'b' = 6144+48+98 = 6290, h = 192^6290 = 6418
	// l=1: l-- → l=0, h ^= (6418<<5)+(6418>>2)+'a' = 205376+1604+97 = 207077, h = 6418^207077 = 201175
	seed := uint32(0)
	h := seed ^ 3
	// byte 2 = 'c' (99)
	h ^= (h << 5) + (h >> 2) + 99
	// byte 1 = 'b' (98)
	h ^= (h << 5) + (h >> 2) + 98
	// byte 0 = 'a' (97)
	h ^= (h << 5) + (h >> 2) + 97
	expected := h
	got := Hash("abc", 0)
	if got != expected {
		t.Errorf("Hash('abc', 0) = %d, want %d", got, expected)
	}
}

func TestHashBytesMatchesHash(t *testing.T) {
	s := "test string for hash"
	h1 := Hash(s, 999)
	h2 := hashBytes([]byte(s), 999)
	if h1 != h2 {
		t.Errorf("Hash and HashBytes differ: %d != %d", h1, h2)
	}
}

// ---------------------------------------------------------------------------
// StringTable — Intern tests
// ---------------------------------------------------------------------------

func TestInternShortSamePointer(t *testing.T) {
	st := NewStringTable(42)
	s1 := st.Intern("hello")
	s2 := st.Intern("hello")
	if s1 != s2 {
		t.Error("Intern should return same pointer for same short string")
	}
}

func TestInternShortDifferentPointers(t *testing.T) {
	st := NewStringTable(42)
	s1 := st.Intern("hello")
	s2 := st.Intern("world")
	if s1 == s2 {
		t.Error("Intern should return different pointers for different strings")
	}
}

func TestInternShortIsShort(t *testing.T) {
	st := NewStringTable(42)
	s := st.Intern("hello")
	if !s.IsShort {
		t.Error("Short string should have IsShort=true")
	}
	if s.Tag() != object.TagShortStr {
		t.Errorf("Short string tag = %d, want %d", s.Tag(), object.TagShortStr)
	}
}

func TestInternShortHashSet(t *testing.T) {
	st := NewStringTable(42)
	s := st.Intern("hello")
	expected := Hash("hello", 42)
	if s.Hash_ != expected {
		t.Errorf("Interned string hash = %d, want %d", s.Hash_, expected)
	}
}

func TestInternLongNotInterned(t *testing.T) {
	st := NewStringTable(42)
	long := strings.Repeat("a", MaxShortLen+1) // 41 bytes
	s1 := st.Intern(long)
	s2 := st.Intern(long)
	if s1 == s2 {
		t.Error("Long strings should NOT be interned (different pointers)")
	}
	if s1.IsShort {
		t.Error("Long string should have IsShort=false")
	}
	if s1.Tag() != object.TagLongStr {
		t.Errorf("Long string tag = %d, want %d", s1.Tag(), object.TagLongStr)
	}
}

func TestInternLongHashZero(t *testing.T) {
	st := NewStringTable(42)
	long := strings.Repeat("b", MaxShortLen+1)
	s := st.Intern(long)
	if s.Hash_ != 0 {
		t.Errorf("Long string hash should be 0 (lazy), got %d", s.Hash_)
	}
}

func TestInternBoundary40(t *testing.T) {
	st := NewStringTable(42)
	s40 := strings.Repeat("x", MaxShortLen) // exactly 40 bytes
	s1 := st.Intern(s40)
	s2 := st.Intern(s40)
	if s1 != s2 {
		t.Error("40-byte string should be interned (same pointer)")
	}
	if !s1.IsShort {
		t.Error("40-byte string should be short")
	}
}

func TestInternBoundary41(t *testing.T) {
	st := NewStringTable(42)
	s41 := strings.Repeat("x", MaxShortLen+1) // 41 bytes
	s1 := st.Intern(s41)
	s2 := st.Intern(s41)
	if s1 == s2 {
		t.Error("41-byte string should NOT be interned (different pointers)")
	}
	if s1.IsShort {
		t.Error("41-byte string should be long")
	}
}

func TestInternEmptyString(t *testing.T) {
	st := NewStringTable(42)
	s1 := st.Intern("")
	s2 := st.Intern("")
	if s1 != s2 {
		t.Error("Empty string should be interned (same pointer)")
	}
	if !s1.IsShort {
		t.Error("Empty string should be short")
	}
	if s1.Data != "" {
		t.Error("Empty string data should be empty")
	}
}

func TestInternCount(t *testing.T) {
	st := NewStringTable(42)
	st.Intern("a")
	st.Intern("b")
	st.Intern("c")
	st.Intern("a") // duplicate — should not increase count
	if st.Count() != 3 {
		t.Errorf("Count = %d, want 3", st.Count())
	}
}

func TestInternLongDoesNotAffectCount(t *testing.T) {
	st := NewStringTable(42)
	long := strings.Repeat("z", MaxShortLen+1)
	st.Intern(long)
	st.Intern(long)
	if st.Count() != 0 {
		t.Errorf("Long strings should not affect count, got %d", st.Count())
	}
}

// ---------------------------------------------------------------------------
// InternBytes tests
// ---------------------------------------------------------------------------

func TestInternBytesMatchesIntern(t *testing.T) {
	st := NewStringTable(42)
	s1 := st.Intern("hello")
	s2 := st.InternBytes([]byte("hello"))
	if s1 != s2 {
		t.Error("InternBytes should return same pointer as Intern for same content")
	}
}

func TestInternBytesLong(t *testing.T) {
	st := NewStringTable(42)
	long := []byte(strings.Repeat("q", MaxShortLen+1))
	s1 := st.InternBytes(long)
	s2 := st.InternBytes(long)
	if s1 == s2 {
		t.Error("InternBytes for long strings should not intern")
	}
}

// ---------------------------------------------------------------------------
// Resize tests
// ---------------------------------------------------------------------------

func TestResizeOnManyStrings(t *testing.T) {
	st := NewStringTable(42)
	strs := make([]*object.LuaString, 0, 300)

	// Insert 300 unique short strings — will trigger multiple resizes
	// (initial 128 buckets → 256 → 512 as count exceeds bucket count)
	for i := 0; i < 300; i++ {
		s := fmt.Sprintf("str_%04d", i)
		strs = append(strs, st.Intern(s))
	}

	if st.Count() != 300 {
		t.Errorf("Count = %d, want 300", st.Count())
	}

	// Verify all strings are still findable after resize
	for i := 0; i < 300; i++ {
		s := fmt.Sprintf("str_%04d", i)
		found := st.Intern(s)
		if found != strs[i] {
			t.Errorf("String %q not found after resize (different pointer)", s)
		}
	}

	// Count should not have changed (all were duplicates)
	if st.Count() != 300 {
		t.Errorf("Count after re-lookup = %d, want 300", st.Count())
	}
}

// ---------------------------------------------------------------------------
// Equal tests
// ---------------------------------------------------------------------------

func TestEqualShortShortSame(t *testing.T) {
	st := NewStringTable(42)
	s1 := st.Intern("hello")
	s2 := st.Intern("hello")
	if !equal(s1, s2) {
		t.Error("Equal should return true for same interned short string")
	}
}

func TestEqualShortShortDifferent(t *testing.T) {
	st := NewStringTable(42)
	s1 := st.Intern("hello")
	s2 := st.Intern("world")
	if equal(s1, s2) {
		t.Error("Equal should return false for different short strings")
	}
}

func TestEqualLongLongSameContent(t *testing.T) {
	st := NewStringTable(42)
	long := strings.Repeat("x", MaxShortLen+1)
	s1 := st.Intern(long)
	s2 := st.Intern(long)
	// Different pointers but same content
	if s1 == s2 {
		t.Fatal("Precondition: long strings should be different pointers")
	}
	if !equal(s1, s2) {
		t.Error("Equal should return true for long strings with same content")
	}
}

func TestEqualLongLongDifferentContent(t *testing.T) {
	st := NewStringTable(42)
	s1 := st.Intern(strings.Repeat("x", MaxShortLen+1))
	s2 := st.Intern(strings.Repeat("y", MaxShortLen+1))
	if equal(s1, s2) {
		t.Error("Equal should return false for long strings with different content")
	}
}

func TestEqualShortLongSameContent(t *testing.T) {
	// A short string and a long string with the same content (shouldn't happen
	// in practice since length determines short/long, but test the logic)
	short := &object.LuaString{Data: "hello", Hash_: 0, IsShort: true}
	long := &object.LuaString{Data: "hello", Hash_: 0, IsShort: false}
	if !equal(short, long) {
		t.Error("Equal should compare content when one is long")
	}
}

func TestEqualSamePointer(t *testing.T) {
	s := &object.LuaString{Data: "test", Hash_: 0, IsShort: true}
	if !equal(s, s) {
		t.Error("Equal should return true for same pointer")
	}
}

// ---------------------------------------------------------------------------
// Seed test
// ---------------------------------------------------------------------------

func TestSeed(t *testing.T) {
	st := NewStringTable(12345)
	if st.Seed() != 12345 {
		t.Errorf("Seed = %d, want 12345", st.Seed())
	}
}

// ---------------------------------------------------------------------------
// Content correctness
// ---------------------------------------------------------------------------

func TestInternedStringContent(t *testing.T) {
	st := NewStringTable(42)
	s := st.Intern("hello world")
	if s.String() != "hello world" {
		t.Errorf("String() = %q, want %q", s.String(), "hello world")
	}
	if s.Len() != 11 {
		t.Errorf("Len() = %d, want 11", s.Len())
	}
}

// ---------------------------------------------------------------------------
// Sweep / weak pointer tests — verify strings are collected by Go GC
// ---------------------------------------------------------------------------

func TestSweepStringsCollectsUnreferencedStrings(t *testing.T) {
	st := NewStringTable(42)

	// Phase 1: Intern 500 unique short strings, holding strong refs
	const N = 500
	refs := make([]*object.LuaString, N)
	for i := 0; i < N; i++ {
		refs[i] = st.Intern(fmt.Sprintf("sweep_%04d", i))
	}
	countBefore := st.Count()
	if countBefore != N {
		t.Fatalf("expected count=%d after interning, got %d", N, countBefore)
	}

	// Phase 2: Drop all strong references
	for i := range refs {
		refs[i] = nil
	}
	refs = nil

	// Phase 3: Force Go GC to collect the unreferenced LuaStrings
	runtime.GC()
	runtime.GC() // second pass to be thorough

	// Phase 4: Sweep the string table — should remove dead weak pointers
	st.SweepStrings()

	countAfter := st.Count()
	t.Logf("count before=%d, after sweep=%d", countBefore, countAfter)

	// We expect a significant decrease. Due to GC timing, we may not get
	// all of them collected, but the vast majority should be gone.
	// Accept if at least 80% were collected.
	if countAfter > N/5 {
		t.Errorf("expected most strings to be collected: before=%d, after=%d (threshold=%d)",
			countBefore, countAfter, N/5)
	}
}

func TestSweepStringsKeepsReferencedStrings(t *testing.T) {
	st := NewStringTable(42)

	// Intern strings and keep strong references to half of them
	const N = 200
	kept := make([]*object.LuaString, 0, N/2)
	for i := 0; i < N; i++ {
		s := st.Intern(fmt.Sprintf("keep_%04d", i))
		if i%2 == 0 {
			kept = append(kept, s)
		}
	}

	if st.Count() != N {
		t.Fatalf("expected count=%d, got %d", N, st.Count())
	}

	// Force GC — only unreferenced strings (odd indices) should be collected
	runtime.GC()
	runtime.GC()
	st.SweepStrings()

	countAfter := st.Count()
	t.Logf("count after sweep=%d (kept %d strong refs)", countAfter, len(kept))

	// The kept strings (100) must still be alive
	if countAfter < len(kept) {
		t.Errorf("expected at least %d strings to survive (kept refs), got %d",
			len(kept), countAfter)
	}

	// Verify the kept strings are still internable (same pointer)
	for i, s := range kept {
		name := fmt.Sprintf("keep_%04d", i*2)
		found := st.Intern(name)
		if found != s {
			t.Errorf("kept string %q: pointer changed after sweep (should be same)", name)
		}
	}

	// Use kept to prevent compiler from optimizing away the references
	runtime.KeepAlive(kept)
}
