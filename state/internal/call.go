// Package internal provides the concrete implementation of state/api interfaces.
package internal

import (
	types "github.com/akzj/go-lua/types/api"
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
	default:
		// TODO: Handle __call metamethod for tables
		panic("Call: attempted to call a non-function value")
	}
}

// callLuaClosure calls a Lua function by executing its bytecode via VM
func (L *LuaState) callLuaClosure(fn types.TValue, nArgs, nResults int) {
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

// luaFrame implements vm.StackFrame for integration with VM
type luaFrame struct {
	closure  types.TValue
	base     int
	prev     vm.StackFrame
	savedPC  int
	kvalues  []types.TValue
	upvals   interface{}
}

func (f *luaFrame) Base() int                     { return f.base }
func (f *luaFrame) Func() types.TValue           { return f.closure }
func (f *luaFrame) Prev() vm.StackFrame       { return f.prev }
func (f *luaFrame) PC() int                      { return f.savedPC }
func (f *luaFrame) SetPC(pc int)                 { f.savedPC = pc }
func (f *luaFrame) Top() int                     { return f.base }

// callCFunction calls a C function directly
func (L *LuaState) callCFunction(fn types.TValue, nArgs, nResults int) {
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

// extractProto extracts the Proto from a closure value
// Returns nil if the value is not a Lua closure
func extractProto(closure interface{}) interface{} {
	// TODO: Implement actual proto extraction from closure
	return nil
}

// extractKValues extracts the constant pool from a prototype
func extractKValues(proto interface{}) []types.TValue {
	// TODO: Implement actual K-values extraction
	return nil
}
