// Additional TValue constructors, accessors, and comparison helpers.
//
// These extend the base types defined in api.go with constructors for
// GC object types and raw equality comparison.
package api

// ---------------------------------------------------------------------------
// GC-type constructors
// These use 'any' to avoid import cycles with table/closure/state packages.
// The caller is responsible for passing the correct pointer type.
// ---------------------------------------------------------------------------

// MakeTable creates a table TValue. t should be a *table.Table.
func MakeTable(t any) TValue { return TValue{Tt: TagTable, Val: t} }

// MakeLuaClosure creates a Lua closure TValue. c should be a *closure.LClosure.
func MakeLuaClosure(c any) TValue { return TValue{Tt: TagLuaClosure, Val: c} }

// MakeCClosure creates a C closure TValue. c should be a *closure.CClosure.
func MakeCClosure(c any) TValue { return TValue{Tt: TagCClosure, Val: c} }

// MakeLightCFunc creates a light C function TValue (no upvalues).
// f should be a stateapi.CFunction.
func MakeLightCFunc(f any) TValue { return TValue{Tt: TagLightCFunc, Val: f} }

// MakeUserdata creates a full userdata TValue.
func MakeUserdata(u *Userdata) TValue { return TValue{Tt: TagUserdata, Val: u} }

// MakeLightUserdata creates a light userdata TValue.
// p is an arbitrary Go value used as an opaque pointer.
func MakeLightUserdata(p any) TValue { return TValue{Tt: TagLightUserdata, Val: p} }

// MakeThread creates a thread TValue. t should be a *stateapi.LuaState.
func MakeThread(t any) TValue { return TValue{Tt: TagThread, Val: t} }

// MakeProto creates a proto TValue (internal, used by compiler/VM).
func MakeProto(p *Proto) TValue { return TValue{Tt: TagProto, Val: p} }

// ---------------------------------------------------------------------------
// GC-type accessors
// These return 'any' — the caller must type-assert to the concrete type.
// ---------------------------------------------------------------------------

// TableVal returns the table pointer. Panics if not a table.
func (v TValue) TableVal() any {
	if v.Tt != TagTable {
		panic("TValue.TableVal: not a table")
	}
	return v.Val
}

// LuaClosureVal returns the Lua closure pointer. Panics if not a Lua closure.
func (v TValue) LuaClosureVal() any {
	if v.Tt != TagLuaClosure {
		panic("TValue.LuaClosureVal: not a Lua closure")
	}
	return v.Val
}

// CClosureVal returns the C closure pointer. Panics if not a C closure.
func (v TValue) CClosureVal() any {
	if v.Tt != TagCClosure {
		panic("TValue.CClosureVal: not a C closure")
	}
	return v.Val
}

// ClosureVal returns the closure pointer (Lua or C). Panics if not a function.
func (v TValue) ClosureVal() any {
	if v.Tt != TagLuaClosure && v.Tt != TagCClosure && v.Tt != TagLightCFunc {
		panic("TValue.ClosureVal: not a function")
	}
	return v.Val
}

// LightCFuncVal returns the light C function. Panics if not a light C function.
func (v TValue) LightCFuncVal() any {
	if v.Tt != TagLightCFunc {
		panic("TValue.LightCFuncVal: not a light C function")
	}
	return v.Val
}

// UserdataVal returns the Userdata pointer. Panics if not a full userdata.
func (v TValue) UserdataVal() *Userdata {
	if v.Tt != TagUserdata {
		panic("TValue.UserdataVal: not a userdata")
	}
	return v.Val.(*Userdata)
}

// LightUserdataVal returns the light userdata value. Panics if not light userdata.
func (v TValue) LightUserdataVal() any {
	if v.Tt != TagLightUserdata {
		panic("TValue.LightUserdataVal: not a light userdata")
	}
	return v.Val
}

// ThreadVal returns the thread pointer. Panics if not a thread.
func (v TValue) ThreadVal() any {
	if v.Tt != TagThread {
		panic("TValue.ThreadVal: not a thread")
	}
	return v.Val
}

// ProtoVal returns the Proto pointer. Panics if not a proto.
func (v TValue) ProtoVal() *Proto {
	if v.Tt != TagProto {
		panic("TValue.ProtoVal: not a proto")
	}
	return v.Val.(*Proto)
}

// ---------------------------------------------------------------------------
// Raw equality (no metamethods)
//
// Reference: lua-master/lvm.c luaV_equalobj with L==NULL (raw mode)
// ---------------------------------------------------------------------------

// RawEqual compares two TValues for raw equality (no metamethods).
//
// Rules:
//   - Different base types → false
//   - Same base type, different variant:
//     * integer vs float → compare values (float must be exact integer)
//     * short string vs long string → compare content
//     * all others → false
//   - Same variant:
//     * nil, false, true → true (singletons)
//     * integer → value equality
//     * float → value equality (NaN != NaN)
//     * short string → pointer equality (interned)
//     * long string → content equality
//     * everything else → pointer identity
func RawEqual(v1, v2 TValue) bool {
	// Different base types → not equal
	if v1.Tt.BaseType() != v2.Tt.BaseType() {
		return false
	}

	// Same base type, possibly different variants
	if v1.Tt != v2.Tt {
		switch v1.Tt {
		case TagInteger:
			// integer == float?
			if v2.Tt == TagFloat {
				f := v2.Val.(float64)
				i2, ok := floatToIntegerEq(f)
				return ok && v1.Val.(int64) == i2
			}
			return false
		case TagFloat:
			// float == integer?
			if v2.Tt == TagInteger {
				f := v1.Val.(float64)
				i1, ok := floatToIntegerEq(f)
				return ok && i1 == v2.Val.(int64)
			}
			return false
		case TagShortStr, TagLongStr:
			// short string vs long string — compare content
			if v2.Tt == TagShortStr || v2.Tt == TagLongStr {
				return v1.Val.(*LuaString).Data == v2.Val.(*LuaString).Data
			}
			return false
		default:
			return false
		}
	}

	// Same variant
	switch v1.Tt {
	case TagNil, TagFalse, TagTrue:
		return true
	case TagInteger:
		return v1.Val.(int64) == v2.Val.(int64)
	case TagFloat:
		return v1.Val.(float64) == v2.Val.(float64) // NaN != NaN by IEEE 754
	case TagShortStr:
		// Interned short strings: pointer equality
		return v1.Val == v2.Val
	case TagLongStr:
		// Long strings: content equality
		return v1.Val.(*LuaString).Data == v2.Val.(*LuaString).Data
	case TagLightUserdata:
		return v1.Val == v2.Val
	default:
		// Tables, closures, userdata, threads: pointer identity
		return v1.Val == v2.Val
	}
}

// floatToIntegerEq converts a float to integer for equality comparison.
// Returns the integer and true only if the float is an exact integer value.
func floatToIntegerEq(f float64) (int64, bool) {
	i := int64(f)
	if float64(i) == f {
		return i, true
	}
	return 0, false
}

// ---------------------------------------------------------------------------
// Type name helper
// ---------------------------------------------------------------------------

// TypeName returns the Lua type name for a TValue.
func TypeNameOf(v TValue) string {
	bt := v.Tt.BaseType()
	if int(bt) < len(TypeNames) {
		return TypeNames[bt]
	}
	return "no value"
}

// ---------------------------------------------------------------------------
// SetObj copies src into dst (used for stack operations).
// ---------------------------------------------------------------------------

// SetObj copies the TValue from src to dst.
func SetObj(dst *TValue, src TValue) {
	dst.Tt = src.Tt
	dst.Val = src.Val
}
