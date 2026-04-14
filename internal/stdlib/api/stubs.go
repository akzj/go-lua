package api

// Stub openers for libraries not yet implemented.
// Each returns 1 (an empty library table) to satisfy OpenAll.

import (
	"os"
	"strings"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

func OpenIO(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{})

	// io.stdin / io.stdout / io.stderr — stub file handles (tables)
	// Many testes just check io.stdin ~= nil or use string.format("%p", io.stdin).
	for _, name := range []string{"stdin", "stdout", "stderr"} {
		L.CreateTable(0, 0) // stub file handle
		L.SetField(-2, name)
	}

	return 1
}

func OpenOS(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{
		"clock": osClockStub,
	})
	return 1
}

func osClockStub(L *luaapi.State) int {
	L.PushNumber(0)
	return 1
}

func OpenDebug(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{
		"setmetatable": debugSetmetatable,
		"getmetatable": debugGetmetatable,
		"traceback":    debugTraceback,
		"getinfo":      debugGetinfo,
		"getupvalue":   debugGetupvalue,
		"sethook":      debugSethook,
	})
	return 1
}

// debug.setmetatable(value, table) — sets the metatable for any value
func debugSetmetatable(L *luaapi.State) int {
	L.CheckAny(1)
	if L.IsNoneOrNil(2) {
		L.SetTop(2)
		L.PushNil() // ensure there's a nil at 2
	}
	L.SetMetatable(1)
	L.SetTop(1) // return the first argument
	return 1
}

// debug.getmetatable(value) — returns the metatable of any value
func debugGetmetatable(L *luaapi.State) int {
	L.CheckAny(1)
	if !L.GetMetatable(1) {
		L.PushNil()
	}
	return 1
}

// debug.traceback([thread,] [message [, level]]) — returns a traceback string
// Minimal stub: just returns the message or an empty string
func debugTraceback(L *luaapi.State) int {
	msg, ok := L.ToString(1)
	if ok {
		L.PushString(msg)
	} else if L.IsNoneOrNil(1) {
		L.PushString("")
	} else {
		L.PushValue(1) // return non-string as-is
	}
	return 1
}

// debug.getinfo([thread,] f [, what]) — returns debug info table
// Mirrors: db_getinfo in ldblib.c
func debugGetinfo(L *luaapi.State) int {
	// Parse arguments: getinfo(level [, what])
	var ar *luaapi.DebugInfo
	var ok bool
	what := "flnSu" // default: all options

	if L.Type(1) == 3 { // number = stack level
		level := int(L.CheckInteger(1))
		if L.GetTop() >= 2 {
			what = L.CheckString(2)
		}
		ar, ok = L.GetStack(level) // level passed directly (like C Lua)
		if !ok {
			L.PushNil()
			return 1
		}
	} else {
		// function argument — not yet supported, return minimal
		L.CreateTable(0, 4)
		L.PushString("")
		L.SetField(-2, "name")
		L.PushString("Lua")
		L.SetField(-2, "what")
		L.PushString("")
		L.SetField(-2, "source")
		L.PushInteger(0)
		L.SetField(-2, "currentline")
		L.PushString("")
		L.SetField(-2, "namewhat")
		return 1
	}

	// Fill additional fields based on 'what' string
	L.GetInfo(what, ar)

	// Build result table
	L.CreateTable(0, 8)

	// Always populate basic fields from ar
	L.PushString(ar.Name)
	L.SetField(-2, "name")
	L.PushString(ar.NameWhat)
	L.SetField(-2, "namewhat")
	L.PushString(ar.What)
	L.SetField(-2, "what")
	L.PushString(ar.Source)
	L.SetField(-2, "source")
	L.PushString(ar.ShortSrc)
	L.SetField(-2, "short_src")
	L.PushInteger(int64(ar.CurrentLine))
	L.SetField(-2, "currentline")
	L.PushInteger(int64(ar.LineDefined))
	L.SetField(-2, "linedefined")
	L.PushInteger(int64(ar.LastLineDefined))
	L.SetField(-2, "lastlinedefined")
	L.PushInteger(int64(ar.NUps))
	L.SetField(-2, "nups")
	L.PushInteger(int64(ar.NParams))
	L.SetField(-2, "nparams")
	L.PushBoolean(ar.IsVararg)
	L.SetField(-2, "isvararg")

	return 1
}

// debug.getupvalue(f, up) — returns name and value of upvalue
func debugGetupvalue(L *luaapi.State) int {
	n := int(L.CheckInteger(2))
	name := L.GetUpvalue(1, n)
	if name == "" {
		L.PushNil()
		return 1
	}
	L.PushString(name) // push name
	L.Insert(-2)       // move name before value (GetUpvalue already pushed value)
	return 2
}

// debug.sethook([thread,] hook, mask [, count]) — stub (no-op)
func debugSethook(L *luaapi.State) int {
	return 0
}

func OpenUTF8(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{})
	return 1
}

func OpenCoroutine(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{
		"wrap":    coroWrapStub,
		"running": coroRunningStub,
	})
	return 1
}

// coroRunningStub implements coroutine.running().
// Returns the running coroutine plus a boolean.
// Since we don't have full coroutine support yet, returns nil, true
// (meaning: main thread is running).
func coroRunningStub(L *luaapi.State) int {
	ismain := L.PushThread()
	L.PushBoolean(ismain)
	return 2
}

// coroWrapStub is a minimal coroutine.wrap that just returns the function.
// This works for simple iterator patterns that don't actually yield.
func coroWrapStub(L *luaapi.State) int {
	L.CheckType(1, 6) // TypeFunction = 6
	L.PushValue(1)
	return 1
}

func OpenPackage(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{
		"searchpath": pkgSearchPath,
	})

	// Set package.path — default Lua search path
	// "./?.lua" covers the common case of loading from the current directory
	// and the directory of the running script.
	L.PushString("./?.lua;./?/init.lua")
	L.SetField(-2, "path")

	// Set package.loaded = registry["_LOADED"]
	L.GetField(luaapi.RegistryIndex, "_LOADED")
	L.SetField(-2, "loaded")

	// Set package.config (separator, template mark, substitution mark, etc.)
	L.PushString(string(os.PathSeparator) + "\n;\n?\n!\n-")
	L.SetField(-2, "config")

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
		tmpl = strings.TrimSpace(tmpl)
		if tmpl == "" {
			continue
		}
		candidate := strings.ReplaceAll(tmpl, "?", name)
		if _, err := os.Stat(candidate); err == nil {
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
