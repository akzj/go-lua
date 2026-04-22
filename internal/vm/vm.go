// VM execution loop — the heart of the Lua interpreter.
//
// This is the Go equivalent of C's lvm.c. The core is execute(),
// a giant switch on opcodes that runs Lua bytecode.
//
// Reference: lua-master/lvm.c, .analysis/05-vm-execution-loop.md
package vm

import (
	"math"

	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/gc"
	"github.com/akzj/go-lua/internal/metamethod"
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/opcode"
	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/table"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const maxTagLoop = 2000

// ---------------------------------------------------------------------------
// Debt-based GC trigger
// ---------------------------------------------------------------------------

// checkGC triggers a GC step when GCDebt has been exhausted (≤ 0).
// GCDebt is decremented by TrackAllocation (called from LinkGC and table
// resize paths). After a collection, SetPause resets debt based on live data.
// Also maintains a counter-based safety net: even if debt hasn't run out,
// trigger GC every 5000 allocations to prevent Go-heap OOM when Lua's
// ObjSize estimates undercount actual Go memory usage.
func checkGC(g *state.GlobalState, L *state.LuaState) {
	if g.GCStepFn == nil {
		return
	}
	g.GCAllocCount++
	if g.GCDebt <= 0 || g.GCAllocCount%5000 == 0 {
		g.GCStepFn(L)
	}
}

// trackTableResize checks if a table has accumulated a size delta from
// resize operations and updates GCDebt accordingly. Call after any table
// mutation that might trigger a rehash (Set, SetInt, SetStr, ResizeArray).
func trackTableResize(g *state.GlobalState, t *table.Table) {
	if delta := t.SizeDelta; delta != 0 {
		t.SizeDelta = 0
		g.TrackAllocation(delta)
	}
}

// ---------------------------------------------------------------------------
// execute — the main VM execution loop
// ---------------------------------------------------------------------------

// finishOp finishes execution of an opcode interrupted by a yield.
// When a metamethod yields (via panic(LuaYield{})), intermediate Go frames
// (callTMRes, tryBinTM, callOrderTM) are destroyed. This function places
// the metamethod result into the correct register before execute resumes.
// Also handles __close interruption (OP_CLOSE/OP_RETURN).
// Mirrors: luaV_finishOp in lvm.c:568-618
func finishOp(L *state.LuaState, ci *state.CallInfo) {
	cl := L.Stack[ci.Func].Val.Obj.(*closure.LClosure)
	code := cl.Proto.Code
	inst := code[ci.SavedPC-1] // interrupted instruction
	op := opcode.GetOpCode(inst)
	base := ci.Func + 1
	switch op {

	// Category 1: Binary arithmetic metamethods
	// savedpc-1 = OP_MMBIN/MMBINI/MMBINK (the interrupted instruction)
	// savedpc-2 = OP_ADD/SUB/etc (the original arithmetic instruction)
	// Pop TM result from stack top → store in RA of original arith op
	case opcode.OP_MMBIN, opcode.OP_MMBINI, opcode.OP_MMBINK:
		L.Top--
		prevInst := code[ci.SavedPC-2]
		dest := base + opcode.GetArgA(prevInst)
		L.Stack[dest].Val = L.Stack[L.Top].Val

	// Category 2: Unary + Table Get
	// Pop TM result from stack top → store in RA of this instruction
	case opcode.OP_UNM, opcode.OP_BNOT, opcode.OP_LEN,
		opcode.OP_GETTABUP, opcode.OP_GETTABLE, opcode.OP_GETI,
		opcode.OP_GETFIELD, opcode.OP_SELF:
		L.Top--
		dest := base + opcode.GetArgA(inst)
		L.Stack[dest].Val = L.Stack[L.Top].Val

	// Category 3: Comparisons
	// Evaluate TM result as boolean, then conditional jump.
	// savedpc points to the OP_JMP instruction after the comparison.
	// If res != k, skip the jump (savedpc++).
	case opcode.OP_LT, opcode.OP_LE,
		opcode.OP_LTI, opcode.OP_LEI,
		opcode.OP_GTI, opcode.OP_GEI,
		opcode.OP_EQ:
		res := !L.Stack[L.Top-1].Val.IsFalsy()
		L.Top--
		if res != (opcode.GetArgK(inst) != 0) {
			ci.SavedPC++ // skip jump
		}

	// Category 4: Concat
	// Reposition TM result, adjust top, continue concat loop
	case opcode.OP_CONCAT:
		top := L.Top - 1
		a := opcode.GetArgA(inst)
		total := (top - 1) - (base + a)
		L.Stack[top-2].Val = L.Stack[top].Val // put TM result in proper position
		L.Top = top - 1
		if total > 1 {
			Concat(L, total) // concat remaining (may yield again)
		}

	// Category 5: Close/Return (already implemented)
	case opcode.OP_CLOSE:
		ci.SavedPC--
	case opcode.OP_RETURN:
		ra := base + opcode.GetArgA(inst)
		L.Top = ra + ci.NRes
		ci.SavedPC--

	default:
		// OP_TFORCALL, OP_CALL, OP_TAILCALL,
		// OP_SETTABUP, OP_SETTABLE, OP_SETI, OP_SETFIELD
		// No action needed — results already in correct place or no result needed
	}
}

// execute runs the VM main loop for the given CallInfo.
// This is the Go equivalent of luaV_execute in lvm.c.
func execute(L *state.LuaState, ci *state.CallInfo) {
startfunc:
	cl := L.Stack[ci.Func].Val.Obj.(*closure.LClosure)
	k := cl.Proto.Constants
	code := cl.Proto.Code
	base := ci.Func + 1

	// Mirrors: luaG_tracecall in ldebug.c — fire call hook at function entry.
	// For tail calls, preTailCall sets CISTTail and savedpc=0, then jumps here.
	// The call hook for non-tail calls is already fired by preCall (non-vararg)
	// or OP_VARARGPREP (vararg). Only tail calls need this path.
	// For vararg tail calls, defer to OP_VARARGPREP.
	if L.HookMask != 0 && ci.CallStatus&state.CISTTail != 0 &&
		ci.SavedPC == 0 && !cl.Proto.IsVararg() {
		callHook(L, ci)
	}

	for {
		inst := code[ci.SavedPC]
		ci.SavedPC++ // increment BEFORE hook check — mirrors C Lua vmfetch

		// Hook dispatch: fire line/count hooks if active.
		// Skip for OP_VARARGPREP — C Lua's luaG_tracecall returns 0 (trap=0)
		// for vararg functions, so traceexec is not called for instruction 0.
		// The call hook and OldPC adjustment happen inside OP_VARARGPREP instead.
		// Mirrors: vmfetch trap check in lvm.c + luaG_tracecall in ldebug.c
		if L.HookMask&(state.MaskLine|state.MaskCount) != 0 && L.AllowHook &&
			opcode.GetOpCode(inst) != opcode.OP_VARARGPREP {
			traceExec(L, ci)
		}
		op := opcode.GetOpCode(inst)
		ra := base + opcode.GetArgA(inst)

		switch op {

		// ===== Load/Move =====

		case opcode.OP_MOVE:
			rb := base + opcode.GetArgB(inst)
			L.Stack[ra].Val = L.Stack[rb].Val

		case opcode.OP_LOADI:
			L.Stack[ra].Val = object.MakeInteger(int64(opcode.GetArgSBx(inst)))

		case opcode.OP_LOADF:
			L.Stack[ra].Val = object.MakeFloat(float64(opcode.GetArgSBx(inst)))

		case opcode.OP_LOADK:
			L.Stack[ra].Val = k[opcode.GetArgBx(inst)]

		case opcode.OP_LOADKX:
			ax := opcode.GetArgAx(code[ci.SavedPC])
			ci.SavedPC++
			L.Stack[ra].Val = k[ax]

		case opcode.OP_LOADFALSE:
			L.Stack[ra].Val = object.False

		case opcode.OP_LFALSESKIP:
			L.Stack[ra].Val = object.False
			ci.SavedPC++ // skip next instruction

		case opcode.OP_LOADTRUE:
			L.Stack[ra].Val = object.True

		case opcode.OP_LOADNIL:
			b := opcode.GetArgB(inst)
			for i := 0; i <= b; i++ {
				L.Stack[ra+i].Val = object.Nil
			}

		// ===== Upvalues =====

		case opcode.OP_GETUPVAL:
			b := opcode.GetArgB(inst)
			L.Stack[ra].Val = cl.UpVals[b].Get(L.Stack)

		case opcode.OP_SETUPVAL:
			b := opcode.GetArgB(inst)
			uv := cl.UpVals[b]
			uv.Set(L.Stack, L.Stack[ra].Val)
			gc.BarrierValue(L.Global, uv, L.Stack[ra].Val) // GC write barrier: upvalue set

		case opcode.OP_CLOSE:
			gc.CloseUpvals(L.Global, L, ra) // barrier-aware close
			closeTBC(L, ra)

		case opcode.OP_TBC:
			// To-be-closed: mark the variable in the TBC linked list
			markTBC(L, ra)

		// ===== Table access =====

		case opcode.OP_GETTABUP:
			b := opcode.GetArgB(inst)
			upval := cl.UpVals[b].Get(L.Stack)
			rc := k[opcode.GetArgC(inst)]
			if upval.IsTable() {
				h := upval.Obj.(*table.Table)
				val, found := h.Get(rc)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, upval, rc, ra)
				}
			} else {
				FinishGet(L, upval, rc, ra)
			}

		case opcode.OP_GETTABLE:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			if rb.IsTable() {
				h := rb.Obj.(*table.Table)
				val, found := h.Get(rc)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, rb, rc, ra)
				}
			} else {
				FinishGet(L, rb, rc, ra)
			}

		case opcode.OP_GETI:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			c := int64(opcode.GetArgC(inst))
			if rb.IsTable() {
				h := rb.Obj.(*table.Table)
				val, found := h.GetInt(c)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, rb, object.MakeInteger(c), ra)
				}
			} else {
				FinishGet(L, rb, object.MakeInteger(c), ra)
			}

		case opcode.OP_GETFIELD:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := k[opcode.GetArgC(inst)]
			if rb.IsTable() {
				h := rb.Obj.(*table.Table)
				val, found := h.Get(rc)
				if found && !val.IsNil() {
					L.Stack[ra].Val = val
				} else {
					FinishGet(L, rb, rc, ra)
				}
			} else {
				FinishGet(L, rb, rc, ra)
			}

		case opcode.OP_SETTABUP:
			b := opcode.GetArgB(inst)
			upval := cl.UpVals[opcode.GetArgA(inst)].Get(L.Stack)
			rb := k[b]
			var rc object.TValue
			if opcode.GetArgK(inst) != 0 {
				rc = k[opcode.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcode.GetArgC(inst)].Val
			}
			ra = 0 // OP_SETTABUP uses A for upvalue index, not register
			if upval.IsTable() {
				tableSetWithMeta(L, upval, rb, rc)
			} else {
				FinishSet(L, upval, rb, rc)
			}

		case opcode.OP_SETTABLE:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			var rc object.TValue
			if opcode.GetArgK(inst) != 0 {
				rc = k[opcode.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcode.GetArgC(inst)].Val
			}
			tval := L.Stack[ra].Val
			if tval.IsTable() {
				tableSetWithMeta(L, tval, rb, rc)
			} else {
				FinishSet(L, tval, rb, rc)
			}

		case opcode.OP_SETI:
			b := int64(opcode.GetArgB(inst))
			var rc object.TValue
			if opcode.GetArgK(inst) != 0 {
				rc = k[opcode.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcode.GetArgC(inst)].Val
			}
			tval := L.Stack[ra].Val
			if tval.IsTable() {
				tableSetWithMeta(L, tval, object.MakeInteger(b), rc)
			} else {
				FinishSet(L, tval, object.MakeInteger(b), rc)
			}

		case opcode.OP_SETFIELD:
			rb := k[opcode.GetArgB(inst)]
			var rc object.TValue
			if opcode.GetArgK(inst) != 0 {
				rc = k[opcode.GetArgC(inst)]
			} else {
				rc = L.Stack[base+opcode.GetArgC(inst)].Val
			}
			tval := L.Stack[ra].Val
			if tval.IsTable() {
				tableSetWithMeta(L, tval, rb, rc)
			} else {
				FinishSet(L, tval, rb, rc)
			}

		case opcode.OP_NEWTABLE:
			b := opcode.GetArgVB(inst)
			c := opcode.GetArgVC(inst)
			if b > 0 {
				b = 1 << (b - 1)
			}
			if opcode.GetArgK(inst) != 0 {
				c += opcode.GetArgAx(code[ci.SavedPC]) * (opcode.MaxArgVC + 1)
			}
			ci.SavedPC++ // skip extra arg
			t := table.New(c, b)
			L.Global.LinkGC(t) // V5: register in allgc chain
			size := t.EstimateBytes()
			t.GCHeader.ObjSize = size
			L.Global.GCTotalBytes += size
			// V5 GC sweep handles dealloc accounting — no AddCleanup needed
			L.Stack[ra].Val = object.TValue{Tt: object.TagTable, Obj: t}

			// Periodic GC: run Lua GC during tight allocation loops.
			// V5 GC handles __gc via finobj list.
			if g := L.Global; !g.GCStopped {
				checkGC(g, L)
			}

		case opcode.OP_SELF:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := k[opcode.GetArgC(inst)]
			L.Stack[ra+1].Val = rb // save table as self
			if rb.IsTable() {
				h := rb.Obj.(*table.Table)
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

		case opcode.OP_ADDI:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			ic := int64(opcode.GetArgSC(inst))
			if rb.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(rb.Integer() + ic)
				ci.SavedPC++ // skip MMBIN
			} else if rb.IsFloat() {
				L.Stack[ra].Val = object.MakeFloat(rb.Float() + float64(ic))
				ci.SavedPC++
			}
			// else: fall through to MMBINI on next instruction

		// ===== Arithmetic with constant =====

		case opcode.OP_ADDK:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			kc := k[opcode.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(rb.Integer() + kc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a + b }, func(a, b float64) float64 { return a + b })
					ci.SavedPC++
				}
			}

		case opcode.OP_SUBK:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			kc := k[opcode.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(rb.Integer() - kc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a - b }, func(a, b float64) float64 { return a - b })
					ci.SavedPC++
				}
			}

		case opcode.OP_MULK:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			kc := k[opcode.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(rb.Integer() * kc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a * b }, func(a, b float64) float64 { return a * b })
					ci.SavedPC++
				}
			}

		case opcode.OP_MODK:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			kc := k[opcode.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(iMod(L, rb.Integer(), kc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = object.MakeInteger(iMod(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = object.MakeFloat(fMod(toFloat(nb), toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		case opcode.OP_POWK:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			kc := k[opcode.GetArgC(inst)]
			nb, ok1 := toNumberNSFloat(rb)
			nc, ok2 := toNumberNSFloat(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeFloat(math.Pow(nb, nc))
				ci.SavedPC++
			}

		case opcode.OP_DIVK:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			kc := k[opcode.GetArgC(inst)]
			nb, ok1 := toNumberNSFloat(rb)
			nc, ok2 := toNumberNSFloat(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeFloat(nb / nc)
				ci.SavedPC++
			}

		case opcode.OP_IDIVK:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			kc := k[opcode.GetArgC(inst)]
			if rb.IsInteger() && kc.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(iDiv(L, rb.Integer(), kc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(kc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = object.MakeInteger(iDiv(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = object.MakeFloat(math.Floor(toFloat(nb) / toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		// ===== Bitwise with constant =====

		case opcode.OP_BANDK:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			kc := k[opcode.GetArgC(inst)]
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeInteger(ib & ic)
				ci.SavedPC++
			}

		case opcode.OP_BORK:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			kc := k[opcode.GetArgC(inst)]
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeInteger(ib | ic)
				ci.SavedPC++
			}

		case opcode.OP_BXORK:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			kc := k[opcode.GetArgC(inst)]
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(kc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeInteger(ib ^ ic)
				ci.SavedPC++
			}

		case opcode.OP_SHLI:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			ic := int64(opcode.GetArgSC(inst))
			ib, ok := toIntegerStrict(rb)
			if ok {
				L.Stack[ra].Val = object.MakeInteger(shiftL(ic, ib))
				ci.SavedPC++
			}

		case opcode.OP_SHRI:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			ic := int64(opcode.GetArgSC(inst))
			ib, ok := toIntegerStrict(rb)
			if ok {
				L.Stack[ra].Val = object.MakeInteger(shiftL(ib, -ic))
				ci.SavedPC++
			}

		// ===== Arithmetic register-register =====

		case opcode.OP_ADD:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(rb.Integer() + rc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a + b }, func(a, b float64) float64 { return a + b })
					ci.SavedPC++
				}
			}

		case opcode.OP_SUB:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(rb.Integer() - rc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a - b }, func(a, b float64) float64 { return a - b })
					ci.SavedPC++
				}
			}

		case opcode.OP_MUL:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(rb.Integer() * rc.Integer())
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					L.Stack[ra].Val = arithBinTV(nb, nc, func(a, b int64) int64 { return a * b }, func(a, b float64) float64 { return a * b })
					ci.SavedPC++
				}
			}

		case opcode.OP_MOD:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(iMod(L, rb.Integer(), rc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = object.MakeInteger(iMod(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = object.MakeFloat(fMod(toFloat(nb), toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		case opcode.OP_POW:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			nb, ok1 := toNumberNSFloat(rb)
			nc, ok2 := toNumberNSFloat(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeFloat(math.Pow(nb, nc))
				ci.SavedPC++
			}

		case opcode.OP_DIV:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			nb, ok1 := toNumberNSFloat(rb)
			nc, ok2 := toNumberNSFloat(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeFloat(nb / nc)
				ci.SavedPC++
			}

		case opcode.OP_IDIV:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			if rb.IsInteger() && rc.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(iDiv(L, rb.Integer(), rc.Integer()))
				ci.SavedPC++
			} else {
				nb, ok1 := toNumberNS(rb)
				nc, ok2 := toNumberNS(rc)
				if ok1 && ok2 {
					if nb.IsInteger() && nc.IsInteger() {
						L.Stack[ra].Val = object.MakeInteger(iDiv(L, nb.Integer(), nc.Integer()))
					} else {
						L.Stack[ra].Val = object.MakeFloat(math.Floor(toFloat(nb) / toFloat(nc)))
					}
					ci.SavedPC++
				}
			}

		// ===== Bitwise register-register =====

		case opcode.OP_BAND:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeInteger(ib & ic)
				ci.SavedPC++
			}

		case opcode.OP_BOR:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeInteger(ib | ic)
				ci.SavedPC++
			}

		case opcode.OP_BXOR:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeInteger(ib ^ ic)
				ci.SavedPC++
			}

		case opcode.OP_SHL:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeInteger(shiftL(ib, ic))
				ci.SavedPC++
			}

		case opcode.OP_SHR:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			ib, ok1 := toIntegerStrict(rb)
			ic, ok2 := toIntegerStrict(rc)
			if ok1 && ok2 {
				L.Stack[ra].Val = object.MakeInteger(shiftL(ib, -ic))
				ci.SavedPC++
			}

		// ===== Metamethod fallback =====

		case opcode.OP_MMBIN:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			tm := metamethod.TMS(opcode.GetArgC(inst))
			prevInst := code[ci.SavedPC-2]
			result := base + opcode.GetArgA(prevInst)
			tryBinTM(L, L.Stack[ra].Val, rb, result, tm, ra-base, opcode.GetArgB(inst))

		case opcode.OP_MMBINI:
			imm := opcode.GetArgSB(inst)
			tm := metamethod.TMS(opcode.GetArgC(inst))
			flip := opcode.GetArgK(inst) != 0
			prevInst := code[ci.SavedPC-2]
			result := base + opcode.GetArgA(prevInst)
			tryBiniTM(L, L.Stack[ra].Val, imm, flip, result, tm, ra-base)

		case opcode.OP_MMBINK:
			imm := k[opcode.GetArgB(inst)]
			tm := metamethod.TMS(opcode.GetArgC(inst))
			flip := opcode.GetArgK(inst) != 0
			prevInst := code[ci.SavedPC-2]
			result := base + opcode.GetArgA(prevInst)
			tryBinKTM(L, L.Stack[ra].Val, imm, flip, result, tm, ra-base)

		// ===== Unary =====

		case opcode.OP_UNM:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			if rb.IsInteger() {
				L.Stack[ra].Val = object.MakeInteger(-rb.Integer())
			} else if rb.IsFloat() {
				L.Stack[ra].Val = object.MakeFloat(-rb.Float())
			} else {
				tryBinTM(L, rb, rb, ra, metamethod.TM_UNM, opcode.GetArgB(inst), opcode.GetArgB(inst))
			}

		case opcode.OP_BNOT:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			ib, ok := toIntegerStrict(rb)
			if ok {
				L.Stack[ra].Val = object.MakeInteger(^ib)
			} else {
				tryBinTM(L, rb, rb, ra, metamethod.TM_BNOT, opcode.GetArgB(inst), opcode.GetArgB(inst))
			}

		case opcode.OP_NOT:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			if rb.IsFalsy() {
				L.Stack[ra].Val = object.True
			} else {
				L.Stack[ra].Val = object.False
			}

		case opcode.OP_LEN:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			ObjLen(L, ra, rb)

		case opcode.OP_CONCAT:
			n := opcode.GetArgB(inst)
			L.Top = ra + n
			Concat(L, n)
			L.Stack[ra].Val = L.Stack[L.Top-1].Val
			L.Top = ci.Top // restore top

			// Periodic GC: string concatenation allocates new strings.
			if g := L.Global; !g.GCStopped {
				checkGC(g, L)
			}

		// ===== Comparison =====

		case opcode.OP_EQ:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			// Bump L.Top to frame top so callTMRes scratch space
			// doesn't clobber live registers (metamethod dispatch).
			L.Top = ci.Top
			cond := EqualObj(L, L.Stack[ra].Val, rb)
			if cond != (opcode.GetArgK(inst) != 0) {
				ci.SavedPC++ // skip jump
			} else {
				ci.SavedPC += opcode.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcode.OP_LT:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			L.Top = ci.Top
			cond := LessThan(L, L.Stack[ra].Val, rb)
			if cond != (opcode.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcode.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcode.OP_LE:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			L.Top = ci.Top
			cond := LessEqual(L, L.Stack[ra].Val, rb)
			if cond != (opcode.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcode.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcode.OP_EQK:
			rb := k[opcode.GetArgB(inst)]
			cond := rawEqualObj(L.Stack[ra].Val, rb)
			if cond != (opcode.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcode.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcode.OP_EQI:
			im := int64(opcode.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() == im
			} else if v.IsFloat() {
				cond = v.Float() == float64(im)
			}
			if cond != (opcode.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcode.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcode.OP_LTI:
			im := int64(opcode.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() < im
			} else if v.IsFloat() {
				cond = v.Float() < float64(im)
			} else {
				// Metamethod fallback: callOrderITM(L, v, im, false, isf, TM_LT)
				isf := opcode.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, false, isf, metamethod.TM_LT)
			}
			if cond != (opcode.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcode.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcode.OP_LEI:
			im := int64(opcode.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() <= im
			} else if v.IsFloat() {
				cond = v.Float() <= float64(im)
			} else {
				isf := opcode.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, false, isf, metamethod.TM_LE)
			}
			if cond != (opcode.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcode.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcode.OP_GTI:
			im := int64(opcode.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() > im
			} else if v.IsFloat() {
				cond = v.Float() > float64(im)
			} else {
				// GTI: a > im ⟺ im < a, so flip=true, TM_LT
				isf := opcode.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, true, isf, metamethod.TM_LT)
			}
			if cond != (opcode.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcode.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcode.OP_GEI:
			im := int64(opcode.GetArgSB(inst))
			v := L.Stack[ra].Val
			var cond bool
			if v.IsInteger() {
				cond = v.Integer() >= im
			} else if v.IsFloat() {
				cond = v.Float() >= float64(im)
			} else {
				// GEI: a >= im ⟺ im <= a, so flip=true, TM_LE
				isf := opcode.GetArgC(inst) != 0
				L.Top = ci.Top
				cond = callOrderITM(L, v, im, true, isf, metamethod.TM_LE)
			}
			if cond != (opcode.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcode.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcode.OP_TEST:
			cond := !L.Stack[ra].Val.IsFalsy()
			if cond != (opcode.GetArgK(inst) != 0) {
				ci.SavedPC++
			} else {
				ci.SavedPC += opcode.GetArgSJ(code[ci.SavedPC]) + 1
			}

		case opcode.OP_TESTSET:
			rb := L.Stack[base+opcode.GetArgB(inst)].Val
			if rb.IsFalsy() == (opcode.GetArgK(inst) != 0) {
				ci.SavedPC++ // condition failed, skip jump
			} else {
				L.Stack[ra].Val = rb
				ci.SavedPC += opcode.GetArgSJ(code[ci.SavedPC]) + 1
			}

		// ===== Jump =====

		case opcode.OP_JMP:
			ci.SavedPC += opcode.GetArgSJ(inst)

		// ===== For loops =====

		case opcode.OP_FORPREP:
			if forPrep(L, ra) {
				ci.SavedPC += opcode.GetArgBx(inst) + 1 // skip loop
			}

		case opcode.OP_FORLOOP:
			if forLoop(L, ra) {
				ci.SavedPC -= opcode.GetArgBx(inst) // jump back
			}

		case opcode.OP_TFORPREP:
			// Swap control and closing variables
			// Before: ra=iter, ra+1=state, ra+2=control, ra+3=closing
			// After:  ra=iter, ra+1=state, ra+2=closing(tbc), ra+3=control
			temp := L.Stack[ra+3].Val
			L.Stack[ra+3].Val = L.Stack[ra+2].Val
			L.Stack[ra+2].Val = temp
			// Mark the closing variable (now at ra+2) as to-be-closed
			// C Lua: halfProtect(luaF_newtbcupval(L, ra + 2))
			markTBC(L, ra+2)
			ci.SavedPC += opcode.GetArgBx(inst)
			// Fall through to TFORCALL
			inst = code[ci.SavedPC]
			ci.SavedPC++
			ra = base + opcode.GetArgA(inst)
			goto tforcall

		case opcode.OP_TFORCALL:
			goto tforcall

		case opcode.OP_TFORLOOP:
			if !L.Stack[ra+3].Val.IsNil() {
				ci.SavedPC -= opcode.GetArgBx(inst) // jump back
			}

		// ===== Call/Return =====

		case opcode.OP_CALL:
			b := opcode.GetArgB(inst)
			nresults := opcode.GetArgC(inst) - 1
			if b != 0 {
				L.Top = ra + b
			}
			// SavedPC already set by dispatch loop
			newci := preCall(L, ra, nresults)
			if newci != nil {
				ci = newci
				goto startfunc
			}
			// C function already executed

		case opcode.OP_TAILCALL:
			b := opcode.GetArgB(inst)
			nparams1 := opcode.GetArgC(inst)
			delta := 0
			if nparams1 != 0 {
				delta = ci.NExtraArgs + nparams1
			}
			if b != 0 {
				L.Top = ra + b
			} else {
				b = L.Top - ra
			}
			if opcode.GetArgK(inst) != 0 {
				gc.CloseUpvals(L.Global, L, base) // barrier-aware close
			}
			n := preTailCall(L, ci, ra, b, delta)
			if n < 0 {
				// Lua function — ci.Func already adjusted by delta
				goto startfunc
			}
			// C function executed — restore func and finish
			ci.Func -= delta
			posCall(L, ci, n)
			goto ret

		case opcode.OP_RETURN:
			b := opcode.GetArgB(inst)
			n := b - 1
			nparams1 := opcode.GetArgC(inst)
			if n < 0 {
				n = L.Top - ra
			}
			if opcode.GetArgK(inst) != 0 {
				// C Lua: save nres, ensure stack space, close upvals+TBC, refresh base/ra
				// Use StatusCloseKTop so callCloseMethod does NOT reset L.Top —
				// return values sit on the stack above the TBC variables.
				ci.NRes = n
				if L.Top < ci.Top {
					L.Top = ci.Top
				}
				gc.CloseUpvals(L.Global, L, base) // barrier-aware close
				closeTBCWithError(L, base, state.StatusCloseKTop, object.Nil, true)
				// After close, stack may have been reallocated by __close calls.
				// Refresh base and ra from ci (which uses offsets, not pointers).
				base = ci.Func + 1
				ra = base + opcode.GetArgA(inst)
			}
			if nparams1 != 0 {
				ci.Func -= ci.NExtraArgs + nparams1
			}
			L.Top = ra + n
			posCall(L, ci, n)
			goto ret

		case opcode.OP_RETURN0:
			if L.HookMask != 0 {
				// Hooks active — fall back to full posCall (fires return hook)
				L.Top = ra
				posCall(L, ci, 0)
			} else {
				// Fast path — no hooks
				nres := ci.NResults()
				L.CI = ci.Prev
				L.Top = base - 1
				for i := 0; i < nres; i++ {
					L.Stack[L.Top].Val = object.Nil
					L.Top++
				}
				if nres < 0 {
					L.Top = base - 1
				}
			}
			goto ret

		case opcode.OP_RETURN1:
			if L.HookMask != 0 {
				// Hooks active — fall back to full posCall (fires return hook)
				L.Top = ra + 1
				posCall(L, ci, 1)
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
						L.Stack[L.Top].Val = object.Nil
						L.Top++
					}
				}
			}
			goto ret

		// ===== Closure/Vararg =====

		case opcode.OP_CLOSURE:
			bx := opcode.GetArgBx(inst)
			p := cl.Proto.Protos[bx]
			pushClosure(L, p, cl.UpVals, base, ra)

			// Periodic GC: closures are heap-allocated objects.
			if g := L.Global; !g.GCStopped {
				checkGC(g, L)
			}

		case opcode.OP_VARARG:
			n := opcode.GetArgC(inst) - 1
			vatab := -1
			if opcode.GetArgK(inst) != 0 {
				vatab = opcode.GetArgB(inst)
			}
			getVarargs(L, ci, ra, n, vatab)

		case opcode.OP_VARARGPREP:
			adjustVarargs(L, ci, cl.Proto)
			// Update base after adjustment
			base = ci.Func + 1
			// Fire call hook AFTER adjustment (deferred from preCall).
			// Mirrors: OP_VARARGPREP in lvm.c calls luaD_hookcall after
			// luaT_adjustvarargs, so debug.getlocal sees correct params.
			if L.HookMask != 0 {
				callHook(L, ci)
				L.OldPC = 1 // next opcode seen as "new" line
			}

		// ===== Table construction =====

		case opcode.OP_SETLIST:
			n := opcode.GetArgVB(inst)
			last := opcode.GetArgVC(inst)
			h := L.Stack[ra].Val.Obj.(*table.Table)
			if n == 0 {
				n = L.Top - ra - 1
			} else {
				L.Top = ci.Top // correct top in case of emergency GC
			}
			last += n
			if opcode.GetArgK(inst) != 0 {
				last += opcode.GetArgAx(code[ci.SavedPC]) * (opcode.MaxArgVC + 1)
				ci.SavedPC++
			}
			// Match C Lua: when 'last' exceeds current array size,
			// pre-allocate the array to exactly 'last' (not power-of-2).
			// This avoids rehash rounding up to the next power of 2.
			if last > h.ArrayLen() {
				h.ResizeArray(last)
			}
			for i := n; i > 0; i-- {
				h.SetInt(int64(last), L.Stack[ra+i].Val)
				last--
			}
			trackTableResize(L.Global, h)  // track resize delta for GC debt
			gc.BarrierBack(L.Global, h)    // GC write barrier: table bulk-set
			L.Top = ci.Top              // restore top

		// ===== Lua 5.5 new opcodes =====

		case opcode.OP_GETVARG:
			// OP_GETVARG: ra = vararg[rc]
			// Mirrors C Lua's luaT_getvararg (ltm.c):
			//   integer key → read from hidden vararg stack slots
			//   string "n"  → return number of extra args
			//   anything else → nil
			rc := L.Stack[base+opcode.GetArgC(inst)].Val
			switch rc.Tt {
			case object.TagInteger:
				idx := rc.N
				nExtra := ci.NExtraArgs
				if uint64(idx-1) < uint64(nExtra) {
					varBase := ci.Func - nExtra
					L.Stack[ra].Val = L.Stack[varBase+int(idx)-1].Val
				} else {
					L.Stack[ra].Val = object.Nil
				}
			case object.TagFloat:
				f := rc.Float()
				if idx, ok := floatToInteger(f); ok {
					nExtra := ci.NExtraArgs
					if uint64(idx-1) < uint64(nExtra) {
						varBase := ci.Func - nExtra
						L.Stack[ra].Val = L.Stack[varBase+int(idx)-1].Val
					} else {
						L.Stack[ra].Val = object.Nil
					}
				} else {
					L.Stack[ra].Val = object.Nil
				}
			case object.TagShortStr, object.TagLongStr:
				s := rc.Obj.(*object.LuaString)
				if s.Data == "n" {
					L.Stack[ra].Val = object.MakeInteger(int64(ci.NExtraArgs))
				} else {
					L.Stack[ra].Val = object.Nil
				}
			default:
				L.Stack[ra].Val = object.Nil
			}

		case opcode.OP_ERRNNIL:
			// Error if value is not nil — used for global redefinition check
			if !L.Stack[ra].Val.IsNil() {
				bx := opcode.GetArgBx(inst)
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

		case opcode.OP_EXTRAARG:
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
			nr := opcode.GetArgC(inst)
			Call(L, ra+3, nr)
		}
		L.Top = ci.Top // restore top
		// Next instruction should be TFORLOOP
		inst = code[ci.SavedPC]
		ci.SavedPC++
		ra = base + opcode.GetArgA(inst)
		if !L.Stack[ra+3].Val.IsNil() {
			ci.SavedPC -= opcode.GetArgBx(inst)
		}
		continue

	ret:
		if ci.CallStatus&state.CISTFresh != 0 {
			return // end this frame
		}
		ci = L.CI
		// Correct 'oldpc' for the caller's frame after a return.
		// The callee overwrites L.OldPC with its own PCs during execution.
		// Set it to the caller's current SavedPC so the line hook won't
		// fire a spurious event for the same line as the CALL instruction.
		// Only done here (not in posCall) because coroutine yield/resume
		// needs OldPC left alone to fire the correct line event on resume.
		// Mirrors: rethook in ldo.c (L->oldpc = pcRel(ci->u.l.savedpc, ...))
		if ci.IsLua() {
			L.OldPC = ci.SavedPC - 1
		}
		goto startfunc
	}
}
