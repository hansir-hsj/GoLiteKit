package redis

import (
	"testing"
	"time"
)

func TestParseSampleConfig(t *testing.T) {
	cfg, err := parse("redis.toml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	opts := buildOptions(cfg)
	if opts.PoolTimeout != time.Second {
		t.Errorf("PoolTimeout = %v, want 1s", opts.PoolTimeout)
	}
	if opts.DialTimeout != 3*time.Second || opts.ReadTimeout != 3*time.Second || opts.WriteTimeout != 3*time.Second {
		t.Errorf("timeouts = %v/%v/%v, want 3s", opts.DialTimeout, opts.ReadTimeout, opts.WriteTimeout)
	}
	if opts.PoolSize != 10 || opts.MinIdleConns != 1 || opts.MaxIdleConns != 5 {
		t.Errorf("pool = %d/%d/%d", opts.PoolSize, opts.MinIdleConns, opts.MaxIdleConns)
	}
}
