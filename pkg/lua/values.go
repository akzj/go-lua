package lua

// ---------------------------------------------------------------------------
// Push functions (Go → Lua stack)
// ---------------------------------------------------------------------------

// PushNil pushes a nil value onto the stack.
func (L *State) PushNil() {
	L.s.PushNil()
}

// PushBoolean pushes a boolean value onto the stack.
func (L *State) PushBoolean(b bool) {
	L.s.PushBoolean(b)
}

// PushInteger pushes an integer value onto the stack.
func (L *State) PushInteger(n int64) {
	L.s.PushInteger(n)
}

// PushNumber pushes a floating-point number onto the stack.
func (L *State) PushNumber(n float64) {
	L.s.PushNumber(n)
}

// PushString pushes a string onto the stack and returns it.
func (L *State) PushString(s string) string {
	return L.s.PushString(s)
}

// PushFString pushes a formatted string onto the stack and returns it.
func (L *State) PushFString(format string, args ...interface{}) string {
	return L.s.PushFString(format, args...)
}

// PushFunction pushes a Go function as a light C function (no upvalues).
func (L *State) PushFunction(f Function) {
	L.s.PushCFunction(L.wrapFunction(f))
}

// PushClosure pushes a Go function as a closure with n upvalues.
// The upvalues must be on the stack before calling this function.
func (L *State) PushClosure(f Function, n int) {
	L.s.PushCClosure(L.wrapFunction(f), n)
}

// PushLightUserdata pushes a light userdata (Go value without metatable support).
func (L *State) PushLightUserdata(p interface{}) {
	L.s.PushLightUserdata(p)
}

// PushGlobalTable pushes the global table onto the stack.
func (L *State) PushGlobalTable() {
	L.s.PushGlobalTable()
}

// PushThread pushes the running thread onto its own stack.
// Returns true if the thread is the main thread.
func (L *State) PushThread() bool {
	return L.s.PushThread()
}

// PushFail pushes a "fail" value (nil in Lua 5.5).
func (L *State) PushFail() {
	L.s.PushFail()
}

// ---------------------------------------------------------------------------
// Type checking
// ---------------------------------------------------------------------------

// Type returns the type of the value at the given index.
func (L *State) Type(idx int) Type {
	return toPublicType(L.s.Type(idx))
}

// TypeName returns the name of the given type.
func (L *State) TypeName(tp Type) string {
	return L.s.TypeName(toInternalType(tp))
}

// IsNil returns true if the value at idx is nil.
func (L *State) IsNil(idx int) bool {
	return L.s.IsNil(idx)
}

// IsNone returns true if the index is not valid.
func (L *State) IsNone(idx int) bool {
	return L.s.IsNone(idx)
}

// IsNoneOrNil returns true if the index is not valid or the value is nil.
func (L *State) IsNoneOrNil(idx int) bool {
	return L.s.IsNoneOrNil(idx)
}

// IsBoolean returns true if the value at idx is a boolean.
func (L *State) IsBoolean(idx int) bool {
	return L.s.IsBoolean(idx)
}

// IsInteger returns true if the value at idx is an integer.
func (L *State) IsInteger(idx int) bool {
	return L.s.IsInteger(idx)
}

// IsNumber returns true if the value at idx is a number or a string
// convertible to a number.
func (L *State) IsNumber(idx int) bool {
	return L.s.IsNumber(idx)
}

// IsString returns true if the value at idx is a string or a number
// (numbers are always convertible to strings).
func (L *State) IsString(idx int) bool {
	return L.s.IsString(idx)
}

// IsFunction returns true if the value at idx is a function (Lua or Go).
func (L *State) IsFunction(idx int) bool {
	return L.s.IsFunction(idx)
}

// IsTable returns true if the value at idx is a table.
func (L *State) IsTable(idx int) bool {
	return L.s.IsTable(idx)
}

// IsCFunction returns true if the value at idx is a Go function.
func (L *State) IsCFunction(idx int) bool {
	return L.s.IsCFunction(idx)
}

// IsUserdata returns true if the value at idx is a userdata (full or light).
func (L *State) IsUserdata(idx int) bool {
	return L.s.IsUserdata(idx)
}

// ---------------------------------------------------------------------------
// Conversion functions (Lua stack → Go)
// ---------------------------------------------------------------------------

// ToBoolean converts the value at idx to a boolean.
// Returns false for nil and false, true for everything else.
func (L *State) ToBoolean(idx int) bool {
	return L.s.ToBoolean(idx)
}

// ToInteger converts the value at idx to an integer.
// Returns (value, true) on success, (0, false) if the value is not
// an integer or a float with an exact integer representation.
func (L *State) ToInteger(idx int) (int64, bool) {
	return L.s.ToInteger(idx)
}

// ToNumber converts the value at idx to a floating-point number.
// Returns (value, true) on success, (0, false) if not convertible.
func (L *State) ToNumber(idx int) (float64, bool) {
	return L.s.ToNumber(idx)
}

// ToString converts the value at idx to a string.
// Returns (value, true) for strings and numbers (coerced in place),
// ("", false) for other types.
func (L *State) ToString(idx int) (string, bool) {
	return L.s.ToString(idx)
}

// ToPointer returns a string representation of the pointer value at idx.
// Useful for debugging.
func (L *State) ToPointer(idx int) string {
	return L.s.ToPointer(idx)
}

// RawLen returns the raw length of the value at idx (no __len metamethod).
func (L *State) RawLen(idx int) int64 {
	return L.s.RawLen(idx)
}

// StringToNumber tries to convert a string to a number and pushes it.
// Returns the string length + 1 on success, 0 on failure.
func (L *State) StringToNumber(s string) int {
	return L.s.StringToNumber(s)
}
