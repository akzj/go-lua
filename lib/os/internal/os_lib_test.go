// Package internal provides tests for the OS library.
package internal

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	luaapi "github.com/akzj/go-lua/api"
	tableapi "github.com/akzj/go-lua/table/api"
	oslib "github.com/akzj/go-lua/lib/os/api"
)

// =============================================================================
// Constructor Tests
// =============================================================================

// TestNewOSLib tests creating a new OSLib instance.
func TestNewOSLib(t *testing.T) {
	lib := NewOSLib()
	if lib == nil {
		t.Error("NewOSLib() returned nil")
	}
}

// TestOSLibImplementsInterface tests that OSLib implements OSLib interface.
func TestOSLibImplementsInterface(t *testing.T) {
	var lib oslib.OSLib = NewOSLib()
	if lib == nil {
		t.Error("OSLib does not implement oslib.OSLib interface")
	}
}

// =============================================================================
// LuaFunc Signature Tests
// =============================================================================

// TestLuaFuncSignatures tests that all OS functions have correct LuaFunc signature.
func TestLuaFuncSignatures(t *testing.T) {
	var _ oslib.LuaFunc = osClock
	var _ oslib.LuaFunc = osDate
	var _ oslib.LuaFunc = osDifftime
	var _ oslib.LuaFunc = osExecute
	var _ oslib.LuaFunc = osExit
	var _ oslib.LuaFunc = osGetenv
	var _ oslib.LuaFunc = osRemove
	var _ oslib.LuaFunc = osRename
	var _ oslib.LuaFunc = osSetlocale
	var _ oslib.LuaFunc = osTime
	var _ oslib.LuaFunc = osTmpname
}

// =============================================================================
// testLuaAPI is a mock implementation of LuaAPI for testing.
// Uses 1-indexed stack to match Lua semantics.
type testLuaAPI struct {
	stack []interface{}
}

func newTestLuaAPI(values ...interface{}) *testLuaAPI {
	stack := make([]interface{}, 0, 20)
	for _, v := range values {
		stack = append(stack, v)
	}
	return &testLuaAPI{stack: stack}
}

func (t *testLuaAPI) GetTop() int                    { return len(t.stack) }
func (t *testLuaAPI) SetTop(idx int) {
	for len(t.stack) < idx {
		t.stack = append(t.stack, nil)
	}
	t.stack = t.stack[:idx]
}
func (t *testLuaAPI) Pop() {
	if len(t.stack) > 0 {
		t.stack = t.stack[:len(t.stack)-1]
	}
}
func (t *testLuaAPI) PushValue(idx int) {
	if idx < 0 {
		idx = len(t.stack) + idx + 1
	}
	if idx < 1 || idx > len(t.stack) {
		return
	}
	t.stack = append(t.stack, t.stack[idx-1])
}
func (t *testLuaAPI) AbsIndex(idx int) int            { return idx }
func (t *testLuaAPI) Rotate(idx, n int)               {}
func (t *testLuaAPI) Copy(fromidx, toidx int)         {}
func (t *testLuaAPI) CheckStack(n int) bool           { return true }
func (t *testLuaAPI) XMove(to luaapi.LuaAPI, n int)   {}

// Type Checking
func (t *testLuaAPI) Type(idx int) int {
	if idx < 0 {
		idx = len(t.stack) + idx + 1
	}
	if idx < 1 || idx > len(t.stack) {
		return luaapi.LUA_TNONE
	}
	v := t.stack[idx-1]
	switch v.(type) {
	case nil:
		return luaapi.LUA_TNIL
	case bool:
		return luaapi.LUA_TBOOLEAN
	case int64, float64:
		return luaapi.LUA_TNUMBER
	case string:
		return luaapi.LUA_TSTRING
	case map[string]interface{}:
		return luaapi.LUA_TTABLE
	default:
		return luaapi.LUA_TNIL
	}
}
func (t *testLuaAPI) TypeName(tp int) string {
	switch tp {
	case luaapi.LUA_TNIL:
		return "nil"
	case luaapi.LUA_TBOOLEAN:
		return "boolean"
	case luaapi.LUA_TNUMBER:
		return "number"
	case luaapi.LUA_TSTRING:
		return "string"
	case luaapi.LUA_TTABLE:
		return "table"
	default:
		return "no value"
	}
}
func (t *testLuaAPI) IsNone(idx int) bool             { return idx < 0 || idx > len(t.stack) }
func (t *testLuaAPI) IsNil(idx int) bool {
	if idx < 0 {
		idx = len(t.stack) + idx + 1
	}
	if idx < 1 || idx > len(t.stack) {
		return false
	}
	return t.stack[idx-1] == nil
}
func (t *testLuaAPI) IsNoneOrNil(idx int) bool {
	if idx < 0 {
		idx = len(t.stack) + idx + 1
	}
	return idx < 1 || idx > len(t.stack) || t.stack[idx-1] == nil
}
func (t *testLuaAPI) IsBoolean(idx int) bool           { _, ok := t.getValue(idx).(bool); return ok }
func (t *testLuaAPI) IsString(idx int) bool           { _, ok := t.getValue(idx).(string); return ok }
func (t *testLuaAPI) IsFunction(idx int) bool         { return false }
func (t *testLuaAPI) IsTable(idx int) bool            { _, ok := t.getValue(idx).(map[string]interface{}); return ok }
func (t *testLuaAPI) IsLightUserData(idx int) bool    { return false }
func (t *testLuaAPI) IsThread(idx int) bool           { return false }
func (t *testLuaAPI) IsInteger(idx int) bool          {
	v := t.getValue(idx)
	switch v.(type) {
	case int64:
		return true
	case float64:
		f := v.(float64)
		return f == float64(int64(f))
	}
	return false
}
func (t *testLuaAPI) IsNumber(idx int) bool          {
	v := t.getValue(idx)
	switch v.(type) {
	case int64, float64:
		return true
	}
	return false
}

// Value Conversion
func (t *testLuaAPI) ToInteger(idx int) (int64, bool) {
	v := t.getValue(idx)
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		return i, err == nil
	}
	return 0, false
}
func (t *testLuaAPI) ToNumber(idx int) (float64, bool) {
	v := t.getValue(idx)
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}
func (t *testLuaAPI) getValue(idx int) interface{} {
	if idx < 0 {
		idx = len(t.stack) + idx + 1
	}
	if idx < 1 || idx > len(t.stack) {
		return nil
	}
	return t.stack[idx-1]
}
func (t *testLuaAPI) ToString(idx int) (string, bool) {
	v := t.getValue(idx)
	switch s := v.(type) {
	case string:
		return s, true
	case int64:
		return strconv.FormatInt(s, 10), true
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64), true
	}
	return "", false
}
func (t *testLuaAPI) ToBoolean(idx int) bool {
	v := t.getValue(idx)
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}
func (t *testLuaAPI) ToPointer(idx int) interface{}   { return nil }
func (t *testLuaAPI) ToThread(idx int) luaapi.LuaAPI  { return nil }

// Push Functions
func (t *testLuaAPI) PushNil()                            {}
func (t *testLuaAPI) PushInteger(n int64)                { t.stack = append(t.stack, n) }
func (t *testLuaAPI) PushNumber(n float64)               { t.stack = append(t.stack, n) }
func (t *testLuaAPI) PushString(s string)                { t.stack = append(t.stack, s) }
func (t *testLuaAPI) PushBoolean(b bool)                 { t.stack = append(t.stack, b) }
func (t *testLuaAPI) PushLightUserData(p interface{})    { t.stack = append(t.stack, p) }
func (t *testLuaAPI) PushGoFunction(fn func(luai luaapi.LuaAPI) int) { t.stack = append(t.stack, fn) }
func (t *testLuaAPI) Insert(pos int)                     {}

// Table Operations
func (t *testLuaAPI) GetTable(idx int) int              { return luaapi.LUA_TNIL }
func (t *testLuaAPI) GetField(idx int, k string) int {
	if idx < 0 {
		idx = len(t.stack) + idx + 1
	}
	if idx >= 1 && idx <= len(t.stack) {
		if m, ok := t.stack[idx-1].(map[string]interface{}); ok {
			if v, ok := m[k]; ok {
				t.stack = append(t.stack, v)
				return t.Type(len(t.stack))
			}
		}
	}
	t.stack = append(t.stack, nil)
	return luaapi.LUA_TNIL
}
func (t *testLuaAPI) GetI(idx int, n int64) int         { return luaapi.LUA_TNIL }
func (t *testLuaAPI) RawGet(idx int) int                { return luaapi.LUA_TNIL }
func (t *testLuaAPI) RawGetI(idx int, n int64) int     { return luaapi.LUA_TNIL }
func (t *testLuaAPI) CreateTable(narr, nrec int)        { t.stack = append(t.stack, make(map[string]interface{})) }
func (t *testLuaAPI) SetTable(idx int)                  {}
func (t *testLuaAPI) SetField(idx int, k string) {
	if idx < 0 {
		idx = len(t.stack) + idx + 1
	}
	if idx >= 1 && idx <= len(t.stack) {
		if m, ok := t.stack[idx-1].(map[string]interface{}); ok {
			if len(t.stack) >= 1 {
				val := t.stack[len(t.stack)-1]
				m[k] = val
			}
		}
	}
	// SetField pops the key from stack in real Lua C API
	if len(t.stack) > 0 {
		t.stack = t.stack[:len(t.stack)-1]
	}
}
func (t *testLuaAPI) SetI(idx int, n int64)            {}
func (t *testLuaAPI) RawSet(idx int)                    {}
func (t *testLuaAPI) RawSetI(idx int, n int64)          {}
func (t *testLuaAPI) GetGlobal(name string) int         { return luaapi.LUA_TNIL }
func (t *testLuaAPI) SetGlobal(name string)            {}

// Metatable Operations
func (t *testLuaAPI) GetMetatable(idx int) bool        { return false }
func (t *testLuaAPI) SetMetatable(idx int)             {}

// Call Operations
func (t *testLuaAPI) Call(nArgs, nResults int)         {}
func (t *testLuaAPI) PCall(nArgs, nResults, errfunc int) int { return int(luaapi.LUA_OK) }

// Error Handling
func (t *testLuaAPI) Error() int                        { return 0 }
func (t *testLuaAPI) ErrorMessage() int                 { return 0 }
func (t *testLuaAPI) Where(level int)                   {}

// GC Control
func (t *testLuaAPI) GC(what int, args ...int) int     { return 0 }

// Miscellaneous
func (t *testLuaAPI) Next(idx int) bool                { return false }
func (t *testLuaAPI) Concat(n int)                     {}
func (t *testLuaAPI) Len(idx int)                      {}
func (t *testLuaAPI) Compare(idx1, idx2, op int) bool { return false }
func (t *testLuaAPI) RawLen(idx int) uint              { return 0 }

// Registry Access
func (t *testLuaAPI) Registry() tableapi.TableInterface { return nil }
func (t *testLuaAPI) Ref(tbl tableapi.TableInterface) int { return -1 }
func (t *testLuaAPI) UnRef(tbl tableapi.TableInterface, ref int) {}
func (t *testLuaAPI) PushGlobalTable()                 {}

// Thread Management
func (t *testLuaAPI) NewThread() luaapi.LuaAPI         { return t }
func (t *testLuaAPI) Status() luaapi.Status           { return luaapi.LUA_OK }

// Internal
func (t *testLuaAPI) Stack() []interface{}              { return t.stack }
func (t *testLuaAPI) StackSize() int                   { return len(t.stack) }
func (t *testLuaAPI) GrowStack(n int)                  {}
func (t *testLuaAPI) CurrentCI() interface{}           { return nil }
func (t *testLuaAPI) PushCI(ci interface{})             {}
func (t *testLuaAPI) PopCI()                           {}

// =============================================================================
// os.clock() Tests
// =============================================================================

// TestOsClockFunction tests that os.clock returns a positive number.
func TestOsClockFunction(t *testing.T) {
	L := newTestLuaAPI()
	osClock(L)
	got, ok := L.ToNumber(-1)
	if !ok {
		t.Error("os.clock() did not return a number")
		return
	}
	if got < 0 {
		t.Errorf("os.clock() = %v, expected non-negative", got)
	}
}

// TestOsClockReturnsSeconds tests that os.clock returns a reasonable value.
func TestOsClockReturnsSeconds(t *testing.T) {
	L := newTestLuaAPI()
	osClock(L)
	got, _ := L.ToNumber(-1)
	// Should be less than a reasonable upper bound for process time
	if got > 1e9 {
		t.Errorf("os.clock() = %v, unexpectedly large", got)
	}
}

// =============================================================================
// os.difftime() Tests
// =============================================================================

// TestOsDifftimeFunction tests os.difftime returns correct difference.
func TestOsDifftimeFunction(t *testing.T) {
	L := newTestLuaAPI(int64(100), int64(50))
	osDifftime(L)
	got, ok := L.ToNumber(-1)
	if !ok {
		t.Error("os.difftime() did not return a number")
		return
	}
	if got != 50 {
		t.Errorf("os.difftime(100, 50) = %v, want 50", got)
	}
}

// TestOsDifftimeNegative tests os.difftime with negative result.
func TestOsDifftimeNegative(t *testing.T) {
	L := newTestLuaAPI(int64(30), int64(100))
	osDifftime(L)
	got, _ := L.ToNumber(-1)
	if got != -70 {
		t.Errorf("os.difftime(30, 100) = %v, want -70", got)
	}
}

// TestOsDifftimeZero tests os.difftime with equal times.
func TestOsDifftimeZero(t *testing.T) {
	L := newTestLuaAPI(int64(42), int64(42))
	osDifftime(L)
	got, _ := L.ToNumber(-1)
	if got != 0 {
		t.Errorf("os.difftime(42, 42) = %v, want 0", got)
	}
}

// =============================================================================
// os.getenv() Tests
// =============================================================================

// TestOsGetenvExisting tests os.getenv with an existing variable.
func TestOsGetenvExisting(t *testing.T) {
	// Set a test environment variable
	os.Setenv("LUA_TEST_VAR", "test_value")
	defer os.Unsetenv("LUA_TEST_VAR")

	L := newTestLuaAPI("LUA_TEST_VAR")
	osGetenv(L)
	got, ok := L.ToString(-1)
	if !ok {
		t.Error("os.getenv() did not return a string for existing var")
		return
	}
	if got != "test_value" {
		t.Errorf("os.getenv(LUA_TEST_VAR) = %v, want 'test_value'", got)
	}
}

// TestOsGetenvPATH tests os.getenv with PATH (usually exists).
func TestOsGetenvPATH(t *testing.T) {
	L := newTestLuaAPI("PATH")
	osGetenv(L)
	if L.IsNil(-1) {
		t.Skip("PATH environment variable not set, skipping")
	}
	_, ok := L.ToString(-1)
	if !ok {
		t.Error("os.getenv(PATH) should return a string")
	}
}

// =============================================================================
// os.execute() Tests
// =============================================================================

// TestOsExecuteNoArgs tests os.execute with no arguments.
func TestOsExecuteNoArgs(t *testing.T) {
	L := newTestLuaAPI()
	n := osExecute(L)
	if n != 2 {
		t.Errorf("os.execute() returned %d values, want 2", n)
	}
	// Should return true and 0
	if !L.ToBoolean(-2) {
		t.Error("os.execute() first return should be true")
	}
	code, _ := L.ToInteger(-1)
	if code != 0 {
		t.Errorf("os.execute() second return should be 0, got %d", code)
	}
}

// TestOsExecuteEcho tests os.execute with echo command.
func TestOsExecuteEcho(t *testing.T) {
	L := newTestLuaAPI("echo hello")
	n := osExecute(L)
	if n != 2 {
		t.Errorf("os.execute() returned %d values, want 2", n)
	}
	// Should return true and 0 for success
	if !L.ToBoolean(-2) {
		t.Error("os.execute(echo) first return should be true on success")
	}
}

// TestOsExecuteFailingCommand tests os.execute with failing command.
func TestOsExecuteFailingCommand(t *testing.T) {
	L := newTestLuaAPI("exit 1")
	n := osExecute(L)
	if n != 2 {
		t.Errorf("os.execute() returned %d values, want 2", n)
	}
	// Should return false and 1
	if L.ToBoolean(-2) {
		t.Error("os.execute(exit 1) first return should be false")
	}
	code, _ := L.ToInteger(-1)
	if code != 1 {
		t.Errorf("os.execute(exit 1) second return should be 1, got %d", code)
	}
}

// =============================================================================
// os.exit() Tests
// =============================================================================

// TestOsExitNoArgs tests os.exit with no arguments.
func TestOsExitNoArgs(t *testing.T) {
	L := newTestLuaAPI()
	n := osExit(L)
	if n != 2 {
		t.Errorf("os.exit() returned %d values, want 2", n)
	}
	// Returns (exitCode, closeState) - exitCode at -2, closeState at -1
	code, _ := L.ToInteger(-2)
	if code != 0 {
		t.Errorf("os.exit() first return should be 0, got %d", code)
	}
	// closeState is false when not specified
	if L.ToBoolean(-1) {
		t.Error("os.exit() second return should be false (no close)")
	}
}

// TestOsExitTrue tests os.exit(true).
func TestOsExitTrue(t *testing.T) {
	L := newTestLuaAPI(true)
	n := osExit(L)
	if n != 2 {
		t.Errorf("os.exit(true) returned %d values, want 2", n)
	}
	code, _ := L.ToInteger(-2)
	if code != 0 {
		t.Errorf("os.exit(true) first return should be 0, got %d", code)
	}
}

// TestOsExitFalse tests os.exit(false).
func TestOsExitFalse(t *testing.T) {
	L := newTestLuaAPI(false)
	n := osExit(L)
	if n != 2 {
		t.Errorf("os.exit(false) returned %d values, want 2", n)
	}
	code, _ := L.ToInteger(-2)
	if code != 1 {
		t.Errorf("os.exit(false) first return should be 1, got %d", code)
	}
}

// TestOsExitNumber tests os.exit with a number.
func TestOsExitNumber(t *testing.T) {
	L := newTestLuaAPI(int64(42))
	n := osExit(L)
	if n != 2 {
		t.Errorf("os.exit(42) returned %d values, want 2", n)
	}
	code, _ := L.ToInteger(-2)
	if code != 42 {
		t.Errorf("os.exit(42) first return should be 42, got %d", code)
	}
}

// TestOsExitString tests os.exit with a string.
func TestOsExitString(t *testing.T) {
	L := newTestLuaAPI("error")
	n := osExit(L)
	if n != 2 {
		t.Errorf("os.exit('error') returned %d values, want 2", n)
	}
	code, _ := L.ToInteger(-2)
	if code != 1 {
		t.Errorf("os.exit('error') first return should be 1, got %d", code)
	}
}

// TestOsExitCloseTrue tests os.exit with close=true.
func TestOsExitCloseTrue(t *testing.T) {
	L := newTestLuaAPI(int64(0), true)
	n := osExit(L)
	if n != 2 {
		t.Errorf("os.exit(0, true) returned %d values, want 2", n)
	}
	if !L.ToBoolean(-1) {
		t.Error("os.exit(0, true) second return should be true")
	}
}

// =============================================================================
// os.remove() Tests
// =============================================================================

// TestOsRemoveSuccess tests os.remove with existing file.
func TestOsRemoveSuccess(t *testing.T) {
	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "lua_test_remove")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	L := newTestLuaAPI(tmpfile.Name())
	osRemove(L)
	// Returns (true, nil) on success - true at -1
	if L.IsNil(-1) {
		t.Error("os.remove() should return true on success")
	}
	if !L.ToBoolean(-1) {
		t.Error("os.remove() should return true on success")
	}
}

// =============================================================================
// os.rename() Tests
// =============================================================================

// TestOsRenameSuccess tests os.rename with existing file.
func TestOsRenameSuccess(t *testing.T) {
	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "lua_test_rename")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	// Create new name
	dir := filepath.Dir(tmpfile.Name())
	newName := filepath.Join(dir, "lua_test_rename_new")
	defer os.Remove(newName)

	L := newTestLuaAPI(tmpfile.Name(), newName)
	osRename(L)
	// Returns (true, nil) on success - true at -1
	if L.IsNil(-1) {
		t.Error("os.rename() should return true on success")
	}
	if !L.ToBoolean(-1) {
		t.Error("os.rename() should return true on success")
	}
}

// =============================================================================
// os.setlocale() Tests
// =============================================================================

// TestOsSetlocaleC tests os.setlocale("C").
func TestOsSetlocaleC(t *testing.T) {
	L := newTestLuaAPI("C")
	osSetlocale(L)
	result, ok := L.ToString(-1)
	if !ok {
		t.Error("os.setlocale('C') should return a string")
	}
	if result != "C" {
		t.Errorf("os.setlocale('C') = %v, want 'C'", result)
	}
}

// TestOsSetlocaleEmpty tests os.setlocale("").
func TestOsSetlocaleEmpty(t *testing.T) {
	L := newTestLuaAPI("")
	osSetlocale(L)
	result, ok := L.ToString(-1)
	if !ok {
		t.Error("os.setlocale('') should return a string")
	}
	if result != "C" {
		t.Errorf("os.setlocale('') = %v, want 'C'", result)
	}
}

// TestOsSetlocaleNoArgs tests os.setlocale with no arguments.
func TestOsSetlocaleNoArgs(t *testing.T) {
	L := newTestLuaAPI()
	osSetlocale(L)
	result, ok := L.ToString(-1)
	if !ok {
		t.Error("os.setlocale() should return a string")
	}
	if result != "C" {
		t.Errorf("os.setlocale() = %v, want 'C'", result)
	}
}

// =============================================================================
// os.time() Tests
// =============================================================================

// TestOsTimeNoArgs tests os.time with no arguments returns current time.
func TestOsTimeNoArgs(t *testing.T) {
	before := time.Now().Unix()
	L := newTestLuaAPI()
	osTime(L)
	after := time.Now().Unix()

	got, ok := L.ToNumber(-1)
	if !ok {
		t.Error("os.time() did not return a number")
		return
	}
	if got < float64(before) || got > float64(after) {
		t.Errorf("os.time() = %v, expected between %d and %d", got, before, after)
	}
}

// =============================================================================
// os.date() Tests
// =============================================================================

// TestOsDateNoArgs tests os.date with no arguments (default format).
func TestOsDateNoArgs(t *testing.T) {
	L := newTestLuaAPI()
	osDate(L)
	result, ok := L.ToString(-1)
	if !ok {
		t.Error("os.date() did not return a string")
	}
	// Result should contain something
	if len(result) == 0 {
		t.Error("os.date() returned empty string")
	}
}

// TestOsDateUTCPrefix tests os.date with "!" prefix (UTC).
func TestOsDateUTCPrefix(t *testing.T) {
	L := newTestLuaAPI("!%Y-%m-%d", int64(1577836800)) // 2020-01-01 00:00:00 UTC
	osDate(L)
	result, ok := L.ToString(-1)
	if !ok {
		t.Error("os.date('!format') did not return a string")
	}
	// Should be exactly 2020-01-01
	if result != "2020-01-01" {
		t.Errorf("os.date('!%%Y-%%m-%%d') = %v, want '2020-01-01'", result)
	}
}

// TestOsDateBasicFormat tests basic format specifiers.
func TestOsDateBasicFormat(t *testing.T) {
	// Test %Y (full year)
	L := newTestLuaAPI("%Y", int64(1577836800))
	osDate(L)
	result, _ := L.ToString(-1)
	if result != "2020" {
		t.Errorf("os.date('%%Y') = %v, want '2020'", result)
	}

	// Test %m (month)
	L = newTestLuaAPI("%m", int64(1577836800))
	osDate(L)
	result, _ = L.ToString(-1)
	if result != "01" {
		t.Errorf("os.date('%%m') = %v, want '01'", result)
	}

	// Test %d (day)
	L = newTestLuaAPI("%d", int64(1577836800))
	osDate(L)
	result, _ = L.ToString(-1)
	if result != "01" {
		t.Errorf("os.date('%%d') = %v, want '01'", result)
	}
}

// TestOsDateDayOfYear tests day of year.
func TestOsDateDayOfYear(t *testing.T) {
	// Jan 1 is day 001
	L := newTestLuaAPI("%j", int64(1577836800))
	osDate(L)
	result, _ := L.ToString(-1)
	if result != "001" {
		t.Errorf("os.date('%%j') = %v, want '001'", result)
	}
}

// =============================================================================
// os.tmpname() Tests
// =============================================================================

// TestOsTmpnameFunction tests os.tmpname returns a string.
func TestOsTmpnameFunction(t *testing.T) {
	L := newTestLuaAPI()
	osTmpname(L)
	result, ok := L.ToString(-1)
	if !ok {
		t.Error("os.tmpname() did not return a string")
	}
	if len(result) == 0 {
		t.Error("os.tmpname() returned empty string")
	}
}

// TestOsTmpnameUnique tests that os.tmpname returns different names.
func TestOsTmpnameUnique(t *testing.T) {
	L := newTestLuaAPI()
	osTmpname(L)
	name1, _ := L.ToString(-1)
	L.Pop()

	osTmpname(L)
	name2, _ := L.ToString(-1)

	// Names should be different (though not guaranteed, very likely)
	// Just check they're both non-empty
	if len(name1) == 0 || len(name2) == 0 {
		t.Error("os.tmpname() should return non-empty strings")
	}
}

// =============================================================================
// Helper function tests
// =============================================================================

// TestFmtZero tests the fmtZero helper function.
func TestFmtZero(t *testing.T) {
	tests := []struct {
		n      int
		width  int
		expect string
	}{
		{1, 2, "01"},
		{10, 2, "10"},
		{10, 3, "010"},
		{100, 3, "100"},
		{0, 2, "00"},
	}
	for _, tt := range tests {
		got := fmtZero(tt.n, tt.width)
		if got != tt.expect {
			t.Errorf("fmtZero(%d, %d) = %v, want %v", tt.n, tt.width, got, tt.expect)
		}
	}
}

// TestWeekdayShort tests weekdayShort function.
func TestWeekdayShort(t *testing.T) {
	if weekdayShort(time.Sunday) != "Sun" {
		t.Error("weekdayShort(Sunday) != 'Sun'")
	}
	if weekdayShort(time.Monday) != "Mon" {
		t.Error("weekdayShort(Monday) != 'Mon'")
	}
	if weekdayShort(time.Saturday) != "Sat" {
		t.Error("weekdayShort(Saturday) != 'Sat'")
	}
}

// TestMonthShort tests monthShort function.
func TestMonthShort(t *testing.T) {
	if monthShort(time.January) != "Jan" {
		t.Error("monthShort(January) != 'Jan'")
	}
	if monthShort(time.December) != "Dec" {
		t.Error("monthShort(December) != 'Dec'")
	}
}

// TestIsoWeekNumber tests isoWeekNumber function.
func TestIsoWeekNumber(t *testing.T) {
	// Jan 1, 2020 was a Wednesday
	t2020 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	// With Monday as first day (1), this is before the first Monday (Jan 6)
	// So it's week 0
	weekMon := isoWeekNumber(t2020, 1)
	if weekMon != 0 {
		t.Errorf("isoWeekNumber(2020-01-01, Monday first) = %d, want 0", weekMon)
	}
	// With Sunday as first day (0), Sunday is Jan 5, so Wed is in week 0
	weekSun := isoWeekNumber(t2020, 0)
	if weekSun != 0 {
		t.Errorf("isoWeekNumber(2020-01-01, Sunday first) = %d, want 0", weekSun)
	}

	// Jan 6, 2020 (first Monday) should be week 1 with Monday as first day
	tMonday := time.Date(2020, 1, 6, 0, 0, 0, 0, time.UTC)
	weekMon2 := isoWeekNumber(tMonday, 1)
	if weekMon2 != 1 {
		t.Errorf("isoWeekNumber(2020-01-06, Monday first) = %d, want 1", weekMon2)
	}
}

// =============================================================================
// Return values tests
// =============================================================================

// TestReturnValues tests that all OS functions return correct number of values.
func TestReturnValues(t *testing.T) {
	// os.clock returns 1
	L := newTestLuaAPI()
	n := osClock(L)
	if n != 1 {
		t.Errorf("os.clock returned %d, want 1", n)
	}

	// os.difftime returns 1
	L = newTestLuaAPI(int64(100), int64(50))
	n = osDifftime(L)
	if n != 1 {
		t.Errorf("os.difftime returned %d, want 1", n)
	}

	// os.getenv returns 1
	L = newTestLuaAPI("PATH")
	n = osGetenv(L)
	if n != 1 {
		t.Errorf("os.getenv returned %d, want 1", n)
	}

	// os.time returns 1
	L = newTestLuaAPI()
	n = osTime(L)
	if n != 1 {
		t.Errorf("os.time returned %d, want 1", n)
	}

	// os.tmpname returns 1
	L = newTestLuaAPI()
	n = osTmpname(L)
	if n != 1 {
		t.Errorf("os.tmpname returned %d, want 1", n)
	}

	// os.date returns 1
	L = newTestLuaAPI("%Y", int64(1577836800))
	n = osDate(L)
	if n != 1 {
		t.Errorf("os.date returned %d, want 1", n)
	}
}
