package golitekit

import (
	"fmt"
	"net/http"
)

// AppError is an HTTP error with a status code, message, and optional internal cause.
type AppError struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	Internal error  `json:"-"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Internal)
	}
	return e.Message
}

// Unwrap returns the internal error for errors.Is/errors.As support.
func (e *AppError) Unwrap() error {
	return e.Internal
}

// ErrBadRequest returns a 400 AppError.
func ErrBadRequest(msg string, internal error) *AppError {
	return &AppError{Code: http.StatusBadRequest, Message: msg, Internal: internal}
}

// ErrUnauthorized returns a 401 AppError.
func ErrUnauthorized(msg string, internal ...error) *AppError {
	var i error
	if len(internal) > 0 {
		i = internal[0]
	}
	return &AppError{Code: http.StatusUnauthorized, Message: msg, Internal: i}
}

// ErrForbidden returns a 403 AppError.
func ErrForbidden(msg string, internal ...error) *AppError {
	var i error
	if len(internal) > 0 {
		i = internal[0]
	}
	return &AppError{Code: http.StatusForbidden, Message: msg, Internal: i}
}

// ErrNotFound returns a 404 AppError.
func ErrNotFound(msg string, internal ...error) *AppError {
	var i error
	if len(internal) > 0 {
		i = internal[0]
	}
	return &AppError{Code: http.StatusNotFound, Message: msg, Internal: i}
}

// ErrMethodNotAllowed returns a 405 AppError.
func ErrMethodNotAllowed(msg string, internal ...error) *AppError {
	var i error
	if len(internal) > 0 {
		i = internal[0]
	}
	return &AppError{Code: http.StatusMethodNotAllowed, Message: msg, Internal: i}
}

// ErrConflict returns a 409 AppError.
func ErrConflict(msg string, internal ...error) *AppError {
	var i error
	if len(internal) > 0 {
		i = internal[0]
	}
	return &AppError{Code: http.StatusConflict, Message: msg, Internal: i}
}

// ErrTooManyRequests returns a 429 AppError.
func ErrTooManyRequests(msg string, internal ...error) *AppError {
	var i error
	if len(internal) > 0 {
		i = internal[0]
	}
	return &AppError{Code: http.StatusTooManyRequests, Message: msg, Internal: i}
}

// ErrTimeout returns a 408 AppError.
func ErrTimeout(msg string, internal ...error) *AppError {
	var i error
	if len(internal) > 0 {
		i = internal[0]
	}
	return &AppError{Code: http.StatusRequestTimeout, Message: msg, Internal: i}
}

// ErrInternal returns a 500 AppError.
func ErrInternal(msg string, internal error) *AppError {
	return &AppError{Code: http.StatusInternalServerError, Message: msg, Internal: internal}
}

// ErrServiceUnavailable returns a 503 AppError.
func ErrServiceUnavailable(msg string, internal ...error) *AppError {
	var i error
	if len(internal) > 0 {
		i = internal[0]
	}
	return &AppError{Code: http.StatusServiceUnavailable, Message: msg, Internal: i}
}

// NewAppError returns an AppError with a custom status code.
func NewAppError(code int, msg string, internal error) *AppError {
	return &AppError{Code: code, Message: msg, Internal: internal}
}

// WrapError returns err as *AppError with the given status code.
// If err is already *AppError it is returned unchanged.
func WrapError(err error, code int) *AppError {
	if err == nil {
		return nil
	}
	if appErr, ok := err.(*AppError); ok {
		return appErr
	}
	return &AppError{Code: code, Message: err.Error(), Internal: err}
}
