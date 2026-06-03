# GoLiteKit

[English](readme.md) | [中文](readme.zh.md)

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.23-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org)
[![Version](https://img.shields.io/badge/version-v1.1.0-blue?style=for-the-badge)](CHANGELOG.md)
[![License](https://img.shields.io/badge/license-MIT-green?style=for-the-badge)](LICENSE)

A lightweight Go web framework built on `net/http`, designed for simplicity and clarity.

## Performance

Performance benchmarking is planned. Until a reproducible benchmark suite is added, GoLiteKit should be evaluated with your own workload and deployment profile.

Future benchmark reports should include the benchmark code, command, machine profile, compared versions, and `benchstat` output.

## Features

- **Generic controllers** — type-safe request binding via `BaseControllerOf[T]`
- **HandlerFunc routes** — lightweight `func(*Context) error` handlers without controller boilerplate
- **Middleware chain** — composable, ordered middleware with per-group support
- **Request binding** — JSON, form-urlencoded, multipart out of the box
- **SSE support** — streaming responses with proper flushing
- **Built-in middleware** — logger, error handler, timeout, rate limiter, gzip, tracker
- **Structured logging** — based on `slog`, with body logging (truncation & redaction), log rotation
- **Service registry** — DB, Redis, Logger via functional options + generic `Set/Get` for custom services
- **Graceful lifecycle** — `Start`, `ListenAndServe` (context-aware), `Shutdown` with configurable timeout
- **Pprof mounting** — protected pprof endpoints with optional loopback-only restriction
- **glk CLI** — scaffold new projects with `glk new`

## Installation

```bash
go get github.com/hansir-hsj/GoLiteKit
```

Requires Go 1.23+.

## Quick Start

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

## Request Binding

Define a request struct and use `BaseControllerOf[T]` — the framework binds JSON,
form-urlencoded, and multipart automatically.

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
    // req.Name, req.Email, req.Age are populated
    return c.ServeJSON(map[string]any{"created": req.Name})
}
```

## REST Controller

`RestControllerOf[T]` wraps every response in a standard JSON envelope:

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

## Path Parameters

```go
// Register: app.GET("/users/{id}", &GetUserController{})

type GetUserController struct {
    glk.BaseController
}

func (c *GetUserController) Serve(ctx context.Context) error {
    id := c.PathValueInt("id", 0)
    return c.ServeJSON(map[string]int{"id": id})
}
```

## Middleware

```go
// Custom middleware
func AuthMiddleware(next glk.Handler) glk.Handler {
    return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
        if r.Header.Get("Authorization") == "" {
            return glk.ErrUnauthorized("missing token")
        }
        return next(ctx, w, r)
    }
}

// Apply globally
app.Use(AuthMiddleware)

// Apply to a group
api := app.Group("/api")
api.Use(AuthMiddleware)
api.GET("/profile", &ProfileController{})
```

## Rate Limiting

```go
limiter := glk.NewRateLimiter(
    10, 10, // 10 req/s per key
    glk.WithGlobalRateLimiter(1000, 1000), // 1000 req/s globally
    glk.WithTTL(5 * time.Minute),
)
app.Use(limiter.RateLimiterAsMiddleware(glk.ByIP))
```

## SSE Streaming

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

## With DB and Redis

Minimal setup excerpt:

```go
import (
    glk "github.com/hansir-hsj/GoLiteKit"
    glkdb "github.com/hansir-hsj/GoLiteKit/db"
    glkredis "github.com/hansir-hsj/GoLiteKit/redis"
)

dbConn, _ := glkdb.NewFromConfig("db.toml")
rdb, _ := glkredis.NewFromConfig("redis.toml")
app := glk.NewApp(glk.WithDB(dbConn), glk.WithRedis(rdb))

// Access in controller
func (c *MyController) Serve(ctx context.Context) error {
    dbConn := c.DB() // *gorm.DB
    rdb := c.Redis() // *redis.Client
    // ...
    return nil
}
```

## HandlerFunc Routes

For simple endpoints that don't need a full controller:

```go
app.GET("/ping", glk.HandlerFunc(func(ctx *glk.Context) error {
    return ctx.ServeJSON(map[string]string{"status": "ok"})
}))
```

## Service Registry

Register and retrieve custom services beyond DB/Redis:

```go
app := glk.NewApp(
    glk.WithService("cache", myCache),
)

// In HandlerFunc
app.GET("/cache", glk.HandlerFunc(func(ctx *glk.Context) error {
    cache := ctx.Service("cache").(MyCache)
    // ...
    return ctx.ServeJSON(map[string]string{"status": "ok"})
}))

// In controller
func (c *MyController) Serve(ctx context.Context) error {
    gcx := glk.GetContext(ctx)
    cache := gcx.Service("cache").(MyCache)
    // ...
    return nil
}
```

## Graceful Shutdown

Use `ListenAndServe` for context-aware lifecycle with automatic graceful shutdown:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

app := glk.NewApp()
app.GET("/hello", &HelloController{})
app.ListenAndServe(ctx) // blocks until signal, then graceful shutdown
```

Or use `Start` + `Shutdown` for manual control:

```go
app.Start()
// ... do other work ...
app.Shutdown(ctx)
```

For tests or custom bind addresses, pass an explicit `ServerConfig`:

```go
app.Start(glk.ServerConfig{Addr: "127.0.0.1:0"})
```

## Pprof

Mount protected pprof endpoints:

```go
app.MountPprof(glk.PprofOptions{
    LoopbackOnly: true, // only accessible from 127.0.0.1/::1
})
// Available at /debug/pprof/
```

## Configuration

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

## Examples

| Directory | Description |
|-----------|-------------|
| [examples/hello](examples/hello) | Minimal GET endpoint |
| [examples/rest_api](examples/rest_api) | REST controller with JSON binding and path params |
| [examples/middleware](examples/middleware) | Custom middleware and route groups |
| [examples/sse](examples/sse) | SSE streaming response |

## glk CLI

Install:

```bash
go install github.com/hansir-hsj/GoLiteKit/glk@latest
```

| Command | Description |
|---------|-------------|
| `glk version` | Print the version of glk |
| `glk new <appName>` | Scaffold a new GoLiteKit project |
| `glk new <appName> --module <modulePath>` | Scaffold with a custom Go module path |
| `glk add controller <name>` | Generate a controller file under `./controller/` |
| `glk add middleware <name>` | Generate a middleware file under `./middleware/` |

Examples:

```bash
# create a new project
glk new myapp

# create with a custom module path
glk new myapp --module github.com/myorg/myapp

# add a controller (snake_case is converted to CamelCase)
glk add controller user_profile   # → controller/user_profile_controller.go

# add a middleware
glk add middleware request_id     # → middleware/request_id_middleware.go
```

## License

[MIT](LICENSE)
