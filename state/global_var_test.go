package state

import (
	"testing"
)

func TestGlobalVariableRead(t *testing.T) {
	// Simple test: set x and print it
	err := DoString(`x = 42`)
	if err != nil {
		t.Errorf("Error setting x: %v", err)
	}
	// Now read it
	err = DoString(`print("x =", x)`)
	if err != nil {
		t.Errorf("Error printing x: %v", err)
	}
}
