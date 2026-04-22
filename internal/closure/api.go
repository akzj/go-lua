// Package api defines Lua closures, C closures, and upvalue types.
//
// LClosure wraps a Proto (compiled function) with captured upvalues.
// CClosure wraps a Go function with associated upvalues.
// UpVal implements the open/closed duality for captured variables.
//
// Reference: .analysis/07-runtime-infrastructure.md §2
package closure

import (
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/state"
)

// MaxUpVal is the maximum number of upvalues per closure.
const MaxUpVal = 255

// ---------------------------------------------------------------------------
// UpVal represents a captured variable (upvalue).
//
// C4 FIX: Index-based approach to avoid dangling pointers on stack reallocation.
//
// Open state: StackIdx >= 0, value lives at L.Stack[StackIdx].Val
// Closed state: StackIdx == -1, value lives in Own
//
// The open upvalue list is sorted by stack level in descending order.
// ---------------------------------------------------------------------------
type UpVal struct {
	object.GCHeader               // GC metadata
	StackIdx int              // stack index when open, -1 when closed
	Own      object.TValue // storage for closed value
	Next     *UpVal           // next in open list (lower stack level)
	Stack    *[]object.StackValue // pointer to owning thread's stack (for cross-thread access)
}

// GC returns the GC header for this upvalue.
func (uv *UpVal) GC() *object.GCHeader { return &uv.GCHeader }

// IsOpen returns true if the upvalue still points to a stack slot.
func (uv *UpVal) IsOpen() bool {
	return uv.StackIdx >= 0
}

// Close captures the current value from the stack and marks as closed.
// The caller must pass the current stack value at uv.StackIdx.
func (uv *UpVal) Close(val object.TValue) {
	uv.Own = val
	uv.StackIdx = -1
	uv.Stack = nil // no longer needed
}

// Get returns the current value of the upvalue.
// For closed upvalues (common case), returns Own directly.
// For open upvalues, reads from the owning thread's stack.
func (uv *UpVal) Get(stack []object.StackValue) object.TValue {
	if uv.StackIdx < 0 {
		return uv.Own
	}
	if uv.Stack != nil {
		return (*uv.Stack)[uv.StackIdx].Val
	}
	return stack[uv.StackIdx].Val
}

// Set sets the value of the upvalue.
// For open upvalues, uses the stored stack reference (falls back to provided stack).
// For closed upvalues, writes to Own.
func (uv *UpVal) Set(stack []object.StackValue, val object.TValue) {
	if uv.StackIdx >= 0 {
		if uv.Stack != nil {
			(*uv.Stack)[uv.StackIdx].Val = val
		} else {
			stack[uv.StackIdx].Val = val
		}
	} else {
		uv.Own = val
	}
}

// ---------------------------------------------------------------------------
// LClosure is a Lua function closure.
// It wraps a Proto with captured upvalues.
// ---------------------------------------------------------------------------
type LClosure struct {
	object.GCHeader               // GC metadata
	Proto  *object.Proto // compiled function prototype
	UpVals []*UpVal         // captured upvalues (len == Proto.Upvalues)
}

// GC returns the GC header for this Lua closure.
func (cl *LClosure) GC() *object.GCHeader { return &cl.GCHeader }

// ---------------------------------------------------------------------------
// CClosure is a Go function closure with associated upvalues.
// Upvalues are stored inline as TValues (no sharing between closures).
//
// I13 FIX: Fn uses state.CFunction (canonical type, not func(any)int).
// ---------------------------------------------------------------------------
type CClosure struct {
	object.GCHeader               // GC metadata
	Fn     state.CFunction  // the Go function
	UpVals []object.TValue  // upvalues stored inline
}

// GC returns the GC header for this C closure.
func (cl *CClosure) GC() *object.GCHeader { return &cl.GCHeader }

// NumUpvals returns the number of upvalues.
func (c *CClosure) NumUpvals() int { return len(c.UpVals) }

// ---------------------------------------------------------------------------
// Constructor functions
// ---------------------------------------------------------------------------

// NewLClosure creates a Lua closure with n upvalue slots (initially nil).
func NewLClosure(p *object.Proto, nUpvals int) *LClosure {
	cl := getLClosure()
	cl.Proto = p
	cl.UpVals = make([]*UpVal, nUpvals)
	return cl
}

// NewCClosure creates a C closure with n upvalue slots (initially nil).
func NewCClosure(fn state.CFunction, nUpvals int) *CClosure {
	cl := &CClosure{
		Fn:     fn,
		UpVals: make([]object.TValue, nUpvals),
	}
	// Initialize upvalues to nil
	for i := range cl.UpVals {
		cl.UpVals[i] = object.Nil
	}
	return cl
}

// ---------------------------------------------------------------------------
// Upvalue management function signatures
// (Implemented in internal/closure/closure.go)
//
// I4 FIX: CloseUpvalues function signature added.
//
// FindUpval finds or creates an open upvalue at the given stack level.
// The open upvalue list is maintained sorted by stack level (descending).
// If an upvalue already exists at this level, it is returned (sharing).
//   func FindUpval(L *state.LuaState, level int) *UpVal
//
// CloseUpvalues closes all open upvalues at or above the given stack level.
// This is called when leaving a scope that has captured variables.
// Needed by OP_CLOSE, OP_RETURN, and TBC close.
//   func CloseUpvalues(L *state.LuaState, level int)
// ---------------------------------------------------------------------------
