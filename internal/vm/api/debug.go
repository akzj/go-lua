package api

// debug.go — Line number tracking and error message formatting.
// Mirrors: luaG_getfuncline, luaG_addinfo from ldebug.c

import (
	"fmt"

	closureapi "github.com/akzj/go-lua/internal/closure/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
)

const (
	// maxIWthAbs matches the codegen constant (MAXIWTHABS in C Lua).
	// An absolute line info entry is emitted every maxIWthAbs instructions.
	maxIWthAbs = 128

	// absLineInfo is the sentinel value in lineinfo signaling absolute info.
	absLineInfo = int8(-0x80) // -128
)

// getBaseLine returns the baseline (line number and PC) for computing the
// line of instruction at 'pc'. It searches the AbsLineInfo table.
// Mirrors: getbaseline in ldebug.c
func getBaseLine(f *objectapi.Proto, pc int) (baseline int, basepc int) {
	if len(f.AbsLineInfo) == 0 || pc < f.AbsLineInfo[0].PC {
		return f.LineDefined, -1
	}
	// Get an estimate — AbsLineInfo entries are placed ~every maxIWthAbs instructions
	i := pc/maxIWthAbs - 1
	if i < 0 {
		i = 0
	}
	// Adjust upward: estimate is a lower bound
	for i+1 < len(f.AbsLineInfo) && pc >= f.AbsLineInfo[i+1].PC {
		i++
	}
	return f.AbsLineInfo[i].Line, f.AbsLineInfo[i].PC
}

// GetFuncLine returns the source line corresponding to instruction 'pc'
// in proto 'f'. Returns -1 if no debug info is available.
// Mirrors: luaG_getfuncline in ldebug.c
func GetFuncLine(f *objectapi.Proto, pc int) int {
	if len(f.LineInfo) == 0 {
		return -1
	}
	baseline, basepc := getBaseLine(f, pc)
	for basepc++; basepc <= pc; basepc++ {
		if f.LineInfo[basepc] != absLineInfo {
			baseline += int(f.LineInfo[basepc])
		}
		// absLineInfo entries are handled by getBaseLine; skip them in the walk
	}
	return baseline
}

// getCurrentLine returns the current source line for a Lua call frame.
// Mirrors: getcurrentline in ldebug.c
func getCurrentLine(ci *stateapi.CallInfo, L *stateapi.LuaState) int {
	if !ci.IsLua() {
		return -1
	}
	cl, ok := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
	if !ok || cl.Proto == nil {
		return -1
	}
	pc := ci.SavedPC - 1
	if pc < 0 {
		pc = 0
	}
	return GetFuncLine(cl.Proto, pc)
}

// ShortSrc creates a short source name for error messages.
// Mirrors: luaO_chunkid in lobject.c
func ShortSrc(source string) string {
	if len(source) == 0 {
		return "[string \"?\"]"
	}
	if source[0] == '=' {
		if len(source) <= 60 {
			return source[1:]
		}
		return source[1:60]
	}
	if source[0] == '@' {
		if len(source) <= 60 {
			return source[1:]
		}
		return "..." + source[len(source)-57:]
	}
	// String source — find first newline
	first := source
	for i := 0; i < len(source); i++ {
		if source[i] == '\n' {
			first = source[:i]
			break
		}
	}
	if len(first) > 45 {
		first = first[:45] + "..."
	}
	return fmt.Sprintf("[string \"%s\"]", first)
}

// addInfo prepends "source:line: " to a message for the current Lua frame.
// Mirrors: luaG_addinfo in ldebug.c
func addInfo(L *stateapi.LuaState, msg string) string {
	ci := L.CI
	if ci == nil || !ci.IsLua() {
		return msg
	}
	cl, ok := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
	if !ok || cl.Proto == nil {
		return msg
	}
	line := getCurrentLine(ci, L)
	src := "?"
	if cl.Proto.Source != nil {
		src = ShortSrc(cl.Proto.Source.Data)
	}
	return fmt.Sprintf("%s:%d: %s", src, line, msg)
}
