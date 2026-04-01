// Package state provides Lua state management implementation.
// Main entry point for the state module.
package state

import (
	stateapi "github.com/akzj/go-lua/state/api"
	"github.com/akzj/go-lua/state/internal"
)

func init() {
	// Initialize the default LuaState factory
	stateapi.DefaultLuaState = internal.NewLuaState(nil)
}

// New creates a new Lua state with its own global state.
func New() stateapi.LuaStateInterface {
	return internal.NewLuaState(nil)
}
