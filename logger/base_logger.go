package logger

import (
	"context"
	"log/slog"
	"runtime"
	"time"
)

// logRecord creates and handles a slog.Record with the given parameters.
func logRecord(ctx context.Context, handler slog.Handler, level slog.Level, msg string, callerSkip int, args ...any) error {
	var pc uintptr
	var pcs [1]uintptr
	runtime.Callers(callerSkip, pcs[:])
	pc = pcs[0]

	r := slog.NewRecord(time.Now(), level, msg, pc)
	r.Add(args...)

	if ctx == nil {
		ctx = context.Background()
	}

	return handler.Handle(ctx, r)
}
