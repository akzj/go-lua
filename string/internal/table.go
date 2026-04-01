package internal

import (
	"bytes"
	"math"
	"unsafe"

	stringapi "github.com/akzj/go-lua/string/api"
)

const MinStringTableSize = 128
const MaxStringTableSize = math.MaxInt32

type stringTable struct {
	hash []*stringapi.TStringImpl
	size int
	nuse int
	seed uint32
}

func NewStringTable() stringapi.StringTable {
	return &stringTable{
		hash: make([]*stringapi.TStringImpl, MinStringTableSize),
		size: MinStringTableSize,
		seed: generateSeed(),
	}
}

func generateSeed() uint32 {
	var localAddr uintptr
	addr := uintptr(unsafe.Pointer(&localAddr))
	seed := uint32(addr)
	seed ^= uint32(addr >> 32)
	seed ^= uint32(addr >> 16)
	seed ^= uint32(addr >> 8)
	if seed == 0 {
		seed = 1
	}
	return seed
}

func (st *stringTable) hashString(data []byte, seed uint32) uint32 {
	h := seed ^ uint32(len(data))
	for i := len(data) - 1; i >= 0; i-- {
		h ^= uint32(data[i])
		h *= 5
		h ^= h >> 2
	}
	return h
}

func (st *stringTable) lmod(h uint32, size int) int {
	return int(h) & (size - 1)
}

func (st *stringTable) NewString(s string) *stringapi.TStringImpl {
	data := []byte(s)
	if len(data) <= stringapi.MaxShortStringLen {
		return st.internShort(data)
	}
	return st.newLongString(data)
}

func (st *stringTable) internShort(data []byte) *stringapi.TStringImpl {
	h := st.hashString(data, st.seed)
	bucket := &st.hash[st.lmod(h, st.size)]

	// Search for existing string - compare only the actual content
	for ts := *bucket; ts != nil; ts = ts.Hnext {
		if ts.IsShort() && int(ts.Shrlen) == len(data) {
			// Compare only len(data) bytes (excluding null terminator)
			if bytes.Equal(ts.Data[:len(data)], data) {
				return ts
			}
		}
	}

	if st.nuse >= st.size {
		st.grow()
		bucket = &st.hash[st.lmod(h, st.size)]
	}

	ts := st.createShortString(data, h)
	ts.Hnext = *bucket
	*bucket = ts
	st.nuse++
	return ts
}

func (st *stringTable) createShortString(data []byte, hash uint32) *stringapi.TStringImpl {
	ts := &stringapi.TStringImpl{
		Next:   nil,
		TT:     stringapi.LUA_VSHRSTR,
		Marked: 0,
		Extra:  0,
		Shrlen: int8(len(data)),
		Hash:   hash,
		Hnext:  nil,
		Data:   make([]byte, len(data)+1),
	}
	copy(ts.Data, data)
	return ts
}

func (st *stringTable) newLongString(data []byte) *stringapi.TStringImpl {
	ts := &stringapi.TStringImpl{
		Next:   nil,
		TT:     stringapi.LUA_VLNGSTR,
		Marked: 0,
		Extra:  0,
		Shrlen: -1,
		Hash:   st.hashString(data, st.seed),
		Hnext:  nil,
		Data:   make([]byte, len(data)),
	}
	copy(ts.Data, data)
	return ts
}

func (st *stringTable) grow() {
	oldSize := st.size
	newSize := oldSize * 2
	if newSize > MaxStringTableSize {
		newSize = MaxStringTableSize
	}
	newHash := make([]*stringapi.TStringImpl, newSize)
	for i := 0; i < oldSize; i++ {
		ts := st.hash[i]
		st.hash[i] = nil
		for ts != nil {
			next := ts.Hnext
			h := st.lmod(ts.Hash, newSize)
			ts.Hnext = newHash[h]
			newHash[h] = ts
			ts = next
		}
	}
	st.hash = newHash
	st.size = newSize
}

func (st *stringTable) Interned(s string) bool {
	data := []byte(s)
	if len(data) > stringapi.MaxShortStringLen {
		return false
	}
	h := st.hashString(data, st.seed)
	bucket := &st.hash[st.lmod(h, st.size)]
	for ts := *bucket; ts != nil; ts = ts.Hnext {
		if ts.IsShort() && int(ts.Shrlen) == len(data) {
			// Compare only len(data) bytes (excluding null terminator)
			if bytes.Equal(ts.Data[:len(data)], data) {
				return true
			}
		}
	}
	return false
}

func (st *stringTable) GetString(s string) *stringapi.TStringImpl {
	data := []byte(s)
	if len(data) > stringapi.MaxShortStringLen {
		return nil
	}
	h := st.hashString(data, st.seed)
	bucket := &st.hash[st.lmod(h, st.size)]
	for ts := *bucket; ts != nil; ts = ts.Hnext {
		if ts.IsShort() && int(ts.Shrlen) == len(data) {
			if bytes.Equal(ts.Data[:len(data)], data) {
				return ts
			}
		}
	}
	return nil
}

func EqualStrings(a, b *stringapi.TStringImpl) bool {
	if a.Len() != b.Len() {
		return false
	}
	return bytes.Equal(a.Data[:a.Len()], b.Data[:b.Len()])
}

var reservedWords = map[string]uint8{
	"and": 1, "break": 2, "do": 3, "else": 4, "elseif": 5, "end": 6,
	"false": 7, "for": 8, "function": 9, "goto": 10, "if": 11, "in": 12,
	"local": 13, "nil": 14, "not": 15, "or": 16, "repeat": 17, "return": 18,
	"then": 19, "true": 20, "until": 21, "while": 22,
}

func (st *stringTable) MarkReservedWord(ts *stringapi.TStringImpl) {
	if ts.IsShort() {
		ts.Extra = 1
	}
}

func IsReservedWord(s string) bool {
	_, ok := reservedWords[s]
	return ok
}

var _ stringapi.StringTable = (*stringTable)(nil)
