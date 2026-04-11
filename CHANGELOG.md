# Changelog

All notable changes to GoLiteKit are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

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
- `benchmarks/` — comparative suite vs Gin / Echo / Chi / Stdlib across 5 scenarios.
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
