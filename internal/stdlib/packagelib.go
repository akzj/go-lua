package stdlib

// Package library searcher functions for Lua 5.5 require() architecture.
//
// package.searchers is a table of searcher functions. Each searcher receives
// a module name and returns either:
//   - a loader function + an extra value (e.g. filename), or
//   - a string describing why it couldn't find the module.
//
// require() iterates through package.searchers, calling each one until
// a loader is found.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	luaapi "github.com/akzj/go-lua/internal/api"
	"github.com/akzj/go-lua/internal/object"
)

// searcher_preload — searcher #1: checks package.preload[modname]
// Returns the preload function + ":preload:" if found, or an error string.
func searcher_preload(L *luaapi.State) int {
	name := L.CheckString(1)
	L.GetGlobal("package")
	if L.IsNil(-1) {
		L.Pop(1)
		L.PushString("no field package.preload['" + name + "']")
		return 1
	}
	tp := L.GetField(-1, "preload")
	if tp != object.TypeTable {
		L.Pop(2) // pop preload + package
		L.PushString("no field package.preload['" + name + "']")
		return 1
	}
	tp = L.GetField(-1, name)
	if tp == object.TypeNil {
		// Not found in preload
		L.Pop(3) // pop nil + preload + package
		L.PushString("no field package.preload['" + name + "']")
		return 1
	}
	// Found: return the preload function + ":preload:" extra
	L.Remove(-2) // remove preload table
	L.Remove(-2) // remove package table, keep function on top
	L.PushString(":preload:")
	return 2
}

// searcher_Lua — searcher #2: searches package.path for a .lua file
// Returns the loader function + filename, or an error string listing tried paths.
func searcher_Lua(L *luaapi.State) int {
	name := L.CheckString(1)

	// Get package.path
	L.GetGlobal("package")
	if L.IsNil(-1) {
		L.Pop(1)
		L.PushString("no field package.path")
		return 1
	}
	tp := L.GetField(-1, "path")
	if tp != object.TypeString {
		L.Pop(2) // pop path + package
		L.Errorf("'package.path' must be a string")
		return 0
	}
	pathStr, _ := L.ToString(-1)
	L.Pop(2) // pop path + package

	// Search for the module file
	filename, tried := searchPathWithFS(L.FileSystem, name, pathStr)
	if filename == "" {
		// Not found — return error string with all tried paths
		var msg strings.Builder
		for _, t := range tried {
			msg.WriteString("no file '")
			msg.WriteString(t)
			msg.WriteString("'\n\t")
		}
		s := msg.String()
		if len(s) >= 2 {
			s = s[:len(s)-2] // remove trailing "\n\t"
		}
		L.PushString(s)
		return 1
	}

	// Found — load the file and return (loader, filename)
	data, err := readFileWithFS(L.FileSystem, filename)
	if err != nil {
		L.PushString("cannot read '" + filename + "': " + err.Error())
		return 1
	}

	code := string(data)
	// Skip UTF-8 BOM if present
	if strings.HasPrefix(code, "\xEF\xBB\xBF") {
		code = code[3:]
	}
	// Skip shebang line
	if len(code) > 0 && code[0] == '#' {
		idx := strings.IndexByte(code, '\n')
		if idx >= 0 {
			code = code[idx:]
		} else {
			code = ""
		}
	}

	source := "@" + filename
	status := L.Load(code, source, "t")
	if status != luaapi.StatusOK {
		msg, _ := L.ToString(-1)
		L.Errorf("error loading module '%s' from file '%s':\n\t%s", name, filename, msg)
		return 0
	}

	// Return (loader_function, filename)
	L.PushString(filename)
	return 2
}

// searcher_Clib — searcher #3: searches package.cpath for C modules
// Stub: Go can't load C shared libraries, so this just returns an error string.
func searcher_Clib(L *luaapi.State) int {
	name := L.CheckString(1)

	// Get package.cpath
	L.GetGlobal("package")
	if L.IsNil(-1) {
		L.Pop(1)
		L.PushString("no field package.cpath")
		return 1
	}
	tp := L.GetField(-1, "cpath")
	if tp != object.TypeString {
		L.Pop(2)
		L.PushString("no field package.cpath")
		return 1
	}
	cpathStr, _ := L.ToString(-1)
	L.Pop(2) // pop cpath + package

	// Search the cpath to produce the correct error messages
	// (C lib searcher never uses custom FS — it's OS-only)
	_, tried := searchPathForSearcher(name, cpathStr)
	if len(tried) == 0 {
		L.PushString("no file (cpath empty)")
		return 1
	}
	var msg strings.Builder
	for _, t := range tried {
		msg.WriteString("no file '")
		msg.WriteString(t)
		msg.WriteString("'\n\t")
	}
	s := msg.String()
	if len(s) >= 2 {
		s = s[:len(s)-2] // remove trailing "\n\t"
	}
	L.PushString(s)
	return 1
}

// searcher_GoGlobal — searcher #3: checks Go global module registry.
// This searcher is inserted between the Lua file searcher and the C lib
// searcher. It calls back to pkg/lua via the GlobalSearcher function pointer
// on the api.State (avoiding circular imports).
func searcher_GoGlobal(L *luaapi.State) int {
	name := L.CheckString(1)
	if L.GlobalSearcher == nil {
		L.PushString("no global Go module '" + name + "'")
		return 1
	}
	opener := L.GlobalSearcher(name)
	if opener == nil {
		L.PushString("no global Go module '" + name + "'")
		return 1
	}
	L.PushCFunction(opener)
	L.PushString(":gomodule:")
	return 2
}

// searcher_Croot — searcher #5: all-in-one C module searcher
// Stub: returns nil (no error message to add).
func searcher_Croot(L *luaapi.State) int {
	L.PushNil()
	return 1
}

// ---------------------------------------------------------------------------
// File helpers — support custom fs.FS or real OS filesystem
// ---------------------------------------------------------------------------

// readFileWithFS reads a file from the given fs.FS, or from the real OS
// filesystem if fsys is nil. Paths for fs.FS are normalized.
func readFileWithFS(fsys fs.FS, filename string) ([]byte, error) {
	if fsys != nil {
		return fs.ReadFile(fsys, toFSPath(filename))
	}
	return os.ReadFile(filename)
}

// statFileWithFS checks if a file exists in the given fs.FS, or in the real
// OS filesystem if fsys is nil.
func statFileWithFS(fsys fs.FS, filename string) error {
	if fsys != nil {
		_, err := fs.Stat(fsys, toFSPath(filename))
		return err
	}
	_, err := os.Stat(filename)
	return err
}

// toFSPath normalizes a filesystem path for use with fs.FS:
// forward slashes, no leading "./" or "/".
func toFSPath(p string) string {
	p = filepath.ToSlash(p)
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	for strings.HasPrefix(p, "/") {
		p = p[1:]
	}
	if p == "" {
		p = "."
	}
	return p
}

// searchPathWithFS searches pathStr for name, using fsys for file existence
// checks (or the real OS filesystem if fsys is nil).
func searchPathWithFS(fsys fs.FS, name, pathStr string) (string, []string) {
	if pathStr == "" {
		return "", nil
	}
	sep := string(os.PathSeparator)
	if fsys != nil {
		sep = "/" // fs.FS always uses forward slashes
	}
	fname := strings.ReplaceAll(name, ".", sep)
	templates := strings.Split(pathStr, ";")
	var tried []string
	for _, tmpl := range templates {
		tmpl = strings.TrimSpace(tmpl)
		if tmpl == "" {
			continue
		}
		candidate := strings.ReplaceAll(tmpl, "?", fname)
		if err := statFileWithFS(fsys, candidate); err == nil {
			return candidate, nil
		}
		tried = append(tried, candidate)
	}
	return "", tried
}

// searchPathForSearcher searches pathStr (semicolon-separated templates) for name.
// Replaces '.' in name with OS path separator, then '?' in each template with the result.
// Returns (found_file, tried_list). If found, tried_list is nil.
// This is the legacy version that always uses the real OS filesystem.
func searchPathForSearcher(name, pathStr string) (string, []string) {
	return searchPathWithFS(nil, name, pathStr)
}

// OpenPackage opens the "package" library for a Lua state.
func OpenPackage(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{
		"searchpath": pkgSearchPath,
	})

	// Set package.path — default Lua search path
	// "./?.lua" covers the common case of loading from the current directory
	// and the directory of the running script.
	L.PushString("./?.lua;./?/init.lua")
	L.SetField(-2, "path")

	// Set package.cpath — default C library search path
	L.PushString("./?.so")
	L.SetField(-2, "cpath")

	// Set package.loaded = registry["_LOADED"]
	L.GetField(luaapi.RegistryIndex, "_LOADED")
	L.SetField(-2, "loaded")

	// Set package.config (separator, template mark, substitution mark, etc.)
	L.PushString(string(os.PathSeparator) + "\n;\n?\n!\n-")
	L.SetField(-2, "config")

	// Set package.preload = registry["_PRELOAD"] (shared table)
	// Mirrors C Lua: luaL_getsubtable(L, LUA_REGISTRYINDEX, LUA_PRELOAD_TABLE)
	L.GetSubTable(luaapi.RegistryIndex, "_PRELOAD")
	L.SetField(-2, "preload")

	// Create package.searchers table with 5 searcher functions.
	// Mirrors C Lua's createsearcherstable() in loadlib.c, plus Go global registry.
	// Searchers: 1=preload, 2=Lua file, 3=Go global, 4=C lib (stub), 5=C root (stub)
	searchers := []luaapi.CFunction{
		searcher_preload,
		searcher_Lua,
		searcher_GoGlobal,
		searcher_Clib,
		searcher_Croot,
	}
	L.CreateTable(len(searchers), 0)
	for i, s := range searchers {
		L.PushCFunction(s)
		L.RawSetI(-2, int64(i+1))
	}
	L.SetField(-2, "searchers")

	return 1
}

// pkgSearchPath implements package.searchpath(name, path [, sep [, rep]])
func pkgSearchPath(L *luaapi.State) int {
	name := L.CheckString(1)
	path := L.CheckString(2)
	sep := L.OptString(3, ".")
	rep := L.OptString(4, string(os.PathSeparator))

	if sep != "" {
		name = strings.ReplaceAll(name, sep, rep)
	}

	var tried strings.Builder
	templates := strings.Split(path, ";")
	for _, tmpl := range templates {
		candidate := strings.ReplaceAll(tmpl, "?", name)
		if err := statFileWithFS(L.FileSystem, candidate); err == nil {
			L.PushString(candidate)
			return 1
		}
		if tried.Len() > 0 {
			tried.WriteString("\n\t")
		}
		tried.WriteString("no file '" + candidate + "'")
	}

	L.PushNil()
	L.PushString(tried.String())
	return 2
}
