package lua

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
