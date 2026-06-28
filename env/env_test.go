package env

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
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

		if RateLimit() != 100 {
			t.Errorf("RateLimit = %v, want %v", RateLimit(), 100)
		}
	})
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
