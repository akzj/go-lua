// Package lua provides a public Go API for embedding the Lua 5.5 interpreter.
//
// This package wraps the internal implementation behind a clean, stable API
// suitable for embedding Lua in Go applications. It follows the C Lua API
// conventions (stack-based value passing, pseudo-indices, protected calls)
// while providing Go-idiomatic error handling and type safety.
//
// # Quick Start
//
// Create a Lua state, execute code, and close it:
//
//	L := lua.NewState()
//	defer L.Close()
//	if err := L.DoString(`print("hello from Lua")`); err != nil {
//	    log.Fatal(err)
//	}
//
// # Registering Go Functions
//
// Go functions can be registered as Lua globals:
//
//	add := func(L *lua.State) int {
//	    a := L.CheckInteger(1)
//	    b := L.CheckInteger(2)
//	    L.PushInteger(a + b)
//	    return 1
//	}
//	L.PushFunction(add)
//	L.SetGlobal("add")
//
// Or use the type-safe generic wrappers:
//
//	lua.Wrap2R(L, func(a, b int64) int64 { return a + b })
//	L.SetGlobal("add")
//
// # Modules
//
// Register Go modules that Lua code can load with require():
//
//	lua.RegisterModule(L, "mylib", map[string]lua.Function{
//	    "hello": func(L *lua.State) int {
//	        L.PushString("world")
//	        return 1
//	    },
//	})
//
// Built-in modules available via require():
//   - "async" — coroutine-based async/await with [Future] and [Scheduler]
//   - "channel" — Go-style channels for Lua ([Channel])
//   - "timer" — setTimeout/setInterval timers ([TimerManager])
//   - "http" — HTTP client (GET/POST with async support)
//   - "json" — JSON encode/decode
//   - "hotreload" — live code reload with state preservation ([ReloadPlan])
//
// # Async / Coroutines
//
// The async system provides Future-based concurrency:
//
//	sched := lua.NewScheduler(L)
//	L.DoString(`
//	    local async = require("async")
//	    async.go("return 42")  -- runs in background
//	`)
//	sched.Tick() // drive coroutines forward
//
// Use [Scheduler] to manage async coroutines. Use [Future] for Go↔Lua
// async communication. Futures support Cancel and context propagation.
//
// # Resource Safety
//
// Prevent Go resource leaks when embedding:
//
//	// Auto-cleanup: __gc calls Close() when Lua GC collects the userdata
//	conn := db.Open(dsn)
//	L.PushResource(conn)
//
//	// Lua 5.5 <close> support: closes on scope exit + __gc safety net
//	L.PushCloseableResource(conn)
//
// In Lua:
//
//	local conn <close> = db.open(dsn)  -- auto-closed on scope exit
//	conn:close()                        -- or explicit close (idempotent)
//
// # Safe Calls
//
// Protected function calls with automatic Lua traceback and Go panic recovery:
//
//	L.GetGlobal("handler")
//	L.PushString(request)
//	if err := L.SafeCall(1, 1); err != nil {
//	    // err contains full Lua stack traceback
//	    log.Printf("error:\n%s", err)
//	}
//
// Wrap Go functions to convert panics into Lua errors:
//
//	L.PushFunction(lua.WrapSafe(func(L *lua.State) int {
//	    data := mustParse(input)  // if this panics → Lua error, not crash
//	    L.PushAny(data)
//	    return 1
//	}))
//
// # Leak Detection
//
// Track registry references during development to find leaks:
//
//	tracker := lua.NewRefTracker()
//	ref := tracker.Ref(L, lua.RegistryIndex)
//	// ... forget to Unref ...
//	leaks := tracker.Leaks()  // ["  ref=3 created at handler.go:42"]
//
// # Hot-Reload
//
// Replace module functions at runtime while preserving state:
//
//	result, err := L.ReloadModule("game.npc")
//	// result.Replaced = 5 functions updated
//	// All upvalue state (counters, caches) preserved
//
// For fine-grained control, use the two-phase commit API:
//
//	plan, err := L.PrepareReload("mymod")  // read-only preparation
//	if plan.HasIncompatible() {
//	    plan.Abort()  // no changes
//	} else {
//	    plan.Commit() // atomic replacement
//	}
//
// # Sandboxing
//
// Create sandboxed states with CPU limits and restricted libraries:
//
//	L := lua.NewSandboxState(lua.SandboxConfig{
//	    CPULimit: 100000, // max instructions
//	})
//
// # State Pool
//
// For high-concurrency servers, reuse Lua states:
//
//	pool := lua.NewStatePool(lua.PoolConfig{MaxStates: 10})
//	L := pool.Get()
//	defer pool.Put(L)
//
// # Type Bridge
//
// High-level type conversion between Go and Lua:
//
//	L.PushAny(map[string]any{"name": "Alice", "age": 30})
//	val := L.ToAny(-1) // map[string]any
//
//	var user User
//	L.ToStruct(-1, &user) // Lua table → Go struct
//
// # Stack-Based API
//
// Values are passed between Go and Lua via a virtual stack. Push functions
// move values from Go to the stack; To/Check functions read values from the
// stack back to Go. Positive indices count from the bottom (1 = first),
// negative indices count from the top (-1 = top element).
//
// # Error Handling
//
// Use [State.DoString] or [State.DoFile] for simple execution with Go error
// returns. For finer control, use [State.Load] + [State.PCall] which return
// status codes and leave error messages on the stack.
//
// # Memory Management
//
// The interpreter uses Go's garbage collector. Call [State.Close] when done
// to release internal resources promptly.
package lua
