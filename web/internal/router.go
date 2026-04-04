package internal

import (
	luaapi "github.com/akzj/go-lua/api/api"
	webapi "github.com/akzj/go-lua/web/api"
)

// RouteTable is an alias for the API type.
type RouteTable = webapi.RouteTable

// NewRouteTable creates a new RouteTable.
func NewRouteTable() webapi.RouteTable {
	return webapi.NewRouteTable()
}

// LuaRouteHandler adapts a Lua function to a Handler.
// This is a stub implementation - full Lua integration requires
// access to the Lua VM's LoadString function which is on LuaLib, not LuaAPI.
type LuaRouteHandler struct {
	ParentVM luaapi.LuaAPI
	FuncName string
	Script   string
}

// Handle implements Handler.
func (h *LuaRouteHandler) Handle(ctx *RequestContext) error {
	// TODO: Full Lua integration - requires LuaLib access
	// For now, return a simple response
	ctx.Write("Lua handler: " + h.FuncName)
	return nil
}
