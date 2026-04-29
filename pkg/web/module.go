// Package web provides a Gin web framework binding for Lua.
//
// Lua usage:
//
//	local web = require("web")
//	local app = web.new()
//	app:get("/", function(ctx) ctx:json(200, {msg = "hello"}) end)
//	app:run(":8080")
package web

import "github.com/akzj/go-lua/pkg/lua"

func init() {
	lua.RegisterGlobal("web", OpenWeb)
}

// OpenWeb opens the "web" module and pushes it onto the stack.
func OpenWeb(L *lua.State) {
	L.NewLib(map[string]lua.Function{
		"new": webNew,
	})
}

// webNew creates a new web App and returns it as userdata.
// Lua: local app = web.new()
func webNew(L *lua.State) int {
	app := newApp(L)
	L.PushUserdata(app)
	setAppMetatable(L)
	return 1
}
