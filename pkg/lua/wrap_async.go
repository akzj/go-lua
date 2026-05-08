package lua

import (
	"fmt"
	"runtime/debug"
)

// WrapAsync wraps a synchronous [Function] so it runs in a background goroutine
// and returns a [*Future] immediately. The caller must use async.await() inside
// a spawned coroutine to retrieve the result.
//
// How it works:
//  1. Captures all Lua arguments from the stack (via [State.ToAny]) on the calling goroutine.
//  2. Creates a [Future] and returns it immediately.
//  3. Spawns a goroutine that creates a fresh worker [State], pushes the captured
//     arguments, calls the original function via PCall, and resolves/rejects the Future.
//
// Return convention (matches standard Go→Lua binding patterns):
//   - Original returns 1 value: Future resolves with that value.
//   - Original returns (nil, "error"): Future rejects with the error string.
//   - Original returns (value, nil): Future resolves with value.
//   - Original returns 0 values: Future resolves with nil.
//   - Original panics: Future rejects with the panic message + stack trace.
//
// The worker State is a minimal fresh State — it has no loaded modules or globals
// beyond the defaults. This is fine because typical Go binding functions only use
// the Lua stack for argument passing and don't call into Lua code.
//
// Example:
//
//	// Register an async version of a Go binding:
//	L.SetField(apiTable, "list_agents", lua.WrapAsync(listAgentsSync))
//
//	-- Lua side:
//	lumina.spawn(function()
//	    local result, err = async.await(api.list_agents({workspace_id = "ws1"}))
//	end)
func WrapAsync(fn Function) Function {
	return func(L *State) int {
		// 1. Capture all arguments from the Lua stack (main thread).
		nArgs := L.GetTop()
		args := make([]any, nArgs)
		for i := 1; i <= nArgs; i++ {
			args[i-1] = L.ToAny(i)
		}

		// 2. Create the Future to return immediately.
		future := NewFuture()

		// 3. Execute the original function in a goroutine with a fresh worker State.
		go func() {
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())
					future.Reject(fmt.Errorf("WrapAsync panic: %v\n%s", r, stack))
				}
			}()

			worker := NewState()
			defer worker.Close()

			// Push captured arguments onto the worker stack.
			for _, arg := range args {
				worker.PushAny(arg)
			}

			// Push the function, then move it below the arguments.
			worker.PushFunction(fn)
			worker.Insert(1) // stack: [fn, arg1, arg2, ...]

			// Call with PCall for protected execution.
			status := worker.PCall(nArgs, MultiRet, 0)
			if status != OK {
				msg, _ := worker.ToString(-1)
				future.Reject(luaError(msg))
				return
			}

			// Interpret return values.
			nRet := worker.GetTop()
			switch {
			case nRet == 0:
				future.Resolve(nil)
			case nRet == 1:
				future.Resolve(worker.ToAny(1))
			default:
				// nRet >= 2: check the (value, error) convention.
				// If first is nil and second is a string → treat as error.
				if worker.IsNil(1) && worker.IsString(2) {
					msg, _ := worker.ToString(2)
					future.Reject(luaError(msg))
				} else {
					// Resolve with first value (second is typically nil error).
					future.Resolve(worker.ToAny(1))
				}
			}
		}()

		// 4. Push the Future and return it to the caller.
		L.PushUserdata(future)
		return 1
	}
}

// WrapAsyncWithContext is like [WrapAsync] but creates a Future with context
// support. The worker goroutine receives a [context.Context] derived from the
// parent State's context. If the parent context is cancelled, the derived
// context is also cancelled, allowing the Go function to abort in-flight
// operations (e.g., HTTP requests).
//
// The Go function can access the context via worker.Context().
//
// Example:
//
//	L.SetField(apiTable, "fetch", lua.WrapAsyncWithContext(fetchSync))
//
//	// In fetchSync:
//	func fetchSync(L *lua.State) int {
//	    ctx := L.Context() // derived context — cancelled if parent cancels
//	    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
//	    // ...
//	}
func WrapAsyncWithContext(fn Function) Function {
	return func(L *State) int {
		// 1. Capture all arguments.
		nArgs := L.GetTop()
		args := make([]any, nArgs)
		for i := 1; i <= nArgs; i++ {
			args[i-1] = L.ToAny(i)
		}

		// 2. Create Future with context derived from the calling State's context.
		parentCtx := L.Context()
		future, ctx := NewFutureWithContext(parentCtx)

		// 3. Execute in goroutine.
		go func() {
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())
					future.Reject(fmt.Errorf("WrapAsync panic: %v\n%s", r, stack))
				}
			}()

			worker := NewState()
			defer worker.Close()
			worker.SetContext(ctx)

			// Push captured arguments.
			for _, arg := range args {
				worker.PushAny(arg)
			}

			// Push function and move below args.
			worker.PushFunction(fn)
			worker.Insert(1)

			status := worker.PCall(nArgs, MultiRet, 0)
			if status != OK {
				msg, _ := worker.ToString(-1)
				future.Reject(luaError(msg))
				return
			}

			nRet := worker.GetTop()
			switch {
			case nRet == 0:
				future.Resolve(nil)
			case nRet == 1:
				future.Resolve(worker.ToAny(1))
			default:
				if worker.IsNil(1) && worker.IsString(2) {
					msg, _ := worker.ToString(2)
					future.Reject(luaError(msg))
				} else {
					future.Resolve(worker.ToAny(1))
				}
			}
		}()

		L.PushUserdata(future)
		return 1
	}
}
