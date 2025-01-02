package logger

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

type ctxKey string

const (
	slogFields ctxKey = "logger_slog_fields"
)

type ContextHandler struct {
	slog.Handler
}

// Handle adds contextual attributes to the Record before calling the underlying
// handler
func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(slogFields).([]slog.Attr); ok {
		for _, v := range attrs {
			r.AddAttrs(v)
		}
	}
	return h.Handler.Handle(ctx, r)
}

// AppendCtx adds an slog attribute to the provided context so that it will be
// included in any Record created with such context
func AppendCtx(parent context.Context, attr slog.Attr) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	if v, ok := parent.Value(slogFields).([]slog.Attr); ok {
		v = append(v, attr)
		return context.WithValue(parent, slogFields, v)
	}
	v := []slog.Attr{}
	v = append(v, attr)
	return context.WithValue(parent, slogFields, v)
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
