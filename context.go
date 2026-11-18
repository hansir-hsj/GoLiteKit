package golitekit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hansir-hsj/GoLiteKit/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const globalContextKey ContextKey = iota

// glkContext holds the Context, LoggerContext and Observer by value, saving
// a context.WithValue layer for each.
type glkContext struct {
	parent    context.Context
	gcx       Context              // embedded by value — no separate allocation
	loggerCtx logger.LoggerContext // embedded by value — no separate allocation
	observer  Observer             // nil: look it up in parent
}

func (c *glkContext) Deadline() (time.Time, bool) { return c.parent.Deadline() }
func (c *glkContext) Done() <-chan struct{}       { return c.parent.Done() }
func (c *glkContext) Err() error                  { return c.parent.Err() }

// Value answers the framework-owned keys directly (O(1), no chain walk).
func (c *glkContext) Value(key any) any {
	switch key {
	case globalContextKey:
		return &c.gcx
	case logger.LoggerKey:
		return &c.loggerCtx
	case observerContextKey{}:
		if c.observer != nil {
			return c.observer
		}
	}
	return c.parent.Value(key)
}

// newContext creates a request-owned context wrapper and attaches the request's
// parent context.
func newContext(r *http.Request) *glkContext {
	return &glkContext{parent: r.Context()}
}

type ContextKey int

type ContextOption func(*Context)

// Context holds request-scoped data for a single HTTP request.
type Context struct {
	request        *http.Request
	rawBody        []byte
	responseWriter http.ResponseWriter

	logger      logger.Logger
	panicLogger *logger.PanicLogger
	services    *Services

	// response is the body set by JSON/String/Bytes/HTML; the last call wins.
	response *bufferedResponse

	sseWriter *SSEWriter

	logID string

	// handledErr is the error ErrorHandlerMiddleware turned into a response
	// (or recovered from a panic), for outer middlewares such as logging.
	handledErr *AppError

	// query caches the parsed URL query (see Query); queryRaw is the raw
	// query it was parsed from. Guarded by dataLock.
	query    url.Values
	queryRaw string

	data     map[string]any
	dataLock sync.RWMutex
}

// panicError wraps a recovered panic value.
type panicError struct{ value any }

func (e panicError) Error() string { return fmt.Sprintf("panic: %v", e.value) }

func (gcx *Context) setHandledError(err *AppError) {
	gcx.dataLock.Lock()
	gcx.handledErr = err
	gcx.dataLock.Unlock()
}

// HandledError returns the error ErrorHandlerMiddleware turned into a
// response, or nil. A recovered panic is a 500 *AppError.
func HandledError(ctx context.Context) error {
	gcx := GetContext(ctx)
	if gcx == nil {
		return nil
	}
	gcx.dataLock.RLock()
	defer gcx.dataLock.RUnlock()
	if gcx.handledErr == nil {
		return nil
	}
	return gcx.handledErr
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

func withContext(ctx context.Context) context.Context {
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

func (gcx *Context) setContextOptions(opts ...ContextOption) *Context {
	for _, opt := range opts {
		opt(gcx)
	}
	return gcx
}

func withRequest(r *http.Request) ContextOption {
	return func(gcx *Context) {
		gcx.request = r
	}
}

func withResponseWriter(w http.ResponseWriter) ContextOption {
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

// RawBody returns a copy of the parsed request body.
func (ctx *Context) RawBody() []byte {
	if len(ctx.rawBody) == 0 {
		return nil
	}
	body := make([]byte, len(ctx.rawBody))
	copy(body, ctx.rawBody)
	return body
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

// Service retrieves a startup-registered custom service.
func (ctx *Context) Service(key string) any {
	if ctx.services == nil {
		return nil
	}
	return ctx.services.customService(key)
}

// bufferedResponse is a response body waiting to be written.
type bufferedResponse struct {
	statusCode  int
	contentType string
	body        []byte
}

// setResponse replaces any previously set response, so the last call to
// JSON/String/Bytes/HTML wins regardless of which method it was.
func (ctx *Context) setResponse(code int, contentType string, body []byte) {
	ctx.response = &bufferedResponse{statusCode: code, contentType: contentType, body: body}
}

// errResponseAlreadyWritten reports a stored response that was dropped
// because a body had already been written.
func errResponseAlreadyWritten() *AppError {
	return ErrInternal("response already written to ResponseWriter; response set by JSON/String/Bytes/HTML was discarded", nil)
}

// writeStoredResponse writes and clears the stored response. A status already
// passed to WriteHeader is kept.
func writeStoredResponse(gcx *Context, w http.ResponseWriter, tw *writeTracker) error {
	if gcx == nil || gcx.response == nil {
		return nil
	}
	resp := gcx.response
	gcx.response = nil
	if tw.bodyStarted.Load() {
		return errResponseAlreadyWritten()
	}

	statusCode := int(tw.status.Load())
	if statusCode == 0 {
		statusCode = resp.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		w.Header().Set("Content-Type", resp.contentType)
		w.WriteHeader(statusCode)
	} else if w.Header().Get("Content-Type") == "" {
		// Still effective when the status is deferred (ErrorHandlerMiddleware).
		w.Header().Set("Content-Type", resp.contentType)
	}
	if !bodyAllowedForStatus(statusCode) {
		return nil
	}
	if _, err := w.Write(resp.body); err != nil {
		return ErrInternal("failed to write response", err)
	}
	return nil
}

// bodyAllowedForStatus mirrors net/http: 1xx, 204 and 304 responses have no body.
func bodyAllowedForStatus(status int) bool {
	switch {
	case status >= 100 && status <= 199:
		return false
	case status == http.StatusNoContent, status == http.StatusNotModified:
		return false
	}
	return true
}

// ContextAsMiddleware writes a response stored by a middleware that answers
// without calling next (e.g. ctx.JSON(403, ...)). Routes write their own
// stored response when they return, so a middleware cannot replace it after
// next; such a response is dropped and an error returned.
func ContextAsMiddleware() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			tw := &writeTracker{ResponseWriter: w}
			if err := next(ctx, tw, r); err != nil {
				return err
			}
			return writeStoredResponse(GetContext(ctx), w, tw)
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

// Query returns query parameter value. The query string is parsed once per
// request and reparsed only if the URL's raw query changes.
func (ctx *Context) Query(key string) string {
	values := ctx.queryValues()
	if values == nil {
		return ""
	}
	return values.Get(key)
}

// queryValues returns the parsed query of the current request, cached.
func (ctx *Context) queryValues() url.Values {
	if ctx.request == nil || ctx.request.URL == nil {
		return nil
	}
	raw := ctx.request.URL.RawQuery
	ctx.dataLock.Lock()
	defer ctx.dataLock.Unlock()
	if ctx.query == nil || ctx.queryRaw != raw {
		ctx.query, _ = url.ParseQuery(raw) // like URL.Query, malformed pairs are dropped
		ctx.queryRaw = raw
	}
	return ctx.query
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

// JSON sets a JSON response, written when the route returns. A later
// JSON/String/Bytes/HTML call replaces it.
func (ctx *Context) JSON(code int, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	ctx.setResponse(code, "application/json", jsonData)
	return nil
}

// String sets a plain text response with status code; see JSON.
func (ctx *Context) String(code int, s string) error {
	ctx.setResponse(code, "text/plain; charset=UTF-8", []byte(s))
	return nil
}

// Bytes sets a binary response with status code; see JSON.
func (ctx *Context) Bytes(code int, data []byte) error {
	ctx.setResponse(code, "application/octet-stream", data)
	return nil
}

// HTML sets an HTML response with status code; see JSON.
func (ctx *Context) HTML(code int, html string) error {
	ctx.setResponse(code, "text/html; charset=UTF-8", []byte(html))
	return nil
}
