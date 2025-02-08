package golite

import (
	"context"
	"encoding/json"
	"github/hsj/golite/logger"
	"log"
	"net/http"
	"strconv"
	"sync"
)

const (
	globalContextKey ContextKey = iota
)

var extensionToContentType = map[string]string{
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "application/javascript",
	".xml":  "text/xml; charset=utf-8",
	".jpg":  "image/jpeg",
}

type ContextKey int

type ContextOption func(*Context)

type Context struct {
	request        *http.Request
	responseWriter http.ResponseWriter
	routerParams   map[string]string
	logger         logger.Logger
	panicLogger    *logger.PanicLogger

	rawResponse  any
	jsonResponse any
	rawFile      []byte
	rawExt       string
	rawHtml      string

	data     map[string]any
	dataLock sync.Mutex
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

func WithRouterParams(params map[string]string) ContextOption {
	return func(gcx *Context) {
		gcx.routerParams = params
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

func (ctx *Context) RouterParams() map[string]string {
	return ctx.routerParams
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

func (ctx *Context) ServeFile(ext string, file []byte) {
	ctx.rawExt = ext
	ctx.rawFile = file
}

func ContextAsMiddleware() Middleware {
	return func(ctx context.Context, queue MiddlewareQueue) error {
		err := queue.Next(ctx)
		if err != nil {
			return err
		}

		gcx := GetContext(ctx)
		if gcx == nil {
			return nil
		}

		w := gcx.ResponseWriter()

		if gcx.jsonResponse != nil {
			w.Header().Set("Content-Type", "application/json")
			if bytes, ok := gcx.jsonResponse.([]byte); ok {
				w.Write(bytes)
			} else {
				jsonData, err := json.Marshal(gcx.jsonResponse)
				if err != nil {
					return err
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
				log.Printf("unsupported response data type： %T", gcx.rawResponse)
			}
		} else if gcx.rawHtml != "" {
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.Write([]byte(gcx.rawHtml))
		} else if gcx.rawFile != nil && gcx.rawExt != "" {
			if contentType := extensionToContentType[gcx.rawExt]; contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			w.Header().Set("Content-Length", strconv.FormatInt(int64(len(gcx.rawFile)), 10))
			w.Write(gcx.rawFile)
		}

		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		return nil
	}
}
