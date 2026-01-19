package golitekit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// NoBody is a special tag type that indicates that the Controller does not need to parse the request body
// Used for requests such as GET, DELETE, etc. that usually have no body
type NoBody struct{}

type Controller interface {
	RequestSizeLimiter

	Init(ctx context.Context) error
	SanityCheck(ctx context.Context) error
	ParseRequest(ctx context.Context, body []byte) error
	Serve(ctx context.Context) error
	Finalize(ctx context.Context) error
}

// BaseController is a unified generic Controller base class
// T can be:
//   - Specific request structure: Automatically parse JSON/Form to c.Request
//   - NoBody：indicates no request body, no parsing is performed
type BaseController[T any] struct {
	request *http.Request
	logger  logger.Logger

	rawBody []byte

	gcx *Context

	Request T // Automatically parsed request data
}

func (c *BaseController[T]) MaxMemorySize() int64 {
	return DefaultMaxMemorySize
}

func (c *BaseController[T]) MaxBodySize() int64 {
	return DefaultMaxBodySize
}

func (c *BaseController[T]) Init(ctx context.Context) error {
	c.gcx = GetContext(ctx)
	c.request = c.gcx.Request()
	c.logger = c.gcx.logger
	c.parseBody()

	return nil
}

func (c *BaseController[T]) SanityCheck(ctx context.Context) error {
	return nil
}

func (c *BaseController[T]) ParseRequest(ctx context.Context, body []byte) error {
	// Check if it is of NoBody type
	var zeroValue T
	if _, isNoBody := any(zeroValue).(NoBody); isNoBody {
		return nil // NoBody type, skip parsing
	}

	// If the body is empty, return directly
	if len(body) == 0 {
		return nil
	}

	ct := c.request.Header.Get("Content-Type")
	switch {
	case strings.Contains(ct, "application/json"):
		return json.Unmarshal(body, &c.Request)
	case strings.Contains(ct, "application/x-www-form-urlencoded"):
		return c.bindFormData(&c.Request)
	case strings.Contains(ct, "multipart/form-data"):
		return c.bindFormData(&c.Request)
	default:
		return json.Unmarshal(body, &c.Request)
	}
}

// bindFormData Bind form data to a structure
func (c *BaseController[T]) bindFormData(dst *T) error {
	dstValue := reflect.ValueOf(dst)
	if dstValue.Kind() != reflect.Pointer {
		return fmt.Errorf("dst must be a pointer")
	}

	dstValue = dstValue.Elem()
	if dstValue.Kind() != reflect.Struct {
		return fmt.Errorf("dst must be a struct pointer")
	}

	dstType := dstValue.Type()

	// Get form data
	forms, err := c.forms()
	if err != nil {
		return err
	}

	// Traverse structure fields
	for i := 0; i < dstValue.NumField(); i++ {
		field := dstValue.Field(i)
		fieldType := dstType.Field(i)

		// Skip fields that cannot be set
		if !field.CanSet() {
			continue
		}

		// using form tag
		formTag := fieldType.Tag.Get("form")
		if formTag == "" {
			// If there is no form tag, try using JSON tags
			formTag = fieldType.Tag.Get("json")
		}
		if formTag == "" {
			// If none exist, use field names
			formTag = fieldType.Name
		}

		// Retrieve values from the form
		values, ok := forms[formTag]
		if !ok || len(values) == 0 {
			continue
		}

		value := values[0]

		// Set values based on field types
		if err := c.setFieldValue(field, value); err != nil {
			return fmt.Errorf("failed to set field %s: %w", fieldType.Name, err)
		}
	}

	return nil
}

// setFieldValue Set field values
func (c *BaseController[T]) setFieldValue(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(intVal)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetUint(uintVal)

	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		field.SetFloat(floatVal)

	case reflect.Bool:
		boolVal := value == "1" || strings.ToLower(value) == "true"
		field.SetBool(boolVal)

	default:
		return fmt.Errorf("unsupported field type: %v", field.Kind())
	}

	return nil
}

func (c *BaseController[T]) SetError(ctx context.Context, err *AppError) {
	SetError(ctx, err)
}

// 400
func (c *BaseController[T]) SetBadRequest(ctx context.Context, msg string, internal error) {
	SetError(ctx, ErrBadRequest(msg, internal))
}

// 401
func (c *BaseController[T]) SetUnauthorized(ctx context.Context, msg string) {
	SetError(ctx, ErrUnauthorized(msg))
}

// 403
func (c *BaseController[T]) SetForbidden(ctx context.Context, msg string) {
	SetError(ctx, ErrForbidden(msg))
}

// 404
func (c *BaseController[T]) SetNotFound(ctx context.Context, msg string) {
	SetError(ctx, ErrNotFound(msg))
}

// 409
func (c *BaseController[T]) SetConflict(ctx context.Context, msg string) {
	SetError(ctx, ErrConflict(msg))
}

// 429
func (c *BaseController[T]) SetTooManyRequests(ctx context.Context, msg string) {
	SetError(ctx, ErrTooManyRequests(msg))
}

// 500
func (c *BaseController[T]) SetInternalError(ctx context.Context, msg string, internal error) {
	SetError(ctx, ErrInternal(msg, internal))
}

// check if have error
func (c *BaseController[T]) HasError(ctx context.Context) bool {
	return HasError(ctx)
}

func (c *BaseController[T]) Serve(ctx context.Context) error {
	return nil
}

func (c *BaseController[T]) Finalize(ctx context.Context) error {
	return nil
}

func (c *BaseController[T]) GetRequest() T {
	return c.Request
}

func (c *BaseController[T]) parseBody() error {
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

	switch {
	case strings.HasPrefix(ct, "application/x-www-form-urlencoded"):
		err = c.request.ParseForm()
	case strings.HasPrefix(ct, "multipart/form-data"):
		err = c.request.ParseMultipartForm(maxMemorySize)
	default:
		if httpReq.Body != nil {
			originBody := httpReq.Body
			// capable of reading data multiple times
			var rawBody []byte
			rawBody, err = io.ReadAll(originBody)
			if err != nil {
				return err
			}
			c.gcx.RawBody = rawBody
			defer originBody.Close()
			httpReq.Body = io.NopCloser(bytes.NewBuffer(rawBody))
		}
	}

	return err
}

func (c *BaseController[T]) ServeRawData(data any) {
	c.gcx.ServeRawData(data)
}

func (c *BaseController[T]) ServeJSON(data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	c.gcx.ServeJSON(jsonData)
	return nil
}

func (c *BaseController[T]) ServeHTML(html string) {
	c.gcx.ServeHTML(html)
}

func (c *BaseController[T]) ServeSSE() *SSEWriter {
	return c.gcx.SSEWriter()
}

func (c *BaseController[T]) QueryInt(key string, def int) int {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		if ival, err := strconv.Atoi(vals[0]); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController[T]) QueryInt64(key string, def int64) int64 {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		if ival, err := strconv.ParseInt(vals[0], 10, 64); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController[T]) QueryFloat32(key string, def float32) float32 {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		if fval, err := strconv.ParseFloat(vals[0], 32); err == nil {
			return float32(fval)
		}
	}
	return def
}

func (c *BaseController[T]) QueryFloat64(key string, def float64) float64 {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		if fval, err := strconv.ParseFloat(vals[0], 64); err == nil {
			return fval
		}
	}
	return def
}

func (c *BaseController[T]) QueryString(key string, def string) string {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		return vals[0]
	}
	return def
}

func (c *BaseController[T]) QueryBool(key string, def bool) bool {
	params := c.request.URL.Query()
	if vals, ok := params[key]; ok {
		return vals[0] == "1" || strings.ToLower(vals[0]) == "true"
	}
	return def
}

func (c *BaseController[T]) forms() (map[string][]string, error) {
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

func (c *BaseController[T]) FormString(key string, def string) string {
	params, err := c.forms()
	if err != nil {
		return def
	}
	if vals, ok := params[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	return def
}

func (c *BaseController[T]) FormInt(key string, def int) int {
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

func (c *BaseController[T]) FormInt64(key string, def int64) int64 {
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

func (c *BaseController[T]) FormFloat32(key string, def float32) float32 {
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

func (c *BaseController[T]) FormFloat64(key string, def float64) float64 {
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

func (c *BaseController[T]) FormBool(key string, def bool) bool {
	params, err := c.forms()
	if err != nil {
		return def
	}
	if vals, ok := params[key]; ok && len(vals) > 0 {
		return vals[0] == "1" || strings.ToLower(vals[0]) == "true"
	}
	return def
}

func (c *BaseController[T]) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return c.request.FormFile(key)
}

func (c *BaseController[T]) PathValueString(key string, def string) string {
	if val := c.request.PathValue(key); val != "" {
		return val
	}
	return def
}

func (c *BaseController[T]) PathValueInt(key string, def int) int {
	if val := c.request.PathValue(key); val != "" {
		if ival, err := strconv.Atoi(val); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController[T]) PathValueInt64(key string, def int64) int64 {
	if val := c.request.PathValue(key); val != "" {
		if ival, err := strconv.ParseInt(val, 10, 64); err == nil {
			return ival
		}
	}
	return def
}

func (c *BaseController[T]) PathValueFloat32(key string, def float32) float32 {
	if val := c.request.PathValue(key); val != "" {
		if fval, err := strconv.ParseFloat(val, 32); err == nil {
			return float32(fval)
		}
	}
	return def
}

func (c *BaseController[T]) PathValueFloat64(key string, def float64) float64 {
	if val := c.request.PathValue(key); val != "" {
		if fval, err := strconv.ParseFloat(val, 64); err == nil {
			return fval
		}
	}
	return def
}

func (c *BaseController[T]) PathValueBool(key string, def bool) bool {
	if val := c.request.PathValue(key); val != "" {
		return val == "1" || strings.ToLower(val) == "true"
	}
	return def
}

func (c *BaseController[T]) AddDebug(ctx context.Context, key string, value any) {
	logger.AddDebug(ctx, key, value)
}

func (c *BaseController[T]) AddTrace(ctx context.Context, key string, value any) {
	logger.AddTrace(ctx, key, value)
}

func (c *BaseController[T]) AddInfo(ctx context.Context, key string, value any) {
	logger.AddInfo(ctx, key, value)
}

func (c *BaseController[T]) AddWarning(ctx context.Context, key string, value any) {
	logger.AddWarning(ctx, key, value)
}

func (c *BaseController[T]) AddFatal(ctx context.Context, key string, value any) {
	logger.AddFatal(ctx, key, value)
}

func (c *BaseController[T]) Debug(ctx context.Context, format string, args ...any) {
	c.logger.Debug(ctx, format, args...)
}

func (c *BaseController[T]) Trace(ctx context.Context, format string, args ...any) {
	c.logger.Trace(ctx, format, args...)
}

func (c *BaseController[T]) Info(ctx context.Context, format string, args ...any) {
	c.logger.Info(ctx, format, args...)
}

func (c *BaseController[T]) Warning(ctx context.Context, format string, args ...any) {
	c.logger.Warning(ctx, format, args...)
}

func (c *BaseController[T]) Fatal(ctx context.Context, format string, args ...any) {
	c.logger.Fatal(ctx, format, args...)
}

func (c *BaseController[T]) SendSSE(event SSEvent) error {
	return c.gcx.SSEWriter().Send(event)
}

func (c *BaseController[T]) SendSSEData(data interface{}) error {
	return c.SendSSE(SSEvent{Data: data})
}

func (c *BaseController[T]) SendSSEEvent(eventType string, data interface{}) error {
	return c.SendSSE(SSEvent{
		Event: eventType,
		Data:  data,
	})
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
