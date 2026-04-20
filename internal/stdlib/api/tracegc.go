// tracegc module — test helper for suppressing/enabling GC finalizer execution.
//
// Provides two functions:
//   - tracegc.stop()  — suppress DrainGCFinalizers (sets GCStopped = true)
//   - tracegc.start() — re-enable DrainGCFinalizers (sets GCStopped = false)
//
// Used by cstack.lua tests to control when finalizers fire.
// This is a stub that provides just enough for the test suite;
// C Lua's tracegc is more complex (tracks individual finalizer calls).
package api

import (
	luaapi "github.com/akzj/go-lua/internal/api/api"
)

func tracegc_stop(L *luaapi.State) int {
	L.SetGCStopped(true)
	return 0
}

func tracegc_start(L *luaapi.State) int {
	L.SetGCStopped(false)
	return 0
}

// OpenTraceGC is the opener for the "tracegc" module.
// Returns a table with stop/start functions.
func OpenTraceGC(L *luaapi.State) int {
	funcs := map[string]luaapi.CFunction{
		"stop":  tracegc_stop,
		"start": tracegc_start,
	}
	L.NewTable()
	for name, fn := range funcs {
		L.PushCFunction(fn)
		L.SetField(-2, name)
	}
	return 1
}
