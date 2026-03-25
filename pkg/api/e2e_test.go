package api

import (
	"bytes"
	"os"
	"testing"
)

// helper to run DoString and get numeric result
func doStringNumber(t *testing.T, code string) float64 {
	t.Helper()
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString(%q) failed: %v", code, err)
	}

	result, ok := L.ToNumber(-1)
	if !ok {
		t.Fatalf("DoString(%q) result is not a number", code)
	}
	return result
}

// helper to run DoString and get string result
func doStringString(t *testing.T, code string) string {
	t.Helper()
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString(%q) failed: %v", code, err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatalf("DoString(%q) result is not a string", code)
	}
	return result
}

// helper to capture stdout during DoString
func doStringCapture(t *testing.T, code string) string {
	t.Helper()

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	L := NewState()
	L.OpenLibs()

	err := L.DoString(code, "test")

	w.Close()
	os.Stdout = old
	L.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	r.Close()

	if err != nil {
		t.Fatalf("DoString(%q) failed: %v", code, err)
	}

	return buf.String()
}

func TestE2E_ReturnArithmetic(t *testing.T) {
	result := doStringNumber(t, "return 1 + 2")
	if result != 3.0 {
		t.Errorf("Expected 3.0, got %f", result)
	}
}

func TestE2E_LocalVariable(t *testing.T) {
	result := doStringNumber(t, "local x = 10; return x * 2")
	if result != 20.0 {
		t.Errorf("Expected 20.0, got %f", result)
	}
}

func TestE2E_PrintHello(t *testing.T) {
	output := doStringCapture(t, `print("Hello")`)
	if output != "Hello\n" {
		t.Errorf("Expected 'Hello\\n', got %q", output)
	}
}

func TestE2E_IfElse(t *testing.T) {
	result := doStringString(t, `local x = 10; if x > 5 then return "big" else return "small" end`)
	if result != "big" {
		t.Errorf("Expected 'big', got %q", result)
	}
}

func TestE2E_ForLoop(t *testing.T) {
	result := doStringNumber(t, "local s = 0; for i = 1, 10 do s = s + i end; return s")
	if result != 55.0 {
		t.Errorf("Expected 55.0, got %f", result)
	}
}

func TestE2E_TableLength(t *testing.T) {
	result := doStringNumber(t, "local t = {1, 2, 3}; return #t")
	if result != 3.0 {
		t.Errorf("Expected 3.0, got %f", result)
	}
}

func TestE2E_FunctionCall(t *testing.T) {
	result := doStringNumber(t, "local function add(a, b) return a + b end; return add(3, 4)")
	if result != 7.0 {
		t.Errorf("Expected 7.0, got %f", result)
	}
}

func TestE2E_PrintType(t *testing.T) {
	output := doStringCapture(t, "print(type(42))")
	if output != "number\n" {
		t.Errorf("Expected 'number\\n', got %q", output)
	}
}

func TestE2E_Assert(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	err := L.DoString("assert(1 + 1 == 2)", "test")
	if err != nil {
		t.Errorf("assert(1 + 1 == 2) should not error, got: %v", err)
	}
}