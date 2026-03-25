// Package api provides the public Lua API.
//
// This package implements the Lua C API in Go, providing a familiar interface
// for Lua programmers. The API is designed to closely match the Lua 5.x C API
// while following Go conventions where appropriate.
//
// # Basic Usage
//
// Create a new state, execute code, and clean up:
//
//	L := NewState()
//	defer L.Close()
//
//	// Execute Lua code
//	if err := L.DoString("print('Hello, World!')", "chunk"); err != nil {
//	    log.Fatal(err)
//	}
//
// # Stack Management
//
// The Lua API uses a virtual stack for passing values between Go and Lua.
// Stack indices can be positive (from bottom, starting at 1) or negative
// (from top, starting at -1):
//
//	L.PushNumber(42)      // Stack: [42]
//	L.PushString("hello") // Stack: [42, "hello"]
//	L.GetTop()            // Returns 2
//
//	// Access by positive index (1-based)
//	n, _ := L.ToNumber(1)      // 42
//	s, _ := L.ToString(2)      // "hello"
//
//	// Access by negative index
//	s, _ = L.ToString(-1)      // "hello" (top)
//	n, _ = L.ToNumber(-2)      // 42
//
// # Function Registration
//
// Register Go functions for Lua to call:
//
//	L.Register("myfunc", func(L *State) int {
//	    // Get arguments from stack
//	    arg, _ := L.ToNumber(1)
//
//	    // Do work...
//	    result := arg * 2
//
//	    // Push result
//	    L.PushNumber(result)
//
//	    // Return number of results
//	    return 1
//	})
//
// # Error Handling
//
// The API uses panic/recover to simulate Lua's longjmp error handling:
//
//	err := L.DoString("error('something went wrong')", "chunk")
//	if err != nil {
//	    if luaErr, ok := err.(*LuaError); ok {
//	        log.Printf("Lua error: %s", luaErr.Message)
//	    }
//	}
//
// # C API Mapping
//
// Go API          | C API           | Description
// ----------------|-----------------|---------------------------
// PushNil()       | lua_pushnil()   | Push nil onto stack
// PushBoolean(b)  | lua_pushboolean | Push boolean value
// PushNumber(n)   | lua_pushnumber  | Push number value
// PushString(s)   | lua_pushstring  | Push string value
// PushFunction(f) | lua_pushfunction| Push Go function
// GetTop()        | lua_gettop()    | Get stack top index
// SetTop(idx)     | lua_settop()    | Set stack top
// ToNumber(idx)   | lua_tonumber    | Convert to number
// ToString(idx)   | lua_tostring    | Convert to string
// ToBoolean(idx)  | lua_toboolean   | Convert to boolean
// IsNil(idx)      | lua_isnil       | Check if nil
// Call(n,r)       | lua_call()      | Call function
// PCall(n,r)      | lua_pcall()     | Protected call
// LoadString(c,n) | lua_load()      | Load code from string
// DoString(c,n)   | luaL_dostring() | Load and execute
// Register(n,f)   | lua_register()  | Register function
//
// # Thread Safety
//
// Lua states are NOT thread-safe. Each goroutine should have its own State.
// For communication between goroutines, use Go channels.
package api

import (
	"github.com/akzj/go-lua/pkg/object"
	"github.com/akzj/go-lua/pkg/state"
	"github.com/akzj/go-lua/pkg/vm"
)

// Function represents a Go function that can be called from Lua.
//
// The function receives the Lua state and should return the number of
// results it pushes onto the stack.
//
// Example:
//
//	func(L *State) int {
//	    // Get argument
//	    arg, ok := L.ToNumber(1)
//	    if !ok {
//	        L.PushString("expected number")
//	        L.Error()
//	    }
//
//	    // Push result
//	    L.PushNumber(arg * 2)
//	    return 1 // One result
//	}
type Function func(L *State) int

// State represents a Lua state (public API).
//
// State is the main interface for interacting with the Lua VM.
// It encapsulates a VM instance and provides the public API.
//
// Each State is independent and can be used in a single goroutine.
// For concurrent execution, create multiple states.
type State struct {
	vm     *vm.VM
	global *state.GlobalState
}

// NewState creates a new Lua state.
//
// This creates a new Lua state with default settings. The state should
// be closed with Close() when no longer needed to free resources.
//
// Returns:
//   - *State: A new Lua state
//
// Example:
//
//	L := NewState()
//	defer L.Close()
func NewState() *State {
	global := state.NewGlobalState()
	v := vm.NewVM(global)
	return &State{
		vm:     v,
		global: global,
	}
}

// Close closes the state and frees all resources.
//
// After calling Close, the state should not be used.
//
// Example:
//
//	L := NewState()
//	defer L.Close()
func (s *State) Close() {
	// Clear the stack
	s.vm.SetTop(0)

	// Clear references
	s.vm.Prototype = nil
	s.vm.CallInfo = nil

	// Note: In a full implementation, this would also:
	// - Trigger garbage collection
	// - Free upvalues
	// - Close any open resources
}

// PushNil pushes nil onto the stack.
//
// This corresponds to lua_pushnil in the C API.
//
// Example:
//
//	L.PushNil() // Stack: [nil]
func (s *State) PushNil() {
	s.vm.Push(object.TValue{Type: object.TypeNil})
}

// PushBoolean pushes a boolean value onto the stack.
//
// This corresponds to lua_pushboolean in the C API.
//
// Parameters:
//   - b: The boolean value to push
//
// Example:
//
//	L.PushBoolean(true)  // Stack: [true]
//	L.PushBoolean(false) // Stack: [true, false]
func (s *State) PushBoolean(b bool) {
	v := object.TValue{Type: object.TypeBoolean}
	v.Value.Bool = b
	s.vm.Push(v)
}

// PushNumber pushes a number onto the stack.
//
// This corresponds to lua_pushnumber in the C API.
// Numbers in Lua are floating-point by default.
//
// Parameters:
//   - n: The number to push
//
// Example:
//
//	L.PushNumber(42)     // Stack: [42]
//	L.PushNumber(3.14)   // Stack: [42, 3.14]
func (s *State) PushNumber(n float64) {
	v := object.TValue{Type: object.TypeNumber}
	v.Value.Num = n
	s.vm.Push(v)
}

// PushString pushes a string onto the stack.
//
// This corresponds to lua_pushstring in the C API.
//
// Parameters:
//   - str: The string to push
//
// Example:
//
//	L.PushString("hello") // Stack: ["hello"]
func (s *State) PushString(str string) {
	v := object.TValue{Type: object.TypeString}
	v.Value.Str = str
	s.vm.Push(v)
}

// PushFunction pushes a Go function onto the stack.
//
// This corresponds to lua_pushfunction in the C API.
// The function can be called from Lua code.
//
// Parameters:
//   - fn: The Go function to push
//
// Example:
//
//	L.PushFunction(func(L *State) int {
//	    L.PushString("Hello from Go!")
//	    return 1
//	})
func (s *State) PushFunction(fn Function) {
	// Create a Go closure
	closure := &object.Closure{
		IsGo: true,
		GoFn: func(vmInterface interface{}) error {
			vm, ok := vmInterface.(*vm.VM)
			if !ok {
				return RuntimeError("invalid VM type")
			}

			// Create a temporary State for the function call
			tempState := &State{vm: vm, global: s.global}

			// Call the Go function
			nResults := fn(tempState)

			// Adjust stack if needed
			// (function should have pushed nResults)
			_ = nResults

			return nil
		},
	}

	v := object.TValue{Type: object.TypeFunction}
	v.Value.GC = closure
	s.vm.Push(v)
}

// GetTop returns the index of the top element in the stack.
//
// This corresponds to lua_gettop in the C API.
// A return value of 0 means the stack is empty.
//
// Returns:
//   - int: The stack top index (0 if empty)
//
// Example:
//
//	L.PushNumber(1)
//	L.PushNumber(2)
//	top := L.GetTop() // 2
func (s *State) GetTop() int {
	return s.vm.GetTop()
}

// SetTop sets the stack top to the given index.
//
// This corresponds to lua_settop in the C API.
// If the new top is higher than the current top, new elements are set to nil.
// If the new top is lower, elements are popped.
//
// Special values:
//   - 0: Clears the stack (pops all elements)
//   - negative: Relative to the current top
//
// Parameters:
//   - index: The new stack top index
//
// Example:
//
//	L.PushNumber(1)
//	L.PushNumber(2)
//	L.PushNumber(3)
//	L.SetTop(1)      // Stack: [1]
//	L.SetTop(0)      // Stack: []
func (s *State) SetTop(index int) {
	s.vm.SetTop(index)
}

// ToNumber converts the value at the given index to a number.
//
// This corresponds to lua_tonumber in the C API.
// Returns the number and true if successful, or 0 and false if the value
// cannot be converted to a number.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - float64: The number value (0 if conversion fails)
//   - bool: True if conversion successful
//
// Example:
//
//	L.PushNumber(42)
//	n, ok := L.ToNumber(1)  // 42, true
//	s, ok := L.ToNumber(-1) // 42, true
func (s *State) ToNumber(index int) (float64, bool) {
	v := s.vm.GetStack(index)
	return v.ToNumber()
}

// ToString converts the value at the given index to a string.
//
// This corresponds to lua_tostring in the C API.
// Returns the string and true if successful, or empty string and false
// if the value cannot be converted to a string.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - string: The string value (empty if conversion fails)
//   - bool: True if conversion successful
//
// Example:
//
//	L.PushString("hello")
//	s, ok := L.ToString(1)  // "hello", true
func (s *State) ToString(index int) (string, bool) {
	v := s.vm.GetStack(index)
	return v.ToString()
}

// ToBoolean converts the value at the given index to a boolean.
//
// This corresponds to lua_toboolean in the C API.
// Returns the boolean and true if the value is a boolean,
// or false and false otherwise.
//
// Note: In Lua, only nil and false are "falsy". This method only returns
// the actual boolean value, not Lua's truthiness. Use IsTruthy for that.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - bool: The boolean value (false if not a boolean)
//   - bool: True if the value is a boolean
//
// Example:
//
//	L.PushBoolean(true)
//	b, ok := L.ToBoolean(1)  // true, true
func (s *State) ToBoolean(index int) (bool, bool) {
	v := s.vm.GetStack(index)
	return v.ToBoolean()
}

// Type returns the type of the value at the given index.
//
// This corresponds to lua_type in the C API.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - object.Type: The type of the value
//
// Example:
//
//	L.PushNumber(42)
//	t := L.Type(1)  // object.TypeNumber
func (s *State) Type(index int) object.Type {
	v := s.vm.GetStack(index)
	return v.Type
}

// TypeName returns the name of the type at the given index.
//
// This corresponds to lua_typename in the C API.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - string: The type name
//
// Example:
//
//	L.PushNumber(42)
//	name := L.TypeName(1)  // "number"
func (s *State) TypeName(index int) string {
	t := s.Type(index)
	return t.String()
}

// IsNil checks if the value at the given index is nil.
//
// This corresponds to lua_isnil in the C API.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - bool: True if the value is nil
//
// Example:
//
//	L.PushNil()
//	if L.IsNil(1) {
//	    // Value is nil
//	}
func (s *State) IsNil(index int) bool {
	v := s.vm.GetStack(index)
	return v.IsNil()
}

// IsBoolean checks if the value at the given index is a boolean.
//
// This corresponds to lua_isboolean in the C API.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - bool: True if the value is a boolean
func (s *State) IsBoolean(index int) bool {
	v := s.vm.GetStack(index)
	return v.IsBoolean()
}

// IsNumber checks if the value at the given index is a number.
//
// This corresponds to lua_isnumber in the C API.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - bool: True if the value is a number
func (s *State) IsNumber(index int) bool {
	v := s.vm.GetStack(index)
	return v.IsNumber()
}

// IsString checks if the value at the given index is a string.
//
// This corresponds to lua_isstring in the C API.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - bool: True if the value is a string
func (s *State) IsString(index int) bool {
	v := s.vm.GetStack(index)
	return v.IsString()
}

// IsFunction checks if the value at the given index is a function.
//
// This corresponds to lua_isfunction in the C API.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - bool: True if the value is a function
func (s *State) IsFunction(index int) bool {
	v := s.vm.GetStack(index)
	return v.IsFunction()
}

// IsTable checks if the value at the given index is a table.
//
// This corresponds to lua_istable in the C API.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - bool: True if the value is a table
func (s *State) IsTable(index int) bool {
	v := s.vm.GetStack(index)
	return v.IsTable()
}

// IsTruthy checks if the value at the given index is "truthy" in Lua terms.
//
// In Lua, only nil and false are falsy. All other values are truthy,
// including 0, empty strings, and empty tables.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - bool: True if the value is truthy
func (s *State) IsTruthy(index int) bool {
	v := s.vm.GetStack(index)
	return !object.IsFalse(v)
}

// Len returns the length of the value at the given index.
//
// This corresponds to lua_len in the C API.
// Works for tables and strings.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - int: The length of the value
//
// Example:
//
//	L.DoString("t = {1, 2, 3}", "")
//	L.GetGlobal("t")
//	length := L.Len(-1)  // 3
func (s *State) Len(index int) int {
	v := s.vm.GetStack(index)
	if v.IsTable() {
		t, ok := v.ToTable()
		if ok {
			return t.Len()
		}
	} else if v.IsString() {
		str, ok := v.ToString()
		if ok {
			return len(str)
		}
	}
	return 0
}

// Pop pops n values from the stack.
//
// This is a convenience function equivalent to:
//
//	L.SetTop(L.GetTop() - n)
//
// Parameters:
//   - n: Number of values to pop
//
// Example:
//
//	L.PushNumber(1)
//	L.PushNumber(2)
//	L.PushNumber(3)
//	L.Pop(2)  // Stack: [1]
func (s *State) Pop(n int) {
	s.vm.SetTop(s.vm.GetTop() - n)
}

// Copy copies the value at index fromIdx to index toIdx.
//
// This corresponds to lua_copy in the C API.
//
// Parameters:
//   - fromIdx: Source stack index
//   - toIdx: Destination stack index
func (s *State) Copy(fromIdx, toIdx int) {
	v := s.vm.GetStack(fromIdx)
	
	// If toIdx is beyond current top, we need to extend the stack
	top := s.GetTop()
	if toIdx > top {
		// Push nils to fill the gap
		for i := top; i < toIdx; i++ {
			s.PushNil()
		}
	}
	
	s.vm.SetStack(toIdx, *v)
}

// Move moves the value at index fromIdx to index toIdx.
//
// This is similar to Copy but is used when transferring values
// between different states (not implemented in this version).
//
// Parameters:
//   - fromIdx: Source stack index
//   - toIdx: Destination stack index
func (s *State) Move(fromIdx, toIdx int) {
	s.Copy(fromIdx, toIdx)
}

// CheckStack ensures the stack has space for at least extra elements.
//
// This corresponds to lua_checkstack in the C API.
// Returns true if successful, false if the stack cannot be extended.
//
// Parameters:
//   - extra: Number of additional elements needed
//
// Returns:
//   - bool: True if stack has sufficient space
func (s *State) CheckStack(extra int) bool {
	// In a full implementation, this would grow the stack if needed
	// For now, we assume the stack is large enough
	return true
}