# Changelog

All notable changes to GoLiteKit are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

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
- `CloneController` deep-copy with sync-primitive safety

**glk CLI**
- `glk new <appName>` — scaffold a new GoLiteKit project
- `glk new --module` — specify a custom Go module path
- `glk add controller <name>` — generate a controller file
- `glk add middleware <name>` — generate a middleware file
- `glk version` — print the current version

**Documentation**
- README (English and Chinese) with quick start and API guide
- Examples: hello, rest-api, middleware, sse
