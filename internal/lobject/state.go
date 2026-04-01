package lobject

/*
** String table and other state types
*/

import "unsafe"

// StringTable for interning strings
type StringTable struct {
	Hash []unsafe.Pointer // array of buckets
	Nuse int              // number of elements
	Size int              // number of buckets
}
