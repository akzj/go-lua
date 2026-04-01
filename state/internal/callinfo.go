// Package internal provides the concrete implementation of state/api interfaces.
package internal

import (
	"github.com/akzj/go-lua/state/api"
)

// callInfo is the concrete implementation of CallInfo.
// Tracks a single function call frame.
type callInfo struct {
	func_   int         // Stack index of the function
	top     int         // Top of the stack for this frame
	prev    *callInfo   // Previous call info (caller)
	nresults int       // Expected number of return values
}

func (ci *callInfo) Func() int {
	return ci.func_
}

func (ci *callInfo) Top() int {
	return ci.top
}

func (ci *callInfo) Prev() api.CallInfo {
	if ci.prev == nil {
		return nil
	}
	return ci.prev
}

func (ci *callInfo) SetFunc(idx int) {
	ci.func_ = idx
}

func (ci *callInfo) SetTop(idx int) {
	ci.top = idx
}

func (ci *callInfo) NResults() int {
	return ci.nresults
}

func (ci *callInfo) SetNResults(n int) {
	ci.nresults = n
}

// SetPrev sets the previous call info (internal method)
func (ci *callInfo) SetPrev(prev api.CallInfo) {
	if prev == nil {
		ci.prev = nil
	} else if typedPrev, ok := prev.(*callInfo); ok {
		ci.prev = typedPrev
	}
}
