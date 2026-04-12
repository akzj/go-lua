package api

// Stub openers for libraries not yet implemented.
// Each returns 1 (an empty library table) to satisfy OpenAll.

import (
	luaapi "github.com/akzj/go-lua/internal/api/api"
)

func OpenIO(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{})
	return 1
}

func OpenOS(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{
		"clock": osClockStub,
	})
	return 1
}

func osClockStub(L *luaapi.State) int {
	L.PushNumber(0)
	return 1
}

func OpenDebug(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{})
	return 1
}

func OpenUTF8(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{})
	return 1
}

func OpenCoroutine(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{})
	return 1
}

func OpenPackage(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{})
	return 1
}
