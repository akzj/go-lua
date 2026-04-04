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
// NewVMFrameManager creates a new VM frame manager for integrated execution.
func NewVMFrameManager() vmapi.VMFrameManager {
	return &Executor{
		stack:  make([]TValue, 32),
		frames: make([]*Frame, 0),
	}
}

// SetStack sets the shared stack for the executor
func (e *Executor) SetStack(stack []types.TValue) {
	// Convert from interface slice to concrete slice
	e.stack = make([]TValue, len(stack))
	for i, v := range stack {
		// Handle nil interface - check using reflection-style approach
		if v == nil {
			e.stack[i] = TValue{Tt: uint8(types.LUA_VNIL)}
			continue
		}
		if tv, ok := v.(*TValue); ok {
			if tv != nil {
				e.stack[i] = *tv
			} else {
				e.stack[i] = TValue{Tt: uint8(types.LUA_VNIL)}
			}
		} else {
			// Wrap interface value in concrete struct
			variant, data := extractVariantAndData(v)
			e.stack[i] = TValue{
				Value: Value{
					Variant: variant,
					Data_:   data,
				},
				Tt: uint8(v.GetTag()),
			}
		}
	}
}

// SetCode sets the bytecode instructions to execute
func (e *Executor) SetCode(code []opcodes.Instruction) {
	e.code = code
	e.pc = 0
}

// SetKValues sets the constant pool for the current frame
func (e *Executor) SetKValues(kvalues []types.TValue) {
	if len(e.frames) > 0 {
		// Convert from interface slice to concrete slice
		concrete := make([]TValue, len(kvalues))
		for i, v := range kvalues {
			if v == nil {
				concrete[i] = TValue{Tt: uint8(types.LUA_VNIL)}
				continue
			}
			if tv, ok := v.(*TValue); ok {
				if tv != nil {
					concrete[i] = *tv
				} else {
					concrete[i] = TValue{Tt: uint8(types.LUA_VNIL)}
				}
			} else {
				variant, data := extractVariantAndData(v)
				concrete[i] = TValue{
					Value: Value{
						Variant: variant,
						Data_:   data,
					},
					Tt: uint8(v.GetTag()),
				}
			}
		}
		e.frames[len(e.frames)-1].kvalues = concrete
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
	var kvalues []TValue
	if kf, ok := frame.(interface{ KValues() []types.TValue }); ok {
		kv := kf.KValues()
		kvalues = make([]TValue, len(kv))
		for i, v := range kv {
			if v == nil {
				kvalues[i] = TValue{Tt: uint8(types.LUA_VNIL)}
				continue
			}
			if tv, ok := v.(*TValue); ok {
				if tv != nil {
					kvalues[i] = *tv
				} else {
					kvalues[i] = TValue{Tt: uint8(types.LUA_VNIL)}
				}
			} else {
				variant, data := extractVariantAndData(v)
				kvalues[i] = TValue{
					Value: Value{
						Variant: variant,
						Data_:   data,
					},
					Tt: uint8(v.GetTag()),
				}
			}
		}
	}
	
	// Get closure value as pointer
	closure := frame.Func()
	var closurePtr *TValue
	if closure == nil {
		closurePtr = &TValue{Tt: uint8(types.LUA_VNIL)}
	} else if ct, ok := closure.(*TValue); ok {
		if ct != nil {
			closurePtr = ct
		} else {
			closurePtr = &TValue{Tt: uint8(types.LUA_VNIL)}
		}
	} else {
		// Wrap interface value in concrete pointer
		variant, data := extractVariantAndData(closure)
		tv := &TValue{
			Value: Value{
				Variant: variant,
				Data_:   data,
			},
			Tt: uint8(closure.GetTag()),
		}
		closurePtr = tv
	}
	
	return &Frame{
		Closure: closurePtr,
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
