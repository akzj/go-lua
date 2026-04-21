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

	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/lex"
	"github.com/akzj/go-lua/internal/metamethod"
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

func getDyndata(fs *FuncState) *Dyndata {
	return fs.Lex.DynData.(*Dyndata)
}

// ---------------------------------------------------------------------------
// Helper: NVarStack — number of variables in the stack for the current function
// Mirrors: luaY_nvarstack in lparser.c
// ---------------------------------------------------------------------------

// NVarStack returns the number of stack slots used by active local variables.
// Mirrors: luaY_nvarstack in lparser.c — calls reglevel to skip globals/constants.
func NVarStack(fs *FuncState) int {
	return int(regLevel(fs, fs.NumActVar))
}

// ---------------------------------------------------------------------------
// Helper: CheckLimit — check a value against a limit, error if exceeded
// Mirrors: luaY_checklimit in lparser.c
// ---------------------------------------------------------------------------

func checkLimit(fs *FuncState, v, lim int, what string) {
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
func throwSyntaxError(fs *FuncState, msg string) {
	ls := fs.Lex
	ls.Token.Type = 0 // remove "near <token>" from message
	ls.Line = ls.LastLine
	lex.LexError(ls, msg, 0)
}

// ---------------------------------------------------------------------------
// SemError — semantic error (C: luaK_semerror)
// ---------------------------------------------------------------------------

// SemError raises a semantic error.
func SemError(ls *lex.LexState, msg string) {
	ls.Token.Type = 0
	ls.Line = ls.LastLine
	lex.LexError(ls, msg, 0)
}

// ---------------------------------------------------------------------------
// getInstruction — get instruction referenced by an ExpDesc
// ---------------------------------------------------------------------------

func getInstruction(fs *FuncState, e *ExpDesc) *uint32 {
	return &fs.Proto.Code[e.Info]
}

// ---------------------------------------------------------------------------
// Line info
// ---------------------------------------------------------------------------

// saveLineInfo saves line info for the instruction at fs.PC-1.
// Mirrors: savelineinfo in lcode.c
func saveLineInfo(fs *FuncState, line int) {
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
func removeLastLineInfo(fs *FuncState) {
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
func removeLastInstruction(fs *FuncState) {
	removeLastLineInfo(fs)
	fs.PC--
}

// FixLine changes the line info for the last instruction.
func FixLine(fs *FuncState, line int) {
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

func previousInstruction(fs *FuncState) *uint32 {
	if fs.PC > fs.LastTarget {
		return &fs.Proto.Code[fs.PC-1]
	}
	return &invalidInstruction
}

// ---------------------------------------------------------------------------
// Instruction emission
// ---------------------------------------------------------------------------

// Code emits an instruction and saves line info. Returns the instruction index.
// Mirrors: luaK_code
func Code(fs *FuncState, i uint32) int {
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

// CodeABCk emits an iABC instruction. Mirrors: luaK_codeABCk
func CodeABCk(fs *FuncState, op opcode.OpCode, a, b, c, k int) int {
	return Code(fs, opcode.CreateABCK(op, a, b, c, k))
}

// CodeABC emits an iABC instruction with k=0.
func CodeABC(fs *FuncState, op opcode.OpCode, a, b, c int) int {
	return CodeABCk(fs, op, a, b, c, 0)
}

// CodeVABCk emits an ivABC instruction. Mirrors: luaK_codevABCk
func CodeVABCk(fs *FuncState, op opcode.OpCode, a, vb, vc, k int) int {
	return Code(fs, opcode.CreateVABCK(op, a, vb, vc, k))
}

// CodeABx emits an iABx instruction. Mirrors: luaK_codeABx
func CodeABx(fs *FuncState, op opcode.OpCode, a, bx int) int {
	return Code(fs, opcode.CreateABx(op, a, bx))
}

// codeAsBx emits an iAsBx instruction.
func codeAsBx(fs *FuncState, op opcode.OpCode, a, sbx int) int {
	return Code(fs, opcode.CreateAsBx(op, a, sbx))
}

// codesJ emits an isJ instruction with k bit. Mirrors: codesJ in lcode.c
func codesJ(fs *FuncState, op opcode.OpCode, sj, k int) int {
	return Code(fs, opcode.CreateSJK(op, sj, k))
}

// codeExtraArg emits an EXTRAARG instruction.
func codeExtraArg(fs *FuncState, a int) int {
	return Code(fs, opcode.CreateAx(opcode.OP_EXTRAARG, a))
}

// Codek emits a LOADK or LOADKX instruction.
func Codek(fs *FuncState, reg, k int) int {
	if k <= opcode.MaxArgBx {
		return CodeABx(fs, opcode.OP_LOADK, reg, k)
	}
	p := CodeABx(fs, opcode.OP_LOADKX, reg, 0)
	codeExtraArg(fs, k)
	return p
}

// ---------------------------------------------------------------------------
// Register management
// ---------------------------------------------------------------------------

// CheckStack checks register-stack level. Mirrors: luaK_checkstack
func CheckStack(fs *FuncState, n int) {
	newstack := int(fs.FreeReg) + n
	if newstack > int(fs.Proto.MaxStackSize) {
		checkLimit(fs, newstack, maxFStack, "registers")
		fs.Proto.MaxStackSize = byte(newstack)
	}
}

// ReserveRegs reserves n registers. Mirrors: luaK_reserveregs
func ReserveRegs(fs *FuncState, n int) {
	CheckStack(fs, n)
	fs.FreeReg += byte(n)
}

// freeReg frees a register if it's not a local variable.
func freeReg(fs *FuncState, reg int) {
	if reg >= NVarStack(fs) {
		fs.FreeReg--
	}
}

// freeRegs frees two registers in proper order.
func freeRegs(fs *FuncState, r1, r2 int) {
	if r1 > r2 {
		freeReg(fs, r1)
		freeReg(fs, r2)
	} else {
		freeReg(fs, r2)
		freeReg(fs, r1)
	}
}

// freeExp frees the register used by an expression (if any).
func freeExp(fs *FuncState, e *ExpDesc) {
	if e.Kind == VNONRELOC {
		freeReg(fs, e.Info)
	}
}

// freeExps frees registers used by two expressions.
func freeExps(fs *FuncState, e1, e2 *ExpDesc) {
	r1 := -1
	if e1.Kind == VNONRELOC {
		r1 = e1.Info
	}
	r2 := -1
	if e2.Kind == VNONRELOC {
		r2 = e2.Info
	}
	freeRegs(fs, r1, r2)
}

// ---------------------------------------------------------------------------
// Constant pool management
// ---------------------------------------------------------------------------

// addK adds a constant value to the Proto's constant list.
func addK(fs *FuncState, v object.TValue) int {
	f := fs.Proto
	k := len(f.Constants)
	f.Constants = append(f.Constants, v)
	return k
}

// k2proto adds a constant with dedup via KCache.
func k2proto(fs *FuncState, key any, v object.TValue) int {
	if idx, ok := fs.KCache[key]; ok {
		return idx
	}
	k := addK(fs, v)
	fs.KCache[key] = k
	return k
}

// stringK adds a string constant, deduplicating *LuaString objects
// across the entire compilation unit via fs.StringCache.
func stringK(fs *FuncState, s string) int {
	ls := fs.StringCache[s]
	if ls == nil {
		ls = &object.LuaString{Data: s, IsShort: len(s) <= 40}
		fs.StringCache[s] = ls
	}
	return k2proto(fs, s, object.MakeString(ls))
}

// IntK adds an integer constant.
func IntK(fs *FuncState, n int64) int {
	return k2proto(fs, n, object.MakeInteger(n))
}

// NumberK adds a float constant.
func NumberK(fs *FuncState, r float64) int {
	if r == 0 {
		// Use a unique key for 0.0 to avoid collision with integer 0
		type floatZeroKey struct{ fs *FuncState }
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
func boolFK(fs *FuncState) int {
	return k2proto(fs, false, object.False)
}

// boolTK adds a true constant.
func boolTK(fs *FuncState) int {
	return k2proto(fs, true, object.True)
}

// nilK adds a nil constant.
func nilK(fs *FuncState) int {
	// Use a unique key for nil (nil can't be a map key in the normal sense)
	type nilKey struct{}
	return k2proto(fs, nilKey{}, object.Nil)
}

// ---------------------------------------------------------------------------
// Jump management
// ---------------------------------------------------------------------------

// getJump returns the destination of a jump instruction.
func getJump(fs *FuncState, pc int) int {
	offset := opcode.GetArgSJ(fs.Proto.Code[pc])
	if offset == NoJump {
		return NoJump
	}
	return (pc + 1) + offset
}

// fixJump fixes a jump instruction to jump to dest.
func fixJump(fs *FuncState, pc, dest int) {
	jmp := &fs.Proto.Code[pc]
	offset := dest - (pc + 1)
	if !(offset >= -opcode.OffsetSJ && offset <= opcode.MaxArgSJ-opcode.OffsetSJ) {
		throwSyntaxError(fs, "control structure too long")
	}
	*jmp = opcode.SetArgSJ(*jmp, offset)
}

// ConcatJumps concatenates jump-list l2 into jump-list *l1.
// Mirrors: luaK_concat
func ConcatJumps(fs *FuncState, l1 *int, l2 int) {
	if l2 == NoJump {
		return
	}
	if *l1 == NoJump {
		*l1 = l2
	} else {
		list := *l1
		var next int
		for {
			next = getJump(fs, list)
			if next == NoJump {
				break
			}
			list = next
		}
		fixJump(fs, list, l2)
	}
}

// Jump emits a JMP instruction. Mirrors: luaK_jump
func Jump(fs *FuncState) int {
	return codesJ(fs, opcode.OP_JMP, NoJump, 0)
}

// Ret emits a return instruction. Mirrors: luaK_ret
func Ret(fs *FuncState, first, nret int) {
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
	CodeABC(fs, op, first, nret+1, 0)
}

// condJump emits a test/comparison opcode followed by a JMP. Returns jump position.
func condJump(fs *FuncState, op opcode.OpCode, a, b, c, k int) int {
	CodeABCk(fs, op, a, b, c, k)
	return Jump(fs)
}

// GetLabel returns current PC and marks it as a jump target.
func GetLabel(fs *FuncState) int {
	fs.LastTarget = fs.PC
	return fs.PC
}

// getJumpControl returns the instruction controlling a jump (its condition).
func getJumpControl(fs *FuncState, pc int) *uint32 {
	pi := &fs.Proto.Code[pc]
	if pc >= 1 && opcode.TestTMode(opcode.GetOpCode(fs.Proto.Code[pc-1])) {
		return &fs.Proto.Code[pc-1]
	}
	return pi
}

// patchTestReg patches a TESTSET instruction's destination register.
func patchTestReg(fs *FuncState, node, reg int) bool {
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
func removeValues(fs *FuncState, list int) {
	for list != NoJump {
		patchTestReg(fs, list, opcode.NoReg)
		list = getJump(fs, list)
	}
}

// patchListAux patches a jump list with value/default targets.
func patchListAux(fs *FuncState, list, vtarget, reg, dtarget int) {
	for list != NoJump {
		next := getJump(fs, list)
		if patchTestReg(fs, list, reg) {
			fixJump(fs, list, vtarget)
		} else {
			fixJump(fs, list, dtarget)
		}
		list = next
	}
}

// PatchList patches all jumps in list to jump to target.
func PatchList(fs *FuncState, list, target int) {
	patchListAux(fs, list, target, opcode.NoReg, target)
}

// PatchToHere patches all jumps in list to jump to current position.
func PatchToHere(fs *FuncState, list int) {
	hr := GetLabel(fs)
	PatchList(fs, list, hr)
}


// ---------------------------------------------------------------------------
// Nil instruction with optimization
// ---------------------------------------------------------------------------

// Nil emits OP_LOADNIL with merge optimization. Mirrors: luaK_nil
func Nil(fs *FuncState, from, n int) {
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
	CodeABC(fs, opcode.OP_LOADNIL, from, n-1, 0)
}

// ---------------------------------------------------------------------------
// Expression helpers
// ---------------------------------------------------------------------------

// tonumeral checks if an expression is a numeric constant.
func tonumeral(e *ExpDesc, v *object.TValue) bool {
	if e.HasJumps() {
		return false
	}
	switch e.Kind {
	case VKINT:
		if v != nil {
			*v = object.MakeInteger(e.IVal)
		}
		return true
	case VKFLT:
		if v != nil {
			*v = object.MakeFloat(e.NVal)
		}
		return true
	default:
		return false
	}
}

// const2val returns the compile-time constant value for a VCONST expression.
func const2val(fs *FuncState, e *ExpDesc) *object.TValue {
	dyd := getDyndata(fs)
	return &dyd.ActVar[e.Info].K
}

// Exp2Const tries to convert an ExpDesc to a compile-time constant TValue.
func Exp2Const(fs *FuncState, e *ExpDesc, v *object.TValue) bool {
	if e.HasJumps() {
		return false
	}
	switch e.Kind {
	case VFALSE:
		*v = object.False
		return true
	case VTRUE:
		*v = object.True
		return true
	case VNIL:
		*v = object.Nil
		return true
	case VKSTR:
		// Use StringCache for dedup if available (via FuncState)
		ls := fs.StringCache[e.StrVal]
		if ls == nil {
			ls = &object.LuaString{Data: e.StrVal, IsShort: len(e.StrVal) <= 40}
			fs.StringCache[e.StrVal] = ls
		}
		*v = object.MakeString(ls)
		return true
	case VCONST:
		cv := const2val(fs, e)
		*v = *cv
		return true
	default:
		return tonumeral(e, v)
	}
}

// const2exp converts a TValue constant into an ExpDesc.
func const2exp(v *object.TValue, e *ExpDesc) {
	switch v.Tt {
	case object.TagInteger:
		e.Kind = VKINT
		e.IVal = v.Integer()
	case object.TagFloat:
		e.Kind = VKFLT
		e.NVal = v.Float()
	case object.TagFalse:
		e.Kind = VFALSE
	case object.TagTrue:
		e.Kind = VTRUE
	case object.TagNil:
		e.Kind = VNIL
	case object.TagShortStr, object.TagLongStr:
		e.Kind = VKSTR
		e.StrVal = v.StringVal().Data
	}
}

// ---------------------------------------------------------------------------
// Expression discharge
// ---------------------------------------------------------------------------

// SetReturns fixes a multi-ret expression to return nresults.
func SetReturns(fs *FuncState, e *ExpDesc, nresults int) {
	pc := getInstruction(fs, e)
	checkLimit(fs, nresults+1, opcode.MaxArgC, "multiple results")
	if e.Kind == VCALL {
		*pc = opcode.SetArgC(*pc, nresults+1)
	} else {
		// VVARARG
		*pc = opcode.SetArgC(*pc, nresults+1)
		*pc = opcode.SetArgA(*pc, int(fs.FreeReg))
		ReserveRegs(fs, 1)
	}
}

// str2K converts a VKSTR expression to VK (adds string to constant pool).
func str2K(fs *FuncState, e *ExpDesc) int {
	e.Info = stringK(fs, e.StrVal)
	e.Kind = VK
	return e.Info
}

// SetOneRet fixes an expression to return one result.
func SetOneRet(fs *FuncState, e *ExpDesc) {
	if e.Kind == VCALL {
		e.Kind = VNONRELOC
		e.Info = opcode.GetArgA(*getInstruction(fs, e))
	} else if e.Kind == VVARARG {
		*getInstruction(fs, e) = opcode.SetArgC(*getInstruction(fs, e), 2)
		e.Kind = VRELOC
	}
}

// VaPar2Local converts a vararg parameter to a regular local.
func VaPar2Local(fs *FuncState, v *ExpDesc) {
	fs.Proto.Flag |= object.PF_VATAB
	v.Kind = VLOCAL
}

// DischargeVars ensures an expression is not a variable.
// Mirrors: luaK_dischargevars
func DischargeVars(fs *FuncState, e *ExpDesc) {
	switch e.Kind {
	case VCONST:
		const2exp(const2val(fs, e), e)
	case VVARGVAR:
		VaPar2Local(fs, e)
		fallthrough
	case VLOCAL:
		temp := e.Var.RegIdx
		e.Info = int(temp)
		e.Kind = VNONRELOC
	case VUPVAL:
		e.Info = CodeABC(fs, opcode.OP_GETUPVAL, 0, e.Info, 0)
		e.Kind = VRELOC
	case VINDEXUP:
		e.Info = CodeABC(fs, opcode.OP_GETTABUP, 0, int(e.Ind.Table), e.Ind.Idx)
		e.Kind = VRELOC
	case VINDEXI:
		freeReg(fs, int(e.Ind.Table))
		e.Info = CodeABC(fs, opcode.OP_GETI, 0, int(e.Ind.Table), e.Ind.Idx)
		e.Kind = VRELOC
	case VINDEXSTR:
		freeReg(fs, int(e.Ind.Table))
		e.Info = CodeABC(fs, opcode.OP_GETFIELD, 0, int(e.Ind.Table), e.Ind.Idx)
		e.Kind = VRELOC
	case VINDEXED:
		freeRegs(fs, int(e.Ind.Table), e.Ind.Idx)
		e.Info = CodeABC(fs, opcode.OP_GETTABLE, 0, int(e.Ind.Table), e.Ind.Idx)
		e.Kind = VRELOC
	case VVARGIND:
		freeRegs(fs, int(e.Ind.Table), e.Ind.Idx)
		e.Info = CodeABC(fs, opcode.OP_GETVARG, 0, int(e.Ind.Table), e.Ind.Idx)
		e.Kind = VRELOC
	case VVARARG, VCALL:
		SetOneRet(fs, e)
	}
}

// discharge2Reg discharges an expression value into register reg.
func discharge2Reg(fs *FuncState, e *ExpDesc, reg int) {
	DischargeVars(fs, e)
	switch e.Kind {
	case VNIL:
		Nil(fs, reg, 1)
	case VFALSE:
		CodeABC(fs, opcode.OP_LOADFALSE, reg, 0, 0)
	case VTRUE:
		CodeABC(fs, opcode.OP_LOADTRUE, reg, 0, 0)
	case VKSTR:
		str2K(fs, e)
		fallthrough
	case VK:
		Codek(fs, reg, e.Info)
	case VKFLT:
		codeFloat(fs, reg, e.NVal)
	case VKINT:
		codeInt(fs, reg, e.IVal)
	case VRELOC:
		pc := getInstruction(fs, e)
		*pc = opcode.SetArgA(*pc, reg)
	case VNONRELOC:
		if reg != e.Info {
			CodeABC(fs, opcode.OP_MOVE, reg, e.Info, 0)
		}
	default:
		// VJMP — nothing to do
		return
	}
	e.Info = reg
	e.Kind = VNONRELOC
}

// discharge2AnyReg discharges to any register.
func discharge2AnyReg(fs *FuncState, e *ExpDesc) {
	if e.Kind != VNONRELOC {
		ReserveRegs(fs, 1)
		discharge2Reg(fs, e, int(fs.FreeReg)-1)
	}
}

// codeLoadBool emits a load boolean instruction at a jump target.
func codeLoadBool(fs *FuncState, a int, op opcode.OpCode) int {
	GetLabel(fs) // mark as jump target
	return CodeABC(fs, op, a, 0, 0)
}

// needValue checks whether a jump list has any jump that doesn't produce a value.
func needValue(fs *FuncState, list int) bool {
	for list != NoJump {
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
func exp2Reg(fs *FuncState, e *ExpDesc, reg int) {
	discharge2Reg(fs, e, reg)
	if e.Kind == VJMP {
		ConcatJumps(fs, &e.T, e.Info)
	}
	if e.HasJumps() {
		var final int
		pf := NoJump
		pt := NoJump
		if needValue(fs, e.T) || needValue(fs, e.F) {
			fj := NoJump
			if e.Kind != VJMP {
				fj = Jump(fs)
			}
			pf = codeLoadBool(fs, reg, opcode.OP_LFALSESKIP)
			pt = codeLoadBool(fs, reg, opcode.OP_LOADTRUE)
			PatchToHere(fs, fj)
		}
		final = GetLabel(fs)
		patchListAux(fs, e.F, final, reg, pf)
		patchListAux(fs, e.T, final, reg, pt)
	}
	e.F = NoJump
	e.T = NoJump
	e.Info = reg
	e.Kind = VNONRELOC
}

// Exp2NextReg ensures final expression result is in next available register.
func Exp2NextReg(fs *FuncState, e *ExpDesc) {
	DischargeVars(fs, e)
	freeExp(fs, e)
	ReserveRegs(fs, 1)
	exp2Reg(fs, e, int(fs.FreeReg)-1)
}

// Exp2AnyReg ensures final expression result is in some register.
func Exp2AnyReg(fs *FuncState, e *ExpDesc) int {
	DischargeVars(fs, e)
	if e.Kind == VNONRELOC {
		if !e.HasJumps() {
			return e.Info
		}
		if e.Info >= NVarStack(fs) {
			exp2Reg(fs, e, e.Info)
			return e.Info
		}
	}
	Exp2NextReg(fs, e)
	return e.Info
}

// Exp2AnyRegUp ensures result is in register, upvalue, or vararg param.
func Exp2AnyRegUp(fs *FuncState, e *ExpDesc) {
	if (e.Kind != VUPVAL && e.Kind != VVARGVAR) || e.HasJumps() {
		Exp2AnyReg(fs, e)
	}
}

// Exp2Val ensures result is in register or is a constant.
func Exp2Val(fs *FuncState, e *ExpDesc) {
	if e.Kind == VJMP || e.HasJumps() {
		Exp2AnyReg(fs, e)
	} else {
		DischargeVars(fs, e)
	}
}

// exp2K tries to make e a K expression fitting in R/K range.
func exp2K(fs *FuncState, e *ExpDesc) bool {
	if e.HasJumps() {
		return false
	}
	var info int
	switch e.Kind {
	case VTRUE:
		info = boolTK(fs)
	case VFALSE:
		info = boolFK(fs)
	case VNIL:
		info = nilK(fs)
	case VKINT:
		info = IntK(fs, e.IVal)
	case VKFLT:
		info = NumberK(fs, e.NVal)
	case VKSTR:
		info = stringK(fs, e.StrVal)
	case VK:
		info = e.Info
	default:
		return false
	}
	if info <= maxIndexRK {
		e.Kind = VK
		e.Info = info
		return true
	}
	return false
}

// exp2RK ensures result is in register or K index. Returns true if K.
func exp2RK(fs *FuncState, e *ExpDesc) bool {
	if exp2K(fs, e) {
		return true
	}
	Exp2AnyReg(fs, e)
	return false
}

// codeABRK emits an instruction with R/K operand.
func codeABRK(fs *FuncState, op opcode.OpCode, a, b int, ec *ExpDesc) {
	k := 0
	if exp2RK(fs, ec) {
		k = 1
	}
	CodeABCk(fs, op, a, b, ec.Info, k)
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
func codeInt(fs *FuncState, reg int, i int64) {
	if fitsBx(i) {
		codeAsBx(fs, opcode.OP_LOADI, reg, int(i))
	} else {
		Codek(fs, reg, IntK(fs, i))
	}
}

// codeFloat emits a LOADF or LOADK instruction.
func codeFloat(fs *FuncState, reg int, f float64) {
	fi := int64(f)
	if float64(fi) == f && fitsBx(fi) {
		codeAsBx(fs, opcode.OP_LOADF, reg, int(fi))
	} else {
		Codek(fs, reg, NumberK(fs, f))
	}
}

// ---------------------------------------------------------------------------
// Store operations
// ---------------------------------------------------------------------------

// StoreVar generates code to store expression ex into variable var.
// Mirrors: luaK_storevar
func StoreVar(fs *FuncState, v *ExpDesc, ex *ExpDesc) {
	switch v.Kind {
	case VLOCAL:
		freeExp(fs, ex)
		exp2Reg(fs, ex, int(v.Var.RegIdx))
		return
	case VUPVAL:
		e := Exp2AnyReg(fs, ex)
		CodeABC(fs, opcode.OP_SETUPVAL, e, v.Info, 0)
	case VINDEXUP:
		codeABRK(fs, opcode.OP_SETTABUP, int(v.Ind.Table), v.Ind.Idx, ex)
	case VINDEXI:
		codeABRK(fs, opcode.OP_SETI, int(v.Ind.Table), v.Ind.Idx, ex)
	case VINDEXSTR:
		codeABRK(fs, opcode.OP_SETFIELD, int(v.Ind.Table), v.Ind.Idx, ex)
	case VVARGIND:
		fs.Proto.Flag |= object.PF_VATAB
		fallthrough
	case VINDEXED:
		codeABRK(fs, opcode.OP_SETTABLE, int(v.Ind.Table), v.Ind.Idx, ex)
	}
	freeExp(fs, ex)
}

// Self emits SELF instruction or equivalent (e.key(e,)).
// Mirrors: luaK_self
func Self(fs *FuncState, e *ExpDesc, key *ExpDesc) {
	Exp2AnyReg(fs, e)
	ereg := e.Info
	freeExp(fs, e)
	base := int(fs.FreeReg)
	e.Info = base
	e.Kind = VNONRELOC
	ReserveRegs(fs, 2)
	// Is method name a short string in valid K index?
	if len(key.StrVal) <= 40 && exp2K(fs, key) {
		CodeABCk(fs, opcode.OP_SELF, base, ereg, key.Info, 0)
	} else {
		Exp2AnyReg(fs, key)
		CodeABC(fs, opcode.OP_MOVE, base+1, ereg, 0)
		CodeABC(fs, opcode.OP_GETTABLE, base, ereg, key.Info)
	}
	freeExp(fs, key)
}

// Indexed creates expression t[k]. Mirrors: luaK_indexed
func Indexed(fs *FuncState, t *ExpDesc, k *ExpDesc) {
	keystr := -1
	if k.Kind == VKSTR {
		keystr = str2K(fs, k)
	}
	if t.Kind == VUPVAL && !isKstr(fs, k) {
		Exp2AnyReg(fs, t)
	}
	if t.Kind == VUPVAL {
		temp := byte(t.Info)
		t.Ind.Table = temp
		fillIdxK(t, k.Info, VINDEXUP)
	} else if t.Kind == VVARGVAR {
		kreg := Exp2AnyReg(fs, k)
		vreg := t.Var.RegIdx
		t.Ind.Table = vreg
		fillIdxK(t, kreg, VVARGIND)
	} else {
		var temp byte
		if t.Kind == VLOCAL {
			temp = t.Var.RegIdx
		} else {
			temp = byte(t.Info)
		}
		t.Ind.Table = temp
		if isKstr(fs, k) {
			fillIdxK(t, k.Info, VINDEXSTR)
		} else if isCint(k) {
			fillIdxK(t, int(k.IVal), VINDEXI)
		} else {
			fillIdxK(t, Exp2AnyReg(fs, k), VINDEXED)
		}
	}
	t.Ind.KeyStr = keystr
	t.Ind.ReadOnly = false
}

// fillIdxK is an auxiliary to set indexed expression fields.
func fillIdxK(t *ExpDesc, idx int, kind ExpKind) {
	t.Ind.Idx = idx
	t.Kind = kind
}

// isKstr checks if expression is a short string constant in valid K index.
func isKstr(fs *FuncState, e *ExpDesc) bool {
	return e.Kind == VK && !e.HasJumps() && e.Info <= maxIndexRK &&
		e.Info < len(fs.Proto.Constants) &&
		fs.Proto.Constants[e.Info].Tt == object.TagShortStr
}

// isKint checks if expression is a literal integer.
func isKint(e *ExpDesc) bool {
	return e.Kind == VKINT && !e.HasJumps()
}

// isCint checks if expression is a literal integer in range for register C.
func isCint(e *ExpDesc) bool {
	return isKint(e) && uint64(e.IVal) <= uint64(opcode.MaxArgC)
}

// isSCint checks if expression is a literal integer fitting in sC.
func isSCint(e *ExpDesc) bool {
	return isKint(e) && fitsC(e.IVal)
}

// isSCnumber checks if expression is a number fitting in sC.
// Returns the encoded sC value and whether it was a float.
func isSCnumber(e *ExpDesc, pi *int, isfloat *int) bool {
	var i int64
	if e.Kind == VKINT {
		i = e.IVal
	} else if e.Kind == VKFLT {
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
func negateCondition(fs *FuncState, e *ExpDesc) {
	pc := getJumpControl(fs, e.Info)
	*pc = opcode.SetArgK(*pc, opcode.GetArgK(*pc)^1)
}

// jumpOnCond emits code to jump if e is cond. Returns jump position.
func jumpOnCond(fs *FuncState, e *ExpDesc, cond int) int {
	if e.Kind == VRELOC {
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

// GoIfTrue emits code to go through if e is true, jump otherwise.
func GoIfTrue(fs *FuncState, e *ExpDesc) {
	var pc int
	DischargeVars(fs, e)
	switch e.Kind {
	case VJMP:
		negateCondition(fs, e)
		pc = e.Info
	case VK, VKFLT, VKINT, VKSTR, VTRUE:
		pc = NoJump
	default:
		pc = jumpOnCond(fs, e, 0)
	}
	ConcatJumps(fs, &e.F, pc)
	PatchToHere(fs, e.T)
	e.T = NoJump
}

// GoIfFalse emits code to go through if e is false, jump otherwise.
func GoIfFalse(fs *FuncState, e *ExpDesc) {
	var pc int
	DischargeVars(fs, e)
	switch e.Kind {
	case VJMP:
		pc = e.Info
	case VNIL, VFALSE:
		pc = NoJump
	default:
		pc = jumpOnCond(fs, e, 1)
	}
	ConcatJumps(fs, &e.T, pc)
	PatchToHere(fs, e.F)
	e.F = NoJump
}

// codeNot emits code for 'not e' with constant folding.
func codeNot(fs *FuncState, e *ExpDesc) {
	switch e.Kind {
	case VNIL, VFALSE:
		e.Kind = VTRUE
	case VK, VKFLT, VKINT, VKSTR, VTRUE:
		e.Kind = VFALSE
	case VJMP:
		negateCondition(fs, e)
	case VRELOC, VNONRELOC:
		discharge2AnyReg(fs, e)
		freeExp(fs, e)
		e.Info = CodeABC(fs, opcode.OP_NOT, 0, e.Info, 0)
		e.Kind = VRELOC
	}
	// Interchange true and false lists
	e.F, e.T = e.T, e.F
	removeValues(fs, e.F)
	removeValues(fs, e.T)
}

// CodeCheckGlobal emits ERRNNIL for global variable checking (Lua 5.5).
func CodeCheckGlobal(fs *FuncState, v *ExpDesc, k int, line int) {
	Exp2AnyReg(fs, v)
	FixLine(fs, line)
	if k >= opcode.MaxArgBx {
		k = 0
	} else {
		k = k + 1
	}
	CodeABx(fs, opcode.OP_ERRNNIL, v.Info, k)
	FixLine(fs, line)
	freeExp(fs, v)
}

// ---------------------------------------------------------------------------
// Constant folding
// ---------------------------------------------------------------------------

// foldbinop returns true if the binary operator can be constant-folded.
func foldbinop(opr BinOpr) bool {
	return opr <= OPR_SHR
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
func constfolding(fs *FuncState, op int, e1 *ExpDesc, e2 *ExpDesc) bool {
	var v1, v2 object.TValue
	if !tonumeral(e1, &v1) || !tonumeral(e2, &v2) || !validop(op, &v1, &v2) {
		return false
	}
	res, ok := object.RawArith(op, v1, v2)
	if !ok {
		return false
	}
	if res.IsInteger() {
		e1.Kind = VKINT
		e1.IVal = res.Integer()
	} else {
		n := res.Float()
		if math.IsNaN(n) || n == 0 {
			return false
		}
		e1.Kind = VKFLT
		e1.NVal = n
	}
	return true
}

// ---------------------------------------------------------------------------
// Operator conversion helpers
// ---------------------------------------------------------------------------

// binopr2op converts a BinOpr to an OpCode.
func binopr2op(opr BinOpr, baser BinOpr, base opcode.OpCode) opcode.OpCode {
	return opcode.OpCode(int(opr) - int(baser) + int(base))
}

// unopr2op converts a UnOpr to an OpCode.
func unopr2op(opr UnOpr) opcode.OpCode {
	return opcode.OpCode(int(opr) - int(OPR_MINUS) + int(opcode.OP_UNM))
}

// binopr2TM converts a BinOpr to a tag method.
func binopr2TM(opr BinOpr) int {
	return int(opr) - int(OPR_ADD) + int(metamethod.TM_ADD)
}

// ---------------------------------------------------------------------------
// Unary operator codegen
// ---------------------------------------------------------------------------

// codeUnExpVal emits code for unary expressions that produce values.
func codeUnExpVal(fs *FuncState, op opcode.OpCode, e *ExpDesc, line int) {
	r := Exp2AnyReg(fs, e)
	freeExp(fs, e)
	e.Info = CodeABC(fs, op, 0, r, 0)
	e.Kind = VRELOC
	FixLine(fs, line)
}

// Prefix applies a prefix operation to expression e.
// Mirrors: luaK_prefix
func Prefix(fs *FuncState, opr UnOpr, e *ExpDesc, line int) {
	ef := ExpDesc{Kind: VKINT, IVal: 0, T: NoJump, F: NoJump}
	DischargeVars(fs, e)
	switch opr {
	case OPR_MINUS, OPR_BNOT:
		if constfolding(fs, int(opr)+object.LuaOpUnm, e, &ef) {
			break
		}
		fallthrough
	case OPR_LEN:
		codeUnExpVal(fs, unopr2op(opr), e, line)
	case OPR_NOT:
		codeNot(fs, e)
	}
}

// ---------------------------------------------------------------------------
// Binary operator codegen
// ---------------------------------------------------------------------------

// finishBinExpVal emits code for binary expressions that produce values.
func finishBinExpVal(fs *FuncState, e1, e2 *ExpDesc, op opcode.OpCode,
	v2, flip, line int, mmop opcode.OpCode, event int) {
	v1 := Exp2AnyReg(fs, e1)
	pc := CodeABCk(fs, op, 0, v1, v2, 0)
	freeExps(fs, e1, e2)
	e1.Info = pc
	e1.Kind = VRELOC
	FixLine(fs, line)
	CodeABCk(fs, mmop, v1, v2, event, flip)
	FixLine(fs, line)
}

// codeBinExpVal emits code for binary expressions over two registers.
func codeBinExpVal(fs *FuncState, opr BinOpr, e1, e2 *ExpDesc, line int) {
	op := binopr2op(opr, OPR_ADD, opcode.OP_ADD)
	v2 := Exp2AnyReg(fs, e2)
	finishBinExpVal(fs, e1, e2, op, v2, 0, line, opcode.OP_MMBIN, binopr2TM(opr))
}

// codeBinI emits code for binary operators with immediate operands.
func codeBinI(fs *FuncState, op opcode.OpCode, e1, e2 *ExpDesc, flip, line int, event int) {
	v2 := int2sC(int(e2.IVal))
	finishBinExpVal(fs, e1, e2, op, v2, flip, line, opcode.OP_MMBINI, event)
}

// codeBinK emits code for binary operators with K operands.
func codeBinK(fs *FuncState, opr BinOpr, e1, e2 *ExpDesc, flip, line int) {
	event := binopr2TM(opr)
	v2 := e2.Info
	op := binopr2op(opr, OPR_ADD, opcode.OP_ADDK)
	finishBinExpVal(fs, e1, e2, op, v2, flip, line, opcode.OP_MMBINK, event)
}

// finishBinExpNeg tries to code a binary op negating its second operand.
func finishBinExpNeg(fs *FuncState, e1, e2 *ExpDesc, op opcode.OpCode, line, event int) bool {
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
func swapExps(e1, e2 *ExpDesc) {
	*e1, *e2 = *e2, *e1
}

// codeBinNoK emits code for binary operators with no constant operand.
func codeBinNoK(fs *FuncState, opr BinOpr, e1, e2 *ExpDesc, flip, line int) {
	if flip != 0 {
		swapExps(e1, e2)
	}
	codeBinExpVal(fs, opr, e1, e2, line)
}

// codeArith emits code for arithmetic operators.
func codeArith(fs *FuncState, opr BinOpr, e1, e2 *ExpDesc, flip, line int) {
	if tonumeral(e2, nil) && exp2K(fs, e2) {
		codeBinK(fs, opr, e1, e2, flip, line)
	} else {
		codeBinNoK(fs, opr, e1, e2, flip, line)
	}
}

// codeCommutative emits code for commutative operators (+, *).
func codeCommutative(fs *FuncState, op BinOpr, e1, e2 *ExpDesc, line int) {
	flip := 0
	if tonumeral(e1, nil) {
		swapExps(e1, e2)
		flip = 1
	}
	if op == OPR_ADD && isSCint(e2) {
		codeBinI(fs, opcode.OP_ADDI, e1, e2, flip, line, binopr2TM(OPR_ADD))
	} else {
		codeArith(fs, op, e1, e2, flip, line)
	}
}

// codeBitwise emits code for bitwise operations.
func codeBitwise(fs *FuncState, opr BinOpr, e1, e2 *ExpDesc, line int) {
	flip := 0
	if e1.Kind == VKINT {
		swapExps(e1, e2)
		flip = 1
	}
	if e2.Kind == VKINT && exp2K(fs, e2) {
		codeBinK(fs, opr, e1, e2, flip, line)
	} else {
		codeBinNoK(fs, opr, e1, e2, flip, line)
	}
}

// codeOrder emits code for order comparisons.
func codeOrder(fs *FuncState, opr BinOpr, e1, e2 *ExpDesc) {
	var r1, r2 int
	var im int
	isfloat := 0
	var op opcode.OpCode

	if isSCnumber(e2, &im, &isfloat) {
		r1 = Exp2AnyReg(fs, e1)
		r2 = im
		op = binopr2op(opr, OPR_LT, opcode.OP_LTI)
	} else if isSCnumber(e1, &im, &isfloat) {
		r1 = Exp2AnyReg(fs, e2)
		r2 = im
		op = binopr2op(opr, OPR_LT, opcode.OP_GTI)
	} else {
		r1 = Exp2AnyReg(fs, e1)
		r2 = Exp2AnyReg(fs, e2)
		op = binopr2op(opr, OPR_LT, opcode.OP_LT)
	}
	freeExps(fs, e1, e2)
	e1.Info = condJump(fs, op, r1, r2, isfloat, 1)
	e1.Kind = VJMP
}

// codeEq emits code for equality comparisons.
func codeEq(fs *FuncState, opr BinOpr, e1, e2 *ExpDesc) {
	var r1, r2 int
	var im int
	isfloat := 0
	var op opcode.OpCode

	if e1.Kind != VNONRELOC {
		swapExps(e1, e2)
	}
	r1 = Exp2AnyReg(fs, e1)
	if isSCnumber(e2, &im, &isfloat) {
		op = opcode.OP_EQI
		r2 = im
	} else if exp2RK(fs, e2) {
		op = opcode.OP_EQK
		r2 = e2.Info
	} else {
		op = opcode.OP_EQ
		r2 = Exp2AnyReg(fs, e2)
	}
	freeExps(fs, e1, e2)
	eqCond := 0
	if opr == OPR_EQ {
		eqCond = 1
	}
	e1.Info = condJump(fs, op, r1, r2, isfloat, eqCond)
	e1.Kind = VJMP
}

// codeconcat emits code for concatenation.
func codeconcat(fs *FuncState, e1, e2 *ExpDesc, line int) {
	ie2 := previousInstruction(fs)
	if opcode.GetOpCode(*ie2) == opcode.OP_CONCAT {
		n := opcode.GetArgB(*ie2)
		freeExp(fs, e2)
		*ie2 = opcode.SetArgA(*ie2, e1.Info)
		*ie2 = opcode.SetArgB(*ie2, n+1)
	} else {
		CodeABC(fs, opcode.OP_CONCAT, e1.Info, 2, 0)
		freeExp(fs, e2)
		FixLine(fs, line)
	}
}

// ---------------------------------------------------------------------------
// Infix — process 1st operand of binary operation
// ---------------------------------------------------------------------------

// Infix processes the 1st operand before reading the 2nd.
// Mirrors: luaK_infix
func Infix(fs *FuncState, op BinOpr, v *ExpDesc) {
	DischargeVars(fs, v)
	switch op {
	case OPR_AND:
		GoIfTrue(fs, v)
	case OPR_OR:
		GoIfFalse(fs, v)
	case OPR_CONCAT:
		Exp2NextReg(fs, v)
	case OPR_ADD, OPR_SUB, OPR_MUL, OPR_DIV, OPR_IDIV,
		OPR_MOD, OPR_POW, OPR_BAND, OPR_BOR, OPR_BXOR,
		OPR_SHL, OPR_SHR:
		if !tonumeral(v, nil) {
			Exp2AnyReg(fs, v)
		}
	case OPR_EQ, OPR_NE:
		if !tonumeral(v, nil) {
			exp2RK(fs, v)
		}
	case OPR_LT, OPR_LE, OPR_GT, OPR_GE:
		var dummy, dummy2 int
		if !isSCnumber(v, &dummy, &dummy2) {
			Exp2AnyReg(fs, v)
		}
	}
}

// ---------------------------------------------------------------------------
// Posfix — finalize binary operation after reading 2nd operand
// ---------------------------------------------------------------------------

// Posfix finalizes code for binary operation.
// Mirrors: luaK_posfix
func Posfix(fs *FuncState, opr BinOpr, e1, e2 *ExpDesc, line int) {
	DischargeVars(fs, e2)
	if foldbinop(opr) && constfolding(fs, int(opr)+object.LuaOpAdd, e1, e2) {
		return
	}
	switch opr {
	case OPR_AND:
		ConcatJumps(fs, &e2.F, e1.F)
		*e1 = *e2
	case OPR_OR:
		ConcatJumps(fs, &e2.T, e1.T)
		*e1 = *e2
	case OPR_CONCAT:
		Exp2NextReg(fs, e2)
		codeconcat(fs, e1, e2, line)
	case OPR_ADD, OPR_MUL:
		codeCommutative(fs, opr, e1, e2, line)
	case OPR_SUB:
		if finishBinExpNeg(fs, e1, e2, opcode.OP_ADDI, line, binopr2TM(OPR_SUB)) {
			break
		}
		fallthrough
	case OPR_DIV, OPR_IDIV, OPR_MOD, OPR_POW:
		codeArith(fs, opr, e1, e2, 0, line)
	case OPR_BAND, OPR_BOR, OPR_BXOR:
		codeBitwise(fs, opr, e1, e2, line)
	case OPR_SHL:
		if isSCint(e1) {
			swapExps(e1, e2)
			codeBinI(fs, opcode.OP_SHLI, e1, e2, 1, line, binopr2TM(OPR_SHL))
		} else if finishBinExpNeg(fs, e1, e2, opcode.OP_SHRI, line, binopr2TM(OPR_SHL)) {
			// coded as (r1 >> -I)
		} else {
			codeBinExpVal(fs, opr, e1, e2, line)
		}
	case OPR_SHR:
		if isSCint(e2) {
			codeBinI(fs, opcode.OP_SHRI, e1, e2, 0, line, binopr2TM(OPR_SHR))
		} else {
			codeBinExpVal(fs, opr, e1, e2, line)
		}
	case OPR_EQ, OPR_NE:
		codeEq(fs, opr, e1, e2)
	case OPR_GT, OPR_GE:
		swapExps(e1, e2)
		opr = BinOpr(int(opr-OPR_GT) + int(OPR_LT))
		fallthrough
	case OPR_LT, OPR_LE:
		codeOrder(fs, opr, e1, e2)
	}
}

// ---------------------------------------------------------------------------
// SetTableSize, SetList
// ---------------------------------------------------------------------------

// SetTableSize sets the table size in a NEWTABLE instruction.
func SetTableSize(fs *FuncState, pc, ra, asize, hsize int) {
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

// SetList emits a SETLIST instruction.
func SetList(fs *FuncState, base, nelems, tostore int) {
	if tostore == luaMultRet {
		tostore = 0
	}
	if nelems <= opcode.MaxArgVC {
		CodeVABCk(fs, opcode.OP_SETLIST, base, tostore, nelems, 0)
	} else {
		extra := nelems / (opcode.MaxArgVC + 1)
		nelems %= (opcode.MaxArgVC + 1)
		CodeVABCk(fs, opcode.OP_SETLIST, base, tostore, nelems, 1)
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

// Finish does a final pass over the code for peephole optimizations.
// Mirrors: luaK_finish
func Finish(fs *FuncState) {
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


