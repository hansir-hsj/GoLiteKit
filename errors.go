package golitekit

import (
	"errors"
	"fmt"
	"net/http"
)

// AppError is an HTTP error. Code is the HTTP status; Status is an optional
// business code for the response "status" field (0 means use Code).
type AppError struct {
	Code     int    `json:"code"`
	Status   int    `json:"status,omitempty"`
	Message  string `json:"message"`
	Internal error  `json:"-"`
}

// WithStatus returns a copy of e with the business code set, e.g.
// ErrBadRequest("invalid phone", nil).WithStatus(10012).
func (e *AppError) WithStatus(status int) *AppError {
	cp := *e
	cp.Status = status
	return &cp
}

// ResponseStatus returns the value for the "status" field of the JSON
// response: the business code if set, otherwise the HTTP status code.
func (e *AppError) ResponseStatus() int {
	if e.Status != 0 {
		return e.Status
	}
	return e.Code
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
func ErrUnauthorized(msg string, internal error) *AppError {
	return &AppError{Code: http.StatusUnauthorized, Message: msg, Internal: internal}
}

// ErrForbidden returns a 403 AppError.
func ErrForbidden(msg string, internal error) *AppError {
	return &AppError{Code: http.StatusForbidden, Message: msg, Internal: internal}
}

// ErrNotFound returns a 404 AppError.
func ErrNotFound(msg string, internal error) *AppError {
	return &AppError{Code: http.StatusNotFound, Message: msg, Internal: internal}
}

// ErrMethodNotAllowed returns a 405 AppError.
func ErrMethodNotAllowed(msg string, internal error) *AppError {
	return &AppError{Code: http.StatusMethodNotAllowed, Message: msg, Internal: internal}
}

// ErrConflict returns a 409 AppError.
func ErrConflict(msg string, internal error) *AppError {
	return &AppError{Code: http.StatusConflict, Message: msg, Internal: internal}
}

// ErrTooManyRequests returns a 429 AppError.
func ErrTooManyRequests(msg string, internal error) *AppError {
	return &AppError{Code: http.StatusTooManyRequests, Message: msg, Internal: internal}
}

// ErrTimeout returns a 408 AppError.
func ErrTimeout(msg string, internal error) *AppError {
	return &AppError{Code: http.StatusRequestTimeout, Message: msg, Internal: internal}
}

// ErrInternal returns a 500 AppError.
func ErrInternal(msg string, internal error) *AppError {
	return &AppError{Code: http.StatusInternalServerError, Message: msg, Internal: internal}
}

// ErrServiceUnavailable returns a 503 AppError.
func ErrServiceUnavailable(msg string, internal error) *AppError {
	return &AppError{Code: http.StatusServiceUnavailable, Message: msg, Internal: internal}
}

// NewAppError returns an AppError with a custom status code.
func NewAppError(code int, msg string, internal error) *AppError {
	return &AppError{Code: code, Message: msg, Internal: internal}
}

// WrapError returns err as *AppError with the given code. A wrapped *AppError
// keeps its codes and message. 5xx messages are not shown to clients.
func WrapError(err error, code int) *AppError {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		if appErr == err {
			return appErr
		}
		return &AppError{Code: appErr.Code, Status: appErr.Status, Message: appErr.Message, Internal: err}
	}
	msg := err.Error()
	if code >= 500 {
		msg = http.StatusText(code)
	}
	return &AppError{Code: code, Message: msg, Internal: err}
}
