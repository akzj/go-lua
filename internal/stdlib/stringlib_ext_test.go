package stdlib

import (
	"testing"
)

// ===== STRING SPLIT =====

func TestStringSplitBasic(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = string.split("a,b,c", ",")
		assert(#r == 3)
		assert(r[1] == "a")
		assert(r[2] == "b")
		assert(r[3] == "c")
	`)
}

func TestStringSplitEmptyParts(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = string.split("a,,b", ",")
		assert(#r == 3)
		assert(r[1] == "a")
		assert(r[2] == "")
		assert(r[3] == "b")
	`)
}

func TestStringSplitMaxsplit(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = string.split("a,b,c,d", ",", 2)
		assert(#r == 3, "expected 3 parts, got " .. #r)
		assert(r[1] == "a")
		assert(r[2] == "b")
		assert(r[3] == "c,d")
	`)
}

func TestStringSplitMaxsplitOne(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = string.split("a,b,c", ",", 1)
		assert(#r == 2)
		assert(r[1] == "a")
		assert(r[2] == "b,c")
	`)
}

func TestStringSplitNoMatch(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = string.split("hello", ",")
		assert(#r == 1)
		assert(r[1] == "hello")
	`)
}

func TestStringSplitSingleChar(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = string.split("a", ",")
		assert(#r == 1)
		assert(r[1] == "a")
	`)
}

func TestStringSplitSpace(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = string.split("hello world foo", " ")
		assert(#r == 3)
		assert(r[1] == "hello")
		assert(r[2] == "world")
		assert(r[3] == "foo")
	`)
}

func TestStringSplitMultiCharSep(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = string.split("a::b::c", "::")
		assert(#r == 3)
		assert(r[1] == "a")
		assert(r[2] == "b")
		assert(r[3] == "c")
	`)
}

func TestStringSplitMethodSyntax(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = ("a,b,c"):split(",")
		assert(#r == 3)
		assert(r[1] == "a")
		assert(r[2] == "b")
		assert(r[3] == "c")
	`)
}

func TestStringSplitEmptyString(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = string.split("", ",")
		assert(#r == 1)
		assert(r[1] == "")
	`)
}

// ===== STRING TRIM =====

func TestStringTrimWhitespace(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.trim("  hello  ") == "hello")
	`)
}

func TestStringTrimTabs(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.trim("\t\nhello\n\t") == "hello")
	`)
}

func TestStringTrimCustomChars(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.trim("--hello--", "-") == "hello")
	`)
}

func TestStringTrimCustomMultiChars(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.trim("xyhelloyx", "xy") == "hello")
	`)
}

func TestStringTrimNoChange(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.trim("hello") == "hello")
	`)
}

func TestStringTrimEmpty(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.trim("") == "")
	`)
}

func TestStringTrimMethodSyntax(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(("  hello  "):trim() == "hello")
	`)
}

// ===== STRING LTRIM =====

func TestStringLtrimWhitespace(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.ltrim("  hello  ") == "hello  ")
	`)
}

func TestStringLtrimCustomChars(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.ltrim("xxhello", "x") == "hello")
	`)
}

func TestStringLtrimNoChange(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.ltrim("hello  ") == "hello  ")
	`)
}

func TestStringLtrimMethodSyntax(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(("  hello"):ltrim() == "hello")
	`)
}

// ===== STRING RTRIM =====

func TestStringRtrimWhitespace(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.rtrim("  hello  ") == "  hello")
	`)
}

func TestStringRtrimCustomChars(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.rtrim("helloxx", "x") == "hello")
	`)
}

func TestStringRtrimNoChange(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.rtrim("  hello") == "  hello")
	`)
}

func TestStringRtrimMethodSyntax(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(("hello  "):rtrim() == "hello")
	`)
}

// ===== STRING STARTSWITH =====

func TestStringStartswithTrue(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.startswith("hello world", "hello") == true)
	`)
}

func TestStringStartswithFalse(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.startswith("hello world", "world") == false)
	`)
}

func TestStringStartswithEmptyPrefix(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.startswith("hello", "") == true)
	`)
}

func TestStringStartswithFullMatch(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.startswith("hello", "hello") == true)
	`)
}

func TestStringStartswithLongerPrefix(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.startswith("hi", "hello") == false)
	`)
}

func TestStringStartswithMethodSyntax(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(("hello"):startswith("he") == true)
	`)
}

// ===== STRING ENDSWITH =====

func TestStringEndswithTrue(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.endswith("hello world", "world") == true)
	`)
}

func TestStringEndswithFalse(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.endswith("hello world", "hello") == false)
	`)
}

func TestStringEndswithEmptySuffix(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.endswith("hello", "") == true)
	`)
}

func TestStringEndswithFullMatch(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.endswith("hello", "hello") == true)
	`)
}

func TestStringEndswithLongerSuffix(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(string.endswith("hi", "hello") == false)
	`)
}

func TestStringEndswithMethodSyntax(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(("hello"):endswith("lo") == true)
	`)
}
