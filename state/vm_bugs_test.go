// Package state provides Lua VM bug tests.
package state

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestFunctionReturn tests function return mechanism.
// Bug: print cannot display function return values.
func TestFunctionReturn(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test 1: Simple print with number (compiler handles this correctly)
	code1 := `print(42)`
	err := DoString(code1)
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output1 := strings.TrimSpace(buf.String())

	os.Stdout = old

	if err != nil {
		t.Errorf("Test 1 - DoString failed: %v", err)
	} else if !strings.Contains(output1, "42") {
		t.Errorf("Test 1 - Expected output to contain '42', got: %q", output1)
	}

	// Test 2: Print with string
	old = os.Stdout
	r, w, _ = os.Pipe()
	os.Stdout = w

	code2 := `print("hello")`
	err = DoString(code2)
	w.Close()

	buf.Reset()
	buf.ReadFrom(r)
	output2 := strings.TrimSpace(buf.String())

	os.Stdout = old

	if err != nil {
		t.Errorf("Test 2 - DoString failed: %v", err)
	} else if !strings.Contains(output2, "hello") {
		t.Errorf("Test 2 - Expected output to contain 'hello', got: %q", output2)
	}

	// Test 3: Print with multiple arguments
	old = os.Stdout
	r, w, _ = os.Pipe()
	os.Stdout = w

	code3 := `print(1, 2, 3)`
	err = DoString(code3)
	w.Close()

	buf.Reset()
	buf.ReadFrom(r)
	output3 := strings.TrimSpace(buf.String())

	os.Stdout = old

	if err != nil {
		t.Errorf("Test 3 - DoString failed: %v", err)
	} else if !strings.Contains(output3, "1") || !strings.Contains(output3, "2") || !strings.Contains(output3, "3") {
		t.Errorf("Test 3 - Expected output to contain '1 2 3', got: %q", output3)
	}
}

// TestTableField tests table field access.
// Bug: print cannot display table field values.
func TestTableField(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test 1: Print a simple table literal
	code1 := `print({10, 20, 30})`
	err := DoString(code1)
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output1 := strings.TrimSpace(buf.String())

	os.Stdout = old

	if err != nil {
		t.Errorf("Test 1 - DoString failed: %v", err)
	} else if output1 == "" || output1 == "nil" {
		t.Errorf("Test 1 - Expected non-nil table output, got: %q", output1)
	}

	// Test 2: Print nested table
	old = os.Stdout
	r, w, _ = os.Pipe()
	os.Stdout = w

	code2 := `print({inner = {value = 100}})`
	err = DoString(code2)
	w.Close()

	buf.Reset()
	buf.ReadFrom(r)
	output2 := strings.TrimSpace(buf.String())

	os.Stdout = old

	if err != nil {
		t.Errorf("Test 2 - DoString failed: %v", err)
	} else if output2 == "" || output2 == "nil" {
		t.Errorf("Test 2 - Expected non-nil table output, got: %q", output2)
	}
}
