// Package internal provides tests for the base library.
package internal

import (
	"testing"

	baselib "github.com/akzj/go-lua/lib/base/api"
)

func TestNewBaseLib(t *testing.T) {
	lib := NewBaseLib()
	if lib == nil {
		t.Error("NewBaseLib() returned nil")
	}
	if _, ok := interface{}(lib).(baselib.BaseLib); !ok {
		t.Error("BaseLib does not implement baselib.BaseLib interface")
	}
}
