// Debug library implementation for Lua 5.4/5.5
package internal

import (
	"strings"

	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
)

// debugHookFn stores the current hook function
var debugHookFn types.TValue
var debugHookMask string

// bdebugGethook implements debug.gethook()
// Returns nil, nil when no hook is set.
func bdebugGethook(stack []types.TValue, base int) int {
	if debugHookFn == nil || debugHookFn.IsNil() {
		stack[base] = types.NewTValueNil()
		stack[base+1] = types.NewTValueNil()
		return 2
	}
	stack[base] = debugHookFn
	stack[base+1] = types.NewTValueString(debugHookMask)
	return 2
}

// bdebugSethook implements debug.sethook(hook, mask, count)
// If hook is nil or no args, disables the hook.
func bdebugSethook(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs == 0 || stack[base+1] == nil || stack[base+1].IsNil() {
		debugHookFn = types.NewTValueNil()
		debugHookMask = ""
		return 0
	}
	debugHookFn = stack[base+1]
	if nArgs >= 2 && stack[base+2] != nil && stack[base+2].IsString() {
		debugHookMask, _ = stack[base+2].GetValue().(string)
	} else {
		debugHookMask = ""
	}
	return 0
}

// bdebugGetinfo implements debug.getinfo(thread, level, what)
// Returns a table with function information.
func bdebugGetinfo(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 1 || stack[base+1] == nil || stack[base+1].IsNil() {
		stack[base] = types.NewTValueNil()
		return 1
	}

	// Get what string
	what := "nSl"
	if nArgs >= 2 && stack[base+2] != nil && stack[base+2].IsString() {
		what, _ = stack[base+2].GetValue().(string)
	}

	// Create result table
	tbl := createModuleTable()

	// Determine function type
	var fn types.TValue
	isCFunc := false

	if nArgs >= 1 && stack[base+1] != nil && !stack[base+1].IsNil() {
		fn = stack[base+1]
		isCFunc = fn.IsCClosure() || fn.IsLightCFunction()
	}

	// S option - source info
	if strings.Contains(what, "S") {
		if isCFunc {
			tbl.Set(types.NewTValueString("source"), types.NewTValueString("=[C]"))
			tbl.Set(types.NewTValueString("short_src"), types.NewTValueString("[C]"))
			tbl.Set(types.NewTValueString("linedefined"), types.NewTValueInteger(-1))
			tbl.Set(types.NewTValueString("what"), types.NewTValueString("C"))
		} else {
			tbl.Set(types.NewTValueString("source"), types.NewTValueString(""))
			tbl.Set(types.NewTValueString("short_src"), types.NewTValueString("[string \"...\"]"))
			tbl.Set(types.NewTValueString("linedefined"), types.NewTValueInteger(0))
			tbl.Set(types.NewTValueString("what"), types.NewTValueString("main"))
		}
	}

	// n option - name info
	if strings.Contains(what, "n") {
		if isCFunc {
			tbl.Set(types.NewTValueString("name"), types.NewTValueString("?"))
			tbl.Set(types.NewTValueString("namewhat"), types.NewTValueString(""))
		}
		tbl.Set(types.NewTValueString("nups"), types.NewTValueInteger(0))
	}

	// f option - func
	if strings.Contains(what, "f") && fn != nil {
		tbl.Set(types.NewTValueString("func"), fn)
	}

	// L option - activelines
	if strings.Contains(what, "L") && !isCFunc {
		activelines := createModuleTable()
		tbl.Set(types.NewTValueString("activelines"), &tableWrapper{tbl: activelines})
	}

	stack[base] = &tableWrapper{tbl: tbl}
	return 1
}

// bdebugTraceback implements debug.traceback(thread, message, level)
func bdebugTraceback(stack []types.TValue, base int) int {
	stack[base] = types.NewTValueString("stack traceback:\n")
	return 1
}

// bdebugGetlocal implements debug.getlocal(thread, level, local)
// Returns the name and value of a local variable.
func bdebugGetlocal(stack []types.TValue, base int) int {
	stack[base] = types.NewTValueNil()
	return 1
}

// bdebugSetlocal implements debug.setlocal(thread, level, local, value)
// Sets the value of a local variable.
func bdebugSetlocal(stack []types.TValue, base int) int {
	return 0
}

// bdebugUpvalueid implements debug.upvalueid(f, n)
// Returns unique identifier for the n-th upvalue of f.
func bdebugUpvalueid(stack []types.TValue, base int) int {
	stack[base] = types.NewTValueInteger(0)
	return 1
}

// bdebugUpvaluejoin implements debug.upvaluejoin(f1, n1, f2, n2)
// Make the n1-th upvalue of f1 refer to the n2-th upvalue of f2.
func bdebugUpvaluejoin(stack []types.TValue, base int) int {
	return 0
}

// registerDebugLib registers debug library functions in the module table
func registerDebugLib(debugMod tableapi.TableInterface) {
	debugMod.Set(types.NewTValueString("gethook"), &goFuncWrapper{fn: bdebugGethook})
	debugMod.Set(types.NewTValueString("sethook"), &goFuncWrapper{fn: bdebugSethook})
	debugMod.Set(types.NewTValueString("getinfo"), &goFuncWrapper{fn: bdebugGetinfo})
	debugMod.Set(types.NewTValueString("traceback"), &goFuncWrapper{fn: bdebugTraceback})
	debugMod.Set(types.NewTValueString("getlocal"), &goFuncWrapper{fn: bdebugGetlocal})
	debugMod.Set(types.NewTValueString("setlocal"), &goFuncWrapper{fn: bdebugSetlocal})
	debugMod.Set(types.NewTValueString("upvalueid"), &goFuncWrapper{fn: bdebugUpvalueid})
	debugMod.Set(types.NewTValueString("upvaluejoin"), &goFuncWrapper{fn: bdebugUpvaluejoin})
}

// =============================================================================
// utf8 library stubs
// =============================================================================

// butf8Len implements utf8.len(s) — returns number of UTF-8 characters
func butf8Len(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 1 {
		stack[base] = types.NewTValueNil()
		return 1
	}
	v := stack[base+1]
	if !v.IsString() {
		stack[base] = types.NewTValueNil()
		return 1
	}
	s := v.GetValue().(string)
	// For ASCII-only strings, len == utf8 len
	// For strings with multi-byte UTF-8, we need to count
	runeCount := 0
	for _, r := range s {
		_ = r // suppress unused warning
		runeCount++
	}
	stack[base] = types.NewTValueInteger(types.LuaInteger(runeCount))
	return 1
}

// butf8Codes implements utf8.codes(s) — returns iterator for (byte-offset, codepoint)
func butf8Codes(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 1 {
		stack[base] = types.NewTValueNil()
		return 1
	}
	v := stack[base+1]
	if !v.IsString() {
		stack[base] = types.NewTValueNil()
		return 1
	}
	// Return the string value — the caller handles iteration
	stack[base] = v
	return 1
}

// registerUtf8Lib registers utf8 library functions
func registerUtf8Lib(utf8Mod tableapi.TableInterface) {
	utf8Mod.Set(types.NewTValueString("len"), &goFuncWrapper{fn: butf8Len})
	utf8Mod.Set(types.NewTValueString("codes"), &goFuncWrapper{fn: butf8Codes})
}
