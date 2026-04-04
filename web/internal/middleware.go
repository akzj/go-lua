package internal

import (
	luaapi "github.com/akzj/go-lua/api/api"
	webapi "github.com/akzj/go-lua/web/api"
)

// Middleware is an alias for the API type.
type Middleware = webapi.Middleware

// MiddlewareFunc is an alias for the API type.
type MiddlewareFunc = webapi.MiddlewareFunc

// Logger is the built-in logger middleware.
var Logger = webapi.Logger

// Recover is the built-in recover middleware.
var Recover = webapi.Recover

// CORS is the built-in CORS middleware.
var CORS = webapi.CORS

// LuaMiddleware adapts a Lua function to a Middleware.
// This is a stub implementation - full Lua integration requires
// access to the Lua VM's LoadString function which is on LuaLib, not LuaAPI.
type LuaMiddleware struct {
	FuncName string
	Script   string
	ParentVM luaapi.LuaAPI
}

// Process implements Middleware.
func (m *LuaMiddleware) Process(ctx *RequestContext, next Handler) error {
	// TODO: Full Lua integration - requires LuaLib access
	// For now, just call next handler
	if next != nil {
		return next.Handle(ctx)
	}
	return nil
}

// NewLuaMiddleware creates a new LuaMiddleware.
func NewLuaMiddleware(parentVM luaapi.LuaAPI, script, funcName string) *LuaMiddleware {
	return &LuaMiddleware{
		ParentVM: parentVM,
		Script:   script,
		FuncName: funcName,
	}
}
