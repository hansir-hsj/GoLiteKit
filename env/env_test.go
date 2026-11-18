package env

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestInit(t *testing.T) {
	t.Run("successful initialization", func(t *testing.T) {
		if err := Init("app.toml"); err != nil {
			t.Errorf("Init() error = %v", err)
		}

		if AppName() != "golitekit" {
			t.Errorf("AppName = %v, want %v", AppName(), "golitekit")
		}

		if Network() != "tcp4" {
			t.Errorf("Network = %v, want %v", Network(), "tcp4")
		}
	})
}

func TestPathResolution(t *testing.T) {
	confDir := t.TempDir()
	path := filepath.Join(confDir, "app.toml")
	content := `[HttpServer]
[HttpServer.Logger]
configFile = "logger.toml"
[HttpServer.DB]
configFile = "/etc/myapp/db.toml"
[HttpServer.TLSConfig]
certFile = "tls/server.crt"
[HttpServer.Static]
staticDir = "/srv/static"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	e, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()

	for name, c := range map[string][2]string{
		"ConfDir":          {e.ConfDir(), confDir},
		"LoggerConfigFile": {e.LoggerConfigFile(), filepath.Join(confDir, "logger.toml")},
		"DBConfigFile":     {e.DBConfigFile(), "/etc/myapp/db.toml"},
		"RedisConfigFile":  {e.RedisConfigFile(), ""},
		"TLSCertFile":      {e.TLSCertFile(), filepath.Join(confDir, "tls/server.crt")},
		"StaticDir":        {e.StaticDir(), "/srv/static"},
		"RootDir":          {e.RootDir(), cwd},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestRelativeStaticDirUsesWorkingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.toml")
	if err := os.WriteFile(path, []byte("[HttpServer.Static]\nstaticDir = \"static\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if got := e.StaticDir(); got != filepath.Join(cwd, "static") {
		t.Fatalf("StaticDir = %q, want %q", got, filepath.Join(cwd, "static"))
	}
}

func TestConcurrentInitAndReads(t *testing.T) {
	configA := writeEnvConfig(t, t.TempDir(), "a")
	configB := writeEnvConfig(t, t.TempDir(), "b")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := configA
			if i%2 == 0 {
				path = configB
			}
			if err := Init(path); err != nil {
				t.Errorf("Init(%q): %v", path, err)
			}
		}(i)
	}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = AppName()
			_ = Network()
			_ = WriteTimeout()
			_ = LoggerConfigFile()
			_ = LogResponseBody()
		}()
	}
	wg.Wait()
}

func TestLoadDoesNotChangeDefault(t *testing.T) {
	if err := Init(writeEnvConfig(t, t.TempDir(), "a")); err != nil {
		t.Fatal(err)
	}
	b, err := Load(writeEnvConfig(t, t.TempDir(), "b"))
	if err != nil {
		t.Fatal(err)
	}
	if b.AppName() != "b" || b.WriteTimeout() != time.Second {
		t.Fatalf("loaded env = %q %v", b.AppName(), b.WriteTimeout())
	}
	if AppName() != "a" {
		t.Fatalf("default AppName = %q, want a (Load must not replace the default)", AppName())
	}
}

func TestNilEnvReturnsDefaults(t *testing.T) {
	var e *Env
	if e.WriteTimeout() != DefaultWriteTimeout || e.MaxHeaderBytes() != DefaultMaxHeaderBytes ||
		e.SSETimeout() != DefaultSSETimeout || e.Addr() != "" || e.TLS() || e.LoggerConfigFile() != "" {
		t.Fatal("nil *Env should return defaults")
	}
}

func writeEnvConfig(t *testing.T, dir, appName string) string {
	t.Helper()
	path := filepath.Join(dir, "app.toml")
	content := `[HttpServer]
appName = "` + appName + `"
network = "tcp"
addr = ":0"

[HttpServer.Timeout]
writeTimeout = 1000
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write env config: %v", err)
	}
	return path
}
