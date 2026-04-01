package api

import (
	"testing"

	"github.com/akzj/go-lua/api/api"
)

// TestConstants verifies all exported constants exist and have expected values.
func TestConstants(t *testing.T) {
	// Basic types
	if LUA_TNIL != api.LUA_TNIL {
		t.Error("LUA_TNIL mismatch")
	}
	if LUA_TBOOLEAN != api.LUA_TBOOLEAN {
		t.Error("LUA_TBOOLEAN mismatch")
	}
	if LUA_TNUMBER != api.LUA_TNUMBER {
		t.Error("LUA_TNUMBER mismatch")
	}
	if LUA_TSTRING != api.LUA_TSTRING {
		t.Error("LUA_TSTRING mismatch")
	}
	if LUA_TTABLE != api.LUA_TTABLE {
		t.Error("LUA_TTABLE mismatch")
	}
	if LUA_TFUNCTION != api.LUA_TFUNCTION {
		t.Error("LUA_TFUNCTION mismatch")
	}

	// Status codes
	if LUA_OK != api.LUA_OK {
		t.Error("LUA_OK mismatch")
	}
	if LUA_YIELD != api.LUA_YIELD {
		t.Error("LUA_YIELD mismatch")
	}
	if LUA_ERRRUN != api.LUA_ERRRUN {
		t.Error("LUA_ERRRUN mismatch")
	}

	// GC constants
	if LUA_GCSTOP != api.LUA_GCSTOP {
		t.Error("LUA_GCSTOP mismatch")
	}
	if LUA_GCCOLLECT != api.LUA_GCCOLLECT {
		t.Error("LUA_GCCOLLECT mismatch")
	}

	// Special values
	if LUA_MULTRET != -1 {
		t.Error("LUA_MULTRET should be -1")
	}
	if LUA_REGISTRYINDEX != -10000 {
		t.Error("LUA_REGISTRYINDEX should be -10000")
	}
}

// TestArithmeticOperators verifies arithmetic operator constants.
func TestArithmeticOperators(t *testing.T) {
	if LUA_OPADD != 0 {
		t.Error("LUA_OPADD should be 0")
	}
	if LUA_OPSUB != 1 {
		t.Error("LUA_OPSUB should be 1")
	}
	if LUA_OPMUL != 2 {
		t.Error("LUA_OPMUL should be 2")
	}
}

// TestComparisonOperators verifies comparison operator constants.
func TestComparisonOperators(t *testing.T) {
	if LUA_OPEQ != 0 {
		t.Error("LUA_OPEQ should be 0")
	}
	if LUA_OPLT != 1 {
		t.Error("LUA_OPLT should be 1")
	}
	if LUA_OPLE != 2 {
		t.Error("LUA_OPLE should be 2")
	}
}

// TestTypename verifies type name strings.
func TestTypename(t *testing.T) {
	if Typename(LUA_TNIL) != "nil" {
		t.Error("Typename(LUA_TNIL) should be 'nil'")
	}
	if Typename(LUA_TSTRING) != "string" {
		t.Error("Typename(LUA_TSTRING) should be 'string'")
	}
	if Typename(LUA_TTABLE) != "table" {
		t.Error("Typename(LUA_TTABLE) should be 'table'")
	}
}

// TestStatusString verifies status string conversion.
func TestStatusString(t *testing.T) {
	if StatusString(LUA_OK) != "OK" {
		t.Error("StatusString(LUA_OK) should be 'OK'")
	}
	if StatusString(LUA_YIELD) != "Yield" {
		t.Error("StatusString(LUA_YIELD) should be 'Yield'")
	}
	if StatusString(LUA_ERRRUN) != "Runtime error" {
		t.Error("StatusString(LUA_ERRRUN) should be 'Runtime error'")
	}
}

// TestLuaL_Reg verifies the LuaL_Reg type.
func TestLuaL_Reg(t *testing.T) {
	reg := LuaL_Reg{
		Name: "test",
		Func: nil,
	}
	if reg.Name != "test" {
		t.Error("LuaL_Reg.Name should be 'test'")
	}
}
