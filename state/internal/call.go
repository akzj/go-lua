// Package internal provides the concrete implementation of state/api interfaces.
package internal

import (
	bcapi "github.com/akzj/go-lua/bytecode/api"
	"github.com/akzj/go-lua/types"
	typesapi "github.com/akzj/go-lua/types/api"
	vm "github.com/akzj/go-lua/vm"
)

// Call implements LuaStateInterface.Call()
// 
// Invariants:
// - nArgs >= 0
// - Stack has nArgs+1 elements (function + args) ending at top
// - Function must be callable (closure, C function, or table with __call)
// 
// Postconditions:
// - Function and arguments are removed from stack
// - nResults values are pushed onto stack (0 if nResults == 0)
func (L *LuaState) Call(nArgs, nResults int) {
	// Get the function position
	// Stack layout: [..., func, arg1, arg2, ..., argN] <- top
	// Function is at: L.top - nArgs - 1
	funcIdx := L.top - nArgs - 1
	
	if funcIdx < 0 {
		// Invalid - not enough arguments
		panic("Call: stack underflow")
	}
	
	// Get the function
	fn := L.stack[funcIdx]
	
	// Handle based on function type
	switch {
	case fn.IsCClosure() || fn.IsLightCFunction():
		// C function - call directly (simplified for now)
		L.callCFunction(fn, nArgs, nResults)
	case fn.IsLClosure():
		// Lua closure - execute via VM
		L.callLuaClosure(fn, nArgs, nResults)
	case fn.IsLightUserData():
		// Check if this is a DoString marker
		if proto := lookupDoStringPrototype(fn.GetValue()); proto != nil {
			L.executeProto(proto, nArgs, nResults)
		} else {
			panic("Call: attempted to call light userdata")
		}
	default:
		// TODO: Handle __call metamethod for tables
		panic("Call: attempted to call a non-function value")
	}
}

// callLuaClosure calls a Lua function by executing its bytecode via VM
func (L *LuaState) callLuaClosure(fn typesapi.TValue, nArgs, nResults int) {
	// Get closure and prototype
	closure := fn.GetValue()
	proto := extractProto(closure)
	
	if proto == nil {
		panic("Call: cannot extract prototype from closure")
	}
	
	// Create new CallInfo for this frame
	newCI := &callInfo{
		func_:   L.top - nArgs - 1, // Position of function
		top:     L.top,              // Current top becomes frame top
		prev:    L.ci,
		nresults: nResults,
	}
	
	// Push new frame onto LuaState
	L.ci = newCI
	
	// Get or create VM executor
	executor := L.getOrCreateExecutor()
	
	// Calculate frame base in the shared stack
	frameBase := newCI.func_
	
	// Create frame data for VM
	frame := &luaFrame{
		closure:    fn,
		base:       frameBase,
		prev:       executor.CurrentFrame(),
		savedPC:    0,
		kvalues:    extractKValues(proto),
		upvals:     nil,
	}
	executor.PushFrame(frame)
	
	// Execute bytecode
	_ = executor.Run()
	
	// Pop frame from VM
	executor.PopFrame()
	
	// Pop CallInfo from LuaState
	L.ci = L.ci.prev
	
	// Adjust stack for results
	newTop := newCI.func_ + nResults
	if nResults >= 0 && nResults != -1 { // -1 means LUA_MULTRET
		L.top = newTop
	} else {
		// Variable results - keep all returned values
		L.top = L.ci.Top()
	}
}

// executeProto executes a bytecode prototype directly (used by DoString)
func (L *LuaState) executeProto(proto bcapi.Prototype, nArgs, nResults int) {
	// Create new CallInfo for this frame
	newCI := &callInfo{
		func_:   L.top - nArgs - 1,
		top:     L.top,
		prev:    L.ci,
		nresults: nResults,
	}

	// Push new frame onto LuaState
	L.ci = newCI

	// Get or create VM executor
	executor := L.getOrCreateExecutor()

	// Set the bytecode code to execute
	code := make([]vm.Instruction, len(proto.GetCode()))
	for i, inst := range proto.GetCode() {
		code[i] = vm.Instruction(inst)
	}
	executor.SetCode(code)

	// Calculate frame base in the shared stack
	frameBase := newCI.func_

	// Create frame data for VM
	var nilTValue typesapi.TValue
	frame := &luaFrame{
		closure:    nilTValue,
		base:       frameBase,
		prev:       executor.CurrentFrame(),
		savedPC:    0,
		kvalues:    extractKValues(proto),
		upvals:     nil,
	}
	executor.PushFrame(frame)

	// Execute bytecode
	_ = executor.Run()

	// Pop frame from VM
	executor.PopFrame()

	// Pop CallInfo from LuaState
	L.ci = L.ci.prev

	// Adjust stack for results
	newTop := newCI.func_ + nResults
	if nResults >= 0 && nResults != -1 {
		L.top = newTop
	} else {
		L.top = L.ci.Top()
	}
}

// luaFrame implements vm.StackFrame for integration with VM
type luaFrame struct {
	closure  typesapi.TValue
	base     int
	prev     vm.StackFrame
	savedPC  int
	kvalues  []typesapi.TValue
	upvals   interface{}
}

func (f *luaFrame) Base() int                     { return f.base }
func (f *luaFrame) Func() typesapi.TValue           { return f.closure }
func (f *luaFrame) Prev() vm.StackFrame       { return f.prev }
func (f *luaFrame) PC() int                      { return f.savedPC }
func (f *luaFrame) SetPC(pc int)                 { f.savedPC = pc }
func (f *luaFrame) Top() int                     { return f.base }
func (f *luaFrame) KValues() []typesapi.TValue     { return f.kvalues }

// callCFunction calls a C function directly
func (L *LuaState) callCFunction(fn typesapi.TValue, nArgs, nResults int) {
	// TODO: Implement C function calling
	// For now, just pop the arguments
	L.top = L.top - nArgs - 1
}

// getOrCreateExecutor returns the VM executor, creating it if necessary
func (L *LuaState) getOrCreateExecutor() vm.VMFrameManager {
	if L.executor == nil {
		L.executor = vm.NewVMFrameManager()
		// Share the stack with executor
		L.executor.SetStack(L.stack)
	}
	return L.executor
}

// extractProto extracts the Prototype from a closure value.
// Returns nil if the value is not a valid closure/prototype.
func extractProto(closure interface{}) bcapi.Prototype {
	// First try to look up in DoString registry
	if proto := lookupDoStringPrototype(closure); proto != nil {
		return proto
	}
	
	// If closure is a bcapi.Prototype directly, return it
	if proto, ok := closure.(bcapi.Prototype); ok {
		return proto
	}
	
	return nil
}

// extractKValues extracts the constant pool from a prototype as []typesapi.TValue
func extractKValues(proto bcapi.Prototype) []typesapi.TValue {
	if proto == nil {
		return nil
	}
	
	constants := proto.GetConstants()
	kvals := make([]typesapi.TValue, len(constants))
	for i, c := range constants {
		kvals[i] = constantToTValue(c)
	}
	return kvals
}

// constantToTValue converts a bcapi.Constant to a typesapi.TValue
func constantToTValue(c *bcapi.Constant) typesapi.TValue {
	switch c.Type {
	case bcapi.ConstNil:
		return types.NewTValueNil()
	case bcapi.ConstInteger:
		return types.NewTValueInteger(typesapi.LuaInteger(c.Int))
	case bcapi.ConstFloat:
		return types.NewTValueFloat(typesapi.LuaNumber(c.Float))
	case bcapi.ConstString:
		return types.NewTValueString(c.Str)
	case bcapi.ConstBool:
		return types.NewTValueBoolean(c.Int != 0)
	}
	return types.NewTValueNil()
}

// wrapStringAsTValue wraps a TString in a TValue
func wrapStringAsTValue(s typesapi.TString) typesapi.TValue {
	// Create a TValue with the string
	tv := types.NewTValueNil()
	// Use NewValueGC to wrap the string as a GC object
	// This is a simplified approach - actual implementation needs proper string handling
	return tv
}
