package golitekit

import (
	"github.com/hansir-hsj/GoLiteKit/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Services holds all external dependencies (DB, Redis, Logger, etc.)
type Services struct {
	db          *gorm.DB
	redis       *redis.Client
	logger      logger.Logger
	panicLogger *logger.PanicLogger
}

type ServiceOption func(*Services)

func WithDB(db *gorm.DB) ServiceOption {
	return func(s *Services) { s.db = db }
}

func WithRedis(client *redis.Client) ServiceOption {
	return func(s *Services) { s.redis = client }
}

func WithLogger(l logger.Logger) ServiceOption {
	return func(s *Services) { s.logger = l }
}

func WithPanicLogger(pl *logger.PanicLogger) ServiceOption {
	return func(s *Services) { s.panicLogger = pl }
}

func (s *Services) DB() *gorm.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Services) Redis() *redis.Client {
	if s == nil {
		return nil
	}
	return s.redis
}

func (s *Services) Logger() logger.Logger {
	if s == nil {
		return nil
	}
	return s.logger
}

func (s *Services) PanicLogger() *logger.PanicLogger {
	if s == nil {
		return nil
	}
	return s.panicLogger
}
