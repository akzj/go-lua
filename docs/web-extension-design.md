# Web Extension Design for go-lua

## 概述

本设计文档描述 go-lua 的 Web 扩展方案，类比 nginx + lua 的模式，使用 **Gin** 作为 HTTP 底层框架，使 Go+Lua 能用于 Web 开发。

## 设计原则

1. **非阻塞 I/O** - 禁止同步阻塞 Lua VM 的 I/O 操作
2. **协程模型** - 使用 Lua Resume/Yield 实现异步处理
3. **不破坏现有 API** - 所有扩展都是纯新增
4. **Gin 集成** - 使用 Gin 处理 HTTP 底层，Lua 处理业务逻辑
5. **Nginx+Lua 风格** - 简洁、链式中间件、请求上下文共享

---

## 1. 模块结构

```
web/
├── api/
│   └── api.go          # 接口定义
├── internal/
│   ├── server.go       # Gin 集成层
│   ├── handler.go      # Lua Handler 适配
│   ├── middleware.go   # Lua 中间件链
│   ├── context.go      # RequestContext
│   ├── router.go       # 路由表 (生成 Gin 路由)
│   ├── lua_handler.go  # Lua 脚本执行器
│   └── lua_lib.go      # Lua 暴露的 web/http API
└── lib/
    └── web.go          # 导出 var Web = internal.NewWeb()
```

---

## 2. 核心接口

### 2.1 WebApp (Gin 适配层)

```go
// WebApp 是 Gin + Lua 的整合应用
// 使用 Gin 处理 HTTP 路由和中间件，Lua 处理请求逻辑
type WebApp interface {
    // HTTP 方法路由 - 接受 Lua 脚本或 Handler
    Get(pattern string, handler interface{}) WebApp   // Lua function or Handler
    Post(pattern string, handler interface{}) WebApp
    Put(pattern string, handler interface{}) WebApp
    Delete(pattern string, handler interface{}) WebApp
    Patch(pattern string, handler interface{}) WebApp
    Head(pattern string, handler interface{}) WebApp
    Options(pattern string, handler interface{}) WebApp
    
    // 中间件
    Use(middleware ...interface{}) WebApp  // Gin middleware or Lua function
    
    // 静态文件
    Static(pattern, root string) WebApp
    
    // 嵌套路由组
    Group(prefix string) WebApp
    
    // 运行服务器
    Run(addr string) error
    
    // 获取 Gin Engine (用于高级配置)
    Engine() *gin.Engine
}

// 为什么使用 Gin 而不是纯 Go?
// - Gin 提供成熟的路由、中间件、参数解析
// - 减少重复造轮子
// - 与现有 Go Web 开发习惯一致
```

### 2.2 LuaHandler (Lua 脚本执行器)

```go
// LuaHandler 将 Lua 函数适配为 Gin HandlerFunc
// Invariant: 每次请求创建新的 Lua State (协程)
type LuaHandler struct {
    ParentVM  api.LuaAPI  // 父 Lua VM (共享代码/库)
    Script    string       // Lua 脚本路径或代码
    FuncName  string       // 函数名
}

// Handle 实现 gin.HandlerFunc
// 流程: 解析请求 -> 创建协程 -> 注入上下文 -> Resume -> 处理 yield -> 返回响应
func (h *LuaHandler) Handle(c *gin.Context) {
    // 1. 解析请求到 RequestContext
    ctx := parseGinContext(c)
    
    // 2. 创建 Lua State (协程)
    childVM := h.ParentVM.NewThread()
    
    // 3. 注入 ctx 到 Lua
    injectLuaContext(childVM, ctx)
    
    // 4. 调用 Lua 函数
    pushLuaFunction(childVM, h.FuncName)
    pushRequestContext(childVM, ctx)
    err := childVM.Call(1, 0)  // 调用 (ctx)
    
    // 5. 处理 yield (异步 I/O)
    for isYield(err) {
        yieldType := getYieldType(childVM)
        switch yieldType {
        case YieldHTTPRequest:
            h.handleHTTPRequest(childVM, c)
        case YieldWebSocket:
            h.handleWebSocket(childVM, c)
        // ...
        }
        err = childVM.Resume()
    }
    
    // 6. 写入 Gin 响应
    writeGinResponse(c, ctx.Response)
}

// 为什么每次请求一个 Lua State?
// - 协程隔离，无并发问题
// - 每个请求独立的内存空间
// - 与 Lua coroutine semantics 一致
```

### 2.3 RequestContext

```go
// RequestContext 保存请求数据，在 Lua 和 Go 间共享
// 写入: Go (解析请求), Lua (业务逻辑)
// 读取: Go (写入响应), Lua (读取请求)
type RequestContext struct {
    // Gin 引用 (用于高级操作)
    Gin *gin.Context
    
    // 请求数据
    Request *WebRequest
    
    // 响应数据 (Lua 写入)
    Response *WebResponse
    
    // 路径参数 (由 Gin 解析)
    Params map[string]string
    
    // Lua State 引用
    LuaVM api.LuaAPI
}

// WebRequest HTTP 请求 (只读)
type WebRequest struct {
    Method  string
    Path    string
    URL     *url.URL
    Header  map[string]string  // 简化: Go map
    Body    []byte
    Query   map[string]string // Query string
    Form    map[string]string // Form data
    JSON    interface{}        // 解析后的 JSON
}

// WebResponse HTTP 响应 (Lua 写入)
type WebResponse struct {
    StatusCode int
    Header     map[string]string
    Body       *bytes.Buffer
}

// NewWebResponse 创建默认响应
func NewWebResponse() *WebResponse {
    return &WebResponse{
        StatusCode: 200,
        Header:     make(map[string]string),
        Body:       bytes.NewBuffer(nil),
    }
}
```

### 2.4 中间件接口

```go
// LuaMiddleware 将 Lua 函数适配为 Gin 中间件
type LuaMiddleware struct {
    Func func(ctx *RequestContext) error  // Lua 函数签名
}

// Process 实现 Middleware 接口
func (m *LuaMiddleware) Process(c *gin.Context) {
    ctx := parseGinContext(c)
    err := m.Func(ctx)
    if err != nil {
        c.AbortWithStatus(500)
    }
}

// 内置 Go 中间件工厂
func Logger() gin.HandlerFunc           // 请求日志
func Recover() gin.HandlerFunc          // 异常恢复
func CORS() gin.HandlerFunc             // 跨域
func Timeout(d time.Duration) gin.HandlerFunc  // 超时
func.Gzip() gin.HandlerFunc             // Gzip 压缩
```

---

## 3. Lua API 设计

### 3.1 web 模块 (web.lua)

```lua
local web = require("web")

-- 创建应用 (返回 Gin wrapper)
local app = web.new()

-- 注册路由 (Lua 函数作为处理器)
app:get("/hello/:name", function(ctx)
    local name = ctx.params.name
    ctx:write("Hello, " .. name)
end)

app:post("/api/users", function(ctx)
    local data = ctx:json()  -- 解析 JSON body
    local res = http.post("http://internal/api/users", {body = json.encode(data)})
    ctx:write_json(res)
end)

-- 中间件 (可以是 Go 或 Lua 函数)
app:use(web.cors())                    -- Go 中间件
app:use(function(ctx, next)            -- Lua 中间件
    print("before")
    next(ctx)
    print("after")
end)

-- 路由组
app:group("/api", function(api)
    api:get("/users", handler)
    api:post("/users", handler)
end)

-- 启动
app:run("127.0.0.1:8080")
```

### 3.2 RequestContext Lua 方法

```lua
-- ctx:write(body)           写入响应体
-- ctx:write(body, status)   写入响应体和状态码
-- ctx:write_json(data)      写入 JSON
-- ctx:redirect(url)         重定向
-- ctx:json()                解析 JSON body
-- ctx:query(key)            获取 query 参数
-- ctx:form(key)             获取 form 参数

-- ctx.params                路径参数 table
-- ctx.request.method        请求方法
-- ctx.request.path          请求路径
-- ctx.request.header        请求头 table
-- ctx.request.body          请求体
-- ctx.request.query         query 参数 table
```

### 3.3 http 模块 (非阻塞 HTTP Client)

```lua
local http = require("web.http")

-- GET 请求 (内部 yield，不阻塞 VM)
local res = http.get("http://example.com/api")
print(res.status)
print(res.body)

-- POST 请求
local res = http.post("http://example.com/api", {
    headers = {["Content-Type"] = "application/json"},
    body = '{"name":"test"}'
})

-- 并发请求 (非阻塞)
local res1 = http.get("http://api1.com/data")
local res2 = http.get("http://api2.com/data")
-- 两个请求同时进行，通过 yield 实现
```

### 3.4 session 模块

```lua
local session = require("web.session")

-- 使用 session
session.start(ctx)
session.set("user_id", 123)
local user_id = session.get("user_id")
session.destroy()
```

---

## 4. 与 Gin 集成

### 4.1 GinMiddleware (Gin 中间件注册)

```go
// GinMiddleware 注册 Lua 中间件到 Gin
func (app *WebApp) Use(middleware ...interface{}) {
    for _, m := range middleware {
        switch v := m.(type) {
        case gin.HandlerFunc:
            app.engine.Use(v)
        case *LuaMiddleware:
            app.engine.Use(gin.HandlerFunc(v.Handle))
        case func(*RequestContext) error:  // Lua 函数签名
            app.engine.Use(gin.HandlerFunc(func(c *gin.Context) {
                ctx := parseGinContext(c)
                if err := v(ctx); err != nil {
                    c.AbortWithStatus(500)
                }
            }))
        }
    }
}
```

### 4.2 GinHandler (Gin 路由注册)

```go
// GinHandler 注册 Lua Handler 到 Gin
func (app *WebApp) Get(pattern string, handler interface{}) WebApp {
    app.engine.Handle("GET", pattern, app.wrapHandler(handler))
    return app
}

func (app *WebApp) wrapHandler(h interface{}) gin.HandlerFunc {
    switch v := h.(type) {
    case gin.HandlerFunc:
        return v
    case *LuaHandler:
        return v.Handle
    case func(*RequestContext):  // Lua handler 签名
        return func(c *gin.Context) {
            ctx := parseGinContext(c)
            v(ctx)
        }
    }
    panic("unsupported handler type")
}
```

---

## 5. 异步模型 (Coroutine + Gin)

### 5.1 设计

```
Gin 接收请求
    ↓
创建 RequestContext
    ↓
创建 Lua State (协程)
    ↓
注入 ctx 到 Lua 全局
    ↓
Lua 函数执行
    ↓ (yield)
Go 执行 I/O (HTTP, DB, File)
    ↓ (resume)
Lua 继续执行
    ↓
写入 Response
    ↓
Gin 发送响应
```

### 5.2 Yield 实现

```go
// YieldPoint 定义可 yield 的 I/O 操作
type YieldPoint int

const (
    YieldHTTPRequest  YieldPoint = 1
    YieldHTTPResponse YieldPoint = 2
    YieldWebSocket    YieldPoint = 3
    YieldDBQuery      YieldPoint = 4
    YieldFileRead     YieldPoint = 5
    YieldFileWrite    YieldPoint = 6
    YieldSleep        YieldPoint = 7
)

// Lua 函数 yield 时设置
// 在 Go 端检查并处理
func handleYield(L api.LuaAPI, c *gin.Context, ctx *RequestContext) {
    yield := getYieldType(L)
    switch yield {
    case YieldHTTPRequest:
        // 解析 Lua 栈获取 http.* 调用参数
        // 执行非阻塞 HTTP 请求
        // 将结果压入 Lua 栈
        // Resume Lua
    case YieldWebSocket:
        // Upgrade 到 WebSocket
        // 处理 WebSocket 消息
    }
}
```

---

## 6. 非阻塞 HTTP Client 实现

```go
// HTTPClient 使用协程池执行非阻塞 HTTP 请求
type HTTPClient struct {
    pool chan struct{}  // 限制并发数
}

// Get 执行 GET 请求 (内部协程，非阻塞)
func (c *HTTPClient) Get(url string, headers map[string]string) (*HTTPResponse, error) {
    // 获取协程
    // 执行请求
    // 返回结果
    // yield/resume 机制
}

// Lua 端调用
// local res = http.get(url, headers)  -- yield
// Lua VM 继续处理其他请求
// HTTP 完成，Resume Lua 协程
// res 被填充
```

---

## 7. 使用示例

### 7.1 Go 端 (main.go)

```go
package main

import (
    "github.com/akzj/go-lua/vm"
    "github.com/akzj/go-lua/web"
)

func main() {
    L := vm.NewLuaState()
    vm.OpenLibs(L)
    
    // 创建 Web 应用
    app := web.New(L)
    
    // 路由 (Lua 脚本)
    app.GET("/hello/:name", "handlers/hello.lua", "handle")
    
    // 中间件
    app.Use(web.Logger())
    app.Use(web.Recover())
    app.Use(web.CORS())
    
    // Lua 中间件
    app.UseLua("middleware/auth.lua", "auth")
    
    // 启动
    app.Run(":8080")
}
```

### 7.2 Lua 端 (handlers/hello.lua)

```lua
local web = require("web")

-- handler 函数
local function handle(ctx)
    local name = ctx.params.name
    
    -- 调用内部 API
    local res = http.get("http://internal:8081/users/" .. name)
    
    -- 写响应
    ctx:write_json({
        name = name,
        data = res.body
    })
end

return {handle = handle}
```

### 7.3 Lua 端 (app.lua - 完全 Lua 开发)

```lua
local web = require("web")

local app = web.new()

-- 路由
app:get("/", function(ctx)
    ctx:write("Welcome to go-lua web!")
end)

app:get("/api/users/:id", function(ctx)
    local id = ctx.params.id
    local user = db.query("SELECT * FROM users WHERE id = ?", id)
    ctx:write_json(user)
end)

app:post("/api/users", function(ctx)
    local data = ctx:json()
    db.execute("INSERT INTO users VALUES (?, ?)", data.name, data.email)
    ctx:write_json({success = true})
end)

-- 中间件
app:use(function(ctx, next)
    local token = ctx.request.header["Authorization"]
    if not token then
        ctx:write("Unauthorized", 401)
        return
    end
    next(ctx)
end)

app:use(web.logger())

-- 运行
app:run("0.0.0.0:8080")
```

---

## 8. 关键设计决策

| 决策 | 选择 | 原因 |
|------|------|------|
| HTTP 底层 | Gin | 成熟稳定，路由/中间件完善 |
| 异步模型 | Lua 协程 + yield | 类比 nginx lua，非阻塞 |
| 请求上下文 | Go 结构 + Lua table | Go 解析，Lua 消费 |
| 中间件 | Gin MW + Lua MW | 混合链，灵活 |
| HTTP Client | 协程池 + yield | 非阻塞，类似 cosocket |
| 路由 | Gin 路由 + Lua 脚本 | Gin 处理匹配，Lua 处理逻辑 |

---

## 9. 与现有模块集成

```go
// 复用 api.LuaAPI
type LuaHandler struct {
    ParentVM api.LuaAPI
}

// 复用 state.LuaState.NewThread()
childVM := parentVM.NewThread()

// 复用 mem.Allocator
// 内存管理保持一致
```

---

## 10. 非目标

1. **不支持热更新** - 需要进程重启
2. **不支持共享内存** - nginx lua 的特殊能力
3. **不支持 Lua 代码缓存** - 后续优化
4. **不支持 RPC** - 后续扩展
