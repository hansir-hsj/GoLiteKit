# GoLiteKit

[English](readme.md) | [中文](readme.zh.md)

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.23-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org)
[![Version](https://img.shields.io/badge/version-v1.2.0-blue?style=for-the-badge)](CHANGELOG.md)
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
- **Built-in middleware** — logger, error handler, timeout, rate limiter, gzip, log IDs
- **Observability** — optional OpenTelemetry adapter for request spans, service spans, and metrics
- **Structured logging** — based on `slog`, with body logging (truncation & redaction), log rotation
- **Custom services** — DB, Redis, Logger via functional options + startup-registered custom dependencies
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
    return c.JSON(http.StatusOK, map[string]any{"created": req.Name})
}
```

### Controller Lifecycle

Controller requests run through this order:

```text
Init → ParseRequest → Validate → Serve → Finalize
```

`ParseRequest` binds JSON/form/multipart data before `Validate`, so validation code can safely inspect `c.GetRequest()` or `c.Request`. Use middleware or `Init` for pre-parse checks such as authentication or feature flags.

Each request gets a fresh controller instance copied from the registered controller prototype. Store immutable route configuration or dependency references on the prototype, and keep request-specific state on the per-request instance.

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

## Observability

GoLiteKit keeps observability abstractions in the core package and provides an optional OpenTelemetry adapter:

```go
import glkotel "github.com/hansir-hsj/GoLiteKit/otel"

app := glk.NewApp(
    glkotel.WithObservability(
        glkotel.WithTracerProvider(tracerProvider),
        glkotel.WithMeterProvider(meterProvider),
    ),
)
```

Create service spans explicitly with context:

```go
ctx, span := glk.StartSpan(ctx, "cache.lookup", glk.StringAttr("component", "cache"))
defer span.End()
```

Use stable span names and bounded metric labels. Do not use raw SQL, raw URLs, user IDs, trace IDs, log IDs, or path parameter values as metric labels.

## Path Parameters

```go
// Register: app.GET("/users/{id}", &GetUserController{})

type GetUserController struct {
    glk.BaseController
}

func (c *GetUserController) Serve(ctx context.Context) error {
    id := c.PathValueInt("id", 0)
    return c.JSON(http.StatusOK, map[string]int{"id": id})
}
```

## Middleware

```go
// Custom middleware
func AuthMiddleware(next glk.Handler) glk.Handler {
    return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
        if r.Header.Get("Authorization") == "" {
            return glk.ErrUnauthorized("missing token", nil)
        }
        return next(ctx, w, r)
    }
}

// Apply globally
app.Use(AuthMiddleware)
app.GET("/profile", &ProfileController{})

// Apply to a group
api := app.Group("/api")
api.Use(AuthMiddleware)
api.GET("/profile", &ProfileController{})
```

Register middleware before registering routes, static files, pprof endpoints, or nested groups. GoLiteKit prebuilds the middleware chain at registration time and panics if `Use` is called after routes were added. Route and middleware registration is intended for application startup and should be done from one goroutine.

## Rate Limiting

```go
limiter := glk.NewRateLimiter(
    10, 10, // 10 req/s per key
    glk.WithGlobalRateLimiter(1000, 1000), // 1000 req/s globally
    glk.WithTTL(5 * time.Minute),
    glk.WithMaxKeys(10000),
)
app.Use(limiter.RateLimiterAsMiddleware(glk.ByIP))
```

Per-key limiters use a safe default TTL and key capacity limit. Use `WithoutTTL()` only when keys are bounded by design.

## SSE Streaming

```go
type StreamController struct {
    glk.BaseController
}

func (c *StreamController) Serve(ctx context.Context) error {
    sse := c.SSE()
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
    return ctx.JSON(http.StatusOK, map[string]string{"status": "ok"})
}))
```

## Custom Services

Register custom services during app construction, then read them from requests:

```go
app := glk.NewApp(
    glk.WithService("cache", myCache),
)

// In HandlerFunc
app.GET("/cache", glk.HandlerFunc(func(ctx *glk.Context) error {
    cache := ctx.Service("cache").(MyCache)
    // ...
    return ctx.JSON(http.StatusOK, map[string]string{"status": "ok"})
}))

// In controller
func (c *MyController) Serve(ctx context.Context) error {
    cache := c.Service("cache").(MyCache)
    // ...
    return nil
}
```

Custom services are read-only during request handling. Use request context data for request-scoped values.

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

For low-level `Server` usage, `srv.Done()` reports the background `Serve` result after `Start`; normal shutdown sends `nil`, while unexpected listener/server failures send the error.

Calling `Start` again while the same `Server` is already running returns an already-started error.

For tests or custom bind addresses, pass an explicit `ServerConfig`:

```go
app.Start(glk.ServerConfig{Addr: "127.0.0.1:0"})
```

Zero-valued timeout and header-limit fields inherit safe defaults from `DefaultServerConfig`, so passing only `Addr` keeps read/write/header/idle timeouts enabled.

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
