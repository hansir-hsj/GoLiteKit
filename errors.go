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

// ErrBadRequest returns a 400 AppError.
func ErrBadRequest(msg string, internal error) *AppError {
	return &AppError{
		Code:     http.StatusBadRequest,
		Message:  msg,
		Internal: internal,
	}
}

// ErrUnauthorized returns a 401 AppError.
func ErrUnauthorized(msg string) *AppError {
	return &AppError{
		Code:    http.StatusUnauthorized,
		Message: msg,
	}
}

// ErrForbidden returns a 403 AppError.
func ErrForbidden(msg string) *AppError {
	return &AppError{
		Code:    http.StatusForbidden,
		Message: msg,
	}
}

// ErrNotFound returns a 404 AppError.
func ErrNotFound(msg string) *AppError {
	return &AppError{
		Code:    http.StatusNotFound,
		Message: msg,
	}
}

// ErrMethodNotAllowed returns a 405 AppError.
func ErrMethodNotAllowed(msg string) *AppError {
	return &AppError{
		Code:    http.StatusMethodNotAllowed,
		Message: msg,
	}
}

// ErrConflict returns a 409 AppError.
func ErrConflict(msg string) *AppError {
	return &AppError{
		Code:    http.StatusConflict,
		Message: msg,
	}
}

// ErrTooManyRequests returns a 429 AppError.
func ErrTooManyRequests(msg string) *AppError {
	return &AppError{
		Code:    http.StatusTooManyRequests,
		Message: msg,
	}
}

// ErrTimeout returns a 408 AppError.
func ErrTimeout(msg string) *AppError {
	return &AppError{
		Code:    http.StatusRequestTimeout,
		Message: msg,
	}
}

// ErrInternal returns a 500 AppError with an optional internal cause.
func ErrInternal(msg string, internal error) *AppError {
	return &AppError{
		Code:     http.StatusInternalServerError,
		Message:  msg,
		Internal: internal,
	}
}

// ErrServiceUnavailable returns a 503 AppError.
func ErrServiceUnavailable(msg string) *AppError {
	return &AppError{
		Code:    http.StatusServiceUnavailable,
		Message: msg,
	}
}

// NewAppError returns an AppError with a custom status code.
func NewAppError(code int, msg string, internal error) *AppError {
	return &AppError{
		Code:     code,
		Message:  msg,
		Internal: internal,
	}
}
