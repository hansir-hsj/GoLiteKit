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
- **内置中间件** — 日志、错误处理、超时、限流、gzip 压缩、请求追踪
- **结构化日志** — 基于 `slog`，支持请求体日志（截断和脱敏）、日志轮转
- **服务注册表** — DB、Redis、Logger 通过 functional options 注入 + 通用 `Set/Get` 支持自定义服务
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

    glk "github.com/hansir-hsj/GoLiteKit"
)

type HelloController struct {
    glk.BaseController
}

func (c *HelloController) Serve(ctx context.Context) error {
    return c.ServeJSON(map[string]string{"message": "hello, world"})
}

func main() {
    app := glk.NewApp()
    app.GET("/hello", &HelloController{})
    app.Run(":8080")
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
    return c.ServeJSON(map[string]any{"created": req.Name})
}
```

### Controller 生命周期

Controller 请求按以下顺序执行：

```text
Init → ParseRequest → Validate → Serve → Finalize
```

`ParseRequest` 会在 `Validate` 之前绑定 JSON/form/multipart 数据，因此校验逻辑可以安全读取 `c.GetRequest()` 或 `c.Request`。认证、feature flag 等解析前检查建议放在 middleware 或 `Init`。

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

## 路径参数

```go
// 注册: app.GET("/users/{id}", &GetUserController{})

type GetUserController struct {
    glk.BaseController
}

func (c *GetUserController) Serve(ctx context.Context) error {
    id := c.PathValueInt("id", 0)
    return c.ServeJSON(map[string]int{"id": id})
}
```

## 中间件

```go
// 自定义中间件
func AuthMiddleware(next glk.Handler) glk.Handler {
    return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
        if r.Header.Get("Authorization") == "" {
            return glk.ErrUnauthorized("missing token")
        }
        return next(ctx, w, r)
    }
}

// 全局应用
app.Use(AuthMiddleware)

// 路由组应用
api := app.Group("/api")
api.Use(AuthMiddleware)
api.GET("/profile", &ProfileController{})
```

## 限流

```go
limiter := glk.NewRateLimiter(
    10, 10, // 每个 key 10 req/s
    glk.WithGlobalRateLimiter(1000, 1000), // 全局 1000 req/s
    glk.WithTTL(5 * time.Minute),
)
app.Use(limiter.RateLimiterAsMiddleware(glk.ByIP))
```

## SSE 流式响应

```go
type StreamController struct {
    glk.BaseController
}

func (c *StreamController) Serve(ctx context.Context) error {
    sse := c.ServeSSE()
    for i := 0; i < 5; i++ {
        sse.Send(glk.SSEvent{Data: fmt.Sprintf("message %d", i)})
        time.Sleep(time.Second)
    }
    return nil
}
```

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
    return ctx.ServeJSON(map[string]string{"status": "ok"})
}))
```

## 服务注册表

注册和获取 DB/Redis 之外的自定义服务：

```go
app := glk.NewApp(
    glk.WithService("cache", myCache),
)

// 在 HandlerFunc 中使用
app.GET("/cache", glk.HandlerFunc(func(ctx *glk.Context) error {
    cache := ctx.Service("cache").(MyCache)
    // ...
    return ctx.ServeJSON(map[string]string{"status": "ok"})
}))

// 在控制器中使用
func (c *MyController) Serve(ctx context.Context) error {
    gcx := glk.GetContext(ctx)
    cache := gcx.Service("cache").(MyCache)
    // ...
    return nil
}
```

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
```

测试或自定义绑定地址时，可以传入显式 `ServerConfig`：

```go
app.Start(glk.ServerConfig{Addr: "127.0.0.1:0"})
```

## Pprof

挂载受保护的 pprof 端点：

```go
app.MountPprof(glk.PprofOptions{
    LoopbackOnly: true, // 仅允许 127.0.0.1/::1 访问
})
// 访问地址: /debug/pprof/
```

## 配置文件

```toml
# app.toml
[HttpServer]
appName = "myapp"
runMode = "debug"
network = "tcp"
addr = ":8080"
enablePprof = false

[HttpServer.Timeout]
readTimeout = 1000
readHeaderTimeout = 200
writeTimeout = 15000
idleTimeout = 5000
shutdownTimeout = 5000

[HttpServer.Logger]
configFile = "logger.toml"

[HttpServer.DB]
configFile = "db.toml"

[HttpServer.Redis]
configFile = "redis.toml"
```

```go
app, err := glk.NewAppFromConfig("app.toml")
```

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
| `glk add controller <name>` | 在 `./controller/` 下生成控制器文件 |
| `glk add middleware <name>` | 在 `./middleware/` 下生成中间件文件 |

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
