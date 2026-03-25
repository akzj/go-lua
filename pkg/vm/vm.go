// Package vm implements the Lua virtual machine core.
//
// The VM is a register-based virtual machine that executes Lua bytecode.
// It features:
//   - Stack-based execution with register windows
//   - RK mode for constant/register operands
//   - CallInfo management for function calls
//   - Upvalue support for closures
//
// # Execution Model
//
// The VM executes bytecode in a fetch-decode-execute loop:
//  1. Fetch instruction from current PC
//  2. Decode instruction format (iABC, iABx, iAsBx, iAx)
//  3. Execute instruction semantics
//  4. Advance PC (unless instruction modifies it)
//
// # RK Mode
//
// Many instructions use RK mode for B/C operands:
//   - If value < 256: register index R(value)
//   - If value >= 256: constant index K(value - 256)
//
// # Stack Layout
//
// Each function call has a stack frame:
//   [0..Base-1]    : Caller's frame
//   [Base]         : Function value
//   [Base+1..]     : Function parameters and locals
//   [StackTop]     : First free slot
package vm

import (
	"fmt"
	"math"
	"math/bits"

	"github.com/akzj/go-lua/pkg/object"
	"github.com/akzj/go-lua/pkg/state"
)

// UpvalueDesc describes an upvalue in a prototype.
type UpvalueDesc struct {
	Index   int  // Stack index or parent upvalue index
	IsLocal bool // True if upvalue is a local variable
}

// Prototype represents a Lua function prototype.
//
// A prototype contains all information needed to execute a function:
//   - Code: Bytecode instructions
//   - Constants: Constant table (numbers, strings, etc.)
//   - Upvalues: Upvalue declarations
//   - Top: Stack top for this call
//   - PC: Program counter within the function
//   - NResults: Expected number of results

//   - NResults: Expected number of results
type CallInfo struct {
	Func      *object.TValue // Function being called
	Base      int            // Stack base for this call
	Top       int            // Stack top for this call
	PC        int            // Program counter
	NResults  int            // Expected number of results
	Status    CallStatus     // Call status
}

// CallStatus represents the status of a call.
type CallStatus int

const (
	// CallOK indicates normal call execution.
	CallOK CallStatus = iota
	// CallYield indicates the call was yielded.
	CallYield
	// CallError indicates the call encountered an error.
	CallError
)

// VM represents a Lua thread/state.
//
// The VM is the core execution engine for Lua bytecode.
// It maintains:
//   - Stack: Value stack for function execution
//   - CallInfo: Stack of call frames
//   - Prototype: Current function being executed
//   - Global: Reference to global state
type VM struct {
	// Stack
	Stack     []object.TValue // Value stack
	StackTop  int             // Current stack top
	StackSize int             // Current stack size

	// Execution state
	PC   int // Program counter
	Base int // Current function base

	// Call info
	CallInfo []*CallInfo // Call info stack
	CI       int         // Current call info index

	// Function
	Prototype *object.Prototype // Current function prototype

	// Global state reference
	Global *state.GlobalState

	// Open upvalues
	OpenUpvalues map[int]*Upvalue

	// To-be-closed variables
	TBCList []int
}

// Upvalue represents an open upvalue.
//
// An upvalue is a reference to a local variable from an enclosing function.
type Upvalue struct {
	Index  int            // Stack index
	Value  *object.TValue // Pointer to the value (when open)
	Closed object.TValue  // Cached value (when closed)
}

// Close closes the upvalue, caching its current value.
func (u *Upvalue) Close() {
	if u.Value != nil {
		u.Closed.CopyFrom(u.Value)
	}
	u.Value = nil
}

// NewVM creates a new VM instance.
//
// Parameters:
//   - global: Reference to the global state
//
// Returns a new VM with initialized stack and call info.
func NewVM(global *state.GlobalState) *VM {
	return &VM{
		Stack:        make([]object.TValue, 2048),
		StackSize:    2048,
		CallInfo:     make([]*CallInfo, 256),
		Global:       global,
		OpenUpvalues: make(map[int]*Upvalue),
	}
}

// Run starts executing bytecode from the current PC.
//
// This method executes instructions in a loop until:
//   - End of bytecode is reached
//   - An error occurs
//   - A return instruction is executed
//
// Returns:
//   - error: Any error that occurred during execution
func (vm *VM) Run() error {
	for {
		if vm.PC >= len(vm.Prototype.Code) {
			break
		}

		instr := Instruction(vm.Prototype.Code[vm.PC])
		vm.PC++

		if err := vm.ExecuteInstruction(instr); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteInstruction executes a single instruction.
//
// This is the core dispatch function that decodes and executes
// one bytecode instruction. It handles all Lua opcodes.
//
// Parameters:
//   - instr: The instruction to execute
//
// Returns:
//   - error: Any error that occurred during execution
func (vm *VM) ExecuteInstruction(instr Instruction) error {
	op := instr.Opcode()

	switch op {
	// Data loading instructions
	case OP_MOVE:
		a, b := instr.A(), instr.B()
		vm.Stack[vm.Base+a].CopyFrom(&vm.Stack[vm.Base+b])

	case OP_LOADI:
		a, sbx := instr.A(), instr.SBx()
		vm.Stack[vm.Base+a].SetNumber(float64(sbx))

	case OP_LOADF:
		a, sbx := instr.A(), instr.SBx()
		vm.Stack[vm.Base+a].SetNumber(float64(sbx))

	case OP_LOADK:
		a, bx := instr.A(), instr.Bx()
		vm.Stack[vm.Base+a].CopyFrom(&vm.Prototype.Constants[bx])

	case OP_LOADKX:
		a := instr.A()
		// Next instruction is EXTRAARG with constant index
		nextInstr := Instruction(vm.Prototype.Code[vm.PC])
		vm.PC++
		ax := nextInstr.Ax()
		vm.Stack[vm.Base+a].CopyFrom(&vm.Prototype.Constants[ax])

	case OP_LOADBOOL:
		a, b, c := instr.A(), instr.B(), instr.C()
		vm.Stack[vm.Base+a].SetBoolean(b != 0)
		if c != 0 {
			vm.PC++ // Skip next instruction
		}

	case OP_LOADNIL:
		a, b := instr.A(), instr.B()
		for i := a; i <= b; i++ {
			vm.Stack[vm.Base+i].SetNil()
		}

	// Arithmetic instructions
	case OP_ADD:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := rb.Value.Num + rc.Value.Num
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_SUB:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := rb.Value.Num - rc.Value.Num
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_MUL:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := rb.Value.Num * rc.Value.Num
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_DIV:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := rb.Value.Num / rc.Value.Num
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_MOD:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := math.Mod(rb.Value.Num, rc.Value.Num)
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_POW:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := math.Pow(rb.Value.Num, rc.Value.Num)
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_IDIV:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := math.Floor(rb.Value.Num / rc.Value.Num)
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_UNM:
		a, b := instr.A(), instr.B()
		rb := vm.getStackValue(vm.Base + b)
		result := -rb.Value.Num
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_BAND:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := float64(int64(rb.Value.Num) & int64(rc.Value.Num))
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_BOR:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := float64(int64(rb.Value.Num) | int64(rc.Value.Num))
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_BXOR:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := float64(int64(rb.Value.Num) ^ int64(rc.Value.Num))
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_SHL:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := float64(int64(rb.Value.Num) << uint64(int64(rc.Value.Num)))
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_SHR:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := float64(uint64(int64(rb.Value.Num)) >> uint64(int64(rc.Value.Num)))
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_BNOT:
		a, b := instr.A(), instr.B()
		rb := vm.getStackValue(vm.Base + b)
		result := float64(^int64(rb.Value.Num))
		vm.Stack[vm.Base+a].SetNumber(result)

	// Comparison instructions
	case OP_EQ:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		rc := vm.getStackValue(vm.Base + c)
		result := object.Equal(rb, rc)
		if result != (a != 0) {
			vm.PC++ // Skip next instruction
		}

	case OP_LT:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		rc := vm.getStackValue(vm.Base + c)
		result := rb.Value.Num < rc.Value.Num
		if result != (a != 0) {
			vm.PC++ // Skip next instruction
		}

	case OP_LE:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		rc := vm.getStackValue(vm.Base + c)
		result := rb.Value.Num <= rc.Value.Num
		if result != (a != 0) {
			vm.PC++ // Skip next instruction
		}

	case OP_EQI:
		a, c := instr.A(), instr.C()
		b := instr.B()
		ra := vm.getStackValue(vm.Base + a)
		kc := vm.getRKValue(c)
		result := object.Equal(ra, kc)
		if result != (b != 0) {
			vm.PC++ // Skip next instruction
		}

	case OP_LTI:
		a, c := instr.A(), instr.C()
		b := instr.B()
		ra := vm.getStackValue(vm.Base + a)
		kc := vm.getRKValue(c)
		result := ra.Value.Num < kc.Value.Num
		if result != (b != 0) {
			vm.PC++ // Skip next instruction
		}

	case OP_LEI:
		a, c := instr.A(), instr.C()
		b := instr.B()
		ra := vm.getStackValue(vm.Base + a)
		kc := vm.getRKValue(c)
		result := ra.Value.Num <= kc.Value.Num
		if result != (b != 0) {
			vm.PC++ // Skip next instruction
		}

	case OP_GTI:
		a, c := instr.A(), instr.C()
		b := instr.B()
		ra := vm.getStackValue(vm.Base + a)
		kc := vm.getRKValue(c)
		result := ra.Value.Num > kc.Value.Num
		if result != (b != 0) {
			vm.PC++ // Skip next instruction
		}

	// Control flow instructions
	case OP_JMP:
		sbx := instr.SBx()
		vm.PC += sbx

	case OP_TEST:
		a, c := instr.A(), instr.C()
		ra := vm.getStackValue(vm.Base + a)
		// In Lua, only nil and false are "falsy"
		isTruthy := !(ra.IsNil() || (ra.IsBoolean() && !ra.Value.Bool))
		if isTruthy != (c != 0) {
			vm.PC++ // Skip next instruction
		}

	case OP_CLOSE:
		a := instr.A()
		vm.closeUpvalues(vm.Base + a)

	case OP_TBC:
		a := instr.A()
		// Mark variable as to-be-closed
		vm.TBCList = append(vm.TBCList, vm.Base+a)

	// Table instructions
	case OP_NEWTABLE:
		a, b, c := instr.A(), instr.B(), instr.C()
		// Decode array size from b and c
		arraySize := decodeSize(b)
		mapSize := decodeSize(c)
		t := object.NewTableWithSize(arraySize, mapSize)
		vm.Stack[vm.Base+a].SetTable(t)

	case OP_GETTABLE:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		rc := vm.getRKValue(c)
		if rb.IsTable() {
			t, _ := rb.ToTable()
			val := t.Get(*rc)
			if val != nil {
				vm.Stack[vm.Base+a].CopyFrom(val)
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		} else {
			return fmt.Errorf("attempt to index a non-table value")
		}

	case OP_SETTABLE:
		a, b, c := instr.A(), instr.B(), instr.C()
		ra := vm.getStackValue(vm.Base + a)
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		if ra.IsTable() {
			t, _ := ra.ToTable()
			t.Set(*rb, *rc)
		} else {
			return fmt.Errorf("attempt to index a non-table value")
		}

	case OP_GETI:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		if rb.IsTable() {
			t, _ := rb.ToTable()
			val := t.GetI(c)
			if val != nil {
				vm.Stack[vm.Base+a].CopyFrom(val)
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		} else {
			return fmt.Errorf("attempt to index a non-table value")
		}

	case OP_SETI:
		a, b, c := instr.A(), instr.B(), instr.C()
		ra := vm.getStackValue(vm.Base + a)
		rb := vm.getRKValue(b)
		if ra.IsTable() {
			t, _ := ra.ToTable()
			t.SetI(c, *rb)
		} else {
			return fmt.Errorf("attempt to index a non-table value")
		}

	case OP_GETFIELD:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		if rb.IsTable() {
			t, _ := rb.ToTable()
			key := vm.Prototype.Constants[c]
			val := t.Get(key)
			if val != nil {
				vm.Stack[vm.Base+a].CopyFrom(val)
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		} else {
			return fmt.Errorf("attempt to index a non-table value")
		}

	case OP_SETFIELD:
		a, b, c := instr.A(), instr.B(), instr.C()
		ra := vm.getStackValue(vm.Base + a)
		if ra.IsTable() {
			t, _ := ra.ToTable()
			key := vm.Prototype.Constants[b]
			rc := vm.getRKValue(c)
			t.Set(key, *rc)
		} else {
			return fmt.Errorf("attempt to index a non-table value")
		}

	// Other instructions
	case OP_CONCAT:
		a, b, c := instr.A(), instr.B(), instr.C()
		var result string
		for i := b; i <= c; i++ {
			rv := vm.getStackValue(vm.Base + i)
			if s, ok := rv.ToString(); ok {
				result += s
			} else {
				result += object.ToStringRaw(rv)
			}
		}
		vm.Stack[vm.Base+a].SetString(result)

	case OP_LEN:
		a, b := instr.A(), instr.B()
		rb := vm.getStackValue(vm.Base + b)
		if rb.IsTable() {
			t, _ := rb.ToTable()
			vm.Stack[vm.Base+a].SetNumber(float64(t.Len()))
		} else if rb.IsString() {
			s, _ := rb.ToString()
			vm.Stack[vm.Base+a].SetNumber(float64(len(s)))
		} else {
			return fmt.Errorf("attempt to get length of a non-table/string value")
		}

	case OP_NOT:
		a, b := instr.A(), instr.B()
		rb := vm.getStackValue(vm.Base + b)
		// In Lua, only nil and false are "falsy"
		isTruthy := !(rb.IsNil() || (rb.IsBoolean() && !rb.Value.Bool))
		vm.Stack[vm.Base+a].SetBoolean(!isTruthy)

	case OP_SELF:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		rc := vm.getRKValue(c)
		// R(A+1) := R(B)
		vm.Stack[vm.Base+a+1].CopyFrom(rb)
		// R(A) := R(B)[RK(C)]
		if rb.IsTable() {
			t, _ := rb.ToTable()
			val := t.Get(*rc)
			if val != nil {
				vm.Stack[vm.Base+a].CopyFrom(val)
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		} else {
			return fmt.Errorf("attempt to index a non-table value")
		}

	case OP_ADDI:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		// c is signed in ADDI
		sc := int16(c)
		result := rb.Value.Num + float64(sc)
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_RETURN:
		a, b := instr.A(), instr.B()
		// b=0: return all values from R(A) to top
		// b>0: return b-1 values starting from R(A)
		// Get call info
		if vm.CI > 0 {
			// Close upvalues
			vm.closeUpvalues(vm.Base)

			// Get number of results to return
			var nResults int
			if b == 0 {
				// Return all values from R(A) to top
				nResults = vm.StackTop - (vm.Base + a)
			} else {
				nResults = b - 1
			}

			// Move results to caller's stack
			if nResults > 0 {
				// Copy results to just above the function
				for i := 0; i < nResults; i++ {
					vm.Stack[vm.Base+i] = vm.Stack[vm.Base+a+i]
				}
			}

			// Restore previous call info
			vm.CI--
			
			// Check if we're back to the initial call (CI == 0)
			if vm.CI == 0 {
				// Top-level return, stop execution
				vm.StackTop = vm.Base + nResults
				vm.PC = len(vm.Prototype.Code)
			} else {
				// Return to caller
				prevBase := vm.CallInfo[vm.CI].Base
				vm.Base = prevBase
				vm.PC = vm.CallInfo[vm.CI].PC
				vm.Prototype = vm.CallInfo[vm.CI].Func.ToFunctionProto()

				// Set stack top to after results
				vm.StackTop = vm.Base + nResults
			}
		} else {
			// Top-level return, stop execution
			vm.PC = len(vm.Prototype.Code)
		}

	default:
		return fmt.Errorf("unimplemented opcode: %d (%s)", op, op.String())
	}

	return nil
}

// getRKValue gets a value from register or constant using RK mode.
//
// RK mode is used for B/C operands in many instructions:
//   - If index < 256: register R(index)
//   - If index >= 256: constant K(index - 256)
//
// Parameters:
//   - index: The raw B/C field value from the instruction
//
// Returns:
//   - *object.TValue: The resolved value
func (vm *VM) getRKValue(index int) *object.TValue {
	if index < 256 {
		// Register
		return &vm.Stack[vm.Base+index]
	}
	// Constant
	kIndex := index - 256
	return &vm.Prototype.Constants[kIndex]
}

// getStackValue gets a value from the stack.
//
// This is a simple stack access without RK resolution.
//
// Parameters:
//   - index: Absolute stack index
//
// Returns:
//   - *object.TValue: The value at the stack index
func (vm *VM) getStackValue(index int) *object.TValue {
	return &vm.Stack[index]
}

// closeUpvalues closes all open upvalues at or above the given index.
//
// When a function returns, all upvalues that reference stack slots
// at or above the return base must be closed (their values cached).
//
// Parameters:
//   - index: The stack index at or above which to close upvalues
func (vm *VM) closeUpvalues(index int) {
	for idx, upvalue := range vm.OpenUpvalues {
		if upvalue.Index >= index {
			upvalue.Close()
			delete(vm.OpenUpvalues, idx)
		}
	}
}

// decodeSize decodes a size field from instruction encoding.
//
// Lua encodes sizes in a special format where:
//   - 0 means 0
//   - Other values are encoded with an exponent
func decodeSize(x int) int {
	if x == 0 {
		return 0
	}
	// Simple decoding: use the value directly
	// In real Lua, there's a more complex encoding
	return x
}

// Call calls a function.
//
// This method sets up a new call frame for function execution.
//
// Parameters:
//   - funcIdx: Stack index of the function to call
//   - nargs: Number of arguments
//   - nresults: Expected number of results
//
// Returns:
//   - error: Any error that occurred during call setup
func (vm *VM) Call(funcIdx int, nargs, nresults int) error {
	// Get function value
	var funcVal *object.TValue
	if funcIdx >= 0 {
		funcVal = &vm.Stack[vm.Base+funcIdx]
	} else {
		funcVal = &vm.Stack[vm.StackTop+funcIdx+1]
	}

	// Check if it's a Lua function
	if !funcVal.IsFunction() {
		return fmt.Errorf("attempt to call a non-function value")
	}

	fn, ok := funcVal.ToFunction()
	if !ok {
		return fmt.Errorf("attempt to call a non-function value")
	}

	if fn.IsGo {
		// Go function call
		// Remember the base where function and args are
		funcBase := vm.Base + funcIdx
		if funcIdx < 0 {
			funcBase = vm.StackTop + funcIdx + 1
		}
		
		// Temporarily adjust base so that args are at indices 1, 2, 3...
		// (function is at funcBase, args start at funcBase+1)
		oldBase := vm.Base
		vm.Base = funcBase + 1
		
		// Call the Go function
		err := fn.GoFn(vm)
		
		// Restore base
		vm.Base = oldBase
		
		if err != nil {
			return err
		}
		
		// Adjust stack: remove function and args, keep nresults
		// Results are on top of stack now
		if nresults >= 0 {
			// Move results to function position
			resultStart := vm.StackTop - nresults
			for i := 0; i < nresults; i++ {
				vm.Stack[funcBase+i] = vm.Stack[resultStart+i]
			}
			vm.StackTop = funcBase + nresults
		} else {
			// All results: move them to function position
			numResults := vm.StackTop - (funcBase + 1 + nargs)
			for i := 0; i < numResults; i++ {
				vm.Stack[funcBase+i] = vm.Stack[funcBase+1+nargs+i]
			}
			vm.StackTop = funcBase + numResults
		}
		
		return nil
	}

	// Lua function call
	proto := fn.Proto
	if proto == nil {
		return fmt.Errorf("function has no prototype")
	}

	// Initialize CallInfo[0] if this is the first call
	if vm.CI == 0 {
		vm.CallInfo[0] = &CallInfo{
			Func:     funcVal,
			Base:     vm.Base,
			Top:      vm.StackTop,
			PC:       0,
			NResults: nresults,
			Status:   CallOK,
		}
	}

	// Create new CallInfo
	vm.CI++
	if vm.CI >= len(vm.CallInfo) {
		// Grow call info stack
		newCallInfo := make([]*CallInfo, len(vm.CallInfo)*2)
		copy(newCallInfo, vm.CallInfo)
		vm.CallInfo = newCallInfo
	}

	newBase := vm.Base + funcIdx + 1
	vm.CallInfo[vm.CI] = &CallInfo{
		Func:     funcVal,
		Base:     newBase,
		Top:      newBase + nargs + proto.MaxStackSize,
		PC:       0,
		NResults: nresults,
		Status:   CallOK,
	}

	// Set up new execution context
	vm.Base = newBase
	vm.Prototype = proto
	vm.PC = 0

	return nil
}

// GetStack returns the value at stack index.
//
// Stack indices can be positive (from base, 1-based) or negative (from top).
//
// Parameters:
//   - index: Stack index (positive from base, negative from top)
//
// Returns:
//   - *object.TValue: The value at the stack index
func (vm *VM) GetStack(index int) *object.TValue {
	if index > 0 {
		// Positive indices are 1-based (Lua convention)
		return &vm.Stack[vm.Base+index-1]
	} else if index < 0 {
		// Negative indices are from top (-1 = top)
		return &vm.Stack[vm.StackTop+index]
	}
	// index == 0 is invalid in Lua, return first element
	return &vm.Stack[vm.Base]
}

// SetStack sets the value at stack index.
//
// Parameters:
//   - index: Stack index (positive from base, negative from top)
//   - value: The value to set
func (vm *VM) SetStack(index int, value object.TValue) {
	if index > 0 {
		vm.Stack[vm.Base+index-1] = value
	} else if index < 0 {
		vm.Stack[vm.StackTop+index] = value
	} else {
		vm.Stack[vm.Base] = value
	}
}

// Push pushes a value onto the stack.
//
// Parameters:
//   - value: The value to push
func (vm *VM) Push(value object.TValue) {
	vm.Stack[vm.StackTop] = value
	vm.StackTop++
}

// Pop pops a value from the stack.
//
// Returns:
//   - object.TValue: The popped value
func (vm *VM) Pop() object.TValue {
	vm.StackTop--
	return vm.Stack[vm.StackTop]
}

// GetTop returns the current stack top index.
//
// Returns:
//   - int: The stack top index
func (vm *VM) GetTop() int {
	return vm.StackTop - vm.Base
}

// SetTop sets the stack top.
//
// Parameters:
//   - index: The new stack top index
func (vm *VM) SetTop(index int) {
	if index > vm.StackTop-vm.Base {
		// Extending stack, fill new slots with nil
		for i := vm.StackTop; i < vm.Base+index; i++ {
			vm.Stack[i].SetNil()
		}
	}
	vm.StackTop = vm.Base + index
}

// Opcode String returns a human-readable name for the opcode.
func (op Opcode) String() string {
	switch op {
	case OP_MOVE:
		return "MOVE"
	case OP_LOADI:
		return "LOADI"
	case OP_LOADF:
		return "LOADF"
	case OP_LOADK:
		return "LOADK"
	case OP_LOADKX:
		return "LOADKX"
	case OP_LOADBOOL:
		return "LOADBOOL"
	case OP_LOADNIL:
		return "LOADNIL"
	case OP_GETUPVAL:
		return "GETUPVAL"
	case OP_SETUPVAL:
		return "SETUPVAL"
	case OP_GETTABUP:
		return "GETTABUP"
	case OP_GETTABLE:
		return "GETTABLE"
	case OP_GETI:
		return "GETI"
	case OP_GETFIELD:
		return "GETFIELD"
	case OP_SETTABUP:
		return "SETTABUP"
	case OP_SETTABLE:
		return "SETTABLE"
	case OP_SETI:
		return "SETI"
	case OP_SETFIELD:
		return "SETFIELD"
	case OP_NEWTABLE:
		return "NEWTABLE"
	case OP_SELF:
		return "SELF"
	case OP_ADDI:
		return "ADDI"
	case OP_ADD:
		return "ADD"
	case OP_SUB:
		return "SUB"
	case OP_MUL:
		return "MUL"
	case OP_MOD:
		return "MOD"
	case OP_POW:
		return "POW"
	case OP_DIV:
		return "DIV"
	case OP_IDIV:
		return "IDIV"
	case OP_BAND:
		return "BAND"
	case OP_BOR:
		return "BOR"
	case OP_BXOR:
		return "BXOR"
	case OP_SHL:
		return "SHL"
	case OP_SHR:
		return "SHR"
	case OP_UNM:
		return "UNM"
	case OP_BNOT:
		return "BNOT"
	case OP_NOT:
		return "NOT"
	case OP_LEN:
		return "LEN"
	case OP_CONCAT:
		return "CONCAT"
	case OP_CLOSE:
		return "CLOSE"
	case OP_TBC:
		return "TBC"
	case OP_JMP:
		return "JMP"
	case OP_EQ:
		return "EQ"
	case OP_LT:
		return "LT"
	case OP_LE:
		return "LE"
	case OP_EQI:
		return "EQI"
	case OP_LEI:
		return "LEI"
	case OP_LTI:
		return "LTI"
	case OP_GTI:
		return "GTI"
	case OP_TEST:
		return "TEST"
	case OP_FORPREP:
		return "FORPREP"
	case OP_FORLOOP:
		return "FORLOOP"
	case OP_FORGPREP:
		return "FORGPREP"
	case OP_FORGLOOP:
		return "FORGLOOP"
	case OP_SETLIST:
		return "SETLIST"
	case OP_CLOSURE:
		return "CLOSURE"
	case OP_VARARG:
		return "VARARG"
	case OP_VARARGPREP:
		return "VARARGPREP"
	case OP_EXTRAARG:
		return "EXTRAARG"
	case OP_RETURN:
		return "RETURN"
	default:
		return fmt.Sprintf("OP_%d", op)
	}
}

// Bits returns the number of bits set in the instruction.
func (i Instruction) Bits() int {
	return bits.OnesCount32(uint32(i))
}

