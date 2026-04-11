# GoLiteKit

[English](readme.md) | [中文](readme.zh.md)

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org)
[![Version](https://img.shields.io/badge/version-v0.2.0-blue?style=for-the-badge)](CHANGELOG.md)
[![License](https://img.shields.io/badge/license-MIT-green?style=for-the-badge)](LICENSE)

轻量级 Go Web 框架，基于 `net/http`，专注简洁和清晰。

## 性能

JSON 绑定场景下，GoLiteKit 与 Gin 的性能差距在 **4% 以内**。
纯路由开销约高 25%，代价来自 pooled context、响应缓冲和结构化日志——
这些是普通路由库所不具备的能力。完整数据：[`benchmarks/`](benchmarks/)

## 特性

- **泛型控制器** — 通过 `BaseController[T]` 实现类型安全的请求绑定
- **中间件链** — 可组合、有序的中间件，支持路由组级别配置
- **请求绑定** — 开箱即用支持 JSON、form-urlencoded、multipart
- **SSE 支持** — 流式响应，自动 flush
- **内置中间件** — 日志、错误处理、超时、限流、gzip 压缩、请求追踪
- **结构化日志** — 基于 `slog`，支持按请求累积字段、日志轮转
- **依赖注入** — DB (gorm)、Redis、Logger 通过 functional options 注入
- **glk 脚手架** — 使用 `glk new` 快速创建项目

## 安装

```bash
go get github.com/hansir-hsj/GoLiteKit
```

需要 Go 1.22+。

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

定义请求结构体，使用 `BaseController[T]` —— 框架自动绑定 JSON、form-urlencoded 和 multipart。

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
    c.ServeData(ctx, users)
    return nil
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
    glk.WithGlobalLimit(1000, 1000), // 全局 1000 req/s
    glk.WithPerKeyLimit(10, 10),     // 每 IP 10 req/s
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

```go
db, _ := glk.NewDB("db.toml")
rdb, _ := glk.NewRedis("redis.toml")

app := glk.NewApp(
    glk.WithDB(db),
    glk.WithRedis(rdb),
)

// 在控制器中使用
func (c *MyController) Serve(ctx context.Context) error {
    db := c.DB()     // *gorm.DB
    rdb := c.Redis() // *redis.Client
    // ...
    return nil
}
```

## 配置文件

```toml
# app.toml
[server]
addr    = ":8080"
network = "tcp"

[logger]
dir      = "./logs"
filename = "app.log"
level    = "INFO"
rotate   = "1day"

[timeout]
timeout     = 3000   # 毫秒
sse_timeout = 60000  # 毫秒
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
