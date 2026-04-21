// debug_impl.go — Debug interface implementation (GetInfo, GetLocal, SetLocal, hooks).
package api

import (
	"fmt"
	"strings"

	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/object"

	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/vm"
)

// ---------------------------------------------------------------------------
// Debug interface (minimal)
// ---------------------------------------------------------------------------

// GetStack fills a DebugInfo for the given call level.
func (L *State) GetStack(level int) (*DebugInfo, bool) {
	ls := L.ls()
	if level < 0 {
		return nil, false
	}
	ci := ls.CI
	for i := 0; i < level && ci != nil; i++ {
		ci = ci.Prev
	}
	if ci == nil || ci == &ls.BaseCI {
		return nil, false
	}
	ar := &DebugInfo{}
	// Store this CI and thread state for GetInfo/GetLocal to use
	ar.CI = ci
	ar.ThreadState = ls
	fval := ls.Stack[ci.Func].Val
	switch fval.Tt {
	case object.TagLuaClosure:
		cl := fval.Val.(*closure.LClosure)
		p := cl.Proto
		if p.Source != nil {
			ar.Source = p.Source.Data
			ar.ShortSrc = shortSrc(p.Source.Data)
		} else {
			ar.Source = "=?"
			ar.ShortSrc = "?"
		}
		ar.LineDefined = p.LineDefined
		ar.LastLineDefined = p.LastLine
		ar.NUps = len(cl.UpVals)
		ar.NParams = int(p.NumParams)
		ar.IsVararg = p.IsVararg()
		// Mirrors C Lua funcinfo: linedefined==0 means "main" chunk
		if p.LineDefined == 0 {
			ar.What = "main"
		} else {
			ar.What = "Lua"
		}
		// Current line
		pc := ci.SavedPC - 1
		if pc < 0 {
			pc = 0
		}
		ar.CurrentLine = vm.GetFuncLine(p, pc)
	case object.TagCClosure, object.TagLightCFunc:
		ar.Source = "=[C]"
		ar.ShortSrc = "[C]"
		ar.What = "C"
		ar.IsVararg = true // All C functions are vararg
		ar.NParams = 0
		if fval.Tt == object.TagCClosure {
			cc := fval.Val.(*closure.CClosure)
			ar.NUps = len(cc.UpVals)
		}
	}
	return ar, true
}

// GetInfo fills debug info fields specified by what string.
// Mirrors: lua_getinfo in lapi.c
func (L *State) GetInfo(what string, ar *DebugInfo) bool {
	if ar == nil {
		return false
	}
	// Use the thread state from the debug info if available (for coroutine inspection)
	ls := L.ls()
	if ts, ok := ar.ThreadState.(*state.LuaState); ok {
		ls = ts
	}
	for i := 0; i < len(what); i++ {
		switch what[i] {
		case 'n':
			if ar.CI == nil {
				break
			}
			queriedCI, ok := ar.CI.(*state.CallInfo)
			if !ok {
				break
			}
			caller := queriedCI.Prev
			if caller == nil {
				break
			}
			// Check if caller is closing TBC vars (CISTClsRet flag)
			// If so, this frame is a __close metamethod call
			if caller.CallStatus&state.CISTClsRet != 0 {
				ar.Name = "close"
				ar.NameWhat = "metamethod"
				break
			}
			// Check if caller is running a hook (CISTHooked flag)
			// If so, this frame was called from a hook dispatch
			if caller.CallStatus&state.CISTHooked != 0 {
				ar.Name = "?"
				ar.NameWhat = "hook"
				break
			}
			// Check if caller is running a finalizer (CISTFin flag)
			// If so, this frame is a __gc metamethod call.
			// Mirrors C Lua's funcnamefromcall order: ClsRet → Hooked → Fin → isLua
			if caller.CallStatus&state.CISTFin != 0 {
				ar.Name = "__gc"
				ar.NameWhat = "metamethod"
				break
			}
			fval := ls.Stack[caller.Func].Val
			if fval.Tt != object.TagLuaClosure {
				break
			}
			cl := fval.Val.(*closure.LClosure)
			p := cl.Proto
			if p == nil {
				break
			}
			pc := caller.SavedPC - 1
			if pc < 0 || pc >= len(p.Code) {
				break
			}
			// Use funcNameFromCode which handles all opcodes:
			// OP_CALL, OP_TAILCALL, OP_TFORCALL, OP_MMBIN, OP_GETTABUP, etc.
			kind, name := vm.FuncNameFromCode(ls, p, pc)
			if name != "" {
				ar.Name = name
				ar.NameWhat = kind
			}
		case 'S', 'l', 'u':
			// Already filled by GetStack
		case 'f':
			// Push the function value onto the stack.
			// Mirrors: lua_getinfo 'f' flag in lapi.c.
			if ar.CI != nil {
				if ci, ok := ar.CI.(*state.CallInfo); ok {
					fval := ls.Stack[ci.Func].Val
					L.push(fval)
				}
			}
		case 'r':
			// Transfer info for call/return hooks
			if ar.CI != nil {
				if ci, ok := ar.CI.(*state.CallInfo); ok {
					if ci.CallStatus&state.CISTHooked != 0 {
						ar.FTransfer = ls.FTransfer
						ar.NTransfer = ls.NTransfer
					}
				}
			}
		case 't':
			// Tail call and extra args info
			if ar.CI != nil {
				if ci, ok := ar.CI.(*state.CallInfo); ok {
					ar.IsTailCall = ci.CallStatus&state.CISTTail != 0
					ar.ExtraArgs = ci.NExtraArgs
				}
			}
		}
	}
	return true
}

// shortSrc creates a short source name for error messages.
// Mirrors luaO_chunkid in lobject.c. LUA_IDSIZE = 60.
func shortSrc(source string) string {
	const idsize = 60
	if len(source) == 0 {
		// Empty string source → [string ""]
		return `[string ""]`
	}
	if source[0] == '=' {
		rest := source[1:]
		if len(rest)+1 <= idsize {
			return rest
		}
		return rest[:idsize-1]
	}
	if source[0] == '@' {
		rest := source[1:]
		if len(rest)+1 <= idsize {
			return rest
		}
		return "..." + rest[len(rest)-(idsize-1-3):]
	}
	// String source: format as [string "source"]
	// PRE=[string " (9), POS="] (2), RETS=... (3), +1 for NUL
	const maxContent = idsize - 9 - 3 - 2 - 1 // = 45
	nl := strings.IndexByte(source, '\n')
	srclen := len(source)
	if srclen <= maxContent && nl < 0 {
		// Small one-line source — keep it as-is
		return fmt.Sprintf("[string \"%s\"]", source)
	}
	// Truncate: stop at first newline, then clamp to maxContent, add "..."
	if nl >= 0 {
		srclen = nl
	}
	if srclen > maxContent {
		srclen = maxContent
	}
	return fmt.Sprintf("[string \"%s...\"]", source[:srclen])
}

// ---------------------------------------------------------------------------
// Debug hooks
// ---------------------------------------------------------------------------

// SetHookFields sets the hook mask, count, and enable flag on the internal state.
func (L *State) SetHookFields(mask, count int) {
	ls := L.ls()
	ls.HookMask = mask
	ls.BaseHookCount = count
	ls.HookCount = count
	ls.AllowHook = true
}

// ClearHookFields clears all hook fields.
func (L *State) ClearHookFields() {
	ls := L.ls()
	ls.HookMask = 0
	ls.BaseHookCount = 0
	ls.HookCount = 0
	ls.AllowHook = true
	ls.Hook = nil
}

// SetHookMarker sets a non-nil marker in Hook to indicate hooks are active.
func (L *State) SetHookMarker() {
	L.ls().Hook = true
}

// HookMask returns the current hook mask.
func (L *State) HookMask() int {
	return L.ls().HookMask
}

// HookCount returns the base hook count (for count hooks).
func (L *State) HookCount() int {
	return L.ls().BaseHookCount
}

// HookActive returns true if any hooks are set.
func (L *State) HookActive() bool {
	return L.ls().HookMask != 0
}

// HasCallFrames returns true if the thread has call frames above the base CI.
// Mirrors: lua_getstack(L, 0, &ar) in C Lua — returns true when ci != &L->base_ci.
func (L *State) HasCallFrames() bool {
	ls := L.ls()
	return ls.CI != nil && ls.CI != &ls.BaseCI
}

// GetFuncProtoInfo inspects a Lua closure at stack index `idx` and returns
// its Proto metadata. For C functions returns defaults with ok=false.
func (L *State) GetFuncProtoInfo(idx int) (source, shortSource, what string, lineDefined, lastLine, nups, nparams int, isVararg, ok bool) {
	v := L.index2val(idx)
	if v == nil {
		return "=[C]", "[C]", "C", 0, 0, 0, 0, true, false
	}
	if v.Tt == object.TagLuaClosure {
		cl := v.Val.(*closure.LClosure)
		p := cl.Proto
		if p != nil {
			src := "=?"
			ssrc := "?"
			if p.Source != nil {
				src = p.Source.String()
				ssrc = shortSrc(src)
			}
			w := "Lua"
			if p.LineDefined == 0 {
				w = "main"
			}
			return src, ssrc, w, p.LineDefined, p.LastLine, len(cl.UpVals), int(p.NumParams), p.IsVararg(), true
		}
	}
	if v.Tt == object.TagCClosure {
		cc := v.Val.(*closure.CClosure)
		return "=[C]", "[C]", "C", 0, 0, len(cc.UpVals), 0, true, false
	}
	// TagLightCFunc or other C function types
	return "=[C]", "[C]", "C", 0, 0, 0, 0, true, false
}

func (L *State) GetLocal(ar *DebugInfo, n int) string {
	ls, ok := ar.ThreadState.(*state.LuaState)
	if !ok {
		ls = L.ls()
	}
	ci, ok := ar.CI.(*state.CallInfo)
	if !ok || ci == nil {
		return ""
	}
	clfn := ls.Stack[ci.Func].Val
	isLua := clfn.Tt == object.TagLuaClosure

	var proto *object.Proto
	if isLua {
		cl, ok := clfn.Val.(*closure.LClosure)
		if ok && cl != nil && cl.Proto != nil {
			proto = cl.Proto
		}
	}

	// Negative n: vararg slots (Lua functions only)
	if n < 0 {
		if proto == nil || !proto.IsVararg() {
			return ""
		}
		numExtra := ci.NExtraArgs
		if numExtra <= 0 {
			return ""
		}
		// Formula: slot = ci.Func - numExtra - n - 1 (n is negative).
		slot := ci.Func - int(numExtra) - n - 1
		// Valid: slot >= ci.Func-numExtra and slot <= ci.Func-1.
		if slot < ci.Func-int(numExtra) || slot > ci.Func-1 {
			return ""
		}
		vm.CheckStack(ls, 1)
		ls.Stack[ls.Top].Val = ls.Stack[slot].Val
		ls.Top++
		return "(vararg)"
	}

	// Try to find a named local (Lua functions only)
	name := ""
	if proto != nil {
		localNum := n
		pc := ci.SavedPC - 1
		if pc < 0 {
			pc = 0
		}
		for i := 0; i < len(proto.LocVars) && proto.LocVars[i].StartPC <= pc; i++ {
			if pc < proto.LocVars[i].EndPC {
				localNum--
				if localNum == 0 {
					if proto.LocVars[i].Name != nil {
						name = proto.LocVars[i].Name.Data
					}
					break
				}
			}
		}
	}

	// If we found a named local, push its value and return
	if name != "" && name != "?" {
		slot := ci.Func + n
		if slot < 0 || slot >= len(ls.Stack) {
			return ""
		}
		vm.CheckStack(ls, 1)
		ls.Stack[ls.Top] = ls.Stack[slot]
		ls.Top++
		return name
	}

	// Fallback: check if n is within CI stack range (unnamed slots).
	// Mirrors C Lua's luaG_findlocal fallback for temporaries.
	base := ci.Func + 1
	var limit int
	if ci == ls.CI {
		limit = ls.Top
	} else if ci.Next != nil {
		limit = ci.Next.Func
	} else {
		limit = ls.Top
	}
	if n > 0 && limit-base >= n {
		slot := base + n - 1
		if slot >= 0 && slot < len(ls.Stack) {
			vm.CheckStack(ls, 1)
			ls.Stack[ls.Top] = ls.Stack[slot]
			ls.Top++
			if isLua {
				return "(temporary)"
			}
			return "(C temporary)"
		}
	}
	return ""
}

func (L *State) SetLocal(ar *DebugInfo, n int) string {
	ls, ok := ar.ThreadState.(*state.LuaState)
	if !ok {
		ls = L.ls()
	}
	ci, ok := ar.CI.(*state.CallInfo)
	if !ok || ci == nil {
		return ""
	}
	clfn := ls.Stack[ci.Func].Val
	isLua := clfn.Tt == object.TagLuaClosure

	var proto *object.Proto
	if isLua {
		cl, ok := clfn.Val.(*closure.LClosure)
		if ok && cl != nil && cl.Proto != nil {
			proto = cl.Proto
		}
	}

	// Negative n: vararg slots (Lua functions only)
	if n < 0 {
		if proto == nil || !proto.IsVararg() {
			return ""
		}
		numExtra := ci.NExtraArgs
		if numExtra <= 0 {
			return ""
		}
		// Same formula as GetLocal: slot = ci.Func - numExtra - n - 1.
		slot := ci.Func - int(numExtra) - n - 1
		if slot < ci.Func-int(numExtra) || slot > ci.Func-1 {
			return ""
		}
		ls.Stack[slot] = ls.Stack[ls.Top-1]
		ls.Top--
		return "(vararg)"
	}

	// Positive n: try named locals first (Lua functions only)
	name := ""
	if proto != nil {
		localNum := n
		pc := ci.SavedPC - 1
		if pc < 0 {
			pc = 0
		}
		for i := 0; i < len(proto.LocVars) && proto.LocVars[i].StartPC <= pc; i++ {
			if pc < proto.LocVars[i].EndPC {
				localNum--
				if localNum == 0 {
					if proto.LocVars[i].Name != nil {
						name = proto.LocVars[i].Name.Data
					}
					break
				}
			}
		}
	}

	// If we found a named local, set its value and return
	if name != "" && name != "?" {
		slot := ci.Func + n
		if slot < 0 || slot >= len(ls.Stack) {
			return ""
		}
		ls.Stack[slot] = ls.Stack[ls.Top-1]
		ls.Top--
		return name
	}

	// Fallback: check if n is within CI stack range (unnamed slots / temporaries).
	// Mirrors C Lua's luaG_findlocal fallback.
	base := ci.Func + 1
	var limit int
	if ci == ls.CI {
		limit = ls.Top
	} else if ci.Next != nil {
		limit = ci.Next.Func
	} else {
		limit = ls.Top
	}
	if n > 0 && limit-base >= n {
		slot := base + n - 1
		if slot >= 0 && slot < len(ls.Stack) {
			ls.Stack[slot] = ls.Stack[ls.Top-1]
			ls.Top--
			if isLua {
				return "(temporary)"
			}
			return "(C temporary)"
		}
	}
	return ""
}
