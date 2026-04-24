package lua

func init() {
	RegisterGlobal("async", OpenAsync)
}

// OpenAsync opens the "async" module and pushes it onto the stack.
// Registered globally via init(), so `require("async")` works automatically.
//
// Lua API:
//
//	local async = require("async")
//	local future = async.go("return 42")        -- run code in a goroutine
//	local val, err = async.await(future)         -- yield until future resolves
//	local f = async.resolve(42)                  -- create an already-resolved future
//	local f = async.reject("oops")               -- create an already-rejected future
func OpenAsync(L *State) {
	L.NewLib(map[string]Function{
		"go":      asyncGo,
		"await":   asyncAwait,
		"resolve": asyncResolve,
		"reject":  asyncReject,
	})
}

// asyncGo starts a goroutine that executes code and returns a Future.
//
// Lua: local future = async.go(code_string)
//
// The code runs in a fresh Lua State (thread-safe). The return value from
// the code becomes the Future's resolved value.
//
// NOTE: Lua functions cannot be passed because a Lua closure is bound to
// its parent State and cannot be moved to another goroutine.
func asyncGo(L *State) int {
	if L.IsFunction(1) {
		L.ArgError(1, "async.go requires a string (Lua functions cannot run in another goroutine)")
		return 0
	}
	code := L.CheckString(1)

	future := NewFuture()
	go func() {
		worker := NewState()
		defer worker.Close()
		err := worker.DoString(code)
		if err != nil {
			future.Reject(err)
			return
		}
		// DoString uses PCall(0, MultiRet, 0), so return values are on the stack
		if worker.GetTop() > 0 {
			val := worker.ToAny(-1)
			future.Resolve(val)
		} else {
			future.Resolve(nil)
		}
	}()

	L.PushUserdata(future)
	return 1
}

// asyncAwait yields the current coroutine until the Future resolves.
//
// Lua: local val, err = async.await(future)
//
// Must be called from within a coroutine managed by a Scheduler.
// If the Future is already done, returns immediately without yielding.
func asyncAwait(L *State) int {
	ud := L.UserdataValue(1)
	if ud == nil {
		L.ArgError(1, "Future expected, got nil")
		return 0
	}
	future, ok := ud.(*Future)
	if !ok {
		L.ArgError(1, "Future expected")
		return 0
	}

	// If already done, return immediately without yielding
	if future.IsDone() {
		val, err := future.Result()
		if err != nil {
			L.PushNil()
			L.PushString(err.Error())
			return 2
		}
		L.PushAny(val)
		return 1
	}

	// Not done — yield with the Future as the yielded value.
	// The Scheduler will see this Future and resume us when it's done.
	L.PushUserdata(future)
	return L.Yield(1)
}

// asyncResolve creates an already-resolved Future.
//
// Lua: local f = async.resolve(42)
func asyncResolve(L *State) int {
	val := L.ToAny(1)
	f := NewFuture()
	f.Resolve(val)
	L.PushUserdata(f)
	return 1
}

// asyncReject creates an already-rejected Future.
//
// Lua: local f = async.reject("something went wrong")
func asyncReject(L *State) int {
	msg := L.CheckString(1)
	f := NewFuture()
	f.Reject(luaError(msg))
	L.PushUserdata(f)
	return 1
}

// luaError is a simple error type for Lua-originated errors.
type luaError string

func (e luaError) Error() string { return string(e) }
