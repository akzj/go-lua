package internal

import (
	luaapi "github.com/akzj/go-lua/api"
	webapi "github.com/akzj/go-lua/web/api"
)

// WebApp is an alias for the API type.
type WebApp = webapi.WebApp

// NewWebApp creates a new WebApp.
func NewWebApp(L luaapi.LuaAPI) webapi.WebApp {
	return webapi.NewWebApp(L)
}
