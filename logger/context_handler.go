package logger

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// LoggerCtxKey is the context key type for the logger context.
type LoggerCtxKey struct{}

// LoggerKey is the context key used to look up *LoggerContext.
// Exported so that host packages can answer Value() queries directly
// without going through context.WithValue chains.
var LoggerKey = LoggerCtxKey{}

type Field struct {
	Level slog.Level
	Key   string
	Value any
	Next  *Field
}

// LoggerContext holds per-request structured log fields that are attached to
// every log record emitted within the request's context.  It is safe for
// concurrent use: multiple goroutines (e.g. the handler goroutine and deferred
// middleware cleanup after a timeout) may call add() and Handle() in parallel.
type LoggerContext struct {
	mu   sync.RWMutex
	Head *Field
}

// Reset clears all accumulated log fields so the instance can be reused.
func (lc *LoggerContext) Reset() {
	lc.mu.Lock()
	lc.Head = nil
	lc.mu.Unlock()
}

type ContextHandler struct {
	slog.Handler
}

// please call WithContext First
func WithLoggerContext(ctx context.Context) context.Context {
	loggerCtx := GetLoggerContext(ctx)
	if loggerCtx == nil {
		loggerCtx = &LoggerContext{}
		return context.WithValue(ctx, LoggerKey, loggerCtx)
	}
	return ctx
}

func GetLoggerContext(ctx context.Context) *LoggerContext {
	loggerCtx := ctx.Value(LoggerKey)
	if lcx, ok := loggerCtx.(*LoggerContext); ok {
		return lcx
	}
	return nil
}

// Handle adds contextual attributes to the Record before calling the underlying
// handler.
func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if logCtx, ok := ctx.Value(LoggerKey).(*LoggerContext); ok {
		logCtx.mu.RLock()
		for node := logCtx.Head; node != nil; node = node.Next {
			// skip lower level field
			if node.Level < r.Level {
				continue
			}
			attr := slog.Attr{
				Key:   node.Key,
				Value: slog.AnyValue(node.Value),
			}
			r.AddAttrs(attr)
		}
		logCtx.mu.RUnlock()
	}
	return h.Handler.Handle(ctx, r)
}

func newContextHandler(target io.Writer, format string, opts *slog.HandlerOptions) *ContextHandler {
	switch strings.ToLower(format) {
	case "json":
		return &ContextHandler{slog.NewJSONHandler(target, opts)}
	case "text":
		fallthrough
	default:
		return &ContextHandler{slog.NewTextHandler(target, opts)}
	}
}

func (logCtx *LoggerContext) add(key string, value any, level slog.Level) {
	if logCtx == nil {
		return
	}

	logCtx.mu.Lock()
	defer logCtx.mu.Unlock()

	if logCtx.Head == nil {
		logCtx.Head = &Field{
			Level: level,
			Key:   key,
			Value: value,
		}
		return
	}

	var last *Field
	for node := logCtx.Head; node != nil; node = node.Next {
		if node.Key == key {
			node.Value = value
			node.Level = level
			return
		}
		last = node
	}

	last.Next = &Field{
		Level: level,
		Key:   key,
		Value: value,
	}
}

func AddDebug(ctx context.Context, key string, value any) {
	addLog(ctx, LevelDebug, key, value)
}

func AddTrace(ctx context.Context, key string, value any) {
	addLog(ctx, LevelTrace, key, value)
}

func AddInfo(ctx context.Context, key string, value any) {
	addLog(ctx, LevelInfo, key, value)
}

func AddWarning(ctx context.Context, key string, value any) {
	addLog(ctx, LevelWarning, key, value)
}

func AddFatal(ctx context.Context, key string, value any) {
	addLog(ctx, LevelFatal, key, value)
}

func addLog(ctx context.Context, level slog.Level, key string, value any) {
	lcx := ctx.Value(LoggerKey)
	logCtx, ok := lcx.(*LoggerContext)
	if !ok {
		// Fail silently: ctx was not initialized with WithLoggerContext.
		return
	}
	logCtx.add(key, value, level)
}
