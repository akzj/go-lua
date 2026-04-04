package internal

import (
	luaapi "github.com/akzj/go-lua/api"
	webapi "github.com/akzj/go-lua/web/api"
)

// AsyncHandler is an alias for the API type.
type AsyncHandler = webapi.AsyncHandler

// NewAsyncHandler creates a new AsyncHandler.
func NewAsyncHandler(parentVM luaapi.LuaAPI, handler Handler) *AsyncHandler {
	return &webapi.AsyncHandler{
		ParentVM: parentVM,
		Handler:  handler,
	}
}

// NewAsyncHandlerWithLua creates a new AsyncHandler with Lua script.
func NewAsyncHandlerWithLua(parentVM luaapi.LuaAPI, script, funcName string) *AsyncHandler {
	return &webapi.AsyncHandler{
		ParentVM:  parentVM,
		LuaScript: script,
		FuncName:  funcName,
	}
}
