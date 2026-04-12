// Package api defines Lua closures, C closures, and upvalue types.
//
// LClosure wraps a Proto (compiled function) with captured upvalues.
// CClosure wraps a Go function with associated upvalues.
// UpVal implements the open/closed duality for captured variables.
//
// Reference: .analysis/07-runtime-infrastructure.md §2
package api

import (
	objectapi "github.com/akzj/go-lua/internal/object/api"
)

// MaxUpVal is the maximum number of upvalues per closure.
const MaxUpVal = 255

// ---------------------------------------------------------------------------
// UpVal represents a captured variable (upvalue).
//
// Open state: Value points to a slot in the owning LuaState's stack.
// Closed state: Value points to Own (the captured copy).
//
// The open upvalue list is sorted by stack level in descending order.
// ---------------------------------------------------------------------------
type UpVal struct {
	Value    *objectapi.TValue // points to stack slot (open) or &Own (closed)
	Own      objectapi.TValue  // storage for closed value
	Next     *UpVal            // next in open list (lower stack level)
	StackIdx int               // stack index this upvalue captures (for sorting)
}

// IsOpen returns true if the upvalue still points to a stack slot.
func (uv *UpVal) IsOpen() bool {
	return uv.Value != &uv.Own
}

// Close captures the current value and redirects the pointer.
// After Close(), the upvalue owns its value independently of the stack.
func (uv *UpVal) Close() {
	uv.Own = *uv.Value // copy value from stack
	uv.Value = &uv.Own // redirect to own storage
}

// ---------------------------------------------------------------------------
// LClosure is a Lua function closure.
// It wraps a Proto with captured upvalues.
// ---------------------------------------------------------------------------
type LClosure struct {
	Proto  *objectapi.Proto // compiled function prototype
	UpVals []*UpVal         // captured upvalues (len == Proto.Upvalues)
}

// ---------------------------------------------------------------------------
// CClosure is a Go function closure with associated upvalues.
// Upvalues are stored inline as TValues (no sharing between closures).
// ---------------------------------------------------------------------------
type CClosure struct {
	Fn     func(L any) int       // the Go function (L typed as *LuaState at call site)
	UpVals []objectapi.TValue    // upvalues stored inline
}

// NumUpvals returns the number of upvalues.
func (c *CClosure) NumUpvals() int { return len(c.UpVals) }

// ---------------------------------------------------------------------------
// Constructor function signatures (implemented in closure.go)
// ---------------------------------------------------------------------------

// NewLClosure creates a Lua closure with n upvalue slots (initially nil).
func NewLClosure(p *objectapi.Proto, nUpvals int) *LClosure {
	cl := &LClosure{
		Proto:  p,
		UpVals: make([]*UpVal, nUpvals),
	}
	return cl
}

// NewCClosure creates a C closure with n upvalue slots (initially nil).
func NewCClosure(fn func(L any) int, nUpvals int) *CClosure {
	cl := &CClosure{
		Fn:     fn,
		UpVals: make([]objectapi.TValue, nUpvals),
	}
	// Initialize upvalues to nil
	for i := range cl.UpVals {
		cl.UpVals[i] = objectapi.Nil
	}
	return cl
}
