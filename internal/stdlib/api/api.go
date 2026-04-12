// Package api defines the interface for Lua standard library registration.
//
// Each library is a Go function that registers Lua functions into a state.
// Libraries use the public Go API (internal/api/) to interact with the VM.
//
// Reference: .analysis/09-standard-libraries.md
package api

import (
	luaapi "github.com/akzj/go-lua/internal/api/api"
)

// OpenFunc is the type for library opener functions.
// Each takes a State and registers its functions. Returns 1 (the library table).
type OpenFunc func(L *luaapi.State) int

// Library describes a standard library for registration.
type Library struct {
	Name string   // library name (e.g., "string", "table")
	Open OpenFunc // opener function
}

// StandardLibraries returns the list of all standard libraries in load order.
// Order matters: base must be first (it defines _G and core functions).
func StandardLibraries() []Library {
	return []Library{
		{"_G", OpenBase},
		{"package", OpenPackage},
		{"coroutine", OpenCoroutine},
		{"debug", OpenDebug},
		{"io", OpenIO},
		{"math", OpenMath},
		{"os", OpenOS},
		{"string", OpenString},
		{"table", OpenTable},
		{"utf8", OpenUTF8},
	}
}

// --- Individual Library Openers ---
// Each function creates a library table, registers functions, and returns 1.

// OpenBase opens the base library (_G): print, type, pairs, ipairs,
// pcall, xpcall, error, assert, tostring, tonumber, rawget, rawset,
// rawequal, rawlen, select, next, setmetatable, getmetatable, load,
// dofile, loadfile, collectgarbage, warn.
func OpenBase(L *luaapi.State) int { return 0 }

// OpenString opens the string library: string.byte, char, find, format,
// gmatch, gsub, len, lower, upper, match, rep, reverse, sub, pack,
// unpack, packsize, dump.
func OpenString(L *luaapi.State) int { return 0 }

// OpenTable opens the table library: table.insert, remove, sort, concat,
// move, pack, unpack.
func OpenTable(L *luaapi.State) int { return 0 }

// OpenMath opens the math library: math.abs, ceil, floor, sqrt, sin, cos,
// tan, asin, acos, atan, exp, log, max, min, fmod, random, randomseed,
// tointeger, type, huge, maxinteger, mininteger, pi.
func OpenMath(L *luaapi.State) int { return 0 }

// OpenIO opens the io library: io.open, close, read, write, lines,
// input, output, tmpfile, type, flush, popen.
func OpenIO(L *luaapi.State) int { return 0 }

// OpenOS opens the os library: os.clock, date, difftime, execute, exit,
// getenv, remove, rename, time, tmpname.
func OpenOS(L *luaapi.State) int { return 0 }

// OpenDebug opens the debug library: debug.getinfo, getlocal, setlocal,
// getmetatable, setmetatable, getupvalue, setupvalue, getuservalue,
// setuservalue, sethook, gethook, traceback, upvalueid, upvaluejoin.
func OpenDebug(L *luaapi.State) int { return 0 }

// OpenUTF8 opens the utf8 library: utf8.char, codepoint, codes, len,
// offset, charpattern.
func OpenUTF8(L *luaapi.State) int { return 0 }

// OpenCoroutine opens the coroutine library: coroutine.create, resume,
// yield, status, wrap, isyieldable, close, running.
func OpenCoroutine(L *luaapi.State) int { return 0 }

// OpenPackage opens the package library: require, package.path,
// package.cpath, package.loaded, package.preload, package.searchers,
// package.searchpath, package.config.
func OpenPackage(L *luaapi.State) int { return 0 }

// OpenAll opens all standard libraries. Convenience function.
func OpenAll(L *luaapi.State) {
	for _, lib := range StandardLibraries() {
		L.Require(lib.Name, luaapi.CFunction(lib.Open), true)
	}
}
