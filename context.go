package golitekit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

const (
	globalContextKey ContextKey = iota
	errorContextKey
	AppErrorKey = "__app_error__"
)

type ContextKey int

type ContextOption func(*Context)

type Context struct {
	request        *http.Request
	RawBody        []byte
	responseWriter http.ResponseWriter
	logger         logger.Logger
	panicLogger    *logger.PanicLogger

	rawResponse  any
	jsonResponse any
	rawHtml      string

	sseWriter *SSEWriter

	data     map[string]any
	dataLock sync.Mutex
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
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
		gcx = &Context{
			data: make(map[string]any),
		}
		return context.WithValue(ctx, globalContextKey, gcx)
	}
	return ctx
}

func SetContextData(ctx context.Context, key string, data any) {
	gcx := GetContext(ctx)
	if gcx != nil {
		gcx.dataLock.Lock()
		defer gcx.dataLock.Unlock()
		gcx.data[key] = data
	}
}

func GetContextData(ctx context.Context, key string) (any, bool) {
	gcx := GetContext(ctx)
	if gcx != nil {
		gcx.dataLock.Lock()
		defer gcx.dataLock.Unlock()
		if v, ok := gcx.data[key]; ok {
			return v, true
		}
	}
	return nil, false
}

func SetError(ctx context.Context, err *AppError) {
	gcx := GetContext(ctx)
	if gcx != nil {
		gcx.dataLock.Lock()
		defer gcx.dataLock.Unlock()
		gcx.data[AppErrorKey] = err
	}
}

func GetError(ctx context.Context) *AppError {
	gcx := GetContext(ctx)
	if gcx != nil {
		gcx.dataLock.Lock()
		defer gcx.dataLock.Unlock()
		if v, ok := gcx.data[AppErrorKey]; ok {
			return v.(*AppError)
		}
	}
	return nil
}

func HasError(ctx context.Context) bool {
	return GetError(ctx) != nil
}

func ClearError(ctx context.Context) {
	gcx := GetContext(ctx)
	if gcx != nil {
		gcx.dataLock.Lock()
		defer gcx.dataLock.Unlock()
		delete(gcx.data, AppErrorKey)
	}
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

func WithLogger(logger logger.Logger) ContextOption {
	return func(gcx *Context) {
		gcx.logger = logger
	}
}

func WithPanicLogger(pl *logger.PanicLogger) ContextOption {
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

func (ctx *Context) ServeRawData(data any) {
	ctx.rawResponse = data
}

func (ctx *Context) ServeJSON(data any) {
	ctx.jsonResponse = data
}

func (ctx *Context) ServeHTML(html string) {
	ctx.rawHtml = html
}

func ContextAsMiddleware() HandlerMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// execute handler chain
			next.ServeHTTP(w, r)

			ctx := r.Context()
			gcx := GetContext(ctx)
			if gcx == nil {
				return
			}

			// if there are errors, do not write response, handle by ErrorHandlerMiddleware
			if HasError(ctx) {
				return
			}

			if gcx.jsonResponse != nil {
				w.Header().Set("Content-Type", "application/json")
				if bytes, ok := gcx.jsonResponse.([]byte); ok {
					w.Write(bytes)
				} else {
					jsonData, err := json.Marshal(gcx.jsonResponse)
					if err != nil {
						SetError(ctx, ErrInternal("Failed to marshal JSON response", err))
					}
					w.Write(jsonData)
				}
			} else if gcx.rawResponse != nil {
				switch body := gcx.rawResponse.(type) {
				case []byte:
					w.Header().Set("Content-Type", "application/octet-stream")
					w.Write(body)
				case string:
					w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
					w.Write([]byte(body))
				default:
					SetError(ctx, ErrInternal("Unsupported response type", nil))
				}
			} else if gcx.rawHtml != "" {
				w.Header().Set("Content-Type", "text/html; charset=UTF-8")
				w.Write([]byte(gcx.rawHtml))
			}
		})
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

func (ctx *Context) ServeSSE() *SSEWriter {
	return ctx.SSEWriter()
}
