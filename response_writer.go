package golitekit

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// DefaultDeferredResponseBufferLimit is kept for compatibility.
//
// Deprecated: response bodies are no longer buffered, so there is no limit.
const DefaultDeferredResponseBufferLimit = 1 << 20

// deferredResponseWriter holds the status and headers until the first Write
// or Flush, so Reset can still replace the response with an error. The body
// is not buffered. Pending headers start as a copy of the real ones.
type deferredResponseWriter struct {
	http.ResponseWriter
	header          http.Header
	statusCode      int
	isCommitted     bool // headers sent to the real writer
	isHeaderWritten bool
	isHijacked      bool
	mu              sync.Mutex
}

func newDeferredResponseWriter(w http.ResponseWriter) *deferredResponseWriter {
	return &deferredResponseWriter{
		ResponseWriter: w,
		header:         cloneHeader(w.Header()),
		statusCode:     http.StatusOK,
	}
}

func cloneHeader(h http.Header) http.Header {
	if c := h.Clone(); c != nil {
		return c
	}
	return make(http.Header)
}

// replaceHeader makes dst hold exactly the entries of src.
func replaceHeader(dst, src http.Header) {
	clear(dst)
	for k, v := range src {
		dst[k] = v
	}
}

func (d *deferredResponseWriter) Header() http.Header {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isCommitted || d.isHijacked {
		return d.ResponseWriter.Header()
	}
	return d.header
}

func (d *deferredResponseWriter) Write(b []byte) (int, error) {
	d.mu.Lock()
	if d.isHijacked {
		d.mu.Unlock()
		return 0, http.ErrHijacked
	}
	d.commitLocked()
	d.mu.Unlock()

	return d.ResponseWriter.Write(b)
}

func (d *deferredResponseWriter) WriteHeader(code int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isHijacked || d.isCommitted {
		return
	}
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		// Informational responses (e.g. 103 Early Hints) are sent right away
		// with the headers set so far. The real headers are restored
		// afterwards, so the pending ones stay discardable by Reset.
		saved := d.ResponseWriter.Header().Clone()
		replaceHeader(d.ResponseWriter.Header(), d.header)
		d.ResponseWriter.WriteHeader(code)
		replaceHeader(d.ResponseWriter.Header(), saved)
		return
	}
	if d.isHeaderWritten {
		return
	}
	d.isHeaderWritten = true
	d.statusCode = code
}

// Commit sends the pending status and headers if nothing has been sent yet.
func (d *deferredResponseWriter) Commit() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isHijacked {
		d.commitLocked()
	}
	return nil
}

func (d *deferredResponseWriter) commitLocked() {
	if d.isCommitted {
		return
	}
	d.isCommitted = true
	replaceHeader(d.ResponseWriter.Header(), d.header)
	d.ResponseWriter.WriteHeader(d.statusCode)
}

// Reset discards the pending status and headers set by the handler, going
// back to the headers of the real writer. It is a no-op once the response has
// been committed or the connection hijacked.
func (d *deferredResponseWriter) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isCommitted || d.isHijacked {
		return
	}
	d.header = cloneHeader(d.ResponseWriter.Header())
	d.statusCode = http.StatusOK
	d.isHeaderWritten = false
}

func (d *deferredResponseWriter) Flush() {
	d.mu.Lock()
	if d.isHijacked {
		d.mu.Unlock()
		return
	}
	d.commitLocked()
	d.mu.Unlock()

	if f, ok := d.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// responseSent reports whether the status line has reached the real writer
// (first Write, Flush, or Commit) or the connection was hijacked, after which
// the response can no longer be replaced.
func (d *deferredResponseWriter) responseSent() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.isCommitted || d.isHijacked
}

func (d *deferredResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := d.ResponseWriter.(http.Hijacker); ok {
		conn, rw, err := hj.Hijack()
		if err != nil {
			return nil, nil, err
		}
		d.mu.Lock()
		d.isHijacked = true
		d.mu.Unlock()
		return conn, rw, nil
	}

	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}

func (d *deferredResponseWriter) Unwrap() http.ResponseWriter {
	return d.ResponseWriter
}

// logCaptureLimit is the minimum number of response bytes captured for
// logging. JSON bodies must be captured whole to be redacted, so capture more
// than the logged size and truncate after redaction.
const logCaptureLimit = 64 << 10

type responseCapture struct {
	http.ResponseWriter
	body         []byte
	total        int64 // bytes written, including ones not captured
	statusCode   int
	wroteHeader  bool
	captureBody  bool
	captureLimit int64
	mu           sync.Mutex
}

func newResponseCapture(w http.ResponseWriter, captureBody bool, maxBodyBytes int64) *responseCapture {
	return &responseCapture{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		captureBody:    captureBody,
		captureLimit:   max(maxBodyBytes, logCaptureLimit),
	}
}

func (r *responseCapture) Write(b []byte) (int, error) {
	r.mu.Lock()
	r.wroteHeader = true
	if r.captureBody {
		r.total += int64(len(b))
		if remaining := r.captureLimit - int64(len(r.body)); remaining > 0 {
			r.body = append(r.body, b[:min(int64(len(b)), remaining)]...)
		}
	}
	r.mu.Unlock()
	return r.ResponseWriter.Write(b)
}

func (r *responseCapture) WriteHeader(code int) {
	r.mu.Lock()
	// 1xx responses (except 101) are forwarded without fixing the status;
	// the final status follows.
	if !r.wroteHeader && (code >= 200 || code == http.StatusSwitchingProtocols) {
		r.statusCode = code
		r.wroteHeader = true
	}
	r.mu.Unlock()
	r.ResponseWriter.WriteHeader(code)
}

// status returns the status code sent so far (200 if nothing was written).
func (r *responseCapture) status() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusCode
}

func (r *responseCapture) headerWritten() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.wroteHeader
}

// capturedBody returns the captured prefix of the body and the total size written.
func (r *responseCapture) capturedBody() ([]byte, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body, r.total
}

func (r *responseCapture) Flush() {
	// Flushing sends the status line (200 if none was set).
	r.mu.Lock()
	r.wroteHeader = true
	r.mu.Unlock()
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
