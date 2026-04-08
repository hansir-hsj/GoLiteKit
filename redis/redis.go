package redis

import (
	"context"
	"fmt"
	"path/filepath"

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
		Password: redisConfig.RConfig.Password,
		DB:       redisConfig.RConfig.DB,
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
