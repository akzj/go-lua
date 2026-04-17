// callerror.go — Function name resolution and type error messages.
// Mirrors: funcnamefromcode, funcnamefromcall, luaG_callerror, luaG_typeerror from ldebug.c
package api

import (
	"fmt"

	closureapi "github.com/akzj/go-lua/internal/closure/api"
	mmapi "github.com/akzj/go-lua/internal/metamethod/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	opcodeapi "github.com/akzj/go-lua/internal/opcode/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
)

// funcNameFromCode examines the instruction at pc to determine the function name.
// Returns (kind, name) or ("", "") if unknown.
// Mirrors: funcnamefromcode in ldebug.c
// FuncNameFromCode examines the instruction at pc to determine the function name.
// Returns (kind, name) or ("", "") if unknown.
// Mirrors: funcnamefromcode in ldebug.c
func FuncNameFromCode(L *stateapi.LuaState, p *objectapi.Proto, pc int) (string, string) {
	if pc < 0 || pc >= len(p.Code) {
		return "", ""
	}
	i := p.Code[pc]
	op := opcodeapi.GetOpCode(i)

	switch op {
	case opcodeapi.OP_CALL, opcodeapi.OP_TAILCALL:
		return BasicGetObjName(p, pc, opcodeapi.GetArgA(i))
	case opcodeapi.OP_TFORCALL:
		return "for iterator", "for iterator"
	case opcodeapi.OP_MMBIN, opcodeapi.OP_MMBINI, opcodeapi.OP_MMBINK:
		tm := opcodeapi.GetArgC(i)
		if tm < len(mmapi.TMNames) {
			name := mmapi.TMNames[tm]
			if len(name) > 2 {
				name = name[2:] // strip "__" prefix
			}
			return "metamethod", name
		}
	case opcodeapi.OP_GETTABUP, opcodeapi.OP_GETTABLE, opcodeapi.OP_GETI,
		opcodeapi.OP_GETFIELD, opcodeapi.OP_SELF:
		return "metamethod", "index"
	case opcodeapi.OP_SETTABUP, opcodeapi.OP_SETTABLE, opcodeapi.OP_SETI,
		opcodeapi.OP_SETFIELD:
		return "metamethod", "newindex"
	case opcodeapi.OP_UNM:
		return "metamethod", "unm"
	case opcodeapi.OP_BNOT:
		return "metamethod", "bnot"
	case opcodeapi.OP_LEN:
		return "metamethod", "len"
	case opcodeapi.OP_CONCAT:
		return "metamethod", "concat"
	case opcodeapi.OP_EQ:
		return "metamethod", "eq"
	case opcodeapi.OP_LT, opcodeapi.OP_LTI, opcodeapi.OP_GTI:
		return "metamethod", "lt"
	case opcodeapi.OP_LE, opcodeapi.OP_LEI, opcodeapi.OP_GEI:
		return "metamethod", "le"
	case opcodeapi.OP_CLOSE, opcodeapi.OP_RETURN:
		return "metamethod", "close"
	}
	return "", ""
}

// callErrorExtra builds the extra info string for a call error.
// Mirrors: luaG_callerror in ldebug.c
// Returns e.g. " (metamethod 'add')" or " (global 'foo')" or "".
func callErrorExtra(L *stateapi.LuaState, funcIdx int) string {
	// Try funcNameFromCode first (examines calling instruction)
	if L.CI != nil && L.CI.IsLua() {
		if cl, ok := L.Stack[L.CI.Func].Val.Val.(*closureapi.LClosure); ok && cl.Proto != nil {
			pc := L.CI.SavedPC - 1 // -1 to get the calling instruction
			kind, name := FuncNameFromCode(L, cl.Proto, pc)
			if kind != "" {
				return " (" + kind + " '" + name + "')"
			}
		}
	}
	// Fall back to VarInfo (traces register origin)
	base := 0
	if L.CI != nil {
		base = L.CI.Func + 1
	}
	reg := funcIdx - base
	if reg >= 0 {
		return VarInfo(L, reg)
	}
	return ""
}

// RunTypeError raises "attempt to <op> a <type> value <varinfo>".
// Mirrors: luaG_typeerror in ldebug.c
// reg is the register index (relative to CI base) holding the offending value.
// If reg < 0, no variable info is added.
func RunTypeError(L *stateapi.LuaState, val objectapi.TValue, op string, reg int) {
	typeName := objectapi.TypeNames[val.Type()]
	extra := ""
	if reg >= 0 {
		extra = VarInfo(L, reg)
	}
	RunError(L, fmt.Sprintf("attempt to %s a %s value%s", op, typeName, extra))
}

// RunTypeErrorByVal raises a type error, finding the variable name by examining
// the current VM instruction to determine which register holds the offending value.
// Mirrors: luaG_typeerror → varinfo in ldebug.c
func RunTypeErrorByVal(L *stateapi.LuaState, val objectapi.TValue, op string) {
	typeName := objectapi.TypeNames[val.Type()]
	extra := ""
	if L.CI != nil && L.CI.IsLua() {
		if cl, ok := L.Stack[L.CI.Func].Val.Val.(*closureapi.LClosure); ok && cl.Proto != nil {
			pc := L.CI.SavedPC - 1
			if pc >= 0 && pc < len(cl.Proto.Code) {
				inst := cl.Proto.Code[pc]
				iop := opcodeapi.GetOpCode(inst)
				reg := -1
				switch iop {
				// For GET* ops, the table is in register B (or upvalue B for GETTABUP)
				case opcodeapi.OP_GETTABLE, opcodeapi.OP_GETI, opcodeapi.OP_GETFIELD, opcodeapi.OP_SELF:
					reg = opcodeapi.GetArgB(inst)
				case opcodeapi.OP_GETTABUP:
					// Table is an upvalue — check upvalue name
					b := opcodeapi.GetArgB(inst)
					if b < len(cl.Proto.Upvalues) && cl.Proto.Upvalues[b].Name != nil {
						uname := cl.Proto.Upvalues[b].Name.Data
						if uname == "_ENV" {
							// For _ENV access, use the key (field C) as the global name
							k := opcodeapi.GetArgC(inst)
							if k < len(cl.Proto.Constants) && cl.Proto.Constants[k].IsString() {
								gname := cl.Proto.Constants[k].Val.(*objectapi.LuaString).Data
								extra = " (global '" + gname + "')"
							}
						} else {
							extra = " (upvalue '" + uname + "')"
						}
					}
				// For SET* ops, the table is in register A (or upvalue A for SETTABUP)
				case opcodeapi.OP_SETTABLE, opcodeapi.OP_SETI, opcodeapi.OP_SETFIELD:
					reg = opcodeapi.GetArgA(inst)
				case opcodeapi.OP_SETTABUP:
					a := opcodeapi.GetArgA(inst)
					if a < len(cl.Proto.Upvalues) && cl.Proto.Upvalues[a].Name != nil {
						uname := cl.Proto.Upvalues[a].Name.Data
						if uname == "_ENV" {
							k := opcodeapi.GetArgB(inst)
							if k < len(cl.Proto.Constants) && cl.Proto.Constants[k].IsString() {
								gname := cl.Proto.Constants[k].Val.(*objectapi.LuaString).Data
								extra = " (global '" + gname + "')"
							}
						} else {
							extra = " (upvalue '" + uname + "')"
						}
					}
				}
				// If we got a register, use VarInfo to get the name
				if reg >= 0 && extra == "" {
					extra = VarInfo(L, reg)
				}
			}
		}
	}
	RunError(L, fmt.Sprintf("attempt to %s a %s value%s", op, typeName, extra))
}
