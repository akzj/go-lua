// Package api provides the public Lua API
// stdlib_debug.go - Debug library implementation
//
// This file implements the Lua 5.4 debug library functions.
//
// # Contract Invariants
//
// 1. debug.getinfo(level) returns nil for invalid levels
// 2. debug.getlocal(level, n) returns (name, value) or nil
// 3. debug.traceback() always returns a string
// 4. All debug functions must not crash on invalid input
//
// # Implementation Notes
//
// - Level 1 = current function, Level 2 = caller, etc.
// - Local variable indices are 1-based in Lua
// - Register indices are 0-based in the VM
// - LocVars must be populated by codegen before debug functions work
package api

import (
	"fmt"
	"strings"

	"github.com/akzj/go-lua/pkg/object"
	"github.com/akzj/go-lua/pkg/vm"
)

// openDebugLib opens the debug library.
// This is called by OpenLibs() to register the debug module.
func (s *State) openDebugLib() {
	// Create debug table
	s.NewTable()
	debugIdx := s.GetTop()

	// Register debug functions
	s.PushFunction(debugGetinfo)
	s.SetField(debugIdx, "getinfo")

	s.PushFunction(debugGetlocal)
	s.SetField(debugIdx, "getlocal")

	s.PushFunction(debugSetlocal)
	s.SetField(debugIdx, "setlocal")

	s.PushFunction(debugTraceback)
	s.SetField(debugIdx, "traceback")

	s.PushFunction(debugDebug)
	s.SetField(debugIdx, "debug")

	s.PushFunction(debugGethook)
	s.SetField(debugIdx, "gethook")

	s.PushFunction(debugSethook)
	s.SetField(debugIdx, "sethook")

	s.PushFunction(debugSetmetatable)
	s.SetField(debugIdx, "setmetatable")

	// Register as "debug" global
	s.SetGlobal("debug")
}

// debugGetinfo implements debug.getinfo([thread,] f [, what])
//
// Returns a table with information about a function.
//
// Parameters:
//   - thread: Optional thread (not supported yet)
//   - f: Function or stack level (1 = current, 2 = caller, etc.)
//   - what: Optional string specifying which fields to include
//
// Returns:
//   - Table with function info, or nil if level/function not found
//
// Invariants:
//   - Returns nil for invalid levels
//   - Always includes source, linedefined, lastlinedefined, what for Lua functions
//   - what="Lua" for Lua functions, "C" for Go functions, "main" for main chunk
func debugGetinfo(L *State) int {
	// Parse arguments
	level := 0 // 0 means function argument
	what := "flnStu"
	var funcArg *object.Closure // Direct function argument

	argIdx := 1

	// Check if first arg is a number (level) or function
	if L.IsNumber(argIdx) {
		levelFloat, _ := L.ToNumber(argIdx)
		level = int(levelFloat)
		argIdx++
		if L.IsString(argIdx) {
			what, _ = L.ToString(argIdx)
		}
	} else if L.IsFunction(argIdx) {
		// Get the function from the stack
		funcVal := L.vm.GetStack(argIdx)
		if funcVal != nil && funcVal.Type == object.TypeFunction {
			funcArg = funcVal.Value.GC.(*object.Closure)
		}
		argIdx++
		if L.IsString(argIdx) {
			what, _ = L.ToString(argIdx)
		}
	}

	// Build info table
	L.NewTable()
	tableIdx := L.GetTop()

	// Get prototype and function info
	var proto *object.Prototype
	var isGo bool
	var ci *vm.CallInfo

	if funcArg != nil {
		// Direct function argument
		isGo = funcArg.IsGo
		if !isGo {
			proto = funcArg.Proto
		}
	} else {
		// Stack level argument
		ci = L.getCallInfoAtLevel(level)
		if ci == nil {
			L.PushNil()
			return 1
		}
		if ci.Closure != nil {
			isGo = ci.Closure.IsGo
			if !isGo {
				proto = ci.Closure.Proto
			}
		}
	}

	// Populate fields based on 'what'
	for _, ch := range what {
		switch ch {
		case 'f':
			// func: the function itself
			if funcArg != nil {
				L.vm.Push(object.TValue{
					Type:  object.TypeFunction,
					Value: object.Value{GC: funcArg},
				})
				L.SetField(tableIdx, "func")
			} else if ci != nil && ci.Func != nil {
				L.vm.Push(*ci.Func)
				L.SetField(tableIdx, "func")
			}
		case 'l':
			// linedefined, lastlinedefined, currentline
			if proto != nil {
				L.PushNumber(float64(0)) // linedefined - not tracked yet
				L.SetField(tableIdx, "linedefined")
				L.PushNumber(float64(0)) // lastlinedefined - not tracked yet
				L.SetField(tableIdx, "lastlinedefined")
				// currentline - get from LineInfo if we have a CallInfo
				currentLine := 0
				if ci != nil {
					pc := ci.PC
					if pc >= 0 && pc < len(proto.LineInfo) {
						currentLine = proto.LineInfo[pc]
					}
				}
				L.PushNumber(float64(currentLine))
				L.SetField(tableIdx, "currentline")
			}
		case 'n':
			// name, namewhat - function name
			L.PushString("") // name - not tracked yet
			L.SetField(tableIdx, "name")
			L.PushString("") // namewhat
			L.SetField(tableIdx, "namewhat")
		case 'S':
			// source, short_src, what, linedefined, lastlinedefined
			if proto != nil {
				source := proto.Source
				if source == "" {
					source = "=?"
				}
				L.PushString(source)
				L.SetField(tableIdx, "source")

				// short_src
				shortSrc := source
				if len(shortSrc) > 60 {
					shortSrc = shortSrc[:60]
				}
				L.PushString(shortSrc)
				L.SetField(tableIdx, "short_src")

				// what - use isGo to determine, not source prefix
				if isGo {
					L.PushString("C")
				} else {
					// Lua function (could be "main" for main chunk, but "Lua" works too)
					L.PushString("Lua")
				}
				L.SetField(tableIdx, "what")

				// linedefined and lastlinedefined
				L.PushNumber(float64(0)) // linedefined - not tracked yet
				L.SetField(tableIdx, "linedefined")
				L.PushNumber(float64(0)) // lastlinedefined - not tracked yet
				L.SetField(tableIdx, "lastlinedefined")

				// currentline
				currentLine := 0
				if ci != nil {
					pc := ci.PC
					if pc >= 0 && pc < len(proto.LineInfo) {
						currentLine = proto.LineInfo[pc]
					}
				}
				L.PushNumber(float64(currentLine))
				L.SetField(tableIdx, "currentline")
			} else if isGo {
				L.PushString("=[C]")
				L.SetField(tableIdx, "source")
				L.PushString("[C]")
				L.SetField(tableIdx, "short_src")
				L.PushString("C")
				L.SetField(tableIdx, "what")
				L.PushNumber(float64(-1)) // linedefined for C functions
				L.SetField(tableIdx, "linedefined")
				L.PushNumber(float64(-1)) // lastlinedefined for C functions
				L.SetField(tableIdx, "lastlinedefined")
				L.PushNumber(float64(-1)) // currentline for C functions
				L.SetField(tableIdx, "currentline")
			}
		case 't':
			// istailcall - not supported yet
			L.PushBoolean(false)
			L.SetField(tableIdx, "istailcall")
		case 'u':
			// nups, nparams, isvararg
			if proto != nil {
				L.PushNumber(float64(len(proto.Upvalues)))
				L.SetField(tableIdx, "nups")
				L.PushNumber(float64(proto.NumParams))
				L.SetField(tableIdx, "nparams")
				L.PushBoolean(proto.IsVarArg)
				L.SetField(tableIdx, "isvararg")
			} else if isGo {
				L.PushNumber(float64(0))
				L.SetField(tableIdx, "nups")
				L.PushNumber(float64(0))
				L.SetField(tableIdx, "nparams")
				L.PushBoolean(false)
				L.SetField(tableIdx, "isvararg")
			}
		}
	}

	return 1
}

// debugGetlocal implements debug.getlocal([thread,] f, local)
//
// Returns the name and value of a local variable.
//
// Parameters:
//   - thread: Optional thread (not supported yet)
//   - f: Stack level (1 = current, 2 = caller, etc.)
//   - local: Local variable index (1-based)
//
// Returns:
//   - name: Variable name (string), or nil if not found
//   - value: Variable value
//
// Invariants:
//   - Returns nil if level or local index is invalid
//   - Local indices are 1-based (Lua convention)
//   - Must handle both parameters and local variables
func debugGetlocal(L *State) int {
	// Get level and local index from arguments
	level, _ := L.ToNumber(1)
	localIdx, _ := L.ToNumber(2)

	// Get CallInfo at level
	ci := L.getCallInfoAtLevel(int(level))
	if ci == nil {
		L.PushNil()
		return 1
	}

	// Get prototype for Lua function
	if ci.Closure == nil || ci.Closure.IsGo {
		L.PushNil()
		return 1
	}

	proto := ci.Closure.Proto
	if proto == nil {
		L.PushNil()
		return 1
	}

	// Find local variable at given index active at current PC
	// Use the current PC from CallInfo
	pc := ci.PC
	if pc < 0 {
		pc = 0
	}

	name, regIdx := findLocalAtPC(proto, pc, int(localIdx))
	if name == "" {
		L.PushNil()
		return 1
	}

	// Get value from stack
	stackIdx := ci.Base + regIdx
	if stackIdx < 0 || stackIdx >= len(L.vm.Stack) {
		L.PushNil()
		return 1
	}

	value := L.vm.Stack[stackIdx]

	// Return name and value
	L.PushString(name)
	L.vm.Push(value)
	return 2
}

// debugSetlocal implements debug.setlocal([thread,] f, local, value)
//
// Sets the value of a local variable.
//
// Parameters:
//   - thread: Optional thread (not supported yet)
//   - f: Stack level (1 = current, 2 = caller, etc.)
//   - local: Local variable index (1-based)
//   - value: New value to set
//
// Returns:
//   - name: Variable name if found, or nil if not found
//
// Invariants:
//   - Returns "no variable" string if variable doesn't exist
//   - Must not crash if level or index is invalid
func debugSetlocal(L *State) int {
	// Get level, local index, and value from arguments
	level, _ := L.ToNumber(1)
	localIdx, _ := L.ToNumber(2)

	// Get CallInfo at level
	ci := L.getCallInfoAtLevel(int(level))
	if ci == nil {
		L.PushNil()
		return 1
	}

	// Get prototype for Lua function
	if ci.Closure == nil || ci.Closure.IsGo {
		L.PushNil()
		return 1
	}

	proto := ci.Closure.Proto
	if proto == nil {
		L.PushNil()
		return 1
	}

	// Find local variable at given index
	pc := ci.PC
	if pc < 0 {
		pc = 0
	}

	name, regIdx := findLocalAtPC(proto, pc, int(localIdx))
	if name == "" {
		L.PushNil()
		return 1
	}

	// Get value from stack (argument 3)
	valueIdx := 3
	if !L.IsNil(valueIdx) {
		// Get the value from the stack
		val := L.vm.GetStack(valueIdx)
		if val != nil {
			// Set value in stack
			stackIdx := ci.Base + regIdx
			if stackIdx >= 0 && stackIdx < len(L.vm.Stack) {
				L.vm.Stack[stackIdx] = *val
			}
		}
	}

	// Return name
	L.PushString(name)
	return 1
}

// debugTraceback implements debug.traceback([thread,] [message [, level]])
//
// Returns a string with a stack traceback.
//
// Parameters:
//   - thread: Optional thread (not supported yet)
//   - message: Optional message to prepend
//   - level: Optional starting level (default 1)
//
// Returns:
//   - String with traceback
//
// Invariants:
//   - Always returns a string, never nil
//   - Includes message if provided
//   - Shows source:line for each frame
func debugTraceback(L *State) int {
	// Parse optional message and level
	message := ""
	level := 1

	argIdx := 1
	if L.IsString(argIdx) {
		message, _ = L.ToString(argIdx)
		argIdx++
	}

	if L.IsNumber(argIdx) {
		levelFloat, _ := L.ToNumber(argIdx)
		level = int(levelFloat)
	}

	// Build traceback string
	var sb strings.Builder

	if message != "" {
		sb.WriteString(message)
		sb.WriteString("\n")
	}

	sb.WriteString("stack traceback:")

	// Walk call stack
	for i := level; ; i++ {
		ci := L.getCallInfoAtLevel(i)
		if ci == nil {
			break
		}

		sb.WriteString("\n\t")
		sb.WriteString(formatFrame(ci, L.vm))
	}

	L.PushString(sb.String())
	return 1
}

// debugDebug implements debug.debug()
//
// Enters an interactive debug mode.
//
// Returns:
//   - No return value
//
// Invariants:
//   - Reads commands from stdin until "cont" is entered
//   - Executes each command as Lua code
func debugDebug(L *State) int {
	// Simple implementation - just return
	// Full implementation would read from stdin and execute commands
	return 0
}

// debugGethook implements debug.gethook([thread])
//
// Returns current hook settings.
//
// Parameters:
//   - thread: Optional thread (not supported yet)
//
// Returns:
//   - hook: Hook function or nil
//   - mask: Hook mask string
//   - count: Hook count
//
// Invariants:
//   - Returns nil if no hook is set
func debugGethook(L *State) int {
	hook, mask, count := L.vm.GetHook()
	
	if hook == nil {
		L.PushNil()
	} else {
		L.vm.Push(object.TValue{
			Type: object.TypeFunction,
			Value: object.Value{GC: hook},
		})
	}
	
	// Build mask string
	maskStr := ""
	if mask&1 != 0 {
		maskStr += "c"
	}
	if mask&2 != 0 {
		maskStr += "r"
	}
	if mask&4 != 0 {
		maskStr += "l"
	}
	L.PushString(maskStr)
	L.PushNumber(float64(count))
	
	return 3
}

// debugSethook implements debug.sethook([thread,] hook, mask [, count])
//
// Sets a debug hook function.
//
// Parameters:
//   - thread: Optional thread (not supported yet)
//   - hook: Hook function or nil to remove hook
//   - mask: Hook mask ("c" for call, "r" for return, "l" for line)
//   - count: Optional count for count hooks
//
// Returns:
//   - No return value
//
// Invariants:
//   - nil hook removes existing hook
//   - Empty mask disables all hooks
func debugSethook(L *State) int {
	// Get arguments
	argIdx := 1
	
	// Check if hook is nil or a function
	var hookClosure *object.Closure
	if L.IsFunction(argIdx) {
		// Get the function from the stack
		funcVal := L.vm.GetStack(argIdx)
		if funcVal != nil && funcVal.Type == object.TypeFunction {
			hookClosure = funcVal.Value.GC.(*object.Closure)
		}
	}
	// If nil or not a function, hookClosure stays nil
	
	argIdx++
	
	// Get mask
	mask := uint8(0)
	if L.IsString(argIdx) {
		maskStr, _ := L.ToString(argIdx)
		for _, ch := range maskStr {
			switch ch {
			case 'c':
				mask |= 1
			case 'r':
				mask |= 2
			case 'l':
				mask |= 4
			}
		}
	}
	
	argIdx++
	
	// Get count
	count := 0
	if L.IsNumber(argIdx) {
		countFloat, _ := L.ToNumber(argIdx)
		count = int(countFloat)
	}
	
	// Set the hook
	L.vm.SetHook(hookClosure, mask, count)
	
	return 0
}

// ============================================================================
// Helper Functions
// ============================================================================

// getCallInfoAtLevel returns the CallInfo at the given stack level.
// Level 1 is the current function, level 2 is its caller, etc.
//
// Parameters:
//   - level: Stack level (1-based)
//
// Returns:
//   - *CallInfo: Call info at level, or nil if invalid
//
// Invariant: level 1 always returns current frame if VM.CI >= 0
func (s *State) getCallInfoAtLevel(level int) *vm.CallInfo {
	return s.vm.GetCallInfoAtLevel(level)
}

// findLocalAtPC finds the local variable at the given index that is active at PC.
//
// Parameters:
//   - proto: Function prototype
//   - pc: Program counter
//   - localIdx: Local variable index (1-based)
//
// Returns:
//   - name: Variable name, or "" if not found
//   - regIdx: Register index for the variable
//
// Invariant: Returns ("", 0) if not found
func findLocalAtPC(proto *object.Prototype, pc int, localIdx int) (name string, regIdx int) {
	if proto == nil || localIdx < 1 {
		return "", 0
	}

	// First, collect all variables and sort by register index
	// In Lua, local variables are numbered by their register position
	var allVars []struct {
		name   string
		regIdx int
	}

	for i := range proto.LocVars {
		lv := &proto.LocVars[i]
		if lv.RegIndex >= 0 {
			allVars = append(allVars, struct {
				name   string
				regIdx int
			}{lv.Name, lv.RegIndex})
		}
	}

	// Sort by register index
	for i := 0; i < len(allVars); i++ {
		for j := i + 1; j < len(allVars); j++ {
			if allVars[j].regIdx < allVars[i].regIdx {
				allVars[i], allVars[j] = allVars[j], allVars[i]
			}
		}
	}

	// Try to find active variables first
	var activeVars []struct {
		name   string
		regIdx int
	}

	for i := range proto.LocVars {
		lv := &proto.LocVars[i]
		// Check if variable is active at PC
		if lv.Start <= pc && pc <= lv.End && lv.RegIndex >= 0 {
			activeVars = append(activeVars, struct {
				name   string
				regIdx int
			}{lv.Name, lv.RegIndex})
		}
	}

	// Sort active vars by register index
	for i := 0; i < len(activeVars); i++ {
		for j := i + 1; j < len(activeVars); j++ {
			if activeVars[j].regIdx < activeVars[i].regIdx {
				activeVars[i], activeVars[j] = activeVars[j], activeVars[i]
			}
		}
	}

	// Return the nth active variable if found
	if localIdx <= len(activeVars) {
		return activeVars[localIdx-1].name, activeVars[localIdx-1].regIdx
	}

	// Fallback: return the nth variable by register index
	// This handles the case where PC range check fails
	if localIdx <= len(allVars) {
		return allVars[localIdx-1].name, allVars[localIdx-1].regIdx
	}

	return "", 0
}

// formatFrame formats a single call frame for traceback.
//
// Parameters:
//   - ci: Call info for the frame
//   - vm: VM instance
//
// Returns:
//   - Formatted string like "source:line: in function 'name'"
//
// Invariant: Never returns empty string
func formatFrame(ci *vm.CallInfo, vmInst *vm.VM) string {
	if ci == nil {
		return "?"
	}

	// Get source and line from prototype
	var source string
	var line int

	if ci.Closure != nil && !ci.Closure.IsGo && ci.Closure.Proto != nil {
		proto := ci.Closure.Proto
		source = proto.Source
		if source == "" {
			source = "=?"
		}

		// Get line from LineInfo
		pc := ci.PC
		if pc >= 0 && pc < len(proto.LineInfo) {
			line = proto.LineInfo[pc]
		}
	} else {
		source = "[C]"
		line = 0
	}

	// Format the frame
	if line > 0 {
		return fmt.Sprintf("%s:%d: in ?", source, line)
	}
	return fmt.Sprintf("%s: in ?", source)
}

// debugSetmetatable implements debug.setmetatable(value, metatable)
// Sets the metatable for the given value and returns the value.
// This is the debug version that doesn't trigger __metatable metamethod.
func debugSetmetatable(L *State) int {
	if L.GetTop() < 2 {
		L.PushString("bad argument #1 to 'setmetatable' (value expected)")
		L.Error()
		return 0
	}

	// Get the value (first argument)
	value := L.vm.GetStack(1)

	// Get the metatable (second argument) - can be nil or table
	mtValue := L.vm.GetStack(2)

	var mt *object.Table
	if mtValue.IsNil() {
		mt = nil
	} else if mtValue.IsTable() {
		mt, _ = mtValue.ToTable()
	} else {
		L.PushString("bad argument #2 to 'setmetatable' (nil or table expected)")
		L.Error()
		return 0
	}

	// Only tables can have their metatable set directly
	if value.IsTable() {
		tbl, _ := value.ToTable()
		tbl.SetMetatable(mt)
	}

	// Return the value
	L.vm.Push(*value)
	return 1
}