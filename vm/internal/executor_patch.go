// Package internal implements the VM execution engine.
package internal

import (
	opcodes "github.com/akzj/go-lua/opcodes/api"
	vmapi "github.com/akzj/go-lua/vm/api"
	types "github.com/akzj/go-lua/types/api"
)

// Compile-time interface checks
var _ vmapi.VMFrameManager = (*Executor)(nil)

// NewVMFrameManager implements vmapi.NewVMFrameManager
func NewVMFrameManager() vmapi.VMFrameManager {
	return &Executor{
		stack:  make([]types.TValue, 32),
		frames: make([]*Frame, 0),
	}
}

// SetStack sets the shared stack for the executor
func (e *Executor) SetStack(stack []types.TValue) {
	e.stack = stack
}

// SetCode sets the bytecode instructions to execute
func (e *Executor) SetCode(code []opcodes.Instruction) {
	e.code = code
	e.pc = 0
}

// SetKValues sets the constant pool for the current frame
func (e *Executor) SetKValues(kvalues []types.TValue) {
	if len(e.frames) > 0 {
		e.frames[len(e.frames)-1].kvalues = kvalues
	}
}

// PushFrame pushes a new frame onto the stack
func (e *Executor) PushFrame(frame vmapi.StackFrame) {
	// Try to get internal Frame
	if f, ok := frame.(*Frame); ok {
		e.frames = append(e.frames, f)
		return
	}
	
	// For external StackFrame implementations, create a wrapper
	e.frames = append(e.frames, NewFrameWrapper(frame))
}

// NewFrameWrapper creates a Frame from an external StackFrame
func NewFrameWrapper(frame vmapi.StackFrame) *Frame {
	// Try to get kvalues from the frame if it has them
	var kvalues []types.TValue
	if kf, ok := frame.(interface{ KValues() []types.TValue }); ok {
		kvalues = kf.KValues()
	}
	return &Frame{
		Closure: frame.Func(),
		base:    frame.Base(),
		savedPC: frame.PC(),
		prev:    nil,
		kvalues: kvalues,
		upvals:  nil,
	}
}

// PopFrame pops the top frame from the stack
func (e *Executor) PopFrame() {
	if len(e.frames) > 0 {
		e.frames = e.frames[:len(e.frames)-1]
	}
}

// CurrentFrame returns the current frame without popping
func (e *Executor) CurrentFrame() vmapi.StackFrame {
	if len(e.frames) == 0 {
		return nil
	}
	return e.frames[len(e.frames)-1]
}

// FrameCount returns the number of frames on the stack
func (e *Executor) FrameCount() int {
	return len(e.frames)
}
