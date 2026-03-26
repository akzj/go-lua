// Package api provides the public Lua API
package api

import (
	"github.com/akzj/go-lua/pkg/vm"
)

// Call calls a function.
//
// This corresponds to lua_call in the C API.
// It calls a function in protected mode, using panic/recover for error handling.
//
// Before calling, the function and its arguments should be on the stack:
//   - Stack: [func, arg1, arg2, ..., argN]
//
// After calling, the results are on the stack:
//   - Stack: [result1, result2, ..., resultM]
//
// Parameters:
//   - nargs: Number of arguments
//   - nresults: Expected number of results (-1 for all results)
//
// Example:
//
//	// Prepare function and arguments
//	L.GetGlobal("myfunc")
//	L.PushNumber(10)
//	L.PushNumber(20)
//
//	// Call with 2 args, 1 result
//	if err := L.Call(2, 1); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Get result
//	result, _ := L.ToNumber(-1)
func (s *State) Call(nargs, nresults int) error {
	// Use protected call internally
	return s.PCall(nargs, nresults, 0)
}

// PCall calls a function in protected mode.
//
// This corresponds to lua_pcall in the C API.
// It calls a function and catches any errors, returning them as error values.
//
// Before calling, the function and its arguments should be on the stack:
//   - Stack: [func, arg1, arg2, ..., argN]
//
// After calling:
//   - On success: Stack: [result1, result2, ..., resultM]
//   - On error: Stack: [error_object]
//
// Parameters:
//   - nargs: Number of arguments
//   - nresults: Expected number of results (-1 for all results)
//   - msgh: Index of message handler (0 for no handler)
//
// Returns:
//   - error: nil on success, *LuaError on error
//
// Example:
//
//	// Prepare function and arguments
//	L.GetGlobal("myfunc")
//	L.PushNumber(10)
//
//	// Call with error handling
//	if err := L.PCall(1, 1, 0); err != nil {
//	    log.Printf("Error: %v", err)
//	} else {
//	    result, _ := L.ToNumber(-1)
//	    log.Printf("Result: %v", result)
//	}
func (s *State) PCall(nargs, nresults int, msgh int) (err error) {
	// Set up panic recovery
	defer func() {
		if r := recover(); r != nil {
			if luaErr, ok := r.(*LuaError); ok {
				err = luaErr
			} else {
				if str, ok := r.(string); ok {
					err = newLuaError(str)
				} else {
					err = newLuaError("unknown error")
				}
			}

			// On error, push error object onto stack
			s.vm.SetTop(s.vm.Base) // Clear args
			s.PushString(err.Error())
		}
	}()

	// Save VM state for proper restoration after nested execution
	savedCI := s.vm.CI
	savedBase := s.vm.Base
	savedPC := s.vm.PC
	savedPrototype := s.vm.Prototype

	// Get function value to check if it's a Go function
	// Function is at negative index -(nargs+1) from top
	// Stack layout: [func, arg1, arg2, ..., argN] where -1=argN, -(nargs+1)=func
	funcIdx := -(nargs + 1)
	funcVal := s.vm.GetStack(funcIdx)
	isGoFunc := funcVal.IsFunction()
	if isGoFunc {
		if fn, ok := funcVal.ToFunction(); ok {
			isGoFunc = fn.IsGo
		}
	}

	// Call the VM - function is at negative index from StackTop
	if err := s.vm.Call(funcIdx, nargs, nresults); err != nil {
		return wrapError(err)
	}

	// For Lua functions, run the bytecode with a bounded loop
	// This prevents the Run loop from executing outer script instructions
	// For Go functions, the call already executed the function
	if isGoFunc {
		// Go function already executed, just adjust results
	} else {
		// Run nested execution loop until CI drops back
		for s.vm.CI > savedCI {
			if s.vm.PC >= len(s.vm.Prototype.Code) {
				break
			}

			instr := vm.Instruction(s.vm.Prototype.Code[s.vm.PC])
			s.vm.PC++

			if execErr := s.vm.ExecuteInstruction(instr); execErr != nil {
				// Restore state before returning error
				s.vm.CI = savedCI
				s.vm.Base = savedBase
				s.vm.PC = savedPC
				s.vm.Prototype = savedPrototype
				return wrapError(execErr)
			}
		}

		// Restore caller's Base, PC and Prototype after nested run
		s.vm.Base = savedBase
		s.vm.PC = savedPC
		s.vm.Prototype = savedPrototype
	}

	// Adjust results
	if nresults >= 0 {
		// Adjust to expected number of results
		currentResults := s.GetTop()
		if currentResults > nresults {
			s.SetTop(nresults)
		} else if currentResults < nresults {
			// Push nil for missing results
			for i := currentResults; i < nresults; i++ {
				s.PushNil()
			}
		}
	}

	return nil
}

// CallK calls a function with continuation.
//
// This corresponds to lua_callk in the C API.
// It's used for C functions that need to yield and resume.
//
// Not fully implemented in this version.
//
// Parameters:
//   - nargs: Number of arguments
//   - nresults: Expected number of results
//   - ctx: Context for continuation
//   - k: Continuation function
//
// Returns:
//   - error: Any error that occurred
func (s *State) CallK(nargs, nresults int, ctx int, k Function) error {
	// For now, just use regular Call
	// In a full implementation, this would support yielding
	return s.Call(nargs, nresults)
}

// PCallK calls a function in protected mode with continuation.
//
// This corresponds to lua_pcallk in the C API.
//
// Not fully implemented in this version.
//
// Parameters:
//   - nargs: Number of arguments
//   - nresults: Expected number of results
//   - msgh: Index of message handler
//   - ctx: Context for continuation
//   - k: Continuation function
//
// Returns:
//   - error: nil on success, *LuaError on error
func (s *State) PCallK(nargs, nresults int, msgh, ctx int, k Function) error {
	// For now, just use regular PCall
	return s.PCall(nargs, nresults, msgh)
}

// Resume resumes a coroutine.
//
// This corresponds to lua_resume in the C API.
// It resumes a yielded coroutine or starts a new one.
//
// Parameters:
//   - from: The state that is resuming (can be nil)
//   - nargs: Number of arguments to pass to the coroutine
//
// Returns:
//   - int: Status code (0 = success, >0 = yielded, <0 = error)
//   - error: Any error that occurred
func (s *State) Resume(from *State, nargs int) (int, error) {
	// Check if this is a coroutine
	// For now, just run normally if there's a prototype
	if s.vm.Prototype == nil {
		return 0, nil // Nothing to run
	}
	if err := s.vm.Run(); err != nil {
		return -1, wrapError(err)
	}
	return 0, nil
}

// Yield yields a coroutine.
//
// This corresponds to lua_yield in the C API.
// It yields the current coroutine, returning values to the resumer.
//
// Parameters:
//   - nresults: Number of results to return
//
// Returns:
//   - int: Number of results
func (s *State) Yield(nresults int) int {
	// In a full implementation, this would:
	// 1. Save the current execution state
	// 2. Return control to the resumer
	// 3. Return the number of results

	// For now, just return nresults
	return nresults
}

// YieldK yields a coroutine with continuation.
//
// This corresponds to lua_yieldk in the C API.
//
// Parameters:
//   - nresults: Number of results to return
//   - ctx: Context for continuation
//   - k: Continuation function
//
// Returns:
//   - int: Number of results
func (s *State) YieldK(nresults, ctx int, k Function) int {
	// For now, just use regular Yield
	return s.Yield(nresults)
}

// Status returns the status of a thread/coroutine.
//
// This corresponds to lua_status in the C API.
//
// Returns:
//   - int: Status code
//     - 0: OK
//     - 1: Yielded
//     - >1: Error code
func (s *State) Status() int {
	// For now, always return OK
	// In a full implementation, this would check the thread status
	return 0
}

// ResetThread resets a thread for reuse.
//
// This corresponds to lua_resetthread in the C API.
//
// Returns:
//   - int: Status code
func (s *State) ResetThread() int {
	// Reset the VM state
	s.vm.SetTop(0)
	s.vm.PC = 0
	s.vm.Base = 0
	s.vm.CI = 0
	return 0
}

// IsYieldable checks if the current function can yield.
//
// This corresponds to lua_isyieldable in the C API.
//
// Returns:
//   - bool: True if yielding is allowed
func (s *State) IsYieldable() bool {
	// For now, always return true
	// In a full implementation, this would check if we're in a C function
	// that was called with CallK/PCallK
	return true
}

// XMove moves values between states.
//
// This corresponds to lua_xmove in the C API.
// It transfers values from one state to another.
//
// Parameters:
//   - to: Target state
//   - n: Number of values to move
func (s *State) XMove(to *State, n int) {
	// Get values from source stack
	for i := 0; i < n; i++ {
		idx := s.GetTop() - n + i + 1
		v := s.vm.GetStack(idx)
		to.vm.Push(*v)
	}

	// Remove from source stack
	s.SetTop(s.GetTop() - n)
}