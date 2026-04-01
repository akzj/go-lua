package internal

import (
	"os"
	"testing"

	luaapi "github.com/akzj/go-lua/api"
	io "github.com/akzj/go-lua/lib/io/api"
	tableapi "github.com/akzj/go-lua/table/api"
)

// mockLuaAPI implements a minimal LuaAPI for testing helper functions.
type mockLuaAPI struct {
	stack []mockValue
}

type mockValue struct {
	tp    int
	value interface{}
}

func newMockLuaAPI() *mockLuaAPI {
	return &mockLuaAPI{stack: make([]mockValue, 0)}
}

// Stack operations
func (m *mockLuaAPI) GetTop() int           { return len(m.stack) }
func (m *mockLuaAPI) SetTop(idx int)         { m.stack = m.stack[:idx] }
func (m *mockLuaAPI) Pop() {
	if len(m.stack) > 0 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}
func (m *mockLuaAPI) PushNil()               { m.stack = append(m.stack, mockValue{tp: luaapi.LUA_TNIL}) }
func (m *mockLuaAPI) PushInteger(n int64)    { m.stack = append(m.stack, mockValue{tp: luaapi.LUA_TNUMBER, value: n}) }
func (m *mockLuaAPI) PushNumber(n float64)   { m.stack = append(m.stack, mockValue{tp: luaapi.LUA_TNUMBER, value: n}) }
func (m *mockLuaAPI) PushString(s string)    { m.stack = append(m.stack, mockValue{tp: luaapi.LUA_TSTRING, value: s}) }
func (m *mockLuaAPI) PushBoolean(b bool)     { m.stack = append(m.stack, mockValue{tp: luaapi.LUA_TBOOLEAN, value: b}) }
func (m *mockLuaAPI) PushLightUserData(p interface{}) { m.stack = append(m.stack, mockValue{tp: luaapi.LUA_TLIGHTUSERDATA, value: p}) }
func (m *mockLuaAPI) PushGoFunction(fn func(L luaapi.LuaAPI) int) {}
func (m *mockLuaAPI) PushValue(idx int)      {}
func (m *mockLuaAPI) AbsIndex(idx int) int   { return idx }
func (m *mockLuaAPI) Rotate(idx, n int)     {}
func (m *mockLuaAPI) Copy(fromidx, toidx int) {}
func (m *mockLuaAPI) Insert(pos int)         {}
func (m *mockLuaAPI) CheckStack(n int) bool   { return true }
func (m *mockLuaAPI) XMove(to luaapi.LuaAPI, n int) {}

// Type checking
func (m *mockLuaAPI) Type(idx int) int {
	if idx < 1 || idx > len(m.stack) {
		return luaapi.LUA_TNONE
	}
	return m.stack[idx-1].tp
}
func (m *mockLuaAPI) TypeName(tp int) string       { return "mock" }
func (m *mockLuaAPI) IsNone(idx int) bool           { return idx < 1 || idx > len(m.stack) }
func (m *mockLuaAPI) IsNil(idx int) bool            { return idx >= 1 && idx <= len(m.stack) && m.stack[idx-1].tp == luaapi.LUA_TNIL }
func (m *mockLuaAPI) IsNoneOrNil(idx int) bool      { return m.IsNone(idx) || m.IsNil(idx) }
func (m *mockLuaAPI) IsBoolean(idx int) bool        { return false }
func (m *mockLuaAPI) IsString(idx int) bool         { return idx >= 1 && idx <= len(m.stack) && m.stack[idx-1].tp == luaapi.LUA_TSTRING }
func (m *mockLuaAPI) IsFunction(idx int) bool       { return false }
func (m *mockLuaAPI) IsTable(idx int) bool          { return false }
func (m *mockLuaAPI) IsLightUserData(idx int) bool  { return idx >= 1 && idx <= len(m.stack) && m.stack[idx-1].tp == luaapi.LUA_TLIGHTUSERDATA }
func (m *mockLuaAPI) IsThread(idx int) bool          { return false }
func (m *mockLuaAPI) IsInteger(idx int) bool         { return false }
func (m *mockLuaAPI) IsNumber(idx int) bool         { return idx >= 1 && idx <= len(m.stack) && m.stack[idx-1].tp == luaapi.LUA_TNUMBER }
func (m *mockLuaAPI) IsUserData(idx int) bool       { return false }

// Value conversion
func (m *mockLuaAPI) ToInteger(idx int) (int64, bool) {
	if idx >= 1 && idx <= len(m.stack) {
		if n, ok := m.stack[idx-1].value.(int64); ok {
			return n, true
		}
	}
	return 0, false
}
func (m *mockLuaAPI) ToNumber(idx int) (float64, bool) {
	if idx >= 1 && idx <= len(m.stack) {
		switch v := m.stack[idx-1].value.(type) {
		case int64:
			return float64(v), true
		case float64:
			return v, true
		}
	}
	return 0, false
}
func (m *mockLuaAPI) ToString(idx int) (string, bool) {
	if idx >= 1 && idx <= len(m.stack) {
		if s, ok := m.stack[idx-1].value.(string); ok {
			return s, true
		}
	}
	return "", false
}
func (m *mockLuaAPI) ToBoolean(idx int) bool       { return false }
func (m *mockLuaAPI) ToPointer(idx int) interface{} {
	if idx >= 1 && idx <= len(m.stack) {
		return m.stack[idx-1].value
	}
	return nil
}
func (m *mockLuaAPI) ToThread(idx int) luaapi.LuaAPI { return nil }

// Table operations
func (m *mockLuaAPI) CreateTable(narr, nrec int)  { m.stack = append(m.stack, mockValue{tp: luaapi.LUA_TTABLE}) }
func (m *mockLuaAPI) GetTable(idx int) int         { return luaapi.LUA_TNIL }
func (m *mockLuaAPI) GetField(idx int, k string) int { return luaapi.LUA_TNIL }
func (m *mockLuaAPI) GetI(idx int, n int64) int    { return luaapi.LUA_TNIL }
func (m *mockLuaAPI) RawGet(idx int) int           { return luaapi.LUA_TNIL }
func (m *mockLuaAPI) RawGetI(idx int, n int64) int { return luaapi.LUA_TNIL }
func (m *mockLuaAPI) SetTable(idx int)             {}
func (m *mockLuaAPI) SetField(idx int, k string)   {}
func (m *mockLuaAPI) SetI(idx int, n int64)        {}
func (m *mockLuaAPI) RawSet(idx int)                {}
func (m *mockLuaAPI) RawSetI(idx int, n int64)     {}
func (m *mockLuaAPI) GetGlobal(name string) int    { return luaapi.LUA_TNIL }
func (m *mockLuaAPI) SetGlobal(name string)        {}

// Metatable operations
func (m *mockLuaAPI) GetMetatable(idx int) bool     { return false }
func (m *mockLuaAPI) SetMetatable(idx int)        {}

// Comparison and length
func (m *mockLuaAPI) Compare(idx1, idx2 int, op int) bool { return false }
func (m *mockLuaAPI) Len(idx int)                {}
func (m *mockLuaAPI) RawLen(idx int) uint        { return 0 }

// Miscellaneous
func (m *mockLuaAPI) Next(idx int) bool          { return false }
func (m *mockLuaAPI) Concat(n int)               {}
func (m *mockLuaAPI) GC(what int, args ...int) int { return 0 }

// Error handling
func (m *mockLuaAPI) Error() int                 { return 0 }
func (m *mockLuaAPI) ErrorMessage() int          { return 0 }
func (m *mockLuaAPI) Where(level int)             {}

// Function calls
func (m *mockLuaAPI) Call(nArgs, nResults int)   {}
func (m *mockLuaAPI) PCall(nArgs, nResults, errfunc int) int { return 0 }
func (m *mockLuaAPI) Resume() error              { return nil }
func (m *mockLuaAPI) Yield(nResults int) error   { return nil }

// Thread/state
func (m *mockLuaAPI) NewThread() luaapi.LuaAPI   { return nil }
func (m *mockLuaAPI) Status() luaapi.Status      { return 0 }
func (m *mockLuaAPI) Global() interface{}         { return nil }
func (m *mockLuaAPI) Stack() []interface{}       { return nil }
func (m *mockLuaAPI) StackSize() int             { return len(m.stack) }
func (m *mockLuaAPI) GrowStack(n int)            {}
func (m *mockLuaAPI) CurrentCI() interface{}       { return nil }
func (m *mockLuaAPI) PushCI(ci interface{})      {}
func (m *mockLuaAPI) PopCI()                     {}

// Registry - using the correct type from table/api
func (m *mockLuaAPI) Registry() tableapi.TableInterface { return nil }
func (m *mockLuaAPI) PushGlobalTable()            {}
func (m *mockLuaAPI) Ref(t tableapi.TableInterface) int { return -1 }
func (m *mockLuaAPI) UnRef(t tableapi.TableInterface, ref int) {}

// TestNewIoLib tests that NewIoLib creates a valid IoLib.
func TestNewIoLib(t *testing.T) {
	lib := NewIoLib()
	if lib == nil {
		t.Fatal("NewIoLib returned nil")
	}
	var _ io.IoLib = lib
}

// TestCheckMode tests file mode validation.
func TestCheckMode(t *testing.T) {
	tests := []struct {
		mode   string
		wantOk bool
	}{
		// Valid modes
		{"r", true},
		{"w", true},
		{"a", true},
		{"r+", true},
		{"w+", true},
		{"a+", true},
		{"rb", true},
		{"wb", true},
		{"ab", true},
		{"r+b", true},
		{"w+b", true},
		{"a+b", true},
		{"br", true},
		{"bw", true},
		{"ba", true},
		// Invalid modes
		{"", false},
		{"x", false},
		{"xyz", false},
		{"123", false},
		{"rx", false},
		{"+r", false},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got := checkMode(tt.mode)
			if got != tt.wantOk {
				t.Errorf("checkMode(%q) = %v, want %v", tt.mode, got, tt.wantOk)
			}
		})
	}
}

// TestNewFileHandle tests creating file handles.
func TestNewFileHandle(t *testing.T) {
	// Create a temp file
	tmpfile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	defer tmpfile.Close()

	// Test creating a file handle
	fh := newFileHandle(tmpfile)
	if fh == nil {
		t.Fatal("newFileHandle returned nil")
	}
	if fh.File == nil {
		t.Error("FileHandle.File is nil")
	}
	if fh.CloseF == nil {
		t.Error("FileHandle.CloseF is nil")
	}

	// Test that we can close it
	err = closeFile(fh)
	if err != nil {
		t.Errorf("closeFile failed: %v", err)
	}
}

// TestToFileOrNil tests extracting file handles from mock stack.
func TestToFileOrNil(t *testing.T) {
	L := newMockLuaAPI()

	// Test nil case
	result := toFileOrNil(L, 1)
	if result != nil {
		t.Error("Expected nil for empty stack")
	}

	// Test nil value
	L.PushNil()
	result = toFileOrNil(L, 1)
	if result != nil {
		t.Error("Expected nil for nil value on stack")
	}
}

// TestParseNumber tests number parsing.
func TestParseNumber(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		ok    bool
	}{
		{"0", 0, true},
		{"123", 123, true},
		{"456789", 456789, true},
		{"", 0, false},
		{"abc", 0, false},
		{"12a", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := parseNumber(tt.input)
			if ok != tt.ok {
				t.Errorf("parseNumber(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("parseNumber(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestConvertMode tests Lua to Go file mode conversion.
func TestConvertMode(t *testing.T) {
	tests := []struct {
		mode   string
		goMode int
	}{
		{"r", os.O_RDONLY},
		{"w", os.O_WRONLY | os.O_CREATE | os.O_TRUNC},
		{"a", os.O_WRONLY | os.O_CREATE | os.O_APPEND},
		{"r+", os.O_RDWR},
		{"w+", os.O_RDWR | os.O_CREATE | os.O_TRUNC},
		{"a+", os.O_RDWR | os.O_CREATE | os.O_APPEND},
		{"rb", os.O_RDONLY},
		{"wb", os.O_WRONLY | os.O_CREATE | os.O_TRUNC},
		{"ab", os.O_WRONLY | os.O_CREATE | os.O_APPEND},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got := convertMode(tt.mode)
			if got != tt.goMode {
				t.Errorf("convertMode(%q) = %d, want %d", tt.mode, got, tt.goMode)
			}
		})
	}
}

// TestIsNumberChar tests number character detection.
func TestIsNumberChar(t *testing.T) {
	tests := []struct {
		c       byte
		isFirst bool
		want    bool
	}{
		{'0', true, true},
		{'5', true, true},
		{'9', true, true},
		{'-', true, true},
		{'+', true, true},
		{'.', true, true},
		{'e', true, true},
		{'E', true, true},
		{'a', true, false},
		{'x', true, false},
		{'0', false, true},
		{'-', false, false},
		{'+', false, false},
		{'.', false, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.c), func(t *testing.T) {
			got := isNumberChar(tt.c, tt.isFirst)
			if got != tt.want {
				t.Errorf("isNumberChar(%q, %v) = %v, want %v", tt.c, tt.isFirst, got, tt.want)
			}
		})
	}
}

// TestOsFileReadWrite tests osFile wrapper.
func TestOsFileReadWrite(t *testing.T) {
	// Create a temp file for testing
	tmpfile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Open for writing
	f, err := os.OpenFile(tmpfile.Name(), os.O_WRONLY, 0666)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}

	fh := &osFile{file: f}

	// Write test
	n, err := fh.Write([]byte("hello"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 5 {
		t.Errorf("Write returned %d, want 5", n)
	}

	f.Close()

	// Open for reading
	f, err = os.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to open file for reading: %v", err)
	}
	defer f.Close()

	fh = &osFile{file: f}

	// Read test
	buf := make([]byte, 10)
	n, err = fh.Read(buf)
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}
	if n != 5 {
		t.Errorf("Read returned %d, want 5", n)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("Read returned %q, want %q", string(buf[:n]), "hello")
	}

	// Seek test
	pos, err := fh.Seek(0, os.SEEK_SET)
	if err != nil {
		t.Errorf("Seek failed: %v", err)
	}
	if pos != 0 {
		t.Errorf("Seek returned %d, want 0", pos)
	}

	// Flush test
	err = fh.Flush()
	if err != nil {
		t.Errorf("Flush failed: %v", err)
	}
}

// TestFClose tests file close functionality.
func TestFClose(t *testing.T) {
	// Create a temp file
	tmpfile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Open file
	f, err := os.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}

	// Create file handle
	var fileInterface io.File = &osFile{file: f}
	fh := &io.FileHandle{
		File:  &fileInterface,
		CloseF: fclose,
	}

	// Close it
	err = closeFile(fh)
	if err != nil {
		t.Errorf("closeFile failed: %v", err)
	}

	// File should be nil now
	if fh.File != nil {
		t.Error("File should be nil after close")
	}
}

// TestCloseAlreadyClosedFile tests closing an already closed file.
func TestCloseAlreadyClosedFile(t *testing.T) {
	// Create a temp file
	tmpfile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Open file
	f, err := os.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}

	var fileInterface io.File = &osFile{file: f}
	fh := &io.FileHandle{
		File:  &fileInterface,
		CloseF: fclose,
	}

	// Close once
	err = closeFile(fh)
	if err != nil {
		t.Errorf("First closeFile failed: %v", err)
	}

	// Close again - should be safe
	err = closeFile(fh)
	if err != nil {
		t.Errorf("Second closeFile failed: %v", err)
	}
}

// TestOpenStdinHandle tests stdin handle creation.
func TestOpenStdinHandle(t *testing.T) {
	fh := openStdinHandle()
	if fh == nil {
		t.Fatal("openStdinHandle returned nil")
	}
	if fh.File == nil {
		t.Error("stdin FileHandle.File is nil")
	}
	// Stdin should not have a close function
	if fh.CloseF != nil {
		t.Error("stdin should not have close function")
	}
}

// TestOpenStdoutHandle tests stdout handle creation.
func TestOpenStdoutHandle(t *testing.T) {
	fh := openStdoutHandle()
	if fh == nil {
		t.Fatal("openStdoutHandle returned nil")
	}
	if fh.File == nil {
		t.Error("stdout FileHandle.File is nil")
	}
	// Stdout should not have a close function
	if fh.CloseF != nil {
		t.Error("stdout should not have close function")
	}
}

// TestOptString tests the optString helper function.
func TestOptString(t *testing.T) {
	L := newMockLuaAPI()

	// Test with empty stack (should return default)
	result := optString(L, 1, "default")
	if result != "default" {
		t.Errorf("optString returned %q, want %q", result, "default")
	}

	// Test with string on stack
	L.PushString("test")
	result = optString(L, 1, "default")
	if result != "test" {
		t.Errorf("optString returned %q, want %q", result, "test")
	}

	// Test with nil on stack (should return default)
	L.Pop()
	L.PushNil()
	result = optString(L, 1, "default")
	if result != "default" {
		t.Errorf("optString returned %q, want %q", result, "default")
	}
}

// TestNewFileHandleWithCloser tests creating file handles with custom close.
func TestNewFileHandleWithCloser(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	customClosed := false
	fh := newFileHandleWithCloser(tmpfile, func(h *io.FileHandle) error {
		customClosed = true
		return nil
	})

	if fh == nil {
		t.Fatal("newFileHandleWithCloser returned nil")
	}
	if fh.CloseF == nil {
		t.Error("CloseF should be set")
	}

	// Close with custom function
	err = closeFile(fh)
	if err != nil {
		t.Errorf("closeFile failed: %v", err)
	}
	if !customClosed {
		t.Error("Custom close function was not called")
	}
}
