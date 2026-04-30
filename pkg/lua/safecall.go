package lua

import (
	"fmt"
	"runtime/debug"
)

// SafeCall calls the function at the top of the stack in protected mode.
// Unlike PCall, it automatically:
//  1. Uses debug.traceback as the message handler (full Lua stack trace on error)
//  2. Recovers from Go panics in the called function (converts to Lua error)
//
// On success, returns nil and leaves nResults on the stack.
// On error, returns an error with the full Lua traceback and pops the error message.
//
// Example:
//
//	L.GetGlobal("myfunction")
//	L.PushString("arg1")
//	err := L.SafeCall(1, 0)
//	if err != nil {
//	    log.Printf("Lua error:\n%s", err)
//	}
func (L *State) SafeCall(nArgs, nResults int) error {
	// Push debug.traceback as message handler BELOW the function+args
	L.GetGlobal("debug")
	L.GetField(-1, "traceback")
	L.Remove(-2) // remove debug table, keep traceback function

	// Insert traceback below function+args: [traceback, func, arg1, arg2, ...]
	msgh := L.GetTop() - nArgs - 1
	L.Insert(msgh)

	status := L.PCall(nArgs, nResults, msgh)
	L.Remove(msgh) // remove traceback function

	if status != OK {
		msg, _ := L.ToString(-1)
		L.Pop(1)
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// SafeCallFunc is a convenience that pushes a Go function, calls it with SafeCall,
// and returns any error. Useful for calling Go functions that might panic.
//
// Example:
//
//	err := L.SafeCallFunc(func(L *lua.State) int {
//	    // This might panic
//	    result := riskyOperation()
//	    L.PushAny(result)
//	    return 1
//	}, 0)
func (L *State) SafeCallFunc(fn Function, nResults int) error {
	L.PushFunction(fn)
	return L.SafeCall(0, nResults)
}

// WrapSafe wraps a Go function so that Go panics are converted to Lua errors.
// Use this when registering Go functions that might panic.
//
// Example:
//
//	L.PushFunction(lua.WrapSafe(func(L *lua.State) int {
//	    // If this panics, Lua gets an error instead of a crash
//	    data := mustParse(L.CheckString(1))
//	    L.PushAny(data)
//	    return 1
//	}))
//	L.SetGlobal("parse")
func WrapSafe(fn Function) Function {
	return func(L *State) int {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				L.Errorf("Go panic: %v\n%s", r, stack)
			}
		}()
		return fn(L)
	}
}
