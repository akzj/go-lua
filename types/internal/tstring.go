// Package internal provides concrete implementations of api interfaces.
package internal

import (
	"github.com/akzj/go-lua/types/api"
)

// TString is the concrete TString implementation.
type TString struct {
	GCObject
	Extra      uint8
	Shrlen     int8
	HashCache  uint32
	Lnglen     uint32
	Hnext      *TString
	Contents_  []byte
}

func (s *TString) IsShort() bool { return s.Shrlen >= 0 }
func (s *TString) Len() int {
	if s.IsShort() {
		return int(s.Shrlen)
	}
	return int(s.Lnglen)
}
func (s *TString) Hash() uint32 { return s.HashCache }

func NewString(contents string) api.TString {
	b := []byte(contents)
	if len(b) <= 40 {
		return &TString{
			GCObject:  GCObject{Tt: uint8(api.Ctb(int(api.LUA_VSHRSTR)))},
			Shrlen:    int8(len(b)),
			HashCache: hashString(b),
		}
	}
	return &TString{
		GCObject:  GCObject{Tt: uint8(api.Ctb(int(api.LUA_VLNGSTR)))},
		Shrlen:    -1,
		Lnglen:    uint32(len(b)),
		HashCache: hashString(b),
		Contents_: b,
	}
}

// Simple hash for strings
func hashString(b []byte) uint32 {
	var h uint32 = 0
	for _, c := range b {
		h = h*31 + uint32(c)
	}
	return h
}
