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

// debug.getinfo([thread,] f [, what]) — stub returning minimal info table
func debugGetinfo(L *luaapi.State) int {
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

// debug.getupvalue(f, up) — stub returning name and value of upvalue
func debugGetupvalue(L *luaapi.State) int {
	// Minimal: return nil (no upvalue info available)
	L.PushNil()
	return 1
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
	L.PushNil()
	L.PushBoolean(true) // true = this is the main thread
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
