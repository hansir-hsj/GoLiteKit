# Changelog

All notable changes to GoLiteKit are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

### Added
- Optional OpenTelemetry observability adapter in `otel/`, including HTTP request spans, service spans, HTTP metrics, and service span metrics.
- Core `Observer`, `Span`, and `StartSpan` abstractions without importing OpenTelemetry from the root package.

### Changed
- Request lifecycle now supports observability wrapping ErrorHandler so spans and metrics see final handled response status.
- Router now creates a fresh controller instance per request from the registered prototype instead of pooling controller instances, preventing request-scoped fields from leaking across requests.
- Router now rejects non-pointer controller values with a clear panic message; register controllers as pointers to structs.
- RateLimiter now applies a default per-key TTL and maximum key capacity to prevent unbounded per-key limiter growth.
- Rate limiter middleware now returns `429` when the per-key capacity limit is exhausted.
- `NewServer` now fills zero-valued timeout and header-limit fields from `DefaultServerConfig`, and default config includes `ReadHeaderTimeout`.
- Request context wrappers are no longer pooled, so context values captured by request goroutines remain readable after the handler returns.
- Response logging now captures only the configured body-log prefix, and ErrorHandler response buffering now has a default size limit before switching to pass-through writes.
- `NewAppFromConfig` now snapshots logger and timeout middleware options at construction time instead of reading mutable global env on each request.
- `env.Init` and env getters are now safe for concurrent config reloads and reads.
- Controller request state now tracks middleware-updated request contexts before lifecycle hooks run.
- Router and RouterGroup now reject middleware registration after routes, static files, pprof endpoints, or nested groups are registered, making prebuilt middleware-chain ordering explicit.
- Static file and pprof handlers now run through the current middleware chain captured at registration time.
- ResponseWriter wrappers now expose `Unwrap()` so `http.ResponseController` can reach underlying writer capabilities.
- `Server.Start` now exposes the background serve result through `Server.Done()`.
- `App.ListenAndServe` now returns if the background server exits unexpectedly instead of waiting only for context cancellation.
- `Server.Start` now rejects repeated starts while the server is already running.
- Custom services are now startup-registered through `WithService` and read-only during request handling.
- HandlerFunc routes now use a direct lightweight route path instead of being adapted into controller lifecycle instances.
- Logger and timeout middleware no longer read global env during request handling; pass explicit options or use `NewAppFromConfig` for config snapshots.

### Fixed
- Gzip compression no longer writes an empty gzip stream for `204 No Content` or `304 Not Modified` responses.
- `App.Start` now clears the current server after background `Serve` exits, allowing a later restart.
- Method-not-allowed catch-all handlers now run through the current middleware chain.
- Internal error strings added to request logs are now redacted for common secret-bearing key/value patterns.
- Deferred response writing now skips commit after a successful connection hijack.

### Removed
- Removed the old `Tracker` public API. Use `StartSpan(ctx, name, attrs...)` instead.
- Removed `ControllerWrapper`, `WrapFunc`, and `WrapHandler`; register `HandlerFunc` / `func(*Context) error` directly or use a standard controller.
- Removed request-time custom service mutation via `Context.SetService`.
- Removed `App.Run(addr)`, `App.RunWithConfig`, `App.RunFromEnv`, and `Server.Run`; use `ListenAndServe(ctx, config)` or `Start`/`Shutdown` for explicit lifecycle control.

---

## [v1.2.0] - 2026-06-05

### Changed
- Controller lifecycle now runs request parsing before validation: `Init → ParseRequest → Validate → Serve → Finalize`.
- `BaseControllerOf.Init` no longer reads or parses request bodies; parsing is owned by `ParseRequest`.
- Custom `RequestParser` implementations now own request parsing from the request/context.

---

## [v1.1.0] - 2026-05-30

### Added
- **HandlerFunc routes**: lightweight `func(*Context) error` handlers as an alternative to controllers.
- **Custom services**: `WithService(key, value)` for startup-registered custom dependencies beyond DB/Redis.
- **Context-aware server lifecycle**: `Start()` (non-blocking), `ListenAndServe(ctx)` (blocks until context cancelled, then graceful shutdown).
- **Protected pprof mounting**: `MountPprof()` with optional `LoopbackOnly` restriction.
- **Safe body logging**: request/response body logging with configurable `MaxBodyBytes` truncation and sensitive field redaction.
- **CI**: dependency and vulnerability checks via GitHub Actions.

### Changed
- **Middleware options injection**: middleware constructors now accept explicit option structs instead of reading from global config.
- Internal error responses no longer expose implementation details to clients (5xx errors return generic message).

### Fixed
- `glk new` project creation hardened against edge cases (empty names, invalid paths).
- Critical bug fixes for timeout middleware, rate limiter token refill, logger rotation, and DB/Redis connection handling.

---

## [v0.3.0] - 2026-04-11

### Changed
- **Error handling unified**: Replace context-based `SetError`/`GetError` with explicit error returns.
  All middleware and handlers now propagate errors through return values.
- **Controller interface simplified**: Reduce from 5 methods to just `Serve()`.
  Optional hooks (`Init`, `Validate`, `Finalize`) are now separate interfaces.
- Rename `SanityCheck` to `Validate` for clarity.
- `Handler` type: `func(ctx, w, r) error` — supports error propagation.
- `Middleware` type: `func(next Handler) Handler` — composable with error handling.

### Added
- `StdMiddleware` adapter for third-party `http.Handler` middleware (e.g., CORS).
- `Unwrap()` method on `AppError` for Go 1.13+ `errors.Is`/`errors.As` support.
- Optional `internal error` parameter for `ErrUnauthorized`, `ErrForbidden`, `ErrNotFound`, etc.
- Optional lifecycle interfaces: `Initializer`, `Validator`, `RequestParser`, `Finalizer`.

### Removed
- `SetError`, `GetError`, `HasError`, `ClearError` — use error returns instead.
- `HandlerMiddleware` type — replaced by `Middleware`.

### Fixed
- `w.Write()` return values now properly handled in `ContextAsMiddleware`.
- `ServeData()` return values no longer discarded in examples.

---

## [v0.2.0] - 2026-04-10

### Changed
- Controller dispatch: replaced `CloneController` deep-copy with `reflect.Type` + `reflect.New`.
  Each request gets a fresh zero-value instance; ~250 lines of copy machinery removed.
- 405 Method Not Allowed response now returns JSON (`{"status":405,"msg":"..."}`) instead of plain text.
- `BaseController[T]` renamed to `BaseControllerOf[T]`; `RestController[T]` renamed to `RestControllerOf[T]`.
  `BaseController` and `RestController` are now no-body aliases — embedding them requires no type parameter.

### Performance
- Pool `glkContext` via `sync.Pool` with `LoggerContext` and `Tracker` embedded by value —
  eliminates 4 `context.WithValue` allocations per request.
- Pre-build middleware chain at route registration — eliminates per-request closure allocations.
- Pool `responseCapture` in `LoggerAsMiddleware` — **-1 alloc/op**.
- Embed `Tracker` in `glkContext` (no heap allocation on `WithTracker`) — **-2 allocs/op**.

### Added
- `docs/` — performance optimization notes.

### Removed
- `CloneController` and all associated deep-copy helpers.
- Exported `WithServices` option (no callers; inconsistent with internal version).
- `RestGetController` alias (superseded by `RestController`).

---

## [v0.1.0] - 2026-04-09

### Added

**Core framework**
- HTTP router built on `net/http` with path parameters (`{id}`)
- Generic request binding: JSON, form-urlencoded, multipart
- `BaseController[T]` and `RestController[T]` with unified JSON response envelope
- SSE streaming via `SSEWriter` with per-event flush
- Middleware chain: error handler, logger, rate limiter, gzip compression
- Per-request `Context` with `Tracker` for structured logging and timing
- Log rotation: time-based rules, configurable file count
- DB (GORM/MySQL) and Redis integration via config files

**glk CLI**
- `glk new <appName>` — scaffold a new GoLiteKit project
- `glk new --module` — specify a custom Go module path
- `glk add controller <name>` — generate a controller file
- `glk add middleware <name>` — generate a middleware file
- `glk version` — print the current version

**Documentation**
- README (English and Chinese) with quick start and API guide
- Examples: hello, rest_api, middleware, sse
