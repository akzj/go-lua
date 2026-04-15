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

// Coroutine status constants (mirrors COS_* in lcorolib.c)
const (
	cosRun  = 0 // running
	cosDead = 1 // dead
	cosYld  = 2 // suspended (yielded)
	cosNorm = 3 // normal (resumed another coroutine)
)

var cosStatName = [4]string{"running", "dead", "suspended", "normal"}

// auxStatus mirrors C Lua's auxstatus: determines the status of coroutine co
// relative to the calling thread L.
func auxStatus(L *luaapi.State, co *luaapi.State) int {
	if L.Internal == co.Internal {
		return cosRun
	}
	status := co.Status()
	switch status {
	case stateapi.StatusYield:
		return cosYld
	case stateapi.StatusOK:
		// Check if it has frames above base CI (= normal, i.e. it resumed another coroutine)
		if co.HasCallFrames() {
			return cosNorm
		}
		if co.GetTop() == 0 {
			return cosDead
		}
		return cosYld // initial state (not started yet)
	default:
		return cosDead // error status = dead
	}
}

// coroStatus returns the status of a coroutine as a string.
// coroutine.status(co) → "running" | "suspended" | "normal" | "dead"
func coroStatus(L *luaapi.State) int {
	co := L.ToThread(1)
	if co == nil {
		L.ArgError(1, "coroutine expected")
		return 0
	}
	L.PushString(cosStatName[auxStatus(L, co)])
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
// coroClose closes a suspended coroutine, running its to-be-closed variables.
// coroutine.close(co) → true | false, errMsg
// Mirrors: luaB_close in lcorolib.c
func coroClose(L *luaapi.State) int {
	// getoptco: if no argument, close the running thread itself
	var co *luaapi.State
	if L.IsNone(1) {
		co = L // close itself
	} else {
		co = L.ToThread(1)
		if co == nil {
			L.ArgError(1, "coroutine expected")
			return 0
		}
	}

	st := auxStatus(L, co)
	switch st {
	case cosDead, cosYld:
		// Can close dead or suspended coroutines
		// Mirrors: luaB_close → lua_closethread in lcorolib.c:176
		coState := co.Internal.(*stateapi.LuaState)
		callerState := L.Internal.(*stateapi.LuaState)
		status := vmapi.CloseThread(coState, callerState)
		if status == stateapi.StatusOK {
			L.PushBoolean(true)
			return 1
		}
		L.PushBoolean(false)
		co.XMove(L, 1) // move error message from co to caller
		return 2
	case cosNorm:
		L.PushString("cannot close a normal coroutine")
		L.Error()
		return 0
	case cosRun:
		// Check if it's the main thread
		L.RawGetI(luaapi.RegistryIndex, int64(stateapi.RegistryIndexMainThread))
		mainThread := L.ToThread(-1)
		L.Pop(1)
		if mainThread != nil && mainThread.Internal == co.Internal {
			L.PushString("cannot close main thread")
			L.Error()
			return 0
		}
		// Running non-main coroutine — close itself
		// Mirrors: luaB_close COS_RUN case in lcorolib.c:194-198
		// CloseThread with L == from will panic(LuaBaseLevel) to unwind
		// past all inner pcalls back to Resume.
		coState := co.Internal.(*stateapi.LuaState)
		callerState := L.Internal.(*stateapi.LuaState)
		vmapi.CloseThread(coState, callerState)
		// CloseThread panics when L == from, so this is unreachable
		return 0
	default:
		L.PushBoolean(false)
		L.PushString("cannot close coroutine")
		return 2
	}
}