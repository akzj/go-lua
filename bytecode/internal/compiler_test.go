// Package internal provides bytecode compiler tests.
package internal

import (
	"testing"

	"github.com/akzj/go-lua/bytecode/api"
)

func TestNewCompiler(t *testing.T) {
	c := NewCompiler("test")
	if c == nil {
		t.Fatal("NewCompiler returned nil")
	}
}

func TestCompilerCompileNil(t *testing.T) {
	c := NewCompiler("test")
	_, err := c.Compile(nil)
	if err == nil {
		t.Fatal("expected error for nil chunk")
	}
}

func TestPrototypeInterface(t *testing.T) {
	proto := &Prototype{
		sourceName:      "test",
		lineDefined:    1,
		lastLineDefined: 10,
		numparams:      2,
		flag:            0,
		maxstacksize:   16,
	}

	// Test interface methods
	if proto.SourceName() != "test" {
		t.Error("SourceName() wrong")
	}
	if proto.LineDefined() != 1 {
		t.Error("LineDefined() wrong")
	}
	if proto.LastLineDefined() != 10 {
		t.Error("LastLineDefined() wrong")
	}
	if proto.NumParams() != 2 {
		t.Error("NumParams() wrong")
	}
	if proto.IsVararg() {
		t.Error("IsVararg() should be false")
	}
	if proto.MaxStackSize() != 16 {
		t.Error("MaxStackSize() wrong")
	}

	// Test internal getters
	if proto.GetCode() != nil {
		t.Error("GetCode() should be nil initially")
	}
	if proto.GetConstants() != nil {
		t.Error("GetConstants() should be nil initially")
	}
}

func TestLocals(t *testing.T) {
	locals := NewLocals()
	if locals.Count() != 0 {
		t.Fatalf("expected 0 locals, got %d", locals.Count())
	}

	locals.Add("x", 0, 0)
	if locals.Count() != 1 {
		t.Fatalf("expected 1 local, got %d", locals.Count())
	}

	v := locals.Get(0)
	if v == nil || v.Name != "x" {
		t.Fatal("Get(0) returned wrong value")
	}

	// Test Get out of bounds
	if locals.Get(-1) != nil {
		t.Fatal("Get(-1) should return nil")
	}
	if locals.Get(100) != nil {
		t.Fatal("Get(100) should return nil")
	}
}

func TestConstantEquals(t *testing.T) {
	c1 := NewConstInteger(42)
	c2 := NewConstInteger(42)
	c3 := NewConstInteger(100)

	if !c1.equals(c2) {
		t.Error("same integers should be equal")
	}
	if c1.equals(c3) {
		t.Error("different integers should not be equal")
	}

	s1 := NewConstString("hello")
	s2 := NewConstString("hello")
	s3 := NewConstString("world")
	if !s1.equals(s2) {
		t.Error("same strings should be equal")
	}
	if s1.equals(s3) {
		t.Error("different strings should not be equal")
	}

	b1 := NewConstBool(true)
	b2 := NewConstBool(true)
	b3 := NewConstBool(false)
	if !b1.equals(b2) {
		t.Error("same bools should be equal")
	}
	if b1.equals(b3) {
		t.Error("different bools should not be equal")
	}
}

func TestAddConstant(t *testing.T) {
	proto := &Prototype{}
	fs := &FuncState{Proto: proto}

	idx1 := fs.addConstant(NewConstInteger(42))
	if idx1 != 0 {
		t.Errorf("first constant should be at index 0, got %d", idx1)
	}

	idx2 := fs.addConstant(NewConstInteger(100))
	if idx2 != 1 {
		t.Errorf("second constant should be at index 1, got %d", idx2)
	}

	// Duplicate should return same index
	idx3 := fs.addConstant(NewConstInteger(42))
	if idx3 != 0 {
		t.Errorf("duplicate constant should return index 0, got %d", idx3)
	}

	if len(proto.k) != 2 {
		t.Errorf("expected 2 constants, got %d", len(proto.k))
	}
}

func TestFuncStateAllocReg(t *testing.T) {
	proto := &Prototype{maxstacksize: 2}
	fs := &FuncState{Proto: proto}

	reg := fs.allocReg()
	if reg != 2 {
		t.Errorf("first alloc should return 2, got %d", reg)
	}
	if proto.maxstacksize != 3 {
		t.Errorf("maxstacksize should be 3, got %d", proto.maxstacksize)
	}

	reg2 := fs.allocReg()
	if reg2 != 3 {
		t.Errorf("second alloc should return 3, got %d", reg2)
	}
}

func TestCompileError(t *testing.T) {
	err := api.NewCompileError(1, 2, "test error")
	if err.Line != 1 || err.Column != 2 {
		t.Fatal("CompileError has wrong line/column")
	}
	if err.Error() != "test error" {
		t.Fatal("CompileError.Error() wrong")
	}
}
