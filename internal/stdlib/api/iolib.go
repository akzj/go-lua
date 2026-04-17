package api

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	luaapi "github.com/akzj/go-lua/internal/api/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
)

// ---------------------------------------------------------------------------
// IO Library — mirrors liolib.c
// ---------------------------------------------------------------------------

// Registry keys for default input/output files (mirrors IO_INPUT/IO_OUTPUT)
const (
	ioInput  = "_IO_input"
	ioOutput = "_IO_output"
	fileMT   = "FILE*" // metatable name for file handles
)

// closeFunc is the type for the close function stored in the file handle.
// nil means the file is closed.
type closeFunc func(L *luaapi.State) int

// ioStream represents an open file handle (mirrors LStream in liolib.c).
type ioStream struct {
	f      *os.File
	closef closeFunc
}

// isClosed returns true if the stream has been closed.
func (s *ioStream) isClosed() bool {
	return s.closef == nil
}

// ---------------------------------------------------------------------------
// Helpers to get/set ioStream from userdata
// ---------------------------------------------------------------------------

// newFileUD creates a new file userdata (closed state) and pushes it.
// Sets the FILE* metatable. Returns the ioStream.
func newFileUD(L *luaapi.State) *ioStream {
	stream := &ioStream{}
	ud := L.NewUserdata(0, 0)
	ud.Data = stream
	// Set FILE* metatable
	L.GetField(luaapi.RegistryIndex, fileMT)
	L.SetMetatable(-2)
	return stream
}

// toStream checks arg at idx is FILE* userdata and returns the ioStream.
func toStream(L *luaapi.State, idx int) *ioStream {
	L.CheckUdata(idx, fileMT)
	ud := L.GetUserdataObj(idx)
	if ud == nil {
		return nil
	}
	s, ok := ud.Data.(*ioStream)
	if !ok {
		return nil
	}
	return s
}

// toFile checks arg at idx is an open FILE* and returns the *os.File.
func toFile(L *luaapi.State, idx int) *os.File {
	s := toStream(L, idx)
	if s == nil || s.isClosed() {
		L.Errorf("attempt to use a closed file")
		return nil
	}
	return s.f
}

// getIOFile returns the *os.File for the default input or output.
func getIOFile(L *luaapi.State, findex string) *os.File {
	L.GetField(luaapi.RegistryIndex, findex)
	ud := L.GetUserdataObj(-1)
	L.Pop(1)
	if ud == nil {
		L.Errorf("default %s file is closed", findex[len("_IO_"):])
		return nil
	}
	s, ok := ud.Data.(*ioStream)
	if !ok || s.isClosed() {
		L.Errorf("default %s file is closed", findex[len("_IO_"):])
		return nil
	}
	return s.f
}

// getIOStream returns the ioStream for the default input or output.
func getIOStream(L *luaapi.State, findex string) *ioStream {
	L.GetField(luaapi.RegistryIndex, findex)
	ud := L.GetUserdataObj(-1)
	L.Pop(1)
	if ud == nil {
		return nil
	}
	s, ok := ud.Data.(*ioStream)
	if !ok {
		return nil
	}
	return s
}

// ---------------------------------------------------------------------------
// File close functions
// ---------------------------------------------------------------------------

// ioFclose closes a regular file (mirrors io_fclose).
func ioFclose(L *luaapi.State) int {
	s := toStream(L, 1)
	err := s.f.Close()
	return pushFileResult(L, err == nil, "", err)
}

// ioNoclose "closes" a standard file — keeps it open, returns fail.
func ioNoclose(L *luaapi.State) int {
	s := toStream(L, 1)
	s.closef = ioNoclose // keep it "open"
	L.PushFail()
	L.PushString("cannot close standard file")
	return 2
}

// auxClose calls the close function for a file handle.
func auxClose(L *luaapi.State) int {
	s := toStream(L, 1)
	cf := s.closef
	s.closef = nil // mark as closed
	return cf(L)
}

// ---------------------------------------------------------------------------
// IO library functions
// ---------------------------------------------------------------------------

func OpenIO(L *luaapi.State) int {
	// Create FILE* metatable
	createFileMeta(L)

	// Create io library table
	L.NewLib(map[string]luaapi.CFunction{
		"close":   ioClose,
		"flush":   ioFlush,
		"input":   ioInputFn,
		"lines":   ioLines,
		"open":    ioOpen,
		"output":  ioOutputFn,
		"read":    ioRead,
		"tmpfile": ioTmpfile,
		"type":    ioType,
		"write":   ioWrite,
	})

	// Create standard file handles
	createStdFile(L, os.Stdin, ioInput, "stdin")
	createStdFile(L, os.Stdout, ioOutput, "stdout")
	createStdFile(L, os.Stderr, "", "stderr")

	return 1
}

// createFileMeta creates the FILE* metatable with methods and metamethods.
func createFileMeta(L *luaapi.State) {
	L.NewMetatable(fileMT)

	// Metamethods
	L.PushCFunction(fGC)
	L.SetField(-2, "__gc")
	L.PushCFunction(fGC)
	L.SetField(-2, "__close")
	L.PushCFunction(fTostring)
	L.SetField(-2, "__tostring")
	L.PushString(fileMT)
	L.SetField(-2, "__name")

	// Methods table (becomes __index)
	L.NewLib(map[string]luaapi.CFunction{
		"read":    fRead,
		"write":   fWrite,
		"lines":   fLines,
		"flush":   fFlush,
		"seek":    fSeek,
		"close":   fClose,
		"setvbuf": fSetvbuf,
	})
	L.SetField(-2, "__index")

	L.Pop(1) // pop metatable
}

// createStdFile creates a standard file handle and registers it.
func createStdFile(L *luaapi.State, f *os.File, regKey string, fieldName string) {
	stream := newFileUD(L)
	stream.f = f
	stream.closef = ioNoclose
	if regKey != "" {
		L.PushValue(-1)
		L.SetField(luaapi.RegistryIndex, regKey)
	}
	L.SetField(-2, fieldName) // io[fieldName] = userdata
}

// ---------------------------------------------------------------------------
// io.type(obj) → "file" | "closed file" | false
// ---------------------------------------------------------------------------

func ioType(L *luaapi.State) int {
	L.CheckAny(1)
	if !L.TestUdata(1, fileMT) {
		L.PushFail()
		return 1
	}
	ud := L.GetUserdataObj(1)
	if ud == nil {
		L.PushFail()
		return 1
	}
	s, ok := ud.Data.(*ioStream)
	if !ok {
		L.PushFail()
		return 1
	}
	if s.isClosed() {
		L.PushString("closed file")
	} else {
		L.PushString("file")
	}
	return 1
}

// ---------------------------------------------------------------------------
// io.open(filename [, mode]) → file handle or nil, errmsg, errno
// ---------------------------------------------------------------------------

func ioOpen(L *luaapi.State) int {
	filename := L.CheckString(1)
	mode := L.OptString(2, "r")

	// Validate mode: must match [rwa]%+?[b]*
	if !checkMode(mode) {
		L.ArgError(2, "invalid mode")
		return 0
	}

	stream := newFileUD(L)
	stream.closef = ioFclose

	var flag int
	switch {
	case strings.HasPrefix(mode, "r+") || strings.HasPrefix(mode, "rb+") || strings.HasPrefix(mode, "r+b"):
		flag = os.O_RDWR
	case strings.HasPrefix(mode, "w+") || strings.HasPrefix(mode, "wb+") || strings.HasPrefix(mode, "w+b"):
		flag = os.O_RDWR | os.O_CREATE | os.O_TRUNC
	case strings.HasPrefix(mode, "a+") || strings.HasPrefix(mode, "ab+") || strings.HasPrefix(mode, "a+b"):
		flag = os.O_RDWR | os.O_CREATE | os.O_APPEND
	case strings.HasPrefix(mode, "r"):
		flag = os.O_RDONLY
	case strings.HasPrefix(mode, "w"):
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case strings.HasPrefix(mode, "a"):
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	default:
		flag = os.O_RDONLY
	}

	f, err := os.OpenFile(filename, flag, 0666)
	if err != nil {
		L.Pop(1) // pop the userdata
		return pushFileResult(L, false, filename, err)
	}
	stream.f = f
	return 1
}

// checkMode validates the mode string for fopen.
// Must match [rwa]%+?[b]*
func checkMode(mode string) bool {
	if len(mode) == 0 {
		return false
	}
	i := 0
	if mode[i] != 'r' && mode[i] != 'w' && mode[i] != 'a' {
		return false
	}
	i++
	if i < len(mode) && mode[i] == '+' {
		i++
	}
	// Rest must be only 'b' characters
	for i < len(mode) {
		if mode[i] != 'b' {
			return false
		}
		i++
	}
	return true
}

// ---------------------------------------------------------------------------
// io.close([file]) — close file or default output
// ---------------------------------------------------------------------------

func ioClose(L *luaapi.State) int {
	if L.IsNone(1) {
		L.GetField(luaapi.RegistryIndex, ioOutput)
	}
	return fClose(L)
}

// f:close()
func fClose(L *luaapi.State) int {
	toFile(L, 1) // check it's open
	return auxClose(L)
}

// __gc / __close metamethod
func fGC(L *luaapi.State) int {
	s := toStream(L, 1) // validates argument is FILE* userdata
	if s.isClosed() {
		return 0
	}
	// Close the file, ignoring errors
	if s.f != nil {
		auxClose(L)
	}
	return 0
}

// __tostring metamethod
func fTostring(L *luaapi.State) int {
	ud := L.GetUserdataObj(1)
	if ud == nil {
		L.PushString("file (closed)")
		return 1
	}
	s, ok := ud.Data.(*ioStream)
	if !ok || s.isClosed() {
		L.PushString("file (closed)")
		return 1
	}
	L.PushString(fmt.Sprintf("file (%p)", s.f))
	return 1
}

// ---------------------------------------------------------------------------
// io.input([file]) / io.output([file])
// ---------------------------------------------------------------------------

func gIOFile(L *luaapi.State, regKey string, mode string) int {
	if !L.IsNoneOrNil(1) {
		filename, ok := L.ToString(1)
		if ok {
			// Open a file with the given name
			stream := newFileUD(L)
			stream.closef = ioFclose
			f, err := os.OpenFile(filename, openFlags(mode), 0666)
			if err != nil {
				L.Errorf("cannot open file '%s' (%s)", filename, getErrnoMsg(err))
				return 0
			}
			stream.f = f
		} else {
			toFile(L, 1) // validate it's an open file
			L.PushValue(1)
		}
		L.SetField(luaapi.RegistryIndex, regKey)
	}
	// Return current value
	L.GetField(luaapi.RegistryIndex, regKey)
	return 1
}

func openFlags(mode string) int {
	switch mode {
	case "r":
		return os.O_RDONLY
	case "w":
		return os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	default:
		return os.O_RDONLY
	}
}

func ioInputFn(L *luaapi.State) int {
	return gIOFile(L, ioInput, "r")
}

func ioOutputFn(L *luaapi.State) int {
	return gIOFile(L, ioOutput, "w")
}

// ---------------------------------------------------------------------------
// io.tmpfile()
// ---------------------------------------------------------------------------

func ioTmpfile(L *luaapi.State) int {
	stream := newFileUD(L)
	stream.closef = ioFclose
	f, err := os.CreateTemp("", "lua_tmpfile_")
	if err != nil {
		L.Pop(1)
		return pushFileResult(L, false, "", err)
	}
	// Remove the file so it's deleted when closed (Unix behavior)
	os.Remove(f.Name())
	stream.f = f
	return 1
}

// ---------------------------------------------------------------------------
// io.flush()
// ---------------------------------------------------------------------------

func ioFlush(L *luaapi.State) int {
	f := getIOFile(L, ioOutput)
	err := f.Sync()
	return pushFileResult(L, err == nil, "", err)
}

func fFlush(L *luaapi.State) int {
	f := toFile(L, 1)
	err := f.Sync()
	return pushFileResult(L, err == nil, "", err)
}

// ---------------------------------------------------------------------------
// f:seek([whence [, offset]])
// ---------------------------------------------------------------------------

func fSeek(L *luaapi.State) int {
	f := toFile(L, 1)
	whenceNames := []string{"set", "cur", "end"}
	whenceValues := []int{io.SeekStart, io.SeekCurrent, io.SeekEnd}
	op := L.CheckOption(2, "cur", whenceNames)
	offset := L.OptInteger(3, 0)

	pos, err := f.Seek(offset, whenceValues[op])
	if err != nil {
		return pushFileResult(L, false, "", err)
	}
	L.PushInteger(pos)
	return 1
}

// ---------------------------------------------------------------------------
// f:setvbuf(mode [, size])
// ---------------------------------------------------------------------------

func fSetvbuf(L *luaapi.State) int {
	toFile(L, 1) // validate it's open
	// Go doesn't have direct control over buffering like C's setvbuf.
	// We accept the call but it's a no-op (files are unbuffered in Go).
	modeNames := []string{"no", "full", "line"}
	L.CheckOption(2, "", modeNames)
	// Just return success
	L.PushBoolean(true)
	return 1
}

// ---------------------------------------------------------------------------
// READ operations
// ---------------------------------------------------------------------------

const maxLenNum = 200 // maximum length of a numeral being read

// gRead is the core read function (mirrors g_read in liolib.c).
// first is the stack index of the first format argument.
func gRead(L *luaapi.State, f *os.File, first int) int {
	nargs := L.GetTop() - first + 1
	if nargs == 0 {
		// No arguments: read a line (chopping newline)
		ok := readLine(L, f, true)
		if !ok {
			L.Pop(1) // remove result
			L.PushFail()
		}
		return 1
	}

	n := first
	success := true
	for i := 0; i < nargs && success; i++ {
		if L.Type(n) == objectapi.TypeNumber {
			count := L.CheckInteger(n)
			if count == 0 {
				success = testEOF(L, f)
			} else {
				success = readChars(L, f, int(count))
			}
		} else {
			p := L.CheckString(n)
			if len(p) > 0 && p[0] == '*' {
				p = p[1:] // skip optional '*'
			}
			if len(p) == 0 {
				L.ArgError(n, "invalid format")
				return 0
			}
			switch p[0] {
			case 'n':
				success = readNumber(L, f)
			case 'l':
				success = readLine(L, f, true) // chop newline
			case 'L':
				success = readLine(L, f, false) // keep newline
			case 'a':
				readAll(L, f)
				success = true // always succeeds
			default:
				L.ArgError(n, "invalid format")
				return 0
			}
		}
		n++
	}

	// Check for file error
	// (Go doesn't have ferror equivalent directly, but read errors
	// would have been caught in individual read functions)

	if !success {
		L.Pop(1)     // remove last result
		L.PushFail() // push nil instead
	}
	return n - first
}

// testEOF tests if we're at EOF. Pushes "" if not at EOF.
func testEOF(L *luaapi.State, f *os.File) bool {
	buf := make([]byte, 1)
	n, err := f.Read(buf)
	if n > 0 {
		// Unread the byte
		f.Seek(-1, io.SeekCurrent)
		L.PushString("")
		return true
	}
	if err == io.EOF {
		L.PushString("")
		return false
	}
	L.PushString("")
	return false
}

// readLine reads a line from f. If chop is true, strips the trailing newline.
func readLine(L *luaapi.State, f *os.File, chop bool) bool {
	var buf strings.Builder
	b := make([]byte, 1)
	for {
		n, err := f.Read(b)
		if n > 0 {
			if b[0] == '\n' {
				if !chop {
					buf.WriteByte('\n')
				}
				L.PushString(buf.String())
				return true
			}
			buf.WriteByte(b[0])
		}
		if err != nil {
			break
		}
	}
	// EOF or error
	s := buf.String()
	L.PushString(s)
	return len(s) > 0
}

// readAll reads the entire remaining file content.
func readAll(L *luaapi.State, f *os.File) {
	data, _ := io.ReadAll(f)
	L.PushString(string(data))
}

// readChars reads exactly n bytes from f.
func readChars(L *luaapi.State, f *os.File, n int) bool {
	buf := make([]byte, n)
	total := 0
	for total < n {
		nr, err := f.Read(buf[total:])
		total += nr
		if err != nil {
			break
		}
	}
	if total > 0 {
		L.PushString(string(buf[:total]))
		return true
	}
	L.PushString("")
	return false
}

// readNumber reads a number from the file (mirrors read_number in liolib.c).
// Uses a state machine to parse the number prefix, then lua_stringtonumber.
func readNumber(L *luaapi.State, f *os.File) bool {
	var rn rnState
	rn.f = f
	rn.n = 0

	// Read first non-space character
	rn.c = rnGetc(&rn)
	for rn.c >= 0 && isSpace(byte(rn.c)) {
		rn.c = rnGetc(&rn)
	}

	// Optional sign
	rnTest2(&rn, '-', '+')

	// Check for hex prefix
	hex := false
	count := 0
	if rn.c == '0' {
		if rnNextc(&rn) {
			if rn.c == 'x' || rn.c == 'X' {
				hex = true
				rnNextc(&rn)
			} else {
				count = 1 // count initial '0'
			}
		}
	}

	// Integral part
	count += rnReadDigits(&rn, hex)

	// Decimal point
	if rn.c == '.' {
		if rnNextc(&rn) {
			count += rnReadDigits(&rn, hex)
		}
	}

	// Exponent
	if count > 0 {
		if hex {
			if rn.c == 'p' || rn.c == 'P' {
				if rnNextc(&rn) {
					rnTest2(&rn, '-', '+')
					rnReadDigits(&rn, false)
				}
			}
		} else {
			if rn.c == 'e' || rn.c == 'E' {
				if rnNextc(&rn) {
					rnTest2(&rn, '-', '+')
					rnReadDigits(&rn, false)
				}
			}
		}
	}

	// Unread lookahead char
	if rn.c >= 0 {
		f.Seek(-1, io.SeekCurrent)
	}

	// Try to convert
	s := string(rn.buff[:rn.n])
	if L.StringToNumber(s) > 0 {
		return true
	}
	// Failed — push nil
	L.PushNil()
	return false
}

// rnState is the state for reading a number (mirrors RN in liolib.c).
type rnState struct {
	f    *os.File
	c    int // current character (lookahead), -1 = EOF
	n    int
	buff [maxLenNum + 1]byte
}

func rnGetc(rn *rnState) int {
	var b [1]byte
	n, err := rn.f.Read(b[:])
	if n > 0 {
		return int(b[0])
	}
	if err != nil {
		return -1
	}
	return -1
}

func rnNextc(rn *rnState) bool {
	if rn.n >= maxLenNum {
		rn.buff[0] = 0 // invalidate
		return false
	}
	rn.buff[rn.n] = byte(rn.c)
	rn.n++
	rn.c = rnGetc(rn)
	return true
}

func rnTest2(rn *rnState, a, b byte) bool {
	if rn.c == int(a) || rn.c == int(b) {
		return rnNextc(rn)
	}
	return false
}

func rnReadDigits(rn *rnState, hex bool) int {
	count := 0
	for rn.c >= 0 {
		if hex {
			if !isXDigit(byte(rn.c)) {
				break
			}
		} else {
			if !isDigit(byte(rn.c)) {
				break
			}
		}
		if !rnNextc(rn) {
			break
		}
		count++
	}
	return count
}

func isSpace(c byte) bool  { return unicode.IsSpace(rune(c)) }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isXDigit(c byte) bool { return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }

// ---------------------------------------------------------------------------
// WRITE operations
// ---------------------------------------------------------------------------

// gWrite is the core write function (mirrors g_write in liolib.c).
func gWrite(L *luaapi.State, f *os.File, arg int) int {
	nargs := L.GetTop() - arg // matches C Lua: excludes file handle at top
	for i := 0; i < nargs; i++ {
		idx := arg + i
		if L.Type(idx) == objectapi.TypeNumber {
			// Format number as string (like C Lua's lua_numbertocstring)
			if v, ok := L.ToInteger(idx); ok && float64(v) == func() float64 { f, _ := L.ToNumber(idx); return f }() {
				s := fmt.Sprintf("%d", v)
				_, err := f.WriteString(s)
				if err != nil {
					return pushFileResult(L, false, "", err)
				}
			} else {
				v, _ := L.ToNumber(idx)
				s := fmt.Sprintf("%.14g", v)
				_, err := f.WriteString(s)
				if err != nil {
					return pushFileResult(L, false, "", err)
				}
			}
		} else {
			s := L.CheckString(idx)
			_, err := f.WriteString(s)
			if err != nil {
				return pushFileResult(L, false, "", err)
			}
		}
	}
	// File handle is already at stack top (pushed by caller)
	return 1
}

// io.read(...)
func ioRead(L *luaapi.State) int {
	return gRead(L, getIOFile(L, ioInput), 1)
}

// f:read(...)
func fRead(L *luaapi.State) int {
	return gRead(L, toFile(L, 1), 2)
}

// io.write(...)
// io.write(...) — matches C Lua's io_write
func ioWrite(L *luaapi.State) int {
	// getIOFile pushes the file handle userdata to the stack top and leaves it there
	L.GetField(luaapi.RegistryIndex, ioOutput) // push file handle (for return)
	ud := L.GetUserdataObj(-1)
	if ud == nil {
		L.Errorf("default %s file is closed", "output")
		return 0
	}
	s, ok := ud.Data.(*ioStream)
	if !ok || s.isClosed() {
		L.Errorf("default %s file is closed", "output")
		return 0
	}
	// Stack: [arg1, arg2, ..., filehandle]
	// gWrite processes args 1..N (the original args), file handle stays at top
	return gWrite(L, s.f, 1)
}

// f:write(...) — matches C Lua's f_write
func fWrite(L *luaapi.State) int {
	f := toFile(L, 1)
	L.PushValue(1) // push copy of file handle to top (for return)
	// Stack: [filehandle, arg2, arg3, ..., filehandle_copy]
	return gWrite(L, f, 2)
}

// ---------------------------------------------------------------------------
// io.lines([filename, ...]) / f:lines(...)
// ---------------------------------------------------------------------------

// MAXARGLINE is the maximum number of format arguments to lines.
const maxArgLine = 250

// auxLines creates the iteration function closure for lines.
// Stack: [file, fmt1, fmt2, ...] where file is at position 1.
// toclose: whether to close the file when iteration ends.
func auxLines(L *luaapi.State, toclose bool) {
	n := L.GetTop() - 1 // number of format arguments
	if n > maxArgLine {
		L.ArgError(maxArgLine+2, "too many arguments")
	}
	// Upvalues: file, n, toclose, fmt1, fmt2, ...
	L.PushValue(1)                  // upvalue 1: file
	L.PushInteger(int64(n))         // upvalue 2: number of formats
	L.PushBoolean(toclose)          // upvalue 3: close when done?
	// Rotate: move the 3 upvalues before the format args
	// Stack before rotate: [file, fmt1..fmtN, file, n, toclose]
	// We need: [file, file, n, toclose, fmt1..fmtN]
	L.Rotate(2, 3) // rotate positions 2..top by 3
	L.PushCClosure(ioReadline, 3+n)
}

// ioReadline is the iterator function for io.lines/f:lines.
func ioReadline(L *luaapi.State) int {
	ud := L.GetUserdataObj(luaapi.UpvalueIndex(1))
	if ud == nil {
		L.Errorf("file is already closed")
		return 0
	}
	s, ok := ud.Data.(*ioStream)
	if !ok || s.isClosed() {
		L.Errorf("file is already closed")
		return 0
	}

	n := int(toIntegerFromIdx(L, luaapi.UpvalueIndex(2)))

	L.SetTop(1)
	if n == 0 {
		// Default: read one line ("l" format)
		L.PushString("l")
		n = 1
	} else {
		// Push format arguments from upvalues
		for i := 1; i <= n; i++ {
			L.PushValue(luaapi.UpvalueIndex(3 + i))
		}
	}

	nResults := gRead(L, s.f, 2)
	if nResults > 0 && L.ToBoolean(-nResults) {
		return nResults
	}

	// EOF or error
	if nResults > 1 {
		// Error message at -nResults+1
		errMsg, _ := L.ToString(-nResults + 1)
		L.Errorf("%s", errMsg)
		return 0
	}

	// Close file if toclose
	if L.ToBoolean(luaapi.UpvalueIndex(3)) {
		L.SetTop(0)
		L.PushValue(luaapi.UpvalueIndex(1))
		auxClose(L)
	}
	return 0
}

// f:lines(...)
func fLines(L *luaapi.State) int {
	toFile(L, 1) // validate it's open
	auxLines(L, false)
	return 1
}

// io.lines([filename, ...])
func ioLines(L *luaapi.State) int {
	if L.IsNone(1) {
		L.PushNil()
	}
	if L.IsNil(1) {
		// No filename: use default input
		L.GetField(luaapi.RegistryIndex, ioInput)
		L.Replace(1)
		toFile(L, 1) // validate
		auxLines(L, false)
		return 1
	}

	// Open a new file
	filename := L.CheckString(1)
	stream := newFileUD(L)
	stream.closef = ioFclose
	f, err := os.Open(filename)
	if err != nil {
		L.Errorf("cannot open file '%s' (%s)", filename, getErrnoMsg(err))
		return 0
	}
	stream.f = f
	L.Replace(1) // replace filename with file handle
	auxLines(L, true)

	// For to-be-closed: return iterator, nil, nil, file
	if true {
		L.PushNil()  // state
		L.PushNil()  // control
		L.PushValue(1) // file as to-be-closed variable
		return 4
	}
	return 1
}

// ---------------------------------------------------------------------------
// Helper: ToIntegerX for upvalue reading
// ---------------------------------------------------------------------------

// toIntegerFromIdx gets an integer from a stack index, returning 0 if not an integer.
func toIntegerFromIdx(L *luaapi.State, idx int) int64 {
	v, ok := L.ToInteger(idx)
	if ok {
		return v
	}
	return 0
}

// Removed — this is defined on *State, can't add methods in a different package.
// We'll use a package-level helper instead.

// ---------------------------------------------------------------------------
// Unused import prevention
// ---------------------------------------------------------------------------
var _ = bufio.NewReader
