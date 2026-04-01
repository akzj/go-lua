package internal

import (
	"testing"
	api "github.com/akzj/go-lua/string/api"
)

func TestTStringImpl_ShortString(t *testing.T) {
	ts := &api.TStringImpl{
		TT:     api.LUA_VSHRSTR,
		Shrlen: 5,
		Data:   []byte("hello"),
	}

	if !ts.IsShort() {
		t.Error("expected IsShort() = true for short string")
	}
	if ts.Len() != 5 {
		t.Errorf("expected Len() = 5, got %d", ts.Len())
	}
}

func TestTStringImpl_LongString(t *testing.T) {
	ts := &api.TStringImpl{
		TT:     api.LUA_VLNGSTR,
		Shrlen: -1,
		Data:   []byte("this is a long string with more than 40 characters"),
	}

	if ts.IsShort() {
		t.Error("expected IsShort() = false for long string")
	}
	if ts.Len() != len(ts.Data) {
		t.Errorf("expected Len() = %d, got %d", len(ts.Data), ts.Len())
	}
}

func TestMaxShortStringLen(t *testing.T) {
	if api.MaxShortStringLen != 40 {
		t.Errorf("expected MaxShortStringLen = 40, got %d", api.MaxShortStringLen)
	}
}

func TestStringVariants(t *testing.T) {
	// LUA_VSHRSTR = LUA_TSTRING | (0 << 4) = 4
	// LUA_VLNGSTR = LUA_TSTRING | (1 << 4) = 4 | 16 = 20
	if api.LUA_VSHRSTR != 4 {
		t.Errorf("expected LUA_VSHRSTR = 4, got %d", api.LUA_VSHRSTR)
	}
	if api.LUA_VLNGSTR != 20 {
		t.Errorf("expected LUA_VLNGSTR = 20, got %d", api.LUA_VLNGSTR)
	}
}
