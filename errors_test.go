package golitekit

import (
	"errors"
	"net/http"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	t.Run("returns message without internal error", func(t *testing.T) {
		err := &AppError{
			Code:    400,
			Message: "bad request",
		}

		if err.Error() != "bad request" {
			t.Errorf("Error() = %s, want bad request", err.Error())
		}
	})

	t.Run("includes internal error in message", func(t *testing.T) {
		internal := errors.New("database connection failed")
		err := &AppError{
			Code:     500,
			Message:  "internal error",
			Internal: internal,
		}

		expected := "internal error: database connection failed"
		if err.Error() != expected {
			t.Errorf("Error() = %s, want %s", err.Error(), expected)
		}
	})
}

func TestErrBadRequest(t *testing.T) {
	internal := errors.New("validation failed")
	err := ErrBadRequest("invalid input", internal)

	if err.Code != http.StatusBadRequest {
		t.Errorf("Code = %d, want %d", err.Code, http.StatusBadRequest)
	}
	if err.Message != "invalid input" {
		t.Errorf("Message = %s, want invalid input", err.Message)
	}
	if err.Internal != internal {
		t.Error("Internal error not set correctly")
	}
}

func TestErrUnauthorized(t *testing.T) {
	err := ErrUnauthorized("please login")

	if err.Code != http.StatusUnauthorized {
		t.Errorf("Code = %d, want %d", err.Code, http.StatusUnauthorized)
	}
	if err.Message != "please login" {
		t.Errorf("Message = %s, want please login", err.Message)
	}
}

func TestErrForbidden(t *testing.T) {
	err := ErrForbidden("access denied")

	if err.Code != http.StatusForbidden {
		t.Errorf("Code = %d, want %d", err.Code, http.StatusForbidden)
	}
	if err.Message != "access denied" {
		t.Errorf("Message = %s, want access denied", err.Message)
	}
}

func TestErrNotFound(t *testing.T) {
	err := ErrNotFound("user not found")

	if err.Code != http.StatusNotFound {
		t.Errorf("Code = %d, want %d", err.Code, http.StatusNotFound)
	}
	if err.Message != "user not found" {
		t.Errorf("Message = %s, want user not found", err.Message)
	}
}

func TestErrMethodNotAllowed(t *testing.T) {
	err := ErrMethodNotAllowed("GET not allowed")

	if err.Code != http.StatusMethodNotAllowed {
		t.Errorf("Code = %d, want %d", err.Code, http.StatusMethodNotAllowed)
	}
}

func TestErrConflict(t *testing.T) {
	err := ErrConflict("resource exists")

	if err.Code != http.StatusConflict {
		t.Errorf("Code = %d, want %d", err.Code, http.StatusConflict)
	}
}

func TestErrTooManyRequests(t *testing.T) {
	err := ErrTooManyRequests("rate limited")

	if err.Code != http.StatusTooManyRequests {
		t.Errorf("Code = %d, want %d", err.Code, http.StatusTooManyRequests)
	}
}

func TestErrTimeout(t *testing.T) {
	err := ErrTimeout("request timeout")

	if err.Code != http.StatusRequestTimeout {
		t.Errorf("Code = %d, want %d", err.Code, http.StatusRequestTimeout)
	}
}

func TestErrInternal(t *testing.T) {
	internal := errors.New("db error")
	err := ErrInternal("something went wrong", internal)

	if err.Code != http.StatusInternalServerError {
		t.Errorf("Code = %d, want %d", err.Code, http.StatusInternalServerError)
	}
	if err.Internal != internal {
		t.Error("Internal error not set correctly")
	}
}

func TestErrServiceUnavailable(t *testing.T) {
	err := ErrServiceUnavailable("service down")

	if err.Code != http.StatusServiceUnavailable {
		t.Errorf("Code = %d, want %d", err.Code, http.StatusServiceUnavailable)
	}
}

func TestNewAppError(t *testing.T) {
	internal := errors.New("custom error")
	err := NewAppError(418, "I'm a teapot", internal)

	if err.Code != 418 {
		t.Errorf("Code = %d, want 418", err.Code)
	}
	if err.Message != "I'm a teapot" {
		t.Errorf("Message = %s, want I'm a teapot", err.Message)
	}
	if err.Internal != internal {
		t.Error("Internal error not set correctly")
	}
}
