// Package api defines the Lua I/O library interface.
// No implementation details - only interfaces.
//
// Reference: lua-master/liolib.c (I/O library)
//
// Constraint: must NOT import internal/ packages to avoid circular dependency.
package api

import (
	"github.com/akzj/go-lua/api"
	types "github.com/akzj/go-lua/types/api"
)

// LuaAPI mirrors the Lua API interface from api/api package.
type LuaAPI = api.LuaAPI

// LuaInteger matches types.LuaInteger.
type LuaInteger = types.LuaInteger

// LuaNumber matches types.LuaNumber.
type LuaNumber = types.LuaNumber

// =============================================================================
// I/O Library Interface
// =============================================================================

// IoLib provides Lua I/O library functions (io.open, io.read, etc.).
//
// Invariants:
// - Open() registers functions in the global table under "io"
// - Returns 1 (number of values pushed on success), per luaopen_* convention
// - Manages default input/output files via registry keys "_IO_input" and "_IO_output"
//
// Design:
// - Uses Go function types directly (not CFunction/unsafe.Pointer)
// - Each function receives LuaAPI for stack access
// - Returns int (number of values pushed)
// - File handles are userdata with metatable "FILE*"
type IoLib interface {
	// Open opens the I/O library, registering its functions.
	// L: the Lua state to operate on
	// Returns: number of values pushed onto the stack (always 1 = the module table)
	//
	// Side effects: sets global variable "io" with all I/O functions
	Open(L LuaAPI) int
}

// LuaFunc is the type for Lua Go functions.
// Matches the signature used in lua.h: typedef int (*lua_CFunction)(lua_State*).
//
// Why not use types.CFunction?
// - types.CFunction is unsafe.Pointer (FFI style)
// - LuaFunc is a proper Go function type for this implementation
type LuaFunc func(L LuaAPI) int

// =============================================================================
// File Handle Types
// =============================================================================

// FileHandle represents an open file in Lua.
// Wraps an *os.File and a close function.
//
// Invariants:
// - If CloseF is nil, the file is considered closed
// - File should not be nil when CloseF is non-nil
//
// Design:
// - CloseF allows custom close behavior (e.g., for popen)
// - This mirrors luaL_Stream from liolib.c
type FileHandle struct {
	// File is the underlying file. May be nil if closed.
	File *File
	// CloseF is the close function. If nil, file is closed.
	CloseF func(h *FileHandle) error
}

// File is the interface for file operations.
// Abstracts *os.File for testability.
//
// Why an interface?
// - Allows mocking in tests
// - Matches the FILE* abstraction from C
type File interface {
	// Read reads up to len(b) bytes into b.
	Read(b []byte) (n int, err error)
	// Write writes len(b) bytes from b.
	Write(b []byte) (n int, err error)
	// Seek sets the offset for the next Read or Write.
	Seek(offset int64, whence int) (int64, error)
	// Close closes the file.
	Close() error
	// Flush flushes buffered data.
	Flush() error
}

// LUA_FILEHANDLE is the metatable name for file handles.
// Mirrors LUA_FILEHANDLE from lauxlib.h.
const LUA_FILEHANDLE = "FILE*"

// Registry keys for default input/output files.
// Mirrors IO_INPUT and IO_OUTPUT from liolib.c.
const (
	IO_INPUT  = "_IO_input"
	IO_OUTPUT = "_IO_output"
)
