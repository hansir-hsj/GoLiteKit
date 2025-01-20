package golite

import (
	"bytes"
	"context"
	"github/hsj/golite/logger"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type RequestSizeLimiter interface {
	MaxMemorySize() int64
}

type Controller interface {
	MaxMemorySize() int64
	Init(ctx context.Context) error
	Serve(ctx context.Context) error
	Finalize(ctx context.Context) error
}

type BaseController struct {
	request        *http.Request
	responseWriter http.ResponseWriter
	logger         logger.Logger

	rawBody []byte

	gcx *Context
}

func (c *BaseController) MaxMemorySize() int64 {
	return 10 << 20
}

func (c *BaseController) Init(ctx context.Context) error {
	c.gcx = GetContext(ctx)
	c.request = c.gcx.Request()
	c.responseWriter = c.gcx.ResponseWriter()
	c.logger = c.gcx.logger
	c.parseBody()

	return nil
}

func (c *BaseController) Serve(ctx context.Context) error {
	return nil
}

func (c *BaseController) Finalize(ctx context.Context) error {
	return nil
}

func (c *BaseController) parseBody() error {
	maxMemorySize := c.MaxMemorySize()
	if maxMemorySize <= 0 {
		maxMemorySize = 10 << 20 // 10M
	}

	var err error
	ct := c.request.Header.Get("Content-Type")

	switch ct {
	case "application/x-www-form-urlencoded":
		err = c.request.ParseForm()
	case "multipart/form-data":
		err = c.request.ParseMultipartForm(maxMemorySize)
	default:
		if c.request.Body != nil {
			originBody := c.request.Body
			// capable of reading data multiple times
			c.rawBody, err = io.ReadAll(originBody)
			if err != nil {
				return err
			}
			defer originBody.Close()
			c.request.Body = io.NopCloser(bytes.NewBuffer(c.rawBody))
		}
	}

	return err
}

func (c *BaseController) ServeRawData(data any) {
	c.gcx.ServeRawData(data)
}

func (c *BaseController) ServeJSON(data any) {
	c.gcx.ServeJSON(data)
}

func (c *BaseController) QueryInt(key string, def int) int {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		if ival, err := strconv.Atoi(vals[0]); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController) QueryInt64(key string, def int64) int64 {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		if ival, err := strconv.ParseInt(vals[0], 10, 64); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController) QueryFloat32(key string, def float32) float32 {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		if fval, err := strconv.ParseFloat(vals[0], 32); err == nil {
			return float32(fval)
		}
	}
	return def
}

func (c *BaseController) QueryFloat64(key string, def float64) float64 {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		if fval, err := strconv.ParseFloat(vals[0], 64); err == nil {
			return fval
		}
	}
	return def
}

func (c *BaseController) QueryString(key string, def string) string {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		return vals[0]
	}
	return def
}

func (c *BaseController) QueryBool(key string, def bool) bool {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		return vals[0] == "1" || strings.ToLower(vals[0]) == "true"
	}
	return def
}

func (c *BaseController) forms() map[string][]string {
	ct := c.request.Header.Get("Content-Type")
	switch ct {
	case "application/x-www-form-urlencoded":
		return c.request.Form
	case "multipart/form-data":
		return c.request.PostForm
	}
	return nil
}

func (c *BaseController) FormString(key string, def string) string {
	params := c.forms()
	if vals, ok := params[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	return def
}

func (c *BaseController) FormInt(key string, def int) int {
	params := c.forms()
	if vals, ok := params[key]; ok && len(vals) > 0 {
		if ival, err := strconv.Atoi(vals[0]); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController) FormInt64(key string, def int64) int64 {
	params := c.forms()
	if vals, ok := params[key]; ok && len(vals) > 0 {
		if ival, err := strconv.ParseInt(vals[0], 10, 64); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController) FormFloat32(key string, def float32) float32 {
	params := c.forms()
	if vals, ok := params[key]; ok && len(vals) > 0 {
		if fval, err := strconv.ParseFloat(vals[0], 32); err == nil {
			return float32(fval)
		}
	}
	return def
}

func (c *BaseController) FormFloat64(key string, def float64) float64 {
	params := c.forms()
	if vals, ok := params[key]; ok && len(vals) > 0 {
		if fval, err := strconv.ParseFloat(vals[0], 64); err == nil {
			return fval
		}
	}
	return def
}

func (c *BaseController) FormBool(key string, def bool) bool {
	params := c.forms()
	if vals, ok := params[key]; ok && len(vals) > 0 {
		return vals[0] == "1" || strings.ToLower(vals[0]) == "true"
	}
	return def
}

func (c *BaseController) RouterParamString(key string, def string) string {
	params := c.gcx.routerParams
	if val, ok := params[key]; ok && val != "" {
		return val
	}
	return def
}

func (c *BaseController) RouterParamInt(key string, def int) int {
	params := c.gcx.routerParams
	if val, ok := params[key]; ok && val != "" {
		if ival, err := strconv.Atoi(val); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController) RouterParamInt64(key string, def int64) int64 {
	params := c.gcx.routerParams
	if val, ok := params[key]; ok && val != "" {
		if ival, err := strconv.ParseInt(val, 10, 64); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController) RouterParamFloat32(key string, def float32) float32 {
	params := c.gcx.routerParams
	if val, ok := params[key]; ok && val != "" {
		if fval, err := strconv.ParseFloat(val, 32); err == nil {
			return float32(fval)
		}
	}
	return def
}

func (c *BaseController) RouterParamFloat64(key string, def float64) float64 {
	params := c.gcx.routerParams
	if val, ok := params[key]; ok && val != "" {
		if fval, err := strconv.ParseFloat(val, 64); err == nil {
			return fval
		}
	}
	return def
}

func (c *BaseController) RouterParamBool(key string, def bool) bool {
	params := c.gcx.routerParams
	if val, ok := params[key]; ok && val != "" {
		return val == "1" || strings.ToLower(val) == "true"
	}
	return def
}

func (c *BaseController) AddDebug(ctx context.Context, key string, value any) {
	logger.AddDebug(ctx, key, value)
}

func (c *BaseController) AddTrace(ctx context.Context, key string, value any) {
	logger.AddTrace(ctx, key, value)
}

func (c *BaseController) AddInfo(ctx context.Context, key string, value any) {
	logger.AddInfo(ctx, key, value)
}

func (c *BaseController) AddWarning(ctx context.Context, key string, value any) {
	logger.AddWarning(ctx, key, value)
}

func (c *BaseController) AddFatal(ctx context.Context, key string, value any) {
	logger.AddFatal(ctx, key, value)
}

func ControllerAsMiddleware(ctx context.Context, controller Controller, w http.ResponseWriter, req *http.Request) Middleware {
	return func(ctx context.Context, w http.ResponseWriter, req *http.Request, queue MiddlewareQueue) error {
		err := controller.Init(ctx)
		if err != nil {
			return err
		}
		err = controller.Serve(ctx)
		if err != nil {
			return err
		}
		err = controller.Finalize(ctx)
		if err != nil {
			return err
		}
		return nil
	}
}
