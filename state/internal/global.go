// Package internal provides the concrete implementation of state/api interfaces.
package internal

import (
	memapi "github.com/akzj/go-lua/mem/api"
	"github.com/akzj/go-lua/state/api"
	tableapi "github.com/akzj/go-lua/table/api"
)

// globalState is the concrete implementation of GlobalState.
// Holds state shared by all threads in the Lua VM.
type globalState struct {
	alloc     memapi.Allocator        // Memory allocator
	registry  tableapi.TableInterface  // Global registry
	mainThread *LuaState         // Main thread (the one that created this global state)
}

func (g *globalState) Allocator() memapi.Allocator {
	return g.alloc
}

func (g *globalState) Registry() tableapi.TableInterface {
	return g.registry
}

func (g *globalState) CurrentThread() api.LuaStateInterface {
	return g.mainThread
}
