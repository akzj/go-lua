// Package state provides Lua state management and execution entry points.
// This file implements DoString for executing Lua code.
//
// Pipeline: Lex → Parse → Compile → Closure → Call
//
// Contract:
// - DoString(code) → error: parses and executes Lua code
// - DoStringOn(L, code) → error: executes on specific state
// - Require(name) → error: loads Lua module
package state

import (
	"fmt"

	bc "github.com/akzj/go-lua/bytecode"
	parse "github.com/akzj/go-lua/parse"
	"github.com/akzj/go-lua/state/internal"
	stateapi "github.com/akzj/go-lua/state/api"
)

// DoString parses and executes Lua code string.
// Returns error on parse/compile failure.
//
// Invariant: If DoString returns nil, code executed successfully.
// If error is returned, no side effects occurred.
func DoString(code string) error {
	return DoStringOn(New(), code)
}

// DoStringOn executes Lua code on a specific state.
// Allows reusing existing state with its global environment.
//
// Preconditions:
// - L must be a valid LuaState
// - code must be valid Lua source
//
// Postconditions:
// - On success: L may have modified global state
// - On error: L is unchanged, error contains parse/compile info
func DoStringOn(L stateapi.LuaStateInterface, code string) (retErr error) {
	// Step 1: Parse - parse.NewParser().Parse(code) → ast.Chunk
	parser := parse.NewParser()
	chunk, err := parser.Parse(code)
	if err != nil {
		return err
	}

	// Step 2: Compile - bytecode.NewCompiler().Compile(chunk) → Prototype
	compiler := bc.NewCompiler("=(DoString)")
	proto, err := compiler.Compile(chunk)
	if err != nil {
		return err
	}

	// Step 3: Register prototype and create closure
	internal.RegisterDoStringClosure(L, proto)

	// Step 4: Execute - state.Call(0, 0)
	// Wrap in recover to catch Lua error() panics (LuaError).
	// Without this, error("msg") at top level crashes the Go process.
	defer func() {
		if r := recover(); r != nil {
			if le, ok := r.(*internal.LuaError); ok {
				if le.Msg != nil {
					retErr = fmt.Errorf("%v", le.Msg.GetValue())
				} else {
					retErr = fmt.Errorf("error object is nil")
				}
			} else {
				// Re-panic for Go bugs (non-LuaError panics)
				panic(r)
			}
		}
	}()

	L.Call(0, 0)

	// Check for execution errors (stored on LuaState by VM executor)
	type errorProvider interface {
		GetLastError() error
	}
	if ep, ok := L.(errorProvider); ok {
		if err := ep.GetLastError(); err != nil {
			return err
		}
	}

	return nil
}

// DoBuffer is like DoString but for byte slice.
func DoBuffer(buff []byte) error {
	return DoString(string(buff))
}

// Require loads and executes a Lua module.
// Returns error if module cannot be loaded or executed.
func Require(name string) error {
	// CONTRACT: Load module by name
	// 1. Check package.loaded[name]
	// 2. If not loaded, try package.path patterns
	// 3. Execute module, store in package.loaded[name]
	// 4. Return module's return value or true
	panic("TODO: implement Require")
}
