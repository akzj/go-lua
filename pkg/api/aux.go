// Package api provides the public Lua API
package api

import (
	"github.com/akzj/go-lua/pkg/object"
)

// absIndex converts a stack index to an absolute index.
//
// Parameters:
//   - index: Stack index (positive from bottom, negative from top)
//
// Returns:
//   - int: Absolute index (positive, 1-based)
func (s *State) absIndex(index int) int {
	if index > 0 {
		return index
	}
	// Negative index: convert to absolute
	// -1 = top, -2 = top-1, etc.
	return s.GetTop() + index + 1
}

// Register registers a Go function as a global variable.
//
// This corresponds to lua_register in the C API.
// It's equivalent to:
//
//	L.PushFunction(fn)
//	L.SetGlobal(name)
//
// Parameters:
//   - name: The global name
//   - fn: The Go function to register
//
// Example:
//
//	L.Register("print", func(L *State) int {
//	    // Implement print
//	    return 0
//	})
func (s *State) Register(name string, fn Function) {
	s.PushFunction(fn)
	s.SetGlobal(name)
}

// SetGlobal sets a global variable.
//
// This corresponds to lua_setglobal in the C API.
// It pops the top value from the stack and sets it as the global variable.
//
// Parameters:
//   - name: The global variable name
//
// Example:
//
//	L.PushNumber(42)
//	L.SetGlobal("answer")
//	// Now global 'answer' = 42
func (s *State) SetGlobal(name string) {
	// Get the value from the stack
	v := s.vm.Pop()

	// Create or get the global table
	globalTable := s.getGlobalTable()

	// Create key
	key := object.TValue{Type: object.TypeString, Value: object.Value{Str: name}}

	// Set in global table
	globalTable.Set(key, v)
}

// GetGlobal gets a global variable.
//
// This corresponds to lua_getglobal in the C API.
// It pushes the value of the global variable onto the stack.
//
// Parameters:
//   - name: The global variable name
//
// Returns:
//   - object.Type: The type of the value
//
// Example:
//
//	L.GetGlobal("print")
//	// 'print' function is now on stack
func (s *State) GetGlobal(name string) object.Type {
	// Get the global table
	globalTable := s.getGlobalTable()

	// Create key
	key := object.TValue{Type: object.TypeString, Value: object.Value{Str: name}}

	// Get from global table
	val := globalTable.Get(key)

	// Push onto stack
	if val != nil {
		s.vm.Push(*val)
		return val.Type
	}

	// Not found, push nil
	s.PushNil()
	return object.TypeNil
}

// getGlobalTable gets or creates the global table.
//
// Returns:
//   - *object.Table: The global table
func (s *State) getGlobalTable() *object.Table {
	// In a full implementation, this would get the global table from the registry
	// For now, create a simple global table
	if s.global.Registry == nil {
		s.global.Registry = object.NewTable()
	}
	return s.global.Registry
}

// SetField sets a field in a table.
//
// This corresponds to lua_setfield in the C API.
// It pops the top value and sets it in the table at the given index.
//
// Parameters:
//   - index: Stack index of the table
//   - key: The field name
//
// Example:
//
//	L.GetGlobal("mytable")
//	L.PushNumber(42)
//	L.SetField(-2, "value")
//	// mytable.value = 42
func (s *State) SetField(index int, key string) {
	// Convert to absolute index BEFORE popping
	absIdx := s.absIndex(index)

	// Get the value to set
	v := s.vm.Pop()

	// Get the table using absolute index
	tVal := s.vm.GetStack(absIdx)
	if !tVal.IsTable() {
		panic("attempt to index a non-table value")
	}

	t, _ := tVal.ToTable()

	// Create key
	keyVal := object.TValue{Type: object.TypeString, Value: object.Value{Str: key}}

	// Set in table
	t.Set(keyVal, v)
}

// GetField gets a field from a table.
//
// This corresponds to lua_getfield in the C API.
// It pushes the field value onto the stack.
//
// Parameters:
//   - index: Stack index of the table
//   - key: The field name
//
// Returns:
//   - object.Type: The type of the value
//
// Example:
//
//	L.GetGlobal("mytable")
//	L.GetField(-1, "value")
//	// mytable.value is now on stack
func (s *State) GetField(index int, key string) object.Type {
	// Get the table
	tVal := s.vm.GetStack(index)
	if !tVal.IsTable() {
		s.PushNil()
		return object.TypeNil
	}

	t, _ := tVal.ToTable()

	// Create key
	keyVal := object.TValue{Type: object.TypeString, Value: object.Value{Str: key}}

	// Get from table
	val := t.Get(keyVal)

	// Push onto stack
	if val != nil {
		s.vm.Push(*val)
		return val.Type
	}

	s.PushNil()
	return object.TypeNil
}

// SetI sets an integer-indexed field in a table.
//
// This corresponds to lua_seti in the C API.
// It pops the top value and sets it at the given integer index.
//
// Parameters:
//   - index: Stack index of the table
//   - i: The integer index
//
// Example:
//
//	L.GetGlobal("myarray")
//	L.PushNumber(42)
//	L.SetI(-2, 1)
//	// myarray[1] = 42
func (s *State) SetI(index int, i int) {
	// Convert to absolute index BEFORE popping
	absIdx := s.absIndex(index)

	// Get the value to set
	v := s.vm.Pop()

	// Get the table using absolute index
	tVal := s.vm.GetStack(absIdx)
	if !tVal.IsTable() {
		panic("attempt to index a non-table value")
	}

	t, _ := tVal.ToTable()
	t.SetI(i, v)
}

// GetI gets an integer-indexed field from a table.
//
// This corresponds to lua_geti in the C API.
// It pushes the value at the given integer index onto the stack.
//
// Parameters:
//   - index: Stack index of the table
//   - i: The integer index
//
// Returns:
//   - object.Type: The type of the value
func (s *State) GetI(index int, i int) object.Type {
	// Get the table
	tVal := s.vm.GetStack(index)
	if !tVal.IsTable() {
		s.PushNil()
		return object.TypeNil
	}

	t, _ := tVal.ToTable()

	val := t.GetI(i)

	if val != nil {
		s.vm.Push(*val)
		return val.Type
	}

	s.PushNil()
	return object.TypeNil
}

// RawSet sets a field in a table without metamethods.
//
// This corresponds to lua_rawset in the C API.
// It pops the top value (value) and sets it in the table at index.
// The key is at index - 1 (popped by this function).
//
// Parameters:
//   - index: Stack index of the table
//
// Example:
//
//	L.GetGlobal("mytable")
//	L.PushString("key")
//	L.PushNumber(42)
//	L.RawSet(-3)
//	// mytable["key"] = 42 (no metamethods)
func (s *State) RawSet(index int) {
	// Convert to absolute index BEFORE popping
	absIdx := s.absIndex(index)

	// Get value (top)
	value := s.vm.Pop()

	// Get key (now top)
	key := s.vm.Pop()

	// Get the table using absolute index
	tVal := s.vm.GetStack(absIdx)
	if !tVal.IsTable() {
		panic("attempt to index a non-table value")
	}

	t, _ := tVal.ToTable()
	t.Set(key, value)
}

// RawGet gets a field from a table without metamethods.
//
// This corresponds to lua_rawget in the C API.
// It pops the key and pushes the value.
//
// Parameters:
//   - index: Stack index of the table
//
// Returns:
//   - object.Type: The type of the value
func (s *State) RawGet(index int) object.Type {
	// Convert to absolute index BEFORE popping
	absIdx := s.absIndex(index)

	// Get key (top)
	key := s.vm.Pop()

	// Get the table using absolute index
	tVal := s.vm.GetStack(absIdx)
	if !tVal.IsTable() {
		s.PushNil()
		return object.TypeNil
	}

	t, _ := tVal.ToTable()

	val := t.Get(key)

	if val != nil {
		s.vm.Push(*val)
		return val.Type
	}

	s.PushNil()
	return object.TypeNil
}

// RawSetI sets an integer field without metamethods.
//
// This corresponds to lua_rawseti in the C API.
//
// Parameters:
//   - index: Stack index of the table
//   - i: The integer index
func (s *State) RawSetI(index int, i int) {
	// Convert to absolute index BEFORE popping
	absIdx := s.absIndex(index)

	// Get value (top)
	value := s.vm.Pop()

	// Get the table using absolute index
	tVal := s.vm.GetStack(absIdx)
	if !tVal.IsTable() {
		panic("attempt to index a non-table value")
	}

	t, _ := tVal.ToTable()
	t.SetI(i, value)
}

// RawGetI gets an integer field without metamethods.
//
// This corresponds to lua_rawgeti in the C API.
//
// Parameters:
//   - index: Stack index of the table
//   - i: The integer index
//
// Returns:
//   - object.Type: The type of the value
func (s *State) RawGetI(index int, i int) object.Type {
	// Get the table
	tVal := s.vm.GetStack(index)
	if !tVal.IsTable() {
		s.PushNil()
		return object.TypeNil
	}

	t, _ := tVal.ToTable()

	val := t.GetI(i)

	if val != nil {
		s.vm.Push(*val)
		return val.Type
	}

	s.PushNil()
	return object.TypeNil
}

// NewTable creates a new table and pushes it onto the stack.
//
// This corresponds to lua_newtable in the C API.
//
// Example:
//
//	L.NewTable()
//	// Empty table is now on stack
func (s *State) NewTable() {
	t := object.NewTable()
	v := object.TValue{Type: object.TypeTable, Value: object.Value{GC: t}}
	s.vm.Push(v)
}

// CreateTable creates a new table with pre-allocated space.
//
// This corresponds to lua_createtable in the C API.
//
// Parameters:
//   - narr: Pre-allocated array size
//   - nrec: Pre-allocated map size
//
// Example:
//
//	L.CreateTable(10, 0)  // Array with 10 elements
//	L.CreateTable(0, 5)   // Map with 5 entries
func (s *State) CreateTable(narr, nrec int) {
	t := object.NewTableWithSize(narr, nrec)
	v := object.TValue{Type: object.TypeTable, Value: object.Value{GC: t}}
	s.vm.Push(v)
}

// Next iterates over a table.
//
// This corresponds to lua_next in the C API.
// It pops the key and pushes the next key-value pair.
//
// Parameters:
//   - index: Stack index of the table
//
// Returns:
//   - bool: True if there are more entries
//
// Example:
//
//	// Iterate over table at index -1
//	L.PushNil()  // First key
//	for L.Next(-2) {
//	    // Key is at -2, value at -1
//	    L.Pop(1)  // Remove value, keep key for next iteration
//	}
func (s *State) Next(index int) bool {
	// Get the table
	tVal := s.vm.GetStack(index)
	if !tVal.IsTable() {
		return false
	}

	t, _ := tVal.ToTable()

	// Get key (top)
	key := s.vm.Pop()

	// Get next key-value pair (pass pointer to key)
	nextKey, nextVal := t.Next(&key)

	if nextKey == nil {
		// End of iteration
		return false
	}

	// Push key and value
	s.vm.Push(*nextKey)
	s.vm.Push(*nextVal)

	return true
}

// LenOp pushes the length of the value at the given index.
//
// This corresponds to lua_len in the C API.
//
// Parameters:
//   - index: Stack index
func (s *State) LenOp(index int) {
	length := s.Len(index)
	s.PushNumber(float64(length))
}

// Error raises a Lua error.
//
// This corresponds to lua_error in the C API.
// It raises an error with the message at the top of the stack.
//
// This function never returns; it panics.
func (s *State) Error() {
	// Get error message from stack
	msg, _ := s.ToString(-1)
	panic(newLuaError(msg))
}

// Errorf raises a formatted Lua error.
//
// This is a convenience function that formats the error message.
//
// Parameters:
//   - format: Format string
//   - args: Format arguments
func (s *State) Errorf(format string, args ...interface{}) {
	msg := format
	if len(args) > 0 {
		msg = format // In a full implementation, use fmt.Sprintf
	}
	panic(newLuaError(msg))
}

// GcControl controls the garbage collector.
//
// This corresponds to lua_gc in the C API.
//
// Parameters:
//   - what: What operation to perform
//   - data: Optional data for the operation
//
// Returns:
//   - int: Result of the operation
func (s *State) GcControl(what int, data int) int {
	// For now, just return 0
	// In a full implementation, this would control the GC
	return 0
}

// GC constants
const (
	GCStop      = iota // Stop GC
	GCRestart          // Restart GC
	GCCollect          // Perform a full collection
	GCCount            // Return memory in KB
	GCCountB           // Return remainder of memory in bytes
	GCStep             // Perform a step
	GCSetPause         // Set pause
	GCSetStepMul       // Set step multiplier
)

// SetAllocFunc sets the memory allocator function.
//
// This corresponds to lua_setallocf in the C API.
//
// Not fully implemented in this version.
func (s *State) SetAllocFunc(f interface{}, ud interface{}) {
	// For now, do nothing
}

// SetUserdata sets the user data for the state.
//
// This corresponds to lua_setuserdat in the C API.
//
// Not fully implemented in this version.
func (s *State) SetUserdata(ud interface{}) {
	// For now, do nothing
}

// Userdata gets the user data for the state.
//
// This corresponds to lua_userdat in the C API.
//
// Returns:
//   - interface{}: The user data
func (s *State) Userdata() interface{} {
	// For now, return nil
	return nil
}

// AtPanic sets the panic function.
//
// This corresponds to lua_atpanic in the C API.
//
// Parameters:
//   - panicf: The panic function
//
// Returns:
//   - Function: The previous panic function
func (s *State) AtPanic(panicf Function) Function {
	// For now, just return nil
	// In a full implementation, this would set the panic function
	return nil
}

// Version returns the Lua version string.
//
// This corresponds to lua_version in the C API.
//
// Returns:
//   - string: The version string
func (s *State) Version() string {
	return "Lua 5.4 (Go implementation)"
}

// UpvalueID returns a unique identifier for an upvalue.
//
// This corresponds to lua_upvalueid in the C API.
//
// Parameters:
//   - funcIdx: Stack index of the function
//   - n: Upvalue index (1-based)
//
// Returns:
//   - string: Unique identifier
func (s *State) UpvalueID(funcIdx, n int) string {
	// For now, return a simple identifier
	return ""
}

// UpvalueJoin joins two upvalues.
//
// This corresponds to lua_upvaluejoin in the C API.
//
// Parameters:
//   - funcIdx1: Stack index of first function
//   - n1: First upvalue index
//   - funcIdx2: Stack index of second function
//   - n2: Second upvalue index
func (s *State) UpvalueJoin(funcIdx1, n1, funcIdx2, n2 int) {
	// For now, do nothing
}

// ToCFunction converts a stack value to a C function.
//
// This corresponds to lua_tocfunction in the C API.
//
// Parameters:
//   - index: Stack index
//
// Returns:
//   - Function: The function, or nil if not a function
func (s *State) ToCFunction(index int) Function {
	v := s.vm.GetStack(index)
	if !v.IsFunction() {
		return nil
	}

	fn, ok := v.ToFunction()
	if !ok || !fn.IsGo {
		return nil
	}

	// Return a wrapper function
	return func(L *State) int {
		// Call the Go function
		if err := fn.GoFn(L.vm); err != nil {
			panic(err)
		}
		return 0
	}
}

// SetFuncs registers multiple functions.
//
// This corresponds to luaL_setfuncs in the C API.
//
// Parameters:
//   - funcs: Map of function names to functions
//   - nup: Number of upvalues
func (s *State) SetFuncs(funcs map[string]Function, nup int) {
	for name, fn := range funcs {
		// Push upvalues if any
		for i := 0; i < nup; i++ {
			s.PushNil()
		}

		// Push function
		s.PushFunction(fn)

		// Set as global
		s.SetGlobal(name)
	}
}

// RequireF requires and loads a module.
//
// This corresponds to luaL_requiref in the C API.
//
// Parameters:
//   - modName: Module name
//   - openf: Function to open the module
//   - glb: Whether to set as global
//
// Returns:
//   - error: Any error that occurred
func (s *State) RequireF(modName string, openf Function, glb bool) error {
	// Get the module loader
	s.GetGlobal("require")
	if s.IsNil(-1) {
		return RuntimeError("require function not found")
	}

	// Push module name
	s.PushString(modName)

	// Call require
	if err := s.PCall(1, 1, 0); err != nil {
		return err
	}

	// Set as global if requested
	if glb {
		s.Copy(-1, -2) // Copy result
		s.SetGlobal(modName)
	}

	return nil
}