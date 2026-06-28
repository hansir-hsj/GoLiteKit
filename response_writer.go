package golitekit

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"sync"
)

const DefaultDeferredResponseBufferLimit = 1 << 20

type deferredResponseWriter struct {
	http.ResponseWriter
	buffer          bytes.Buffer
	header          http.Header
	statusCode      int
	bufferLimit     int
	isCommitted     bool
	isFlushed       bool // true once data has been committed to the real writer via Flush
	isHeaderWritten bool
	isHijacked      bool
	mu              sync.Mutex
}

func newDeferredResponseWriter(w http.ResponseWriter) *deferredResponseWriter {
	return &deferredResponseWriter{
		ResponseWriter: w,
		header:         make(http.Header),
		statusCode:     http.StatusOK,
		bufferLimit:    DefaultDeferredResponseBufferLimit,
	}
}

func (d *deferredResponseWriter) Header() http.Header {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isCommitted {
		return d.ResponseWriter.Header()
	}
	if d.isHijacked {
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
	if d.isHijacked {
		return 0, http.ErrHijacked
	}

	if d.bufferLimit > 0 && d.buffer.Len()+len(b) > d.bufferLimit {
		if err := d.commitLocked(); err != nil {
			return 0, err
		}
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
	if d.isHijacked {
		return
	}
	d.isHeaderWritten = true
	d.statusCode = code
}

// Commit writes cached response to the actual ResponseWriter.
func (d *deferredResponseWriter) Commit() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isCommitted {
		return nil
	}
	if d.isHijacked {
		return nil
	}
	return d.commitLocked()
}

func (d *deferredResponseWriter) commitLocked() error {
	d.isCommitted = true

	for k, v := range d.header {
		for _, vv := range v {
			d.ResponseWriter.Header().Add(k, vv)
		}
	}

	d.ResponseWriter.WriteHeader(d.statusCode)

	_, err := d.ResponseWriter.Write(d.buffer.Bytes())
	d.buffer.Reset()
	return err
}

func (d *deferredResponseWriter) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isFlushed {
		// Data already sent to the real writer; cannot undo.
		return
	}

	d.buffer.Reset()
	// Clear the header map in-place rather than allocating a new one.
	for k := range d.header {
		delete(d.header, k)
	}
	d.statusCode = http.StatusOK
	d.bufferLimit = DefaultDeferredResponseBufferLimit
	d.isCommitted = false
	d.isHeaderWritten = false
	d.isHijacked = false
}

func (d *deferredResponseWriter) Flush() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isHijacked {
		return
	}
	if !d.isFlushed {
		// First flush: commit buffered headers/body and switch to streaming pass-through.
		d.isFlushed = true
		d.isCommitted = true
		for k, v := range d.header {
			for _, vv := range v {
				d.ResponseWriter.Header().Add(k, vv)
			}
		}
		d.ResponseWriter.WriteHeader(d.statusCode)
		if d.buffer.Len() > 0 {
			_, _ = d.ResponseWriter.Write(d.buffer.Bytes())
			d.buffer.Reset()
		}
	}

	if f, ok := d.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (d *deferredResponseWriter) IsFlushed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.isFlushed
}

func (d *deferredResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := d.ResponseWriter.(http.Hijacker); ok {
		conn, rw, err := hj.Hijack()
		if err != nil {
			return nil, nil, err
		}
		d.mu.Lock()
		d.isHijacked = true
		d.buffer.Reset()
		d.mu.Unlock()
		return conn, rw, nil
	}

	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}

func (d *deferredResponseWriter) Unwrap() http.ResponseWriter {
	return d.ResponseWriter
}

type responseCapture struct {
	http.ResponseWriter
	body         []byte
	statusCode   int
	captureBody  bool
	maxBodyBytes int64
	mu           sync.Mutex
}

func newResponseCapture(w http.ResponseWriter, captureBody bool, maxBodyBytes int64) *responseCapture {
	return &responseCapture{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		captureBody:    captureBody,
		maxBodyBytes:   maxBodyBytes,
	}
}

func (r *responseCapture) Write(b []byte) (int, error) {
	if r.captureBody && r.maxBodyBytes > 0 {
		r.mu.Lock()
		remaining := int(r.maxBodyBytes) - len(r.body)
		if remaining > 0 {
			if len(b) < remaining {
				remaining = len(b)
			}
			r.body = append(r.body, b[:remaining]...)
		}
		r.mu.Unlock()
	}
	return r.ResponseWriter.Write(b)
}

func (r *responseCapture) WriteHeader(code int) {
	r.mu.Lock()
	r.statusCode = code
	r.mu.Unlock()
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

func (r *responseCapture) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
