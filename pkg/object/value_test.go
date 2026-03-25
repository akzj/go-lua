package object

import (
	"testing"
)

// Test Type constants
func TestTypeConstants(t *testing.T) {
	tests := []struct {
		typ  Type
		name string
	}{
		{TypeNil, "nil"},
		{TypeBoolean, "boolean"},
		{TypeLightUserData, "lightuserdata"},
		{TypeNumber, "number"},
		{TypeString, "string"},
		{TypeTable, "table"},
		{TypeFunction, "function"},
		{TypeUserData, "userdata"},
		{TypeThread, "thread"},
		{TypeProto, "proto"},
		{TypeUpValue, "upvalue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.typ.String() != tt.name {
				t.Errorf("Type.String() = %q, want %q", tt.typ.String(), tt.name)
			}
		})
	}
}

// Test TValue creation functions
func TestTValueCreation(t *testing.T) {
	t.Run("NewNil", func(t *testing.T) {
		v := NewNil()
		if !v.IsNil() {
			t.Error("NewNil() should create nil value")
		}
	})

	t.Run("NewBoolean", func(t *testing.T) {
		vTrue := NewBoolean(true)
		if !vTrue.IsBoolean() {
			t.Error("NewBoolean(true) should create boolean value")
		}
		b, ok := vTrue.ToBoolean()
		if !ok || !b {
			t.Error("NewBoolean(true) ToBoolean() should return true")
		}

		vFalse := NewBoolean(false)
		b, ok = vFalse.ToBoolean()
		if !ok || b {
			t.Error("NewBoolean(false) ToBoolean() should return false")
		}
	})

	t.Run("NewNumber", func(t *testing.T) {
		v := NewNumber(42.5)
		if !v.IsNumber() {
			t.Error("NewNumber() should create number value")
		}
		n, ok := v.ToNumber()
		if !ok || n != 42.5 {
			t.Errorf("NewNumber(42.5) ToNumber() = %v, want 42.5", n)
		}
	})

	t.Run("NewInteger", func(t *testing.T) {
		v := NewInteger(100)
		if !v.IsNumber() {
			t.Error("NewInteger() should create number value")
		}
		n, ok := v.ToNumber()
		if !ok || n != 100.0 {
			t.Errorf("NewInteger(100) ToNumber() = %v, want 100.0", n)
		}
	})

	t.Run("NewString", func(t *testing.T) {
		v := NewString("hello")
		if !v.IsString() {
			t.Error("NewString() should create string value")
		}
		s, ok := v.ToString()
		if !ok || s != "hello" {
			t.Errorf("NewString(\"hello\") ToString() = %v, want \"hello\"", s)
		}
	})

	t.Run("NewTable", func(t *testing.T) {
		table := NewTable()
		v := NewTableValue(table)
		if !v.IsTable() {
			t.Error("NewTable() should create table value")
		}
		tbl, ok := v.ToTable()
		if !ok || tbl != table {
			t.Error("NewTable() ToTable() should return the same table")
		}
	})

	t.Run("NewFunction", func(t *testing.T) {
		closure := &Closure{IsGo: true}
		v := NewFunction(closure)
		if !v.IsFunction() {
			t.Error("NewFunction() should create function value")
		}
		fn, ok := v.ToFunction()
		if !ok || fn != closure {
			t.Error("NewFunction() ToFunction() should return the same closure")
		}
	})

	t.Run("NewUserData", func(t *testing.T) {
		ud := &UserData{Value: "test"}
		v := NewUserData(ud)
		if !v.IsUserData() {
			t.Error("NewUserData() should create userdata value")
		}
		userData, ok := v.ToUserData()
		if !ok || userData != ud {
			t.Error("NewUserData() ToUserData() should return the same userdata")
		}
	})

	t.Run("NewThread", func(t *testing.T) {
		thread := &Thread{}
		v := NewThread(thread)
		if !v.IsThread() {
			t.Error("NewThread() should create thread value")
		}
		th, ok := v.ToThread()
		if !ok || th != thread {
			t.Error("NewThread() ToThread() should return the same thread")
		}
	})

	t.Run("NewLightUserData", func(t *testing.T) {
		ptr := &struct{}{}
		v := NewLightUserData(ptr)
		if !v.IsLightUserData() {
			t.Error("NewLightUserData() should create light userdata value")
		}
		p, ok := v.ToLightUserData()
		if !ok || p != ptr {
			t.Error("NewLightUserData() ToLightUserData() should return the same pointer")
		}
	})
}

// Test TValue type checking methods
func TestTValueTypeChecking(t *testing.T) {
	tests := []struct {
		name     string
		value    *TValue
		isNil    bool
		isBool   bool
		isNumber bool
		isString bool
		isTable  bool
		isFunc   bool
		isUD     bool
		isThread bool
		isLUD    bool
	}{
		{
			name:     "nil",
			value:    NewNil(),
			isNil:    true,
			isBool:   false,
			isNumber: false,
			isString: false,
			isTable:  false,
			isFunc:   false,
			isUD:     false,
			isThread: false,
			isLUD:    false,
		},
		{
			name:     "boolean",
			value:    NewBoolean(true),
			isNil:    false,
			isBool:   true,
			isNumber: false,
			isString: false,
			isTable:  false,
			isFunc:   false,
			isUD:     false,
			isThread: false,
			isLUD:    false,
		},
		{
			name:     "number",
			value:    NewNumber(42.0),
			isNil:    false,
			isBool:   false,
			isNumber: true,
			isString: false,
			isTable:  false,
			isFunc:   false,
			isUD:     false,
			isThread: false,
			isLUD:    false,
		},
		{
			name:     "string",
			value:    NewString("test"),
			isNil:    false,
			isBool:   false,
			isNumber: false,
			isString: true,
			isTable:  false,
			isFunc:   false,
			isUD:     false,
			isThread: false,
			isLUD:    false,
		},
		{
			name:     "table",
			value:    NewTableValue(NewTable()),
			isNil:    false,
			isBool:   false,
			isNumber: false,
			isString: false,
			isTable:  true,
			isFunc:   false,
			isUD:     false,
			isThread: false,
			isLUD:    false,
		},
		{
			name:     "function",
			value:    NewFunction(&Closure{IsGo: true}),
			isNil:    false,
			isBool:   false,
			isNumber: false,
			isString: false,
			isTable:  false,
			isFunc:   true,
			isUD:     false,
			isThread: false,
			isLUD:    false,
		},
		{
			name:     "userdata",
			value:    NewUserData(&UserData{}),
			isNil:    false,
			isBool:   false,
			isNumber: false,
			isString: false,
			isTable:  false,
			isFunc:   false,
			isUD:     true,
			isThread: false,
			isLUD:    false,
		},
		{
			name:     "thread",
			value:    NewThread(&Thread{}),
			isNil:    false,
			isBool:   false,
			isNumber: false,
			isString: false,
			isTable:  false,
			isFunc:   false,
			isUD:     false,
			isThread: true,
			isLUD:    false,
		},
		{
			name:     "lightuserdata",
			value:    NewLightUserData(nil),
			isNil:    false,
			isBool:   false,
			isNumber: false,
			isString: false,
			isTable:  false,
			isFunc:   false,
			isUD:     false,
			isThread: false,
			isLUD:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value.IsNil() != tt.isNil {
				t.Errorf("IsNil() = %v, want %v", tt.value.IsNil(), tt.isNil)
			}
			if tt.value.IsBoolean() != tt.isBool {
				t.Errorf("IsBoolean() = %v, want %v", tt.value.IsBoolean(), tt.isBool)
			}
			if tt.value.IsNumber() != tt.isNumber {
				t.Errorf("IsNumber() = %v, want %v", tt.value.IsNumber(), tt.isNumber)
			}
			if tt.value.IsString() != tt.isString {
				t.Errorf("IsString() = %v, want %v", tt.value.IsString(), tt.isString)
			}
			if tt.value.IsTable() != tt.isTable {
				t.Errorf("IsTable() = %v, want %v", tt.value.IsTable(), tt.isTable)
			}
			if tt.value.IsFunction() != tt.isFunc {
				t.Errorf("IsFunction() = %v, want %v", tt.value.IsFunction(), tt.isFunc)
			}
			if tt.value.IsUserData() != tt.isUD {
				t.Errorf("IsUserData() = %v, want %v", tt.value.IsUserData(), tt.isUD)
			}
			if tt.value.IsThread() != tt.isThread {
				t.Errorf("IsThread() = %v, want %v", tt.value.IsThread(), tt.isThread)
			}
			if tt.value.IsLightUserData() != tt.isLUD {
				t.Errorf("IsLightUserData() = %v, want %v", tt.value.IsLightUserData(), tt.isLUD)
			}
		})
	}
}

// Test TValue conversion methods
func TestTValueConversion(t *testing.T) {
	t.Run("ToNumber", func(t *testing.T) {
		// Valid conversion
		v := NewNumber(42.5)
		n, ok := v.ToNumber()
		if !ok || n != 42.5 {
			t.Errorf("ToNumber() = %v, %v, want 42.5, true", n, ok)
		}

		// Invalid conversion
		v2 := NewString("test")
		n, ok = v2.ToNumber()
		if ok || n != 0 {
			t.Errorf("ToNumber() on string = %v, %v, want 0, false", n, ok)
		}
	})

	t.Run("ToString", func(t *testing.T) {
		// Valid conversion
		v := NewString("hello")
		s, ok := v.ToString()
		if !ok || s != "hello" {
			t.Errorf("ToString() = %v, %v, want \"hello\", true", s, ok)
		}

		// Invalid conversion
		v2 := NewNumber(42.0)
		s, ok = v2.ToString()
		if ok || s != "" {
			t.Errorf("ToString() on number = %v, %v, want \"\", false", s, ok)
		}
	})

	t.Run("ToBoolean", func(t *testing.T) {
		// Valid conversion - true
		v := NewBoolean(true)
		b, ok := v.ToBoolean()
		if !ok || !b {
			t.Errorf("ToBoolean() = %v, %v, want true, true", b, ok)
		}

		// Valid conversion - false
		v2 := NewBoolean(false)
		b, ok = v2.ToBoolean()
		if !ok || b {
			t.Errorf("ToBoolean() = %v, %v, want false, true", b, ok)
		}

		// Invalid conversion
		v3 := NewNumber(42.0)
		b, ok = v3.ToBoolean()
		if ok || b {
			t.Errorf("ToBoolean() on number = %v, %v, want false, false", b, ok)
		}
	})

	t.Run("ToTable", func(t *testing.T) {
		// Valid conversion
		table := NewTable()
		v := NewTableValue(table)
		tbl, ok := v.ToTable()
		if !ok || tbl != table {
			t.Errorf("ToTable() = %v, %v, want %v, true", tbl, ok, table)
		}

		// Invalid conversion
		v2 := NewNumber(42.0)
		tbl, ok = v2.ToTable()
		if ok || tbl != nil {
			t.Errorf("ToTable() on number = %v, %v, want nil, false", tbl, ok)
		}
	})

	t.Run("ToFunction", func(t *testing.T) {
		// Valid conversion
		closure := &Closure{IsGo: true}
		v := NewFunction(closure)
		fn, ok := v.ToFunction()
		if !ok || fn != closure {
			t.Errorf("ToFunction() = %v, %v, want %v, true", fn, ok, closure)
		}

		// Invalid conversion
		v2 := NewNumber(42.0)
		fn, ok = v2.ToFunction()
		if ok || fn != nil {
			t.Errorf("ToFunction() on number = %v, %v, want nil, false", fn, ok)
		}
	})

	t.Run("ToUserData", func(t *testing.T) {
		// Valid conversion
		ud := &UserData{Value: "test"}
		v := NewUserData(ud)
		userData, ok := v.ToUserData()
		if !ok || userData != ud {
			t.Errorf("ToUserData() = %v, %v, want %v, true", userData, ok, ud)
		}

		// Invalid conversion
		v2 := NewNumber(42.0)
		userData, ok = v2.ToUserData()
		if ok || userData != nil {
			t.Errorf("ToUserData() on number = %v, %v, want nil, false", userData, ok)
		}
	})

	t.Run("ToThread", func(t *testing.T) {
		// Valid conversion
		thread := &Thread{}
		v := NewThread(thread)
		th, ok := v.ToThread()
		if !ok || th != thread {
			t.Errorf("ToThread() = %v, %v, want %v, true", th, ok, thread)
		}

		// Invalid conversion
		v2 := NewNumber(42.0)
		th, ok = v2.ToThread()
		if ok || th != nil {
			t.Errorf("ToThread() on number = %v, %v, want nil, false", th, ok)
		}
	})

	t.Run("ToLightUserData", func(t *testing.T) {
		// Valid conversion
		ptr := &struct{}{}
		v := NewLightUserData(ptr)
		p, ok := v.ToLightUserData()
		if !ok || p != ptr {
			t.Errorf("ToLightUserData() = %v, %v, want %v, true", p, ok, ptr)
		}

		// Invalid conversion
		v2 := NewNumber(42.0)
		p, ok = v2.ToLightUserData()
		if ok || p != nil {
			t.Errorf("ToLightUserData() on number = %v, %v, want nil, false", p, ok)
		}
	})
}

// Test TValue Set methods
func TestTValueSetMethods(t *testing.T) {
	t.Run("SetNil", func(t *testing.T) {
		v := NewNumber(42.0)
		v.SetNil()
		if !v.IsNil() {
			t.Error("SetNil() should set value to nil")
		}
	})

	t.Run("SetBoolean", func(t *testing.T) {
		v := NewNil()
		v.SetBoolean(true)
		if !v.IsBoolean() {
			t.Error("SetBoolean() should set value to boolean")
		}
		b, ok := v.ToBoolean()
		if !ok || !b {
			t.Error("SetBoolean(true) should set value to true")
		}
	})

	t.Run("SetNumber", func(t *testing.T) {
		v := NewNil()
		v.SetNumber(42.5)
		if !v.IsNumber() {
			t.Error("SetNumber() should set value to number")
		}
		n, ok := v.ToNumber()
		if !ok || n != 42.5 {
			t.Errorf("SetNumber(42.5) ToNumber() = %v, want 42.5", n)
		}
	})

	t.Run("SetInteger", func(t *testing.T) {
		v := NewNil()
		v.SetInteger(100)
		if !v.IsNumber() {
			t.Error("SetInteger() should set value to number")
		}
		n, ok := v.ToNumber()
		if !ok || n != 100.0 {
			t.Errorf("SetInteger(100) ToNumber() = %v, want 100.0", n)
		}
	})

	t.Run("SetString", func(t *testing.T) {
		v := NewNil()
		v.SetString("hello")
		if !v.IsString() {
			t.Error("SetString() should set value to string")
		}
		s, ok := v.ToString()
		if !ok || s != "hello" {
			t.Errorf("SetString(\"hello\") ToString() = %v, want \"hello\"", s)
		}
	})

	t.Run("SetTable", func(t *testing.T) {
		v := NewNil()
		table := NewTable()
		v.SetTable(table)
		if !v.IsTable() {
			t.Error("SetTable() should set value to table")
		}
		tbl, ok := v.ToTable()
		if !ok || tbl != table {
			t.Error("SetTable() should set the correct table")
		}
	})

	t.Run("SetFunction", func(t *testing.T) {
		v := NewNil()
		closure := &Closure{IsGo: true}
		v.SetFunction(closure)
		if !v.IsFunction() {
			t.Error("SetFunction() should set value to function")
		}
		fn, ok := v.ToFunction()
		if !ok || fn != closure {
			t.Error("SetFunction() should set the correct closure")
		}
	})

	t.Run("SetUserData", func(t *testing.T) {
		v := NewNil()
		ud := &UserData{Value: "test"}
		v.SetUserData(ud)
		if !v.IsUserData() {
			t.Error("SetUserData() should set value to userdata")
		}
		userData, ok := v.ToUserData()
		if !ok || userData != ud {
			t.Error("SetUserData() should set the correct userdata")
		}
	})

	t.Run("SetThread", func(t *testing.T) {
		v := NewNil()
		thread := &Thread{}
		v.SetThread(thread)
		if !v.IsThread() {
			t.Error("SetThread() should set value to thread")
		}
		th, ok := v.ToThread()
		if !ok || th != thread {
			t.Error("SetThread() should set the correct thread")
		}
	})

	t.Run("SetLightUserData", func(t *testing.T) {
		v := NewNil()
		ptr := &struct{}{}
		v.SetLightUserData(ptr)
		if !v.IsLightUserData() {
			t.Error("SetLightUserData() should set value to light userdata")
		}
		p, ok := v.ToLightUserData()
		if !ok || p != ptr {
			t.Error("SetLightUserData() should set the correct pointer")
		}
	})
}

// Test TValue CopyFrom and Clear
func TestTValueCopyAndClear(t *testing.T) {
	t.Run("CopyFrom", func(t *testing.T) {
		src := NewString("hello")
		dst := NewNil()
		dst.CopyFrom(src)

		if dst.Type != src.Type {
			t.Error("CopyFrom() should copy the type")
		}
		s, ok := dst.ToString()
		if !ok || s != "hello" {
			t.Error("CopyFrom() should copy the value")
		}
	})

	t.Run("Clear", func(t *testing.T) {
		v := NewString("hello")
		v.Clear()
		if !v.IsNil() {
			t.Error("Clear() should set value to nil")
		}
	})
}

// Test IsCollectable
func TestTValueIsCollectable(t *testing.T) {
	tests := []struct {
		name         string
		value        *TValue
		isCollectable bool
	}{
		{"nil", NewNil(), false},
		{"boolean", NewBoolean(true), false},
		{"number", NewNumber(42.0), false},
		{"lightuserdata", NewLightUserData(nil), false},
		{"string", NewString("test"), true},
		{"table", NewTableValue(NewTable()), true},
		{"function", NewFunction(&Closure{}), true},
		{"userdata", NewUserData(&UserData{}), true},
		{"thread", NewThread(&Thread{}), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value.IsCollectable() != tt.isCollectable {
				t.Errorf("IsCollectable() = %v, want %v", tt.value.IsCollectable(), tt.isCollectable)
			}
		})
	}
}

// Test IsFalse (Lua truthiness)
func TestIsFalse(t *testing.T) {
	tests := []struct {
		name   string
		value  *TValue
		isFalse bool
	}{
		{"nil", NewNil(), true},
		{"false", NewBoolean(false), true},
		{"true", NewBoolean(true), false},
		{"number_zero", NewNumber(0), false},
		{"number_nonzero", NewNumber(42), false},
		{"empty_string", NewString(""), false},
		{"string", NewString("hello"), false},
		{"empty_table", NewTableValue(NewTable()), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsFalse(tt.value) != tt.isFalse {
				t.Errorf("IsFalse() = %v, want %v", IsFalse(tt.value), tt.isFalse)
			}
		})
	}
}

// Test Equal
func TestEqual(t *testing.T) {
	t.Run("same_type_equal", func(t *testing.T) {
		v1 := NewNumber(42.0)
		v2 := NewNumber(42.0)
		if !Equal(v1, v2) {
			t.Error("Equal() should return true for equal numbers")
		}

		v3 := NewString("hello")
		v4 := NewString("hello")
		if !Equal(v3, v4) {
			t.Error("Equal() should return true for equal strings")
		}

		v5 := NewBoolean(true)
		v6 := NewBoolean(true)
		if !Equal(v5, v6) {
			t.Error("Equal() should return true for equal booleans")
		}
	})

	t.Run("same_type_not_equal", func(t *testing.T) {
		v1 := NewNumber(42.0)
		v2 := NewNumber(43.0)
		if Equal(v1, v2) {
			t.Error("Equal() should return false for different numbers")
		}

		v3 := NewString("hello")
		v4 := NewString("world")
		if Equal(v3, v4) {
			t.Error("Equal() should return false for different strings")
		}
	})

	t.Run("different_type", func(t *testing.T) {
		v1 := NewNumber(42.0)
		v2 := NewString("42")
		if Equal(v1, v2) {
			t.Error("Equal() should return false for different types")
		}
	})

	t.Run("nil_equality", func(t *testing.T) {
		v1 := NewNil()
		v2 := NewNil()
		if !Equal(v1, v2) {
			t.Error("Equal() should return true for nil values")
		}
	})
}

// Test ToStringRaw
func TestToStringRaw(t *testing.T) {
	tests := []struct {
		name   string
		value  *TValue
		expect string
	}{
		{"nil", NewNil(), "nil"},
		{"true", NewBoolean(true), "true"},
		{"false", NewBoolean(false), "false"},
		{"integer", NewInteger(42), "42"},
		{"float", NewNumber(42.5), "42.5"},
		{"string", NewString("hello"), "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToStringRaw(tt.value)
			if result != tt.expect {
				t.Errorf("ToStringRaw() = %q, want %q", result, tt.expect)
			}
		})
	}
}

// Test Table GCObject implementation
func TestTableGCObject(t *testing.T) {
	table := NewTable()
	var _ GCObject = table // Compile-time check
	table.gcObject()       // Should not panic
}

// Test Closure GCObject implementation
func TestClosureGCObject(t *testing.T) {
	closure := &Closure{IsGo: true}
	var _ GCObject = closure // Compile-time check
	closure.gcObject()       // Should not panic
}

// Test UserData GCObject implementation
func TestUserDataGCObject(t *testing.T) {
	ud := &UserData{Value: "test"}
	var _ GCObject = ud // Compile-time check
	ud.gcObject()       // Should not panic
}

// Test Thread GCObject implementation
func TestThreadGCObject(t *testing.T) {
	thread := &Thread{}
	var _ GCObject = thread // Compile-time check
	thread.gcObject()       // Should not panic
}

// Test Prototype GCObject implementation
func TestPrototypeGCObject(t *testing.T) {
	proto := &Prototype{}
	var _ GCObject = proto // Compile-time check
	proto.gcObject()       // Should not panic
}

// Test Upvalue GCObject implementation
func TestUpvalueGCObject(t *testing.T) {
	v := NewNumber(42.0)
	upval := NewUpvalue(0, v)
	var _ GCObject = upval // Compile-time check
	upval.gcObject()       // Should not panic
}

// Test GCString
func TestGCString(t *testing.T) {
	s := NewGCString("hello")
	var _ GCObject = s // Compile-time check
	s.gcObject()       // Should not panic

	if s.Value != "hello" {
		t.Errorf("GCString.Value = %q, want \"hello\"", s.Value)
	}

	if s.Hash == 0 {
		t.Error("GCString.Hash should not be zero")
	}
}

// Test Upvalue operations
func TestUpvalueOperations(t *testing.T) {
	t.Run("Get", func(t *testing.T) {
		v := NewNumber(42.0)
		upval := NewUpvalue(0, v)
		
		result := upval.Get()
		n, ok := result.ToNumber()
		if !ok || n != 42.0 {
			t.Errorf("Upvalue.Get() = %v, want 42.0", n)
		}
	})

	t.Run("Set", func(t *testing.T) {
		v := NewNumber(42.0)
		upval := NewUpvalue(0, v)
		
		upval.Set(NewNumber(100.0))
		n, ok := v.ToNumber()
		if !ok || n != 100.0 {
			t.Errorf("Upvalue.Set() should update value to 100.0, got %v", n)
		}
	})

	t.Run("Close", func(t *testing.T) {
		v := NewNumber(42.0)
		upval := NewUpvalue(0, v)
		
		upval.Close()
		
		if !upval.Closed {
			t.Error("Upvalue.Close() should set Closed to true")
		}
		
		// Value should be cached
		n, ok := upval.Get().ToNumber()
		if !ok || n != 42.0 {
			t.Errorf("Upvalue.Get() after Close() = %v, want 42.0", n)
		}
	})
}

// Test Prototype structure
func TestPrototype(t *testing.T) {
	proto := &Prototype{
		Code:         []Instruction{0, 1, 2},
		Constants:    []TValue{*NewNumber(42.0), *NewString("hello")},
		Upvalues:     []UpvalueDesc{{Index: 0, IsLocal: true}},
		Prototypes:   []*Prototype{},
		Source:       "test.lua",
		LineInfo:     []int{1, 2, 3},
		NumParams:    2,
		IsVarArg:     false,
		MaxStackSize: 10,
	}

	if len(proto.Code) != 3 {
		t.Errorf("Prototype.Code length = %d, want 3", len(proto.Code))
	}

	if len(proto.Constants) != 2 {
		t.Errorf("Prototype.Constants length = %d, want 2", len(proto.Constants))
	}

	if proto.NumParams != 2 {
		t.Errorf("Prototype.NumParams = %d, want 2", proto.NumParams)
	}
}

// Test Instruction type
func TestInstruction(t *testing.T) {
	var instr Instruction = 0x12345678
	if instr != 0x12345678 {
		t.Error("Instruction should be a 32-bit value")
	}
}

// Test TypeName function
func TestTypeName(t *testing.T) {
	v := NewNumber(42.0)
	name := TypeName(v)
	if name != "number" {
		t.Errorf("TypeName() = %q, want \"number\"", name)
	}
}

// Test edge cases
func TestEdgeCases(t *testing.T) {
	t.Run("ZeroNumber", func(t *testing.T) {
		v := NewNumber(0)
		if !v.IsNumber() {
			t.Error("NewNumber(0) should create number value")
		}
		n, ok := v.ToNumber()
		if !ok || n != 0 {
			t.Errorf("ToNumber() = %v, want 0", n)
		}
	})

	t.Run("EmptyString", func(t *testing.T) {
		v := NewString("")
		if !v.IsString() {
			t.Error("NewString(\"\") should create string value")
		}
		s, ok := v.ToString()
		if !ok || s != "" {
			t.Errorf("ToString() = %q, want \"\"", s)
		}
	})

	t.Run("NegativeNumber", func(t *testing.T) {
		v := NewNumber(-42.5)
		n, ok := v.ToNumber()
		if !ok || n != -42.5 {
			t.Errorf("ToNumber() = %v, want -42.5", n)
		}
	})

	t.Run("LargeInteger", func(t *testing.T) {
		v := NewInteger(1<<60)
		n, ok := v.ToNumber()
		if !ok {
			t.Error("ToNumber() should succeed for large integer")
		}
		expected := float64(int64(1 << 60))
		if n != expected {
			t.Errorf("ToNumber() = %v, want %v", n, expected)
		}
	})
}

// Test type conversion boundary cases
func TestConversionBoundaryCases(t *testing.T) {
	t.Run("NumberToStringConversion", func(t *testing.T) {
		// Note: ToString() only works for string types
		// ToStringRaw() is used for converting numbers to strings
		v := NewNumber(42.0)
		_, ok := v.ToString()
		if ok {
			t.Error("ToString() should not work for number types")
		}

		s := ToStringRaw(v)
		if s != "42" {
			t.Errorf("ToStringRaw() = %q, want \"42\"", s)
		}
	})

	t.Run("FloatPrecision", func(t *testing.T) {
		v := NewNumber(3.14159265359)
		n, ok := v.ToNumber()
		if !ok {
			t.Error("ToNumber() should succeed")
		}
		if n != 3.14159265359 {
			t.Errorf("ToNumber() = %v, want 3.14159265359", n)
		}
	})
}

// Test GCObject interface compliance
func TestGCObjectInterface(t *testing.T) {
	// All these should implement GCObject interface
	var _ GCObject = &Table{}
	var _ GCObject = &Closure{}
	var _ GCObject = &UserData{}
	var _ GCObject = &Thread{}
	var _ GCObject = &Prototype{}
	var _ GCObject = &Upvalue{}
	var _ GCObject = &GCString{}
}