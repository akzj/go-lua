package ltm

import (
	"testing"

	"github.com/akzj/go-lua/internal/lobject"
)

func TestInit(t *testing.T) {
	L := &lobject.LuaState{}
	
	// Init should not panic
	Init(L)
}

func TestTypeNames(t *testing.T) {
	tests := []struct {
		idx   int
		name  string
	}{
		{0, "no value"},
		{1, "nil"},
		{2, "boolean"},
		{4, "number"},
		{5, "string"},
		{6, "table"},
		{7, "function"},
	}
	
	for _, tt := range tests {
		if TypeNames[tt.idx] != tt.name {
			t.Errorf("TypeNames[%d] = %q, want %q", tt.idx, TypeNames[tt.idx], tt.name)
		}
	}
}

func TestGetTM(t *testing.T) {
	// GetTM with nil table should return nil
	tm := GetTM(nil, lobject.TM_INDEX, nil)
	if tm != nil {
		t.Error("GetTM with nil table should return nil")
	}
}

func TestGetTMByObj(t *testing.T) {
	L := &lobject.LuaState{}
	
	// GetTMByObj with nil object
	tm := GetTMByObj(L, nil, lobject.TM_INDEX)
	if tm != nil {
		t.Error("GetTMByObj with nil object should return nil")
	}
}
