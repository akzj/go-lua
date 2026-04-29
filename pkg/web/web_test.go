package web_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
	_ "github.com/akzj/go-lua/pkg/web" // register "web" module via init()
)

// helper: run Lua code, extract app global, return http.Handler
func setupApp(t *testing.T, code string) http.Handler {
	t.Helper()
	L := lua.NewState()
	t.Cleanup(L.Close)

	if err := L.DoString(code); err != nil {
		t.Fatalf("DoString: %v", err)
	}

	// Retrieve the global "app" variable
	L.GetGlobal("app")
	if L.IsNil(-1) {
		t.Fatal("global 'app' is nil — Lua code must set a global 'app'")
	}
	ud := L.CheckUserdata(-1)
	L.Pop(1)

	type httpHandler interface {
		ServeHTTP(http.ResponseWriter, *http.Request)
	}
	h, ok := ud.(httpHandler)
	if !ok {
		t.Fatalf("app userdata does not implement http.Handler: %T", ud)
	}
	return h
}

func doRequest(t *testing.T, h http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func jsonBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("json unmarshal: %v\nbody: %s", err, w.Body.String())
	}
	return m
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestGetRoute(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:get("/hello", function(ctx)
			ctx:json(200, { msg = "world" })
		end)
	`)

	w := doRequest(t, h, "GET", "/hello", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := jsonBody(t, w)
	if m["msg"] != "world" {
		t.Fatalf("msg = %v, want 'world'", m["msg"])
	}
}

func TestPostRoute(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:post("/echo", function(ctx)
			local data = ctx:bind_json()
			ctx:json(201, data)
		end)
	`)

	w := doRequest(t, h, "POST", "/echo", `{"name":"alice"}`)
	if w.Code != 201 {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	m := jsonBody(t, w)
	if m["name"] != "alice" {
		t.Fatalf("name = %v, want 'alice'", m["name"])
	}
}

func TestPathParam(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:get("/users/:id", function(ctx)
			local id = ctx:param("id")
			ctx:json(200, { id = id })
		end)
	`)

	w := doRequest(t, h, "GET", "/users/42", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := jsonBody(t, w)
	if m["id"] != "42" {
		t.Fatalf("id = %v, want '42'", m["id"])
	}
}

func TestQueryParam(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:get("/search", function(ctx)
			local q = ctx:query("q", "default")
			ctx:json(200, { query = q })
		end)
	`)

	w := doRequest(t, h, "GET", "/search?q=hello", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := jsonBody(t, w)
	if m["query"] != "hello" {
		t.Fatalf("query = %v, want 'hello'", m["query"])
	}

	// Test default value
	w2 := doRequest(t, h, "GET", "/search", "")
	m2 := jsonBody(t, w2)
	if m2["query"] != "default" {
		t.Fatalf("query default = %v, want 'default'", m2["query"])
	}
}

func TestStringResponse(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:get("/text", function(ctx)
			ctx:string(200, "hello text")
		end)
	`)

	w := doRequest(t, h, "GET", "/text", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "hello text" {
		t.Fatalf("body = %q, want 'hello text'", got)
	}
}

func TestHTMLResponse(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:get("/page", function(ctx)
			ctx:html(200, "<h1>hi</h1>")
		end)
	`)

	w := doRequest(t, h, "GET", "/page", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
	if got := w.Body.String(); got != "<h1>hi</h1>" {
		t.Fatalf("body = %q, want '<h1>hi</h1>'", got)
	}
}

func TestMiddleware(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:use(function(ctx)
			ctx:set_header("X-Custom", "middleware-hit")
			ctx:next()
		end)
		app:get("/mw", function(ctx)
			ctx:json(200, { ok = true })
		end)
	`)

	w := doRequest(t, h, "GET", "/mw", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("X-Custom"); got != "middleware-hit" {
		t.Fatalf("X-Custom = %q, want 'middleware-hit'", got)
	}
}

func TestRouteGroup(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		local api = app:group("/api")
		api:get("/items", function(ctx)
			ctx:json(200, { items = {"a", "b"} })
		end)
	`)

	w := doRequest(t, h, "GET", "/api/items", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := jsonBody(t, w)
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("items not array: %v", m["items"])
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
}

func TestLuaErrorReturns500(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:get("/boom", function(ctx)
			error("intentional error")
		end)
	`)

	w := doRequest(t, h, "GET", "/boom", "")
	if w.Code != 500 {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	m := jsonBody(t, w)
	errMsg, _ := m["error"].(string)
	if !strings.Contains(errMsg, "intentional error") {
		t.Fatalf("error = %q, want to contain 'intentional error'", errMsg)
	}
}

func TestContextSetGet(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:use(function(ctx)
			ctx:set("user", "alice")
			ctx:next()
		end)
		app:get("/who", function(ctx)
			local user = ctx:get("user")
			ctx:json(200, { user = user })
		end)
	`)

	w := doRequest(t, h, "GET", "/who", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := jsonBody(t, w)
	if m["user"] != "alice" {
		t.Fatalf("user = %v, want 'alice'", m["user"])
	}
}

func TestContextMethodAndPath(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:get("/info", function(ctx)
			ctx:json(200, {
				method = ctx:method(),
				path = ctx:path(),
				ip = ctx:client_ip(),
			})
		end)
	`)

	w := doRequest(t, h, "GET", "/info", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := jsonBody(t, w)
	if m["method"] != "GET" {
		t.Fatalf("method = %v, want 'GET'", m["method"])
	}
	if m["path"] != "/info" {
		t.Fatalf("path = %v, want '/info'", m["path"])
	}
}

func TestPutDeletePatch(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:put("/item", function(ctx)
			ctx:json(200, { method = "PUT" })
		end)
		app:delete("/item", function(ctx)
			ctx:json(200, { method = "DELETE" })
		end)
		app:patch("/item", function(ctx)
			ctx:json(200, { method = "PATCH" })
		end)
	`)

	for _, method := range []string{"PUT", "DELETE", "PATCH"} {
		w := doRequest(t, h, method, "/item", "")
		if w.Code != 200 {
			t.Fatalf("%s: status = %d, want 200", method, w.Code)
		}
		m := jsonBody(t, w)
		if m["method"] != method {
			t.Fatalf("%s: method = %v, want %q", method, m["method"], method)
		}
	}
}

func TestNestedGroup(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		local v1 = app:group("/v1")
		local users = v1:group("/users")
		users:get("/:id", function(ctx)
			ctx:json(200, { id = ctx:param("id"), version = "v1" })
		end)
	`)

	w := doRequest(t, h, "GET", "/v1/users/99", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := jsonBody(t, w)
	if m["id"] != "99" {
		t.Fatalf("id = %v, want '99'", m["id"])
	}
	if m["version"] != "v1" {
		t.Fatalf("version = %v, want 'v1'", m["version"])
	}
}

func TestHeaderAccess(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:get("/hdr", function(ctx)
			local auth = ctx:header("Authorization")
			ctx:json(200, { auth = auth })
		end)
	`)

	req := httptest.NewRequest("GET", "/hdr", nil)
	req.Header.Set("Authorization", "Bearer token123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := jsonBody(t, w)
	if m["auth"] != "Bearer token123" {
		t.Fatalf("auth = %v, want 'Bearer token123'", m["auth"])
	}
}

func TestAbortMiddleware(t *testing.T) {
	h := setupApp(t, `
		local web = require("web")
		app = web.new()
		app:use(function(ctx)
			local key = ctx:header("X-Api-Key")
			if key ~= "secret" then
				ctx:abort()
				ctx:json(403, { error = "forbidden" })
				return
			end
			ctx:next()
		end)
		app:get("/protected", function(ctx)
			ctx:json(200, { data = "sensitive" })
		end)
	`)

	// Without key → 403
	w := doRequest(t, h, "GET", "/protected", "")
	if w.Code != 403 {
		t.Fatalf("no key: status = %d, want 403", w.Code)
	}

	// With correct key → 200
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("X-Api-Key", "secret")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req)
	if w2.Code != 200 {
		t.Fatalf("with key: status = %d, want 200", w2.Code)
	}
	m := jsonBody(t, w2)
	if m["data"] != "sensitive" {
		t.Fatalf("data = %v, want 'sensitive'", m["data"])
	}
}
