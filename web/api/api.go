// Package api defines Web extension interfaces for go-lua.
// Implements the web extension with HTTP server, routing, middleware, and Lua context bridging.
//
// Design constraints:
// - All I/O operations must be async (yield-based)
// - Must not block the Lua VM
// - Must integrate with existing api.LuaAPI
package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	luaapi "github.com/akzj/go-lua/api"
)

// LuaAPI mirrors the Lua API interface from api/api package.
type LuaAPI = luaapi.LuaAPI

// =============================================================================
// Yield Points
// =============================================================================

// YieldPoint identifies where a Lua coroutine can yield for async I/O.
// Used by VM to know when it's safe to switch coroutines.
type YieldPoint int

const (
	YieldNone         YieldPoint = 0 // Not a yield point
	YieldHTTPRequest  YieldPoint = 1 // Before sending HTTP request
	YieldHTTPResponse YieldPoint = 2 // Before sending response
	YieldFileRead     YieldPoint = 3 // Before reading file
	YieldFileWrite    YieldPoint = 4 // Before writing file
	YieldSleep        YieldPoint = 5 // Before sleeping
	YieldWebSocket    YieldPoint = 6 // Before WebSocket operation
)

// Why explicit YieldPoint?
// - VM needs to know when safe to yield
// - Not all C function calls can yield

// =============================================================================
// HTTP Server
// =============================================================================

// HTTPServer is the HTTP server interface.
// Invariant: After Listen returns, server is in Running state until Close/Shutdown.
type HTTPServer interface {
	// Listen starts the HTTP server on addr.
	// Post: After return, server is listening on addr.
	Listen(addr string) error

	// Close immediately shuts down the server.
	Close() error

	// Shutdown gracefully shuts down the server.
	Shutdown(ctx interface{}) error

	// Addr returns the server's listening address.
	Addr() string
}

// HTTPServerImpl is the implementation of HTTPServer.
type HTTPServerImpl struct {
	addr   string
	server *http.Server
}

// Listen implements HTTPServer.
func (s *HTTPServerImpl) Listen(addr string) error {
	s.addr = addr
	s.server = &http.Server{Addr: addr}
	return s.server.ListenAndServe()
}

// Close implements HTTPServer.
func (s *HTTPServerImpl) Close() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// Shutdown implements HTTPServer.
func (s *HTTPServerImpl) Shutdown(ctx interface{}) error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// Addr implements HTTPServer.
func (s *HTTPServerImpl) Addr() string {
	return s.addr
}

// NewHTTPServer creates a new HTTPServer.
func NewHTTPServer() HTTPServer {
	return &HTTPServerImpl{}
}

// Why not expose net/http Server directly?
// - Need to intercept ServeHTTP to inject Lua VM
// - Need to manage per-request lifecycle with coroutines

// =============================================================================
// Handler
// =============================================================================

// Handler processes HTTP requests.
// Returns error to abort middleware chain; nil to continue.
type Handler interface {
	// Handle processes the request.
	// Returns nil on success.
	// Returns error to stop middleware chain and send error response.
	// Invariant: Handle must not block synchronously (must yield).
	Handle(ctx *RequestContext) error
}

// HandlerFunc allows plain functions to implement Handler.
type HandlerFunc func(ctx *RequestContext) error

func (f HandlerFunc) Handle(ctx *RequestContext) error {
	return f(ctx)
}

// Why return error instead of directly writing response?
// - Response is written via ctx.Response (shared state)
// - Allows middleware to modify response
// - Enables error propagation

// =============================================================================
// Request & Response
// =============================================================================

// Request represents an HTTP request (immutable after creation).
// Why immutable?
// - Request data doesn't change during processing
// - Avoids concurrency issues
type Request struct {
	Method  string
	Path    string
	URL     interface{} // *url.URL - avoid import cycle
	Header  interface{} // http.Header - avoid import cycle
	Body    []byte

	// Parsed data (lazily populated)
	Form interface{} // url.Values
	JSON interface{}
}

// Response represents an HTTP response.
// Invariant: Header/Body can be modified by middleware chain.
type Response struct {
	StatusCode int
	Header     interface{} // http.Header or map[string]string
	Body       []byte
}

// ResponseHeader provides a map-like interface for response headers.
type ResponseHeader map[string]string

// NewResponse creates a new Response with defaults.
func NewResponse() *Response {
	return &Response{
		StatusCode: 200,
		Header:     make(ResponseHeader),
		Body:       []byte{},
	}
}

// =============================================================================
// RequestContext
// =============================================================================

// RequestContext holds request data and state for the entire request lifecycle.
// Invariant: Single instance per request; not shared across requests.
type RequestContext struct {
	// Public fields
	Request  *Request
	Response *Response
	Params   interface{} // url.Values or map[string]string
	LuaVM    LuaAPI      // Lua VM reference for coroutine operations

	// Internal state - for middleware chain
	index    int
	handlers []Handler
}

// Write sends a string response.
func (ctx *RequestContext) Write(data string) {
	ctx.Response.Body = append(ctx.Response.Body, data...)
}

// WriteJSON sends a JSON response.
func (ctx *RequestContext) WriteJSON(v interface{}) error {
	data, err := encodeJSON(v)
	if err != nil {
		return err
	}
	if h, ok := ctx.Response.Header.(ResponseHeader); ok {
		h["Content-Type"] = "application/json"
	}
	ctx.Response.Body = data
	return nil
}

// Redirect sends a redirect response.
func (ctx *RequestContext) Redirect(url string, code int) {
	ctx.Response.StatusCode = code
	if h, ok := ctx.Response.Header.(ResponseHeader); ok {
		h["Location"] = url
	}
}

// Next advances to the next handler in the middleware chain.
func (ctx *RequestContext) Next() error {
	ctx.index++
	if ctx.index >= len(ctx.handlers) {
		return nil
	}
	handler := ctx.handlers[ctx.index]
	return handler.Handle(ctx)
}

// GetParam returns a path parameter by name.
func (ctx *RequestContext) GetParam(name string) string {
	switch p := ctx.Params.(type) {
	case url.Values:
		return p.Get(name)
	case map[string]string:
		return p[name]
	}
	return ""
}

// GetQuery returns a query parameter by name.
func (ctx *RequestContext) GetQuery(name string) string {
	if u, ok := ctx.Request.URL.(*url.URL); ok {
		return u.Query().Get(name)
	}
	return ""
}

// GetHeader returns a request header by name.
func (ctx *RequestContext) GetHeader(name string) string {
	switch h := ctx.Request.Header.(type) {
	case http.Header:
		if vals := h[name]; len(vals) > 0 {
			return vals[0]
		}
	case map[string]string:
		return h[name]
	case ResponseHeader:
		return h[name]
	}
	return ""
}

// SetStatus sets the response status code.
func (ctx *RequestContext) SetStatus(code int) {
	ctx.Response.StatusCode = code
}

// SetHeader sets a response header.
func (ctx *RequestContext) SetHeader(name, value string) {
	if h, ok := ctx.Response.Header.(ResponseHeader); ok {
		h[name] = value
	}
}

// GetBody returns the request body as string.
func (ctx *RequestContext) GetBody() string {
	return string(ctx.Request.Body)
}

// Abort stops the middleware chain and sends an error response.
func (ctx *RequestContext) Abort(code int, message string) error {
	ctx.Response.StatusCode = code
	ctx.Response.Body = []byte(message)
	return fmt.Errorf("%s", message)
}

// NewRequestContext creates a new RequestContext with defaults.
func NewRequestContext() *RequestContext {
	return &RequestContext{
		Response: NewResponse(),
		Params:   make(url.Values),
	}
}

// Why embed handlers in context?
// - Enables recursive middleware execution
// - ctx:next() pattern from Express/Koa

// encodeJSON provides basic JSON encoding.
func encodeJSON(v interface{}) ([]byte, error) {
	switch val := v.(type) {
	case string:
		return []byte(fmt.Sprintf("%q", val)), nil
	case int, int8, int16, int32, int64:
		return []byte(fmt.Sprintf("%d", val)), nil
	case float32, float64:
		return []byte(fmt.Sprintf("%g", val)), nil
	case bool:
		if val {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case nil:
		return []byte("null"), nil
	case []interface{}:
		return encodeJSONArray(val)
	case map[string]interface{}:
		return encodeJSONObject(val)
	default:
		return []byte(fmt.Sprintf("%v", val)), nil
	}
}

func encodeJSONArray(arr []interface{}) ([]byte, error) {
	result := "["
	for i, v := range arr {
		if i > 0 {
			result += ","
		}
		data, err := encodeJSON(v)
		if err != nil {
			return nil, err
		}
		result += string(data)
	}
	result += "]"
	return []byte(result), nil
}

func encodeJSONObject(obj map[string]interface{}) ([]byte, error) {
	result := "{"
	first := true
	for k, v := range obj {
		if !first {
			result += ","
		}
		first = false
		result += fmt.Sprintf("%q:", k)
		data, err := encodeJSON(v)
		if err != nil {
			return nil, err
		}
		result += string(data)
	}
	result += "}"
	return []byte(result), nil
}

// =============================================================================
// Middleware
// =============================================================================

// Middleware processes requests before/after handlers.
// Implements the onion model: pre -> next -> post.
type Middleware interface {
	// Process handles the request.
	// next: the next handler (middleware or final handler)
	// Returns error to abort chain.
	Process(ctx *RequestContext, next Handler) error
}

// MiddlewareFunc allows plain functions to implement Middleware.
type MiddlewareFunc func(ctx *RequestContext, next Handler) error

func (f MiddlewareFunc) Process(ctx *RequestContext, next Handler) error {
	return f(ctx, next)
}

// Built-in middleware factories.
var (
	// Logger returns a middleware that logs requests.
	Logger Middleware = &loggerMiddleware{}

	// Recover returns a middleware that recovers from panics.
	Recover Middleware = &recoverMiddleware{}

	// CORS returns a middleware that handles CORS.
	CORS Middleware = &corsMiddleware{}
)

type loggerMiddleware struct{}

func (m *loggerMiddleware) Process(ctx *RequestContext, next Handler) error {
	return next.Handle(ctx)
}

type recoverMiddleware struct{}

func (m *recoverMiddleware) Process(ctx *RequestContext, next Handler) error {
	defer func() {
		if r := recover(); r != nil {
			ctx.Response.StatusCode = 500
			ctx.Response.Body = []byte(fmt.Sprintf("Internal Server Error: %v", r))
		}
	}()
	return next.Handle(ctx)
}

type corsMiddleware struct{}

func (m *corsMiddleware) Process(ctx *RequestContext, next Handler) error {
	// Handle preflight
	if ctx.Request != nil && ctx.Request.Method == "OPTIONS" {
		ctx.Response.StatusCode = 204
		if h, ok := ctx.Response.Header.(ResponseHeader); ok {
			h["Access-Control-Allow-Origin"] = "*"
			h["Access-Control-Allow-Methods"] = "GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS"
			h["Access-Control-Allow-Headers"] = "Content-Type, Authorization"
		}
		return nil
	}

	err := next.Handle(ctx)

	// Add CORS headers
	if h, ok := ctx.Response.Header.(ResponseHeader); ok {
		h["Access-Control-Allow-Origin"] = "*"
	}
	return err
}

// =============================================================================
// RouteTable
// =============================================================================

// RouteTable provides a fluent routing API.
// Similar to chi, gin style chaining.
type RouteTable interface {
	// HTTP method routes.
	Get(pattern string, h Handler) RouteTable
	Post(pattern string, h Handler) RouteTable
	Put(pattern string, h Handler) RouteTable
	Delete(pattern string, h Handler) RouteTable
	Patch(pattern string, h Handler) RouteTable
	Head(pattern string, h Handler) RouteTable
	Options(pattern string, h Handler) RouteTable

	// Multiple methods for same route.
	Any(methods []string, pattern string, h Handler) RouteTable

	// Static file serving.
	Static(pattern, dir string) RouteTable

	// Register middleware.
	Use(mw ...Middleware) RouteTable

	// Nested route group.
	Group(prefix string, fn func(RouteTable)) RouteTable

	// Build returns the final Handler.
	// Post: Build() cannot be called twice.
	Build() Handler
}

// RouteTableImpl is the implementation of RouteTable.
type RouteTableImpl struct {
	routes   []*Route
	group    string
	handlers []Handler
	mws      []Middleware
}

// Route represents a single route.
type Route struct {
	Method  string
	Pattern string
	Handler Handler
}

// NewRouteTable creates a new RouteTable.
func NewRouteTable() RouteTable {
	return &RouteTableImpl{
		routes:   make([]*Route, 0),
		handlers: make([]Handler, 0),
		mws:      make([]Middleware, 0),
	}
}

func (r *RouteTableImpl) Get(pattern string, h Handler) RouteTable {
	return r.addRoute("GET", pattern, h)
}

func (r *RouteTableImpl) Post(pattern string, h Handler) RouteTable {
	return r.addRoute("POST", pattern, h)
}

func (r *RouteTableImpl) Put(pattern string, h Handler) RouteTable {
	return r.addRoute("PUT", pattern, h)
}

func (r *RouteTableImpl) Delete(pattern string, h Handler) RouteTable {
	return r.addRoute("DELETE", pattern, h)
}

func (r *RouteTableImpl) Patch(pattern string, h Handler) RouteTable {
	return r.addRoute("PATCH", pattern, h)
}

func (r *RouteTableImpl) Head(pattern string, h Handler) RouteTable {
	return r.addRoute("HEAD", pattern, h)
}

func (r *RouteTableImpl) Options(pattern string, h Handler) RouteTable {
	return r.addRoute("OPTIONS", pattern, h)
}

func (r *RouteTableImpl) Any(methods []string, pattern string, h Handler) RouteTable {
	for _, method := range methods {
		r.addRoute(strings.ToUpper(method), pattern, h)
	}
	return r
}

func (r *RouteTableImpl) Static(pattern, dir string) RouteTable {
	return r
}

func (r *RouteTableImpl) Use(mw ...Middleware) RouteTable {
	r.mws = append(r.mws, mw...)
	return r
}

func (r *RouteTableImpl) Group(prefix string, fn func(RouteTable)) RouteTable {
	group := &RouteTableImpl{
		group:  r.group + prefix,
		routes: make([]*Route, 0),
		mws:    make([]Middleware, 0),
	}
	fn(group)
	r.routes = append(r.routes, group.routes...)
	return r
}



// BuildHandlerChain creates a handler chain from middlewares and final handler.
func BuildHandlerChain(middlewares []Middleware, final Handler) Handler {
	handlers := make([]Handler, len(middlewares)+1)
	for i, mw := range middlewares {
		handlers[i] = &middlewareAdapter{mw: mw}
	}
	handlers[len(handlers)-1] = final

	return HandlerFunc(func(ctx *RequestContext) error {
		ctx.handlers = handlers
		return ctx.Next()
	})
}

func (r *RouteTableImpl) Build() Handler {
	handlers := make([]Handler, 0, len(r.mws)+1)
	for _, mw := range r.mws {
		handlers = append(handlers, &middlewareAdapter{mw: mw})
	}
	handlers = append(handlers, &routerHandler{routes: r.routes})

	return HandlerFunc(func(ctx *RequestContext) error {
		ctx.handlers = handlers
		return ctx.Next()
	})
}

// middlewareAdapter wraps a Middleware as a Handler.
type middlewareAdapter struct {
	mw Middleware
}

func (a *middlewareAdapter) Handle(ctx *RequestContext) error {
	// Find next handler index
	nextIdx := -1
	for i, h := range ctx.handlers {
		if h == a {
			nextIdx = i + 1
			break
		}
	}

	var next Handler
	if nextIdx >= 0 && nextIdx < len(ctx.handlers) {
		next = ctx.handlers[nextIdx]
	}

	return a.mw.Process(ctx, next)
}

func (r *RouteTableImpl) addRoute(method, pattern string, h Handler) RouteTable {
	r.routes = append(r.routes, &Route{
		Method:  method,
		Pattern: r.group + pattern,
		Handler: h,
	})
	return r
}

type routerHandler struct {
	routes []*Route
}

func (h *routerHandler) Handle(ctx *RequestContext) error {
	method := ctx.Request.Method
	path := ctx.Request.Path

	for _, route := range h.routes {
		if route.Method != method && route.Method != "ANY" {
			continue
		}

		params, ok := matchRoute(route.Pattern, path)
		if ok {
			ctx.Params = params
			return route.Handler.Handle(ctx)
		}
	}

	ctx.Response.StatusCode = 404
	ctx.Write("Not Found")
	return nil
}

func matchRoute(pattern, path string) (map[string]string, bool) {
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	if len(patternParts) != len(pathParts) {
		return nil, false
	}

	params := make(map[string]string)
	for i, part := range patternParts {
		if strings.HasPrefix(part, ":") {
			params[part[1:]] = pathParts[i]
		} else if part != pathParts[i] && part != "*" {
			return nil, false
		}
	}

	return params, true
}

// Why chainable API over config table?
// - More idiomatic Go
// - Type-safe, compile-time checking
// - Clear intent

// =============================================================================
// Async HTTP Client
// =============================================================================

// HTTPClient provides non-blocking HTTP requests.
// Implementation: Lua script calls -> yield -> Go executes -> Resume.
type HTTPClient interface {
	// Get 发起 GET 请求 (非阻塞).
	Get(url string, headers map[string]string) (*HTTPResponse, error)

	// Post 发起 POST 请求 (非阻塞).
	Post(url string, headers map[string]string, body []byte) (*HTTPResponse, error)

	// Put 发起 PUT 请求 (非阻塞).
	Put(url string, headers map[string]string, body []byte) (*HTTPResponse, error)

	// Delete 发起 DELETE 请求 (非阻塞).
	Delete(url string, headers map[string]string) (*HTTPResponse, error)
}

// HTTPResponse is the response from HTTPClient.
type HTTPResponse struct {
	StatusCode int
	Header     map[string]string
	Body       []byte
}

// Why non-blocking HTTP client?
// - Avoid blocking Lua VM
// - Similar to nginx lua cosocket
// - Enable concurrent requests

// =============================================================================
// WebApp (Main Application Interface)
// =============================================================================

// WebApp is the main web application interface.
// Wraps HTTP framework with Lua scripting support.
type WebApp interface {
	// HTTP method routes - handler can be Handler, HandlerFunc, or Lua script path.
	GET(pattern string, handler interface{}) WebApp
	POST(pattern string, handler interface{}) WebApp
	PUT(pattern string, handler interface{}) WebApp
	DELETE(pattern string, handler interface{}) WebApp
	PATCH(pattern string, handler interface{}) WebApp
	HEAD(pattern string, handler interface{}) WebApp
	OPTIONS(pattern string, handler interface{}) WebApp

	// Middleware registration.
	// Can be Middleware, MiddlewareFunc, or Lua script path for Lua middleware.
	Use(middleware ...interface{}) WebApp

	// Static file serving.
	Static(pattern, root string) WebApp

	// Nested route group.
	Group(prefix string, fn func(WebApp)) WebApp

	// Run starts the HTTP server.
	Run(addr string) error
}

// WebAppImpl is the implementation of WebApp.
type WebAppImpl struct {
	server     *http.Server
	middleware []Middleware
	routes     map[string]map[string]Handler
	LuaVM      LuaAPI
}

// NewWebApp creates a new WebApp with the given Lua VM.
func NewWebApp(L LuaAPI) WebApp {
	return &WebAppImpl{
		routes:     make(map[string]map[string]Handler),
		LuaVM:      L,
		middleware: make([]Middleware, 0),
	}
}

func (app *WebAppImpl) GET(pattern string, handler interface{}) WebApp {
	return app.addRoute("GET", pattern, handler)
}

func (app *WebAppImpl) POST(pattern string, handler interface{}) WebApp {
	return app.addRoute("POST", pattern, handler)
}

func (app *WebAppImpl) PUT(pattern string, handler interface{}) WebApp {
	return app.addRoute("PUT", pattern, handler)
}

func (app *WebAppImpl) DELETE(pattern string, handler interface{}) WebApp {
	return app.addRoute("DELETE", pattern, handler)
}

func (app *WebAppImpl) PATCH(pattern string, handler interface{}) WebApp {
	return app.addRoute("PATCH", pattern, handler)
}

func (app *WebAppImpl) HEAD(pattern string, handler interface{}) WebApp {
	return app.addRoute("HEAD", pattern, handler)
}

func (app *WebAppImpl) OPTIONS(pattern string, handler interface{}) WebApp {
	return app.addRoute("OPTIONS", pattern, handler)
}

func (app *WebAppImpl) addRoute(method, pattern string, handler interface{}) WebApp {
	h := app.wrapHandler(handler)
	if _, ok := app.routes[method]; !ok {
		app.routes[method] = make(map[string]Handler)
	}
	app.routes[method][pattern] = h
	return app
}

func (app *WebAppImpl) wrapHandler(h interface{}) Handler {
	switch v := h.(type) {
	case Handler:
		return v
	case HandlerFunc:
		return v
	case func(*RequestContext) error:
		return HandlerFunc(v)
	default:
		panic(fmt.Sprintf("unsupported handler type: %T", h))
	}
}

func (app *WebAppImpl) Use(middleware ...interface{}) WebApp {
	for _, m := range middleware {
		switch v := m.(type) {
		case Middleware:
			app.middleware = append(app.middleware, v)
		case MiddlewareFunc:
			app.middleware = append(app.middleware, v)
		case func(*RequestContext, Handler) error:
			app.middleware = append(app.middleware, MiddlewareFunc(v))
		default:
			panic(fmt.Sprintf("unsupported middleware type: %T", m))
		}
	}
	return app
}

func (app *WebAppImpl) Static(pattern, root string) WebApp {
	return app
}

func (app *WebAppImpl) Group(prefix string, fn func(WebApp)) WebApp {
	group := &WebAppImpl{
		routes:     make(map[string]map[string]Handler),
		LuaVM:      app.LuaVM,
		middleware: app.middleware,
	}
	fn(group)
	for method, routes := range group.routes {
		if _, ok := app.routes[method]; !ok {
			app.routes[method] = make(map[string]Handler)
		}
		for pattern, handler := range routes {
			app.routes[method][prefix+pattern] = handler
		}
	}
	return app
}

func (app *WebAppImpl) Run(addr string) error {
	handler := app.buildHandler()
	app.server = &http.Server{Addr: addr, Handler: handler}
	return app.server.ListenAndServe()
}

func (app *WebAppImpl) buildHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req := &Request{
			Method: r.Method,
			Path:   r.URL.Path,
			URL:    r.URL,
			Header: r.Header,
		}

		ctx := &RequestContext{
			Request:  req,
			Response: NewResponse(),
			Params:   r.URL.Query(),
		}

		// Find the route handler
		var routeHandler Handler
		if routeMethod, ok := app.routes[r.Method]; ok {
			if handler, ok := routeMethod[r.URL.Path]; ok {
				routeHandler = handler
			} else {
				for pattern, handler := range routeMethod {
					if params, ok := matchRoute(pattern, r.URL.Path); ok {
						ctx.Params = params
						routeHandler = handler
						break
					}
				}
			}
		}

		if routeHandler == nil {
			routeHandler = HandlerFunc(func(ctx *RequestContext) error {
				ctx.Response.StatusCode = 404
				ctx.Write("Not Found")
				return nil
			})
		}

		// Build the handler chain using the helper
		chain := BuildHandlerChain(app.middleware, routeHandler)

		// Execute the chain
		chain.Handle(ctx)

		if h, ok := ctx.Response.Header.(ResponseHeader); ok {
			for k, v := range h {
				w.Header().Set(k, v)
			}
		}
		w.WriteHeader(ctx.Response.StatusCode)
		if ctx.Response.Body != nil {
			w.Write(ctx.Response.Body)
		}
	}
}

// =============================================================================
// Lua Library Registration
// =============================================================================

// WebLib is the web library interface for Lua registration.
type WebLib interface {
	// Open registers web functions in the Lua global table.
	Open(L LuaAPI) int
}

// OpenLibs registers all web libraries into the Lua VM.
func OpenLibs(L LuaAPI) {
	webLib := &webLibImpl{}
	// Use PushGoFunction and SetGlobal instead of RequireF for simpler module registration
	webLib.Open(L)
}

type webLibImpl struct{}

func (l *webLibImpl) Open(L LuaAPI) int {
	L.CreateTable(0, 16)

	L.PushGoFunction(func(L LuaAPI) int {
		webapp := NewWebApp(L)
		L.PushLightUserData(webapp)
		return 1
	})
	L.SetField(-2, "new")

	L.PushLightUserData(Logger)
	L.SetField(-2, "logger")

	L.PushLightUserData(CORS)
	L.SetField(-2, "cors")

	L.PushLightUserData(Recover)
	L.SetField(-2, "recover")

	L.CreateTable(0, 8)
	L.PushGoFunction(func(L LuaAPI) int {
		L.CreateTable(0, 3)
		L.PushInteger(200)
		L.SetField(-2, "status")
		L.PushString("OK")
		L.SetField(-2, "body")
		return 1
	})
	L.SetField(-2, "get")

	L.SetGlobal("web")
	return 1
}

// =============================================================================
// Async Handler
// =============================================================================

// AsyncHandler adapts Handler to framework-specific handler.
type AsyncHandler struct {
	ParentVM  LuaAPI
	Handler   Handler
	LuaScript string
	FuncName  string
}

func (h *AsyncHandler) ServeHTTP(w, r interface{}) {
	respWriter, ok := w.(http.ResponseWriter)
	if !ok {
		return
	}
	req, ok := r.(*http.Request)
	if !ok {
		return
	}

	reqObj := &Request{
		Method: req.Method,
		Path:   req.URL.Path,
		URL:    req.URL,
		Header: req.Header,
	}

	ctx := &RequestContext{
		Request:  reqObj,
		Response: NewResponse(),
		Params:   req.URL.Query(),
		LuaVM:    h.ParentVM.NewThread(),
	}

	if h.Handler != nil {
		h.Handler.Handle(ctx)
	}

	if h, ok := ctx.Response.Header.(ResponseHeader); ok {
		for k, v := range h {
			respWriter.Header().Set(k, v)
		}
	}
	respWriter.WriteHeader(ctx.Response.StatusCode)
	if ctx.Response.Body != nil {
		respWriter.Write(ctx.Response.Body)
	}
}

func (h *AsyncHandler) HandleRequest(ctx *RequestContext) {
	if h.Handler != nil {
		h.Handler.Handle(ctx)
	}
}

// =============================================================================
// LuaContextBridge
// =============================================================================

// LuaContextBridge provides methods to interact between Lua and Go contexts.
type LuaContextBridge interface {
	InjectContext(L LuaAPI, ctx *RequestContext)
	ExtractYieldType(L LuaAPI) YieldPoint
	PushResult(L LuaAPI, v interface{})
	GetYieldArgs(L LuaAPI) interface{}
}

// LuaContextBridgeImpl is the implementation.
type LuaContextBridgeImpl struct{}

// NewLuaContextBridge creates a new bridge instance.
func NewLuaContextBridge() LuaContextBridge {
	return &LuaContextBridgeImpl{}
}

func (b *LuaContextBridgeImpl) InjectContext(L LuaAPI, ctx *RequestContext) {
	L.PushLightUserData(ctx)
	L.SetGlobal("__web_ctx")

	L.CreateTable(0, 8)
	L.PushString(ctx.Request.Method)
	L.SetField(-2, "method")
	L.PushString(ctx.Request.Path)
	L.SetField(-2, "path")
	L.PushString(string(ctx.Request.Body))
	L.SetField(-2, "body")
	L.SetGlobal("ctx")
}

func (b *LuaContextBridgeImpl) ExtractYieldType(L LuaAPI) YieldPoint {
	L.GetGlobal("__yield_type")
	if L.IsNil(-1) {
		L.Pop()
		return YieldNone
	}
	yt, _ := L.ToInteger(-1)
	L.Pop()
	return YieldPoint(yt)
}

func (b *LuaContextBridgeImpl) PushResult(L LuaAPI, v interface{}) {
	switch val := v.(type) {
	case string:
		L.PushString(val)
	case int:
		L.PushInteger(int64(val))
	case int64:
		L.PushInteger(val)
	case float64:
		L.PushNumber(val)
	case bool:
		L.PushBoolean(val)
	case []byte:
		L.PushString(string(val))
	default:
		L.PushNil()
	}
}

func (b *LuaContextBridgeImpl) GetYieldArgs(L LuaAPI) interface{} {
	return nil
}
