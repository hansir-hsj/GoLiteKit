# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Communication / 沟通偏好

默认中文优先，并提供简洁英文对应；生成文档时也优先采用中英双语且保持精炼。
Default to Chinese first with a concise English counterpart; generated docs should be bilingual and brief where appropriate.

## Build & Test Commands

```bash
# Run all tests
go test ./...

# Run race tests
go test -race ./...

# Vet
go vet ./...

# Format
go fmt ./...

# Run CLI command package tests
go test -v ./glk/cmd
```

Go 1.23+ is required. GitHub Actions CI exists under `.github/workflows/ci.yml` and runs formatting, vet, tests, race tests, example checks, and vulnerability checks.

## Architecture

GoLiteKit is a lightweight Go web framework built on Go 1.22's `http.ServeMux`. Requests flow through a middleware chain before reaching a Controller.

### Request Lifecycle

```
HTTP Request → ErrorHandler → Logger → Tracker → Timeout → Context → [group middlewares] → Controller
```

Controller lifecycle:

```text
Init → ParseRequest → Validate → Serve → Finalize
```

- **ErrorHandlerMiddleware**: Outermost. Buffers response via `deferredResponseWriter`; catches panics (logs to PanicLogger, returns 500) and `AppError`s (formatted JSON with logID).
- **ContextAsMiddleware**: Writes the final response after the controller chain completes (JSON/raw/HTML based on what the controller set).
- **Controller lifecycle**: `ParseRequest` runs before `Validate`, so validation can safely inspect `c.GetRequest()` or `c.Request`. Use middleware or `Init` for pre-parse checks such as authentication or feature flags.

### Core Types

- **`Server`** (`server.go`): Thin `http.Server` lifecycle wrapper with TLS, `Start`, `ListenAndServe`, blocking `Run`, graceful shutdown, and listener address access. `Router`/`App` own mux, middleware, static files, and pprof mounting.
- **`Controller` interface** (`controller.go`): Primary endpoint contract requiring `RequestSizeLimiter` plus `Serve(ctx context.Context) error`; embedding `BaseController`/`BaseControllerOf[T]` supplies the size limits and default lifecycle behavior.
- **Optional lifecycle interfaces** (`controller.go`): `Initializer`, `Validator`, `RequestParser`, `Finalizer`, and `Resettable` add per-controller hooks when implemented. When present, hooks run as `Init → ParseRequest → Validate → Serve → Finalize`.
- **`BaseControllerOf[T any]`** (`controller.go`): Generic base with automatic JSON/form/multipart binding. Use `BaseController` for no-body endpoints.
- **`RestControllerOf[T any]`** (`rest_controller.go`): REST JSON envelope controller. Use `RestController` for no-body REST endpoints.
- **`Context`** (`context.go`): Thread-safe request-scoped data carrier. Holds request/response/logger/SSE writer. Response writing is deferred until `ContextAsMiddleware` runs.
- **`RouterGroup`** (`router_group.go`): Routes grouped under a prefix with optional group-level middleware.
- **`MiddlewareQueue`** (`middleware.go`): Applied in reverse order (outermost middleware wraps innermost).

### Error Handling

`AppError` (`errors.go`) carries HTTP status, user-facing message, and internal error. Factory functions: `ErrBadRequest`, `ErrUnauthorized`, `ErrForbidden`, `ErrNotFound`, `ErrMethodNotAllowed`, `ErrConflict`, `ErrTooManyRequests`, `ErrTimeout`, `ErrInternal`, `ErrServiceUnavailable`.

### Sub-Packages

| Package | Purpose |
|---------|---------|
| `config` | Pluggable config parser (JSON/TOML/YAML). `Register(ext, decoder)` for custom formats. |
| `env` | Loads `app.toml` — server address, timeouts, TLS, rate limits, pprof, static dir, SSE timeout. |
| `logger` | `slog`-based logging. `ConsoleLogger` (stdout), `FileLogger` (rotation by time/size/lines, old file cleanup), `PanicLogger` (separate panic log). `ContextHandler` injects context-scoped `Field` chain into records. |
| `db` | MySQL via GORM, configured by `db.toml`. |
| `redis` | Redis via `go-redis/v9`, configured by `redis.toml`. |
| `glk` | CLI scaffolding tool (`go install github.com/hansir-hsj/GoLiteKit/glk@latest`). Uses embedded Go templates. |

### Key Conventions

- Controllers are pooled per route; request-scoped state is reset before reuse.
- `BaseControllerOf` type parameter `T` defines the request body struct for automatic parsing.
- `BaseControllerOf.ParseRequest` owns JSON/form/multipart parsing; `Init` only attaches request-scoped framework state.
- The `Tracker` (`tracker.go`) provides stack-based service call timing — `Start(name)`/`End()` pairs.
- `RateLimiter` (`rate_limiter.go`) uses token buckets with optional global + per-key limiters and TTL-based key expiration.
- `ControllerWrapper` (`controller_wrapper.go`) adapts `http.HandlerFunc`/`http.Handler` to the `Controller` interface via `WrapFunc()`/`WrapHandler()`.
