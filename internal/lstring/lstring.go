package lstring

import (
	"bytes"
	"unsafe"

	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** Memory-allocation error message must be preallocated
 */
const MEMERRMSG = "not enough memory"

/*
** Minimum size for the String table (must be power of 2)
 */
const MINSTRTABSIZE = 128

/*
** Maximum size for String table
 */
const MAXSTRTB = 1<<31 - 1

/*
** String interning cache - module-level map for string deduplication
** This prevents creating duplicate TString objects for the same string content
 */
var stringCache = make(map[string]*lobject.TString)

/*
** String equality
 */
func EqStr(a, b *lobject.TString) bool {
	alen := StrLen(a)
	blen := StrLen(b)
	if alen != blen {
		return false
	}
	ab := GetStr(a)
	bb := GetStr(b)
	return bytes.Equal(ab, bb)
}

func StrLen(ts *lobject.TString) int {
	return lobject.StrLen(ts)
}

/*
** Get string contents
 */
func GetStr(ts *lobject.TString) []byte {
	if lobject.IsShrStr(ts) {
		return getShrStr(ts)
	}
	return getLngStr(ts)
}

func getShrStr(ts *lobject.TString) []byte {
	// Use unsafe to get string data from Contents
	if ts.Contents == nil {
		return nil
	}
	length := int(ts.Shrlen)
	return unsafe.Slice((*byte)(unsafe.Pointer(ts.Contents)), length)
}

func getLngStr(ts *lobject.TString) []byte {
	if ts.Contents == nil {
		return nil
	}
	length := int(ts.U.Lnglen)
	return unsafe.Slice((*byte)(unsafe.Pointer(ts.Contents)), length)
}

/*
** Hash function for strings
 */
func HashString(str []byte, seed uint32) uint32 {
	h := seed ^ uint32(len(str))
	for _, c := range str {
		h ^= uint32(c)
		h *= 33
	}
	return h
}

/*
** Hash long string
 */
func HashLongStr(ts *lobject.TString) uint32 {
	if ts.Extra == 0 {
		ts.Hash = HashString(getLngStr(ts), ts.Hash)
		ts.Extra = 1
	}
	return ts.Hash
}

/*
** Check if string is reserved word
 */
func IsReserved(ts *lobject.TString) bool {
	return lobject.IsShrStr(ts) && ts.Extra > 0
}

/*
** Create a new short string with proper interning
** FIXED: Store bytes in a module-level cache to prevent dangling pointers
**        and ensure string deduplication
 */
func NewString(L *lstate.LuaState, s string) *lobject.TString {
	// Check if string already exists in cache (string interning)
	if existing, ok := stringCache[s]; ok {
		return existing
	}

	ts := &lobject.TString{}
	ts.Extra = 0
	ts.Hash = HashString([]byte(s), 0)
	ts.Shrlen = int8(len(s))
	ts.U.Lnglen = 0

	// Store string data - allocate on heap (not stack) to prevent dangling pointer
	// The Data slice is stored as a field on TString or kept alive by the cache
	if len(s) > 0 {
		data := []byte(s) // This allocates on heap, data variable keeps the slice alive
		// Pin the slice to prevent GC - store reference in module-level cache
		ts.Contents = &data[0]
		// Keep data alive - the cache holds the reference
		stringCache[s] = ts
		// Also need to keep the actual byte slice alive - use a separate map
		keepAlive(s, data)
	}
	return ts
}

// Module-level map to keep byte slices alive for string interning
var byteSliceCache = make(map[string][]byte)

func keepAlive(s string, data []byte) {
	byteSliceCache[s] = data
}

/*
** Create a new long string
** FIXED: Same approach as NewString - use heap-allocated data
 */
func NewLongString(L *lstate.LuaState, s string) *lobject.TString {
	// Long strings are typically not interned, but we still need proper allocation
	ts := &lobject.TString{}
	ts.Extra = 0
	ts.Shrlen = -1 // marks it as long string
	ts.U.Lnglen = uint64(len(s))
	ts.Hash = HashString([]byte(s), 0)
	if len(s) > 0 {
		data := []byte(s) // heap allocated
		ts.Contents = &data[0]
	}
	return ts
}
