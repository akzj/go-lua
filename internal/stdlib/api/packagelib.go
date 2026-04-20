package api

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
	"os"
	"strings"

	luaapi "github.com/akzj/go-lua/internal/api/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
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
	if tp != objectapi.TypeTable {
		L.Pop(2) // pop preload + package
		L.PushString("no field package.preload['" + name + "']")
		return 1
	}
	tp = L.GetField(-1, name)
	if tp == objectapi.TypeNil {
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
	if tp != objectapi.TypeString {
		L.Pop(2) // pop path + package
		L.Errorf("'package.path' must be a string")
		return 0
	}
	pathStr, _ := L.ToString(-1)
	L.Pop(2) // pop path + package

	// Search for the module file
	filename, tried := searchPathForSearcher(name, pathStr)
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
	data, err := os.ReadFile(filename)
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
	if tp != objectapi.TypeString {
		L.Pop(2)
		L.PushString("no field package.cpath")
		return 1
	}
	cpathStr, _ := L.ToString(-1)
	L.Pop(2) // pop cpath + package

	// Search the cpath to produce the correct error messages
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

// searcher_Croot — searcher #4: all-in-one C module searcher
// Stub: returns nil (no error message to add).
func searcher_Croot(L *luaapi.State) int {
	L.PushNil()
	return 1
}

// searchPathForSearcher searches pathStr (semicolon-separated templates) for name.
// Replaces '.' in name with OS path separator, then '?' in each template with the result.
// Returns (found_file, tried_list). If found, tried_list is nil.
func searchPathForSearcher(name, pathStr string) (string, []string) {
	if pathStr == "" {
		return "", nil
	}
	fname := strings.ReplaceAll(name, ".", string(os.PathSeparator))
	templates := strings.Split(pathStr, ";")
	var tried []string
	for _, tmpl := range templates {
		tmpl = strings.TrimSpace(tmpl)
		if tmpl == "" {
			continue
		}
		candidate := strings.ReplaceAll(tmpl, "?", fname)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		tried = append(tried, candidate)
	}
	return "", tried
}
