package internal

import (
	luaapi "github.com/akzj/go-lua/api"
	webapi "github.com/akzj/go-lua/web/api"
)

// WebLib is an alias for the API type.
type WebLib = webapi.WebLib

// OpenLibs registers all web libraries.
func OpenLibs(L luaapi.LuaAPI) {
	webapi.OpenLibs(L)
}
