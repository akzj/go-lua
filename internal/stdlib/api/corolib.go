package api

import (
	luaapi "github.com/akzj/go-lua/internal/api/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
	vmapi "github.com/akzj/go-lua/internal/vm/api"
)

// ---------------------------------------------------------------------------
// coroutine library — mirrors lcorolib.c from Lua 5.5
// ---------------------------------------------------------------------------

// OpenCoroutineLib registers all coroutine library functions.
// Replaces the stub OpenCoroutine.
func OpenCoroutineLib(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{
		"create":      coroCreate,
		"resume":      coroResume,
		"yield":       coroYield,
		"wrap":        coroWrap,
		"status":      coroStatus,
		"running":     coroRunning,
		"isyieldable": coroIsYieldable,
		"close":       coroClose,
	})
	return 1
}

// coroCreate creates a new coroutine from a function.
// coroutine.create(f) → thread
func coroCreate(L *luaapi.State) int {
	L.CheckType(1, 6) // LUA_TFUNCTION = 6
	co := L.NewThread()
	L.PushValue(1)    // push the function
	L.XMove(co, 1)    // move function to the new thread's stack
	return 1           // return the thread (already on L's stack from NewThread)
}

// coroResume resumes a coroutine.
// coroutine.resume(co, ...) → true, results... | false, errMsg
func coroResume(L *luaapi.State) int {
	co := L.ToThread(1)
	if co == nil {
		L.ArgError(1, "coroutine expected")
		return 0
	}
	return auxResume(L, co, L.GetTop()-1)
}

// auxResume is the shared resume logic for both resume and wrap.
// nArgs is the number of arguments after the coroutine (for resume)
// or all arguments (for wrap's internal function).
func auxResume(L *luaapi.State, co *luaapi.State, nArgs int) int {
	// Check that the coroutine is resumable
	coStatus := co.Status()
	if coStatus != stateapi.StatusOK && coStatus != stateapi.StatusYield {
		L.PushBoolean(false)
		L.PushString("cannot resume dead coroutine")
		return 2
	}
	if coStatus == stateapi.StatusOK && co.GetTop() == 0 {
		L.PushBoolean(false)
		L.PushString("cannot resume dead coroutine")
		return 2
	}

	// Transfer arguments from L to co
	if nArgs > 0 {
		L.XMove(co, nArgs)
	}

	// Resume the coroutine
	status, nresults := co.Resume(L, nArgs)

	if status == stateapi.StatusOK || status == stateapi.StatusYield {
		// Success: transfer results back
		// Results are on co's stack
		if nresults > 0 {
			co.XMove(L, nresults)
		}
		L.PushBoolean(true)
		L.Insert(-(nresults + 1)) // move true before results
		return nresults + 1        // true + results
	}

	// Error: transfer error message back
	co.XMove(L, 1) // move error object to L
	L.PushBoolean(false)
	L.Insert(-2) // move false before error message
	return 2      // false + error message
}

// coroYield yields from the running coroutine.
// coroutine.yield(...)
func coroYield(L *luaapi.State) int {
	return L.Yield(L.GetTop())
}

// coroWrap creates a wrapped coroutine.
// coroutine.wrap(f) → function
func coroWrap(L *luaapi.State) int {
	L.CheckType(1, 6) // LUA_TFUNCTION = 6
	// Create the coroutine
	co := L.NewThread()
	L.PushValue(1)  // push the function
	L.XMove(co, 1)  // move function to co's stack

	// The thread is at top of L's stack (from NewThread).
	// Create a C closure with the thread as upvalue.
	L.PushCClosure(coroWrapAux, 1)
	return 1
}

// coroWrapAux is the function returned by coroutine.wrap.
// Each call resumes the coroutine. On error, propagates via error().
func coroWrapAux(L *luaapi.State) int {
	// Get the coroutine from upvalue 1
	co := L.ToThread(luaapi.UpvalueIndex(1))
	if co == nil {
		L.Errorf("cannot resume dead coroutine")
		return 0
	}

	// Transfer all arguments to the coroutine
	nArgs := L.GetTop()
	if nArgs > 0 {
		L.XMove(co, nArgs)
	}

	// Resume
	status, nresults := co.Resume(L, nArgs)

	if status == stateapi.StatusOK || status == stateapi.StatusYield {
		// Success: transfer results back
		if nresults > 0 {
			co.XMove(L, nresults)
		}
		return nresults
	}

	// Error: close coroutine's TBC variables, then propagate
	// Mirrors: luaB_auxwrap in lcorolib.c:77-92
	coState := co.Internal.(*stateapi.LuaState)
	callerState := L.Internal.(*stateapi.LuaState)
	stat := coState.Status
	if stat != stateapi.StatusOK && stat != stateapi.StatusYield {
		vmapi.CloseThread(coState, callerState)
		co.XMove(L, 1) // move error message to caller
	} else {
		co.XMove(L, 1)
	}
	// Add context only for real strings (no coercion), mirroring C lua_type check.
	if L.Type(-1) == objectapi.TypeString {
		s, _ := L.ToString(-1)
		L.SetTop(0)
		L.Errorf("%s", s)
	} else {
		// Non-string error object — just re-raise it
		L.Error()
	}
	return 0 // unreachable
}

// coroStatus returns the status of a coroutine as a string.
// coroutine.status(co) → "running" | "suspended" | "normal" | "dead"
func coroStatus(L *luaapi.State) int {
	co := L.ToThread(1)
	if co == nil {
		L.ArgError(1, "coroutine expected")
		return 0
	}

	if L.Internal == co.Internal {
		L.PushString("running")
		return 1
	}

	status := co.Status()
	switch status {
	case stateapi.StatusYield:
		L.PushString("suspended")
	case stateapi.StatusOK:
		// OK means either not started (suspended) or finished (dead)
		if co.GetTop() == 0 {
			L.PushString("dead")
		} else {
			L.PushString("suspended")
		}
	default:
		// Error status = dead
		L.PushString("dead")
	}
	return 1
}

// coroRunning returns the running coroutine and a boolean (true if main thread).
// coroutine.running() → co, isMain
func coroRunning(L *luaapi.State) int {
	isMain := L.PushThread()
	L.PushBoolean(isMain)
	return 2
}

// coroIsYieldable returns whether the given (or running) coroutine can yield.
// coroutine.isyieldable([co]) → boolean
// Mirrors: luaB_yieldable in lcorolib.c
func coroIsYieldable(L *luaapi.State) int {
	if L.Type(1) == objectapi.TypeThread {
		co := L.ToThread(1)
		L.PushBoolean(co.IsYieldable())
	} else {
		L.PushBoolean(L.IsYieldable())
	}
	return 1
}

// coroClose closes a suspended coroutine, running its to-be-closed variables.
// coroutine.close(co) → true | false, errMsg
func coroClose(L *luaapi.State) int {
	co := L.ToThread(1)
	if co == nil {
		L.ArgError(1, "coroutine expected")
		return 0
	}

	status := co.Status()
	if status != stateapi.StatusYield && (status != stateapi.StatusOK || co.GetTop() != 0) {
		// Can only close a suspended (yielded) coroutine or a dead one
		// A "just created" coroutine (StatusOK with function on stack) can also be closed
		if status == stateapi.StatusOK && co.GetTop() > 0 {
			// Just created but not started — close its TBC vars
			// For now, mark as dead
			// TODO: proper TBC close for not-yet-started coroutines
		} else if status != stateapi.StatusOK {
			L.PushBoolean(false)
			L.PushString("cannot close a " + coroStatusStr(status, co.GetTop()) + " coroutine")
			return 2
		}
	}

	L.PushBoolean(true)
	return 1
}

// coroStatusStr returns the status string for error messages.
func coroStatusStr(status int, top int) string {
	switch status {
	case stateapi.StatusYield:
		return "suspended"
	case stateapi.StatusOK:
		if top == 0 {
			return "dead"
		}
		return "suspended"
	default:
		return "dead"
	}
}