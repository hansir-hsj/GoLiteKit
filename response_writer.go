package golitekit

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"sync"
)

type timeoutResponseWriter struct {
	http.ResponseWriter
	isTimeout       bool
	isHeaderWritten bool
	statusCode      int
	mu              sync.Mutex
}

func newTimeoutResponseWriter(w http.ResponseWriter) *timeoutResponseWriter {
	return &timeoutResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (tw *timeoutResponseWriter) Write(b []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if tw.isTimeout {
		return 0, http.ErrHandlerTimeout
	}

	if !tw.isHeaderWritten {
		tw.isHeaderWritten = true
		tw.statusCode = http.StatusOK
		tw.ResponseWriter.WriteHeader(tw.statusCode)
	}

	return tw.ResponseWriter.Write(b)
}

func (tw *timeoutResponseWriter) WriteHeader(code int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if tw.isTimeout {
		return
	}

	if tw.isHeaderWritten {
		return
	}

	tw.isHeaderWritten = true
	tw.statusCode = code
	tw.ResponseWriter.WriteHeader(tw.statusCode)
}

func (tw *timeoutResponseWriter) markTimeout() {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	tw.isTimeout = true
}

func (tw *timeoutResponseWriter) Header() http.Header {
	return tw.ResponseWriter.Header()
}

func (tw *timeoutResponseWriter) Flush() {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if tw.isTimeout {
		return
	}

	if f, ok := tw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (tw *timeoutResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := tw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}

	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}

type deferredResponseWriter struct {
	http.ResponseWriter
	buffer          bytes.Buffer
	header          http.Header
	statusCode      int
	isCommitted     bool
	isHeaderWritten bool
	mu              sync.Mutex
}

func newDeferredResponseWriter(w http.ResponseWriter) *deferredResponseWriter {
	return &deferredResponseWriter{
		ResponseWriter: w,
		header:         make(http.Header),
		statusCode:     http.StatusOK,
	}
}

func (d *deferredResponseWriter) Header() http.Header {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isCommitted {
		return d.ResponseWriter.Header()
	}

	return d.header
}

func (d *deferredResponseWriter) Write(b []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isCommitted {
		return d.ResponseWriter.Write(b)
	}

	return d.buffer.Write(b)
}

func (d *deferredResponseWriter) WriteHeader(code int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isHeaderWritten {
		return
	}
	d.isHeaderWritten = true
	d.statusCode = code
}

// commit all cached responses to the actual ResponseWriter
func (d *deferredResponseWriter) Commit() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isCommitted {
		return nil
	}
	d.isCommitted = true

	for k, v := range d.header {
		for _, vv := range v {
			d.ResponseWriter.Header().Add(k, vv)
		}
	}

	d.ResponseWriter.WriteHeader(d.statusCode)

	_, err := d.ResponseWriter.Write(d.buffer.Bytes())
	return err
}

func (d *deferredResponseWriter) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.buffer.Reset()
	d.header = make(http.Header)
	d.statusCode = http.StatusOK
	d.isCommitted = false
	d.isHeaderWritten = false
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

func (d *deferredResponseWriter) Flush() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isCommitted {
		return
	}

	if f, ok := d.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (d *deferredResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := d.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}

	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}

type responseCapture struct {
	http.ResponseWriter
	body       []byte
	statusCode int
}

func newResponseCapture(w http.ResponseWriter) *responseCapture {
	return &responseCapture{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (r *responseCapture) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return r.ResponseWriter.Write(b)
}

func (r *responseCapture) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseCapture) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *responseCapture) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}

	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}
