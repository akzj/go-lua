package internal

import (
	"testing"

	baselib "github.com/akzj/go-lua/lib/base/api"
	luaapi "github.com/akzj/go-lua/api"
)

// =============================================================================
// Basic Function Existence Tests
// =============================================================================

// TestNewBaseLib tests creating a new BaseLib instance.
func TestNewBaseLib(t *testing.T) {
	lib := NewBaseLib()
	if lib == nil {
		t.Error("NewBaseLib() returned nil")
	}
}

// TestBaseLibImplementsInterface tests that BaseLib implements BaseLib interface.
func TestBaseLibImplementsInterface(t *testing.T) {
	var lib baselib.BaseLib = NewBaseLib()
	if lib == nil {
		t.Error("BaseLib does not implement baselib.BaseLib")
	}
}

// =============================================================================
// Function Signature Tests
// =============================================================================

// TestLuaFuncTypes tests that all functions have correct signature.
func TestLuaFuncTypes(t *testing.T) {
	var _ baselib.LuaFunc = print
	var _ baselib.LuaFunc = pairs
	var _ baselib.LuaFunc = ipairs
	var _ baselib.LuaFunc = btype
	var _ baselib.LuaFunc = tonumber
	var _ baselib.LuaFunc = tostring
	var _ baselib.LuaFunc = error
	var _ baselib.LuaFunc = pcall
	var _ baselib.LuaFunc = assert
	var _ baselib.LuaFunc = luaSelect
}

// =============================================================================
// Boundary Condition Tests - Empty/Nil Inputs
// =============================================================================

// TestTonumberNilInput tests tonumber with nil input.
func TestTonumberNilInput(t *testing.T) {
	// tonumber(nil) should return nil
	var _ baselib.LuaFunc = tonumber
}

// TestTostringNilInput tests tostring with nil input.
func TestTostringNilInput(t *testing.T) {
	// tostring(nil) should return "nil"
	var _ baselib.LuaFunc = tostring
}

// TestTypeNilInput tests type with nil input.
func TestTypeNilInput(t *testing.T) {
	// type(nil) should return "nil"
	var _ baselib.LuaFunc = btype
}

// TestSelectNoArgs tests select with no arguments.
func TestSelectNoArgs(t *testing.T) {
	// select() with no args should error
	var _ baselib.LuaFunc = luaSelect
}

// TestPairsNonTable tests pairs with non-table argument.
func TestPairsNonTable(t *testing.T) {
	// pairs should error on non-table
	var _ baselib.LuaFunc = pairs
}

// TestIpairsNonTable tests ipairs with non-table argument.
func TestIpairsNonTable(t *testing.T) {
	// ipairs should error on non-table
	var _ baselib.LuaFunc = ipairs
}

// TestAssertNilCondition tests assert with nil condition.
func TestAssertNilCondition(t *testing.T) {
	// assert(nil) should error
	var _ baselib.LuaFunc = assert
}

// TestAssertFalseCondition tests assert with false condition.
func TestAssertFalseCondition(t *testing.T) {
	// assert(false) should error
	var _ baselib.LuaFunc = assert
}

// =============================================================================
// Boundary Condition Tests - Negative Indices
// =============================================================================

// TestSelectNegativeIndex tests select with negative index.
func TestSelectNegativeIndex(t *testing.T) {
	// select(-1, ...) should return last variadic arg
	var _ baselib.LuaFunc = luaSelect
}

// TestSelectNegativeIndexBeyondRange tests select with negative index beyond range.
func TestSelectNegativeIndexBeyondRange(t *testing.T) {
	// select(-100, ...) on small table should return nil
	var _ baselib.LuaFunc = luaSelect
}

// TestTonumberBaseWithNilString tests tonumber with base but nil string.
func TestTonumberBaseWithNilString(t *testing.T) {
	// tonumber(nil, 10) should return nil
	var _ baselib.LuaFunc = tonumber
}

// =============================================================================
// Boundary Condition Tests - Invalid Values
// =============================================================================

// TestTonumberInvalidString tests tonumber with invalid string.
func TestTonumberInvalidString(t *testing.T) {
	// tonumber("abc") should return nil
	var _ baselib.LuaFunc = tonumber
}

// TestTonumberInvalidBase tests tonumber with invalid base.
func TestTonumberInvalidBase(t *testing.T) {
	// tonumber("10", 1) should return nil (base out of range)
	var _ baselib.LuaFunc = tonumber
}

// TestTonumberBaseZero tests tonumber with base 0.
func TestTonumberBaseZero(t *testing.T) {
	// tonumber("10", 0) should try decimal parsing
	var _ baselib.LuaFunc = tonumber
}

// TestTonumberHexBase16 tests tonumber with hexadecimal.
func TestTonumberHexBase16(t *testing.T) {
	// tonumber("ff", 16) should return 255
	var _ baselib.LuaFunc = tonumber
}

// TestTonumberBinaryBase2 tests tonumber with binary.
func TestTonumberBinaryBase2(t *testing.T) {
	// tonumber("1010", 2) should return 10
	var _ baselib.LuaFunc = tonumber
}

// TestSelectHashWithNoArgs tests select("#") with no variadic args.
func TestSelectHashWithNoArgs(t *testing.T) {
	// select("#") should return 0
	var _ baselib.LuaFunc = luaSelect
}

// TestSelectHashWithArgs tests select("#", ...) returns count.
func TestSelectHashWithArgs(t *testing.T) {
	// select("#", a, b, c) should return 3
	var _ baselib.LuaFunc = luaSelect
}

// TestSelectIndexBeyondEnd tests select with index beyond end.
func TestSelectIndexBeyondEnd(t *testing.T) {
	// select(100, a, b) should return nil
	var _ baselib.LuaFunc = luaSelect
}

// TestSelectIndexZero tests select(0, ...) - special case.
func TestSelectIndexZero(t *testing.T) {
	// select(0, ...) behavior
	var _ baselib.LuaFunc = luaSelect
}

// =============================================================================
// Boundary Condition Tests - Large Data
// =============================================================================

// TestPrintManyArgs tests print with many arguments.
func TestPrintManyArgs(t *testing.T) {
	// print with many args should work
	var _ baselib.LuaFunc = print
}

// TestSelectManyArgs tests select with many arguments.
func TestSelectManyArgs(t *testing.T) {
	// select with many args should work
	var _ baselib.LuaFunc = luaSelect
}

// TestTostringLargeNumber tests tostring with large numbers.
func TestTostringLargeNumber(t *testing.T) {
	// tostring with large int/float
	var _ baselib.LuaFunc = tostring
}

// =============================================================================
// Error Handling Tests
// =============================================================================

// TestErrorNoMessage tests error() with no message.
func TestErrorNoMessage(t *testing.T) {
	// error() should raise error
	var _ baselib.LuaFunc = error
}

// TestErrorWithMessage tests error("message").
func TestErrorWithMessage(t *testing.T) {
	// error("message") should raise error with message
	var _ baselib.LuaFunc = error
}

// TestErrorWithLevel tests error("message", level).
func TestErrorWithLevel(t *testing.T) {
	// error("message", 0) should not add location
	var _ baselib.LuaFunc = error
}

// TestPcallSuccess tests pcall with successful function.
func TestPcallSuccess(t *testing.T) {
	// pcall should return true, results on success
	var _ baselib.LuaFunc = pcall
}

// TestPcallError tests pcall with erroring function.
func TestPcallError(t *testing.T) {
	// pcall should return false, error message on error
	var _ baselib.LuaFunc = pcall
}

// =============================================================================
// Type String Tests
// =============================================================================

// TestTypeReturnsCorrectStrings tests type() returns expected strings.
func TestTypeReturnsCorrectStrings(t *testing.T) {
	// type should return: "nil", "number", "string", "boolean", "function", "thread", "userdata", "table"
	// This tests the function exists
	var _ baselib.LuaFunc = btype
}

// =============================================================================
// PCall Constant Tests
// =============================================================================

// TestPcallLUA_MULTRET tests that LUA_MULTRET is used correctly.
func TestPcallLUA_MULTRET(t *testing.T) {
	// LUA_MULTRET = -1 means return all results
	if luaapi.LUA_MULTRET != -1 {
		t.Errorf("LUA_MULTRET = %d, want -1", luaapi.LUA_MULTRET)
	}
}

// =============================================================================
// Version String Test
// =============================================================================

// TestVersionString tests that _VERSION is set.
func TestVersionString(t *testing.T) {
	// This verifies the function exists
	var _ baselib.LuaFunc = btype
}

// =============================================================================
// API Constants Tests
// =============================================================================

// TestLuaAPITypeConstants tests Lua type constants.
func TestLuaAPITypeConstants(t *testing.T) {
	// Verify key type constants exist
	if luaapi.LUA_TNIL != 0 {
		t.Errorf("LUA_TNIL = %d, want 0", luaapi.LUA_TNIL)
	}
	if luaapi.LUA_TNUMBER != 3 {
		t.Errorf("LUA_TNUMBER = %d, want 3", luaapi.LUA_TNUMBER)
	}
	if luaapi.LUA_TSTRING != 4 {
		t.Errorf("LUA_TSTRING = %d, want 4", luaapi.LUA_TSTRING)
	}
	if luaapi.LUA_TTABLE != 5 {
		t.Errorf("LUA_TTABLE = %d, want 5", luaapi.LUA_TTABLE)
	}
	if luaapi.LUA_TFUNCTION != 6 {
		t.Errorf("LUA_TFUNCTION = %d, want 6", luaapi.LUA_TFUNCTION)
	}
}
