package golitekit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorHandlerMiddleware_AppError(t *testing.T) {
	t.Run("handles AppError and returns JSON response", func(t *testing.T) {
		middleware := ErrorHandlerMiddleware()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithContext(r.Context())
			r = r.WithContext(ctx)
			SetError(ctx, ErrBadRequest("invalid input", nil))
		})

		wrapped := middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(WithContext(req.Context()))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		contentType := rec.Header().Get("Content-Type")
		if contentType != "application/json; charset=utf-8" {
			t.Errorf("Content-Type = %s, want application/json; charset=utf-8", contentType)
		}

		var resp Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.Status != http.StatusBadRequest {
			t.Errorf("response status = %d, want %d", resp.Status, http.StatusBadRequest)
		}
		if resp.Msg != "invalid input" {
			t.Errorf("response msg = %s, want invalid input", resp.Msg)
		}
	})

	t.Run("handles different error codes", func(t *testing.T) {
		testCases := []struct {
			name     string
			err      *AppError
			wantCode int
		}{
			{"BadRequest", ErrBadRequest("bad", nil), http.StatusBadRequest},
			{"Unauthorized", ErrUnauthorized("unauth"), http.StatusUnauthorized},
			{"Forbidden", ErrForbidden("forbidden"), http.StatusForbidden},
			{"NotFound", ErrNotFound("not found"), http.StatusNotFound},
			{"Conflict", ErrConflict("conflict"), http.StatusConflict},
			{"TooManyRequests", ErrTooManyRequests("rate limit"), http.StatusTooManyRequests},
			{"Internal", ErrInternal("internal", nil), http.StatusInternalServerError},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				middleware := ErrorHandlerMiddleware()

				handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					SetError(r.Context(), tc.err)
				})

				wrapped := middleware(handler)

				req := httptest.NewRequest("GET", "/test", nil)
				req = req.WithContext(WithContext(req.Context()))
				rec := httptest.NewRecorder()

				wrapped.ServeHTTP(rec, req)

				if rec.Code != tc.wantCode {
					t.Errorf("status = %d, want %d", rec.Code, tc.wantCode)
				}
			})
		}
	})
}

func TestErrorHandlerMiddleware_Panic(t *testing.T) {
	t.Run("recovers from panic and returns 500", func(t *testing.T) {
		middleware := ErrorHandlerMiddleware()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("something went wrong")
		})

		wrapped := middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(WithContext(req.Context()))
		rec := httptest.NewRecorder()

		// Should not panic
		wrapped.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}

		var resp Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.Msg != "Internal Server Error" {
			t.Errorf("response msg = %s, want Internal Server Error", resp.Msg)
		}
	})

	t.Run("calls panic callback", func(t *testing.T) {
		var panicValue any
		var panicRequest *http.Request

		middleware := ErrorHandlerMiddleware(
			WithPanicCallback(func(r *http.Request, recovered any) {
				panicValue = recovered
				panicRequest = r
			}),
		)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		})

		wrapped := middleware(handler)

		req := httptest.NewRequest("GET", "/callback-test", nil)
		req = req.WithContext(WithContext(req.Context()))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		if panicValue != "test panic" {
			t.Errorf("panic value = %v, want test panic", panicValue)
		}
		if panicRequest == nil {
			t.Error("expected request to be passed to callback")
		}
	})
}

func TestErrorHandlerMiddleware_NoError(t *testing.T) {
	t.Run("commits response when no error", func(t *testing.T) {
		middleware := ErrorHandlerMiddleware()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom", "value")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("created"))
		})

		wrapped := middleware(handler)

		req := httptest.NewRequest("POST", "/test", nil)
		req = req.WithContext(WithContext(req.Context()))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
		}
		if rec.Body.String() != "created" {
			t.Errorf("body = %s, want created", rec.Body.String())
		}
		if rec.Header().Get("X-Custom") != "value" {
			t.Error("expected custom header to be preserved")
		}
	})
}

func TestErrorHandlerMiddleware_CustomFormatter(t *testing.T) {
	t.Run("uses custom error formatter", func(t *testing.T) {
		customFormatter := func(w http.ResponseWriter, err *AppError, logID string) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(err.Code)
			w.Write([]byte("Custom: " + err.Message))
		}

		middleware := ErrorHandlerMiddleware(
			WithErrorFormatter(customFormatter),
		)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			SetError(r.Context(), ErrNotFound("item not found"))
		})

		wrapped := middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(WithContext(req.Context()))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		if rec.Header().Get("Content-Type") != "text/plain" {
			t.Error("expected custom Content-Type")
		}
		if rec.Body.String() != "Custom: item not found" {
			t.Errorf("body = %s, want Custom: item not found", rec.Body.String())
		}
	})
}

func TestErrorHandlerMiddleware_ErrorCallback(t *testing.T) {
	t.Run("calls error callback", func(t *testing.T) {
		var callbackErr *AppError
		var callbackReq *http.Request

		middleware := ErrorHandlerMiddleware(
			WithErrorCallback(func(r *http.Request, err *AppError) {
				callbackErr = err
				callbackReq = r
			}),
		)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			SetError(r.Context(), ErrBadRequest("test error", nil))
		})

		wrapped := middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(WithContext(req.Context()))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		if callbackErr == nil {
			t.Fatal("expected error callback to be called")
		}
		if callbackErr.Message != "test error" {
			t.Errorf("callback error msg = %s, want test error", callbackErr.Message)
		}
		if callbackReq == nil {
			t.Error("expected request to be passed to callback")
		}
	})
}
