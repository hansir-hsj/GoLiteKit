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

Each request gets a fresh controller instance made by a **shallow copy** of the registered controller prototype:

- Fields of the prototype are copied by value, so pointer, map and slice fields point to the same data for every request. Use them for shared, read-only configuration and for dependencies that are safe for concurrent use (e.g. `*sql.DB`, a repository). Never write to a map or slice that came from the prototype.
- A `sync.Mutex` or other value-typed sync field is copied too, so it only guards that one request's instance. To protect state shared between requests, store a pointer to it.
- `Request` (from `BaseControllerOf[T]`) is reset to its zero value on every copy. Set request defaults in `Init`, which runs before `ParseRequest`.

When a controller needs per-request mutable state or constructor logic, register a `ControllerFactory` (or any `func() glk.Controller`) instead; it is called once per request:

```go
app.GET("/report", glk.ControllerFactory(func() glk.Controller {
    return &ReportController{repo: repo, rows: make([]Row, 0, 64)}
}))
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

`status` is the business status: `0` means success and any other value means failure, so clients can always check `status == 0`. The HTTP status code carries the transport result. There are two ways to report an error:

- `return glk.ErrBadRequest("invalid phone", nil).WithStatus(10012)` replies with HTTP 400 and `{"status": 10012, ...}`. Without `WithStatus`, `status` is the HTTP code (e.g. `404`), which is also what panics (`500`) and unmatched routes (`404`/`405`) return.
- `c.ServeError(ctx, 10012, "invalid phone")` replies with HTTP 200 and `{"status": 10012, ...}`, for APIs whose clients only look at `status`. `0` is not a valid error code and is replaced with `-1`; `ServeErrorMsg` always uses `-1`.

Business codes are logged in the `biz_status` field.

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

Without `WithTracerProvider` / `WithMeterProvider` the global OpenTelemetry providers are used (no-op until the application sets them). Instruments use the scope `github.com/hansir-hsj/GoLiteKit/otel` and report durations in seconds:

| Metric | Unit | Notes |
|--------|------|-------|
| `http.server.request.duration` | s | semconv; labels `http.request.method` (unknown methods as `_OTHER`), `http.response.status_code`, `http.route`, `url.scheme`, `error.type` |
| `glk.http.server.requests` | 1 | request count |
| `glk.service.span.calls` / `glk.service.span.duration` | 1 / s | spans started with `glk.StartSpan` |
| `glk.http.server.duration_ms` / `glk.service.span.duration_ms` | ms | deprecated, still recorded; move dashboards to the seconds metrics |

Unmatched requests (404/405) have no `http.route`.

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

An `App` wraps every route in a default stack, outermost first:

```
UseOuter middlewares → [OpenTelemetry] → Logger → LogID → ErrorHandler → Timeout → ContextAs → Use middlewares → group middlewares → route
```

- `app.Use` middlewares run inside the error handler: errors they return become JSON error responses, and they see the response a route stores with `ctx.JSON` / `c.JSON` (it is written as soon as the route returns).
- A middleware can answer instead of calling `next` (e.g. `return gcx.JSON(http.StatusForbidden, ...)`), but cannot replace the route's response after `next` returns: it has already been written, and the later response is discarded with a logged error. Decide before calling `next`, or wrap the `ResponseWriter`.
- `app.UseOuter` middlewares wrap the whole default stack, so headers they set (e.g. CORS) also apply to error responses. Errors they return bypass the error handler and are written as plain-text HTTP errors.

### Compression

```go
app.Use(glk.CompressionMiddleware()) // optional level, default gzip.DefaultCompression
```

Responses are gzip-compressed when the client accepts it and the body is at least `DefaultCompressionMinSize` (1 KB). HEAD requests, 1xx/204/304, partial content (206), already encoded responses and already compressed types (images except SVG, audio, video, archives, fonts, PDF) are sent as is. A stream flushed before reaching 1 KB stays uncompressed. Strong ETags become weak on compressed responses.

### Request ID

The default stack gives every request a log ID. It is logged as `logid`, returned in the `logid` field of JSON envelopes and in the `X-Request-ID` response header (error and panic responses included). The ID is always generated by the service, so clients cannot choose or collide log IDs. A valid incoming `X-Request-ID` (1-64 characters from `[A-Za-z0-9._:-]`) is logged as `upstream_request_id`, so logs can still be correlated with a gateway or upstream service; anything else is ignored.

```go
// Behind a gateway that always sets X-Request-ID: use its ID as the log ID.
app := glk.NewApp(glk.WithLogIDOptions(glk.LogIDOptions{TrustIncoming: true}))
```

Only set `TrustIncoming` when every request passes through a proxy that sets or overwrites the header. `LogIDOptions` also has `Header` (custom header name) and `OmitResponseHeader`. Cross-service tracing via `traceparent` is handled by the OpenTelemetry middleware, which logs `trace_id` and `span_id`.

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

Per-key limiters use a safe default TTL and key capacity limit. When the capacity is reached, the least recently used key is evicted, so a flood of new keys cannot lock out other clients. Use `WithoutTTL()` only when keys are bounded by design.

Key functions:

- `glk.ByIP` — the connection's remote IP. Behind a reverse proxy this is the proxy's IP.
- `glk.ByClientIP("127.0.0.1", "10.0.0.0/8")` — the client IP behind the given trusted proxies (from `X-Forwarded-For` / `X-Real-IP`, only when the connection comes from a trusted proxy).
- `glk.ByPath` — the matched route pattern (e.g. `GET /users/{id}`), so every route has one shared limit.

## SSE Streaming

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
            return err // client went away
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

With `NewAppFromConfig`, requests sent with `Accept: text/event-stream` get `[HttpServer.SSE] timeout` (default 300 s) instead of the normal handler timeout; the server write deadline is extended only once the handler actually responds with `text/event-stream`.

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

// The clients stay yours: the app does not close them. To close them with
// the app (after the server has stopped):
app.OnClose(func(ctx context.Context) error {
    sqlDB, err := dbConn.DB()
    if err != nil {
        return err
    }
    return errors.Join(sqlDB.Close(), rdb.Close())
})

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
app.Close(ctx) // release resources once the app will not be started again
```

`ListenAndServe` closes the app after the server stops; `Shutdown` does not, so the app can be restarted. `Close` runs the functions registered with `OnClose` in reverse registration order and closes the loggers created by `NewAppFromConfig`. Clients and loggers you pass in (`WithDB`, `WithRedis`, `WithLogger`, ...) stay yours and are not closed; register them with `OnClose` to close them with the app:

```go
app.OnClose(func(ctx context.Context) error {
    return tracerProvider.Shutdown(ctx) // e.g. flush OpenTelemetry spans
})
```

For low-level `Server` usage, `srv.Done()` reports the background `Serve` result after `Start`; normal shutdown sends `nil`, while unexpected listener/server failures send the error.

Calling `Start` again while the same `Server` is already running returns an already-started error.

For tests or custom bind addresses, pass an explicit `ServerConfig`:

```go
app.Start(glk.ServerConfig{Addr: "127.0.0.1:0"})
```

Zero-valued timeout and header-limit fields inherit safe defaults from `DefaultServerConfig`, so passing only `Addr` keeps read/write/header/idle timeouts enabled.

## Pprof

Prefer serving pprof on its own loopback listener, so it is never reachable through the public port or a reverse proxy:

```go
// Stops when ctx is canceled; returns the bound address.
if _, err := glk.ServePprof(ctx, "127.0.0.1:6060"); err != nil {
    log.Fatal(err)
}
```

With `NewAppFromConfig`, set `enablePprof = true` and `pprofAddr = "127.0.0.1:6060"`; the listener starts with the app and stops on `Close`. Without `pprofAddr`, pprof is mounted on the app's port, loopback-only.

Mounting on the app router is also possible:

```go
app.MountPprof(glk.PprofOptions{
    LoopbackOnly: true, // only 127.0.0.1/::1, and no forwarding headers
})
// Available at /debug/pprof/
```

`LoopbackOnly` rejects requests carrying `X-Forwarded-For`, `X-Real-IP` or `Forwarded`, because behind a reverse proxy on the same host every request comes from 127.0.0.1. A proxy that adds none of these headers still gets through, which is why the separate listener is recommended.

## Configuration

```toml
# app.toml
[HttpServer]
appName = "myapp"
runMode = "debug"
network = "tcp"
addr = ":8080"
enablePprof = false
pprofAddr = "127.0.0.1:6060" # optional: serve pprof on its own listener

[HttpServer.Timeout]
readTimeout = 1000
readHeaderTimeout = 200
writeTimeout = 15000
idleTimeout = 5000
shutdownTimeout = 5000

[HttpServer.SSE]
timeout = 300000 # handler timeout for text/event-stream requests, ms

[HttpServer.Logger]
configFile = "logger.toml"

[HttpServer.DB]
configFile = "db.toml"

[HttpServer.Redis]
configFile = "redis.toml"
```

```go
app, err := glk.NewAppFromConfig("app.toml")
// Read config values from the app, e.g. app.Env().Addr(), app.Env().WriteTimeout().
```

The loaded config is kept on the app, so several apps with different configs can run in one process. The package-level `env` functions (`env.Addr()`, ...) read the most recently loaded config and remain for compatibility; prefer `app.Env()`. `env.Load(path)` reads a config without changing that default.

Paths in the config: `configFile`, `certFile` and `keyFile` are relative to the directory of the app config file, so the app finds them no matter where it is started from; `staticDir` is relative to the working directory. Absolute paths are used as is. `db.NewFromConfig()` and `redis.NewFromConfig()` without arguments use `[HttpServer.DB]` / `[HttpServer.Redis]` `configFile`, falling back to `db.toml` / `redis.toml` next to the app config. Generated projects accept `-config path/to/app.toml`.

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
| `glk new <appName> --force` | Overwrite the generated files in an existing directory without asking |
| `glk new <appName> --dry-run` | Print the files that would be created |
| `glk new <appName> --skip-tidy` | Do not run `go mod tidy` |
| `glk add controller <name>` | Generate a controller file under `./controller/` |
| `glk add middleware <name>` | Generate a middleware file under `./middleware/` |

`<appName>` must be a relative path below the current directory. `glk new` never deletes an existing directory: in a non-empty one it asks before overwriting the generated files and keeps everything else. If writing fails, the files and directories created by that run are removed. `glk add` never overwrites an existing file. Errors exit with status 1.

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
