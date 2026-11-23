package db

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseSampleConfig(t *testing.T) {
	cfg, err := parse("db.toml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Timeout != 1500 || cfg.ReadTimeout != 1500 || cfg.WriteTimeout != 1500 {
		t.Errorf("timeouts not decoded: %+v", cfg.DbTimeout)
	}
	if cfg.MaxOpenConns != 10 || cfg.MaxIdleConns != 5 || cfg.ConnMaxLifeTime != 600000 {
		t.Errorf("conn pool not decoded: %+v", cfg.DbConn)
	}
	for _, want := range []string{"parseTime=true", "timeout=1.5s", "readTimeout=1.5s", "tcp(127.0.0.1:3306)"} {
		if !strings.Contains(cfg.DSN, want) {
			t.Errorf("DSN %q missing %q", cfg.DSN, want)
		}
	}
}

func TestDefaultDialTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.toml")
	if err := os.WriteFile(path, []byte("[db]\nhost = \"127.0.0.1\"\nport = 3306\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.DSN, "timeout=5s") {
		t.Fatalf("DSN %q should default the dial timeout to 5s", cfg.DSN)
	}
}

func TestNonMySQLDriverRequiresDSN(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.toml")
	if err := os.WriteFile(path, []byte("[db]\ndriver = \"sqlite\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := parse(path); err == nil || !strings.Contains(err.Error(), "dsn is required") {
		t.Fatalf("parse err = %v, want dsn required", err)
	}
}

func TestUnknownDriver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.toml")
	if err := os.WriteFile(path, []byte("[db]\ndriver = \"nope\"\ndsn = \"x\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFromConfig(path); err == nil || !strings.Contains(err.Error(), "unknown database driver") {
		t.Fatalf("NewFromConfig err = %v, want unknown driver", err)
	}
}

// A server that accepts but never sends the MySQL greeting used to hang the
// startup ping forever (no read timeout, no context).
func TestNewFromConfigPingTimesOut(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close() // hold the connection open without replying
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	path := filepath.Join(t.TempDir(), "db.toml")
	content := fmt.Sprintf("[db]\nhost = \"127.0.0.1\"\nport = %d\n[db.Timeout]\ntimeout = 300\n", port)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = NewFromConfig(path)
	if err == nil {
		t.Fatal("expected ping error")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("NewFromConfig took %v, want it bounded by the 300ms timeout", elapsed)
	}
}
