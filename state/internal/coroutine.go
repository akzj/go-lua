// Coroutine support for Lua 5.4/5.5
package internal

import (
	"unsafe"

	types "github.com/akzj/go-lua/types/api"
)

// LuaThread represents a Lua coroutine/thread
type LuaThread struct {
	Function types.TValue // The function to execute
	Status   string       // "suspended", "running", "dead", "normal"
}

// threadCounter is used to create unique thread IDs
var threadCounter uint64 = 0

// newLuaThread creates a new Lua thread object
func newLuaThread(fn types.TValue) *LuaThread {
	threadCounter++
	return &LuaThread{
		Function: fn,
		Status:   "suspended",
	}
}

// getThreadData extracts LuaThread from a TValue if it's a thread
func getThreadData(t types.TValue) *LuaThread {
	if t.IsNil() {
		return nil
	}
	if t.IsThread() {
		data := t.GetValue()
		if thread, ok := data.(*LuaThread); ok {
			return thread
		}
		// Could be stored as unsafe pointer
		if ptr, ok := data.(unsafe.Pointer); ok {
			return (*LuaThread)(ptr)
		}
	}
	return nil
}

var mainThread *LuaThread

func init() {
	mainThread = &LuaThread{
		Function: types.NewTValueNil(),
		Status:   "running",
	}
}

// bcoroutineRunning implements coroutine.running()
func bcoroutineRunning(stack []types.TValue, base int) int {
	threadPtr := unsafe.Pointer(mainThread)
	threadVal := types.NewTValueThread(threadPtr)
	stack[base] = threadVal
	stack[base+1] = types.NewTValueBoolean(true)
	return 2
}

// bcoroutineCreate implements coroutine.create(f)
func bcoroutineCreate(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 1 {
		stack[base] = types.NewTValueNil()
		return 1
	}
	fn := stack[base+1]
	if !fn.IsFunction() {
		stack[base] = types.NewTValueNil()
		return 1
	}
	thread := newLuaThread(fn)
	threadPtr := unsafe.Pointer(thread)
	stack[base] = types.NewTValueThread(threadPtr)
	return 1
}

// bcoroutineResume implements coroutine.resume(co, ...)
func bcoroutineResume(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 1 {
		stack[base] = types.NewTValueBoolean(false)
		stack[base+1] = types.NewTValueString("cannot resume non-thead value")
		return 2
	}
	co := getThreadData(stack[base+1])
	if co == nil {
		stack[base] = types.NewTValueBoolean(false)
		stack[base+1] = types.NewTValueString("cannot resume non-thead value")
		return 2
	}
	if co.Status == "dead" {
		stack[base] = types.NewTValueBoolean(false)
		stack[base+1] = types.NewTValueString("cannot resume dead coroutine")
		return 2
	}
	// Mark as running - stub can't actually execute
	co.Status = "running"
	stack[base] = types.NewTValueBoolean(true)
	return 1
}

// bcoroutineYield implements coroutine.yield(...)
func bcoroutineYield(stack []types.TValue, base int) int {
	return 0
}

// bcoroutineStatus implements coroutine.status(co)
func bcoroutineStatus(stack []types.TValue, base int) int {
	nArgs := realArgCount(stack, base)
	if nArgs < 1 {
		stack[base] = types.NewTValueString("dead")
		return 1
	}
	co := getThreadData(stack[base+1])
	if co == nil {
		stack[base] = types.NewTValueString("dead")
		return 1
	}
	stack[base] = types.NewTValueString(co.Status)
	return 1
}

// bcoroutineIsyieldable implements coroutine.isyieldable()
func bcoroutineIsyieldable(stack []types.TValue, base int) int {
	stack[base] = types.NewTValueBoolean(false)
	return 1
}

// bcoroutineWrap implements coroutine.wrap(f)
func bcoroutineWrap(stack []types.TValue, base int) int {
	stack[base] = types.NewTValueNil()
	return 1
}
