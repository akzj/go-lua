// Package api defines the interface for the Lua VM execution engine.
//
// This package merges the responsibilities of C's lvm.c (execution loop)
// and ldo.c (call/return/error handling) because they are mutually recursive.
// In Go, this avoids circular imports while matching the C reality.
//
// All shared types (LuaError, Status, CIST_* flags, CallInfo, LuaState)
// are defined in state/api. This package only defines VM-specific signatures.
//
// Reference: .analysis/05-vm-execution-loop.md, .analysis/04-call-return-error.md
package vm

// ---------------------------------------------------------------------------
// VM function signatures (implemented in internal/vm/)
//
// These are documented here as the public contract. The actual implementations
// will be package-level functions in internal/vm/.
//
// Core execution:
//   func Execute(L *state.LuaState)
//     Runs the VM main loop for the current CallInfo.
//     Handles Lua→Lua calls internally via trampoline (no recursion).
//     Returns only when hitting a CIST_Fresh boundary or error.
//
// Call/return:
//   func Call(L *state.LuaState, funcIdx int, nResults int)
//     Performs a function call. Equivalent to luaD_call.
//
//   func CallNoYield(L *state.LuaState, funcIdx int, nResults int)
//     Performs a non-yieldable function call. Equivalent to luaD_callnoyield.
//
//   func PreCall(L *state.LuaState, funcIdx int, nResults int) *state.CallInfo
//     Prepares a function call. For C functions: executes and returns nil.
//     For Lua functions: creates CallInfo and returns it.
//
//   func PosCall(L *state.LuaState, ci *state.CallInfo, nResults int)
//     Post-call cleanup: moves results, adjusts top, unwinds CI.
//
// Protected calls:
//   func PCall(L *state.LuaState, funcIdx int, nResults int, errFunc int) int
//     Protected function call. Returns status code.
//
// Error handling:
//   func Throw(L *state.LuaState, status int)
//     Raises a Lua error via panic(state.LuaError{...}).
//
// Stack management:
//   func GrowStack(L *state.LuaState, n int)
//     Ensures n free stack slots.
//
// Coroutine support:
//   func Resume(L *state.LuaState, from *state.LuaState, nArgs int) int
//     Resumes a coroutine.
//
//   func Yield(L *state.LuaState, nResults int) int
//     Yields a coroutine.
// ---------------------------------------------------------------------------
