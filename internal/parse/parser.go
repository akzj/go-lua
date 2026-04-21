// Parser for Lua — single-pass recursive descent compiler.
//
// This is the Go translation of C Lua's lparser.c (2202 lines).
// It parses Lua source into bytecode by calling codegen functions.
//
// Reference: lua-master/lparser.c, .analysis/06-compiler-pipeline.md §3
package parse

import (
	"fmt"

	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/lex"
	"github.com/akzj/go-lua/internal/opcode"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	maxUpval    = 255 // maximum upvalues per function
	maxCnst     = 1<<30 - 1 // max constructor elements
	unaryPriority = 12 // priority for unary operators
)

// LOOPVARKIND controls whether for-loop vars are read-only.
const loopVarKind = RDKCONST

// ---------------------------------------------------------------------------
// Forward declarations (recursive non-terminals)
// ---------------------------------------------------------------------------

// statement and expr are the two mutually-recursive entry points.
// They are defined as methods via function variables to handle the forward reference.

// ---------------------------------------------------------------------------
// Token helpers
// ---------------------------------------------------------------------------

// getFS returns the current FuncState from the LexState.
func getFS(ls *lex.LexState) *FuncState {
	return ls.FuncState.(*FuncState)
}

// testNext tests whether the current token matches c; if so, skips it.
func testNext(ls *lex.LexState, c lex.TokenType) bool {
	if ls.Token.Type == c {
		lex.Next(ls)
		return true
	}
	return false
}

// check asserts the current token is c.
func check(ls *lex.LexState, c lex.TokenType) {
	if ls.Token.Type != c {
		errorExpected(ls, c)
	}
}

// checkNext checks current token is c and consumes it.
func checkNext(ls *lex.LexState, c lex.TokenType) {
	check(ls, c)
	lex.Next(ls)
}

// checkMatch matches a closing token (end, ), ]).
func checkMatch(ls *lex.LexState, what, who lex.TokenType, where int) {
	if !testNext(ls, what) {
		if where == ls.Line {
			errorExpected(ls, what)
		} else {
			msg := fmt.Sprintf("%s expected (to close %s at line %d)",
				lex.Token2Str(what), lex.Token2Str(who), where)
			lex.SyntaxErr(ls, msg)
		}
	}
}

// strCheckName reads and returns an identifier name.
func strCheckName(ls *lex.LexState) string {
	check(ls, lex.TK_NAME)
	s := ls.Token.StrVal
	lex.Next(ls)
	return s
}

// errorExpected raises an error for an expected token.
func errorExpected(ls *lex.LexState, token lex.TokenType) {
	msg := fmt.Sprintf("%s expected", lex.Token2Str(token))
	lex.SyntaxErr(ls, msg)
}

// checkCondition checks a condition, raising a syntax error if false.
func checkCondition(ls *lex.LexState, cond bool, msg string) {
	if !cond {
		lex.SyntaxErr(ls, msg)
	}
}

// blockFollow checks if the current token can end a block.
func blockFollow(ls *lex.LexState, withUntil bool) bool {
	switch ls.Token.Type {
	case lex.TK_ELSE, lex.TK_ELSEIF, lex.TK_END, lex.TK_EOS:
		return true
	case lex.TK_UNTIL:
		return withUntil
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Expression init helpers
// ---------------------------------------------------------------------------

// initExp initializes an ExpDesc.
func initExp(e *ExpDesc, kind ExpKind, info int) {
	e.F = NoJump
	e.T = NoJump
	e.Kind = kind
	e.Info = info
}

// codeString initializes an ExpDesc as VKSTR.
func codeString(e *ExpDesc, s string) {
	e.F = NoJump
	e.T = NoJump
	e.Kind = VKSTR
	e.StrVal = s
}

// codeName reads a name and inits as VKSTR.
func codeName(ls *lex.LexState, e *ExpDesc) {
	codeString(e, strCheckName(ls))
}

// hasmultret checks if expression kind has multiple returns.
func hasmultret(k ExpKind) bool {
	return k == VCALL || k == VVARARG
}

// vkisindexed checks if expression kind is an indexed variable.
func vkisindexed(k ExpKind) bool {
	return k >= VINDEXED && k <= VINDEXSTR
}

// vkisvar checks if expression kind is a variable.
func vkisvar(k ExpKind) bool {
	return (k >= VLOCAL && k <= VINDEXSTR) || k == VVARGIND
}

// varglobal checks if a VarDesc is a global declaration.
func varglobal(vd *VarDesc) bool {
	return vd.Kind == GDKREG || vd.Kind == GDKCONST
}

// varinreg checks if a VarDesc occupies a register.
// Matches C: varinreg(v) = (v->vd.kind <= RDKTOCLOSE)
// Kinds 0-3 are in registers; 4+ (RDKCTC, GDKREG, GDKCONST) are not.
func varinreg(vd *VarDesc) bool {
	return vd.Kind <= RDKTOCLOSE
}

// ---------------------------------------------------------------------------
// Variable management
// ---------------------------------------------------------------------------

// registerLocalVar adds a local variable to Proto.LocVars (debug info).
func registerLocalVar(ls *lex.LexState, fs *FuncState, name string) int {
	f := fs.Proto
	idx := fs.NDebugVars
	f.LocVars = append(f.LocVars, object.LocVar{
		Name:    &object.LuaString{Data: name, IsShort: len(name) <= 40},
		StartPC: fs.PC,
	})
	fs.NDebugVars++
	return idx
}

// newVarKind creates a new variable with given name and kind.
func newVarKind(ls *lex.LexState, name string, kind byte) int {
	fs := getFS(ls)
	dyd := ls.DynData.(*Dyndata)
	dyd.ActVar = append(dyd.ActVar, VarDesc{Name: name, Kind: kind})
	return len(dyd.ActVar) - 1 - fs.FirstLocal
}

// newLocalVar creates a new regular local variable.
func newLocalVar(ls *lex.LexState, name string) int {
	return newVarKind(ls, name, VDKREG)
}

// newLocalVarLiteral creates a new local with a literal name.
func newLocalVarLiteral(ls *lex.LexState, name string) int {
	return newLocalVar(ls, name)
}

// getLocalVarDesc returns the VarDesc for a given variable index.
func getLocalVarDesc(fs *FuncState, vidx int) *VarDesc {
	dyd := fs.Lex.DynData.(*Dyndata)
	return &dyd.ActVar[fs.FirstLocal+vidx]
}

// regLevel returns the register level for nvar active variables.
func regLevel(fs *FuncState, nvar int16) byte {
	for nvar > 0 {
		nvar--
		vd := getLocalVarDesc(fs, int(nvar))
		if varinreg(vd) {
			return vd.RegIdx + 1
		}
	}
	return 0
}

// localDebugInfo returns the LocVar for a given variable index.
func localDebugInfo(fs *FuncState, vidx int) *object.LocVar {
	vd := getLocalVarDesc(fs, vidx)
	if !varinreg(vd) {
		return nil
	}
	idx := vd.PIdx
	if idx >= len(fs.Proto.LocVars) {
		return nil // global declaration — no LocVar entry
	}
	return &fs.Proto.LocVars[idx]
}

// initVar creates an expression representing a local variable.
func initVar(fs *FuncState, e *ExpDesc, vidx int) {
	e.F = NoJump
	e.T = NoJump
	e.Kind = VLOCAL
	e.Var.VarIdx = int16(vidx)
	e.Var.RegIdx = getLocalVarDesc(fs, vidx).RegIdx
}

// checkReadonly raises an error if assigning to a readonly variable.
func checkReadonly(ls *lex.LexState, e *ExpDesc) {
	fs := getFS(ls)
	var varname string
	switch e.Kind {
	case VCONST:
		dyd := ls.DynData.(*Dyndata)
		varname = dyd.ActVar[e.Info].Name
	case VLOCAL, VVARGVAR:
		vd := getLocalVarDesc(fs, int(e.Var.VarIdx))
		if vd.Kind != VDKREG {
			varname = vd.Name
		}
	case VUPVAL:
		up := &fs.Proto.Upvalues[e.Info]
		if up.Kind != VDKREG {
			varname = up.Name.Data
		}
	case VVARGIND:
		fs.Proto.Flag |= object.PF_VATAB
		e.Kind = VINDEXED
		fallthrough
	case VINDEXUP, VINDEXSTR, VINDEXED:
		if e.Ind.ReadOnly {
			varname = fs.Proto.Constants[e.Ind.KeyStr].StringVal().Data
		}
	case VINDEXI:
		return // integer index cannot be read-only
	}
	if varname != "" {
		SemError(ls, fmt.Sprintf("attempt to assign to const variable '%s'", varname))
	}
}

// adjustLocalVars activates nvars new local variables.
func adjustLocalVars(ls *lex.LexState, nvars int) {
	fs := getFS(ls)
	rl := regLevel(fs, fs.NumActVar)
	for i := 0; i < nvars; i++ {
		vidx := int(fs.NumActVar)
		fs.NumActVar++
		vd := getLocalVarDesc(fs, vidx)
		vd.RegIdx = rl
		rl++
		vd.PIdx = registerLocalVar(ls, fs, vd.Name)
		checkLimit(fs, int(rl), MaxVars, "local variables")
	}
}

// removeVars deactivates locals down to tolevel.
// Unlike C Lua's removevars which decrements dyd->actvar.n (a relative shrink),
// we do NOT shrink dyd.ActVar here. C Lua's solveGotos accesses entries past
// the decremented .n (relying on C's lack of bounds checking). In Go, we must
// keep those entries accessible until the function scope ends. The cleanup
// happens in closeFunc which truncates dyd.ActVar to fs.FirstLocal.
func removeVars(fs *FuncState, tolevel int16) {
	for fs.NumActVar > tolevel {
		fs.NumActVar--
		lv := localDebugInfo(fs, int(fs.NumActVar))
		if lv != nil {
			lv.EndPC = fs.PC
		}
	}
}

// ---------------------------------------------------------------------------
// Upvalue management
// ---------------------------------------------------------------------------

// searchUpvalue searches for an existing upvalue by name.
func searchUpvalue(fs *FuncState, name string) int {
	for i := 0; i < int(fs.NumUps); i++ {
		if fs.Proto.Upvalues[i].Name != nil && fs.Proto.Upvalues[i].Name.Data == name {
			return i
		}
	}
	return -1
}

// allocUpvalue allocates a new upvalue descriptor.
func allocUpvalue(fs *FuncState) *object.UpvalDesc {
	checkLimit(fs, int(fs.NumUps)+1, maxUpval, "upvalues")
	fs.Proto.Upvalues = append(fs.Proto.Upvalues, object.UpvalDesc{})
	idx := int(fs.NumUps)
	fs.NumUps++
	return &fs.Proto.Upvalues[idx]
}

// newUpvalue creates a new upvalue from a resolved variable.
func newUpvalue(fs *FuncState, name string, v *ExpDesc) int {
	up := allocUpvalue(fs)
	prev := fs.Prev
	if v.Kind == VLOCAL {
		up.InStack = true
		up.Idx = getLocalVarDesc(prev, int(v.Var.VarIdx)).RegIdx
		up.Kind = getLocalVarDesc(prev, int(v.Var.VarIdx)).Kind
	} else {
		up.InStack = false
		up.Idx = byte(v.Info)
		up.Kind = prev.Proto.Upvalues[v.Info].Kind
	}
	up.Name = &object.LuaString{Data: name, IsShort: len(name) <= 40}
	return int(fs.NumUps) - 1
}

// searchVar searches for an active variable with name n.
func searchVar(fs *FuncState, n string, v *ExpDesc) int {
	for i := int(fs.NumActVar) - 1; i >= 0; i-- {
		vd := getLocalVarDesc(fs, i)
		if varglobal(vd) {
			if vd.Name == "" { // collective declaration (*)
				if v.Info < 0 { // no previous collective?
					v.Info = fs.FirstLocal + i
				}
			} else { // named global
				if vd.Name == n { // found
					initExp(v, VGLOBAL, fs.FirstLocal+i)
					return int(VGLOBAL)
				} else if v.Info == -1 { // active preambular?
					v.Info = -2
				}
			}
		} else if vd.Name == n { // found local
			if vd.Kind == RDKCTC { // compile-time constant
				initExp(v, VCONST, fs.FirstLocal+i)
			} else {
				initVar(fs, v, i)
				if vd.Kind == RDKVAVAR {
					v.Kind = VVARGVAR
				}
			}
			return int(v.Kind)
		}
	}
	return -1 // not found
}

// markUpval marks the block where a variable was defined.
func markUpval(fs *FuncState, level int) {
	bl := fs.Block
	for bl.NumActVar > int16(level) {
		bl = bl.Prev
	}
	bl.HasUpval = true
	fs.NeedClose = true
}

// markToBeClosed marks current block as having a to-be-closed variable.
func markToBeClosed(fs *FuncState) {
	bl := fs.Block
	bl.HasUpval = true
	bl.InsideTBC = true
	fs.NeedClose = true
}

// singleVarAux recursively resolves a variable.
func singleVarAux(fs *FuncState, n string, v *ExpDesc, base bool) {
	sr := searchVar(fs, n, v)
	if sr >= 0 { // found
		if !base {
			if v.Kind == VVARGVAR {
				VaPar2Local(fs, v)
			}
			if v.Kind == VLOCAL {
				markUpval(fs, int(v.Var.VarIdx))
			}
		}
	} else { // not found at current level
		idx := searchUpvalue(fs, n)
		if idx < 0 {
			if fs.Prev != nil {
				singleVarAux(fs.Prev, n, v, false)
			}
			if v.Kind == VLOCAL || v.Kind == VUPVAL {
				idx = newUpvalue(fs, n, v)
			} else {
				return // global or constant — nothing to do
			}
		}
		initExp(v, VUPVAL, idx)
	}
}

// buildGlobal resolves a variable as _ENV[name].
func buildGlobal(ls *lex.LexState, varname string, v *ExpDesc) {
	fs := getFS(ls)
	var key ExpDesc
	initExp(v, VGLOBAL, -1)
	singleVarAux(fs, ls.EnvName, v, true)
	if v.Kind == VGLOBAL {
		SemError(ls, fmt.Sprintf("%s is global when accessing variable '%s'", ls.EnvName, varname))
	}
	Exp2AnyRegUp(fs, v)
	codeString(&key, varname)
	Indexed(fs, v, &key)
}

// buildVar resolves a variable, handling global declarations.
func buildVar(ls *lex.LexState, varname string, v *ExpDesc) {
	fs := getFS(ls)
	initExp(v, VGLOBAL, -1)
	singleVarAux(fs, varname, v, true)
	if v.Kind == VGLOBAL {
		info := v.Info
		if info == -2 {
			SemError(ls, fmt.Sprintf("variable '%s' not declared", varname))
		}
		buildGlobal(ls, varname, v)
		dyd := ls.DynData.(*Dyndata)
		if info != -1 && dyd.ActVar[info].Kind == GDKCONST {
			v.Ind.ReadOnly = true
		}
	}
}

// singleVar resolves a name to local/upvalue/global.
func singleVar(ls *lex.LexState, v *ExpDesc) {
	buildVar(ls, strCheckName(ls), v)
}

// ---------------------------------------------------------------------------
// Goto/Label management
// ---------------------------------------------------------------------------

// jumpScopeError raises an error for goto jumping into scope.
func jumpScopeError(ls *lex.LexState, gt *LabelDesc) {
	fs := getFS(ls)
	vd := getLocalVarDesc(fs, int(gt.NumActVar))
	varname := vd.Name
	if varname == "" {
		varname = "*"
	}
	SemError(ls, fmt.Sprintf("<goto %s> at line %d jumps into the scope of '%s'",
		gt.Name, gt.Line, varname))
}

// closeGoto resolves a goto to a label.
func closeGoto(ls *lex.LexState, g int, label *LabelDesc, bup bool) {
	fs := getFS(ls)
	dyd := ls.DynData.(*Dyndata)
	gt := &dyd.Gotos[g]
	if gt.NumActVar < label.NumActVar {
		jumpScopeError(ls, gt)
	}
	if gt.Close || (label.NumActVar < gt.NumActVar && bup) {
		stklevel := regLevel(fs, label.NumActVar)
		// Swap jump and close
		fs.Proto.Code[gt.PC+1] = fs.Proto.Code[gt.PC]
		fs.Proto.Code[gt.PC] = opcode.CreateABCK(opcode.OP_CLOSE, int(stklevel), 0, 0, 0)
		gt.PC++
	}
	PatchList(fs, gt.PC, label.PC)
	// Remove goto from list
	copy(dyd.Gotos[g:], dyd.Gotos[g+1:])
	dyd.Gotos = dyd.Gotos[:len(dyd.Gotos)-1]
}

// findLabel searches for an active label starting at index ilb.
func findLabel(ls *lex.LexState, name string, ilb int) *LabelDesc {
	dyd := ls.DynData.(*Dyndata)
	for i := ilb; i < len(dyd.Labels); i++ {
		if dyd.Labels[i].Name == name {
			return &dyd.Labels[i]
		}
	}
	return nil
}

// newLabelEntry adds a new label to the given list.
func newLabelEntry(ls *lex.LexState, list *[]LabelDesc, name string, line, pc int) int {
	fs := getFS(ls)
	n := len(*list)
	*list = append(*list, LabelDesc{
		Name:      name,
		Line:      line,
		NumActVar: fs.NumActVar,
		Close:     false,
		PC:        pc,
	})
	return n
}

// newGotoEntry creates a goto entry with JMP + placeholder CLOSE.
func newGotoEntry(ls *lex.LexState, name string, line int) int {
	fs := getFS(ls)
	dyd := ls.DynData.(*Dyndata)
	pc := Jump(fs)
	CodeABC(fs, opcode.OP_CLOSE, 0, 1, 0) // placeholder
	return newLabelEntry(ls, &dyd.Gotos, name, line, pc)
}

// createLabel creates a new label and solves pending gotos.
func createLabel(ls *lex.LexState, name string, line int, last bool) {
	fs := getFS(ls)
	dyd := ls.DynData.(*Dyndata)
	l := newLabelEntry(ls, &dyd.Labels, name, line, GetLabel(fs))
	if last {
		dyd.Labels[l].NumActVar = fs.Block.NumActVar
	}
}

// solveGotos resolves pending gotos when a block is closed.
func solveGotos(fs *FuncState, bl *BlockCnt) {
	ls := fs.Lex
	dyd := ls.DynData.(*Dyndata)
	outlevel := regLevel(fs, bl.NumActVar)
	igt := bl.FirstGoto
	for igt < len(dyd.Gotos) {
		gt := &dyd.Gotos[igt]
		lb := findLabel(ls, gt.Name, bl.FirstLabel)
		if lb != nil {
			closeGoto(ls, igt, lb, bl.HasUpval)
		} else {
			if bl.HasUpval && regLevel(fs, gt.NumActVar) > outlevel {
				gt.Close = true
			}
			gt.NumActVar = bl.NumActVar
			igt++
		}
	}
	dyd.Labels = dyd.Labels[:bl.FirstLabel]
}

// checkRepeated checks for duplicate labels.
func checkRepeated(ls *lex.LexState, name string) {
	fs := getFS(ls)
	lb := findLabel(ls, name, fs.FirstLabel)
	if lb != nil {
		SemError(ls, fmt.Sprintf("label '%s' already defined on line %d", name, lb.Line))
	}
}

// undefGoto raises an error for an undefined goto.
func undefGoto(ls *lex.LexState, gt *LabelDesc) {
	SemError(ls, fmt.Sprintf("no visible label '%s' for <goto> at line %d", gt.Name, gt.Line))
}

// ---------------------------------------------------------------------------
// Block/scope management
// ---------------------------------------------------------------------------

// enterBlock pushes a new block scope.
func enterBlock(fs *FuncState, bl *BlockCnt, isloop byte) {
	bl.IsLoop = isloop
	bl.NumActVar = fs.NumActVar
	dyd := fs.Lex.DynData.(*Dyndata)
	bl.FirstLabel = len(dyd.Labels)
	bl.FirstGoto = len(dyd.Gotos)
	bl.HasUpval = false
	bl.InsideTBC = fs.Block != nil && fs.Block.InsideTBC
	bl.Prev = fs.Block
	fs.Block = bl
}

// leaveBlock pops a block scope.
func leaveBlock(fs *FuncState) {
	bl := fs.Block
	ls := fs.Lex
	stklevel := regLevel(fs, bl.NumActVar)
	if bl.Prev != nil && bl.HasUpval {
		CodeABC(fs, opcode.OP_CLOSE, int(stklevel), 0, 0)
	}
	fs.FreeReg = stklevel
	removeVars(fs, bl.NumActVar)
	if bl.IsLoop == 2 { // has pending breaks
		createLabel(ls, ls.BreakName, 0, false)
	}
	solveGotos(fs, bl)
	// Now that solveGotos has processed all pending gotos (adjusting their
	// NumActVar to bl.NumActVar), it is safe to truncate dyd.ActVar.
	// removeVars deliberately does NOT truncate (solveGotos needs the entries),
	// so we do the equivalent of C Lua's dyd->actvar.n here.
	dyd := ls.DynData.(*Dyndata)
	newLen := fs.FirstLocal + int(fs.NumActVar)
	if newLen < len(dyd.ActVar) {
		dyd.ActVar = dyd.ActVar[:newLen]
	}
	if bl.Prev == nil {
		if bl.FirstGoto < len(dyd.Gotos) {
			undefGoto(ls, &dyd.Gotos[bl.FirstGoto])
		}
	}
	fs.Block = bl.Prev
}

// ---------------------------------------------------------------------------
// Function management
// ---------------------------------------------------------------------------

// addPrototype creates a nested Proto.
func addPrototype(ls *lex.LexState) *object.Proto {
	fs := getFS(ls)
	f := fs.Proto
	child := &object.Proto{}
	f.Protos = append(f.Protos, child)
	fs.NProtos++
	return child
}

// codeClosure emits OP_CLOSURE instruction.
func codeClosure(ls *lex.LexState, v *ExpDesc) {
	fs := getFS(ls).Prev
	initExp(v, VRELOC, CodeABx(fs, opcode.OP_CLOSURE, 0, fs.NProtos-1))
	Exp2NextReg(fs, v)
}

// openFunc initializes a FuncState for a new function.
func openFunc(ls *lex.LexState, fs *FuncState, bl *BlockCnt) {
	f := fs.Proto
	if ls.FuncState != nil {
		fs.Prev = ls.FuncState.(*FuncState)
	}
	fs.Lex = ls
	ls.FuncState = fs
	fs.PC = 0
	fs.PrevLine = f.LineDefined
	fs.IWthAbs = 0
	fs.LastTarget = 0
	fs.FreeReg = 0
	fs.NProtos = 0
	fs.NumUps = 0
	fs.NDebugVars = 0
	fs.NumActVar = 0
	fs.NeedClose = false
	dyd := ls.DynData.(*Dyndata)
	fs.FirstLocal = len(dyd.ActVar)
	fs.FirstLabel = len(dyd.Labels)
	fs.Block = nil
	f.Source = &object.LuaString{Data: ls.Source, IsShort: len(ls.Source) <= 40}
	f.MaxStackSize = 2 // registers 0/1 always valid
	fs.KCache = make(map[any]int)
	// Inherit or create shared string cache for the compilation unit
	if fs.Prev != nil {
		fs.StringCache = fs.Prev.StringCache
	} else {
		fs.StringCache = make(map[string]*object.LuaString)
	}
	enterBlock(fs, bl, 0)
}

// closeFunc finalizes a Proto.
func closeFunc(ls *lex.LexState) {
	fs := getFS(ls)
	f := fs.Proto
	Ret(fs, int(regLevel(fs, fs.NumActVar)), 0)
	leaveBlock(fs)
	Finish(fs)
	// Shrink slices to exact size
	f.Code = f.Code[:fs.PC]
	f.LineInfo = f.LineInfo[:fs.PC]
	f.Constants = f.Constants[:len(f.Constants)]
	f.Protos = f.Protos[:fs.NProtos]
	f.LocVars = f.LocVars[:fs.NDebugVars]
	f.Upvalues = f.Upvalues[:fs.NumUps]
	ls.FuncState = fs.Prev
}

// ===========================================================================
// GRAMMAR RULES
// ===========================================================================

// ---------------------------------------------------------------------------
// Adjust assignment
// ---------------------------------------------------------------------------

// adjustAssign adjusts multiple assignment.
func adjustAssign(ls *lex.LexState, nvars, nexps int, e *ExpDesc) {
	fs := getFS(ls)
	needed := nvars - nexps
	CheckStack(fs, needed)
	if hasmultret(e.Kind) {
		extra := needed + 1
		if extra < 0 {
			extra = 0
		}
		SetReturns(fs, e, extra)
	} else {
		if e.Kind != VVOID {
			Exp2NextReg(fs, e)
		}
		if needed > 0 {
			Nil(fs, int(fs.FreeReg), needed)
		}
	}
	if needed > 0 {
		ReserveRegs(fs, needed)
	} else {
		fs.FreeReg = byte(int(fs.FreeReg) + needed) // subtract extra
	}
}

// ---------------------------------------------------------------------------
// Table constructors
// ---------------------------------------------------------------------------

// ConsControl tracks table constructor state.
type ConsControl struct {
	V          ExpDesc  // last list item read
	T          *ExpDesc // table descriptor
	NH         int      // total hash elements
	NA         int      // total array elements already stored
	ToStore    int      // pending array elements
	MaxToStore int      // max pending before flush
}

// maxToStoreCalc computes the limit for pending elements.
func maxToStoreCalc(fs *FuncState) int {
	numfree := MaxFStack - int(fs.FreeReg)
	if numfree >= 160 {
		return numfree / 5
	} else if numfree >= 80 {
		return 10
	}
	return 1
}

func recfield(ls *lex.LexState, cc *ConsControl) {
	fs := getFS(ls)
	reg := fs.FreeReg
	var tab, key, val ExpDesc
	if ls.Token.Type == lex.TK_NAME {
		codeName(ls, &key)
	} else { // '['
		yindex(ls, &key)
	}
	cc.NH++
	checkNext(ls, '=')
	tab = *cc.T
	Indexed(fs, &tab, &key)
	expr(ls, &val)
	StoreVar(fs, &tab, &val)
	fs.FreeReg = reg
}

func closeListField(fs *FuncState, cc *ConsControl) {
	Exp2NextReg(fs, &cc.V)
	cc.V.Kind = VVOID
	if cc.ToStore >= cc.MaxToStore {
		SetList(fs, cc.T.Info, cc.NA, cc.ToStore)
		cc.NA += cc.ToStore
		cc.ToStore = 0
	}
}

func lastListField(fs *FuncState, cc *ConsControl) {
	if cc.ToStore == 0 {
		return
	}
	if hasmultret(cc.V.Kind) {
		SetReturns(fs, &cc.V, LuaMultRet)
		SetList(fs, cc.T.Info, cc.NA, LuaMultRet)
		cc.NA--
	} else {
		if cc.V.Kind != VVOID {
			Exp2NextReg(fs, &cc.V)
		}
		SetList(fs, cc.T.Info, cc.NA, cc.ToStore)
	}
	cc.NA += cc.ToStore
}

func listfield(ls *lex.LexState, cc *ConsControl) {
	expr(ls, &cc.V)
	cc.ToStore++
}

func field(ls *lex.LexState, cc *ConsControl) {
	switch ls.Token.Type {
	case lex.TK_NAME:
		if lex.Lookahead(ls) != '=' {
			listfield(ls, cc)
		} else {
			recfield(ls, cc)
		}
	case '[':
		recfield(ls, cc)
	default:
		listfield(ls, cc)
	}
}

func constructor(ls *lex.LexState, t *ExpDesc) {
	fs := getFS(ls)
	line := ls.Line
	pc := CodeVABCk(fs, opcode.OP_NEWTABLE, 0, 0, 0, 0)
	Code(fs, 0) // space for extra arg
	var cc ConsControl
	cc.NA = 0
	cc.NH = 0
	cc.ToStore = 0
	cc.T = t
	initExp(t, VNONRELOC, int(fs.FreeReg))
	ReserveRegs(fs, 1)
	initExp(&cc.V, VVOID, 0)
	checkNext(ls, '{')
	cc.MaxToStore = maxToStoreCalc(fs)
	for {
		if ls.Token.Type == '}' {
			break
		}
		if cc.V.Kind != VVOID {
			closeListField(fs, &cc)
		}
		field(ls, &cc)
		checkLimit(fs, cc.ToStore+cc.NA+cc.NH, maxCnst, "items in a constructor")
		if !testNext(ls, ',') && !testNext(ls, ';') {
			break
		}
	}
	checkMatch(ls, '}', '{', line)
	lastListField(fs, &cc)
	SetTableSize(fs, pc, t.Info, cc.NA, cc.NH)
}

// ---------------------------------------------------------------------------
// Function body and parameters
// ---------------------------------------------------------------------------

func setVararg(fs *FuncState) {
	fs.Proto.Flag |= object.PF_VAHID
	CodeABC(fs, opcode.OP_VARARGPREP, 0, 0, 0)
}

func parlist(ls *lex.LexState) {
	fs := getFS(ls)
	f := fs.Proto
	nparams := 0
	varargk := false
	if ls.Token.Type != ')' {
		for {
			switch ls.Token.Type {
			case lex.TK_NAME:
				newLocalVar(ls, strCheckName(ls))
				nparams++
			case lex.TK_DOTS:
				varargk = true
				lex.Next(ls)
				if ls.Token.Type == lex.TK_NAME {
					newVarKind(ls, strCheckName(ls), RDKVAVAR)
				} else {
					newLocalVarLiteral(ls, "(vararg table)")
				}
			default:
				lex.SyntaxErr(ls, "<name> or '...' expected")
			}
			if varargk || !testNext(ls, ',') {
				break
			}
		}
	}
	adjustLocalVars(ls, nparams)
	f.NumParams = byte(fs.NumActVar)
	if varargk {
		setVararg(fs)
		adjustLocalVars(ls, 1)
	}
	ReserveRegs(fs, int(fs.NumActVar))
}

func body(ls *lex.LexState, e *ExpDesc, ismethod bool, line int) {
	var newFS FuncState
	var bl BlockCnt
	newFS.Proto = addPrototype(ls)
	newFS.Proto.LineDefined = line
	openFunc(ls, &newFS, &bl)
	checkNext(ls, '(')
	if ismethod {
		newLocalVarLiteral(ls, "self")
		adjustLocalVars(ls, 1)
	}
	parlist(ls)
	checkNext(ls, ')')
	statList(ls)
	newFS.Proto.LastLine = ls.Line
	checkMatch(ls, lex.TK_END, lex.TK_FUNCTION, line)
	codeClosure(ls, e)
	closeFunc(ls)
}

// ---------------------------------------------------------------------------
// Expression list and function arguments
// ---------------------------------------------------------------------------

func explist(ls *lex.LexState, v *ExpDesc) int {
	n := 1
	expr(ls, v)
	for testNext(ls, ',') {
		Exp2NextReg(getFS(ls), v)
		expr(ls, v)
		n++
	}
	return n
}

func funcargs(ls *lex.LexState, f *ExpDesc) {
	fs := getFS(ls)
	var args ExpDesc
	line := ls.Line
	switch ls.Token.Type {
	case '(':
		lex.Next(ls)
		if ls.Token.Type == ')' {
			args.Kind = VVOID
		} else {
			explist(ls, &args)
			if hasmultret(args.Kind) {
				SetReturns(fs, &args, LuaMultRet)
			}
		}
		checkMatch(ls, ')', '(', line)
	case '{':
		constructor(ls, &args)
	case lex.TK_STRING:
		codeString(&args, ls.Token.StrVal)
		lex.Next(ls)
	default:
		lex.SyntaxErr(ls, "function arguments expected")
	}
	base := f.Info
	var nparams int
	if hasmultret(args.Kind) {
		nparams = LuaMultRet
	} else {
		if args.Kind != VVOID {
			Exp2NextReg(fs, &args)
		}
		nparams = int(fs.FreeReg) - (base + 1)
	}
	initExp(f, VCALL, CodeABC(fs, opcode.OP_CALL, base, nparams+1, 2))
	FixLine(fs, line)
	fs.FreeReg = byte(base + 1)
}

// ---------------------------------------------------------------------------
// Expression parsing
// ---------------------------------------------------------------------------

func primaryexp(ls *lex.LexState, v *ExpDesc) {
	switch ls.Token.Type {
	case '(':
		line := ls.Line
		lex.Next(ls)
		expr(ls, v)
		checkMatch(ls, ')', '(', line)
		DischargeVars(getFS(ls), v)
	case lex.TK_NAME:
		singleVar(ls, v)
	default:
		lex.SyntaxErr(ls, "unexpected symbol")
	}
}

func suffixedexp(ls *lex.LexState, v *ExpDesc) {
	fs := getFS(ls)
	primaryexp(ls, v)
	for {
		switch ls.Token.Type {
		case '.':
			fieldsel(ls, v)
		case '[':
			var key ExpDesc
			Exp2AnyRegUp(fs, v)
			yindex(ls, &key)
			Indexed(fs, v, &key)
		case ':':
			var key ExpDesc
			lex.Next(ls)
			codeName(ls, &key)
			Self(fs, v, &key)
			funcargs(ls, v)
		case '(', lex.TK_STRING, '{':
			Exp2NextReg(fs, v)
			funcargs(ls, v)
		default:
			return
		}
	}
}

func fieldsel(ls *lex.LexState, v *ExpDesc) {
	fs := getFS(ls)
	var key ExpDesc
	Exp2AnyRegUp(fs, v)
	lex.Next(ls) // skip dot or colon
	codeName(ls, &key)
	Indexed(fs, v, &key)
}

func yindex(ls *lex.LexState, v *ExpDesc) {
	lex.Next(ls) // skip '['
	expr(ls, v)
	Exp2Val(getFS(ls), v)
	checkNext(ls, ']')
}

func simpleexp(ls *lex.LexState, v *ExpDesc) {
	switch ls.Token.Type {
	case lex.TK_FLT:
		initExp(v, VKFLT, 0)
		v.NVal = ls.Token.FltVal
	case lex.TK_INT:
		initExp(v, VKINT, 0)
		v.IVal = ls.Token.IntVal
	case lex.TK_STRING:
		codeString(v, ls.Token.StrVal)
	case lex.TK_NIL:
		initExp(v, VNIL, 0)
	case lex.TK_TRUE:
		initExp(v, VTRUE, 0)
	case lex.TK_FALSE:
		initExp(v, VFALSE, 0)
	case lex.TK_DOTS:
		fs := getFS(ls)
		checkCondition(ls, fs.Proto.IsVararg(), "cannot use '...' outside a vararg function")
		initExp(v, VVARARG, CodeABC(fs, opcode.OP_VARARG, 0, int(fs.Proto.NumParams), 1))
	case '{':
		constructor(ls, v)
		return
	case lex.TK_FUNCTION:
		lex.Next(ls)
		body(ls, v, false, ls.Line)
		return
	default:
		suffixedexp(ls, v)
		return
	}
	lex.Next(ls)
}

// getUnOpr maps token to unary operator.
func getUnOpr(op lex.TokenType) UnOpr {
	switch op {
	case lex.TK_NOT:
		return OPR_NOT
	case '-':
		return OPR_MINUS
	case '~':
		return OPR_BNOT
	case '#':
		return OPR_LEN
	default:
		return OPR_NOUNOPR
	}
}

// getBinOpr maps token to binary operator.
func getBinOpr(op lex.TokenType) BinOpr {
	switch op {
	case '+':
		return OPR_ADD
	case '-':
		return OPR_SUB
	case '*':
		return OPR_MUL
	case '%':
		return OPR_MOD
	case '^':
		return OPR_POW
	case '/':
		return OPR_DIV
	case lex.TK_IDIV:
		return OPR_IDIV
	case '&':
		return OPR_BAND
	case '|':
		return OPR_BOR
	case '~':
		return OPR_BXOR
	case lex.TK_SHL:
		return OPR_SHL
	case lex.TK_SHR:
		return OPR_SHR
	case lex.TK_CONCAT:
		return OPR_CONCAT
	case lex.TK_NE:
		return OPR_NE
	case lex.TK_EQ:
		return OPR_EQ
	case '<':
		return OPR_LT
	case lex.TK_LE:
		return OPR_LE
	case '>':
		return OPR_GT
	case lex.TK_GE:
		return OPR_GE
	case lex.TK_AND:
		return OPR_AND
	case lex.TK_OR:
		return OPR_OR
	default:
		return OPR_NOBINOPR
	}
}

// priority table for binary operators (left, right precedence).
var priority = [...]struct{ left, right int }{
	{10, 10}, {10, 10}, // + -
	{11, 11}, {11, 11}, // * %
	{14, 13},           // ^ (right assoc)
	{11, 11}, {11, 11}, // / //
	{6, 6}, {4, 4}, {5, 5}, // & | ~
	{7, 7}, {7, 7}, // << >>
	{9, 8},    // .. (right assoc)
	{3, 3}, {3, 3}, {3, 3}, // == < <=
	{3, 3}, {3, 3}, {3, 3}, // ~= > >=
	{2, 2}, {1, 1}, // and or
}

// enterLevel increments parser nesting depth and checks for overflow.
// Mirrors: luaY_checklimit with nCcalls in C Lua's lparser.c
func enterLevel(ls *lex.LexState) {
	ls.NestLevel++
	if ls.NestLevel > ls.MaxNestLevel {
		lex.LexError(ls, "chunk has too many syntax levels", 0)
	}
}

// leaveLevel decrements parser nesting depth.
func leaveLevel(ls *lex.LexState) {
	ls.NestLevel--
}

func subexpr(ls *lex.LexState, v *ExpDesc, limit int) BinOpr {
	enterLevel(ls)
	defer leaveLevel(ls)
	uop := getUnOpr(ls.Token.Type)
	if uop != OPR_NOUNOPR {
		line := ls.Line
		lex.Next(ls)
		subexpr(ls, v, unaryPriority)
		Prefix(getFS(ls), uop, v, line)
	} else {
		simpleexp(ls, v)
	}
	op := getBinOpr(ls.Token.Type)
	for op != OPR_NOBINOPR && priority[op].left > limit {
		var v2 ExpDesc
		line := ls.Line
		lex.Next(ls)
		Infix(getFS(ls), op, v)
		nextop := subexpr(ls, &v2, priority[op].right)
		Posfix(getFS(ls), op, v, &v2, line)
		op = nextop
	}
	// leavelevel
	return op
}

func expr(ls *lex.LexState, v *ExpDesc) {
	subexpr(ls, v, 0)
}

// ===========================================================================
// STATEMENTS
// ===========================================================================

// ---------------------------------------------------------------------------
// Statement helpers
// ---------------------------------------------------------------------------

// LHSAssign chains left-hand side variables in multi-assignment.
type LHSAssign struct {
	Prev *LHSAssign
	V    ExpDesc
}

// cond parses a condition expression and returns the false-jump list.
func cond(ls *lex.LexState) int {
	var v ExpDesc
	expr(ls, &v)
	if v.Kind == VNIL {
		v.Kind = VFALSE
	}
	GoIfTrue(getFS(ls), &v)
	return v.F
}

// exp1 parses a single expression and puts its result in next register.
func exp1(ls *lex.LexState) {
	var e ExpDesc
	expr(ls, &e)
	Exp2NextReg(getFS(ls), &e)
}

// fixForJump fixes a for-loop jump instruction at pc to jump to dest.
func fixForJump(fs *FuncState, pc, dest int, back bool) {
	jmp := &fs.Proto.Code[pc]
	offset := dest - (pc + 1)
	if back {
		offset = -offset
	}
	if offset > opcode.MaxArgBx {
		lex.SyntaxErr(fs.Lex, "control structure too long")
	}
	*jmp = opcode.SetArgBx(*jmp, offset)
}

// ---------------------------------------------------------------------------
// statList — parse a list of statements
// ---------------------------------------------------------------------------

func statList(ls *lex.LexState) {
	for !blockFollow(ls, true) {
		if ls.Token.Type == lex.TK_RETURN {
			statement(ls)
			return // 'return' must be last statement
		}
		statement(ls)
	}
}

// block parses a block (enter/leave block around statlist).
func block(ls *lex.LexState) {
	fs := getFS(ls)
	var bl BlockCnt
	enterBlock(fs, &bl, 0)
	statList(ls)
	leaveBlock(fs)
}

// ---------------------------------------------------------------------------
// Assignment statements
// ---------------------------------------------------------------------------

// checkConflict checks table assignment conflicts in multi-assignment.
func checkConflict(ls *lex.LexState, lh *LHSAssign, v *ExpDesc) {
	fs := getFS(ls)
	extra := fs.FreeReg
	conflict := false
	for ; lh != nil; lh = lh.Prev {
		if vkisindexed(lh.V.Kind) {
			if lh.V.Kind == VINDEXUP {
				if v.Kind == VUPVAL && lh.V.Ind.Table == byte(v.Info) {
					conflict = true
					lh.V.Kind = VINDEXSTR
					lh.V.Ind.Table = extra
				}
			} else {
				if v.Kind == VLOCAL && lh.V.Ind.Table == v.Var.RegIdx {
					conflict = true
					lh.V.Ind.Table = extra
				}
				if lh.V.Kind == VINDEXED && v.Kind == VLOCAL &&
					lh.V.Ind.Idx == int(v.Var.RegIdx) {
					conflict = true
					lh.V.Ind.Idx = int(extra)
				}
			}
		}
	}
	if conflict {
		if v.Kind == VLOCAL {
			CodeABC(fs, opcode.OP_MOVE, int(extra), int(v.Var.RegIdx), 0)
		} else {
			CodeABC(fs, opcode.OP_GETUPVAL, int(extra), v.Info, 0)
		}
		ReserveRegs(fs, 1)
	}
}

// storeVarTop stores the top-of-stack value to a variable.
func storeVarTop(fs *FuncState, v *ExpDesc) {
	var e ExpDesc
	initExp(&e, VNONRELOC, int(fs.FreeReg)-1)
	StoreVar(fs, v, &e)
}

// restAssign recursively parses multi-assignment.
func restAssign(ls *lex.LexState, lh *LHSAssign, nvars int) {
	var e ExpDesc
	checkCondition(ls, vkisvar(lh.V.Kind), "syntax error")
	checkReadonly(ls, &lh.V)
	if testNext(ls, ',') {
		var nv LHSAssign
		nv.Prev = lh
		suffixedexp(ls, &nv.V)
		if !vkisindexed(nv.V.Kind) {
			checkConflict(ls, lh, &nv.V)
		}
		// enterlevel
		restAssign(ls, &nv, nvars+1)
		// leavelevel
	} else {
		checkNext(ls, '=')
		nexps := explist(ls, &e)
		if nexps != nvars {
			adjustAssign(ls, nvars, nexps, &e)
		} else {
			SetOneRet(getFS(ls), &e)
			StoreVar(getFS(ls), &lh.V, &e)
			return // avoid default
		}
	}
	storeVarTop(getFS(ls), &lh.V)
}

// ---------------------------------------------------------------------------
// If statement
// ---------------------------------------------------------------------------

func testThenBlock(ls *lex.LexState, escapelist *int) {
	fs := getFS(ls)
	lex.Next(ls) // skip IF or ELSEIF
	condtrue := cond(ls)
	checkNext(ls, lex.TK_THEN)
	block(ls)
	if ls.Token.Type == lex.TK_ELSE || ls.Token.Type == lex.TK_ELSEIF {
		ConcatJumps(fs, escapelist, Jump(fs))
	}
	PatchToHere(fs, condtrue)
}

func ifStat(ls *lex.LexState, line int) {
	fs := getFS(ls)
	escapelist := NoJump
	testThenBlock(ls, &escapelist)
	for ls.Token.Type == lex.TK_ELSEIF {
		testThenBlock(ls, &escapelist)
	}
	if testNext(ls, lex.TK_ELSE) {
		block(ls)
	}
	checkMatch(ls, lex.TK_END, lex.TK_IF, line)
	PatchToHere(fs, escapelist)
}

// ---------------------------------------------------------------------------
// While statement
// ---------------------------------------------------------------------------

func whileStat(ls *lex.LexState, line int) {
	fs := getFS(ls)
	lex.Next(ls) // skip WHILE
	whileinit := GetLabel(fs)
	condexit := cond(ls)
	var bl BlockCnt
	enterBlock(fs, &bl, 1)
	checkNext(ls, lex.TK_DO)
	block(ls)
	PatchList(fs, Jump(fs), whileinit)
	checkMatch(ls, lex.TK_END, lex.TK_WHILE, line)
	leaveBlock(fs)
	PatchToHere(fs, condexit)
}

// ---------------------------------------------------------------------------
// Repeat statement
// ---------------------------------------------------------------------------

func repeatStat(ls *lex.LexState, line int) {
	fs := getFS(ls)
	repeatInit := GetLabel(fs)
	var bl1, bl2 BlockCnt
	enterBlock(fs, &bl1, 1) // loop block
	enterBlock(fs, &bl2, 0) // scope block
	lex.Next(ls)          // skip REPEAT
	statList(ls)
	checkMatch(ls, lex.TK_UNTIL, lex.TK_REPEAT, line)
	condexit := cond(ls)
	leaveBlock(fs) // finish scope
	if bl2.HasUpval {
		exit := Jump(fs)
		PatchToHere(fs, condexit)
		CodeABC(fs, opcode.OP_CLOSE, int(regLevel(fs, bl2.NumActVar)), 0, 0)
		condexit = Jump(fs)
		PatchToHere(fs, exit)
	}
	PatchList(fs, condexit, repeatInit)
	leaveBlock(fs) // finish loop
}

// ---------------------------------------------------------------------------
// For statements
// ---------------------------------------------------------------------------

func forBody(ls *lex.LexState, base, line, nvars int, isgen bool) {
	var forprep, forloop opcode.OpCode
	if isgen {
		forprep = opcode.OP_TFORPREP
		forloop = opcode.OP_TFORLOOP
	} else {
		forprep = opcode.OP_FORPREP
		forloop = opcode.OP_FORLOOP
	}
	var bl BlockCnt
	fs := getFS(ls)
	checkNext(ls, lex.TK_DO)
	prep := CodeABx(fs, forprep, base, 0)
	fs.FreeReg-- // both forprep remove one register
	enterBlock(fs, &bl, 0)
	adjustLocalVars(ls, nvars)
	ReserveRegs(fs, nvars)
	block(ls)
	leaveBlock(fs)
	fixForJump(fs, prep, GetLabel(fs), false)
	if isgen {
		CodeABC(fs, opcode.OP_TFORCALL, base, 0, nvars)
		FixLine(fs, line)
	}
	endfor := CodeABx(fs, forloop, base, 0)
	fixForJump(fs, endfor, prep+1, true)
	FixLine(fs, line)
}

func forNum(ls *lex.LexState, varname string, line int) {
	fs := getFS(ls)
	base := int(fs.FreeReg)
	newLocalVarLiteral(ls, "(for state)")
	newLocalVarLiteral(ls, "(for state)")
	newVarKind(ls, varname, loopVarKind)
	checkNext(ls, '=')
	exp1(ls) // initial value
	checkNext(ls, ',')
	exp1(ls) // limit
	if testNext(ls, ',') {
		exp1(ls) // optional step
	} else {
		codeInt(fs, int(fs.FreeReg), 1)
		ReserveRegs(fs, 1)
	}
	adjustLocalVars(ls, 2)
	forBody(ls, base, line, 1, false)
}

func forList(ls *lex.LexState, indexname string) {
	fs := getFS(ls)
	var e ExpDesc
	nvars := 4 // function, state, closing, control
	base := int(fs.FreeReg)
	newLocalVarLiteral(ls, "(for state)")
	newLocalVarLiteral(ls, "(for state)")
	newLocalVarLiteral(ls, "(for state)")
	newVarKind(ls, indexname, loopVarKind)
	for testNext(ls, ',') {
		newLocalVar(ls, strCheckName(ls))
		nvars++
	}
	checkNext(ls, lex.TK_IN)
	line := ls.Line
	adjustAssign(ls, 4, explist(ls, &e), &e)
	adjustLocalVars(ls, 3)
	markToBeClosed(fs)
	CheckStack(fs, 2)
	forBody(ls, base, line, nvars-3, true)
}

func forStat(ls *lex.LexState, line int) {
	fs := getFS(ls)
	var bl BlockCnt
	enterBlock(fs, &bl, 1)
	lex.Next(ls) // skip FOR
	varname := strCheckName(ls)
	switch ls.Token.Type {
	case '=':
		forNum(ls, varname, line)
	case ',', lex.TK_IN:
		forList(ls, varname)
	default:
		lex.SyntaxErr(ls, "'=' or 'in' expected")
	}
	checkMatch(ls, lex.TK_END, lex.TK_FOR, line)
	leaveBlock(fs)
}

// ---------------------------------------------------------------------------
// Local statements
// ---------------------------------------------------------------------------

func getVarAttribute(ls *lex.LexState, df byte) byte {
	if testNext(ls, '<') {
		attr := strCheckName(ls)
		checkNext(ls, '>')
		switch attr {
		case "const":
			return RDKCONST
		case "close":
			return RDKTOCLOSE
		default:
			SemError(ls, fmt.Sprintf("unknown attribute '%s'", attr))
		}
	}
	return df
}

func checkToClose(fs *FuncState, level int) {
	if level != -1 {
		markToBeClosed(fs)
		CodeABC(fs, opcode.OP_TBC, int(regLevel(fs, int16(level))), 0, 0)
	}
}

func localFunc(ls *lex.LexState) {
	var b ExpDesc
	fs := getFS(ls)
	fvar := int(fs.NumActVar)
	newLocalVar(ls, strCheckName(ls))
	adjustLocalVars(ls, 1)
	body(ls, &b, false, ls.Line)
	localDebugInfo(fs, fvar).StartPC = fs.PC
}

func localStat(ls *lex.LexState) {
	fs := getFS(ls)
	toclose := -1
	nvars := 0
	var vidx int
	var e ExpDesc
	defkind := getVarAttribute(ls, VDKREG)
	for {
		vname := strCheckName(ls)
		kind := getVarAttribute(ls, defkind)
		vidx = newVarKind(ls, vname, kind)
		if kind == RDKTOCLOSE {
			if toclose != -1 {
				SemError(ls, "multiple to-be-closed variables in local list")
			}
			toclose = int(fs.NumActVar) + nvars
		}
		nvars++
		if !testNext(ls, ',') {
			break
		}
	}
	var nexps int
	if testNext(ls, '=') {
		nexps = explist(ls, &e)
	} else {
		e.Kind = VVOID
		nexps = 0
	}
	vd := getLocalVarDesc(fs, vidx)
	if nvars == nexps && vd.Kind == RDKCONST && Exp2Const(fs, &e, &vd.K) {
		vd.Kind = RDKCTC
		adjustLocalVars(ls, nvars-1)
		fs.NumActVar++
	} else {
		adjustAssign(ls, nvars, nexps, &e)
		adjustLocalVars(ls, nvars)
	}
	checkToClose(fs, toclose)
}

// ---------------------------------------------------------------------------
// Function statement
// ---------------------------------------------------------------------------

func funcName(ls *lex.LexState, v *ExpDesc) bool {
	ismethod := false
	singleVar(ls, v)
	for ls.Token.Type == '.' {
		fieldsel(ls, v)
	}
	if ls.Token.Type == ':' {
		ismethod = true
		fieldsel(ls, v)
	}
	return ismethod
}

func funcStat(ls *lex.LexState, line int) {
	var v, b ExpDesc
	lex.Next(ls) // skip FUNCTION
	ismethod := funcName(ls, &v)
	checkReadonly(ls, &v)
	body(ls, &b, ismethod, line)
	StoreVar(getFS(ls), &v, &b)
	FixLine(getFS(ls), line)
}

// ---------------------------------------------------------------------------
// Expression statement (call or assignment)
// ---------------------------------------------------------------------------

func exprStat(ls *lex.LexState) {
	fs := getFS(ls)
	var v LHSAssign
	suffixedexp(ls, &v.V)
	if ls.Token.Type == '=' || ls.Token.Type == ',' {
		v.Prev = nil
		restAssign(ls, &v, 1)
	} else {
		checkCondition(ls, v.V.Kind == VCALL, "syntax error")
		inst := getInstruction(fs, &v.V)
		*inst = opcode.SetArgC(*inst, 1) // call uses no results
	}
}

// ---------------------------------------------------------------------------
// Return statement
// ---------------------------------------------------------------------------

func retStat(ls *lex.LexState) {
	fs := getFS(ls)
	var e ExpDesc
	first := int(regLevel(fs, fs.NumActVar))
	var nret int
	if blockFollow(ls, true) || ls.Token.Type == ';' {
		nret = 0
	} else {
		nret = explist(ls, &e)
		if hasmultret(e.Kind) {
			SetReturns(fs, &e, LuaMultRet)
			if e.Kind == VCALL && nret == 1 && !fs.Block.InsideTBC {
				inst := getInstruction(fs, &e)
				*inst = opcode.SetOpCode(*inst, opcode.OP_TAILCALL)
			}
			nret = LuaMultRet
		} else {
			if nret == 1 {
				first = Exp2AnyReg(fs, &e)
			} else {
				Exp2NextReg(fs, &e)
			}
		}
	}
	Ret(fs, first, nret)
	testNext(ls, ';')
}

// ---------------------------------------------------------------------------
// Goto, break, label
// ---------------------------------------------------------------------------

func gotoStat(ls *lex.LexState, line int) {
	name := strCheckName(ls)
	newGotoEntry(ls, name, line)
}

func breakStat(ls *lex.LexState, line int) {
	bl := getFS(ls).Block
	for bl != nil {
		if bl.IsLoop != 0 {
			goto ok
		}
		bl = bl.Prev
	}
	lex.SyntaxErr(ls, "break outside loop")
ok:
	bl.IsLoop = 2 // signal pending breaks
	lex.Next(ls)
	newGotoEntry(ls, ls.BreakName, line)
}

func labelStat(ls *lex.LexState, name string, line int) {
	checkNext(ls, lex.TK_DBCOLON)
	for ls.Token.Type == ';' || ls.Token.Type == lex.TK_DBCOLON {
		statement(ls)
	}
	checkRepeated(ls, name)
	createLabel(ls, name, line, blockFollow(ls, false))
}

// ---------------------------------------------------------------------------
// Lua 5.5 global statements
// ---------------------------------------------------------------------------

func getGlobalAttribute(ls *lex.LexState, df byte) byte {
	kind := getVarAttribute(ls, df)
	switch kind {
	case RDKTOCLOSE:
		SemError(ls, "global variables cannot be to-be-closed")
		return kind
	case RDKCONST:
		return GDKCONST
	default:
		return kind
	}
}

func checkGlobal(ls *lex.LexState, varname string, line int) {
	fs := getFS(ls)
	var v ExpDesc
	buildGlobal(ls, varname, &v)
	k := v.Ind.KeyStr
	CodeCheckGlobal(fs, &v, k, line)
}

func initGlobal(ls *lex.LexState, nvars, firstidx, n, line int) {
	if n == nvars {
		var e ExpDesc
		nexps := explist(ls, &e)
		adjustAssign(ls, nvars, nexps, &e)
	} else {
		fs := getFS(ls)
		var v ExpDesc
		varname := getLocalVarDesc(fs, firstidx+n).Name
		buildGlobal(ls, varname, &v)
		// enterlevel
		initGlobal(ls, nvars, firstidx, n+1, line)
		// leavelevel
		checkGlobal(ls, varname, line)
		storeVarTop(fs, &v)
	}
}

func globalNames(ls *lex.LexState, defkind byte) {
	fs := getFS(ls)
	nvars := 0
	var lastidx int
	for {
		vname := strCheckName(ls)
		kind := getGlobalAttribute(ls, defkind)
		lastidx = newVarKind(ls, vname, kind)
		nvars++
		if !testNext(ls, ',') {
			break
		}
	}
	if testNext(ls, '=') {
		initGlobal(ls, nvars, lastidx-nvars+1, 0, ls.Line)
	}
	fs.NumActVar = int16(int(fs.NumActVar) + nvars)
}

func globalStat(ls *lex.LexState) {
	fs := getFS(ls)
	defkind := getGlobalAttribute(ls, GDKREG)
	if !testNext(ls, '*') {
		globalNames(ls, defkind)
	} else {
		newVarKind(ls, "", defkind)
		fs.NumActVar++
	}
}

func globalFunc(ls *lex.LexState, line int) {
	var v, b ExpDesc
	fs := getFS(ls)
	fname := strCheckName(ls)
	newVarKind(ls, fname, GDKREG)
	fs.NumActVar++
	buildGlobal(ls, fname, &v)
	body(ls, &b, false, ls.Line)
	checkGlobal(ls, fname, line)
	StoreVar(fs, &v, &b)
	FixLine(fs, line)
}

func globalStatFunc(ls *lex.LexState, line int) {
	lex.Next(ls) // skip 'global'
	if testNext(ls, lex.TK_FUNCTION) {
		globalFunc(ls, line)
	} else {
		globalStat(ls)
	}
}

// ---------------------------------------------------------------------------
// The big statement switch
// ---------------------------------------------------------------------------

func statement(ls *lex.LexState) {
	line := ls.Line
	enterLevel(ls)
	defer leaveLevel(ls)
	switch ls.Token.Type {
	case ';':
		lex.Next(ls)
	case lex.TK_IF:
		ifStat(ls, line)
	case lex.TK_WHILE:
		whileStat(ls, line)
	case lex.TK_DO:
		lex.Next(ls)
		block(ls)
		checkMatch(ls, lex.TK_END, lex.TK_DO, line)
	case lex.TK_FOR:
		forStat(ls, line)
	case lex.TK_REPEAT:
		repeatStat(ls, line)
	case lex.TK_FUNCTION:
		funcStat(ls, line)
	case lex.TK_LOCAL:
		lex.Next(ls)
		if testNext(ls, lex.TK_FUNCTION) {
			localFunc(ls)
		} else {
			localStat(ls)
		}
	case lex.TK_DBCOLON:
		lex.Next(ls)
		labelStat(ls, strCheckName(ls), line)
	case lex.TK_RETURN:
		lex.Next(ls)
		retStat(ls)
	case lex.TK_BREAK:
		breakStat(ls, line)
	case lex.TK_GOTO:
		lex.Next(ls)
		gotoStat(ls, line)
	default:
		// "global" is a context-sensitive (soft) keyword in Lua 5.5.
		// It is recognized at statement start when followed by '<', TK_NAME,
		// '*', or TK_FUNCTION, but can still be used as a variable name
		// in other positions (e.g., "global = 1").
		if ls.Token.Type == lex.TK_NAME && ls.Token.StrVal == "global" {
			lk := lex.Lookahead(ls)
			if lk == '<' || lk == lex.TK_NAME || lk == '*' || lk == lex.TK_FUNCTION {
				globalStatFunc(ls, line)
				break
			}
		}
		exprStat(ls)
	}
	fs := getFS(ls)
	fs.FreeReg = regLevel(fs, fs.NumActVar)
	// leavelevel
}

// ===========================================================================
// ENTRY POINT
// ===========================================================================

// Parse compiles Lua source code into a Proto (function prototype).
// This is the sole public API of the parser.
//
// Mirrors: luaY_parser + mainfunc in lparser.c
func Parse(source string, reader lex.LexReader) *object.Proto {
	ls := lex.NewLexState(reader, source)
	lex.SetInput(ls) // prime the lexer with first character
	// Note: shebang skipping is handled by the file loader (DoFile),
	// NOT here. load() from strings must not skip '#' lines.
	ls.EnvName = "_ENV"
	ls.BreakName = "break"
	var dyd Dyndata
	ls.DynData = &dyd

	var fs FuncState
	var bl BlockCnt
	fs.Proto = &object.Proto{}
	fs.Proto.LineDefined = 0

	openFunc(ls, &fs, &bl)
	setVararg(&fs) // main function is always vararg

	// Create _ENV as upvalue[0]
	env := allocUpvalue(&fs)
	env.InStack = true
	env.Idx = 0
	env.Kind = VDKREG
	env.Name = &object.LuaString{Data: ls.EnvName, IsShort: true}

	lex.Next(ls) // read first token
	statList(ls)
	check(ls, lex.TK_EOS)
	closeFunc(ls)

	return fs.Proto
}
