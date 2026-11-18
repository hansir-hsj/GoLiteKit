# GoLiteKit

[English](readme.md) | [中文](readme.zh.md)

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.23-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org)
[![Version](https://img.shields.io/badge/version-v1.2.0-blue?style=for-the-badge)](CHANGELOG.md)
[![License](https://img.shields.io/badge/license-MIT-green?style=for-the-badge)](LICENSE)

轻量级 Go Web 框架，基于 `net/http`，专注简洁和清晰。

## 性能

性能基准测试仍在规划中。在加入可复现的 benchmark 套件之前，建议结合你自己的业务负载和部署环境评估 GoLiteKit。

未来的性能报告应包含 benchmark 代码、运行命令、机器环境、对比版本和 `benchstat` 输出。

## 特性

- **泛型控制器** — 通过 `BaseControllerOf[T]` 实现类型安全的请求绑定
- **HandlerFunc 路由** — 轻量级 `func(*Context) error` 处理器，无需控制器样板代码
- **中间件链** — 可组合、有序的中间件，支持路由组级别配置
- **请求绑定** — 开箱即用支持 JSON、form-urlencoded、multipart
- **SSE 支持** — 流式响应，自动 flush
- **内置中间件** — 日志、错误处理、超时、限流、gzip 压缩、log ID
- **可观测性** — 可选 OpenTelemetry 适配器，支持请求 span、业务 span 和 metrics
- **结构化日志** — 基于 `slog`，支持请求体日志（截断和脱敏）、日志轮转
- **自定义服务** — DB、Redis、Logger 通过 functional options 注入，并支持启动期注册自定义依赖
- **优雅生命周期** — `Start`、`ListenAndServe`（上下文感知）、`Shutdown` 可配置超时
- **Pprof 挂载** — 受保护的 pprof 端点，可选仅限本地回环访问
- **glk 脚手架** — 使用 `glk new` 快速创建项目

## 安装

```bash
go get github.com/hansir-hsj/GoLiteKit
```

需要 Go 1.23+。

## 快速开始

```go
package main

import (
    "context"
    "net/http"
    "os"
    "os/signal"

    glk "github.com/hansir-hsj/GoLiteKit"
)

type HelloController struct {
    glk.BaseController
}

func (c *HelloController) Serve(ctx context.Context) error {
    return c.JSON(http.StatusOK, map[string]string{"message": "hello, world"})
}

func main() {
    app := glk.NewApp()
    app.GET("/hello", &HelloController{})

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    app.ListenAndServe(ctx, glk.ServerConfig{Addr: ":8080"})
}
```

```bash
curl http://localhost:8080/hello
# {"message":"hello, world"}
```

## 请求绑定

定义请求结构体，使用 `BaseControllerOf[T]` —— 框架自动绑定 JSON、form-urlencoded 和 multipart。

```go
type CreateUserReq struct {
    Name  string `json:"name"  form:"name"`
    Email string `json:"email" form:"email"`
    Age   int    `json:"age"   form:"age"`
}

type CreateUserController struct {
	glk.BaseControllerOf[CreateUserReq]
}

func (c *CreateUserController) Serve(ctx context.Context) error {
    req := c.GetRequest()
    // req.Name, req.Email, req.Age 已自动填充
    return c.JSON(http.StatusOK, map[string]any{"created": req.Name})
}
```

### Controller 生命周期

Controller 请求按以下顺序执行：

```text
Init → ParseRequest → Validate → Serve → Finalize
```

`ParseRequest` 会在 `Validate` 之前绑定 JSON/form/multipart 数据，因此校验逻辑可以安全读取 `c.GetRequest()` 或 `c.Request`。认证、feature flag 等解析前检查建议放在 middleware 或 `Init`。

每个请求都会对注册时的 controller 原型做一次**浅拷贝**，得到一个新实例：

- 原型字段按值复制，指针、map、slice 字段在所有请求间指向同一份数据。它们只适合放共享的只读配置，以及可并发使用的依赖（如 `*sql.DB`、repository）。不要写入来自原型的 map 或 slice。
- `sync.Mutex` 等值类型的同步字段也会被复制，只能保护当前请求自己的实例。需要保护跨请求共享的状态时，请保存指向它的指针。
- `BaseControllerOf[T]` 中的 `Request` 在每次复制时都会被重置为零值。请求默认值请在 `Init` 中设置，`Init` 在 `ParseRequest` 之前执行。

如果 controller 需要请求级的可变状态或构造逻辑，请改为注册 `ControllerFactory`（或任意 `func() glk.Controller`），它会在每个请求调用一次：

```go
app.GET("/report", glk.ControllerFactory(func() glk.Controller {
    return &ReportController{repo: repo, rows: make([]Row, 0, 64)}
}))
```

## REST 控制器

`RestControllerOf[T]` 将每个响应封装为统一的 JSON 格式：

```json
{"status": 0, "msg": "OK", "data": ..., "logid": "..."}
```

```go
type ListUsersController struct {
    glk.RestController
}

func (c *ListUsersController) Serve(ctx context.Context) error {
    users := []string{"alice", "bob"}
    return c.ServeData(ctx, users)
}
```

`status` 是业务状态码：`0` 表示成功，其他任何值都表示失败，客户端统一用 `status == 0` 判断成败即可。HTTP 状态码只表达传输层结果。返回错误有两种方式：

- `return glk.ErrBadRequest("手机号格式错误", nil).WithStatus(10012)` 返回 HTTP 400 和 `{"status": 10012, ...}`。不调用 `WithStatus` 时，`status` 等于 HTTP 状态码（如 `404`）；panic（`500`）和未匹配的路由（`404`/`405`）也是这样。
- `c.ServeError(ctx, 10012, "手机号格式错误")` 返回 HTTP 200 和 `{"status": 10012, ...}`，适合客户端只看 `status` 的接口。`0` 不能用作错误码，传入 `0` 时会被替换为 `-1`；`ServeErrorMsg` 固定使用 `-1`。

业务码会记录在日志的 `biz_status` 字段中。

## 可观测性

GoLiteKit 在核心包中保留轻量抽象，并通过可选 `otel/` 子包接入 OpenTelemetry：

```go
import glkotel "github.com/hansir-hsj/GoLiteKit/otel"

app := glk.NewApp(
    glkotel.WithObservability(
        glkotel.WithTracerProvider(tracerProvider),
        glkotel.WithMeterProvider(meterProvider),
    ),
)
```

业务操作 span 显式通过 `context.Context` 创建：

```go
ctx, span := glk.StartSpan(ctx, "cache.lookup", glk.StringAttr("component", "cache"))
defer span.End()
```

span 名称和 metric label 必须保持稳定、低基数。不要把原始 SQL、原始 URL、用户 ID、trace ID、log ID 或路径参数值作为 metric label。

未传 `WithTracerProvider` / `WithMeterProvider` 时使用 OpenTelemetry 全局 provider（应用设置之前为 no-op）。instrumentation scope 为 `github.com/hansir-hsj/GoLiteKit/otel`，耗时单位为秒：

| 指标 | 单位 | 说明 |
|------|------|------|
| `http.server.request.duration` | s | semconv；label 为 `http.request.method`（非标准方法记为 `_OTHER`）、`http.response.status_code`、`http.route`、`url.scheme`、`error.type` |
| `glk.http.server.requests` | 1 | 请求数 |
| `glk.service.span.calls` / `glk.service.span.duration` | 1 / s | 通过 `glk.StartSpan` 创建的 span |
| `glk.http.server.duration_ms` / `glk.service.span.duration_ms` | ms | 已废弃，仍会上报；请把看板迁移到秒级指标 |

未匹配的请求（404/405）不带 `http.route`。

## 路径参数

```go
// 注册: app.GET("/users/{id}", &GetUserController{})

type GetUserController struct {
    glk.BaseController
}

func (c *GetUserController) Serve(ctx context.Context) error {
    id := c.PathValueInt("id", 0)
    return c.JSON(http.StatusOK, map[string]int{"id": id})
}
```

## 中间件

```go
// 自定义中间件
func AuthMiddleware(next glk.Handler) glk.Handler {
    return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
        if r.Header.Get("Authorization") == "" {
            return glk.ErrUnauthorized("missing token", nil)
        }
        return next(ctx, w, r)
    }
}

// 全局应用
app.Use(AuthMiddleware)
app.GET("/profile", &ProfileController{})

// 路由组应用
api := app.Group("/api")
api.Use(AuthMiddleware)
api.GET("/profile", &ProfileController{})
```

中间件必须先于路由、静态资源、pprof 端点或嵌套路由组注册。GoLiteKit 会在注册时预构建 middleware chain；如果在添加路由后再调用 `Use`，会直接 panic，避免认证、权限等中间件被误以为已经生效。路由和中间件注册应在应用启动阶段由单个 goroutine 完成。

`App` 会给每个路由套上默认中间件栈，从外到内依次为：

```
UseOuter 中间件 → [OpenTelemetry] → Logger → LogID → ErrorHandler → Timeout → ContextAs → Use 中间件 → 路由组中间件 → 路由
```

- `app.Use` 注册的中间件在错误处理之内：它们返回的错误会变成 JSON 错误响应；路由通过 `ctx.JSON` / `c.JSON` 设置的响应在路由返回时立即写出，所以这些中间件能看到响应体。
- 中间件可以不调用 `next`、直接设置响应（如 `return gcx.JSON(http.StatusForbidden, ...)`），但不能在 `next` 返回后替换路由的响应：此时响应已经写出，后设置的响应会被丢弃，并记录一条错误日志。需要改响应时，请在调用 `next` 之前决定，或者包装 `ResponseWriter`。
- `app.UseOuter` 注册的中间件包在整个默认栈外层，它们设置的响应头（如 CORS）对错误响应同样生效；它们返回的错误不经过 ErrorHandler，按纯文本 HTTP 错误写出。

### 压缩

```go
app.Use(glk.CompressionMiddleware()) // 可选压缩级别，默认 gzip.DefaultCompression
```

客户端接受 gzip 且响应体不小于 `DefaultCompressionMinSize`（1 KB）时压缩。HEAD 请求、1xx/204/304、部分内容（206）、已设置编码的响应，以及已压缩的类型（SVG 以外的图片、音视频、压缩包、字体、PDF）原样发送。不到 1 KB 就 flush 的流不压缩。压缩后的响应会把强 ETag 改为弱 ETag。

### 请求 ID

默认中间件栈会为每个请求分配一个 log ID：日志中记为 `logid` 字段，JSON 外层结构中返回 `logid` 字段，并写入 `X-Request-ID` 响应头（错误和 panic 响应同样带上）。log ID 始终由服务自己生成，客户端无法指定或撞上别人的 log ID。如果请求带有合法的 `X-Request-ID`（1-64 个 `[A-Za-z0-9._:-]` 字符），会记录为 `upstream_request_id` 字段，便于和网关或上游服务的日志关联；不合法的值直接忽略。

```go
// 网关总会设置 X-Request-ID：直接用它作为 log ID
app := glk.NewApp(glk.WithLogIDOptions(glk.LogIDOptions{TrustIncoming: true}))
```

只有所有请求都经过会设置或覆盖该请求头的代理时，才应开启 `TrustIncoming`。`LogIDOptions` 还支持 `Header`（自定义请求头名称）和 `OmitResponseHeader`（不写响应头）。基于 `traceparent` 的跨服务追踪由 OpenTelemetry 中间件负责，它会在日志中记录 `trace_id` 和 `span_id`。

## 限流

```go
limiter := glk.NewRateLimiter(
    10, 10, // 每个 key 10 req/s
    glk.WithGlobalRateLimiter(1000, 1000), // 全局 1000 req/s
    glk.WithTTL(5 * time.Minute),
    glk.WithMaxKeys(10000),
)
app.Use(limiter.RateLimiterAsMiddleware(glk.ByIP))
```

每个 key 的 limiter 默认带 TTL 和 key 数量上限。达到上限后会淘汰最久未使用的 key，因此大量新 key 不会把其他客户端挡在外面。只有当 key 集合天然有界时，才建议显式使用 `WithoutTTL()`。

key 函数：

- `glk.ByIP`：连接的对端 IP。在反向代理后面拿到的是代理的 IP。
- `glk.ByClientIP("127.0.0.1", "10.0.0.0/8")`：在给定的可信代理之后解析真实客户端 IP（仅当连接来自可信代理时才读取 `X-Forwarded-For` / `X-Real-IP`）。
- `glk.ByPath`：匹配到的路由模板（如 `GET /users/{id}`），每个路由共享一个限额。

## SSE 流式响应

```go
type StreamController struct {
    glk.BaseController
}

func (c *StreamController) Serve(ctx context.Context) error {
    sse := c.SSE()
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    for i := 0; i < 5; i++ {
        if err := sse.Send(glk.SSEvent{Data: fmt.Sprintf("message %d", i)}); err != nil {
            return err // 客户端已断开
        }
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
        }
    }
    return nil
}
```

使用 `NewAppFromConfig` 时，带 `Accept: text/event-stream` 的请求使用 `[HttpServer.SSE] timeout`（默认 300 秒）代替普通的 handler 超时；只有 handler 真正以 `text/event-stream` 响应时，才会延长 server 的写超时。

## 集成 DB 和 Redis

最小配置片段：

```go
import (
    glk "github.com/hansir-hsj/GoLiteKit"
    glkdb "github.com/hansir-hsj/GoLiteKit/db"
    glkredis "github.com/hansir-hsj/GoLiteKit/redis"
)

dbConn, _ := glkdb.NewFromConfig("db.toml")
rdb, _ := glkredis.NewFromConfig("redis.toml")
app := glk.NewApp(glk.WithDB(dbConn), glk.WithRedis(rdb))

// 客户端仍归调用方所有，app 不会关闭它们。需要随 app 一起关闭时
// （在 server 停止之后执行）：
app.OnClose(func(ctx context.Context) error {
    sqlDB, err := dbConn.DB()
    if err != nil {
        return err
    }
    return errors.Join(sqlDB.Close(), rdb.Close())
})

// 在控制器中使用
func (c *MyController) Serve(ctx context.Context) error {
    dbConn := c.DB() // *gorm.DB
    rdb := c.Redis() // *redis.Client
    // ...
    return nil
}
```

## HandlerFunc 路由

对于不需要完整控制器的简单端点：

```go
app.GET("/ping", glk.HandlerFunc(func(ctx *glk.Context) error {
    return ctx.JSON(http.StatusOK, map[string]string{"status": "ok"})
}))
```

## 自定义服务

在应用创建时注册自定义服务，并在请求处理中读取：

```go
app := glk.NewApp(
    glk.WithService("cache", myCache),
)

// 在 HandlerFunc 中使用
app.GET("/cache", glk.HandlerFunc(func(ctx *glk.Context) error {
    cache := ctx.Service("cache").(MyCache)
    // ...
    return ctx.JSON(http.StatusOK, map[string]string{"status": "ok"})
}))

// 在控制器中使用
func (c *MyController) Serve(ctx context.Context) error {
    cache := c.Service("cache").(MyCache)
    // ...
    return nil
}
```

自定义服务在请求处理中只读。请求级临时值请使用 request context data。

## 优雅关闭

使用 `ListenAndServe` 实现上下文感知的生命周期管理，自动优雅关闭：

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

app := glk.NewApp()
app.GET("/hello", &HelloController{})
app.ListenAndServe(ctx) // 阻塞直到收到信号，然后优雅关闭
```

或使用 `Start` + `Shutdown` 手动控制：

```go
app.Start()
// ... 做其他事情 ...
app.Shutdown(ctx)
app.Close(ctx) // 确定不再启动后释放资源
```

`ListenAndServe` 在 server 停止后会自动关闭 app；`Shutdown` 不会，以便 app 可以重新启动。`Close` 会按注册的逆序执行通过 `OnClose` 注册的清理函数，并关闭 `NewAppFromConfig` 创建的 logger。调用方传入的客户端和 logger（`WithDB`、`WithRedis`、`WithLogger` 等）仍归调用方所有，app 不会关闭；需要随 app 一起关闭时，用 `OnClose` 注册：

```go
app.OnClose(func(ctx context.Context) error {
    return tracerProvider.Shutdown(ctx) // 例如 flush OpenTelemetry span
})
```

如果直接使用底层 `Server`，`srv.Done()` 会返回 `Start` 后后台 `Serve` 的结果；正常关闭返回 `nil`，异常 listener/server 退出会返回对应错误。

同一个 `Server` 已在运行时，再次调用 `Start` 会返回 already-started 错误。

测试或自定义绑定地址时，可以传入显式 `ServerConfig`：

```go
app.Start(glk.ServerConfig{Addr: "127.0.0.1:0"})
```

超时和 header 限制字段为零值时，会继承 `DefaultServerConfig` 的安全默认值；因此只传 `Addr` 也会保留读写、请求头和空闲连接超时。

## Pprof

推荐把 pprof 放在单独的回环地址监听，这样无论是公网端口还是反向代理都访问不到：

```go
// ctx 取消时停止；返回实际监听地址
if _, err := glk.ServePprof(ctx, "127.0.0.1:6060"); err != nil {
    log.Fatal(err)
}
```

使用 `NewAppFromConfig` 时，配置 `enablePprof = true` 和 `pprofAddr = "127.0.0.1:6060"`，该监听会随 app 启动、在 `Close` 时停止。未配置 `pprofAddr` 时，pprof 挂载在 app 端口上，并仅允许本地回环访问。

也可以挂载到 app 路由上：

```go
app.MountPprof(glk.PprofOptions{
    LoopbackOnly: true, // 仅允许 127.0.0.1/::1，且不能带转发头
})
// 访问地址: /debug/pprof/
```

`LoopbackOnly` 会拒绝带有 `X-Forwarded-For`、`X-Real-IP` 或 `Forwarded` 的请求，因为同机反向代理转发的请求都来自 127.0.0.1。如果代理不添加这些头，仍然无法识别，所以更推荐单独监听。

## 配置文件

```toml
# app.toml
[HttpServer]
appName = "myapp"
runMode = "debug"
network = "tcp"
addr = ":8080"
enablePprof = false
pprofAddr = "127.0.0.1:6060" # 可选：pprof 单独监听

[HttpServer.Timeout]
readTimeout = 1000
readHeaderTimeout = 200
writeTimeout = 15000
idleTimeout = 5000
shutdownTimeout = 5000

[HttpServer.SSE]
timeout = 300000 # text/event-stream 请求的 handler 超时，毫秒

[HttpServer.Logger]
configFile = "logger.toml"

[HttpServer.DB]
configFile = "db.toml"

[HttpServer.Redis]
configFile = "redis.toml"
```

```go
app, err := glk.NewAppFromConfig("app.toml")
// 从 app 读取配置值，例如 app.Env().Addr()、app.Env().WriteTimeout()
```

加载的配置保存在 app 上，同一进程可以运行多个配置不同的 App。包级的 `env` 函数（`env.Addr()` 等）读取的是最近一次加载的配置，保留它们是为了兼容；推荐使用 `app.Env()`。`env.Load(path)` 可以读取配置而不改变这个默认值。

配置中的路径：`configFile`、`certFile`、`keyFile` 相对于 app 配置文件所在目录解析，因此从任何目录启动都能找到；`staticDir` 相对于启动时的工作目录解析。绝对路径原样使用。`db.NewFromConfig()` / `redis.NewFromConfig()` 不传参数时，优先使用 `[HttpServer.DB]` / `[HttpServer.Redis]` 的 `configFile`，未配置时使用 app 配置文件同目录下的 `db.toml` / `redis.toml`。脚手架生成的项目支持 `-config path/to/app.toml` 指定配置文件。

## 示例

| 目录 | 说明 |
|------|------|
| [examples/hello](examples/hello) | 最简单的 GET 接口 |
| [examples/rest_api](examples/rest_api) | REST 控制器：JSON 绑定 + 路径参数 |
| [examples/middleware](examples/middleware) | 自定义中间件 + 路由组 |
| [examples/sse](examples/sse) | SSE 流式响应 |

## glk 脚手架

安装：

```bash
go install github.com/hansir-hsj/GoLiteKit/glk@latest
```

| 命令 | 说明 |
|------|------|
| `glk version` | 显示 glk 版本 |
| `glk new <appName>` | 创建新的 GoLiteKit 项目 |
| `glk new <appName> --module <modulePath>` | 创建项目并指定自定义 Go module 路径 |
| `glk new <appName> --force` | 目录已存在时不询问，直接覆盖生成的文件 |
| `glk new <appName> --dry-run` | 只打印将要创建的文件 |
| `glk new <appName> --skip-tidy` | 不执行 `go mod tidy` |
| `glk add controller <name>` | 在 `./controller/` 下生成控制器文件 |
| `glk add middleware <name>` | 在 `./middleware/` 下生成中间件文件 |

`<appName>` 必须是当前目录下的相对路径。`glk new` 从不删除已有目录：目录非空时会先询问，再覆盖生成的文件，其他文件保留；写入失败时删除本次创建的文件和目录。`glk add` 不会覆盖已有文件。出错时退出码为 1。

示例：

```bash
# 创建新项目
glk new myapp

# 指定自定义 module 路径
glk new myapp --module github.com/myorg/myapp

# 生成控制器（snake_case 自动转为 CamelCase）
glk add controller user_profile   # → controller/user_profile_controller.go

# 生成中间件
glk add middleware request_id     # → middleware/request_id_middleware.go
```

## License

[MIT](LICENSE)
