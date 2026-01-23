package golitekit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeferredResponseWriter_Write(t *testing.T) {
	t.Run("buffers write before commit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		dw := newDeferredResponseWriter(rec)

		n, err := dw.Write([]byte("hello"))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n != 5 {
			t.Errorf("bytes written = %d, want 5", n)
		}

		// Original writer should not have received data yet
		if rec.Body.Len() > 0 {
			t.Error("data should be buffered, not written to original")
		}

		// Buffer should contain data
		if string(dw.Buffer()) != "hello" {
			t.Errorf("buffer = %s, want hello", string(dw.Buffer()))
		}
	})

	t.Run("writes directly after commit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		dw := newDeferredResponseWriter(rec)

		dw.Commit()

		n, err := dw.Write([]byte("direct"))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n != 6 {
			t.Errorf("bytes written = %d, want 6", n)
		}

		if rec.Body.String() != "direct" {
			t.Errorf("body = %s, want direct", rec.Body.String())
		}
	})
}

func TestDeferredResponseWriter_WriteHeader(t *testing.T) {
	t.Run("stores status code before commit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		dw := newDeferredResponseWriter(rec)

		dw.WriteHeader(http.StatusCreated)

		if dw.StatusCode() != http.StatusCreated {
			t.Errorf("status = %d, want %d", dw.StatusCode(), http.StatusCreated)
		}

		// Original writer should not have received status yet
		if rec.Code != http.StatusOK {
			t.Error("status should be buffered, not written to original")
		}
	})

	t.Run("ignores duplicate WriteHeader calls", func(t *testing.T) {
		rec := httptest.NewRecorder()
		dw := newDeferredResponseWriter(rec)

		dw.WriteHeader(http.StatusCreated)
		dw.WriteHeader(http.StatusNotFound) // Should be ignored

		if dw.StatusCode() != http.StatusCreated {
			t.Errorf("status = %d, want %d", dw.StatusCode(), http.StatusCreated)
		}
	})
}

func TestDeferredResponseWriter_Header(t *testing.T) {
	t.Run("returns buffered header before commit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		dw := newDeferredResponseWriter(rec)

		dw.Header().Set("X-Custom", "value")

		if dw.Header().Get("X-Custom") != "value" {
			t.Error("expected header to be set in buffer")
		}

		// Original should not have header yet
		if rec.Header().Get("X-Custom") != "" {
			t.Error("header should be buffered, not written to original")
		}
	})

	t.Run("returns original header after commit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		dw := newDeferredResponseWriter(rec)

		dw.Commit()

		header := dw.Header()
		header.Set("X-After", "commit")

		if rec.Header().Get("X-After") != "commit" {
			t.Error("expected header to be written to original after commit")
		}
	})
}

func TestDeferredResponseWriter_Commit(t *testing.T) {
	t.Run("writes buffered data to original", func(t *testing.T) {
		rec := httptest.NewRecorder()
		dw := newDeferredResponseWriter(rec)

		dw.Header().Set("X-Custom", "value")
		dw.WriteHeader(http.StatusCreated)
		dw.Write([]byte("buffered content"))

		err := dw.Commit()
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		if rec.Code != http.StatusCreated {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
		}
		if rec.Body.String() != "buffered content" {
			t.Errorf("body = %s, want buffered content", rec.Body.String())
		}
		if rec.Header().Get("X-Custom") != "value" {
			t.Error("expected custom header to be committed")
		}
	})

	t.Run("does nothing on second commit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		dw := newDeferredResponseWriter(rec)

		dw.Write([]byte("first"))
		dw.Commit()

		// Write after commit
		dw.Write([]byte("-second"))

		// Second commit should be no-op
		dw.Commit()

		if rec.Body.String() != "first-second" {
			t.Errorf("body = %s, want first-second", rec.Body.String())
		}
	})

	t.Run("IsCommitted returns correct state", func(t *testing.T) {
		rec := httptest.NewRecorder()
		dw := newDeferredResponseWriter(rec)

		if dw.IsCommitted() {
			t.Error("should not be committed initially")
		}

		dw.Commit()

		if !dw.IsCommitted() {
			t.Error("should be committed after Commit()")
		}
	})
}

func TestDeferredResponseWriter_Reset(t *testing.T) {
	t.Run("clears buffered data", func(t *testing.T) {
		rec := httptest.NewRecorder()
		dw := newDeferredResponseWriter(rec)

		dw.Header().Set("X-Custom", "value")
		dw.WriteHeader(http.StatusCreated)
		dw.Write([]byte("data"))

		dw.Reset()

		if len(dw.Buffer()) > 0 {
			t.Error("buffer should be empty after reset")
		}
		if dw.StatusCode() != http.StatusOK {
			t.Errorf("status should be reset to 200, got %d", dw.StatusCode())
		}
		if dw.Header().Get("X-Custom") != "" {
			t.Error("headers should be cleared after reset")
		}
		if dw.IsCommitted() {
			t.Error("should not be committed after reset")
		}
	})
}

func TestResponseCapture(t *testing.T) {
	t.Run("captures response body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rc := newResponseCapture(rec)

		rc.Write([]byte("hello"))
		rc.Write([]byte(" world"))

		if string(rc.body) != "hello world" {
			t.Errorf("captured body = %s, want hello world", string(rc.body))
		}

		// Should also write to original
		if rec.Body.String() != "hello world" {
			t.Errorf("original body = %s, want hello world", rec.Body.String())
		}
	})

	t.Run("captures status code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rc := newResponseCapture(rec)

		rc.WriteHeader(http.StatusNotFound)

		if rc.statusCode != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rc.statusCode, http.StatusNotFound)
		}

		// Should also write to original
		if rec.Code != http.StatusNotFound {
			t.Errorf("original status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("default status is 200", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rc := newResponseCapture(rec)

		if rc.statusCode != http.StatusOK {
			t.Errorf("default status = %d, want %d", rc.statusCode, http.StatusOK)
		}
	})
}