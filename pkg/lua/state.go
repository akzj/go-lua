package lua

import (
	"github.com/akzj/go-lua/internal/api"
	"github.com/akzj/go-lua/internal/stdlib"
)

// State is the main Lua interpreter state.
// It wraps the internal API state and provides a clean public interface.
type State struct {
	s *api.State
}

// NewState creates a new Lua state with all standard libraries loaded.
func NewState() *State {
	s := api.NewState()
	stdlib.OpenAll(s)
	return &State{s: s}
}

// NewBareState creates a new Lua state without loading standard libraries.
// Use this when you need full control over which libraries are available.
func NewBareState() *State {
	return &State{s: api.NewState()}
}

// Close releases all resources associated with the Lua state.
func (L *State) Close() {
	L.s.Close()
}

// wrapFunction wraps a public Function into an internal CFunction.
func (L *State) wrapFunction(f Function) api.CFunction {
	return func(apiL *api.State) int {
		return f(&State{s: apiL})
	}
}

// ---------------------------------------------------------------------------
// Stack manipulation
// ---------------------------------------------------------------------------

// GetTop returns the index of the top element (= number of elements on the stack).
func (L *State) GetTop() int {
	return L.s.GetTop()
}

// SetTop sets the stack top to idx. If the new top is larger than the old one,
// new elements are filled with nil. If idx is 0, all stack elements are removed.
func (L *State) SetTop(idx int) {
	L.s.SetTop(idx)
}

// Pop removes n elements from the top of the stack.
func (L *State) Pop(n int) {
	L.s.Pop(n)
}

// AbsIndex converts a potentially negative index to an absolute index.
func (L *State) AbsIndex(idx int) int {
	return L.s.AbsIndex(idx)
}

// CheckStack ensures that the stack has space for at least n extra elements.
// Returns false if it cannot fulfill the request.
func (L *State) CheckStack(n int) bool {
	return L.s.CheckStack(n)
}

// Copy copies the value at fromIdx to toIdx.
func (L *State) Copy(fromIdx, toIdx int) {
	L.s.Copy(fromIdx, toIdx)
}

// Rotate rotates the stack elements between idx and the top by n positions.
func (L *State) Rotate(idx, n int) {
	L.s.Rotate(idx, n)
}

// Insert moves the top element to idx, shifting elements above.
func (L *State) Insert(idx int) {
	L.s.Insert(idx)
}

// Remove removes the element at idx, shifting elements above it down.
func (L *State) Remove(idx int) {
	L.s.Remove(idx)
}

// Replace replaces the value at idx with the top element, popping the top.
func (L *State) Replace(idx int) {
	L.s.Replace(idx)
}

// PushValue pushes a copy of the element at idx onto the stack.
func (L *State) PushValue(idx int) {
	L.s.PushValue(idx)
}
