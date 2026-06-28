package golitekit

import (
	"fmt"
	"sync"

	"github.com/hansir-hsj/GoLiteKit/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Services holds framework dependencies and startup-registered custom services.
type Services struct {
	db                      *gorm.DB
	redis                   *redis.Client
	logger                  logger.Logger
	panicLogger             *logger.PanicLogger
	observer                Observer
	observabilityMiddleware Middleware

	mu     sync.RWMutex
	custom map[string]any
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

// WithService registers a named custom service during app construction.
func WithService(key string, value any) ServiceOption {
	return func(s *Services) { s.registerCustom(key, value) }
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

func (s *Services) registerCustom(key string, value any) {
	if key == "" {
		panic("golitekit: service key must not be empty")
	}
	if value == nil {
		panic(fmt.Sprintf("golitekit: service %q must not be nil", key))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.custom == nil {
		s.custom = make(map[string]any)
	}
	s.custom[key] = value
}

func (s *Services) customService(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.custom == nil {
		return nil
	}
	return s.custom[key]
}
