package lobject

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
