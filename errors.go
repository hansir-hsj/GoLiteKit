package golitekit

import (
	"fmt"
	"net/http"
)

type AppError struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	Internal error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Internal)
	}
	return e.Message
}

func ErrBadRequest(msg string, internal error) *AppError {
	return &AppError{
		Code:     http.StatusBadRequest,
		Message:  msg,
		Internal: internal,
	}
}

func ErrUnauthorized(msg string) *AppError {
	return &AppError{
		Code:    http.StatusUnauthorized,
		Message: msg,
	}
}

func ErrForbidden(msg string) *AppError {
	return &AppError{
		Code:    http.StatusForbidden,
		Message: msg,
	}
}

func ErrNotFound(msg string) *AppError {
	return &AppError{
		Code:    http.StatusNotFound,
		Message: msg,
	}
}

func ErrMethodNotAllowed(msg string) *AppError {
	return &AppError{
		Code:    http.StatusMethodNotAllowed,
		Message: msg,
	}
}

func ErrConflict(msg string) *AppError {
	return &AppError{
		Code:    http.StatusConflict,
		Message: msg,
	}
}

func ErrTooManyRequests(msg string) *AppError {
	return &AppError{
		Code:    http.StatusTooManyRequests,
		Message: msg,
	}
}

func ErrTimeout(msg string) *AppError {
	return &AppError{
		Code:    http.StatusRequestTimeout,
		Message: msg,
	}
}

func ErrInternal(msg string, internal error) *AppError {
	return &AppError{
		Code:     http.StatusInternalServerError,
		Message:  msg,
		Internal: internal,
	}
}

func ErrServiceUnavailable(msg string) *AppError {
	return &AppError{
		Code:    http.StatusServiceUnavailable,
		Message: msg,
	}
}

func NewAppError(code int, msg string, internal error) *AppError {
	return &AppError{
		Code:     code,
		Message:  msg,
		Internal: internal,
	}
}
