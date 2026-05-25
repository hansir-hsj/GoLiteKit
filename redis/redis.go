package redis

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hansir-hsj/GoLiteKit/config"
	"github.com/hansir-hsj/GoLiteKit/env"

	"github.com/redis/go-redis/v9"
)

type RConfigTimeout struct {
	PoolTimeout  int `toml:"poolTimeout"`
	DialTimeout  int `toml:"dialTimeout"`
	ReadTimeout  int `toml:"readTimeout"`
	WriteTimeout int `toml:"writeTimeout"`
}

type RConfigConn struct {
	PoolSize     int `toml:"poolSize"`
	MinIdleConns int `toml:"minIdleConns"`
	MaxIdleConns int `toml:"maxIdleConns"`
}

type RConfig struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Protocol string `toml:"protocol"`
	DB       int    `toml:"db"`

	RConfigTimeout `toml:"Timeout"`
	RConfigConn    `toml:"Conn"`
}

type Config struct {
	RConfig `toml:"redis"`
}

func parse(conf string) (*Config, error) {
	var redisConfig Config
	if err := config.Parse(conf, &redisConfig); err != nil {
		return nil, fmt.Errorf("failed to parse redis config: %w", err)
	}
	return &redisConfig, nil
}

func buildOptions(cfg *Config) redis.Options {
	opts := redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	}

	if cfg.Username != "" {
		opts.Username = cfg.Username
	}
	if cfg.Protocol != "" {
		if p, err := strconv.Atoi(cfg.Protocol); err == nil {
			opts.Protocol = p
		}
	}

	if cfg.PoolTimeout > 0 {
		opts.PoolTimeout = time.Duration(cfg.PoolTimeout) * time.Millisecond
	}
	if cfg.DialTimeout > 0 {
		opts.DialTimeout = time.Duration(cfg.DialTimeout) * time.Millisecond
	}
	if cfg.ReadTimeout > 0 {
		opts.ReadTimeout = time.Duration(cfg.ReadTimeout) * time.Millisecond
	}
	if cfg.WriteTimeout > 0 {
		opts.WriteTimeout = time.Duration(cfg.WriteTimeout) * time.Millisecond
	}

	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}
	if cfg.MinIdleConns > 0 {
		opts.MinIdleConns = cfg.MinIdleConns
	}
	if cfg.MaxIdleConns > 0 {
		opts.MaxIdleConns = cfg.MaxIdleConns
	}

	return opts
}

// NewFromConfig creates a new Redis client from config file
func NewFromConfig(conf ...string) (*redis.Client, error) {
	var redisConf string
	if len(conf) > 0 {
		redisConf = conf[0]
	} else {
		redisConf = filepath.Join(env.ConfDir(), "redis.toml")
	}

	cfg, err := parse(redisConf)
	if err != nil {
		return nil, err
	}

	opts := buildOptions(cfg)
	rdb := redis.NewClient(&opts)

	pong, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		rdb.Close()
		return nil, fmt.Errorf("redis ping error: %w", err)
	}
	if pong != "PONG" {
		rdb.Close()
		return nil, fmt.Errorf("redis ping failed: unexpected response %s", pong)
	}

	return rdb, nil
}

// Close closes the Redis connection
func Close(client *redis.Client) error {
	if client == nil {
		return nil
	}
	return client.Close()
}

// Ping checks if the Redis connection is alive
func Ping(ctx context.Context, client *redis.Client) error {
	if client == nil {
		return fmt.Errorf("redis client is nil")
	}
	return client.Ping(ctx).Err()
}
