package lua

import (
	"sync"

	"github.com/akzj/go-lua/internal/api"
)

// ModuleOpener is a function that opens/registers a module into a Lua State.
// It should push exactly one value (the module table) onto the stack and return 1.
// Typically called automatically when require("name") triggers the global registry searcher.
type ModuleOpener func(L *State)

var (
	globalMu       sync.RWMutex
	globalRegistry = make(map[string]ModuleOpener)
)

// RegisterGlobal registers a module opener in the global registry.
// Thread-safe. Typically called from package init() functions.
// When any State calls require("name"), the opener will be invoked
// if the module isn't found in package.loaded or package.preload.
//
// Example:
//
//	func init() {
//	    lua.RegisterGlobal("json", func(L *lua.State) {
//	        L.NewTableFrom(map[string]any{})
//	        L.PushFunction(jsonEncode)
//	        L.SetField(-2, "encode")
//	        L.PushFunction(jsonDecode)
//	        L.SetField(-2, "decode")
//	    })
//	}
func RegisterGlobal(name string, opener ModuleOpener) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalRegistry[name] = opener
}

// UnregisterGlobal removes a module from the global registry.
// Thread-safe.
func UnregisterGlobal(name string) {
	globalMu.Lock()
	defer globalMu.Unlock()
	delete(globalRegistry, name)
}

// GlobalModules returns a copy of all registered global module names.
// Thread-safe.
func GlobalModules() []string {
	globalMu.RLock()
	defer globalMu.RUnlock()
	names := make([]string, 0, len(globalRegistry))
	for name := range globalRegistry {
		names = append(names, name)
	}
	return names
}

// lookupGlobal looks up a module in the global registry.
// Returns the opener and true if found, nil and false otherwise.
// Thread-safe (uses RLock).
func lookupGlobal(name string) (ModuleOpener, bool) {
	globalMu.RLock()
	defer globalMu.RUnlock()
	opener, ok := globalRegistry[name]
	return opener, ok
}

// installGlobalSearcher sets up the internal hook so that
// package.searchers includes the Go global registry.
// Called during NewState() and NewSandboxState().
func (L *State) installGlobalSearcher() {
	L.s.GlobalSearcher = func(name string) api.CFunction {
		opener, ok := lookupGlobal(name)
		if !ok {
			return nil
		}
		// Return a CFunction that wraps the opener.
		// The CFunction is called by require() as a loader with (modname, extra) args.
		return func(apiL *api.State) int {
			wrapper := &State{s: apiL}
			topBefore := wrapper.GetTop()
			opener(wrapper)
			// If opener didn't push anything, push true as the module value
			if wrapper.GetTop() == topBefore {
				wrapper.PushBoolean(true)
			}
			return 1
		}
	}
}
