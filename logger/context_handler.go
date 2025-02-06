package logger

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

type loggerCtxKey string

const (
	loggerKey loggerCtxKey = "logger_ctx_key"
)

type Field struct {
	Level slog.Level
	Key   string
	Value any
	Next  *Field
}

type LoggerContext struct {
	Head *Field
}

type ContextHandler struct {
	slog.Handler
}

// please call WithContext First
func WithLoggerContext(ctx context.Context) context.Context {
	loggerCtx := GetLoggerContext(ctx)
	if loggerCtx == nil {
		loggerCtx = &LoggerContext{}
		return context.WithValue(ctx, loggerKey, loggerCtx)
	}
	return ctx
}

func GetLoggerContext(ctx context.Context) *LoggerContext {
	loggerCtx := ctx.Value(loggerKey)
	if lcx, ok := loggerCtx.(*LoggerContext); ok {
		return lcx
	}
	return nil
}

// Handle adds contextual attributes to the Record before calling the underlying
// handler
func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if logCtx, ok := ctx.Value(loggerKey).(*LoggerContext); ok {
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

	if logCtx.Head == nil {
		logCtx.Head = &Field{
			Level: level,
			Key:   key,
			Value: value,
		}
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
	lcx := ctx.Value(loggerKey)
	logCtx, ok := lcx.(*LoggerContext)
	if !ok {
		panic("LoggerContext not init, please call WithLoggerContext first")
	}
	logCtx.add(key, value, level)
}
