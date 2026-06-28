package golitekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hansir-hsj/GoLiteKit/env"
	"github.com/hansir-hsj/GoLiteKit/logger"
)

type captureLogger struct {
	fields map[string]any
}

func (l *captureLogger) Debug(ctx context.Context, msg string, args ...any)   {}
func (l *captureLogger) Trace(ctx context.Context, msg string, args ...any)   {}
func (l *captureLogger) Warning(ctx context.Context, msg string, args ...any) {}
func (l *captureLogger) Error(ctx context.Context, msg string, args ...any)   {}
func (l *captureLogger) Fatal(ctx context.Context, msg string, args ...any)   {}
func (l *captureLogger) Close() error                                         { return nil }

func (l *captureLogger) Info(ctx context.Context, msg string, args ...any) {
	logCtx := logger.GetLoggerContext(ctx)
	if logCtx == nil {
		return
	}
	for field := logCtx.Head; field != nil; field = field.Next {
		l.fields[field.Key] = field.Value
	}
}

func writeAppConfig(t *testing.T, dir string, logResponseBody bool, writeTimeout int) string {
	t.Helper()
	path := filepath.Join(dir, "app.toml")
	value := "false"
	if logResponseBody {
		value = "true"
	}
	content := `[HttpServer]
appName = "test"
network = "tcp"
addr = ":0"

[HttpServer.Timeout]
writeTimeout = ` + strconv.Itoa(writeTimeout) + `

[HttpServer.Logger]
logResponseBody = ` + value + `
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write app config: %v", err)
	}
	return path
}

func TestNewAppFromConfigFreezesLoggerOptions(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	firstConfig := writeAppConfig(t, firstDir, true, 1000)
	secondConfig := writeAppConfig(t, secondDir, false, 1000)

	log := &captureLogger{fields: make(map[string]any)}
	panicLog, err := logger.NewPanicLogger()
	if err != nil {
		t.Fatalf("NewPanicLogger: %v", err)
	}
	defer panicLog.Close()

	app, err := NewAppFromConfig(firstConfig, WithLogger(log), WithPanicLogger(panicLog))
	if err != nil {
		t.Fatalf("NewAppFromConfig: %v", err)
	}
	app.GET("/body", func(ctx *Context) error {
		return ctx.JSON(200, map[string]string{"ok": "yes"})
	})

	if err := env.Init(secondConfig); err != nil {
		t.Fatalf("second env init: %v", err)
	}

	req := httptest.NewRequest("GET", "/body", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if _, ok := log.fields["response"]; !ok {
		t.Fatalf("expected response body to be logged from app config, fields=%v", log.fields)
	}
}

func TestNewAppFromConfigFreezesTimeoutOptions(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	firstConfig := writeAppConfig(t, firstDir, false, 1000)
	secondConfig := writeAppConfig(t, secondDir, false, 50)

	panicLog, err := logger.NewPanicLogger()
	if err != nil {
		t.Fatalf("NewPanicLogger: %v", err)
	}
	defer panicLog.Close()

	var remaining time.Duration
	app, err := NewAppFromConfig(firstConfig, WithPanicLogger(panicLog))
	if err != nil {
		t.Fatalf("NewAppFromConfig: %v", err)
	}
	app.Use(func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("expected request deadline")
			}
			remaining = time.Until(deadline)
			return next(ctx, w, r)
		}
	})
	app.GET("/deadline", func(ctx *Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"ok": "yes"})
	})

	if err := env.Init(secondConfig); err != nil {
		t.Fatalf("second env init: %v", err)
	}

	req := httptest.NewRequest("GET", "/deadline", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if remaining < 500*time.Millisecond {
		t.Fatalf("expected first config timeout to remain active, remaining=%v", remaining)
	}
}
