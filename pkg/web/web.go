package web

import (
	"net/http"
	"sync"

	"github.com/akzj/go-lua/pkg/lua"
	"github.com/gin-gonic/gin"
)

const appMetaName = "web.App"

// App wraps a gin.Engine and the Lua state that owns it.
// All Lua handler calls are serialized via mu since a single Lua State
// is not thread-safe. The mutex is acquired once per request at the
// top of the handler chain (via a gin middleware installed first).
type App struct {
	engine *gin.Engine
	L      *lua.State
	mu     sync.Mutex
	refs   []int // handler refs to clean up
}

func newApp(L *lua.State) *App {
	gin.SetMode(gin.ReleaseMode)
	app := &App{
		engine: gin.New(),
		L:      L,
	}
	// Install a top-level middleware that acquires the Lua mutex
	// for the entire request lifetime. This prevents deadlocks when
	// ctx:next() chains through multiple Lua handlers.
	app.engine.Use(func(c *gin.Context) {
		app.mu.Lock()
		defer app.mu.Unlock()
		c.Next()
	})
	return app
}

// ServeHTTP implements http.Handler so the app can be used with httptest.
func (app *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	app.engine.ServeHTTP(w, r)
}

// ---------------------------------------------------------------------------
// Metatable setup
// ---------------------------------------------------------------------------

func setAppMetatable(L *lua.State) {
	if L.NewMetatable(appMetaName) {
		// __index = method table
		L.NewTableFrom(map[string]any{})
		pushAppMethods(L)
		L.SetField(-2, "__index")
	}
	L.SetMetatable(-2)
}

func pushAppMethods(L *lua.State) {
	methods := map[string]lua.Function{
		"get":    appRoute("GET"),
		"post":   appRoute("POST"),
		"put":    appRoute("PUT"),
		"delete": appRoute("DELETE"),
		"patch":  appRoute("PATCH"),
		"use":    appUse,
		"group":  appGroup,
		"static": appStatic,
		"run":    appRun,
	}
	idx := L.AbsIndex(-1) // __index table
	for name, fn := range methods {
		L.PushFunction(fn)
		L.SetField(idx, name)
	}
}

// ---------------------------------------------------------------------------
// Helper: extract *App from userdata at stack position 1
// ---------------------------------------------------------------------------

func checkApp(L *lua.State) *App {
	ud := L.CheckUserdata(1)
	app, ok := ud.(*App)
	if !ok {
		L.ArgError(1, "web.App expected")
		return nil // unreachable
	}
	return app
}

// ---------------------------------------------------------------------------
// Route methods: app:get(path, handler), app:post(path, handler), etc.
// ---------------------------------------------------------------------------

// appRoute returns a lua.Function that registers a route for the given HTTP method.
func appRoute(method string) lua.Function {
	return func(L *lua.State) int {
		app := checkApp(L)
		path := L.CheckString(2)
		L.CheckType(3, lua.TypeFunction)

		// Store the handler function in the registry.
		L.PushValue(3)
		ref := L.Ref(lua.RegistryIndex)
		app.refs = append(app.refs, ref)

		app.engine.Handle(method, path, app.makeLuaHandler(ref))
		return 0
	}
}

// makeLuaHandler creates a gin.HandlerFunc that calls a Lua handler by registry ref.
// The Lua mutex is already held by the top-level middleware, so no locking here.
func (app *App) makeLuaHandler(ref int) gin.HandlerFunc {
	return func(c *gin.Context) {
		L := app.L
		// Push handler function from registry.
		L.RawGetI(lua.RegistryIndex, int64(ref))
		// Push ctx userdata.
		pushContext(L, c)
		// Call handler(ctx). PCall returns status code.
		status := L.PCall(1, 0, 0)
		if status != lua.OK {
			msg, _ := L.ToString(-1)
			L.Pop(1)
			if !c.Writer.Written() {
				c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Middleware: app:use(handler)
// ---------------------------------------------------------------------------

func appUse(L *lua.State) int {
	app := checkApp(L)
	L.CheckType(2, lua.TypeFunction)

	L.PushValue(2)
	ref := L.Ref(lua.RegistryIndex)
	app.refs = append(app.refs, ref)

	app.engine.Use(app.makeLuaHandler(ref))
	return 0
}

// ---------------------------------------------------------------------------
// Route groups: local g = app:group(prefix)
// ---------------------------------------------------------------------------

// Group wraps a gin.RouterGroup for Lua.
type Group struct {
	group *gin.RouterGroup
	app   *App // back-reference for makeLuaHandler
}

const groupMetaName = "web.Group"

func appGroup(L *lua.State) int {
	app := checkApp(L)
	prefix := L.CheckString(2)
	g := &Group{
		group: app.engine.Group(prefix),
		app:   app,
	}
	L.PushUserdata(g)
	setGroupMetatable(L)
	return 1
}

func setGroupMetatable(L *lua.State) {
	if L.NewMetatable(groupMetaName) {
		L.NewTableFrom(map[string]any{})
		pushGroupMethods(L)
		L.SetField(-2, "__index")
	}
	L.SetMetatable(-2)
}

func pushGroupMethods(L *lua.State) {
	methods := map[string]lua.Function{
		"get":    groupRoute("GET"),
		"post":   groupRoute("POST"),
		"put":    groupRoute("PUT"),
		"delete": groupRoute("DELETE"),
		"patch":  groupRoute("PATCH"),
		"use":    groupUse,
		"group":  groupGroup,
	}
	idx := L.AbsIndex(-1)
	for name, fn := range methods {
		L.PushFunction(fn)
		L.SetField(idx, name)
	}
}

func checkGroup(L *lua.State) *Group {
	ud := L.CheckUserdata(1)
	g, ok := ud.(*Group)
	if !ok {
		L.ArgError(1, "web.Group expected")
		return nil
	}
	return g
}

func groupRoute(method string) lua.Function {
	return func(L *lua.State) int {
		g := checkGroup(L)
		path := L.CheckString(2)
		L.CheckType(3, lua.TypeFunction)

		L.PushValue(3)
		ref := L.Ref(lua.RegistryIndex)
		g.app.refs = append(g.app.refs, ref)

		g.group.Handle(method, path, g.app.makeLuaHandler(ref))
		return 0
	}
}

func groupUse(L *lua.State) int {
	g := checkGroup(L)
	L.CheckType(2, lua.TypeFunction)

	L.PushValue(2)
	ref := L.Ref(lua.RegistryIndex)
	g.app.refs = append(g.app.refs, ref)

	g.group.Use(g.app.makeLuaHandler(ref))
	return 0
}

func groupGroup(L *lua.State) int {
	g := checkGroup(L)
	prefix := L.CheckString(2)
	sub := &Group{
		group: g.group.Group(prefix),
		app:   g.app,
	}
	L.PushUserdata(sub)
	setGroupMetatable(L)
	return 1
}

// ---------------------------------------------------------------------------
// Static files: app:static(relativePath, root)
// ---------------------------------------------------------------------------

func appStatic(L *lua.State) int {
	app := checkApp(L)
	relativePath := L.CheckString(2)
	root := L.CheckString(3)
	app.engine.Static(relativePath, root)
	return 0
}

// ---------------------------------------------------------------------------
// Run: app:run(addr)
// ---------------------------------------------------------------------------

func appRun(L *lua.State) int {
	app := checkApp(L)
	addr := L.OptString(2, ":8080")
	// Run blocks. Errors are returned as nil + error string.
	if err := app.engine.Run(addr); err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}
	return 0
}
