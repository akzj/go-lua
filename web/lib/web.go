// Package lib provides the exported web module.
package lib

import (
	luaapi "github.com/akzj/go-lua/api"
	webapi "github.com/akzj/go-lua/web/api"
)

// Web is the exported web module constructor.
var Web = webapi.NewWebApp

// OpenWebLib opens the web library for the given Lua VM.
func OpenWebLib(L luaapi.LuaAPI) {
	webapi.OpenLibs(L)
}
