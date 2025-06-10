package golitekit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

const (
	DefaultMaxMemorySize = 10 << 20
	DefaultMaxBodySize   = 10 << 20
)

type RequestSizeLimiter interface {
	MaxMemorySize() int64
	MaxBodySize() int64
}

type Controller interface {
	RequestSizeLimiter

	Init(ctx context.Context) error
	Serve(ctx context.Context) error
	Finalize(ctx context.Context) error
}

type BaseController struct {
	request *http.Request
	logger  logger.Logger

	rawBody []byte

	gcx *Context
}

func (c *BaseController) MaxMemorySize() int64 {
	return DefaultMaxMemorySize
}

func (c *BaseController) MaxBodySize() int64 {
	return DefaultMaxBodySize
}

func (c *BaseController) Init(ctx context.Context) error {
	c.gcx = GetContext(ctx)
	c.request = c.gcx.Request()
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
		maxMemorySize = DefaultMaxMemorySize // 10M
	}
	maxBodySize := c.MaxBodySize()
	if maxBodySize <= 0 {
		maxBodySize = DefaultMaxBodySize // 10M
	}

	httpReq := c.request
	httpReq.Body = http.MaxBytesReader(c.gcx.responseWriter, c.request.Body, maxBodySize)

	var err error
	ct := c.request.Header.Get("Content-Type")

	switch ct {
	case "application/x-www-form-urlencoded":
		err = c.request.ParseForm()
	case "multipart/form-data":
		err = c.request.ParseMultipartForm(maxMemorySize)
	default:
		if httpReq.Body != nil {
			originBody := httpReq.Body
			// capable of reading data multiple times
			c.rawBody, err = io.ReadAll(originBody)
			if err != nil {
				return err
			}
			defer originBody.Close()
			httpReq.Body = io.NopCloser(bytes.NewBuffer(c.rawBody))
		}
	}

	return err
}

func (c *BaseController) ServeRawData(data any) {
	c.gcx.ServeRawData(data)
}

func (c *BaseController) ServeJSON(data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	c.gcx.ServeJSON(jsonData)
	return nil
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

func (c *BaseController) forms() (map[string][]string, error) {
	ct := c.request.Header.Get("Content-Type")
	ct, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return nil, err
	}

	switch ct {
	case "application/x-www-form-urlencoded":
		return c.request.Form, nil
	case "multipart/form-data":
		return c.request.PostForm, nil
	}
	return nil, nil
}

func (c *BaseController) FormString(key string, def string) string {
	params, err := c.forms()
	if err != nil {
		return def
	}
	if vals, ok := params[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	return def
}

func (c *BaseController) FormInt(key string, def int) int {
	params, err := c.forms()
	if err != nil {
		return def
	}
	if vals, ok := params[key]; ok && len(vals) > 0 {
		if ival, err := strconv.Atoi(vals[0]); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController) FormInt64(key string, def int64) int64 {
	params, err := c.forms()
	if err != nil {
		return def
	}
	if vals, ok := params[key]; ok && len(vals) > 0 {
		if ival, err := strconv.ParseInt(vals[0], 10, 64); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController) FormFloat32(key string, def float32) float32 {
	params, err := c.forms()
	if err != nil {
		return def
	}
	if vals, ok := params[key]; ok && len(vals) > 0 {
		if fval, err := strconv.ParseFloat(vals[0], 32); err == nil {
			return float32(fval)
		}
	}
	return def
}

func (c *BaseController) FormFloat64(key string, def float64) float64 {
	params, err := c.forms()
	if err != nil {
		return def
	}
	if vals, ok := params[key]; ok && len(vals) > 0 {
		if fval, err := strconv.ParseFloat(vals[0], 64); err == nil {
			return fval
		}
	}
	return def
}

func (c *BaseController) FormBool(key string, def bool) bool {
	params, err := c.forms()
	if err != nil {
		return def
	}
	if vals, ok := params[key]; ok && len(vals) > 0 {
		return vals[0] == "1" || strings.ToLower(vals[0]) == "true"
	}
	return def
}

func (c *BaseController) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return c.request.FormFile(key)
}

func (c *BaseController) PathValueString(key string, def string) string {
	if val := c.request.PathValue(key); val != "" {
		return val
	}
	return def
}

func (c *BaseController) PathValueInt(key string, def int) int {
	if val := c.request.PathValue(key); val != "" {
		if ival, err := strconv.Atoi(val); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController) PathValueInt64(key string, def int64) int64 {
	if val := c.request.PathValue(key); val != "" {
		if ival, err := strconv.ParseInt(val, 10, 64); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController) PathValueFloat32(key string, def float32) float32 {
	if val := c.request.PathValue(key); val != "" {
		if fval, err := strconv.ParseFloat(val, 32); err == nil {
			return float32(fval)
		}
	}
	return def
}

func (c *BaseController) PathValueFloat64(key string, def float64) float64 {
	if val := c.request.PathValue(key); val != "" {
		if fval, err := strconv.ParseFloat(val, 64); err == nil {
			return fval
		}
	}
	return def
}

func (c *BaseController) PathValueBool(key string, def bool) bool {
	if val := c.request.PathValue(key); val != "" {
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

func (c *BaseController) Debug(ctx context.Context, format string, args ...any) {
	c.logger.Debug(ctx, format, args...)
}

func (c *BaseController) Trace(ctx context.Context, format string, args ...any) {
	c.logger.Trace(ctx, format, args...)
}

func (c *BaseController) Info(ctx context.Context, format string, args ...any) {
	c.logger.Info(ctx, format, args...)
}

func (c *BaseController) Warning(ctx context.Context, format string, args ...any) {
	c.logger.Warning(ctx, format, args...)
}

func (c *BaseController) Fatal(ctx context.Context, format string, args ...any) {
	c.logger.Fatal(ctx, format, args...)
}

func controllerAsMiddleware(c Controller) Middleware {
	return func(ctx context.Context, queue MiddlewareQueue) error {
		err := c.Init(ctx)
		if err != nil {
			return err
		}
		err = c.Serve(ctx)
		if err != nil {
			return err
		}
		err = c.Finalize(ctx)
		if err != nil {
			return err
		}
		return queue.Next(ctx)
	}
}

func CloneController(src Controller) Controller {
	srcValue := reflect.ValueOf(src)
	if srcValue.Kind() == reflect.Ptr {
		if srcValue.IsNil() {
			return nil
		}
		srcValue = srcValue.Elem()
	}
	dstValue := reflect.New(srcValue.Type()).Elem()
	copyFields(srcValue, dstValue)
	return dstValue.Addr().Interface().(Controller)
}

func copyFields(src, dst reflect.Value) {
	for i := 0; i < src.NumField(); i++ {
		srcField := src.Field(i)
		dstField := dst.Field(i)
		if !dstField.CanSet() {
			continue
		}
		switch srcField.Kind() {
		case reflect.Struct:
			copyFields(srcField, dstField)
		case reflect.Ptr:
			if srcField.IsNil() {
				continue
			}
			newPtr := reflect.New(srcField.Type().Elem())
			copyFields(srcField.Elem(), newPtr.Elem())
			dstField.Set(newPtr)
		case reflect.Slice:
			if srcField.IsNil() {
				continue
			}
			dstField.Set(reflect.MakeSlice(srcField.Type(), srcField.Len(), srcField.Cap()))
			for j := 0; j < srcField.Len(); j++ {
				copyFields(srcField.Index(j), dstField.Index(j))
			}
		case reflect.Map:
			if srcField.IsNil() {
				continue
			}
			dstField.Set(reflect.MakeMap(srcField.Type()))
			for _, key := range srcField.MapKeys() {
				newKey := reflect.New(key.Type()).Elem()
				copyFields(key, newKey)
				newValue := reflect.New(srcField.MapIndex(key).Type()).Elem()
				copyFields(srcField.MapIndex(key), newValue)
				dstField.SetMapIndex(newKey, newValue)
			}
		case reflect.Array:
			for j := 0; j < srcField.Len(); j++ {
				copyFields(srcField.Index(j), dstField.Index(j))
			}
		case reflect.Chan:
			if srcField.IsNil() {
				continue
			}
			dstField.Set(srcField)
		default:
			dstField.Set(srcField)
		}
	}
}
