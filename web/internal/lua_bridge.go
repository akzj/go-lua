package internal

import (
	luaapi "github.com/akzj/go-lua/api/api"
	webapi "github.com/akzj/go-lua/web/api"
)

// LuaContextBridge is an alias for the API type.
type LuaContextBridge = webapi.LuaContextBridge

// NewLuaContextBridge creates a new LuaContextBridge.
func NewLuaContextBridge() webapi.LuaContextBridge {
	return webapi.NewLuaContextBridge()
}

// HTTPRequest represents an HTTP request for async operations.
type HTTPRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

// InjectContextFunc is the function type for InjectContext to avoid import cycle.
type InjectContextFunc func(L luaapi.LuaAPI, ctx interface{})
