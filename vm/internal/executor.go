// Package internal implements the VM execution engine.
package internal

import (
	"fmt"
	"strconv"
	"strings"
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
// =============================================================================
// Protected Call Support (for pcall/xpcall)
// =============================================================================

// activeExecutor holds the currently running executor for goroutine-local access.
// This allows GoFuncs (like pcall) to call back into the executor.
// Safe because each goroutine has its own executor and Run() is not reentrant.
var activeExecutor *Executor

// ProtectedCallFromGoFunc performs a protected function call from within a GoFunc.
// It wraps executeCall in a recover to catch LuaError panics.
// The function to call must already be at stack[base], with args following.
// Returns (nResults, nil) on success or (0, errorTValue) on error.
func ProtectedCallFromGoFunc(base, nArgs, nResults int) (int, interface{}) {
	e := activeExecutor
	if e == nil {
		return 0, "no active executor for protected call"
	}
	return e.protectedCall(base, nArgs, nResults)
}

// luaErrorMsg is a duck-type interface to extract Msg from LuaError
// without importing state/internal (which would create a circular dependency).
type luaErrorMsg interface {
	Error() string
}

// luaErrorWithMsg extracts the Msg field from a LuaError-like panic value.
// LuaError has: Msg types.TValue. We use reflection-free duck typing.
type luaErrorWithTValue interface {
	GetMsg() types.TValue
}

// protectedCall wraps executeCall in recover to catch LuaError panics.
func (e *Executor) protectedCall(base, nArgs, nResults int) (retN int, retErr interface{}) {
	// Save executor state so we can restore on error
	savedPC := e.pc
	savedCode := e.code
	savedKvalues := e.kvalues
	savedFrameCount := len(e.frames)
	savedErr := e.err
	
	defer func() {
		if r := recover(); r != nil {
			// Restore executor state on panic
			e.pc = savedPC
			e.code = savedCode
			e.kvalues = savedKvalues
			e.err = savedErr
			// Pop any frames that were pushed during the failed call
			for len(e.frames) > savedFrameCount {
				e.frames = e.frames[:len(e.frames)-1]
			}
			if savedFrameCount > 0 {
				e.kvalues = e.frames[savedFrameCount-1].kvalues
			}
			
			// Try to extract the error message from the panic value.
			// LuaError has Msg field of type types.TValue.
			// We use struct field extraction via interface.
			retN = 0
			retErr = extractPanicError(r)
		}
	}()
	
	ok := e.executeCall(base, nArgs, nResults)
	if !ok {
		if e.err != nil {
			errMsg := e.err.Error()
			e.err = savedErr
			return 0, errMsg
		}
		return 0, "call failed"
	}
	
	// For Lua closures, executeCall pushes a frame and returns true.
	// We need to run the VM loop until that frame completes.
	// The frame was pushed, so run until frame count returns to savedFrameCount.
	for len(e.frames) > savedFrameCount {
		if !e.executeNext() {
			break
		}
	}
	
	// Check if the VM loop ended with an error
	if e.err != nil && e.err != savedErr {
		errMsg := e.err.Error()
		e.err = savedErr
		return 0, errMsg
	}
	
	return nResults, nil
}

// extractPanicError converts a recover() value into a handlePcall-friendly error.
// It handles LuaError (from state/internal) via duck-typing to avoid circular imports.
func extractPanicError(r interface{}) interface{} {
	// Check if it has a Msg field that is a types.TValue
	// LuaError struct: { Msg types.TValue }
	// We can't import LuaError, but we can use reflect or duck-type.
	// Duck-type approach: check for Error() string method first.
	
	// Try to extract Msg via a known interface
	type msgProvider interface {
		GetMsg() types.TValue
	}
	if mp, ok := r.(msgProvider); ok {
		return &luaErrorValue{msg: mp.GetMsg()}
	}
	
	// Try reflection to get .Msg field (LuaError has public Msg field)
	// Use fmt to get the string representation
	if err, ok := r.(error); ok {
		return err.Error()
	}
	
	return fmt.Sprintf("%v", r)
}

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
	stringMetatable tableapi.TableInterface // Shared metatable for all strings (__index = string lib)
	lastCallNRet int                  // Number of results from last GoFunc call (for multi-return propagation)
	lastCallBase int                  // Base register of last call (for B=0 arg count computation)
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
	Value TValue    // Used when closed (variable has left scope) or for non-stack upvalues
	open  *TValue   // When non-nil, points to the actual stack slot (open upvalue)
}

// Get returns the current value of the upvalue.
func (uv *UpVal) Get() *TValue {
	if uv.open != nil {
		return uv.open
	}
	return &uv.Value
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

// SetStringMetatable sets the shared metatable for all string values.
func (e *Executor) SetStringMetatable(mt tableapi.TableInterface) {
	e.stringMetatable = mt
}

// SetGlobalEnv sets the global environment table for the executor
func (e *Executor) SetGlobalEnv(env tableapi.TableInterface) {
	e.globalEnv = env
	e.globalEnvPtr = &globalEnvWrapper{env: env}
}

// SetGlobalEnvUpval initializes an UpVal to point to the global environment table.
// Used to set up the main frame's _ENV upvalue.
func (e *Executor) SetGlobalEnvUpval(uv *UpVal) {
	if e.globalEnv != nil {
		uv.Value = TValue{
			Value: Value{Variant: types.ValueGC, Data_: e.globalEnv},
			Tt:    uint8(types.Ctb(int(types.LUA_VTABLE))),
		}
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
	// Set active executor for goroutine-local access by GoFuncs (pcall etc.)
	prevExecutor := activeExecutor
	activeExecutor = e
	defer func() { activeExecutor = prevExecutor }()
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
		e.setBoolean(e.RA(vmapi.GetArgA(inst)), false)

	case opcodes.OP_LFALSESKIP:
		e.setBoolean(e.RA(vmapi.GetArgA(inst)), false)
		e.pc++

	case opcodes.OP_LOADTRUE:
		e.setBoolean(e.RA(vmapi.GetArgA(inst)), true)

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
			e.copyValue(e.RA(a), frame.upvals[b].Get())
		} else if b == 0 && e.globalEnv != nil {
			// Upval 0 is _ENV — fall back to globalEnv when frame has no upvals
			ra := e.RA(a)
			ra.Value.Variant = types.ValueGC
			ra.Value.Data_ = e.globalEnv
			ra.Tt = uint8(types.Ctb(int(types.LUA_VTABLE)))
		} else {
			e.setNil(e.RA(a))
		}

	case opcodes.OP_SETUPVAL:
		b := vmapi.GetArgB(inst)
		frame := e.currentFrame()
		if frame != nil && b < len(frame.upvals) {
			e.copyValue(frame.upvals[b].Get(), e.RA(vmapi.GetArgA(inst)))
		} else if b == 0 {
			// SETUPVAL on _ENV (upval 0) — update the global environment reference
			ra := e.RA(vmapi.GetArgA(inst))
			if ra.IsTable() {
				if tbl := e.getTable(ra); tbl != nil {
					e.globalEnv = tbl
				}
			}
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
			// GETTABUP C field is always a constant index, use e.k() not e.rk()
			e.finishGet(e.RA(a), frame.upvals[b].Get(), e.k(int(c)))
		} else if b == 0 {
			// b==0 means upval[0]/_ENV. c is raw 0-based constant index.
			if e.globalEnv != nil {
				globalTValue := &TValue{
					Value: Value{Variant: types.ValueGC, Data_: e.globalEnv},
					Tt:    uint8(types.Ctb(int(types.LUA_VTABLE))),
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
			e.finishSet(frame.upvals[a].Get(), e.k(b), e.rk(c))
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
		if rb.IsString() {
			if s, ok := rb.GetValue().(string); ok {
				str := s
			e.setInteger(e.RA(a), types.LuaInteger(len(str)))
			}
		} else if rb.IsTable() {
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
		if cond != vmapi.HasKBit(inst) {
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
		if cond != vmapi.HasKBit(inst) {
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
			// Lua 5.4: convert float to int if exact, then compare as integers
			f := float64(ra.GetFloat())
			fi := int64(f)
			if f == float64(fi) {
				cond = fi == int64(b)
			}
		}
		if cond != vmapi.HasKBit(inst) {
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
		// b = number of arguments: b-1 fixed args (b includes func slot)
		// b == 0 means variable args — use lastCallNRet to determine count
		nArgs := b
		if b == 0 && e.lastCallNRet > 0 {
			// Variable args: B=0 means "use top".
			// The previous call placed lastCallNRet results starting at lastCallBase.
			// This call's function is at frameBase+a.
			// nArgs = (lastCallBase - (frameBase+a)) + lastCallNRet
			// This accounts for any fixed args between the function and the multi-return.
			outerBase := frameBase(e) + a
			nArgs = (e.lastCallBase - outerBase) + e.lastCallNRet
		}
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
		} else if b == 0 {
			// Multi-return: return all values from R[A] to top
			// Use lastCallNRet as a hint, or count non-nil values
			nRet = e.lastCallNRet
			if nRet == 0 {
				// Fallback: count non-nil values from R[A]
				for i := 0; i < 256; i++ {
					r := e.reg(base + a + i)
					if r.Tt == uint8(types.LUA_TNIL) || r.Tt == 0 {
						break
					}
					nRet++
				}
			}
		}
		// b==1 means no return values: nRet=0
		e.lastCallNRet = nRet

		// The caller's frame is below the current one.
		// The caller expects results starting at the function slot (calleeBase - 1).
		calleeBase := e.currentFrame().base

		// Copy return values to caller's expected position (function slot)
		for i := 0; i < nRet; i++ {
			src := e.reg(base + a + i)
			dst := e.reg(calleeBase - 1 + i)
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

		// Clear the function slot (caller expects nil for 0-return functions)
		calleeBase := e.currentFrame().base
		e.setNil(e.reg(calleeBase - 1))

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

		// Copy single return value to caller's expected position (function slot)
		calleeBase := e.currentFrame().base
		src := e.reg(base + a)
		dst := e.reg(calleeBase - 1)
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

		// Propagate upvalues to the new closure using the child prototype's upvalue descriptors.
		// Each descriptor says: Instack=1 → capture from parent's register; Instack=0 → copy from parent's upvalue.
		uvDescs := subProto.GetUpvalues()
		if len(uvDescs) > 0 {
			newClosure.upvals = make([]*UpVal, len(uvDescs))
			for i, desc := range uvDescs {
				if desc.Instack == 1 {
					// Capture from parent's register — create open upvalue pointing to stack slot
					uv := &UpVal{open: e.reg(frameBase(e) + int(desc.Idx))}
					newClosure.upvals[i] = uv
				} else {
					// Copy from parent's upvalue (chained capture)
					if frame.upvals != nil && int(desc.Idx) < len(frame.upvals) {
						newClosure.upvals[i] = frame.upvals[desc.Idx]
					} else if desc.Name == "_ENV" && e.globalEnv != nil {
						// Main frame has no upvals array but child needs _ENV — use globalEnv as proper table
						envUpval := &UpVal{}
						envUpval.Value.Tt = uint8(types.Ctb(int(types.LUA_VTABLE)))
						envUpval.Value.Value.Variant = types.ValueGC
						envUpval.Value.Value.Data_ = e.globalEnv
						newClosure.upvals[i] = envUpval
					} else {
						// Fallback: create nil upvalue
						newClosure.upvals[i] = &UpVal{}
					}
				}
			}
		} else if frame.upvals != nil && len(frame.upvals) > 0 {
			// No upvalue descriptors (old-style) — copy all parent upvals for backward compat
			newClosure.upvals = make([]*UpVal, len(frame.upvals))
			copy(newClosure.upvals, frame.upvals)
		} else if e.globalEnv != nil {
			// Top-level frame with no upvals: create _ENV upval from globalEnv as proper table
			envUpval := &UpVal{}
			envUpval.Value.Tt = uint8(types.Ctb(int(types.LUA_VTABLE)))
			envUpval.Value.Value.Variant = types.ValueGC
			envUpval.Value.Value.Data_ = e.globalEnv
			newClosure.upvals = []*UpVal{envUpval}
		}

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
		e.setNil(e.RA(vmapi.GetArgA(inst)))

	case opcodes.OP_CONCAT:
		a := vmapi.GetArgA(inst)
		b := vmapi.GetArgB(inst)
		result := ""
		for i := 0; i < b; i++ {
			r := e.reg(frameBase(e) + a + i)
			if r.IsString() {
				result += e.toString(r)
			} else if r.IsInteger() {
				result += fmt.Sprintf("%d", r.GetInteger())
			} else if r.IsFloat() {
				// Use %.14g like Lua for float formatting
				result += fmt.Sprintf("%.14g", r.GetFloat())
			} else {
				e.err = fmt.Errorf("attempt to concatenate a non-string value")
				return false
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
	e.lastCallBase = base
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
			nRet := f(e.stack, base)
			e.lastCallNRet = nRet
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
			nRet := gf(e.stack, base)
			e.lastCallNRet = nRet
			return true
		}
		e.err = fmt.Errorf("attempt to call non-Go-function CClosure")
		return false
	}

	// Handle Lua closures (LClosure)
	if fn.IsLClosure() {
		val := fn.GetValue()

		// Check for pcall/xpcall FIRST — these need special VM-level handling
		if _, ok := val.(vmapi.PcallTag); ok {
			return e.handlePcall(base, nArgs, nResults)
		}
		if _, ok := val.(vmapi.XpcallTag); ok {
			return e.handleXpcall(base, nArgs, nResults)
		}

		// Check for vm/api.GoFunc (from goFuncWrapper via setGlobal).
		// This uses []types.TValue, not the internal GoFunc type.
		if apiFunc, ok := val.(vmapi.GoFunc); ok {
			// Bridge: GoFuncs count args as len(stack)-base-1.
			// gfBase offsets args so GoFuncs that return more values than
			// args have room to write (e.g. ipairs returns 3 from 1 arg).
			// GoFuncs will see inflated nArgs due to extra slots, so they
			// must check for nil trailing args (not just count them).
			gfBase := 4 // room for extra return values
			sliceSize := gfBase + nArgs + 4 // extra after args too
			args := make([]types.TValue, sliceSize)
			for i := 0; i < nArgs; i++ {
				args[gfBase+i] = e.reg(base + i)
			}
			nRet := apiFunc(args, gfBase)
			e.lastCallNRet = nRet
			// Copy results back: GoFunc writes to args[gfBase..gfBase+nRet-1]
			for i := 0; i < nRet; i++ {
				result := args[gfBase+i]
				dst := e.reg(base + i)
				if result == nil {
					dst.Tt = uint8(types.LUA_TNIL)
					dst.Value.Variant = types.ValueGC
					dst.Value.Data_ = nil
					continue
				}
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

		// Check for internal GoFunc (direct GoFunc storage)
		if gf, ok := val.(GoFunc); ok {
			nRet := gf(e.stack, base)
			e.lastCallNRet = nRet
			return true
		}

		// Check for goFuncWrapper (GoFunc stored in module tables like string, math).
		// fn.GetValue() returns *goFuncWrapper. We need to unwrap it.
		if unwrapper, ok := val.(goFuncUnwrapper); ok {
			apiFunc := unwrapper.unwrapGoFunc()
			// Bridge: same gfBase offset approach as apiFunc bridge.
			extra := 4
			sliceSize := nArgs + 2*extra
			args := make([]types.TValue, sliceSize)
			gfBase := extra
			for i := 0; i < nArgs; i++ {
				args[gfBase+i] = e.reg(base + i)
			}
			nRet := apiFunc(args, gfBase)
			e.lastCallNRet = nRet
			for i := 0; i < nRet; i++ {
				result := args[gfBase+i]
				dst := e.reg(base + i)
				if result == nil {
					dst.Tt = uint8(types.LUA_TNIL)
					dst.Value.Variant = types.ValueGC
					dst.Value.Data_ = nil
					continue
				}
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

		// Push new frame — COPY the closure TValue so it survives stack mutations
		// (OP_RETURN writes return values to calleeBase, which would overwrite fn)
		closureCopy := &TValue{Value: fn.Value, Tt: fn.Tt}
		newFrame := &Frame{
			Closure: closureCopy,
			base:    base + 1, // +1: skip function slot, register 0 = first parameter
			prev:    e.currentFrame(),
			savedPC: 0,
			kvalues: kvals,
		}
		// Copy upvalues from the closure to the frame so GETTABUP/GETUPVAL can find them
		if lci, ok := lc.(*luaClosureImpl); ok && lci.upvals != nil {
			newFrame.upvals = lci.upvals
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
	dst.Value.Variant = 0
	dst.Value.Data_ = nil
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
			wrapper := (*globalEnvWrapper)(ptr)
			if wrapper != nil && wrapper.env != nil {
				tval := &TValue{
					Value: Value{Variant: types.ValueGC, Data_: wrapper.env},
					Tt:    uint8(types.Ctb(int(types.LUA_VTABLE))),
				}
				tbl2 := e.getTable(tval)
				if tbl2 != nil {
					result := tbl2.Get(key)
					if result == nil || result.IsNil() {
						e.setNil(ra)
						return
					}
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

	// String metatable: when indexing a string, use __index from string metatable
	if t.IsString() {
		if e.stringMetatable != nil {
			e.finishGetWithMetatable(ra, e.stringMetatable, t, key)
			return
		}
		e.setNil(ra)
		return
	}

	if !t.IsTable() {
		e.setNil(ra)
		return
	}
	tbl := e.getTable(t)
	if tbl == nil {
		e.setNil(ra)
		return
	}
	result := tbl.Get(key)
	if result != nil && !result.IsNil() {
		// Key found in table — copy result to ra
		if rv, ok := result.(*TValue); ok {
			e.copyValue(ra, rv)
		} else {
			variant, data := extractVariantAndData(result)
			ra.Tt = uint8(result.GetTag())
			ra.Value.Variant = variant
			ra.Value.Data_ = data
		}
		return
	}
	// Key not found — check metatable __index chain
	mt := tbl.GetMetatable()
	if mt == nil {
		e.setNil(ra)
		return
	}
	mtTbl := tableapi.WrapRawTableFactory(mt)
	if mtTbl == nil {
		e.setNil(ra)
		return
	}
	e.finishGetWithMetatable(ra, mtTbl, t, key)
}

// finishGetWithMetatable looks up __index in a metatable and resolves the key.
// Handles __index as table (recursive) or function (call).
// Uses MAXTAGLOOP to prevent infinite metatable chains.
func (e *Executor) finishGetWithMetatable(ra *TValue, mtTbl tableapi.TableInterface, origObj, key *TValue) {
	indexKey := &TValue{
		Value: Value{Variant: types.ValueGC, Data_: "__index"},
		Tt:    uint8(types.Ctb(int(types.LUA_VSHRSTR))),
	}
	for loop := 0; loop < MAXTAGLOOP; loop++ {
		indexVal := mtTbl.Get(indexKey)
		if indexVal == nil || indexVal.IsNil() {
			e.setNil(ra)
			return
		}
		// __index is a table: look up key in that table
		if indexVal.IsTable() {
			if idxTbl, ok := indexVal.GetValue().(tableapi.TableInterface); ok {
				result := idxTbl.Get(key)
				if result != nil && !result.IsNil() {
					if rv, ok := result.(*TValue); ok {
						e.copyValue(ra, rv)
					} else {
						variant, data := extractVariantAndData(result)
						ra.Tt = uint8(result.GetTag())
						ra.Value.Variant = variant
						ra.Value.Data_ = data
					}
					return
				}
				// Not found in __index table — check ITS metatable
				nextMt := idxTbl.GetMetatable()
				if nextMt == nil {
					e.setNil(ra)
					return
				}
				mtTbl = tableapi.WrapRawTableFactory(nextMt)
				if mtTbl == nil {
					e.setNil(ra)
					return
				}
				continue // loop to check next level
			}
		}
		// __index is a function: call it with (origObj, key)
		if indexVal.IsFunction() || indexVal.IsLClosure() || indexVal.IsCClosure() || indexVal.IsLightCFunction() {
			// Save ra's offset so we can recompute it after stack reallocation
			raOffset := int(uintptr(unsafe.Pointer(ra)) - uintptr(unsafe.Pointer(&e.stack[0]))) / int(unsafe.Sizeof(TValue{}))

			// Copy origObj and key BEFORE append — append may reallocate the stack,
			// invalidating any pointers into it
			origObjCopy := *origObj
			keyCopy := *key

			// Pre-extend stack with 3 slots, then compute callBase
			e.stack = append(e.stack, TValue{}, TValue{}, TValue{})
			callBase := len(e.stack) - 3

			fnSlot := &e.stack[callBase]
			variant, data := extractVariantAndData(indexVal)
			fnSlot.Tt = uint8(indexVal.GetTag())
			fnSlot.Value.Variant = variant
			fnSlot.Value.Data_ = data

			e.stack[callBase + 1] = origObjCopy
			e.stack[callBase + 2] = keyCopy

			// Save caller state
			savedPC := e.pc
			savedCode := e.code
			savedKvalues := e.kvalues
			savedFrameCount := len(e.frames)

			e.currentFrame().savedPC = e.pc
			e.executeCall(callBase, 3, 2)

			// If executeCall pushed a Lua closure frame, run it to completion
			if len(e.frames) > savedFrameCount {
				for len(e.frames) > savedFrameCount {
					if !e.executeNext() {
						break
					}
				}
				// Restore caller state
				e.pc = savedPC
				e.code = savedCode
				e.kvalues = savedKvalues
			}

			// Result is at callBase. Copy to ra (recomputed after potential realloc)
			result := e.stack[callBase]
			ra = &e.stack[raOffset]
			*ra = result
			// Shrink stack back
			e.stack = e.stack[:callBase]
			return
		}
		// __index is neither table nor function
		e.setNil(ra)
		return
	}
	e.err = fmt.Errorf("'__index' chain too long; possible loop")
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
	ra := e.RA(a)
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
	ra := e.RA(a)
	rb := e.reg(frameBase(e) + b)
	rc := e.reg(frameBase(e) + c)
	// Try integer path first (including string-to-integer coercion)
	bi, bok := coerceToInteger(rb)
	ci, cok := coerceToInteger(rc)
	if bok && cok && !rb.IsFloat() && !rc.IsFloat() {
		e.setInteger(ra, iop(bi, ci))
	} else {
		e.setFloat(ra, fop(getFloat(rb), getFloat(rc)))
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opArithK(inst opcodes.Instruction, iop func(a, b types.LuaInteger) types.LuaInteger, fop func(a, b types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.RA(a)
	rb := e.reg(frameBase(e) + b)
	kc := e.k(c)
	bi, bok := coerceToInteger(rb)
	ci, cok := coerceToInteger(kc)
	if bok && cok && !rb.IsFloat() && !kc.IsFloat() {
		e.setInteger(ra, iop(bi, ci))
	} else {
		e.setFloat(ra, fop(getFloat(rb), getFloat(kc)))
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opArithfK(inst opcodes.Instruction, fop func(a, b types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.RA(a)
	e.setFloat(ra, fop(getFloat(e.reg(frameBase(e)+b)), getFloat(e.k(c))))
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opArithf(inst opcodes.Instruction, fop func(a, b types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.RA(a)
	e.setFloat(ra, fop(getFloat(e.reg(frameBase(e)+b)), getFloat(e.reg(frameBase(e)+c))))
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opBitwise(inst opcodes.Instruction, op func(a, b types.LuaInteger) types.LuaInteger) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.RA(a)
	rb := e.reg(frameBase(e) + b)
	rc := e.reg(frameBase(e) + c)
	bi, bok := coerceToInteger(rb)
	ci, cok := coerceToInteger(rc)
	if bok && cok {
		e.setInteger(ra, op(bi, ci))
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opBitwiseK(inst opcodes.Instruction, op func(a, b types.LuaInteger) types.LuaInteger) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.RA(a)
	rb := e.reg(frameBase(e) + b)
	kc := e.k(c)
	bi, bok := coerceToInteger(rb)
	ci, cok := coerceToInteger(kc)
	if bok && cok {
		e.setInteger(ra, op(bi, ci))
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
	ra := e.RA(a)
	rb_raw := e.reg(frameBase(e) + b)
	// Coerce string to integer for shift operations
	var rb *TValue
	if bi, ok := coerceToInteger(rb_raw); ok && !rb_raw.IsInteger() {
		tmp := &TValue{}
		e.setInteger(tmp, bi)
		rb = tmp
	} else {
		rb = rb_raw
	}
	if rb.IsInteger() {
		ib := rb.GetInteger()
		if left {
			e.setInteger(ra, ib<<types.LuaInteger(sc))
		} else {
			// Lua >> is logical right shift (unsigned)
			e.setInteger(ra, types.LuaInteger(uint64(ib)>>uint64(sc)))
		}
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opShift(inst opcodes.Instruction, left bool) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	c := vmapi.GetArgC(inst)
	ra := e.RA(a)
	rb := e.reg(frameBase(e) + b)
	rc := e.reg(frameBase(e) + c)
	if rb.IsInteger() && rc.IsInteger() {
		ib := rb.GetInteger()
		ic := rc.GetInteger()
		if left {
			e.setInteger(ra, ib<<ic)
		} else {
			// Lua >> is logical right shift (unsigned)
			e.setInteger(ra, types.LuaInteger(uint64(ib)>>uint64(ic)))
		}
	}
	// Note: pc++ removed - executeNext() already increments pc
}

func (e *Executor) opUnary(inst opcodes.Instruction, iop func(v types.LuaInteger) types.LuaInteger, fop func(v types.LuaNumber) types.LuaNumber) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	ra := e.RA(a)
	rb := e.reg(frameBase(e) + b)
	if rb.IsInteger() {
		e.setInteger(ra, iop(rb.GetInteger()))
	} else if rb.IsFloat() {
		e.setFloat(ra, fop(rb.GetFloat()))
	} else if rb.IsString() {
		// String-to-number coercion for unary operators
		if i, ok := coerceToInteger(rb); ok {
			e.setInteger(ra, iop(i))
		} else if n, ok := coerceStringToNumber(rb); ok {
			e.setFloat(ra, fop(n))
		}
	}
	// Note: pc++ removed - executeNext() already increments pc
}

// =============================================================================
// Comparison Operations
// =============================================================================

func (e *Executor) opCompare(inst opcodes.Instruction, lessThan bool) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	ra := e.reg(frameBase(e) + a)
	rb := e.reg(frameBase(e) + b)
	var cond bool
	if lessThan {
		cond = e.lessThan(ra, rb)
	} else {
		cond = e.equalValues(ra, rb)
	}
	if cond != vmapi.HasKBit(inst) {
		e.pc++
	}
}

func (e *Executor) opCompareLE(inst opcodes.Instruction) {
	a := vmapi.GetArgA(inst)
	b := vmapi.GetArgB(inst)
	ra := e.reg(frameBase(e) + a)
	rb := e.reg(frameBase(e) + b)
	if e.lessEqual(ra, rb) != vmapi.HasKBit(inst) {
		e.pc++
	}
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
	if a.IsFloat() && b.IsFloat() {
		return float64(a.GetFloat()) < float64(b.GetFloat())
	}
	// Mixed int/float: use luaLessThanMixed for precision
	if a.IsInteger() && b.IsFloat() {
		return intLessFloat(int64(a.GetInteger()), float64(b.GetFloat()))
	}
	if a.IsFloat() && b.IsInteger() {
		return floatLessInt(float64(a.GetFloat()), int64(b.GetInteger()))
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
	if a.IsFloat() && b.IsFloat() {
		return numToFloat64(a) <= numToFloat64(b)
	}
	// Mixed int/float: use precision-safe helpers
	if a.IsInteger() && b.IsFloat() {
		return intLessEqualFloat(int64(a.GetInteger()), float64(b.GetFloat()))
	}
	if a.IsFloat() && b.IsInteger() {
		return floatLessEqualInt(float64(a.GetFloat()), int64(b.GetInteger()))
	}
	if a.IsString() && b.IsString() {
		sa, _ := a.GetValue().(string)
		sb, _ := b.GetValue().(string)
		return sa <= sb
	}
	return false
}


// Lua 5.4 mixed int/float comparison helpers.
// These avoid precision loss by not converting large ints to float.

// intLessFloat: is int64(i) < float64(f)?
func intLessFloat(i int64, f float64) bool {
	if math.IsNaN(f) {
		return false // NaN is not less than anything
	}
	if f >= 9223372036854775808.0 { // f >= 2^63
		return true // any int64 < f
	}
	if f < -9223372036854775808.0 { // f < -2^63
		return false // any int64 > f
	}
	// f is in int64 range; truncate and compare
	fi := int64(f)
	if i < fi {
		return true
	}
	if i > fi {
		return false
	}
	// i == fi; check if f has a fractional part > 0
	return float64(fi) < f
}

// floatLessInt: is float64(f) < int64(i)?
func floatLessInt(f float64, i int64) bool {
	if math.IsNaN(f) {
		return false
	}
	if f >= 9223372036854775808.0 {
		return false
	}
	if f < -9223372036854775808.0 {
		return true
	}
	fi := int64(f)
	if fi < i {
		return true
	}
	if fi > i {
		return false
	}
	// fi == i; check if f has a fractional part < 0 (i.e., f < fi)
	return f < float64(fi)
}

// intLessEqualFloat: is int64(i) <= float64(f)?
func intLessEqualFloat(i int64, f float64) bool {
	return !floatLessInt(f, i)
}

// floatLessEqualInt: is float64(f) <= int64(i)?
func floatLessEqualInt(f float64, i int64) bool {
	return !intLessFloat(i, f)
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
	if a.IsFloat() && b.IsFloat() {
		return float64(a.GetFloat()) == float64(b.GetFloat())
	}
	// Mixed int/float comparison (Lua 5.4 semantics):
	// Convert the float to int64. If exact match, compare as integers.
	// Otherwise they are NOT equal (avoids precision loss).
	if a.IsFloat() && b.IsInteger() {
		f := float64(a.GetFloat())
		fi := int64(f)
		if f == float64(fi) {
			return types.LuaInteger(fi) == b.GetInteger()
		}
		return false
	}
	if a.IsInteger() && b.IsFloat() {
		f := float64(b.GetFloat())
		fi := int64(f)
		if f == float64(fi) {
			return a.GetInteger() == types.LuaInteger(fi)
		}
		return false
	}
	if a.IsBoolean() && b.IsBoolean() {
		return a.IsTrue() == b.IsTrue()
	}
	if a.IsString() && b.IsString() {
		sa, _ := a.GetValue().(string)
		sb, _ := b.GetValue().(string)
		return sa == sb
	}
	// Tables and functions compare by reference identity (pointer equality).
	// Two table/function values are equal iff they are the SAME object.
	if a.IsTable() && b.IsTable() {
		return a.Value.Data_ == b.Value.Data_
	}
	if a.IsFunction() && b.IsFunction() {
		return a.Value.Data_ == b.Value.Data_
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
	// String-to-number coercion for arithmetic (standard Lua behavior)
	if tval.IsString() {
		if n, ok := coerceStringToNumber(tval); ok {
			return n
		}
	}
	return 0
}

// coerceStringToNumber attempts to convert a string TValue to a float.
// Returns the float value and true if successful.
func coerceStringToNumber(tval *TValue) (types.LuaNumber, bool) {
	s, ok := tval.Value.Data_.(string)
	if !ok {
		return 0, false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// Try parsing as float (handles "3e0", "0.5", etc.)
	f, err := strconv.ParseFloat(s, 64)
	if err == nil {
		return types.LuaNumber(f), true
	}
	// Try parsing hex: 0x or 0X prefix (integers and hex floats)
	if len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		// Try hex integer first
		i, err := strconv.ParseInt(s[2:], 16, 64)
		if err == nil {
			return types.LuaNumber(i), true
		}
		// Try hex float (0xAA.0, 0xF0.ABp2, etc.)
		if hf, ok := parseHexFloatString(s[2:]); ok {
			return types.LuaNumber(hf), true
		}
	}
	return 0, false
}

// coerceToInteger attempts to convert a TValue to an integer.
// Handles integers, floats (if whole), and strings.
func coerceToInteger(tval *TValue) (types.LuaInteger, bool) {
	if tval.IsInteger() {
		return tval.GetInteger(), true
	}
	if tval.IsFloat() {
		f := float64(tval.GetFloat())
		i := int64(f)
		if f == float64(i) {
			return types.LuaInteger(i), true
		}
		return 0, false
	}
	if tval.IsString() {
		s, ok := tval.Value.Data_.(string)
		if !ok {
			return 0, false
		}
		s = strings.TrimSpace(s)
		// Try decimal integer first (base 10, no octal)
		i, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			return types.LuaInteger(i), true
		}
		// Try hex integer
		if len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
			hi, herr := strconv.ParseInt(s[2:], 16, 64)
			if herr == nil {
				return types.LuaInteger(hi), true
			}
			// Try hex float → truncate to int
			if hf, ok := parseHexFloatString(s[2:]); ok {
				fi := int64(hf)
				if hf == float64(fi) {
					return types.LuaInteger(fi), true
				}
			}
		}
		// Try float, convert to int if whole
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr == nil {
			fi := int64(f)
			if f == float64(fi) {
				return types.LuaInteger(fi), true
			}
		}
		return 0, false
	}
	return 0, false
}

// parseHexFloatString parses a hex float string (after the 0x prefix).
// Supports: F0.0, F0.ABp2, .FF, FF, FFp-2
func parseHexFloatString(s string) (float64, bool) {
	s = strings.ToLower(s)
	var intPart, fracPart string
	var expPart string

	// Split on 'p' for binary exponent
	if idx := strings.IndexByte(s, 'p'); idx >= 0 {
		expPart = s[idx+1:]
		s = s[:idx]
	}

	// Split on '.' for integer and fractional parts
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		intPart = s[:idx]
		fracPart = s[idx+1:]
	} else {
		intPart = s
	}

	if intPart == "" && fracPart == "" {
		return 0, false
	}

	// Parse integer part
	var result float64
	if intPart != "" {
		iv, err := strconv.ParseUint(intPart, 16, 64)
		if err != nil {
			return 0, false
		}
		result = float64(iv)
	}

	// Parse fractional part
	if fracPart != "" {
		frac := 0.0
		for i, c := range fracPart {
			var digit float64
			if c >= '0' && c <= '9' {
				digit = float64(c - '0')
			} else if c >= 'a' && c <= 'f' {
				digit = float64(c - 'a' + 10)
			} else {
				return 0, false
			}
			scale := 1.0
			for j := 0; j <= i; j++ {
				scale *= 16
			}
			frac += digit / scale
		}
		result += frac
	}

	// Apply binary exponent (p/P)
	if expPart != "" {
		exp, err := strconv.ParseInt(expPart, 10, 64)
		if err != nil {
			return result, true
		}
		pow := 1.0
		if exp >= 0 {
			for i := int64(0); i < exp; i++ {
				pow *= 2
			}
		} else {
			for i := int64(0); i > exp; i-- {
				pow /= 2
			}
		}
		result *= pow
	}

	return result, true
}

// isNumeric returns true if the TValue is a number or a string that can be coerced to a number.
func isNumeric(tval *TValue) bool {
	return tval.IsInteger() || tval.IsFloat() || (tval.IsString() && canCoerceToNumber(tval))
}

func canCoerceToNumber(tval *TValue) bool {
	_, ok := coerceStringToNumber(tval)
	return ok
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

// handlePcall implements pcall(f, ...) at the VM level.
// Stack layout: stack[base]=pcall, stack[base+1]=f, stack[base+2..]=args
// After: stack[base]=true/false, stack[base+1..]=results/errmsg
func (e *Executor) handlePcall(base, nArgs, nResults int) bool {
// nArgs includes pcall itself in the count from OP_CALL's B field
	// stack[base] = pcall, stack[base+1] = f, stack[base+2..] = extra args
	// The function to call is at base+1, with (nArgs-2) extra args
	// (nArgs counts: pcall + f + extra_args, but B field is nArgs+1 for CALL)
	// Actually: from OP_CALL, B = nArgs which is total args INCLUDING function slot
	// So nArgs = B = total slots. pcall is at base, f is at base+1.
	// The inner call has (nArgs - 2) extra args: stack[base+2..base+nArgs-1]
	
	fnBase := base + 1
	innerNArgs := nArgs - 1 // f + its args (subtract pcall slot)
	if innerNArgs < 1 {
		// pcall() with no arguments — error
		dst := e.reg(base)
		dst.Tt = uint8(types.LUA_VFALSE)
		dst.Value.Data_ = nil
		dst2 := e.reg(base + 1)
		*dst2 = bcConstantToTValue(&bcapi.Constant{Type: bcapi.ConstString, Str: "bad argument #1 to 'pcall' (value expected)"})
		return true
	}

	// Use protectedCall which wraps executeCall + VM loop in recover
	_, errVal := e.protectedCall(fnBase, innerNArgs, nResults)
	
	if errVal != nil {
		// Error occurred — write false + error message at base
		dst := e.reg(base)
		dst.Tt = uint8(types.LUA_VFALSE)
		dst.Value.Data_ = nil
		
		// Extract error message
		dst2 := e.reg(base + 1)
		switch ev := errVal.(type) {
		case *luaErrorValue:
			// LuaError from error() — extract the message TValue
			variant, data := extractVariantAndData(ev.msg)
			dst2.Tt = uint8(ev.msg.GetTag())
			dst2.Value.Variant = variant
			dst2.Value.Data_ = data
		case string:
			*dst2 = bcConstantToTValue(&bcapi.Constant{Type: bcapi.ConstString, Str: ev})
		default:
			*dst2 = bcConstantToTValue(&bcapi.Constant{Type: bcapi.ConstString, Str: fmt.Sprintf("%v", ev)})
		}
		return true
	}
	
	// Success — results are already at fnBase (the function slot was overwritten by return values)
	// We need to shift: stack[base] = true, stack[base+1..] = results from fnBase
	// But results from the inner call are at fnBase-1 = base (the pcall slot) per OP_RETURN logic
	// Actually, OP_RETURN copies results to calleeBase-1. calleeBase = fnBase+1 (base+2).
	// So results go to base+1. We need base=true, base+1..=results.
	// The results are already at base (the pcall slot) from OP_RETURN.
	// We need to shift everything right by 1 and put true at base.
	
	// Actually: protectedCall runs executeCall which for Lua closures pushes a frame
	// with base=fnBase+1, then runs VM loop. OP_RETURN copies results to calleeBase-1=fnBase.
	// So after protectedCall, results are at fnBase = base+1. Perfect!
	// We just need to set base = true.
	
	dst := e.reg(base)
	dst.Tt = uint8(types.LUA_VTRUE)
	dst.Value.Data_ = nil
	return true
}

// handleXpcall implements xpcall(f, handler, ...) at the VM level.
// Stack layout: stack[base]=xpcall, stack[base+1]=f, stack[base+2]=handler, stack[base+3..]=args
func (e *Executor) handleXpcall(base, nArgs, nResults int) bool {
	if nArgs < 3 {
		dst := e.reg(base)
		dst.Tt = uint8(types.LUA_VFALSE)
		dst.Value.Data_ = nil
		dst2 := e.reg(base + 1)
		*dst2 = bcConstantToTValue(&bcapi.Constant{Type: bcapi.ConstString, Str: "bad argument #1 to 'xpcall' (value expected)"})
		return true
	}

	// Save handler before overwriting stack
	handler := *e.reg(base + 2)

	// Move f and args to be contiguous: f at base+1, args at base+2..
	// Currently: [xpcall, f, handler, arg1, arg2, ...]
	// Need:      [xpcall, f, arg1, arg2, ...]
	// Shift args left over handler slot
	for i := base + 2; i < base + nArgs - 1; i++ {
		src := e.reg(i + 1)
		dst := e.reg(i)
		*dst = *src
	}

	fnBase := base + 1
	innerNArgs := nArgs - 2 // subtract xpcall and handler slots

	_, errVal := e.protectedCall(fnBase, innerNArgs, nResults)

	if errVal != nil {
		// Error — call handler with error message
		var errTValue TValue
		switch ev := errVal.(type) {
		case *luaErrorValue:
			variant, data := extractVariantAndData(ev.msg)
			errTValue.Tt = uint8(ev.msg.GetTag())
			errTValue.Value.Variant = variant
			errTValue.Value.Data_ = data
		case string:
			errTValue = bcConstantToTValue(&bcapi.Constant{Type: bcapi.ConstString, Str: ev})
		default:
			errTValue = bcConstantToTValue(&bcapi.Constant{Type: bcapi.ConstString, Str: fmt.Sprintf("%v", ev)})
		}

		// Try to call handler(errMsg)
		// Put handler at base+1, errMsg at base+2
		hDst := e.reg(base + 1)
		*hDst = handler
		eDst := e.reg(base + 2)
		*eDst = errTValue

		_, handlerErr := e.protectedCall(base+1, 2, 1)
		
		dst := e.reg(base)
		dst.Tt = uint8(types.LUA_VFALSE)
		dst.Value.Data_ = nil
		
		if handlerErr != nil {
			// Handler also failed — use original error
			dst2 := e.reg(base + 1)
			*dst2 = errTValue
		}
		// else handler result is already at base+1
		return true
	}

	// Success
	dst := e.reg(base)
	dst.Tt = uint8(types.LUA_VTRUE)
	dst.Value.Data_ = nil
	return true
}

// luaErrorValue wraps a LuaError panic value for type-safe extraction in handlePcall.
type luaErrorValue struct {
	msg types.TValue
}
