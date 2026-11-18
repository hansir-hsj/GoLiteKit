package golitekit

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestAppCloseRunsClosersInReverseOnce(t *testing.T) {
	app := NewApp()
	var order []int
	errA := errors.New("a failed")
	app.OnClose(func(context.Context) error { order = append(order, 1); return errA })
	app.OnClose(func(context.Context) error { order = append(order, 2); return nil })
	app.OnClose(func(context.Context) error { order = append(order, 3); return nil })

	err := app.Close(context.Background())
	if !errors.Is(err, errA) {
		t.Fatalf("Close error = %v, want it to include errA", err)
	}
	if len(order) != 3 || order[0] != 3 || order[1] != 2 || order[2] != 1 {
		t.Fatalf("close order = %v, want [3 2 1] (all run even after a failure)", order)
	}

	if err2 := app.Close(context.Background()); !errors.Is(err2, errA) || len(order) != 3 {
		t.Fatalf("second Close ran closers again or changed result: %v %v", err2, order)
	}
}

// Clients passed with WithDB/WithRedis belong to the caller: closing the app
// leaves them usable, e.g. by another app sharing them.
func TestAppCloseLeavesCallerDBAndRedisOpen(t *testing.T) {
	// Neither client connects until used, so no servers are needed.
	gdb, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{
		DSN:                       "u:p@tcp(127.0.0.1:1)/db",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer rdb.Close()

	app := NewApp(WithDB(gdb), WithRedis(rdb))
	if err := app.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := sqlDB.PingContext(context.Background()); err != nil && err.Error() == "sql: database is closed" {
		t.Fatal("app closed the caller's DB")
	}
	if err := rdb.Ping(context.Background()).Err(); errors.Is(err, redis.ErrClosed) {
		t.Fatal("app closed the caller's Redis client")
	}
}

// The documented way to tie a client's lifetime to the app.
func TestAppOnCloseClosesRegisteredClient(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	app := NewApp(WithRedis(rdb))
	app.OnClose(func(context.Context) error { return rdb.Close() })

	if err := app.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := rdb.Ping(context.Background()).Err(); !errors.Is(err, redis.ErrClosed) {
		t.Fatalf("redis ping after Close = %v, want ErrClosed", err)
	}
}

func TestAppListenAndServeClosesAppAfterShutdown(t *testing.T) {
	app := NewApp()
	closed := make(chan struct{})
	app.OnClose(func(context.Context) error { close(closed); return nil })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.ListenAndServe(ctx, ServerConfig{Addr: addr}) }()

	// Wait until the server is up, then stop it.
	for i := 0; i < 100; i++ {
		if resp, err := http.Get("http://" + addr + "/"); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ListenAndServe = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe did not return")
	}
	select {
	case <-closed:
	default:
		t.Fatal("app was not closed after ListenAndServe returned")
	}
}

func TestAppShutdownDoesNotCloseResources(t *testing.T) {
	app := NewApp()
	closed := false
	app.OnClose(func(context.Context) error { closed = true; return nil })

	if err := app.Start(ServerConfig{Addr: "127.0.0.1:0"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if closed {
		t.Fatal("Shutdown must not close resources so the app can be restarted")
	}
}
