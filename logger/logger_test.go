package logger

import (
	"context"
	"testing"
)

func TestLogger(t *testing.T) {
	ctx := WithLoggerContext(context.Background())
	log, _ := NewLogger("logs/logger.toml")
	log.Debug(ctx, "debug")
	log.Trace(ctx, "trace")
	log.Info(ctx, "info")
	log.Warning(ctx, "warning")
	log.Fatal(ctx, "fatal")

	AddDebug(ctx, "request-id", "request-id_testing")
	AddInfo(ctx, "request-time", "request-time_testing")
	AddWarning(ctx, "request-day", "request-day_testing")
	log.Info(ctx, "info with context")
}

func TestStruct(t *testing.T) {
	type User struct {
		ID        string `json:"id"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Email     string `json:"email"`
		Password  string `json:"password"`
	}
	ctx := WithLoggerContext(context.Background())
	log, _ := NewLogger("logs/logger.toml")
	u := &User{
		ID:        "user-12234",
		FirstName: "Jan",
		LastName:  "Doe",
		Email:     "jan@example.com",
		Password:  "pass-12334",
	}
	log.Info(ctx, "info", "user", u)
}

func TestConsole(t *testing.T) {
	ctx := WithLoggerContext(context.Background())
	log, _ := NewLogger()
	log.Debug(ctx, "debug")
}

func TestRotate(t *testing.T) {
	ctx := WithLoggerContext(context.Background())
	log, _ := NewLogger("logs/logger.toml")
	for i := 0; i < 10000; i++ {
		log.Info(ctx, "info", "times", i)
	}
	log.Trace(ctx, "new file")
	log.Info(ctx, "new file")
}
