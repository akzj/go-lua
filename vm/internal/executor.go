// Package internal implements the VM execution engine.
package internal

import (
	"fmt"
	"os"
	"math"
	"unsafe"

	bcapi "github.com/akzj/go-lua/bytecode/api"
	opcodes "github.com/akzj/go-lua/opcodes/api"
	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
	vmapi "github.com/akzj/go-lua/vm/api"
)

// =============================================================================
// Constants
// =============================================================================

const (
	MAXTAGLOOP = 2000
	MAX_FSTACK = opcodes.MAXARG_A
	NO_REG     = opcodes.MAX_FSTACK
)

// =============================================================================
// Value/TValue Implementation
// =============================================================================

type Value struct {
	Variant types.ValueVariant
	Data_   interface{}
}

func (v *Value) GetGC() *types.GCObject {
	if v.Variant != types.ValueGC {
		panic("not GC")
	}
	return v.Data_.(*types.GCObject)
}

func (v *Value) GetPointer() unsafe.Pointer {
	if v.Variant != types.ValuePointer {
		panic("not pointer")
	}
	return v.Data_.(unsafe.Pointer)
}

func (v *Value) GetCFunction() unsafe.Pointer {
	if v.Variant != types.ValueCFunction {
		panic("not C function")
	}
	return v.Data_.(unsafe.Pointer)
}

func (v *Value) GetInteger() types.LuaInteger {
	if v.Variant != types.ValueInteger {
		panic("not integer")
	}
	return v.Data_.(types.LuaInteger)
}

func (v *Value) GetFloat() types.LuaNumber {
	if v.Variant != types.ValueFloat {
		panic("not float")
	}
	return v.Data_.(types.LuaNumber)
}

type TValue struct {
	Value Value
	Tt    uint8
}

func (t *TValue) IsNil() bool              { return types.Novariant(int(t.Tt)) == types.LUA_TNIL }
func (t *TValue) IsBoolean() bool           { return types.Novariant(int(t.Tt)) == types.LUA_TBOOLEAN }
func (t *TValue) IsNumber() bool            { return types.Novariant(int(t.Tt)) == types.LUA_TNUMBER }
func (t *TValue) IsInteger() bool           { return int(t.Tt) == types.LUA_VNUMINT }
func (t *TValue) IsFloat() bool             { return int(t.Tt) == types.LUA_VNUMFLT }
func (t *TValue) IsString() bool            { return types.Novariant(int(t.Tt)) == types.LUA_TSTRING }
func (t *TValue) IsTable() bool             { return int(t.Tt) == types.Ctb(int(types.LUA_VTABLE)) }
func (t *TValue) IsFunction() bool          { return types.Novariant(int(t.Tt)) == types.LUA_TFUNCTION }
func (t *TValue) IsThread() bool            { return int(t.Tt) == types.Ctb(int(types.LUA_VTHREAD)) }
func (t *TValue) IsLightUserData() bool      { return int(t.Tt) == types.LUA_VLIGHTUSERDATA }
func (t *TValue) IsUserData() bool          { return int(t.Tt) == types.Ctb(int(types.LUA_VUSERDATA)) }
func (t *TValue) IsCollectable() bool       { return int(t.Tt)&types.BIT_ISCOLLECTABLE != 0 }
func (t *TValue) IsTrue() bool              { return !t.IsNil() && !t.IsFalse() }
func (t *TValue) IsFalse() bool             { return int(t.Tt) == types.LUA_VFALSE }
func (t *TValue) IsLClosure() bool          { return int(t.Tt) == types.Ctb(int(types.LUA_VLCL)) }
func (t *TValue) IsCClosure() bool          { return int(t.Tt) == types.Ctb(int(types.LUA_VCCL)) }
func (t *TValue) IsLightCFunction() bool    { return int(t.Tt) == types.LUA_VLCF }
func (t *TValue) IsClosure() bool           { return t.IsLClosure() || t.IsCClosure() }
func (t *TValue) IsProto() bool             { return int(t.Tt) == types.Ctb(int(types.LUA_VPROTO)) }
func (t *TValue) IsUpval() bool             { return int(t.Tt) == types.Ctb(int(types.LUA_VUPVAL)) }
func (t *TValue) IsShortString() bool       { return int(t.Tt) == types.Ctb(int(types.LUA_VSHRSTR)) }
func (t *TValue) IsLongString() bool        { return int(t.Tt) == types.Ctb(int(types.LUA_VLNGSTR)) }
func (t *TValue) IsEmpty() bool             { return types.Novariant(int(t.Tt)) == types.LUA_TNIL }
func (t *TValue) GetTag() int               { return int(t.Tt) }
func (t *TValue) GetBaseType() int          { return types.Novariant(int(t.Tt)) }
func (t *TValue) GetValue() interface{}     { return t.Value.Data_ }
func (t *TValue) GetGC() *types.GCObject   { return t.Value.GetGC() }
func (t *TValue) GetInteger() types.LuaInteger { return t.Value.GetInteger() }
func (t *TValue) GetFloat() types.LuaNumber    { return t.Value.GetFloat() }
func (t *TValue) GetPointer() unsafe.Pointer   { return t.Value.GetPointer() }

// extractVariantAndData extracts variant and data from a types.TValue interface
func extractVariantAndData(v types.TValue) (types.ValueVariant, interface{}) {
	// Handle nil interface
	if v == nil {
		return types.ValueGC, nil
	}
	if v.IsInteger() {
		return types.ValueInteger, v.GetInteger()
	}
	if v.IsFloat() {
		return types.ValueFloat, v.GetFloat()
	}
	if v.IsNil() {
		return types.ValueGC, nil
	}
	// Check IsFunction BEFORE IsBoolean (goFuncWrapper returns true for both)
	if v.IsFunction() {
		return types.ValueGC, v.GetValue()
	}
	// Check boolean by type tag, not IsTrue/IsFalse (which have Lua truthiness semantics)
	if v.IsBoolean() {
		if v.IsFalse() {
			return types.ValueGC, false
		}
		return types.ValueGC, true
	}
	if v.IsTable() {
		return types.ValueGC, v.GetValue()
	}
	if v.IsLightCFunction() {
		return types.ValueCFunction, v.GetPointer()
	}
	return types.ValueGC, v.GetValue()
}

// =============================================================================
// VM Executor
// =============================================================================

// globalEnvWrapper wraps an interface value to allow pointer extraction

// goFuncUnwrapper allows VM to call GoFuncs stored in tables via goFuncWrapper.
type goFuncUnwrapper interface {
    unwrapGoFunc() vmapi.GoFunc
}
type globalEnvWrapper struct {
	env tableapi.TableInterface
}

type Executor struct {
	stack     []TValue              // Value stack (concrete internal type)
	code      []opcodes.Instruction // Bytecode instructions
	kvalues   []TValue              // Constants (K values)
	pc        int
	err       error
	frames    []*Frame
	globalEnv tableapi.TableInterface // Global environment table for variable lookups
	globalEnvPtr *globalEnvWrapper    // Pointer wrapper for lightuserdata extraction
}

type Frame struct {
	Closure  *TValue
	base     int
	prev     *Frame
	savedPC  int
	kvalues  []TValue
	upvals   []*UpVal
}

type UpVal struct {
	Value TValue
}

func (f *Frame) Base() int                     { return f.base }
func (f *Frame) Func() types.TValue           { return f.Closure }
func (f *Frame) Prev() vmapi.StackFrame       { return f.prev }
func (f *Frame) PC() int                      { return f.savedPC }
func (f *Frame) SetPC(pc int)                 { f.savedPC = pc }
func (f *Frame) Top() int                     { return f.base }

func NewExecutor() vmapi.VMExecutor {
	return &Executor{
		stack:  make([]TValue, 32),
		frames: make([]*Frame, 0),
	}
}

// SetGlobalEnv sets the global environment table for the executor
func (e *Executor) SetGlobalEnv(env tableapi.TableInterface) {
	e.globalEnv = env
	e.globalEnvPtr = &globalEnvWrapper{env: env}
}

func (e *Executor) Execute(inst opcodes.Instruction) bool {
	op := vmapi.GetOpCode(inst)
	return e.executeOp(op, inst)
}

func (e *Executor) Run() error {
	if e.err != nil {
		return e.err
	}
	for e.executeNext() {
	}
	return e.err
}

func (e *Executor) executeNext() bool {
	if len(e.frames) == 0 || e.pc >= len(e.code) {
		return false
	}
	inst := e.code[e.pc]
	e.pc++
	op := vmapi.GetOpCode(inst)
	return e.executeOp(op, inst)
}

// =============================================================================
// Opcode Execution
// =============================================================================

func (e *Executor) executeOp(op opcodes.OpCode, inst opcodes.Instruction) bool {
	switch op {
	case opcodes.OP_MOVE:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		dst := e.RA(a)
		src := e.reg(frameBase(e) + b)
		e.copyValue(dst, src)

	case opcodes.OP_LOADI:
		a := vmapi.GetArgA(inst)
		bx := vmapi.GetsBx(inst)
		e.setInteger(e.RA(a), types.LuaInteger(bx))

	case opcodes.OP_LOADF:
		a := vmapi.GetArgA(inst)
		bx := vmapi.GetsBx(inst)
		e.setFloat(e.RA(a), types.LuaNumber(bx))

	case opcodes.OP_LOADK:
		a := vmapi.GetArgA(inst)
		bx := vmapi.GetArgBx(inst)
		e.setReg(frameBase(e)+a, e.k(bx))

	case opcodes.OP_LOADKX:
		a := vmapi.GetArgA(inst)
		e.pc++
		if e.pc < len(e.code) {
			ax := vmapi.GetArgAx(e.code[e.pc-1])
			e.copyValue(e.RA(a), e.k(ax))
		}

	case opcodes.OP_LOADFALSE:
		e.setBoolean(e.reg(vmapi.GetArgA(inst)), false)

	case opcodes.OP_LFALSESKIP:
		e.setBoolean(e.reg(vmapi.GetArgA(inst)), false)
		e.pc++

	case opcodes.OP_LOADTRUE:
		e.setBoolean(e.reg(vmapi.GetArgA(inst)), true)

	case opcodes.OP_LOADNIL:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		for i := 0; i <= b; i++ {
			e.setNil(e.reg(frameBase(e) + a + i))
		}

	case opcodes.OP_GETUPVAL:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		frame := e.currentFrame()
		if frame != nil && b < len(frame.upvals) {
			e.copyValue(e.RA(a), &frame.upvals[b].Value)
		} else {
			e.setNil(e.RA(a))
		}

	case opcodes.OP_SETUPVAL:
		b := vmapi.GetArgB(inst)
		frame := e.currentFrame()
		if frame != nil && b < len(frame.upvals) {
			e.copyValue(&frame.upvals[b].Value, e.reg(vmapi.GetArgA(inst)))
		}

	case opcodes.OP_GETTABUP:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		frame := e.currentFrame()
		
		// Check if we have upvals
		hasUpvals := frame != nil && frame.upvals != nil && b < len(frame.upvals)
		
		if hasUpvals {
			// Normal path: get upval from frame
			e.finishGet(e.RA(a), &frame.upvals[b].Value, e.rk(c))
		} else if b == 0 {
			// b==0 means upval[0]/_ENV. c is raw 0-based constant index.
			if e.globalEnvPtr != nil {
				globalTValue := &TValue{
					Value: Value{Variant: types.ValuePointer, Data_: unsafe.Pointer(e.globalEnvPtr)},
					Tt:    uint8(types.LUA_VLIGHTUSERDATA),
				}
				e.finishGet(e.RA(a), globalTValue, e.k(int(c)))
			} else {
				e.setNil(e.RA(a))
			}
		} else {
			e.setNil(e.RA(a))
		}

	case opcodes.OP_GETTABLE:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		e.finishGet(e.RA(a), e.reg(frameBase(e)+b), e.rk(c))
		_ = a

	case opcodes.OP_GETI:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		e.finishGet(e.RA(a), e.reg(frameBase(e)+b), newIntValue(types.LuaInteger(c)))

	case opcodes.OP_GETFIELD:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		e.finishGet(e.RA(a), e.reg(frameBase(e)+b), e.k(c))

	case opcodes.OP_SETTABUP:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		frame := e.currentFrame()
		hasUpvals := frame != nil && frame.upvals != nil && a < len(frame.upvals)
		if hasUpvals {
			e.finishSet(&frame.upvals[a].Value, e.k(b), e.rk(c))
		} else if e.globalEnv != nil {
			// Store to globalEnv for global variables
			globalTValue := &TValue{
				Value: Value{Variant: types.ValueGC, Data_: e.globalEnv},
				Tt:    uint8(types.Ctb(int(types.LUA_VTABLE))),
			}
			e.finishSet(globalTValue, e.k(b), e.rk(c))
		}

	case opcodes.OP_SETTABLE:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		e.finishSet(e.reg(frameBase(e)+a), e.rk(b), e.rk(c))

	case opcodes.OP_SETI:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		e.finishSet(e.reg(frameBase(e)+a), newIntValue(types.LuaInteger(b)), e.rk(c))

	case opcodes.OP_SETFIELD:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		e.finishSet(e.reg(frameBase(e)+a), e.k(b), e.rk(c))

	case opcodes.OP_NEWTABLE:
		a := vmapi.GetArgA(inst)
		e.setTable(e.RA(a), tableapi.NewTable(nil))

	case opcodes.OP_SELF:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		e.copyValue(e.reg(frameBase(e) + a+1), e.reg(frameBase(e)+b))
		e.finishGet(e.RA(a), e.reg(frameBase(e)+b), e.k(c))

	// Arithmetic opcodes
	case opcodes.OP_ADDI:
		e.opArithI(inst, func(a, b types.LuaInteger) types.LuaInteger { return a + b }, func(a, b types.LuaNumber) types.LuaNumber { return a + b })

	case opcodes.OP_ADDK:
		e.opArithK(inst, func(a, b types.LuaInteger) types.LuaInteger { return a + b }, func(a, b types.LuaNumber) types.LuaNumber { return a + b })

	case opcodes.OP_SUBK:
		e.opArithK(inst, func(a, b types.LuaInteger) types.LuaInteger { return a - b }, func(a, b types.LuaNumber) types.LuaNumber { return a - b })

	case opcodes.OP_MULK:
		e.opArithK(inst, func(a, b types.LuaInteger) types.LuaInteger { return a * b }, func(a, b types.LuaNumber) types.LuaNumber { return a * b })

	case opcodes.OP_MODK:
		e.opArithK(inst, e.integerMod, e.floatMod)

	case opcodes.OP_POWK:
		e.opArithfK(inst, func(a, b types.LuaNumber) types.LuaNumber { return types.LuaNumber(math.Pow(float64(a), float64(b))) })

	case opcodes.OP_DIVK:
		e.opArithfK(inst, func(a, b types.LuaNumber) types.LuaNumber { return a / b })

	case opcodes.OP_IDIVK:
		e.opArithK(inst, e.integerDiv, func(a, b types.LuaNumber) types.LuaNumber { return types.LuaNumber(math.Floor(float64(a / b))) })

	case opcodes.OP_BANDK:
		e.opBitwiseK(inst, func(a, b types.LuaInteger) types.LuaInteger { return a & b })

	case opcodes.OP_BORK:
		e.opBitwiseK(inst, func(a, b types.LuaInteger) types.LuaInteger { return a | b })

	case opcodes.OP_BXORK:
		e.opBitwiseK(inst, func(a, b types.LuaInteger) types.LuaInteger { return a ^ b })

	case opcodes.OP_SHLI:
		e.opShiftI(inst, true)

	case opcodes.OP_SHRI:
		e.opShiftI(inst, false)

	case opcodes.OP_ADD:
		e.opArith(inst, func(a, b types.LuaInteger) types.LuaInteger { return a + b }, func(a, b types.LuaNumber) types.LuaNumber { return a + b })

	case opcodes.OP_SUB:
		e.opArith(inst, func(a, b types.LuaInteger) types.LuaInteger { return a - b }, func(a, b types.LuaNumber) types.LuaNumber { return a - b })

	case opcodes.OP_MUL:
		e.opArith(inst, func(a, b types.LuaInteger) types.LuaInteger { return a * b }, func(a, b types.LuaNumber) types.LuaNumber { return a * b })

	case opcodes.OP_MOD:
		e.opArith(inst, e.integerMod, e.floatMod)

	case opcodes.OP_POW:
		e.opArithf(inst, func(a, b types.LuaNumber) types.LuaNumber { return types.LuaNumber(math.Pow(float64(a), float64(b))) })

	case opcodes.OP_DIV:
		e.opArithf(inst, func(a, b types.LuaNumber) types.LuaNumber { return a / b })

	case opcodes.OP_IDIV:
		e.opArith(inst, e.integerDiv, func(a, b types.LuaNumber) types.LuaNumber { return types.LuaNumber(math.Floor(float64(a / b))) })

	case opcodes.OP_BAND:
		e.opBitwise(inst, func(a, b types.LuaInteger) types.LuaInteger { return a & b })

	case opcodes.OP_BOR:
		e.opBitwise(inst, func(a, b types.LuaInteger) types.LuaInteger { return a | b })

	case opcodes.OP_BXOR:
		e.opBitwise(inst, func(a, b types.LuaInteger) types.LuaInteger { return a ^ b })

	case opcodes.OP_SHL:
		e.opShift(inst, true)

	case opcodes.OP_SHR:
		e.opShift(inst, false)

	// Unary opcodes
	case opcodes.OP_UNM:
		e.opUnary(inst, func(v types.LuaInteger) types.LuaInteger { return -v }, func(v types.LuaNumber) types.LuaNumber { return -v })

	case opcodes.OP_BNOT:
		e.opUnary(inst, func(v types.LuaInteger) types.LuaInteger { return ^v }, func(v types.LuaNumber) types.LuaNumber { return -v })

	case opcodes.OP_NOT:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		e.setBoolean(e.RA(a), !e.reg(frameBase(e)+b).IsTrue())

	case opcodes.OP_LEN:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		rb := e.reg(frameBase(e) + b)
		if rb.IsTable() {
			if tbl := e.getTable(rb); tbl != nil {
				e.setInteger(e.RA(a), types.LuaInteger(tbl.Len()))
			}
		}

	// Comparison opcodes
	case opcodes.OP_EQ:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		cond := e.equalValues(e.reg(frameBase(e)+a), e.reg(frameBase(e)+b))
		if cond == vmapi.HasKBit(inst) {
			e.pc++
		}
		_ = c

	case opcodes.OP_LT:
		e.opCompare(inst, true)

	case opcodes.OP_LE:
		e.opCompareLE(inst)

	case opcodes.OP_EQK:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		cond := e.equalValues(e.reg(frameBase(e)+a), e.k(b))
		if cond == vmapi.HasKBit(inst) {
			e.pc++
		}

	case opcodes.OP_EQI:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetsBx(inst)
		ra := e.reg(frameBase(e) + a)
		cond := false
		if ra.IsInteger() {
			cond = ra.GetInteger() == types.LuaInteger(b)
		} else if ra.IsFloat() {
			cond = float64(ra.GetFloat()) == float64(b)
		}
		if cond == vmapi.HasKBit(inst) {
			e.pc++
		}

	case opcodes.OP_LTI:
		e.compareImm(inst, false)

	case opcodes.OP_LEI:
		e.compareImm(inst, true)

	case opcodes.OP_GTI:
		e.compareImmGT(inst)

	case opcodes.OP_GEI:
		e.compareImmGE(inst)

	case opcodes.OP_TEST:
		a := vmapi.GetArgA(inst)
		isFalse := !e.reg(frameBase(e) + a).IsTrue()
		// k=0: JMP if register is FALSE
		// k=1: JMP if register is TRUE
		if isFalse == vmapi.HasKBit(inst) {
			e.pc++
		}

	case opcodes.OP_TESTSET:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		rb := e.reg(frameBase(e) + b)
		isFalse := !rb.IsTrue()
		if isFalse != vmapi.HasKBit(inst) {
			e.pc++
		} else {
			e.copyValue(e.RA(a), rb)
		}

	// Control flow opcodes
	case opcodes.OP_JMP:
		sj := vmapi.GetsBx(inst)
		e.pc += sj

	case opcodes.OP_CALL, opcodes.OP_TAILCALL:
		// Execute function call
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		// b = number of arguments (0 means variable args)
		// If b == 0, args are until a nil or end of code
		// Otherwise b is the actual count
		nArgs := b
		return e.executeCall(frameBase(e)+a, nArgs, c)

	case opcodes.OP_RETURN:
		// OP_RETURN A B k — return R[A], R[A+1], ..., R[A+B-2]
		// B=1 means no return values, B=0 means return up to top
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		base := frameBase(e)

		if len(e.frames) <= 1 {
			// Returning from top-level chunk — exit VM loop
			return false
		}

		// Calculate number of return values
		nRet := 0
		if b >= 2 {
			nRet = b - 1
		}
		// b==0 (multi-return) and b==1 (no returns) both handled: nRet=0

		// The caller's frame is below the current one.
		// The caller expects results starting at the callee's base (the function slot).
		calleeBase := e.currentFrame().base

		// Copy return values to caller's expected position
		for i := 0; i < nRet; i++ {
			src := e.reg(base + a + i)
			dst := e.reg(calleeBase + i)
			*dst = *src
		}

		// Pop current frame and restore caller state
		e.frames = e.frames[:len(e.frames)-1]
		e.kvalues = e.currentFrame().kvalues
		e.pc = e.currentFrame().savedPC

		// Restore caller's bytecode
		callerLC, ok := e.currentFrame().Closure.GetValue().(luaClosure)
		if ok {
			rawCode := callerLC.GetProto().GetCode()
			code := make([]opcodes.Instruction, len(rawCode))
			for i, c := range rawCode {
				code[i] = opcodes.Instruction(c)
			}
			e.code = code
		}

	case opcodes.OP_RETURN0:
		// OP_RETURN0 — return with no values
		if len(e.frames) <= 1 {
			return false
		}

		// Pop current frame and restore caller state
		e.frames = e.frames[:len(e.frames)-1]
		e.kvalues = e.currentFrame().kvalues
		e.pc = e.currentFrame().savedPC

		// Restore caller's bytecode
		callerLC, ok := e.currentFrame().Closure.GetValue().(luaClosure)
		if ok {
			rawCode := callerLC.GetProto().GetCode()
			code := make([]opcodes.Instruction, len(rawCode))
			for i, c := range rawCode {
				code[i] = opcodes.Instruction(c)
			}
			e.code = code
		}

	case opcodes.OP_RETURN1:
		// OP_RETURN1 A — return R[A] (exactly one value)
		a := vmapi.GetArgA(inst)
		base := frameBase(e)

		if len(e.frames) <= 1 {
			return false
		}

		// Copy single return value to caller's expected position
		calleeBase := e.currentFrame().base
		src := e.reg(base + a)
		dst := e.reg(calleeBase)
		*dst = *src

		// Pop current frame and restore caller state
		e.frames = e.frames[:len(e.frames)-1]
		e.kvalues = e.currentFrame().kvalues
		e.pc = e.currentFrame().savedPC

		// Restore caller's bytecode
		callerLC, ok := e.currentFrame().Closure.GetValue().(luaClosure)
		if ok {
			rawCode := callerLC.GetProto().GetCode()
			code := make([]opcodes.Instruction, len(rawCode))
			for i, c := range rawCode {
				code[i] = opcodes.Instruction(c)
			}
			e.code = code
		}

	// For loop opcodes
	case opcodes.OP_FORPREP:
		// Prepare numeric for loop: R[A+2] -= R[A+1], then jump to FORLOOP
		// This pre-subtracts step so FORLOOP's first increment yields the correct start value
		a := vmapi.GetArgA(inst)
		base := frameBase(e)
		ra1 := e.reg(base + a + 1) // step
		ra2 := e.reg(base + a + 2) // initial index
		if ra2.IsInteger() && ra1.IsInteger() {
			setInt(ra2, getInt(ra2)-getInt(ra1))
		} else if ra2.IsFloat() || ra1.IsFloat() {
			// Handle float for loops
			var idx, step types.LuaNumber
			if ra2.IsFloat() {
				idx = getFloat(ra2)
			} else {
				idx = types.LuaNumber(getInt(ra2))
			}
			if ra1.IsFloat() {
				step = getFloat(ra1)
			} else {
				step = types.LuaNumber(getInt(ra1))
			}
			setFloat(ra2, idx-step)
		}
		e.pc += vmapi.GetsBx(inst)

	case opcodes.OP_FORLOOP:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetsBx(inst)
		ra := e.RA(a)
		ra1 := e.reg(frameBase(e) + a + 1)
		ra2 := e.reg(frameBase(e) + a + 2)
		if ra.IsInteger() && ra1.IsInteger() {
			step := getInt(ra1)
			idx := getInt(ra2)
			limit := getInt(ra)
			newIdx := idx + step
			if (step > 0 && newIdx <= limit) || (step < 0 && newIdx >= limit) {
				setInt(ra2, newIdx)
				e.pc += b
			}
		}

	case opcodes.OP_TFORPREP:
		e.pc += vmapi.GetsBx(inst)

	case opcodes.OP_TFORCALL:
		// Generic for loop: call iterator function
		// R[A+4]..R[A+3+C] := R[A](R[A+1], R[A+2])
		// A = iterator func, A+1 = state, A+2 = control variable
		// C = number of loop variables (results to produce)
		a := vmapi.GetArgA(inst)
		c := vmapi.GetArgC(inst)
		base := frameBase(e)

		// Number of results = C (the number of loop variables)
		nResults := c
		if nResults == 0 {
			nResults = 2 // default: key, value
		}

		// Copy iterator function and args to a temp call area past the loop vars
		// Temp area starts at R[A+3+nResults] to avoid clobbering loop var slots
		callBase := base + a + 3 + nResults
		e.copyValue(e.reg(callBase), e.reg(base+a))     // iterator function
		e.copyValue(e.reg(callBase+1), e.reg(base+a+1)) // invariant state
		e.copyValue(e.reg(callBase+2), e.reg(base+a+2)) // control variable

		// Call the iterator: nArgs=2 (state, control), nResults
		e.executeCall(callBase, 3, nResults)

		// Copy all results to loop variable slots R[A+3..A+3+nResults-1]
		// Also update control variable R[A+2] = first result
		if !e.reg(callBase).IsNil() {
			for i := 0; i < nResults; i++ {
				e.copyValue(e.reg(base+a+3+i), e.reg(callBase+i))
			}
			e.copyValue(e.reg(base+a+2), e.reg(callBase)) // control = first result
		} else {
			e.setNil(e.reg(base + a + 3)) // signal loop end to TFORLOOP
		}

	case opcodes.OP_TFORLOOP:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetsBx(inst)
		if !e.reg(frameBase(e) + a + 3).IsNil() {
			e.pc += b
		}

	// Table/closure opcodes
	case opcodes.OP_SETLIST:
		a := vmapi.GetArgA(inst)
		vb := int(inst>>opcodes.POS_vB) & ((1 << opcodes.SIZE_vB) - 1)
		vc := int(inst>>opcodes.POS_vC) & ((1 << opcodes.SIZE_vC) - 1)
		tbl := e.getTable(e.RA(a))
		if tbl != nil && vb > 0 {
			for i := 1; i <= vb; i++ {
				tbl.SetInt(types.LuaInteger(int(vc)+i), e.reg(frameBase(e) + a+i))
			}
		}

	case opcodes.OP_CLOSURE:
		a := vmapi.GetArgA(inst)
		bx := vmapi.GetArgBx(inst)

		// Get current frame's closure and extract its prototype
		frame := e.currentFrame()
		if frame == nil || frame.Closure == nil {
			e.err = fmt.Errorf("OP_CLOSURE: no current frame/closure")
			return false
		}
		parentLC, ok := frame.Closure.GetValue().(luaClosure)
		if !ok {
			e.err = fmt.Errorf("OP_CLOSURE: current frame closure does not implement luaClosure")
			return false
		}
		parentProto := parentLC.GetProto()
		subProtos := parentProto.GetSubProtos()
		if bx < 0 || bx >= len(subProtos) {
			e.err = fmt.Errorf("OP_CLOSURE: sub-prototype index %d out of range (have %d)", bx, len(subProtos))
			return false
		}
		subProto := subProtos[bx]

		// Create a new luaClosureImpl wrapping the sub-prototype.
		// This satisfies the luaClosure duck-type interface so executeCall can use it.
		newClosure := &luaClosureImpl{proto: subProto}

		// Set the result register to an LClosure TValue
		dst := e.reg(frameBase(e) + a)
		dst.Tt = uint8(types.Ctb(int(types.LUA_VLCL)))
		dst.Value.Variant = types.ValueGC
		dst.Value.Data_ = newClosure

	case opcodes.OP_VARARG:
		a := vmapi.GetArgA(inst)
		c := vmapi.GetArgC(inst)
		for i := 0; i < c-1; i++ {
			e.setNil(e.reg(frameBase(e) + a + i))
		}

	case opcodes.OP_GETVARG:
		e.setNil(e.reg(vmapi.GetArgA(inst)))

	case opcodes.OP_CONCAT:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		result := ""
		for i := 0; i < b; i++ {
			if r := e.reg(frameBase(e) + a + i); r.IsString() {
				result += e.toString(r)
			}
		}
		e.setString(e.RA(a), result)

	case opcodes.OP_CLOSE, opcodes.OP_TBC:
		// No-op for now

	case opcodes.OP_MMBIN, opcodes.OP_MMBINI, opcodes.OP_MMBINK:
		return false

	case opcodes.OP_ERRNNIL:
		a := vmapi.GetArgA(inst)
		if !e.RA(a).IsNil() {
			e.err = fmt.Errorf("value is not nil")
			return false
		}

	case opcodes.OP_VARARGPREP, opcodes.OP_EXTRAARG:
		// No-op

	default:
		e.err = fmt.Errorf("unknown opcode: %v", op)
		return false
	}
	return true
}

// =============================================================================
// Helper Functions
// =============================================================================

func frameBase(e *Executor) int {
	if len(e.frames) == 0 {
		return 0
	}
	return e.frames[len(e.frames)-1].base
}

func (e *Executor) currentFrame() *Frame {
	if len(e.frames) == 0 {
		return nil
	}
	return e.frames[len(e.frames)-1]
}

func (e *Executor) reg(pos int) *TValue {
	for len(e.stack) <= pos {
		e.stack = append(e.stack, TValue{})
	}
	return &e.stack[pos]
}

// RA returns a pointer to register A relative to the current frame base.
// Use this for ALL instruction register operands in the main dispatch loop.
func (e *Executor) RA(a int) *TValue {
	return e.reg(frameBase(e) + a)
}

func (e *Executor) setReg(pos int, val *TValue) {
	for len(e.stack) <= pos {
		e.stack = append(e.stack, TValue{})
	}
	e.stack[pos] = *val
}

func (e *Executor) k(idx int) *TValue {
	frame := e.currentFrame()
	if frame != nil && idx >= 0 && idx < len(frame.kvalues) {
		return &frame.kvalues[idx]
	}
	return &TValue{}
}

func (e *Executor) rk(idx int) *TValue {
	if idx <= opcodes.MAXINDEXRK {
		return e.reg(frameBase(e) + idx)
	}
	return e.k(idx - opcodes.MAXINDEXRK - 1)
}

func (e *Executor) copyValue(dst, src *TValue) {
	*dst = *src
}

func (e *Executor) setNil(dst *TValue) {
	dst.Tt = uint8(types.LUA_VNIL)
}

func (e *Executor) setBuiltinPrint(dst *TValue) {
	// Create a marker for builtin print function
	dst.Tt = uint8(types.LUA_VLCF) // Light C function marker
	dst.Value.Variant = types.ValueCFunction
	dst.Value.Data_ = unsafe.Pointer(printBuiltin)
}

// luaClosure is a duck-type interface satisfied by types/internal.LClosure
// and by luaClosureImpl (created by OP_CLOSURE in the VM).
// It lets vm/internal extract the prototype without importing types/internal.
type luaClosure interface {
	GetProto() bcapi.Prototype
}

// luaClosureImpl is a VM-local closure wrapper that satisfies the luaClosure
// duck-type interface. Created by OP_CLOSURE when the VM needs to instantiate
// a new Lua closure from a sub-prototype.
type luaClosureImpl struct {
	proto  bcapi.Prototype
	upvals []*UpVal
}

func (c *luaClosureImpl) GetProto() bcapi.Prototype { return c.proto }

// GoFunc is the duck-type interface for Go functions callable from the VM.
// Implemented by state/internal when registering base library functions.
// Receives the VM's stack and base position, returns number of results pushed.
type GoFunc func(stack []TValue, base int) int

// executeCall handles function calls (LClosure, CClosure, LightCFunction).
func (e *Executor) executeCall(base, nArgs, nResults int) bool {
	fn := e.reg(base)
		if fn.IsNil() {
		e.err = fmt.Errorf("attempt to call nil value")
		return false
	}

	// Handle LightCFunction (raw C function pointer, e.g. builtin print)
	if fn.IsLightCFunction() {
		// fn.GetValue() returns Value.Data_. For setGlobal, this is the
		// unsafe.Pointer value that was stored directly.
		ptr := fn.GetValue()
		if ptr == nil {
			e.err = fmt.Errorf("attempt to call nil light C function")
			return false
		}
		rawPtr, ok := ptr.(unsafe.Pointer)
		if !ok {
			e.err = fmt.Errorf("light C function has invalid pointer type")
			return false
		}
		if rawPtr == unsafe.Pointer(printBuiltin) {
			e.builtinPrint(base, nArgs)
			return true
		}
		// Dereference the pointer to get the GoFunc interface{}.
		// setGlobal stores: NewTValueLightCFunction(unsafe.Pointer(fn))
		// So fn.GetValue() == unsafe.Pointer(fn) == &fn_variable.
		// Dereferencing: *(*interface{})(rawPtr) gives us fn_variable.
		gf := *(*interface{})(rawPtr)
		if f, ok := gf.(GoFunc); ok {
			f(e.stack, base)
			return true
		}
		e.err = fmt.Errorf("attempt to call non-Go-function light C function")
		return false
	}

	// Handle CClosure (Go function with potential upvalues)
	if fn.IsCClosure() {
		// CClosure stores its function value in a wrapper accessed via fn.GetValue()
		// Try GoFunc duck-type from the stored value
		if gf, ok := fn.GetValue().(GoFunc); ok {
			gf(e.stack, base)
			return true
		}
		e.err = fmt.Errorf("attempt to call non-Go-function CClosure")
		return false
	}

	// Handle Lua closures (LClosure)
	if fn.IsLClosure() {
		val := fn.GetValue()

		// Check for vm/api.GoFunc (from goFuncWrapper via setGlobal).
		// This uses []types.TValue, not the internal GoFunc type.
		if apiFunc, ok := val.(vmapi.GoFunc); ok {
			// Bridge: convert []TValue to []types.TValue for the call.
			args := make([]types.TValue, nArgs+1) // +1 for the function itself
			for i := 0; i <= nArgs; i++ {
				args[i] = e.reg(base + i)
			}
			nRet := apiFunc(args, 0)
			// Copy results back from args to VM stack.
			// GoFuncs write results starting at args[0] (= stack[base]).
			for i := 0; i < nRet; i++ {
				result := args[i]
				dst := e.reg(base + i)
				variant, data := extractVariantAndData(result)
				dst.Tt = uint8(result.GetTag())
				dst.Value.Variant = variant
				dst.Value.Data_ = data
			}
			// Clear remaining slots if caller expects more results
			if nResults > nRet && nResults != -1 {
				for i := nRet; i < nResults; i++ {
					dst := e.reg(base + i)
					dst.Tt = uint8(types.LUA_TNIL)
					dst.Value.Variant = types.ValueGC
					dst.Value.Data_ = nil
				}
			}
			return true
		}

		// Check for internal GoFunc (direct GoFunc storage)
		if gf, ok := val.(GoFunc); ok {
			gf(e.stack, base)
			return true
		}

		// Check for goFuncWrapper (GoFunc stored in module tables like string, math).
		// fn.GetValue() returns *goFuncWrapper. We need to unwrap it.
		if unwrapper, ok := val.(goFuncUnwrapper); ok {
			apiFunc := unwrapper.unwrapGoFunc()
			// Bridge: convert []TValue to []types.TValue for the call.
			args := make([]types.TValue, nArgs+1)
			for i := 0; i <= nArgs; i++ {
				args[i] = e.reg(base + i)
			}
			nRet := apiFunc(args, 0)
			for i := 0; i < nRet; i++ {
				result := args[i]
				dst := e.reg(base + i)
				variant, data := extractVariantAndData(result)
				dst.Tt = uint8(result.GetTag())
				dst.Value.Variant = variant
				dst.Value.Data_ = data
			}
			if nResults > nRet && nResults != -1 {
				for i := nRet; i < nResults; i++ {
					dst := e.reg(base + i)
					dst.Tt = uint8(types.LUA_TNIL)
					dst.Value.Variant = types.ValueGC
					dst.Value.Data_ = nil
				}
			}
			return true
		}

		// Otherwise it's a real Lua closure
		lc, ok := val.(luaClosure)
		if !ok {
			e.err = fmt.Errorf("LClosure value does not implement luaClosure interface")
			return false
		}
		proto := lc.GetProto()

		// Save current PC so we can resume here after return
		e.currentFrame().savedPC = e.pc

		// Build kvalues from prototype constants
		kvals := convertProtoConstants(proto)

		// Push new frame
		newFrame := &Frame{
			Closure: fn,
			base:    base,
			prev:    e.currentFrame(),
			savedPC: 0,
			kvalues: kvals,
		}
		e.frames = append(e.frames, newFrame)

		// Switch to new closure's bytecode
		rawCode := proto.GetCode()
		code := make([]opcodes.Instruction, len(rawCode))
		for i, c := range rawCode {
			code[i] = opcodes.Instruction(c)
		}
		e.code = code
		e.pc = 0
		e.kvalues = newFrame.kvalues

		return true
	}

	e.err = fmt.Errorf("attempt to call value of type %d", fn.GetBaseType())
	return false
}

// convertProtoConstants converts a Prototype's constant pool to executor-local []TValue.
func convertProtoConstants(proto bcapi.Prototype) []TValue {
	consts := proto.GetConstants()
	kvals := make([]TValue, len(consts))
	for i, c := range consts {
		kvals[i] = bcConstantToTValue(c)
	}
	return kvals
}

func bcConstantToTValue(c *bcapi.Constant) TValue {
	switch c.Type {
	case bcapi.ConstNil:
		return TValue{Tt: uint8(types.LUA_VNIL)}
	case bcapi.ConstInteger:
		return TValue{Value: Value{Variant: types.ValueInteger, Data_: types.LuaInteger(c.Int)}, Tt: uint8(types.LUA_VNUMINT)}
	case bcapi.ConstFloat:
		return TValue{Value: Value{Variant: types.ValueFloat, Data_: types.LuaNumber(c.Float)}, Tt: uint8(types.LUA_VNUMFLT)}
	case bcapi.ConstString:
		return TValue{Value: Value{Variant: types.ValueGC, Data_: c.Str}, Tt: uint8(types.Ctb(int(types.LUA_VSHRSTR)))}
	case bcapi.ConstBool:
		tt := uint8(types.LUA_VFALSE)
		if c.Int != 0 {
			tt = uint8(types.LUA_VTRUE)
		}
		return TValue{Tt: tt}
	}
	return TValue{Tt: uint8(types.LUA_VNIL)}
}

// builtinPrint implements the print function
func (e *Executor) builtinPrint(base, nArgs int) {
	// nArgs includes function slot, so actual args start at base+1
	// We need to figure out how many args were actually passed
	// In Lua, we count until we hit a nil or reach the end
	numArgs := nArgs - 1 // nArgs includes the function itself
	if numArgs < 1 {
		fmt.Println()
		os.Stdout.Sync()
		return
	}
	
	// Print arguments separated by tabs
	for i := 0; i < numArgs; i++ {
		pos := base + 1 + i
		if pos >= len(e.stack) {
			fmt.Print("nil")
		} else {
			arg := &e.stack[pos]
			if arg.IsNil() {
				fmt.Print("nil")
			} else if arg.IsInteger() {
				fmt.Print(arg.GetInteger())
			} else if arg.IsFloat() {
				fmt.Print(float64(arg.GetFloat()))
			} else if arg.IsString() {
				if s, ok := arg.GetValue().(string); ok {
					fmt.Print(s)
				}
			} else if arg.IsTrue() {
				fmt.Print("true")
			} else if arg.IsFalse() {
				fmt.Print("false")
			} else if arg.IsTable() {
				// Print table as table: 0xXXXXXXX
			} else if arg.IsLightUserData() {
			} else if arg.IsFunction() {
			} else {
				// Fallback for other types - print type name
				fmt.Print("unknown")
			}
		}
		if i < numArgs-1 {
			fmt.Print("\t")
		}
	}
	fmt.Println()
		os.Stdout.Sync()
}

// printBuiltin is the actual print function implementation
var printBuiltin uintptr

func init() {
	// Initialize the print builtin function pointer
	printBuiltin = 1 // Non-zero marker for builtin
}

func (e *Executor) setBoolean(dst *TValue, b bool) {
	if b {
		dst.Tt = uint8(types.LUA_VTRUE)
	} else {
		dst.Tt = uint8(types.LUA_VFALSE)
	}
}

func (e *Executor) setInteger(dst *TValue, i types.LuaInteger) {
	dst.Tt = uint8(types.LUA_VNUMINT)
	dst.Value.Variant = types.ValueInteger
	dst.Value.Data_ = i
}

func (e *Executor) setFloat(dst *TValue, n types.LuaNumber) {
	dst.Tt = uint8(types.LUA_VNUMFLT)
	dst.Value.Variant = types.ValueFloat
	dst.Value.Data_ = n
}

func (e *Executor) setString(dst *TValue, s string) {
	dst.Tt = uint8(types.Ctb(int(types.LUA_VSHRSTR)))
	dst.Value.Variant = types.ValueGC
	dst.Value.Data_ = s
}

func (e *Executor) setTable(dst *TValue, tbl tableapi.TableInterface) {
	dst.Tt = uint8(types.Ctb(int(types.LUA_VTABLE)))
	dst.Value.Variant = types.ValueGC
	dst.Value.Data_ = tbl
}

func (e *Executor) getTable(tval *TValue) tableapi.TableInterface {
	if tval.IsTable() {
		if impl, ok := tval.GetValue().(tableapi.TableInterface); ok {
			return impl
		}
	}
	return nil
}

func (e *Executor) toString(tval *TValue) string {
	if tval.IsString() {
		if s, ok := tval.Value.Data_.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", tval.Value.Data_)
	}
	if tval.IsInteger() {
		return fmt.Sprintf("%d", tval.GetInteger())
	}
	if tval.IsFloat() {
		return fmt.Sprintf("%g", tval.GetFloat())
	}
	if tval.IsNil() {
		return "nil"
	}
	if tval.IsFalse() {
		return "false"
	}
	if tval.IsBoolean() {
		return "true"
	}
	return fmt.Sprintf("%v", tval.Value.Data_)
}

func (e *Executor) finishGet(ra, t, key *TValue) {
	// Handle lightuserdata (globalEnv pointer stored as LUA_VLIGHTUSERDATA)
	if t.IsLightUserData() {
		if ptr := t.GetPointer(); ptr != nil {
			// Cast to globalEnvWrapper and extract the table
			wrapper := (*globalEnvWrapper)(ptr)
			if wrapper != nil && wrapper.env != nil {
				tval := &TValue{
					Value: Value{Variant: types.ValueGC, Data_: wrapper.env},
					Tt:    uint8(types.Ctb(int(types.LUA_VTABLE))),
				}
				tbl2 := e.getTable(tval)
				if tbl2 != nil {
					result := tbl2.Get(key)
					if result == nil {
						e.setNil(ra)
						return
					}
					// result is *table/internal.TValue which implements types.TValue
					// Use extractVariantAndData to get the data
					variant, data := extractVariantAndData(result)
					ra.Tt = uint8(result.GetTag())
					ra.Value.Variant = variant
					ra.Value.Data_ = data
					return
				}
			}
		}
		e.setNil(ra)
		return
	}

	if !t.IsTable() {
		e.setNil(ra)
		return
	}
	if tbl := e.getTable(t); tbl != nil {
		result := tbl.Get(key)
		if rv, ok := result.(*TValue); ok {
			e.copyValue(ra, rv)
		} else {
			// Wrap interface in concrete
			variant, data := extractVariantAndData(result)
			ra.Tt = uint8(result.GetTag())
			ra.Value.Variant = variant
			ra.Value.Data_ = data
		}
	} else {
		e.setNil(ra)
	}
}

func (e *Executor) finishSet(t, key, value *TValue) {

	if !t.IsTable() {
		return
	}
	if tbl := e.getTable(t); tbl != nil {
		tbl.Set(key, value)
	} else {
	}
}

// =============================================================================
// Arithmetic Operations
// =============================================================================

func (e *Executor) opArithI(inst opcodes.Instruction, iop func(a, b types.LuaInteger) types.LuaInteger, fop func(a, b types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	sc := int(c)
	if sc >= 1<<(opcodes.SIZE_C-1) {
		sc -= 1 << opcodes.SIZE_C
	}
	rb := e.reg(frameBase(e) + b)
	ra := e.reg(a)
	if rb.IsInteger() {
		e.setInteger(ra, iop(rb.GetInteger(), types.LuaInteger(sc)))
	} else if rb.IsFloat() {
		e.setFloat(ra, fop(rb.GetFloat(), types.LuaNumber(sc)))
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opArith(inst opcodes.Instruction, iop func(a, b types.LuaInteger) types.LuaInteger, fop func(a, b types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.reg(a)
	rb := e.reg(frameBase(e) + b)
	rc := e.reg(frameBase(e) + c)
	if rb.IsInteger() && rc.IsInteger() {
		e.setInteger(ra, iop(rb.GetInteger(), rc.GetInteger()))
	} else {
		e.setFloat(ra, fop(getFloat(rb), getFloat(rc)))
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opArithK(inst opcodes.Instruction, iop func(a, b types.LuaInteger) types.LuaInteger, fop func(a, b types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.reg(a)
	rb := e.reg(frameBase(e) + b)
	kc := e.k(c)
	if rb.IsInteger() && kc.IsInteger() {
		e.setInteger(ra, iop(rb.GetInteger(), kc.GetInteger()))
	} else {
		e.setFloat(ra, fop(getFloat(rb), getFloat(kc)))
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opArithfK(inst opcodes.Instruction, fop func(a, b types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.reg(a)
	e.setFloat(ra, fop(getFloat(e.reg(frameBase(e)+b)), getFloat(e.k(c))))
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opArithf(inst opcodes.Instruction, fop func(a, b types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.reg(a)
	e.setFloat(ra, fop(getFloat(e.reg(frameBase(e)+b)), getFloat(e.reg(frameBase(e)+c))))
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opBitwise(inst opcodes.Instruction, op func(a, b types.LuaInteger) types.LuaInteger) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.reg(a)
	rb := e.reg(frameBase(e) + b)
	rc := e.reg(frameBase(e) + c)
	if rb.IsInteger() && rc.IsInteger() {
		e.setInteger(ra, op(rb.GetInteger(), rc.GetInteger()))
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opBitwiseK(inst opcodes.Instruction, op func(a, b types.LuaInteger) types.LuaInteger) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.reg(a)
	rb := e.reg(frameBase(e) + b)
	kc := e.k(c)
	if rb.IsInteger() && kc.IsInteger() {
		e.setInteger(ra, op(rb.GetInteger(), kc.GetInteger()))
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opShiftI(inst opcodes.Instruction, left bool) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	sc := int(c)
	if sc >= 1<<(opcodes.SIZE_C-1) {
		sc -= 1 << opcodes.SIZE_C
	}
	ra := e.reg(a)
	rb := e.reg(frameBase(e) + b)
	if rb.IsInteger() {
		ib := rb.GetInteger()
		if left {
			e.setInteger(ra, ib<<types.LuaInteger(sc))
		} else {
			e.setInteger(ra, ib>>types.LuaInteger(sc))
		}
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opShift(inst opcodes.Instruction, left bool) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.reg(a)
	rb := e.reg(frameBase(e) + b)
	rc := e.reg(frameBase(e) + c)
	if rb.IsInteger() && rc.IsInteger() {
		ib := rb.GetInteger()
		ic := rc.GetInteger()
		if left {
			e.setInteger(ra, ib<<ic)
		} else {
			e.setInteger(ra, ib>>ic)
		}
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opUnary(inst opcodes.Instruction, iop func(v types.LuaInteger) types.LuaInteger, fop func(v types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	ra := e.reg(a)
	rb := e.reg(frameBase(e) + b)
	if rb.IsInteger() {
		e.setInteger(ra, iop(rb.GetInteger()))
	} else if rb.IsFloat() {
		e.setFloat(ra, fop(rb.GetFloat()))
	}
	// Note: pc++ removed - executeNext() already increments pc
}

// =============================================================================
// Comparison Operations
// =============================================================================

func (e *Executor) opCompare(inst opcodes.Instruction, lessThan bool) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.reg(frameBase(e) + a)
	rc := e.reg(frameBase(e) + c)
	var cond bool
	if lessThan {
		cond = e.lessThan(ra, rc)
	} else {
		cond = e.equalValues(ra, rc)
	}
	if cond == vmapi.HasKBit(inst) {
		e.pc++
	}
	_ = b
}

func (e *Executor) opCompareLE(inst opcodes.Instruction) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.reg(frameBase(e) + a)
	rc := e.reg(frameBase(e) + c)
	if e.lessEqual(ra, rc) != vmapi.HasKBit(inst) {
		e.pc++
	}
	_ = b
	_ = a
}

func (e *Executor) compareImm(inst opcodes.Instruction, lessEqual bool) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetsBx(inst)
	ra := e.reg(frameBase(e) + a)
	cond := e.lessThanInt(ra, b)
	if lessEqual {
		cond = e.lessEqualInt(ra, b)
	}
	if cond != vmapi.HasKBit(inst) {
		e.pc++
	}
}

func (e *Executor) compareImmGT(inst opcodes.Instruction) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetsBx(inst)
	ra := e.reg(frameBase(e) + a)
	cond := e.greaterThanInt(ra, b)
	if cond != vmapi.HasKBit(inst) {
		e.pc++
	}
}

func (e *Executor) compareImmGE(inst opcodes.Instruction) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetsBx(inst)
	ra := e.reg(frameBase(e) + a)
	cond := e.greaterEqualInt(ra, b)
	if cond != vmapi.HasKBit(inst) {
		e.pc++
	}
}

// numToFloat64 converts any numeric TValue to float64.
// Handles both integer and float variants safely.
func numToFloat64(v types.TValue) float64 {
	if v.IsInteger() {
		return float64(v.GetInteger())
	}
	if v.IsFloat() {
		return float64(v.GetFloat())
	}
	return 0
}

func (e *Executor) lessThan(a, b *TValue) bool {
	if a.IsInteger() && b.IsInteger() {
		return a.GetInteger() < b.GetInteger()
	}
	if a.IsNumber() && b.IsNumber() {
		return numToFloat64(a) < numToFloat64(b)
	}
	if a.IsString() && b.IsString() {
		sa, _ := a.GetValue().(string)
		sb, _ := b.GetValue().(string)
		return sa < sb
	}
	return false
}

func (e *Executor) lessEqual(a, b types.TValue) bool {
	if a.IsInteger() && b.IsInteger() {
		return a.GetInteger() <= b.GetInteger()
	}
	if a.IsNumber() && b.IsNumber() {
		return numToFloat64(a) <= numToFloat64(b)
	}
	if a.IsString() && b.IsString() {
		sa, _ := a.GetValue().(string)
		sb, _ := b.GetValue().(string)
		return sa <= sb
	}
	return false
}

func (e *Executor) equalValues(a, b *TValue) bool {
	if a.IsNil() && b.IsNil() {
		return true
	}
	if a.IsNil() || b.IsNil() {
		return false
	}
	if a.IsInteger() && b.IsInteger() {
		return a.GetInteger() == b.GetInteger()
	}
	// Mixed int/float comparison
	if a.IsNumber() && b.IsNumber() {
		return numToFloat64(a) == numToFloat64(b)
	}
	if a.IsBoolean() && b.IsBoolean() {
		return a.IsTrue() == b.IsTrue()
	}
	if a.IsString() && b.IsString() {
		sa, _ := a.GetValue().(string)
		sb, _ := b.GetValue().(string)
		return sa == sb
	}
	return false
}

func (e *Executor) lessThanInt(a *TValue, b int) bool {
	if a.IsInteger() {
		return a.GetInteger() < types.LuaInteger(b)
	}
	if a.IsFloat() {
		return float64(a.GetFloat()) < float64(b)
	}
	return false
}

func (e *Executor) lessEqualInt(a *TValue, b int) bool {
	if a.IsInteger() {
		return a.GetInteger() <= types.LuaInteger(b)
	}
	if a.IsFloat() {
		return float64(a.GetFloat()) <= float64(b)
	}
	return false
}

func (e *Executor) greaterThanInt(a *TValue, b int) bool {
	if a.IsInteger() {
		return a.GetInteger() > types.LuaInteger(b)
	}
	if a.IsFloat() {
		return float64(a.GetFloat()) > float64(b)
	}
	return false
}

func (e *Executor) greaterEqualInt(a *TValue, b int) bool {
	if a.IsInteger() {
		return a.GetInteger() >= types.LuaInteger(b)
	}
	if a.IsFloat() {
		return float64(a.GetFloat()) >= float64(b)
	}
	return false
}

// =============================================================================
// Math Helpers
// =============================================================================

func (e *Executor) integerMod(m, n types.LuaInteger) types.LuaInteger {
	if n == 0 {
		e.err = fmt.Errorf("modulo by zero")
		return 0
	}
	r := m % n
	if r != 0 && (r^n) < 0 {
		r += n
	}
	return r
}

func (e *Executor) floatMod(m, n types.LuaNumber) types.LuaNumber {
	return types.LuaNumber(math.Mod(float64(m), float64(n)))
}

func (e *Executor) integerDiv(m, n types.LuaInteger) types.LuaInteger {
	if n == 0 {
		e.err = fmt.Errorf("division by zero")
		return 0
	}
	q := m / n
	if (m^n) < 0 && m%n != 0 {
		q -= 1
	}
	return q
}

// =============================================================================
// Value Extractors
// =============================================================================

func getInt(tval *TValue) types.LuaInteger {
	if tval.IsInteger() {
		return tval.GetInteger()
	}
	if tval.IsFloat() {
		return types.LuaInteger(tval.GetFloat())
	}
	return 0
}

func getFloat(tval *TValue) types.LuaNumber {
	if tval.IsInteger() {
		return types.LuaNumber(tval.GetInteger())
	}
	if tval.IsFloat() {
		return tval.GetFloat()
	}
	return 0
}

func setInt(tval *TValue, i types.LuaInteger) {
	tval.Tt = uint8(types.LUA_VNUMINT)
	tval.Value.Variant = types.ValueInteger
	tval.Value.Data_ = i
}

func setFloat(tval *TValue, n types.LuaNumber) {
	tval.Tt = uint8(types.LUA_VNUMFLT)
	tval.Value.Variant = types.ValueFloat
	tval.Value.Data_ = n
}

func newIntValue(i types.LuaInteger) *TValue {
	return &TValue{
		Tt:   uint8(types.LUA_VNUMINT),
		Value: Value{Variant: types.ValueInteger, Data_: i},
	}
}

func newLightCFunctionValue(fn uintptr) *TValue {
	return &TValue{
		Tt:   uint8(types.LUA_VLCF),
		Value: Value{Variant: types.ValueCFunction, Data_: unsafe.Pointer(fn)},
	}
}
