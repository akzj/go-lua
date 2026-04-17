package api

// Stub openers for libraries not yet implemented.
// Each returns 1 (an empty library table) to satisfy OpenAll.

import (
	"fmt"
	"os"
	"strings"

	luaapi "github.com/akzj/go-lua/internal/api/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
	vmapi "github.com/akzj/go-lua/internal/vm/api"
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
		"clock":     osClockStub,
		"setlocale": osSetlocaleStub,
	})
	return 1
}

func osClockStub(L *luaapi.State) int {
	L.PushNumber(0)
	return 1
}

// osSetlocaleStub returns nil (locale not available in Go).
// This allows tests like `if os.setlocale("pt_BR") then ... end` to skip.
func osSetlocaleStub(L *luaapi.State) int {
	L.PushNil()
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
		"getlocal":     debugGetlocal,
		"setlocal":     debugSetlocal,
		"getregistry":  debugGetregistry,
		"setuservalue": debugSetuservalue,
		"getuservalue": debugGetuservalue,
	})

	// Create _HOOKKEY table in registry with weak keys.
	// Mirrors C Lua's HOOKKEY in ldblib.c — a table mapping threads to hook functions.
	// The metatable has __mode='k' so threads can be GC'd.
	L.PushValue(luaapi.RegistryIndex)     // push registry
	L.NewTable()                           // hook table
	L.NewTable()                           // metatable for hook table
	L.PushString("k")
	L.SetField(-2, "__mode")               // mt.__mode = 'k'
	L.SetMetatable(-2)                     // setmetatable(hooktable, mt)
	L.SetField(-2, "_HOOKKEY")             // registry._HOOKKEY = hooktable
	L.Pop(1)                               // pop registry

	return 1
}

// debug.getregistry() — returns the registry table
// Mirrors: luaB_getregistry in ldblib.c
func debugGetregistry(L *luaapi.State) int {
	L.PushValue(luaapi.RegistryIndex)
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
// Mirrors: luaB_traceback in ldblib.c
func debugTraceback(L *luaapi.State) int {
	// Handle optional thread argument (mirrors getthread in ldblib.c)
	arg := 1
	L1 := L // target state for traceback
	if L.Type(1) == objectapi.TypeThread {
		L1 = L.ToThread(1)
		arg = 2
	}

	msg, hasMsg := L.ToString(arg)
	// Default level: 1 for same thread, 0 for different thread (matches C Lua)
	defaultLevel := 1
	if L1 != L {
		defaultLevel = 0
	}
	level := defaultLevel
	if L.Type(arg) == objectapi.TypeNumber { // number as first arg = level
		level = int(L.CheckInteger(arg))
		hasMsg = false
	} else {
		if !hasMsg && !L.IsNoneOrNil(arg) {
			// Non-string, non-nil message — return as-is
			L.PushValue(arg)
			return 1
		}
		if L.GetTop() >= arg+1 {
			level = int(L.OptInteger(arg+1, int64(defaultLevel)))
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
		ar, ok := L1.GetStack(level)
		if !ok {
			break
		}
		L1.GetInfo("Slnt", ar)
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
			// Try to get function name for C functions
			L1.GetInfo("n", ar)
			if ar.Name != "" {
				buf.WriteString(fmt.Sprintf("function '%s'", ar.Name))
			} else {
				// Try to find function name by searching globals
				// Mirrors: pushglobalfuncname in lauxlib.c
				name := ""
				if L1.PushFuncFromDebug(ar) {
					funcIdx := L1.GetTop()
					funcPtr := L1.ToPointer(funcIdx)
					// Search _G for this function
					L1.PushGlobalTable()
					L1.PushNil()
					for L1.Next(-2) {
						// Compare using ToPointer for safe function comparison
						if L1.ToPointer(-1) == funcPtr && funcPtr != "" {
							if s, ok := L1.ToString(-2); ok {
								name = s
							}
							L1.Pop(2) // pop key and value
							break
						}
						L1.Pop(1) // pop value, keep key
					}
					L1.Pop(1) // pop _G
					L1.Pop(1) // pop function
				}
				if name != "" {
					buf.WriteString(fmt.Sprintf("function '%s'", name))
				} else {
					buf.WriteString("?")
				}
			}
		} else {
			// Try to get function name
			L1.GetInfo("n", ar)
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

// pushActiveLines pushes a table {[line]=true} for all active lines in a Lua
// function's prototype, or pushes nil for C functions. Mirrors collectvalidlines
// in ldebug.c.
func pushActiveLines(L *luaapi.State, idx int) {
	cl := L.GetLClosure(idx)
	if cl == nil {
		L.PushNil()
		return
	}
	p := cl.Proto
	L.CreateTable(0, len(p.LineInfo))
	if len(p.LineInfo) > 0 {
		currentLine := p.LineDefined
		startI := 0
		// For vararg functions, skip first instruction (OP_VARARGPREP)
		if p.IsVararg() {
			// nextline for instruction 0
			delta := p.LineInfo[0]
			if delta == -128 { // absLineInfo sentinel
				for _, ai := range p.AbsLineInfo {
					if ai.PC == 0 {
						currentLine = ai.Line
						break
					}
				}
			} else {
				currentLine += int(delta)
			}
			startI = 1
		}
		for i := startI; i < len(p.LineInfo); i++ {
			delta := p.LineInfo[i]
			if delta == -128 { // absLineInfo sentinel
				for _, ai := range p.AbsLineInfo {
					if ai.PC == i {
						currentLine = ai.Line
						break
					}
				}
			} else {
				currentLine += int(delta)
			}
			L.PushBoolean(true)
			L.RawSetI(-2, int64(currentLine))
		}
	}
}

// debug.getinfo([thread,] f [, what]) — returns debug info table
// Mirrors: db_getinfo in ldblib.c
func debugGetinfo(L *luaapi.State) int {
	// Handle optional thread argument (mirrors getthread in ldblib.c)
	arg := 1
	L1 := L // target state
	if L.Type(1) == objectapi.TypeThread {
		L1 = L.ToThread(1)
		arg = 2
	}

	// Parse arguments: getinfo([thread,] level|func [, what])
	var ar *luaapi.DebugInfo
	var ok bool
	what := "flnStu" // default: all options (matches C Lua)

	if L.Type(arg) == objectapi.TypeNumber { // number = stack level
		level := int(L.CheckInteger(arg))
		if L.GetTop() >= arg+1 {
			what = L.CheckString(arg + 1)
		}
		// Validate: '>' is invalid when using stack level
		if strings.Contains(what, ">") {
			L.ArgError(arg+1, "invalid option '>'")
		}
		// Validate option characters
		for _, c := range what {
			if !strings.ContainsRune("flnSrtupLa", c) {
				L.ArgError(arg+1, "invalid option")
			}
		}
		ar, ok = L1.GetStack(level) // use L1 (target thread)
		if !ok {
			L.PushNil()
			return 1
		}
	} else {
		// function argument — inspect the function directly
		L.CheckAny(arg)
		what = "flnStu" // default for function arg
		if L.GetTop() >= arg+1 {
			what = L.CheckString(arg + 1)
		}
		// Validate: '>' is invalid in user-supplied options
		if strings.Contains(what, ">") {
			L.ArgError(arg+1, "invalid option '>'")
		}
		// Validate option characters
		for _, c := range what {
			if !strings.ContainsRune("flnSrtupLa", c) {
				L.ArgError(arg+1, "invalid option")
			}
		}

		L.CreateTable(0, 10)

		src, shortSrc, whatKind, lineDefined, lastLine, nups, nparams, isVararg, _ := L.GetFuncProtoInfo(arg)

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
			L.PushValue(arg) // push the function itself
			L.SetField(-2, "func")
		}
		if strings.Contains(what, "L") {
			pushActiveLines(L, arg) // push activelines table (or nil for C func)
			L.SetField(-2, "activelines")
		}
		return 1
	}

	// Fill additional fields based on 'what' string
	L1.GetInfo(what, ar)

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

	// Handle "r" (transfer info for call/return hooks)
	if strings.Contains(what, "r") {
		L.PushInteger(int64(ar.FTransfer))
		L.SetField(-2, "ftransfer")
		L.PushInteger(int64(ar.NTransfer))
		L.SetField(-2, "ntransfer")
	}

	// Handle "f" (push function) and "L" (activelines) for stack-level path
	if strings.Contains(what, "f") || strings.Contains(what, "L") {
		if L1.PushFuncFromDebug(ar) {
			funcIdx := L1.GetTop() // function is now on L1's stack
			if L1 != L {
				// Transfer function from L1 to L
				L1.XMove(L, 1) // moves top of L1 to L
				funcIdx = L.GetTop()
			}
			if strings.Contains(what, "f") {
				L.PushValue(funcIdx)
				L.SetField(-3, "func") // table is at funcIdx-1, but after push it's -3
			}
			if strings.Contains(what, "L") {
				pushActiveLines(L, funcIdx)
				L.SetField(-3, "activelines")
			}
			L.Pop(1) // pop the function
		} else {
			if strings.Contains(what, "L") {
				L.PushNil()
				L.SetField(-2, "activelines")
			}
		}
	}

	return 1
}

// debug.getupvalue(f, up) — returns name and value of upvalue
func debugGetupvalue(L *luaapi.State) int {
	L.CheckType(1, objectapi.TypeFunction)
	n := int(L.CheckInteger(2))
	name, ok := L.GetUpvalue(1, n)
	if !ok {
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
	name, ok := L.SetUpvalue(1, n)
	if !ok {
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
	// getthread: check if arg 1 is a thread
	arg := 0
	var L1 *stateapi.LuaState
	if L.Type(1) == objectapi.TypeThread {
		t := L.ToThread(1)
		L1 = t.Internal.(*stateapi.LuaState)
		arg = 1
	} else {
		L1 = L.Internal.(*stateapi.LuaState)
		arg = 0
	}
	_ = arg // arg not used for gethook (no further args)

	mask := L1.HookMask
	// Get hook function from target thread's Hook field
	hookVal, ok := L1.Hook.(objectapi.TValue)
	if !ok || hookVal.Val == nil || hookVal.Tt == objectapi.TagNil {
		L.PushBoolean(false) // luaL_pushfail — no hook
		return 1
	}
	// Push hook function onto calling thread's stack (safely)
	ls := L.Internal.(*stateapi.LuaState)
	vmapi.CheckStack(ls, 1)
	ls.Stack[ls.Top].Val = hookVal
	ls.Top++

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
	L.PushInteger(int64(L1.BaseHookCount))
	return 3
}

func debugSethook(L *luaapi.State) int {
	// getthread: check if arg 1 is a thread
	arg := 0
	var L1 *stateapi.LuaState
	if L.Type(1) == objectapi.TypeThread {
		t := L.ToThread(1)
		L1 = t.Internal.(*stateapi.LuaState)
		arg = 1
	} else {
		L1 = L.Internal.(*stateapi.LuaState)
		arg = 0
	}

	if L.IsNoneOrNil(arg + 1) {
		// Turn off hooks: debug.sethook() or debug.sethook(thread)
		L1.Hook = nil
		L1.HookMask = 0
		L1.BaseHookCount = 0
		L1.HookCount = 0
		L1.AllowHook = true
		return 0
	}

	// debug.sethook([thread,] func, mask [, count])
	L.CheckType(arg+1, objectapi.TypeFunction)
	smask := L.CheckString(arg + 2)
	count := 0
	if L.IsNumber(arg + 3) {
		v, _ := L.ToInteger(arg + 3)
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

	// Store hook function as TValue on target thread's Hook field (per-thread).
	// Push the function value to top, then read the TValue from the stack.
	L.PushValue(arg + 1)
	callerLS := L.Internal.(*stateapi.LuaState)
	L1.Hook = callerLS.Stack[callerLS.Top-1].Val
	L.Pop(1)

	// Set hook mask and enable hooks on target thread
	L1.HookMask = mask
	L1.BaseHookCount = count
	L1.HookCount = count
	// Do NOT set AllowHook here — if called from inside a hook,
	// AllowHook must remain false until hookDispatch restores it.
	// C Lua's db_sethook does not touch allowhook when setting hooks.

	// When activating line hooks on the CURRENT thread (not a coroutine),
	// initialize OldPC to the calling Lua frame's current PC so that
	// TraceExec doesn't see a stale OldPC=0 and fire a spurious line hook.
	if mask&4 != 0 && L1 == callerLS { // MaskLine = 4; same thread
		// The caller's CI is the sethook C frame; its prev is the Lua caller
		if callerLS.CI != nil && callerLS.CI.Prev != nil && callerLS.CI.Prev.IsLua() {
			L1.OldPC = callerLS.CI.Prev.SavedPC
		}
	}

	return 0
}

// OpenUTF8 moved to utf8lib.go

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

// getLocalNameFromProto returns the name of local variable n from prototype p.
func getLocalNameFromProto(p *objectapi.Proto, idx int) string {
	if p == nil {
		return ""
	}
	if idx == 0 {
		if p.IsVararg() {
			return "(*vararg*)"
		}
		return ""
	}
	// Parameters have StartPC == 0 (declared at function entry).
	// Iterate LocVars with StartPC == 0, count named locals.
	for i := 0; i < len(p.LocVars) && p.LocVars[i].StartPC == 0; i++ {
		if p.LocVars[i].Name != nil {
			if idx == 1 {
				return p.LocVars[i].Name.Data
			}
			idx--
		}
	}
	return ""
}

// debug.getlocal — 3 forms (mirrors luaB_getlocal in C Lua ldblib.c)
func debugGetlocal(L *luaapi.State) int {
	n := int(L.CheckInteger(L.GetTop())) // n is ALWAYS last arg

	if L.Type(1) == objectapi.TypeFunction {
		// (func, n) — name only, return 1
		cl := L.GetLClosure(1)
		if cl == nil || cl.Proto == nil {
			L.PushNil()
			return 1
		}
		p := cl.Proto
		if n == 0 {
			if p.IsVararg() {
				L.PushString("(*vararg*)")
				return 1
			}
			L.PushNil()
			return 1
		}
		if n < 0 || n > int(p.NumParams) {
			L.PushNil()
			return 1
		}
		name := getLocalNameFromProto(p, n)
		if name == "" {
			L.PushNil()
			return 1
		}
		L.PushString(name)
		return 1
	}

	if L.Type(1) == objectapi.TypeThread {
		thread := L.ToThread(1)
		if L.Type(2) == objectapi.TypeFunction {
			// (co, func, n) — name only from func's proto, return 1
			// C Lua ignores the coroutine argument entirely here.
			cl := L.GetLClosure(2)
			if cl == nil || cl.Proto == nil {
				L.PushNil()
				return 1
			}
			p := cl.Proto
			if n == 0 {
				if p.IsVararg() {
					L.PushString("(*vararg*)")
					return 1
				}
				L.PushNil()
				return 1
			}
			if n < 0 || n > int(p.NumParams) {
				L.PushNil()
				return 1
			}
			name := getLocalNameFromProto(p, n)
			if name == "" {
				L.PushNil()
				return 1
			}
			L.PushString(name)
			return 1
		}
		// (co, level, n) — name + value, return 2
		level := int(L.CheckInteger(2))
		ar, ok := thread.GetStack(level)
		if !ok {
			L.ArgError(2, "invalid level")
			return 0
		}
		name := thread.GetLocal(ar, n)
		if name == "" {
			L.PushNil() // no local found, push nil
			return 1
		}
		// GetLocal pushed the value onto thread's stack — transfer to L
		thread.XMove(L, 1)
		L.PushString(name) // push name on L
		L.Insert(-2)       // move name below value: [name, value]
		return 2
	}

	// (level, n) — name + value, return 2
	level := int(L.CheckInteger(1))
	ar, ok := L.GetStack(level)
	if !ok {
		L.ArgError(1, "invalid level")
		return 0
	}
	name := L.GetLocal(ar, n)
	if name == "" {
		L.PushNil() // no local found, push nil
		return 1
	}
	L.PushString(name) // push name on top of value
	L.Insert(-2)       // move name below value: [name, value]
	return 2
}

// debug.setlocal(level, n, value) → name or nil
func debugSetlocal(L *luaapi.State) int {
	n := int(L.CheckInteger(L.GetTop() - 1)) // n is 2nd to last arg
	if L.Type(1) == objectapi.TypeThread {
		// (co, level, n, value)
		thread := L.ToThread(1)
		level := int(L.CheckInteger(2))
		ar, ok := thread.GetStack(level)
		if !ok {
			L.ArgError(2, "invalid level")
			return 0
		}
		// Transfer value from L to thread's stack
		L.XMove(thread, 1) // move top value (the new value) to thread
		name := thread.SetLocal(ar, n)
		if name == "" {
			// SetLocal didn't consume the value — pop it from thread
			thread.Pop(1)
			L.PushNil()
		} else {
			L.PushString(name)
		}
		return 1
	}
	// (level, n, value)
	level := int(L.CheckInteger(1))
	ar, ok := L.GetStack(level)
	if !ok {
		L.ArgError(1, "invalid level")
		return 0
	}
	name := L.SetLocal(ar, n)
	if name == "" {
		L.PushNil()
	} else {
		L.PushString(name)
	}
	return 1
}

// debug.getuservalue(u, n) → value, bool
// Mirrors: luaB_getuservalue in ldblib.c
// Returns the nth user value of userdata u, plus a boolean indicating success.
func debugGetuservalue(L *luaapi.State) int {
	n := 1
	if L.GetTop() >= 2 {
		n = int(L.CheckInteger(2))
	}
	tp := L.GetIUserValue(1, n)
	L.PushBoolean(tp != objectapi.TypeNone)
	return 2
}

// debug.setuservalue(u, value, n) → u (or fail)
// Mirrors: luaB_setuservalue in ldblib.c
// Sets the nth user value of userdata u to value. Returns u on success, or fail.
func debugSetuservalue(L *luaapi.State) int {
	n := 1
	if L.GetTop() >= 3 {
		n = int(L.CheckInteger(3))
	}
	L.CheckAny(1)
	L.CheckAny(2)
	if !L.SetIUserValue(1, n) {
		L.PushBoolean(false) // luaL_pushfail
		return 1
	}
	L.PushValue(1) // return the userdata
	return 1
}
