// Bridge provides high-level type conversion between Go and Lua values.
//
// These functions eliminate the need for manual stack operations when
// converting between Go and Lua types. They are convenience wrappers
// over the low-level stack API.
//
// # PushAny — Go → Lua
//
// [State.PushAny] pushes any Go value onto the Lua stack, automatically
// selecting the appropriate Lua type. Common types (bool, int, float64,
// string) use fast paths without reflection.
//
// # ToAny — Lua → Go
//
// [State.ToAny] reads a Lua value from the stack and returns it as a Go
// value. Tables are converted to map[string]any or []any depending on
// their key structure.
//
// # ToStruct — Lua table → Go struct
//
// [State.ToStruct] reads a Lua table into a Go struct using field name
// mapping (via `lua` struct tags or lowercased field names).
//
// # RegisterModule — register a Go module for require()
//
// [RegisterModule] registers a set of Go functions so that Lua code can
// load them with require(name).
package lua

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// PushAny — Go value → Lua stack
// ---------------------------------------------------------------------------

// PushAny pushes any Go value onto the Lua stack, automatically selecting
// the appropriate Lua type:
//
//	Go type              → Lua type
//	nil                  → nil
//	bool                 → boolean
//	int, int8..int64     → integer
//	uint, uint8..uint64  → integer (or number if > math.MaxInt64)
//	float32, float64     → number
//	string               → string
//	[]byte               → string
//	[]T                  → table (array, 1-indexed)
//	map[string]T         → table (hash)
//	map[K]V              → table (hash, keys converted via fmt.Sprint)
//	Function             → function
//	struct               → table (exported fields, using `lua` tag or lowercase name)
//	*struct              → same as struct (dereferences pointer)
//	any other            → light userdata
//
// Nested values are handled recursively.
func (L *State) PushAny(value any) {
	if value == nil {
		L.PushNil()
		return
	}

	// Fast path: common types without reflection.
	switch v := value.(type) {
	case bool:
		L.PushBoolean(v)
	case int:
		L.PushInteger(int64(v))
	case int64:
		L.PushInteger(v)
	case int32:
		L.PushInteger(int64(v))
	case int16:
		L.PushInteger(int64(v))
	case int8:
		L.PushInteger(int64(v))
	case uint:
		pushUint64(L, uint64(v))
	case uint64:
		pushUint64(L, v)
	case uint32:
		L.PushInteger(int64(v))
	case uint16:
		L.PushInteger(int64(v))
	case uint8:
		L.PushInteger(int64(v))
	case float64:
		L.PushNumber(v)
	case float32:
		L.PushNumber(float64(v))
	case string:
		L.PushString(v)
	case []byte:
		L.PushString(string(v))
	case Function:
		L.PushFunction(v)
	case map[string]any:
		L.pushMapStringAny(v)
	case map[string]string:
		L.pushMapStringString(v)
	case []any:
		L.pushSliceAny(v)
	default:
		// Slow path: use reflection for structs, other maps, other slices.
		L.pushReflect(reflect.ValueOf(value))
	}
}

func pushUint64(L *State, v uint64) {
	if v <= math.MaxInt64 {
		L.PushInteger(int64(v))
	} else {
		L.PushNumber(float64(v))
	}
}

func (L *State) pushMapStringAny(m map[string]any) {
	L.CreateTable(0, len(m))
	for k, v := range m {
		L.PushString(k)
		L.PushAny(v)
		L.RawSet(-3)
	}
}

func (L *State) pushMapStringString(m map[string]string) {
	L.CreateTable(0, len(m))
	for k, v := range m {
		L.PushString(k)
		L.PushString(v)
		L.RawSet(-3)
	}
}

func (L *State) pushSliceAny(s []any) {
	L.CreateTable(len(s), 0)
	for i, v := range s {
		L.PushAny(v)
		L.RawSetI(-2, int64(i+1))
	}
}

func (L *State) pushReflect(rv reflect.Value) {
	// Dereference pointers.
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			L.PushNil()
			return
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Struct:
		L.pushStruct(rv)
	case reflect.Map:
		L.pushReflectMap(rv)
	case reflect.Slice:
		if rv.IsNil() {
			L.PushNil()
			return
		}
		L.pushReflectSlice(rv)
	case reflect.Array:
		L.pushReflectSlice(rv)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		L.PushInteger(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		pushUint64(L, rv.Uint())
	case reflect.Float32, reflect.Float64:
		L.PushNumber(rv.Float())
	case reflect.Bool:
		L.PushBoolean(rv.Bool())
	case reflect.String:
		L.PushString(rv.String())
	default:
		// chan, func, unsafe.Pointer, etc. → light userdata
		if rv.CanInterface() {
			L.PushLightUserdata(rv.Interface())
		} else {
			L.PushNil()
		}
	}
}

func (L *State) pushStruct(rv reflect.Value) {
	rt := rv.Type()
	n := rt.NumField()
	L.CreateTable(0, n)
	for i := 0; i < n; i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}

		name := structFieldName(field)
		if name == "-" {
			continue
		}

		L.PushString(name)
		L.PushAny(rv.Field(i).Interface())
		L.RawSet(-3)
	}
}

func (L *State) pushReflectMap(rv reflect.Value) {
	if rv.IsNil() {
		L.PushNil()
		return
	}
	keys := rv.MapKeys()
	L.CreateTable(0, len(keys))
	for _, k := range keys {
		// Convert key to string.
		switch k.Kind() {
		case reflect.String:
			L.PushString(k.String())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			L.PushInteger(k.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			pushUint64(L, k.Uint())
		default:
			L.PushString(fmt.Sprint(k.Interface()))
		}
		L.PushAny(rv.MapIndex(k).Interface())
		L.RawSet(-3)
	}
}

func (L *State) pushReflectSlice(rv reflect.Value) {
	n := rv.Len()
	L.CreateTable(n, 0)
	for i := 0; i < n; i++ {
		L.PushAny(rv.Index(i).Interface())
		L.RawSetI(-2, int64(i+1))
	}
}

// structFieldName returns the Lua key name for a struct field.
// Uses the `lua` struct tag if present, otherwise lowercases the first letter.
func structFieldName(f reflect.StructField) string {
	if tag := f.Tag.Get("lua"); tag != "" {
		// Support `lua:"name,omitempty"` — take only the name part.
		if idx := strings.IndexByte(tag, ','); idx >= 0 {
			return tag[:idx]
		}
		return tag
	}
	// Lowercase first letter.
	name := f.Name
	if len(name) == 0 {
		return name
	}
	return strings.ToLower(name[:1]) + name[1:]
}

// ---------------------------------------------------------------------------
// ToAny — Lua stack → Go value
// ---------------------------------------------------------------------------

// ToAny reads the value at the given stack index and returns it as a Go value:
//
//	Lua type    → Go type
//	nil         → nil (interface{})
//	boolean     → bool
//	integer     → int64
//	number      → float64
//	string      → string
//	table       → []any (sequential integer keys 1..n) or map[string]any
//	userdata    → the stored Go value (via [State.UserdataValue])
//	function    → nil (cannot convert)
//	thread      → nil (cannot convert)
//
// Table detection: if the table has exactly n entries and all keys are
// integers 1..n, it is returned as []any. Otherwise it is returned as
// map[string]any with non-string keys converted via strconv or fmt.
func (L *State) ToAny(idx int) any {
	switch L.Type(idx) {
	case TypeNil, TypeNone:
		return nil
	case TypeBoolean:
		return L.ToBoolean(idx)
	case TypeNumber:
		if L.IsInteger(idx) {
			v, _ := L.ToInteger(idx)
			return v
		}
		v, _ := L.ToNumber(idx)
		return v
	case TypeString:
		v, _ := L.ToString(idx)
		return v
	case TypeTable:
		return L.toTable(idx)
	case TypeUserdata, TypeLightUserdata:
		return L.UserdataValue(idx)
	default:
		// TypeFunction, TypeThread — cannot convert meaningfully.
		return nil
	}
}

func (L *State) toTable(idx int) any {
	idx = L.AbsIndex(idx)

	// Count total entries and check if it's a pure array.
	length := L.LenI(idx) // rawlen — number of sequential integer keys from 1
	if length > 0 {
		// Count all keys to see if they're exactly 1..length.
		count := int64(0)
		L.PushNil()
		for L.Next(idx) {
			count++
			L.Pop(1) // pop value, keep key for next iteration
		}
		if count == length {
			// Pure array.
			arr := make([]any, length)
			for i := int64(1); i <= length; i++ {
				L.GetI(idx, i)
				arr[i-1] = L.ToAny(-1)
				L.Pop(1)
			}
			return arr
		}
	}

	// Map.
	m := make(map[string]any)
	L.PushNil()
	for L.Next(idx) {
		key := L.tableKeyToString(-2)
		m[key] = L.ToAny(-1)
		L.Pop(1) // pop value, keep key
	}
	return m
}

// tableKeyToString converts a Lua table key at the given index to a Go string.
// Does not modify the stack.
func (L *State) tableKeyToString(idx int) string {
	switch L.Type(idx) {
	case TypeString:
		s, _ := L.ToString(idx)
		return s
	case TypeNumber:
		if L.IsInteger(idx) {
			v, _ := L.ToInteger(idx)
			return strconv.FormatInt(v, 10)
		}
		v, _ := L.ToNumber(idx)
		return strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return L.TolString(idx)
	}
}

// ---------------------------------------------------------------------------
// ToStruct — Lua table → Go struct
// ---------------------------------------------------------------------------

// ToStruct reads a Lua table at the given stack index into a Go struct.
// The dest argument must be a non-nil pointer to a struct.
//
// Field mapping uses the `lua` struct tag if present, otherwise the field
// name with its first letter lowercased. Fields tagged with `lua:"-"` are
// skipped. Unexported fields are always skipped.
//
// Supported field types: string, bool, all int/uint/float variants, and
// nested slices/maps (via [State.ToAny]).
//
// Example:
//
//	type Config struct {
//	    Host  string  `lua:"host"`
//	    Port  int64   `lua:"port"`
//	    Debug bool    `lua:"debug"`
//	}
//	var cfg Config
//	err := L.ToStruct(-1, &cfg)
func (L *State) ToStruct(idx int, dest any) error {
	if L.Type(idx) != TypeTable {
		return fmt.Errorf("lua.ToStruct: expected table at index %d, got %s",
			idx, L.TypeName(L.Type(idx)))
	}
	idx = L.AbsIndex(idx)

	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("lua.ToStruct: dest must be a non-nil pointer to struct")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("lua.ToStruct: dest must point to a struct, got %s", rv.Kind())
	}

	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}

		name := structFieldName(field)
		if name == "-" {
			continue
		}

		L.GetField(idx, name)
		if !L.IsNoneOrNil(-1) {
			setStructField(L, rv.Field(i), -1)
		}
		L.Pop(1)
	}
	return nil
}

// setStructField sets a reflect.Value from the Lua value at the given stack index.
func setStructField(L *State, fv reflect.Value, idx int) {
	switch fv.Kind() {
	case reflect.String:
		if s, ok := L.ToString(idx); ok {
			fv.SetString(s)
		}
	case reflect.Bool:
		fv.SetBool(L.ToBoolean(idx))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v, ok := L.ToInteger(idx); ok {
			fv.SetInt(v)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v, ok := L.ToInteger(idx); ok {
			fv.SetUint(uint64(v))
		}
	case reflect.Float32, reflect.Float64:
		if v, ok := L.ToNumber(idx); ok {
			fv.SetFloat(v)
		}
	case reflect.Slice, reflect.Map, reflect.Interface:
		if L.IsTable(idx) {
			val := L.ToAny(idx)
			if val != nil {
				rval := reflect.ValueOf(val)
				if rval.Type().AssignableTo(fv.Type()) {
					fv.Set(rval)
				}
			}
		}
	case reflect.Struct:
		if L.IsTable(idx) {
			// Recursively fill nested struct.
			_ = L.ToStruct(idx, fv.Addr().Interface())
		}
	case reflect.Ptr:
		// Pointer to struct — allocate and fill.
		if fv.Type().Elem().Kind() == reflect.Struct && L.IsTable(idx) {
			newVal := reflect.New(fv.Type().Elem())
			_ = L.ToStruct(idx, newVal.Interface())
			fv.Set(newVal)
		}
	}
}

// ---------------------------------------------------------------------------
// RegisterModule — register a Go module for require()
// ---------------------------------------------------------------------------

// RegisterModule registers a set of Go functions as a Lua module that can
// be loaded via require(name). It sets package.preload[name] to an opener
// function that creates a table with the given functions.
//
// Example:
//
//	RegisterModule(L, "mymod", map[string]lua.Function{
//	    "greet": greetFunc,
//	    "add":   addFunc,
//	})
//	// Lua: local m = require("mymod"); m.greet("world")
func RegisterModule(L *State, name string, funcs map[string]Function) {
	L.GetGlobal("package")
	if L.IsNil(-1) {
		L.Pop(1)
		return // no package library loaded
	}
	L.GetField(-1, "preload")
	if L.IsNil(-1) {
		L.Pop(2)
		return // no preload table
	}

	// Capture funcs in closure.
	fns := funcs
	L.PushFunction(func(L *State) int {
		L.NewTable()
		L.SetFuncs(fns, 0)
		return 1
	})
	L.SetField(-2, name)
	L.Pop(2) // pop preload + package
}
