package lobject

import (
	"fmt"
	"unsafe"
)

/*
** TString: String type
 */
type TString struct {
	CommonHeader
	Extra    LuByte   // reserved words for short strings; "has hash" for longs
	Shrlen   LsByte   // length for short strings, negative for long strings
	Hash     uint32   // hash value
	U        TStringU // union for lnglen or hnext
	Contents *byte    // pointer to content (for long strings)
	Falloc   LuaAlloc // deallocation function for external strings
	Ud       interface{}
}

type TStringU struct {
	Lnglen uint64 // length for long strings
}

/*
** Check if string is short
 */
func IsShrStr(ts *TString) bool {
	return ts.Shrlen >= 0
}

/*
** Get short string length
 */
func ShrLen(ts *TString) int {
	return int(ts.Shrlen)
}

/*
** Get long string length
 */
func LngLen(ts *TString) uint64 {
	return ts.U.Lnglen
}

/*
** Get string length
 */
func StrLen(ts *TString) int {
	if IsShrStr(ts) {
		return ShrLen(ts)
	}
	return int(LngLen(ts))
}

/*
** Compare two strings by content (for use in table key comparison)
** Returns true if both strings have the same content
 */
func TestStringValue(ts1, ts2 *TString) bool {
	fmt.Printf("DEBUG TestStringValue: ts1=%p, ts2=%p\n", ts1, ts2)
	if ts1 == ts2 {
		fmt.Println("DEBUG TestStringValue: same pointer, return true")
		return true
	}
	len1 := StrLen(ts1)
	len2 := StrLen(ts2)
	fmt.Printf("DEBUG TestStringValue: len1=%d, len2=%d\n", len1, len2)
	if len1 != len2 {
		return false
	}
	// Compare byte content
	for i := 0; i < len1; i++ {
		c1 := *(*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(ts1.Contents)) + uintptr(i)))
		c2 := *(*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(ts2.Contents)) + uintptr(i)))
		if c1 != c2 {
			fmt.Printf("DEBUG TestStringValue: mismatch at %d: %c vs %c\n", i, c1, c2)
			return false
		}
	}
	fmt.Println("DEBUG TestStringValue: strings match!")
	return true
}

/*
** Check if string is external
 */
func IsExtStr(ts *TString) bool {
	// A string is external if it's a long string with kind != LSTRREG
	return !IsShrStr(ts) && int(ts.Shrlen) != LSTRREG
}

/*
** String kind (LSTRREG, LSTRFIX, LSTRMEM)
 */
func StrKind(ts *TString) int {
	return int(ts.Shrlen)
}
