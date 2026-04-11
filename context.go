package golitekit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hansir-hsj/GoLiteKit/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const globalContextKey ContextKey = iota

// reset clears all request-scoped fields so the instance can be reused from the pool.
func (gcx *Context) reset() {
	gcx.request = nil
	gcx.RawBody = nil
	gcx.responseWriter = nil
	gcx.logger = nil
	gcx.panicLogger = nil
	gcx.services = nil
	gcx.rawResponse = nil
	gcx.jsonResponse = nil
	gcx.rawHtml = ""
	gcx.statusCode = 0
	gcx.sseWriter = nil
	for k := range gcx.data {
		delete(gcx.data, k)
	}
}

// glkContext is a pooled context that embeds both the GoLiteKit Context and the
// logger's LoggerContext by value, avoiding two context.WithValue allocations
// per request.
type glkContext struct {
	parent    context.Context
	gcx       Context              // embedded by value — no separate allocation
	loggerCtx logger.LoggerContext // embedded by value — no separate allocation
	tracker   Tracker              // embedded by value — avoids context.WithValue for tracker
}

// glkCtxPool reuses *glkContext (and its embedded structs) across requests.
var glkCtxPool = sync.Pool{New: func() any { return &glkContext{} }}

func (c *glkContext) Deadline() (time.Time, bool) { return c.parent.Deadline() }
func (c *glkContext) Done() <-chan struct{}       { return c.parent.Done() }
func (c *glkContext) Err() error                  { return c.parent.Err() }

// Value answers the two framework-owned keys directly (O(1), no chain walk).
func (c *glkContext) Value(key any) any {
	switch key {
	case globalContextKey:
		return &c.gcx
	case logger.LoggerKey:
		return &c.loggerCtx
	case trackerKey:
		if c.tracker.started {
			return &c.tracker
		}
	}
	return c.parent.Value(key)
}

// release resets all embedded contexts and returns c to the pool.
func (c *glkContext) release() {
	c.parent = nil
	c.gcx.reset()
	c.loggerCtx.Reset()
	c.tracker.resetForPool()
	glkCtxPool.Put(c)
}

// newContext retrieves a *glkContext from the pool and attaches the request's
// parent context. The caller must call glkCtx.release() after the request.
func newContext(r *http.Request) *glkContext {
	gctx := glkCtxPool.Get().(*glkContext)
	gctx.parent = r.Context()
	gctx.gcx.reset()
	gctx.loggerCtx.Reset()
	return gctx
}

type ContextKey int

type ContextOption func(*Context)

// Context holds request-scoped data for a single HTTP request.
type Context struct {
	request        *http.Request
	RawBody        []byte
	responseWriter http.ResponseWriter

	logger      logger.Logger
	panicLogger *logger.PanicLogger
	services    *Services

	rawResponse  any
	jsonResponse any
	rawHtml      string
	statusCode   int

	sseWriter *SSEWriter

	data     map[string]any
	dataLock sync.RWMutex
}

type SSEvent struct {
	Event string `json:"event,omitempty"`
	Data  any    `json:"data"`
	ID    string `json:"id,omitempty"`
	Retry int    `json:"retry,omitempty"`
}

type SSEWriter struct {
	w http.ResponseWriter
}

func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return &SSEWriter{w: w}
}

func GetContext(ctx context.Context) *Context {
	gcx := ctx.Value(globalContextKey)
	if c, ok := gcx.(*Context); ok {
		return c
	}
	return nil
}

func WithContext(ctx context.Context) context.Context {
	gcx := GetContext(ctx)
	if gcx == nil {
		gcx = &Context{}
		return context.WithValue(ctx, globalContextKey, gcx)
	}
	return ctx
}

// withServices injects services and their loggers into context (internal use).
func withServices(s *Services) ContextOption {
	return func(gcx *Context) {
		gcx.services = s
		if s != nil {
			gcx.logger = s.logger
			gcx.panicLogger = s.panicLogger
		}
	}
}

func SetContextData(ctx context.Context, key string, data any) {
	gcx := GetContext(ctx)
	if gcx != nil {
		gcx.dataLock.Lock()
		defer gcx.dataLock.Unlock()
		if gcx.data == nil {
			gcx.data = make(map[string]any)
		}
		gcx.data[key] = data
	}
}

func GetContextData(ctx context.Context, key string) (any, bool) {
	gcx := GetContext(ctx)
	if gcx != nil {
		gcx.dataLock.RLock()
		defer gcx.dataLock.RUnlock()
		if gcx.data != nil {
			if v, ok := gcx.data[key]; ok {
				return v, true
			}
		}
	}
	return nil, false
}

func (gcx *Context) SetContextOptions(opts ...ContextOption) *Context {
	for _, opt := range opts {
		opt(gcx)
	}
	return gcx
}

func WithRequest(r *http.Request) ContextOption {
	return func(gcx *Context) {
		gcx.request = r
	}
}

func WithResponseWriter(w http.ResponseWriter) ContextOption {
	return func(gcx *Context) {
		gcx.responseWriter = w
	}
}

func withLogger(logger logger.Logger) ContextOption {
	return func(gcx *Context) {
		gcx.logger = logger
	}
}

func withPanicLogger(pl *logger.PanicLogger) ContextOption {
	return func(gcx *Context) {
		gcx.panicLogger = pl
	}
}

func (ctx *Context) Request() *http.Request {
	return ctx.request
}

func (ctx *Context) ResponseWriter() http.ResponseWriter {
	return ctx.responseWriter
}

func (ctx *Context) Logger() logger.Logger {
	return ctx.logger
}

func (ctx *Context) PanicLogger() *logger.PanicLogger {
	return ctx.panicLogger
}

func (ctx *Context) DB() *gorm.DB {
	if ctx.services == nil {
		return nil
	}
	return ctx.services.DB()
}

func (ctx *Context) Redis() *redis.Client {
	if ctx.services == nil {
		return nil
	}
	return ctx.services.Redis()
}

func (ctx *Context) ServeRawData(data any) {
	ctx.rawResponse = data
}

func (ctx *Context) ServeJSON(data any) {
	ctx.jsonResponse = data
}

func (ctx *Context) ServeHTML(html string) {
	ctx.rawHtml = html
}

// ContextAsMiddleware writes the buffered response stored in Context (via
// ServeJSON / ServeRawData / ServeHTML) after the inner handler returns.
// Errors returned by the inner handler are propagated without writing a response.
func ContextAsMiddleware() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			err := next(ctx, w, r)
			if err != nil {
				return err
			}

			gcx := GetContext(ctx)
			if gcx == nil {
				return nil
			}

			statusCode := http.StatusOK
			if gcx.statusCode != 0 {
				statusCode = gcx.statusCode
			}

			if gcx.jsonResponse != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(statusCode)
				if bytes, ok := gcx.jsonResponse.([]byte); ok {
					if _, err := w.Write(bytes); err != nil {
						return ErrInternal("failed to write response", err)
					}
				} else {
					jsonData, err := json.Marshal(gcx.jsonResponse)
					if err != nil {
						return ErrInternal("Failed to marshal JSON response", err)
					}
					if _, err := w.Write(jsonData); err != nil {
						return ErrInternal("failed to write response", err)
					}
				}
			} else if gcx.rawResponse != nil {
				switch body := gcx.rawResponse.(type) {
				case []byte:
					w.Header().Set("Content-Type", "application/octet-stream")
					w.WriteHeader(statusCode)
					if _, err := w.Write(body); err != nil {
						return ErrInternal("failed to write response", err)
					}
				case string:
					w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
					w.WriteHeader(statusCode)
					if _, err := w.Write([]byte(body)); err != nil {
						return ErrInternal("failed to write response", err)
					}
				default:
					return ErrInternal("Unsupported response type", nil)
				}
			} else if gcx.rawHtml != "" {
				w.Header().Set("Content-Type", "text/html; charset=UTF-8")
				w.WriteHeader(statusCode)
				if _, err := w.Write([]byte(gcx.rawHtml)); err != nil {
					return ErrInternal("failed to write response", err)
				}
			}

			return nil
		}
	}
}

func (sse *SSEWriter) Send(event SSEvent) error {
	if event.ID != "" {
		if _, err := fmt.Fprintf(sse.w, "id: %s\n", sse.sanitize(event.ID)); err != nil {
			return err
		}
	}

	if event.Event != "" {
		if _, err := fmt.Fprintf(sse.w, "event: %s\n", sse.sanitize(event.Event)); err != nil {
			return err
		}
	}

	if event.Retry > 0 {
		if _, err := fmt.Fprintf(sse.w, "retry: %d\n", event.Retry); err != nil {
			return err
		}
	}

	data, err := sse.serializeData(event.Data)
	if err != nil {
		return err
	}

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		if _, err := fmt.Fprintf(sse.w, "data: %s\n", sse.sanitize(line)); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(sse.w, "\n"); err != nil {
		return err
	}

	if f, ok := sse.w.(http.Flusher); ok {
		f.Flush()
	}

	return nil
}

func (sse *SSEWriter) sanitize(data string) string {
	data = strings.ReplaceAll(data, "\r", "")
	data = strings.ReplaceAll(data, "\n", "")
	return data
}

func (sse *SSEWriter) serializeData(data any) (string, error) {
	switch v := data.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		jsonData, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(jsonData), nil
	}
}

func (ctx *Context) SSEWriter() *SSEWriter {
	if ctx.sseWriter == nil {
		ctx.sseWriter = NewSSEWriter(ctx.responseWriter)
	}
	return ctx.sseWriter
}

// Query returns query parameter value.
func (ctx *Context) Query(key string) string {
	if ctx.request == nil {
		return ""
	}
	return ctx.request.URL.Query().Get(key)
}

// QueryDefault returns query parameter or default value if empty.
func (ctx *Context) QueryDefault(key, defaultValue string) string {
	if v := ctx.Query(key); v != "" {
		return v
	}
	return defaultValue
}

// Param returns path parameter value (Go 1.22+).
func (ctx *Context) Param(key string) string {
	if ctx.request == nil {
		return ""
	}
	return ctx.request.PathValue(key)
}

// JSON writes JSON response with status code.
func (ctx *Context) JSON(code int, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	ctx.statusCode = code
	ctx.jsonResponse = jsonData
	return nil
}

// String writes plain text response with status code.
func (ctx *Context) String(code int, s string) error {
	ctx.statusCode = code
	ctx.rawResponse = s
	return nil
}

// HTML writes HTML response with status code.
func (ctx *Context) HTML(code int, html string) error {
	ctx.statusCode = code
	ctx.rawHtml = html
	return nil
}

// Services returns the service container.
func (ctx *Context) Services() *Services {
	return ctx.services
}
