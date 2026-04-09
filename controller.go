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
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	DefaultMaxMemorySize = 10 << 20
	DefaultMaxBodySize   = 10 << 20
)

type RequestSizeLimiter interface {
	MaxMemorySize() int64
	MaxBodySize() int64
}

// NoBody indicates the controller does not need request body parsing.
type NoBody struct{}

// Controller is the interface every request handler must implement.
type Controller interface {
	RequestSizeLimiter

	Init(ctx context.Context) error
	SanityCheck(ctx context.Context) error
	ParseRequest(ctx context.Context, body []byte) error
	Serve(ctx context.Context) error
	Finalize(ctx context.Context) error
}

// BaseController is a generic controller base. T is the request struct or NoBody.
type BaseController[T any] struct {
	request *http.Request
	logger  logger.Logger
	rawBody []byte
	gcx     *Context
	Request T
}

func (c *BaseController[T]) MaxMemorySize() int64 {
	return DefaultMaxMemorySize
}

func (c *BaseController[T]) MaxBodySize() int64 {
	return DefaultMaxBodySize
}

func (c *BaseController[T]) Init(ctx context.Context) error {
	c.gcx = GetContext(ctx)
	if c.gcx == nil {
		return fmt.Errorf("golitekit: context not initialized; ensure WithContext is called before the controller")
	}
	c.request = c.gcx.Request()
	c.logger = c.gcx.logger

	if err := c.parseBody(); err != nil {
		return err
	}

	return nil
}

func (c *BaseController[T]) DB() *gorm.DB {
	if c.gcx == nil {
		return nil
	}
	return c.gcx.DB()
}

func (c *BaseController[T]) Redis() *redis.Client {
	if c.gcx == nil {
		return nil
	}
	return c.gcx.Redis()
}

func (c *BaseController[T]) SanityCheck(ctx context.Context) error {
	return nil
}

func (c *BaseController[T]) ParseRequest(ctx context.Context, body []byte) error {
	var zeroValue T
	if _, isNoBody := any(zeroValue).(NoBody); isNoBody {
		return nil
	}

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

// bindFormData binds form data to a struct.
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

	forms, err := c.forms()
	if err != nil {
		return err
	}

	for i := 0; i < dstValue.NumField(); i++ {
		field := dstValue.Field(i)
		fieldType := dstType.Field(i)

		if !field.CanSet() {
			continue
		}

		formTag := fieldType.Tag.Get("form")
		if formTag == "" {
			formTag = fieldType.Tag.Get("json")
		}
		if formTag == "" {
			formTag = fieldType.Name
		}

		values, ok := forms[formTag]
		if !ok || len(values) == 0 {
			continue
		}

		value := values[0]

		if err := c.setFieldValue(field, value); err != nil {
			return fmt.Errorf("failed to set field %s: %w", fieldType.Name, err)
		}
	}

	return nil
}

// setFieldValue sets a struct field from string value.
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

	case reflect.Ptr:
		// Allocate a new value of the pointed-to type, parse the string into it,
		// then point the field at the new value.  This supports optional fields
		// declared as *string, *int64, *bool, etc.
		elem := reflect.New(field.Type().Elem()).Elem()
		if err := c.setFieldValue(elem, value); err != nil {
			return err
		}
		ptr := reflect.New(field.Type().Elem())
		ptr.Elem().Set(elem)
		field.Set(ptr)

	default:
		return fmt.Errorf("unsupported field type: %v", field.Kind())
	}

	return nil
}

// Error sets a custom AppError and returns it.
func (c *BaseController[T]) Error(err *AppError) error {
	c.gcx.setError(err)
	return err
}

// BadRequest sets a 400 error and returns it.
func (c *BaseController[T]) BadRequest(msg string, internal error) error {
	return c.Error(ErrBadRequest(msg, internal))
}

// Unauthorized sets a 401 error and returns it.
func (c *BaseController[T]) Unauthorized(msg string) error {
	return c.Error(ErrUnauthorized(msg))
}

// Forbidden sets a 403 error and returns it.
func (c *BaseController[T]) Forbidden(msg string) error {
	return c.Error(ErrForbidden(msg))
}

// NotFound sets a 404 error and returns it.
func (c *BaseController[T]) NotFound(msg string) error {
	return c.Error(ErrNotFound(msg))
}

// Conflict sets a 409 error and returns it.
func (c *BaseController[T]) Conflict(msg string) error {
	return c.Error(ErrConflict(msg))
}

// TooManyRequests sets a 429 error and returns it.
func (c *BaseController[T]) TooManyRequests(msg string) error {
	return c.Error(ErrTooManyRequests(msg))
}

// InternalError sets a 500 error and returns it.
func (c *BaseController[T]) InternalError(msg string, internal error) error {
	return c.Error(ErrInternal(msg, internal))
}

// HasError checks if there is an error in the current context.
func (c *BaseController[T]) HasError() bool {
	if c.gcx == nil || c.request == nil {
		return false
	}
	return HasError(c.request.Context())
}

// Deprecated: Use c.Error(err) instead.
func (c *BaseController[T]) SetError(ctx context.Context, err *AppError) {
	c.gcx.setError(err)
}

// Deprecated: Use c.BadRequest(msg, err) instead.
func (c *BaseController[T]) SetBadRequest(ctx context.Context, msg string, internal error) {
	c.gcx.setError(ErrBadRequest(msg, internal))
}

// Deprecated: Use c.Unauthorized(msg) instead.
func (c *BaseController[T]) SetUnauthorized(ctx context.Context, msg string) {
	c.gcx.setError(ErrUnauthorized(msg))
}

// Deprecated: Use c.Forbidden(msg) instead.
func (c *BaseController[T]) SetForbidden(ctx context.Context, msg string) {
	c.gcx.setError(ErrForbidden(msg))
}

// Deprecated: Use c.NotFound(msg) instead.
func (c *BaseController[T]) SetNotFound(ctx context.Context, msg string) {
	c.gcx.setError(ErrNotFound(msg))
}

// Deprecated: Use c.Conflict(msg) instead.
func (c *BaseController[T]) SetConflict(ctx context.Context, msg string) {
	c.gcx.setError(ErrConflict(msg))
}

// Deprecated: Use c.TooManyRequests(msg) instead.
func (c *BaseController[T]) SetTooManyRequests(ctx context.Context, msg string) {
	c.gcx.setError(ErrTooManyRequests(msg))
}

// Deprecated: Use c.InternalError(msg, err) instead.
func (c *BaseController[T]) SetInternalError(ctx context.Context, msg string, internal error) {
	c.gcx.setError(ErrInternal(msg, internal))
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
		maxMemorySize = DefaultMaxMemorySize
	}
	maxBodySize := c.MaxBodySize()
	if maxBodySize <= 0 {
		maxBodySize = DefaultMaxBodySize
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
			defer originBody.Close() // registered before ReadAll; fires even on read error
			var rawBody []byte
			rawBody, err = io.ReadAll(originBody)
			if err != nil {
				return err
			}
			c.gcx.RawBody = rawBody
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
		if c.request.MultipartForm != nil {
			return c.request.MultipartForm.Value, nil
		}
		return nil, nil
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
	if c.logger != nil {
		c.logger.Debug(ctx, format, args...)
	}
}

func (c *BaseController[T]) Trace(ctx context.Context, format string, args ...any) {
	if c.logger != nil {
		c.logger.Trace(ctx, format, args...)
	}
}

func (c *BaseController[T]) Info(ctx context.Context, format string, args ...any) {
	if c.logger != nil {
		c.logger.Info(ctx, format, args...)
	}
}

func (c *BaseController[T]) Warning(ctx context.Context, format string, args ...any) {
	if c.logger != nil {
		c.logger.Warning(ctx, format, args...)
	}
}

func (c *BaseController[T]) Fatal(ctx context.Context, format string, args ...any) {
	if c.logger != nil {
		c.logger.Fatal(ctx, format, args...)
	}
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

// CloneController returns a deep copy of src, skipping sync primitives and channels.
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

// isSyncType checks if the type is a sync primitive.
func isSyncType(t reflect.Type) bool {
	typeName := t.String()
	if strings.HasPrefix(typeName, "sync.") {
		return true
	}
	switch t.Name() {
	case "Mutex", "RWMutex", "WaitGroup", "Cond", "Once", "Pool", "Map":
		if t.PkgPath() == "sync" {
			return true
		}
	}
	return false
}

// deepCopyValue creates a deep copy of a value.
func deepCopyValue(src reflect.Value) reflect.Value {
	if !src.IsValid() {
		return src
	}

	switch src.Kind() {
	case reflect.Ptr:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		dst := reflect.New(src.Type().Elem())
		if src.Type().Elem().Kind() == reflect.Struct {
			copyFields(src.Elem(), dst.Elem())
		} else {
			// Pointer to a primitive or non-struct type: copy the value directly.
			dst.Elem().Set(src.Elem())
		}
		return dst

	case reflect.Interface:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		elem := src.Elem()
		copiedElem := deepCopyValue(elem)
		dst := reflect.New(src.Type()).Elem()
		if copiedElem.IsValid() {
			dst.Set(copiedElem)
		}
		return dst

	case reflect.Struct:
		dst := reflect.New(src.Type()).Elem()
		copyFields(src, dst)
		return dst

	case reflect.Slice:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		dst := reflect.MakeSlice(src.Type(), src.Len(), src.Cap())
		for i := 0; i < src.Len(); i++ {
			dst.Index(i).Set(deepCopyValue(src.Index(i)))
		}
		return dst

	case reflect.Map:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		dst := reflect.MakeMap(src.Type())
		for _, key := range src.MapKeys() {
			copiedKey := deepCopyValue(key)
			copiedValue := deepCopyValue(src.MapIndex(key))
			dst.SetMapIndex(copiedKey, copiedValue)
		}
		return dst

	case reflect.Array:
		dst := reflect.New(src.Type()).Elem()
		for i := 0; i < src.Len(); i++ {
			dst.Index(i).Set(deepCopyValue(src.Index(i)))
		}
		return dst

	default:
		dst := reflect.New(src.Type()).Elem()
		dst.Set(src)
		return dst
	}
}

func copyFields(src, dst reflect.Value) {
	srcType := src.Type()

	for i := 0; i < src.NumField(); i++ {
		srcField := src.Field(i)
		dstField := dst.Field(i)
		fieldType := srcType.Field(i)

		if !dstField.CanSet() {
			continue
		}

		// Skip sync primitives.
		if isSyncType(fieldType.Type) {
			continue
		}

		// If the struct embeds sync types, copy its fields individually.
		if srcField.Kind() == reflect.Struct && containsSyncType(fieldType.Type) {
			copyFields(srcField, dstField)
			continue
		}

		switch srcField.Kind() {
		case reflect.Interface:
			if !srcField.IsNil() {
				copied := deepCopyValue(srcField)
				if copied.IsValid() {
					dstField.Set(copied)
				}
			}

		case reflect.Struct:
			copyFields(srcField, dstField)

		case reflect.Ptr:
			if srcField.IsNil() {
				continue
			}
		// Skip pointers to sync primitives.
		if isSyncType(srcField.Type().Elem()) {
				continue
			}
			newPtr := reflect.New(srcField.Type().Elem())
			if srcField.Type().Elem().Kind() == reflect.Struct {
				copyFields(srcField.Elem(), newPtr.Elem())
			} else {
				// Pointer to a primitive or non-struct type: copy the value directly.
				newPtr.Elem().Set(srcField.Elem())
			}
			dstField.Set(newPtr)

		case reflect.Slice:
			if srcField.IsNil() {
				continue
			}
			dstField.Set(reflect.MakeSlice(srcField.Type(), srcField.Len(), srcField.Cap()))
			for j := 0; j < srcField.Len(); j++ {
				srcElem := srcField.Index(j)
				dstElem := dstField.Index(j)
				if srcElem.Kind() == reflect.Struct {
					copyFields(srcElem, dstElem)
				} else if srcElem.Kind() == reflect.Ptr || srcElem.Kind() == reflect.Interface {
					copied := deepCopyValue(srcElem)
					if copied.IsValid() {
						dstElem.Set(copied)
					}
				} else {
					dstElem.Set(srcElem)
				}
			}

		case reflect.Map:
			if srcField.IsNil() {
				continue
			}
			dstField.Set(reflect.MakeMap(srcField.Type()))
			for _, key := range srcField.MapKeys() {
				copiedKey := deepCopyValue(key)
				copiedValue := deepCopyValue(srcField.MapIndex(key))
				dstField.SetMapIndex(copiedKey, copiedValue)
			}

		case reflect.Array:
			for j := 0; j < srcField.Len(); j++ {
				srcElem := srcField.Index(j)
				dstElem := dstField.Index(j)
				if srcElem.Kind() == reflect.Struct {
					copyFields(srcElem, dstElem)
				} else if srcElem.Kind() == reflect.Ptr || srcElem.Kind() == reflect.Interface {
					copied := deepCopyValue(srcElem)
					if copied.IsValid() {
						dstElem.Set(copied)
					}
				} else {
					dstElem.Set(srcElem)
				}
			}

		case reflect.Chan:
			continue

		case reflect.Func:
			if !srcField.IsNil() {
				dstField.Set(srcField)
			}

		default:
			dstField.Set(srcField)
		}
	}
}

// containsSyncType checks if a struct type contains sync primitives.
func containsSyncType(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if isSyncType(field.Type) {
			return true
		}
		if field.Type.Kind() == reflect.Struct {
			if containsSyncType(field.Type) {
				return true
			}
		}
	}
	return false
}
