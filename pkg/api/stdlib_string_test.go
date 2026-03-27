// Package api provides the public Lua API
// This file tests the string standard library module
package api

import (
	"testing"
)

// Debug test to check string module registration
func TestStringModule_Debug(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	// Check if string global exists
	L.GetGlobal("string")
	t.Logf("string type: %v", L.Type(-1))
	
	if L.IsTable(-1) {
		t.Log("string is a table")
		// Check if len is a function
		L.GetField(-1, "len")
		t.Logf("string.len type: %v", L.Type(-1))
		if L.IsFunction(-1) {
			t.Log("string.len is a function")
		} else {
			t.Error("string.len is NOT a function!")
		}
		L.Pop(1)
	} else {
		t.Error("string is NOT a table!")
	}
}

// ============================================================================
// string.len tests
// ============================================================================

func TestStringLength_Basic(t *testing.T) {
	result := doStringNumber(t, `return string.len("hello")`)
	if result != 5 {
		t.Errorf("Expected 5, got %f", result)
	}
}

func TestStringLength_Empty(t *testing.T) {
	result := doStringNumber(t, `return string.len("")`)
	if result != 0 {
		t.Errorf("Expected 0, got %f", result)
	}
}

func TestStringLength_Unicode(t *testing.T) {
	// Lua strings are byte sequences - len counts bytes, not runes
	// "日本語" = 3 chars × 3 bytes each = 9 bytes
	result := doStringNumber(t, `return string.len("日本語")`)
	if result != 9 {
		t.Errorf("Expected 9 (9 UTF-8 bytes), got %f", result)
	}
}

func TestStringLength_UnicodeMixed(t *testing.T) {
	// Mix of ASCII and Unicode - count bytes
	// "abc" = 3 bytes, "日本語" = 9 bytes, "def" = 3 bytes = 15
	result := doStringNumber(t, `return string.len("abc日本語def")`)
	if result != 15 {
		t.Errorf("Expected 15 (6 ASCII + 9 UTF-8 bytes), got %f", result)
	}
}

// ============================================================================
// string.sub tests
// ============================================================================

func TestStringSub_Basic(t *testing.T) {
	result := doStringString(t, `return string.sub("hello", 1, 3)`)
	if result != "hel" {
		t.Errorf("Expected 'hel', got %q", result)
	}
}

func TestStringSub_FullString(t *testing.T) {
	result := doStringString(t, `return string.sub("hello", 1, 5)`)
	if result != "hello" {
		t.Errorf("Expected 'hello', got %q", result)
	}
}

func TestStringSub_NegativeStart(t *testing.T) {
	// -1 means last character
	result := doStringString(t, `return string.sub("hello", -2, -1)`)
	if result != "lo" {
		t.Errorf("Expected 'lo', got %q", result)
	}
}

func TestStringSub_NegativeStartOnly(t *testing.T) {
	// From -3 to end (default j is -1)
	result := doStringString(t, `return string.sub("hello", -3)`)
	if result != "llo" {
		t.Errorf("Expected 'llo', got %q", result)
	}
}

func TestStringSub_OutOfBounds(t *testing.T) {
	// Start beyond string length
	result := doStringString(t, `return string.sub("hello", 10, 20)`)
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestStringSub_StartBeforeOne(t *testing.T) {
	// Start at 0 should clamp to 1
	result := doStringString(t, `return string.sub("hello", 0, 3)`)
	if result != "hel" {
		t.Errorf("Expected 'hel', got %q", result)
	}
}

func TestStringSub_EmptyString(t *testing.T) {
	result := doStringString(t, `return string.sub("", 1, 1)`)
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestStringSub_Unicode(t *testing.T) {
	// Lua strings are byte sequences - sub indexes by bytes
	// "日本語テスト" in UTF-8: each char is 3 bytes
	// bytes 2-4 = second byte of "日" + first two bytes of "本"
	result := doStringString(t, `return string.sub("hello", 2, 4)`)
	if result != "ell" {
		t.Errorf("Expected 'ell', got %q", result)
	}
}

func TestStringSub_StartGreaterThanEnd(t *testing.T) {
	// i > j should return empty string
	result := doStringString(t, `return string.sub("hello", 4, 2)`)
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

// ============================================================================
// string.upper tests
// ============================================================================

func TestStringUpper_Basic(t *testing.T) {
	result := doStringString(t, `return string.upper("hello")`)
	if result != "HELLO" {
		t.Errorf("Expected 'HELLO', got %q", result)
	}
}

func TestStringUpper_Mixed(t *testing.T) {
	result := doStringString(t, `return string.upper("HeLLo WoRLd")`)
	if result != "HELLO WORLD" {
		t.Errorf("Expected 'HELLO WORLD', got %q", result)
	}
}

func TestStringUpper_Empty(t *testing.T) {
	result := doStringString(t, `return string.upper("")`)
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestStringUpper_AlreadyUpper(t *testing.T) {
	result := doStringString(t, `return string.upper("HELLO")`)
	if result != "HELLO" {
		t.Errorf("Expected 'HELLO', got %q", result)
	}
}

func TestStringUpper_NumbersAndSymbols(t *testing.T) {
	result := doStringString(t, `return string.upper("abc123!@#")`)
	if result != "ABC123!@#" {
		t.Errorf("Expected 'ABC123!@#', got %q", result)
	}
}

// ============================================================================
// string.lower tests
// ============================================================================

func TestStringLower_Basic(t *testing.T) {
	result := doStringString(t, `return string.lower("HELLO")`)
	if result != "hello" {
		t.Errorf("Expected 'hello', got %q", result)
	}
}

func TestStringLower_Mixed(t *testing.T) {
	result := doStringString(t, `return string.lower("HeLLo WoRLd")`)
	if result != "hello world" {
		t.Errorf("Expected 'hello world', got %q", result)
	}
}

func TestStringLower_Empty(t *testing.T) {
	result := doStringString(t, `return string.lower("")`)
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestStringLower_AlreadyLower(t *testing.T) {
	result := doStringString(t, `return string.lower("hello")`)
	if result != "hello" {
		t.Errorf("Expected 'hello', got %q", result)
	}
}

func TestStringLower_NumbersAndSymbols(t *testing.T) {
	result := doStringString(t, `return string.lower("ABC123!@#")`)
	if result != "abc123!@#" {
		t.Errorf("Expected 'abc123!@#', got %q", result)
	}
}

// ============================================================================
// string.find tests
// ============================================================================

func TestStringFind_Basic(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`local s, e = string.find("hello world", "world"); return s, e`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	start, _ := L.ToNumber(-2)
	end, _ := L.ToNumber(-1)
	if start != 7 || end != 11 {
		t.Errorf("Expected start=7, end=11, got start=%f, end=%f", start, end)
	}
}

func TestStringFind_NotFound(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`local s, e = string.find("hello", "xyz"); return s, e`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	if !L.IsNil(-2) {
		t.Error("Expected nil for start when pattern not found")
	}
}

func TestStringFind_EmptyPattern(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`local s, e = string.find("hello", ""); return s, e`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	start, _ := L.ToNumber(-2)
	end, _ := L.ToNumber(-1)
	if start != 1 || end != 0 {
		t.Errorf("Expected start=1, end=0 for empty pattern, got start=%f, end=%f", start, end)
	}
}

func TestStringFind_WithInit(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	// Find "o" starting from position 5
	err := L.DoString(`local s, e = string.find("hello world", "o", 5); return s, e`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	start, _ := L.ToNumber(-2)
	end, _ := L.ToNumber(-1)
	if start != 5 || end != 5 {
		t.Errorf("Expected start=5, end=5, got start=%f, end=%f", start, end)
	}
}

func TestStringFind_NegativeInit(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	// Find from position -5 (which is position 7 in "hello world")
	err := L.DoString(`local s, e = string.find("hello world", "o", -5); return s, e`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	start, _ := L.ToNumber(-2)
	end, _ := L.ToNumber(-1)
	if start != 8 || end != 8 {
		t.Errorf("Expected start=8, end=8, got start=%f, end=%f", start, end)
	}
}

func TestStringFind_FirstCharacter(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`local s, e = string.find("hello", "h"); return s, e`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	start, _ := L.ToNumber(-2)
	end, _ := L.ToNumber(-1)
	if start != 1 || end != 1 {
		t.Errorf("Expected start=1, end=1, got start=%f, end=%f", start, end)
	}
}

// ============================================================================
// string.format tests
// ============================================================================

func TestStringFormat_String(t *testing.T) {
	result := doStringString(t, `return string.format("Hello %s", "World")`)
	if result != "Hello World" {
		t.Errorf("Expected 'Hello World', got %q", result)
	}
}

func TestStringFormat_Integer(t *testing.T) {
	result := doStringString(t, `return string.format("Number: %d", 42)`)
	if result != "Number: 42" {
		t.Errorf("Expected 'Number: 42', got %q", result)
	}
}

func TestStringFormat_Float(t *testing.T) {
	result := doStringString(t, `return string.format("Pi: %f", 3.14159)`)
	if result != "Pi: 3.141590" {
		t.Errorf("Expected 'Pi: 3.141590', got %q", result)
	}
}

func TestStringFormat_Percent(t *testing.T) {
	result := doStringString(t, `return string.format("100%%")`)
	if result != "100%" {
		t.Errorf("Expected '100%%', got %q", result)
	}
}

func TestStringFormat_Multiple(t *testing.T) {
	result := doStringString(t, `return string.format("%s has %d apples", "Alice", 5)`)
	if result != "Alice has 5 apples" {
		t.Errorf("Expected 'Alice has 5 apples', got %q", result)
	}
}

func TestStringFormat_Quoted(t *testing.T) {
	result := doStringString(t, `return string.format("%q", "hello")`)
	// %q should quote the string
	if result != `"hello"` {
		t.Errorf("Expected '\"hello\"', got %q", result)
	}
}

func TestStringFormat_Empty(t *testing.T) {
	result := doStringString(t, `return string.format("")`)
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestStringFormat_NoFormat(t *testing.T) {
	result := doStringString(t, `return string.format("no format specifiers")`)
	if result != "no format specifiers" {
		t.Errorf("Expected 'no format specifiers', got %q", result)
	}
}

// ============================================================================
// string.rep tests
// ============================================================================

func TestStringRep_Basic(t *testing.T) {
	result := doStringString(t, `return string.rep("ab", 3)`)
	if result != "ababab" {
		t.Errorf("Expected 'ababab', got %q", result)
	}
}

func TestStringRep_Zero(t *testing.T) {
	result := doStringString(t, `return string.rep("hello", 0)`)
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestStringRep_One(t *testing.T) {
	result := doStringString(t, `return string.rep("x", 1)`)
	if result != "x" {
		t.Errorf("Expected 'x', got %q", result)
	}
}

func TestStringRep_WithSeparator(t *testing.T) {
	result := doStringString(t, `return string.rep("a", 3, "-")`)
	if result != "a-a-a" {
		t.Errorf("Expected 'a-a-a', got %q", result)
	}
}

func TestStringRep_EmptyString(t *testing.T) {
	result := doStringString(t, `return string.rep("", 5)`)
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestStringRep_EmptySeparator(t *testing.T) {
	result := doStringString(t, `return string.rep("x", 3, "")`)
	if result != "xxx" {
		t.Errorf("Expected 'xxx', got %q", result)
	}
}

// ============================================================================
// string.reverse tests
// ============================================================================

func TestStringReverse_Basic(t *testing.T) {
	result := doStringString(t, `return string.reverse("hello")`)
	if result != "olleh" {
		t.Errorf("Expected 'olleh', got %q", result)
	}
}

func TestStringReverse_Empty(t *testing.T) {
	result := doStringString(t, `return string.reverse("")`)
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestStringReverse_Single(t *testing.T) {
	result := doStringString(t, `return string.reverse("a")`)
	if result != "a" {
		t.Errorf("Expected 'a', got %q", result)
	}
}

func TestStringReverse_Palindrome(t *testing.T) {
	result := doStringString(t, `return string.reverse("radar")`)
	if result != "radar" {
		t.Errorf("Expected 'radar', got %q", result)
	}
}

func TestStringReverse_Unicode(t *testing.T) {
	// Lua reverses by bytes, not runes
	result := doStringString(t, `return string.reverse("abc")`)
	if result != "cba" {
		t.Errorf("Expected 'cba', got %q", result)
	}
}

// ============================================================================
// string.byte tests
// ============================================================================

func TestStringByte_Single(t *testing.T) {
	result := doStringNumber(t, `return string.byte("A")`)
	if result != 65 {
		t.Errorf("Expected 65 (ASCII for 'A'), got %f", result)
	}
}

func TestStringByte_WithIndex(t *testing.T) {
	result := doStringNumber(t, `return string.byte("ABC", 2)`)
	if result != 66 {
		t.Errorf("Expected 66 (ASCII for 'B'), got %f", result)
	}
}

func TestStringByte_Range(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`return string.byte("ABC", 1, 3)`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Should return 3 values: 65, 66, 67
	if L.GetTop() != 3 {
		t.Errorf("Expected 3 return values, got %d", L.GetTop())
	}

	b1, _ := L.ToNumber(-3)
	b2, _ := L.ToNumber(-2)
	b3, _ := L.ToNumber(-1)

	if b1 != 65 || b2 != 66 || b3 != 67 {
		t.Errorf("Expected [65, 66, 67], got [%f, %f, %f]", b1, b2, b3)
	}
}

func TestStringByte_Empty(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`return string.byte("")`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Empty string should return nothing
	if L.GetTop() != 0 {
		t.Errorf("Expected 0 return values for empty string, got %d", L.GetTop())
	}
}

func TestStringByte_NegativeIndex(t *testing.T) {
	result := doStringNumber(t, `return string.byte("ABC", -1)`)
	if result != 67 {
		t.Errorf("Expected 67 (ASCII for 'C'), got %f", result)
	}
}

func TestStringByte_Unicode(t *testing.T) {
	// Lua string.byte returns byte value, not Unicode codepoint
	// "日" in UTF-8 starts with byte 0xE6 = 230
	result := doStringNumber(t, `return string.byte("日")`)
	if result != 230 {
		t.Errorf("Expected 230 (first UTF-8 byte of '日'), got %f", result)
	}
}

// ============================================================================
// string.char tests
// ============================================================================

func TestStringChar_Single(t *testing.T) {
	result := doStringString(t, `return string.char(65)`)
	if result != "A" {
		t.Errorf("Expected 'A', got %q", result)
	}
}

func TestStringChar_Multiple(t *testing.T) {
	result := doStringString(t, `return string.char(65, 66, 67)`)
	if result != "ABC" {
		t.Errorf("Expected 'ABC', got %q", result)
	}
}

func TestStringChar_Zero(t *testing.T) {
	result := doStringString(t, `return string.char(0)`)
	if result != "\x00" {
		t.Errorf("Expected null character, got %q", result)
	}
}

func TestStringChar_Empty(t *testing.T) {
	// No arguments should return empty string
	result := doStringString(t, `return string.char()`)
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestStringChar_Unicode(t *testing.T) {
	// Lua string.char only accepts 0-255 (byte values)
	// Test with a high byte value
	result := doStringString(t, `return string.char(230)`)
	if result != "\xe6" {
		t.Errorf("Expected byte 0xE6, got %q", result)
	}
}

// ============================================================================
// string.gsub tests
// ============================================================================

func TestStringGsub_Basic(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`local s, n = string.gsub("hello world", "o", "a"); return s, n`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, _ := L.ToString(-2)
	count, _ := L.ToNumber(-1)

	if result != "hella warld" {
		t.Errorf("Expected 'hella warld', got %q", result)
	}
	if count != 2 {
		t.Errorf("Expected count 2, got %f", count)
	}
}

func TestStringGsub_NoMatch(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`local s, n = string.gsub("hello", "x", "y"); return s, n`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, _ := L.ToString(-2)
	count, _ := L.ToNumber(-1)

	if result != "hello" {
		t.Errorf("Expected 'hello', got %q", result)
	}
	if count != 0 {
		t.Errorf("Expected count 0, got %f", count)
	}
}

func TestStringGsub_WithLimit(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`local s, n = string.gsub("aaa", "a", "b", 2); return s, n`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, _ := L.ToString(-2)
	count, _ := L.ToNumber(-1)

	if result != "bba" {
		t.Errorf("Expected 'bba', got %q", result)
	}
	if count != 2 {
		t.Errorf("Expected count 2, got %f", count)
	}
}

func TestStringGsub_EmptyString(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`local s, n = string.gsub("", "x", "y"); return s, n`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, _ := L.ToString(-2)
	count, _ := L.ToNumber(-1)

	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
	if count != 0 {
		t.Errorf("Expected count 0, got %f", count)
	}
}

func TestStringGsub_DeletePattern(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	// Replace with empty string to delete
	err := L.DoString(`local s, n = string.gsub("hello world", "l", ""); return s, n`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, _ := L.ToString(-2)
	count, _ := L.ToNumber(-1)

	if result != "heo word" {
		t.Errorf("Expected 'heo word', got %q", result)
	}
	if count != 3 {
		t.Errorf("Expected count 3, got %f", count)
	}
}

// ============================================================================
// string.match tests
// ============================================================================

func TestStringMatch_Basic(t *testing.T) {
	result := doStringString(t, `return string.match("hello world", "world")`)
	if result != "world" {
		t.Errorf("Expected 'world', got %q", result)
	}
}

func TestStringMatch_NotFound(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`return string.match("hello", "xyz")`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	if !L.IsNil(-1) {
		t.Error("Expected nil when pattern not found")
	}
}

func TestStringMatch_WithInit(t *testing.T) {
	result := doStringString(t, `return string.match("hello hello", "hello", 7)`)
	if result != "hello" {
		t.Errorf("Expected 'hello', got %q", result)
	}
}

func TestStringMatch_NegativeInit(t *testing.T) {
	result := doStringString(t, `return string.match("hello world", "world", -5)`)
	if result != "world" {
		t.Errorf("Expected 'world', got %q", result)
	}
}

func TestStringMatch_EmptyString(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`return string.match("", "x")`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	if !L.IsNil(-1) {
		t.Error("Expected nil when searching empty string")
	}
}

// ============================================================================
// string.gmatch tests
// ============================================================================

func TestStringGmatch_Basic(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	// gmatch returns an iterator, collect all matches
	err := L.DoString(`
		local result = {}
		for s in string.gmatch("hello world world", "world") do
			table.insert(result, s)
		end
		return result[1], result[2]
	`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	r1, _ := L.ToString(-2)
	r2, _ := L.ToString(-1)

	if r1 != "world" || r2 != "world" {
		t.Errorf("Expected 'world', 'world', got %q, %q", r1, r2)
	}
}

func TestStringGmatch_NoMatch(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	// No matches - iterator should not yield anything
	err := L.DoString(`
		local count = 0
		for s in string.gmatch("hello", "xyz") do
			count = count + 1
		end
		return count
	`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	count, _ := L.ToNumber(-1)
	if count != 0 {
		t.Errorf("Expected 0 iterations, got %f", count)
	}
}

func TestStringGmatch_EmptyString(t *testing.T) {
	L := newState(t)
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`
		local count = 0
		for s in string.gmatch("", "x") do
			count = count + 1
		end
		return count
	`, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	count, _ := L.ToNumber(-1)
	if count != 0 {
		t.Errorf("Expected 0 iterations, got %f", count)
	}
}

// ============================================================================
// Edge Cases and Error Handling
// ============================================================================

func TestStringLen_InvalidArg(t *testing.T) {
	// When called with no argument, should return 0
	result := doStringNumber(t, `return string.len()`)
	if result != 0 {
		t.Errorf("Expected 0 for no argument, got %f", result)
	}
}

func TestStringSub_InvalidArg(t *testing.T) {
	// When called with no string argument, should return empty string
	result := doStringString(t, `return string.sub()`)
	if result != "" {
		t.Errorf("Expected empty string for no argument, got %q", result)
	}
}

func TestStringUpper_InvalidArg(t *testing.T) {
	// When called with no argument, should return empty string
	result := doStringString(t, `return string.upper()`)
	if result != "" {
		t.Errorf("Expected empty string for no argument, got %q", result)
	}
}

func TestStringLower_InvalidArg(t *testing.T) {
	// When called with no argument, should return empty string
	result := doStringString(t, `return string.lower()`)
	if result != "" {
		t.Errorf("Expected empty string for no argument, got %q", result)
	}
}

func TestStringReverse_InvalidArg(t *testing.T) {
	// When called with no argument, should return empty string
	result := doStringString(t, `return string.reverse()`)
	if result != "" {
		t.Errorf("Expected empty string for no argument, got %q", result)
	}
}

// Helper function to create a new state
func newState(t *testing.T) *State {
	t.Helper()
	L := NewState()
	if L == nil {
		t.Fatal("NewState returned nil")
	}
	return L
}