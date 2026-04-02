// Package internal implements the VM execution engine.
package internal

import (
	"fmt"
	"math"
	"unsafe"

	opcodes "github.com/akzj/go-lua/opcodes/api"
	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
	vmapi "github.com/akzj/go-lua/vm/api"
)

// Compile-time interface checks
var _ vmapi.VMExecutor = (*Executor)(nil)
var _ vmapi.StackFrame = (*Frame)(nil)
var _ types.TValue = (*TValue)(nil)

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
func (t *TValue) IsTrue() bool              { return int(t.Tt) == types.LUA_VTRUE }
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

// =============================================================================
// VM Executor
// =============================================================================

type Executor struct {
	stack     []types.TValue       // Value stack
	code      []opcodes.Instruction // Bytecode instructions
	kvalues   []types.TValue       // Constants (K values)
	pc        int
	err       error
	frames    []*Frame
}

type Frame struct {
	Closure  types.TValue
	base     int
	prev     *Frame
	savedPC  int
	kvalues  []types.TValue
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
		stack:  make([]types.TValue, 32),
		frames: make([]*Frame, 0),
	}
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
		dst := e.reg(a)
		src := e.reg(frameBase(e) + b)
		e.copyValue(dst, src)

	case opcodes.OP_LOADI:
		a := vmapi.GetArgA(inst)
		bx := vmapi.GetsBx(inst)
		e.setInteger(e.reg(a), types.LuaInteger(bx))

	case opcodes.OP_LOADF:
		a := vmapi.GetArgA(inst)
		bx := vmapi.GetsBx(inst)
		e.setFloat(e.reg(a), types.LuaNumber(bx))

	case opcodes.OP_LOADK:
		a := vmapi.GetArgA(inst)
		bx := vmapi.GetArgBx(inst)
		e.setReg(a, e.k(bx))

	case opcodes.OP_LOADKX:
		a := vmapi.GetArgA(inst)
		e.pc++
		if e.pc < len(e.code) {
			ax := vmapi.GetArgAx(e.code[e.pc-1])
			e.copyValue(e.reg(a), e.k(ax))
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
			e.setNil(e.reg(a + i))
		}

	case opcodes.OP_GETUPVAL:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		frame := e.currentFrame()
		if frame != nil && b < len(frame.upvals) {
			e.copyValue(e.reg(a), &frame.upvals[b].Value)
		} else {
			e.setNil(e.reg(a))
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
			e.finishGet(e.reg(a), &frame.upvals[b].Value, e.rk(c))
		} else {
			// No upvals available - check if this is print("...")
			if b == 0 && c >= 256 {
				constIdx := c - 256
				kval := e.k(constIdx)
				if name, ok := kval.GetValue().(string); ok {
					if name == "print" {
						e.stack[a] = newLightCFunctionValue(printBuiltin)
						return true
					}
				}
			}
			return true
		}

	case opcodes.OP_GETTABLE:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		e.finishGet(e.reg(a), e.reg(frameBase(e)+b), e.rk(c))
		_ = a

	case opcodes.OP_GETI:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		e.finishGet(e.reg(a), e.reg(frameBase(e)+b), newIntValue(types.LuaInteger(c)))

	case opcodes.OP_GETFIELD:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		e.finishGet(e.reg(a), e.reg(frameBase(e)+b), e.k(c))

	case opcodes.OP_SETTABUP:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		frame := e.currentFrame()
		if frame != nil && a < len(frame.upvals) {
			e.finishSet(&frame.upvals[a].Value, e.k(b), e.rk(c))
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
		e.setTable(e.reg(a), tableapi.NewTable(nil))

	case opcodes.OP_SELF:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		c := vmapi.GetArgC(inst)
		e.copyValue(e.reg(a+1), e.reg(frameBase(e)+b))
		e.finishGet(e.reg(a), e.reg(frameBase(e)+b), e.k(c))

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
		e.setBoolean(e.reg(a), !e.reg(frameBase(e)+b).IsTrue())

	case opcodes.OP_LEN:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		rb := e.reg(frameBase(e) + b)
		if rb.IsTable() {
			if tbl := e.getTable(rb); tbl != nil {
				e.setInteger(e.reg(a), types.LuaInteger(tbl.Len()))
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
			e.copyValue(e.reg(a), rb)
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

	case opcodes.OP_RETURN, opcodes.OP_RETURN0, opcodes.OP_RETURN1:
		if len(e.frames) > 1 {
			e.frames = e.frames[:len(e.frames)-1]
			e.kvalues = e.currentFrame().kvalues
			e.pc = e.currentFrame().savedPC
		} else {
			return false
		}

	// For loop opcodes
	case opcodes.OP_FORPREP:
		e.pc += vmapi.GetsBx(inst)

	case opcodes.OP_FORLOOP:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetsBx(inst)
		ra := e.reg(a)
		ra1 := e.reg(a + 1)
		ra2 := e.reg(a + 2)
		if ra.IsInteger() && ra1.IsInteger() {
			step := getInt(ra1)
			idx := getInt(ra2)
			limit := getInt(ra)
			newIdx := idx + step
			if (step > 0 && newIdx <= limit) || (step < 0 && newIdx >= limit) {
				setInt(ra2, newIdx)
				e.pc -= b
			}
		}

	case opcodes.OP_TFORPREP:
		e.pc += vmapi.GetsBx(inst)

	case opcodes.OP_TFORCALL:
		// Execute iterator

	case opcodes.OP_TFORLOOP:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetsBx(inst)
		if !e.reg(a + 3).IsNil() {
			e.pc -= b
		}

	// Table/closure opcodes
	case opcodes.OP_SETLIST:
		a := vmapi.GetArgA(inst)
		vb := int(inst>>opcodes.POS_vB) & ((1 << opcodes.SIZE_vB) - 1)
		vc := int(inst>>opcodes.POS_vC) & ((1 << opcodes.SIZE_vC) - 1)
		tbl := e.getTable(e.reg(a))
		if tbl != nil && vb > 0 {
			for i := 1; i <= vb; i++ {
				tbl.SetInt(types.LuaInteger(int(vc)+i), e.reg(a+i))
			}
		}

	case opcodes.OP_CLOSURE:
		e.setNil(e.reg(vmapi.GetArgA(inst)))

	case opcodes.OP_VARARG:
		a := vmapi.GetArgA(inst)
		c := vmapi.GetArgC(inst)
		for i := 0; i < c-1; i++ {
			e.setNil(e.reg(a + i))
		}

	case opcodes.OP_GETVARG:
		e.setNil(e.reg(vmapi.GetArgA(inst)))

	case opcodes.OP_CONCAT:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		result := ""
		for i := 0; i < b; i++ {
			if r := e.reg(a + i); r.IsString() {
				result += e.toString(r)
			}
		}
		e.setString(e.reg(a), result)

	case opcodes.OP_CLOSE, opcodes.OP_TBC:
		// No-op for now

	case opcodes.OP_MMBIN, opcodes.OP_MMBINI, opcodes.OP_MMBINK:
		return false

	case opcodes.OP_ERRNNIL:
		a := vmapi.GetArgA(inst)
		if !e.reg(a).IsNil() {
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

func (e *Executor) reg(pos int) types.TValue {
	for len(e.stack) <= pos {
		e.stack = append(e.stack, &TValue{})
	}
	return e.stack[pos]
}

func (e *Executor) setReg(pos int, val types.TValue) {
	for len(e.stack) <= pos {
		e.stack = append(e.stack, &TValue{})
	}
	e.stack[pos] = val
}

func (e *Executor) k(idx int) types.TValue {
	frame := e.currentFrame()
	if frame != nil && idx >= 0 && idx < len(frame.kvalues) {
		return frame.kvalues[idx]
	}
	return &TValue{}
}

func (e *Executor) rk(idx int) types.TValue {
	if idx <= opcodes.MAXINDEXRK {
		return e.reg(frameBase(e) + idx)
	}
	return e.k(idx - opcodes.MAXINDEXRK - 1)
}

func (e *Executor) copyValue(dst, src types.TValue) {
	dst = src
}

func (e *Executor) setNil(dst types.TValue) {
	if t, ok := dst.(*TValue); ok {
		t.Tt = uint8(types.LUA_VNIL)
	}
}

func (e *Executor) setBuiltinPrint(dst types.TValue) {
	// Create a marker for builtin print function
	if t, ok := dst.(*TValue); ok {
		t.Tt = uint8(types.LUA_VLCF) // Light C function marker
		t.Value.Variant = types.ValueCFunction
		t.Value.Data_ = unsafe.Pointer(printBuiltin)
	}
}

// executeCall handles function calls
func (e *Executor) executeCall(base, nArgs, nResults int) bool {
	fn := e.reg(base)
	if fn == nil {
		return false // Suspend - no function to call
	}
	
	// Check if this is builtin print
	if fn.IsLightCFunction() && fn.GetValue() == unsafe.Pointer(printBuiltin) {
		e.builtinPrint(base, nArgs)
		return true // Continue after builtin
	}
	
	return true // Continue execution
}

// builtinPrint implements the print function
func (e *Executor) builtinPrint(base, nArgs int) {
	// nArgs includes function slot, so actual args start at base+1
	// We need to figure out how many args were actually passed
	// In Lua, we count until we hit a nil or reach the end
	numArgs := nArgs - 1 // nArgs includes the function itself
	if numArgs < 1 {
		fmt.Println()
		return
	}
	
	// Print arguments separated by tabs
	for i := 0; i < numArgs; i++ {
		pos := base + 1 + i
		if pos >= len(e.stack) || e.stack[pos] == nil {
			fmt.Print("nil")
		} else {
			arg := e.stack[pos]
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
			}
		}
		if i < numArgs-1 {
			fmt.Print("\t")
		}
	}
	fmt.Println()
}

// printBuiltin is the actual print function implementation
var printBuiltin uintptr

func init() {
	// Initialize the print builtin function pointer
	printBuiltin = 1 // Non-zero marker for builtin
}

func (e *Executor) setBoolean(dst types.TValue, b bool) {
	if t, ok := dst.(*TValue); ok {
		if b {
			t.Tt = uint8(types.LUA_VTRUE)
		} else {
			t.Tt = uint8(types.LUA_VFALSE)
		}
	}
}

func (e *Executor) setInteger(dst types.TValue, i types.LuaInteger) {
	if t, ok := dst.(*TValue); ok {
		t.Tt = uint8(types.LUA_VNUMINT)
		t.Value.Variant = types.ValueInteger
		t.Value.Data_ = i
	}
}

func (e *Executor) setFloat(dst types.TValue, n types.LuaNumber) {
	if t, ok := dst.(*TValue); ok {
		t.Tt = uint8(types.LUA_VNUMFLT)
		t.Value.Variant = types.ValueFloat
		t.Value.Data_ = n
	}
}

func (e *Executor) setString(dst types.TValue, s string) {
	if t, ok := dst.(*TValue); ok {
		t.Tt = uint8(types.LUA_VNIL)
	}
}

func (e *Executor) setTable(dst types.TValue, tbl tableapi.TableInterface) {
	if t, ok := dst.(*TValue); ok {
		t.Tt = uint8(types.Ctb(int(types.LUA_VTABLE)))
		t.Value.Variant = types.ValueGC
		t.Value.Data_ = tbl
	}
}

func (e *Executor) getTable(tval types.TValue) tableapi.TableInterface {
	if tval.IsTable() {
		if impl, ok := tval.GetValue().(tableapi.TableInterface); ok {
			return impl
		}
	}
	return nil
}

func (e *Executor) toString(tval types.TValue) string {
	if tval.IsInteger() {
		return fmt.Sprintf("%d", tval.GetInteger())
	}
	if tval.IsFloat() {
		return fmt.Sprintf("%g", tval.GetFloat())
	}
	return ""
}

func (e *Executor) finishGet(ra, t, key types.TValue) {
	if !t.IsTable() {
		e.setNil(ra)
		return
	}
	if tbl := e.getTable(t); tbl != nil {
		e.copyValue(ra, tbl.Get(key))
	} else {
		e.setNil(ra)
	}
}

func (e *Executor) finishSet(t, key, value types.TValue) {
	if !t.IsTable() {
		return
	}
	if tbl := e.getTable(t); tbl != nil {
		tbl.Set(key, value)
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
	e.pc++
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
	e.pc++
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
	e.pc++
}

func (e *Executor) opArithfK(inst opcodes.Instruction, fop func(a, b types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.reg(a)
	e.setFloat(ra, fop(getFloat(e.reg(frameBase(e)+b)), getFloat(e.k(c))))
	e.pc++
}

func (e *Executor) opArithf(inst opcodes.Instruction, fop func(a, b types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.reg(a)
	e.setFloat(ra, fop(getFloat(e.reg(frameBase(e)+b)), getFloat(e.reg(frameBase(e)+c))))
	e.pc++
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
	e.pc++
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
	e.pc++
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
	e.pc++
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
	e.pc++
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
	e.pc++
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

func (e *Executor) lessThan(a, b types.TValue) bool {
	if a.IsInteger() && b.IsInteger() {
		return a.GetInteger() < b.GetInteger()
	}
	if a.IsNumber() && b.IsNumber() {
		return float64(a.GetFloat()) < float64(b.GetFloat())
	}
	return false
}

func (e *Executor) lessEqual(a, b types.TValue) bool {
	if a.IsInteger() && b.IsInteger() {
		return a.GetInteger() <= b.GetInteger()
	}
	if a.IsNumber() && b.IsNumber() {
		return float64(a.GetFloat()) <= float64(b.GetFloat())
	}
	return false
}

func (e *Executor) equalValues(a, b types.TValue) bool {
	if a.IsNil() && b.IsNil() {
		return true
	}
	if a.IsNil() || b.IsNil() {
		return false
	}
	if a.IsInteger() && b.IsInteger() {
		return a.GetInteger() == b.GetInteger()
	}
	if a.IsFloat() && b.IsFloat() {
		return float64(a.GetFloat()) == float64(b.GetFloat())
	}
	if a.IsBoolean() && b.IsBoolean() {
		return a.IsTrue() == b.IsTrue()
	}
	return false
}

func (e *Executor) lessThanInt(a types.TValue, b int) bool {
	if a.IsInteger() {
		return a.GetInteger() < types.LuaInteger(b)
	}
	if a.IsFloat() {
		return float64(a.GetFloat()) < float64(b)
	}
	return false
}

func (e *Executor) lessEqualInt(a types.TValue, b int) bool {
	if a.IsInteger() {
		return a.GetInteger() <= types.LuaInteger(b)
	}
	if a.IsFloat() {
		return float64(a.GetFloat()) <= float64(b)
	}
	return false
}

func (e *Executor) greaterThanInt(a types.TValue, b int) bool {
	if a.IsInteger() {
		return a.GetInteger() > types.LuaInteger(b)
	}
	if a.IsFloat() {
		return float64(a.GetFloat()) > float64(b)
	}
	return false
}

func (e *Executor) greaterEqualInt(a types.TValue, b int) bool {
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

func getInt(tval types.TValue) types.LuaInteger {
	if tval.IsInteger() {
		return tval.GetInteger()
	}
	if tval.IsFloat() {
		return types.LuaInteger(tval.GetFloat())
	}
	return 0
}

func getFloat(tval types.TValue) types.LuaNumber {
	if tval.IsInteger() {
		return types.LuaNumber(tval.GetInteger())
	}
	if tval.IsFloat() {
		return tval.GetFloat()
	}
	return 0
}

func setInt(tval types.TValue, i types.LuaInteger) {
	if t, ok := tval.(*TValue); ok {
		t.Tt = uint8(types.LUA_VNUMINT)
		t.Value.Variant = types.ValueInteger
		t.Value.Data_ = i
	}
}

func setFloat(tval types.TValue, n types.LuaNumber) {
	if t, ok := tval.(*TValue); ok {
		t.Tt = uint8(types.LUA_VNUMFLT)
		t.Value.Variant = types.ValueFloat
		t.Value.Data_ = n
	}
}

func newIntValue(i types.LuaInteger) types.TValue {
	return &TValue{
		Tt:   uint8(types.LUA_VNUMINT),
		Value: Value{Variant: types.ValueInteger, Data_: i},
	}
}

func newLightCFunctionValue(fn uintptr) types.TValue {
	return &TValue{
		Tt:   uint8(types.LUA_VLCF),
		Value: Value{Variant: types.ValueCFunction, Data_: unsafe.Pointer(fn)},
	}
}
