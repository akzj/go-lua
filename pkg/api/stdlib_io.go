// Package api provides the public Lua API
// This file implements the io standard library module
package api

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/akzj/go-lua/pkg/object"
)

// FileHandle represents a Lua file handle
type FileHandle struct {
	file   *os.File
	reader *bufio.Reader
	closed bool
}

// openIOLib registers the io module
func (s *State) openIOLib() {
	// Create module table
	s.NewTable()
	tableIdx := s.GetTop()

	// Register functions
	funcs := map[string]Function{
		"open":   stdIOOpen,
		"lines":  stdIOLines,
		"input":  stdIOInput,
		"output": stdIOOutput,
	}

	for name, fn := range funcs {
		s.PushFunction(fn)
		s.SetField(tableIdx, name)
	}

	// Store default input/output files
	// stdin
	s.NewTable()
	stdinIdx := s.GetTop()
	s.PushFunction(stdIOStdinRead)
	s.SetField(stdinIdx, "read")
	s.PushFunction(stdIOStdinClose)
	s.SetField(stdinIdx, "close")
	s.SetField(tableIdx, "stdin")

	// stdout
	s.NewTable()
	stdoutIdx := s.GetTop()
	s.PushFunction(stdIOStdoutWrite)
	s.SetField(stdoutIdx, "write")
	s.PushFunction(stdIOStdoutClose)
	s.SetField(stdoutIdx, "close")
	s.SetField(tableIdx, "stdout")

	// stderr
	s.NewTable()
	stderrIdx := s.GetTop()
	s.PushFunction(stdIOStderrWrite)
	s.SetField(stderrIdx, "write")
	s.PushFunction(stdIOStderrClose)
	s.SetField(stderrIdx, "close")
	s.SetField(tableIdx, "stderr")

	// Register as global
	s.SetGlobal("io")
}

// stdIOOpen implements io.open(filename [, mode])
// Opens a file and returns a file handle, or nil + error message on failure.
func stdIOOpen(L *State) int {
	filename, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		L.PushString("io.open: filename expected")
		return 2
	}

	// Get mode (default: "r")
	mode := "r"
	if L.GetTop() >= 2 {
		mode, _ = L.ToString(2)
	}

	// Open file
	file, err := os.OpenFile(filename, parseMode(mode), 0644)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	// Create file handle
	fh := &FileHandle{
		file:   file,
		reader: bufio.NewReader(file),
		closed: false,
	}

	// Create userdata-like table with methods
	L.NewTable()
	tableIdx := L.GetTop()

	// Store the file handle as a light userdata
	L.PushLightUserData(fh)
	L.SetField(tableIdx, "_handle")

	// Add methods
	L.PushFunction(stdIOFileRead)
	L.SetField(tableIdx, "read")
	L.PushFunction(stdIOFileWrite)
	L.SetField(tableIdx, "write")
	L.PushFunction(stdIOFileClose)
	L.SetField(tableIdx, "close")
	L.PushFunction(stdIOFileLines)
	L.SetField(tableIdx, "lines")

	return 1
}

// parseMode converts Lua mode string to os.OpenFile flags
func parseMode(mode string) int {
	switch mode {
	case "r":
		return os.O_RDONLY
	case "w":
		return os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case "a":
		return os.O_WRONLY | os.O_CREATE | os.O_APPEND
	case "r+":
		return os.O_RDWR
	case "w+":
		return os.O_RDWR | os.O_CREATE | os.O_TRUNC
	case "a+":
		return os.O_RDWR | os.O_CREATE | os.O_APPEND
	default:
		return os.O_RDONLY
	}
}

// getFileHandle extracts FileHandle from table at index
func getFileHandle(L *State, idx int) *FileHandle {
	// Get the _handle field
	tVal := L.vm.GetStack(idx)
	if !tVal.IsTable() {
		return nil
	}

	t, _ := tVal.ToTable()
	key := object.TValue{Type: object.TypeString, Value: object.Value{Str: "_handle"}}
	handleVal := t.Get(key)

	if handleVal == nil || handleVal.Type != object.TypeLightUserData {
		return nil
	}

	fh, ok := handleVal.Value.Ptr.(*FileHandle)
	if !ok {
		return nil
	}
	return fh
}

// stdIOFileRead implements file:read(...)
// Reads from file according to format strings.
func stdIOFileRead(L *State) int {
	fh := getFileHandle(L, 1)
	if fh == nil || fh.closed {
		L.PushNil()
		L.PushString("file is closed")
		return 2
	}

	top := L.GetTop()
	if top == 1 {
		// No format specified, read line
		line, err := fh.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			L.PushNil()
			L.PushString(err.Error())
			return 2
		}
		// If we got content (even with EOF), return it
		if len(line) > 0 {
			// Remove trailing newline
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			L.PushString(line)
			return 1
		}
		// No content and EOF
		L.PushNil()
		return 1
	}

	// Process format arguments
	results := 0
	for i := 2; i <= top; i++ {
		format, ok := L.ToString(i)
		if !ok {
			continue
		}

		switch format {
		case "*l", "*L":
			// Read line
			line, err := fh.reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					L.PushNil()
				} else {
					L.PushNil()
					return results + 1
				}
			} else {
				if format == "*l" && len(line) > 0 && line[len(line)-1] == '\n' {
					line = line[:len(line)-1]
				}
				L.PushString(line)
			}
			results++

		case "*a", "*A":
			// Read all
			data, err := io.ReadAll(fh.reader)
			if err != nil {
				L.PushNil()
			} else {
				L.PushString(string(data))
			}
			results++

		case "*n":
			// Read number
			var num float64
			_, err := fmt.Fscanf(fh.reader, "%f", &num)
			if err != nil {
				L.PushNil()
			} else {
				L.PushNumber(num)
			}
			results++

		default:
			// Read n bytes
			var n int
			fmt.Sscanf(format, "%d", &n)
			if n > 0 {
				buf := make([]byte, n)
				bytesRead, err := fh.reader.Read(buf)
				if err != nil {
					L.PushNil()
				} else {
					L.PushString(string(buf[:bytesRead]))
				}
				results++
			}
		}
	}

	return results
}

// stdIOFileWrite implements file:write(...)
// Writes values to file.
func stdIOFileWrite(L *State) int {
	fh := getFileHandle(L, 1)
	if fh == nil || fh.closed {
		L.PushNil()
		L.PushString("file is closed")
		return 2
	}

	top := L.GetTop()
	for i := 2; i <= top; i++ {
		str, ok := L.ToString(i)
		if !ok {
			L.PushNil()
			L.PushString("string expected")
			return 2
		}

		_, err := fh.file.WriteString(str)
		if err != nil {
			L.PushNil()
			L.PushString(err.Error())
			return 2
		}
	}

	L.PushBoolean(true)
	return 1
}

// stdIOFileClose implements file:close()
// Closes the file.
func stdIOFileClose(L *State) int {
	fh := getFileHandle(L, 1)
	if fh == nil || fh.closed {
		L.PushBoolean(true)
		return 1
	}

	err := fh.file.Close()
	fh.closed = true

	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	L.PushBoolean(true)
	return 1
}

// stdIOFileLines implements file:lines()
// Returns an iterator for lines.
func stdIOFileLines(L *State) int {
	fh := getFileHandle(L, 1)
	if fh == nil || fh.closed {
		L.PushNil()
		return 1
	}

	// Create iterator function
	L.PushFunction(func(L *State) int {
		if fh.closed {
			return 0
		}

		line, err := fh.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fh.closed = true
				return 0
			}
			return 0
		}

		// Remove trailing newline
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		L.PushString(line)
		return 1
	})

	return 1
}

// stdIOLines implements io.lines(filename)
// Returns an iterator for lines in a file.
func stdIOLines(L *State) int {
	filename, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	file, err := os.Open(filename)
	if err != nil {
		L.PushNil()
		return 1
	}

	reader := bufio.NewReader(file)

	// Create iterator function
	L.PushFunction(func(L *State) int {
		line, err := reader.ReadString('\n')
		if err != nil {
			file.Close()
			return 0
		}

		// Remove trailing newline
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		L.PushString(line)
		return 1
	})

	return 1
}

// stdIOInput implements io.input([file])
// Gets or sets default input file.
func stdIOInput(L *State) int {
	// TODO: Implement default input file management
	// For now, just return stdin
	L.GetGlobal("io")
	L.GetField(-1, "stdin")
	return 1
}

// stdIOOutput implements io.output([file])
// Gets or sets default output file.
func stdIOOutput(L *State) int {
	// TODO: Implement default output file management
	// For now, just return stdout
	L.GetGlobal("io")
	L.GetField(-1, "stdout")
	return 1
}

// Standard file functions (stdin, stdout, stderr)

func stdIOStdinRead(L *State) int {
	reader := bufio.NewReader(os.Stdin)
	top := L.GetTop()

	if top == 0 {
		// Read line
		line, err := reader.ReadString('\n')
		if err != nil {
			L.PushNil()
			return 1
		}
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		L.PushString(line)
		return 1
	}

	// Process format arguments (similar to file:read)
	return 0 // TODO: implement
}

func stdIOStdinClose(L *State) int {
	L.PushBoolean(true)
	return 1
}

func stdIOStdoutWrite(L *State) int {
	top := L.GetTop()
	for i := 1; i <= top; i++ {
		str, ok := L.ToString(i)
		if ok {
			fmt.Fprint(os.Stdout, str)
		}
	}
	L.PushBoolean(true)
	return 1
}

func stdIOStdoutClose(L *State) int {
	L.PushBoolean(true)
	return 1
}

func stdIOStderrWrite(L *State) int {
	top := L.GetTop()
	for i := 1; i <= top; i++ {
		str, ok := L.ToString(i)
		if ok {
			fmt.Fprint(os.Stderr, str)
		}
	}
	L.PushBoolean(true)
	return 1
}

func stdIOStderrClose(L *State) int {
	L.PushBoolean(true)
	return 1
}