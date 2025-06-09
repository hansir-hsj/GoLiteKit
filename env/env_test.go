package env

import (
	"testing"
)

func TestInit(t *testing.T) {
	t.Run("successful initialization", func(t *testing.T) {
		if err := Init("app.toml"); err != nil {
			t.Errorf("Init() error = %v", err)
		}

		if defaultEnv.AppName != "golitekit" {
			t.Errorf("AppName = %v, want %v", defaultEnv.AppName, "golitekit")
		}

		if defaultEnv.Network != "tcp4" {
			t.Errorf("Network = %v, want %v", defaultEnv.Network, "tcp4")
		}

		if defaultEnv.RateLimit != 100 {
			t.Errorf("RateLimit = %v, want %v", defaultEnv.RateLimit, 100)
		}
	})
}
