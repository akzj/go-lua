// Package api defines the interface for the Lua VM execution engine.
//
// This package merges the responsibilities of C's lvm.c (execution loop)
// and ldo.c (call/return/error handling) because they are mutually recursive.
// In Go, this avoids circular imports while matching the C reality.
//
// Reference: .analysis/05-vm-execution-loop.md, .analysis/04-call-return-error.md
package api

import (
	objectapi "github.com/akzj/go-lua/internal/object/api"
)

// --- Error Handling ---

// LuaError represents a Lua runtime error, used with panic/recover
// to implement C's setjmp/longjmp error handling model.
type LuaError struct {
	Status  Status          // error status code
	Message objectapi.TValue // error object (usually a string, but can be any value)
}

func (e *LuaError) Error() string {
	return "lua error" // detail in Message
}

// Status represents the result status of a Lua operation.
type Status int

const (
	StatusOK        Status = 0 // LUA_OK
	StatusYield     Status = 1 // LUA_YIELD
	StatusErrRun    Status = 2 // LUA_ERRRUN
	StatusErrSyntax Status = 3 // LUA_ERRSYNTAX
	StatusErrMem    Status = 4 // LUA_ERRMEM
	StatusErrErr    Status = 5 // LUA_ERRERR
)

// MultiReturn signals "keep all return values" (LUA_MULTRET = -1).
const MultiReturn = -1

// --- Call Info Status Flags ---

// CallStatus flag bits, matching C's CIST_* constants.
const (
	CISTNResults  uint32 = 0xFF       // bits 0-7: nresults + 1
	CISTC         uint32 = 1 << 15    // call is running a C function
	CISTFresh     uint32 = 1 << 16    // fresh luaV_execute frame (boundary marker)
	CISTTail      uint32 = 1 << 17    // call was tail-called
	CISTTBC       uint32 = 1 << 18    // has to-be-closed variables
	CISTYPCall    uint32 = 1 << 19    // yieldable protected call
	CISTHooked    uint32 = 1 << 20    // running a debug hook
	CISTOAH       uint32 = 1 << 21    // original allowhook value
	CISTClsRet    uint32 = 1 << 22    // closing tbc variables on return
	CISTHookYield uint32 = 1 << 23    // last hook call yielded
	CISTFin       uint32 = 1 << 24    // function called a finalizer
)

// MaxCCalls is the maximum C-call depth (maps to Go call depth).
const MaxCCalls = 200

// MaxResults is the maximum number of results a function can return.
const MaxResults = 250
