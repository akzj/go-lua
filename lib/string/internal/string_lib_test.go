// Package internal provides tests for the string library.
package internal

import (
	"testing"

	stringlib "github.com/akzj/go-lua/lib/string/api"
)

func TestNewStringLib(t *testing.T) {
	lib := NewStringLib()
	if lib == nil {
		t.Error("NewStringLib() returned nil")
	}
	if _, ok := interface{}(lib).(stringlib.StringLib); !ok {
		t.Error("StringLib does not implement stringlib.StringLib interface")
	}
}
