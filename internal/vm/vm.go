// VM execution loop — the heart of the Lua interpreter.
//
// This is the Go equivalent of C's lvm.c. The core is Execute(),
// a giant switch on opcodes that runs Lua bytecode.
//
// Reference: lua-master/lvm.c, .analysis/05-vm-execution-loop.md
package vm

import (
	"math"
	"sync/atomic"

	closureapi "github.com/akzj/go-lua/internal/closure"
	mmapi "github.com/akzj/go-lua/internal/metamethod"
	objectapi "github.com/akzj/go-lua/internal/object"
	opcodeapi "github.com/akzj/go-lua/internal/opcode"
	stateapi "github.com/akzj/go-lua/internal/state"
	tableapi "github.com/akzj/go-lua/internal/table"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const maxTagLoop = 2000

// ---------------------------------------------------------------------------
// Periodic GC helper
// ---------------------------------------------------------------------------

// checkPeriodicGC increments the allocation counter and triggers a GC step
// every 5000 allocations if a step function is registered.
func checkPeriodicGC(g *stateapi.GlobalState, L *stateapi.LuaState) {
	g.GCAllocCount++
	if g.GCAllocCount%5000 == 0 && g.GCStepFn != nil {
		g.GCStepFn(L)
	}
}

// ---------------------------------------------------------------------------
// Execute — the main VM execution loop
// ---------------------------------------------------------------------------

// FinishOp finishes execution of an opcode interrupted by a yield.
// When a metamethod yields (via panic(LuaYield{})), intermediate Go frames
// (callTMRes, tryBinTM, callOrderTM) are destroyed. This function places
// the metamethod result into the correct register before Execute resumes.
// Also handles __close interruption (OP_CLOSE/OP_RETURN).
// Mirrors: luaV_finishOp in lvm.c:568-618
func FinishOp(L *stateapi.LuaState, ci *stateapi.CallInfo) {
	cl := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
	code := cl.Proto.Code
	inst := code[ci.SavedPC-1] // interrupted instruction
	op := opcodeapi.GetOpCode(inst)
	base := ci.Func + 1
	switch op {

	// Category 1: Binary arithmetic metamethods
	// savedpc-1 = OP_MMBIN/MMBINI/MMBINK (the interrupted instruction)
	// savedpc-2 = OP_ADD/SUB/etc (the original arithmetic instruction)
	// Pop TM result from stack top → store in RA of original arith op
	case opcodeapi.OP_MMBIN, opcodeapi.OP_MMBINI, opcodeapi.OP_MMBINK:
		L.Top--
		prevInst := code[ci.SavedPC-2]
		dest := base + opcodeapi.GetArgA(prevInst)
		L.Stack[dest].Val = L.Stack[L.Top].Val

	// Category 2: Unary + Table Get
	// Pop TM result from stack top → store in RA of this instruction
	case opcodeapi.OP_UNM, opcodeapi.OP_BNOT, opcodeapi.OP_LEN,
		opcodeapi.OP_GETTABUP, opcodeapi.OP_GETTABLE, opcodeapi.OP_GETI,
		opcodeapi.OP_GETFIELD, opcodeapi.OP_SELF:
		L.Top--
		dest := base + opcodeapi.GetArgA(inst)
		L.Stack[dest].Val = L.Stack[L.Top].Val

	// Category 3: Comparisons
	// Evaluate TM result as boolean, then conditional jump.
	// savedpc points to the OP_JMP instruction after the comparison.
	// If res != k, skip the jump (savedpc++).
	case opcodeapi.OP_LT, opcodeapi.OP_LE,
		opcodeapi.OP_LTI, opcodeapi.OP_LEI,
		opcodeapi.OP_GTI, opcodeapi.OP_GEI,
		opcodeapi.OP_EQ:
		res := !L.Stack[L.Top-1].Val.IsFalsy()
		L.Top--
		if res != (opcodeapi.GetArgK(inst) != 0) {
			ci.SavedPC++ // skip jump
		}

	// Category 4: Concat
	// Reposition TM result, adjust top, continue concat loop
	case opcodeapi.OP_CONCAT:
		top := L.Top - 1
		a := opcodeapi.GetArgA(inst)
		total := (top - 1) - (base + a)
		L.Stack[top-2].Val = L.Stack[top].Val // put TM result in proper position
		L.Top = top - 1
		if total > 1 {
			Concat(L, total) // concat remaining (may yield again)
		}

	// Category 5: Close/Return (already implemented)
	case opcodeapi.OP_CLOSE:
		ci.SavedPC--
	case opcodeapi.OP_RETURN:
		ra := base + opcodeapi.GetArgA(inst)
		L.Top = ra + ci.NRes
		ci.SavedPC--

	default:
		// OP_TFORCALL, OP_CALL, OP_TAILCALL,
		// OP_SETTABUP, OP_SETTABLE, OP_SETI, OP_SETFIELD
		// No action needed — results already in correct place or no result needed
	}
}

// Execute runs the VM main loop for the given CallInfo.
// This is the Go equivalent of luaV_execute in lvm.c.
func Execute(L *stateapi.LuaState, ci *stateapi.CallInfo) {
startfunc:
	cl := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
	k := cl.Proto.Constants
	code := cl.Proto.Code
	base := ci.Func + 1

	// Mirrors: luaG_tracecall in ldebug.c — fire call hook at function entry.
	// For tail calls, PreTailCall sets CISTTail and savedpc=0, then jumps here.
	// The call hook for non-tail calls is already fired by PreCall (non-vararg)
	// or OP_VARARGPREP (vararg). Only tail calls need this path.
	// For vararg tail calls, defer to OP_VARARGPREP.
	if L.HookMask != 0 && ci.CallStatus&stateapi.CISTTail != 0 &&
		ci.SavedPC == 0 && !cl.Proto.IsVararg() {
		CallHook(L, ci)
	}

	for {
		inst := code[ci.SavedPC]
		ci.SavedPC++ // increment BEFORE hook check — mirrors C Lua vmfetch

		// Hook dispatch: fire line/count hooks if active.
		// Skip for OP_VARARGPREP — C Lua's luaG_tracecall returns 0 (trap=0)
		// for vararg functions, so traceexec is not called for instruction 0.
		// The call hook and OldPC adjustment happen inside OP_VARARGPREP instead.
		// Mirrors: vmfetch trap check in lvm.c + luaG_tracecall in ldebug.c
		if L.HookMask&(stateapi.MaskLine|stateapi.MaskCount) != 0 && L.AllowHook &&
			opcodeapi.GetOpCode(inst) != opcodeapi.OP_VARARGPREP {
			TraceExec(L, ci)
		}
		op := opcodeapi.GetOpCode(inst)
		ra := base + opcodeapi.GetArgA(inst)

		switch op {

		// ===== Load/Move =====

		case opcodeapi.OP_MOVE:
			rb := base + opcodeapi.GetArgB(inst)
			L.Stack[ra].Val = L.Stack[rb].Val

		case opcodeapi.OP_LOADI:
			L.Stack[ra].Val = objectapi.MakeInteger(int64(opcodeapi.GetArgSBx(inst)))

		case opcodeapi.OP_LOADF:
			L.Stack[ra].Val = objectapi.MakeFloat(float64(opcodeapi.GetArgSBx(inst)))

		case opcodeapi.OP_LOADK:
			L.Stack[ra].Val = k[opcodeapi.GetArgBx(inst)]

		case opcodeapi.OP_LOADKX:
			ax := opcodeapi.GetArgAx(code[ci.SavedPC])
			ci.SavedPC++
			L.Stack[ra].Val = k[ax]

		case opcodeapi.OP_LOADFALSE:
			L.Stack[ra].Val = objectapi.False

		case opcodeapi.OP_LFALSESKIP:
			L.Stack[ra].Val = objectapi.False
			ci.SavedPC++ // skip next instruction

		case opcodeapi.OP_LOADTRUE:
			L.Stack[ra].Val = objectapi.True

		case opcodeapi.OP_LOADNIL:
			b := opcodeapi.GetArgB(inst)
			for i := 0; i <= b; i++ {
				L.Stack[ra+i].Val = objectapi.Nil
			}

		// ===== Upvalues =====

		case opcodeapi.OP_GETUPVAL:
			b := opcodeapi.GetArgB(inst)
			L.Stack[ra].Val = cl.UpVals[b].Get(L.Stack)

		case opcodeapi.OP_SETUPVAL:
			b := opcodeapi.GetArgB(inst)
			cl.UpVals[b].Set(L.Stack, L.Stack[ra].Val)

		case opcodeapi.OP_CLOSE:
			closureapi.CloseUpvals(L, ra)
			CloseTBC(L, ra)

		case opcodeapi.OP_TBC:
			// To-be-closed: mark the variable in the TBC linked list
			MarkTBC(L, ra)

		// ===== Table access =====

		case opcodeapi.OP_GETTABUP:
			b := opcodeapi.GetArgB(inst)
			upval := cl.UpVals[b].Get(L.Stack)
			rc := k[opcodeapi.GetArgC(inst)]
			if upval.IsTable() {
				h := upval.Val.(*tableapi.Table)
				val, found := h.Get(rc)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, upval, rc, ra)
				}
			} else {
				FinishGet(L, upval, rc, ra)
			}

		case opcodeapi.OP_GETTABLE:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsTable() {
				h := rb.Val.(*tableapi.Table)
				val, found := h.Get(rc)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, rb, rc, ra)
				}
			} else {
				FinishGet(L, rb, rc, ra)
			}

		case opcodeapi.OP_GETI:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			c := int64(opcodeapi.GetArgC(inst))
			if rb.IsTable() {
				h := rb.Val.(*tableapi.Table)
				val, found := h.GetInt(c)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, rb, objectapi.MakeInteger(c), ra)
				}
			} else {
				FinishGet(L, rb, objectapi.MakeInteger(c), ra)
			}

		case opcodeapi.OP_GETFIELD:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := k[opcodeapi.GetArgC(inst)]
			if rb.IsTable() {
				h := rb.Val.(*tableapi.Table)
				val, found := h.Get(rc)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, rb, rc, ra)
				}
			} else {
				FinishGet(L, rb, rc, ra)
			}

		case opcodeapi.OP_SETTABUP:
			b := opcodeapi.GetArgB(inst)
			upval := cl.UpVals[opcodeapi.GetArgA(inst)].Get(L.Stack)
			rb := k[b]
			var rc objectapi.TValue
			if opcodeapi.GetArgK(inst) != 0 {
				rc = k[opcodeapi.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcodeapi.GetArgC(inst)].Val
			}
			ra = 0 // OP_SETTABUP uses A for upvalue index, not register
			if upval.IsTable() {
				tableSetWithMeta(L, upval, rb, rc)
			} else {
				FinishSet(L, upval, rb, rc)
			}

		case opcodeapi.OP_SETTABLE:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			var rc objectapi.TValue
			if opcodeapi.GetArgK(inst) != 0 {
				rc = k[opcodeapi.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcodeapi.GetArgC(inst)].Val
			}
			tval := L.Stack[ra].Val
			if tval.IsTable() {
				tableSetWithMeta(L, tval, rb, rc)
			} else {
				FinishSet(L, tval, rb, rc)
			}

		case opcodeapi.OP_SETI:
			b := int64(opcodeapi.GetArgB(inst))
			var rc objectapi.TValue
			if opcodeapi.GetArgK(inst) != 0 {
				rc = k[opcodeapi.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcodeapi.GetArgC(inst)].Val
			}
			tval := L.Stack[ra].Val
			if tval.IsTable() {
				tableSetWithMeta(L, tval, objectapi.MakeInteger(b), rc)
			} else {
				FinishSet(L, tval, objectapi.MakeInteger(b), rc)
			}

		case opcodeapi.OP_SETFIELD:
			rb := k[opcodeapi.GetArgB(inst)]
			var rc objectapi.TValue
			if opcodeapi.GetArgK(inst) != 0 {
				rc = k[opcodeapi.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcodeapi.GetArgC(inst)].Val
			}
			tval := L.Stack[ra].Val
			if tval.IsTable() {
				tableSetWithMeta(L, tval, rb, rc)
			} else {
				FinishSet(L, tval, rb, rc)
			}

		case opcodeapi.OP_NEWTABLE:
			b := opcodeapi.GetArgVB(inst)
			c := opcodeapi.GetArgVC(inst)
			if b > 0 {
				b = 1 << (b - 1)
			}
			if opcodeapi.GetArgK(inst) != 0 {
				c += opcodeapi.GetArgAx(code[ci.SavedPC]) * (opcodeapi.MaxArgVC + 1)
			}
			ci.SavedPC++ // skip extra arg
			t := tableapi.New(c, b)
			L.Global.LinkGC(t) // V5: register in allgc chain
			size := t.EstimateBytes()
			atomic.AddInt64(&L.Global.GCTotalBytes, size)
			// V5 GC sweep handles dealloc accounting — no AddCleanup needed
			L.Stack[ra].Val = objectapi.TValue{Tt: objectapi.TagTable, Val: t}

			// Periodic GC: run Lua GC during tight allocation loops.
			// V5 GC handles __gc via finobj list.
			if g := L.Global; !g.GCStopped {
				checkPeriodicGC(g, L)
			}

		case opcodeapi.OP_SELF:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := k[opcodeapi.GetArgC(inst)]
			L.Stack[ra+1].Val = rb // save table as self
			if rb.IsTable() {
				h := rb.Val.(*tableapi.Table)
				val, found := h.Get(rc)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, rb, rc, ra)
				}
			} else {
				FinishGet(L, rb, rc, ra)
			}

		// ===== Arithmetic with immediate =====

		case opcodeapi.OP_ADDI:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ic := int64(opcodeapi.GetArgSC(inst))
			if rb.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() + ic)
				ci.SavedPC++ // skip MMBIN
			} else if rb.IsFloat() {
				L.Stack[ra].Val = objectapi.MakeFloat(rb.Float() + float64(ic))
				ci.SavedPC++
			}
			// else: fall through to MMBINI on next instruction

		// ===== Arithmetic with constant =====

		case opcodeapi.OP_ADDK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() + kc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a + b }, func(a, b float64) float64 { return a + b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_SUBK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() - kc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a - b }, func(a, b float64) float64 { return a - b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_MULK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() * kc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a * b }, func(a, b float64) float64 { return a * b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_MODK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(IMod(L, rb.Integer(), kc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = objectapi.MakeInteger(IMod(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = objectapi.MakeFloat(FMod(toFloat(nb), toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_POWK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			nb, ok1 := ToNumberNS(rb)
			nc, ok2 := ToNumberNS(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeFloat(math.Pow(nb, nc))
				ci.SavedPC++
			}

		case opcodeapi.OP_DIVK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			nb, ok1 := ToNumberNS(rb)
			nc, ok2 := ToNumberNS(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeFloat(nb / nc)
				ci.SavedPC++
			}

		case opcodeapi.OP_IDIVK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(IDiv(L, rb.Integer(), kc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = objectapi.MakeInteger(IDiv(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = objectapi.MakeFloat(math.Floor(toFloat(nb) / toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		// ===== Bitwise with constant =====

		case opcodeapi.OP_BANDK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib & ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BORK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib | ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BXORK:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			kc := k[opcodeapi.GetArgC(inst)]
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib ^ ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_SHLI:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ic := int64(opcodeapi.GetArgSC(inst))
			ib, ok := toIntegerStrict(rb)
			if ok {
				L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ic, ib))
				ci.SavedPC++
			}

		case opcodeapi.OP_SHRI:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ic := int64(opcodeapi.GetArgSC(inst))
			ib, ok := toIntegerStrict(rb)
			if ok {
				L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ib, -ic))
				ci.SavedPC++
			}

		// ===== Arithmetic register-register =====

		case opcodeapi.OP_ADD:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() + rc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a + b }, func(a, b float64) float64 { return a + b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_SUB:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() - rc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a - b }, func(a, b float64) float64 { return a - b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_MUL:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() * rc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a * b }, func(a, b float64) float64 { return a * b })
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_MOD:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(IMod(L, rb.Integer(), rc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = objectapi.MakeInteger(IMod(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = objectapi.MakeFloat(FMod(toFloat(nb), toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		case opcodeapi.OP_POW:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			nb, ok1 := ToNumberNS(rb)
			nc, ok2 := ToNumberNS(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeFloat(math.Pow(nb, nc))
				ci.SavedPC++
			}

		case opcodeapi.OP_DIV:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			nb, ok1 := ToNumberNS(rb)
			nc, ok2 := ToNumberNS(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeFloat(nb / nc)
				ci.SavedPC++
			}

		case opcodeapi.OP_IDIV:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(IDiv(L, rb.Integer(), rc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = objectapi.MakeInteger(IDiv(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = objectapi.MakeFloat(math.Floor(toFloat(nb) / toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		// ===== Bitwise register-register =====

		case opcodeapi.OP_BAND:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib & ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BOR:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib | ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_BXOR:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ib ^ ic)
				ci.SavedPC++
			}

		case opcodeapi.OP_SHL:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ib, ic))
				ci.SavedPC++
			}

		case opcodeapi.OP_SHR:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ib, -ic))
				ci.SavedPC++
			}

		// ===== Metamethod fallback =====

		case opcodeapi.OP_MMBIN:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			tm := mmapi.TMS(opcodeapi.GetArgC(inst))
			prevInst := code[ci.SavedPC-2]
			result := base + opcodeapi.GetArgA(prevInst)
			tryBinTM(L, L.Stack[ra].Val, rb, result, tm, ra-base, opcodeapi.GetArgB(inst))

		case opcodeapi.OP_MMBINI:
			imm := opcodeapi.GetArgSB(inst)
			tm := mmapi.TMS(opcodeapi.GetArgC(inst))
			flip := opcodeapi.GetArgK(inst) != 0
			prevInst := code[ci.SavedPC-2]
			result := base + opcodeapi.GetArgA(prevInst)
			tryBiniTM(L, L.Stack[ra].Val, imm, flip, result, tm, ra-base)

		case opcodeapi.OP_MMBINK:
			imm := k[opcodeapi.GetArgB(inst)]
			tm := mmapi.TMS(opcodeapi.GetArgC(inst))
			flip := opcodeapi.GetArgK(inst) != 0
			prevInst := code[ci.SavedPC-2]
			result := base + opcodeapi.GetArgA(prevInst)
			tryBinKTM(L, L.Stack[ra].Val, imm, flip, result, tm, ra-base)

		// ===== Unary =====

		case opcodeapi.OP_UNM:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			if rb.IsInteger() {
				L.Stack[ra].Val = objectapi.MakeInteger(-rb.Integer())
			} else if rb.IsFloat() {
				L.Stack[ra].Val = objectapi.MakeFloat(-rb.Float())
			} else {
				tryBinTM(L, rb, rb, ra, mmapi.TM_UNM, opcodeapi.GetArgB(inst), opcodeapi.GetArgB(inst))
			}

		case opcodeapi.OP_BNOT:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ib, ok := toIntegerStrict(rb)
			if ok {
				L.Stack[ra].Val = objectapi.MakeInteger(^ib)
			} else {
				tryBinTM(L, rb, rb, ra, mmapi.TM_BNOT, opcodeapi.GetArgB(inst), opcodeapi.GetArgB(inst))
			}

		case opcodeapi.OP_NOT:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			if rb.IsFalsy() {
				L.Stack[ra].Val = objectapi.True
			} else {
				L.Stack[ra].Val = objectapi.False
			}

		case opcodeapi.OP_LEN:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			ObjLen(L, ra, rb)

		case opcodeapi.OP_CONCAT:
			n := opcodeapi.GetArgB(inst)
			L.Top = ra + n
			Concat(L, n)
			L.Stack[ra].Val = L.Stack[L.Top-1].Val
			L.Top = ci.Top // restore top

			// Periodic GC: string concatenation allocates new strings.
			if g := L.Global; !g.GCStopped {
				checkPeriodicGC(g, L)
			}

		// ===== Comparison =====

		case opcodeapi.OP_EQ:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			// Bump L.Top to frame top so callTMRes scratch space
			// doesn't clobber live registers (metamethod dispatch).
			L.Top = ci.Top
			cond := EqualObj(L, L.Stack[ra].Val, rb)
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++ // skip jump
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_LT:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			L.Top = ci.Top
			cond := LessThan(L, L.Stack[ra].Val, rb)
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_LE:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			L.Top = ci.Top
			cond := LessEqual(L, L.Stack[ra].Val, rb)
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_EQK:
			rb := k[opcodeapi.GetArgB(inst)]
			cond := RawEqualObj(L.Stack[ra].Val, rb)
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_EQI:
			im := int64(opcodeapi.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() == im
			} else if v.IsFloat() {
				cond = v.Float() == float64(im)
			}
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_LTI:
			im := int64(opcodeapi.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() < im
			} else if v.IsFloat() {
				cond = v.Float() < float64(im)
			} else {
				// Metamethod fallback: callOrderITM(L, v, im, false, isf, TM_LT)
				isf := opcodeapi.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, false, isf, mmapi.TM_LT)
			}
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_LEI:
			im := int64(opcodeapi.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() <= im
			} else if v.IsFloat() {
				cond = v.Float() <= float64(im)
			} else {
				isf := opcodeapi.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, false, isf, mmapi.TM_LE)
			}
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_GTI:
			im := int64(opcodeapi.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() > im
			} else if v.IsFloat() {
				cond = v.Float() > float64(im)
			} else {
				// GTI: a > im ⟺ im < a, so flip=true, TM_LT
				isf := opcodeapi.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, true, isf, mmapi.TM_LT)
			}
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_GEI:
			im := int64(opcodeapi.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() >= im
			} else if v.IsFloat() {
				cond = v.Float() >= float64(im)
			} else {
				// GEI: a >= im ⟺ im <= a, so flip=true, TM_LE
				isf := opcodeapi.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, true, isf, mmapi.TM_LE)
			}
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_TEST:
			cond := !L.Stack[ra].Val.IsFalsy()
			if cond != (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcodeapi.OP_TESTSET:
			rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
			if rb.IsFalsy() == (opcodeapi.GetArgK(inst) != 0) {
				ci.SavedPC++ // condition failed, skip jump
			} else {
				L.Stack[ra].Val = rb
				ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
			}

		// ===== Jump =====

		case opcodeapi.OP_JMP:
			ci.SavedPC += opcodeapi.GetArgSJ(inst)

		// ===== For loops =====

		case opcodeapi.OP_FORPREP:
			if ForPrep(L, ra) {
				ci.SavedPC += opcodeapi.GetArgBx(inst) + 1 // skip loop
			}

		case opcodeapi.OP_FORLOOP:
			if ForLoop(L, ra) {
				ci.SavedPC -= opcodeapi.GetArgBx(inst) // jump back
			}

		case opcodeapi.OP_TFORPREP:
			// Swap control and closing variables
			// Before: ra=iter, ra+1=state, ra+2=control, ra+3=closing
			// After:  ra=iter, ra+1=state, ra+2=closing(tbc), ra+3=control
			temp := L.Stack[ra+3].Val
			L.Stack[ra+3].Val = L.Stack[ra+2].Val
			L.Stack[ra+2].Val = temp
			// Mark the closing variable (now at ra+2) as to-be-closed
			// C Lua: halfProtect(luaF_newtbcupval(L, ra + 2))
			MarkTBC(L, ra+2)
			ci.SavedPC += opcodeapi.GetArgBx(inst)
			// Fall through to TFORCALL
			inst = code[ci.SavedPC]
			ci.SavedPC++
			ra = base + opcodeapi.GetArgA(inst)
			goto tforcall

		case opcodeapi.OP_TFORCALL:
			goto tforcall

		case opcodeapi.OP_TFORLOOP:
			if !L.Stack[ra+3].Val.IsNil() {
				ci.SavedPC -= opcodeapi.GetArgBx(inst) // jump back
			}

		// ===== Call/Return =====

		case opcodeapi.OP_CALL:
			b := opcodeapi.GetArgB(inst)
			nresults := opcodeapi.GetArgC(inst) - 1
			if b != 0 {
				L.Top = ra + b
			}
			// SavedPC already set by dispatch loop
			newci := PreCall(L, ra, nresults)
			if newci != nil {
				ci = newci
				goto startfunc
			}
			// C function already executed

		case opcodeapi.OP_TAILCALL:
			b := opcodeapi.GetArgB(inst)
			nparams1 := opcodeapi.GetArgC(inst)
			delta := 0
			if nparams1 != 0 {
				delta = ci.NExtraArgs + nparams1
			}
			if b != 0 {
				L.Top = ra + b
			} else {
				b = L.Top - ra
			}
			if opcodeapi.GetArgK(inst) != 0 {
				closureapi.CloseUpvals(L, base)
			}
			n := PreTailCall(L, ci, ra, b, delta)
			if n < 0 {
				// Lua function — ci.Func already adjusted by delta
				goto startfunc
			}
			// C function executed — restore func and finish
			ci.Func -= delta
			PosCall(L, ci, n)
			goto ret

		case opcodeapi.OP_RETURN:
			b := opcodeapi.GetArgB(inst)
			n := b - 1
			nparams1 := opcodeapi.GetArgC(inst)
			if n < 0 {
				n = L.Top - ra
			}
			if opcodeapi.GetArgK(inst) != 0 {
				// C Lua: save nres, ensure stack space, close upvals+TBC, refresh base/ra
				// Use StatusCloseKTop so callCloseMethod does NOT reset L.Top —
				// return values sit on the stack above the TBC variables.
				ci.NRes = n
				if L.Top < ci.Top {
					L.Top = ci.Top
				}
				closureapi.CloseUpvals(L, base)
				CloseTBCWithError(L, base, stateapi.StatusCloseKTop, objectapi.Nil, true)
				// After close, stack may have been reallocated by __close calls.
				// Refresh base and ra from ci (which uses offsets, not pointers).
				base = ci.Func + 1
				ra = base + opcodeapi.GetArgA(inst)
			}
			if nparams1 != 0 {
				ci.Func -= ci.NExtraArgs + nparams1
			}
			L.Top = ra + n
			PosCall(L, ci, n)
			goto ret

		case opcodeapi.OP_RETURN0:
			if L.HookMask != 0 {
				// Hooks active — fall back to full PosCall (fires return hook)
				L.Top = ra
				PosCall(L, ci, 0)
			} else {
				// Fast path — no hooks
				nres := ci.NResults()
				L.CI = ci.Prev
				L.Top = base - 1
				for i := 0; i < nres; i++ {
					L.Stack[L.Top].Val = objectapi.Nil
					L.Top++
				}
				if nres < 0 {
					L.Top = base - 1
				}
			}
			goto ret

		case opcodeapi.OP_RETURN1:
			if L.HookMask != 0 {
				// Hooks active — fall back to full PosCall (fires return hook)
				L.Top = ra + 1
				PosCall(L, ci, 1)
			} else {
				// Fast path — no hooks
				nres := ci.NResults()
				L.CI = ci.Prev
				if nres == 0 {
					L.Top = base - 1
				} else {
					L.Stack[base-1].Val = L.Stack[ra].Val
					L.Top = base
					for i := 1; i < nres; i++ {
						L.Stack[L.Top].Val = objectapi.Nil
						L.Top++
					}
				}
			}
			goto ret

		// ===== Closure/Vararg =====

		case opcodeapi.OP_CLOSURE:
			bx := opcodeapi.GetArgBx(inst)
			p := cl.Proto.Protos[bx]
			PushClosure(L, p, cl.UpVals, base, ra)

			// Periodic GC: closures are heap-allocated objects.
			if g := L.Global; !g.GCStopped {
				checkPeriodicGC(g, L)
			}

		case opcodeapi.OP_VARARG:
			n := opcodeapi.GetArgC(inst) - 1
			vatab := -1
			if opcodeapi.GetArgK(inst) != 0 {
				vatab = opcodeapi.GetArgB(inst)
			}
			GetVarargs(L, ci, ra, n, vatab)

		case opcodeapi.OP_VARARGPREP:
			AdjustVarargs(L, ci, cl.Proto)
			// Update base after adjustment
			base = ci.Func + 1
			// Fire call hook AFTER adjustment (deferred from PreCall).
			// Mirrors: OP_VARARGPREP in lvm.c calls luaD_hookcall after
			// luaT_adjustvarargs, so debug.getlocal sees correct params.
			if L.HookMask != 0 {
				CallHook(L, ci)
				L.OldPC = 1 // next opcode seen as "new" line
			}

		// ===== Table construction =====

		case opcodeapi.OP_SETLIST:
			n := opcodeapi.GetArgVB(inst)
			last := opcodeapi.GetArgVC(inst)
			h := L.Stack[ra].Val.Val.(*tableapi.Table)
			if n == 0 {
				n = L.Top - ra - 1
			}
			last += n
			if opcodeapi.GetArgK(inst) != 0 {
				last += opcodeapi.GetArgAx(code[ci.SavedPC]) * (opcodeapi.MaxArgVC + 1)
				ci.SavedPC++
			}
			for i := n; i > 0; i-- {
				h.SetInt(int64(last), L.Stack[ra+i].Val)
				last--
			}
			L.Top = ci.Top // restore top

		// ===== Lua 5.5 new opcodes =====

		case opcodeapi.OP_GETVARG:
			// OP_GETVARG: ra = vararg[rc]
			// Mirrors C Lua's luaT_getvararg (ltm.c):
			//   integer key → read from hidden vararg stack slots
			//   string "n"  → return number of extra args
			//   anything else → nil
			rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
			switch rc.Tt {
			case objectapi.TagInteger:
				idx := rc.Val.(int64)
				nExtra := ci.NExtraArgs
				if uint64(idx-1) < uint64(nExtra) {
					varBase := ci.Func - nExtra
					L.Stack[ra].Val = L.Stack[varBase+int(idx)-1].Val
				} else {
					L.Stack[ra].Val = objectapi.Nil
				}
			case objectapi.TagFloat:
				f := rc.Val.(float64)
				if idx, ok := FloatToInteger(f); ok {
					nExtra := ci.NExtraArgs
					if uint64(idx-1) < uint64(nExtra) {
						varBase := ci.Func - nExtra
						L.Stack[ra].Val = L.Stack[varBase+int(idx)-1].Val
					} else {
						L.Stack[ra].Val = objectapi.Nil
					}
				} else {
					L.Stack[ra].Val = objectapi.Nil
				}
			case objectapi.TagShortStr, objectapi.TagLongStr:
				s := rc.Val.(*objectapi.LuaString)
				if s.Data == "n" {
					L.Stack[ra].Val = objectapi.MakeInteger(int64(ci.NExtraArgs))
				} else {
					L.Stack[ra].Val = objectapi.Nil
				}
			default:
				L.Stack[ra].Val = objectapi.Nil
			}

		case opcodeapi.OP_ERRNNIL:
			// Error if value is not nil — used for global redefinition check
			if !L.Stack[ra].Val.IsNil() {
				bx := opcodeapi.GetArgBx(inst)
				if bx > 0 {
					// bx-1 is the constant index for the variable name
					name := k[bx-1]
					if name.IsString() {
						RunError(L, "global '"+name.StringVal().String()+"' already defined")
					} else {
						RunError(L, "global already defined")
					}
				} else {
					RunError(L, "global already defined")
				}
			}

		case opcodeapi.OP_EXTRAARG:
			// Should never be executed directly
			panic("OP_EXTRAARG should not be executed")

		default:
			RunError(L, "unknown opcode")
		}
		continue

	tforcall:
		// Generic for: call iterator
		L.Stack[ra+5].Val = L.Stack[ra+3].Val // copy control
		L.Stack[ra+4].Val = L.Stack[ra+1].Val // copy state
		L.Stack[ra+3].Val = L.Stack[ra].Val   // copy function
		L.Top = ra + 3 + 3
		{
			nr := opcodeapi.GetArgC(inst)
			Call(L, ra+3, nr)
		}
		L.Top = ci.Top // restore top
		// Next instruction should be TFORLOOP
		inst = code[ci.SavedPC]
		ci.SavedPC++
		ra = base + opcodeapi.GetArgA(inst)
		if !L.Stack[ra+3].Val.IsNil() {
			ci.SavedPC -= opcodeapi.GetArgBx(inst)
		}
		continue

	ret:
		if ci.CallStatus&stateapi.CISTFresh != 0 {
			return // end this frame
		}
		ci = L.CI
		// Correct 'oldpc' for the caller's frame after a return.
		// The callee overwrites L.OldPC with its own PCs during execution.
		// Set it to the caller's current SavedPC so the line hook won't
		// fire a spurious event for the same line as the CALL instruction.
		// Only done here (not in PosCall) because coroutine yield/resume
		// needs OldPC left alone to fire the correct line event on resume.
		// Mirrors: rethook in ldo.c (L->oldpc = pcRel(ci->u.l.savedpc, ...))
		if ci.IsLua() {
			L.OldPC = ci.SavedPC - 1
		}
		goto startfunc
	}
}
