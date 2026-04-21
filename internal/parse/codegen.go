// Code generator for Lua — translates expression descriptors into bytecode.
//
// This is the Go translation of C Lua's lcode.c (1972 lines).
// It handles instruction emission, constant pool management, register
// allocation, jump list management, expression discharge, and operator codegen.
//
// Reference: lua-master/lcode.c, .analysis/06-compiler-pipeline.md §4
package parse

import (
	"fmt"
	"math"

	"github.com/akzj/go-lua/internal/lex"
	"github.com/akzj/go-lua/internal/metamethod"
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/opcode"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	// maxIndexRK is the maximum K index that fits in an R/K operand.
	maxIndexRK = opcode.MaxArgB // 255

	// maxFStack is the maximum register index (= MaxArgA = 255).
	maxFStack = opcode.MaxArgA

	// limLineDiff is the limit for relative line info differences.
	limLineDiff = 0x80

	// maxIWthAbs is the max instructions between absolute line info entries.
	maxIWthAbs = 128

	// absLineInfo is the sentinel value in lineinfo signaling absolute info.
	absLineInfo = int8(-0x80) // -128

	// luaMultRet is the multi-return sentinel (LUA_MULTRET = -1).
	luaMultRet = -1
)

// ---------------------------------------------------------------------------
// Helper: get the Dyndata from the LexState
// ---------------------------------------------------------------------------

func getDyndata(fs *funcState) *dyndata {
	return fs.Lex.DynData.(*dyndata)
}

// ---------------------------------------------------------------------------
// Helper: NVarStack — number of variables in the stack for the current function
// Mirrors: luaY_nvarstack in lparser.c
// ---------------------------------------------------------------------------

// nVarStack returns the number of stack slots used by active local variables.
// Mirrors: luaY_nvarstack in lparser.c — calls reglevel to skip globals/constants.
func nVarStack(fs *funcState) int {
	return int(regLevel(fs, fs.NumActVar))
}

// ---------------------------------------------------------------------------
// Helper: CheckLimit — check a value against a limit, error if exceeded
// Mirrors: luaY_checklimit in lparser.c
// ---------------------------------------------------------------------------

func checkLimit(fs *funcState, v, lim int, what string) {
	if v > lim {
		line := fs.Proto.LineDefined
		var where string
		if line == 0 {
			where = "main function"
		} else {
			where = fmt.Sprintf("function at line %d", line)
		}
		msg := fmt.Sprintf("too many %s (limit is %d) in %s", what, lim, where)
		throwSyntaxError(fs, msg)
	}
}

// throwSyntaxError raises a syntax error via the lexer.
func throwSyntaxError(fs *funcState, msg string) {
	ls := fs.Lex
	ls.Token.Type = 0 // remove "near <token>" from message
	ls.Line = ls.LastLine
	lex.LexError(ls, msg, 0)
}

// ---------------------------------------------------------------------------
// SemError — semantic error (C: luaK_semerror)
// ---------------------------------------------------------------------------

// semError raises a semantic error.
func semError(ls *lex.LexState, msg string) {
	ls.Token.Type = 0
	ls.Line = ls.LastLine
	lex.LexError(ls, msg, 0)
}

// ---------------------------------------------------------------------------
// getInstruction — get instruction referenced by an ExpDesc
// ---------------------------------------------------------------------------

func getInstruction(fs *funcState, e *expDesc) *uint32 {
	return &fs.Proto.Code[e.Info]
}

// ---------------------------------------------------------------------------
// Line info
// ---------------------------------------------------------------------------

// saveLineInfo saves line info for the instruction at fs.PC-1.
// Mirrors: savelineinfo in lcode.c
func saveLineInfo(fs *funcState, line int) {
	f := fs.Proto
	linedif := line - fs.PrevLine
	pc := fs.PC - 1

	if abs(linedif) >= limLineDiff || int(fs.IWthAbs) >= maxIWthAbs {
		// Need absolute line info
		f.AbsLineInfo = append(f.AbsLineInfo, object.AbsLineInfo{PC: pc, Line: line})
		linedif = int(absLineInfo)
		fs.IWthAbs = 1
	} else {
		fs.IWthAbs++
	}

	// Grow lineinfo if needed
	for len(f.LineInfo) <= pc {
		f.LineInfo = append(f.LineInfo, 0)
	}
	f.LineInfo[pc] = int8(linedif)
	fs.PrevLine = line
}

// removeLastLineInfo removes line info for the last instruction.
func removeLastLineInfo(fs *funcState) {
	f := fs.Proto
	pc := fs.PC - 1
	if f.LineInfo[pc] != absLineInfo {
		fs.PrevLine -= int(f.LineInfo[pc])
		fs.IWthAbs--
	} else {
		// Absolute line info
		fs.Proto.AbsLineInfo = fs.Proto.AbsLineInfo[:len(fs.Proto.AbsLineInfo)-1]
		fs.IWthAbs = maxIWthAbs + 1 // force next to be absolute
	}
}

// removeLastInstruction removes the last instruction and its line info.
func removeLastInstruction(fs *funcState) {
	removeLastLineInfo(fs)
	fs.PC--
}

// fixLine changes the line info for the last instruction.
func fixLine(fs *funcState, line int) {
	removeLastLineInfo(fs)
	saveLineInfo(fs, line)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ---------------------------------------------------------------------------
// previousInstruction — get the previous instruction safely
// ---------------------------------------------------------------------------

// invalidInstruction is returned when there may be a jump target between
// the current and previous instruction.
var invalidInstruction uint32 = ^uint32(0)

func previousInstruction(fs *funcState) *uint32 {
	if fs.PC > fs.LastTarget {
		return &fs.Proto.Code[fs.PC-1]
	}
	return &invalidInstruction
}

// ---------------------------------------------------------------------------
// Instruction emission
// ---------------------------------------------------------------------------

// codeInstr emits an instruction and saves line info. Returns the instruction index.
// Mirrors: luaK_code
func codeInstr(fs *funcState, i uint32) int {
	f := fs.Proto
	// Grow code array if needed
	for len(f.Code) <= fs.PC {
		f.Code = append(f.Code, 0)
	}
	f.Code[fs.PC] = i
	fs.PC++
	saveLineInfo(fs, fs.Lex.LastLine)
	return fs.PC - 1
}

// codeABCk emits an iABC instruction. Mirrors: luaK_codeABCk
func codeABCk(fs *funcState, op opcode.OpCode, a, b, c, k int) int {
	return codeInstr(fs, opcode.CreateABCK(op, a, b, c, k))
}

// codeABC emits an iABC instruction with k=0.
func codeABC(fs *funcState, op opcode.OpCode, a, b, c int) int {
	return codeABCk(fs, op, a, b, c, 0)
}

// codeVABCk emits an ivABC instruction. Mirrors: luaK_codevABCk
func codeVABCk(fs *funcState, op opcode.OpCode, a, vb, vc, k int) int {
	return codeInstr(fs, opcode.CreateVABCK(op, a, vb, vc, k))
}

// codeABx emits an iABx instruction. Mirrors: luaK_codeABx
func codeABx(fs *funcState, op opcode.OpCode, a, bx int) int {
	return codeInstr(fs, opcode.CreateABx(op, a, bx))
}

// codeAsBx emits an iAsBx instruction.
func codeAsBx(fs *funcState, op opcode.OpCode, a, sbx int) int {
	return codeInstr(fs, opcode.CreateAsBx(op, a, sbx))
}

// codesJ emits an isJ instruction with k bit. Mirrors: codesJ in lcode.c
func codesJ(fs *funcState, op opcode.OpCode, sj, k int) int {
	return codeInstr(fs, opcode.CreateSJK(op, sj, k))
}

// codeExtraArg emits an EXTRAARG instruction.
func codeExtraArg(fs *funcState, a int) int {
	return codeInstr(fs, opcode.CreateAx(opcode.OP_EXTRAARG, a))
}

// codek emits a LOADK or LOADKX instruction.
func codek(fs *funcState, reg, k int) int {
	if k <= opcode.MaxArgBx {
		return codeABx(fs, opcode.OP_LOADK, reg, k)
	}
	p := codeABx(fs, opcode.OP_LOADKX, reg, 0)
	codeExtraArg(fs, k)
	return p
}

// ---------------------------------------------------------------------------
// Register management
// ---------------------------------------------------------------------------

// checkStack checks register-stack level. Mirrors: luaK_checkstack
func checkStack(fs *funcState, n int) {
	newstack := int(fs.FreeReg) + n
	if newstack > int(fs.Proto.MaxStackSize) {
		checkLimit(fs, newstack, maxFStack, "registers")
		fs.Proto.MaxStackSize = byte(newstack)
	}
}

// reserveRegs reserves n registers. Mirrors: luaK_reserveregs
func reserveRegs(fs *funcState, n int) {
	checkStack(fs, n)
	fs.FreeReg += byte(n)
}

// freeReg frees a register if it's not a local variable.
func freeReg(fs *funcState, reg int) {
	if reg >= nVarStack(fs) {
		fs.FreeReg--
	}
}

// freeRegs frees two registers in proper order.
func freeRegs(fs *funcState, r1, r2 int) {
	if r1 > r2 {
		freeReg(fs, r1)
		freeReg(fs, r2)
	} else {
		freeReg(fs, r2)
		freeReg(fs, r1)
	}
}

// freeExp frees the register used by an expression (if any).
func freeExp(fs *funcState, e *expDesc) {
	if e.Kind == vNONRELOC {
		freeReg(fs, e.Info)
	}
}

// freeExps frees registers used by two expressions.
func freeExps(fs *funcState, e1, e2 *expDesc) {
	r1 := -1
	if e1.Kind == vNONRELOC {
		r1 = e1.Info
	}
	r2 := -1
	if e2.Kind == vNONRELOC {
		r2 = e2.Info
	}
	freeRegs(fs, r1, r2)
}

// ---------------------------------------------------------------------------
// Constant pool management
// ---------------------------------------------------------------------------

// addK adds a constant value to the Proto's constant list.
func addK(fs *funcState, v object.TValue) int {
	f := fs.Proto
	k := len(f.Constants)
	f.Constants = append(f.Constants, v)
	return k
}

// k2proto adds a constant with dedup via KCache.
func k2proto(fs *funcState, key any, v object.TValue) int {
	if idx, ok := fs.KCache[key]; ok {
		return idx
	}
	k := addK(fs, v)
	fs.KCache[key] = k
	return k
}

// stringK adds a string constant, deduplicating *LuaString objects
// across the entire compilation unit via fs.StringCache.
func stringK(fs *funcState, s string) int {
	ls := fs.StringCache[s]
	if ls == nil {
		ls = &object.LuaString{Data: s, IsShort: len(s) <= 40}
		fs.StringCache[s] = ls
	}
	return k2proto(fs, s, object.MakeString(ls))
}

// intK adds an integer constant.
func intK(fs *funcState, n int64) int {
	return k2proto(fs, n, object.MakeInteger(n))
}

// numberK adds a float constant.
func numberK(fs *funcState, r float64) int {
	if r == 0 {
		// Use a unique key for 0.0 to avoid collision with integer 0
		type floatZeroKey struct{ fs *funcState }
		return k2proto(fs, floatZeroKey{fs}, object.MakeFloat(r))
	}
	// Use the float value with a perturbation as key to avoid integer collision
	nbm := 52 // DBL_MANT_DIG for double
	q := math.Ldexp(1.0, -nbm+1)
	k := r * (1 + q)
	fi := int64(k)
	if float64(fi) != k {
		// Not an integer value — try to reuse
		idx, ok := fs.KCache[k]
		if ok {
			// Check for collision
			if object.RawEqual(fs.Proto.Constants[idx], object.MakeFloat(r)) {
				return idx
			}
		}
		if !ok {
			return k2proto(fs, k, object.MakeFloat(r))
		}
	}
	// Key is still an integer or collision — just add without reuse
	return addK(fs, object.MakeFloat(r))
}

// boolFK adds a false constant.
func boolFK(fs *funcState) int {
	return k2proto(fs, false, object.False)
}

// boolTK adds a true constant.
func boolTK(fs *funcState) int {
	return k2proto(fs, true, object.True)
}

// nilK adds a nil constant.
func nilK(fs *funcState) int {
	// Use a unique key for nil (nil can't be a map key in the normal sense)
	type nilKey struct{}
	return k2proto(fs, nilKey{}, object.Nil)
}

// ---------------------------------------------------------------------------
// Jump management
// ---------------------------------------------------------------------------

// getJump returns the destination of a jump instruction.
func getJump(fs *funcState, pc int) int {
	offset := opcode.GetArgSJ(fs.Proto.Code[pc])
	if offset == noJump {
		return noJump
	}
	return (pc + 1) + offset
}

// fixJump fixes a jump instruction to jump to dest.
func fixJump(fs *funcState, pc, dest int) {
	jmp := &fs.Proto.Code[pc]
	offset := dest - (pc + 1)
	if !(offset >= -opcode.OffsetSJ && offset <= opcode.MaxArgSJ-opcode.OffsetSJ) {
		throwSyntaxError(fs, "control structure too long")
	}
	*jmp = opcode.SetArgSJ(*jmp, offset)
}

// concatJumps concatenates jump-list l2 into jump-list *l1.
// Mirrors: luaK_concat
func concatJumps(fs *funcState, l1 *int, l2 int) {
	if l2 == noJump {
		return
	}
	if *l1 == noJump {
		*l1 = l2
	} else {
		list := *l1
		var next int
		for {
			next = getJump(fs, list)
			if next == noJump {
				break
			}
			list = next
		}
		fixJump(fs, list, l2)
	}
}

// jump emits a JMP instruction. Mirrors: luaK_jump
func jump(fs *funcState) int {
	return codesJ(fs, opcode.OP_JMP, noJump, 0)
}

// ret emits a return instruction. Mirrors: luaK_ret
func ret(fs *funcState, first, nret int) {
	var op opcode.OpCode
	switch nret {
	case 0:
		op = opcode.OP_RETURN0
	case 1:
		op = opcode.OP_RETURN1
	default:
		op = opcode.OP_RETURN
	}
	checkLimit(fs, nret+1, opcode.MaxArgB, "returns")
	codeABC(fs, op, first, nret+1, 0)
}

// condJump emits a test/comparison opcode followed by a JMP. Returns jump position.
func condJump(fs *funcState, op opcode.OpCode, a, b, c, k int) int {
	codeABCk(fs, op, a, b, c, k)
	return jump(fs)
}

// getLabel returns current PC and marks it as a jump target.
func getLabel(fs *funcState) int {
	fs.LastTarget = fs.PC
	return fs.PC
}

// getJumpControl returns the instruction controlling a jump (its condition).
func getJumpControl(fs *funcState, pc int) *uint32 {
	pi := &fs.Proto.Code[pc]
	if pc >= 1 && opcode.TestTMode(opcode.GetOpCode(fs.Proto.Code[pc-1])) {
		return &fs.Proto.Code[pc-1]
	}
	return pi
}

// patchTestReg patches a TESTSET instruction's destination register.
func patchTestReg(fs *funcState, node, reg int) bool {
	i := getJumpControl(fs, node)
	if opcode.GetOpCode(*i) != opcode.OP_TESTSET {
		return false
	}
	if reg != opcode.NoReg && reg != opcode.GetArgB(*i) {
		*i = opcode.SetArgA(*i, reg)
	} else {
		*i = opcode.CreateABCK(opcode.OP_TEST, opcode.GetArgB(*i), 0, 0, opcode.GetArgK(*i))
	}
	return true
}

// removeValues traverses a jump list ensuring no one produces a value.
func removeValues(fs *funcState, list int) {
	for list != noJump {
		patchTestReg(fs, list, opcode.NoReg)
		list = getJump(fs, list)
	}
}

// patchListAux patches a jump list with value/default targets.
func patchListAux(fs *funcState, list, vtarget, reg, dtarget int) {
	for list != noJump {
		next := getJump(fs, list)
		if patchTestReg(fs, list, reg) {
			fixJump(fs, list, vtarget)
		} else {
			fixJump(fs, list, dtarget)
		}
		list = next
	}
}

// patchList patches all jumps in list to jump to target.
func patchList(fs *funcState, list, target int) {
	patchListAux(fs, list, target, opcode.NoReg, target)
}

// patchToHere patches all jumps in list to jump to current position.
func patchToHere(fs *funcState, list int) {
	hr := getLabel(fs)
	patchList(fs, list, hr)
}

// ---------------------------------------------------------------------------
// Nil instruction with optimization
// ---------------------------------------------------------------------------

// nilExpr emits OP_LOADNIL with merge optimization. Mirrors: luaK_nil
func nilExpr(fs *funcState, from, n int) {
	l := from + n - 1
	prev := previousInstruction(fs)
	if opcode.GetOpCode(*prev) == opcode.OP_LOADNIL {
		pfrom := opcode.GetArgA(*prev)
		pl := pfrom + opcode.GetArgB(*prev)
		if (pfrom <= from && from <= pl+1) || (from <= pfrom && pfrom <= l+1) {
			if pfrom < from {
				from = pfrom
			}
			if pl > l {
				l = pl
			}
			*prev = opcode.SetArgA(*prev, from)
			*prev = opcode.SetArgB(*prev, l-from)
			return
		}
	}
	codeABC(fs, opcode.OP_LOADNIL, from, n-1, 0)
}

// ---------------------------------------------------------------------------
// Expression helpers
// ---------------------------------------------------------------------------

// tonumeral checks if an expression is a numeric constant.
func tonumeral(e *expDesc, v *object.TValue) bool {
	if e.HasJumps() {
		return false
	}
	switch e.Kind {
	case vKINT:
		if v != nil {
			*v = object.MakeInteger(e.IVal)
		}
		return true
	case vKFLT:
		if v != nil {
			*v = object.MakeFloat(e.NVal)
		}
		return true
	default:
		return false
	}
}

// const2val returns the compile-time constant value for a VCONST expression.
func const2val(fs *funcState, e *expDesc) *object.TValue {
	dyd := getDyndata(fs)
	return &dyd.ActVar[e.Info].K
}

// exp2Const tries to convert an ExpDesc to a compile-time constant TValue.
func exp2Const(fs *funcState, e *expDesc, v *object.TValue) bool {
	if e.HasJumps() {
		return false
	}
	switch e.Kind {
	case vFALSE:
		*v = object.False
		return true
	case vTRUE:
		*v = object.True
		return true
	case vNIL:
		*v = object.Nil
		return true
	case vKSTR:
		// Use StringCache for dedup if available (via FuncState)
		ls := fs.StringCache[e.StrVal]
		if ls == nil {
			ls = &object.LuaString{Data: e.StrVal, IsShort: len(e.StrVal) <= 40}
			fs.StringCache[e.StrVal] = ls
		}
		*v = object.MakeString(ls)
		return true
	case vCONST:
		cv := const2val(fs, e)
		*v = *cv
		return true
	default:
		return tonumeral(e, v)
	}
}

// const2exp converts a TValue constant into an ExpDesc.
func const2exp(v *object.TValue, e *expDesc) {
	switch v.Tt {
	case object.TagInteger:
		e.Kind = vKINT
		e.IVal = v.Integer()
	case object.TagFloat:
		e.Kind = vKFLT
		e.NVal = v.Float()
	case object.TagFalse:
		e.Kind = vFALSE
	case object.TagTrue:
		e.Kind = vTRUE
	case object.TagNil:
		e.Kind = vNIL
	case object.TagShortStr, object.TagLongStr:
		e.Kind = vKSTR
		e.StrVal = v.StringVal().Data
	}
}

// ---------------------------------------------------------------------------
// Expression discharge
// ---------------------------------------------------------------------------

// setReturns fixes a multi-ret expression to return nresults.
func setReturns(fs *funcState, e *expDesc, nresults int) {
	pc := getInstruction(fs, e)
	checkLimit(fs, nresults+1, opcode.MaxArgC, "multiple results")
	if e.Kind == vCALL {
		*pc = opcode.SetArgC(*pc, nresults+1)
	} else {
		// VVARARG
		*pc = opcode.SetArgC(*pc, nresults+1)
		*pc = opcode.SetArgA(*pc, int(fs.FreeReg))
		reserveRegs(fs, 1)
	}
}

// str2K converts a VKSTR expression to VK (adds string to constant pool).
func str2K(fs *funcState, e *expDesc) int {
	e.Info = stringK(fs, e.StrVal)
	e.Kind = vK
	return e.Info
}

// setOneRet fixes an expression to return one result.
func setOneRet(fs *funcState, e *expDesc) {
	if e.Kind == vCALL {
		e.Kind = vNONRELOC
		e.Info = opcode.GetArgA(*getInstruction(fs, e))
	} else if e.Kind == vVARARG {
		*getInstruction(fs, e) = opcode.SetArgC(*getInstruction(fs, e), 2)
		e.Kind = vRELOC
	}
}

// vaPar2Local converts a vararg parameter to a regular local.
func vaPar2Local(fs *funcState, v *expDesc) {
	fs.Proto.Flag |= object.PF_VATAB
	v.Kind = vLOCAL
}

// dischargeVars ensures an expression is not a variable.
// Mirrors: luaK_dischargevars
func dischargeVars(fs *funcState, e *expDesc) {
	switch e.Kind {
	case vCONST:
		const2exp(const2val(fs, e), e)
	case vVARGVAR:
		vaPar2Local(fs, e)
		fallthrough
	case vLOCAL:
		temp := e.Var.RegIdx
		e.Info = int(temp)
		e.Kind = vNONRELOC
	case vUPVAL:
		e.Info = codeABC(fs, opcode.OP_GETUPVAL, 0, e.Info, 0)
		e.Kind = vRELOC
	case vINDEXUP:
		e.Info = codeABC(fs, opcode.OP_GETTABUP, 0, int(e.Ind.Table), e.Ind.Idx)
		e.Kind = vRELOC
	case vINDEXI:
		freeReg(fs, int(e.Ind.Table))
		e.Info = codeABC(fs, opcode.OP_GETI, 0, int(e.Ind.Table), e.Ind.Idx)
		e.Kind = vRELOC
	case vINDEXSTR:
		freeReg(fs, int(e.Ind.Table))
		e.Info = codeABC(fs, opcode.OP_GETFIELD, 0, int(e.Ind.Table), e.Ind.Idx)
		e.Kind = vRELOC
	case vINDEXED:
		freeRegs(fs, int(e.Ind.Table), e.Ind.Idx)
		e.Info = codeABC(fs, opcode.OP_GETTABLE, 0, int(e.Ind.Table), e.Ind.Idx)
		e.Kind = vRELOC
	case vVARGIND:
		freeRegs(fs, int(e.Ind.Table), e.Ind.Idx)
		e.Info = codeABC(fs, opcode.OP_GETVARG, 0, int(e.Ind.Table), e.Ind.Idx)
		e.Kind = vRELOC
	case vVARARG, vCALL:
		setOneRet(fs, e)
	}
}

// discharge2Reg discharges an expression value into register reg.
func discharge2Reg(fs *funcState, e *expDesc, reg int) {
	dischargeVars(fs, e)
	switch e.Kind {
	case vNIL:
		nilExpr(fs, reg, 1)
	case vFALSE:
		codeABC(fs, opcode.OP_LOADFALSE, reg, 0, 0)
	case vTRUE:
		codeABC(fs, opcode.OP_LOADTRUE, reg, 0, 0)
	case vKSTR:
		str2K(fs, e)
		fallthrough
	case vK:
		codek(fs, reg, e.Info)
	case vKFLT:
		codeFloat(fs, reg, e.NVal)
	case vKINT:
		codeInt(fs, reg, e.IVal)
	case vRELOC:
		pc := getInstruction(fs, e)
		*pc = opcode.SetArgA(*pc, reg)
	case vNONRELOC:
		if reg != e.Info {
			codeABC(fs, opcode.OP_MOVE, reg, e.Info, 0)
		}
	default:
		// VJMP — nothing to do
		return
	}
	e.Info = reg
	e.Kind = vNONRELOC
}

// discharge2AnyReg discharges to any register.
func discharge2AnyReg(fs *funcState, e *expDesc) {
	if e.Kind != vNONRELOC {
		reserveRegs(fs, 1)
		discharge2Reg(fs, e, int(fs.FreeReg)-1)
	}
}

// codeLoadBool emits a load boolean instruction at a jump target.
func codeLoadBool(fs *funcState, a int, op opcode.OpCode) int {
	getLabel(fs) // mark as jump target
	return codeABC(fs, op, a, 0, 0)
}

// needValue checks whether a jump list has any jump that doesn't produce a value.
func needValue(fs *funcState, list int) bool {
	for list != noJump {
		i := *getJumpControl(fs, list)
		if opcode.GetOpCode(i) != opcode.OP_TESTSET {
			return true
		}
		list = getJump(fs, list)
	}
	return false
}

// exp2Reg ensures final expression result is in register reg.
// Mirrors: exp2reg in lcode.c
func exp2Reg(fs *funcState, e *expDesc, reg int) {
	discharge2Reg(fs, e, reg)
	if e.Kind == vJMP {
		concatJumps(fs, &e.T, e.Info)
	}
	if e.HasJumps() {
		var final int
		pf := noJump
		pt := noJump
		if needValue(fs, e.T) || needValue(fs, e.F) {
			fj := noJump
			if e.Kind != vJMP {
				fj = jump(fs)
			}
			pf = codeLoadBool(fs, reg, opcode.OP_LFALSESKIP)
			pt = codeLoadBool(fs, reg, opcode.OP_LOADTRUE)
			patchToHere(fs, fj)
		}
		final = getLabel(fs)
		patchListAux(fs, e.F, final, reg, pf)
		patchListAux(fs, e.T, final, reg, pt)
	}
	e.F = noJump
	e.T = noJump
	e.Info = reg
	e.Kind = vNONRELOC
}

// exp2NextReg ensures final expression result is in next available register.
func exp2NextReg(fs *funcState, e *expDesc) {
	dischargeVars(fs, e)
	freeExp(fs, e)
	reserveRegs(fs, 1)
	exp2Reg(fs, e, int(fs.FreeReg)-1)
}

// exp2AnyReg ensures final expression result is in some register.
func exp2AnyReg(fs *funcState, e *expDesc) int {
	dischargeVars(fs, e)
	if e.Kind == vNONRELOC {
		if !e.HasJumps() {
			return e.Info
		}
		if e.Info >= nVarStack(fs) {
			exp2Reg(fs, e, e.Info)
			return e.Info
		}
	}
	exp2NextReg(fs, e)
	return e.Info
}

// exp2AnyRegUp ensures result is in register, upvalue, or vararg param.
func exp2AnyRegUp(fs *funcState, e *expDesc) {
	if (e.Kind != vUPVAL && e.Kind != vVARGVAR) || e.HasJumps() {
		exp2AnyReg(fs, e)
	}
}

// exp2Val ensures result is in register or is a constant.
func exp2Val(fs *funcState, e *expDesc) {
	if e.Kind == vJMP || e.HasJumps() {
		exp2AnyReg(fs, e)
	} else {
		dischargeVars(fs, e)
	}
}

// exp2K tries to make e a K expression fitting in R/K range.
func exp2K(fs *funcState, e *expDesc) bool {
	if e.HasJumps() {
		return false
	}
	var info int
	switch e.Kind {
	case vTRUE:
		info = boolTK(fs)
	case vFALSE:
		info = boolFK(fs)
	case vNIL:
		info = nilK(fs)
	case vKINT:
		info = intK(fs, e.IVal)
	case vKFLT:
		info = numberK(fs, e.NVal)
	case vKSTR:
		info = stringK(fs, e.StrVal)
	case vK:
		info = e.Info
	default:
		return false
	}
	if info <= maxIndexRK {
		e.Kind = vK
		e.Info = info
		return true
	}
	return false
}

// exp2RK ensures result is in register or K index. Returns true if K.
func exp2RK(fs *funcState, e *expDesc) bool {
	if exp2K(fs, e) {
		return true
	}
	exp2AnyReg(fs, e)
	return false
}

// codeABRK emits an instruction with R/K operand.
func codeABRK(fs *funcState, op opcode.OpCode, a, b int, ec *expDesc) {
	k := 0
	if exp2RK(fs, ec) {
		k = 1
	}
	codeABCk(fs, op, a, b, ec.Info, k)
}

// ---------------------------------------------------------------------------
// Load integer/float helpers
// ---------------------------------------------------------------------------

// fitsC checks if an integer fits in a signed sC operand.
func fitsC(i int64) bool {
	return uint64(i)+uint64(opcode.OffsetSC) <= uint64(opcode.MaxArgC)
}

// fitsBx checks if an integer fits in a signed sBx operand.
func fitsBx(i int64) bool {
	return -int64(opcode.OffsetSBx) <= i && i <= int64(opcode.MaxArgBx)-int64(opcode.OffsetSBx)
}

// int2sC converts a signed integer to the unsigned sC encoding.
func int2sC(i int) int {
	return i + opcode.OffsetSC
}

// codeInt emits a LOADI or LOADK instruction.
func codeInt(fs *funcState, reg int, i int64) {
	if fitsBx(i) {
		codeAsBx(fs, opcode.OP_LOADI, reg, int(i))
	} else {
		codek(fs, reg, intK(fs, i))
	}
}

// codeFloat emits a LOADF or LOADK instruction.
func codeFloat(fs *funcState, reg int, f float64) {
	fi := int64(f)
	if float64(fi) == f && fitsBx(fi) {
		codeAsBx(fs, opcode.OP_LOADF, reg, int(fi))
	} else {
		codek(fs, reg, numberK(fs, f))
	}
}

// ---------------------------------------------------------------------------
// Store operations
// ---------------------------------------------------------------------------

// storeVar generates code to store expression ex into variable var.
// Mirrors: luaK_storevar
func storeVar(fs *funcState, v *expDesc, ex *expDesc) {
	switch v.Kind {
	case vLOCAL:
		freeExp(fs, ex)
		exp2Reg(fs, ex, int(v.Var.RegIdx))
		return
	case vUPVAL:
		e := exp2AnyReg(fs, ex)
		codeABC(fs, opcode.OP_SETUPVAL, e, v.Info, 0)
	case vINDEXUP:
		codeABRK(fs, opcode.OP_SETTABUP, int(v.Ind.Table), v.Ind.Idx, ex)
	case vINDEXI:
		codeABRK(fs, opcode.OP_SETI, int(v.Ind.Table), v.Ind.Idx, ex)
	case vINDEXSTR:
		codeABRK(fs, opcode.OP_SETFIELD, int(v.Ind.Table), v.Ind.Idx, ex)
	case vVARGIND:
		fs.Proto.Flag |= object.PF_VATAB
		fallthrough
	case vINDEXED:
		codeABRK(fs, opcode.OP_SETTABLE, int(v.Ind.Table), v.Ind.Idx, ex)
	}
	freeExp(fs, ex)
}

// selfExpr emits SELF instruction or equivalent (e.key(e,)).
// Mirrors: luaK_self
func selfExpr(fs *funcState, e *expDesc, key *expDesc) {
	exp2AnyReg(fs, e)
	ereg := e.Info
	freeExp(fs, e)
	base := int(fs.FreeReg)
	e.Info = base
	e.Kind = vNONRELOC
	reserveRegs(fs, 2)
	// Is method name a short string in valid K index?
	if len(key.StrVal) <= 40 && exp2K(fs, key) {
		codeABCk(fs, opcode.OP_SELF, base, ereg, key.Info, 0)
	} else {
		exp2AnyReg(fs, key)
		codeABC(fs, opcode.OP_MOVE, base+1, ereg, 0)
		codeABC(fs, opcode.OP_GETTABLE, base, ereg, key.Info)
	}
	freeExp(fs, key)
}

// indexed creates expression t[k]. Mirrors: luaK_indexed
func indexed(fs *funcState, t *expDesc, k *expDesc) {
	keystr := -1
	if k.Kind == vKSTR {
		keystr = str2K(fs, k)
	}
	if t.Kind == vUPVAL && !isKstr(fs, k) {
		exp2AnyReg(fs, t)
	}
	if t.Kind == vUPVAL {
		temp := byte(t.Info)
		t.Ind.Table = temp
		fillIdxK(t, k.Info, vINDEXUP)
	} else if t.Kind == vVARGVAR {
		kreg := exp2AnyReg(fs, k)
		vreg := t.Var.RegIdx
		t.Ind.Table = vreg
		fillIdxK(t, kreg, vVARGIND)
	} else {
		var temp byte
		if t.Kind == vLOCAL {
			temp = t.Var.RegIdx
		} else {
			temp = byte(t.Info)
		}
		t.Ind.Table = temp
		if isKstr(fs, k) {
			fillIdxK(t, k.Info, vINDEXSTR)
		} else if isCint(k) {
			fillIdxK(t, int(k.IVal), vINDEXI)
		} else {
			fillIdxK(t, exp2AnyReg(fs, k), vINDEXED)
		}
	}
	t.Ind.KeyStr = keystr
	t.Ind.ReadOnly = false
}

// fillIdxK is an auxiliary to set indexed expression fields.
func fillIdxK(t *expDesc, idx int, kind expKind) {
	t.Ind.Idx = idx
	t.Kind = kind
}

// isKstr checks if expression is a short string constant in valid K index.
func isKstr(fs *funcState, e *expDesc) bool {
	return e.Kind == vK && !e.HasJumps() && e.Info <= maxIndexRK &&
		e.Info < len(fs.Proto.Constants) &&
		fs.Proto.Constants[e.Info].Tt == object.TagShortStr
}

// isKint checks if expression is a literal integer.
func isKint(e *expDesc) bool {
	return e.Kind == vKINT && !e.HasJumps()
}

// isCint checks if expression is a literal integer in range for register C.
func isCint(e *expDesc) bool {
	return isKint(e) && uint64(e.IVal) <= uint64(opcode.MaxArgC)
}

// isSCint checks if expression is a literal integer fitting in sC.
func isSCint(e *expDesc) bool {
	return isKint(e) && fitsC(e.IVal)
}

// isSCnumber checks if expression is a number fitting in sC.
// Returns the encoded sC value and whether it was a float.
func isSCnumber(e *expDesc, pi *int, isfloat *int) bool {
	var i int64
	if e.Kind == vKINT {
		i = e.IVal
	} else if e.Kind == vKFLT {
		fi := int64(e.NVal)
		if float64(fi) != e.NVal {
			return false
		}
		i = fi
		*isfloat = 1
	} else {
		return false
	}
	if !e.HasJumps() && fitsC(i) {
		*pi = int2sC(int(i))
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Conditional jumps and boolean logic
// ---------------------------------------------------------------------------

// negateCondition negates a comparison condition.
func negateCondition(fs *funcState, e *expDesc) {
	pc := getJumpControl(fs, e.Info)
	*pc = opcode.SetArgK(*pc, opcode.GetArgK(*pc)^1)
}

// jumpOnCond emits code to jump if e is cond. Returns jump position.
func jumpOnCond(fs *funcState, e *expDesc, cond int) int {
	if e.Kind == vRELOC {
		ie := *getInstruction(fs, e)
		if opcode.GetOpCode(ie) == opcode.OP_NOT {
			removeLastInstruction(fs)
			return condJump(fs, opcode.OP_TEST, opcode.GetArgB(ie), 0, 0, boolToInt(cond == 0))
		}
	}
	discharge2AnyReg(fs, e)
	freeExp(fs, e)
	return condJump(fs, opcode.OP_TESTSET, opcode.NoReg, e.Info, 0, cond)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// goIfTrue emits code to go through if e is true, jump otherwise.
func goIfTrue(fs *funcState, e *expDesc) {
	var pc int
	dischargeVars(fs, e)
	switch e.Kind {
	case vJMP:
		negateCondition(fs, e)
		pc = e.Info
	case vK, vKFLT, vKINT, vKSTR, vTRUE:
		pc = noJump
	default:
		pc = jumpOnCond(fs, e, 0)
	}
	concatJumps(fs, &e.F, pc)
	patchToHere(fs, e.T)
	e.T = noJump
}

// goIfFalse emits code to go through if e is false, jump otherwise.
func goIfFalse(fs *funcState, e *expDesc) {
	var pc int
	dischargeVars(fs, e)
	switch e.Kind {
	case vJMP:
		pc = e.Info
	case vNIL, vFALSE:
		pc = noJump
	default:
		pc = jumpOnCond(fs, e, 1)
	}
	concatJumps(fs, &e.T, pc)
	patchToHere(fs, e.F)
	e.F = noJump
}

// codeNot emits code for 'not e' with constant folding.
func codeNot(fs *funcState, e *expDesc) {
	switch e.Kind {
	case vNIL, vFALSE:
		e.Kind = vTRUE
	case vK, vKFLT, vKINT, vKSTR, vTRUE:
		e.Kind = vFALSE
	case vJMP:
		negateCondition(fs, e)
	case vRELOC, vNONRELOC:
		discharge2AnyReg(fs, e)
		freeExp(fs, e)
		e.Info = codeABC(fs, opcode.OP_NOT, 0, e.Info, 0)
		e.Kind = vRELOC
	}
	// Interchange true and false lists
	e.F, e.T = e.T, e.F
	removeValues(fs, e.F)
	removeValues(fs, e.T)
}

// codeCheckGlobal emits ERRNNIL for global variable checking (Lua 5.5).
func codeCheckGlobal(fs *funcState, v *expDesc, k int, line int) {
	exp2AnyReg(fs, v)
	fixLine(fs, line)
	if k >= opcode.MaxArgBx {
		k = 0
	} else {
		k = k + 1
	}
	codeABx(fs, opcode.OP_ERRNNIL, v.Info, k)
	fixLine(fs, line)
	freeExp(fs, v)
}

// ---------------------------------------------------------------------------
// Constant folding
// ---------------------------------------------------------------------------

// foldbinop returns true if the binary operator can be constant-folded.
func foldbinop(opr binOpr) bool {
	return opr <= oprSHR
}

// validop checks if constant folding is safe for the given operation.
func validop(op int, v1, v2 *object.TValue) bool {
	switch op {
	case object.LuaOpBAnd, object.LuaOpBOr, object.LuaOpBXor,
		object.LuaOpShl, object.LuaOpShr, object.LuaOpBNot:
		// Use floor-mode conversion (matches C's LUA_FLOORN2I)
		_, ok1 := object.ToIntegerNS(*v1)
		_, ok2 := object.ToIntegerNS(*v2)
		return ok1 && ok2
	case object.LuaOpDiv, object.LuaOpIDiv, object.LuaOpMod:
		n2, ok := v2.ToNumber()
		return ok && n2 != 0
	default:
		return true
	}
}

// constfolding tries to constant-fold an operation. Returns true on success.
func constfolding(fs *funcState, op int, e1 *expDesc, e2 *expDesc) bool {
	var v1, v2 object.TValue
	if !tonumeral(e1, &v1) || !tonumeral(e2, &v2) || !validop(op, &v1, &v2) {
		return false
	}
	res, ok := object.RawArith(op, v1, v2)
	if !ok {
		return false
	}
	if res.IsInteger() {
		e1.Kind = vKINT
		e1.IVal = res.Integer()
	} else {
		n := res.Float()
		if math.IsNaN(n) || n == 0 {
			return false
		}
		e1.Kind = vKFLT
		e1.NVal = n
	}
	return true
}

// ---------------------------------------------------------------------------
// Operator conversion helpers
// ---------------------------------------------------------------------------

// binopr2op converts a BinOpr to an OpCode.
func binopr2op(opr binOpr, baser binOpr, base opcode.OpCode) opcode.OpCode {
	return opcode.OpCode(int(opr) - int(baser) + int(base))
}

// unopr2op converts a UnOpr to an OpCode.
func unopr2op(opr unOpr) opcode.OpCode {
	return opcode.OpCode(int(opr) - int(oprMINUS) + int(opcode.OP_UNM))
}

// binopr2TM converts a BinOpr to a tag method.
func binopr2TM(opr binOpr) int {
	return int(opr) - int(oprADD) + int(metamethod.TM_ADD)
}

// ---------------------------------------------------------------------------
// Unary operator codegen
// ---------------------------------------------------------------------------

// codeUnExpVal emits code for unary expressions that produce values.
func codeUnExpVal(fs *funcState, op opcode.OpCode, e *expDesc, line int) {
	r := exp2AnyReg(fs, e)
	freeExp(fs, e)
	e.Info = codeABC(fs, op, 0, r, 0)
	e.Kind = vRELOC
	fixLine(fs, line)
}

// prefix applies a prefix operation to expression e.
// Mirrors: luaK_prefix
func prefix(fs *funcState, opr unOpr, e *expDesc, line int) {
	ef := expDesc{Kind: vKINT, IVal: 0, T: noJump, F: noJump}
	dischargeVars(fs, e)
	switch opr {
	case oprMINUS, oprBNOT:
		if constfolding(fs, int(opr)+object.LuaOpUnm, e, &ef) {
			break
		}
		fallthrough
	case oprLEN:
		codeUnExpVal(fs, unopr2op(opr), e, line)
	case oprNOT:
		codeNot(fs, e)
	}
}

// ---------------------------------------------------------------------------
// Binary operator codegen
// ---------------------------------------------------------------------------

// finishBinExpVal emits code for binary expressions that produce values.
func finishBinExpVal(fs *funcState, e1, e2 *expDesc, op opcode.OpCode,
	v2, flip, line int, mmop opcode.OpCode, event int) {
	v1 := exp2AnyReg(fs, e1)
	pc := codeABCk(fs, op, 0, v1, v2, 0)
	freeExps(fs, e1, e2)
	e1.Info = pc
	e1.Kind = vRELOC
	fixLine(fs, line)
	codeABCk(fs, mmop, v1, v2, event, flip)
	fixLine(fs, line)
}

// codeBinExpVal emits code for binary expressions over two registers.
func codeBinExpVal(fs *funcState, opr binOpr, e1, e2 *expDesc, line int) {
	op := binopr2op(opr, oprADD, opcode.OP_ADD)
	v2 := exp2AnyReg(fs, e2)
	finishBinExpVal(fs, e1, e2, op, v2, 0, line, opcode.OP_MMBIN, binopr2TM(opr))
}

// codeBinI emits code for binary operators with immediate operands.
func codeBinI(fs *funcState, op opcode.OpCode, e1, e2 *expDesc, flip, line int, event int) {
	v2 := int2sC(int(e2.IVal))
	finishBinExpVal(fs, e1, e2, op, v2, flip, line, opcode.OP_MMBINI, event)
}

// codeBinK emits code for binary operators with K operands.
func codeBinK(fs *funcState, opr binOpr, e1, e2 *expDesc, flip, line int) {
	event := binopr2TM(opr)
	v2 := e2.Info
	op := binopr2op(opr, oprADD, opcode.OP_ADDK)
	finishBinExpVal(fs, e1, e2, op, v2, flip, line, opcode.OP_MMBINK, event)
}

// finishBinExpNeg tries to code a binary op negating its second operand.
func finishBinExpNeg(fs *funcState, e1, e2 *expDesc, op opcode.OpCode, line, event int) bool {
	if !isKint(e2) {
		return false
	}
	i2 := e2.IVal
	if !(fitsC(i2) && fitsC(-i2)) {
		return false
	}
	v2 := int(i2)
	finishBinExpVal(fs, e1, e2, op, int2sC(-v2), 0, line, opcode.OP_MMBINI, event)
	// Correct metamethod argument
	fs.Proto.Code[fs.PC-1] = opcode.SetArgB(fs.Proto.Code[fs.PC-1], int2sC(v2))
	return true
}

// swapExps swaps two expression descriptors.
func swapExps(e1, e2 *expDesc) {
	*e1, *e2 = *e2, *e1
}

// codeBinNoK emits code for binary operators with no constant operand.
func codeBinNoK(fs *funcState, opr binOpr, e1, e2 *expDesc, flip, line int) {
	if flip != 0 {
		swapExps(e1, e2)
	}
	codeBinExpVal(fs, opr, e1, e2, line)
}

// codeArith emits code for arithmetic operators.
func codeArith(fs *funcState, opr binOpr, e1, e2 *expDesc, flip, line int) {
	if tonumeral(e2, nil) && exp2K(fs, e2) {
		codeBinK(fs, opr, e1, e2, flip, line)
	} else {
		codeBinNoK(fs, opr, e1, e2, flip, line)
	}
}

// codeCommutative emits code for commutative operators (+, *).
func codeCommutative(fs *funcState, op binOpr, e1, e2 *expDesc, line int) {
	flip := 0
	if tonumeral(e1, nil) {
		swapExps(e1, e2)
		flip = 1
	}
	if op == oprADD && isSCint(e2) {
		codeBinI(fs, opcode.OP_ADDI, e1, e2, flip, line, binopr2TM(oprADD))
	} else {
		codeArith(fs, op, e1, e2, flip, line)
	}
}

// codeBitwise emits code for bitwise operations.
func codeBitwise(fs *funcState, opr binOpr, e1, e2 *expDesc, line int) {
	flip := 0
	if e1.Kind == vKINT {
		swapExps(e1, e2)
		flip = 1
	}
	if e2.Kind == vKINT && exp2K(fs, e2) {
		codeBinK(fs, opr, e1, e2, flip, line)
	} else {
		codeBinNoK(fs, opr, e1, e2, flip, line)
	}
}

// codeOrder emits code for order comparisons.
func codeOrder(fs *funcState, opr binOpr, e1, e2 *expDesc) {
	var r1, r2 int
	var im int
	isfloat := 0
	var op opcode.OpCode

	if isSCnumber(e2, &im, &isfloat) {
		r1 = exp2AnyReg(fs, e1)
		r2 = im
		op = binopr2op(opr, oprLT, opcode.OP_LTI)
	} else if isSCnumber(e1, &im, &isfloat) {
		r1 = exp2AnyReg(fs, e2)
		r2 = im
		op = binopr2op(opr, oprLT, opcode.OP_GTI)
	} else {
		r1 = exp2AnyReg(fs, e1)
		r2 = exp2AnyReg(fs, e2)
		op = binopr2op(opr, oprLT, opcode.OP_LT)
	}
	freeExps(fs, e1, e2)
	e1.Info = condJump(fs, op, r1, r2, isfloat, 1)
	e1.Kind = vJMP
}

// codeEq emits code for equality comparisons.
func codeEq(fs *funcState, opr binOpr, e1, e2 *expDesc) {
	var r1, r2 int
	var im int
	isfloat := 0
	var op opcode.OpCode

	if e1.Kind != vNONRELOC {
		swapExps(e1, e2)
	}
	r1 = exp2AnyReg(fs, e1)
	if isSCnumber(e2, &im, &isfloat) {
		op = opcode.OP_EQI
		r2 = im
	} else if exp2RK(fs, e2) {
		op = opcode.OP_EQK
		r2 = e2.Info
	} else {
		op = opcode.OP_EQ
		r2 = exp2AnyReg(fs, e2)
	}
	freeExps(fs, e1, e2)
	eqCond := 0
	if opr == oprEQ {
		eqCond = 1
	}
	e1.Info = condJump(fs, op, r1, r2, isfloat, eqCond)
	e1.Kind = vJMP
}

// codeconcat emits code for concatenation.
func codeconcat(fs *funcState, e1, e2 *expDesc, line int) {
	ie2 := previousInstruction(fs)
	if opcode.GetOpCode(*ie2) == opcode.OP_CONCAT {
		n := opcode.GetArgB(*ie2)
		freeExp(fs, e2)
		*ie2 = opcode.SetArgA(*ie2, e1.Info)
		*ie2 = opcode.SetArgB(*ie2, n+1)
	} else {
		codeABC(fs, opcode.OP_CONCAT, e1.Info, 2, 0)
		freeExp(fs, e2)
		fixLine(fs, line)
	}
}

// ---------------------------------------------------------------------------
// Infix — process 1st operand of binary operation
// ---------------------------------------------------------------------------

// infix processes the 1st operand before reading the 2nd.
// Mirrors: luaK_infix
func infix(fs *funcState, op binOpr, v *expDesc) {
	dischargeVars(fs, v)
	switch op {
	case oprAND:
		goIfTrue(fs, v)
	case oprOR:
		goIfFalse(fs, v)
	case oprCONCAT:
		exp2NextReg(fs, v)
	case oprADD, oprSUB, oprMUL, oprDIV, oprIDIV,
		oprMOD, oprPOW, oprBAND, oprBOR, oprBXOR,
		oprSHL, oprSHR:
		if !tonumeral(v, nil) {
			exp2AnyReg(fs, v)
		}
	case oprEQ, oprNE:
		if !tonumeral(v, nil) {
			exp2RK(fs, v)
		}
	case oprLT, oprLE, oprGT, oprGE:
		var dummy, dummy2 int
		if !isSCnumber(v, &dummy, &dummy2) {
			exp2AnyReg(fs, v)
		}
	}
}

// ---------------------------------------------------------------------------
// Posfix — finalize binary operation after reading 2nd operand
// ---------------------------------------------------------------------------

// posfix finalizes code for binary operation.
// Mirrors: luaK_posfix
func posfix(fs *funcState, opr binOpr, e1, e2 *expDesc, line int) {
	dischargeVars(fs, e2)
	if foldbinop(opr) && constfolding(fs, int(opr)+object.LuaOpAdd, e1, e2) {
		return
	}
	switch opr {
	case oprAND:
		concatJumps(fs, &e2.F, e1.F)
		*e1 = *e2
	case oprOR:
		concatJumps(fs, &e2.T, e1.T)
		*e1 = *e2
	case oprCONCAT:
		exp2NextReg(fs, e2)
		codeconcat(fs, e1, e2, line)
	case oprADD, oprMUL:
		codeCommutative(fs, opr, e1, e2, line)
	case oprSUB:
		if finishBinExpNeg(fs, e1, e2, opcode.OP_ADDI, line, binopr2TM(oprSUB)) {
			break
		}
		fallthrough
	case oprDIV, oprIDIV, oprMOD, oprPOW:
		codeArith(fs, opr, e1, e2, 0, line)
	case oprBAND, oprBOR, oprBXOR:
		codeBitwise(fs, opr, e1, e2, line)
	case oprSHL:
		if isSCint(e1) {
			swapExps(e1, e2)
			codeBinI(fs, opcode.OP_SHLI, e1, e2, 1, line, binopr2TM(oprSHL))
		} else if finishBinExpNeg(fs, e1, e2, opcode.OP_SHRI, line, binopr2TM(oprSHL)) {
			// coded as (r1 >> -I)
		} else {
			codeBinExpVal(fs, opr, e1, e2, line)
		}
	case oprSHR:
		if isSCint(e2) {
			codeBinI(fs, opcode.OP_SHRI, e1, e2, 0, line, binopr2TM(oprSHR))
		} else {
			codeBinExpVal(fs, opr, e1, e2, line)
		}
	case oprEQ, oprNE:
		codeEq(fs, opr, e1, e2)
	case oprGT, oprGE:
		swapExps(e1, e2)
		opr = binOpr(int(opr-oprGT) + int(oprLT))
		fallthrough
	case oprLT, oprLE:
		codeOrder(fs, opr, e1, e2)
	}
}

// ---------------------------------------------------------------------------
// SetTableSize, SetList
// ---------------------------------------------------------------------------

// setTableSize sets the table size in a NEWTABLE instruction.
func setTableSize(fs *funcState, pc, ra, asize, hsize int) {
	inst := &fs.Proto.Code[pc]
	extra := asize / (opcode.MaxArgVC + 1)
	rc := asize % (opcode.MaxArgVC + 1)
	k := 0
	if extra > 0 {
		k = 1
	}
	hsizeEnc := 0
	if hsize != 0 {
		hsizeEnc = int(object.CeilLog2(uint(hsize))) + 1
	}
	*inst = opcode.CreateVABCK(opcode.OP_NEWTABLE, ra, hsizeEnc, rc, k)
	fs.Proto.Code[pc+1] = opcode.CreateAx(opcode.OP_EXTRAARG, extra)
}

// setList emits a SETLIST instruction.
func setList(fs *funcState, base, nelems, tostore int) {
	if tostore == luaMultRet {
		tostore = 0
	}
	if nelems <= opcode.MaxArgVC {
		codeVABCk(fs, opcode.OP_SETLIST, base, tostore, nelems, 0)
	} else {
		extra := nelems / (opcode.MaxArgVC + 1)
		nelems %= (opcode.MaxArgVC + 1)
		codeVABCk(fs, opcode.OP_SETLIST, base, tostore, nelems, 1)
		codeExtraArg(fs, extra)
	}
	fs.FreeReg = byte(base + 1)
}

// ---------------------------------------------------------------------------
// Finish — final pass over the code
// ---------------------------------------------------------------------------

// finaltarget follows jump chains to find the final target.
func finaltarget(code []uint32, i int) int {
	for count := 0; count < 100; count++ {
		pc := code[i]
		if opcode.GetOpCode(pc) != opcode.OP_JMP {
			break
		}
		i += opcode.GetArgSJ(pc) + 1
	}
	return i
}

// finishCode does a final pass over the code for peephole optimizations.
// Mirrors: luaK_finish
func finishCode(fs *funcState) {
	p := fs.Proto
	if p.Flag&object.PF_VATAB != 0 {
		p.Flag &^= object.PF_VAHID
	}
	for i := 0; i < fs.PC; i++ {
		pc := &p.Code[i]
		switch opcode.GetOpCode(*pc) {
		case opcode.OP_RETURN0, opcode.OP_RETURN1:
			if !(fs.NeedClose || (p.Flag&object.PF_VAHID != 0)) {
				break
			}
			*pc = opcode.SetOpCode(*pc, opcode.OP_RETURN)
			fallthrough
		case opcode.OP_RETURN, opcode.OP_TAILCALL:
			if fs.NeedClose {
				*pc = opcode.SetArgK(*pc, 1)
			}
			if p.Flag&object.PF_VAHID != 0 {
				*pc = opcode.SetArgC(*pc, int(p.NumParams)+1)
			}
		case opcode.OP_GETVARG:
			if p.Flag&object.PF_VATAB != 0 {
				*pc = opcode.SetOpCode(*pc, opcode.OP_GETTABLE)
			}
		case opcode.OP_VARARG:
			if p.Flag&object.PF_VATAB != 0 {
				*pc = opcode.SetArgK(*pc, 1)
			}
		case opcode.OP_JMP:
			target := finaltarget(p.Code, i)
			fixJump(fs, i, target)
		}
	}
}
