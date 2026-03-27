// Package api provides the public Lua API
// This file implements the tracegc module (stub implementation)
package api

// openTracegcLib registers the tracegc module (stub)
// This module is used by Lua 5.4 tests to trace garbage collection.
// Our stub implementation provides the interface but does nothing.
func (s *State) openTracegcLib() {
	funcs := map[string]Function{
		"start": stdTracegcStart,
		"stop":  stdTracegcStop,
	}
	s.RegisterModule("tracegc", funcs)
}

// stdTracegcStart implements tracegc.start() - stub
func stdTracegcStart(L *State) int {
	// Stub implementation - does nothing
	return 0
}

// stdTracegcStop implements tracegc.stop() - stub
func stdTracegcStop(L *State) int {
	// Stub implementation - does nothing
	return 0
}