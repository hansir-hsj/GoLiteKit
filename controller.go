package golitekit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
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

// Controller is the minimal interface for handling HTTP requests.
// Embed BaseController and implement Serve() for most use cases.
type Controller interface {
	RequestSizeLimiter
	Serve(ctx context.Context) error
}

// Optional lifecycle hooks. Implement these interfaces to customize behavior.

// Initializer is called first to set up per-request state.
type Initializer interface {
	Init(ctx context.Context) error
}

// Validator is called after ParseRequest to validate parsed request data and controller state.
type Validator interface {
	Validate(ctx context.Context) error
}

// RequestParser is called after Init and before Validate, and owns request parsing.
// Custom parsers should read from the request or context as needed.
// BaseControllerOf implements JSON/form/multipart parsing automatically.
type RequestParser interface {
	ParseRequest(ctx context.Context) error
}

// Finalizer is called after Serve for cleanup, metrics, or audit logging.
type Finalizer interface {
	Finalize(ctx context.Context) error
}

// BaseControllerOf is a generic controller base. T is the request struct type.
// Use BaseController directly when no request body is needed.
type BaseControllerOf[T any] struct {
	request *http.Request
	logger  logger.Logger
	gcx     *Context
	// limits is the outer controller, set by the router so that overridden
	// MaxMemorySize/MaxBodySize take effect (embedding has no virtual dispatch).
	limits  RequestSizeLimiter
	Request T
}

// sizeLimitBinder is implemented by BaseControllerOf and promoted to the
// controllers embedding it.
type sizeLimitBinder interface {
	bindSizeLimits(RequestSizeLimiter)
}

func (c *BaseControllerOf[T]) bindSizeLimits(l RequestSizeLimiter) {
	c.limits = l
}

// BaseController is the no-body controller base (alias for BaseControllerOf[NoBody]).
// Embed this when the endpoint does not parse a request body.
type BaseController = BaseControllerOf[NoBody]

func (c *BaseControllerOf[T]) MaxMemorySize() int64 {
	return DefaultMaxMemorySize
}

func (c *BaseControllerOf[T]) MaxBodySize() int64 {
	return DefaultMaxBodySize
}

func (c *BaseControllerOf[T]) Init(ctx context.Context) error {
	c.gcx = GetContext(ctx)
	if c.gcx == nil {
		return fmt.Errorf("golitekit: context not initialized; ensure the controller runs through Router")
	}
	c.request = c.gcx.Request()
	c.logger = c.gcx.logger
	return nil
}

func (c *BaseControllerOf[T]) DB() *gorm.DB {
	if c.gcx == nil {
		return nil
	}
	return c.gcx.DB()
}

func (c *BaseControllerOf[T]) Redis() *redis.Client {
	if c.gcx == nil {
		return nil
	}
	return c.gcx.Redis()
}

func (c *BaseControllerOf[T]) Service(key string) any {
	if c.gcx == nil {
		return nil
	}
	return c.gcx.Service(key)
}

func (c *BaseControllerOf[T]) Validate(ctx context.Context) error {
	return nil
}

func (c *BaseControllerOf[T]) ParseRequest(ctx context.Context) error {
	var zeroValue T
	if _, isNoBody := any(zeroValue).(NoBody); isNoBody {
		return nil
	}

	if err := c.parseBody(); err != nil {
		return err
	}

	// Form types: parseBody already called ParseForm/ParseMultipartForm,
	// so data is in request.Form — bind directly without checking body.
	if mt := mediaType(c.request); mt == mediaTypeForm || mt == mediaTypeMultipart {
		return c.bindFormData(&c.Request)
	}

	// For all other types (JSON, etc.) rely on rawBody populated by parseBody.
	if len(c.gcx.rawBody) == 0 {
		return nil
	}
	return json.Unmarshal(c.gcx.rawBody, &c.Request)
}

// bindFormData binds form data to a struct; see bindForm.
func (c *BaseControllerOf[T]) bindFormData(dst *T) error {
	forms, err := c.forms()
	if err != nil {
		return err
	}
	return bindForm(dst, forms)
}

// requestParseError converts a ParseRequest error into a client error. Known
// decoder and body-limit errors get a fixed message so Go type and field
// internals are not echoed to the client; the cause is kept as Internal.
func requestParseError(err error) *AppError {
	var maxBytesErr *http.MaxBytesError
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &maxBytesErr):
		return NewAppError(http.StatusRequestEntityTooLarge, "Request body too large", err)
	case errors.As(err, &syntaxErr):
		return ErrBadRequest("Invalid JSON body", err)
	case errors.As(err, &typeErr):
		if typeErr.Field != "" {
			return ErrBadRequest(fmt.Sprintf("Invalid type for field %q", typeErr.Field), err)
		}
		return ErrBadRequest("Invalid JSON body", err)
	}
	return WrapError(err, http.StatusBadRequest)
}

// BadRequest returns a 400 AppError. Use as: return c.BadRequest(...)
func (c *BaseControllerOf[T]) BadRequest(msg string, internal error) error {
	return ErrBadRequest(msg, internal)
}

// Unauthorized returns a 401 AppError.
func (c *BaseControllerOf[T]) Unauthorized(msg string) error {
	return ErrUnauthorized(msg, nil)
}

// Forbidden returns a 403 AppError.
func (c *BaseControllerOf[T]) Forbidden(msg string) error {
	return ErrForbidden(msg, nil)
}

// NotFound returns a 404 AppError.
func (c *BaseControllerOf[T]) NotFound(msg string) error {
	return ErrNotFound(msg, nil)
}

// Conflict returns a 409 AppError.
func (c *BaseControllerOf[T]) Conflict(msg string) error {
	return ErrConflict(msg, nil)
}

// TooManyRequests returns a 429 AppError.
func (c *BaseControllerOf[T]) TooManyRequests(msg string) error {
	return ErrTooManyRequests(msg, nil)
}

// InternalError returns a 500 AppError.
func (c *BaseControllerOf[T]) InternalError(msg string, internal error) error {
	return ErrInternal(msg, internal)
}

func (c *BaseControllerOf[T]) Serve(ctx context.Context) error {
	return nil
}

func (c *BaseControllerOf[T]) Finalize(ctx context.Context) error {
	return nil
}

func (c *BaseControllerOf[T]) GetRequest() T {
	return c.Request
}

// requestResetter is implemented by BaseControllerOf (and promoted to
// controllers embedding it) so the router can clear Request on each
// per-request copy of a controller prototype.
type requestResetter interface {
	resetRequest()
}

// resetRequest sets Request to its zero value. Request data must not be
// preset on a registered prototype: the copy is shallow, so maps and slices
// in it would be decoded into by concurrent requests. Set defaults in Init.
func (c *BaseControllerOf[T]) resetRequest() {
	var zero T
	c.Request = zero
}

func (c *BaseControllerOf[T]) parseBody() error {
	var limits RequestSizeLimiter = c
	if c.limits != nil {
		limits = c.limits
	}
	maxMemorySize := limits.MaxMemorySize()
	if maxMemorySize <= 0 {
		maxMemorySize = DefaultMaxMemorySize
	}
	maxBodySize := limits.MaxBodySize()
	if maxBodySize <= 0 {
		maxBodySize = DefaultMaxBodySize
	}

	httpReq := c.request
	httpReq.Body = http.MaxBytesReader(c.gcx.responseWriter, c.request.Body, maxBodySize)

	var err error
	switch mediaType(c.request) {
	case mediaTypeForm:
		err = c.request.ParseForm()
	case mediaTypeMultipart:
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
			c.gcx.rawBody = rawBody
			httpReq.Body = io.NopCloser(bytes.NewBuffer(rawBody))
		}
	}

	return err
}

func (c *BaseControllerOf[T]) JSON(code int, data any) error {
	return c.gcx.JSON(code, data)
}

func (c *BaseControllerOf[T]) String(code int, s string) error {
	return c.gcx.String(code, s)
}

func (c *BaseControllerOf[T]) Bytes(code int, data []byte) error {
	return c.gcx.Bytes(code, data)
}

func (c *BaseControllerOf[T]) HTML(code int, html string) error {
	return c.gcx.HTML(code, html)
}

func (c *BaseControllerOf[T]) SSE() *SSEWriter {
	return c.gcx.SSEWriter()
}

func (c *BaseControllerOf[T]) QueryInt(key string, def int) int {
	return parseValue(c.queryValue(key), def, strconv.Atoi)
}

func (c *BaseControllerOf[T]) QueryInt64(key string, def int64) int64 {
	return parseValue(c.queryValue(key), def, func(s string) (int64, error) {
		return strconv.ParseInt(s, 10, 64)
	})
}

func (c *BaseControllerOf[T]) QueryFloat32(key string, def float32) float32 {
	return parseValue(c.queryValue(key), def, func(s string) (float32, error) {
		v, err := strconv.ParseFloat(s, 32)
		return float32(v), err
	})
}

func (c *BaseControllerOf[T]) QueryFloat64(key string, def float64) float64 {
	return parseValue(c.queryValue(key), def, func(s string) (float64, error) {
		return strconv.ParseFloat(s, 64)
	})
}

func (c *BaseControllerOf[T]) QueryString(key string, def string) string {
	return parseValue(c.queryValue(key), def, func(s string) (string, error) {
		return s, nil
	})
}

func (c *BaseControllerOf[T]) QueryBool(key string, def bool) bool {
	return parseValue(c.queryValue(key), def, parseBool)
}

func (c *BaseControllerOf[T]) forms() (map[string][]string, error) {
	switch mediaType(c.request) {
	case mediaTypeForm:
		return c.request.Form, nil
	case mediaTypeMultipart:
		if c.request.MultipartForm != nil {
			return c.request.MultipartForm.Value, nil
		}
		return nil, nil
	}
	return nil, nil
}

func parseValue[T any](value string, def T, parse func(string) (T, error)) T {
	if value == "" {
		return def
	}
	parsed, err := parse(value)
	if err != nil {
		return def
	}
	return parsed
}

// parseBool accepts the strconv.ParseBool forms (1/t/true/0/f/false, any
// case) plus on/off, which HTML checkboxes submit; other values are errors, so
// the *Bool helpers return the default.
func parseBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "on":
		return true, nil
	case "off":
		return false, nil
	}
	return strconv.ParseBool(value)
}

// queryValue returns the first value of key, using the request Context's
// cached query parse.
func (c *BaseControllerOf[T]) queryValue(key string) string {
	if c.gcx != nil {
		return c.gcx.Query(key)
	}
	return c.request.URL.Query().Get(key)
}

func (c *BaseControllerOf[T]) formValue(key string) string {
	params, err := c.forms()
	if err != nil {
		return ""
	}
	if vals, ok := params[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (c *BaseControllerOf[T]) pathValue(key string) string {
	return c.request.PathValue(key)
}

func (c *BaseControllerOf[T]) FormString(key string, def string) string {
	return parseValue(c.formValue(key), def, func(s string) (string, error) {
		return s, nil
	})
}

func (c *BaseControllerOf[T]) FormInt(key string, def int) int {
	return parseValue(c.formValue(key), def, strconv.Atoi)
}

func (c *BaseControllerOf[T]) FormInt64(key string, def int64) int64 {
	return parseValue(c.formValue(key), def, func(s string) (int64, error) {
		return strconv.ParseInt(s, 10, 64)
	})
}

func (c *BaseControllerOf[T]) FormFloat32(key string, def float32) float32 {
	return parseValue(c.formValue(key), def, func(s string) (float32, error) {
		v, err := strconv.ParseFloat(s, 32)
		return float32(v), err
	})
}

func (c *BaseControllerOf[T]) FormFloat64(key string, def float64) float64 {
	return parseValue(c.formValue(key), def, func(s string) (float64, error) {
		return strconv.ParseFloat(s, 64)
	})
}

func (c *BaseControllerOf[T]) FormBool(key string, def bool) bool {
	return parseValue(c.formValue(key), def, parseBool)
}

func (c *BaseControllerOf[T]) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return c.request.FormFile(key)
}

func (c *BaseControllerOf[T]) PathValueString(key string, def string) string {
	return parseValue(c.pathValue(key), def, func(s string) (string, error) {
		return s, nil
	})
}

func (c *BaseControllerOf[T]) PathValueInt(key string, def int) int {
	return parseValue(c.pathValue(key), def, strconv.Atoi)
}

func (c *BaseControllerOf[T]) PathValueInt64(key string, def int64) int64 {
	return parseValue(c.pathValue(key), def, func(s string) (int64, error) {
		return strconv.ParseInt(s, 10, 64)
	})
}

func (c *BaseControllerOf[T]) PathValueFloat32(key string, def float32) float32 {
	return parseValue(c.pathValue(key), def, func(s string) (float32, error) {
		v, err := strconv.ParseFloat(s, 32)
		return float32(v), err
	})
}

func (c *BaseControllerOf[T]) PathValueFloat64(key string, def float64) float64 {
	return parseValue(c.pathValue(key), def, func(s string) (float64, error) {
		return strconv.ParseFloat(s, 64)
	})
}

func (c *BaseControllerOf[T]) PathValueBool(key string, def bool) bool {
	return parseValue(c.pathValue(key), def, parseBool)
}

func (c *BaseControllerOf[T]) AddDebug(ctx context.Context, key string, value any) {
	logger.AddDebug(ctx, key, value)
}

func (c *BaseControllerOf[T]) AddTrace(ctx context.Context, key string, value any) {
	logger.AddTrace(ctx, key, value)
}

func (c *BaseControllerOf[T]) AddInfo(ctx context.Context, key string, value any) {
	logger.AddInfo(ctx, key, value)
}

func (c *BaseControllerOf[T]) AddWarning(ctx context.Context, key string, value any) {
	logger.AddWarning(ctx, key, value)
}

func (c *BaseControllerOf[T]) AddFatal(ctx context.Context, key string, value any) {
	logger.AddFatal(ctx, key, value)
}

func (c *BaseControllerOf[T]) Debug(ctx context.Context, format string, args ...any) {
	if c.logger != nil {
		c.logger.Debug(ctx, format, args...)
	}
}

func (c *BaseControllerOf[T]) Trace(ctx context.Context, format string, args ...any) {
	if c.logger != nil {
		c.logger.Trace(ctx, format, args...)
	}
}

func (c *BaseControllerOf[T]) Info(ctx context.Context, format string, args ...any) {
	if c.logger != nil {
		c.logger.Info(ctx, format, args...)
	}
}

func (c *BaseControllerOf[T]) Warning(ctx context.Context, format string, args ...any) {
	if c.logger != nil {
		c.logger.Warning(ctx, format, args...)
	}
}

// Fatal logs at FATAL level and returns; it does not exit the process (see
// logger.Logger). Return an error from Serve to end the request.
func (c *BaseControllerOf[T]) Fatal(ctx context.Context, format string, args ...any) {
	if c.logger != nil {
		c.logger.Fatal(ctx, format, args...)
	}
}

func (c *BaseControllerOf[T]) SendSSE(event SSEvent) error {
	return c.gcx.SSEWriter().Send(event)
}

func (c *BaseControllerOf[T]) SendSSEData(data interface{}) error {
	return c.SendSSE(SSEvent{Data: data})
}

func (c *BaseControllerOf[T]) SendSSEEvent(eventType string, data interface{}) error {
	return c.SendSSE(SSEvent{
		Event: eventType,
		Data:  data,
	})
}
