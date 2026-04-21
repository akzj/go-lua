package lua

// ---------------------------------------------------------------------------
// Table operations
// ---------------------------------------------------------------------------

// NewTable pushes a new empty table onto the stack.
func (L *State) NewTable() {
	L.s.NewTable()
}

// CreateTable pushes a new table with pre-allocated space for nArr array
// elements and nRec hash elements.
func (L *State) CreateTable(nArr, nRec int) {
	L.s.CreateTable(nArr, nRec)
}

// GetTable pushes t[k] where t is the value at idx and k is the value
// at the top of the stack. Pops the key. Returns the type of the pushed value.
// May trigger __index metamethod.
func (L *State) GetTable(idx int) Type {
	return toPublicType(L.s.GetTable(idx))
}

// GetField pushes t[key] where t is the value at idx.
// Returns the type of the pushed value.
func (L *State) GetField(idx int, key string) Type {
	return toPublicType(L.s.GetField(idx, key))
}

// GetI pushes t[n] where t is the value at idx.
// Returns the type of the pushed value.
func (L *State) GetI(idx int, n int64) Type {
	return toPublicType(L.s.GetI(idx, n))
}

// GetGlobal pushes the value of the global variable name.
// Returns the type of the pushed value.
func (L *State) GetGlobal(name string) Type {
	return toPublicType(L.s.GetGlobal(name))
}

// SetTable does t[k] = v where t is at idx, k is at top-1, v is at top.
// Pops both the key and value. May trigger __newindex metamethod.
func (L *State) SetTable(idx int) {
	L.s.SetTable(idx)
}

// SetField does t[key] = v where t is at idx and v is at the top of the stack.
// Pops the value.
func (L *State) SetField(idx int, key string) {
	L.s.SetField(idx, key)
}

// SetI does t[n] = v where t is at idx and v is at the top of the stack.
// Pops the value.
func (L *State) SetI(idx int, n int64) {
	L.s.SetI(idx, n)
}

// SetGlobal pops a value and sets it as the global variable name.
func (L *State) SetGlobal(name string) {
	L.s.SetGlobal(name)
}

// RawGet pushes t[k] without invoking metamethods.
// t is at idx, k is at top. Pops the key.
func (L *State) RawGet(idx int) Type {
	return toPublicType(L.s.RawGet(idx))
}

// RawGetI pushes t[n] without invoking metamethods.
func (L *State) RawGetI(idx int, n int64) Type {
	return toPublicType(L.s.RawGetI(idx, n))
}

// RawSet does t[k] = v without invoking metamethods.
// t is at idx, k at top-1, v at top. Pops key and value.
func (L *State) RawSet(idx int) {
	L.s.RawSet(idx)
}

// RawSetI does t[n] = v without invoking metamethods.
// t is at idx, v at top. Pops the value.
func (L *State) RawSetI(idx int, n int64) {
	L.s.RawSetI(idx, n)
}

// GetMetatable pushes the metatable of the value at idx.
// Returns false if the value has no metatable (nothing pushed).
func (L *State) GetMetatable(idx int) bool {
	return L.s.GetMetatable(idx)
}

// SetMetatable pops a table from the stack and sets it as the metatable
// of the value at idx.
func (L *State) SetMetatable(idx int) {
	L.s.SetMetatable(idx)
}

// GetMetafield pushes the metamethod field from the metatable of the value
// at idx. Returns true if found, false if not (nothing pushed).
func (L *State) GetMetafield(idx int, field string) bool {
	return L.s.GetMetafield(idx, field)
}

// Next pops a key and pushes the next key–value pair from the table at idx.
// Returns false when the traversal is complete (nothing pushed).
func (L *State) Next(idx int) bool {
	return L.s.Next(idx)
}

// Len pushes the length of the value at idx onto the stack.
// May trigger __len metamethod.
func (L *State) Len(idx int) {
	L.s.Len(idx)
}

// RawEqual compares two values for equality without metamethods.
func (L *State) RawEqual(idx1, idx2 int) bool {
	return L.s.RawEqual(idx1, idx2)
}

// Compare compares two values using the given comparison operation.
// May trigger __eq, __lt, or __le metamethods.
func (L *State) Compare(idx1, idx2 int, op CompareOp) bool {
	return L.s.Compare(idx1, idx2, op)
}

// Concat concatenates the n values at the top of the stack, pops them,
// and pushes the result. May trigger __concat metamethod.
func (L *State) Concat(n int) {
	L.s.Concat(n)
}

// Arith performs an arithmetic or bitwise operation on the values at the
// top of the stack. For binary operations, pops two operands; for unary,
// pops one. Pushes the result.
func (L *State) Arith(op ArithOp) {
	L.s.Arith(op)
}

// GetSubTable ensures that t[fname] is a table, creating it if needed.
// t is at idx. Pushes the subtable. Returns true if it already existed.
func (L *State) GetSubTable(idx int, fname string) bool {
	return L.s.GetSubTable(idx, fname)
}
