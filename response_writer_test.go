package golitekit

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	writeDeadline time.Time
}

func (r *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	r.writeDeadline = deadline
	return nil
}

type hijackRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (r *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	r.hijacked = true
	server, client := net.Pipe()
	_ = client.Close()
	return server, bufio.NewReadWriter(bufio.NewReader(server), bufio.NewWriter(server)), nil
}

func (d *deferredResponseWriter) Buffer() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.buffer.Bytes()
}

func (d *deferredResponseWriter) StatusCode() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.statusCode
}

func (d *deferredResponseWriter) IsCommitted() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.isCommitted
}

func (d *deferredResponseWriter) IsHijacked() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.isHijacked
}

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

	t.Run("commits and passes through when buffer limit exceeded", func(t *testing.T) {
		rec := httptest.NewRecorder()
		dw := newDeferredResponseWriter(rec)

		large := make([]byte, DefaultDeferredResponseBufferLimit+1)
		for i := range large {
			large[i] = 'a'
		}

		n, err := dw.Write(large)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n != len(large) {
			t.Fatalf("bytes written = %d, want %d", n, len(large))
		}
		if !dw.IsCommitted() {
			t.Fatal("writer should commit after buffer limit is exceeded")
		}
		if rec.Body.Len() != len(large) {
			t.Fatalf("body length = %d, want %d", rec.Body.Len(), len(large))
		}
		if len(dw.Buffer()) != 0 {
			t.Fatalf("buffer length = %d, want 0 after pass-through", len(dw.Buffer()))
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

// ============================================================================
// Flush / SSE streaming
// ============================================================================

func TestDeferredResponseWriter_Flush_CommitsOnFirstCall(t *testing.T) {
	// First Flush must commit buffered headers and body to the real writer.
	rec := httptest.NewRecorder()
	dw := newDeferredResponseWriter(rec)

	dw.Header().Set("Content-Type", "text/event-stream")
	dw.WriteHeader(http.StatusOK)
	dw.Write([]byte("data: hello\n\n"))

	dw.Flush()

	if !dw.IsFlushed() {
		t.Error("IsFlushed should be true after Flush()")
	}
	if !dw.IsCommitted() {
		t.Error("IsCommitted should be true after Flush()")
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "data: hello") {
		t.Errorf("body = %q, expected SSE data", rec.Body.String())
	}
}

func TestDeferredResponseWriter_ResponseControllerFlush(t *testing.T) {
	fr := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	dw := newDeferredResponseWriter(fr)

	if err := http.NewResponseController(dw).Flush(); err != nil {
		t.Fatalf("ResponseController.Flush: %v", err)
	}
	if fr.flushCount == 0 {
		t.Fatal("expected flush to reach underlying writer")
	}
}

func TestDeferredResponseWriter_ResponseControllerSetWriteDeadline(t *testing.T) {
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	dw := newDeferredResponseWriter(rec)
	deadline := time.Now().Add(time.Second)

	if err := http.NewResponseController(dw).SetWriteDeadline(deadline); err != nil {
		t.Fatalf("ResponseController.SetWriteDeadline: %v", err)
	}
	if !rec.writeDeadline.Equal(deadline) {
		t.Fatalf("write deadline = %v, want %v", rec.writeDeadline, deadline)
	}
}

func TestDeferredResponseWriter_CommitSkippedAfterHijack(t *testing.T) {
	rec := &hijackRecorder{ResponseRecorder: httptest.NewRecorder()}
	dw := newDeferredResponseWriter(rec)
	dw.WriteHeader(http.StatusCreated)
	dw.Write([]byte("buffered"))

	conn, _, err := dw.Hijack()
	if err != nil {
		t.Fatalf("Hijack: %v", err)
	}
	_ = conn.Close()

	if !rec.hijacked {
		t.Fatal("expected underlying writer to be hijacked")
	}
	if !dw.IsHijacked() {
		t.Fatal("expected deferred writer to record hijacked state")
	}
	if err := dw.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want untouched recorder status %d", rec.Code, http.StatusOK)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want no commit after hijack", rec.Body.String())
	}
}

func TestDeferredResponseWriter_Flush_PassThrough(t *testing.T) {
	// Writes after the first Flush go directly to the real writer (no buffering).
	rec := httptest.NewRecorder()
	dw := newDeferredResponseWriter(rec)

	dw.Flush() // commit empty buffer

	dw.Write([]byte("streaming chunk"))

	if !strings.Contains(rec.Body.String(), "streaming chunk") {
		t.Errorf("body = %q, expected pass-through write", rec.Body.String())
	}
}

func TestDeferredResponseWriter_Reset_IgnoredAfterFlush(t *testing.T) {
	// Reset must be a no-op once data has been flushed to the real writer.
	rec := httptest.NewRecorder()
	dw := newDeferredResponseWriter(rec)

	dw.Write([]byte("sent"))
	dw.Flush()

	// Reset should not clear flushed state.
	dw.Reset()

	if !dw.IsFlushed() {
		t.Error("IsFlushed should remain true after attempted Reset")
	}
}

// ============================================================================

func TestResponseCapture(t *testing.T) {
	t.Run("captures response body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rc := newResponseCapture(rec, true, DefaultLogBodyLimit)

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
		rc := newResponseCapture(rec, true, DefaultLogBodyLimit)

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
		rc := newResponseCapture(rec, true, DefaultLogBodyLimit)

		if rc.statusCode != http.StatusOK {
			t.Errorf("default status = %d, want %d", rc.statusCode, http.StatusOK)
		}
	})

	t.Run("captures response body up to limit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rc := newResponseCapture(rec, true, 4)

		rc.Write([]byte("hello"))
		rc.Write([]byte(" world"))

		if string(rc.body) != "hell" {
			t.Errorf("captured body = %q, want hell", string(rc.body))
		}
		if rec.Body.String() != "hello world" {
			t.Errorf("original body = %q, want full response", rec.Body.String())
		}
	})

	t.Run("does not capture response body when disabled", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rc := newResponseCapture(rec, false, DefaultLogBodyLimit)

		rc.Write([]byte("hello"))

		if len(rc.body) != 0 {
			t.Errorf("captured body length = %d, want 0", len(rc.body))
		}
		if rec.Body.String() != "hello" {
			t.Errorf("original body = %q, want hello", rec.Body.String())
		}
	})
}

func TestResponseCapture_ResponseControllerFlush(t *testing.T) {
	fr := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	rc := newResponseCapture(fr, false, DefaultLogBodyLimit)

	if err := http.NewResponseController(rc).Flush(); err != nil {
		t.Fatalf("ResponseController.Flush: %v", err)
	}
	if fr.flushCount == 0 {
		t.Fatal("expected flush to reach underlying writer")
	}
}

func TestResponseCapture_ResponseControllerSetWriteDeadline(t *testing.T) {
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	rc := newResponseCapture(rec, false, DefaultLogBodyLimit)
	deadline := time.Now().Add(time.Second)

	if err := http.NewResponseController(rc).SetWriteDeadline(deadline); err != nil {
		t.Fatalf("ResponseController.SetWriteDeadline: %v", err)
	}
	if !rec.writeDeadline.Equal(deadline) {
		t.Fatalf("write deadline = %v, want %v", rec.writeDeadline, deadline)
	}
}
