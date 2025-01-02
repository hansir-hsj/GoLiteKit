package golite

import (
	"context"
	"log"
	"net/http"
	"sync"
)

const (
	globalContextKey ContextKey = iota
)

type ContextKey int

type ContextOption func(*Context)

type Context struct {
	request        *http.Request
	responseWriter http.ResponseWriter
	routerParams   map[string]string

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

func (ctx *Context) Request() *http.Request {
	return ctx.request
}

func (ctx *Context) ResponseWriter() http.ResponseWriter {
	return ctx.responseWriter
}

func (ctx *Context) RouterParams() map[string]string {
	return ctx.routerParams
}

func (ctx *Context) ServeRawData(data any) {
	header := ctx.responseWriter.Header()
	switch body := data.(type) {
	case []byte:
		if header.Get("Content-Type") == "" {
			header.Set("Content-Type", "application/octet-stream")
		}
		ctx.responseWriter.Write(body)
	case string:
		if header.Get("Content-Type") == "" {
			header.Set("Content-Type", "text/plain; charset=UTF-8")
		}
		ctx.responseWriter.Write([]byte(body))
	default:
		log.Printf("unsported response data type： %T", data)
	}
}

func (ctx *Context) ServeJSON(data any) {
	header := ctx.responseWriter.Header()
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "application/json")
	}
	ctx.ServeRawData(data)
}
