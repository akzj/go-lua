package api

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	luaapi "github.com/akzj/go-lua/internal/api/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
)

// ---------------------------------------------------------------------------
// Base library — registered into _G
// Reference: lua-master/lbaselib.c
// ---------------------------------------------------------------------------

func luaB_print(L *luaapi.State) int {
	n := L.GetTop()
	for i := 1; i <= n; i++ {
		s := L.TolString(i)
		if i > 1 {
			fmt.Print("\t")
		}
		fmt.Print(s)
		L.Pop(1) // pop the string pushed by TolString
	}
	fmt.Println()
	return 0
}

// luaB_require — Lua 5.5 require() using package.searchers.
// Mirrors ll_require in C Lua loadlib.c:
// 1. Check package.loaded[modname] — if truthy, return it
// 2. Call findloader to iterate package.searchers
// 3. Call the loader with (modname, extra)
// 4. Store result in package.loaded[modname]
// 5. Return (result, extra)
func luaB_require(L *luaapi.State) int {
	name := L.CheckString(1)
	L.SetTop(1) // ensure clean stack: [name] at index 1

	// Push _LOADED table at index 2
	L.GetField(luaapi.RegistryIndex, "_LOADED") // stack: [name, _LOADED]
	tp := L.GetField(2, name)                   // stack: [name, _LOADED, _LOADED[name]]
	if L.ToBoolean(3) {
		// Already loaded — return the value (only 1 return for cached modules)
		return 1
	}
	L.Pop(1) // pop nil/false result; stack: [name, _LOADED]

	// findloader: iterate package.searchers to find a loader
	// Get package.searchers
	L.GetGlobal("package")                       // stack: [name, _LOADED, package]
	tp = L.GetField(-1, "searchers")             // stack: [name, _LOADED, package, searchers]
	if tp != objectapi.TypeTable {
		L.Errorf("'package.searchers' must be a table")
		return 0
	}
	L.Remove(-2) // remove package; stack: [name, _LOADED, searchers] (searchers at index 3)

	// Build error message from searcher failures
	var errMsgs strings.Builder

	for i := int64(1); ; i++ {
		tp = L.RawGetI(3, i) // stack: [name, _LOADED, searchers, searcher_i]
		if tp == objectapi.TypeNil {
			// No more searchers — build error and fail
			L.Pop(1) // pop nil
			L.Errorf("module '%s' not found:%s", name, errMsgs.String())
			return 0
		}

		// Call searcher(name) → (loader_or_msg, extra)
		L.PushString(name)
		L.Call(1, 2) // stack: [name, _LOADED, searchers, result1, result2]

		if L.IsFunction(-2) {
			// Found a loader! Stack: [..., loader, extra]
			// Mirrors C Lua ll_require exactly:
			L.Rotate(-2, 1) // stack: [..., extra, loader]
			L.PushValue(1)  // push modname as 1st arg
			L.PushValue(-3) // push extra as 2nd arg; stack: [..., extra, loader, name, extra]
			L.Call(2, 1)    // call loader(name, extra); stack: [..., extra, result]

			// Store in _LOADED if non-nil (setfield pops the value)
			if !L.IsNil(-1) {
				L.SetField(2, name) // _LOADED[name] = result (pops result)
			} else {
				L.Pop(1) // pop nil result
			}
			// Stack: [..., extra]

			// Get _LOADED[name] — may have been set by the module itself
			if L.GetField(2, name) == objectapi.TypeNil {
				// Module set no value — use true
				L.Pop(1) // pop nil
				L.PushBoolean(true)
				L.PushValue(-1)          // dup true
				L.SetField(2, name)      // _LOADED[name] = true (pops dup)
			}
			// Stack: [..., extra, loaded_result]
			L.Rotate(-2, 1) // stack: [..., loaded_result, extra]
			return 2
		}

		if L.IsString(-2) {
			// Searcher returned error string
			s, _ := L.ToString(-2)
			errMsgs.WriteString("\n\t")
			errMsgs.WriteString(s)
		}
		L.Pop(2) // pop both returns, continue to next searcher
	}
}

func luaB_warn(L *luaapi.State) int {
	n := L.GetTop()
	L.CheckString(1)
	for i := 2; i <= n; i++ {
		L.CheckString(i)
	}
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		s, _ := L.ToString(i)
		sb.WriteString(s)
	}
	fmt.Fprintln(os.Stderr, "Lua warning: "+sb.String())
	return 0
}

func luaB_type(L *luaapi.State) int {
	t := L.Type(1)
	L.ArgCheck(t != luaapi.TypeNone, 1, "value expected")
	L.PushString(L.TypeName(t))
	return 1
}

func luaB_tonumber(L *luaapi.State) int {
	if L.IsNoneOrNil(2) { // standard conversion
		if L.Type(1) == objectapi.TypeNumber {
			L.SetTop(1)
			return 1
		}
		s, ok := L.ToString(1)
		if ok && L.StringToNumber(s) != 0 {
			return 1
		}
		L.CheckAny(1)
	} else {
		base := L.CheckInteger(2)
		L.CheckType(1, objectapi.TypeString)
		s, _ := L.ToString(1)
		L.ArgCheck(base >= 2 && base <= 36, 2, "base out of range")
		n, ok := strToInt(s, int(base))
		if ok {
			L.PushInteger(n)
			return 1
		}
	}
	L.PushFail()
	return 1
}

// strToInt converts string s in given base to integer
func strToInt(s string, base int) (int64, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0, false
	}
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}
	if len(s) == 0 {
		return 0, false
	}
	var n uint64
	foundDigit := false
	for i, c := range s {
		var digit int
		if c >= '0' && c <= '9' {
			digit = int(c - '0')
		} else if c >= 'a' && c <= 'z' {
			digit = int(c-'a') + 10
		} else if c >= 'A' && c <= 'Z' {
			digit = int(c-'A') + 10
		} else if unicode.IsSpace(c) {
			// Verify rest is all whitespace
			rest := s[i:]
			if strings.TrimSpace(rest) != "" {
				return 0, false
			}
			break
		} else {
			return 0, false
		}
		if digit >= base {
			return 0, false
		}
		foundDigit = true
		n = n*uint64(base) + uint64(digit)
	}
	if !foundDigit {
		return 0, false
	}
	if neg {
		return -int64(n), true
	}
	return int64(n), true
}

func luaB_error(L *luaapi.State) int {
	level := L.OptInteger(2, 1)
	L.SetTop(1)
	if L.Type(1) == objectapi.TypeString && level > 0 {
		L.Where(int(level))
		L.PushValue(1)
		L.Concat(2)
	}
	L.Error()
	return 0 // unreachable
}

func luaB_getmetatable(L *luaapi.State) int {
	L.CheckAny(1)
	if !L.GetMetatable(1) {
		L.PushNil()
		return 1
	}
	// Check for __metatable field
	if L.GetMetafield(1, "__metatable") {
		// __metatable found — it's already on stack from GetMetafield
		return 1
	}
	// Return the metatable itself (already on stack from GetMetatable)
	return 1
}

func luaB_setmetatable(L *luaapi.State) int {
	t := L.Type(2)
	L.CheckType(1, objectapi.TypeTable)
	L.ArgExpected(t == objectapi.TypeNil || t == objectapi.TypeTable, 2, "nil or table")
	if L.GetMetafield(1, "__metatable") {
		L.Errorf("cannot change a protected metatable")
	}
	L.SetTop(2)
	L.SetMetatable(1)
	return 1
}

func luaB_rawequal(L *luaapi.State) int {
	L.CheckAny(1)
	L.CheckAny(2)
	L.PushBoolean(L.RawEqual(1, 2))
	return 1
}

func luaB_rawlen(L *luaapi.State) int {
	t := L.Type(1)
	L.ArgExpected(t == objectapi.TypeTable || t == objectapi.TypeString, 1, "table or string")
	L.PushInteger(L.RawLen(1))
	return 1
}

func luaB_rawget(L *luaapi.State) int {
	L.CheckType(1, objectapi.TypeTable)
	L.CheckAny(2)
	L.SetTop(2)
	L.RawGet(1)
	return 1
}

func luaB_rawset(L *luaapi.State) int {
	L.CheckType(1, objectapi.TypeTable)
	L.CheckAny(2)
	L.CheckAny(3)
	L.SetTop(3)
	L.RawSet(1)
	return 1
}

func luaB_tostring(L *luaapi.State) int {
	L.CheckAny(1)
	L.TolString(1)
	return 1
}

func luaB_assert(L *luaapi.State) int {
	if L.ToBoolean(1) {
		return L.GetTop() // return all arguments
	}
	L.CheckAny(1) // there must be a condition
	L.Remove(1)   // remove it
	L.PushString("assertion failed!")
	L.SetTop(1) // leave only message
	return luaB_error(L)
}

func luaB_select(L *luaapi.State) int {
	n := L.GetTop()
	if L.Type(1) == objectapi.TypeString {
		s, _ := L.ToString(1)
		if len(s) > 0 && s[0] == '#' {
			L.PushInteger(int64(n - 1))
			return 1
		}
	}
	i := L.CheckInteger(1)
	if i < 0 {
		i = int64(n) + i
	} else if i > int64(n) {
		i = int64(n)
	}
	L.ArgCheck(1 <= i, 1, "index out of range")
	return n - int(i)
}

func luaB_next(L *luaapi.State) int {
	L.CheckType(1, objectapi.TypeTable)
	L.SetTop(2) // create a 2nd argument if there isn't one
	if L.Next(1) {
		return 2
	}
	L.PushNil()
	return 1
}

// pairsCont is the continuation for pairs() after yield from __pairs metamethod.
// Mirrors: pairscont in lbaselib.c:280
func pairsCont(L *stateapi.LuaState, status int, ctx int) int {
	return 4 // __pairs did all the work, just return its 4 results
}

func luaB_pairs(L *luaapi.State) int {
	L.CheckAny(1)
	if L.GetMetafield(1, "__pairs") {
		L.PushValue(1)
		L.CallK(1, 4, 0, pairsCont) // get 4 values from metamethod (iter, state, control, closing)
		return 4
	}
	L.PushCFunction(luaB_next)
	L.PushValue(1)
	L.PushNil()
	return 3
}

func ipairsAux(L *luaapi.State) int {
	i := L.CheckInteger(2)
	i++
	L.PushInteger(i)
	tp := L.GetI(1, i)
	if tp == objectapi.TypeNil {
		return 1
	}
	return 2
}

var ipairsAuxTV = luaapi.WrapCFunction(ipairsAux)

func luaB_ipairs(L *luaapi.State) int {
	L.CheckAny(1)
	L.PushCFunctionSame(ipairsAuxTV)
	L.PushValue(1)
	L.PushInteger(0)
	return 3
}

// finishPcallCont is the continuation for pcall/xpcall after yield/error recovery.
// Mirrors: finishpcall in lbaselib.c:471
// KFunction signature: func(L *stateapi.LuaState, status int, ctx int) int
// ctx (extra): number of extra stack items to skip when returning results.
//   pcall: ctx=0 (no extra items to skip)
//   xpcall: ctx=2 (skip original func + handler)
func finishPcallCont(L *stateapi.LuaState, status int, ctx int) int {
	if status != stateapi.StatusOK && status != stateapi.StatusYield {
		// Error path: push false, then the error message
		// The error object is already at L.Top-1 (placed by SetErrorObj in finishPCallK)
		L.Stack[L.Top].Val = objectapi.False
		L.Top++
		// Swap: move false before error message
		// Stack: ... errMsg false → ... false errMsg
		L.Stack[L.Top-1].Val, L.Stack[L.Top-2].Val = L.Stack[L.Top-2].Val, L.Stack[L.Top-1].Val
		return 2
	}
	// Success: return all results minus 'extra' items to skip.
	// Mirrors: return lua_gettop(L) - (int)extra in C Lua
	return L.Top - (L.CI.Func + 1) - ctx
}

func luaB_pcall(L *luaapi.State) int {
	L.CheckAny(1)
	L.PushBoolean(true) // first result if no errors
	L.Insert(1)         // put it in place
	// Set continuation BEFORE calling PCall — this enables PATH B (yieldable).
	// Mirrors: luaB_pcall in lbaselib.c sets finishpcall as continuation.
	ls := L.Internal.(*stateapi.LuaState)
	ls.CI.K = finishPcallCont
	ls.CI.Ctx = 0
	status := L.PCall(L.GetTop()-2, luaapi.MultiRet, 0)
	return finishPcallCont(ls, status, 0)
}

func luaB_xpcall(L *luaapi.State) int {
	n := L.GetTop()
	L.CheckType(2, objectapi.TypeFunction) // check error function
	// Mirrors: luaB_xpcall in lbaselib.c
	// Stack: [func(1), handler(2), arg1(3), ..., argN]
	L.PushBoolean(true) // first result
	L.PushValue(1)      // function copy
	L.Rotate(3, 2)      // move true+func_copy below args
	// Stack: [func(1), handler(2), true(3), func_copy(4), arg1(5), ..., argN]
	// Set continuation BEFORE calling PCall — enables PATH B (yieldable).
	// Mirrors: lua_pcallk(L, n-2, MULTRET, 2, 2, finishpcall) in C Lua
	ls := L.Internal.(*stateapi.LuaState)
	ls.CI.K = finishPcallCont
	ls.CI.Ctx = 2 // skip 2 items (func + handler) when returning results
	status := L.PCall(n-2, luaapi.MultiRet, 2)
	return finishPcallCont(ls, status, 2)
}

// isValidLoadMode checks if a load mode string is valid ("b", "t", "bt", "tb").
func isValidLoadMode(mode string) bool {
	for _, c := range mode {
		if c != 'b' && c != 't' {
			return false
		}
	}
	return len(mode) > 0 && len(mode) <= 2
}

func luaB_load(L *luaapi.State) int {
	s, ok := L.ToString(1)
	mode := L.OptString(3, "bt")
	env := 0
	if !L.IsNone(4) {
		env = 4
	}
	// Validate mode: C Lua's load checks mode via luaL_argcheck
	if !isValidLoadMode(mode) {
		L.ArgError(3, "invalid mode")
		return 0
	}
	var status int
	if ok { // loading a string
		chunkname := L.OptString(2, s)
		status = L.Load(s, chunkname, mode)
	} else {
		// loading from a reader function
		chunkname := L.OptString(2, "=(load)")
		L.CheckType(1, 6) // TypeFunction
		// Call reader repeatedly, collecting strings
		var chunks []string
		readerErr := false
		for {
			L.PushValue(1) // push reader function
			st := L.PCall(0, 1, 0)
			if st != luaapi.StatusOK {
				// reader function errored — return nil, errmsg
				return loadAux(L, st, env)
			}
			if L.IsNil(-1) || L.IsNone(-1) {
				L.Pop(1)
				break
			}
			chunk, isStr := L.ToString(-1)
			if !isStr {
				L.Pop(1)
				readerErr = true
				break
			}
			if len(chunk) == 0 {
				L.Pop(1)
				break
			}
			chunks = append(chunks, chunk)
			L.Pop(1)
		}
		if readerErr {
			L.PushString(chunkname + ": reader function must return a string")
			return loadAux(L, luaapi.StatusErrSyntax, env)
		}
		combined := strings.Join(chunks, "")
		status = L.Load(combined, chunkname, mode)
	}
	return loadAux(L, status, env)
}

// loadAux handles load/loadfile results — matches C Lua's load_aux.
func loadAux(L *luaapi.State, status, env int) int {
	if status == luaapi.StatusOK {
		if env != 0 {
			L.PushValue(env) // push env table
			if _, ok := L.SetUpvalue(-2, 1); !ok {
				L.Pop(1) // remove 'env' if not used (no upvalue to set)
			}
		}
		return 1
	}
	// error: push fail, then error message
	L.PushFail()
	L.Insert(-2) // fail before error message
	return 2
}

// dofileCont is the continuation for dofile() after yield from the loaded chunk.
// Mirrors: dofilecont in lbaselib.c:419
func dofileCont(L *stateapi.LuaState, status int, ctx int) int {
	return L.Top - (L.CI.Func + 1) - 1
}

func luaB_dofile(L *luaapi.State) int {
	fname := L.OptString(1, "")
	L.SetTop(1)
	if luaB_loadfileImpl(L, fname) != 1 {
		// loadfile returned (nil, errmsg) — error
		L.Error()
		return 0
	}
	// Call the loaded chunk, passing through all results.
	// Use CallK so that yields inside the dofile'd chunk can be resumed.
	L.CallK(0, luaapi.MultiRet, 0, dofileCont)
	ls := L.Internal.(*stateapi.LuaState)
	return dofileCont(ls, 0, 0)
}

func luaB_loadfile(L *luaapi.State) int {
	fname := L.OptString(1, "")
	return luaB_loadfileImpl(L, fname)
}

// luaB_loadfileImpl implements loadfile(filename [, mode [, env]])
func luaB_loadfileImpl(L *luaapi.State, fname string) int {
	mode := L.OptString(2, "bt")
	env := 0
	if !L.IsNone(3) {
		env = 3
	}

	if fname == "" {
		// Read from stdin — not commonly needed for tests
		L.PushFail()
		L.PushString("loadfile from stdin not supported")
		return 2
	}

	data, err := os.ReadFile(fname)
	if err != nil {
		L.PushFail()
		L.PushString(err.Error())
		return 2
	}

	// Skip UTF-8 BOM if present
	src := string(data)
	if strings.HasPrefix(src, "\xEF\xBB\xBF") {
		src = src[3:]
	}

	// Skip first line if it starts with '#' (shebang)
	if len(src) > 0 && src[0] == '#' {
		idx := strings.IndexByte(src, '\n')
		if idx >= 0 {
			src = src[idx:] // keep the newline (preserves line numbering)
			// If binary signature follows the newline, skip it to expose the signature
			if len(src) > 1 && src[0] == '\n' && src[1] == '\x1b' {
				src = src[1:]
			}
		} else {
			src = "" // single-line shebang, no more content
		}
	}

	chunkname := "@" + fname
	status := L.Load(src, chunkname, mode)
	return loadAux(L, status, env)
}

func luaB_collectgarbage(L *luaapi.State) int {
	opts := []string{"stop", "restart", "collect", "count", "step", "isrunning", "generational", "incremental", "param"}
	o := L.CheckOption(1, "collect", opts)
	switch o {
	case 0: // stop
		L.SetGCStopped(true)
		L.PushInteger(0)
		return 1
	case 1: // restart
		L.SetGCStopped(false)
		L.PushInteger(0)
		return 1
	case 2: // collect
		// V5: Run Lua mark-and-sweep GC cycle (includes calling __gc finalizers)
		L.GCCollect()
		L.SweepStrings()
		L.PushInteger(0)
		return 1
	case 3: // count
		kb := float64(L.GCTotalBytes()) / 1024.0
		L.PushNumber(kb)
		return 1
	case 4: // step
		// V5: Run Lua mark-and-sweep GC step
		L.GCCollect()
		L.SweepStrings()
		L.PushBoolean(true)
		return 1
	case 5: // isrunning
		L.PushBoolean(L.IsGCRunning())
		return 1
	case 6: // generational
		prev := L.SetGCMode("generational")
		L.PushString(prev)
		return 1
	case 7: // incremental
		prev := L.SetGCMode("incremental")
		L.PushString(prev)
		return 1
	case 8: // param
		params := []string{"minormul", "majorminor", "minormajor", "pause", "stepmul", "stepsize"}
		p := L.CheckOption(2, "", params)
		paramName := params[p]
		value := L.OptInteger(3, -1)
		if value < 0 {
			// query only — return current value
			L.PushInteger(L.GetGCParam(paramName))
		} else {
			// set and return previous value
			prev := L.SetGCParam(paramName, value)
			L.PushInteger(prev)
		}
		return 1
	default:
		L.PushInteger(0)
		return 1
	}
}

// OpenBase opens the base library into _G.
func OpenBase(L *luaapi.State) int {
	L.PushGlobalTable()
	baseFuncs := map[string]luaapi.CFunction{
		"assert":         luaB_assert,
		"collectgarbage": luaB_collectgarbage,
		"dofile":         luaB_dofile,
		"error":          luaB_error,
		"getmetatable":   luaB_getmetatable,
		"ipairs":         luaB_ipairs,
		"load":           luaB_load,
		"loadfile":       luaB_loadfile,
		"next":           luaB_next,
		"pairs":          luaB_pairs,
		"pcall":          luaB_pcall,
		"print":          luaB_print,
		"rawequal":       luaB_rawequal,
		"rawget":         luaB_rawget,
		"rawlen":         luaB_rawlen,
		"rawset":         luaB_rawset,
		"require":        luaB_require,
		"select":         luaB_select,
		"setmetatable":   luaB_setmetatable,
		"tonumber":       luaB_tonumber,
		"tostring":       luaB_tostring,
		"type":           luaB_type,
		"unpack":         tabUnpack, // global alias for table.unpack (Lua 5.5 compat)
		"warn":           luaB_warn,
		"xpcall":         luaB_xpcall,
	}
	L.SetFuncs(baseFuncs, 0)
	// Set global _G
	L.PushValue(-1)
	L.SetField(-2, "_G")
	// Set _VERSION
	L.PushString("Lua 5.5")
	L.SetField(-2, "_VERSION")
	return 1
}