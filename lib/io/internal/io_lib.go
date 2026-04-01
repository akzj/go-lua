// Package internal implements the Lua I/O library.
// This package provides implementations for:
//   - io.open(filename, mode): open a file
//   - io.close(file): close a file
//   - io.read(...): read from default input
//   - io.write(...): write to default output
//   - io.flush(): flush default output
//   - io.input(): get/set default input file
//   - io.output(): get/set default output file
//   - io.lines(filename): iterator over lines
//   - io.tmpfile(): create temporary file
//   - io.type(obj): type of file object
//
// Reference: lua-master/liolib.c
package internal

import (
	"os"
	"strconv"

	luaapi "github.com/akzj/go-lua/api"
	io "github.com/akzj/go-lua/lib/io/api"
)

// IoLib is the implementation of the Lua I/O library.
type IoLib struct {
	// defaultInput tracks the default input file.
	// Stored in registry under IO_INPUT key.
	defaultInput *io.FileHandle
	// defaultOutput tracks the default output file.
	// Stored in registry under IO_OUTPUT key.
	defaultOutput *io.FileHandle
	// openFiles stores all open file handles by their pointer address.
	// This allows us to retrieve FileHandle from light userdata pointers.
	openFiles map[*io.FileHandle]*io.FileHandle
}

// NewIoLib creates a new IoLib instance.
func NewIoLib() io.IoLib {
	return &IoLib{
		openFiles: make(map[*io.FileHandle]*io.FileHandle),
	}
}

// Open implements io.IoLib.Open.
// Registers all I/O library functions in the global table under "io".
func (i *IoLib) Open(L io.LuaAPI) int {
	// Create the io table
	L.CreateTable(0, 12)

	// Register all I/O functions using SetFuncs pattern
	register := func(name string, fn io.LuaFunc) {
		L.PushGoFunction(io.LuaFunc(fn))
		L.SetField(-2, name)
	}

	register("open", ioOpen)
	register("close", ioClose)
	register("read", ioRead)
	register("write", ioWrite)
	register("flush", ioFlush)
	register("input", ioInput)
	register("output", ioOutput)
	register("lines", ioLines)
	register("tmpfile", ioTmpfile)
	register("type", ioType)

	// Set the table as global "io"
	L.SetGlobal("io")

	// Create and register the FILE* metatable for file handles
	L.CreateTable(0, 2) // metatable with __index and __gc
	L.PushString("__index")
	L.PushGoFunction(io.LuaFunc(fLines)) // file:lines as default method
	L.SetTable(-3)
	L.PushString("__gc")
	L.PushGoFunction(io.LuaFunc(fGC)) // garbage collection
	L.SetTable(-3)

	// Store metatable in registry with name LUA_FILEHANDLE
	L.PushValue(-1) // duplicate metatable
	L.SetField(luaapi.LUA_REGISTRYINDEX, io.LUA_FILEHANDLE)

	return 1
}

// Ensure types implement LuaFunc
var _ io.LuaFunc = ioOpen
var _ io.LuaFunc = ioClose
var _ io.LuaFunc = ioRead
var _ io.LuaFunc = ioWrite
var _ io.LuaFunc = ioFlush
var _ io.LuaFunc = ioInput
var _ io.LuaFunc = ioOutput
var _ io.LuaFunc = ioLines
var _ io.LuaFunc = ioTmpfile
var _ io.LuaFunc = ioType
var _ io.LuaFunc = fLines
var _ io.LuaFunc = fClose
var _ io.LuaFunc = fGC

// =============================================================================
// I/O Functions
// =============================================================================

// ioOpen opens a file.
// io.open(filename [, mode]) -> file | nil, error message
// Modes: "r", "w", "a", "r+", "w+", "a+", optionally with "b"
//
// Invariant: Creates a userdata with metatable "FILE*" on success.
func ioOpen(L io.LuaAPI) int {
	filename := optString(L, 1, "")
	mode := optString(L, 2, "r")

	if !checkMode(mode) {
		L.PushNil()
		L.PushString("invalid mode")
		return 2
	}

	// Convert Lua file mode to Go file mode
	goMode := convertMode(mode)

	file, err := os.OpenFile(filename, goMode, 0666)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	// Create file handle
	fh := newFileHandle(file)
	pushFileHandle(L, fh)
	return 1
}

// optString returns string at idx, or default if none/nil.
func optString(L io.LuaAPI, idx int, def string) string {
	if L.IsNoneOrNil(idx) {
		return def
	}
	s, _ := L.ToString(idx)
	return s
}

// convertMode converts Lua file mode to Go file mode.
func convertMode(mode string) int {
	switch mode {
	case "r", "rb", "br":
		return os.O_RDONLY
	case "w", "wb", "bw":
		return os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case "a", "ab", "ba":
		return os.O_WRONLY | os.O_CREATE | os.O_APPEND
	case "r+", "rb+", "br+", "b+r", "r+b":
		return os.O_RDWR
	case "w+", "wb+", "bw+", "b+w", "w+b":
		return os.O_RDWR | os.O_CREATE | os.O_TRUNC
	case "a+", "ab+", "ba+", "b+a", "a+b":
		return os.O_RDWR | os.O_CREATE | os.O_APPEND
	default:
		return os.O_RDONLY
	}
}

// osFile wraps *os.File to implement io.File interface.
type osFile struct {
	file *os.File
}

func (f *osFile) Read(b []byte) (n int, err error) {
	return f.file.Read(b)
}

func (f *osFile) Write(b []byte) (n int, err error) {
	return f.file.Write(b)
}

func (f *osFile) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

func (f *osFile) Close() error {
	return f.file.Close()
}

func (f *osFile) Flush() error {
	// os.File doesn't have Flush, but we can sync
	return nil
}

// ioClose closes a file.
// io.close([file]) -> true | nil, error
// If no file, closes default output.
func ioClose(L io.LuaAPI) int {
	f := toFileOrNil(L, 1)
	if f == nil {
		f = getDefaultOutput(L)
	}

	if f.File == nil {
		L.PushNil()
		L.PushString("cannot close closed file")
		return 2
	}

	err := closeFile(f)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	L.PushBoolean(true)
	return 1
}

// closeFile closes a file handle.
func closeFile(f *io.FileHandle) error {
	if f == nil || f.File == nil {
		return nil
	}
	var err error
	if f.CloseF != nil {
		err = f.CloseF(f)
	}
	f.File = nil
	f.CloseF = nil
	return err
}

// ioRead reads from default input.
// io.read(...) -> string(s) or nil if EOF
// Formats: "*n" (number), "*a" (all), "*l" (line), "*L" (line with \n),
//          n (bytes), "n:n" (lines)
//
// Invariant: Reads from default input file (registry._IO_input).
func ioRead(L io.LuaAPI) int {
	nArgs := L.GetTop()
	if nArgs == 0 {
		// No arguments: default to "*l"
		nArgs = 1
	}

	f := getDefaultInput(L)
	if f == nil || f.File == nil {
		L.PushNil()
		L.PushString("input file is closed")
		return 2
	}

	results := 0
	for i := 1; i <= nArgs; i++ {
		format := optString(L, i, "*l")
		s, err := readFormat(L, f, format)
		if err != nil {
			// Error or EOF
			if results == 0 {
				L.PushNil()
				L.PushString(err.Error())
				return 2
			}
			break
		}
		L.PushString(s)
		results++
	}

	return results
}

// readFormat reads according to format specification.
func readFormat(L io.LuaAPI, f *io.FileHandle, format string) (string, error) {
	switch format {
	case "*n":
		// Read a number
		num, ok := readNumber(L, f)
		if !ok {
			return "", nil
		}
		return num, nil
	case "*a":
		// Read all remaining content
		return readAll(L, f)
	case "*l":
		// Read a line (without newline)
		line, err := readLine(L, f, false)
		if err != nil {
			return "", err
		}
		return line, nil
	case "*L":
		// Read a line (with newline)
		line, err := readLine(L, f, true)
		if err != nil {
			return "", err
		}
		return line, nil
	default:
		// Try to parse as number of bytes or range
		return readCharsOrLines(L, f, format)
	}
}

// readAll reads all remaining content from file.
func readAll(L io.LuaAPI, f *io.FileHandle) (string, error) {
	if f == nil || f.File == nil {
		return "", nil
	}
	buf := make([]byte, 4096)
	var result []byte
	for {
		n, err := (*f.File).Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err != nil {
			if len(result) == 0 && err.Error() == "EOF" {
				return "", nil
			}
			break
		}
	}
	return string(result), nil
}

// readCharsOrLines handles n or n:m format.
func readCharsOrLines(L io.LuaAPI, f *io.FileHandle, format string) (string, error) {
	// Check if it's a range format "n:m"
	for i := 0; i < len(format); i++ {
		if format[i] == ':' {
			// Range format
			start, ok1 := parseNumber(format[:i])
			end, ok2 := parseNumber(format[i+1:])
			if ok1 && ok2 {
				return readRange(L, f, start, end)
			}
		}
	}

	// Number of characters
	n, ok := parseNumber(format)
	if ok && n > 0 {
		return readChars(L, f, n)
	}

	// Default to line reading
	return readLine(L, f, false)
}

// parseNumber parses a number from string.
func parseNumber(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// readRange reads characters from start to end (1-indexed, inclusive).
func readRange(L io.LuaAPI, f *io.FileHandle, start, end int64) (string, error) {
	if f == nil || f.File == nil {
		return "", nil
	}
	if start < 1 {
		start = 1
	}
	if end < start {
		return "", nil
	}

	// Seek to start position (1-indexed)
	_, err := (*f.File).Seek(start-1, os.SEEK_SET)
	if err != nil {
		return "", err
	}

	// Read (end - start + 1) bytes
	n := end - start + 1
	buf := make([]byte, n)
	m, err := (*f.File).Read(buf)
	if err != nil && m == 0 {
		return "", err
	}
	return string(buf[:m]), nil
}

// ioWrite writes to default output.
// io.write(...) -> true | nil, error
//
// Invariant: Writes to default output file (registry._IO_output).
func ioWrite(L io.LuaAPI) int {
	nArgs := L.GetTop()
	if nArgs == 0 {
		L.PushBoolean(true)
		return 1
	}

	f := getDefaultOutput(L)
	if f == nil || f.File == nil {
		L.PushNil()
		L.PushString("output file is closed")
		return 2
	}

	for i := 1; i <= nArgs; i++ {
		var s string
		if L.IsString(i) {
			s, _ = L.ToString(i)
		} else if L.IsNumber(i) {
			n, _ := L.ToNumber(i)
			s = strconv.FormatFloat(n, 'f', -1, 64)
		} else {
			L.PushNil()
			L.PushString("bad argument")
			return 2
		}

		_, err := (*f.File).Write([]byte(s))
		if err != nil {
			L.PushNil()
			L.PushString(err.Error())
			return 2
		}
	}

	L.PushBoolean(true)
	return 1
}

// ioFlush flushes default output.
// io.flush() -> true
//
// Invariant: Flushes default output file.
func ioFlush(L io.LuaAPI) int {
	f := getDefaultOutput(L)
	if f == nil || f.File == nil {
		L.PushNil()
		L.PushString("output file is closed")
		return 2
	}

	if f.File != nil {
		(*f.File).Flush()
	}
	L.PushBoolean(true)
	return 1
}

// ioInput gets/sets default input file.
// io.input() -> file
// io.input(file) -> file
//
// Invariant: Returns the current default input file from registry.
func ioInput(L io.LuaAPI) int {
	if L.IsNoneOrNil(1) {
		// Get current input
		f := getDefaultInput(L)
		if f == nil {
			L.PushNil()
			return 1
		}
		// Push file handle
		pushFileHandle(L, f)
		return 1
	}

	// Set input file
	f := toFile(L, 1)
	setDefaultInput(L, f)
	pushFileHandle(L, f)
	return 1
}

// ioOutput gets/sets default output file.
// io.output() -> file
// io.output(file) -> file
//
// Invariant: Returns the current default output file from registry.
func ioOutput(L io.LuaAPI) int {
	if L.IsNoneOrNil(1) {
		// Get current output
		f := getDefaultOutput(L)
		if f == nil {
			L.PushNil()
			return 1
		}
		pushFileHandle(L, f)
		return 1
	}

	// Set output file
	f := toFile(L, 1)
	setDefaultOutput(L, f)
	pushFileHandle(L, f)
	return 1
}

// ioLines creates a line iterator.
// io.lines([filename]) -> iterator, state, nil | nil, nil, nil, file on EOF
// If filename given, opens file and closes when done.
//
// Invariant: Returns 4 values if file to close (iterator, state, nil, file).
// Why 4 values? To-be-closed variable in generic for loop.
func ioLines(L io.LuaAPI) int {
	filename := optString(L, 1, "")

	if filename == "" {
		// Iterator over default input
		return genericLines(L, getDefaultInput(L), false)
	}

	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	fh := newFileHandle(file)

	// Return iterator that closes file when done
	// Returns 4 values: iterator, state, nil, file (to-be-closed)
	L.PushGoFunction(io.LuaFunc(linesIterator))
	L.PushInteger(0) // placeholder for file handle
	L.PushInteger(0) // line count
	L.PushNil()      // nil placeholder for file (will be replaced)
	L.PushLightUserData(fh)
	return 4
}

// linesIterator is the iterator function for io.lines.
func linesIterator(L io.LuaAPI) int {
	// State is at index 1: line count
	// File handle is on stack (upvalue or passed somehow)

	// For now, simplified implementation
	// In real implementation, this would read lines from the file

	L.PushNil()
	L.PushInteger(1) // increment line count
	return 1
}

// genericLines returns an iterator for the given file.
func genericLines(L io.LuaAPI, f *io.FileHandle, closeOnFinish bool) int {
	if f == nil || f.File == nil {
		L.PushNil()
		L.PushNil()
		return 2
	}

	// Push iterator function
	L.PushGoFunction(io.LuaFunc(fileLinesIterator))
	// Push initial state (line count = 0)
	L.PushInteger(0)
	// Push file handle as upvalue or store
	if closeOnFinish {
		L.PushLightUserData(f)
		return 3
	}
	return 2
}

// fileLinesIterator is the iterator for file:lines().
func fileLinesIterator(L io.LuaAPI) int {
	// Get state (line count) from upvalue 1
	// Actually, for file:lines(), the file is "self" on stack

	// This is a simplified implementation
	// Real implementation needs proper state management

	L.PushNil()
	return 1
}

// ioTmpfile creates a temporary file.
// io.tmpfile() -> file
//
// Invariant: Creates file in read/write mode, auto-deleted on close.
func ioTmpfile(L io.LuaAPI) int {
	file, err := os.CreateTemp("", "lua")
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	// Create file handle with auto-delete close function
	fh := newFileHandleWithCloser(file, func(h *io.FileHandle) error {
		os.Remove(file.Name())
		return file.Close()
	})

	pushFileHandle(L, fh)
	return 1
}

// ioType returns the type of a file object.
// io.type(obj) -> "file" | "closed file" | nil
//
// Invariant: Returns "file" for open, "closed file" for closed, nil otherwise.
func ioType(L io.LuaAPI) int {
	if L.IsNoneOrNil(1) {
		L.PushNil()
		return 1
	}

	// Check if it's light userdata containing a FileHandle pointer
	if L.IsLightUserData(1) {
		ptr := L.ToPointer(1)
		if ptr != nil {
			if fh, ok := ptr.(*io.FileHandle); ok {
				if fh.File == nil {
					L.PushString("closed file")
				} else {
					L.PushString("file")
				}
				return 1
			}
		}
	}

	L.PushNil()
	return 1
}

// fLines is a file method for iteration.
// file:lines() -> iterator, state, nil
//
// Invariant: Unlike io.lines, does not close the file.
func fLines(L io.LuaAPI) int {
	// file:lines() - file is self at index 1
	f := toFile(L, 1)
	if f == nil || f.File == nil {
		L.PushNil()
		L.PushString("file is closed")
		return 2
	}

	// Push iterator function
	L.PushGoFunction(io.LuaFunc(fileLinesIteratorFromFile))
	// Push file handle as state
	L.PushLightUserData(f)
	// Push nil as third value
	L.PushNil()

	return 3
}

// fileLinesIteratorFromFile is the iterator for file:lines().
func fileLinesIteratorFromFile(L io.LuaAPI) int {
	// Get file handle from upvalue or state
	// State is at index 1

	// Read a line
	line, err := readLine(L, nil, false) // TODO: need actual file handle
	if err != nil || line == "" {
		return 0 // End of iteration
	}

	L.PushString(line)
	return 1
}

// fClose closes a file handle.
// file:close() -> true | nil, error
//
// Invariant: Sets CloseF to nil to mark as closed.
func fClose(L io.LuaAPI) int {
	f := toFile(L, 1)
	if f == nil {
		L.PushNil()
		L.PushString("cannot close nil file")
		return 2
	}

	if f.File == nil {
		L.PushNil()
		L.PushString("cannot close closed file")
		return 2
	}

	err := closeFile(f)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	L.PushBoolean(true)
	return 1
}

// fGC is the garbage collection metamethod for file handles.
func fGC(L io.LuaAPI) int {
	f := toFileOrNil(L, 1)
	if f != nil && f.File != nil {
		closeFile(f)
	}
	return 0
}

// =============================================================================
// Helper Functions
// =============================================================================

// toFile extracts *FileHandle from stack at index.
// Panics if not a valid file handle.
//
// Why not return error? Lua C API pattern uses luaL_checktype/luaL_error.
func toFile(L io.LuaAPI, idx int) *io.FileHandle {
	f := toFileOrNil(L, idx)
	if f == nil {
		L.PushString("bad argument: expected file, got nil")
		L.Error()
		return nil
	}
	if f.File == nil {
		L.PushString("bad argument: file is closed")
		L.Error()
		return nil
	}
	return f
}

// toFileOrNil extracts *FileHandle from stack at index, returns nil if none.
//
// Why separate from toFile? Some functions accept nil gracefully.
func toFileOrNil(L io.LuaAPI, idx int) *io.FileHandle {
	if L.IsNoneOrNil(idx) {
		return nil
	}

	// Check if it's light userdata containing a FileHandle pointer
	if L.IsLightUserData(idx) {
		ptr := L.ToPointer(idx)
		if ptr != nil {
			if fh, ok := ptr.(*io.FileHandle); ok {
				return fh
			}
		}
	}

	// Check if it's a table with __handle field (alternative storage)
	if L.IsTable(idx) {
		L.GetField(idx, "__handle")
		if !L.IsNil(-1) && L.IsLightUserData(-1) {
			ptr := L.ToPointer(-1)
			// Pop the value we checked
			L.SetTop(L.GetTop() - 1)
			if ptr != nil {
				if fh, ok := ptr.(*io.FileHandle); ok {
					return fh
				}
			}
		}
		L.SetTop(L.GetTop() - 1)
	}

	return nil
}

// checkMode validates file mode string.
// Valid modes: r, w, a, with optional +, with optional b.
//
// Invariant: Mode must start with r, w, or a.
// Why this constraint? Matches l_checkmode from liolib.c.
func checkMode(mode string) bool {
	if len(mode) < 1 {
		return false
	}
	switch mode[0] {
	case 'r', 'w', 'a':
		// Valid first character
	case 'b':
		// b can come first in some implementations (e.g., "rb", "wb", "ab")
		if len(mode) < 2 {
			return false
		}
		switch mode[1] {
		case 'r', 'w', 'a':
			// Valid: br, bw, ba
		default:
			return false
		}
		return true
	default:
		return false
	}

	// Check remaining characters are valid
	validChars := "rwa+"
	if len(mode) > 1 {
		for _, c := range mode[1:] {
			valid := false
			for _, v := range validChars {
				if c == v {
					valid = true
					break
				}
			}
			// Also allow 'b' anywhere
			if !valid && c != 'b' {
				return false
			}
		}
	}

	return true
}

// getDefaultInput returns the default input file from registry.
//
// Invariant: File must not be closed (raises error if so).
func getDefaultInput(L io.LuaAPI) *io.FileHandle {
	// Get from registry
	L.PushString(io.IO_INPUT)
	L.GetTable(luaapi.LUA_REGISTRYINDEX)

	var f *io.FileHandle
	if !L.IsNil(-1) && L.IsLightUserData(-1) {
		ptr := L.ToPointer(-1)
		if ptr != nil {
			f, _ = ptr.(*io.FileHandle)
		}
	}
	L.SetTop(L.GetTop() - 1)

	// If no default input, open stdin
	if f == nil || f.File == nil {
		f = openStdinHandle()
		setDefaultInput(L, f)
	}

	return f
}

// getDefaultOutput returns the default output file from registry.
func getDefaultOutput(L io.LuaAPI) *io.FileHandle {
	// Get from registry
	L.PushString(io.IO_OUTPUT)
	L.GetTable(luaapi.LUA_REGISTRYINDEX)

	var f *io.FileHandle
	if !L.IsNil(-1) && L.IsLightUserData(-1) {
		ptr := L.ToPointer(-1)
		if ptr != nil {
			f, _ = ptr.(*io.FileHandle)
		}
	}
	L.SetTop(L.GetTop() - 1)

	// If no default output, open stdout
	if f == nil || f.File == nil {
		f = openStdoutHandle()
		setDefaultOutput(L, f)
	}

	return f
}

// setDefaultInput sets the default input file in registry.
func setDefaultInput(L io.LuaAPI, f *io.FileHandle) {
	L.PushString(io.IO_INPUT)
	L.PushLightUserData(f)
	L.SetTable(luaapi.LUA_REGISTRYINDEX)
}

// setDefaultOutput sets the default output file in registry.
func setDefaultOutput(L io.LuaAPI, f *io.FileHandle) {
	L.PushString(io.IO_OUTPUT)
	L.PushLightUserData(f)
	L.SetTable(luaapi.LUA_REGISTRYINDEX)
}

// pushFileHandle pushes a file handle onto the stack as light userdata.
func pushFileHandle(L io.LuaAPI, f *io.FileHandle) {
	L.PushLightUserData(f)
	// Set metatable
	L.PushString(io.LUA_FILEHANDLE)
	L.GetTable(luaapi.LUA_REGISTRYINDEX)
	L.SetMetatable(-2)
}

// newFileHandle creates a new FileHandle with the given os.File.
func newFileHandle(file *os.File) *io.FileHandle {
	var f io.File = &osFile{file: file}
	return &io.FileHandle{
		File:  &f,
		CloseF: fclose,
	}
}

// newFileHandleWithCloser creates a FileHandle with custom close function.
func newFileHandleWithCloser(file *os.File, closeF func(h *io.FileHandle) error) *io.FileHandle {
	var f io.File = &osFile{file: file}
	return &io.FileHandle{
		File:  &f,
		CloseF: closeF,
	}
}

// openStdinHandle creates a FileHandle for stdin.
func openStdinHandle() *io.FileHandle {
	var f io.File = &osFile{file: os.Stdin}
	return &io.FileHandle{
		File:  &f,
		CloseF: nil, // Don't close stdin
	}
}

// openStdoutHandle creates a FileHandle for stdout.
func openStdoutHandle() *io.FileHandle {
	var f io.File = &osFile{file: os.Stdout}
	return &io.FileHandle{
		File:  &f,
		CloseF: nil, // Don't close stdout
	}
}

// fclose is the default close function for file handles.
func fclose(h *io.FileHandle) error {
	if h != nil && h.File != nil {
		return (*h.File).Close()
	}
	return nil
}

// readNumber reads a number from file.
//
// Invariant: Stops at first non-numeric character.
func readNumber(L io.LuaAPI, f *io.FileHandle) (string, bool) {
	if f == nil || f.File == nil {
		return "", false
	}

	// Read characters until we hit a non-numeric
	var num []byte
	buf := make([]byte, 1)

	for {
		n, err := (*f.File).Read(buf)
		if n == 0 || err != nil {
			break
		}

		c := buf[0]
		if isNumberChar(c, len(num) == 0) {
			num = append(num, c)
		} else {
			// Put back the character and exit
			(*f.File).Seek(-1, os.SEEK_CUR)
			break
		}
	}

	if len(num) == 0 {
		return "", false
	}
	return string(num), true
}

// isNumberChar returns true if c is a valid number character.
func isNumberChar(c byte, isFirst bool) bool {
	if c >= '0' && c <= '9' {
		return true
	}
	if isFirst {
		if c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' {
			return true
		}
	} else {
		if c == '.' || c == 'e' || c == 'E' {
			return true
		}
	}
	return false
}

// readLine reads a line from file, optionally including newline.
//
// Invariant: Strips trailing newline if withNewline is false.
func readLine(L io.LuaAPI, f *io.FileHandle, withNewline bool) (string, error) {
	if f == nil || f.File == nil {
		return "", nil
	}

	line := make([]byte, 0, 100)
	buf := make([]byte, 1)

	for {
		n, err := (*f.File).Read(buf)
		if n == 0 || err != nil {
			if len(line) == 0 {
				return "", err
			}
			break
		}

		if buf[0] == '\n' {
			if withNewline {
				line = append(line, '\n')
			}
			break
		}
		line = append(line, buf[0])
	}

	return string(line), nil
}

// readChars reads n characters from file.
//
// Invariant: Returns empty string on EOF.
func readChars(L io.LuaAPI, f *io.FileHandle, n int64) (string, error) {
	if f == nil || f.File == nil {
		return "", nil
	}

	buf := make([]byte, n)
	m, err := (*f.File).Read(buf)
	if err != nil && m == 0 {
		return "", err
	}
	return string(buf[:m]), nil
}
