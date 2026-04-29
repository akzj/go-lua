package web

import (
	"io"
	"net/http"

	"github.com/akzj/go-lua/pkg/lua"
	"github.com/gin-gonic/gin"
)

const ctxMetaName = "web.Context"

// luaContext wraps a *gin.Context for Lua access.
type luaContext struct {
	c *gin.Context
}

// pushContext creates a luaContext userdata and pushes it onto the stack.
func pushContext(L *lua.State, c *gin.Context) {
	ctx := &luaContext{c: c}
	L.PushUserdata(ctx)
	setCtxMetatable(L)
}

func setCtxMetatable(L *lua.State) {
	if L.NewMetatable(ctxMetaName) {
		// __index = method table
		L.NewTableFrom(map[string]any{})
		pushCtxMethods(L)
		L.SetField(-2, "__index")
	}
	L.SetMetatable(-2)
}

func pushCtxMethods(L *lua.State) {
	methods := map[string]lua.Function{
		"param":      ctxParam,
		"query":      ctxQuery,
		"header":     ctxHeader,
		"body":       ctxBody,
		"bind_json":  ctxBindJSON,
		"json":       ctxJSON,
		"string":     ctxString,
		"html":       ctxHTML,
		"redirect":   ctxRedirect,
		"set_header": ctxSetHeader,
		"set":        ctxSet,
		"get":        ctxGet,
		"next":       ctxNext,
		"abort":      ctxAbort,
		"status":     ctxStatus,
		"client_ip":  ctxClientIP,
		"method":     ctxMethod,
		"path":       ctxPath,
	}
	idx := L.AbsIndex(-1)
	for name, fn := range methods {
		L.PushFunction(fn)
		L.SetField(idx, name)
	}
}

// ---------------------------------------------------------------------------
// Helper: extract *luaContext from userdata at stack position 1
// ---------------------------------------------------------------------------

func checkCtx(L *lua.State) *gin.Context {
	ud := L.CheckUserdata(1)
	ctx, ok := ud.(*luaContext)
	if !ok {
		L.ArgError(1, "web.Context expected")
		return nil
	}
	return ctx.c
}

// ---------------------------------------------------------------------------
// Context methods
// ---------------------------------------------------------------------------

// ctx:param(name) → string
func ctxParam(L *lua.State) int {
	c := checkCtx(L)
	name := L.CheckString(2)
	L.PushString(c.Param(name))
	return 1
}

// ctx:query(name [, default]) → string
func ctxQuery(L *lua.State) int {
	c := checkCtx(L)
	name := L.CheckString(2)
	def := L.OptString(3, "")
	L.PushString(c.DefaultQuery(name, def))
	return 1
}

// ctx:header(name) → string
func ctxHeader(L *lua.State) int {
	c := checkCtx(L)
	name := L.CheckString(2)
	L.PushString(c.GetHeader(name))
	return 1
}

// ctx:body() → string
func ctxBody(L *lua.State) int {
	c := checkCtx(L)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}
	L.PushString(string(body))
	return 1
}

// ctx:bind_json() → table | nil, error
func ctxBindJSON(L *lua.State) int {
	c := checkCtx(L)
	var data map[string]any
	if err := c.ShouldBindJSON(&data); err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}
	L.PushAny(data)
	return 1
}

// ctx:json(status, table)
func ctxJSON(L *lua.State) int {
	c := checkCtx(L)
	status := int(L.CheckInteger(2))
	data := L.ToAny(3)
	c.JSON(status, data)
	return 0
}

// ctx:string(status, text)
func ctxString(L *lua.State) int {
	c := checkCtx(L)
	status := int(L.CheckInteger(2))
	text := L.CheckString(3)
	c.String(status, "%s", text)
	return 0
}

// ctx:html(status, html)
func ctxHTML(L *lua.State) int {
	c := checkCtx(L)
	status := int(L.CheckInteger(2))
	html := L.CheckString(3)
	c.Data(status, "text/html; charset=utf-8", []byte(html))
	return 0
}

// ctx:redirect(status, url)
func ctxRedirect(L *lua.State) int {
	c := checkCtx(L)
	status := int(L.CheckInteger(2))
	url := L.CheckString(3)
	c.Redirect(status, url)
	return 0
}

// ctx:set_header(key, value)
func ctxSetHeader(L *lua.State) int {
	c := checkCtx(L)
	key := L.CheckString(2)
	value := L.CheckString(3)
	c.Header(key, value)
	return 0
}

// ctx:set(key, value)
func ctxSet(L *lua.State) int {
	c := checkCtx(L)
	key := L.CheckString(2)
	value := L.ToAny(3)
	c.Set(key, value)
	return 0
}

// ctx:get(key) → value | nil
func ctxGet(L *lua.State) int {
	c := checkCtx(L)
	key := L.CheckString(2)
	val, exists := c.Get(key)
	if !exists {
		L.PushNil()
		return 1
	}
	L.PushAny(val)
	return 1
}

// ctx:next()
func ctxNext(L *lua.State) int {
	c := checkCtx(L)
	c.Next()
	return 0
}

// ctx:abort()
func ctxAbort(L *lua.State) int {
	c := checkCtx(L)
	c.Abort()
	return 0
}

// ctx:status() → int
func ctxStatus(L *lua.State) int {
	c := checkCtx(L)
	L.PushInteger(int64(c.Writer.Status()))
	return 1
}

// ctx:client_ip() → string
func ctxClientIP(L *lua.State) int {
	c := checkCtx(L)
	L.PushString(c.ClientIP())
	return 1
}

// ctx:method() → string
func ctxMethod(L *lua.State) int {
	c := checkCtx(L)
	L.PushString(c.Request.Method)
	return 1
}

// ctx:path() → string
func ctxPath(L *lua.State) int {
	c := checkCtx(L)
	L.PushString(c.Request.URL.Path)
	return 1
}

// ---------------------------------------------------------------------------
// Helpers for testing — extract engine from App userdata
// ---------------------------------------------------------------------------

// Engine returns the underlying gin.Engine for testing purposes.
func Engine(app *App) http.Handler {
	return app.engine
}
