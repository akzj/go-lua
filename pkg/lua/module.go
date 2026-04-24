package lua

// Module is the standard interface for go-lua extension modules.
// Third-party libraries implement this to be loadable via [LoadModules].
//
// Example:
//
//	type JSONModule struct{}
//	func (JSONModule) Name() string { return "json" }
//	func (JSONModule) Open(L *lua.State) {
//	    L.NewTableFrom(map[string]any{})
//	    L.PushFunction(jsonEncode)
//	    L.SetField(-2, "encode")
//	}
type Module interface {
	// Name returns the module name used in require().
	Name() string
	// Open registers the module into the given Lua State.
	// It should push exactly one value (the module table) onto the stack.
	Open(L *State)
}

// LoadModules loads multiple modules into a State by registering each
// module's Open method into package.preload. After this call, each module
// is available via require("name").
//
// This is the per-State equivalent of [RegisterGlobal]. Use LoadModules when
// you want a module available only in specific States rather than globally.
func LoadModules(L *State, modules ...Module) {
	for _, m := range modules {
		name := m.Name()
		opener := m.Open // capture for closure
		L.preloadModule(name, opener)
	}
}

// preloadModule registers an opener into package.preload[name].
func (L *State) preloadModule(name string, opener func(L *State)) {
	L.GetGlobal("package")
	if L.IsNil(-1) {
		L.Pop(1)
		return
	}
	L.GetField(-1, "preload")
	if !L.IsTable(-1) {
		L.Pop(2) // pop preload + package
		return
	}
	L.PushFunction(func(L *State) int {
		topBefore := L.GetTop()
		opener(L)
		if L.GetTop() == topBefore {
			L.PushBoolean(true)
		}
		return 1
	})
	L.SetField(-2, name)
	L.Pop(2) // pop preload + package
}
