# GoLiteKit

[English](readme.md) | [中文](readme.zh.md)

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org)
[![Version](https://img.shields.io/badge/version-v0.2.0-blue?style=for-the-badge)](CHANGELOG.md)
[![License](https://img.shields.io/badge/license-MIT-green?style=for-the-badge)](LICENSE)

A lightweight Go web framework built on `net/http`, designed for simplicity and clarity.

## Performance

GoLiteKit benchmarks within **4% of Gin** on JSON-binding workloads.
Pure routing overhead is ~20% higher due to pooled context, response buffering, and structured logging —
features that plain routers omit. Full results: [`benchmarks/`](benchmarks/)

## Features

- **Generic controllers** — type-safe request binding via `BaseController[T]`
- **Middleware chain** — composable, ordered middleware with per-group support
- **Request binding** — JSON, form-urlencoded, multipart out of the box
- **SSE support** — streaming responses with proper flushing
- **Built-in middleware** — logger, error handler, timeout, rate limiter, gzip, tracker
- **Structured logging** — based on `slog`, with per-request field accumulation and log rotation
- **Service injection** — DB (gorm), Redis, Logger wired in via functional options
- **glk CLI** — scaffold new projects with `glk new`

## Installation

```bash
go get github.com/hansir-hsj/GoLiteKit
```

Requires Go 1.22+.

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

Define a request struct and use `BaseController[T]` — the framework binds JSON,
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
    c.ServeData(ctx, users)
    return nil
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
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Authorization") == "" {
            glk.SetError(r.Context(), glk.ErrUnauthorized("missing token"))
            return
        }
        next.ServeHTTP(w, r)
    })
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
    glk.WithGlobalLimit(1000, 1000), // 1000 req/s globally
    glk.WithPerKeyLimit(10, 10),     // 10 req/s per IP
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

```go
db, _ := glk.NewDB("db.toml")
rdb, _ := glk.NewRedis("redis.toml")

app := glk.NewApp(
    glk.WithDB(db),
    glk.WithRedis(rdb),
)

// Access in controller
func (c *MyController) Serve(ctx context.Context) error {
    db := c.DB()    // *gorm.DB
    rdb := c.Redis() // *redis.Client
    // ...
    return nil
}
```

## Configuration

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
timeout     = 3000   # ms
sse_timeout = 60000  # ms
```

```go
app, err := glk.NewAppFromConfig("app.toml")
```

## Examples

| Directory | Description |
|-----------|-------------|
| [examples/hello](examples/hello) | Minimal GET endpoint |
| [examples/rest-api](examples/rest-api) | REST controller with JSON binding and path params |
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
