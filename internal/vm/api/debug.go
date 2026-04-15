package api

// debug.go — Line number tracking, error message formatting, and variable info.
// Mirrors: luaG_getfuncline, luaG_addinfo, varinfo, kname from ldebug.c

import (
	"fmt"
	"strings"

	closureapi "github.com/akzj/go-lua/internal/closure/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	opcodeapi "github.com/akzj/go-lua/internal/opcode/api"
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
	// String source: [string "content"] or [string "content..."]
	// maxContent = LUA_IDSIZE(60) - len([string ") - len(...) - len("]) - 1
	const maxContent = 45
	nl := strings.IndexByte(source, '\n')
	srclen := len(source)
	if srclen <= maxContent && nl < 0 {
		// Short one-line source — keep as-is
		return `[string "` + source + `"]`
	}
	// Truncated: stop at first newline, clamp to maxContent, add "..."
	if nl >= 0 && nl < srclen {
		srclen = nl
	}
	if srclen > maxContent {
		srclen = maxContent
	}
	return `[string "` + source[:srclen] + `..."]`
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

// ---------------------------------------------------------------------------
// Variable info for error messages
// Mirrors: varinfo, kname, basicgetobjname from ldebug.c
// ---------------------------------------------------------------------------

// kname returns ("constant", name) if K[index] is a string constant.
// Mirrors: kname in ldebug.c
func kname(p *objectapi.Proto, index int) (kind string, name string) {
	if index < 0 || index >= len(p.Constants) {
		return "", "?"
	}
	kv := p.Constants[index]
	if kv.IsString() {
		return "constant", kv.Val.(*objectapi.LuaString).Data
	}
	if kv.IsInteger() {
		return "constant", fmt.Sprintf("%d", kv.Val.(int64))
	}
	if kv.IsFloat() {
		return "constant", fmt.Sprintf("%g", kv.Val.(float64))
	}
	return "", "?"
}

// kname2 returns just the string value of constant at index (for field names).
func kname2(p *objectapi.Proto, index int) string {
	if index < 0 || index >= len(p.Constants) {
		return "?"
	}
	kv := p.Constants[index]
	if kv.IsString() {
		return kv.Val.(*objectapi.LuaString).Data
	}
	return "?"
}

// findSetReg scans backward from 'lastpc' to find the instruction that
// last set register 'reg'. Returns the PC of that instruction, or -1.
// Simplified version of findsetreg in ldebug.c.
func findSetReg(p *objectapi.Proto, lastpc int, reg int) int {
	for pc := lastpc - 1; pc >= 0; pc-- {
		inst := p.Code[pc]
		op := opcodeapi.GetOpCode(inst)
		a := opcodeapi.GetArgA(inst)
		switch op {
		case opcodeapi.OP_LOADK, opcodeapi.OP_LOADKX, opcodeapi.OP_LOADFALSE,
			opcodeapi.OP_LOADTRUE, opcodeapi.OP_LOADNIL, opcodeapi.OP_LOADI,
			opcodeapi.OP_LOADF, opcodeapi.OP_MOVE, opcodeapi.OP_GETUPVAL,
			opcodeapi.OP_CLOSURE, opcodeapi.OP_GETTABUP, opcodeapi.OP_GETTABLE,
			opcodeapi.OP_GETI, opcodeapi.OP_GETFIELD, opcodeapi.OP_SELF:
			if a == reg {
				return pc
			}
		}
	}
	return -1
}

// basicGetObjName traces the origin of register 'reg' at 'pc' in proto 'p'.
// Returns (kind, name) where kind is "constant", "local", "upvalue", or "".
// Simplified version of basicgetobjname in ldebug.c.
func BasicGetObjName(p *objectapi.Proto, pc int, reg int) (kind string, name string) {
	setpc := findSetReg(p, pc, reg)
	if setpc < 0 {
		// No instruction found that sets this register — try local variable name
		if name := locVarName(p, pc, reg); name != "" {
			return "local", name
		}
		return "", ""
	}
	inst := p.Code[setpc]
	op := opcodeapi.GetOpCode(inst)
	switch op {
	case opcodeapi.OP_LOADK:
		return kname(p, opcodeapi.GetArgBx(inst))
	case opcodeapi.OP_LOADKX:
		if setpc+1 < len(p.Code) {
			return kname(p, opcodeapi.GetArgAx(p.Code[setpc+1]))
		}
	case opcodeapi.OP_MOVE:
		b := opcodeapi.GetArgB(inst)
		if b < opcodeapi.GetArgA(inst) {
			return BasicGetObjName(p, setpc, b)
		}
	case opcodeapi.OP_GETUPVAL:
		b := opcodeapi.GetArgB(inst)
		if b < len(p.Upvalues) && p.Upvalues[b].Name != nil {
			return "upvalue", p.Upvalues[b].Name.Data
		}
	case opcodeapi.OP_GETTABUP:
		// Table access from upvalue: upvalues[B][K[C]]
		k := opcodeapi.GetArgC(inst)
		if k < len(p.Constants) {
			return "field", kname2(p, k)
		}
	case opcodeapi.OP_GETFIELD:
		// Table field access: reg[A] = reg[B][K[C]]
		k := opcodeapi.GetArgC(inst)
		if k < len(p.Constants) {
			return "field", kname2(p, k)
		}
	}
	// Fallback: check LocVars for a local variable name at the call site PC.
	if name := locVarName(p, pc, reg); name != "" {
		return "local", name
	}
	return "", ""
}

// locVarName returns the local variable name for register 'reg' at instruction 'pc',
// or "" if not found. Mirrors: locvarname in ldebug.c.
func locVarName(p *objectapi.Proto, pc int, reg int) string {
	idx := 0
	for i := range p.LocVars {
		if p.LocVars[i].StartPC <= pc && pc < p.LocVars[i].EndPC {
			if idx == reg {
				if p.LocVars[i].Name != nil {
					return p.LocVars[i].Name.Data
				}
				return ""
			}
			idx++
		}
	}
	return ""
}

// VarInfo returns a formatted variable description for a register value,
// e.g. " (constant '15')" or " (local 'x')". Returns "" if unknown.
// Mirrors: varinfo + formatvarinfo in ldebug.c
func VarInfo(L *stateapi.LuaState, reg int) string {
	ci := L.CI
	if ci == nil || !ci.IsLua() {
		return ""
	}
	cl, ok := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
	if !ok || cl.Proto == nil {
		return ""
	}
	pc := ci.SavedPC - 1
	if pc < 0 {
		pc = 0
	}
	kind, name := BasicGetObjName(cl.Proto, pc, reg)
	if kind == "" {
		return ""
	}
	return fmt.Sprintf(" (%s '%s')", kind, name)
}
