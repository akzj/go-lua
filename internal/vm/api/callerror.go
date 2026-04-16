// callerror.go — Function name resolution for error messages.
// Mirrors: funcnamefromcode, funcnamefromcall, luaG_callerror from ldebug.c
package api

import (
	closureapi "github.com/akzj/go-lua/internal/closure/api"
	mmapi "github.com/akzj/go-lua/internal/metamethod/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	opcodeapi "github.com/akzj/go-lua/internal/opcode/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
)

// funcNameFromCode examines the instruction at pc to determine the function name.
// Returns (kind, name) or ("", "") if unknown.
// Mirrors: funcnamefromcode in ldebug.c
func funcNameFromCode(L *stateapi.LuaState, p *objectapi.Proto, pc int) (string, string) {
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
			kind, name := funcNameFromCode(L, cl.Proto, pc)
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
