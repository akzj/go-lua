// callerror.go — Function name resolution and type error messages.
// Mirrors: funcnamefromcode, funcnamefromcall, luaG_callerror, luaG_typeerror from ldebug.c
package vm

import (
	"fmt"

	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/metamethod"
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/opcode"
	"github.com/akzj/go-lua/internal/state"
)

// funcNameFromCode examines the instruction at pc to determine the function name.
// Returns (kind, name) or ("", "") if unknown.
// Mirrors: funcnamefromcode in ldebug.c
// FuncNameFromCode examines the instruction at pc to determine the function name.
// Returns (kind, name) or ("", "") if unknown.
// Mirrors: funcnamefromcode in ldebug.c
func FuncNameFromCode(L *state.LuaState, p *object.Proto, pc int) (string, string) {
	if pc < 0 || pc >= len(p.Code) {
		return "", ""
	}
	i := p.Code[pc]
	op := opcode.GetOpCode(i)

	switch op {
	case opcode.OP_CALL, opcode.OP_TAILCALL:
		return basicGetObjName(p, pc, opcode.GetArgA(i))
	case opcode.OP_TFORCALL:
		return "for iterator", "for iterator"
	case opcode.OP_MMBIN, opcode.OP_MMBINI, opcode.OP_MMBINK:
		tm := opcode.GetArgC(i)
		if tm < len(metamethod.TMNames) {
			name := metamethod.TMNames[tm]
			if len(name) > 2 {
				name = name[2:] // strip "__" prefix
			}
			return "metamethod", name
		}
	case opcode.OP_GETTABUP, opcode.OP_GETTABLE, opcode.OP_GETI,
		opcode.OP_GETFIELD, opcode.OP_SELF:
		return "metamethod", "index"
	case opcode.OP_SETTABUP, opcode.OP_SETTABLE, opcode.OP_SETI,
		opcode.OP_SETFIELD:
		return "metamethod", "newindex"
	case opcode.OP_UNM:
		return "metamethod", "unm"
	case opcode.OP_BNOT:
		return "metamethod", "bnot"
	case opcode.OP_LEN:
		return "metamethod", "len"
	case opcode.OP_CONCAT:
		return "metamethod", "concat"
	case opcode.OP_EQ:
		return "metamethod", "eq"
	case opcode.OP_LT, opcode.OP_LTI, opcode.OP_GTI:
		return "metamethod", "lt"
	case opcode.OP_LE, opcode.OP_LEI, opcode.OP_GEI:
		return "metamethod", "le"
	case opcode.OP_CLOSE, opcode.OP_RETURN:
		return "metamethod", "close"
	}
	return "", ""
}

// callErrorExtra builds the extra info string for a call error.
// Mirrors: luaG_callerror in ldebug.c
// Returns e.g. " (metamethod 'add')" or " (global 'foo')" or "".
func callErrorExtra(L *state.LuaState, funcIdx int) string {
	// Try funcNameFromCode first (examines calling instruction)
	if L.CI != nil && L.CI.IsLua() {
		if cl, ok := L.Stack[L.CI.Func].Val.Val.(*closure.LClosure); ok && cl.Proto != nil {
			pc := L.CI.SavedPC - 1 // -1 to get the calling instruction
			kind, name := FuncNameFromCode(L, cl.Proto, pc)
			if kind != "" {
				return " (" + kind + " '" + name + "')"
			}
		}
	}
	// Fall back to varInfo (traces register origin)
	base := 0
	if L.CI != nil {
		base = L.CI.Func + 1
	}
	reg := funcIdx - base
	if reg >= 0 {
		return varInfo(L, reg)
	}
	return ""
}

// runTypeError raises "attempt to <op> a <type> value <varinfo>".
// Mirrors: luaG_typeerror in ldebug.c
// reg is the register index (relative to CI base) holding the offending value.
// If reg < 0, no variable info is added.
func runTypeError(L *state.LuaState, val object.TValue, op string, reg int) {
	typeName := metamethod.ObjTypeName(L.Global, val)
	extra := ""
	if reg >= 0 {
		extra = varInfo(L, reg)
	}
	RunError(L, fmt.Sprintf("attempt to %s a %s value%s", op, typeName, extra))
}

// runTypeErrorByVal raises a type error, finding the variable name by examining
// the current VM instruction to determine which register holds the offending value.
// Mirrors: luaG_typeerror → varinfo in ldebug.c
func runTypeErrorByVal(L *state.LuaState, val object.TValue, op string) {
	typeName := metamethod.ObjTypeName(L.Global, val)
	extra := ""
	if L.CI != nil && L.CI.IsLua() {
		if cl, ok := L.Stack[L.CI.Func].Val.Val.(*closure.LClosure); ok && cl.Proto != nil {
			pc := L.CI.SavedPC - 1
			if pc >= 0 && pc < len(cl.Proto.Code) {
				inst := cl.Proto.Code[pc]
				iop := opcode.GetOpCode(inst)
				reg := -1
				switch iop {
				// For GET* ops, the table is in register B (or upvalue B for GETTABUP)
				case opcode.OP_GETTABLE, opcode.OP_GETI, opcode.OP_GETFIELD, opcode.OP_SELF:
					reg = opcode.GetArgB(inst)
				case opcode.OP_GETTABUP:
					// Table is an upvalue — check upvalue name
					b := opcode.GetArgB(inst)
					if b < len(cl.Proto.Upvalues) && cl.Proto.Upvalues[b].Name != nil {
						uname := cl.Proto.Upvalues[b].Name.Data
						if uname == "_ENV" {
							// For _ENV access, use the key (field C) as the global name
							k := opcode.GetArgC(inst)
							if k < len(cl.Proto.Constants) && cl.Proto.Constants[k].IsString() {
								gname := cl.Proto.Constants[k].Val.(*object.LuaString).Data
								extra = " (global '" + gname + "')"
							}
						} else {
							extra = " (upvalue '" + uname + "')"
						}
					}
				// For SET* ops, the table is in register A (or upvalue A for SETTABUP)
				case opcode.OP_SETTABLE, opcode.OP_SETI, opcode.OP_SETFIELD:
					reg = opcode.GetArgA(inst)
				case opcode.OP_SETTABUP:
					a := opcode.GetArgA(inst)
					if a < len(cl.Proto.Upvalues) && cl.Proto.Upvalues[a].Name != nil {
						uname := cl.Proto.Upvalues[a].Name.Data
						if uname == "_ENV" {
							k := opcode.GetArgB(inst)
							if k < len(cl.Proto.Constants) && cl.Proto.Constants[k].IsString() {
								gname := cl.Proto.Constants[k].Val.(*object.LuaString).Data
								extra = " (global '" + gname + "')"
							}
						} else {
							extra = " (upvalue '" + uname + "')"
						}
					}
				}
				// If we got a register, use varInfo to get the name
				if reg >= 0 && extra == "" {
					extra = varInfo(L, reg)
				}
			}
		}
	}
	RunError(L, fmt.Sprintf("attempt to %s a %s value%s", op, typeName, extra))
}
