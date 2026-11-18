package golitekit

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

type logEntry struct {
	level  string
	msg    string
	fields map[string]any
}

// recordLogger records every log call with the request's context fields.
type recordLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

func (l *recordLogger) record(ctx context.Context, level, msg string) {
	fields := map[string]any{}
	if logCtx := logger.GetLoggerContext(ctx); logCtx != nil {
		for f := logCtx.Head; f != nil; f = f.Next {
			fields[f.Key] = f.Value
		}
	}
	l.mu.Lock()
	l.entries = append(l.entries, logEntry{level: level, msg: msg, fields: fields})
	l.mu.Unlock()
}

func (l *recordLogger) Debug(ctx context.Context, msg string, args ...any) {
	l.record(ctx, "DEBUG", msg)
}
func (l *recordLogger) Trace(ctx context.Context, msg string, args ...any) {
	l.record(ctx, "TRACE", msg)
}
func (l *recordLogger) Info(ctx context.Context, msg string, args ...any) { l.record(ctx, "INFO", msg) }
func (l *recordLogger) Warning(ctx context.Context, msg string, args ...any) {
	l.record(ctx, "WARN", msg)
}
func (l *recordLogger) Error(ctx context.Context, msg string, args ...any) {
	l.record(ctx, "ERROR", msg)
}
func (l *recordLogger) Fatal(ctx context.Context, msg string, args ...any) {
	l.record(ctx, "FATAL", msg)
}
func (l *recordLogger) Close() error { return nil }

func (l *recordLogger) only(t *testing.T) logEntry {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) != 1 {
		t.Fatalf("log entries = %+v, want exactly 1", l.entries)
	}
	return l.entries[0]
}

func newTestPanicLogger(t *testing.T) (*logger.PanicLogger, string) {
	t.Helper()
	dir := t.TempDir()
	conf := filepath.Join(dir, "logger.toml")
	content := fmt.Sprintf("[logger]\ndir = %q\nrotateRule = \"no\"\n", dir)
	if err := os.WriteFile(conf, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	pl, err := logger.NewPanicLogger(conf)
	if err != nil {
		t.Fatalf("NewPanicLogger: %v", err)
	}
	t.Cleanup(func() { pl.Close() })
	return pl, filepath.Join(dir, "panic.log")
}

func TestDefaultStackLogsPanicOnceWithFinalStatus(t *testing.T) {
	logs := &recordLogger{}
	pl, panicPath := newTestPanicLogger(t)
	app := NewApp(WithLogger(logs), WithPanicLogger(pl))
	app.GET("/panic", func(ctx *Context) error { panic("boom") })

	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	entry := logs.only(t)
	if entry.level != "ERROR" || entry.fields["status"] != http.StatusInternalServerError {
		t.Fatalf("log entry = %+v, want ERROR with status 500", entry)
	}
	if !strings.Contains(fmt.Sprint(entry.fields["err_internal"]), "panic: boom") {
		t.Fatalf("err_internal = %v, want panic value", entry.fields["err_internal"])
	}
	content, err := os.ReadFile(panicPath)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(content), "Recover from panic"); n != 1 {
		t.Fatalf("panic reported %d times, want 1", n)
	}
}

func TestDefaultStackLogsHandledErrorWithFields(t *testing.T) {
	logs := &recordLogger{}
	app := NewApp(WithLogger(logs))
	app.GET("/bad", func(ctx *Context) error {
		return fmt.Errorf("lookup: %w", ErrNotFound("user not found", nil))
	})

	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/bad", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 from wrapped AppError", rec.Code)
	}
	entry := logs.only(t)
	if entry.level != "WARN" {
		t.Fatalf("level = %s, want WARN", entry.level)
	}
	for key, want := range map[string]any{"status": http.StatusNotFound, "err_code": http.StatusNotFound, "method": http.MethodGet} {
		if entry.fields[key] != want {
			t.Fatalf("field %s = %v, want %v (fields %+v)", key, entry.fields[key], want, entry.fields)
		}
	}
	if entry.fields["logid"] == nil {
		t.Fatal("logid field missing")
	}
}

func TestNewAppFromConfigAllowsUseWithPprofEnabled(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "app.toml")
	if err := os.WriteFile(conf, []byte("[HttpServer]\nenablePprof = true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	app, err := NewAppFromConfig(conf)
	if err != nil {
		t.Fatalf("NewAppFromConfig: %v", err)
	}

	used := false
	app.Use(func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			used = true
			return next(ctx, w, r)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/cmdline", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("pprof status = %d, want 200", rec.Code)
	}
	if !used {
		t.Fatal("middleware registered after NewAppFromConfig did not run on pprof route")
	}
}

func TestRequestParseErrors(t *testing.T) {
	type body struct {
		Age int `json:"age"`
	}
	r := newTestRouter()
	r.POST("/p", &struct {
		BaseControllerOf[body]
	}{})

	cases := []struct {
		body string
		code int
		msg  string
	}{
		{`{"age":`, http.StatusBadRequest, "Invalid JSON body"},
		{`{"age":"x"}`, http.StatusBadRequest, `Invalid type for field "age"`},
		{`{"age":1,"pad":"` + strings.Repeat("x", DefaultMaxBodySize) + `"}`, http.StatusRequestEntityTooLarge, "Request body too large"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/p", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)
		var resp Response
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if rec.Code != tc.code || resp.Msg != tc.msg {
			t.Errorf("body %.20q: got %d %q, want %d %q", tc.body, rec.Code, resp.Msg, tc.code, tc.msg)
		}
	}
}

func runTimeout(t *testing.T, handler Handler) (error, *httptest.ResponseRecorder) {
	t.Helper()
	mw := TimeoutMiddleware(TimeoutOptions{Duration: 20 * time.Millisecond})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := withContext(req.Context())
	rec := httptest.NewRecorder()
	return mw(handler)(ctx, rec, req.WithContext(ctx)), rec
}

func TestTimeoutMiddlewareOutcomes(t *testing.T) {
	t.Run("late successful response is kept", func(t *testing.T) {
		err, rec := runTimeout(t, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			<-ctx.Done()
			w.WriteHeader(http.StatusCreated)
			return nil
		})
		if err != nil || rec.Code != http.StatusCreated {
			t.Fatalf("err = %v, status = %d; want nil, 201", err, rec.Code)
		}
	})
	t.Run("no response after deadline is 503", func(t *testing.T) {
		err, _ := runTimeout(t, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			<-ctx.Done()
			return nil
		})
		if appErr := WrapError(err, 0); appErr == nil || appErr.Code != http.StatusServiceUnavailable {
			t.Fatalf("err = %v, want 503", err)
		}
	})
	t.Run("wrapped deadline error is 503", func(t *testing.T) {
		err, _ := runTimeout(t, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			<-ctx.Done()
			return WrapError(ctx.Err(), http.StatusInternalServerError)
		})
		if appErr := WrapError(err, 0); appErr == nil || appErr.Code != http.StatusServiceUnavailable {
			t.Fatalf("err = %v, want 503", err)
		}
	})
	t.Run("other errors pass through", func(t *testing.T) {
		want := ErrBadRequest("bad", nil)
		err, _ := runTimeout(t, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			<-ctx.Done()
			return want
		})
		if err != want {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})
}

func TestAppErrorWithStatus(t *testing.T) {
	base := ErrBadRequest("bad", nil)
	withBiz := base.WithStatus(10012)

	if base.Status != 0 {
		t.Fatal("WithStatus must not modify the receiver")
	}
	if withBiz.Code != http.StatusBadRequest || withBiz.ResponseStatus() != 10012 {
		t.Fatalf("got Code %d ResponseStatus %d", withBiz.Code, withBiz.ResponseStatus())
	}
	if base.ResponseStatus() != http.StatusBadRequest {
		t.Fatalf("ResponseStatus without business code = %d, want HTTP code", base.ResponseStatus())
	}
	if got := WrapError(fmt.Errorf("ctx: %w", withBiz), http.StatusInternalServerError); got.ResponseStatus() != 10012 {
		t.Fatalf("WrapError lost business status: %+v", got)
	}
}

func TestWrapErrorFindsWrappedAppError(t *testing.T) {
	inner := ErrForbidden("no access", nil)
	wrapped := fmt.Errorf("check: %w", inner)

	got := WrapError(wrapped, http.StatusInternalServerError)
	if got.Code != http.StatusForbidden || got.Message != "no access" {
		t.Fatalf("WrapError = %+v, want 403 no access", got)
	}
	if !errors.Is(got, inner) {
		t.Fatal("wrapped chain lost")
	}
	if WrapError(inner, http.StatusInternalServerError) != inner {
		t.Fatal("an *AppError itself should be returned unchanged")
	}
}

func TestSensitiveKeyMatching(t *testing.T) {
	for _, key := range []string{"password", "userPassword", "accessToken", "access_token", "X-Api-Key", "client_secret", "privateKey", "Authorization", "pwd", "auth", "sessionId"} {
		if !isSensitiveKey(key) {
			t.Errorf("isSensitiveKey(%q) = false, want true", key)
		}
	}
	for _, key := range []string{"author", "name", "page", "email"} {
		if isSensitiveKey(key) {
			t.Errorf("isSensitiveKey(%q) = true, want false", key)
		}
	}
}

func TestLoggedLargeJSONResponseIsRedactedOrOmitted(t *testing.T) {
	big := `{"token":"t0p","data":"` + strings.Repeat("x", 8000) + `"}`
	rc := newResponseCapture(httptest.NewRecorder(), true, DefaultLogBodyLimit)
	rc.Write([]byte(big))
	body, total := rc.capturedBody()
	got := sanitizeCapturedBody(body, total, DefaultLogBodyLimit, "application/json")
	// Larger than the log limit but within the capture limit: redacted first,
	// then truncated (instead of the old "[invalid json body omitted]").
	if strings.Contains(got, "t0p") || strings.Contains(got, "omitted") || !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("logged body = %.80q..., want redacted then truncated", got)
	}

	huge := `{"token":"t0p","data":"` + strings.Repeat("x", logCaptureLimit) + `"}`
	rc = newResponseCapture(httptest.NewRecorder(), true, DefaultLogBodyLimit)
	rc.Write([]byte(huge))
	body, total = rc.capturedBody()
	got = sanitizeCapturedBody(body, total, DefaultLogBodyLimit, "application/json")
	if strings.Contains(got, "t0p") || !strings.Contains(got, "omitted") {
		t.Fatalf("logged body = %.80q, want omitted", got)
	}
}

// Middlewares added with Use wrap the route, so a writer-wrapping one such as
// compression must see the response stored by ctx.JSON.
func TestUseCompressionCompressesStoredResponse(t *testing.T) {
	app := NewApp(WithLogger(&recordLogger{}))
	app.Use(CompressionMiddleware())
	app.GET("/big", func(ctx *Context) error {
		return ctx.String(http.StatusOK, bigBody)
	})

	req := httptest.NewRequest(http.MethodGet, "/big", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
	}
	if got := gunzip(t, rec.Body.Bytes()); got != bigBody {
		t.Fatalf("body = %.40q, want stored response", got)
	}
}

// A middleware may answer with a stored response instead of calling next, but
// a response stored after next returns is discarded: the route's response was
// already written. The mistake is logged.
func TestMiddlewareStoredResponse(t *testing.T) {
	logs := &recordLogger{}
	app := NewApp(WithLogger(logs))
	app.Use(func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			if r.URL.Path == "/deny" {
				return GetContext(ctx).JSON(http.StatusForbidden, map[string]string{"error": "denied"})
			}
			if err := next(ctx, w, r); err != nil {
				return err
			}
			if r.URL.Path == "/late" {
				return GetContext(ctx).String(http.StatusTeapot, "late")
			}
			return nil
		}
	})
	app.GET("/deny", func(ctx *Context) error { return ctx.String(http.StatusOK, "route") })
	app.GET("/late", func(ctx *Context) error { return ctx.String(http.StatusOK, "route") })

	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/deny", nil))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "denied") {
		t.Fatalf("/deny: status = %d body = %q, want the middleware's 403", rec.Code, rec.Body.String())
	}

	logs.entries = nil
	rec = httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/late", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "route" {
		t.Fatalf("/late: status = %d body = %q, want the route's response", rec.Code, rec.Body.String())
	}
	if entry := logs.only(t); !strings.Contains(fmt.Sprint(entry.fields["err_message"]), "already written") {
		t.Fatalf("/late: log = %+v, want the discarded response reported", entry)
	}
}

// Setting the status directly and then storing a body is not an error: the
// status set first is kept and the stored body written.
func TestStatusThenStoredResponse(t *testing.T) {
	app := NewApp(WithLogger(&recordLogger{}))
	app.GET("/created", func(ctx *Context) error {
		ctx.ResponseWriter().WriteHeader(http.StatusCreated)
		return ctx.JSON(http.StatusOK, map[string]int{"id": 1})
	})

	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/created", nil))

	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"id":1`) {
		t.Fatalf("status = %d body = %q, want 201 with the stored body", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
}

func TestStoredResponseWithNoContentHasNoBody(t *testing.T) {
	logs := &recordLogger{}
	app := NewApp(WithLogger(logs))
	app.DELETE("/item", func(ctx *Context) error {
		return ctx.JSON(http.StatusNoContent, nil)
	})

	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/item", nil))

	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("status = %d body = %q, want empty 204", rec.Code, rec.Body.String())
	}
	if entry := logs.only(t); entry.fields["status"] != http.StatusNoContent || entry.level != "INFO" {
		t.Fatalf("log entry = %+v, want INFO with status 204", entry)
	}
}

// Headers set by outer middlewares are replaced, not duplicated, by the
// handler, and do not leak from a 1xx response into an error response.
func TestHandlerHeadersReplaceOuterHeaders(t *testing.T) {
	app := NewApp(WithLogger(&recordLogger{}))
	app.UseOuter(func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Cache-Control", "no-store")
			return next(ctx, w, r)
		}
	})
	app.GET("/cached", func(ctx *Context) error {
		ctx.ResponseWriter().Header().Set("Cache-Control", "max-age=60")
		return ctx.String(http.StatusOK, "ok")
	})
	app.GET("/hint-then-fail", func(ctx *Context) error {
		w := ctx.ResponseWriter()
		w.Header().Set("Link", "</a.css>; rel=preload")
		w.WriteHeader(http.StatusEarlyHints)
		return ErrBadRequest("bad", nil)
	})

	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/cached", nil))
	if got := rec.Header().Values("Cache-Control"); len(got) != 1 || got[0] != "max-age=60" {
		t.Fatalf("Cache-Control = %q, want only the handler value", got)
	}

	hinted := &statusLog{ResponseRecorder: httptest.NewRecorder()}
	app.Handler().ServeHTTP(hinted, httptest.NewRequest(http.MethodGet, "/hint-then-fail", nil))
	if len(hinted.codes) != 2 || hinted.codes[0] != http.StatusEarlyHints || hinted.headers[0].Get("Link") == "" {
		t.Fatalf("codes = %v headers = %v, want 103 with Link first", hinted.codes, hinted.headers)
	}
	if hinted.Code != http.StatusBadRequest || hinted.Header().Get("Link") != "" {
		t.Fatalf("status = %d Link = %q, want 400 without the 1xx headers", hinted.Code, hinted.Header().Get("Link"))
	}
	if hinted.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("outer header lost on error response: %q", hinted.Header().Get("Cache-Control"))
	}
}

func TestLoggerRecordsFinalStatusAfterEarlyHints(t *testing.T) {
	logs := &recordLogger{}
	app := NewApp(WithLogger(logs))
	app.GET("/hint", func(ctx *Context) error {
		ctx.ResponseWriter().WriteHeader(http.StatusEarlyHints)
		return ctx.String(http.StatusAccepted, "ok")
	})

	app.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hint", nil))

	if entry := logs.only(t); entry.fields["status"] != http.StatusAccepted {
		t.Fatalf("logged status = %v, want 202", entry.fields["status"])
	}
}

func TestErrorHandlerRepanicsErrAbortHandler(t *testing.T) {
	h := ErrorHandlerMiddleware()(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		panic(http.ErrAbortHandler)
	})
	defer func() {
		if p := recover(); p != http.ErrAbortHandler {
			t.Fatalf("recovered %v, want http.ErrAbortHandler", p)
		}
	}()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(withContext(req.Context()))
	_ = h(req.Context(), httptest.NewRecorder(), req)
}

func TestAppListenAndServeForcesCloseAfterShutdownTimeout(t *testing.T) {
	started := make(chan struct{})
	app := NewApp()
	app.GET("/slow", func(ctx *Context) error {
		close(started)
		time.Sleep(2 * time.Second)
		return nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.ListenAndServe(ctx, ServerConfig{Addr: addr, ShutdownTimeout: 50 * time.Millisecond})
	}()

	go func() {
		for i := 0; i < 50; i++ {
			resp, err := http.Get("http://" + addr + "/slow")
			if err == nil {
				resp.Body.Close()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	<-started
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ListenAndServe = %v, want shutdown deadline error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ListenAndServe did not return after shutdown timeout")
	}
	if app.currentServer() != nil {
		t.Fatal("server should be cleared after forced close")
	}
}

func TestServerTLSNegotiatesHTTP2(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)
	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", TLSCertFile: certFile, TLSKeyFile: keyFile})
	if err := srv.Start(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Proto))
	})); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
	}}
	resp, err := client.Get("https://" + srv.Addr())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Fatalf("protocol = %s, want HTTP/2", resp.Proto)
	}
}

func writeSelfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
