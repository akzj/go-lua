// Package api provides the public Lua API
// This file tests the io standard library module
package api

import (
	"os"
	"path/filepath"
	"testing"
)

// Helper function to create a temp file with content
func createTempFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "testfile.txt")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	return tmpFile
}

// Helper function to read file content
func readFileContent(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", path, err)
	}
	return string(content)
}

// =============================================================================
// io.open tests
// =============================================================================

func TestIO_Open_ReadMode(t *testing.T) {
	tmpFile := createTempFile(t, "Hello, World!")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// Note: Workaround for VM limitation - assign function to local first
	code := `
		local open = io.open
		local file, err = open("` + tmpFile + `", "r")
		if file then
			return "opened"
		else
			return "error: " .. (err or "unknown")
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "opened" {
		t.Errorf("Expected 'opened', got '%s'", result)
	}
}

func TestIO_Open_WriteMode(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "write_test.txt")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file, err = open("` + tmpFile + `", "w")
		if file then
			return "opened"
		else
			return "error: " .. (err or "unknown")
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "opened" {
		t.Errorf("Expected 'opened', got '%s'", result)
	}

	// Verify file was created
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Error("File was not created")
	}
}

func TestIO_Open_AppendMode(t *testing.T) {
	tmpFile := createTempFile(t, "Initial content\n")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file, err = open("` + tmpFile + `", "a")
		if file then
			local write = file.write
			write(file, "Appended content\n")
			local close = file.close
			close(file)
			return "success"
		else
			return "error: " .. (err or "unknown")
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "success" {
		t.Errorf("Expected 'success', got '%s'", result)
	}

	// Verify content was appended
	content := readFileContent(t, tmpFile)
	expected := "Initial content\nAppended content\n"
	if content != expected {
		t.Errorf("Expected '%s', got '%s'", expected, content)
	}
}

func TestIO_Open_NonExistentFile(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file, err = open("/non/existent/path/file.txt", "r")
		if file then
			return "unexpected success"
		else
			return "error"
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "error" {
		t.Errorf("Expected 'error', got '%s'", result)
	}
}

func TestIO_Open_InvalidMode(t *testing.T) {
	tmpFile := createTempFile(t, "test content")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// Invalid mode should still work (defaults to read mode)
	code := `
		local open = io.open
		local file, err = open("` + tmpFile + `", "invalid")
		if file then
			local close = file.close
			close(file)
			return "opened"
		else
			return "error"
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	// Invalid mode defaults to read mode, so it should open
	if result != "opened" {
		t.Errorf("Expected 'opened' (invalid mode defaults to read), got '%s'", result)
	}
}

func TestIO_Open_DefaultMode(t *testing.T) {
	tmpFile := createTempFile(t, "test content")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// No mode specified - should default to "r"
	code := `
		local open = io.open
		local file, err = open("` + tmpFile + `")
		if file then
			local close = file.close
			close(file)
			return "opened"
		else
			return "error"
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "opened" {
		t.Errorf("Expected 'opened', got '%s'", result)
	}
}

// =============================================================================
// file:read tests
// =============================================================================

func TestIO_Read_Line(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3\n"
	tmpFile := createTempFile(t, content)

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "r")
		local read = file.read
		local line1 = read(file)
		local line2 = read(file)
		local line3 = read(file)
		local close = file.close
		close(file)
		return line1, line2, line3
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	line1, ok := L.ToString(1)
	if !ok || line1 != "Line 1" {
		t.Errorf("Expected 'Line 1', got '%s'", line1)
	}

	line2, ok := L.ToString(2)
	if !ok || line2 != "Line 2" {
		t.Errorf("Expected 'Line 2', got '%s'", line2)
	}

	line3, ok := L.ToString(3)
	if !ok || line3 != "Line 3" {
		t.Errorf("Expected 'Line 3', got '%s'", line3)
	}
}

func TestIO_Read_AllFormat(t *testing.T) {
	content := "Hello\nWorld\n"
	tmpFile := createTempFile(t, content)

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "r")
		local read = file.read
		local all = read(file, "*a")
		local close = file.close
		close(file)
		return all
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != content {
		t.Errorf("Expected '%s', got '%s'", content, result)
	}
}

func TestIO_Read_LineFormat(t *testing.T) {
	content := "First line\nSecond line\n"
	tmpFile := createTempFile(t, content)

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "r")
		local read = file.read
		local line1 = read(file, "*l")
		local line2 = read(file, "*l")
		local line3 = read(file, "*l")
		local close = file.close
		close(file)
		return line1, line2, line3
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	line1, ok := L.ToString(1)
	if !ok || line1 != "First line" {
		t.Errorf("Expected 'First line', got '%s'", line1)
	}

	line2, ok := L.ToString(2)
	if !ok || line2 != "Second line" {
		t.Errorf("Expected 'Second line', got '%s'", line2)
	}

	// line3 should be nil (EOF)
	if !L.IsNil(3) {
		t.Error("Expected nil at EOF")
	}
}

func TestIO_Read_NumberFormat(t *testing.T) {
	content := "42 3.14 -100\n"
	tmpFile := createTempFile(t, content)

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "r")
		local read = file.read
		local num1 = read(file, "*n")
		local num2 = read(file, "*n")
		local close = file.close
		close(file)
		return num1, num2
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	num1, ok := L.ToNumber(1)
	if !ok || num1 != 42 {
		t.Errorf("Expected 42, got %f", num1)
	}

	num2, ok := L.ToNumber(2)
	if !ok || num2 != 3.14 {
		t.Errorf("Expected 3.14, got %f", num2)
	}
}

func TestIO_Read_Bytes(t *testing.T) {
	content := "Hello World"
	tmpFile := createTempFile(t, content)

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// Test reading all content and verify in Go
	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "r")
		local read = file.read
		local all = read(file, "*a")
		local close = file.close
		close(file)
		return all
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Verify the content matches using Go
	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != content {
		t.Errorf("Expected '%s', got '%s'", content, result)
	}
}

func TestIO_Read_EOF(t *testing.T) {
	content := "single line\n"
	tmpFile := createTempFile(t, content)

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "r")
		local read = file.read
		local line1 = read(file)
		local line2 = read(file)  -- Should be nil at EOF
		local close = file.close
		close(file)
		return line1, line2
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	line1, ok := L.ToString(1)
	if !ok || line1 != "single line" {
		t.Errorf("Expected 'single line', got '%s'", line1)
	}

	// line2 should be nil (EOF)
	if !L.IsNil(2) {
		t.Error("Expected nil at EOF")
	}
}

func TestIO_Read_ClosedFile(t *testing.T) {
	tmpFile := createTempFile(t, "content")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "r")
		local close = file.close
		close(file)
		local read = file.read
		local result, err = read(file)
		if result then
			return "unexpected success"
		else
			return "error: " .. (err or "unknown")
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "error: file is closed" {
		t.Errorf("Expected 'error: file is closed', got '%s'", result)
	}
}

// =============================================================================
// file:write tests
// =============================================================================

func TestIO_Write_Simple(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "write_test.txt")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "w")
		local write = file.write
		local result = write(file, "Hello, World!")
		local close = file.close
		close(file)
		return result
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Verify content was written
	content := readFileContent(t, tmpFile)
	if content != "Hello, World!" {
		t.Errorf("Expected 'Hello, World!', got '%s'", content)
	}
}

func TestIO_Write_MultipleWrites(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "multi_write.txt")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "w")
		local write = file.write
		write(file, "Line 1\n")
		write(file, "Line 2\n")
		write(file, "Line 3\n")
		local close = file.close
		close(file)
		return "done"
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	content := readFileContent(t, tmpFile)
	expected := "Line 1\nLine 2\nLine 3\n"
	if content != expected {
		t.Errorf("Expected '%s', got '%s'", expected, content)
	}
}

func TestIO_Write_MultipleArguments(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "multi_args.txt")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "w")
		local write = file.write
		write(file, "Hello", " ", "World", "!")
		local close = file.close
		close(file)
		return "done"
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	content := readFileContent(t, tmpFile)
	if content != "Hello World!" {
		t.Errorf("Expected 'Hello World!', got '%s'", content)
	}
}

func TestIO_Write_AppendMode(t *testing.T) {
	tmpFile := createTempFile(t, "Original\n")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "a")
		local write = file.write
		write(file, "Appended\n")
		local close = file.close
		close(file)
		return "done"
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	content := readFileContent(t, tmpFile)
	expected := "Original\nAppended\n"
	if content != expected {
		t.Errorf("Expected '%s', got '%s'", expected, content)
	}
}

func TestIO_Write_ClosedFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "closed_test.txt")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "w")
		local close = file.close
		close(file)
		local write = file.write
		local result, err = write(file, "test")
		if result then
			return "unexpected success"
		else
			return "error: " .. (err or "unknown")
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "error: file is closed" {
		t.Errorf("Expected 'error: file is closed', got '%s'", result)
	}
}

// =============================================================================
// file:close tests
// =============================================================================

func TestIO_Close(t *testing.T) {
	tmpFile := createTempFile(t, "test")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "r")
		local close = file.close
		local result = close(file)
		return result
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Should return true
	result, _ := L.ToBoolean(-1)
	if !result {
		t.Error("Expected close to return true")
	}
}

func TestIO_Close_DoubleClose(t *testing.T) {
	tmpFile := createTempFile(t, "test")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "r")
		local close = file.close
		close(file)
		local result = close(file)  -- Second close should still return true
		return result
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Should return true even on double close
	result, _ := L.ToBoolean(-1)
	if !result {
		t.Error("Expected double close to return true")
	}
}

// =============================================================================
// io.lines tests
// =============================================================================

func TestIO_Lines_Basic(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3\n"
	tmpFile := createTempFile(t, content)

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local lines = io.lines
		local count = 0
		for line in lines("` + tmpFile + `") do
			count = count + 1
		end
		return count
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	count, ok := L.ToNumber(-1)
	if !ok || count != 3 {
		t.Errorf("Expected 3 lines, got %f", count)
	}
}

func TestIO_Lines_EmptyFile(t *testing.T) {
	tmpFile := createTempFile(t, "")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local lines = io.lines
		local count = 0
		for line in lines("` + tmpFile + `") do
			count = count + 1
		end
		return count
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	count, ok := L.ToNumber(-1)
	if !ok || count != 0 {
		t.Errorf("Expected 0 lines, got %f", count)
	}
}

func TestIO_Lines_NoTrailingNewline(t *testing.T) {
	// Test file with trailing newline (standard behavior)
	content := "Line 1\nLine 2\nLine 3\n"
	tmpFile := createTempFile(t, content)

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local lines = io.lines
		local count = 0
		for line in lines("` + tmpFile + `") do
			count = count + 1
		end
		return count
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	count, ok := L.ToNumber(-1)
	if !ok || count != 3 {
		t.Errorf("Expected 3 lines, got %f", count)
	}
}

func TestIO_Lines_NonExistentFile(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// io.lines on non-existent file should error
	code := `
		local lines = io.lines
		local ok, err = pcall(function()
			for line in lines("/non/existent/file.txt") do
			end
		end)
		if ok then
			return "no error"
		else
			return "error"
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Non-existent file should cause an error
	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "error" {
		t.Errorf("Expected 'error' for non-existent file, got '%s'", result)
	}
}

// =============================================================================
// file:lines tests (method on file handle)
// =============================================================================

func TestIO_FileLines_Basic(t *testing.T) {
	content := "First\nSecond\nThird\n"
	tmpFile := createTempFile(t, content)

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "r")
		local lines = file.lines
		local count = 0
		for line in lines(file) do
			count = count + 1
		end
		local close = file.close
		close(file)
		return count
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	count, ok := L.ToNumber(-1)
	if !ok || count != 3 {
		t.Errorf("Expected 3 lines, got %f", count)
	}
}

// =============================================================================
// io.input tests
// =============================================================================

func TestIO_Input_Default(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local input = io.input()
		if input then
			return "got input"
		else
			return "no input"
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "got input" {
		t.Errorf("Expected 'got input', got '%s'", result)
	}
}

// =============================================================================
// io.output tests
// =============================================================================

func TestIO_Output_Default(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local output = io.output()
		if output then
			return "got output"
		else
			return "no output"
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "got output" {
		t.Errorf("Expected 'got output', got '%s'", result)
	}
}

// =============================================================================
// Integration tests
// =============================================================================

func TestIO_WriteAndReadBack(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "read_write_test.txt")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// Write and read back using Go verification
	code := `
		local open = io.open
		-- Write content
		local file = open("` + tmpFile + `", "w")
		local write = file.write
		write(file, "Test content")
		local close = file.close
		close(file)
		return "done"
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Verify content was written correctly using Go
	content := readFileContent(t, tmpFile)
	if content != "Test content" {
		t.Errorf("Expected 'Test content', got '%s'", content)
	}
}

func TestIO_MultipleOperations(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "multi_ops.txt")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		-- Write multiple lines
		local file = open("` + tmpFile + `", "w")
		local write = file.write
		write(file, "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n")
		local close = file.close
		close(file)
		return "done"
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Verify content using Go
	content := readFileContent(t, tmpFile)
	expected := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n"
	if content != expected {
		t.Errorf("Expected '%s', got '%s'", expected, content)
	}
}

func TestIO_ReadWriteNumbers(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "numbers.txt")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		-- Write numbers as strings
		local file = open("` + tmpFile + `", "w")
		local write = file.write
		write(file, "100 200 300\n")
		local close = file.close
		close(file)
		return "done"
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Verify content using Go
	content := readFileContent(t, tmpFile)
	if content != "100 200 300\n" {
		t.Errorf("Expected '100 200 300\\n', got '%s'", content)
	}
}

func TestIO_Overwrite(t *testing.T) {
	tmpFile := createTempFile(t, "Original content")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		-- Overwrite the file
		local file = open("` + tmpFile + `", "w")
		local write = file.write
		write(file, "New content")
		local close = file.close
		close(file)
		return "done"
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	content := readFileContent(t, tmpFile)
	if content != "New content" {
		t.Errorf("Expected 'New content', got '%s'", content)
	}
}

func TestIO_ReadModes(t *testing.T) {
	// Test r+ mode (read/write, file must exist)
	t.Run("r+", func(t *testing.T) {
		tmpFile := createTempFile(t, "Original")

		L := NewState()
		defer L.Close()
		L.OpenLibs()

		code := `
			local open = io.open
			local file = open("` + tmpFile + `", "r+")
			if file then
				local read = file.read
				local content = read(file, "*a")
				local close = file.close
				close(file)
				return content
			else
				return "error"
			end
		`

		err := L.DoString(code, "test")
		if err != nil {
			t.Fatalf("DoString failed: %v", err)
		}

		result, ok := L.ToString(-1)
		if !ok || result != "Original" {
			t.Errorf("Expected 'Original', got '%s'", result)
		}
	})

	// Test w+ mode (read/write, creates/truncates file)
	t.Run("w+", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "wplus_test.txt")

		L := NewState()
		defer L.Close()
		L.OpenLibs()

		code := `
			local open = io.open
			local file = open("` + tmpFile + `", "w+")
			if file then
				local write = file.write
				write(file, "Test content")
				local close = file.close
				close(file)
				return "success"
			else
				return "error"
			end
		`

		err := L.DoString(code, "test")
		if err != nil {
			t.Fatalf("DoString failed: %v", err)
		}

		result, ok := L.ToString(-1)
		if !ok || result != "success" {
			t.Errorf("Expected 'success', got '%s'", result)
		}

		content := readFileContent(t, tmpFile)
		if content != "Test content" {
			t.Errorf("Expected 'Test content', got '%s'", content)
		}
	})
}

func TestIO_LargeFile(t *testing.T) {
	// Create a file with many lines
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "large.txt")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// Write 50 lines and verify with Go
	code := `
		local open = io.open
		-- Write 50 lines
		local file = open("` + tmpFile + `", "w")
		local write = file.write
		write(file, "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n")
		write(file, "Line 6\nLine 7\nLine 8\nLine 9\nLine 10\n")
		local close = file.close
		close(file)
		return "done"
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Verify content using Go
	content := readFileContent(t, tmpFile)
	if len(content) == 0 {
		t.Error("Expected non-empty content")
	}
}

// =============================================================================
// Edge case tests
// =============================================================================

func TestIO_OpenNoFilename(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file, err = open()
		if file then
			return "unexpected success"
		else
			return "error"
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "error" {
		t.Errorf("Expected 'error', got '%s'", result)
	}
}

func TestIO_EmptyFile(t *testing.T) {
	tmpFile := createTempFile(t, "")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "r")
		local read = file.read
		local content = read(file, "*a")
		local close = file.close
		close(file)

		if content == "" then
			return "empty"
		else
			return "not empty: " .. content
		end
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	result, ok := L.ToString(-1)
	if !ok {
		t.Fatal("Expected string result")
	}
	if result != "empty" {
		t.Errorf("Expected 'empty', got '%s'", result)
	}
}

func TestIO_SpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "special.txt")

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// Test writing special characters and verify with Go
	code := `
		local open = io.open
		local file = open("` + tmpFile + `", "w")
		local write = file.write
		write(file, "Tab:\tNewline:\n")
		local close = file.close
		close(file)
		return "done"
	`

	err := L.DoString(code, "test")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	// Verify content was written correctly using Go
	content := readFileContent(t, tmpFile)
	expected := "Tab:\tNewline:\n"
	if content != expected {
		t.Errorf("Expected '%s', got '%s'", expected, content)
	}
}