// Package api defines the public Go API for the Lua VM.
//
// This is the Go equivalent of C's lua_* functions (lapi.c) and
// luaL_* auxiliary functions (lauxlib.c).
// It provides a stack-based API for manipulating Lua state from Go.
// All standard library implementations use this API.
//
// Reference: .analysis/09-standard-libraries.md §1-§2
package api

import (
	objectapi "github.com/akzj/go-lua/internal/object/api"
)

// CFunction is the type for Go functions callable from Lua.
// It receives the Lua state and returns the number of results pushed.
type CFunction func(L *State) int

// State is the public Lua state handle.
// It wraps the internal LuaState and provides the stack-based API.
type State struct {
	// Internal fields set during construction — not exported.
	// The implementation file will define these.
	Internal any // *stateapi.LuaState (avoids circular import)
}

// --- Pseudo-Indices ---

const (
	RegistryIndex = -1000000 - 1000 // LUA_REGISTRYINDEX
)

// UpvalueIndex returns the pseudo-index for upvalue i (1-based).
func UpvalueIndex(i int) int {
	return RegistryIndex - i
}

// Registry keys for well-known values.
const (
	RIdxMainThread = 1 // LUA_RIDX_MAINTHREAD
	RIdxGlobals    = 2 // LUA_RIDX_GLOBALS — the _G table
)

// --- State Creation ---

// NewState creates a new Lua state with all standard libraries loaded.
// Returns nil on allocation failure.
func NewState() *State { return nil } // placeholder

// Close releases all resources associated with the state.
func (L *State) Close() {}

// --- Stack Manipulation ---

// GetTop returns the index of the top element (= number of elements).
func (L *State) GetTop() int { return 0 }

// SetTop sets the stack top to idx. If new top > old top, fills with nil.
// If idx < 0, sets relative to current top.
func (L *State) SetTop(idx int) {}

// CheckStack ensures at least n free stack slots. Returns false on failure.
func (L *State) CheckStack(n int) bool { return false }

// AbsIndex converts a possibly-negative index to an absolute index.
// Pseudo-indices are returned as-is.
func (L *State) AbsIndex(idx int) int { return 0 }

// Pop removes n elements from the top of the stack.
func (L *State) Pop(n int) {}

// Copy copies the value at fromIdx to toIdx without removing.
func (L *State) Copy(fromIdx, toIdx int) {}

// Rotate rotates the stack elements between idx and top by n positions.
func (L *State) Rotate(idx, n int) {}

// Insert moves top element to idx, shifting elements up.
func (L *State) Insert(idx int) {}

// Remove removes the element at idx, shifting elements down.
func (L *State) Remove(idx int) {}

// Replace replaces the value at idx with the top element, popping top.
func (L *State) Replace(idx int) {}

// --- Push Functions (Go → Lua Stack) ---

// PushNil pushes a nil value.
func (L *State) PushNil() {}

// PushBoolean pushes a boolean value.
func (L *State) PushBoolean(b bool) {}

// PushInteger pushes an integer value.
func (L *State) PushInteger(n int64) {}

// PushNumber pushes a float value.
func (L *State) PushNumber(n float64) {}

// PushString pushes a string value. Returns the interned string.
func (L *State) PushString(s string) string { return "" }

// PushFString pushes a formatted string (like fmt.Sprintf).
func (L *State) PushFString(format string, args ...interface{}) string { return "" }

// PushCFunction pushes a Go function as a light C function (no upvalues).
func (L *State) PushCFunction(f CFunction) {}

// PushCClosure pushes a Go function as a closure with n upvalues.
// The n upvalues are popped from the stack.
func (L *State) PushCClosure(f CFunction, n int) {}

// PushValue pushes a copy of the value at the given index.
func (L *State) PushValue(idx int) {}

// PushLightUserdata pushes a light userdata (raw Go interface{}).
func (L *State) PushLightUserdata(p interface{}) {}

// PushGlobalTable pushes the global table onto the stack.
func (L *State) PushGlobalTable() {}

// --- Type Checking ---

// Type returns the type of the value at the given index.
func (L *State) Type(idx int) objectapi.Type { return 0 }

// TypeName returns the name of the given type.
func (L *State) TypeName(tp objectapi.Type) string { return "" }

// IsNil returns true if the value at idx is nil.
func (L *State) IsNil(idx int) bool { return false }

// IsNone returns true if the index is not valid.
func (L *State) IsNone(idx int) bool { return false }

// IsNoneOrNil returns true if the index is not valid or the value is nil.
func (L *State) IsNoneOrNil(idx int) bool { return false }

// IsBoolean returns true if the value is a boolean.
func (L *State) IsBoolean(idx int) bool { return false }

// IsInteger returns true if the value is an integer (NOT coercive).
func (L *State) IsInteger(idx int) bool { return false }

// IsNumber returns true if the value is a number or a convertible string (COERCIVE).
func (L *State) IsNumber(idx int) bool { return false }

// IsString returns true if the value is a string or a number (COERCIVE).
func (L *State) IsString(idx int) bool { return false }

// IsFunction returns true if the value is a function (Lua or Go).
func (L *State) IsFunction(idx int) bool { return false }

// IsTable returns true if the value is a table.
func (L *State) IsTable(idx int) bool { return false }

// IsCFunction returns true if the value is a Go function.
func (L *State) IsCFunction(idx int) bool { return false }

// IsUserdata returns true if the value is a userdata (full or light).
func (L *State) IsUserdata(idx int) bool { return false }

// --- Conversion Functions (Lua Stack → Go) ---

// ToBoolean converts the value to boolean. false and nil → false, all else → true.
func (L *State) ToBoolean(idx int) bool { return false }

// ToInteger converts the value to integer. Returns (value, ok).
func (L *State) ToInteger(idx int) (int64, bool) { return 0, false }

// ToNumber converts the value to float. Returns (value, ok).
func (L *State) ToNumber(idx int) (float64, bool) { return 0, false }

// ToString converts the value to string. Returns (value, ok).
// WARNING: Coerces numbers to strings IN-PLACE on the stack.
func (L *State) ToString(idx int) (string, bool) { return "", false }

// ToGoFunction returns the Go function at idx, or nil.
func (L *State) ToGoFunction(idx int) CFunction { return nil }

// RawLen returns the raw length (no __len metamethod).
func (L *State) RawLen(idx int) int64 { return 0 }

// --- Table Operations ---

// GetTable pushes t[k] where t is at idx and k is at top. Pops k.
func (L *State) GetTable(idx int) objectapi.Type { return 0 }

// GetField pushes t[key] where t is at idx.
func (L *State) GetField(idx int, key string) objectapi.Type { return 0 }

// GetI pushes t[n] where t is at idx.
func (L *State) GetI(idx int, n int64) objectapi.Type { return 0 }

// GetGlobal pushes the value of the global variable name.
func (L *State) GetGlobal(name string) objectapi.Type { return 0 }

// SetTable does t[k] = v where t is at idx, k at top-1, v at top. Pops k+v.
func (L *State) SetTable(idx int) {}

// SetField does t[key] = v where t is at idx, v at top. Pops v.
func (L *State) SetField(idx int, key string) {}

// SetI does t[n] = v where t is at idx, v at top. Pops v.
func (L *State) SetI(idx int, n int64) {}

// SetGlobal pops a value and sets it as the global variable name.
func (L *State) SetGlobal(name string) {}

// RawGet pushes t[k] without metamethods. t at idx, k at top. Pops k.
func (L *State) RawGet(idx int) objectapi.Type { return 0 }

// RawGetI pushes t[n] without metamethods.
func (L *State) RawGetI(idx int, n int64) objectapi.Type { return 0 }

// RawSet does t[k] = v without metamethods. Pops k+v.
func (L *State) RawSet(idx int) {}

// RawSetI does t[n] = v without metamethods. Pops v.
func (L *State) RawSetI(idx int, n int64) {}

// CreateTable pushes a new table with pre-allocated space.
func (L *State) CreateTable(nArr, nRec int) {}

// NewTable pushes a new empty table.
func (L *State) NewTable() {}

// GetMetatable pushes the metatable of the value at idx. Returns false if none.
func (L *State) GetMetatable(idx int) bool { return false }

// SetMetatable pops a table and sets it as metatable for value at idx.
func (L *State) SetMetatable(idx int) {}

// Next implements table traversal. Pops key, pushes next key+value.
func (L *State) Next(idx int) bool { return false }

// Len pushes the length of the value at idx (may invoke __len).
func (L *State) Len(idx int) {}

// RawEqual compares two values without metamethods.
func (L *State) RawEqual(idx1, idx2 int) bool { return false }

// Compare compares two values. op: OpEQ, OpLT, OpLE.
func (L *State) Compare(idx1, idx2 int, op CompareOp) bool { return false }

// CompareOp is the comparison operation type.
type CompareOp int

const (
	OpEQ CompareOp = 0 // ==
	OpLT CompareOp = 1 // <
	OpLE CompareOp = 2 // <=
)

// Concat concatenates the n values at the top of the stack.
func (L *State) Concat(n int) {}

// Arith performs an arithmetic operation.
func (L *State) Arith(op ArithOp) {}

// ArithOp is the arithmetic operation type.
type ArithOp int

const (
	OpAdd  ArithOp = 0
	OpSub  ArithOp = 1
	OpMul  ArithOp = 2
	OpMod  ArithOp = 3
	OpPow  ArithOp = 4
	OpDiv  ArithOp = 5
	OpIDiv ArithOp = 6
	OpBAnd ArithOp = 7
	OpBOr  ArithOp = 8
	OpBXor ArithOp = 9
	OpShl  ArithOp = 10
	OpShr  ArithOp = 11
	OpUnm  ArithOp = 12
	OpBNot ArithOp = 13
)

// --- Call/Load Functions ---

// Call calls a function. nArgs arguments are on the stack above the function.
func (L *State) Call(nArgs, nResults int) {}

// PCall performs a protected call. Returns status code.
func (L *State) PCall(nArgs, nResults, msgHandler int) int { return 0 }

// Load loads a Lua chunk from a string. Pushes the compiled function.
func (L *State) Load(code string, name string, mode string) int { return 0 }

// DoString loads and executes a string.
func (L *State) DoString(code string) error { return nil }

// DoFile loads and executes a file.
func (L *State) DoFile(filename string) error { return nil }

// --- Error Functions ---

// Error raises a Lua error with the value at the top of the stack.
func (L *State) Error() int { return 0 }

// --- Userdata ---

// NewUserdata creates a new full userdata with nUV user values.
func (L *State) NewUserdata(size int, nUV int) interface{} { return nil }

// --- Upvalue Access ---

// GetUpvalue pushes the value of upvalue n of the closure at funcIdx.
func (L *State) GetUpvalue(funcIdx, n int) string { return "" }

// SetUpvalue sets upvalue n of the closure at funcIdx from the top value.
func (L *State) SetUpvalue(funcIdx, n int) string { return "" }

// --- GC ---

// GC performs a garbage collection operation.
func (L *State) GC(what GCWhat, args ...int) int { return 0 }

// GCWhat is the GC operation type.
type GCWhat int

const (
	GCStop      GCWhat = 0
	GCRestart   GCWhat = 1
	GCCollect   GCWhat = 2
	GCCount     GCWhat = 3
	GCCountB    GCWhat = 4
	GCStep      GCWhat = 5
	GCIsRunning GCWhat = 9
	GCGen       GCWhat = 10
	GCInc       GCWhat = 11
)

// --- Auxiliary Functions (luaL_*) ---

// CheckString checks that argument at idx is a string and returns it.
func (L *State) CheckString(idx int) string { return "" }

// CheckInteger checks that argument at idx is an integer and returns it.
func (L *State) CheckInteger(idx int) int64 { return 0 }

// CheckNumber checks that argument at idx is a number and returns it.
func (L *State) CheckNumber(idx int) float64 { return 0 }

// CheckType checks that argument at idx has the given type.
func (L *State) CheckType(idx int, tp objectapi.Type) {}

// CheckAny checks that there is an argument at idx.
func (L *State) CheckAny(idx int) {}

// OptString returns the string at idx, or def if nil/none.
func (L *State) OptString(idx int, def string) string { return "" }

// OptInteger returns the integer at idx, or def if nil/none.
func (L *State) OptInteger(idx int, def int64) int64 { return 0 }

// OptNumber returns the number at idx, or def if nil/none.
func (L *State) OptNumber(idx int, def float64) float64 { return 0 }

// ArgError raises an error for argument arg with extra message.
func (L *State) ArgError(arg int, extraMsg string) int { return 0 }

// TypeError raises a type error for argument arg.
func (L *State) TypeError(arg int, tname string) int { return 0 }

// Where pushes "source:line: " for the given call level.
func (L *State) Where(level int) {}

// Errorf raises a formatted error.
func (L *State) Errorf(format string, args ...interface{}) int { return 0 }

// SetFuncs registers functions from a map into the table at the top of stack.
func (L *State) SetFuncs(funcs map[string]CFunction, nUp int) {}

// Require calls openf to load a module, stores in package.loaded.
func (L *State) Require(modname string, openf CFunction, global bool) {}

// NewLib creates a new table and registers functions into it.
func (L *State) NewLib(funcs map[string]CFunction) {}

// Ref creates a reference in the table at idx. Pops value.
func (L *State) Ref(idx int) int { return 0 }

// Unref frees a reference in the table at idx.
func (L *State) Unref(idx int, ref int) {}

// RefNil is the reference value for nil.
const RefNil = -1

// NoRef is the "no reference" value.
const NoRef = -2

// --- Debug Interface ---

// GetStack fills a DebugInfo for the given call level.
func (L *State) GetStack(level int) (*DebugInfo, bool) { return nil, false }

// GetInfo fills debug info fields specified by what string.
func (L *State) GetInfo(what string, ar *DebugInfo) bool { return false }

// DebugInfo holds debug information about a function activation.
type DebugInfo struct {
	Source          string // source of the chunk
	ShortSrc        string // short source (for error messages)
	LineDefined     int    // line where definition starts
	LastLineDefined int    // line where definition ends
	CurrentLine     int    // current line
	Name            string // function name (if known)
	NameWhat        string // "global", "local", "method", "field", ""
	What            string // "Lua", "C", "main"
	NUps            int    // number of upvalues
	NParams         int    // number of parameters
	IsVararg        bool   // is a vararg function
}

// --- Debug Hook API (I8 FIX) ---

// SetHook sets the debug hook function with the given mask and count.
func (L *State) SetHook(f interface{}, mask, count int) {}

// GetHook returns the current debug hook function.
func (L *State) GetHook() interface{} { return nil }

// GetHookMask returns the current hook mask.
func (L *State) GetHookMask() int { return 0 }

// GetHookCount returns the current hook count.
func (L *State) GetHookCount() int { return 0 }

// GetLocal gets the value of a local variable. Returns name or "" if invalid.
func (L *State) GetLocal(ar *DebugInfo, n int) string { return "" }

// SetLocal sets the value of a local variable. Returns name or "" if invalid.
func (L *State) SetLocal(ar *DebugInfo, n int) string { return "" }

// --- Coroutine API (C9 FIX) ---

// NewThread creates a new Lua thread (coroutine) sharing the same global state.
func (L *State) NewThread() *State { return nil }

// PushThread pushes the thread onto its own stack. Returns true if main thread.
func (L *State) PushThread() bool { return false }

// Resume starts or resumes a coroutine. Returns (status, true if resumable).
func (L *State) Resume(from *State, nArgs int) (int, bool) { return 0, false }

// YieldK yields a coroutine with a continuation.
func (L *State) YieldK(nResults int, ctx int, k CFunction) int { return 0 }

// Yield yields a coroutine (no continuation).
func (L *State) Yield(nResults int) int { return 0 }

// IsYieldable returns true if the running coroutine can yield.
func (L *State) IsYieldable() bool { return false }

// XMove moves n values between two threads sharing the same global state.
func (L *State) XMove(to *State, n int) {}

// ToThread converts the value at idx to a State (thread). Returns nil if not a thread.
func (L *State) ToThread(idx int) *State { return nil }

// --- Userdata API (I9 FIX) ---

// ToUserdata returns the userdata value at idx, or nil.
func (L *State) ToUserdata(idx int) interface{} { return nil }

// GetIUserValue pushes the n-th user value of the userdata at idx.
// Returns the type of the pushed value.
func (L *State) GetIUserValue(idx int, n int) objectapi.Type { return 0 }

// SetIUserValue sets the n-th user value of the userdata at idx from top. Pops value.
// Returns true if the userdata has that user value.
func (L *State) SetIUserValue(idx int, n int) bool { return false }

// ToPointer returns a generic pointer representation of the value at idx.
func (L *State) ToPointer(idx int) interface{} { return nil }

