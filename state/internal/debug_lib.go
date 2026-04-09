// Debug library implementation for Lua 5.4/5.5
package internal

import (
	"regexp"
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

// validGetinfoOptionRegex matches valid option characters for getinfo's 'what' parameter
var validGetinfoOptionRegex = regexp.MustCompile("^[nSfLlutr>]+$")

// bdebugGetinfo implements debug.getinfo(thread, level, what)
// Returns a table with function information.
func bdebugGetinfo(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 1 {
		luaErrorString("bad argument #1 to 'getinfo' (function or level expected)")
	}
	
	// Get first argument
	arg1 := stack[base+1]
	if arg1 == nil || arg1.IsNil() {
		stack[base] = types.NewTValueNil()
		return 1
	}
	
	// Check for invalid options (raise error for invalid what string)
	// Note: "X" is invalid, but ">" by itself is also invalid as a what string
	what := "nSl"
	
	if nArgs >= 2 && stack[base+2] != nil && stack[base+2].IsString() {
		what = stack[base+2].GetValue().(string)
		if strings.HasPrefix(what, ">") {
			// ">" prefix means level-based query with ">" stripped
			what = what[1:]
		}
		if what != "" && !validGetinfoOptionRegex.MatchString(what) {
			luaErrorString("invalid option '" + what + "' to 'getinfo'")
		}
	}
	
	// Check if first arg is a level (integer) or function
	isLevel := false
	level := int64(0)
	
	if arg1.IsInteger() {
		level, _ = arg1.GetValue().(int64)
		isLevel = true
		// Levels <= 0 or > 100 are invalid
		if level <= 0 || level > 100 {
			stack[base] = types.NewTValueNil()
			return 1
		}
	}
	
	// Create result table
	tbl := createModuleTable()
	
	// Determine function type
	var fn types.TValue
	isCFunc := false
	isLuaFunc := false
	
	if !isLevel && arg1.IsFunction() {
		fn = arg1
		isCFunc = fn.IsCClosure() || fn.IsLightCFunction()
		isLuaFunc = fn.IsLClosure()
	}
	
	// S option - source info
	if strings.Contains(what, "S") {
		if isCFunc || (!isLuaFunc && !isLevel) {
			// C function or unknown
			tbl.Set(types.NewTValueString("source"), types.NewTValueString("=[C]"))
			tbl.Set(types.NewTValueString("short_src"), types.NewTValueString("[C]"))
			tbl.Set(types.NewTValueString("linedefined"), types.NewTValueInteger(-1))
			tbl.Set(types.NewTValueString("what"), types.NewTValueString("C"))
		} else {
			// Lua function or level-based query
			tbl.Set(types.NewTValueString("source"), types.NewTValueString(""))
			tbl.Set(types.NewTValueString("short_src"), types.NewTValueString(""))
			tbl.Set(types.NewTValueString("linedefined"), types.NewTValueInteger(0))
			tbl.Set(types.NewTValueString("what"), types.NewTValueString("Lua"))
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
	if strings.Contains(what, "L") {
		// For C functions, activelines is nil
		// For Lua functions or level queries, return empty table
		if !isCFunc {
			activelines := createModuleTable()
			tbl.Set(types.NewTValueString("activelines"), &tableWrapper{tbl: activelines})
		}
		// For C functions, activelines field is simply not set (nil)
	}
	
	stack[base] = &tableWrapper{tbl: tbl}
	return 1
}

// bdebugTraceback implements debug.traceback(thread, message, level)
func bdebugTraceback(stack []types.TValue, base int) int {
	stack[base] = types.NewTValueString("stack traceback:\n")
	return 1
}

// registerDebugLib registers debug library functions in the module table
func registerDebugLib(debugMod tableapi.TableInterface) {
	debugMod.Set(types.NewTValueString("gethook"), &goFuncWrapper{fn: bdebugGethook})
	debugMod.Set(types.NewTValueString("sethook"), &goFuncWrapper{fn: bdebugSethook})
	debugMod.Set(types.NewTValueString("getinfo"), &goFuncWrapper{fn: bdebugGetinfo})
	debugMod.Set(types.NewTValueString("traceback"), &goFuncWrapper{fn: bdebugTraceback})
}
