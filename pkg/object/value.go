// Package object implements Lua's object system.
//
// This package provides the core data structures for representing Lua values,
// including the TValue type system, garbage collection interfaces, and function
// prototypes.
//
// # TValue Type System
//
// TValue is the fundamental value representation in Lua. It consists of a type
// tag and a value union. In Go, we use a struct with specific fields instead of
// a C-style union for type safety.
//
// # Supported Types
//
// The package supports all Lua types:
//   - TypeNil: nil value
//   - TypeBoolean: boolean values (true/false)
//   - TypeNumber: numeric values (float64)
//   - TypeString: string values
//   - TypeTable: table values
//   - TypeFunction: function values (Lua and Go closures)
//   - TypeUserData: user data values
//   - TypeThread: coroutine/thread values
//   - TypeProto: function prototypes
//   - TypeUpValue: upvalue references
//
// # Example Usage
//
//	// Create a TValue
//	v := NewNumber(42.0)
//
//	// Type checking
//	if v.IsNumber() {
//	    n, ok := v.ToNumber()
//	    fmt.Println(n) // 42.0
//	}
//
//	// Type conversion
//	v2 := NewString("hello")
//	s, ok := v2.ToString()
package object

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Type represents Lua type tags.
//
// These constants correspond to Lua's internal type system.
// The type tag is stored in TValue.Type field.
type Type uint8

const (
	// TypeNil represents the nil type.
	TypeNil Type = iota

	// TypeBoolean represents boolean type (true/false).
	TypeBoolean

	// TypeLightUserData represents light userdata (pointer-like values).
	TypeLightUserData

	// TypeNumber represents numeric values (integers and floats).
	TypeNumber

	// TypeString represents string values.
	TypeString

	// TypeTable represents table values (Lua's primary data structure).
	TypeTable

	// TypeFunction represents function values (closures and C functions).
	TypeFunction

	// TypeUserData represents full userdata values.
	TypeUserData

	// TypeThread represents thread/coroutine values.
	TypeThread

	// TypeProto represents function prototypes (pre-compiled functions).
	TypeProto

	// TypeUpValue represents upvalue references.
	TypeUpValue
)

// String returns a human-readable name for the type.
func (t Type) String() string {
	switch t {
	case TypeNil:
		return "nil"
	case TypeBoolean:
		return "boolean"
	case TypeLightUserData:
		return "lightuserdata"
	case TypeNumber:
		return "number"
	case TypeString:
		return "string"
	case TypeTable:
		return "table"
	case TypeFunction:
		return "function"
	case TypeUserData:
		return "userdata"
	case TypeThread:
		return "thread"
	case TypeProto:
		return "proto"
	case TypeUpValue:
		return "upvalue"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// Value holds the actual data of a Lua value.
//
// This struct serves as a union-like container for different Lua value types.
// Only the field corresponding to the current type is valid at any time.
//
// Fields:
//   - Num: Used for TypeNumber (float64 representation)
//   - Bool: Used for TypeBoolean
//   - Str: Used for TypeString (interned strings)
//   - Int: Used for integer numbers (optimized storage)
//   - Ptr: Used for TypeLightUserData (unsafe pointer)
//   - GC: Used for collectable objects (tables, functions, etc.)
type Value struct {
	Num   float64     // For TypeNumber
	Bool  bool        // For TypeBoolean
	Str   string      // For TypeString (interned)
	Int   int64       // For integer numbers
	Ptr   interface{} // For light userdata
	GC    GCObject    // For collectable objects
}

// TValue represents a tagged Lua value.
//
// TValue is the fundamental value representation in the Lua VM.
// Every value on the stack, in tables, or in registers is a TValue.
//
// The Type field indicates what kind of value is stored,
// and the Value field contains the actual data.
//
// Example:
//
//	var v TValue
//	v.SetNumber(42.0)
//	if v.IsNumber() {
//	    n, _ := v.ToNumber()
//	    fmt.Println(n) // 42.0
//	}
type TValue struct {
	Value Value
	Type  Type
	IsInt bool
}

// NewNil creates a new TValue with nil value.
func NewNil() *TValue {
	return &TValue{Type: TypeNil}
}

// NewBoolean creates a new TValue with boolean value.
func NewBoolean(b bool) *TValue {
	return &TValue{
		Type:  TypeBoolean,
		Value: Value{Bool: b},
	}
}

// NewNumber creates a new TValue with number value.
func NewNumber(n float64) *TValue {
	return &TValue{
		Type:  TypeNumber,
		Value: Value{Num: n},
		IsInt: false,
	}
}

// NewInteger creates a new TValue with integer value.
func NewInteger(i int64) *TValue {
	return &TValue{
		Type:  TypeNumber,
		Value: Value{Int: i, Num: float64(i)},
		IsInt: true,
	}
}

// NewString creates a new TValue with string value.
func NewString(s string) *TValue {
	return &TValue{
		Type:  TypeString,
		Value: Value{Str: s},
	}
}

// NewTableValue creates a new TValue with table value.
func NewTableValue(t *Table) *TValue {
	return &TValue{
		Type:  TypeTable,
		Value: Value{GC: t},
	}
}

// NewFunction creates a new TValue with function value.
func NewFunction(fn *Closure) *TValue {
	return &TValue{
		Type:  TypeFunction,
		Value: Value{GC: fn},
	}
}

// NewUserData creates a new TValue with userdata value.
func NewUserData(ud *UserData) *TValue {
	return &TValue{
		Type:  TypeUserData,
		Value: Value{GC: ud},
	}
}

// NewThread creates a new TValue with thread value.
func NewThread(th *Thread) *TValue {
	return &TValue{
		Type:  TypeThread,
		Value: Value{GC: th},
	}
}

// NewLightUserData creates a new TValue with light userdata value.
func NewLightUserData(ptr interface{}) *TValue {
	return &TValue{
		Type:  TypeLightUserData,
		Value: Value{Ptr: ptr},
	}
}

// IsNil returns true if the value is nil.
func (v *TValue) IsNil() bool {
	return v.Type == TypeNil
}

// IsBoolean returns true if the value is a boolean.
func (v *TValue) IsBoolean() bool {
	return v.Type == TypeBoolean
}

// IsNumber returns true if the value is a number.
func (v *TValue) IsNumber() bool {
	return v.Type == TypeNumber
}

// IsString returns true if the value is a string.
func (v *TValue) IsString() bool {
	return v.Type == TypeString
}

// IsTable returns true if the value is a table.
func (v *TValue) IsTable() bool {
	return v.Type == TypeTable
}

// IsFunction returns true if the value is a function.
func (v *TValue) IsFunction() bool {
	return v.Type == TypeFunction
}

// IsUserData returns true if the value is a userdata.
func (v *TValue) IsUserData() bool {
	return v.Type == TypeUserData
}

// IsThread returns true if the value is a thread.
func (v *TValue) IsThread() bool {
	return v.Type == TypeThread
}

// IsLightUserData returns true if the value is a light userdata.
func (v *TValue) IsLightUserData() bool {
	return v.Type == TypeLightUserData
}

// IsCollectable returns true if the value is a collectable object (GC object).
func (v *TValue) IsCollectable() bool {
	switch v.Type {
	case TypeString, TypeTable, TypeFunction, TypeUserData, TypeThread, TypeProto, TypeUpValue:
		return true
	default:
		return false
	}
}

// ToNumber converts the value to a number.
//
// Returns the number value and true if successful,
// or 0 and false if the value cannot be converted to a number.
//
// ToNumber converts the value to a number.
//
// Returns the numeric value and true if successful,
// or 0 and false if the value cannot be converted.
//
// For TypeNumber, returns the stored number.
// For TypeString, attempts to parse the string as a number
// with whitespace trimming and hex prefix support.
func (v *TValue) ToNumber() (float64, bool) {
	if v.Type == TypeNumber {
		return v.Value.Num, true
	}
	if v.Type == TypeString {
		num, _, ok := LuaStringToNumber(v.Value.Str, 0)
		return num, ok
	}
	return 0, false
}

// LuaStringToNumber converts a Lua string to a number.
// It handles whitespace trimming, hex prefixes (0x/0X), and optional base.
// When base is 0, it auto-detects: 0x/0X prefix for hex, otherwise decimal.
// When base is provided (2-36), it parses the string in that base.
// Returns (number, isInteger, success).
func LuaStringToNumber(s string, base int) (num float64, isInt bool, ok bool) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0, false, false
	}

	// Handle explicit base
	if base >= 2 && base <= 36 {
		val, err := strconv.ParseInt(s, base, 64)
		if err != nil {
			return 0, false, false
		}
		return float64(val), true, true
	}

	// Auto-detect: check for hex prefix (handle negative sign)
	neg := false
	hexStr := s
	if len(s) > 0 && s[0] == '-' {
		neg = true
		hexStr = s[1:]
	}
	if len(hexStr) >= 2 && (hexStr[0] == '0' && (hexStr[1] == 'x' || hexStr[1] == 'X')) {
		val, err := strconv.ParseInt(hexStr[2:], 16, 64)
		if err != nil {
			return 0, false, false
		}
		if neg {
			val = -val
		}
		return float64(val), true, true
	}

	// Try parsing as float
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, false
	}

	// Check if it's an integer value
	// Scientific notation (e/E) or decimal point means it's a float
	hasFloatSyntax := strings.ContainsAny(s, "eE.")
	if hasFloatSyntax {
		return val, false, true
	}
	if math.Trunc(val) == val && !math.IsInf(val, 0) && math.Abs(val) <= float64(1<<53) {
		return val, true, true
	}
	return val, false, true
}

// ToString converts the value to a string.
//
// Returns the string value and true if successful,
// or empty string and false if the value is not a string.
//
// Currently, only TypeString values can be converted.
// Future versions may support number-to-string conversion.
func (v *TValue) ToString() (string, bool) {
	if v.Type == TypeString {
		return v.Value.Str, true
	}
	return "", false
}

// ToBoolean converts the value to a boolean.
//
// Returns the boolean value and true if the value is a boolean,
// or false and false otherwise.
//
// Note: In Lua, only nil and false are "falsy" values.
// This method only returns the actual boolean value, not Lua's truthiness.
func (v *TValue) ToBoolean() (bool, bool) {
	if v.Type == TypeBoolean {
		return v.Value.Bool, true
	}
	return false, false
}

// ToTable converts the value to a table.
//
// Returns the table pointer and true if the value is a table,
// or nil and false otherwise.
func (v *TValue) ToTable() (*Table, bool) {
	if v.Type == TypeTable && v.Value.GC != nil {
		t, ok := v.Value.GC.(*Table)
		return t, ok
	}
	return nil, false
}

// ToFunction converts the value to a function.
//
// Returns the closure pointer and true if the value is a function,
// or nil and false otherwise.
func (v *TValue) ToFunction() (*Closure, bool) {
	if v.Type == TypeFunction && v.Value.GC != nil {
		fn, ok := v.Value.GC.(*Closure)
		return fn, ok
	}
	return nil, false
}

// ToFunctionProto returns the prototype of a function.
//
// Returns the prototype pointer if the value is a Lua function,
// or nil otherwise.
func (v *TValue) ToFunctionProto() *Prototype {
	if v.Type == TypeFunction && v.Value.GC != nil {
		fn, ok := v.Value.GC.(*Closure)
		if ok && !fn.IsGo {
			return fn.Proto
		}
	}
	return nil
}

// ToUserData converts the value to a userdata.
//
// Returns the userdata pointer and true if the value is a userdata,
// or nil and false otherwise.
func (v *TValue) ToUserData() (*UserData, bool) {
	if v.Type == TypeUserData && v.Value.GC != nil {
		ud, ok := v.Value.GC.(*UserData)
		return ud, ok
	}
	return nil, false
}

// ToThread converts the value to a thread.
//
// Returns the thread pointer and true if the value is a thread,
// or nil and false otherwise.
func (v *TValue) ToThread() (*Thread, bool) {
	if v.Type == TypeThread && v.Value.GC != nil {
		th, ok := v.Value.GC.(*Thread)
		return th, ok
	}
	return nil, false
}

// ToProto converts the value to a prototype.
//
// Returns the prototype pointer and true if the value is a prototype,
// or nil and false otherwise.
func (v *TValue) ToProto() (*Prototype, bool) {
	if v.Type == TypeProto && v.Value.GC != nil {
		p, ok := v.Value.GC.(*Prototype)
		return p, ok
	}
	return nil, false
}

// ToLightUserData converts the value to a light userdata pointer.
//
// Returns the pointer and true if the value is a light userdata,
// or nil and false otherwise.
func (v *TValue) ToLightUserData() (interface{}, bool) {
	if v.Type == TypeLightUserData {
		return v.Value.Ptr, true
	}
	return nil, false
}

// SetNil sets the value to nil.
func (v *TValue) SetNil() {
	v.Type = TypeNil
	v.Value = Value{}
}

// SetBoolean sets the value to a boolean.
func (v *TValue) SetBoolean(b bool) {
	v.Type = TypeBoolean
	v.Value = Value{Bool: b}
}

// SetNumber sets the value to a number.
func (v *TValue) SetNumber(n float64) {
	v.Type = TypeNumber
	v.Value = Value{Num: n}
	v.IsInt = false
}

// SetInteger sets the value to an integer number.
func (v *TValue) SetInteger(i int64) {
	v.Type = TypeNumber
	v.Value = Value{Int: i, Num: float64(i)}
	v.IsInt = true
}

// SetString sets the value to a string.
func (v *TValue) SetString(s string) {
	v.Type = TypeString
	v.Value = Value{Str: s}
}

// SetTable sets the value to a table.
func (v *TValue) SetTable(t *Table) {
	v.Type = TypeTable
	v.Value = Value{GC: t}
}

// SetFunction sets the value to a function.
func (v *TValue) SetFunction(fn *Closure) {
	v.Type = TypeFunction
	v.Value = Value{GC: fn}
}

// SetUserData sets the value to a userdata.
func (v *TValue) SetUserData(ud *UserData) {
	v.Type = TypeUserData
	v.Value = Value{GC: ud}
}

// SetThread sets the value to a thread.
func (v *TValue) SetThread(th *Thread) {
	v.Type = TypeThread
	v.Value = Value{GC: th}
}

// SetLightUserData sets the value to a light userdata.
func (v *TValue) SetLightUserData(ptr interface{}) {
	v.Type = TypeLightUserData
	v.Value = Value{Ptr: ptr}
}

// SetProto sets the value to a prototype.
func (v *TValue) SetProto(p *Prototype) {
	v.Type = TypeProto
	v.Value = Value{GC: p}
}

// CopyFrom copies the value from another TValue.
func (v *TValue) CopyFrom(src *TValue) {
	v.Type = src.Type
	v.Value = src.Value
	v.IsInt = src.IsInt
}

// Clear clears the value to nil.
func (v *TValue) Clear() {
	v.SetNil()
}

// GCObject interface for collectable objects.
//
// All collectable objects in Lua must implement this interface.
// The gcObject method is a marker method used internally by the GC.
//
// Implementing types include:
//   - Table
//   - Closure (functions)
//   - UserData
//   - Thread
//   - Prototype
//   - Upvalue
//   - GCString
type GCObject interface {
	gcObject()
}

// Table represents a Lua table.
//
// Tables are the primary data structure in Lua, used for arrays,
// dictionaries, objects, and modules.
//
// Implementation uses a hybrid array-map structure for performance:
//   - Array part for integer keys 1..N
//   - Map part for other keys
type Table struct {
	// Array part for integer keys 1..N
	Array []TValue

	// Map part for other keys
	Map map[Value]*TValue

	// Metatable for operator overloading
	Metatable *Table

	// Flags
	IsArray bool // True if only has array part
	Length  int  // Cached length
}

// gcObject marks Table as a GC object.
func (t *Table) gcObject() {}

// NewTable creates a new empty table.
func NewTable() *Table {
	return &Table{
		Array: make([]TValue, 0),
		Map:   make(map[Value]*TValue),
		IsArray: true,
	}
}

// Closure represents a Lua function closure.
//
// A closure consists of a prototype and its upvalues.
// There are two types of closures:
//   - Lua closures (LClosure): functions defined in Lua
//   - Go closures (GoClosure): functions defined in Go
type Closure struct {
	// IsGo indicates if this is a Go function
	IsGo bool

	// Prototype for Lua functions
	Proto *Prototype

	// Go function
	GoFn GoFunction

	// Upvalues
	Upvalues []*Upvalue
}

// gcObject marks Closure as a GC object.
func (c *Closure) gcObject() {}

// GoFunction is the type for Go functions callable from Lua.
//
// The function receives the VM state and returns an error.
// Results should be pushed onto the stack.
type GoFunction func(vm interface{}) error

// UserData represents a Lua userdata object.
//
// Userdata is a mechanism for storing arbitrary Go data in Lua.
// Full userdata has its own metatable and can have a __gc metamethod.
type UserData struct {
	Value     interface{} // The actual Go value
	Metatable *Table      // Optional metatable
}

// gcObject marks UserData as a GC object.
func (u *UserData) gcObject() {}

// Thread represents a Lua thread (coroutine).
//
// Threads in Lua are coroutines that can be suspended and resumed.
// Each thread has its own execution stack and state.
type Thread struct {
	// VM reference (simplified for now)
	Stack []TValue
	Top   int
}

// gcObject marks Thread as a GC object.
func (t *Thread) gcObject() {}

// Prototype represents a function prototype (pre-compiled function).
//
// A prototype contains all the information needed to create a closure:
//   - Bytecode instructions
//   - Constant table
//   - Upvalue information
//   - Debug information
//
// This corresponds to Proto in Lua's C implementation.
type Prototype struct {
	// Bytecode instructions
	Code []Instruction

	// Constant table (numbers, strings, etc.)
	Constants []TValue

	// Upvalue information
	Upvalues []UpvalueDesc

	// Nested prototypes (for closures)
	Prototypes []*Prototype

	// Source information
	Source     string // Source file name
	LineInfo   []int  // Line number for each instruction
	LocVars    []LocVar

	// Function properties
	NumParams    int  // Number of fixed parameters
	IsVarArg     bool // Is vararg function
	MaxStackSize int  // Max stack size needed
}

// gcObject marks Prototype as a GC object.
func (p *Prototype) gcObject() {}

// Instruction represents a bytecode instruction.
//
// Lua uses 32-bit instructions with different formats:
//   - iABC: Three operands (A, B, C)
//   - iABx: Two operands (A, Bx)
//   - iAsBx: Two operands (A, sBx where sBx is signed)
//   - iAx: One operand (Ax)
type Instruction uint32

// UpvalueDesc describes an upvalue in a prototype.
type UpvalueDesc struct {
	Index    int  // Stack index or parent upvalue index
	IsLocal  bool // True if upvalue is from local variable
}

// LocVar represents a local variable debug info.
type LocVar struct {
	Name     string // Variable name
	Start    int    // Start PC
	End      int    // End PC
	RegIndex int    // Register index for stack access
}

// Upvalue represents an open upvalue.
//
// An upvalue is a reference to a local variable from an enclosing function.
// When the variable goes out of scope, the upvalue "closes" and stores
// the value internally.
//
// This corresponds to UpVal in Lua's C implementation.
type Upvalue struct {
	// Index in the stack
	Index int

	// Pointer to the value (when open)
	Value *TValue

	// Cached value (when closed)
	Cached TValue

	// Whether the upvalue is closed
	Closed bool
}

// gcObject marks Upvalue as a GC object.
func (u *Upvalue) gcObject() {}

// NewUpvalue creates a new upvalue.
func NewUpvalue(index int, value *TValue) *Upvalue {
	return &Upvalue{
		Index: index,
		Value: value,
	}
}

// Get returns the upvalue's current value.
func (u *Upvalue) Get() *TValue {
	if u.Closed {
		return &u.Cached
	}
	return u.Value
}

// Set sets the upvalue's value.
func (u *Upvalue) Set(v *TValue) {
	if u.Closed {
		u.Cached.CopyFrom(v)
	} else {
		u.Value.CopyFrom(v)
	}
}

// Close closes the upvalue, caching its current value.
func (u *Upvalue) Close() {
	if !u.Closed {
		u.Cached.CopyFrom(u.Value)
		u.Closed = true
		u.Value = nil
	}
}

// GCString represents an interned string.
//
// Strings in Lua are interned to save memory and enable fast comparison.
// Each unique string value has exactly one GCString instance.
type GCString struct {
	Value string
	Hash  uint32
}

// gcObject marks GCString as a GC object.
func (s *GCString) gcObject() {}

// NewGCString creates a new GCString.
func NewGCString(value string) *GCString {
	return &GCString{
		Value: value,
		Hash:  hashString(value),
	}
}

// hashString computes a hash for a string.
func hashString(s string) uint32 {
	var h uint32
	for i := 0; i < len(s); i++ {
		h = h*31 + uint32(s[i])
	}
	return h
}

// TypeName returns the type name for a TValue.
func TypeName(v *TValue) string {
	return v.Type.String()
}

// IsFalse returns true if the value is "falsy" in Lua terms.
//
// In Lua, only nil and false are falsy. All other values are truthy,
// including 0, empty strings, and empty tables.
func IsFalse(v *TValue) bool {
	return v.IsNil() || (v.IsBoolean() && !v.Value.Bool)
}

// Equal compares two TValue values for equality.
//
// This performs a shallow comparison. For tables and other complex types,
// it compares by reference, not by content.
func Equal(a, b *TValue) bool {
	if a.Type != b.Type {
		return false
	}

	switch a.Type {
	case TypeNil:
		return true
	case TypeBoolean:
		return a.Value.Bool == b.Value.Bool
	case TypeNumber:
		return a.Value.Num == b.Value.Num
	case TypeString:
		return a.Value.Str == b.Value.Str
	case TypeLightUserData:
		return a.Value.Ptr == b.Value.Ptr
	default:
		// For collectable types, compare by reference
		return a.Value.GC == b.Value.GC
	}
}

// ToStringRaw converts any TValue to a string representation.
//
// This is different from ToString() which only works for string types.
// This method converts numbers, booleans, etc. to their string representation.
func ToStringRaw(v *TValue) string {
	switch v.Type {
	case TypeNil:
		return "nil"
	case TypeBoolean:
		if v.Value.Bool {
			return "true"
		}
		return "false"
	case TypeNumber:
		// Check if it's an integer
		// Check IsInt flag first for exact integer representation
		if v.IsInt {
			return strconv.FormatInt(v.Value.Int, 10)
		}
		if v.Value.Num == float64(int64(v.Value.Num)) {
			return strconv.FormatInt(int64(v.Value.Num), 10)
		}
		return strconv.FormatFloat(v.Value.Num, 'g', -1, 64)
	case TypeString:
		return v.Value.Str
	case TypeTable:
		return fmt.Sprintf("table: %p", v.Value.GC)
	case TypeFunction:
		return fmt.Sprintf("function: %p", v.Value.GC)
	case TypeUserData:
		return fmt.Sprintf("userdata: %p", v.Value.GC)
	case TypeThread:
		return fmt.Sprintf("thread: %p", v.Value.GC)
	default:
		return fmt.Sprintf("%s: %p", v.Type.String(), v.Value.GC)
	}
}

// Table Operations
// These methods provide table get/set operations for object.Table

// Get retrieves a value from the table by key.
func (t *Table) Get(key TValue) *TValue {
	// Check if key is an integer suitable for array access
	if key.Type == TypeNumber {
		num := key.Value.Num
		intVal := int64(num)

		// Check if the number is actually an integer
		if float64(intVal) == num {
			// Check if it's in the valid array range (1-indexed)
			if intVal >= 1 && int(intVal) <= len(t.Array) {
				// Array access (1-indexed, so subtract 1 for 0-indexed slice)
				return &t.Array[intVal-1]
			}
		}
	}

	// Map access for all other cases
	if t.Map == nil {
		return nil
	}
	if val, ok := t.Map[key.Value]; ok {
		return val
	}

	// Key not found
	return nil
}

// GetI retrieves a value from the table by integer index.
func (t *Table) GetI(idx int) *TValue {
	if idx >= 1 && idx <= len(t.Array) {
		return &t.Array[idx-1]
	}
	return nil
}

// Set sets a value in the table for the given key.
func (t *Table) Set(key, value TValue) {
	// Check if key is an integer suitable for array access
	if key.Type == TypeNumber {
		num := key.Value.Num
		intVal := int64(num)

		// Check if the number is actually an integer
		if float64(intVal) == num && intVal >= 1 {
			idx := int(intVal)

			// Extend array if necessary
			if idx > len(t.Array) {
				// Grow array to accommodate the new index
				newArray := make([]TValue, idx)
				copy(newArray, t.Array)
				t.Array = newArray
			}

			// Set value in array (1-indexed, so subtract 1)
			t.Array[idx-1] = value
			t.IsArray = len(t.Map) == 0
			return
		}
	}

	// Map access for all other cases
	if t.Map == nil {
		t.Map = make(map[Value]*TValue)
	}

	// Create a copy of the value to store
	valCopy := value
	t.Map[key.Value] = &valCopy
	t.IsArray = false
}

// SetI sets a value in the table at the given integer index.
func (t *Table) SetI(idx int, value TValue) {
	if idx < 1 {
		// Invalid index, store in map instead
		key := NewInteger(int64(idx))
		t.Set(*key, value)
		return
	}

	// Extend array if necessary
	if idx > len(t.Array) {
		newArray := make([]TValue, idx)
		copy(newArray, t.Array)
		t.Array = newArray
	}

	t.Array[idx-1] = value
	t.IsArray = len(t.Map) == 0
}

// Len returns the length of the table (# operator semantics).
func (t *Table) Len() int {
	// Find the actual length by checking for nil values
	length := 0
	for i := len(t.Array); i > 0; i-- {
		if t.Array[i-1].Type != TypeNil {
			length = i
			break
		}
	}
	t.Length = length
	return length
}

// GetMetatable returns the metatable of the table.
func (t *Table) GetMetatable() *Table {
	return t.Metatable
}

// SetMetatable sets the metatable of the table.
func (t *Table) SetMetatable(mt *Table) {
	t.Metatable = mt
}

// Next returns the next key-value pair in the table.
//
// This is used for iteration (pairs/ipairs).
// Pass nil to get the first pair.
// Returns nil, nil when iteration is complete.
//
// Parameters:
//   - key: The current key (nil to start)
//
// Returns:
//   - *TValue: The next key
//   - *TValue: The next value
func (t *Table) Next(key *TValue) (*TValue, *TValue) {
	// Simple implementation: iterate over Map only
	// For a full implementation, would also iterate over Array

	if t.Map == nil || len(t.Map) == 0 {
		return nil, nil
	}

	// Collect and sort map keys for deterministic iteration order
	keys := make([]string, 0, len(t.Map))
	for k := range t.Map {
		keys = append(keys, k.Str)
	}
	sort.Strings(keys)

	if key == nil || key.Type == TypeNil {
		// Return first entry (sorted order)
		k := keys[0]
		v := t.Map[Value{Str: k}]
		return &TValue{Type: TypeString, Value: Value{Str: k}}, v
	}

	// Find the entry after the given key in sorted order
	for i, k := range keys {
		if k == key.Value.Str {
			if i+1 < len(keys) {
				nextK := keys[i+1]
				v := t.Map[Value{Str: nextK}]
				return &TValue{Type: TypeString, Value: Value{Str: nextK}}, v
			}
			return nil, nil // was the last key
		}
	}

	return nil, nil
}

// NewTableWithSize creates a new table with optional pre-allocation.
func NewTableWithSize(arraySize, mapSize int) *Table {
	return &Table{
		Array:   make([]TValue, 0, arraySize),
		Map:     make(map[Value]*TValue, mapSize),
		IsArray: mapSize == 0,
		Length:  0,
	}
}

// Instruction methods

// Opcode returns the opcode of the instruction.
func (i Instruction) Opcode() uint8 {
	return uint8(i & 0x7F)
}

// A returns the A field of the instruction.
func (i Instruction) A() int {
	return int((i >> 7) & 0xFF)
}

// B returns the B field of the instruction (8 bits).
func (i Instruction) B() int {
	return int((i >> 24) & 0xFF)
}

// C returns the C field of the instruction (9 bits).
func (i Instruction) C() int {
	return int((i >> 15) & 0x1FF)
}

// Bx returns the Bx field of the instruction (17 bits).
func (i Instruction) Bx() int {
	return int((i >> 15) & 0x1FFFF)
}

// sBx returns the signed Bx field of the instruction (17 bits, bias 0xFFFF).
func (i Instruction) SBx() int {
	return int((i >> 15) & 0x1FFFF) - 0xFFFF
}

// Ax returns the Ax field of the instruction.
func (i Instruction) Ax() int {
	return int((i >> 7) & 0x1FFFFFF)
}
