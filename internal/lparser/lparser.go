package lparser

/*
** $Id: lparser.go $
** Lua Parser and Code Generator
** Ported from lparser.c, lparser.h, lcode.c, lcode.h
*/

import (
	"fmt"
	"math"

	"github.com/akzj/go-lua/internal/lfunc"
	"github.com/akzj/go-lua/internal/llex"
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lopcodes"
	"github.com/akzj/go-lua/internal/lstate"
	"github.com/akzj/go-lua/internal/lstring"
	"github.com/akzj/go-lua/internal/lzio"
)

/*
** Maximum number of variable declarations per function
 */
const MAXVARS = 200

/*
** LUA_MULTRET - special return value for multiple returns
 */
const LUA_MULTRET = -1

/*
** NO_JUMP - marks end of patch list
 */
const NO_JUMP = -1

/*
** Expression kinds
 */
type expkind int

const (
	VVOID expkind = iota // empty expression
	VNIL                 // constant nil
	VTRUE                // constant true
	VFALSE               // constant false
	VK                   // constant in 'k'
	VKFLT                // floating constant
	VKINT                // integer constant
	VKSTR                // string constant
	VNONRELOC            // expression in fixed register
	VLOCAL               // local variable
	VVARGVAR             // vararg parameter
	VGLOBAL              // global variable
	VUPVAL               // upvalue variable
	VCONST               // compile-time constant variable
	VINDEXED             // indexed variable
	VVARGIND             // indexed vararg parameter
	VINDEXUP             // indexed upvalue
	VINDEXI              // indexed with constant integer
	VINDEXSTR            // indexed with literal string
	VJMP                 // test/comparison jump
	VRELOC               // can put result in any register
	VCALL                // function call
	VVARARG              // vararg expression
)

/*
** Expression descriptor
 */
type Expdesc struct {
	K expkind
	U struct {
		Ival   int64
		Nval   float64
		Strval *lobject.TString
		Info   int
		Ind    struct {
			Idx    uint8
			T      uint8
			Ro     uint8
			Keystr int
		}
		Var struct {
			Ridx uint8
			Vidx int16
		}
	}
	T int // patch list: exit when true
	F int // patch list: exit when false
}

/*
** Variable kinds
 */
const (
	VDKREG = iota // regular local
	RDKCONST      // local constant
	RDKVAVAR      // vararg parameter
	RDKTOCLOSE    // to-be-closed
	RDKCTC        // local compile-time constant
	GDKREG        // regular global
	GDKCONST      // global constant
)

/*
** BlockCnt - block list node
 */
type BlockCnt struct {
	Previous   *BlockCnt
	FirstLabel int
	FirstGoto  int
	NActVar    int16
	Upval      uint8
	IsLoop     uint8
	InsideTBC  uint8
}

/*
** Vardesc - description of an active variable
 */
type Vardesc struct {
	Kind  uint8
	Ridx  uint8
	Pidx  int16
	Name  *lobject.TString
	K     lobject.TValue
}

/*
** Labeldesc - description of pending goto statements
 */
type Labeldesc struct {
	Name    *lobject.TString
	Pc      int
	Line    int
	NActVar int16
	Close   uint8
}

/*
** Labellist - list of labels or gotos
 */
type Labellist struct {
	Arr  []Labeldesc
	N    int
	Size int
}

/*
** Dyndata - dynamic structures used by parser
 */
type Dyndata struct {
	ActVar struct {
		Arr  []Vardesc
		N    int
		Size int
	}
	Gt    Labellist
	Label Labellist
}

/*
** FuncState - state needed to generate code for a function
** This is THE FuncState type used by parser, code generator, and lexer
 */
type FuncState struct {
	F             *lobject.Proto
	Prev          *FuncState
	Ls            *llex.LexState
	Bl            *BlockCnt
	Kcache        *lobject.Table
	Pc            int
	LastTarget    int
	PreviousLine  int
	NK            int
	Np            int
	NAbsLineInfo  int
	FirstLocal    int
	FirstLabel    int
	NDebugVars    int16
	NActVar       int16
	Nups          uint8
	FreeReg       uint8
	IWthAbs       uint8
	NeedClose     uint8
}

/*
** Binary operators
 */
type BinOpr int

const (
	OPR_ADD BinOpr = iota
	OPR_SUB
	OPR_MUL
	OPR_MOD
	OPR_POW
	OPR_DIV
	OPR_IDIV
	OPR_BAND
	OPR_BOR
	OPR_BXOR
	OPR_SHL
	OPR_SHR
	OPR_CONCAT
	OPR_EQ
	OPR_LT
	OPR_LE
	OPR_NE
	OPR_GT
	OPR_GE
	OPR_AND
	OPR_OR
	OPR_NOBINOPR
)

/*
** Unary operators
 */
type UnOpr int

const (
	OPR_MINUS UnOpr = iota
	OPR_BNOT
	OPR_NOT
	OPR_LEN
	OPR_NOUNOPR
)

/*
** GetFuncState returns the FuncState from LexState's Fs field
 */
func GetFuncState(ls *llex.LexState) *FuncState {
	if ls == nil {
		return nil
	}
	fs, ok := ls.Fs.(*FuncState)
	if !ok {
		return nil
	}
	return fs
}

/*
** Test if expression kind is a variable
 */
func VkIsVar(k expkind) bool {
	return k >= VLOCAL && k <= VINDEXSTR
}

/*
** Test if expression kind is indexed
 */
func VkIsIndexed(k expkind) bool {
	return k >= VINDEXED && k <= VINDEXSTR
}

/*
** Test if multiple return
 */
func HasMultRet(k expkind) bool {
	return k == VCALL || k == VVARARG
}

/*
** Get local variable descriptor
 */
func GetLocalVarDesc(fs *FuncState, vidx int) *Vardesc {
	if fs == nil || fs.Ls == nil {
		return nil
	}
	dyd, ok := fs.Ls.Dyd.(*Dyndata)
	if !ok || dyd == nil {
		return nil
	}
	return &dyd.ActVar.Arr[fs.FirstLocal+vidx]
}

/*
** Get number of variables in register stack
 */
func NVarStack(fs *FuncState) uint8 {
	return regLevel(fs, int(fs.NActVar))
}

func regLevel(fs *FuncState, nvar int) uint8 {
	for nvar > 0 {
		nvar--
		vd := GetLocalVarDesc(fs, nvar)
		if vd == nil {
			continue
		}
		if vd.Kind < RDKTOCLOSE {
			return uint8(vd.Ridx) + 1
		}
	}
	return 0
}

/*
** Initialize expression
 */
func InitExp(e *Expdesc, k expkind, i int) {
	e.F = NO_JUMP
	e.T = NO_JUMP
	e.K = k
	e.U.Info = i
}

/*
** Has jumps
 */
func hasjumps(e *Expdesc) bool {
	return e.T != e.F
}

/*
** Get instruction from expdesc
 */
func getinstruction(fs *FuncState, e *Expdesc) lobject.LUInt32 {
	return fs.F.Code[e.U.Info]
}

/*
** Code instruction
 */
func Code(fs *FuncState, i lobject.LUInt32) int {
	fs.Pc++
	if fs.Pc >= len(fs.F.Code) {
		fs.F.Code = append(fs.F.Code, i)
	} else {
		fs.F.Code[fs.Pc-1] = i
	}
	return fs.Pc - 1
}

/*
** Code ABC instruction
 */
func CodeABC(fs *FuncState, o lopcodes.OpCode, a, b, c int) int {
	return Code(fs, lopcodes.CREATE_ABCk(o, a, b, c, 0))
}

/*
** Code ABx instruction
 */
func CodeABx(fs *FuncState, o lopcodes.OpCode, a, bx int) int {
	return Code(fs, lopcodes.CREATE_ABx(o, a, bx))
}

/*
** Jump - generate jump instruction
 */
func Jump(fs *FuncState) int {
	return Code(fs, lopcodes.CREATE_sJ(lopcodes.OP_JMP, NO_JUMP, 0))
}

/*
** Get jump target
 */
func getjump(fs *FuncState, pc int) int {
	offset := lopcodes.GETARG_sJ(fs.F.Code[pc])
	if offset == NO_JUMP {
		return NO_JUMP
	}
	return pc + 1 + offset
}

/*
** Fix jump offset
 */
func fixjump(fs *FuncState, pc, dest int) {
	offset := dest - (pc + 1)
	inst := &fs.F.Code[pc]
	lopcodes.SETARG_sJ(inst, offset)
}

/*
** Patch list to target
 */
func PatchList(fs *FuncState, list, target int) {
	for list != NO_JUMP {
		next := getjump(fs, list)
		fixjump(fs, list, target)
		list = next
	}
}

/*
** Patch to here
 */
func PatchToHere(fs *FuncState, list int) {
	PatchList(fs, list, fs.Pc)
}

/*
** Concat jump lists
 */
func Concat(fs *FuncState, l1 *int, l2 int) {
	if l2 == NO_JUMP {
		return
	}
	if *l1 == NO_JUMP {
		*l1 = l2
		return
	}
	list := *l1
	for {
		next := getjump(fs, list)
		if next == NO_JUMP {
			break
		}
		list = next
	}
	fixjump(fs, list, l2)
}

/*
** Check stack
 */
func CheckStack(fs *FuncState, n int) {
	newstack := int(fs.FreeReg) + n
	if newstack > int(fs.F.Maxstacksize) {
		if newstack > lopcodes.MAX_STACK {
			return
		}
		fs.F.Maxstacksize = lobject.LuByte(newstack)
	}
}

/*
** Reserve registers
 */
func ReserveRegs(fs *FuncState, n int) {
	CheckStack(fs, n)
	fs.FreeReg += uint8(n)
}

/*
** Free register
 */
func freereg(fs *FuncState, reg int) {
	if reg >= int(NVarStack(fs)) {
		fs.FreeReg--
	}
}

/*
** Free expression registers
 */
func freeexp(fs *FuncState, e *Expdesc) {
	if e.K == VNONRELOC {
		freereg(fs, e.U.Info)
	}
}

/*
** Nil instruction
 */
func Nil(fs *FuncState, from, n int) {
	CodeABC(fs, lopcodes.OP_LOADNIL, from, n-1, 0)
}

/*
** Load constant
 */
func loadk(fs *FuncState, reg, k int) int {
	if k <= lopcodes.MAXARG_Bx {
		return CodeABx(fs, lopcodes.OP_LOADK, reg, k)
	}
	p := CodeABx(fs, lopcodes.OP_LOADKX, reg, 0)
	CodeABx(fs, lopcodes.OP_EXTRAARG, 0, k)
	return p
}

/*
** Get label
 */
func getlabel(fs *FuncState) int {
	fs.LastTarget = fs.Pc
	return fs.Pc
}

/*
** Add constant to prototype
 */
func addk(fs *FuncState, v *lobject.TValue) int {
	f := fs.F
	fs.NK++
	if fs.NK > len(f.K) {
		newK := make([]lobject.TValue, len(f.K)*2+1)
		copy(newK, f.K)
		f.K = newK
	}
	lobject.SetObj(&f.K[fs.NK-1], v)
	return fs.NK - 1
}

/*
** Int constant
 */
func intK(fs *FuncState, n int64) int {
	var v lobject.TValue
	lobject.SetIntValue(&v, n)
	return addk(fs, &v)
}

/*
** Nil constant
 */
func nilK(fs *FuncState) int {
	var v lobject.TValue
	lobject.SetNilValue(&v)
	return addk(fs, &v)
}

/*
** Discharge variables
 */
func DischargeVars(fs *FuncState, e *Expdesc) {
	switch e.K {
	case VLOCAL:
		e.U.Info = int(e.U.Var.Ridx)
		e.K = VNONRELOC
	case VUPVAL:
		e.U.Info = CodeABC(fs, lopcodes.OP_GETUPVAL, 0, e.U.Info, 0)
		e.K = VRELOC
	case VGLOBAL:
		// Global variable - load from _ENV table
		// Simplified: just mark as non-relocatable with special handling
		e.K = VNONRELOC
	case VINDEXED:
		// Handle indexed access later
	case VCONST:
		e.K = VNONRELOC
		e.U.Info = -1
	case VJMP, VCALL, VVARARG:
		// Already has a location
	default:
		// Other cases: nothing to discharge
	}
}

/*
** Ensure expression is in a register
 */
func exp2reg(fs *FuncState, e *Expdesc, reg int) {
	DischargeVars(fs, e)
	switch e.K {
	case VNIL:
		Nil(fs, reg, 1)
	case VTRUE:
		CodeABC(fs, lopcodes.OP_LOADTRUE, reg, 0, 0)
	case VFALSE:
		CodeABC(fs, lopcodes.OP_LOADFALSE, reg, 0, 0)
	case VK:
		loadk(fs, reg, e.U.Info)
	case VKFLT:
		loadk(fs, reg, intK(fs, int64(e.U.Nval)))
	case VKINT:
		loadk(fs, reg, intK(fs, e.U.Ival))
	case VKSTR:
		loadk(fs, reg, addk(fs, &lobject.TValue{}))
	case VRELOC:
		inst := &fs.F.Code[e.U.Info]
		lopcodes.SETARG_A(inst, reg)
	case VNONRELOC:
		if reg != e.U.Info {
			CodeABC(fs, lopcodes.OP_MOVE, reg, e.U.Info, 0)
		}
	case VJMP:
		Concat(fs, &e.T, e.U.Info)
	default:
		// Nothing to do
	}
	if e.K != VJMP {
		e.F = NO_JUMP
	}
	e.T = NO_JUMP
	e.U.Info = reg
	e.K = VNONRELOC
}

/*
** Ensure expression has a value
 */
func Exp2Val(fs *FuncState, e *Expdesc) {
	if e.K == VJMP || hasjumps(e) {
		Exp2AnyReg(fs, e)
	} else {
		DischargeVars(fs, e)
	}
}

/*
** Ensure expression is in a register
 */
func Exp2AnyReg(fs *FuncState, e *Expdesc) int {
	DischargeVars(fs, e)
	if e.K != VNONRELOC {
		ReserveRegs(fs, 1)
		exp2reg(fs, e, int(fs.FreeReg-1))
	}
	return e.U.Info
}

/*
** Ensure expression is in next register
 */
func Exp2NextReg(fs *FuncState, e *Expdesc) {
	DischargeVars(fs, e)
	freeexp(fs, e)
	ReserveRegs(fs, 1)
	exp2reg(fs, e, int(fs.FreeReg-1))
}

/*
** Store variable
 */
func StoreVar(fs *FuncState, v *Expdesc, e *Expdesc) {
	switch v.K {
	case VLOCAL:
		Exp2Val(fs, e)
		freeexp(fs, e)
		exp2reg(fs, e, int(v.U.Var.Ridx))
	case VUPVAL:
		r := Exp2AnyReg(fs, e)
		CodeABC(fs, lopcodes.OP_SETUPVAL, r, v.U.Info, 0)
		freeexp(fs, e)
	case VGLOBAL:
		// Global variable - use temporary register
		Exp2Val(fs, e)
		r := int(fs.FreeReg)
		ReserveRegs(fs, 1)
		exp2reg(fs, e, r)
		// Set global via _ENV - simplified
	case VINDEXED:
		// table[key] = value - simplified
		Exp2Val(fs, e)
		CodeABC(fs, lopcodes.OP_SETTABLE, int(v.U.Ind.T), int(v.U.Ind.Idx), 0)
		freeexp(fs, e)
	default:
		// Invalid
	}
}

/*
** Conditional jump
 */
func condjump(fs *FuncState, op lopcodes.OpCode, a, b, c, k int) int {
	CodeABC(fs, op, a, b, c)
	return Jump(fs)
}

/*
** Go if true
 */
func GoIfTrue(fs *FuncState, e *Expdesc) {
	var pc int
	DischargeVars(fs, e)
	switch e.K {
	case VJMP:
		pc = e.U.Info
	case VTRUE, VFALSE, VNIL:
		pc = NO_JUMP
	case VK, VKINT, VKFLT:
		pc = NO_JUMP
	default:
		Exp2AnyReg(fs, e)
		freeexp(fs, e)
		pc = condjump(fs, lopcodes.OP_TESTSET, lopcodes.NO_REG, e.U.Info, 0, 1)
	}
	Concat(fs, &e.F, pc)
	PatchToHere(fs, e.T)
	e.T = NO_JUMP
}

/*
** Go if false
 */
func GoIfFalse(fs *FuncState, e *Expdesc) {
	var pc int
	DischargeVars(fs, e)
	switch e.K {
	case VJMP:
		pc = e.U.Info
	case VNIL, VFALSE:
		pc = NO_JUMP
	default:
		Exp2AnyReg(fs, e)
		freeexp(fs, e)
		pc = condjump(fs, lopcodes.OP_TESTSET, lopcodes.NO_REG, e.U.Info, 0, 0)
	}
	Concat(fs, &e.T, pc)
	PatchToHere(fs, e.F)
	e.F = NO_JUMP
}

/*
** Prefix operator
 */
func Prefix(fs *FuncState, op UnOpr, e *Expdesc, line int) {
	DischargeVars(fs, e)
	switch op {
	case OPR_MINUS:
		if e.K == VKINT {
			e.U.Ival = -e.U.Ival
		} else if e.K == VKFLT {
			e.U.Nval = -e.U.Nval
		} else {
			r := Exp2AnyReg(fs, e)
			e.U.Info = CodeABC(fs, lopcodes.OP_UNM, 0, r, 0)
			e.K = VRELOC
		}
	case OPR_NOT:
		switch e.K {
		case VNIL, VFALSE:
			e.K = VTRUE
		case VTRUE, VK, VKINT, VKFLT:
			e.K = VFALSE
		case VJMP:
			e.T, e.F = e.F, e.T
		case VRELOC, VNONRELOC:
			Exp2AnyReg(fs, e)
			freeexp(fs, e)
			e.U.Info = CodeABC(fs, lopcodes.OP_NOT, 0, e.U.Info, 0)
			e.K = VRELOC
		}
		e.T, e.F = e.F, e.T
	case OPR_LEN:
		r := Exp2AnyReg(fs, e)
		e.U.Info = CodeABC(fs, lopcodes.OP_LEN, 0, r, 0)
		e.K = VRELOC
	}
}

/*
** Binary operator infix
 */
func Infix(fs *FuncState, op BinOpr, v *Expdesc) {
	DischargeVars(fs, v)
	switch op {
	case OPR_AND:
		GoIfTrue(fs, v)
	case OPR_OR:
		GoIfFalse(fs, v)
	case OPR_CONCAT:
		Exp2NextReg(fs, v)
	case OPR_ADD, OPR_SUB, OPR_MUL, OPR_DIV, OPR_IDIV, OPR_MOD, OPR_POW:
		// Keep value for optimization
	default:
		Exp2AnyReg(fs, v)
	}
}

/*
** Binary operator postfix
 */
func Posfix(fs *FuncState, op BinOpr, e1 *Expdesc, e2 *Expdesc, line int) {
	DischargeVars(fs, e2)
	switch op {
	case OPR_AND:
		PatchToHere(fs, e1.F)
		*e1 = *e2
	case OPR_OR:
		PatchToHere(fs, e1.T)
		*e1 = *e2
	case OPR_CONCAT:
		Exp2NextReg(fs, e2)
		CodeABC(fs, lopcodes.OP_CONCAT, e1.U.Info, 2, 0)
		freeexp(fs, e2)
	case OPR_ADD:
		_ = Exp2AnyReg(fs, e2)
		e1.U.Info = CodeABC(fs, lopcodes.OP_ADD, 0, e1.U.Info, e2.U.Info)
		e1.K = VRELOC
		freeexp(fs, e2)
	case OPR_SUB:
		_ = Exp2AnyReg(fs, e2)
		e1.U.Info = CodeABC(fs, lopcodes.OP_SUB, 0, e1.U.Info, e2.U.Info)
		e1.K = VRELOC
		freeexp(fs, e2)
	case OPR_MUL:
		_ = Exp2AnyReg(fs, e2)
		e1.U.Info = CodeABC(fs, lopcodes.OP_MUL, 0, e1.U.Info, e2.U.Info)
		e1.K = VRELOC
		freeexp(fs, e2)
	case OPR_DIV:
		_ = Exp2AnyReg(fs, e2)
		e1.U.Info = CodeABC(fs, lopcodes.OP_DIV, 0, e1.U.Info, e2.U.Info)
		e1.K = VRELOC
		freeexp(fs, e2)
	default:
		// Complex operators - use general approach
		Exp2AnyReg(fs, e2)
		freeexp(fs, e2)
		Exp2AnyReg(fs, e1)
		e1.K = VNONRELOC
	}
}

/*
** Indexed access
 */
func Indexed(fs *FuncState, t *Expdesc, k *Expdesc) {
	t.U.Ind.T = uint8(t.U.Info)
	if k.K == VKSTR {
		t.U.Ind.Idx = uint8(k.U.Info)
		t.K = VINDEXED
	} else {
		r := Exp2AnyReg(fs, k)
		t.U.Ind.Idx = uint8(r)
		t.K = VINDEXED
	}
}

/*
** Create code string expression
 */
func CodeString(e *Expdesc, s *lobject.TString) {
	InitExp(e, VKSTR, 0)
	e.U.Strval = s
}

/*
** Create code name expression
 */
func CodeName(ls *llex.LexState, e *Expdesc) {
	CodeString(e, StrCheckName(ls))
}

/*
** Check name token and return it
 */
func StrCheckName(ls *llex.LexState) *lobject.TString {
	if ls.T.Token != llex.TK_NAME {
		llex.SyntaxError(ls, "unexpected symbol")
	}
	ts := ls.T.SemInfo.Ts
	llex.Next(ls)
	return ts
}

/*
** Test next token is c
 */
func TestNext(ls *llex.LexState, c int) bool {
	if ls.T.Token == c {
		llex.Next(ls)
		return true
	}
	return false
}

/*
** Check next token is c
 */
func Check(ls *llex.LexState, c int) {
	if ls.T.Token != c {
		llex.SyntaxError(ls, fmt.Sprintf("%s expected", llex.Token2Str(ls, c)))
	}
}

/*
** Check and skip next token
 */
func CheckMatch(ls *llex.LexState, what, who, where int) {
	if !TestNext(ls, what) {
		llex.SyntaxError(ls, "unmatched tokens")
	}
}

/*
** Search upvalue
 */
func SearchUpvalue(fs *FuncState, name *lobject.TString) int {
	for i := 0; i < int(fs.Nups); i++ {
		if fs.F.Upvalues[i].Name == name {
			return i
		}
	}
	return -1
}

/*
** Open function
 */
func OpenFunc(ls *llex.LexState, fs *FuncState, bl *BlockCnt) {
	f := fs.F
	fs.Prev = GetFuncState(ls)
	fs.Ls = ls
	llex.SetFs(ls, fs)
	fs.Pc = 0
	fs.PreviousLine = f.Linedefined
	fs.IWthAbs = 0
	fs.LastTarget = 0
	fs.FreeReg = 0
	fs.NK = 0
	fs.NAbsLineInfo = 0
	fs.Np = 0
	fs.Nups = 0
	fs.NDebugVars = 0
	fs.NActVar = 0
	fs.NeedClose = 0
	fs.Bl = nil
	f.Source = ls.Source
	f.Maxstacksize = 2
	fs.Kcache = nil
	EnterBlock(fs, bl, 0)
}

/*
** Close function
 */
func CloseFunc(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	Ret(fs, int(NVarStack(fs)), 0)
	LeaveBlock(fs)
	Finish(fs)
	llex.SetFs(ls, nil)
}

/*
** Enter block
 */
func EnterBlock(fs *FuncState, bl *BlockCnt, isLoop uint8) {
	bl.IsLoop = isLoop
	bl.NActVar = fs.NActVar
	bl.FirstLabel = 0
	bl.FirstGoto = 0
	bl.Upval = 0
	if fs.Bl != nil {
		bl.InsideTBC = fs.Bl.InsideTBC
	} else {
		bl.InsideTBC = 0
	}
	bl.Previous = fs.Bl
	fs.Bl = bl
}

/*
** Leave block
 */
func LeaveBlock(fs *FuncState) {
	if fs.Bl == nil {
		return
	}
	bl := fs.Bl
	fs.Bl = bl.Previous
}

/*
** Statement list
 */
func StatList(ls *llex.LexState) {
	for !BlockFollow(ls, true) {
		if ls.T.Token == llex.TK_RETURN {
			Statement(ls)
			return
		}
		Statement(ls)
	}
}

/*
** Test if current token is in follow set of a block
 */
func BlockFollow(ls *llex.LexState, withUntil bool) bool {
	switch ls.T.Token {
	case llex.TK_ELSE, llex.TK_ELSEIF, llex.TK_END, llex.TK_EOS:
		return true
	case llex.TK_UNTIL:
		return withUntil
	}
	return false
}

/*
** Statement parser
 */
func Statement(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}

	switch ls.T.Token {
	case llex.TK_IF:
		llex.Next(ls)
		ifStatement(ls)
	case llex.TK_WHILE:
		llex.Next(ls)
		whileStatement(ls)
	case llex.TK_DO:
		llex.Next(ls)
		Block(ls)
		CheckMatch(ls, llex.TK_END, llex.TK_DO, 0)
	case llex.TK_FOR:
		llex.Next(ls)
		forStatement(ls)
	case llex.TK_REPEAT:
		llex.Next(ls)
		repeatStatement(ls)
	case llex.TK_FUNCTION:
		llex.Next(ls)
		funcStatement(ls)
	case llex.TK_LOCAL:
		llex.Next(ls)
		localStatement(ls)
	case llex.TK_GLOBAL:
		llex.Next(ls)
		globalStatement(ls)
	case llex.TK_RETURN:
		llex.Next(ls)
		returnStatement(ls)
	case llex.TK_BREAK:
		breakStatement(ls)
	case ';':
		llex.Next(ls)
	default:
		// Expression statement (function call or assignment)
		exprStatement(ls)
	}
}

/*
** Local statement - handles "local x = expr"
 */
func localStatement(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}

	// Check for local function
	if TestNext(ls, llex.TK_FUNCTION) {
		localFunc(ls)
		return
	}

	// Parse local variable names
	var names []*lobject.TString
	for {
		if ls.T.Token != llex.TK_NAME {
			break
		}
		name := StrCheckName(ls)
		names = append(names, name)
		if ls.T.Token != ',' {
			break
		}
		llex.Next(ls) // skip ','
	}

	nexps := 0
	if ls.T.Token == '=' {
		// Has initialization
		llex.Next(ls) // skip '='

		// Parse expressions
		for {
			var e Expdesc
			Expr(ls, &e)

			// Store first expression
			if len(names) > 0 && nexps == 0 {
				// First expression - store in register
				Exp2NextReg(fs, &e)
			} else if nexps > 0 {
				Exp2NextReg(fs, &e)
			}

			nexps++
			if !TestNext(ls, ',') {
				break
			}
		}
	}

	// Initialize remaining variables with nil
	for i := nexps; i < len(names); i++ {
		Nil(fs, int(fs.FreeReg), 1)
		fs.FreeReg++
	}

	// Register local variables
	for i, name := range names {
		vd := &Vardesc{
			Kind: VDKREG,
			Ridx: uint8(i),
			Name: name,
		}
		if dyd, ok := fs.Ls.Dyd.(*Dyndata); ok && dyd != nil {
			dyd.ActVar.Arr = append(dyd.ActVar.Arr, *vd)
		}
		fs.NActVar++
	}
}

/*
** Local function
 */
func localFunc(ls *llex.LexState) {
	// Skip for now - full implementation would parse function body
	StrCheckName(ls)
}

/*
** Global statement
 */
func globalStatement(ls *llex.LexState) {
	var v Expdesc
	singlevar(ls, &v)

	if ls.T.Token == '=' {
		llex.Next(ls)
		var e Expdesc
		Expr(ls, &e)
		StoreVar(GetFuncState(ls), &v, &e)
	}
}

/*
** Function statement
 */
func funcStatement(ls *llex.LexState) {
	var v Expdesc
	kind := singlevaraux(GetFuncState(ls), StrCheckName(ls), &v, 1)
	if kind == VVOID {
		llex.SyntaxError(ls, "unexpected symbol")
	}
	// Skip body for now
	if ls.T.Token == '(' {
		llex.Next(ls)
		if ls.T.Token != ')' {
			llex.SyntaxError(ls, "function parameters expected")
		}
		llex.Next(ls)
	}
	CheckMatch(ls, llex.TK_END, llex.TK_FUNCTION, 0)
}

/*
** Return statement
 */
func returnStatement(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	// Simple return - no expressions
	Ret(fs, int(NVarStack(fs)), 0)
}

/*
** Expression statement
 */
func exprStatement(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}

	var e Expdesc
	Expr(ls, &e)

	if e.K == VCALL {
		// Function call - already handled
	} else {
		// Could be assignment
	}
}

/*
** If statement
 */
func ifStatement(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	line := ls.LastLine
	escapelist := NO_JUMP
	testThenBlock(ls, &escapelist)
	for ls.T.Token == llex.TK_ELSEIF {
		testThenBlock(ls, &escapelist)
	}
	if TestNext(ls, llex.TK_ELSE) {
		Block(ls)
	}
	CheckMatch(ls, llex.TK_END, llex.TK_IF, line)
	PatchToHere(fs, escapelist)
}

func testThenBlock(ls *llex.LexState, escapelist *int) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	// TK_IF/TK_ELSEIF already consumed by caller
	condtrue := cond(ls)
	TestNext(ls, llex.TK_THEN)
	Block(ls)
	if ls.T.Token == llex.TK_ELSE || ls.T.Token == llex.TK_ELSEIF {
		Concat(fs, escapelist, Jump(fs))
	}
	PatchToHere(fs, condtrue)
}

/*
** While statement
 */
func whileStatement(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	line := ls.LastLine
	whileinit := fs.Pc
	condexit := cond(ls)
	var bl BlockCnt
	EnterBlock(fs, &bl, 1)
	TestNext(ls, llex.TK_DO)
	Block(ls)
	PatchList(fs, Jump(fs), whileinit)
	CheckMatch(ls, llex.TK_END, llex.TK_WHILE, line)
	LeaveBlock(fs)
	PatchToHere(fs, condexit)
}

/*
** For statement
 */
func forStatement(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	line := ls.LastLine
	var bl BlockCnt
	EnterBlock(fs, &bl, 1)
	// Note: TK_FOR already consumed by Statement()
	varname := StrCheckName(ls)
	forNum(ls, varname, line)
	CheckMatch(ls, llex.TK_END, llex.TK_FOR, line)
	LeaveBlock(fs)
}

/*
** For body (numeric and generic)
 */
func forBody(ls *llex.LexState, base, line, nvars int, isgen bool) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	TestNext(ls, llex.TK_DO)
	var prep int
	if isgen {
		prep = CodeABx(fs, lopcodes.OP_TFORPREP, base, 0)
	} else {
		prep = CodeABx(fs, lopcodes.OP_FORPREP, base, 0)
	}
	fs.FreeReg--
	var bl BlockCnt
	EnterBlock(fs, &bl, 0)
	AdjustLocalVars(ls, nvars)
	ReserveRegs(fs, nvars)
	Block(ls)
	LeaveBlock(fs)
	FixForJump(fs, prep, fs.Pc)
	if isgen {
		CodeABC(fs, lopcodes.OP_TFORCALL, base, 0, nvars)
		FixLine(fs, line)
	}
	var endfor int
	if isgen {
		endfor = CodeABx(fs, lopcodes.OP_TFORLOOP, base, 0)
	} else {
		endfor = CodeABx(fs, lopcodes.OP_FORLOOP, base, 0)
	}
	FixForJump(fs, endfor, prep+1)
	FixLine(fs, line)
}

/*
** Numeric for loop
 */
func forNum(ls *llex.LexState, varname *lobject.TString, line int) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	base := fs.FreeReg
	// Create 3 internal variables: "(for state)", "(for state)", and control variable
	for i := 0; i < 3; i++ {
		if dyd, ok := fs.Ls.Dyd.(*Dyndata); ok && dyd != nil {
			dyd.ActVar.Arr = append(dyd.ActVar.Arr, Vardesc{
				Kind: VDKREG,
				Ridx: uint8(base) + uint8(i),
				Name: lstring.NewString(ls.L, ""),
			})
		}
		fs.NActVar++
	}
	TestNext(ls, '=')
	exp1(ls)
	TestNext(ls, ',')
	exp1(ls)
	if TestNext(ls, ',') {
		exp1(ls)
	} else {
		ReserveRegs(fs, 1)
		CodeABC(fs, lopcodes.OP_LOADI, int(fs.FreeReg-1), 1, 0)
	}
	AdjustLocalVars(ls, 2)
	forBody(ls, int(base), line, 1, false)
}

/*
** Generic for loop
 */
func forList(ls *llex.LexState, indexname *lobject.TString) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	nvars := 4
	base := fs.FreeReg
	fs.NActVar++
	fs.NActVar++
	fs.NActVar++
	fs.NActVar++
	for TestNext(ls, ',') {
		StrCheckName(ls)
		fs.NActVar++
		nvars++
	}
	TestNext(ls, llex.TK_IN)
	line := ls.LastLine
	var e Expdesc
	nexps := expList(ls, &e)
	adjustAssign(ls, 4, nexps, &e)
	AdjustLocalVars(ls, 3)
	ReserveRegs(fs, 2)
	forBody(ls, int(base), line, nvars-3, true)
}

/*
** Break statement
 */
func breakStatement(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	// Find enclosing loop block
	bl := fs.Bl
	for bl != nil {
		if bl.IsLoop != 0 {
			bl.IsLoop = 2 // mark as having pending breaks
			llex.Next(ls) // consume 'break'
			Jump(fs) // generate jump
			break
		}
		bl = bl.Previous
	}
}

/*
** Repeat statement
 */
func repeatStatement(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	line := ls.LastLine
	var bl BlockCnt
	EnterBlock(fs, &bl, 1)  // loop block
	// TK_REPEAT already consumed by Statement()
	Block(ls)
	CheckMatch(ls, llex.TK_UNTIL, llex.TK_REPEAT, line)
	condexit := cond(ls)
	PatchList(fs, condexit, fs.Pc)  // jump back to start
	LeaveBlock(fs) // finish loop
}

func cond(ls *llex.LexState) int {
	fs := GetFuncState(ls)
	if fs == nil {
		return NO_JUMP
	}
	var v Expdesc
	Expr(ls, &v)
	if v.K == VNIL {
		InitExp(&v, VFALSE, 0)
	}
	GoIfTrue(fs, &v)
	return v.F
}

/*
** Block parser
 */
func Block(ls *llex.LexState) {
	var bl BlockCnt
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	EnterBlock(fs, &bl, 0)
	StatList(ls)
	LeaveBlock(fs)
}

/*
** Fix line number
 */
func FixLine(fs *FuncState, line int) {
	// Simplified - line info handling
}

/*
** Set table size
 */
func SetTableSize(fs *FuncState, pc, ra, asize, hsize int) {
	CodeABC(fs, lopcodes.OP_NEWTABLE, ra, hsize, asize)
}

/*
** Set list
 */
func SetList(fs *FuncState, base, nelems, tostore int) {
	if tostore == LUA_MULTRET {
		tostore = 0
	}
	CodeABC(fs, lopcodes.OP_SETLIST, base, tostore, nelems)
}

/*
** Finish - finalize code generation
 */
func Finish(fs *FuncState) {
	// Post-processing pass
}

/*
** Ret - code return instruction
 */
func Ret(fs *FuncState, first, nret int) {
	switch nret {
	case 0:
		CodeABC(fs, lopcodes.OP_RETURN0, first, 0, 0)
	case 1:
		CodeABC(fs, lopcodes.OP_RETURN1, first, 0, 0)
	default:
		CodeABC(fs, lopcodes.OP_RETURN, first, nret+1, 0)
	}
}

/*
** Expression parser
 */
func Expr(ls *llex.LexState, v *Expdesc) {
	subExpr(ls, v, 0)
}

/*
** Subexpression parser with precedence
 */
func subExpr(ls *llex.LexState, v *Expdesc, level int) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}

	// Unary operators
	var op UnOpr
	switch ls.T.Token {
	case '-':
		op = OPR_MINUS
	case llex.TK_NOT:
		op = OPR_NOT
	case '#':
		op = OPR_LEN
	default:
		op = OPR_NOUNOPR
	}

	if op != OPR_NOUNOPR {
		llex.Next(ls)
		subExpr(ls, v, level+1)
		Prefix(fs, op, v, ls.LastLine)
	} else {
		// Simple expression
		simpleExp(ls, v)
	}

	// Binary operators
	for {
		bop := getBinOpr(ls.T.Token)
		if bop == OPR_NOBINOPR {
			break
		}
		llex.Next(ls)
		Infix(fs, bop, v)
		subExpr(ls, v, level+1)
		Posfix(fs, bop, v, v, ls.LastLine)
	}
}

/*
** Get binary operator from token
 */
func getBinOpr(token int) BinOpr {
	switch token {
	case '+':
		return OPR_ADD
	case '-':
		return OPR_SUB
	case '*':
		return OPR_MUL
	case '/':
		return OPR_DIV
	case '%':
		return OPR_MOD
	case '^':
		return OPR_POW
	case llex.TK_IDIV:
		return OPR_IDIV
	case '&':
		return OPR_BAND
	case '|':
		return OPR_BOR
	case '~':
		return OPR_BXOR
	case llex.TK_SHL:
		return OPR_SHL
	case llex.TK_SHR:
		return OPR_SHR
	case llex.TK_CONCAT, '.':
		return OPR_CONCAT
	case llex.TK_EQ, '=':
		return OPR_EQ
	case llex.TK_NE:
		return OPR_NE
	case '<':
		return OPR_LT
	case '>':
		return OPR_GT
	case llex.TK_LE:
		return OPR_LE
	case llex.TK_GE:
		return OPR_GE
	case llex.TK_AND:
		return OPR_AND
	case llex.TK_OR:
		return OPR_OR
	}
	return OPR_NOBINOPR
}

/*
** Simple expression parser
 */
func simpleExp(ls *llex.LexState, v *Expdesc) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}

	switch ls.T.Token {
	case llex.TK_INT:
		InitExp(v, VKINT, 0)
		v.U.Ival = ls.T.SemInfo.I
		llex.Next(ls)
	case llex.TK_FLT:
		InitExp(v, VKFLT, 0)
		v.U.Nval = ls.T.SemInfo.R
		llex.Next(ls)
	case llex.TK_STRING:
		CodeString(v, ls.T.SemInfo.Ts)
		llex.Next(ls)
	case llex.TK_NIL:
		InitExp(v, VNIL, 0)
		llex.Next(ls)
	case llex.TK_TRUE:
		InitExp(v, VTRUE, 0)
		llex.Next(ls)
	case llex.TK_FALSE:
		InitExp(v, VFALSE, 0)
		llex.Next(ls)
	case llex.TK_NAME:
		// IMPORTANT: Save name BEFORE calling Next
		name := ls.T.SemInfo.Ts
		llex.Next(ls)
		singlevaraux(fs, name, v, 1)
	case '{':
		llex.Next(ls)
		constructor(ls, v)
	case '(':
		llex.Next(ls)
		Expr(ls, v)
		CheckMatch(ls, ')', '(', ls.LastLine)
	case llex.TK_FUNCTION:
		llex.Next(ls)
		body(ls, v, 0, ls.LastLine)
	default:
		llex.SyntaxError(ls, "unexpected symbol")
	}
}

// Constructor parser
func constructor(ls *llex.LexState, v *Expdesc) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}

	// Create new table
	_ = CodeABx(fs, lopcodes.OP_NEWTABLE, int(fs.FreeReg), 0)
	InitExp(v, VNONRELOC, int(fs.FreeReg))
	ReserveRegs(fs, 1)

	// Parse elements
	if ls.T.Token != '}' {
		n := 1
		for ls.T.Token != '}' {
			switch ls.T.Token {
			case llex.TK_NAME:
				recfield(ls)
			case '[':
				listfield(ls)
			default:
				listfield(ls)
			}
			if ls.T.Token != ',' {
				break
			}
			llex.Next(ls)
			n++
		}
	}
	CheckMatch(ls, '}', '{', 0)
}

// List field
func listfield(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	var v Expdesc
	Expr(ls, &v)
	Exp2NextReg(fs, &v)
}

// Record field
func recfield(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	// key = value
	var key, val Expdesc
	CodeName(ls, &key)
	// skip '=' is done by CodeName
	llex.Next(ls) // skip '='
	Expr(ls, &val)
	Indexed(fs, &val, &key)
}

// Single variable
func singlevar(ls *llex.LexState, v *Expdesc) {
	singlevaraux(GetFuncState(ls), StrCheckName(ls), v, 1)
}

// Single variable aux
func singlevaraux(fs *FuncState, n *lobject.TString, v *Expdesc, base int) expkind {
	if fs == nil {
		InitExp(v, VVOID, 0)
		return VVOID
	}

	// Search local variables
	for i := int(fs.NActVar) - 1; i >= 0; i-- {
		vd := GetLocalVarDesc(fs, i)
		if vd != nil && vd.Name == n {
			v.U.Var.Ridx = vd.Ridx
			v.U.Var.Vidx = int16(i)
			InitExp(v, VLOCAL, 0)
			return VLOCAL
		}
	}

	// Search upvalues
	idx := SearchUpvalue(fs, n)
	if idx >= 0 {
		InitExp(v, VUPVAL, idx)
		return VUPVAL
	}

	// Global variable
	InitExp(v, VGLOBAL, 0)
	return VGLOBAL
}

// Function body
func body(ls *llex.LexState, v *Expdesc, isMethod, line int) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}

	// Parse parameters
	if ls.T.Token == '(' {
		llex.Next(ls)
		if ls.T.Token != ')' {
			// Parse parameters
			for ls.T.Token == llex.TK_NAME {
				// Register parameter
				llex.Next(ls)
			}
		}
		CheckMatch(ls, ')', '(', line)
	}

	StatList(ls)
	CheckMatch(ls, llex.TK_END, llex.TK_FUNCTION, line)
}

/*
** LuaY_parser - main parser entry point
 */
func LuaY_parser(L *lstate.LuaState, z *lzio.ZIO, buff *lzio.Mbuffer, name string) *lobject.LClosure {
	// Create lexer state
	ls := &llex.LexState{}
	ls.Buff = buff
	ls.Z = z
	ls.L = L

	// Create source string
	source := lstring.NewString(L, name)
	ls.Source = source

	// Initialize lexer input - read first character
	firstChar := lzio.Zgetc(z)
	llex.SetInput(L, ls, z, source, firstChar)

	// Create dynamic data
	dyd := &Dyndata{
		ActVar: struct {
			Arr  []Vardesc
			N    int
			Size int
		}{},
	}
	dyd.ActVar.Arr = make([]Vardesc, 0, 8)
	ls.Dyd = dyd

	// Create function state
	var fs FuncState
	fs.F = &lobject.Proto{
		Linedefined: 0,
		Source:      source,
		Code:        make([]lobject.LUInt32, 0, 16),
		K:           make([]lobject.TValue, 0, 8),
		Upvalues:    make([]lobject.Upvaldesc, 0, 4),
	}
	fs.Ls = ls

	// Open function
	var bl BlockCnt
	OpenFunc(ls, &fs, &bl)

	// Parse statements - get first token
	llex.Next(ls)
	StatList(ls)

	// Check for end of stream
	Check(ls, llex.TK_EOS)

	// Close function
	CloseFunc(ls)

	// Create closure
	cl := lfunc.NewLClosure(L, int(fs.Nups))
	cl.P = fs.F

	// Initialize upvalues
	lfunc.InitUpvals(L, cl)

	return cl
}

/*
** Wrapper functions for lcode compatibility
** (These are now in the same package)
 */
func lcode_Exp2NextReg(fs *FuncState, e *Expdesc) {
	Exp2NextReg(fs, e)
}

// Unused constants
const (
	_MAXSTACK   = lopcodes.MAX_STACK
	_OFFSET_sJ  = lopcodes.OFFSET_sJ
	_MAXARG_sJ  = lopcodes.MAXARG_sJ
	_MAXARG_Bx  = lopcodes.MAXARG_Bx
	_MAX_FSTACK = lopcodes.MAX_STACK
)

func fitsC(i int64) bool    { return i >= 0 && i <= lopcodes.MAXARG_C }
func fitsBx(i int64) bool  { return i >= -lopcodes.OFFSET_sBx && i <= lopcodes.MAXARG_Bx-lopcodes.OFFSET_sBx }
func numadd(a, b float64) float64 { return a + b }
func numsub(a, b float64) float64 { return a - b }
func nummul(a, b float64) float64 { return a * b }
func numdiv(a, b float64) float64 { return a / b }
func nummod(a, b float64) float64 { return math.Mod(a, b) }
func numpow(a, b float64) float64 { return math.Pow(a, b) }
func numunm(a float64) float64    { return -a }
func numisnan(a float64) bool      { return math.IsNaN(a) }

/*
** Adjust the number of local variables to nactvar
 */
func AdjustLocalVars(ls *llex.LexState, n int) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	for fs.NActVar < int16(n) {
		fs.NActVar++
	}
}

/*
** Fix a for loop jump instruction
 */
func FixForJump(fs *FuncState, pc, dest int) {
	if pc >= len(fs.F.Code) {
		return
	}
	fs.F.Code[pc] = lopcodes.CREATE_sJ(lopcodes.OP_FORLOOP, dest-pc-1, 0)
}

/*
** Parse a single expression
 */
func exp1(ls *llex.LexState) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	var e Expdesc
	Expr(ls, &e)
	Exp2NextReg(fs, &e)
}

/*
** Parse a list of expressions
 */
func expList(ls *llex.LexState, e *Expdesc) int {
	fs := GetFuncState(ls)
	if fs == nil {
		return 0
	}
	n := 1
	Expr(ls, e)
	for TestNext(ls, ',') {
		Exp2NextReg(fs, e)
		Expr(ls, e)
		n++
	}
	return n
}

/*
** Adjust assignment for multiple values
 */
func adjustAssign(ls *llex.LexState, n1, n2 int, e *Expdesc) {
	fs := GetFuncState(ls)
	if fs == nil {
		return
	}
	extra := n2 - n1
	if HasMultRet(e.K) {
		extra++
		if extra < 0 {
			extra = 0
		}
	}
	if extra > 0 {
		ReserveRegs(fs, extra)
	}
	Exp2NextReg(fs, e)
}
