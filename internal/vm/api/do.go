// Call, return, error handling, and stack management for the Lua VM.
//
// This is the Go equivalent of C's ldo.c. It handles:
// - Error handling via panic/recover (Go's equivalent of setjmp/longjmp)
// - Stack reallocation and growth
// - Function call preparation and post-call cleanup
// - Protected calls (pcall)
// - Parser integration
//
// Reference: lua-master/ldo.c, .analysis/04-call-return-error.md
package api

import (
	closureapi "github.com/akzj/go-lua/internal/closure/api"
	lexapi "github.com/akzj/go-lua/internal/lex/api"
	luastringapi "github.com/akzj/go-lua/internal/luastring/api"
	mmapi "github.com/akzj/go-lua/internal/metamethod/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	parseapi "github.com/akzj/go-lua/internal/parse/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
	tableapi "github.com/akzj/go-lua/internal/table/api"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	luaMinStack   = 20 // LUA_MINSTACK
	stackErrSpace = 200
	errorStackSize = stateapi.MaxStack + stackErrSpace
)

// MaxCCMT is the maximum number of __call metamethod chain depth.
const MaxCCMT = 0xF << stateapi.CISTCCMTShift

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

// Throw raises a Lua error by panicking with a stateapi.LuaError.
// This is the Go equivalent of C's luaD_throw / LUAI_THROW.
// The panic will be caught by the nearest PCall/RunProtected.
func Throw(L *stateapi.LuaState, status int) {
	panic(stateapi.LuaError{Status: status})
}

// SetErrorObj sets the error object on the stack at oldtop.
// Mirrors: luaD_seterrorobj in ldo.c
func SetErrorObj(L *stateapi.LuaState, errcode int, oldtop int) {
	// Bounds check: if oldtop is beyond the stack, we can't set the error.
	// This can happen during cascading errors in __close after stack overflow.
	if oldtop >= len(L.Stack) {
		Throw(L, stateapi.StatusErrErr)
		return
	}
	if errcode == stateapi.StatusErrMem {
		// Memory error — use pre-registered message
		L.Stack[oldtop].Val = objectapi.MakeString(L.Global.MemErrMsg)
	} else {
		// Move error object from top-1 to oldtop
		if L.Top > 0 && L.Top-1 < len(L.Stack) {
			L.Stack[oldtop].Val = L.Stack[L.Top-1].Val
		}
	}
	L.Top = oldtop + 1
}

// ErrorErr raises an error during error handling (error in error handler).
// Mirrors: luaD_errerr in ldo.c
func ErrorErr(L *stateapi.LuaState) {
	// Bounds check: if L.Top is at or beyond the stack limit,
	// we can't push the error message. Just throw directly.
	if L.Top >= len(L.Stack) {
		Throw(L, stateapi.StatusErrErr)
		return
	}
	L.Stack[L.Top].Val = makeInternedString(L, "error in error handling")
	L.Top++
	Throw(L, stateapi.StatusErrErr)
}

// ErrorMsg calls the error handler (if set) then throws a runtime error.
// Mirrors: luaG_errormsg in ldebug.c
// The error object must already be on the stack at L.Top-1.
func ErrorMsg(L *stateapi.LuaState) {
	// If error object is nil, replace with "<no error object>" string.
	// Mirrors: ldebug.c:849-852
	if L.Top > 0 && L.Top-1 < len(L.Stack) {
		if L.Stack[L.Top-1].Val.Tt.IsStrictNil() {
			L.Stack[L.Top-1].Val = makeInternedString(L, "<no error object>")
		}
	}
	if L.ErrFunc != 0 {
		errFunc := L.Stack[L.ErrFunc].Val
		// Bounds check: need L.Top to be a valid index for writing
		if L.Top >= len(L.Stack) {
			// No room to rearrange stack for handler call — error in error handling
			ErrorErr(L)
			return
		}
		// Stack: [..., errmsg] (at Top-1)
		// Rearrange to: [..., handler, errmsg]
		L.Stack[L.Top].Val = L.Stack[L.Top-1].Val // copy errmsg up
		L.Stack[L.Top-1].Val = errFunc             // put handler below
		L.Top++
		// Do NOT reset NCCalls here. C Lua's luaG_errormsg does not
		// reset nCcalls — the error handler inherits the current
		// (high) nCcalls value, so if it tries to recurse deeply,
		// it will hit the C stack overflow limit quickly.
		// RunProtected will save/restore NCCalls on error.
		status := RunProtected(L, func() {
			CallNoYield(L, L.Top-2, 1)
		})
		if status != stateapi.StatusOK {
			// Error in error handler
			ErrorErr(L)
		}
		// handler's return value is now at Top-1, replacing original error
	}
	Throw(L, stateapi.StatusErrRun)
}

// RunError raises a runtime error with a string message.
// Mirrors: luaG_runerror in ldebug.c — adds source:line: prefix for Lua frames.
func RunError(L *stateapi.LuaState, msg string) {
	msg = addInfo(L, msg)
	stateapi.PushValue(L, makeInternedString(L, msg))
	ErrorMsg(L)
}

// ---------------------------------------------------------------------------
// Stack management
// ---------------------------------------------------------------------------

// ReallocStack reallocates the stack to newsize.
// Returns true on success. If raiseerror is true, panics on failure.
// Mirrors: luaD_reallocstack in ldo.c
func ReallocStack(L *stateapi.LuaState, newsize int) {
	oldsize := len(L.Stack)
	newStack := make([]objectapi.StackValue, newsize)
	copy(newStack, L.Stack)
	// Initialize new slots to nil
	for i := oldsize; i < newsize; i++ {
		newStack[i].Val = objectapi.Nil
	}
	L.Stack = newStack
}

// GrowStack ensures at least n free stack slots above L.Top.
// If raiseerror is true, raises an error on stack overflow.
// Mirrors: luaD_growstack in ldo.c
func GrowStack(L *stateapi.LuaState, n int, raiseerror bool) bool {
	size := len(L.Stack)
	if size > stateapi.MaxStack {
		// Already beyond normal stack limit (in error stack space).
		// If there's room in current allocation, allow it — this lets
		// the error handler run within the error stack headroom.
		if L.Top+n+stateapi.ExtraStack <= size {
			return true
		}
		// Try to grow to errorStackSize if not already there.
		errSize := errorStackSize + stateapi.ExtraStack
		if size < errSize {
			ReallocStack(L, errSize)
		}
		// Raise "stack overflow" — do NOT return true here.
		// C Lua (ldo.c:384-386): realloc to ERRORSTACKSIZE, then raise error.
		// Returning true would let recursion continue consuming error space,
		// leaving nothing for the error handler.
		if raiseerror {
			RunError(L, "stack overflow")
		}
		return false
	}
	if n < stateapi.MaxStack {
		newsize := size + (size >> 1) // 1.5x
		needed := L.Top + n + stateapi.ExtraStack
		if newsize > stateapi.MaxStack {
			newsize = stateapi.MaxStack
		}
		if newsize < needed {
			newsize = needed
		}
		if newsize <= stateapi.MaxStack {
			ReallocStack(L, newsize+stateapi.ExtraStack)
			return true
		}
	}
	// Stack overflow — grow to error stack size for error handling room
	ReallocStack(L, errorStackSize+stateapi.ExtraStack)
	if raiseerror {
		RunError(L, "stack overflow")
	}
	return false
}

// CheckStack ensures at least n free stack slots, growing if needed.
func CheckStack(L *stateapi.LuaState, n int) {
	if L.Top+n > L.StackLast() {
		GrowStack(L, n, true)
	}
}

// IncTop increments L.Top with a stack check.
// Mirrors: luaD_inctop in ldo.c
func IncTop(L *stateapi.LuaState) {
	L.Top++
	CheckStack(L, 0)
}

// StackInUse computes how much of the stack is in use.
// Mirrors: stackinuse in ldo.c
func StackInUse(L *stateapi.LuaState) int {
	lim := L.Top
	for ci := L.CI; ci != nil; ci = ci.Prev {
		if lim < ci.Top {
			lim = ci.Top
		}
	}
	res := lim + 1
	if res < luaMinStack {
		res = luaMinStack
	}
	return res
}

// ShrinkStack reduces the stack size if it's much larger than needed.
// Mirrors: luaD_shrinkstack in ldo.c
func ShrinkStack(L *stateapi.LuaState) {
	inuse := StackInUse(L)
	maxUse := inuse * 3
	if maxUse > stateapi.MaxStack {
		maxUse = stateapi.MaxStack
	}
	if inuse <= stateapi.MaxStack && len(L.Stack) > maxUse+stateapi.ExtraStack {
		nsize := inuse * 2
		if nsize > stateapi.MaxStack {
			nsize = stateapi.MaxStack
		}
		ReallocStack(L, nsize+stateapi.ExtraStack)
	}
	stateapi.ShrinkCI(L)
}

// ---------------------------------------------------------------------------
// Call mechanics
// ---------------------------------------------------------------------------

// getFunc returns the TValue at stack index funcIdx.
func getFunc(L *stateapi.LuaState, funcIdx int) objectapi.TValue {
	return L.Stack[funcIdx].Val
}

// nextCI returns the next CallInfo, allocating if needed.
func nextCI(L *stateapi.LuaState) *stateapi.CallInfo {
	if L.CI.Next != nil {
		return L.CI.Next
	}
	return stateapi.NewCI(L)
}

// prepCallInfo allocates and initializes a new CallInfo.
func prepCallInfo(L *stateapi.LuaState, funcIdx int, status uint32, top int) *stateapi.CallInfo {
	ci := nextCI(L)
	L.CI = ci
	ci.Func = funcIdx
	ci.CallStatus = status
	ci.Top = top
	return ci
}

// precallC handles the call to a C function (Go function).
// Executes the function immediately and calls PosCall.
func precallC(L *stateapi.LuaState, funcIdx int, status uint32, f stateapi.CFunction) int {
	// Ensure minimum stack size
	CheckStack(L, luaMinStack)
	ci := prepCallInfo(L, funcIdx, status|stateapi.CISTC, L.Top+luaMinStack)
	// Fire call hook if active
	if L.HookMask&stateapi.MaskCall != 0 {
		CallHook(L, ci)
	}
	// Execute the C function
	n := f(L)
	PosCall(L, ci, n)
	return n
}

// TryFuncTM tries the __call metamethod for a non-function value.
// Shifts the stack to make room for the metamethod and returns updated status.
// Mirrors: tryfuncTM in ldo.c
func TryFuncTM(L *stateapi.LuaState, funcIdx int, status uint32) uint32 {
	tm := mmapi.GetTMByObj(L.Global, L.Stack[funcIdx].Val, mmapi.TM_CALL)
	if tm.IsNil() {
		// Build error message with context.
		// Mirrors: luaG_callerror in ldebug.c
		typeName := objectapi.TypeNames[L.Stack[funcIdx].Val.Type()]
		extra := callErrorExtra(L, funcIdx)
		RunError(L, "attempt to call a "+typeName+" value"+extra)
	}
	// Shift stack up to make room for metamethod
	for p := L.Top; p > funcIdx; p-- {
		L.Stack[p].Val = L.Stack[p-1].Val
	}
	L.Top++
	L.Stack[funcIdx].Val = tm // metamethod is the new function
	if status&MaxCCMT == MaxCCMT {
		RunError(L, "'__call' chain too long")
	}
	return status + (1 << stateapi.CISTCCMTShift)
}

// PreCall prepares a function call. For C functions, executes immediately
// and returns nil. For Lua functions, creates a CallInfo and returns it.
// Mirrors: luaD_precall in ldo.c
func PreCall(L *stateapi.LuaState, funcIdx int, nResults int) *stateapi.CallInfo {
	status := uint32(nResults + 1)
retry:
	fval := L.Stack[funcIdx].Val
	switch fval.Tt {
	case objectapi.TagLuaClosure:
		cl := fval.Val.(*closureapi.LClosure)
		p := cl.Proto
		narg := L.Top - funcIdx - 1 // number of actual arguments
		nfixparams := int(p.NumParams)
		fsize := int(p.MaxStackSize)
		CheckStack(L, fsize)
		ci := prepCallInfo(L, funcIdx, status, funcIdx+1+fsize)
		ci.SavedPC = 0 // starting point
		// Complete missing arguments with nil
		for ; narg < nfixparams; narg++ {
			L.Stack[L.Top].Val = objectapi.Nil
			L.Top++
		}
		// Fire call hook if active.
		// For vararg functions (PF_VAHID), defer the call hook to OP_VARARGPREP
		// (after AdjustVarargs shifts the stack). This matches C Lua where
		// luaG_tracecall returns 0 for vararg functions, and the hook fires
		// inside OP_VARARGPREP instead.
		if L.HookMask != 0 && !p.IsVararg() {
			CallHook(L, ci)
		}
		return ci

	case objectapi.TagCClosure:
		cc := fval.Val.(*closureapi.CClosure)
		precallC(L, funcIdx, status, cc.Fn)
		return nil

	case objectapi.TagLightCFunc:
		f := fval.Val.(stateapi.CFunction)
		precallC(L, funcIdx, status, f)
		return nil

	default:
		// Not a function — try __call metamethod
		CheckStack(L, 1)
		status = TryFuncTM(L, funcIdx, status)
		goto retry
	}
}

// PosCall performs post-call cleanup: moves results, adjusts top, unwinds CI.
// Mirrors: luaD_poscall in ldo.c
func PosCall(L *stateapi.LuaState, ci *stateapi.CallInfo, nres int) {
	wanted := ci.NResults()
	res := ci.Func // destination for results

	// Fire return hook and restore OldPC for caller.
	// Mirrors: rethook in ldo.c — called when ANY hook is active, not just MaskRet.
	// The OldPC restoration is unconditional (needed by line hook even when
	// return hook is off). This is critical: without it, the line hook fires
	// spurious events when returning from calls (e.g. hook function returns).
	if L.HookMask != 0 {
		if L.AllowHook && L.HookMask&stateapi.MaskRet != 0 {
			retHook(L, ci, nres)
		}
		// Restore OldPC for the caller's frame (unconditional).
		// Mirrors: rethook in ldo.c: L->oldpc = pcRel(ci->u.l.savedpc, ...)
		// Must run even when AllowHook is false (during hook dispatch),
		// otherwise changedline() sees stale OldPC and misses a line event.
		if prev := ci.Prev; prev != nil && prev.IsLua() {
			L.OldPC = prev.SavedPC - 1
		}
	}

	// Move results to proper place
	moveResults(L, res, nres, wanted)

	// Back to caller
	L.CI = ci.Prev
}

// hookDispatch is the common hook dispatcher.
// Calls the hook function with (event [, line]) arguments.
// Mirrors: luaD_hook in ldo.c
// event: "call", "return", "line", "count", "tail call"
// line: line number for line hooks, -1 otherwise
// ftransfer/ntransfer: parameter/return value transfer info (0 if N/A)
func hookDispatch(L *stateapi.LuaState, event string, line int, ftransfer int, ntransfer int) {
	hookVal, ok := L.Hook.(objectapi.TValue)
	if !ok || hookVal.Tt == objectapi.TagNil || hookVal.Val == nil {
		return
	}
	if !L.AllowHook {
		return
	}

	ci := L.CI
	savedTop := L.Top
	savedCITop := ci.Top
	savedAllowHook := L.AllowHook
	L.AllowHook = false // cannot call hooks inside a hook
	ci.CallStatus |= stateapi.CISTHooked // mark caller as hook frame

	// Set transfer info for debug.getinfo "r" flag
	L.FTransfer = ftransfer
	L.NTransfer = ntransfer

	// Protect entire activation register (mirrors luaD_hook in ldo.c)
	// For Lua functions, L.Top may be below ci.Top. Push hook args above ci.Top
	// to avoid overwriting registers.
	if ci.IsLua() && L.Top < ci.Top {
		L.Top = ci.Top
	}

	defer func() {
		L.AllowHook = savedAllowHook
		ci.CallStatus &^= stateapi.CISTHooked // clear hook flag
		ci.Top = savedCITop
		L.Top = savedTop
	}()

	// Ensure stack space: hook_func + event_name + optional_line_arg
	CheckStack(L, 4)

	st := L.Global.StringTable.(*luastringapi.StringTable)

	// Push hook function
	L.Stack[L.Top].Val = hookVal
	L.Top++

	// Push event name
	L.Stack[L.Top].Val = objectapi.MakeString(st.Intern(event))
	L.Top++

	// For line hooks, push line number as second arg
	nargs := 1
	if line >= 0 {
		L.Stack[L.Top].Val = objectapi.MakeInteger(int64(line))
		L.Top++
		nargs = 2
	}

	Call(L, L.Top-nargs-1, 0)
}

// retHook fires the return hook if set.
// Mirrors: rethook in ldo.c
func retHook(L *stateapi.LuaState, ci *stateapi.CallInfo, nres int) {
	// For vararg functions, ci.Func has already been moved back by OP_RETURN.
	// Temporarily restore it to the "virtual func" position (after OP_VARARGPREP)
	// so that debug.getlocal sees the correct locals during the return hook.
	// Mirrors: rethook in ldo.c
	delta := 0
	if ci.IsLua() {
		cl, ok := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
		if ok && cl.Proto != nil && cl.Proto.IsVararg() {
			delta = ci.NExtraArgs + int(cl.Proto.NumParams) + 1
		}
	}
	if delta != 0 {
		ci.Func += delta // back to virtual 'func'
	}
	ftransfer := (L.Top - nres) - ci.Func
	hookDispatch(L, "return", -1, ftransfer, nres)
	if delta != 0 {
		ci.Func -= delta // restore
	}
}

// CallHook fires the call hook for a new function call.
// Mirrors: luaD_hookcall in ldo.c
func CallHook(L *stateapi.LuaState, ci *stateapi.CallInfo) {
	L.OldPC = 0 // set 'oldpc' for new function
	if L.HookMask&stateapi.MaskCall == 0 {
		return
	}
	event := "call"
	if ci.CallStatus&stateapi.CISTTail != 0 {
		event = "tail call"
	}
	if ci.IsLua() {
		// ftransfer=1 (first param), ntransfer=numparams
		numparams := 0
		cl, ok := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
		if ok && cl.Proto != nil {
			numparams = int(cl.Proto.NumParams)
		}
		ci.SavedPC++ // hooks assume 'pc' is already incremented
		hookDispatch(L, event, -1, 1, numparams)
		ci.SavedPC-- // correct 'pc'
	} else {
		// For C functions: ftransfer=1, ntransfer=narg (top - func - 1)
		narg := L.Top - ci.Func - 1
		if narg < 0 {
			narg = 0
		}
		hookDispatch(L, event, -1, 1, narg)
	}
}

// TraceExec handles line and count hooks during VM execution.
// Returns true if trap should stay active.
// Mirrors: luaG_traceexec in ldebug.c
func TraceExec(L *stateapi.LuaState, ci *stateapi.CallInfo) bool {
	mask := L.HookMask
	if mask&(stateapi.MaskLine|stateapi.MaskCount) == 0 {
		return false // no line/count hooks, turn off trap
	}

	cl, ok := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
	if !ok || cl.Proto == nil {
		return false
	}
	p := cl.Proto

	// Count hook
	countHook := false
	if mask&stateapi.MaskCount != 0 {
		L.HookCount--
		if L.HookCount == 0 {
			L.HookCount = L.BaseHookCount // reset
			countHook = true
		}
	}
	if !countHook && mask&stateapi.MaskLine == 0 {
		return true // no line hook and count != 0
	}

	if countHook {
		hookDispatch(L, "count", -1, 0, 0)
	}

	if mask&stateapi.MaskLine != 0 {
		npci := ci.SavedPC - 1 // PC of instruction about to execute (SavedPC already incremented)
		if npci < 0 {
			npci = 0
		}
		if npci >= len(p.Code) {
			npci = len(p.Code) - 1
		}
		oldpc := L.OldPC
		if oldpc < 0 || oldpc >= len(p.Code) {
			// OldPC is out of bounds for this proto — likely stale from a
			// different function. Clamp to 0 (matches C Lua behavior).
			oldpc = 0
		}
		// Fire line hook when:
		// 1. npci <= oldpc: backward jump (loop) or same instruction
		// 2. line changed between oldpc and npci
		
		// Mirrors: luaG_traceexec in ldebug.c
		if npci <= oldpc || GetFuncLine(p, oldpc) != GetFuncLine(p, npci) {
			newline := GetFuncLine(p, npci)
			hookDispatch(L, "line", newline, 0, 0)
		}
		L.OldPC = npci
	}

	return true // keep trap active
}

// moveResults moves nres results to res, adjusting for wanted count.
// After moving results, clears stale stack slots between the new Top and old Top
// so Go's GC can collect objects that are no longer reachable from Lua.
func moveResults(L *stateapi.LuaState, res int, nres int, wanted int) {
	oldTop := L.Top
	switch wanted {
	case 0: // no values needed
		L.Top = res
		clearStackSlots(L, res, oldTop)
		return
	case 1: // one value needed
		if nres == 0 {
			L.Stack[res].Val = objectapi.Nil
		} else {
			L.Stack[res].Val = L.Stack[L.Top-nres].Val
		}
		L.Top = res + 1
		clearStackSlots(L, res+1, oldTop)
		return
	case stateapi.MultiRet: // all results
		genMoveResults(L, res, nres, nres)
		// genMoveResults sets Top = res+nres, clear above that
		clearStackSlots(L, L.Top, oldTop)
		return
	default: // specific number of results
		genMoveResults(L, res, nres, wanted)
		clearStackSlots(L, L.Top, oldTop)
		return
	}
}

// clearStackSlots nils out stack slots in [from, to) so Go's GC can
// collect objects that are no longer reachable from Lua. This is critical
// because Lua "pops" values by decrementing Top without clearing slots,
// which leaves Go pointers alive and prevents runtime.SetFinalizer from firing.
func clearStackSlots(L *stateapi.LuaState, from, to int) {
	for i := from; i < to; i++ {
		L.Stack[i].Val = objectapi.Nil
	}
}

// genMoveResults is the generic result mover.
func genMoveResults(L *stateapi.LuaState, res int, nres int, wanted int) {
	firstResult := L.Top - nres
	if nres > wanted {
		nres = wanted
	}
	for i := 0; i < nres; i++ {
		L.Stack[res+i].Val = L.Stack[firstResult+i].Val
	}
	for i := nres; i < wanted; i++ {
		L.Stack[res+i].Val = objectapi.Nil
	}
	L.Top = res + wanted
}

// PreTailCall prepares a tail call. Returns number of C function results,
// or -1 for Lua function (caller should continue the loop).
// delta is the vararg adjustment: ci.NExtraArgs + nparams1 (or 0 for non-vararg).
// Mirrors: luaD_pretailcall in ldo.c
func PreTailCall(L *stateapi.LuaState, ci *stateapi.CallInfo, funcIdx int, narg1 int, delta int) int {
	status := uint32(stateapi.MultiRet + 1)
retry:
	fval := L.Stack[funcIdx].Val
	switch fval.Tt {
	case objectapi.TagCClosure:
		cc := fval.Val.(*closureapi.CClosure)
		return precallC(L, funcIdx, status, cc.Fn)

	case objectapi.TagLightCFunc:
		f := fval.Val.(stateapi.CFunction)
		return precallC(L, funcIdx, status, f)

	case objectapi.TagLuaClosure:
		cl := fval.Val.(*closureapi.LClosure)
		p := cl.Proto
		fsize := int(p.MaxStackSize)
		nfixparams := int(p.NumParams)
		CheckStack(L, fsize-delta)
		ci.Func -= delta // restore 'func' (undo vararg shift)
		// Move function and arguments down to ci.Func
		for i := 0; i < narg1; i++ {
			L.Stack[ci.Func+i].Val = L.Stack[funcIdx+i].Val
		}
		funcIdx = ci.Func // now points to moved function
		// Complete missing arguments
		for ; narg1 <= nfixparams; narg1++ {
			L.Stack[funcIdx+narg1].Val = objectapi.Nil
		}
		ci.Top = funcIdx + 1 + fsize
		ci.SavedPC = 0
		ci.CallStatus |= stateapi.CISTTail
		L.Top = funcIdx + narg1
		return -1 // Lua function

	default:
		CheckStack(L, 1)
		status = TryFuncTM(L, funcIdx, status)
		narg1++
		goto retry
	}
}

// Call performs a function call. For Lua functions, calls Execute.
// Mirrors: luaD_call / ccall in ldo.c
func Call(L *stateapi.LuaState, funcIdx int, nResults int) {
	// Increment C call depth and check for overflow.
	// Mirrors C Lua's luaE_incCstack + luaE_checkcstack:
	//   == MaxCCalls: raise "C stack overflow" (error handler gets 10% buffer)
	//   >= MaxCCalls * 11/10: ErrorErr (no handler, exhausted buffer)
	L.NCCalls++
	if L.CCalls() >= stateapi.MaxCCalls {
		if L.CCalls() == stateapi.MaxCCalls {
			RunError(L, "C stack overflow")
		} else if L.CCalls() >= stateapi.MaxCCalls*11/10 {
			ErrorErr(L)
		}
		// Between MaxCCalls and MaxCCalls*1.1: allow (error handler buffer)
	}
	ci := PreCall(L, funcIdx, nResults)
	if ci != nil {
		// Lua function — execute it
		ci.CallStatus |= stateapi.CISTFresh
		Execute(L, ci)
	}
	L.NCCalls--
}

// CallNoYield performs a non-yieldable function call.
// Mirrors: luaD_callnoyield in ldo.c
func CallNoYield(L *stateapi.LuaState, funcIdx int, nResults int) {
	L.NCCalls += 0x00010001 // increment both C calls and non-yieldable count
	if L.CCalls() >= stateapi.MaxCCalls {
		if L.CCalls() == stateapi.MaxCCalls {
			RunError(L, "C stack overflow")
		} else if L.CCalls() >= stateapi.MaxCCalls*11/10 {
			ErrorErr(L)
		}
	}
	ci := PreCall(L, funcIdx, nResults)
	if ci != nil {
		ci.CallStatus |= stateapi.CISTFresh
		Execute(L, ci)
	}
	L.NCCalls -= 0x00010001
}

// ---------------------------------------------------------------------------
// Protected calls
// ---------------------------------------------------------------------------

// RunProtected runs a function in protected mode using Go's panic/recover.
// Returns the status code (StatusOK on success, error status on failure).
// Mirrors: luaD_rawrunprotected in ldo.c
func RunProtected(L *stateapi.LuaState, f func()) (status int) {
	oldNCCalls := L.NCCalls
	status = stateapi.StatusOK
	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case stateapi.LuaBaseLevel:
				// Self-close: propagate past all inner error handlers
				// to the outermost RunProtected (Resume).
				// Mirrors: luaD_throwbaselevel in ldo.c
				panic(e)
			case stateapi.LuaError:
				status = e.Status
				L.NCCalls = oldNCCalls
			case stateapi.LuaYield:
				status = stateapi.StatusYield
				L.NCCalls = oldNCCalls
			case *lexapi.SyntaxError:
				// Convert syntax error to LUA_ERRSYNTAX
				// Push error message string on stack
				errStr := e.Error()
				stateapi.PushValue(L, makeInternedString(L, errStr))
				status = stateapi.StatusErrSyntax
				L.NCCalls = oldNCCalls
			default:
				panic(r) // re-panic non-Lua errors
			}
		}
	}()
	f()
	return status
}

// PCall performs a protected function call.
// On error, restores the call stack and sets the error object.
// Mirrors: luaD_pcall in ldo.c
// finishPCallK finishes a pcall that was interrupted by a yield.
// Mirrors: finishpcallk in ldo.c
// Returns the status to pass to the continuation function.
func finishPCallK(L *stateapi.LuaState, ci *stateapi.CallInfo) int {
	status := ci.GetRecst() // retrieve saved error status from bits 12-14
	if status == stateapi.StatusOK {
		// No error — was interrupted by a yield
		status = stateapi.StatusYield
	} else {
		// Error path: close TBC vars YIELDABLY.
		// Restore allowhook from the saved OAH bit.
		// Mirrors: finishpcallk in ldo.c:812-830
		if ci.CallStatus&stateapi.CISTOAH != 0 {
			L.AllowHook = true
		} else {
			L.AllowHook = false
		}
		// Use FuncIdx (saved func position), not ci.Func.
		// Mirrors: restorestack(L, ci->u2.funcidx) in C Lua
		funcIdx := ci.FuncIdx
		errObj := objectapi.Nil
		if L.Top > funcIdx {
			errObj = L.Stack[L.Top-1].Val
		}
		// Save error object at funcIdx so it survives across yields
		L.Stack[funcIdx].Val = errObj
		L.Top = funcIdx + 1
		CloseTBCWithError(L, funcIdx, status, errObj, true) // yieldable!
		// Recover error object from funcIdx (in case close changed L.Top)
		errObj = L.Stack[funcIdx].Val
		// Put error back at top for SetErrorObj
		L.Stack[L.Top-1].Val = errObj
		SetErrorObj(L, status, funcIdx)
		ShrinkStack(L)
		ci.SetRecst(stateapi.StatusOK) // clear for next iteration
	}
	ci.CallStatus &^= stateapi.CISTYPCall
	L.ErrFunc = ci.OldErrFunc
	return status
}

func PCall(L *stateapi.LuaState, funcIdx int, nResults int, errFunc int) int {
	oldCI := L.CI
	oldAllowHook := L.AllowHook
	oldErrFunc := L.ErrFunc
	// C Lua: old_top = savestack(L, c.func) — saves function position, not L->top
	oldTop := funcIdx

	L.ErrFunc = errFunc

	// PATH B: Yieldable with continuation — use plain Call (unprotected).
	// Errors propagate to Resume's RunProtected → precover → finishPCallK.
	// Mirrors: lua_pcallk PATH B in lapi.c:1099-1108
	if L.Yieldable() && oldCI.K != nil {
		oldCI.CallStatus |= stateapi.CISTYPCall
		oldCI.FuncIdx = funcIdx     // save func position for error recovery
		oldCI.OldErrFunc = oldErrFunc
		Call(L, funcIdx, nResults)   // PLAIN call — errors propagate!
		// If we reach here, no error occurred
		oldCI.CallStatus &^= stateapi.CISTYPCall
		L.ErrFunc = oldErrFunc
		return stateapi.StatusOK
	}

	// PATH A: Non-yieldable — use RunProtected (catches errors locally).
	// Mirrors: lua_pcallk PATH A → luaD_pcall in ldo.c
	status := RunProtected(L, func() {
		Call(L, funcIdx, nResults)
	})

	if status == stateapi.StatusYield {
		// Yield inside non-continuation pcall — propagate
		oldCI.CallStatus |= stateapi.CISTYPCall
		oldCI.OldErrFunc = oldErrFunc
		L.ErrFunc = oldErrFunc
		panic(stateapi.LuaYield{})
	}

	if status != stateapi.StatusOK {
		// Restore state (mirrors C Lua luaD_pcall order)
		L.CI = oldCI
		L.AllowHook = oldAllowHook
		// Close open upvalues at or above oldTop.
		// C Lua: luaF_close first calls luaF_closeupval before handling TBC.
		// Without this, upvalues captured by closures created inside the
		// pcall'd function would still point at abandoned stack slots,
		// causing them to read nil after the stack is reused.
		closureapi.CloseUpvals(L, oldTop)
		// Close TBC vars created inside the pcall'd function.
		// C Lua: status = luaD_closeprotected(L, old_top, status)
		if L.TBCList >= oldTop {
			errObj := objectapi.Nil
			if L.Top > oldTop {
				errObj = L.Stack[L.Top-1].Val
			}
			status, errObj = CloseProtected(L, oldTop, status, errObj)
			if L.Top > oldTop {
				L.Stack[L.Top-1].Val = errObj
			}
		}
		SetErrorObj(L, status, oldTop)
		ShrinkStack(L)
	}
	L.ErrFunc = oldErrFunc
	return status
}

// ---------------------------------------------------------------------------
// Parser integration
// ---------------------------------------------------------------------------

// ProtectedParser calls the parser in protected mode.
// Pushes the resulting closure on the stack.
// Mirrors: luaD_protectedparser in ldo.c
func ProtectedParser(L *stateapi.LuaState, reader lexapi.LexReader, source string) int {
	// Increment non-yieldable count during parsing
	L.NCCalls += 0x00010000

	oldTop := L.Top
	status := RunProtected(L, func() {
		FParser(L, reader, source)
	})

	if status != stateapi.StatusOK {
		// Parsing failed
		SetErrorObj(L, status, oldTop)
	}

	L.NCCalls -= 0x00010000
	return status
}

// FParser calls the parser and pushes the resulting closure on the stack.
// Mirrors: f_parser in ldo.c
func FParser(L *stateapi.LuaState, reader lexapi.LexReader, source string) {
	// Parse source into a Proto
	proto := parseapi.Parse(source, reader)

	// Intern all strings in the proto tree so they have proper hashes.
	// The parser creates LuaString with Hash_=0 (state-independent parsing).
	// Table lookups require proper hashes for correct bucket placement.
	internProtoStrings(L, proto)

	// Create an LClosure wrapping the proto
	cl := closureapi.NewLClosure(proto, len(proto.Upvalues))

	// Push the closure on the stack
	stateapi.PushValue(L, objectapi.TValue{
		Tt:  objectapi.TagLuaClosure,
		Val: cl,
	})

	// Initialize upvalues
	closureapi.InitUpvals(cl)

	// Wire _ENV (upvalue[0]) to the global table.
	// In C Lua, f_parser does:
	//   if (cl->nupvalues >= 1) {
	//     Table *reg = hvalue(&G(L)->l_registry);
	//     const TValue *gt = luaH_getint(reg, LUA_RIDX_GLOBALS);
	//     setobj(L, cl->upvals[0]->v.p, gt);
	//   }
	if len(cl.UpVals) > 0 {
		gt := GetGlobalTable(L)
		uv := cl.UpVals[0]
		uv.Close(objectapi.TValue{Tt: objectapi.TagTable, Val: gt})
	}
}

// Load compiles Lua source and pushes the resulting closure.
// Returns StatusOK on success, StatusErrSyntax on parse error.
func Load(L *stateapi.LuaState, reader lexapi.LexReader, source string) int {
	return ProtectedParser(L, reader, source)
}

// ---------------------------------------------------------------------------
// Coroutine support (basic stubs)
// ---------------------------------------------------------------------------

// resumeError removes nArgs from the coroutine stack, pushes an error message,
// and returns StatusErrRun. Mirrors: resume_error in ldo.c:907-915.
func resumeError(L *stateapi.LuaState, msg string, nArgs int) (int, int) {
	L.Top -= nArgs // remove args from the stack
	st := L.Global.StringTable.(*luastringapi.StringTable)
	stateapi.PushValue(L, objectapi.MakeString(st.Intern(msg)))
	return stateapi.StatusErrRun, 1
}

// Resume resumes a coroutine.
// Mirrors: lua_resume in ldo.c (simplified)
func Resume(L *stateapi.LuaState, from *stateapi.LuaState, nArgs int) (int, int) {
	if L.Status == stateapi.StatusOK {
		// Starting a new coroutine
		if L.CI != &L.BaseCI {
			return resumeError(L, "cannot resume non-suspended coroutine", nArgs)
		} else if L.Top-(L.CI.Func+1) == nArgs {
			// No function on stack (only args) — dead coroutine
			return resumeError(L, "cannot resume dead coroutine", nArgs)
		}
	} else if L.Status != stateapi.StatusYield {
		return resumeError(L, "cannot resume dead coroutine", nArgs)
	}

	if from != nil {
		L.NCCalls = from.NCCalls & 0xFFFF
	} else {
		L.NCCalls = 0
	}
	L.NCCalls++

	// Catch LuaBaseLevel (self-close) that propagates past all inner
	// RunProtected calls. This is the outermost handler.
	// Mirrors: luaD_throwbaselevel reaching the base luaD_rawrunprotected.
	var status int
	var baseLevelCaught bool
	var baseLevelStatus int
	func() {
		defer func() {
			if r := recover(); r != nil {
				if bl, ok := r.(stateapi.LuaBaseLevel); ok {
					baseLevelCaught = true
					baseLevelStatus = bl.Status
				} else {
					panic(r) // re-panic anything else
				}
			}
		}()
		status = RunProtected(L, func() {
			if L.Status == stateapi.StatusOK {
				// Starting — call the function on the stack
				funcIdx := L.Top - nArgs - 1
				Call(L, funcIdx, stateapi.MultiRet)
			} else {
				// Resuming from yield
				L.Status = stateapi.StatusOK
				ci := L.CI
				if !ci.IsLua() {
					// C function with continuation
					if ci.K != nil {
						n := ci.K(L, stateapi.StatusYield, ci.Ctx)
						PosCall(L, ci, n)
					}
				}
				// Continue executing
				unroll(L)
			}
		})
	}()

	if baseLevelCaught {
		// Self-close terminated the coroutine.
		// The coroutine is already reset (CI at base, TBC closed).
		L.Status = baseLevelStatus
		return baseLevelStatus, 0
	}

	// Continue running after recoverable errors.
	// Mirrors: precover in ldo.c
	status = precover(L, status)

	L.Status = status
	var nresults int
	if status == stateapi.StatusYield {
		nresults = L.CI.NYield
	} else if status != stateapi.StatusOK {
		// Unrecoverable error — push error message like C Lua's seterrorobj.
		// Mirrors: lua_resume error path in ldo.c:997-1001
		// Copy error from top-1 to top, increment top.
		// This ensures the error object survives xmove + closethread later.
		if L.Top > 0 && L.Top < len(L.Stack) {
			L.Stack[L.Top].Val = L.Stack[L.Top-1].Val
			L.Top++
		}
		L.CI.Top = L.Top
		nresults = L.Top - (L.CI.Func + 1)
	} else {
		nresults = L.Top - (L.CI.Func + 1)
	}
	return status, nresults
}

// findPCall searches the CI chain for a suspended protected call
// (a "recover point"). Returns the CI with CISTYPCall set, or nil.
// Mirrors: findpcall in ldo.c
func findPCall(L *stateapi.LuaState) *stateapi.CallInfo {
	for ci := L.CI; ci != nil; ci = ci.Prev {
		if ci.CallStatus&stateapi.CISTYPCall != 0 {
			return ci
		}
	}
	return nil
}

// precover recovers from errors during unroll by finding pcall recovery
// points. When an error occurs during unroll (e.g., __close method errors
// after yield-resume), this finds the nearest pcall and re-runs unroll
// in protected mode.
// Mirrors: precover in ldo.c
func precover(L *stateapi.LuaState, status int) int {
	for isErrorStatus(status) {
		ci := findPCall(L)
		if ci == nil {
			break // no recovery point — unrecoverable error
		}
		L.CI = ci // go down to recovery function
		// Save error status for finishPCallK to retrieve.
		// Uses bits 12-14 of CallStatus (not ci.Ctx, which is continuation context).
		// Mirrors: setcistrecst(ci, status) in C Lua precover (ldo.c:960)
		ci.SetRecst(status)
		status = RunProtected(L, func() {
			unroll(L)
		})
	}
	return status
}

// isErrorStatus returns true for error statuses (not OK, not Yield).
func isErrorStatus(status int) bool {
	return status != stateapi.StatusOK && status != stateapi.StatusYield
}

// unroll executes the full continuation stack.
func unroll(L *stateapi.LuaState) {
	for L.CI != &L.BaseCI {
		ci := L.CI
		if !ci.IsLua() {
			// C function — finish its call.
			// Mirrors: finishCcall in ldo.c
			// Check CISTYPCall first — higher priority than CISTClsRet.
			// When both are set (pcall error close yielded), finishPCallK
			// must run to place the pcall error result.
			if ci.CallStatus&stateapi.CISTYPCall != 0 {
				ci.CallStatus &^= stateapi.CISTClsRet // clear close flag
				status := finishPCallK(L, ci)
				if ci.K != nil {
					n := ci.K(L, status, ci.Ctx)
					PosCall(L, ci, n)
				} else {
					if isErrorStatus(status) {
						errMsg := L.Stack[ci.Func].Val
						top := L.Top
						CheckStack(L, 2)
						L.Stack[top].Val = objectapi.False
						L.Stack[top+1].Val = errMsg
						L.Top = top + 2
						PosCall(L, ci, 2)
					} else {
						nres := L.Top - (ci.Func + 1)
						PosCall(L, ci, nres)
					}
				}
			} else if ci.CallStatus&stateapi.CISTClsRet != 0 {
				// Normal return close (no pcall) — just redo PosCall.
				PosCall(L, ci, ci.NRes)
			} else {
				// Normal C function resume (no pcall, no close).
				status := stateapi.StatusYield
				if ci.K != nil {
					n := ci.K(L, status, ci.Ctx)
					PosCall(L, ci, n)
				} else {
					nres := L.Top - (ci.Func + 1)
					PosCall(L, ci, nres)
				}
			}
		} else {
			// Finish any interrupted op before resuming execution.
			// Mirrors: luaV_finishOp call in ldo.c unroll() —
			// called UNCONDITIONALLY for all Lua CIs.
			FinishOp(L, ci)
			Execute(L, ci)
		}
	}
}

// Yield yields a coroutine.
// Mirrors: lua_yieldk in ldo.c (simplified)
func Yield(L *stateapi.LuaState, nResults int) {
	if !L.Yieldable() {
		if L != L.Global.MainThread {
			RunError(L, "attempt to yield across a C-call boundary")
		} else {
			RunError(L, "attempt to yield from outside a coroutine")
		}
	}
	L.Status = stateapi.StatusYield
	ci := L.CI
	ci.NYield = nResults
	if !ci.IsLua() {
		// C function yield
		panic(stateapi.LuaYield{NResults: nResults})
	}
}

// ---------------------------------------------------------------------------
// Utility: get the global table (_G) from the registry
// ---------------------------------------------------------------------------

// GetGlobalTable returns the global table from the registry.
func GetGlobalTable(L *stateapi.LuaState) *tableapi.Table {
	reg := L.Global.Registry.Val.(*tableapi.Table)
	gval, _ := reg.GetInt(int64(stateapi.RegistryIndexGlobals))
	return gval.Val.(*tableapi.Table)
}

// ---------------------------------------------------------------------------
// internProtoStrings walks a Proto tree and interns all LuaString values
// through the state's StringTable so they have proper hash values.
// The parser creates LuaString with Hash_=0 (state-independent parsing).
// Table hash lookups require matching hashes for correct bucket placement.
// ---------------------------------------------------------------------------

func internProtoStrings(L *stateapi.LuaState, p *objectapi.Proto) {
	st := L.Global.StringTable.(*luastringapi.StringTable)
	// longCache deduplicates long strings within this proto tree,
	// since st.Intern only deduplicates short strings.
	longCache := make(map[string]*objectapi.LuaString)
	internProtoStringsRec(st, p, longCache)
}

func internProtoStringsRec(st *luastringapi.StringTable, p *objectapi.Proto, longCache map[string]*objectapi.LuaString) {
	// Intern source name
	if p.Source != nil {
		p.Source = st.Intern(p.Source.Data)
	}

	// Intern constants
	for i := range p.Constants {
		k := &p.Constants[i]
		switch k.Tt {
		case objectapi.TagShortStr:
			old := k.Val.(*objectapi.LuaString)
			interned := st.Intern(old.Data)
			*k = objectapi.MakeString(interned)
		case objectapi.TagLongStr:
			old := k.Val.(*objectapi.LuaString)
			if cached, ok := longCache[old.Data]; ok {
				*k = objectapi.MakeString(cached)
			} else {
				longCache[old.Data] = old
			}
		}
	}

	// Intern upvalue names
	for i := range p.Upvalues {
		if p.Upvalues[i].Name != nil {
			p.Upvalues[i].Name = st.Intern(p.Upvalues[i].Name.Data)
		}
	}

	// Intern local variable names
	for i := range p.LocVars {
		if p.LocVars[i].Name != nil {
			p.LocVars[i].Name = st.Intern(p.LocVars[i].Name.Data)
		}
	}

	// Recurse into nested protos
	for _, child := range p.Protos {
		internProtoStringsRec(st, child, longCache)
	}
}

// ---------------------------------------------------------------------------
// To-be-closed (TBC) variable support
// ---------------------------------------------------------------------------

// getLocalName returns the name of the local variable at the given stack offset
// from the function slot, using Proto.LocVars debug info.
// Mirrors: luaF_getlocalname in lfunc.c + luaG_findlocal in ldebug.c
func getLocalName(L *stateapi.LuaState, ci *stateapi.CallInfo, idx int) string {
	if ci == nil || !ci.IsLua() {
		return "?"
	}
	fn := L.Stack[ci.Func].Val
	if fn.Tt != objectapi.TagLuaClosure {
		return "?"
	}
	cl, ok := fn.Val.(*closureapi.LClosure)
	if !ok || cl == nil || cl.Proto == nil {
		return "?"
	}
	proto := cl.Proto
	// idx is stack offset from function slot (1-based: 1=first local)
	// Current PC for this call frame
	pc := ci.SavedPC - 1
	if pc < 0 {
		pc = 0
	}
	// Count active locals at this PC (mirrors luaF_getlocalname)
	localNum := idx // 1-based local number to find
	for i := 0; i < len(proto.LocVars) && proto.LocVars[i].StartPC <= pc; i++ {
		if pc < proto.LocVars[i].EndPC { // variable is active
			localNum--
			if localNum == 0 {
				if proto.LocVars[i].Name != nil {
					return proto.LocVars[i].Name.Data
				}
				return "?"
			}
		}
	}
	return "?"
}

// maxTBCDelta is the maximum delta between TBC list entries.
// Matches C Lua's MAXDELTA (USHRT_MAX = 65535).
// When the gap between consecutive TBC variables exceeds this,
// dummy nodes (delta=0) are inserted every maxTBCDelta slots.
const maxTBCDelta = 65535

// MarkTBC marks a stack slot as to-be-closed.
// For large gaps between TBC variables, inserts dummy nodes every maxTBCDelta
// slots so the uint16 delta never overflows.
// Mirrors: luaF_newtbcupval in lfunc.c
func MarkTBC(L *stateapi.LuaState, level int) {
	obj := L.Stack[level].Val
	// false and nil don't need closing (C Lua: l_isfalse check)
	if obj.IsNil() || obj.Tt == objectapi.TagFalse {
		return
	}
	// Check that __close metamethod exists (C Lua: checkclosemth)
	tm := mmapi.GetTMByObj(L.Global, obj, mmapi.TM_CLOSE)
	if tm.IsNil() {
		// Get variable name from debug info (C Lua: luaG_findlocal)
		vname := "?"
		if L.CI != nil {
			idx := level - L.CI.Func // stack offset from function slot
			vname = getLocalName(L, L.CI, idx)
		}
		RunError(L, "variable '"+vname+"' got a non-closable value")
	}
	// Insert dummy nodes for large gaps, matching C Lua's luaF_newtbcupval.
	// C Lua: while (cast_uint(level - L->tbclist.p) > MAXDELTA) {
	//          L->tbclist.p += MAXDELTA; L->tbclist.p->tbclist.delta = 0; }
	// We use TBCList=-1 to mean "no previous TBC". For the first TBC,
	// treat the previous position as -1 (virtual base).
	prev := L.TBCList // -1 if no previous TBC
	if prev < 0 {
		prev = -1
	}
	for level-prev > maxTBCDelta {
		prev += maxTBCDelta
		L.Stack[prev].TBCDelta = 0 // dummy node: delta=0
	}
	// Now the gap from prev to level fits in uint16
	if prev < 0 {
		// First TBC variable (no dummies were needed either)
		L.Stack[level].TBCDelta = uint16(level + 1)
	} else {
		L.Stack[level].TBCDelta = uint16(level - prev)
	}
	L.TBCList = level
}

// popTBCList removes the top element from the TBC list, including any
// dummy nodes below it. Returns the new TBC list head.
// Mirrors: poptbclist in lfunc.c
func popTBCList(L *stateapi.LuaState) {
	tbc := L.TBCList
	delta := int(L.Stack[tbc].TBCDelta)
	L.Stack[tbc].TBCDelta = 0 // clear

	if delta <= 0 {
		// delta should be > 0 for real nodes (assert in C Lua)
		L.TBCList = -1
		return
	}
	tbc -= delta
	// Skip dummy nodes (delta == 0) going backwards by maxTBCDelta each
	for tbc >= 0 && L.Stack[tbc].TBCDelta == 0 {
		tbc -= maxTBCDelta
	}
	if tbc < 0 {
		L.TBCList = -1
	} else {
		L.TBCList = tbc
	}
}

// CloseTBC calls __close on all TBC variables from L.TBCList down to (but not including) level.
// Then resets L.TBCList to the previous TBC variable below level.
// status: stateapi.StatusOK for normal close, or an error status for error close.
// errObj: the error object to pass to __close (nil for normal close).
// Mirrors: luaF_close (the TBC portion) in lfunc.c + prepcallclosemth + callclosemethod
func CloseTBC(L *stateapi.LuaState, level int) {
	CloseTBCWithError(L, level, stateapi.StatusOK, objectapi.Nil, true)
}

// CloseTBCWithError is CloseTBC with error status and error object.
// For normal close: status=StatusOK, errObj=Nil → __close(obj) with 1 arg
// For error close: status!=StatusOK, errObj=error → __close(obj, err) with 2 args
// yieldable controls whether __close can yield (false in CloseProtected path).
func CloseTBCWithError(L *stateapi.LuaState, level int, status int, errObj objectapi.TValue, yieldable bool) {
	for L.TBCList >= level {
		tbc := L.TBCList
		// Pop from TBC list first (removes real node + any dummy chain below it).
		// This matches C Lua's luaF_close: poptbclist(L) before prepcallclosemth.
		popTBCList(L)

		// Call __close metamethod if the value is not nil and not false
		obj := L.Stack[tbc].Val
		if obj.IsNil() || (obj.Tt == objectapi.TagFalse) {
			continue
		}

		tm := mmapi.GetTMByObj(L.Global, obj, mmapi.TM_CLOSE)
		callCloseMethod(L, tm, obj, tbc, status, errObj, yieldable)
	}
}

// callCloseMethod calls a __close metamethod: tm(obj) or tm(obj, err).
// This is the unprotected version used by OP_CLOSE / OP_RETURN.
// yieldable: true for normal close (OP_CLOSE/OP_RETURN), false for CloseProtected.
// Mirrors: prepcallclosemth + callclosemethod in lfunc.c
func callCloseMethod(L *stateapi.LuaState, tm, obj objectapi.TValue, level int, status int, errObj objectapi.TValue, yieldable bool) {
	// C Lua's prepcallclosemth has a three-way switch:
	//   StatusOK       → reset L.Top to level+1 (call at TBC var level)
	//   StatusCloseKTop → don't change L.Top (return values above TBC)
	//   error status   → set error obj at level+1, L.Top = level+2
	isError := false
	switch status {
	case stateapi.StatusOK:
		L.Top = level + 1
	case stateapi.StatusCloseKTop:
		// Don't reset L.Top — return values are above the TBC variable
	default:
		// Error close: set error object at level+1
		isError = true
		if level+2 < len(L.Stack) {
			L.Stack[level+1].Val = errObj
			L.Top = level + 2
		}
	}

	top := L.Top
	// __close(obj) for normal/CLOSEKTOP, __close(obj, err) for error
	nargs := 1
	if isError {
		nargs = 2
	}
	needed := top + 1 + nargs
	if needed >= len(L.Stack) {
		// During stack overflow recovery the stack is already at errorStackSize.
		// We cannot grow further. But prepcallclosemth already reset L.Top
		// to near the TBC level, so there should be room. If not, we have
		// a deeper problem — skip the close call rather than panic.
		return
	}
	L.Stack[top].Val = tm
	L.Stack[top+1].Val = obj
	if isError {
		L.Stack[top+2].Val = errObj
	}
	L.Top = top + 1 + nargs
	// Mark current CI as closing TBC vars
	oldStatus := L.CI.CallStatus
	L.CI.CallStatus |= stateapi.CISTClsRet
	if yieldable {
		Call(L, top, 0)
	} else {
		CallNoYield(L, top, 0)
	}
	L.CI.CallStatus = oldStatus
}

// CloseProtected closes TBC variables in protected mode.
// Used by PCall error path: if a __close method errors, the error
// replaces the previous one and closing continues with remaining vars.
// Mirrors: luaD_closeprotected in ldo.c
func CloseProtected(L *stateapi.LuaState, level int, status int, errObj objectapi.TValue) (int, objectapi.TValue) {
	oldCI := L.CI
	oldAllowHook := L.AllowHook
	for L.TBCList >= level {
		// runProtectedCatchBaseLevel wraps RunProtected and also catches
		// LuaBaseLevel panics that RunProtected re-panics. In C Lua,
		// luaD_throwbaselevel longjmps to the BASE rawrunprotected, but
		// Go's panic/recover always catches at the nearest recover.
		// RunProtected re-panics LuaBaseLevel so it can reach Resume's
		// outer wrapper for the self-close-from-within case. But when
		// CloseProtected is called from an EXTERNAL close (e.g., main
		// thread closing a coroutine whose __close re-closes itself),
		// the LuaBaseLevel must not escape. We catch it here and convert
		// to a status code, which is what C Lua's rawrunprotected does.
		var newStatus int
		var baseLevelCaught bool
		func() {
			defer func() {
				if r := recover(); r != nil {
					if bl, ok := r.(stateapi.LuaBaseLevel); ok {
						baseLevelCaught = true
						newStatus = bl.Status
					} else {
						panic(r)
					}
				}
			}()
			newStatus = RunProtected(L, func() {
				CloseTBCWithError(L, level, status, errObj, false)
			})
		}()
		if baseLevelCaught {
			// The __close handler triggered a self-close (re-close).
			// The nested CloseThread already reset the coroutine and
			// closed all TBC vars. Return the status directly.
			return newStatus, errObj
		}
		if newStatus == stateapi.StatusOK {
			return status, errObj // all closed successfully
		}
		// A __close method errored. The new error replaces the old one.
		L.CI = oldCI
		L.AllowHook = oldAllowHook
		status = newStatus
		// The new error object is on the stack at L.Top-1.
		// But L.Top may be in a bad state after the error.
		// Safely extract the error object with bounds checking.
		if L.Top > 0 && L.Top-1 < len(L.Stack) {
			errObj = L.Stack[L.Top-1].Val
		}
		// Reset L.Top to near level to prevent it from climbing
		// during cascading errors. C Lua's closepaux → luaF_close →
		// prepcallclosemth resets L.Top on each iteration.
		// We do the same here to keep L.Top bounded.
		if level+2 < len(L.Stack) {
			L.Top = level + 2
		}
	}
	return status, errObj
}

// CloseThread closes all TBC variables in a coroutine and resets it.
// Mirrors: lua_closethread → luaE_resetthread in lstate.c:315
func CloseThread(L *stateapi.LuaState, from *stateapi.LuaState) int {
	if from != nil {
		L.NCCalls = from.NCCalls & 0xFFFF
	} else {
		L.NCCalls = 0
	}

	origStatus := L.Status // save BEFORE resetCI

	// resetCI (lstate.c:151)
	ci := &L.BaseCI
	L.CI = ci
	ci.Func = 0
	L.Stack[0].Val = objectapi.Nil
	ci.Top = 1 + stateapi.BasicStackSize/2
	ci.K = nil
	ci.CallStatus = stateapi.CISTC
	L.Status = stateapi.StatusOK
	L.ErrFunc = 0

	// Convert yield to OK
	status := origStatus
	if status == stateapi.StatusYield {
		status = stateapi.StatusOK
	}

	// Close TBC from level 1 in protected mode
	errObj := objectapi.Nil
	if L.Top > 1 {
		errObj = L.Stack[L.Top-1].Val
	}
	newStatus, newErrObj := CloseProtected(L, 1, status, errObj)
	if newStatus != stateapi.StatusOK {
		// Place the error object at stack[1] and set Top = 2.
		// We can't use SetErrorObj here because L.Top may be wrong after
		// CloseProtected ran __close methods. Use the returned errObj directly.
		L.Stack[1].Val = newErrObj
		L.Top = 2
		// If closing itself, throw to base level (bypasses all inner pcalls).
		// Mirrors: lua_closethread → luaD_throwbaselevel in lstate.c:335
		if L == from {
			panic(stateapi.LuaBaseLevel{Status: newStatus})
		}
		return newStatus
	}
	L.Top = 1
	// If closing itself, throw to base level even on OK status.
	// Mirrors: lua_closethread → luaD_throwbaselevel in lstate.c:335
	if L == from {
		panic(stateapi.LuaBaseLevel{Status: status})
	}
	return status
}
