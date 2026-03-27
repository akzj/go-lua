package api

import (
	"github.com/akzj/go-lua/pkg/object"
)

// openCoroutineLib registers the coroutine library.
// Note: This is a stub implementation. Full coroutine support requires
// significant VM changes to support stackful coroutines.
func (s *State) openCoroutineLib() {
	s.NewTable()
	coIdx := s.GetTop()

	s.PushFunction(coroutineRunning)
	s.SetField(coIdx, "running")

	s.PushFunction(coroutineStatus)
	s.SetField(coIdx, "status")

	s.PushFunction(coroutineIsyieldable)
	s.SetField(coIdx, "isyieldable")

	s.PushFunction(coroutineCreate)
	s.SetField(coIdx, "create")

	s.PushFunction(coroutineWrap)
	s.SetField(coIdx, "wrap")

	s.PushFunction(coroutineResume)
	s.SetField(coIdx, "resume")

	s.PushFunction(coroutineYield)
	s.SetField(coIdx, "yield")

	s.PushFunction(coroutineClose)
	s.SetField(coIdx, "close")

	s.SetGlobal("coroutine")
}

func coroutineRunning(L *State) int {
	// Return a thread value for the main thread
	// coroutine.lua expects type(main) == "thread"
	thread := object.NewThread(&object.Thread{})
	L.vm.Push(*thread)
	L.PushBoolean(true)
	return 2
}

func coroutineStatus(L *State) int {
	if L.GetTop() < 1 {
		L.PushString("running")
		return 1
	}
	co := L.vm.GetStack(1)
	if co.IsNil() {
		L.PushString("running")
	} else {
		L.PushString("dead")
	}
	return 1
}

func coroutineIsyieldable(L *State) int {
	L.PushBoolean(false)
	return 1
}

func coroutineCreate(L *State) int {
	L.NewTable()
	return 1
}

func coroutineWrap(L *State) int {
	if L.GetTop() >= 1 {
		f := L.vm.GetStack(1)
		L.vm.Push(*f)
	} else {
		L.PushNil()
	}
	return 1
}

func coroutineResume(L *State) int {
	// Check if argument is valid
	if L.GetTop() < 1 {
		L.PushBoolean(false)
		L.PushString("missing argument to coroutine.resume")
		return 2
	}
	co := L.vm.GetStack(1)
	if !co.IsThread() && !co.IsTable() && !co.IsNil() {
		L.PushBoolean(false)
		L.PushString("bad argument #1 to 'resume' (thread expected)")
		return 2
	}
	L.PushBoolean(false)
	L.PushString("coroutines not fully implemented")
	return 2
}

func coroutineYield(L *State) int {
	L.PushString("attempt to yield from outside a coroutine")
	L.Error()
	return 0
}

func coroutineClose(L *State) int {
	L.PushBoolean(true)
	return 1
}
