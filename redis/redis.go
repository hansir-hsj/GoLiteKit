package redis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hansir-hsj/GoLiteKit/config"
	"github.com/hansir-hsj/GoLiteKit/env"

	"github.com/redis/go-redis/v9"
)

var (
	defaultRedis *RedisClient
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

	RConfigTimeout `toml:"Timeout"`
	RConfigConn    `toml:"Conn"`
}

type Config struct {
	RConfig `toml:"redis"`

	redis.Options
}

type RedisClient struct {
	*redis.Client
}

func NewRedis() *RedisClient {
	return defaultRedis
}

func parse(conf string) (*Config, error) {
	var redisConfig Config
	if err := config.Parse(conf, &redisConfig); err != nil {
		return nil, err
	}

	options := redis.Options{
		Addr:     fmt.Sprintf("%s:%d", redisConfig.Host, redisConfig.Port),
		Password: redisConfig.RConfig.Password,
	}
	redisConfig.Options = options

	return &redisConfig, nil
}

func Init(conf ...string) error {
	var redisConf string
	if len(conf) > 0 {
		redisConf = conf[0]
	} else {
		redisConf = filepath.Join(env.ConfDir(), "redis.toml")
	}
	config, err := parse(redisConf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "redis config parse error: %v\n", err)
		return err
	}

	rdb := redis.NewClient(&config.Options)
	pong, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "redis ping error: %v\n", err)
		return err
	}
	if pong != "PONG" {
		fmt.Fprintf(os.Stderr, "redis ping error: %v\n", err)
		return err
	}

	client := &RedisClient{
		Client: rdb,
	}

	defaultRedis = client

	return nil
}
