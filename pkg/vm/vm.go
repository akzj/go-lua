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
	Func      *object.TValue   // Function being called
	Closure   *object.Closure  // Reference to closure for upvalue access
	Base      int              // Stack base for this call
	Top       int              // Stack top for this call
	PC        int              // Program counter
	NResults  int              // Expected number of results
	Status    CallStatus       // Call status
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

	// Open upvalues — tracks open object.Upvalue instances by stack index
	OpenUpvalues map[int]*object.Upvalue

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
		OpenUpvalues: make(map[int]*object.Upvalue),
	}
}

// getCurrentClosure returns the closure of the currently executing function.
// getCurrentLine returns the current line number from LineInfo, or 0 if unavailable.
func (vm *VM) getCurrentLine() int {
	if vm.Prototype != nil && vm.PC > 0 && vm.PC-1 < len(vm.Prototype.LineInfo) {
		return vm.Prototype.LineInfo[vm.PC-1] // PC-1 because PC was already incremented
	}
	return 0
}

// getCurrentSource returns the current source file name.
func (vm *VM) getCurrentSource() string {
	if vm.Prototype != nil && vm.Prototype.Source != "" {
		source := vm.Prototype.Source
		// Strip the "@" prefix used by LoadFile
		if len(source) > 0 && source[0] == '@' {
			source = source[1:]
		}
		return source
	}
	return "?"
}

// runtimeError creates a formatted runtime error with source location.
func (vm *VM) runtimeError(format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	line := vm.getCurrentLine()
	source := vm.getCurrentSource()
	if line > 0 {
		return fmt.Errorf("%s:%d: %s", source, line, msg)
	}
	return fmt.Errorf("%s: %s", source, msg)
}

func (vm *VM) getCurrentClosure() *object.Closure {
	if vm.CI >= 0 && vm.CallInfo[vm.CI] != nil {
		return vm.CallInfo[vm.CI].Closure
	}
	return nil
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
		// Try __add metamethod
		if result, ok := vm.tryBinaryMetamethod(rb, rc, MetaAdd); ok {
			if result != nil {
				vm.Stack[vm.Base+a].CopyFrom(result)
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		} else {
			result := rb.Value.Num + rc.Value.Num
			vm.Stack[vm.Base+a].SetNumber(result)
		}

	case OP_SUB:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		// Try __sub metamethod
		if result, ok := vm.tryBinaryMetamethod(rb, rc, MetaSub); ok {
			if result != nil {
				vm.Stack[vm.Base+a].CopyFrom(result)
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		} else {
			result := rb.Value.Num - rc.Value.Num
			vm.Stack[vm.Base+a].SetNumber(result)
		}

	case OP_MUL:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		// Try __mul metamethod
		if result, ok := vm.tryBinaryMetamethod(rb, rc, MetaMul); ok {
			if result != nil {
				vm.Stack[vm.Base+a].CopyFrom(result)
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		} else {
			result := rb.Value.Num * rc.Value.Num
			vm.Stack[vm.Base+a].SetNumber(result)
		}

	case OP_DIV:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		// Try __div metamethod
		if result, ok := vm.tryBinaryMetamethod(rb, rc, MetaDiv); ok {
			if result != nil {
				vm.Stack[vm.Base+a].CopyFrom(result)
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		} else {
			result := rb.Value.Num / rc.Value.Num
			vm.Stack[vm.Base+a].SetNumber(result)
		}

	case OP_MOD:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		// Try __mod metamethod
		if result, ok := vm.tryBinaryMetamethod(rb, rc, MetaMod); ok {
			if result != nil {
				vm.Stack[vm.Base+a].CopyFrom(result)
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		} else {
			result := math.Mod(rb.Value.Num, rc.Value.Num)
			vm.Stack[vm.Base+a].SetNumber(result)
		}

	case OP_POW:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		// Try __pow metamethod
		if result, ok := vm.tryBinaryMetamethod(rb, rc, MetaPow); ok {
			if result != nil {
				vm.Stack[vm.Base+a].CopyFrom(result)
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		} else {
			result := math.Pow(rb.Value.Num, rc.Value.Num)
			vm.Stack[vm.Base+a].SetNumber(result)
		}

	case OP_IDIV:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		result := math.Floor(rb.Value.Num / rc.Value.Num)
		vm.Stack[vm.Base+a].SetNumber(result)

	case OP_UNM:
		a, b := instr.A(), instr.B()
		rb := vm.getStackValue(vm.Base + b)
		// Try __unm metamethod
		if result, ok := vm.tryUnaryMetamethod(rb, MetaUnm); ok {
			if result != nil {
				vm.Stack[vm.Base+a].CopyFrom(result)
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		} else {
			result := -rb.Value.Num
			vm.Stack[vm.Base+a].SetNumber(result)
		}

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
		// Try __eq metamethod for tables
		if rb.IsTable() && rc.IsTable() {
			tb, _ := rb.ToTable()
			tc, _ := rc.ToTable()
			mmb := vm.GetMetamethod(tb, MetaEq)
			mmc := vm.GetMetamethod(tc, MetaEq)
			if mmb != nil && mmb.IsFunction() {
				result := vm.CallMetamethod(mmb, []object.TValue{*rb, *rc})
				vm.Stack[vm.Base+a].SetBoolean(result != nil && !object.IsFalse(result))
			} else if mmc != nil && mmc.IsFunction() {
				result := vm.CallMetamethod(mmc, []object.TValue{*rb, *rc})
				vm.Stack[vm.Base+a].SetBoolean(result != nil && !object.IsFalse(result))
			} else {
				// No metamethod - use direct comparison
				result := object.Equal(rb, rc)
				vm.Stack[vm.Base+a].SetBoolean(result)
			}
		} else {
			result := object.Equal(rb, rc)
			vm.Stack[vm.Base+a].SetBoolean(result)
		}

	case OP_LT:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		rc := vm.getStackValue(vm.Base + c)
		// Try __lt metamethod for tables
		if result, ok := vm.tryBinaryMetamethod(rb, rc, MetaLt); ok {
			vm.Stack[vm.Base+a].SetBoolean(result != nil && !object.IsFalse(result))
		} else {
			result := rb.Value.Num < rc.Value.Num
			vm.Stack[vm.Base+a].SetBoolean(result)
		}

	case OP_LE:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		rc := vm.getStackValue(vm.Base + c)
		// Try __le metamethod for tables
		if result, ok := vm.tryBinaryMetamethod(rb, rc, MetaLe); ok {
			vm.Stack[vm.Base+a].SetBoolean(result != nil && !object.IsFalse(result))
		} else {
			result := rb.Value.Num <= rc.Value.Num
			vm.Stack[vm.Base+a].SetBoolean(result)
		}

	case OP_EQI:
		a, c := instr.A(), instr.C()
		b := instr.B()
		ra := vm.getStackValue(vm.Base + a)
		kc := vm.getRKValue(c)
		result := object.Equal(ra, kc)
		// Lua 5.4: store boolean result (considering the 'b' modifier)
		// When b=0, store result directly; when b=1, store negated result
		if (result != false) != (b != 0) {
			vm.Stack[vm.Base+a].SetBoolean(true)
		} else {
			vm.Stack[vm.Base+a].SetBoolean(false)
		}

	case OP_LTI:
		a, c := instr.A(), instr.C()
		b := instr.B()
		ra := vm.getStackValue(vm.Base + a)
		kc := vm.getRKValue(c)
		result := ra.Value.Num < kc.Value.Num
		// Lua 5.4: store boolean result (considering the 'b' modifier)
		if (result != false) != (b != 0) {
			vm.Stack[vm.Base+a].SetBoolean(true)
		} else {
			vm.Stack[vm.Base+a].SetBoolean(false)
		}

	case OP_LEI:
		a, c := instr.A(), instr.C()
		b := instr.B()
		ra := vm.getStackValue(vm.Base + a)
		kc := vm.getRKValue(c)
		result := ra.Value.Num <= kc.Value.Num
		// Lua 5.4: store boolean result (considering the 'b' modifier)
		if (result != false) != (b != 0) {
			vm.Stack[vm.Base+a].SetBoolean(true)
		} else {
			vm.Stack[vm.Base+a].SetBoolean(false)
		}

	case OP_GTI:
		a, c := instr.A(), instr.C()
		b := instr.B()
		ra := vm.getStackValue(vm.Base + a)
		kc := vm.getRKValue(c)
		result := ra.Value.Num > kc.Value.Num
		// Lua 5.4: store boolean result (considering the 'b' modifier)
		if (result != false) != (b != 0) {
			vm.Stack[vm.Base+a].SetBoolean(true)
		} else {
			vm.Stack[vm.Base+a].SetBoolean(false)
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

	case OP_VARARGPREP:
		// Vararg preparation — no-op for now; the main chunk is always vararg
		// and the arguments are already set up by the caller.
		// In a full implementation, this would adjust the stack for vararg functions.

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
			if val != nil && !val.IsNil() {
				vm.Stack[vm.Base+a].CopyFrom(val)
			} else {
				// Key not found - check for __index metamethod
				mm := vm.GetMetamethod(t, MetaIndex)
				if mm != nil {
					if mm.IsTable() {
						// __index is a table - look up in that table
						mt, _ := mm.ToTable()
						mval := mt.Get(*rc)
						if mval != nil {
							vm.Stack[vm.Base+a].CopyFrom(mval)
						} else {
							vm.Stack[vm.Base+a].SetNil()
						}
					} else if mm.IsFunction() {
						// __index is a function - call it: __index(table, key)
						result := vm.CallMetamethod(mm, []object.TValue{*rb, *rc})
						if result != nil {
							vm.Stack[vm.Base+a].CopyFrom(result)
						} else {
							vm.Stack[vm.Base+a].SetNil()
						}
					} else {
						vm.Stack[vm.Base+a].SetNil()
					}
				} else {
					vm.Stack[vm.Base+a].SetNil()
				}
			}
		} else {
			return vm.runtimeError("attempt to index a non-table value")
		}

	case OP_SETTABLE:
		a, b, c := instr.A(), instr.B(), instr.C()
		ra := vm.getStackValue(vm.Base + a)
		rb := vm.getRKValue(b)
		rc := vm.getRKValue(c)
		if ra.IsTable() {
			t, _ := ra.ToTable()
			// Check if key already exists
			existing := t.Get(*rb)
			if existing != nil && !existing.IsNil() {
				// Key exists - update directly
				t.Set(*rb, *rc)
			} else {
				// Key doesn't exist - check for __newindex metamethod
				mm := vm.GetMetamethod(t, MetaNewIndex)
				if mm != nil {
					if mm.IsTable() {
						// __newindex is a table - store there
						mt, _ := mm.ToTable()
						mt.Set(*rb, *rc)
					} else if mm.IsFunction() {
						// __newindex is a function - call it
						vm.CallMetamethod(mm, []object.TValue{*ra, *rb, *rc})
					} else {
						// No valid __newindex - store in original table
						t.Set(*rb, *rc)
					}
				} else {
					// No __newindex - store in table
					t.Set(*rb, *rc)
				}
			}
		} else {
			return vm.runtimeError("attempt to index a non-table value")
		}

	case OP_GETI:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		if rb.IsTable() {
			t, _ := rb.ToTable()
			val := t.GetI(c)
			if val != nil && !val.IsNil() {
				vm.Stack[vm.Base+a].CopyFrom(val)
			} else {
				// Key not found - check for __index metamethod
				mm := vm.GetMetamethod(t, MetaIndex)
				if mm != nil {
					key := object.NewInteger(int64(c))
					if mm.IsTable() {
						mt, _ := mm.ToTable()
						mval := mt.Get(*key)
						if mval != nil {
							vm.Stack[vm.Base+a].CopyFrom(mval)
						} else {
							vm.Stack[vm.Base+a].SetNil()
						}
					} else if mm.IsFunction() {
						result := vm.CallMetamethod(mm, []object.TValue{*rb, *key})
						if result != nil {
							vm.Stack[vm.Base+a].CopyFrom(result)
						} else {
							vm.Stack[vm.Base+a].SetNil()
						}
					} else {
						vm.Stack[vm.Base+a].SetNil()
					}
				} else {
					vm.Stack[vm.Base+a].SetNil()
				}
			}
		} else {
			return vm.runtimeError("attempt to index a non-table value")
		}

	case OP_SETI:
		a, b, c := instr.A(), instr.B(), instr.C()
		ra := vm.getStackValue(vm.Base + a)
		rb := vm.getRKValue(b)
		if ra.IsTable() {
			t, _ := ra.ToTable()
			// Check if key already exists
			existing := t.GetI(c)
			if existing != nil && !existing.IsNil() {
				t.SetI(c, *rb)
			} else {
				// Key doesn't exist - check for __newindex
				mm := vm.GetMetamethod(t, MetaNewIndex)
				if mm != nil {
					key := object.NewInteger(int64(c))
					if mm.IsTable() {
						mt, _ := mm.ToTable()
						mt.Set(*key, *rb)
					} else if mm.IsFunction() {
						vm.CallMetamethod(mm, []object.TValue{*ra, *key, *rb})
					} else {
						t.SetI(c, *rb)
					}
				} else {
					t.SetI(c, *rb)
				}
			}
		} else {
			return vm.runtimeError("attempt to index a non-table value")
		}

	case OP_GETFIELD:
		a, b, c := instr.A(), instr.B(), instr.C()
		rb := vm.getStackValue(vm.Base + b)
		if rb.IsTable() {
			t, _ := rb.ToTable()
			key := vm.Prototype.Constants[c]
			val := t.Get(key)
			if val != nil && !val.IsNil() {
				vm.Stack[vm.Base+a].CopyFrom(val)
			} else {
				// Key not found - check for __index metamethod
				mm := vm.GetMetamethod(t, MetaIndex)
				if mm != nil {
					if mm.IsTable() {
						// __index is a table - look up in that table
						mt, _ := mm.ToTable()
						mval := mt.Get(key)
						if mval != nil {
							vm.Stack[vm.Base+a].CopyFrom(mval)
						} else {
							vm.Stack[vm.Base+a].SetNil()
						}
					} else if mm.IsFunction() {
						// __index is a function - call it: __index(table, key)
						result := vm.CallMetamethod(mm, []object.TValue{*rb, key})
						if result != nil {
							vm.Stack[vm.Base+a].CopyFrom(result)
						} else {
							vm.Stack[vm.Base+a].SetNil()
						}
					} else {
						vm.Stack[vm.Base+a].SetNil()
					}
				} else {
					vm.Stack[vm.Base+a].SetNil()
				}
			}
		} else {
			return vm.runtimeError("attempt to index a non-table value")
		}

	case OP_SETFIELD:
		a, b, c := instr.A(), instr.B(), instr.C()
		ra := vm.getStackValue(vm.Base + a)
		if ra.IsTable() {
			t, _ := ra.ToTable()
			key := vm.Prototype.Constants[b]
			rc := vm.getRKValue(c)
			// Check if key already exists
			existing := t.Get(key)
			if existing != nil && !existing.IsNil() {
				// Key exists - update directly
				t.Set(key, *rc)
			} else {
				// Key doesn't exist - check for __newindex metamethod
				mm := vm.GetMetamethod(t, MetaNewIndex)
				if mm != nil {
					if mm.IsTable() {
						mt, _ := mm.ToTable()
						mt.Set(key, *rc)
					} else if mm.IsFunction() {
						vm.CallMetamethod(mm, []object.TValue{*ra, key, *rc})
					} else {
						t.Set(key, *rc)
					}
				} else {
					t.Set(key, *rc)
				}
			}
		} else {
			return vm.runtimeError("attempt to index a non-table value")
		}

	// Other instructions
	case OP_CONCAT:
		a, b, c := instr.A(), instr.B(), instr.C()
		// Check for __concat metamethod on any operand
		hasConcatMetamethod := false
		for i := b; i <= c; i++ {
			rv := vm.getStackValue(vm.Base + i)
			if rv.IsTable() {
				t, _ := rv.ToTable()
				mm := vm.GetMetamethod(t, MetaConcat)
				if mm != nil && mm.IsFunction() {
					hasConcatMetamethod = true
					break
				}
			}
		}
		
		if hasConcatMetamethod {
			// Use metamethod for concatenation - fold left to right
			result := vm.getStackValue(vm.Base + b)
			for i := b + 1; i <= c; i++ {
				next := vm.getStackValue(vm.Base + i)
				// Find metamethod
				var mm *object.TValue
				if result.IsTable() {
					t, _ := result.ToTable()
					mm = vm.GetMetamethod(t, MetaConcat)
				}
				if mm == nil && next.IsTable() {
					t, _ := next.ToTable()
					mm = vm.GetMetamethod(t, MetaConcat)
				}
				if mm != nil && mm.IsFunction() {
					res := vm.CallMetamethod(mm, []object.TValue{*result, *next})
					if res != nil {
						result = res
					} else {
						// Fallback to string concat
						s1, _ := result.ToString()
						s2, _ := next.ToString()
						result = &object.TValue{}
						result.SetString(s1 + s2)
					}
				} else {
					// Fallback to string concat
					s1, _ := result.ToString()
					s2, _ := next.ToString()
					result = &object.TValue{}
					result.SetString(s1 + s2)
				}
			}
			vm.Stack[vm.Base+a].CopyFrom(result)
		} else {
			// Standard string concatenation
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
		}

	case OP_LEN:
		a, b := instr.A(), instr.B()
		rb := vm.getStackValue(vm.Base + b)
		if rb.IsTable() {
			t, _ := rb.ToTable()
			// Check for __len metamethod
			mm := vm.GetMetamethod(t, MetaLen)
			if mm != nil && mm.IsFunction() {
				result := vm.CallMetamethod(mm, []object.TValue{*rb})
				if result != nil {
					vm.Stack[vm.Base+a].CopyFrom(result)
				} else {
					vm.Stack[vm.Base+a].SetNumber(0)
				}
			} else {
				vm.Stack[vm.Base+a].SetNumber(float64(t.Len()))
			}
		} else if rb.IsString() {
			s, _ := rb.ToString()
			vm.Stack[vm.Base+a].SetNumber(float64(len(s)))
		} else {
			return vm.runtimeError("attempt to get length of a non-table/string value")
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
			return vm.runtimeError("attempt to index a non-table value")
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

			// Move results to caller's stack: replace the closure slot (Base-1) and following slots.
			destBase := vm.Base - 1
			if nResults > 0 {
				for i := 0; i < nResults; i++ {
					vm.Stack[destBase+i] = vm.Stack[vm.Base+a+i]
				}
			}

			vm.CI--

			if vm.CI == 0 {
				vm.Base = vm.CallInfo[0].Base
				vm.StackTop = destBase + nResults
				vm.PC = len(vm.Prototype.Code)
			} else {
				vm.Base = vm.CallInfo[vm.CI].Base
				vm.PC = vm.CallInfo[vm.CI].PC
				vm.Prototype = vm.CallInfo[vm.CI].Closure.Proto
				vm.StackTop = destBase + nResults
			}
		} else {
			// Top-level return, stop execution
			vm.PC = len(vm.Prototype.Code)
		}

	// Upvalue instructions
	case OP_GETUPVAL:
		a, b := instr.A(), instr.B()
		closure := vm.getCurrentClosure()
		if closure != nil && b < len(closure.Upvalues) {
			upval := closure.Upvalues[b]
			val := upval.Get()
			vm.Stack[vm.Base+a].CopyFrom(val)
		} else {
			vm.Stack[vm.Base+a].SetNil()
		}

	case OP_SETUPVAL:
		a, b := instr.A(), instr.B()
		closure := vm.getCurrentClosure()
		if closure != nil && b < len(closure.Upvalues) {
			upval := closure.Upvalues[b]
			upval.Set(&vm.Stack[vm.Base+a])
		}

	case OP_GETTABUP:
		a, b, c := instr.A(), instr.B(), instr.C()
		closure := vm.getCurrentClosure()
		if closure == nil || b >= len(closure.Upvalues) {
			return fmt.Errorf("invalid upvalue index %d", b)
		}
		upval := closure.Upvalues[b]
		upvalVal := upval.Get()
		if !upvalVal.IsTable() {
			return vm.runtimeError("attempt to index a non-table upvalue")
		}
		t, _ := upvalVal.ToTable()
		key := vm.getRKValue(c)
		val := t.Get(*key)
		if val != nil && !val.IsNil() {
			vm.Stack[vm.Base+a].CopyFrom(val)
		} else {
			// Key not found - check for __index metamethod
			mm := vm.GetMetamethod(t, MetaIndex)
			if mm != nil {
				if mm.IsTable() {
					mt, _ := mm.ToTable()
					mval := mt.Get(*key)
					if mval != nil {
						vm.Stack[vm.Base+a].CopyFrom(mval)
					} else {
						vm.Stack[vm.Base+a].SetNil()
					}
				} else if mm.IsFunction() {
					result := vm.CallMetamethod(mm, []object.TValue{*upvalVal, *key})
					if result != nil {
						vm.Stack[vm.Base+a].CopyFrom(result)
					} else {
						vm.Stack[vm.Base+a].SetNil()
					}
				} else {
					vm.Stack[vm.Base+a].SetNil()
				}
			} else {
				vm.Stack[vm.Base+a].SetNil()
			}
		}

	case OP_SETTABUP:
		a, b, c := instr.A(), instr.B(), instr.C()
		closure := vm.getCurrentClosure()
		if closure == nil || a >= len(closure.Upvalues) {
			return fmt.Errorf("invalid upvalue index %d", a)
		}
		upval := closure.Upvalues[a]
		upvalVal := upval.Get()
		if !upvalVal.IsTable() {
			return vm.runtimeError("attempt to index a non-table upvalue")
		}
		t, _ := upvalVal.ToTable()
		key := vm.getRKValue(b)
		val := vm.getRKValue(c)
		// Check if key already exists
		existing := t.Get(*key)
		if existing != nil && !existing.IsNil() {
			t.Set(*key, *val)
		} else {
			// Key doesn't exist - check for __newindex
			mm := vm.GetMetamethod(t, MetaNewIndex)
			if mm != nil {
				if mm.IsTable() {
					mt, _ := mm.ToTable()
					mt.Set(*key, *val)
				} else if mm.IsFunction() {
					vm.CallMetamethod(mm, []object.TValue{*upvalVal, *key, *val})
				} else {
					t.Set(*key, *val)
				}
			} else {
				t.Set(*key, *val)
			}
		}

	// Function call instruction
	case OP_CALL:
		a, b, c := instr.A(), instr.B(), instr.C()
		funcSlot := vm.Base + a
		funcVal := &vm.Stack[funcSlot]

		// Check if it's a function
		if !funcVal.IsFunction() {
			// Check if it's a table with __call metamethod
			if funcVal.IsTable() {
				t, _ := funcVal.ToTable()
				mm := vm.GetMetamethod(t, MetaCall)
				if mm != nil && mm.IsFunction() {
					// Call __call(table, ...args)
					closure, _ := mm.ToFunction()
					
					// Determine number of arguments
					var nargs int
					if b == 0 {
						nargs = vm.StackTop - funcSlot - 1
					} else {
						nargs = b - 1
					}

					// Shift args up by 1 to make room for table
					for i := nargs - 1; i >= 0; i-- {
						vm.Stack[funcSlot+2+i] = vm.Stack[funcSlot+1+i]
					}
					// Insert table as first arg
					vm.Stack[funcSlot+1] = *funcVal
					// Replace function with __call
					vm.Stack[funcSlot] = *mm
					nargs++ // Now we have one more arg (the table)

					if closure.IsGo {
						// Go function call
						oldBase := vm.Base
						vm.Base = funcSlot + 1
						vm.StackTop = funcSlot + 1 + nargs

						err := closure.GoFn(vm)
						vm.Base = oldBase
						if err != nil {
							return err
						}
						// Adjust stack top based on results
						nresults := c - 1
						if nresults >= 0 {
							vm.StackTop = funcSlot + nresults
						}
						return nil
					} else {
						// Lua function call - set up CallInfo and continue
						proto := closure.Proto
						if proto == nil {
							return vm.runtimeError("__call function has no prototype")
						}

						// Save current PC
						vm.CallInfo[vm.CI].PC = vm.PC

						// Push new CallInfo
						vm.CI++
						if vm.CI >= len(vm.CallInfo) {
							newCI := make([]*CallInfo, len(vm.CallInfo)*2)
							copy(newCI, vm.CallInfo)
							vm.CallInfo = newCI
						}

						newBase := funcSlot + 1
						neededTop := newBase + proto.MaxStackSize
						vm.ensureStack(neededTop)

						vm.CallInfo[vm.CI] = &CallInfo{
							Func:    mm,
							Closure: closure,
							Base:    newBase,
							Top:     neededTop,
							NResults: c - 1,
						}

						// Set up upvalues
						if closure.Upvalues != nil {
							for i := range closure.Upvalues {
								if i < len(proto.Upvalues) {
									// Upvalue already captured
								}
							}
						}

						vm.Base = newBase
						vm.StackTop = neededTop
						vm.Prototype = proto
						vm.PC = 0
						return nil // Return to Run() loop which will execute the __call function
					}
				}
			}
			return vm.runtimeError("attempt to call a non-function value")
		}
		fn, ok := funcVal.ToFunction()
		if !ok {
			return vm.runtimeError("attempt to call a non-function value")
		}

		// Determine number of arguments
		var nargs int
		if b == 0 {
			nargs = vm.StackTop - funcSlot - 1
		} else {
			nargs = b - 1
		}

		// Determine number of results
		nresults := c - 1 // c=0 means all results, c=1 means 0 results, c=2 means 1 result

		if fn.IsGo {
			// Go function call
			oldBase := vm.Base
			startTop := funcSlot + 1 + nargs // Save where results will start
			vm.Base = funcSlot + 1
			vm.StackTop = startTop

			err := fn.GoFn(vm)

			resultsTop := vm.StackTop
			vm.Base = oldBase

			if err != nil {
				return err
			}

			// Calculate actual results produced by the Go function
			numResults := resultsTop - startTop
			if numResults < 0 {
				numResults = 0
			}

			// Move results to funcSlot position
			if nresults >= 0 {
				// Fixed number of results
				for i := 0; i < nresults && i < numResults; i++ {
					vm.Stack[funcSlot+i].CopyFrom(&vm.Stack[startTop+i])
				}
				// Fill missing results with nil
				for i := numResults; i < nresults; i++ {
					vm.Stack[funcSlot+i].SetNil()
				}
				vm.StackTop = funcSlot + nresults
			} else {
				// All results (nresults == -1)
				for i := 0; i < numResults; i++ {
					vm.Stack[funcSlot+i].CopyFrom(&vm.Stack[startTop+i])
				}
				vm.StackTop = funcSlot + numResults
			}
		} else {
			// Lua function call
			proto := fn.Proto
			if proto == nil {
				return vm.runtimeError("function has no prototype")
			}

			// Save current PC in current CallInfo
			vm.CallInfo[vm.CI].PC = vm.PC

			// Push new CallInfo
			vm.CI++
			if vm.CI >= len(vm.CallInfo) {
				newCallInfo := make([]*CallInfo, len(vm.CallInfo)*2)
				copy(newCallInfo, vm.CallInfo)
				vm.CallInfo = newCallInfo
			}

			newBase := funcSlot + 1
			neededTop := newBase + proto.MaxStackSize
			vm.ensureStack(neededTop)

			vm.CallInfo[vm.CI] = &CallInfo{
				Func:     funcVal,
				Closure:  fn,
				Base:     newBase,
				Top:      neededTop,
				PC:       0,
				NResults: nresults,
				Status:   CallOK,
			}

			// Pad missing args with nil
			for i := nargs; i < proto.NumParams; i++ {
				vm.Stack[newBase+i].SetNil()
			}

			// Set up new execution context
			vm.Base = newBase
			vm.Prototype = proto
			vm.PC = 0
			vm.StackTop = neededTop

			// Return nil — the Run() loop will continue executing the new function's bytecode
		}

	// Numeric for loop instructions
	case OP_FORPREP:
		a, sbx := instr.A(), instr.SBx()
		// R(A) = initial, R(A+1) = limit, R(A+2) = step
		init := vm.Stack[vm.Base+a].Value.Num
		step := vm.Stack[vm.Base+a+2].Value.Num
		// Subtract step so first FORLOOP iteration adds it back
		vm.Stack[vm.Base+a].SetNumber(init - step)
		// Jump forward to FORLOOP
		vm.PC += sbx

	case OP_FORLOOP:
		a, sbx := instr.A(), instr.SBx()
		step := vm.Stack[vm.Base+a+2].Value.Num
		// Add step
		idx := vm.Stack[vm.Base+a].Value.Num + step
		vm.Stack[vm.Base+a].SetNumber(idx)
		limit := vm.Stack[vm.Base+a+1].Value.Num

		// Check loop condition
		continueLoop := false
		if step > 0 {
			continueLoop = idx <= limit
		} else {
			continueLoop = idx >= limit
		}

		if continueLoop {
			vm.Stack[vm.Base+a+3].SetNumber(idx) // Update external loop variable
			vm.PC += sbx                          // Jump back to loop body
		}

	// Generic for loop instructions
	case OP_FORGPREP:
		_, sbx := instr.A(), instr.SBx()
		vm.PC += sbx // Jump forward to FORGLOOP

	case OP_FORGLOOP:
		a, sbx := instr.A(), instr.SBx()
		// Call iterator: R(A)(R(A+1), R(A+2))
		iterFunc := &vm.Stack[vm.Base+a]
		if !iterFunc.IsFunction() {
			return vm.runtimeError("for iterator is not a function")
		}
		fn, _ := iterFunc.ToFunction()

		if fn.IsGo {
			oldBase := vm.Base
			oldTop := vm.StackTop
			vm.Base = vm.Base + a + 1
			vm.StackTop = vm.Base + 2 // 2 args (state, control)

			err := fn.GoFn(vm)

			resultsTop := vm.StackTop
			vm.Base = oldBase

			if err != nil {
				return err
			}

			// Results go to R(A+3), R(A+4), ...
			firstResult := oldBase + a + 3
			numResults := resultsTop - (oldBase + a + 3)
			if numResults < 0 {
				numResults = 0
			}

			// Check if first result is nil
			if numResults > 0 && !vm.Stack[firstResult].IsNil() {
				// Update control variable R(A+2) = first result
				vm.Stack[oldBase+a+2].CopyFrom(&vm.Stack[firstResult])
				vm.StackTop = oldTop
				vm.PC += sbx // Jump back to loop body
			} else {
				vm.StackTop = oldTop
				// Loop ends, continue to next instruction
			}
		} else {
			return fmt.Errorf("Lua iterator functions in generic for not yet supported")
		}

	// Table initialization
	case OP_SETLIST:
		a, b, c := instr.A(), instr.B(), instr.C()
		t, ok := vm.Stack[vm.Base+a].ToTable()
		if !ok {
			return fmt.Errorf("SETLIST target is not a table")
		}

		count := b
		if count == 0 {
			count = vm.StackTop - (vm.Base + a) - 1
		}

		offset := c // base offset for array indices
		for i := 1; i <= count; i++ {
			t.SetI(offset+i, vm.Stack[vm.Base+a+i])
		}

	// Closure creation
	case OP_CLOSURE:
		a, bx := instr.A(), instr.Bx()
		// Get child prototype
		childProto := vm.Prototype.Prototypes[bx]

		// Create new closure
		newClosure := &object.Closure{
			IsGo:     false,
			Proto:    childProto,
			Upvalues: make([]*object.Upvalue, len(childProto.Upvalues)),
		}

		// Set up upvalues from UpvalueDesc
		currentClosure := vm.getCurrentClosure()
		for i, uvDesc := range childProto.Upvalues {
			if uvDesc.IsLocal {
				// Capture from current stack
				stackIdx := vm.Base + uvDesc.Index
				// Check if there's already an open upvalue for this stack slot
				if existing, ok := vm.OpenUpvalues[stackIdx]; ok {
					// Share the same object.Upvalue so close propagates
					newClosure.Upvalues[i] = existing
				} else {
					uv := &object.Upvalue{
						Index: stackIdx,
						Value: &vm.Stack[stackIdx],
					}
					vm.OpenUpvalues[stackIdx] = uv
					newClosure.Upvalues[i] = uv
				}
			} else {
				// Copy from parent closure's upvalues
				if currentClosure != nil && uvDesc.Index < len(currentClosure.Upvalues) {
					parentUV := currentClosure.Upvalues[uvDesc.Index]
					newClosure.Upvalues[i] = parentUV // Share the same upvalue
				} else {
					newClosure.Upvalues[i] = &object.Upvalue{}
				}
			}
		}

		// Store closure in register
		vm.Stack[vm.Base+a].SetFunction(newClosure)

	// Vararg instruction
	case OP_VARARG:
		a, c := instr.A(), instr.C()
		numWanted := c - 1 // c=0 means all

		// For now, just set nil (varargs are complex and rarely needed in basic tests)
		if numWanted < 0 {
			numWanted = 0
		}
		for i := 0; i < numWanted; i++ {
			vm.Stack[vm.Base+a+i].SetNil()
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
	// Get function value and compute funcBase
	var funcVal *object.TValue
	var funcBase int
	if funcIdx >= 0 {
		funcVal = &vm.Stack[vm.Base+funcIdx]
		funcBase = vm.Base + funcIdx
	} else {
		// Negative indices: -1 = top of stack
		funcVal = &vm.Stack[vm.StackTop+funcIdx]
		funcBase = vm.StackTop + funcIdx
	}

	// Check if it's a Lua function
	if !funcVal.IsFunction() {
		return vm.runtimeError("attempt to call a non-function value")
	}

	fn, ok := funcVal.ToFunction()
	if !ok {
		return vm.runtimeError("attempt to call a non-function value")
	}

	if fn.IsGo {
		// Go function call
		// funcBase is already computed above
		
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
		// Results are on top of stack now, starting at funcBase+1+nargs
		numResults := vm.StackTop - (funcBase + 1)
		if numResults < 0 {
			numResults = 0
		}
		
		if nresults >= 0 {
			// Move results to function position
			resultStart := vm.StackTop - nresults
			for i := 0; i < nresults; i++ {
				vm.Stack[funcBase+i] = vm.Stack[resultStart+i]
			}
			vm.StackTop = funcBase + nresults
		} else {
			// All results: move them to function position
			for i := 0; i < numResults; i++ {
				vm.Stack[funcBase+i] = vm.Stack[funcBase+1+i]
			}
			vm.StackTop = funcBase + numResults
		}
		
		return nil
	}

	// Lua function call
	proto := fn.Proto
	if proto == nil {
		return vm.runtimeError("function has no prototype")
	}

	// Initialize CallInfo[0] if this is the first call
	if vm.CI == 0 {
		vm.CallInfo[0] = &CallInfo{
			Func:     funcVal,
			Closure:  fn,
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

	newBase := funcBase + 1
	vm.CallInfo[vm.CI] = &CallInfo{
		Func:     funcVal,
		Closure:  fn,
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
// ProtectedCall calls a Lua function with protected execution.
// If an error occurs during execution, it is returned.
// The function and its arguments must already be on the stack.
func (vm *VM) ProtectedCall(funcIdx, nargs, nresults int) error {
	// Save VM state
	savedCI := vm.CI
	savedBase := vm.Base
	savedPC := vm.PC
	savedPrototype := vm.Prototype

	// Check if it's a Go function BEFORE calling (since Call changes Base for Lua functions)
	fn, _ := vm.Stack[vm.Base+funcIdx].ToFunction()
	isGo := fn != nil && fn.IsGo

	// Call the function
	vm.Call(funcIdx, nargs, nresults)

	// If it's a Go function, Call() already executed it
	if isGo {
		return nil
	}

	// Run nested execution loop until CI drops back
	for vm.CI > savedCI {
		if vm.PC >= len(vm.Prototype.Code) {
			break
		}

		instr := Instruction(vm.Prototype.Code[vm.PC])
		vm.PC++

		if err := vm.ExecuteInstruction(instr); err != nil {
			// Restore state before returning error
			vm.CI = savedCI
			vm.Base = savedBase
			vm.PC = savedPC
			vm.Prototype = savedPrototype
			return err
		}
	}

	// Restore caller's Base, PC and Prototype after nested run
	vm.Base = savedBase
	vm.PC = savedPC
	vm.Prototype = savedPrototype

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
	vm.ensureStack(vm.StackTop + 1)
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
	newTop := vm.Base + index
	vm.ensureStack(newTop)
	if index > vm.StackTop-vm.Base {
		// Extending stack, fill new slots with nil
		for i := vm.StackTop; i < newTop; i++ {
			vm.Stack[i].SetNil()
		}
	}
	vm.StackTop = newTop
}

// ensureStack grows the stack if needed to accommodate the given top index.
func (vm *VM) ensureStack(neededTop int) {
	for neededTop >= len(vm.Stack) {
		newStack := make([]object.TValue, len(vm.Stack)*2)
		copy(newStack, vm.Stack)
		vm.Stack = newStack
	}
}

// Metamethod name constants
const (
	MetaIndex    = "__index"
	MetaNewIndex = "__newindex"
	MetaCall     = "__call"
	MetaAdd      = "__add"
	MetaSub      = "__sub"
	MetaMul      = "__mul"
	MetaDiv      = "__div"
	MetaMod      = "__mod"
	MetaPow      = "__pow"
	MetaUnm      = "__unm"
	MetaEq       = "__eq"
	MetaLt       = "__lt"
	MetaLe       = "__le"
	MetaToString = "__tostring"
	MetaConcat   = "__concat"
	MetaLen      = "__len"
	MetaPairs    = "__pairs"
	MetaIPairs   = "__ipairs"
)

// getMetamethod retrieves a metamethod from a table's metatable.
// Returns nil if the table has no metatable or the metamethod is not found.
func (vm *VM) GetMetamethod(t *object.Table, methodName string) *object.TValue {
	if t == nil {
		return nil
	}
	mt := t.GetMetatable()
	if mt == nil {
		return nil
	}
	// Look up the metamethod in the metatable
	key := object.TValue{Type: object.TypeString}
	key.Value.Str = methodName
	val := mt.Get(key)
	if val == nil || val.IsNil() {
		// Try direct map access (workaround for map key comparison issues)
		if mt.Map != nil {
			for k, v := range mt.Map {
				if k.Str == methodName {
					return v
				}
			}
		}
		return nil
	}
	return val
}

// callMetamethod calls a Go function metamethod with the given arguments.
// Returns the result of the metamethod call, or nil if the call fails.
// Note: Currently only supports Go functions for simplicity.
func (vm *VM) CallMetamethod(fn *object.TValue, args []object.TValue) *object.TValue {
	if fn == nil || !fn.IsFunction() {
		return nil
	}

	closure, ok := fn.ToFunction()
	if !ok {
		return nil
	}

	// Save current state
	oldBase := vm.Base
	oldTop := vm.StackTop

	// Set up stack for metamethod call ABOVE the caller's full register window.
	// Using vm.StackTop alone is wrong — it may not account for temporary registers
	// the caller is still building (e.g., arguments to an outer print() call).
	// vm.Base + Prototype.MaxStackSize is the safe high-water mark for the caller's frame.
	callBase := vm.StackTop
	if vm.Prototype != nil {
		callerMax := vm.Base + vm.Prototype.MaxStackSize
		if callerMax > callBase {
			callBase = callerMax
		}
	}
	vm.ensureStack(callBase + len(args) + 50)

	// Stack layout: [function] [arg1] [arg2] ...
	vm.Stack[callBase].CopyFrom(fn)
	for i := range args {
		vm.Stack[callBase+1+i].CopyFrom(&args[i])
	}

	vm.StackTop = callBase + 1 + len(args)

	var result *object.TValue

	if closure.IsGo {
		// Go function call
		vm.Base = callBase + 1
		err := closure.GoFn(vm)
		if err == nil && vm.StackTop > callBase {
			result = &vm.Stack[callBase]
		}
	} else {
		// Lua function call - execute inline
		proto := closure.Proto
		if proto != nil && len(proto.Code) > 0 {
			// Save more state for Lua function
			oldPC := vm.PC
			oldProto := vm.Prototype
			oldCI := vm.CI

			// Set up CallInfo - need to save current frame first
			if vm.CI >= 0 && vm.CallInfo[vm.CI] != nil {
				vm.CallInfo[vm.CI].PC = vm.PC
			}

			// Push new CallInfo
			vm.CI++
			if vm.CI >= len(vm.CallInfo) {
				newCI := make([]*CallInfo, len(vm.CallInfo)*2)
				copy(newCI, vm.CallInfo)
				vm.CallInfo = newCI
			}

			newBase := callBase + 1
			neededTop := newBase + proto.MaxStackSize
			vm.ensureStack(neededTop)

			vm.CallInfo[vm.CI] = &CallInfo{
				Func:     fn,
				Closure:  closure,
				Base:     newBase,
				Top:      neededTop,
				PC:       0,
				NResults: 1,
			}

			// Copy arguments to their proper locations (they're already at newBase+0, newBase+1, etc.)
			// No need to move them - they're in the right place.
			// Just pad missing args with nil.
			for i := len(args); i < proto.NumParams; i++ {
				vm.Stack[newBase+i].SetNil()
			}

			// Set up execution context
			vm.Base = newBase
			vm.Prototype = proto
			vm.PC = 0
			// IMPORTANT: Don't reset StackTop here - it should already be correct
			// The arguments are at newBase+0, newBase+1, etc. and StackTop should be
			// at least newBase + max(len(args), proto.NumParams)
			if vm.StackTop < newBase+proto.MaxStackSize {
				vm.StackTop = newBase + proto.MaxStackSize
			}

			// Execute until we return from this call frame
			startCI := vm.CI
			for vm.PC < len(vm.Prototype.Code) {
				instr := Instruction(vm.Prototype.Code[vm.PC])
				vm.PC++
				if err := vm.ExecuteInstruction(instr); err != nil {
					break
				}
				// Check if we've returned from the metamethod
				if vm.CI < startCI {
					break
				}
			}

			// Get result from where RETURN placed it
			if vm.StackTop > callBase {
				result = &vm.Stack[callBase]
			}

			// Restore state
			vm.PC = oldPC
			vm.Prototype = oldProto
			vm.CI = oldCI
		}
	}

	// Restore state
	vm.Base = oldBase
	vm.StackTop = oldTop

	return result
}

// tryBinaryMetamethod tries to apply a binary metamethod for two operands.
// Returns the result and true if a metamethod was found and called.
func (vm *VM) tryBinaryMetamethod(rb, rc *object.TValue, methodName string) (*object.TValue, bool) {
	// Check if either operand is a table with the metamethod
	if rb.IsTable() {
		t, _ := rb.ToTable()
		mm := vm.GetMetamethod(t, methodName)
		if mm != nil && mm.IsFunction() {
			result := vm.CallMetamethod(mm, []object.TValue{*rb, *rc})
			return result, true
		}
	}
	if rc.IsTable() {
		t, _ := rc.ToTable()
		mm := vm.GetMetamethod(t, methodName)
		if mm != nil && mm.IsFunction() {
			result := vm.CallMetamethod(mm, []object.TValue{*rb, *rc})
			return result, true
		}
	}
	return nil, false
}

// tryUnaryMetamethod tries to apply a unary metamethod for one operand.
// Returns the result and true if a metamethod was found and called.
func (vm *VM) tryUnaryMetamethod(rb *object.TValue, methodName string) (*object.TValue, bool) {
	if rb.IsTable() {
		t, _ := rb.ToTable()
		mm := vm.GetMetamethod(t, methodName)
		if mm != nil && mm.IsFunction() {
			result := vm.CallMetamethod(mm, []object.TValue{*rb})
			return result, true
		}
	}
	return nil, false
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

