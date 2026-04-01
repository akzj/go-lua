// Package api defines the Lua OS library interface.
// No implementation details - only interfaces.
//
// Reference: lua-master/loslib.c
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
// OS Library Interface
// =============================================================================

// OSLib provides Lua OS library functions (os.date, os.time, etc.).
//
// Invariants:
// - Open() registers functions in the global table under "os"
// - Returns 1 (number of values pushed on success), per luaopen_* convention
//
// Design:
// - Uses Go function types directly (not CFunction/unsafe.Pointer)
// - Each function receives LuaAPI for stack access
// - Returns int (number of values pushed)
type OSLib interface {
	// Open opens the OS library, registering its functions.
	// L: the Lua state to operate on
	// Returns: number of values pushed onto the stack (always 1 = the module table)
	//
	// Side effects: sets global variable "os" with all OS functions
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
// Locale Categories
// =============================================================================
// These constants match Lua 5.5.1 locale categories.
// Why not strings? Constants provide compile-time safety and match C API.

const (
	LOCALE_ALL       = 0
	LOCALE_COLLATE   = 1
	LOCALE_CTYPE     = 2
	LOCALE_MONETARY  = 3
	LOCALE_NUMERIC   = 4
	LOCALE_TIME      = 5
	LOCALE_CHARCLASS = LOCALE_CTYPE
)

// =============================================================================
// OS Function Signatures
// =============================================================================
// All functions follow the pattern: func(L LuaAPI) int returning number of
// results pushed onto the stack.
//
// Pushing results:
// - For timestamp results: use L.PushInteger(int64(seconds))
// - For strings: use L.PushString(s)
// - For boolean: use L.PushBoolean(b)
// - For nil: use L.PushNil()
//
// os.clock() - Returns CPU time used by the program in seconds.
// Returns: number (seconds of CPU time)
//
// os.date([format [, time]]) - Returns current date/time.
// format: string with format specifiers (see Lua 5.5.1 reference)
//   %a - abbreviated weekday name
//   %A - full weekday name
//   %b - abbreviated month name
//   %B - full month name
//   %c - date and time
//   %d - day of month [01-31]
//   %H - hour [00-23]
//   %I - hour [01-12]
//   %j - day of year [001-366]
//   %m - month [01-12]
//   %M - minute [00-59]
//   %p - AM/PM
//   %S - second [00-59]
//   %U - week number [00-53] (Sunday first)
//   %w - weekday [0-6] (Sunday = 0)
//   %W - week number [00-53] (Monday first)
//   %x - date
//   %X - time
//   %y - year [00-99]
//   %Y - year (full)
//   %% - literal '%'
//   %G - ISO 8601 year
//   %g - ISO 8601 year (2 digits)
//   %u - ISO 8601 weekday (1-7, Monday=1)
//   %V - ISO 8601 week number
// time: optional timestamp (defaults to current time)
// Returns: string (if format starts with '!') or table
//
// Why os.date is special:
// - Lua date format differs from Go (strftime)
// - Must map %p (AM/PM), %I (12-hour), %V (ISO week) correctly
// - If format starts with '!', use UTC timezone
//
// os.difftime(t1, t2) - Returns difference between two times in seconds.
// t1, t2: timestamps from os.time()
// Returns: number (t1 - t2 in seconds)
//
// os.execute([command]) - Executes shell command.
// command: shell command string (nil/none returns nil)
// Returns: true/"exit", exitcode if successful, nil+error message if failed
//
// os.exit([code [, close]]) - Terminates the VM.
// code: exit code (true=0, string=1, number=used as exit code, default=0)
// close: if true, close the Lua state before exiting
// Returns: nothing (typically doesn't return)
//
// Why not actually exit in Go?
// - os.exit in Go terminates the entire process
// - Library should return error/close status to VM instead
// - The VM's main loop handles actual process exit
//
// os.getenv(varname) - Returns environment variable value.
// varname: name of environment variable
// Returns: string value, or nil if not set
//
// os.remove(filename) - Deletes file or directory.
// filename: path to file
// Returns: true on success, nil+error message on failure
//
// os.rename(oldname, newname) - Renames/moves file.
// oldname: current file path
// newname: new file path
// Returns: true on success, nil+error message on failure
//
// os.setlocale(locale [, category]) - Sets locale.
// locale: locale name (e.g., "C", "en_US.UTF-8", "")
// category: locale category (default: all)
// Returns: new locale string, or nil if failed
//
// os.time([table]) - Returns current timestamp or timestamp from table.
// table: optional table with fields: year, month, day, hour, min, sec, isdst
// Returns: timestamp (seconds since epoch), or nil if invalid
//
// os.tmpname() - Returns a temporary file name.
// Returns: string (temporary file path)
// WARNING: Always use os.tmpname() + os.remove() together to avoid leaks
