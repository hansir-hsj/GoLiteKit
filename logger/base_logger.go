package logger

import (
	"context"
	"log/slog"
	"runtime"
	"time"
)

// logRecord creates and handles a slog.Record with the given parameters. The
// caller's program counter is only collected when addSource is set, since
// runtime.Callers is comparatively expensive.
func logRecord(ctx context.Context, handler slog.Handler, level slog.Level, msg string, callerSkip int, addSource bool, args ...any) error {
	var pc uintptr
	if addSource {
		var pcs [1]uintptr
		runtime.Callers(callerSkip, pcs[:])
		pc = pcs[0]
	}

	r := slog.NewRecord(time.Now(), level, msg, pc)
	r.Add(args...)

	if ctx == nil {
		ctx = context.Background()
	}

	return handler.Handle(ctx, r)
}
