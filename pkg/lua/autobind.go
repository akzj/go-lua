package lua

import (
	"reflect"
)

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// PushGoFunc pushes an arbitrary Go function onto the Lua stack as a
// Lua-callable function. Parameters are read from the Lua stack via
// reflection and return values are pushed back automatically.
//
// Supported parameter types: string, bool, int/int8/../int64,
// uint/uint8/../uint64, float32/float64, map[string]any, []any,
// any (interface{}), structs (via ToStruct), *struct.
//
// Supported return types: same as parameters, plus error.
// If the last return is error and non-nil, a Lua error is raised.
// If the last return is error and nil, it is not pushed.
//
// If Lua passes fewer args than the function expects, missing parameters
// receive their Go zero value.
//
// Example:
//
//	L.PushGoFunc(func(name string, age int) string {
//	    return fmt.Sprintf("Hello %s, age %d", name, age)
//	})
//	L.SetGlobal("greet")
//	// Lua: greet("world", 42) → "Hello world, age 42"
func (L *State) PushGoFunc(fn any) {
	rv := reflect.ValueOf(fn)
	rt := rv.Type()

	if rt.Kind() != reflect.Func {
		panic("PushGoFunc: argument must be a function")
	}

	numIn := rt.NumIn()
	numOut := rt.NumOut()
	hasErrorReturn := numOut > 0 && rt.Out(numOut-1).Implements(errorType)
	isVariadic := rt.IsVariadic()

	L.PushFunction(func(L *State) int {
		luaTop := L.GetTop()

		var args []reflect.Value
		if isVariadic {
			// For variadic functions, expand all args individually.
			// reflect.Call on a variadic func packs trailing args into the slice.
			// e.g. func(string, ...any) called with ("a","b","c") needs
			// args = [ValueOf("a"), ValueOf("b"), ValueOf("c")]
			variadicIdx := numIn - 1
			variadicElemType := rt.In(variadicIdx).Elem()

			// Fixed params first.
			nFixed := variadicIdx
			totalArgs := luaTop
			if totalArgs < nFixed {
				totalArgs = nFixed // pad missing fixed args with zero
			}
			args = make([]reflect.Value, totalArgs)
			for i := 0; i < nFixed; i++ {
				if i < luaTop {
					args[i] = luaArgToReflect(L, i+1, rt.In(i))
				} else {
					args[i] = reflect.Zero(rt.In(i))
				}
			}
			// Variadic args: each one individually.
			for i := nFixed; i < luaTop; i++ {
				args[i] = luaArgToReflect(L, i+1, variadicElemType)
			}
		} else {
			args = make([]reflect.Value, numIn)
			for i := 0; i < numIn; i++ {
				if i < luaTop {
					args[i] = luaArgToReflect(L, i+1, rt.In(i))
				} else {
					args[i] = reflect.Zero(rt.In(i))
				}
			}
		}

		// Call the Go function.
		results := rv.Call(args)

		// Push results onto the Lua stack.
		nResults := 0
		for i, r := range results {
			if hasErrorReturn && i == numOut-1 {
				if !r.IsNil() {
					return L.Errorf("%s", r.Interface().(error).Error())
				}
				continue // nil error → skip
			}
			L.PushAny(r.Interface())
			nResults++
		}
		return nResults
	})
}

// luaArgToReflect reads a Lua value at stack index idx and converts it
// to a reflect.Value of the given target type.
func luaArgToReflect(L *State, idx int, target reflect.Type) reflect.Value {
	// Handle interface{}/any — just use ToAny.
	if target.Kind() == reflect.Interface {
		val := L.ToAny(idx)
		if val == nil {
			return reflect.Zero(target)
		}
		return reflect.ValueOf(val)
	}

	switch target.Kind() {
	case reflect.String:
		s, _ := L.ToString(idx)
		return reflect.ValueOf(s).Convert(target)

	case reflect.Int:
		n, _ := L.ToInteger(idx)
		return reflect.ValueOf(int(n))
	case reflect.Int8:
		n, _ := L.ToInteger(idx)
		return reflect.ValueOf(int8(n))
	case reflect.Int16:
		n, _ := L.ToInteger(idx)
		return reflect.ValueOf(int16(n))
	case reflect.Int32:
		n, _ := L.ToInteger(idx)
		return reflect.ValueOf(int32(n))
	case reflect.Int64:
		n, _ := L.ToInteger(idx)
		return reflect.ValueOf(n)

	case reflect.Uint:
		n, _ := L.ToInteger(idx)
		return reflect.ValueOf(uint(n))
	case reflect.Uint8:
		n, _ := L.ToInteger(idx)
		return reflect.ValueOf(uint8(n))
	case reflect.Uint16:
		n, _ := L.ToInteger(idx)
		return reflect.ValueOf(uint16(n))
	case reflect.Uint32:
		n, _ := L.ToInteger(idx)
		return reflect.ValueOf(uint32(n))
	case reflect.Uint64:
		n, _ := L.ToInteger(idx)
		return reflect.ValueOf(uint64(n))

	case reflect.Float32:
		n, _ := L.ToNumber(idx)
		return reflect.ValueOf(float32(n))
	case reflect.Float64:
		n, _ := L.ToNumber(idx)
		return reflect.ValueOf(n)

	case reflect.Bool:
		return reflect.ValueOf(L.ToBoolean(idx))

	case reflect.Map:
		val := L.ToAny(idx)
		if val == nil {
			return reflect.Zero(target)
		}
		rv := reflect.ValueOf(val)
		if rv.Type().ConvertibleTo(target) {
			return rv.Convert(target)
		}
		return rv

	case reflect.Slice:
		val := L.ToAny(idx)
		if val == nil {
			return reflect.Zero(target)
		}
		return reflect.ValueOf(val)

	case reflect.Struct:
		ptr := reflect.New(target)
		_ = L.ToStruct(idx, ptr.Interface())
		return ptr.Elem()

	case reflect.Ptr:
		if target.Elem().Kind() == reflect.Struct {
			ptr := reflect.New(target.Elem())
			_ = L.ToStruct(idx, ptr.Interface())
			return ptr
		}
		return reflect.Zero(target)

	default:
		// Fallback: try ToAny.
		val := L.ToAny(idx)
		if val == nil {
			return reflect.Zero(target)
		}
		return reflect.ValueOf(val)
	}
}
