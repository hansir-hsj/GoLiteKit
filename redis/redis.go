package redis

import (
	"context"
	"fmt"
	"path/filepath"
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

	redis.Options
}

func parse(conf string) (*Config, error) {
	var redisConfig Config
	if err := config.Parse(conf, &redisConfig); err != nil {
		return nil, fmt.Errorf("failed to parse redis config: %w", err)
	}

	options := redis.Options{
		Addr:     fmt.Sprintf("%s:%d", redisConfig.Host, redisConfig.Port),
		Username: redisConfig.RConfig.Username,
		Password: redisConfig.RConfig.Password,
		DB:       redisConfig.RConfig.DB,
	}

	if redisConfig.RConfigTimeout.DialTimeout > 0 {
		options.DialTimeout = time.Duration(redisConfig.RConfigTimeout.DialTimeout) * time.Millisecond
	}
	if redisConfig.RConfigTimeout.ReadTimeout > 0 {
		options.ReadTimeout = time.Duration(redisConfig.RConfigTimeout.ReadTimeout) * time.Millisecond
	}
	if redisConfig.RConfigTimeout.WriteTimeout > 0 {
		options.WriteTimeout = time.Duration(redisConfig.RConfigTimeout.WriteTimeout) * time.Millisecond
	}
	if redisConfig.RConfigTimeout.PoolTimeout > 0 {
		options.PoolTimeout = time.Duration(redisConfig.RConfigTimeout.PoolTimeout) * time.Millisecond
	}
	if redisConfig.RConfigConn.PoolSize > 0 {
		options.PoolSize = redisConfig.RConfigConn.PoolSize
	}
	if redisConfig.RConfigConn.MinIdleConns > 0 {
		options.MinIdleConns = redisConfig.RConfigConn.MinIdleConns
	}
	if redisConfig.RConfigConn.MaxIdleConns > 0 {
		options.MaxIdleConns = redisConfig.RConfigConn.MaxIdleConns
	}

	redisConfig.Options = options

	return &redisConfig, nil
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

	rdb := redis.NewClient(&cfg.Options)
	pong, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		return nil, fmt.Errorf("redis ping error: %w", err)
	}
	if pong != "PONG" {
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
