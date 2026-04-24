package lua

import "fmt"

// ---------------------------------------------------------------------------
// Convenience APIs — reduce boilerplate for common table operations
// ---------------------------------------------------------------------------

// GetFieldString reads t[key] as a string where t is the value at idx.
// Returns "" if the field is nil or not convertible to a string.
// This is equivalent to: L.GetField(idx, key); s, _ := L.ToString(-1); L.Pop(1)
func (L *State) GetFieldString(idx int, key string) string {
	L.GetField(idx, key)
	defer L.Pop(1)
	s, _ := L.ToString(-1)
	return s
}

// GetFieldInt reads t[key] as an int64 where t is the value at idx.
// Returns 0 if the field is nil or not convertible to an integer.
func (L *State) GetFieldInt(idx int, key string) int64 {
	L.GetField(idx, key)
	defer L.Pop(1)
	n, _ := L.ToInteger(-1)
	return n
}

// GetFieldNumber reads t[key] as a float64 where t is the value at idx.
// Returns 0 if the field is nil or not convertible to a number.
func (L *State) GetFieldNumber(idx int, key string) float64 {
	L.GetField(idx, key)
	defer L.Pop(1)
	n, _ := L.ToNumber(-1)
	return n
}

// GetFieldBool reads t[key] as a bool where t is the value at idx.
// Returns false if the field is nil or false.
func (L *State) GetFieldBool(idx int, key string) bool {
	L.GetField(idx, key)
	defer L.Pop(1)
	return L.ToBoolean(-1)
}

// GetFieldAny reads t[key] and converts it to a Go value using ToAny.
// Returns nil if the field is nil.
func (L *State) GetFieldAny(idx int, key string) any {
	L.GetField(idx, key)
	defer L.Pop(1)
	return L.ToAny(-1)
}

// SetFields sets multiple fields on the table at idx.
// Values are pushed using PushAny (supports all Go types).
// Equivalent to calling PushAny(v); SetField(idx, k) for each entry.
func (L *State) SetFields(idx int, fields map[string]any) {
	absIdx := L.AbsIndex(idx)
	for k, v := range fields {
		L.PushAny(v)
		L.SetField(absIdx, k)
	}
}

// NewTableFrom creates a new table, fills it with the given fields, and
// leaves it on top of the stack. Values are pushed using PushAny.
func (L *State) NewTableFrom(fields map[string]any) {
	L.CreateTable(0, len(fields))
	for k, v := range fields {
		L.PushAny(v)
		L.SetField(-2, k)
	}
}

// GetFieldRef reads t[key] and if it's a function, stores it in the Lua
// registry and returns the reference ID. Returns RefNil if the field is
// not a function. The caller is responsible for calling
// L.Unref(RegistryIndex, ref) when the reference is no longer needed.
func (L *State) GetFieldRef(idx int, key string) int {
	L.GetField(idx, key)
	if L.Type(-1) != TypeFunction {
		L.Pop(1)
		return RefNil
	}
	return L.Ref(RegistryIndex) // pops the function, stores in registry
}

// ---------------------------------------------------------------------------
// CallSafe — PCall with Go error return
// ---------------------------------------------------------------------------

// CallSafe calls a function in protected mode and returns a Go error on failure.
// The function and nArgs arguments must already be on the stack (same as PCall).
// On success, nResults results are on the stack and err is nil.
// On failure, the error message is popped from the stack and returned as a Go error.
//
// Example:
//
//	L.PushFunction(fn)
//	L.PushAny(arg)
//	err := L.CallSafe(1, 1)
//	if err != nil { log.Fatal(err) }
func (L *State) CallSafe(nArgs, nResults int) error {
	status := L.PCall(nArgs, nResults, 0)
	if status != OK {
		msg, _ := L.ToString(-1)
		L.Pop(1)
		if msg == "" {
			msg = "unknown error (status " + statusString(status) + ")"
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// statusString returns a human-readable label for a Lua status code.
func statusString(status int) string {
	switch status {
	case ErrRun:
		return "runtime"
	case ErrMem:
		return "memory"
	case ErrErr:
		return "error handler"
	case ErrSyntax:
		return "syntax"
	default:
		return "unknown"
	}
}

// ---------------------------------------------------------------------------
// ToMap — safe table to map conversion
// ---------------------------------------------------------------------------

// ToMap reads the value at idx as a map[string]any.
// Returns (map, true) if the value is a table with string keys,
// (nil, false) otherwise (including pure-array tables or non-table values).
// This is a typed convenience wrapper around ToAny for the common case.
func (L *State) ToMap(idx int) (map[string]any, bool) {
	if !L.IsTable(idx) {
		return nil, false
	}
	result := L.ToAny(idx)
	if m, ok := result.(map[string]any); ok {
		return m, true
	}
	// ToAny returns []any for pure-array tables.
	return nil, false
}

// ---------------------------------------------------------------------------
// CallRef — call a function from registry reference
// ---------------------------------------------------------------------------

// CallRef retrieves a function from the registry by reference ID and calls it
// in protected mode. nArgs arguments must already be on the stack.
// Returns nil on success (nResults on stack), or a Go error.
// Returns an error if the ref does not point to a function.
//
// Example:
//
//	L.PushAny(eventData) // push 1 arg
//	err := L.CallRef(refID, 1, 0)
func (L *State) CallRef(ref int, nArgs, nResults int) error {
	L.RawGetI(RegistryIndex, int64(ref))
	if L.Type(-1) != TypeFunction {
		L.Pop(1)
		// Also pop the nArgs that are already on stack.
		if nArgs > 0 {
			L.Pop(nArgs)
		}
		return fmt.Errorf("registry ref %d is not a function", ref)
	}
	// Move function below the arguments.
	// Stack: [arg1, arg2, ..., argN, func] → [func, arg1, ..., argN]
	if nArgs > 0 {
		L.Insert(-(nArgs + 1))
	}
	return L.CallSafe(nArgs, nResults)
}

// ---------------------------------------------------------------------------
// ForEach — safe table iteration
// ---------------------------------------------------------------------------

// ForEach iterates over all key-value pairs in the table at idx.
// For each pair, the callback receives the State with key at -2 and value at -1.
// The callback must NOT pop the key or value — ForEach handles cleanup.
// If the callback returns false, iteration stops early.
//
// Example:
//
//	L.ForEach(-1, func(L *lua.State) bool {
//	    k, _ := L.ToString(-2)
//	    v := L.ToAny(-1)
//	    fmt.Println(k, v)
//	    return true // continue
//	})
func (L *State) ForEach(idx int, fn func(*State) bool) {
	absIdx := L.AbsIndex(idx)
	L.PushNil()
	for L.Next(absIdx) {
		// stack: [... key, value]
		if !fn(L) {
			L.Pop(2) // pop key + value
			return
		}
		L.Pop(1) // pop value, keep key for next iteration
	}
}
