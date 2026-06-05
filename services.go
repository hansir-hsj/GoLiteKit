package golitekit

import (
	"sync"

	"github.com/hansir-hsj/GoLiteKit/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Services holds all external dependencies (DB, Redis, Logger, etc.)
type Services struct {
	db                      *gorm.DB
	redis                   *redis.Client
	logger                  logger.Logger
	panicLogger             *logger.PanicLogger
	observer                Observer
	observabilityMiddleware Middleware

	mu       sync.RWMutex
	registry map[string]any
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

func WithObserver(observer Observer) ServiceOption {
	return func(s *Services) { s.observer = observer }
}

func WithObservabilityMiddleware(m Middleware) ServiceOption {
	return func(s *Services) { s.observabilityMiddleware = m }
}

// WithService registers a named service in the generic registry.
func WithService(key string, value any) ServiceOption {
	return func(s *Services) { s.Set(key, value) }
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

func (s *Services) Observer() Observer {
	if s == nil {
		return nil
	}
	return s.observer
}

func (s *Services) ObservabilityMiddleware() Middleware {
	if s == nil {
		return nil
	}
	return s.observabilityMiddleware
}

// Set stores a named service.
func (s *Services) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.registry == nil {
		s.registry = make(map[string]any)
	}
	s.registry[key] = value
}

// Get retrieves a named service.
func (s *Services) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.registry == nil {
		return nil
	}
	return s.registry[key]
}
