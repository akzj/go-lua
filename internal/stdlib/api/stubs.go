package api

// Stub openers for libraries not yet implemented.
// Each returns 1 (an empty library table) to satisfy OpenAll.

import (
	"fmt"
	"os"
	"strings"

	luaapi "github.com/akzj/go-lua/internal/api/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
)

func OpenIO(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{})

	// io.stdin / io.stdout / io.stderr — stub file handles.
	// Use light userdata so rawlen() correctly errors (not tables).
	// Each gets a unique pointer value for string.format("%p") identity.
	type ioStub struct{ name string }
	for _, name := range []string{"stdin", "stdout", "stderr"} {
		L.PushLightUserdata(&ioStub{name})
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
		"setupvalue":   debugSetupvalue,
		"upvaluejoin":  debugUpvaluejoin,
		"upvalueid":    debugUpvalueid,
		"sethook":      debugSethook,
		"gethook":      debugGethook,
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
// Mirrors: luaB_traceback in ldblib.c (simplified)
func debugTraceback(L *luaapi.State) int {
	// Parse arguments
	arg := 1
	// Skip thread argument if present (we don't support coroutine tracing)
	msg, hasMsg := L.ToString(arg)
	level := 1
	if L.Type(arg) == 3 { // number as first arg = level
		level = int(L.CheckInteger(arg))
		hasMsg = false
	} else {
		if !hasMsg && !L.IsNoneOrNil(arg) {
			// Non-string, non-nil message — return as-is
			L.PushValue(arg)
			return 1
		}
		if L.GetTop() >= arg+1 {
			level = int(L.OptInteger(arg+1, 1))
		}
	}

	// Build traceback
	var buf strings.Builder
	if hasMsg {
		buf.WriteString(msg)
		buf.WriteString("\n")
	}
	buf.WriteString("stack traceback:")

	for level < 200 { // safety limit
		ar, ok := L.GetStack(level)
		if !ok {
			break
		}
		L.GetInfo("Slnt", ar)
		buf.WriteString("\n\t")
		buf.WriteString(ar.ShortSrc)
		if ar.CurrentLine > 0 {
			buf.WriteString(fmt.Sprintf(":%d", ar.CurrentLine))
		}
		buf.WriteString(": in ")

		// Determine frame description
		if ar.What == "main" {
			buf.WriteString("main chunk")
		} else if ar.What == "C" {
			buf.WriteString("?")
		} else {
			// Try to get function name
			L.GetInfo("n", ar)
			if ar.Name != "" {
				if ar.NameWhat == "metamethod" {
					buf.WriteString(fmt.Sprintf("metamethod '%s'", ar.Name))
				} else {
					buf.WriteString(fmt.Sprintf("%s '%s'", ar.NameWhat, ar.Name))
				}
			} else {
				buf.WriteString("function <")
				buf.WriteString(ar.ShortSrc)
				if ar.LineDefined > 0 {
					buf.WriteString(fmt.Sprintf(":%d", ar.LineDefined))
				}
				buf.WriteString(">")
			}
		}
		level++
	}
	if level >= 200 {
		buf.WriteString("\n\t...")
	}

	L.PushString(buf.String())
	return 1
}

// debug.getinfo([thread,] f [, what]) — returns debug info table
// Mirrors: db_getinfo in ldblib.c
func debugGetinfo(L *luaapi.State) int {
	// Parse arguments: getinfo(level [, what])
	var ar *luaapi.DebugInfo
	var ok bool
	what := "flnStu" // default: all options (matches C Lua)

	if L.Type(1) == 3 { // number = stack level
		level := int(L.CheckInteger(1))
		if L.GetTop() >= 2 {
			what = L.CheckString(2)
		}
		// Validate: '>' is invalid when using stack level
		if strings.Contains(what, ">") {
			L.ArgError(2, "invalid option '>'")
		}
		// Validate option characters
		for _, c := range what {
			if !strings.ContainsRune("flnSrtupLa", c) {
				L.ArgError(2, "invalid option")
			}
		}
		ar, ok = L.GetStack(level) // level passed directly (like C Lua)
		if !ok {
			L.PushNil()
			return 1
		}
	} else {
		// function argument — inspect the function directly
		L.CheckAny(1)
		what = "flnStu" // default for function arg
		if L.GetTop() >= 2 {
			what = L.CheckString(2)
		}
		// Validate: '>' is invalid in user-supplied options
		if strings.Contains(what, ">") {
			L.ArgError(2, "invalid option '>'")
		}
		// Validate option characters
		for _, c := range what {
			if !strings.ContainsRune("flnSrtupLa", c) {
				L.ArgError(2, "invalid option")
			}
		}

		L.CreateTable(0, 10)

		src, shortSrc, whatKind, lineDefined, lastLine, nups, nparams, isVararg, _ := L.GetFuncProtoInfo(1)

		if strings.Contains(what, "S") {
			L.PushString(src)
			L.SetField(-2, "source")
			L.PushString(shortSrc)
			L.SetField(-2, "short_src")
			L.PushInteger(int64(lineDefined))
			L.SetField(-2, "linedefined")
			L.PushInteger(int64(lastLine))
			L.SetField(-2, "lastlinedefined")
			L.PushString(whatKind)
			L.SetField(-2, "what")
		}
		if strings.Contains(what, "u") {
			L.PushInteger(int64(nups))
			L.SetField(-2, "nups")
			L.PushInteger(int64(nparams))
			L.SetField(-2, "nparams")
			L.PushBoolean(isVararg)
			L.SetField(-2, "isvararg")
		}
		if strings.Contains(what, "n") {
			// When inspecting by function value (not stack level),
			// name/namewhat are unknown — push nil
			L.PushNil()
			L.SetField(-2, "name")
			L.PushString("")
			L.SetField(-2, "namewhat")
		}
		if strings.Contains(what, "l") {
			L.PushInteger(-1)
			L.SetField(-2, "currentline")
		}
		if strings.Contains(what, "t") {
			L.PushBoolean(false)
			L.SetField(-2, "istailcall")
			L.PushInteger(0)
			L.SetField(-2, "extraargs")
		}
		if strings.Contains(what, "f") {
			L.PushValue(1) // push the function itself
			L.SetField(-2, "func")
		}
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
	L.PushBoolean(ar.IsTailCall)
	L.SetField(-2, "istailcall")
	L.PushInteger(int64(ar.ExtraArgs))
	L.SetField(-2, "extraargs")

	return 1
}

// debug.getupvalue(f, up) — returns name and value of upvalue
func debugGetupvalue(L *luaapi.State) int {
	L.CheckType(1, objectapi.TypeFunction)
	n := int(L.CheckInteger(2))
	name := L.GetUpvalue(1, n)
	if name == "" {
		return 0 // no results when upvalue doesn't exist (matches C Lua)
	}
	L.PushString(name) // push name
	L.Insert(-2)       // move name before value (GetUpvalue already pushed value)
	return 2
}

// debug.setupvalue(f, up, value) — sets upvalue and returns its name
func debugSetupvalue(L *luaapi.State) int {
	L.CheckAny(3)
	L.CheckType(1, objectapi.TypeFunction)
	n := int(L.CheckInteger(2))
	// SetUpvalue pops the value from the top of the stack
	L.PushValue(3) // push the value to top for SetUpvalue to consume
	name := L.SetUpvalue(1, n)
	if name == "" {
		return 0 // no results when upvalue doesn't exist
	}
	L.PushString(name)
	return 1
}

// debug.upvaluejoin(f1, n1, f2, n2) — make f1's n1-th upvalue share f2's n2-th upvalue
func debugUpvaluejoin(L *luaapi.State) int {
	L.CheckType(1, objectapi.TypeFunction)
	n1 := int(L.CheckInteger(2))
	L.CheckType(3, objectapi.TypeFunction)
	n2 := int(L.CheckInteger(4))
	f1 := L.GetLClosure(1)
	f2 := L.GetLClosure(3)
	if f1 == nil {
		L.ArgError(1, "Lua function expected")
	}
	if f2 == nil {
		L.ArgError(3, "Lua function expected")
	}
	if n1 < 1 || n1 > len(f1.UpVals) {
		L.ArgError(2, "invalid upvalue index")
	}
	if n2 < 1 || n2 > len(f2.UpVals) {
		L.ArgError(4, "invalid upvalue index")
	}
	f1.UpVals[n1-1] = f2.UpVals[n2-1]
	return 0
}

// debug.upvalueid(f, n) — returns a unique identifier for the n-th upvalue
// Mirrors: db_upvalueid in ldblib.c
func debugUpvalueid(L *luaapi.State) int {
	L.CheckType(1, objectapi.TypeFunction)
	n := int(L.CheckInteger(2))
	f := L.GetLClosure(1)
	if f == nil {
		// C closures: use address of the upvalue TValue slot
		// For now, just push fail
		L.PushBoolean(false)
		return 1
	}
	if n < 1 || n > len(f.UpVals) {
		L.ArgError(2, "invalid upvalue index")
	}
	uv := f.UpVals[n-1]
	if uv == nil {
		L.PushBoolean(false)
		return 1
	}
	L.PushLightUserdata(uv) // unique pointer identity
	return 1
}

// debug.sethook([thread,] hook, mask [, count])
// Mirrors C Lua's db_sethook in ldblib.c
// debug.gethook([thread]) — returns hook function, mask string, count
// Mirrors: db_gethook in ldblib.c
func debugGethook(L *luaapi.State) int {
	// TODO: thread argument support (for now, always use current thread)
	mask := L.HookMask()
	// Get hook function from registry
	L.PushString("__debug_hook__")
	L.GetTable(luaapi.RegistryIndex)
	if L.IsNil(-1) {
		L.Pop(1)
		L.PushBoolean(false) // luaL_pushfail
		return 1
	}
	// Build mask string
	var buf []byte
	if mask&1 != 0 { // MaskCall
		buf = append(buf, 'c')
	}
	if mask&2 != 0 { // MaskRet
		buf = append(buf, 'r')
	}
	if mask&4 != 0 { // MaskLine
		buf = append(buf, 'l')
	}
	L.PushString(string(buf))
	L.PushInteger(int64(L.HookCount()))
	return 3
}

func debugSethook(L *luaapi.State) int {
	arg := 1
	// TODO: thread argument support (for now, always use current thread)

	if L.IsNoneOrNil(arg) {
		// Turn off hooks: debug.sethook() or debug.sethook(nil)
		// IMPORTANT: clear registry hook first, then disable mask.
		// In close/return hook contexts, ClearHookFields first can allow
		// pending returns to run with HookMask==0 before registry is cleared,
		// causing flaky hook visibility.
		L.PushString("__debug_hook__")
		L.PushNil()
		L.SetTable(luaapi.RegistryIndex)
		L.ClearHookFields()
		return 0
	}

	// debug.sethook(func, mask [, count])
	L.CheckType(arg, 6) // LUA_TFUNCTION
	smask := L.CheckString(arg + 1)
	count := 0
	if L.IsNumber(arg + 2) {
		v, _ := L.ToInteger(arg + 2)
		count = int(v)
	}

	// Parse mask string: 'c'=call, 'r'=return, 'l'=line
	mask := 0
	for _, c := range smask {
		switch c {
		case 'c':
			mask |= 1 // MaskCall
		case 'r':
			mask |= 2 // MaskRet
		case 'l':
			mask |= 4 // MaskLine
		}
	}
	if count > 0 {
		mask |= 8 // MaskCount
	}

	// Store hook function in registry["__debug_hook__"]
	L.PushString("__debug_hook__")
	L.PushValue(arg) // push the hook function
	L.SetTable(luaapi.RegistryIndex)

	// Set hook mask and enable hooks
	L.SetHookFields(mask, count)
	L.SetHookMarker()

	return 0
}

func OpenUTF8(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{})
	// utf8.charpattern — pattern matching a single UTF-8 character
	// Mirrors: UTF8PATT in lutf8lib.c: "[\0-\x7F\xC2-\xFD][\x80-\xBF]*"
	L.PushString("[\x00-\x7F\xC2-\xFD][\x80-\xBF]*")
	L.SetField(-2, "charpattern")
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

	// Set package.cpath — default C library search path
	L.PushString("./?.so")
	L.SetField(-2, "cpath")

	// Set package.loaded = registry["_LOADED"]
	L.GetField(luaapi.RegistryIndex, "_LOADED")
	L.SetField(-2, "loaded")

	// Set package.config (separator, template mark, substitution mark, etc.)
	L.PushString(string(os.PathSeparator) + "\n;\n?\n!\n-")
	L.SetField(-2, "config")

	// Set package.preload = {} (empty table for preloaded modules)
	// C Lua: luaL_getsubtable(L, LUA_REGISTRYINDEX, LUA_PRELOAD_TABLE)
	L.CreateTable(0, 0)
	L.SetField(-2, "preload")

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
